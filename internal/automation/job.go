package automation

import (
	"encoding/json"
	"time"
)

// JobType enumerates the automation workflows supported by the scheduler.
type JobType string

const (
	// JobInventorySync pulls CMDB/asset inventories to refresh server metadata.
	JobInventorySync JobType = "inventory.sync"
	// JobPolicyAudit re-evaluates policy baselines and security posture.
	JobPolicyAudit JobType = "policy.audit"
	// JobSecurityScan executes guardrails (DHCP snooping, rogue detection, etc.).
	JobSecurityScan JobType = "security.scan"
	// JobAnalyticsSnapshot refreshes monitoring + analytics exports.
	JobAnalyticsSnapshot JobType = "analytics.snapshot"
	// JobNotificationFanout delivers alert payloads to downstream channels.
	JobNotificationFanout JobType = "notification.fanout"
	// JobWorkflowExecution orchestrates a workflow definition run.
	JobWorkflowExecution JobType = "workflow.execution"
)

// JobStatus describes lifecycle state for in-memory queues.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

// Job defines an automation task.
type Job struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenantId"`
	Type        JobType           `json:"type"`
	Source      string            `json:"source,omitempty"`
	TriggeredBy string            `json:"triggeredBy,omitempty"`
	Payload     json.RawMessage   `json:"payload,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	NotBefore   time.Time         `json:"notBefore,omitempty"`
	Attempts    int               `json:"attempts"`
	Status      JobStatus         `json:"status"`
	CreatedAt   time.Time         `json:"createdAt"`
}

const (
	JobSourceManual   = "api.manual"
	JobSourceSchedule = "automation.schedule"
	JobSourceWorkflow = "workflow.execution"
)

const (
	JobActorSystem    = "system"
	JobActorScheduler = "system.scheduler"
	JobActorWorkflow  = "workflow.service"
)

// Options configure the in-memory scheduler behavior.
type Options struct {
	QueueSize      int
	WorkerCount    int
	MaxAttempts    int
	DefaultTimeout time.Duration
}

// Snapshot exposes the scheduler runtime state for diagnostics.
type Snapshot struct {
	PendingJobs        int       `json:"pendingJobs"`
	ActiveWorkers      int       `json:"activeWorkers"`
	RegisteredHandlers int       `json:"registeredHandlers"`
	StartedAt          time.Time `json:"startedAt"`
	UptimeSeconds      int64     `json:"uptimeSeconds"`
	AverageWaitMillis  float64   `json:"averageWaitMillis"`
	AverageRunMillis   float64   `json:"averageRunMillis"`
	CompletedJobs      int64     `json:"completedJobs"`
	FailedJobs         int64     `json:"failedJobs"`
	RetryScheduled     int64     `json:"retryScheduled"`
}
