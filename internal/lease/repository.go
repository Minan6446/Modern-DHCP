package lease

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/storage"
	"modern-dhcp/pkg/models"
)

// Repository defines persistence methods for leases.
type Repository interface {
	GetActiveLease(ctx context.Context, tenantID, identifier string) (*models.Lease, error)
	GetLeaseByID(ctx context.Context, tenantID, leaseID string) (*models.Lease, error)
	CreateLease(ctx context.Context, lease *models.Lease) error
	UpdateLease(ctx context.Context, lease *models.Lease) error
	ListLeasesByState(ctx context.Context, tenantID, poolID, state string, limit int) ([]models.Lease, error)
	CountActiveLeases(ctx context.Context, tenantID string) (int, error)
	CountActiveLeasesByIdentifier(ctx context.Context, tenantID, identifier string) (int, error)
	CountActiveLeasesByUser(ctx context.Context, tenantID, userID string) (int, error)
	CountActiveLeasesByPool(ctx context.Context, tenantID string, poolIDs []string) (map[string]int64, error)
	CountActiveLeasesByDeviceType(ctx context.Context, tenantID string) (map[string]int64, error)
	UpdateSecurityState(ctx context.Context, tenantID, identifier, state string, updatedAt time.Time) error
	UpdateSecurityStateByID(ctx context.Context, tenantID, leaseID, state string, updatedAt time.Time) error
	ListLeases(ctx context.Context, tenantID string, state string, limit, offset int) ([]models.Lease, error)
	ListActiveIPs(ctx context.Context, tenantID, poolID string) ([]string, error)
	ListCooldownIPs(ctx context.Context, tenantID, poolID string, reference time.Time) ([]string, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error)
	GetActivePrefixLease(ctx context.Context, tenantID, clientID string, iapdID uint32) (*models.PrefixLease, error)
	GetPrefixLeaseByID(ctx context.Context, tenantID, prefixLeaseID string) (*models.PrefixLease, error)
	CreatePrefixLease(ctx context.Context, lease *models.PrefixLease) error
	UpdatePrefixLease(ctx context.Context, lease *models.PrefixLease) error
	ListActivePrefixes(ctx context.Context, tenantID, poolID string) ([]models.PrefixLease, error)
	ListPrefixLeases(ctx context.Context, tenantID, state string, limit, offset int) ([]models.PrefixLease, error)
	SearchLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) ([]models.Lease, error)
	CountLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) (int, error)
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

// MySQLRepository implements Repository using sqlx.
type MySQLRepository struct {
	db     *sqlx.DB
	router tenantHandleProvider
}

// NewRepository builds a MySQL-backed lease repository.
func NewRepository(db *sqlx.DB, opts ...RepositoryOption) *MySQLRepository {
	repo := &MySQLRepository{db: db}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

// ErrNotFound indicates missing row.
var ErrNotFound = errors.New("lease not found")

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

// GetActiveLease fetches a lease by identifier (MAC/client-id).
func (r *MySQLRepository) GetActiveLease(ctx context.Context, tenantID, identifier string) (*models.Lease, error) {
	const query = `
SELECT *
FROM leases_v4
WHERE tenant_id = ?
  AND (hardware_addr = ? OR client_id = ?)
  AND state = 'ACTIVE'
LIMIT 1`

	var lease models.Lease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, tenantID, identifier, identifier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &lease, nil
}

// GetLeaseByID fetches a lease regardless of state.
func (r *MySQLRepository) GetLeaseByID(ctx context.Context, tenantID, leaseID string) (*models.Lease, error) {
	const query = `
SELECT *
FROM leases_v4
WHERE tenant_id = ?
  AND id = ?
LIMIT 1`
	var lease models.Lease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, tenantID, leaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lease, nil
}

// CreateLease inserts a new lease.
func (r *MySQLRepository) CreateLease(ctx context.Context, lease *models.Lease) error {
	db, err := r.tenantDB(ctx, lease.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO leases_v4 (
	id, tenant_id, pool_id, ip_address, hardware_addr, client_id,
	user_id, mobility_anchor_id, device_type, last_access_point_id, last_controller_id,
	last_geo_zone, mobility_location_hint, mdm_managed, mdm_source, mdm_tags, mdm_observed_at,
	relay_info, session_continuity,
	expires_at, state, security_state, cooldown_until, conflict_history, created_at, updated_at)
VALUES (
	:id, :tenant_id, :pool_id, :ip_address, :hardware_addr, :client_id,
	:user_id, :mobility_anchor_id, :device_type, :last_access_point_id, :last_controller_id,
	:last_geo_zone, :mobility_location_hint, :mdm_managed, :mdm_source, :mdm_tags, :mdm_observed_at,
	:relay_info, :session_continuity,
	:expires_at, :state, :security_state, :cooldown_until, :conflict_history, :created_at, :updated_at)`, lease)
	return err
}

// UpdateLease persists changes to an existing lease.
func (r *MySQLRepository) UpdateLease(ctx context.Context, lease *models.Lease) error {
	db, err := r.tenantDB(ctx, lease.TenantID)
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
WHERE id = :id AND tenant_id = :tenant_id`, lease)
	return err
}

// ListLeasesByState returns leases from a pool filtered by state.
func (r *MySQLRepository) ListLeasesByState(ctx context.Context, tenantID, poolID, state string, limit int) ([]models.Lease, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	const query = `
SELECT *
FROM leases_v4
WHERE tenant_id = ?
	AND pool_id = ?
	AND state = ?
ORDER BY updated_at ASC
LIMIT ?`
	var leases []models.Lease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, tenantID, poolID, state, limit); err != nil {
		return nil, err
	}
	return leases, nil
}

