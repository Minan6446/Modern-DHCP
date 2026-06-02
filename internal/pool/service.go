package pool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/pkg/models"
)

var (
	ErrInvalidCIDR            = errors.New("CIDR 不合法")
	ErrReserveOutOfRange      = errors.New("预留百分比超出范围")
	ErrLeaseProfileRequired   = errors.New("缺少租约策略 ID")
	ErrLeaseTimeOutOfRange    = errors.New("租期超出允许范围")
	ErrLeaseTimeOrder         = errors.New("最长租期不能小于最小租期")
	ErrInvalidScope           = errors.New("地址池作用域不合法")
	ErrParentScope            = errors.New("父作用域不兼容")
	ErrInvalidRange           = errors.New("地址池范围不合法")
	ErrRangeOutside           = errors.New("地址池范围超出父级边界")
	ErrExclusionInvalid       = errors.New("排除地址段不合法")
	ErrExclusionOverlap       = errors.New("排除地址段存在重叠")
	ErrInvalidParent          = errors.New("父级引用不合法")
	ErrVLANRequired           = errors.New("该作用域需要 VLAN ID")
	ErrEndpointMetadata       = errors.New("端口作用域需要接口/SSID/位置")
	ErrInvalidVLAN            = errors.New("VLAN ID 不合法")
	ErrParentVLANMismatch     = errors.New("VLAN ID 必须与父级一致")
	ErrPoolNotFound           = errors.New("未找到匹配的地址池")
	ErrPoolAmbiguous          = errors.New("IP 匹配到多个地址池")
	ErrBindingNotFound        = errors.New("未找到绑定记录")
	ErrBindingTypeUnsupported = errors.New("仅支持 MAC 地址绑定")
)

const (
	scopeGlobal = "GLOBAL"
	scopeSubnet = "SUBNET"
	scopeVLAN   = "VLAN"
	scopePort   = "PORT"
)

var scopeOrder = map[string]int{
	scopeGlobal: 0,
	scopeSubnet: 1,
	scopeVLAN:   2,
	scopePort:   3,
}

const (
	metricOutcomeSuccess = "success"
	metricOutcomeError   = "error"
	metricLabelUnknown   = "unknown"
)

// Service exposes business logic for pools and bindings.
type Service struct {
	repo    Repository
	logger  *zap.Logger
	quota   tenant.QuotaEnforcer
	metrics *metrics.Collector
}

// ServiceOption configures optional dependencies for the pool service.
type ServiceOption func(*Service)

// WithQuotaEnforcer wires tenant quota checks into pool mutations.
func WithQuotaEnforcer(enforcer tenant.QuotaEnforcer) ServiceOption {
	return func(s *Service) {
		s.quota = enforcer
	}
}

// WithMetricsCollector wires Prometheus metrics into the pool service.
func WithMetricsCollector(collector *metrics.Collector) ServiceOption {
	return func(s *Service) {
		s.metrics = collector
	}
}

// NewService creates a Service.
func NewService(repo Repository, logger *zap.Logger, opts ...ServiceOption) *Service {
	svc := &Service{repo: repo, logger: logger}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// GetPool retrieves a pool by identifier.
func (s *Service) GetPool(ctx context.Context, scope ResourceScope, poolID string) (_ *models.AddressPool, err error) {
	done := s.trackPoolOperation(scope, "get_pool")
	defer func() { done(err) }()
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.GetPool(ctx, scope.AccessScope(), poolID)
}

// GetLeaseProfile returns the lease profile by ID.
func (s *Service) GetLeaseProfile(ctx context.Context, scope ResourceScope, profileID string) (_ *models.LeaseProfile, err error) {
	done := s.trackPoolOperation(scope, "get_lease_profile")
	defer func() { done(err) }()
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.GetLeaseProfile(ctx, scope.AccessScope(), profileID)
}

// PoolCreateRequest represents the required data to create a pool.
type PoolCreateRequest struct {
	TenantID       string
	Scope          string
	ParentID       *string
	Name           string
	CIDR           string
	Network        string
	Netmask        string
	RangeStart     string
	RangeEnd       string
	Gateway        string
	Option43       string
	DNS            models.StringList
	VLANID         *int
	InterfaceID    *string
	SSID           *string
	Location       *string
	ReservePercent int
	MinLeaseTime   int
	MaxLeaseTime   int
	LeaseProfileID string
	Tags           []byte
	Exclusions     models.IPRangeList
	AllocationMode string
	PriorityWeight int
	Status         string
}

// PoolUpdateRequest updates mutable pool fields.
type PoolUpdateRequest struct {
	PoolID string
	PoolCreateRequest
}

// BindingCreateRequest captures static binding input.
type BindingCreateRequest struct {
	TenantID       string
	Identifier     string
	IdentifierType string
	PoolID         string
	IPAddress      string
	LeaseProfileID string
	Metadata       []byte
}

// ListPools returns paginated pools for the tenant/scope.
func (s *Service) ListPools(ctx context.Context, scope ResourceScope, limit, offset int) (_ []models.AddressPool, err error) {
	done := s.trackPoolOperation(scope, "list_pools")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.ListPools(ctx, scope.AccessScope(), limit, offset)
}

// ListPoolsByFamily returns paginated pools for the tenant/scope filtered by IP family (4 or 6).
func (s *Service) ListPoolsByFamily(ctx context.Context, scope ResourceScope, family, limit, offset int) (_ []models.AddressPool, err error) {
	done := s.trackPoolOperation(scope, "list_pools_family")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.ListPoolsByFamily(ctx, scope.AccessScope(), family, limit, offset)
}

// UpsertUsageDaily writes daily usage snapshots for pools.
func (s *Service) UpsertUsageDaily(ctx context.Context, scope ResourceScope, entries []models.PoolUsageDaily) (err error) {
	done := s.trackPoolOperation(scope, "upsert_pool_usage_daily")
	defer func() { done(err) }()
	if s.repo == nil {
		return ErrPoolNotFound
	}
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return err
	}
	for i := range entries {
		entries[i].TenantID = tenantID
	}
	return s.repo.UpsertPoolUsageDaily(ctx, tenantID, entries)
}

