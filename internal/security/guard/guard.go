package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/security/detector"
	"modern-dhcp/internal/security/ipsgdai"
	"modern-dhcp/internal/security/maclist"
	"modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/security/radius"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

var (
	ErrUntrustedBinding  = errors.New("security guard: untrusted binding")
	ErrRateLimited       = errors.New("security guard: rate limited")
	ErrSuspiciousTraffic = errors.New("security guard: suspicious traffic")
	ErrMACNotAllowed     = errors.New("security guard: mac not allowed")
	ErrPolicyDenied      = errors.New("security guard: policy violation")
	errNilContext        = errors.New("security guard: context is nil")
)

// Guard orchestrates pre-processing checks before DHCP logic executes.
type Guard interface {
	Check(ctx context.Context, ctxData *Context) error
	CheckRenewACL(ctx context.Context, mac string) error
	OnLeaseChange(ctx context.Context, change *LeaseChange)
	OnSecurityEvent(ctx context.Context, evt Event)
}

// Context captures the minimal metadata required for guard evaluation.
type Context struct {
	TenantID     string
	MessageType  string
	RequestPhase string
	MAC          string
	ClientID     string
	CircuitID    string
	RemoteID     string
	RelayAgent   map[string]string
	PortID       string
	VLANID       int
	InterfaceID  string
	RequestedIP  string
	GIAddr       string
	Timestamp    time.Time
}

// LeaseChange describes lease lifecycle events that should propagate to network controllers.
type LeaseChange struct {
	TenantID    string
	MACAddress  string
	ClientID    string
	IPAddress   string
	PoolID      string
	VLANID      *int
	InterfaceID string
	PortID      string
	ExpiresAt   time.Time
	Action      string
	MDMManaged  bool
	MDMSource   string
	MDMTags     []string
	MDMObserved *time.Time
}

// Lease change actions pushed to downstream systems.
const (
	LeaseActionGranted  = "granted"
	LeaseActionRenewed  = "renewed"
	LeaseActionReleased = "released"
)

// EventType enumerates security signal categories.
type EventType string

const (
	EventTypeSnooping EventType = "snooping"
	EventTypeRate     EventType = "ratelimit"
	EventTypeDetector EventType = "detector"
	EventTypeIPSG     EventType = "ipsg"
	EventTypeRADIUS   EventType = "radius"
	EventTypeACL      EventType = "acl"
)

// GraylistAction defines how a graylisted MAC should be treated.
type GraylistAction string

const (
	GraylistActionMonitor GraylistAction = "monitor"
	GraylistActionBlock   GraylistAction = "block"
)

// MACACL captures whitelist/blacklist/graylist policies.
type MACACL struct {
	Whitelist          []string
	Blacklist          []string
	Graylist           []string
	GraylistAction     GraylistAction
	EnforceWhitelist   bool
	BindingExemptACL   bool
	RenewExemptBlocked bool
	DefaultAction      string
}

const macACLDefaultActionAllow = "allow"

// Event encapsulates asynchronous signals (DAI alerts, RADIUS CoA, etc.).
type Event struct {
	TenantID string
	Type     EventType
	Reason   string
	Details  map[string]any
}

// Dependencies describes optional subsystems wired into the guard pipeline.
type Dependencies struct {
	Snooping         snooping.Store
	Limiter          ratelimit.Limiter
	BindingService   *pool.Service
	LeaseService     *lease.Service
	Detector         detector.Detector
	Publisher        ipsgdai.Publisher
	Radius           radius.Client
	Metrics          *metrics.Collector
	RateObserver     ratelimit.Observer
	SnoopingObserver snooping.Observer
	Quarantine       QuarantineSink
	Logger           *zap.Logger
	MACACL           *MACACL
	MacList          maclist.Evaluator
	Audit            *audit.Service
	Policy           policy.Evaluator
}

// QuarantineSink receives security verdicts to update downstream systems (e.g. lease service).
type QuarantineSink interface {
	Apply(ctx context.Context, signal QuarantineSignal)
}

// QuarantineSignal conveys the metadata required to mark a client as suspect or blocked.
type QuarantineSignal struct {
	TenantID string
	MAC      string
	ClientID string
	State    detector.SecurityState
	Reason   string
}

