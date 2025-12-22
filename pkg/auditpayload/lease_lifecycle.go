package auditpayload

import "time"

// LeaseLifecycle captures allocation and release activities.
type LeaseLifecycle struct {
	LeaseID       string    `json:"leaseId"`
	TenantID      string    `json:"tenantId"`
	PoolID        string    `json:"poolId"`
	IPAddress     string    `json:"ipAddress"`
	MACAddress    string    `json:"macAddress"`
	ClientID      string    `json:"clientId,omitempty"`
	Action        string    `json:"action"`
	Reused        bool      `json:"reused"`
	Actor         string    `json:"actor"`
	Source        string    `json:"source"`
	Reason        string    `json:"reason,omitempty"`
	RequestedAt   time.Time `json:"requestedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	SecurityState string    `json:"securityState,omitempty"`
	PreviousState string    `json:"previousState,omitempty"`
}
