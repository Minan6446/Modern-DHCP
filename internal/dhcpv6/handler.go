package dhcpv6

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	leasefsm "modern-dhcp/internal/dhcpv6/leasefsm"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/mobility"
	"modern-dhcp/internal/mobility/mdm"
	"modern-dhcp/internal/observability"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/relay"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

type staticPrefixDecision struct {
	Prefix string
	Length byte
	Reason string
}

// NACKError indicates the client request should be rejected with a negative ACK semantic.
type NACKError struct {
	Reason string
}

func (e *NACKError) Error() string {
	if e == nil {
		return "dhcpv6 nack"
	}
	if strings.TrimSpace(e.Reason) == "" {
		return "dhcpv6 nack"
	}
	return e.Reason
}

type leaseService interface {
	AllocateOrReuseWithMetadata(ctx context.Context, scope lease.ResourceScope, identifier string, profile models.LeaseProfile, poolID, ip string, meta lease.AllocationMetadata) (*lease.Result, error)
	AllocateOrReusePrefixes(ctx context.Context, scope lease.ResourceScope, clientID string, profile models.LeaseProfile, poolRef *models.AddressPool, requests []lease.PrefixRequest) ([]lease.PrefixDelegation, error)
	ReleasePrefix(ctx context.Context, scope lease.ResourceScope, clientID string, iapdID uint32) error
	DeclinePrefix(ctx context.Context, scope lease.ResourceScope, clientID string, iapdID uint32) error
	ListPrefixLeases(ctx context.Context, scope lease.ResourceScope, state string, limit, offset int) ([]models.PrefixLease, error)
	FindIPv6EUI64BindingByMAC(ctx context.Context, scope lease.ResourceScope, mac, prefix string) (*lease.IPv6EUI64Binding, error)
	FindIPv6EUI64BindingByIPv6Addr(ctx context.Context, scope lease.ResourceScope, prefix, ipv6Addr string) (*lease.IPv6EUI64Binding, error)
	CreateIPv6EUI64Binding(ctx context.Context, scope lease.ResourceScope, binding lease.IPv6EUI64Binding) (*lease.IPv6EUI64Binding, error)
}

type poolService interface {
	ResolvePool(ctx context.Context, scope pool.ResourceScope, selector pool.MetadataSelector) (*models.AddressPool, error)
	GetPool(ctx context.Context, scope pool.ResourceScope, poolID string) (*models.AddressPool, error)
	GetLeaseProfile(ctx context.Context, scope pool.ResourceScope, profileID string) (*models.LeaseProfile, error)
	FindBinding(ctx context.Context, scope pool.ResourceScope, identifier, ip string) (*models.StaticBinding, error)
}

// Handler handles DHCPv6 flows.
type Handler struct {
	leaseSvc         leaseService
	policy           *policy.Engine
	poolSvc          poolService
	metrics          *metrics.Collector
	logger           *zap.Logger
	relayAuth        *relay.Authenticator
	relaySel         *relay.Selector
	fingerprinter    *mobility.Detector
	mdmService       *mdm.Service
	staticPDBindings map[string]staticPrefixDecision
	eui64Cache       map[string]lease.IPv6EUI64Binding
	eui64CacheMu     sync.RWMutex
	ipv6ProbeTimeout time.Duration
	ipv6ConflictFn   func(ctx context.Context, targetIP string, timeout time.Duration) (bool, error)
}

// NewHandler builds a DHCPv6 handler.
func NewHandler(leaseSvc *lease.Service, policyEngine *policy.Engine, poolSvc *pool.Service, metricsCollector *metrics.Collector, authenticator *relay.Authenticator, selector *relay.Selector, logger *zap.Logger, fingerprinter *mobility.Detector, mdmSvc *mdm.Service) *Handler {
	return &Handler{
		leaseSvc:         leaseSvc,
		policy:           policyEngine,
		poolSvc:          poolSvc,
		metrics:          metricsCollector,
		relayAuth:        authenticator,
		relaySel:         selector,
		logger:           logger,
		fingerprinter:    fingerprinter,
		mdmService:       mdmSvc,
		staticPDBindings: make(map[string]staticPrefixDecision),
		eui64Cache:       make(map[string]lease.IPv6EUI64Binding),
		ipv6ProbeTimeout: time.Second,
		ipv6ConflictFn:   probeIPv6Conflict,
	}
}

// SetStaticPDBindings loads static DUID->prefix mappings (for example, from viper-backed config).
func (h *Handler) SetStaticPDBindings(bindings map[string]staticPrefixDecision) {
	if h == nil {
		return
	}
	cloned := make(map[string]staticPrefixDecision, len(bindings))
	for k, v := range bindings {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		cloned[key] = v
	}
	h.staticPDBindings = cloned
}

// SetStaticPDBindingsFromConfig configures static mappings from DUID to prefix strings.
func (h *Handler) SetStaticPDBindingsFromConfig(bindings map[string]string) {
	converted := make(map[string]staticPrefixDecision, len(bindings))
	for duid, prefix := range bindings {
		trimmedPrefix := strings.TrimSpace(prefix)
		if trimmedPrefix == "" {
			continue
		}
		parsed, err := netip.ParsePrefix(trimmedPrefix)
		if err != nil {
			continue
		}
		converted[duid] = staticPrefixDecision{Prefix: parsed.Masked().String(), Length: byte(parsed.Bits()), Reason: "static-config"}
	}
	h.SetStaticPDBindings(converted)
}

