package server

import (
	"archive/zip"
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"net/smtp"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	otelecho "go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/automation"
	approvals "modern-dhcp/internal/automation/approvals"
	workflowsvc "modern-dhcp/internal/automation/workflow"
	"modern-dhcp/internal/collab"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/dashboard"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/ha"
	iotstore "modern-dhcp/internal/iot"
	iotregistry "modern-dhcp/internal/iot/registry"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/notifications"
	"modern-dhcp/internal/ops"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/rbac"
	"modern-dhcp/internal/reporting"
	"modern-dhcp/internal/resource"
	"modern-dhcp/internal/scopeutil"
	"modern-dhcp/internal/security/maclist"
	securitypolicy "modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/superadmin"
	"modern-dhcp/internal/tenant"
	"modern-dhcp/internal/visualization"
	"modern-dhcp/pkg/auditpayload"
	"modern-dhcp/pkg/models"
)

var statusStreamUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	systemTenantID        = "global"
	defaultAccessScopeID  = "global"
	defaultLeaseProfileID = "default-lease-profile"
)

const (
	headerResourceScope = "X-Resource-Scope"
	headerAccessGroups  = "X-Access-Groups"
	headerAccessLabels  = "X-Access-Labels"
)

func (s *HTTPServer) requestResourceScope(c echo.Context) string {
	return defaultAccessScopeID
}

func (s *HTTPServer) legacyTenantFromScope(scope string) string {
	return systemTenantID
}

func (s *HTTPServer) resourceScopeContext(c echo.Context) (string, string) {
	scope := s.accessScopeFromContext(c)
	return scope.ScopeID, scope.EffectiveTenant()
}

func (s *HTTPServer) poolScopeRef(c echo.Context) pool.ResourceScope {
	return pool.ResourceScopeFromAccess(s.accessScopeFromContext(c))
}

func (s *HTTPServer) leaseScopeRef(c echo.Context) lease.ResourceScope {
	return lease.ResourceScopeFromAccess(s.accessScopeFromContext(c))
}

// hydrateCustomOptions loads persisted custom options into the in-memory store on startup.
func (s *HTTPServer) hydrateCustomOptions() {
	if s == nil || s.optionRepo == nil || s.optionStore == nil {
		return
	}
	ctx := context.Background()
	logger := LoggerFromContext(ctx, s.logger)
	items, err := s.optionRepo.List(ctx, "")
	if err != nil {
		if logger != nil {
			logger.Warn("hydrate custom options", zap.Error(err))
		}
		return
	}
	for _, item := range items {
		s.optionStore.upsert(item)
	}
}

// hydrateTemplateConfigs loads persisted DHCP option templates into memory on startup.
func (s *HTTPServer) hydrateTemplateConfigs() {
	if s == nil || s.templateRepo == nil || s.templateStore == nil {
		return
	}
	ctx := context.Background()
	logger := LoggerFromContext(ctx, s.logger)
	items, err := s.templateRepo.List(ctx, "")
	if err != nil {
		if logger != nil {
			logger.Warn("hydrate dhcp templates", zap.Error(err))
		}
		return
	}
	for _, item := range items {
		s.templateStore.upsert(item)
	}
}

// hydrateOptionScopes loads persisted DHCP option scopes into memory on startup.
func (s *HTTPServer) hydrateOptionScopes() {
	if s == nil || s.scopeRepo == nil || s.scopeStore == nil {
		return
	}
	ctx := context.Background()
	logger := LoggerFromContext(ctx, s.logger)
	items, err := s.scopeRepo.List(ctx, "")
	if err != nil {
		if logger != nil {
			logger.Warn("hydrate dhcp option scopes", zap.Error(err))
		}
		return
	}
	for _, item := range items {
		s.scopeStore.upsert(item)
	}
}

func (s *HTTPServer) accessScopeFromContext(c echo.Context) resource.AccessScope {
	if c == nil {
		return resource.AccessScope{}
	}
	if scope, ok := c.Get(contextAccessScopeKey).(resource.AccessScope); ok {
		return scope
	}
	scope := s.buildAccessScope(c)
	c.Set(contextAccessScopeKey, scope)
	setRequestContextValue(c, contextKeyTenantID, scope.EffectiveTenant())
	return scope
}

func (s *HTTPServer) buildAccessScope(c echo.Context) resource.AccessScope {
	principal := strings.TrimSpace(s.principalFromContext(c))
	if principal == "" {
		principal = strings.TrimSpace(s.actorFromContext(c))
	}
	if principal == "" {
		principal = "anonymous"
	}
	scopeID := strings.TrimSpace(s.requestResourceScope(c))
	if scopeID == "" {
		scopeID = defaultAccessScopeID
	}
	opts := make([]resource.AccessScopeOption, 0, 3)
	if tenant := strings.TrimSpace(s.legacyTenantFromScope(scopeID)); tenant != "" {
		opts = append(opts, resource.WithTenantID(tenant))
	}
	if groups := s.requestAccessGroups(c); len(groups) > 0 {
		opts = append(opts, resource.WithGroupIDs(groups...))
	}
	if labels := s.requestAccessLabels(c); len(labels) > 0 {
		opts = append(opts, resource.WithLabels(labels))
	}
	return resource.NewAccessScope(scopeID, principal, opts...)
}

func (s *HTTPServer) requestAccessGroups(c echo.Context) []string {
	if c == nil {
		return nil
	}
	var inputs []string
	if header := c.Request().Header.Get(headerAccessGroups); header != "" {
		inputs = append(inputs, strings.Split(header, ",")...)
	}
	if groups, ok := c.QueryParams()["group"]; ok {
		inputs = append(inputs, groups...)
	}
	return normalizeAccessList(inputs)
}

func (s *HTTPServer) requestAccessLabels(c echo.Context) map[string]string {
	if c == nil {
		return nil
	}
	var inputs []string
	if header := c.Request().Header.Get(headerAccessLabels); header != "" {
		inputs = append(inputs, strings.Split(header, ",")...)
	}
	if labels, ok := c.QueryParams()["label"]; ok {
		inputs = append(inputs, labels...)
	}
	return normalizeAccessLabels(inputs)
}

func normalizeAccessList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeAccessLabels(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	labels := make(map[string]string)
	for _, raw := range values {
		key, value := splitLabelDirective(raw)
		if key == "" {
			continue
		}
		labels[key] = value
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func splitLabelDirective(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	for _, sep := range []string{"=", ":"} {
		if idx := strings.Index(trimmed, sep); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			value := strings.TrimSpace(trimmed[idx+1:])
			if key == "" {
				return "", ""
			}
			return key, value
		}
	}
	return trimmed, ""
}

func (s *HTTPServer) principalContext(c echo.Context) rbac.PrincipalContext {
	if c == nil {
		return rbac.PrincipalContext{}
	}
	if ctx, ok := c.Get(contextPrincipalContextKey).(rbac.PrincipalContext); ok && ctx.UserID != "" {
		return ctx
	}
	scope := s.accessScopeFromContext(c)
	principalID := strings.TrimSpace(s.principalFromContext(c))
	if principalID == "" && scope.Principal != "" {
		principalID = scope.Principal
	}
	opts := make([]rbac.PrincipalContextOption, 0, 1)
	if credential, ok := c.Get(contextCredentialKey).(string); ok {
		if trimmed := strings.TrimSpace(credential); trimmed != "" {
			opts = append(opts, rbac.WithPrincipalSession(trimmed))
		}
	}
	principalCtx := rbac.NewPrincipalContext(principalID, scope, opts...)
	c.Set(contextPrincipalContextKey, principalCtx)
	return principalCtx
}

type dashboardStreamMessage struct {
	Type        string                    `json:"type"`
	GeneratedAt time.Time                 `json:"generatedAt"`
	TenantID    string                    `json:"tenantId,omitempty"`
	Snapshot    *dashboard.StreamSnapshot `json:"snapshot,omitempty"`
	Items       []dashboard.StreamEntry   `json:"items,omitempty"`
}

type streamSeenSet struct {
	order    []string
	lookup   map[string]struct{}
	capacity int
}

func newStreamSeenSet(capacity int) *streamSeenSet {
	if capacity <= 0 {
		capacity = 512
	}
	return &streamSeenSet{
		order:    make([]string, 0, capacity),
		lookup:   make(map[string]struct{}, capacity),
		capacity: capacity,
	}
}

func (s *streamSeenSet) seen(key string) bool {
	if s == nil {
		return false
	}
	_, ok := s.lookup[key]
	return ok
}

func (s *streamSeenSet) remember(key string) {
	if s == nil || key == "" {
		return
	}
	if _, ok := s.lookup[key]; ok {
		return
	}
	s.lookup[key] = struct{}{}
	s.order = append(s.order, key)
	if len(s.order) <= s.capacity {
		return
	}
	oldest := s.order[0]
	s.order = s.order[1:]
	delete(s.lookup, oldest)
}

func streamEntryKey(entry dashboard.StreamEntry) string {
	if entry.ID == "" {
		return string(entry.Type) + "@" + strconv.FormatInt(entry.OccurredAt.UnixNano(), 10)
	}
	return string(entry.Type) + ":" + entry.ID
}

type apiKeyResponse struct {
	ID           string     `json:"id"`
	DisplayName  string     `json:"displayName"`
	Role         string     `json:"role"`
	Capabilities []string   `json:"capabilities"`
	Description  string     `json:"description,omitempty"`
	IPWhitelist  []string   `json:"ipWhitelist,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	OwnerUserID  string     `json:"ownerUserId,omitempty"`
	CreatedBy    string     `json:"createdBy,omitempty"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
	Status       string     `json:"status"`
	Token        string     `json:"token,omitempty"`
}

type apiKeyCreateRequest struct {
	DisplayName  string   `json:"displayName"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description"`
	IPWhitelist  []string `json:"ipWhitelist"`
	ExpiresAt    string   `json:"expiresAt"`
	OwnerUserID  string   `json:"ownerUserId"`
}

type apiKeyUpdateRequest struct {
	DisplayName  *string   `json:"displayName"`
	Role         *string   `json:"role"`
	Capabilities *[]string `json:"capabilities"`
	Description  *string   `json:"description"`
	IPWhitelist  *[]string `json:"ipWhitelist"`
	ExpiresAt    *string   `json:"expiresAt"`
	Enabled      *bool     `json:"enabled"`
}

type sessionResponse struct {
	Token              string       `json:"token"`
	RefreshToken       string       `json:"refreshToken"`
	ExpiresAt          string       `json:"expiresAt"`
	Actor              sessionActor `json:"actor"`
	TenantID           string       `json:"tenantId"`
	AuthMethod         string       `json:"authMethod,omitempty"`
	MustChangePassword bool         `json:"mustChangePassword,omitempty"`
}

type sessionActor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

type credentialLoginResponse struct {
	Token              string `json:"token"`
	RefreshToken       string `json:"refreshToken"`
	ExpiresAt          string `json:"expiresAt"`
	PrincipalID        string `json:"principalId"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	TenantID           string `json:"tenantId"`
	MustChangePassword bool   `json:"mustChangePassword,omitempty"`
}

type sessionSnapshotPayload struct {
	Actor        string   `json:"actor"`
	PrincipalID  string   `json:"principalId"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	TenantID     string   `json:"tenantId"`
}

type sessionListItemResponse struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Username   string    `json:"username"`
	TenantID   string    `json:"tenantId"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	Status     string    `json:"status"`
	Current    bool      `json:"current"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type sessionListResponse struct {
	Items  []sessionListItemResponse `json:"items"`
	Total  int                       `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

type sessionMetadata struct {
	AuthMethod string `json:"authMethod"`
	ClientIP   string `json:"clientIp"`
	UserAgent  string `json:"userAgent"`
}

type loginGuardState struct {
	FailedUntilCaptcha int       `json:"failedUntilCaptcha"`
	LockUntil          time.Time `json:"lockUntil"`
}

type loginCaptchaChallenge struct {
	ID        string
	Username  string
	Code      string
	ExpiresAt time.Time
}

type passwordResetChallenge struct {
	ID        string
	UserID    string
	Username  string
	Email     string
	Code      string
	Verified  bool
	ExpiresAt time.Time
}

type rbacRoleRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Permissions  int      `json:"permissions"`
	Capabilities []string `json:"capabilities"`
}

type rbacRoleResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Permissions  int      `json:"permissions"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type systemUserResponse struct {
	ID                 string     `json:"id"`
	PrincipalID        string     `json:"principalId"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Email              string     `json:"email,omitempty"`
	Role               string     `json:"role"`
	Capabilities       []string   `json:"capabilities"`
	TenantID           string     `json:"tenantId"`
	Status             string     `json:"status"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type userListResponse struct {
	Items  []systemUserResponse `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

type userCreateResponse struct {
	User              systemUserResponse `json:"user"`
	TemporaryPassword string             `json:"temporaryPassword,omitempty"`
}

func newAPIKeyResponse(key auth.APIKey) apiKeyResponse {
	resp := apiKeyResponse{
		ID:           key.ID,
		DisplayName:  key.Name,
		Role:         normalizeRole(key.Role),
		Capabilities: apiKeyCapabilitiesFromMetadata(key),
		IPWhitelist:  apiKeyIPWhitelistFromMetadata(key),
		CreatedAt:    key.CreatedAt.UTC(),
		CreatedBy:    strings.TrimSpace(key.CreatedBy),
	}
	if key.Description.Valid {
		resp.Description = strings.TrimSpace(key.Description.String)
	}
	if key.LastUsedAt.Valid {
		ts := key.LastUsedAt.Time.UTC()
		resp.LastUsedAt = &ts
	}
	if key.ExpiresAt.Valid {
		ts := key.ExpiresAt.Time.UTC()
		resp.ExpiresAt = &ts
	}
	if key.OwnerUserID.Valid {
		resp.OwnerUserID = strings.TrimSpace(key.OwnerUserID.String)
	}
	if key.RevokedAt.Valid {
		ts := key.RevokedAt.Time.UTC()
		resp.RevokedAt = &ts
	}
	now := time.Now().UTC()
	switch {
	case key.RevokedAt.Valid:
		resp.Status = "revoked"
	case key.ExpiresAt.Valid && now.After(key.ExpiresAt.Time):
		resp.Status = "expired"
	default:
		resp.Status = "active"
	}
	return resp
}

func apiKeyCapabilitiesFromMetadata(key auth.APIKey) []string {
	if len(key.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(key.Metadata, &meta); err == nil {
			if raw, ok := meta["capabilities"]; ok {
				switch arr := raw.(type) {
				case []any:
					caps := make([]string, 0, len(arr))
					for _, entry := range arr {
						if text, ok := entry.(string); ok {
							trimmed := strings.TrimSpace(text)
							if trimmed != "" {
								caps = append(caps, trimmed)
							}
						}
					}
					if len(caps) > 0 {
						sort.Strings(caps)
						return caps
					}
				case []string:
					caps := make([]string, 0, len(arr))
					for _, entry := range arr {
						trimmed := strings.TrimSpace(entry)
						if trimmed != "" {
							caps = append(caps, trimmed)
						}
					}
					if len(caps) > 0 {
						sort.Strings(caps)
						return caps
					}
				}
			}
		}
	}
	base := capabilitiesForRole(key.Role)
	clone := append([]string(nil), base...)
	sort.Strings(clone)
	return clone
}

func apiKeyIPWhitelistFromMetadata(key auth.APIKey) []string {
	if len(key.Metadata) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(key.Metadata, &meta); err != nil {
		return nil
	}
	raw, ok := meta["ipWhitelist"]
	if !ok {
		return nil
	}
	result := make([]string, 0)
	switch arr := raw.(type) {
	case []any:
		for _, entry := range arr {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	case []string:
		for _, entry := range arr {
			trimmed := strings.TrimSpace(entry)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func newSystemUserResponse(user auth.User) systemUserResponse {
	role := normalizeRole(user.Role)
	resp := systemUserResponse{
		ID:                 user.ID,
		PrincipalID:        user.PrincipalID(),
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Role:               role,
		Capabilities:       capabilitiesForRole(role),
		Status:             strings.ToLower(strings.TrimSpace(user.Status)),
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          user.CreatedAt.UTC(),
		UpdatedAt:          user.UpdatedAt.UTC(),
	}
	if user.Email.Valid {
		resp.Email = strings.TrimSpace(user.Email.String)
	}
	if user.LastLoginAt.Valid {
		ts := user.LastLoginAt.Time.UTC()
		resp.LastLoginAt = &ts
	}
	return resp
}

func parseSessionMetadata(payload json.RawMessage) sessionMetadata {
	if len(payload) == 0 {
		return sessionMetadata{}
	}
	var meta sessionMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return sessionMetadata{}
	}
	meta.AuthMethod = strings.TrimSpace(meta.AuthMethod)
	meta.ClientIP = strings.TrimSpace(meta.ClientIP)
	meta.UserAgent = strings.TrimSpace(meta.UserAgent)
	return meta
}

func sessionUsernameFromKey(key auth.APIKey) string {
	name := strings.TrimSpace(key.Name)
	if strings.HasPrefix(name, "session:") {
		value := strings.TrimSpace(strings.TrimPrefix(name, "session:"))
		if value != "" {
			return value
		}
	}
	if createdBy := strings.TrimSpace(key.CreatedBy); createdBy != "" {
		return createdBy
	}
	if principal := strings.TrimSpace(key.PrincipalID); principal != "" {
		return principal
	}
	return "unknown"
}

func sessionTenantFromKey(key auth.APIKey) string {
	if key.TenantScope.Valid {
		tenantID := strings.TrimSpace(key.TenantScope.String)
		if tenantID != "" {
			return tenantID
		}
	}
	return "global"
}

func sessionUserIDFromKey(key auth.APIKey) string {
	if key.OwnerUserID.Valid {
		id := strings.TrimSpace(key.OwnerUserID.String)
		if id != "" {
			return id
		}
	}
	if id := userIDFromPrincipal(key.PrincipalID); id != "" {
		return id
	}
	return strings.TrimSpace(key.PrincipalID)
}

func sessionListItemFromKey(key auth.APIKey) sessionListItemResponse {
	meta := parseSessionMetadata(key.Metadata)
	createdAt := key.CreatedAt.UTC()
	lastSeenAt := createdAt
	if key.LastUsedAt.Valid {
		lastSeenAt = key.LastUsedAt.Time.UTC()
	}
	ip := "-"
	if meta.ClientIP != "" {
		ip = meta.ClientIP
	}
	userAgent := "-"
	if meta.UserAgent != "" {
		userAgent = meta.UserAgent
	}
	return sessionListItemResponse{
		ID:         key.ID,
		UserID:     sessionUserIDFromKey(key),
		Username:   sessionUsernameFromKey(key),
		TenantID:   sessionTenantFromKey(key),
		IP:         ip,
		UserAgent:  userAgent,
		Status:     "active",
		Current:    false,
		CreatedAt:  createdAt,
		LastSeenAt: lastSeenAt,
	}
}

func sessionStatus(lastSeenAt time.Time, now time.Time, isCurrent bool, hasLastSeen bool) string {
	if isCurrent {
		return "active"
	}
	if !hasLastSeen {
		return "stale"
	}
	if now.Sub(lastSeenAt) > 30*time.Minute {
		return "stale"
	}
	return "active"
}

func hashCredentialToken(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func sanitizeRequestedCapabilities(role string, requested []string) []string {
	if len(requested) == 0 {
		base := capabilitiesForRole(role)
		clone := append([]string(nil), base...)
		sort.Strings(clone)
		return clone
	}
	allowed := make(map[string]struct{})
	for _, capName := range capabilitiesForRole(role) {
		allowed[capName] = struct{}{}
	}
	uniq := make(map[string]struct{}, len(requested))
	result := make([]string, 0, len(requested))
	for _, capName := range requested {
		name := strings.TrimSpace(capName)
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			continue
		}
		if _, exists := uniq[name]; exists {
			continue
		}
		uniq[name] = struct{}{}
		result = append(result, name)
	}
	if len(result) == 0 {
		return sanitizeRequestedCapabilities(role, nil)
	}
	sort.Strings(result)
	return result
}

func userIDFromPrincipal(principal string) string {
	trimmed := strings.TrimSpace(principal)
	if strings.HasPrefix(trimmed, "user:") && len(trimmed) > len("user:") {
		return strings.TrimSpace(trimmed[len("user:"):])
	}
	return ""
}

const (
	loginCaptchaThreshold = 3
	loginLockThreshold    = 5
	loginLockDuration     = 10 * time.Minute
	loginCaptchaTTL       = 2 * time.Minute
	passwordResetTTL      = 10 * time.Minute
)

func loginGuardKey(username, clientIP string) string {
	user := strings.ToLower(strings.TrimSpace(username))
	ip := strings.TrimSpace(clientIP)
	if ip == "" {
		ip = "unknown"
	}
	if user == "" {
		user = "anonymous"
	}
	return user + "|" + ip
}

func randomCaptchaCode(length int) string {
	if length <= 0 {
		length = 5
	}
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, length)
	if _, err := crand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = byte((time.Now().UnixNano() + int64(i)) % int64(len(chars)))
		}
	}
	result := make([]byte, length)
	for i := range buf {
		result[i] = chars[int(buf[i])%len(chars)]
	}
	return string(result)
}

func (s *HTTPServer) pruneExpiredCaptcha(now time.Time) {
	for key, item := range s.loginCaptcha {
		if now.After(item.ExpiresAt) {
			delete(s.loginCaptcha, key)
		}
	}
}

func (s *HTTPServer) issueCaptcha(username string) loginCaptchaChallenge {
	now := time.Now().UTC()
	s.loginGuardMu.Lock()
	defer s.loginGuardMu.Unlock()
	s.pruneExpiredCaptcha(now)
	id := uuid.NewString()
	challenge := loginCaptchaChallenge{
		ID:        id,
		Username:  strings.TrimSpace(strings.ToLower(username)),
		Code:      randomCaptchaCode(5),
		ExpiresAt: now.Add(loginCaptchaTTL),
	}
	s.loginCaptcha[id] = challenge
	return challenge
}

func (s *HTTPServer) verifyCaptcha(username, captchaID, captchaCode string) bool {
	id := strings.TrimSpace(captchaID)
	code := strings.ToUpper(strings.TrimSpace(captchaCode))
	user := strings.TrimSpace(strings.ToLower(username))
	if id == "" || code == "" {
		return false
	}
	now := time.Now().UTC()
	s.loginGuardMu.Lock()
	defer s.loginGuardMu.Unlock()
	challenge, ok := s.loginCaptcha[id]
	if !ok {
		return false
	}
	if now.After(challenge.ExpiresAt) {
		delete(s.loginCaptcha, id)
		return false
	}
	if challenge.Username != "" && challenge.Username != user {
		return false
	}
	valid := strings.EqualFold(challenge.Code, code)
	if valid {
		delete(s.loginCaptcha, id)
	}
	return valid
}

func (s *HTTPServer) loginGuardSnapshot(key string) loginGuardState {
	now := time.Now().UTC()
	s.loginGuardMu.Lock()
	defer s.loginGuardMu.Unlock()
	state := s.loginGuards[key]
	if !state.LockUntil.IsZero() && now.After(state.LockUntil) {
		state.LockUntil = time.Time{}
		state.FailedUntilCaptcha = 0
		s.loginGuards[key] = state
	}
	return state
}

func (s *HTTPServer) recordLoginFailure(key string) loginGuardState {
	now := time.Now().UTC()
	s.loginGuardMu.Lock()
	defer s.loginGuardMu.Unlock()
	state := s.loginGuards[key]
	if !state.LockUntil.IsZero() && now.After(state.LockUntil) {
		state.LockUntil = time.Time{}
		state.FailedUntilCaptcha = 0
	}
	state.FailedUntilCaptcha++
	if state.FailedUntilCaptcha >= loginLockThreshold {
		state.LockUntil = now.Add(loginLockDuration)
	}
	s.loginGuards[key] = state
	return state
}

func (s *HTTPServer) clearLoginGuard(key string) {
	s.loginGuardMu.Lock()
	defer s.loginGuardMu.Unlock()
	delete(s.loginGuards, key)
}

func (s *HTTPServer) handleLoginCaptcha() echo.HandlerFunc {
	return func(c echo.Context) error {
		username := strings.TrimSpace(c.QueryParam("username"))
		challenge := s.issueCaptcha(username)
		return c.JSON(http.StatusOK, map[string]any{
			"captchaId":   challenge.ID,
			"captchaCode": challenge.Code,
			"expiresAt":   challenge.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func (s *HTTPServer) pruneExpiredPasswordReset(now time.Time) {
	for id, item := range s.passwordResetChallenges {
		if now.After(item.ExpiresAt) {
			delete(s.passwordResetChallenges, id)
		}
	}
}

func (s *HTTPServer) issuePasswordReset(user auth.User, email string) passwordResetChallenge {
	now := time.Now().UTC()
	s.passwordResetMu.Lock()
	defer s.passwordResetMu.Unlock()
	s.pruneExpiredPasswordReset(now)
	challenge := passwordResetChallenge{
		ID:        uuid.NewString(),
		UserID:    strings.TrimSpace(user.ID),
		Username:  strings.TrimSpace(strings.ToLower(user.Username)),
		Email:     strings.TrimSpace(strings.ToLower(email)),
		Code:      fmt.Sprintf("%06d", 100000+int(now.UnixNano()%900000)),
		Verified:  false,
		ExpiresAt: now.Add(passwordResetTTL),
	}
	s.passwordResetChallenges[challenge.ID] = challenge
	return challenge
}

func (s *HTTPServer) handlePasswordResetStart() echo.HandlerFunc {
	type request struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		username := strings.TrimSpace(payload.Username)
		email := strings.TrimSpace(strings.ToLower(payload.Email))
		if username == "" || email == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "username and email are required")
		}
		user, err := s.authService.FindUserByUsername(c.Request().Context(), username)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "account verification failed")
		}
		storedEmail := strings.TrimSpace(strings.ToLower(user.Email.String))
		if storedEmail == "" || storedEmail != email {
			return echo.NewHTTPError(http.StatusBadRequest, "account verification failed")
		}
		challenge := s.issuePasswordReset(user, email)
		if err := s.sendPasswordResetCodeMail(c.Request().Context(), challenge); err != nil {
			s.clearPasswordResetChallenge(challenge.ID)
			logger := LoggerFromContext(c.Request().Context(), s.logger)
			if logger != nil {
				logger.Warn("password reset mail send failed", zap.String("username", challenge.Username), zap.String("email", challenge.Email), zap.Error(err))
			}
			return echo.NewHTTPError(http.StatusBadGateway, "failed to send verification code")
		}
		return c.JSON(http.StatusOK, map[string]any{
			"challengeId": challenge.ID,
			"expiresAt":   challenge.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func (s *HTTPServer) clearPasswordResetChallenge(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.passwordResetMu.Lock()
	defer s.passwordResetMu.Unlock()
	delete(s.passwordResetChallenges, id)
}

func (s *HTTPServer) sendPasswordResetCodeMail(ctx context.Context, challenge passwordResetChallenge) error {
	s.smtpMu.RLock()
	cfg := s.smtpConfig
	s.smtpMu.RUnlock()
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 || strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("smtp config not ready")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return fmt.Errorf("invalid smtp from address")
	}
	if _, err := mail.ParseAddress(challenge.Email); err != nil {
		return fmt.Errorf("invalid recipient address")
	}
	ttlMinutes := int(time.Until(challenge.ExpiresAt).Minutes())
	if ttlMinutes < 1 {
		ttlMinutes = 1
	}
	subject := "Modern DHCP 密码重置验证码"
	body := fmt.Sprintf("您正在执行 Modern DHCP 账号密码重置。\r\n\r\n验证码：%s\r\n有效期：%d 分钟\r\n账号：%s\r\n\r\n如果这不是您的操作，请忽略本邮件并联系管理员。\r\n", challenge.Code, ttlMinutes, challenge.Username)
	return s.sendSMTPMessage(ctx, cfg, challenge.Email, subject, body)
}

func (s *HTTPServer) handlePasswordResetVerify() echo.HandlerFunc {
	type request struct {
		ChallengeID string `json:"challengeId"`
		Code        string `json:"code"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		id := strings.TrimSpace(payload.ChallengeID)
		code := strings.TrimSpace(payload.Code)
		if id == "" || code == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "challengeId and code are required")
		}
		now := time.Now().UTC()
		s.passwordResetMu.Lock()
		defer s.passwordResetMu.Unlock()
		s.pruneExpiredPasswordReset(now)
		challenge, ok := s.passwordResetChallenges[id]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "verification challenge not found")
		}
		if !strings.EqualFold(challenge.Code, code) {
			return echo.NewHTTPError(http.StatusBadRequest, "verification code invalid")
		}
		challenge.Verified = true
		s.passwordResetChallenges[id] = challenge
		return c.JSON(http.StatusOK, map[string]any{
			"verified":    true,
			"challengeId": id,
		})
	}
}

func (s *HTTPServer) handlePasswordResetComplete() echo.HandlerFunc {
	type request struct {
		ChallengeID string `json:"challengeId"`
		NewPassword string `json:"newPassword"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		id := strings.TrimSpace(payload.ChallengeID)
		newPassword := strings.TrimSpace(payload.NewPassword)
		if id == "" || newPassword == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "challengeId and newPassword are required")
		}
		now := time.Now().UTC()
		s.passwordResetMu.Lock()
		s.pruneExpiredPasswordReset(now)
		challenge, ok := s.passwordResetChallenges[id]
		if !ok {
			s.passwordResetMu.Unlock()
			return echo.NewHTTPError(http.StatusBadRequest, "reset challenge expired")
		}
		if !challenge.Verified {
			s.passwordResetMu.Unlock()
			return echo.NewHTTPError(http.StatusBadRequest, "verification required")
		}
		delete(s.passwordResetChallenges, id)
		s.passwordResetMu.Unlock()

		if err := s.authService.UpdatePassword(c.Request().Context(), challenge.UserID, newPassword, false); err != nil {
			if errors.Is(err, auth.ErrPasswordTooShort) {
				return echo.NewHTTPError(http.StatusBadRequest, "password too short")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		s.recordAudit(c.Request().Context(), "global", challenge.Username, "auth.password.reset", map[string]any{
			"username": challenge.Username,
			"email":    challenge.Email,
			"result":   "success",
		}, withResource("auth_user"))
		return c.JSON(http.StatusOK, map[string]any{"success": true})
	}
}

func (s *HTTPServer) handleCredentialLogin() echo.HandlerFunc {
	type request struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		TenantID    string `json:"tenantId"`
		CaptchaID   string `json:"captchaId"`
		CaptchaCode string `json:"captchaCode"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		username := strings.TrimSpace(payload.Username)
		password := strings.TrimSpace(payload.Password)
		if username == "" || password == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
		}
		ctx := c.Request().Context()
		clientIP := c.RealIP()
		userAgent := c.Request().UserAgent()
		guardKey := loginGuardKey(username, clientIP)
		guard := s.loginGuardSnapshot(guardKey)
		now := time.Now().UTC()

		if !guard.LockUntil.IsZero() && now.Before(guard.LockUntil) {
			remainingSeconds := int(guard.LockUntil.Sub(now).Seconds())
			if remainingSeconds < 1 {
				remainingSeconds = 1
			}
			s.recordAudit(ctx, strings.TrimSpace(payload.TenantID), username, "auth.login.abnormal", map[string]any{
				"tenantId":         strings.TrimSpace(payload.TenantID),
				"ip":               clientIP,
				"userAgent":        userAgent,
				"authMethod":       "password",
				"result":           "failed",
				"reason":           "account temporarily locked",
				"requiresCaptcha":  true,
				"failedAttempts":   guard.FailedUntilCaptcha,
				"lockUntil":        guard.LockUntil.UTC().Format(time.RFC3339),
				"remainingSeconds": remainingSeconds,
			}, withResource("auth_session"))
			return c.JSON(http.StatusLocked, map[string]any{
				"message":          "account temporarily locked",
				"requiresCaptcha":  true,
				"lockUntil":        guard.LockUntil.UTC().Format(time.RFC3339),
				"remainingSeconds": remainingSeconds,
			})
		}

		if guard.FailedUntilCaptcha >= loginCaptchaThreshold {
			if !s.verifyCaptcha(username, payload.CaptchaID, payload.CaptchaCode) {
				challenge := s.issueCaptcha(username)
				s.recordAudit(ctx, strings.TrimSpace(payload.TenantID), username, "auth.login.abnormal", map[string]any{
					"tenantId":        strings.TrimSpace(payload.TenantID),
					"ip":              clientIP,
					"userAgent":       userAgent,
					"authMethod":      "password",
					"result":          "failed",
					"reason":          "captcha validation failed",
					"requiresCaptcha": true,
					"failedAttempts":  guard.FailedUntilCaptcha,
				}, withResource("auth_session"))
				return c.JSON(http.StatusBadRequest, map[string]any{
					"message":          "captcha required",
					"requiresCaptcha":  true,
					"failedAttempts":   guard.FailedUntilCaptcha,
					"captchaId":        challenge.ID,
					"captchaCode":      challenge.Code,
					"captchaExpiresAt": challenge.ExpiresAt.UTC().Format(time.RFC3339),
				})
			}
		}

		replyInvalid := func(reason string) error {
			state := s.recordLoginFailure(guardKey)
			challenge := loginCaptchaChallenge{}
			if state.FailedUntilCaptcha >= loginCaptchaThreshold {
				challenge = s.issueCaptcha(username)
			}
			failurePayload := map[string]any{
				"message":         "invalid username or password",
				"requiresCaptcha": state.FailedUntilCaptcha >= loginCaptchaThreshold,
				"failedAttempts":  state.FailedUntilCaptcha,
			}
			if !state.LockUntil.IsZero() {
				failurePayload["lockUntil"] = state.LockUntil.UTC().Format(time.RFC3339)
			}
			if challenge.ID != "" {
				failurePayload["captchaId"] = challenge.ID
				failurePayload["captchaCode"] = challenge.Code
				failurePayload["captchaExpiresAt"] = challenge.ExpiresAt.UTC().Format(time.RFC3339)
			}
			s.recordAudit(ctx, strings.TrimSpace(payload.TenantID), username, "auth.login.failed", map[string]any{
				"tenantId":   strings.TrimSpace(payload.TenantID),
				"ip":         clientIP,
				"userAgent":  userAgent,
				"authMethod": "password",
				"result":     "failed",
				"reason":     reason,
			}, withResource("auth_session"))
			if state.FailedUntilCaptcha >= loginCaptchaThreshold || !state.LockUntil.IsZero() {
				s.recordAudit(ctx, strings.TrimSpace(payload.TenantID), username, "auth.login.abnormal", map[string]any{
					"tenantId":        strings.TrimSpace(payload.TenantID),
					"ip":              clientIP,
					"userAgent":       userAgent,
					"authMethod":      "password",
					"result":          "failed",
					"reason":          "repeated login failures",
					"requiresCaptcha": state.FailedUntilCaptcha >= loginCaptchaThreshold,
					"failedAttempts":  state.FailedUntilCaptcha,
					"lockUntil": func() string {
						if state.LockUntil.IsZero() {
							return ""
						}
						return state.LockUntil.UTC().Format(time.RFC3339)
					}(),
				}, withResource("auth_session"))
			}
			status := http.StatusUnauthorized
			if !state.LockUntil.IsZero() {
				status = http.StatusLocked
			}
			return c.JSON(status, failurePayload)
		}
		if s.superAdmin != nil && s.matchSuperAdminUsername(username) {
			user, session, tenantID, err := s.authenticateSuperAdmin(ctx, payload.TenantID, password, clientIP, userAgent)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidCredentials) {
					return replyInvalid("invalid username or password")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			s.clearLoginGuard(guardKey)
			resp := newCredentialLoginResponse(user, session, tenantID)
			csrf := setCSRFCookie(c)
			setAuthCookie(c, resp.Token, session.ExpiresAt)
			var previousLoginAt *string
			if user.LastLoginAt.Valid {
				ts := user.LastLoginAt.Time.UTC().Format(time.RFC3339)
				previousLoginAt = &ts
			}
			s.recordAudit(ctx, tenantID, user.Username, "auth.login", map[string]any{
				"principalId": resp.PrincipalID,
				"tenantId":    tenantID,
				"ip":          clientIP,
				"userAgent":   userAgent,
				"sessionId": func() string {
					token := strings.TrimSpace(session.Token)
					if len(token) > 16 {
						return token[:16]
					}
					return token
				}(),
				"authMethod": "superadmin",
				"result":     "success",
			}, withResource("auth_session"))
			return c.JSON(http.StatusOK, map[string]any{
				"token":        resp.Token,
				"refreshToken": resp.RefreshToken,
				"expiresAt":    resp.ExpiresAt,
				"profile": map[string]any{
					"id":          resp.PrincipalID,
					"displayName": resp.DisplayName,
					"role":        resp.Role,
					"tenantId":    resp.TenantID,
				},
				"csrfToken":     csrf,
				"lastLoginAt":   previousLoginAt,
				"abnormalLogin": false,
			})
		}
		user, err := s.authService.Authenticate(ctx, username, password)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				return replyInvalid("invalid username or password")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.clearLoginGuard(guardKey)
		var previousLoginAt *string
		if user.LastLoginAt.Valid {
			ts := user.LastLoginAt.Time.UTC().Format(time.RFC3339)
			previousLoginAt = &ts
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = "global"
		}
		session, err := s.authService.IssueSession(ctx, user, auth.SessionOptions{TenantID: tenantID, ClientIP: clientIP, UserAgent: userAgent})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := newCredentialLoginResponse(user, session, tenantID)
		csrf := setCSRFCookie(c)
		setAuthCookie(c, resp.Token, session.ExpiresAt)
		s.recordAudit(ctx, tenantID, user.Username, "auth.login", map[string]any{
			"principalId": resp.PrincipalID,
			"tenantId":    tenantID,
			"ip":          clientIP,
			"userAgent":   userAgent,
			"sessionId": func() string {
				token := strings.TrimSpace(session.Token)
				if len(token) > 16 {
					return token[:16]
				}
				return token
			}(),
			"authMethod": "password",
			"result":     "success",
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, map[string]any{
			"token":        resp.Token,
			"refreshToken": resp.RefreshToken,
			"expiresAt":    resp.ExpiresAt,
			"profile": map[string]any{
				"id":          resp.PrincipalID,
				"displayName": resp.DisplayName,
				"role":        resp.Role,
				"tenantId":    resp.TenantID,
			},
			"csrfToken":     csrf,
			"lastLoginAt":   previousLoginAt,
			"abnormalLogin": false,
		})
	}
}

func (s *HTTPServer) handleAPIKeyExchange() echo.HandlerFunc {
	type request struct {
		APIKey   string `json:"apiKey"`
		TenantID string `json:"tenantId"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		apiKey := strings.TrimSpace(payload.APIKey)
		if apiKey == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "apiKey is required")
		}
		ctx := c.Request().Context()
		meta, err := s.resolveAPIKeyMetadata(ctx, apiKey)
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) || errors.Is(err, auth.ErrAPIKeyRevoked) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid api key")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if strings.TrimSpace(meta.DisplayName) == "" {
			meta.DisplayName = apiKey
		}
		if strings.TrimSpace(meta.PrincipalID) == "" {
			meta.PrincipalID = apiKey
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = "default"
		}
		resp := credentialLoginResponse{
			Token:       apiKey,
			PrincipalID: meta.PrincipalID,
			DisplayName: meta.DisplayName,
			Role:        normalizeRole(meta.Role),
			TenantID:    tenantID,
		}
		s.recordAudit(ctx, tenantID, meta.DisplayName, "auth.api_key.exchange", map[string]any{
			"principalId": meta.PrincipalID,
			"ip":          c.RealIP(),
			"userAgent":   c.Request().UserAgent(),
			"authMethod":  "api_key",
			"result":      "success",
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleSessionSnapshot() echo.HandlerFunc {
	return func(c echo.Context) error {
		principal := s.principalFromContext(c)
		if principal == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "principal not resolved")
		}
		actor := s.actorFromContext(c)
		role := s.roleFromContext(c)
		caps := capabilitySliceFromContext(c)
		snapshot := sessionSnapshotPayload{
			Actor:        actor,
			PrincipalID:  principal,
			Role:         role,
			Capabilities: caps,
			TenantID:     systemTenantID,
		}
		return c.JSON(http.StatusOK, snapshot)
	}
}

func (s *HTTPServer) handleListSessions() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "session management is disabled")
		}
		limit := 50
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "limit must be a positive integer")
			}
			if value > 200 {
				value = 200
			}
			limit = value
		}
		offset := 0
		if raw := strings.TrimSpace(c.QueryParam("offset")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "offset must be a non-negative integer")
			}
			offset = value
		}
		query := strings.ToLower(strings.TrimSpace(c.QueryParam("q")))
		if query == "" {
			query = strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		}
		status := strings.ToLower(strings.TrimSpace(c.QueryParam("status")))
		tenantFilter := strings.TrimSpace(c.QueryParam("tenantId"))
		currentHash := hashCredentialToken(s.credentialFromContext(c))

		keys, err := s.authService.ListAPIKeys(c.Request().Context(), auth.APIKeyFilter{Kinds: []string{auth.KeyKindSession}})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		now := time.Now().UTC()
		items := make([]sessionListItemResponse, 0, len(keys))
		for _, key := range keys {
			if !key.Valid(now) {
				continue
			}
			item := sessionListItemFromKey(key)
			item.Current = currentHash != "" && strings.EqualFold(currentHash, key.TokenHash)
			item.Status = sessionStatus(item.LastSeenAt, now, item.Current, key.LastUsedAt.Valid)
			if tenantFilter != "" && !strings.EqualFold(item.TenantID, tenantFilter) {
				continue
			}
			if status != "" && status != item.Status {
				continue
			}
			if query != "" {
				matched := strings.Contains(strings.ToLower(item.Username), query) ||
					strings.Contains(strings.ToLower(item.IP), query) ||
					strings.Contains(strings.ToLower(item.UserAgent), query) ||
					strings.Contains(strings.ToLower(item.TenantID), query)
				if !matched {
					continue
				}
			}
			items = append(items, item)
		}
		total := len(items)
		if offset >= total {
			return c.JSON(http.StatusOK, sessionListResponse{Items: []sessionListItemResponse{}, Total: total, Limit: limit, Offset: offset})
		}
		end := offset + limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, sessionListResponse{Items: items[offset:end], Total: total, Limit: limit, Offset: offset})
	}
}

func (s *HTTPServer) handleDeleteSession() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "session management is disabled")
		}
		sessionID := strings.TrimSpace(c.Param("sessionId"))
		if sessionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "sessionId is required")
		}
		ctx := c.Request().Context()
		if err := s.authService.RevokeAPIKey(ctx, sessionID, s.actorFromContext(c)); err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "session not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.session.revoke", map[string]any{
			"sessionId": sessionID,
		}, withResource("auth_session"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleCapabilities() echo.HandlerFunc {
	return func(c echo.Context) error {
		principal := s.principalFromContext(c)
		actor := s.actorFromContext(c)
		role := s.roleFromContext(c)
		caps := capabilitySliceFromContext(c)
		payload := struct {
			Actor        string   `json:"actor"`
			PrincipalID  string   `json:"principalId"`
			Role         string   `json:"role"`
			Capabilities []string `json:"capabilities"`
			TenantID     string   `json:"tenantId"`
		}{
			Actor:        actor,
			PrincipalID:  principal,
			Role:         role,
			Capabilities: caps,
			TenantID:     systemTenantID,
		}
		return c.JSON(http.StatusOK, payload)
	}
}

func (s *HTTPServer) handleRBACCapabilities() echo.HandlerFunc {
	return func(c echo.Context) error {
		payload := struct {
			Capabilities []string `json:"capabilities"`
		}{
			Capabilities: allCapabilities(),
		}
		return c.JSON(http.StatusOK, payload)
	}
}

func (s *HTTPServer) handleLogout() echo.HandlerFunc {
	return func(c echo.Context) error {
		token := s.credentialFromContext(c)
		ctx := c.Request().Context()
		if token != "" && s.authService != nil {
			if err := s.authService.RevokeToken(ctx, token, s.actorFromContext(c)); err != nil && !errors.Is(err, auth.ErrAPIKeyNotFound) {
				if !errors.Is(err, auth.ErrAPIKeyRevoked) {
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
			}
		}
		clearAuthCookie(c)
		clearCSRFCookie(c)
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.logout", map[string]any{
			"principalId": s.principalFromContext(c),
		}, withResource("auth_session"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handlePasswordChange() echo.HandlerFunc {
	type request struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		username := strings.TrimSpace(payload.Username)
		if username == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "username is required")
		}
		currentPassword := strings.TrimSpace(payload.CurrentPassword)
		newPassword := strings.TrimSpace(payload.NewPassword)
		if currentPassword == "" || newPassword == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "currentPassword and newPassword are required")
		}
		ctx := c.Request().Context()
		if s.superAdmin != nil && s.matchSuperAdminUsername(username) {
			if _, _, err := s.superAdmin.Authenticate(currentPassword); err != nil {
				if errors.Is(err, superadmin.ErrInvalidCredentials) {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			if err := s.superAdmin.UpdatePassword(currentPassword, newPassword); err != nil {
				switch {
				case errors.Is(err, superadmin.ErrInvalidCredentials):
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
				case errors.Is(err, superadmin.ErrWeakPassword), errors.Is(err, superadmin.ErrPasswordReuse):
					return echo.NewHTTPError(http.StatusBadRequest, err.Error())
				default:
					return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
				}
			}
			s.recordAudit(ctx, "default", s.actorFromContext(c), "auth.super_admin.password.change", map[string]any{
				"username": s.superAdminUsername,
			}, withResource("super_admin"))
			return c.NoContent(http.StatusNoContent)
		}
		user, err := s.authService.Authenticate(ctx, username, currentPassword)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		callerID := userIDFromPrincipal(s.principalFromContext(c))
		if callerID != "" && callerID != user.ID && !hasRequiredRole(s.roleFromContext(c), RoleAdmin) {
			return echo.NewHTTPError(http.StatusForbidden, "insufficient role to change another user's password")
		}
		if err := s.authService.UpdatePassword(ctx, user.ID, newPassword, false); err != nil {
			if errors.Is(err, auth.ErrPasswordTooShort) {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "user not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, "global", s.actorFromContext(c), "auth.password.change", map[string]any{
			"principalId": user.PrincipalID(),
		}, withResource("auth_user"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleListUsers() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		limit := 50
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "limit must be a positive integer")
			}
			if value > 200 {
				value = 200
			}
			limit = value
		}
		offset := 0
		if raw := strings.TrimSpace(c.QueryParam("offset")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "offset must be a non-negative integer")
			}
			offset = value
		}
		filter := auth.UserFilter{
			Limit:    limit,
			Offset:   offset,
			Status:   c.QueryParam("status"),
			Query:    c.QueryParam("q"),
			SortBy:   c.QueryParam("sort"),
			SortDesc: strings.EqualFold(c.QueryParam("order"), "desc"),
		}
		if tenant := strings.TrimSpace(c.QueryParam("tenantId")); tenant != "" {
			filter.TenantID = &tenant
		}
		ctx := c.Request().Context()
		users, total, err := s.authService.ListUsers(ctx, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items := make([]systemUserResponse, 0, len(users))
		for _, user := range users {
			items = append(items, newSystemUserResponse(user))
		}
		return c.JSON(http.StatusOK, userListResponse{
			Items:  items,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}

func (s *HTTPServer) handleCreateUser() echo.HandlerFunc {
	type request struct {
		Username           string `json:"username"`
		DisplayName        string `json:"displayName"`
		Email              string `json:"email"`
		Role               string `json:"role"`
		TenantID           string `json:"tenantId"`
		Status             string `json:"status"`
		Password           string `json:"password"`
		MustChangePassword *bool  `json:"mustChangePassword"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		user, tempPassword, err := s.authService.CreateUser(ctx, auth.CreateUserOptions{
			Username:           payload.Username,
			DisplayName:        payload.DisplayName,
			Email:              payload.Email,
			Role:               payload.Role,
			TenantID:           payload.TenantID,
			Status:             payload.Status,
			Password:           payload.Password,
			MustChangePassword: payload.MustChangePassword,
		})
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrPasswordTooShort):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			case errors.Is(err, auth.ErrUserExists):
				return echo.NewHTTPError(http.StatusConflict, err.Error())
			case errors.Is(err, auth.ErrInvalidCredentials):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		response := userCreateResponse{User: newSystemUserResponse(user)}
		if tempPassword != "" {
			response.TemporaryPassword = tempPassword
		}
		s.recordAudit(ctx, "global", s.actorFromContext(c), "auth.user.create", s.auditPayloadFromRequest(c, map[string]any{
			"userId":     user.ID,
			"username":   user.Username,
			"mustChange": user.MustChangePassword,
			"result":     "success",
		}), withResource("auth_user"))
		return c.JSON(http.StatusCreated, response)
	}
}

func (s *HTTPServer) handleListAuthProviders() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "auth providers are disabled")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		ctx := c.Request().Context()
		providers, err := s.authService.ListIdentityProviders(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		filtered := make([]auth.ManagedIdentityProvider, 0, len(providers))
		for _, provider := range providers {
			if keyword != "" {
				name := strings.ToLower(strings.TrimSpace(provider.Name))
				typ := strings.ToLower(strings.TrimSpace(provider.Type))
				if !strings.Contains(name, keyword) && !strings.Contains(typ, keyword) {
					continue
				}
			}
			filtered = append(filtered, provider)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, map[string]any{"items": []auth.IdentityProvider{}, "total": total, "limit": page.Limit, "offset": page.Offset})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, map[string]any{"items": filtered[page.Offset:end], "total": total, "limit": page.Limit, "offset": page.Offset})
	}
}

func (s *HTTPServer) handleListRoles() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		ctx := c.Request().Context()
		roles, err := s.rbacRepo.ListRoles(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		summaries := make([]rbacRoleResponse, 0, len(roles))
		for _, role := range roles {
			item := newRBACRoleResponse(role)
			if keyword != "" {
				name := strings.ToLower(strings.TrimSpace(item.Name))
				desc := strings.ToLower(strings.TrimSpace(item.Description))
				if !strings.Contains(name, keyword) && !strings.Contains(desc, keyword) {
					continue
				}
			}
			summaries = append(summaries, item)
		}
		sort.Slice(summaries, func(i, j int) bool {
			return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
		})
		total := len(summaries)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, map[string]any{"items": []rbacRoleResponse{}, "total": total, "limit": page.Limit, "offset": page.Offset})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, map[string]any{"items": summaries[page.Offset:end], "total": total, "limit": page.Limit, "offset": page.Offset})
	}
}

func (s *HTTPServer) handleCreateRole() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		var req rbacRoleRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "role name is required")
		}
		if len(name) > 64 {
			return echo.NewHTTPError(http.StatusBadRequest, "role name must be 64 characters or fewer")
		}
		if len(req.Capabilities) == 0 && req.Permissions < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "permissions must be non-negative")
		}
		ctx := c.Request().Context()
		if _, err := s.rbacRepo.GetRole(ctx, name); err == nil {
			return echo.NewHTTPError(http.StatusConflict, "role already exists")
		} else if !errors.Is(err, rbac.ErrRoleNotFound) {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		description := strings.TrimSpace(req.Description)
		now := time.Now().UTC()
		caps := sanitizeRequestedCapabilities(RoleAdmin, req.Capabilities)
		if len(caps) == 0 {
			caps = permissionsToCapabilities(req.Permissions)
		}
		role := rbac.Role{
			Name:         name,
			Description:  description,
			Capabilities: caps,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.rbacRepo.CreateRole(ctx, role); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "rbac.role.create", map[string]any{
			"roleName":    role.Name,
			"permissions": permissionsFromCapabilities(role.Capabilities),
		}, withResource("rbac_role"))
		return c.JSON(http.StatusCreated, newRBACRoleResponse(role))
	}
}

func (s *HTTPServer) handleUpdateRole() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		roleID := strings.TrimSpace(c.Param("roleId"))
		if roleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "roleId is required")
		}
		if isProtectedRole(roleID) {
			return echo.NewHTTPError(http.StatusForbidden, "built-in roles cannot be modified")
		}
		var req rbacRoleRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = roleID
		}
		if name != roleID {
			return echo.NewHTTPError(http.StatusBadRequest, "renaming roles is not supported")
		}
		if len(req.Capabilities) == 0 && req.Permissions < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "permissions must be non-negative")
		}
		description := strings.TrimSpace(req.Description)
		ctx := c.Request().Context()
		caps := sanitizeRequestedCapabilities(RoleAdmin, req.Capabilities)
		if len(caps) == 0 {
			caps = permissionsToCapabilities(req.Permissions)
		}
		role := rbac.Role{
			Name:         name,
			Description:  description,
			Capabilities: caps,
			UpdatedAt:    time.Now().UTC(),
		}
		if err := s.rbacRepo.UpdateRole(ctx, roleID, role); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "role not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "rbac.role.update", map[string]any{
			"roleName":    name,
			"permissions": permissionsFromCapabilities(role.Capabilities),
		}, withResource("rbac_role"))
		return c.JSON(http.StatusOK, newRBACRoleResponse(role))
	}
}

func (s *HTTPServer) handleDeleteRole() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		roleID := strings.TrimSpace(c.Param("roleId"))
		if roleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "roleId is required")
		}
		if isProtectedRole(roleID) {
			return echo.NewHTTPError(http.StatusForbidden, "built-in roles cannot be removed")
		}
		ctx := c.Request().Context()
		if err := s.rbacRepo.DeleteRole(ctx, roleID); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "role not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "rbac.role.delete", map[string]any{
			"roleName": roleID,
		}, withResource("rbac_role"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleCreateAuthProvider() echo.HandlerFunc {
	type request struct {
		ID       string         `json:"id"`
		Name     string         `json:"name"`
		Type     string         `json:"type"`
		Endpoint string         `json:"endpoint"`
		Config   map[string]any `json:"config"`
		Enabled  *bool          `json:"enabled"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "auth providers are disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		provider, err := s.authService.CreateIdentityProvider(ctx, auth.CreateIdentityProviderOptions{
			ID:       payload.ID,
			Name:     payload.Name,
			Type:     payload.Type,
			Endpoint: payload.Endpoint,
			Config:   payload.Config,
			Enabled:  payload.Enabled,
		})
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrIdentityProviderExists):
				return echo.NewHTTPError(http.StatusConflict, err.Error())
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.provider.create", map[string]any{
			"id":   provider.ID,
			"type": provider.Type,
		}, withResource("auth_provider"))
		return c.JSON(http.StatusCreated, provider)
	}
}

func (s *HTTPServer) handleUpdateAuthProvider() echo.HandlerFunc {
	type request struct {
		Name     *string        `json:"name"`
		Endpoint *string        `json:"endpoint"`
		Config   map[string]any `json:"config"`
		Enabled  *bool          `json:"enabled"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "auth providers are disabled")
		}
		providerID := strings.TrimSpace(c.Param("providerId"))
		if providerID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "providerId is required")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		opts := auth.UpdateIdentityProviderOptions{
			Name:     payload.Name,
			Endpoint: payload.Endpoint,
			Enabled:  payload.Enabled,
		}
		if payload.Config != nil {
			opts.Config = &payload.Config
		}
		ctx := c.Request().Context()
		provider, err := s.authService.UpdateIdentityProvider(ctx, providerID, opts)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrIdentityProviderNotFound):
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.provider.update", map[string]any{
			"id": provider.ID,
		}, withResource("auth_provider"))
		return c.JSON(http.StatusOK, provider)
	}
}

func (s *HTTPServer) handleUpdateUser() echo.HandlerFunc {
	type request struct {
		Username           *string `json:"username"`
		DisplayName        *string `json:"displayName"`
		Email              *string `json:"email"`
		Role               *string `json:"role"`
		TenantID           *string `json:"tenantId"`
		Status             *string `json:"status"`
		Password           *string `json:"password"`
		MustChangePassword *bool   `json:"mustChangePassword"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		userID := strings.TrimSpace(c.Param("userId"))
		if userID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "userId is required")
		}
		if _, err := uuid.Parse(userID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "userId must be a valid UUID")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		beforeUser, beforeErr := s.authService.GetUserByID(ctx, userID)
		if beforeErr != nil && !errors.Is(beforeErr, auth.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusInternalServerError, beforeErr.Error())
		}
		user, err := s.authService.UpdateUser(ctx, userID, auth.UpdateUserOptions{
			Username:           payload.Username,
			DisplayName:        payload.DisplayName,
			Email:              payload.Email,
			Role:               payload.Role,
			TenantID:           payload.TenantID,
			Status:             payload.Status,
			Password:           payload.Password,
			MustChangePassword: payload.MustChangePassword,
		})
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrUserNotFound):
				return echo.NewHTTPError(http.StatusNotFound, "user not found")
			case errors.Is(err, auth.ErrPasswordTooShort):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			case errors.Is(err, auth.ErrUserExists):
				return echo.NewHTTPError(http.StatusConflict, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		action := "auth.user.update"
		if beforeErr == nil {
			beforeStatus := strings.ToLower(strings.TrimSpace(beforeUser.Status))
			afterStatus := strings.ToLower(strings.TrimSpace(user.Status))
			if beforeStatus != afterStatus {
				switch afterStatus {
				case "disabled", "inactive", "blocked", "suspended":
					action = "auth.user.disable"
				case "active", "enabled":
					action = "auth.user.enable"
				}
			}
		}
		if action == "auth.user.update" && payload.Status != nil {
			status := strings.ToLower(strings.TrimSpace(*payload.Status))
			switch status {
			case "inactive", "disabled", "blocked", "suspended":
				action = "auth.user.disable"
			case "active", "enabled":
				action = "auth.user.enable"
			}
		}
		if action == "auth.user.update" && payload.Password != nil && strings.TrimSpace(*payload.Password) != "" {
			action = "auth.user.password.reset"
		}
		s.recordAudit(ctx, "global", s.actorFromContext(c), action, s.auditPayloadFromRequest(c, map[string]any{
			"userId":   user.ID,
			"username": user.Username,
			"status":   user.Status,
			"result":   "success",
		}), withResource("auth_user"))
		return c.JSON(http.StatusOK, newSystemUserResponse(user))
	}
}

func (s *HTTPServer) handleGetUserRoles() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		userID := strings.TrimSpace(c.Param("userId"))
		if userID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "userId is required")
		}
		if _, err := uuid.Parse(userID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "userId must be a valid UUID")
		}
		ctx := c.Request().Context()
		roles, err := s.authService.ListUserRoles(ctx, userID)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrUserNotFound):
				return echo.NewHTTPError(http.StatusNotFound, "user not found")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(http.StatusOK, roles)
	}
}

func (s *HTTPServer) handleReplaceUserRoles() echo.HandlerFunc {
	type request struct {
		Roles []string `json:"roles"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		userID := strings.TrimSpace(c.Param("userId"))
		if userID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "userId is required")
		}
		if _, err := uuid.Parse(userID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "userId must be a valid UUID")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		roles, err := s.authService.ReplaceUserRoles(ctx, userID, payload.Roles)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrUserNotFound):
				return echo.NewHTTPError(http.StatusNotFound, "user not found")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		s.recordAudit(ctx, "global", s.actorFromContext(c), "auth.user.roles.replace", s.auditPayloadFromRequest(c, map[string]any{
			"id":     userID,
			"roles":  roles,
			"result": "success",
		}), withResource("auth_user"))
		return c.JSON(http.StatusOK, roles)
	}
}

func (s *HTTPServer) handleDeleteUser() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "user directory is disabled")
		}
		userID := strings.TrimSpace(c.Param("userId"))
		if userID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "userId is required")
		}
		if _, err := uuid.Parse(userID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "userId must be a valid UUID")
		}
		ctx := c.Request().Context()
		if err := s.authService.DeleteUser(ctx, userID); err != nil {
			switch {
			case errors.Is(err, auth.ErrUserNotFound):
				return echo.NewHTTPError(http.StatusNotFound, "user not found")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.user.delete", s.auditPayloadFromRequest(c, map[string]any{
			"userId": userID,
			"result": "success",
		}), withResource("auth_user"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleSessionCreate() echo.HandlerFunc {
	type request struct {
		Username   string         `json:"username"`
		Password   string         `json:"password"`
		TenantID   string         `json:"tenantId"`
		MFACode    string         `json:"mfaCode"`
		AuthMethod string         `json:"authMethod"`
		Provider   map[string]any `json:"provider"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		username := strings.TrimSpace(payload.Username)
		password := strings.TrimSpace(payload.Password)
		if username == "" || password == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "username and password are required")
		}
		ctx := c.Request().Context()
		if s.superAdmin != nil && s.matchSuperAdminUsername(username) {
			user, session, tenantID, err := s.authenticateSuperAdmin(ctx, payload.TenantID, password, c.RealIP(), c.Request().UserAgent())
			if err != nil {
				if errors.Is(err, auth.ErrInvalidCredentials) {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			resp := s.newSessionResponse(ctx, user, tenantID, session, "superadmin")
			s.recordAudit(ctx, tenantID, user.Username, "auth.session.create", map[string]any{
				"principalId": user.PrincipalID(),
				"tenantId":    tenantID,
				"authMethod":  "superadmin",
			}, withResource("auth_session"))
			return c.JSON(http.StatusOK, resp)
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		authReq := auth.ProviderRequest{
			Username:   username,
			Password:   password,
			MFACode:    payload.MFACode,
			TenantHint: tenantID,
		}
		if len(payload.Provider) > 0 {
			authReq.Claims = payload.Provider
		}
		result, err := s.authService.AuthenticateWithProvider(ctx, payload.AuthMethod, authReq)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
			}
			if errors.Is(err, auth.ErrProviderNotFound) {
				return echo.NewHTTPError(http.StatusBadRequest, "auth provider not configured")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		user := result.User
		if tenantID == "" {
			tenantID = "global"
		}
		sessionToken, err := s.authService.IssueSession(ctx, user, auth.SessionOptions{TenantID: tenantID, AuthMethod: result.Method, ClientIP: c.RealIP(), UserAgent: c.Request().UserAgent()})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := s.newSessionResponse(ctx, user, tenantID, sessionToken, result.Method)
		s.recordAudit(ctx, tenantID, user.Username, "auth.session.create", map[string]any{
			"principalId": user.PrincipalID(),
			"tenantId":    tenantID,
			"authMethod":  result.Method,
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleSessionRefresh() echo.HandlerFunc {
	type request struct {
		RefreshToken string `json:"refreshToken"`
	}
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "credential authentication is disabled")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		refreshToken := strings.TrimSpace(payload.RefreshToken)
		if refreshToken == "" {
			refreshToken = strings.TrimSpace(s.credentialFromContext(c))
		}
		if refreshToken == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "refreshToken is required")
		}
		ctx := c.Request().Context()
		sessionToken, user, tenantID, authMethod, err := s.authService.RefreshSession(ctx, refreshToken)
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) || errors.Is(err, auth.ErrAPIKeyRevoked) {
				s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.session.timeout", s.auditPayloadFromRequest(c, map[string]any{
					"result":     "failed",
					"reason":     "refresh token invalid or expired",
					"authMethod": "refresh_token",
				}), withResource("auth_session"))
				return echo.NewHTTPError(http.StatusUnauthorized, "refresh token invalid")
			}
			if errors.Is(err, auth.ErrUserNotFound) {
				s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "auth.session.timeout", s.auditPayloadFromRequest(c, map[string]any{
					"result":     "failed",
					"reason":     "principal not found during refresh",
					"authMethod": "refresh_token",
				}), withResource("auth_session"))
				return echo.NewHTTPError(http.StatusUnauthorized, "principal not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if strings.TrimSpace(tenantID) == "" {
			tenantID = "global"
		}
		resp := s.newSessionResponse(ctx, user, tenantID, sessionToken, authMethod)
		csrf := setCSRFCookie(c)
		setAuthCookie(c, resp.Token, sessionToken.ExpiresAt)
		s.recordAudit(ctx, tenantID, user.Username, "auth.session.refresh", map[string]any{
			"tenantId":    tenantID,
			"principalId": user.PrincipalID(),
			"authMethod":  authMethod,
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, map[string]any{
			"token":              resp.Token,
			"refreshToken":       resp.RefreshToken,
			"expiresAt":          resp.ExpiresAt,
			"actor":              resp.Actor,
			"tenantId":           resp.TenantID,
			"authMethod":         resp.AuthMethod,
			"mustChangePassword": resp.MustChangePassword,
			"csrfToken":          csrf,
		})
	}
}

func newCredentialLoginResponse(user auth.User, session auth.SessionToken, tenantID string) credentialLoginResponse {
	role := normalizeRole(user.Role)
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = user.Username
	}
	expires := session.ExpiresAt.UTC().Format(time.RFC3339)
	if tenantID == "" {
		tenantID = "default"
	}
	return credentialLoginResponse{
		Token:              session.Token,
		RefreshToken:       session.Token,
		ExpiresAt:          expires,
		PrincipalID:        user.PrincipalID(),
		DisplayName:        displayName,
		Role:               role,
		TenantID:           tenantID,
		MustChangePassword: user.MustChangePassword,
	}
}

func capabilitySliceFromContext(c echo.Context) []string {
	capsMap := capabilitiesFromContext(c)
	if len(capsMap) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(capsMap))
	for capName := range capsMap {
		result = append(result, capName)
	}
	sort.Strings(result)
	return result
}

func (s *HTTPServer) credentialFromContext(c echo.Context) string {
	if c == nil {
		return ""
	}
	if credential, ok := c.Get(contextCredentialKey).(string); ok && credential != "" {
		return credential
	}
	if header := c.Request().Header.Get("Authorization"); header != "" {
		if token := bearerToken(header); token != "" {
			return token
		}
	}
	if apiKey := strings.TrimSpace(c.Request().Header.Get("X-API-Key")); apiKey != "" {
		return apiKey
	}
	if cookie, err := c.Request().Cookie("auth_token"); err == nil {
		value := strings.TrimSpace(cookie.Value)
		if value != "" {
			return value
		}
	}
	return ""
}

func setAuthCookie(c echo.Context, token string, expiresAt time.Time) {
	if c == nil || token == "" {
		return
	}
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	if !expiresAt.IsZero() {
		cookie.Expires = expiresAt.UTC()
		cookie.MaxAge = int(time.Until(expiresAt).Seconds())
	}
	c.SetCookie(cookie)
}

func clearAuthCookie(c echo.Context) {
	if c == nil {
		return
	}
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
	c.SetCookie(cookie)
}

func setCSRFCookie(c echo.Context) string {
	if c == nil {
		return ""
	}
	token := uuid.NewString()
	cookie := &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)
	return token
}

func clearCSRFCookie(c echo.Context) {
	if c == nil {
		return
	}
	cookie := &http.Cookie{
		Name:     "csrf_token",
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
	c.SetCookie(cookie)
}

func (s *HTTPServer) resolveAPIKeyMetadata(ctx context.Context, token string) (APIKeyMetadata, error) {
	if s.auth != nil {
		if meta, err := s.auth.ResolveAPIKey(ctx, token); err == nil {
			if strings.TrimSpace(meta.DisplayName) == "" {
				meta.DisplayName = token
			}
			if strings.TrimSpace(meta.PrincipalID) == "" {
				meta.PrincipalID = token
			}
			if strings.TrimSpace(meta.Role) == "" {
				meta.Role = RoleReader
			}
			return meta, nil
		} else if !errors.Is(err, auth.ErrAPIKeyNotFound) {
			return APIKeyMetadata{}, err
		}
	}
	if s.authService != nil {
		meta, err := s.authService.LookupToken(ctx, token)
		if err != nil {
			return APIKeyMetadata{}, err
		}
		result := APIKeyMetadata{
			DisplayName: meta.DisplayName,
			Role:        meta.Role,
			PrincipalID: meta.PrincipalID,
		}
		if strings.TrimSpace(result.DisplayName) == "" {
			result.DisplayName = token
		}
		if strings.TrimSpace(result.PrincipalID) == "" {
			result.PrincipalID = token
		}
		if strings.TrimSpace(result.Role) == "" {
			result.Role = RoleReader
		}
		return result, nil
	}
	return APIKeyMetadata{}, auth.ErrAPIKeyNotFound
}

func (s *HTTPServer) handleListAPIKeys() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "api key service unavailable")
		}
		principal := s.principalFromContext(c)
		includeRevoked := strings.EqualFold(c.QueryParam("includeRevoked"), "true")
		filter := auth.APIKeyFilter{
			IncludeRevoked: includeRevoked,
			Kinds:          []string{auth.KeyKindService},
		}
		if owner := userIDFromPrincipal(principal); owner != "" {
			if _, err := uuid.Parse(owner); err == nil {
				filter.OwnerUserID = &owner
			}
		}
		keys, err := s.authService.ListAPIKeys(c.Request().Context(), filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		now := time.Now().UTC()
		responses := make([]apiKeyResponse, 0, len(keys))
		for _, key := range keys {
			if !includeRevoked && !key.Valid(now) {
				continue
			}
			responses = append(responses, newAPIKeyResponse(key))
		}
		return c.JSON(http.StatusOK, responses)
	}
}

func (s *HTTPServer) handleCreateAPIKey() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "api key service unavailable")
		}
		var payload apiKeyCreateRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(payload.DisplayName)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "displayName is required")
		}
		role := normalizeRole(payload.Role)
		if role == "" {
			role = RoleReader
		}
		caps := sanitizeRequestedCapabilities(role, payload.Capabilities)
		principal := s.principalFromContext(c)
		if principal == "" {
			principal = s.actorFromContext(c)
		}
		tenantScope := systemTenantID
		trimmedExpiry := strings.TrimSpace(payload.ExpiresAt)
		var expires *time.Time
		if trimmedExpiry != "" {
			parsed, err := time.Parse(time.RFC3339, trimmedExpiry)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be RFC3339")
			}
			expiry := parsed.UTC()
			now := time.Now().UTC()
			if expiry.Before(now) {
				return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be in the future")
			}
			expires = &expiry
		}
		var metadata map[string]any
		if desc := strings.TrimSpace(payload.Description); desc != "" {
			metadata = map[string]any{"description": desc}
		}
		if len(caps) > 0 {
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata["capabilities"] = caps
		}
		if len(payload.IPWhitelist) > 0 {
			normalized := make([]string, 0, len(payload.IPWhitelist))
			for _, entry := range payload.IPWhitelist {
				trimmed := strings.TrimSpace(entry)
				if trimmed != "" {
					normalized = append(normalized, trimmed)
				}
			}
			if len(normalized) > 0 {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["ipWhitelist"] = normalized
			}
		}
		owner := strings.TrimSpace(payload.OwnerUserID)
		if owner == "" {
			owner = userIDFromPrincipal(principal)
		}
		var ownerPtr *string
		if owner != "" {
			if _, err := uuid.Parse(owner); err == nil {
				ownerPtr = &owner
			}
		}
		token, key, err := s.authService.CreateAPIKey(
			c.Request().Context(),
			name,
			role,
			principal,
			tenantScope,
			s.actorFromContext(c),
			payload.Description,
			metadata,
			expires,
			ownerPtr,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		auditPayload := map[string]any{
			"keyId":        key.ID,
			"role":         key.Role,
			"tenantScope":  tenantScope,
			"ownerUserId":  owner,
			"capabilities": caps,
			"createdBy":    s.actorFromContext(c),
		}
		if expires != nil {
			auditPayload["expiresAt"] = expires.Format(time.RFC3339)
		}
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"auth.api_key.create",
			auditPayload,
			withResource("auth_api_key"),
		)
		resp := newAPIKeyResponse(*key)
		resp.Token = token
		return c.JSON(http.StatusCreated, resp)
	}
}

func (s *HTTPServer) handleDeleteAPIKey() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "api key service unavailable")
		}
		keyID := strings.TrimSpace(c.Param("keyId"))
		if keyID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "keyId is required")
		}
		if err := s.authService.RevokeAPIKey(c.Request().Context(), keyID, s.actorFromContext(c)); err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "auth.api_key.revoke", map[string]any{"keyId": keyID}, withResource("auth_api_key"))
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "auth.api_key.disable", map[string]any{"keyId": keyID}, withResource("auth_api_key"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handleUpdateAPIKey() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.authService == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "api key service unavailable")
		}
		keyID := strings.TrimSpace(c.Param("keyId"))
		if keyID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "keyId is required")
		}
		var payload apiKeyUpdateRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		hasField := payload.DisplayName != nil || payload.Role != nil || payload.Capabilities != nil || payload.Description != nil || payload.IPWhitelist != nil || payload.ExpiresAt != nil || payload.Enabled != nil
		if !hasField {
			return echo.NewHTTPError(http.StatusBadRequest, "no updatable field provided")
		}
		opts := auth.UpdateAPIKeyOptions{
			DisplayName: payload.DisplayName,
			Role:        payload.Role,
			Description: payload.Description,
			Enabled:     payload.Enabled,
		}
		if payload.ExpiresAt != nil {
			trimmed := strings.TrimSpace(*payload.ExpiresAt)
			if trimmed == "" {
				opts.ClearExpiresAt = true
			} else {
				parsed, err := time.Parse(time.RFC3339, trimmed)
				if err != nil {
					return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be RFC3339")
				}
				expiry := parsed.UTC()
				opts.ExpiresAt = &expiry
			}
		}

		if payload.Capabilities != nil || payload.IPWhitelist != nil {
			roleForCaps := RoleReader
			if payload.Role != nil {
				roleForCaps = normalizeRole(*payload.Role)
			}
			meta := map[string]any{}
			if payload.Capabilities != nil {
				meta["capabilities"] = sanitizeRequestedCapabilities(roleForCaps, *payload.Capabilities)
			}
			if payload.IPWhitelist != nil {
				normalized := make([]string, 0, len(*payload.IPWhitelist))
				for _, item := range *payload.IPWhitelist {
					trimmed := strings.TrimSpace(item)
					if trimmed != "" {
						normalized = append(normalized, trimmed)
					}
				}
				meta["ipWhitelist"] = normalized
			}
			opts.Metadata = &meta
		}

		updated, err := s.authService.UpdateAPIKey(c.Request().Context(), keyID, s.actorFromContext(c), opts)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrAPIKeyNotFound):
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}

		action := "auth.api_key.update"
		if payload.Enabled != nil {
			if *payload.Enabled {
				action = "auth.api_key.enable"
			} else {
				action = "auth.api_key.disable"
			}
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), action, map[string]any{
			"keyId":        keyID,
			"displayName":  updated.Name,
			"role":         updated.Role,
			"enabled":      !updated.RevokedAt.Valid,
			"capabilities": apiKeyCapabilitiesFromMetadata(updated),
		}, withResource("auth_api_key"))

		return c.JSON(http.StatusOK, newAPIKeyResponse(updated))
	}
}

func (s *HTTPServer) requireSuperAdminPasswordReset() echo.MiddlewareFunc {
	if s.superAdmin == nil || s.superAdminAPIKey == "" {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !s.superAdmin.RequiresReset() {
				return next(c)
			}
			credential, _ := c.Get(contextCredentialKey).(string)
			if credential == "" || credential != s.superAdminAPIKey {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusLocked, "super admin must reset default password")
		}
	}
}

func (s *HTTPServer) matchSuperAdminUsername(value string) bool {
	if strings.TrimSpace(value) == "" && s.superAdminUsername == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), s.superAdminUsername)
}

func (s *HTTPServer) authenticateSuperAdmin(ctx context.Context, tenantHint, password, clientIP, userAgent string) (auth.User, auth.SessionToken, string, error) {
	if s.superAdmin == nil || s.authService == nil {
		return auth.User{}, auth.SessionToken{}, "", auth.ErrInvalidCredentials
	}
	ok, mustReset, err := s.superAdmin.Authenticate(password)
	if err != nil {
		if errors.Is(err, superadmin.ErrInvalidCredentials) {
			return auth.User{}, auth.SessionToken{}, "", auth.ErrInvalidCredentials
		}
		return auth.User{}, auth.SessionToken{}, "", err
	}
	if !ok {
		return auth.User{}, auth.SessionToken{}, "", auth.ErrInvalidCredentials
	}
	tenantID := strings.TrimSpace(tenantHint)
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	user := auth.User{
		ID:                 "superadmin",
		Username:           s.superAdminUsername,
		DisplayName:        "Super Administrator",
		Role:               RoleAdmin,
		Status:             "active",
		MustChangePassword: mustReset,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	session, err := s.authService.IssueSession(ctx, user, auth.SessionOptions{TenantID: tenantID, AuthMethod: "superadmin", ClientIP: clientIP, UserAgent: userAgent})
	if err != nil {
		return auth.User{}, auth.SessionToken{}, "", err
	}
	return user, session, tenantID, nil
}

func (s *HTTPServer) serveConsoleAsset(asset string) echo.HandlerFunc {
	return func(c echo.Context) error {
		data, err := consoleFS.ReadFile(asset)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "console asset not found")
		}
		c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
		if _, err := c.Response().Write(data); err != nil {
			return err
		}
		return nil
	}
}

func (s *HTTPServer) handleListPolicyDrafts(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		drafts, err := policySvc.ListDrafts(c.Request().Context(), tenantID, page.Limit)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, drafts)
	}
}

func (s *HTTPServer) handleGetPolicyDraft(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		draftID := strings.TrimSpace(c.Param("draftId"))
		if draftID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "draftId is required")
		}
		draft, err := policySvc.GetDraft(c.Request().Context(), tenantID, draftID)
		if err != nil {
			if errors.Is(err, policy.ErrDraftNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, draft)
	}
}

func (s *HTTPServer) handleCreatePolicyDraft(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			Name        string             `json:"name"`
			Description string             `json:"description"`
			Rules       []policy.DraftRule `json:"rules"`
			Metadata    json.RawMessage    `json:"metadata"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if len(payload.Rules) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "at least one rule is required")
		}
		_, tenantID := s.resourceScopeContext(c)
		actor := s.actorFromContext(c)
		ctx := c.Request().Context()
		draft, err := policySvc.CreateDraft(ctx, policy.CreateDraftRequest{
			TenantID:    tenantID,
			Name:        payload.Name,
			Description: payload.Description,
			Rules:       payload.Rules,
			Metadata:    payload.Metadata,
			Actor:       actor,
		})
		if err != nil {
			switch {
			case errors.Is(err, policy.ErrDraftEmptyRules), errors.Is(err, policy.ErrPriorityRange):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		change := auditpayload.ConfigChange{
			Resource:   "policy_draft",
			Action:     "create",
			Identifier: draft.ID,
			After: map[string]any{
				"name":   draft.Name,
				"status": draft.Status,
			},
			Fields: []string{"name", "status"},
		}
		s.recordAudit(ctx, tenantID, actor, "policy.draft.create", change, withResource("policy_draft"))
		return c.JSON(http.StatusCreated, draft)
	}
}

func (s *HTTPServer) handleUpdatePolicyDraft(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			Name        *string             `json:"name"`
			Description *string             `json:"description"`
			Rules       *[]policy.DraftRule `json:"rules"`
			Metadata    *json.RawMessage    `json:"metadata"`
			Status      *string             `json:"status"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		draftID := strings.TrimSpace(c.Param("draftId"))
		if draftID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "draftId is required")
		}
		actor := s.actorFromContext(c)
		ctx := c.Request().Context()
		req := policy.UpdateDraftRequest{
			TenantID:    tenantID,
			DraftID:     draftID,
			Name:        payload.Name,
			Description: payload.Description,
			Rules:       payload.Rules,
			Metadata:    payload.Metadata,
			Status:      payload.Status,
			Actor:       actor,
		}
		draft, err := policySvc.UpdateDraft(ctx, req)
		if err != nil {
			switch {
			case errors.Is(err, policy.ErrDraftNotFound):
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			case errors.Is(err, policy.ErrDraftEmptyRules), errors.Is(err, policy.ErrPriorityRange):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		fields := make([]string, 0, 5)
		if payload.Name != nil {
			fields = append(fields, "name")
		}
		if payload.Description != nil {
			fields = append(fields, "description")
		}
		if payload.Rules != nil {
			fields = append(fields, "rules")
		}
		if payload.Metadata != nil {
			fields = append(fields, "metadata")
		}
		if payload.Status != nil {
			fields = append(fields, "status")
		}
		change := auditpayload.ConfigChange{
			Resource:   "policy_draft",
			Action:     "update",
			Identifier: draft.ID,
			After: map[string]any{
				"name":        draft.Name,
				"status":      draft.Status,
				"updatedAt":   draft.UpdatedAt,
				"description": draft.Description,
			},
			Fields: fields,
		}
		s.recordAudit(ctx, tenantID, actor, "policy.draft.update", change, withResource("policy_draft"))
		return c.JSON(http.StatusOK, draft)
	}
}

func (s *HTTPServer) handleDeletePolicyDraft(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		draftID := strings.TrimSpace(c.Param("draftId"))
		if draftID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "draftId is required")
		}
		ctx := c.Request().Context()
		if err := policySvc.DeleteDraft(ctx, tenantID, draftID); err != nil {
			if errors.Is(err, policy.ErrDraftNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "policy_draft",
			Action:     "delete",
			Identifier: draftID,
		}
		s.recordAudit(ctx, tenantID, actor, "policy.draft.delete", change, withResource("policy_draft"))
		return c.NoContent(http.StatusNoContent)
	}
}

func (s *HTTPServer) handlePublishPolicyDraft(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			Changelog  string          `json:"changelog"`
			Metadata   json.RawMessage `json:"metadata"`
			RollbackOf string          `json:"rollbackOf"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		draftID := strings.TrimSpace(c.Param("draftId"))
		if draftID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "draftId is required")
		}
		actor := s.actorFromContext(c)
		ctx := c.Request().Context()
		version, err := policySvc.PublishDraft(ctx, policy.PublishDraftRequest{
			TenantID:   tenantID,
			DraftID:    draftID,
			Actor:      actor,
			Changelog:  payload.Changelog,
			Metadata:   payload.Metadata,
			RollbackOf: payload.RollbackOf,
		})
		if err != nil {
			switch {
			case errors.Is(err, policy.ErrDraftNotFound):
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			case errors.Is(err, policy.ErrDraftAlreadyClosed):
				return echo.NewHTTPError(http.StatusConflict, err.Error())
			case errors.Is(err, policy.ErrDraftEmptyRules), errors.Is(err, policy.ErrPriorityRange):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		versionChange := auditpayload.ConfigChange{
			Resource:   "policy_version",
			Action:     "publish",
			Identifier: version.ID,
			After: map[string]any{
				"version":     version.Version,
				"derivedFrom": version.DerivedFrom.String,
				"publishedAt": version.PublishedAt,
			},
			Fields: []string{"version", "derivedFrom", "publishedAt"},
		}
		s.recordAudit(ctx, tenantID, actor, "policy.version.publish", versionChange, withResource("policy_version"))
		s.recordAudit(ctx, tenantID, actor, "policy.draft.publish", map[string]any{
			"draftId": draftID,
			"version": version.Version,
		}, withResource("policy_draft"))
		return c.JSON(http.StatusCreated, version)
	}
}

func (s *HTTPServer) handleListPolicyVersions(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		versions, err := policySvc.ListVersions(c.Request().Context(), tenantID, page.Limit)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, versions)
	}
}

func (s *HTTPServer) handleGetPolicyVersion(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		versionParam := strings.TrimSpace(c.Param("versionNumber"))
		if versionParam == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "versionNumber is required")
		}
		versionNumber, err := strconv.Atoi(versionParam)
		if err != nil || versionNumber <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "versionNumber must be a positive integer")
		}
		version, err := policySvc.GetVersion(c.Request().Context(), tenantID, versionNumber)
		if err != nil {
			if errors.Is(err, policy.ErrVersionNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, version)
	}
}

func (s *HTTPServer) handleEvaluatePolicyRules(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			DraftID string             `json:"draftId"`
			Rules   []policy.DraftRule `json:"rules"`
			Input   map[string]any     `json:"input"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if len(payload.Rules) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "rules are required")
		}
		_, tenantID := s.resourceScopeContext(c)
		var input policy.Input
		if len(payload.Input) > 0 {
			raw, marshalErr := json.Marshal(payload.Input)
			if marshalErr != nil {
				return echo.NewHTTPError(http.StatusBadRequest, marshalErr.Error())
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		input.TenantID = tenantID
		ctx := c.Request().Context()
		decision, err := policySvc.EvaluateDraft(ctx, policy.EvaluateDraftRequest{
			TenantID: tenantID,
			DraftID:  strings.TrimSpace(payload.DraftID),
			Input:    input,
		})
		if err != nil {
			switch {
			case errors.Is(err, policy.ErrNoMatchingRule):
				return c.JSON(http.StatusOK, map[string]any{
					"matched":  false,
					"decision": nil,
				})
			case errors.Is(err, policy.ErrDraftEmptyRules), errors.Is(err, policy.ErrPriorityRange):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				msg := err.Error()
				if strings.Contains(msg, "engine unavailable") {
					return echo.NewHTTPError(http.StatusServiceUnavailable, msg)
				}
				if strings.HasPrefix(msg, "policy:") {
					return echo.NewHTTPError(http.StatusBadRequest, msg)
				}
				return echo.NewHTTPError(http.StatusInternalServerError, msg)
			}
		}
		return c.JSON(http.StatusOK, map[string]any{
			"matched":  true,
			"decision": decision,
		})
	}
}

func (s *HTTPServer) handleRollbackPolicyVersion(policySvc *policy.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		versionParam := strings.TrimSpace(c.Param("versionNumber"))
		if versionParam == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "versionNumber is required")
		}
		versionNumber, err := strconv.Atoi(versionParam)
		if err != nil || versionNumber <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "versionNumber must be a positive integer")
		}
		var payload struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Changelog   string          `json:"changelog"`
			Metadata    json.RawMessage `json:"metadata"`
		}
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		actor := s.actorFromContext(c)
		ctx := c.Request().Context()
		version, err := policySvc.RollbackVersion(ctx, policy.RollbackVersionRequest{
			TenantID:      tenantID,
			VersionNumber: versionNumber,
			Actor:         actor,
			Name:          payload.Name,
			Description:   payload.Description,
			Changelog:     payload.Changelog,
			Metadata:      payload.Metadata,
		})
		if err != nil {
			switch {
			case errors.Is(err, policy.ErrVersionNotFound):
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			case errors.Is(err, policy.ErrDraftEmptyRules), errors.Is(err, policy.ErrPriorityRange):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		actorName := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "policy_version",
			Action:     "rollback",
			Identifier: version.ID,
			After: map[string]any{
				"version":     version.Version,
				"rollbackOf":  version.RollbackOf.String,
				"publishedAt": version.PublishedAt,
			},
			Fields: []string{"version", "rollbackOf", "publishedAt"},
		}
		s.recordAudit(ctx, tenantID, actorName, "policy.version.rollback", change, withResource("policy_version"))
		return c.JSON(http.StatusCreated, version)
	}
}

func (s *HTTPServer) mountResourceRoutes(apiGroup *echo.Group, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, iotSvc *iotregistry.Service) {
	_ = policyEngine

	resourceGroup := apiGroup.Group("")
	if s.options.RequireAuth {
		resourceGroup.Use(RequireRole(RoleReader))
	}
	adminGroup := resourceGroup.Group("")
	if s.options.RequireAuth {
		adminGroup.Use(RequireRole(RoleAdmin))
	}

	resourceGroup.GET("/policies/drafts", s.handleListPolicyDrafts(policySvc), RequireCapability(CapabilityPolicyRead))
	resourceGroup.GET("/policies/drafts/:draftId", s.handleGetPolicyDraft(policySvc), RequireCapability(CapabilityPolicyRead))
	adminGroup.POST("/policies/drafts", s.handleCreatePolicyDraft(policySvc), RequireCapability(CapabilityPolicyWrite))
	adminGroup.PATCH("/policies/drafts/:draftId", s.handleUpdatePolicyDraft(policySvc), RequireCapability(CapabilityPolicyWrite))
	adminGroup.DELETE("/policies/drafts/:draftId", s.handleDeletePolicyDraft(policySvc), RequireCapability(CapabilityPolicyWrite))
	adminGroup.POST("/policies/drafts/:draftId/publish", s.handlePublishPolicyDraft(policySvc), RequireCapability(CapabilityPolicyWrite))

	// Expose pool导出在非 core 路径，兼容前端请求 /pools/export
	resourceGroup.GET("/pools/export", s.handleDHCPPoolsExportCSV(), RequireCapability(CapabilityPoolRead))
	// Expose pool导入在非 core 路径，兼容前端请求 /pools/import
	resourceGroup.POST("/pools/import", s.handleDHCPPoolsImportCSV(), RequireCapability(CapabilityPoolWrite))
	// Expose pool CRUD 在非 core 路径，兼容前端直接请求 /pools
	resourceGroup.GET("/pools", s.handleDHCPPoolsList(), RequireCapability(CapabilityPoolRead))
	resourceGroup.GET("/pools/stats", s.handleDHCPPoolsStats(), RequireCapability(CapabilityPoolRead))
	resourceGroup.GET("/pools/:poolId", s.handleDHCPPoolGet(), RequireCapability(CapabilityPoolRead))
	resourceGroup.GET("/pools/:poolId/usage", s.handleDHCPPoolUsage(), RequireCapability(CapabilityPoolRead))
	resourceGroup.GET("/pools/:poolId/history", s.handleDHCPPoolHistory(), RequireCapability(CapabilityPoolRead))
	resourceGroup.GET("/pools/:poolId/conflicts", s.handleDHCPPoolConflicts(), RequireCapability(CapabilityPoolRead))
	adminGroup.POST("/pools", s.handleDHCPPoolsCreate(), RequireCapability(CapabilityPoolWrite))
	adminGroup.PATCH("/pools/:poolId", s.handleDHCPPoolsUpdate(), RequireCapability(CapabilityPoolWrite))
	adminGroup.DELETE("/pools/:poolId", s.handleDHCPPoolDelete(), RequireCapability(CapabilityPoolWrite))
	adminGroup.POST("/pools/:poolId/reconcile", s.handleDHCPPoolsReconcile(), RequireCapability(CapabilityPoolWrite))
	resourceGroup.GET("/policies/versions", s.handleListPolicyVersions(policySvc), RequireCapability(CapabilityPolicyRead))
	resourceGroup.GET("/policies/versions/:versionNumber", s.handleGetPolicyVersion(policySvc), RequireCapability(CapabilityPolicyRead))
	adminGroup.POST("/policies/versions/:versionNumber/rollback", s.handleRollbackPolicyVersion(policySvc), RequireCapability(CapabilityPolicyWrite))
	resourceGroup.POST("/policies/evaluate", s.handleEvaluatePolicyRules(policySvc), RequireCapability(CapabilityPolicyRead))

	resourceGroup.GET("/policies", func(c echo.Context) error {
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		rules, err := policySvc.ListRules(c.Request().Context(), tenantID, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, rules)
	}, RequireCapability(CapabilityPolicyRead))

	adminGroup.POST("/policies", func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			Priority   int             `json:"priority"`
			Conditions json.RawMessage `json:"conditions"`
			Actions    json.RawMessage `json:"actions"`
			Enabled    *bool           `json:"enabled"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		ctx := c.Request().Context()
		rule, err := policySvc.CreateRule(ctx, policy.CreateRuleRequest{
			TenantID:   tenantID,
			Priority:   payload.Priority,
			Conditions: payload.Conditions,
			Actions:    payload.Actions,
			Enabled:    enabled,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "policy_rule",
			Action:     "create",
			Identifier: rule.ID,
			After: map[string]any{
				"priority": rule.Priority,
				"enabled":  rule.Enabled,
			},
			Fields: []string{"priority", "enabled"},
		}
		s.recordAudit(ctx, tenantID, actor, "policy.rule.create", change, withResource("policy_rule"))
		return c.JSON(http.StatusCreated, rule)
	}, RequireCapability(CapabilityPolicyWrite))

	adminGroup.PUT("/policies/:policyId", func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		var payload struct {
			Priority   int             `json:"priority"`
			Conditions json.RawMessage `json:"conditions"`
			Actions    json.RawMessage `json:"actions"`
			Enabled    *bool           `json:"enabled"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		ctx := c.Request().Context()
		rule, err := policySvc.UpdateRule(ctx, policy.UpdateRuleRequest{
			RuleID: c.Param("policyId"),
			CreateRuleRequest: policy.CreateRuleRequest{
				TenantID:   tenantID,
				Priority:   payload.Priority,
				Conditions: payload.Conditions,
				Actions:    payload.Actions,
				Enabled:    enabled,
			},
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "policy_rule",
			Action:     "update",
			Identifier: rule.ID,
			After: map[string]any{
				"priority": rule.Priority,
				"enabled":  rule.Enabled,
			},
			Fields: []string{"priority", "enabled"},
		}
		s.recordAudit(ctx, tenantID, actor, "policy.rule.update", change, withResource("policy_rule"))
		return c.JSON(http.StatusOK, rule)
	}, RequireCapability(CapabilityPolicyWrite))

	adminGroup.DELETE("/policies/:policyId", func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		ruleID := strings.TrimSpace(c.Param("policyId"))
		if ruleID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "policyId is required")
		}
		ctx := c.Request().Context()
		if err := policySvc.DeleteRule(ctx, tenantID, ruleID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "policy_rule",
			Action:     "delete",
			Identifier: ruleID,
		}
		s.recordAudit(ctx, tenantID, actor, "policy.rule.delete", change, withResource("policy_rule"))
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityPolicyWrite))

	if securityPolicySvc != nil {
		resourceGroup.GET("/security/policies", s.handleSecurityPolicyList(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyRead))
		resourceGroup.GET("/security/policies/:ruleId", s.handleSecurityPolicyGet(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyRead))
		adminGroup.POST("/security/policies", s.handleSecurityPolicyCreate(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
		adminGroup.PUT("/security/policies/:ruleId", s.handleSecurityPolicyUpdate(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
		adminGroup.DELETE("/security/policies/:ruleId", s.handleSecurityPolicyDelete(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
	}

	type (
		poolPayload struct {
			Scope          string             `json:"scope"`
			ParentID       *string            `json:"parentId"`
			Name           string             `json:"name"`
			CIDR           string             `json:"cidr"`
			Network        string             `json:"network"`
			Netmask        string             `json:"netmask"`
			RangeStart     string             `json:"rangeStart"`
			RangeEnd       string             `json:"rangeEnd"`
			Gateway        string             `json:"gateway"`
			Option43       string             `json:"option43"`
			DNS            []string           `json:"dns"`
			VLANID         *int               `json:"vlanId"`
			InterfaceID    *string            `json:"interfaceId"`
			SSID           *string            `json:"ssid"`
			Location       *string            `json:"location"`
			ReservePercent int                `json:"reservePercent"`
			LeaseProfileID string             `json:"leaseProfileId"`
			Tags           []string           `json:"tags"`
			Exclusions     models.IPRangeList `json:"exclusions"`
			AllocationMode string             `json:"allocationMode"`
			PriorityWeight int                `json:"priorityWeight"`
			Status         string             `json:"status"`
		}
		poolFindPayload struct {
			Scope       string  `json:"scope"`
			ParentID    *string `json:"parentId"`
			VLANID      *int    `json:"vlanId"`
			InterfaceID *string `json:"interfaceId"`
			SSID        *string `json:"ssid"`
			Location    *string `json:"location"`
			Limit       int     `json:"limit"`
		}
		bindingPayload struct {
			Identifier     string          `json:"identifier"`
			IdentifierType string          `json:"identifierType"`
			PoolID         string          `json:"poolId"`
			IPAddress      string          `json:"ipAddress"`
			LeaseProfileID string          `json:"leaseProfileId"`
			Metadata       json.RawMessage `json:"metadata"`
		}
	)

	buildPoolRequest := func(tenantID string, payload poolPayload) pool.PoolCreateRequest {
		return pool.PoolCreateRequest{
			TenantID:       tenantID,
			Scope:          strings.TrimSpace(payload.Scope),
			ParentID:       payload.ParentID,
			Name:           strings.TrimSpace(payload.Name),
			CIDR:           strings.TrimSpace(payload.CIDR),
			Network:        strings.TrimSpace(payload.Network),
			Netmask:        strings.TrimSpace(payload.Netmask),
			RangeStart:     strings.TrimSpace(payload.RangeStart),
			RangeEnd:       strings.TrimSpace(payload.RangeEnd),
			Gateway:        strings.TrimSpace(payload.Gateway),
			Option43:       strings.TrimSpace(payload.Option43),
			DNS:            payload.DNS,
			VLANID:         payload.VLANID,
			InterfaceID:    payload.InterfaceID,
			SSID:           payload.SSID,
			Location:       payload.Location,
			ReservePercent: payload.ReservePercent,
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Tags:           encodeTagPayload(payload.Tags...),
			Exclusions:     payload.Exclusions,
			AllocationMode: strings.TrimSpace(payload.AllocationMode),
			PriorityWeight: payload.PriorityWeight,
			Status:         strings.TrimSpace(payload.Status),
		}
	}
	makeBindingRequest := func(tenantID string, payload bindingPayload) pool.BindingCreateRequest {
		metadata := []byte(nil)
		if len(payload.Metadata) > 0 {
			metadata = append([]byte(nil), payload.Metadata...)
		}
		return pool.BindingCreateRequest{
			TenantID:       tenantID,
			Identifier:     strings.TrimSpace(payload.Identifier),
			IdentifierType: strings.TrimSpace(payload.IdentifierType),
			PoolID:         strings.TrimSpace(payload.PoolID),
			IPAddress:      strings.TrimSpace(payload.IPAddress),
			LeaseProfileID: strings.TrimSpace(payload.LeaseProfileID),
			Metadata:       metadata,
		}
	}

	resourceGroup.GET("/pools/:poolId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		poolObj, err := poolSvc.GetPool(c.Request().Context(), scopeRef, poolID)
		if err != nil {
			// Normalize missing pool errors to 404 instead of bubbling sql.ErrNoRows as 500
			if errors.Is(err, pool.ErrPoolNotFound) || errors.Is(err, pool.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, poolObj)
	}, RequireCapability(CapabilityPoolRead))

	adminGroup.POST("/pools", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload poolPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		if strings.TrimSpace(payload.Name) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		ctx := c.Request().Context()
		poolObj, err := poolSvc.CreatePool(ctx, scopeRef, buildPoolRequest(tenantID, payload))
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "create",
			Identifier: poolObj.ID,
			After:      auditMapFromPool(*poolObj),
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "pool.create", change, withResource("pool"))
		return c.JSON(http.StatusCreated, poolObj)
	}, RequireCapability(CapabilityPoolWrite))

	adminGroup.PATCH("/pools/:poolId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload poolPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		poolID := strings.TrimSpace(c.Param("poolId"))
		if poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "poolId is required")
		}
		ctx := c.Request().Context()
		beforePool, err := poolSvc.GetPool(ctx, scopeRef, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrPoolNotFound) || errors.Is(err, pool.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		updated, err := poolSvc.UpdatePool(ctx, scopeRef, pool.PoolUpdateRequest{
			PoolID:            poolID,
			PoolCreateRequest: buildPoolRequest(tenantID, payload),
		})
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		before := auditMapFromPool(*beforePool)
		after := auditMapFromPool(*updated)
		change := auditpayload.ConfigChange{
			Resource:   "pool",
			Action:     "update",
			Identifier: poolID,
			Before:     before,
			After:      after,
			Diff:       diffAuditMaps(before, after),
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "pool.update", change, withResource("pool"))
		return c.JSON(http.StatusOK, updated)
	}, RequireCapability(CapabilityPoolWrite))

	resourceGroup.POST("/pools/find", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		var payload poolFindPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		filter := pool.MetadataFilter{
			Scope:       strings.TrimSpace(payload.Scope),
			ParentID:    payload.ParentID,
			VLANID:      payload.VLANID,
			InterfaceID: payload.InterfaceID,
			SSID:        payload.SSID,
			Location:    payload.Location,
			Limit:       payload.Limit,
		}
		if filter.Limit <= 0 || filter.Limit > 500 {
			filter.Limit = 50
		}
		pools, err := poolSvc.FindPools(c.Request().Context(), scopeRef, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, pools)
	}, RequireCapability(CapabilityPoolRead))

	resourceGroup.GET("/bindings", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		if page.Limit == 0 {
			page.Limit = 100
		}
		filter := pool.BindingFilter{
			IdentifierType: "mac",
			MAC:            strings.TrimSpace(c.QueryParam("mac")),
			IP:             strings.TrimSpace(c.QueryParam("ip")),
			PoolID:         strings.TrimSpace(c.QueryParam("poolId")),
		}
		if filter.MAC == "" {
			filter.MAC = strings.TrimSpace(c.QueryParam("identifier"))
		}
		bindings, err := poolSvc.ListBindings(c.Request().Context(), scopeRef, filter, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, bindings)
	}, RequireCapability(CapabilityBindingRead))

	adminGroup.POST("/bindings", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload bindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		payload.IdentifierType = strings.ToLower(strings.TrimSpace(payload.IdentifierType))
		if payload.IdentifierType == "" {
			payload.IdentifierType = "mac"
		}
		if payload.IdentifierType != "mac" {
			return echo.NewHTTPError(http.StatusBadRequest, "only MAC bindings are supported")
		}
		req := makeBindingRequest(tenantID, payload)
		if req.Identifier == "" || req.PoolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "identifier and poolId are required")
		}
		ctx := c.Request().Context()
		binding, err := poolSvc.CreateBinding(ctx, scopeRef, req)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "create",
			Identifier: binding.ID,
			After: map[string]any{
				"identifier": binding.Identifier,
				"poolId":     binding.PoolID,
				"ipAddress":  binding.IPAddress,
			},
			Fields: []string{"identifier", "poolId", "ipAddress"},
		}
		s.recordAudit(ctx, tenantID, actor, "binding.create", change, withResource("binding"))
		return c.JSON(http.StatusCreated, binding)
	}, RequireCapability(CapabilityBindingManage))

	adminGroup.PATCH("/bindings/:bindingId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload bindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		payload.IdentifierType = strings.ToLower(strings.TrimSpace(payload.IdentifierType))
		if payload.IdentifierType == "" {
			payload.IdentifierType = "mac"
		}
		if payload.IdentifierType != "mac" {
			return echo.NewHTTPError(http.StatusBadRequest, "only MAC bindings are supported")
		}
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "bindingId is required")
		}
		ctx := c.Request().Context()
		binding, err := poolSvc.UpdateBinding(ctx, scopeRef, bindingID, makeBindingRequest(tenantID, payload))
		if err != nil {
			if errors.Is(err, pool.ErrBindingNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "binding not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "update",
			Identifier: binding.ID,
			After: map[string]any{
				"identifier": binding.Identifier,
				"poolId":     binding.PoolID,
				"ipAddress":  binding.IPAddress,
			},
			Fields: []string{"identifier", "poolId", "ipAddress"},
		}
		s.recordAudit(ctx, tenantID, actor, "binding.update", change, withResource("binding"))
		return c.JSON(http.StatusOK, binding)
	}, RequireCapability(CapabilityBindingManage))

	adminGroup.DELETE("/bindings/:bindingId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		scopeRef := s.poolScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "bindingId is required")
		}
		ctx := c.Request().Context()
		actor := s.actorFromContext(c)
		if err := poolSvc.DeleteBinding(ctx, scopeRef, bindingID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		change := auditpayload.ConfigChange{
			Resource:   "binding",
			Action:     "delete",
			Identifier: bindingID,
		}
		s.recordAudit(ctx, tenantID, actor, "binding.delete", change, withResource("binding"))
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityBindingManage))

	adminGroup.GET("/quotas", s.handleTenantQuotaGet(), RequireCapability(CapabilityTenantQuotaRead))
	adminGroup.PUT("/quotas", s.handleTenantQuotaUpdate(), RequireCapability(CapabilityTenantQuotaWrite))

	resourceGroup.GET("/rbac/assignments", s.handleListAssignments(), RequireCapability(CapabilityRBACAssignmentRead))
	adminGroup.POST("/rbac/assignments", s.handleCreateAssignment(), RequireCapability(CapabilityRBACAssignmentWrite))
	adminGroup.DELETE("/rbac/assignments/:assignmentId", s.handleDeleteAssignment(), RequireCapability(CapabilityRBACAssignmentWrite))

	type (
		iotDevicePayload struct {
			DeviceID       string            `json:"deviceId"`
			DisplayName    string            `json:"displayName"`
			HardwareAddr   string            `json:"hardwareAddr"`
			ProfileID      string            `json:"profileId"`
			LeaseProfileID string            `json:"leaseProfileId"`
			SleepClass     string            `json:"sleepClass"`
			SleepInterval  string            `json:"sleepInterval"`
			OfflineWindow  string            `json:"offlineWindow"`
			SleepyHint     bool              `json:"sleepyHint"`
			Status         string            `json:"status"`
			Firmware       string            `json:"firmwareVersion"`
			Labels         map[string]string `json:"labels"`
			Metadata       json.RawMessage   `json:"metadata"`
			LastSeen       string            `json:"lastSeen"`
		}
		iotProfilePayload struct {
			Name           string          `json:"name"`
			Description    string          `json:"description"`
			SleepClass     string          `json:"sleepClass"`
			SleepInterval  string          `json:"sleepInterval"`
			OfflineWindow  string          `json:"offlineWindow"`
			LeaseProfileID string          `json:"leaseProfileId"`
			SleepyCapable  bool            `json:"sleepyCapable"`
			Metadata       json.RawMessage `json:"metadata"`
		}
		heartbeatPayload struct {
			Status   string `json:"status"`
			LastSeen string `json:"lastSeen"`
		}
	)

	buildDeviceRequest := func(tenantID, deviceID string, payload iotDevicePayload) (iotregistry.DeviceUpsertRequest, error) {
		sleepInterval, err := parseDurationField(payload.SleepInterval)
		if err != nil {
			return iotregistry.DeviceUpsertRequest{}, echo.NewHTTPError(http.StatusBadRequest, "sleepInterval must be Go duration string")
		}
		offlineWindow, err := parseDurationField(payload.OfflineWindow)
		if err != nil {
			return iotregistry.DeviceUpsertRequest{}, echo.NewHTTPError(http.StatusBadRequest, "offlineWindow must be Go duration string")
		}
		lastSeen, err := parseOptionalTime(payload.LastSeen)
		if err != nil {
			return iotregistry.DeviceUpsertRequest{}, echo.NewHTTPError(http.StatusBadRequest, "lastSeen must be RFC3339 timestamp")
		}
		return iotregistry.DeviceUpsertRequest{
			TenantID:       tenantID,
			DeviceID:       deviceID,
			DisplayName:    payload.DisplayName,
			HardwareAddr:   payload.HardwareAddr,
			ProfileID:      payload.ProfileID,
			LeaseProfileID: payload.LeaseProfileID,
			SleepClass:     payload.SleepClass,
			SleepInterval:  sleepInterval,
			OfflineWindow:  offlineWindow,
			SleepyHint:     payload.SleepyHint,
			Status:         payload.Status,
			Firmware:       payload.Firmware,
			Labels:         payload.Labels,
			Metadata:       payload.Metadata,
			LastSeen:       lastSeen,
		}, nil
	}

	buildProfileRequest := func(tenantID, profileID string, payload iotProfilePayload) (iotregistry.ProfileUpsertRequest, error) {
		sleepInterval, err := parseDurationField(payload.SleepInterval)
		if err != nil {
			return iotregistry.ProfileUpsertRequest{}, echo.NewHTTPError(http.StatusBadRequest, "sleepInterval must be Go duration string")
		}
		offlineWindow, err := parseDurationField(payload.OfflineWindow)
		if err != nil {
			return iotregistry.ProfileUpsertRequest{}, echo.NewHTTPError(http.StatusBadRequest, "offlineWindow must be Go duration string")
		}
		return iotregistry.ProfileUpsertRequest{
			TenantID:       tenantID,
			ProfileID:      profileID,
			Name:           payload.Name,
			Description:    payload.Description,
			SleepClass:     payload.SleepClass,
			SleepInterval:  sleepInterval,
			OfflineWindow:  offlineWindow,
			LeaseProfileID: payload.LeaseProfileID,
			SleepyCapable:  payload.SleepyCapable,
			Metadata:       payload.Metadata,
		}, nil
	}

	iotGroup := adminGroup.Group("/iot")

	iotGroup.GET("/devices", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		if page.Limit == 0 {
			page.Limit = 100
		}
		opts := iotregistry.DeviceListOptions{
			ProfileID:  c.QueryParam("profileId"),
			Status:     c.QueryParam("status"),
			SleepyOnly: strings.EqualFold(c.QueryParam("sleepyOnly"), "true"),
			Search:     c.QueryParam("search"),
			Limit:      page.Limit,
			Offset:     page.Offset,
		}
		devices, err := s.iotRegistry.ListDevices(c.Request().Context(), tenantID, opts)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, buildIoTDeviceResponses(devices))
	}, RequireCapability(CapabilityIoTRegistryRead))

	iotGroup.GET("/devices/:deviceId", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "deviceId is required")
		}
		device, err := s.iotRegistry.GetDevice(c.Request().Context(), tenantID, deviceID)
		if err != nil {
			if errors.Is(err, iotstore.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "device not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, buildIoTDeviceResponse(device))
	}, RequireCapability(CapabilityIoTRegistryRead))

	iotGroup.POST("/devices", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		var payload iotDevicePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		deviceID := strings.TrimSpace(payload.DeviceID)
		if deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "deviceId is required")
		}
		req, err := buildDeviceRequest(tenantID, deviceID, payload)
		if err != nil {
			return err
		}
		device, err := s.iotRegistry.UpsertDevice(c.Request().Context(), req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusCreated, buildIoTDeviceResponse(device))
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.PUT("/devices/:deviceId", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		var payload iotDevicePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "deviceId is required")
		}
		req, err := buildDeviceRequest(tenantID, deviceID, payload)
		if err != nil {
			return err
		}
		device, err := s.iotRegistry.UpsertDevice(c.Request().Context(), req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, buildIoTDeviceResponse(device))
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.DELETE("/devices/:deviceId", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "deviceId is required")
		}
		if err := s.iotRegistry.DeleteDevice(c.Request().Context(), tenantID, deviceID); err != nil {
			if errors.Is(err, iotstore.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "device not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.POST("/devices/:deviceId/heartbeat", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		var payload heartbeatPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "deviceId is required")
		}
		seenAt := time.Now().UTC()
		if strings.TrimSpace(payload.LastSeen) != "" {
			parsed, err := time.Parse(time.RFC3339, payload.LastSeen)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "lastSeen must be RFC3339 timestamp")
			}
			seenAt = parsed.UTC()
		}
		if err := s.iotRegistry.RecordHeartbeat(c.Request().Context(), tenantID, deviceID, payload.Status, seenAt); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.GET("/profiles", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		profiles, err := s.iotRegistry.ListProfiles(c.Request().Context(), tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, buildIoTProfileResponses(profiles))
	}, RequireCapability(CapabilityIoTRegistryRead))

	iotGroup.POST("/profiles", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		var payload iotProfilePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		req, err := buildProfileRequest(tenantID, "", payload)
		if err != nil {
			return err
		}
		profile, err := s.iotRegistry.CreateProfile(c.Request().Context(), req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusCreated, buildIoTProfileResponse(profile))
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.PUT("/profiles/:profileId", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		var payload iotProfilePayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		_, tenantID := s.resourceScopeContext(c)
		profileID := strings.TrimSpace(c.Param("profileId"))
		if profileID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "profileId is required")
		}
		req, err := buildProfileRequest(tenantID, profileID, payload)
		if err != nil {
			return err
		}
		profile, err := s.iotRegistry.UpdateProfile(c.Request().Context(), req)
		if err != nil {
			if errors.Is(err, iotstore.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "profile not found")
			}
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, buildIoTProfileResponse(profile))
	}, RequireCapability(CapabilityIoTRegistryManage))

	iotGroup.DELETE("/profiles/:profileId", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		profileID := strings.TrimSpace(c.Param("profileId"))
		if profileID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "profileId is required")
		}
		if err := s.iotRegistry.DeleteProfile(c.Request().Context(), tenantID, profileID); err != nil {
			if errors.Is(err, iotstore.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "profile not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityIoTRegistryManage))

	resourceGroup.GET("/leases", func(c echo.Context) error {
		scopeRef := s.leaseScopeRef(c)
		state := c.QueryParam("state")
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		leases, err := leaseSvc.ListLeases(c.Request().Context(), scopeRef, state, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, leases)
	}, RequireCapability(CapabilityLeaseRead))

	resourceGroup.GET("/leases/insights", s.handleLeaseInsights(leaseSvc, poolSvc), RequireCapability(CapabilityLeaseRead))

	resourceGroup.GET("/leases/history", s.handleLeaseHistoryList(leaseSvc), RequireCapability(CapabilityLeaseRead))

	adminGroup.POST("/leases/:leaseId/release", func(c echo.Context) error {
		return s.handleLeaseAction(c, leaseSvc, "release")
	}, RequireCapability(CapabilityLeaseManage))

	adminGroup.POST("/leases/:leaseId/decline", func(c echo.Context) error {
		return s.handleLeaseAction(c, leaseSvc, "decline")
	}, RequireCapability(CapabilityLeaseManage))

	adminGroup.POST("/leases/:leaseId/cooldown/clear", func(c echo.Context) error {
		scopeRef := s.leaseScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		leaseID := c.Param("leaseId")
		ctx := c.Request().Context()
		leaseObj, changed, err := leaseSvc.ClearCooldown(ctx, scopeRef, leaseID)
		if err != nil {
			if errors.Is(err, lease.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "lease not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !changed {
			c.Response().Header().Set("Warning", `299 - "lease not in cooldown"`)
		} else {
			actor := s.actorFromContext(c)
			payload := leaseLifecyclePayload(leaseObj, "cooldown_clear", actor, "")
			s.recordAudit(ctx, tenantID, actor, "lease.cooldown_clear", payload, withResource("lease"))
		}
		steps := buildLeaseOperationSteps(leaseObj, "cooldown_clear", "", changed)
		status := "success"
		if !changed {
			status = "noop"
		}
		envelope := s.buildOperationResponse(ctx, scopeRef, "lease.cooldown.clear", status, leaseObj, changed, steps)
		return c.JSON(http.StatusOK, envelope)
	}, RequireCapability(CapabilityLeaseManage))

	adminGroup.POST("/leases/:leaseId/security-state", func(c echo.Context) error {
		scopeRef := s.leaseScopeRef(c)
		tenantID := scopeRef.TenantOrDefault()
		leaseID := c.Param("leaseId")
		var stateReq struct {
			State string `json:"state"`
		}
		if err := c.Bind(&stateReq); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if strings.TrimSpace(stateReq.State) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "state is required")
		}
		ctx := c.Request().Context()
		leaseObj, previous, changed, err := leaseSvc.UpdateLeaseSecurityState(ctx, scopeRef, leaseID, stateReq.State)
		if err != nil {
			if errors.Is(err, lease.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "lease not found")
			}
			if errors.Is(err, lease.ErrInvalidSecurityState) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid security state")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if !changed {
			c.Response().Header().Set("Warning", `299 - "security state unchanged"`)
		}
		actor := s.actorFromContext(c)
		auditPayload := leaseLifecyclePayload(leaseObj, "security_state", actor, "")
		auditPayload.PreviousState = previous
		s.recordAudit(ctx, tenantID, actor, "lease.security_state", auditPayload, withResource("lease"))
		steps := buildSecurityStateSteps(leaseObj, previous, stateReq.State, changed)
		status := "success"
		if !changed {
			status = "noop"
		}
		envelope := s.buildOperationResponse(ctx, scopeRef, "lease.security_state", status, leaseObj, changed, steps)
		return c.JSON(http.StatusOK, envelope)
	}, RequireCapability(CapabilityLeaseManage))

	adminGroup.GET("/prefix-leases", func(c echo.Context) error {
		scopeRef := s.leaseScopeRef(c)
		state := c.QueryParam("state")
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		leases, err := leaseSvc.ListPrefixLeases(c.Request().Context(), scopeRef, state, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, leases)
	}, RequireCapability(CapabilityLeaseRead))

	adminGroup.POST("/prefix-leases/:prefixLeaseId/release", func(c echo.Context) error {
		return s.handlePrefixLeaseAction(c, leaseSvc, "release")
	}, RequireCapability(CapabilityLeaseManage))

	adminGroup.POST("/prefix-leases/:prefixLeaseId/decline", func(c echo.Context) error {
		return s.handlePrefixLeaseAction(c, leaseSvc, "decline")
	}, RequireCapability(CapabilityLeaseManage))

	reportGroup := adminGroup.Group("/reports", RequireCapability(CapabilityReportRead))
	reportGroup.GET("/monthly-usage", s.handleReportMonthlyUsage)
	reportGroup.GET("/security", s.handleReportSecurityCompliance)
	reportGroup.GET("/capacity", s.handleReportCapacityPlanning)
	reportGroup.GET("/audit-trail", s.handleReportAuditTrail)

	adminGroup.POST("/leases/history/export", s.handleLeaseHistoryExport(), RequireCapabilities(CapabilityLeaseRead, CapabilityReportRead))

	resourceGroup.GET("/login-audit", func(c echo.Context) error {
		if s.auditSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "audit service unavailable")
		}
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		filter := audit.ListEventsFilter{Limit: page.Limit, Offset: page.Offset, ExcludeActions: []string{"admin.activity"}}
		if operationType := strings.TrimSpace(c.QueryParam("operationType")); operationType != "" {
			if strings.HasSuffix(operationType, ".*") {
				filter.ActionPrefixes = []string{strings.TrimSuffix(operationType, "*")}
			} else if strings.HasSuffix(operationType, "*") {
				filter.ActionPrefixes = []string{strings.TrimSuffix(operationType, "*")}
			} else {
				filter.Actions = []string{operationType}
			}
		}
		if actor := strings.TrimSpace(c.QueryParam("username")); actor != "" {
			filter.Actor = actor
		}
		if correlationID := strings.TrimSpace(c.QueryParam("requestId")); correlationID != "" {
			filter.CorrelationID = correlationID
		}
		resultFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("result")))
		onlyFailed := resultFilter == "failed"
		onlySuccess := resultFilter == "success"
		if start := strings.TrimSpace(c.QueryParam("startAt")); start != "" {
			if ts, err := time.Parse(time.RFC3339, start); err == nil {
				filter.StartAt = &ts
			} else {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid startAt")
			}
		}
		if end := strings.TrimSpace(c.QueryParam("endAt")); end != "" {
			if ts, err := time.Parse(time.RFC3339, end); err == nil {
				filter.EndAt = &ts
			} else {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid endAt")
			}
		}
		total, err := s.auditSvc.CountEventsFiltered(ctx, tenantID, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		events, err := s.auditSvc.ListEventsFiltered(ctx, tenantID, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		type loginAuditRecord struct {
			ID               string         `json:"id"`
			Username         string         `json:"username"`
			OperationType    string         `json:"operationType,omitempty"`
			ResourceType     string         `json:"resourceType,omitempty"`
			ResourceID       string         `json:"resourceId,omitempty"`
			ResourceName     string         `json:"resourceName,omitempty"`
			ResourceLink     string         `json:"resourceLink,omitempty"`
			ResourceSummary  string         `json:"resourceSummary,omitempty"`
			OperationSummary string         `json:"operationSummary,omitempty"`
			IP               string         `json:"ip"`
			Result           string         `json:"result"`
			Reason           string         `json:"reason,omitempty"`
			SessionID        string         `json:"sessionId,omitempty"`
			RequestID        string         `json:"requestId,omitempty"`
			TraceID          string         `json:"traceId,omitempty"`
			Location         string         `json:"location,omitempty"`
			UserAgent        string         `json:"userAgent,omitempty"`
			Before           map[string]any `json:"before,omitempty"`
			After            map[string]any `json:"after,omitempty"`
			Diff             map[string]any `json:"diff,omitempty"`
			RawPayload       map[string]any `json:"rawPayload,omitempty"`
			CreatedAt        time.Time      `json:"createdAt"`
		}
		toString := func(m map[string]any, key string) string {
			if v, ok := m[key]; ok {
				switch val := v.(type) {
				case string:
					return val
				case fmt.Stringer:
					return val.String()
				case []byte:
					return string(val)
				}
			}
			return ""
		}
		toInt := func(m map[string]any, key string) int {
			if v, ok := m[key]; ok {
				switch val := v.(type) {
				case int:
					return val
				case int32:
					return int(val)
				case int64:
					return int(val)
				case float64:
					return int(val)
				case json.Number:
					if iv, err := val.Int64(); err == nil {
						return int(iv)
					}
				case string:
					if iv, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
						return iv
					}
				}
			}
			return 0
		}
		containsCI := func(value, keyword string) bool {
			if keyword == "" {
				return true
			}
			return strings.Contains(strings.ToLower(strings.TrimSpace(value)), strings.ToLower(strings.TrimSpace(keyword)))
		}
		toMap := func(value any) map[string]any {
			switch typed := value.(type) {
			case map[string]any:
				return typed
			case map[string]string:
				result := make(map[string]any, len(typed))
				for key, item := range typed {
					result[key] = item
				}
				return result
			default:
				if typed == nil {
					return nil
				}
				data, err := json.Marshal(typed)
				if err != nil {
					return nil
				}
				parsed := map[string]any{}
				if err := json.Unmarshal(data, &parsed); err != nil {
					return nil
				}
				return parsed
			}
		}
		firstNonEmpty := func(values ...string) string {
			for _, value := range values {
				if text := strings.TrimSpace(value); text != "" {
					return text
				}
			}
			return ""
		}
		resourceKeyword := strings.TrimSpace(c.QueryParam("resourceKeyword"))
		ipKeyword := strings.TrimSpace(c.QueryParam("ip"))
		idKeyword := strings.TrimSpace(c.QueryParam("keyword"))
		records := make([]loginAuditRecord, 0, len(events))
		for _, evt := range events {
			payload := map[string]any{}
			if len(evt.Payload) > 0 {
				_ = json.Unmarshal(evt.Payload, &payload)
			}
			ip := toString(payload, "ip")
			if ip == "" {
				ip = toString(payload, "remoteAddr")
			}
			requestID := toString(payload, "requestId")
			if requestID == "" {
				requestID = toString(payload, "correlationId")
			}
			if requestID == "" {
				requestID = evt.CorrelationID
			}
			sessionID := toString(payload, "sessionId")
			traceID := toString(payload, "traceId")
			before := toMap(payload["before"])
			after := toMap(payload["after"])
			diff := toMap(payload["diff"])
			resourceType := firstNonEmpty(evt.Resource, toString(payload, "resource"), toString(payload, "resourceType"))
			resourceID := firstNonEmpty(
				toString(payload, "identifier"),
				toString(payload, "roleId"),
				toString(payload, "roleName"),
				toString(payload, "userId"),
				toString(payload, "poolId"),
				toString(payload, "scopeId"),
				toString(payload, "nodeId"),
				toString(payload, "templateId"),
				toString(payload, "configKey"),
				toString(payload, "username"),
				toString(payload, "id"),
				toString(after, "userId"),
				toString(before, "userId"),
				toString(after, "username"),
				toString(before, "username"),
				toString(after, "templateId"),
				toString(before, "templateId"),
			)
			resourceName := firstNonEmpty(
				toString(payload, "name"),
				toString(payload, "roleName"),
				toString(payload, "displayName"),
				toString(payload, "username"),
				toString(payload, "email"),
				toString(payload, "scopeName"),
				toString(payload, "templateName"),
				toString(payload, "poolName"),
				toString(payload, "nodeName"),
				toString(payload, "configName"),
				toString(after, "displayName"),
				toString(before, "displayName"),
				toString(after, "username"),
				toString(before, "username"),
				toString(after, "templateName"),
				toString(before, "templateName"),
			)
			if resourceName == "" {
				resourceName = firstNonEmpty(toString(after, "name"), toString(before, "name"), toString(after, "id"), toString(before, "id"))
			}
			resourceSummary := ""
			resourceLink := ""
			if strings.Contains(strings.ToLower(resourceType), "pool") || strings.HasPrefix(evt.Action, "pool.") {
				cidr := firstNonEmpty(toString(after, "cidr"), toString(before, "cidr"), toString(payload, "cidr"))
				if resourceName != "" && cidr != "" {
					resourceSummary = resourceName + " (" + cidr + ")"
				} else {
					resourceSummary = firstNonEmpty(resourceName, cidr)
				}
				resourceLink = "/pool/ipv4"
			} else if strings.Contains(strings.ToLower(resourceType), "cluster") || strings.Contains(strings.ToLower(resourceType), "ha") || strings.HasPrefix(evt.Action, "cluster.") {
				nodeIP := firstNonEmpty(toString(payload, "ip"), toString(after, "ip"), toString(before, "ip"), toString(payload, "nodeIp"))
				resourceSummary = firstNonEmpty(resourceName, nodeIP, resourceID)
				resourceLink = "/cluster/overview"
			} else if strings.Contains(strings.ToLower(resourceType), "config") || strings.HasPrefix(evt.Action, "ops.system") || strings.HasPrefix(evt.Action, "system.") {
				configKey := firstNonEmpty(toString(payload, "configKey"), resourceID)
				if configKey == "" {
					changes := toMap(payload["changes"])
					for key := range changes {
						configKey = key
						break
					}
				}
				resourceSummary = firstNonEmpty(resourceName, configKey)
				resourceLink = "/system/login-audit"
			} else if strings.Contains(strings.ToLower(resourceType), "auth") || strings.HasPrefix(evt.Action, "auth.user.") {
				resourceSummary = firstNonEmpty(resourceName, resourceID)
				resourceLink = "/system/user"
			} else if strings.Contains(strings.ToLower(resourceType), "rbac") || strings.HasPrefix(evt.Action, "rbac.role.") {
				resourceSummary = firstNonEmpty(resourceName, resourceID)
				resourceLink = "/system/user"
			} else {
				resourceSummary = firstNonEmpty(resourceName, resourceID)
			}
			if !containsCI(ip, ipKeyword) {
				continue
			}
			if resourceKeyword != "" {
				if !containsCI(resourceName, resourceKeyword) && !containsCI(resourceID, resourceKeyword) && !containsCI(resourceType, resourceKeyword) && !containsCI(resourceSummary, resourceKeyword) {
					continue
				}
			}
			if idKeyword != "" {
				idMatched :=
					containsCI(evt.AuditID, idKeyword) ||
						containsCI(requestID, idKeyword) ||
						containsCI(sessionID, idKeyword) ||
						containsCI(traceID, idKeyword)
				if !idMatched {
					continue
				}
			}
			result := toString(payload, "result")
			if result == "" {
				if strings.Contains(evt.Action, ".failed") {
					result = "failed"
				} else if statusCode := toInt(payload, "statusCode"); statusCode >= http.StatusBadRequest {
					result = "failed"
				} else {
					result = "success"
				}
			}
			if onlyFailed && !strings.EqualFold(result, "failed") {
				continue
			}
			if onlySuccess && !strings.EqualFold(result, "success") {
				continue
			}
			reason := toString(payload, "reason")
			if reason == "" {
				reason = toString(payload, "message")
			}
			if reason == "" {
				reason = toString(payload, "path")
			}
			records = append(records, loginAuditRecord{
				ID:               evt.AuditID,
				Username:         normalizeAuditActor(firstNonEmpty(evt.Actor, toString(payload, "username"), toString(after, "username"), toString(before, "username"))),
				OperationType:    evt.Action,
				ResourceType:     resourceType,
				ResourceID:       resourceID,
				ResourceName:     resourceName,
				ResourceLink:     resourceLink,
				ResourceSummary:  resourceSummary,
				OperationSummary: firstNonEmpty(reason, resourceSummary),
				IP:               ip,
				Result:           result,
				Reason:           reason,
				SessionID:        sessionID,
				RequestID:        requestID,
				TraceID:          traceID,
				Location:         toString(payload, "location"),
				UserAgent: func() string {
					ua := toString(payload, "userAgent")
					if ua == "" {
						ua = toString(payload, "ua")
					}
					return ua
				}(),
				Before:     before,
				After:      after,
				Diff:       diff,
				RawPayload: payload,
				CreatedAt:  evt.CreatedAt,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"code":    0,
			"message": "",
			"data": map[string]any{
				"items":    records,
				"total":    total,
				"page":     (page.Offset / page.Limit) + 1,
				"pageSize": page.Limit,
			},
		})
	}, RequireCapability(CapabilityAuditRead))

	resourceGroup.GET("/audit", func(c echo.Context) error {
		if s.auditSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "audit service unavailable")
		}
		_, tenantID := s.resourceScopeContext(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		filter := audit.ListEventsFilter{Limit: page.Limit, Offset: page.Offset}
		useFilter := false
		if actor := strings.TrimSpace(c.QueryParam("actor")); actor != "" {
			filter.Actor = actor
			useFilter = true
		}
		if actionList := strings.TrimSpace(c.QueryParam("actions")); actionList != "" {
			parts := strings.Split(actionList, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					filter.Actions = append(filter.Actions, trimmed)
				}
			}
			if len(filter.Actions) > 0 {
				useFilter = true
			}
		}
		if resource := strings.TrimSpace(c.QueryParam("resource")); resource != "" {
			filter.Resource = resource
			useFilter = true
		}
		if corr := strings.TrimSpace(c.QueryParam("correlationId")); corr != "" {
			filter.CorrelationID = corr
			useFilter = true
		}
		var events []models.AuditEvent
		if useFilter {
			events, err = s.auditSvc.ListEventsFiltered(ctx, tenantID, filter)
		} else {
			events, err = s.auditSvc.ListEvents(ctx, tenantID, page.Limit, page.Offset)
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, events)
	}, RequireCapability(CapabilityAuditRead))
}

func (s *HTTPServer) handleTenantQuotaGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.tenantQuota == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "tenant quota service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		quota, err := s.tenantQuota.GetQuota(c.Request().Context(), tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, quota)
	}
}

func (s *HTTPServer) handleTenantQuotaUpdate() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.tenantQuota == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "tenant quota service disabled")
		}
		_, tenantID := s.resourceScopeContext(c)
		var payload struct {
			PoolLimit          *int `json:"poolLimit"`
			LeaseLimit         *int `json:"leaseLimit"`
			ClientLimit        *int `json:"clientLimit"`
			APIRequestLimit    *int `json:"apiRequestLimit"`
			AutomationJobLimit *int `json:"automationJobLimit"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if payload.PoolLimit == nil && payload.LeaseLimit == nil && payload.ClientLimit == nil && payload.APIRequestLimit == nil && payload.AutomationJobLimit == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "at least one limit must be provided")
		}
		validateNonNegative := func(value *int, field string) error {
			if value != nil && *value < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, field+" cannot be negative")
			}
			return nil
		}
		if err := validateNonNegative(payload.PoolLimit, "poolLimit"); err != nil {
			return err
		}
		if err := validateNonNegative(payload.LeaseLimit, "leaseLimit"); err != nil {
			return err
		}
		if err := validateNonNegative(payload.ClientLimit, "clientLimit"); err != nil {
			return err
		}
		if err := validateNonNegative(payload.APIRequestLimit, "apiRequestLimit"); err != nil {
			return err
		}
		if err := validateNonNegative(payload.AutomationJobLimit, "automationJobLimit"); err != nil {
			return err
		}
		ctx := c.Request().Context()
		quota, err := s.tenantQuota.GetQuota(ctx, tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if payload.PoolLimit != nil {
			quota.PoolLimit = *payload.PoolLimit
		}
		if payload.LeaseLimit != nil {
			quota.LeaseLimit = *payload.LeaseLimit
		}
		if payload.ClientLimit != nil {
			quota.ClientLimit = *payload.ClientLimit
		}
		if payload.APIRequestLimit != nil {
			quota.APIRequestLimit = *payload.APIRequestLimit
		}
		if payload.AutomationJobLimit != nil {
			quota.AutomationJobLimit = *payload.AutomationJobLimit
		}
		quota.TenantID = tenantID
		updated, err := s.tenantQuota.SaveQuota(ctx, quota)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "tenant_quota",
			Action:     "update",
			Identifier: tenantID,
			After: map[string]any{
				"tenantId":           tenantID,
				"poolLimit":          updated.PoolLimit,
				"leaseLimit":         updated.LeaseLimit,
				"clientLimit":        updated.ClientLimit,
				"apiRequestLimit":    updated.APIRequestLimit,
				"automationJobLimit": updated.AutomationJobLimit,
			},
			Fields: []string{"poolLimit", "leaseLimit", "clientLimit", "apiRequestLimit", "automationJobLimit"},
		}
		s.recordAudit(ctx, tenantID, actor, "tenant.quota.update", change, withResource("tenant_quota"))
		return c.JSON(http.StatusOK, updated)
	}
}

func (s *HTTPServer) handleListAssignments() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		principalID := strings.TrimSpace(c.QueryParam("principalId"))
		if principalID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "principalId is required")
		}
		roleName := strings.ToLower(strings.TrimSpace(c.QueryParam("roleName")))
		assignments, err := s.rbacRepo.ListAssignments(c.Request().Context(), principalID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		filtered := make([]rbac.Assignment, 0, len(assignments))
		for _, assignment := range assignments {
			if roleName != "" && !strings.Contains(strings.ToLower(strings.TrimSpace(assignment.RoleName)), roleName) {
				continue
			}
			filtered = append(filtered, assignment)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, map[string]any{"items": []rbac.Assignment{}, "total": total, "limit": page.Limit, "offset": page.Offset})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, map[string]any{"items": filtered[page.Offset:end], "total": total, "limit": page.Limit, "offset": page.Offset})
	}
}

func (s *HTTPServer) handleCreateAssignment() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		var payload struct {
			PrincipalID  string            `json:"principalId"`
			RoleName     string            `json:"roleName"`
			OrgUnitID    string            `json:"orgUnitId"`
			ResourceType string            `json:"resourceType"`
			ResourceID   string            `json:"resourceId"`
			ExpiresAt    string            `json:"expiresAt"`
			Attributes   map[string]string `json:"attributes"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		principalID := strings.TrimSpace(payload.PrincipalID)
		if principalID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "principalId is required")
		}
		roleName := strings.TrimSpace(payload.RoleName)
		if roleName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "roleName is required")
		}
		var expiresAt *time.Time
		if value := strings.TrimSpace(payload.ExpiresAt); value != "" {
			ts, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be RFC3339 timestamp")
			}
			expiresAt = &ts
		}
		attributesRaw, err := json.Marshal(payload.Attributes)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "attributes must be a valid object")
		}
		assignment := &rbac.Assignment{
			ID:           uuid.NewString(),
			PrincipalID:  principalID,
			RoleName:     roleName,
			OrgUnitID:    csvOptionalString(payload.OrgUnitID),
			ResourceType: csvOptionalString(payload.ResourceType),
			ResourceID:   csvOptionalString(payload.ResourceID),
			CreatedBy:    s.actorFromContext(c),
			CreatedAt:    time.Now().UTC(),
			Attributes:   bytesToRawJSON(attributesRaw),
			ExpiresAt:    expiresAt,
		}
		if err := s.rbacRepo.CreateAssignment(c.Request().Context(), assignment); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := assignment.CreatedBy
		change := auditpayload.ConfigChange{
			Resource:   "rbac_assignment",
			Action:     "create",
			Identifier: assignment.ID,
			After: map[string]any{
				"principalId": principalID,
				"roleName":    roleName,
			},
			Fields: []string{"principalId", "roleName"},
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), actor, "rbac.assignment.create", change, withResource("rbac_assignment"))
		return c.JSON(http.StatusCreated, assignment)
	}
}

func (s *HTTPServer) handleDeleteAssignment() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		assignmentID := strings.TrimSpace(c.Param("assignmentId"))
		if assignmentID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "assignmentId is required")
		}
		ctx := c.Request().Context()
		if err := s.rbacRepo.DeleteAssignment(ctx, assignmentID); err != nil {
			if errors.Is(err, rbac.ErrAssignmentNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		change := auditpayload.ConfigChange{
			Resource:   "rbac_assignment",
			Action:     "delete",
			Identifier: assignmentID,
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), actor, "rbac.assignment.delete", change, withResource("rbac_assignment"))
		return c.NoContent(http.StatusNoContent)
	}
}

type iotDeviceResponse struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenantId"`
	DeviceID       string            `json:"deviceId"`
	DisplayName    string            `json:"displayName,omitempty"`
	HardwareAddr   string            `json:"hardwareAddr,omitempty"`
	ProfileID      string            `json:"profileId,omitempty"`
	LeaseProfileID string            `json:"leaseProfileId,omitempty"`
	SleepClass     string            `json:"sleepClass"`
	SleepInterval  time.Duration     `json:"sleepInterval"`
	OfflineWindow  time.Duration     `json:"offlineWindow"`
	SleepyHint     bool              `json:"sleepyHint"`
	Status         string            `json:"status"`
	Firmware       string            `json:"firmwareVersion,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Metadata       json.RawMessage   `json:"metadata,omitempty"`
	LastSeen       *time.Time        `json:"lastSeen,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type iotProfileResponse struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenantId"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	SleepClass     string          `json:"sleepClass"`
	SleepInterval  time.Duration   `json:"sleepInterval"`
	OfflineWindow  time.Duration   `json:"offlineWindow"`
	LeaseProfileID string          `json:"leaseProfileId,omitempty"`
	SleepyCapable  bool            `json:"sleepyCapable"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func buildIoTDeviceResponses(devices []models.IoTDevice) []iotDeviceResponse {
	if len(devices) == 0 {
		return []iotDeviceResponse{}
	}
	out := make([]iotDeviceResponse, 0, len(devices))
	for _, device := range devices {
		out = append(out, buildIoTDeviceResponse(&device))
	}
	return out
}

func buildIoTDeviceResponse(device *models.IoTDevice) iotDeviceResponse {
	resp := iotDeviceResponse{
		ID:             device.ID,
		TenantID:       device.TenantID,
		DeviceID:       device.DeviceID,
		DisplayName:    device.DisplayName,
		HardwareAddr:   device.HardwareAddr,
		ProfileID:      device.ProfileID,
		LeaseProfileID: device.LeaseProfileID,
		SleepClass:     device.SleepClass,
		SleepInterval:  device.SleepInterval,
		OfflineWindow:  device.OfflineWindow,
		SleepyHint:     device.SleepyHint,
		Status:         device.Status,
		Firmware:       device.Firmware,
		Labels:         decodeLabelMap(device.Labels),
		Metadata:       bytesToRawJSON(device.Metadata),
		CreatedAt:      device.CreatedAt,
		UpdatedAt:      device.UpdatedAt,
	}
	if device.LastSeen != nil {
		clone := device.LastSeen.UTC()
		resp.LastSeen = &clone
	}
	return resp
}

func buildIoTProfileResponses(profiles []models.IoTDeviceProfile) []iotProfileResponse {
	if len(profiles) == 0 {
		return []iotProfileResponse{}
	}
	out := make([]iotProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, buildIoTProfileResponse(&profile))
	}
	return out
}

func buildIoTProfileResponse(profile *models.IoTDeviceProfile) iotProfileResponse {
	return iotProfileResponse{
		ID:             profile.ID,
		TenantID:       profile.TenantID,
		Name:           profile.Name,
		Description:    profile.Description,
		SleepClass:     profile.SleepClass,
		SleepInterval:  profile.SleepInterval,
		OfflineWindow:  profile.OfflineWindow,
		LeaseProfileID: profile.LeaseProfileID,
		SleepyCapable:  profile.SleepyCapable,
		Metadata:       bytesToRawJSON(profile.Metadata),
		CreatedAt:      profile.CreatedAt,
		UpdatedAt:      profile.UpdatedAt,
	}
}

func decodeLabelMap(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var labels map[string]string
	if err := json.Unmarshal(data, &labels); err != nil {
		return nil
	}
	return labels
}

func bytesToRawJSON(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	return json.RawMessage(buf)
}

func parseDurationField(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

func parseOptionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

func (s *HTTPServer) handleAPIVersions() echo.HandlerFunc {
	versions := append([]string(nil), s.apiVersions...)
	defaultVersion := s.defaultOpenAPIVersion()
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"versions": versions,
			"default":  defaultVersion,
		})
	}
}

func (s *HTTPServer) handleUIMetadata() echo.HandlerFunc {
	catalog := []string{
		CapabilityPolicyRead,
		CapabilityPolicyWrite,
		CapabilityPolicyBYODManage,
		CapabilityPoolRead,
		CapabilityPoolWrite,
		CapabilityBindingRead,
		CapabilityBindingManage,
		CapabilityLeaseRead,
		CapabilityLeaseManage,
		CapabilityReportRead,
		CapabilityAuditRead,
		CapabilityUserRead,
		CapabilityUserManage,
		CapabilityRBACRoleRead,
		CapabilityRBACRoleManage,
		CapabilityTenantQuotaRead,
		CapabilityTenantQuotaWrite,
		CapabilityRBACAssignmentRead,
		CapabilityRBACAssignmentWrite,
	}
	sort.Strings(catalog)
	defaultVersion := s.defaultOpenAPIVersion()
	versions := append([]string(nil), s.apiVersions...)
	strict := s.options.RBAC.EnforceCapabilities && !s.options.RBAC.ShadowMode
	uiOptions := s.options.UI
	return func(c echo.Context) error {
		currentCaps := capabilitiesFromContext(c)
		grantedSet := make(map[string]struct{}, len(currentCaps))
		tempSet := make(map[string]struct{})
		for capName := range currentCaps {
			name := strings.TrimSpace(capName)
			if name == "" {
				continue
			}
			if strings.HasPrefix(name, "temp.") {
				trimmed := strings.TrimPrefix(name, "temp.")
				if trimmed == "" {
					continue
				}
				tempSet[trimmed] = struct{}{}
				grantedSet[trimmed] = struct{}{}
				continue
			}
			grantedSet[name] = struct{}{}
		}
		granted := make([]string, 0, len(grantedSet))
		for name := range grantedSet {
			granted = append(granted, name)
		}
		sort.Strings(granted)
		temporary := make([]string, 0, len(tempSet))
		for name := range tempSet {
			temporary = append(temporary, name)
		}
		sort.Strings(temporary)
		catalogCopy := append([]string(nil), catalog...)
		return c.JSON(http.StatusOK, map[string]any{
			"apiVersions":    versions,
			"defaultVersion": defaultVersion,
			"capabilities": map[string]any{
				"granted":   granted,
				"temporary": temporary,
				"catalog":   catalogCopy,
			},
			"rbac": map[string]any{
				"strict":     strict,
				"shadowMode": s.options.RBAC.ShadowMode,
			},
			"ui": uiOptions,
		})
	}
}

func (s *HTTPServer) handleOpenAPIIndex() echo.HandlerFunc {
	return func(c echo.Context) error {
		type entry struct {
			Version string `json:"version"`
			Path    string `json:"path"`
		}
		base := strings.TrimSpace(s.options.API.OpenAPI.ServePath)
		if base == "" {
			base = "/openapi"
		}
		if !strings.HasPrefix(base, "/") {
			base = "/" + base
		}
		base = strings.TrimRight(base, "/")
		if base == "" {
			base = "/openapi"
		}
		records := make([]entry, 0, len(s.openAPISpecs))
		for version := range s.openAPISpecs {
			records = append(records, entry{Version: version, Path: fmt.Sprintf("%s/%s", base, version)})
		}
		sort.Slice(records, func(i, j int) bool { return records[i].Version < records[j].Version })
		return c.JSON(http.StatusOK, map[string]any{"versions": records})
	}
}

func (s *HTTPServer) handleOpenAPISpec() echo.HandlerFunc {
	return func(c echo.Context) error {
		version := normalizeAPIVersion(c.Param("version"))
		if version == "" {
			version = s.defaultOpenAPIVersion()
		}
		fileName, ok := s.openAPISpecs[version]
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "openapi spec not found")
		}
		data, err := fs.ReadFile(openAPIFS, fileName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		c.Response().Header().Set(echo.HeaderContentType, "application/yaml")
		if _, err := c.Response().Write(data); err != nil {
			return err
		}
		return nil
	}
}

func (s *HTTPServer) handleHealthz() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
		defer cancel()

		status := "ok"
		details := make(map[string]any)
		for _, hook := range s.options.HealthHooks {
			if hook == nil {
				continue
			}
			if err := hook.Check(ctx); err != nil {
				details[hook.Name()] = err.Error()
				status = "degraded"
			} else {
				details[hook.Name()] = "ok"
			}
		}
		if s.options.Coordinator != nil {
			details["failover"] = s.options.Coordinator.Snapshot()
		}
		resp := map[string]any{
			"status":  status,
			"time":    time.Now().UTC().Format(time.RFC3339),
			"details": details,
		}
		code := http.StatusOK
		if status != "ok" {
			code = http.StatusServiceUnavailable
		}
		return c.JSON(code, resp)
	}
}

func (s *HTTPServer) defaultOpenAPIVersion() string {
	preferred := normalizeAPIVersion(s.options.API.DefaultVersion)
	if preferred != "" {
		if _, ok := s.openAPISpecs[preferred]; ok {
			return preferred
		}
		if containsVersion(s.apiVersions, preferred) {
			return preferred
		}
	}
	for _, version := range s.apiVersions {
		if _, ok := s.openAPISpecs[version]; ok {
			return version
		}
	}
	if len(s.openAPISpecs) == 0 {
		return ""
	}
	versions := make([]string, 0, len(s.openAPISpecs))
	for version := range s.openAPISpecs {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions[0]
}

func normalizeAPIVersion(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func containsVersion(list []string, version string) bool {
	for _, v := range list {
		if v == version {
			return true
		}
	}
	return false
}

func sanitizeAPIVersions(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	versions := make([]string, 0, len(raw))
	for _, candidate := range raw {
		version := normalizeAPIVersion(candidate)
		if version == "" {
			continue
		}
		if _, ok := seen[version]; ok {
			continue
		}
		seen[version] = struct{}{}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return []string{"v1"}
	}
	return versions
}

func discoverOpenAPISpecs() map[string]string {
	entries, err := fs.ReadDir(openAPIFS, ".")
	if err != nil {
		return map[string]string{}
	}
	specs := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
			continue
		}
		version := normalizeAPIVersion(strings.TrimPrefix(strings.TrimSuffix(name, ".yaml"), "openapi_"))
		if version == "" {
			continue
		}
		specs[version] = name
	}
	return specs
}
func (s *HTTPServer) handleVisualizationTopology(c echo.Context) error {
	if s.visualization == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "visualization disabled")
	}
	scope := s.leaseScopeRef(c)
	if tenantID := strings.TrimSpace(c.QueryParam("tenantId")); tenantID != "" {
		scope = scope.WithTenantOverride(tenantID)
	}
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId or scope is required")
	}
	snapshot, err := s.visualization.Topology(c.Request().Context(), scope)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleVisualizationHeatmap(c echo.Context) error {
	if s.visualization == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "visualization disabled")
	}
	scope := s.leaseScopeRef(c)
	if tenantID := strings.TrimSpace(c.QueryParam("tenantId")); tenantID != "" {
		scope = scope.WithTenantOverride(tenantID)
	}
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId or scope is required")
	}
	snapshot, err := s.visualization.LeaseHeatmap(c.Request().Context(), scope)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

// HTTPServer wraps the Echo engine.
type HTTPServer struct {
	echo                          *echo.Echo
	logger                        *zap.Logger
	options                       Options
	automation                    *automation.Service
	automationApprovals           *approvals.Service
	automationApprovalPolicy      map[approvals.RequestType]struct{}
	workflowSvc                   *workflowsvc.Service
	opsSvc                        *ops.Service
	auditSvc                      *audit.Service
	authService                   *auth.Service
	leaseSvc                      *lease.Service
	poolSvc                       *pool.Service
	otelHTTPDuration              metric.Float64Histogram
	otelHTTPErrors                metric.Int64Counter
	haConfig                      config.HAConfig
	haConfigMu                    sync.RWMutex
	haLbConfig                    clusterHaLbConfigDTO
	haLbConfigMu                  sync.RWMutex
	failoverEvents                []clusterFailoverEvent
	failoverEventsMu              sync.RWMutex
	syncConfig                    clusterSyncConfigDTO
	syncConfigMu                  sync.RWMutex
	scalePolicy                   clusterScalePolicyDTO
	scalePolicyMu                 sync.RWMutex
	syncStatus                    []clusterSyncStatusDTO
	syncStatusMu                  sync.RWMutex
	syncTransactions              []clusterSyncTransactionDTO
	syncTransactionsArchive       []clusterSyncTransactionDTO
	syncTransactionsMu            sync.RWMutex
	syncResponderMu               sync.Mutex
	syncResponderListener         net.Listener
	syncResponderDone             chan struct{}
	syncResponderLastError        string
	syncTxnReconcileMu            sync.Mutex
	syncTxnReconcileCancel        context.CancelFunc
	syncTxnReconcileDone          chan struct{}
	syncTxnReconcileStateMu       sync.RWMutex
	syncTxnReconcileLastRun       string
	syncTxnReconcileLastErr       string
	syncTxnReconcileSuccessTotal  int
	syncTxnReconcileFailureTotal  int
	syncTxnReconcileFailureStreak int
	syncTxnReconcileLastAlertAt   time.Time
	scaleEvents                   []clusterFailoverEvent
	scaleEventsMu                 sync.RWMutex
	failoverPlans                 map[string]clusterFailoverPlanDTO
	failoverPlansMu               sync.RWMutex
	backupPlan                    clusterBackupPlanDTO
	backupPlanMu                  sync.RWMutex
	optimizationPlan              clusterOptimizationPlanDTO
	optimizationPlanMu            sync.RWMutex
	backupHistory                 []clusterBackupRecordDTO
	backupHistoryMu               sync.RWMutex
	clusterControlPlane           clusterControlPlaneDTO
	clusterControlPlaneMu         sync.RWMutex
	clusterControlMembers         map[string]clusterControlMemberDTO
	clusterControlMembersMu       sync.RWMutex
	clusterCommands               []clusterCommandDTO
	clusterCommandsMu             sync.RWMutex
	clusterNodes                  map[string]clusterOverviewNode
	clusterNodesMu                sync.RWMutex
	clusterNodeAuth               map[string]string
	clusterNodeAuthMu             sync.RWMutex
	securityPolicy                *securitypolicy.Service
	macListSvc                    *maclist.Service
	monitor                       *monitoring.Aggregator
	alertFeed                     *monitoring.AlertFeed
	alertManager                  *alerting.Manager
	alertController               *monitoring.AlertController
	alertRoutes                   *alerting.RoutingStore
	alertRuleRepo                 alerting.RuleStore
	alertRouteRepo                alerting.RouteStore
	alertConfigRepo               alerting.ConfigStore
	alertTemplateRepo             alerting.TemplateStore
	alertReceiverRepo             alerting.ReceiverStore
	dutySchedule                  *alerting.DutySchedule
	notificationDispatcher        *notifications.Dispatcher
	notificationChannels          []notificationChannelDTO
	smsMu                         sync.RWMutex
	smsConfig                     smsConfig
	webhookMu                     sync.RWMutex
	webhookConfig                 webhookConfig
	alertConfigMu                 sync.RWMutex
	alertThresholdStore           map[string]alertThresholdConfig
	alertNotifyStore              map[string]alertNotifyConfig
	alertTemplateStore            map[string][]alertTemplateDTO
	alertReceiverStore            map[string][]alertReceiverDTO
	dashboardSvc                  *dashboard.Service
	reportSvc                     *reporting.Service
	visualization                 *visualization.Service
	haSvc                         *ha.Service
	haOnce                        sync.Once
	collabHub                     *collab.Hub
	iotRegistry                   *iotregistry.Service
	auth                          *Authenticator
	rateLimit                     *rateLimiter
	quota                         *quotaEnforcer
	rbacResolver                  *rbac.Resolver
	tenantQuota                   *tenant.Service
	rbacRepo                      rbac.Repository
	jwt                           *JWTValidator
	apiVersions                   []string
	openAPISpecs                  map[string]string
	superAdmin                    *superadmin.Manager
	superAdminUsername            string
	superAdminAPIKey              string
	optionRepo                    OptionRepository
	templateRepo                  TemplateRepository
	scopeRepo                     ScopeRepository
	templateApplyHistoryRepo      TemplateApplyHistoryRepository
	optionStore                   *dhcpOptionStore
	templateStore                 *dhcpTemplateStore
	scopeStore                    *dhcpOptionScopeStore
	accessSecurityStore           *accessSecurityStore
	bootTime                      time.Time
	routeUsage                    *RouteUsageRecorder
	alertRuleStore                map[string][]alertRuleDTO
	reportTasks                   map[string][]reportTaskDTO
	alertRuleMu                   sync.Mutex
	notificationMu                sync.Mutex
	reportMu                      sync.Mutex
	smtpMu                        sync.RWMutex
	smtpConfig                    smtpConfig
	lastSMTPTest                  time.Time
	configStore                   ClusterConfigStore
	joinJobs                      map[string]clusterJoinJobDTO
	joinJobsMu                    sync.RWMutex
	joinCancels                   map[string]context.CancelFunc
	joinCancelsMu                 sync.Mutex
	loginGuardMu                  sync.Mutex
	loginGuards                   map[string]loginGuardState
	loginCaptcha                  map[string]loginCaptchaChallenge
	passwordResetMu               sync.Mutex
	passwordResetChallenges       map[string]passwordResetChallenge
	apiTLSRuntimeActive           bool
}

// NewHTTPServer configures the Echo runtime and registers all API routes.
func NewHTTPServer(logger *zap.Logger, opts Options, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, auditSvc *audit.Service, reportSvc *reporting.Service, vizSvc *visualization.Service, iotSvc *iotregistry.Service, collabHub *collab.Hub, quotaSvc *tenant.Service, rbacRepo rbac.Repository, authSvc *auth.Service, macListSvc *maclist.Service) (*HTTPServer, error) {
	var otelDuration metric.Float64Histogram
	var otelErrors metric.Int64Counter

	if leaseSvc == nil {
		return nil, fmt.Errorf("http server: lease service required")
	}
	if policySvc == nil {
		return nil, fmt.Errorf("http server: policy service required")
	}
	if securityPolicySvc == nil {
		return nil, fmt.Errorf("http server: security policy service required")
	}
	if poolSvc == nil {
		return nil, fmt.Errorf("http server: pool service required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c == nil {
			return
		}
		if c.Response().Committed {
			return
		}
		reqCtx := c.Request().Context()
		status := http.StatusInternalServerError
		message := "internal server error"
		var he *echo.HTTPError
		if errors.As(err, &he) {
			status = he.Code
			if he.Message != nil {
				message = fmt.Sprint(he.Message)
			}
		}
		requestID := c.Response().Header().Get(echo.HeaderXRequestID)
		if requestID == "" {
			requestID = c.Request().Header.Get(echo.HeaderXRequestID)
		}
		payload := map[string]any{
			"status":    "error",
			"code":      status,
			"message":   message,
			"path":      c.Path(),
			"method":    c.Request().Method,
			"requestId": requestID,
		}
		if status == http.StatusNotFound {
			payload["hint"] = "检查 API 前缀 (/api/v1/core) 与路由是否启用"
			payload["suggestion"] = "确认路径、方法与租户/权限头部是否正确"
			payload["route"] = c.Path()
			payload["uri"] = c.Request().RequestURI
		}
		log := LoggerFromContext(reqCtx, logger)
		log.Error("http request error",
			zap.Int("status", status),
			zap.String("method", c.Request().Method),
			zap.String("path", c.Path()),
			zap.String("uri", c.Request().RequestURI),
			zap.String("requestId", requestID),
			zap.String("traceId", traceIDFromContext(reqCtx)),
			zap.String("spanId", spanIDFromContext(reqCtx)),
			zap.Error(err),
		)
		_ = c.JSON(status, payload)
	}
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(requestIDMiddleware())
	e.Use((&HTTPServer{logger: logger}).loggerMiddleware())
	e.Use(auditStatusMiddleware())
	e.Use(middleware.Secure())
	if opts.Telemetry.Enabled {
		serviceName := strings.TrimSpace(opts.Telemetry.ServiceName)
		if serviceName == "" {
			serviceName = "modern-dhcp"
		}
		e.Use(otelecho.Middleware(serviceName))
		e.Use(traceContextHeaderMiddleware())
		meter := otel.GetMeterProvider().Meter(serviceName)
		otelDuration, _ = meter.Float64Histogram("http.server.duration", metric.WithUnit("s"))
		otelErrors, _ = meter.Int64Counter("http.server.errors")
	}
	if opts.CORS.Enabled {
		corsCfg := middleware.CORSConfig{
			AllowOrigins:     opts.CORS.AllowedOrigins,
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-API-Key", "X-Tenant-ID", "X-CSRF-Token"},
			AllowCredentials: true,
		}
		if len(corsCfg.AllowOrigins) == 0 {
			corsCfg.AllowOrigins = []string{"*"}
		}
		e.Use(middleware.CORSWithConfig(corsCfg))
	}
	jwtValidator, err := NewJWTValidator(opts.OAuth)
	if err != nil {
		return nil, fmt.Errorf("http server: oauth setup failed: %w", err)
	}
	tokenProvider := opts.TokenProvider
	if tokenProvider == nil && authSvc != nil {
		tokenProvider = authSvc
	}
	coordinator := opts.Coordinator
	if coordinator == nil && opts.FailoverController != nil {
		coordinator = opts.FailoverController
	}
	opts.Coordinator = coordinator

	dashboardSvc := dashboard.NewService(dashboard.Options{
		Aggregator:   opts.Monitoring.Aggregator,
		AlertFeed:    opts.Monitoring.AlertFeed,
		Automation:   opts.Automation,
		Audit:        auditSvc,
		Coordinator:  coordinator,
		HealthHooks:  adaptDashboardHooks(opts.HealthHooks),
		Logger:       logger,
		CacheTTL:     opts.Dashboard.CacheTTL,
		StreamLimit:  opts.Dashboard.DefaultStreamLimit,
		StreamMax:    opts.Dashboard.MaxStreamLimit,
		HotspotLimit: opts.Dashboard.HotspotLimit,
	})

	var haService *ha.Service
	if opts.FailoverController != nil {
		haService = ha.NewService(ha.Options{Controller: opts.FailoverController, RunbookPaths: opts.HARunbooks, Logger: logger})
	}

	server := &HTTPServer{
		echo:                     e,
		logger:                   logger,
		options:                  opts,
		otelHTTPDuration:         otelDuration,
		otelHTTPErrors:           otelErrors,
		haConfig:                 opts.HAConfig,
		haLbConfig:               defaultClusterHaLbFromConfig(opts.HAConfig),
		failoverEvents:           []clusterFailoverEvent{},
		syncConfig:               defaultClusterSyncConfig(),
		scalePolicy:              defaultClusterScalePolicy(),
		syncStatus:               []clusterSyncStatusDTO{},
		syncTransactions:         []clusterSyncTransactionDTO{},
		scaleEvents:              []clusterFailoverEvent{},
		backupPlan:               defaultClusterBackupPlan(),
		optimizationPlan:         defaultClusterOptimizationPlan(),
		backupHistory:            []clusterBackupRecordDTO{},
		clusterControlPlane:      defaultClusterControlPlane(),
		clusterControlMembers:    make(map[string]clusterControlMemberDTO),
		clusterNodes:             make(map[string]clusterOverviewNode),
		clusterNodeAuth:          make(map[string]string),
		automation:               opts.Automation,
		automationApprovals:      opts.AutomationApprovals,
		automationApprovalPolicy: opts.AutomationApprovalPolicy,
		workflowSvc:              opts.Workflow,
		opsSvc:                   ops.NewService(opts.OpsSupport, opts.OpsSettingsStore, logger),
		auditSvc:                 auditSvc,
		authService:              authSvc,
		leaseSvc:                 leaseSvc,
		poolSvc:                  poolSvc,
		securityPolicy:           securityPolicySvc,
		macListSvc:               macListSvc,
		monitor:                  opts.Monitoring.Aggregator,
		alertFeed:                opts.Monitoring.AlertFeed,
		alertManager:             opts.Monitoring.AlertManager,
		alertController:          opts.Monitoring.AlertController,
		alertRoutes:              opts.Alerting.Routes,
		alertRuleRepo:            opts.Alerting.RuleStore,
		alertRouteRepo:           opts.Alerting.RouteStore,
		alertConfigRepo:          opts.Alerting.ConfigStore,
		alertTemplateRepo:        opts.Alerting.TemplateStore,
		alertReceiverRepo:        opts.Alerting.ReceiverStore,
		dutySchedule:             opts.Alerting.Schedule,
		notificationDispatcher:   opts.Alerting.Dispatcher,
		alertThresholdStore:      make(map[string]alertThresholdConfig),
		alertNotifyStore:         make(map[string]alertNotifyConfig),
		alertTemplateStore:       make(map[string][]alertTemplateDTO),
		alertReceiverStore:       make(map[string][]alertReceiverDTO),
		dashboardSvc:             dashboardSvc,
		reportSvc:                reportSvc,
		visualization:            vizSvc,
		haSvc:                    haService,
		iotRegistry:              iotSvc,
		collabHub:                collabHub,
		auth:                     NewAuthenticator(opts.RequireAuth, opts.APIKeys, jwtValidator, tokenProvider),
		rateLimit:                newRateLimiter(opts.API.RateLimit),
		quota:                    newQuotaEnforcer(opts.API.Quota),
		rbacResolver:             opts.RBAC.Resolver,
		tenantQuota:              quotaSvc,
		rbacRepo:                 rbacRepo,
		jwt:                      jwtValidator,
		apiVersions:              sanitizeAPIVersions(opts.API.Versions),
		openAPISpecs:             discoverOpenAPISpecs(),
		superAdmin:               opts.SuperAdmin.Manager,
		superAdminUsername:       opts.SuperAdmin.Username,
		superAdminAPIKey:         opts.SuperAdmin.APIKey,
		optionRepo:               opts.OptionRepo,
		templateRepo:             opts.TemplateRepo,
		scopeRepo:                opts.ScopeRepo,
		templateApplyHistoryRepo: opts.TemplateApplyHistoryRepo,
		optionStore:              newDHCPOptionStore(),
		configStore:              opts.ClusterConfigStore,
		templateStore:            newDHCPTemplateStore(),
		scopeStore:               newDHCPOptionScopeStore(),
		accessSecurityStore:      newAccessSecurityStore(),
		bootTime:                 time.Now().UTC(),
		routeUsage:               NewRouteUsageRecorder(),
		alertRuleStore:           make(map[string][]alertRuleDTO),
		reportTasks:              make(map[string][]reportTaskDTO),
		smtpConfig:               smtpConfig{UseTLS: true, Port: 587},
		failoverPlans:            make(map[string]clusterFailoverPlanDTO),
		clusterCommands:          make([]clusterCommandDTO, 0),
		joinJobs:                 make(map[string]clusterJoinJobDTO),
		joinCancels:              make(map[string]context.CancelFunc),
		loginGuards:              make(map[string]loginGuardState),
		loginCaptcha:             make(map[string]loginCaptchaChallenge),
		passwordResetChallenges:  make(map[string]passwordResetChallenge),
	}
	if server.options.JoinJobExecutor == nil {
		server.options.JoinJobExecutor = &runtimeJoinJobExecutor{server: server}
	}
	server.bootstrapAlertConfig()
	if server.openAPISpecs == nil {
		server.openAPISpecs = make(map[string]string)
	}
	if server.configStore == nil {
		server.configStore = noopClusterConfigStore{}
	}
	server.restoreClusterConfigs(context.Background())
	if server.alertRoutes == nil {
		server.alertRoutes = alerting.NewRoutingStore(nil, "bootstrap")
	}
	if server.dutySchedule == nil {
		server.dutySchedule = alerting.NewDutySchedule(nil, "bootstrap")
	}
	if server.alertController != nil && server.alertRuleRepo != nil {
		ctx := context.Background()
		for _, tenantID := range server.alertController.Tenants() {
			server.reloadAlertControllerRules(ctx, tenantID)
		}
	}
	server.loadSMTPConfig()
	server.loadSMSConfig()
	server.loadWebhookConfig()
	server.loadNotificationChannels()
	if server.superAdmin != nil && server.superAdminUsername == "" {
		server.superAdminUsername = superadmin.DefaultUsername
	}
	server.hydrateCustomOptions()
	server.hydrateTemplateConfigs()
	server.hydrateOptionScopes()
	e.Use(server.enrichRequestContext)
	if auditSvc != nil {
		e.Use(server.auditAccessMiddleware)
	}
	e.Use(server.instrumentationMiddleware)
	e.Use(server.routeUsageMiddleware())
	server.registerRoutes(leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
	server.registerHARoutes()
	server.refreshClusterSyncResponder()
	server.startSyncTransactionsReconciler()
	return server, nil
}

func (s *HTTPServer) routeUsageMiddleware() echo.MiddlewareFunc {
	if s == nil || s.routeUsage == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}
	}
	recorderMiddleware := s.routeUsage.Middleware()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		wrapped := recorderMiddleware(next)
		return func(c echo.Context) error {
			if skipRouteUsagePath(c.Path()) {
				return next(c)
			}
			return wrapped(c)
		}
	}
}

func skipRouteUsagePath(path string) bool {
	if path == "" {
		return false
	}
	if path == "/" {
		return true
	}
	skipPrefixes := []string{"/openapi", "/console", "/static", "/assets"}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (s *HTTPServer) registerRoutes(leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, iotSvc *iotregistry.Service) {
	if s == nil || s.echo == nil {
		return
	}
	e := s.echo

	e.GET("/healthz", s.handleHealthz())
	e.GET("/api/versions", s.handleAPIVersions())
	e.GET("/api/ui", s.handleUIMetadata())

	if s.options.API.OpenAPI.Enabled {
		base := strings.TrimSpace(s.options.API.OpenAPI.ServePath)
		if base == "" {
			base = "/openapi"
		}
		if !strings.HasPrefix(base, "/") {
			base = "/" + base
		}
		base = strings.TrimRight(base, "/")
		if base == "" {
			base = "/openapi"
		}
		e.GET(base, s.handleOpenAPIIndex())
		e.GET(base+"/:version", s.handleOpenAPISpec())
	}

	e.GET("/", s.serveConsoleAsset("ui/policy_selector.html"))
	e.GET("/console/policy-selector", s.serveConsoleAsset("ui/policy_selector.html"))
	e.GET("/console/lease-security", s.serveConsoleAsset("ui/lease_security.html"))
	e.GET("/console/prefix-delegations", s.serveConsoleAsset("ui/prefix_delegations.html"))

	versions := s.apiVersions
	if len(versions) == 0 {
		versions = []string{"v1"}
	}
	for _, version := range versions {
		base := fmt.Sprintf("/api/%s", version)
		versionRoot := e.Group(base)
		versionRoot.GET("", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]any{
				"status":  "ok",
				"version": version,
			})
		})
		s.mountAPIRoutes(versionRoot, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
	}

	// Provide legacy compatibility for clients that omit the /api/<version> prefix.
	aliasRoot := e.Group("")
	s.mountAPIRoutes(aliasRoot, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
}

func (s *HTTPServer) mountAPIRoutes(root *echo.Group, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, iotSvc *iotregistry.Service) {
	root.GET("/healthz", s.handleHealthz())
	root.GET("/versions", s.handleAPIVersions())
	root.GET("/auth/captcha", s.handleLoginCaptcha())
	root.POST("/auth/password/reset/start", s.handlePasswordResetStart())
	root.POST("/auth/password/reset/verify", s.handlePasswordResetVerify())
	root.POST("/auth/password/reset/complete", s.handlePasswordResetComplete())
	root.POST("/auth/login", s.handleCredentialLogin())
	root.POST("/cluster/join", s.handleClusterJoin())
	root.GET("/cluster/state", s.handleClusterState())
	root.POST("/auth/api-keys/exchange", s.handleAPIKeyExchange())
	root.POST("/auth/session", s.handleSessionCreate())
	root.POST("/auth/session/refresh", s.handleSessionRefresh())

	secured := root.Group("")
	s.applyAPIMiddleware(secured)
	secured.Use(s.primaryWriteGuardMiddleware())
	s.mountSecuredAPIRoutes(secured, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
}

func (s *HTTPServer) mountSecuredAPIRoutes(secured *echo.Group, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, iotSvc *iotregistry.Service) {
	secured.GET("/ui", s.handleUIMetadata())
	if s.options.RequireAuth {
		secured.GET("/ui/system/summary", s.handleUISystemSummary(), RequireRole(RoleAdmin), RequireCapabilities(
			CapabilityUserRead,
			CapabilityTenantQuotaRead,
			CapabilityRBACRoleRead,
		))
	} else {
		secured.GET("/ui/system/summary", s.handleUISystemSummary(), RequireCapabilities(
			CapabilityUserRead,
			CapabilityTenantQuotaRead,
			CapabilityRBACRoleRead,
		))
	}

	s.mountResourceRoutes(secured, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
	s.mountNetworkRoutes(secured)

	core := secured.Group("/core")
	if s.options.RequireAuth {
		core.Use(RequireRole(RoleReader))
	}
	s.mountCoreOpsRoutes(core)
	core.GET("/notifications/smtp", s.handleGetSMTPConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/smtp", s.handleSaveSMTPConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/smtp/test", s.handleTestSMTPConfig(), RequireRole(RoleAdmin))
	core.GET("/notifications/sms", s.handleGetSMSConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/sms", s.handleSaveSMSConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/sms/test", s.handleTestSMSConfig(), RequireRole(RoleAdmin))
	core.GET("/notifications/webhook", s.handleGetWebhookConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/webhook", s.handleSaveWebhookConfig(), RequireRole(RoleAdmin))
	core.POST("/notifications/webhook/test", s.handleTestWebhookConfig(), RequireRole(RoleAdmin))

	authGroup := secured.Group("/auth")
	if s.options.RequireAuth {
		authGroup.Use(RequireRole(RoleReader))
	}
	authGroup.GET("/api-keys", s.handleListAPIKeys())
	if s.options.RequireAuth {
		authGroup.POST("/api-keys", s.handleCreateAPIKey(), RequireRole(RoleAdmin), RequireCapability(CapabilityAPIKeyManage))
		authGroup.PATCH("/api-keys/:keyId", s.handleUpdateAPIKey(), RequireRole(RoleAdmin), RequireCapability(CapabilityAPIKeyManage))
		authGroup.DELETE("/api-keys/:keyId", s.handleDeleteAPIKey(), RequireRole(RoleAdmin), RequireCapability(CapabilityAPIKeyManage))
	} else {
		authGroup.POST("/api-keys", s.handleCreateAPIKey(), RequireCapability(CapabilityAPIKeyManage))
		authGroup.PATCH("/api-keys/:keyId", s.handleUpdateAPIKey(), RequireCapability(CapabilityAPIKeyManage))
		authGroup.DELETE("/api-keys/:keyId", s.handleDeleteAPIKey(), RequireCapability(CapabilityAPIKeyManage))
	}
	authGroup.GET("/users", s.handleListUsers(), RequireCapability(CapabilityUserRead))
	if s.options.RequireAuth {
		authGroup.POST("/users", s.handleCreateUser(), RequireRole(RoleAdmin), RequireCapability(CapabilityUserManage))
		authGroup.PUT("/users/:userId", s.handleUpdateUser(), RequireRole(RoleAdmin), RequireCapability(CapabilityUserManage))
		authGroup.GET("/users/:userId/roles", s.handleGetUserRoles(), RequireCapability(CapabilityUserRead))
		authGroup.PUT("/users/:userId/roles", s.handleReplaceUserRoles(), RequireRole(RoleAdmin), RequireCapability(CapabilityUserManage))
		authGroup.DELETE("/users/:userId", s.handleDeleteUser(), RequireRole(RoleAdmin), RequireCapability(CapabilityUserManage))
	} else {
		authGroup.POST("/users", s.handleCreateUser(), RequireCapability(CapabilityUserManage))
		authGroup.PUT("/users/:userId", s.handleUpdateUser(), RequireCapability(CapabilityUserManage))
		authGroup.GET("/users/:userId/roles", s.handleGetUserRoles(), RequireCapability(CapabilityUserRead))
		authGroup.PUT("/users/:userId/roles", s.handleReplaceUserRoles(), RequireCapability(CapabilityUserManage))
		authGroup.DELETE("/users/:userId", s.handleDeleteUser(), RequireCapability(CapabilityUserManage))
	}
	if s.options.RequireAuth {
		authGroup.GET("/providers", s.handleListAuthProviders(), RequireRole(RoleAdmin), RequireCapability(CapabilityAuthProviderRead))
		authGroup.POST("/providers", s.handleCreateAuthProvider(), RequireRole(RoleAdmin), RequireCapability(CapabilityAuthProviderManage))
		authGroup.PATCH("/providers/:providerId", s.handleUpdateAuthProvider(), RequireRole(RoleAdmin), RequireCapability(CapabilityAuthProviderManage))
	} else {
		authGroup.GET("/providers", s.handleListAuthProviders(), RequireCapability(CapabilityAuthProviderRead))
		authGroup.POST("/providers", s.handleCreateAuthProvider(), RequireCapability(CapabilityAuthProviderManage))
		authGroup.PATCH("/providers/:providerId", s.handleUpdateAuthProvider(), RequireCapability(CapabilityAuthProviderManage))
	}
	authGroup.GET("/session", s.handleSessionSnapshot())
	authGroup.GET("/sessions", s.handleListSessions(), RequireCapability(CapabilityUserRead))
	if s.options.RequireAuth {
		authGroup.DELETE("/sessions/:sessionId", s.handleDeleteSession(), RequireRole(RoleAdmin), RequireCapability(CapabilityUserManage))
	} else {
		authGroup.DELETE("/sessions/:sessionId", s.handleDeleteSession(), RequireCapability(CapabilityUserManage))
	}
	authGroup.GET("/capabilities", s.handleCapabilities())
	authGroup.POST("/logout", s.handleLogout())
	authGroup.DELETE("/session", s.handleLogout())
	authGroup.POST("/password", s.handlePasswordChange())

	rbacGroup := secured.Group("/rbac")
	if s.options.RequireAuth {
		rbacGroup.Use(RequireRole(RoleReader))
	}
	rbacGroup.GET("/capabilities", s.handleRBACCapabilities(), RequireCapability(CapabilityRBACRoleRead))
	rbacGroup.GET("/roles", s.handleListRoles(), RequireCapability(CapabilityRBACRoleRead))
	rbacGroup.POST("/roles", s.handleCreateRole(), RequireRole(RoleAdmin), RequireCapability(CapabilityRBACRoleManage))
	rbacGroup.PUT("/roles/:roleId", s.handleUpdateRole(), RequireRole(RoleAdmin), RequireCapability(CapabilityRBACRoleManage))
	rbacGroup.DELETE("/roles/:roleId", s.handleDeleteRole(), RequireRole(RoleAdmin), RequireCapability(CapabilityRBACRoleManage))

	monitoring := secured.Group("/monitoring")
	if s.options.RequireAuth {
		monitoring.Use(RequireRole(RoleReader))
	}
	monitoring.GET("/realtime/snapshot", s.handleMonitoringRealtimeSnapshot)
	monitoring.GET("/metrics/:metric", s.handleMonitoringMetrics)
	monitoring.GET("/latency", s.handleMonitoringLatency)
	monitoring.GET("/servers/perf", s.handleMonitoringServersPerf)
	monitoring.GET("/allocations/breakdown", s.handleMonitoringAllocationsBreakdown)
	monitoring.GET("/overview", s.handleMonitoringOverview)
	monitoring.GET("/pools", s.handleMonitoringPools)
	monitoring.GET("/pools/usage", s.handleMonitoringPoolsUsage)
	monitoring.GET("/pools/summary", s.handleMonitoringPoolsSummary)
	monitoring.GET("/requests", s.handleMonitoringRequests)
	monitoring.GET("/requests/abnormal", s.handleMonitoringAbnormalRequests)
	monitoring.GET("/health", s.handleMonitoringHealth)
	monitoring.GET("/security", s.handleMonitoringSecurity)
	monitoring.GET("/security/events", s.handleMonitoringSecurityEvents)
	monitoring.GET("/system-health", s.handleMonitoringSystemHealth)
	monitoring.GET("/automation", s.handleMonitoringAutomation())
	monitoring.GET("/leases/status", s.handleMonitoringLeaseStatus)
	monitoring.GET("/anomalies", s.handleMonitoringAnomalies)
	monitoring.GET("/synthetic", s.handleMonitoringSynthetic)
	monitoring.GET("/events/timeline", s.handleMonitoringTimeline)
	monitoring.GET("/operations", s.handleMonitoringOperations)
	monitoring.GET("/alerts", s.handleAlertFeed)
	monitoring.GET("/alerts/feed", s.handleAlertFeed)
	monitoring.GET("/alerts/rules", s.handleAlertRules)
	monitoring.GET("/alerts/events", s.handleAlertEvents)
	monitoring.GET("/alerts/active/summary", s.handleAlertActiveSummary)
	monitoring.GET("/alerts/active/trend", s.handleAlertActiveTrend)
	monitoring.GET("/alerts/history/summary", s.handleAlertHistorySummary)
	monitoring.GET("/alerts/history/compare", s.handleAlertHistoryCompare)
	monitoring.GET("/alerts/config/thresholds", s.handleAlertThresholdsGet)
	monitoring.GET("/alerts/config/notify", s.handleAlertNotifyGet)
	monitoring.GET("/alerts/templates", s.handleAlertTemplates)
	monitoring.GET("/alerts/receivers", s.handleAlertReceivers)
	monitoring.POST("/alerts/receivers/import", s.handleAlertReceiversImport)
	monitoring.GET("/alerts/receivers/export", s.handleAlertReceiversExport)
	monitoring.GET("/alerts/analytics/summary", s.handleAlertAnalyticsSummary)
	monitoring.GET("/alerts/analytics/type-distribution", s.handleAlertTypeDistribution)
	monitoring.GET("/alerts/analytics/time-distribution", s.handleAlertTimeDistribution)
	monitoring.GET("/alerts/analytics/trend", s.handleAlertTrendCompare)
	monitoring.POST("/alerts/rules", s.handleAlertRuleUpsert)
	monitoring.DELETE("/alerts/rules/:ruleId", s.handleAlertRuleDelete)
	monitoring.GET("/alerts/incidents", s.handleAlertIncidents)
	monitoring.GET("/alerts/routes", s.handleAlertRoutesSnapshot)
	monitoring.GET("/duty-roster", s.handleDutyRosterSnapshot)
	monitoring.GET("/notifications/channels", s.handleListNotificationChannels)
	monitoring.GET("/analytics", s.handleMonitoringAnalytics)
	monitoring.GET("/integrations/cmdb", s.handleMonitoringCMDBSync)
	if s.options.RequireAuth {
		monitoring.POST("/alerts/:alertId/ack", s.handleAlertAcknowledge, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/:alertId/suppress", s.handleAlertSuppress, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/:alertId/silence", s.handleAlertSilence, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/:alertId/escalate", s.handleAlertEscalate, RequireRole(RoleAdmin))
		monitoring.PUT("/alerts/routes", s.handleAlertRoutesUpdate, RequireRole(RoleAdmin))
		monitoring.PUT("/duty-roster", s.handleDutyRosterUpdate, RequireRole(RoleAdmin))
		monitoring.POST("/notifications/test", s.handleSendNotificationTest, RequireRole(RoleAdmin))
		monitoring.POST("/notifications/channels", s.handleUpsertNotificationChannel, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/config/thresholds", s.handleAlertThresholdsSave, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/config/notify", s.handleAlertNotifySave, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/templates", s.handleAlertTemplateCreate, RequireRole(RoleAdmin))
		monitoring.PUT("/alerts/templates/:templateId", s.handleAlertTemplateUpdate, RequireRole(RoleAdmin))
		monitoring.DELETE("/alerts/templates/:templateId", s.handleAlertTemplateDelete, RequireRole(RoleAdmin))
		monitoring.POST("/alerts/receivers", s.handleAlertReceiverCreate, RequireRole(RoleAdmin))
		monitoring.PUT("/alerts/receivers/:receiverId", s.handleAlertReceiverUpdate, RequireRole(RoleAdmin))
		monitoring.DELETE("/alerts/receivers/:receiverId", s.handleAlertReceiverDelete, RequireRole(RoleAdmin))
	} else {
		monitoring.POST("/alerts/:alertId/ack", s.handleAlertAcknowledge)
		monitoring.POST("/alerts/:alertId/suppress", s.handleAlertSuppress)
		monitoring.POST("/alerts/:alertId/silence", s.handleAlertSilence)
		monitoring.POST("/alerts/:alertId/escalate", s.handleAlertEscalate)
		monitoring.PUT("/alerts/routes", s.handleAlertRoutesUpdate)
		monitoring.PUT("/duty-roster", s.handleDutyRosterUpdate)
		monitoring.POST("/notifications/test", s.handleSendNotificationTest)
		monitoring.POST("/notifications/channels", s.handleUpsertNotificationChannel)
		monitoring.POST("/alerts/config/thresholds", s.handleAlertThresholdsSave)
		monitoring.POST("/alerts/config/notify", s.handleAlertNotifySave)
		monitoring.POST("/alerts/templates", s.handleAlertTemplateCreate)
		monitoring.PUT("/alerts/templates/:templateId", s.handleAlertTemplateUpdate)
		monitoring.DELETE("/alerts/templates/:templateId", s.handleAlertTemplateDelete)
		monitoring.POST("/alerts/receivers", s.handleAlertReceiverCreate)
		monitoring.PUT("/alerts/receivers/:receiverId", s.handleAlertReceiverUpdate)
		monitoring.DELETE("/alerts/receivers/:receiverId", s.handleAlertReceiverDelete)
	}
	monitoring.GET("/status/stream", s.handleStatusStream())

	debug := secured.Group("/debug")
	if s.options.RequireAuth {
		debug.Use(RequireRole(RoleAdmin))
	}
	debug.GET("/routes", s.handleDebugRoutes())

	cluster := secured.Group("/cluster")
	if s.options.RequireAuth {
		cluster.Use(RequireCapability(CapabilityHARead))
	}
	cluster.GET("/overview", s.handleClusterOverview())
	cluster.GET("/control", s.handleClusterControlGet())
	cluster.GET("/commands", s.handleClusterCommandsList(), RequireCapability(CapabilityHARead))
	cluster.POST("/commands", s.handleClusterCommandCreate(), RequireCapability(CapabilityHAManage))
	cluster.POST("/initialize", s.handleClusterInitialize(), RequireCapability(CapabilityHAManage))
	cluster.POST("/delete", s.handleClusterDelete(), RequireCapability(CapabilityHAManage))
	cluster.GET("/members/:memberId", s.handleClusterMemberDetail(), RequireCapability(CapabilityHARead))
	cluster.POST("/members/:memberId/leave", s.handleClusterMemberLeave(), RequireCapability(CapabilityHAManage))
	cluster.GET("/ha/config", s.handleClusterHAConfig())
	cluster.PUT("/ha/config", s.handleClusterHAConfigUpdate(), RequireCapability(CapabilityHAManage))
	cluster.GET("/ha-lb/config", s.handleClusterHaLbConfig())
	cluster.PUT("/ha-lb/config", s.handleClusterHaLbConfigUpdate(), RequireCapability(CapabilityHAManage))
	cluster.POST("/ha-lb/failover-test", s.handleClusterHaLbFailoverTest(), RequireCapability(CapabilityHAManage))
	cluster.GET("/ha-lb/failover-events", s.handleClusterHaLbFailoverEvents())
	cluster.GET("/failover/plans", s.handleClusterFailoverPlansList())
	cluster.GET("/failover/plans/:planId", s.handleClusterFailoverPlanGet())
	cluster.POST("/failover/plans", s.handleClusterFailoverPlanCreate(), RequireCapability(CapabilityHAManage))
	cluster.GET("/sync/config", s.handleClusterSyncConfig())
	cluster.PUT("/sync/config", s.handleClusterSyncConfigUpdate(), RequireCapability(CapabilityHAManage))
	cluster.GET("/scale/policy", s.handleClusterScalePolicy())
	cluster.PUT("/scale/policy", s.handleClusterScalePolicyUpdate(), RequireCapability(CapabilityHAManage))
	cluster.POST("/sync/trigger", s.handleClusterSyncTrigger(), RequireCapability(CapabilityHAManage))
	cluster.GET("/sync/status", s.handleClusterSyncStatus())
	cluster.GET("/sync/stats", s.handleClusterSyncStats())
	cluster.GET("/sync/transactions", s.handleClusterSyncTransactions())
	cluster.GET("/sync/transactions/:txId", s.handleClusterSyncTransactionGet())
	cluster.POST("/sync/transactions/:txId/retry", s.handleClusterSyncTransactionRetry(), RequireCapability(CapabilityHAManage))
	cluster.GET("/scale/events", s.handleClusterScaleEvents())
	cluster.GET("/backup/plan", s.handleClusterBackupPlan())
	cluster.PUT("/backup/plan", s.handleClusterBackupPlanUpdate(), RequireCapability(CapabilityHAManage))
	cluster.GET("/backup/optimization", s.handleClusterOptimizationPlan())
	cluster.PUT("/backup/optimization", s.handleClusterOptimizationPlanUpdate(), RequireCapability(CapabilityHAManage))
	cluster.POST("/backup/run", s.handleClusterBackupRun(), RequireCapability(CapabilityHAManage))
	cluster.POST("/backup/restore", s.handleClusterBackupRestore(), RequireCapability(CapabilityHAManage))
	cluster.POST("/backup/optimize", s.handleClusterOptimizationRun(), RequireCapability(CapabilityHAManage))
	cluster.GET("/backup/history", s.handleClusterBackupHistory())
	cluster.POST("/nodes", s.handleClusterNodeCreate(), RequireCapability(CapabilityHAManage))
	cluster.PUT("/nodes/:nodeId", s.handleClusterNodeUpdate(), RequireCapability(CapabilityHAManage))
	cluster.DELETE("/nodes/:nodeId", s.handleClusterNodeDelete(), RequireCapability(CapabilityHAManage))
	cluster.GET("/join-jobs", s.handleClusterJoinJobsList(), RequireCapability(CapabilityHARead))
	cluster.GET("/join-jobs/:jobId", s.handleClusterJoinJobGet(), RequireCapability(CapabilityHARead))
	cluster.POST("/join-jobs", s.handleClusterJoinJobCreate(), RequireCapability(CapabilityHAManage))
	cluster.POST("/join-jobs/:jobId/cancel", s.handleClusterJoinJobCancel(), RequireCapability(CapabilityHAManage))
	cluster.POST("/join-jobs/:jobId/retry", s.handleClusterJoinJobRetry(), RequireCapability(CapabilityHAManage))

	security := secured.Group("/security")
	if s.options.RequireAuth {
		security.Use(RequireCapability(CapabilitySecurityPolicyRead))
	}
	security.GET("/overview", s.handleSecurityOverview())

	dashboard := secured.Group("/dashboard")
	if s.options.RequireAuth {
		dashboard.Use(RequireRole(RoleReader))
	}
	dashboard.GET("/health", s.handleDashboardHealth)
	dashboard.GET("/kpis", s.handleDashboardKPIs)
	dashboard.GET("/streams", s.handleDashboardStreams)
	dashboard.GET("/streams/live", s.handleDashboardStreamsLive)
	dashboard.GET("/insights", s.handleDashboardInsights)
	dashboard.GET("/insights/overview", s.handleInsightsOverview)

	automation := secured.Group("/automation")
	if s.options.RequireAuth {
		automation.Use(RequireRole(RoleAdmin))
	}
	automation.GET("/snapshot", s.handleAutomationSnapshot())
	automation.GET("/schedules", s.handleAutomationSchedules())
	automation.PUT("/schedules/:jobType", s.handleAutomationUpsertSchedule())
	automation.GET("/jobs", s.handleAutomationListJobs())
	automation.GET("/jobs/:jobId", s.handleAutomationGetJob())
	automation.POST("/jobs", s.handleAutomationEnqueue())

	automationApprovals := automation.Group("/approvals")
	automationApprovals.GET("", s.handleAutomationApprovalsList())
	automationApprovals.GET("/:requestId", s.handleAutomationApprovalsGet())
	automationApprovals.POST("/:requestId/approve", s.handleAutomationApprovalApprove())
	automationApprovals.POST("/:requestId/reject", s.handleAutomationApprovalReject())
	automationApprovals.POST("/:requestId/apply", s.handleAutomationApprovalApply())

	workflows := automation.Group("/workflows")
	if s.options.RequireAuth {
		workflows.Use(RequireCapability(CapabilityPolicyWrite))
	}
	workflows.GET("/definitions", s.handleWorkflowListDefinitions())
	workflows.POST("/definitions", s.handleWorkflowCreateDefinition())
	workflows.POST("/definitions/:definitionId/publish", s.handleWorkflowPublishDefinition())
	workflows.POST("/executions", s.handleWorkflowStartExecution())

	visualization := secured.Group("/visualization")
	if s.options.RequireAuth {
		visualization.Use(RequireRole(RoleReader))
	}
	visualization.GET("/topology", s.handleVisualizationTopology)
	visualization.GET("/heatmap", s.handleVisualizationHeatmap)

	dhcp := secured.Group("/dhcp")
	if s.options.RequireAuth {
		dhcp.Use(RequireRole(RoleReader))
	}
	dhcpOptions := dhcp.Group("/options")
	dhcpOptions.GET("", s.handleDHCPOptionList(), RequireCapability(CapabilityPolicyRead))
	dhcpOptions.GET("/catalog", s.handleDHCPOptionCatalog(), RequireCapability(CapabilityPolicyRead))
	dhcpOptionsManage := dhcpOptions.Group("", RequireCapability(CapabilityPolicyWrite))
	dhcpOptionsManage.POST("", s.handleDHCPOptionCreate())
	dhcpOptionsManage.PUT("/:optionId", s.handleDHCPOptionUpdate())
	dhcpOptionsManage.DELETE("/:optionId", s.handleDHCPOptionDelete())

	dhcpTemplates := dhcp.Group("/templates")
	dhcpTemplates.GET("", s.handleDHCOTemplateList(), RequireCapability(CapabilityPolicyRead))
	dhcpTemplates.GET("/:templateId/history", s.handleDHCOTemplateApplyHistory(), RequireCapability(CapabilityPolicyRead))
	dhcpTemplatesManage := dhcpTemplates.Group("", RequireCapability(CapabilityPolicyWrite))
	dhcpTemplatesManage.POST("", s.handleDHCOTemplateCreate())
	dhcpTemplatesManage.PUT("/:templateId", s.handleDHCOTemplateUpdate())
	dhcpTemplatesManage.DELETE("/:templateId", s.handleDHCOTemplateDelete())
	dhcpTemplatesManage.POST("/:templateId/apply", s.handleDHCOTemplateApply())
	dhcpTemplatesManage.POST("/:templateId/sync", s.handleDHCOTemplateSync())

	dhcpScopes := dhcp.Group("/scopes")
	dhcpScopes.GET("", s.handleDHCPScopeList(), RequireCapability(CapabilityPolicyRead))
	dhcpScopesManage := dhcpScopes.Group("", RequireCapability(CapabilityPolicyWrite))
	dhcpScopesManage.POST("", s.handleDHCPScopeCreate())
	dhcpScopesManage.PUT("/:scopeId", s.handleDHCPScopeUpdate())
	dhcpScopesManage.DELETE("/:scopeId", s.handleDHCPScopeDelete())

	opsSupport := secured.Group("/ops")
	if s.options.RequireAuth {
		opsSupport.Use(RequireRole(RoleAdmin))
	}
	opsSupport.GET("/system", s.handleOpsSystemSummary())
	opsSupport.PUT("/system", s.handleOpsUpdateSystem())
	opsSupport.PATCH("/settings/theme", s.handleOpsUpdateTheme())
	opsSupport.PATCH("/settings/locale", s.handleOpsUpdateLocale())
	opsSupport.PATCH("/settings/maintenance", s.handleOpsUpdateMaintenance())
	opsSupport.PATCH("/settings/ntp", s.handleOpsUpdateNTP())
	opsSupport.POST("/settings/ntp/sync", s.handleOpsSyncNTP())
	opsSupport.GET("/import-export", s.handleOpsImportExport())
	opsSupport.POST("/import-export/jobs", s.handleOpsCreateTransferJob())
	opsSupport.GET("/import-export/jobs", s.handleOpsListTransferJobs())
	opsSupport.GET("/import-export/jobs/:jobId", s.handleOpsGetTransferJob())
	opsSupport.GET("/help-center", s.handleOpsHelpCenter())
	opsSupport.GET("/help-center/articles", s.handleOpsHelpArticles())
	opsSupport.GET("/help-center/faq", s.handleOpsHelpFAQ())
	opsSupport.GET("/help-center/releases", s.handleOpsHelpReleases())
	opsSupport.GET("/support", s.handleOpsSupportDirectory())
	opsSupport.GET("/scripts", s.handleOpsScriptCatalog())
	opsSupport.POST("/scripts/:scriptName/runs", s.handleOpsStartScriptRun())
	opsSupport.GET("/scripts/runs", s.handleOpsListScriptRuns())
	opsSupport.GET("/scripts/runs/:runId", s.handleOpsGetScriptRun())
	opsSupport.POST("/scripts/runs/:runId/approve", s.handleOpsApproveScriptRun())
	opsSupport.POST("/scripts/runs/:runId/reject", s.handleOpsRejectScriptRun())
	opsSupport.GET("/route-usage", s.handleOpsRouteUsage())

	helpCenter := secured.Group("/help-center")
	if s.options.RequireAuth {
		helpCenter.Use(RequireRole(RoleAdmin))
	}
	helpCenter.GET("/snapshot", s.handleHelpCenterSnapshot())

	integrations := secured.Group("/integrations")
	if s.options.RequireAuth {
		integrations.Use(RequireRole(RoleAdmin))
	}
	integrations.GET("/overview", s.handleIntegrationsOverview())

	maintenance := secured.Group("/maintenance")
	if s.options.RequireAuth {
		maintenance.Use(RequireRole(RoleAdmin))
	}
	maintenance.GET("/overview", s.handleMaintenanceOverview())
	maintenance.GET("/plan", s.handleMaintenancePlan())
	maintenance.GET("/playbooks", s.handleMaintenancePlaybooks())

	backups := secured.Group("/backups")
	if s.options.RequireAuth {
		backups.Use(RequireRole(RoleAdmin))
	}
	backups.GET("/wizard", s.handleBackupWizard())
	backups.GET("/schedules", s.handleBackupSchedules())
	backups.GET("/destinations", s.handleBackupDestinations())

	performance := secured.Group("/performance")
	if s.options.RequireAuth {
		performance.Use(RequireRole(RoleAdmin))
	}
	performance.GET("/diagnostics", s.handlePerformanceDiagnostics())

	diagnostics := secured.Group("/diagnostics")
	if s.options.RequireAuth {
		diagnostics.Use(RequireRole(RoleAdmin))
	}
	diagnostics.GET("/snapshot", s.handleDiagnosticsSnapshot())

	operations := secured.Group("/operations")
	if s.options.RequireAuth {
		operations.Use(RequireRole(RoleReader))
	}
	operations.GET("/audit-activity", s.handleAuditActivity)

	reports := secured.Group("/reports")
	if s.options.RequireAuth {
		reports.Use(RequireRole(RoleAdmin))
	}
	reports.GET("", s.handleReportTasks, RequireCapability(CapabilityReportRead))
	reports.POST("", s.handleReportTaskCreate, RequireCapability(CapabilityReportRead))
	reports.POST("/lease-daily", s.handleLeaseDailySchedule(), RequireCapability(CapabilityReportRead))
}

// Start runs the HTTP server.
func (s *HTTPServer) Start(port int) error {
	server := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: s.echo}
	tlsOpts := s.options.APITLS
	if !tlsOpts.Enabled {
		s.apiTLSRuntimeActive = false
		return s.echo.StartServer(server)
	}
	if strings.TrimSpace(tlsOpts.CertFile) == "" || strings.TrimSpace(tlsOpts.KeyFile) == "" {
		return fmt.Errorf("http server: tls enabled but certFile/keyFile missing")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(tlsOpts.ClientCAFile) != "" {
		caBytes, err := os.ReadFile(strings.TrimSpace(tlsOpts.ClientCAFile))
		if err != nil {
			return fmt.Errorf("http server: read client ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return fmt.Errorf("http server: parse client ca pem failed")
		}
		tlsConfig.ClientCAs = pool
	}
	if tlsOpts.RequireClientCert {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else if tlsConfig.ClientCAs != nil {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}
	server.TLSConfig = tlsConfig
	s.apiTLSRuntimeActive = true
	return server.ListenAndServeTLS(strings.TrimSpace(tlsOpts.CertFile), strings.TrimSpace(tlsOpts.KeyFile))
}

// Shutdown gracefully stops the server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.stopClusterSyncResponder()
	s.stopSyncTransactionsReconciler()
	return s.echo.Shutdown(ctx)
}

func (s *HTTPServer) actorFromContext(c echo.Context) string {
	if actor, ok := c.Get(contextActorKey).(string); ok && actor != "" {
		return actor
	}
	return "anonymous"
}

func (s *HTTPServer) principalFromContext(c echo.Context) string {
	if c == nil {
		return ""
	}
	if principal, ok := c.Get(contextPrincipalKey).(string); ok && principal != "" {
		return principal
	}
	return ""
}

func (s *HTTPServer) roleFromContext(c echo.Context) string {
	if role, ok := c.Get(contextRoleKey).(string); ok && role != "" {
		return role
	}
	return ""
}

func (s *HTTPServer) resolvePrincipalTenants(ctx context.Context, principal string) []string {
	_ = ctx
	_ = principal
	return []string{systemTenantID}
}

func (s *HTTPServer) auditTenantFromContext(c echo.Context) string {
	scope := s.leaseScopeRef(c)
	tenant := strings.TrimSpace(scope.TenantOrDefault())
	if tenant == "" {
		return systemTenantID
	}
	return tenant
}

func (s *HTTPServer) recordAudit(ctx context.Context, tenantID, actor, action string, payload any, opts ...auditOption) {
	logger := LoggerFromContext(ctx, s.logger)
	if s.auditSvc == nil {
		return
	}
	actor = normalizeAuditActor(actor)
	var raw json.RawMessage
	if payload != nil {
		enriched := payload
		if data, err := json.Marshal(payload); err == nil {
			asMap := map[string]any{}
			if err := json.Unmarshal(data, &asMap); err == nil {
				if strings.TrimSpace(toStringAny(asMap["ip"])) == "" {
					if clientIP := ClientIPFromContext(ctx); clientIP != "" {
						asMap["ip"] = clientIP
					}
				}
				if strings.TrimSpace(toStringAny(asMap["userAgent"])) == "" {
					if ua := UserAgentFromContext(ctx); ua != "" {
						asMap["userAgent"] = ua
					}
				}
				if strings.TrimSpace(toStringAny(asMap["requestId"])) == "" {
					if reqID := RequestIDFromContext(ctx); reqID != "" {
						asMap["requestId"] = reqID
					}
				}
				if strings.TrimSpace(toStringAny(asMap["traceId"])) == "" {
					if traceID := traceIDFromContext(ctx); traceID != "" {
						asMap["traceId"] = traceID
					}
				}
				enriched = asMap
			}
		}
		if data, err := json.Marshal(enriched); err != nil {
			logger.Warn("failed to marshal audit payload", zap.Error(err))
		} else {
			raw = data
		}
	}
	req := audit.RecordEventRequest{
		TenantID:      tenantID,
		Actor:         actor,
		Action:        action,
		Source:        "api.http",
		CorrelationID: correlationIDFromContext(ctx),
		Payload:       raw,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	state := auditStatusFromContext(ctx)
	if err := s.auditSvc.RecordEvent(ctx, req); err != nil {
		if state != nil {
			state.recorded = true
			state.failed = true
		}
		logger.Warn("failed to record audit event", zap.Error(err), zap.String("traceId", traceIDFromContext(ctx)))
	} else if state != nil {
		state.recorded = true
	}
}

func (s *HTTPServer) currentClusterFencingEpoch() string {
	if s == nil {
		return ""
	}
	if svc := s.ensureHAService(); svc != nil {
		if epoch := strings.TrimSpace(clusterFencingEpoch(svc.Snapshot())); epoch != "" {
			return epoch
		}
	}
	if s.options.Coordinator != nil {
		if epoch := strings.TrimSpace(clusterFencingEpoch(s.options.Coordinator.Snapshot())); epoch != "" {
			return epoch
		}
	}
	return ""
}

func (s *HTTPServer) recordClusterSwitchAuditEvent(ctx context.Context, tenantID, actor, stage, trigger, reason, oldPrimary, newPrimary, result string, rollback bool) {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "event"
	}
	action := "cluster.failover." + stage
	payload := map[string]any{
		"trigger":    strings.TrimSpace(trigger),
		"reason":     strings.TrimSpace(reason),
		"oldPrimary": strings.TrimSpace(oldPrimary),
		"newPrimary": strings.TrimSpace(newPrimary),
		"epoch":      s.currentClusterFencingEpoch(),
		"result":     strings.TrimSpace(result),
		"rollback":   rollback,
		"stage":      stage,
	}
	s.recordAudit(ctx, tenantID, actor, action, payload, withResource("cluster"))
}

func toStringAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func normalizeAuditActor(actor string) string {
	value := strings.TrimSpace(actor)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "session:") {
		trimmed := strings.TrimSpace(value[len("session:"):])
		if trimmed != "" {
			return trimmed
		}
	}
	return value
}

type auditOption func(*audit.RecordEventRequest)

func withSource(source string) auditOption {
	return func(req *audit.RecordEventRequest) {
		if source != "" {
			req.Source = source
		}
	}
}

func withResource(resource string) auditOption {
	return func(req *audit.RecordEventRequest) {
		if resource != "" {
			req.Resource = resource
		}
	}
}

func (s *HTTPServer) auditPayloadFromRequest(c echo.Context, payload map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range payload {
		merged[key] = value
	}
	if c == nil {
		return merged
	}
	ctx := c.Request().Context()
	if _, exists := merged["ip"]; !exists {
		merged["ip"] = c.RealIP()
	}
	if _, exists := merged["userAgent"]; !exists {
		merged["userAgent"] = c.Request().UserAgent()
	}
	if _, exists := merged["requestId"]; !exists {
		merged["requestId"] = requestIDFromEcho(c)
	}
	if _, exists := merged["traceId"]; !exists {
		merged["traceId"] = traceIDFromContext(ctx)
	}
	return merged
}

func requestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c == nil {
				return next(c)
			}
			req := c.Request()
			if req == nil {
				return next(c)
			}
			id := strings.TrimSpace(req.Header.Get(echo.HeaderXRequestID))
			if id == "" {
				id = strings.TrimSpace(c.Response().Header().Get(echo.HeaderXRequestID))
			}
			if id == "" {
				id = uuid.NewString()
			}
			c.Response().Header().Set(echo.HeaderXRequestID, id)
			req.Header.Set(echo.HeaderXRequestID, id)
			c.Set(echo.HeaderXRequestID, id)
			setRequestContextValue(c, contextKeyRequestID, id)
			return next(c)
		}
	}
}

func (s *HTTPServer) enrichRequestContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if env := strings.ToLower(strings.TrimSpace(s.options.Environment)); env != "" {
			setRequestContextValue(c, contextKeyEnvironment, env)
		}
		if clientIP := strings.TrimSpace(c.RealIP()); clientIP != "" {
			setRequestContextValue(c, contextKeyClientIP, clientIP)
		}
		if req := c.Request(); req != nil {
			if ua := strings.TrimSpace(req.UserAgent()); ua != "" {
				setRequestContextValue(c, contextKeyUserAgent, ua)
			}
		}
		if correlation := requestIDFromEcho(c); correlation != "" {
			setRequestContextValue(c, contextKeyCorrelationID, correlation)
			req := c.Request()
			if req == nil || RequestIDFromContext(req.Context()) == "" {
				setRequestContextValue(c, contextKeyRequestID, correlation)
			}
		}
		return next(c)
	}
}

func (s *HTTPServer) auditAccessMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		if s.auditSvc == nil {
			return err
		}
		path := c.Path()
		if !strings.HasPrefix(path, "/api/") {
			return err
		}
		scope := s.leaseScopeRef(c)
		method := c.Request().Method
		if method == http.MethodOptions {
			return err
		}
		status := c.Response().Status
		if status == 0 {
			status = http.StatusOK
		}
		reqCtx := c.Request().Context()
		actor := s.actorFromContext(c)
		role := s.roleFromContext(c)
		payload := auditpayload.AdminActivity{
			Actor:          actor,
			Role:           role,
			Method:         method,
			Path:           path,
			Query:          captureQueryParams(c.Request()),
			StatusCode:     status,
			RemoteAddr:     c.RealIP(),
			UserAgent:      c.Request().UserAgent(),
			DurationMillis: time.Since(start).Milliseconds(),
			CorrelationID:  correlationIDFromContext(reqCtx),
			Sensitive:      isSensitivePath(path, method),
		}
		if meta := scopeutil.FromLeaseScope(scope); !meta.IsZero() {
			payload.Scope = meta
		}
		tenantID := strings.TrimSpace(scope.TenantOrDefault())
		if tenantID == "" {
			tenantID = systemTenantID
		}
		resource := resourceFromPath(path)
		s.recordAudit(reqCtx, tenantID, actor, "admin.activity", payload, withSource("api.http"), withResource(resource))
		return err
	}
}

func (s *HTTPServer) instrumentationMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		status := c.Response().Status
		if status == 0 {
			status = http.StatusOK
		}
		durationSeconds := time.Since(start).Seconds()
		path := c.Path()
		method := c.Request().Method
		statusLabel := strconv.Itoa(status)
		if s.options.Metrics != nil {
			s.options.Metrics.HTTPRequests.WithLabelValues(path, method, statusLabel).Inc()
			if s.options.Metrics.HTTPRequestLatency != nil {
				s.options.Metrics.HTTPRequestLatency.WithLabelValues(path, method, statusLabel).Observe(durationSeconds)
			}
			if status >= http.StatusInternalServerError && s.options.Metrics.HTTPErrors != nil {
				s.options.Metrics.HTTPErrors.WithLabelValues(path, method, statusLabel).Inc()
			}
		}
		if s.otelHTTPDuration != nil {
			s.otelHTTPDuration.Record(c.Request().Context(), durationSeconds, metric.WithAttributes(
				attribute.String("http.target", path),
				attribute.String("http.method", method),
				attribute.String("http.status_code", statusLabel),
			))
		}
		if s.otelHTTPErrors != nil && status >= http.StatusInternalServerError {
			s.otelHTTPErrors.Add(c.Request().Context(), 1, metric.WithAttributes(
				attribute.String("http.target", path),
				attribute.String("http.method", method),
				attribute.String("http.status_code", statusLabel),
			))
		}
		return err
	}
}

// traceContextHeaderMiddleware propagates a user-friendly trace ID header when a span is present.
func traceContextHeaderMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			if span := trace.SpanFromContext(c.Request().Context()); span != nil {
				sc := span.SpanContext()
				if sc.IsValid() {
					c.Response().Header().Set("X-Trace-Id", sc.TraceID().String())
				}
			}
			return err
		}
	}
}

func (s *HTTPServer) resolveCapabilitiesMiddleware() echo.MiddlewareFunc {
	if s.rbacResolver == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	strict := s.options.RBAC.EnforceCapabilities && !s.options.RBAC.ShadowMode
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set(contextCapabilityStrictKey, strict)
			principalCtx := s.principalContext(c)
			principal := strings.TrimSpace(principalCtx.UserID)
			if principal == "" {
				return next(c)
			}
			logger := LoggerFromContext(c.Request().Context(), s.logger)
			scope := principalCtx.EffectiveScope()
			tenantID := scope.EffectiveTenant()
			resolution, err := s.rbacResolver.Resolve(
				c.Request().Context(),
				principal,
				rbac.ResolveOptions{Principal: principalCtx},
			)
			if err != nil {
				if logger != nil {
					logger.Warn("rbac resolution failed", zap.String("principalId", principal), zap.String("tenantId", tenantID), zap.Error(err))
				}
				if s.options.RBAC.EnforceCapabilities && !s.options.RBAC.ShadowMode {
					return echo.NewHTTPError(http.StatusForbidden, "rbac resolution failed")
				}
				return next(c)
			}
			resCopy := resolution
			c.Set(contextResolutionKey, &resCopy)
			c.Set(contextCapabilitiesKey, resCopy.Capabilities)
			return next(c)
		}
	}
}

// csrfMiddleware implements double-submit check: for unsafe methods, require X-CSRF-Token header matching csrf_token cookie.
func (s *HTTPServer) csrfMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := strings.ToUpper(c.Request().Method)
			if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
				return next(c)
			}
			// login/refresh endpoints may not have cookie yet; middleware is applied only to secured groups.
			csrfCookie, err := c.Cookie("csrf_token")
			if err != nil || strings.TrimSpace(csrfCookie.Value) == "" {
				return echo.NewHTTPError(http.StatusForbidden, "csrf token missing")
			}
			headervalue := strings.TrimSpace(c.Request().Header.Get("X-CSRF-Token"))
			if headervalue == "" || headervalue != strings.TrimSpace(csrfCookie.Value) {
				return echo.NewHTTPError(http.StatusForbidden, "csrf token invalid")
			}
			return next(c)
		}
	}
}

func (s *HTTPServer) newSessionResponse(ctx context.Context, user auth.User, tenantID string, session auth.SessionToken, authMethod string) sessionResponse {
	role := normalizeRole(user.Role)
	actorName := strings.TrimSpace(user.DisplayName)
	if actorName == "" {
		actorName = user.Username
	}
	_ = ctx
	if strings.TrimSpace(tenantID) == "" {
		tenantID = systemTenantID
	}
	expires := session.ExpiresAt.UTC().Format(time.RFC3339)
	return sessionResponse{
		Token:        session.Token,
		RefreshToken: session.Token,
		ExpiresAt:    expires,
		AuthMethod:   authMethod,
		Actor: sessionActor{
			ID:           user.PrincipalID(),
			Name:         actorName,
			Role:         role,
			Capabilities: capabilitiesForRole(role),
		},
		TenantID:           tenantID,
		MustChangePassword: user.MustChangePassword,
	}
}

func (s *HTTPServer) sessionResponseFromLegacy(resp credentialLoginResponse) sessionResponse {
	role := normalizeRole(resp.Role)
	actorName := strings.TrimSpace(resp.DisplayName)
	if actorName == "" {
		actorName = resp.PrincipalID
	}
	tenantID := strings.TrimSpace(resp.TenantID)
	if tenantID == "" {
		tenantID = systemTenantID
	}
	return sessionResponse{
		Token:        resp.Token,
		RefreshToken: resp.Token,
		ExpiresAt:    resp.ExpiresAt,
		AuthMethod:   "legacy",
		Actor: sessionActor{
			ID:           resp.PrincipalID,
			Name:         actorName,
			Role:         role,
			Capabilities: capabilitiesForRole(role),
		},
		TenantID:           tenantID,
		MustChangePassword: resp.MustChangePassword,
	}
}

func permissionsFromCapabilities(capabilities []string) int {
	for _, capName := range capabilities {
		capName = strings.TrimSpace(capName)
		if strings.HasPrefix(capName, "perm:") {
			if value, err := strconv.Atoi(strings.TrimPrefix(capName, "perm:")); err == nil {
				return value
			}
		}
	}
	return len(capabilities)
}

func permissionsToCapabilities(value int) []string {
	if value < 0 {
		value = 0
	}
	return []string{"perm:" + strconv.Itoa(value)}
}

func newRBACRoleResponse(role rbac.Role) rbacRoleResponse {
	return rbacRoleResponse{
		ID:           role.Name,
		Name:         role.Name,
		Description:  role.Description,
		Permissions:  permissionsFromCapabilities(role.Capabilities),
		Capabilities: role.Capabilities,
	}
}

func classifyRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/") && strings.Contains(path, "/core/"):
		return "core"
	case strings.HasPrefix(path, "/api/") && strings.Contains(path, "/monitoring/"):
		return "monitoring"
	case strings.HasPrefix(path, "/api/") && strings.Contains(path, "/auth/"):
		return "auth"
	case strings.HasPrefix(path, "/api/"):
		return "api"
	default:
		return "other"
	}
}

func (s *HTTPServer) handleDebugRoutes() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.echo == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "router not initialized")
		}
		routes := s.echo.Routes()
		payload := make([]map[string]string, 0, len(routes))
		for _, r := range routes {
			payload = append(payload, map[string]string{
				"method": r.Method,
				"path":   r.Path,
				"name":   r.Name,
				"group":  classifyRoute(r.Path),
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status": "ok",
			"count":  len(payload),
			"routes": payload,
		})
	}
}

func isProtectedRole(name string) bool {
	return strings.EqualFold(name, RoleAdmin) || strings.EqualFold(name, RoleReader)
}

func containsTenant(tenants []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, candidate := range tenants {
		if strings.EqualFold(strings.TrimSpace(candidate), target) {
			return true
		}
	}
	return false
}

func capabilitiesForRole(role string) []string {
	switch normalizeRole(role) {
	case RoleAdmin:
		return []string{
			CapabilityPolicyRead,
			CapabilityPolicyWrite,
			CapabilityPolicyBYODManage,
			CapabilityPoolRead,
			CapabilityPoolWrite,
			CapabilityBindingRead,
			CapabilityBindingManage,
			CapabilityLeaseRead,
			CapabilityLeaseManage,
			CapabilityReportRead,
			CapabilityAuditRead,
			CapabilityUserRead,
			CapabilityUserManage,
			CapabilityAPIKeyManage,
			CapabilityRBACRoleRead,
			CapabilityRBACRoleManage,
			CapabilitySecurityPolicyRead,
			CapabilitySecurityPolicyManage,
			CapabilitySecurityView,
			CapabilitySecurityManage,
			CapabilityHARead,
			CapabilityHAManage,
			CapabilityTenantQuotaRead,
			CapabilityTenantQuotaWrite,
			CapabilityRBACAssignmentRead,
			CapabilityRBACAssignmentWrite,
			CapabilityIoTRegistryRead,
			CapabilityIoTRegistryManage,
		}
	default:
		return []string{
			CapabilityPolicyRead,
			CapabilityPoolRead,
			CapabilityBindingRead,
			CapabilityLeaseRead,
			CapabilityReportRead,
			CapabilityAuditRead,
			CapabilityUserRead,
			CapabilitySecurityPolicyRead,
			CapabilitySecurityView,
			CapabilityHARead,
		}
	}
}

func buildHealthChecks(snapshot monitoring.SystemHealthSnapshot) []healthCheck {
	checks := []healthCheck{
		{
			Name:    "cpu",
			Status:  classifyResource(snapshot.CPUPercent, 75, 90),
			Details: fmt.Sprintf("%.1f%% utilized", snapshot.CPUPercent),
		},
		{
			Name:    "memory",
			Status:  classifyResource(snapshot.MemoryPercent, 75, 90),
			Details: fmt.Sprintf("%.1f%% ( %s ) used", snapshot.MemoryPercent, formatBytes(snapshot.MemoryUsedBytes)),
		},
		{
			Name:    "disk",
			Status:  classifyResource(snapshot.DiskPercent, 80, 90),
			Details: fmt.Sprintf("%.1f%% capacity", snapshot.DiskPercent),
		},
		{
			Name:    "network",
			Status:  "ok",
			Details: fmt.Sprintf("rx %s / tx %s", formatBytes(snapshot.NetworkRxBytes), formatBytes(snapshot.NetworkTxBytes)),
		},
		{
			Name:    "goroutines",
			Status:  classifyResource(float64(snapshot.Goroutines), 5000, 10000),
			Details: fmt.Sprintf("%d goroutines", snapshot.Goroutines),
		},
	}
	return checks
}

func classifyResource(value, warnThreshold, criticalThreshold float64) string {
	switch {
	case value >= criticalThreshold && criticalThreshold > 0:
		return "error"
	case value >= warnThreshold && warnThreshold > 0:
		return "warning"
	default:
		return "ok"
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		exp++
		div *= unit
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (s *HTTPServer) serverUptimeSeconds() int64 {
	if s == nil || s.bootTime.IsZero() {
		return 0
	}
	return int64(time.Since(s.bootTime).Seconds())
}

func (s *HTTPServer) clusterStatusSnapshot() clusterStatus {
	if s == nil || s.options.Coordinator == nil {
		return clusterStatus{Role: "standalone", State: "normal"}
	}
	snapshot := s.options.Coordinator.Snapshot()
	warnings := make([]string, 0, 2)
	if snapshot.ManualFailbackSet {
		warnings = append(warnings, "manual failback armed")
	}
	if !snapshot.PeerLastSeen.IsZero() && time.Since(snapshot.PeerLastSeen) > 5*time.Minute {
		warnings = append(warnings, "peer heartbeat stale")
	}
	nodes := []string{"self"}
	if !snapshot.PeerLastSeen.IsZero() {
		nodes = append(nodes, "peer")
	}
	return clusterStatus{
		Role:     string(snapshot.Role),
		State:    string(snapshot.State),
		Nodes:    nodes,
		Warnings: warnings,
	}
}

func captureQueryParams(r *http.Request) map[string]string {
	if r == nil {
		return nil
	}
	values := r.URL.Query()
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string)
	count := 0
	for key, val := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		result[trimmed] = strings.Join(val, ",")
		count++
		if count >= 8 {
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func resourceFromPath(path string) string {
	clean := strings.Trim(path, "/")
	if clean == "" {
		return "api"
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		if part == "api" || strings.HasPrefix(part, "v") {
			continue
		}
		if strings.HasPrefix(part, ":") {
			continue
		}
		return part
	}
	return parts[len(parts)-1]
}

func isSensitivePath(path, method string) bool {
	keywords := []string{"pools", "policies", "leases", "bindings", "audit", "monitoring", "prefix"}
	for _, kw := range keywords {
		if strings.Contains(path, kw) {
			return true
		}
	}
	return method != http.MethodGet
}

func requestIDFromEcho(c echo.Context) string {
	if c == nil {
		return ""
	}
	req := c.Request()
	if req != nil {
		if id := RequestIDFromContext(req.Context()); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(c.Response().Header().Get(echo.HeaderXRequestID)); id != "" {
		return id
	}
	if req != nil {
		if id := strings.TrimSpace(req.Header.Get(echo.HeaderXRequestID)); id != "" {
			return id
		}
	}
	if id, ok := c.Get(echo.HeaderXRequestID).(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return ""
}

func correlationIDFromContext(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyCorrelationID)
}

type auditStatus struct {
	recorded bool
	failed   bool
}

type auditStatusCtxKey struct{}

func auditStatusFromContext(ctx context.Context) *auditStatus {
	if ctx == nil {
		return nil
	}
	if state, ok := ctx.Value(auditStatusCtxKey{}).(*auditStatus); ok {
		return state
	}
	return nil
}

// auditStatusMiddleware tracks audit recording status and surfaces failures via response headers.
func auditStatusMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			state := &auditStatus{}
			req := c.Request()
			ctx := context.WithValue(req.Context(), auditStatusCtxKey{}, state)
			c.SetRequest(req.WithContext(ctx))

			err := next(c)
			if state.failed {
				c.Response().Header().Set("X-Audit-Status", "failed")
			} else if state.recorded {
				c.Response().Header().Set("X-Audit-Status", "ok")
			}
			return err
		}
	}
}

func traceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

func spanIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		return sc.SpanID().String()
	}
	return ""
}

// loggerMiddleware enriches the request context with a logger carrying request/trace identifiers.
func (s *HTTPServer) loggerMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c == nil || s.logger == nil {
				return next(c)
			}
			req := c.Request()
			ctx := req.Context()
			fields := make([]zap.Field, 0, 3)
			if reqID := requestIDFromEcho(c); reqID != "" {
				fields = append(fields, zap.String("requestId", reqID))
			}
			if traceID := traceIDFromContext(ctx); traceID != "" {
				fields = append(fields, zap.String("traceId", traceID))
			}
			if spanID := spanIDFromContext(ctx); spanID != "" {
				fields = append(fields, zap.String("spanId", spanID))
			}
			logger := s.logger.With(fields...)
			ctx = context.WithValue(ctx, contextKeyLogger, logger)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

func leaseLifecyclePayload(leaseObj *models.Lease, action, actor, reason string) auditpayload.LeaseLifecycle {
	payload := auditpayload.LeaseLifecycle{
		Action:      action,
		Actor:       actor,
		Source:      "api.http",
		RequestedAt: time.Now().UTC(),
	}
	if leaseObj != nil {
		payload.LeaseID = leaseObj.ID
		payload.TenantID = leaseObj.TenantID
		payload.PoolID = leaseObj.PoolID
		payload.IPAddress = leaseObj.IPAddress
		payload.MACAddress = leaseObj.HardwareAddr
		payload.ClientID = leaseObj.ClientID
		payload.ExpiresAt = leaseObj.ExpiresAt
		payload.SecurityState = leaseObj.SecurityState
	}
	if reason != "" {
		payload.Reason = reason
	}
	return payload
}

func (s *HTTPServer) recordPolicyEval(result string) {
	if s.options.Metrics == nil {
		return
	}
	s.options.Metrics.PolicyHits.WithLabelValues(result).Inc()
}

func (s *HTTPServer) recordLeaseEvent(reused bool) {
	if s.options.Metrics == nil {
		return
	}
	action := "allocated"
	if reused {
		action = "reused"
	}
	s.options.Metrics.LeaseEvents.WithLabelValues(action).Inc()
}

func (s *HTTPServer) handleLeaseAction(c echo.Context, leaseSvc *lease.Service, action string) error {
	scopeRef := s.leaseScopeRef(c)
	tenantID := scopeRef.TenantOrDefault()
	leaseID := c.Param("leaseId")
	var payload leaseActionRequest
	if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	var (
		leaseObj *models.Lease
		changed  bool
		err      error
	)
	switch action {
	case "release":
		leaseObj, changed, err = leaseSvc.ReleaseLease(ctx, scopeRef, leaseID)
	case "decline":
		leaseObj, changed, err = leaseSvc.DeclineLease(ctx, scopeRef, leaseID)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported action")
	}
	if err != nil {
		if errors.Is(err, lease.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "lease not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !changed {
		c.Response().Header().Set("Warning", `299 - "lease already `+action+`"`)
	} else {
		actor := s.actorFromContext(c)
		lifecycle := leaseLifecyclePayload(leaseObj, action, actor, payload.Reason)
		s.recordAudit(ctx, tenantID, actor, "lease."+action, lifecycle, withResource("lease"))
	}
	steps := buildLeaseOperationSteps(leaseObj, action, payload.Reason, changed)
	status := "success"
	if !changed {
		status = "noop"
	}
	envelope := s.buildOperationResponse(ctx, scopeRef, "lease."+action, status, leaseObj, changed, steps)
	return c.JSON(http.StatusOK, envelope)
}

func buildLeaseOperationSteps(leaseObj *models.Lease, action, reason string, changed bool) []models.OperationStep {
	if leaseObj == nil {
		return nil
	}
	steps := []models.OperationStep{
		{Label: "租约", Detail: leaseObj.IPAddress, Status: "info"},
		{Label: "子网", Detail: leaseObj.PoolID, Status: "info"},
	}
	if reason != "" {
		steps = append(steps, models.OperationStep{Label: "原因", Detail: reason, Status: "warning"})
	}
	finalDetail := fmt.Sprintf("%s → %s", strings.ToUpper(action), leaseObj.State)
	finalStatus := "success"
	if !changed {
		finalDetail = "状态未改变"
		finalStatus = "warning"
	}
	steps = append(steps, models.OperationStep{Label: "生命周期", Detail: finalDetail, Status: finalStatus})
	return steps
}

func buildSimulateRelayData(ctxPayload *simulateRelayContext) (map[string]string, map[string]any) {
	if ctxPayload == nil {
		return nil, nil
	}
	attrs := make(map[string]string, len(ctxPayload.Attributes)+3)
	for k, v := range ctxPayload.Attributes {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			attrs[key] = trimmed
		}
	}
	if val := strings.TrimSpace(ctxPayload.GIAddr); val != "" {
		attrs["giaddr"] = val
	}
	if val := strings.TrimSpace(ctxPayload.LinkAddr); val != "" {
		attrs["link-addr"] = val
		if _, ok := attrs["giaddr"]; !ok {
			attrs["giaddr"] = val
		}
	}
	if val := strings.TrimSpace(ctxPayload.PeerAddr); val != "" {
		attrs["peer-addr"] = val
	}
	if len(attrs) == 0 {
		return nil, nil
	}
	relayInfo := make(map[string]any, len(attrs))
	for k, v := range attrs {
		relayInfo[k] = v
	}
	return attrs, relayInfo
}

func resolveSimulateVLAN(explicit int, ctxPayload *simulateRelayContext, attrs map[string]string) int {
	if explicit > 0 {
		return explicit
	}
	if ctxPayload != nil && ctxPayload.VLANID != nil && *ctxPayload.VLANID > 0 {
		return *ctxPayload.VLANID
	}
	return parseRelayVLANFromAttrs(attrs)
}

func parseRelayVLANFromAttrs(attrs map[string]string) int {
	if len(attrs) == 0 {
		return 0
	}
	for _, key := range []string{"vlan-id", "agent.vlan-id"} {
		if raw, ok := attrs[key]; ok {
			if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				return v
			}
		}
	}
	return 0
}

func (s *HTTPServer) handlePrefixLeaseAction(c echo.Context, leaseSvc *lease.Service, action string) error {
	scopeRef := s.leaseScopeRef(c)
	tenantID := scopeRef.TenantOrDefault()
	leaseID := c.Param("prefixLeaseId")
	var payload leaseActionRequest
	if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	var (
		leaseObj *models.PrefixLease
		changed  bool
		err      error
	)
	switch action {
	case "release":
		leaseObj, changed, err = leaseSvc.ReleasePrefixByID(ctx, scopeRef, leaseID)
	case "decline":
		leaseObj, changed, err = leaseSvc.DeclinePrefixByID(ctx, scopeRef, leaseID)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported action")
	}
	if err != nil {
		if errors.Is(err, lease.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "prefix lease not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !changed {
		c.Response().Header().Set("Warning", `299 - "prefix lease already `+action+`"`)
	} else {
		actor := s.actorFromContext(c)
		auditPayload := map[string]any{
			"prefixLeaseId": leaseObj.ID,
			"action":        action,
			"state":         leaseObj.State,
			"prefix":        leaseObj.Prefix,
		}
		if payload.Reason != "" {
			auditPayload["reason"] = payload.Reason
		}
		s.recordAudit(ctx, tenantID, actor, "prefix."+action, auditPayload, withResource("prefix"))
	}
	steps := buildPrefixLeaseSteps(leaseObj, action, payload.Reason, changed)
	status := "success"
	if !changed {
		status = "noop"
	}
	envelope := s.buildOperationResponse(ctx, scopeRef, "prefix."+action, status, leaseObj, changed, steps)
	return c.JSON(http.StatusOK, envelope)
}

func buildPrefixLeaseSteps(leaseObj *models.PrefixLease, action, reason string, changed bool) []models.OperationStep {
	if leaseObj == nil {
		return nil
	}
	steps := []models.OperationStep{
		{Label: "前缀", Detail: leaseObj.Prefix, Status: "info"},
		{Label: "客户端", Detail: leaseObj.ClientID, Status: "info"},
	}
	if reason != "" {
		steps = append(steps, models.OperationStep{Label: "原因", Detail: reason, Status: "warning"})
	}
	finalDetail := fmt.Sprintf("%s → %s", strings.ToUpper(action), leaseObj.State)
	finalStatus := "success"
	if !changed {
		finalDetail = "状态未改变"
		finalStatus = "warning"
	}
	steps = append(steps, models.OperationStep{Label: "生命周期", Detail: finalDetail, Status: finalStatus})
	return steps
}

func buildSecurityStateSteps(leaseObj *models.Lease, previous, next string, changed bool) []models.OperationStep {
	if leaseObj == nil {
		return nil
	}
	steps := []models.OperationStep{
		{Label: "租约", Detail: leaseObj.IPAddress, Status: "info"},
		{Label: "安全态", Detail: fmt.Sprintf("%s → %s", strings.ToUpper(previous), strings.ToUpper(next)), Status: "info"},
	}
	status := "success"
	detail := "安全态已同步"
	if !changed {
		status = "warning"
		detail = "安全态未变化"
	}
	steps = append(steps, models.OperationStep{Label: "验证", Detail: detail, Status: status})
	return steps
}

func (s *HTTPServer) handleMonitoringOverview(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	tenantID := scope.TenantOrDefault()
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	logger := LoggerFromContext(ctx, s.logger)
	overview, err := s.monitor.Overview(ctx, scope, limit)
	if err != nil {
		if logger != nil {
			logger.Error("monitoring overview failed", zap.String("tenantId", tenantID), zap.Error(err))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build monitoring overview")
	}
	return c.JSON(http.StatusOK, overview)
}

func (s *HTTPServer) handleMonitoringRealtimeSnapshot(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	health := s.monitor.Health(ctx)
	reqs := s.monitor.Requests(scope)
	success, failure, buckets := aggregateRequestStats(reqs)
	window := s.monitor.RequestWindow()
	pps := rateFromCount(success+failure, window)
	snapshot := map[string]any{
		"pps":              pps,
		"cpu":              health.CPUPercent,
		"cpuCores":         health.CPUCores,
		"memory":           health.MemoryPercent,
		"memoryUsedBytes":  health.MemoryUsedBytes,
		"memoryTotalBytes": health.MemoryTotalBytes,
		"disk":             health.DiskPercent,
		"diskUsedBytes":    health.DiskUsedBytes,
		"diskTotalBytes":   health.DiskTotalBytes,
		"latencyP50":       percentileFromBuckets(buckets, 0.5),
		"latencyP95":       percentileFromBuckets(buckets, 0.95),
		"latencyP99":       percentileFromBuckets(buckets, 0.99),
		"successRate":      computeSuccessRate(success, failure),
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleMonitoringMetrics(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	metric := strings.TrimSpace(c.Param("metric"))
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	reqs := s.monitor.Requests(scope)
	counts := requestCountsByMessage(reqs)
	success, failure, _ := aggregateRequestStats(reqs)
	window := s.monitor.RequestWindow()
	now := time.Now().UTC().Format(time.RFC3339)
	points := []map[string]any{}

	if val, ok := resolveDHCPMetric(metric, counts, window); ok {
		points = append(points, map[string]any{"timestamp": now, "value": val})
		return c.JSON(http.StatusOK, points)
	}

	switch metric {
	case "successRate":
		points = append(points, map[string]any{"timestamp": now, "value": computeSuccessRate(success, failure)})
	case "requests":
		points = append(points, map[string]any{"timestamp": now, "value": success + failure})
	default:
		points = append(points, map[string]any{"timestamp": now, "value": 0})
	}
	return c.JSON(http.StatusOK, points)
}

func (s *HTTPServer) handleMonitoringLatency(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	buckets := mergeLatencyBuckets(s.monitor.Requests(scope))
	points := make([]map[string]any, 0, len(buckets))
	for i, b := range buckets {
		label := ""
		switch {
		case b.UpperBoundMs <= 0 && i == len(buckets)-1:
			label = ">" + bucketLabel(buckets, i-1)
		case b.UpperBoundMs <= 0:
			label = ">unknown"
		default:
			label = bucketLabel(buckets, i)
		}
		points = append(points, map[string]any{
			"bucket": label,
			"value":  b.Count,
		})
	}
	return c.JSON(http.StatusOK, points)
}

func (s *HTTPServer) handleMonitoringServersPerf(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	health := s.monitor.Health(ctx)
	reqs := s.monitor.Requests(scope)
	success, failure, _ := aggregateRequestStats(reqs)
	window := s.monitor.RequestWindow()
	pps := rateFromCount(success+failure, window)
	errorRate := 0.0
	total := success + failure
	if total > 0 {
		errorRate = (float64(failure) / float64(total)) * 100
	}
	resp := []map[string]any{
		{
			"node":              "local",
			"packetsPerSecond":  pps,
			"bandwidthMbps":     ppsToMbps(pps),
			"queueDepth":        0,
			"errorRate":         errorRate,
			"threadUtilization": health.CPUPercent,
		},
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) handleMonitoringAllocationsBreakdown(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	reqs := s.monitor.Requests(scope)
	success, failure, failures := allocationFailures(reqs)
	resp := map[string]any{
		"total":    int64(success + failure),
		"success":  int64(success),
		"failures": failures,
	}
	return c.JSON(http.StatusOK, resp)
}

func aggregateRequestStats(snapshots []monitoring.RequestPhaseSnapshot) (success uint64, failure uint64, buckets []monitoring.LatencyBucket) {
	for _, snap := range snapshots {
		success += snap.Success
		failure += snap.Failure
		if len(snap.LatencyBuckets) == 0 {
			continue
		}
		if len(buckets) == 0 {
			buckets = make([]monitoring.LatencyBucket, len(snap.LatencyBuckets))
			for i, b := range snap.LatencyBuckets {
				buckets[i].UpperBoundMs = b.UpperBoundMs
			}
		}
		for i, b := range snap.LatencyBuckets {
			if i < len(buckets) {
				buckets[i].Count += b.Count
			}
		}
	}
	return success, failure, buckets
}

func mergeLatencyBuckets(snapshots []monitoring.RequestPhaseSnapshot) []monitoring.LatencyBucket {
	_, _, buckets := aggregateRequestStats(snapshots)
	return buckets
}

func percentileFromBuckets(buckets []monitoring.LatencyBucket, p float64) float64 {
	if p <= 0 || p > 1 || len(buckets) == 0 {
		return 0
	}
	var total uint64
	for _, b := range buckets {
		total += b.Count
	}
	if total == 0 {
		return 0
	}
	threshold := uint64(float64(total) * p)
	if threshold == 0 {
		threshold = 1
	}
	var cumulative uint64
	lastUpper := buckets[len(buckets)-1].UpperBoundMs
	for _, b := range buckets {
		cumulative += b.Count
		if cumulative >= threshold {
			if b.UpperBoundMs > 0 {
				return b.UpperBoundMs
			}
			if lastUpper > 0 {
				return lastUpper
			}
			return 0
		}
		if b.UpperBoundMs > 0 {
			lastUpper = b.UpperBoundMs
		}
	}
	return lastUpper
}

func rateFromCount(count uint64, window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	return float64(count) / window.Seconds()
}

func bucketLabel(buckets []monitoring.LatencyBucket, idx int) string {
	if idx < 0 || idx >= len(buckets) {
		return "unknown"
	}
	upper := buckets[idx].UpperBoundMs
	if upper <= 0 && idx > 0 {
		upper = buckets[idx-1].UpperBoundMs
	}
	if upper <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("<=%.1fms", upper)
}

func requestCountsByMessage(snapshots []monitoring.RequestPhaseSnapshot) map[string]uint64 {
	counts := make(map[string]uint64)
	for _, snap := range snapshots {
		key := strings.ToUpper(strings.TrimSpace(snap.Message))
		counts[key] += snap.Success + snap.Failure
	}
	return counts
}

func resolveDHCPMetric(metric string, counts map[string]uint64, window time.Duration) (float64, bool) {
	parts := strings.Split(metric, ".")
	if len(parts) == 3 && strings.EqualFold(parts[0], "dhcp") {
		message := strings.ToUpper(parts[1])
		kind := strings.ToLower(parts[2])
		pps := rateFromCount(counts[message], window)
		switch kind {
		case "pps":
			return pps, true
		case "bandwidth":
			return ppsToMbps(pps), true
		}
	}
	return 0, false
}

func ppsToMbps(pps float64) float64 {
	const avgPacketBytes = 300.0
	return (pps * avgPacketBytes * 8) / 1_000_000
}

func computeSuccessRate(success, failure uint64) float64 {
	total := success + failure
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func allocationFailures(snapshots []monitoring.RequestPhaseSnapshot) (success uint64, failure uint64, failures []map[string]any) {
	failureCounts := make(map[string]uint64)
	for _, snap := range snapshots {
		success += snap.Success
		failure += snap.Failure
		if snap.Failure > 0 {
			reason := failureReasonForMessage(snap.Message)
			failureCounts[reason] += snap.Failure
		}
	}
	if len(failureCounts) == 0 && failure > 0 {
		failureCounts["other"] = failure
	}
	for reason, count := range failureCounts {
		failures = append(failures, map[string]any{
			"reason": reason,
			"count":  count,
		})
	}
	sort.Slice(failures, func(i, j int) bool {
		return failures[i]["count"].(uint64) > failures[j]["count"].(uint64)
	})
	return success, failure, failures
}

func failureReasonForMessage(message string) string {
	switch strings.ToUpper(strings.TrimSpace(message)) {
	case "NAK":
		return "policy"
	case "DECLINE":
		return "conflict"
	case "REQUEST":
		return "timeout"
	default:
		return "other"
	}
}

func mostRecentRouteUpdate(routes []alerting.RoutingRule) time.Time {
	var latest time.Time
	for _, r := range routes {
		if r.UpdatedAt.After(latest) {
			latest = r.UpdatedAt
		}
	}
	return latest
}

func (s *HTTPServer) handleMonitoringPools(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	tenantID := scope.TenantOrDefault()
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	logger := LoggerFromContext(ctx, s.logger)
	pools, err := s.monitor.Pools(ctx, scope, limit)
	if err != nil {
		if logger != nil {
			logger.Error("monitoring pools failed", zap.String("tenantId", tenantID), zap.Error(err))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build pool utilization snapshot")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"pools":       pools,
	})
}

func (s *HTTPServer) handleMonitoringPoolsUsage(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	usage, err := s.monitor.Pools(ctx, scope, limit)
	if err != nil {
		if isMissingTableErr(err) {
			return c.JSON(http.StatusOK, []any{})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build pool utilization snapshot")
	}
	resp := make([]map[string]any, 0, len(usage))
	for _, u := range usage {
		resp = append(resp, map[string]any{
			"poolId":      u.PoolID,
			"poolName":    u.Name,
			"used":        u.Allocated,
			"total":       u.Capacity,
			"utilization": u.Utilization,
		})
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) handleMonitoringPoolsSummary(c echo.Context) error {
	if s.poolSvc == nil || s.leaseSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	poolScope := pool.ResourceScopeFromAccess(scope.AccessScope())
	allPools, err := listAllPools(ctx, s.poolSvc, poolScope, 200)
	if err != nil {
		if isMissingTableErr(err) {
			return c.JSON(http.StatusOK, map[string]any{
				"generatedAt": time.Now().UTC(),
				"summary": map[string]any{
					"totalPools":   0,
					"ipv4Pools":    0,
					"ipv6Pools":    0,
					"totalIps":     0,
					"allocatedIps": 0,
					"availableIps": 0,
				},
			})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load pool summary")
	}
	poolIDs := make([]string, 0, len(allPools))
	for _, poolObj := range allPools {
		poolIDs = append(poolIDs, poolObj.ID)
	}
	counts, err := s.leaseSvc.CountActiveLeasesByPool(ctx, scope, poolIDs)
	if err != nil {
		if !isMissingTableErr(err) {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load lease summary")
		}
		counts = map[string]int64{}
	}

	var totalIps int64
	var allocatedIps int64
	ipv4Pools := 0
	ipv6Pools := 0
	for _, poolObj := range allPools {
		cidr := strings.TrimSpace(poolObj.CIDR)
		isV4 := false
		parsed := false
		if cidr != "" {
			if prefix, err := netip.ParsePrefix(cidr); err == nil {
				parsed = true
				if prefix.Addr().Is4() {
					isV4 = true
					ipv4Pools++
				} else if prefix.Addr().Is6() {
					ipv6Pools++
				}
			}
		}
		if !parsed {
			if start, err := netip.ParseAddr(strings.TrimSpace(poolObj.RangeStart)); err == nil {
				if start.Is4() {
					isV4 = true
					ipv4Pools++
				} else if start.Is6() {
					ipv6Pools++
				}
			}
		}
		if isV4 {
			capacity := monitoring.CalculatePoolCapacity(poolObj)
			totalIps += capacity
			allocatedIps += counts[poolObj.ID]
		}
	}
	availableIps := totalIps - allocatedIps
	if availableIps < 0 {
		availableIps = 0
	}

	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"summary": map[string]any{
			"totalPools":   len(allPools),
			"ipv4Pools":    ipv4Pools,
			"ipv6Pools":    ipv6Pools,
			"totalIps":     totalIps,
			"allocatedIps": allocatedIps,
			"availableIps": availableIps,
		},
	})
}

func (s *HTTPServer) handleMonitoringRequests(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"requests":    s.monitor.Requests(scope),
	})
}

func (s *HTTPServer) handleMonitoringAbnormalRequests(c echo.Context) error {
	return c.JSON(http.StatusOK, []any{})
}

func (s *HTTPServer) handleMonitoringLeaseStatus(c echo.Context) error {
	ctx := c.Request().Context()
	response := map[string]any{
		"active":  0,
		"expired": 0,
		"pending": 0,
		"failed":  0,
	}
	_ = ctx
	return c.JSON(http.StatusOK, response)
}

func (s *HTTPServer) handleMonitoringAnomalies(c echo.Context) error {
	return c.JSON(http.StatusOK, []any{})
}

func (s *HTTPServer) handleMonitoringSynthetic(c echo.Context) error {
	return c.JSON(http.StatusOK, []any{})
}

func (s *HTTPServer) handleMonitoringHealth(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	ctx := c.Request().Context()
	payload := map[string]any{
		"system": s.monitor.Health(ctx),
	}
	if s.options.Coordinator != nil {
		payload["ha"] = s.options.Coordinator.Snapshot()
	}
	return c.JSON(http.StatusOK, payload)
}

func (s *HTTPServer) handleMonitoringSecurity(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	snapshot := s.monitor.Security(scope)
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"security":    snapshot,
	})
}

func (s *HTTPServer) handleMonitoringSecurityEvents(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	var window time.Duration
	if raw := strings.TrimSpace(c.QueryParam("window")); raw != "" {
		dur, parseErr := time.ParseDuration(raw)
		if parseErr != nil || dur <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "window must be a positive duration (e.g. 10m)")
		}
		window = dur
	}
	if limit <= 0 {
		limit = 50
	}
	snapshot := s.monitor.SecurityEvents(scope, window, limit)
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleMonitoringSystemHealth(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	ctx := c.Request().Context()
	resp := s.buildSystemHealthResponse(ctx)
	return c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) handleMonitoringTimeline(c echo.Context) error {
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	if limit <= 0 {
		limit = 50
	}
	items := s.timelineEntries(c.Request().Context(), tenantID, limit)
	resp := timelineResponse{
		GeneratedAt: time.Now().UTC(),
		Items:       items,
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) handleMonitoringOperations(c echo.Context) error {
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	if limit <= 0 {
		limit = 25
	}
	snapshot := s.safeOperationSnapshot(c.Request().Context(), tenantID, limit, 0)
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) requireDashboardService() (*dashboard.Service, error) {
	if s.dashboardSvc == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "dashboard disabled")
	}
	return s.dashboardSvc, nil
}

func (s *HTTPServer) handleDashboardHealth(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	summary, healthErr := svc.Health(c.Request().Context())
	if healthErr != nil {
		logger := LoggerFromContext(c.Request().Context(), s.logger)
		if logger != nil {
			logger.Warn("dashboard health degraded", zap.Error(healthErr))
		}
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard health snapshot")
	}
	return c.JSON(http.StatusOK, summary)
}

func (s *HTTPServer) handleDashboardKPIs(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	logger := LoggerFromContext(ctx, s.logger)
	snapshot, kpiErr := svc.KPIs(ctx, scope, limit)
	if kpiErr != nil {
		if logger != nil {
			logger.Warn("dashboard kpi degraded", zap.String("tenantId", tenantID), zap.Error(kpiErr))
		}
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard kpis")
	}

	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) dashboardStreamOptions(c echo.Context) (dashboard.StreamOptions, error) {
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return dashboard.StreamOptions{}, err
	}
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return dashboard.StreamOptions{}, err
	}
	includeAlerts := true
	includeOps := true
	if raw := strings.TrimSpace(c.QueryParam("types")); raw != "" {
		includeAlerts = false
		includeOps = false
		for _, token := range strings.Split(raw, ",") {
			switch strings.ToLower(strings.TrimSpace(token)) {
			case "alert", "alerts", "events":
				includeAlerts = true
			case "ops", "operations", "audit":
				includeOps = true
			}
		}
		if !includeAlerts && !includeOps {
			includeAlerts = true
			includeOps = true
		}
	}
	var since time.Time
	if raw := strings.TrimSpace(c.QueryParam("since")); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return dashboard.StreamOptions{}, echo.NewHTTPError(http.StatusBadRequest, "since must be RFC3339 timestamp")
		}
		since = parsed
	}
	return dashboard.StreamOptions{
		Scope:             scope,
		TenantID:          tenantID,
		Limit:             limit,
		IncludeAlerts:     includeAlerts,
		IncludeOperations: includeOps,
		Since:             since,
	}, nil
}

func (s *HTTPServer) handleDashboardStreams(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	options, err := s.dashboardStreamOptions(c)
	if err != nil {
		return err
	}
	snapshot, streamErr := svc.Streams(c.Request().Context(), options)
	if streamErr != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard streams")
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleDashboardStreamsLive(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	options, err := s.dashboardStreamOptions(c)
	if err != nil {
		return err
	}
	conn, err := statusStreamUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()
	conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		return nil
	})
	initial, err := svc.Streams(ctx, options)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard streams")
	}
	seenCap := options.Limit * 4
	if seenCap < 256 {
		seenCap = 256
	}
	seen := newStreamSeenSet(seenCap)
	for _, entry := range initial.Items {
		seen.remember(streamEntryKey(entry))
	}
	since := latestStreamTimestamp(initial.Items, options.Since)
	initialMsg := dashboardStreamMessage{
		Type:        "stream.snapshot",
		GeneratedAt: initial.GeneratedAt,
		TenantID:    options.TenantID,
		Snapshot:    &initial,
	}
	if err := writeDashboardStream(conn, initialMsg); err != nil {
		return nil
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			nextOpts := options
			nextOpts.Since = since
			streamSnapshot, streamErr := svc.Streams(ctx, nextOpts)
			if streamErr != nil {
				logger := LoggerFromContext(ctx, s.logger)
				if logger != nil {
					logger.Debug("dashboard stream refresh failed", zap.String("tenantId", options.TenantID), zap.Error(streamErr))
				}
				continue
			}
			fresh, latest := filterNewStreamEntries(streamSnapshot.Items, seen)
			if len(fresh) == 0 {
				continue
			}
			if latest.After(since) {
				since = latest
			}
			delta := dashboardStreamMessage{
				Type:        "stream.delta",
				GeneratedAt: streamSnapshot.GeneratedAt,
				TenantID:    options.TenantID,
				Items:       fresh,
			}
			if err := writeDashboardStream(conn, delta); err != nil {
				return nil
			}
		case <-heartbeat.C:
			if err := writeDashboardStream(conn, dashboardStreamMessage{Type: "stream.ping", GeneratedAt: time.Now().UTC(), TenantID: options.TenantID}); err != nil {
				return nil
			}
		}
	}
}

func filterNewStreamEntries(entries []dashboard.StreamEntry, seen *streamSeenSet) ([]dashboard.StreamEntry, time.Time) {
	if len(entries) == 0 {
		return nil, time.Time{}
	}
	fresh := make([]dashboard.StreamEntry, 0, len(entries))
	var latest time.Time
	for idx := len(entries) - 1; idx >= 0; idx-- {
		entry := entries[idx]
		key := streamEntryKey(entry)
		if seen != nil && seen.seen(key) {
			continue
		}
		fresh = append(fresh, entry)
		if seen != nil {
			seen.remember(key)
		}
		if entry.OccurredAt.After(latest) {
			latest = entry.OccurredAt
		}
	}
	return fresh, latest
}

func latestStreamTimestamp(entries []dashboard.StreamEntry, baseline time.Time) time.Time {
	latest := baseline
	for _, entry := range entries {
		if entry.OccurredAt.After(latest) {
			latest = entry.OccurredAt
		}
	}
	return latest
}

func writeDashboardStream(conn *websocket.Conn, msg dashboardStreamMessage) error {
	if conn == nil {
		return nil
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(msg)
}

func (s *HTTPServer) handleDashboardInsights(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	snapshot, insightErr := svc.Insights(c.Request().Context(), scope)
	logger := LoggerFromContext(c.Request().Context(), s.logger)
	if insightErr != nil && logger != nil {
		logger.Warn("dashboard insights degraded", zap.String("tenantId", tenantID), zap.Error(insightErr))
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleInsightsOverview(c echo.Context) error {
	return s.handleDashboardInsights(c)
}

func (s *HTTPServer) renderAutomationSnapshot(c echo.Context) error {
	if s.automation == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
	}
	snapshot := s.automation.Snapshot()
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleAutomationSnapshot() echo.HandlerFunc {
	return func(c echo.Context) error {
		return s.renderAutomationSnapshot(c)
	}
}

func (s *HTTPServer) handleMonitoringAutomation() echo.HandlerFunc {
	return func(c echo.Context) error {
		return s.renderAutomationSnapshot(c)
	}
}

func (s *HTTPServer) handleAutomationSchedules() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		tenantFilter := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantFilter == "" {
			tenantFilter = strings.TrimSpace(c.QueryParam("tenant"))
		}
		summaries := s.automation.Schedules()
		response := make([]automationScheduleResponse, 0, len(summaries))
		for _, summary := range summaries {
			if tenantFilter != "" {
				if strings.EqualFold(tenantFilter, "all") || tenantFilter == "*" {
					// no filtering requested
				} else if !strings.EqualFold(summary.Schedule.TenantID, tenantFilter) {
					continue
				}
			}
			response = append(response, mapAutomationSchedule(summary))
		}
		return c.JSON(http.StatusOK, response)
	}
}

func (s *HTTPServer) handleAutomationUpsertSchedule() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		logger := LoggerFromContext(c.Request().Context(), s.logger)
		jobType := automation.JobType(strings.TrimSpace(c.Param("jobType")))
		if jobType == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobType is required")
		}
		scope := s.leaseScopeRef(c)
		var payload automationScheduleUpsertRequest
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		intervalText := strings.TrimSpace(payload.Interval)
		if intervalText == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "interval is required")
		}
		interval, err := time.ParseDuration(intervalText)
		if err != nil || interval <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "interval is invalid")
		}
		initialDelayText := strings.TrimSpace(payload.InitialDelay)
		initialDelay := time.Duration(0)
		if initialDelayText != "" {
			delay, parseErr := time.ParseDuration(initialDelayText)
			if parseErr != nil || delay < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "initialDelay is invalid")
			}
			initialDelay = delay
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = strings.TrimSpace(scope.TenantOrDefault())
		}
		if tenantID == "" {
			tenantID = systemTenantID
		}
		cfg := automation.ScheduleConfig{
			Enabled:      payload.Enabled,
			Interval:     interval,
			InitialDelay: initialDelay,
			TenantID:     tenantID,
			Labels:       copyStringMap(payload.Labels),
			Payload:      cloneRawMessage(payload.Payload),
			Channels:     append([]string(nil), payload.Channels...),
		}
		current, hasCurrent := s.automation.Schedule(jobType)
		requestType := approvals.RequestTypeScheduleUpdate
		if hasCurrent && scheduleToggleOnly(current, cfg) {
			requestType = approvals.RequestTypeScheduleToggle
		}
		approvalNeeded := s.needsAutomationApproval(requestType)
		if approvalNeeded && s.automationApprovals == nil {
			if logger != nil {
				logger.Warn("automation approval required but service unavailable", zap.String("jobType", string(jobType)), zap.String("requestType", string(requestType)))
			}
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation approvals unavailable")
		}
		if approvalNeeded && s.requiresAutomationApproval(requestType) {
			proposed := buildScheduleChangeFromRequest(jobType, tenantID, intervalText, initialDelayText, payload)
			var originalRaw json.RawMessage
			if hasCurrent {
				original := buildScheduleChange(jobType, current)
				raw, marshalErr := json.Marshal(original)
				if marshalErr != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, marshalErr.Error())
				}
				originalRaw = raw
			}
			proposedRaw, marshalErr := json.Marshal(proposed)
			if marshalErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, marshalErr.Error())
			}
			change, submitErr := s.automationApprovals.SubmitChange(c.Request().Context(), approvals.SubmitChangeInput{
				TenantID:        tenantID,
				JobType:         string(jobType),
				RequestType:     requestType,
				RequestedBy:     s.actorFromContext(c),
				OriginalConfig:  originalRaw,
				ProposedConfig:  proposedRaw,
				Payload:         cloneRawMessage(payload.Payload),
				RequireApproval: true,
			})
			if submitErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, submitErr.Error())
			}
			baseSchedule := current
			if !hasCurrent {
				baseSchedule = cfg
			}
			response := mapAutomationSchedule(automation.ScheduleSummary{Type: jobType, Schedule: baseSchedule})
			if change != nil {
				if envelope := mapAutomationApproval(*change); envelope != nil {
					response.PendingApproval = envelope
				}
			}
			response.ProposedConfig = &proposed
			return c.JSON(http.StatusAccepted, response)
		}
		s.automation.UpdateSchedule(jobType, cfg)
		stored, ok := s.automation.Schedule(jobType)
		if !ok {
			stored = cfg
		}
		resp := mapAutomationSchedule(automation.ScheduleSummary{Type: jobType, Schedule: stored})
		s.recordAudit(c.Request().Context(), tenantID, s.actorFromContext(c), "automation.schedule.update", map[string]any{
			"jobType":  string(jobType),
			"enabled":  resp.Enabled,
			"interval": resp.Interval,
		}, withResource("automation_schedule"))
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleAutomationListJobs() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		store := s.automationJobStore()
		if store == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation history unavailable")
		}
		scope := s.leaseScopeRef(c)
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		limit := page.Limit
		if limit <= 0 {
			limit = 50
		}
		offset := page.Offset
		if offset < 0 {
			offset = 0
		}
		tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantID == "" {
			tenantID = strings.TrimSpace(scope.TenantOrDefault())
		}
		if tenantID == "" {
			tenantID = systemTenantID
		}
		params := c.QueryParams()
		opts := automation.ListJobsOptions{
			TenantID:    tenantID,
			Types:       automationJobTypesFromParams(params),
			Statuses:    automationJobStatusesFromParams(params),
			Sources:     automationJobSourcesFromParams(params),
			TriggeredBy: strings.TrimSpace(c.QueryParam("triggeredBy")),
			Limit:       limit,
			Offset:      offset,
		}
		runs, err := store.ListJobs(c.Request().Context(), opts)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		countOpts := opts
		countOpts.Limit = 0
		countOpts.Offset = 0
		total, err := store.CountJobs(c.Request().Context(), countOpts)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := automationJobListResponse{
			Items:  runs,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleAutomationGetJob() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		store := s.automationJobStore()
		if store == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation history unavailable")
		}
		jobID := strings.TrimSpace(c.Param("jobId"))
		if jobID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobId is required")
		}
		scope := s.leaseScopeRef(c)
		tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantID == "" {
			tenantID = strings.TrimSpace(scope.TenantOrDefault())
		}
		if tenantID == "" {
			tenantID = systemTenantID
		}
		run, err := store.GetJob(c.Request().Context(), jobID)
		if err != nil {
			if errors.Is(err, automation.ErrJobNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "job not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if tenantID != "" && !strings.EqualFold(tenantID, run.TenantID) {
			return echo.NewHTTPError(http.StatusNotFound, "job not found")
		}
		return c.JSON(http.StatusOK, run)
	}
}

func (s *HTTPServer) handleAutomationEnqueue() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		logger := LoggerFromContext(c.Request().Context(), s.logger)
		scope := s.leaseScopeRef(c)
		var req automationJobRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if strings.TrimSpace(string(req.Type)) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "type is required")
		}
		tenantID := strings.TrimSpace(req.TenantID)
		if tenantID == "" {
			tenantID = strings.TrimSpace(scope.TenantOrDefault())
		}
		if tenantID == "" {
			tenantID = systemTenantID
		}
		payload := automation.MergeChannels(cloneRawMessage(req.Payload), req.Channels)
		triggeredBy := strings.TrimSpace(req.TriggeredBy)
		if triggeredBy == "" {
			triggeredBy = s.actorFromContext(c)
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			source = automation.JobSourceManual
		}
		job := automation.Job{
			ID:          uuid.NewString(),
			TenantID:    tenantID,
			Type:        req.Type,
			Labels:      req.Labels,
			Payload:     payload,
			Priority:    req.Priority,
			Source:      source,
			TriggeredBy: triggeredBy,
		}
		if req.NotBefore != nil {
			job.NotBefore = req.NotBefore.UTC()
		}
		approvalNeeded := s.needsAutomationApproval(approvals.RequestTypeJobRun)
		if approvalNeeded && s.automationApprovals == nil {
			if logger != nil {
				logger.Warn("automation approval required but service unavailable", zap.String("jobType", string(job.Type)))
			}
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation approvals unavailable")
		}
		if s.requiresAutomationApproval(approvals.RequestTypeJobRun) {
			proposal := buildJobProposal(job, req)
			proposedRaw, marshalErr := json.Marshal(proposal)
			if marshalErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, marshalErr.Error())
			}
			change, submitErr := s.automationApprovals.SubmitChange(c.Request().Context(), approvals.SubmitChangeInput{
				TenantID:        job.TenantID,
				JobType:         string(job.Type),
				RequestType:     approvals.RequestTypeJobRun,
				RequestedBy:     triggeredBy,
				ProposedConfig:  proposedRaw,
				Payload:         cloneRawMessage(req.Payload),
				RequireApproval: true,
			})
			if submitErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, submitErr.Error())
			}
			resp := automationJobEnqueueResponse{}
			if change != nil {
				if envelope := mapAutomationApproval(*change); envelope != nil {
					resp.PendingApproval = &automationJobApprovalInfo{
						Approval: *envelope,
						Proposed: proposal,
					}
				}
			}
			return c.JSON(http.StatusAccepted, resp)
		}
		if err := s.automation.Enqueue(c.Request().Context(), job); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusAccepted, automationJobEnqueueResponse{JobID: job.ID})
	}
}

func (s *HTTPServer) handleAutomationApprovalsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireAutomationApprovals()
		if err != nil {
			return err
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		statuses, statusErr := automationApprovalStatusesFromParams(c.QueryParams())
		if statusErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, statusErr.Error())
		}
		jobTypes := automationQueryTokens(c.QueryParams(), "jobType", "jobTypes", "type", "types")
		opts := approvals.ListOptions{
			TenantID: strings.TrimSpace(c.QueryParam("tenantId")),
			JobTypes: jobTypes,
			Statuses: statuses,
			Limit:    page.Limit,
			Offset:   page.Offset,
		}
		requests, listErr := svc.List(c.Request().Context(), opts)
		if listErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
		}
		resp := automationApprovalListResponse{
			Items:  make([]automationApprovalResponse, 0, len(requests)),
			Limit:  page.Limit,
			Offset: page.Offset,
			Count:  len(requests),
		}
		for _, req := range requests {
			resp.Items = append(resp.Items, mapAutomationApprovalDetail(req))
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleAutomationApprovalsGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireAutomationApprovals()
		if err != nil {
			return err
		}
		requestID := strings.TrimSpace(c.Param("requestId"))
		if requestID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "requestId is required")
		}
		req, getErr := svc.Get(c.Request().Context(), requestID)
		if getErr != nil {
			if errors.Is(getErr, approvals.ErrRequestNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "approval request not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, getErr.Error())
		}
		return c.JSON(http.StatusOK, mapAutomationApprovalDetail(*req))
	}
}

func (s *HTTPServer) handleAutomationApprovalApprove() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireAutomationApprovals()
		if err != nil {
			return err
		}
		requestID := strings.TrimSpace(c.Param("requestId"))
		if requestID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "requestId is required")
		}
		var payload automationApprovalDecisionRequest
		if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) && !errors.Is(bindErr, io.ErrUnexpectedEOF) {
			return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
		}
		approver := strings.TrimSpace(payload.Approver)
		if approver == "" {
			approver = s.actorFromContext(c)
		}
		change, approveErr := svc.Approve(c.Request().Context(), requestID, approver, strings.TrimSpace(payload.Note))
		if approveErr != nil {
			if errors.Is(approveErr, approvals.ErrRequestNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "approval request not found")
			}
			return echo.NewHTTPError(http.StatusBadRequest, approveErr.Error())
		}
		s.recordAudit(c.Request().Context(), "global", approver, "automation.approval.approve", map[string]any{
			"requestId": change.ID,
			"jobType":   change.JobType,
			"status":    change.Status,
		}, withResource("automation_approval"))
		return c.JSON(http.StatusOK, mapAutomationApprovalDetail(*change))
	}
}

func (s *HTTPServer) handleAutomationApprovalReject() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireAutomationApprovals()
		if err != nil {
			return err
		}
		requestID := strings.TrimSpace(c.Param("requestId"))
		if requestID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "requestId is required")
		}
		var payload automationApprovalDecisionRequest
		if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) && !errors.Is(bindErr, io.ErrUnexpectedEOF) {
			return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
		}
		approver := strings.TrimSpace(payload.Approver)
		if approver == "" {
			approver = s.actorFromContext(c)
		}
		change, rejectErr := svc.Reject(c.Request().Context(), requestID, approver, strings.TrimSpace(payload.Note))
		if rejectErr != nil {
			if errors.Is(rejectErr, approvals.ErrRequestNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "approval request not found")
			}
			return echo.NewHTTPError(http.StatusBadRequest, rejectErr.Error())
		}
		s.recordAudit(c.Request().Context(), "global", approver, "automation.approval.reject", map[string]any{
			"requestId": change.ID,
			"jobType":   change.JobType,
			"status":    change.Status,
		}, withResource("automation_approval"))
		return c.JSON(http.StatusOK, mapAutomationApprovalDetail(*change))
	}
}

func (s *HTTPServer) handleAutomationApprovalApply() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireAutomationApprovals()
		if err != nil {
			return err
		}
		requestID := strings.TrimSpace(c.Param("requestId"))
		if requestID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "requestId is required")
		}
		change, applyErr := svc.Apply(c.Request().Context(), requestID)
		if applyErr != nil {
			if errors.Is(applyErr, approvals.ErrRequestNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "approval request not found")
			}
			return echo.NewHTTPError(http.StatusBadRequest, applyErr.Error())
		}
		s.recordAudit(c.Request().Context(), "global", s.actorFromContext(c), "automation.approval.apply", map[string]any{
			"requestId": change.ID,
			"jobType":   change.JobType,
			"status":    change.Status,
		}, withResource("automation_approval"))
		return c.JSON(http.StatusOK, mapAutomationApprovalDetail(*change))
	}
}

func (s *HTTPServer) requireWorkflowService() (*workflowsvc.Service, error) {
	if s.workflowSvc == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "workflow service unavailable")
	}
	return s.workflowSvc, nil
}

func (s *HTTPServer) handleWorkflowListDefinitions() echo.HandlerFunc {
	type response struct {
		Items  []workflowsvc.Definition `json:"items"`
		Total  int                      `json:"total"`
		Limit  int                      `json:"limit"`
		Offset int                      `json:"offset"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireWorkflowService()
		if err != nil {
			return err
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		statusTokens := c.QueryParams()["status"]
		if raw := strings.TrimSpace(c.QueryParam("statuses")); raw != "" {
			statusTokens = append(statusTokens, raw)
		}
		defs, listErr := svc.ListDefinitions(c.Request().Context(), workflowsvc.ListDefinitionsOptions{
			Statuses: normalizeWorkflowStatuses(statusTokens),
			Search:   strings.TrimSpace(c.QueryParam("search")),
			Limit:    page.Limit,
			Offset:   page.Offset,
		})
		if listErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
		}
		return c.JSON(http.StatusOK, response{Items: defs, Total: len(defs), Limit: page.Limit, Offset: page.Offset})
	}
}

func (s *HTTPServer) handleWorkflowCreateDefinition() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireWorkflowService()
		if err != nil {
			return err
		}
		var payload workflowDefinitionRequest
		if bindErr := c.Bind(&payload); bindErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
		}
		if strings.TrimSpace(payload.Name) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		if specErr := payload.Spec.Validate(); specErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, specErr.Error())
		}
		if payload.Status != "" {
			if normalized, ok := workflowStatusFromValue(string(payload.Status)); ok {
				payload.Status = normalized
			} else {
				return echo.NewHTTPError(http.StatusBadRequest, "status is invalid")
			}
		}
		def, createErr := svc.CreateDefinition(c.Request().Context(), workflowsvc.CreateDefinitionInput{
			ID:          strings.TrimSpace(payload.ID),
			Name:        strings.TrimSpace(payload.Name),
			Description: strings.TrimSpace(payload.Description),
			Labels:      payload.Labels,
			Spec:        payload.Spec,
			Status:      payload.Status,
		})
		if createErr != nil {
			switch {
			case errors.Is(createErr, workflowsvc.ErrDefinitionConflict):
				return echo.NewHTTPError(http.StatusConflict, createErr.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, createErr.Error())
			}
		}
		return c.JSON(http.StatusCreated, def)
	}
}

func (s *HTTPServer) handleWorkflowPublishDefinition() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireWorkflowService()
		if err != nil {
			return err
		}
		definitionID := strings.TrimSpace(c.Param("definitionId"))
		if definitionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "definitionId is required")
		}
		var payload workflowPublishRequest
		if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
		}
		def, publishErr := svc.PublishDefinition(c.Request().Context(), definitionID, payload.Version)
		if publishErr != nil {
			switch {
			case errors.Is(publishErr, workflowsvc.ErrDefinitionNotFound):
				return echo.NewHTTPError(http.StatusNotFound, publishErr.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, publishErr.Error())
			}
		}
		return c.JSON(http.StatusOK, def)
	}
}

func (s *HTTPServer) handleWorkflowStartExecution() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireWorkflowService()
		if err != nil {
			return err
		}
		scope := s.leaseScopeRef(c)
		var payload workflowExecutionRequest
		if bindErr := c.Bind(&payload); bindErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
		}
		payload.DefinitionID = strings.TrimSpace(payload.DefinitionID)
		if payload.DefinitionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "definitionId is required")
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = strings.TrimSpace(scope.TenantOrDefault())
		}
		if tenantID == "" {
			tenantID = systemTenantID
		}
		triggeredBy := strings.TrimSpace(payload.TriggeredBy)
		if triggeredBy == "" {
			triggeredBy = s.actorFromContext(c)
		}
		jobID, execErr := svc.StartExecution(c.Request().Context(), workflowsvc.StartExecutionInput{
			DefinitionID: payload.DefinitionID,
			Version:      payload.Version,
			TenantID:     tenantID,
			TriggeredBy:  triggeredBy,
			Context:      payload.Context,
			Labels:       payload.Labels,
			Priority:     payload.Priority,
		})
		if execErr != nil {
			switch {
			case errors.Is(execErr, workflowsvc.ErrDefinitionNotFound):
				return echo.NewHTTPError(http.StatusNotFound, execErr.Error())
			case errors.Is(execErr, workflowsvc.ErrDefinitionInactive):
				return echo.NewHTTPError(http.StatusConflict, execErr.Error())
			case errors.Is(execErr, workflowsvc.ErrDispatcherUnavailable):
				return echo.NewHTTPError(http.StatusServiceUnavailable, execErr.Error())
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, execErr.Error())
			}
		}
		return c.JSON(http.StatusAccepted, map[string]string{"jobId": jobID})
	}
}

func (s *HTTPServer) handleAlertFeed(c echo.Context) error {
	if s.alertFeed == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert feed unavailable")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	snapshot := s.alertFeed.Snapshot(tenantID, limit)
	if len(snapshot.Alerts) == 0 {
		snapshot = s.sampleAlertSnapshot(tenantID, limit)
	}
	return c.JSON(http.StatusOK, snapshot)
}

type alertRuleDTO struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	Duration  int     `json:"duration"`
	Severity  string  `json:"severity"`
	Enabled   bool    `json:"enabled"`
	Match     string  `json:"match,omitempty"`
	TenantID  string  `json:"tenantId,omitempty"`
}

type notificationChannelDTO struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Target  string            `json:"target"`
	Method  string            `json:"method,omitempty"`
	Secret  string            `json:"secret,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Enabled bool              `json:"enabled"`
}

type alertThresholdConfig struct {
	Resource struct {
		PoolUsage          int `json:"poolUsage"`
		LeaseUsage         int `json:"leaseUsage"`
		RenewFail          int `json:"renewFail"`
		LeaseTimeDrift     int `json:"leaseTimeDrift"`
		FailedRequestRatio int `json:"failedRequestRatio"`
		SubnetImbalance    int `json:"subnetImbalance"`
		LogErrorThreshold  int `json:"logErrorThreshold"`
	} `json:"resource"`
	Server struct {
		ResponseTimeout int  `json:"responseTimeout"`
		ResponseTimeMs  int  `json:"responseTimeMs"`
		CPUUsage        int  `json:"cpuUsage"`
		MemoryUsage     int  `json:"memoryUsage"`
		ProcessCheck    bool `json:"processCheck"`
	} `json:"server"`
	Network struct {
		ConflictSensitivity         string `json:"conflictSensitivity"`
		AbnormalQps                 int    `json:"abnormalQps"`
		DuplicateIpDetection        bool   `json:"duplicateIpDetection"`
		UnauthorizedServerDetection bool   `json:"unauthorizedServerDetection"`
	} `json:"network"`
}

type alertNotifyConfig struct {
	Channels struct {
		Email      bool   `json:"email"`
		Sms        bool   `json:"sms"`
		Webhook    bool   `json:"webhook"`
		WebhookURL string `json:"webhookUrl,omitempty"`
	} `json:"channels"`
	Policies struct {
		Emergency []string `json:"emergency"`
		Critical  []string `json:"critical"`
		Info      []string `json:"info"`
	} `json:"policies"`
}

type alertTemplateDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Lang      string    `json:"lang"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject,omitempty"`
	Body      string    `json:"body,omitempty"`
	Variables []string  `json:"variables,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	TenantID  string    `json:"tenantId,omitempty"`
}

type alertReceiverDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone,omitempty"`
	Levels       []string `json:"levels"`
	Department   string   `json:"department,omitempty"`
	Schedule     string   `json:"schedule,omitempty"`
	ServerGroups []string `json:"serverGroups,omitempty"`
	TenantID     string   `json:"tenantId,omitempty"`
}

type alertAnalyticsSummaryDTO struct {
	Total          int    `json:"total"`
	MTTR           string `json:"mttr"`
	TopType        string `json:"topType"`
	ResolutionRate int    `json:"resolutionRate"`
}

type alertTypeDistributionDTO struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type alertTimeDistributionDTO struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type alertTrendPointDTO struct {
	TS        string `json:"ts"`
	Emergency int    `json:"emergency"`
	Critical  int    `json:"critical"`
	Warning   int    `json:"warning"`
	Info      int    `json:"info"`
}

type alertTrendCompareDTO struct {
	Current   []alertTrendCount `json:"current"`
	Previous  []alertTrendCount `json:"previous"`
	Anomalies []alertAnomaly    `json:"anomalies,omitempty"`
}

type alertTrendCount struct {
	TS    string `json:"ts"`
	Count int    `json:"count"`
}

type alertAnomaly struct {
	TS   string `json:"ts"`
	Desc string `json:"desc"`
}

type alertActiveSummaryDTO struct {
	Active    int `json:"active"`
	Emergency int `json:"emergency"`
	Critical  int `json:"critical"`
	Warning   int `json:"warning"`
	Info      int `json:"info"`
	Servers   struct {
		Healthy int `json:"healthy"`
		Total   int `json:"total"`
	} `json:"servers"`
}

type AlertHistorySummary struct {
	Total          int `json:"total"`
	ResolutionRate int `json:"resolutionRate"`
	InProgress     int `json:"inProgress"`
}

func defaultAlertThresholds() alertThresholdConfig {
	var cfg alertThresholdConfig
	cfg.Resource.PoolUsage = 85
	cfg.Resource.LeaseUsage = 90
	cfg.Resource.RenewFail = 10
	cfg.Resource.LeaseTimeDrift = 30
	cfg.Resource.FailedRequestRatio = 5
	cfg.Resource.SubnetImbalance = 20
	cfg.Resource.LogErrorThreshold = 50
	cfg.Server.ResponseTimeout = 5
	cfg.Server.ResponseTimeMs = 200
	cfg.Server.CPUUsage = 85
	cfg.Server.MemoryUsage = 85
	cfg.Server.ProcessCheck = true
	cfg.Network.ConflictSensitivity = "medium"
	cfg.Network.AbnormalQps = 120
	cfg.Network.DuplicateIpDetection = true
	cfg.Network.UnauthorizedServerDetection = true
	return cfg
}

func (s *HTTPServer) loadAlertThresholds(ctx context.Context, tenantKey string) (alertThresholdConfig, error) {
	if s.alertConfigRepo != nil {
		cfg, err := s.alertConfigRepo.GetThresholds(ctx, tenantKey)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return defaultAlertThresholds(), nil
			}
			return alertThresholdConfig{}, err
		}
		return alertThresholdConfig{
			Resource: struct {
				PoolUsage          int `json:"poolUsage"`
				LeaseUsage         int `json:"leaseUsage"`
				RenewFail          int `json:"renewFail"`
				LeaseTimeDrift     int `json:"leaseTimeDrift"`
				FailedRequestRatio int `json:"failedRequestRatio"`
				SubnetImbalance    int `json:"subnetImbalance"`
				LogErrorThreshold  int `json:"logErrorThreshold"`
			}{
				PoolUsage:          cfg.ResourcePoolUsage,
				LeaseUsage:         cfg.ResourceLeaseUsage,
				RenewFail:          cfg.ResourceRenewFail,
				LeaseTimeDrift:     cfg.ResourceLeaseTimeDrift,
				FailedRequestRatio: cfg.ResourceFailedRequestRatio,
				SubnetImbalance:    cfg.ResourceSubnetImbalance,
				LogErrorThreshold:  cfg.ResourceLogErrorThreshold,
			},
			Server: struct {
				ResponseTimeout int  `json:"responseTimeout"`
				ResponseTimeMs  int  `json:"responseTimeMs"`
				CPUUsage        int  `json:"cpuUsage"`
				MemoryUsage     int  `json:"memoryUsage"`
				ProcessCheck    bool `json:"processCheck"`
			}{
				ResponseTimeout: cfg.ServerResponseTimeout,
				ResponseTimeMs:  cfg.ServerResponseTimeMs,
				CPUUsage:        cfg.ServerCPUUsage,
				MemoryUsage:     cfg.ServerMemoryUsage,
				ProcessCheck:    cfg.ServerProcessCheck,
			},
			Network: struct {
				ConflictSensitivity         string `json:"conflictSensitivity"`
				AbnormalQps                 int    `json:"abnormalQps"`
				DuplicateIpDetection        bool   `json:"duplicateIpDetection"`
				UnauthorizedServerDetection bool   `json:"unauthorizedServerDetection"`
			}{
				ConflictSensitivity:         cfg.NetworkConflictSensitivity,
				AbnormalQps:                 cfg.NetworkAbnormalQPS,
				DuplicateIpDetection:        cfg.NetworkDuplicateIPDetection,
				UnauthorizedServerDetection: cfg.NetworkUnauthorizedDHCP,
			},
		}, nil
	}

	s.alertConfigMu.RLock()
	defer s.alertConfigMu.RUnlock()
	cfg, ok := s.alertThresholdStore[tenantKey]
	if !ok {
		cfg = defaultAlertThresholds()
	}
	return cfg, nil
}

func (s *HTTPServer) saveAlertThresholds(ctx context.Context, tenantKey string, payload alertThresholdConfig) error {
	if strings.TrimSpace(tenantKey) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId required")
	}
	if s.alertConfigRepo != nil {
		repoCfg := alerting.Thresholds{
			TenantID:                    tenantKey,
			ResourcePoolUsage:           payload.Resource.PoolUsage,
			ResourceLeaseUsage:          payload.Resource.LeaseUsage,
			ResourceRenewFail:           payload.Resource.RenewFail,
			ResourceLeaseTimeDrift:      payload.Resource.LeaseTimeDrift,
			ResourceFailedRequestRatio:  payload.Resource.FailedRequestRatio,
			ResourceSubnetImbalance:     payload.Resource.SubnetImbalance,
			ResourceLogErrorThreshold:   payload.Resource.LogErrorThreshold,
			ServerResponseTimeout:       payload.Server.ResponseTimeout,
			ServerResponseTimeMs:        payload.Server.ResponseTimeMs,
			ServerCPUUsage:              payload.Server.CPUUsage,
			ServerMemoryUsage:           payload.Server.MemoryUsage,
			ServerProcessCheck:          payload.Server.ProcessCheck,
			NetworkConflictSensitivity:  payload.Network.ConflictSensitivity,
			NetworkAbnormalQPS:          payload.Network.AbnormalQps,
			NetworkDuplicateIPDetection: payload.Network.DuplicateIpDetection,
			NetworkUnauthorizedDHCP:     payload.Network.UnauthorizedServerDetection,
			UpdatedAt:                   time.Now().UTC(),
			UpdatedBy:                   "system",
		}
		return s.alertConfigRepo.SaveThresholds(ctx, repoCfg)
	}

	s.alertConfigMu.Lock()
	s.alertThresholdStore[tenantKey] = payload
	s.alertConfigMu.Unlock()
	return nil
}

func (s *HTTPServer) loadAlertNotify(ctx context.Context, tenantKey string) (alertNotifyConfig, error) {
	if s.alertConfigRepo != nil {
		cfg, err := s.alertConfigRepo.GetNotify(ctx, tenantKey)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return defaultAlertNotify(), nil
			}
			return alertNotifyConfig{}, err
		}
		return alertNotifyConfig{
			Channels: struct {
				Email      bool   `json:"email"`
				Sms        bool   `json:"sms"`
				Webhook    bool   `json:"webhook"`
				WebhookURL string `json:"webhookUrl,omitempty"`
			}{
				Email:      cfg.Channels.Email,
				Sms:        cfg.Channels.SMS,
				Webhook:    cfg.Channels.Webhook,
				WebhookURL: cfg.WebhookURL,
			},
			Policies: struct {
				Emergency []string `json:"emergency"`
				Critical  []string `json:"critical"`
				Info      []string `json:"info"`
			}{
				Emergency: cfg.Policies.Emergency,
				Critical:  cfg.Policies.Critical,
				Info:      cfg.Policies.Info,
			},
		}, nil
	}

	s.alertConfigMu.RLock()
	defer s.alertConfigMu.RUnlock()
	cfg, ok := s.alertNotifyStore[tenantKey]
	if !ok {
		cfg = defaultAlertNotify()
	}
	return cfg, nil
}

func (s *HTTPServer) saveAlertNotify(ctx context.Context, tenantKey string, payload alertNotifyConfig) error {
	if strings.TrimSpace(tenantKey) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId required")
	}
	if s.alertConfigRepo != nil {
		repoCfg := alerting.NotifyConfig{
			TenantID:   tenantKey,
			Channels:   alerting.Channels{Email: payload.Channels.Email, SMS: payload.Channels.Sms, Webhook: payload.Channels.Webhook},
			Policies:   alerting.Policies{Emergency: payload.Policies.Emergency, Critical: payload.Policies.Critical, Info: payload.Policies.Info},
			WebhookURL: strings.TrimSpace(payload.Channels.WebhookURL),
			UpdatedAt:  time.Now().UTC(),
			UpdatedBy:  "system",
		}
		return s.alertConfigRepo.SaveNotify(ctx, repoCfg)
	}
	s.alertConfigMu.Lock()
	s.alertNotifyStore[tenantKey] = payload
	s.alertConfigMu.Unlock()
	return nil
}

func (s *HTTPServer) listAlertTemplates(ctx context.Context, tenantKey string) ([]alertTemplateDTO, error) {
	if s.alertTemplateRepo != nil {
		templates, err := s.alertTemplateRepo.ListTemplates(ctx, tenantKey)
		if err != nil {
			return nil, err
		}
		items := make([]alertTemplateDTO, 0, len(templates))
		for _, t := range templates {
			items = append(items, alertTemplateDTO{
				ID:        t.ID,
				Name:      t.Name,
				Lang:      t.Lang,
				Channel:   t.Channel,
				Subject:   t.Subject,
				Body:      t.Body,
				Variables: append([]string(nil), t.Variables...),
				UpdatedAt: t.UpdatedAt,
				TenantID:  t.TenantID,
			})
		}
		return items, nil
	}
	s.alertConfigMu.RLock()
	items := append([]alertTemplateDTO(nil), s.alertTemplateStore[tenantKey]...)
	s.alertConfigMu.RUnlock()
	return items, nil
}

func (s *HTTPServer) createAlertTemplate(ctx context.Context, tenantKey string, payload alertTemplateDTO) (alertTemplateDTO, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return alertTemplateDTO{}, echo.NewHTTPError(http.StatusBadRequest, "template name required")
	}
	if strings.TrimSpace(payload.Channel) == "" {
		payload.Channel = "email"
	}
	if payload.ID == "" {
		payload.ID = uuid.NewString()
	}
	if payload.Lang == "" {
		payload.Lang = "zh-CN"
	}
	payload.TenantID = tenantKey
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = time.Now().UTC()
	}
	if s.alertTemplateRepo != nil {
		tpl := alerting.Template{
			ID:        payload.ID,
			TenantID:  tenantKey,
			Name:      strings.TrimSpace(payload.Name),
			Lang:      strings.TrimSpace(payload.Lang),
			Channel:   strings.TrimSpace(payload.Channel),
			Subject:   strings.TrimSpace(payload.Subject),
			Body:      strings.TrimSpace(payload.Body),
			Variables: append([]string(nil), payload.Variables...),
			UpdatedAt: payload.UpdatedAt,
			UpdatedBy: "system",
		}
		if err := s.alertTemplateRepo.CreateTemplate(ctx, &tpl); err != nil {
			return alertTemplateDTO{}, err
		}
		payload.UpdatedAt = tpl.UpdatedAt
		return payload, nil
	}

	s.alertConfigMu.Lock()
	s.alertTemplateStore[tenantKey] = append(s.alertTemplateStore[tenantKey], payload)
	s.alertConfigMu.Unlock()
	return payload, nil
}

func (s *HTTPServer) updateAlertTemplate(ctx context.Context, tenantKey, id string, payload alertTemplateDTO) (alertTemplateDTO, error) {
	if strings.TrimSpace(id) == "" {
		return alertTemplateDTO{}, echo.NewHTTPError(http.StatusBadRequest, "templateId required")
	}
	if s.alertTemplateRepo != nil {
		tpl := alerting.Template{
			ID:        id,
			TenantID:  tenantKey,
			Name:      strings.TrimSpace(payload.Name),
			Lang:      strings.TrimSpace(payload.Lang),
			Channel:   strings.TrimSpace(payload.Channel),
			Subject:   strings.TrimSpace(payload.Subject),
			Body:      strings.TrimSpace(payload.Body),
			Variables: append([]string(nil), payload.Variables...),
			UpdatedAt: payload.UpdatedAt,
			UpdatedBy: "system",
		}
		if tpl.UpdatedAt.IsZero() {
			tpl.UpdatedAt = time.Now().UTC()
		}
		if err := s.alertTemplateRepo.UpdateTemplate(ctx, &tpl); err != nil {
			return alertTemplateDTO{}, err
		}
		payload.ID = id
		payload.TenantID = tenantKey
		if payload.UpdatedAt.IsZero() {
			payload.UpdatedAt = tpl.UpdatedAt
		}
		return payload, nil
	}

	s.alertConfigMu.Lock()
	defer s.alertConfigMu.Unlock()
	items := s.alertTemplateStore[tenantKey]
	for idx := range items {
		if items[idx].ID == id {
			payload.ID = id
			payload.TenantID = tenantKey
			if payload.UpdatedAt.IsZero() {
				payload.UpdatedAt = time.Now().UTC()
			}
			items[idx] = payload
			s.alertTemplateStore[tenantKey] = items
			return payload, nil
		}
	}
	return alertTemplateDTO{}, echo.NewHTTPError(http.StatusNotFound, "template not found")
}

func (s *HTTPServer) deleteAlertTemplate(ctx context.Context, tenantKey, id string) error {
	if strings.TrimSpace(id) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "templateId required")
	}
	if s.alertTemplateRepo != nil {
		return s.alertTemplateRepo.DeleteTemplate(ctx, tenantKey, id)
	}
	s.alertConfigMu.Lock()
	items := s.alertTemplateStore[tenantKey]
	for idx := range items {
		if items[idx].ID == id {
			s.alertTemplateStore[tenantKey] = append(items[:idx], items[idx+1:]...)
			s.alertConfigMu.Unlock()
			return nil
		}
	}
	s.alertConfigMu.Unlock()
	return echo.NewHTTPError(http.StatusNotFound, "template not found")
}

func (s *HTTPServer) listAlertReceivers(ctx context.Context, tenantKey string) ([]alertReceiverDTO, error) {
	if s.alertReceiverRepo != nil {
		receivers, err := s.alertReceiverRepo.ListReceivers(ctx, tenantKey)
		if err != nil {
			return nil, err
		}
		items := make([]alertReceiverDTO, 0, len(receivers))
		for _, r := range receivers {
			items = append(items, alertReceiverDTO{
				ID:           r.ID,
				Name:         r.Name,
				Email:        r.Email,
				Phone:        r.Phone,
				Levels:       append([]string(nil), r.Levels...),
				Department:   r.Department,
				Schedule:     r.Schedule,
				ServerGroups: append([]string(nil), r.ServerGroups...),
				TenantID:     r.TenantID,
			})
		}
		return items, nil
	}
	s.alertConfigMu.RLock()
	items := append([]alertReceiverDTO(nil), s.alertReceiverStore[tenantKey]...)
	s.alertConfigMu.RUnlock()
	return items, nil
}

func (s *HTTPServer) createAlertReceiver(ctx context.Context, tenantKey string, payload alertReceiverDTO) (alertReceiverDTO, error) {
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Email) == "" {
		return alertReceiverDTO{}, echo.NewHTTPError(http.StatusBadRequest, "name and email required")
	}
	if payload.ID == "" {
		payload.ID = uuid.NewString()
	}
	if len(payload.Levels) == 0 {
		payload.Levels = []string{"emergency"}
	}
	payload.TenantID = tenantKey
	if s.alertReceiverRepo != nil {
		rec := alerting.Receiver{
			ID:           payload.ID,
			TenantID:     tenantKey,
			Name:         strings.TrimSpace(payload.Name),
			Email:        strings.TrimSpace(payload.Email),
			Phone:        strings.TrimSpace(payload.Phone),
			Levels:       append([]string(nil), payload.Levels...),
			Department:   strings.TrimSpace(payload.Department),
			Schedule:     strings.TrimSpace(payload.Schedule),
			ServerGroups: append([]string(nil), payload.ServerGroups...),
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			UpdatedBy:    "system",
		}
		if err := s.alertReceiverRepo.CreateReceiver(ctx, &rec); err != nil {
			return alertReceiverDTO{}, err
		}
		return payload, nil
	}
	s.alertConfigMu.Lock()
	s.alertReceiverStore[tenantKey] = append(s.alertReceiverStore[tenantKey], payload)
	s.alertConfigMu.Unlock()
	return payload, nil
}

func (s *HTTPServer) updateAlertReceiver(ctx context.Context, tenantKey, id string, payload alertReceiverDTO) (alertReceiverDTO, error) {
	if strings.TrimSpace(id) == "" {
		return alertReceiverDTO{}, echo.NewHTTPError(http.StatusBadRequest, "receiverId required")
	}
	if s.alertReceiverRepo != nil {
		rec := alerting.Receiver{
			ID:           id,
			TenantID:     tenantKey,
			Name:         strings.TrimSpace(payload.Name),
			Email:        strings.TrimSpace(payload.Email),
			Phone:        strings.TrimSpace(payload.Phone),
			Levels:       append([]string(nil), payload.Levels...),
			Department:   strings.TrimSpace(payload.Department),
			Schedule:     strings.TrimSpace(payload.Schedule),
			ServerGroups: append([]string(nil), payload.ServerGroups...),
			UpdatedAt:    time.Now().UTC(),
			UpdatedBy:    "system",
		}
		if err := s.alertReceiverRepo.UpdateReceiver(ctx, &rec); err != nil {
			return alertReceiverDTO{}, err
		}
		payload.ID = id
		payload.TenantID = tenantKey
		return payload, nil
	}

	s.alertConfigMu.Lock()
	defer s.alertConfigMu.Unlock()
	items := s.alertReceiverStore[tenantKey]
	for idx := range items {
		if items[idx].ID == id {
			payload.ID = id
			payload.TenantID = tenantKey
			items[idx] = payload
			s.alertReceiverStore[tenantKey] = items
			return payload, nil
		}
	}
	return alertReceiverDTO{}, echo.NewHTTPError(http.StatusNotFound, "receiver not found")
}

func (s *HTTPServer) deleteAlertReceiver(ctx context.Context, tenantKey, id string) error {
	if strings.TrimSpace(id) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "receiverId required")
	}
	if s.alertReceiverRepo != nil {
		return s.alertReceiverRepo.DeleteReceiver(ctx, tenantKey, id)
	}
	s.alertConfigMu.Lock()
	items := s.alertReceiverStore[tenantKey]
	for idx := range items {
		if items[idx].ID == id {
			s.alertReceiverStore[tenantKey] = append(items[:idx], items[idx+1:]...)
			s.alertConfigMu.Unlock()
			return nil
		}
	}
	s.alertConfigMu.Unlock()
	return echo.NewHTTPError(http.StatusNotFound, "receiver not found")
}

func defaultAlertNotify() alertNotifyConfig {
	var cfg alertNotifyConfig
	cfg.Channels.Email = true
	cfg.Channels.Sms = false
	cfg.Channels.Webhook = true
	cfg.Channels.WebhookURL = "https://hooks.example.com/alerts"
	cfg.Policies.Emergency = []string{"email", "sms", "webhook"}
	cfg.Policies.Critical = []string{"email", "webhook"}
	cfg.Policies.Info = []string{"email"}
	return cfg
}

func (s *HTTPServer) bootstrapAlertConfig() {
	if s == nil {
		return
	}
	tenantKey := strings.ToLower(systemTenantID)
	s.alertConfigMu.Lock()
	defer s.alertConfigMu.Unlock()
	if len(s.alertThresholdStore) == 0 {
		s.alertThresholdStore[tenantKey] = defaultAlertThresholds()
	}
	if len(s.alertNotifyStore) == 0 {
		s.alertNotifyStore[tenantKey] = defaultAlertNotify()
	}
	if len(s.alertTemplateStore[tenantKey]) == 0 {
		s.alertTemplateStore[tenantKey] = []alertTemplateDTO{
			{ID: uuid.NewString(), Name: "默认邮件模板", Lang: "zh-CN", Channel: "email", Subject: "DHCP 告警", UpdatedAt: time.Now().UTC(), TenantID: systemTenantID},
			{ID: uuid.NewString(), Name: "English template", Lang: "en-US", Channel: "email", Subject: "DHCP Alert", UpdatedAt: time.Now().Add(-48 * time.Hour).UTC(), TenantID: systemTenantID},
		}
	}
	if len(s.alertReceiverStore[tenantKey]) == 0 {
		s.alertReceiverStore[tenantKey] = []alertReceiverDTO{
			{ID: uuid.NewString(), Name: "张三", Email: "zhangsan@example.com", Phone: "13800000000", Levels: []string{"emergency", "critical"}, Department: "网络运维", TenantID: systemTenantID},
			{ID: uuid.NewString(), Name: "李四", Email: "lisi@example.com", Phone: "13900000000", Levels: []string{"all"}, Department: "平台值守", TenantID: systemTenantID},
		}
	}
}

type reportTaskDTO struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	DownloadURL string    `json:"downloadUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	TenantID    string    `json:"tenantId,omitempty"`
	Format      string    `json:"format,omitempty"`
	Range       struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"range,omitempty"`
}

func mapLifecycleToStatus(l monitoring.AlertLifecycle) string {
	switch l {
	case monitoring.AlertLifecycleAcknowledged:
		return "ack"
	case monitoring.AlertLifecycleSuppressed:
		return "resolved"
	case monitoring.AlertLifecycleEscalated:
		return "firing"
	default:
		return "firing"
	}
}

func (s *HTTPServer) handleAlertRules(c echo.Context) error {
	if s.alertController == nil && s.alertRuleRepo == nil && s.alertRuleStore == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alerting disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	page, size, err := parsePageParams(c)
	if err != nil {
		return err
	}
	keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))

	items := make([]alertRuleDTO, 0)
	controllerCount := 0
	if s.alertController != nil {
		for _, r := range s.alertController.Rules() {
			items = append(items, alertRuleDTO{
				ID:        r.ID,
				Name:      r.Name,
				Metric:    r.Expression,
				Operator:  r.Operator,
				Threshold: r.Threshold,
				Duration:  r.DurationSeconds,
				Severity:  string(r.Severity),
				Enabled:   true,
				Match:     r.DetailTemplate,
				TenantID:  tenantID,
			})
		}
		controllerCount = len(items)
	}
	if s.alertRuleRepo != nil {
		ctx := c.Request().Context()
		offset := (page - 1) * size
		rules, total, listErr := s.alertRuleRepo.ListRules(ctx, tenantID, keyword, size, offset)
		if listErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
		}
		for _, r := range rules {
			items = append(items, alertRuleDTO{
				ID:        r.ID,
				Name:      r.Name,
				Metric:    r.Expression,
				Operator:  r.Operator,
				Threshold: r.Threshold,
				Duration:  r.DurationSec,
				Severity:  r.Severity,
				Enabled:   r.Enabled,
				Match:     r.MatchTemplate,
				TenantID:  r.TenantID,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"items": items,
			"total": total + controllerCount,
		})
	}

	// fallback to in-memory store
	s.alertRuleMu.Lock()
	if stored := s.alertRuleStore[strings.ToLower(tenantID)]; len(stored) > 0 {
		items = append(items, stored...)
	}
	s.alertRuleMu.Unlock()
	filtered := make([]alertRuleDTO, 0, len(items))
	for _, it := range items {
		if keyword != "" && !strings.Contains(strings.ToLower(it.Name), keyword) && !strings.Contains(strings.ToLower(it.Metric), keyword) {
			continue
		}
		filtered = append(filtered, it)
	}
	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items": filtered[start:end],
		"total": total,
	})
}

func (s *HTTPServer) handleAlertRuleUpsert(c echo.Context) error {
	if s.alertFeed == nil && s.alertController == nil && s.alertRuleRepo == nil && s.alertRuleStore == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alerting disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	var payload alertRuleDTO
	if bindErr := c.Bind(&payload); bindErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Metric = strings.TrimSpace(payload.Metric)
	payload.Operator = strings.TrimSpace(payload.Operator)
	payload.Severity = strings.TrimSpace(payload.Severity)
	if payload.Name == "" || payload.Metric == "" || payload.Operator == "" || payload.Severity == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, metric, operator and severity are required")
	}
	if payload.Threshold <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "threshold must be > 0")
	}
	if payload.Duration <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "duration must be > 0")
	}
	hadRuleID := strings.TrimSpace(payload.ID) != ""
	if payload.ID == "" {
		payload.ID = uuid.NewString()
	}
	payload.TenantID = tenantID
	if payload.Enabled == false {
		payload.Enabled = true
	}
	if s.alertRuleRepo != nil {
		rule := alerting.Rule{
			ID:            payload.ID,
			TenantID:      payload.TenantID,
			Name:          payload.Name,
			Expression:    payload.Metric,
			Operator:      payload.Operator,
			Threshold:     payload.Threshold,
			DurationSec:   payload.Duration,
			Severity:      payload.Severity,
			Enabled:       payload.Enabled,
			MatchTemplate: payload.Match,
		}
		if err := s.alertRuleRepo.UpsertRule(c.Request().Context(), &rule); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.reloadAlertControllerRules(c.Request().Context(), tenantID)
		action := "alert.rule.create"
		if hadRuleID {
			action = "alert.rule.update"
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), action, s.auditPayloadFromRequest(c, map[string]any{
			"ruleId":    payload.ID,
			"name":      payload.Name,
			"metric":    payload.Metric,
			"operator":  payload.Operator,
			"threshold": payload.Threshold,
			"duration":  payload.Duration,
			"severity":  payload.Severity,
			"enabled":   payload.Enabled,
		}), withResource("alert_rule"))
		return c.JSON(http.StatusOK, payload)
	}
	tenantKey := strings.ToLower(tenantID)
	s.alertRuleMu.Lock()
	defer s.alertRuleMu.Unlock()
	existing := s.alertRuleStore[tenantKey]
	replaced := false
	for idx, r := range existing {
		if r.ID == payload.ID {
			existing[idx] = payload
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, payload)
	}
	s.alertRuleStore[tenantKey] = existing
	action := "alert.rule.create"
	if replaced {
		action = "alert.rule.update"
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), action, s.auditPayloadFromRequest(c, map[string]any{
		"ruleId":    payload.ID,
		"name":      payload.Name,
		"metric":    payload.Metric,
		"operator":  payload.Operator,
		"threshold": payload.Threshold,
		"duration":  payload.Duration,
		"severity":  payload.Severity,
		"enabled":   payload.Enabled,
	}), withResource("alert_rule"))
	return c.JSON(http.StatusOK, payload)
}

func (s *HTTPServer) handleAlertRuleDelete(c echo.Context) error {
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	ruleID := strings.TrimSpace(c.Param("ruleId"))
	if ruleID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
	}
	if s.alertRuleRepo != nil {
		if err := s.alertRuleRepo.DeleteRule(c.Request().Context(), tenantID, ruleID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "rule not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.reloadAlertControllerRules(c.Request().Context(), tenantID)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.rule.delete", s.auditPayloadFromRequest(c, map[string]any{
			"ruleId": ruleID,
		}), withResource("alert_rule"))
		return c.NoContent(http.StatusNoContent)
	}
	tenantKey := strings.ToLower(tenantID)
	s.alertRuleMu.Lock()
	defer s.alertRuleMu.Unlock()
	items := s.alertRuleStore[tenantKey]
	filtered := make([]alertRuleDTO, 0, len(items))
	removed := false
	for _, it := range items {
		if it.ID == ruleID {
			removed = true
			continue
		}
		filtered = append(filtered, it)
	}
	if !removed {
		return echo.NewHTTPError(http.StatusNotFound, "rule not found")
	}
	s.alertRuleStore[tenantKey] = filtered
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.rule.delete", s.auditPayloadFromRequest(c, map[string]any{
		"ruleId": ruleID,
	}), withResource("alert_rule"))
	return c.NoContent(http.StatusNoContent)
}

func (s *HTTPServer) reloadAlertControllerRules(ctx context.Context, tenantID string) {
	if s == nil || s.alertController == nil || s.alertRuleRepo == nil {
		return
	}
	logger := LoggerFromContext(ctx, s.logger)
	const maxRules = 500
	rules, _, err := s.alertRuleRepo.ListRules(ctx, tenantID, "", maxRules, 0)
	if err != nil {
		if logger != nil {
			logger.Warn("reload alert rules failed", zap.String("tenantId", tenantID), zap.Error(err))
		}
		return
	}
	configs := make([]config.AlertRuleConfig, 0, len(rules))
	for _, r := range rules {
		expr := buildAlertRuleExpression(r.Expression, r.Operator, r.Threshold)
		if expr == "" {
			continue
		}
		configs = append(configs, config.AlertRuleConfig{
			ID:         r.ID,
			Expression: expr,
			Severity:   strings.ToLower(r.Severity),
			Summary:    fallbackRuleSummary(r),
			Detail:     strings.TrimSpace(r.MatchTemplate),
			TTL:        time.Duration(r.DurationSec) * time.Second,
		})
	}
	s.alertController.SetRules(configs)
}

func buildAlertRuleExpression(metric, operator string, threshold float64) string {
	metric = strings.TrimSpace(metric)
	operator = strings.TrimSpace(operator)
	if metric == "" {
		return ""
	}
	if operator == "" || threshold == 0 {
		return metric
	}
	return fmt.Sprintf("%s %s %g", metric, operator, threshold)
}

func fallbackRuleSummary(rule alerting.Rule) string {
	name := strings.TrimSpace(rule.Name)
	if name != "" {
		return name
	}
	if rule.ID != "" {
		return fmt.Sprintf("Rule %s triggered", rule.ID)
	}
	return "Alert rule triggered"
}

func (s *HTTPServer) handleAlertEvents(c echo.Context) error {
	if s.alertFeed == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert feed unavailable")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	page, size, err := parsePageParams(c)
	if err != nil {
		return err
	}
	status := strings.ToLower(strings.TrimSpace(c.QueryParam("status")))
	severity := strings.ToLower(strings.TrimSpace(c.QueryParam("severity")))
	limit := page * size
	snapshot := s.alertFeed.Snapshot(tenantID, limit)
	filtered := make([]monitoring.AlertFeedEntry, 0, len(snapshot.Alerts))
	for _, entry := range snapshot.Alerts {
		if status != "" && status != strings.ToLower(string(entry.Lifecycle)) && status != mapLifecycleToStatus(entry.Lifecycle) {
			continue
		}
		if severity != "" && severity != strings.ToLower(string(entry.Severity)) {
			continue
		}
		filtered = append(filtered, entry)
	}
	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-start)
	for _, entry := range filtered[start:end] {
		items = append(items, map[string]any{
			"id":        entry.ID,
			"ruleId":    entry.Fingerprint,
			"severity":  strings.ToLower(string(entry.Severity)),
			"message":   entry.Summary,
			"status":    mapLifecycleToStatus(entry.Lifecycle),
			"assignee":  entry.Assignee,
			"createdAt": entry.CreatedAt,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"items":       items,
		"total":       total,
	})
}

func (s *HTTPServer) alertTenantKey(c echo.Context) string {
	tenantID, _ := s.monitoringTenantID(c)
	key := strings.ToLower(strings.TrimSpace(tenantID))
	if key == "" {
		key = strings.ToLower(systemTenantID)
	}
	return key
}

func (s *HTTPServer) currentDHCPServerHealthSummary(ctx context.Context) (healthy int, total int) {
	if s == nil {
		return 0, 0
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var snapshot failover.StatusSnapshot
	var nodes []failover.NodeStatus

	svc := s.ensureHAService()
	if svc != nil {
		snapshot = svc.Snapshot()
		nodes = svc.Nodes(ctx)
	} else if s.options.Coordinator != nil {
		snapshot = s.options.Coordinator.Snapshot()
	}

	var health *monitoring.SystemHealthSnapshot
	if s.monitor != nil {
		current := s.monitor.Health(ctx)
		health = &current
	}

	overviewNodes := buildClusterNodes(nodes, snapshot, health)
	overviewNodes = s.mergeClusterNodes(overviewNodes)
	total = len(overviewNodes)
	for _, node := range overviewNodes {
		if strings.EqualFold(strings.TrimSpace(node.Health), "healthy") {
			healthy++
		}
	}
	return healthy, total
}

type alertWindowStats struct {
	Count          int
	Resolved       int
	InProgress     int
	ResolutionRate int
	MTTRMinutes    float64
	MTTRDisplay    string
}

func (s *HTTPServer) alertFeedEntries(tenantKey string, limit int) []monitoring.AlertFeedEntry {
	if s == nil || s.alertFeed == nil {
		return nil
	}
	snapshot := s.alertFeed.Snapshot(tenantKey, limit)
	if len(snapshot.Alerts) == 0 {
		return nil
	}
	entries := make([]monitoring.AlertFeedEntry, 0, len(snapshot.Alerts))
	for _, entry := range snapshot.Alerts {
		entries = append(entries, entry)
	}
	return entries
}

func filterAlertEntriesByRange(entries []monitoring.AlertFeedEntry, from, to time.Time) []monitoring.AlertFeedEntry {
	if len(entries) == 0 {
		return nil
	}
	filtered := make([]monitoring.AlertFeedEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.CreatedAt.IsZero() {
			continue
		}
		if entry.CreatedAt.Before(from) || entry.CreatedAt.After(to) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func computeAlertWindowStats(entries []monitoring.AlertFeedEntry) alertWindowStats {
	stats := alertWindowStats{Count: len(entries), MTTRDisplay: "-"}
	if len(entries) == 0 {
		return stats
	}
	var mttrTotalMinutes float64
	var mttrSamples int
	for _, entry := range entries {
		if mapLifecycleToStatus(entry.Lifecycle) == "resolved" {
			stats.Resolved++
			if !entry.UpdatedAt.IsZero() && !entry.CreatedAt.IsZero() && entry.UpdatedAt.After(entry.CreatedAt) {
				mttrTotalMinutes += entry.UpdatedAt.Sub(entry.CreatedAt).Minutes()
				mttrSamples++
			}
		}
	}
	stats.InProgress = maxInt(0, stats.Count-stats.Resolved)
	if stats.Count > 0 {
		stats.ResolutionRate = int(math.Round(float64(stats.Resolved) / float64(stats.Count) * 100))
	}
	if mttrSamples > 0 {
		stats.MTTRMinutes = mttrTotalMinutes / float64(mttrSamples)
		if stats.MTTRMinutes >= 60 {
			stats.MTTRDisplay = fmt.Sprintf("%.1fh", stats.MTTRMinutes/60)
		} else {
			stats.MTTRDisplay = fmt.Sprintf("%.1fm", stats.MTTRMinutes)
		}
	}
	return stats
}

func parseAnalyticsRangeWindow(raw string, fallback time.Duration) time.Duration {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return fallback
	}
	switch text {
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	}
	if parsed, err := time.ParseDuration(text); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func (s *HTTPServer) handleAlertActiveSummary(c echo.Context) error {
	healthy, total := s.currentDHCPServerHealthSummary(c.Request().Context())
	summary := alertActiveSummaryDTO{Servers: struct {
		Healthy int "json:\"healthy\""
		Total   int "json:\"total\""
	}{Healthy: healthy, Total: total}}
	tenantID := s.alertTenantKey(c)
	if s.alertFeed != nil {
		snapshot := s.alertFeed.Snapshot(tenantID, 200)
		summary.Active = len(snapshot.Alerts)
		for _, entry := range snapshot.Alerts {
			switch strings.ToLower(string(entry.Severity)) {
			case "emergency":
				summary.Emergency++
			case "critical":
				summary.Critical++
			case "warning":
				summary.Warning++
			default:
				summary.Info++
			}
		}
	}
	return c.JSON(http.StatusOK, summary)
}

func (s *HTTPServer) handleAlertActiveTrend(c echo.Context) error {
	points := make([]alertTrendPointDTO, 0, 12)
	now := time.Now().UTC().Truncate(time.Hour)
	aggregated := make(map[int]*alertTrendPointDTO)
	if s.alertFeed != nil {
		snapshot := s.alertFeed.Snapshot(s.alertTenantKey(c), 500)
		for _, entry := range snapshot.Alerts {
			if entry.CreatedAt.IsZero() {
				continue
			}
			if now.Sub(entry.CreatedAt) > 24*time.Hour {
				continue
			}
			offset := int(now.Sub(entry.CreatedAt).Hours())
			slot := 24 - offset
			if slot < 0 {
				slot = 0
			}
			bucket := aggregated[slot]
			if bucket == nil {
				aggregated[slot] = &alertTrendPointDTO{TS: entry.CreatedAt.UTC().Format(time.RFC3339)}
				bucket = aggregated[slot]
			}
			switch strings.ToLower(string(entry.Severity)) {
			case "emergency":
				bucket.Emergency++
			case "critical":
				bucket.Critical++
			case "warning":
				bucket.Warning++
			default:
				bucket.Info++
			}
		}
	}
	if len(aggregated) == 0 {
		for i := 0; i < 12; i++ {
			points = append(points, alertTrendPointDTO{
				TS:        now.Add(time.Duration(-2*i) * time.Hour).Format(time.RFC3339),
				Emergency: maxInt(0, 5-i),
				Critical:  maxInt(0, 4-i),
				Warning:   maxInt(0, 3-i),
				Info:      maxInt(0, 2+i/2),
			})
		}
	} else {
		for _, v := range aggregated {
			points = append(points, *v)
		}
		sort.Slice(points, func(i, j int) bool {
			return strings.Compare(points[i].TS, points[j].TS) < 0
		})
	}
	return c.JSON(http.StatusOK, points)
}

func (s *HTTPServer) handleAlertHistorySummary(c echo.Context) error {
	from, to := parseTimeRange(c.QueryParam("from"), c.QueryParam("to"), 7*24*time.Hour)
	entries := s.alertFeedEntries(s.alertTenantKey(c), 2000)
	stats := computeAlertWindowStats(filterAlertEntriesByRange(entries, from, to))
	summary := AlertHistorySummary{Total: stats.Count, ResolutionRate: stats.ResolutionRate, InProgress: stats.InProgress}
	return c.JSON(http.StatusOK, summary)
}

func (s *HTTPServer) handleAlertHistoryCompare(c echo.Context) error {
	from, to := parseTimeRange(c.QueryParam("from"), c.QueryParam("to"), 7*24*time.Hour)
	window := to.Sub(from)
	if window <= 0 {
		window = 7 * 24 * time.Hour
		from = to.Add(-window)
	}
	prevFrom := from.Add(-window)
	prevTo := from

	entries := s.alertFeedEntries(s.alertTenantKey(c), 3000)
	currentStats := computeAlertWindowStats(filterAlertEntriesByRange(entries, from, to))
	previousStats := computeAlertWindowStats(filterAlertEntriesByRange(entries, prevFrom, prevTo))

	current := struct {
		Count          int `json:"count"`
		ResolutionRate int `json:"resolutionRate"`
		MTTR           any `json:"mttr,omitempty"`
	}{Count: currentStats.Count, ResolutionRate: currentStats.ResolutionRate, MTTR: currentStats.MTTRDisplay}
	previous := struct {
		Count          int `json:"count"`
		ResolutionRate int `json:"resolutionRate"`
		MTTR           any `json:"mttr,omitempty"`
	}{Count: previousStats.Count, ResolutionRate: previousStats.ResolutionRate, MTTR: previousStats.MTTRDisplay}
	return c.JSON(http.StatusOK, map[string]any{
		"current":  current,
		"previous": previous,
	})
}

func (s *HTTPServer) handleAlertThresholdsGet(c echo.Context) error {
	cfg, err := s.loadAlertThresholds(c.Request().Context(), s.alertTenantKey(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, cfg)
}

func (s *HTTPServer) handleAlertThresholdsSave(c echo.Context) error {
	key := s.alertTenantKey(c)
	var payload alertThresholdConfig
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := s.saveAlertThresholds(c.Request().Context(), key, payload); err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.config.thresholds.update", s.auditPayloadFromRequest(c, map[string]any{
		"tenantId": key,
		"resource": payload.Resource,
		"server":   payload.Server,
		"network":  payload.Network,
	}), withResource("alert_config"))
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

func (s *HTTPServer) handleAlertNotifyGet(c echo.Context) error {
	cfg, err := s.loadAlertNotify(c.Request().Context(), s.alertTenantKey(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, cfg)
}

func (s *HTTPServer) handleAlertNotifySave(c echo.Context) error {
	key := s.alertTenantKey(c)
	var payload alertNotifyConfig
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := s.saveAlertNotify(c.Request().Context(), key, payload); err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.config.notify.update", s.auditPayloadFromRequest(c, map[string]any{
		"tenantId": key,
		"channels": payload.Channels,
		"policies": payload.Policies,
	}), withResource("alert_config"))
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

func (s *HTTPServer) handleAlertTemplates(c echo.Context) error {
	items, err := s.listAlertTemplates(c.Request().Context(), s.alertTenantKey(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

func (s *HTTPServer) handleAlertTemplateCreate(c echo.Context) error {
	key := s.alertTenantKey(c)
	var payload alertTemplateDTO
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	created, err := s.createAlertTemplate(c.Request().Context(), key, payload)
	if err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.template.create", s.auditPayloadFromRequest(c, map[string]any{
		"templateId": created.ID,
		"name":       created.Name,
		"lang":       created.Lang,
		"channel":    created.Channel,
	}), withResource("alert_template"))
	return c.JSON(http.StatusOK, created)
}

func (s *HTTPServer) handleAlertTemplateUpdate(c echo.Context) error {
	key := s.alertTenantKey(c)
	id := c.Param("templateId")
	var payload alertTemplateDTO
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	updated, err := s.updateAlertTemplate(c.Request().Context(), key, id, payload)
	if err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.template.update", s.auditPayloadFromRequest(c, map[string]any{
		"templateId": updated.ID,
		"name":       updated.Name,
		"lang":       updated.Lang,
		"channel":    updated.Channel,
	}), withResource("alert_template"))
	return c.JSON(http.StatusOK, updated)
}

func (s *HTTPServer) handleAlertTemplateDelete(c echo.Context) error {
	key := s.alertTenantKey(c)
	id := c.Param("templateId")
	if err := s.deleteAlertTemplate(c.Request().Context(), key, id); err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.template.delete", s.auditPayloadFromRequest(c, map[string]any{
		"templateId": strings.TrimSpace(id),
	}), withResource("alert_template"))
	return c.NoContent(http.StatusOK)
}

func (s *HTTPServer) handleAlertReceivers(c echo.Context) error {
	items, err := s.listAlertReceivers(c.Request().Context(), s.alertTenantKey(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

func (s *HTTPServer) handleAlertReceiverCreate(c echo.Context) error {
	key := s.alertTenantKey(c)
	var payload alertReceiverDTO
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	created, err := s.createAlertReceiver(c.Request().Context(), key, payload)
	if err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.receiver.create", s.auditPayloadFromRequest(c, map[string]any{
		"receiverId": created.ID,
		"name":       created.Name,
		"email":      created.Email,
		"levels":     append([]string(nil), created.Levels...),
	}), withResource("alert_receiver"))
	return c.JSON(http.StatusOK, created)
}

func (s *HTTPServer) handleAlertReceiverUpdate(c echo.Context) error {
	key := s.alertTenantKey(c)
	id := c.Param("receiverId")
	var payload alertReceiverDTO
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	updated, err := s.updateAlertReceiver(c.Request().Context(), key, id, payload)
	if err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.receiver.update", s.auditPayloadFromRequest(c, map[string]any{
		"receiverId": updated.ID,
		"name":       updated.Name,
		"email":      updated.Email,
		"levels":     append([]string(nil), updated.Levels...),
	}), withResource("alert_receiver"))
	return c.JSON(http.StatusOK, updated)
}

func (s *HTTPServer) handleAlertReceiverDelete(c echo.Context) error {
	key := s.alertTenantKey(c)
	id := c.Param("receiverId")
	if err := s.deleteAlertReceiver(c.Request().Context(), key, id); err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.receiver.delete", s.auditPayloadFromRequest(c, map[string]any{
		"receiverId": strings.TrimSpace(id),
	}), withResource("alert_receiver"))
	return c.NoContent(http.StatusOK)
}

func (s *HTTPServer) handleAlertReceiversImport(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if file == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file required")
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.receiver.import", s.auditPayloadFromRequest(c, map[string]any{
		"fileName": file.Filename,
		"size":     file.Size,
	}), withResource("alert_receiver"))
	// For now just acknowledge upload; content parsing can be added later.
	return c.JSON(http.StatusOK, map[string]any{"status": "imported", "size": file.Size})
}

func (s *HTTPServer) handleAlertReceiversExport(c echo.Context) error {
	key := s.alertTenantKey(c)
	s.alertConfigMu.RLock()
	items := s.alertReceiverStore[key]
	s.alertConfigMu.RUnlock()
	buf := &bytes.Buffer{}
	_, _ = buf.WriteString("name,email,phone,levels\n")
	for _, r := range items {
		_, _ = buf.WriteString(fmt.Sprintf("%s,%s,%s,%s\n", r.Name, r.Email, r.Phone, strings.Join(r.Levels, ";")))
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.receiver.export", s.auditPayloadFromRequest(c, map[string]any{
		"count": len(items),
	}), withResource("alert_receiver"))
	c.Response().Header().Set(echo.HeaderContentType, "text/csv")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=alert_receivers.csv")
	return c.Blob(http.StatusOK, "text/csv", buf.Bytes())
}

func (s *HTTPServer) handleAlertAnalyticsSummary(c echo.Context) error {
	window := parseAnalyticsRangeWindow(c.QueryParam("range"), 30*24*time.Hour)
	to := time.Now().UTC()
	from := to.Add(-window)
	entries := s.alertFeedEntries(s.alertTenantKey(c), 3000)
	ranged := filterAlertEntriesByRange(entries, from, to)
	stats := computeAlertWindowStats(ranged)
	typeCount := make(map[string]int)
	for _, entry := range ranged {
		kind := strings.TrimSpace(entry.Category)
		if kind == "" {
			kind = strings.TrimSpace(entry.Summary)
		}
		if kind == "" {
			kind = "未知"
		}
		typeCount[kind]++
	}
	topType := "-"
	topCount := 0
	for kind, cnt := range typeCount {
		if cnt > topCount {
			topType = kind
			topCount = cnt
		}
	}
	summary := alertAnalyticsSummaryDTO{Total: stats.Count, MTTR: stats.MTTRDisplay, TopType: topType, ResolutionRate: stats.ResolutionRate}
	return c.JSON(http.StatusOK, summary)
}

func (s *HTTPServer) handleAlertTypeDistribution(c echo.Context) error {
	window := parseAnalyticsRangeWindow(c.QueryParam("range"), 30*24*time.Hour)
	to := time.Now().UTC()
	from := to.Add(-window)
	entries := filterAlertEntriesByRange(s.alertFeedEntries(s.alertTenantKey(c), 3000), from, to)
	typeCount := make(map[string]int)
	for _, entry := range entries {
		kind := strings.TrimSpace(entry.Category)
		if kind == "" {
			kind = strings.TrimSpace(entry.Summary)
		}
		if kind == "" {
			kind = "未知"
		}
		typeCount[kind]++
	}
	data := make([]alertTypeDistributionDTO, 0, len(typeCount))
	for kind, cnt := range typeCount {
		data = append(data, alertTypeDistributionDTO{Type: kind, Count: cnt})
	}
	sort.Slice(data, func(i, j int) bool {
		if data[i].Count == data[j].Count {
			return data[i].Type < data[j].Type
		}
		return data[i].Count > data[j].Count
	})
	return c.JSON(http.StatusOK, data)
}

func (s *HTTPServer) handleAlertTimeDistribution(c echo.Context) error {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	entries := filterAlertEntriesByRange(s.alertFeedEntries(s.alertTenantKey(c), 2000), from, now)
	bucketCounts := make([]int, 12)
	for _, entry := range entries {
		hour := entry.CreatedAt.UTC().Hour()
		idx := hour / 2
		if idx < 0 {
			idx = 0
		}
		if idx >= len(bucketCounts) {
			idx = len(bucketCounts) - 1
		}
		bucketCounts[idx]++
	}
	data := make([]alertTimeDistributionDTO, 0, 12)
	for i := 0; i < 12; i++ {
		data = append(data, alertTimeDistributionDTO{Hour: i * 2, Count: bucketCounts[i]})
	}
	return c.JSON(http.StatusOK, data)
}

func (s *HTTPServer) handleAlertTrendCompare(c echo.Context) error {
	window := parseAnalyticsRangeWindow(c.QueryParam("range"), 7*24*time.Hour)
	now := time.Now().UTC()
	to := now
	from := to.Add(-window)
	entries := filterAlertEntriesByRange(s.alertFeedEntries(s.alertTenantKey(c), 5000), from.Add(-window), to)

	current := make([]alertTrendCount, 0, 7)
	previous := make([]alertTrendCount, 0, 7)
	anomalies := make([]alertAnomaly, 0)
	for i := 0; i < 7; i++ {
		dayStart := now.Add(time.Duration(-(6 - i)) * 24 * time.Hour).Truncate(24 * time.Hour)
		dayEnd := dayStart.Add(24 * time.Hour)
		prevStart := dayStart.Add(-7 * 24 * time.Hour)
		prevEnd := prevStart.Add(24 * time.Hour)

		currentCount := 0
		previousCount := 0
		for _, entry := range entries {
			created := entry.CreatedAt.UTC()
			if !created.Before(dayStart) && created.Before(dayEnd) {
				currentCount++
			}
			if !created.Before(prevStart) && created.Before(prevEnd) {
				previousCount++
			}
		}
		current = append(current, alertTrendCount{TS: dayStart.Format(time.RFC3339), Count: currentCount})
		previous = append(previous, alertTrendCount{TS: prevStart.Format(time.RFC3339), Count: previousCount})
		if previousCount > 0 && currentCount >= int(math.Ceil(float64(previousCount)*1.5)) {
			anomalies = append(anomalies, alertAnomaly{TS: dayStart.Format(time.RFC3339), Desc: "告警量异常升高"})
		}
	}
	resp := alertTrendCompareDTO{
		Current:   current,
		Previous:  previous,
		Anomalies: anomalies,
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) handleReportTasks(c echo.Context) error {
	scope := s.leaseScopeRef(c)
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	if tenantID == "" {
		tenantID = systemTenantID
	}
	page, size, err := parsePageParams(c)
	if err != nil {
		return err
	}
	status := strings.TrimSpace(c.QueryParam("status"))
	typeFilter := strings.TrimSpace(c.QueryParam("type"))
	tenantKey := strings.ToLower(tenantID)
	s.reportMu.Lock()
	items := append([]reportTaskDTO(nil), s.reportTasks[tenantKey]...)
	s.reportMu.Unlock()
	filtered := make([]reportTaskDTO, 0, len(items))
	for _, it := range items {
		if status != "" && !strings.EqualFold(status, it.Status) {
			continue
		}
		if typeFilter != "" && !strings.EqualFold(typeFilter, it.Type) {
			continue
		}
		filtered = append(filtered, it)
	}
	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items": filtered[start:end],
		"total": total,
	})
}

func (s *HTTPServer) handleReportTaskCreate(c echo.Context) error {
	scope := s.leaseScopeRef(c)
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	if tenantID == "" {
		tenantID = systemTenantID
	}
	var payload reportTaskDTO
	if bindErr := c.Bind(&payload); bindErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	payload.Type = strings.TrimSpace(payload.Type)
	payload.Format = strings.TrimSpace(payload.Format)
	if payload.Type == "" || payload.Format == "" || payload.Range.From == "" || payload.Range.To == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "type, format, range.from and range.to are required")
	}
	payload.ID = uuid.NewString()
	payload.Status = "pending"
	payload.CreatedAt = time.Now().UTC()
	payload.TenantID = tenantID
	tenantKey := strings.ToLower(tenantID)
	s.reportMu.Lock()
	s.reportTasks[tenantKey] = append(s.reportTasks[tenantKey], payload)
	s.reportMu.Unlock()
	return c.JSON(http.StatusOK, payload)
}

func (s *HTTPServer) handleMonitoringAnalytics(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	if limit == 0 {
		limit = 50
	}
	snapshot, err := s.monitor.Analytics(c.Request().Context(), scope, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleAlertSilence(c echo.Context) error {
	type request struct {
		Channel    string `json:"channel"`
		TTLSeconds int64  `json:"ttlSeconds"`
		Assignee   string `json:"assignee"`
		Note       string `json:"note"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	entry, err := s.performAlertSilence(c, payload.Channel, payload.Assignee, payload.TTLSeconds)
	if err != nil {
		return err
	}
	s.dispatchAlertNotification(c.Request().Context(), entry, payload.Channel, "alert.silenced", strings.TrimSpace(payload.Note))
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.action.silence", s.auditPayloadFromRequest(c, alertEntryAuditPayload(entry, map[string]any{
		"ttlSeconds": payload.TTLSeconds,
		"note":       strings.TrimSpace(payload.Note),
	})), withResource("alert_event"))
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleAlertEscalate(c echo.Context) error {
	type request struct {
		Assignee string `json:"assignee"`
		Channel  string `json:"channel"`
		Note     string `json:"note"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	entry, err := s.updateAlertEntry(c, 0, false, func(tenantID, alertID string) (monitoring.AlertFeedEntry, error) {
		return s.alertFeed.Escalate(tenantID, alertID, payload.Assignee, payload.Channel)
	})
	if err != nil {
		return err
	}
	if s.alertManager != nil {
		event := alerting.Event{
			ID:         entry.ID,
			Severity:   entry.Severity,
			TenantID:   entry.TenantID,
			Category:   entry.Category,
			Summary:    entry.Summary,
			Details:    entry.Details,
			Resources:  append([]string(nil), entry.Tags...),
			OccurredAt: time.Now().UTC(),
			Labels: map[string]string{
				"lifecycle":       string(entry.Lifecycle),
				"escalationLevel": strconv.Itoa(entry.EscalationLevel),
			},
		}
		if entry.Assignee != "" {
			event.Labels["assignee"] = entry.Assignee
		}
		if payload.Channel != "" {
			event.Channels = []string{strings.ToLower(strings.TrimSpace(payload.Channel))}
		}
		s.alertManager.Notify(c.Request().Context(), event)
	}
	s.dispatchAlertNotification(c.Request().Context(), entry, payload.Channel, "alert.escalated", strings.TrimSpace(payload.Note))
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.action.escalate", s.auditPayloadFromRequest(c, alertEntryAuditPayload(entry, map[string]any{
		"note": strings.TrimSpace(payload.Note),
	})), withResource("alert_event"))
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleAlertIncidents(c echo.Context) error {
	if s.alertFeed == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert feed unavailable")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	snapshot := s.alertFeed.Snapshot(tenantID, limit)
	if len(snapshot.Alerts) == 0 {
		snapshot = s.sampleAlertSnapshot(tenantID, limit)
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleAlertRoutesSnapshot(c echo.Context) error {
	ctx := c.Request().Context()
	if s.alertRouteRepo != nil {
		routes, err := s.alertRouteRepo.ListRoutes(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if s.alertRoutes == nil {
			s.alertRoutes = alerting.NewRoutingStore(routes, "bootstrap")
		}
		actor := strings.TrimSpace(s.actorFromContext(c))
		if actor == "" {
			actor = "system"
		}
		return c.JSON(http.StatusOK, alerting.RoutingSnapshot{
			UpdatedAt: mostRecentRouteUpdate(routes),
			UpdatedBy: actor,
			Rules:     routes,
		})
	}
	if s.alertRoutes == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert routing unavailable")
	}
	snapshot := s.alertRoutes.Snapshot()
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleAlertRoutesUpdate(c echo.Context) error {
	if s.alertRoutes == nil && s.alertRouteRepo == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert routing unavailable")
	}
	type request struct {
		Rules []alerting.RoutingRule `json:"rules"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	actor := s.actorFromContext(c)
	if s.alertRouteRepo != nil {
		if err := s.alertRouteRepo.ReplaceRoutes(c.Request().Context(), payload.Rules, actor); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if s.alertRoutes == nil {
		s.alertRoutes = alerting.NewRoutingStore(payload.Rules, actor)
	} else {
		s.alertRoutes.Replace(payload.Rules, actor)
	}
	if s.alertManager != nil {
		s.alertManager.ConfigureRoutes(alerting.BuildRoutes(routingRulesToConfigs(payload.Rules)))
	}
	snapshot := s.alertRoutes.Snapshot()
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.route.update", s.auditPayloadFromRequest(c, map[string]any{
		"ruleCount": len(payload.Rules),
	}), withResource("alert_route"))
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleDutyRosterSnapshot(c echo.Context) error {
	if s.dutySchedule == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "duty schedule unavailable")
	}
	roster := s.dutySchedule.Snapshot()
	return c.JSON(http.StatusOK, roster)
}

func (s *HTTPServer) handleDutyRosterUpdate(c echo.Context) error {
	if s.dutySchedule == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "duty schedule unavailable")
	}
	type request struct {
		Shifts []alerting.DutyShift `json:"shifts"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	actor := s.actorFromContext(c)
	roster := s.dutySchedule.Replace(payload.Shifts, actor)
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.roster.update", s.auditPayloadFromRequest(c, map[string]any{
		"shiftCount": len(payload.Shifts),
	}), withResource("alert_roster"))
	return c.JSON(http.StatusOK, roster)
}

func (s *HTTPServer) handleListNotificationChannels(c echo.Context) error {
	s.notificationMu.Lock()
	items := append([]notificationChannelDTO(nil), s.notificationChannels...)
	s.notificationMu.Unlock()
	return c.JSON(http.StatusOK, map[string]any{"data": items})
}

func (s *HTTPServer) handleUpsertNotificationChannel(c echo.Context) error {
	s.ensureNotificationDispatcher()
	if s.notificationDispatcher == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "notifications disabled")
	}
	var payload notificationChannelDTO
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	logger := LoggerFromContext(c.Request().Context(), s.logger)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = strings.ToLower(strings.TrimSpace(payload.Type))
	payload.Target = strings.TrimSpace(payload.Target)
	payload.Method = strings.ToUpper(strings.TrimSpace(payload.Method))
	if payload.Name == "" || payload.Type == "" || payload.Target == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "名称、类型和目标地址必填")
	}
	if payload.Method == "" {
		payload.Method = http.MethodPost
	}
	if err := s.registerNotificationChannel(payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	s.notificationMu.Lock()
	replaced := false
	for idx, ch := range s.notificationChannels {
		if strings.EqualFold(ch.Name, payload.Name) {
			s.notificationChannels[idx] = payload
			replaced = true
			break
		}
	}
	if !replaced {
		s.notificationChannels = append(s.notificationChannels, payload)
	}
	s.notificationMu.Unlock()
	if err := s.persistNotificationChannels(); err != nil && logger != nil {
		logger.Warn("persist notification channels", zap.Error(err))
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "notification.channel.upsert", s.auditPayloadFromRequest(c, map[string]any{
		"name":   payload.Name,
		"type":   payload.Type,
		"target": payload.Target,
		"method": payload.Method,
	}), withResource("notification_channel"))
	return c.JSON(http.StatusOK, payload)
}

type smtpConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"useTLS"`
}

type smsConfig struct {
	Provider        string `json:"provider"`
	AccessKeyID     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	SignName        string `json:"signName"`
	TemplateCode    string `json:"templateCode"`
	TestPhone       string `json:"testPhone"`
}

type webhookConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Token        string            `json:"token"`
	HeaderKey    string            `json:"headerKey"`
	Payload      string            `json:"payload"`
	Headers      map[string]string `json:"headers,omitempty"`
	Channels     []string          `json:"channels,omitempty"`
	DingTalkURL  string            `json:"dingTalkUrl,omitempty"`
	FeishuURL    string            `json:"feishuUrl,omitempty"`
	WecomURL     string            `json:"wecomUrl,omitempty"`
	SlackURL     string            `json:"slackUrl,omitempty"`
	PhoneNumbers string            `json:"phoneNumbers,omitempty"`
}

const smtpConfigPath = "data/smtp_config.json"
const smsConfigPath = "data/sms_config.json"
const webhookConfigPath = "data/webhook_config.json"
const notificationChannelsPath = "data/notification_channels.json"

func (s *HTTPServer) handleGetSMTPConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.smtpMu.RLock()
		cfg := s.smtpConfig
		s.smtpMu.RUnlock()
		// Avoid echoing password back to clients.
		cfg.Password = ""
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "", "data": cfg})
	}
}

func (s *HTTPServer) persistSMTPConfig() error {
	if s == nil {
		return nil
	}
	s.smtpMu.RLock()
	cfg := s.smtpConfig
	s.smtpMu.RUnlock()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(smtpConfigPath, data, 0o600); err != nil {
		return err
	}
	return nil
}

func (s *HTTPServer) persistSMSConfig() error {
	if s == nil {
		return nil
	}
	s.smsMu.RLock()
	cfg := s.smsConfig
	s.smsMu.RUnlock()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(smsConfigPath, data, 0o600)
}

func (s *HTTPServer) persistWebhookConfig() error {
	if s == nil {
		return nil
	}
	s.webhookMu.RLock()
	cfg := s.webhookConfig
	s.webhookMu.RUnlock()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(webhookConfigPath, data, 0o600)
}

func (s *HTTPServer) handleGetSMSConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.smsMu.RLock()
		cfg := s.smsConfig
		s.smsMu.RUnlock()
		cfg.AccessKeySecret = ""
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "", "data": cfg})
	}
}

func (s *HTTPServer) handleSaveSMSConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload smsConfig
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		payload.Provider = strings.TrimSpace(payload.Provider)
		payload.AccessKeyID = strings.TrimSpace(payload.AccessKeyID)
		payload.AccessKeySecret = strings.TrimSpace(payload.AccessKeySecret)
		payload.SignName = strings.TrimSpace(payload.SignName)
		payload.TemplateCode = strings.TrimSpace(payload.TemplateCode)
		payload.TestPhone = strings.TrimSpace(payload.TestPhone)
		if payload.Provider == "" || payload.AccessKeyID == "" || payload.AccessKeySecret == "" || payload.SignName == "" || payload.TemplateCode == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "provider、accessKey、签名与模板必填")
		}
		s.smsMu.Lock()
		s.smsConfig = payload
		s.smsMu.Unlock()
		if err := s.persistSMSConfig(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "ok"})
	}
}

func (s *HTTPServer) handleTestSMSConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.smsMu.RLock()
		cfg := s.smsConfig
		s.smsMu.RUnlock()
		if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.SignName) == "" || strings.TrimSpace(cfg.TemplateCode) == "" || strings.TrimSpace(cfg.TestPhone) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "请先保存完整短信配置并填写测试手机号")
		}
		// Placeholder for SMS provider integration; respond accepted for now.
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "测试请求已记录（未实际发送）"})
	}
}

func (s *HTTPServer) handleGetWebhookConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.webhookMu.RLock()
		cfg := s.webhookConfig
		s.webhookMu.RUnlock()
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "", "data": cfg})
	}
}

func (s *HTTPServer) handleSaveWebhookConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload webhookConfig
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		payload.URL = strings.TrimSpace(payload.URL)
		payload.DingTalkURL = strings.TrimSpace(payload.DingTalkURL)
		payload.FeishuURL = strings.TrimSpace(payload.FeishuURL)
		payload.WecomURL = strings.TrimSpace(payload.WecomURL)
		payload.SlackURL = strings.TrimSpace(payload.SlackURL)
		payload.PhoneNumbers = strings.TrimSpace(payload.PhoneNumbers)
		payload.Method = strings.ToUpper(strings.TrimSpace(payload.Method))
		payload.Token = strings.TrimSpace(payload.Token)
		payload.HeaderKey = strings.TrimSpace(payload.HeaderKey)
		allowedChannels := map[string]struct{}{"dingtalk": {}, "feishu": {}, "wecom": {}, "slack": {}, "phone": {}}
		cleanChannels := make([]string, 0, len(payload.Channels))
		seen := map[string]struct{}{}
		for _, channel := range payload.Channels {
			key := strings.ToLower(strings.TrimSpace(channel))
			if key == "" {
				continue
			}
			if _, ok := allowedChannels[key]; !ok {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			cleanChannels = append(cleanChannels, key)
		}
		payload.Channels = cleanChannels
		hasPrimary := payload.URL != ""
		hasChannelTarget := payload.DingTalkURL != "" || payload.FeishuURL != "" || payload.WecomURL != "" || payload.SlackURL != "" || payload.PhoneNumbers != ""
		if !hasPrimary && !hasChannelTarget {
			return echo.NewHTTPError(http.StatusBadRequest, "Webhook URL 或渠道地址至少填写一项")
		}
		if payload.Method == "" {
			payload.Method = http.MethodPost
		}
		s.webhookMu.Lock()
		s.webhookConfig = payload
		s.webhookMu.Unlock()
		if err := s.persistWebhookConfig(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "notification.webhook.update", s.auditPayloadFromRequest(c, map[string]any{
			"url":          payload.URL,
			"method":       payload.Method,
			"headerKey":    payload.HeaderKey,
			"channels":     append([]string(nil), payload.Channels...),
			"dingTalkUrl":  payload.DingTalkURL,
			"feishuUrl":    payload.FeishuURL,
			"wecomUrl":     payload.WecomURL,
			"slackUrl":     payload.SlackURL,
			"phoneNumbers": payload.PhoneNumbers,
		}), withResource("notification_webhook"))
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "ok"})
	}
}

func (s *HTTPServer) handleTestWebhookConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.webhookMu.RLock()
		cfg := s.webhookConfig
		s.webhookMu.RUnlock()
		targetURL := strings.TrimSpace(cfg.URL)
		if targetURL == "" {
			targetURL = strings.TrimSpace(cfg.DingTalkURL)
		}
		if targetURL == "" {
			targetURL = strings.TrimSpace(cfg.FeishuURL)
		}
		if targetURL == "" {
			targetURL = strings.TrimSpace(cfg.WecomURL)
		}
		if targetURL == "" {
			targetURL = strings.TrimSpace(cfg.SlackURL)
		}
		if targetURL == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "请先保存 Webhook 配置")
		}
		method := cfg.Method
		if method == "" {
			method = http.MethodPost
		}
		payload := strings.TrimSpace(cfg.Payload)
		req, err := http.NewRequest(method, targetURL, strings.NewReader(payload))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		if cfg.HeaderKey != "" && cfg.Token != "" {
			req.Header.Set(cfg.HeaderKey, cfg.Token)
		}
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Webhook 请求失败: %v", err))
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Webhook 返回状态码 %d", resp.StatusCode))
		}
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "Webhook 测试成功"})
	}
}

func (s *HTTPServer) persistNotificationChannels() error {
	if s == nil {
		return nil
	}
	s.notificationMu.Lock()
	channels := append([]notificationChannelDTO(nil), s.notificationChannels...)
	s.notificationMu.Unlock()
	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(notificationChannelsPath, data, 0o600)
}

func (s *HTTPServer) loadNotificationChannels() {
	blob, err := os.ReadFile(notificationChannelsPath)
	if err != nil {
		return
	}
	logger := LoggerFromContext(context.Background(), s.logger)
	s.ensureNotificationDispatcher()
	var channels []notificationChannelDTO
	if err := json.Unmarshal(blob, &channels); err != nil {
		if logger != nil {
			logger.Warn("notification channels load failed", zap.Error(err))
		}
		return
	}
	for _, ch := range channels {
		if err := s.registerNotificationChannel(ch); err != nil && logger != nil {
			logger.Warn("register notification channel", zap.String("channel", ch.Name), zap.Error(err))
		}
	}
	s.notificationMu.Lock()
	s.notificationChannels = channels
	s.notificationMu.Unlock()
}

func (s *HTTPServer) registerNotificationChannel(ch notificationChannelDTO) error {
	if s == nil || s.notificationDispatcher == nil {
		return fmt.Errorf("notifications disabled")
	}
	typeKey := strings.ToLower(strings.TrimSpace(ch.Type))
	switch typeKey {
	case "", "webhook":
		sender := notifications.NewWebhookSender(ch.Target, notifications.WebhookOptions{
			Secret:  ch.Secret,
			Method:  ch.Method,
			Headers: ch.Headers,
			Timeout: 5 * time.Second,
		}, s.logger)
		s.notificationDispatcher.RegisterChannel(ch.Name, sender)
		return nil
	case "email":
		s.smtpMu.RLock()
		smtpCfg := s.smtpConfig
		s.smtpMu.RUnlock()
		if strings.TrimSpace(smtpCfg.Host) == "" || strings.TrimSpace(smtpCfg.From) == "" {
			return fmt.Errorf("smtp config required for email channel")
		}
		sender := notifications.NewEmailSender(notifications.EmailOptions{
			Host:     smtpCfg.Host,
			Port:     smtpCfg.Port,
			Username: smtpCfg.Username,
			Password: smtpCfg.Password,
			From:     smtpCfg.From,
			UseTLS:   smtpCfg.UseTLS,
			To:       ch.Target,
		})
		s.notificationDispatcher.RegisterChannel(ch.Name, sender)
		return nil
	default:
		return fmt.Errorf("channel type %s not supported", ch.Type)
	}
}

func (s *HTTPServer) ensureNotificationDispatcher() {
	if s == nil || s.notificationDispatcher != nil {
		return
	}
	logger := s.logger
	if logger != nil {
		logger = logger.Named("notifications")
	} else {
		logger = zap.NewNop()
	}
	s.notificationDispatcher = notifications.NewDispatcher(5*time.Second, logger)
}

func (s *HTTPServer) loadSMTPConfig() {
	blob, err := os.ReadFile(smtpConfigPath)
	if err != nil {
		return
	}
	logger := LoggerFromContext(context.Background(), s.logger)
	var cfg smtpConfig
	if err := json.Unmarshal(blob, &cfg); err != nil {
		if logger != nil {
			logger.Warn("smtp config load failed", zap.Error(err))
		}
		return
	}
	if cfg.Port <= 0 || cfg.Port > 65535 || strings.TrimSpace(cfg.Host) == "" {
		return
	}
	s.smtpMu.Lock()
	s.smtpConfig = cfg
	s.smtpMu.Unlock()
}

func (s *HTTPServer) loadSMSConfig() {
	blob, err := os.ReadFile(smsConfigPath)
	if err != nil {
		return
	}
	logger := LoggerFromContext(context.Background(), s.logger)
	var cfg smsConfig
	if err := json.Unmarshal(blob, &cfg); err != nil {
		if logger != nil {
			logger.Warn("sms config load failed", zap.Error(err))
		}
		return
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return
	}
	s.smsMu.Lock()
	s.smsConfig = cfg
	s.smsMu.Unlock()
}

func (s *HTTPServer) loadWebhookConfig() {
	blob, err := os.ReadFile(webhookConfigPath)
	if err != nil {
		return
	}
	logger := LoggerFromContext(context.Background(), s.logger)
	var cfg webhookConfig
	if err := json.Unmarshal(blob, &cfg); err != nil {
		if logger != nil {
			logger.Warn("webhook config load failed", zap.Error(err))
		}
		return
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return
	}
	s.webhookMu.Lock()
	s.webhookConfig = cfg
	s.webhookMu.Unlock()
}

func (s *HTTPServer) handleSaveSMTPConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		logger := LoggerFromContext(c.Request().Context(), s.logger)
		var payload smtpConfig
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		payload.Host = strings.TrimSpace(payload.Host)
		payload.Username = strings.TrimSpace(payload.Username)
		payload.From = strings.TrimSpace(payload.From)
		if payload.Host == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "host is required")
		}
		if payload.Port <= 0 || payload.Port > 65535 {
			return echo.NewHTTPError(http.StatusBadRequest, "port must be between 1 and 65535")
		}
		if payload.From == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "from is required")
		}
		if _, err := mail.ParseAddress(payload.From); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid from address")
		}
		s.smtpMu.Lock()
		if payload.Password == "" {
			payload.Password = s.smtpConfig.Password
		}
		s.smtpConfig = payload
		s.smtpMu.Unlock()
		_ = s.persistSMTPConfig()
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "notification.smtp.update", s.auditPayloadFromRequest(c, map[string]any{
			"host":   payload.Host,
			"port":   payload.Port,
			"from":   payload.From,
			"useTLS": payload.UseTLS,
		}), withResource("notification_smtp"))
		if logger != nil {
			logger.Info("smtp config saved",
				zap.String("host", payload.Host),
				zap.Int("port", payload.Port),
				zap.String("from", payload.From),
				zap.Bool("tls", payload.UseTLS),
				zap.String("actor", s.actorFromContext(c)),
			)
		}
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "saved"})
	}
}

func (s *HTTPServer) handleTestSMTPConfig() echo.HandlerFunc {
	return func(c echo.Context) error {
		logger := LoggerFromContext(c.Request().Context(), s.logger)
		var payload struct {
			smtpConfig
			To string `json:"to"`
		}
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		s.smtpMu.RLock()
		saved := s.smtpConfig
		s.smtpMu.RUnlock()
		if payload.Host == "" {
			payload.Host = saved.Host
		}
		if payload.Port == 0 {
			payload.Port = saved.Port
		}
		if payload.Username == "" {
			payload.Username = saved.Username
		}
		if payload.Password == "" {
			payload.Password = saved.Password
		}
		if payload.From == "" {
			payload.From = saved.From
		}
		if !payload.UseTLS {
			payload.UseTLS = saved.UseTLS
		}
		payload.Host = strings.TrimSpace(payload.Host)
		payload.From = strings.TrimSpace(payload.From)
		payload.To = strings.TrimSpace(payload.To)
		if payload.Host == "" || payload.From == "" || payload.Port == 0 || payload.To == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "host, port, from, to are required")
		}
		if _, err := mail.ParseAddress(payload.From); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid from address")
		}
		if _, err := mail.ParseAddress(payload.To); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid recipient")
		}
		if delta := time.Since(s.lastSMTPTest); delta < 5*time.Second {
			return echo.NewHTTPError(http.StatusTooManyRequests, "test too frequent")
		}
		if err := s.sendSMTPTest(c.Request().Context(), payload.smtpConfig, payload.To); err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
		s.lastSMTPTest = time.Now()
		if logger != nil {
			logger.Info("smtp test sent",
				zap.String("host", payload.Host),
				zap.Int("port", payload.Port),
				zap.String("from", payload.From),
				zap.String("to", payload.To),
				zap.Bool("tls", payload.UseTLS),
			)
		}
		return c.JSON(http.StatusOK, map[string]any{"code": 0, "message": "test sent"})
	}
}

func (s *HTTPServer) sendSMTPTest(ctx context.Context, cfg smtpConfig, to string) error {
	return s.sendSMTPMessage(
		ctx,
		cfg,
		to,
		"Modern DHCP SMTP 连通性测试",
		"这是一封来自 Modern DHCP 告警的 SMTP 连通性测试邮件。如果你收到了这封邮件，说明 SMTP 配置可用。",
	)
}

func (s *HTTPServer) sendSMTPMessage(ctx context.Context, cfg smtpConfig, to, subject, body string) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	var (
		conn net.Conn
		err  error
	)
	// Port 465 commonly expects implicit TLS; otherwise start plain then upgrade if requested.
	if cfg.UseTLS && cfg.Port == 465 {
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	applySMTPDeadline(ctx, conn)
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if cfg.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: cfg.Host}
			if err := client.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}
	message := buildSMTPMessage(cfg.From, to, subject, body)
	return smtpSendMail(client, cfg, to, message)
}

func smtpSendMail(client *smtp.Client, cfg smtpConfig, to, message string) error {
	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(message)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildSMTPMessage(from, to, subject, body string) string {
	headers := map[string]string{
		"From":    from,
		"To":      to,
		"Subject": subject,
	}
	var b strings.Builder
	for k, v := range headers {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\r\n")
	return b.String()
}

func applySMTPDeadline(ctx context.Context, conn net.Conn) {
	if conn == nil {
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		return
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
}

func (s *HTTPServer) handleSendNotificationTest(c echo.Context) error {
	if s.notificationDispatcher == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "notifications disabled")
	}
	type request struct {
		Channel  string         `json:"channel"`
		Summary  string         `json:"summary"`
		Severity string         `json:"severity"`
		Body     map[string]any `json:"body"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	channel := strings.ToLower(strings.TrimSpace(payload.Channel))
	if channel == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "channel is required")
	}
	severity := strings.ToLower(strings.TrimSpace(payload.Severity))
	if severity == "" {
		severity = string(alerting.SeverityInfo)
	}
	summary := strings.TrimSpace(payload.Summary)
	if summary == "" {
		summary = "Test notification"
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	metadata := map[string]string{
		"channel": channel,
		"actor":   s.actorFromContext(c),
	}
	msg := notifications.Message{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Topic:     "monitoring.alerts",
		Summary:   summary,
		Severity:  severity,
		Body:      payload.Body,
		Metadata:  metadata,
		Source:    "modern-dhcp",
		CreatedAt: time.Now().UTC(),
	}
	ctx := c.Request().Context()
	logger := LoggerFromContext(ctx, s.logger)
	if err := s.notificationDispatcher.Dispatch(ctx, msg, channel); err != nil {
		if logger != nil {
			logger.Warn("notification test failed", zap.String("channel", channel), zap.Error(err))
		}
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusAccepted, map[string]any{
		"messageId": msg.ID,
		"status":    "delivered",
	})
}

func (s *HTTPServer) dispatchAlertNotification(ctx context.Context, entry monitoring.AlertFeedEntry, channel, topic, note string) {
	if s.notificationDispatcher == nil {
		return
	}
	logger := LoggerFromContext(ctx, s.logger)
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		return
	}
	metadata := map[string]string{
		"alertId":   entry.ID,
		"tenantId":  entry.TenantID,
		"lifecycle": string(entry.Lifecycle),
	}
	if entry.Assignee != "" {
		metadata["assignee"] = entry.Assignee
	}
	if note = strings.TrimSpace(note); note != "" {
		metadata["note"] = note
	}
	msg := notifications.Message{
		ID:       uuid.NewString(),
		TenantID: entry.TenantID,
		Topic:    topic,
		Summary:  entry.Summary,
		Severity: strings.ToLower(string(entry.Severity)),
		Body: map[string]any{
			"category": entry.Category,
			"details":  entry.Details,
			"channel":  entry.Channel,
			"tags":     append([]string(nil), entry.Tags...),
		},
		Metadata:  metadata,
		Source:    entry.Source,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.notificationDispatcher.Dispatch(ctx, msg, channel); err != nil {
		if logger != nil {
			logger.Warn("alert notification dispatch failed", zap.String("channel", channel), zap.String("alertId", entry.ID), zap.Error(err))
		}
	}
}

func routingRulesToConfigs(rules []alerting.RoutingRule) []alerting.RouteConfig {
	if len(rules) == 0 {
		return []alerting.RouteConfig{}
	}
	configs := make([]alerting.RouteConfig, 0, len(rules))
	for _, rule := range rules {
		configs = append(configs, alerting.RouteConfig{
			Name:       rule.Name,
			Severities: append([]string(nil), rule.Severities...),
			Channels:   append([]string(nil), rule.Channels...),
		})
	}
	return configs
}

func (s *HTTPServer) updateAlertEntry(c echo.Context, ttlSeconds int64, suppress bool, mutate func(tenantID, alertID string) (monitoring.AlertFeedEntry, error)) (monitoring.AlertFeedEntry, error) {
	if s.alertFeed == nil {
		return monitoring.AlertFeedEntry{}, echo.NewHTTPError(http.StatusServiceUnavailable, "alert feed unavailable")
	}
	alertID := strings.TrimSpace(c.Param("alertId"))
	if alertID == "" {
		return monitoring.AlertFeedEntry{}, echo.NewHTTPError(http.StatusBadRequest, "alertId is required")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return monitoring.AlertFeedEntry{}, err
	}
	entry, updateErr := mutate(tenantID, alertID)
	if updateErr != nil {
		switch {
		case errors.Is(updateErr, monitoring.ErrAlertNotFound):
			return monitoring.AlertFeedEntry{}, echo.NewHTTPError(http.StatusNotFound, updateErr.Error())
		default:
			return monitoring.AlertFeedEntry{}, echo.NewHTTPError(http.StatusInternalServerError, updateErr.Error())
		}
	}
	s.applyAlertMute(entry.Fingerprint, ttlSeconds, suppress)
	return entry, nil
}

func (s *HTTPServer) performAlertSilence(c echo.Context, channel, assignee string, ttlSeconds int64) (monitoring.AlertFeedEntry, error) {
	var until time.Time
	if ttlSeconds > 0 {
		until = time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
	}
	return s.updateAlertEntry(c, ttlSeconds, true, func(tenantID, alertID string) (monitoring.AlertFeedEntry, error) {
		return s.alertFeed.Suppress(tenantID, alertID, channel, assignee, until)
	})
}

func (s *HTTPServer) handleAlertAcknowledge(c echo.Context) error {
	if s.alertFeed == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alert feed unavailable")
	}
	alertID := strings.TrimSpace(c.Param("alertId"))
	if alertID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "alertId is required")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	type request struct {
		Assignee   string `json:"assignee"`
		TTLSeconds int64  `json:"ttlSeconds"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	entry, updateErr := s.alertFeed.Acknowledge(tenantID, alertID, payload.Assignee)
	if updateErr != nil {
		switch {
		case errors.Is(updateErr, monitoring.ErrAlertNotFound):
			return echo.NewHTTPError(http.StatusNotFound, updateErr.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, updateErr.Error())
		}
	}
	s.applyAlertMute(entry.Fingerprint, payload.TTLSeconds, false)
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.action.ack", s.auditPayloadFromRequest(c, alertEntryAuditPayload(entry, map[string]any{
		"ttlSeconds": payload.TTLSeconds,
	})), withResource("alert_event"))
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleAlertSuppress(c echo.Context) error {
	type request struct {
		Channel    string `json:"channel"`
		TTLSeconds int64  `json:"ttlSeconds"`
		Assignee   string `json:"assignee"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	entry, err := s.performAlertSilence(c, payload.Channel, payload.Assignee, payload.TTLSeconds)
	if err != nil {
		return err
	}
	s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "alert.action.suppress", s.auditPayloadFromRequest(c, alertEntryAuditPayload(entry, map[string]any{
		"ttlSeconds": payload.TTLSeconds,
	})), withResource("alert_event"))
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleMonitoringCMDBSync(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	scope, err := s.monitoringScopeRef(c)
	if err != nil {
		return err
	}
	tenantID := scope.TenantOrDefault()
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	if limit <= 0 {
		limit = 25
	}
	ctx := c.Request().Context()
	hotspots, err := s.monitor.Pools(ctx, scope, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	analytics, err := s.monitor.Analytics(ctx, scope, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	alertTotals := monitoring.AlertFeedTotals{}
	if s.alertFeed != nil {
		alertTotals = s.alertFeed.Totals(tenantID)
	}
	payload := monitoring.CMDBSyncPayload{
		TenantID:         tenantID,
		Source:           "modern-dhcp",
		GeneratedAt:      time.Now().UTC(),
		PoolHotspots:     hotspots,
		CapacityInsights: analytics.Capacity,
		AnomalyInsights:  analytics.Anomalies,
		AlertTotals:      alertTotals,
	}
	return c.JSON(http.StatusOK, payload)
}

func (s *HTTPServer) applyAlertMute(fingerprint string, ttlSeconds int64, suppress bool) {
	if s.alertManager == nil || fingerprint == "" {
		return
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	if suppress {
		s.alertManager.Suppress(fingerprint, ttl)
		return
	}
	s.alertManager.Acknowledge(fingerprint, ttl)
}

func (s *HTTPServer) handleOpsSystemSummary() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		summary, err := s.opsSvc.SystemSummary(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, summary)
	}
}

func (s *HTTPServer) handleOpsRouteUsage() echo.HandlerFunc {
	type entry struct {
		Key      string `json:"key"`
		Hits     int    `json:"hits"`
		LastSeen string `json:"lastSeen"`
	}
	return func(c echo.Context) error {
		if s.routeUsage == nil {
			return c.JSON(http.StatusOK, []entry{})
		}
		snapshot := s.routeUsage.Snapshot()
		result := make([]entry, 0, len(snapshot))
		for _, item := range snapshot {
			last := ""
			if !item.LastSeen.IsZero() {
				last = item.LastSeen.UTC().Format(time.RFC3339)
			}
			result = append(result, entry{Key: item.Key, Hits: item.Hits, LastSeen: last})
		}
		return c.JSON(http.StatusOK, result)
	}
}

func (s *HTTPServer) handleOpsUpdateSystem() echo.HandlerFunc {
	type request struct {
		Theme                      *string `json:"theme"`
		Locale                     *string `json:"locale"`
		MaintenanceMode            *bool   `json:"maintenanceMode"`
		MaintenanceWindow          *string `json:"maintenanceWindow"`
		Announcement               *string `json:"announcement"`
		AdminSessionTimeoutMinutes *int    `json:"adminSessionTimeoutMinutes"`
		AutoLogoutEnabled          *bool   `json:"autoLogoutEnabled"`
		SystemLogRetentionDays     *int    `json:"systemLogRetentionDays"`
		AuditLogRetentionDays      *int    `json:"auditLogRetentionDays"`
		LogPushEnabled             *bool   `json:"logPushEnabled"`
		LogPushEndpoint            *string `json:"logPushEndpoint"`
		LogPushMinLevel            *string `json:"logPushMinLevel"`
		LogPushChannels            *string `json:"logPushChannels"`
		LogPushPhones              *string `json:"logPushPhones"`
		LogPushDingTalkEndpoint    *string `json:"logPushDingTalkEndpoint"`
		LogPushFeishuEndpoint      *string `json:"logPushFeishuEndpoint"`
		LogPushWecomEndpoint       *string `json:"logPushWecomEndpoint"`
		LogPushSlackEndpoint       *string `json:"logPushSlackEndpoint"`
		NTPEnabled                 *bool   `json:"ntpEnabled"`
		NTPServers                 *string `json:"ntpServers"`
		NTPIntervalMinutes         *int    `json:"ntpIntervalMinutes"`
		NTPTimeoutSeconds          *int    `json:"ntpTimeoutSeconds"`
		Timezone                   *string `json:"timezone"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		update := ops.SystemSettingsUpdate{
			Theme:                      payload.Theme,
			Locale:                     payload.Locale,
			MaintenanceMode:            payload.MaintenanceMode,
			MaintenanceWindow:          payload.MaintenanceWindow,
			Announcement:               payload.Announcement,
			AdminSessionTimeoutMinutes: payload.AdminSessionTimeoutMinutes,
			AutoLogoutEnabled:          payload.AutoLogoutEnabled,
			SystemLogRetentionDays:     payload.SystemLogRetentionDays,
			AuditLogRetentionDays:      payload.AuditLogRetentionDays,
			LogPushEnabled:             payload.LogPushEnabled,
			LogPushEndpoint:            payload.LogPushEndpoint,
			LogPushMinLevel:            payload.LogPushMinLevel,
			LogPushChannels:            payload.LogPushChannels,
			LogPushPhones:              payload.LogPushPhones,
			LogPushDingTalkEndpoint:    payload.LogPushDingTalkEndpoint,
			LogPushFeishuEndpoint:      payload.LogPushFeishuEndpoint,
			LogPushWecomEndpoint:       payload.LogPushWecomEndpoint,
			LogPushSlackEndpoint:       payload.LogPushSlackEndpoint,
			NTPEnabled:                 payload.NTPEnabled,
			NTPServers:                 payload.NTPServers,
			NTPIntervalMinutes:         payload.NTPIntervalMinutes,
			NTPTimeoutSeconds:          payload.NTPTimeoutSeconds,
			Timezone:                   payload.Timezone,
		}
		if emptySettingsUpdate(update) {
			return echo.NewHTTPError(http.StatusBadRequest, "no settings provided")
		}
		return s.applyOpsSettingsMutation(c, update, "ops.system.update")
	}
}

func (s *HTTPServer) handleOpsUpdateTheme() echo.HandlerFunc {
	type request struct {
		Theme string `json:"theme"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		value := strings.TrimSpace(payload.Theme)
		if value == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "theme is required")
		}
		update := ops.SystemSettingsUpdate{Theme: &value}
		return s.applyOpsSettingsMutation(c, update, "ops.system.theme")
	}
}

func (s *HTTPServer) handleOpsUpdateLocale() echo.HandlerFunc {
	type request struct {
		Locale string `json:"locale"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		value := strings.TrimSpace(payload.Locale)
		if value == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "locale is required")
		}
		update := ops.SystemSettingsUpdate{Locale: &value}
		return s.applyOpsSettingsMutation(c, update, "ops.system.locale")
	}
}

func (s *HTTPServer) handleOpsUpdateMaintenance() echo.HandlerFunc {
	type request struct {
		MaintenanceMode   *bool   `json:"maintenanceMode"`
		MaintenanceWindow *string `json:"maintenanceWindow"`
		Announcement      *string `json:"announcement"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		update := ops.SystemSettingsUpdate{
			MaintenanceMode:   payload.MaintenanceMode,
			MaintenanceWindow: payload.MaintenanceWindow,
			Announcement:      payload.Announcement,
		}
		if emptySettingsUpdate(update) {
			return echo.NewHTTPError(http.StatusBadRequest, "no maintenance fields provided")
		}
		return s.applyOpsSettingsMutation(c, update, "ops.system.maintenance")
	}
}

func (s *HTTPServer) handleOpsUpdateNTP() echo.HandlerFunc {
	type request struct {
		NTPEnabled         *bool   `json:"ntpEnabled"`
		NTPServers         *string `json:"ntpServers"`
		NTPIntervalMinutes *int    `json:"ntpIntervalMinutes"`
		NTPTimeoutSeconds  *int    `json:"ntpTimeoutSeconds"`
		Timezone           *string `json:"timezone"`
	}
	return func(c echo.Context) error {
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		update := ops.SystemSettingsUpdate{
			NTPEnabled:         payload.NTPEnabled,
			NTPServers:         payload.NTPServers,
			NTPIntervalMinutes: payload.NTPIntervalMinutes,
			NTPTimeoutSeconds:  payload.NTPTimeoutSeconds,
			Timezone:           payload.Timezone,
		}
		if emptySettingsUpdate(update) {
			return echo.NewHTTPError(http.StatusBadRequest, "no ntp fields provided")
		}
		return s.applyOpsSettingsMutation(c, update, "ops.system.ntp")
	}
}

func (s *HTTPServer) handleOpsSyncNTP() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		ctx := c.Request().Context()
		before, err := s.opsSvc.CurrentSettings(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := s.actorFromContext(c)
		after, err := s.opsSvc.SyncNTP(ctx, actor)
		if err != nil {
			summary, sumErr := s.opsSvc.SystemSummary(ctx)
			if sumErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, sumErr.Error())
			}
			payload := settingsAuditPayload(before, after)
			payload["syncError"] = err.Error()
			s.recordAudit(
				ctx,
				s.auditTenantFromContext(c),
				actor,
				"ops.system.ntp.sync",
				payload,
				withResource("ops_system"),
			)
			switch {
			case errors.Is(err, ops.ErrSettingsStoreUnavailable):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "settings store unavailable")
			case errors.Is(err, ops.ErrNTPNoServers):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			case errors.Is(err, ops.ErrNTPSyncFailed):
				return c.JSON(http.StatusBadGateway, map[string]any{
					"message": err.Error(),
					"summary": summary,
				})
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		summary, sumErr := s.opsSvc.SystemSummary(ctx)
		if sumErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, sumErr.Error())
		}
		s.recordAudit(
			ctx,
			s.auditTenantFromContext(c),
			actor,
			"ops.system.ntp.sync",
			settingsAuditPayload(before, after),
			withResource("ops_system"),
		)
		return c.JSON(http.StatusOK, summary)
	}
}

func (s *HTTPServer) handleOpsImportExport() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		summary := s.opsSvc.ImportExportSummary(c.Request().Context())
		return c.JSON(http.StatusOK, summary)
	}
}

func (s *HTTPServer) handleOpsCreateTransferJob() echo.HandlerFunc {
	type request struct {
		Kind      string            `json:"kind"`
		Resource  string            `json:"resource"`
		Format    string            `json:"format"`
		SourceURI string            `json:"sourceUri"`
		TargetURI string            `json:"targetUri"`
		Reason    string            `json:"reason"`
		Metadata  map[string]string `json:"metadata"`
		SizeBytes int64             `json:"sizeBytes"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		requestedBy := s.actorFromContext(c)
		if requestedBy == "" {
			requestedBy = s.principalFromContext(c)
		}
		job, err := s.opsSvc.StartTransferJob(c.Request().Context(), ops.TransferRequest{
			Kind:        ops.TransferKind(strings.ToLower(strings.TrimSpace(payload.Kind))),
			Resource:    strings.TrimSpace(payload.Resource),
			Format:      strings.TrimSpace(payload.Format),
			SourceURI:   strings.TrimSpace(payload.SourceURI),
			TargetURI:   strings.TrimSpace(payload.TargetURI),
			Reason:      strings.TrimSpace(payload.Reason),
			Metadata:    cloneStringMap(payload.Metadata),
			RequestedBy: requestedBy,
			SizeBytes:   payload.SizeBytes,
		})
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrImportExportDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "import/export disabled")
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		action, resource := migrationTransferAuditMetadata(job)
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			action,
			transferJobAuditPayload(job),
			withResource(resource),
		)
		return c.JSON(http.StatusAccepted, job)
	}
}

func alertEntryAuditPayload(entry monitoring.AlertFeedEntry, extras map[string]any) map[string]any {
	payload := map[string]any{
		"alertId":         entry.ID,
		"tenantId":        entry.TenantID,
		"fingerprint":     entry.Fingerprint,
		"severity":        strings.ToLower(string(entry.Severity)),
		"lifecycle":       string(entry.Lifecycle),
		"assignee":        entry.Assignee,
		"channel":         entry.Channel,
		"escalationLevel": entry.EscalationLevel,
	}
	for key, value := range extras {
		payload[key] = value
	}
	return payload
}

func migrationTransferAuditMetadata(job *ops.TransferJob) (action, resource string) {
	action = "ops.transfer.job.start"
	resource = "ops_transfer_job"
	if job == nil {
		return action, resource
	}
	kind := strings.ToLower(strings.TrimSpace(string(job.Kind)))
	if kind == "" {
		kind = "run"
	}
	resourceKey := strings.ToLower(strings.TrimSpace(job.Resource))
	switch {
	case strings.Contains(resourceKey, "pool"):
		return "migration.pool." + kind, "migration_pool"
	case strings.Contains(resourceKey, "binding"):
		return "migration.binding." + kind, "migration_binding"
	case strings.Contains(resourceKey, "lease"):
		return "migration.lease." + kind, "migration_lease"
	case strings.Contains(resourceKey, "archive"):
		return "migration.archive." + kind, "migration_archive"
	case strings.Contains(resourceKey, "cleanup"):
		return "migration.cleanup." + kind, "migration_cleanup"
	default:
		return "migration.transfer." + kind, "migration_transfer"
	}
}

func (s *HTTPServer) handleOpsListTransferJobs() echo.HandlerFunc {
	type response struct {
		Items  []ops.TransferJob `json:"items"`
		Total  int               `json:"total"`
		Limit  int               `json:"limit"`
		Offset int               `json:"offset"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		fetchLimit := page.Offset + page.Limit
		if fetchLimit <= 0 {
			fetchLimit = page.Limit
		}
		jobs, listErr := s.opsSvc.ListTransferJobs(c.Request().Context(), fetchLimit)
		if listErr != nil {
			switch {
			case errors.Is(listErr, ops.ErrImportExportDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "import/export disabled")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
			}
		}
		statusFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("status")))
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		filtered := make([]ops.TransferJob, 0, len(jobs))
		for _, job := range jobs {
			if statusFilter != "" && strings.ToLower(strings.TrimSpace(string(job.Status))) != statusFilter {
				continue
			}
			if keyword != "" {
				if !strings.Contains(strings.ToLower(strings.TrimSpace(job.Resource)), keyword) &&
					!strings.Contains(strings.ToLower(strings.TrimSpace(job.RequestedBy)), keyword) &&
					!strings.Contains(strings.ToLower(strings.TrimSpace(job.ID)), keyword) {
					continue
				}
			}
			filtered = append(filtered, job)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, response{Items: []ops.TransferJob{}, Total: total, Limit: page.Limit, Offset: page.Offset})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, response{Items: filtered[page.Offset:end], Total: total, Limit: page.Limit, Offset: page.Offset})
	}
}

func (s *HTTPServer) handleOpsGetTransferJob() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		jobID := strings.TrimSpace(c.Param("jobId"))
		if jobID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobId is required")
		}
		job, err := s.opsSvc.GetTransferJob(c.Request().Context(), jobID)
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrImportExportDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "import/export disabled")
			case errors.Is(err, ops.ErrTransferJobNotFound):
				return respondNotFound(c, "任务不存在或已过期")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(http.StatusOK, job)
	}
}

func (s *HTTPServer) handleMaintenancePlan() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		plan := s.opsSvc.MaintenancePlan(c.Request().Context())
		return c.JSON(http.StatusOK, plan)
	}
}

func (s *HTTPServer) handleMaintenancePlaybooks() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		plan := s.opsSvc.MaintenancePlan(c.Request().Context())
		return c.JSON(http.StatusOK, plan.Playbooks)
	}
}

func (s *HTTPServer) handleBackupWizard() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		wizard := s.opsSvc.BackupWizard(c.Request().Context())
		return c.JSON(http.StatusOK, wizard)
	}
}

func (s *HTTPServer) handleBackupSchedules() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		wizard := s.opsSvc.BackupWizard(c.Request().Context())
		return c.JSON(http.StatusOK, wizard.Schedules)
	}
}

func (s *HTTPServer) handleBackupDestinations() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		wizard := s.opsSvc.BackupWizard(c.Request().Context())
		return c.JSON(http.StatusOK, wizard.Destinations)
	}
}

func (s *HTTPServer) handlePerformanceDiagnostics() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		diagnostics := s.opsSvc.PerformanceDiagnostics(c.Request().Context())
		return c.JSON(http.StatusOK, diagnostics)
	}
}

func (s *HTTPServer) applyOpsSettingsMutation(c echo.Context, update ops.SystemSettingsUpdate, action string) error {
	if s.opsSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
	}
	ctx := c.Request().Context()
	before, err := s.opsSvc.CurrentSettings(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	actor := s.actorFromContext(c)
	after, err := s.opsSvc.UpdateSettings(ctx, update, actor)
	if err != nil {
		switch {
		case errors.Is(err, ops.ErrSettingsStoreUnavailable):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "settings store unavailable")
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	summary, sumErr := s.opsSvc.SystemSummary(ctx)
	if sumErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, sumErr.Error())
	}
	derivedActions := deriveSystemActionsFromSettings(before, after)
	payload := settingsAuditPayload(before, after)
	if len(derivedActions) > 0 {
		payload["derivedActions"] = append([]string(nil), derivedActions...)
	}
	s.recordAudit(
		ctx,
		s.auditTenantFromContext(c),
		actor,
		action,
		payload,
		withResource("ops_system"),
	)
	return c.JSON(http.StatusOK, summary)
}

func deriveSystemActionsFromSettings(before, after ops.SystemSettings) []string {
	actions := make([]string, 0, 2)
	if before.MaintenanceMode != after.MaintenanceMode {
		if after.MaintenanceMode {
			actions = append(actions, "system.service.stop")
		} else {
			actions = append(actions, "system.service.start")
		}
	}
	if before.MaintenanceWindow != after.MaintenanceWindow || before.Announcement != after.Announcement {
		actions = append(actions, "system.reload")
	}
	return actions
}

func emptySettingsUpdate(update ops.SystemSettingsUpdate) bool {
	return update.Theme == nil && update.Locale == nil && update.MaintenanceMode == nil && update.MaintenanceWindow == nil && update.Announcement == nil && update.AdminSessionTimeoutMinutes == nil && update.AutoLogoutEnabled == nil && update.SystemLogRetentionDays == nil && update.AuditLogRetentionDays == nil && update.LogPushEnabled == nil && update.LogPushEndpoint == nil && update.LogPushMinLevel == nil && update.LogPushChannels == nil && update.LogPushPhones == nil && update.LogPushDingTalkEndpoint == nil && update.LogPushFeishuEndpoint == nil && update.LogPushWecomEndpoint == nil && update.LogPushSlackEndpoint == nil && update.NTPEnabled == nil && update.NTPServers == nil && update.NTPIntervalMinutes == nil && update.NTPTimeoutSeconds == nil && update.Timezone == nil && update.NTPSyncStatus == nil && update.NTPLastSyncAt == nil
}

func settingsAuditPayload(before, after ops.SystemSettings) map[string]any {
	changes := map[string]map[string]any{}
	if before.Theme != after.Theme {
		changes["theme"] = map[string]any{"before": before.Theme, "after": after.Theme}
	}
	if before.Locale != after.Locale {
		changes["locale"] = map[string]any{"before": before.Locale, "after": after.Locale}
	}
	if before.MaintenanceMode != after.MaintenanceMode {
		changes["maintenanceMode"] = map[string]any{"before": before.MaintenanceMode, "after": after.MaintenanceMode}
	}
	if before.MaintenanceWindow != after.MaintenanceWindow {
		changes["maintenanceWindow"] = map[string]any{"before": before.MaintenanceWindow, "after": after.MaintenanceWindow}
	}
	if before.Announcement != after.Announcement {
		changes["announcement"] = map[string]any{"before": before.Announcement, "after": after.Announcement}
	}
	if before.AdminSessionTimeoutMinutes != after.AdminSessionTimeoutMinutes {
		changes["adminSessionTimeoutMinutes"] = map[string]any{"before": before.AdminSessionTimeoutMinutes, "after": after.AdminSessionTimeoutMinutes}
	}
	if before.AutoLogoutEnabled != after.AutoLogoutEnabled {
		changes["autoLogoutEnabled"] = map[string]any{"before": before.AutoLogoutEnabled, "after": after.AutoLogoutEnabled}
	}
	if before.SystemLogRetentionDays != after.SystemLogRetentionDays {
		changes["systemLogRetentionDays"] = map[string]any{"before": before.SystemLogRetentionDays, "after": after.SystemLogRetentionDays}
	}
	if before.AuditLogRetentionDays != after.AuditLogRetentionDays {
		changes["auditLogRetentionDays"] = map[string]any{"before": before.AuditLogRetentionDays, "after": after.AuditLogRetentionDays}
	}
	if before.LogPushEnabled != after.LogPushEnabled {
		changes["logPushEnabled"] = map[string]any{"before": before.LogPushEnabled, "after": after.LogPushEnabled}
	}
	if before.LogPushEndpoint != after.LogPushEndpoint {
		changes["logPushEndpoint"] = map[string]any{"before": before.LogPushEndpoint, "after": after.LogPushEndpoint}
	}
	if before.LogPushMinLevel != after.LogPushMinLevel {
		changes["logPushMinLevel"] = map[string]any{"before": before.LogPushMinLevel, "after": after.LogPushMinLevel}
	}
	if before.LogPushChannels != after.LogPushChannels {
		changes["logPushChannels"] = map[string]any{"before": before.LogPushChannels, "after": after.LogPushChannels}
	}
	if before.LogPushPhones != after.LogPushPhones {
		changes["logPushPhones"] = map[string]any{"before": before.LogPushPhones, "after": after.LogPushPhones}
	}
	if before.LogPushDingTalkEndpoint != after.LogPushDingTalkEndpoint {
		changes["logPushDingTalkEndpoint"] = map[string]any{"before": before.LogPushDingTalkEndpoint, "after": after.LogPushDingTalkEndpoint}
	}
	if before.LogPushFeishuEndpoint != after.LogPushFeishuEndpoint {
		changes["logPushFeishuEndpoint"] = map[string]any{"before": before.LogPushFeishuEndpoint, "after": after.LogPushFeishuEndpoint}
	}
	if before.LogPushWecomEndpoint != after.LogPushWecomEndpoint {
		changes["logPushWecomEndpoint"] = map[string]any{"before": before.LogPushWecomEndpoint, "after": after.LogPushWecomEndpoint}
	}
	if before.LogPushSlackEndpoint != after.LogPushSlackEndpoint {
		changes["logPushSlackEndpoint"] = map[string]any{"before": before.LogPushSlackEndpoint, "after": after.LogPushSlackEndpoint}
	}
	if before.NTPEnabled != after.NTPEnabled {
		changes["ntpEnabled"] = map[string]any{"before": before.NTPEnabled, "after": after.NTPEnabled}
	}
	if before.NTPServers != after.NTPServers {
		changes["ntpServers"] = map[string]any{"before": before.NTPServers, "after": after.NTPServers}
	}
	if before.NTPIntervalMinutes != after.NTPIntervalMinutes {
		changes["ntpIntervalMinutes"] = map[string]any{"before": before.NTPIntervalMinutes, "after": after.NTPIntervalMinutes}
	}
	if before.NTPTimeoutSeconds != after.NTPTimeoutSeconds {
		changes["ntpTimeoutSeconds"] = map[string]any{"before": before.NTPTimeoutSeconds, "after": after.NTPTimeoutSeconds}
	}
	if before.Timezone != after.Timezone {
		changes["timezone"] = map[string]any{"before": before.Timezone, "after": after.Timezone}
	}
	if before.NTPSyncStatus != after.NTPSyncStatus {
		changes["ntpSyncStatus"] = map[string]any{"before": before.NTPSyncStatus, "after": after.NTPSyncStatus}
	}
	beforeSync := ""
	afterSync := ""
	if before.NTPLastSyncAt != nil {
		beforeSync = before.NTPLastSyncAt.UTC().Format(time.RFC3339)
	}
	if after.NTPLastSyncAt != nil {
		afterSync = after.NTPLastSyncAt.UTC().Format(time.RFC3339)
	}
	if beforeSync != afterSync {
		changes["ntpLastSyncAt"] = map[string]any{"before": beforeSync, "after": afterSync}
	}
	if len(changes) == 0 {
		changes = nil
	}
	return map[string]any{
		"before":  before,
		"after":   after,
		"changes": changes,
	}
}

func (s *HTTPServer) handleOpsHelpCenter() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		info := s.opsSvc.HelpCenter(c.Request().Context())
		return c.JSON(http.StatusOK, info)
	}
}

func (s *HTTPServer) handleOpsHelpArticles() echo.HandlerFunc {
	type response struct {
		Items       []ops.HelpArticle `json:"items"`
		Total       int               `json:"total"`
		Limit       int               `json:"limit"`
		Offset      int               `json:"offset"`
		GeneratedAt time.Time         `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		articles := s.opsSvc.HelpArticles(c.Request().Context())
		filtered := make([]ops.HelpArticle, 0, len(articles))
		for _, item := range articles {
			if keyword != "" {
				title := strings.ToLower(strings.TrimSpace(item.Title))
				summary := strings.ToLower(strings.TrimSpace(item.Summary))
				if !strings.Contains(title, keyword) && !strings.Contains(summary, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, response{Items: []ops.HelpArticle{}, Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, response{Items: filtered[page.Offset:end], Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
	}
}

func (s *HTTPServer) handleOpsHelpFAQ() echo.HandlerFunc {
	type response struct {
		Items       []ops.HelpFAQEntry `json:"items"`
		Total       int                `json:"total"`
		Limit       int                `json:"limit"`
		Offset      int                `json:"offset"`
		GeneratedAt time.Time          `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		category := strings.ToLower(strings.TrimSpace(c.QueryParam("category")))
		entries := s.opsSvc.FAQEntries(c.Request().Context())
		filtered := make([]ops.HelpFAQEntry, 0, len(entries))
		for _, item := range entries {
			if category != "" && strings.ToLower(strings.TrimSpace(item.Category)) != category {
				continue
			}
			if keyword != "" {
				question := strings.ToLower(strings.TrimSpace(item.Question))
				answer := strings.ToLower(strings.TrimSpace(item.Answer))
				if !strings.Contains(question, keyword) && !strings.Contains(answer, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, response{Items: []ops.HelpFAQEntry{}, Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, response{Items: filtered[page.Offset:end], Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
	}
}

func (s *HTTPServer) handleOpsHelpReleases() echo.HandlerFunc {
	type response struct {
		Items       []ops.ReleaseNote `json:"items"`
		Total       int               `json:"total"`
		Limit       int               `json:"limit"`
		Offset      int               `json:"offset"`
		GeneratedAt time.Time         `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		releases := s.opsSvc.ReleaseNotes(c.Request().Context())
		filtered := make([]ops.ReleaseNote, 0, len(releases))
		for _, item := range releases {
			if keyword != "" {
				title := strings.ToLower(strings.TrimSpace(item.Title))
				version := strings.ToLower(strings.TrimSpace(item.Version))
				highlights := strings.ToLower(strings.Join(item.Highlights, " "))
				if !strings.Contains(title, keyword) && !strings.Contains(version, keyword) && !strings.Contains(highlights, keyword) {
					continue
				}
			}
			filtered = append(filtered, item)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, response{Items: []ops.ReleaseNote{}, Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, response{Items: filtered[page.Offset:end], Total: total, Limit: page.Limit, Offset: page.Offset, GeneratedAt: time.Now().UTC()})
	}
}

func (s *HTTPServer) handleOpsSupportDirectory() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		dir := s.opsSvc.SupportDirectory()
		return c.JSON(http.StatusOK, dir)
	}
}

func (s *HTTPServer) handleOpsScriptCatalog() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		catalog := s.opsSvc.ScriptCatalog()
		return c.JSON(http.StatusOK, catalog)
	}
}

func (s *HTTPServer) handleOpsStartScriptRun() echo.HandlerFunc {
	type request struct {
		Reason         string            `json:"reason"`
		Args           []string          `json:"args"`
		Env            map[string]string `json:"env"`
		TimeoutSeconds int64             `json:"timeoutSeconds"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		scriptName := strings.TrimSpace(c.Param("scriptName"))
		if scriptName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "scriptName is required")
		}
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if payload.TimeoutSeconds < 0 {
			return respondValidationError(c, "timeoutSeconds 必须大于等于 0", fieldError{Field: "timeoutSeconds", Message: "请输入不小于 0 的秒数"})
		}
		timeout := time.Duration(payload.TimeoutSeconds) * time.Second
		requestedBy := s.principalFromContext(c)
		if requestedBy == "" {
			requestedBy = s.actorFromContext(c)
		}
		run, err := s.opsSvc.StartScriptRun(c.Request().Context(), ops.StartScriptRunInput{
			ScriptName:  scriptName,
			RequestedBy: requestedBy,
			Role:        s.roleFromContext(c),
			Reason:      strings.TrimSpace(payload.Reason),
			Args:        append([]string(nil), payload.Args...),
			Env:         payload.Env,
			Timeout:     timeout,
		})
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			case errors.Is(err, ops.ErrScriptNotFound):
				return respondNotFound(c, "脚本不存在或未被允许")
			case errors.Is(err, ops.ErrScriptRoleDenied):
				return echo.NewHTTPError(http.StatusForbidden, "当前角色无法执行该脚本")
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"ops.script_run.start",
			scriptRunAuditPayload(run),
			withResource("ops_script_run"),
		)
		if action, resource, ok := deriveSystemActionFromScriptName(run.ScriptName); ok {
			s.recordAudit(
				c.Request().Context(),
				s.auditTenantFromContext(c),
				s.actorFromContext(c),
				action,
				scriptRunAuditPayload(run),
				withResource(resource),
			)
		}
		return c.JSON(http.StatusAccepted, run)
	}
}

func (s *HTTPServer) handleOpsListScriptRuns() echo.HandlerFunc {
	type response struct {
		Items  []ops.ScriptRun `json:"items"`
		Total  int             `json:"total"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		fetchLimit := page.Offset + page.Limit
		if fetchLimit <= 0 {
			fetchLimit = page.Limit
		}
		runs, listErr := s.opsSvc.ListScriptRuns(c.Request().Context(), fetchLimit)
		if listErr != nil {
			switch {
			case errors.Is(listErr, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
			}
		}
		statusFilter := strings.ToLower(strings.TrimSpace(c.QueryParam("status")))
		keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
		filtered := make([]ops.ScriptRun, 0, len(runs))
		for _, run := range runs {
			if statusFilter != "" && strings.ToLower(strings.TrimSpace(string(run.Status))) != statusFilter {
				continue
			}
			if keyword != "" {
				if !strings.Contains(strings.ToLower(strings.TrimSpace(run.ScriptName)), keyword) &&
					!strings.Contains(strings.ToLower(strings.TrimSpace(run.RequestedBy)), keyword) &&
					!strings.Contains(strings.ToLower(strings.TrimSpace(run.ID)), keyword) {
					continue
				}
			}
			filtered = append(filtered, run)
		}
		total := len(filtered)
		if page.Offset >= total {
			return c.JSON(http.StatusOK, response{Items: []ops.ScriptRun{}, Total: total, Limit: page.Limit, Offset: page.Offset})
		}
		end := page.Offset + page.Limit
		if end > total {
			end = total
		}
		return c.JSON(http.StatusOK, response{Items: filtered[page.Offset:end], Total: total, Limit: page.Limit, Offset: page.Offset})
	}
}

func (s *HTTPServer) handleOpsGetScriptRun() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		runID := strings.TrimSpace(c.Param("runId"))
		if runID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "runId is required")
		}
		run, err := s.opsSvc.GetScriptRun(c.Request().Context(), runID)
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			case errors.Is(err, ops.ErrScriptRunNotFound):
				return respondNotFound(c, "执行记录不存在或已被清理")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(http.StatusOK, run)
	}
}

func (s *HTTPServer) handleOpsApproveScriptRun() echo.HandlerFunc {
	type request struct {
		Note string `json:"note"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		runID := strings.TrimSpace(c.Param("runId"))
		if runID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "runId is required")
		}
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		run, err := s.opsSvc.ApproveScriptRun(c.Request().Context(), runID, s.actorFromContext(c), strings.TrimSpace(payload.Note))
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			case errors.Is(err, ops.ErrScriptRunNotFound):
				return respondNotFound(c, "执行记录不存在或已被清理")
			case errors.Is(err, ops.ErrScriptRunNotPendingApproval):
				return echo.NewHTTPError(http.StatusConflict, "当前执行不在审批状态")
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"ops.script_run.approve",
			scriptRunAuditPayload(run),
			withResource("ops_script_run"),
		)
		if action, resource, ok := deriveSystemActionFromScriptName(run.ScriptName); ok {
			s.recordAudit(
				c.Request().Context(),
				s.auditTenantFromContext(c),
				s.actorFromContext(c),
				action,
				scriptRunAuditPayload(run),
				withResource(resource),
			)
		}
		return c.JSON(http.StatusOK, run)
	}
}

func (s *HTTPServer) handleOpsRejectScriptRun() echo.HandlerFunc {
	type request struct {
		Note string `json:"note"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		runID := strings.TrimSpace(c.Param("runId"))
		if runID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "runId is required")
		}
		var payload request
		if err := c.Bind(&payload); err != nil && !errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		run, err := s.opsSvc.RejectScriptRun(c.Request().Context(), runID, s.actorFromContext(c), strings.TrimSpace(payload.Note))
		if err != nil {
			switch {
			case errors.Is(err, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			case errors.Is(err, ops.ErrScriptRunNotFound):
				return respondNotFound(c, "执行记录不存在或已被清理")
			case errors.Is(err, ops.ErrScriptRunNotPendingApproval):
				return echo.NewHTTPError(http.StatusConflict, "当前执行不在审批状态")
			default:
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"ops.script_run.reject",
			scriptRunAuditPayload(run),
			withResource("ops_script_run"),
		)
		return c.JSON(http.StatusOK, run)
	}
}

func (s *HTTPServer) handleDiagnosticsSnapshot() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 50
		}
		scope := s.leaseScopeRef(c)
		if tenantID := strings.TrimSpace(c.QueryParam("tenantId")); tenantID != "" {
			scope = scope.WithTenantOverride(tenantID)
		}
		tenantID := strings.TrimSpace(scope.TenantOrDefault())
		snapshot := diagnosticsSnapshot{
			GeneratedAt: time.Now().UTC(),
			TenantID:    tenantID,
			System:      s.buildSystemHealthResponse(ctx),
		}
		if s.monitor == nil {
			snapshot.Notes = append(snapshot.Notes, "monitoring disabled: analytics unavailable")
		} else if tenantID == "" {
			snapshot.Notes = append(snapshot.Notes, "tenantId not provided: analytics skipped")
		} else if analytics, analyticsErr := s.monitor.Analytics(ctx, scope, limit); analyticsErr == nil {
			snapshot.Analytics = &analytics
		} else {
			snapshot.Notes = append(snapshot.Notes, "monitoring analytics failed: "+analyticsErr.Error())
		}
		if s.automation != nil {
			snap := s.automation.Snapshot()
			snapshot.Automation = &snap
		} else {
			snapshot.Notes = append(snapshot.Notes, "automation scheduler disabled")
		}
		if s.opsSvc != nil {
			catalog := s.opsSvc.ScriptCatalog()
			snapshot.Scripts = &catalog
		} else {
			snapshot.Notes = append(snapshot.Notes, "ops support unavailable: script catalog missing")
		}
		format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))
		download := strings.ToLower(strings.TrimSpace(c.QueryParam("download")))
		if format == "zip" || download == "zip" {
			return s.respondDiagnosticsZip(c, snapshot)
		}
		return c.JSON(http.StatusOK, snapshot)
	}
}

func (s *HTTPServer) respondDiagnosticsZip(c echo.Context, snapshot diagnosticsSnapshot) error {
	buffer := bytes.NewBuffer(nil)
	archive := zip.NewWriter(buffer)
	if err := writeJSONZipFile(archive, "snapshot.json", snapshot); err != nil {
		archive.Close()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := writeJSONZipFile(archive, "system.json", snapshot.System); err != nil {
		archive.Close()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if snapshot.Analytics != nil {
		if err := writeJSONZipFile(archive, "analytics.json", snapshot.Analytics); err != nil {
			archive.Close()
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if snapshot.Automation != nil {
		if err := writeJSONZipFile(archive, "automation.json", snapshot.Automation); err != nil {
			archive.Close()
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if snapshot.Scripts != nil {
		if err := writeJSONZipFile(archive, "scripts.json", snapshot.Scripts); err != nil {
			archive.Close()
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if len(snapshot.Notes) > 0 {
		if err := addTextZipFile(archive, "notes.txt", strings.Join(snapshot.Notes, "\n")); err != nil {
			archive.Close()
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if err := archive.Close(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	filename := fmt.Sprintf("diagnostics_%s.zip", time.Now().UTC().Format("20060102T150405Z"))
	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, "application/zip")
	resp.Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", filename))
	resp.WriteHeader(http.StatusOK)
	if _, err := resp.Write(buffer.Bytes()); err != nil {
		return err
	}
	return nil
}

func writeJSONZipFile(archive *zip.Writer, name string, payload any) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func addTextZipFile(archive *zip.Writer, name, content string) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	if content == "" {
		content = "(empty)"
	}
	_, err = writer.Write([]byte(content))
	return err
}

func scriptRunAuditPayload(run *ops.ScriptRun) map[string]any {
	if run == nil {
		return nil
	}
	payload := map[string]any{
		"runId":            run.ID,
		"script":           run.ScriptName,
		"status":           string(run.Status),
		"requiresApproval": run.RequiresApproval,
	}
	if run.RequestedBy != "" {
		payload["requestedBy"] = run.RequestedBy
	}
	if run.Role != "" {
		payload["requestedRole"] = run.Role
	}
	if run.Reason != "" {
		payload["reason"] = run.Reason
	}
	if len(run.ExtraArgs) > 0 {
		payload["extraArgs"] = append([]string(nil), run.ExtraArgs...)
	}
	if run.ApprovedBy != "" {
		payload["approvedBy"] = run.ApprovedBy
	}
	if !run.ApprovedAt.IsZero() {
		payload["approvedAt"] = run.ApprovedAt
	}
	if run.ApprovalNote != "" {
		payload["approvalNote"] = run.ApprovalNote
	}
	if run.RejectedBy != "" {
		payload["rejectedBy"] = run.RejectedBy
	}
	if !run.RejectedAt.IsZero() {
		payload["rejectedAt"] = run.RejectedAt
	}
	if run.RejectionNote != "" {
		payload["rejectionNote"] = run.RejectionNote
	}
	return payload
}

func deriveSystemActionFromScriptName(scriptName string) (action, resource string, ok bool) {
	name := strings.ToLower(strings.TrimSpace(scriptName))
	switch {
	case strings.Contains(name, "upgrade"):
		return "system.upgrade", "system_upgrade", true
	case strings.Contains(name, "reload") || strings.Contains(name, "restart"):
		return "system.reload", "system_service", true
	case strings.Contains(name, "shutdown") || strings.Contains(name, "stop"):
		return "system.service.stop", "system_service", true
	case strings.Contains(name, "startup") || strings.Contains(name, "start"):
		return "system.service.start", "system_service", true
	default:
		return "", "", false
	}
}

func transferJobAuditPayload(job *ops.TransferJob) map[string]any {
	if job == nil {
		return nil
	}
	payload := map[string]any{
		"jobId":       job.ID,
		"kind":        job.Kind,
		"resource":    job.Resource,
		"format":      job.Format,
		"status":      job.Status,
		"sourceUri":   job.SourceURI,
		"targetUri":   job.TargetURI,
		"artifact":    job.ArtifactPath,
		"requestedBy": job.RequestedBy,
	}
	if job.Reason != "" {
		payload["reason"] = job.Reason
	}
	if len(job.Metadata) > 0 {
		payload["metadata"] = cloneStringMap(job.Metadata)
	}
	if job.Error != "" {
		payload["error"] = job.Error
	}
	return payload
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dup := make(map[string]string, len(src))
	for k, v := range src {
		dup[k] = v
	}
	return dup
}

func (s *HTTPServer) handleAuditActivity(c echo.Context) error {
	if s.auditSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "audit service disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	page, err := parsePagination(c)
	if err != nil {
		return err
	}
	snapshot := s.safeOperationSnapshot(c.Request().Context(), tenantID, page.Limit, page.Offset)
	return c.JSON(http.StatusOK, snapshot)
}

type statusMessage struct {
	Type        string                          `json:"type"`
	GeneratedAt time.Time                       `json:"generatedAt"`
	TenantID    string                          `json:"tenantId,omitempty"`
	System      monitoring.SystemHealthSnapshot `json:"system"`
	HA          any                             `json:"ha,omitempty"`
	Overview    *monitoring.OverviewSnapshot    `json:"overview,omitempty"`
}

type systemHealthResponse struct {
	GeneratedAt   time.Time     `json:"generatedAt"`
	UptimeSeconds int64         `json:"uptimeSeconds"`
	Cluster       clusterStatus `json:"cluster"`
	Checks        []healthCheck `json:"checks"`
}

type diagnosticsSnapshot struct {
	GeneratedAt time.Time                     `json:"generatedAt"`
	TenantID    string                        `json:"tenantId,omitempty"`
	System      systemHealthResponse          `json:"system"`
	Analytics   *monitoring.AnalyticsSnapshot `json:"analytics,omitempty"`
	Automation  *automation.Snapshot          `json:"automation,omitempty"`
	Scripts     *ops.ScriptCatalog            `json:"scripts,omitempty"`
	Notes       []string                      `json:"notes,omitempty"`
}

type clusterStatus struct {
	Role     string   `json:"role"`
	State    string   `json:"state"`
	Nodes    []string `json:"nodes,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type healthCheck struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Details   string `json:"details,omitempty"`
	LatencyMs int    `json:"latencyMs,omitempty"`
}

func (s *HTTPServer) handleStatusStream() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.monitor == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
		}
		conn, err := statusStreamUpgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.Close()
		scope := s.leaseScopeRef(c)
		if tenantID := strings.TrimSpace(c.QueryParam("tenantId")); tenantID != "" {
			scope = scope.WithTenantOverride(tenantID)
		}
		ctx, cancel := context.WithCancel(c.Request().Context())
		defer cancel()
		conn.SetCloseHandler(func(code int, text string) error {
			cancel()
			return nil
		})
		if payload := s.buildStatusPayload(ctx, scope); payload != nil {
			if err := conn.WriteJSON(payload); err != nil {
				return nil
			}
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if payload := s.buildStatusPayload(ctx, scope); payload != nil {
					if err := conn.WriteJSON(payload); err != nil {
						return nil
					}
				}
			}
		}
	}
}

func (s *HTTPServer) buildStatusPayload(ctx context.Context, scope lease.ResourceScope) *statusMessage {
	if s.monitor == nil {
		return nil
	}
	logger := LoggerFromContext(ctx, s.logger)
	tenantID := strings.TrimSpace(scope.TenantOrDefault())
	message := &statusMessage{
		Type:        "status",
		GeneratedAt: time.Now().UTC(),
		TenantID:    tenantID,
		System:      s.monitor.Health(ctx),
	}
	if tenantID == "" {
		message.TenantID = ""
	}
	if s.options.Coordinator != nil {
		message.HA = s.options.Coordinator.Snapshot()
	}
	if tenantID != "" {
		if overview, err := s.monitor.Overview(ctx, scope, 10); err == nil {
			message.Overview = &overview
		} else if logger != nil {
			logger.Debug("status stream overview failed", zap.String("tenantId", tenantID), zap.Error(err))
		}
	}
	return message
}

func (s *HTTPServer) buildSystemHealthResponse(ctx context.Context) systemHealthResponse {
	var snapshot monitoring.SystemHealthSnapshot
	if s.monitor != nil {
		snapshot = s.monitor.Health(ctx)
	}
	return systemHealthResponse{
		GeneratedAt:   time.Now().UTC(),
		UptimeSeconds: s.serverUptimeSeconds(),
		Cluster:       s.clusterStatusSnapshot(),
		Checks:        buildHealthChecks(snapshot),
	}
}

func (s *HTTPServer) convertAuditEvent(evt models.AuditEvent) operationLogEntry {
	entry := operationLogEntry{
		ID:        evt.AuditID,
		Actor:     evt.Actor,
		Action:    evt.Action,
		Resource:  evt.Resource,
		Scope:     evt.Source,
		Status:    classifyAuditStatus(evt.Action),
		CreatedAt: evt.CreatedAt.UTC(),
	}
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("audit-%d", evt.ID)
	}
	entry.Description = fmt.Sprintf("%s performed %s", strings.TrimSpace(evt.Actor), evt.Action)
	if len(evt.Payload) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(evt.Payload, &metadata); err == nil && len(metadata) > 0 {
			entry.Metadata = metadata
		}
	}
	return entry
}

func classifyAuditStatus(action string) string {
	value := strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(value, "fail"), strings.Contains(value, "error"), strings.Contains(value, "deny"):
		return "error"
	case strings.Contains(value, "warn"), strings.Contains(value, "pending"):
		return "warning"
	default:
		return "success"
	}
}

type automationScheduleResponse struct {
	Type            automation.JobType          `json:"type"`
	Enabled         bool                        `json:"enabled"`
	TenantID        string                      `json:"tenantId,omitempty"`
	Interval        string                      `json:"interval"`
	InitialDelay    string                      `json:"initialDelay,omitempty"`
	Labels          map[string]string           `json:"labels,omitempty"`
	Channels        []string                    `json:"channels,omitempty"`
	Payload         json.RawMessage             `json:"payload,omitempty"`
	PendingApproval *automationApprovalEnvelope `json:"pendingApproval,omitempty"`
	ProposedConfig  *automationScheduleChange   `json:"proposedConfig,omitempty"`
}

type automationScheduleChange struct {
	Type         automation.JobType `json:"type"`
	Enabled      bool               `json:"enabled"`
	TenantID     string             `json:"tenantId"`
	Interval     string             `json:"interval"`
	InitialDelay string             `json:"initialDelay,omitempty"`
	Labels       map[string]string  `json:"labels,omitempty"`
	Channels     []string           `json:"channels,omitempty"`
	Payload      json.RawMessage    `json:"payload,omitempty"`
}

type automationApprovalEnvelope struct {
	RequestID    string                  `json:"requestId"`
	TenantID     string                  `json:"tenantId"`
	JobType      string                  `json:"jobType"`
	RequestType  approvals.RequestType   `json:"requestType"`
	Status       approvals.RequestStatus `json:"status"`
	RequestedBy  string                  `json:"requestedBy"`
	RequestedAt  string                  `json:"requestedAt"`
	ApproverID   string                  `json:"approverId,omitempty"`
	DecidedAt    string                  `json:"decidedAt,omitempty"`
	DecisionNote string                  `json:"decisionNote,omitempty"`
	AppliedAt    string                  `json:"appliedAt,omitempty"`
}

type automationJobEnqueueResponse struct {
	JobID           string                     `json:"jobId,omitempty"`
	PendingApproval *automationJobApprovalInfo `json:"pendingApproval,omitempty"`
}

type automationJobApprovalInfo struct {
	Approval automationApprovalEnvelope `json:"approval"`
	Proposed automationJobProposal      `json:"proposed"`
}

type automationJobProposal struct {
	Type        automation.JobType `json:"type"`
	TenantID    string             `json:"tenantId"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Channels    []string           `json:"channels,omitempty"`
	Payload     json.RawMessage    `json:"payload,omitempty"`
	TriggeredBy string             `json:"triggeredBy,omitempty"`
	Source      string             `json:"source,omitempty"`
	Priority    *int               `json:"priority,omitempty"`
	NotBefore   string             `json:"notBefore,omitempty"`
}

type automationApprovalResponse struct {
	ID             string                  `json:"id"`
	TenantID       string                  `json:"tenantId"`
	JobType        string                  `json:"jobType"`
	RequestType    approvals.RequestType   `json:"requestType"`
	Status         approvals.RequestStatus `json:"status"`
	RequestedBy    string                  `json:"requestedBy"`
	RequestedAt    string                  `json:"requestedAt"`
	ApproverID     string                  `json:"approverId,omitempty"`
	DecidedAt      string                  `json:"decidedAt,omitempty"`
	DecisionNote   string                  `json:"decisionNote,omitempty"`
	AutoApplied    bool                    `json:"autoApplied"`
	AppliedAt      string                  `json:"appliedAt,omitempty"`
	Version        int                     `json:"version"`
	OriginalConfig json.RawMessage         `json:"originalConfig,omitempty"`
	ProposedConfig json.RawMessage         `json:"proposedConfig,omitempty"`
	Payload        json.RawMessage         `json:"payload,omitempty"`
}

type automationApprovalListResponse struct {
	Items  []automationApprovalResponse `json:"items"`
	Limit  int                          `json:"limit"`
	Offset int                          `json:"offset"`
	Count  int                          `json:"count"`
}

type automationApprovalDecisionRequest struct {
	Approver string `json:"approver"`
	Note     string `json:"note"`
}

type automationScheduleUpsertRequest struct {
	Enabled      bool              `json:"enabled"`
	TenantID     string            `json:"tenantId"`
	Interval     string            `json:"interval"`
	InitialDelay string            `json:"initialDelay"`
	Labels       map[string]string `json:"labels"`
	Channels     []string          `json:"channels"`
	Payload      json.RawMessage   `json:"payload"`
}

type automationJobRequest struct {
	Type        automation.JobType `json:"type"`
	TenantID    string             `json:"tenantId"`
	Labels      map[string]string  `json:"labels"`
	Payload     json.RawMessage    `json:"payload"`
	Channels    []string           `json:"channels"`
	Priority    int                `json:"priority"`
	TriggeredBy string             `json:"triggeredBy"`
	Source      string             `json:"source"`
	NotBefore   *time.Time         `json:"notBefore"`
}

type automationJobListResponse struct {
	Items  []automation.JobRun `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type workflowDefinitionRequest struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Labels      map[string]string            `json:"labels"`
	Spec        workflowsvc.Spec             `json:"spec"`
	Status      workflowsvc.DefinitionStatus `json:"status"`
}

type workflowPublishRequest struct {
	Version int `json:"version"`
}

type workflowExecutionRequest struct {
	DefinitionID string            `json:"definitionId"`
	Version      int               `json:"version"`
	TenantID     string            `json:"tenantId"`
	TriggeredBy  string            `json:"triggeredBy"`
	Context      map[string]any    `json:"context"`
	Labels       map[string]string `json:"labels"`
	Priority     int               `json:"priority"`
}

func mapAutomationSchedule(summary automation.ScheduleSummary) automationScheduleResponse {
	cfg := summary.Schedule
	resp := automationScheduleResponse{
		Type:     summary.Type,
		Enabled:  cfg.Enabled,
		TenantID: cfg.TenantID,
		Interval: cfg.Interval.String(),
		Labels:   copyStringMap(cfg.Labels),
		Channels: append([]string(nil), cfg.Channels...),
		Payload:  cloneRawMessage(cfg.Payload),
	}
	if cfg.InitialDelay > 0 {
		resp.InitialDelay = cfg.InitialDelay.String()
	}
	return resp
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *HTTPServer) automationJobStore() automation.JobStore {
	if s == nil || s.automation == nil {
		return nil
	}
	return s.automation.Store()
}

func (s *HTTPServer) needsAutomationApproval(requestType approvals.RequestType) bool {
	if s == nil {
		return false
	}
	if len(s.automationApprovalPolicy) == 0 {
		return false
	}
	_, ok := s.automationApprovalPolicy[requestType]
	return ok
}

func (s *HTTPServer) requiresAutomationApproval(requestType approvals.RequestType) bool {
	if s == nil || s.automationApprovals == nil {
		return false
	}
	return s.needsAutomationApproval(requestType)
}

func (s *HTTPServer) requireAutomationApprovals() (*approvals.Service, error) {
	if s == nil || s.automationApprovals == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "automation approvals disabled")
	}
	return s.automationApprovals, nil
}

func mapAutomationApproval(req approvals.ChangeRequest) *automationApprovalEnvelope {
	if strings.TrimSpace(req.ID) == "" {
		return nil
	}
	envelope := automationApprovalEnvelope{
		RequestID:   req.ID,
		JobType:     strings.TrimSpace(req.JobType),
		RequestType: req.RequestType,
		Status:      req.Status,
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		RequestedAt: req.RequestedAt.UTC().Format(time.RFC3339),
	}
	if approver := strings.TrimSpace(req.ApproverID); approver != "" {
		envelope.ApproverID = approver
	}
	if req.DecidedAt != nil {
		envelope.DecidedAt = req.DecidedAt.UTC().Format(time.RFC3339)
	}
	if note := strings.TrimSpace(req.DecisionNote); note != "" {
		envelope.DecisionNote = note
	}
	if req.AppliedAt != nil {
		envelope.AppliedAt = req.AppliedAt.UTC().Format(time.RFC3339)
	}
	return &envelope
}

func mapAutomationApprovalDetail(req approvals.ChangeRequest) automationApprovalResponse {
	resp := automationApprovalResponse{
		ID:          req.ID,
		JobType:     strings.TrimSpace(req.JobType),
		RequestType: req.RequestType,
		Status:      req.Status,
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		RequestedAt: req.RequestedAt.UTC().Format(time.RFC3339),
		AutoApplied: req.AutoApplied,
		Version:     req.Version,
	}
	if approver := strings.TrimSpace(req.ApproverID); approver != "" {
		resp.ApproverID = approver
	}
	if req.DecidedAt != nil {
		resp.DecidedAt = req.DecidedAt.UTC().Format(time.RFC3339)
	}
	if note := strings.TrimSpace(req.DecisionNote); note != "" {
		resp.DecisionNote = note
	}
	if req.AppliedAt != nil {
		resp.AppliedAt = req.AppliedAt.UTC().Format(time.RFC3339)
	}
	if len(req.OriginalConfig) > 0 {
		resp.OriginalConfig = cloneRawMessage(req.OriginalConfig)
	}
	if len(req.ProposedConfig) > 0 {
		resp.ProposedConfig = cloneRawMessage(req.ProposedConfig)
	}
	if len(req.Payload) > 0 {
		resp.Payload = cloneRawMessage(req.Payload)
	}
	return resp
}

func buildScheduleChange(jobType automation.JobType, cfg automation.ScheduleConfig) automationScheduleChange {
	change := automationScheduleChange{
		Type:     jobType,
		Enabled:  cfg.Enabled,
		TenantID: cfg.TenantID,
		Interval: cfg.Interval.String(),
		Labels:   copyStringMap(cfg.Labels),
		Channels: append([]string(nil), cfg.Channels...),
		Payload:  cloneRawMessage(cfg.Payload),
	}
	if cfg.InitialDelay > 0 {
		change.InitialDelay = cfg.InitialDelay.String()
	}
	return change
}

func buildScheduleChangeFromRequest(jobType automation.JobType, tenantID, intervalText, initialDelayText string, payload automationScheduleUpsertRequest) automationScheduleChange {
	change := automationScheduleChange{
		Type:     jobType,
		Enabled:  payload.Enabled,
		TenantID: tenantID,
		Interval: intervalText,
		Labels:   copyStringMap(payload.Labels),
		Channels: append([]string(nil), payload.Channels...),
		Payload:  cloneRawMessage(payload.Payload),
	}
	if strings.TrimSpace(initialDelayText) != "" {
		change.InitialDelay = initialDelayText
	}
	return change
}

func scheduleToggleOnly(current, next automation.ScheduleConfig) bool {
	if current.Enabled == next.Enabled {
		return false
	}
	if current.Interval != next.Interval || current.InitialDelay != next.InitialDelay {
		return false
	}
	if !strings.EqualFold(current.TenantID, next.TenantID) {
		return false
	}
	if !equalStringMap(current.Labels, next.Labels) {
		return false
	}
	if !equalStringSlice(current.Channels, next.Channels) {
		return false
	}
	if !bytes.Equal(current.Payload, next.Payload) {
		return false
	}
	return true
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildJobProposal(job automation.Job, req automationJobRequest) automationJobProposal {
	proposal := automationJobProposal{
		Type:        job.Type,
		TenantID:    job.TenantID,
		Labels:      copyStringMap(job.Labels),
		Channels:    append([]string(nil), req.Channels...),
		Payload:     cloneRawMessage(req.Payload),
		TriggeredBy: job.TriggeredBy,
		Source:      job.Source,
	}
	if req.Priority != 0 {
		priority := req.Priority
		proposal.Priority = &priority
	}
	if req.NotBefore != nil {
		proposal.NotBefore = req.NotBefore.UTC().Format(time.RFC3339)
	}
	return proposal
}

func automationJobTypesFromParams(params map[string][]string) []automation.JobType {
	tokens := automationQueryTokens(params, "type", "types", "jobType", "jobTypes")
	if len(tokens) == 0 {
		return nil
	}
	result := make([]automation.JobType, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, automation.JobType(token))
	}
	return result
}

func automationJobStatusesFromParams(params map[string][]string) []automation.JobStatus {
	tokens := automationQueryTokens(params, "status", "statuses")
	if len(tokens) == 0 {
		return nil
	}
	result := make([]automation.JobStatus, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, automation.JobStatus(token))
	}
	return result
}

func automationJobSourcesFromParams(params map[string][]string) []string {
	return automationQueryTokens(params, "source", "sources")
}

func automationApprovalStatusesFromParams(params map[string][]string) ([]approvals.RequestStatus, error) {
	tokens := automationQueryTokens(params, "status", "statuses", "state", "states")
	if len(tokens) == 0 {
		return nil, nil
	}
	result := make([]approvals.RequestStatus, 0, len(tokens))
	for _, token := range tokens {
		normalized := strings.ToLower(strings.TrimSpace(token))
		switch approvals.RequestStatus(normalized) {
		case approvals.RequestStatusPending, approvals.RequestStatusApproved, approvals.RequestStatusRejected, approvals.RequestStatusApplied, approvals.RequestStatusExpired:
			result = append(result, approvals.RequestStatus(normalized))
		default:
			return nil, fmt.Errorf("invalid approval status %q", token)
		}
	}
	return result, nil
}

func automationQueryTokens(params map[string][]string, keys ...string) []string {
	if len(keys) == 0 || len(params) == 0 {
		return nil
	}
	combined := make([]string, 0, len(keys))
	for _, key := range keys {
		if values, ok := params[key]; ok {
			combined = append(combined, values...)
		}
	}
	if len(combined) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(combined))
	result := make([]string, 0, len(combined))
	for _, raw := range combined {
		parts := strings.Split(raw, ",")
		if len(parts) == 0 {
			parts = []string{raw}
		}
		for _, part := range parts {
			token := strings.TrimSpace(part)
			if token == "" {
				continue
			}
			if _, exists := seen[token]; exists {
				continue
			}
			seen[token] = struct{}{}
			result = append(result, token)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneRawMessage(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	return cp
}

func normalizeWorkflowStatuses(values []string) []workflowsvc.DefinitionStatus {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[workflowsvc.DefinitionStatus]struct{}, len(values))
	statuses := make([]workflowsvc.DefinitionStatus, 0, len(values))
	for _, raw := range values {
		for _, token := range strings.Split(raw, ",") {
			status, ok := workflowStatusFromValue(token)
			if !ok {
				continue
			}
			if _, exists := seen[status]; exists {
				continue
			}
			seen[status] = struct{}{}
			statuses = append(statuses, status)
		}
	}
	if len(statuses) == 0 {
		return nil
	}
	return statuses
}

func workflowStatusFromValue(value string) (workflowsvc.DefinitionStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(workflowsvc.DefinitionStatusDraft):
		return workflowsvc.DefinitionStatusDraft, true
	case string(workflowsvc.DefinitionStatusActive):
		return workflowsvc.DefinitionStatusActive, true
	case string(workflowsvc.DefinitionStatusArchived):
		return workflowsvc.DefinitionStatusArchived, true
	default:
		return "", false
	}
}

type operationLogSnapshot struct {
	GeneratedAt time.Time           `json:"generatedAt"`
	Entries     []operationLogEntry `json:"entries"`
}

type operationLogEntry struct {
	ID          string         `json:"id"`
	Actor       string         `json:"actor"`
	Action      string         `json:"action"`
	Resource    string         `json:"resource"`
	Scope       string         `json:"scope,omitempty"`
	Status      string         `json:"status"`
	Description string         `json:"description,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type timelineResponse struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	NextCursor  string         `json:"nextCursor,omitempty"`
	Items       []timelineItem `json:"items"`
}

type timelineItem struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Severity   string            `json:"severity"`
	Summary    string            `json:"summary"`
	Source     string            `json:"source"`
	OccurredAt time.Time         `json:"occurredAt"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func (s *HTTPServer) sampleAlertSnapshot(tenantID string, limit int) monitoring.AlertFeedSnapshot {
	entries := monitoring.SampleAlertEntries(tenantID)
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	snapshot := monitoring.AlertFeedSnapshot{
		GeneratedAt: time.Now().UTC(),
		Totals:      sampleAlertTotals(entries),
	}
	if limit > 0 {
		snapshot.Alerts = make([]monitoring.AlertFeedEntry, limit)
		copy(snapshot.Alerts, entries[:limit])
	}
	return snapshot
}

func (s *HTTPServer) safeOperationSnapshot(ctx context.Context, tenantID string, limit, offset int) operationLogSnapshot {
	logger := LoggerFromContext(ctx, s.logger)
	snapshot, err := s.fetchOperationSnapshot(ctx, tenantID, limit, offset)
	if err != nil {
		if logger != nil {
			logger.Warn("operation log degraded", zap.String("tenantId", tenantID), zap.Error(err))
		}
		return s.sampleOperationSnapshot(tenantID, limit)
	}
	return snapshot
}

func (s *HTTPServer) fetchOperationSnapshot(ctx context.Context, tenantID string, limit, offset int) (operationLogSnapshot, error) {
	if limit <= 0 {
		limit = 25
	}
	if s.auditSvc == nil {
		return operationLogSnapshot{}, errors.New("audit service disabled")
	}
	events, err := s.auditSvc.ListEvents(ctx, tenantID, limit, offset)
	if err != nil {
		return operationLogSnapshot{}, err
	}
	entries := make([]operationLogEntry, 0, len(events))
	for _, evt := range events {
		entries = append(entries, s.convertAuditEvent(evt))
	}
	return operationLogSnapshot{
		GeneratedAt: time.Now().UTC(),
		Entries:     entries,
	}, nil
}

func (s *HTTPServer) sampleOperationSnapshot(tenantID string, limit int) operationLogSnapshot {
	now := time.Now().UTC()
	entries := []operationLogEntry{
		{
			ID:          "audit-sample-policy",
			Actor:       "svc-automation",
			Action:      "policy.publish",
			Resource:    "policy/global-rebalance",
			Scope:       tenantID,
			Status:      "success",
			Description: "Automation pipeline promoted global policy",
			CreatedAt:   now.Add(-3 * time.Minute),
			Metadata: map[string]any{
				"version":  "2024.06.1",
				"approver": "a.franklin",
			},
		},
		{
			ID:          "audit-sample-failover",
			Actor:       "duty-ops",
			Action:      "failover.test",
			Resource:    "cluster/dc-a1",
			Scope:       tenantID,
			Status:      "warning",
			Description: "Triggered synthetic failover to verify probes",
			CreatedAt:   now.Add(-9 * time.Minute),
			Metadata: map[string]any{
				"probe":   "dhcp4",
				"success": false,
			},
		},
		{
			ID:          "audit-sample-reclaim",
			Actor:       "audit-bot",
			Action:      "lease.reclaim",
			Resource:    "pool/hq-prod-vlan12",
			Scope:       tenantID,
			Status:      "success",
			Description: "Reclaimed expired leases from oversubscribed pool",
			CreatedAt:   now.Add(-17 * time.Minute),
			Metadata: map[string]any{
				"count":      320,
				"durationMs": 8700,
			},
		},
		{
			ID:          "audit-sample-guardrail",
			Actor:       "security-watch",
			Action:      "policy.block",
			Resource:    "guardrail/edge-relays",
			Scope:       tenantID,
			Status:      "error",
			Description: "Guardrail blocked unauthorized relay enrollment",
			CreatedAt:   now.Add(-24 * time.Minute),
			Metadata: map[string]any{
				"relayId":  "edge-relay-11",
				"reason":   "missing attestation",
				"ticketId": "INC-4813",
			},
		},
	}
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	return operationLogSnapshot{
		GeneratedAt: now,
		Entries:     entries[:limit],
	}
}

type dashboardHealthHookAdapter struct {
	hook HealthHook
}

func (h dashboardHealthHookAdapter) Name() string {
	return h.hook.Name()
}

func (h dashboardHealthHookAdapter) Check(ctx context.Context) error {
	return h.hook.Check(ctx)
}

func adaptDashboardHooks(hooks []HealthHook) []dashboard.HealthHook {
	if len(hooks) == 0 {
		return nil
	}
	adapters := make([]dashboard.HealthHook, 0, len(hooks))
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		adapters = append(adapters, dashboardHealthHookAdapter{hook: hook})
	}
	if len(adapters) == 0 {
		return nil
	}
	return adapters
}

func sampleAlertTotals(entries []monitoring.AlertFeedEntry) monitoring.AlertFeedTotals {
	var totals monitoring.AlertFeedTotals
	for _, entry := range entries {
		switch entry.Lifecycle {
		case monitoring.AlertLifecycleAcknowledged:
			totals.Acknowledged++
		case monitoring.AlertLifecycleSuppressed:
			totals.Suppressed++
		default:
			totals.Open++
		}
	}
	return totals
}

func (s *HTTPServer) timelineEntries(ctx context.Context, tenantID string, limit int) []timelineItem {
	items := make([]timelineItem, 0, limit*2)
	if s.alertFeed != nil {
		alerts := s.alertFeed.Snapshot(tenantID, limit).Alerts
		for _, alert := range alerts {
			items = append(items, timelineItemFromAlert(alert))
		}
	} else {
		for _, alert := range monitoring.SampleAlertEntries(tenantID) {
			items = append(items, timelineItemFromAlert(alert))
		}
	}
	operations := s.safeOperationSnapshot(ctx, tenantID, limit, 0)
	for _, entry := range operations.Entries {
		items = append(items, timelineItemFromOperation(entry))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func timelineItemFromAlert(entry monitoring.AlertFeedEntry) timelineItem {
	metadata := map[string]string{
		"category":  entry.Category,
		"lifecycle": string(entry.Lifecycle),
	}
	if entry.Assignee != "" {
		metadata["assignee"] = entry.Assignee
	}
	if entry.Channel != "" {
		metadata["channel"] = entry.Channel
	}
	return timelineItem{
		ID:         entry.ID,
		Type:       "alert",
		Severity:   strings.ToLower(string(entry.Severity)),
		Summary:    entry.Summary,
		Source:     entry.Source,
		OccurredAt: entry.CreatedAt,
		Metadata:   metadata,
	}
}

func timelineItemFromOperation(entry operationLogEntry) timelineItem {
	metadata := make(map[string]string, len(entry.Metadata)+2)
	for key, val := range entry.Metadata {
		metadata[key] = fmt.Sprint(val)
	}
	metadata["action"] = entry.Action
	if entry.Scope != "" {
		metadata["scope"] = entry.Scope
	}
	return timelineItem{
		ID:         entry.ID,
		Type:       "operation",
		Severity:   entry.Status,
		Summary:    entry.Description,
		Source:     entry.Resource,
		OccurredAt: entry.CreatedAt,
		Metadata:   metadata,
	}
}

func (s *HTTPServer) monitoringTenantID(c echo.Context) (string, error) {
	return systemTenantID, nil
}

func (s *HTTPServer) monitoringScopeRef(c echo.Context) (lease.ResourceScope, error) {
	scope := s.leaseScopeRef(c)
	if tenant, err := scope.TenantIDOrErr(); err == nil && tenant != "" {
		return scope, nil
	}
	return scope.WithTenantOverride(systemTenantID), nil
}

func parsePageParams(c echo.Context) (int, int, error) {
	page := 1
	size := 20
	if raw := strings.TrimSpace(c.QueryParam("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "page must be a positive integer")
		}
		page = v
	}
	if raw := strings.TrimSpace(c.QueryParam("pageSize")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "pageSize must be a positive integer")
		}
		if v > 500 {
			v = 500
		}
		size = v
	}
	return page, size, nil
}

func parseTimeRange(fromRaw, toRaw string, fallback time.Duration) (time.Time, time.Time) {
	to := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(toRaw)); err == nil {
		to = parsed
	}
	from := to.Add(-fallback)
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(fromRaw)); err == nil {
		from = parsed
	}
	return from, to
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *HTTPServer) parseLimitQuery(c echo.Context) (int, error) {
	raw := strings.TrimSpace(c.QueryParam("limit"))
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "limit must be a positive integer")
	}
	return limit, nil
}

func (s *HTTPServer) handleReportMonthlyUsage(c echo.Context) error {
	if s.reportSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	scope := s.leaseScopeRef(c)
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	report, err := s.reportSvc.MonthlyUsage(c.Request().Context(), scope, limit)
	if err != nil {
		return s.translateReportingError(err)
	}
	return c.JSON(http.StatusOK, report)
}

func (s *HTTPServer) handleReportSecurityCompliance(c echo.Context) error {
	if s.reportSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	scope := s.leaseScopeRef(c)
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	report, err := s.reportSvc.SecurityCompliance(c.Request().Context(), scope, limit)
	if err != nil {
		return s.translateReportingError(err)
	}
	return c.JSON(http.StatusOK, report)
}

func (s *HTTPServer) handleReportCapacityPlanning(c echo.Context) error {
	if s.reportSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	scope := s.leaseScopeRef(c)
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	report, err := s.reportSvc.CapacityPlanning(c.Request().Context(), scope, limit)
	if err != nil {
		return s.translateReportingError(err)
	}
	return c.JSON(http.StatusOK, report)
}

func (s *HTTPServer) handleReportAuditTrail(c echo.Context) error {
	if s.reportSvc == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reporting disabled")
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	resource := strings.TrimSpace(c.QueryParam("resource"))
	correlation := strings.TrimSpace(c.QueryParam("correlationId"))
	scope := s.leaseScopeRef(c)
	if strings.TrimSpace(scope.TenantOrDefault()) == "" {
		scope = scope.WithTenantOverride(systemTenantID)
	}
	report, err := s.reportSvc.AuditTrail(c.Request().Context(), scope, resource, correlation, limit)
	if err != nil {
		return s.translateReportingError(err)
	}
	return c.JSON(http.StatusOK, report)
}

func (s *HTTPServer) translateReportingError(err error) error {
	if errors.Is(err, reporting.ErrReportingDisabled) {
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	}
	if errors.Is(err, reporting.ErrTenantScopeRequired) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

type leaseActionRequest struct {
	Reason string `json:"reason"`
}

type simulateRelayContext struct {
	GIAddr     string            `json:"giAddr"`
	LinkAddr   string            `json:"linkAddr"`
	PeerAddr   string            `json:"peerAddr"`
	VLANID     *int              `json:"vlanId"`
	Attributes map[string]string `json:"attributes"`
}