// CountActiveLeases returns how many active leases exist for the tenant.
func (r *MySQLRepository) CountActiveLeases(ctx context.Context, tenantID string) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE tenant_id = ?
	AND state = 'ACTIVE'`
	var count int
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query, tenantID); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByIdentifier returns how many active leases exist for the MAC/client identifier.
func (r *MySQLRepository) CountActiveLeasesByIdentifier(ctx context.Context, tenantID, identifier string) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE tenant_id = ?
	AND state = 'ACTIVE'
	AND (hardware_addr = ? OR client_id = ?)`
	var count int
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query, tenantID, identifier, identifier); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByUser returns how many active leases are bound to a given user identifier.
func (r *MySQLRepository) CountActiveLeasesByUser(ctx context.Context, tenantID, userID string) (int, error) {
	const query = `
SELECT COUNT(*)
FROM leases_v4
WHERE tenant_id = ?
	AND state = 'ACTIVE'
	AND user_id = ?`
	var count int
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &count, query, tenantID, userID); err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveLeasesByPool returns the number of active leases per pool.
func (r *MySQLRepository) CountActiveLeasesByPool(ctx context.Context, tenantID string, poolIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(poolIDs))
	if len(poolIDs) == 0 {
		return counts, nil
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	query, args, err := sqlx.In(`
SELECT pool_id, COUNT(*) AS total
FROM leases_v4
WHERE tenant_id = ?
	AND state = 'ACTIVE'
	AND pool_id IN (?)
GROUP BY pool_id`, tenantID, poolIDs)
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
func (r *MySQLRepository) CountActiveLeasesByDeviceType(ctx context.Context, tenantID string) (map[string]int64, error) {
	const query = `
SELECT COALESCE(NULLIF(device_type, ''), 'unknown') AS device_type,
	   COUNT(*) AS total
FROM leases_v4
WHERE tenant_id = ?
	AND state = 'ACTIVE'
GROUP BY COALESCE(NULLIF(device_type, ''), 'unknown')`
	var rows []struct {
		DeviceType string `db:"device_type"`
		Count      int64  `db:"total"`
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &rows, query, tenantID); err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.DeviceType] = row.Count
	}
	return counts, nil
}

// UpdateSecurityState updates the security posture for active leases tied to an identifier.
func (r *MySQLRepository) UpdateSecurityState(ctx context.Context, tenantID, identifier, state string, updatedAt time.Time) error {
	const query = `
UPDATE leases_v4
SET security_state = ?, updated_at = ?
WHERE tenant_id = ? AND (hardware_addr = ? OR client_id = ?)`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, query, state, updatedAt, tenantID, identifier, identifier)
	return err
}

