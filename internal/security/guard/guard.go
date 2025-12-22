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
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/security/detector"
	"modern-dhcp/internal/security/ipsgdai"
	"modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/security/radius"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
	"modern-dhcp/pkg/auditpayload"
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
	OnLeaseChange(ctx context.Context, change *LeaseChange)
	OnSecurityEvent(ctx context.Context, evt Event)
}

// Context captures the minimal metadata required for guard evaluation.
type Context struct {
	TenantID    string
	MessageType string
	MAC         string
	ClientID    string
	CircuitID   string
	RemoteID    string
	RelayAgent  map[string]string
	PortID      string
	VLANID      int
	InterfaceID string
	RequestedIP string
	GIAddr      string
	Timestamp   time.Time
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
	Whitelist        []string
	Blacklist        []string
	Graylist         []string
	GraylistAction   GraylistAction
	EnforceWhitelist bool
}

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
	Detector         detector.Detector
	Publisher        ipsgdai.Publisher
	Radius           radius.Client
	Metrics          *metrics.Collector
	RateObserver     ratelimit.Observer
	SnoopingObserver snooping.Observer
	Quarantine       QuarantineSink
	Logger           *zap.Logger
	MACACL           *MACACL
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
	deps   Dependencies
	macACL *macACL
	policy policy.Evaluator
}

// New builds a guard that wires available dependencies.
func New(deps Dependencies) Guard {
	if deps.Logger == nil {
		deps.Logger = zap.NewNop()
	}
	return &guardImpl{deps: deps, macACL: normalizeMACACL(deps.MACACL), policy: deps.Policy}
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
	g.recordSecurityAudit(ctx, payload, "security.alert")
}

type noopGuard struct{}

func (n *noopGuard) Check(ctx context.Context, ctxData *Context) error {
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
	if g == nil || g.macACL == nil || mac == "" {
		return nil
	}
	if g.macACL.blacklist != nil {
		if _, ok := g.macACL.blacklist[mac]; ok {
			g.deps.Logger.Info("mac blacklist rejection", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("portId", ctxData.PortID))
			g.recordEvent(EventTypeACL, "deny")
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "mac blacklist", "block", map[string]any{"mac": ctxData.MAC})
			return ErrMACNotAllowed
		}
	}
	if g.macACL.enforceWhitelist {
		if g.macACL.whitelist == nil {
			g.recordEvent(EventTypeACL, "deny")
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "whitelist required", "block", nil)
			return ErrMACNotAllowed
		}
		if _, ok := g.macACL.whitelist[mac]; !ok {
			g.deps.Logger.Info("mac whitelist miss", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC))
			g.recordEvent(EventTypeACL, "deny")
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, "deny", "whitelist miss", "block", nil)
			return ErrMACNotAllowed
		}
	}
	if g.macACL.graylist != nil {
		if _, ok := g.macACL.graylist[mac]; ok {
			g.deps.Logger.Info("mac graylist hit", zap.String("tenantId", ctxData.TenantID), zap.String("mac", ctxData.MAC), zap.String("action", string(g.macACL.grayAction)))
			g.recordEvent(EventTypeACL, "gray")
			verdict := string(g.macACL.grayAction)
			g.emitSecurityEvent(ctx, ctxData, EventTypeACL, verdict, "graylist", string(g.macACL.grayAction), nil)
			if g.macACL.grayAction == GraylistActionBlock {
				return ErrMACNotAllowed
			}
		}
	}
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
	whitelist        map[string]struct{}
	blacklist        map[string]struct{}
	graylist         map[string]struct{}
	grayAction       GraylistAction
	enforceWhitelist bool
}

func normalizeMACACL(cfg *MACACL) *macACL {
	if cfg == nil {
		return nil
	}
	acl := &macACL{
		whitelist:        normalizeMACEntries(cfg.Whitelist),
		blacklist:        normalizeMACEntries(cfg.Blacklist),
		graylist:         normalizeMACEntries(cfg.Graylist),
		grayAction:       cfg.GraylistAction,
		enforceWhitelist: cfg.EnforceWhitelist,
	}
	if acl.grayAction == "" {
		acl.grayAction = GraylistActionMonitor
	}
	if acl.grayAction != GraylistActionMonitor && acl.grayAction != GraylistActionBlock {
		acl.grayAction = GraylistActionMonitor
	}
	if !acl.enforceWhitelist && len(acl.whitelist) > 0 {
		acl.enforceWhitelist = true
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
	return m.enforceWhitelist || len(m.whitelist) > 0 || len(m.blacklist) > 0 || len(m.graylist) > 0
}
