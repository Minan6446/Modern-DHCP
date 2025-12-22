package auditpayload

// LeaseConflict captures key fields for DHCP lease conflict events.
type LeaseConflict struct {
	LeaseID       string `json:"leaseId"`
	PoolID        string `json:"poolId"`
	TenantID      string `json:"tenantId"`
	IPAddress     string `json:"ipAddress"`
	Signal        string `json:"signal"`
	Reason        string `json:"reason,omitempty"`
	Identifier    string `json:"identifier,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	ConflictCount int    `json:"conflictCount"`
}
