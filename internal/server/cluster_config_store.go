package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ClusterConfigStore persists shared cluster configuration (HA/LB, sync, backup, etc.).
type ClusterConfigStore interface {
	Save(ctx context.Context, key string, value any) error
	Load(ctx context.Context, key string, dest any) (bool, error)
}

type noopClusterConfigStore struct{}

func (noopClusterConfigStore) Save(ctx context.Context, key string, value any) error { return nil }
func (noopClusterConfigStore) Load(ctx context.Context, key string, dest any) (bool, error) {
	return false, nil
}

// NewSQLClusterConfigStore creates a MySQL-backed store (uses table cluster_config).
func NewSQLClusterConfigStore(db *sqlx.DB, logger *zap.Logger) ClusterConfigStore {
	store := &sqlClusterConfigStore{db: db, table: "cluster_config", logger: logger}
	if db != nil {
		_ = store.ensureTable(context.Background())
	}
	return store
}

type sqlClusterConfigStore struct {
	db     *sqlx.DB
	table  string
	logger *zap.Logger
}

func (s *sqlClusterConfigStore) ensureTable(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("cluster config store: db is nil")
	}
	schema := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	ckey VARCHAR(64) NOT NULL PRIMARY KEY,
	cvalue JSON NOT NULL,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
)`, s.table)
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *sqlClusterConfigStore) Save(ctx context.Context, key string, value any) error {
	if s.db == nil {
		return nil
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cluster config %s: %w", key, err)
	}
	query := fmt.Sprintf("INSERT INTO %s (ckey, cvalue) VALUES (?, ?) ON DUPLICATE KEY UPDATE cvalue = VALUES(cvalue)", s.table)
	_, err = s.db.ExecContext(ctx, query, key, string(bytes))
	return err
}

func (s *sqlClusterConfigStore) Load(ctx context.Context, key string, dest any) (bool, error) {
	if s.db == nil {
		return false, nil
	}
	query := fmt.Sprintf("SELECT cvalue FROM %s WHERE ckey = ?", s.table)
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, query, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !raw.Valid {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw.String), dest); err != nil {
		return false, fmt.Errorf("unmarshal cluster config %s: %w", key, err)
	}
	return true, nil
}
