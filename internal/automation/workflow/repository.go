package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	// ErrDefinitionNotFound is returned when a workflow definition cannot be located.
	ErrDefinitionNotFound = errors.New("workflow: definition not found")
	// ErrDefinitionConflict is returned when a conflicting definition already exists.
	ErrDefinitionConflict = errors.New("workflow: definition conflict")
)

// Repository provides persistence helpers for workflow definitions.
type Repository interface {
	CreateDefinition(ctx context.Context, def *Definition) error
	UpdateDefinition(ctx context.Context, def *Definition) error
	ArchiveDefinition(ctx context.Context, id string) error
	GetDefinition(ctx context.Context, id string, version int) (*Definition, error)
	ListDefinitions(ctx context.Context, opts ListDefinitionsOptions) ([]Definition, error)
}

// ListDefinitionsOptions filters a definition listing request.
type ListDefinitionsOptions struct {
	Statuses []DefinitionStatus
	Search   string
	Limit    int
	Offset   int
}

// NewRepository constructs a Repository backed by sqlx.
func NewRepository(db *sqlx.DB) Repository {
	return &mysqlRepository{db: db}
}

var _ Repository = (*mysqlRepository)(nil)

type mysqlRepository struct {
	db *sqlx.DB
}

// CreateDefinition persists a new workflow definition (version 1 by default).
func (r *mysqlRepository) CreateDefinition(ctx context.Context, def *Definition) error {
	if def == nil {
		return errors.New("workflow: definition required")
	}
	if err := validateDefinition(def); err != nil {
		return err
	}
	if def.Version == 0 {
		def.Version = 1
	}
	if def.Status == "" {
		def.Status = DefinitionStatusDraft
	}
	now := time.Now().UTC()
	if def.CreatedAt.IsZero() {
		def.CreatedAt = now
	}
	def.UpdatedAt = def.CreatedAt

	labelsJSON, err := encodeLabels(def.Labels)
	if err != nil {
		return err
	}
	specJSON, err := json.Marshal(def.Spec)
	if err != nil {
		return fmt.Errorf("workflow: marshal spec: %w", err)
	}

	params := map[string]any{
		"id":          def.ID,
		"version":     def.Version,
		"name":        def.Name,
		"description": def.Description,
		"labels":      labelsJSON,
		"spec":        specJSON,
		"status":      def.Status,
		"created_at":  def.CreatedAt,
		"updated_at":  def.UpdatedAt,
	}

	const query = `
INSERT INTO workflow_definitions (
    id, version, name, description, labels, spec, status, created_at, updated_at)
VALUES (
    :id, :version, :name, :description, :labels, :spec, :status, :created_at, :updated_at)`

	if _, err := r.db.NamedExecContext(ctx, query, params); err != nil {
		if isDuplicateErr(err) {
			return ErrDefinitionConflict
		}
		return err
	}
	return nil
}

// UpdateDefinition applies a new revision to an existing workflow definition.
func (r *mysqlRepository) UpdateDefinition(ctx context.Context, def *Definition) error {
	if def == nil {
		return errors.New("workflow: definition required")
	}
	if def.Version <= 0 {
		return errors.New("workflow: current version required for update")
	}
	if err := validateDefinition(def); err != nil {
		return err
	}

	labelsJSON, err := encodeLabels(def.Labels)
	if err != nil {
		return err
	}
	specJSON, err := json.Marshal(def.Spec)
	if err != nil {
		return fmt.Errorf("workflow: marshal spec: %w", err)
	}

	prevVersion := def.Version
	now := time.Now().UTC()
	def.UpdatedAt = now

	params := map[string]any{
		"id":           def.ID,
		"name":         def.Name,
		"description":  def.Description,
		"labels":       labelsJSON,
		"spec":         specJSON,
		"status":       def.Status,
		"updated_at":   def.UpdatedAt,
		"prev_version": prevVersion,
	}

	const query = `
UPDATE workflow_definitions
SET name = :name,
    description = :description,
    labels = :labels,
    spec = :spec,
    status = :status,
    updated_at = :updated_at,
    version = version + 1
WHERE id = :id AND version = :prev_version`

	res, err := r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDefinitionNotFound
	}
	def.Version = prevVersion + 1
	return nil
}

