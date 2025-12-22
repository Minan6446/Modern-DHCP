package config

import (
	"fmt"
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Config captures all tunables required by the Modern-DHCP service runtime.
type Config struct {
	Service       ServiceConfig       `mapstructure:"service"`
	Deployment    DeploymentConfig    `mapstructure:"deployment"`
	Storage       StorageMatrixConfig `mapstructure:"storage"`
	Tenancy       TenancyConfig       `mapstructure:"tenancy"`
	RBAC          RBACConfig          `mapstructure:"rbac"`
	Backup        BackupConfig        `mapstructure:"backup"`
	UI            UIConfig            `mapstructure:"ui"`
	API           APIConfig           `mapstructure:"api"`
	MySQL         MySQLConfig         `mapstructure:"mysql"`
	Postgres      PostgresConfig      `mapstructure:"postgres"`
	Redis         RedisConfig         `mapstructure:"redis"`
	Kafka         KafkaConfig         `mapstructure:"kafka"`
	HA            HAConfig            `mapstructure:"ha"`
	Policy        PolicyConfig        `mapstructure:"policy"`
	IoT           IoTConfig           `mapstructure:"iot"`
	Security      SecurityConfig      `mapstructure:"security"`
	Relay         RelayProxyConfig    `mapstructure:"relay"`
	Monitoring    MonitoringConfig    `mapstructure:"monitoring"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Events        EventsConfig        `mapstructure:"events"`
	Automation    AutomationConfig    `mapstructure:"automation"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	OpsSupport    OpsSupportConfig    `mapstructure:"opsSupport"`
}

// ServiceConfig describes high-level server behaviors.
type ServiceConfig struct {
	Name        string              `mapstructure:"name"`
	Env         string              `mapstructure:"env"`
	NodeID      string              `mapstructure:"nodeID"`
	BindAddress string              `mapstructure:"bindAddress"`
	DHCPv4Port  int                 `mapstructure:"dhcpv4Port"`
	DHCPv6Port  int                 `mapstructure:"dhcpv6Port"`
	APIPort     int                 `mapstructure:"apiPort"`
	MetricsPort int                 `mapstructure:"metricsPort"`
	EnableIPv6  bool                `mapstructure:"enableIPv6"`
	EnableBOOTP bool                `mapstructure:"enableBOOTP"`
	CORS        CORSConfig          `mapstructure:"cors"`
	DHCPv4      DHCPv4RuntimeConfig `mapstructure:"dhcpv4"`
}

// DHCPv4RuntimeConfig tunes the UDP server concurrency knobs.
type DHCPv4RuntimeConfig struct {
	WorkerCount    int   `mapstructure:"workerCount"`
	QueueDepth     int   `mapstructure:"queueDepth"`
	ListenerFanout int   `mapstructure:"listenerFanout"`
	RXBatchSize    int   `mapstructure:"rxBatchSize"`
	ReusePort      *bool `mapstructure:"reusePort"`
}

// UIConfig describes SPA-level preferences surfaced to clients.
type UIConfig struct {
	Themes        ThemeConfig         `mapstructure:"themes"`
	Localization  LocalizationConfig  `mapstructure:"localization"`
	Accessibility AccessibilityConfig `mapstructure:"accessibility"`
	Collaboration CollaborationConfig `mapstructure:"collaboration"`
	Visualization VisualizationConfig `mapstructure:"visualization"`
}

// ThemeConfig enumerates supported UI themes.
type ThemeConfig struct {
	Default             string   `mapstructure:"default"`
	Supported           []string `mapstructure:"supported"`
	AllowTenantOverride bool     `mapstructure:"allowTenantOverride"`
}

// LocalizationConfig defines language packs and detection behavior.
type LocalizationConfig struct {
	DefaultLocale string   `mapstructure:"defaultLocale"`
	Supported     []string `mapstructure:"supported"`
	Fallback      string   `mapstructure:"fallback"`
	DetectBrowser bool     `mapstructure:"detectBrowser"`
}

// AccessibilityConfig toggles inclusive-experience features.
type AccessibilityConfig struct {
	HighContrastEnabled bool          `mapstructure:"highContrastEnabled"`
	FontScaleMin        float64       `mapstructure:"fontScaleMin"`
	FontScaleMax        float64       `mapstructure:"fontScaleMax"`
	ReducedMotion       bool          `mapstructure:"reducedMotion"`
	ScreenReaderHints   bool          `mapstructure:"screenReaderHints"`
	FocusOutline        bool          `mapstructure:"focusOutline"`
	AnnounceUpdates     bool          `mapstructure:"announceLiveRegions"`
	ValidationStrategy  string        `mapstructure:"validationStrategy"`
	LiveRegionDebounce  time.Duration `mapstructure:"liveRegionDebounce"`
}

// CollaborationConfig captures collaborative editing policies.
type CollaborationConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	OptimisticLockTTL  time.Duration `mapstructure:"optimisticLockTtl"`
	PresenceHeartbeat  time.Duration `mapstructure:"presenceHeartbeat"`
	MaxConcurrentLocks int           `mapstructure:"maxConcurrentLocks"`
	CommentLengthLimit int           `mapstructure:"commentLengthLimit"`
	ApprovalLevels     int           `mapstructure:"approvalLevels"`
	TaskQueueEnabled   bool          `mapstructure:"taskQueueEnabled"`
}

// VisualizationConfig tunes heavy frontend canvases.
type VisualizationConfig struct {
	Enable3D               bool          `mapstructure:"enable3d"`
	MapProvider            string        `mapstructure:"mapProvider"`
	LeaseHeatmapResolution string        `mapstructure:"leaseHeatmapResolution"`
	TopologyAutoDiscovery  bool          `mapstructure:"topologyAutoDiscovery"`
	MaxCanvasNodes         int           `mapstructure:"maxCanvasNodes"`
	SnapshotInterval       time.Duration `mapstructure:"snapshotInterval"`
}

// StorageMatrixConfig documents the supported persistence backends per environment.
type StorageMatrixConfig struct {
	Relational  RelationalStorageConfig  `mapstructure:"relational"`
	NoSQL       NoSQLStorageConfig       `mapstructure:"nosql"`
	Distributed DistributedStorageConfig `mapstructure:"distributed"`
	Cloud       CloudStorageConfig       `mapstructure:"cloud"`
}

// TenancyConfig toggles cross-tenant isolation modes and router hints.
type TenancyConfig struct {
	Mode          string                          `mapstructure:"mode"`
	DefaultSchema string                          `mapstructure:"defaultSchema"`
	Router        TenantRouterConfig              `mapstructure:"router"`
	Dedicated     map[string]TenantDatabaseConfig `mapstructure:"dedicated"`
}

// TenantRouterConfig tunes connection pooling for tenant-specific handles.
type TenantRouterConfig struct {
	CacheTTL     time.Duration `mapstructure:"cacheTtl"`
	IdleTTL      time.Duration `mapstructure:"idleTtl"`
	MaxOpenConns int           `mapstructure:"maxOpenConns"`
	MaxIdleConns int           `mapstructure:"maxIdleConns"`
	ConnMaxLife  time.Duration `mapstructure:"connMaxLifetime"`
}

// TenantDatabaseConfig enumerates dedicated data sources per tenant.
type TenantDatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
	Schema string `mapstructure:"schema"`
}

// RelationalStorageConfig captures relational database options and priorities.
type RelationalStorageConfig struct {
	Primary  string                `mapstructure:"primary"`
	Failover string                `mapstructure:"failover"`
	MySQL    RelationalInstance    `mapstructure:"mysql"`
	Postgres PostgresInstance      `mapstructure:"postgres"`
	RDS      CloudRelationalTarget `mapstructure:"rds"`
}

