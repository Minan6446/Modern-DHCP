package models

import (
	"encoding/json"
	"time"
)

const (
	AllocationModeSequential       = "SEQUENTIAL"
	AllocationModeRoundRobin       = "ROUND_ROBIN"
	AllocationModePriorityWeighted = "PRIORITY_WEIGHTED"
)

const (
	SecurityStateOK      = "OK"
	SecurityStateSuspect = "SUSPECT"
	SecurityStateBlocked = "BLOCKED"
)

// Tenant represents an isolated customer/organization.
type Tenant struct {
	ID           string    `db:"id" json:"id"`
	Name         string    `db:"name" json:"name"`
	BrandTheme   string    `db:"brand_theme" json:"brandTheme"`
	QuotaPools   int       `db:"quota_pools" json:"quotaPools"`
	QuotaLeases  int       `db:"quota_leases" json:"quotaLeases"`
	DBDriver     string    `db:"db_driver" json:"dbDriver,omitempty"`
	DBDSN        string    `db:"db_dsn" json:"dbDsn,omitempty"`
	DBSchema     string    `db:"db_schema" json:"dbSchema,omitempty"`
	LogoURL      string    `db:"logo_url" json:"logoUrl,omitempty"`
	PrimaryColor string    `db:"primary_color" json:"primaryColor,omitempty"`
	AccentColor  string    `db:"accent_color" json:"accentColor,omitempty"`
	LoginMessage string    `db:"login_message" json:"loginMessage,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

// TenantQuota captures resource limits enforced per tenant.
type TenantQuota struct {
	TenantID           string    `db:"tenant_id" json:"tenantId"`
	PoolLimit          int       `db:"pool_limit" json:"poolLimit"`
	LeaseLimit         int       `db:"lease_limit" json:"leaseLimit"`
	ClientLimit        int       `db:"client_limit" json:"clientLimit"`
	APIRequestLimit    int       `db:"api_request_limit" json:"apiRequestLimit"`
	AutomationJobLimit int       `db:"automation_job_limit" json:"automationJobLimit"`
	UpdatedAt          time.Time `db:"updated_at" json:"updatedAt"`
}

// AddressPool models the hierarchical address pool abstraction.
type AddressPool struct {
	ID             string      `db:"id" json:"id"`
	TenantID       string      `db:"tenant_id" json:"tenantId"`
	Scope          string      `db:"scope" json:"scope"`
	ParentID       *string     `db:"parent_id" json:"parentId"`
	Name           string      `db:"name" json:"name"`
	CIDR           string      `db:"cidr" json:"cidr"`
	Network        string      `db:"network" json:"network"`
	Netmask        string      `db:"netmask" json:"netmask"`
	RangeStart     string      `db:"range_start" json:"rangeStart"`
	RangeEnd       string      `db:"range_end" json:"rangeEnd"`
	Gateway        string      `db:"gateway" json:"gateway"`
	Option43       string      `db:"option_43" json:"option43"`
	DNS            StringList  `db:"dns" json:"dns"`
	VLANID         *int        `db:"vlan_id" json:"vlanId"`
	InterfaceID    *string     `db:"interface_id" json:"interfaceId"`
	SSID           *string     `db:"ssid" json:"ssid"`
	Location       *string     `db:"location" json:"location"`
	GeoCode        *string     `db:"geo_code" json:"geoCode"`
	DeviceProfile  *string     `db:"device_profile" json:"deviceProfile"`
	TagFingerprint *string     `db:"tag_fingerprint" json:"tagFingerprint"`
	ReservePercent int         `db:"reserve_percent" json:"reservePercent"`
	MinLeaseTime   int         `db:"min_lease_time" json:"leaseTime"`
	MaxLeaseTime   int         `db:"max_lease_time" json:"maxLeaseTime"`
	LeaseProfileID string      `db:"lease_profile_id" json:"leaseProfileId"`
	Tags           []byte      `db:"tags" json:"tags"`
	Exclusions     IPRangeList `db:"exclusions" json:"exclusions"`
	AllocationMode string      `db:"allocation_mode" json:"allocationMode"`
	PriorityWeight int         `db:"priority_weight" json:"priorityWeight"`
	Status         string      `db:"status" json:"status"`
	CreatedAt      time.Time   `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time   `db:"updated_at" json:"updatedAt"`
}

// Lease represents DHCP lease records.
type Lease struct {
	ID                   string          `db:"id" json:"id"`
	TenantID             string          `db:"tenant_id" json:"tenantId"`
	PoolID               string          `db:"pool_id" json:"poolId"`
	IPAddress            string          `db:"ip_address" json:"ipAddress"`
	HardwareAddr         string          `db:"hardware_addr" json:"hardwareAddr"`
	ClientID             string          `db:"client_id" json:"clientId"`
	UserID               string          `db:"user_id" json:"userId"`
	MobilityAnchorID     string          `db:"mobility_anchor_id" json:"mobilityAnchorId"`
	DeviceType           string          `db:"device_type" json:"deviceType"`
	LastAccessPointID    string          `db:"last_access_point_id" json:"lastAccessPointId"`
	LastControllerID     string          `db:"last_controller_id" json:"lastControllerId"`
	LastGeoZone          string          `db:"last_geo_zone" json:"lastGeoZone"`
	MobilityLocationHint string          `db:"mobility_location_hint" json:"mobilityLocationHint"`
	MDMManaged           bool            `db:"mdm_managed" json:"mdmManaged"`
	MDMSource            string          `db:"mdm_source" json:"mdmSource"`
	MDMTags              json.RawMessage `db:"mdm_tags" json:"mdmTags"`
	MDMObservedAt        *time.Time      `db:"mdm_observed_at" json:"mdmObservedAt"`
	RelayInfo            []byte          `db:"relay_info" json:"relayInfo"`
	SessionContinuity    json.RawMessage `db:"session_continuity" json:"sessionContinuity"`
	ExpiresAt            time.Time       `db:"expires_at" json:"expiresAt"`
	State                string          `db:"state" json:"state"`
	SecurityState        string          `db:"security_state" json:"securityState"`
	CooldownUntil        *time.Time      `db:"cooldown_until" json:"cooldownUntil"`
	ConflictHistory      []byte          `db:"conflict_history" json:"conflictHistory"`
	CreatedAt            time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt            time.Time       `db:"updated_at" json:"updatedAt"`
}

