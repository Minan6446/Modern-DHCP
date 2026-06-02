package guard

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/pool"
	securitypolicy "modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
	"modern-dhcp/pkg/models"
)

func TestGuardMACACLBlacklist(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist: map[string]struct{}{"00:11:22:33:44:55": {}},
		},
	}
	ctxData := &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"}
	if err := g.enforceMACACL(context.Background(), ctxData, "00:11:22:33:44:55"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected mac blacklist rejection, got %v", err)
	}
}

func TestGuardMACACLWhitelist(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			whitelist:        map[string]struct{}{"00:aa:bb:cc:dd:ee": {}},
			enforceWhitelist: true,
		},
	}
	ctxData := &Context{TenantID: "tenant-b", MAC: "00:ff:ee:dd:cc:bb"}
	if err := g.enforceMACACL(context.Background(), ctxData, "00:ff:ee:dd:cc:bb"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected whitelist miss rejection, got %v", err)
	}
}

func TestGuardMACACLUnmatchedAllowWhenWhitelistNotEnforced(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			whitelist:        map[string]struct{}{"00:aa:bb:cc:dd:ee": {}},
			enforceWhitelist: false,
			defaultAction:    macACLDefaultActionAllow,
		},
	}
	ctxData := &Context{TenantID: "tenant-b", MAC: "00:ff:ee:dd:cc:bb"}
	if err := g.enforceMACACL(context.Background(), ctxData, "00:ff:ee:dd:cc:bb"); err != nil {
		t.Fatalf("expected unmatched allow when enforceWhitelist=false, got %v", err)
	}
}

func TestGuardMACACLUnmatchedDenyWhenWhitelistEnforced(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			enforceWhitelist: true,
			defaultAction:    macACLDefaultActionAllow,
		},
	}
	ctxData := &Context{TenantID: "tenant-b", MAC: "00:ff:ee:dd:cc:bb"}
	if err := g.enforceMACACL(context.Background(), ctxData, "00:ff:ee:dd:cc:bb"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected unmatched deny when enforceWhitelist=true, got %v", err)
	}
}

func TestGuardMACACLGraylistBlock(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			graylist:   map[string]struct{}{"00:99:88:77:66:55": {}},
			grayAction: GraylistActionBlock,
		},
	}
	ctxData := &Context{TenantID: "tenant-c", MAC: "00:99:88:77:66:55"}
	if err := g.enforceMACACL(context.Background(), ctxData, "00:99:88:77:66:55"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected graylist block, got %v", err)
	}
}

func TestNormalizeMACACLForcesDefaultActionAllow(t *testing.T) {
	acl := normalizeMACACL(&MACACL{
		Whitelist:        []string{"00:aa:bb:cc:dd:ee"},
		EnforceWhitelist: false,
		DefaultAction:    "block",
	})
	if acl == nil {
		t.Fatalf("expected acl to be initialized")
	}
	if acl.defaultAction != macACLDefaultActionAllow {
		t.Fatalf("expected default action to be forced to allow, got %s", acl.defaultAction)
	}
	g := &guardImpl{deps: Dependencies{Logger: zap.NewNop()}, macACL: acl}
	if err := g.enforceMACACL(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"}, "00:11:22:33:44:55"); err != nil {
		t.Fatalf("expected allow after forced default action, got %v", err)
	}
}

func TestGuardMACACLWhitelistTakesPrecedenceOverGraylist(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			whitelist:        map[string]struct{}{"00:11:22:33:44:55": {}},
			graylist:         map[string]struct{}{"00:11:22:33:44:55": {}},
			grayAction:       GraylistActionBlock,
			enforceWhitelist: false,
			defaultAction:    macACLDefaultActionAllow,
		},
	}
	err := g.enforceMACACL(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"}, "00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("expected whitelist hit to allow even when graylist also matches, got %v", err)
	}
}

func TestGuardMACACLBlacklistTakesPrecedenceOverWhitelist(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			whitelist:        map[string]struct{}{"00:11:22:33:44:55": {}},
			blacklist:        map[string]struct{}{"00:11:22:33:44:55": {}},
			enforceWhitelist: false,
			defaultAction:    macACLDefaultActionAllow,
		},
	}
	err := g.enforceMACACL(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"}, "00:11:22:33:44:55")
	if !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected blacklist to deny even when whitelist also matches, got %v", err)
	}
}

func TestGuardCheckBindingExemptACLAllowsBoundMAC(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist:        map[string]struct{}{"00:11:22:33:44:55": {}},
			enforceWhitelist: true,
			defaultAction:    macACLDefaultActionAllow,
		},
		bindingLookup:    &stubBindingLookup{binding: &models.StaticBinding{IPAddress: "10.0.0.55"}},
		bindingExemptACL: true,
	}
	err := g.Check(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"})
	if err != nil {
		t.Fatalf("expected bound mac to bypass ACL when binding exemption enabled, got %v", err)
	}
}