// RelationalInstance describes an on-prem or self-managed deployment.
type RelationalInstance struct {
	Enabled        bool          `mapstructure:"enabled"`
	DSN            string        `mapstructure:"dsn"`
	MaxConnections int           `mapstructure:"maxConnections"`
	ConnMaxLife    time.Duration `mapstructure:"connMaxLifetime"`
	TLS            TLSConfig     `mapstructure:"tls"`
	MigrationTool  string        `mapstructure:"migrationTool"`
}

// PostgresInstance mirrors RelationalInstance while keeping Postgres extras.
type PostgresInstance struct {
	Enabled        bool          `mapstructure:"enabled"`
	DSN            string        `mapstructure:"dsn"`
	MaxConnections int           `mapstructure:"maxConnections"`
	ConnMaxLife    time.Duration `mapstructure:"connMaxLifetime"`
	SSLMode        string        `mapstructure:"sslMode"`
	MigrationTool  string        `mapstructure:"migrationTool"`
}

// CloudRelationalTarget references managed SQL offerings.
type CloudRelationalTarget struct {
	Provider       string   `mapstructure:"provider"`
	Instance       string   `mapstructure:"instance"`
	Endpoints      []string `mapstructure:"endpoints"`
	FailoverPolicy string   `mapstructure:"failoverPolicy"`
}

// NoSQLStorageConfig lists cache/document backing stores.
type NoSQLStorageConfig struct {
	RedisCluster RedisClusterConfig `mapstructure:"redis"`
	Mongo        MongoConfig        `mapstructure:"mongo"`
}

// RedisClusterConfig describes Redis cluster mode deployment details.
type RedisClusterConfig struct {
	Mode      string   `mapstructure:"mode"`
	Addresses []string `mapstructure:"addresses"`
	Username  string   `mapstructure:"username"`
	Password  string   `mapstructure:"password"`
	UseTLS    bool     `mapstructure:"useTls"`
}

// MongoConfig captures MongoDB replica set / sharded cluster info.
type MongoConfig struct {
	Enabled    bool      `mapstructure:"enabled"`
	URI        string    `mapstructure:"uri"`
	Database   string    `mapstructure:"database"`
	ReplicaSet string    `mapstructure:"replicaSet"`
	TLS        TLSConfig `mapstructure:"tls"`
}

// DistributedStorageConfig enumerates coordination backends.
type DistributedStorageConfig struct {
	Etcd   DistributedBackend `mapstructure:"etcd"`
	Consul DistributedBackend `mapstructure:"consul"`
	ZK     DistributedBackend `mapstructure:"zookeeper"`
}

// DistributedBackend describes endpoint and quorum requirements.
type DistributedBackend struct {
	Enabled   bool      `mapstructure:"enabled"`
	Endpoints []string  `mapstructure:"endpoints"`
	Namespace string    `mapstructure:"namespace"`
	TLS       TLSConfig `mapstructure:"tls"`
}

// CloudStorageConfig references cloud-native databases and object stores.
type CloudStorageConfig struct {
	ObjectStores []CloudObjectStore `mapstructure:"objectStores"`
	Secrets      []string           `mapstructure:"secrets"`
}

// CloudObjectStore describes S3/Blob/Bucket definitions for backups.
type CloudObjectStore struct {
	Provider string `mapstructure:"provider"`
	Bucket   string `mapstructure:"bucket"`
	Region   string `mapstructure:"region"`
	Prefix   string `mapstructure:"prefix"`
	KMSKey   string `mapstructure:"kmsKey"`
}

// BackupConfig drives automated backup/restore policies.
type BackupConfig struct {
	Enabled             bool               `mapstructure:"enabled"`
	Schedule            string             `mapstructure:"schedule"`
	FullInterval        time.Duration      `mapstructure:"fullInterval"`
	IncrementalInterval time.Duration      `mapstructure:"incrementalInterval"`
	RetentionDays       int                `mapstructure:"retentionDays"`
	Locations           []string           `mapstructure:"locations"`
	CrossRegion         CrossRegionBackup  `mapstructure:"crossRegion"`
	Verification        BackupVerification `mapstructure:"verification"`
	RestoreUI           RestoreInterface   `mapstructure:"restoreUi"`
}

// CrossRegionBackup governs off-site replication of backup sets.
type CrossRegionBackup struct {
	Enabled   bool          `mapstructure:"enabled"`
	Regions   []string      `mapstructure:"regions"`
	Transport string        `mapstructure:"transport"`
	LagTarget time.Duration `mapstructure:"lagTarget"`
}

// BackupVerification configures automated restore tests.
type BackupVerification struct {
	Enabled      bool   `mapstructure:"enabled"`
	Schedule     string `mapstructure:"schedule"`
	SandboxEnv   string `mapstructure:"sandboxEnv"`
	Retention    int    `mapstructure:"retention"`
	AlertChannel string `mapstructure:"alertChannel"`
}

// RestoreInterface documents one-click/graphical restore hooks.
type RestoreInterface struct {
	Enabled      bool   `mapstructure:"enabled"`
	URL          string `mapstructure:"url"`
	AuthProvider string `mapstructure:"authProvider"`
	AuditSink    string `mapstructure:"auditSink"`
}

// DeploymentConfig enumerates the supported runtime footprints and scaling knobs.
type DeploymentConfig struct {
	Modes          []string                 `mapstructure:"modes"`
	BareMetal      BareMetalDeployment      `mapstructure:"bareMetal"`
	VirtualMachine VirtualMachineDeployment `mapstructure:"virtualMachine"`
	Containers     ContainerDeployment      `mapstructure:"containers"`
	Kubernetes     KubernetesDeployment     `mapstructure:"kubernetes"`
	Cloud          CloudDeployment          `mapstructure:"cloud"`
	Scaling        ScalingConfig            `mapstructure:"scaling"`
	Resource       ResourceAdjustmentConfig `mapstructure:"resource"`
	Geo            GeoDeploymentConfig      `mapstructure:"geo"`
	Hybrid         HybridDeploymentConfig   `mapstructure:"hybrid"`
}

// BareMetalDeployment captures physical server rollout settings.
type BareMetalDeployment struct {
	ProvisioningTool string   `mapstructure:"provisioningTool"`
	OSImage          string   `mapstructure:"osImage"`
	KernelArgs       []string `mapstructure:"kernelArgs"`
	RaidLayout       string   `mapstructure:"raidLayout"`
	NicBonding       string   `mapstructure:"nicBonding"`
	OutOfBand        string   `mapstructure:"outOfBand"`
}

// VirtualMachineDeployment captures hypervisor template metadata.
type VirtualMachineDeployment struct {
	Hypervisors []string `mapstructure:"hypervisors"`
	Template    string   `mapstructure:"template"`
	CPU         int      `mapstructure:"cpu"`
	MemoryGB    int      `mapstructure:"memoryGB"`
	DiskGB      int      `mapstructure:"diskGB"`
	Drivers     []string `mapstructure:"drivers"`
}

// ContainerDeployment describes single-node container packaging.
type ContainerDeployment struct {
	Runtime        string   `mapstructure:"runtime"`
	Image          string   `mapstructure:"image"`
	Tag            string   `mapstructure:"tag"`
	ComposeFile    string   `mapstructure:"composeFile"`
	PublishedPorts []string `mapstructure:"publishedPorts"`
	StorageClass   string   `mapstructure:"storageClass"`
}

// KubernetesDeployment documents the cluster-native expectations.
type KubernetesDeployment struct {
	Distribution string            `mapstructure:"distribution"`
	Namespace    string            `mapstructure:"namespace"`
	HelmChart    string            `mapstructure:"helmChart"`
	ValuesFile   string            `mapstructure:"valuesFile"`
	NodeSelector map[string]string `mapstructure:"nodeSelector"`
	Tolerations  []string          `mapstructure:"tolerations"`
	HPA          HPAConfig         `mapstructure:"hpa"`
}

