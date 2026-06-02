package server

import (
	"context"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/automation"
	approvals "modern-dhcp/internal/automation/approvals"
	workflowsvc "modern-dhcp/internal/automation/workflow"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/ha"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/notifications"
	"modern-dhcp/internal/ops"
	"modern-dhcp/internal/rbac"
	"modern-dhcp/internal/superadmin"
)

// APIKeyMetadata is attached to each static management API key.
type APIKeyMetadata struct {
	DisplayName string
	Role        string
	PrincipalID string
}

// Options configures HTTP server behavior.
type Options struct {
	RequireAuth              bool
	APIKeys                  map[string]APIKeyMetadata
	OAuth                    OAuthOptions
	API                      APIOptions
	APITLS                   APITLSOptions
	Environment              string
	RBAC                     RBACOptions
	HAConfig                 config.HAConfig
	Metrics                  *metrics.Collector
	PrometheusEnabled        bool
	PrometheusEndpoint       string
	CORS                     CORSOptions
	HealthHooks              []HealthHook
	Coordinator              failover.StatusReporter
	FailoverController       FailoverController
	HARunbooks               []string
	Monitoring               MonitoringOptions
	Alerting                 AlertingOptions
	Dashboard                DashboardOptions
	Automation               *automation.Service
	AutomationApprovals      *approvals.Service
	AutomationApprovalPolicy map[approvals.RequestType]struct{}
	Workflow                 *workflowsvc.Service
	OpsSupport               ops.Options
	OpsSettingsStore         ops.SettingsStore
	OptionRepo               OptionRepository
	TemplateRepo             TemplateRepository
	ScopeRepo                ScopeRepository
	TemplateApplyHistoryRepo TemplateApplyHistoryRepository
	ClusterConfigStore       ClusterConfigStore
	JoinJobExecutor          JoinJobExecutor
	UI                       UIOptions
	SuperAdmin               SuperAdminOptions
	TokenProvider            auth.TokenProvider
	Telemetry                TelemetryOptions
}

type APITLSOptions struct {
	Enabled           bool
	AutoSelfSigned    bool
	CertFile          string
	KeyFile           string
	ClientCAFile      string
	RequireClientCert bool
	SelfSignedDir     string
}

// JoinJobExecutor provides integration hooks for node join phases.
type JoinJobExecutor interface {
	SnapshotNode(ctx context.Context, jobID, nodeID, peerAddress string) error
	CatchUpNode(ctx context.Context, jobID, nodeID, peerAddress string) error
	VerifyNode(ctx context.Context, jobID, nodeID, peerAddress string) error
}

// APIOptions configures versioning, docs, and usage controls.
type APIOptions struct {
	Versions       []string
	DefaultVersion string
	OpenAPI        OpenAPIOptions
	RateLimit      RateLimitOptions
	Quota          QuotaOptions
}

// OAuthOptions carry bearer token validation settings.
type OAuthOptions struct {
	Enabled        bool
	Issuer         string
	Audience       string
	JWKSURL        string
	JWKSCacheTTL   time.Duration
	HMACSecret     string
	RequiredScopes []string
	ScopeRoles     map[string]string
	DefaultRole    string
	ClockSkew      time.Duration
}

// OpenAPIOptions expose documentation endpoints.
type OpenAPIOptions struct {
	Enabled   bool
	ServePath string
}

// RateLimitOptions define throttling for API traffic.
type RateLimitOptions struct {
	Enabled      bool
	Default      RateLimitProfile
	PerAPIKey    map[string]RateLimitProfile
	PerRole      map[string]RateLimitProfile
	FallbackToIP bool
}

// RateLimitProfile captures token-bucket params.
type RateLimitProfile struct {
	RequestsPerMinute int
	Burst             int
}

// QuotaOptions enforce day-level quotas.
type QuotaOptions struct {
	Enabled          bool
	DefaultDaily     int
	PerAPIKey        map[string]int
	PerRole          map[string]int
	ResetHourUTC     int
	IncludeWriteOnly bool
}

// MonitoringOptions configures the monitoring API surface.
type MonitoringOptions struct {
	Aggregator      *monitoring.Aggregator
	AlertFeed       *monitoring.AlertFeed
	AlertManager    *alerting.Manager
	AlertController *monitoring.AlertController
}

// AlertingOptions wires workflow stores and notification dispatcher.
type AlertingOptions struct {
	Routes        *alerting.RoutingStore
	Schedule      *alerting.DutySchedule
	Dispatcher    *notifications.Dispatcher
	RuleStore     alerting.RuleStore
	RouteStore    alerting.RouteStore
	ConfigStore   alerting.ConfigStore
	TemplateStore alerting.TemplateStore
	ReceiverStore alerting.ReceiverStore
}

