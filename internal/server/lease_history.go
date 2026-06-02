package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/reporting"
	"modern-dhcp/pkg/models"
)

type leaseHistoryItem struct {
	LeaseID       string     `json:"leaseId"`
	PoolID        string     `json:"poolId"`
	IPAddress     string     `json:"ipAddress"`
	Identifier    string     `json:"identifier"`
	ClientID      string     `json:"clientId"`
	State         string     `json:"state"`
	SecurityState string     `json:"securityState"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	CooldownUntil *time.Time `json:"cooldownUntil,omitempty"`
}

type leaseHistoryExportPayload struct {
	Format      string `json:"format"`
	Destination string `json:"destination"`
	State       string `json:"state"`
	Identifier  string `json:"identifier"`
	IPAddress   string `json:"ipAddress"`
	From        string `json:"from"`
	To          string `json:"to"`
	Limit       int    `json:"limit"`
}

type leaseDailySchedulePayload struct {
	TenantID      string `json:"tenantId"`
	Format        string `json:"format"`
	Destination   string `json:"destination"`
	IntervalHours int    `json:"intervalHours"`
	State         string `json:"state"`
	Identifier    string `json:"identifier"`
	IPAddress     string `json:"ipAddress"`
	From          string `json:"from"`
	To            string `json:"to"`
	Limit         int    `json:"limit"`
}

func (s *HTTPServer) handleLeaseHistoryList(leaseSvc *lease.Service) echo.HandlerFunc {
	if leaseSvc == nil {
		return func(c echo.Context) error {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "lease service disabled")
		}
	}
	type response struct {
		TenantID string             `json:"tenantId"`
		Items    []leaseHistoryItem `json:"items"`
		Limit    int                `json:"limit"`
		Offset   int                `json:"offset"`
		Total    int                `json:"total"`
	}
	return func(c echo.Context) error {
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		filter, err := buildLeaseHistoryFilter(queryFilterInput{
			state:      c.QueryParam("state"),
			identifier: c.QueryParam("identifier"),
			ip:         c.QueryParam("ip"),
			from:       c.QueryParam("from"),
			to:         c.QueryParam("to"),
			poolId:     c.QueryParam("poolId"),
			limit:      page.Limit,
			offset:     page.Offset,
		})
		if err != nil {
			return err
		}
		scopeRef := s.leaseScopeRef(c).WithTenantOverride(tenantID)
		records, total, err := leaseSvc.History(c.Request().Context(), scopeRef, filter)
		if err != nil {
			if errors.Is(err, lease.ErrTenantRequired) {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := response{
			TenantID: tenantID,
			Items:    mapLeaseHistoryItems(records),
			Limit:    page.Limit,
			Offset:   page.Offset,
			Total:    total,
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleLeaseHistoryExport() echo.HandlerFunc {
	type response struct {
		Status   string             `json:"status"`
		Artifact reporting.Artifact `json:"artifact"`
	}
	return func(c echo.Context) error {
		if s.reportSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		var payload leaseHistoryExportPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		filter, err := buildLeaseHistoryFilter(queryFilterInput{
			state:      payload.State,
			identifier: payload.Identifier,
			ip:         payload.IPAddress,
			from:       payload.From,
			to:         payload.To,
			poolId:     "",
			limit:      payload.Limit,
			offset:     0,
		})
		if err != nil {
			return err
		}
		scope := s.leaseScopeRef(c).WithTenantOverride(tenantID)
		artifact, err := s.reportSvc.ExportLeaseHistory(c.Request().Context(), reporting.LeaseHistoryExportRequest{
			TenantID:    tenantID,
			Scope:       scope,
			Format:      reporting.ParseFormat(payload.Format),
			Destination: payload.Destination,
			Filter:      filter,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusCreated, response{Status: "exported", Artifact: artifact})
	}
}

func (s *HTTPServer) handleLeaseDailySchedule() echo.HandlerFunc {
	type response struct {
		Status string                     `json:"status"`
		Job    reporting.ExportJobSummary `json:"job"`
	}
	return func(c echo.Context) error {
		if s.reportSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
		}
		var payload leaseDailySchedulePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = systemTenantID
		}
		filter, err := buildLeaseHistoryFilter(queryFilterInput{
			state:      payload.State,
			identifier: payload.Identifier,
			ip:         payload.IPAddress,
			from:       payload.From,
			to:         payload.To,
			poolId:     "",
			limit:      payload.Limit,
			offset:     0,
		})
		if err != nil {
			return err
		}
		scope := s.leaseScopeRef(c).WithTenantOverride(tenantID)
		interval := time.Duration(payload.IntervalHours) * time.Hour
		job, err := s.reportSvc.ScheduleLeaseHistoryExport(reporting.LeaseHistoryScheduleRequest{
			TenantID:    tenantID,
			Scope:       scope,
			Format:      reporting.ParseFormat(payload.Format),
			Destination: payload.Destination,
			Filter:      filter,
			Interval:    interval,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusAccepted, response{Status: "scheduled", Job: job})
	}
}

type queryFilterInput struct {
	state      string
	identifier string
	ip         string
	from       string
	to         string
	poolId     string
	limit      int
	offset     int
}

func buildLeaseHistoryFilter(input queryFilterInput) (models.LeaseHistoryFilter, error) {
	filter := models.LeaseHistoryFilter{
		State:      strings.ToUpper(strings.TrimSpace(input.state)),
		Identifier: strings.TrimSpace(input.identifier),
		IPAddress:  strings.TrimSpace(input.ip),
		PoolID:     strings.TrimSpace(input.poolId),
		Limit:      input.limit,
		Offset:     input.offset,
	}
	if input.from != "" {
		parsed, err := time.Parse(time.RFC3339, input.from)
		if err != nil {
			return models.LeaseHistoryFilter{}, echo.NewHTTPError(http.StatusBadRequest, "from must be RFC3339 timestamp")
		}
		filter.From = parsed
	}
	if input.to != "" {
		parsed, err := time.Parse(time.RFC3339, input.to)
		if err != nil {
			return models.LeaseHistoryFilter{}, echo.NewHTTPError(http.StatusBadRequest, "to must be RFC3339 timestamp")
		}
		filter.To = parsed
	}
	return filter, nil
}

func mapLeaseHistoryItems(records []models.Lease) []leaseHistoryItem {
	items := make([]leaseHistoryItem, 0, len(records))
	for _, lease := range records {
		items = append(items, leaseHistoryItem{
			LeaseID:       lease.ID,
			PoolID:        lease.PoolID,
			IPAddress:     lease.IPAddress,
			Identifier:    lease.HardwareAddr,
			ClientID:      lease.ClientID,
			State:         lease.State,
			SecurityState: lease.SecurityState,
			UpdatedAt:     lease.UpdatedAt,
			ExpiresAt:     lease.ExpiresAt,
			CooldownUntil: lease.CooldownUntil,
		})
	}
	return items
}