type guardImpl struct {
	deps             Dependencies
	macACL           *macACL
	policy           policy.Evaluator
	bindingSvc       *pool.Service
	bindingLookup    bindingLookup
	leaseSvc         leaseValidityChecker
	bindingExemptACL bool
}

type leaseValidityChecker interface {
	HasValidLease(ctx context.Context, mac string) (bool, error)
}

type bindingLookup interface {
	FindBinding(ctx context.Context, scope pool.ResourceScope, identifier, ip string) (*models.StaticBinding, error)
}

// New builds a guard that wires available dependencies.
func New(deps Dependencies) Guard {
	if deps.Logger == nil {
		deps.Logger = zap.NewNop()
	}
	acl := normalizeMACACL(deps.MACACL)
	if acl != nil {
		deps.Logger.Info("未命中名单默认动作已固定为 allow，EnforceWhitelist={当前值}",
			zap.Bool("enforceWhitelist", acl.enforceWhitelist),
			zap.String("defaultAction", acl.defaultAction),
		)
	}
	exempt := false
	if acl != nil {
		exempt = acl.bindingExemptACL
	}
	return &guardImpl{deps: deps, macACL: acl, policy: deps.Policy, bindingSvc: deps.BindingService, bindingLookup: deps.BindingService, leaseSvc: deps.LeaseService, bindingExemptACL: exempt}
}

// NewNoop returns a guard instance that always allows traffic.
func NewNoop() Guard {
	return &noopGuard{}
}

func (g *guardImpl) Check(ctx context.Context, ctxData *Context) error {
	if ctxData == nil {
		g.observeLatency("error", 0)
		return errNilContext
	}
	result := "allow"
	start := time.Now()
	mac := normalizeMAC(ctxData.MAC)
	defer func() {
		g.observeLatency(result, time.Since(start))
	}()
	if g.bindingLookup != nil && mac != "" {
		scopeRef := pool.NewResourceScope(ctxData.TenantID, ctxData.TenantID)
		binding, err := g.bindingLookup.FindBinding(ctx, scopeRef, mac, "")
		if err != nil {
			if !errors.Is(err, pool.ErrNotFound) {
				g.deps.Logger.Warn("binding lookup failed before mac acl", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.Error(err))
			}
		} else if binding != nil && g.bindingExemptACL {
			phase := normalizeACLRequestPhase(ctxData.RequestPhase)
			traceID := traceIDFromContext(ctx)
			observeMACACLCheck("allow", "none", phase)
			g.deps.Logger.Info("绑定豁免放行", zap.String("mac", ctxData.MAC), zap.String("bindingIP", binding.IPAddress), zap.String("action", "allow"), zap.String("list_type", "none"), zap.String("request_phase", phase), zap.String("trace_id", traceID))
			return nil
		}
	}
	if err := g.enforceMACACL(ctx, ctxData, mac); err != nil {
		result = "deny"
		return err
	}
	if err := g.enforcePolicy(ctx, ctxData); err != nil {
		result = "deny"
		return err
	}
	if g.deps.Snooping != nil {
		lookup := snooping.LookupKey{
			TenantID: ctxData.TenantID,
			MAC:      mac,
			PortID:   ctxData.PortID,
			VLANID:   ctxData.VLANID,
		}
		if _, err := g.deps.Snooping.Lookup(ctx, lookup); err != nil {
			g.observeSnooping(ctxData, mac, err)
			if errors.Is(err, snooping.ErrBindingNotFound) || errors.Is(err, snooping.ErrUntrusted) {
				g.deps.Logger.Info("snooping rejection", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.Error(err))
				g.recordEvent(EventTypeSnooping, "deny")
				g.emitSecurityEvent(ctx, ctxData, EventTypeSnooping, "deny", err.Error(), "block", map[string]any{"portId": ctxData.PortID, "vlanId": ctxData.VLANID})
				result = "deny"
				return ErrUntrustedBinding
			}
			g.deps.Logger.Debug("snooping lookup failed", zap.Error(err))
			g.recordEvent(EventTypeSnooping, "error")
			g.emitSecurityEvent(ctx, ctxData, EventTypeSnooping, "error", err.Error(), "observe", map[string]any{"portId": ctxData.PortID})
		} else {
			g.observeSnooping(ctxData, mac, nil)
		}
	}

	if g.deps.Limiter != nil {
		key := ratelimit.Key{
			TenantID:  ctxData.TenantID,
			MAC:       mac,
			PortID:    ctxData.PortID,
			IPAddress: pickFirstNonEmpty(ctxData.RequestedIP, ctxData.GIAddr),
		}
		decision, err := g.deps.Limiter.Allow(ctx, key)
		if err != nil {
			result = "error"
			g.recordEvent(EventTypeRate, "error")
			g.emitSecurityEvent(ctx, ctxData, EventTypeRate, "error", err.Error(), "observe", map[string]any{"mac": ctxData.MAC})
			return err
		}
		if !decision.Allowed {
			g.observeRateLimitHit(ctxData, mac, decision.RetryAfter)
			g.deps.Logger.Info("rate limit rejection", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.Duration("retryAfter", decision.RetryAfter))
			g.recordEvent(EventTypeRate, "deny")
			details := map[string]any{"retryAfter": decision.RetryAfter.String()}
			g.emitSecurityEvent(ctx, ctxData, EventTypeRate, "deny", "rate limit exceeded", "block", details)
			result = "deny"
			return ErrRateLimited
		}
	}

	if g.deps.Detector != nil {
		verdict := g.deps.Detector.Observe(ctx, detector.Event{
			TenantID:    ctxData.TenantID,
			MAC:         mac,
			MessageType: ctxData.MessageType,
			CircuitID:   ctxData.CircuitID,
			RemoteID:    ctxData.RemoteID,
			PortID:      ctxData.PortID,
			GIAddr:      ctxData.GIAddr,
			Timestamp:   ctxData.Timestamp,
		})
		g.applyQuarantine(ctx, ctxData, verdict)
		if verdict.Block {
			g.deps.Logger.Info("detector rejection", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("reason", verdict.Reason))
			g.recordEvent(EventTypeDetector, "deny")
			g.emitSecurityEvent(ctx, ctxData, EventTypeDetector, "deny", verdict.Reason, "block", map[string]any{"state": verdict.State})
			result = "deny"
			return ErrSuspiciousTraffic
		}
	}
	return nil
}