func TestGuardCheckBindingExemptACLDisabledStillRunsACL(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist:        map[string]struct{}{"00:11:22:33:44:55": {}},
			enforceWhitelist: false,
			defaultAction:    macACLDefaultActionAllow,
		},
		bindingLookup:    &stubBindingLookup{binding: &models.StaticBinding{IPAddress: "10.0.0.55"}},
		bindingExemptACL: false,
	}
	err := g.Check(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"})
	if !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected ACL denial when binding exemption disabled, got %v", err)
	}
}

func TestCheckRenewACLAllowsNonBlacklistedMAC(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist:          map[string]struct{}{"00:aa:bb:cc:dd:ee": {}},
			renewExemptBlocked: true,
		},
	}
	ctx := lease.WithTenantContext(context.Background(), "tenant-a")
	if err := g.CheckRenewACL(ctx, "00:11:22:33:44:55"); err != nil {
		t.Fatalf("expected non-blacklisted mac to pass renew acl, got %v", err)
	}
}

func TestCheckRenewACLDeniesBlacklistedWithoutExempt(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist:          map[string]struct{}{"00:11:22:33:44:55": {}},
			renewExemptBlocked: false,
		},
	}
	ctx := lease.WithTenantContext(context.Background(), "tenant-a")
	if err := g.CheckRenewACL(ctx, "00:11:22:33:44:55"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected blacklisted mac to be denied without exemption, got %v", err)
	}
}

func TestCheckRenewACLExemptsBlacklistedWithValidLease(t *testing.T) {
	g := &guardImpl{
		deps: Dependencies{Logger: zap.NewNop()},
		macACL: &macACL{
			blacklist:          map[string]struct{}{"00:11:22:33:44:55": {}},
			renewExemptBlocked: true,
		},
		leaseSvc: &stubLeaseChecker{ok: true},
	}
	ctx := lease.WithTenantContext(context.Background(), "tenant-a")
	if err := g.CheckRenewACL(ctx, "00:11:22:33:44:55"); err != nil {
		t.Fatalf("expected renew exemption to allow blacklisted mac with valid lease, got %v", err)
	}
}

func TestCheckRenewACLLogsExemptTriggeredTrue(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	g := &guardImpl{
		deps: Dependencies{Logger: logger},
		macACL: &macACL{
			blacklist:          map[string]struct{}{"00:11:22:33:44:55": {}},
			renewExemptBlocked: true,
		},
		leaseSvc: &stubLeaseChecker{ok: true},
	}
	ctx := lease.WithTenantContext(context.Background(), "tenant-a")
	if err := g.CheckRenewACL(ctx, "00:11:22:33:44:55"); err != nil {
		t.Fatalf("expected exemption allow, got %v", err)
	}
	entries := observed.FilterMessage("续约校验触发豁免放行").All()
	if len(entries) != 1 {
		t.Fatalf("expected one exemption log entry, got %d", len(entries))
	}
	if triggered, ok := entries[0].ContextMap()["renewExemptTriggered"].(bool); !ok || !triggered {
		t.Fatalf("expected renewExemptTriggered=true in log, got %v", entries[0].ContextMap()["renewExemptTriggered"])
	}
}

func TestCheckRenewACLLogsExemptTriggeredFalseOnDeny(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	g := &guardImpl{
		deps: Dependencies{Logger: logger},
		macACL: &macACL{
			blacklist:          map[string]struct{}{"00:11:22:33:44:55": {}},
			renewExemptBlocked: false,
		},
	}
	ctx := lease.WithTenantContext(context.Background(), "tenant-a")
	if err := g.CheckRenewACL(ctx, "00:11:22:33:44:55"); !errors.Is(err, ErrMACNotAllowed) {
		t.Fatalf("expected blacklist deny, got %v", err)
	}
	entries := observed.FilterMessage("续约校验拒绝（黑名单）").All()
	if len(entries) != 1 {
		t.Fatalf("expected one deny log entry, got %d", len(entries))
	}
	if triggered, ok := entries[0].ContextMap()["renewExemptTriggered"].(bool); !ok || triggered {
		t.Fatalf("expected renewExemptTriggered=false in log, got %v", entries[0].ContextMap()["renewExemptTriggered"])
	}
}

