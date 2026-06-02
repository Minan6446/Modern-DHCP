package automation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// JobRun captures a persisted automation job execution record.
type JobRun struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenantId"`
	Type          JobType           `json:"type"`
	Status        JobStatus         `json:"status"`
	Source        string            `json:"source"`
	TriggeredBy   string            `json:"triggeredBy,omitempty"`
	Priority      int               `json:"priority"`
	Attempts      int               `json:"attempts"`
	PayloadHash   string            `json:"payloadHash,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Payload       json.RawMessage   `json:"payload,omitempty"`
	ResultSummary string            `json:"resultSummary,omitempty"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	NotBefore     *time.Time        `json:"notBefore,omitempty"`
	QueuedAt      time.Time         `json:"queuedAt"`
	StartedAt     *time.Time        `json:"startedAt,omitempty"`
	CompletedAt   *time.Time        `json:"completedAt,omitempty"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// ListJobsOptions tunes listing queries.
type ListJobsOptions struct {
	TenantID    string
	Types       []JobType
	Statuses    []JobStatus
	Sources     []string
	TriggeredBy string
	Limit       int
	Offset      int
}

// JobRecorder tracks lifecycle transitions for jobs queued through the scheduler.
type JobRecorder interface {
	RecordPending(ctx context.Context, job Job) error
	MarkRunning(ctx context.Context, job Job) error
	MarkSucceeded(ctx context.Context, job Job, summary string) error
	MarkFailed(ctx context.Context, job Job, summary, errorMessage string) error
}

// JobStore exposes persistence helpers for automation job history.
type JobStore interface {
	JobRecorder
	ListJobs(ctx context.Context, opts ListJobsOptions) ([]JobRun, error)
	CountJobs(ctx context.Context, opts ListJobsOptions) (int, error)
	GetJob(ctx context.Context, id string) (*JobRun, error)
}

type sqlJobStore struct {
	db *sqlx.DB
}

// NewJobStore wires a SQL-backed job history store.
func NewJobStore(db *sqlx.DB) JobStore {
	return &sqlJobStore{db: db}
}

func (s *sqlJobStore) RecordPending(ctx context.Context, job Job) error {
	if s == nil || s.db == nil {
		return nil
	}
	params, err := buildJobParams(job)
	if err != nil {
		return err
	}
	params["status"] = JobStatusPending
	params["queued_at"] = normalizeTime(job.CreatedAt)
	params["updated_at"] = time.Now().UTC()
	params["attempts"] = job.Attempts

	const insert = `
INSERT INTO automation_jobs (
	id, job_type, status, source, triggered_by, priority, attempts,
    payload_hash, labels, payload, result_summary, error_message, not_before,
    queued_at, started_at, completed_at, updated_at)
VALUES (
	:id, :job_type, :status, :source, :triggered_by, :priority, :attempts,
    :payload_hash, :labels, :payload, '', '', :not_before,
    :queued_at, NULL, NULL, :updated_at)`

	_, err = s.db.NamedExecContext(ctx, insert, params)
	if err == nil {
		return nil
	}
	if !isDuplicateErr(err) {
		return err
	}

	const update = `
UPDATE automation_jobs
SET status = :status,
    attempts = :attempts,
    not_before = :not_before,
    payload = :payload,
    labels = :labels,
    priority = :priority,
	    result_summary = '',
	    error_message = '',
	    started_at = NULL,
	    completed_at = NULL,
    updated_at = :updated_at
WHERE id = :id`
	_, err = s.db.NamedExecContext(ctx, update, params)
	return err
}

func (s *sqlJobStore) MarkRunning(ctx context.Context, job Job) error {
	if s == nil || s.db == nil {
		return nil
	}
	attempts := job.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	now := time.Now().UTC()
	query := `
UPDATE automation_jobs
SET status = ?, attempts = ?, started_at = ?, updated_at = ?
WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, JobStatusRunning, attempts, now, now, job.ID)
	return err
}

