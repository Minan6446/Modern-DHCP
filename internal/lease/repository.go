package lease

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"modern-dhcp/internal/cache"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/resource"
	"modern-dhcp/internal/storage"
	"modern-dhcp/pkg/models"
)

// Repository defines persistence methods for leases.
type Repository interface {
	GetActiveLease(ctx context.Context, scope resource.AccessScope, identifier string) (*models.Lease, error)
	GetLeaseByID(ctx context.Context, scope resource.AccessScope, leaseID string) (*models.Lease, error)
	GetLeaseProfile(ctx context.Context, scope resource.AccessScope, profileID string) (*models.LeaseProfile, error)
	CreateLease(ctx context.Context, lease *models.Lease) error
	UpdateLease(ctx context.Context, lease *models.Lease) error
	ListLeasesByState(ctx context.Context, scope resource.AccessScope, poolID, state string, limit int) ([]models.Lease, error)
	CountActiveLeases(ctx context.Context, scope resource.AccessScope) (int, error)
	CountActiveLeasesByIdentifier(ctx context.Context, scope resource.AccessScope, identifier string) (int, error)
	CountActiveLeasesByUser(ctx context.Context, scope resource.AccessScope, userID string) (int, error)
	CountActiveLeasesByPool(ctx context.Context, scope resource.AccessScope, poolIDs []string) (map[string]int64, error)
	CountActiveLeasesByDeviceType(ctx context.Context, scope resource.AccessScope) (map[string]int64, error)
	CountLeasesCreatedSince(ctx context.Context, scope resource.AccessScope, since time.Time) (int, error)
	CountConflictLeasesSince(ctx context.Context, scope resource.AccessScope, since time.Time) (int, error)
	AverageLeaseDurationHours(ctx context.Context, scope resource.AccessScope) (float64, error)
	UpdateSecurityState(ctx context.Context, scope resource.AccessScope, identifier, state string, updatedAt time.Time) error
	UpdateSecurityStateByID(ctx context.Context, scope resource.AccessScope, leaseID, state string, updatedAt time.Time) error
	ListLeases(ctx context.Context, scope resource.AccessScope, state string, limit, offset int) ([]models.Lease, error)
	ListActiveIPs(ctx context.Context, scope resource.AccessScope, poolID string) ([]string, error)
	ListCooldownIPs(ctx context.Context, scope resource.AccessScope, poolID string, reference time.Time) ([]string, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error)
	ArchiveLeases(ctx context.Context, leases []models.Lease, archivedAt time.Time) error
	PurgeLeaseHistory(ctx context.Context, before time.Time, limit int) (int64, error)
	CleanupLegacyActiveSnapshots(ctx context.Context, limit int) (int64, error)

	GetActivePrefixLease(ctx context.Context, scope resource.AccessScope, clientID string, iapdID uint32) (*models.PrefixLease, error)
	GetPrefixLeaseByID(ctx context.Context, scope resource.AccessScope, prefixLeaseID string) (*models.PrefixLease, error)
	CreatePrefixLease(ctx context.Context, lease *models.PrefixLease) error
	UpdatePrefixLease(ctx context.Context, lease *models.PrefixLease) error
	ListActivePrefixes(ctx context.Context, scope resource.AccessScope, poolID string) ([]models.PrefixLease, error)
	ListPrefixLeases(ctx context.Context, scope resource.AccessScope, state string, limit, offset int) ([]models.PrefixLease, error)
	SearchLeaseHistory(ctx context.Context, scope resource.AccessScope, filter models.LeaseHistoryFilter) ([]models.Lease, int, error)
	FindIPv6EUI64BindingByMAC(ctx context.Context, scope resource.AccessScope, mac, prefix string) (*IPv6EUI64Binding, error)
	FindIPv6EUI64BindingByIPv6Addr(ctx context.Context, scope resource.AccessScope, prefix, ipv6Addr string) (*IPv6EUI64Binding, error)
	CreateIPv6EUI64Binding(ctx context.Context, binding *IPv6EUI64Binding) error
}

type tenantHandleProvider interface {
	Handle(ctx context.Context, tenantID string) (storage.TenantHandle, error)
}

// RepositoryOption customizes repository construction.
type RepositoryOption func(*MySQLRepository)

// WithTenantRouter toggles tenant-aware storage for lease persistence.
func WithTenantRouter(router tenantHandleProvider) RepositoryOption {
	return func(r *MySQLRepository) {
		r.router = router
	}
}

// WithCache enables L1+L2 caching for hot lease lookups.
func WithCache(store cache.Store, ttl time.Duration) RepositoryOption {
	return func(r *MySQLRepository) {
		r.cache = store
		if ttl > 0 {
			r.cacheTTL = ttl
		}
	}
}

// WithMetrics enables cache instrumentation.
func WithMetrics(c *metrics.Collector) RepositoryOption {
	return func(r *MySQLRepository) {
		r.metrics = c
	}
}

// MySQLRepository implements Repository using sqlx.
type MySQLRepository struct {
	db          *sqlx.DB
	router      tenantHandleProvider
	cache       cache.Store
	activeStore redis.UniversalClient
	cacheTTL    time.Duration
	metrics     *metrics.Collector
}

// WithRedisActiveStore enables Redis-backed active lease indexes.
func WithRedisActiveStore(client redis.UniversalClient) RepositoryOption {
	return func(r *MySQLRepository) {
		r.activeStore = client
	}
}

