package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/config"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

func TestAllocateUsesPreferredIPWhenFree(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease), used: []string{"10.0.0.2"}}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolA": {
			ID:             "poolA",
			TenantID:       "tenant1",
			CIDR:           "10.0.0.0/29",
			ReservePercent: 0,
		},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())

	res, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:bb", models.LeaseProfile{}, "poolA", "10.0.0.5")
	if err != nil {
		t.Fatalf("AllocateOrReuse returned error: %v", err)
	}
	if res.Lease.IPAddress != "10.0.0.5" {
		t.Fatalf("expected preferred ip 10.0.0.5, got %s", res.Lease.IPAddress)
	}
	if res.Pool == nil || res.Pool.ID != "poolA" {
		t.Fatalf("expected pool info on result")
	}
}

func TestUpdateSecurityState(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:bb": {
			ID:            "lease-1",
			TenantID:      "tenant1",
			HardwareAddr:  "aa:bb",
			State:         leaseStateActive,
			SecurityState: models.SecurityStateOK,
		},
	}}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{}, zap.NewNop())
	if err := svc.UpdateSecurityState(context.Background(), "tenant1", "aa:bb", models.SecurityStateBlocked); err != nil {
		t.Fatalf("UpdateSecurityState returned error: %v", err)
	}
	if repo.active["aa:bb"].SecurityState != models.SecurityStateBlocked {
		t.Fatalf("expected security state %s, got %s", models.SecurityStateBlocked, repo.active["aa:bb"].SecurityState)
	}
}

func TestUpdateLeaseSecurityStateByID(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:bb": {
			ID:            "lease-1",
			TenantID:      "tenant1",
			HardwareAddr:  "aa:bb",
			State:         leaseStateActive,
			SecurityState: models.SecurityStateOK,
		},
	}}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{}, zap.NewNop())
	lease, previous, changed, err := svc.UpdateLeaseSecurityState(context.Background(), "tenant1", "lease-1", models.SecurityStateBlocked)
	if err != nil {
		t.Fatalf("UpdateLeaseSecurityState returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected change flag")
	}
	if previous != models.SecurityStateOK {
		t.Fatalf("expected previous state OK, got %s", previous)
	}
	if lease.SecurityState != models.SecurityStateBlocked {
		t.Fatalf("expected security state BLOCKED, got %s", lease.SecurityState)
	}
}

func TestUpdateLeaseSecurityStateRejectsInvalid(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:cc": {
			ID:            "lease-2",
			TenantID:      "tenant1",
			HardwareAddr:  "aa:cc",
			State:         leaseStateActive,
			SecurityState: models.SecurityStateOK,
		},
	}}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{}, zap.NewNop())
	if _, _, _, err := svc.UpdateLeaseSecurityState(context.Background(), "tenant1", "lease-2", "unknown"); !errors.Is(err, ErrInvalidSecurityState) {
		t.Fatalf("expected ErrInvalidSecurityState, got %v", err)
	}
}

func TestAllocateFallsBackWhenPreferredUnavailable(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease), used: []string{"10.0.0.2", "10.0.0.3"}}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolA": {
			ID:             "poolA",
			TenantID:       "tenant1",
			CIDR:           "10.0.0.0/29",
			ReservePercent: 0,
		},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())

	res, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:cc", models.LeaseProfile{}, "poolA", "10.0.0.2")
	if err != nil {
		t.Fatalf("AllocateOrReuse returned error: %v", err)
	}
	if res.Lease.IPAddress != "10.0.0.1" {
		t.Fatalf("expected fallback ip 10.0.0.1, got %s", res.Lease.IPAddress)
	}
}

