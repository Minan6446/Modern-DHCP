package dhcpv6

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

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

// Handler handles DHCPv6 flows.
type Handler struct {
	leaseSvc      *lease.Service
	policy        *policy.Engine
	poolSvc       *pool.Service
	metrics       *metrics.Collector
	logger        *zap.Logger
	relayAuth     *relay.Authenticator
	relaySel      *relay.Selector
	fingerprinter *mobility.Detector
	mdmService    *mdm.Service
}

// NewHandler builds a DHCPv6 handler.
func NewHandler(leaseSvc *lease.Service, policyEngine *policy.Engine, poolSvc *pool.Service, metricsCollector *metrics.Collector, authenticator *relay.Authenticator, selector *relay.Selector, logger *zap.Logger, fingerprinter *mobility.Detector, mdmSvc *mdm.Service) *Handler {
	return &Handler{leaseSvc: leaseSvc, policy: policyEngine, poolSvc: poolSvc, metrics: metricsCollector, relayAuth: authenticator, relaySel: selector, logger: logger, fingerprinter: fingerprinter, mdmService: mdmSvc}
}

// HandleRequest allocates addresses or prefixes.
func (h *Handler) HandleRequest(ctx context.Context, tenantID string, pkt Packet) (*lease.Result, error) {
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
	decision, err := h.policy.Evaluate(ctx, policy.Input{
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
	if err != nil {
		h.logger.Warn("no policy match", zap.Error(err))
	}

	profile := models.LeaseProfile{DefaultDuration: 12 * time.Hour}
	poolID := ""
	if decision != nil {
		profile = decision.LeaseProfile
		if decision.PoolID != "" {
			poolID = decision.PoolID
		} else if resolved := h.resolvePoolSelector(ctx, tenantID, decision.PoolSelector); resolved != "" {
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

	ip := ""
	if pkt.IAAddr != nil {
		ip = pkt.IAAddr.String()
	} else if pkt.LinkAddr != nil {
		ip = pkt.LinkAddr.String()
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
	result, err := h.leaseSvc.AllocateOrReuseWithMetadata(ctx, tenantID, pkt.DUID, profile, poolID, ip, allocMeta)
	if err != nil {
		return nil, err
	}
	h.attachSLAAC(result, pkt, profile)
	if err := h.attachPrefixDelegation(ctx, tenantID, result, pkt, profile); err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) resolvePoolSelector(ctx context.Context, tenantID string, selector *policy.PoolSelector) string {
	if selector == nil || h.poolSvc == nil {
		return ""
	}
	metadataSelector := pool.MetadataSelector{
		InterfaceID:   selector.InterfaceID,
		SSID:          selector.SSID,
		Location:      selector.Location,
		VLANID:        selector.VLANID,
		AccessPointID: selector.AccessPointID,
		ControllerID:  selector.ControllerID,
		GeoZone:       selector.GeoZone,
	}
	started := time.Now()
	poolObj, err := h.poolSvc.ResolvePool(ctx, tenantID, metadataSelector)
	observability.ObserveSelectorMetrics(h.metrics, tenantID, metadataSelector, poolObj, err, started)
	if err != nil {
		if !errors.Is(err, pool.ErrPoolNotFound) {
			h.logger.Warn("pool selector resolution failed", zap.Error(err))
		}
		return ""
	}
	if poolObj == nil {
		return ""
	}
	resolutionPayload := auditpayload.PoolResolution{
		PoolSelector: auditpayload.PoolSelector{
			InterfaceID:   strings.TrimSpace(selector.InterfaceID),
			SSID:          strings.TrimSpace(selector.SSID),
			Location:      strings.TrimSpace(selector.Location),
			VLANID:        selector.VLANID,
			AccessPointID: strings.TrimSpace(selector.AccessPointID),
			ControllerID:  strings.TrimSpace(selector.ControllerID),
			GeoZone:       strings.TrimSpace(selector.GeoZone),
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

// HandleRelease processes DHCPv6 RELEASE for prefixes.
func (h *Handler) HandleRelease(ctx context.Context, tenantID string, pkt Packet) error {
	if err := h.authorizeRelay(tenantID, relay.BuildMetadata(tenantID, pkt.LinkAddr, pkt.RelayInfo, parseRelayVLAN(pkt.RelayInfo), nil)); err != nil {
		return err
	}
	if len(pkt.PrefixRequests) == 0 {
		return nil
	}
	for _, req := range pkt.PrefixRequests {
		if err := h.leaseSvc.ReleasePrefix(ctx, tenantID, pkt.DUID, req.IAPDID); err != nil {
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
	for _, req := range pkt.PrefixRequests {
		if err := h.leaseSvc.DeclinePrefix(ctx, tenantID, pkt.DUID, req.IAPDID); err != nil {
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
	seed := buildSLAACSeed(pkt)
	addr, ok := deriveSLAACAddress(prefix, seed)
	if !ok {
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
		Address:           addr,
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

func (h *Handler) attachPrefixDelegation(ctx context.Context, tenantID string, result *lease.Result, pkt Packet, profile models.LeaseProfile) error {
	if result == nil || result.Pool == nil || len(pkt.PrefixRequests) == 0 {
		return nil
	}
	delegations, err := h.leaseSvc.AllocateOrReusePrefixes(ctx, tenantID, pkt.DUID, profile, result.Pool, pkt.PrefixRequests)
	if err != nil {
		return err
	}
	result.PrefixDelegations = delegations
	return nil
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
