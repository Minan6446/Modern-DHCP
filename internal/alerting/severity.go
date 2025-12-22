package alerting

import "strings"

// Severity captures the alert criticality tiers.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityMajor    Severity = "MAJOR"
	SeverityWarning  Severity = "WARNING"
	SeverityInfo     Severity = "INFO"
)

// ParseSeverity maps user-provided strings to canonical severities.
func ParseSeverity(raw string) (Severity, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(SeverityCritical):
		return SeverityCritical, true
	case string(SeverityMajor):
		return SeverityMajor, true
	case string(SeverityWarning):
		return SeverityWarning, true
	case string(SeverityInfo):
		return SeverityInfo, true
	default:
		return "", false
	}
}
