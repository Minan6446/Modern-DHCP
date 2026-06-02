package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

const maxStaticBindingSamples = 25
const maxStaticBindingScan = 5000

type staticBindingSummaryResponse struct {
	Total          int                           `json:"total"`
	Active         int                           `json:"active"`
	Reserved       int                           `json:"reserved"`
	PendingImports int                           `json:"pendingImports"`
	ByType         map[string]int                `json:"byType"`
	Bindings       []staticBindingRecordResponse `json:"bindings"`
}

type staticBindingRecordResponse struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenantId"`
	Identifier     string `json:"identifier"`
	IdentifierType string `json:"identifierType"`
	Device         string `json:"device"`
	MacAddress     string `json:"macAddress"`
	IPAddress      string `json:"ipAddress"`
	Hostname       string `json:"hostname,omitempty"`
	Status         string `json:"status"`
	LastSeenAt     string `json:"lastSeenAt,omitempty"`
}

type staticBindingMetadata struct {
	Device        string
	Hostname      string
	MACAddress    string
	Status        string
	LastSeenAt    string
	PendingImport bool
}

func (s *HTTPServer) mountNetworkRoutes(apiGroup *echo.Group) {
	if apiGroup == nil {
		return
	}
	network := apiGroup.Group("/network")
	if s.options.RequireAuth {
		network.Use(RequireRole(RoleReader))
	}
	network.GET("/static-bindings/summary", s.handleStaticBindingSummary(), RequireCapability(CapabilityBindingRead))
}

