package auditpayload

// SecurityEvent represents guard or detector findings propagated to audit trails.
type SecurityEvent struct {
	TenantID    string            `json:"tenantId"`
	SignalType  string            `json:"signalType"`
	Verdict     string            `json:"verdict"`
	MAC         string            `json:"mac,omitempty"`
	ClientID    string            `json:"clientId,omitempty"`
	PortID      string            `json:"portId,omitempty"`
	InterfaceID string            `json:"interfaceId,omitempty"`
	VLANID      int               `json:"vlanId,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Confidence  string            `json:"confidence,omitempty"`
	Action      string            `json:"action,omitempty"`
	Details     map[string]any    `json:"details,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}
