package reporting

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/pkg/models"
)

const (
	historyExportDefaultLimit = 500
	historyExportMaxLimit     = 2000
)

var (
	// ErrHistorySourceUnavailable is returned when no lease history source is wired.
	ErrHistorySourceUnavailable = errors.New("reporting: lease history source unavailable")
	// ErrRendererUnavailable is returned when no renderer is configured.
	ErrRendererUnavailable = errors.New("reporting: renderer unavailable")
	// ErrExportPathUnavailable indicates exports cannot be persisted.
	ErrExportPathUnavailable = errors.New("reporting: export path not configured")
)

// LeaseHistorySource exposes lease history retrieval for exports.
type LeaseHistorySource interface {
	History(ctx context.Context, scope lease.ResourceScope, filter models.LeaseHistoryFilter) ([]models.Lease, int, error)
}

// LeaseHistoryExportRequest describes an ad-hoc export payload.
type LeaseHistoryExportRequest struct {
	TenantID    string
	Scope       lease.ResourceScope
	Format      Format
	Destination string
	Filter      models.LeaseHistoryFilter
}

// LeaseHistoryScheduleRequest configures a recurring history export.
type LeaseHistoryScheduleRequest struct {
	TenantID    string
	Scope       lease.ResourceScope
	Format      Format
	Destination string
	Filter      models.LeaseHistoryFilter
	Interval    time.Duration
}

// ExportJobSummary returns scheduling metadata to callers.
type ExportJobSummary struct {
	JobID    string        `json:"jobId"`
	Report   string        `json:"report"`
	TenantID string        `json:"tenantId"`
	Format   Format        `json:"format"`
	Interval time.Duration `json:"interval"`
	NextRun  time.Time     `json:"nextRun"`
}

