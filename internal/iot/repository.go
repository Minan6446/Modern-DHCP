package iot

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/pkg/models"
)

// ErrNotFound indicates that no record matched the requested lookup.
var ErrNotFound = errors.New("iot: not found")

// DeviceFilter narrows device searches in the registry.
type DeviceFilter struct {
	ProfileID  string
	Status     string
	SleepyOnly bool
	Search     string
	Limit      int
	Offset     int
}

// Repository exposes persistence operations for the IoT registry.
type Repository interface {
	UpsertDevice(ctx context.Context, device *models.IoTDevice) error
	GetDevice(ctx context.Context, tenantID, deviceID string) (*models.IoTDevice, error)
	GetDeviceByID(ctx context.Context, tenantID, id string) (*models.IoTDevice, error)
	ListDevices(ctx context.Context, tenantID string, filter DeviceFilter) ([]models.IoTDevice, error)
	DeleteDevice(ctx context.Context, tenantID, deviceID string) error
	RecordHeartbeat(ctx context.Context, tenantID, deviceID string, lastSeen time.Time, status string) error

	CreateProfile(ctx context.Context, profile *models.IoTDeviceProfile) error
	UpdateProfile(ctx context.Context, profile *models.IoTDeviceProfile) error
	GetProfile(ctx context.Context, tenantID, profileID string) (*models.IoTDeviceProfile, error)
	ListProfiles(ctx context.Context, tenantID string) ([]models.IoTDeviceProfile, error)
	DeleteProfile(ctx context.Context, tenantID, profileID string) error
}

// MySQLRepository persists IoT registry state through sqlx.
type MySQLRepository struct {
	db *sqlx.DB
}

// NewRepository constructs a MySQL-backed IoT registry repository.
func NewRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) tenantDB(ctx context.Context, tenantID string) (*sqlx.DB, error) {
	return r.db, nil
}

