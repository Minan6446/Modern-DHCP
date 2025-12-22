package lease

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

// Err definitions for allocation flow.
var (
	ErrPoolRequired         = errors.New("lease: pool id required")
	ErrPoolReaderMissing    = errors.New("lease: pool reader missing")
	ErrNoAvailableIP        = errors.New("lease: no available addresses in pool")
	ErrUnsupportedCIDR      = errors.New("lease: invalid pool CIDR")
	ErrNoAvailablePrefix    = errors.New("lease: no available prefixes in pool")
	ErrInvalidSecurityState = errors.New("lease: invalid security state")
	ErrIdentifierLeaseLimit = errors.New("lease: max active leases reached for identifier")
	ErrUserLeaseLimit       = errors.New("lease: max active leases reached for user")
	ErrPoolProtection       = errors.New("lease: pool is in protection mode")
	ErrClientIsolated       = errors.New("lease: client isolated due to security policy")
	ErrTenantRequired       = errors.New("lease: tenant id required")
)

// ConflictProber checks whether an IP address already responds on the network.
type ConflictProber interface {
	Probe(ctx context.Context, addr netip.Addr) (bool, time.Duration, error)
}

const (
	leaseStateActive      = "ACTIVE"
	leaseStateDeclined    = "DECLINED"
	leaseStateReleased    = "RELEASED"
	leaseStateCooldown    = "COOLDOWN"
	leaseStateQuarantined = "QUARANTINED"
)

const (
	conflictSignalDHCPDecline  = "dhcp-decline"
	conflictSignalAdminDecline = "admin-decline"
	conflictSignalProbe        = "icmp-probe"
	auditActorSystem           = "system"
	auditActionLeaseConflict   = "lease.conflict_detected"
	historyDefaultLimit        = 100
	historyMaxLimit            = 1000
)

// PoolReader exposes read-only pool lookups used during allocation.
type PoolReader interface {
	GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error)
}

// ReplicationJournal ensures lease writes are durably persisted to the CDC/journal plane
// before ACKs are returned to clients.
type ReplicationJournal interface {
	ConfirmLease(ctx context.Context, lease *models.Lease) error
}

// Service coordinates lease lifecycle operations.
type Service struct {
	repo           Repository
	poolReader     PoolReader
	cfg            config.PolicyConfig
	exhaustion     config.ExhaustionConfig
	logger         *zap.Logger
	quota          tenant.QuotaEnforcer
	notifier       NotificationScheduler
	metrics        *metrics.Collector
	replication    ReplicationJournal
	auditSvc       *audit.Service
	rrMu           sync.Mutex
	rrCursor       map[string]uint32
	isolationMu    sync.Mutex
	isolation      map[string]*isolationEntry
	poolMu         sync.Mutex
	poolStates     map[string]poolThresholdState
	conflictProber ConflictProber
	conflictCfg    config.ConflictPreventionConfig
}

// ServiceOption configures optional lease service dependencies.
type ServiceOption func(*Service)

// WithNotificationScheduler wires a job scheduler for lease notifications.
func WithNotificationScheduler(scheduler NotificationScheduler) ServiceOption {
	return func(s *Service) {
		s.notifier = scheduler
	}
}

// WithMetricsCollector wires Prometheus metrics into the lease service.
func WithMetricsCollector(collector *metrics.Collector) ServiceOption {
	return func(s *Service) {
		s.metrics = collector
	}
}

// WithAuditService wires audit logging for lease lifecycle events.
func WithAuditService(svc *audit.Service) ServiceOption {
	return func(s *Service) {
		s.auditSvc = svc
	}
}

// WithQuotaEnforcer wires tenant quota enforcement into allocations.
func WithQuotaEnforcer(enforcer tenant.QuotaEnforcer) ServiceOption {
	return func(s *Service) {
		s.quota = enforcer
	}
}

// WithExhaustionConfig wires security exhaustion settings.
func WithExhaustionConfig(cfg config.ExhaustionConfig) ServiceOption {
	return func(s *Service) {
		s.exhaustion = cfg
	}
}

// WithReplicationJournal wires a dual-write journal that must confirm CDC ingestion
// before allocations are acknowledged to clients.
func WithReplicationJournal(journal ReplicationJournal) ServiceOption {
	return func(s *Service) {
		s.replication = journal
	}
}

// WithConflictPrevention wires an IP conflict prober into the lease service.
func WithConflictPrevention(cfg config.ConflictPreventionConfig, prober ConflictProber) ServiceOption {
	return func(s *Service) {
		s.conflictCfg = cfg
		s.conflictProber = prober
	}
}

// Result contains allocation decision metadata.
type Result struct {
	Lease             *models.Lease       `json:"lease"`
	Reused            bool                `json:"reused"`
	Profile           models.LeaseProfile `json:"profile"`
	RenewalTime       time.Duration       `json:"renewalTime"`
	RebindingTime     time.Duration       `json:"rebindingTime"`
	Pool              *models.AddressPool `json:"pool,omitempty"`
	SLAAC             *SLAACInfo          `json:"slaac,omitempty"`
	PrefixDelegations []PrefixDelegation  `json:"prefixDelegations,omitempty"`
	Notification      *LeaseNotification  `json:"notification,omitempty"`
}

// SLAACInfo surfaces stateless autoconfig details for DHCPv6 interop.
type SLAACInfo struct {
	Prefix            string        `json:"prefix"`
	Address           string        `json:"address,omitempty"`
	PreferredLifetime time.Duration `json:"preferredLifetime,omitempty"`
	ValidLifetime     time.Duration `json:"validLifetime,omitempty"`
	Mode              string        `json:"mode,omitempty"`
}

// PrefixDelegation tracks IA_PD metadata for clients that requested prefixes.
type PrefixDelegation struct {
	IAPDID            uint32        `json:"iapdId"`
	Prefix            string        `json:"prefix"`
	PrefixLength      byte          `json:"prefixLength"`
	PreferredLifetime time.Duration `json:"preferredLifetime"`
	ValidLifetime     time.Duration `json:"validLifetime"`
}

// LeaseNotification captures when to alert about an upcoming expiration.
type LeaseNotification struct {
	SendAfter time.Time     `json:"sendAfter"`
	Lead      time.Duration `json:"lead"`
}

// AllocationMetadata provides optional identity/context for exhaustion controls.
type AllocationMetadata struct {
	UserID     string
	Mobility   MobilityMetadata
	Compliance ComplianceMetadata
}

// MobilityMetadata captures roaming context passed from handlers.
type MobilityMetadata struct {
	AnchorID          string
	AccessPointID     string
	ControllerID      string
	GeoZone           string
	LocationHint      string
	SessionContinuity map[string]any
}

// ComplianceMetadata captures MDM insights at allocation time.
type ComplianceMetadata struct {
	Valid      bool
	Managed    bool
	Source     string
	Tags       []string
	ObservedAt time.Time
}

