package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"modern-dhcp/internal/automation"
	approvals "modern-dhcp/internal/automation/approvals"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/notifications"
)

func setupAutomation(autoCfg config.AutomationConfig, notifyCfg config.NotificationsConfig, dispatcher *notifications.Dispatcher, aggregator *monitoring.Aggregator, jobStore automation.JobStore, db *sqlx.DB, baseLogger *zap.Logger) (*automation.Service, *approvals.Service, map[approvals.RequestType]struct{}) {
	if !autoCfg.Enabled {
		return nil, nil, nil
	}
	logger := baseLogger
	if logger != nil {
		logger = baseLogger.Named("automation")
	} else {
		logger = zap.NewNop()
	}
	service := automation.NewService(automation.ServiceOptions{
		SchedulerOptions: automation.Options{
			QueueSize:      autoCfg.Scheduler.QueueSize,
			WorkerCount:    autoCfg.Scheduler.WorkerCount,
			MaxAttempts:    autoCfg.Scheduler.MaxAttempts,
			DefaultTimeout: autoCfg.Scheduler.DefaultTimeout,
		},
		Schedules: buildAutomationSchedules(autoCfg),
		Logger:    logger,
		Store:     jobStore,
	})

	var (
		approvalService *approvals.Service
		approvalPolicy  map[approvals.RequestType]struct{}
	)

	if autoCfg.Approvals.Enabled {
		approvalPolicy = buildApprovalPolicy(autoCfg.Approvals.RequireFor, logger)
		if db == nil {
			logger.Warn("automation approvals enabled without database connection")
		} else if service == nil {
			logger.Warn("automation approvals enabled but automation service unavailable")
		} else {
			applier := &automationApprovalApplier{automation: service, logger: logger}
			var err error
			approvalService, err = approvals.NewService(approvals.ServiceOptions{
				Repository:         approvals.NewRepository(db),
				Logger:             logger,
				AutoApplyOnApprove: autoCfg.Approvals.AutoApplyOnApprove,
				Applier:            applier,
			})
			if err != nil {
				logger.Fatal("init automation approvals", zap.Error(err))
			}
		}
	}

	if dispatcher != nil {
		service.RegisterHandler(automation.JobNotificationFanout, automation.NewNotificationFanoutHandler(dispatcher, logger, scheduleChannels(autoCfg, automation.JobNotificationFanout)))
		if notifyCfg.Integrations.CMDB.Enabled && notifyCfg.Integrations.CMDB.Channel != "" {
			channel := notifyCfg.Integrations.CMDB.Channel
			service.RegisterHandler(automation.JobInventorySync, automation.HandlerFunc(func(ctx context.Context, job automation.Job) error {
				msg := notifications.Message{
					ID:        job.ID,
					TenantID:  job.TenantID,
					Topic:     string(automation.JobInventorySync),
					Summary:   "Inventory synchronization triggered",
					Severity:  "info",
					Metadata:  job.Labels,
					CreatedAt: time.Now().UTC(),
				}
				return dispatcher.Dispatch(ctx, msg, channel)
			}))
		}
		if aggregator != nil && notifyCfg.Integrations.Monitoring.Enabled && notifyCfg.Integrations.Monitoring.Channel != "" {
			channel := notifyCfg.Integrations.Monitoring.Channel
			service.RegisterHandler(automation.JobAnalyticsSnapshot, automation.HandlerFunc(func(ctx context.Context, job automation.Job) error {
				scope := lease.NewResourceScope("", job.TenantID)
				snapshot, err := aggregator.Overview(ctx, scope, 25)
				if err != nil {
					return err
				}
				msg := notifications.Message{
					ID:        job.ID,
					TenantID:  job.TenantID,
					Topic:     string(automation.JobAnalyticsSnapshot),
					Summary:   "Monitoring snapshot refreshed",
					Severity:  "info",
					Body:      map[string]any{"overview": snapshot},
					Metadata:  job.Labels,
					CreatedAt: time.Now().UTC(),
				}
				return dispatcher.Dispatch(ctx, msg, channel)
			}))
		}
	}

	if approvalService != nil && approvalPolicy == nil {
		approvalPolicy = make(map[approvals.RequestType]struct{})
	}

	return service, approvalService, approvalPolicy
}