func TestAllocateFailsWhenPoolExhausted(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease), used: []string{"10.0.1.1", "10.0.1.2"}}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolB": {
			ID:             "poolB",
			TenantID:       "tenant1",
			CIDR:           "10.0.1.0/30",
			ReservePercent: 0,
		},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())

	_, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:dd", models.LeaseProfile{}, "poolB", "")
	if !errors.Is(err, ErrNoAvailableIP) {
		t.Fatalf("expected ErrNoAvailableIP, got %v", err)
	}
}

func TestAllocateIPv6FromPrefix(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease)}
	ipv6CIDR := "2001:db8::/124"
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolV6": {
			ID:             "poolV6",
			TenantID:       "tenant1",
			CIDR:           ipv6CIDR,
			ReservePercent: 0,
		},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())

	res, err := svc.AllocateOrReuse(context.Background(), "tenant1", "duid-01", models.LeaseProfile{}, "poolV6", "")
	if err != nil {
		t.Fatalf("AllocateOrReuse returned error: %v", err)
	}
	addr, err := netip.ParseAddr(res.Lease.IPAddress)
	if err != nil {
		t.Fatalf("lease address invalid: %v", err)
	}
	prefix, _ := netip.ParsePrefix(ipv6CIDR)
	if !prefix.Contains(addr) {
		t.Fatalf("expected lease within %s, got %s", ipv6CIDR, addr)
	}
	if res.Pool == nil || res.Pool.ID != "poolV6" {
		t.Fatalf("expected pool metadata on result")
	}
}

func TestAllocateEnforcesMacLimit(t *testing.T) {
	repo := &stubLeaseRepo{
		active:          make(map[string]*models.Lease),
		identifierCount: map[string]int{"aa:bb": 3},
	}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolA": {ID: "poolA", TenantID: "tenant1", CIDR: "10.0.0.0/29", ReservePercent: 0},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop(),
		WithExhaustionConfig(config.ExhaustionConfig{
			MaxLeasesPerMAC:    2,
			Isolation:          config.IsolationConfig{TemporaryDuration: time.Minute},
			PoolWarnPercent:    80,
			PoolProtectPercent: 95,
		}))
	if _, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:bb", models.LeaseProfile{}, "poolA", ""); !errors.Is(err, ErrIdentifierLeaseLimit) {
		t.Fatalf("expected ErrIdentifierLeaseLimit, got %v", err)
	}
	if _, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:bb", models.LeaseProfile{}, "poolA", ""); !errors.Is(err, ErrClientIsolated) {
		t.Fatalf("expected ErrClientIsolated after isolation, got %v", err)
	}
}

func TestAllocateEnforcesUserLimit(t *testing.T) {
	repo := &stubLeaseRepo{
		active:    make(map[string]*models.Lease),
		userCount: map[string]int{"user-1": 5},
	}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolA": {ID: "poolA", TenantID: "tenant1", CIDR: "10.0.0.0/29", ReservePercent: 0},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop(),
		WithExhaustionConfig(config.ExhaustionConfig{MaxLeasesPerUser: 2, Isolation: config.IsolationConfig{TemporaryDuration: time.Minute}}))
	meta := AllocationMetadata{UserID: "user-1"}
	if _, err := svc.AllocateOrReuseWithMetadata(context.Background(), "tenant1", "bb:cc", models.LeaseProfile{}, "poolA", "", meta); !errors.Is(err, ErrUserLeaseLimit) {
		t.Fatalf("expected ErrUserLeaseLimit, got %v", err)
	}
}

func TestPoolProtectionThreshold(t *testing.T) {
	repo := &stubLeaseRepo{
		active: make(map[string]*models.Lease),
		used:   []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"},
	}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{
		"poolA": {ID: "poolA", TenantID: "tenant1", CIDR: "10.0.0.0/29", ReservePercent: 0},
	}}
	svc := NewService(repo, poolReader, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop(),
		WithExhaustionConfig(config.ExhaustionConfig{PoolWarnPercent: 50, PoolProtectPercent: 80}))
	if _, err := svc.AllocateOrReuse(context.Background(), "tenant1", "cc:dd", models.LeaseProfile{}, "poolA", ""); !errors.Is(err, ErrPoolProtection) {
		t.Fatalf("expected ErrPoolProtection, got %v", err)
	}
}

