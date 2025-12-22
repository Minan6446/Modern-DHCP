package auditpayload

// PoolSelector captures the metadata used to resolve a pool via selectors.
type PoolSelector struct {
	InterfaceID   string `json:"interfaceId,omitempty"`
	SSID          string `json:"ssid,omitempty"`
	Location      string `json:"location,omitempty"`
	VLANID        int    `json:"vlanId,omitempty"`
	AccessPointID string `json:"accessPointId,omitempty"`
	ControllerID  string `json:"controllerId,omitempty"`
	GeoZone       string `json:"geoZone,omitempty"`
}

// PoolSnapshot records identifying attributes about the resolved pool.
type PoolSnapshot struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Scope          string  `json:"scope"`
	ParentID       *string `json:"parentId,omitempty"`
	CIDR           string  `json:"cidr"`
	LeaseProfileID string  `json:"leaseProfileId,omitempty"`
}

// PoolMetadata surfaces the metadata tags that guided the selection.
type PoolMetadata struct {
	VLANID        *int    `json:"vlanId,omitempty"`
	InterfaceID   *string `json:"interfaceId,omitempty"`
	SSID          *string `json:"ssid,omitempty"`
	Location      *string `json:"location,omitempty"`
	AccessPointID *string `json:"accessPointId,omitempty"`
	ControllerID  *string `json:"controllerId,omitempty"`
	GeoZone       *string `json:"geoZone,omitempty"`
	Tags          []byte  `json:"tags,omitempty"`
}

// PoolResolution is emitted through audit/log channels when metadata resolves a pool.
type PoolResolution struct {
	PoolSelector PoolSelector `json:"poolSelector"`
	ResolvedPool PoolSnapshot `json:"resolvedPool"`
	Metadata     PoolMetadata `json:"metadata"`
}
