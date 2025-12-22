package monitoring

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.uber.org/zap"

	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

// Aggregator assembles monitoring payloads for dashboards.
type Aggregator struct {
	pools          *pool.Service
	leases         LeaseStatsReader
	tracker        RequestTracker
	rateTracker    RateLimitTracker
	snoopTracker   SnoopingTracker
	securityWindow time.Duration
	logger         *zap.Logger
}

// LeaseStatsReader exposes aggregate lease counters for monitoring snapshots.
type LeaseStatsReader interface {
	CountActiveLeasesByPool(ctx context.Context, tenantID string, poolIDs []string) (map[string]int64, error)
	CountActiveLeasesByDeviceType(ctx context.Context, tenantID string) (map[string]int64, error)
}

// Options configures Aggregator dependencies.
type Options struct {
	PoolService     *pool.Service
	LeaseRepo       LeaseStatsReader
	RequestTracker  RequestTracker
	RateTracker     RateLimitTracker
	SnoopingTracker SnoopingTracker
	SecurityWindow  time.Duration
	Logger          *zap.Logger
}

// NewAggregator creates a new Aggregator instance.
func NewAggregator(opts Options) *Aggregator {
	window := opts.SecurityWindow
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &Aggregator{
		pools:          opts.PoolService,
		leases:         opts.LeaseRepo,
		tracker:        opts.RequestTracker,
		rateTracker:    opts.RateTracker,
		snoopTracker:   opts.SnoopingTracker,
		securityWindow: window,
		logger:         opts.Logger,
	}
}

// Overview returns the combined dashboard snapshot.
func (a *Aggregator) Overview(ctx context.Context, tenantID string, limit int) (OverviewSnapshot, error) {
	usage, counts, err := a.poolUsage(ctx, tenantID, limit)
	if err != nil {
		return OverviewSnapshot{}, err
	}
	requests := a.requestSnapshots(tenantID)
	distribution, err := a.clientDistribution(ctx, tenantID, usage, counts)
	if err != nil {
		return OverviewSnapshot{}, err
	}
	health := a.systemHealth(ctx)
	return OverviewSnapshot{
		GeneratedAt:        time.Now().UTC(),
		PoolUsage:          usage,
		RequestPhases:      requests,
		ClientDistribution: distribution,
		SystemHealth:       health,
		Security:           a.securitySnapshot(tenantID),
	}, nil
}

func (a *Aggregator) securitySnapshot(tenantID string) SecuritySnapshot {
	snapshot := SecuritySnapshot{}
	window := a.securityWindow
	if window <= 0 {
		return snapshot
	}
	if a.rateTracker != nil {
		snapshot.RateLimit = a.rateTracker.Snapshot(tenantID, window)
	}
	if a.snoopTracker != nil {
		snapshot.Snooping = a.snoopTracker.Snapshot(tenantID, window)
	}
	return snapshot
}

// Pools returns pool utilization snapshots.
func (a *Aggregator) Pools(ctx context.Context, tenantID string, limit int) ([]PoolUsageSummary, error) {
	usage, _, err := a.poolUsage(ctx, tenantID, limit)
	return usage, err
}

// Requests returns DHCP lifecycle stats for the tenant.
func (a *Aggregator) Requests(tenantID string) []RequestPhaseSnapshot {
	return a.requestSnapshots(tenantID)
}

// Health returns the latest system metrics.
func (a *Aggregator) Health(ctx context.Context) SystemHealthSnapshot {
	return a.systemHealth(ctx)
}

// Security returns guard-level telemetry for a tenant.
func (a *Aggregator) Security(tenantID string) SecuritySnapshot {
	if a == nil {
		return SecuritySnapshot{}
	}
	return a.securitySnapshot(tenantID)
}

func (a *Aggregator) poolUsage(ctx context.Context, tenantID string, limit int) ([]PoolUsageSummary, map[string]int64, error) {
	if a.pools == nil || a.leases == nil {
		return nil, nil, errors.New("monitoring: pool service unavailable")
	}
	if limit <= 0 {
		limit = 50
	}
	pools, err := a.pools.ListPools(ctx, tenantID, limit, 0)
	if err != nil {
		return nil, nil, err
	}
	poolIDs := make([]string, 0, len(pools))
	for _, p := range pools {
		poolIDs = append(poolIDs, p.ID)
	}
	counts, err := a.leases.CountActiveLeasesByPool(ctx, tenantID, poolIDs)
	if err != nil {
		return nil, nil, err
	}
	usage := make([]PoolUsageSummary, 0, len(pools))
	for _, poolObj := range pools {
		capacity := CalculatePoolCapacity(poolObj)
		allocated := counts[poolObj.ID]
		util := 0.0
		if capacity > 0 {
			util = (float64(allocated) / float64(capacity)) * 100
		}
		usage = append(usage, PoolUsageSummary{
			PoolID:      poolObj.ID,
			Name:        poolObj.Name,
			Scope:       poolObj.Scope,
			VLANID:      derefInt(poolObj.VLANID),
			Location:    strings.TrimSpace(derefString(poolObj.Location)),
			Allocated:   allocated,
			Capacity:    capacity,
			Utilization: util,
		})
	}
	sort.Slice(usage, func(i, j int) bool {
		return usage[i].Utilization > usage[j].Utilization
	})
	return usage, counts, nil
}

