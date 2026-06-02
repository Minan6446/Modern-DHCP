package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/ops"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

const (
	clusterConfigKeyHA                      = "cluster_ha_config"
	clusterConfigKeyHaLb                    = "cluster_ha_lb_config"
	clusterConfigKeySync                    = "cluster_sync_config"
	clusterConfigKeySyncTransactions        = "cluster_sync_transactions"
	clusterConfigKeySyncTransactionsArchive = "cluster_sync_transactions_archive"
	clusterConfigKeyScale                   = "cluster_scale_policy"
	clusterConfigKeyBackupPlan              = "cluster_backup_plan"
	clusterConfigKeyBackupHistory           = "cluster_backup_history"
	clusterConfigKeyOptimizationPlan        = "cluster_optimization_plan"
	clusterConfigKeyNodes                   = "cluster_nodes"
	clusterConfigKeyNodeAuth                = "cluster_node_auth"
	clusterConfigKeyControlPlane            = "cluster_control_plane"
	clusterConfigKeyControlMembers          = "cluster_control_members"
	clusterConfigKeyCommands                = "cluster_commands"
	clusterConfigKeyJoinJobs                = "cluster_join_jobs"
	clusterNodeCurrentVersion               = "v2.2.0"
	defaultSyncAckCriticalMs                = 2000
	defaultSyncAckWarnMs                    = 1000
	defaultSyncStorageLagCriticalMs         = 500
	defaultSyncStorageLagWarnMs             = 250
)

type clusterOverviewResponse struct {
	Mode                 string                `json:"mode"`
	DHCPRole             string                `json:"dhcpRole"`
	DHCPHealth           string                `json:"dhcpHealth"`
	FailoverReady        bool                  `json:"failoverReady"`
	ReplicationLagMs     int                   `json:"replicationLagMs"`
	ReplicationOK        bool                  `json:"replicationOk,omitempty"`
	ReplicationHealth    string                `json:"replicationHealth,omitempty"`
	ReplicationError     string                `json:"replicationError,omitempty"`
	ReplicationTx        string                `json:"replicationTx,omitempty"`
	ReplicationMode      string                `json:"replicationMode,omitempty"`
	ReplicationSource    string                `json:"replicationSource,omitempty"`
	ReplicationState     string                `json:"replicationState,omitempty"`
	ReplicationUpdatedAt string                `json:"replicationUpdatedAt,omitempty"`
	StorageRoleAligned   bool                  `json:"storageRoleAligned"`
	MySQLRole            string                `json:"mysqlRole"`
	MySQLHealth          string                `json:"mysqlHealth"`
	MySQLLagMs           int                   `json:"mysqlLagMs"`
	RedisRole            string                `json:"redisRole"`
	RedisHealth          string                `json:"redisHealth"`
	RedisOffsetLag       int64                 `json:"redisOffsetLag"`
	FencingEpoch         string                `json:"fencingEpoch"`
	WriteGateOpen        bool                  `json:"writeGateOpen"`
	Nodes                []clusterOverviewNode `json:"nodes"`
	PendingActions       int                   `json:"pendingActions"`
	JoinPending          int                   `json:"joinPending"`
	JoinFailed           int                   `json:"joinFailed"`
	JoinCompleted        int                   `json:"joinCompleted"`
}

type clusterOverviewNode struct {
	ID            string  `json:"id"`
	NodeType      string  `json:"nodeType,omitempty"`
	NodeState     string  `json:"nodeState,omitempty"`
	Role          string  `json:"role"`
	Address       string  `json:"address"`
	Self          bool    `json:"self,omitempty"`
	Managed       bool    `json:"managed,omitempty"`
	HasAuth       bool    `json:"hasAuth"`
	Version       string  `json:"version"`
	Health        string  `json:"health"`
	Disabled      bool    `json:"disabled"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
	ActiveLeases  int64   `json:"activeLeasesRedis"`
	TotalLeases   int64   `json:"totalLeasesMySQL"`
	SyncLagMs     int     `json:"syncLagMs"`
	LastHeartbeat string  `json:"lastHeartbeat"`
}

type clusterHaConfigDTO struct {
	Mode          string                 `json:"mode"`
	Primary       string                 `json:"primary"`
	StandbyNodes  []string               `json:"standbyNodes"`
	LoadBalancing clusterHaLoadBalancing `json:"loadBalancing"`
	Failover      clusterHaFailover      `json:"failover"`
	Replication   clusterHaReplication   `json:"replication"`
	SplitBrain    clusterHaSplitBrain    `json:"splitBrain"`
	Recovery      clusterHaRecovery      `json:"recovery"`
}

type clusterHaLoadBalancing struct {
	Enabled   bool   `json:"enabled"`
	VirtualIP string `json:"virtualIp,omitempty"`
	Balancer  string `json:"balancer,omitempty"`
	Method    string `json:"method,omitempty"`
}

type clusterHaFailover struct {
	IntervalMs       int `json:"intervalMs"`
	TimeoutMs        int `json:"timeoutMs"`
	FailureThreshold int `json:"failureThreshold"`
}

type clusterHaReplication struct {
	Mechanism           string `json:"mechanism"`
	SnapshotIntervalSec int    `json:"snapshotIntervalSec"`
	LagAlertMs          int    `json:"lagAlertMs"`
	SyncMode            string `json:"syncMode"`
}

type clusterHaSplitBrain struct {
	Strategy    string `json:"strategy"`
	Arbiter     string `json:"arbiter,omitempty"`
	FenceScript string `json:"fenceScript,omitempty"`
	SharedLock  string `json:"sharedLock,omitempty"`
}

type clusterHaRecovery struct {
	ManualAction string `json:"manualAction,omitempty"`
}

type clusterHaLbConfigDTO struct {
	Mode          string                 `json:"mode"`
	HeartbeatSec  int                    `json:"heartbeatSec"`
	FailThreshold int                    `json:"failThreshold"`
	SplitBrain    string                 `json:"splitBrain"`
	AutoSwitch    bool                   `json:"autoSwitch"`
	Algorithm     string                 `json:"algorithm"`
	AutoBalance   bool                   `json:"autoBalance"`
	Weights       []clusterHaLbWeightDTO `json:"weights"`
}

type clusterHaLbWeightDTO struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Weight int    `json:"weight"`
}

type clusterFailoverEvent struct {
	ID          string `json:"id"`
	Time        string `json:"time"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	StatusLabel string `json:"statusLabel"`
	TxID        string `json:"txId,omitempty"`
	Phase       string `json:"phase,omitempty"`
	AckAt       string `json:"ackAt,omitempty"`
	ErrorCode   string `json:"errorCode,omitempty"`
}

type clusterSyncConfigDTO struct {
	Mode           string `json:"mode"`
	Transport      string `json:"transport,omitempty"`
	LeaseFreq      string `json:"leaseFreq"`
	ConfigFreq     string `json:"configFreq"`
	ConflictPolicy string `json:"conflictPolicy"`
}

type clusterScalePolicyDTO struct {
	CPU      int `json:"cpu"`
	Mem      int `json:"mem"`
	QPS      int `json:"qps"`
	MaxNodes int `json:"maxNodes"`
}

type clusterSyncStatusDTO struct {
	Type                    string `json:"type"`
	Latency                 string `json:"latency"`
	Last                    string `json:"last"`
	Health                  string `json:"health"`
	MySQLLagMs              int    `json:"mysqlLagMs,omitempty"`
	MySQLLagLevel           string `json:"mysqlLagLevel,omitempty"`
	MySQLLagWarnMs          int    `json:"mysqlLagWarnMs,omitempty"`
	MySQLLagCriticalMs      int    `json:"mysqlLagCriticalMs,omitempty"`
	RedisOffsetLag          int64  `json:"redisOffsetLag,omitempty"`
	RedisOffsetLagLevel     string `json:"redisOffsetLagLevel,omitempty"`
	RedisOffsetLagWarn      int64  `json:"redisOffsetLagWarn,omitempty"`
	RedisOffsetLagCritical  int64  `json:"redisOffsetLagCritical,omitempty"`
	Rfc6853AckLatencyMs     int    `json:"rfc6853AckLatencyMs,omitempty"`
	Rfc6853AckLatencyLevel  string `json:"rfc6853AckLatencyLevel,omitempty"`
	Rfc6853AckWarnMs        int    `json:"rfc6853AckWarnMs,omitempty"`
	Rfc6853AckCriticalMs    int    `json:"rfc6853AckCriticalMs,omitempty"`
	Transport               string `json:"transport,omitempty"`
	PeerAddress             string `json:"peerAddress,omitempty"`
	ResponderUp             bool   `json:"responderUp,omitempty"`
	ResponderErr            string `json:"responderErr,omitempty"`
	LastTxID                string `json:"lastTxId,omitempty"`
	Phase                   string `json:"phase,omitempty"`
	SourceNode              string `json:"sourceNode,omitempty"`
	TargetNode              string `json:"targetNode,omitempty"`
	BndupdAt                string `json:"bndupdAt,omitempty"`
	BndackAt                string `json:"bndackAt,omitempty"`
	AckAt                   string `json:"ackAt,omitempty"`
	ErrorCode               string `json:"errorCode,omitempty"`
	ErrorMessage            string `json:"errorMessage,omitempty"`
	ReconcileLastRun        string `json:"reconcileLastRun,omitempty"`
	ReconcileLastErr        string `json:"reconcileLastErr,omitempty"`
	ReconcileSuccessTotal   int    `json:"reconcileSuccessTotal,omitempty"`
	ReconcileFailureTotal   int    `json:"reconcileFailureTotal,omitempty"`
	ReconcileFailureStreak  int    `json:"reconcileFailureStreak,omitempty"`
	ReconcileIntervalSec    int    `json:"reconcileIntervalSec,omitempty"`
	ReconcileAlertThreshold int    `json:"reconcileAlertThreshold,omitempty"`
}

type clusterSyncStatsDTO struct {
	WindowHours        int     `json:"windowHours"`
	Total              int     `json:"total"`
	Committed          int     `json:"committed"`
	Failed             int     `json:"failed"`
	AckTimeoutCount    int     `json:"ackTimeoutCount"`
	SuccessRatePercent float64 `json:"successRatePercent"`
	AvgAckLatencyMs    int     `json:"avgAckLatencyMs"`
	P95AckLatencyMs    int     `json:"p95AckLatencyMs"`
}

type clusterSyncTransactionDTO struct {
	TxID          string `json:"txId"`
	Type          string `json:"type"`
	SourceNode    string `json:"sourceNode"`
	TargetNode    string `json:"targetNode"`
	RetryOf       string `json:"retryOf,omitempty"`
	RetryStrategy string `json:"retryStrategy,omitempty"`
	Phase         string `json:"phase"`
	CreatedAt     string `json:"createdAt"`
	BndupdAt      string `json:"bndupdAt,omitempty"`
	BndackAt      string `json:"bndackAt,omitempty"`
	AckAt         string `json:"ackAt,omitempty"`
	CommitAt      string `json:"commitAt,omitempty"`
	LatencyMs     int    `json:"latencyMs,omitempty"`
	ErrorCode     string `json:"errorCode,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
}

type clusterSyncTriggerRequest struct {
	Type            string `json:"type"`
	SourceNode      string `json:"sourceNode"`
	TargetNode      string `json:"targetNode"`
	SimulateFailure bool   `json:"simulateFailure"`
}

type clusterSyncRetryRequest struct {
	Strategy        string `json:"strategy"`
	SimulateFailure bool   `json:"simulateFailure"`
}

const (
	failoverPlanStatusPending    = "PENDING"
	failoverPlanStatusRunning    = "RUNNING"
	failoverPlanStatusSuccess    = "SUCCESS"
	failoverPlanStatusFailed     = "FAILED"
	failoverPlanStatusRolledBack = "ROLLED_BACK"

	failoverPlanStepPending = "PENDING"
	failoverPlanStepRunning = "RUNNING"
	failoverPlanStepSuccess = "SUCCESS"
	failoverPlanStepFailed  = "FAILED"
	failoverPlanStepSkipped = "SKIPPED"
)

type clusterFailoverPlanCreateRequest struct {
	Reason              string `json:"reason"`
	SourceNode          string `json:"sourceNode,omitempty"`
	TargetNode          string `json:"targetNode,omitempty"`
	DryRun              bool   `json:"dryRun"`
	SimulateFailureStep string `json:"simulateFailureStep,omitempty"`
}

type clusterFailoverPlanStepDTO struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
	Rollback   bool   `json:"rollback,omitempty"`
}

type clusterFailoverPlanDTO struct {
	PlanID            string                       `json:"planId"`
	Status            string                       `json:"status"`
	Reason            string                       `json:"reason"`
	DryRun            bool                         `json:"dryRun"`
	SourceNode        string                       `json:"sourceNode,omitempty"`
	TargetNode        string                       `json:"targetNode,omitempty"`
	CreatedAt         string                       `json:"createdAt"`
	UpdatedAt         string                       `json:"updatedAt"`
	FinishedAt        string                       `json:"finishedAt,omitempty"`
	FailedStep        string                       `json:"failedStep,omitempty"`
	RollbackTriggered bool                         `json:"rollbackTriggered"`
	RollbackStatus    string                       `json:"rollbackStatus,omitempty"`
	Steps             []clusterFailoverPlanStepDTO `json:"steps"`
}

type clusterBackupPlanDTO struct {
	Freq     string   `json:"freq"`
	Retain   string   `json:"retain"`
	Storage  string   `json:"storage"`
	Contents []string `json:"contents"`
}

type clusterOptimizationPlanDTO struct {
	IpDefrag     bool `json:"ipDefrag"`
	LeaseTune    bool `json:"leaseTune"`
	PoolForecast bool `json:"poolForecast"`
}

type clusterBackupRecordDTO struct {
	Time      string `json:"time"`
	Type      string `json:"type"`
	Size      string `json:"size"`
	Status    string `json:"status"`
	TxID      string `json:"txId,omitempty"`
	Phase     string `json:"phase,omitempty"`
	AckAt     string `json:"ackAt,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

type clusterBackupRestoreRequest struct {
	Time string `json:"time"`
}

type clusterNodePayload struct {
	ID            string                `json:"id"`
	Address       string                `json:"address"`
	Role          string                `json:"role"`
	Auth          string                `json:"auth,omitempty"`
	Disabled      *bool                 `json:"disabled,omitempty"`
	Version       string                `json:"version"`
	Health        string                `json:"health"`
	CPUPercent    float64               `json:"cpuPercent"`
	MemoryPercent float64               `json:"memoryPercent"`
	SyncLagMs     int                   `json:"syncLagMs"`
	HAConfig      *clusterHaConfigDTO   `json:"haConfig,omitempty"`
	HaLbConfig    *clusterHaLbConfigDTO `json:"haLbConfig,omitempty"`
	SyncConfig    *clusterSyncConfigDTO `json:"syncConfig,omitempty"`
}

type securityOverviewResponse struct {
	RateLimitHits      int                    `json:"rateLimitHits"`
	SnoopingViolations int                    `json:"snoopingViolations"`
	RogueServers       int                    `json:"rogueServers"`
	Controls           []securityControlState `json:"controls"`
	RecentFindings     []securityAlertEntry   `json:"recentFindings"`
}

type securityControlState struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Severity    string `json:"severity,omitempty"`
	LastEventAt string `json:"lastEventAt,omitempty"`
}

type securityAlertEntry struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	Details     string   `json:"details"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Lifecycle   string   `json:"lifecycle"`
	Source      string   `json:"source"`
	TenantID    string   `json:"tenantId"`
	Assignee    string   `json:"assignee,omitempty"`
	Channel     string   `json:"channel,omitempty"`
	Fingerprint string   `json:"fingerprint"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

type dhcpOptionDefinitionResponse struct {
	Name        string `json:"name"`
	Code        int    `json:"code"`
	Category    string `json:"category"`
	Description string `json:"description"`
	UsageCount  int    `json:"usageCount,omitempty"`
}

type dhcpOptionTemplateSummary struct {
	Name        string `json:"name"`
	Options     int    `json:"options"`
	UsedBy      int    `json:"usedBy"`
	Description string `json:"description"`
}

type dhcpOptionCatalogResponse struct {
	Standard  []dhcpOptionDefinitionResponse `json:"standard"`
	Custom    []dhcpOptionDefinitionResponse `json:"custom"`
	Templates []dhcpOptionTemplateSummary    `json:"templates"`
}

type maintenanceOverviewResponse struct {
	BackupsEnabled bool                       `json:"backupsEnabled"`
	BackupWindow   string                     `json:"backupWindow"`
	LastBackupAt   string                     `json:"lastBackupAt"`
	Tasks          []maintenanceTaskResponse  `json:"tasks"`
	Forecasts      []capacityForecastResponse `json:"forecasts"`
}

type maintenanceTaskResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Owner  string `json:"owner"`
	Window string `json:"window"`
	Status string `json:"status"`
}

type capacityForecastResponse struct {
	Metric        string `json:"metric"`
	Current       int    `json:"current"`
	Limit         int    `json:"limit"`
	Projection30d int    `json:"projection30d"`
}

type supportResourceResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updatedAt"`
	Link      string `json:"link"`
}

type helpCenterSnapshotResponse struct {
	Docs              []supportResourceResponse `json:"docs"`
	OpenTickets       int                       `json:"openTickets"`
	LatestVersion     string                    `json:"latestVersion"`
	ReleaseHighlights []string                  `json:"releaseHighlights"`
}

type integrationOverviewResponse struct {
	Adapters             []integrationAdapterResponse `json:"adapters"`
	WebhookDeliveries24h int                          `json:"webhookDeliveries24h"`
	APICalls24h          int                          `json:"apiCalls24h"`
}

type integrationAdapterResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Endpoint   string `json:"endpoint"`
	LastSyncAt string `json:"lastSyncAt,omitempty"`
}

