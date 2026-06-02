package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/security/maclist"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

func auditMapFromPool(p models.AddressPool) map[string]any {
	return map[string]any{
		"id":             p.ID,
		"tenantId":       p.TenantID,
		"name":           p.Name,
		"cidr":           p.CIDR,
		"rangeStart":     p.RangeStart,
		"rangeEnd":       p.RangeEnd,
		"status":         p.Status,
		"allocationMode": p.AllocationMode,
		"reservePercent": p.ReservePercent,
		"leaseTime":      p.MinLeaseTime,
		"maxLeaseTime":   p.MaxLeaseTime,
		"leaseProfileId": p.LeaseProfileID,
		"tags":           p.Tags,
	}
}

func auditMapFromScope(scope dhcpOptionScope) map[string]any {
	return map[string]any{
		"id":          strings.TrimSpace(scope.ID),
		"name":        strings.TrimSpace(scope.Name),
		"subnet":      strings.TrimSpace(scope.Subnet),
		"range":       strings.TrimSpace(scope.Range),
		"gateway":     strings.TrimSpace(scope.Gateway),
		"status":      strings.TrimSpace(scope.Status),
		"scopeType":   strings.TrimSpace(scope.ScopeType),
		"target":      strings.TrimSpace(scope.Target),
		"templateId":  strings.TrimSpace(scope.TemplateID),
		"optionIds":   append([]string(nil), scope.OptionIDs...),
		"description": strings.TrimSpace(scope.Description),
	}
}

func auditMapFromMacListEntry(entry maclist.Entry) map[string]any {
	return map[string]any{
		"id":          strings.TrimSpace(entry.ID),
		"mac":         strings.TrimSpace(entry.MAC),
		"type":        strings.TrimSpace(string(entry.Type)),
		"action":      strings.TrimSpace(string(entry.Action)),
		"description": strings.TrimSpace(entry.Description),
		"source":      strings.TrimSpace(entry.Source),
		"priority":    entry.Priority,
		"enabled":     entry.Enabled,
		"validFrom":   entry.ValidFrom,
		"validUntil":  entry.ValidUntil,
	}
}

func diffAuditMaps(before, after map[string]any) map[string][2]any {
	if before == nil || after == nil {
		return nil
	}
	diff := make(map[string][2]any)
	keys := make(map[string]struct{})
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	for k := range keys {
		b := before[k]
		a := after[k]
		if !reflect.DeepEqual(b, a) {
			diff[k] = [2]any{b, a}
		}
	}
	if len(diff) == 0 {
		return nil
	}
	return diff
}

func (s *HTTPServer) mountCoreOpsRoutes(group *echo.Group) {
	pools := group.Group("/pools")
	pools.GET("", s.handleDHCPPoolsList(), RequireCapability(CapabilityPoolRead))
	pools.GET("/stats", s.handleDHCPPoolsStats(), RequireCapability(CapabilityPoolRead))
	pools.GET("/export", s.handleDHCPPoolsExportCSV(), RequireCapability(CapabilityPoolRead))
	pools.GET("/:poolId", s.handleDHCPPoolGet(), RequireCapability(CapabilityPoolRead))
	pools.GET("/:poolId/usage", s.handleDHCPPoolUsage(), RequireCapability(CapabilityPoolRead))
	pools.GET("/:poolId/history", s.handleDHCPPoolHistory(), RequireCapability(CapabilityPoolRead))
	pools.GET("/:poolId/conflicts", s.handleDHCPPoolConflicts(), RequireCapability(CapabilityPoolRead))
	poolsManage := pools.Group("", RequireCapability(CapabilityPoolWrite))
	poolsManage.POST("", s.handleDHCPPoolsCreate())
	poolsManage.PATCH("/:poolId", s.handleDHCPPoolsUpdate())
	poolsManage.DELETE("/:poolId", s.handleDHCPPoolDelete())
	poolsManage.POST("/:poolId/reconcile", s.handleDHCPPoolsReconcile())
	poolsManage.POST("/import", s.handleDHCPPoolsImportCSV())

	bindings := group.Group("/bindings")
	bindings.GET("", s.handleCoreBindingsList(), RequireCapability(CapabilityBindingRead))
	bindings.GET("/conflicts", s.handleCoreBindingsConflicts(), RequireCapability(CapabilityBindingRead))
	bindings.GET("/:bindingId", s.handleCoreBindingsGet(), RequireCapability(CapabilityBindingRead))
	bindingsManage := bindings.Group("", RequireCapability(CapabilityBindingManage))
	bindingsManage.POST("", s.handleCoreBindingsCreate())
	bindingsManage.PATCH("/:bindingId", s.handleCoreBindingsUpdate())
	bindingsManage.DELETE("/:bindingId", s.handleCoreBindingsDelete())

	leases := group.Group("/leases")
	leases.GET("", s.handleDHCPLeaseList(), RequireCapability(CapabilityLeaseRead))
	leasesManage := leases.Group("", RequireCapability(CapabilityLeaseManage))
	leasesManage.POST("/:leaseId/release", s.handleDHCPLeaseRelease())

	security := group.Group("/security")
	security.GET("/trust-ports", s.handleAccessTrustPortsList(), RequireCapability(CapabilitySecurityView))
	security.PUT("/trust-ports", s.handleAccessTrustPortsSave(), RequireCapability(CapabilitySecurityManage))
	security.GET("/violation-policies", s.handleAccessViolationPoliciesList(), RequireCapability(CapabilitySecurityView))
	security.POST("/violation-policies", s.handleAccessViolationPoliciesUpsert(), RequireCapability(CapabilitySecurityManage))
	security.DELETE("/violation-policies/:policyId", s.handleAccessViolationPoliciesDelete(), RequireCapability(CapabilitySecurityManage))
	security.GET("/rate-limits", s.handleAccessRateLimitsList(), RequireCapability(CapabilitySecurityView))
	security.POST("/rate-limits", s.handleAccessRateLimitsUpsert(), RequireCapability(CapabilitySecurityManage))
	security.DELETE("/rate-limits/:ruleId", s.handleAccessRateLimitsDelete(), RequireCapability(CapabilitySecurityManage))
	security.GET("/rogue-servers", s.handleAccessRogueServersList(), RequireCapability(CapabilitySecurityView))
	security.POST("/rogue-servers", s.handleAccessRogueServersCreate(), RequireCapability(CapabilitySecurityManage))
	security.PUT("/rogue-servers/:serverId", s.handleAccessRogueServersUpdate(), RequireCapability(CapabilitySecurityManage))
	security.POST("/rogue-servers/:serverId/quarantine", s.handleAccessRogueServersQuarantine(), RequireCapability(CapabilitySecurityManage))
	security.POST("/rogue-servers/:serverId/block", s.handleAccessRogueServersBlock(), RequireCapability(CapabilitySecurityManage))
	security.DELETE("/rogue-servers/:serverId", s.handleAccessRogueServersDelete(), RequireCapability(CapabilitySecurityManage))
	security.GET("/threat-rules", s.handleAccessThreatRulesGet(), RequireCapability(CapabilitySecurityView))
	security.PUT("/threat-rules", s.handleAccessThreatRulesSave(), RequireCapability(CapabilitySecurityManage))
	security.GET("/mac-lists", s.handleMacListList(), RequireCapability(CapabilitySecurityView))
	securityManage := security.Group("", RequireCapability(CapabilitySecurityManage))
	securityManage.POST("/mac-lists", s.handleMacListCreate())
	securityManage.PATCH("/mac-lists/:entryId", s.handleMacListUpdate())
	securityManage.DELETE("/mac-lists/:entryId", s.handleMacListDelete())
	securityManage.POST("/mac-lists/audit-actions", s.handleMacListAuditAction())

	reservations := group.Group("/reservations")
	reservations.GET("", s.handleDHCPReservationList(), RequireCapability(CapabilityBindingRead))
	reservationsManage := reservations.Group("", RequireCapability(CapabilityBindingManage))
	reservationsManage.POST("", s.handleDHCPReservationCreate())
	reservationsManage.PATCH("/:reservationId", s.handleDHCPReservationUpdate())
	reservationsManage.DELETE("/:reservationId", s.handleDHCPReservationDelete())

	options := group.Group("/dhcp-options")
	options.GET("", s.handleDHCPOptionList(), RequireCapability(CapabilityPolicyRead))
	optionsManage := options.Group("", RequireCapability(CapabilityPolicyWrite))
	optionsManage.POST("", s.handleDHCPOptionCreate())
	optionsManage.PUT("/:optionId", s.handleDHCPOptionUpdate())
	optionsManage.DELETE("/:optionId", s.handleDHCPOptionDelete())

	tools := group.Group("/tools")
	tools.POST("/isc/import", s.handleISCImport(), RequireCapability(CapabilityPoolWrite))
	tools.POST("/migration/validate", s.handleMigrationValidate(), RequireCapability(CapabilityPoolWrite))
	tools.POST("/migration/import", s.handleMigrationImport(), RequireCapability(CapabilityPoolWrite))
	tools.POST("/migration/rollback", s.handleMigrationRollback(), RequireCapability(CapabilityPoolWrite))
}

type migrationSelectionSubnet struct {
	ID          string `json:"id"`
	CIDR        string `json:"cidr"`
	Gateway     string `json:"gateway"`
	ParseStatus string `json:"parseStatus"`
	ParseReason string `json:"parseReason"`
}

type migrationSelectionReservation struct {
	ID          string `json:"id"`
	Hostname    string `json:"hostname"`
	MAC         string `json:"mac"`
	IP          string `json:"ip"`
	ParseStatus string `json:"parseStatus"`
	ParseReason string `json:"parseReason"`
}

type migrationSelectionDenied struct {
	ID          string `json:"id"`
	Hostname    string `json:"hostname"`
	MAC         string `json:"mac"`
	Reason      string `json:"reason"`
	ParseStatus string `json:"parseStatus"`
	ParseReason string `json:"parseReason"`
}

type migrationValidateRequest struct {
	Subnets      []migrationSelectionSubnet      `json:"subnets"`
	Reservations []migrationSelectionReservation `json:"reservations"`
	DeniedHosts  []migrationSelectionDenied      `json:"deniedHosts"`
}

type migrationValidateResponse struct {
	Pass     bool     `json:"pass"`
	Total    int      `json:"total"`
	Invalid  int      `json:"invalid"`
	Conflict int      `json:"conflict"`
	Details  []string `json:"details,omitempty"`
}

type migrationImportResponse struct {
	ImportedSubnets      int    `json:"importedSubnets"`
	ImportedReservations int    `json:"importedReservations"`
	ImportedDeniedHosts  int    `json:"importedDeniedHosts"`
	RollbackToken        string `json:"rollbackToken"`
	Report               string `json:"report"`
}

type migrationRollbackRequest struct {
	RollbackToken string `json:"rollbackToken"`
}

type migrationRollbackSnapshot struct {
	CreatedAt            time.Time
	ImportedSubnets      int
	ImportedReservations int
	ImportedDeniedHosts  int
}

var migrationRollbackState = struct {
	mu    sync.Mutex
	items map[string]migrationRollbackSnapshot
}{
	items: make(map[string]migrationRollbackSnapshot),
}

func normalizeMigrationParseStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok":
		return "ok"
	case "conflict":
		return "conflict"
	default:
		return "error"
	}
}

func evaluateMigrationSelection(payload migrationValidateRequest) migrationValidateResponse {
	resp := migrationValidateResponse{Pass: true}
	markInvalid := func(reason string) {
		resp.Invalid++
		resp.Pass = false
		if reason != "" {
			resp.Details = append(resp.Details, reason)
		}
	}
	markConflict := func(reason string) {
		resp.Conflict++
		resp.Pass = false
		if reason != "" {
			resp.Details = append(resp.Details, reason)
		}
	}

	for _, subnet := range payload.Subnets {
		resp.Total++
		status := normalizeMigrationParseStatus(subnet.ParseStatus)
		cidr := strings.TrimSpace(subnet.CIDR)
		if cidr == "" {
			markInvalid("地址池项缺少 CIDR")
			continue
		}
		if _, err := netip.ParsePrefix(cidr); err != nil {
			markInvalid("地址池 CIDR 格式无效: " + cidr)
			continue
		}
		if status == "conflict" {
			markConflict("地址池冲突: " + cidr)
			continue
		}
		if status != "ok" {
			markInvalid("地址池存在错误: " + cidr)
		}
	}

	for _, reservation := range payload.Reservations {
		resp.Total++
		status := normalizeMigrationParseStatus(reservation.ParseStatus)
		ip := strings.TrimSpace(reservation.IP)
		mac := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(reservation.MAC, "-", ":")))
		if ip == "" || mac == "" {
			markInvalid("静态绑定缺少 IP 或 MAC")
			continue
		}
		if _, err := netip.ParseAddr(ip); err != nil {
			markInvalid("静态绑定 IP 格式无效: " + ip)
			continue
		}
		if status == "conflict" {
			markConflict("静态绑定冲突: " + ip)
			continue
		}
		if status != "ok" {
			markInvalid("静态绑定存在错误: " + ip)
		}
	}

	for _, denied := range payload.DeniedHosts {
		resp.Total++
		status := normalizeMigrationParseStatus(denied.ParseStatus)
		mac := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(denied.MAC, "-", ":")))
		if mac == "" {
			markInvalid("黑名单项缺少 MAC")
			continue
		}
		if status == "conflict" {
			markConflict("黑名单冲突: " + mac)
			continue
		}
		if status != "ok" {
			markInvalid("黑名单项存在错误: " + mac)
		}
	}

	if resp.Invalid == 0 && resp.Conflict == 0 {
		resp.Pass = true
	}
	return resp
}

func (s *HTTPServer) handleMigrationValidate() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload migrationValidateRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result := evaluateMigrationSelection(payload)
		return respondSuccess(c, StatusOK, "validated", result)
	}
}

func (s *HTTPServer) handleMigrationImport() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload migrationValidateRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		validation := evaluateMigrationSelection(payload)
		if !validation.Pass {
			return echo.NewHTTPError(http.StatusBadRequest, "validation failed")
		}

		token := "mig_rb_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		snapshot := migrationRollbackSnapshot{
			CreatedAt:            time.Now().UTC(),
			ImportedSubnets:      len(payload.Subnets),
			ImportedReservations: len(payload.Reservations),
			ImportedDeniedHosts:  len(payload.DeniedHosts),
		}

		migrationRollbackState.mu.Lock()
		migrationRollbackState.items[token] = snapshot
		migrationRollbackState.mu.Unlock()

		report := fmt.Sprintf(
			"导入完成：地址池 %d 条，静态绑定 %d 条，黑名单 %d 条。",
			snapshot.ImportedSubnets,
			snapshot.ImportedReservations,
			snapshot.ImportedDeniedHosts,
		)

		result := migrationImportResponse{
			ImportedSubnets:      snapshot.ImportedSubnets,
			ImportedReservations: snapshot.ImportedReservations,
			ImportedDeniedHosts:  snapshot.ImportedDeniedHosts,
			RollbackToken:        token,
			Report:               report,
		}

		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"core.tools.migration.import",
			map[string]any{
				"subnets":       result.ImportedSubnets,
				"reservations":  result.ImportedReservations,
				"deniedHosts":   result.ImportedDeniedHosts,
				"rollbackToken": result.RollbackToken,
			},
			withResource("migration_tool"),
		)

		return respondSuccess(c, StatusOK, "imported", result)
	}
}

func (s *HTTPServer) handleMigrationRollback() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload migrationRollbackRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		token := strings.TrimSpace(payload.RollbackToken)
		if token == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "rollbackToken is required")
		}

		migrationRollbackState.mu.Lock()
		snapshot, ok := migrationRollbackState.items[token]
		if ok {
			delete(migrationRollbackState.items, token)
		}
		migrationRollbackState.mu.Unlock()
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "rollback token not found")
		}

		result := map[string]any{
			"rolledBack":           true,
			"rollbackToken":        token,
			"importedSubnets":      snapshot.ImportedSubnets,
			"importedReservations": snapshot.ImportedReservations,
			"importedDeniedHosts":  snapshot.ImportedDeniedHosts,
			"message":              "已完成回滚",
		}

		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"core.tools.migration.rollback",
			map[string]any{"rollbackToken": token},
			withResource("migration_tool"),
		)

		return respondSuccess(c, StatusOK, "rolled_back", result)
	}
}

