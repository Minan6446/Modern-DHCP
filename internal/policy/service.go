package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/events"
	"modern-dhcp/pkg/models"
)

var (
	ErrPriorityRange      = errors.New("priority must be >= 0")
	ErrDraftNotFound      = errors.New("policy: draft not found")
	ErrDraftAlreadyClosed = errors.New("policy: draft already published")
	ErrDraftEmptyRules    = errors.New("policy: draft must contain at least one rule")
	ErrVersionNotFound    = errors.New("policy: version not found")
	ErrNoMatchingRule     = errors.New("policy: no matching rule")
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

// DraftRule represents a rule stored within a draft payload.
type DraftRule struct {
	ID         string          `json:"id"`
	Priority   int             `json:"priority"`
	Conditions json.RawMessage `json:"conditions"`
	Actions    json.RawMessage `json:"actions"`
	Enabled    *bool           `json:"enabled,omitempty"`
}

// CreateDraftRequest captures inputs for a new policy draft.
type CreateDraftRequest struct {
	TenantID    string
	Name        string
	Description string
	Rules       []DraftRule
	Metadata    json.RawMessage
	Actor       string
}

// UpdateDraftRequest mutates an existing draft.
type UpdateDraftRequest struct {
	TenantID    string
	DraftID     string
	Name        *string
	Description *string
	Rules       *[]DraftRule
	Metadata    *json.RawMessage
	Status      *string
	Actor       string
}

// PublishDraftRequest promotes a draft to an active version.
type PublishDraftRequest struct {
	TenantID   string
	DraftID    string
	Actor      string
	Changelog  string
	Metadata   json.RawMessage
	RollbackOf string
}

// EvaluateDraftRequest evaluates draft rules without persisting them.
type EvaluateDraftRequest struct {
	TenantID string
	DraftID  string
	Rules    []DraftRule
	Input    Input
}

// RollbackVersionRequest describes a rollback operation to a prior version.
type RollbackVersionRequest struct {
	TenantID      string
	VersionNumber int
	Actor         string
	Name          string
	Description   string
	Changelog     string
	Metadata      json.RawMessage
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

func (s *Service) ListDrafts(ctx context.Context, tenantID string, limit int) ([]models.PolicyDraft, error) {
	return s.repo.ListDrafts(ctx, tenantID, limit)
}

func (s *Service) GetDraft(ctx context.Context, tenantID, draftID string) (*models.PolicyDraft, error) {
	draft, err := s.repo.GetDraft(ctx, tenantID, draftID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDraftNotFound
		}
		return nil, err
	}
	return draft, nil
}

func (s *Service) CreateDraft(ctx context.Context, req CreateDraftRequest) (*models.PolicyDraft, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("policy: draft name required")
	}
	if req.TenantID == "" {
		return nil, errors.New("policy: tenant id required")
	}
	if req.Actor == "" {
		return nil, errors.New("policy: actor required")
	}
	normalized, _, err := normalizeDraftRules(req.TenantID, req.Rules)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, ErrDraftEmptyRules
	}
	rulesPayload, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	draft := &models.PolicyDraft{
		ID:          uuid.NewString(),
		TenantID:    req.TenantID,
		Name:        name,
		Description: toNullString(req.Description),
		Status:      "draft",
		Rules:       rulesPayload,
		Metadata:    cloneJSON(req.Metadata),
		CreatedBy:   req.Actor,
		UpdatedBy:   req.Actor,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.InsertDraft(ctx, draft); err != nil {
		return nil, err
	}
	return draft, nil
}

func (s *Service) UpdateDraft(ctx context.Context, req UpdateDraftRequest) (*models.PolicyDraft, error) {
	draft, err := s.repo.GetDraft(ctx, req.TenantID, req.DraftID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDraftNotFound
		}
		return nil, err
	}
	if req.Actor == "" {
		return nil, errors.New("policy: actor required")
	}
	now := time.Now().UTC()
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return nil, errors.New("policy: draft name required")
		}
		draft.Name = trimmed
	}
	if req.Description != nil {
		draft.Description = toNullString(*req.Description)
	}
	if req.Metadata != nil {
		draft.Metadata = cloneJSON(*req.Metadata)
	}
	if req.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		if status == "draft" || status == "archived" || status == "published" {
			draft.Status = status
		} else {
			return nil, fmt.Errorf("policy: unsupported draft status %q", status)
		}
	}
	if req.Rules != nil {
		normalized, _, err := normalizeDraftRules(req.TenantID, *req.Rules)
		if err != nil {
			return nil, err
		}
		if len(normalized) == 0 {
			return nil, ErrDraftEmptyRules
		}
		payload, marshalErr := json.Marshal(normalized)
		if marshalErr != nil {
			return nil, marshalErr
		}
		draft.Rules = payload
	}
	draft.UpdatedBy = req.Actor
	draft.UpdatedAt = now
	if err := s.repo.UpdateDraft(ctx, draft); err != nil {
		return nil, err
	}
	return draft, nil
}

