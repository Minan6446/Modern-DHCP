package relay

import (
	"net"
	"strconv"
	"strings"
)

// Metadata captures normalized relay identity used for pool routing.
type Metadata struct {
	TenantID   string
	RelayID    string
	GIAddr     string
	CircuitID  string
	RemoteID   string
	VLANID     int
	VRF        string
	VPNID      string
	MPLSVPN    string
	Region     string
	DataCenter string
	UserGroups []string
	Attributes map[string]string
}

// Attribute returns the first matching attribute key if present.
func (m Metadata) Attribute(keys ...string) string {
	if len(m.Attributes) == 0 {
		return ""
	}
	for _, key := range keys {
		if val, ok := m.Attributes[key]; ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// BuildMetadata constructs Metadata from raw relay attributes.
func BuildMetadata(tenantID string, giaddr net.IP, attrs map[string]string, fallbackVLAN int, userGroups []string) Metadata {
	meta := Metadata{
		TenantID: tenantID,
		GIAddr:   ipToString(giaddr),
	}
	if len(attrs) > 0 {
		meta.Attributes = make(map[string]string, len(attrs))
		for k, v := range attrs {
			meta.Attributes[k] = v
		}
	}
	meta.CircuitID = firstAttr(attrs, "circuit-id", "agent.circuit-id", "port-id", "interface-id")
	meta.RemoteID = firstAttr(attrs, "remote-id", "agent.remote-id")
	meta.VLANID = parseVLAN(firstAttr(attrs, "vlan-id", "agent.vlan-id"))
	if meta.VLANID <= 0 {
		meta.VLANID = fallbackVLAN
	}
	meta.VRF = firstAttr(attrs, "vrf", "route-domain", "vss-id")
	meta.VPNID = firstAttr(attrs, "vpn-id", "mpls.vpn-id")
	if meta.VPNID == "" {
		meta.VPNID = firstAttr(attrs, "subscriber-id")
	}
	meta.MPLSVPN = meta.VPNID
	meta.Region = firstAttr(attrs, "region", "site", "location")
	meta.DataCenter = firstAttr(attrs, "datacenter", "data-center", "dc")
	if meta.RemoteID != "" {
		meta.RelayID = meta.RemoteID
	} else if meta.CircuitID != "" {
		meta.RelayID = meta.CircuitID
	} else {
		meta.RelayID = meta.GIAddr
	}
	if len(userGroups) > 0 {
		meta.UserGroups = append([]string{}, userGroups...)
	}
	return meta
}

func firstAttr(attrs map[string]string, keys ...string) string {
	if len(attrs) == 0 {
		return ""
	}
	for _, key := range keys {
		if val, ok := attrs[key]; ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func parseVLAN(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	if v < 0 || v > 4094 {
		return 0
	}
	return v
}

func ipToString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	v4 := net.IP(ip).To4()
	if v4 != nil {
		return v4.String()
	}
	return net.IP(ip).String()
}