// ListUsageDaily returns daily usage snapshots for a pool.
func (s *Service) ListUsageDaily(ctx context.Context, scope ResourceScope, poolID string, from, to time.Time) (_ []models.PoolUsageDaily, err error) {
	done := s.trackPoolOperation(scope, "list_pool_usage_daily")
	defer func() { done(err) }()
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return nil, err
	}
	return s.repo.ListPoolUsageDaily(ctx, tenantID, poolID, from, to)
}

// CountPools returns total pools under the tenant/scope.
func (s *Service) CountPools(ctx context.Context, scope ResourceScope) (_ int, err error) {
	done := s.trackPoolOperation(scope, "count_pools")
	defer func() { done(err) }()
	if s.repo == nil {
		return 0, ErrPoolNotFound
	}
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return 0, err
	}
	return s.repo.CountPools(ctx, tenantID)
}

// CountPoolsByFamily counts pools for a tenant filtered by IP family (4 or 6).
func (s *Service) CountPoolsByFamily(ctx context.Context, scope ResourceScope, family int) (_ int, err error) {
	done := s.trackPoolOperation(scope, "count_pools_family")
	defer func() { done(err) }()
	if s.repo == nil {
		return 0, ErrPoolNotFound
	}
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return 0, err
	}
	return s.repo.CountPoolsByFamily(ctx, tenantID, family)
}

// FindPools filters pools by metadata fields.
func (s *Service) FindPools(ctx context.Context, scope ResourceScope, filter MetadataFilter) (_ []models.AddressPool, err error) {
	done := s.trackPoolOperation(scope, "find_pools")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.FindPools(ctx, scope.AccessScope(), normalizeFilter(filter))
}

// ResolvePool attempts to locate the most specific pool for the selector metadata.
func (s *Service) ResolvePool(ctx context.Context, scope ResourceScope, selector MetadataSelector) (_ *models.AddressPool, err error) {
	done := s.trackPoolOperation(scope, "resolve_pool")
	defer func() { done(err) }()
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	pool, err := s.repo.ResolvePool(ctx, scope.AccessScope(), normalizeSelector(selector))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	if pool == nil {
		return nil, ErrPoolNotFound
	}
	return pool, nil
}

