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

	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/pkg/models"
)

var (
	ErrInvalidCIDR          = errors.New("invalid cidr")
	ErrReserveOutOfRange    = errors.New("reserve percent out of range")
	ErrLeaseProfileRequired = errors.New("lease profile id required")
	ErrInvalidScope         = errors.New("invalid pool scope")
	ErrParentScope          = errors.New("parent scope incompatible")
	ErrInvalidRange         = errors.New("invalid allocation range")
	ErrRangeOutside         = errors.New("allocation range outside parent bounds")
	ErrExclusionInvalid     = errors.New("invalid exclusion range")
	ErrExclusionOverlap     = errors.New("exclusion ranges overlap")
	ErrInvalidParent        = errors.New("invalid parent reference")
	ErrVLANRequired         = errors.New("vlan id required for scope")
	ErrEndpointMetadata     = errors.New("port scope requires interface, ssid, or location")
	ErrInvalidVLAN          = errors.New("invalid vlan id")
	ErrParentVLANMismatch   = errors.New("vlan id must match parent")
	ErrPoolNotFound         = errors.New("matching pool not found")
	ErrBindingNotFound      = errors.New("binding not found")
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

// Service exposes business logic for pools and bindings.
type Service struct {
	repo   Repository
	logger *zap.Logger
	quota  tenant.QuotaEnforcer
}

// ServiceOption configures optional dependencies for the pool service.
type ServiceOption func(*Service)

// WithQuotaEnforcer wires tenant quota checks into pool mutations.
func WithQuotaEnforcer(enforcer tenant.QuotaEnforcer) ServiceOption {
	return func(s *Service) {
		s.quota = enforcer
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
func (s *Service) GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error) {
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	return s.repo.GetPool(ctx, tenantID, poolID)
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
	VLANID         *int
	InterfaceID    *string
	SSID           *string
	Location       *string
	ReservePercent int
	LeaseProfileID string
	Tags           []byte
	Exclusions     models.IPRangeList
	AllocationMode string
	PriorityWeight int
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

// ListPools returns paginated pools for the tenant.
func (s *Service) ListPools(ctx context.Context, tenantID string, limit, offset int) ([]models.AddressPool, error) {
	return s.repo.ListPools(ctx, tenantID, limit, offset)
}

// FindPools filters pools by metadata fields.
func (s *Service) FindPools(ctx context.Context, tenantID string, filter MetadataFilter) ([]models.AddressPool, error) {
	return s.repo.FindPools(ctx, tenantID, normalizeFilter(filter))
}

// ResolvePool attempts to locate the most specific pool for the selector metadata.
func (s *Service) ResolvePool(ctx context.Context, tenantID string, selector MetadataSelector) (*models.AddressPool, error) {
	if s.repo == nil {
		return nil, ErrPoolNotFound
	}
	normalized := normalizeSelector(selector)
	for _, iface := range []string{normalized.InterfaceID, normalized.AccessPointID} {
		if iface == "" {
			continue
		}
		pools, err := s.repo.FindPools(ctx, tenantID, MetadataFilter{InterfaceID: strPtr(iface), Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(pools) > 0 {
			return &pools[0], nil
		}
	}
	if normalized.SSID != "" {
		pools, err := s.repo.FindPools(ctx, tenantID, MetadataFilter{SSID: strPtr(normalized.SSID), Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(pools) > 0 {
			return &pools[0], nil
		}
	}
	for _, loc := range []string{normalized.Location, normalized.ControllerID, normalized.GeoZone} {
		if strings.TrimSpace(loc) == "" {
			continue
		}
		pools, err := s.repo.FindPools(ctx, tenantID, MetadataFilter{Location: strPtr(loc), Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(pools) > 0 {
			return &pools[0], nil
		}
	}
	if normalized.VLANID > 0 {
		pools, err := s.repo.FindPools(ctx, tenantID, MetadataFilter{VLANID: intPtr(normalized.VLANID), Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(pools) > 0 {
			return &pools[0], nil
		}
	}
	return nil, ErrPoolNotFound
}

// CreatePool validates and persists a new pool.
func (s *Service) CreatePool(ctx context.Context, req PoolCreateRequest) (*models.AddressPool, error) {
	scope, err := normalizeScope(req.Scope)
	if err != nil {
		return nil, err
	}
	if req.LeaseProfileID == "" {
		return nil, ErrLeaseProfileRequired
	}
	parent, err := s.loadParent(ctx, req.TenantID, req.ParentID)
	if err != nil {
		return nil, err
	}
	if parent == nil && scope != scopeGlobal {
		return nil, ErrParentScope
	}
	plan, err := s.planPool(scope, req, parent)
	if err != nil {
		return nil, err
	}
	if s.quota != nil {
		if err := s.quota.EnsurePoolCapacity(ctx, req.TenantID); err != nil {
			return nil, err
		}
	}
	allocationMode := resolveAllocationMode(req.AllocationMode, parent)
	priorityWeight := resolvePriorityWeight(req.PriorityWeight, parent)
	now := time.Now().UTC()
	pool := &models.AddressPool{
		ID:             uuid.NewString(),
		TenantID:       req.TenantID,
		Scope:          plan.scope,
		ParentID:       req.ParentID,
		Name:           req.Name,
		CIDR:           plan.cidr,
		Network:        plan.network,
		Netmask:        plan.netmask,
		RangeStart:     plan.rangeStart.String(),
		RangeEnd:       plan.rangeEnd.String(),
		VLANID:         plan.vlanID,
		InterfaceID:    plan.interfaceID,
		SSID:           plan.ssid,
		Location:       plan.location,
		ReservePercent: plan.reservePercent,
		LeaseProfileID: req.LeaseProfileID,
		Tags:           req.Tags,
		Exclusions:     plan.exclusions,
		AllocationMode: allocationMode,
		PriorityWeight: priorityWeight,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.InsertPool(ctx, pool); err != nil {
		return nil, err
	}
	s.logger.Info("created address pool", zap.String("poolId", pool.ID), zap.String("tenantId", pool.TenantID), zap.String("scope", plan.scope))
	return pool, nil
}

// UpdatePool mutates an existing pool.
func (s *Service) UpdatePool(ctx context.Context, req PoolUpdateRequest) (*models.AddressPool, error) {
	if req.ParentID != nil && *req.ParentID == req.PoolID {
		return nil, ErrInvalidParent
	}
	pool, err := s.repo.GetPool(ctx, req.TenantID, req.PoolID)
	if err != nil {
		return nil, err
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
	rangeStart := req.RangeStart
	if rangeStart == "" {
		rangeStart = pool.RangeStart
	}
	rangeEnd := req.RangeEnd
	if rangeEnd == "" {
		rangeEnd = pool.RangeEnd
	}
	leaseProfileID := req.LeaseProfileID
	if leaseProfileID == "" {
		leaseProfileID = pool.LeaseProfileID
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
		exclusions = req.Exclusions
	}
	merged := PoolCreateRequest{
		TenantID:       req.TenantID,
		Scope:          scope,
		ParentID:       req.ParentID,
		Name:           name,
		CIDR:           cidr,
		Network:        network,
		Netmask:        netmask,
		RangeStart:     rangeStart,
		RangeEnd:       rangeEnd,
		VLANID:         req.VLANID,
		InterfaceID:    req.InterfaceID,
		SSID:           req.SSID,
		Location:       req.Location,
		ReservePercent: req.ReservePercent,
		LeaseProfileID: leaseProfileID,
		Tags:           req.Tags,
		Exclusions:     exclusions,
		AllocationMode: allocationMode,
		PriorityWeight: priorityWeight,
	}
	parent, err := s.loadParent(ctx, req.TenantID, merged.ParentID)
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
	pool.Scope = plan.scope
	pool.ParentID = merged.ParentID
	pool.Name = merged.Name
	pool.CIDR = plan.cidr
	pool.Network = plan.network
	pool.Netmask = plan.netmask
	pool.RangeStart = plan.rangeStart.String()
	pool.RangeEnd = plan.rangeEnd.String()
	pool.VLANID = plan.vlanID
	pool.InterfaceID = plan.interfaceID
	pool.SSID = plan.ssid
	pool.Location = plan.location
	pool.ReservePercent = plan.reservePercent
	pool.LeaseProfileID = merged.LeaseProfileID
	pool.AllocationMode = mode
	pool.PriorityWeight = weight
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

// DeletePool removes a pool by ID.
func (s *Service) DeletePool(ctx context.Context, tenantID, poolID string) error {
	return s.repo.DeletePool(ctx, tenantID, poolID)
}

// ListBindings lists static bindings.
func (s *Service) ListBindings(ctx context.Context, tenantID string, limit, offset int) ([]models.StaticBinding, error) {
	return s.repo.ListBindings(ctx, tenantID, limit, offset)
}

// CreateBinding inserts a new static binding.
func (s *Service) CreateBinding(ctx context.Context, req BindingCreateRequest) (*models.StaticBinding, error) {
	if req.Identifier == "" || req.IdentifierType == "" {
		return nil, errors.New("identifier and type required")
	}
	if _, err := s.repo.GetPool(ctx, req.TenantID, req.PoolID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	if _, err := netip.ParseAddr(req.IPAddress); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	binding := &models.StaticBinding{
		ID:             uuid.NewString(),
		TenantID:       req.TenantID,
		Identifier:     req.Identifier,
		IdentifierType: req.IdentifierType,
		PoolID:         req.PoolID,
		IPAddress:      req.IPAddress,
		LeaseProfileID: req.LeaseProfileID,
		Metadata:       req.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.InsertBinding(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

// UpdateBinding mutates metadata for an existing static binding.
func (s *Service) UpdateBinding(ctx context.Context, bindingID string, req BindingCreateRequest) (*models.StaticBinding, error) {
	if req.Identifier == "" || req.IdentifierType == "" {
		return nil, errors.New("identifier and type required")
	}
	binding, err := s.repo.GetBinding(ctx, req.TenantID, bindingID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	if _, err := s.repo.GetPool(ctx, req.TenantID, req.PoolID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, err
	}
	if _, err := netip.ParseAddr(req.IPAddress); err != nil {
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

// DeleteBinding deletes a static binding.
func (s *Service) DeleteBinding(ctx context.Context, tenantID, bindingID string) error {
	return s.repo.DeleteBinding(ctx, tenantID, bindingID)
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

func (s *Service) loadParent(ctx context.Context, tenantID string, parentID *string) (*models.AddressPool, error) {
	if parentID == nil || *parentID == "" {
		return nil, nil
	}
	parent, err := s.repo.GetPool(ctx, tenantID, *parentID)
	if err != nil {
		return nil, err
	}
	scope, err := normalizeScope(parent.Scope)
	if err != nil {
		return nil, err
	}
	parent.Scope = scope
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
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 50
	}
	return filter
}

func normalizeSelector(sel MetadataSelector) MetadataSelector {
	sel.InterfaceID = strings.TrimSpace(sel.InterfaceID)
	sel.SSID = strings.TrimSpace(sel.SSID)
	sel.Location = strings.TrimSpace(sel.Location)
	sel.AccessPointID = strings.TrimSpace(sel.AccessPointID)
	sel.ControllerID = strings.TrimSpace(sel.ControllerID)
	sel.GeoZone = strings.TrimSpace(sel.GeoZone)
	if sel.VLANID < 0 {
		sel.VLANID = 0
	}
	return sel
}

func strPtr(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func intPtr(value int) *int {
	copy := value
	return &copy
}