func TestMarkDeclinedUpdatesLease(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:ee": {
			ID:           "lease-1",
			TenantID:     "tenant1",
			HardwareAddr: "aa:ee",
			State:        leaseStateActive,
		},
	}}
	poolReader := &stubPoolReader{pools: map[string]*models.AddressPool{}}
	svc := NewService(repo, poolReader, config.PolicyConfig{
		DefaultLeaseProfile: config.LeaseProfileConfig{NotificationLead: time.Minute},
		Lifecycle:           config.LifecycleConfig{CooldownDuration: time.Minute * 5, MaxCooldowns: 5},
	}, zap.NewNop())

	if err := svc.MarkDeclined(context.Background(), "tenant1", "aa:ee", "conflict"); err != nil {
		t.Fatalf("MarkDeclined returned error: %v", err)
	}
	lease := repo.active["aa:ee"]
	if lease.State != leaseStateCooldown {
		t.Fatalf("expected lease state %s, got %s", leaseStateCooldown, lease.State)
	}
	if lease.CooldownUntil == nil {
		t.Fatalf("expected cooldown to be set")
	}
}

func TestMarkDeclinedQuarantinesAfterMax(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:ff": {
			ID:           "lease-2",
			TenantID:     "tenant1",
			HardwareAddr: "aa:ff",
			State:        leaseStateActive,
		},
	}}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{
		DefaultLeaseProfile: config.LeaseProfileConfig{NotificationLead: time.Minute},
		Lifecycle:           config.LifecycleConfig{CooldownDuration: time.Minute, MaxCooldowns: 1},
	}, zap.NewNop())
	if err := svc.MarkDeclined(context.Background(), "tenant1", "aa:ff", "conflict"); err != nil {
		t.Fatalf("MarkDeclined returned error: %v", err)
	}
	lease := repo.active["aa:ff"]
	if lease.State != leaseStateQuarantined {
		t.Fatalf("expected lease state %s, got %s", leaseStateQuarantined, lease.State)
	}
	if lease.CooldownUntil != nil {
		t.Fatalf("expected cooldown cleared for quarantined lease")
	}
}

func TestMarkDeclinedEmitsAuditEvent(t *testing.T) {
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:gg": {
			ID:           "lease-4",
			TenantID:     "tenant1",
			HardwareAddr: "aa:gg",
			ClientID:     "client-gg",
			PoolID:       "pool1",
			IPAddress:    "10.0.0.25",
			State:        leaseStateActive,
		},
	}}
	auditRepo := &stubAuditRepo{}
	auditSvc := audit.NewService(auditRepo, zap.NewNop())
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{
		Lifecycle: config.LifecycleConfig{CooldownDuration: time.Minute, MaxCooldowns: 5},
	}, zap.NewNop(), WithAuditService(auditSvc))
	ctx := context.Background()
	if err := svc.MarkDeclined(ctx, "tenant1", "aa:gg", "duplicate"); err != nil {
		t.Fatalf("MarkDeclined returned error: %v", err)
	}
	if len(auditRepo.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditRepo.events))
	}
	event := auditRepo.events[0]
	if event.Action != auditActionLeaseConflict {
		t.Fatalf("expected action %s, got %s", auditActionLeaseConflict, event.Action)
	}
	var payload auditpayload.LeaseConflict
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if payload.Signal != conflictSignalDHCPDecline {
		t.Fatalf("expected signal %s, got %s", conflictSignalDHCPDecline, payload.Signal)
	}
	if payload.Reason != "duplicate" {
		t.Fatalf("expected reason 'duplicate', got %s", payload.Reason)
	}
	if payload.ConflictCount == 0 {
		t.Fatalf("expected conflict count recorded")
	}
	if payload.LeaseID != "lease-4" {
		t.Fatalf("expected lease id lease-4, got %s", payload.LeaseID)
	}
}