// UpdateSecurityStateByID updates the security posture for a lease referenced by its ID.
func (r *MySQLRepository) UpdateSecurityStateByID(ctx context.Context, tenantID, leaseID, state string, updatedAt time.Time) error {
	const query = `
UPDATE leases_v4
SET security_state = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, query, state, updatedAt, tenantID, leaseID)
	return err
}

// ListLeases returns leases filtered by optional state.
func (r *MySQLRepository) ListLeases(ctx context.Context, tenantID string, state string, limit, offset int) ([]models.Lease, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := "SELECT * FROM leases_v4 WHERE tenant_id = ?"
	args := []any{tenantID}
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
func (r *MySQLRepository) ListActiveIPs(ctx context.Context, tenantID, poolID string) ([]string, error) {
	const query = `SELECT ip_address FROM leases_v4 WHERE tenant_id = ? AND pool_id = ? AND state = 'ACTIVE'`
	var ips []string
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &ips, query, tenantID, poolID); err != nil {
		return nil, err
	}
	return ips, nil
}

// ListCooldownIPs returns addresses still under cooldown for the pool.
func (r *MySQLRepository) ListCooldownIPs(ctx context.Context, tenantID, poolID string, reference time.Time) ([]string, error) {
	const query = `
SELECT ip_address
FROM leases_v4
WHERE tenant_id = ?
	AND pool_id = ?
	AND (
				(cooldown_until IS NOT NULL AND cooldown_until > ?)
		 OR state = 'QUARANTINED'
			)`
	var ips []string
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &ips, query, tenantID, poolID, reference); err != nil {
		return nil, err
	}
	return ips, nil
}

// ListExpiredLeases returns leases whose expiry is older than the cutoff.
func (r *MySQLRepository) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
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

// ListPrefixLeases returns prefix delegations for a tenant.
func (r *MySQLRepository) ListPrefixLeases(ctx context.Context, tenantID, state string, limit, offset int) ([]models.PrefixLease, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := "SELECT * FROM prefix_leases_v6 WHERE tenant_id = ?"
	args := []any{tenantID}
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
func (r *MySQLRepository) SearchLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) ([]models.Lease, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	whereClause, args := buildLeaseHistoryFilters(filter)
	query := `SELECT * FROM leases_v4 WHERE tenant_id = ?`
	params := []any{tenantID}
	if whereClause != "" {
		query += " AND " + whereClause
		params = append(params, args...)
	}
	query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	params = append(params, limit, offset)
	var leases []models.Lease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, params...); err != nil {
		return nil, err
	}
	return leases, nil
}

// CountLeaseHistory returns total rows matching the supplied history filter.
func (r *MySQLRepository) CountLeaseHistory(ctx context.Context, tenantID string, filter models.LeaseHistoryFilter) (int, error) {
	whereClause, args := buildLeaseHistoryFilters(filter)
	query := `SELECT COUNT(*) FROM leases_v4 WHERE tenant_id = ?`
	params := []any{tenantID}
	if whereClause != "" {
		query += " AND " + whereClause
		params = append(params, args...)
	}
	var total int
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if err := db.GetContext(ctx, &total, query, params...); err != nil {
		return 0, err
	}
	return total, nil
}

func buildLeaseHistoryFilters(filter models.LeaseHistoryFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if state := strings.TrimSpace(filter.State); state != "" {
		clauses = append(clauses, "state = ?")
		args = append(args, strings.ToUpper(state))
	}
	if identifier := strings.TrimSpace(filter.Identifier); identifier != "" {
		clauses = append(clauses, "(hardware_addr = ? OR client_id = ?)")
		args = append(args, identifier, identifier)
	}
	if ip := strings.TrimSpace(filter.IPAddress); ip != "" {
		clauses = append(clauses, "ip_address = ?")
		args = append(args, ip)
	}
	if !filter.From.IsZero() {
		clauses = append(clauses, "updated_at >= ?")
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		clauses = append(clauses, "updated_at <= ?")
		args = append(args, filter.To)
	}
	return strings.Join(clauses, " AND "), args
}

// GetActivePrefixLease fetches a delegated prefix tracked per client/IAPD.
func (r *MySQLRepository) GetActivePrefixLease(ctx context.Context, tenantID, clientID string, iapdID uint32) (*models.PrefixLease, error) {
	const query = `
SELECT *
FROM prefix_leases_v6
WHERE tenant_id = ?
  AND client_id = ?
  AND iapd_id = ?
  AND state = 'ACTIVE'
LIMIT 1`
	var lease models.PrefixLease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, tenantID, clientID, iapdID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lease, nil
}

// GetPrefixLeaseByID fetches a delegated prefix by primary key.
func (r *MySQLRepository) GetPrefixLeaseByID(ctx context.Context, tenantID, prefixLeaseID string) (*models.PrefixLease, error) {
	const query = `
SELECT *
FROM prefix_leases_v6
WHERE tenant_id = ?
  AND id = ?
LIMIT 1`
	var lease models.PrefixLease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &lease, query, tenantID, prefixLeaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lease, nil
}

// CreatePrefixLease inserts a newly delegated prefix.
func (r *MySQLRepository) CreatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	db, err := r.tenantDB(ctx, lease.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO prefix_leases_v6 (
	id, tenant_id, pool_id, client_id, iapd_id,
	prefix, prefix_length, state, expires_at, created_at, updated_at)
VALUES (
	:id, :tenant_id, :pool_id, :client_id, :iapd_id,
	:prefix, :prefix_length, :state, :expires_at, :created_at, :updated_at)`, lease)
	return err
}

// UpdatePrefixLease updates expiry metadata for a delegated prefix.
func (r *MySQLRepository) UpdatePrefixLease(ctx context.Context, lease *models.PrefixLease) error {
	db, err := r.tenantDB(ctx, lease.TenantID)
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
WHERE id = :id AND tenant_id = :tenant_id`, lease)
	return err
}

// ListActivePrefixes lists delegated prefixes for a pool.
func (r *MySQLRepository) ListActivePrefixes(ctx context.Context, tenantID, poolID string) ([]models.PrefixLease, error) {
	const query = `SELECT * FROM prefix_leases_v6 WHERE tenant_id = ? AND pool_id = ? AND state = 'ACTIVE'`
	var leases []models.PrefixLease
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &leases, query, tenantID, poolID); err != nil {
		return nil, err
	}
	return leases, nil
}