// PrefixLease persists delegated IPv6 prefixes (IA_PD).
type PrefixLease struct {
	ID        string    `db:"id" json:"id"`
	TenantID  string    `db:"tenant_id" json:"tenantId"`
	PoolID    string    `db:"pool_id" json:"poolId"`
	ClientID  string    `db:"client_id" json:"clientId"`
	IAPDID    uint32    `db:"iapd_id" json:"iapdId"`
	Prefix    string    `db:"prefix" json:"prefix"`
	PrefixLen int       `db:"prefix_length" json:"prefixLength"`
	State     string    `db:"state" json:"state"`
	ExpiresAt time.Time `db:"expires_at" json:"expiresAt"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

// LeaseProfile defines duration policy per pool or policy result.
type LeaseProfile struct {
	ID                  string        `db:"id" json:"id"`
	TenantID            string        `db:"tenant_id" json:"tenantId"`
	Name                string        `db:"name" json:"name"`
	DefaultDuration     time.Duration `db:"default_duration" json:"defaultDuration"`
	MinDuration         time.Duration `db:"min_duration" json:"minDuration"`
	MaxDuration         time.Duration `db:"max_duration" json:"maxDuration"`
	RenewalTime         time.Duration `db:"renewal_time" json:"renewalTime"`
	RebindingTime       time.Duration `db:"rebinding_time" json:"rebindingTime"`
	NotificationLead    time.Duration `db:"notification_lead" json:"notificationLead"`
	Infinite            bool          `db:"infinite" json:"infinite"`
	SleepyCapable       bool          `db:"sleepy_capable" json:"sleepyCapable"`
	SleepyOfflineWindow time.Duration `db:"sleepy_offline_window" json:"sleepyOfflineWindow"`
	SleepyHoldDuration  time.Duration `db:"sleepy_hold_duration" json:"sleepyHoldDuration"`
	MobilityGracePeriod time.Duration `db:"mobility_grace_period" json:"mobilityGracePeriod"`
	DeviceType          string        `db:"device_type" json:"deviceType"`
	CreatedAt           time.Time     `db:"created_at" json:"createdAt"`
	UpdatedAt           time.Time     `db:"updated_at" json:"updatedAt"`
}

// PolicyRule describes a conditional allocation rule.
type PolicyRule struct {
	ID         string    `db:"id" json:"id"`
	TenantID   string    `db:"tenant_id" json:"tenantId"`
	Priority   int       `db:"priority" json:"priority"`
	Conditions []byte    `db:"conditions" json:"conditions"`
	Actions    []byte    `db:"actions" json:"actions"`
	Enabled    bool      `db:"enabled" json:"enabled"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time `db:"updated_at" json:"updatedAt"`
}

// StaticBinding represents pre-assigned addresses.
type StaticBinding struct {
	ID              string     `db:"id" json:"id"`
	TenantID        string     `db:"tenant_id" json:"tenantId"`
	Identifier      string     `db:"identifier" json:"identifier"`
	IdentifierType  string     `db:"identifier_type" json:"identifierType"`
	PoolID          string     `db:"pool_id" json:"poolId"`
	IPAddress       string     `db:"ip_address" json:"ipAddress"`
	LeaseProfileID  string     `db:"lease_profile_id" json:"leaseProfileId"`
	Metadata        []byte     `db:"metadata" json:"metadata"`
	Status          string     `db:"status" json:"status"`
	StatusSource    *string    `db:"status_source" json:"statusSource"`
	LastSeenAt      *time.Time `db:"last_seen_at" json:"lastSeenAt"`
	StatusUpdatedAt *time.Time `db:"status_updated_at" json:"statusUpdatedAt"`
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`
}

// AuditEvent captures administrative actions for compliance tracing.
type AuditEvent struct {
	ID            int64     `db:"id" json:"id"`
	AuditID       string    `db:"audit_id" json:"auditId"`
	TenantID      string    `db:"tenant_id" json:"tenantId"`
	Actor         string    `db:"actor" json:"actor"`
	Action        string    `db:"action" json:"action"`
	Source        string    `db:"source" json:"source"`
	Resource      string    `db:"resource" json:"resource"`
	CorrelationID string    `db:"correlation_id" json:"correlationId"`
	Payload       []byte    `db:"payload" json:"payload"`
	CreatedAt     time.Time `db:"created_at" json:"createdAt"`
}
