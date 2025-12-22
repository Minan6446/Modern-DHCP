package main

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/automation"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/notifications"
)

func setupAutomation(autoCfg config.AutomationConfig, notifyCfg config.NotificationsConfig, dispatcher *notifications.Dispatcher, aggregator *monitoring.Aggregator, jobStore automation.JobStore, baseLogger *zap.Logger) *automation.Service {
	if !autoCfg.Enabled {
		return nil
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
		Schedules: buildAutomationSchedules(autoCfg.Jobs),
		Logger:    logger,
		Store:     jobStore,
	})

	if dispatcher != nil {
		service.RegisterHandler(automation.JobNotificationFanout, automation.NewNotificationFanoutHandler(dispatcher, logger, autoCfg.Jobs.NotificationFanout.Channels))
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
				snapshot, err := aggregator.Overview(ctx, job.TenantID, 25)
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

	return service
}

func buildAutomationSchedules(cfg config.AutomationJobsConfig) map[automation.JobType]automation.ScheduleConfig {
	schedules := map[automation.JobType]automation.ScheduleConfig{}
	addSchedule := func(jobType automation.JobType, jobCfg config.AutomationJobConfig) {
		if !jobCfg.Enabled || jobCfg.Interval <= 0 {
			return
		}
		payload := encodeJobPayload(jobCfg)
		schedules[jobType] = automation.ScheduleConfig{
			Enabled:      true,
			Interval:     jobCfg.Interval,
			InitialDelay: jobCfg.InitialDelay,
			TenantID:     jobCfg.TenantID,
			Labels:       jobCfg.Labels,
			Payload:      payload,
			Channels:     append([]string(nil), jobCfg.Channels...),
		}
	}
	addSchedule(automation.JobInventorySync, cfg.Inventory)
	addSchedule(automation.JobPolicyAudit, cfg.PolicyAudit)
	addSchedule(automation.JobSecurityScan, cfg.SecurityScan)
	addSchedule(automation.JobAnalyticsSnapshot, cfg.Analytics)
	addSchedule(automation.JobNotificationFanout, cfg.NotificationFanout)
	return schedules
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
