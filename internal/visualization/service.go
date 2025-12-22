package visualization

import (
	"context"
	"errors"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

// ErrDisabled indicates visualization service is not configured.
var ErrDisabled = errors.New("visualization disabled")

const (
	defaultMaxNodes    = 200
	defaultHeatmapSize = 60
)

// GeoPoint provides an explicit latitude/longitude mapping for a location label.
type GeoPoint struct {
	Latitude  float64
	Longitude float64
}

// Options configures the visualization service.
type Options struct {
	Logger              *zap.Logger
	PoolService         *pool.Service
	LeaseReader         monitoring.LeaseStatsReader
	MaxNodes            int
	HeatmapLimit        int
	LocationCoordinates map[string]GeoPoint
}

// Service projects monitoring/pool state onto UI-friendly canvases.
type Service struct {
	logger        *zap.Logger
	pools         *pool.Service
	leases        monitoring.LeaseStatsReader
	maxNodes      int
	heatmapLimit  int
	locationHints map[string]GeoPoint
}

// NewService returns a visualization service if feature flags are enabled.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	maxNodes := opts.MaxNodes
	if maxNodes <= 0 {
		maxNodes = defaultMaxNodes
	}
	heatmapLimit := opts.HeatmapLimit
	if heatmapLimit <= 0 {
		heatmapLimit = defaultHeatmapSize
	}
	locationHints := make(map[string]GeoPoint)
	for key, point := range opts.LocationCoordinates {
		locationHints[strings.ToLower(strings.TrimSpace(key))] = point
	}
	return &Service{
		logger:        logger,
		pools:         opts.PoolService,
		leases:        opts.LeaseReader,
		maxNodes:      maxNodes,
		heatmapLimit:  heatmapLimit,
		locationHints: locationHints,
	}
}

