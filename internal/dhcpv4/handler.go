package dhcpv4

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/dhcpv4/leasefsm"
	dhcpv4metrics "modern-dhcp/internal/dhcpv4/metrics"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/mobility"
	"modern-dhcp/internal/mobility/mdm"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/observability"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/relay"
	securityguard "modern-dhcp/internal/security/guard"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

// Packet abstracts the minimal DHCPv4 packet data needed for policy evaluation.
type Packet struct {
	XID                  uint32
	CHAddr               net.HardwareAddr
	ClientID             string
	RequestPhase         string
	CIAddr               net.IP
	GIAddr               net.IP
	RelayAgentInfo       map[string]string
	Option82Present      bool
	Option82CircuitID    string
	Option82RemoteID     string
	Option82SubscriberID string
	Option82Error        string
	VendorClass          string
	UserClass            string
	RequestedIP          net.IP
	Options              map[byte][]byte
	Broadcast            bool
	IsBOOTP              bool
	IPClass              string
}

const (
	dhcpv4RequestPhaseRequest = "request"
	dhcpv4RequestPhaseRenew   = "renew"
	dhcpv4RequestPhaseRebind  = "rebind"
)

// Handler processes DHCPv4 messages.
type Handler struct {
	leaseSvc      *lease.Service
	policy        *policy.Engine
	poolSvc       *pool.Service
	metrics       *metrics.Collector
	dhcpMetrics   dhcpv4metrics.Observer
	recorder      monitoring.RequestRecorder
	logger        *zap.Logger
	guard         securityguard.Guard
	relayAuth     *relay.Authenticator
	relaySel      *relay.Selector
	affinity      mobility.Cache
	fingerprinter *mobility.Detector
	mdmService    *mdm.Service
}

// NewHandler creates a v4 handler.
func NewHandler(leaseSvc *lease.Service, policyEngine *policy.Engine, poolSvc *pool.Service, metricsCollector *metrics.Collector, guard securityguard.Guard, authenticator *relay.Authenticator, selector *relay.Selector, logger *zap.Logger, recorder monitoring.RequestRecorder, mobilityCache mobility.Cache, fingerprinter *mobility.Detector, mdmSvc *mdm.Service) *Handler {
	if guard == nil {
		guard = securityguard.NewNoop()
	}
	return &Handler{leaseSvc: leaseSvc, policy: policyEngine, poolSvc: poolSvc, metrics: metricsCollector, dhcpMetrics: dhcpv4metrics.NewObserver(metricsCollector), recorder: recorder, guard: guard, relayAuth: authenticator, relaySel: selector, logger: logger, affinity: mobilityCache, fingerprinter: fingerprinter, mdmService: mdmSvc}
}

// HandleDiscover performs policy evaluation and pre-allocates state.
func (h *Handler) HandleDiscover(ctx context.Context, tenantID string, pkt Packet) (res *lease.Result, err error) {
	opStarted := time.Now()
	defer h.observeLeaseOpDuration("allocate", opStarted)
	started := time.Now()
	defer func() { h.observeLifecycle(tenantID, "DISCOVER", started, err) }()
	if err := h.authorizeRelay(tenantID, pkt); err != nil {
		return nil, err
	}
	if err := h.runGuard(ctx, tenantID, "discover", pkt, "discover"); err != nil {
		return nil, err
	}
	id := h.identity(pkt)
	compliance := h.lookupComplianceMetadata(tenantID, pkt)
	profile, poolID := h.leasePlan(ctx, tenantID, id, pkt, compliance.Valid && compliance.Managed)
	requestedIP := h.desiredIP(pkt)
	meta := h.allocationMetadata(pkt)
	meta.Compliance = compliance
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	result, err := h.leaseSvc.AllocateOrReuseWithMetadata(ctx, scopeRef, id, profile, poolID, requestedIP, meta)
	if err == nil {
		if result != nil && result.Lease != nil {
			if transitionErr := h.leaseSvc.UpdateDHCPv4LeaseFSMByLeaseID(ctx, scopeRef, result.Lease.ID, leasefsm.StateOffered); transitionErr != nil {
				return nil, transitionErr
			}
		}
		h.rememberMobilityAffinity(ctx, tenantID, id, pkt, meta, result)
	}
	return result, err
}

