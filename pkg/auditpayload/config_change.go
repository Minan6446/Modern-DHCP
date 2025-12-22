package auditpayload

// ConfigChange tracks write operations across pools, policies, ACLs, etc.
type ConfigChange struct {
	Resource   string            `json:"resource"`
	Action     string            `json:"action"`
	Identifier string            `json:"identifier"`
	Before     map[string]any    `json:"before,omitempty"`
	After      map[string]any    `json:"after,omitempty"`
	Fields     []string          `json:"fields,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Diff       map[string][2]any `json:"diff,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