// CreatePool validates and persists a new pool.
func (s *Service) CreatePool(ctx context.Context, scopeRef ResourceScope, req PoolCreateRequest) (_ *models.AddressPool, err error) {
	done := s.trackPoolOperation(scopeRef, "create_pool")
	defer func() { done(err) }()
	tenantID, err := scopeRef.TenantIDOrErr()
	if err != nil {
		return nil, err
	}
	req.TenantID = tenantID
	scopeName, err := normalizeScope(req.Scope)
	if err != nil {
		return nil, err
	}
	if req.LeaseProfileID == "" {
		return nil, ErrLeaseProfileRequired
	}
	minLeaseTime, maxLeaseTime, err := normalizePoolLeaseTimes(req.MinLeaseTime, req.MaxLeaseTime)
	if err != nil {
		return nil, err
	}
	parent, err := s.loadParent(ctx, scopeRef, tenantID, req.ParentID)
	if err != nil {
		return nil, err
	}
	if parent == nil && scopeName != scopeGlobal {
		return nil, ErrParentScope
	}
	plan, err := s.planPool(scopeName, req, parent)
	if err != nil {
		return nil, err
	}
	if s.quota != nil {
		if err := s.quota.EnsurePoolCapacity(ctx, tenantID); err != nil {
			return nil, err
		}
	}
	allocationMode := resolveAllocationMode(req.AllocationMode, parent)
	priorityWeight := resolvePriorityWeight(req.PriorityWeight, parent)
	status := normalizeStatus(req.Status)
	now := time.Now().UTC()
	pool := &models.AddressPool{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		Scope:          plan.scope,
		ParentID:       req.ParentID,
		Name:           req.Name,
		CIDR:           plan.cidr,
		Network:        plan.network,
		Netmask:        plan.netmask,
		RangeStart:     plan.rangeStart.String(),
		RangeEnd:       plan.rangeEnd.String(),
		Gateway:        req.Gateway,
		Option43:       strings.TrimSpace(req.Option43),
		DNS:            req.DNS,
		VLANID:         plan.vlanID,
		InterfaceID:    plan.interfaceID,
		SSID:           plan.ssid,
		Location:       plan.location,
		ReservePercent: plan.reservePercent,
		MinLeaseTime:   minLeaseTime,
		MaxLeaseTime:   maxLeaseTime,
		LeaseProfileID: req.LeaseProfileID,
		Tags:           req.Tags,
		Exclusions:     plan.exclusions,
		AllocationMode: allocationMode,
		PriorityWeight: priorityWeight,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.InsertPool(ctx, pool); err != nil {
		return nil, err
	}
	s.logger.Info("created address pool", zap.String("poolId", pool.ID), zap.String("tenantId", tenantID), zap.String("scope", plan.scope))
	return pool, nil
}

// UpdatePool mutates an existing pool.
func (s *Service) UpdatePool(ctx context.Context, scopeRef ResourceScope, req PoolUpdateRequest) (_ *models.AddressPool, err error) {
	done := s.trackPoolOperation(scopeRef, "update_pool")
	defer func() { done(err) }()
	tenantID, err := scopeRef.TenantIDOrErr()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.TenantID) == "" {
		req.TenantID = tenantID
	}
	if req.ParentID != nil && *req.ParentID == req.PoolID {
		return nil, ErrInvalidParent
	}
	pool, err := s.repo.GetPool(ctx, scopeRef.AccessScope(), req.PoolID)
	if err != nil {
		return nil, err
	}
	parentID := req.ParentID
	if parentID == nil {
		parentID = pool.ParentID
	}
	scope := req.Scope
	if scope == "" {
		scope = pool.Scope
	}
	if scope == "" {
		scope = scopeGlobal
	}
	name := req.Name
	if name == "" {
		name = pool.Name
	}
	cidr := req.CIDR
	if cidr == "" {
		cidr = pool.CIDR
	}
	network := req.Network
	if network == "" {
		network = pool.Network
	}
	netmask := req.Netmask
	if netmask == "" {
		netmask = pool.Netmask
	}
	// Allow clearing rangeStart/rangeEnd: empty values fall back to CIDR host range in planPool.
	rangeStart := req.RangeStart
	rangeEnd := req.RangeEnd
	gateway := req.Gateway
	if gateway == "" {
		gateway = pool.Gateway
	}
	option43 := strings.TrimSpace(req.Option43)
	if option43 == "" {
		option43 = pool.Option43
	}
	dns := pool.DNS
	if len(req.DNS) > 0 {
		dns = req.DNS
	}
	vlanID := req.VLANID
	if vlanID == nil {
		vlanID = pool.VLANID
	}
	interfaceID := req.InterfaceID
	if interfaceID == nil {
		interfaceID = pool.InterfaceID
	}
	ssid := req.SSID
	if ssid == nil {
		ssid = pool.SSID
	}
	location := req.Location
	if location == nil {
		location = pool.Location
	}
	leaseProfileID := req.LeaseProfileID
	if leaseProfileID == "" {
		leaseProfileID = pool.LeaseProfileID
	}
	minLeaseTime := req.MinLeaseTime
	if minLeaseTime == 0 {
		minLeaseTime = pool.MinLeaseTime
	}
	maxLeaseTime := req.MaxLeaseTime
	if maxLeaseTime == 0 {
		maxLeaseTime = pool.MaxLeaseTime
	}
	minLeaseTime, maxLeaseTime, err = normalizePoolLeaseTimes(minLeaseTime, maxLeaseTime)
	if err != nil {
		return nil, err
	}
	allocationMode := req.AllocationMode
	if strings.TrimSpace(allocationMode) == "" {
		allocationMode = pool.AllocationMode
	}
	priorityWeight := req.PriorityWeight
	if priorityWeight <= 0 {
		priorityWeight = pool.PriorityWeight
	}
	exclusions := pool.Exclusions
	if req.Exclusions != nil {
		if len(req.Exclusions) > 0 {
			exclusions = req.Exclusions
		}
	}
	reservePercent := req.ReservePercent
	if reservePercent == 0 {
		reservePercent = pool.ReservePercent
	}
	status := pool.Status
	if strings.TrimSpace(req.Status) != "" {
		status = req.Status
	}
	merged := PoolCreateRequest{
		TenantID:       tenantID,
		Scope:          scope,
		ParentID:       parentID,
		Name:           name,
		CIDR:           cidr,
		Network:        network,
		Netmask:        netmask,
		RangeStart:     rangeStart,
		RangeEnd:       rangeEnd,
		Gateway:        gateway,
		Option43:       option43,
		DNS:            dns,
		VLANID:         vlanID,
		InterfaceID:    interfaceID,
		SSID:           ssid,
		Location:       location,
		ReservePercent: reservePercent,
		MinLeaseTime:   minLeaseTime,
		MaxLeaseTime:   maxLeaseTime,
		LeaseProfileID: leaseProfileID,
		Tags:           req.Tags,
		Exclusions:     exclusions,
		AllocationMode: allocationMode,
		PriorityWeight: priorityWeight,
		Status:         status,
	}
	parent, err := s.loadParent(ctx, scopeRef, tenantID, merged.ParentID)
	if err != nil {
		return nil, err
	}
	if parent == nil && merged.Scope != scopeGlobal {
		return nil, ErrParentScope
	}
	plan, err := s.planPool(merged.Scope, merged, parent)
	if err != nil {
		return nil, err
	}
	mode := resolveAllocationMode(merged.AllocationMode, parent)
	weight := resolvePriorityWeight(merged.PriorityWeight, parent)
	status = normalizeStatus(status)
	pool.Scope = plan.scope
	pool.ParentID = merged.ParentID
	pool.Name = merged.Name
	pool.CIDR = plan.cidr
	pool.Network = plan.network
	pool.Netmask = plan.netmask
	pool.RangeStart = plan.rangeStart.String()
	pool.RangeEnd = plan.rangeEnd.String()
	pool.Gateway = merged.Gateway
	pool.Option43 = strings.TrimSpace(merged.Option43)
	pool.DNS = merged.DNS
	pool.VLANID = plan.vlanID
	pool.InterfaceID = plan.interfaceID
	pool.SSID = plan.ssid
	pool.Location = plan.location
	pool.ReservePercent = plan.reservePercent
	pool.MinLeaseTime = merged.MinLeaseTime
	pool.MaxLeaseTime = merged.MaxLeaseTime
	pool.LeaseProfileID = merged.LeaseProfileID
	pool.AllocationMode = mode
	pool.PriorityWeight = weight
	pool.Status = status
	pool.TenantID = tenantID
	if len(merged.Tags) > 0 {
		pool.Tags = merged.Tags
	}
	pool.Exclusions = plan.exclusions
	pool.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdatePool(ctx, pool); err != nil {
		return nil, err
	}
	return pool, nil
}