// HPAConfig mirrors the auto-scaling envelope when running on Kubernetes.
type HPAConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	MinReplicas        int           `mapstructure:"minReplicas"`
	MaxReplicas        int           `mapstructure:"maxReplicas"`
	ScaleUpThreshold   int           `mapstructure:"scaleUpThreshold"`
	ScaleDownThreshold int           `mapstructure:"scaleDownThreshold"`
	Cooldown           time.Duration `mapstructure:"cooldown"`
	Metrics            []string      `mapstructure:"metrics"`
}

// CloudDeployment spells out managed dependencies in public clouds.
type CloudDeployment struct {
	Providers    []string `mapstructure:"providers"`
	LoadBalancer string   `mapstructure:"loadBalancer"`
	Database     string   `mapstructure:"database"`
	Cache        string   `mapstructure:"cache"`
	InfraAsCode  []string `mapstructure:"infraAsCode"`
	Regions      []string `mapstructure:"regions"`
}

// ScalingConfig centralizes thresholds used by auto-scale controllers.
type ScalingConfig struct {
	AutoHorizontalScaling bool          `mapstructure:"autoHorizontalScaling"`
	Metric                string        `mapstructure:"metric"`
	ScaleOutThreshold     int           `mapstructure:"scaleOutThreshold"`
	ScaleInThreshold      int           `mapstructure:"scaleInThreshold"`
	MinReplicas           int           `mapstructure:"minReplicas"`
	MaxReplicas           int           `mapstructure:"maxReplicas"`
	Cooldown              time.Duration `mapstructure:"cooldown"`
}

// ResourceAdjustmentConfig allows on-demand CPU/memory envelopes.
type ResourceAdjustmentConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	CPU             ResourceRange `mapstructure:"cpu"`
	MemoryGB        ResourceRange `mapstructure:"memoryGB"`
	TriggerPolicies []string      `mapstructure:"triggerPolicies"`
}

// ResourceRange bounds a tunable resource with discrete steps.
type ResourceRange struct {
	Min  int `mapstructure:"min"`
	Max  int `mapstructure:"max"`
	Step int `mapstructure:"step"`
}

// GeoDeploymentConfig captures region/edge placements.
type GeoDeploymentConfig struct {
	Enabled     bool         `mapstructure:"enabled"`
	EdgeSupport bool         `mapstructure:"edgeSupport"`
	Regions     []RegionSpec `mapstructure:"regions"`
}

// RegionSpec documents a region's role within the fleet.
type RegionSpec struct {
	Name          string        `mapstructure:"name"`
	Role          string        `mapstructure:"role"`
	Provider      string        `mapstructure:"provider"`
	Clusters      int           `mapstructure:"clusters"`
	LatencyBudget time.Duration `mapstructure:"latencyBudget"`
	EdgeSites     []string      `mapstructure:"edgeSites"`
}

// HybridDeploymentConfig wires hybrid-cloud networking policy.
type HybridDeploymentConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	VPNMesh        bool     `mapstructure:"vpnMesh"`
	TransitType    string   `mapstructure:"transitType"`
	CloudProviders []string `mapstructure:"cloudProviders"`
	OnPremRegions  []string `mapstructure:"onPremRegions"`
	Policy         string   `mapstructure:"policy"`
}

// APIConfig captures REST API surface features.
type APIConfig struct {
	Versions       []string           `mapstructure:"versions"`
	DefaultVersion string             `mapstructure:"defaultVersion"`
	OpenAPI        OpenAPIConfig      `mapstructure:"openapi"`
	RateLimit      APIRateLimitConfig `mapstructure:"rateLimit"`
	Quota          APIQuotaConfig     `mapstructure:"quota"`
}

// OpenAPIConfig toggles documentation endpoints.
type OpenAPIConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	ServePath string `mapstructure:"servePath"`
}

// APIRateLimitConfig defines throttling per API consumer.
type APIRateLimitConfig struct {
	Enabled      bool                           `mapstructure:"enabled"`
	Default      APIRateLimitProfile            `mapstructure:"default"`
	PerAPIKey    map[string]APIRateLimitProfile `mapstructure:"perApiKey"`
	PerRole      map[string]APIRateLimitProfile `mapstructure:"perRole"`
	FallbackToIP bool                           `mapstructure:"fallbackToIp"`
}

// APIRateLimitProfile stores basic token-bucket parameters.
type APIRateLimitProfile struct {
	RequestsPerMinute int `mapstructure:"requestsPerMinute"`
	Burst             int `mapstructure:"burst"`
}

// APIQuotaConfig enforces daily request caps.
type APIQuotaConfig struct {
	Enabled          bool           `mapstructure:"enabled"`
	DefaultDaily     int            `mapstructure:"defaultDaily"`
	PerAPIKey        map[string]int `mapstructure:"perApiKey"`
	PerRole          map[string]int `mapstructure:"perRole"`
	ResetHourUTC     int            `mapstructure:"resetHourUTC"`
	IncludeWriteOnly bool           `mapstructure:"includeWriteOnly"`
}

// CORSConfig configures cross-origin access to the management API.
type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	AllowedOrigins []string `mapstructure:"allowedOrigins"`
}

// MySQLConfig holds database settings.
type MySQLConfig struct {
	DSN          string        `mapstructure:"dsn"`
	MaxOpenConns int           `mapstructure:"maxOpenConns"`
	MaxIdleConns int           `mapstructure:"maxIdleConns"`
	ConnMaxLife  time.Duration `mapstructure:"connMaxLifetime"`
	ConnMaxIdle  time.Duration `mapstructure:"connMaxIdleTime"`
}

// PostgresConfig holds PostgreSQL connection settings.
type PostgresConfig struct {
	DSN          string        `mapstructure:"dsn"`
	MaxOpenConns int           `mapstructure:"maxOpenConns"`
	MaxIdleConns int           `mapstructure:"maxIdleConns"`
	ConnMaxLife  time.Duration `mapstructure:"connMaxLifetime"`
	ConnMaxIdle  time.Duration `mapstructure:"connMaxIdleTime"`
}

// RedisConfig keeps caching parameters.
type RedisConfig struct {
	Addresses []string `mapstructure:"addresses"`
	Username  string   `mapstructure:"username"`
	Password  string   `mapstructure:"password"`
	Database  int      `mapstructure:"database"`
}

// KafkaConfig describes bus connectivity.
type KafkaConfig struct {
	Brokers  []string    `mapstructure:"brokers"`
	ClientID string      `mapstructure:"clientID"`
	Topics   KafkaTopics `mapstructure:"topics"`
}

// KafkaTopics centralizes topic names.
type KafkaTopics struct {
	LeaseEvents    string `mapstructure:"leaseEvents"`
	SecurityEvents string `mapstructure:"securityEvents"`
	PolicyEvents   string `mapstructure:"policyEvents"`
}

// HAConfig determines high availability behavior.
type HAConfig struct {
	Mode              string             `mapstructure:"mode"`
	HeartbeatInterval time.Duration      `mapstructure:"heartbeatInterval"`
	FailoverTimeout   time.Duration      `mapstructure:"failoverTimeout"`
	Partner           PartnerConfig      `mapstructure:"partner"`
	Coordinator       CoordinatorConfig  `mapstructure:"coordinator"`
	Coordinators      []NamedCoordinator `mapstructure:"coordinators"`
	Replication       ReplicationConfig  `mapstructure:"replication"`
	ConfigSync        ConfigSyncConfig   `mapstructure:"configSync"`
	Node              HANodeMetadata     `mapstructure:"node"`
	Ingress           IngressConfig      `mapstructure:"ingress"`
	Broadcast         BroadcastConfig    `mapstructure:"broadcast"`
	Discovery         DiscoveryConfig    `mapstructure:"discovery"`
	Probe             ProbeConfig        `mapstructure:"probe"`
	Failback          FailbackConfig     `mapstructure:"failback"`
	Runbooks          []string           `mapstructure:"runbooks"`
}

