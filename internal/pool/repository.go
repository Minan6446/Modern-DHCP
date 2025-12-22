package pool

import (
	"context"
	"database/sql"
	"strings"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/storage"
	"modern-dhcp/pkg/models"
)

// Repository exposes persistence methods for address pools and bindings.
type Repository interface {
	ListPools(ctx context.Context, tenantID string, limit, offset int) ([]models.AddressPool, error)
	CountPools(ctx context.Context, tenantID string) (int, error)
	GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error)
	InsertPool(ctx context.Context, pool *models.AddressPool) error
	UpdatePool(ctx context.Context, pool *models.AddressPool) error
	DeletePool(ctx context.Context, tenantID, poolID string) error
	FindPools(ctx context.Context, tenantID string, filter MetadataFilter) ([]models.AddressPool, error)

	ListBindings(ctx context.Context, tenantID string, limit, offset int) ([]models.StaticBinding, error)
	GetBinding(ctx context.Context, tenantID, bindingID string) (*models.StaticBinding, error)
	InsertBinding(ctx context.Context, binding *models.StaticBinding) error
	UpdateBinding(ctx context.Context, binding *models.StaticBinding) error
	DeleteBinding(ctx context.Context, tenantID, bindingID string) error
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

// MySQLRepository implements Repository using sqlx.
type MySQLRepository struct {
	db     *sqlx.DB
	router tenantHandleProvider
}

// NewRepository builds a repository backed by MySQL.
func NewRepository(db *sqlx.DB, opts ...RepositoryOption) *MySQLRepository {
	repo := &MySQLRepository{db: db}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

// ErrNotFound indicates a missing resource.
var ErrNotFound = sql.ErrNoRows

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

func (r *MySQLRepository) ListPools(ctx context.Context, tenantID string, limit, offset int) ([]models.AddressPool, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM address_pools WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
	var pools []models.AddressPool
	if err := db.SelectContext(ctx, &pools, query, tenantID, limit, offset); err != nil {
		return nil, err
	}
	return pools, nil
}

func (r *MySQLRepository) CountPools(ctx context.Context, tenantID string) (int, error) {
	const query = `SELECT COUNT(*) FROM address_pools WHERE tenant_id = ?`
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

func (r *MySQLRepository) GetPool(ctx context.Context, tenantID, poolID string) (*models.AddressPool, error) {
	const query = `SELECT * FROM address_pools WHERE tenant_id = ? AND id = ?`
	var pool models.AddressPool
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &pool, query, tenantID, poolID); err != nil {
		return nil, err
	}
	return &pool, nil
}

func (r *MySQLRepository) InsertPool(ctx context.Context, pool *models.AddressPool) error {
	db, err := r.tenantDB(ctx, pool.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO address_pools (
	id, tenant_id, scope, parent_id, name, cidr, network, netmask,
	range_start, range_end, exclusions, vlan_id, interface_id,
    ssid, location, reserve_percent, lease_profile_id, tags,
    allocation_mode, priority_weight,
    created_at, updated_at)
VALUES (
	:id, :tenant_id, :scope, :parent_id, :name, :cidr, :network, :netmask,
	:range_start, :range_end, :exclusions, :vlan_id, :interface_id,
	:ssid, :location, :reserve_percent, :lease_profile_id, :tags,
	:allocation_mode, :priority_weight,
    :created_at, :updated_at)`, pool)
	return err
}

func (r *MySQLRepository) UpdatePool(ctx context.Context, pool *models.AddressPool) error {
	db, err := r.tenantDB(ctx, pool.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
UPDATE address_pools SET
	scope = :scope,
    parent_id = :parent_id,
    name = :name,
    cidr = :cidr,
	network = :network,
	netmask = :netmask,
	range_start = :range_start,
	range_end = :range_end,
	exclusions = :exclusions,
    vlan_id = :vlan_id,
    interface_id = :interface_id,
    ssid = :ssid,
    location = :location,
    reserve_percent = :reserve_percent,
    lease_profile_id = :lease_profile_id,
	tags = :tags,
	allocation_mode = :allocation_mode,
	priority_weight = :priority_weight,
    updated_at = :updated_at
WHERE id = :id AND tenant_id = :tenant_id`, pool)
	return err
}

func (r *MySQLRepository) FindPools(ctx context.Context, tenantID string, filter MetadataFilter) ([]models.AddressPool, error) {
	var clauses []string
	var args []any
	clauses = append(clauses, "tenant_id = ?")
	args = append(args, tenantID)
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
	var pools []models.AddressPool
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &pools, query.String(), args...); err != nil {
		return nil, err
	}
	return pools, nil
}

func (r *MySQLRepository) DeletePool(ctx context.Context, tenantID, poolID string) error {
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM address_pools WHERE tenant_id = ? AND id = ?`, tenantID, poolID)
	return err
}

func (r *MySQLRepository) ListBindings(ctx context.Context, tenantID string, limit, offset int) ([]models.StaticBinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM static_bindings WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
	var bindings []models.StaticBinding
	if err := db.SelectContext(ctx, &bindings, query, tenantID, limit, offset); err != nil {
		return nil, err
	}
	return bindings, nil
}

func (r *MySQLRepository) GetBinding(ctx context.Context, tenantID, bindingID string) (*models.StaticBinding, error) {
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const query = `SELECT * FROM static_bindings WHERE tenant_id = ? AND id = ?`
	var binding models.StaticBinding
	if err := db.GetContext(ctx, &binding, query, tenantID, bindingID); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (r *MySQLRepository) InsertBinding(ctx context.Context, binding *models.StaticBinding) error {
	db, err := r.tenantDB(ctx, binding.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO static_bindings (
    id, tenant_id, identifier, identifier_type, pool_id, ip_address,
    lease_profile_id, metadata, created_at, updated_at)
VALUES (
    :id, :tenant_id, :identifier, :identifier_type, :pool_id, :ip_address,
    :lease_profile_id, :metadata, :created_at, :updated_at)`, binding)
	return err
}

func (r *MySQLRepository) UpdateBinding(ctx context.Context, binding *models.StaticBinding) error {
	db, err := r.tenantDB(ctx, binding.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
UPDATE static_bindings SET
	identifier = :identifier,
	identifier_type = :identifier_type,
	pool_id = :pool_id,
	ip_address = :ip_address,
	lease_profile_id = :lease_profile_id,
	metadata = :metadata,
	updated_at = :updated_at
WHERE id = :id AND tenant_id = :tenant_id`, binding)
	return err
}

func (r *MySQLRepository) DeleteBinding(ctx context.Context, tenantID, bindingID string) error {
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM static_bindings WHERE tenant_id = ? AND id = ?`, tenantID, bindingID)
	return err
}