// CheckRenewACL applies simplified ACL checks for renew/rebind phases.
func (g *guardImpl) CheckRenewACL(ctx context.Context, mac string) error {
	mac = normalizeMAC(mac)
	if g == nil || mac == "" {
		return nil
	}
	tenantID := lease.TenantIDFromContext(ctx)
	requestPhase := requestPhaseFromContext(ctx)
	traceID := traceIDFromContext(ctx)
	g.deps.Logger.Info("续约校验", zap.String("tenantId", tenantID), zap.String("mac", mac), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID))

	blacklisted := false
	if g.deps.MacList != nil {
		res, err := g.deps.MacList.Evaluate(ctx, tenantID, mac)
		if err != nil {
			g.deps.Logger.Warn("renew mac list evaluation failed", zap.String("tenantId", tenantID), zap.String("mac", mac), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Error(err))
		} else if res.Entry != nil && res.Entry.Type == maclist.ListTypeBlacklist {
			blacklisted = true
		}
	}
	if !blacklisted && g.macACL != nil && g.macACL.blacklist != nil {
		_, blacklisted = g.macACL.blacklist[mac]
	}
	if !blacklisted {
		observeMACACLCheck("allow", "none", requestPhase)
		g.deps.Logger.Info("续约校验通过（非黑名单）", zap.String("mac", mac), zap.String("action", "allow"), zap.String("list_type", "none"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Bool("renewExemptTriggered", false))
		return nil
	}

	if g.macACL != nil && g.macACL.renewExemptBlocked && g.leaseSvc != nil {
		hasLease, err := g.leaseSvc.HasValidLease(ctx, mac)
		if err != nil {
			g.deps.Logger.Warn("续约豁免检查失败", zap.String("mac", mac), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Error(err))
		} else if hasLease {
			observeMACACLCheck("allow", "black", requestPhase)
			g.deps.Logger.Info("续约校验触发豁免放行", zap.String("mac", mac), zap.String("action", "allow"), zap.String("list_type", "black"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Bool("renewExemptTriggered", true))
			return nil
		}
	}

	observeMACACLCheck("block", "black", requestPhase)
	g.deps.Logger.Info("续约校验拒绝（黑名单）", zap.String("mac", mac), zap.String("action", "block"), zap.String("list_type", "black"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Bool("renewExemptTriggered", false))
	return ErrMACNotAllowed
}

func (g *guardImpl) applyQuarantine(ctx context.Context, ctxData *Context, verdict detector.Verdict) {
	if g.deps.Quarantine == nil {
		return
	}
	if verdict.State == "" || verdict.State == detector.SecurityStateOK {
		return
	}
	signal := QuarantineSignal{
		TenantID: ctxData.TenantID,
		MAC:      strings.ToLower(ctxData.MAC),
		ClientID: ctxData.ClientID,
		State:    verdict.State,
		Reason:   verdict.Reason,
	}
	g.deps.Quarantine.Apply(ctx, signal)
	g.emitSecurityEvent(ctx, ctxData, EventTypeDetector, string(verdict.State), verdict.Reason, "quarantine", map[string]any{"state": verdict.State})
}

func (g *guardImpl) enforcePolicy(ctx context.Context, ctxData *Context) error {
	if g == nil || g.policy == nil || ctxData == nil {
		return nil
	}
	decision := g.policy.Evaluate(ctx, ctxData.TenantID, policy.EvaluationContext{
		MAC:         ctxData.MAC,
		IP:          pickFirstNonEmpty(ctxData.RequestedIP, ctxData.GIAddr),
		VLANID:      ctxData.VLANID,
		InterfaceID: ctxData.InterfaceID,
		PortID:      ctxData.PortID,
	})
	if !decision.Matched {
		return nil
	}
	details := map[string]any{
		"ruleId":   decision.RuleID,
		"ruleName": decision.Name,
		"effect":   decision.Effect,
	}
	reason := policyDecisionReason(decision)
	switch decision.Effect {
	case policy.EffectAllow:
		g.recordEvent(EventTypeACL, "allow")
		g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "allow", reason, "allow", details)
		return nil
	case policy.EffectMonitor:
		g.recordEvent(EventTypeACL, "monitor")
		g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "monitor", reason, "observe", details)
		return nil
	case policy.EffectQuarantine:
		g.recordEvent(EventTypeACL, "quarantine")
		g.applyPolicyQuarantine(ctx, ctxData, reason)
		g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", reason, "quarantine", details)
		return ErrPolicyDenied
	default:
		g.recordEvent(EventTypeACL, "deny")
		g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", reason, "block", details)
		return ErrPolicyDenied
	}
}