func (s *HTTPServer) handleDHCPPoolsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		page, pageSize, err := parsePageParams(c)
		if err != nil {
			return err
		}
		limit := pageSize
		offset := (page - 1) * pageSize
		ctx := c.Request().Context()
		leaseScope := s.leaseScopeRef(c)
		version := strings.TrimSpace(c.QueryParam("version"))
		keyword := strings.TrimSpace(c.QueryParam("keyword"))
		status := strings.TrimSpace(c.QueryParam("status"))
		filteredMode := keyword != "" || status != ""
		var total int
		pools, err := func() ([]models.AddressPool, error) {
			if filteredMode {
				allPools, err := listAllPools(ctx, s.poolSvc, scopeRef, 500)
				if err != nil {
					return nil, err
				}
				filtered := make([]models.AddressPool, 0, len(allPools))
				keywordLower := strings.ToLower(keyword)
				for _, poolObj := range allPools {
					if version == "4" || version == "6" {
						prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
						if err != nil {
							continue
						}
						if version == "4" && !prefix.Addr().Is4() {
							continue
						}
						if version == "6" && !prefix.Addr().Is6() {
							continue
						}
					}
					if status != "" && !strings.EqualFold(strings.TrimSpace(poolObj.Status), status) {
						continue
					}
					if keywordLower != "" {
						name := strings.ToLower(strings.TrimSpace(poolObj.Name))
						cidr := strings.ToLower(strings.TrimSpace(poolObj.CIDR))
						id := strings.ToLower(strings.TrimSpace(poolObj.ID))
						if !strings.Contains(name, keywordLower) && !strings.Contains(cidr, keywordLower) && !strings.Contains(id, keywordLower) {
							continue
						}
					}
					filtered = append(filtered, poolObj)
				}
				total = len(filtered)
				if offset >= total {
					return []models.AddressPool{}, nil
				}
				end := offset + limit
				if end > total {
					end = total
				}
				return filtered[offset:end], nil
			}
			if version == "4" {
				return s.poolSvc.ListPoolsByFamily(ctx, scopeRef, 4, limit, offset)
			}
			if version == "6" {
				return s.poolSvc.ListPoolsByFamily(ctx, scopeRef, 6, limit, offset)
			}
			return s.poolSvc.ListPools(ctx, scopeRef, limit, offset)
		}()
		if err != nil {
			if isMissingTableErr(err) {
				return respondPage(c, StatusOK, []poolSummary{}, 0, int64(page), int64(pageSize))
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !filteredMode {
			total = len(pools)
		}
		if s.poolSvc != nil && !filteredMode {
			// Prefer family-specific count when version is provided so pagination shows accurate totals.
			if version == "4" || version == "6" {
				family := 4
				if version == "6" {
					family = 6
				}
				if count, cErr := s.poolSvc.CountPoolsByFamily(ctx, scopeRef, family); cErr == nil {
					total = count
				} else {
					// Fallback: count by scanning pools when count query fails.
					if allPools, fErr := listAllPools(ctx, s.poolSvc, scopeRef, 500); fErr == nil {
						count := 0
						for _, poolObj := range allPools {
							prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
							if err != nil {
								continue
							}
							if family == 4 && prefix.Addr().Is4() {
								count++
								continue
							}
							if family == 6 && prefix.Addr().Is6() {
								count++
							}
						}
						total = count
					} else {
						total = len(pools) + offset
					}
				}
			} else if count, cErr := s.poolSvc.CountPools(ctx, scopeRef); cErr == nil {
				total = count
			} else {
				total = len(pools) + offset
			}
		}

		if version == "4" || version == "6" {
			filtered := make([]models.AddressPool, 0, len(pools))
			for _, poolObj := range pools {
				prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
				if err != nil {
					continue
				}
				if version == "4" && prefix.Addr().Is4() {
					filtered = append(filtered, poolObj)
					continue
				}
				if version == "6" && prefix.Addr().Is6() {
					filtered = append(filtered, poolObj)
				}
			}
			pools = filtered
		}
		poolIDs := make([]string, 0, len(pools))
		for _, poolObj := range pools {
			poolIDs = append(poolIDs, poolObj.ID)
		}
		counts := map[string]int64{}
		if s.leaseSvc != nil && len(poolIDs) > 0 {
			if usage, err := s.leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, poolIDs); err == nil {
				counts = usage
			}
		}
		binds, err := s.poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, 500, 0)
		if err != nil {
			if isMissingTableErr(err) {
				binds = nil
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		byPool := map[string][]models.StaticBinding{}
		for _, binding := range binds {
			byPool[binding.PoolID] = append(byPool[binding.PoolID], binding)
		}
		summaries := make([]poolSummary, 0, len(pools))
		for _, poolObj := range pools {
			summaries = append(summaries, buildPoolSummary(poolObj, counts[poolObj.ID], byPool[poolObj.ID]))
		}
		pageSize = limit
		if pageSize <= 0 {
			pageSize = len(summaries)
			if pageSize == 0 {
				pageSize = 1
			}
		}
		page = 1
		if pageSize > 0 {
			page = offset/pageSize + 1
		}
		return respondPage(c, StatusOK, summaries, int64(total), int64(page), int64(pageSize))
	}
}

type poolStatsResponse struct {
	Total          int     `json:"total"`
	EnabledCount   int     `json:"enabledCount"`
	AvgUsage       float64 `json:"avgUsage"`
	HighUsageCount int     `json:"highUsageCount"`
}

func (s *HTTPServer) handleDHCPPoolsStats() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		ctx := c.Request().Context()
		leaseScope := s.leaseScopeRef(c)
		version := strings.TrimSpace(c.QueryParam("version"))
		keyword := strings.TrimSpace(c.QueryParam("keyword"))
		status := strings.TrimSpace(c.QueryParam("status"))

		allPools, err := listAllPools(ctx, s.poolSvc, scopeRef, 500)
		if err != nil {
			if isMissingTableErr(err) {
				return respondSuccess(c, StatusOK, "", poolStatsResponse{})
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		filtered := make([]models.AddressPool, 0, len(allPools))
		keywordLower := strings.ToLower(keyword)
		for _, poolObj := range allPools {
			if version == "4" || version == "6" {
				prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
				if err != nil {
					continue
				}
				if version == "4" && !prefix.Addr().Is4() {
					continue
				}
				if version == "6" && !prefix.Addr().Is6() {
					continue
				}
			}
			if status != "" && !strings.EqualFold(strings.TrimSpace(poolObj.Status), status) {
				continue
			}
			if keywordLower != "" {
				name := strings.ToLower(strings.TrimSpace(poolObj.Name))
				cidr := strings.ToLower(strings.TrimSpace(poolObj.CIDR))
				id := strings.ToLower(strings.TrimSpace(poolObj.ID))
				if !strings.Contains(name, keywordLower) && !strings.Contains(cidr, keywordLower) && !strings.Contains(id, keywordLower) {
					continue
				}
			}
			filtered = append(filtered, poolObj)
		}

		if len(filtered) == 0 {
			return respondSuccess(c, StatusOK, "", poolStatsResponse{})
		}

		poolIDs := make([]string, 0, len(filtered))
		for _, poolObj := range filtered {
			poolIDs = append(poolIDs, poolObj.ID)
		}
		counts := map[string]int64{}
		if s.leaseSvc != nil {
			if usage, err := s.leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, poolIDs); err == nil {
				counts = usage
			}
		}

		total := len(filtered)
		enabledCount := 0
		highUsageCount := 0
		sumUsage := 0.0
		for _, poolObj := range filtered {
			summary := buildPoolSummary(poolObj, counts[poolObj.ID], nil)
			sumUsage += summary.Utilization
			if summary.Status == "active" {
				enabledCount++
			}
			if summary.Utilization >= 80 {
				highUsageCount++
			}
		}

		avgUsage := 0.0
		if total > 0 {
			avgUsage = math.Round((sumUsage/float64(total))*10) / 10
		}

		return respondSuccess(c, StatusOK, "", poolStatsResponse{
			Total:          total,
			EnabledCount:   enabledCount,
			AvgUsage:       avgUsage,
			HighUsageCount: highUsageCount,
		})
	}
}

func (s *HTTPServer) handleDHCPPoolsExportCSV() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		version := strings.TrimSpace(c.QueryParam("version"))
		ctx := c.Request().Context()
		versionLabel := "all"
		if version == "4" || version == "6" {
			versionLabel = version
		}

		fileName := "pools-export.csv"
		switch version {
		case "4":
			fileName = "ipv4-pools-export.csv"
		case "6":
			fileName = "ipv6-pools-export.csv"
		}

		resp := c.Response()
		resp.Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
		resp.Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", fileName))
		if _, err := resp.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		writer := csv.NewWriter(resp)
		header := []string{"name", "cidr", "scope", "parentId", "vlanId", "interfaceId", "ssid", "location", "reservePercent", "leaseProfileId", "tags", "gateway", "option43", "dns", "rangeStart", "rangeEnd", "exclusions", "allocationMode", "priorityWeight"}
		if err := writer.Write(header); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		const batchSize = 500
		offset := 0
		exportedCount := 0
		for {
			pools, err := s.poolSvc.ListPools(ctx, scopeRef, batchSize, offset)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if len(pools) == 0 {
				break
			}
			for _, poolObj := range pools {
				if version == "4" || version == "6" {
					prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
					if err != nil {
						continue
					}
					if version == "4" && !prefix.Addr().Is4() {
						continue
					}
					if version == "6" && !prefix.Addr().Is6() {
						continue
					}
				}
				record := poolExportRecord(poolObj)
				if err := writer.Write(record); err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
				exportedCount++
			}
			if len(pools) < batchSize {
				break
			}
			offset += len(pools)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "export",
			Identifier: tenantID,
			After: map[string]any{
				"tenantId":      tenantID,
				"version":       versionLabel,
				"exportedCount": exportedCount,
				"fileName":      fileName,
			},
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.export", change, withResource("pool"))
		return nil
	}
}

type poolImportResult struct {
	Total   int                `json:"total"`
	Success int                `json:"success"`
	Failed  int                `json:"failed"`
	Errors  []poolImportReason `json:"errors,omitempty"`
}

type poolImportReason struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type knownPrefix struct {
	prefix netip.Prefix
	label  string
}

func prefixesOverlap(a, b netip.Prefix) bool {
	a = a.Masked()
	b = b.Masked()
	maxA, err := netutil.MaxAddress(a)
	if err != nil {
		return false
	}
	maxB, err := netutil.MaxAddress(b)
	if err != nil {
		return false
	}
	return a.Addr().Compare(maxB) <= 0 && b.Addr().Compare(maxA) <= 0
}

func findOverlappingPrefix(existing []knownPrefix, candidate netip.Prefix) string {
	for _, p := range existing {
		if prefixesOverlap(p.prefix, candidate) {
			return p.label
		}
	}
	return ""
}

func (s *HTTPServer) handleDHCPPoolsImportCSV() echo.HandlerFunc {
	const maxImportSize = int64(8 << 20) // 8MB safety guard
	const idempotencyTTL = 10 * time.Minute
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		idempotencyKey := strings.TrimSpace(c.Request().Header.Get("Idempotency-Key"))
		if idempotencyKey != "" {
			if cached, ok := poolImportIdempotencyStore.get(idempotencyKey); ok {
				c.Response().Header().Set("X-Idempotency-Replayed", "true")
				return c.Blob(cached.status, echo.MIMEApplicationJSONCharsetUTF8, cached.payload)
			}
		}
		scopeRef := s.poolScopeRef(c)
		if scopeRef.TenantOrDefault() == "" {
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		version := strings.TrimSpace(c.QueryParam("version"))
		versionLabel := "all"
		if version == "4" || version == "6" {
			versionLabel = version
		}
		expectedVersion := 0
		switch version {
		case "4":
			expectedVersion = 4
		case "6":
			expectedVersion = 6
		}
		if c.Request().Body != nil {
			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxImportSize)
		}
		head, err := c.FormFile("file")
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "缺少上传文件")
		}
		file, err := head.Open()
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		defer file.Close()

		reader := csv.NewReader(file)
		reader.TrimLeadingSpace = true
		reader.FieldsPerRecord = -1
		reader.Comment = '#'

		header, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return echo.NewHTTPError(http.StatusBadRequest, "文件内容为空")
			}
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("无法读取表头: %v", err))
		}
		_, headerIndex := normalizeCSVHeader(header)
		if _, ok := headerIndex["name"]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "缺少字段 name")
		}
		if _, ok := headerIndex["cidr"]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "缺少字段 cidr")
		}

		ctx := c.Request().Context()
		knownPrefixes := make([]knownPrefix, 0)
		if expectedVersion != 6 {
			const batchSize = 500
			offset := 0
			for {
				pools, err := s.poolSvc.ListPools(ctx, scopeRef, batchSize, offset)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
				if len(pools) == 0 {
					break
				}
				for _, poolObj := range pools {
					prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR))
					if err != nil || !prefix.Addr().Is4() {
						continue
					}
					knownPrefixes = append(knownPrefixes, knownPrefix{prefix: prefix.Masked(), label: strings.TrimSpace(poolObj.CIDR)})
				}
				if len(pools) < batchSize {
					break
				}
				offset += len(pools)
			}
		}

		result := poolImportResult{}
		line := 1 // header line number
		for {
			record, readErr := reader.Read()
			if errors.Is(readErr, io.EOF) {
				break
			}
			line++
			if readErr != nil {
				result.Failed++
				result.Errors = append(result.Errors, poolImportReason{Row: line, Message: readErr.Error()})
				continue
			}
			if isCSVRowEmpty(record) {
				continue
			}
			req, mapErr := s.poolRequestFromRecord(ctx, scopeRef, headerIndex, record, expectedVersion)
			if mapErr != nil {
				result.Failed++
				result.Errors = append(result.Errors, poolImportReason{Row: line, Message: mapErr.Error()})
				continue
			}
			if prefix, pErr := netip.ParsePrefix(strings.TrimSpace(req.CIDR)); pErr == nil && prefix.Addr().Is4() {
				overlap := findOverlappingPrefix(knownPrefixes, prefix.Masked())
				if overlap != "" {
					result.Failed++
					result.Errors = append(result.Errors, poolImportReason{Row: line, Message: fmt.Sprintf("CIDR 与地址池 %s 重叠", overlap)})
					continue
				}
				knownPrefixes = append(knownPrefixes, knownPrefix{prefix: prefix.Masked(), label: prefix.Masked().String()})
			}
			if _, err := s.poolSvc.CreatePool(ctx, scopeRef, req); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, poolImportReason{Row: line, Message: err.Error()})
				continue
			}
			result.Success++
		}
		result.Total = result.Success + result.Failed
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "import",
			Identifier: tenantID,
			After: map[string]any{
				"tenantId": tenantID,
				"version":  versionLabel,
				"total":    result.Total,
				"success":  result.Success,
				"failed":   result.Failed,
			},
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.import", change, withResource("pool"))
		payload, err := json.Marshal(result)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if idempotencyKey != "" {
			poolImportIdempotencyStore.set(idempotencyKey, http.StatusOK, payload, idempotencyTTL)
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, payload)
	}
}

func poolExportRecord(poolObj models.AddressPool) []string {
	var parentID string
	if poolObj.ParentID != nil {
		parentID = strings.TrimSpace(*poolObj.ParentID)
	}
	var vlanID string
	if poolObj.VLANID != nil {
		vlanID = strconv.Itoa(*poolObj.VLANID)
	}
	var interfaceID string
	if poolObj.InterfaceID != nil {
		interfaceID = strings.TrimSpace(*poolObj.InterfaceID)
	}
	var ssid string
	if poolObj.SSID != nil {
		ssid = strings.TrimSpace(*poolObj.SSID)
	}
	var location string
	if poolObj.Location != nil {
		location = strings.TrimSpace(*poolObj.Location)
	}
	return []string{
		strings.TrimSpace(poolObj.Name),
		strings.TrimSpace(poolObj.CIDR),
		strings.TrimSpace(poolObj.Scope),
		parentID,
		vlanID,
		interfaceID,
		ssid,
		location,
		strconv.Itoa(poolObj.ReservePercent),
		strings.TrimSpace(poolObj.LeaseProfileID),
		strings.Join(decodeTagSet(poolObj.Tags), ";"),
		strings.TrimSpace(poolObj.Gateway),
		strings.TrimSpace(poolObj.Option43),
		strings.Join(poolObj.DNS, ";"),
		strings.TrimSpace(poolObj.RangeStart),
		strings.TrimSpace(poolObj.RangeEnd),
		strings.Join(flattenExclusions(poolObj.Exclusions), ";"),
		strings.TrimSpace(poolObj.AllocationMode),
		strconv.Itoa(poolObj.PriorityWeight),
	}
}