func (s *HTTPServer) handleClusterOverview() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		overview := clusterOverviewResponse{
			Mode:           "active-passive",
			Nodes:          []clusterOverviewNode{},
			PendingActions: 0,
		}

		svc := s.ensureHAService()
		var snapshot failover.StatusSnapshot
		var nodes []failover.NodeStatus
		if svc != nil {
			snapshot = svc.Snapshot()
			nodes = svc.Nodes(ctx)
			if mode := normalizeClusterMode(svc.Mode()); mode != "" {
				overview.Mode = mode
			}
		} else if s.options.Coordinator != nil {
			snapshot = s.options.Coordinator.Snapshot()
		}

		if overview.Mode == "" {
			overview.Mode = deriveClusterMode(nodes, snapshot)
		}

		overview.FailoverReady = clusterFailoverReady(snapshot)
		overview.ReplicationLagMs = replicationLagMillis(snapshot.PeerLastSeen)
		if snapshot.Replication.ApplyLagMs > 0 {
			overview.ReplicationLagMs = snapshot.Replication.ApplyLagMs
		}
		overview.WriteGateOpen = clusterWriteGateOpen(snapshot.Role)
		overview.FencingEpoch = clusterFencingEpoch(snapshot)
		overview.ReplicationOK = snapshot.Replication.Healthy
		overview.ReplicationHealth = clusterReplicationHealth(snapshot.Replication)
		overview.ReplicationError = strings.TrimSpace(snapshot.Replication.LastError)
		overview.ReplicationTx = strings.TrimSpace(snapshot.Replication.LastOffset)
		overview.ReplicationMode = strings.TrimSpace(snapshot.Replication.Mode)
		overview.ReplicationSource = strings.TrimSpace(snapshot.Replication.Source)
		overview.ReplicationState = strings.TrimSpace(snapshot.Replication.State)
		if !snapshot.Replication.UpdatedAt.IsZero() {
			overview.ReplicationUpdatedAt = snapshot.Replication.UpdatedAt.UTC().Format(time.RFC3339)
		}

		var health *monitoring.SystemHealthSnapshot
		if s.monitor != nil {
			current := s.monitor.Health(ctx)
			health = &current
		}

		activeLeases, totalLeases := s.clusterLeaseCounters(ctx, s.leaseScopeRef(c))

		overview.Nodes = buildClusterNodes(nodes, snapshot, health)
		overview.Nodes = s.mergeClusterNodes(overview.Nodes)
		overview.Nodes = s.mergeClusterControlMembers(overview.Nodes)
		overview.Nodes = applyClusterLeaseCounters(overview.Nodes, activeLeases, totalLeases)
		overview.DHCPRole = clusterDHCPRole(snapshot.Role)
		overview.DHCPHealth = clusterDHCPHealth(snapshot.State)
		overview.MySQLRole = normalizeReportedServiceRole(snapshot.Replication.MySQLRole)
		overview.MySQLHealth = clusterMySQLHealth(snapshot.Replication)
		overview.MySQLLagMs = overview.ReplicationLagMs
		overview.RedisRole = normalizeReportedServiceRole(snapshot.Replication.RedisRole)
		overview.RedisOffsetLag = int64(overview.ReplicationLagMs)
		overview.RedisHealth = clusterRedisHealth(snapshot.State, overview.Nodes)
		overview.StorageRoleAligned = clusterStorageRoleAligned(overview.DHCPRole, overview.MySQLRole, overview.RedisRole)
		joinPending, joinFailed, joinCompleted := s.joinJobCounters()
		overview.JoinPending = joinPending
		overview.JoinFailed = joinFailed
		overview.JoinCompleted = joinCompleted
		overview.PendingActions = countClusterPendingActions(snapshot, overview.Nodes)
		overview.PendingActions += joinPending + joinFailed

		return c.JSON(http.StatusOK, overview)
	}
}

func clusterReplicationHealth(telemetry failover.ReplicationTelemetry) string {
	if strings.TrimSpace(telemetry.LastError) != "" {
		return "error"
	}
	if telemetry.Healthy {
		return "healthy"
	}
	if telemetry.ApplyLagMs > 0 || strings.TrimSpace(telemetry.LastOffset) != "" || !telemetry.UpdatedAt.IsZero() {
		return "degraded"
	}
	return "unknown"
}

func clusterDHCPRole(role failover.Role) string {
	switch role {
	case failover.RolePrimary:
		return "primary"
	case failover.RoleStandby:
		return "standby"
	default:
		return "unknown"
	}
}

func resolveClusterDHCPRole(snapshotRole failover.Role, nodes []clusterOverviewNode) string {
	resolved := clusterDHCPRole(snapshotRole)
	if resolved != "unknown" {
		return resolved
	}

	enabledCount := 0
	activeCount := 0
	standbyCount := 0
	for _, node := range nodes {
		if node.Disabled {
			continue
		}
		enabledCount++
		nodeRole := strings.ToLower(strings.TrimSpace(node.Role))
		if nodeRole == "active" {
			activeCount++
		}
		if nodeRole == "standby" {
			standbyCount++
		}

		isSelf := node.Self || strings.EqualFold(strings.TrimSpace(node.ID), "self")
		if !isSelf {
			continue
		}
		switch nodeRole {
		case "active":
			return "primary"
		case "standby":
			return "standby"
		}
	}

	if enabledCount == 1 {
		if activeCount == 1 {
			return "primary"
		}
		if standbyCount == 1 {
			return "standby"
		}
	}

	return resolved
}

func resolveClusterDHCPHealth(snapshotState failover.State, snapshotRole failover.Role, nodes []clusterOverviewNode) string {
	base := clusterDHCPHealth(snapshotState)
	if snapshotRole != failover.RoleUnknown {
		return base
	}

	enabledCount := 0
	var single *clusterOverviewNode
	for i := range nodes {
		node := &nodes[i]
		if node.Disabled {
			continue
		}
		enabledCount++
		single = node
		isSelf := node.Self || strings.EqualFold(strings.TrimSpace(node.ID), "self")
		if isSelf {
			return normalizeClusterHealth(node.Health, base)
		}
	}

	if enabledCount == 1 && single != nil {
		return normalizeClusterHealth(single.Health, base)
	}

	return base
}

func normalizeClusterHealth(health string, fallback string) string {
	v := strings.ToLower(strings.TrimSpace(health))
	switch v {
	case "healthy", "normal", "ok", "正常", "健康":
		return "healthy"
	case "warning", "warn", "告警":
		return "warning"
	case "critical", "error", "异常", "严重":
		return "critical"
	default:
		return fallback
	}
}

func clusterDHCPHealth(state failover.State) string {
	switch state {
	case failover.StateNormal, failover.StateInit:
		return "healthy"
	case failover.StateCommInterrupted:
		return "warning"
	case failover.StatePartnerDown:
		return "critical"
	default:
		return "warning"
	}
}

func clusterWriteGateOpen(role failover.Role) bool {
	return role == failover.RolePrimary || role == failover.RoleUnknown
}

func clusterFencingEpoch(snapshot failover.StatusSnapshot) string {
	if v := strings.TrimSpace(snapshot.FencingEpoch); v != "" {
		return v
	}
	if v := strings.TrimSpace(snapshot.Replication.LastOffset); v != "" {
		return v
	}
	if !snapshot.Replication.UpdatedAt.IsZero() {
		return strconv.FormatInt(snapshot.Replication.UpdatedAt.UTC().UnixMilli(), 10)
	}
	return ""
}

func clusterStorageRoleForDHCPRole(dhcpRole string) string {
	switch strings.ToLower(strings.TrimSpace(dhcpRole)) {
	case "primary":
		return "primary"
	case "standby":
		return "replica"
	default:
		return "unknown"
	}
}

func normalizeReportedServiceRole(role string) string {
	v := strings.ToLower(strings.TrimSpace(role))
	switch v {
	case "primary", "active", "master", "leader":
		return "primary"
	case "replica", "standby", "secondary", "slave", "follower":
		return "replica"
	default:
		return "unknown"
	}
}

func clusterMySQLHealth(telemetry failover.ReplicationTelemetry) string {
	return clusterReplicationHealth(telemetry)
}

func clusterRedisHealth(state failover.State, nodes []clusterOverviewNode) string {
	for _, node := range nodes {
		if strings.EqualFold(strings.TrimSpace(node.Role), "active") {
			return firstNonEmpty(strings.TrimSpace(node.Health), clusterDHCPHealth(state))
		}
	}
	if len(nodes) > 0 {
		return firstNonEmpty(strings.TrimSpace(nodes[0].Health), clusterDHCPHealth(state))
	}
	return clusterDHCPHealth(state)
}

func clusterStorageRoleAligned(dhcpRole, mysqlRole, redisRole string) bool {
	expected := clusterStorageRoleForDHCPRole(dhcpRole)
	if expected == "unknown" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(mysqlRole), expected) && strings.EqualFold(strings.TrimSpace(redisRole), expected)
}

func (s *HTTPServer) mergeClusterNodes(base []clusterOverviewNode) []clusterOverviewNode {
	custom := s.customClusterNodes()
	if len(custom) == 0 {
		for i := range base {
			base[i].Managed = false
			base[i].HasAuth = s.nodeAuthConfigured(base[i].ID)
		}
		return base
	}
	index := make(map[string]int, len(base))
	for i, node := range base {
		if node.ID != "" {
			index[node.ID] = i
		}
	}
	for _, node := range custom {
		if idx, ok := index[node.ID]; ok {
			merged := base[idx]
			if strings.TrimSpace(node.Role) != "" {
				merged.Role = node.Role
			}
			if strings.TrimSpace(node.Address) != "" {
				merged.Address = node.Address
			}
			if strings.TrimSpace(node.Version) != "" {
				merged.Version = node.Version
			}
			if strings.TrimSpace(node.Health) != "" {
				merged.Health = node.Health
			}
			merged.Disabled = node.Disabled
			merged.Managed = true
			base[idx] = merged
			continue
		}
		base = append(base, node)
	}
	for i := range base {
		base[i].HasAuth = s.nodeAuthConfigured(base[i].ID)
	}
	return base
}

func (s *HTTPServer) clusterLeaseCounters(ctx context.Context, scope lease.ResourceScope) (int64, int64) {
	if s == nil || s.leaseSvc == nil {
		return 0, 0
	}
	logger := LoggerFromContext(ctx, s.logger)
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	active, err := s.leaseSvc.CountActiveLeases(ctx, scope)
	if err != nil {
		if logger != nil {
			logger.Warn("cluster overview active lease count failed", zap.Error(err))
		}
		active = 0
	}
	_, historyTotal, err := s.leaseSvc.History(ctx, scope, models.LeaseHistoryFilter{Limit: 1, Offset: 0})
	if err != nil {
		if logger != nil {
			logger.Warn("cluster overview lease history count failed", zap.Error(err))
		}
		historyTotal = 0
	}
	activeCount := int64(active)
	if activeCount < 0 {
		activeCount = 0
	}
	totalCount := activeCount + int64(historyTotal)
	if totalCount < activeCount {
		totalCount = activeCount
	}
	return activeCount, totalCount
}

func applyClusterLeaseCounters(nodes []clusterOverviewNode, activeLeases, totalLeases int64) []clusterOverviewNode {
	if len(nodes) == 0 {
		return nodes
	}
	idx := -1
	for i := range nodes {
		if strings.EqualFold(strings.TrimSpace(nodes[i].ID), "self") {
			idx = i
			break
		}
	}
	if idx < 0 {
		for i := range nodes {
			if strings.EqualFold(strings.TrimSpace(nodes[i].Role), "active") {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		idx = 0
	}
	nodes[idx].ActiveLeases = maxInt64Value(activeLeases, 0)
	nodes[idx].TotalLeases = maxInt64Value(totalLeases, nodes[idx].ActiveLeases)
	return nodes
}

func maxInt64Value(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *HTTPServer) customClusterNodes() []clusterOverviewNode {
	s.clusterNodesMu.RLock()
	defer s.clusterNodesMu.RUnlock()
	result := make([]clusterOverviewNode, 0, len(s.clusterNodes))
	for _, node := range s.clusterNodes {
		cloned := node
		cloned.Version = normalizeClusterNodeVersion(cloned.Version)
		cloned.Managed = true
		cloned.HasAuth = s.nodeAuthConfigured(cloned.ID)
		if cloned.LastHeartbeat == "" {
			cloned.LastHeartbeat = time.Now().UTC().Format(time.RFC3339)
		}
		result = append(result, cloned)
	}
	return result
}

func toAuditMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func (s *HTTPServer) persistClusterConfig(ctx context.Context, key string, value any) {
	if s == nil || s.configStore == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := LoggerFromContext(ctx, s.logger)
	if err := s.configStore.Save(ctx, key, value); err != nil && logger != nil {
		logger.Warn("persist cluster config", zap.String("key", key), zap.Error(err))
		return
	}
	actor := strings.TrimSpace(GetUserID(ctx))
	if actor == "" {
		actor = "system"
	}
	tenantID := strings.TrimSpace(GetTenantID(ctx))
	if tenantID == "" {
		tenantID = systemTenantID
	}
	change := auditpayload.ConfigChange{
		Resource:   "cluster_config",
		Action:     "update",
		Identifier: key,
		After:      toAuditMap(value),
	}
	s.recordAudit(ctx, tenantID, actor, "config.update", change, withResource("cluster_config"))
}

func (s *HTTPServer) persistClusterConfigNoAudit(ctx context.Context, key string, value any) {
	if s == nil || s.configStore == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := LoggerFromContext(ctx, s.logger)
	if err := s.configStore.Save(ctx, key, value); err != nil && logger != nil {
		logger.Warn("persist cluster config", zap.String("key", key), zap.Error(err))
	}
}

func (s *HTTPServer) persistClusterConfigNoAuditErr(ctx context.Context, key string, value any) error {
	if s == nil || s.configStore == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := LoggerFromContext(ctx, s.logger)
	if err := s.configStore.Save(ctx, key, value); err != nil {
		if logger != nil {
			logger.Warn("persist cluster config", zap.String("key", key), zap.Error(err))
		}
		return err
	}
	return nil
}

func (s *HTTPServer) restoreClusterConfigs(ctx context.Context) {
	if s == nil || s.configStore == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := LoggerFromContext(ctx, s.logger)
	var haCfg clusterHaConfigDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyHA, &haCfg); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyHA), zap.Error(err))
		}
	} else if ok {
		s.setHAConfig(s.fromClusterHaConfigDTO(haCfg))
	}
	var haLb clusterHaLbConfigDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyHaLb, &haLb); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyHaLb), zap.Error(err))
		}
	} else if ok {
		s.setHaLbConfig(normalizeHaLbConfigPayload(haLb))
	}
	var syncCfg clusterSyncConfigDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeySync, &syncCfg); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeySync), zap.Error(err))
		}
	} else if ok {
		s.setSyncConfig(normalizeSyncConfigPayload(syncCfg))
	}
	var scaleCfg clusterScalePolicyDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyScale, &scaleCfg); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyScale), zap.Error(err))
		}
	} else if ok {
		s.setScalePolicy(normalizeScalePolicyPayload(scaleCfg))
	}
	var backupPlan clusterBackupPlanDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyBackupPlan, &backupPlan); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyBackupPlan), zap.Error(err))
		}
	} else if ok {
		s.setBackupPlan(normalizeBackupPlanPayload(backupPlan))
	}
	var backupHistory []clusterBackupRecordDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyBackupHistory, &backupHistory); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyBackupHistory), zap.Error(err))
		}
	} else if ok {
		s.restoreBackupHistory(backupHistory)
	}
	var optPlan clusterOptimizationPlanDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyOptimizationPlan, &optPlan); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyOptimizationPlan), zap.Error(err))
		}
	} else if ok {
		s.setOptimizationPlan(optPlan)
	}
	var nodes []clusterOverviewNode
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyNodes, &nodes); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyNodes), zap.Error(err))
		}
	} else if ok {
		s.restoreClusterNodes(nodes)
	}
	var nodeAuth map[string]string
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyNodeAuth, &nodeAuth); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyNodeAuth), zap.Error(err))
		}
	} else if ok {
		s.restoreNodeAuth(nodeAuth)
	}
	var txs []clusterSyncTransactionDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeySyncTransactions, &txs); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeySyncTransactions), zap.Error(err))
		}
	} else if ok {
		s.restoreSyncTransactions(txs)
	}
	var archivedTxs []clusterSyncTransactionDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeySyncTransactionsArchive, &archivedTxs); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeySyncTransactionsArchive), zap.Error(err))
		}
	} else if ok {
		s.restoreSyncTransactionsArchive(archivedTxs)
	}
	var controlPlane clusterControlPlaneDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyControlPlane, &controlPlane); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyControlPlane), zap.Error(err))
		}
	} else if ok {
		s.restoreClusterControlPlane(controlPlane)
	}
	var controlMembers []clusterControlMemberDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyControlMembers, &controlMembers); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyControlMembers), zap.Error(err))
		}
	} else if ok {
		s.restoreClusterControlMembers(controlMembers)
	}
	var commands []clusterCommandDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyCommands, &commands); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyCommands), zap.Error(err))
		}
	} else if ok {
		s.restoreClusterCommands(commands)
	}
	var joinJobs []clusterJoinJobDTO
	if ok, err := s.configStore.Load(ctx, clusterConfigKeyJoinJobs, &joinJobs); err != nil {
		if logger != nil {
			logger.Warn("load cluster config", zap.String("key", clusterConfigKeyJoinJobs), zap.Error(err))
		}
	} else if ok {
		s.restoreJoinJobs(joinJobs)
	}
}

func (s *HTTPServer) persistClusterNodes(ctx context.Context) {
	nodes := s.customClusterNodes()
	s.persistClusterConfig(ctx, clusterConfigKeyNodes, nodes)
}

func (s *HTTPServer) persistBackupHistory(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.backupHistoryMu.RLock()
	records := append([]clusterBackupRecordDTO{}, s.backupHistory...)
	s.backupHistoryMu.RUnlock()
	s.persistClusterConfig(ctx, clusterConfigKeyBackupHistory, records)
}

func (s *HTTPServer) restoreBackupHistory(records []clusterBackupRecordDTO) {
	if s == nil {
		return
	}
	if len(records) == 0 {
		return
	}
	clean := make([]clusterBackupRecordDTO, 0, len(records))
	for _, item := range records {
		if strings.TrimSpace(item.Time) == "" {
			continue
		}
		clean = append(clean, item)
	}
	s.backupHistoryMu.Lock()
	s.backupHistory = clean
	s.backupHistoryMu.Unlock()
}

func (s *HTTPServer) restoreClusterNodes(nodes []clusterOverviewNode) {
	s.clusterNodesMu.Lock()
	defer s.clusterNodesMu.Unlock()
	if s.clusterNodes == nil {
		s.clusterNodes = make(map[string]clusterOverviewNode)
	}
	s.clusterNodes = make(map[string]clusterOverviewNode, len(nodes))
	for _, node := range nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		node.ID = id
		node.Version = normalizeClusterNodeVersion(node.Version)
		node.HasAuth = s.nodeAuthConfigured(id)
		s.clusterNodes[id] = node
	}
}

func (s *HTTPServer) nodeAuthConfigured(nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || s == nil {
		return false
	}
	s.clusterNodeAuthMu.RLock()
	defer s.clusterNodeAuthMu.RUnlock()
	if s.clusterNodeAuth == nil {
		return false
	}
	_, ok := s.clusterNodeAuth[nodeID]
	return ok
}

func (s *HTTPServer) upsertNodeAuth(nodeID, authValue string) bool {
	nodeID = strings.TrimSpace(nodeID)
	authValue = strings.TrimSpace(authValue)
	if nodeID == "" || authValue == "" || s == nil {
		return false
	}
	s.clusterNodeAuthMu.Lock()
	defer s.clusterNodeAuthMu.Unlock()
	if s.clusterNodeAuth == nil {
		s.clusterNodeAuth = make(map[string]string)
	}
	s.clusterNodeAuth[nodeID] = authValue
	return true
}