// DashboardOptions customizes control plane cache + stream settings.
type DashboardOptions struct {
	CacheTTL           time.Duration
	DefaultStreamLimit int
	MaxStreamLimit     int
	HotspotLimit       int
}

// UIOptions describes SPA bootstrap metadata to expose via API.
type UIOptions struct {
	Themes        ThemeOptions         `json:"themes"`
	Localization  LocalizationOptions  `json:"localization"`
	Accessibility AccessibilityOptions `json:"accessibility"`
	Collaboration CollaborationOptions `json:"collaboration"`
	Visualization VisualizationOptions `json:"visualization"`
}

// RBACOptions wires the capability resolver into the HTTP server.
type RBACOptions struct {
	Resolver            *rbac.Resolver
	EnforceCapabilities bool
	ShadowMode          bool
}

// ThemeOptions configure available UI themes.
type ThemeOptions struct {
	Default             string   `json:"default"`
	Supported           []string `json:"supported"`
	AllowTenantOverride bool     `json:"allowTenantOverride"`
}

// LocalizationOptions list supported locales.
type LocalizationOptions struct {
	DefaultLocale string   `json:"defaultLocale"`
	Supported     []string `json:"supported"`
	Fallback      string   `json:"fallback"`
	DetectBrowser bool     `json:"detectBrowser"`
}

// AccessibilityOptions toggle inclusive features.
type AccessibilityOptions struct {
	HighContrastEnabled bool    `json:"highContrastEnabled"`
	FontScaleMin        float64 `json:"fontScaleMin"`
	FontScaleMax        float64 `json:"fontScaleMax"`
	ReducedMotion       bool    `json:"reducedMotion"`
	ScreenReaderHints   bool    `json:"screenReaderHints"`
	FocusOutline        bool    `json:"focusOutline"`
	AnnounceLiveRegions bool    `json:"announceLiveRegions"`
	ValidationStrategy  string  `json:"validationStrategy"`
	LiveRegionDebounce  int64   `json:"liveRegionDebounceMillis"`
}

// CollaborationOptions govern real-time editing semantics.
type CollaborationOptions struct {
	Enabled            bool  `json:"enabled"`
	OptimisticLockTTL  int64 `json:"optimisticLockTtlMillis"`
	PresenceHeartbeat  int64 `json:"presenceHeartbeatMillis"`
	MaxConcurrentLocks int   `json:"maxConcurrentLocks"`
	CommentLengthLimit int   `json:"commentLengthLimit"`
	ApprovalLevels     int   `json:"approvalLevels"`
	TaskQueueEnabled   bool  `json:"taskQueueEnabled"`
}

// VisualizationOptions control heavy canvases and providers.
type VisualizationOptions struct {
	Enable3D               bool   `json:"enable3d"`
	MapProvider            string `json:"mapProvider"`
	LeaseHeatmapResolution string `json:"leaseHeatmapResolution"`
	TopologyAutoDiscovery  bool   `json:"topologyAutoDiscovery"`
	MaxCanvasNodes         int    `json:"maxCanvasNodes"`
	SnapshotIntervalMillis int64  `json:"snapshotIntervalMillis"`
}

// TelemetryOptions configures telemetry settings.
type TelemetryOptions struct {
	Enabled      bool    `json:"enabled"`
	Endpoint     string  `json:"endpoint"`
	Insecure     bool    `json:"insecure"`
	SamplerRatio float64 `json:"samplerRatio"`
	ServiceName  string  `json:"serviceName"`
	Environment  string  `json:"environment"`
}

// SuperAdminOptions wires the console super admin bootstrapper.
type SuperAdminOptions struct {
	Enabled  bool
	Username string
	APIKey   string
	Manager  *superadmin.Manager
}

// CORSOptions determines cross-origin behavior for the HTTP server.
type CORSOptions struct {
	Enabled        bool
	AllowedOrigins []string
}

// HealthHook allows subsystems to feed health information into /healthz.
type HealthHook interface {
	Name() string
	Check(ctx context.Context) error
}

type healthFunc struct {
	name string
	fn   func(ctx context.Context) error
}

// FailoverController exposes manual failover hooks to management APIs.
type FailoverController interface {
	ha.Controller
}

// NewHealthHook registers a named callback for /healthz aggregation.
func NewHealthHook(name string, fn func(ctx context.Context) error) HealthHook {
	return &healthFunc{name: name, fn: fn}
}

func (h *healthFunc) Name() string { return h.name }

func (h *healthFunc) Check(ctx context.Context) error {
	if h == nil || h.fn == nil {
		return nil
	}
	return h.fn(ctx)
}