func TestClearCooldownReleasesLease(t *testing.T) {
	now := time.Now().UTC()
	repo := &stubLeaseRepo{active: map[string]*models.Lease{
		"aa:11": {
			ID:            "lease-3",
			TenantID:      "tenant1",
			HardwareAddr:  "aa:11",
			State:         leaseStateCooldown,
			CooldownUntil: ptrToTime(now.Add(10 * time.Minute)),
		},
	}}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{}, zap.NewNop())
	lease, changed, err := svc.ClearCooldown(context.Background(), "tenant1", "lease-3")
	if err != nil {
		t.Fatalf("ClearCooldown returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected change flag")
	}
	if lease.State != leaseStateReleased {
		t.Fatalf("expected released state, got %s", lease.State)
	}
	if lease.CooldownUntil != nil {
		t.Fatalf("expected cooldown cleared")
	}
}

func TestAllocateRoundRobinModeCyclesAddresses(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease)}
	pool := &models.AddressPool{
		ID:             "poolRR",
		TenantID:       "tenant1",
		CIDR:           "10.0.0.0/29",
		AllocationMode: models.AllocationModeRoundRobin,
		PriorityWeight: 50,
	}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{"poolRR": pool}}, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())
	ctx := context.Background()
	ips := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		identifier := fmt.Sprintf("aa:rr:%02d", i)
		res, err := svc.AllocateOrReuse(ctx, "tenant1", identifier, models.LeaseProfile{}, "poolRR", "")
		if err != nil {
			t.Fatalf("AllocateOrReuse failed: %v", err)
		}
		ips = append(ips, res.Lease.IPAddress)
	}
	expected := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	for i, want := range expected {
		if ips[i] != want {
			t.Fatalf("expected ip %s at position %d, got %s", want, i, ips[i])
		}
	}
}

func TestPriorityWeightedFallsBackWhenPreferredRangeBusy(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease), used: []string{"10.0.0.1"}}
	pool := &models.AddressPool{
		ID:             "poolPW",
		TenantID:       "tenant1",
		CIDR:           "10.0.0.0/29",
		AllocationMode: models.AllocationModePriorityWeighted,
		PriorityWeight: 10,
	}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{"poolPW": pool}}, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())
	res, err := svc.AllocateOrReuse(context.Background(), "tenant1", "aa:pw", models.LeaseProfile{}, "poolPW", "")
	if err != nil {
		t.Fatalf("AllocateOrReuse returned error: %v", err)
	}
	if res.Lease.IPAddress != "10.0.0.2" {
		t.Fatalf("expected fallback ip 10.0.0.2, got %s", res.Lease.IPAddress)
	}
}

func TestAllocateOrReusePrefixes(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease)}
	pool := &models.AddressPool{ID: "poolV6", TenantID: "tenant1", CIDR: "2001:db8::/48"}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{"poolV6": pool}}, config.PolicyConfig{DefaultLeaseProfile: config.LeaseProfileConfig{DefaultDuration: time.Hour}}, zap.NewNop())
	requests := []PrefixRequest{{IAPDID: 1, PrefixLength: 56}}
	delegations, err := svc.AllocateOrReusePrefixes(context.Background(), "tenant1", "duid-1", models.LeaseProfile{DefaultDuration: time.Hour}, pool, requests)
	if err != nil {
		t.Fatalf("AllocateOrReusePrefixes returned error: %v", err)
	}
	if len(delegations) != 1 {
		t.Fatalf("expected one delegation, got %d", len(delegations))
	}
	second, err := svc.AllocateOrReusePrefixes(context.Background(), "tenant1", "duid-1", models.LeaseProfile{DefaultDuration: time.Hour}, pool, requests)
	if err != nil {
		t.Fatalf("second allocation error: %v", err)
	}
	if second[0].Prefix != delegations[0].Prefix {
		t.Fatalf("expected reuse of prefix %s, got %s", delegations[0].Prefix, second[0].Prefix)
	}
}