func (s *HTTPServer) deleteNodeAuth(nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || s == nil {
		return false
	}
	s.clusterNodeAuthMu.Lock()
	defer s.clusterNodeAuthMu.Unlock()
	if s.clusterNodeAuth == nil {
		return false
	}
	if _, ok := s.clusterNodeAuth[nodeID]; !ok {
		return false
	}
	delete(s.clusterNodeAuth, nodeID)
	return true
}

func (s *HTTPServer) snapshotNodeAuth() map[string]string {
	if s == nil {
		return nil
	}
	s.clusterNodeAuthMu.RLock()
	defer s.clusterNodeAuthMu.RUnlock()
	if len(s.clusterNodeAuth) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(s.clusterNodeAuth))
	for k, v := range s.clusterNodeAuth {
		result[k] = v
	}
	return result
}

func (s *HTTPServer) restoreNodeAuth(values map[string]string) {
	if s == nil {
		return
	}
	s.clusterNodeAuthMu.Lock()
	defer s.clusterNodeAuthMu.Unlock()
	clean := make(map[string]string, len(values))
	for k, v := range values {
		id := strings.TrimSpace(k)
		secret := strings.TrimSpace(v)
		if id == "" || secret == "" {
			continue
		}
		clean[id] = secret
	}
	s.clusterNodeAuth = clean
}

func (s *HTTPServer) persistNodeAuth(ctx context.Context) {
	if s == nil {
		return
	}
	s.persistClusterConfig(ctx, clusterConfigKeyNodeAuth, s.snapshotNodeAuth())
}

func (s *HTTPServer) handleClusterHAConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		cfg := s.currentHAConfig()
		return c.JSON(http.StatusOK, s.toClusterHaConfigDTO(cfg))
	}
}

func (s *HTTPServer) handleClusterHAConfigUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterHaConfigDTO
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cfg := s.fromClusterHaConfigDTO(payload)
		s.setHAConfig(cfg)
		s.persistClusterConfig(ctx, clusterConfigKeyHA, s.toClusterHaConfigDTO(cfg))
		return c.JSON(http.StatusOK, s.toClusterHaConfigDTO(cfg))
	}
}

func (s *HTTPServer) handleClusterSyncConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentSyncConfig())
	}
}

func (s *HTTPServer) handleClusterSyncConfigUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterSyncConfigDTO
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		normalized := normalizeSyncConfigPayload(payload)
		s.setSyncConfig(normalized)
		s.persistClusterConfig(ctx, clusterConfigKeySync, normalized)
		return c.JSON(http.StatusOK, normalized)
	}
}

func (s *HTTPServer) handleClusterScalePolicy() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentScalePolicy())
	}
}

func (s *HTTPServer) handleClusterScalePolicyUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterScalePolicyDTO
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		normalized := normalizeScalePolicyPayload(payload)
		s.setScalePolicy(normalized)
		s.persistClusterConfig(ctx, clusterConfigKeyScale, normalized)
		return c.JSON(http.StatusOK, normalized)
	}
}

func (s *HTTPServer) handleClusterSyncStatus() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentSyncStatusWithJoin())
	}
}

func (s *HTTPServer) handleClusterSyncStats() echo.HandlerFunc {
	return func(c echo.Context) error {
		hours := 24
		if raw := strings.TrimSpace(c.QueryParam("hours")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				hours = parsed
			}
		}
		return c.JSON(http.StatusOK, s.buildSyncStats(hours))
	}
}

func (s *HTTPServer) handleClusterSyncTransactions() echo.HandlerFunc {
	return func(c echo.Context) error {
		limit := 20
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		return c.JSON(http.StatusOK, s.recentSyncTransactions(limit))
	}
}

func (s *HTTPServer) handleClusterSyncTransactionGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		txID := strings.TrimSpace(c.Param("txId"))
		if txID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "txId is required")
		}
		tx, ok := s.syncTransactionByID(txID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "sync transaction not found")
		}
		return c.JSON(http.StatusOK, tx)
	}
}

func (s *HTTPServer) handleClusterSyncTransactionRetry() echo.HandlerFunc {
	return func(c echo.Context) error {
		txID := strings.TrimSpace(c.Param("txId"))
		if txID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "txId is required")
		}
		existing, ok := s.syncTransactionByID(txID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "sync transaction not found")
		}

		var payload clusterSyncRetryRequest
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		strategy := strings.TrimSpace(payload.Strategy)
		if strategy == "" {
			strategy = "new_tx"
		}
		if strategy != "new_tx" && strategy != "same_tx" {
			return echo.NewHTTPError(http.StatusBadRequest, "strategy must be new_tx or same_tx")
		}

		if strategy == "same_tx" {
			event, err := s.restartSyncTransaction(existing, payload.SimulateFailure)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, event)
		}

		event := s.startSyncTransaction(existing.Type, existing.SourceNode, existing.TargetNode, txID, strategy, payload.SimulateFailure)
		return c.JSON(http.StatusOK, event)
	}
}

func (s *HTTPServer) handleClusterSyncTrigger() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload clusterSyncTriggerRequest
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		syncType := strings.TrimSpace(payload.Type)
		if syncType == "" {
			syncType = "lease"
		}
		sourceNode, targetNode := s.resolveSyncEndpoints(payload.SourceNode, payload.TargetNode)

		event := s.startSyncTransaction(syncType, sourceNode, targetNode, "", "new_tx", payload.SimulateFailure)
		return c.JSON(http.StatusOK, event)
	}
}

func (s *HTTPServer) resolveSyncEndpoints(sourceNode, targetNode string) (string, string) {
	source := strings.TrimSpace(sourceNode)
	target := strings.TrimSpace(targetNode)
	if source != "" && target != "" {
		if strings.EqualFold(source, target) {
			return source, "standby"
		}
		return source, target
	}

	ctx := context.Background()
	merged := s.customClusterNodes()
	if svc := s.ensureHAService(); svc != nil {
		nodes := buildClusterNodes(svc.Nodes(ctx), svc.Snapshot(), nil)
		merged = s.mergeClusterNodes(nodes)
	}

	active := ""
	standby := ""
	for _, node := range merged {
		if node.Disabled {
			continue
		}
		address := strings.TrimSpace(node.Address)
		if address == "" {
			address = strings.TrimSpace(node.ID)
		}
		if address == "" {
			continue
		}
		if active == "" && strings.EqualFold(strings.TrimSpace(node.Role), "active") {
			active = address
		}
		if standby == "" && strings.EqualFold(strings.TrimSpace(node.Role), "standby") {
			standby = address
		}
	}

	if source == "" {
		if active != "" {
			source = active
		} else {
			source = "active"
		}
	}
	if target == "" {
		if standby != "" {
			target = standby
		} else {
			target = "standby"
		}
	}
	if strings.EqualFold(source, target) {
		if standby != "" && !strings.EqualFold(source, standby) {
			target = standby
		} else {
			target = "standby"
		}
	}
	return source, target
}

func (s *HTTPServer) startSyncTransaction(syncType, sourceNode, targetNode, retryOf, retryStrategy string, simulateFailure bool) clusterFailoverEvent {
	now := time.Now().UTC()
	tx := clusterSyncTransactionDTO{
		TxID:          fmt.Sprintf("tx-%d", now.UnixNano()),
		Type:          syncType,
		SourceNode:    sourceNode,
		TargetNode:    targetNode,
		RetryOf:       retryOf,
		RetryStrategy: retryStrategy,
		Phase:         "bndupd_sent",
		CreatedAt:     now.Format(time.RFC3339),
		BndupdAt:      now.Format(time.RFC3339),
	}
	s.appendSyncTransaction(tx)

	detail := fmt.Sprintf("触发一次 %s：BNDUPD 已发送，等待 BNDACK", syncType)
	if retryOf != "" {
		detail = fmt.Sprintf("重试事务 %s（%s）：BNDUPD 已发送，等待 BNDACK", retryOf, retryStrategy)
	}

	event := clusterFailoverEvent{
		ID:          tx.TxID,
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "手动同步",
		Detail:      detail,
		Status:      "running",
		Type:        "warning",
		StatusLabel: "进行中",
		TxID:        tx.TxID,
		Phase:       tx.Phase,
	}
	s.appendScaleEvent(event)
	go s.executeSyncTransaction(tx.TxID, syncType, sourceNode, targetNode, simulateFailure)

	return event
}

func (s *HTTPServer) restartSyncTransaction(existing clusterSyncTransactionDTO, simulateFailure bool) (clusterFailoverEvent, error) {
	now := time.Now().UTC()
	updated := existing
	updated.Phase = "bndupd_sent"
	updated.CreatedAt = now.Format(time.RFC3339)
	updated.BndupdAt = now.Format(time.RFC3339)
	updated.BndackAt = ""
	updated.AckAt = ""
	updated.CommitAt = ""
	updated.LatencyMs = 0
	updated.ErrorCode = ""
	updated.ErrorMessage = ""
	updated.RetryStrategy = "same_tx"

	if !s.upsertSyncTransaction(updated) {
		return clusterFailoverEvent{}, echo.NewHTTPError(http.StatusNotFound, "sync transaction not found")
	}

	event := clusterFailoverEvent{
		ID:          updated.TxID,
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "手动同步",
		Detail:      fmt.Sprintf("重试事务 %s（same_tx）：BNDUPD 已重新发送，等待 BNDACK", updated.TxID),
		Status:      "running",
		Type:        "warning",
		StatusLabel: "进行中",
		TxID:        updated.TxID,
		Phase:       updated.Phase,
	}
	s.updateScaleEvent(updated.TxID, event.Status, event.StatusLabel, event.Type, event.Detail)
	go s.executeSyncTransaction(updated.TxID, updated.Type, updated.SourceNode, updated.TargetNode, simulateFailure)

	return event, nil
}

func (s *HTTPServer) executeSyncTransaction(txID, syncType, sourceNode, targetNode string, simulateFailure bool) {
	if simulateFailure {
		time.Sleep(160 * time.Millisecond)
		s.failSyncTransaction(txID, "ACK_TIMEOUT", "等待 BNDACK 超时")
		return
	}
	ackTimeout := s.currentHAConfig().Replication.AckTimeout
	if ackTimeout <= 0 {
		ackTimeout = 2 * time.Second
	}
	msg := clusterSyncWireMessage{
		TxID:       txID,
		Type:       syncType,
		SourceNode: sourceNode,
		TargetNode: targetNode,
		Phase:      "bndupd_sent",
		SentAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	latency, err := s.sendClusterSyncBndupd(context.Background(), msg, ackTimeout)
	if err != nil {
		code, message := classifySyncTransportError(err)
		s.failSyncTransaction(txID, code, message)
		return
	}
	s.completeSyncTransactionWithLatency(txID, latency)
}

func classifySyncTransportError(err error) (string, string) {
	if err == nil {
		return "ACK_IO_ERROR", "同步通道异常"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "ACK_TIMEOUT", "等待 BNDACK 超时"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "ACK_TIMEOUT", "等待 BNDACK 超时"
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "refused") {
		return "ACK_CONN_REFUSED", "对端拒绝连接"
	}
	if strings.Contains(lower, "mismatch") || strings.Contains(lower, "decode") || strings.Contains(lower, "invalid") {
		return "ACK_PROTOCOL_ERROR", "同步协议响应异常"
	}
	if msg == "" {
		msg = "同步通道异常"
	}
	return "ACK_IO_ERROR", msg
}

func (s *HTTPServer) handleClusterScaleEvents() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentScaleEvents())
	}
}

func (s *HTTPServer) handleClusterBackupPlan() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentBackupPlan())
	}
}

func (s *HTTPServer) handleClusterBackupPlanUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterBackupPlanDTO
		if err := c.Bind(&payload); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		}
		normalized := normalizeBackupPlanPayload(payload)
		s.setBackupPlan(normalized)
		s.persistClusterConfig(ctx, clusterConfigKeyBackupPlan, normalized)
		return c.JSON(http.StatusOK, normalized)
	}
}

func (s *HTTPServer) handleClusterOptimizationPlan() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentOptimizationPlan())
	}
}

func (s *HTTPServer) handleClusterOptimizationPlanUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterOptimizationPlanDTO
		if err := c.Bind(&payload); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		}
		s.setOptimizationPlan(payload)
		s.persistClusterConfig(ctx, clusterConfigKeyOptimizationPlan, payload)
		return c.JSON(http.StatusOK, payload)
	}
}

func (s *HTTPServer) handleClusterBackupRun() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		record := s.startBackupRunRecord(ctx, s.actorFromContext(c))
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "backup.run", s.auditPayloadFromRequest(c, map[string]any{
			"type":      record.Type,
			"status":    record.Status,
			"phase":     record.Phase,
			"txId":      record.TxID,
			"errorCode": record.ErrorCode,
		}), withResource("cluster_backup"))
		return c.JSON(http.StatusOK, record)
	}
}

func (s *HTTPServer) handleClusterBackupRestore() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload clusterBackupRestoreRequest
		if err := c.Bind(&payload); err != nil || strings.TrimSpace(payload.Time) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "time is required"})
		}
		restorePoint := strings.TrimSpace(payload.Time)
		record := s.startBackupRestoreRecord(c.Request().Context(), s.actorFromContext(c), restorePoint)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "backup.restore", s.auditPayloadFromRequest(c, map[string]any{
			"restorePoint": restorePoint,
			"type":         record.Type,
			"status":       record.Status,
			"phase":        record.Phase,
			"txId":         record.TxID,
			"errorCode":    record.ErrorCode,
		}), withResource("cluster_backup"))
		return c.JSON(http.StatusOK, record)
	}
}

func (s *HTTPServer) handleClusterOptimizationRun() echo.HandlerFunc {
	return func(c echo.Context) error {
		record := s.appendBackupRecord(clusterBackupRecordDTO{Type: "优化分析", Size: "-", Status: "进行中"})
		return c.JSON(http.StatusOK, record)
	}
}

func (s *HTTPServer) startBackupRunRecord(ctx context.Context, actor string) clusterBackupRecordDTO {
	if ctx == nil {
		ctx = context.Background()
	}
	req := ops.TransferRequest{
		Kind:        ops.TransferKindExport,
		Resource:    "cluster-backup",
		Format:      "json",
		TargetURI:   "cluster/backup/export",
		Reason:      "cluster backup run",
		RequestedBy: firstNonEmpty(strings.TrimSpace(actor), "cluster-system"),
		Metadata: map[string]string{
			"surface": "cluster",
			"action":  "backup_run",
		},
	}
	job, err := s.startBackupTransfer(ctx, req)
	if err != nil {
		code := "BACKUP_EXPORT_START_FAILED"
		return s.appendBackupRecord(clusterBackupRecordDTO{
			Type:      "立即备份",
			Size:      "-",
			Status:    "提交失败",
			Phase:     "failed",
			ErrorCode: code,
		})
	}
	return s.appendBackupRecord(clusterBackupRecordDTO{
		Type:      "立即备份",
		Size:      formatTransferSize(job.SizeBytes),
		Status:    "已提交",
		TxID:      job.ID,
		Phase:     string(job.Status),
		AckAt:     transferAckAt(job),
		ErrorCode: backupTransferErrorCode(job),
	})
}

func (s *HTTPServer) startBackupRestoreRecord(ctx context.Context, actor, restorePoint string) clusterBackupRecordDTO {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceURI := s.resolveRestoreSourceURI(ctx, restorePoint)
	if strings.TrimSpace(sourceURI) == "" {
		return s.appendBackupRecord(clusterBackupRecordDTO{
			Type:      "恢复",
			Size:      "-",
			Status:    fmt.Sprintf("恢复提交失败：%s", restorePoint),
			Phase:     "failed",
			ErrorCode: "BACKUP_RESTORE_SOURCE_NOT_FOUND",
		})
	}
	req := ops.TransferRequest{
		Kind:        ops.TransferKindImport,
		Resource:    "cluster-backup",
		Format:      "json",
		SourceURI:   sourceURI,
		Reason:      "cluster backup restore",
		RequestedBy: firstNonEmpty(strings.TrimSpace(actor), "cluster-system"),
		Metadata: map[string]string{
			"surface":       "cluster",
			"action":        "backup_restore",
			"restore_point": restorePoint,
		},
	}
	job, err := s.startBackupTransfer(ctx, req)
	if err != nil {
		return s.appendBackupRecord(clusterBackupRecordDTO{
			Type:      "恢复",
			Size:      "-",
			Status:    fmt.Sprintf("恢复提交失败：%s", restorePoint),
			Phase:     "failed",
			ErrorCode: "BACKUP_RESTORE_START_FAILED",
		})
	}
	return s.appendBackupRecord(clusterBackupRecordDTO{
		Type:      "恢复",
		Size:      formatTransferSize(job.SizeBytes),
		Status:    fmt.Sprintf("已提交恢复：%s", restorePoint),
		TxID:      job.ID,
		Phase:     string(job.Status),
		AckAt:     transferAckAt(job),
		ErrorCode: backupTransferErrorCode(job),
	})
}

func (s *HTTPServer) startBackupTransfer(ctx context.Context, req ops.TransferRequest) (*ops.TransferJob, error) {
	if s == nil || s.opsSvc == nil {
		return nil, errors.New("ops service unavailable")
	}
	return s.opsSvc.StartTransferJob(ctx, req)
}

func (s *HTTPServer) resolveRestoreSourceURI(ctx context.Context, restorePoint string) string {
	restorePoint = strings.TrimSpace(restorePoint)
	if restorePoint == "" {
		return ""
	}
	if strings.ContainsAny(restorePoint, `\\/`) || strings.Contains(restorePoint, ".") {
		return restorePoint
	}
	if s == nil || s.opsSvc == nil {
		return ""
	}
	if job, err := s.opsSvc.GetTransferJob(ctx, restorePoint); err == nil && job != nil {
		if strings.TrimSpace(job.ArtifactPath) != "" {
			return job.ArtifactPath
		}
	}

	restoreAt := parseClusterTime(restorePoint)
	if !restoreAt.IsZero() {
		history := s.currentBackupHistory()
		for _, rec := range history {
			ts := parseClusterTime(rec.Time)
			if ts.IsZero() {
				continue
			}
			if ts.Equal(restoreAt) || durationAbs(ts.Sub(restoreAt)) <= time.Minute {
				if strings.TrimSpace(rec.TxID) == "" {
					break
				}
				if job, err := s.opsSvc.GetTransferJob(ctx, strings.TrimSpace(rec.TxID)); err == nil && job != nil && strings.TrimSpace(job.ArtifactPath) != "" {
					return job.ArtifactPath
				}
			}
		}
	}

	jobs, err := s.opsSvc.ListTransferJobs(ctx, 50)
	if err != nil {
		return ""
	}
	for _, job := range jobs {
		if job.Kind != ops.TransferKindExport {
			continue
		}
		if strings.TrimSpace(job.ArtifactPath) == "" {
			continue
		}
		if strings.TrimSpace(job.ID) == restorePoint {
			return job.ArtifactPath
		}
		if job.Status != ops.TransferStatusSucceeded && job.Status != ops.TransferStatusRunning && job.Status != ops.TransferStatusPending {
			continue
		}
		if !restoreAt.IsZero() {
			jobTs := transferRecordTime(job)
			parsed := parseClusterTime(jobTs)
			if !parsed.IsZero() && (parsed.Equal(restoreAt) || durationAbs(parsed.Sub(restoreAt)) <= time.Minute) {
				return job.ArtifactPath
			}
			continue
		}
		return job.ArtifactPath
	}
	return ""
}