func encodeJobPayload(jobCfg config.AutomationJobConfig) json.RawMessage {
	if len(jobCfg.Payload) == 0 {
		return nil
	}
	body := make(map[string]any, len(jobCfg.Payload))
	for k, v := range jobCfg.Payload {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return raw
}

func buildAutomationSchedules(cfg config.AutomationConfig) map[automation.JobType]automation.ScheduleConfig {
	schedules := make(map[automation.JobType]automation.ScheduleConfig)
	addSchedule := func(jobType automation.JobType, jobCfg config.AutomationJobConfig) {
		if isEmptyAutomationJobConfig(jobCfg) {
			return
		}
		schedules[jobType] = scheduleFromJobConfig(jobCfg)
	}

	for rawType, jobCfg := range cfg.Schedules {
		jobType := automation.JobType(strings.TrimSpace(rawType))
		if jobType == "" {
			continue
		}
		addSchedule(jobType, jobCfg)
	}

	legacy := map[automation.JobType]config.AutomationJobConfig{
		automation.JobInventorySync:      cfg.Jobs.Inventory,
		automation.JobPolicyAudit:        cfg.Jobs.PolicyAudit,
		automation.JobSecurityScan:       cfg.Jobs.SecurityScan,
		automation.JobAnalyticsSnapshot:  cfg.Jobs.Analytics,
		automation.JobNotificationFanout: cfg.Jobs.NotificationFanout,
		automation.JobWorkflowExecution:  cfg.Jobs.WorkflowExecution,
	}
	for jobType, jobCfg := range legacy {
		if _, exists := schedules[jobType]; exists {
			continue
		}
		addSchedule(jobType, jobCfg)
	}
	return schedules
}

func scheduleFromJobConfig(jobCfg config.AutomationJobConfig) automation.ScheduleConfig {
	payload := encodeJobPayload(jobCfg)
	return automation.ScheduleConfig{
		Enabled:      jobCfg.Enabled,
		Interval:     jobCfg.Interval,
		InitialDelay: jobCfg.InitialDelay,
		TenantID:     strings.TrimSpace(jobCfg.TenantID),
		Labels:       copyStringMap(jobCfg.Labels),
		Payload:      payload,
		Channels:     copyStringSlice(jobCfg.Channels),
	}
}

func scheduleChannels(cfg config.AutomationConfig, jobType automation.JobType) []string {
	jobCfg, ok := automationJobConfigForType(cfg, jobType)
	if !ok {
		return nil
	}
	return copyStringSlice(jobCfg.Channels)
}

func automationJobConfigForType(cfg config.AutomationConfig, jobType automation.JobType) (config.AutomationJobConfig, bool) {
	if cfg.Schedules != nil {
		if entry, ok := cfg.Schedules[string(jobType)]; ok {
			return entry, true
		}
	}
	switch jobType {
	case automation.JobInventorySync:
		return cfg.Jobs.Inventory, true
	case automation.JobPolicyAudit:
		return cfg.Jobs.PolicyAudit, true
	case automation.JobSecurityScan:
		return cfg.Jobs.SecurityScan, true
	case automation.JobAnalyticsSnapshot:
		return cfg.Jobs.Analytics, true
	case automation.JobNotificationFanout:
		return cfg.Jobs.NotificationFanout, true
	case automation.JobWorkflowExecution:
		return cfg.Jobs.WorkflowExecution, true
	default:
		return config.AutomationJobConfig{}, false
	}
}

func isEmptyAutomationJobConfig(jobCfg config.AutomationJobConfig) bool {
	if jobCfg.Enabled {
		return false
	}
	if jobCfg.Interval > 0 || jobCfg.InitialDelay > 0 {
		return false
	}
	if strings.TrimSpace(jobCfg.TenantID) != "" {
		return false
	}
	if len(jobCfg.Labels) > 0 {
		return false
	}
	if len(jobCfg.Payload) > 0 {
		return false
	}
	if len(jobCfg.Channels) > 0 {
		return false
	}
	return true
}

func buildApprovalPolicy(entries []string, logger *zap.Logger) map[approvals.RequestType]struct{} {
	if len(entries) == 0 {
		return make(map[approvals.RequestType]struct{})
	}
	policy := make(map[approvals.RequestType]struct{}, len(entries))
	for _, raw := range entries {
		token := strings.TrimSpace(strings.ToLower(raw))
		switch token {
		case string(approvals.RequestTypeScheduleUpdate):
			policy[approvals.RequestTypeScheduleUpdate] = struct{}{}
		case string(approvals.RequestTypeScheduleToggle):
			policy[approvals.RequestTypeScheduleToggle] = struct{}{}
		case string(approvals.RequestTypeJobRun):
			policy[approvals.RequestTypeJobRun] = struct{}{}
		case "":
			// ignore empty entries
		default:
			if logger != nil {
				logger.Warn("unrecognized automation approval requirement", zap.String("entry", raw))
			}
		}
	}
	return policy
}

type automationApprovalApplier struct {
	automation *automation.Service
	logger     *zap.Logger
}

func (a *automationApprovalApplier) ApplyChange(ctx context.Context, request approvals.ChangeRequest) error {
	if a.automation == nil {
		return errors.New("automation approvals: automation service unavailable")
	}
	switch request.RequestType {
	case approvals.RequestTypeScheduleUpdate, approvals.RequestTypeScheduleToggle:
		jobType, cfg, err := parseScheduleProposal(request.JobType, request.ProposedConfig)
		if err != nil {
			return err
		}
		a.automation.UpdateSchedule(jobType, cfg)
		return nil
	case approvals.RequestTypeJobRun:
		job, err := parseJobProposal(request.JobType, request.ProposedConfig)
		if err != nil {
			return err
		}
		return a.automation.Enqueue(ctx, job)
	default:
		return fmt.Errorf("automation approvals: unsupported request type %s", request.RequestType)
	}
}

type scheduleProposal struct {
	Type         string            `json:"type"`
	Enabled      bool              `json:"enabled"`
	TenantID     string            `json:"tenantId"`
	Interval     string            `json:"interval"`
	InitialDelay string            `json:"initialDelay"`
	Labels       map[string]string `json:"labels"`
	Channels     []string          `json:"channels"`
	Payload      json.RawMessage   `json:"payload"`
}

func parseScheduleProposal(fallbackType string, raw json.RawMessage) (automation.JobType, automation.ScheduleConfig, error) {
	var proposal scheduleProposal
	if len(raw) == 0 {
		return "", automation.ScheduleConfig{}, errors.New("automation approvals: missing schedule proposal")
	}
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return "", automation.ScheduleConfig{}, fmt.Errorf("automation approvals: decode schedule proposal: %w", err)
	}
	jobType := strings.TrimSpace(proposal.Type)
	if jobType == "" {
		jobType = strings.TrimSpace(fallbackType)
	}
	if jobType == "" {
		return "", automation.ScheduleConfig{}, errors.New("automation approvals: schedule job type required")
	}
	interval, err := time.ParseDuration(strings.TrimSpace(proposal.Interval))
	if err != nil || interval <= 0 {
		return "", automation.ScheduleConfig{}, fmt.Errorf("automation approvals: invalid interval %q", proposal.Interval)
	}
	initialDelay := time.Duration(0)
	if strings.TrimSpace(proposal.InitialDelay) != "" {
		delay, parseErr := time.ParseDuration(strings.TrimSpace(proposal.InitialDelay))
		if parseErr != nil || delay < 0 {
			return "", automation.ScheduleConfig{}, fmt.Errorf("automation approvals: invalid initialDelay %q", proposal.InitialDelay)
		}
		initialDelay = delay
	}
	cfg := automation.ScheduleConfig{
		Enabled:      proposal.Enabled,
		Interval:     interval,
		InitialDelay: initialDelay,
		TenantID:     strings.TrimSpace(proposal.TenantID),
		Labels:       copyStringMap(proposal.Labels),
		Payload:      cloneJSON(proposal.Payload),
		Channels:     copyStringSlice(proposal.Channels),
	}
	if cfg.TenantID == "" {
		return "", automation.ScheduleConfig{}, errors.New("automation approvals: schedule tenantId required")
	}
	return automation.JobType(jobType), cfg, nil
}

