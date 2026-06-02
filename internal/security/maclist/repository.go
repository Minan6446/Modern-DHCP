package maclist

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	dbutil "modern-dhcp/internal/db"

	"github.com/jmoiron/sqlx"
)

// Repository persists MAC list entries.
type Repository interface {
	List(ctx context.Context, tenantID string) ([]Entry, error)
	Get(ctx context.Context, tenantID, id string) (*Entry, error)
	FindByMAC(ctx context.Context, tenantID, mac string) (*Entry, error)
	Create(ctx context.Context, entry *Entry) error
	Update(ctx context.Context, entry *Entry) error
	Delete(ctx context.Context, tenantID, id string) error
}

var (
	// ErrNotFound indicates no record exists.
	ErrNotFound = errors.New("mac list: entry not found")
)

type sqlRepository struct {
	db *sqlx.DB
}

// NewRepository constructs a Repository backed by sqlx.
func NewRepository(db *sqlx.DB) Repository {
	return &sqlRepository{db: db}
}

func (r *sqlRepository) List(ctx context.Context, tenantID string) ([]Entry, error) {
	const query = `
SELECT id, mac, list_type, action, description, source, priority, enabled, valid_from, valid_until, metadata, created_at, updated_at
FROM security_mac_lists
WHERE 1=1
ORDER BY priority ASC, updated_at DESC`
	rows := make([]Entry, 0)
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *sqlRepository) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	const query = `
SELECT id, mac, list_type, action, description, source, priority, enabled, valid_from, valid_until, metadata, created_at, updated_at
FROM security_mac_lists
WHERE id = ?
LIMIT 1`
	var entry Entry
	if err := r.db.GetContext(ctx, &entry, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &entry, nil
}

func (r *sqlRepository) FindByMAC(ctx context.Context, tenantID, mac string) (*Entry, error) {
	const query = `
	SELECT id, mac, list_type, action, description, source, priority, enabled, valid_from, valid_until, metadata, created_at, updated_at
	FROM security_mac_lists
	WHERE mac = ? AND enabled = 1 AND (valid_from IS NULL OR valid_from <= ?) AND (valid_until IS NULL OR valid_until >= ?)
	ORDER BY
		CASE list_type
			WHEN 'blacklist' THEN 0
			WHEN 'whitelist' THEN 1
			WHEN 'graylist' THEN 2
			ELSE 3
		END,
		priority ASC,
		updated_at DESC
	LIMIT 1`
	var entry Entry
	now := time.Now().UTC()
	if err := r.db.GetContext(ctx, &entry, query, mac, now, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &entry, nil
}

func (r *sqlRepository) Create(ctx context.Context, entry *Entry) error {
	if entry == nil {
		return errors.New("mac list: entry required")
	}
	now := time.Now().UTC()
	entry.CreatedAt = now
	entry.UpdatedAt = now
	const query = `
INSERT INTO security_mac_lists (id, mac, list_type, action, description, source, priority, enabled, valid_from, valid_until, metadata, created_at, updated_at)
VALUES (:id, :mac, :list_type, :action, :description, :source, :priority, :enabled, :valid_from, :valid_until, :metadata, :created_at, :updated_at)`
	params := map[string]any{
		"id":          entry.ID,
		"mac":         normalizeMAC(entry.MAC),
		"list_type":   strings.ToLower(string(entry.Type)),
		"action":      strings.ToLower(string(entry.Action)),
		"description": entry.Description,
		"source":      entry.Source,
		"priority":    entry.Priority,
		"enabled":     boolToInt(entry.Enabled),
		"valid_from":  entry.ValidFrom,
		"valid_until": entry.ValidUntil,
		"metadata":    entry.Metadata,
		"created_at":  entry.CreatedAt,
		"updated_at":  entry.UpdatedAt,
	}
	_, err := r.db.NamedExecContext(ctx, query, params)
	return err
}

func (r *sqlRepository) Update(ctx context.Context, entry *Entry) error {
	if entry == nil {
		return errors.New("mac list: entry required")
	}
	now := time.Now().UTC()
	entry.UpdatedAt = now
	const query = `
UPDATE security_mac_lists
SET mac = :mac,
    list_type = :list_type,
    action = :action,
    description = :description,
    source = :source,
    priority = :priority,
    enabled = :enabled,
    valid_from = :valid_from,
    valid_until = :valid_until,
    metadata = :metadata,
    updated_at = :updated_at
WHERE id = :id`
	params := map[string]any{
		"id":          entry.ID,
		"mac":         normalizeMAC(entry.MAC),
		"list_type":   strings.ToLower(string(entry.Type)),
		"action":      strings.ToLower(string(entry.Action)),
		"description": entry.Description,
		"source":      entry.Source,
		"priority":    entry.Priority,
		"enabled":     boolToInt(entry.Enabled),
		"valid_from":  entry.ValidFrom,
		"valid_until": entry.ValidUntil,
		"metadata":    entry.Metadata,
		"updated_at":  entry.UpdatedAt,
	}
	res, err := r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqlRepository) Delete(ctx context.Context, tenantID, id string) error {
	const query = `DELETE FROM security_mac_lists WHERE id = ?`
	return dbutil.WithTx(ctx, r.db, func(tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx, query, id)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(mac)
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToLower(mac)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