// ArchiveDefinition marks a workflow definition as archived (read-only).
func (r *mysqlRepository) ArchiveDefinition(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("workflow: id required")
	}
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
UPDATE workflow_definitions
SET status = ?, updated_at = ?, version = version + 1
WHERE id = ? AND status != ?`, DefinitionStatusArchived, now, id, DefinitionStatusArchived)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDefinitionNotFound
	}
	return nil
}

// GetDefinition returns a workflow definition by ID and optional version (latest when <= 0).
func (r *mysqlRepository) GetDefinition(ctx context.Context, id string, version int) (*Definition, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("workflow: id required")
	}

	baseQuery := `
SELECT id, version, name, description, labels, spec, status, created_at, updated_at
FROM workflow_definitions
WHERE id = ?`
	args := []any{id}

	if version > 0 {
		baseQuery += " AND version = ?"
		args = append(args, version)
	} else {
		baseQuery += " ORDER BY version DESC LIMIT 1"
	}

	var rec definitionRecord
	if err := r.db.GetContext(ctx, &rec, baseQuery, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDefinitionNotFound
		}
		return nil, err
	}
	return rec.toDefinition()
}

// ListDefinitions returns workflow definitions filtered by the provided options.
func (r *mysqlRepository) ListDefinitions(ctx context.Context, opts ListDefinitionsOptions) ([]Definition, error) {
	query := `
SELECT id, version, name, description, labels, spec, status, created_at, updated_at
FROM workflow_definitions
WHERE 1 = 1`
	args := make([]any, 0)

	if len(opts.Statuses) > 0 {
		placeholders := make([]string, len(opts.Statuses))
		for i, status := range opts.Statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		query += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}

	if search := strings.TrimSpace(opts.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query += " AND (LOWER(name) LIKE ? OR LOWER(description) LIKE ?)"
		args = append(args, like, like)
	}

	query += " ORDER BY updated_at DESC, id ASC"

	limit := clampLimit(opts.Limit)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	var rows []definitionRecord
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	defs := make([]Definition, 0, len(rows))
	for _, rec := range rows {
		def, err := rec.toDefinition()
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}
	return defs, nil
}

// definitionRecord reflects the stored workflow_definitions table shape.
type definitionRecord struct {
	ID          string           `db:"id"`
	Version     int              `db:"version"`
	Name        string           `db:"name"`
	Description string           `db:"description"`
	LabelsJSON  []byte           `db:"labels"`
	SpecJSON    []byte           `db:"spec"`
	Status      DefinitionStatus `db:"status"`
	CreatedAt   time.Time        `db:"created_at"`
	UpdatedAt   time.Time        `db:"updated_at"`
}

func (r definitionRecord) toDefinition() (*Definition, error) {
	var spec Spec
	if len(r.SpecJSON) == 0 {
		return nil, errors.New("workflow: definition missing spec payload")
	}
	if err := json.Unmarshal(r.SpecJSON, &spec); err != nil {
		return nil, fmt.Errorf("workflow: unmarshal spec: %w", err)
	}

	labels, err := decodeLabels(r.LabelsJSON)
	if err != nil {
		return nil, err
	}

	def := &Definition{
		ID:          r.ID,
		Version:     r.Version,
		Name:        r.Name,
		Description: r.Description,
		Labels:      labels,
		Spec:        spec,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	return def, nil
}

func validateDefinition(def *Definition) error {
	if strings.TrimSpace(def.ID) == "" {
		return errors.New("workflow: definition id required")
	}
	if strings.TrimSpace(def.Name) == "" {
		return errors.New("workflow: definition name required")
	}
	if err := def.Spec.Validate(); err != nil {
		return err
	}
	return nil
}

func encodeLabels(labels map[string]string) ([]byte, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	return json.Marshal(labels)
}

func decodeLabels(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var labels map[string]string
	if err := json.Unmarshal(raw, &labels); err != nil {
		return nil, fmt.Errorf("workflow: unmarshal labels: %w", err)
	}
	return labels, nil
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

func isDuplicateErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
