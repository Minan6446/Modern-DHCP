package ops

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrSettingsStoreUnavailable = errors.New("ops: settings store unavailable")

// SettingsStore persists mutable platform-level settings surfaced under /ops/system.
type SettingsStore interface {
	Load(ctx context.Context) (SystemSettings, error)
	Save(ctx context.Context, settings SystemSettings) (SystemSettings, error)
}

// SystemSettings capture mutable platform metadata rendered by the ops APIs.
type SystemSettings struct {
	Theme             string    `db:"theme" json:"theme"`
	Locale            string    `db:"locale" json:"locale"`
	MaintenanceMode   bool      `db:"maintenance_mode" json:"maintenanceMode"`
	MaintenanceWindow string    `db:"maintenance_window" json:"maintenanceWindow"`
	Announcement      string    `db:"announcement" json:"announcement"`
	UpdatedAt         time.Time `db:"updated_at" json:"updatedAt"`
	UpdatedBy         string    `db:"updated_by" json:"updatedBy"`
}

// sqlSettingsStore stores system settings inside the shared relational database.
type sqlSettingsStore struct {
	db       *sqlx.DB
	defaults SystemSettings
}

// NewSQLSettingsStore wraps a sqlx handle to persist platform settings.
func NewSQLSettingsStore(db *sqlx.DB, defaults SystemSettings) SettingsStore {
	return &sqlSettingsStore{db: db, defaults: defaults}
}

func (s *sqlSettingsStore) Load(ctx context.Context) (SystemSettings, error) {
	if s == nil || s.db == nil {
		return SystemSettings{}, ErrSettingsStoreUnavailable
	}
	var settings SystemSettings
	err := s.db.GetContext(ctx, &settings, `SELECT theme, locale, maintenance_mode, maintenance_window, announcement, updated_at, updated_by FROM ops_system_settings WHERE id = 1`)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SystemSettings{}, err
	}
	defaults := s.defaults
	if defaults.UpdatedBy == "" {
		defaults.UpdatedBy = "system"
	}
	defaults.UpdatedAt = time.Now().UTC()
	if err := s.seed(ctx, defaults); err != nil {
		return SystemSettings{}, err
	}
	return defaults, nil
}

func (s *sqlSettingsStore) Save(ctx context.Context, settings SystemSettings) (SystemSettings, error) {
	if s == nil || s.db == nil {
		return SystemSettings{}, ErrSettingsStoreUnavailable
	}
	if settings.UpdatedBy == "" {
		settings.UpdatedBy = "system"
	}
	settings.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE ops_system_settings SET theme = ?, locale = ?, maintenance_mode = ?, maintenance_window = ?, announcement = ?, updated_at = ?, updated_by = ? WHERE id = 1`, settings.Theme, settings.Locale, settings.MaintenanceMode, settings.MaintenanceWindow, settings.Announcement, settings.UpdatedAt, settings.UpdatedBy)
	if err != nil {
		return SystemSettings{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return SystemSettings{}, err
	}
	if rows == 0 {
		if err := s.seed(ctx, settings); err != nil {
			return SystemSettings{}, err
		}
	}
	return settings, nil
}

func (s *sqlSettingsStore) seed(ctx context.Context, settings SystemSettings) error {
	if s == nil || s.db == nil {
		return ErrSettingsStoreUnavailable
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO ops_system_settings (id, theme, locale, maintenance_mode, maintenance_window, announcement, updated_at, updated_by) VALUES (1, ?, ?, ?, ?, ?, ?, ?)`, settings.Theme, settings.Locale, settings.MaintenanceMode, settings.MaintenanceWindow, settings.Announcement, settings.UpdatedAt, settings.UpdatedBy)
	return err
}