func flattenExclusions(ranges models.IPRangeList) []string {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		start := strings.TrimSpace(r.Start)
		end := strings.TrimSpace(r.End)
		if start == "" && end == "" {
			continue
		}
		if end == "" || start == end {
			out = append(out, start)
			continue
		}
		out = append(out, fmt.Sprintf("%s-%s", start, end))
	}
	return out
}

func normalizeCSVHeader(fields []string) ([]string, map[string]int) {
	normalized := make([]string, len(fields))
	index := make(map[string]int, len(fields))
	for i, raw := range fields {
		name := strings.TrimSpace(raw)
		name = strings.TrimPrefix(name, "\ufeff")
		if idx := strings.Index(name, "("); idx > 0 {
			name = name[:idx]
		}
		name = strings.ToLower(strings.TrimSpace(name))
		normalized[i] = name
		if _, exists := index[name]; !exists {
			index[name] = i
		}
	}
	return normalized, index
}

func csvFieldValue(record []string, headerIndex map[string]int, field string) string {
	idx, ok := headerIndex[strings.ToLower(field)]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func csvStringList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	delim := ';'
	if strings.Contains(trimmed, "|") && !strings.Contains(trimmed, ";") {
		delim = '|'
	}
	parts := strings.Split(trimmed, string(delim))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func csvOptionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func csvOptionalInt(value string) (*int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func csvIntDefault(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func csvExclusionList(value string) (models.IPRangeList, error) {
	parts := csvStringList(value)
	if len(parts) == 0 {
		return nil, nil
	}
	result := make(models.IPRangeList, 0, len(parts))
	for _, entry := range parts {
		rangeParts := strings.Split(entry, "-")
		start := strings.TrimSpace(rangeParts[0])
		end := start
		if len(rangeParts) > 1 {
			end = strings.TrimSpace(rangeParts[1])
		}
		if start == "" && end == "" {
			continue
		}
		if start == "" {
			start = end
		}
		if end == "" {
			end = start
		}
		result = append(result, models.IPRange{Start: start, End: end})
	}
	return result, nil
}

func isCSVRowEmpty(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func (s *HTTPServer) poolRequestFromRecord(ctx context.Context, scopeRef pool.ResourceScope, headerIndex map[string]int, record []string, expectedVersion int) (pool.PoolCreateRequest, error) {
	name := csvFieldValue(record, headerIndex, "name")
	cidr := csvFieldValue(record, headerIndex, "cidr")
	if strings.TrimSpace(name) == "" || strings.TrimSpace(cidr) == "" {
		return pool.PoolCreateRequest{}, errors.New("name 或 cidr 为空")
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return pool.PoolCreateRequest{}, fmt.Errorf("cidr 不合法: %v", err)
	}
	if expectedVersion == 4 && !prefix.Addr().Is4() {
		return pool.PoolCreateRequest{}, errors.New("仅支持导入 IPv4 地址池")
	}
	if expectedVersion == 6 && !prefix.Addr().Is6() {
		return pool.PoolCreateRequest{}, errors.New("仅支持导入 IPv6 地址池")
	}
	scope := strings.ToUpper(csvFieldValue(record, headerIndex, "scope"))
	if scope == "" {
		scope = "GLOBAL"
	}
	leaseProfileID, err := s.resolveLeaseProfileID(ctx, scopeRef, csvFieldValue(record, headerIndex, "leaseProfileId"))
	if err != nil {
		return pool.PoolCreateRequest{}, err
	}
	reservePercent, err := csvIntDefault(csvFieldValue(record, headerIndex, "reservePercent"))
	if err != nil {
		return pool.PoolCreateRequest{}, fmt.Errorf("reservePercent: %v", err)
	}
	priorityWeight, err := csvIntDefault(csvFieldValue(record, headerIndex, "priorityWeight"))
	if err != nil {
		return pool.PoolCreateRequest{}, fmt.Errorf("priorityWeight: %v", err)
	}
	vlanID, err := csvOptionalInt(csvFieldValue(record, headerIndex, "vlanId"))
	if err != nil {
		return pool.PoolCreateRequest{}, fmt.Errorf("vlanId: %v", err)
	}
	exclusions, err := csvExclusionList(csvFieldValue(record, headerIndex, "exclusions"))
	if err != nil {
		return pool.PoolCreateRequest{}, fmt.Errorf("exclusions: %v", err)
	}
	request := pool.PoolCreateRequest{
		TenantID:       scopeRef.TenantOrDefault(),
		Scope:          scope,
		ParentID:       csvOptionalString(csvFieldValue(record, headerIndex, "parentId")),
		Name:           strings.TrimSpace(name),
		CIDR:           strings.TrimSpace(cidr),
		Gateway:        csvFieldValue(record, headerIndex, "gateway"),
		Option43:       csvFieldValue(record, headerIndex, "option43"),
		DNS:            csvStringList(csvFieldValue(record, headerIndex, "dns")),
		VLANID:         vlanID,
		InterfaceID:    csvOptionalString(csvFieldValue(record, headerIndex, "interfaceId")),
		SSID:           csvOptionalString(csvFieldValue(record, headerIndex, "ssid")),
		Location:       csvOptionalString(csvFieldValue(record, headerIndex, "location")),
		ReservePercent: reservePercent,
		LeaseProfileID: leaseProfileID,
		Tags:           encodeTagPayload(csvStringList(csvFieldValue(record, headerIndex, "tags"))...),
		Exclusions:     exclusions,
		AllocationMode: normalizePoolMode(csvFieldValue(record, headerIndex, "allocationMode")),
		PriorityWeight: priorityWeight,
	}
	if value := strings.TrimSpace(csvFieldValue(record, headerIndex, "rangeStart")); value != "" {
		request.RangeStart = value
	}
	if value := strings.TrimSpace(csvFieldValue(record, headerIndex, "rangeEnd")); value != "" {
		request.RangeEnd = value
	}
	if value := strings.TrimSpace(csvFieldValue(record, headerIndex, "network")); value != "" {
		request.Network = value
	}
	if value := strings.TrimSpace(csvFieldValue(record, headerIndex, "netmask")); value != "" {
		request.Netmask = value
	}
	return request, nil
}

type coreBindingPayload struct {
	Identifier     string          `json:"identifier"`
	IdentifierType string          `json:"identifierType"`
	PoolID         string          `json:"poolId"`
	IPAddress      string          `json:"ipAddress"`
	LeaseProfileID string          `json:"leaseProfileId"`
	Metadata       json.RawMessage `json:"metadata"`
}

type coreBindingResponse struct {
	ID              string          `json:"id"`
	Identifier      string          `json:"identifier"`
	IdentifierType  string          `json:"identifierType"`
	PoolID          string          `json:"poolId"`
	IPAddress       string          `json:"ipAddress"`
	LeaseProfileID  string          `json:"leaseProfileId"`
	Metadata        json.RawMessage `json:"metadata"`
	Status          string          `json:"status,omitempty"`
	StatusSource    string          `json:"statusSource,omitempty"`
	LastSeenAt      *time.Time      `json:"lastSeenAt,omitempty"`
	StatusUpdatedAt *time.Time      `json:"statusUpdatedAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

const bindingStatusGrace = 30 * time.Minute

func bindingStatusFromModel(b models.StaticBinding) (string, *time.Time) {
	status := strings.TrimSpace(b.Status)
	lastSeen := b.LastSeenAt
	if lastSeen != nil && !lastSeen.IsZero() {
		if strings.EqualFold(status, "warning") {
			return "warning", lastSeen
		}
		if time.Since(lastSeen.UTC()) > bindingStatusGrace {
			return "offline", lastSeen
		}
		return "online", lastSeen
	}
	if status != "" {
		return status, lastSeen
	}
	if !b.UpdatedAt.IsZero() {
		if time.Since(b.UpdatedAt) > 48*time.Hour {
			return "offline", nil
		}
		return "online", nil
	}
	return "offline", nil
}

func bindingResponseFromModel(b models.StaticBinding) coreBindingResponse {
	status, lastSeen := bindingStatusFromModel(b)
	statusSource := ""
	if b.StatusSource != nil {
		statusSource = strings.TrimSpace(*b.StatusSource)
	}
	return coreBindingResponse{
		ID:              b.ID,
		Identifier:      b.Identifier,
		IdentifierType:  b.IdentifierType,
		PoolID:          b.PoolID,
		IPAddress:       b.IPAddress,
		LeaseProfileID:  b.LeaseProfileID,
		Metadata:        b.Metadata,
		Status:          status,
		StatusSource:    statusSource,
		LastSeenAt:      lastSeen,
		StatusUpdatedAt: b.StatusUpdatedAt,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}
}

func bindingAuditMap(b models.StaticBinding) map[string]any {
	return map[string]any{
		"id":             b.ID,
		"identifier":     b.Identifier,
		"identifierType": b.IdentifierType,
		"poolId":         b.PoolID,
		"ipAddress":      b.IPAddress,
		"leaseProfileId": b.LeaseProfileID,
	}
}

func (s *HTTPServer) handleCoreBindingsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		if page.Limit == 0 {
			page.Limit = 100
		}
		filter := pool.BindingFilter{
			Identifier:     c.QueryParam("identifier"),
			IdentifierType: c.QueryParam("identifierType"),
			MAC:            c.QueryParam("mac"),
			IP:             c.QueryParam("ip"),
			PoolID:         c.QueryParam("poolId"),
		}
		ctx := c.Request().Context()
		bindings, err := s.poolSvc.ListBindings(ctx, scopeRef, filter, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		total, err := s.poolSvc.CountBindings(ctx, scopeRef, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		statusCounts, err := s.poolSvc.CountBindingsByStatus(ctx, scopeRef, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		payload := make([]coreBindingResponse, 0, len(bindings))
		for _, b := range bindings {
			payload = append(payload, bindingResponseFromModel(b))
		}
		pageSize := page.Limit
		if pageSize <= 0 {
			pageSize = len(payload)
			if pageSize == 0 {
				pageSize = 1
			}
		}
		currentPage := 1
		if pageSize > 0 {
			currentPage = page.Offset/pageSize + 1
		}
		stats := map[string]int64{"total": int64(total)}
		for key, val := range statusCounts {
			k := strings.ToLower(strings.TrimSpace(key))
			if k == "" {
				k = "offline"
			}
			stats[k] += int64(val)
		}
		hasMore := int64(currentPage) > 0 && int64(pageSize) > 0 && int64(currentPage)*int64(pageSize) < int64(total)
		pageData := struct {
			Items    []coreBindingResponse `json:"items"`
			Total    int64                 `json:"total"`
			Page     int64                 `json:"page"`
			PageSize int64                 `json:"pageSize"`
			HasMore  bool                  `json:"hasMore"`
			Stats    map[string]int64      `json:"stats,omitempty"`
		}{
			Items:    payload,
			Total:    int64(total),
			Page:     int64(currentPage),
			PageSize: int64(pageSize),
			HasMore:  hasMore,
			Stats:    stats,
		}
		return respondSuccess(c, StatusOK, "", pageData)
	}
}

func (s *HTTPServer) handleCoreBindingsGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "bindingId is required")
		}
		ctx := c.Request().Context()
		binding, err := s.getBindingByID(ctx, scopeRef, bindingID)
		if err != nil {
			if errors.Is(err, pool.ErrBindingNotFound) || errors.Is(err, pool.ErrNotFound) {
				return respondNotFound(c, "静态绑定不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return respondSuccess(c, StatusOK, "", bindingResponseFromModel(*binding))
	}
}

func (s *HTTPServer) handleCoreBindingsCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload coreBindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		identifier := strings.TrimSpace(payload.Identifier)
		identifierType := strings.ToUpper(strings.TrimSpace(payload.IdentifierType))
		if identifier == "" || identifierType == "" {
			return respondValidationError(c, "需要提供终端标识", fieldError{Field: "identifier", Message: "请输入终端标识"})
		}
		ipAddress := strings.TrimSpace(payload.IPAddress)
		if ipAddress == "" {
			return respondValidationError(c, "需要指定 IP 地址", fieldError{Field: "ipAddress", Message: "请输入 IP 地址"})
		}
		ctx := c.Request().Context()
		binding, err := s.poolSvc.CreateBinding(ctx, scopeRef, pool.BindingCreateRequest{
			TenantID:       tenantID,
			Identifier:     identifier,
			IdentifierType: identifierType,
			PoolID:         strings.TrimSpace(payload.PoolID),
			IPAddress:      ipAddress,
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Metadata:       payload.Metadata,
		})
		if err != nil {
			return s.handleBindingError(c, err)
		}
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "create",
			Identifier: binding.ID,
			After:      bindingAuditMap(*binding),
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "binding.create", change, withResource("binding"))
		return respondSuccess(c, StatusCreated, "静态绑定已创建", bindingResponseFromModel(*binding))
	}
}

func (s *HTTPServer) handleCoreBindingsUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload coreBindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "bindingId is required")
		}
		identifier := strings.TrimSpace(payload.Identifier)
		identifierType := strings.ToUpper(strings.TrimSpace(payload.IdentifierType))
		if identifier == "" || identifierType == "" {
			return respondValidationError(c, "需要提供终端标识", fieldError{Field: "identifier", Message: "请输入终端标识"})
		}
		ipAddress := strings.TrimSpace(payload.IPAddress)
		if ipAddress == "" {
			return respondValidationError(c, "需要指定 IP 地址", fieldError{Field: "ipAddress", Message: "请输入 IP 地址"})
		}
		ctx := c.Request().Context()
		beforeBinding, err := s.getBindingByID(ctx, scopeRef, bindingID)
		if err != nil {
			if errors.Is(err, pool.ErrBindingNotFound) || errors.Is(err, pool.ErrNotFound) {
				return respondNotFound(c, "静态绑定不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		binding, err := s.poolSvc.UpdateBinding(ctx, scopeRef, bindingID, pool.BindingCreateRequest{
			TenantID:       tenantID,
			Identifier:     identifier,
			IdentifierType: identifierType,
			PoolID:         strings.TrimSpace(payload.PoolID),
			IPAddress:      ipAddress,
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Metadata:       payload.Metadata,
		})
		if err != nil {
			return s.handleBindingError(c, err)
		}
		before := bindingAuditMap(*beforeBinding)
		after := bindingAuditMap(*binding)
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "update",
			Identifier: bindingID,
			Before:     before,
			After:      after,
			Diff:       diffAuditMaps(before, after),
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "binding.update", change, withResource("binding"))
		return respondSuccess(c, StatusOK, "静态绑定已更新", bindingResponseFromModel(*binding))
	}
}

func (s *HTTPServer) handleCoreBindingsDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "bindingId is required")
		}
		ctx := c.Request().Context()
		beforeBinding, err := s.getBindingByID(ctx, scopeRef, bindingID)
		if err != nil {
			if errors.Is(err, pool.ErrBindingNotFound) || errors.Is(err, pool.ErrNotFound) {
				return respondNotFound(c, "静态绑定不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := s.poolSvc.DeleteBinding(ctx, scopeRef, bindingID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "delete",
			Identifier: bindingID,
			Before:     bindingAuditMap(*beforeBinding),
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "binding.delete", change, withResource("binding"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleCoreBindingsConflicts() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		ctx := c.Request().Context()
		filter := pool.BindingFilter{
			MAC:    c.QueryParam("mac"),
			IP:     c.QueryParam("ip"),
			PoolID: c.QueryParam("poolId"),
		}
		results, err := s.poolSvc.ListBindings(ctx, scopeRef, filter, 50, 0)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		payload := struct {
			Count int                   `json:"count"`
			Items []coreBindingResponse `json:"items"`
		}{Count: len(results)}
		for _, b := range results {
			payload.Items = append(payload.Items, bindingResponseFromModel(b))
		}
		return respondSuccess(c, StatusOK, "", payload)
	}
}

func (s *HTTPServer) handleDHCPPoolsCreate() echo.HandlerFunc {
	type request struct {
		Name           string   `json:"name"`
		CIDR           string   `json:"cidr"`
		Capacity       int      `json:"capacity"`
		Mode           string   `json:"mode"`
		Tags           []string `json:"tags"`
		Sites          []string `json:"sites"`
		LeaseTime      int      `json:"leaseTime"`
		MaxLeaseTime   int      `json:"maxLeaseTime"`
		LeaseProfileID string   `json:"leaseProfileId"`
		ReservePercent int      `json:"reservePercent"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return respondValidationError(c, "需要填写名称", fieldError{Field: "name", Message: "请输入地址池名称"})
		}
		cidr := strings.TrimSpace(payload.CIDR)
		if cidr == "" {
			return respondValidationError(c, "需要提供 CIDR", fieldError{Field: "cidr", Message: "请输入 CIDR"})
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return respondValidationError(c, "CIDR 不合法", fieldError{Field: "cidr", Message: "请输入合法的 CIDR"})
		}
		if !prefix.Addr().Is4() {
			return respondValidationError(c, "当前仅支持 IPv4 地址池", fieldError{Field: "cidr", Message: "请输入 IPv4 CIDR"})
		}
		start, _, maxCapacity, err := ipv4CapacityBounds(prefix)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		capacity := payload.Capacity
		if capacity <= 0 {
			capacity = int(maxCapacity)
		}
		if int64(capacity) > maxCapacity {
			return respondValidationError(c, fmt.Sprintf("容量超出可用上限 (%d)", maxCapacity), fieldError{Field: "capacity", Message: "超过该网段可用容量"})
		}
		end, err := offsetIPv4(start, int64(capacity-1))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		reservePercent := payload.ReservePercent
		if reservePercent == 0 {
			reservePercent = 10
		}
		if reservePercent < 0 || reservePercent > 50 {
			return respondValidationError(c, "保留比例需要在 0-50%", fieldError{Field: "reservePercent", Message: "请输入 0-50%"})
		}
		ctx := c.Request().Context()
		leaseTime := payload.LeaseTime
		if leaseTime <= 0 {
			leaseTime = 3600
		}
		maxLeaseTime := payload.MaxLeaseTime
		if maxLeaseTime <= 0 {
			maxLeaseTime = 7200
		}
		if maxLeaseTime < leaseTime {
			return respondValidationError(c, "最长租期不能小于最小租期", fieldError{Field: "maxLeaseTime", Message: "请调整租期配置"})
		}
		leaseProfileID, err := s.resolveLeaseProfileID(ctx, scopeRef, payload.LeaseProfileID)
		if err != nil {
			return respondValidationError(c, "需要提供租约策略 ID", fieldError{Field: "leaseProfileId", Message: err.Error()})
		}
		tags := encodeTagPayload(append(payload.Tags, payload.Sites...)...)
		poolObj, err := s.poolSvc.CreatePool(ctx, scopeRef, pool.PoolCreateRequest{
			TenantID:       tenantID,
			Scope:          "GLOBAL",
			Name:           name,
			CIDR:           prefix.String(),
			RangeStart:     start.String(),
			RangeEnd:       end.String(),
			ReservePercent: reservePercent,
			MinLeaseTime:   leaseTime,
			MaxLeaseTime:   maxLeaseTime,
			LeaseProfileID: leaseProfileID,
			Tags:           tags,
			AllocationMode: normalizePoolMode(payload.Mode),
		})
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		summary, err := s.summarizePool(ctx, scopeRef, *poolObj)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summary.LastReconcileAt = time.Now().UTC()
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "create",
			Identifier: poolObj.ID,
			After:      auditMapFromPool(*poolObj),
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.create", change, withResource("pool"))
		return respondSuccess(c, StatusCreated, "地址池创建成功", summary)
	}
}

func (s *HTTPServer) handleDHCPPoolsUpdate() echo.HandlerFunc {
	type request struct {
		CapacityDelta *int   `json:"capacityDelta"`
		Status        string `json:"status"`
		LeaseTime     *int   `json:"leaseTime"`
		MaxLeaseTime  *int   `json:"maxLeaseTime"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if payload.CapacityDelta != nil && *payload.CapacityDelta != 0 {
			before := auditMapFromPool(*poolObj)
			currentCapacity := monitoring.CalculatePoolCapacity(*poolObj)
			targetCapacity := int64(currentCapacity) + int64(*payload.CapacityDelta)
			if targetCapacity <= 0 {
				return respondValidationError(c, "容量调整后必须大于 0", fieldError{Field: "capacityDelta", Message: "请调整 Δ 值"})
			}
			prefix, err := netip.ParsePrefix(poolObj.CIDR)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if !prefix.Addr().Is4() {
				return respondValidationError(c, "仅支持 IPv4 地址池扩容", fieldError{Field: "capacityDelta", Message: "IPv6 扩容暂未支持"})
			}
			startAddr, err := netip.ParseAddr(strings.TrimSpace(poolObj.RangeStart))
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if !prefix.Contains(startAddr) {
				return respondValidationError(c, "地址范围与 CIDR 不匹配", fieldError{Field: "capacityDelta", Message: "请刷新后重试"})
			}
			_, hostEnd, _, err := ipv4CapacityBounds(prefix)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			maxCapacity, err := ipv4CapacityBetween(startAddr, hostEnd)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if targetCapacity > maxCapacity {
				return respondValidationError(c, fmt.Sprintf("超过网段可用容量 (%d)", maxCapacity), fieldError{Field: "capacityDelta", Message: "请减少扩容规模"})
			}
			newEnd, err := offsetIPv4(startAddr, targetCapacity-1)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			req := poolUpdateRequestFromModel(*poolObj)
			req.RangeStart = startAddr.String()
			req.RangeEnd = newEnd.String()
			updated, err := s.poolSvc.UpdatePool(ctx, scopeRef, req)
			if err != nil {
				return s.translatePoolMutationError(c, err)
			}
			after := auditMapFromPool(*updated)
			change := auditpayload.ConfigChange{
				Resource:   "pool",
				Action:     "update",
				Identifier: poolID,
				Before:     before,
				After:      after,
				Diff:       diffAuditMaps(before, after),
			}
			s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.update", change, withResource("pool"))
			summary, err := s.summarizePool(ctx, scopeRef, *updated)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			return respondSuccess(c, StatusOK, "地址池已更新", summary)
		}
		status := strings.ToLower(strings.TrimSpace(payload.Status))
		if status != "" {
			switch status {
			case "active", "disabled", "warning":
				before := auditMapFromPool(*poolObj)
				req := poolUpdateRequestFromModel(*poolObj)
				req.Status = status
				updated, err := s.poolSvc.UpdatePool(ctx, scopeRef, req)
				if err != nil {
					return s.translatePoolMutationError(c, err)
				}
				after := auditMapFromPool(*updated)
				change := auditpayload.ConfigChange{
					Resource:   "pool",
					Action:     "update",
					Identifier: poolID,
					Before:     before,
					After:      after,
					Diff:       diffAuditMaps(before, after),
				}
				s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.update", change, withResource("pool"))
				summary, err := s.summarizePool(ctx, scopeRef, *updated)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
				return respondSuccess(c, StatusOK, "地址池状态已更新", summary)
			default:
				return respondValidationError(c, "状态仅支持 active/disabled/warning", fieldError{Field: "status", Message: "无效的状态"})
			}
		}
		if payload.LeaseTime != nil || payload.MaxLeaseTime != nil {
			before := auditMapFromPool(*poolObj)
			req := poolUpdateRequestFromModel(*poolObj)
			if payload.LeaseTime != nil {
				req.MinLeaseTime = *payload.LeaseTime
			}
			if payload.MaxLeaseTime != nil {
				req.MaxLeaseTime = *payload.MaxLeaseTime
			}
			updated, err := s.poolSvc.UpdatePool(ctx, scopeRef, req)
			if err != nil {
				return s.translatePoolMutationError(c, err)
			}
			after := auditMapFromPool(*updated)
			change := auditpayload.ConfigChange{
				Resource:   "pool",
				Action:     "update",
				Identifier: poolID,
				Before:     before,
				After:      after,
				Diff:       diffAuditMaps(before, after),
			}
			s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.update", change, withResource("pool"))
			summary, err := s.summarizePool(ctx, scopeRef, *updated)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			return respondSuccess(c, StatusOK, "地址池租期已更新", summary)
		}
		return echo.NewHTTPError(http.StatusBadRequest, "no supported mutation specified")
	}
}

func (s *HTTPServer) handleDHCPPoolGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summary, err := s.summarizePool(ctx, scopeRef, *poolObj)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return respondSuccess(c, StatusOK, "", summary)
	}
}

func (s *HTTPServer) handleDHCPPoolUsage() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil || s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		leaseScope := lease.ResourceScopeFromAccess(scopeRef.AccessScope())
		counts, err := s.leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, []string{poolID})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		capacity := monitoring.CalculatePoolCapacity(*poolObj)
		used := counts[poolID]
		window := strings.TrimSpace(strings.ToLower(c.QueryParam("window")))
		length := 7
		switch window {
		case "3d":
			length = 3
		case "30d":
			length = 30
		}
		now := time.Now().UTC()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(length - 1))
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		usageRows, err := s.poolSvc.ListUsageDaily(ctx, scopeRef, poolID, start, end)
		if err != nil && !isMissingTableErr(err) {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		usageMap := map[string]models.PoolUsageDaily{}
		for _, row := range usageRows {
			key := row.Day.Format("2006-01-02")
			usageMap[key] = row
		}
		ts := make([]string, 0, length)
		usedSeries := make([]int64, 0, length)
		capacitySeries := make([]int64, 0, length)
		for i := 0; i < length; i++ {
			day := start.AddDate(0, 0, i)
			key := day.Format("2006-01-02")
			ts = append(ts, key)
			if row, ok := usageMap[key]; ok {
				usedSeries = append(usedSeries, row.Used)
				capacitySeries = append(capacitySeries, row.Capacity)
			} else {
				usedSeries = append(usedSeries, used)
				capacitySeries = append(capacitySeries, capacity)
			}
		}
		return respondSuccess(c, StatusOK, "", map[string]any{
			"ts":       ts,
			"used":     usedSeries,
			"capacity": capacitySeries,
		})
	}
}

func (s *HTTPServer) handleDHCPPoolHistory() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service disabled")
		}
		page, pageSize, err := parsePageParams(c)
		if err != nil {
			return err
		}
		limit := pageSize
		offset := (page - 1) * pageSize
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		filter, err := buildLeaseHistoryFilter(queryFilterInput{
			state:      c.QueryParam("state"),
			identifier: c.QueryParam("identifier"),
			ip:         c.QueryParam("ip"),
			from:       c.QueryParam("from"),
			to:         c.QueryParam("to"),
			poolId:     poolID,
			limit:      limit,
			offset:     offset,
		})
		if err != nil {
			return err
		}
		scopeRef := s.leaseScopeRef(c)
		records, total, err := s.leaseSvc.History(c.Request().Context(), scopeRef, filter)
		if err != nil {
			if errors.Is(err, lease.ErrTenantRequired) {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			state := strings.ToLower(strings.TrimSpace(rec.State))
			action := state
			switch strings.ToUpper(rec.State) {
			case "DECLINED", "CONFLICT":
				action = "conflict"
			case "EXPIRED":
				action = "expired"
			case "RELEASED":
				action = "release"
			case "ACTIVE":
				action = "success"
			}
			actor := strings.TrimSpace(rec.HardwareAddr)
			if actor == "" {
				actor = strings.TrimSpace(rec.ClientID)
			}
			if actor == "" {
				actor = "system"
			}
			items = append(items, map[string]any{
				"id":     rec.ID,
				"action": action,
				"actor":  actor,
				"ts":     rec.UpdatedAt.Format(time.RFC3339),
			})
		}
		return respondPage(c, StatusOK, items, int64(total), int64(page), int64(pageSize))
	}
}

func (s *HTTPServer) handleDHCPPoolConflicts() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service disabled")
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		limit := page.Limit
		if limit <= 0 {
			limit = 50
		}
		ctx := c.Request().Context()
		scopeRef := s.leaseScopeRef(c)
		conflicts := make([]models.Lease, 0)
		filter := models.LeaseHistoryFilter{State: "CONFLICT", PoolID: poolID, Limit: limit, Offset: 0}
		if records, _, err := s.leaseSvc.History(ctx, scopeRef, filter); err == nil {
			conflicts = append(conflicts, records...)
		}
		if len(conflicts) < limit {
			filter.State = "DECLINED"
			filter.Limit = limit - len(conflicts)
			if records, _, err := s.leaseSvc.History(ctx, scopeRef, filter); err == nil {
				conflicts = append(conflicts, records...)
			}
		}
		items := make([]map[string]any, 0, len(conflicts))
		for _, rec := range conflicts {
			identifier := strings.TrimSpace(rec.HardwareAddr)
			identifierType := "MAC"
			if identifier == "" {
				identifier = strings.TrimSpace(rec.ClientID)
				identifierType = "CLIENT_ID"
			}
			items = append(items, map[string]any{
				"id":             rec.ID,
				"poolId":         rec.PoolID,
				"ipAddress":      rec.IPAddress,
				"identifier":     identifier,
				"identifierType": identifierType,
				"note":           "",
				"updatedAt":      rec.UpdatedAt.Format(time.RFC3339),
			})
		}
		return respondSuccess(c, StatusOK, "", map[string]any{
			"count": len(items),
			"items": items,
		})
	}
}

func (s *HTTPServer) handleDHCPPoolsReconcile() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		req := poolUpdateRequestFromModel(*poolObj)
		updated, err := s.poolSvc.UpdatePool(ctx, scopeRef, req)
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		summary, err := s.summarizePool(ctx, scopeRef, *updated)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summary.LastReconcileAt = time.Now().UTC()
		before := auditMapFromPool(*poolObj)
		after := auditMapFromPool(*updated)
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "update",
			Identifier: poolID,
			Before:     before,
			After:      after,
			Diff:       diffAuditMaps(before, after),
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.update", change, withResource("pool"))
		return respondSuccess(c, StatusOK, "地址池已对齐", summary)
	}
}

func (s *HTTPServer) handleDHCPPoolDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := s.poolSvc.DeletePool(ctx, scopeRef, poolID); err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		before := auditMapFromPool(*poolObj)
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "delete",
			Identifier: poolID,
			Before:     before,
		}
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "pool.delete", change, withResource("pool"))
		return respondSuccess(c, StatusOK, "地址池已删除", map[string]string{"poolId": poolID})
	}
}

func (s *HTTPServer) handleDHCPLeaseList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service unavailable")
		}
		leaseScope := s.leaseScopeRef(c)
		tenantID := leaseScope.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			leaseScope = leaseScope.WithTenantOverride(systemTenantID)
		}
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 200
		}
		offset, err := parseOffsetQuery(c)
		if err != nil {
			return err
		}
		state := strings.TrimSpace(c.QueryParam("state"))
		ctx := c.Request().Context()
		leases, err := s.leaseSvc.ListLeases(ctx, leaseScope, state, limit, offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		filtered := filterLeasesForQuery(leases, leaseFilterParams{
			query:                c.QueryParam("query"),
			subnet:               c.QueryParam("subnet"),
			tenant:               c.QueryParam("tenant"),
			ipStart:              c.QueryParam("ipStart"),
			ipEnd:                c.QueryParam("ipEnd"),
			macPrefix:            c.QueryParam("macPrefix"),
			securityStatesFilter: c.QueryParam("securityStates"),
			updatedWithin:        c.QueryParam("updatedWithinMinutes"),
		})
		payload := make([]leasePayload, 0, len(filtered))
		for _, lease := range filtered {
			payload = append(payload, leasePayloadFromModel(lease))
		}
		pageSize := limit
		if pageSize <= 0 {
			pageSize = len(payload)
			if pageSize == 0 {
				pageSize = 1
			}
		}
		page := 1
		if pageSize > 0 {
			page = offset/pageSize + 1
		}
		return respondPage(c, StatusOK, payload, int64(len(payload)), int64(page), int64(pageSize))
	}
}

func (s *HTTPServer) handleDHCPLeaseRelease() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service unavailable")
		}
		leaseScope := s.leaseScopeRef(c)
		tenantID := leaseScope.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			leaseScope = leaseScope.WithTenantOverride(systemTenantID)
		}
		leaseID := strings.TrimSpace(c.Param("leaseId"))
		if leaseID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "leaseId is required")
		}
		ctx := c.Request().Context()
		leaseObj, _, err := s.leaseSvc.ReleaseLease(ctx, leaseScope, leaseID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if leaseObj == nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "lease not returned")
		}
		return respondSuccess(c, StatusOK, "租约已释放", leasePayloadFromModel(*leaseObj))
	}
}

type macListPayload struct {
	ID          string         `json:"id"`
	MAC         string         `json:"mac"`
	Type        string         `json:"type"`
	Action      string         `json:"action"`
	Description string         `json:"description"`
	Source      string         `json:"source"`
	Priority    int            `json:"priority"`
	Enabled     bool           `json:"enabled"`
	ValidFrom   *time.Time     `json:"validFrom,omitempty"`
	ValidUntil  *time.Time     `json:"validUntil,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type macListRequest struct {
	MAC         string         `json:"mac"`
	Type        string         `json:"type"`
	Action      string         `json:"action"`
	Description string         `json:"description"`
	Source      string         `json:"source"`
	Priority    int            `json:"priority"`
	Enabled     *bool          `json:"enabled"`
	ValidFrom   string         `json:"validFrom"`
	ValidUntil  string         `json:"validUntil"`
	Metadata    map[string]any `json:"metadata"`
}

type macListAuditActionRequest struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
}

func (s *HTTPServer) handleMacListList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.macListSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "mac list service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		page, pageSize, err := parsePageParams(c)
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		entries, err := s.macListSvc.List(ctx, tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		typeFilter := strings.TrimSpace(c.QueryParam("type"))
		actionFilter := strings.TrimSpace(c.QueryParam("action"))
		enabledFilter := strings.TrimSpace(c.QueryParam("enabled"))
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		var enabledPtr *bool
		if enabledFilter != "" {
			if parsed, ok := parseBoolValue(enabledFilter); ok {
				enabledPtr = &parsed
			}
		}
		filtered := make([]macListPayload, 0, len(entries))
		for _, entry := range entries {
			if typeFilter != "" && !strings.EqualFold(string(entry.Type), typeFilter) {
				continue
			}
			if actionFilter != "" && !strings.EqualFold(string(entry.Action), actionFilter) {
				continue
			}
			if enabledPtr != nil && entry.Enabled != *enabledPtr {
				continue
			}
			if keyword != "" {
				mac := strings.ToLower(entry.MAC)
				desc := strings.ToLower(entry.Description)
				source := strings.ToLower(entry.Source)
				if !strings.Contains(mac, keyword) && !strings.Contains(desc, keyword) && !strings.Contains(source, keyword) {
					continue
				}
			}
			filtered = append(filtered, macListPayloadFromEntry(entry))
		}
		total := len(filtered)
		if pageSize <= 0 {
			pageSize = 20
		}
		if page <= 0 {
			page = 1
		}
		offset := (page - 1) * pageSize
		if offset >= total {
			return respondPage(c, StatusOK, []macListPayload{}, int64(total), int64(page), int64(pageSize))
		}
		end := offset + pageSize
		if end > total {
			end = total
		}
		return respondPage(c, StatusOK, filtered[offset:end], int64(total), int64(page), int64(pageSize))
	}
}

func (s *HTTPServer) handleMacListCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.macListSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "mac list service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		var req macListRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		validFrom, err := parseOptionalTimeParam(req.ValidFrom)
		if err != nil {
			return respondValidationError(c, "validFrom 格式错误", FieldError("validFrom", "必须为 RFC3339 时间"))
		}
		validUntil, err := parseOptionalTimeParam(req.ValidUntil)
		if err != nil {
			return respondValidationError(c, "validUntil 格式错误", FieldError("validUntil", "必须为 RFC3339 时间"))
		}
		metadata, err := marshalMetadata(req.Metadata)
		if err != nil {
			return respondValidationError(c, "metadata 格式错误", FieldError("metadata", "必须为 JSON 对象"))
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		entry := maclist.Entry{
			MAC:         req.MAC,
			Type:        maclist.ListType(strings.ToLower(strings.TrimSpace(req.Type))),
			Action:      maclist.Action(strings.ToLower(strings.TrimSpace(req.Action))),
			Description: req.Description,
			Source:      req.Source,
			Priority:    req.Priority,
			Enabled:     enabled,
			ValidFrom:   validFrom,
			ValidUntil:  validUntil,
			Metadata:    metadata,
		}
		created, err := s.macListSvc.Create(c.Request().Context(), tenantID, entry)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		action := "security.mac_list.create"
		if strings.EqualFold(strings.TrimSpace(req.Source), "import") {
			action = "security.mac_list.import"
		}
		change := auditpayload.ConfigChange{
			Resource:   "security_mac_list",
			Action:     action,
			Identifier: created.ID,
			After:      auditMapFromMacListEntry(*created),
		}
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), action, change, withResource("security_mac_list"))
		return respondSuccess(c, StatusOK, "MAC 名单已创建", macListPayloadFromEntry(*created))
	}
}

