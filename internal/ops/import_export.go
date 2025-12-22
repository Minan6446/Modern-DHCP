package ops

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrImportExportDisabled = errors.New("ops: import/export disabled")
	ErrTransferJobNotFound  = errors.New("ops: transfer job not found")
)

// TransferKind identifies whether the job is an import or export.
type TransferKind string

const (
	TransferKindImport TransferKind = "import"
	TransferKindExport TransferKind = "export"
)

// TransferStatus represents the lifecycle state of a transfer job.
type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusRunning   TransferStatus = "running"
	TransferStatusSucceeded TransferStatus = "succeeded"
	TransferStatusFailed    TransferStatus = "failed"
)

// TransferRequest describes a bulk import/export job request.
type TransferRequest struct {
	Kind        TransferKind
	Resource    string
	Format      string
	SourceURI   string
	TargetURI   string
	Reason      string
	Metadata    map[string]string
	RequestedBy string
	SizeBytes   int64
}

// TransferJob surfaces the execution metadata for a transfer.
type TransferJob struct {
	ID           string            `json:"id"`
	Kind         TransferKind      `json:"kind"`
	Resource     string            `json:"resource"`
	Format       string            `json:"format"`
	Status       TransferStatus    `json:"status"`
	RequestedBy  string            `json:"requestedBy"`
	Reason       string            `json:"reason,omitempty"`
	SourceURI    string            `json:"sourceUri,omitempty"`
	TargetURI    string            `json:"targetUri,omitempty"`
	ArtifactPath string            `json:"artifactPath,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	SizeBytes    int64             `json:"sizeBytes,omitempty"`
	Error        string            `json:"error,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	StartedAt    time.Time         `json:"startedAt"`
	CompletedAt  time.Time         `json:"completedAt"`
}

func (job *TransferJob) clone() TransferJob {
	if job == nil {
		return TransferJob{}
	}
	dup := *job
	if len(job.Metadata) > 0 {
		dup.Metadata = cloneStringMap(job.Metadata)
	}
	return dup
}

const (
	transferHistoryLimit = 50
	transferQueueSize    = 32
)

type transferManager struct {
	opts    ImportExportOptions
	logger  *zap.Logger
	jobs    map[string]*TransferJob
	order   []string
	mu      sync.RWMutex
	queue   chan string
	closing chan struct{}
	once    sync.Once
}

