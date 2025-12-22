package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/events"
	"modern-dhcp/pkg/models"
)

var (
	ErrPriorityRange = errors.New("priority must be >= 0")
)

// Service exposes CRUD helpers for policy rules.
type Service struct {
	repo      Repository
	engine    *Engine
	publisher events.PolicyPublisher
	logger    *zap.Logger
}

// NewService creates a policy service.
func NewService(repo Repository, engine *Engine, publisher events.PolicyPublisher, logger *zap.Logger) *Service {
	return &Service{repo: repo, engine: engine, publisher: publisher, logger: logger}
}

// CreateRuleRequest carries user-supplied rule data.
type CreateRuleRequest struct {
	TenantID   string
	Priority   int
	Conditions json.RawMessage
	Actions    json.RawMessage
	Enabled    bool
}

// UpdateRuleRequest updates an existing rule.
type UpdateRuleRequest struct {
	RuleID string
	CreateRuleRequest
}

func (s *Service) ListRules(ctx context.Context, tenantID string, limit, offset int) ([]models.PolicyRule, error) {
	return s.repo.ListRules(ctx, tenantID, limit, offset)
}

func (s *Service) CreateRule(ctx context.Context, req CreateRuleRequest) (*models.PolicyRule, error) {
	if req.Priority < 0 {
		return nil, ErrPriorityRange
	}
	if err := ValidateConditions(req.Conditions); err != nil {
		return nil, fmt.Errorf("invalid conditions: %w", err)
	}
	if err := ValidateActions(req.Actions); err != nil {
		return nil, fmt.Errorf("invalid actions: %w", err)
	}
	now := time.Now().UTC()
	rule := &models.PolicyRule{
		ID:         uuid.NewString(),
		TenantID:   req.TenantID,
		Priority:   req.Priority,
		Conditions: req.Conditions,
		Actions:    req.Actions,
		Enabled:    req.Enabled,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.InsertRule(ctx, rule); err != nil {
		return nil, err
	}
	s.invalidate(req.TenantID)
	s.publishEvent(ctx, "created", rule)
	return rule, nil
}

func (s *Service) UpdateRule(ctx context.Context, req UpdateRuleRequest) (*models.PolicyRule, error) {
	if req.Priority < 0 {
		return nil, ErrPriorityRange
	}
	if len(req.Conditions) > 0 {
		if err := ValidateConditions(req.Conditions); err != nil {
			return nil, fmt.Errorf("invalid conditions: %w", err)
		}
	}
	if len(req.Actions) > 0 {
		if err := ValidateActions(req.Actions); err != nil {
			return nil, fmt.Errorf("invalid actions: %w", err)
		}
	}
	rule, err := s.repo.GetRule(ctx, req.TenantID, req.RuleID)
	if err != nil {
		return nil, err
	}
	rule.Priority = req.Priority
	if len(req.Conditions) > 0 {
		rule.Conditions = req.Conditions
	}
	if len(req.Actions) > 0 {
		rule.Actions = req.Actions
	}
	rule.Enabled = req.Enabled
	rule.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateRule(ctx, rule); err != nil {
		return nil, err
	}
	s.invalidate(req.TenantID)
	s.publishEvent(ctx, "updated", rule)
	return rule, nil
}

func (s *Service) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRule(ctx, tenantID, ruleID); err != nil {
		return err
	}
	s.invalidate(tenantID)
	s.publishEvent(ctx, "deleted", rule)
	return nil
}

func (s *Service) invalidate(tenantID string) {
	if s.engine != nil {
		s.engine.Invalidate(tenantID)
	}
	s.logger.Debug("invalidated policy cache", zap.String("tenantId", tenantID))
}

func (s *Service) publishEvent(ctx context.Context, action string, rule *models.PolicyRule) {
	if s.publisher == nil || rule == nil {
		return
	}
	body, err := json.Marshal(struct {
		ID         string          `json:"id"`
		TenantID   string          `json:"tenantId"`
		Priority   int             `json:"priority"`
		Conditions json.RawMessage `json:"conditions"`
		Actions    json.RawMessage `json:"actions"`
		Enabled    bool            `json:"enabled"`
	}{
		ID:         rule.ID,
		TenantID:   rule.TenantID,
		Priority:   rule.Priority,
		Conditions: rule.Conditions,
		Actions:    rule.Actions,
		Enabled:    rule.Enabled,
	})
	if err != nil {
		s.logger.Warn("marshal policy event", zap.Error(err))
		return
	}
	evt := events.PolicyEvent{
		Action:   action,
		TenantID: rule.TenantID,
		RuleID:   rule.ID,
		Body:     body,
		Version:  time.Now().UnixNano(),
		At:       time.Now().UTC(),
	}
	if err := s.publisher.Publish(ctx, evt); err != nil {
		s.logger.Warn("publish policy event", zap.Error(err))
	}
}