func TestGuardEnforcePolicyBlock(t *testing.T) {
	eval := &stubPolicyEvaluator{decision: securitypolicy.Decision{Matched: true, RuleID: "rule-block", Name: "deny", Effect: securitypolicy.EffectBlock}}
	g := &guardImpl{
		deps:   Dependencies{Logger: zap.NewNop()},
		policy: eval,
	}
	err := g.enforcePolicy(context.Background(), &Context{TenantID: "tenant-a", MAC: "00:11:22:33:44:55"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected ErrPolicyDenied, got %v", err)
	}
	if eval.lastTenant != "tenant-a" {
		t.Fatalf("expected tenant-a evaluation, got %s", eval.lastTenant)
	}
}

func TestGuardEnforcePolicyQuarantine(t *testing.T) {
	sink := &stubQuarantineSink{}
	eval := &stubPolicyEvaluator{decision: securitypolicy.Decision{Matched: true, RuleID: "rule-quarantine", Name: "quarantine", Effect: securitypolicy.EffectQuarantine}}
	g := &guardImpl{
		deps:   Dependencies{Logger: zap.NewNop(), Quarantine: sink},
		policy: eval,
	}
	err := g.enforcePolicy(context.Background(), &Context{TenantID: "tenant-b", MAC: "aa:bb:cc:dd:ee:ff", ClientID: "client-7"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("expected ErrPolicyDenied, got %v", err)
	}
	if len(sink.signals) != 1 {
		t.Fatalf("expected 1 quarantine signal, got %d", len(sink.signals))
	}
	if sink.signals[0].Reason == "" {
		t.Fatalf("expected quarantine reason to be populated")
	}
}

func TestGuardEnforcePolicyAllow(t *testing.T) {
	eval := &stubPolicyEvaluator{decision: securitypolicy.Decision{Matched: true, RuleID: "rule-allow", Effect: securitypolicy.EffectAllow}}
	g := &guardImpl{
		deps:   Dependencies{Logger: zap.NewNop()},
		policy: eval,
	}
	if err := g.enforcePolicy(context.Background(), &Context{TenantID: "tenant-c", MAC: "00:aa:bb:cc:dd:ee"}); err != nil {
		t.Fatalf("expected allow decision, got %v", err)
	}
}

func TestGuardRateLimitObserver(t *testing.T) {
	observer := &stubRateObserver{}
	limiter := &stubLimiter{decision: ratelimit.Decision{Allowed: false, RetryAfter: time.Second}}
	g := &guardImpl{deps: Dependencies{Limiter: limiter, Logger: zap.NewNop(), RateObserver: observer}}
	ctxData := &Context{TenantID: "tenant-z", MAC: "aa:bb", PortID: "gi1/0/9", Timestamp: time.Unix(100, 0)}
	err := g.Check(context.Background(), ctxData)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if len(observer.hits) != 1 {
		t.Fatalf("expected observer to capture hit, got %d", len(observer.hits))
	}
	if observer.hits[0].TenantID != "tenant-z" || observer.hits[0].MAC != "aa:bb" {
		t.Fatalf("unexpected hit metadata: %+v", observer.hits[0])
	}
}

func TestGuardSnoopingObserver(t *testing.T) {
	observer := &stubSnoopingObserver{}
	store := &stubSnoopingStore{err: snooping.ErrUntrusted}
	g := &guardImpl{deps: Dependencies{Snooping: store, Logger: zap.NewNop(), SnoopingObserver: observer}}
	ctxData := &Context{TenantID: "tenant-x", MAC: "11:22", PortID: "gi1/0/7"}
	err := g.Check(context.Background(), ctxData)
	if !errors.Is(err, ErrUntrustedBinding) {
		t.Fatalf("expected ErrUntrustedBinding, got %v", err)
	}
	if len(observer.events) != 1 {
		t.Fatalf("expected observer to capture event")
	}
	if observer.events[0].Result != snooping.ObservationResultUntrusted {
		t.Fatalf("expected untrusted result, got %s", observer.events[0].Result)
	}
}

type stubPolicyEvaluator struct {
	decision   securitypolicy.Decision
	lastTenant string
	lastCtx    securitypolicy.EvaluationContext
}

func (s *stubPolicyEvaluator) Evaluate(ctx context.Context, tenantID string, input securitypolicy.EvaluationContext) securitypolicy.Decision {
	s.lastTenant = tenantID
	s.lastCtx = input
	return s.decision
}

func (s *stubPolicyEvaluator) Invalidate(tenantID string) {}

type stubQuarantineSink struct {
	signals []QuarantineSignal
}

func (s *stubQuarantineSink) Apply(ctx context.Context, signal QuarantineSignal) {
	s.signals = append(s.signals, signal)
}

type stubRateObserver struct {
	hits []ratelimit.Hit
}

func (s *stubRateObserver) Record(hit ratelimit.Hit) {
	s.hits = append(s.hits, hit)
}

type stubLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (s *stubLimiter) Allow(ctx context.Context, key ratelimit.Key) (ratelimit.Decision, error) {
	if s.err != nil {
		return ratelimit.Decision{}, s.err
	}
	return s.decision, nil
}

type stubSnoopingObserver struct {
	events []snooping.Observation
}

func (s *stubSnoopingObserver) Record(obs snooping.Observation) {
	s.events = append(s.events, obs)
}

type stubSnoopingStore struct {
	binding *snooping.Binding
	err     error
}

func (s *stubSnoopingStore) Lookup(ctx context.Context, key snooping.LookupKey) (*snooping.Binding, error) {
	return s.binding, s.err
}

func (s *stubSnoopingStore) StreamChanges(ctx context.Context) (<-chan snooping.Binding, error) {
	ch := make(chan snooping.Binding)
	close(ch)
	return ch, nil
}

type stubBindingLookup struct {
	binding *models.StaticBinding
	err     error
}

func (s *stubBindingLookup) FindBinding(ctx context.Context, scope pool.ResourceScope, identifier, ip string) (*models.StaticBinding, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.binding, nil
}

type stubLeaseChecker struct {
	ok  bool
	err error
}

func (s *stubLeaseChecker) HasValidLease(ctx context.Context, mac string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.ok, nil
}
