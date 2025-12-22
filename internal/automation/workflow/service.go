package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/automation"
)

var (
	// ErrDispatcherUnavailable indicates the workflow dispatcher dependency has not been wired.
	ErrDispatcherUnavailable = errors.New("workflow: dispatcher unavailable")
	// ErrDefinitionInactive indicates the workflow definition is not active and cannot run.
	ErrDefinitionInactive = errors.New("workflow: definition not active")
)

// JobDispatcher abstracts the automation scheduler service for enqueuing jobs.
type JobDispatcher interface {
	Enqueue(ctx context.Context, job automation.Job) error
}

// Clock abstracts time for easier testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// ServiceOptions configure the workflow service wiring.
type ServiceOptions struct {
	Repository Repository
	Dispatcher JobDispatcher
	Logger     *zap.Logger
	Clock      Clock
}

// Service coordinates workflow definition CRUD and dispatcher hand-offs.
type Service struct {
	repo       Repository
	dispatcher JobDispatcher
	logger     *zap.Logger
	clock      Clock
}

// NewService constructs a workflow service instance.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Repository == nil {
		return nil, errors.New("workflow: repository required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	clock := opts.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Service{
		repo:       opts.Repository,
		dispatcher: opts.Dispatcher,
		logger:     logger,
		clock:      clock,
	}, nil
}

// CreateDefinitionInput captures fields required to store a new workflow definition.
type CreateDefinitionInput struct {
	ID          string
	Name        string
	Description string
	Labels      map[string]string
	Spec        Spec
	Status      DefinitionStatus
}

// CreateDefinition stores a new workflow definition in draft or active state.
func (s *Service) CreateDefinition(ctx context.Context, in CreateDefinitionInput) (*Definition, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("workflow: name required")
	}
	if err := in.Spec.Validate(); err != nil {
		return nil, err
	}
	def := &Definition{
		ID:          in.ID,
		Version:     1,
		Name:        in.Name,
		Description: in.Description,
		Labels:      cloneStringMap(in.Labels),
		Spec:        in.Spec,
		Status:      in.Status,
		CreatedAt:   s.clock.Now(),
	}
	if strings.TrimSpace(def.ID) == "" {
		def.ID = uuid.NewString()
	}
	if def.Status == "" {
		def.Status = DefinitionStatusDraft
	}
	def.UpdatedAt = def.CreatedAt

	if err := s.repo.CreateDefinition(ctx, def); err != nil {
		return nil, err
	}
	s.logger.Info("workflow definition created", zap.String("definitionId", def.ID), zap.String("status", string(def.Status)))
	return def, nil
}

// UpdateDefinitionInput contains data for updating an existing definition.
type UpdateDefinitionInput struct {
	ID          string
	Version     int
	Name        string
	Description string
	Labels      map[string]string
	Spec        Spec
	Status      DefinitionStatus
}

