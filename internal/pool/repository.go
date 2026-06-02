package pool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"modern-dhcp/internal/cache"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/resource"
	"modern-dhcp/internal/storage"
	"modern-dhcp/pkg/models"
)

// Repository exposes persistence methods for address pools and bindings.
type Repository interface {
	ListPools(ctx context.Context, scope resource.AccessScope, limit, offset int) ([]models.AddressPool, error)
	ListPoolsByFamily(ctx context.Context, scope resource.AccessScope, family, limit, offset int) ([]models.AddressPool, error)
	GetLeaseProfile(ctx context.Context, scope resource.AccessScope, profileID string) (*models.LeaseProfile, error)
	CountPools(ctx context.Context, tenantID string) (int, error)
	CountPoolsByFamily(ctx context.Context, tenantID string, family int) (int, error)
	GetPool(ctx context.Context, scope resource.AccessScope, poolID string) (*models.AddressPool, error)
	InsertPool(ctx context.Context, pool *models.AddressPool) error

	UpdatePool(ctx context.Context, pool *models.AddressPool) error
	DeletePool(ctx context.Context, scope resource.AccessScope, poolID string) error
	FindPools(ctx context.Context, scope resource.AccessScope, filter MetadataFilter) ([]models.AddressPool, error)
	ResolvePool(ctx context.Context, scope resource.AccessScope, selector MetadataSelector) (*models.AddressPool, error)

	ListBindings(ctx context.Context, scope resource.AccessScope, filter BindingFilter, limit, offset int) ([]models.StaticBinding, error)
	CountBindings(ctx context.Context, scope resource.AccessScope, filter BindingFilter) (int, error)
	CountBindingsByStatus(ctx context.Context, scope resource.AccessScope, filter BindingFilter) (map[string]int, error)
	ListBindingsByPool(ctx context.Context, scope resource.AccessScope, poolID string) ([]models.StaticBinding, error)
	GetBinding(ctx context.Context, scope resource.AccessScope, bindingID string) (*models.StaticBinding, error)
	UpsertPoolUsageDaily(ctx context.Context, tenantID string, entries []models.PoolUsageDaily) error
	ListPoolUsageDaily(ctx context.Context, tenantID, poolID string, from, to time.Time) ([]models.PoolUsageDaily, error)
	FindBinding(ctx context.Context, scope resource.AccessScope, identifier, ip string) (*models.StaticBinding, error)
	InsertBinding(ctx context.Context, binding *models.StaticBinding) error
	UpdateBinding(ctx context.Context, binding *models.StaticBinding) error
	UpdateBindingStatus(ctx context.Context, scope resource.AccessScope, bindingID string, status, source string, lastSeenAt time.Time) error
	DeleteBinding(ctx context.Context, scope resource.AccessScope, bindingID string) error
	EvictTenantCache(ctx context.Context, tenantID string)
	EvictAllCache(ctx context.Context)
}

// tenantHandleProvider describes the subset of storage.TenantRouter we rely on.
type tenantHandleProvider interface {
	Handle(ctx context.Context, tenantID string) (storage.TenantHandle, error)
}

// RepositoryOption customizes repository behavior.
type RepositoryOption func(*MySQLRepository)

// WithTenantRouter enables tenant-aware routing for repository queries.
func WithTenantRouter(router tenantHandleProvider) RepositoryOption {
	return func(r *MySQLRepository) {
		r.router = router
	}
}

// WithLogger enables slow-query logging.
func WithLogger(logger *zap.Logger) RepositoryOption {
	return func(r *MySQLRepository) {
		r.logger = logger
	}
}

// WithMetrics enables DB query latency metrics.
func WithMetrics(collector *metrics.Collector) RepositoryOption {
	return func(r *MySQLRepository) {
		r.metrics = collector
	}
}

// WithSlowQueryThreshold customizes slow query logging threshold.
func WithSlowQueryThreshold(threshold time.Duration) RepositoryOption {
	return func(r *MySQLRepository) {
		if threshold > 0 {
			r.slowThreshold = threshold
		}
	}
}

// MySQLRepository implements Repository using sqlx.
type MySQLRepository struct {
	db            *sqlx.DB
	router        tenantHandleProvider
	logger        *zap.Logger
	metrics       *metrics.Collector
	cache         cache.Store
	cacheTTL      time.Duration
	slowThreshold time.Duration
	lagWarnAfter  time.Duration
	lagCheckEvery time.Duration
	lagMu         sync.Mutex
	lagLastCheck  map[string]time.Time
}