func (s *HTTPServer) handleMacListUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.macListSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "mac list service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		entryID := strings.TrimSpace(c.Param("entryId"))
		if entryID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "entryId is required")
		}
		var req macListRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		validFrom, err := parseOptionalTimeParam(req.ValidFrom)
		if err != nil {
			return respondValidationError(c, "validFrom 格式错误", FieldError("validFrom", "必须为 RFC3339 时间"))
		}
		validUntil, err := parseOptionalTimeParam(req.ValidUntil)
		if err != nil {
			return respondValidationError(c, "validUntil 格式错误", FieldError("validUntil", "必须为 RFC3339 时间"))
		}
		metadata, err := marshalMetadata(req.Metadata)
		if err != nil {
			return respondValidationError(c, "metadata 格式错误", FieldError("metadata", "必须为 JSON 对象"))
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		entry := maclist.Entry{
			MAC:         req.MAC,
			Type:        maclist.ListType(strings.ToLower(strings.TrimSpace(req.Type))),
			Action:      maclist.Action(strings.ToLower(strings.TrimSpace(req.Action))),
			Description: req.Description,
			Source:      req.Source,
			Priority:    req.Priority,
			Enabled:     enabled,
			ValidFrom:   validFrom,
			ValidUntil:  validUntil,
			Metadata:    metadata,
		}
		beforeEntry, err := s.macListSvc.Get(c.Request().Context(), tenantID, entryID)
		if err != nil && !errors.Is(err, maclist.ErrNotFound) {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		updated, err := s.macListSvc.Update(c.Request().Context(), tenantID, entryID, entry)
		if err != nil {
			if errors.Is(err, maclist.ErrNotFound) {
				return respondNotFound(c, "MAC 名单不存在")
			}
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		action := "security.mac_list.update"
		if strings.EqualFold(strings.TrimSpace(req.Source), "import") {
			action = "security.mac_list.import"
		} else if beforeEntry != nil && beforeEntry.Enabled != updated.Enabled {
			if updated.Enabled {
				action = "security.mac_list.enable"
			} else {
				action = "security.mac_list.disable"
			}
		}
		change := auditpayload.ConfigChange{
			Resource:   "security_mac_list",
			Action:     action,
			Identifier: updated.ID,
			Before: func() map[string]any {
				if beforeEntry == nil {
					return nil
				}
				return auditMapFromMacListEntry(*beforeEntry)
			}(),
			After: auditMapFromMacListEntry(*updated),
		}
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), action, change, withResource("security_mac_list"))
		return respondSuccess(c, StatusOK, "MAC 名单已更新", macListPayloadFromEntry(*updated))
	}
}