// PartnerConfig captures peer endpoint and protocol defaults.
type PartnerConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Address string        `mapstructure:"address"`
	Port    int           `mapstructure:"port"`
	MCLT    time.Duration `mapstructure:"mclt"`
}

// HANodeMetadata uniquely identifies a node in discovery/registry flows.
type HANodeMetadata struct {
	ID     string `mapstructure:"id"`
	Region string `mapstructure:"region"`
	Zone   string `mapstructure:"zone"`
	Weight int    `mapstructure:"weight"`
}

// CoordinatorConfig captures distributed lock backend settings.
type CoordinatorConfig struct {
	Backend   string        `mapstructure:"backend"`
	Endpoints []string      `mapstructure:"endpoints"`
	TLS       TLSConfig     `mapstructure:"tls"`
	Key       string        `mapstructure:"key"`
	Token     string        `mapstructure:"token"`
	LeaseTTL  time.Duration `mapstructure:"leaseTTL"`
	Interval  time.Duration `mapstructure:"interval"`
}

// NamedCoordinator allows configuring multiple coordinator backends.
type NamedCoordinator struct {
	Name string `mapstructure:"name"`
	CoordinatorConfig
}

// TLSConfig points to certificate materials.
type TLSConfig struct {
	CAFile   string `mapstructure:"caFile"`
	CertFile string `mapstructure:"certFile"`
	KeyFile  string `mapstructure:"keyFile"`
}

// ReplicationConfig describes CDC/Binlog synchronization for standby nodes.
type ReplicationConfig struct {
	CDCEnabled       bool          `mapstructure:"cdcEnabled"`
	BinlogSource     string        `mapstructure:"binlogSource"`
	SnapshotInterval time.Duration `mapstructure:"snapshotInterval"`
	Brokers          []string      `mapstructure:"brokers"`
	Topic            string        `mapstructure:"topic"`
	AckTimeout       time.Duration `mapstructure:"ackTimeout"`
}

// ConfigSyncConfig governs GitOps/etcd configuration validation.
type ConfigSyncConfig struct {
	Backend          string        `mapstructure:"backend"`
	Path             string        `mapstructure:"path"`
	ExpectedChecksum string        `mapstructure:"expectedChecksum"`
	RefreshInterval  time.Duration `mapstructure:"refreshInterval"`
	Enforce          bool          `mapstructure:"enforce"`
}

// IngressConfig defines front-door load balancer modes.
type IngressConfig struct {
	Mode          string           `mapstructure:"mode"`
	VIPInterface  string           `mapstructure:"vipInterface"`
	VIPAddress    string           `mapstructure:"vipAddress"`
	DNSEntry      string           `mapstructure:"dnsEntry"`
	AnycastPrefix string           `mapstructure:"anycastPrefix"`
	HealthGrace   time.Duration    `mapstructure:"healthGrace"`
	DNS           IngressDNSConfig `mapstructure:"dns"`
}

// IngressDNSConfig configures DNS withdrawal/registration.
type IngressDNSConfig struct {
	Provider string        `mapstructure:"provider"`
	Endpoint string        `mapstructure:"endpoint"`
	Token    string        `mapstructure:"token"`
	TTL      time.Duration `mapstructure:"ttl"`
}

// BroadcastConfig configures state dissemination across nodes.
type BroadcastConfig struct {
	Backend   string   `mapstructure:"backend"`
	Endpoints []string `mapstructure:"endpoints"`
	Topic     string   `mapstructure:"topic"`
}

// DiscoveryConfig controls registry-based auto-discovery.
type DiscoveryConfig struct {
	Backend          string        `mapstructure:"backend"`
	Endpoints        []string      `mapstructure:"endpoints"`
	TTL              time.Duration `mapstructure:"ttl"`
	RegisterInterval time.Duration `mapstructure:"registerInterval"`
	DrainGrace       time.Duration `mapstructure:"drainGrace"`
}

// ProbeConfig tunes active health checking between peers or coordination agents.
type ProbeConfig struct {
	Backend          string        `mapstructure:"backend"`
	Target           string        `mapstructure:"target"`
	Port             int           `mapstructure:"port"`
	Interval         time.Duration `mapstructure:"interval"`
	Timeout          time.Duration `mapstructure:"timeout"`
	FailureThreshold int           `mapstructure:"failureThreshold"`
	Payload          string        `mapstructure:"payload"`
	SerfTags         []string      `mapstructure:"serfTags"`
	ConsulDatacenter string        `mapstructure:"consulDatacenter"`
	ConsulToken      string        `mapstructure:"consulToken"`
}

// FailbackConfig toggles automatic vs manual failback policies.
type FailbackConfig struct {
	Mode         string        `mapstructure:"mode"` // auto | manual
	StablePeriod time.Duration `mapstructure:"stablePeriod"`
}

// PolicyConfig defines rule processing limits.
type PolicyConfig struct {
	CacheTTL            time.Duration        `mapstructure:"cacheTTL"`
	MaxRulesPerTenant   int                  `mapstructure:"maxRulesPerTenant"`
	DefaultLeaseProfile LeaseProfileConfig   `mapstructure:"defaultLeaseProfile"`
	Lifecycle           LifecycleConfig      `mapstructure:"lifecycle"`
	MobileProfiles      MobileProfilesConfig `mapstructure:"mobileProfiles"`
}

// MobileProfilesConfig drives mobile device fingerprint matching rules.
type MobileProfilesConfig struct {
	Enabled  bool                `mapstructure:"enabled"`
	Profiles []MobileProfileSpec `mapstructure:"profiles"`
}

// MobileProfileSpec maps heuristics to high-level platform/persona tags.
type MobileProfileSpec struct {
	Name                string   `mapstructure:"name"`
	Platform            string   `mapstructure:"platform"`
	Persona             string   `mapstructure:"persona"`
	Tags                []string `mapstructure:"tags"`
	VendorClassContains []string `mapstructure:"vendorClassContains"`
	UserClassContains   []string `mapstructure:"userClassContains"`
	RelayKeywords       []string `mapstructure:"relayKeywords"`
	OUIs                []string `mapstructure:"ouis"`
	Option55Contains    []int    `mapstructure:"option55Contains"`
}

// IoTConfig captures knobs for IoT aware services.
type IoTConfig struct {
	Registry IoTRegistryConfig `mapstructure:"registry"`
}

// IoTRegistryConfig tunes the registry defaults exposed via admin APIs.
type IoTRegistryConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	DefaultSleepInterval time.Duration `mapstructure:"defaultSleepInterval"`
	DefaultOfflineWindow time.Duration `mapstructure:"defaultOfflineWindow"`
}

// LeaseProfileConfig stores lease duration policies.
type LeaseProfileConfig struct {
	DefaultDuration     time.Duration `mapstructure:"default"`
	MinDuration         time.Duration `mapstructure:"min"`
	MaxDuration         time.Duration `mapstructure:"max"`
	NotificationLead    time.Duration `mapstructure:"notificationLead"`
	RenewalPercent      float64       `mapstructure:"renewalPercent"`
	RebindingPercent    float64       `mapstructure:"rebindingPercent"`
	Permanent           bool          `mapstructure:"permanent"`
	SleepyCapable       bool          `mapstructure:"sleepyCapable"`
	SleepyOfflineWindow time.Duration `mapstructure:"sleepyOfflineWindow"`
	SleepyHoldDuration  time.Duration `mapstructure:"sleepyHoldDuration"`
	MobilityGracePeriod time.Duration `mapstructure:"mobilityGracePeriod"`
}