func (s *HTTPServer) handleStaticBindingSummary() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		ctx := c.Request().Context()
		tenantParam := strings.TrimSpace(c.QueryParam("tenantId"))
		limit := maxStaticBindingSamples
		if rawLimit := strings.TrimSpace(c.QueryParam("limit")); rawLimit != "" {
			if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > maxStaticBindingScan {
			limit = maxStaticBindingScan
		}

		tenantIDs := determineTenantScope(c, s, ctx, tenantParam)
		collector := newBindingSummaryCollector(limit)

		baseScope := s.poolScopeRef(c)
		for _, tenantID := range tenantIDs {
			scopeRef := baseScope.WithTenantOverride(tenantID)
			if err := s.collectStaticBindings(ctx, scopeRef, collector); err != nil {
				logger := LoggerFromContext(ctx, s.logger)
				if logger != nil {
					logger.Warn("static binding summary query failed", zap.String("tenantId", tenantID), zap.Error(err))
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}

		summary := staticBindingSummaryResponse{
			Total:          collector.total,
			Active:         collector.statusCounts["online"],
			Reserved:       collector.reserved(),
			PendingImports: collector.pendingImports,
			ByType:         collector.typeCounts,
			Bindings:       collector.records,
		}

		sort.Slice(summary.Bindings, func(i, j int) bool {
			return compareBindingRecords(summary.Bindings[i], summary.Bindings[j])
		})

		return c.JSON(http.StatusOK, summary)
	}
}

func newStaticBindingRecord(binding models.StaticBinding) (staticBindingRecordResponse, bool) {
	meta := decodeStaticBindingMetadata(binding.Metadata)

	mac := meta.MACAddress
	if mac == "" && strings.EqualFold(binding.IdentifierType, "mac") {
		mac = binding.Identifier
	}
	mac = normalizeBindingMAC(mac)

	pending := meta.PendingImport
	status := normalizeBindingStatus(meta.Status)
	if status == "" {
		if binding.Status != "" {
			status = normalizeBindingStatus(binding.Status)
		}
		if status == "" {
			if pending {
				status = "pending"
			} else {
				status = inferBindingStatusFromTimestamps(binding)
			}
		}
	}

	hostname := strings.TrimSpace(meta.Hostname)
	lastSeen := strings.TrimSpace(meta.LastSeenAt)
	if lastSeen == "" && binding.LastSeenAt != nil && !binding.LastSeenAt.IsZero() {
		lastSeen = binding.LastSeenAt.UTC().Format(time.RFC3339)
	}
	if lastSeen == "" && !binding.UpdatedAt.IsZero() {
		lastSeen = binding.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if status != "warning" && status != "pending" && binding.LastSeenAt != nil && !binding.LastSeenAt.IsZero() {
		if time.Since(binding.LastSeenAt.UTC()) > bindingStatusGrace {
			status = "offline"
		} else {
			status = "online"
		}
	}

	device := firstNonEmpty(meta.Device, hostname, binding.Identifier, binding.ID)
	device = strings.TrimSpace(device)
	if device == "" {
		device = "static-binding"
	}

	record := staticBindingRecordResponse{
		ID:             binding.ID,
		TenantID:       strings.TrimSpace(binding.TenantID),
		Identifier:     strings.TrimSpace(binding.Identifier),
		IdentifierType: normalizeBindingType(binding.IdentifierType),
		Device:         device,
		MacAddress:     mac,
		IPAddress:      strings.TrimSpace(binding.IPAddress),
		Hostname:       hostname,
		Status:         status,
		LastSeenAt:     lastSeen,
	}

	if record.IPAddress == "" {
		record.IPAddress = "unknown"
	}
	if record.Status == "" {
		record.Status = "online"
	}

	return record, pending || record.Status == "pending"
}

func decodeStaticBindingMetadata(raw []byte) staticBindingMetadata {
	if len(raw) == 0 {
		return staticBindingMetadata{}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return staticBindingMetadata{}
	}
	meta := staticBindingMetadata{}
	meta.Device = firstNonEmpty(
		mapString(data, "device"),
		mapString(data, "deviceName"),
		mapString(data, "name"),
	)
	meta.Hostname = firstNonEmpty(
		mapString(data, "hostname"),
		mapString(data, "host"),
		mapString(data, "fqdn"),
	)
	meta.MACAddress = firstNonEmpty(
		mapString(data, "macAddress"),
		mapString(data, "mac"),
		mapString(data, "hardwareAddress"),
	)
	meta.Status = mapString(data, "status")
	meta.LastSeenAt = firstNonEmpty(
		mapString(data, "lastSeenAt"),
		mapString(data, "lastSeen"),
		mapString(data, "updatedAt"),
	)
	meta.PendingImport = mapBool(data, "pendingImport", "pending", "importInProgress")
	return meta
}

func mapString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	if value, ok := data[key]; ok {
		switch v := value.(type) {
		case string:
			return strings.TrimSpace(v)
		case fmt.Stringer:
			return strings.TrimSpace(v.String())
		case float64:
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		case int64, int32, int:
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		case bool:
			if v {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

func mapBool(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		if data == nil {
			continue
		}
		value, ok := data[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v
		case string:
			normalized := strings.ToLower(strings.TrimSpace(v))
			if normalized == "true" || normalized == "1" || normalized == "yes" {
				return true
			}
		case float64:
			return v != 0
		case int, int32, int64:
			return fmt.Sprintf("%v", v) != "0"
		}
	}
	return false
}

func normalizeBindingStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown":
		return ""
	case "online", "active", "up", "healthy":
		return "online"
	case "offline", "inactive", "down", "unreachable":
		return "offline"
	case "pending", "importing", "queued":
		return "pending"
	default:
		return ""
	}
}

func inferBindingStatusFromTimestamps(binding models.StaticBinding) string {
	if binding.UpdatedAt.IsZero() {
		return "online"
	}
	if time.Since(binding.UpdatedAt) > 48*time.Hour {
		return "offline"
	}
	return "online"
}

func normalizeBindingMAC(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(":", "", "-", "", ".", "")
	cleaned := replacer.Replace(trimmed)
	if len(cleaned) != 12 {
		return trimmed
	}
	var builder strings.Builder
	for i := 0; i < len(cleaned); i += 2 {
		if builder.Len() > 0 {
			builder.WriteByte(':')
		}
		builder.WriteString(cleaned[i : i+2])
	}
	return builder.String()
}

func normalizeBindingType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "mac", "hwaddr", "ethernet", "hardware":
		return "mac"
	case "duid", "client-id", "dhcpv6":
		return "duid"
	case "circuit-id", "relay", "option82":
		return "circuit-id"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

type bindingSummaryCollector struct {
	limit          int
	total          int
	pendingImports int
	statusCounts   map[string]int
	typeCounts     map[string]int
	records        []staticBindingRecordResponse
}

func newBindingSummaryCollector(limit int) *bindingSummaryCollector {
	if limit <= 0 {
		limit = maxStaticBindingSamples
	}
	return &bindingSummaryCollector{
		limit:        limit,
		statusCounts: map[string]int{"online": 0, "offline": 0, "pending": 0},
		typeCounts:   make(map[string]int),
		records:      make([]staticBindingRecordResponse, 0, limit),
	}
}

func (c *bindingSummaryCollector) add(binding models.StaticBinding) {
	record, pending := newStaticBindingRecord(binding)
	status := record.Status
	if status == "" {
		status = "online"
	}
	status = strings.ToLower(status)
	if _, ok := c.statusCounts[status]; !ok {
		c.statusCounts[status] = 0
	}
	c.statusCounts[status]++
	if pending {
		c.pendingImports++
	}
	typeKey := record.IdentifierType
	if typeKey == "" {
		typeKey = "unknown"
	}
	c.typeCounts[typeKey]++
	c.total++
	if len(c.records) < c.limit {
		c.records = append(c.records, record)
	}
}

func (c *bindingSummaryCollector) reserved() int {
	reserved := c.total - c.statusCounts["online"]
	if reserved < 0 {
		return 0
	}
	return reserved
}

func (s *HTTPServer) collectStaticBindings(ctx context.Context, scopeRef pool.ResourceScope, collector *bindingSummaryCollector) error {
	if collector == nil {
		return nil
	}
	pageSize := 200
	offset := 0
	processed := 0
	for {
		batch, err := s.poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, pageSize, offset)
		if err != nil {
			return err
		}
		for _, binding := range batch {
			collector.add(binding)
		}
		processed += len(batch)
		if len(batch) < pageSize || processed >= maxStaticBindingScan {
			break
		}
		offset += len(batch)
	}
	return nil
}

func determineTenantScope(c echo.Context, s *HTTPServer, ctx context.Context, tenantParam string) []string {
	scope := []string{}
	switch {
	case strings.EqualFold(tenantParam, "all"):
		principal := s.principalFromContext(c)
		scope = s.resolvePrincipalTenants(ctx, principal)
	case tenantParam != "":
		scope = []string{tenantParam}
	default:
		if tenant := strings.TrimSpace(s.poolScopeRef(c).TenantOrDefault()); tenant != "" {
			scope = []string{tenant}
		} else {
			principal := s.principalFromContext(c)
			scope = s.resolvePrincipalTenants(ctx, principal)
			if len(scope) == 0 {
				scope = []string{systemTenantID}
			}
		}
	}
	return scope
}

func compareBindingRecords(a, b staticBindingRecordResponse) bool {
	ta := parseBindingTimestamp(a.LastSeenAt)
	tb := parseBindingTimestamp(b.LastSeenAt)
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	if a.Device == b.Device {
		return a.ID < b.ID
	}
	return a.Device < b.Device
}

func parseBindingTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts
	}
	return time.Time{}
}
