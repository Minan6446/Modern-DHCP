package guard

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	securitypolicy "modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
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
