package alerting

import (
	"sort"
	"strings"
)

// Fingerprint returns a deterministic key for deduplicating alert events.
func Fingerprint(event Event) string {
	builder := strings.Builder{}
	builder.WriteString(strings.ToLower(strings.TrimSpace(event.TenantID)))
	builder.WriteString("|")
	builder.WriteString(strings.ToUpper(string(event.Severity)))
	builder.WriteString("|")
	builder.WriteString(strings.ToUpper(strings.TrimSpace(event.Category)))
	builder.WriteString("|")
	if len(event.Resources) > 0 {
		sorted := append([]string(nil), event.Resources...)
		sort.Strings(sorted)
		builder.WriteString(strings.Join(sorted, ","))
	}
	return builder.String()
}