// HandleRequest finalizes allocation for DHCPREQUEST packets.
func (h *Handler) HandleRequest(ctx context.Context, tenantID string, pkt Packet) (res *lease.Result, err error) {
	op := "allocate"
	if pkt.RequestPhase == dhcpv4RequestPhaseRenew || pkt.RequestPhase == dhcpv4RequestPhaseRebind {
		op = "renew"
	}
	opStarted := time.Now()
	defer h.observeLeaseOpDuration(op, opStarted)
	started := time.Now()
	defer func() { h.observeLifecycle(tenantID, "REQUEST", started, err) }()
	if err := h.authorizeRelay(tenantID, pkt); err != nil {
		return nil, err
	}
	if err := h.runGuard(ctx, tenantID, "request", pkt, pkt.RequestPhase); err != nil {
		return nil, err
	}
	id := h.identity(pkt)
	compliance := h.lookupComplianceMetadata(tenantID, pkt)
	profile, poolID := h.leasePlan(ctx, tenantID, id, pkt, compliance.Valid && compliance.Managed)
	requestedIP := h.desiredIP(pkt)
	meta := h.allocationMetadata(pkt)
	meta.Compliance = compliance
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	result, err := h.leaseSvc.AllocateOrReuseWithMetadata(ctx, scopeRef, id, profile, poolID, requestedIP, meta)
	if err != nil {
		if isSyncAckGateError(err) && h.logger != nil {
			h.logger.Warn("dhcpv4 request blocked by sync ack gate",
				zap.String("tenant", tenantID),
				zap.String("identifier", id),
				zap.String("phase", pkt.RequestPhase),
				zap.Error(err),
			)
		}
		return nil, err
	}
	target := leasefsm.StateBound
	switch pkt.RequestPhase {
	case dhcpv4RequestPhaseRenew:
		target = leasefsm.StateRenewing
	case dhcpv4RequestPhaseRebind:
		target = leasefsm.StateRebinding
	}
	if result != nil && result.Lease != nil {
		if err := h.leaseSvc.UpdateDHCPv4LeaseFSMByLeaseID(ctx, scopeRef, result.Lease.ID, target); err != nil {
			return nil, err
		}
	}
	h.rememberMobilityAffinity(ctx, tenantID, id, pkt, meta, result)
	h.emitLeaseChange(ctx, tenantID, pkt, result, securityguard.LeaseActionGranted)
	return result, nil
}

func isSyncAckGateError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "sync ack gate")
}

// HandleBootRequest responds to BOOTP clients that do not speak DHCP extensions.
func (h *Handler) HandleBootRequest(ctx context.Context, tenantID string, pkt Packet) (res *lease.Result, err error) {
	started := time.Now()
	defer func() { h.observeLifecycle(tenantID, "BOOTREQUEST", started, err) }()
	if err := h.authorizeRelay(tenantID, pkt); err != nil {
		return nil, err
	}
	if err := h.runGuard(ctx, tenantID, "bootp", pkt, dhcpv4RequestPhaseRequest); err != nil {
		return nil, err
	}
	id := h.identity(pkt)
	compliance := h.lookupComplianceMetadata(tenantID, pkt)
	profile, poolID := h.leasePlan(ctx, tenantID, id, pkt, compliance.Valid && compliance.Managed)
	requestedIP := h.desiredIP(pkt)
	meta := h.allocationMetadata(pkt)
	meta.Compliance = compliance
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	result, err := h.leaseSvc.AllocateOrReuseWithMetadata(ctx, scopeRef, id, profile, poolID, requestedIP, meta)
	if err != nil {
		return nil, err
	}
	h.rememberMobilityAffinity(ctx, tenantID, id, pkt, meta, result)
	h.emitLeaseChange(ctx, tenantID, pkt, result, securityguard.LeaseActionGranted)
	return result, nil
}

