package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/netutil"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/pkg/models"
)

func (s *HTTPServer) mountCoreOpsRoutes(group *echo.Group) {
	pools := group.Group("/pools")
	pools.GET("", s.handleDHCPPoolsList(), RequireCapability(CapabilityPoolRead))
	poolsManage := pools.Group("", RequireCapability(CapabilityPoolWrite))
	poolsManage.POST("", s.handleDHCPPoolsCreate())
	poolsManage.PATCH("/:poolId", s.handleDHCPPoolsUpdate())
	poolsManage.POST("/:poolId/reconcile", s.handleDHCPPoolsReconcile())

	leases := group.Group("/leases")
	leases.GET("", s.handleDHCPLeaseList(), RequireCapability(CapabilityLeaseRead))
	leasesManage := leases.Group("", RequireCapability(CapabilityLeaseManage))
	leasesManage.POST("/:leaseId:release", s.handleDHCPLeaseRelease())

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
}

func (s *HTTPServer) handleDHCPPoolsList() echo.HandlerFunc {
	type response struct {
		Items []poolSummary `json:"items"`
		Total int           `json:"total"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 100
		}
		offset, err := parseOffsetQuery(c)
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		pools, err := s.poolSvc.ListPools(ctx, tenantID, limit, offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		poolIDs := make([]string, 0, len(pools))
		for _, poolObj := range pools {
			poolIDs = append(poolIDs, poolObj.ID)
		}
		counts := map[string]int64{}
		if s.leaseSvc != nil && len(poolIDs) > 0 {
			if usage, err := s.leaseSvc.CountActiveLeasesByPool(ctx, tenantID, poolIDs); err == nil {
				counts = usage
			}
		}
		bindings, err := s.poolSvc.ListBindings(ctx, tenantID, 500, 0)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		byPool := map[string][]models.StaticBinding{}
		for _, binding := range bindings {
			byPool[binding.PoolID] = append(byPool[binding.PoolID], binding)
		}
		summaries := make([]poolSummary, 0, len(pools))
		for _, poolObj := range pools {
			summaries = append(summaries, buildPoolSummary(poolObj, counts[poolObj.ID], byPool[poolObj.ID]))
		}
		return c.JSON(http.StatusOK, response{Items: summaries, Total: len(summaries)})
	}
}

func (s *HTTPServer) handleDHCPPoolsCreate() echo.HandlerFunc {
	type request struct {
		Name             string   `json:"name"`
		CIDR             string   `json:"cidr"`
		Capacity         int      `json:"capacity"`
		Mode             string   `json:"mode"`
		LeaseStrategy    string   `json:"leaseStrategy"`
		LeaseTimeSeconds int      `json:"leaseTimeSeconds"`
		Tags             []string `json:"tags"`
		Sites            []string `json:"sites"`
		LeaseProfileID   string   `json:"leaseProfileId"`
		ReservePercent   int      `json:"reservePercent"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		capacity := payload.Capacity
		if capacity <= 0 {
			capacity = 256
		}
		start, _, maxCapacity, err := ipv4CapacityBounds(prefix)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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
		leaseProfileID, err := s.resolveLeaseProfileID(ctx, tenantID, payload.LeaseProfileID)
		if err != nil {
			return respondValidationError(c, "需要提供租约策略 ID", fieldError{Field: "leaseProfileId", Message: err.Error()})
		}
		tags := encodeTagPayload(append(payload.Tags, payload.Sites...)...)
		poolObj, err := s.poolSvc.CreatePool(ctx, pool.PoolCreateRequest{
			TenantID:       tenantID,
			Scope:          "GLOBAL",
			Name:           name,
			CIDR:           prefix.String(),
			RangeStart:     start.String(),
			RangeEnd:       end.String(),
			ReservePercent: reservePercent,
			LeaseProfileID: leaseProfileID,
			Tags:           tags,
			AllocationMode: normalizePoolMode(payload.Mode),
		})
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		summary, err := s.summarizePool(ctx, tenantID, *poolObj)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summary.LastReconcileAt = time.Now().UTC()
		return c.JSON(http.StatusCreated, summary)
	}
}