// NewRepository builds a repository backed by MySQL.
func NewRepository(db *sqlx.DB, opts ...RepositoryOption) *MySQLRepository {
	repo := &MySQLRepository{
		db:            db,
		slowThreshold: 200 * time.Millisecond,
		cacheTTL:      30 * time.Second,
		lagWarnAfter:  2 * time.Second,
		lagCheckEvery: 30 * time.Second,
		lagLastCheck:  make(map[string]time.Time),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

// WithCache enables L1+L2 caching for hot pool queries.
func WithCache(store cache.Store, ttl time.Duration) RepositoryOption {
	return func(r *MySQLRepository) {
		r.cache = store
		if ttl > 0 {
			r.cacheTTL = ttl
		}
	}
}

// ErrNotFound indicates a missing resource.
var ErrNotFound = sql.ErrNoRows

type rwHandles struct {
	writer     *sqlx.DB
	reader     *sqlx.DB
	readerRole string
}

func (r *MySQLRepository) tenantHandles(ctx context.Context, tenantID string) (rwHandles, error) {
	writer := r.db
	reader := r.db
	readerRole := "primary"
	if tenantID != "" && r.router != nil {
		handle, err := r.router.Handle(ctx, tenantID)
		if err != nil {
			return rwHandles{}, err
		}
		if handle.DB != nil {
			writer = handle.DB
			reader = handle.DB
		}
		if handle.Replica != nil {
			reader = handle.Replica
			readerRole = "replica"
			r.maybeWarnReplicaLag(ctx, tenantID, writer, reader)
		}
		if handle.Schema != "" {
			writer = storage.SchemaAware(writer, handle.Schema)
			reader = storage.SchemaAware(reader, handle.Schema)
		}
	}
	if reader == nil {
		reader = writer
		readerRole = "primary"
	}
	return rwHandles{writer: writer, reader: reader, readerRole: readerRole}, nil
}

func (r *MySQLRepository) maybeWarnReplicaLag(ctx context.Context, tenantID string, writer, reader *sqlx.DB) {
	if r == nil || r.logger == nil || writer == nil || reader == nil || r.lagWarnAfter <= 0 {
		return
	}
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now()
	r.lagMu.Lock()
	last := r.lagLastCheck[tenantID]
	if !last.IsZero() && now.Sub(last) < r.lagCheckEvery {
		r.lagMu.Unlock()
		return
	}
	r.lagLastCheck[tenantID] = now
	r.lagMu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var writerTS, readerTS float64
	if err := writer.GetContext(checkCtx, &writerTS, "SELECT UNIX_TIMESTAMP(UTC_TIMESTAMP(6))"); err != nil {
		return
	}
	if err := reader.GetContext(checkCtx, &readerTS, "SELECT UNIX_TIMESTAMP(UTC_TIMESTAMP(6))"); err != nil {
		return
	}
	lag := writerTS - readerTS
	if lag < 0 {
		lag = -lag
	}
	lagDuration := time.Duration(lag * float64(time.Second))
	if lagDuration >= r.lagWarnAfter {
		r.logger.Warn("read replica lag detected", zap.String("tenant", tenantID), zap.Duration("lag", lagDuration), zap.Duration("threshold", r.lagWarnAfter))
	}
}

func tenantFromAccessScope(scope resource.AccessScope) (string, error) {
	tenant := strings.TrimSpace(scope.EffectiveTenant())
	if tenant == "" {
		return "", resource.ErrScopeRequired
	}
	return tenant, nil
}

func (r *MySQLRepository) cacheKeyPool(tenantID, poolID string) string {
	return fmt.Sprintf("pool:%s:id:%s", tenantID, poolID)
}

func (r *MySQLRepository) cacheKeyPoolList(tenantID string, limit, offset int) string {
	return fmt.Sprintf("pool:%s:list:%d:%d", tenantID, limit, offset)
}

func (r *MySQLRepository) cacheKeyPoolListFamily(tenantID string, family, limit, offset int) string {
	return fmt.Sprintf("pool:%s:list:%d:%d:%d", tenantID, family, limit, offset)
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
	_ = r.cache.DeletePrefix(ctx, fmt.Sprintf("pool:%s:", tenantID))
}

func (r *MySQLRepository) EvictTenantCache(ctx context.Context, tenantID string) {
	r.evictTenant(ctx, strings.TrimSpace(tenantID))
}

func (r *MySQLRepository) EvictAllCache(ctx context.Context) {
	if r == nil || r.cache == nil {
		return
	}
	_ = r.cache.DeletePrefix(ctx, "pool:")
}

// BindingFilter allows filtering static bindings by identifier, IP, or pool.
type BindingFilter struct {
	Identifier     string
	IdentifierType string
	MAC            string
	IP             string
	PoolID         string
}

func (r *MySQLRepository) recordQuery(db *sql.DB, role, operation, tenant string, started time.Time, rows int, err error) {
	if r.logger != nil {
		elapsed := time.Since(started)
		if r.slowThreshold > 0 && elapsed >= r.slowThreshold {
			r.logger.Warn("slow query", zap.String("op", operation), zap.String("tenant", tenant), zap.String("role", role), zap.Duration("duration", elapsed), zap.Int("rows", rows), zap.Error(err))
		}
	}
	if r.metrics != nil && r.metrics.PoolServiceLatency != nil {
		outcome := metricOutcomeSuccess
		if err != nil {
			outcome = metricOutcomeError
		}
		r.metrics.PoolServiceLatency.WithLabelValues(tenant, "repo_"+operation, outcome).Observe(time.Since(started).Seconds())
	}
	r.reportPoolStats(db, role, tenant)
}

func (r *MySQLRepository) ListPools(ctx context.Context, scope resource.AccessScope, limit, offset int) ([]models.AddressPool, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if r.cache != nil {
		var cached []models.AddressPool
		cacheKey := r.cacheKeyPoolList(tenantID, limit, offset)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &cached); cErr == nil && ok {
			r.recordCache("pool", "list", "any", "hit")
			return cached, nil
		} else if cErr == nil {
			r.recordCache("pool", "list", "any", "miss")
		}
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	db := handles.reader
	const query = `SELECT * FROM address_pools ORDER BY created_at DESC LIMIT ? OFFSET ?`
	started := time.Now()
	var pools []models.AddressPool
	if err := db.SelectContext(ctx, &pools, query, limit, offset); err != nil {
		r.recordQuery(db.DB, handles.readerRole, "list_pools", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(db.DB, handles.readerRole, "list_pools", tenantID, started, len(pools), nil)
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyPoolList(tenantID, limit, offset), pools, r.cacheTTL)
	}
	return pools, nil
}

func (r *MySQLRepository) ListPoolsByFamily(ctx context.Context, scope resource.AccessScope, family, limit, offset int) ([]models.AddressPool, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if family != 4 && family != 6 {
		return nil, errors.New("invalid ip family")
	}
	if r.cache != nil {
		var cached []models.AddressPool
		cacheKey := r.cacheKeyPoolListFamily(tenantID, family, limit, offset)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &cached); cErr == nil && ok {
			r.recordCache("pool", "list_family", "any", "hit")
			return cached, nil
		} else if cErr == nil {
			r.recordCache("pool", "list_family", "any", "miss")
		}
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	db := handles.reader
	query := `SELECT * FROM address_pools WHERE 1=1`
	if family == 4 {
		query += " AND cidr LIKE '%.%'"
	} else {
		query += " AND cidr LIKE '%:%'"
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	started := time.Now()
	var pools []models.AddressPool
	if err := db.SelectContext(ctx, &pools, query, limit, offset); err != nil {
		r.recordQuery(db.DB, handles.readerRole, "list_pools_family", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(db.DB, handles.readerRole, "list_pools_family", tenantID, started, len(pools), nil)
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyPoolListFamily(tenantID, family, limit, offset), pools, r.cacheTTL)
	}
	return pools, nil
}

func (r *MySQLRepository) CountPools(ctx context.Context, tenantID string) (int, error) {
	const query = `SELECT COUNT(*) FROM address_pools`
	started := time.Now()
	var count int
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := handles.reader.GetContext(ctx, &count, query); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "count_pools", tenantID, started, 0, err)
		return 0, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "count_pools", tenantID, started, count, nil)
	return count, nil
}

// CountPoolsByFamily returns pool totals by IP family (4 or 6) for a tenant.
func (r *MySQLRepository) CountPoolsByFamily(ctx context.Context, tenantID string, family int) (int, error) {
	query := `SELECT COUNT(*) FROM address_pools WHERE 1=1`
	switch family {
	case 4:
		query += " AND cidr LIKE '%.%'"
	case 6:
		query += " AND cidr LIKE '%:%'"
	default:
		return 0, errors.New("invalid ip family")
	}
	started := time.Now()
	var count int
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := handles.reader.GetContext(ctx, &count, query); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "count_pools_family", tenantID, started, 0, err)
		return 0, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "count_pools_family", tenantID, started, count, nil)
	return count, nil
}