// TopologyNode describes an item in the network graph.
type TopologyNode struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	Position Position          `json:"position"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Position stores 2D coordinates on the canvas.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TopologyEdge links two nodes.
type TopologyEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Health string `json:"health"`
}

// TopologySnapshot aggregates nodes/edges.
type TopologySnapshot struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	TenantID    string         `json:"tenantId"`
	Nodes       []TopologyNode `json:"nodes"`
	Edges       []TopologyEdge `json:"edges"`
}

// HeatmapPoint describes lease intensity for a geohash bucket.
type HeatmapPoint struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	ActiveLeases int     `json:"activeLeases"`
	Alerts       int     `json:"alerts"`
}

// HeatmapSnapshot is the payload for map visualizations.
type HeatmapSnapshot struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	TenantID    string         `json:"tenantId"`
	Points      []HeatmapPoint `json:"points"`
}

// Topology builds a graph from the tenant's pools and utilization.
func (s *Service) Topology(ctx context.Context, tenantID string) (TopologySnapshot, error) {
	if s == nil || s.pools == nil || s.leases == nil {
		return TopologySnapshot{}, ErrDisabled
	}
	pools, err := s.pools.ListPools(ctx, tenantID, s.maxNodes, 0)
	if err != nil {
		return TopologySnapshot{}, err
	}
	if len(pools) == 0 {
		return TopologySnapshot{GeneratedAt: time.Now().UTC(), TenantID: tenantID}, nil
	}
	counts, err := s.leases.CountActiveLeasesByPool(ctx, tenantID, collectPoolIDs(pools))
	if err != nil {
		return TopologySnapshot{}, err
	}
	nodes, edges := s.buildGraphArtifacts(pools, counts)
	return TopologySnapshot{
		GeneratedAt: time.Now().UTC(),
		TenantID:    tenantID,
		Nodes:       nodes,
		Edges:       edges,
	}, nil
}

// LeaseHeatmap projects pool utilization onto geo buckets.
func (s *Service) LeaseHeatmap(ctx context.Context, tenantID string) (HeatmapSnapshot, error) {
	if s == nil || s.pools == nil || s.leases == nil {
		return HeatmapSnapshot{}, ErrDisabled
	}
	pools, err := s.pools.ListPools(ctx, tenantID, s.heatmapLimit, 0)
	if err != nil {
		return HeatmapSnapshot{}, err
	}
	if len(pools) == 0 {
		return HeatmapSnapshot{GeneratedAt: time.Now().UTC(), TenantID: tenantID}, nil
	}
	counts, err := s.leases.CountActiveLeasesByPool(ctx, tenantID, collectPoolIDs(pools))
	if err != nil {
		return HeatmapSnapshot{}, err
	}
	points := s.buildHeatmapPoints(pools, counts)
	return HeatmapSnapshot{
		GeneratedAt: time.Now().UTC(),
		TenantID:    tenantID,
		Points:      points,
	}, nil
}

func (s *Service) buildGraphArtifacts(pools []models.AddressPool, counts map[string]int64) ([]TopologyNode, []TopologyEdge) {
	type decoratedPool struct {
		pool      models.AddressPool
		allocated int64
		capacity  int64
	}
	buckets := make(map[string][]decoratedPool)
	for _, poolObj := range pools {
		scope := normalizeScopeLabel(poolObj.Scope)
		buckets[scope] = append(buckets[scope], decoratedPool{
			pool:      poolObj,
			allocated: counts[poolObj.ID],
			capacity:  monitoring.CalculatePoolCapacity(poolObj),
		})
	}
	for scope := range buckets {
		list := buckets[scope]
		sort.SliceStable(list, func(i, j int) bool {
			return utilization(list[i].allocated, list[i].capacity) > utilization(list[j].allocated, list[j].capacity)
		})
		buckets[scope] = list
	}
	scopeOrder := []string{"GLOBAL", "SUBNET", "VLAN", "PORT"}
	rows := make([]string, 0, len(scopeOrder))
	rows = append(rows, scopeOrder...)
	for scope := range buckets {
		if !containsScope(rows, scope) {
			rows = append(rows, scope)
		}
	}
	const (
		rowSpacing = 150.0
		colSpacing = 190.0
	)
	nodes := make([]TopologyNode, 0, len(pools))
	nodeSet := make(map[string]struct{}, len(pools))
	for rowIdx, scope := range rows {
		entries := buckets[scope]
		if len(entries) == 0 {
			continue
		}
		centerOffset := float64(len(entries)-1) / 2
		for colIdx, entry := range entries {
			pos := Position{X: (float64(colIdx) - centerOffset) * colSpacing, Y: float64(rowIdx) * rowSpacing}
			util := utilization(entry.allocated, entry.capacity)
			meta := poolMetadata(entry.pool, entry.allocated, entry.capacity)
			node := TopologyNode{
				ID:       entry.pool.ID,
				Label:    entry.pool.Name,
				Type:     strings.ToLower(scope),
				Status:   statusFromUtil(util),
				Position: pos,
				Metadata: meta,
			}
			nodes = append(nodes, node)
			nodeSet[entry.pool.ID] = struct{}{}
		}
	}
	edges := make([]TopologyEdge, 0, len(pools))
	for _, poolObj := range pools {
		if poolObj.ParentID == nil {
			continue
		}
		if _, ok := nodeSet[*poolObj.ParentID]; !ok {
			continue
		}
		if _, ok := nodeSet[poolObj.ID]; !ok {
			continue
		}
		edges = append(edges, TopologyEdge{
			ID:     poolObj.ID + "->" + *poolObj.ParentID,
			Source: *poolObj.ParentID,
			Target: poolObj.ID,
			Type:   "pool",
			Health: "up",
		})
	}
	return nodes, edges
}

func (s *Service) buildHeatmapPoints(pools []models.AddressPool, counts map[string]int64) []HeatmapPoint {
	aggregated := make(map[string]*HeatmapPoint)
	for _, poolObj := range pools {
		location := strings.TrimSpace(derefString(poolObj.Location))
		if location == "" {
			continue
		}
		allocated := counts[poolObj.ID]
		if allocated == 0 {
			continue
		}
		capacity := monitoring.CalculatePoolCapacity(poolObj)
		lat, lon := s.resolveCoordinates(location)
		key := location
		point, exists := aggregated[key]
		if !exists {
			point = &HeatmapPoint{Latitude: lat, Longitude: lon}
			aggregated[key] = point
		}
		point.ActiveLeases += int(allocated)
		point.Alerts += alertLevel(utilization(allocated, capacity))
	}
	points := make([]HeatmapPoint, 0, len(aggregated))
	for _, point := range aggregated {
		points = append(points, *point)
	}
	sort.SliceStable(points, func(i, j int) bool {
		return points[i].ActiveLeases > points[j].ActiveLeases
	})
	return points
}

func (s *Service) resolveCoordinates(location string) (float64, float64) {
	if location == "" {
		return 0, 0
	}
	key := strings.ToLower(location)
	if point, ok := s.locationHints[key]; ok {
		return point.Latitude, point.Longitude
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(location))
	seed := hash.Sum32()
	lat := -60 + float64(seed%120000)/1000.0
	hash.Reset()
	_, _ = hash.Write([]byte(location + "::lon"))
	lonSeed := hash.Sum32()
	lon := -170 + float64(lonSeed%340000)/1000.0
	return lat, lon
}

func poolMetadata(poolObj models.AddressPool, allocated, capacity int64) map[string]string {
	metadata := map[string]string{
		"poolId":    poolObj.ID,
		"scope":     poolObj.Scope,
		"cidr":      poolObj.CIDR,
		"range":     strings.TrimSpace(poolObj.RangeStart) + "-" + strings.TrimSpace(poolObj.RangeEnd),
		"allocated": strconv.FormatInt(allocated, 10),
		"capacity":  strconv.FormatInt(capacity, 10),
	}
	if poolObj.VLANID != nil {
		metadata["vlanId"] = strconv.Itoa(*poolObj.VLANID)
	}
	if poolObj.Location != nil {
		metadata["location"] = strings.TrimSpace(*poolObj.Location)
	}
	if poolObj.ParentID != nil {
		metadata["parentId"] = *poolObj.ParentID
	}
	return metadata
}

func utilization(allocated, capacity int64) float64 {
	if capacity <= 0 {
		if allocated > 0 {
			return 100
		}
		return 0
	}
	return (float64(allocated) / float64(capacity)) * 100
}

func statusFromUtil(util float64) string {
	switch {
	case util >= 95:
		return "critical"
	case util >= 85:
		return "warning"
	default:
		return "healthy"
	}
}

func alertLevel(util float64) int {
	switch {
	case util >= 95:
		return 2
	case util >= 85:
		return 1
	default:
		return 0
	}
}

func normalizeScopeLabel(scope string) string {
	s := strings.TrimSpace(strings.ToUpper(scope))
	if s == "" {
		return "GLOBAL"
	}
	return s
}

func collectPoolIDs(pools []models.AddressPool) []string {
	ids := make([]string, 0, len(pools))
	for _, poolObj := range pools {
		ids = append(ids, poolObj.ID)
	}
	return ids
}

func containsScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