// HandleDecline records lease declines from clients.
func (h *Handler) HandleDecline(ctx context.Context, tenantID string, pkt Packet) (err error) {
	opStarted := time.Now()
	defer h.observeLeaseOpDuration("release", opStarted)
	started := time.Now()
	defer func() { h.observeLifecycle(tenantID, "DECLINE", started, err) }()
	if err := h.authorizeRelay(tenantID, pkt); err != nil {
		return err
	}
	if err := h.runGuard(ctx, tenantID, "decline", pkt, dhcpv4RequestPhaseRequest); err != nil {
		return err
	}
	id := h.identity(pkt)
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	if _, _, err := h.leaseSvc.ReleaseByIdentifier(ctx, scopeRef, id); err != nil {
		return err
	}
	return h.leaseSvc.UpdateDHCPv4LeaseFSMByIdentifier(ctx, scopeRef, id, leasefsm.StateReleased)
}

// HandleRelease records lease release events from clients.
func (h *Handler) HandleRelease(ctx context.Context, tenantID string, pkt Packet) (err error) {
	opStarted := time.Now()
	defer h.observeLeaseOpDuration("release", opStarted)
	started := time.Now()
	defer func() { h.observeLifecycle(tenantID, "RELEASE", started, err) }()
	if err := h.authorizeRelay(tenantID, pkt); err != nil {
		return err
	}
	if err := h.runGuard(ctx, tenantID, "release", pkt, dhcpv4RequestPhaseRequest); err != nil {
		return err
	}
	id := h.identity(pkt)
	scopeRef := lease.NewResourceScope(tenantID, tenantID)
	if _, _, err := h.leaseSvc.ReleaseByIdentifier(ctx, scopeRef, id); err != nil {
		return err
	}
	return h.leaseSvc.UpdateDHCPv4LeaseFSMByIdentifier(ctx, scopeRef, id, leasefsm.StateReleased)
}

func (h *Handler) observeLeaseOpDuration(op string, started time.Time) {
	if h == nil || h.dhcpMetrics == nil {
		return
	}
	h.dhcpMetrics.ObserveLeaseOperation(op, started)
}

func (h *Handler) leasePlan(ctx context.Context, tenantID, identifier string, pkt Packet, mdmManaged bool) (models.LeaseProfile, string) {
	relayInfo := pkt.RelayAgentInfo
	vlanID := parseVLAN(relayValue(pkt, "vlan-id", "agent.vlan-id"))
	userGroups := policy.DeriveUserGroups(pkt.UserClass, relayInfo)
	location := policy.DeriveLocation(relayInfo)
	ssid := policy.DeriveSSID(relayInfo)
	userID := policy.DeriveUserID(relayInfo)
	apID := policy.DeriveAccessPointID(relayInfo)
	controllerID := policy.DeriveControllerID(relayInfo)
	geoZone := policy.DeriveGeoZone(relayInfo)
	deviceType, devicePersona, deviceTags := h.deviceClassification(pkt)
	input := policy.Input{
		TenantID:      tenantID,
		MAC:           pkt.CHAddr.String(),
		ClientID:      pkt.ClientID,
		UserID:        userID,
		VendorClass:   pkt.VendorClass,
		UserClass:     pkt.UserClass,
		DeviceType:    deviceType,
		DevicePersona: devicePersona,
		DeviceTags:    deviceTags,
		MDMManaged:    mdmManaged,
		UserGroups:    userGroups,
		Location:      location,
		SSID:          ssid,
		VLANID:        vlanID,
		AccessPointID: apID,
		ControllerID:  controllerID,
		GeoZone:       geoZone,
		Option60:      pkt.VendorClass,
		Option82:      relayInfo,
		RelayInfo:     relayAttributes(pkt),
		IPv4Class:     pkt.IPClass,
		IsBOOTP:       pkt.IsBOOTP,
		Timestamp:     time.Now().UTC(),
	}
	decision, err := h.policy.Evaluate(ctx, input)
	if err != nil {
		h.logger.Debug("policy evaluation", zap.Error(err))
	}
	profile := models.LeaseProfile{}
	poolID := ""
	var selector *policy.PoolSelector
	if decision != nil {
		if decision.LeaseProfile.DefaultDuration > 0 || decision.LeaseProfile.MinDuration > 0 {
			profile = decision.LeaseProfile
		}
		if decision.PoolID != "" {
			poolID = decision.PoolID
		} else if decision.PoolSelector != nil {
			selector = decision.PoolSelector
		}
	}
	if poolID == "" && h.affinity != nil {
		if entry, ok := h.affinity.Lookup(ctx, tenantID, identifier); ok && entry.PoolID != "" {
			poolID = entry.PoolID
		}
	}
	if poolID == "" && selector != nil {
		if resolved := h.resolvePoolSelector(ctx, tenantID, selector); resolved != "" {
			poolID = resolved
		}
	}
	if poolID == "" && h.relaySel != nil {
		meta := relay.BuildMetadata(tenantID, pkt.GIAddr, relayInfo, vlanID, userGroups)
		if relayPool, ok := h.relaySel.Resolve(meta); ok {
			poolID = relayPool
		}
	}
	if poolID == "" {
		poolID = h.fallbackPool(pkt)
	}
	if poolID == "" {
		poolID = "default"
	}
	profile = h.applyPoolLeaseTimes(ctx, tenantID, poolID, profile)
	return profile, poolID
}