// SetIPv6ConflictProbeTimeout updates ICMPv6 conflict detection timeout.
func (h *Handler) SetIPv6ConflictProbeTimeout(timeout time.Duration) {
	if h == nil {
		return
	}
	if timeout <= 0 {
		h.ipv6ProbeTimeout = time.Second
		return
	}
	h.ipv6ProbeTimeout = timeout
}

// HandleSolicit runs allocation prechecks for SOLICIT and returns an offer candidate.
func (h *Handler) HandleSolicit(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
	return h.HandleRequest(ctx, tenantID, pkt)
}

// HandleRequest allocates addresses or prefixes.
func (h *Handler) HandleRequest(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	vlanID := parseRelayVLAN(pkt.RelayInfo)
	userGroups := policy.DeriveUserGroups(pkt.UserClass, pkt.RelayInfo)
	location := policy.DeriveLocation(pkt.RelayInfo)
	ssid := policy.DeriveSSID(pkt.RelayInfo)
	userID := policy.DeriveUserID(pkt.RelayInfo)
	deviceType, devicePersona, deviceTags := h.deviceClassification(pkt)
	apID := policy.DeriveAccessPointID(pkt.RelayInfo)
	controllerID := policy.DeriveControllerID(pkt.RelayInfo)
	geoZone := policy.DeriveGeoZone(pkt.RelayInfo)
	meta := relay.BuildMetadata(tenantID, pkt.LinkAddr, pkt.RelayInfo, vlanID, userGroups)
	if err := h.authorizeRelay(tenantID, meta); err != nil {
		return nil, err
	}
	relayInfo := relayPolicyAttributes(pkt)
	compliance := h.lookupComplianceMetadata(tenantID, pkt)
	var decision *policy.Decision
	if h.policy != nil {
		evaluated, evalErr := h.policy.Evaluate(ctx, policy.Input{
			TenantID:      tenantID,
			ClientID:      pkt.DUID,
			UserID:        userID,
			VendorClass:   pkt.VendorClass,
			UserClass:     pkt.UserClass,
			DeviceType:    deviceType,
			DevicePersona: devicePersona,
			DeviceTags:    deviceTags,
			MDMManaged:    compliance.Valid && compliance.Managed,
			UserGroups:    userGroups,
			Location:      location,
			SSID:          ssid,
			VLANID:        vlanID,
			AccessPointID: apID,
			ControllerID:  controllerID,
			GeoZone:       geoZone,
			Option82:      pkt.RelayInfo,
			RelayInfo:     relayInfo,
			SupportsSLAAC: pkt.SupportsSLAAC,
			Timestamp:     time.Now().UTC(),
		})
		if evalErr != nil && h.logger != nil {
			h.logger.Warn("no policy match", zap.Error(evalErr))
		}
		decision = evaluated
	}

	profile := models.LeaseProfile{DefaultDuration: 12 * time.Hour}
	poolID := ""
	if decision != nil {
		profile = decision.LeaseProfile
		if decision.PoolID != "" {
			poolID = decision.PoolID
		} else if resolved := h.resolvePoolSelector(ctx, tenantID, decision.PoolSelector, meta, decision.Metadata); resolved != "" {
			poolID = resolved
		}
	}
	if poolID == "" && h.relaySel != nil {
		if relayPool, ok := h.relaySel.Resolve(meta); ok {
			poolID = relayPool
		}
	}
	if poolID == "" {
		poolID = "default-v6"
	}
	profile = h.resolvePoolLeaseProfile(ctx, tenantID, poolID, profile)

	ip := ""
	if pkt.IAAddr != nil {
		ip = pkt.IAAddr.String()
	} else if pkt.LinkAddr != nil {
		ip = pkt.LinkAddr.String()
	}
	eui64IP, nackErr := h.resolveEUI64IPv6Binding(ctx, scopeRef, poolID, pkt)
	if nackErr != nil {
		return nil, nackErr
	}
	if eui64IP != "" {
		ip = eui64IP
	}

	anchorID := policy.DeriveMobilityAnchorID(pkt.RelayInfo)
	session := make(map[string]any)
	if anchorID != "" {
		session["anchorId"] = anchorID
	}
	if ip != "" {
		session["linkAddr"] = ip
	}
	if geoZone != "" {
		session["geoZone"] = geoZone
	}
	if controllerID != "" {
		session["controllerId"] = controllerID
	}
	if len(session) == 0 {
		session = nil
	}
	allocMeta := lease.AllocationMetadata{
		UserID: userID,
		Mobility: lease.MobilityMetadata{
			AnchorID:          anchorID,
			AccessPointID:     apID,
			ControllerID:      controllerID,
			GeoZone:           geoZone,
			LocationHint:      location,
			SessionContinuity: session,
		},
		Compliance: compliance,
	}
	if h.leaseSvc == nil {
		return nil, errors.New("dhcpv6: lease service is nil")
	}
	result, err := h.leaseSvc.AllocateOrReuseWithMetadata(ctx, scopeRef, pkt.DUID, profile, poolID, ip, allocMeta)
	if err != nil {
		return nil, err
	}
	h.attachSLAAC(result, pkt, profile)
	if err := h.attachPrefixDelegation(ctx, scopeRef, result, pkt, profile, decision); err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) resolvePoolLeaseProfile(ctx context.Context, tenantID, poolID string, fallback models.LeaseProfile) models.LeaseProfile {
	if h == nil || h.poolSvc == nil {
		return fallback
	}
	trimmedTenant := strings.TrimSpace(tenantID)
	trimmedPoolID := strings.TrimSpace(poolID)
	if trimmedTenant == "" || trimmedPoolID == "" {
		return fallback
	}
	scopeRef := pool.NewResourceScope(trimmedTenant, trimmedTenant)
	poolObj, err := h.poolSvc.GetPool(ctx, scopeRef, trimmedPoolID)
	if err != nil || poolObj == nil {
		if err != nil && h.logger != nil {
			h.logger.Warn("dhcpv6 load pool for lease profile failed", zap.String("tenantId", trimmedTenant), zap.String("poolId", trimmedPoolID), zap.Error(err))
		}
		return fallback
	}
	resolved := fallback
	hasPoolLeaseConfig := false
	if poolObj.MinLeaseTime > 0 {
		resolved.DefaultDuration = time.Duration(poolObj.MinLeaseTime) * time.Second
		hasPoolLeaseConfig = true
	}
	if poolObj.MaxLeaseTime > 0 {
		resolved.MaxDuration = time.Duration(poolObj.MaxLeaseTime) * time.Second
		hasPoolLeaseConfig = true
	}
	if resolved.MaxDuration > 0 && resolved.DefaultDuration > resolved.MaxDuration {
		resolved.DefaultDuration = resolved.MaxDuration
	}
	if hasPoolLeaseConfig {
		if h.logger != nil {
			h.logger.Info("dhcpv6 lease time resolved from pool", zap.String("tenantId", trimmedTenant), zap.String("poolId", trimmedPoolID), zap.Int("leaseTime", poolObj.MinLeaseTime), zap.Int("maxLeaseTime", poolObj.MaxLeaseTime))
		}
		return resolved
	}
	profileID := strings.TrimSpace(poolObj.LeaseProfileID)
	if profileID == "" {
		return resolved
	}
	profile, err := h.poolSvc.GetLeaseProfile(ctx, scopeRef, profileID)
	if err != nil || profile == nil {
		if err != nil && h.logger != nil {
			h.logger.Warn("dhcpv6 load lease profile from pool failed", zap.String("tenantId", trimmedTenant), zap.String("poolId", trimmedPoolID), zap.String("leaseProfileId", profileID), zap.Error(err))
		}
		return resolved
	}
	resolved = *profile
	if resolved.DefaultDuration <= 0 {
		resolved.DefaultDuration = fallback.DefaultDuration
	}
	if resolved.MaxDuration <= 0 {
		resolved.MaxDuration = fallback.MaxDuration
	}
	if h.logger != nil {
		h.logger.Info("dhcpv6 lease profile resolved from pool", zap.String("tenantId", trimmedTenant), zap.String("poolId", trimmedPoolID), zap.String("leaseProfileId", profileID), zap.Duration("defaultDuration", resolved.DefaultDuration), zap.Duration("maxDuration", resolved.MaxDuration))
	}
	return resolved
}