// LifecycleConfig contains advanced lease lifecycle tunables.
type LifecycleConfig struct {
	AutoReclaim         AutoReclaimConfig `mapstructure:"autoReclaim"`
	CooldownDuration    time.Duration     `mapstructure:"cooldownDuration"`
	MaxCooldowns        int               `mapstructure:"maxCooldowns"`
	ConflictBackoff     time.Duration     `mapstructure:"conflictBackoff"`
	MobilityAffinityTTL time.Duration     `mapstructure:"mobilityAffinityTTL"`
}

// AutoReclaimConfig drives the background reclaim worker.
type AutoReclaimConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	Interval    time.Duration `mapstructure:"interval"`
	GracePeriod time.Duration `mapstructure:"gracePeriod"`
	BatchSize   int           `mapstructure:"batchSize"`
}

// SecurityConfig holds security guardrails.
type SecurityConfig struct {
	Enabled            bool                     `mapstructure:"enabled"`
	Snooping           SnoopingConfig           `mapstructure:"snooping"`
	RateLimit          RateLimitConfig          `mapstructure:"rateLimit"`
	Detection          DetectionConfig          `mapstructure:"detection"`
	IPSGDAI            IPSGDAIConfig            `mapstructure:"ipsgdai"`
	Radius             RadiusConfig             `mapstructure:"radius"`
	MACACL             MACACLConfig             `mapstructure:"macAcl"`
	Exhaustion         ExhaustionConfig         `mapstructure:"exhaustion"`
	MDM                MDMConfig                `mapstructure:"mdm"`
	ConflictPrevention ConflictPreventionConfig `mapstructure:"conflictPrevention"`
	Policy             SecurityPolicyConfig     `mapstructure:"policy"`
}

// SecurityPolicyConfig toggles guard policy evaluation.
type SecurityPolicyConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	CacheTTL time.Duration `mapstructure:"cacheTtl"`
}

// ConflictPreventionConfig tunes proactive IP conflict detection.
type ConflictPreventionConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	ProbeTimeout     time.Duration `mapstructure:"probeTimeout"`
	HoldDuration     time.Duration `mapstructure:"holdDuration"`
	MaxAttempts      int           `mapstructure:"maxAttempts"`
	ReclaimScanLimit int           `mapstructure:"reclaimScanLimit"`
}

// MDMConfig describes external mobile device management connectors.
type MDMConfig struct {
	Enabled      bool              `mapstructure:"enabled"`
	PollInterval time.Duration     `mapstructure:"pollInterval"`
	CacheTTL     time.Duration     `mapstructure:"cacheTTL"`
	Intune       IntuneConfig      `mapstructure:"intune"`
	Jamf         JamfConfig        `mapstructure:"jamf"`
	AirWatch     AirWatchConfig    `mapstructure:"airwatch"`
	Tags         map[string]string `mapstructure:"tags"`
}

// IntuneConfig captures Azure AD client credentials for Microsoft Intune.
type IntuneConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	TenantID     string        `mapstructure:"tenantId"`
	ClientID     string        `mapstructure:"clientId"`
	ClientSecret string        `mapstructure:"clientSecret"`
	Authority    string        `mapstructure:"authority"`
	Scopes       []string      `mapstructure:"scopes"`
	SyncWindow   time.Duration `mapstructure:"syncWindow"`
	DeviceFilter []string      `mapstructure:"deviceFilter"`
}

// JamfConfig stores API credentials for Jamf Pro tenants.
type JamfConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	BaseURL      string        `mapstructure:"baseUrl"`
	ClientID     string        `mapstructure:"clientId"`
	ClientSecret string        `mapstructure:"clientSecret"`
	Site         string        `mapstructure:"site"`
	SyncWindow   time.Duration `mapstructure:"syncWindow"`
}

// AirWatchConfig stores Workspace ONE UEM connector details.
type AirWatchConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Host       string        `mapstructure:"host"`
	APIKey     string        `mapstructure:"apiKey"`
	Username   string        `mapstructure:"username"`
	Password   string        `mapstructure:"password"`
	TenantCode string        `mapstructure:"tenantCode"`
	SyncWindow time.Duration `mapstructure:"syncWindow"`
}

// RelayProxyConfig drives Section 11 relay/proxy capabilities.
type RelayProxyConfig struct {
	Enabled        bool                      `mapstructure:"enabled"`
	Option82       RelayOption82Config       `mapstructure:"option82"`
	Authentication RelayAuthenticationConfig `mapstructure:"authentication"`
	PoolSelection  RelayPoolSelectionConfig  `mapstructure:"poolSelection"`
	LoadBalancing  RelayLoadBalancingConfig  `mapstructure:"loadBalancing"`
	CrossDomain    RelayCrossDomainConfig    `mapstructure:"crossDomain"`
}

// RelayOption82Config customizes sub-option decoding and preservation.
type RelayOption82Config struct {
	PreserveRaw       bool                          `mapstructure:"preserveRaw"`
	SubOptionMappings map[string]RelaySubOptionSpec `mapstructure:"subOptionMappings"`
	VendorProfiles    map[string]RelayVendorProfile `mapstructure:"vendorProfiles"`
}

// RelaySubOptionSpec maps a sub-option code to a metadata key and decode format.
type RelaySubOptionSpec struct {
	Key     string   `mapstructure:"key"`
	Format  string   `mapstructure:"format"`
	Aliases []string `mapstructure:"aliases"`
}

// RelayVendorProfile describes vendor-specific TLVs under sub-option 9.
type RelayVendorProfile struct {
	EnterpriseID int                           `mapstructure:"enterpriseId"`
	SubOptions   map[string]RelaySubOptionSpec `mapstructure:"subOptions"`
}

// RelayAuthenticationConfig defines trusted relay identities and secrets.
type RelayAuthenticationConfig struct {
	Required      bool                 `mapstructure:"required"`
	ReplayWindow  time.Duration        `mapstructure:"replayWindow"`
	AllowedAgents []RelayAgentIdentity `mapstructure:"allowedAgents"`
	SharedSecrets map[string]string    `mapstructure:"sharedSecrets"`
}

// RelayAgentIdentity constrains which relays may talk to a server.
type RelayAgentIdentity struct {
	RelayID    string   `mapstructure:"relayId"`
	GIAddr     string   `mapstructure:"giaddr"`
	RemoteID   string   `mapstructure:"remoteId"`
	CircuitID  string   `mapstructure:"circuitId"`
	VLANRanges []string `mapstructure:"vlanRanges"`
	VPNID      string   `mapstructure:"vpnId"`
	Region     string   `mapstructure:"region"`
}

// RelayPoolSelectionConfig wires GIAddr/VRF/MPLS aware pool routing.
type RelayPoolSelectionConfig struct {
	DefaultPool string            `mapstructure:"defaultPool"`
	GiaddrRules []RelayGiaddrRule `mapstructure:"giaddrRules"`
	Rules       []RelayPoolRule   `mapstructure:"rules"`
}

// RelayGiaddrRule pins a GIAddr to a specific pool.
type RelayGiaddrRule struct {
	TenantID string `mapstructure:"tenantId"`
	GIAddr   string `mapstructure:"giaddr"`
	PoolID   string `mapstructure:"poolId"`
}

// RelayPoolRule matches relay metadata to pool IDs.
type RelayPoolRule struct {
	PoolID string             `mapstructure:"poolId"`
	Match  RelayMatchCriteria `mapstructure:"match"`
}