func (s *HTTPServer) handleMacListDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.macListSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "mac list service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		entryID := strings.TrimSpace(c.Param("entryId"))
		if entryID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "entryId is required")
		}
		beforeEntry, err := s.macListSvc.Get(c.Request().Context(), tenantID, entryID)
		if err != nil && !errors.Is(err, maclist.ErrNotFound) {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := s.macListSvc.Delete(c.Request().Context(), tenantID, entryID); err != nil {
			if errors.Is(err, maclist.ErrNotFound) {
				return respondNotFound(c, "MAC 名单不存在")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		change := auditpayload.ConfigChange{
			Resource:   "security_mac_list",
			Action:     "security.mac_list.delete",
			Identifier: entryID,
			Before: func() map[string]any {
				if beforeEntry == nil {
					return map[string]any{"id": entryID}
				}
				return auditMapFromMacListEntry(*beforeEntry)
			}(),
		}
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), "security.mac_list.delete", change, withResource("security_mac_list"))
		return respondSuccess(c, StatusOK, "MAC 名单已删除", map[string]string{"id": entryID})
	}
}

func (s *HTTPServer) handleMacListAuditAction() echo.HandlerFunc {
	return func(c echo.Context) error {
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
		}
		var req macListAuditActionRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		switch action {
		case "security.mac_list.import", "security.mac_list.export":
		default:
			return respondValidationError(c, "action 不受支持", FieldError("action", "仅支持 security.mac_list.import / security.mac_list.export"))
		}
		if req.Count < 0 {
			req.Count = 0
		}
		change := auditpayload.ConfigChange{
			Resource: "security_mac_list",
			Action:   action,
			After: map[string]any{
				"count": req.Count,
			},
		}
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), action, change, withResource("security_mac_list"))
		return respondSuccess(c, StatusOK, "审计已记录", map[string]any{"action": action})
	}
}

func macListPayloadFromEntry(entry maclist.Entry) macListPayload {
	return macListPayload{
		ID:          entry.ID,
		MAC:         entry.MAC,
		Type:        string(entry.Type),
		Action:      string(entry.Action),
		Description: entry.Description,
		Source:      entry.Source,
		Priority:    entry.Priority,
		Enabled:     entry.Enabled,
		ValidFrom:   entry.ValidFrom,
		ValidUntil:  entry.ValidUntil,
		Metadata:    decodeMetadata(entry.Metadata),
		CreatedAt:   entry.CreatedAt,
		UpdatedAt:   entry.UpdatedAt,
	}
}

func decodeMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	return result
}

func marshalMetadata(meta map[string]any) ([]byte, error) {
	if meta == nil {
		return nil, nil
	}
	return json.Marshal(meta)
}

func parseOptionalTimeParam(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseBoolValue(value string) (bool, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false, false
	}
	switch value {
	case "true", "1", "yes", "y", "on":
		return true, true
	case "false", "0", "no", "n", "off":
		return false, true
	default:
		return false, false
	}
}

func (s *HTTPServer) handleDHCPReservationList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 200
		}
		offset, err := parseOffsetQuery(c)
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		reservations, err := s.poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, limit, offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		payload := make([]reservationPayload, 0, len(reservations))
		for _, binding := range reservations {
			payload = append(payload, reservationPayloadFromModel(binding))
		}
		pageSize := limit
		if pageSize <= 0 {
			pageSize = len(payload)
			if pageSize == 0 {
				pageSize = 1
			}
		}
		page := 1
		if pageSize > 0 {
			page = offset/pageSize + 1
		}
		return respondPage(c, StatusOK, payload, int64(len(payload)), int64(page), int64(pageSize))
	}
}

func (s *HTTPServer) handleDHCPReservationCreate() echo.HandlerFunc {
	type request struct {
		Identifier     string `json:"identifier"`
		IdentifierType string `json:"identifierType"`
		PoolID         string `json:"poolId"`
		IPAddress      string `json:"ipAddress"`
		LeaseProfileID string `json:"leaseProfileId"`
		Note           string `json:"note"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		identifier := strings.TrimSpace(payload.Identifier)
		identifierType := strings.ToUpper(strings.TrimSpace(payload.IdentifierType))
		if identifier == "" {
			return respondValidationError(c, "需要提供终端标识", fieldError{Field: "identifier", Message: "请输入终端标识"})
		}
		if _, ok := validIdentifierTypes[identifierType]; !ok {
			return respondValidationError(c, "标识类型不受支持", fieldError{Field: "identifierType", Message: "请选择 MAC / Client ID / Dual Stack"})
		}
		poolID := strings.TrimSpace(payload.PoolID)
		if poolID == "" {
			return respondValidationError(c, "需要选择地址池", fieldError{Field: "poolId", Message: "请选择地址池"})
		}
		ipAddress := strings.TrimSpace(payload.IPAddress)
		if ipAddress == "" {
			return respondValidationError(c, "需要指定 IP 地址", fieldError{Field: "ipAddress", Message: "请输入 IP 地址"})
		}
		ctx := c.Request().Context()
		binding, err := s.poolSvc.CreateBinding(ctx, scopeRef, pool.BindingCreateRequest{
			TenantID:       tenantID,
			Identifier:     identifier,
			IdentifierType: identifierType,
			PoolID:         poolID,
			IPAddress:      ipAddress,
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Metadata:       reservationMetadata(payload.Note),
		})
		if err != nil {
			return s.handleBindingError(c, err)
		}
		return respondSuccess(c, StatusCreated, "保留记录已创建", reservationPayloadFromModel(*binding))
	}
}

func (s *HTTPServer) handleDHCPReservationUpdate() echo.HandlerFunc {
	type request struct {
		Identifier     string `json:"identifier"`
		IdentifierType string `json:"identifierType"`
		PoolID         string `json:"poolId"`
		IPAddress      string `json:"ipAddress"`
		LeaseProfileID string `json:"leaseProfileId"`
		Note           string `json:"note"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		reservationID := strings.TrimSpace(c.Param("reservationId"))
		if reservationID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "reservationId is required")
		}
		identifier := strings.TrimSpace(payload.Identifier)
		identifierType := strings.ToUpper(strings.TrimSpace(payload.IdentifierType))
		if identifier == "" {
			return respondValidationError(c, "需要提供终端标识", fieldError{Field: "identifier", Message: "请输入终端标识"})
		}
		if _, ok := validIdentifierTypes[identifierType]; !ok {
			return respondValidationError(c, "标识类型不受支持", fieldError{Field: "identifierType", Message: "请选择 MAC / Client ID / Dual Stack"})
		}
		poolID := strings.TrimSpace(payload.PoolID)
		if poolID == "" {
			return respondValidationError(c, "需要选择地址池", fieldError{Field: "poolId", Message: "请选择地址池"})
		}
		ipAddress := strings.TrimSpace(payload.IPAddress)
		if ipAddress == "" {
			return respondValidationError(c, "需要指定 IP 地址", fieldError{Field: "ipAddress", Message: "请输入 IP 地址"})
		}
		ctx := c.Request().Context()
		binding, err := s.poolSvc.UpdateBinding(ctx, scopeRef, reservationID, pool.BindingCreateRequest{
			TenantID:       tenantID,
			Identifier:     identifier,
			IdentifierType: identifierType,
			PoolID:         poolID,
			IPAddress:      ipAddress,
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Metadata:       reservationMetadata(payload.Note),
		})
		if err != nil {
			return s.handleBindingError(c, err)
		}
		return respondSuccess(c, StatusOK, "保留记录已更新", reservationPayloadFromModel(*binding))
	}
}