// HandleConfirm validates the client-provided address against the allocated prefix.
// If the address does not belong to the expected subnet, it returns a zero-lifetime lease result.
func (h *Handler) HandleConfirm(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
	result, err := h.HandleRequest(ctx, tenantID, pkt)
	if err != nil {
		return nil, err
	}
	if pkt.IAAddr == nil {
		return result, nil
	}
	if result == nil || result.Pool == nil {
		if h.logger != nil {
			h.logger.Warn("dhcpv6 confirm subnet validation failed", zap.String("reason", "missing pool in result"), zap.String("clientAddr", pkt.IAAddr.String()))
		}
		return zeroLifetimeConfirmResult(pkt), nil
	}
	if !confirmAddressWithinPool(result.Pool.CIDR, pkt.IAAddr) {
		if h.logger != nil {
			h.logger.Warn("dhcpv6 confirm rejected address outside pool", zap.String("poolCIDR", result.Pool.CIDR), zap.String("clientAddr", pkt.IAAddr.String()))
		}
		return zeroLifetimeConfirmResult(pkt), nil
	}
	return result, nil
}

// HandleRenew processes DHCPv6 RENEW with idempotent reuse semantics.
func (h *Handler) HandleRenew(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
	if err := leasefsm.ValidateTransition(leasefsm.LeaseStateBound, leasefsm.LeaseStateRenewing); err != nil {
		return nil, err
	}
	result, err := h.HandleRequest(ctx, tenantID, pkt)
	if err != nil {
		return nil, err
	}
	if isIdempotentRenewRequest(pkt, result) && h.logger != nil {
		leaseIP := ""
		if result != nil && result.Lease != nil {
			leaseIP = result.Lease.IPAddress
		}
		h.logger.Debug("dhcpv6 renew idempotent lease reuse", zap.String("duid", pkt.DUID), zap.String("leaseIP", leaseIP))
	}
	if err := leasefsm.ValidateTransition(leasefsm.LeaseStateRenewing, leasefsm.LeaseStateBound); err != nil {
		return nil, err
	}
	return result, nil
}

