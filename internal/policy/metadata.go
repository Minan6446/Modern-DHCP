package policy

import "strings"

// OptionMetadataBinding documents how specific DHCP options are projected into
// policy metadata keys. This acts as the single source of truth for both code
// comments and README tables so operators know which relay/controller headers
// they must preserve.
type OptionMetadataBinding struct {
	Protocol     string
	Option       string
	MetadataKeys []string
	Description  string
}

// OptionMetadataBindings enumerates the DHCP options we normalize today. When
// introducing new relay hints, extend this slice and keep README.md in sync.
var OptionMetadataBindings = []OptionMetadataBinding{
	{
		Protocol:     "DHCPv4 + DHCPv6",
		Option:       "Option 60 (v4) / Option 16 (v6) Vendor Class",
		MetadataKeys: []string{"vendorClass", "option60", "deviceType"},
		Description:  "Used directly by policy conditions and to infer coarse device categories via ClassifyDeviceType.",
	},
	{
		Protocol:     "DHCPv4 + DHCPv6",
		Option:       "Option 77 (v4) / Option 15 (v6) User Class",
		MetadataKeys: []string{"userClass", "userGroups"},
		Description:  "Raw user-class strings are exposed for policy matching while DeriveUserGroups tokenizes them into logical tags.",
	},
	{
		Protocol:     "DHCPv4 Relay Agent Info / DHCPv6 Interface-ID",
		Option:       "Option 82 sub-option 1 / Option 18 interface-id",
		MetadataKeys: []string{"circuit-id", "agent.circuit-id", "port-id", "interface-id"},
		Description:  "Identifies the physical access circuit/port, feeding Guard context as well as pool selectors that target interface-bound pools.",
	},
	{
		Protocol:     "DHCPv4 Relay Agent Info / DHCPv6 Remote-ID",
		Option:       "Option 82 sub-option 2 / Option 37 remote-id",
		MetadataKeys: []string{"remote-id", "agent.remote-id", "location", "site", "room", "zone", "building"},
		Description:  "Provides relay identity or controller-provided site tags which DeriveLocation normalizes for policy matching.",
	},
	{
		Protocol:     "DHCPv4 Relay Vendor Sub-Options / Controller headers",
		Option:       "Option 82 vendor SSID/BSSID annotations",
		MetadataKeys: []string{"ssid", "wifi-ssid", "wireless-ssid", "essid", "ap-ssid", "ap-id", "ap.name", "ap-mac", "bssid"},
		Description:  "Maps AP telemetry (SSID/BSSID) into Input.SSID and AccessPointID so Wi-Fi specific rules can be authored.",
	},
	{
		Protocol:     "DHCPv4 Relay Agent Info / Controller headers",
		Option:       "Option 82 controller annotations",
		MetadataKeys: []string{"controller-id", "controller", "wlc-id", "ap-controller"},
		Description:  "Identifies the WLAN controller handling the client for policy and mobility affinity decisions.",
	},
	{
		Protocol:     "DHCPv4 Relay Vendor Sub-Options / Controller headers",
		Option:       "Option 125 mobility anchor annotations",
		MetadataKeys: []string{"mobility-anchor-id", "mobility.anchor", "anchor-id", "anchor"},
		Description:  "Supplies the roaming anchor identifier used to preserve IP assignments across subnets.",
	},
	{
		Protocol:     "DHCPv4 Relay Agent Info / Controller headers",
		Option:       "Option 82 geo-zone annotations",
		MetadataKeys: []string{"geo-zone", "geozone", "campus-zone", "zone"},
		Description:  "Provides coarse geographic zoning data used for location-aware allocations.",
	},
	{
		Protocol:     "DHCPv4 Relay Agent Info / DHCPv6 Relay Hints",
		Option:       "Option 82 VLAN metadata / controller vlan-id",
		MetadataKeys: []string{"vlan-id", "agent.vlan-id"},
		Description:  "Normalizes Layer-2 segment identifiers so policies can constrain by VLAN or feed pool selectors with VLAN context.",
	},
}

// DeriveSSID extracts SSID metadata from relay information (Option 82 or controller headers).
func DeriveSSID(relay map[string]string) string {
	return firstRelayValue(relay, "ssid", "wifi-ssid", "wireless-ssid", "essid", "ap-ssid")
}