func TestResolvePoolBoundsIPv4(t *testing.T) {
	repo := &stubLeaseRepo{active: make(map[string]*models.Lease)}
	svc := NewService(repo, &stubPoolReader{pools: map[string]*models.AddressPool{}}, config.PolicyConfig{}, zap.NewNop())
	pool := &models.AddressPool{CIDR: "10.0.0.0/29"}
	prefix, err := netip.ParsePrefix(pool.CIDR)
	if err != nil {
		t.Fatalf("failed to parse cidr: %v", err)
	}
	start, end := svc.resolvePoolBounds(pool, prefix)
	if start.String() != "10.0.0.1" || end.String() != "10.0.0.6" {
		t.Fatalf("unexpected bounds: %s - %s", start, end)
	}
}

type stubLeaseRepo struct {
	active          map[string]*models.Lease
	used            []string
	cooldown        []string
	prefixActive    map[string]*models.PrefixLease
	prefixList      []models.PrefixLease
	identifierCount map[string]int
	userCount       map[string]int
	poolCounts      map[string]int64
	deviceCounts    map[string]int64
	historyRecords  []models.Lease
	historyTotal    int
}

func (r *stubLeaseRepo) GetActiveLease(ctx context.Context, tenantID, identifier string) (*models.Lease, error) {
	if lease, ok := r.active[identifier]; ok {
		return lease, nil
	}
	return nil, ErrNotFound
}

func (r *stubLeaseRepo) GetLeaseByID(ctx context.Context, tenantID, leaseID string) (*models.Lease, error) {
	for _, lease := range r.active {
		if lease.ID == leaseID && lease.TenantID == tenantID {
			return lease, nil
		}
	}
	return nil, ErrNotFound
}

func (r *stubLeaseRepo) CreateLease(ctx context.Context, lease *models.Lease) error {
	r.active[lease.HardwareAddr] = lease
	r.used = append(r.used, lease.IPAddress)
	return nil
}

func (r *stubLeaseRepo) UpdateLease(ctx context.Context, lease *models.Lease) error {
	r.active[lease.HardwareAddr] = lease
	return nil
}

func (r *stubLeaseRepo) ListLeasesByState(ctx context.Context, tenantID, poolID, state string, limit int) ([]models.Lease, error) {
	var leases []models.Lease
	for _, lease := range r.active {
		if lease.TenantID != tenantID {
			continue
		}
		if poolID != "" && lease.PoolID != poolID {
			continue
		}
		if state != "" && lease.State != state {
			continue
		}
		leases = append(leases, *lease)
	}
	if limit > 0 && len(leases) > limit {
		leases = leases[:limit]
	}
	return leases, nil
}

func (r *stubLeaseRepo) CountActiveLeases(ctx context.Context, tenantID string) (int, error) {
	count := 0
	for _, lease := range r.active {
		if lease.TenantID != tenantID {
			continue
		}
		if lease.State == leaseStateActive {
			count++
		}
	}
	return count, nil
}

func (r *stubLeaseRepo) CountActiveLeasesByIdentifier(ctx context.Context, tenantID, identifier string) (int, error) {
	if len(r.identifierCount) > 0 {
		if val, ok := r.identifierCount[strings.ToLower(identifier)]; ok {
			return val, nil
		}
	}
	count := 0
	for _, lease := range r.active {
		if lease.TenantID != tenantID {
			continue
		}
		if lease.State != leaseStateActive {
			continue
		}
		if strings.EqualFold(lease.HardwareAddr, identifier) || strings.EqualFold(lease.ClientID, identifier) {
			count++
		}
	}
	return count, nil
}

