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
	Theme                      string     `db:"theme" json:"theme"`
	Locale                     string     `db:"locale" json:"locale"`
	MaintenanceMode            bool       `db:"maintenance_mode" json:"maintenanceMode"`
	MaintenanceWindow          string     `db:"maintenance_window" json:"maintenanceWindow"`
	Announcement               string     `db:"announcement" json:"announcement"`
	AdminSessionTimeoutMinutes int        `db:"admin_session_timeout_minutes" json:"adminSessionTimeoutMinutes"`
	AutoLogoutEnabled          bool       `db:"auto_logout_enabled" json:"autoLogoutEnabled"`
	SystemLogRetentionDays     int        `db:"system_log_retention_days" json:"systemLogRetentionDays"`
	AuditLogRetentionDays      int        `db:"audit_log_retention_days" json:"auditLogRetentionDays"`
	LogPushEnabled             bool       `db:"log_push_enabled" json:"logPushEnabled"`
	LogPushEndpoint            string     `db:"log_push_endpoint" json:"logPushEndpoint"`
	LogPushMinLevel            string     `db:"log_push_min_level" json:"logPushMinLevel"`
	LogPushChannels            string     `db:"log_push_channels" json:"logPushChannels"`
	LogPushPhones              string     `db:"log_push_phones" json:"logPushPhones"`
	LogPushDingTalkEndpoint    string     `db:"log_push_dingtalk_endpoint" json:"logPushDingTalkEndpoint"`
	LogPushFeishuEndpoint      string     `db:"log_push_feishu_endpoint" json:"logPushFeishuEndpoint"`
	LogPushWecomEndpoint       string     `db:"log_push_wecom_endpoint" json:"logPushWecomEndpoint"`
	LogPushSlackEndpoint       string     `db:"log_push_slack_endpoint" json:"logPushSlackEndpoint"`
	NTPEnabled                 bool       `db:"ntp_enabled" json:"ntpEnabled"`
	NTPServers                 string     `db:"ntp_servers" json:"ntpServers"`
	NTPIntervalMinutes         int        `db:"ntp_interval_minutes" json:"ntpIntervalMinutes"`
	NTPTimeoutSeconds          int        `db:"ntp_timeout_seconds" json:"ntpTimeoutSeconds"`
	Timezone                   string     `db:"timezone" json:"timezone"`
	NTPSyncStatus              string     `db:"ntp_sync_status" json:"ntpSyncStatus"`
	NTPLastSyncAt              *time.Time `db:"ntp_last_sync_at" json:"ntpLastSyncAt,omitempty"`
	UpdatedAt                  time.Time  `db:"updated_at" json:"updatedAt"`
	UpdatedBy                  string     `db:"updated_by" json:"updatedBy"`
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
	err := s.db.GetContext(ctx, &settings, `SELECT theme, locale, maintenance_mode, COALESCE(maintenance_window, '') AS maintenance_window, COALESCE(announcement, '') AS announcement, admin_session_timeout_minutes, auto_logout_enabled, system_log_retention_days, audit_log_retention_days, log_push_enabled, COALESCE(log_push_endpoint, '') AS log_push_endpoint, COALESCE(log_push_min_level, 'warning') AS log_push_min_level, log_push_channels, log_push_phones, log_push_dingtalk_endpoint, log_push_feishu_endpoint, log_push_wecom_endpoint, log_push_slack_endpoint, ntp_enabled, COALESCE(ntp_servers, '') AS ntp_servers, ntp_interval_minutes, ntp_timeout_seconds, COALESCE(timezone, '') AS timezone, COALESCE(ntp_sync_status, '') AS ntp_sync_status, ntp_last_sync_at, updated_at, updated_by FROM ops_system_settings WHERE id = 1`)
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
	result, err := s.db.ExecContext(ctx, `UPDATE ops_system_settings SET theme = ?, locale = ?, maintenance_mode = ?, maintenance_window = ?, announcement = ?, admin_session_timeout_minutes = ?, auto_logout_enabled = ?, system_log_retention_days = ?, audit_log_retention_days = ?, log_push_enabled = ?, log_push_endpoint = ?, log_push_min_level = ?, log_push_channels = ?, log_push_phones = ?, log_push_dingtalk_endpoint = ?, log_push_feishu_endpoint = ?, log_push_wecom_endpoint = ?, log_push_slack_endpoint = ?, ntp_enabled = ?, ntp_servers = ?, ntp_interval_minutes = ?, ntp_timeout_seconds = ?, timezone = ?, ntp_sync_status = ?, ntp_last_sync_at = ?, updated_at = ?, updated_by = ? WHERE id = 1`, settings.Theme, settings.Locale, settings.MaintenanceMode, settings.MaintenanceWindow, settings.Announcement, settings.AdminSessionTimeoutMinutes, settings.AutoLogoutEnabled, settings.SystemLogRetentionDays, settings.AuditLogRetentionDays, settings.LogPushEnabled, settings.LogPushEndpoint, settings.LogPushMinLevel, settings.LogPushChannels, settings.LogPushPhones, settings.LogPushDingTalkEndpoint, settings.LogPushFeishuEndpoint, settings.LogPushWecomEndpoint, settings.LogPushSlackEndpoint, settings.NTPEnabled, settings.NTPServers, settings.NTPIntervalMinutes, settings.NTPTimeoutSeconds, settings.Timezone, settings.NTPSyncStatus, settings.NTPLastSyncAt, settings.UpdatedAt, settings.UpdatedBy)
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO ops_system_settings (id, theme, locale, maintenance_mode, maintenance_window, announcement, admin_session_timeout_minutes, auto_logout_enabled, system_log_retention_days, audit_log_retention_days, log_push_enabled, log_push_endpoint, log_push_min_level, log_push_channels, log_push_phones, log_push_dingtalk_endpoint, log_push_feishu_endpoint, log_push_wecom_endpoint, log_push_slack_endpoint, ntp_enabled, ntp_servers, ntp_interval_minutes, ntp_timeout_seconds, timezone, ntp_sync_status, ntp_last_sync_at, updated_at, updated_by) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, settings.Theme, settings.Locale, settings.MaintenanceMode, settings.MaintenanceWindow, settings.Announcement, settings.AdminSessionTimeoutMinutes, settings.AutoLogoutEnabled, settings.SystemLogRetentionDays, settings.AuditLogRetentionDays, settings.LogPushEnabled, settings.LogPushEndpoint, settings.LogPushMinLevel, settings.LogPushChannels, settings.LogPushPhones, settings.LogPushDingTalkEndpoint, settings.LogPushFeishuEndpoint, settings.LogPushWecomEndpoint, settings.LogPushSlackEndpoint, settings.NTPEnabled, settings.NTPServers, settings.NTPIntervalMinutes, settings.NTPTimeoutSeconds, settings.Timezone, settings.NTPSyncStatus, settings.NTPLastSyncAt, settings.UpdatedAt, settings.UpdatedBy)
	return err
}