func (g *guardImpl) applyPolicyQuarantine(ctx context.Context, ctxData *Context, reason string) {
	if g == nil || g.deps.Quarantine == nil || ctxData == nil {
		return
	}
	signal := QuarantineSignal{
		TenantID: ctxData.TenantID,
		MAC:      strings.ToLower(ctxData.MAC),
		ClientID: ctxData.ClientID,
		State:    detector.SecurityStateBlocked,
		Reason:   reason,
	}
	g.deps.Quarantine.Apply(ctx, signal)
}

func policyDecisionReason(decision policy.Decision) string {
	name := strings.TrimSpace(decision.Name)
	if name == "" {
		return fmt.Sprintf("policy rule %s", decision.RuleID)
	}
	return fmt.Sprintf("policy rule %s (%s)", decision.RuleID, name)
}

func (g *guardImpl) OnLeaseChange(ctx context.Context, change *LeaseChange) {
	if change == nil {
		return
	}
	if g.deps.Publisher != nil {
		snap := ipsgdai.LeaseSnapshot{
			TenantID:    change.TenantID,
			MACAddress:  change.MACAddress,
			IPAddress:   change.IPAddress,
			PoolID:      change.PoolID,
			VLANID:      change.VLANID,
			InterfaceID: change.InterfaceID,
			PortID:      change.PortID,
			ExpiresAt:   change.ExpiresAt,
			Action:      change.Action,
			MDMManaged:  change.MDMManaged,
			MDMSource:   change.MDMSource,
			MDMTags:     change.MDMTags,
			MDMObserved: change.MDMObserved,
		}
		if err := g.deps.Publisher.Publish(ctx, snap); err != nil {
			g.deps.Logger.Debug("ipsg publish failed", zap.Error(err))
			g.recordEvent(EventTypeIPSG, "error")
		}
	}
	if g.deps.Radius != nil {
		report := radius.AccountingReport{
			TenantID:   change.TenantID,
			MACAddress: change.MACAddress,
			ClientID:   change.ClientID,
			IPAddress:  change.IPAddress,
			Action:     change.Action,
			ExpiresAt:  change.ExpiresAt,
		}
		if err := g.deps.Radius.Accounting(ctx, report); err != nil {
			g.deps.Logger.Debug("radius accounting failed", zap.Error(err))
			g.recordEvent(EventTypeRADIUS, "error")
		}
	}
}