func newTransferManager(opts ImportExportOptions, logger *zap.Logger) (*transferManager, error) {
	if strings.TrimSpace(opts.StoragePath) == "" {
		return nil, errors.New("ops: import/export storagePath required")
	}
	if len(opts.Formats) == 0 {
		opts.Formats = []string{"csv"}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	mgr := &transferManager{
		opts:    opts,
		logger:  logger.Named("transfer-manager"),
		jobs:    make(map[string]*TransferJob),
		order:   make([]string, 0, transferHistoryLimit),
		queue:   make(chan string, transferQueueSize),
		closing: make(chan struct{}),
	}
	go mgr.worker()
	return mgr, nil
}

func (m *transferManager) StartTransfer(ctx context.Context, req TransferRequest) (*TransferJob, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	kind := strings.ToLower(strings.TrimSpace(string(req.Kind)))
	if kind == "" {
		return nil, errors.New("ops: transfer kind required")
	}
	switch TransferKind(kind) {
	case TransferKindImport, TransferKindExport:
	default:
		return nil, fmt.Errorf("ops: unsupported transfer kind %q", kind)
	}
	resource := strings.TrimSpace(req.Resource)
	if resource == "" {
		return nil, errors.New("ops: resource required")
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(m.opts.Formats[0]))
	}
	if !m.formatAllowed(format) {
		return nil, fmt.Errorf("ops: format %q not allowed", format)
	}
	requestedBy := strings.TrimSpace(req.RequestedBy)
	if requestedBy == "" {
		return nil, errors.New("ops: requestedBy required")
	}
	var sourceSize int64
	if TransferKind(kind) == TransferKindImport {
		if strings.TrimSpace(req.SourceURI) == "" {
			return nil, errors.New("ops: sourceUri required for import jobs")
		}
		resolved := req.SourceURI
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(m.opts.StoragePath, resolved)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("ops: source file not found: %w", err)
		}
		if m.opts.MaxFileSize > 0 && info.Size() > m.opts.MaxFileSize {
			return nil, fmt.Errorf("ops: file exceeds max size (%d bytes)", m.opts.MaxFileSize)
		}
		sourceSize = info.Size()
	}

	job := &TransferJob{
		ID:          uuid.NewString(),
		Kind:        TransferKind(kind),
		Resource:    resource,
		Format:      format,
		Status:      TransferStatusPending,
		RequestedBy: requestedBy,
		Reason:      strings.TrimSpace(req.Reason),
		SourceURI:   strings.TrimSpace(req.SourceURI),
		TargetURI:   strings.TrimSpace(req.TargetURI),
		Metadata:    cloneStringMap(req.Metadata),
		CreatedAt:   time.Now().UTC(),
	}
	if sourceSize > 0 {
		job.SizeBytes = sourceSize
	} else if req.SizeBytes > 0 {
		job.SizeBytes = req.SizeBytes
	}
	m.storeJob(job)

	select {
	case m.queue <- job.ID:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.closing:
		return nil, errors.New("ops: transfer manager shutting down")
	}

	cloned := job.clone()
	return &cloned, nil
}

func (m *transferManager) List(limit int) []TransferJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.order) {
		limit = len(m.order)
	}
	result := make([]TransferJob, 0, limit)
	for i := 0; i < limit; i++ {
		id := m.order[i]
		if job, ok := m.jobs[id]; ok {
			result = append(result, job.clone())
		}
	}
	return result
}

func (m *transferManager) Get(jobID string) (*TransferJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, ErrTransferJobNotFound
	}
	cloned := job.clone()
	return &cloned, nil
}

func (m *transferManager) Close() {
	m.once.Do(func() {
		close(m.closing)
		close(m.queue)
	})
}

func (m *transferManager) worker() {
	for jobID := range m.queue {
		if jobID == "" {
			continue
		}
		m.run(jobID)
	}
}

func (m *transferManager) run(jobID string) {
	m.updateJob(jobID, func(job *TransferJob) {
		job.Status = TransferStatusRunning
		job.StartedAt = time.Now().UTC()
	})

	var err error
	snapshot, getErr := m.Get(jobID)
	if getErr != nil {
		err = getErr
	} else {
		switch snapshot.Kind {
		case TransferKindExport:
			err = m.generateExport(snapshot)
		case TransferKindImport:
			err = m.applyImport(snapshot)
		default:
			err = fmt.Errorf("ops: unsupported transfer kind %q", snapshot.Kind)
		}
	}

	completed := time.Now().UTC()
	m.updateJob(jobID, func(job *TransferJob) {
		job.CompletedAt = completed
		if err != nil {
			job.Status = TransferStatusFailed
			job.Error = err.Error()
			return
		}
		job.Status = TransferStatusSucceeded
		job.Error = ""
	})
	if err != nil {
		m.logger.Warn("import/export job failed", zap.String("jobId", jobID), zap.Error(err))
	}
}