// ExportLeaseHistory generates a lease history artifact and persists it to disk.
func (s *Service) ExportLeaseHistory(ctx context.Context, req LeaseHistoryExportRequest) (Artifact, error) {
	if s.historySource == nil {
		return Artifact{}, ErrHistorySourceUnavailable
	}
	if s.renderer == nil {
		return Artifact{}, ErrRendererUnavailable
	}
	if s.exportPath == "" {
		return Artifact{}, ErrExportPathUnavailable
	}
	format := req.Format
	if format == "" {
		format = FormatCSV
	}
	normalizedScope, tenantID, err := normalizeTenantScope(req.Scope, req.TenantID)
	if err != nil {
		return Artifact{}, err
	}
	filter := normalizeHistoryFilter(req.Filter)
	data, err := s.collectLeaseHistory(ctx, normalizedScope, filter)
	if err != nil {
		return Artifact{}, err
	}
	artifact, err := s.renderer.RenderLeaseHistory(ctx, tenantID, data, format)
	if err != nil {
		return Artifact{}, err
	}
	if err := s.persistArtifact(&artifact, tenantID, req.Destination); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

// ScheduleLeaseHistoryExport registers a recurring export job.
func (s *Service) ScheduleLeaseHistoryExport(req LeaseHistoryScheduleRequest) (ExportJobSummary, error) {
	if s.historySource == nil {
		return ExportJobSummary{}, ErrHistorySourceUnavailable
	}
	if s.renderer == nil {
		return ExportJobSummary{}, ErrRendererUnavailable
	}
	if s.exportPath == "" {
		return ExportJobSummary{}, ErrExportPathUnavailable
	}
	normalizedScope, tenantID, err := normalizeTenantScope(req.Scope, req.TenantID)
	if err != nil {
		return ExportJobSummary{}, err
	}
	interval := req.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	format := req.Format
	if format == "" {
		format = FormatCSV
	}
	job := &leaseHistoryJob{
		id: uuid.NewString(),
		req: LeaseHistoryScheduleRequest{
			TenantID:    tenantID,
			Scope:       normalizedScope,
			Format:      format,
			Destination: req.Destination,
			Filter:      normalizeHistoryFilter(req.Filter),
			Interval:    interval,
		},
		service: s,
		stopCh:  make(chan struct{}),
		nextRun: time.Now().UTC().Add(interval),
	}
	s.jobMu.Lock()
	if s.leaseJobs == nil {
		s.leaseJobs = make(map[string]*leaseHistoryJob)
	}
	s.leaseJobs[job.id] = job
	s.jobMu.Unlock()
	go job.run()
	summary := ExportJobSummary{
		JobID:    job.id,
		Report:   "lease-history",
		TenantID: tenantID,
		Format:   format,
		Interval: interval,
		NextRun:  job.nextRun,
	}
	return summary, nil
}

// Close stops background schedulers.
func (s *Service) Close() {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	for id, job := range s.leaseJobs {
		job.stop()
		delete(s.leaseJobs, id)
	}
}

func (s *Service) collectLeaseHistory(ctx context.Context, scope lease.ResourceScope, filter models.LeaseHistoryFilter) ([]models.Lease, error) {
	normalizedScope, _, err := normalizeTenantScope(scope, "")
	if err != nil {
		return nil, err
	}
	batch, total, err := s.historySource.History(ctx, normalizedScope, filter)
	if err != nil {
		return nil, err
	}
	if len(batch) >= total || filter.Limit >= total {
		return batch, nil
	}
	records := make([]models.Lease, 0, total)
	records = append(records, batch...)
	offset := filter.Offset + filter.Limit
	for len(records) < total {
		next := filter
		next.Offset = offset
		nextBatch, _, err := s.historySource.History(ctx, normalizedScope, next)
		if err != nil {
			return nil, err
		}
		records = append(records, nextBatch...)
		if len(nextBatch) < next.Limit {
			break
		}
		offset += next.Limit
	}
	return records, nil
}

func (s *Service) persistArtifact(artifact *Artifact, tenantID, destination string) error {
	targetDir, err := s.resolveDestination(tenantID, destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	targetPath := filepath.Join(targetDir, artifact.Name)
	if err := os.WriteFile(targetPath, artifact.Data, 0o644); err != nil {
		return err
	}
	artifact.Path = targetPath
	return nil
}

func (s *Service) resolveDestination(tenantID, destination string) (string, error) {
	if s.exportPath == "" {
		return "", ErrExportPathUnavailable
	}
	subdir := strings.TrimSpace(destination)
	if subdir == "" {
		subdir = tenantID
	} else {
		subdir = filepath.Join(subdir, tenantID)
	}
	clean := filepath.Clean(subdir)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		clean = tenantID
	}
	return filepath.Join(s.exportPath, clean), nil
}

func normalizeHistoryFilter(filter models.LeaseHistoryFilter) models.LeaseHistoryFilter {
	if filter.Limit <= 0 {
		filter.Limit = historyExportDefaultLimit
	}
	if filter.Limit > historyExportMaxLimit {
		filter.Limit = historyExportMaxLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

type leaseHistoryJob struct {
	id       string
	req      LeaseHistoryScheduleRequest
	service  *Service
	stopCh   chan struct{}
	stopOnce sync.Once
	nextRun  time.Time
}

func (j *leaseHistoryJob) run() {
	j.service.executeScheduledExport(j)
	ticker := time.NewTicker(j.req.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			j.service.executeScheduledExport(j)
		case <-j.stopCh:
			return
		}
	}
}

func (j *leaseHistoryJob) stop() {
	j.stopOnce.Do(func() { close(j.stopCh) })
}

func (s *Service) executeScheduledExport(job *leaseHistoryJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, err := s.ExportLeaseHistory(ctx, LeaseHistoryExportRequest{
		TenantID:    job.req.TenantID,
		Scope:       job.req.Scope,
		Format:      job.req.Format,
		Destination: job.req.Destination,
		Filter:      job.req.Filter,
	})
	if err != nil && s.logger != nil {
		s.logger.Warn("reporting: scheduled lease history export failed", zap.String("jobId", job.id), zap.String("tenantId", job.req.TenantID), zap.Error(err))
	}
	job.nextRun = time.Now().UTC().Add(job.req.Interval)
}
