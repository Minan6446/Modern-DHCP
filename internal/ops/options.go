package ops

import "time"

// Options bundles Ops & Support knobs for service wiring.
type Options struct {
	System       OpsSystemOptions
	ImportExport ImportExportOptions
	HelpCenter   HelpCenterOptions
	Support      ExternalSupportOptions
	Scripts      ScriptRunnerOptions
}

// OpsSystemOptions mirrors system management toggles.
type OpsSystemOptions struct {
	Enabled           bool
	AllowConfigExport bool
	AllowConfigImport bool
	BackupLocation    string
	MaintenanceWindow string
	Theme             string
	Locale            string
	MaintenanceMode   bool
	Announcement      string
}

// ImportExportOptions governs bulk import/export pipelines.
type ImportExportOptions struct {
	Enabled     bool
	StoragePath string
	ObjectStore string
	Formats     []string
	MaxFileSize int64
}

// HelpCenterOptions exposes knowledge base metadata.
type HelpCenterOptions struct {
	Enabled           bool
	BaseURL           string
	ArticlesPath      string
	FAQPath           string
	ReleaseNotesPath  string
	FeaturedTopics    []string
	ExternalSearchURL string
	CacheTTL          time.Duration
}

// ExternalSupportOptions references ticketing and escalation channels.
type ExternalSupportOptions struct {
	Enabled          bool
	TicketURL        string
	ChatURL          string
	Phone            string
	EscalationPolicy string
	Contacts         []string
}

// ScriptRunnerOptions enumerates approved automation scripts.
type ScriptRunnerOptions struct {
	Enabled        bool
	DefaultTimeout time.Duration
	MaxConcurrent  int
	SandboxImage   string
	Catalog        []ScriptDescriptor
}

// ScriptDescriptor describes a script entry.
type ScriptDescriptor struct {
	Name             string
	Description      string
	Command          string
	Args             []string
	AllowedRoles     []string
	RequiresApproval bool
}