func transferAckAt(job *ops.TransferJob) string {
	if job == nil {
		return ""
	}
	if !job.CompletedAt.IsZero() {
		return job.CompletedAt.UTC().Format(time.RFC3339)
	}
	if !job.StartedAt.IsZero() {
		return job.StartedAt.UTC().Format(time.RFC3339)
	}
	if !job.CreatedAt.IsZero() {
		return job.CreatedAt.UTC().Format(time.RFC3339)
	}
	return ""
}

func backupTransferErrorCode(job *ops.TransferJob) string {
	if job == nil || strings.TrimSpace(job.Error) == "" {
		return ""
	}
	if job.Status == ops.TransferStatusFailed {
		return "BACKUP_TRANSFER_FAILED"
	}
	return ""
}

func formatTransferSize(sizeBytes int64) string {
	if sizeBytes <= 0 {
		return "-"
	}
	mb := float64(sizeBytes) / (1024 * 1024)
	if mb >= 1 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	kb := float64(sizeBytes) / 1024
	return fmt.Sprintf("%.1f KB", kb)
}

func (s *HTTPServer) handleClusterBackupHistory() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.currentBackupHistory())
	}
}

func normalizeHaLbConfigPayload(payload clusterHaLbConfigDTO) clusterHaLbConfigDTO {
	if payload.Mode == "" {
		payload.Mode = "active-passive"
	}
	if payload.HeartbeatSec <= 0 {
		payload.HeartbeatSec = 5
	}
	if payload.FailThreshold <= 0 {
		payload.FailThreshold = 3
	}
	if len(payload.Weights) == 0 {
		payload.Weights = defaultHaLbWeights()
	}
	return payload
}

func normalizeScalePolicyPayload(payload clusterScalePolicyDTO) clusterScalePolicyDTO {
	if payload.CPU <= 0 {
		payload.CPU = 70
	}
	if payload.Mem <= 0 {
		payload.Mem = 70
	}
	if payload.QPS <= 0 {
		payload.QPS = 8000
	}
	if payload.MaxNodes <= 0 {
		payload.MaxNodes = 12
	}
	return payload
}

func normalizeBackupPlanPayload(payload clusterBackupPlanDTO) clusterBackupPlanDTO {
	if payload.Freq == "" {
		payload.Freq = "daily"
	}
	if payload.Retain == "" {
		payload.Retain = "30"
	}
	if payload.Storage == "" {
		payload.Storage = "hybrid"
	}
	return payload
}

func normalizeSyncConfigPayload(payload clusterSyncConfigDTO) clusterSyncConfigDTO {
	if payload.Mode == "" {
		payload.Mode = "strong"
	}
	transport := strings.ToLower(strings.TrimSpace(payload.Transport))
	if transport == "" {
		transport = "tcp"
	}
	if transport != "mock" && transport != "tcp" {
		transport = "tcp"
	}
	payload.Transport = transport
	if payload.LeaseFreq == "" {
		payload.LeaseFreq = "1"
	}
	if payload.ConfigFreq == "" {
		payload.ConfigFreq = "5"
	}
	if payload.ConflictPolicy == "" {
		payload.ConflictPolicy = "version"
	}
	return payload
}

func (s *HTTPServer) handleClusterHaLbConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		cfg := s.currentHaLbConfig()
		return c.JSON(http.StatusOK, cfg)
	}
}

func (s *HTTPServer) handleClusterHaLbConfigUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterHaLbConfigDTO
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		normalized := normalizeHaLbConfigPayload(payload)
		s.setHaLbConfig(normalized)
		s.persistClusterConfig(ctx, clusterConfigKeyHaLb, normalized)
		return c.JSON(http.StatusOK, normalized)
	}
}

func (s *HTTPServer) handleClusterHaLbFailoverEvents() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.failoverHistory())
	}
}

func (s *HTTPServer) handleClusterFailoverPlansList() echo.HandlerFunc {
	return func(c echo.Context) error {
		items := s.listFailoverPlans()
		return c.JSON(http.StatusOK, map[string]any{"items": items, "count": len(items)})
	}
}

func (s *HTTPServer) handleClusterFailoverPlanGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		planID := strings.TrimSpace(c.Param("planId"))
		if planID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "planId is required")
		}
		plan, ok := s.getFailoverPlan(planID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "failover plan not found")
		}
		return c.JSON(http.StatusOK, plan)
	}
}

func (s *HTTPServer) handleClusterFailoverPlanCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterFailoverPlanCreateRequest
		if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = "manual orchestrated failover"
		}
		sourceNode, targetNode := s.resolveSyncEndpoints(req.SourceNode, req.TargetNode)
		now := time.Now().UTC()
		plan := clusterFailoverPlanDTO{
			PlanID:            uuid.NewString(),
			Status:            failoverPlanStatusPending,
			Reason:            reason,
			DryRun:            req.DryRun,
			SourceNode:        sourceNode,
			TargetNode:        targetNode,
			CreatedAt:         now.Format(time.RFC3339),
			UpdatedAt:         now.Format(time.RFC3339),
			RollbackTriggered: false,
			Steps: []clusterFailoverPlanStepDTO{
				{ID: "precheck", Title: "预检查", Status: failoverPlanStepPending},
				{ID: "freeze_old_primary", Title: "冻结旧主", Status: failoverPlanStepPending},
				{ID: "promote_storage", Title: "提升存储", Status: failoverPlanStepPending},
				{ID: "promote_dhcp", Title: "提升 DHCP", Status: failoverPlanStepPending},
				{ID: "switch_traffic", Title: "切流量", Status: failoverPlanStepPending},
				{ID: "verify", Title: "验证", Status: failoverPlanStepPending},
			},
		}
		s.setFailoverPlan(plan)

		s.appendFailoverEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("fo-plan-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "切换计划已创建",
			Detail:      fmt.Sprintf("计划 %s 已创建，等待执行", plan.PlanID),
			Status:      "running",
			Type:        "warning",
			StatusLabel: "进行中",
			TxID:        plan.PlanID,
			Phase:       "plan_created",
		})

		tenantID := s.auditTenantFromContext(c)
		actor := s.actorFromContext(c)
		trigger := "manual"
		if plan.DryRun {
			trigger = "manual_dry_run"
		}
		s.recordClusterSwitchAuditEvent(c.Request().Context(), tenantID, actor, "trigger", trigger, plan.Reason, plan.SourceNode, plan.TargetNode, "pending", false)
		s.recordAudit(c.Request().Context(), tenantID, actor, "cluster.failover.plan.create", map[string]any{
			"planId":      plan.PlanID,
			"reason":      plan.Reason,
			"dryRun":      plan.DryRun,
			"sourceNode":  plan.SourceNode,
			"targetNode":  plan.TargetNode,
			"createdAt":   plan.CreatedAt,
			"createdBy":   actor,
			"failureStep": strings.TrimSpace(req.SimulateFailureStep),
		}, withResource("cluster"))

		go s.runFailoverPlan(plan.PlanID, strings.TrimSpace(req.SimulateFailureStep), tenantID, actor, trigger, plan.Reason, plan.SourceNode, plan.TargetNode)
		return c.JSON(http.StatusAccepted, plan)
	}
}

func (s *HTTPServer) runFailoverPlan(planID, simulateFailureStep, tenantID, actor, trigger, reason, oldPrimary, newPrimary string) {
	stepOrder := []string{"precheck", "freeze_old_primary", "promote_storage", "promote_dhcp", "switch_traffic", "verify"}
	executed := make([]string, 0, len(stepOrder))

	if _, ok := s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		p.Status = failoverPlanStatusRunning
		p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}); !ok {
		return
	}
	s.recordClusterSwitchAuditEvent(context.Background(), tenantID, actor, "running", trigger, reason, oldPrimary, newPrimary, "running", false)

	for _, stepID := range stepOrder {
		if !s.startFailoverPlanStep(planID, stepID) {
			return
		}
		if err := s.executeFailoverPlanStep(planID, stepID, simulateFailureStep); err != nil {
			s.failFailoverPlanStep(planID, stepID, err)
			s.rollbackFailoverPlan(planID, executed, tenantID, actor, trigger)
			return
		}
		s.completeFailoverPlanStep(planID, stepID, failoverPlanStepSuccess, "")
		executed = append(executed, stepID)
	}

	final, ok := s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		now := time.Now().UTC().Format(time.RFC3339)
		p.Status = failoverPlanStatusSuccess
		p.UpdatedAt = now
		p.FinishedAt = now
	})
	if ok {
		s.recordClusterSwitchAuditEvent(context.Background(), tenantID, actor, "result", trigger, reason, oldPrimary, newPrimary, "success", false)
		s.appendFailoverEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("fo-plan-done-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "切换计划完成",
			Detail:      fmt.Sprintf("计划 %s 已按步骤执行完成", final.PlanID),
			Status:      "success",
			Type:        "success",
			StatusLabel: "完成",
			TxID:        final.PlanID,
			Phase:       "verify",
		})
	}
}

func (s *HTTPServer) executeFailoverPlanStep(planID, stepID, simulateFailureStep string) error {
	if strings.EqualFold(strings.TrimSpace(simulateFailureStep), stepID) {
		return fmt.Errorf("simulated failure at step %s", stepID)
	}
	if stepID == "precheck" {
		cfg := s.currentHAConfig()
		if strings.TrimSpace(cfg.Mode) == "" || strings.EqualFold(strings.TrimSpace(cfg.Mode), "disabled") {
			return errors.New("ha mode is disabled")
		}
		if strings.TrimSpace(cfg.Partner.Address) == "" && strings.TrimSpace(cfg.Node.ID) == "" {
			return errors.New("ha config is incomplete for orchestrated failover")
		}
		ctx := context.Background()
		merged := s.customClusterNodes()
		if svc := s.ensureHAService(); svc != nil {
			merged = s.mergeClusterNodes(buildClusterNodes(svc.Nodes(ctx), svc.Snapshot(), nil))
		}
		hasActive := false
		hasStandby := false
		for _, node := range merged {
			if node.Disabled {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(node.Role), "active") {
				hasActive = true
			}
			if strings.EqualFold(strings.TrimSpace(node.Role), "standby") {
				hasStandby = true
			}
		}
		if !hasActive || !hasStandby {
			return errors.New("precheck failed: active/standby pair is not ready")
		}
	}
	time.Sleep(35 * time.Millisecond)
	return nil
}

func (s *HTTPServer) rollbackFailoverPlan(planID string, executed []string, tenantID, actor, trigger string) {
	rollbackStatus := failoverPlanStatusSuccess
	rollbackSteps := map[string]string{
		"switch_traffic":     "rollback_switch_traffic",
		"promote_dhcp":       "rollback_promote_dhcp",
		"promote_storage":    "rollback_promote_storage",
		"freeze_old_primary": "rollback_unfreeze_old_primary",
	}

	for i := len(executed) - 1; i >= 0; i-- {
		forward := executed[i]
		rollbackID, ok := rollbackSteps[forward]
		if !ok {
			continue
		}
		s.ensureFailoverRollbackStep(planID, rollbackID)
		if !s.startFailoverPlanStep(planID, rollbackID) {
			rollbackStatus = failoverPlanStatusFailed
			break
		}
		time.Sleep(20 * time.Millisecond)
		s.completeFailoverPlanStep(planID, rollbackID, failoverPlanStepSuccess, "")
	}

	plan, ok := s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		now := time.Now().UTC().Format(time.RFC3339)
		p.RollbackTriggered = true
		if rollbackStatus == failoverPlanStatusSuccess {
			p.RollbackStatus = "SUCCESS"
			p.Status = failoverPlanStatusRolledBack
		} else {
			p.RollbackStatus = "FAILED"
			p.Status = failoverPlanStatusFailed
		}
		p.UpdatedAt = now
		p.FinishedAt = now
	})
	if !ok {
		return
	}

	eventStatus := "failed"
	eventType := "danger"
	eventLabel := "失败"
	detail := fmt.Sprintf("计划 %s 执行失败，回滚未完全成功", plan.PlanID)
	if strings.EqualFold(plan.RollbackStatus, "SUCCESS") {
		eventStatus = "success"
		eventType = "warning"
		eventLabel = "已回滚"
		detail = fmt.Sprintf("计划 %s 执行失败，回滚已完成", plan.PlanID)
	}

	s.appendFailoverEvent(clusterFailoverEvent{
		ID:          fmt.Sprintf("fo-plan-rollback-%d", time.Now().UnixNano()),
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "切换计划回滚",
		Detail:      detail,
		Status:      eventStatus,
		Type:        eventType,
		StatusLabel: eventLabel,
		TxID:        plan.PlanID,
		Phase:       "rollback",
	})

	result := "failed"
	if strings.EqualFold(plan.RollbackStatus, "SUCCESS") {
		result = "rolled_back"
	}
	s.recordClusterSwitchAuditEvent(context.Background(), tenantID, actor, "result", trigger, plan.Reason, plan.SourceNode, plan.TargetNode, result, true)
}

func (s *HTTPServer) startFailoverPlanStep(planID, stepID string) bool {
	_, ok := s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		now := time.Now().UTC()
		for i := range p.Steps {
			if p.Steps[i].ID != stepID {
				continue
			}
			p.Steps[i].Status = failoverPlanStepRunning
			p.Steps[i].StartedAt = now.Format(time.RFC3339)
			p.Steps[i].FinishedAt = ""
			p.Steps[i].Error = ""
			break
		}
		p.UpdatedAt = now.Format(time.RFC3339)
	})
	return ok
}

func (s *HTTPServer) completeFailoverPlanStep(planID, stepID, status, detail string) {
	_, _ = s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		now := time.Now().UTC()
		for i := range p.Steps {
			if p.Steps[i].ID != stepID {
				continue
			}
			p.Steps[i].Status = status
			p.Steps[i].Detail = detail
			p.Steps[i].FinishedAt = now.Format(time.RFC3339)
			if p.Steps[i].StartedAt != "" {
				if started, err := time.Parse(time.RFC3339, p.Steps[i].StartedAt); err == nil {
					p.Steps[i].DurationMs = int64(now.Sub(started) / time.Millisecond)
				}
			}
			break
		}
		p.UpdatedAt = now.Format(time.RFC3339)
	})
}

func (s *HTTPServer) failFailoverPlanStep(planID, stepID string, stepErr error) {
	msg := "unknown error"
	if stepErr != nil {
		msg = strings.TrimSpace(stepErr.Error())
	}
	_, _ = s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		now := time.Now().UTC()
		for i := range p.Steps {
			if p.Steps[i].ID != stepID {
				continue
			}
			p.Steps[i].Status = failoverPlanStepFailed
			p.Steps[i].Error = msg
			p.Steps[i].FinishedAt = now.Format(time.RFC3339)
			if p.Steps[i].StartedAt != "" {
				if started, err := time.Parse(time.RFC3339, p.Steps[i].StartedAt); err == nil {
					p.Steps[i].DurationMs = int64(now.Sub(started) / time.Millisecond)
				}
			}
			break
		}
		p.Status = failoverPlanStatusFailed
		p.FailedStep = stepID
		p.UpdatedAt = now.Format(time.RFC3339)
	})
}

func (s *HTTPServer) ensureFailoverRollbackStep(planID, stepID string) {
	_, _ = s.updateFailoverPlan(planID, func(p *clusterFailoverPlanDTO) {
		for i := range p.Steps {
			if p.Steps[i].ID == stepID {
				return
			}
		}
		title := strings.TrimPrefix(stepID, "rollback_")
		title = strings.ReplaceAll(title, "_", " ")
		p.Steps = append(p.Steps, clusterFailoverPlanStepDTO{
			ID:       stepID,
			Title:    "回滚 " + title,
			Status:   failoverPlanStepPending,
			Rollback: true,
		})
	})
}

func (s *HTTPServer) setFailoverPlan(plan clusterFailoverPlanDTO) {
	s.failoverPlansMu.Lock()
	defer s.failoverPlansMu.Unlock()
	if s.failoverPlans == nil {
		s.failoverPlans = make(map[string]clusterFailoverPlanDTO)
	}
	s.failoverPlans[plan.PlanID] = plan
}

func (s *HTTPServer) getFailoverPlan(planID string) (clusterFailoverPlanDTO, bool) {
	s.failoverPlansMu.RLock()
	defer s.failoverPlansMu.RUnlock()
	if s.failoverPlans == nil {
		return clusterFailoverPlanDTO{}, false
	}
	plan, ok := s.failoverPlans[planID]
	return plan, ok
}

func (s *HTTPServer) updateFailoverPlan(planID string, mutate func(*clusterFailoverPlanDTO)) (clusterFailoverPlanDTO, bool) {
	s.failoverPlansMu.Lock()
	defer s.failoverPlansMu.Unlock()
	if s.failoverPlans == nil {
		return clusterFailoverPlanDTO{}, false
	}
	plan, ok := s.failoverPlans[planID]
	if !ok {
		return clusterFailoverPlanDTO{}, false
	}
	if mutate != nil {
		mutate(&plan)
	}
	s.failoverPlans[planID] = plan
	return plan, true
}

