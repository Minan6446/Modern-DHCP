package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service coordinates CRUD operations for security policies.
type Service struct {
	repo      Repository
	evaluator Evaluator
	logger    *zap.Logger
}

// ServiceOption customizes service construction.
type ServiceOption func(*Service)

// WithEvaluator wires a live evaluator for cache invalidations.
func WithEvaluator(eval Evaluator) ServiceOption {
	return func(s *Service) {
		s.evaluator = eval
	}
}

// NewService builds a policy service.
func NewService(repo Repository, logger *zap.Logger, opts ...ServiceOption) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	svc := &Service{repo: repo, logger: logger}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// List returns all rules for a tenant ordered by priority.
func (s *Service) List(ctx context.Context, tenantID string) ([]Rule, error) {
	return s.repo.ListRules(ctx, strings.TrimSpace(tenantID))
}

// Get fetches a specific rule by id.
func (s *Service) Get(ctx context.Context, tenantID, ruleID string) (*Rule, error) {
	return s.repo.GetRule(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(ruleID))
}

// Create inserts a new rule using the provided spec.
func (s *Service) Create(ctx context.Context, tenantID string, spec RuleSpec) (*Rule, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	priority := spec.Priority
	if priority == 0 {
		priority = 100
	}
	rule := Rule{
		ID:          uuid.NewString(),
		TenantID:    strings.TrimSpace(tenantID),
		Name:        strings.TrimSpace(spec.Name),
		Description: strings.TrimSpace(spec.Description),
		Priority:    priority,
		Effect:      ParseEffect(string(spec.Effect)),
		Enabled:     spec.Enabled,
		Matches:     buildMatches(spec.Matches),
	}
	if err := s.repo.CreateRule(ctx, &rule); err != nil {
		return nil, err
	}
	s.invalidate(rule.TenantID)
	return &rule, nil
}

// Update mutates an existing rule.
func (s *Service) Update(ctx context.Context, tenantID, ruleID string, spec RuleSpec) (*Rule, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	priority := spec.Priority
	if priority == 0 {
		priority = 100
	}
	rule := Rule{
		ID:          strings.TrimSpace(ruleID),
		TenantID:    strings.TrimSpace(tenantID),
		Name:        strings.TrimSpace(spec.Name),
		Description: strings.TrimSpace(spec.Description),
		Priority:    priority,
		Effect:      ParseEffect(string(spec.Effect)),
		Enabled:     spec.Enabled,
		Matches:     buildMatches(spec.Matches),
	}
	if err := s.repo.UpdateRule(ctx, &rule); err != nil {
		return nil, err
	}
	s.invalidate(rule.TenantID)
	return &rule, nil
}

// Delete removes the rule and any matches.
func (s *Service) Delete(ctx context.Context, tenantID, ruleID string) error {
	if err := s.repo.DeleteRule(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(ruleID)); err != nil {
		return err
	}
	s.invalidate(strings.TrimSpace(tenantID))
	return nil
}

func (s *Service) invalidate(tenantID string) {
	if s.evaluator == nil || tenantID == "" {
		return
	}
	s.evaluator.Invalidate(tenantID)
}

func validateSpec(spec RuleSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("security policy: name required")
	}
	if len(spec.Matches) == 0 {
		return errors.New("security policy: at least one match required")
	}
	for idx, match := range spec.Matches {
		if !isValidMatchType(match.Type) {
			return fmt.Errorf("security policy: match %d uses unsupported type %s", idx, match.Type)
		}
		if strings.TrimSpace(match.Value) == "" {
			return fmt.Errorf("security policy: match %d value required", idx)
		}
	}
	return nil
}

func isValidMatchType(t MatchType) bool {
	switch t {
	case MatchMAC, MatchIP, MatchCIDR, MatchVLAN, MatchInterface, MatchPort:
		return true
	default:
		return false
	}
}

func buildMatches(specs []MatchSpec) []Match {
	matches := make([]Match, 0, len(specs))
	for _, spec := range specs {
		matches = append(matches, Match{
			Type:   spec.Type,
			Value:  strings.TrimSpace(spec.Value),
			Negate: spec.Negate,
		})
	}
	return matches
}
