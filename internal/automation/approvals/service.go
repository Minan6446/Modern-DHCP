package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Applier applies approved automation changes.
type Applier interface {
	ApplyChange(ctx context.Context, request ChangeRequest) error
}

// ServiceOptions configure the approval service wiring.
type ServiceOptions struct {
	Repository         Repository
	Logger             *zap.Logger
	AutoApplyOnApprove bool
	Applier            Applier
}

// Service coordinates automation change approvals.
type Service struct {
	repo               Repository
	logger             *zap.Logger
	autoApplyOnApprove bool
	applier            Applier
}

// NewService builds a Service instance.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Repository == nil {
		return nil, errors.New("automation: approvals repository required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		repo:               opts.Repository,
		logger:             logger.Named("automation-approvals"),
		autoApplyOnApprove: opts.AutoApplyOnApprove,
		applier:            opts.Applier,
	}, nil
}

// SubmitChangeInput captures data necessary to register a change.
type SubmitChangeInput struct {
	TenantID        string
	JobType         string
	RequestType     RequestType
	RequestedBy     string
	OriginalConfig  json.RawMessage
	ProposedConfig  json.RawMessage
	Payload         json.RawMessage
	RequireApproval bool
	AutoApply       bool
}

// SubmitChange stores a change request, optionally auto-applies when approval is not required.
func (s *Service) SubmitChange(ctx context.Context, in SubmitChangeInput) (*ChangeRequest, error) {
	if strings.TrimSpace(in.JobType) == "" {
		return nil, errors.New("automation: jobType required")
	}
	if strings.TrimSpace(in.RequestedBy) == "" {
		return nil, errors.New("automation: requestedBy required")
	}
	if in.RequestType == "" {
		return nil, errors.New("automation: requestType required")
	}
	now := time.Now().UTC()
	status := RequestStatusPending
	approver := ""
	decidedAt := (*time.Time)(nil)
	decisionNote := ""
	autoApplied := false
	requireApproval := in.RequireApproval
	if !requireApproval {
		status = RequestStatusApproved
		approver = "system"
		decidedAt = &now
		decisionNote = "auto-approved"
		autoApplied = true
	}
	request := ChangeRequest{
		ID:          uuid.NewString(),
		JobType:     strings.TrimSpace(in.JobType),
		RequestType: in.RequestType,
		Status:      status,
		RequestedBy: strings.TrimSpace(in.RequestedBy),
		RequestedAt: now,
		ApproverID:  approver,
	}
	if decidedAt != nil {
		request.DecidedAt = decidedAt
		request.DecisionNote = decisionNote
	}
	request.OriginalConfig = cloneJSON(in.OriginalConfig)
	request.ProposedConfig = cloneJSON(in.ProposedConfig)
	request.Payload = cloneJSON(in.Payload)
	request.CreatedAt = now
	request.UpdatedAt = now
	request.AutoApplied = autoApplied
	if err := s.repo.Create(ctx, &request); err != nil {
		return nil, err
	}
	cloned := request.Clone()
	if !requireApproval && (in.AutoApply || s.autoApplyOnApprove) {
		if err := s.applyAndMark(ctx, cloned); err != nil {
			s.logger.Warn("failed to auto-apply automation change", zap.String("requestId", cloned.ID), zap.Error(err))
		}
	}
	return &cloned, nil
}

// Get retrieves a change request by identifier.
func (s *Service) Get(ctx context.Context, id string) (*ChangeRequest, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("automation: approvals repository unavailable")
	}
	return s.repo.Get(ctx, strings.TrimSpace(id))
}

// List queries change requests via repository filters.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]ChangeRequest, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("automation: approvals repository unavailable")
	}
	entries, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	results := make([]ChangeRequest, 0, len(entries))
	for _, entry := range entries {
		results = append(results, entry.Clone())
	}
	return results, nil
}

// Approve marks a pending request as approved and optionally applies it.
func (s *Service) Approve(ctx context.Context, id, approver, note string) (*ChangeRequest, error) {
	updated, err := s.repo.UpdateDecision(ctx, id, approver, note, RequestStatusApproved, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if s.autoApplyOnApprove {
		if err := s.applyAndMark(ctx, updated.Clone()); err != nil {
			s.logger.Warn("failed to auto-apply approved change", zap.String("requestId", updated.ID), zap.Error(err))
		}
	}
	return updated, nil
}

// Reject marks a pending request as rejected.
func (s *Service) Reject(ctx context.Context, id, approver, note string) (*ChangeRequest, error) {
	return s.repo.UpdateDecision(ctx, id, approver, note, RequestStatusRejected, time.Now().UTC())
}

// Apply executes the change via the configured applier when approved.
func (s *Service) Apply(ctx context.Context, id string) (*ChangeRequest, error) {
	if s.applier == nil {
		return nil, errors.New("automation: approval applier unavailable")
	}
	request, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if request.Status != RequestStatusApproved {
		return nil, fmt.Errorf("automation: change request %s not approved", request.ID)
	}
	if err := s.applyAndMark(ctx, request.Clone()); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, request.ID)
}

// applyAndMark invokes the applier and updates persistence.
func (s *Service) applyAndMark(ctx context.Context, request ChangeRequest) error {
	if s.applier == nil {
		return errors.New("automation: no applier configured")
	}
	if err := s.applier.ApplyChange(ctx, request); err != nil {
		return err
	}
	return s.repo.MarkApplied(ctx, request.ID, time.Now().UTC())
}
