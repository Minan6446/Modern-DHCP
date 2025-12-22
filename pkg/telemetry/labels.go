package telemetry

import (
	"strconv"
	"strings"
)

const (
	labelUnknown = "unknown"
	labelNone    = "none"
)

// ScopeLabel normalizes scope strings for Prometheus labels.
func ScopeLabel(scope string) string {
	scope = strings.TrimSpace(strings.ToUpper(scope))
	if scope == "" {
		return labelUnknown
	}
	return scope
}

// StringLabel normalizes arbitrary selector strings (interface, SSID, location).
func StringLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return labelNone
	}
	return value
}

// OptionalStringLabel normalizes optional pointer strings.
func OptionalStringLabel(value *string) string {
	if value == nil {
		return labelNone
	}
	return StringLabel(*value)
}

// VLANLabel normalizes VLAN integers.
func VLANLabel(vlan int) string {
	if vlan <= 0 {
		return labelNone
	}
	return strconv.Itoa(vlan)
}

// OptionalVLANLabel normalizes optional VLAN pointers.
func OptionalVLANLabel(vlan *int) string {
	if vlan == nil {
		return labelNone
	}
	return VLANLabel(*vlan)
}

// ResultLabel renders resolved/miss outcomes.
func ResultLabel(resolved bool) string {
	if resolved {
		return "resolved"
	}
	return "miss"
}
