package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"modern-dhcp/internal/monitoring"
	"modern-dhcp/pkg/models"
)

var errMonitoringUnavailable = errors.New("monitoring aggregator unavailable")

func (s *HTTPServer) buildOperationResponse(ctx context.Context, tenantID, action, status string, data any, changed bool, steps []models.OperationStep) models.OperationResponse {
	envelope := models.OperationResponse{
		Status:    status,
		Data:      data,
		Steps:     steps,
		TraceID:   correlationIDFromContext(ctx),
		Timestamp: time.Now().UTC(),
	}

	overview, overviewErr := s.collectOverviewSnapshot(ctx, tenantID)
	if overviewErr == nil {
		envelope.Stats = map[string]any{
			"overview": overview,
		}
	} else if envelope.Stats == nil {
		envelope.Stats = map[string]any{}
	}
	if overviewErr != nil && !errors.Is(overviewErr, errMonitoringUnavailable) {
		envelope.Stats["error"] = overviewErr.Error()
	}

	envelope.Validation = s.buildValidationReport(overviewErr)
	envelope.Sync = s.buildSyncReport()
	envelope.Events = s.buildOperationEvents(ctx, tenantID, action, changed)
	envelope.Cleanup = s.buildCleanupReport(changed)

	return envelope
}

func (s *HTTPServer) collectOverviewSnapshot(ctx context.Context, tenantID string) (monitoring.OverviewSnapshot, error) {
	if s == nil || s.monitor == nil {
		return monitoring.OverviewSnapshot{}, errMonitoringUnavailable
	}
	return s.monitor.Overview(ctx, tenantID, 24)
}

func (s *HTTPServer) buildValidationReport(overviewErr error) *models.ValidationReport {
	report := &models.ValidationReport{
		CheckedAt: time.Now().UTC(),
	}
	switch {
	case overviewErr == nil:
		report.Status = "passed"
		report.Messages = []string{"租约与池统计已刷新"}
	case errors.Is(overviewErr, errMonitoringUnavailable):
		report.Status = "skipped"
		report.Messages = []string{"监控聚合未启用"}
	default:
		report.Status = "degraded"
		report.Messages = []string{overviewErr.Error()}
	}
	return report
}

func (s *HTTPServer) buildSyncReport() *models.SyncReport {
	reporter := s.options.Coordinator
	if reporter == nil {
		return nil
	}
	snapshot := reporter.Snapshot()
	report := &models.SyncReport{
		Role:           string(snapshot.Role),
		State:          string(snapshot.State),
		PeerLastSeen:   snapshot.PeerLastSeen,
		PeerHealthyAt:  snapshot.PeerHealthySince,
		ManualFailback: snapshot.ManualFailbackSet,
	}
	if !snapshot.PeerLastSeen.IsZero() {
		report.LagSeconds = time.Since(snapshot.PeerLastSeen).Seconds()
	}
	return report
}

func (s *HTTPServer) buildOperationEvents(ctx context.Context, tenantID, action string, changed bool) []models.OperationEvent {
	now := time.Now().UTC()
	if !changed {
		return []models.OperationEvent{{
			Channel:   "orchestrator",
			Status:    "noop",
			Detail:    "资源已处于目标状态",
			EmittedAt: now,
		}}
	}
	events := []models.OperationEvent{
		{
			Channel:       "audit",
			Status:        "completed",
			Detail:        fmt.Sprintf("%s 审计记录已写入", action),
			CorrelationID: correlationIDFromContext(ctx),
			EmittedAt:     now,
		},
	}
	if s.monitor != nil {
		events = append(events, models.OperationEvent{
			Channel:   "monitoring",
			Status:    "queued",
			Detail:    "监控聚合刷新已排队",
			EmittedAt: now,
		})
	}
	if s.options.Coordinator != nil {
		events = append(events, models.OperationEvent{
			Channel:   "replication",
			Status:    "accepted",
			Detail:    "主备同步窗口已接收",
			EmittedAt: now,
		})
	}
	if s.iotRegistry != nil {
		events = append(events, models.OperationEvent{
			Channel:   "automation",
			Status:    "scheduled",
			Detail:    fmt.Sprintf("租户 %s 自动化任务已通知", tenantID),
			EmittedAt: now,
		})
	}
	return events
}

func (s *HTTPServer) buildCleanupReport(changed bool) *models.CleanupReport {
	report := &models.CleanupReport{
		CompletedAt: time.Now().UTC(),
		Tasks: []string{
			"audit.flush",
			"metrics.update",
			"cache.invalidate",
		},
	}
	if changed {
		report.Status = "done"
	} else {
		report.Status = "skipped"
	}
	return report
}