func (g *guardImpl) OnSecurityEvent(ctx context.Context, evt Event) {
	if g.deps.Logger != nil {
		g.deps.Logger.Info("security event", zap.String("tenantId", evt.TenantID), zap.String("type", string(evt.Type)), zap.String("reason", evt.Reason), zap.Any("details", evt.Details))
	}
	g.recordEvent(evt.Type, "external")
	payload := auditpayload.SecurityEvent{
		TenantID:   evt.TenantID,
		SignalType: string(evt.Type),
		Verdict:    "external",
		Reason:     evt.Reason,
		Action:     "alert",
		Details:    evt.Details,
	}
	payload.Scope = auditpayload.ScopeFromTenant(evt.TenantID)
	g.recordSecurityAudit(ctx, payload, "security.alert")
}

type noopGuard struct{}

func (n *noopGuard) Check(ctx context.Context, ctxData *Context) error {
	return nil
}

func (n *noopGuard) CheckRenewACL(ctx context.Context, mac string) error {
	return nil
}

func (n *noopGuard) OnLeaseChange(ctx context.Context, change *LeaseChange) {}

func (n *noopGuard) OnSecurityEvent(ctx context.Context, evt Event) {}

func (g *guardImpl) recordEvent(eventType EventType, result string) {
	if g == nil || g.deps.Metrics == nil || g.deps.Metrics.SecurityEvents == nil {
		return
	}
	g.deps.Metrics.SecurityEvents.WithLabelValues(string(eventType), result).Inc()
}

func (g *guardImpl) emitSecurityEvent(ctx context.Context, ctxData *Context, eventType EventType, verdict, reason, action string, details map[string]any) {
	if g == nil || ctxData == nil {
		return
	}
	payload := auditpayload.SecurityEvent{
		TenantID:    ctxData.TenantID,
		SignalType:  string(eventType),
		Verdict:     verdict,
		MAC:         ctxData.MAC,
		ClientID:    ctxData.ClientID,
		PortID:      ctxData.PortID,
		InterfaceID: ctxData.InterfaceID,
		VLANID:      ctxData.VLANID,
		Reason:      reason,
		Action:      action,
		Details:     details,
	}
	payload.Scope = auditpayload.ScopeFromTenant(ctxData.TenantID)
	auditAction := "security.alert"
	switch strings.ToLower(verdict) {
	case "deny":
		auditAction = "security.violation"
	case "error":
		auditAction = "security.error"
	}
	if strings.EqualFold(action, "block") && auditAction != "security.violation" {
		auditAction = "security.violation"
	}
	g.recordSecurityAudit(ctx, payload, auditAction)
}

func (g *guardImpl) recordSecurityAudit(ctx context.Context, payload auditpayload.SecurityEvent, action string) {
	if g == nil || g.deps.Audit == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		if g.deps.Logger != nil {
			g.deps.Logger.Warn("security audit marshal failed", zap.Error(err))
		}
		return
	}
	tenantID := payload.TenantID
	if strings.TrimSpace(tenantID) == "" {
		tenantID = "system"
	}
	req := audit.RecordEventRequest{
		TenantID: tenantID,
		Actor:    "security-guard",
		Action:   action,
		Source:   "guard",
		Resource: "security",
		Payload:  data,
	}
	if err := g.deps.Audit.RecordEvent(ctx, req); err != nil && g.deps.Logger != nil {
		g.deps.Logger.Warn("security audit persist failed", zap.Error(err))
	}
}

func (g *guardImpl) observeLatency(result string, duration time.Duration) {
	if g == nil || g.deps.Metrics == nil || g.deps.Metrics.SecurityGuardLatency == nil {
		return
	}
	g.deps.Metrics.SecurityGuardLatency.WithLabelValues(result).Observe(duration.Seconds())
}