func (r *MySQLRepository) GetPool(ctx context.Context, scope resource.AccessScope, poolID string) (*models.AddressPool, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if r.cache != nil {
		var cached models.AddressPool
		cacheKey := r.cacheKeyPool(tenantID, poolID)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &cached); cErr == nil && ok {
			r.recordCache("pool", "get", "any", "hit")
			return &cached, nil
		} else if cErr == nil {
			r.recordCache("pool", "get", "any", "miss")
		}
	}
	const query = `SELECT * FROM address_pools WHERE id = ?`
	started := time.Now()
	var pool models.AddressPool
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := handles.reader.GetContext(ctx, &pool, query, poolID); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "get_pool", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "get_pool", tenantID, started, 1, nil)
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyPool(tenantID, poolID), pool, r.cacheTTL)
	}
	return &pool, nil
}

func (r *MySQLRepository) GetLeaseProfile(ctx context.Context, scope resource.AccessScope, profileID string) (*models.LeaseProfile, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM lease_profiles WHERE id = ? LIMIT 1`
	started := time.Now()
	var profile models.LeaseProfile
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := handles.reader.GetContext(ctx, &profile, query, strings.TrimSpace(profileID)); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "get_lease_profile", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "get_lease_profile", tenantID, started, 1, nil)
	return &profile, nil
}

func (r *MySQLRepository) InsertPool(ctx context.Context, pool *models.AddressPool) error {
	handles, err := r.tenantHandles(ctx, "")
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.NamedExecContext(ctx, `
INSERT INTO address_pools (
	id, scope, parent_id, name, cidr, network, netmask,
	range_start, range_end, gateway, option_43, dns, exclusions, vlan_id, interface_id,
	ssid, location, reserve_percent, min_lease_time, max_lease_time, lease_profile_id, tags,
	allocation_mode, priority_weight, status,
	created_at, updated_at)
VALUES (
	:id, :scope, :parent_id, :name, :cidr, :network, :netmask,
	:range_start, :range_end, :gateway, :option_43, :dns, :exclusions, :vlan_id, :interface_id,
	:ssid, :location, :reserve_percent, :min_lease_time, :max_lease_time, :lease_profile_id, :tags,
	:allocation_mode, :priority_weight, :status,
	:created_at, :updated_at)`, pool)
	r.recordQuery(handles.writer.DB, "primary", "insert_pool", "", started, 1, err)
	if tenantID := strings.TrimSpace(pool.TenantID); tenantID != "" {
		r.evictTenant(ctx, tenantID)
	} else {
		r.evictTenant(ctx, "")
	}
	return err
}

func (r *MySQLRepository) UpdatePool(ctx context.Context, pool *models.AddressPool) error {
	handles, err := r.tenantHandles(ctx, "")
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.NamedExecContext(ctx, `
UPDATE address_pools SET
	scope = :scope,
	parent_id = :parent_id,
	name = :name,
	cidr = :cidr,
	network = :network,
	netmask = :netmask,
	range_start = :range_start,
	range_end = :range_end,
	gateway = :gateway,
	dns = :dns,
	option_43 = :option_43,
	exclusions = :exclusions,
	vlan_id = :vlan_id,
	interface_id = :interface_id,
	ssid = :ssid,
	location = :location,
	reserve_percent = :reserve_percent,
	min_lease_time = :min_lease_time,
	max_lease_time = :max_lease_time,
	lease_profile_id = :lease_profile_id,
	tags = :tags,
	allocation_mode = :allocation_mode,
	priority_weight = :priority_weight,
	status = :status,
	updated_at = :updated_at
WHERE id = :id`, pool)
	r.recordQuery(handles.writer.DB, "primary", "update_pool", "", started, 1, err)
	if tenantID := strings.TrimSpace(pool.TenantID); tenantID != "" {
		r.evictTenant(ctx, tenantID)
	} else {
		r.evictTenant(ctx, "")
	}
	return err
}

func (r *MySQLRepository) DeletePool(ctx context.Context, scope resource.AccessScope, poolID string) error {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.ExecContext(ctx, `DELETE FROM address_pools WHERE id = ?`, poolID)
	r.recordQuery(handles.writer.DB, "primary", "delete_pool", tenantID, started, 0, err)
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) FindPools(ctx context.Context, scope resource.AccessScope, filter MetadataFilter) ([]models.AddressPool, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	r.evictTenant(ctx, tenantID)
	clauses := []string{"1=1"}
	args := []any{}
	if scope := strings.TrimSpace(strings.ToUpper(filter.Scope)); scope != "" {
		clauses = append(clauses, "scope = ?")
		args = append(args, scope)
	}
	if filter.ParentID != nil && strings.TrimSpace(*filter.ParentID) != "" {
		clauses = append(clauses, "parent_id = ?")
		args = append(args, strings.TrimSpace(*filter.ParentID))
	}
	if filter.VLANID != nil {
		clauses = append(clauses, "vlan_id = ?")
		args = append(args, *filter.VLANID)
	}
	if filter.InterfaceID != nil && strings.TrimSpace(*filter.InterfaceID) != "" {
		clauses = append(clauses, "interface_id = ?")
		args = append(args, strings.TrimSpace(*filter.InterfaceID))
	}
	if filter.SSID != nil && strings.TrimSpace(*filter.SSID) != "" {
		clauses = append(clauses, "ssid = ?")
		args = append(args, strings.TrimSpace(*filter.SSID))
	}
	if filter.Location != nil && strings.TrimSpace(*filter.Location) != "" {
		clauses = append(clauses, "location = ?")
		args = append(args, strings.TrimSpace(*filter.Location))
	}
	if filter.GeoCode != nil && strings.TrimSpace(*filter.GeoCode) != "" {
		clauses = append(clauses, "geo_code = ?")
		args = append(args, strings.TrimSpace(*filter.GeoCode))
	}
	if filter.DeviceProfile != nil && strings.TrimSpace(*filter.DeviceProfile) != "" {
		clauses = append(clauses, "device_profile = ?")
		args = append(args, strings.TrimSpace(*filter.DeviceProfile))
	}
	if filter.TagFingerprint != nil && strings.TrimSpace(*filter.TagFingerprint) != "" {
		clauses = append(clauses, "tag_fingerprint = ?")
		args = append(args, strings.TrimSpace(*filter.TagFingerprint))
	}
	query := strings.Builder{}
	query.WriteString("SELECT * FROM address_pools WHERE ")
	query.WriteString(strings.Join(clauses, " AND "))
	query.WriteString(" ORDER BY scope ASC, created_at DESC")
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query.WriteString(" LIMIT ?")
	args = append(args, limit)
	started := time.Now()
	var pools []models.AddressPool
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := handles.reader.SelectContext(ctx, &pools, query.String(), args...); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "find_pools", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "find_pools", tenantID, started, len(pools), nil)
	return pools, nil
}

const resolvePoolQuery = `
WITH demand AS (
		SELECT pool_id,
					 SUM(CASE WHEN created_at >= NOW() - INTERVAL 30 MINUTE THEN 1 ELSE 0 END) AS recent_alloc,
					 SUM(CASE WHEN created_at >= NOW() - INTERVAL 180 MINUTE THEN 1 ELSE 0 END) AS window_alloc
		FROM leases_v4
		GROUP BY pool_id
),
candidate AS (
		SELECT p.*,
					 (CASE WHEN :interface_id <> '' AND p.interface_id = :interface_id THEN 120 ELSE 0 END) +
					 (CASE WHEN :access_point_id <> '' AND p.interface_id = :access_point_id THEN 110 ELSE 0 END) +
					 (CASE WHEN :ssid <> '' AND p.ssid = :ssid THEN 95 ELSE 0 END) +
					 (CASE WHEN :location <> '' AND p.location = :location THEN 90 ELSE 0 END) +
					 (CASE WHEN :controller_id <> '' AND p.location = :controller_id THEN 85 ELSE 0 END) +
					 (CASE WHEN :geo_zone <> '' AND p.location = :geo_zone THEN 80 ELSE 0 END) +
					 (CASE WHEN :geo_code <> '' AND p.geo_code = :geo_code THEN 85 ELSE 0 END) +
					 (CASE WHEN :device_profile <> '' AND p.device_profile = :device_profile THEN 75 ELSE 0 END) +
					 (CASE WHEN :tag_fingerprint <> '' AND p.tag_fingerprint = :tag_fingerprint THEN 60 ELSE 0 END) +
					 (CASE WHEN :vlan_id > 0 AND p.vlan_id = :vlan_id THEN 50 ELSE 0 END) AS match_score,
					 CASE p.scope
						 WHEN 'PORT' THEN 400
						 WHEN 'VLAN' THEN 300
						 WHEN 'SUBNET' THEN 200
						 WHEN 'GLOBAL' THEN 100
						 ELSE 0
					 END AS scope_score,
					 LEAST(GREATEST(100 - IFNULL(p.reserve_percent, 0), 0), 100) AS capacity_score,
					 LEAST(IFNULL(p.priority_weight, 0), 200) AS priority_score,
					 IFNULL(d.recent_alloc, 0) AS recent_alloc,
					 IFNULL(d.window_alloc, 0) AS window_alloc,
					 (0.6 * IFNULL(d.recent_alloc, 0) + 0.4 * (GREATEST(IFNULL(d.window_alloc, 0) - IFNULL(d.recent_alloc, 0), 0) / 5)) AS predicted_demand,
					 LEAST((0.6 * IFNULL(d.recent_alloc, 0) + 0.4 * (GREATEST(IFNULL(d.window_alloc, 0) - IFNULL(d.recent_alloc, 0), 0) / 5)) * 5, 300) AS predicted_penalty
		FROM address_pools p
		LEFT JOIN demand d ON d.pool_id = p.id
		WHERE p.status = 'active'
			AND (
				(:interface_id <> '' AND p.interface_id = :interface_id) OR
				(:access_point_id <> '' AND p.interface_id = :access_point_id) OR
				(:ssid <> '' AND p.ssid = :ssid) OR
				(:location <> '' AND p.location = :location) OR
				(:controller_id <> '' AND p.location = :controller_id) OR
				(:geo_zone <> '' AND p.location = :geo_zone) OR
				(:geo_code <> '' AND p.geo_code = :geo_code) OR
				(:device_profile <> '' AND p.device_profile = :device_profile) OR
				(:tag_fingerprint <> '' AND p.tag_fingerprint = :tag_fingerprint) OR
				(:vlan_id > 0 AND p.vlan_id = :vlan_id)
			)
)
SELECT *
FROM candidate
WHERE match_score > 0
ORDER BY (match_score + scope_score + priority_score + capacity_score - predicted_penalty) DESC,
				 match_score DESC,
				 scope_score DESC,
				 priority_score DESC,
				 capacity_score DESC,
				 predicted_penalty ASC,
				 updated_at DESC
LIMIT 1`

func (r *MySQLRepository) ResolvePool(ctx context.Context, scope resource.AccessScope, selector MetadataSelector) (*models.AddressPool, error) {
	if selector.InterfaceID == "" && selector.AccessPointID == "" && selector.SSID == "" && selector.Location == "" && selector.ControllerID == "" && selector.GeoZone == "" && selector.GeoCode == "" && selector.DeviceProfile == "" && selector.TagFingerprint == "" && selector.VLANID <= 0 {
		return nil, ErrNotFound
	}
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	args := map[string]any{
		"interface_id":    selector.InterfaceID,
		"access_point_id": selector.AccessPointID,
		"ssid":            selector.SSID,
		"location":        selector.Location,
		"controller_id":   selector.ControllerID,
		"geo_zone":        selector.GeoZone,
		"geo_code":        selector.GeoCode,
		"device_profile":  selector.DeviceProfile,
		"tag_fingerprint": selector.TagFingerprint,
		"vlan_id":         selector.VLANID,
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	selectorStarted := time.Now()
	candidateCount := 0
	if handles.reader != nil {
		const resolvePoolCountQuery = `
SELECT COUNT(*) AS cnt
FROM address_pools p
WHERE p.status = 'active'
  AND (
    (:interface_id <> '' AND p.interface_id = :interface_id) OR
    (:access_point_id <> '' AND p.interface_id = :access_point_id) OR
    (:ssid <> '' AND p.ssid = :ssid) OR
    (:location <> '' AND p.location = :location) OR
    (:controller_id <> '' AND p.location = :controller_id) OR
    (:geo_zone <> '' AND p.location = :geo_zone) OR
    (:geo_code <> '' AND p.geo_code = :geo_code) OR
    (:device_profile <> '' AND p.device_profile = :device_profile) OR
    (:tag_fingerprint <> '' AND p.tag_fingerprint = :tag_fingerprint) OR
    (:vlan_id > 0 AND p.vlan_id = :vlan_id)
  )`
		if rows, err := handles.reader.NamedQueryContext(ctx, resolvePoolCountQuery, args); err == nil {
			defer rows.Close()
			if rows.Next() {
				var c int
				if scanErr := rows.Scan(&c); scanErr == nil {
					candidateCount = c
				}
			}
		}
	}
	started := time.Now()
	rows, err := handles.reader.NamedQueryContext(ctx, resolvePoolQuery, args)
	if err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "resolve_pool", tenantID, started, 0, err)
		r.recordSelectorMetrics(selectorStarted, tenantID, selector, "error", candidateCount)
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var pool models.AddressPool
		if err := rows.StructScan(&pool); err != nil {
			r.recordQuery(handles.reader.DB, handles.readerRole, "resolve_pool", tenantID, started, 0, err)
			r.recordSelectorMetrics(selectorStarted, tenantID, selector, "error", candidateCount)
			return nil, err
		}
		r.recordQuery(handles.reader.DB, handles.readerRole, "resolve_pool", tenantID, started, 1, nil)
		r.recordSelectorMetrics(selectorStarted, tenantID, selector, "hit", candidateCount)
		return &pool, nil
	}
	if err := rows.Err(); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "resolve_pool", tenantID, started, 0, err)
		r.recordSelectorMetrics(selectorStarted, tenantID, selector, "error", candidateCount)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "resolve_pool", tenantID, started, 0, ErrNotFound)
	r.recordSelectorMetrics(selectorStarted, tenantID, selector, "miss", candidateCount)
	return nil, ErrNotFound
}

func (r *MySQLRepository) ListBindings(ctx context.Context, scope resource.AccessScope, filter BindingFilter, limit, offset int) ([]models.StaticBinding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	clauses := []string{"1=1"}
	args := []any{}
	if v := strings.TrimSpace(filter.Identifier); v != "" {
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IdentifierType); v != "" {
		clauses = append(clauses, "identifier_type = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.MAC); v != "" {
		clauses = append(clauses, "identifier_type = 'MAC'")
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IP); v != "" {
		clauses = append(clauses, "ip_address = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.PoolID); v != "" {
		clauses = append(clauses, "pool_id = ?")
		args = append(args, v)
	}
	query := "SELECT * FROM static_bindings WHERE " + strings.Join(clauses, " AND ") + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	started := time.Now()
	var bindings []models.StaticBinding
	if err := handles.reader.SelectContext(ctx, &bindings, query, args...); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "list_bindings", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "list_bindings", tenantID, started, len(bindings), nil)
	return bindings, nil
}

// CountBindings returns total bindings matching the filter for pagination.
func (r *MySQLRepository) CountBindings(ctx context.Context, scope resource.AccessScope, filter BindingFilter) (int, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return 0, err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	clauses := []string{"1=1"}
	args := []any{}
	if v := strings.TrimSpace(filter.Identifier); v != "" {
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IdentifierType); v != "" {
		clauses = append(clauses, "identifier_type = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.MAC); v != "" {
		clauses = append(clauses, "identifier_type = 'MAC'")
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IP); v != "" {
		clauses = append(clauses, "ip_address = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.PoolID); v != "" {
		clauses = append(clauses, "pool_id = ?")
		args = append(args, v)
	}
	query := "SELECT COUNT(*) FROM static_bindings WHERE " + strings.Join(clauses, " AND ")
	started := time.Now()
	var total int
	if err := handles.reader.GetContext(ctx, &total, query, args...); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings", tenantID, started, 0, err)
		return 0, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings", tenantID, started, total, nil)
	return total, nil
}

// CountBindingsByStatus returns counts grouped by status for the given filter.
func (r *MySQLRepository) CountBindingsByStatus(ctx context.Context, scope resource.AccessScope, filter BindingFilter) (map[string]int, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	clauses := []string{"1=1"}
	args := []any{}
	if v := strings.TrimSpace(filter.Identifier); v != "" {
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IdentifierType); v != "" {
		clauses = append(clauses, "identifier_type = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.MAC); v != "" {
		clauses = append(clauses, "identifier_type = 'MAC'")
		clauses = append(clauses, "identifier = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.IP); v != "" {
		clauses = append(clauses, "ip_address = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.PoolID); v != "" {
		clauses = append(clauses, "pool_id = ?")
		args = append(args, v)
	}
	query := "SELECT COALESCE(status, '') AS status, COUNT(*) AS total FROM static_bindings WHERE " + strings.Join(clauses, " AND ") + " GROUP BY status"
	started := time.Now()
	rows, err := handles.reader.QueryxContext(ctx, query, args...)
	if err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings_status", tenantID, started, 0, err)
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var total int
		if scanErr := rows.Scan(&status, &total); scanErr != nil {
			r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings_status", tenantID, started, 0, scanErr)
			return nil, scanErr
		}
		counts[status] = total
	}
	if err := rows.Err(); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings_status", tenantID, started, 0, err)
		return nil, err
	}
	total := 0
	for _, v := range counts {
		total += v
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "count_bindings_status", tenantID, started, total, nil)
	return counts, nil
}

func (r *MySQLRepository) GetBinding(ctx context.Context, scope resource.AccessScope, bindingID string) (*models.StaticBinding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM static_bindings WHERE id = ?`
	started := time.Now()
	var binding models.StaticBinding
	if err := handles.reader.GetContext(ctx, &binding, query, bindingID); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "get_binding", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "get_binding", tenantID, started, 1, nil)
	return &binding, nil
}