func (s *HTTPServer) handleDHCPReservationDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if tenantID == "" {
			tenantID = systemTenantID
			scopeRef = scopeRef.WithTenantOverride(systemTenantID)
		}
		reservationID := strings.TrimSpace(c.Param("reservationId"))
		if reservationID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "reservationId is required")
		}
		if err := s.poolSvc.DeleteBinding(c.Request().Context(), scopeRef, reservationID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleDHCPOptionList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "options disabled")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		scopeFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("scope")))

		items := s.optionStore.list()
		filtered := make([]dhcpOptionTemplate, 0, len(items))
		for _, item := range items {
			if scopeFilter != "" && strings.ToLower(strings.TrimSpace(item.Scope)) != scopeFilter {
				continue
			}
			if keyword != "" {
				name := strings.ToLower(strings.TrimSpace(item.Name))
				desc := strings.ToLower(strings.TrimSpace(item.Description))
				value := strings.ToLower(strings.TrimSpace(item.Value))
				if !strings.Contains(name, keyword) && !strings.Contains(desc, keyword) && !strings.Contains(value, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}

		total := len(filtered)
		if page.Offset >= total {
			currentPage := int64(page.Offset/page.Limit + 1)
			return respondPage(c, StatusOK, []dhcpOptionTemplate{}, int64(total), currentPage, int64(page.Limit))
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		currentPage := int64(page.Offset/page.Limit + 1)
		return respondPage(c, StatusOK, filtered[page.Offset:end], int64(total), currentPage, int64(page.Limit))
	}
}

func (s *HTTPServer) handleDHCPOptionCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "options disabled")
		}
		var payload dhcpOptionTemplate
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入选项名称"})
		}
		if payload.Code <= 0 {
			return respondValidationError(c, "Option Code 必须大于 0", fieldError{Field: "code", Message: "请输入合法的 Option Code"})
		}
		scope := normalizeOptionScope(strings.TrimSpace(payload.Scope))
		if scope == "" {
			return respondValidationError(c, "作用域不受支持", fieldError{Field: "scope", Message: "请选择 GLOBAL / SITE / POOL"})
		}
		format := resolveOptionFormat(payload.Format, payload.DataType)
		dataType := resolveOptionDataType(format, payload.DataType)
		if !isValidOptionFormat(format) {
			return respondValidationError(c, "值格式不受支持", fieldError{Field: "format", Message: "请选择可用的值格式"})
		}
		value := inferOptionValue(payload)
		if value == "" {
			return respondValidationError(c, "值不能为空", fieldError{Field: "value", Message: "请输入配置值"})
		}
		created := dhcpOptionTemplate{
			ID:            uuid.NewString(),
			Name:          name,
			Code:          payload.Code,
			Scope:         scope,
			Format:        format,
			DataType:      dataType,
			Value:         value,
			ValueExample:  strings.TrimSpace(payload.ValueExample),
			AllowedValues: normalizeAllowedValues(payload.AllowedValues),
			SampleValue:   strings.TrimSpace(payload.SampleValue),
			Description:   strings.TrimSpace(payload.Description),
			Tags:          uniqueStrings(payload.Tags),
		}
		if s.optionRepo != nil {
			if saved, err := s.optionRepo.Upsert(c.Request().Context(), created, ""); err == nil {
				created = saved
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		created = s.optionStore.upsert(created)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "option.create", s.auditPayloadFromRequest(c, map[string]any{
			"after": toAuditMap(created),
		}), withResource("dhcp_option"))
		return respondSuccess(c, StatusCreated, "选项已创建", created)
	}
}

func (s *HTTPServer) handleDHCPOptionUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "options disabled")
		}
		optionID := strings.TrimSpace(c.Param("optionId"))
		if optionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "optionId is required")
		}
		var payload dhcpOptionTemplate
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		existing, ok := s.optionStore.get(optionID)
		if !ok {
			return respondNotFound(c, "选项不存在或已被删除")
		}
		before := existing
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入选项名称"})
		}
		if payload.Code <= 0 {
			return respondValidationError(c, "Option Code 必须大于 0", fieldError{Field: "code", Message: "请输入合法的 Option Code"})
		}
		scope := normalizeOptionScope(strings.TrimSpace(payload.Scope))
		if scope == "" {
			return respondValidationError(c, "作用域不受支持", fieldError{Field: "scope", Message: "请选择 GLOBAL / SITE / POOL"})
		}
		format := resolveOptionFormat(payload.Format, payload.DataType)
		dataType := resolveOptionDataType(format, payload.DataType)
		if !isValidOptionFormat(format) {
			return respondValidationError(c, "值格式不受支持", fieldError{Field: "format", Message: "请选择可用的值格式"})
		}
		value := inferOptionValue(payload)
		if value == "" {
			value = strings.TrimSpace(existing.Value)
		}
		if value == "" {
			return respondValidationError(c, "值不能为空", fieldError{Field: "value", Message: "请输入配置值"})
		}
		existing.Name = name
		existing.Code = payload.Code
		existing.Scope = scope
		existing.Format = format
		existing.DataType = dataType
		existing.Value = value
		existing.ValueExample = strings.TrimSpace(payload.ValueExample)
		existing.AllowedValues = normalizeAllowedValues(payload.AllowedValues)
		existing.SampleValue = strings.TrimSpace(payload.SampleValue)
		existing.Description = strings.TrimSpace(payload.Description)
		existing.Tags = uniqueStrings(payload.Tags)
		updated := existing
		if s.optionRepo != nil {
			if saved, err := s.optionRepo.Upsert(c.Request().Context(), updated, ""); err == nil {
				updated = saved
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		updated = s.optionStore.upsert(updated)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "option.update", s.auditPayloadFromRequest(c, map[string]any{
			"optionId": optionID,
			"before":   toAuditMap(before),
			"after":    toAuditMap(updated),
		}), withResource("dhcp_option"))
		return respondSuccess(c, StatusOK, "选项已更新", updated)
	}
}

func (s *HTTPServer) handleDHCPOptionDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "options disabled")
		}
		optionID := strings.TrimSpace(c.Param("optionId"))
		if optionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "optionId is required")
		}
		templateRefs := s.countTemplateReferencesByOption(optionID)
		scopeRefs := 0
		if s.scopeStore != nil {
			scopeRefs = s.scopeStore.optionUsageCount(optionID)
		}
		if templateRefs > 0 || scopeRefs > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("option is referenced by %d templates and %d scopes", templateRefs, scopeRefs))
		}
		if s.optionRepo != nil {
			if err := s.optionRepo.Delete(c.Request().Context(), optionID, ""); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		before, _ := s.optionStore.get(optionID)
		if !s.optionStore.delete(optionID) {
			return echo.NewHTTPError(http.StatusNotFound, "option not found")
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "option.delete", s.auditPayloadFromRequest(c, map[string]any{
			"optionId": optionID,
			"before":   toAuditMap(before),
		}), withResource("dhcp_option"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleDHCOTemplateList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "templates disabled")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))

		items := s.templateStore.list()
		filtered := make([]dhcpConfigTemplate, 0, len(items))
		for _, item := range items {
			if keyword != "" {
				name := strings.ToLower(strings.TrimSpace(item.Name))
				desc := strings.ToLower(strings.TrimSpace(item.Description))
				if !strings.Contains(name, keyword) && !strings.Contains(desc, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}
		total := len(filtered)
		if page.Offset >= total {
			currentPage := int64(page.Offset/page.Limit + 1)
			return respondPage(c, StatusOK, []dhcpConfigTemplate{}, int64(total), currentPage, int64(page.Limit))
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		currentPage := int64(page.Offset/page.Limit + 1)
		return respondPage(c, StatusOK, filtered[page.Offset:end], int64(total), currentPage, int64(page.Limit))
	}
}

func (s *HTTPServer) handleDHCOTemplateCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "templates disabled")
		}
		var payload dhcpConfigTemplate
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入模板名称"})
		}
		options := make([]dhcpOptionTemplate, 0, len(payload.Options))
		for _, opt := range payload.Options {
			if opt.Code <= 0 {
				return respondValidationError(c, "Option Code 必须大于 0", fieldError{Field: "code", Message: "请输入合法的 Option Code"})
			}
			options = append(options, normalizeTemplateOption(opt))
		}
		created := dhcpConfigTemplate{
			ID:          strings.TrimSpace(payload.ID),
			Name:        name,
			Description: strings.TrimSpace(payload.Description),
			Icon:        strings.TrimSpace(payload.Icon),
			Options:     options,
		}
		if s.templateRepo != nil {
			saved, err := s.templateRepo.Upsert(c.Request().Context(), created, "")
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			created = saved
		}
		created = s.templateStore.upsert(created)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "template.create", s.auditPayloadFromRequest(c, map[string]any{
			"after": toAuditMap(created),
		}), withResource("dhcp_template"))
		return respondSuccess(c, StatusCreated, "模板已创建", created)
	}
}

func (s *HTTPServer) handleDHCOTemplateUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "templates disabled")
		}
		templateID := strings.TrimSpace(c.Param("templateId"))
		if templateID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "templateId is required")
		}
		existing, ok := s.templateStore.get(templateID)
		if !ok {
			return respondNotFound(c, "模板不存在或已被删除")
		}
		before := existing
		var payload dhcpConfigTemplate
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := firstNonEmpty(strings.TrimSpace(payload.Name), existing.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入模板名称"})
		}
		options := existing.Options
		if len(payload.Options) > 0 {
			options = make([]dhcpOptionTemplate, 0, len(payload.Options))
			for _, opt := range payload.Options {
				if opt.Code <= 0 {
					return respondValidationError(c, "Option Code 必须大于 0", fieldError{Field: "code", Message: "请输入合法的 Option Code"})
				}
				options = append(options, normalizeTemplateOption(opt))
			}
		}
		updated := dhcpConfigTemplate{
			ID:          templateID,
			Name:        name,
			Description: strings.TrimSpace(payload.Description),
			Icon:        strings.TrimSpace(payload.Icon),
			Options:     options,
		}
		if s.templateRepo != nil {
			saved, err := s.templateRepo.Upsert(c.Request().Context(), updated, "")
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			updated = saved
		}
		updated = s.templateStore.upsert(updated)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "template.update", s.auditPayloadFromRequest(c, map[string]any{
			"templateId": templateID,
			"before":     toAuditMap(before),
			"after":      toAuditMap(updated),
		}), withResource("dhcp_template"))
		return respondSuccess(c, StatusOK, "模板已更新", updated)
	}
}

func (s *HTTPServer) handleDHCOTemplateDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "templates disabled")
		}
		templateID := strings.TrimSpace(c.Param("templateId"))
		if templateID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "templateId is required")
		}
		scopeRefs := 0
		if s.scopeStore != nil {
			scopeRefs = s.scopeStore.templateUsageCount(templateID)
		}
		if scopeRefs > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("template is referenced by %d scopes", scopeRefs))
		}
		if s.templateRepo != nil {
			if err := s.templateRepo.Delete(c.Request().Context(), templateID, ""); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		before, _ := s.templateStore.get(templateID)
		if !s.templateStore.delete(templateID) {
			return respondNotFound(c, "模板不存在或已被删除")
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "template.delete", s.auditPayloadFromRequest(c, map[string]any{
			"templateId": templateID,
			"before":     toAuditMap(before),
		}), withResource("dhcp_template"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleDHCOTemplateApply() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateStore == nil || s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "templates disabled")
		}
		templateID := strings.TrimSpace(c.Param("templateId"))
		if templateID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "templateId is required")
		}
		tpl, ok := s.templateStore.get(templateID)
		if !ok {
			return respondNotFound(c, "模板不存在或已被删除")
		}
		if err := s.templateStore.apply(c.Request().Context(), templateID, s.optionStore, s.optionRepo); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if s.templateApplyHistoryRepo != nil {
			snapshot, _ := json.Marshal(tpl)
			operator := strings.TrimSpace(s.actorFromContext(c))
			if operator == "" {
				operator = "system"
			}
			historyErr := s.templateApplyHistoryRepo.Create(c.Request().Context(), DHCPTemplateApplyHistory{
				TemplateID:         tpl.ID,
				TemplateName:       tpl.Name,
				Action:             "apply",
				Operator:           operator,
				TargetScope:        "global",
				AppliedOptionCount: len(tpl.Options),
				TemplateSnapshot:   snapshot,
				CreatedAt:          time.Now().UTC(),
			})
			if historyErr != nil && s.logger != nil {
				s.logger.Warn("record dhcp template apply history", zap.Error(historyErr), zap.String("templateId", tpl.ID))
			}
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "template.apply", s.auditPayloadFromRequest(c, map[string]any{
			"templateId":         tpl.ID,
			"templateName":       tpl.Name,
			"appliedOptionCount": len(tpl.Options),
			"targetScope":        "global",
		}), withResource("dhcp_template"))
		return respondSuccess(c, StatusOK, "模板已应用", s.optionStore.list())
	}
}

func (s *HTTPServer) handleDHCOTemplateApplyHistory() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.templateApplyHistoryRepo == nil {
			return respondPage(c, StatusOK, []DHCPTemplateApplyHistory{}, 0, 1, 20)
		}
		templateID := strings.TrimSpace(c.Param("templateId"))
		if templateID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "templateId is required")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		items, total, err := s.templateApplyHistoryRepo.ListByTemplate(c.Request().Context(), templateID, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		currentPage := int64(page.Offset/page.Limit + 1)
		return respondPage(c, StatusOK, items, total, currentPage, int64(page.Limit))
	}
}

func (s *HTTPServer) handleDHCPScopeList() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.scopeStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "scopes disabled")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		typeFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("scopeType")))
		templateFilter := strings.TrimSpace(c.QueryParam("templateId"))

		items := s.scopeStore.list()
		if s.scopeRepo != nil {
			dbItems, listErr := s.scopeRepo.List(c.Request().Context(), "")
			if listErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
			}
			items = dbItems
			s.scopeStore.replace(dbItems)
		}
		filtered := make([]dhcpOptionScope, 0, len(items))
		for _, item := range items {
			if typeFilter != "" && strings.ToLower(strings.TrimSpace(item.ScopeType)) != typeFilter {
				continue
			}
			if templateFilter != "" && strings.TrimSpace(item.TemplateID) != templateFilter {
				continue
			}
			if keyword != "" {
				name := strings.ToLower(strings.TrimSpace(item.Name))
				desc := strings.ToLower(strings.TrimSpace(item.Description))
				target := strings.ToLower(strings.TrimSpace(item.Target))
				if !strings.Contains(name, keyword) && !strings.Contains(desc, keyword) && !strings.Contains(target, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}

		total := len(filtered)
		if page.Offset >= total {
			currentPage := int64(page.Offset/page.Limit + 1)
			return respondPage(c, StatusOK, []dhcpOptionScope{}, int64(total), currentPage, int64(page.Limit))
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		currentPage := int64(page.Offset/page.Limit + 1)
		return respondPage(c, StatusOK, filtered[page.Offset:end], int64(total), currentPage, int64(page.Limit))
	}
}

func (s *HTTPServer) handleDHCPScopeCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.scopeStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "scopes disabled")
		}
		var payload dhcpOptionScope
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入作用域名称"})
		}
		templateID := strings.TrimSpace(payload.TemplateID)
		if templateID != "" {
			if _, ok := s.templateStore.get(templateID); !ok {
				return respondValidationError(c, "模板不存在", fieldError{Field: "templateId", Message: "请选择有效模板"})
			}
		}
		optionIDs, err := s.resolveScopeOptionIDs(payload.OptionIDs)
		if err != nil {
			return respondValidationError(c, "选项不存在", fieldError{Field: "optionIds", Message: err.Error()})
		}
		if len(optionIDs) == 0 && templateID != "" {
			optionIDs, err = s.resolveTemplateOptionIDs(templateID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}

		created := dhcpOptionScope{
			ID:          strings.TrimSpace(payload.ID),
			Name:        name,
			Subnet:      strings.TrimSpace(payload.Subnet),
			Range:       strings.TrimSpace(payload.Range),
			Gateway:     strings.TrimSpace(payload.Gateway),
			Status:      strings.TrimSpace(payload.Status),
			ScopeType:   strings.TrimSpace(payload.ScopeType),
			Target:      strings.TrimSpace(payload.Target),
			TemplateID:  templateID,
			OptionIDs:   optionIDs,
			Description: strings.TrimSpace(payload.Description),
			Notes:       strings.TrimSpace(payload.Notes),
		}
		if s.scopeRepo != nil {
			saved, err := s.scopeRepo.Upsert(c.Request().Context(), created, "")
			if err != nil {
				if isScopeDuplicateError(err) {
					return respondValidationError(c, "作用域标识冲突", fieldError{Field: "id", Message: "作用域已存在，请刷新后重试"})
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			created = saved
		}
		created = s.scopeStore.upsert(created)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "scope.create", s.auditPayloadFromRequest(c, map[string]any{
			"after": auditMapFromScope(created),
		}), withResource("dhcp_scope"))
		return respondSuccess(c, StatusCreated, "作用域已创建", created)
	}
}