func normalizePoolLeaseTimes(minLeaseTime, maxLeaseTime int) (int, int, error) {
	if minLeaseTime <= 0 {
		minLeaseTime = 3600
	}
	if maxLeaseTime <= 0 {
		maxLeaseTime = 7200
	}
	if minLeaseTime < 300 || minLeaseTime > 604800 {
		return 0, 0, ErrLeaseTimeOutOfRange
	}
	if maxLeaseTime < 300 || maxLeaseTime > 604800 {
		return 0, 0, ErrLeaseTimeOutOfRange
	}
	if maxLeaseTime < minLeaseTime {
		return 0, 0, ErrLeaseTimeOrder
	}
	return minLeaseTime, maxLeaseTime, nil
}

// DeletePool removes a pool by ID.
func (s *Service) DeletePool(ctx context.Context, scope ResourceScope, poolID string) (err error) {
	done := s.trackPoolOperation(scope, "delete_pool")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return err
	}
	return s.repo.DeletePool(ctx, scope.AccessScope(), poolID)
}

// ListBindings lists static bindings with optional filter.
func (s *Service) ListBindings(ctx context.Context, scope ResourceScope, filter BindingFilter, limit, offset int) (_ []models.StaticBinding, err error) {
	done := s.trackPoolOperation(scope, "list_bindings")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.ListBindings(ctx, scope.AccessScope(), filter, limit, offset)
}

// CountBindings returns the total number of bindings matching the filter.
func (s *Service) CountBindings(ctx context.Context, scope ResourceScope, filter BindingFilter) (_ int, err error) {
	done := s.trackPoolOperation(scope, "count_bindings")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return 0, err
	}
	return s.repo.CountBindings(ctx, scope.AccessScope(), filter)
}

// CountBindingsByStatus returns counts grouped by status for the given filter.
func (s *Service) CountBindingsByStatus(ctx context.Context, scope ResourceScope, filter BindingFilter) (_ map[string]int, err error) {
	done := s.trackPoolOperation(scope, "count_bindings_status")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.CountBindingsByStatus(ctx, scope.AccessScope(), filter)
}

// ListBindingsByPool returns all bindings within a pool.
func (s *Service) ListBindingsByPool(ctx context.Context, scope ResourceScope, poolID string) (_ []models.StaticBinding, err error) {
	done := s.trackPoolOperation(scope, "list_bindings_pool")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	return s.repo.ListBindingsByPool(ctx, scope.AccessScope(), poolID)
}

// FindBinding resolves a static binding by identifier or IP.
func (s *Service) FindBinding(ctx context.Context, scope ResourceScope, identifier, ip string) (_ *models.StaticBinding, err error) {
	done := s.trackPoolOperation(scope, "find_binding")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(identifier) == "" && strings.TrimSpace(ip) == "" {
		return nil, ErrBindingNotFound
	}
	binding, err := s.repo.FindBinding(ctx, scope.AccessScope(), identifier, ip)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return binding, nil
}

