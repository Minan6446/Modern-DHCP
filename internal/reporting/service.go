package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/pkg/auditpayload"
)

var (
	// ErrReportingDisabled is returned when reporting dependencies are missing.
	ErrReportingDisabled = errors.New("reporting: service disabled")
)

// Options wires dependencies for the reporting service.
type Options struct {
	Monitor      *monitoring.Aggregator
	Audit        *audit.Service
	LeaseHistory LeaseHistorySource
	Renderer     Renderer
	ExportPath   string
	Logger       *zap.Logger
}

// Service delivers compliance and audit-friendly reports.
type Service struct {
	monitor       *monitoring.Aggregator
	audit         *audit.Service
	logger        *zap.Logger
	historySource LeaseHistorySource
	renderer      Renderer
	exportPath    string
	leaseJobs     map[string]*leaseHistoryJob
	jobMu         sync.Mutex
}

// NewService constructs a reporting service.
func NewService(opts Options) *Service {
	renderer := opts.Renderer
	if renderer == nil {
		renderer = NewCSVRenderer()
	}
	return &Service{
		monitor:       opts.Monitor,
		audit:         opts.Audit,
		logger:        opts.Logger,
		historySource: opts.LeaseHistory,
		renderer:      renderer,
		exportPath:    opts.ExportPath,
	}
}

// MonthlyUsage generates the monthly address utilization snapshot.
func (s *Service) MonthlyUsage(ctx context.Context, tenantID string, limit int) (MonthlyUsageReport, error) {
	if s.monitor == nil {
		return MonthlyUsageReport{}, ErrReportingDisabled
	}
	if limit <= 0 {
		limit = 50
	}
	overview, err := s.monitor.Overview(ctx, tenantID, limit)
	if err != nil {
		return MonthlyUsageReport{}, err
	}
	report := MonthlyUsageReport{
		TenantID:           tenantID,
		GeneratedAt:        overview.GeneratedAt,
		PoolUsage:          overview.PoolUsage,
		ClientDistribution: overview.ClientDistribution,
		RequestPhases:      overview.RequestPhases,
		Security:           overview.Security,
	}
	for _, pool := range overview.PoolUsage {
		if pool.Utilization >= 90 {
			report.Notes = append(report.Notes, fmt.Sprintf("pool %s is %.1f%% utilized", pool.Name, pool.Utilization))
		} else if pool.Utilization >= 80 {
			report.Notes = append(report.Notes, fmt.Sprintf("pool %s nearing capacity (%.1f%%)", pool.Name, pool.Utilization))
		}
	}
	return report, nil
}

// SecurityCompliance assembles guard findings and admin activity logs.
func (s *Service) SecurityCompliance(ctx context.Context, tenantID string, limit int) (SecurityComplianceReport, error) {
	if s.audit == nil {
		return SecurityComplianceReport{}, ErrReportingDisabled
	}
	if limit <= 0 {
		limit = 200
	}
	filter := audit.ListEventsFilter{
		Actions: []string{"security.alert", "security.violation", "security.error", "admin.activity"},
		Limit:   limit,
	}
	events, err := s.audit.ListEventsFiltered(ctx, tenantID, filter)
	if err != nil {
		return SecurityComplianceReport{}, err
	}
	report := SecurityComplianceReport{TenantID: tenantID, GeneratedAt: time.Now().UTC()}
	for _, evt := range events {
		switch evt.Action {
		case "security.alert", "security.violation", "security.error":
			incident := SecurityIncident{Action: evt.Action, Actor: evt.Actor, CreatedAt: evt.CreatedAt}
			var payload auditpayload.SecurityEvent
			if len(evt.Payload) > 0 {
				if err := json.Unmarshal(evt.Payload, &payload); err == nil {
					incident.Payload = payload
				}
			}
			if evt.Action == "security.violation" {
				report.Violations = append(report.Violations, incident)
				report.Summary.TotalViolations++
				report.Summary.LastViolationAt = &incident.CreatedAt
			} else {
				report.Alerts = append(report.Alerts, incident)
				report.Summary.TotalAlerts++
			}
		case "admin.activity":
			var payload auditpayload.AdminActivity
			if len(evt.Payload) > 0 {
				if err := json.Unmarshal(evt.Payload, &payload); err != nil {
					if s.logger != nil {
						s.logger.Debug("reporting: admin activity decode failed", zap.Error(err))
					}
				}
			}
			record := AdminActivityRecord{
				Actor:       pickValue(payload.Actor, evt.Actor),
				Role:        payload.Role,
				Method:      payload.Method,
				Path:        payload.Path,
				StatusCode:  payload.StatusCode,
				Sensitive:   payload.Sensitive,
				ObservedAt:  evt.CreatedAt,
				Correlation: payload.CorrelationID,
			}
			report.AdminActivities = append(report.AdminActivities, record)
		}
	}
	return report, nil
}

// CapacityPlanning highlights pools approaching saturation.
func (s *Service) CapacityPlanning(ctx context.Context, tenantID string, limit int) (CapacityPlanningReport, error) {
	if s.monitor == nil {
		return CapacityPlanningReport{}, ErrReportingDisabled
	}
	if limit <= 0 {
		limit = 100
	}
	pools, err := s.monitor.Pools(ctx, tenantID, limit)
	if err != nil {
		return CapacityPlanningReport{}, err
	}
	report := CapacityPlanningReport{TenantID: tenantID, GeneratedAt: time.Now().UTC()}
	for _, pool := range pools {
		if pool.Utilization >= 90 {
			report.SaturatedPools = append(report.SaturatedPools, pool)
		} else if pool.Utilization >= 75 {
			report.HotPools = append(report.HotPools, pool)
		}
	}
	for _, pool := range report.SaturatedPools {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("Add capacity or split pool %s (%.1f%%)", pool.Name, pool.Utilization))
	}
	for _, pool := range report.HotPools {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("Plan expansion for %s within next cycle (%.1f%%)", pool.Name, pool.Utilization))
	}
	if len(report.Recommendations) == 0 {
		report.Recommendations = append(report.Recommendations, "Utilization within healthy range; continue monitoring")
	}
	return report, nil
}

// AuditTrail builds a chronological view filtered by resource/correlation ID.
func (s *Service) AuditTrail(ctx context.Context, tenantID, resource, correlationID string, limit int) (AuditTrailReport, error) {
	if s.audit == nil {
		return AuditTrailReport{}, ErrReportingDisabled
	}
	if limit <= 0 {
		limit = 200
	}
	filter := audit.ListEventsFilter{Resource: resource, CorrelationID: correlationID, Limit: limit}
	events, err := s.audit.ListEventsFiltered(ctx, tenantID, filter)
	if err != nil {
		return AuditTrailReport{}, err
	}
	report := AuditTrailReport{TenantID: tenantID, Resource: resource, Generated: time.Now().UTC()}
	for _, evt := range events {
		entry := AuditTimelineEntry{
			AuditID:       evt.AuditID,
			Action:        evt.Action,
			Actor:         evt.Actor,
			Resource:      evt.Resource,
			CorrelationID: evt.CorrelationID,
			CreatedAt:     evt.CreatedAt,
			Raw:           evt,
		}
		if len(evt.Payload) > 0 {
			if payloadMap, err := decodePayloadMap(evt.Payload); err == nil {
				entry.Payload = payloadMap
			} else if s.logger != nil {
				s.logger.Debug("reporting: audit payload decode failed", zap.Error(err))
			}
		}
		report.Events = append(report.Events, entry)
	}
	return report, nil
}

func pickValue(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func decodePayloadMap(raw json.RawMessage) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
