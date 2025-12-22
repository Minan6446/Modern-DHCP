package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/config"
	dbutil "modern-dhcp/internal/db"
)

// TenantHandle describes a tenant-scoped database handle plus optional schema hint.
type TenantHandle struct {
	DB     *sqlx.DB
	Schema string
}

// TenantRouter hands out tenant-specific handles according to the configured tenancy mode.
type TenantRouter struct {
	defaultHandle TenantHandle
	mode          string
	overrides     map[string]config.TenantDatabaseConfig
	poolCfg       config.TenantRouterConfig
	mu            sync.RWMutex
	handles       map[string]TenantHandle
}

// NewTenantRouter builds a router that can serve shared or dedicated tenant handles.
func NewTenantRouter(defaultDB *sqlx.DB, cfg config.TenancyConfig) *TenantRouter {
	mode := cfg.Mode
	if mode == "" {
		mode = "shared"
	}
	return &TenantRouter{
		defaultHandle: TenantHandle{DB: defaultDB, Schema: cfg.DefaultSchema},
		mode:          mode,
		overrides:     cfg.Dedicated,
		poolCfg:       cfg.Router,
		handles:       make(map[string]TenantHandle),
	}
}

// Handle returns the database handle and schema for the provided tenant ID.
func (r *TenantRouter) Handle(ctx context.Context, tenantID string) (TenantHandle, error) {
	if tenantID == "" || r.mode == "shared" || r.overrides == nil {
		return r.defaultHandle, nil
	}
	override, ok := r.overrides[tenantID]
	if !ok {
		return r.defaultHandle, nil
	}
	r.mu.RLock()
	handle, ok := r.handles[tenantID]
	r.mu.RUnlock()
	if ok {
		return handle, nil
	}
	db, err := r.openTenantDB(ctx, override)
	if err != nil {
		return TenantHandle{}, err
	}
	handle = TenantHandle{DB: db, Schema: override.Schema}
	r.mu.Lock()
	r.handles[tenantID] = handle
	r.mu.Unlock()
	return handle, nil
}

// Close releases all cached tenant handles except the default one.
func (r *TenantRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for tenantID, handle := range r.handles {
		if handle.DB != nil {
			_ = handle.DB.Close()
		}
		delete(r.handles, tenantID)
	}
	return nil
}

func (r *TenantRouter) openTenantDB(ctx context.Context, cfg config.TenantDatabaseConfig) (*sqlx.DB, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = DriverMySQL
	}
	dsn := cfg.DSN
	if dsn == "" {
		return nil, fmt.Errorf("tenant router: missing DSN for driver %s", driver)
	}
	db, err := sqlx.Open(driverAlias(driver), dsn)
	if err != nil {
		return nil, err
	}
	if r.poolCfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(r.poolCfg.MaxOpenConns)
	}
	if r.poolCfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(r.poolCfg.MaxIdleConns)
	}
	if r.poolCfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(r.poolCfg.ConnMaxLife)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func driverAlias(driver string) string {
	switch driver {
	case DriverPostgres, "postgresql", "pg":
		return "postgres"
	default:
		return "mysql"
	}
}

// SchemaAware wraps sqlx to ensure the schema search path is applied before queries (best-effort).
func SchemaAware(db *sqlx.DB, schema string) *sqlx.DB {
	if db == nil || schema == "" {
		return db
	}
	// Attempt to set default schema; errors ignored to avoid impacting request flow.
	if _, err := db.Exec(fmt.Sprintf("USE %s", schema)); err != nil {
		return db
	}
	return db
}

// TenantAwareTx executes fn inside a transaction using the tenant handle.
func (r *TenantRouter) TenantAwareTx(ctx context.Context, tenantID string, iso sql.IsolationLevel, fn func(*sqlx.Tx) error) error {
	handle, err := r.Handle(ctx, tenantID)
	if err != nil {
		return err
	}
	if handle.DB == nil {
		return fmt.Errorf("tenant %s: database handle is nil", tenantID)
	}
	return dbutil.WithTx(ctx, handle.DB, fn, dbutil.TxWithIsolation(iso))
}