func (s *sqlJobStore) MarkSucceeded(ctx context.Context, job Job, summary string) error {
	if s == nil || s.db == nil {
		return nil
	}
	attempts := job.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	now := time.Now().UTC()
	query := `
UPDATE automation_jobs
SET status = ?, attempts = ?, result_summary = ?, error_message = '', completed_at = ?, updated_at = ?
WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, JobStatusSucceeded, attempts, summary, now, now, job.ID)
	return err
}

func (s *sqlJobStore) MarkFailed(ctx context.Context, job Job, summary, errMsg string) error {
	if s == nil || s.db == nil {
		return nil
	}
	attempts := job.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	now := time.Now().UTC()
	query := `
UPDATE automation_jobs
SET status = ?, attempts = ?, result_summary = ?, error_message = ?, completed_at = ?, updated_at = ?
WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, JobStatusFailed, attempts, summary, errMsg, now, now, job.ID)
	return err
}

func (s *sqlJobStore) ListJobs(ctx context.Context, opts ListJobsOptions) ([]JobRun, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	query := `
SELECT id, job_type, status, source, triggered_by, priority, attempts,
       payload_hash, labels, payload, result_summary, error_message, not_before,
       queued_at, started_at, completed_at, updated_at
FROM automation_jobs
WHERE 1 = 1`
	args := make([]any, 0)
	if len(opts.Types) > 0 {
		query += " AND job_type IN (" + placeholders(len(opts.Types)) + ")"
		for _, t := range opts.Types {
			args = append(args, t)
		}
	}
	if len(opts.Statuses) > 0 {
		query += " AND status IN (" + placeholders(len(opts.Statuses)) + ")"
		for _, st := range opts.Statuses {
			args = append(args, st)
		}
	}
	if len(opts.Sources) > 0 {
		query += " AND source IN (" + placeholders(len(opts.Sources)) + ")"
		for _, src := range opts.Sources {
			args = append(args, src)
		}
	}
	if trig := strings.TrimSpace(opts.TriggeredBy); trig != "" {
		query += " AND triggered_by = ?"
		args = append(args, trig)
	}
	query += " ORDER BY updated_at DESC, id DESC"
	limit := clampLimit(opts.Limit)
	query += " LIMIT ?"
	args = append(args, limit)
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows := make([]jobRow, 0, limit)
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	runs := make([]JobRun, 0, len(rows))
	for _, row := range rows {
		run, err := row.toJobRun()
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *sqlJobStore) CountJobs(ctx context.Context, opts ListJobsOptions) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	query := "SELECT COUNT(1) FROM automation_jobs WHERE 1 = 1"
	args := make([]any, 0)
	if len(opts.Types) > 0 {
		query += " AND job_type IN (" + placeholders(len(opts.Types)) + ")"
		for _, t := range opts.Types {
			args = append(args, t)
		}
	}
	if len(opts.Statuses) > 0 {
		query += " AND status IN (" + placeholders(len(opts.Statuses)) + ")"
		for _, st := range opts.Statuses {
			args = append(args, st)
		}
	}
	if len(opts.Sources) > 0 {
		query += " AND source IN (" + placeholders(len(opts.Sources)) + ")"
		for _, src := range opts.Sources {
			args = append(args, src)
		}
	}
	if trig := strings.TrimSpace(opts.TriggeredBy); trig != "" {
		query += " AND triggered_by = ?"
		args = append(args, trig)
	}
	var count int
	if err := s.db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *sqlJobStore) GetJob(ctx context.Context, id string) (*JobRun, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("automation: job store unavailable")
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("automation: job id required")
	}
	row := jobRow{}
	query := `
SELECT id, job_type, status, source, triggered_by, priority, attempts,
       payload_hash, labels, payload, result_summary, error_message, not_before,
       queued_at, started_at, completed_at, updated_at
FROM automation_jobs WHERE id = ?`
	if err := s.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	run, err := row.toJobRun()
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ErrJobNotFound indicates the requested job history entry does not exist.
var ErrJobNotFound = errors.New("automation: job not found")