func (r *stubLeaseRepo) CountActiveLeasesByUser(ctx context.Context, tenantID, userID string) (int, error) {
	if len(r.userCount) > 0 {
		if val, ok := r.userCount[userID]; ok {
			return val, nil
		}
	}
	count := 0
	for _, lease := range r.active {
		if lease.TenantID != tenantID || lease.State != leaseStateActive {
			continue
		}
		if lease.UserID == userID && userID != "" {
			count++
		}
	}
	return count, nil
}

func (r *stubLeaseRepo) CountActiveLeasesByPool(ctx context.Context, tenantID string, poolIDs []string) (map[string]int64, error) {
	if len(r.poolCounts) > 0 {
		result := make(map[string]int64, len(r.poolCounts))
		for k, v := range r.poolCounts {
			result[k] = v
		}
		return result, nil
	}
	filter := make(map[string]struct{}, len(poolIDs))
	for _, id := range poolIDs {
		filter[id] = struct{}{}
	}
	counts := make(map[string]int64)
	for _, lease := range r.active {
		if lease.TenantID != tenantID || lease.State != leaseStateActive {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[lease.PoolID]; !ok {
				continue
			}
		}
		counts[lease.PoolID]++
	}
	return counts, nil
}

func (r *stubLeaseRepo) CountActiveLeasesByDeviceType(ctx context.Context, tenantID string) (map[string]int64, error) {
	if len(r.deviceCounts) > 0 {
		result := make(map[string]int64, len(r.deviceCounts))
		for k, v := range r.deviceCounts {
			result[k] = v
		}
		return result, nil
	}
	counts := make(map[string]int64)
	for _, lease := range r.active {
		if lease.TenantID != tenantID || lease.State != leaseStateActive {
			continue
		}
		key := strings.TrimSpace(lease.DeviceType)
		if key == "" {
			key = "unknown"
		}
		counts[key]++
	}
	return counts, nil
}

func (r *stubLeaseRepo) UpdateSecurityState(ctx context.Context, tenantID, identifier, state string, updatedAt time.Time) error {
	if lease, ok := r.active[identifier]; ok {
		lease.SecurityState = state
		lease.UpdatedAt = updatedAt
	}
	return nil
}

func (r *stubLeaseRepo) UpdateSecurityStateByID(ctx context.Context, tenantID, leaseID, state string, updatedAt time.Time) error {
	for _, lease := range r.active {
		if lease.ID == leaseID && lease.TenantID == tenantID {
			lease.SecurityState = state
			lease.UpdatedAt = updatedAt
			return nil
		}
	}
	return ErrNotFound
}

func (r *stubLeaseRepo) ListLeases(ctx context.Context, tenantID string, state string, limit, offset int) ([]models.Lease, error) {
	return nil, nil
}

func (r *stubLeaseRepo) ListActiveIPs(ctx context.Context, tenantID, poolID string) ([]string, error) {
	ips := make([]string, len(r.used))
	copy(ips, r.used)
	return ips, nil
}

func (r *stubLeaseRepo) ListCooldownIPs(ctx context.Context, tenantID, poolID string, reference time.Time) ([]string, error) {
	return r.cooldown, nil
}

func (r *stubLeaseRepo) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error) {
	return nil, nil
}

func (r *stubLeaseRepo) GetActivePrefixLease(ctx context.Context, tenantID, clientID string, iapdID uint32) (*models.PrefixLease, error) {
	if r.prefixActive == nil {
		return nil, ErrNotFound
	}
	key := fmt.Sprintf("%s:%d", clientID, iapdID)
	if lease, ok := r.prefixActive[key]; ok {
		return lease, nil
	}
	return nil, ErrNotFound
}

func (r *stubLeaseRepo) GetPrefixLeaseByID(ctx context.Context, tenantID, prefixLeaseID string) (*models.PrefixLease, error) {
	if r.prefixActive == nil {
		return nil, ErrNotFound
	}
	for _, lease := range r.prefixActive {
		if lease.ID == prefixLeaseID && lease.TenantID == tenantID {
			return lease, nil
		}
	}
	return nil, ErrNotFound
}

