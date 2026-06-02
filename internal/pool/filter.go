package pool

// MetadataFilter constrains pool queries by metadata fields.
type MetadataFilter struct {
	Scope          string
	ParentID       *string
	VLANID         *int
	InterfaceID    *string
	SSID           *string
	Location       *string
	GeoCode        *string
	DeviceProfile  *string
	TagFingerprint *string
	Limit          int
}

// MetadataSelector represents the attributes collected from relays/APs for pool lookup.
type MetadataSelector struct {
	InterfaceID    string
	SSID           string
	Location       string
	GeoCode        string
	DeviceProfile  string
	TagFingerprint string
	VLANID         int
	AccessPointID  string
	ControllerID   string
	GeoZone        string
}
