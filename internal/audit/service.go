package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/pkg/models"
)

var (
	ErrTenantRequired = errors.New("tenant id required")
	ErrActorRequired  = errors.New("actor required")
	ErrActionRequired = errors.New("action required")
	ErrInvalidPayload = errors.New("payload must be valid json")
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// ListEventsFilter constrains queries across actors/actions/resources/correlation IDs.
type ListEventsFilter struct {
	Actor         string
	Actions       []string
	Resource      string
	CorrelationID string
	Limit         int
	Offset        int
}

// Service coordinates audit event capture and queries.
type Service struct {
	repo   Repository
	logger *zap.Logger
}

// RecordEventRequest describes a new audit entry to persist.
type RecordEventRequest struct {
	TenantID      string
	Actor         string
	Action        string
	Source        string
	Resource      string
	AuditID       string
	CorrelationID string
	Payload       json.RawMessage
}

// NewService builds an audit service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// RecordEvent validates and persists an audit record.
func (s *Service) RecordEvent(ctx context.Context, req RecordEventRequest) error {
	if req.TenantID == "" {
		return ErrTenantRequired
	}
	if req.Actor == "" {
		return ErrActorRequired
	}
	if req.Action == "" {
		return ErrActionRequired
	}
	if len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return ErrInvalidPayload
	}

	if req.Source == "" {
		req.Source = "api"
	}
	auditID := req.AuditID
	if auditID == "" {
		auditID = uuid.NewString()
	}
	event := &models.AuditEvent{
		AuditID:       auditID,
		TenantID:      req.TenantID,
		Actor:         req.Actor,
		Action:        req.Action,
		Source:        req.Source,
		Resource:      req.Resource,
		CorrelationID: req.CorrelationID,
		Payload:       req.Payload,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.repo.InsertEvent(ctx, event); err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.Debug("recorded audit event", zap.String("tenantId", req.TenantID), zap.String("action", req.Action))
	}
	return nil
}

// ListEvents fetches recent audit entries for a tenant.
func (s *Service) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]models.AuditEvent, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListEvents(ctx, tenantID, limit, offset)
}

// ListEventsFiltered returns audit entries respecting the supplied filter options.
func (s *Service) ListEventsFiltered(ctx context.Context, tenantID string, filter ListEventsFilter) ([]models.AuditEvent, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultListLimit
	}
	if filter.Limit > maxListLimit {
		filter.Limit = maxListLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListEventsFiltered(ctx, tenantID, filter)
}