func (m *transferManager) generateExport(job *TransferJob) error {
	targetDir := filepath.Join(m.opts.StoragePath, "exports", sanitizeSegment(job.Resource))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	fileName := fmt.Sprintf("%s-%s.%s", sanitizeSegment(job.Resource), time.Now().UTC().Format("20060102-150405"), job.Format)
	targetPath := filepath.Join(targetDir, fileName)
	payload, err := m.exportPayload(job)
	if err != nil {
		return err
	}
	if err := os.WriteFile(targetPath, payload, 0o600); err != nil {
		return err
	}
	info, err := os.Stat(targetPath)
	if err == nil {
		m.updateJob(job.ID, func(stored *TransferJob) {
			stored.SizeBytes = info.Size()
		})
	}
	artifactPath := targetPath
	externalURI := m.externalURI(filepath.ToSlash(filepath.Join("exports", sanitizeSegment(job.Resource), fileName)))
	m.updateJob(job.ID, func(stored *TransferJob) {
		stored.ArtifactPath = artifactPath
		stored.TargetURI = externalURI
	})
	return nil
}

func (m *transferManager) exportPayload(job *TransferJob) ([]byte, error) {
	switch job.Format {
	case "csv":
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"resource", "jobId", "generatedAt"})
		_ = writer.Write([]string{job.Resource, job.ID, time.Now().UTC().Format(time.RFC3339)})
		writer.Flush()
		return buf.Bytes(), writer.Error()
	case "json":
		body := map[string]any{
			"resource":    job.Resource,
			"jobId":       job.ID,
			"generatedAt": time.Now().UTC(),
			"metadata":    job.Metadata,
		}
		return json.MarshalIndent(body, "", "  ")
	default:
		content := fmt.Sprintf("resource=%s jobId=%s generatedAt=%s\n", job.Resource, job.ID, time.Now().UTC().Format(time.RFC3339))
		return []byte(content), nil
	}
}

func (m *transferManager) applyImport(job *TransferJob) error {
	if strings.TrimSpace(job.SourceURI) == "" {
		return errors.New("ops: sourceUri required for import jobs")
	}
	sourcePath := job.SourceURI
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(m.opts.StoragePath, sourcePath)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if m.opts.MaxFileSize > 0 && info.Size() > m.opts.MaxFileSize {
		return fmt.Errorf("ops: file exceeds max size (%d bytes)", m.opts.MaxFileSize)
	}
	targetDir := filepath.Join(m.opts.StoragePath, "imports", sanitizeSegment(job.Resource))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	fileName := fmt.Sprintf("%s-%s-%s", sanitizeSegment(job.Resource), job.ID, filepath.Base(sourcePath))
	targetPath := filepath.Join(targetDir, fileName)
	if err := copyFile(sourcePath, targetPath); err != nil {
		return err
	}
	info, err = os.Stat(targetPath)
	if err == nil {
		m.updateJob(job.ID, func(stored *TransferJob) {
			stored.SizeBytes = info.Size()
		})
	}
	m.updateJob(job.ID, func(stored *TransferJob) {
		stored.ArtifactPath = targetPath
	})
	return nil
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func (m *transferManager) storeJob(job *TransferJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = job
	m.order = append([]string{job.ID}, m.order...)
	if len(m.order) > transferHistoryLimit {
		stale := m.order[transferHistoryLimit:]
		m.order = m.order[:transferHistoryLimit]
		for _, id := range stale {
			delete(m.jobs, id)
		}
	}
}

func (m *transferManager) updateJob(jobID string, fn func(*TransferJob)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[jobID]; ok {
		fn(job)
	}
}

func (m *transferManager) formatAllowed(format string) bool {
	for _, allowed := range m.opts.Formats {
		if strings.EqualFold(strings.TrimSpace(allowed), format) {
			return true
		}
	}
	return false
}

func (m *transferManager) externalURI(relPath string) string {
	if strings.TrimSpace(m.opts.ObjectStore) == "" || strings.TrimSpace(relPath) == "" {
		return ""
	}
	base := strings.TrimRight(m.opts.ObjectStore, "/")
	return fmt.Sprintf("%s/%s", base, strings.TrimLeft(relPath, "/"))
}

func sanitizeSegment(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "data"
	}
	trimmed = strings.ToLower(trimmed)
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	result := builder.String()
	if strings.Trim(result, "-") == "" {
		return "data"
	}
	return result
}
