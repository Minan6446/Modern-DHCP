package monitoring

import "strings"

// normalizeTenant trims tenant identifiers and defaults blanks to "default".
func normalizeTenant(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "default"
	}
	return raw
}