func (h *Handler) applyPoolLeaseTimes(ctx context.Context, tenantID, poolID string, profile models.LeaseProfile) models.LeaseProfile {
	if h == nil || h.poolSvc == nil {
		return profile
	}
	tenantID = strings.TrimSpace(tenantID)
	poolID = strings.TrimSpace(poolID)
	if tenantID == "" || poolID == "" {
		return profile
	}
	poolObj, err := h.poolSvc.GetPool(ctx, pool.NewResourceScope(tenantID, tenantID), poolID)
	if err != nil || poolObj == nil {
		return profile
	}
	if poolObj.MinLeaseTime > 0 {
		profile.DefaultDuration = time.Duration(poolObj.MinLeaseTime) * time.Second
	}
	if poolObj.MaxLeaseTime > 0 {
		profile.MaxDuration = time.Duration(poolObj.MaxLeaseTime) * time.Second
	}
	if profile.MaxDuration > 0 && profile.DefaultDuration > profile.MaxDuration {
		profile.DefaultDuration = profile.MaxDuration
	}
	return profile
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
	scopeRef := pool.NewResourceScope("", tenantID)
	poolObj, err := h.poolSvc.ResolvePool(ctx, scopeRef, metadataSelector)
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

func (h *Handler) fallbackPool(pkt Packet) string {
	switch pkt.IPClass {
	case "A":
		return "class-a"
	case "B":
		return "class-b"
	case "C":
		return "class-c"
	default:
		return "default"
	}
}

func (h *Handler) desiredIP(pkt Packet) string {
	if pkt.RequestedIP != nil {
		return pkt.RequestedIP.String()
	}
	if pkt.CIAddr != nil && !pkt.CIAddr.Equal(net.IPv4zero) {
		return pkt.CIAddr.String()
	}
	if pkt.GIAddr != nil && !pkt.GIAddr.Equal(net.IPv4zero) {
		return pkt.GIAddr.String()
	}
	return ""
}

func (h *Handler) identity(pkt Packet) string {
	if pkt.ClientID != "" {
		return pkt.ClientID
	}
	return pkt.CHAddr.String()
}

func (h *Handler) allocationMetadata(pkt Packet) lease.AllocationMetadata {
	relayInfo := pkt.RelayAgentInfo
	mobility := lease.MobilityMetadata{
		AnchorID:      policy.DeriveMobilityAnchorID(relayInfo),
		AccessPointID: policy.DeriveAccessPointID(relayInfo),
		ControllerID:  policy.DeriveControllerID(relayInfo),
		GeoZone:       policy.DeriveGeoZone(relayInfo),
		LocationHint:  policy.DeriveLocation(relayInfo),
	}
	session := make(map[string]any)
	if mobility.AnchorID != "" {
		session["anchorId"] = mobility.AnchorID
	}
	if requested := h.desiredIP(pkt); requested != "" {
		session["requestedIp"] = requested
	}
	if mobility.GeoZone != "" {
		session["geoZone"] = mobility.GeoZone
	}
	if mobility.ControllerID != "" {
		session["controllerId"] = mobility.ControllerID
	}
	if len(session) > 0 {
		mobility.SessionContinuity = session
	}
	return lease.AllocationMetadata{UserID: policy.DeriveUserID(relayInfo), Mobility: mobility}
}

func (h *Handler) deviceClassification(pkt Packet) (string, string, []string) {
	relayInfo := pkt.RelayAgentInfo
	deviceType := policy.ClassifyDeviceType(pkt.VendorClass, pkt.UserClass, relayInfo)
	if h.fingerprinter == nil {
		return deviceType, "", nil
	}
	signals := mobility.DeviceSignals{
		VendorClass: pkt.VendorClass,
		UserClass:   pkt.UserClass,
		RelayInfo:   relayInfo,
		MAC:         pkt.CHAddr,
		Option55:    parseOption55(pkt.Options[OptionParameterRequestList]),
	}
	if match, ok := h.fingerprinter.Match(signals); ok {
		persona := match.Persona
		if deviceType == "" && match.Platform != "" {
			deviceType = match.Platform
		}
		if match.Platform != "" {
			deviceType = match.Platform
		}
		return deviceType, persona, match.Tags
	}
	return deviceType, "", nil
}

func parseOption55(raw []byte) []int {
	if len(raw) == 0 {
		return nil
	}
	codes := make([]int, 0, len(raw))
	for _, b := range raw {
		codes = append(codes, int(b))
	}
	return codes
}

func (h *Handler) rememberMobilityAffinity(ctx context.Context, tenantID, identifier string, pkt Packet, meta lease.AllocationMetadata, res *lease.Result) {
	if h.affinity == nil || res == nil || res.Lease == nil {
		return
	}
	poolID := strings.TrimSpace(res.Lease.PoolID)
	if poolID == "" {
		return
	}
	coords := mobility.Coordinates{
		AnchorID:      meta.Mobility.AnchorID,
		AccessPointID: meta.Mobility.AccessPointID,
		ControllerID:  meta.Mobility.ControllerID,
		GeoZone:       meta.Mobility.GeoZone,
		Location:      meta.Mobility.LocationHint,
		VLANID:        parseVLAN(relayValue(pkt, "vlan-id", "agent.vlan-id")),
	}
	h.affinity.Remember(ctx, tenantID, identifier, mobility.Entry{PoolID: poolID, Coordinates: coords})
}

func (h *Handler) lookupComplianceMetadata(tenantID string, pkt Packet) lease.ComplianceMetadata {
	if h.mdmService == nil {
		return lease.ComplianceMetadata{}
	}
	deviceID := pkt.ClientID
	if deviceID == "" {
		deviceID = strings.ToLower(pkt.CHAddr.String())
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

func (h *Handler) observeLifecycle(tenantID, message string, started time.Time, err error) {
	success := err == nil
	duration := time.Since(started)
	if h.metrics != nil {
		outcome := "success"
		if !success {
			outcome = "failure"
		}
		h.metrics.DHCPRequestLifecycle.WithLabelValues(tenantID, "dhcpv4", message, outcome).Inc()
		h.metrics.DHCPRequestLatency.WithLabelValues(tenantID, "dhcpv4", message).Observe(duration.Seconds())
	}
	if h.recorder != nil {
		h.recorder.ObserveRequest(tenantID, "dhcpv4", message, success, duration)
	}
}

func (h *Handler) authorizeRelay(tenantID string, pkt Packet) error {
	if h.relayAuth == nil {
		return nil
	}
	vlanID := parseVLAN(relayValue(pkt, "vlan-id", "agent.vlan-id"))
	meta := relay.BuildMetadata(tenantID, pkt.GIAddr, pkt.RelayAgentInfo, vlanID, nil)
	if err := h.relayAuth.Authorize(meta); err != nil {
		h.logger.Warn("relay authentication failed", zap.String("tenantId", tenantID), zap.String("relayId", meta.RelayID), zap.Error(err))
		return err
	}
	return nil
}

func relayAttributes(pkt Packet) map[string]any {
	if len(pkt.RelayAgentInfo) == 0 && (pkt.GIAddr == nil || pkt.GIAddr.Equal(net.IPv4zero)) {
		return nil
	}
	attrs := make(map[string]any, len(pkt.RelayAgentInfo)+1)
	for k, v := range pkt.RelayAgentInfo {
		attrs[k] = v
	}
	if pkt.GIAddr != nil && !pkt.GIAddr.Equal(net.IPv4zero) {
		attrs["giaddr"] = pkt.GIAddr.String()
	}
	return attrs
}

func (h *Handler) runGuard(ctx context.Context, tenantID, messageType string, pkt Packet, phase string) error {
	if h.guard == nil {
		return nil
	}
	normalizedPhase := phase
	ctx = lease.WithTenantContext(ctx, tenantID)
	ctx = securityguard.WithRequestPhaseContext(ctx, phase)
	if normalizedPhase == "" {
		normalizedPhase = dhcpv4RequestPhaseRequest
	}
	if phase == dhcpv4RequestPhaseRenew || phase == dhcpv4RequestPhaseRebind {
		h.logger.Info("dhcpv4续约校验", zap.String("tenantId", tenantID), zap.String("phase", phase), zap.String("mac", pkt.CHAddr.String()))
		return h.guard.CheckRenewACL(ctx, pkt.CHAddr.String())
	}
	h.logger.Info("dhcpv4首次分配校验", zap.String("tenantId", tenantID), zap.String("phase", phase), zap.String("mac", pkt.CHAddr.String()))
	ctxData := &securityguard.Context{
		TenantID:     tenantID,
		MessageType:  messageType,
		RequestPhase: normalizedPhase,
		MAC:          pkt.CHAddr.String(),
		ClientID:     pkt.ClientID,
		CircuitID:    relayValue(pkt, "circuit-id", "agent.circuit-id"),
		RemoteID:     relayValue(pkt, "remote-id", "agent.remote-id"),
		RelayAgent:   pkt.RelayAgentInfo,
		PortID:       relayValue(pkt, "port-id", "circuit-id", "agent.circuit-id"),
		InterfaceID:  relayValue(pkt, "interface-id"),
		RequestedIP:  ipToString(pkt.RequestedIP),
		GIAddr:       ipToString(pkt.GIAddr),
		Timestamp:    time.Now().UTC(),
	}
	ctxData.VLANID = parseVLAN(relayValue(pkt, "vlan-id"))
	return h.guard.Check(ctx, ctxData)
}

func (h *Handler) emitLeaseChange(ctx context.Context, tenantID string, pkt Packet, res *lease.Result, action string) {
	if h.guard == nil || res == nil || res.Lease == nil {
		return
	}
	change := &securityguard.LeaseChange{
		TenantID:    tenantID,
		MACAddress:  pkt.CHAddr.String(),
		ClientID:    pkt.ClientID,
		IPAddress:   res.Lease.IPAddress,
		PoolID:      res.Lease.PoolID,
		PortID:      relayValue(pkt, "port-id", "circuit-id", "agent.circuit-id"),
		InterfaceID: poolInterface(res.Pool, pkt),
		ExpiresAt:   res.Lease.ExpiresAt,
		Action:      action,
		MDMManaged:  res.Lease.MDMManaged,
		MDMSource:   strings.TrimSpace(res.Lease.MDMSource),
		MDMTags:     decodeMDMTags(res.Lease.MDMTags),
	}
	if res.Lease.MDMObservedAt != nil {
		change.MDMObserved = res.Lease.MDMObservedAt
	}
	change.VLANID = poolVLAN(res.Pool)
	h.guard.OnLeaseChange(ctx, change)
}

func relayValue(pkt Packet, keys ...string) string {
	for _, key := range keys {
		if val, ok := pkt.RelayAgentInfo[key]; ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func parseVLAN(raw string) int {
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return v
}

func ipToString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func poolVLAN(pool *models.AddressPool) *int {
	if pool == nil || pool.VLANID == nil {
		return nil
	}
	v := *pool.VLANID
	return &v
}

func poolInterface(pool *models.AddressPool, pkt Packet) string {
	if pool != nil && pool.InterfaceID != nil {
		return strings.TrimSpace(*pool.InterfaceID)
	}
	return relayValue(pkt, "interface-id")
}

func decodeMDMTags(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	clean := make([]string, 0, len(tags))
	for _, tag := range tags {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean
}
