package db

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/config"
)

// NewPostgres returns a configured sqlx DB handle backed by pgx.
func NewPostgres(ctx context.Context, cfg config.PostgresConfig) (*sqlx.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres dsn is empty")
	}
	db, err := sqlx.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	applyPoolSettings(db, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLife, cfg.ConnMaxIdle)

	if err := pingWithRetry(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}