// RelayMatchCriteria enumerates the metadata keys a rule can inspect.
type RelayMatchCriteria struct {
	TenantID   string `mapstructure:"tenantId"`
	RelayID    string `mapstructure:"relayId"`
	GIAddr     string `mapstructure:"giaddr"`
	CircuitID  string `mapstructure:"circuitId"`
	RemoteID   string `mapstructure:"remoteId"`
	VLANID     int    `mapstructure:"vlanId"`
	VRF        string `mapstructure:"vrf"`
	VPNID      string `mapstructure:"vpnId"`
	Region     string `mapstructure:"region"`
	DataCenter string `mapstructure:"dataCenter"`
	MPLSVPN    string `mapstructure:"mplsVpn"`
	UserGroup  string `mapstructure:"userGroup"`
}

// RelayLoadBalancingConfig balances relays across DHCP nodes.
type RelayLoadBalancingConfig struct {
	Mode     string              `mapstructure:"mode"`
	HashKeys []string            `mapstructure:"hashKeys"`
	Servers  []RelayServerConfig `mapstructure:"servers"`
}

// RelayServerConfig represents a DHCP node candidate.
type RelayServerConfig struct {
	ID     string `mapstructure:"id"`
	Weight int    `mapstructure:"weight"`
	Region string `mapstructure:"region"`
	Zone   string `mapstructure:"zone"`
}

// RelayCrossDomainConfig contains cross-VLAN/L3/DC/MPLS hints.
type RelayCrossDomainConfig struct {
	RegionFallbacks map[string][]string `mapstructure:"regionFallbacks"`
	VRFRules        []RelayVRFRule      `mapstructure:"vrfRules"`
	MPLSVPNRules    []RelayMPLSRule     `mapstructure:"mplsVpnRules"`
	MultiVLANPools  []RelayVLANPool     `mapstructure:"multiVlanPools"`
}

// RelayVRFRule binds a VRF identifier to a pool.
type RelayVRFRule struct {
	VRF    string `mapstructure:"vrf"`
	PoolID string `mapstructure:"poolId"`
}

// RelayMPLSRule maps an MPLS VPN ID to a pool.
type RelayMPLSRule struct {
	VPNID  string `mapstructure:"vpnId"`
	PoolID string `mapstructure:"poolId"`
}

// RelayVLANPool allows one pool to serve multiple VLAN ranges.
type RelayVLANPool struct {
	PoolID string   `mapstructure:"poolId"`
	VLANs  []string `mapstructure:"vlans"`
}

// SnoopingConfig describes DHCP snooping ingest behavior.
type SnoopingConfig struct {
	Enabled         bool                `mapstructure:"enabled"`
	GRPCEndpoint    string              `mapstructure:"grpcEndpoint"`
	RefreshInterval time.Duration       `mapstructure:"refreshInterval"`
	CacheTTL        time.Duration       `mapstructure:"cacheTTL"`
	TrustedPorts    []string            `mapstructure:"trustedPorts"`
	Redis           SnoopingStoreConfig `mapstructure:"redis"`
}

// SnoopingStoreConfig configures the remote binding cache.
type SnoopingStoreConfig struct {
	Address  string `mapstructure:"addr"`
	Database int    `mapstructure:"db"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// RateLimitConfig sets throttling policies.
type RateLimitConfig struct {
	Default   RateLimitProfile            `mapstructure:"default"`
	Overrides map[string]RateLimitProfile `mapstructure:"overrides"`
}

// RateLimitProfile holds per-entity PPS configuration.
type RateLimitProfile struct {
	PerMacPPS  int `mapstructure:"perMacPPS"`
	PerPortPPS int `mapstructure:"perPortPPS"`
	PerIPPPS   int `mapstructure:"perIPPPS"`
	Burst      int `mapstructure:"burst"`
}

// DetectionConfig defines anomaly thresholds.
type DetectionConfig struct {
	DeclineSpikeThreshold     int           `mapstructure:"declineSpikeThreshold"`
	StarvationWindow          time.Duration `mapstructure:"starvationWindow"`
	Option82MismatchTolerance int           `mapstructure:"option82MismatchTolerance"`
	DiscoverBurstThreshold    int           `mapstructure:"discoverBurstThreshold"`
	DiscoverToRequestRatio    float64       `mapstructure:"discoverToRequestRatio"`
	BlockDuration             time.Duration `mapstructure:"blockDuration"`
	RogueServerAllowlist      []string      `mapstructure:"rogueServerAllowlist"`
	MACDriftThreshold         int           `mapstructure:"macDriftThreshold"`
}

// MACACLConfig defines static MAC allow/deny policies.
type MACACLConfig struct {
	Whitelist        []string `mapstructure:"whitelist"`
	Blacklist        []string `mapstructure:"blacklist"`
	Graylist         []string `mapstructure:"graylist"`
	GraylistAction   string   `mapstructure:"graylistAction"`
	EnforceWhitelist bool     `mapstructure:"enforceWhitelist"`
}

// ExhaustionConfig defines safeguards against address exhaustion attacks.
type ExhaustionConfig struct {
	MaxLeasesPerMAC    int             `mapstructure:"maxLeasesPerMac"`
	MaxLeasesPerUser   int             `mapstructure:"maxLeasesPerUser"`
	PoolWarnPercent    int             `mapstructure:"poolWarnPercent"`
	PoolProtectPercent int             `mapstructure:"poolProtectPercent"`
	Isolation          IsolationConfig `mapstructure:"isolation"`
}

// IsolationConfig controls temporary vs permanent quarantine behavior.
type IsolationConfig struct {
	TemporaryDuration time.Duration `mapstructure:"temporaryDuration"`
	PermanentAfter    int           `mapstructure:"permanentAfter"`
}

// IPSGDAIConfig configures IP Source Guard/DAI integration.
type IPSGDAIConfig struct {
	PublishURL string `mapstructure:"publishURL"`
	Secret     string `mapstructure:"secret"`
	Retry      int    `mapstructure:"retry"`
}

// RadiusConfig describes RADIUS/802.1X integration points.
type RadiusConfig struct {
	Servers []RadiusServerConfig `mapstructure:"servers"`
	COA     COAConfig            `mapstructure:"coa"`
}

// RadiusServerConfig captures server targets and secrets.
type RadiusServerConfig struct {
	Address      string        `mapstructure:"addr"`
	SharedSecret string        `mapstructure:"sharedSecret"`
	Timeout      time.Duration `mapstructure:"timeout"`
}

// COAConfig describes Change-of-Authorization listener settings.
type COAConfig struct {
	Listen string `mapstructure:"listen"`
}

// MonitoringConfig controls telemetry exporters.
type MonitoringConfig struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Tracing    TracingConfig    `mapstructure:"tracing"`
	Alerting   AlertingConfig   `mapstructure:"alerting"`
}

// AuthConfig defines management API authentication.
type AuthConfig struct {
	Enabled    bool                    `mapstructure:"enabled"`
	APIKeys    map[string]APIKeyConfig `mapstructure:"apiKeys"`
	OAuth      OAuthConfig             `mapstructure:"oauth"`
	Providers  ProviderCatalogConfig   `mapstructure:"providers"`
	SuperAdmin SuperAdminConfig        `mapstructure:"superAdmin"`
}

// ProviderCatalogConfig lists identity provider toggles.
type ProviderCatalogConfig struct {
	Local LocalProviderConfig `mapstructure:"local"`
	LDAP  LDAPProviderConfig  `mapstructure:"ldap"`
}

// LocalProviderConfig toggles the built-in password provider.
type LocalProviderConfig struct {
	Disabled bool `mapstructure:"disabled"`
	Default  bool `mapstructure:"default"`
}

// LDAPProviderConfig wires an LDAP directory as an identity source.
type LDAPProviderConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	Default              bool          `mapstructure:"default"`
	URL                  string        `mapstructure:"url"`
	BindDN               string        `mapstructure:"bindDN"`
	BindPassword         string        `mapstructure:"bindPassword"`
	UserBaseDN           string        `mapstructure:"userBaseDN"`
	UserFilter           string        `mapstructure:"userFilter"`
	UsernameAttribute    string        `mapstructure:"usernameAttribute"`
	DisplayNameAttribute string        `mapstructure:"displayNameAttribute"`
	EmailAttribute       string        `mapstructure:"emailAttribute"`
	UseStartTLS          bool          `mapstructure:"useStartTLS"`
	SkipTLSVerify        bool          `mapstructure:"skipTLSVerify"`
	Timeout              time.Duration `mapstructure:"timeout"`
}

// SuperAdminConfig seeds the built-in console administrator.
type SuperAdminConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Username  string `mapstructure:"username"`
	APIKeyRef string `mapstructure:"apiKeyRef"`
	StateFile string `mapstructure:"stateFile"`
}

// RBACConfig tunes capability enforcement strategy.
type RBACConfig struct {
	EnforceCapabilities bool          `mapstructure:"enforceCapabilities"`
	ShadowMode          bool          `mapstructure:"shadowMode"`
	CacheTTL            time.Duration `mapstructure:"cacheTTL"`
}

// APIKeyConfig describes a single API token's metadata and role binding.
type APIKeyConfig struct {
	DisplayName string `mapstructure:"displayName"`
	Role        string `mapstructure:"role"`
	PrincipalID string `mapstructure:"principalId"`
}

// OAuthConfig enables JWT/OIDC bearer authentication.
type OAuthConfig struct {
	Enabled        bool              `mapstructure:"enabled"`
	Issuer         string            `mapstructure:"issuer"`
	Audience       string            `mapstructure:"audience"`
	JWKSURL        string            `mapstructure:"jwksURL"`
	JWKSCacheTTL   time.Duration     `mapstructure:"jwksCacheTTL"`
	HMACSecret     string            `mapstructure:"hmacSecret"`
	RequiredScopes []string          `mapstructure:"requiredScopes"`
	ScopeRoles     map[string]string `mapstructure:"scopeRoles"`
	DefaultRole    string            `mapstructure:"defaultRole"`
	ClockSkew      time.Duration     `mapstructure:"clockSkew"`
}

// EventsConfig configures outbound webhooks for change notifications.
type EventsConfig struct {
	PolicyWebhooks []string `mapstructure:"policyWebhooks"`
}

// PrometheusConfig toggles metrics exporter.
type PrometheusConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"`
}

