package models

import "time"

const (
	IoTDeviceStatusActive  = "ACTIVE"
	IoTDeviceStatusDormant = "DORMANT"
	IoTDeviceStatusRetired = "RETIRED"

	IoTSleepClassNormal = "NORMAL"
	IoTSleepClassSleepy = "SLEEPY"
)

// IoTDeviceProfile defines reusable IoT-specific behaviors that map onto lease policies.
type IoTDeviceProfile struct {
	ID             string        `db:"id" json:"id"`
	TenantID       string        `db:"tenant_id" json:"tenantId"`
	Name           string        `db:"name" json:"name"`
	Description    string        `db:"description" json:"description"`
	SleepClass     string        `db:"sleep_class" json:"sleepClass"`
	SleepInterval  time.Duration `db:"sleep_interval" json:"sleepInterval"`
	OfflineWindow  time.Duration `db:"offline_window" json:"offlineWindow"`
	LeaseProfileID string        `db:"lease_profile_id" json:"leaseProfileId"`
	SleepyCapable  bool          `db:"sleepy_capable" json:"sleepyCapable"`
	Metadata       []byte        `db:"metadata" json:"metadata"`
	CreatedAt      time.Time     `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time     `db:"updated_at" json:"updatedAt"`
}

// IoTDevice tracks registered IoT identities for lease orchestration.
type IoTDevice struct {
	ID             string        `db:"id" json:"id"`
	TenantID       string        `db:"tenant_id" json:"tenantId"`
	DeviceID       string        `db:"device_id" json:"deviceId"`
	DisplayName    string        `db:"display_name" json:"displayName"`
	HardwareAddr   string        `db:"hardware_addr" json:"hardwareAddr"`
	ProfileID      string        `db:"profile_id" json:"profileId"`
	LeaseProfileID string        `db:"lease_profile_id" json:"leaseProfileId"`
	SleepClass     string        `db:"sleep_class" json:"sleepClass"`
	SleepInterval  time.Duration `db:"sleep_interval" json:"sleepInterval"`
	OfflineWindow  time.Duration `db:"offline_window" json:"offlineWindow"`
	SleepyHint     bool          `db:"sleepy_hint" json:"sleepyHint"`
	Status         string        `db:"status" json:"status"`
	Firmware       string        `db:"firmware_version" json:"firmwareVersion"`
	Labels         []byte        `db:"labels" json:"labels"`
	Metadata       []byte        `db:"metadata" json:"metadata"`
	LastSeen       *time.Time    `db:"last_seen" json:"lastSeen"`
	CreatedAt      time.Time     `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time     `db:"updated_at" json:"updatedAt"`
}