// HandleRebind processes DHCPv6 REBIND with idempotent reuse semantics.
func (h *Handler) HandleRebind(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
	if err := leasefsm.ValidateTransition(leasefsm.LeaseStateBound, leasefsm.LeaseStateRebinding); err != nil {
		return nil, err
	}
	result, err := h.HandleRequest(ctx, tenantID, pkt)
	if err != nil {
		return nil, err
	}
	if isIdempotentRenewRequest(pkt, result) && h.logger != nil {
		leaseIP := ""
		if result != nil && result.Lease != nil {
			leaseIP = result.Lease.IPAddress
		}
		h.logger.Debug("dhcpv6 rebind idempotent lease reuse", zap.String("duid", pkt.DUID), zap.String("leaseIP", leaseIP))
	}
	if err := leasefsm.ValidateTransition(leasefsm.LeaseStateRebinding, leasefsm.LeaseStateBound); err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) resolveEUI64IPv6Binding(ctx context.Context, scope lease.ResourceScope, poolID string, pkt Packet) (string, error) {
	if h == nil || h.leaseSvc == nil || h.poolSvc == nil || len(pkt.ClientMAC) == 0 || strings.TrimSpace(poolID) == "" {
		return "", nil
	}
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	if tenantID == "" {
		return "", nil
	}
	poolScope := pool.NewResourceScope(tenantID, tenantID)
	poolObj, err := h.poolSvc.GetPool(ctx, poolScope, poolID)
	if err != nil || poolObj == nil {
		return "", nil
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
	if err != nil || !prefix.Addr().Is6() {
		return "", nil
	}
	euiPrefix := canonicalEUI64Prefix(prefix)
	mac := strings.ToLower(strings.TrimSpace(pkt.ClientMAC.String()))
	cacheKey := eui64BindingCacheKey(tenantID, mac, euiPrefix.String())

	if binding, ok := h.getCachedEUI64Binding(cacheKey); ok {
		if err := validateRequestedAddress(pkt, binding.IPv6Addr); err != nil {
			return "", err
		}
		if h.logger != nil {
			h.logger.Info("dhcpv6 eui64 binding cache hit", zap.String("mac", mac), zap.String("ipv6", binding.IPv6Addr))
		}
		return binding.IPv6Addr, nil
	}

	binding, err := h.leaseSvc.FindIPv6EUI64BindingByMAC(ctx, scope, mac, euiPrefix.String())
	if err == nil && binding != nil {
		h.setCachedEUI64Binding(cacheKey, *binding)
		if err := validateRequestedAddress(pkt, binding.IPv6Addr); err != nil {
			return "", err
		}
		if h.logger != nil {
			h.logger.Info("dhcpv6 eui64 binding found", zap.String("mac", mac), zap.String("ipv6", binding.IPv6Addr))
		}
		return binding.IPv6Addr, nil
	}
	if err != nil && !errors.Is(err, lease.ErrNotFound) {
		if h.metrics != nil && h.metrics.DHCPv6EUI64Binding != nil {
			h.metrics.DHCPv6EUI64Binding.WithLabelValues("failed").Inc()
		}
		return "", err
	}

	euiAddr, err := DeriveIPv6FromEUI64(euiPrefix, pkt.ClientMAC)
	if err != nil {
		return "", err
	}
	if h.logger != nil {
		h.logger.Info("dhcpv6 eui64 address calculated", zap.String("mac", mac), zap.String("ipv6", euiAddr.String()))
	}
	if err := validateRequestedAddress(pkt, euiAddr.String()); err != nil {
		return "", err
	}

	owner, ownerErr := h.leaseSvc.FindIPv6EUI64BindingByIPv6Addr(ctx, scope, euiPrefix.String(), euiAddr.String())
	if ownerErr == nil && owner != nil && !strings.EqualFold(owner.MAC, mac) {
		if h.metrics != nil && h.metrics.DHCPv6EUI64Binding != nil {
			h.metrics.DHCPv6EUI64Binding.WithLabelValues("failed").Inc()
		}
		return "", &NACKError{Reason: "dhcpv6 nack: address already bound to another mac"}
	}
	if ownerErr != nil && !errors.Is(ownerErr, lease.ErrNotFound) {
		return "", ownerErr
	}

	conflictTimeout := h.ipv6ProbeTimeout
	if conflictTimeout <= 0 {
		conflictTimeout = time.Second
	}
	conflicted, probeErr := h.ipv6ConflictFn(ctx, euiAddr.String(), conflictTimeout)
	if h.metrics != nil && h.metrics.DHCPv6IPv6Conflicts != nil {
		if conflicted {
			h.metrics.DHCPv6IPv6Conflicts.WithLabelValues("detected").Inc()
		} else {
			h.metrics.DHCPv6IPv6Conflicts.WithLabelValues("clear").Inc()
		}
	}
	if h.logger != nil {
		h.logger.Info("dhcpv6 icmpv6 conflict probe", zap.String("mac", mac), zap.String("ipv6", euiAddr.String()), zap.Bool("conflict", conflicted), zap.Duration("timeout", conflictTimeout))
	}
	if probeErr != nil && h.logger != nil {
		h.logger.Warn("dhcpv6 icmpv6 conflict probe error", zap.String("mac", mac), zap.String("ipv6", euiAddr.String()), zap.Error(probeErr))
	}
	if conflicted {
		if h.metrics != nil && h.metrics.DHCPv6EUI64Binding != nil {
			h.metrics.DHCPv6EUI64Binding.WithLabelValues("failed").Inc()
		}
		return "", &NACKError{Reason: "dhcpv6 nack: ipv6 address conflict detected"}
	}

	created, err := h.leaseSvc.CreateIPv6EUI64Binding(ctx, scope, lease.IPv6EUI64Binding{
		MAC:       mac,
		Prefix:    euiPrefix.String(),
		IPv6Addr:  euiAddr.String(),
		LeaseTime: 365 * 24 * time.Hour,
	})
	if err != nil {
		if h.metrics != nil && h.metrics.DHCPv6EUI64Binding != nil {
			h.metrics.DHCPv6EUI64Binding.WithLabelValues("failed").Inc()
		}
		return "", err
	}
	if created != nil {
		h.setCachedEUI64Binding(cacheKey, *created)
	}
	if h.metrics != nil && h.metrics.DHCPv6EUI64Binding != nil {
		h.metrics.DHCPv6EUI64Binding.WithLabelValues("success").Inc()
	}
	if h.logger != nil {
		h.logger.Info("dhcpv6 eui64 binding created", zap.String("mac", mac), zap.String("ipv6", euiAddr.String()))
	}
	return euiAddr.String(), nil
}

func validateRequestedAddress(pkt Packet, boundAddr string) error {
	if pkt.IAAddr == nil {
		return nil
	}
	req := strings.ToLower(strings.TrimSpace(pkt.IAAddr.String()))
	bound := strings.ToLower(strings.TrimSpace(boundAddr))
	if req == "" || bound == "" || req == bound {
		return nil
	}
	return &NACKError{Reason: "dhcpv6 nack: requested address does not match bound eui64 address"}
}

func canonicalEUI64Prefix(prefix netip.Prefix) netip.Prefix {
	bits := prefix.Bits()
	if bits <= 0 || bits > 64 {
		bits = 64
	}
	masked := netip.PrefixFrom(prefix.Masked().Addr(), bits).Masked()
	if bits == 64 {
		return masked
	}
	return netip.PrefixFrom(masked.Addr(), 64).Masked()
}

func eui64BindingCacheKey(tenantID, mac, prefix string) string {
	return strings.ToLower(strings.TrimSpace(tenantID)) + "|" + strings.ToLower(strings.TrimSpace(mac)) + "|" + strings.TrimSpace(prefix)
}

func (h *Handler) getCachedEUI64Binding(key string) (lease.IPv6EUI64Binding, bool) {
	h.eui64CacheMu.RLock()
	defer h.eui64CacheMu.RUnlock()
	b, ok := h.eui64Cache[key]
	return b, ok
}

func (h *Handler) setCachedEUI64Binding(key string, binding lease.IPv6EUI64Binding) {
	h.eui64CacheMu.Lock()
	defer h.eui64CacheMu.Unlock()
	h.eui64Cache[key] = binding
}

func confirmAddressWithinPool(poolCIDR string, clientAddr net.IP) bool {
	if clientAddr == nil {
		return false
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(poolCIDR))
	if err != nil || !prefix.Addr().Is6() {
		return false
	}
	addr, ok := netip.AddrFromSlice(clientAddr)
	if !ok || !addr.Is6() {
		return false
	}
	return prefix.Masked().Contains(addr)
}

func zeroLifetimeConfirmResult(pkt Packet) *lease.Result {
	addr := "::"
	if pkt.IAAddr != nil {
		addr = pkt.IAAddr.String()
	}
	return &lease.Result{
		Lease: &models.Lease{IPAddress: addr},
		Profile: models.LeaseProfile{
			DefaultDuration: 0,
			MaxDuration:     0,
			RenewalTime:     0,
			RebindingTime:   0,
		},
	}
}

func isIdempotentRenewRequest(pkt Packet, result *lease.Result) bool {
	if pkt.IAAddr == nil || result == nil || result.Lease == nil || !result.Reused {
		return false
	}
	requested := pkt.IAAddr.To16()
	if requested == nil || pkt.IAAddr.To4() != nil {
		return false
	}
	allocated := net.ParseIP(strings.TrimSpace(result.Lease.IPAddress)).To16()
	if allocated == nil {
		return false
	}
	return allocated.Equal(requested)
}

func (h *Handler) resolvePoolSelector(ctx context.Context, tenantID string, selector *policy.PoolSelector, relayMeta relay.Metadata, decisionMetadata map[string]any) string {
	if poolID := resolveRelayPoolFromPolicyMetadata(decisionMetadata, relayMeta); poolID != "" {
		if h.logger != nil {
			h.logger.Debug("pool selector resolved via policy relay rule", zap.String("tenantId", tenantID), zap.String("poolId", poolID), zap.String("relayId", relayMeta.RelayID), zap.String("interfaceId", relayMeta.CircuitID), zap.String("remoteId", relayMeta.RemoteID))
		}
		return poolID
	}

	if selector != nil && h.poolSvc != nil {
		metadataSelector := pool.MetadataSelector{
			InterfaceID:   selector.InterfaceID,
			SSID:          selector.SSID,
			Location:      selector.Location,
			VLANID:        selector.VLANID,
			AccessPointID: selector.AccessPointID,
			ControllerID:  selector.ControllerID,
			GeoZone:       selector.GeoZone,
		}
		if strings.TrimSpace(metadataSelector.InterfaceID) == "" {
			metadataSelector.InterfaceID = relayInterfaceID(relayMeta)
		}
		started := time.Now()
		scopeRef := pool.NewResourceScope("", tenantID)
		poolObj, err := h.poolSvc.ResolvePool(ctx, scopeRef, metadataSelector)
		observability.ObserveSelectorMetrics(h.metrics, tenantID, metadataSelector, poolObj, err, started)
		if err != nil {
			if !errors.Is(err, pool.ErrPoolNotFound) {
				h.logger.Warn("pool selector resolution failed", zap.Error(err))
			}
		} else if poolObj != nil {
			resolutionPayload := auditpayload.PoolResolution{
				PoolSelector: auditpayload.PoolSelector{
					InterfaceID:   strings.TrimSpace(metadataSelector.InterfaceID),
					SSID:          strings.TrimSpace(metadataSelector.SSID),
					Location:      strings.TrimSpace(metadataSelector.Location),
					VLANID:        metadataSelector.VLANID,
					AccessPointID: strings.TrimSpace(metadataSelector.AccessPointID),
					ControllerID:  strings.TrimSpace(metadataSelector.ControllerID),
					GeoZone:       strings.TrimSpace(metadataSelector.GeoZone),
				},
				ResolvedPool: auditpayload.PoolSnapshot{
					ID:             poolObj.ID,
					Name:           poolObj.Name,
					Scope:          poolObj.Scope,
					ParentID:       poolObj.ParentID,
					CIDR:           poolObj.CIDR,
					LeaseProfileID: poolObj.LeaseProfileID,
				},
				Metadata: auditpayload.PoolMetadata{
					VLANID:      poolObj.VLANID,
					InterfaceID: poolObj.InterfaceID,
					SSID:        poolObj.SSID,
					Location:    poolObj.Location,
					Tags:        poolObj.Tags,
				},
			}
			if h.logger != nil {
				h.logger.Debug("pool selector resolved", zap.String("tenantId", tenantID), zap.Any("poolResolution", resolutionPayload))
			}
			return poolObj.ID
		}
	}

	if h.relaySel != nil {
		if relayPool, ok := h.relaySel.Resolve(relayMeta); ok {
			if h.logger != nil {
				h.logger.Debug("pool selector resolved via relay selector", zap.String("tenantId", tenantID), zap.String("poolId", relayPool), zap.String("relayId", relayMeta.RelayID), zap.String("interfaceId", relayMeta.CircuitID), zap.String("remoteId", relayMeta.RemoteID))
			}
			return relayPool
		}
	}

	return ""
}

func relayInterfaceID(meta relay.Metadata) string {
	if v := strings.TrimSpace(meta.CircuitID); v != "" {
		return v
	}
	if v := strings.TrimSpace(meta.Attribute("interface-id", "agent.circuit-id")); v != "" {
		return v
	}
	return ""
}

func relayRemoteID(meta relay.Metadata) string {
	if v := strings.TrimSpace(meta.RemoteID); v != "" {
		return v
	}
	if v := strings.TrimSpace(meta.Attribute("remote-id", "agent.remote-id")); v != "" {
		return v
	}
	return ""
}

func resolveRelayPoolFromPolicyMetadata(metadata map[string]any, relayMeta relay.Metadata) string {
	if len(metadata) == 0 {
		return ""
	}
	if poolID := resolveRelayPoolRuleList(metadata["relayPoolRules"], relayMeta); poolID != "" {
		return poolID
	}
	if poolID := resolveRelayPoolRuleList(metadata["relaySelectionRules"], relayMeta); poolID != "" {
		return poolID
	}
	interfaceID := strings.ToLower(relayInterfaceID(relayMeta))
	if interfaceID != "" {
		for _, key := range []string{"relayPoolByInterface", "poolByInterface"} {
			if poolID := stringMapLookup(metadata[key], interfaceID); poolID != "" {
				return poolID
			}
		}
	}
	remoteID := strings.ToLower(relayRemoteID(relayMeta))
	if remoteID != "" {
		for _, key := range []string{"relayPoolByRemote", "poolByRemote"} {
			if poolID := stringMapLookup(metadata[key], remoteID); poolID != "" {
				return poolID
			}
		}
	}
	return ""
}

func resolveRelayPoolRuleList(raw any, relayMeta relay.Metadata) string {
	rules, ok := raw.([]any)
	if !ok {
		return ""
	}
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		poolID := strings.TrimSpace(toStringAny(rule["poolId"]))
		if poolID == "" {
			poolID = strings.TrimSpace(toStringAny(rule["pool"]))
		}
		if poolID == "" {
			continue
		}
		match := rule
		if rawMatch, exists := rule["match"]; exists {
			if typed, ok := rawMatch.(map[string]any); ok {
				match = typed
			}
		}
		if relayRuleMatched(match, relayMeta) {
			return poolID
		}
	}
	return ""
}