// TracingConfig chooses a tracing backend.
type TracingConfig struct {
	Exporter string `mapstructure:"exporter"`
	Endpoint string `mapstructure:"endpoint"`
}

// AlertingConfig powers the intelligent alert manager runtime.
type AlertingConfig struct {
	Enabled          bool                         `mapstructure:"enabled"`
	EvaluateInterval time.Duration                `mapstructure:"evaluateInterval"`
	DedupeWindow     time.Duration                `mapstructure:"dedupeWindow"`
	EscalationWindow time.Duration                `mapstructure:"escalationWindow"`
	Routes           []AlertRouteConfig           `mapstructure:"routes"`
	Notifiers        AlertNotifierConfig          `mapstructure:"notifiers"`
	Policies         map[string]AlertPolicyConfig `mapstructure:"policies"`
	DefaultPolicy    AlertPolicyConfig            `mapstructure:"defaultPolicy"`
	Rules            []AlertRuleConfig            `mapstructure:"rules"`
}

// AlertRouteConfig describes how severities fan out to channels.
type AlertRouteConfig struct {
	Name       string   `mapstructure:"name"`
	Severities []string `mapstructure:"severities"`
	Channels   []string `mapstructure:"channels"`
}

// AlertNotifierConfig captures multi-channel notifier wiring.
type AlertNotifierConfig struct {
	Email   []AlertEmailNotifier   `mapstructure:"email"`
	SMS     []AlertSMSNotifier     `mapstructure:"sms"`
	Voice   []AlertVoiceNotifier   `mapstructure:"voice"`
	Chat    []AlertChatNotifier    `mapstructure:"chat"`
	Webhook []AlertWebhookNotifier `mapstructure:"webhook"`
	SNMP    []AlertSNMPNotifier    `mapstructure:"snmp"`
	Syslog  []AlertSyslogNotifier  `mapstructure:"syslog"`
}

// AlertEmailNotifier wires SMTP-style alerts.
type AlertEmailNotifier struct {
	Name       string   `mapstructure:"name"`
	From       string   `mapstructure:"from"`
	Recipients []string `mapstructure:"recipients"`
}

// AlertSMSNotifier wires SMS downstreams.
type AlertSMSNotifier struct {
	Name    string   `mapstructure:"name"`
	Numbers []string `mapstructure:"numbers"`
}

// AlertVoiceNotifier wires IVR/call-tree downstreams.
type AlertVoiceNotifier struct {
	Name    string   `mapstructure:"name"`
	Numbers []string `mapstructure:"numbers"`
}

// AlertChatNotifier delivers to chat/webhook endpoints.
type AlertChatNotifier struct {
	Name    string `mapstructure:"name"`
	Channel string `mapstructure:"channel"`
	Webhook string `mapstructure:"webhook"`
}

// AlertWebhookNotifier pushes JSON payloads to automation systems.
type AlertWebhookNotifier struct {
	Name   string `mapstructure:"name"`
	URL    string `mapstructure:"url"`
	Secret string `mapstructure:"secret"`
}

// AlertSNMPNotifier configures SNMP trap delivery.
type AlertSNMPNotifier struct {
	Name      string `mapstructure:"name"`
	Target    string `mapstructure:"target"`
	Community string `mapstructure:"community"`
}

// AlertSyslogNotifier configures syslog forwarding.
type AlertSyslogNotifier struct {
	Name    string `mapstructure:"name"`
	Address string `mapstructure:"address"`
	Network string `mapstructure:"network"`
}

// AlertPolicyConfig tunes per-tenant alert generation.
type AlertPolicyConfig struct {
	Enabled        *bool                    `mapstructure:"enabled"`
	SilenceWindow  time.Duration            `mapstructure:"silenceWindow"`
	PoolSampleSize int                      `mapstructure:"poolSampleSize"`
	PoolThresholds PoolThresholdConfig      `mapstructure:"poolThresholds"`
	Latency        LatencyThresholdConfig   `mapstructure:"latency"`
	ErrorRate      ErrorRateThresholdConfig `mapstructure:"errorRate"`
	RateLimit      RateLimitAlertConfig     `mapstructure:"rateLimit"`
}

// AlertRuleConfig defines a DSL rule for advanced alert detection.
type AlertRuleConfig struct {
	ID         string        `mapstructure:"id"`
	Expression string        `mapstructure:"expression"`
	Severity   string        `mapstructure:"severity"`
	Summary    string        `mapstructure:"summary"`
	Detail     string        `mapstructure:"detail"`
	Channels   []string      `mapstructure:"channels"`
	TTL        time.Duration `mapstructure:"ttl"`
}

// PoolThresholdConfig expresses utilization breakpoints in percent.
type PoolThresholdConfig struct {
	Warning  float64 `mapstructure:"warning"`
	Major    float64 `mapstructure:"major"`
	Critical float64 `mapstructure:"critical"`
}

// LatencyThresholdConfig captures acceptable request timings (ms).
type LatencyThresholdConfig struct {
	Major    float64 `mapstructure:"major"`
	Critical float64 `mapstructure:"critical"`
}

// ErrorRateThresholdConfig tracks acceptable failure percentages.
type ErrorRateThresholdConfig struct {
	Warning  float64 `mapstructure:"warning"`
	Major    float64 `mapstructure:"major"`
	Critical float64 `mapstructure:"critical"`
}