// DeriveLocation normalizes location/site metadata for policy matching.
func DeriveLocation(relay map[string]string) string {
	if loc := firstRelayValue(relay, "location", "site", "room", "zone", "building"); loc != "" {
		return loc
	}
	return firstRelayValue(relay, "remote-id", "agent.remote-id")
}

// DeriveGeoZone extracts geo/campus zone hints separate from verbose location strings.
func DeriveGeoZone(relay map[string]string) string {
	return firstRelayValue(relay, "geo-zone", "geozone", "campus-zone", "zone")
}

// DeriveUserID extracts subscriber/account identifiers from relay metadata.
func DeriveUserID(relay map[string]string) string {
	return firstRelayValue(relay, "user-id", "subscriber-id", "account-id", "pppoe-id")
}

// DeriveAccessPointID returns the AP/BSSID identifier when present.
func DeriveAccessPointID(relay map[string]string) string {
	return firstRelayValue(relay, "ap-id", "ap.name", "ap-mac", "radio-mac", "bssid")
}

// DeriveControllerID returns the controller or WLC identifier when present.
func DeriveControllerID(relay map[string]string) string {
	return firstRelayValue(relay, "controller-id", "controller", "wlc-id", "ap-controller")
}

// DeriveMobilityAnchorID extracts the roaming anchor identifier.
func DeriveMobilityAnchorID(relay map[string]string) string {
	return firstRelayValue(relay, "mobility-anchor-id", "mobility.anchor", "anchor-id", "anchor")
}

// ClassifyDeviceType infers a coarse device category from vendor/user-class hints or relay annotations.
func ClassifyDeviceType(vendorClass, userClass string, relay map[string]string) string {
	source := strings.ToLower(strings.Join(filterNonEmpty([]string{
		firstRelayValue(relay, "device-type", "dev-type"),
		vendorClass,
		userClass,
	}), " "))
	if source == "" {
		return ""
	}
	rules := []struct {
		label    string
		keywords []string
	}{
		{label: "ip_camera", keywords: []string{"camera", "cam"}},
		{label: "ip_phone", keywords: []string{"phone", "sip", "voip"}},
		{label: "printer", keywords: []string{"printer", "print"}},
		{label: "wireless_ap", keywords: []string{"access point", "wifi ap", "wireless ap"}},
		{label: "iot_sensor", keywords: []string{"sensor", "iot", "controller"}},
	}
	for _, rule := range rules {
		match := true
		for _, kw := range rule.keywords {
			if !strings.Contains(source, kw) {
				match = false
				break
			}
		}
		if match {
			return rule.label
		}
	}
	return ""
}

// DeriveUserGroups builds a set of logical user-group tags from DHCP user-class or relay metadata.
func DeriveUserGroups(userClass string, relay map[string]string) []string {
	var groups []string
	for _, token := range tokenizeList(userClass) {
		groups = appendUnique(groups, token)
	}
	for _, key := range []string{"user-group", "policy-group"} {
		if val := strings.TrimSpace(relay[key]); val != "" {
			for _, token := range tokenizeList(val) {
				groups = appendUnique(groups, token)
			}
		}
	}
	lowerBlob := strings.ToLower(userClass)
	if strings.Contains(lowerBlob, "guest") {
		groups = appendUnique(groups, "guest")
	}
	if strings.Contains(lowerBlob, "staff") || strings.Contains(lowerBlob, "corp") {
		groups = appendUnique(groups, "employee")
	}
	return groups
}

func firstRelayValue(relay map[string]string, keys ...string) string {
	if len(relay) == 0 {
		return ""
	}
	for _, key := range keys {
		if val, ok := relay[key]; ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func tokenizeList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	f := func(r rune) bool {
		switch r {
		case ',', ';', '|', '/', '\\', '\n', '\r', '\t':
			return true
		}
		return false
	}
	parts := strings.FieldsFunc(raw, f)
	var tokens []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			tokens = append(tokens, trimmed)
		}
	}
	return tokens
}

func appendUnique(items []string, candidate string) []string {
	if candidate == "" {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(existing, candidate) {
			return items
		}
	}
	return append(items, candidate)
}

func filterNonEmpty(values []string) []string {
	var out []string
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