// NewRepository builds a MySQL-backed lease repository.
func NewRepository(db *sqlx.DB, opts ...RepositoryOption) *MySQLRepository {
	repo := &MySQLRepository{db: db, cacheTTL: 15 * time.Second}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

// ErrNotFound indicates missing row.
var ErrNotFound = errors.New("lease not found")

func normalizeBindingMAC(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}

func normalizeBindingIPv6Addr(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

func normalizeBindingPrefix(prefix string) string {
	return strings.TrimSpace(prefix)
}

func (r *MySQLRepository) FindIPv6EUI64BindingByMAC(ctx context.Context, scope resource.AccessScope, mac, prefix string) (*IPv6EUI64Binding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `
SELECT tenant_id, mac, prefix, ipv6_addr, lease_seconds, created_at, updated_at
FROM ipv6_eui64_bindings
WHERE tenant_id = ? AND mac = ? AND prefix = ?
LIMIT 1`
	var row struct {
		TenantID    string    `db:"tenant_id"`
		MAC         string    `db:"mac"`
		Prefix      string    `db:"prefix"`
		IPv6Addr    string    `db:"ipv6_addr"`
		LeaseSecond int64     `db:"lease_seconds"`
		CreatedAt   time.Time `db:"created_at"`
		UpdatedAt   time.Time `db:"updated_at"`
	}
	if err := db.GetContext(ctx, &row, query, tenantID, normalizeBindingMAC(mac), normalizeBindingPrefix(prefix)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &IPv6EUI64Binding{
		TenantID:  row.TenantID,
		MAC:       row.MAC,
		Prefix:    row.Prefix,
		IPv6Addr:  row.IPv6Addr,
		LeaseTime: time.Duration(row.LeaseSecond) * time.Second,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *MySQLRepository) FindIPv6EUI64BindingByIPv6Addr(ctx context.Context, scope resource.AccessScope, prefix, ipv6Addr string) (*IPv6EUI64Binding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `
SELECT tenant_id, mac, prefix, ipv6_addr, lease_seconds, created_at, updated_at
FROM ipv6_eui64_bindings
WHERE tenant_id = ? AND prefix = ? AND ipv6_addr = ?
LIMIT 1`
	var row struct {
		TenantID    string    `db:"tenant_id"`
		MAC         string    `db:"mac"`
		Prefix      string    `db:"prefix"`
		IPv6Addr    string    `db:"ipv6_addr"`
		LeaseSecond int64     `db:"lease_seconds"`
		CreatedAt   time.Time `db:"created_at"`
		UpdatedAt   time.Time `db:"updated_at"`
	}
	if err := db.GetContext(ctx, &row, query, tenantID, normalizeBindingPrefix(prefix), normalizeBindingIPv6Addr(ipv6Addr)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &IPv6EUI64Binding{
		TenantID:  row.TenantID,
		MAC:       row.MAC,
		Prefix:    row.Prefix,
		IPv6Addr:  row.IPv6Addr,
		LeaseTime: time.Duration(row.LeaseSecond) * time.Second,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *MySQLRepository) CreateIPv6EUI64Binding(ctx context.Context, binding *IPv6EUI64Binding) error {
	if binding == nil {
		return errors.New("lease: nil ipv6 eui64 binding")
	}
	tenantID := strings.TrimSpace(binding.TenantID)
	if tenantID == "" {
		return resource.ErrScopeRequired
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = binding.CreatedAt
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO ipv6_eui64_bindings (tenant_id, mac, prefix, ipv6_addr, lease_seconds, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	ipv6_addr = VALUES(ipv6_addr),
	lease_seconds = VALUES(lease_seconds),
	updated_at = VALUES(updated_at)`,
		tenantID,
		normalizeBindingMAC(binding.MAC),
		normalizeBindingPrefix(binding.Prefix),
		normalizeBindingIPv6Addr(binding.IPv6Addr),
		int64(binding.LeaseTime/time.Second),
		binding.CreatedAt,
		binding.UpdatedAt,
	)
	return err
}

func (r *MySQLRepository) tenantDB(ctx context.Context, tenantID string) (*sqlx.DB, error) {
	if tenantID == "" || r.router == nil {
		return r.db, nil
	}
	handle, err := r.router.Handle(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if handle.DB == nil {
		return r.db, nil
	}
	db := handle.DB
	if handle.Schema != "" {
		db = storage.SchemaAware(db, handle.Schema)
	}
	return db, nil
}

func tenantFromAccessScope(scope resource.AccessScope) (string, error) {
	tenantID := strings.TrimSpace(scope.EffectiveTenant())
	if tenantID == "" {
		return "", resource.ErrScopeRequired
	}
	return tenantID, nil
}

func (r *MySQLRepository) cacheKeyActiveLease(tenantID, identifier string) string {
	return fmt.Sprintf("lease-active:%s:%s", tenantID, strings.TrimSpace(identifier))
}

func (r *MySQLRepository) cacheKeyLease(tenantID, leaseID string) string {
	return fmt.Sprintf("lease:%s:id:%s", tenantID, leaseID)
}

func (r *MySQLRepository) redisActivePoolKey(tenantID, poolID string) string {
	return fmt.Sprintf("lease-active-pool:%s:%s", tenantID, strings.TrimSpace(poolID))
}

func (r *MySQLRepository) redisLeaseIndexKey(tenantID, leaseID string) string {
	return fmt.Sprintf("lease-active-idx:%s:%s", tenantID, strings.TrimSpace(leaseID))
}

func (r *MySQLRepository) redisActiveLeaseObjectKey(tenantID, leaseID string) string {
	return fmt.Sprintf("lease-active-obj:%s:%s", tenantID, strings.TrimSpace(leaseID))
}

func (r *MySQLRepository) redisActiveIdentifierKey(tenantID, identifier string) string {
	return fmt.Sprintf("lease-active-ident:%s:%s", tenantID, strings.TrimSpace(identifier))
}

func (r *MySQLRepository) redisActiveUserKey(tenantID, userID string) string {
	return fmt.Sprintf("lease-active-user:%s:%s", tenantID, strings.TrimSpace(userID))
}

func (r *MySQLRepository) redisActiveDeviceKey(tenantID, deviceType string) string {
	return fmt.Sprintf("lease-active-device:%s:%s", tenantID, strings.TrimSpace(deviceType))
}

func (r *MySQLRepository) redisActiveExpiryKey(tenantID string) string {
	return fmt.Sprintf("lease-active-expiry:%s", strings.TrimSpace(tenantID))
}

func (r *MySQLRepository) redisCountActiveLeases(ctx context.Context, tenantID string) (int64, bool) {
	if r.activeStore == nil {
		return 0, false
	}
	pattern := r.redisActivePoolKey(tenantID, "*")
	now := time.Now().Unix()
	var (
		total  int64
		cursor uint64
	)
	for {
		keys, next, err := r.activeStore.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return 0, false
		}
		if len(keys) > 0 {
			pipe := r.activeStore.Pipeline()
			remCmds := make([]*redis.IntCmd, 0, len(keys))
			cardCmds := make([]*redis.IntCmd, 0, len(keys))
			for _, key := range keys {
				remCmds = append(remCmds, pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now)))
				cardCmds = append(cardCmds, pipe.ZCard(ctx, key))
			}
			if _, err := pipe.Exec(ctx); err != nil {
				return 0, false
			}
			for i := range cardCmds {
				if remCmds[i].Err() != nil || cardCmds[i].Err() != nil {
					continue
				}
				total += cardCmds[i].Val()
			}
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return total, true
}

func (r *MySQLRepository) redisListActiveLeases(
	ctx context.Context,
	tenantID string,
	filter func(*models.Lease) bool,
	sortDesc bool,
	limit, offset int,
) ([]models.Lease, error) {
	if r.activeStore == nil {
		return nil, nil
	}
	pattern := r.redisActiveLeaseObjectKey(tenantID, "*")
	items := make([]models.Lease, 0, max(32, limit+offset))
	var cursor uint64
	for {
		keys, next, err := r.activeStore.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			payload, getErr := r.activeStore.Get(ctx, key).Bytes()
			if getErr != nil {
				continue
			}
			var lease models.Lease
			if err := json.Unmarshal(payload, &lease); err != nil {
				continue
			}
			if filter != nil && !filter(&lease) {
				continue
			}
			items = append(items, lease)
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	sort.Slice(items, func(i, j int) bool {
		if sortDesc {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	if offset >= len(items) {
		return []models.Lease{}, nil
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end], nil
}

func (r *MySQLRepository) redisRebuildActiveIndexesFromObjects(ctx context.Context) error {
	if r.activeStore == nil {
		return nil
	}
	var cursor uint64
	for {
		keys, next, err := r.activeStore.Scan(ctx, cursor, "lease-active-obj:*:*", 200).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			payload, getErr := r.activeStore.Get(ctx, key).Bytes()
			if getErr != nil {
				continue
			}
			var lease models.Lease
			if err := json.Unmarshal(payload, &lease); err != nil {
				continue
			}
			tenantID := strings.TrimSpace(lease.TenantID)
			if tenantID == "" {
				tenantID = "global"
			}
			if strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE") {
				r.syncRedisActiveLeaseIndex(ctx, tenantID, &lease, true)
			} else {
				r.syncRedisActiveLeaseIndex(ctx, tenantID, &lease, false)
			}
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return nil
}

func (r *MySQLRepository) recordCache(resource, operation, layer, result string) {
	if r.metrics != nil && r.metrics.CacheEvents != nil {
		r.metrics.CacheEvents.WithLabelValues(resource, operation, layer, result).Inc()
	}
}

func (r *MySQLRepository) evictTenant(ctx context.Context, tenantID string) {
	if r.cache == nil {
		return
	}
	_ = r.cache.DeletePrefix(ctx, "lease:"+tenantID+":")
}

// GetActiveLease fetches a lease by identifier (MAC/client-id).
func (r *MySQLRepository) GetActiveLease(ctx context.Context, scope resource.AccessScope, identifier string) (*models.Lease, error) {
	const query = `
SELECT *
FROM leases_v4
	WHERE (hardware_addr = ? OR client_id = ?)
  AND state = 'ACTIVE'
LIMIT 1`

	var lease models.Lease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		idKey := r.redisActiveIdentifierKey(tenantID, identifier)
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		_ = pipe.ZRemRangeByScore(ctx, idKey, "-inf", fmt.Sprintf("%d", now))
		ids := pipe.ZRange(ctx, idKey, 0, 0)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return nil, execErr
		}
		leaseIDs, idsErr := ids.Result()
		if idsErr != nil {
			return nil, idsErr
		}
		if len(leaseIDs) == 0 {
			return nil, ErrNotFound
		}
		payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseIDs[0])).Bytes()
		if getErr == redis.Nil {
			_ = r.activeStore.ZRem(ctx, idKey, leaseIDs[0]).Err()
			return nil, ErrNotFound
		}
		if getErr != nil {
			return nil, getErr
		}
		var lease models.Lease
		if err := json.Unmarshal(payload, &lease); err != nil {
			return nil, err
		}
		return &lease, nil
	}
	if r.cache != nil {
		cacheKey := r.cacheKeyActiveLease(tenantID, identifier)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &lease); cErr == nil && ok {
			r.recordCache("lease", "get_active", "any", "hit")
			return &lease, nil
		} else if cErr == nil {
			r.recordCache("lease", "get_active", "any", "miss")
		}
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, identifier, identifier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if r.cache != nil {
		ttl := time.Until(lease.ExpiresAt)
		if ttl <= 0 {
			ttl = r.cacheTTL
		}
		if ttl <= 0 {
			ttl = 15 * time.Second
		}
		_ = r.cache.SetJSON(ctx, r.cacheKeyActiveLease(tenantID, identifier), lease, ttl)
	}

	return &lease, nil
}

// GetLeaseByID fetches a lease regardless of state.
func (r *MySQLRepository) GetLeaseByID(ctx context.Context, scope resource.AccessScope, leaseID string) (*models.Lease, error) {
	const query = `
SELECT *
FROM leases_v4
	WHERE id = ?
LIMIT 1`
	var lease models.Lease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID)).Bytes()
		if getErr == nil {
			if err := json.Unmarshal(payload, &lease); err == nil {
				return &lease, nil
			}
		}
	}
	if r.cache != nil {
		cacheKey := r.cacheKeyLease(tenantID, leaseID)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &lease); cErr == nil && ok {
			r.recordCache("lease", "get", "any", "hit")
			return &lease, nil
		} else if cErr == nil {
			r.recordCache("lease", "get", "any", "miss")
		}
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, leaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyLease(tenantID, leaseID), lease, r.cacheTTL)
	}
	return &lease, nil
}

// GetLeaseProfile fetches lease profile by identifier.
func (r *MySQLRepository) GetLeaseProfile(ctx context.Context, scope resource.AccessScope, profileID string) (*models.LeaseProfile, error) {
	const query = `
SELECT *
FROM lease_profiles
WHERE id = ?
LIMIT 1`
	var profile models.LeaseProfile
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &profile, query, profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &profile, nil
}

// CreateLease inserts a new lease.
func (r *MySQLRepository) CreateLease(ctx context.Context, lease *models.Lease) error {
	tenantID := strings.TrimSpace(lease.TenantID)
	if r.activeStore != nil {
		if strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE") {
			r.syncActiveLeaseCache(ctx, lease)
			_ = r.deleteLeaseSnapshotByID(ctx, tenantID, lease.ID)
			r.evictTenant(ctx, tenantID)
			return nil
		}
		if err := r.ArchiveLeases(ctx, []models.Lease{*lease}, lease.UpdatedAt); err != nil {
			return err
		}
		_ = r.deleteLeaseSnapshotByID(ctx, tenantID, lease.ID)
		r.evictTenant(ctx, tenantID)
		return nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO leases_v4 (
	id, pool_id, ip_address, hardware_addr, client_id,
	user_id, mobility_anchor_id, device_type, last_access_point_id, last_controller_id,
	last_geo_zone, mobility_location_hint, mdm_managed, mdm_source, mdm_tags, mdm_observed_at,
	relay_info, session_continuity,
	expires_at, state, security_state, cooldown_until, conflict_history, created_at, updated_at)
VALUES (
	:id, :pool_id, :ip_address, :hardware_addr, :client_id,
	:user_id, :mobility_anchor_id, :device_type, :last_access_point_id, :last_controller_id,
	:last_geo_zone, :mobility_location_hint, :mdm_managed, :mdm_source, :mdm_tags, :mdm_observed_at,
	:relay_info, :session_continuity,
	:expires_at, :state, :security_state, :cooldown_until, :conflict_history, :created_at, :updated_at)`, lease)
	if err == nil {
		r.syncActiveLeaseCache(ctx, lease)
	}
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) deleteLeaseSnapshotByID(ctx context.Context, tenantID, leaseID string) error {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DELETE FROM leases_v4 WHERE id = ?", leaseID)
	return err
}

// UpdateLease persists changes to an existing lease.
func (r *MySQLRepository) UpdateLease(ctx context.Context, lease *models.Lease) error {
	tenantID := strings.TrimSpace(lease.TenantID)
	if r.activeStore != nil {
		r.syncActiveLeaseCache(ctx, lease)
		r.evictTenant(ctx, tenantID)
		if strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE") {
			_ = r.deleteLeaseSnapshotByID(ctx, tenantID, lease.ID)
			return nil
		}
		if err := r.ArchiveLeases(ctx, []models.Lease{*lease}, lease.UpdatedAt); err != nil {
			return err
		}
		_ = r.deleteLeaseSnapshotByID(ctx, tenantID, lease.ID)
		return nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
UPDATE leases_v4 SET
	pool_id = :pool_id,
	ip_address = :ip_address,
	expires_at = :expires_at,
	state = :state,
	cooldown_until = :cooldown_until,
	mobility_anchor_id = :mobility_anchor_id,
	last_access_point_id = :last_access_point_id,
	last_controller_id = :last_controller_id,
	last_geo_zone = :last_geo_zone,
	mobility_location_hint = :mobility_location_hint,
	mdm_managed = :mdm_managed,
	mdm_source = :mdm_source,
	mdm_tags = :mdm_tags,
	mdm_observed_at = :mdm_observed_at,
	session_continuity = :session_continuity,
	conflict_history = :conflict_history,
	updated_at = :updated_at
WHERE id = :id`, lease)
	if err == nil {
		r.syncActiveLeaseCache(ctx, lease)
	}
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) syncActiveLeaseCache(ctx context.Context, lease *models.Lease) {
	if r.cache == nil || lease == nil {
		if r.activeStore == nil {
			return
		}
	}
	tenantID := strings.TrimSpace(lease.TenantID)
	if tenantID == "" {
		tenantID = "global"
	}
	hwKey := r.cacheKeyActiveLease(tenantID, lease.HardwareAddr)
	clientKey := r.cacheKeyActiveLease(tenantID, lease.ClientID)

	if strings.ToUpper(strings.TrimSpace(lease.State)) != "ACTIVE" {
		if strings.TrimSpace(lease.HardwareAddr) != "" {
			if r.cache != nil {
				_ = r.cache.Delete(ctx, hwKey)
			}
		}
		if strings.TrimSpace(lease.ClientID) != "" {
			if r.cache != nil {
				_ = r.cache.Delete(ctx, clientKey)
			}
		}
		if r.activeStore != nil {
			r.syncRedisActiveLeaseIndex(ctx, tenantID, lease, false)
		}
		return
	}

	ttl := time.Until(lease.ExpiresAt)
	if ttl <= 0 {
		ttl = r.cacheTTL
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	if r.cache != nil && strings.TrimSpace(lease.HardwareAddr) != "" {
		_ = r.cache.SetJSON(ctx, hwKey, lease, ttl)
	}
	if r.cache != nil && strings.TrimSpace(lease.ClientID) != "" {
		_ = r.cache.SetJSON(ctx, clientKey, lease, ttl)
	}
	if r.activeStore != nil {
		r.syncRedisActiveLeaseIndex(ctx, tenantID, lease, true)
	}
}

func (r *MySQLRepository) syncRedisActiveLeaseIndex(ctx context.Context, tenantID string, lease *models.Lease, isActive bool) {
	if r.activeStore == nil || lease == nil {
		return
	}
	leaseID := strings.TrimSpace(lease.ID)
	if leaseID == "" {
		return
	}
	idxKey := r.redisLeaseIndexKey(tenantID, leaseID)
	pipe := r.activeStore.TxPipeline()
	prev, err := r.activeStore.Get(ctx, idxKey).Result()
	if err != nil && err != redis.Nil {
		return
	}
	if prev != "" {
		parts := strings.Split(prev, "|")
		if len(parts) >= 2 {
			prevPool := strings.TrimSpace(parts[0])
			prevIP := strings.TrimSpace(parts[1])
			if prevPool != "" && prevIP != "" {
				pipe.ZRem(ctx, r.redisActivePoolKey(tenantID, prevPool), prevIP)
			}
			if len(parts) >= 3 {
				prevHardware := strings.TrimSpace(parts[2])
				if prevHardware != "" {
					pipe.ZRem(ctx, r.redisActiveIdentifierKey(tenantID, prevHardware), leaseID)
				}
			}
			if len(parts) >= 4 {
				prevClient := strings.TrimSpace(parts[3])
				if prevClient != "" {
					pipe.ZRem(ctx, r.redisActiveIdentifierKey(tenantID, prevClient), leaseID)
				}
			}
			if len(parts) >= 5 {
				prevUser := strings.TrimSpace(parts[4])
				if prevUser != "" {
					pipe.ZRem(ctx, r.redisActiveUserKey(tenantID, prevUser), leaseID)
				}
			}
			if len(parts) >= 6 {
				prevDeviceType := strings.TrimSpace(parts[5])
				if prevDeviceType != "" {
					pipe.ZRem(ctx, r.redisActiveDeviceKey(tenantID, prevDeviceType), leaseID)
				}
			}
		}
	}
	if !isActive {
		pipe.Del(ctx, idxKey)
		pipe.Del(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID))
		pipe.ZRem(ctx, r.redisActiveExpiryKey(tenantID), leaseID)
		_, _ = pipe.Exec(ctx)
		return
	}
	poolID := strings.TrimSpace(lease.PoolID)
	ip := strings.TrimSpace(lease.IPAddress)
	hardware := strings.TrimSpace(lease.HardwareAddr)
	clientID := strings.TrimSpace(lease.ClientID)
	userID := strings.TrimSpace(lease.UserID)
	deviceType := strings.TrimSpace(lease.DeviceType)
	if deviceType == "" {
		deviceType = "unknown"
	}
	if poolID == "" || ip == "" {
		pipe.Del(ctx, idxKey)
		pipe.Del(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID))
		pipe.ZRem(ctx, r.redisActiveExpiryKey(tenantID), leaseID)
		_, _ = pipe.Exec(ctx)
		return
	}
	expiryScore := float64(lease.ExpiresAt.Unix())
	pipe.ZAdd(ctx, r.redisActivePoolKey(tenantID, poolID), redis.Z{
		Score:  expiryScore,
		Member: ip,
	})
	if hardware != "" {
		pipe.ZAdd(ctx, r.redisActiveIdentifierKey(tenantID, hardware), redis.Z{Score: expiryScore, Member: leaseID})
	}
	if clientID != "" {
		pipe.ZAdd(ctx, r.redisActiveIdentifierKey(tenantID, clientID), redis.Z{Score: expiryScore, Member: leaseID})
	}
	if userID != "" {
		pipe.ZAdd(ctx, r.redisActiveUserKey(tenantID, userID), redis.Z{Score: expiryScore, Member: leaseID})
	}
	pipe.ZAdd(ctx, r.redisActiveDeviceKey(tenantID, deviceType), redis.Z{Score: expiryScore, Member: leaseID})
	pipe.ZAdd(ctx, r.redisActiveExpiryKey(tenantID), redis.Z{Score: expiryScore, Member: leaseID})
	if payload, marshalErr := json.Marshal(lease); marshalErr == nil {
		ttl := time.Until(lease.ExpiresAt)
		if ttl <= 0 {
			ttl = 15 * time.Second
		}
		pipe.Set(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID), payload, ttl)
	}
	pipe.Set(ctx, idxKey, strings.Join([]string{poolID, ip, hardware, clientID, userID, deviceType}, "|"), 7*24*time.Hour)
	_, _ = pipe.Exec(ctx)
}

// ListLeasesByState returns leases from a pool filtered by state.
func (r *MySQLRepository) ListLeasesByState(ctx context.Context, scope resource.AccessScope, poolID, state string, limit int) ([]models.Lease, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	const query = `
SELECT *
FROM leases_v4
WHERE pool_id = ?
	AND state = ?
ORDER BY updated_at ASC
LIMIT ?`
	var leases []models.Lease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil && strings.EqualFold(strings.TrimSpace(state), "ACTIVE") {
		return r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
			return strings.EqualFold(strings.TrimSpace(lease.PoolID), strings.TrimSpace(poolID)) && strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE")
		}, false, limit, 0)
	}
	if r.activeStore != nil {
		const historyQuery = `
SELECT
	lease_id AS id,
	tenant_id,
	pool_id,
	ip_address,
	hardware_addr,
	client_id,
	user_id,
	mobility_anchor_id,
	device_type,
	last_access_point_id,
	last_controller_id,
	last_geo_zone,
	mobility_location_hint,
	mdm_managed,
	mdm_source,
	mdm_tags,
	mdm_observed_at,
	relay_info,
	session_continuity,
	expires_at,
	final_state AS state,
	security_state,
	cooldown_until,
	conflict_history,
	created_at,
	updated_at
FROM lease_history_v4
WHERE pool_id = ?
	AND final_state = ?
ORDER BY updated_at ASC
LIMIT ?`
		var history []models.Lease
		db, err := r.tenantDB(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if err := db.SelectContext(ctx, &history, historyQuery, poolID, strings.ToUpper(strings.TrimSpace(state)), limit); err != nil {
			return nil, err
		}
		return history, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, poolID, state, limit); err != nil {
		return nil, err
	}
	return leases, nil
}

// CountActiveLeases returns how many active leases exist for the tenant.
func (r *MySQLRepository) CountActiveLeases(ctx context.Context, scope resource.AccessScope) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE state = 'ACTIVE'`
	var count int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	if redisCount, ok := r.redisCountActiveLeases(ctx, tenantID); ok {
		if redisCount < 0 {
			redisCount = 0
		}
		return int(redisCount), nil
	}
	if r.activeStore != nil {
		return 0, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByIdentifier returns how many active leases exist for the MAC/client identifier.
func (r *MySQLRepository) CountActiveLeasesByIdentifier(ctx context.Context, scope resource.AccessScope, identifier string) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE state = 'ACTIVE'
	AND (hardware_addr = ? OR client_id = ?)`
	var count int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	if r.activeStore != nil {
		key := r.redisActiveIdentifierKey(tenantID, identifier)
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		_ = pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now))
		card := pipe.ZCard(ctx, key)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return 0, execErr
		}
		if card.Err() != nil {
			return 0, card.Err()
		}
		return int(card.Val()), nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query, identifier, identifier); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByUser returns how many active leases are bound to a given user identifier.
func (r *MySQLRepository) CountActiveLeasesByUser(ctx context.Context, scope resource.AccessScope, userID string) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE state = 'ACTIVE'
	AND user_id = ?`
	var count int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	if r.activeStore != nil {
		key := r.redisActiveUserKey(tenantID, userID)
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		_ = pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now))
		card := pipe.ZCard(ctx, key)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return 0, execErr
		}
		if card.Err() != nil {
			return 0, card.Err()
		}
		return int(card.Val()), nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query, userID); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByPool returns the number of active leases per pool.
func (r *MySQLRepository) CountActiveLeasesByPool(ctx context.Context, scope resource.AccessScope, poolIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(poolIDs))
	if len(poolIDs) == 0 {
		return counts, nil
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		remCmds := make([]*redis.IntCmd, 0, len(poolIDs))
		countCmds := make([]*redis.IntCmd, 0, len(poolIDs))
		for _, poolID := range poolIDs {
			key := r.redisActivePoolKey(tenantID, poolID)
			remCmds = append(remCmds, pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now)))
			countCmds = append(countCmds, pipe.ZCard(ctx, key))
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
		for i, poolID := range poolIDs {
			if remCmds[i].Err() != nil || countCmds[i].Err() != nil {
				continue
			}
			if c := countCmds[i].Val(); c > 0 {
				counts[poolID] = c
			}
		}
		return counts, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT pool_id, COUNT(*) AS total
FROM leases_v4
WHERE state = 'ACTIVE'
	AND pool_id IN (?)
GROUP BY pool_id`, poolIDs)
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)
	var rows []struct {
		PoolID string `db:"pool_id"`
		Count  int64  `db:"total"`
	}
	if err := db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.PoolID] = row.Count
	}
	return counts, nil
}

// CountActiveLeasesByDeviceType returns active leases grouped by device type.
func (r *MySQLRepository) CountActiveLeasesByDeviceType(ctx context.Context, scope resource.AccessScope) (map[string]int64, error) {
	const query = `
SELECT COALESCE(NULLIF(device_type, ''), 'unknown') AS device_type,
	   COUNT(*) AS total
FROM leases_v4
WHERE state = 'ACTIVE'
GROUP BY COALESCE(NULLIF(device_type, ''), 'unknown')`
	var rows []struct {
		DeviceType string `db:"device_type"`
		Count      int64  `db:"total"`
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		pattern := r.redisActiveDeviceKey(tenantID, "*")
		prefix := r.redisActiveDeviceKey(tenantID, "")
		now := time.Now().Unix()
		counts := make(map[string]int64)
		var cursor uint64
		for {
			keys, next, scanErr := r.activeStore.Scan(ctx, cursor, pattern, 200).Result()
			if scanErr != nil {
				counts = nil
				break
			}
			if len(keys) > 0 {
				pipe := r.activeStore.Pipeline()
				remCmds := make([]*redis.IntCmd, 0, len(keys))
				cardCmds := make([]*redis.IntCmd, 0, len(keys))
				for _, key := range keys {
					remCmds = append(remCmds, pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now)))
					cardCmds = append(cardCmds, pipe.ZCard(ctx, key))
				}
				if _, execErr := pipe.Exec(ctx); execErr != nil {
					counts = nil
					break
				}
				for i, key := range keys {
					if remCmds[i].Err() != nil || cardCmds[i].Err() != nil {
						continue
					}
					device := strings.TrimPrefix(key, prefix)
					if device == "" {
						device = "unknown"
					}
					if c := cardCmds[i].Val(); c > 0 {
						counts[device] = c
					}
				}
			}
			if counts == nil || next == 0 {
				break
			}
			cursor = next
		}
		if counts == nil {
			return nil, errors.New("redis active device count failed")
		}
		return counts, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.DeviceType] = row.Count
	}
	return counts, nil
}

// CountLeasesCreatedSince returns how many leases were created since the given time.
func (r *MySQLRepository) CountLeasesCreatedSince(ctx context.Context, scope resource.AccessScope, since time.Time) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE created_at >= ?`
	const archivedQuery = `
SELECT COUNT(*)
FROM lease_history_v4
WHERE created_at >= ?`
	var count int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if r.activeStore != nil {
		activeLeases, err := r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
			return strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE")
		}, true, 0, 0)
		if err != nil {
			return 0, err
		}
		activeCount := 0
		for i := range activeLeases {
			if !activeLeases[i].CreatedAt.Before(since) {
				activeCount++
			}
		}
		archivedCount := 0
		if err := db.GetContext(ctx, &archivedCount, archivedQuery, since); err != nil {
			return 0, err
		}
		return activeCount + archivedCount, nil
	}
	if err := db.GetContext(ctx, &count, query, since); err != nil {
		return 0, err
	}
	return count, nil
}

// CountConflictLeasesSince returns how many leases were marked as conflict/declined since the given time.
func (r *MySQLRepository) CountConflictLeasesSince(ctx context.Context, scope resource.AccessScope, since time.Time) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE state IN ('DECLINED','CONFLICT')
	AND updated_at >= ?`
	const snapshotQuery = `
SELECT COUNT(*)
FROM lease_history_v4
WHERE final_state IN ('DECLINED','CONFLICT')
	AND updated_at >= ?`
	var count int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if r.activeStore != nil {
		activeLeases, err := r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
			state := strings.ToUpper(strings.TrimSpace(lease.State))
			return state == "DECLINED" || state == "CONFLICT"
		}, true, 0, 0)
		if err != nil {
			return 0, err
		}
		activeCount := 0
		for i := range activeLeases {
			if !activeLeases[i].UpdatedAt.Before(since) {
				activeCount++
			}
		}
		snapshotCount := 0
		if err := db.GetContext(ctx, &snapshotCount, snapshotQuery, since); err != nil {
			return 0, err
		}
		return activeCount + snapshotCount, nil
	}
	if err := db.GetContext(ctx, &count, query, since); err != nil {
		return 0, err
	}
	return count, nil
}

// AverageLeaseDurationHours returns average lease duration (in hours) for active leases.
func (r *MySQLRepository) AverageLeaseDurationHours(ctx context.Context, scope resource.AccessScope) (float64, error) {
	const query = `
SELECT AVG(TIMESTAMPDIFF(SECOND, created_at, expires_at))
FROM leases_v4
WHERE state = 'ACTIVE'
	AND expires_at > created_at`
	var avgSeconds sql.NullFloat64
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	if r.activeStore != nil {
		activeLeases, err := r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
			return strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE")
		}, true, 0, 0)
		if err != nil {
			return 0, err
		}
		if len(activeLeases) == 0 {
			return 0, nil
		}
		var totalSeconds float64
		var samples float64
		for i := range activeLeases {
			lease := activeLeases[i]
			if !lease.ExpiresAt.After(lease.CreatedAt) {
				continue
			}
			totalSeconds += lease.ExpiresAt.Sub(lease.CreatedAt).Seconds()
			samples++
		}
		if samples == 0 {
			return 0, nil
		}
		return (totalSeconds / samples) / 3600.0, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &avgSeconds, query); err != nil {
		return 0, err
	}
	if !avgSeconds.Valid {
		return 0, nil
	}
	return avgSeconds.Float64 / 3600.0, nil
}

// UpdateSecurityState updates the security posture for active leases tied to an identifier.
func (r *MySQLRepository) UpdateSecurityState(ctx context.Context, scope resource.AccessScope, identifier, state string, updatedAt time.Time) error {
	const query = `
UPDATE leases_v4
SET security_state = ?, updated_at = ?
WHERE (hardware_addr = ? OR client_id = ?)`
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return err
	}
	if r.activeStore != nil {
		key := r.redisActiveIdentifierKey(tenantID, identifier)
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		_ = pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now))
		members := pipe.ZRange(ctx, key, 0, -1)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return execErr
		}
		leaseIDs, listErr := members.Result()
		if listErr != nil {
			return listErr
		}
		for _, leaseID := range leaseIDs {
			payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID)).Bytes()
			if getErr == redis.Nil {
				_ = r.activeStore.ZRem(ctx, key, leaseID).Err()
				continue
			}
			if getErr != nil {
				return getErr
			}
			var lease models.Lease
			if err := json.Unmarshal(payload, &lease); err != nil {
				continue
			}
			lease.SecurityState = state
			lease.UpdatedAt = updatedAt
			r.syncActiveLeaseCache(ctx, &lease)
		}
		r.evictTenant(ctx, tenantID)
		return nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, query, state, updatedAt, identifier, identifier)
	r.evictTenant(ctx, tenantID)
	return err
}

// UpdateSecurityStateByID updates the security posture for a lease referenced by its ID.
func (r *MySQLRepository) UpdateSecurityStateByID(ctx context.Context, scope resource.AccessScope, leaseID, state string, updatedAt time.Time) error {
	const query = `
UPDATE leases_v4
SET security_state = ?, updated_at = ?
WHERE id = ?`
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return err
	}
	if r.activeStore != nil {
		payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID)).Bytes()
		if getErr == nil {
			var lease models.Lease
			if err := json.Unmarshal(payload, &lease); err == nil {
				lease.SecurityState = state
				lease.UpdatedAt = updatedAt
				r.syncActiveLeaseCache(ctx, &lease)
				r.evictTenant(ctx, tenantID)
				return nil
			}
		}
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, query, state, updatedAt, leaseID)
	r.evictTenant(ctx, tenantID)
	return err
}

// ListLeases returns leases filtered by optional state.
func (r *MySQLRepository) ListLeases(ctx context.Context, scope resource.AccessScope, state string, limit, offset int) ([]models.Lease, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		normalizedState := strings.ToUpper(strings.TrimSpace(state))
		if normalizedState == "ACTIVE" {
			return r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
				return strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE")
			}, true, limit, offset)
		}
		historyQuery := `
SELECT
	lease_id AS id,
	tenant_id,
	pool_id,
	ip_address,
	hardware_addr,
	client_id,
	user_id,
	mobility_anchor_id,
	device_type,
	last_access_point_id,
	last_controller_id,
	last_geo_zone,
	mobility_location_hint,
	mdm_managed,
	mdm_source,
	mdm_tags,
	mdm_observed_at,
	relay_info,
	session_continuity,
	expires_at,
	final_state AS state,
	security_state,
	cooldown_until,
	conflict_history,
	created_at,
	updated_at
FROM lease_history_v4
WHERE 1=1`
		historyArgs := []any{}
		if normalizedState != "" {
			historyQuery += " AND final_state = ?"
			historyArgs = append(historyArgs, normalizedState)
		}
		historyQuery += " ORDER BY archived_at DESC LIMIT ? OFFSET ?"
		historyArgs = append(historyArgs, limit+offset, 0)
		var historyRows []models.Lease
		db, err := r.tenantDB(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if err := db.SelectContext(ctx, &historyRows, historyQuery, historyArgs...); err != nil {
			return nil, err
		}
		if normalizedState == "" {
			active, err := r.redisListActiveLeases(ctx, tenantID, func(lease *models.Lease) bool {
				return strings.EqualFold(strings.TrimSpace(lease.State), "ACTIVE")
			}, true, limit+offset, 0)
			if err != nil {
				return nil, err
			}
			merged := append(active, historyRows...)
			sort.Slice(merged, func(i, j int) bool {
				return merged[i].UpdatedAt.After(merged[j].UpdatedAt)
			})
			if offset >= len(merged) {
				return []models.Lease{}, nil
			}
			end := len(merged)
			if offset+limit < end {
				end = offset + limit
			}
			return merged[offset:end], nil
		}
		if offset >= len(historyRows) {
			return []models.Lease{}, nil
		}
		end := len(historyRows)
		if offset+limit < end {
			end = offset + limit
		}
		return historyRows[offset:end], nil
	}
	query := "SELECT * FROM leases_v4 WHERE 1=1"
	args := []any{}
	if state != "" {
		query += " AND state = ?"
		args = append(args, state)
	}
	query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	var leases []models.Lease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, args...); err != nil {
		return nil, err
	}
	return leases, nil
}

// ListActiveIPs returns currently allocated IPs for a pool.
func (r *MySQLRepository) ListActiveIPs(ctx context.Context, scope resource.AccessScope, poolID string) ([]string, error) {
	const query = `SELECT ip_address FROM leases_v4 WHERE pool_id = ? AND state = 'ACTIVE'`
	var ips []string
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		key := r.redisActivePoolKey(tenantID, poolID)
		now := time.Now().Unix()
		pipe := r.activeStore.Pipeline()
		_ = pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now))
		members := pipe.ZRange(ctx, key, 0, -1)
		if _, execErr := pipe.Exec(ctx); execErr != nil {
			return nil, execErr
		}
		vals, mErr := members.Result()
		if mErr != nil {
			return nil, mErr
		}
		return vals, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &ips, query, poolID); err != nil {
		return nil, err
	}
	return ips, nil
}

// ListCooldownIPs returns addresses still under cooldown for the pool.
func (r *MySQLRepository) ListCooldownIPs(ctx context.Context, scope resource.AccessScope, poolID string, reference time.Time) ([]string, error) {
	const query = `
SELECT ip_address
FROM leases_v4
WHERE pool_id = ?
	AND (
				(cooldown_until IS NOT NULL AND cooldown_until > ?)
		 OR state = 'QUARANTINED'
			)`
	var ips []string
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.activeStore != nil {
		const historyCooldownQuery = `
SELECT ip_address
FROM lease_history_v4
WHERE pool_id = ?
	AND (
			(cooldown_until IS NOT NULL AND cooldown_until > ?)
		 OR final_state = 'QUARANTINED'
		)`
		db, err := r.tenantDB(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if err := db.SelectContext(ctx, &ips, historyCooldownQuery, poolID, reference); err != nil {
			return nil, err
		}
		return ips, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &ips, query, poolID, reference); err != nil {
		return nil, err
	}
	return ips, nil
}

// ListExpiredLeases returns leases whose expiry is older than the cutoff.
func (r *MySQLRepository) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if r.activeStore != nil {
		leases := make([]models.Lease, 0, limit)
		seen := make(map[string]struct{}, limit)
		expiryPattern := "lease-active-expiry:*"
		expiryPrefix := "lease-active-expiry:"
		beforeScore := fmt.Sprintf("%d", before.Unix())
		var cursor uint64
		for len(leases) < limit {
			keys, next, err := r.activeStore.Scan(ctx, cursor, expiryPattern, 200).Result()
			if err != nil {
				return nil, err
			}
			for _, expiryKey := range keys {
				tenantID := strings.TrimPrefix(expiryKey, expiryPrefix)
				if strings.TrimSpace(tenantID) == "" {
					continue
				}
				remaining := int64(limit - len(leases))
				if remaining <= 0 {
					break
				}
				leaseIDs, rangeErr := r.activeStore.ZRangeByScore(ctx, expiryKey, &redis.ZRangeBy{
					Min:    "-inf",
					Max:    beforeScore,
					Offset: 0,
					Count:  remaining,
				}).Result()
				if rangeErr != nil {
					continue
				}
				for _, leaseID := range leaseIDs {
					if _, ok := seen[leaseID]; ok {
						continue
					}
					payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID)).Bytes()
					if getErr == redis.Nil {
						_ = r.activeStore.ZRem(ctx, expiryKey, leaseID).Err()
						continue
					}
					if getErr != nil {
						continue
					}
					var lease models.Lease
					if err := json.Unmarshal(payload, &lease); err != nil {
						continue
					}
					if lease.ExpiresAt.After(before) {
						continue
					}
					state := strings.ToUpper(strings.TrimSpace(lease.State))
					if state != "ACTIVE" && state != "DECLINED" && state != "COOLDOWN" && state != "QUARANTINED" {
						continue
					}
					seen[leaseID] = struct{}{}
					leases = append(leases, lease)
					if len(leases) >= limit {
						break
					}
				}
				if len(leases) >= limit {
					break
				}
			}
			if next == 0 || len(leases) >= limit {
				break
			}
			cursor = next
		}
		if len(leases) == 0 {
			if rebuildErr := r.redisRebuildActiveIndexesFromObjects(ctx); rebuildErr == nil {
				cursor = 0
				for len(leases) < limit {
					keys, next, err := r.activeStore.Scan(ctx, cursor, expiryPattern, 200).Result()
					if err != nil {
						break
					}
					for _, expiryKey := range keys {
						tenantID := strings.TrimPrefix(expiryKey, expiryPrefix)
						if strings.TrimSpace(tenantID) == "" {
							continue
						}
						remaining := int64(limit - len(leases))
						if remaining <= 0 {
							break
						}
						leaseIDs, rangeErr := r.activeStore.ZRangeByScore(ctx, expiryKey, &redis.ZRangeBy{Min: "-inf", Max: beforeScore, Offset: 0, Count: remaining}).Result()
						if rangeErr != nil {
							continue
						}
						for _, leaseID := range leaseIDs {
							if _, ok := seen[leaseID]; ok {
								continue
							}
							payload, getErr := r.activeStore.Get(ctx, r.redisActiveLeaseObjectKey(tenantID, leaseID)).Bytes()
							if getErr != nil {
								continue
							}
							var lease models.Lease
							if err := json.Unmarshal(payload, &lease); err != nil {
								continue
							}
							if lease.ExpiresAt.After(before) {
								continue
							}
							state := strings.ToUpper(strings.TrimSpace(lease.State))
							if state != "ACTIVE" && state != "DECLINED" && state != "COOLDOWN" && state != "QUARANTINED" {
								continue
							}
							seen[leaseID] = struct{}{}
							leases = append(leases, lease)
							if len(leases) >= limit {
								break
							}
						}
						if len(leases) >= limit {
							break
						}
					}
					if next == 0 || len(leases) >= limit {
						break
					}
					cursor = next
				}
			}

			// Backward compatibility for pre-expiry-index data.
			cursor = 0
			for len(leases) < limit {
				keys, next, err := r.activeStore.Scan(ctx, cursor, "lease-active-obj:*:*", 200).Result()
				if err != nil {
					break
				}
				for _, key := range keys {
					payload, getErr := r.activeStore.Get(ctx, key).Bytes()
					if getErr != nil {
						continue
					}
					var lease models.Lease
					if err := json.Unmarshal(payload, &lease); err != nil {
						continue
					}
					if _, ok := seen[lease.ID]; ok {
						continue
					}
					if lease.ExpiresAt.After(before) {
						continue
					}
					state := strings.ToUpper(strings.TrimSpace(lease.State))
					if state != "ACTIVE" && state != "DECLINED" && state != "COOLDOWN" && state != "QUARANTINED" {
						continue
					}
					seen[lease.ID] = struct{}{}
					leases = append(leases, lease)
					if len(leases) >= limit {
						break
					}
				}
				if next == 0 || len(leases) >= limit {
					break
				}
				cursor = next
			}
		}
		sort.Slice(leases, func(i, j int) bool {
			return leases[i].ExpiresAt.Before(leases[j].ExpiresAt)
		})
		return leases, nil
	}
	const baseQuery = `
SELECT *
FROM leases_v4
WHERE expires_at <= ?
		AND state IN ('ACTIVE','DECLINED','COOLDOWN','QUARANTINED')
ORDER BY expires_at ASC
LIMIT ?`
	var leases []models.Lease
	if err := r.db.SelectContext(ctx, &leases, baseQuery, before, limit); err != nil {
		return nil, err
	}
	return leases, nil
}

// ArchiveLeases moves historical lease snapshots into lease_history_v4.
func (r *MySQLRepository) ArchiveLeases(ctx context.Context, leases []models.Lease, archivedAt time.Time) error {
	if len(leases) == 0 {
		return nil
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const stmt = `
INSERT INTO lease_history_v4 (
	lease_id, tenant_id, pool_id, ip_address, hardware_addr, client_id, user_id,
	mobility_anchor_id, device_type, last_access_point_id, last_controller_id, last_geo_zone,
	mobility_location_hint, mdm_managed, mdm_source, mdm_tags, mdm_observed_at,
	relay_info, session_continuity, expires_at, final_state, security_state, cooldown_until,
	conflict_history, created_at, updated_at, archived_at
) VALUES (
	?, ?, ?, ?, ?, ?, ?,
	?, ?, ?, ?, ?,
	?, ?, ?, ?, ?,
	?, ?, ?, ?, ?, ?,
	?, ?, ?, ?
)
ON DUPLICATE KEY UPDATE
	final_state=VALUES(final_state),
	security_state=VALUES(security_state),
	updated_at=VALUES(updated_at),
	archived_at=VALUES(archived_at)`

	for i := range leases {
		item := leases[i]
		tenantID := strings.TrimSpace(item.TenantID)
		if tenantID == "" {
			tenantID = "global"
		}
		if _, err = tx.ExecContext(ctx, stmt,
			item.ID,
			tenantID,
			item.PoolID,
			item.IPAddress,
			item.HardwareAddr,
			nullIfEmpty(item.ClientID),
			nullIfEmpty(item.UserID),
			item.MobilityAnchorID,
			nullIfEmpty(item.DeviceType),
			item.LastAccessPointID,
			item.LastControllerID,
			item.LastGeoZone,
			item.MobilityLocationHint,
			item.MDMManaged,
			nullIfEmpty(item.MDMSource),
			emptyJSONAsNull(item.MDMTags),
			item.MDMObservedAt,
			emptyJSONAsNull(item.RelayInfo),
			emptyJSONAsNull(item.SessionContinuity),
			item.ExpiresAt,
			item.State,
			emptyDefault(item.SecurityState, "OK"),
			item.CooldownUntil,
			emptyJSONAsNull(item.ConflictHistory),
			item.CreatedAt,
			item.UpdatedAt,
			archivedAt,
		); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

// PurgeLeaseHistory deletes archived lease history records older than cutoff.
func (r *MySQLRepository) PurgeLeaseHistory(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	const stmt = `
DELETE FROM lease_history_v4
WHERE archived_at < ?
ORDER BY archived_at ASC
LIMIT ?`
	res, err := r.db.ExecContext(ctx, stmt, before, limit)
	if err != nil {
		return 0, err
	}
	rows, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return 0, rowsErr
	}
	return rows, nil
}

// CleanupLegacyActiveSnapshots removes stale ACTIVE rows left in MySQL during Redis-first migration.
func (r *MySQLRepository) CleanupLegacyActiveSnapshots(ctx context.Context, limit int) (int64, error) {
	if r.activeStore == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	const stmt = `
DELETE FROM leases_v4
WHERE state = 'ACTIVE'
ORDER BY updated_at ASC
LIMIT ?`
	res, err := r.db.ExecContext(ctx, stmt, limit)
	if err != nil {
		return 0, err
	}
	rows, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return 0, rowsErr
	}
	return rows, nil
}

func nullIfEmpty(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func emptyDefault(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func emptyJSONAsNull(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

// ListPrefixLeases returns prefix delegations for a tenant.
func (r *MySQLRepository) ListPrefixLeases(ctx context.Context, scope resource.AccessScope, state string, limit, offset int) ([]models.PrefixLease, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	query := "SELECT * FROM prefix_leases_v6 WHERE 1=1"
	args := []any{}
	if state != "" {
		query += " AND state = ?"
		args = append(args, state)
	}
	query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	var leases []models.PrefixLease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, args...); err != nil {
		return nil, err
	}
	return leases, nil
}

// SearchLeaseHistory returns lease records for a tenant filtered by metadata/time window.
func (r *MySQLRepository) SearchLeaseHistory(ctx context.Context, scope resource.AccessScope, filter models.LeaseHistoryFilter) ([]models.Lease, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, 0, err
	}
	whereClause, args := buildLeaseHistoryFilters(filter)
	query := `SELECT
	h.lease_id AS id,
	h.tenant_id,
	h.pool_id,
	h.ip_address,
	h.hardware_addr,
	h.client_id,
	h.user_id,
	h.mobility_anchor_id,
	h.device_type,
	h.last_access_point_id,
	h.last_controller_id,
	h.last_geo_zone,
	h.mobility_location_hint,
	h.mdm_managed,
	h.mdm_source,
	h.mdm_tags,
	h.mdm_observed_at,
	h.relay_info,
	h.session_continuity,
	h.expires_at,
	h.final_state AS state,
	h.security_state,
	h.cooldown_until,
	h.conflict_history,
	h.created_at,
	h.updated_at,
	COUNT(*) OVER () AS total_count
FROM lease_history_v4 h
WHERE 1=1`
	params := []any{}
	if whereClause != "" {
		query += " AND " + whereClause
		params = append(params, args...)
	}
	query += " ORDER BY h.archived_at DESC LIMIT ? OFFSET ?"
	params = append(params, limit, offset)
	type leaseHistoryRow struct {
		models.Lease
		Total int `db:"total_count"`
	}
	var rows []leaseHistoryRow
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	if err := db.SelectContext(ctx, &rows, query, params...); err != nil {
		return nil, 0, err
	}
	leases := make([]models.Lease, len(rows))
	total := 0
	for i, row := range rows {
		leases[i] = row.Lease
		if i == 0 {
			total = row.Total
		}
	}
	if len(rows) == 0 && offset > 0 {
		count, err := r.countLeaseHistory(ctx, scope, filter)
		if err != nil {
			return nil, 0, err
		}
		return leases, count, nil
	}
	return leases, total, nil
}

func buildLeaseHistoryFilters(filter models.LeaseHistoryFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if state := strings.TrimSpace(filter.State); state != "" {
		clauses = append(clauses, "h.final_state = ?")
		args = append(args, strings.ToUpper(state))
	}
	if poolID := strings.TrimSpace(filter.PoolID); poolID != "" {
		clauses = append(clauses, "h.pool_id = ?")
		args = append(args, poolID)
	}
	if identifier := strings.TrimSpace(filter.Identifier); identifier != "" {
		clauses = append(clauses, "(h.hardware_addr = ? OR h.client_id = ?)")
		args = append(args, identifier, identifier)
	}
	if ip := strings.TrimSpace(filter.IPAddress); ip != "" {
		clauses = append(clauses, "h.ip_address = ?")
		args = append(args, ip)
	}
	if !filter.From.IsZero() {
		clauses = append(clauses, "h.archived_at >= ?")
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		clauses = append(clauses, "h.archived_at <= ?")
		args = append(args, filter.To)
	}
	return strings.Join(clauses, " AND "), args
}

func (r *MySQLRepository) countLeaseHistory(ctx context.Context, scope resource.AccessScope, filter models.LeaseHistoryFilter) (int, error) {
	whereClause, args := buildLeaseHistoryFilters(filter)
	query := `SELECT COUNT(*) FROM lease_history_v4 h WHERE 1=1`
	params := []any{}
	if whereClause != "" {
		query += " AND " + whereClause
		params = append(params, args...)
	}
	var total int
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &total, query, params...); err != nil {
		return 0, err
	}
	return total, nil
}

// GetActivePrefixLease fetches a delegated prefix tracked per client/IAPD.
func (r *MySQLRepository) GetActivePrefixLease(ctx context.Context, scope resource.AccessScope, clientID string, iapdID uint32) (*models.PrefixLease, error) {
	const query = `
SELECT *
FROM prefix_leases_v6
WHERE client_id = ?
  AND iapd_id = ?
  AND state = 'ACTIVE'
LIMIT 1`
	var lease models.PrefixLease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, clientID, iapdID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lease, nil
}

// GetPrefixLeaseByID fetches a delegated prefix by primary key.
func (r *MySQLRepository) GetPrefixLeaseByID(ctx context.Context, scope resource.AccessScope, prefixLeaseID string) (*models.PrefixLease, error) {
	const query = `
SELECT *
FROM prefix_leases_v6
WHERE id = ?
LIMIT 1`
	var lease models.PrefixLease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, prefixLeaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lease, nil
}

// CreatePrefixLease inserts a newly delegated prefix.
func (r *MySQLRepository) CreatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	db, err := r.tenantDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO prefix_leases_v6 (
	id, pool_id, client_id, iapd_id,
	prefix, prefix_length, state, expires_at, created_at, updated_at)
VALUES (
	:id, :pool_id, :client_id, :iapd_id,
	:prefix, :prefix_length, :state, :expires_at, :created_at, :updated_at)`, lease)
	r.evictTenant(ctx, "")
	return err
}

// UpdatePrefixLease updates expiry metadata for a delegated prefix.
func (r *MySQLRepository) UpdatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	db, err := r.tenantDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
UPDATE prefix_leases_v6 SET
	prefix = :prefix,
	prefix_length = :prefix_length,
	state = :state,
	expires_at = :expires_at,
	updated_at = :updated_at
WHERE id = :id`, lease)
	r.evictTenant(ctx, "")
	return err
}

// ListActivePrefixes lists delegated prefixes for a pool.
func (r *MySQLRepository) ListActivePrefixes(ctx context.Context, scope resource.AccessScope, poolID string) ([]models.PrefixLease, error) {
	const query = `SELECT * FROM prefix_leases_v6 WHERE pool_id = ? AND state = 'ACTIVE'`
	var leases []models.PrefixLease
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, poolID); err != nil {
		return nil, err
	}
	return leases, nil
}