func (s *HTTPServer) listFailoverPlans() []clusterFailoverPlanDTO {
	s.failoverPlansMu.RLock()
	defer s.failoverPlansMu.RUnlock()
	items := make([]clusterFailoverPlanDTO, 0, len(s.failoverPlans))
	for _, plan := range s.failoverPlans {
		items = append(items, plan)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

func (s *HTTPServer) handleClusterHaLbFailoverTest() echo.HandlerFunc {
	return func(c echo.Context) error {
		event := clusterFailoverEvent{
			ID:          fmt.Sprintf("fo-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "手动切换测试",
			Detail:      "已触发一次切换/重分配测试",
			Status:      "success",
			Type:        "info",
			StatusLabel: "完成",
		}
		s.appendFailoverEvent(event)
		return c.JSON(http.StatusOK, event)
	}
}

func (s *HTTPServer) handleClusterNodeCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload clusterNodePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		node, err := s.clusterNodeFromPayload(payload)
		if err != nil {
			return err
		}
		authUpdated := s.upsertNodeAuth(node.ID, payload.Auth)
		if authUpdated {
			node.HasAuth = true
			s.persistNodeAuth(ctx)
		} else {
			node.HasAuth = s.nodeAuthConfigured(node.ID)
		}
		s.setCustomClusterNode(node)
		s.persistClusterNodes(ctx)
		s.reconcileHAConfigFromClusterNodes(ctx)
		if err := s.upsertFailoverMembership(ctx, node); err != nil && s.logger != nil {
			s.logger.Warn("upsert failover membership", zap.String("nodeId", node.ID), zap.Error(err))
		}
		s.applyNodeConfigPayload(ctx, payload)
		joinJobID := s.enqueueNodeJoinIfNeeded(ctx, node, "create")
		if joinJobID != "" {
			c.Response().Header().Set("X-Join-Job-Id", joinJobID)
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.node.create", s.auditPayloadFromRequest(c, map[string]any{
			"nodeId":      node.ID,
			"address":     node.Address,
			"role":        node.Role,
			"disabled":    node.Disabled,
			"health":      node.Health,
			"version":     node.Version,
			"hasAuth":     node.HasAuth,
			"authUpdated": authUpdated,
		}), withResource("cluster_node"))
		return c.JSON(http.StatusCreated, node)
	}
}

func (s *HTTPServer) handleClusterNodeUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		id := strings.TrimSpace(c.Param("nodeId"))
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "nodeId is required")
		}
		var payload clusterNodePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		id = s.resolveClusterNodeMutationID(id, payload.Address)
		existing, ok := s.customClusterNodeByID(id)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "node not found")
		}
		payload.ID = id
		if strings.TrimSpace(payload.Address) == "" {
			payload.Address = existing.Address
		}
		if strings.TrimSpace(payload.Role) == "" {
			payload.Role = existing.Role
		}
		if strings.TrimSpace(payload.Version) == "" {
			payload.Version = existing.Version
		}
		if strings.TrimSpace(payload.Health) == "" {
			payload.Health = existing.Health
		}
		if payload.CPUPercent == 0 {
			payload.CPUPercent = existing.CPUPercent
		}
		if payload.MemoryPercent == 0 {
			payload.MemoryPercent = existing.MemoryPercent
		}
		if payload.SyncLagMs == 0 {
			payload.SyncLagMs = existing.SyncLagMs
		}
		if payload.Disabled == nil {
			d := existing.Disabled
			payload.Disabled = &d
		}

		node, err := s.clusterNodeFromPayload(payload)
		if err != nil {
			return err
		}
		authUpdated := s.upsertNodeAuth(node.ID, payload.Auth)
		if authUpdated {
			node.HasAuth = true
			s.persistNodeAuth(ctx)
		} else {
			node.HasAuth = s.nodeAuthConfigured(node.ID)
		}
		s.setCustomClusterNode(node)
		s.persistClusterNodes(ctx)
		s.reconcileHAConfigFromClusterNodes(ctx)
		if err := s.upsertFailoverMembership(ctx, node); err != nil && s.logger != nil {
			s.logger.Warn("upsert failover membership", zap.String("nodeId", node.ID), zap.Error(err))
		}
		s.applyNodeConfigPayload(ctx, payload)
		joinJobID := s.enqueueNodeJoinIfNeeded(ctx, node, "update")
		if joinJobID != "" {
			c.Response().Header().Set("X-Join-Job-Id", joinJobID)
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.node.update", s.auditPayloadFromRequest(c, map[string]any{
			"nodeId":      node.ID,
			"before":      toAuditMap(existing),
			"after":       toAuditMap(node),
			"hasAuth":     node.HasAuth,
			"authUpdated": authUpdated,
		}), withResource("cluster_node"))
		return c.JSON(http.StatusOK, node)
	}
}

func (s *HTTPServer) handleClusterNodeDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		id := strings.TrimSpace(c.Param("nodeId"))
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "nodeId is required")
		}
		id = s.resolveClusterNodeMutationID(id, "")
		existing, ok := s.customClusterNodeByID(id)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "node not found")
		}
		if !s.deleteCustomClusterNode(id) {
			return echo.NewHTTPError(http.StatusNotFound, "node not found")
		}
		authRemoved := s.deleteNodeAuth(id)
		if authRemoved {
			s.persistNodeAuth(ctx)
		}
		s.persistClusterNodes(ctx)
		s.reconcileHAConfigFromClusterNodes(ctx)
		if err := s.removeFailoverMembership(ctx, id); err != nil && s.logger != nil {
			s.logger.Warn("remove failover membership", zap.String("nodeId", id), zap.Error(err))
		}
		s.cancelNodeJoinJobs(id)
		s.appendScaleEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("node-delete-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "节点删除",
			Detail:      fmt.Sprintf("节点 %s 已删除", id),
			Status:      "success",
			Type:        "warning",
			StatusLabel: "完成",
		})
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.node.delete", s.auditPayloadFromRequest(c, map[string]any{
			"nodeId":      id,
			"before":      toAuditMap(existing),
			"authRemoved": authRemoved,
		}), withResource("cluster_node"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) enqueueNodeJoinIfNeeded(ctx context.Context, node clusterOverviewNode, action string) string {
	if s == nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(node.Role), "standby") || node.Disabled {
		return ""
	}
	nodeID := strings.TrimSpace(node.ID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(node.Address)
	}
	if nodeID == "" {
		return ""
	}
	job, created := s.ensureJoinJobForNode(nodeID, strings.TrimSpace(node.Address), "cluster-api")
	if !created {
		return job.JobID
	}
	now := job.CreatedAt
	s.appendScaleEvent(clusterFailoverEvent{
		ID:          fmt.Sprintf("node-join-%s", job.JobID),
		Time:        now.Format("2006-01-02 15:04:05"),
		Title:       "节点加入编排",
		Detail:      fmt.Sprintf("节点 %s (%s) 已触发加入流程", nodeID, action),
		Status:      "running",
		Type:        "warning",
		StatusLabel: "进行中",
		TxID:        job.JobID,
		Phase:       joinJobStateRegistered,
	})
	return job.JobID
}

func (s *HTTPServer) cancelNodeJoinJobs(nodeID string) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || s == nil {
		return
	}
	s.joinJobsMu.RLock()
	jobIDs := make([]string, 0)
	for _, job := range s.joinJobs {
		if strings.TrimSpace(job.NodeID) == nodeID {
			jobIDs = append(jobIDs, job.JobID)
		}
	}
	s.joinJobsMu.RUnlock()
	for _, jobID := range jobIDs {
		s.cancelJoinJobPipeline(jobID)
	}
}

func (s *HTTPServer) clusterNodeFromPayload(payload clusterNodePayload) (clusterOverviewNode, error) {
	id := strings.TrimSpace(payload.ID)
	address := strings.TrimSpace(payload.Address)
	if id == "" {
		return clusterOverviewNode{}, echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if address == "" {
		return clusterOverviewNode{}, echo.NewHTTPError(http.StatusBadRequest, "address is required")
	}
	role := strings.TrimSpace(payload.Role)
	if role == "" {
		role = "standby"
	}
	health := strings.TrimSpace(payload.Health)
	if health == "" {
		health = "healthy"
	}
	node := clusterOverviewNode{
		ID:            id,
		Address:       address,
		Role:          role,
		Disabled:      payload.Disabled != nil && *payload.Disabled,
		Version:       normalizeClusterNodeVersion(payload.Version),
		Health:        health,
		CPUPercent:    payload.CPUPercent,
		MemoryPercent: payload.MemoryPercent,
		SyncLagMs:     payload.SyncLagMs,
		LastHeartbeat: time.Now().UTC().Format(time.RFC3339),
	}
	return node, nil
}

func (s *HTTPServer) setCustomClusterNode(node clusterOverviewNode) bool {
	s.clusterNodesMu.Lock()
	defer s.clusterNodesMu.Unlock()
	if s.clusterNodes == nil {
		s.clusterNodes = make(map[string]clusterOverviewNode)
	}
	_, existed := s.clusterNodes[node.ID]
	s.clusterNodes[node.ID] = node
	return existed
}

func (s *HTTPServer) customClusterNodeByID(id string) (clusterOverviewNode, bool) {
	s.clusterNodesMu.RLock()
	defer s.clusterNodesMu.RUnlock()
	if s.clusterNodes == nil {
		return clusterOverviewNode{}, false
	}
	node, ok := s.clusterNodes[id]
	return node, ok
}

func (s *HTTPServer) resolveClusterNodeMutationID(id string, hintAddress string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	if _, ok := s.customClusterNodeByID(id); ok {
		return id
	}

	for _, node := range s.customClusterNodes() {
		if strings.EqualFold(strings.TrimSpace(node.Address), id) && strings.TrimSpace(node.ID) != "" {
			return node.ID
		}
	}

	if !strings.EqualFold(id, "self") {
		return id
	}

	primaryID := strings.TrimSpace(s.currentHAConfig().Node.ID)
	if primaryID != "" {
		if _, ok := s.customClusterNodeByID(primaryID); ok {
			return primaryID
		}
	}

	if addr := strings.TrimSpace(hintAddress); addr != "" {
		for _, node := range s.customClusterNodes() {
			if strings.EqualFold(strings.TrimSpace(node.Address), addr) && strings.TrimSpace(node.ID) != "" {
				return node.ID
			}
		}
	}

	for _, node := range s.customClusterNodes() {
		if node.Disabled || strings.TrimSpace(node.ID) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(node.Role), "active") {
			return node.ID
		}
	}

	return id
}

func (s *HTTPServer) deleteCustomClusterNode(id string) bool {
	s.clusterNodesMu.Lock()
	defer s.clusterNodesMu.Unlock()
	if s.clusterNodes == nil {
		return false
	}
	if _, ok := s.clusterNodes[id]; !ok {
		return false
	}
	delete(s.clusterNodes, id)
	return true
}

func (s *HTTPServer) applyNodeConfigPayload(ctx context.Context, payload clusterNodePayload) {
	if ctx == nil {
		ctx = context.Background()
	}
	if payload.HAConfig != nil {
		cfg := s.fromClusterHaConfigDTO(*payload.HAConfig)
		s.setHAConfig(cfg)
		s.persistClusterConfig(ctx, clusterConfigKeyHA, s.toClusterHaConfigDTO(cfg))
	}
	if payload.HaLbConfig != nil {
		cfg := normalizeHaLbConfigPayload(*payload.HaLbConfig)
		s.setHaLbConfig(cfg)
		s.persistClusterConfig(ctx, clusterConfigKeyHaLb, cfg)
	}
	if payload.SyncConfig != nil {
		cfg := normalizeSyncConfigPayload(*payload.SyncConfig)
		s.setSyncConfig(cfg)
		s.persistClusterConfig(ctx, clusterConfigKeySync, cfg)
	}
}

func defaultHaLbWeights() []clusterHaLbWeightDTO {
	return []clusterHaLbWeightDTO{
		{ID: "node-a", Role: "active", Weight: 50},
		{ID: "node-b", Role: "standby", Weight: 30},
		{ID: "node-c", Role: "worker", Weight: 20},
	}
}

func defaultClusterHaLbFromConfig(cfg config.HAConfig) clusterHaLbConfigDTO {
	mode := cfg.Mode
	if mode == "" {
		mode = "active-passive"
	}
	return clusterHaLbConfigDTO{
		Mode:          mode,
		HeartbeatSec:  int(cfg.HeartbeatInterval / time.Second),
		FailThreshold: cfg.Probe.FailureThreshold,
		SplitBrain:    cfg.Coordinator.Backend,
		AutoSwitch:    true,
		Algorithm:     cfg.Ingress.DNS.Provider,
		AutoBalance:   !strings.EqualFold(cfg.Ingress.Mode, "disabled"),
		Weights:       defaultHaLbWeights(),
	}
}

func defaultFailoverEvents() []clusterFailoverEvent {
	return []clusterFailoverEvent{
		{
			ID:          "fo-1",
			Time:        time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05"),
			Title:       "自动故障转移",
			Detail:      "主节点探测失败 3 次，切换到备用节点",
			Status:      "success",
			Type:        "success",
			StatusLabel: "完成",
		},
		{
			ID:          "fo-2",
			Time:        time.Now().Add(-48 * time.Hour).Format("2006-01-02 15:04:05"),
			Title:       "负载重分配",
			Detail:      "根据实时 QPS 将 15% 流量迁移至 node-c",
			Status:      "success",
			Type:        "warning",
			StatusLabel: "完成",
		},
	}
}

func defaultClusterSyncConfig() clusterSyncConfigDTO {
	return clusterSyncConfigDTO{
		Mode:           "strong",
		Transport:      "tcp",
		LeaseFreq:      "1",
		ConfigFreq:     "5",
		ConflictPolicy: "version",
	}
}

func defaultClusterScalePolicy() clusterScalePolicyDTO {
	return clusterScalePolicyDTO{CPU: 70, Mem: 70, QPS: 8000, MaxNodes: 12}
}

func defaultClusterSyncStatus() []clusterSyncStatusDTO {
	return []clusterSyncStatusDTO{
		{Type: "租约同步", Latency: "12 ms", Last: "20 秒前", Health: "健康"},
		{Type: "配置同步", Latency: "45 ms", Last: "35 秒前", Health: "良好"},
		{Type: "心跳同步", Latency: "5 ms", Last: "10 秒前", Health: "健康"},
	}
}

