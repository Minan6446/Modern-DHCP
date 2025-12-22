package reporting

import (
	"strings"
	"time"
)

// Format enumerates supported export formats.
type Format string

const (
	// FormatCSV emits a comma-separated artifact.
	FormatCSV Format = "csv"
	// FormatPDF represents a PDF artifact (pending implementation).
	FormatPDF Format = "pdf"
)

// ParseFormat normalizes caller input, defaulting to CSV.
func ParseFormat(value string) Format {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(FormatPDF):
		return FormatPDF
	default:
		return FormatCSV
	}
}

// Artifact describes an exported report payload.
type Artifact struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Format      Format    `json:"format"`
	ContentType string    `json:"contentType"`
	Size        int       `json:"size"`
	GeneratedAt time.Time `json:"generatedAt"`
	Data        []byte    `json:"-"`
}