type jobRow struct {
	ID            string         `db:"id"`
	Type          string         `db:"job_type"`
	Status        string         `db:"status"`
	Source        string         `db:"source"`
	TriggeredBy   string         `db:"triggered_by"`
	Priority      int            `db:"priority"`
	Attempts      int            `db:"attempts"`
	PayloadHash   string         `db:"payload_hash"`
	LabelsJSON    []byte         `db:"labels"`
	Payload       []byte         `db:"payload"`
	ResultSummary sql.NullString `db:"result_summary"`
	ErrorMessage  sql.NullString `db:"error_message"`
	NotBefore     sql.NullTime   `db:"not_before"`
	QueuedAt      time.Time      `db:"queued_at"`
	StartedAt     sql.NullTime   `db:"started_at"`
	CompletedAt   sql.NullTime   `db:"completed_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

func (r jobRow) toJobRun() (JobRun, error) {
	labels, err := decodeLabelsJSON(r.LabelsJSON)
	if err != nil {
		return JobRun{}, err
	}
	run := JobRun{
		ID:          r.ID,
		Type:        JobType(r.Type),
		Status:      JobStatus(r.Status),
		Source:      r.Source,
		TriggeredBy: r.TriggeredBy,
		Priority:    r.Priority,
		Attempts:    r.Attempts,
		PayloadHash: r.PayloadHash,
		Labels:      labels,
		Payload:     cloneRawMessage(json.RawMessage(r.Payload)),
		QueuedAt:    r.QueuedAt.UTC(),
		UpdatedAt:   r.UpdatedAt.UTC(),
	}
	if r.ResultSummary.Valid {
		run.ResultSummary = r.ResultSummary.String
	}
	if r.ErrorMessage.Valid {
		run.ErrorMessage = r.ErrorMessage.String
	}
	if r.NotBefore.Valid {
		ts := r.NotBefore.Time.UTC()
		run.NotBefore = &ts
	}
	if r.StartedAt.Valid {
		ts := r.StartedAt.Time.UTC()
		run.StartedAt = &ts
	}
	if r.CompletedAt.Valid {
		ts := r.CompletedAt.Time.UTC()
		run.CompletedAt = &ts
	}
	return run, nil
}

func buildJobParams(job Job) (map[string]any, error) {
	labelsJSON, err := encodeLabels(job.Labels)
	if err != nil {
		return nil, err
	}
	payloadHash := hashPayload(job.Payload)
	params := map[string]any{
		"id":           job.ID,
		"job_type":     job.Type,
		"source":       strings.TrimSpace(job.Source),
		"triggered_by": strings.TrimSpace(job.TriggeredBy),
		"priority":     job.Priority,
		"attempts":     job.Attempts,
		"payload_hash": payloadHash,
		"labels":       labelsJSON,
		"payload":      cloneRawMessage(job.Payload),
		"not_before":   nullableTime(job.NotBefore),
	}
	return params, nil
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	ts := value.UTC()
	return &ts
}

func encodeLabels(labels map[string]string) ([]byte, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	return json.Marshal(labels)
}

func decodeLabelsJSON(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var labels map[string]string
	if err := json.Unmarshal(raw, &labels); err != nil {
		return nil, fmt.Errorf("automation: decode labels: %w", err)
	}
	return labels, nil
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	clone := make([]byte, len(raw))
	copy(clone, raw)
	return json.RawMessage(clone)
}

func hashPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func placeholders(length int) string {
	parts := make([]string, length)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 500:
		return 500
	default:
		return limit
	}
}

func normalizeTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func isDuplicateErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	var pqErr interface{ SQLState() string }
	if errors.As(err, &pqErr) {
		// PostgreSQL unique_violation SQLSTATE 23505
		return pqErr.SQLState() == "23505"
	}
	return false
}