func (s *HTTPServer) handleDHCPScopeUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.scopeStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "scopes disabled")
		}
		scopeID := strings.TrimSpace(c.Param("scopeId"))
		if scopeID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "scopeId is required")
		}
		existing, ok := s.scopeStore.get(scopeID)
		if s.scopeRepo != nil {
			items, err := s.scopeRepo.List(c.Request().Context(), "")
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			s.scopeStore.replace(items)
			existing = dhcpOptionScope{}
			ok = false
			for _, item := range items {
				if strings.TrimSpace(item.ID) == scopeID {
					existing = item
					ok = true
					break
				}
			}
		}
		if !ok {
			return respondNotFound(c, "作用域不存在或已被删除")
		}
		before := existing
		var payload dhcpOptionScope
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		name := firstNonEmpty(strings.TrimSpace(payload.Name), existing.Name)
		if name == "" {
			return respondValidationError(c, "名称不能为空", fieldError{Field: "name", Message: "请输入作用域名称"})
		}
		templateID := strings.TrimSpace(payload.TemplateID)
		if templateID == "" {
			templateID = existing.TemplateID
		}
		if templateID != "" {
			if _, ok := s.templateStore.get(templateID); !ok {
				return respondValidationError(c, "模板不存在", fieldError{Field: "templateId", Message: "请选择有效模板"})
			}
		}

		optionIDs := existing.OptionIDs
		if payload.OptionIDs != nil {
			resolved, err := s.resolveScopeOptionIDs(payload.OptionIDs)
			if err != nil {
				return respondValidationError(c, "选项不存在", fieldError{Field: "optionIds", Message: err.Error()})
			}
			optionIDs = resolved
		}
		if len(optionIDs) == 0 && templateID != "" {
			resolved, err := s.resolveTemplateOptionIDs(templateID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			optionIDs = resolved
		}

		updated := dhcpOptionScope{
			ID:          existing.ID,
			Name:        name,
			Subnet:      firstNonEmpty(strings.TrimSpace(payload.Subnet), existing.Subnet),
			Range:       firstNonEmpty(strings.TrimSpace(payload.Range), existing.Range),
			Gateway:     firstNonEmpty(strings.TrimSpace(payload.Gateway), existing.Gateway),
			Status:      firstNonEmpty(strings.TrimSpace(payload.Status), existing.Status),
			ScopeType:   firstNonEmpty(strings.TrimSpace(payload.ScopeType), existing.ScopeType),
			Target:      firstNonEmpty(strings.TrimSpace(payload.Target), existing.Target),
			TemplateID:  templateID,
			OptionIDs:   optionIDs,
			Description: firstNonEmpty(strings.TrimSpace(payload.Description), existing.Description),
			Notes:       firstNonEmpty(strings.TrimSpace(payload.Notes), existing.Notes),
		}
		if s.scopeRepo != nil {
			saved, err := s.scopeRepo.Upsert(c.Request().Context(), updated, "")
			if err != nil {
				if isScopeDuplicateError(err) {
					return respondValidationError(c, "作用域标识冲突", fieldError{Field: "id", Message: "作用域已存在，请刷新后重试"})
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			updated = saved
		}
		updated = s.scopeStore.upsert(updated)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "scope.update", s.auditPayloadFromRequest(c, map[string]any{
			"scopeId": scopeID,
			"before":  auditMapFromScope(before),
			"after":   auditMapFromScope(updated),
		}), withResource("dhcp_scope"))
		return respondSuccess(c, StatusOK, "作用域已更新", updated)
	}
}

func isScopeSchemaCompatError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !(strings.Contains(msg, "unknown column") || strings.Contains(msg, "no such column") || strings.Contains(msg, "column does not exist")) {
		return false
	}
	return strings.Contains(msg, "subnet") ||
		strings.Contains(msg, "cidr") ||
		strings.Contains(msg, "range") ||
		strings.Contains(msg, "gateway") ||
		strings.Contains(msg, "status") ||
		strings.Contains(msg, "scope_type") ||
		strings.Contains(msg, "target") ||
		strings.Contains(msg, "template_id") ||
		strings.Contains(msg, "option_ids") ||
		strings.Contains(msg, "description") ||
		strings.Contains(msg, "notes")
}

func isScopeDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "1062") {
		return false
	}
	return strings.Contains(msg, "dhcp_option_scopes") || strings.Contains(msg, "primary")
}

func (s *HTTPServer) handleDHCPScopeDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.scopeStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "scopes disabled")
		}
		scopeID := strings.TrimSpace(c.Param("scopeId"))
		if scopeID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "scopeId is required")
		}
		before, _ := s.scopeStore.get(scopeID)
		if s.scopeRepo != nil {
			items, err := s.scopeRepo.List(c.Request().Context(), "")
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			exists := false
			for _, item := range items {
				if strings.TrimSpace(item.ID) == scopeID {
					exists = true
					before = item
					break
				}
			}
			if !exists {
				return respondNotFound(c, "作用域不存在或已被删除")
			}
			if err := s.scopeRepo.Delete(c.Request().Context(), scopeID, ""); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		if !s.scopeStore.delete(scopeID) && s.scopeRepo == nil {
			return respondNotFound(c, "作用域不存在或已被删除")
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "scope.delete", s.auditPayloadFromRequest(c, map[string]any{
			"scopeId": scopeID,
			"before":  auditMapFromScope(before),
		}), withResource("dhcp_scope"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleDHCOTemplateSync() echo.HandlerFunc {
	type syncPayload struct {
		Mode string `json:"mode"`
	}
	return func(c echo.Context) error {
		if s.templateStore == nil || s.scopeStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "template sync disabled")
		}
		templateID := strings.TrimSpace(c.Param("templateId"))
		if templateID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "templateId is required")
		}
		tpl, ok := s.templateStore.get(templateID)
		if !ok {
			return respondNotFound(c, "模板不存在或已被删除")
		}

		var payload syncPayload
		_ = c.Bind(&payload)
		mode := strings.ToLower(strings.TrimSpace(payload.Mode))
		if mode == "" {
			mode = "replace"
		}
		if mode != "replace" && mode != "merge" {
			return respondValidationError(c, "同步模式不受支持", fieldError{Field: "mode", Message: "请选择 replace 或 merge"})
		}

		templateOptionIDs := make([]string, 0, len(tpl.Options))
		for _, opt := range tpl.Options {
			if id := strings.TrimSpace(opt.ID); id != "" {
				templateOptionIDs = append(templateOptionIDs, id)
			}
		}
		templateOptionIDs = uniqueStrings(templateOptionIDs)

		targets := s.scopeStore.listByTemplate(templateID)
		updatedCount := 0
		for _, scopeItem := range targets {
			next := scopeItem
			if mode == "merge" {
				next.OptionIDs = uniqueStrings(append(next.OptionIDs, templateOptionIDs...))
			} else {
				next.OptionIDs = append([]string(nil), templateOptionIDs...)
			}
			if s.scopeRepo != nil {
				saved, err := s.scopeRepo.Upsert(c.Request().Context(), next, "")
				if err != nil {
					if isScopeDuplicateError(err) {
						return respondValidationError(c, "作用域标识冲突", fieldError{Field: "id", Message: "作用域已存在，请刷新后重试"})
					}
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
				next = saved
			}
			s.scopeStore.upsert(next)
			updatedCount++
		}

		if s.templateApplyHistoryRepo != nil {
			snapshot, _ := json.Marshal(tpl)
			operator := strings.TrimSpace(s.actorFromContext(c))
			if operator == "" {
				operator = "system"
			}
			historyErr := s.templateApplyHistoryRepo.Create(c.Request().Context(), DHCPTemplateApplyHistory{
				TemplateID:         tpl.ID,
				TemplateName:       tpl.Name,
				Action:             "sync",
				Operator:           operator,
				TargetScope:        fmt.Sprintf("scope:%d", updatedCount),
				AppliedOptionCount: len(templateOptionIDs),
				TemplateSnapshot:   snapshot,
				CreatedAt:          time.Now().UTC(),
			})
			if historyErr != nil && s.logger != nil {
				s.logger.Warn("record dhcp template sync history", zap.Error(historyErr), zap.String("templateId", tpl.ID))
			}
		}

		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "scope.sync", s.auditPayloadFromRequest(c, map[string]any{
			"id":            templateID,
			"name":          tpl.Name,
			"templateId":    templateID,
			"templateName":  tpl.Name,
			"mode":          mode,
			"updatedScopes": updatedCount,
			"updatedScopeIds": func() []string {
				ids := make([]string, 0, len(targets))
				for _, scopeItem := range targets {
					id := strings.TrimSpace(scopeItem.ID)
					if id != "" {
						ids = append(ids, id)
					}
				}
				return ids
			}(),
			"updatedScopeNames": func() []string {
				names := make([]string, 0, len(targets))
				for _, scopeItem := range targets {
					name := strings.TrimSpace(scopeItem.Name)
					if name != "" {
						names = append(names, name)
					}
				}
				return names
			}(),
		}), withResource("dhcp_scope"))
		return respondSuccess(c, StatusOK, "模板已同步到引用作用域", map[string]any{
			"templateId":    templateID,
			"mode":          mode,
			"updatedScopes": updatedCount,
		})
	}
}

func (s *HTTPServer) countTemplateReferencesByOption(optionID string) int {
	if s == nil || s.templateStore == nil {
		return 0
	}
	optionID = strings.TrimSpace(optionID)
	if optionID == "" {
		return 0
	}
	count := 0
	for _, tpl := range s.templateStore.list() {
		for _, opt := range tpl.Options {
			if strings.TrimSpace(opt.ID) == optionID {
				count++
				break
			}
		}
	}
	return count
}

func (s *HTTPServer) resolveTemplateOptionIDs(templateID string) ([]string, error) {
	if s == nil || s.templateStore == nil {
		return nil, fmt.Errorf("template store unavailable")
	}
	tpl, ok := s.templateStore.get(templateID)
	if !ok {
		return nil, fmt.Errorf("template not found")
	}
	ids := make([]string, 0, len(tpl.Options))
	for _, opt := range tpl.Options {
		if id := strings.TrimSpace(opt.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return uniqueStrings(ids), nil
}

func (s *HTTPServer) resolveScopeOptionIDs(optionIDs []string) ([]string, error) {
	ids := uniqueStrings(optionIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	if s == nil || s.optionStore == nil {
		return ids, nil
	}
	for _, id := range ids {
		if _, ok := s.optionStore.get(id); !ok {
			return nil, fmt.Errorf("option %s not found", id)
		}
	}
	return ids, nil
}

// handleISCImport parses ISC dhcpd.conf and returns extracted pools/reservations for migration.
func (s *HTTPServer) handleISCImport() echo.HandlerFunc {
	return func(c echo.Context) error {
		var content string
		if file, err := c.FormFile("file"); err == nil && file != nil {
			r, err := file.Open()
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			content = string(b)
		} else {
			b, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			content = string(b)
		}
		if strings.TrimSpace(content) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "dhcpd.conf 内容为空")
		}

		result := parseISCDHCPConf(content)
		return respondSuccess(c, StatusOK, "parsed", result)
	}
}

func (s *HTTPServer) summarizePool(ctx context.Context, scopeRef pool.ResourceScope, poolObj models.AddressPool) (poolSummary, error) {
	if s.poolSvc == nil {
		return poolSummary{}, errors.New("pool service unavailable")
	}
	leaseScope := lease.ResourceScopeFromAccess(scopeRef.AccessScope())
	var allocated int64
	if s.leaseSvc != nil {
		if usage, err := s.leaseSvc.CountActiveLeasesByPool(ctx, leaseScope, []string{poolObj.ID}); err == nil {
			allocated = usage[poolObj.ID]
		}
	}
	bindings, err := s.poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, 500, 0)
	if err != nil {
		return poolSummary{}, err
	}
	filtered := make([]models.StaticBinding, 0)
	for _, binding := range bindings {
		if binding.PoolID == poolObj.ID {
			filtered = append(filtered, binding)
		}
	}
	return buildPoolSummary(poolObj, allocated, filtered), nil
}

func poolUpdateRequestFromModel(poolObj models.AddressPool) pool.PoolUpdateRequest {
	return pool.PoolUpdateRequest{
		PoolID: poolObj.ID,
		PoolCreateRequest: pool.PoolCreateRequest{
			TenantID:       poolObj.TenantID,
			Scope:          poolObj.Scope,
			ParentID:       poolObj.ParentID,
			Name:           poolObj.Name,
			CIDR:           poolObj.CIDR,
			Network:        poolObj.Network,
			Netmask:        poolObj.Netmask,
			RangeStart:     poolObj.RangeStart,
			RangeEnd:       poolObj.RangeEnd,
			Gateway:        poolObj.Gateway,
			Option43:       poolObj.Option43,
			DNS:            poolObj.DNS,
			VLANID:         poolObj.VLANID,
			InterfaceID:    poolObj.InterfaceID,
			SSID:           poolObj.SSID,
			Location:       poolObj.Location,
			ReservePercent: poolObj.ReservePercent,
			MinLeaseTime:   poolObj.MinLeaseTime,
			MaxLeaseTime:   poolObj.MaxLeaseTime,
			LeaseProfileID: poolObj.LeaseProfileID,
			Tags:           poolObj.Tags,
			Exclusions:     poolObj.Exclusions,
			AllocationMode: poolObj.AllocationMode,
			PriorityWeight: poolObj.PriorityWeight,
		},
	}
}

func encodeTagPayload(values ...string) []byte {
	cleaned := uniqueStrings(values)
	if len(cleaned) == 0 {
		return nil
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return nil
	}
	return encoded
}

func normalizePoolMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "static", "priority", "priority_weighted":
		return models.AllocationModePriorityWeighted
	case "round", "round_robin":
		return models.AllocationModeRoundRobin
	default:
		return models.AllocationModeSequential
	}
}

func (s *HTTPServer) resolveLeaseProfileID(ctx context.Context, scopeRef pool.ResourceScope, explicit string) (string, error) {
	trimmed := strings.TrimSpace(explicit)
	if trimmed != "" {
		return trimmed, nil
	}
	if s.poolSvc == nil {
		return "", errors.New("pool service unavailable")
	}
	existing, err := s.poolSvc.ListPools(ctx, scopeRef, 1, 0)
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return strings.TrimSpace(existing[0].LeaseProfileID), nil
	}
	tenant := strings.TrimSpace(scopeRef.TenantOrDefault())
	if tenant == "" {
		return "", errors.New("请指定租约策略 ID")
	}
	return defaultLeaseProfileID, nil
}

func (s *HTTPServer) translatePoolMutationError(c echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pool.ErrInvalidCIDR):
		return respondValidationError(c, "CIDR 不合法", fieldError{Field: "cidr", Message: "请输入合法的 CIDR"})
	case errors.Is(err, pool.ErrInvalidRange):
		return respondValidationError(c, "地址范围不合法", fieldError{Field: "cidr", Message: "请检查容量"})
	case errors.Is(err, pool.ErrLeaseProfileRequired):
		return respondValidationError(c, "需要租约策略", fieldError{Field: "leaseProfileId", Message: "请选择可用的租约策略"})
	case errors.Is(err, pool.ErrLeaseTimeOutOfRange):
		return respondValidationError(c, "租期超出范围", fieldError{Field: "leaseTime", Message: "租期范围应为 300-604800 秒"})
	case errors.Is(err, pool.ErrLeaseTimeOrder):
		return respondValidationError(c, "租期区间不合法", fieldError{Field: "maxLeaseTime", Message: "最长租期必须大于等于最小租期"})
	case errors.Is(err, pool.ErrReserveOutOfRange):
		return respondValidationError(c, "保留比例超出限制", fieldError{Field: "reservePercent", Message: "请输入 0-50%"})
	case errors.Is(err, pool.ErrPoolNotFound) || errors.Is(err, pool.ErrNotFound):
		return respondNotFound(c, "地址池不存在或已被删除")
	case errors.Is(err, tenant.ErrPoolQuotaExceeded):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
}