func (s *Service) DeleteDraft(ctx context.Context, tenantID, draftID string) error {
	if err := s.repo.DeleteDraft(ctx, tenantID, draftID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDraftNotFound
		}
		return err
	}
	return nil
}

func (s *Service) PublishDraft(ctx context.Context, req PublishDraftRequest) (*models.PolicyVersion, error) {
	draft, err := s.repo.GetDraft(ctx, req.TenantID, req.DraftID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDraftNotFound
		}
		return nil, err
	}
	if strings.EqualFold(draft.Status, "published") {
		return nil, ErrDraftAlreadyClosed
	}
	if req.Actor == "" {
		return nil, errors.New("policy: actor required")
	}
	rules, err := decodeDraftRules(draft.Rules)
	if err != nil {
		return nil, err
	}
	normalized, policyRules, err := normalizeDraftRules(req.TenantID, rules)
	if err != nil {
		return nil, err
	}
	if len(policyRules) == 0 {
		return nil, ErrDraftEmptyRules
	}
	now := time.Now().UTC()
	for i := range policyRules {
		policyRules[i].TenantID = req.TenantID
		policyRules[i].CreatedAt = now
		policyRules[i].UpdatedAt = now
	}
	if err := s.repo.ReplaceRules(ctx, req.TenantID, policyRules); err != nil {
		return nil, err
	}
	s.invalidate(req.TenantID)

	versionNumber, err := s.repo.LatestVersionNumber(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}
	versionNumber++

	metadata := cloneJSON(draft.Metadata)
	if len(req.Metadata) > 0 {
		metadata = cloneJSON(req.Metadata)
	}
	rulesPayload, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	version := &models.PolicyVersion{
		ID:          uuid.NewString(),
		TenantID:    req.TenantID,
		Version:     versionNumber,
		DerivedFrom: sql.NullString{String: draft.ID, Valid: true},
		Changelog:   toNullString(req.Changelog),
		Rules:       rulesPayload,
		Metadata:    metadata,
		PublishedBy: req.Actor,
		PublishedAt: now,
		RollbackOf:  toNullString(req.RollbackOf),
		CreatedAt:   now,
	}
	if err := s.repo.InsertVersion(ctx, version); err != nil {
		return nil, err
	}

	draft.Status = "published"
	draft.UpdatedAt = now
	draft.UpdatedBy = req.Actor
	draft.PublishedAt = sql.NullTime{Time: now, Valid: true}
	if len(req.Metadata) > 0 {
		draft.Metadata = cloneJSON(req.Metadata)
	}
	draft.Rules = rulesPayload
	if err := s.repo.UpdateDraft(ctx, draft); err != nil {
		return nil, err
	}
	return version, nil
}

func (s *Service) ListVersions(ctx context.Context, tenantID string, limit int) ([]models.PolicyVersion, error) {
	return s.repo.ListVersions(ctx, tenantID, limit)
}