// NewService builds a lease service.
func NewService(repo Repository, poolReader PoolReader, cfg config.PolicyConfig, logger *zap.Logger, opts ...ServiceOption) *Service {
	svc := &Service{
		repo:       repo,
		poolReader: poolReader,
		cfg:        cfg,
		logger:     logger,
		rrCursor:   make(map[string]uint32),
		isolation:  make(map[string]*isolationEntry),
		poolStates: make(map[string]poolThresholdState),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// AllocateOrReuse either reuses an existing lease or creates a new one.
func (s *Service) AllocateOrReuse(ctx context.Context, tenantID, identifier string, profile models.LeaseProfile, poolID, ip string) (*Result, error) {
	return s.AllocateOrReuseWithMetadata(ctx, tenantID, identifier, profile, poolID, ip, AllocationMetadata{})
}

// History returns historical lease records for a tenant with optional filters.
func (s *Service) History(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) ([]models.Lease, int, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, ErrTenantRequired
	}
	if filter.Limit <= 0 {
		filter.Limit = historyDefaultLimit
	}
	if filter.Limit > historyMaxLimit {
		filter.Limit = historyMaxLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	records, err := s.repo.SearchLeaseHistory(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountLeaseHistory(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// AllocateOrReuseWithMetadata augments allocation with user context for exhaustion controls.
func (s *Service) AllocateOrReuseWithMetadata(ctx context.Context, tenantID, identifier string, profile models.LeaseProfile, poolID, ip string, meta AllocationMetadata) (*Result, error) {
	if err := s.checkIsolation(tenantID, identifier); err != nil {
		return nil, err
	}
	profile = s.normalizeProfile(profile)
	maxAttempts := s.conflictAttemptBudget()
	attempt := 0

allocate:
	attempt++
	if attempt > maxAttempts {
		return nil, ErrNoAvailableIP
	}
	existing, err := s.repo.GetActiveLease(ctx, tenantID, identifier)
	if err == nil {
		s.logger.Debug("reusing lease", zap.String("leaseId", existing.ID))
		pool, perr := s.ensurePool(ctx, tenantID, existing.PoolID)
		if perr != nil {
			return nil, perr
		}
		if meta.UserID != "" && existing.UserID != meta.UserID {
			existing.UserID = meta.UserID
		}
		s.applyMobilityMetadata(existing, meta)
		s.applyComplianceMetadata(existing, meta)
		s.applyLeaseTiming(existing, profile)
		if err := s.repo.UpdateLease(ctx, existing); err != nil {
			return nil, err
		}
		if err := s.confirmReplication(ctx, existing); err != nil {
			return nil, err
		}
		res := &Result{Lease: existing, Reused: true, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
		retry, err := s.enforceConflictPrevention(ctx, tenantID, identifier, res)
		if err != nil {
			return nil, err
		}
		if retry {
			goto allocate
		}
		res.Notification = s.computeNotification(existing, profile)
		if res.Notification != nil {
			s.enqueueNotificationJob(ctx, NotificationJob{
				LeaseID:   existing.ID,
				TenantID:  existing.TenantID,
				PoolID:    existing.PoolID,
				SendAfter: res.Notification.SendAfter,
				Lead:      res.Notification.Lead,
				Kind:      "address",
				Metadata:  map[string]string{"ip": existing.IPAddress},
			})
		}
		return res, nil
	}
	if err != nil && err != ErrNotFound {
		return nil, err
	}

	if err := s.enforceLeaseLimits(ctx, tenantID, identifier, meta); err != nil {
		return nil, err
	}
	if s.quota != nil {
		if err := s.quota.EnsureLeaseCapacity(ctx, tenantID); err != nil {
			return nil, err
		}
	}

	if poolID == "" {
		return nil, ErrPoolRequired
	}
	pool, err := s.ensurePool(ctx, tenantID, poolID)
	if err != nil {
		return nil, err
	}
	selectedIP, err := s.pickIPAddress(ctx, tenantID, pool, ip)
	if err != nil {
		if errors.Is(err, ErrNoAvailableIP) && s.conflictEnabled() {
			if reclaimed, ok, reclaimErr := s.tryReclaimConflictLease(ctx, tenantID, identifier, profile, pool, meta); reclaimErr != nil {
				return nil, reclaimErr
			} else if ok {
				return reclaimed, nil
			}
		}
		return nil, err
	}

	now := time.Now().UTC()
	lease := &models.Lease{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		PoolID:        poolID,
		IPAddress:     selectedIP,
		HardwareAddr:  identifier,
		ClientID:      identifier,
		UserID:        meta.UserID,
		ExpiresAt:     now,
		State:         leaseStateActive,
		SecurityState: models.SecurityStateOK,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.applyMobilityMetadata(lease, meta)
	s.applyComplianceMetadata(lease, meta)
	s.applyLeaseTiming(lease, profile)

	if err := s.repo.CreateLease(ctx, lease); err != nil {
		return nil, err
	}
	if err := s.confirmReplication(ctx, lease); err != nil {
		return nil, err
	}

	res := &Result{Lease: lease, Reused: false, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
	retry, err := s.enforceConflictPrevention(ctx, tenantID, identifier, res)
	if err != nil {
		return nil, err
	}
	if retry {
		goto allocate
	}
	s.logger.Info("allocated new lease", zap.String("leaseId", lease.ID), zap.String("ip", lease.IPAddress))
	res.Notification = s.computeNotification(lease, profile)
	if res.Notification != nil {
		s.enqueueNotificationJob(ctx, NotificationJob{
			LeaseID:   lease.ID,
			TenantID:  lease.TenantID,
			PoolID:    lease.PoolID,
			SendAfter: res.Notification.SendAfter,
			Lead:      res.Notification.Lead,
			Kind:      "address",
			Metadata:  map[string]string{"ip": lease.IPAddress},
		})
	}
	return res, nil
}

func (s *Service) confirmReplication(ctx context.Context, lease *models.Lease) error {
	if s.replication == nil {
		return nil
	}
	start := time.Now()
	if err := s.replication.ConfirmLease(ctx, lease); err != nil {
		return err
	}
	if s.metrics != nil && s.metrics.LeaseReplicationLag != nil {
		s.metrics.LeaseReplicationLag.WithLabelValues(lease.TenantID).Observe(time.Since(start).Seconds())
	}
	return nil
}

func (s *Service) ensurePool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error) {
	if s.poolReader == nil {
		return nil, ErrPoolReaderMissing
	}
	return s.poolReader.GetPool(ctx, tenantID, poolID)
}

func (s *Service) pickIPAddress(ctx context.Context, tenantID string, pool *models.AddressPool, preferred string) (string, error) {
	if pool == nil {
		return "", ErrPoolRequired
	}
	if s.poolReader == nil {
		return "", ErrPoolReaderMissing
	}
	prefix, err := netip.ParsePrefix(pool.CIDR)
	if err != nil {
		return "", ErrUnsupportedCIDR
	}
	usedIPs, err := s.repo.ListActiveIPs(ctx, tenantID, pool.ID)
	if err != nil {
		return "", err
	}
	usedSet := make(map[string]struct{}, len(usedIPs))
	for _, addr := range usedIPs {
		usedSet[addr] = struct{}{}
	}
	cooldownSet := make(map[string]struct{})
	if cooldownIPs, err := s.repo.ListCooldownIPs(ctx, tenantID, pool.ID, time.Now().UTC()); err == nil {
		for _, addr := range cooldownIPs {
			cooldownSet[addr] = struct{}{}
		}
	} else {
		return "", err
	}
	rangeStart, rangeEnd := s.resolvePoolBounds(pool, prefix)
	if err := s.evaluatePoolThresholds(pool, len(usedSet), len(cooldownSet), rangeStart, rangeEnd); err != nil {
		return "", err
	}
	mode := s.normalizeAllocationMode(pool)
	var (
		ipv4Ex []ipv4Range
		ipv6Ex []ipv6Range
	)
	if prefix.Addr().Is4() {
		ipv4Ex = s.buildIPv4Exclusions(pool, prefix, rangeStart, rangeEnd)
	} else {
		ipv6Ex = s.buildIPv6Exclusions(pool, prefix, rangeStart, rangeEnd)
	}
	if preferred != "" {
		if addr, err := netip.ParseAddr(preferred); err == nil && prefix.Contains(addr) && isUsableHost(prefix, addr) && withinBounds(addr, rangeStart, rangeEnd) && !isExcluded(addr, ipv4Ex, ipv6Ex) {
			candidate := addr.String()
			if _, blocked := cooldownSet[candidate]; blocked {
				// remain under cooldown, pick another
			} else if _, exists := usedSet[candidate]; !exists {
				return candidate, nil
			}
		}
	}
	if prefix.Addr().Is4() {
		start32 := addrToUint32(rangeStart)
		end32 := addrToUint32(rangeEnd)
		switch mode {
		case models.AllocationModeRoundRobin:
			if candidate, ok := s.nextAvailableIPv4RoundRobin(pool.ID, start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex); ok {
				return candidate, nil
			}
		case models.AllocationModePriorityWeighted:
			if candidate, ok := nextAvailableIPv4Priority(start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex, s.normalizePriorityWeight(pool)); ok {
				return candidate, nil
			}
		}
		if candidate, ok := nextAvailableIPv4Sequential(start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex); ok {
			return candidate, nil
		}
		return "", ErrNoAvailableIP
	}
	if candidate, ok := nextAvailableIPv6(rangeStart, rangeEnd, usedSet, cooldownSet, pool.ReservePercent, ipv6Ex); ok {
		return candidate, nil
	}
	return "", ErrNoAvailableIP
}

// MarkDeclined marks an active lease as declined/conflicted.

func (s *Service) MarkDeclined(ctx context.Context, tenantID, identifier, reason string) error {
	lease, err := s.repo.GetActiveLease(ctx, tenantID, identifier)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	now := time.Now().UTC()
	lease.UpdatedAt = now
	conflictCount := s.appendConflict(lease, conflictSignalDHCPDecline, reason)
	s.recordConflictMetric(tenantID, conflictSignalDHCPDecline)
	s.recordConflictAudit(ctx, lease, conflictSignalDHCPDecline, reason, conflictCount)
	s.applyCooldownState(lease, conflictCount, now)
	return s.repo.UpdateLease(ctx, lease)
}

// ReleaseLease marks a lease as released by administrators.
func (s *Service) ReleaseLease(ctx context.Context, tenantID, leaseID string) (*models.Lease, bool, error) {
	lease, err := s.repo.GetLeaseByID(ctx, tenantID, leaseID)
	if err != nil {
		return nil, false, err
	}
	if lease.State == leaseStateReleased {
		return lease, false, nil
	}
	now := time.Now().UTC()
	lease.State = leaseStateReleased
	lease.ExpiresAt = now
	lease.UpdatedAt = now
	lease.CooldownUntil = nil
	if err := s.repo.UpdateLease(ctx, lease); err != nil {
		return nil, false, err
	}
	return lease, true, nil
}

// DeclineLease marks a lease as declined/conflicted by administrators.
func (s *Service) DeclineLease(ctx context.Context, tenantID, leaseID string) (*models.Lease, bool, error) {
	lease, err := s.repo.GetLeaseByID(ctx, tenantID, leaseID)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	lease.ExpiresAt = now
	lease.UpdatedAt = now
	conflictCount := s.appendConflict(lease, conflictSignalAdminDecline, "")
	s.recordConflictMetric(tenantID, conflictSignalAdminDecline)
	s.recordConflictAudit(ctx, lease, conflictSignalAdminDecline, "", conflictCount)
	s.applyCooldownState(lease, conflictCount, now)
	if err := s.repo.UpdateLease(ctx, lease); err != nil {
		return nil, false, err
	}
	return lease, true, nil
}

// ClearCooldown removes cooldown/quarantine state so the lease can be reissued.
func (s *Service) ClearCooldown(ctx context.Context, tenantID, leaseID string) (*models.Lease, bool, error) {
	lease, err := s.repo.GetLeaseByID(ctx, tenantID, leaseID)
	if err != nil {
		return nil, false, err
	}
	if lease.State != leaseStateCooldown && lease.State != leaseStateQuarantined && lease.CooldownUntil == nil {
		return lease, false, nil
	}
	now := time.Now().UTC()
	lease.State = leaseStateReleased
	lease.CooldownUntil = nil
	lease.UpdatedAt = now
	lease.ExpiresAt = now
	if err := s.repo.UpdateLease(ctx, lease); err != nil {
		return nil, false, err
	}
	return lease, true, nil
}

// ReleasePrefix releases a delegated prefix for a client/IAPD combination.
func (s *Service) ReleasePrefix(ctx context.Context, tenantID, clientID string, iapdID uint32) error {
	return s.updatePrefixState(ctx, tenantID, clientID, iapdID, leaseStateReleased)
}

// DeclinePrefix marks a delegated prefix as declined/conflicted.
func (s *Service) DeclinePrefix(ctx context.Context, tenantID, clientID string, iapdID uint32) error {
	return s.updatePrefixState(ctx, tenantID, clientID, iapdID, leaseStateDeclined)
}

func isUsableHost(prefix netip.Prefix, addr netip.Addr) bool {
	if !prefix.Contains(addr) {
		return false
	}
	if addr.Is4() {
		hostBits := 32 - prefix.Bits()
		if hostBits <= 1 {
			return true
		}
		network := prefix.Masked().Addr()
		start := addrToUint32(network)
		value := addrToUint32(addr)
		if value == start {
			return false
		}
		broadcast := start + uint32((1<<hostBits)-1)
		return value != broadcast
	}
	return true
}

func nextAvailableIPv4Sequential(start, end uint32, used map[string]struct{}, cooldown map[string]struct{}, reservePercent int, exclusions []ipv4Range) (string, bool) {
	allocEnd, ok := applyReserveWindow(start, end, reservePercent)
	if !ok {
		return "", false
	}
	return scanIPv4Range(start, allocEnd, used, cooldown, exclusions)
}

func nextAvailableIPv4Priority(start, end uint32, used map[string]struct{}, cooldown map[string]struct{}, reservePercent int, exclusions []ipv4Range, weight int) (string, bool) {
	allocEnd, ok := applyReserveWindow(start, end, reservePercent)
	if !ok {
		return "", false
	}
	preferredEnd := allocEnd
	if weight > 0 && weight < 100 {
		span := int64(allocEnd-start) + 1
		preferred := (span * int64(weight)) / 100
		if preferred <= 0 {
			preferred = 1
		}
		preferredEnd = start + uint32(preferred-1)
		if preferredEnd > allocEnd {
			preferredEnd = allocEnd
		}
	}
	if ip, ok := scanIPv4Range(start, preferredEnd, used, cooldown, exclusions); ok {
		return ip, true
	}
	if preferredEnd < allocEnd {
		return scanIPv4Range(preferredEnd+1, allocEnd, used, cooldown, exclusions)
	}
	return "", false
}

func (s *Service) nextAvailableIPv4RoundRobin(poolID string, start, end uint32, used map[string]struct{}, cooldown map[string]struct{}, reservePercent int, exclusions []ipv4Range) (string, bool) {
	allocEnd, ok := applyReserveWindow(start, end, reservePercent)
	if !ok {
		return "", false
	}
	window := int64(allocEnd-start) + 1
	if window <= 0 {
		return "", false
	}
	cursor := s.roundRobinCursor(poolID, start, allocEnd)
	processed := int64(0)
	value := cursor
	for processed < window {
		value = incrementIPv4(value, start, allocEnd)
		processed++
		if isIPv4Excluded(value, exclusions) {
			continue
		}
		ip := uint32ToAddr(value).String()
		if _, blocked := cooldown[ip]; blocked {
			continue
		}
		if _, exists := used[ip]; exists {
			continue
		}
		s.storeRoundRobinCursor(poolID, value)
		return ip, true
	}
	return "", false
}

func scanIPv4Range(start, end uint32, used map[string]struct{}, cooldown map[string]struct{}, exclusions []ipv4Range) (string, bool) {
	if end < start {
		return "", false
	}
	for candidate := start; candidate <= end; candidate++ {
		if isIPv4Excluded(candidate, exclusions) {
			continue
		}
		ip := uint32ToAddr(candidate).String()
		if _, blocked := cooldown[ip]; blocked {
			continue
		}
		if _, exists := used[ip]; exists {
			continue
		}
		return ip, true
	}
	return "", false
}

func applyReserveWindow(start, end uint32, reservePercent int) (uint32, bool) {
	if end < start {
		return 0, false
	}
	span := int64(end-start) + 1
	if span <= 0 {
		return 0, false
	}
	alloc := span
	if reservePercent > 0 {
		if reservePercent >= 100 {
			return 0, false
		}
		reserved := (span * int64(reservePercent)) / 100
		if reserved >= alloc {
			return 0, false
		}
		alloc -= reserved
	}
	allocEnd := start + uint32(alloc-1)
	if allocEnd > end {
		allocEnd = end
	}
	return allocEnd, true
}

func incrementIPv4(current, start, end uint32) uint32 {
	if current < start || current > end {
		return start
	}
	if current == end {
		return start
	}
	return current + 1
}

func (s *Service) roundRobinCursor(poolID string, start, end uint32) uint32 {
	s.rrMu.Lock()
	defer s.rrMu.Unlock()
	value, ok := s.rrCursor[poolID]
	if !ok || value < start || value > end {
		return end
	}
	return value
}

func (s *Service) storeRoundRobinCursor(poolID string, value uint32) {
	s.rrMu.Lock()
	s.rrCursor[poolID] = value
	s.rrMu.Unlock()
}

func (s *Service) normalizeAllocationMode(pool *models.AddressPool) string {
	if pool == nil {
		return models.AllocationModeSequential
	}
	mode := strings.TrimSpace(strings.ToUpper(pool.AllocationMode))
	switch mode {
	case models.AllocationModeRoundRobin, models.AllocationModePriorityWeighted:
		return mode
	default:
		return models.AllocationModeSequential
	}
}

func (s *Service) normalizePriorityWeight(pool *models.AddressPool) int {
	if pool == nil || pool.PriorityWeight <= 0 {
		return 50
	}
	if pool.PriorityWeight > 100 {
		return 100
	}
	return pool.PriorityWeight
}

func nextAvailableIPv6(start netip.Addr, end netip.Addr, used map[string]struct{}, cooldown map[string]struct{}, reservePercent int, exclusions []ipv6Range) (string, bool) {
	startInt := netutil.AddrToBig(start)
	endInt := netutil.AddrToBig(end)
	span := new(big.Int).Sub(endInt, startInt)
	span.Add(span, big.NewInt(1))
	if span.Sign() <= 0 {
		return "", false
	}
	usable := new(big.Int).Set(span)
	if reservePercent > 0 && reservePercent < 100 {
		reserved := new(big.Int).Mul(span, big.NewInt(int64(reservePercent)))
		reserved.Div(reserved, big.NewInt(100))
		if reserved.Cmp(usable) >= 0 {
			return "", false
		}
		usable.Sub(usable, reserved)
	}
	if usable.Sign() <= 0 {
		return "", false
	}
	for attempt := 0; attempt < 2048; attempt++ {
		offset, err := rand.Int(rand.Reader, usable)
		if err != nil {
			return "", false
		}
		candidateInt := new(big.Int).Add(startInt, offset)
		if candidateInt.Cmp(endInt) > 0 {
			continue
		}
		if isIPv6Excluded(candidateInt, exclusions) {
			continue
		}
		addr, ok := netutil.BigToAddr(candidateInt)
		if !ok {
			continue
		}
		ip := addr.String()
		if _, blocked := cooldown[ip]; blocked {
			continue
		}
		if _, exists := used[ip]; exists {
			continue
		}
		return ip, true
	}
	return "", false
}

func addrToUint32(addr netip.Addr) uint32 {
	a := addr.Unmap()
	if !a.Is4() {
		return 0
	}
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func uint32ToAddr(v uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{
		byte(v >> 24),
		byte(v >> 16),
		byte(v >> 8),
		byte(v),
	})
}

type ipv4Range struct {
	start uint32
	end   uint32
}

type ipv6Range struct {
	start *big.Int
	end   *big.Int
}

type conflictEntry struct {
	Timestamp time.Time `json:"ts"`
	Signal    string    `json:"signal,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

func (s *Service) appendConflict(lease *models.Lease, signal, reason string) int {
	if lease == nil {
		return 0
	}
	entry := conflictEntry{Timestamp: time.Now().UTC(), Signal: signal, Reason: reason}
	var history []conflictEntry
	if len(lease.ConflictHistory) > 0 {
		if err := json.Unmarshal(lease.ConflictHistory, &history); err != nil {
			s.logger.Warn("failed to unmarshal conflict history", zap.String("leaseId", lease.ID), zap.Error(err))
		}
	}
	history = append(history, entry)
	const maxConflictEntries = 20
	if len(history) > maxConflictEntries {
		history = history[len(history)-maxConflictEntries:]
	}
	data, err := json.Marshal(history)
	if err != nil {
		s.logger.Warn("failed to marshal conflict history", zap.String("leaseId", lease.ID), zap.Error(err))
		return len(history)
	}
	lease.ConflictHistory = data
	return len(history)
}

func (s *Service) recordConflictMetric(tenantID, signal string) {
	if s.metrics == nil || s.metrics.LeaseConflicts == nil {
		return
	}
	if tenantID == "" {
		tenantID = "unknown"
	}
	if signal == "" {
		signal = "unspecified"
	}
	s.metrics.LeaseConflicts.WithLabelValues(tenantID, signal).Inc()
}

func (s *Service) recordConflictAudit(ctx context.Context, lease *models.Lease, signal, reason string, conflictCount int) {
	if s.auditSvc == nil || lease == nil {
		return
	}
	payload := auditpayload.LeaseConflict{
		LeaseID:       lease.ID,
		PoolID:        lease.PoolID,
		TenantID:      lease.TenantID,
		IPAddress:     lease.IPAddress,
		Signal:        signal,
		Reason:        reason,
		Identifier:    lease.HardwareAddr,
		ClientID:      lease.ClientID,
		ConflictCount: conflictCount,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to marshal conflict audit payload", zap.String("leaseId", lease.ID), zap.Error(err))
		}
		return
	}
	if err := s.auditSvc.RecordEvent(ctx, audit.RecordEventRequest{
		TenantID: lease.TenantID,
		Actor:    auditActorSystem,
		Action:   auditActionLeaseConflict,
		Payload:  data,
	}); err != nil && s.logger != nil {
		s.logger.Warn("failed to persist conflict audit", zap.String("leaseId", lease.ID), zap.Error(err))
	}
}

func (s *Service) resolvePoolBounds(pool *models.AddressPool, prefix netip.Prefix) (netip.Addr, netip.Addr) {
	start, end, err := netutil.DefaultHostRange(prefix)
	if err != nil {
		s.logger.Warn("failed to compute default pool range", zap.Error(err))
		return prefix.Masked().Addr(), prefix.Masked().Addr()
	}
	if pool == nil {
		return start, end
	}
	if pool.RangeStart != "" {
		if addr, err := netip.ParseAddr(pool.RangeStart); err == nil && prefix.Contains(addr) {
			start = addr
		} else if pool.RangeStart != "" {
			s.logger.Debug("invalid pool rangeStart", zap.String("poolId", pool.ID), zap.String("value", pool.RangeStart))
		}
	}
	if pool.RangeEnd != "" {
		if addr, err := netip.ParseAddr(pool.RangeEnd); err == nil && prefix.Contains(addr) {
			end = addr
		} else if pool.RangeEnd != "" {
			s.logger.Debug("invalid pool rangeEnd", zap.String("poolId", pool.ID), zap.String("value", pool.RangeEnd))
		}
	}
	if start.Compare(end) > 0 {
		fallbackStart, fallbackEnd, err := netutil.DefaultHostRange(prefix)
		if err == nil {
			start, end = fallbackStart, fallbackEnd
		}
	}
	if !start.IsValid() || !end.IsValid() {
		start = prefix.Masked().Addr()
		end = prefix.Masked().Addr()
	}
	if prefix.Addr().Is4() {
		start = start.Unmap()
		end = end.Unmap()
		if !start.Is4() {
			start = prefix.Masked().Addr().Unmap()
		}
		if !end.Is4() {
			end = prefix.Masked().Addr().Unmap()
		}
	}
	return start, end
}

func (s *Service) buildIPv4Exclusions(pool *models.AddressPool, prefix netip.Prefix, start, end netip.Addr) []ipv4Range {
	if pool == nil || len(pool.Exclusions) == 0 || !prefix.Addr().Is4() {
		return nil
	}
	startBound := addrToUint32(start)
	endBound := addrToUint32(end)
	ranges := make([]ipv4Range, 0, len(pool.Exclusions))
	for _, ex := range pool.Exclusions {
		sAddr, err1 := netip.ParseAddr(ex.Start)
		eAddr, err2 := netip.ParseAddr(ex.End)
		if err1 != nil || err2 != nil || !sAddr.Is4() || !eAddr.Is4() {
			s.logger.Debug("skipping invalid IPv4 exclusion", zap.String("poolId", pool.ID), zap.String("range", ex.Start+"-"+ex.End))
			continue
		}
		if !prefix.Contains(sAddr) || !prefix.Contains(eAddr) {
			continue
		}
		sVal := addrToUint32(sAddr)
		eVal := addrToUint32(eAddr)
		if eVal < startBound || sVal > endBound {
			continue
		}
		if sVal < startBound {
			sVal = startBound
		}
		if eVal > endBound {
			eVal = endBound
		}
		ranges = append(ranges, ipv4Range{start: sVal, end: eVal})
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})
	return ranges
}

func (s *Service) buildIPv6Exclusions(pool *models.AddressPool, prefix netip.Prefix, start, end netip.Addr) []ipv6Range {
	if pool == nil || len(pool.Exclusions) == 0 || prefix.Addr().Is4() {
		return nil
	}
	startBound := netutil.AddrToBig(start)
	endBound := netutil.AddrToBig(end)
	ranges := make([]ipv6Range, 0, len(pool.Exclusions))
	for _, ex := range pool.Exclusions {
		sAddr, err1 := netip.ParseAddr(ex.Start)
		eAddr, err2 := netip.ParseAddr(ex.End)
		if err1 != nil || err2 != nil || sAddr.Is4() || eAddr.Is4() {
			s.logger.Debug("skipping invalid IPv6 exclusion", zap.String("poolId", pool.ID), zap.String("range", ex.Start+"-"+ex.End))
			continue
		}
		if !prefix.Contains(sAddr) || !prefix.Contains(eAddr) {
			continue
		}
		sVal := netutil.AddrToBig(sAddr)
		eVal := netutil.AddrToBig(eAddr)
		if eVal.Cmp(startBound) < 0 || sVal.Cmp(endBound) > 0 {
			continue
		}
		if sVal.Cmp(startBound) < 0 {
			sVal = new(big.Int).Set(startBound)
		} else {
			sVal = new(big.Int).Set(sVal)
		}
		if eVal.Cmp(endBound) > 0 {
			eVal = new(big.Int).Set(endBound)
		} else {
			eVal = new(big.Int).Set(eVal)
		}
		ranges = append(ranges, ipv6Range{start: sVal, end: eVal})
	}
	sort.Slice(ranges, func(i, j int) bool {
		cmp := ranges[i].start.Cmp(ranges[j].start)
		if cmp == 0 {
			return ranges[i].end.Cmp(ranges[j].end) < 0
		}
		return cmp < 0
	})
	return ranges
}

func withinBounds(addr, start, end netip.Addr) bool {
	return addr.Compare(start) >= 0 && addr.Compare(end) <= 0
}

func isExcluded(addr netip.Addr, v4 []ipv4Range, v6 []ipv6Range) bool {
	if addr.Is4() {
		return isIPv4Excluded(addrToUint32(addr), v4)
	}
	return isIPv6Excluded(netutil.AddrToBig(addr), v6)
}

func isIPv4Excluded(value uint32, ranges []ipv4Range) bool {
	for _, r := range ranges {
		if value < r.start {
			return false
		}
		if value >= r.start && value <= r.end {
			return true
		}
	}
	return false
}

func isIPv6Excluded(value *big.Int, ranges []ipv6Range) bool {
	for _, r := range ranges {
		if value.Cmp(r.start) < 0 {
			return false
		}
		if value.Cmp(r.start) >= 0 && value.Cmp(r.end) <= 0 {
			return true
		}
	}
	return false
}

func (s *Service) normalizeProfile(profile models.LeaseProfile) models.LeaseProfile {
	defaults := s.cfg.DefaultLeaseProfile
	if profile.DefaultDuration == 0 {
		profile.DefaultDuration = defaults.DefaultDuration
	}
	if profile.MinDuration == 0 {
		profile.MinDuration = defaults.MinDuration
	}
	if profile.MaxDuration == 0 {
		profile.MaxDuration = defaults.MaxDuration
	}
	if profile.NotificationLead == 0 {
		profile.NotificationLead = defaults.NotificationLead
	}
	if profile.Infinite || defaults.Permanent {
		profile.Infinite = true
	}
	if profile.DefaultDuration <= 0 && !profile.Infinite {
		profile.DefaultDuration = time.Hour
	}
	if profile.MinDuration > 0 && profile.DefaultDuration < profile.MinDuration {
		profile.DefaultDuration = profile.MinDuration
	}
	if profile.MaxDuration > 0 && profile.DefaultDuration > profile.MaxDuration {
		profile.DefaultDuration = profile.MaxDuration
	}
	if profile.RenewalTime == 0 {
		if defaults.RenewalPercent > 0 {
			profile.RenewalTime = durationPercent(profile.DefaultDuration, defaults.RenewalPercent)
		} else if profile.DefaultDuration > 0 {
			profile.RenewalTime = profile.DefaultDuration / 2
		}
	}
	if profile.RebindingTime == 0 {
		if defaults.RebindingPercent > 0 {
			profile.RebindingTime = durationPercent(profile.DefaultDuration, defaults.RebindingPercent)
		} else if profile.DefaultDuration > 0 {
			profile.RebindingTime = time.Duration(float64(profile.DefaultDuration) * 0.875)
		}
	}
	if profile.RebindingTime > 0 && profile.RenewalTime > profile.RebindingTime {
		profile.RenewalTime = profile.RebindingTime / 2
	}
	if profile.SleepyCapable || defaults.SleepyCapable {
		if !profile.SleepyCapable {
			profile.SleepyCapable = defaults.SleepyCapable
		}
		if profile.SleepyOfflineWindow == 0 {
			profile.SleepyOfflineWindow = defaults.SleepyOfflineWindow
		}
		if profile.SleepyHoldDuration == 0 {
			profile.SleepyHoldDuration = defaults.SleepyHoldDuration
		}
	}
	if profile.MobilityGracePeriod == 0 {
		profile.MobilityGracePeriod = defaults.MobilityGracePeriod
	}
	if profile.Infinite {
		profile.RenewalTime = 0
		profile.RebindingTime = 0
	}
	return profile
}

func (s *Service) enforceLeaseLimits(ctx context.Context, tenantID, identifier string, meta AllocationMetadata) error {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return nil
	}
	if limit := s.exhaustion.MaxLeasesPerMAC; limit > 0 {
		count, err := s.repo.CountActiveLeasesByIdentifier(ctx, tenantID, identifier)
		if err != nil {
			return err
		}
		if count >= limit {
			s.applyIsolationPolicy(tenantID, identifier, "mac_limit")
			s.recordLeaseMetric("identifier_limit")
			return ErrIdentifierLeaseLimit
		}
	}
	if meta.UserID != "" && s.exhaustion.MaxLeasesPerUser > 0 {
		count, err := s.repo.CountActiveLeasesByUser(ctx, tenantID, meta.UserID)
		if err != nil {
			return err
		}
		if count >= s.exhaustion.MaxLeasesPerUser {
			s.applyIsolationPolicy(tenantID, identifier, "user_limit")
			s.recordLeaseMetric("user_limit")
			return ErrUserLeaseLimit
		}
	}
	return nil
}

func (s *Service) evaluatePoolThresholds(pool *models.AddressPool, activeCount, cooldownCount int, start, end netip.Addr) error {
	if pool == nil || !start.IsValid() || !end.IsValid() || !start.Is4() || !end.Is4() {
		return nil
	}
	capacity := capacityIPv4(start, end)
	if capacity <= 0 {
		return nil
	}
	consumed := activeCount + cooldownCount
	percent := (float64(consumed) / float64(capacity)) * 100
	warn := s.exhaustion.PoolWarnPercent
	protect := s.exhaustion.PoolProtectPercent
	level := poolLevelNormal
	if protect > 0 && percent >= float64(protect) {
		level = poolLevelProtect
	} else if warn > 0 && percent >= float64(warn) {
		level = poolLevelWarn
	}
	s.updatePoolLevel(pool.ID, level, percent)
	if level == poolLevelProtect {
		s.recordLeaseMetric("pool_protection")
		return ErrPoolProtection
	}
	return nil
}

func (s *Service) updatePoolLevel(poolID, level string, percent float64) {
	if poolID == "" {
		return
	}
	s.poolMu.Lock()
	prev := s.poolStates[poolID]
	if prev.Level == level {
		s.poolMu.Unlock()
		return
	}
	s.poolStates[poolID] = poolThresholdState{Level: level}
	s.poolMu.Unlock()
	switch level {
	case poolLevelWarn:
		if s.logger != nil {
			s.logger.Warn("address pool nearing capacity", zap.String("poolId", poolID), zap.Float64("utilization", percent))
		}
	case poolLevelProtect:
		if s.logger != nil {
			s.logger.Error("address pool entered protection mode", zap.String("poolId", poolID), zap.Float64("utilization", percent))
		}
	case poolLevelNormal:
		if s.logger != nil {
			s.logger.Info("address pool recovered", zap.String("poolId", poolID), zap.Float64("utilization", percent))
		}
	}
}

func capacityIPv4(start, end netip.Addr) int64 {
	start32 := int64(addrToUint32(start))
	end32 := int64(addrToUint32(end))
	if end32 < start32 {
		return 0
	}
	return (end32 - start32) + 1
}

func (s *Service) checkIsolation(tenantID, identifier string) error {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return nil
	}
	s.isolationMu.Lock()
	defer s.isolationMu.Unlock()
	entry, ok := s.isolation[s.isolationKey(tenantID, identifier)]
	if !ok {
		return nil
	}
	if !entry.Permanent && !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt) {
		delete(s.isolation, s.isolationKey(tenantID, identifier))
		return nil
	}
	return ErrClientIsolated
}

func (s *Service) applyIsolationPolicy(tenantID, identifier, reason string) {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return
	}
	isoCfg := s.exhaustion.Isolation
	if isoCfg.TemporaryDuration <= 0 && isoCfg.PermanentAfter <= 0 {
		return
	}
	key := s.isolationKey(tenantID, identifier)
	s.isolationMu.Lock()
	entry := s.isolation[key]
	if entry == nil {
		entry = &isolationEntry{}
		s.isolation[key] = entry
	}
	entry.Count++
	entry.Reason = reason
	permanent := isoCfg.PermanentAfter > 0 && entry.Count >= isoCfg.PermanentAfter
	if permanent {
		entry.Permanent = true
		entry.ExpiresAt = time.Time{}
	} else {
		dur := isoCfg.TemporaryDuration
		if dur <= 0 {
			dur = 5 * time.Minute
		}
		entry.ExpiresAt = time.Now().UTC().Add(dur)
		entry.Permanent = false
	}
	s.isolationMu.Unlock()
	if s.logger != nil {
		fields := []zap.Field{
			zap.String("tenantId", tenantID),
			zap.String("identifier", identifier),
			zap.String("reason", reason),
			zap.Bool("permanent", permanent),
			zap.Int("violations", entry.Count),
		}
		if !permanent {
			fields = append(fields, zap.Time("expiresAt", entry.ExpiresAt))
		}
		s.logger.Warn("client isolated due to exhaustion policy", fields...)
	}
	s.recordLeaseMetric("client_isolated")
}

func (s *Service) isolationKey(tenantID, identifier string) string {
	return tenantID + "|" + identifier
}

func (s *Service) recordLeaseMetric(action string) {
	if action == "" || s.metrics == nil || s.metrics.LeaseEvents == nil {
		return
	}
	s.metrics.LeaseEvents.WithLabelValues(action).Inc()
}

type isolationEntry struct {
	Permanent bool
	ExpiresAt time.Time
	Count     int
	Reason    string
}

type poolThresholdState struct {
	Level string
}

const (
	poolLevelNormal  = "normal"
	poolLevelWarn    = "warn"
	poolLevelProtect = "protect"
)

func (s *Service) applyLeaseTiming(lease *models.Lease, profile models.LeaseProfile) {
	now := time.Now().UTC()
	lease.UpdatedAt = now
	if profile.Infinite {
		lease.ExpiresAt = now.Add(24 * time.Hour * 365 * 100)
		return
	}
	lease.ExpiresAt = now.Add(profile.DefaultDuration)
}

func (s *Service) applyMobilityMetadata(lease *models.Lease, meta AllocationMetadata) {
	if lease == nil {
		return
	}
	mob := meta.Mobility
	if mob.AnchorID != "" {
		lease.MobilityAnchorID = mob.AnchorID
	}
	if mob.AccessPointID != "" {
		lease.LastAccessPointID = mob.AccessPointID
	}
	if mob.ControllerID != "" {
		lease.LastControllerID = mob.ControllerID
	}
	if mob.GeoZone != "" {
		lease.LastGeoZone = mob.GeoZone
	}
	if mob.LocationHint != "" {
		lease.MobilityLocationHint = mob.LocationHint
	}
	if len(mob.SessionContinuity) > 0 {
		payload, err := json.Marshal(mob.SessionContinuity)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to marshal session continuity", zap.Error(err), zap.String("leaseId", lease.ID))
			}
		} else {
			lease.SessionContinuity = payload
		}
	}
}

func (s *Service) applyComplianceMetadata(lease *models.Lease, meta AllocationMetadata) {
	if lease == nil || !meta.Compliance.Valid {
		return
	}
	comp := meta.Compliance
	lease.MDMManaged = comp.Managed
	lease.MDMSource = comp.Source
	lease.MDMTags = nil
	if len(comp.Tags) > 0 {
		payload, err := json.Marshal(comp.Tags)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("failed to marshal compliance tags", zap.Error(err), zap.String("leaseId", lease.ID))
			}
		} else {
			lease.MDMTags = payload
		}
	}
	if !comp.ObservedAt.IsZero() {
		t := comp.ObservedAt.UTC()
		lease.MDMObservedAt = &t
	} else {
		lease.MDMObservedAt = nil
	}
}

func durationPercent(base time.Duration, pct float64) time.Duration {
	if base <= 0 || pct <= 0 {
		return 0
	}
	return time.Duration(float64(base) * (pct / 100.0))
}

func (s *Service) computeNotification(lease *models.Lease, profile models.LeaseProfile) *LeaseNotification {
	if lease == nil || profile.NotificationLead <= 0 {
		return nil
	}
	notifyAt := lease.ExpiresAt.Add(-profile.NotificationLead)
	now := time.Now().UTC()
	if notifyAt.Before(now) {
		notifyAt = now
	}
	s.logger.Debug("lease notification scheduled", zap.String("leaseId", lease.ID), zap.Time("notifyAt", notifyAt))
	return &LeaseNotification{SendAfter: notifyAt, Lead: profile.NotificationLead}
}

func (s *Service) enqueueNotificationJob(ctx context.Context, job NotificationJob) {
	if s.notifier == nil || job.SendAfter.IsZero() {
		return
	}
	if err := s.notifier.ScheduleLeaseNotification(ctx, job); err != nil {
		s.logger.Warn("failed to enqueue lease notification", zap.Error(err), zap.String("leaseId", job.LeaseID))
	}
}

func (s *Service) updatePrefixState(ctx context.Context, tenantID, clientID string, iapdID uint32, state string) error {
	lease, err := s.repo.GetActivePrefixLease(ctx, tenantID, clientID, iapdID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	now := time.Now().UTC()
	lease.State = state
	lease.UpdatedAt = now
	lease.ExpiresAt = now
	return s.repo.UpdatePrefixLease(ctx, lease)
}

// ReleasePrefixByID releases a delegated prefix via its lease ID.
func (s *Service) ReleasePrefixByID(ctx context.Context, tenantID, prefixLeaseID string) (*models.PrefixLease, bool, error) {
	return s.updatePrefixStateByID(ctx, tenantID, prefixLeaseID, leaseStateReleased)
}

// DeclinePrefixByID marks a delegated prefix as declined via its lease ID.
func (s *Service) DeclinePrefixByID(ctx context.Context, tenantID, prefixLeaseID string) (*models.PrefixLease, bool, error) {
	return s.updatePrefixStateByID(ctx, tenantID, prefixLeaseID, leaseStateDeclined)
}

// ListPrefixLeases exposes prefix delegation search for HTTP handlers.
func (s *Service) ListPrefixLeases(ctx context.Context, tenantID, state string, limit, offset int) ([]models.PrefixLease, error) {
	return s.repo.ListPrefixLeases(ctx, tenantID, state, limit, offset)
}

// ListLeases exposes repository search for HTTP handlers.
func (s *Service) ListLeases(ctx context.Context, tenantID, state string, limit, offset int) ([]models.Lease, error) {
	return s.repo.ListLeases(ctx, tenantID, state, limit, offset)
}

// CountActiveLeasesByPool returns utilization counters keyed by pool ID.
func (s *Service) CountActiveLeasesByPool(ctx context.Context, tenantID string, poolIDs []string) (map[string]int64, error) {
	if s.repo == nil {
		return nil, errors.New("lease: repository unavailable")
	}
	return s.repo.CountActiveLeasesByPool(ctx, tenantID, poolIDs)
}

// UpdateSecurityState persists the security posture for a lease identifier.

// UpdateSecurityState persists the security posture for a lease identifier (MAC/client-id).
func (s *Service) UpdateSecurityState(ctx context.Context, tenantID, identifier, state string) error {
	if tenantID == "" || identifier == "" {
		return errors.New("lease: tenant and identifier required")
	}
	normalized, err := normalizeSecurityState(state)
	if err != nil {
		return err
	}
	return s.repo.UpdateSecurityState(ctx, tenantID, identifier, normalized, time.Now().UTC())
}

// UpdateLeaseSecurityState sets the security posture for a specific lease ID.
func (s *Service) UpdateLeaseSecurityState(ctx context.Context, tenantID, leaseID, state string) (*models.Lease, string, bool, error) {
	if tenantID == "" || leaseID == "" {
		return nil, "", false, errors.New("lease: tenant and lease id required")
	}
	normalized, err := normalizeSecurityState(state)
	if err != nil {
		return nil, "", false, err
	}
	lease, err := s.repo.GetLeaseByID(ctx, tenantID, leaseID)
	if err != nil {
		return nil, "", false, err
	}
	previous := lease.SecurityState
	if previous == normalized {
		return lease, previous, false, nil
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateSecurityStateByID(ctx, tenantID, leaseID, normalized, now); err != nil {
		return nil, "", false, err
	}
	lease.SecurityState = normalized
	lease.UpdatedAt = now
	return lease, previous, true, nil
}

func (s *Service) updatePrefixStateByID(ctx context.Context, tenantID, prefixLeaseID string, state string) (*models.PrefixLease, bool, error) {
	lease, err := s.repo.GetPrefixLeaseByID(ctx, tenantID, prefixLeaseID)
	if err != nil {
		return nil, false, err
	}
	if lease.State == state {
		return lease, false, nil
	}
	now := time.Now().UTC()
	lease.State = state
	lease.UpdatedAt = now
	lease.ExpiresAt = now
	if err := s.repo.UpdatePrefixLease(ctx, lease); err != nil {
		return nil, false, err
	}
	return lease, true, nil
}

func (s *Service) applyCooldownState(lease *models.Lease, conflictCount int, now time.Time) {
	if lease == nil {
		return
	}
	if s.shouldQuarantine(conflictCount) {
		lease.State = leaseStateQuarantined
		lease.CooldownUntil = nil
		return
	}
	lease.State = leaseStateCooldown
	if dur := s.cooldownDuration(); dur > 0 {
		deadline := now.Add(dur)
		lease.CooldownUntil = &deadline
	} else {
		lease.CooldownUntil = nil
	}
}

func (s *Service) shouldQuarantine(conflicts int) bool {
	max := s.cfg.Lifecycle.MaxCooldowns
	return max > 0 && conflicts >= max
}

func (s *Service) cooldownDuration() time.Duration {
	if dur := s.cfg.Lifecycle.CooldownDuration; dur > 0 {
		return dur
	}
	if fallback := s.cfg.DefaultLeaseProfile.NotificationLead; fallback > 0 {
		return fallback
	}
	return 5 * time.Minute
}

func normalizeSecurityState(state string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(state))
	if value == "" {
		return models.SecurityStateOK, nil
	}
	switch value {
	case models.SecurityStateOK, models.SecurityStateSuspect, models.SecurityStateBlocked:
		return value, nil
	default:
		return "", ErrInvalidSecurityState
	}
}

func (s *Service) conflictEnabled() bool {
	return s.conflictProber != nil && s.conflictCfg.Enabled
}

func (s *Service) conflictAttemptBudget() int {
	if !s.conflictEnabled() {
		return 1
	}
	if s.conflictCfg.MaxAttempts > 1 {
		return s.conflictCfg.MaxAttempts
	}
	return 3
}

func (s *Service) conflictProbeTimeout() time.Duration {
	if s.conflictCfg.ProbeTimeout > 0 {
		return s.conflictCfg.ProbeTimeout
	}
	return time.Second
}

func (s *Service) conflictHoldDuration() time.Duration {
	if s.conflictCfg.HoldDuration > 0 {
		return s.conflictCfg.HoldDuration
	}
	return 5 * time.Second
}

func (s *Service) conflictScanLimit() int {
	if s.conflictCfg.ReclaimScanLimit > 0 {
		return s.conflictCfg.ReclaimScanLimit
	}
	return 1
}

func (s *Service) enforceConflictPrevention(ctx context.Context, tenantID, identifier string, res *Result) (bool, error) {
	if !s.conflictEnabled() || res == nil || res.Lease == nil {
		return false, nil
	}
	ip := strings.TrimSpace(res.Lease.IPAddress)
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is4() {
		return false, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, s.conflictProbeTimeout())
	defer cancel()
	alive, _, probeErr := s.conflictProber.Probe(probeCtx, addr)
	if probeErr != nil {
		if s.logger != nil {
			s.logger.Warn("icmp probe failed", zap.String("ip", ip), zap.Error(probeErr))
		}
		return false, nil
	}
	if !alive {
		return false, nil
	}
	now := time.Now().UTC()
	res.Lease.UpdatedAt = now
	res.Lease.ExpiresAt = now
	count := s.appendConflict(res.Lease, conflictSignalProbe, "icmp echo response")
	s.recordConflictMetric(tenantID, conflictSignalProbe)
	s.recordConflictAudit(ctx, res.Lease, conflictSignalProbe, "icmp echo response", count)
	s.applyCooldownState(res.Lease, count, now)
	if hold := s.conflictHoldDuration(); hold > 0 && res.Lease.State == leaseStateCooldown {
		deadline := now.Add(hold)
		res.Lease.CooldownUntil = &deadline
	}
	if err := s.repo.UpdateLease(ctx, res.Lease); err != nil {
		return false, err
	}
	if err := s.confirmReplication(ctx, res.Lease); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) tryReclaimConflictLease(ctx context.Context, tenantID, identifier string, profile models.LeaseProfile, pool *models.AddressPool, meta AllocationMetadata) (*Result, bool, error) {
	if !s.conflictEnabled() || pool == nil {
		return nil, false, nil
	}
	limit := s.conflictScanLimit()
	candidates, err := s.repo.ListLeasesByState(ctx, tenantID, pool.ID, leaseStateCooldown, limit)
	if err != nil {
		return nil, false, err
	}
	for i := range candidates {
		leaseCopy := candidates[i]
		if !s.isProbeConflict(&leaseCopy) {
			continue
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(leaseCopy.IPAddress))
		if err != nil || !addr.Is4() {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, s.conflictProbeTimeout())
		alive, _, probeErr := s.conflictProber.Probe(probeCtx, addr)
		cancel()
		if probeErr != nil {
			if s.logger != nil {
				s.logger.Warn("reclaim probe failed", zap.String("ip", leaseCopy.IPAddress), zap.Error(probeErr))
			}
			continue
		}
		if alive {
			s.refreshProbeCooldown(&leaseCopy)
			if err := s.repo.UpdateLease(ctx, &leaseCopy); err != nil && s.logger != nil {
				s.logger.Warn("extend cooldown failed", zap.String("leaseId", leaseCopy.ID), zap.Error(err))
			}
			continue
		}
		s.resetConflictHistory(&leaseCopy)
		leaseCopy.HardwareAddr = identifier
		leaseCopy.ClientID = identifier
		leaseCopy.UserID = meta.UserID
		leaseCopy.State = leaseStateActive
		leaseCopy.CooldownUntil = nil
		leaseCopy.UpdatedAt = time.Now().UTC()
		leaseCopy.ExpiresAt = leaseCopy.UpdatedAt
		s.applyMobilityMetadata(&leaseCopy, meta)
		s.applyComplianceMetadata(&leaseCopy, meta)
		s.applyLeaseTiming(&leaseCopy, profile)
		if err := s.repo.UpdateLease(ctx, &leaseCopy); err != nil {
			return nil, false, err
		}
		if err := s.confirmReplication(ctx, &leaseCopy); err != nil {
			return nil, false, err
		}
		res := &Result{Lease: &leaseCopy, Reused: false, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
		res.Notification = s.computeNotification(&leaseCopy, profile)
		if res.Notification != nil {
			s.enqueueNotificationJob(ctx, NotificationJob{
				LeaseID:   leaseCopy.ID,
				TenantID:  leaseCopy.TenantID,
				PoolID:    leaseCopy.PoolID,
				SendAfter: res.Notification.SendAfter,
				Lead:      res.Notification.Lead,
				Kind:      "address",
				Metadata:  map[string]string{"ip": leaseCopy.IPAddress},
			})
		}
		if s.logger != nil {
			s.logger.Info("reclaimed abandoned lease", zap.String("ip", leaseCopy.IPAddress), zap.String("poolId", leaseCopy.PoolID))
		}
		return res, true, nil
	}
	return nil, false, nil
}

func (s *Service) isProbeConflict(lease *models.Lease) bool {
	history := s.decodeConflictHistory(lease)
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Signal == conflictSignalProbe {
			return true
		}
	}
	return false
}

func (s *Service) decodeConflictHistory(lease *models.Lease) []conflictEntry {
	if lease == nil || len(lease.ConflictHistory) == 0 {
		return nil
	}
	var history []conflictEntry
	if err := json.Unmarshal(lease.ConflictHistory, &history); err != nil {
		if s.logger != nil {
			s.logger.Debug("decode conflict history", zap.String("leaseId", lease.ID), zap.Error(err))
		}
		return nil
	}
	return history
}

func (s *Service) refreshProbeCooldown(lease *models.Lease) {
	if lease == nil {
		return
	}
	lease.UpdatedAt = time.Now().UTC()
	if hold := s.conflictHoldDuration(); hold > 0 {
		deadline := lease.UpdatedAt.Add(hold)
		lease.CooldownUntil = &deadline
	}
}

func (s *Service) resetConflictHistory(lease *models.Lease) {
	if lease != nil {
		lease.ConflictHistory = nil
	}
}
