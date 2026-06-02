package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"modern-dhcp/pkg/auditpayload"
)

func (s *HTTPServer) accessTenantID(c echo.Context) string {
	scopeRef := s.poolScopeRef(c)
	tenantID := scopeRef.TenantOrDefault()
	if tenantID == "" {
		tenantID = systemTenantID
	}
	return tenantID
}

func (s *HTTPServer) handleAccessTrustPortsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		items := s.accessSecurityStore.listTrustPorts(s.accessTenantID(c))
		return respondSuccess(c, StatusOK, "", items)
	}
}

func (s *HTTPServer) handleAccessTrustPortsSave() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		var payload []accessTrustPort
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		s.accessSecurityStore.replaceTrustPorts(s.accessTenantID(c), payload)
		return respondSuccess(c, StatusOK, "信任端口已保存", nil)
	}
}

func (s *HTTPServer) handleAccessViolationPoliciesList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		items := s.accessSecurityStore.listViolationPolicies(s.accessTenantID(c))
		return respondSuccess(c, StatusOK, "", items)
	}
}

func (s *HTTPServer) handleAccessViolationPoliciesUpsert() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		tenantID := s.accessTenantID(c)
		var payload accessViolationPolicy
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		action := "create"
		for _, item := range s.accessSecurityStore.listViolationPolicies(tenantID) {
			if strings.EqualFold(strings.TrimSpace(item.ID), strings.TrimSpace(payload.ID)) && strings.TrimSpace(payload.ID) != "" {
				action = "update"
				break
			}
		}
		payload.Action = strings.TrimSpace(strings.ToLower(payload.Action))
		saved := s.accessSecurityStore.upsertViolationPolicy(tenantID, payload)
		ctx := c.Request().Context()
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "security_policy_rule",
			Action:     action,
			Identifier: saved.ID,
			After: map[string]any{
				"name":                 saved.Name,
				"action":               saved.Action,
				"blockDurationSeconds": saved.BlockDurationSeconds,
				"alertChannels":        saved.AlertChannels,
			},
			Fields: []string{"name", "action", "blockDurationSeconds", "alertChannels"},
		}
		s.recordAudit(ctx, tenantID, actor, "security.policy.rule."+action, change, withResource("security_policy_rule"))
		return respondSuccess(c, StatusOK, "违规策略已保存", saved)
	}
}

func (s *HTTPServer) handleAccessViolationPoliciesDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		tenantID := s.accessTenantID(c)
		policyID := strings.TrimSpace(c.Param("policyId"))
		if policyID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "policyId is required")
		}
		if !s.accessSecurityStore.deleteViolationPolicy(tenantID, policyID) {
			return respondNotFound(c, "违规策略不存在")
		}
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), "security.policy.rule.delete", auditpayload.ConfigChange{
			Resource:   "security_policy_rule",
			Action:     "delete",
			Identifier: policyID,
		}, withResource("security_policy_rule"))
		return respondSuccess(c, StatusOK, "违规策略已删除", map[string]string{"id": policyID})
	}
}

func (s *HTTPServer) handleAccessRateLimitsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		page, pageSize, err := parsePageParams(c)
		if err != nil {
			return err
		}
		all := s.accessSecurityStore.listRateLimits(s.accessTenantID(c))
		total := len(all)
		offset := (page - 1) * pageSize
		if offset >= total {
			return respondPage(c, StatusOK, []accessRateLimitRule{}, int64(total), int64(page), int64(pageSize))
		}
		end := offset + pageSize
		if end > total {
			end = total
		}
		return respondPage(c, StatusOK, all[offset:end], int64(total), int64(page), int64(pageSize))
	}
}

func (s *HTTPServer) handleAccessRateLimitsUpsert() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		var payload accessRateLimitRule
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		saved := s.accessSecurityStore.upsertRateLimit(s.accessTenantID(c), payload)
		return respondSuccess(c, StatusOK, "限速规则已保存", saved)
	}
}

func (s *HTTPServer) handleAccessRateLimitsDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		ruleID := strings.TrimSpace(c.Param("ruleId"))
		if ruleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
		}
		if !s.accessSecurityStore.deleteRateLimit(s.accessTenantID(c), ruleID) {
			return respondNotFound(c, "限速规则不存在")
		}
		return respondSuccess(c, StatusOK, "限速规则已删除", map[string]string{"id": ruleID})
	}
}

func (s *HTTPServer) handleAccessRogueServersList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		items := s.accessSecurityStore.listRogueServers(s.accessTenantID(c))
		return respondSuccess(c, StatusOK, "", items)
	}
}

func (s *HTTPServer) handleAccessRogueServersCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		var payload accessRogueServer
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		created := s.accessSecurityStore.createRogueServer(s.accessTenantID(c), payload)
		return respondSuccess(c, StatusOK, "非法服务器记录已创建", created)
	}
}

func (s *HTTPServer) handleAccessRogueServersUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		serverID := strings.TrimSpace(c.Param("serverId"))
		if serverID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "serverId is required")
		}
		var payload accessRogueServer
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		updated, ok := s.accessSecurityStore.updateRogueServer(s.accessTenantID(c), serverID, payload)
		if !ok {
			return respondNotFound(c, "非法服务器记录不存在")
		}
		return respondSuccess(c, StatusOK, "非法服务器记录已更新", updated)
	}
}

func (s *HTTPServer) handleAccessRogueServersDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		serverID := strings.TrimSpace(c.Param("serverId"))
		if serverID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "serverId is required")
		}
		if !s.accessSecurityStore.deleteRogueServer(s.accessTenantID(c), serverID) {
			return respondNotFound(c, "非法服务器记录不存在")
		}
		return respondSuccess(c, StatusOK, "非法服务器记录已删除", map[string]string{"id": serverID})
	}
}

func (s *HTTPServer) handleAccessRogueServersQuarantine() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		serverID := strings.TrimSpace(c.Param("serverId"))
		if serverID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "serverId is required")
		}
		updated, ok := s.accessSecurityStore.markRogueAction(s.accessTenantID(c), serverID, "quarantine")
		if !ok {
			return respondNotFound(c, "非法服务器记录不存在")
		}
		return respondSuccess(c, StatusOK, "已执行隔离动作", updated)
	}
}

func (s *HTTPServer) handleAccessRogueServersBlock() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		serverID := strings.TrimSpace(c.Param("serverId"))
		if serverID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "serverId is required")
		}
		updated, ok := s.accessSecurityStore.markRogueAction(s.accessTenantID(c), serverID, "block")
		if !ok {
			return respondNotFound(c, "非法服务器记录不存在")
		}
		return respondSuccess(c, StatusOK, "已执行阻断动作", updated)
	}
}

func (s *HTTPServer) handleAccessThreatRulesGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		item := s.accessSecurityStore.getThreatRules(s.accessTenantID(c))
		return respondSuccess(c, StatusOK, "", item)
	}
}

func (s *HTTPServer) handleAccessThreatRulesSave() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.accessSecurityStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "access security store unavailable")
		}
		var payload accessThreatRuleConfig
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		saved := s.accessSecurityStore.saveThreatRules(s.accessTenantID(c), payload)
		return respondSuccess(c, StatusOK, "威胁规则已保存", saved)
	}
}
