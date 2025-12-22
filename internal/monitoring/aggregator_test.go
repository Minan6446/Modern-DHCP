package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
	"modern-dhcp/pkg/models"
)

func TestAggregatorOverviewReturnsSnapshots(t *testing.T) {
	pools := []models.AddressPool{
		{
			ID:         "poolA",
			Name:       "Main",
			TenantID:   "tenant1",
			Scope:      "GLOBAL",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.10",
			VLANID:     intPtr(100),
			Location:   strPtr("dc1"),
		},
		{
			ID:         "poolB",
			Name:       "Edge",
			TenantID:   "tenant1",
			Scope:      "GLOBAL",
			RangeStart: "192.168.1.1",
			RangeEnd:   "192.168.1.2",
		},
	}
	poolSvc := pool.NewService(&stubPoolRepo{pools: pools}, zap.NewNop())
	tracker := &stubTracker{snapshots: map[string][]RequestPhaseSnapshot{
		"tenant1": {
			{Protocol: "dhcpv4", Message: "DISCOVER", Success: 10},
		},
	}}
	rateTracker := NewRateLimitTracker(8)
	rateTracker.Record(ratelimit.Hit{TenantID: "tenant1", MAC: "aa:bb", PortID: "gi1/0/1", OccurredAt: time.Now().UTC()})
	snoopTracker := NewSnoopingTracker(8)
	snoopTracker.Record(snooping.Observation{TenantID: "tenant1", Result: snooping.ObservationResultTrusted, PortID: "gi1/0/1", MAC: "aa:bb", OccurredAt: time.Now().UTC()})
	leaseStats := &stubLeaseStats{
		poolCounts:   map[string]int64{"poolA": 7, "poolB": 1},
		deviceCounts: map[string]int64{"laptop": 5, "unknown": 3},
	}
	agg := NewAggregator(Options{
		PoolService:     poolSvc,
		LeaseRepo:       leaseStats,
		RequestTracker:  tracker,
		RateTracker:     rateTracker,
		SnoopingTracker: snoopTracker,
		Logger:          zap.NewNop(),
	})

	overview, err := agg.Overview(context.Background(), "tenant1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(overview.PoolUsage) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(overview.PoolUsage))
	}
	if overview.PoolUsage[0].PoolID != "poolA" {
		t.Fatalf("expected poolA to be most utilized, got %s", overview.PoolUsage[0].PoolID)
	}
	if len(overview.RequestPhases) != 1 {
		t.Fatalf("expected request snapshot propagated")
	}
	if len(overview.ClientDistribution.ByDeviceType) == 0 {
		t.Fatalf("expected client distribution to include device types")
	}
	if overview.SystemHealth.Goroutines == 0 {
		t.Fatalf("expected system health goroutines populated")
	}
	if overview.Security.RateLimit.TotalHits == 0 {
		t.Fatalf("expected rate limit stats populated")
	}
	if overview.Security.Snooping.Total == 0 {
		t.Fatalf("expected snooping stats populated")
	}
}

// --- Test helpers ---

type stubTracker struct {
	snapshots map[string][]RequestPhaseSnapshot
}

func (s *stubTracker) ObserveRequest(string, string, string, bool, time.Duration) {}

func (s *stubTracker) Snapshot(tenantID string) []RequestPhaseSnapshot {
	return s.snapshots[tenantID]
}

type stubLeaseStats struct {
	poolCounts   map[string]int64
	deviceCounts map[string]int64
}

func (s *stubLeaseStats) CountActiveLeasesByPool(_ context.Context, _ string, poolIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(poolIDs))
	for _, id := range poolIDs {
		counts[id] = s.poolCounts[id]
	}
	return counts, nil
}

func (s *stubLeaseStats) CountActiveLeasesByDeviceType(_ context.Context, _ string) (map[string]int64, error) {
	return s.deviceCounts, nil
}

type stubPoolRepo struct {
	pools []models.AddressPool
}

func (s *stubPoolRepo) ListPools(_ context.Context, tenantID string, limit, offset int) ([]models.AddressPool, error) {
	return s.pools, nil
}

func (s *stubPoolRepo) CountPools(context.Context, string) (int, error) {
	return len(s.pools), nil
}

func (s *stubPoolRepo) GetPool(context.Context, string, string) (*models.AddressPool, error) {
	return nil, errors.New("not implemented")
}

func (s *stubPoolRepo) InsertPool(context.Context, *models.AddressPool) error {
	return errors.New("not implemented")
}
func (s *stubPoolRepo) UpdatePool(context.Context, *models.AddressPool) error {
	return errors.New("not implemented")
}
func (s *stubPoolRepo) DeletePool(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (s *stubPoolRepo) FindPools(context.Context, string, pool.MetadataFilter) ([]models.AddressPool, error) {
	return nil, errors.New("not implemented")
}
func (s *stubPoolRepo) ListBindings(context.Context, string, int, int) ([]models.StaticBinding, error) {
	return nil, errors.New("not implemented")
}
func (s *stubPoolRepo) GetBinding(context.Context, string, string) (*models.StaticBinding, error) {
	return nil, errors.New("not implemented")
}
func (s *stubPoolRepo) InsertBinding(context.Context, *models.StaticBinding) error {
	return errors.New("not implemented")
}
func (s *stubPoolRepo) UpdateBinding(context.Context, *models.StaticBinding) error {
	return errors.New("not implemented")
}
func (s *stubPoolRepo) DeleteBinding(context.Context, string, string) error {
	return errors.New("not implemented")
}

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }
