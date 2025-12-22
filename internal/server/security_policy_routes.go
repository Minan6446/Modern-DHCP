package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	securitypolicy "modern-dhcp/internal/security/policy"
	"modern-dhcp/pkg/auditpayload"
)

type securityPolicyRuleRequest struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Priority    int                          `json:"priority"`
	Effect      string                       `json:"effect"`
	Enabled     *bool                        `json:"enabled"`
	Matches     []securityPolicyMatchRequest `json:"matches"`
}

type securityPolicyMatchRequest struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Negate bool   `json:"negate"`
}

func (r securityPolicyRuleRequest) toSpec() securitypolicy.RuleSpec {
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	matches := make([]securitypolicy.MatchSpec, 0, len(r.Matches))
	for _, match := range r.Matches {
		matches = append(matches, securitypolicy.MatchSpec{
			Type:   securitypolicy.MatchType(strings.ToLower(strings.TrimSpace(match.Type))),
			Value:  strings.TrimSpace(match.Value),
			Negate: match.Negate,
		})
	}
	return securitypolicy.RuleSpec{
		Name:        strings.TrimSpace(r.Name),
		Description: strings.TrimSpace(r.Description),
		Priority:    r.Priority,
		Effect:      securitypolicy.ParseEffect(r.Effect),
		Enabled:     enabled,
		Matches:     matches,
	}
}

func bindSecurityPolicySpec(c echo.Context) (securitypolicy.RuleSpec, error) {
	var req securityPolicyRuleRequest
	if err := c.Bind(&req); err != nil {
		return securitypolicy.RuleSpec{}, err
	}
	return req.toSpec(), nil
}

func securityPolicyHTTPError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, securitypolicy.ErrRuleNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	msg := strings.ToLower(err.Error())
	if strings.HasPrefix(msg, "security policy:") {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

func (s *HTTPServer) handleSecurityPolicyList(svc *securitypolicy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if svc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "security policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		rules, err := svc.List(c.Request().Context(), tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if rules == nil {
			rules = []securitypolicy.Rule{}
		}
		return c.JSON(http.StatusOK, rules)
	}
}

func (s *HTTPServer) handleSecurityPolicyGet(svc *securitypolicy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if svc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "security policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		ruleID := strings.TrimSpace(c.Param("ruleId"))
		if ruleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
		}
		rule, err := svc.Get(c.Request().Context(), tenantID, ruleID)
		if err != nil {
			return securityPolicyHTTPError(err)
		}
		if rule == nil {
			return echo.NewHTTPError(http.StatusNotFound, "security policy rule not found")
		}
		return c.JSON(http.StatusOK, rule)
	}
}

func (s *HTTPServer) handleSecurityPolicyCreate(svc *securitypolicy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if svc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "security policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		spec, err := bindSecurityPolicySpec(c)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		rule, err := svc.Create(ctx, tenantID, spec)
		if err != nil {
			return securityPolicyHTTPError(err)
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "security_policy_rule",
			Action:     "create",
			Identifier: rule.ID,
			After: map[string]any{
				"name":     rule.Name,
				"priority": rule.Priority,
				"effect":   rule.Effect,
				"enabled":  rule.Enabled,
			},
			Fields: []string{"name", "priority", "effect", "enabled"},
		}
		s.recordAudit(ctx, tenantID, actor, "security.policy.rule.create", change, withResource("security_policy_rule"))
		return c.JSON(http.StatusCreated, rule)
	}
}

func (s *HTTPServer) handleSecurityPolicyUpdate(svc *securitypolicy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if svc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "security policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		ruleID := strings.TrimSpace(c.Param("ruleId"))
		if ruleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
		}
		spec, err := bindSecurityPolicySpec(c)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		rule, err := svc.Update(ctx, tenantID, ruleID, spec)
		if err != nil {
			return securityPolicyHTTPError(err)
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "security_policy_rule",
			Action:     "update",
			Identifier: rule.ID,
			After: map[string]any{
				"name":     rule.Name,
				"priority": rule.Priority,
				"effect":   rule.Effect,
				"enabled":  rule.Enabled,
			},
			Fields: []string{"name", "priority", "effect", "enabled"},
		}
		s.recordAudit(ctx, tenantID, actor, "security.policy.rule.update", change, withResource("security_policy_rule"))
		return c.JSON(http.StatusOK, rule)
	}
}

func (s *HTTPServer) handleSecurityPolicyDelete(svc *securitypolicy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if svc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "security policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		ruleID := strings.TrimSpace(c.Param("ruleId"))
		if ruleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
		}
		ctx := c.Request().Context()
		if err := svc.Delete(ctx, tenantID, ruleID); err != nil {
			return securityPolicyHTTPError(err)
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "security_policy_rule",
			Action:     "delete",
			Identifier: ruleID,
		}
		s.recordAudit(ctx, tenantID, actor, "security.policy.rule.delete", change, withResource("security_policy_rule"))
		return c.NoContent(http.StatusNoContent)
	}
}