// UpsertDevice creates or updates a device registration keyed by tenant/device-id.
func (r *MySQLRepository) UpsertDevice(ctx context.Context, device *models.IoTDevice) error {
	db, err := r.tenantDB(ctx, device.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO iot_devices (
	id, device_id, display_name, hardware_addr,
    profile_id, lease_profile_id, sleep_class, sleep_interval, offline_window,
    sleepy_hint, status, firmware_version, labels, metadata, last_seen,
    created_at, updated_at)
VALUES (
	:id, :device_id, :display_name, :hardware_addr,
    :profile_id, :lease_profile_id, :sleep_class, :sleep_interval, :offline_window,
    :sleepy_hint, :status, :firmware_version, :labels, :metadata, :last_seen,
    :created_at, :updated_at)
ON DUPLICATE KEY UPDATE
    display_name = VALUES(display_name),
    hardware_addr = VALUES(hardware_addr),
    profile_id = VALUES(profile_id),
    lease_profile_id = VALUES(lease_profile_id),
    sleep_class = VALUES(sleep_class),
    sleep_interval = VALUES(sleep_interval),
    offline_window = VALUES(offline_window),
    sleepy_hint = VALUES(sleepy_hint),
    status = VALUES(status),
    firmware_version = VALUES(firmware_version),
    labels = VALUES(labels),
    metadata = VALUES(metadata),
    last_seen = VALUES(last_seen),
    updated_at = VALUES(updated_at)`, device)
	return err
}

// GetDevice fetches a device by natural key.
func (r *MySQLRepository) GetDevice(ctx context.Context, tenantID, deviceID string) (*models.IoTDevice, error) {
	const query = `SELECT * FROM iot_devices WHERE device_id = ?`
	var device models.IoTDevice
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &device, query, deviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &device, nil
}

// GetDeviceByID fetches a device by internal identifier.
func (r *MySQLRepository) GetDeviceByID(ctx context.Context, tenantID, id string) (*models.IoTDevice, error) {
	const query = `SELECT * FROM iot_devices WHERE id = ?`
	var device models.IoTDevice
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &device, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &device, nil
}

// ListDevices returns registered devices filtered by optional criteria.
func (r *MySQLRepository) ListDevices(ctx context.Context, tenantID string, filter DeviceFilter) ([]models.IoTDevice, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var (
		builder strings.Builder
		args    []any
	)
	builder.WriteString("SELECT * FROM iot_devices WHERE 1=1")
	if filter.Status != "" {
		builder.WriteString(" AND status = ?")
		args = append(args, filter.Status)
	}
	if filter.ProfileID != "" {
		builder.WriteString(" AND profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.SleepyOnly {
		builder.WriteString(" AND sleepy_hint = 1")
	}
	if filter.Search != "" {
		builder.WriteString(" AND (device_id LIKE ? OR display_name LIKE ?)")
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm, searchTerm)
	}
	builder.WriteString(" ORDER BY updated_at DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var devices []models.IoTDevice
	if err := db.SelectContext(ctx, &devices, builder.String(), args...); err != nil {
		return nil, err
	}
	return devices, nil
}

// DeleteDevice removes a registered device.
func (r *MySQLRepository) DeleteDevice(ctx context.Context, tenantID, deviceID string) error {
	const query = `DELETE FROM iot_devices WHERE device_id = ?`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, query, deviceID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrNotFound
	}
	return err
}

// RecordHeartbeat updates last-seen metadata for a device.
func (r *MySQLRepository) RecordHeartbeat(ctx context.Context, tenantID, deviceID string, lastSeen time.Time, status string) error {
	const query = `UPDATE iot_devices SET last_seen = ?, status = ?, updated_at = ? WHERE device_id = ?`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, query, lastSeen, status, time.Now().UTC(), deviceID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateProfile inserts a reusable IoT device profile.
func (r *MySQLRepository) CreateProfile(ctx context.Context, profile *models.IoTDeviceProfile) error {
	db, err := r.tenantDB(ctx, profile.TenantID)
	if err != nil {
		return err
	}
	_, err = db.NamedExecContext(ctx, `
INSERT INTO iot_device_profiles (
	id, name, description, sleep_class,
    sleep_interval, offline_window, lease_profile_id,
    sleepy_capable, metadata, created_at, updated_at)
VALUES (
	:id, :name, :description, :sleep_class,
    :sleep_interval, :offline_window, :lease_profile_id,
    :sleepy_capable, :metadata, :created_at, :updated_at)`, profile)
	return err
}

// UpdateProfile persists profile updates.
func (r *MySQLRepository) UpdateProfile(ctx context.Context, profile *models.IoTDeviceProfile) error {
	db, err := r.tenantDB(ctx, profile.TenantID)
	if err != nil {
		return err
	}
	res, err := db.NamedExecContext(ctx, `
UPDATE iot_device_profiles SET
    name = :name,
    description = :description,
    sleep_class = :sleep_class,
    sleep_interval = :sleep_interval,
    offline_window = :offline_window,
    lease_profile_id = :lease_profile_id,
    sleepy_capable = :sleepy_capable,
    metadata = :metadata,
    updated_at = :updated_at
WHERE id = :id`, profile)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetProfile fetches a profile by id.
func (r *MySQLRepository) GetProfile(ctx context.Context, tenantID, profileID string) (*models.IoTDeviceProfile, error) {
	const query = `SELECT * FROM iot_device_profiles WHERE id = ?`
	var profile models.IoTDeviceProfile
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if err := db.GetContext(ctx, &profile, query, profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &profile, nil
}

// ListProfiles returns all profiles for a tenant.
func (r *MySQLRepository) ListProfiles(ctx context.Context, tenantID string) ([]models.IoTDeviceProfile, error) {
	const query = `SELECT * FROM iot_device_profiles ORDER BY updated_at DESC`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var profiles []models.IoTDeviceProfile
	if err := db.SelectContext(ctx, &profiles, query); err != nil {
		return nil, err
	}
	return profiles, nil
}

// DeleteProfile removes a profile definition.
func (r *MySQLRepository) DeleteProfile(ctx context.Context, tenantID, profileID string) error {
	const query = `DELETE FROM iot_device_profiles WHERE id = ?`
	db, err := r.tenantDB(ctx, tenantID)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, query, profileID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}