type jobProposal struct {
	Type        string            `json:"type"`
	TenantID    string            `json:"tenantId"`
	Labels      map[string]string `json:"labels"`
	Channels    []string          `json:"channels"`
	Payload     json.RawMessage   `json:"payload"`
	TriggeredBy string            `json:"triggeredBy"`
	Source      string            `json:"source"`
	Priority    *int              `json:"priority"`
	NotBefore   string            `json:"notBefore"`
}

func parseJobProposal(fallbackType string, raw json.RawMessage) (automation.Job, error) {
	if len(raw) == 0 {
		return automation.Job{}, errors.New("automation approvals: missing job proposal")
	}
	var proposal jobProposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return automation.Job{}, fmt.Errorf("automation approvals: decode job proposal: %w", err)
	}
	jobType := strings.TrimSpace(proposal.Type)
	if jobType == "" {
		jobType = strings.TrimSpace(fallbackType)
	}
	if jobType == "" {
		return automation.Job{}, errors.New("automation approvals: job type required")
	}
	tenantID := strings.TrimSpace(proposal.TenantID)
	if tenantID == "" {
		return automation.Job{}, errors.New("automation approvals: tenantId required")
	}
	job := automation.Job{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Type:        automation.JobType(jobType),
		TriggeredBy: strings.TrimSpace(proposal.TriggeredBy),
		Source:      strings.TrimSpace(proposal.Source),
		Labels:      copyStringMap(proposal.Labels),
	}
	if job.Source == "" {
		job.Source = automation.JobSourceManual
	}
	if job.TriggeredBy == "" {
		job.TriggeredBy = automation.JobActorSystem
	}
	job.Payload = automation.MergeChannels(cloneJSON(proposal.Payload), proposal.Channels)
	if proposal.Priority != nil {
		job.Priority = *proposal.Priority
	}
	if strings.TrimSpace(proposal.NotBefore) != "" {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(proposal.NotBefore))
		if err != nil {
			return automation.Job{}, fmt.Errorf("automation approvals: invalid notBefore %q", proposal.NotBefore)
		}
		job.NotBefore = ts.UTC()
	}
	return job, nil
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	dup := make([]byte, len(raw))
	copy(dup, raw)
	return dup
}