func (s *HTTPServer) handleDHCPPoolsUpdate() echo.HandlerFunc {
	type request struct {
		CapacityDelta *int   `json:"capacityDelta"`
		Status        string `json:"status"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, tenantID, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if payload.CapacityDelta != nil && *payload.CapacityDelta != 0 {
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
			updated, err := s.poolSvc.UpdatePool(ctx, req)
			if err != nil {
				return s.translatePoolMutationError(c, err)
			}
			summary, err := s.summarizePool(ctx, tenantID, *updated)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			return c.JSON(http.StatusOK, summary)
		}
		if strings.TrimSpace(payload.Status) != "" {
			return respondValidationError(c, "地址池状态切换暂未开放", fieldError{Field: "status", Message: "后端尚未启用状态切换"})
		}
		return echo.NewHTTPError(http.StatusBadRequest, "no supported mutation specified")
	}
}

func (s *HTTPServer) handleDHCPPoolsReconcile() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		poolObj, err := s.poolSvc.GetPool(ctx, tenantID, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrNotFound) || errors.Is(err, pool.ErrPoolNotFound) {
				return respondNotFound(c, "地址池不存在或已被删除")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		req := poolUpdateRequestFromModel(*poolObj)
		updated, err := s.poolSvc.UpdatePool(ctx, req)
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		summary, err := s.summarizePool(ctx, tenantID, *updated)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summary.LastReconcileAt = time.Now().UTC()
		return c.JSON(http.StatusOK, summary)
	}
}

func (s *HTTPServer) handleDHCPLeaseList() echo.HandlerFunc {
	type response struct {
		Items []leasePayload `json:"items"`
		Total int            `json:"total"`
	}
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		leases, err := s.leaseSvc.ListLeases(ctx, tenantID, state, limit, offset)
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
		return c.JSON(http.StatusOK, response{Items: payload, Total: len(payload)})
	}
}

func (s *HTTPServer) handleDHCPLeaseRelease() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.leaseSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		leaseID := strings.TrimSpace(c.Param("leaseId"))
		if leaseID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "leaseId is required")
		}
		ctx := c.Request().Context()
		leaseObj, _, err := s.leaseSvc.ReleaseLease(ctx, tenantID, leaseID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if leaseObj == nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "lease not returned")
		}
		return c.JSON(http.StatusOK, leasePayloadFromModel(*leaseObj))
	}
}

func (s *HTTPServer) handleDHCPReservationList() echo.HandlerFunc {
	type response struct {
		Items []reservationPayload `json:"items"`
		Total int                  `json:"total"`
	}
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		reservations, err := s.poolSvc.ListBindings(ctx, tenantID, limit, offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		payload := make([]reservationPayload, 0, len(reservations))
		for _, binding := range reservations {
			payload = append(payload, reservationPayloadFromModel(binding))
		}
		return c.JSON(http.StatusOK, response{Items: payload, Total: len(payload)})
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
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		binding, err := s.poolSvc.CreateBinding(ctx, pool.BindingCreateRequest{
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
		return c.JSON(http.StatusCreated, reservationPayloadFromModel(*binding))
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
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		binding, err := s.poolSvc.UpdateBinding(ctx, reservationID, pool.BindingCreateRequest{
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
		return c.JSON(http.StatusOK, reservationPayloadFromModel(*binding))
	}
}

func (s *HTTPServer) handleDHCPReservationDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := s.requestTenantID(c)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		reservationID := strings.TrimSpace(c.Param("reservationId"))
		if reservationID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "reservationId is required")
		}
		if err := s.poolSvc.DeleteBinding(c.Request().Context(), tenantID, reservationID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleDHCPOptionList() echo.HandlerFunc {
	type response struct {
		Items []dhcpOptionTemplate `json:"items"`
		Total int                  `json:"total"`
	}
	return func(c echo.Context) error {
		if s.optionStore == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "options disabled")
		}
		items := s.optionStore.list()
		return c.JSON(http.StatusOK, response{Items: items, Total: len(items)})
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
		format := strings.TrimSpace(payload.Format)
		if !isValidOptionFormat(format) {
			return respondValidationError(c, "值格式不受支持", fieldError{Field: "format", Message: "请选择可用的值格式"})
		}
		value := strings.TrimSpace(payload.Value)
		if value == "" {
			return respondValidationError(c, "值不能为空", fieldError{Field: "value", Message: "请输入配置值"})
		}
		created := s.optionStore.upsert(dhcpOptionTemplate{
			ID:          uuid.NewString(),
			Name:        name,
			Code:        payload.Code,
			Scope:       scope,
			Format:      format,
			Value:       value,
			Description: strings.TrimSpace(payload.Description),
			Tags:        uniqueStrings(payload.Tags),
		})
		return c.JSON(http.StatusCreated, created)
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
		format := strings.TrimSpace(payload.Format)
		if !isValidOptionFormat(format) {
			return respondValidationError(c, "值格式不受支持", fieldError{Field: "format", Message: "请选择可用的值格式"})
		}
		value := strings.TrimSpace(payload.Value)
		if value == "" {
			return respondValidationError(c, "值不能为空", fieldError{Field: "value", Message: "请输入配置值"})
		}
		existing.Name = name
		existing.Code = payload.Code
		existing.Scope = scope
		existing.Format = format
		existing.Value = value
		existing.Description = strings.TrimSpace(payload.Description)
		existing.Tags = uniqueStrings(payload.Tags)
		updated := s.optionStore.upsert(existing)
		return c.JSON(http.StatusOK, updated)
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
		if !s.optionStore.delete(optionID) {
			return echo.NewHTTPError(http.StatusNotFound, "option not found")
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) summarizePool(ctx context.Context, tenantID string, poolObj models.AddressPool) (poolSummary, error) {
	if s.poolSvc == nil {
		return poolSummary{}, errors.New("pool service unavailable")
	}
	var allocated int64
	if s.leaseSvc != nil {
		if usage, err := s.leaseSvc.CountActiveLeasesByPool(ctx, tenantID, []string{poolObj.ID}); err == nil {
			allocated = usage[poolObj.ID]
		}
	}
	bindings, err := s.poolSvc.ListBindings(ctx, tenantID, 500, 0)
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
			VLANID:         poolObj.VLANID,
			InterfaceID:    poolObj.InterfaceID,
			SSID:           poolObj.SSID,
			Location:       poolObj.Location,
			ReservePercent: poolObj.ReservePercent,
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

func (s *HTTPServer) resolveLeaseProfileID(ctx context.Context, tenantID, explicit string) (string, error) {
	trimmed := strings.TrimSpace(explicit)
	if trimmed != "" {
		return trimmed, nil
	}
	if s.poolSvc == nil {
		return "", errors.New("pool service unavailable")
	}
	existing, err := s.poolSvc.ListPools(ctx, tenantID, 1, 0)
	if err != nil {
		return "", err
	}
	if len(existing) == 0 {
		return "", errors.New("请指定租约策略 ID")
	}
	return strings.TrimSpace(existing[0].LeaseProfileID), nil
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
	Capacity        int64                `json:"capacity"`
	Allocated       int64                `json:"allocated"`
	Reserved        int                  `json:"reserved"`
	Tenant          string               `json:"tenant"`
	Mode            string               `json:"mode"`
	Status          string               `json:"status"`
	FailoverState   string               `json:"failoverState"`
	LeaseTimeSec    int                  `json:"leaseTimeSeconds"`
	NextAvailable   string               `json:"nextAvailable"`
	Scopes          int                  `json:"scopes"`
	Sites           []string             `json:"sites"`
	Tags            []string             `json:"tags"`
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
	status := "ok"
	failover := "primary"
	leaseTime := 3600
	reservationsPayload := make([]reservationPayload, 0, len(reservations))
	for _, res := range reservations {
		reservationsPayload = append(reservationsPayload, reservationPayloadFromModel(res))
	}
	sort.Slice(reservationsPayload, func(i, j int) bool {
		return reservationsPayload[i].UpdatedAt.After(reservationsPayload[j].UpdatedAt)
	})
	summary := poolSummary{
		ID:              poolObj.ID,
		Name:            poolObj.Name,
		CIDR:            poolObj.CIDR,
		Capacity:        capacity,
		Allocated:       allocated,
		Reserved:        len(reservationsPayload),
		Tenant:          poolObj.TenantID,
		Mode:            strings.ToLower(strings.TrimSpace(poolObj.AllocationMode)),
		Status:          status,
		FailoverState:   failover,
		LeaseTimeSec:    leaseTime,
		NextAvailable:   strings.TrimSpace(poolObj.RangeStart),
		Scopes:          1,
		Sites:           nil,
		Tags:            tags,
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

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type apiErrorResponse struct {
	Message     string       `json:"message"`
	FieldErrors []fieldError `json:"fieldErrors,omitempty"`
}

func respondValidationError(c echo.Context, message string, fields ...fieldError) error {
	payload := apiErrorResponse{Message: message}
	if len(fields) > 0 {
		payload.FieldErrors = fields
	}
	return c.JSON(http.StatusBadRequest, payload)
}

func respondNotFound(c echo.Context, message string) error {
	return c.JSON(http.StatusNotFound, apiErrorResponse{Message: message})
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

func (s *HTTPServer) handleBindingError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, pool.ErrPoolNotFound):
		return respondValidationError(c, "选择的地址池不存在或不可访问", fieldError{Field: "poolId", Message: "请选择有效的地址池"})
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
	return c.JSON(http.StatusBadRequest, apiErrorResponse{Message: err.Error()})
}
