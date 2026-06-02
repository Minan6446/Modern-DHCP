package db

import (
	"context"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/config"
)

// NewMySQL returns a configured sqlx DB handle.
func NewMySQL(ctx context.Context, cfg config.MySQLConfig) (*sqlx.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql dsn is empty")
	}
	db, err := sqlx.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	applyPoolSettings(db, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLife, cfg.ConnMaxIdle)

	if err := pingWithRetry(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

// NewMySQLReplica returns a configured sqlx DB handle for a read-replica if defined.
func NewMySQLReplica(ctx context.Context, cfg config.MySQLConfig) (*sqlx.DB, error) {
	if cfg.ReplicaDSN == "" {
		return nil, nil
	}
	replicaCfg := cfg
	replicaCfg.DSN = cfg.ReplicaDSN
	if ctx == nil {
		ctx = context.Background()
	}
	db, err := sqlx.Open("mysql", replicaCfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql replica: %w", err)
	}
	applyPoolSettings(db, replicaCfg.ReplicaMaxOpenConns, replicaCfg.ReplicaMaxIdleConns, replicaCfg.ReplicaConnMaxLife, replicaCfg.ReplicaConnMaxIdle)
	if err := pingWithRetry(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql replica: %w", err)
	}
	return db, nil
}
