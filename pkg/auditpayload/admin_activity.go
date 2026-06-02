package auditpayload

// AdminActivity captures request metadata for privileged API calls.
type AdminActivity struct {
	Actor          string            `json:"actor"`
	Role           string            `json:"role"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Query          map[string]string `json:"query,omitempty"`
	StatusCode     int               `json:"statusCode"`
	RemoteAddr     string            `json:"remoteAddr,omitempty"`
	UserAgent      string            `json:"userAgent,omitempty"`
	DurationMillis int64             `json:"durationMillis"`
	CorrelationID  string            `json:"correlationId,omitempty"`
	Sensitive      bool              `json:"sensitive"`
	Scope          ScopeMetadata     `json:"scope,omitempty"`
}
