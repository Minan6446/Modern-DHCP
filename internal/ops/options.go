package ops

import "time"

// Options bundles Ops & Support knobs for service wiring.
type Options struct {
	System       OpsSystemOptions
	ImportExport ImportExportOptions
	HelpCenter   HelpCenterOptions
	Support      ExternalSupportOptions
	Scripts      ScriptRunnerOptions
	Maintenance  MaintenanceOptions
	Backups      BackupOptions
	Performance  PerformanceOptions
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

// MaintenanceOptions describe maintenance playbooks and upgrade plans.
type MaintenanceOptions struct {
	Enabled      bool
	Playbooks    []MaintenancePlaybookOption
	Windows      []MaintenanceWindowOption
	UpgradePlans []MaintenanceUpgradePlanOption
}

// MaintenancePlaybookOption encapsulates maintenance wizard data.
type MaintenancePlaybookOption struct {
	ID          string
	Title       string
	Description string
	Tags        []string
	Steps       []MaintenanceStepOption
}

// MaintenanceStepOption captures an individual maintenance action.
type MaintenanceStepOption struct {
	ID                string
	Title             string
	Summary           string
	Duration          time.Duration
	Responsible       string
	RequiresApproval  bool
	DependsOn         []string
	AutomationJobType string
}

// MaintenanceWindowOption lists recurring maintenance windows.
type MaintenanceWindowOption struct {
	ID       string
	Name     string
	Cron     string
	Duration time.Duration
	Timezone string
}

// MaintenanceUpgradePlanOption describes an upgrade execution plan.
type MaintenanceUpgradePlanOption struct {
	ID            string
	Version       string
	Summary       string
	ScheduledFor  string
	Prerequisites []string
	Steps         []MaintenanceStepOption
	RollbackPlan  []MaintenanceStepOption
	ReleaseNotes  string
}

// BackupOptions capture backup wizard metadata.
type BackupOptions struct {
	Enabled      bool
	Schedules    []BackupScheduleOption
	Destinations []BackupDestinationOption
	RestoreFlows []RestoreWorkflowOption
	Verification BackupVerificationOption
}

// BackupScheduleOption enumerates recurring backup jobs.
type BackupScheduleOption struct {
	ID           string
	Name         string
	Cron         string
	Retention    time.Duration
	Window       time.Duration
	Type         string
	Enabled      bool
	Destinations []string
}

// BackupDestinationOption documents backup storage targets.
type BackupDestinationOption struct {
	ID          string
	Name        string
	Kind        string
	Endpoint    string
	Credentials string
	Metadata    map[string]string
}

// RestoreWorkflowOption captures guided restore flows.
type RestoreWorkflowOption struct {
	ID          string
	Name        string
	Description string
	Steps       []MaintenanceStepOption
	Checks      []string
	Approvals   []string
}

// BackupVerificationOption mirrors verification policies for backups.
type BackupVerificationOption struct {
	Enabled      bool
	Schedule     string
	Retention    int
	Sandbox      string
	AlertChannel string
}

// PerformanceOptions define diagnostic probes and remediation playbooks.
type PerformanceOptions struct {
	Enabled         bool
	Probes          []PerformanceProbeOption
	Indicators      []PerformanceIndicatorOption
	Recommendations []PerformancePlaybookOption
}

// PerformanceProbeOption describes a diagnostic probe.
type PerformanceProbeOption struct {
	ID          string
	Name        string
	Description string
	Command     string
	Interval    time.Duration
	SLO         float64
	Units       string
	Thresholds  map[string]float64
}

// PerformanceIndicatorOption enumerates KPIs for the diagnostics panel.
type PerformanceIndicatorOption struct {
	ID          string
	Name        string
	Description string
	Target      float64
	Units       string
}

// PerformancePlaybookOption provides remediation tactics for bottlenecks.
type PerformancePlaybookOption struct {
	ID      string
	Title   string
	Summary string
	Impact  string
	Steps   []MaintenanceStepOption
	Signals []string
}