func (r *MySQLRepository) FindBinding(ctx context.Context, scope resource.AccessScope, identifier, ip string) (*models.StaticBinding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `
SELECT *
FROM static_bindings
WHERE (
				(identifier <> '' AND identifier = ?)
		 OR (ip_address <> '' AND ip_address = ?)
			)
ORDER BY
	CASE WHEN identifier = ? THEN 0 ELSE 1 END,
	updated_at DESC
LIMIT 1`
	started := time.Now()
	var binding models.StaticBinding
	if err := handles.reader.GetContext(ctx, &binding, query, identifier, ip, identifier); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "find_binding", tenantID, started, 0, err)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "find_binding", tenantID, started, 1, nil)
	return &binding, nil
}

func (r *MySQLRepository) ListBindingsByPool(ctx context.Context, scope resource.AccessScope, poolID string) ([]models.StaticBinding, error) {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return nil, err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM static_bindings WHERE pool_id = ?`
	started := time.Now()
	var bindings []models.StaticBinding
	if err := handles.reader.SelectContext(ctx, &bindings, query, poolID); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "list_bindings_pool", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "list_bindings_pool", tenantID, started, len(bindings), nil)
	return bindings, nil
}

func (r *MySQLRepository) InsertBinding(ctx context.Context, binding *models.StaticBinding) error {
	handles, err := r.tenantHandles(ctx, "")
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.NamedExecContext(ctx, `
INSERT INTO static_bindings (
	id, identifier, identifier_type, pool_id, ip_address,
	lease_profile_id, metadata, status, status_source, last_seen_at, status_updated_at,
	created_at, updated_at)
VALUES (
	:id, :identifier, :identifier_type, :pool_id, :ip_address,
	:lease_profile_id, :metadata, :status, :status_source, :last_seen_at, :status_updated_at,
	:created_at, :updated_at)`, binding)
	r.recordQuery(handles.writer.DB, "primary", "insert_binding", "", started, 1, err)
	r.evictTenant(ctx, "")
	return err
}

func (r *MySQLRepository) UpdateBinding(ctx context.Context, binding *models.StaticBinding) error {
	handles, err := r.tenantHandles(ctx, "")
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.NamedExecContext(ctx, `
UPDATE static_bindings SET
	identifier = :identifier,
	identifier_type = :identifier_type,
	pool_id = :pool_id,
	ip_address = :ip_address,
	lease_profile_id = :lease_profile_id,
	metadata = :metadata,
	status = :status,
	status_source = :status_source,
	last_seen_at = :last_seen_at,
	status_updated_at = :status_updated_at,
	updated_at = :updated_at
WHERE id = :id`, binding)
	r.recordQuery(handles.writer.DB, "primary", "update_binding", "", started, 1, err)
	r.evictTenant(ctx, "")
	return err
}

func (r *MySQLRepository) UpdateBindingStatus(ctx context.Context, scope resource.AccessScope, bindingID string, status, source string, lastSeenAt time.Time) error {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bindingID) == "" {
		return errors.New("binding id required")
	}
	now := time.Now().UTC()
	started := time.Now()
	_, err = handles.writer.ExecContext(ctx, `
UPDATE static_bindings
SET status = ?, status_source = ?, last_seen_at = ?, status_updated_at = ?, updated_at = ?
WHERE id = ?`,
		strings.TrimSpace(status), strings.TrimSpace(source), lastSeenAt, now, now, bindingID)
	r.recordQuery(handles.writer.DB, "primary", "update_binding_status", tenantID, started, 1, err)
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) DeleteBinding(ctx context.Context, scope resource.AccessScope, bindingID string) error {
	tenantID, err := tenantFromAccessScope(scope)
	if err != nil {
		return err
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return err
	}
	started := time.Now()
	_, err = handles.writer.ExecContext(ctx, `DELETE FROM static_bindings WHERE id = ?`, bindingID)
	r.recordQuery(handles.writer.DB, "primary", "delete_binding", tenantID, started, 0, err)
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) UpsertPoolUsageDaily(ctx context.Context, tenantID string, entries []models.PoolUsageDaily) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return errors.New("tenant id required")
	}
	if len(entries) == 0 {
		return nil
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return err
	}
	query := `
INSERT INTO pool_usage_daily (
	tenant_id, pool_id, day, used, capacity, created_at, updated_at
) VALUES (
	:tenant_id, :pool_id, :day, :used, :capacity, :created_at, :updated_at
) ON DUPLICATE KEY UPDATE
	used = VALUES(used),
	capacity = VALUES(capacity),
	updated_at = VALUES(updated_at)`
	started := time.Now()
	tx, err := handles.writer.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for i := range entries {
		entries[i].TenantID = tenantID
		if entries[i].CreatedAt.IsZero() {
			entries[i].CreatedAt = time.Now().UTC()
		}
		entries[i].UpdatedAt = time.Now().UTC()
		if _, execErr := tx.NamedExecContext(ctx, query, entries[i]); execErr != nil {
			err = execErr
			break
		}
	}
	if err != nil {
		r.recordQuery(handles.writer.DB, "primary", "upsert_pool_usage_daily", tenantID, started, 0, err)
		return err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		r.recordQuery(handles.writer.DB, "primary", "upsert_pool_usage_daily", tenantID, started, 0, commitErr)
		return commitErr
	}
	r.recordQuery(handles.writer.DB, "primary", "upsert_pool_usage_daily", tenantID, started, len(entries), nil)
	return nil
}

func (r *MySQLRepository) ListPoolUsageDaily(ctx context.Context, tenantID, poolID string, from, to time.Time) ([]models.PoolUsageDaily, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, errors.New("tenant id required")
	}
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return nil, errors.New("pool id required")
	}
	handles, err := r.tenantHandles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	query := `SELECT tenant_id, pool_id, day, used, capacity, created_at, updated_at
FROM pool_usage_daily
WHERE tenant_id = ? AND pool_id = ? AND day BETWEEN ? AND ?
ORDER BY day ASC`
	started := time.Now()
	rows := []models.PoolUsageDaily{}
	if err := handles.reader.SelectContext(ctx, &rows, query, tenantID, poolID, from, to); err != nil {
		r.recordQuery(handles.reader.DB, handles.readerRole, "list_pool_usage_daily", tenantID, started, 0, err)
		return nil, err
	}
	r.recordQuery(handles.reader.DB, handles.readerRole, "list_pool_usage_daily", tenantID, started, len(rows), nil)
	return rows, nil
}

func (r *MySQLRepository) reportPoolStats(db *sql.DB, role, tenant string) {
	if r.metrics == nil || r.metrics.DBPoolStats == nil || db == nil {
		return
	}
	stats := db.Stats()
	r.metrics.DBPoolStats.WithLabelValues(tenant, role, "open").Set(float64(stats.OpenConnections))
	r.metrics.DBPoolStats.WithLabelValues(tenant, role, "in_use").Set(float64(stats.InUse))
	r.metrics.DBPoolStats.WithLabelValues(tenant, role, "idle").Set(float64(stats.Idle))
	r.metrics.DBPoolStats.WithLabelValues(tenant, role, "max_open").Set(float64(stats.MaxOpenConnections))
}

func (r *MySQLRepository) recordSelectorMetrics(started time.Time, tenant string, selector MetadataSelector, result string, candidates int) {
	if r.metrics == nil {
		return
	}
	if r.metrics.PoolSelectorResolutions != nil {
		scope := "any"
		vlan := ""
		if selector.VLANID > 0 {
			vlan = strconv.Itoa(selector.VLANID)
		}
		r.metrics.PoolSelectorResolutions.WithLabelValues(
			sanitizeLabel(tenant),
			scope,
			sanitizeLabel(vlan),
			sanitizeLabel(selector.InterfaceID),
			sanitizeLabel(selector.SSID),
			sanitizeLabel(selector.Location),
			sanitizeLabel(result),
		).Inc()
	}
	if r.metrics.PoolSelectorLatency != nil {
		r.metrics.PoolSelectorLatency.WithLabelValues(sanitizeLabel(tenant), sanitizeLabel(result)).Observe(time.Since(started).Seconds())
	}
}

func sanitizeLabel(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return metricLabelUnknown
	}
	return val
}