func (a *Aggregator) clientDistribution(ctx context.Context, tenantID string, pools []PoolUsageSummary, poolCounts map[string]int64) (ClientDistributionSnapshot, error) {
	var snapshot ClientDistributionSnapshot
	if len(pools) > 0 {
		byVLAN := map[string]int64{}
		byLocation := map[string]int64{}
		for _, entry := range pools {
			keyVLAN := "unassigned"
			if entry.VLANID > 0 {
				keyVLAN = strconv.Itoa(entry.VLANID)
			}
			byVLAN[keyVLAN] += entry.Allocated
			keyLocation := entry.Location
			if keyLocation == "" {
				keyLocation = "unknown"
			}
			byLocation[keyLocation] += entry.Allocated
		}
		snapshot.ByVLAN = toDimensionCounts(byVLAN)
		snapshot.ByLocation = toDimensionCounts(byLocation)
	}
	if a.leases == nil {
		return snapshot, nil
	}
	types, err := a.leases.CountActiveLeasesByDeviceType(ctx, tenantID)
	if err != nil {
		return snapshot, err
	}
	snapshot.ByDeviceType = toDimensionCounts(types)
	return snapshot, nil
}

func (a *Aggregator) requestSnapshots(tenantID string) []RequestPhaseSnapshot {
	if a.tracker == nil {
		return nil
	}
	return a.tracker.Snapshot(tenantID)
}

func (a *Aggregator) systemHealth(ctx context.Context) SystemHealthSnapshot {
	health := SystemHealthSnapshot{Timestamp: time.Now().UTC(), Goroutines: runtime.NumGoroutine()}
	if percentages, err := cpu.PercentWithContext(ctx, 100*time.Millisecond, false); err == nil && len(percentages) > 0 {
		health.CPUPercent = percentages[0]
	}
	if memStats, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		health.MemoryPercent = memStats.UsedPercent
		health.MemoryUsedBytes = memStats.Used
	}
	if diskStats, err := disk.UsageWithContext(ctx, systemRoot()); err == nil {
		health.DiskPercent = diskStats.UsedPercent
	}
	if netStats, err := net.IOCountersWithContext(ctx, false); err == nil && len(netStats) > 0 {
		health.NetworkRxBytes = netStats[0].BytesRecv
		health.NetworkTxBytes = netStats[0].BytesSent
	}
	return health
}

// CalculatePoolCapacity returns the total available IPv4 addresses for the pool range.
func CalculatePoolCapacity(poolObj models.AddressPool) int64 {
	start, errStart := netip.ParseAddr(strings.TrimSpace(poolObj.RangeStart))
	end, errEnd := netip.ParseAddr(strings.TrimSpace(poolObj.RangeEnd))
	if errStart != nil || errEnd != nil {
		return 0
	}
	if start.Is4() && end.Is4() {
		return ipv4Capacity(start, end)
	}
	return 0
}

func ipv4Capacity(start, end netip.Addr) int64 {
	start32 := int64(addrToUint32(start))
	end32 := int64(addrToUint32(end))
	if end32 < start32 {
		return 0
	}
	return (end32 - start32) + 1
}

func addrToUint32(addr netip.Addr) uint32 {
	if !addr.Is4() {
		return 0
	}
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func derefInt(val *int) int {
	if val == nil {
		return 0
	}
	return *val
}

func derefString(val *string) string {
	if val == nil {
		return ""
	}
	return *val
}

func toDimensionCounts(source map[string]int64) []DimensionCount {
	out := make([]DimensionCount, 0, len(source))
	for key, count := range source {
		out = append(out, DimensionCount{Key: key, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func systemRoot() string {
	if runtime.GOOS == "windows" {
		if drive := os.Getenv("SystemDrive"); drive != "" {
			return drive + "\\"
		}
		return "c:\\"
	}
	return "/"
}