func defaultScaleEvents() []clusterFailoverEvent {
	return []clusterFailoverEvent{
		{ID: "sc-1", Time: time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05"), Title: "自动扩容", Detail: "QPS 超过 10k，新增 node-d", Status: "success", Type: "success", StatusLabel: "完成"},
		{ID: "sc-2", Time: time.Now().Add(-26 * time.Hour).Format("2006-01-02 15:04:05"), Title: "手动缩容", Detail: "夜间低峰移除 node-e", Status: "success", Type: "warning", StatusLabel: "完成"},
	}
}

func defaultClusterBackupPlan() clusterBackupPlanDTO {
	return clusterBackupPlanDTO{
		Freq:     "daily",
		Retain:   "30",
		Storage:  "hybrid",
		Contents: []string{"配置数据", "租约数据", "日志数据"},
	}
}

func defaultClusterOptimizationPlan() clusterOptimizationPlanDTO {
	return clusterOptimizationPlanDTO{IpDefrag: true, LeaseTune: true, PoolForecast: true}
}

func defaultClusterBackupHistory() []clusterBackupRecordDTO {
	now := time.Now()
	return []clusterBackupRecordDTO{
		{Time: now.Add(-2 * time.Hour).Format("2006-01-02 15:04"), Type: "每日", Size: "320 MB", Status: "成功"},
		{Time: now.Add(-26 * time.Hour).Format("2006-01-02 15:04"), Type: "每日", Size: "318 MB", Status: "成功"},
		{Time: now.Add(-50 * time.Hour).Format("2006-01-02 15:04"), Type: "每日", Size: "319 MB", Status: "成功"},
	}
}

func (s *HTTPServer) currentSyncConfig() clusterSyncConfigDTO {
	s.syncConfigMu.RLock()
	defer s.syncConfigMu.RUnlock()
	return s.syncConfig
}

func (s *HTTPServer) currentSyncTransportMode() string {
	mode := strings.ToLower(strings.TrimSpace(s.currentSyncConfig().Transport))
	if mode == "tcp" {
		return "tcp"
	}
	if mode == "mock" {
		if strings.TrimSpace(s.clusterSyncPeerAddress()) != "" {
			return "tcp"
		}
		return "mock"
	}
	if strings.TrimSpace(s.clusterSyncPeerAddress()) != "" {
		return "tcp"
	}
	return "mock"
}

func (s *HTTPServer) setSyncConfig(cfg clusterSyncConfigDTO) {
	prevTransport := s.currentSyncTransportMode()
	s.syncConfigMu.Lock()
	s.syncConfig = cfg
	s.syncConfigMu.Unlock()

	if prevTransport != s.currentSyncTransportMode() {
		s.refreshClusterSyncResponder()
	}
}

func (s *HTTPServer) currentScalePolicy() clusterScalePolicyDTO {
	s.scalePolicyMu.RLock()
	defer s.scalePolicyMu.RUnlock()
	return s.scalePolicy
}

func (s *HTTPServer) setScalePolicy(cfg clusterScalePolicyDTO) {
	s.scalePolicyMu.Lock()
	defer s.scalePolicyMu.Unlock()
	s.scalePolicy = cfg
}

func (s *HTTPServer) currentSyncStatus() []clusterSyncStatusDTO {
	s.syncStatusMu.RLock()
	defer s.syncStatusMu.RUnlock()
	return append([]clusterSyncStatusDTO{}, s.syncStatus...)
}

func (s *HTTPServer) currentSyncStatusWithJoin() []clusterSyncStatusDTO {
	items := s.currentSyncStatus()
	transport := s.currentSyncTransportMode()
	peerAddress := s.clusterSyncPeerAddress()
	responderUp, responderErr := s.clusterSyncResponderStatus()
	mysqlLagMs, redisOffsetLag := s.currentSyncStorageMetrics()
	ackLatencyMs := parseLatencyMillis(items)
	metricThreshold := s.currentSyncMetricThresholds()
	ackLevel := classifySyncMetricLevel(ackLatencyMs, metricThreshold.ackWarnMs, metricThreshold.ackCriticalMs)
	mysqlLevel := classifySyncMetricLevel(int64(mysqlLagMs), metricThreshold.storageWarnMs, metricThreshold.storageCriticalMs)
	redisLevel := classifySyncMetricLevel(redisOffsetLag, metricThreshold.storageWarnMs, metricThreshold.storageCriticalMs)
	reconcileLastRun, reconcileLastErr := s.syncTransactionsReconcileStatus()
	reconcileSuccessTotal, reconcileFailureTotal, reconcileFailureStreak := s.syncTransactionsReconcileCounters()
	reconcileIntervalSec := int(s.syncTransactionsReconcileInterval() / time.Second)
	reconcileAlertThreshold := s.syncTransactionsReconcileFailureAlertThreshold()
	for i := range items {
		items[i].Transport = transport
		items[i].PeerAddress = peerAddress
		items[i].ResponderUp = responderUp
		items[i].ResponderErr = responderErr
		items[i].MySQLLagMs = mysqlLagMs
		items[i].MySQLLagLevel = mysqlLevel
		items[i].MySQLLagWarnMs = int(metricThreshold.storageWarnMs)
		items[i].MySQLLagCriticalMs = int(metricThreshold.storageCriticalMs)
		items[i].RedisOffsetLag = redisOffsetLag
		items[i].RedisOffsetLagLevel = redisLevel
		items[i].RedisOffsetLagWarn = metricThreshold.storageWarnMs
		items[i].RedisOffsetLagCritical = metricThreshold.storageCriticalMs
		items[i].Rfc6853AckLatencyMs = int(ackLatencyMs)
		items[i].Rfc6853AckLatencyLevel = ackLevel
		items[i].Rfc6853AckWarnMs = int(metricThreshold.ackWarnMs)
		items[i].Rfc6853AckCriticalMs = int(metricThreshold.ackCriticalMs)
		items[i].ReconcileLastRun = reconcileLastRun
		items[i].ReconcileLastErr = reconcileLastErr
		items[i].ReconcileSuccessTotal = reconcileSuccessTotal
		items[i].ReconcileFailureTotal = reconcileFailureTotal
		items[i].ReconcileFailureStreak = reconcileFailureStreak
		items[i].ReconcileIntervalSec = reconcileIntervalSec
		items[i].ReconcileAlertThreshold = reconcileAlertThreshold
	}
	txs := s.recentSyncTransactions(1)
	if len(txs) > 0 {
		tx := txs[0]
		if tx.LatencyMs > 0 {
			ackLatencyMs = int64(tx.LatencyMs)
			ackLevel = classifySyncMetricLevel(ackLatencyMs, metricThreshold.ackWarnMs, metricThreshold.ackCriticalMs)
		}
		if strings.TrimSpace(tx.ErrorCode) != "" {
			ackLevel = "critical"
		}
		for i := range items {
			items[i].LastTxID = tx.TxID
			items[i].Phase = tx.Phase
			items[i].SourceNode = tx.SourceNode
			items[i].TargetNode = tx.TargetNode
			items[i].BndupdAt = tx.BndupdAt
			items[i].BndackAt = tx.BndackAt
			if tx.AckAt != "" {
				items[i].AckAt = tx.AckAt
			} else {
				items[i].AckAt = tx.BndackAt
			}
			if tx.LatencyMs > 0 {
				items[i].Latency = fmt.Sprintf("%d ms", tx.LatencyMs)
			}
			items[i].Rfc6853AckLatencyMs = int(ackLatencyMs)
			items[i].Rfc6853AckLatencyLevel = ackLevel
			if tx.ErrorCode != "" {
				items[i].ErrorCode = tx.ErrorCode
				items[i].ErrorMessage = tx.ErrorMessage
				items[i].Health = "异常"
			}
		}
	}
	source, target := s.resolveSyncEndpoints("", "")
	for i := range items {
		if strings.TrimSpace(items[i].SourceNode) == "" {
			items[i].SourceNode = source
		}
		if strings.TrimSpace(items[i].TargetNode) == "" {
			items[i].TargetNode = target
		}
	}
	if join, ok := s.joinSyncStatus(); ok {
		join.MySQLLagMs = mysqlLagMs
		join.MySQLLagLevel = mysqlLevel
		join.MySQLLagWarnMs = int(metricThreshold.storageWarnMs)
		join.MySQLLagCriticalMs = int(metricThreshold.storageCriticalMs)
		join.RedisOffsetLag = redisOffsetLag
		join.RedisOffsetLagLevel = redisLevel
		join.RedisOffsetLagWarn = metricThreshold.storageWarnMs
		join.RedisOffsetLagCritical = metricThreshold.storageCriticalMs
		join.Rfc6853AckLatencyMs = int(ackLatencyMs)
		join.Rfc6853AckLatencyLevel = ackLevel
		join.Rfc6853AckWarnMs = int(metricThreshold.ackWarnMs)
		join.Rfc6853AckCriticalMs = int(metricThreshold.ackCriticalMs)
		items = append(items, join)
	}
	return items
}

type syncMetricThreshold struct {
	storageWarnMs     int64
	storageCriticalMs int64
	ackWarnMs         int64
	ackCriticalMs     int64
}

func (s *HTTPServer) currentSyncMetricThresholds() syncMetricThreshold {
	ackCritical := int64(defaultSyncAckCriticalMs)
	ackWarn := int64(defaultSyncAckWarnMs)
	if cfg := s.currentHAConfig(); cfg.Replication.AckTimeout > 0 {
		ackCritical = int64(cfg.Replication.AckTimeout / time.Millisecond)
		ackWarn = ackCritical / 2
	}
	if ackWarn < int64(defaultSyncAckWarnMs) {
		ackWarn = int64(defaultSyncAckWarnMs)
	}
	if ackCritical < ackWarn {
		ackCritical = ackWarn
	}

	storageCritical := ackCritical / 4
	storageWarn := storageCritical / 2
	if storageCritical < int64(defaultSyncStorageLagCriticalMs) {
		storageCritical = int64(defaultSyncStorageLagCriticalMs)
	}
	if storageWarn < int64(defaultSyncStorageLagWarnMs) {
		storageWarn = int64(defaultSyncStorageLagWarnMs)
	}

	return syncMetricThreshold{
		storageWarnMs:     storageWarn,
		storageCriticalMs: storageCritical,
		ackWarnMs:         ackWarn,
		ackCriticalMs:     ackCritical,
	}
}

func (s *HTTPServer) currentSyncStorageMetrics() (int, int64) {
	var snapshot failover.StatusSnapshot
	if svc := s.ensureHAService(); svc != nil {
		snapshot = svc.Snapshot()
	} else if s.options.Coordinator != nil {
		snapshot = s.options.Coordinator.Snapshot()
	}
	mysqlLagMs := replicationLagMillis(snapshot.PeerLastSeen)
	if snapshot.Replication.ApplyLagMs > 0 {
		mysqlLagMs = snapshot.Replication.ApplyLagMs
	}
	if mysqlLagMs < 0 {
		mysqlLagMs = 0
	}
	return mysqlLagMs, int64(mysqlLagMs)
}

func parseLatencyMillis(items []clusterSyncStatusDTO) int64 {
	for _, item := range items {
		raw := strings.TrimSpace(strings.ToLower(item.Latency))
		if raw == "" {
			continue
		}
		raw = strings.ReplaceAll(raw, "ms", "")
		raw = strings.TrimSpace(raw)
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

func classifySyncMetricLevel(value, warnThreshold, criticalThreshold int64) string {
	if criticalThreshold > 0 && value >= criticalThreshold {
		return "critical"
	}
	if warnThreshold > 0 && value >= warnThreshold {
		return "warning"
	}
	return "healthy"
}

func (s *HTTPServer) currentBackupPlan() clusterBackupPlanDTO {
	s.backupPlanMu.RLock()
	defer s.backupPlanMu.RUnlock()
	return s.backupPlan
}

func (s *HTTPServer) setBackupPlan(cfg clusterBackupPlanDTO) {
	s.backupPlanMu.Lock()
	defer s.backupPlanMu.Unlock()
	s.backupPlan = cfg
}

func (s *HTTPServer) currentOptimizationPlan() clusterOptimizationPlanDTO {
	s.optimizationPlanMu.RLock()
	defer s.optimizationPlanMu.RUnlock()
	return s.optimizationPlan
}

func (s *HTTPServer) setOptimizationPlan(cfg clusterOptimizationPlanDTO) {
	s.optimizationPlanMu.Lock()
	defer s.optimizationPlanMu.Unlock()
	s.optimizationPlan = cfg
}

func (s *HTTPServer) currentBackupHistory() []clusterBackupRecordDTO {
	s.backupHistoryMu.RLock()
	items := append([]clusterBackupRecordDTO{}, s.backupHistory...)
	s.backupHistoryMu.RUnlock()

	if s == nil || s.opsSvc == nil {
		return items
	}
	jobs, err := s.opsSvc.ListTransferJobs(context.Background(), 50)
	if err != nil {
		return items
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.TxID)
		if key == "" {
			key = strings.TrimSpace(item.Time) + "|" + strings.TrimSpace(item.Type)
		}
		if key != "" {
			seen[key] = struct{}{}
		}
	}

	for _, job := range jobs {
		rec := clusterBackupRecordFromTransfer(job)
		key := strings.TrimSpace(rec.TxID)
		if key == "" {
			key = strings.TrimSpace(rec.Time) + "|" + strings.TrimSpace(rec.Type)
		}
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		items = append(items, rec)
	}

	sort.SliceStable(items, func(i, j int) bool {
		return parseClusterTime(items[i].Time).After(parseClusterTime(items[j].Time))
	})
	if len(items) > 50 {
		items = items[:50]
	}
	return items
}

func (s *HTTPServer) appendBackupRecord(record clusterBackupRecordDTO) clusterBackupRecordDTO {
	if strings.TrimSpace(record.Time) == "" {
		record.Time = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(record.Type) == "" {
		record.Type = "备份"
	}
	if strings.TrimSpace(record.Size) == "" {
		record.Size = "-"
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = "已提交"
	}
	s.backupHistoryMu.Lock()
	defer s.backupHistoryMu.Unlock()
	s.backupHistory = append([]clusterBackupRecordDTO{record}, s.backupHistory...)
	if len(s.backupHistory) > 50 {
		s.backupHistory = s.backupHistory[:50]
	}
	go s.persistBackupHistory(context.Background())
	return record
}

func clusterBackupRecordFromTransfer(job ops.TransferJob) clusterBackupRecordDTO {
	typeLabel := "恢复"
	if job.Kind == ops.TransferKindExport {
		typeLabel = "立即备份"
	}
	status := "执行中"
	phase := string(job.Status)
	switch job.Status {
	case ops.TransferStatusPending:
		status = "已提交"
	case ops.TransferStatusRunning:
		status = "执行中"
	case ops.TransferStatusSucceeded:
		status = "成功"
	case ops.TransferStatusFailed:
		status = "失败"
	}
	rec := clusterBackupRecordDTO{
		Time:      transferRecordTime(job),
		Type:      typeLabel,
		Size:      formatTransferSize(job.SizeBytes),
		Status:    status,
		TxID:      strings.TrimSpace(job.ID),
		Phase:     phase,
		AckAt:     transferAckAt(&job),
		ErrorCode: "",
	}
	if strings.TrimSpace(job.Error) != "" {
		rec.ErrorCode = "BACKUP_TRANSFER_FAILED"
	}
	return rec
}

func transferRecordTime(job ops.TransferJob) string {
	if !job.CreatedAt.IsZero() {
		return job.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !job.StartedAt.IsZero() {
		return job.StartedAt.UTC().Format(time.RFC3339)
	}
	if !job.CompletedAt.IsZero() {
		return job.CompletedAt.UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func parseClusterTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006/1/2 15:04:05", "2006/1/2 15:04"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func durationAbs(v time.Duration) time.Duration {
	if v < 0 {
		return -v
	}
	return v
}

func (s *HTTPServer) reconcileHAConfigFromClusterNodes(ctx context.Context) {
	if s == nil {
		return
	}
	nodes := s.customClusterNodes()
	activeID := ""
	standbyAddr := ""
	for _, node := range nodes {
		if node.Disabled {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(node.Role))
		address := strings.TrimSpace(node.Address)
		if role == "active" && activeID == "" {
			activeID = firstNonEmpty(strings.TrimSpace(node.ID), address)
		}
		if role == "standby" && standbyAddr == "" {
			standbyAddr = firstNonEmpty(address, strings.TrimSpace(node.ID))
		}
	}

	cfg := s.currentHAConfig()
	changed := false
	if activeID != "" && cfg.Node.ID != activeID {
		cfg.Node.ID = activeID
		changed = true
	}
	if cfg.Partner.Address != standbyAddr {
		cfg.Partner.Address = standbyAddr
		cfg.Partner.Enabled = standbyAddr != ""
		changed = true
	}
	if !changed {
		return
	}
	s.setHAConfig(cfg)
	s.persistClusterConfig(ctx, clusterConfigKeyHA, s.toClusterHaConfigDTO(cfg))
}

func (s *HTTPServer) upsertFailoverMembership(ctx context.Context, node clusterOverviewNode) error {
	if s == nil {
		return nil
	}
	svc := s.ensureHAService()
	if svc == nil {
		return nil
	}
	role := strings.ToLower(strings.TrimSpace(node.Role))
	if role != "active" && role != "standby" {
		return nil
	}
	reg := failover.NodeRegistration{
		ID:       strings.TrimSpace(node.ID),
		Address:  strings.TrimSpace(node.Address),
		Region:   "",
		Zone:     "",
		Weight:   0,
		Disabled: node.Disabled,
		Role:     failover.RoleStandby,
	}
	if role == "active" {
		reg.Role = failover.RolePrimary
	}
	if reg.ID == "" {
		reg.ID = reg.Address
	}
	if reg.ID == "" {
		return nil
	}
	err := svc.UpsertNode(ctx, reg)
	if err != nil {
		s.appendScaleEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("node-membership-upsert-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "动态成员写入失败",
			Detail:      fmt.Sprintf("节点 %s 成员写入失败：%v", reg.ID, err),
			Status:      "failed",
			Type:        "danger",
			StatusLabel: "失败",
			ErrorCode:   "MEMBERSHIP_UPSERT_FAILED",
		})
		return err
	}
	s.appendScaleEvent(clusterFailoverEvent{
		ID:          fmt.Sprintf("node-membership-upsert-ok-%d", time.Now().UnixNano()),
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "动态成员写入",
		Detail:      fmt.Sprintf("节点 %s 成员已写入（role=%s, disabled=%t）", reg.ID, reg.Role, reg.Disabled),
		Status:      "success",
		Type:        "info",
		StatusLabel: "完成",
		ErrorCode:   "",
	})
	return nil
}

func (s *HTTPServer) removeFailoverMembership(ctx context.Context, nodeID string) error {
	if s == nil {
		return nil
	}
	svc := s.ensureHAService()
	if svc == nil {
		return nil
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	err := svc.RemoveNode(ctx, nodeID)
	if err != nil {
		s.appendScaleEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("node-membership-remove-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "动态成员删除失败",
			Detail:      fmt.Sprintf("节点 %s 成员删除失败：%v", nodeID, err),
			Status:      "failed",
			Type:        "danger",
			StatusLabel: "失败",
			ErrorCode:   "MEMBERSHIP_REMOVE_FAILED",
		})
		return err
	}
	s.appendScaleEvent(clusterFailoverEvent{
		ID:          fmt.Sprintf("node-membership-remove-ok-%d", time.Now().UnixNano()),
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "动态成员删除",
		Detail:      fmt.Sprintf("节点 %s 成员已删除", nodeID),
		Status:      "success",
		Type:        "warning",
		StatusLabel: "完成",
		ErrorCode:   "",
	})
	return nil
}

func (s *HTTPServer) syncClusterNodeFromHAMember(ctx context.Context, reg failover.NodeRegistration) {
	if s == nil {
		return
	}
	id := strings.TrimSpace(reg.ID)
	if id == "" {
		id = strings.TrimSpace(reg.Address)
	}
	if id == "" {
		return
	}
	role := "standby"
	if reg.Role == failover.RolePrimary {
		role = "active"
	}
	node := clusterOverviewNode{
		ID:            id,
		Address:       firstNonEmpty(strings.TrimSpace(reg.Address), id),
		Role:          role,
		Disabled:      reg.Disabled,
		Version:       clusterNodeCurrentVersion,
		Health:        "healthy",
		CPUPercent:    0,
		MemoryPercent: 0,
		SyncLagMs:     0,
		LastHeartbeat: time.Now().UTC().Format(time.RFC3339),
	}
	if current, ok := s.customClusterNodeByID(id); ok {
		node.Version = normalizeClusterNodeVersion(firstNonEmpty(strings.TrimSpace(current.Version), node.Version))
		node.Health = firstNonEmpty(strings.TrimSpace(current.Health), node.Health)
		node.CPUPercent = current.CPUPercent
		node.MemoryPercent = current.MemoryPercent
		node.SyncLagMs = current.SyncLagMs
		node.LastHeartbeat = firstNonEmpty(strings.TrimSpace(current.LastHeartbeat), node.LastHeartbeat)
	}
	s.setCustomClusterNode(node)
	s.persistClusterNodes(ctx)
	s.reconcileHAConfigFromClusterNodes(ctx)
}

func (s *HTTPServer) removeClusterNodeForHAMember(ctx context.Context, nodeID string) {
	if s == nil {
		return
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return
	}
	if !s.deleteCustomClusterNode(nodeID) {
		return
	}
	s.persistClusterNodes(ctx)
	s.reconcileHAConfigFromClusterNodes(ctx)
	s.cancelNodeJoinJobs(nodeID)
}

func (s *HTTPServer) currentScaleEvents() []clusterFailoverEvent {
	s.scaleEventsMu.RLock()
	defer s.scaleEventsMu.RUnlock()
	return append([]clusterFailoverEvent{}, s.scaleEvents...)
}

func (s *HTTPServer) appendScaleEvent(ev clusterFailoverEvent) {
	s.scaleEventsMu.Lock()
	defer s.scaleEventsMu.Unlock()
	s.scaleEvents = append([]clusterFailoverEvent{ev}, s.scaleEvents...)
	if len(s.scaleEvents) > 50 {
		s.scaleEvents = s.scaleEvents[:50]
	}
}

func (s *HTTPServer) updateScaleEvent(id, status, statusLabel, eventType, detail string) {
	s.scaleEventsMu.Lock()
	defer s.scaleEventsMu.Unlock()
	for i := range s.scaleEvents {
		if s.scaleEvents[i].ID != id {
			continue
		}
		if status != "" {
			s.scaleEvents[i].Status = status
		}
		if statusLabel != "" {
			s.scaleEvents[i].StatusLabel = statusLabel
		}
		if eventType != "" {
			s.scaleEvents[i].Type = eventType
		}
		if detail != "" {
			s.scaleEvents[i].Detail = detail
		}
		return
	}
}

func (s *HTTPServer) currentHaLbConfig() clusterHaLbConfigDTO {
	s.haLbConfigMu.RLock()
	defer s.haLbConfigMu.RUnlock()
	return s.haLbConfig
}

func (s *HTTPServer) setHaLbConfig(cfg clusterHaLbConfigDTO) {
	s.haLbConfigMu.Lock()
	defer s.haLbConfigMu.Unlock()
	s.haLbConfig = cfg
}

func (s *HTTPServer) failoverHistory() []clusterFailoverEvent {
	s.failoverEventsMu.RLock()
	defer s.failoverEventsMu.RUnlock()
	return append([]clusterFailoverEvent{}, s.failoverEvents...)
}

func (s *HTTPServer) appendFailoverEvent(ev clusterFailoverEvent) {
	s.failoverEventsMu.Lock()
	defer s.failoverEventsMu.Unlock()
	s.failoverEvents = append([]clusterFailoverEvent{ev}, s.failoverEvents...)
	if len(s.failoverEvents) > 50 {
		s.failoverEvents = s.failoverEvents[:50]
	}
}

func (s *HTTPServer) currentHAConfig() config.HAConfig {
	s.haConfigMu.RLock()
	defer s.haConfigMu.RUnlock()
	return s.haConfig
}

func (s *HTTPServer) setHAConfig(cfg config.HAConfig) {
	s.haConfigMu.Lock()
	defer s.haConfigMu.Unlock()
	s.haConfig = cfg
}

func (s *HTTPServer) toClusterHaConfigDTO(cfg config.HAConfig) clusterHaConfigDTO {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	standby := []string{}
	if cfg.Partner.Address != "" {
		standby = append(standby, cfg.Partner.Address)
	}
	mechanism := "binlog"
	if cfg.Replication.CDCEnabled {
		mechanism = "cdc"
	}
	return clusterHaConfigDTO{
		Mode:         mode,
		Primary:      strings.TrimSpace(cfg.Node.ID),
		StandbyNodes: standby,
		LoadBalancing: clusterHaLoadBalancing{
			Enabled:   !strings.EqualFold(cfg.Ingress.Mode, "disabled") && strings.TrimSpace(cfg.Ingress.Mode) != "",
			VirtualIP: strings.TrimSpace(cfg.Ingress.VIPAddress),
			Balancer:  strings.TrimSpace(cfg.Ingress.Mode),
			Method:    strings.TrimSpace(cfg.Ingress.DNS.Provider),
		},
		Failover: clusterHaFailover{
			IntervalMs:       int(cfg.HeartbeatInterval / time.Millisecond),
			TimeoutMs:        int(cfg.FailoverTimeout / time.Millisecond),
			FailureThreshold: cfg.Probe.FailureThreshold,
		},
		Replication: clusterHaReplication{
			Mechanism:           mechanism,
			SnapshotIntervalSec: int(cfg.Replication.SnapshotInterval / time.Second),
			LagAlertMs:          int(cfg.Replication.AckTimeout / time.Millisecond),
			SyncMode:            strings.TrimSpace(cfg.ConfigSync.Backend),
		},
		SplitBrain: clusterHaSplitBrain{
			Strategy:    strings.TrimSpace(cfg.Coordinator.Backend),
			Arbiter:     strings.TrimSpace(cfg.Coordinator.Key),
			FenceScript: strings.TrimSpace(cfg.ConfigSync.Path),
			SharedLock:  strings.TrimSpace(cfg.Coordinator.Token),
		},
		Recovery: clusterHaRecovery{
			ManualAction: strings.TrimSpace(cfg.Failback.Mode),
		},
	}
}

func (s *HTTPServer) fromClusterHaConfigDTO(dto clusterHaConfigDTO) config.HAConfig {
	cfg := s.currentHAConfig()
	cfg.Mode = strings.TrimSpace(dto.Mode)
	cfg.Node.ID = strings.TrimSpace(dto.Primary)
	if len(dto.StandbyNodes) > 0 {
		cfg.Partner.Enabled = true
		cfg.Partner.Address = strings.TrimSpace(dto.StandbyNodes[0])
	} else {
		cfg.Partner.Enabled = false
		cfg.Partner.Address = ""
	}
	cfg.HeartbeatInterval = time.Duration(dto.Failover.IntervalMs) * time.Millisecond
	cfg.FailoverTimeout = time.Duration(dto.Failover.TimeoutMs) * time.Millisecond
	cfg.Probe.FailureThreshold = dto.Failover.FailureThreshold
	cfg.Replication.CDCEnabled = strings.EqualFold(dto.Replication.Mechanism, "cdc")
	cfg.Replication.SnapshotInterval = time.Duration(dto.Replication.SnapshotIntervalSec) * time.Second
	cfg.Replication.AckTimeout = time.Duration(dto.Replication.LagAlertMs) * time.Millisecond
	cfg.ConfigSync.Backend = strings.TrimSpace(dto.Replication.SyncMode)
	cfg.Ingress.VIPAddress = strings.TrimSpace(dto.LoadBalancing.VirtualIP)
	cfg.Ingress.Mode = strings.TrimSpace(dto.LoadBalancing.Balancer)
	cfg.Ingress.DNS.Provider = strings.TrimSpace(dto.LoadBalancing.Method)
	if !dto.LoadBalancing.Enabled {
		cfg.Ingress.Mode = "disabled"
	}
	cfg.Coordinator.Backend = strings.TrimSpace(dto.SplitBrain.Strategy)
	cfg.Coordinator.Key = strings.TrimSpace(dto.SplitBrain.Arbiter)
	cfg.Coordinator.Token = strings.TrimSpace(dto.SplitBrain.SharedLock)
	cfg.ConfigSync.Path = strings.TrimSpace(dto.SplitBrain.FenceScript)
	cfg.Failback.Mode = strings.TrimSpace(dto.Recovery.ManualAction)
	return cfg
}

func normalizeClusterMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "active-active":
		return "active-active"
	case "active-passive":
		return "active-passive"
	default:
		return ""
	}
}

func deriveClusterMode(nodes []failover.NodeStatus, snapshot failover.StatusSnapshot) string {
	primaryCount := 0
	for _, node := range nodes {
		if node.Role == failover.RolePrimary {
			primaryCount++
		}
	}
	if primaryCount > 1 {
		return "active-active"
	}
	if len(nodes) > 1 {
		return "active-passive"
	}
	if snapshot.Role == failover.RolePrimary || snapshot.Role == failover.RoleStandby {
		return "active-passive"
	}
	return "active-passive"
}

func clusterFailoverReady(snapshot failover.StatusSnapshot) bool {
	if snapshot.State == "" {
		return true
	}
	return snapshot.State == failover.StateNormal && !snapshot.ManualFailbackSet
}

func replicationLagMillis(lastSeen time.Time) int {
	if lastSeen.IsZero() {
		return 0
	}
	lag := time.Since(lastSeen)
	if lag < 0 {
		lag = 0
	}
	return int(lag / time.Millisecond)
}

func buildClusterNodes(nodes []failover.NodeStatus, snapshot failover.StatusSnapshot, health *monitoring.SystemHealthSnapshot) []clusterOverviewNode {
	result := make([]clusterOverviewNode, 0, len(nodes))
	if len(nodes) == 0 {
		if snapshot.Role == "" {
			return result
		}
		nodes = append(nodes, failover.NodeStatus{
			ID:            "controller",
			Role:          snapshot.Role,
			State:         snapshot.State,
			Self:          true,
			LastHeartbeat: time.Now().UTC(),
			PeerLastSeen:  snapshot.PeerLastSeen,
		})
	}
	now := time.Now().UTC()
	for _, node := range nodes {
		cpu := 0.0
		mem := 0.0
		if node.Self && health != nil {
			cpu = health.CPUPercent
			mem = health.MemoryPercent
		}
		syncLag := 0
		if !node.PeerLastSeen.IsZero() {
			lag := now.Sub(node.PeerLastSeen)
			if lag < 0 {
				lag = 0
			}
			syncLag = int(lag / time.Millisecond)
		}
		heartbeat := ""
		if !node.LastHeartbeat.IsZero() {
			heartbeat = node.LastHeartbeat.UTC().Format(time.RFC3339)
		}
		result = append(result, clusterOverviewNode{
			ID:            clusterNodeID(node),
			Role:          clusterRole(node.Role),
			Address:       clusterAddress(node),
			Self:          node.Self,
			Version:       clusterNodeCurrentVersion,
			Health:        clusterHealth(node),
			Disabled:      false,
			CPUPercent:    cpu,
			MemoryPercent: mem,
			SyncLagMs:     syncLag,
			LastHeartbeat: heartbeat,
		})
	}
	return result
}

func normalizeClusterNodeVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || strings.EqualFold(v, "unknown") {
		return clusterNodeCurrentVersion
	}
	if strings.EqualFold(v, "1.3.5") || strings.EqualFold(v, "v1.3.5") {
		return clusterNodeCurrentVersion
	}
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		return "v" + strings.TrimSpace(v[1:])
	}
	return "v" + v
}

func clusterNodeID(node failover.NodeStatus) string {
	if strings.TrimSpace(node.ID) != "" {
		return node.ID
	}
	if node.Self {
		return "self"
	}
	return "peer"
}

func clusterRole(role failover.Role) string {
	switch role {
	case failover.RolePrimary:
		return "active"
	case failover.RoleStandby:
		return "standby"
	default:
		return "worker"
	}
}

func clusterAddress(node failover.NodeStatus) string {
	parts := []string{}
	if strings.TrimSpace(node.Region) != "" {
		parts = append(parts, strings.TrimSpace(node.Region))
	}
	if strings.TrimSpace(node.Zone) != "" {
		parts = append(parts, strings.TrimSpace(node.Zone))
	}
	if len(parts) == 0 {
		return clusterNodeID(node)
	}
	return strings.Join(parts, "/")
}

func clusterHealth(node failover.NodeStatus) string {
	if len(node.Warnings) > 0 {
		return "warning"
	}
	switch node.State {
	case failover.StatePartnerDown:
		return "critical"
	case failover.StateCommInterrupted:
		return "warning"
	case failover.StateNormal, failover.StateInit:
		return "healthy"
	default:
		return "warning"
	}
}

func countClusterPendingActions(snapshot failover.StatusSnapshot, nodes []clusterOverviewNode) int {
	pending := 0
	if snapshot.ManualFailbackSet {
		pending++
	}
	for _, node := range nodes {
		if node.Disabled {
			pending++
		}
		if node.Health == "warning" || node.Health == "critical" {
			pending++
		}
	}
	if snapshot.State == failover.StateCommInterrupted || snapshot.State == failover.StatePartnerDown {
		pending++
	}
	return pending
}

func (s *HTTPServer) recentSyncTransactions(limit int) []clusterSyncTransactionDTO {
	s.syncTransactionsMu.RLock()
	defer s.syncTransactionsMu.RUnlock()
	if limit <= 0 || limit > len(s.syncTransactions) {
		limit = len(s.syncTransactions)
	}
	result := make([]clusterSyncTransactionDTO, limit)
	copy(result, s.syncTransactions[:limit])
	return result
}

