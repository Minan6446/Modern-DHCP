package registry

import (
	"encoding/json"
	"time"
)

// DeviceUpsertRequest captures the payload for registering or updating an IoT device.
type DeviceUpsertRequest struct {
	TenantID       string
	DeviceID       string
	DisplayName    string
	HardwareAddr   string
	ProfileID      string
	LeaseProfileID string
	SleepClass     string
	SleepInterval  time.Duration
	OfflineWindow  time.Duration
	SleepyHint     bool
	Status         string
	Firmware       string
	Labels         map[string]string
	Metadata       json.RawMessage
	LastSeen       *time.Time
}

// DeviceListOptions narrows registry queries.
type DeviceListOptions struct {
	ProfileID  string
	Status     string
	SleepyOnly bool
	Search     string
	Limit      int
	Offset     int
}

// ProfileUpsertRequest manages IoT device profile CRUD.
type ProfileUpsertRequest struct {
	TenantID       string
	ProfileID      string
	Name           string
	Description    string
	SleepClass     string
	SleepInterval  time.Duration
	OfflineWindow  time.Duration
	LeaseProfileID string
	SleepyCapable  bool
	Metadata       json.RawMessage
}