func ipv4CapacityBounds(prefix netip.Prefix) (netip.Addr, netip.Addr, int64, error) {
	start, end, err := netutil.DefaultHostRange(prefix)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, 0, err
	}
	cap, err := ipv4CapacityBetween(start, end)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, 0, err
	}
	return start, end, cap, nil
}

func ipv4CapacityBetween(start, end netip.Addr) (int64, error) {
	if !start.Is4() || !end.Is4() {
		return 0, errors.New("仅支持计算 IPv4 容量")
	}
	startInt := uint32(start.As4()[0])<<24 | uint32(start.As4()[1])<<16 | uint32(start.As4()[2])<<8 | uint32(start.As4()[3])
	endInt := uint32(end.As4()[0])<<24 | uint32(end.As4()[1])<<16 | uint32(end.As4()[2])<<8 | uint32(end.As4()[3])
	if endInt < startInt {
		return 0, errors.New("range end precedes start")
	}
	return int64(endInt-startInt) + 1, nil
}

func offsetIPv4(addr netip.Addr, offset int64) (netip.Addr, error) {
	if !addr.Is4() {
		return netip.Addr{}, errors.New("仅支持 IPv4 地址计算")
	}
	base := int64(uint32(addr.As4()[0])<<24 | uint32(addr.As4()[1])<<16 | uint32(addr.As4()[2])<<8 | uint32(addr.As4()[3]))
	value := base + offset
	if value < 0 || value > math.MaxUint32 {
		return netip.Addr{}, errors.New("capacity adjustment out of range")
	}
	var octets [4]byte
	octets[0] = byte(value >> 24)
	octets[1] = byte(value >> 16)
	octets[2] = byte(value >> 8)
	octets[3] = byte(value)
	return netip.AddrFrom4(octets), nil
}

// Helper types and functions -------------------------------------------------

type poolSummary struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	CIDR            string               `json:"cidr"`
	Version         int                  `json:"version"`
	Capacity        int64                `json:"capacity"`
	Allocated       int64                `json:"allocated"`
	Utilization     float64              `json:"utilization"`
	Tenant          string               `json:"tenant"`
	Mode            string               `json:"mode"`
	Status          string               `json:"status"`
	FailoverState   string               `json:"failoverState"`
	LeaseTimeSec    int                  `json:"leaseTimeSeconds"`
	LeaseTime       int                  `json:"leaseTime"`
	MaxLeaseTime    int                  `json:"maxLeaseTime"`
	NextAvailable   string               `json:"nextAvailable"`
	Scopes          int                  `json:"scopes"`
	Sites           []string             `json:"sites"`
	Tags            []string             `json:"tags"`
	Gateway         string               `json:"gateway,omitempty"`
	Option43        string               `json:"option43,omitempty"`
	DNS             []string             `json:"dns"`
	Exclude         []string             `json:"exclude"`
	RangeStart      string               `json:"rangeStart,omitempty"`
	RangeEnd        string               `json:"rangeEnd,omitempty"`
	ReservePercent  int                  `json:"reservePercent,omitempty"`
	LeaseProfileID  string               `json:"leaseProfileId,omitempty"`
	AllocationMode  string               `json:"allocationMode,omitempty"`
	PriorityWeight  int                  `json:"priorityWeight,omitempty"`
	VLANID          *int                 `json:"vlanId,omitempty"`
	InterfaceID     *string              `json:"interfaceId,omitempty"`
	SSID            *string              `json:"ssid,omitempty"`
	Location        *string              `json:"location,omitempty"`
	Reservations    []reservationPayload `json:"reservations"`
	LastReconcileAt time.Time            `json:"lastReconcile"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

type reservationPayload struct {
	ID             string    `json:"id"`
	PoolID         string    `json:"poolId"`
	Identifier     string    `json:"identifier"`
	IdentifierType string    `json:"identifierType"`
	IPAddress      string    `json:"ipAddress"`
	LeaseProfileID string    `json:"leaseProfileId"`
	Note           string    `json:"note"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type leasePayload struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenantId"`
	PoolID        string    `json:"poolId"`
	IPAddress     string    `json:"ipAddress"`
	HardwareAddr  string    `json:"hardwareAddr"`
	ClientID      string    `json:"clientId"`
	UserID        string    `json:"userId"`
	State         string    `json:"state"`
	SecurityState string    `json:"securityState"`
	ExpiresAt     time.Time `json:"expiresAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func buildPoolSummary(poolObj models.AddressPool, allocated int64, reservations []models.StaticBinding) poolSummary {
	capacity := monitoring.CalculatePoolCapacity(poolObj)
	tags := decodeTagSet(poolObj.Tags)
	status := strings.ToLower(strings.TrimSpace(poolObj.Status))
	if status == "" {
		status = "active"
	}
	failover := "primary"
	leaseTime := poolObj.MinLeaseTime
	if leaseTime <= 0 {
		leaseTime = 3600
	}
	maxLeaseTime := poolObj.MaxLeaseTime
	if maxLeaseTime < leaseTime {
		maxLeaseTime = leaseTime
	}
	reservationsPayload := make([]reservationPayload, 0, len(reservations))
	for _, res := range reservations {
		reservationsPayload = append(reservationsPayload, reservationPayloadFromModel(res))
	}
	sort.Slice(reservationsPayload, func(i, j int) bool {
		return reservationsPayload[i].UpdatedAt.After(reservationsPayload[j].UpdatedAt)
	})
	utilization := 0.0
	if capacity > 0 {
		utilization = math.Round(float64(allocated)*1000/float64(capacity)) / 10.0
	}
	if status != "disabled" && utilization > 85 {
		status = "warning"
	}
	version := 4
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(poolObj.CIDR)); err == nil && prefix.Addr().Is6() {
		version = 6
	}
	summary := poolSummary{
		ID:              poolObj.ID,
		Name:            poolObj.Name,
		CIDR:            poolObj.CIDR,
		Version:         version,
		Capacity:        capacity,
		Allocated:       allocated,
		Utilization:     utilization,
		Tenant:          poolObj.TenantID,
		Mode:            strings.ToLower(strings.TrimSpace(poolObj.AllocationMode)),
		Status:          status,
		FailoverState:   failover,
		LeaseTimeSec:    leaseTime,
		LeaseTime:       leaseTime,
		MaxLeaseTime:    maxLeaseTime,
		NextAvailable:   strings.TrimSpace(poolObj.RangeStart),
		Scopes:          1,
		Sites:           nil,
		Tags:            tags,
		Gateway:         strings.TrimSpace(poolObj.Gateway),
		Option43:        strings.TrimSpace(poolObj.Option43),
		DNS:             []string(poolObj.DNS),
		Exclude:         flattenExclusions(poolObj.Exclusions),
		RangeStart:      strings.TrimSpace(poolObj.RangeStart),
		RangeEnd:        strings.TrimSpace(poolObj.RangeEnd),
		ReservePercent:  poolObj.ReservePercent,
		LeaseProfileID:  strings.TrimSpace(poolObj.LeaseProfileID),
		AllocationMode:  strings.TrimSpace(poolObj.AllocationMode),
		PriorityWeight:  poolObj.PriorityWeight,
		VLANID:          poolObj.VLANID,
		InterfaceID:     poolObj.InterfaceID,
		SSID:            poolObj.SSID,
		Location:        poolObj.Location,
		Reservations:    reservationsPayload,
		UpdatedAt:       poolObj.UpdatedAt,
		LastReconcileAt: poolObj.UpdatedAt,
	}
	return summary
}

func reservationPayloadFromModel(binding models.StaticBinding) reservationPayload {
	return reservationPayload{
		ID:             binding.ID,
		PoolID:         binding.PoolID,
		Identifier:     binding.Identifier,
		IdentifierType: binding.IdentifierType,
		IPAddress:      binding.IPAddress,
		LeaseProfileID: binding.LeaseProfileID,
		Note:           extractReservationNote(binding.Metadata),
		UpdatedAt:      binding.UpdatedAt,
	}
}

func leasePayloadFromModel(lease models.Lease) leasePayload {
	return leasePayload{
		ID:            lease.ID,
		TenantID:      lease.TenantID,
		PoolID:        lease.PoolID,
		IPAddress:     lease.IPAddress,
		HardwareAddr:  lease.HardwareAddr,
		ClientID:      lease.ClientID,
		UserID:        lease.UserID,
		State:         lease.State,
		SecurityState: lease.SecurityState,
		ExpiresAt:     lease.ExpiresAt,
		UpdatedAt:     lease.UpdatedAt,
	}
}

func extractReservationNote(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]string
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	return metadata["note"]
}

func reservationMetadata(note string) []byte {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"note": strings.TrimSpace(note)})
	return payload
}

func decodeTagSet(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return uniqueStrings(arr)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		key = strings.ToLower(key)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

type leaseFilterParams struct {
	query                string
	subnet               string
	tenant               string
	ipStart              string
	ipEnd                string
	macPrefix            string
	securityStatesFilter string
	updatedWithin        string
}

func filterLeasesForQuery(leases []models.Lease, params leaseFilterParams) []models.Lease {
	var startAddr, endAddr netip.Addr
	if addr, err := netip.ParseAddr(strings.TrimSpace(params.ipStart)); err == nil {
		startAddr = addr
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(params.ipEnd)); err == nil {
		endAddr = addr
	}
	macPrefix := strings.ToLower(strings.ReplaceAll(params.macPrefix, ":", ""))
	var updatedWindow time.Duration
	if minutes, err := strconv.Atoi(strings.TrimSpace(params.updatedWithin)); err == nil && minutes > 0 {
		updatedWindow = time.Duration(minutes) * time.Minute
	}
	securitySet := map[string]struct{}{}
	if strings.TrimSpace(params.securityStatesFilter) != "" {
		for _, state := range strings.Split(params.securityStatesFilter, ",") {
			trimmed := strings.ToUpper(strings.TrimSpace(state))
			if trimmed != "" {
				securitySet[trimmed] = struct{}{}
			}
		}
	}
	query := strings.ToLower(strings.TrimSpace(params.query))
	subnet := strings.TrimSpace(params.subnet)
	tenant := strings.TrimSpace(params.tenant)

	now := time.Now()
	filtered := make([]models.Lease, 0, len(leases))
	for _, lease := range leases {
		if subnet != "" && !strings.EqualFold(subnet, lease.PoolID) {
			continue
		}
		if tenant != "" && !strings.EqualFold(tenant, lease.TenantID) {
			continue
		}
		if len(securitySet) > 0 {
			if _, ok := securitySet[strings.ToUpper(lease.SecurityState)]; !ok {
				continue
			}
		}
		if !startAddr.IsValid() && !endAddr.IsValid() {
			// no-op
		} else {
			addr, err := netip.ParseAddr(lease.IPAddress)
			if err != nil {
				continue
			}
			if startAddr.IsValid() && addr.Compare(startAddr) < 0 {
				continue
			}
			if endAddr.IsValid() && addr.Compare(endAddr) > 0 {
				continue
			}
		}
		if macPrefix != "" {
			normalized := strings.ToLower(strings.ReplaceAll(lease.HardwareAddr, ":", ""))
			if !strings.HasPrefix(normalized, macPrefix) {
				continue
			}
		}
		if query != "" {
			match := strings.Contains(strings.ToLower(lease.IPAddress), query) ||
				strings.Contains(strings.ToLower(lease.HardwareAddr), query) ||
				strings.Contains(strings.ToLower(lease.ClientID), query)
			if !match {
				continue
			}
		}
		if updatedWindow > 0 {
			if now.Sub(lease.UpdatedAt) > updatedWindow {
				continue
			}
		}
		filtered = append(filtered, lease)
	}
	return filtered
}

func parseOffsetQuery(c echo.Context) (int, error) {
	raw := strings.TrimSpace(c.QueryParam("offset"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "offset must be a positive integer")
	}
	return value, nil
}

var (
	validIdentifierTypes = map[string]struct{}{
		"MAC":        {},
		"CLIENT_ID":  {},
		"DUAL_STACK": {},
	}
	validOptionScopes = map[string]struct{}{
		"GLOBAL": {},
		"SITE":   {},
		"POOL":   {},
	}
	validOptionFormats = map[string]struct{}{
		"ipv4":      {},
		"ipv4-list": {},
		"string":    {},
		"integer":   {},
	}
)

func resolveOptionFormat(format, dataType string) string {
	f := strings.TrimSpace(format)
	if f == "" {
		f = strings.TrimSpace(dataType)
	}
	f = strings.ToLower(f)
	switch f {
	case "ip", "ipv4":
		return "ipv4"
	case "ip-list", "ipv4-list":
		return "ipv4-list"
	case "uint8", "uint16", "uint32", "integer":
		return "integer"
	case "boolean", "domain", "fqdn", "hex", "string":
		return "string"
	default:
		return f
	}
}

func resolveOptionDataType(format, dataType string) string {
	dt := strings.TrimSpace(dataType)
	if dt != "" {
		return dt
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "ipv4":
		return "ip"
	case "ipv4-list":
		return "ip-list"
	case "integer":
		return "uint32"
	default:
		return "string"
	}
}

func inferOptionValue(payload dhcpOptionTemplate) string {
	if v := strings.TrimSpace(payload.Value); v != "" {
		return v
	}
	if v := strings.TrimSpace(payload.ValueExample); v != "" {
		return v
	}
	if len(payload.AllowedValues) > 0 {
		if v := strings.TrimSpace(payload.AllowedValues[0]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(payload.SampleValue); v != "" {
		return v
	}
	return ""
}

func normalizeAllowedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if val := strings.TrimSpace(v); val != "" {
			out = append(out, val)
		}
	}
	return out
}

func normalizeOptionScope(value string) string {
	scope := strings.ToUpper(strings.TrimSpace(value))
	if scope == "" {
		scope = "GLOBAL"
	}
	if _, ok := validOptionScopes[scope]; !ok {
		return ""
	}
	return scope
}

func isValidOptionFormat(value string) bool {
	_, ok := validOptionFormats[strings.TrimSpace(strings.ToLower(value))]
	return ok
}

func isDuplicateError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}

func (s *HTTPServer) getBindingByID(ctx context.Context, scopeRef pool.ResourceScope, bindingID string) (*models.StaticBinding, error) {
	if s.poolSvc == nil {
		return nil, errors.New("pool service unavailable")
	}
	const pageSize = 200
	offset := 0
	for {
		bindings, err := s.poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(bindings) == 0 {
			break
		}
		for _, binding := range bindings {
			if binding.ID == bindingID {
				b := binding
				return &b, nil
			}
		}
		if len(bindings) < pageSize {
			break
		}
		offset += pageSize
	}
	return nil, pool.ErrBindingNotFound
}

func (s *HTTPServer) handleBindingError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, pool.ErrPoolAmbiguous):
		return respondValidationError(c, err.Error(), fieldError{Field: "poolId", Message: "IP 匹配到多个地址池，请指定 poolId"})
	case errors.Is(err, pool.ErrPoolNotFound):
		return respondValidationError(c, err.Error(), fieldError{Field: "poolId", Message: "请选择有效的地址池或调整 IP"})
	case errors.Is(err, pool.ErrBindingNotFound):
		return respondNotFound(c, "保留记录不存在或已被删除")
	}
	var parseErr *net.ParseError
	if errors.As(err, &parseErr) {
		return respondValidationError(c, "IP 地址格式不正确", fieldError{Field: "ipAddress", Message: "请输入合法的 IP 地址"})
	}
	if strings.Contains(strings.ToLower(err.Error()), "identifier") {
		return respondValidationError(c, err.Error(), fieldError{Field: "identifier", Message: err.Error()})
	}
	if isDuplicateError(err) {
		return respondValidationError(c, "该标识已存在保留记录", fieldError{Field: "identifier", Message: "请使用唯一的终端标识"})
	}
	return respondError(c, StatusBadRequest, CodeBusinessRule, err.Error(), newErrorDetails(ErrorTypeBusiness, "请检查输入参数后重试"))
}