func (s *HTTPServer) buildSyncStats(hours int) clusterSyncStatsDTO {
	if hours <= 0 {
		hours = 24
	}
	windowStart := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	txs := s.recentSyncTransactions(0)
	stats := clusterSyncStatsDTO{WindowHours: hours}
	latencies := make([]int, 0, len(txs))

	for _, tx := range txs {
		createdAt, err := time.Parse(time.RFC3339, tx.CreatedAt)
		if err != nil || createdAt.Before(windowStart) {
			continue
		}
		stats.Total++
		phase := strings.ToLower(strings.TrimSpace(tx.Phase))
		if strings.Contains(phase, "commit") {
			stats.Committed++
		}
		if strings.Contains(phase, "fail") {
			stats.Failed++
		}
		if strings.EqualFold(strings.TrimSpace(tx.ErrorCode), "ACK_TIMEOUT") {
			stats.AckTimeoutCount++
		}
		if tx.LatencyMs > 0 {
			latencies = append(latencies, tx.LatencyMs)
		}
	}

	if stats.Total > 0 {
		stats.SuccessRatePercent = (float64(stats.Committed) / float64(stats.Total)) * 100
	}
	if len(latencies) > 0 {
		sum := 0
		for _, v := range latencies {
			sum += v
		}
		stats.AvgAckLatencyMs = int(math.Round(float64(sum) / float64(len(latencies))))
		sort.Ints(latencies)
		idx := int(math.Ceil(0.95*float64(len(latencies)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		stats.P95AckLatencyMs = latencies[idx]
	}

	return stats
}

func (s *HTTPServer) syncTransactionByID(txID string) (clusterSyncTransactionDTO, bool) {
	s.syncTransactionsMu.RLock()
	defer s.syncTransactionsMu.RUnlock()
	for _, tx := range s.syncTransactions {
		if tx.TxID == txID {
			return tx, true
		}
	}
	return clusterSyncTransactionDTO{}, false
}

func (s *HTTPServer) snapshotSyncTransactions() []clusterSyncTransactionDTO {
	s.syncTransactionsMu.RLock()
	defer s.syncTransactionsMu.RUnlock()
	result := make([]clusterSyncTransactionDTO, len(s.syncTransactions))
	copy(result, s.syncTransactions)
	return result
}

func (s *HTTPServer) snapshotSyncTransactionsArchive() []clusterSyncTransactionDTO {
	s.syncTransactionsMu.RLock()
	defer s.syncTransactionsMu.RUnlock()
	result := make([]clusterSyncTransactionDTO, len(s.syncTransactionsArchive))
	copy(result, s.syncTransactionsArchive)
	return result
}

func (s *HTTPServer) persistSyncTransactions(ctx context.Context) {
	_ = s.persistSyncTransactionsErr(ctx)
}

func (s *HTTPServer) persistSyncTransactionsErr(ctx context.Context) error {
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeySyncTransactions, s.snapshotSyncTransactions()); err != nil {
		return err
	}
	return s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeySyncTransactionsArchive, s.snapshotSyncTransactionsArchive())
}

func (s *HTTPServer) restoreSyncTransactions(txs []clusterSyncTransactionDTO) {
	txs = sanitizeSyncTransactions(txs)
	s.syncTransactionsMu.Lock()
	defer s.syncTransactionsMu.Unlock()
	s.syncTransactions = append([]clusterSyncTransactionDTO{}, txs...)
	s.applySyncTransactionsPolicyLocked(time.Now().UTC())
}

func (s *HTTPServer) restoreSyncTransactionsArchive(txs []clusterSyncTransactionDTO) {
	txs = sanitizeSyncTransactions(txs)
	limit := s.syncTransactionsArchiveLimit()
	s.syncTransactionsMu.Lock()
	defer s.syncTransactionsMu.Unlock()
	merged := append(append([]clusterSyncTransactionDTO{}, txs...), s.syncTransactionsArchive...)
	merged = sanitizeSyncTransactions(merged)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	s.syncTransactionsArchive = merged
}

func sanitizeSyncTransactions(txs []clusterSyncTransactionDTO) []clusterSyncTransactionDTO {
	if len(txs) == 0 {
		return nil
	}
	result := make([]clusterSyncTransactionDTO, 0, len(txs))
	seen := make(map[string]struct{}, len(txs))
	for _, tx := range txs {
		txID := strings.TrimSpace(tx.TxID)
		if txID == "" {
			continue
		}
		if _, ok := seen[txID]; ok {
			continue
		}
		tx.TxID = txID
		seen[txID] = struct{}{}
		result = append(result, tx)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (s *HTTPServer) syncTransactionsMaxEntries() int {
	if s == nil {
		return 100
	}
	limit := s.options.HAConfig.Replication.SyncTxnMaxEntries
	if limit <= 0 {
		return 100
	}
	return limit
}

func (s *HTTPServer) syncTransactionsRetention() time.Duration {
	if s == nil {
		return 0
	}
	retention := s.options.HAConfig.Replication.SyncTxnRetention
	if retention <= 0 {
		return 0
	}
	return retention
}

func (s *HTTPServer) syncTransactionsArchiveLimit() int {
	if s == nil {
		return 500
	}
	limit := s.options.HAConfig.Replication.SyncTxnArchiveLimit
	if limit <= 0 {
		return 500
	}
	return limit
}

func (s *HTTPServer) applySyncTransactionsPolicyLocked(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	maxEntries := s.syncTransactionsMaxEntries()
	retention := s.syncTransactionsRetention()
	archiveLimit := s.syncTransactionsArchiveLimit()

	active := make([]clusterSyncTransactionDTO, 0, len(s.syncTransactions))
	dropped := make([]clusterSyncTransactionDTO, 0)
	cutoff := now.Add(-retention)

	for _, tx := range s.syncTransactions {
		if retention > 0 {
			if createdAt, err := time.Parse(time.RFC3339, tx.CreatedAt); err == nil && createdAt.Before(cutoff) {
				dropped = append(dropped, tx)
				continue
			}
		}
		if len(active) < maxEntries {
			active = append(active, tx)
			continue
		}
		dropped = append(dropped, tx)
	}

	s.syncTransactions = active
	if len(dropped) == 0 {
		if len(s.syncTransactionsArchive) > archiveLimit {
			s.syncTransactionsArchive = s.syncTransactionsArchive[:archiveLimit]
		}
		return
	}
	s.syncTransactionsArchive = append(dropped, s.syncTransactionsArchive...)
	if len(s.syncTransactionsArchive) > archiveLimit {
		s.syncTransactionsArchive = s.syncTransactionsArchive[:archiveLimit]
	}
}

func (s *HTTPServer) appendSyncTransaction(tx clusterSyncTransactionDTO) {
	s.syncTransactionsMu.Lock()
	s.syncTransactions = append([]clusterSyncTransactionDTO{tx}, s.syncTransactions...)
	s.applySyncTransactionsPolicyLocked(time.Now().UTC())
	s.syncTransactionsMu.Unlock()
	s.persistSyncTransactions(context.Background())
}

func (s *HTTPServer) upsertSyncTransaction(tx clusterSyncTransactionDTO) bool {
	updated := false
	s.syncTransactionsMu.Lock()
	for i := range s.syncTransactions {
		if s.syncTransactions[i].TxID != tx.TxID {
			continue
		}
		s.syncTransactions[i] = tx
		updated = true
		break
	}
	s.applySyncTransactionsPolicyLocked(time.Now().UTC())
	s.syncTransactionsMu.Unlock()
	if updated {
		s.persistSyncTransactions(context.Background())
	}
	return updated
}

func (s *HTTPServer) completeSyncTransaction(txID string) {
	s.completeSyncTransactionWithLatency(txID, 0)
}

func (s *HTTPServer) completeSyncTransactionWithLatency(txID string, latency time.Duration) {
	now := time.Now().UTC()
	changed := false

	s.syncTransactionsMu.Lock()
	for i := range s.syncTransactions {
		if s.syncTransactions[i].TxID != txID {
			continue
		}
		changed = true
		s.syncTransactions[i].Phase = "committed"
		s.syncTransactions[i].BndackAt = now.Format(time.RFC3339)
		s.syncTransactions[i].AckAt = now.Format(time.RFC3339)
		s.syncTransactions[i].CommitAt = now.Format(time.RFC3339)
		if latency > 0 {
			s.syncTransactions[i].LatencyMs = int(latency / time.Millisecond)
		} else {
			start, err := time.Parse(time.RFC3339, s.syncTransactions[i].CreatedAt)
			if err == nil {
				lag := now.Sub(start)
				if lag < 0 {
					lag = 0
				}
				s.syncTransactions[i].LatencyMs = int(lag / time.Millisecond)
			}
		}
		break
	}
	s.applySyncTransactionsPolicyLocked(now)
	s.syncTransactionsMu.Unlock()
	if changed {
		s.persistSyncTransactions(context.Background())
	}

	s.syncStatusMu.Lock()
	for i := range s.syncStatus {
		health := strings.ToLower(strings.TrimSpace(s.syncStatus[i].Health))
		if strings.Contains(health, "critical") || strings.Contains(health, "异常") {
			continue
		}
		s.syncStatus[i].Health = "健康"
		s.syncStatus[i].Last = "刚刚"
	}
	s.syncStatusMu.Unlock()

	s.updateScaleEvent(txID, "success", "完成", "success", "BNDUPD/BNDACK 两阶段提交完成")
}

func (s *HTTPServer) failSyncTransaction(txID, code, message string) {
	now := time.Now().UTC()
	changed := false

	s.syncTransactionsMu.Lock()
	for i := range s.syncTransactions {
		if s.syncTransactions[i].TxID != txID {
			continue
		}
		changed = true
		s.syncTransactions[i].Phase = "failed"
		s.syncTransactions[i].ErrorCode = code
		s.syncTransactions[i].ErrorMessage = message
		start, err := time.Parse(time.RFC3339, s.syncTransactions[i].CreatedAt)
		if err == nil {
			lag := now.Sub(start)
			if lag < 0 {
				lag = 0
			}
			s.syncTransactions[i].LatencyMs = int(lag / time.Millisecond)
		}
		break
	}
	s.applySyncTransactionsPolicyLocked(now)
	s.syncTransactionsMu.Unlock()
	if changed {
		s.persistSyncTransactions(context.Background())
	}

	s.syncStatusMu.Lock()
	for i := range s.syncStatus {
		s.syncStatus[i].Health = "异常"
		s.syncStatus[i].Last = "刚刚"
	}
	s.syncStatusMu.Unlock()

	s.updateScaleEvent(txID, "failed", "失败", "danger", fmt.Sprintf("同步失败：%s(%s)", message, code))
}

func (s *HTTPServer) handleSecurityOverview() echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := s.leaseScopeRef(c)
		tenantID := strings.TrimSpace(scope.TenantOrDefault())
		if tenantID == "" {
			tenantID = systemTenantID
			scope = scope.WithTenantOverride(systemTenantID)
		}

		response := securityOverviewResponse{
			Controls:       []securityControlState{},
			RecentFindings: []securityAlertEntry{},
		}

		var security monitoring.SecuritySnapshot
		if s.monitor != nil {
			security = s.monitor.Security(scope)
		}

		response.RateLimitHits = security.RateLimit.TotalHits
		response.SnoopingViolations = security.Snooping.Untrusted + security.Snooping.Misses
		response.RogueServers = security.Snooping.Untrusted

		response.Controls = append(response.Controls, buildRateLimitControl(security.RateLimit))
		response.Controls = append(response.Controls, buildSnoopingControl(security.Snooping))
		response.Controls = append(response.Controls, buildRogueControl(security.Snooping))

		if s.alertFeed != nil {
			snapshot := s.alertFeed.Snapshot(tenantID, 5)
			for _, entry := range snapshot.Alerts {
				response.RecentFindings = append(response.RecentFindings, mapSecurityAlert(entry))
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}

func buildRateLimitControl(window monitoring.RateLimitWindow) securityControlState {
	status, severity := classifyRateLimit(window.TotalHits)
	return securityControlState{
		ID:          "rate-limit",
		Name:        "端口速率限制",
		Status:      status,
		Severity:    severity,
		LastEventAt: formatIfSet(window.LastHit),
	}
}

func buildSnoopingControl(window monitoring.SnoopingWindow) securityControlState {
	status, severity := classifySnooping(window)
	return securityControlState{
		ID:          "snooping",
		Name:        "DHCP Snooping",
		Status:      status,
		Severity:    severity,
		LastEventAt: formatIfSet(window.LastEvent),
	}
}

func buildRogueControl(window monitoring.SnoopingWindow) securityControlState {
	status, severity := classifyRogue(window.Untrusted)
	return securityControlState{
		ID:          "rogue-detection",
		Name:        "非法服务器扫描",
		Status:      status,
		Severity:    severity,
		LastEventAt: formatIfSet(window.LastEvent),
	}
}

func classifyRateLimit(totalHits int) (string, string) {
	switch {
	case totalHits > 500:
		return "degraded", "critical"
	case totalHits > 100:
		return "enabled", "warning"
	case totalHits > 0:
		return "enabled", "info"
	default:
		return "enabled", "info"
	}
}

func classifySnooping(window monitoring.SnoopingWindow) (string, string) {
	switch {
	case window.Errors > 0:
		return "degraded", "critical"
	case window.Untrusted > 10:
		return "degraded", "warning"
	case window.Untrusted+window.Misses > 0:
		return "enabled", "info"
	default:
		return "enabled", "info"
	}
}

func classifyRogue(untrusted int) (string, string) {
	switch {
	case untrusted > 5:
		return "degraded", "critical"
	case untrusted > 0:
		return "enabled", "warning"
	default:
		return "enabled", "info"
	}
}

func formatIfSet(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func mapSecurityAlert(entry monitoring.AlertFeedEntry) securityAlertEntry {
	alert := securityAlertEntry{
		ID:          entry.ID,
		Summary:     entry.Summary,
		Details:     entry.Details,
		Category:    entry.Category,
		Severity:    normalizeAlertSeverity(entry.Severity),
		Lifecycle:   string(entry.Lifecycle),
		Source:      entry.Source,
		TenantID:    entry.TenantID,
		Fingerprint: entry.Fingerprint,
		Tags:        append([]string(nil), entry.Tags...),
		CreatedAt:   formatIfSet(entry.CreatedAt),
		UpdatedAt:   formatIfSet(entry.UpdatedAt),
	}
	if alert.CreatedAt == "" {
		alert.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if alert.UpdatedAt == "" {
		alert.UpdatedAt = alert.CreatedAt
	}
	if entry.Assignee != "" {
		alert.Assignee = entry.Assignee
	}
	if entry.Channel != "" {
		alert.Channel = entry.Channel
	}
	return alert
}

func normalizeAlertSeverity(sev alerting.Severity) string {
	switch sev {
	case alerting.SeverityCritical, alerting.SeverityMajor:
		return "critical"
	case alerting.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

func (s *HTTPServer) handleDHCPOptionCatalog() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "dhcp option catalog unavailable")
		}

		response := dhcpOptionCatalogResponse{
			Standard:  defaultStandardDHCPOptions(),
			Custom:    []dhcpOptionDefinitionResponse{},
			Templates: []dhcpOptionTemplateSummary{},
		}

		items := s.optionStore.list()
		if len(items) > 0 {
			response.Custom = make([]dhcpOptionDefinitionResponse, 0, len(items))
			response.Templates = make([]dhcpOptionTemplateSummary, 0, len(items))
		}
		for _, item := range items {
			response.Custom = append(response.Custom, dhcpOptionDefinitionResponse{
				Name:        item.Name,
				Code:        item.Code,
				Category:    strings.ToLower(item.Scope),
				Description: item.Description,
				UsageCount:  maxInt(1, len(item.Tags)*3),
			})
			description := firstNonEmpty(item.Description, "自定义 DHCP 模板")
			response.Templates = append(response.Templates, dhcpOptionTemplateSummary{
				Name:        item.Name,
				Options:     1,
				UsedBy:      maxInt(1, len(item.Tags)),
				Description: description,
			})
		}
		sort.Slice(response.Custom, func(i, j int) bool {
			if strings.EqualFold(response.Custom[i].Name, response.Custom[j].Name) {
				return response.Custom[i].Code < response.Custom[j].Code
			}
			return strings.ToLower(response.Custom[i].Name) < strings.ToLower(response.Custom[j].Name)
		})
		sort.Slice(response.Templates, func(i, j int) bool {
			return strings.ToLower(response.Templates[i].Name) < strings.ToLower(response.Templates[j].Name)
		})

		return c.JSON(http.StatusOK, response)
	}
}

func (s *HTTPServer) handleMaintenanceOverview() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		ctx := c.Request().Context()
		plan := s.opsSvc.MaintenancePlan(ctx)
		wizard := s.opsSvc.BackupWizard(ctx)
		summary, err := s.opsSvc.SystemSummary(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		scope := s.leaseScopeRef(c)
		tenantID := strings.TrimSpace(scope.TenantOrDefault())
		if tenantID == "" {
			tenantID = systemTenantID
			scope = scope.WithTenantOverride(systemTenantID)
		}

		overview := maintenanceOverviewResponse{
			BackupsEnabled: wizard.Enabled,
			BackupWindow:   firstNonEmpty(formatBackupWindow(wizard.Schedules), summary.MaintenanceWindow),
			LastBackupAt:   formatIfSet(wizard.GeneratedAt),
			Tasks:          buildMaintenanceTasks(plan),
			Forecasts:      s.buildCapacityForecasts(ctx, scope),
		}
		if overview.LastBackupAt == "" {
			overview.LastBackupAt = formatIfSet(plan.GeneratedAt)
		}
		return c.JSON(http.StatusOK, overview)
	}
}

func (s *HTTPServer) handleHelpCenterSnapshot() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		ctx := c.Request().Context()
		info := s.opsSvc.HelpCenter(ctx)
		resources := buildSupportResources(info)
		directory := s.opsSvc.SupportDirectory()
		openTickets := 0
		if directory.Enabled {
			openTickets = len(directory.Contacts)
			if openTickets == 0 && strings.TrimSpace(directory.TicketURL) != "" {
				openTickets = 1
			}
		}
		latestVersion, highlights := latestRelease(info.Releases)
		snapshot := helpCenterSnapshotResponse{
			Docs:              resources,
			OpenTickets:       openTickets,
			LatestVersion:     latestVersion,
			ReleaseHighlights: highlights,
		}
		return c.JSON(http.StatusOK, snapshot)
	}
}

func (s *HTTPServer) handleIntegrationsOverview() echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := s.leaseScopeRef(c)
		tenantID := strings.TrimSpace(scope.TenantOrDefault())
		if tenantID == "" {
			tenantID = systemTenantID
			scope = scope.WithTenantOverride(systemTenantID)
		}

		adapters := map[string]*integrationAdapterResponse{}
		var routesSnapshot alerting.RoutingSnapshot
		if s.alertRoutes != nil {
			routesSnapshot = s.alertRoutes.Snapshot()
			for _, rule := range routesSnapshot.Rules {
				for _, channel := range rule.Channels {
					adapter := ensureIntegrationAdapter(adapters, channel)
					if adapter.Name == "" {
						adapter.Name = displayNameForChannel(channel)
					}
					if adapter.Status == "" || adapter.Status == "connected" {
						if rule.Enabled {
							adapter.Status = "connected"
						} else {
							adapter.Status = "warning"
						}
					}
					if adapter.LastSyncAt == "" {
						adapter.LastSyncAt = formatIfSet(rule.UpdatedAt)
					}
				}
			}
		}

		if s.notificationDispatcher != nil {
			for _, channel := range s.notificationDispatcher.Channels() {
				adapter := ensureIntegrationAdapter(adapters, channel)
				if adapter.Name == "" {
					adapter.Name = displayNameForChannel(channel)
				}
				if adapter.Status == "" {
					adapter.Status = "connected"
				}
			}
		}

		list := make([]integrationAdapterResponse, 0, len(adapters))
		for key, adapter := range adapters {
			adapter.ID = key
			if adapter.Type == "" {
				adapter.Type = classifyIntegrationType(key)
			}
			if adapter.Status == "" {
				adapter.Status = "disconnected"
			}
			list = append(list, *adapter)
		}
		sort.Slice(list, func(i, j int) bool { return strings.Compare(list[i].ID, list[j].ID) < 0 })

		overview := integrationOverviewResponse{
			Adapters:             list,
			WebhookDeliveries24h: estimateWebhookDeliveries(routesSnapshot),
			APICalls24h:          s.estimateAPICalls24h(scope),
		}

		return c.JSON(http.StatusOK, overview)
	}
}

func formatBackupWindow(schedules []ops.BackupSchedule) string {
	if len(schedules) == 0 {
		return ""
	}
	schedule := schedules[0]
	label := strings.TrimSpace(schedule.Cron)
	if label == "" {
		label = "@unknown"
	}
	duration := schedule.WindowMinutes
	if duration > 0 {
		return fmt.Sprintf("%s (%d min)", label, duration)
	}
	return label
}

func buildMaintenanceTasks(plan ops.MaintenancePlan) []maintenanceTaskResponse {
	tasks := make([]maintenanceTaskResponse, 0, len(plan.Playbooks))

	for _, playbook := range plan.Playbooks {
		id := firstNonEmpty(playbook.ID, fmt.Sprintf("playbook-%d", len(tasks)+1))
		owner := "ops-team"
		if len(playbook.Steps) > 0 {
			owner = firstNonEmpty(playbook.Steps[0].Responsible, owner)
		}
		windowLabel := "rolling"
		if len(plan.Windows) > 0 {
			windowLabel = summarizeWindow(plan.Windows[0])
		}
		status := "scheduled"
		tasks = append(tasks, maintenanceTaskResponse{ID: id, Title: playbook.Title, Owner: owner, Window: windowLabel, Status: status})
	}

	if len(tasks) > 6 {
		tasks = tasks[:6]
	}
	return tasks
}

func (s *HTTPServer) buildCapacityForecasts(ctx context.Context, scope lease.ResourceScope) []capacityForecastResponse {
	forecasts := []capacityForecastResponse{}
	if s.monitor != nil {
		if scope.IsZero() {
			scope = scope.WithTenantOverride(systemTenantID)
		}
		if pools, err := s.monitor.Pools(ctx, scope, 5); err == nil {
			limit := 3
			if len(pools) < limit {
				limit = len(pools)
			}
			for idx := 0; idx < limit; idx++ {
				poolEntry := pools[idx]
				current := int(math.Round(poolEntry.Utilization))
				if current < 0 {
					current = 0
				}
				if current > 100 {
					current = 100
				}
				projection := current + 10
				if projection > 100 {
					projection = 100
				}
				forecasts = append(forecasts, capacityForecastResponse{
					Metric:        fmt.Sprintf("pool:%s", firstNonEmpty(poolEntry.Name, poolEntry.PoolID)),
					Current:       current,
					Limit:         100,
					Projection30d: projection,
				})
			}
		}
	}
	if len(forecasts) == 0 {
		forecasts = append(forecasts, capacityForecastResponse{Metric: "lease-utilization", Current: 0, Limit: 100, Projection30d: 0})
	}
	return forecasts
}

func buildSupportResources(info ops.HelpCenterInfo) []supportResourceResponse {
	resources := make([]supportResourceResponse, 0, len(info.Articles)+len(info.FAQ))
	base := strings.TrimSuffix(strings.TrimSpace(info.BaseURL), "/")
	for _, article := range info.Articles {
		link := resolveHelpLink(base, article.URL, article.Path)
		resources = append(resources, supportResourceResponse{
			ID:        firstNonEmpty(article.ID, article.Path, article.URL),
			Title:     article.Title,
			Type:      classifySupportType(article.Title, article.Path),
			UpdatedAt: formatIfSet(article.UpdatedAt),
			Link:      link,
		})
	}
	for _, faq := range info.FAQ {
		resources = append(resources, supportResourceResponse{
			ID:        firstNonEmpty(faq.ID, faq.Question),
			Title:     faq.Question,
			Type:      "faq",
			UpdatedAt: formatIfSet(faq.UpdatedAt),
			Link:      resolveHelpLink(base, "", faq.ID),
		})
	}
	if len(resources) > 10 {
		resources = resources[:10]
	}
	return resources
}

func latestRelease(releases []ops.ReleaseNote) (string, []string) {
	if len(releases) == 0 {
		return "", []string{}
	}
	latest := releases[0]
	for _, candidate := range releases[1:] {
		if candidate.PublishedAt.After(latest.PublishedAt) {
			latest = candidate
		}
	}
	return latest.Version, append([]string(nil), latest.Highlights...)
}

func ensureIntegrationAdapter(store map[string]*integrationAdapterResponse, channel string) *integrationAdapterResponse {
	key := strings.ToLower(strings.TrimSpace(channel))
	if key == "" {
		key = "default"
	}
	if adapter, exists := store[key]; exists {
		return adapter
	}
	adapter := &integrationAdapterResponse{}
	store[key] = adapter
	return adapter
}

func displayNameForChannel(channel string) string {
	trimmed := strings.TrimSpace(channel)
	if trimmed == "" {
		return "Integration"
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		parts = []string{trimmed}
	}
	for idx, part := range parts {
		lower := strings.ToLower(part)
		if lower == "" {
			continue
		}
		parts[idx] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}

func classifyIntegrationType(id string) string {
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "cmdb"):
		return "cmdb"
	case strings.Contains(lower, "itsm"), strings.Contains(lower, "servicenow"):
		return "itsm"
	case strings.Contains(lower, "webhook"), strings.Contains(lower, "http"):
		return "webhook"
	default:
		return "monitoring"
	}
}

func estimateWebhookDeliveries(snapshot alerting.RoutingSnapshot) int {
	deliveries := 0
	for _, rule := range snapshot.Rules {
		if !rule.Enabled {
			continue
		}
		for _, channel := range rule.Channels {
			lower := strings.ToLower(channel)
			if strings.Contains(lower, "webhook") || strings.Contains(lower, "http") {
				deliveries += 10
			}
		}
	}
	return deliveries
}

func (s *HTTPServer) estimateAPICalls24h(scope lease.ResourceScope) int {
	if s.monitor == nil {
		return 0
	}
	if scope.IsZero() {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	total := 0
	for _, phase := range s.monitor.Requests(scope) {
		total += int(phase.Success) + int(phase.Failure)
	}
	return total
}

func summarizeWindow(window ops.MaintenanceWindow) string {
	label := strings.TrimSpace(window.Name)
	cron := strings.TrimSpace(window.Cron)
	if label != "" && cron != "" {
		return fmt.Sprintf("%s (%s)", label, cron)
	}
	if label != "" {
		return label
	}
	if cron != "" {
		return cron
	}
	return "rolling"
}

func resolveHelpLink(base, url, path string) string {
	if strings.TrimSpace(url) != "" {
		return url
	}
	if strings.TrimSpace(path) == "" {
		return base
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if base == "" {
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	}
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}

func classifySupportType(title, path string) string {
	combined := strings.ToLower(title + " " + path)
	switch {
	case strings.Contains(combined, "api"):
		return "api"
	case strings.Contains(combined, "guide"):
		return "guide"
	case strings.Contains(combined, "faq"):
		return "faq"
	default:
		return "doc"
	}
}

func defaultStandardDHCPOptions() []dhcpOptionDefinitionResponse {
	return []dhcpOptionDefinitionResponse{
		{Name: "Router", Code: 3, Category: "basic", Description: "Default gateway for subnet", UsageCount: 128},
		{Name: "DNS Servers", Code: 6, Category: "basic", Description: "Primary and secondary DNS servers", UsageCount: 124},
		{Name: "Domain Name", Code: 15, Category: "core", Description: "Client domain search suffix", UsageCount: 98},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