// RateLimitAlertConfig tracks guard throttling thresholds.
type RateLimitAlertConfig struct {
	Window   time.Duration `mapstructure:"window"`
	Warning  int           `mapstructure:"warning"`
	Major    int           `mapstructure:"major"`
	Critical int           `mapstructure:"critical"`
}

// AutomationConfig controls the background automation scheduler.
type AutomationConfig struct {
	Enabled   bool                      `mapstructure:"enabled"`
	Scheduler AutomationSchedulerConfig `mapstructure:"scheduler"`
	Jobs      AutomationJobsConfig      `mapstructure:"jobs"`
}

// AutomationSchedulerConfig configures the base scheduler knobs.
type AutomationSchedulerConfig struct {
	QueueSize      int           `mapstructure:"queueSize"`
	WorkerCount    int           `mapstructure:"workerCount"`
	MaxAttempts    int           `mapstructure:"maxAttempts"`
	DefaultTimeout time.Duration `mapstructure:"defaultTimeout"`
}

// AutomationJobsConfig enumerates supported recurring jobs.
type AutomationJobsConfig struct {
	Inventory          AutomationJobConfig `mapstructure:"inventory"`
	PolicyAudit        AutomationJobConfig `mapstructure:"policyAudit"`
	SecurityScan       AutomationJobConfig `mapstructure:"securityScan"`
	Analytics          AutomationJobConfig `mapstructure:"analytics"`
	NotificationFanout AutomationJobConfig `mapstructure:"notificationFanout"`
}

// AutomationJobConfig describes a single recurring job schedule.
type AutomationJobConfig struct {
	Enabled      bool                   `mapstructure:"enabled"`
	Interval     time.Duration          `mapstructure:"interval"`
	InitialDelay time.Duration          `mapstructure:"initialDelay"`
	TenantID     string                 `mapstructure:"tenantId"`
	Labels       map[string]string      `mapstructure:"labels"`
	Payload      map[string]interface{} `mapstructure:"payload"`
	Channels     []string               `mapstructure:"channels"`
}

// NotificationsConfig wires outbound notification channels.
type NotificationsConfig struct {
	Enabled        bool                           `mapstructure:"enabled"`
	DefaultTimeout time.Duration                  `mapstructure:"defaultTimeout"`
	Channels       []NotificationChannelConfig    `mapstructure:"channels"`
	Integrations   NotificationIntegrationsConfig `mapstructure:"integrations"`
}

// NotificationChannelConfig captures outbound channel metadata.
type NotificationChannelConfig struct {
	Name     string            `mapstructure:"name"`
	Type     string            `mapstructure:"type"`
	Endpoint string            `mapstructure:"endpoint"`
	Method   string            `mapstructure:"method"`
	Secret   string            `mapstructure:"secret"`
	Headers  map[string]string `mapstructure:"headers"`
}

// NotificationIntegrationsConfig references key downstream systems.
type NotificationIntegrationsConfig struct {
	CMDB       NotificationIntegration `mapstructure:"cmdb"`
	Monitoring NotificationIntegration `mapstructure:"monitoring"`
}

// NotificationIntegration maps an integration to a channel.
type NotificationIntegration struct {
	Enabled bool   `mapstructure:"enabled"`
	Channel string `mapstructure:"channel"`
}

// OpsSupportConfig aggregates Ops & Support platform knobs.
type OpsSupportConfig struct {
	System       OpsSystemConfig       `mapstructure:"system"`
	ImportExport ImportExportConfig    `mapstructure:"importExport"`
	HelpCenter   HelpCenterConfig      `mapstructure:"helpCenter"`
	Support      ExternalSupportConfig `mapstructure:"support"`
	Scripts      ScriptRunnerConfig    `mapstructure:"scripts"`
}

// OpsSystemConfig governs core system management behaviors.
type OpsSystemConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	AllowConfigExport bool   `mapstructure:"allowConfigExport"`
	AllowConfigImport bool   `mapstructure:"allowConfigImport"`
	BackupLocation    string `mapstructure:"backupLocation"`
	MaintenanceWindow string `mapstructure:"maintenanceWindow"`
	Theme             string `mapstructure:"theme"`
	Locale            string `mapstructure:"locale"`
	MaintenanceMode   bool   `mapstructure:"maintenanceMode"`
	Announcement      string `mapstructure:"announcement"`
}

// ImportExportConfig tunes bulk data import/export pipelines.
type ImportExportConfig struct {
	Enabled     bool     `mapstructure:"enabled"`
	StoragePath string   `mapstructure:"storagePath"`
	ObjectStore string   `mapstructure:"objectStore"`
	Formats     []string `mapstructure:"formats"`
	MaxFileSize int64    `mapstructure:"maxFileSize"`
}

// HelpCenterConfig describes knowledge base metadata.
type HelpCenterConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	BaseURL           string        `mapstructure:"baseUrl"`
	ArticlesPath      string        `mapstructure:"articlesPath"`
	FAQPath           string        `mapstructure:"faqPath"`
	ReleaseNotesPath  string        `mapstructure:"releaseNotesPath"`
	FeaturedTopics    []string      `mapstructure:"featuredTopics"`
	ExternalSearchURL string        `mapstructure:"externalSearchUrl"`
	CacheTTL          time.Duration `mapstructure:"cacheTtl"`
}

// ExternalSupportConfig references ticketing / escalation paths.
type ExternalSupportConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	TicketURL        string   `mapstructure:"ticketUrl"`
	ChatURL          string   `mapstructure:"chatUrl"`
	Phone            string   `mapstructure:"phone"`
	EscalationPolicy string   `mapstructure:"escalationPolicy"`
	Contacts         []string `mapstructure:"contacts"`
}

// ScriptRunnerConfig controls approved automation scripts.
type ScriptRunnerConfig struct {
	Enabled        bool                     `mapstructure:"enabled"`
	DefaultTimeout time.Duration            `mapstructure:"defaultTimeout"`
	MaxConcurrent  int                      `mapstructure:"maxConcurrent"`
	SandboxImage   string                   `mapstructure:"sandboxImage"`
	Catalog        []ScriptDescriptorConfig `mapstructure:"catalog"`
}

// ScriptDescriptorConfig enumerates an approved script entry.
type ScriptDescriptorConfig struct {
	Name             string   `mapstructure:"name"`
	Description      string   `mapstructure:"description"`
	Command          string   `mapstructure:"command"`
	Args             []string `mapstructure:"args"`
	AllowedRoles     []string `mapstructure:"allowedRoles"`
	RequiresApproval bool     `mapstructure:"requiresApproval"`
}

// Load loads configuration from the provided file and environment overrides.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		if dc.DecodeHook == nil {
			dc.DecodeHook = mapstructure.DecodeHookFunc(flattenStringMapDecodeHook)
			return
		}
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.DecodeHookFunc(flattenStringMapDecodeHook),
			dc.DecodeHook,
		)
	}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func flattenStringMapDecodeHook(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
	target := reflect.TypeOf(map[string]string{})
	if to != target {
		return data, nil
	}

	src, ok := data.(map[string]interface{})
	if !ok {
		return data, nil
	}

	flat := make(map[string]string, len(src))
	if err := flattenNestedStringMap("", src, flat); err != nil {
		return nil, err
	}
	return flat, nil
}

func flattenNestedStringMap(prefix string, in map[string]interface{}, out map[string]string) error {
	for k, raw := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch v := raw.(type) {
		case string:
			out[key] = v
		case map[string]interface{}:
			if err := flattenNestedStringMap(key, v, out); err != nil {
				return err
			}
		default:
			return fmt.Errorf("expected string value for key %s, got %T", key, raw)
		}
	}
	return nil
}
