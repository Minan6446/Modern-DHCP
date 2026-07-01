package lease

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/dhcpv4/lock"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/pool"
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
	ErrPrimaryRequired      = errors.New("lease: write requires primary role")
)

// ConflictProber checks whether an IP address already responds on the network.
type ConflictProber interface {
	ProbeARP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error)
	ProbeICMP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error)
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
	GetPool(ctx context.Context, scope pool.ResourceScope, poolID string) (*models.AddressPool, error)
	FindBinding(ctx context.Context, scope pool.ResourceScope, identifier, ip string) (*models.StaticBinding, error)
	ListBindingsByPool(ctx context.Context, scope pool.ResourceScope, poolID string) ([]models.StaticBinding, error)
}

// ReplicationJournal ensures lease writes are durably persisted to the CDC/journal plane
// before ACKs are returned to clients.
type ReplicationJournal interface {
	ConfirmLease(ctx context.Context, lease *models.Lease) error
}

// SyncAckGate blocks response paths until peer synchronization ACK is confirmed.
type SyncAckGate interface {
	AwaitLeaseSyncAck(ctx context.Context, lease *models.Lease) error
}

// FailoverStatusReporter provides HA role snapshots for write guard checks.
type FailoverStatusReporter interface {
	Snapshot() failover.StatusSnapshot
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
	syncAckGate    SyncAckGate
	syncAckPolicy  string
	failoverStatus FailoverStatusReporter
	auditSvc       *audit.Service
	rrMu           sync.Mutex
	rrCursor       map[string]uint32
	isolationMu    sync.Mutex
	isolation      map[string]*isolationEntry
	poolMu         sync.Mutex
	poolStates     map[string]poolThresholdState
	conflictProber ConflictProber
	conflictCfg    config.ConflictPreventionConfig
	redisClient    redis.UniversalClient
	allocLock      *lock.Manager
	bitmapFirst    bool
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

// WithSyncAckGate wires peer ACK confirmation into lease write paths.
func WithSyncAckGate(gate SyncAckGate) ServiceOption {
	return func(s *Service) {
		s.syncAckGate = gate
	}
}

// WithSyncAckPolicy controls behavior when peer ACK confirmation fails.
// Supported values: strict, degraded.
func WithSyncAckPolicy(policy string) ServiceOption {
	return func(s *Service) {
		s.syncAckPolicy = strings.ToLower(strings.TrimSpace(policy))
	}
}

// WithFailoverStatusReporter wires HA snapshot reporting for primary write guard.
func WithFailoverStatusReporter(reporter FailoverStatusReporter) ServiceOption {
	return func(s *Service) {
		s.failoverStatus = reporter
	}
}

// WithConflictPrevention wires an IP conflict prober into the lease service.
func WithConflictPrevention(cfg config.ConflictPreventionConfig, prober ConflictProber) ServiceOption {
	return func(s *Service) {
		s.conflictCfg = cfg
		s.conflictProber = prober
	}
}

// WithRedisAllocator enables Redis lock and bitmap acceleration for IPv4 allocation.
func WithRedisAllocator(client redis.UniversalClient) ServiceOption {
	return func(s *Service) {
		s.redisClient = client
		s.allocLock = lock.NewManager(client)
	}
}

// WithBitmapFirstAllocation skips ListActiveIPs DB query and uses Redis bitmap as
// the primary allocator. Falls back to DB-based sequential scan if bitmap fails.
// Only effective when Redis allocator is also enabled.
func WithBitmapFirstAllocation(enabled bool) ServiceOption {
	return func(s *Service) {
		s.bitmapFirst = enabled
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
		repo:          repo,
		poolReader:    poolReader,
		cfg:           cfg,
		logger:        logger,
		syncAckPolicy: "strict",
		rrCursor:      make(map[string]uint32),
		isolation:     make(map[string]*isolationEntry),
		poolStates:    make(map[string]poolThresholdState),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func (s *Service) tenantFromScope(scope ResourceScope) (string, error) {
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	if tenantID == "" {
		return "", ErrTenantRequired
	}
	return tenantID, nil
}

// HasValidLease reports whether the given MAC currently has an unexpired active lease.
func (s *Service) HasValidLease(ctx context.Context, mac string) (bool, error) {
	identifier := strings.TrimSpace(mac)
	if identifier == "" {
		return false, nil
	}
	tenantID := TenantIDFromContext(ctx)
	if tenantID == "" {
		return false, nil
	}
	scopeRef := NewResourceScope(tenantID, tenantID)
	leaseRecord, err := s.repo.GetActiveLease(ctx, scopeRef.AccessScope(), identifier)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if leaseRecord == nil {
		return false, nil
	}
	return leaseRecord.ExpiresAt.After(time.Now().UTC()), nil
}

func (s *Service) ensurePrimaryWrite(op string) error {
	if s == nil || s.failoverStatus == nil {
		return nil
	}
	snap := s.failoverStatus.Snapshot()
	if snap.Role == failover.RolePrimary || snap.Role == failover.RoleUnknown {
		return nil
	}
	return fmt.Errorf("%w: operation=%s role=%s state=%s", ErrPrimaryRequired, strings.TrimSpace(op), snap.Role, snap.State)
}

// AllocateOrReuse either reuses an existing lease or creates a new one.
func (s *Service) AllocateOrReuse(ctx context.Context, scope ResourceScope, identifier string, profile models.LeaseProfile, poolID, ip string) (*Result, error) {
	return s.AllocateOrReuseWithMetadata(ctx, scope, identifier, profile, poolID, ip, AllocationMetadata{})
}

// History returns historical lease records for a tenant with optional filters.
func (s *Service) History(ctx context.Context, scope ResourceScope, filter models.LeaseHistoryFilter) ([]models.Lease, int, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, 0, err
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
	records, total, err := s.repo.SearchLeaseHistory(ctx, scope.AccessScope(), filter)
	if err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// AllocateOrReuseWithMetadata augments allocation with user context for exhaustion controls.
func (s *Service) AllocateOrReuseWithMetadata(ctx context.Context, scope ResourceScope, identifier string, profile models.LeaseProfile, poolID, ip string, meta AllocationMetadata) (*Result, error) {
	tenantID, err := s.tenantFromScope(scope)
	if err != nil {
		return nil, err
	}
	if err := s.ensurePrimaryWrite("lease.allocate_or_reuse"); err != nil {
		return nil, err
	}
	access := scope.AccessScope()
	if err := s.checkIsolation(tenantID, identifier); err != nil {
		return nil, err
	}
	profile = s.normalizeProfile(profile)
	maxAttempts := s.conflictAttemptBudget()

	for attempt := 0; attempt < maxAttempts; attempt++ {
	existing, err := s.repo.GetActiveLease(ctx, access, identifier)
	if err == nil {
		s.logger.Debug("reusing lease", zap.String("leaseId", existing.ID))
		pool, perr := s.ensurePool(ctx, scope, existing.PoolID)
		if perr != nil {
			return nil, perr
		}
		if meta.UserID != "" && existing.UserID != meta.UserID {
			existing.UserID = meta.UserID
		}
		s.applyMobilityMetadata(existing, meta)
		s.applyComplianceMetadata(existing, meta)
		s.applyLeaseTiming(existing, profile)
		_ = s.bitmapSetByIP(ctx, pool, existing.IPAddress, false)
		if err := s.persistLeaseUpdate(ctx, existing, pool); err != nil {
			return nil, err
		}
		s.recordBindingSeen(ctx, scope, identifier, existing.IPAddress)
		res := &Result{Lease: existing, Reused: true, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
		retry, err := s.enforceConflictPrevention(ctx, tenantID, identifier, res)
		if err != nil {
			return nil, err
		}
		if retry {
			continue
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
	if errors.Is(err, ErrNotFound) {
		// proceed to new allocation
	} else {
		return nil, err
	}

	if err := s.enforceLeaseLimits(ctx, scope, tenantID, identifier, meta); err != nil {
		return nil, err
	}
	if s.quota != nil {
		if err := s.quota.EnsureLeaseCapacity(ctx, tenantID); err != nil {
			return nil, err
		}
	}

	var binding *models.StaticBinding
	bindingMatch := false
	if candidate, match, err := s.lookupBinding(ctx, scope, identifier, ip); err != nil {
		return nil, err
	} else {
		binding = candidate
		bindingMatch = match
		if binding != nil && bindingMatch {
			poolID = binding.PoolID
			if binding.IPAddress != "" {
				ip = binding.IPAddress
			}
		}
	}

	if poolID == "" {
		return nil, ErrPoolRequired
	}
	pool, err := s.ensurePool(ctx, scope, poolID)
	if err != nil {
		return nil, err
	}
	poolLock, err := s.acquirePoolLock(ctx, pool.ID)
	if err != nil {
		return nil, err
	}
	defer poolLock.Unlock(context.Background())
	reserved, err := s.bindingsForPool(ctx, scope, pool.ID)
	if err != nil {
		return nil, err
	}
	bindingForAllocation := binding
	if !bindingMatch {
		bindingForAllocation = nil
	}
	selectedIP, err := s.pickIPAddress(ctx, scope, pool, ip, bindingForAllocation, reserved)
	if err != nil {
		if errors.Is(err, ErrNoAvailableIP) && s.conflictEnabled() {
			if reclaimed, ok, reclaimErr := s.tryReclaimConflictLease(ctx, scope, identifier, profile, pool, meta); reclaimErr != nil {
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

	_ = s.bitmapSetByIP(ctx, pool, lease.IPAddress, false)
	if err := s.persistLeaseCreate(ctx, lease, pool); err != nil {
		return nil, err
	}
	s.recordBindingSeen(ctx, scope, identifier, lease.IPAddress)

	res := &Result{Lease: lease, Reused: false, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
	retry, err := s.enforceConflictPrevention(ctx, tenantID, identifier, res)
	if err != nil {
		return nil, err
	}
	if retry {
		continue
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
return nil, ErrNoAvailableIP
}

func (s *Service) recordBindingSeen(ctx context.Context, scope ResourceScope, identifier, ip string) {
	if s.poolReader == nil {
		return
	}
	writer, ok := s.poolReader.(interface {
		RecordBindingSeen(ctx context.Context, scope pool.ResourceScope, identifier, ip, source string, seenAt time.Time) error
	})
	if !ok {
		return
	}
	if strings.TrimSpace(identifier) == "" && strings.TrimSpace(ip) == "" {
		return
	}
	_ = writer.RecordBindingSeen(ctx, pool.ResourceScopeFromAccess(scope.AccessScope()), identifier, ip, "dhcp", time.Now().UTC())
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

func (s *Service) confirmSyncAck(ctx context.Context, lease *models.Lease) error {
	if s.syncAckGate == nil || lease == nil {
		return nil
	}
	policy := strings.ToLower(strings.TrimSpace(s.syncAckPolicy))
	if policy == "" {
		policy = "strict"
	}
	if err := s.syncAckGate.AwaitLeaseSyncAck(ctx, lease); err != nil {
		reason := classifySyncAckError(err)
		if s.metrics != nil && s.metrics.LeaseSyncAckFailures != nil {
			s.metrics.LeaseSyncAckFailures.WithLabelValues(strings.TrimSpace(lease.TenantID), policy, reason).Inc()
		}
		if policy == "degraded" {
			if s.logger != nil {
				s.logger.Warn("lease sync ack gate degraded", zap.String("leaseId", lease.ID), zap.String("reason", reason), zap.Error(err))
			}
			return nil
		}
		return fmt.Errorf("sync ack gate (%s): %w", reason, err)
	}
	return nil
}

func classifySyncAckError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return "unknown"
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return "ack_timeout"
	}
	if strings.Contains(msg, "refused") {
		return "ack_conn_refused"
	}
	if strings.Contains(msg, "reject") {
		return "ack_rejected"
	}
	if strings.Contains(msg, "mismatch") || strings.Contains(msg, "invalid") || strings.Contains(msg, "decode") {
		return "ack_protocol_error"
	}
	return "ack_io_error"
}

func (s *Service) ensurePool(ctx context.Context, scope ResourceScope, poolID string) (*models.AddressPool, error) {
	if s.poolReader == nil {
		return nil, ErrPoolReaderMissing
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	poolScope := pool.ResourceScopeFromAccess(scope.AccessScope())
	return s.poolReader.GetPool(ctx, poolScope, poolID)
}

func (s *Service) lookupBinding(ctx context.Context, scope ResourceScope, identifier, requestedIP string) (*models.StaticBinding, bool, error) {
	if s.poolReader == nil {
		return nil, false, nil
	}
	if strings.TrimSpace(identifier) == "" && strings.TrimSpace(requestedIP) == "" {
		return nil, false, nil
	}
	pBinding, err := s.poolReader.FindBinding(ctx, pool.ResourceScopeFromAccess(scope.AccessScope()), identifier, requestedIP)
	if err != nil {
		if errors.Is(err, pool.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	matched := false
	if pBinding != nil && pBinding.Identifier != "" {
		matched = strings.EqualFold(pBinding.Identifier, identifier)
	}
	return pBinding, matched, nil
}

func (s *Service) bindingsForPool(ctx context.Context, scope ResourceScope, poolID string) ([]models.StaticBinding, error) {
	if s.poolReader == nil || poolID == "" {
		return nil, nil
	}
	bindings, err := s.poolReader.ListBindingsByPool(ctx, pool.ResourceScopeFromAccess(scope.AccessScope()), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return bindings, nil
}

func (s *Service) pickIPAddress(ctx context.Context, scope ResourceScope, pool *models.AddressPool, preferred string, binding *models.StaticBinding, reserved []models.StaticBinding) (string, error) {
	if pool == nil {
		return "", ErrPoolRequired
	}
	if s.poolReader == nil {
		return "", ErrPoolReaderMissing
	}
	if _, err := scope.TenantIDOrErr(); err != nil {
		return "", err
	}
	prefix, err := netip.ParsePrefix(pool.CIDR)
	if err != nil {
		return "", ErrUnsupportedCIDR
	}
	usedIPs, err := s.repo.ListActiveIPs(ctx, scope.AccessScope(), pool.ID)
	if err != nil {
		return "", err
	}
	usedSet := make(map[string]struct{}, len(usedIPs))
	for _, addr := range usedIPs {
		usedSet[addr] = struct{}{}
	}
	// When bitmap-first is active, use Redis bitmap as the primary allocator and
	// skip the DB-based usedSet. If bitmap succeeds we avoid the N+1 query entirely.
	// If bitmap fails, we fall through to the sequential path below.
	useBitmapFirst := s.bitmapEnabled() && s.bitmapFirst && prefix.Addr().Is4()
	if useBitmapFirst {
		usedSet = make(map[string]struct{})
	}
	cooldownSet := make(map[string]struct{})
	if cooldownIPs, err := s.repo.ListCooldownIPs(ctx, scope.AccessScope(), pool.ID, time.Now().UTC()); err == nil {
		for _, addr := range cooldownIPs {
			cooldownSet[addr] = struct{}{}
		}
	} else {
		return "", err
	}
	reservedSet := make(map[string]struct{})
	if len(reserved) > 0 {
		for _, b := range reserved {
			if binding != nil && b.ID == binding.ID {
				continue
			}
			if b.IPAddress == "" {
				continue
			}
			reservedSet[b.IPAddress] = struct{}{}
			usedSet[b.IPAddress] = struct{}{}
		}
	}
	rangeStart, rangeEnd := s.resolvePoolBounds(pool, prefix)
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
	if binding != nil && binding.IPAddress != "" {
		if addr, err := netip.ParseAddr(binding.IPAddress); err == nil && prefix.Contains(addr) && isUsableHost(prefix, addr) && withinBounds(addr, rangeStart, rangeEnd) && !isExcluded(addr, ipv4Ex, ipv6Ex) {
			candidate := addr.String()
			if _, blocked := cooldownSet[candidate]; blocked {
				return "", ErrNoAvailableIP
			}
			if _, taken := usedSet[candidate]; !taken {
				ok, acdErr := s.acdCheckCandidate(ctx, pool.ID, candidate)
				if acdErr != nil {
					return "", acdErr
				}
				if ok {
					return candidate, nil
				}
				usedSet[candidate] = struct{}{}
			}
			return "", ErrNoAvailableIP
		}
	}
	if preferred != "" {
		if addr, err := netip.ParseAddr(preferred); err == nil && prefix.Contains(addr) && isUsableHost(prefix, addr) && withinBounds(addr, rangeStart, rangeEnd) && !isExcluded(addr, ipv4Ex, ipv6Ex) {
			candidate := addr.String()
			if _, blocked := cooldownSet[candidate]; blocked {
				// remain under cooldown, pick another
			} else if _, exists := usedSet[candidate]; !exists {
				ok, acdErr := s.acdCheckCandidate(ctx, pool.ID, candidate)
				if acdErr != nil {
					return "", acdErr
				}
				if ok {
					return candidate, nil
				}
				usedSet[candidate] = struct{}{}
			}
		}
	}
	if err := s.evaluatePoolThresholds(pool, len(usedSet), len(cooldownSet), rangeStart, rangeEnd); err != nil {
		return "", err
	}
	if prefix.Addr().Is4() {
		start32 := addrToUint32(rangeStart)
		end32 := addrToUint32(rangeEnd)
		if candidate, ok := s.bitmapFindAvailableIPv4(ctx, pool, rangeStart, rangeEnd, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex); ok {
			ok, acdErr := s.acdCheckCandidate(ctx, pool.ID, candidate)
			if acdErr != nil {
				return "", acdErr
			}
			if ok {
				return candidate, nil
			}
		}
		// Bitmap returned no candidate. If bitmap-first was active, lazily load
		// usedSet from DB so the sequential fallback has accurate data.
		if useBitmapFirst {
			usedIPs, loadErr := s.repo.ListActiveIPs(ctx, scope.AccessScope(), pool.ID)
			if loadErr != nil {
				return "", loadErr
			}
			usedSet = make(map[string]struct{}, len(usedIPs))
			for _, addr := range usedIPs {
				usedSet[addr] = struct{}{}
			}
			// Re-check pool thresholds with real data
			if err := s.evaluatePoolThresholds(pool, len(usedSet), len(cooldownSet), rangeStart, rangeEnd); err != nil {
				return "", err
			}
		}
		for attempt := 0; attempt < s.conflictAttemptBudget()*8; attempt++ {
			candidate := ""
			found := false
			switch mode {
			case models.AllocationModeRoundRobin:
				candidate, found = s.nextAvailableIPv4RoundRobin(pool.ID, start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex)
			case models.AllocationModePriorityWeighted:
				candidate, found = nextAvailableIPv4Priority(start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex, s.normalizePriorityWeight(pool))
			default:
				candidate, found = nextAvailableIPv4Sequential(start32, end32, usedSet, cooldownSet, pool.ReservePercent, ipv4Ex)
			}
			if !found || candidate == "" {
				break
			}
			ok, acdErr := s.acdCheckCandidate(ctx, pool.ID, candidate)
			if acdErr != nil {
				return "", acdErr
			}
			if ok {
				return candidate, nil
			}
			usedSet[candidate] = struct{}{}
			cooldownSet[candidate] = struct{}{}
		}
		return "", ErrNoAvailableIP
	}
	if candidate, ok := nextAvailableIPv6(rangeStart, rangeEnd, usedSet, cooldownSet, pool.ReservePercent, ipv6Ex); ok {
		return candidate, nil
	}
	return "", ErrNoAvailableIP
}

// MarkDeclined marks an active lease as declined/conflicted.

func (s *Service) MarkDeclined(ctx context.Context, scope ResourceScope, identifier, reason string) error {
	tenantID, err := s.tenantFromScope(scope)
	if err != nil {
		return err
	}
	if err := s.ensurePrimaryWrite("lease.mark_declined"); err != nil {
		return err
	}
	lease, err := s.repo.GetActiveLease(ctx, scope.AccessScope(), identifier)
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
func (s *Service) ReleaseLease(ctx context.Context, scope ResourceScope, leaseID string) (*models.Lease, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, false, err
	}
	if err := s.ensurePrimaryWrite("lease.release"); err != nil {
		return nil, false, err
	}
	lease, err := s.repo.GetLeaseByID(ctx, scope.AccessScope(), leaseID)
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
	if poolObj, pErr := s.ensurePool(ctx, scope, lease.PoolID); pErr == nil {
		_ = s.bitmapSetByIP(ctx, poolObj, lease.IPAddress, true)
	}
	return lease, true, nil
}

// DeclineLease marks a lease as declined/conflicted by administrators.
func (s *Service) DeclineLease(ctx context.Context, scope ResourceScope, leaseID string) (*models.Lease, bool, error) {
	tenantID, err := s.tenantFromScope(scope)
	if err != nil {
		return nil, false, err
	}
	if err := s.ensurePrimaryWrite("lease.decline"); err != nil {
		return nil, false, err
	}
	lease, err := s.repo.GetLeaseByID(ctx, scope.AccessScope(), leaseID)
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
func (s *Service) ClearCooldown(ctx context.Context, scope ResourceScope, leaseID string) (*models.Lease, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, false, err
	}
	if err := s.ensurePrimaryWrite("lease.clear_cooldown"); err != nil {
		return nil, false, err
	}
	lease, err := s.repo.GetLeaseByID(ctx, scope.AccessScope(), leaseID)
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


// DeclinePrefix marks a delegated prefix as declined/conflicted.


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

func (s *Service) enforceLeaseLimits(ctx context.Context, scope ResourceScope, tenantID, identifier string, meta AllocationMetadata) error {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return nil
	}
	if limit := s.exhaustion.MaxLeasesPerMAC; limit > 0 {
		count, err := s.repo.CountActiveLeasesByIdentifier(ctx, scope.AccessScope(), identifier)
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
		count, err := s.repo.CountActiveLeasesByUser(ctx, scope.AccessScope(), meta.UserID)
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



// ReleasePrefixByID releases a delegated prefix via its lease ID.


// DeclinePrefixByID marks a delegated prefix as declined via its lease ID.


// ListPrefixLeases exposes prefix delegation search for HTTP handlers.


// ListLeases exposes repository search for HTTP handlers.
func (s *Service) ListLeases(ctx context.Context, scope ResourceScope, state string, limit, offset int) ([]models.Lease, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, err
	}
	return s.repo.ListLeases(ctx, scope.AccessScope(), state, limit, offset)
}

// CountActiveLeasesByPool returns utilization counters keyed by pool ID.
func (s *Service) CountActiveLeasesByPool(ctx context.Context, scope ResourceScope, poolIDs []string) (map[string]int64, error) {
	if s.repo == nil {
		return nil, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, err
	}
	return s.repo.CountActiveLeasesByPool(ctx, scope.AccessScope(), poolIDs)
}

// CountActiveLeases returns how many active leases exist for the tenant.
func (s *Service) CountActiveLeases(ctx context.Context, scope ResourceScope) (int, error) {
	if s.repo == nil {
		return 0, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return 0, err
	}
	return s.repo.CountActiveLeases(ctx, scope.AccessScope())
}

// CountLeasesCreatedSince returns how many leases were created since the given time.
func (s *Service) CountLeasesCreatedSince(ctx context.Context, scope ResourceScope, since time.Time) (int, error) {
	if s.repo == nil {
		return 0, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return 0, err
	}
	return s.repo.CountLeasesCreatedSince(ctx, scope.AccessScope(), since)
}

// CountConflictLeasesSince returns how many leases are marked as conflict/declined since the given time.
func (s *Service) CountConflictLeasesSince(ctx context.Context, scope ResourceScope, since time.Time) (int, error) {
	if s.repo == nil {
		return 0, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return 0, err
	}
	return s.repo.CountConflictLeasesSince(ctx, scope.AccessScope(), since)
}

// AverageLeaseDurationHours returns average lease duration (in hours) for active leases.
func (s *Service) AverageLeaseDurationHours(ctx context.Context, scope ResourceScope) (float64, error) {
	if s.repo == nil {
		return 0, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return 0, err
	}
	return s.repo.AverageLeaseDurationHours(ctx, scope.AccessScope())
}

// CountActiveLeasesByDeviceType aggregates active leases grouped by device type.
func (s *Service) CountActiveLeasesByDeviceType(ctx context.Context, scope ResourceScope) (map[string]int64, error) {
	if s.repo == nil {
		return nil, errors.New("lease: repository unavailable")
	}
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, err
	}
	return s.repo.CountActiveLeasesByDeviceType(ctx, scope.AccessScope())
}

// UpdateSecurityState persists the security posture for a lease identifier.

// UpdateSecurityState persists the security posture for a lease identifier (MAC/client-id).
func (s *Service) UpdateSecurityState(ctx context.Context, scope ResourceScope, identifier, state string) error {
	if _, err := s.tenantFromScope(scope); err != nil {
		return err
	}
	if identifier == "" {
		return errors.New("lease: tenant and identifier required")
	}
	normalized, err := normalizeSecurityState(state)
	if err != nil {
		return err
	}
	return s.repo.UpdateSecurityState(ctx, scope.AccessScope(), identifier, normalized, time.Now().UTC())
}

// UpdateLeaseSecurityState sets the security posture for a specific lease ID.
func (s *Service) UpdateLeaseSecurityState(ctx context.Context, scope ResourceScope, leaseID, state string) (*models.Lease, string, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, "", false, err
	}
	if leaseID == "" {
		return nil, "", false, errors.New("lease: tenant and lease id required")
	}
	normalized, err := normalizeSecurityState(state)
	if err != nil {
		return nil, "", false, err
	}
	lease, err := s.repo.GetLeaseByID(ctx, scope.AccessScope(), leaseID)
	if err != nil {
		return nil, "", false, err
	}
	previous := lease.SecurityState
	if previous == normalized {
		return lease, previous, false, nil
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateSecurityStateByID(ctx, scope.AccessScope(), leaseID, normalized, now); err != nil {
		return nil, "", false, err
	}
	lease.SecurityState = normalized
	lease.UpdatedAt = now
	return lease, previous, true, nil
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

func (s *Service) applyCooldownState(lease *models.Lease, conflictCount int, now time.Time) {
	if lease == nil {
		return
	}
	if conflictCount > 0 {
		if s.shouldQuarantine(conflictCount) {
			s.applyIsolationPolicy(lease.TenantID, lease.HardwareAddr, "conflict_exhaustion")
		}
		lease.State = leaseStateCooldown
		lease.ExpiresAt = now.Add(s.cooldownDuration())
		lease.SecurityState = "quarantined"
		s.recordLeaseMetric("lease_cooldown")
	}
}

func (s *Service) shouldQuarantine(conflicts int) bool {
	threshold := s.exhaustion.Isolation.PermanentAfter
	return threshold > 0 && conflicts >= threshold
}

func (s *Service) cooldownDuration() time.Duration {
	if s.exhaustion.PoolProtectPercent > 0 {
		// Use a fixed cooldown duration derived from exhaustion config
		return 5 * time.Minute
	}
	return 5 * time.Minute
}

func (s *Service) conflictEnabled() bool {
	return s.conflictCfg.Enabled
}

func (s *Service) conflictAttemptBudget() int {
	if s.conflictCfg.MaxAttempts > 0 {
		return s.conflictCfg.MaxAttempts
	}
	return 3
}

func (s *Service) conflictProbeTimeout() time.Duration {
	if s.conflictCfg.ProbeTimeout > 0 {
		return s.conflictCfg.ProbeTimeout
	}
	return 500 * time.Millisecond
}

func (s *Service) conflictHoldDuration() time.Duration {
	if s.conflictCfg.HoldDuration > 0 {
		return s.conflictCfg.HoldDuration
	}
	return 10 * time.Second
}

func (s *Service) conflictScanLimit() int {
	if s.conflictCfg.ReclaimScanLimit > 0 {
		return s.conflictCfg.ReclaimScanLimit
	}
	return 64
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
	alive, _, probeErr := s.conflictProber.ProbeICMP(probeCtx, addr)
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

func (s *Service) isProbeConflict(lease *models.Lease) bool {
	if lease == nil {
		return false
	}
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

func (s *Service) tryReclaimConflictLease(ctx context.Context, scope ResourceScope, identifier string, profile models.LeaseProfile, pool *models.AddressPool, meta AllocationMetadata) (*Result, bool, error) {
	leases, err := s.repo.ListLeasesByState(ctx, scope.AccessScope(), pool.ID, leaseStateCooldown, s.conflictScanLimit())
	if err != nil {
		return nil, false, err
	}
	for _, candidate := range leases {
		if s.isProbeConflict(&candidate) {
			continue
		}
		if s.logger != nil {
			s.logger.Info("reclaiming cooldown lease for reallocation",
				zap.String("leaseId", candidate.ID), zap.String("ip", candidate.IPAddress))
		}
		reclaimed := candidate
		reclaimed.HardwareAddr = identifier
		reclaimed.ClientID = identifier
		reclaimed.UserID = meta.UserID
		reclaimed.State = leaseStateActive
		reclaimed.SecurityState = models.SecurityStateOK
		reclaimed.ConflictHistory = nil
		s.applyMobilityMetadata(&reclaimed, meta)
		s.applyComplianceMetadata(&reclaimed, meta)
		s.applyLeaseTiming(&reclaimed, profile)
		reclaimed.ExpiresAt = time.Now().UTC()
		reclaimed.UpdatedAt = reclaimed.ExpiresAt
		if err := s.repo.UpdateLease(ctx, &reclaimed); err != nil {
			return nil, false, err
		}
		if err := s.confirmReplication(ctx, &reclaimed); err != nil {
			return nil, false, err
		}
		res := &Result{Lease: &reclaimed, Reused: true, Profile: profile, RenewalTime: profile.RenewalTime, RebindingTime: profile.RebindingTime, Pool: pool}
		res.Notification = s.computeNotification(&reclaimed, profile)
		if res.Notification != nil {
			s.enqueueNotificationJob(ctx, NotificationJob{
				LeaseID:   reclaimed.ID,
				TenantID:  reclaimed.TenantID,
				PoolID:    reclaimed.PoolID,
				SendAfter: res.Notification.SendAfter,
				Lead:      res.Notification.Lead,
				Kind:      "address",
				Metadata:  map[string]string{"ip": reclaimed.IPAddress},
			})
		}
		return res, true, nil
	}
	return nil, false, nil
}