// CreateBinding inserts a new static binding.
func (s *Service) CreateBinding(ctx context.Context, scope ResourceScope, req BindingCreateRequest) (_ *models.StaticBinding, err error) {
	done := s.trackPoolOperation(scope, "create_binding")
	defer func() { done(err) }()
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return nil, err
	}
	req.TenantID = tenantID
	if req.Identifier == "" || req.IdentifierType == "" {
		return nil, errors.New("identifier and type required")
	}
	req.IdentifierType = strings.ToLower(strings.TrimSpace(req.IdentifierType))
	if req.IdentifierType != "mac" {
		return nil, ErrBindingTypeUnsupported
	}
	ipAddr, err := netip.ParseAddr(strings.TrimSpace(req.IPAddress))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.PoolID) == "" {
		poolID, matchErr := s.resolvePoolByIP(ctx, scope, ipAddr)
		if matchErr != nil {
			return nil, matchErr
		}
		req.PoolID = poolID
	}
	if _, err := s.repo.GetPool(ctx, scope.AccessScope(), req.PoolID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	statusUpdatedAt := now
	statusSource := "manual"
	binding := &models.StaticBinding{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		Identifier:      req.Identifier,
		IdentifierType:  req.IdentifierType,
		PoolID:          req.PoolID,
		IPAddress:       req.IPAddress,
		LeaseProfileID:  req.LeaseProfileID,
		Metadata:        req.Metadata,
		Status:          "offline",
		StatusSource:    &statusSource,
		StatusUpdatedAt: &statusUpdatedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.InsertBinding(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

// UpdateBinding mutates metadata for an existing static binding.
func (s *Service) UpdateBinding(ctx context.Context, scope ResourceScope, bindingID string, req BindingCreateRequest) (_ *models.StaticBinding, err error) {
	done := s.trackPoolOperation(scope, "update_binding")
	defer func() { done(err) }()
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return nil, err
	}
	req.TenantID = tenantID
	if req.Identifier == "" || req.IdentifierType == "" {
		return nil, errors.New("identifier and type required")
	}
	req.IdentifierType = strings.ToLower(strings.TrimSpace(req.IdentifierType))
	if req.IdentifierType != "mac" {
		return nil, ErrBindingTypeUnsupported
	}
	binding, err := s.repo.GetBinding(ctx, scope.AccessScope(), bindingID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	ipAddr, err := netip.ParseAddr(strings.TrimSpace(req.IPAddress))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.PoolID) == "" {
		poolID, matchErr := s.resolvePoolByIP(ctx, scope, ipAddr)
		if matchErr != nil {
			return nil, matchErr
		}
		req.PoolID = poolID
	}
	if _, err := s.repo.GetPool(ctx, scope.AccessScope(), req.PoolID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	binding.Identifier = req.Identifier
	binding.IdentifierType = req.IdentifierType
	binding.PoolID = req.PoolID
	binding.IPAddress = req.IPAddress
	binding.LeaseProfileID = req.LeaseProfileID
	binding.Metadata = req.Metadata
	binding.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateBinding(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

// resolvePoolByIP finds the single pool whose CIDR contains the IP and whose exclusions do not block it.
func (s *Service) resolvePoolByIP(ctx context.Context, scope ResourceScope, ip netip.Addr) (string, error) {
	if s.repo == nil {
		return "", ErrPoolNotFound
	}
	if !ip.Is4() {
		return "", fmt.Errorf("%w: 仅支持 IPv4 地址匹配", ErrPoolNotFound)
	}
	const pageSize = 200
	var candidates []models.AddressPool
	for offset := 0; ; offset += pageSize {
		pools, err := s.repo.ListPoolsByFamily(ctx, scope.AccessScope(), 4, pageSize, offset)
		if err != nil {
			return "", err
		}
		if len(pools) == 0 {
			break
		}
		for _, pool := range pools {
			cidr := strings.TrimSpace(pool.CIDR)
			if cidr == "" {
				continue
			}
			prefix, perr := netip.ParsePrefix(cidr)
			if perr != nil {
				continue
			}
			if !prefix.Contains(ip) {
				continue
			}
			candidates = append(candidates, pool)
		}
		if len(pools) < pageSize {
			break
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("%w: IP %s 未找到匹配地址池", ErrPoolNotFound, ip)
	}
	if len(candidates) > 1 {
		ids := make([]string, len(candidates))
		for i, p := range candidates {
			ids[i] = p.ID
		}
		return "", fmt.Errorf("%w: IP %s 匹配多个地址池: %s", ErrPoolAmbiguous, ip, strings.Join(ids, ","))
	}
	return candidates[0].ID, nil
}

// ipInExclusions is retained for potential reuse; currently bindings ignore exclusions.
func ipInExclusions(ip netip.Addr, exclusions models.IPRangeList) bool {
	for _, rng := range exclusions {
		startStr := strings.TrimSpace(rng.Start)
		endStr := strings.TrimSpace(rng.End)
		if startStr == "" && endStr == "" {
			continue
		}
		start, err := netip.ParseAddr(startStr)
		if err != nil {
			continue
		}
		end := start
		if endStr != "" {
			parsed, perr := netip.ParseAddr(endStr)
			if perr != nil {
				continue
			}
			end = parsed
		}
		if start.Compare(ip) <= 0 && ip.Compare(end) <= 0 {
			return true
		}
	}
	return false
}

// RecordBindingSeen updates binding status when a device is observed.
func (s *Service) RecordBindingSeen(ctx context.Context, scope ResourceScope, identifier, ip, source string, seenAt time.Time) (err error) {
	done := s.trackPoolOperation(scope, "binding_seen")
	defer func() { done(err) }()
	if s.repo == nil {
		return ErrBindingNotFound
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return err
	}
	if strings.TrimSpace(identifier) == "" && strings.TrimSpace(ip) == "" {
		return nil
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	binding, err := s.repo.FindBinding(ctx, scope.AccessScope(), identifier, ip)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if binding == nil {
		return nil
	}
	status := "online"
	if err := s.repo.UpdateBindingStatus(ctx, scope.AccessScope(), binding.ID, status, source, seenAt); err != nil {
		return err
	}
	return nil
}

// DeleteBinding deletes a static binding.
func (s *Service) DeleteBinding(ctx context.Context, scope ResourceScope, bindingID string) (err error) {
	done := s.trackPoolOperation(scope, "delete_binding")
	defer func() { done(err) }()
	if _, err := scope.TenantIDOrErr(); err != nil {
		return err
	}
	return s.repo.DeleteBinding(ctx, scope.AccessScope(), bindingID)
}

func (s *Service) InvalidateTenantCache(ctx context.Context, tenantID string) {
	if s == nil || s.repo == nil {
		return
	}
	if strings.TrimSpace(tenantID) == "" {
		return
	}
	s.repo.EvictTenantCache(ctx, tenantID)
}

func (s *Service) InvalidateAllCache(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	s.repo.EvictAllCache(ctx)
}

type poolPlan struct {
	scope          string
	prefix         netip.Prefix
	cidr           string
	network        string
	netmask        string
	rangeStart     netip.Addr
	rangeEnd       netip.Addr
	reservePercent int
	exclusions     models.IPRangeList
	vlanID         *int
	interfaceID    *string
	ssid           *string
	location       *string
}

type metadataPlan struct {
	vlanID      *int
	interfaceID *string
	ssid        *string
	location    *string
}

func (s *Service) loadParent(ctx context.Context, scope ResourceScope, tenantID string, parentID *string) (*models.AddressPool, error) {
	if parentID == nil || *parentID == "" {
		return nil, nil
	}
	parent, err := s.repo.GetPool(ctx, scope.AccessScope(), *parentID)
	if err != nil {
		return nil, err
	}
	normalizedScope, err := normalizeScope(parent.Scope)
	if err != nil {
		return nil, err
	}
	parent.Scope = normalizedScope
	return parent, nil
}

func normalizeScope(scope string) (string, error) {
	value := strings.TrimSpace(strings.ToUpper(scope))
	if value == "" {
		value = scopeGlobal
	}
	if _, ok := scopeOrder[value]; !ok {
		return "", ErrInvalidScope
	}
	return value, nil
}

func resolveAllocationMode(value string, parent *models.AddressPool) string {
	candidate := strings.TrimSpace(strings.ToUpper(value))
	if candidate == "" && parent != nil {
		candidate = strings.TrimSpace(strings.ToUpper(parent.AllocationMode))
	}
	switch candidate {
	case models.AllocationModeRoundRobin, models.AllocationModePriorityWeighted:
		return candidate
	default:
		return models.AllocationModeSequential
	}
}

func resolvePriorityWeight(value int, parent *models.AddressPool) int {
	if value <= 0 {
		if parent != nil && parent.PriorityWeight > 0 {
			value = parent.PriorityWeight
		} else {
			value = 50
		}
	}
	if value > 100 {
		value = 100
	}
	return value
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "disabled":
		return "disabled"
	case "warning":
		return "warning"
	default:
		return "active"
	}
}

func (s *Service) planPool(scope string, req PoolCreateRequest, parent *models.AddressPool) (*poolPlan, error) {
	prefix, cidr, network, netmask, err := resolvePrefixInputs(req.CIDR, req.Network, req.Netmask)
	if err != nil {
		return nil, err
	}
	if err := validateScopeTransition(scope, parent); err != nil {
		return nil, err
	}
	reserve, err := resolveReserve(req.ReservePercent, parent)
	if err != nil {
		return nil, err
	}
	start, end, err := resolveRangeBounds(prefix, req.RangeStart, req.RangeEnd)
	if err != nil {
		return nil, err
	}
	exclusions, err := normalizeExclusions(prefix, start, end, req.Exclusions)
	if err != nil {
		return nil, err
	}
	meta, err := s.resolveMetadata(scope, req, parent)
	if err != nil {
		return nil, err
	}
	if parent != nil {
		parentPrefix, err := netip.ParsePrefix(parent.CIDR)
		if err != nil {
			return nil, err
		}
		childMax, err := netutil.MaxAddress(prefix)
		if err != nil {
			return nil, err
		}
		if !parentPrefix.Contains(prefix.Masked().Addr()) || !parentPrefix.Contains(childMax) {
			return nil, ErrInvalidRange
		}
		parentStart, parentEnd, err := resolveRangeBounds(parentPrefix, parent.RangeStart, parent.RangeEnd)
		if err != nil {
			return nil, err
		}
		if start.Compare(parentStart) < 0 || end.Compare(parentEnd) > 0 {
			return nil, ErrRangeOutside
		}
		if !parentPrefix.Contains(start) || !parentPrefix.Contains(end) {
			return nil, ErrInvalidRange
		}
	}
	return &poolPlan{
		scope:          scope,
		prefix:         prefix,
		cidr:           cidr,
		network:        network,
		netmask:        netmask,
		rangeStart:     start,
		rangeEnd:       end,
		reservePercent: reserve,
		exclusions:     exclusions,
		vlanID:         meta.vlanID,
		interfaceID:    meta.interfaceID,
		ssid:           meta.ssid,
		location:       meta.location,
	}, nil
}

func (s *Service) resolveMetadata(scope string, req PoolCreateRequest, parent *models.AddressPool) (*metadataPlan, error) {
	vlanID, err := normalizeVLAN(req.VLANID)
	if err != nil {
		return nil, err
	}
	userProvidedVLAN := vlanID != nil
	meta := &metadataPlan{
		vlanID:      vlanID,
		interfaceID: normalizeStringPtr(req.InterfaceID),
		ssid:        normalizeStringPtr(req.SSID),
		location:    normalizeStringPtr(req.Location),
	}
	if meta.location == nil && parent != nil && parent.Location != nil {
		meta.location = normalizeStringPtr(parent.Location)
	}
	inheritVLAN := func() (*int, error) {
		if parent != nil && parent.VLANID != nil {
			inherited, err := normalizeVLAN(parent.VLANID)
			if err != nil {
				return nil, err
			}
			return inherited, nil
		}
		return nil, ErrVLANRequired
	}
	switch scope {
	case scopeGlobal:
		// no-op
	case scopeSubnet:
		// allow optional location overrides, but no additional requirements
	case scopeVLAN:
		if !userProvidedVLAN {
			return nil, ErrVLANRequired
		}
	case scopePort:
		inherited, err := inheritVLAN()
		if err != nil {
			return nil, err
		}
		if userProvidedVLAN && *meta.vlanID != *inherited {
			return nil, ErrParentVLANMismatch
		}
		meta.vlanID = inherited
		if meta.interfaceID == nil && meta.ssid == nil && meta.location == nil {
			return nil, ErrEndpointMetadata
		}
	default:
		return nil, ErrInvalidScope
	}
	return meta, nil
}

func normalizeVLAN(value *int) (*int, error) {
	if value == nil {
		return nil, nil
	}
	v := *value
	if v <= 0 || v > 4094 {
		return nil, ErrInvalidVLAN
	}
	copy := v
	return &copy, nil
}

func normalizeStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	copy := trimmed
	return &copy
}

func resolvePrefixInputs(cidr, network, netmask string) (netip.Prefix, string, string, string, error) {
	if strings.TrimSpace(cidr) != "" {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err != nil {
			return netip.Prefix{}, "", "", "", ErrInvalidCIDR
		}
		prefix = prefix.Masked()
		return prefix, prefix.String(), prefix.Masked().Addr().String(), formatMaskString(prefix.Bits(), prefix.Addr().Is4()), nil
	}
	network = strings.TrimSpace(network)
	netmask = strings.TrimSpace(netmask)
	if network == "" || netmask == "" {
		return netip.Prefix{}, "", "", "", ErrInvalidCIDR
	}
	addr, err := netip.ParseAddr(network)
	if err != nil {
		return netip.Prefix{}, "", "", "", ErrInvalidCIDR
	}
	maskBits, maskStr, err := parseMaskBits(netmask, addr.Is4())
	if err != nil {
		return netip.Prefix{}, "", "", "", err
	}
	prefix := netip.PrefixFrom(addr, maskBits).Masked()
	return prefix, prefix.String(), prefix.Masked().Addr().String(), maskStr, nil
}

func parseMaskBits(mask string, ipv4 bool) (int, string, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(mask, "/"))
	if trimmed == "" {
		return 0, "", ErrInvalidCIDR
	}
	if bits, err := strconv.Atoi(trimmed); err == nil {
		maxBits := 128
		if ipv4 {
			maxBits = 32
		}
		if bits < 0 || bits > maxBits {
			return 0, "", ErrInvalidCIDR
		}
		return bits, formatMaskString(bits, ipv4), nil
	}
	if ipv4 {
		ip := net.ParseIP(strings.TrimSpace(mask))
		if ip == nil {
			return 0, "", ErrInvalidCIDR
		}
		v4 := ip.To4()
		if v4 == nil {
			return 0, "", ErrInvalidCIDR
		}
		ones, bits := net.IPMask(v4).Size()
		if bits != 32 {
			return 0, "", ErrInvalidCIDR
		}
		return ones, fmt.Sprintf("%d.%d.%d.%d", v4[0], v4[1], v4[2], v4[3]), nil
	}
	return 0, "", ErrInvalidCIDR
}

func formatMaskString(bits int, ipv4 bool) string {
	if !ipv4 {
		return fmt.Sprintf("/%d", bits)
	}
	mask := net.CIDRMask(bits, 32)
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

func resolveRangeBounds(prefix netip.Prefix, startStr, endStr string) (netip.Addr, netip.Addr, error) {
	start, end, err := netutil.DefaultHostRange(prefix)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	if strings.TrimSpace(startStr) != "" {
		addr, err := parseRangeAddr(strings.TrimSpace(startStr), prefix.Addr().Is4())
		if err != nil {
			return netip.Addr{}, netip.Addr{}, err
		}
		if !prefix.Contains(addr) {
			return netip.Addr{}, netip.Addr{}, ErrInvalidRange
		}
		start = addr
	}
	if strings.TrimSpace(endStr) != "" {
		addr, err := parseRangeAddr(strings.TrimSpace(endStr), prefix.Addr().Is4())
		if err != nil {
			return netip.Addr{}, netip.Addr{}, err
		}
		if !prefix.Contains(addr) {
			return netip.Addr{}, netip.Addr{}, ErrInvalidRange
		}
		end = addr
	}
	if start.Compare(end) > 0 {
		return netip.Addr{}, netip.Addr{}, ErrInvalidRange
	}
	return start, end, nil
}

func parseRangeAddr(value string, expectIPv4 bool) (netip.Addr, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return netip.Addr{}, ErrInvalidRange
	}
	if expectIPv4 && !addr.Is4() {
		return netip.Addr{}, ErrInvalidRange
	}
	if !expectIPv4 && addr.Is4() {
		return netip.Addr{}, ErrInvalidRange
	}
	return addr, nil
}

func normalizeExclusions(prefix netip.Prefix, start, end netip.Addr, ranges models.IPRangeList) (models.IPRangeList, error) {
	if len(ranges) == 0 {
		return nil, nil
	}
	parsed := make([]struct {
		start netip.Addr
		end   netip.Addr
	}, 0, len(ranges))
	expectIPv4 := prefix.Addr().Is4()
	for _, r := range ranges {
		if strings.TrimSpace(r.Start) == "" || strings.TrimSpace(r.End) == "" {
			return nil, ErrExclusionInvalid
		}
		sAddr, err := parseRangeAddr(r.Start, expectIPv4)
		if err != nil {
			return nil, ErrExclusionInvalid
		}
		eAddr, err := parseRangeAddr(r.End, expectIPv4)
		if err != nil {
			return nil, ErrExclusionInvalid
		}
		if sAddr.Compare(eAddr) > 0 {
			return nil, ErrExclusionInvalid
		}
		if sAddr.Compare(start) < 0 || eAddr.Compare(end) > 0 {
			return nil, ErrExclusionInvalid
		}
		if !prefix.Contains(sAddr) || !prefix.Contains(eAddr) {
			return nil, ErrExclusionInvalid
		}
		parsed = append(parsed, struct {
			start netip.Addr
			end   netip.Addr
		}{start: sAddr, end: eAddr})
	}
	sort.Slice(parsed, func(i, j int) bool {
		cmp := parsed[i].start.Compare(parsed[j].start)
		if cmp == 0 {
			return parsed[i].end.Compare(parsed[j].end) < 0
		}
		return cmp < 0
	})
	for i := 1; i < len(parsed); i++ {
		if parsed[i].start.Compare(parsed[i-1].end) <= 0 {
			return nil, ErrExclusionOverlap
		}
	}
	out := make(models.IPRangeList, len(parsed))
	for i, pr := range parsed {
		out[i] = models.IPRange{Start: pr.start.String(), End: pr.end.String()}
	}
	return out, nil
}

func validateScopeTransition(scope string, parent *models.AddressPool) error {
	childOrder, ok := scopeOrder[scope]
	if !ok {
		return ErrInvalidScope
	}
	if parent == nil {
		if scope != scopeGlobal {
			return ErrParentScope
		}
		return nil
	}
	parentScope, _ := normalizeScope(parent.Scope)
	parentOrder := scopeOrder[parentScope]
	if childOrder != parentOrder+1 {
		return ErrParentScope
	}
	return nil
}

func resolveReserve(requested int, parent *models.AddressPool) (int, error) {
	value := requested
	if value == 0 {
		if parent != nil && parent.ReservePercent > 0 {
			value = parent.ReservePercent
		} else {
			value = 10
		}
	}
	if value < 0 || value > 50 {
		return 0, ErrReserveOutOfRange
	}
	return value, nil
}

func normalizeFilter(filter MetadataFilter) MetadataFilter {
	filter.Scope = strings.TrimSpace(strings.ToUpper(filter.Scope))
	if filter.ParentID != nil {
		trimmed := strings.TrimSpace(*filter.ParentID)
		if trimmed == "" {
			filter.ParentID = nil
		} else {
			filter.ParentID = &trimmed
		}
	}
	if filter.InterfaceID != nil {
		trimmed := strings.TrimSpace(*filter.InterfaceID)
		if trimmed == "" {
			filter.InterfaceID = nil
		} else {
			filter.InterfaceID = &trimmed
		}
	}
	if filter.SSID != nil {
		trimmed := strings.TrimSpace(*filter.SSID)
		if trimmed == "" {
			filter.SSID = nil
		} else {
			filter.SSID = &trimmed
		}
	}
	if filter.Location != nil {
		trimmed := strings.TrimSpace(*filter.Location)
		if trimmed == "" {
			filter.Location = nil
		} else {
			filter.Location = &trimmed
		}
	}
	if filter.GeoCode != nil {
		trimmed := strings.TrimSpace(*filter.GeoCode)
		if trimmed == "" {
			filter.GeoCode = nil
		} else {
			filter.GeoCode = &trimmed
		}
	}
	if filter.DeviceProfile != nil {
		trimmed := strings.TrimSpace(*filter.DeviceProfile)
		if trimmed == "" {
			filter.DeviceProfile = nil
		} else {
			filter.DeviceProfile = &trimmed
		}
	}
	if filter.TagFingerprint != nil {
		trimmed := strings.TrimSpace(*filter.TagFingerprint)
		if trimmed == "" {
			filter.TagFingerprint = nil
		} else {
			filter.TagFingerprint = &trimmed
		}
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 50
	}
	return filter
}

func normalizeSelector(sel MetadataSelector) MetadataSelector {
	sel.InterfaceID = strings.TrimSpace(sel.InterfaceID)
	sel.SSID = strings.TrimSpace(sel.SSID)
	sel.Location = strings.TrimSpace(sel.Location)
	sel.GeoCode = strings.TrimSpace(sel.GeoCode)
	sel.DeviceProfile = strings.TrimSpace(sel.DeviceProfile)
	sel.TagFingerprint = strings.TrimSpace(sel.TagFingerprint)
	sel.AccessPointID = strings.TrimSpace(sel.AccessPointID)
	sel.ControllerID = strings.TrimSpace(sel.ControllerID)
	sel.GeoZone = strings.TrimSpace(sel.GeoZone)
	if sel.VLANID < 0 {
		sel.VLANID = 0
	}
	return sel
}

func (s *Service) trackPoolOperation(scope ResourceScope, operation string) func(error) {
	if s == nil || s.metrics == nil || s.metrics.PoolServiceLatency == nil {
		return func(error) {}
	}
	tenant := sanitizeTenantLabel(scope.TenantOrDefault())
	started := time.Now()
	return func(err error) {
		outcome := metricOutcomeSuccess
		if err != nil {
			outcome = metricOutcomeError
		}
		s.metrics.PoolServiceLatency.WithLabelValues(tenant, operation, outcome).Observe(time.Since(started).Seconds())
	}
}

func sanitizeTenantLabel(tenant string) string {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return metricLabelUnknown
	}
	return tenant
}