// UpdateDefinition updates the workflow definition and bumps the version.
func (s *Service) UpdateDefinition(ctx context.Context, in UpdateDefinitionInput) (*Definition, error) {
	if strings.TrimSpace(in.ID) == "" {
		return nil, errors.New("workflow: id required")
	}
	if in.Version <= 0 {
		return nil, errors.New("workflow: current version required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("workflow: name required")
	}
	if err := in.Spec.Validate(); err != nil {
		return nil, err
	}
	def := &Definition{
		ID:          in.ID,
		Version:     in.Version,
		Name:        in.Name,
		Description: in.Description,
		Labels:      cloneStringMap(in.Labels),
		Spec:        in.Spec,
		Status:      in.Status,
	}
	if def.Status == "" {
		def.Status = DefinitionStatusDraft
	}
	if err := s.repo.UpdateDefinition(ctx, def); err != nil {
		return nil, err
	}
	s.logger.Info("workflow definition updated", zap.String("definitionId", def.ID), zap.Int("version", def.Version))
	return def, nil
}

// PublishDefinition marks the workflow as active by bumping its version.
func (s *Service) PublishDefinition(ctx context.Context, id string, version int) (*Definition, error) {
	def, err := s.repo.GetDefinition(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if def.Status == DefinitionStatusArchived {
		return nil, errors.New("workflow: cannot publish archived definition")
	}
	def.Status = DefinitionStatusActive
	if err := s.repo.UpdateDefinition(ctx, def); err != nil {
		return nil, err
	}
	s.logger.Info("workflow definition published", zap.String("definitionId", def.ID), zap.Int("version", def.Version))
	return def, nil
}

// ArchiveDefinition flips a definition into archived state.
func (s *Service) ArchiveDefinition(ctx context.Context, id string) error {
	if err := s.repo.ArchiveDefinition(ctx, id); err != nil {
		return err
	}
	s.logger.Info("workflow definition archived", zap.String("definitionId", id))
	return nil
}

// GetDefinition fetches a definition by ID/version (latest version when version <= 0).
func (s *Service) GetDefinition(ctx context.Context, id string, version int) (*Definition, error) {
	return s.repo.GetDefinition(ctx, id, version)
}

// ListDefinitions delegates to the repository listing helper.
func (s *Service) ListDefinitions(ctx context.Context, opts ListDefinitionsOptions) ([]Definition, error) {
	return s.repo.ListDefinitions(ctx, opts)
}

// StartExecutionInput encapsulates a workflow run request.
type StartExecutionInput struct {
	DefinitionID string
	Version      int
	TenantID     string
	TriggeredBy  string
	Context      map[string]any
	Labels       map[string]string
	Priority     int
}

// StartExecution validates the definition and enqueues a workflow job.
func (s *Service) StartExecution(ctx context.Context, in StartExecutionInput) (string, error) {
	if s.dispatcher == nil {
		return "", ErrDispatcherUnavailable
	}
	if strings.TrimSpace(in.DefinitionID) == "" {
		return "", errors.New("workflow: definitionId required")
	}
	if strings.TrimSpace(in.TenantID) == "" {
		return "", errors.New("workflow: tenantId required")
	}
	def, err := s.repo.GetDefinition(ctx, in.DefinitionID, in.Version)
	if err != nil {
		return "", err
	}
	if def.Status != DefinitionStatusActive {
		return "", ErrDefinitionInactive
	}

	payloadRaw, err := json.Marshal(executionPayload{
		DefinitionID: def.ID,
		Version:      def.Version,
		TenantID:     in.TenantID,
		TriggeredBy:  in.TriggeredBy,
		Context:      in.Context,
		Labels:       cloneStringMap(def.Labels),
		Spec:         def.Spec,
	})
	if err != nil {
		return "", fmt.Errorf("workflow: marshal execution payload: %w", err)
	}

	jobID := uuid.NewString()
	triggeredBy := strings.TrimSpace(in.TriggeredBy)
	if triggeredBy == "" {
		triggeredBy = automation.JobActorWorkflow
	}
	job := automation.Job{
		ID:          jobID,
		Type:        automation.JobWorkflowExecution,
		TenantID:    in.TenantID,
		Labels:      mergeLabels(def.Labels, in.Labels),
		Payload:     payloadRaw,
		Priority:    in.Priority,
		Source:      automation.JobSourceWorkflow,
		TriggeredBy: triggeredBy,
	}

	if err := s.dispatcher.Enqueue(ctx, job); err != nil {
		return "", err
	}
	s.logger.Info("workflow execution enqueued",
		zap.String("definitionId", def.ID),
		zap.Int("version", def.Version),
		zap.String("jobId", jobID))
	return jobID, nil
}

// executionPayload is embedded in workflow execution jobs.
type executionPayload struct {
	DefinitionID string            `json:"definitionId"`
	Version      int               `json:"version"`
	TenantID     string            `json:"tenantId"`
	TriggeredBy  string            `json:"triggeredBy,omitempty"`
	Context      map[string]any    `json:"context,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Spec         Spec              `json:"spec"`
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeLabels(base map[string]string, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	out := cloneStringMap(base)
	if out == nil {
		out = make(map[string]string, len(overrides))
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