func (s *Service) GetVersion(ctx context.Context, tenantID string, versionNumber int) (*models.PolicyVersion, error) {
	version, err := s.repo.GetVersion(ctx, tenantID, versionNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	return version, nil
}

func (s *Service) RollbackVersion(ctx context.Context, req RollbackVersionRequest) (*models.PolicyVersion, error) {
	if req.TenantID == "" {
		return nil, errors.New("policy: tenant id required")
	}
	if req.VersionNumber <= 0 {
		return nil, errors.New("policy: version number required")
	}
	if strings.TrimSpace(req.Actor) == "" {
		return nil, errors.New("policy: actor required")
	}
	version, err := s.repo.GetVersion(ctx, req.TenantID, req.VersionNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	rules, err := decodeDraftRules(version.Rules)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, ErrDraftEmptyRules
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("Rollback of version %d", version.Version)
	}
	description := req.Description
	metadata := version.Metadata
	if len(req.Metadata) > 0 {
		metadata = cloneJSON(req.Metadata)
	}
	draft, err := s.CreateDraft(ctx, CreateDraftRequest{
		TenantID:    req.TenantID,
		Name:        name,
		Description: description,
		Rules:       rules,
		Metadata:    metadata,
		Actor:       req.Actor,
	})
	if err != nil {
		return nil, err
	}
	changelog := strings.TrimSpace(req.Changelog)
	if changelog == "" {
		changelog = fmt.Sprintf("Rollback to version %d", version.Version)
	}
	rollbackVersion, err := s.PublishDraft(ctx, PublishDraftRequest{
		TenantID:   req.TenantID,
		DraftID:    draft.ID,
		Actor:      req.Actor,
		Changelog:  changelog,
		Metadata:   req.Metadata,
		RollbackOf: version.ID,
	})
	if err != nil {
		return nil, err
	}
	return rollbackVersion, nil
}

func (s *Service) EvaluateDraft(ctx context.Context, req EvaluateDraftRequest) (*Decision, error) {
	if s.engine == nil {
		return nil, errors.New("policy: engine unavailable")
	}
	if req.TenantID == "" {
		return nil, errors.New("policy: tenant id required")
	}
	if req.Input.TenantID == "" {
		req.Input.TenantID = req.TenantID
	}
	_, policyRules, err := normalizeDraftRules(req.TenantID, req.Rules)
	if err != nil {
		return nil, err
	}
	if len(policyRules) == 0 {
		return nil, ErrDraftEmptyRules
	}
	decision, err := s.evaluateRules(req.Input, policyRules)
	if err != nil {
		return nil, err
	}
	return decision, nil
}

func (s *Service) evaluateRules(input Input, rules []models.PolicyRule) (*Decision, error) {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		match, actions := s.engine.matchRule(rule, input)
		if !match {
			continue
		}
		var decision Decision
		if err := json.Unmarshal(actions, &decision); err != nil {
			return nil, err
		}
		return &decision, nil
	}
	return nil, ErrNoMatchingRule
}

func (s *Service) Evaluate(ctx context.Context, input Input) (*Decision, error) {
	if s.engine == nil {
		return nil, errors.New("policy: engine unavailable")
	}
	return s.engine.Evaluate(ctx, input)
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

func normalizeDraftRules(tenantID string, rules []DraftRule) ([]DraftRule, []models.PolicyRule, error) {
	normalized := make([]DraftRule, 0, len(rules))
	modeled := make([]models.PolicyRule, 0, len(rules))
	seenIDs := make(map[string]struct{})
	for idx, raw := range rules {
		rule := raw
		if rule.Priority < 0 {
			return nil, nil, ErrPriorityRange
		}
		if len(rule.Conditions) == 0 {
			rule.Conditions = json.RawMessage([]byte("{}"))
		}
		if len(rule.Actions) == 0 {
			return nil, nil, fmt.Errorf("policy: rule %d missing actions", idx)
		}
		if err := ValidateConditions(rule.Conditions); err != nil {
			return nil, nil, fmt.Errorf("policy: rule %d invalid conditions: %w", idx, err)
		}
		if err := ValidateActions(rule.Actions); err != nil {
			return nil, nil, fmt.Errorf("policy: rule %d invalid actions: %w", idx, err)
		}
		ruleID := strings.TrimSpace(rule.ID)
		if ruleID == "" {
			ruleID = uuid.NewString()
		}
		if _, exists := seenIDs[ruleID]; exists {
			return nil, nil, fmt.Errorf("policy: duplicate rule id %q", ruleID)
		}
		seenIDs[ruleID] = struct{}{}
		rule.ID = ruleID
		rule.Conditions = cloneJSON(rule.Conditions)
		rule.Actions = cloneJSON(rule.Actions)
		enabled := true
		if raw.Enabled != nil {
			enabled = *raw.Enabled
		}
		ruleEnabled := enabled
		rule.Enabled = &ruleEnabled
		normalized = append(normalized, rule)
		modeled = append(modeled, models.PolicyRule{
			ID:         ruleID,
			TenantID:   tenantID,
			Priority:   rule.Priority,
			Conditions: cloneJSON(rule.Conditions),
			Actions:    cloneJSON(rule.Actions),
			Enabled:    enabled,
		})
	}
	return normalized, modeled, nil
}

func decodeDraftRules(raw json.RawMessage) ([]DraftRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rules []DraftRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

func toNullString(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}