func (g *guardImpl) enforceMACACL(ctx context.Context, ctxData *Context, mac string) error {
	if g == nil || mac == "" {
		return nil
	}
	requestPhase := normalizeACLRequestPhase(ctxData.RequestPhase)
	traceID := traceIDFromContext(ctx)
	if g.deps.MacList != nil {
		res, err := g.deps.MacList.Evaluate(ctx, ctxData.TenantID, mac)
		if err != nil {
			g.deps.Logger.Warn("mac list evaluation failed", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.Error(err))
		} else if res.Entry != nil {
			details := map[string]any{"entryId": res.Entry.ID, "mac": ctxData.MAC, "listType": res.Entry.Type, "source": res.Entry.Source, "priority": res.Entry.Priority}
			listType := normalizeACLListType(string(res.Entry.Type))
			switch res.Entry.Action {
			case maclist.ActionBlock:
				observeMACACLCheck("block", listType, requestPhase)
				g.deps.Logger.Info("mac list block", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", "block"), zap.String("list_type", listType), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.String("source", res.Entry.Source))
				g.recordEvent(EventTypeACL, "deny")
				g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "mac list block", "block", details)
				return ErrMACNotAllowed
			case maclist.ActionAllow:
				observeMACACLCheck("allow", listType, requestPhase)
				g.deps.Logger.Info("mac list allow", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", "allow"), zap.String("list_type", listType), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.String("source", res.Entry.Source))
				g.recordEvent(EventTypeACL, "allow")
				g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "allow", "mac list allow", "allow", details)
				return nil
			default:
				observeMACACLCheck("monitor", listType, requestPhase)
				g.deps.Logger.Info("mac list monitor", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", "monitor"), zap.String("list_type", listType), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.String("source", res.Entry.Source))
				g.recordEvent(EventTypeACL, "gray")
				g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "gray", "mac list monitor", "monitor", details)
				return nil
			}
		}
	}
	if g.macACL == nil {
		observeMACACLCheck("allow", "none", requestPhase)
		g.deps.Logger.Info("MAC未命中任何黑白灰名单，允许获取地址池IP",
			zap.String("mac", ctxData.MAC),
			zap.Bool("enforceWhitelist", false),
			zap.String("action", macACLDefaultActionAllow),
			zap.String("list_type", "none"),
			zap.String("request_phase", requestPhase),
			zap.String("trace_id", traceID),
		)
		return nil
	}
	if g.macACL.blacklist != nil {
		if _, ok := g.macACL.blacklist[mac]; ok {
			observeMACACLCheck("block", "black", requestPhase)
			g.deps.Logger.Info("mac blacklist rejection", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", "block"), zap.String("list_type", "black"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID), zap.String("portId", ctxData.PortID))
			g.recordEvent(EventTypeACL, "deny")
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "mac blacklist", "block", map[string]any{"mac": ctxData.MAC})
			return ErrMACNotAllowed
		}
	}
	if g.macACL.whitelist != nil {
		if _, ok := g.macACL.whitelist[mac]; ok {
			observeMACACLCheck("allow", "white", requestPhase)
			g.deps.Logger.Info("mac whitelist allow", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", "allow"), zap.String("list_type", "white"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID))
			g.recordEvent(EventTypeACL, "allow")
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "allow", "mac whitelist hit", "allow", map[string]any{"mac": ctxData.MAC})
			return nil
		}
	}
	if g.macACL.graylist != nil {
		if _, ok := g.macACL.graylist[mac]; ok {
			observeMACACLCheck(string(g.macACL.grayAction), "gray", requestPhase)
			g.deps.Logger.Info("mac graylist hit", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", string(g.macACL.grayAction)), zap.String("list_type", "gray"), zap.String("request_phase", requestPhase), zap.String("trace_id", traceID))
			g.recordEvent(EventTypeACL, "gray")
			verdict := string(g.macACL.grayAction)
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, verdict, "graylist", string(g.macACL.grayAction), nil)
			if g.macACL.grayAction == GraylistActionBlock {
				return ErrMACNotAllowed
			}
			return nil
		}
	}
	if g.macACL.enforceWhitelist {
		observeMACACLCheck("block", "none", requestPhase)
		g.deps.Logger.Info("白名单强制模式开启，未命中白名单拒绝分配IP",
			zap.String("mac", ctxData.MAC),
			zap.Bool("enforceWhitelist", g.macACL.enforceWhitelist),
			zap.String("action", "block"),
			zap.String("list_type", "none"),
			zap.String("request_phase", requestPhase),
			zap.String("trace_id", traceID),
		)
		g.recordEvent(EventTypeACL, "deny")
		g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "whitelist miss", "block", nil)
		return ErrMACNotAllowed
	}
	observeMACACLCheck("allow", "none", requestPhase)
	g.deps.Logger.Info("MAC未命中任何黑白灰名单，允许获取地址池IP",
		zap.String("mac", ctxData.MAC),
		zap.Bool("enforceWhitelist", g.macACL.enforceWhitelist),
		zap.String("action", g.macACL.defaultAction),
		zap.String("list_type", "none"),
		zap.String("request_phase", requestPhase),
		zap.String("trace_id", traceID),
	)
	return nil
}

func pickFirstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func (g *guardImpl) observeRateLimitHit(ctxData *Context, mac string, retry time.Duration) {
	if g == nil || g.deps.RateObserver == nil || ctxData == nil {
		return
	}
	hit := ratelimit.Hit{
		TenantID:   ctxData.TenantID,
		MAC:        mac,
		PortID:     ctxData.PortID,
		IPAddress:  pickFirstNonEmpty(ctxData.RequestedIP, ctxData.GIAddr),
		RetryAfter: retry,
		OccurredAt: ctxData.Timestamp,
	}
	if hit.OccurredAt.IsZero() {
		hit.OccurredAt = time.Now().UTC()
	}
	g.deps.RateObserver.Record(hit)
}

func (g *guardImpl) observeSnooping(ctxData *Context, mac string, err error) {
	if g == nil || g.deps.SnoopingObserver == nil || ctxData == nil {
		return
	}
	obs := snooping.Observation{
		TenantID:   ctxData.TenantID,
		MAC:        mac,
		PortID:     ctxData.PortID,
		VLANID:     ctxData.VLANID,
		OccurredAt: ctxData.Timestamp,
	}
	if obs.OccurredAt.IsZero() {
		obs.OccurredAt = time.Now().UTC()
	}
	switch {
	case err == nil:
		obs.Result = snooping.ObservationResultTrusted
	case errors.Is(err, snooping.ErrUntrusted):
		obs.Result = snooping.ObservationResultUntrusted
		obs.Reason = "untrusted"
	case errors.Is(err, snooping.ErrBindingNotFound):
		obs.Result = snooping.ObservationResultMiss
		obs.Reason = "missing"
	default:
		obs.Result = snooping.ObservationResultError
		if err != nil {
			obs.Reason = err.Error()
		}
	}
	g.deps.SnoopingObserver.Record(obs)
}

func normalizeMAC(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

type macACL struct {
	whitelist          map[string]struct{}
	blacklist          map[string]struct{}
	graylist           map[string]struct{}
	grayAction         GraylistAction
	enforceWhitelist   bool
	bindingExemptACL   bool
	renewExemptBlocked bool
	defaultAction      string
}

func normalizeMACACL(cfg *MACACL) *macACL {
	if cfg == nil {
		return nil
	}
	acl := &macACL{
		whitelist:          normalizeMACEntries(cfg.Whitelist),
		blacklist:          normalizeMACEntries(cfg.Blacklist),
		graylist:           normalizeMACEntries(cfg.Graylist),
		grayAction:         cfg.GraylistAction,
		enforceWhitelist:   cfg.EnforceWhitelist,
		bindingExemptACL:   cfg.BindingExemptACL,
		renewExemptBlocked: cfg.RenewExemptBlocked,
		defaultAction:      strings.ToLower(strings.TrimSpace(cfg.DefaultAction)),
	}
	if acl.grayAction == "" {
		acl.grayAction = GraylistActionMonitor
	}
	if acl.grayAction != GraylistActionMonitor && acl.grayAction != GraylistActionBlock {
		acl.grayAction = GraylistActionMonitor
	}
	if acl.defaultAction != macACLDefaultActionAllow {
		acl.defaultAction = macACLDefaultActionAllow
	}
	if !acl.hasRules() {
		return nil
	}
	return acl
}

func normalizeMACEntries(entries []string) map[string]struct{} {
	if len(entries) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if norm := normalizeMAC(entry); norm != "" {
			m[norm] = struct{}{}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func (m *macACL) hasRules() bool {
	return m.bindingExemptACL || m.enforceWhitelist || len(m.whitelist) > 0 || len(m.blacklist) > 0 || len(m.graylist) > 0 || m.defaultAction == macACLDefaultActionAllow
}