func relayRuleMatched(match map[string]any, relayMeta relay.Metadata) bool {
	if len(match) == 0 {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["tenantId"])); want != "" && !strings.EqualFold(want, relayMeta.TenantID) {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["relayId"])); want != "" && !strings.EqualFold(want, relayMeta.RelayID) {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["giaddr"])); want != "" && !strings.EqualFold(want, relayMeta.GIAddr) {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["interfaceId"])); want != "" && !strings.EqualFold(want, relayInterfaceID(relayMeta)) {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["circuitId"])); want != "" && !strings.EqualFold(want, relayInterfaceID(relayMeta)) {
		return false
	}
	if want := strings.TrimSpace(toStringAny(match["remoteId"])); want != "" && !strings.EqualFold(want, relayRemoteID(relayMeta)) {
		return false
	}
	wantVLAN := toIntAny(match["vlanId"])
	if wantVLAN > 0 && relayMeta.VLANID != wantVLAN {
		return false
	}
	return true
}

func stringMapLookup(raw any, key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return ""
	}
	typed, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	for k, v := range typed {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.TrimSpace(toStringAny(v))
		}
	}
	return ""
}

func toStringAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func toIntAny(value any) int {
	if value == nil {
		return 0
	}
	s := strings.TrimSpace(fmt.Sprint(value))
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

// HandleRelease processes DHCPv6 RELEASE for prefixes.
func (h *Handler) HandleRelease(ctx context.Context, tenantID string, pkt Packet) error {
	if err := h.authorizeRelay(tenantID, relay.BuildMetadata(tenantID, pkt.LinkAddr, pkt.RelayInfo, parseRelayVLAN(pkt.RelayInfo), nil)); err != nil {
		return err
	}
	if len(pkt.PrefixRequests) == 0 {
		return nil
	}
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	for _, req := range pkt.PrefixRequests {
		if err := h.leaseSvc.ReleasePrefix(ctx, scopeRef, pkt.DUID, req.IAPDID); err != nil {
			return err
		}
	}
	return nil
}

// HandleDecline processes DHCPv6 DECLINE for prefixes.
func (h *Handler) HandleDecline(ctx context.Context, tenantID string, pkt Packet) error {
	if err := h.authorizeRelay(tenantID, relay.BuildMetadata(tenantID, pkt.LinkAddr, pkt.RelayInfo, parseRelayVLAN(pkt.RelayInfo), nil)); err != nil {
		return err
	}
	if len(pkt.PrefixRequests) == 0 {
		return nil
	}
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	for _, req := range pkt.PrefixRequests {
		if err := h.leaseSvc.DeclinePrefix(ctx, scopeRef, pkt.DUID, req.IAPDID); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) attachSLAAC(result *lease.Result, pkt Packet, profile models.LeaseProfile) {
	if result == nil || result.Pool == nil || !pkt.SupportsSLAAC {
		return
	}
	prefix, err := netip.ParsePrefix(result.Pool.CIDR)
	if err != nil || !prefix.Addr().Is6() {
		return
	}
	seed, err := buildSLAACSeed(pkt)
	if err != nil {
		return
	}
	addr, err := deriveSLAACAddress(prefix, seed)
	if err != nil {
		return
	}
	preferred := profile.DefaultDuration
	if preferred <= 0 {
		preferred = time.Hour
	}
	valid := profile.MaxDuration
	if valid < preferred {
		valid = preferred * 2
	}
	result.SLAAC = &lease.SLAACInfo{
		Prefix:            prefix.String(),
		Address:           addr.String(),
		PreferredLifetime: preferred,
		ValidLifetime:     valid,
		Mode:              "hybrid",
	}
}

func (h *Handler) lookupComplianceMetadata(tenantID string, pkt Packet) lease.ComplianceMetadata {
	if h.mdmService == nil {
		return lease.ComplianceMetadata{}
	}
	deviceID := pkt.DUID
	if deviceID == "" && len(pkt.ClientMAC) > 0 {
		deviceID = strings.ToLower(pkt.ClientMAC.String())
	}
	record, ok := h.mdmService.Lookup(tenantID, deviceID)
	if !ok {
		return lease.ComplianceMetadata{}
	}
	return lease.ComplianceMetadata{
		Valid:      true,
		Managed:    record.Managed,
		Source:     record.Source,
		Tags:       append([]string(nil), record.Tags...),
		ObservedAt: record.ObservedAt,
	}
}

func (h *Handler) attachPrefixDelegation(ctx context.Context, scope lease.ResourceScope, result *lease.Result, pkt Packet, profile models.LeaseProfile, decision *policy.Decision) error {
	if result == nil || result.Pool == nil || len(pkt.PrefixRequests) == 0 {
		return nil
	}
	requests := make([]lease.PrefixRequest, len(pkt.PrefixRequests))
	copy(requests, pkt.PrefixRequests)
	reasons := make(map[uint32]string, len(requests))
	for i := range requests {
		staticPD, staticErr := h.resolveStaticPrefixDelegation(ctx, scope, pkt.DUID, requests[i], decision)
		if staticErr != nil {
			return staticErr
		}
		if staticPD == nil {
			reasons[requests[i].IAPDID] = "dynamic"
			continue
		}
		parsedPrefix, err := netip.ParsePrefix(staticPD.Prefix)
		if err != nil {
			return err
		}
		if !parsedPrefix.Addr().Is6() {
			return fmt.Errorf("dhcpv6 static prefix must be ipv6: %s", staticPD.Prefix)
		}
		poolPrefix, err := netip.ParsePrefix(strings.TrimSpace(result.Pool.CIDR))
		if err != nil || !poolPrefix.Addr().Is6() {
			return fmt.Errorf("dhcpv6 invalid pool cidr for static pd: %s", result.Pool.CIDR)
		}
		if !poolPrefix.Masked().Contains(parsedPrefix.Masked().Addr()) {
			return fmt.Errorf("dhcpv6 static prefix %s outside pool %s", staticPD.Prefix, result.Pool.CIDR)
		}
		requests[i].Prefix = net.ParseIP(parsedPrefix.Masked().Addr().String())
		if staticPD.Length > 0 {
			requests[i].PrefixLength = staticPD.Length
		} else {
			requests[i].PrefixLength = byte(parsedPrefix.Bits())
		}
		reasons[requests[i].IAPDID] = staticPD.Reason
	}
	delegations, err := h.leaseSvc.AllocateOrReusePrefixes(ctx, scope, pkt.DUID, profile, result.Pool, requests)
	if err != nil {
		return err
	}
	result.PrefixDelegations = delegations
	for i := range delegations {
		reason := reasons[delegations[i].IAPDID]
		if reason == "" {
			reason = "dynamic"
		}
		if h.logger != nil {
			h.logger.Info("dhcpv6 pd allocation decided", zap.String("duid", pkt.DUID), zap.String("prefix", delegations[i].Prefix), zap.Uint8("prefixLength", delegations[i].PrefixLength), zap.String("reason", reason))
		}
	}
	return nil
}

func (h *Handler) resolveStaticPrefixDelegation(ctx context.Context, scope lease.ResourceScope, duid string, req lease.PrefixRequest, decision *policy.Decision) (*staticPrefixDecision, error) {
	if staticPD, ok := h.staticPDBindings[strings.ToLower(strings.TrimSpace(duid))]; ok {
		if err := h.rejectStaticPrefixConflict(ctx, scope, duid, req.IAPDID, staticPD.Prefix); err != nil {
			return nil, err
		}
		resolved := staticPD
		if strings.TrimSpace(resolved.Reason) == "" {
			resolved.Reason = "static-config"
		}
		return &resolved, nil
	}
	if staticPD, ok := staticPrefixFromPolicy(decision); ok {
		if err := h.rejectStaticPrefixConflict(ctx, scope, duid, req.IAPDID, staticPD.Prefix); err != nil {
			return nil, err
		}
		return &staticPD, nil
	}
	if h.poolSvc == nil {
		return nil, nil
	}
	binding, err := h.poolSvc.FindBinding(ctx, pool.ResourceScopeFromAccess(scope.AccessScope()), duid, "")
	if err != nil || binding == nil {
		return nil, nil
	}
	staticPD, ok := staticPrefixFromBinding(binding.Metadata)
	if !ok {
		return nil, nil
	}
	if err := h.rejectStaticPrefixConflict(ctx, scope, duid, req.IAPDID, staticPD.Prefix); err != nil {
		return nil, err
	}
	return &staticPD, nil
}

func (h *Handler) rejectStaticPrefixConflict(ctx context.Context, scope lease.ResourceScope, duid string, iapdID uint32, prefix string) error {
	if h.leaseSvc == nil {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	const pageSize = 256
	offset := 0
	for {
		leases, err := h.leaseSvc.ListPrefixLeases(ctx, scope, "ACTIVE", pageSize, offset)
		if err != nil {
			return err
		}
		for i := range leases {
			candidate := fmt.Sprintf("%s/%d", leases[i].Prefix, leases[i].PrefixLen)
			if !strings.EqualFold(candidate, prefix) {
				continue
			}
			if leases[i].ClientID == duid && leases[i].IAPDID == iapdID {
				continue
			}
			return fmt.Errorf("dhcpv6 static pd prefix conflict: %s already allocated", prefix)
		}
		if len(leases) < pageSize {
			break
		}
		offset += len(leases)
	}
	return nil
}

func staticPrefixFromPolicy(decision *policy.Decision) (staticPrefixDecision, bool) {
	if decision == nil || len(decision.Metadata) == 0 {
		return staticPrefixDecision{}, false
	}
	raw, ok := decision.Metadata["staticPrefix"]
	if !ok {
		raw, ok = decision.Metadata["pdStaticPrefix"]
	}
	if !ok {
		return staticPrefixDecision{}, false
	}
	prefix := strings.TrimSpace(fmt.Sprint(raw))
	if prefix == "" {
		return staticPrefixDecision{}, false
	}
	parsed, err := netip.ParsePrefix(prefix)
	if err != nil {
		return staticPrefixDecision{}, false
	}
	return staticPrefixDecision{Prefix: parsed.Masked().String(), Length: byte(parsed.Bits()), Reason: "static-policy"}, true
}

func staticPrefixFromBinding(metadata []byte) (staticPrefixDecision, bool) {
	if len(metadata) == 0 {
		return staticPrefixDecision{}, false
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return staticPrefixDecision{}, false
	}
	raw, ok := decoded["pdPrefix"]
	if !ok {
		raw, ok = decoded["staticPrefix"]
	}
	if !ok {
		return staticPrefixDecision{}, false
	}
	prefix := strings.TrimSpace(fmt.Sprint(raw))
	if prefix == "" {
		return staticPrefixDecision{}, false
	}
	parsed, err := netip.ParsePrefix(prefix)
	if err != nil {
		return staticPrefixDecision{}, false
	}
	return staticPrefixDecision{Prefix: parsed.Masked().String(), Length: byte(parsed.Bits()), Reason: "static-binding"}, true
}

func relayPolicyAttributes(pkt Packet) map[string]any {
	attrs := map[string]any{"iaid": pkt.IAID}
	for k, v := range pkt.RelayInfo {
		attrs[k] = v
	}
	if pkt.LinkAddr != nil {
		addr := pkt.LinkAddr.String()
		if addr != "" {
			attrs["link-addr"] = addr
			if _, ok := attrs["giaddr"]; !ok {
				attrs["giaddr"] = addr
			}
		}
	}
	if pkt.PeerAddr != nil {
		if peer := pkt.PeerAddr.String(); peer != "" {
			attrs["peer-addr"] = peer
		}
	}
	return attrs
}

func parseRelayVLAN(relay map[string]string) int {
	if len(relay) == 0 {
		return 0
	}
	for _, key := range []string{"vlan-id", "agent.vlan-id"} {
		if raw, ok := relay[key]; ok {
			if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				return v
			}
		}
	}
	return 0
}

func (h *Handler) authorizeRelay(tenantID string, meta relay.Metadata) error {
	if h == nil || h.relayAuth == nil {
		return nil
	}
	if err := h.relayAuth.Authorize(meta); err != nil {
		if h.logger != nil {
			h.logger.Warn("relay authentication failed", zap.String("tenantId", tenantID), zap.String("relayId", meta.RelayID), zap.String("giaddr", meta.GIAddr), zap.Error(err))
		}
		return err
	}
	return nil
}

func (h *Handler) deviceClassification(pkt Packet) (string, string, []string) {
	deviceType := policy.ClassifyDeviceType(pkt.VendorClass, pkt.UserClass, pkt.RelayInfo)
	if h == nil || h.fingerprinter == nil {
		return deviceType, "", nil
	}
	signals := mobility.DeviceSignals{
		VendorClass: pkt.VendorClass,
		UserClass:   pkt.UserClass,
		RelayInfo:   pkt.RelayInfo,
		MAC:         pkt.ClientMAC,
	}
	if match, ok := h.fingerprinter.Match(signals); ok {
		persona := match.Persona
		if match.Platform != "" {
			deviceType = match.Platform
		}
		return deviceType, persona, match.Tags
	}
	return deviceType, "", nil
}