func (r *stubLeaseRepo) CreatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	if r.prefixActive == nil {
		r.prefixActive = make(map[string]*models.PrefixLease)
	}
	key := fmt.Sprintf("%s:%d", lease.ClientID, lease.IAPDID)
	r.prefixActive[key] = lease
	r.prefixList = append(r.prefixList, *lease)
	return nil
}

func (r *stubLeaseRepo) UpdatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	if r.prefixActive == nil {
		r.prefixActive = make(map[string]*models.PrefixLease)
	}
	key := fmt.Sprintf("%s:%d", lease.ClientID, lease.IAPDID)
	r.prefixActive[key] = lease
	return nil
}

func (r *stubLeaseRepo) ListActivePrefixes(ctx context.Context, tenantID, poolID string) ([]models.PrefixLease, error) {
	out := make([]models.PrefixLease, len(r.prefixList))
	copy(out, r.prefixList)
	return out, nil
}

func (r *stubLeaseRepo) ListPrefixLeases(ctx context.Context, tenantID, state string, limit, offset int) ([]models.PrefixLease, error) {
	out := make([]models.PrefixLease, len(r.prefixList))
	copy(out, r.prefixList)
	return out, nil
}

func (r *stubLeaseRepo) SearchLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) ([]models.Lease, error) {
	filtered := r.filterHistoryRecords(tenantID, filter)
	if len(filtered) == 0 {
		return nil, nil
	}
	start := filter.Offset
	if start >= len(filtered) {
		return nil, nil
	}
	end := len(filtered)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	result := make([]models.Lease, end-start)
	copy(result, filtered[start:end])
	return result, nil
}

func (r *stubLeaseRepo) CountLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) (int, error) {
	if r.historyTotal > 0 {
		return r.historyTotal, nil
	}
	return len(r.filterHistoryRecords(tenantID, filter)), nil
}

func (r *stubLeaseRepo) filterHistoryRecords(tenantID string, filter models.LeaseHistoryFilter) []models.Lease {
	if len(r.historyRecords) == 0 {
		return nil
	}
	results := make([]models.Lease, 0, len(r.historyRecords))
	for _, lease := range r.historyRecords {
		if tenantID != "" && lease.TenantID != "" && lease.TenantID != tenantID {
			continue
		}
		if filter.Identifier != "" && !strings.EqualFold(lease.HardwareAddr, filter.Identifier) && !strings.EqualFold(lease.ClientID, filter.Identifier) {
			continue
		}
		if filter.IPAddress != "" && lease.IPAddress != filter.IPAddress {
			continue
		}
		if filter.State != "" && lease.State != filter.State {
			continue
		}
		if !filter.From.IsZero() && lease.UpdatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && lease.UpdatedAt.After(filter.To) {
			continue
		}
		results = append(results, lease)
	}
	return results
}

type stubPoolReader struct {
	pools map[string]*models.AddressPool
}

func (s *stubPoolReader) GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error) {
	pool, ok := s.pools[poolID]
	if !ok || pool.TenantID != tenantID {
		return nil, errors.New("pool not found")
	}
	return pool, nil
}

func ptrToTime(t time.Time) *time.Time {
	return &t
}

type stubAuditRepo struct {
	events []models.AuditEvent
}

func (r *stubAuditRepo) InsertEvent(ctx context.Context, event *models.AuditEvent) error {
	if event == nil {
		return nil
	}
	copyEvent := *event
	r.events = append(r.events, copyEvent)
	return nil
}

func (r *stubAuditRepo) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]models.AuditEvent, error) {
	return nil, nil
}

func (r *stubAuditRepo) ListEventsFiltered(ctx context.Context, tenantID string, filter audit.ListEventsFilter) ([]models.AuditEvent, error) {
	return nil, nil
}
