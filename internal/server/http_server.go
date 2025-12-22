package server

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/automation"
	workflowsvc "modern-dhcp/internal/automation/workflow"
	"modern-dhcp/internal/collab"
	"modern-dhcp/internal/dashboard"
	"modern-dhcp/internal/ha"
	iotstore "modern-dhcp/internal/iot"
	iotregistry "modern-dhcp/internal/iot/registry"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/ops"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	"modern-dhcp/internal/rbac"
	"modern-dhcp/internal/reporting"
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

type apiKeyResponse struct {
	ID           string     `json:"id"`
	DisplayName  string     `json:"displayName"`
	Role         string     `json:"role"`
	Capabilities []string   `json:"capabilities"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	Token        string     `json:"token,omitempty"`
}

type apiKeyCreateRequest struct {
	DisplayName  string   `json:"displayName"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	TenantScope  string   `json:"tenantScope"`
	Description  string   `json:"description"`
	ExpiresAt    string   `json:"expiresAt"`
	OwnerUserID  string   `json:"ownerUserId"`
}

type sessionResponse struct {
	Token              string          `json:"token"`
	RefreshToken       string          `json:"refreshToken"`
	ExpiresAt          string          `json:"expiresAt"`
	Actor              sessionActor    `json:"actor"`
	Tenants            []tenantSummary `json:"tenants"`
	ActiveTenantID     string          `json:"activeTenantId,omitempty"`
	AuthMethod         string          `json:"authMethod,omitempty"`
	MustChangePassword bool            `json:"mustChangePassword,omitempty"`
}

type sessionActor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

type tenantSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	IsDefault bool   `json:"isDefault"`
}

type sessionContextResponse struct {
	TenantID  string `json:"tenantId"`
	AppliedAt string `json:"appliedAt"`
}

type tenantSessionView struct {
	TenantID     string             `json:"tenantId"`
	Actor        sessionActor       `json:"actor"`
	Capabilities []string           `json:"capabilities"`
	Tenants      []tenantSummary    `json:"tenants"`
	Quota        models.TenantQuota `json:"quota"`
	Active       bool               `json:"active"`
}

type credentialLoginResponse struct {
	Token              string `json:"token"`
	ExpiresAt          string `json:"expiresAt"`
	PrincipalID        string `json:"principalId"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	TenantID           string `json:"tenantId"`
	MustChangePassword bool   `json:"mustChangePassword,omitempty"`
}

type sessionSnapshotPayload struct {
	Actor          string   `json:"actor"`
	PrincipalID    string   `json:"principalId"`
	Role           string   `json:"role"`
	Capabilities   []string `json:"capabilities"`
	Tenants        []string `json:"tenants"`
	ActiveTenantID string   `json:"activeTenantId,omitempty"`
}

func newAPIKeyResponse(key auth.APIKey) apiKeyResponse {
	resp := apiKeyResponse{
		ID:           key.ID,
		DisplayName:  key.Name,
		Role:         normalizeRole(key.Role),
		Capabilities: apiKeyCapabilitiesFromMetadata(key),
		CreatedAt:    key.CreatedAt.UTC(),
	}
	if key.LastUsedAt.Valid {
		ts := key.LastUsedAt.Time.UTC()
		resp.LastUsedAt = &ts
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

func (s *HTTPServer) handleCredentialLogin() echo.HandlerFunc {
	type request struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TenantID string `json:"tenantId"`
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
		user, err := s.authService.Authenticate(ctx, username, password)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			tenantID = strings.TrimSpace(user.TenantID)
		}
		if tenantID == "" {
			tenantID = "default"
		}
		session, err := s.authService.IssueSession(ctx, user, auth.SessionOptions{TenantID: tenantID})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := newCredentialLoginResponse(user, session, tenantID)
		s.rememberTenantContext(user.PrincipalID(), tenantID)
		s.recordAudit(ctx, tenantID, user.Username, "auth.login", map[string]any{
			"principalId": resp.PrincipalID,
			"tenantId":    tenantID,
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, resp)
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
		s.rememberTenantContext(meta.PrincipalID, tenantID)
		s.recordAudit(ctx, tenantID, meta.DisplayName, "auth.api_key.exchange", map[string]any{
			"principalId": meta.PrincipalID,
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
		ctx := c.Request().Context()
		tenants := s.resolvePrincipalTenants(ctx, principal)
		snapshot := sessionSnapshotPayload{
			Actor:        actor,
			PrincipalID:  principal,
			Role:         role,
			Capabilities: caps,
			Tenants:      tenants,
		}
		if active := s.lookupTenantContext(principal); active != "" {
			snapshot.ActiveTenantID = active
		}
		return c.JSON(http.StatusOK, snapshot)
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
		s.recordAudit(ctx, user.TenantID, s.actorFromContext(c), "auth.password.change", map[string]any{
			"principalId": user.PrincipalID(),
		}, withResource("auth_user"))
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
			tenantID = strings.TrimSpace(user.TenantID)
		}
		if tenantID == "" {
			tenantID = "default"
		}
		sessionToken, err := s.authService.IssueSession(ctx, user, auth.SessionOptions{TenantID: tenantID, AuthMethod: result.Method})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		resp := s.newSessionResponse(ctx, user, tenantID, sessionToken, result.Method)
		s.rememberTenantContext(user.PrincipalID(), tenantID)
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
			return echo.NewHTTPError(http.StatusBadRequest, "refreshToken is required")
		}
		ctx := c.Request().Context()
		sessionToken, user, tenantID, authMethod, err := s.authService.RefreshSession(ctx, refreshToken)
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeyNotFound) || errors.Is(err, auth.ErrAPIKeyRevoked) {
				return echo.NewHTTPError(http.StatusUnauthorized, "refresh token invalid")
			}
			if errors.Is(err, auth.ErrUserNotFound) {
				return echo.NewHTTPError(http.StatusUnauthorized, "principal not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if strings.TrimSpace(tenantID) == "" {
			tenantID = strings.TrimSpace(user.TenantID)
		}
		if tenantID == "" {
			if remembered := s.lookupTenantContext(user.PrincipalID()); remembered != "" {
				tenantID = remembered
			}
		}
		if tenantID == "" {
			tenantID = "default"
		}
		resp := s.newSessionResponse(ctx, user, tenantID, sessionToken, authMethod)
		s.rememberTenantContext(user.PrincipalID(), tenantID)
		s.recordAudit(ctx, tenantID, user.Username, "auth.session.refresh", map[string]any{
			"principalId": user.PrincipalID(),
			"authMethod":  authMethod,
		}, withResource("auth_session"))
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleSessionTenantUpdate() echo.HandlerFunc {
	type request struct {
		TenantID string `json:"tenantId"`
	}
	return func(c echo.Context) error {
		principal := s.principalFromContext(c)
		if principal == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "principal not resolved")
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(payload.TenantID)
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		ctx := c.Request().Context()
		allowed := s.resolvePrincipalTenants(ctx, principal)
		if !containsTenant(allowed, tenantID) {
			return echo.NewHTTPError(http.StatusForbidden, "tenant not permitted")
		}
		s.rememberTenantContext(principal, tenantID)
		s.recordAudit(ctx, tenantID, s.actorFromContext(c), "auth.session.tenant", map[string]any{
			"tenantId": tenantID,
		}, withResource("auth_session"))
		resp := sessionContextResponse{
			TenantID:  tenantID,
			AppliedAt: time.Now().UTC().Format(time.RFC3339),
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleSessionTenantView() echo.HandlerFunc {
	return func(c echo.Context) error {
		principal := s.principalFromContext(c)
		if principal == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "principal not resolved")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		ctx := c.Request().Context()
		memberships := s.resolvePrincipalTenants(ctx, principal)
		if len(memberships) > 0 && !containsTenant(memberships, tenantID) {
			return echo.NewHTTPError(http.StatusForbidden, "tenant not permitted")
		}
		quota := models.TenantQuota{TenantID: tenantID}
		if s.tenantQuota != nil {
			if q, err := s.tenantQuota.GetQuota(ctx, tenantID); err == nil {
				quota = q
			} else if !errors.Is(err, sql.ErrNoRows) {
				if s.logger != nil {
					s.logger.Warn("tenant quota lookup failed", zap.String("tenantId", tenantID), zap.Error(err))
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to load tenant quota")
			}
		}
		actor := sessionActor{
			ID:           principal,
			Name:         s.actorFromContext(c),
			Role:         s.roleFromContext(c),
			Capabilities: capabilitySliceFromContext(c),
		}
		resp := tenantSessionView{
			TenantID:     tenantID,
			Actor:        actor,
			Capabilities: capabilitySliceFromContext(c),
			Tenants:      buildTenantSummaries(memberships, actor.Role),
			Quota:        quota,
			Active:       strings.EqualFold(tenantID, s.requestTenantID(c)),
		}
		if len(resp.Tenants) == 0 {
			resp.Tenants = buildTenantSummaries([]string{tenantID}, actor.Role)
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleListTenants() echo.HandlerFunc {
	return func(c echo.Context) error {
		principal := s.principalFromContext(c)
		if principal == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "principal not resolved")
		}
		ctx := c.Request().Context()
		tenants := s.resolvePrincipalTenants(ctx, principal)
		summaries := buildTenantSummaries(tenants, s.roleFromContext(c))
		return c.JSON(http.StatusOK, summaries)
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
	return ""
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
		} else if err != nil && !errors.Is(err, auth.ErrAPIKeyNotFound) {
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
			filter.OwnerUserID = &owner
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
		tenantScope := strings.TrimSpace(payload.TenantScope)
		if tenantScope != "" && s.rbacRepo != nil {
			allowed := s.resolvePrincipalTenants(c.Request().Context(), principal)
			if len(allowed) > 0 && !containsTenant(allowed, tenantScope) {
				return echo.NewHTTPError(http.StatusForbidden, "tenant scope not accessible")
			}
		}
		var expires *time.Time
		if trimmed := strings.TrimSpace(payload.ExpiresAt); trimmed != "" {
			parsed, err := time.Parse(time.RFC3339, trimmed)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be RFC3339")
			}
			expires = &parsed
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
		var ownerPtr *string
		if owner := strings.TrimSpace(payload.OwnerUserID); owner != "" {
			captured := owner
			ownerPtr = &captured
		} else if derived := userIDFromPrincipal(principal); derived != "" {
			captured := derived
			ownerPtr = &captured
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
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"auth.api_key.create",
			map[string]any{"keyId": key.ID, "role": key.Role, "tenantScope": tenantScope},
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
		return c.NoContent(http.StatusNoContent)
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

func (s *HTTPServer) mountTenantRoutes(apiGroup *echo.Group, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, iotSvc *iotregistry.Service) {
	_ = policyEngine

	tenantGroup := apiGroup.Group("/tenants/:tenantId")
	if s.options.RequireAuth {
		tenantGroup.Use(RequireRole(RoleReader))
	}
	tenantAdminGroup := tenantGroup.Group("")
	if s.options.RequireAuth {
		tenantAdminGroup.Use(RequireRole(RoleAdmin))
	}

	tenantGroup.GET("/policies", func(c echo.Context) error {
		tenantID := c.Param("tenantId")
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

	tenantAdminGroup.POST("/policies", func(c echo.Context) error {
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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

	tenantAdminGroup.PUT("/policies/:policyId", func(c echo.Context) error {
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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

	tenantAdminGroup.DELETE("/policies/:policyId", func(c echo.Context) error {
		if policySvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "policy service disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantGroup.GET("/security/policies", s.handleSecurityPolicyList(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyRead))
		tenantGroup.GET("/security/policies/:ruleId", s.handleSecurityPolicyGet(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyRead))
		tenantAdminGroup.POST("/security/policies", s.handleSecurityPolicyCreate(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
		tenantAdminGroup.PUT("/security/policies/:ruleId", s.handleSecurityPolicyUpdate(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
		tenantAdminGroup.DELETE("/security/policies/:ruleId", s.handleSecurityPolicyDelete(securityPolicySvc), RequireCapability(CapabilitySecurityPolicyManage))
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

	tenantGroup.GET("/pools", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		if page.Limit == 0 {
			page.Limit = 100
		}
		pools, err := poolSvc.ListPools(c.Request().Context(), tenantID, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, pools)
	}, RequireCapability(CapabilityPoolRead))

	tenantGroup.GET("/pools/:poolId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		poolID := strings.TrimSpace(c.Param("poolId"))
		if tenantID == "" || poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and poolId are required")
		}
		poolObj, err := poolSvc.GetPool(c.Request().Context(), tenantID, poolID)
		if err != nil {
			if errors.Is(err, pool.ErrPoolNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, poolObj)
	}, RequireCapability(CapabilityPoolRead))

	tenantAdminGroup.POST("/pools", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload poolPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		if strings.TrimSpace(payload.Name) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		ctx := c.Request().Context()
		poolObj, err := poolSvc.CreatePool(ctx, buildPoolRequest(tenantID, payload))
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		return c.JSON(http.StatusCreated, poolObj)
	}, RequireCapability(CapabilityPoolWrite))

	tenantAdminGroup.PATCH("/pools/:poolId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload poolPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		poolID := strings.TrimSpace(c.Param("poolId"))
		if tenantID == "" || poolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and poolId are required")
		}
		ctx := c.Request().Context()
		updated, err := poolSvc.UpdatePool(ctx, pool.PoolUpdateRequest{
			PoolID:            poolID,
			PoolCreateRequest: buildPoolRequest(tenantID, payload),
		})
		if err != nil {
			return s.translatePoolMutationError(c, err)
		}
		return c.JSON(http.StatusOK, updated)
	}, RequireCapability(CapabilityPoolWrite))

	tenantGroup.POST("/pools/find", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		pools, err := poolSvc.FindPools(c.Request().Context(), tenantID, filter)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, pools)
	}, RequireCapability(CapabilityPoolRead))

	tenantGroup.GET("/bindings", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		if page.Limit == 0 {
			page.Limit = 100
		}
		bindings, err := poolSvc.ListBindings(c.Request().Context(), tenantID, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, bindings)
	}, RequireCapability(CapabilityBindingRead))

	tenantAdminGroup.POST("/bindings", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload bindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		req := makeBindingRequest(tenantID, payload)
		if req.Identifier == "" || req.PoolID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "identifier and poolId are required")
		}
		ctx := c.Request().Context()
		binding, err := poolSvc.CreateBinding(ctx, req)
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

	tenantAdminGroup.PATCH("/bindings/:bindingId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		var payload bindingPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if tenantID == "" || bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and bindingId are required")
		}
		ctx := c.Request().Context()
		binding, err := poolSvc.UpdateBinding(ctx, bindingID, makeBindingRequest(tenantID, payload))
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

	tenantAdminGroup.DELETE("/bindings/:bindingId", func(c echo.Context) error {
		if poolSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "pool service unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		bindingID := strings.TrimSpace(c.Param("bindingId"))
		if tenantID == "" || bindingID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and bindingId are required")
		}
		ctx := c.Request().Context()
		actor := s.actorFromContext(c)
		if err := poolSvc.DeleteBinding(ctx, tenantID, bindingID); err != nil {
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

	tenantAdminGroup.GET("/quotas", s.handleTenantQuotaGet(), RequireCapability(CapabilityTenantQuotaRead))
	tenantAdminGroup.PUT("/quotas", s.handleTenantQuotaUpdate(), RequireCapability(CapabilityTenantQuotaWrite))

	tenantAdminGroup.GET("/rbac/assignments", s.handleListAssignments(), RequireCapability(CapabilityRBACAssignmentRead))
	tenantAdminGroup.POST("/rbac/assignments", s.handleCreateAssignment(), RequireCapability(CapabilityRBACAssignmentWrite))
	tenantAdminGroup.DELETE("/rbac/assignments/:assignmentId", s.handleDeleteAssignment(), RequireCapability(CapabilityRBACAssignmentWrite))

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

	iotGroup := tenantAdminGroup.Group("/iot")

	iotGroup.GET("/devices", func(c echo.Context) error {
		if s.iotRegistry == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "iot registry disabled")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if tenantID == "" || deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and deviceId are required")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if tenantID == "" || deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and deviceId are required")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if tenantID == "" || deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and deviceId are required")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		deviceID := strings.TrimSpace(c.Param("deviceId"))
		if tenantID == "" || deviceID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and deviceId are required")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		profileID := strings.TrimSpace(c.Param("profileId"))
		if tenantID == "" || profileID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and profileId are required")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		profileID := strings.TrimSpace(c.Param("profileId"))
		if tenantID == "" || profileID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId and profileId are required")
		}
		if err := s.iotRegistry.DeleteProfile(c.Request().Context(), tenantID, profileID); err != nil {
			if errors.Is(err, iotstore.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "profile not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}, RequireCapability(CapabilityIoTRegistryManage))

	tenantGroup.GET("/leases", func(c echo.Context) error {
		tenantID := c.Param("tenantId")
		state := c.QueryParam("state")
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		leases, err := leaseSvc.ListLeases(c.Request().Context(), tenantID, state, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, leases)
	}, RequireCapability(CapabilityLeaseRead))

	tenantGroup.GET("/leases/history", s.handleLeaseHistoryList(leaseSvc), RequireCapability(CapabilityLeaseRead))

	tenantAdminGroup.POST("/leases/:leaseId/release", func(c echo.Context) error {
		return s.handleLeaseAction(c, leaseSvc, "release")
	}, RequireCapability(CapabilityLeaseManage))

	tenantAdminGroup.POST("/leases/:leaseId/decline", func(c echo.Context) error {
		return s.handleLeaseAction(c, leaseSvc, "decline")
	}, RequireCapability(CapabilityLeaseManage))

	tenantAdminGroup.POST("/leases/:leaseId/cooldown/clear", func(c echo.Context) error {
		tenantID := c.Param("tenantId")
		leaseID := c.Param("leaseId")
		ctx := c.Request().Context()
		leaseObj, changed, err := leaseSvc.ClearCooldown(ctx, tenantID, leaseID)
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
		envelope := s.buildOperationResponse(ctx, tenantID, "lease.cooldown.clear", status, leaseObj, changed, steps)
		return c.JSON(http.StatusOK, envelope)
	}, RequireCapability(CapabilityLeaseManage))

	tenantAdminGroup.POST("/leases/:leaseId/security-state", func(c echo.Context) error {
		tenantID := c.Param("tenantId")
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
		leaseObj, previous, changed, err := leaseSvc.UpdateLeaseSecurityState(ctx, tenantID, leaseID, stateReq.State)
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
		envelope := s.buildOperationResponse(ctx, tenantID, "lease.security_state", status, leaseObj, changed, steps)
		return c.JSON(http.StatusOK, envelope)
	}, RequireCapability(CapabilityLeaseManage))

	tenantAdminGroup.GET("/prefix-leases", func(c echo.Context) error {
		tenantID := c.Param("tenantId")
		state := c.QueryParam("state")
		page, err := parsePagination(c)
		if err != nil {
			return err
		}
		leases, err := leaseSvc.ListPrefixLeases(c.Request().Context(), tenantID, state, page.Limit, page.Offset)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, leases)
	}, RequireCapability(CapabilityLeaseRead))

	tenantAdminGroup.POST("/prefix-leases/:prefixLeaseId/release", func(c echo.Context) error {
		return s.handlePrefixLeaseAction(c, leaseSvc, "release")
	}, RequireCapability(CapabilityLeaseManage))

	tenantAdminGroup.POST("/prefix-leases/:prefixLeaseId/decline", func(c echo.Context) error {
		return s.handlePrefixLeaseAction(c, leaseSvc, "decline")
	}, RequireCapability(CapabilityLeaseManage))

	reportGroup := tenantAdminGroup.Group("/reports", RequireCapability(CapabilityReportRead))
	reportGroup.GET("/monthly-usage", s.handleReportMonthlyUsage)
	reportGroup.GET("/security", s.handleReportSecurityCompliance)
	reportGroup.GET("/capacity", s.handleReportCapacityPlanning)
	reportGroup.GET("/audit-trail", s.handleReportAuditTrail)

	tenantAdminGroup.POST("/leases/history/export", s.handleLeaseHistoryExport(), RequireCapabilities(CapabilityLeaseRead, CapabilityReportRead))

	tenantAdminGroup.GET("/audit", func(c echo.Context) error {
		if s.auditSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "audit service unavailable")
		}
		tenantID := c.Param("tenantId")
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
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
		principalID := strings.TrimSpace(c.QueryParam("principalId"))
		if principalID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "principalId is required")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		assignments, err := s.rbacRepo.ListAssignmentsByTenant(c.Request().Context(), principalID, tenantID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, assignments)
	}
}

func (s *HTTPServer) handleCreateAssignment() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.rbacRepo == nil {
			return echo.NewHTTPError(http.StatusNotImplemented, "rbac repository unavailable")
		}
		tenantID := strings.TrimSpace(c.Param("tenantId"))
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
		}
		var payload struct {
			PrincipalID  string          `json:"principalId"`
			RoleName     string          `json:"roleName"`
			OrgUnitID    *string         `json:"orgUnitId"`
			ResourceType *string         `json:"resourceType"`
			ResourceID   *string         `json:"resourceId"`
			ExpiresAt    *string         `json:"expiresAt"`
			Attributes   json.RawMessage `json:"attributes"`
			Global       bool            `json:"global"`
		}
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		principalID := strings.TrimSpace(payload.PrincipalID)
		roleName := strings.TrimSpace(payload.RoleName)
		if principalID == "" || roleName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "principalId and roleName are required")
		}
		var expiresAt *time.Time
		if payload.ExpiresAt != nil {
			value := strings.TrimSpace(*payload.ExpiresAt)
			if value != "" {
				ts, err := time.Parse(time.RFC3339, value)
				if err != nil {
					return echo.NewHTTPError(http.StatusBadRequest, "expiresAt must be RFC3339 timestamp")
				}
				expiresAt = &ts
			}
		}
		var tenantScope *string
		if !payload.Global {
			tenantScope = &tenantID
		}
		assignment := &rbac.Assignment{
			ID:           uuid.NewString(),
			PrincipalID:  principalID,
			RoleName:     roleName,
			TenantID:     tenantScope,
			OrgUnitID:    payload.OrgUnitID,
			ResourceType: payload.ResourceType,
			ResourceID:   payload.ResourceID,
			CreatedBy:    s.actorFromContext(c),
			CreatedAt:    time.Now().UTC(),
			Attributes:   payload.Attributes,
			ExpiresAt:    expiresAt,
		}
		if err := s.rbacRepo.CreateAssignment(c.Request().Context(), assignment); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		actor := assignment.CreatedBy
		scope := "global"
		if assignment.TenantID != nil && *assignment.TenantID != "" {
			scope = *assignment.TenantID
		}
		change := auditpayload.ConfigChange{
			Resource:   "rbac_assignment",
			Action:     "create",
			Identifier: assignment.ID,
			After: map[string]any{
				"principalId": principalID,
				"roleName":    roleName,
				"tenantScope": scope,
			},
			Fields: []string{"principalId", "roleName", "tenantScope"},
		}
		s.recordAudit(c.Request().Context(), tenantID, actor, "rbac.assignment.create", change, withResource("rbac_assignment"))
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
		tenantID := strings.TrimSpace(c.Param("tenantId"))
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
		s.recordAudit(ctx, tenantID, actor, "rbac.assignment.delete", change, withResource("rbac_assignment"))
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
		data, err := openAPIFS.ReadFile(fileName)
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
	entries, err := openAPIFS.ReadDir(".")
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
	tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
	if tenantID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
	}
	snapshot, err := s.visualization.Topology(c.Request().Context(), tenantID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleVisualizationHeatmap(c echo.Context) error {
	if s.visualization == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "visualization disabled")
	}
	tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
	if tenantID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
	}
	snapshot, err := s.visualization.LeaseHeatmap(c.Request().Context(), tenantID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

// HTTPServer wraps the Echo engine.
type HTTPServer struct {
	echo               *echo.Echo
	logger             *zap.Logger
	options            Options
	automation         *automation.Service
	workflowSvc        *workflowsvc.Service
	opsSvc             *ops.Service
	auditSvc           *audit.Service
	authService        *auth.Service
	leaseSvc           *lease.Service
	poolSvc            *pool.Service
	securityPolicy     *securitypolicy.Service
	monitor            *monitoring.Aggregator
	alertFeed          *monitoring.AlertFeed
	alertManager       *alerting.Manager
	alertController    *monitoring.AlertController
	dashboardSvc       *dashboard.Service
	reportSvc          *reporting.Service
	visualization      *visualization.Service
	haSvc              *ha.Service
	haOnce             sync.Once
	collabHub          *collab.Hub
	iotRegistry        *iotregistry.Service
	auth               *Authenticator
	rateLimit          *rateLimiter
	quota              *quotaEnforcer
	rbacResolver       *rbac.Resolver
	tenantQuota        *tenant.Service
	rbacRepo           rbac.Repository
	jwt                *JWTValidator
	apiVersions        []string
	openAPISpecs       map[string]string
	superAdmin         *superadmin.Manager
	superAdminUsername string
	superAdminAPIKey   string
	tenantCtxMu        sync.RWMutex
	tenantContexts     map[string]string
	optionStore        *dhcpOptionStore
	bootTime           time.Time
}

// NewHTTPServer configures the Echo runtime and registers all API routes.
func NewHTTPServer(logger *zap.Logger, opts Options, leaseSvc *lease.Service, policyEngine *policy.Engine, policySvc *policy.Service, securityPolicySvc *securitypolicy.Service, poolSvc *pool.Service, auditSvc *audit.Service, reportSvc *reporting.Service, vizSvc *visualization.Service, iotSvc *iotregistry.Service, collabHub *collab.Hub, quotaSvc *tenant.Service, rbacRepo rbac.Repository, authSvc *auth.Service) (*HTTPServer, error) {
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
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	if opts.CORS.Enabled {
		corsCfg := middleware.CORSConfig{
			AllowOrigins:     opts.CORS.AllowedOrigins,
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-API-Key"},
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
		echo:               e,
		logger:             logger,
		options:            opts,
		automation:         opts.Automation,
		workflowSvc:        opts.Workflow,
		opsSvc:             ops.NewService(opts.OpsSupport, opts.OpsSettingsStore, logger),
		auditSvc:           auditSvc,
		authService:        authSvc,
		leaseSvc:           leaseSvc,
		poolSvc:            poolSvc,
		securityPolicy:     securityPolicySvc,
		monitor:            opts.Monitoring.Aggregator,
		alertFeed:          opts.Monitoring.AlertFeed,
		alertManager:       opts.Monitoring.AlertManager,
		alertController:    opts.Monitoring.AlertController,
		dashboardSvc:       dashboardSvc,
		reportSvc:          reportSvc,
		visualization:      vizSvc,
		haSvc:              haService,
		iotRegistry:        iotSvc,
		collabHub:          collabHub,
		auth:               NewAuthenticator(opts.RequireAuth, opts.APIKeys, jwtValidator, tokenProvider),
		rateLimit:          newRateLimiter(opts.API.RateLimit),
		quota:              newQuotaEnforcer(opts.API.Quota),
		rbacResolver:       opts.RBAC.Resolver,
		tenantQuota:        quotaSvc,
		rbacRepo:           rbacRepo,
		jwt:                jwtValidator,
		apiVersions:        sanitizeAPIVersions(opts.API.Versions),
		openAPISpecs:       discoverOpenAPISpecs(),
		superAdmin:         opts.SuperAdmin.Manager,
		superAdminUsername: opts.SuperAdmin.Username,
		superAdminAPIKey:   opts.SuperAdmin.APIKey,
		tenantContexts:     make(map[string]string),
		optionStore:        newDHCPOptionStore(),
		bootTime:           time.Now().UTC(),
	}
	if server.openAPISpecs == nil {
		server.openAPISpecs = make(map[string]string)
	}
	if server.superAdmin != nil && server.superAdminUsername == "" {
		server.superAdminUsername = superadmin.DefaultUsername
	}
	e.Use(server.enrichRequestContext)
	if auditSvc != nil {
		e.Use(server.auditAccessMiddleware)
	}
	e.Use(server.instrumentationMiddleware)
	server.registerRoutes(leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)
	server.registerHARoutes()
	return server, nil
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
		versionRoot.GET("/healthz", s.handleHealthz())
		versionRoot.GET("/versions", s.handleAPIVersions())
		versionRoot.POST("/auth/login", s.handleCredentialLogin())
		versionRoot.POST("/auth/api-keys/exchange", s.handleAPIKeyExchange())
		versionRoot.POST("/auth/session", s.handleSessionCreate())
		versionRoot.POST("/auth/session/refresh", s.handleSessionRefresh())

		secured := versionRoot.Group("")
		s.applyAPIMiddleware(secured)
		secured.GET("/ui", s.handleUIMetadata())
		secured.GET("/tenants", s.handleListTenants())
		secured.PUT("/session/tenant", s.handleSessionTenantUpdate())
		secured.GET("/session/tenants/:tenantId", s.handleSessionTenantView())

		s.mountTenantRoutes(secured, leaseSvc, policyEngine, policySvc, securityPolicySvc, poolSvc, iotSvc)

		core := secured.Group("/core")
		if s.options.RequireAuth {
			core.Use(RequireRole(RoleReader))
		}
		s.mountCoreOpsRoutes(core)

		authGroup := secured.Group("/auth")
		if s.options.RequireAuth {
			authGroup.Use(RequireRole(RoleReader))
		}
		authGroup.GET("/api-keys", s.handleListAPIKeys())
		if s.options.RequireAuth {
			authGroup.POST("/api-keys", s.handleCreateAPIKey(), RequireRole(RoleAdmin))
			authGroup.DELETE("/api-keys/:keyId", s.handleDeleteAPIKey(), RequireRole(RoleAdmin))
		} else {
			authGroup.POST("/api-keys", s.handleCreateAPIKey())
			authGroup.DELETE("/api-keys/:keyId", s.handleDeleteAPIKey())
		}
		authGroup.GET("/session", s.handleSessionSnapshot())
		authGroup.POST("/logout", s.handleLogout())
		authGroup.DELETE("/session", s.handleLogout())
		authGroup.POST("/password", s.handlePasswordChange())

		monitoring := secured.Group("/monitoring")
		if s.options.RequireAuth {
			monitoring.Use(RequireRole(RoleReader))
		}
		monitoring.GET("/overview", s.handleMonitoringOverview)
		monitoring.GET("/pools", s.handleMonitoringPools)
		monitoring.GET("/requests", s.handleMonitoringRequests)
		monitoring.GET("/health", s.handleMonitoringHealth)
		monitoring.GET("/security", s.handleMonitoringSecurity)
		monitoring.GET("/system-health", s.handleMonitoringSystemHealth)
		monitoring.GET("/events/timeline", s.handleMonitoringTimeline)
		monitoring.GET("/operations", s.handleMonitoringOperations)
		monitoring.GET("/alerts", s.handleAlertFeed)
		monitoring.GET("/alerts/feed", s.handleAlertFeed)
		monitoring.GET("/alerts/rules", s.handleAlertRules)
		monitoring.GET("/analytics", s.handleMonitoringAnalytics)
		monitoring.GET("/integrations/cmdb", s.handleMonitoringCMDBSync)
		if s.options.RequireAuth {
			monitoring.POST("/alerts/:alertId/ack", s.handleAlertAcknowledge, RequireRole(RoleAdmin))
			monitoring.POST("/alerts/:alertId/suppress", s.handleAlertSuppress, RequireRole(RoleAdmin))
		} else {
			monitoring.POST("/alerts/:alertId/ack", s.handleAlertAcknowledge)
			monitoring.POST("/alerts/:alertId/suppress", s.handleAlertSuppress)
		}
		monitoring.GET("/status/stream", s.handleStatusStream())

		dashboard := secured.Group("/dashboard")
		if s.options.RequireAuth {
			dashboard.Use(RequireRole(RoleReader))
		}
		dashboard.GET("/health", s.handleDashboardHealth)
		dashboard.GET("/kpis", s.handleDashboardKPIs)
		dashboard.GET("/streams", s.handleDashboardStreams)
		dashboard.GET("/insights", s.handleDashboardInsights)
		dashboard.GET("/insights/overview", s.handleInsightsOverview)

		automation := secured.Group("/automation")
		if s.options.RequireAuth {
			automation.Use(RequireRole(RoleAdmin))
		}
		automation.GET("/snapshot", s.handleAutomationSnapshot())
		automation.GET("/schedules", s.handleAutomationSchedules())
		automation.GET("/jobs", s.handleAutomationListJobs())
		automation.GET("/jobs/:jobId", s.handleAutomationGetJob())
		automation.POST("/jobs", s.handleAutomationEnqueue())

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

		opsSupport := secured.Group("/ops")
		if s.options.RequireAuth {
			opsSupport.Use(RequireRole(RoleAdmin))
		}
		opsSupport.GET("/system", s.handleOpsSystemSummary())
		opsSupport.PUT("/system", s.handleOpsUpdateSystem())
		opsSupport.PATCH("/settings/theme", s.handleOpsUpdateTheme())
		opsSupport.PATCH("/settings/locale", s.handleOpsUpdateLocale())
		opsSupport.PATCH("/settings/maintenance", s.handleOpsUpdateMaintenance())
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
		reports.POST("/lease-daily", s.handleLeaseDailySchedule(), RequireCapability(CapabilityReportRead))
	}
}

type ctxKey string

const (
	correlationContextKey ctxKey = "auditCorrelationId"
)

// Start runs the HTTP server.
func (s *HTTPServer) Start(port int) error {
	return s.echo.StartServer(&http.Server{Addr: ":" + strconv.Itoa(port), Handler: s.echo})
}

// Shutdown gracefully stops the server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

func (s *HTTPServer) actorFromContext(c echo.Context) string {
	if actor, ok := c.Get(contextActorKey).(string); ok && actor != "" {
		return actor
	}
	return "anonymous"
}

func (s *HTTPServer) roleFromContext(c echo.Context) string {
	if role, ok := c.Get(contextRoleKey).(string); ok && role != "" {
		return role
	}
	return RoleReader
}

func (s *HTTPServer) principalFromContext(c echo.Context) string {
	if principal, ok := c.Get(contextPrincipalKey).(string); ok && principal != "" {
		return principal
	}
	if credential, ok := c.Get(contextCredentialKey).(string); ok && credential != "" {
		return credential
	}
	return ""
}

func (s *HTTPServer) resolvePrincipalTenants(ctx context.Context, principal string) []string {
	if principal == "" || s.rbacRepo == nil {
		return []string{"default"}
	}
	assignments, err := s.rbacRepo.ListAssignments(ctx, principal)
	if err != nil {
		s.logger.Debug("rbac: list assignments failed", zap.String("principalId", principal), zap.Error(err))
		return []string{"default"}
	}
	set := make(map[string]struct{})
	for _, assignment := range assignments {
		if assignment.TenantID != nil && *assignment.TenantID != "" {
			set[*assignment.TenantID] = struct{}{}
		}
	}
	if len(set) == 0 {
		return []string{"default"}
	}
	result := make([]string, 0, len(set))
	for tenantID := range set {
		result = append(result, tenantID)
	}
	slices.Sort(result)
	return result
}

func (s *HTTPServer) auditTenantFromContext(c echo.Context) string {
	if tenantID := s.requestTenantID(c); tenantID != "" {
		return tenantID
	}
	return "system"
}

func (s *HTTPServer) requestTenantID(c echo.Context) string {
	if tenantID := strings.TrimSpace(c.Param("tenantId")); tenantID != "" {
		return tenantID
	}
	if tenantID := strings.TrimSpace(c.QueryParam("tenantId")); tenantID != "" {
		return tenantID
	}
	if tenantID := strings.TrimSpace(c.Request().Header.Get("X-Tenant-ID")); tenantID != "" {
		s.rememberTenantContext(s.principalFromContext(c), tenantID)
		return tenantID
	}
	if tenantID := s.lookupTenantContext(s.principalFromContext(c)); strings.TrimSpace(tenantID) != "" {
		return strings.TrimSpace(tenantID)
	}
	return ""
}

func (s *HTTPServer) recordAudit(ctx context.Context, tenantID, actor, action string, payload any, opts ...auditOption) {
	if s.auditSvc == nil {
		return
	}
	var raw json.RawMessage
	if payload != nil {
		if data, err := json.Marshal(payload); err != nil {
			s.logger.Warn("failed to marshal audit payload", zap.Error(err))
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
	if err := s.auditSvc.RecordEvent(ctx, req); err != nil {
		s.logger.Warn("failed to record audit event", zap.Error(err))
	}
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

func (s *HTTPServer) enrichRequestContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		correlation := requestIDFromEcho(c)
		if correlation != "" {
			ctx := context.WithValue(req.Context(), correlationContextKey, correlation)
			c.SetRequest(req.WithContext(ctx))
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
		tenantID := s.auditTenantFromContext(c)
		resource := resourceFromPath(path)
		s.recordAudit(reqCtx, tenantID, actor, "admin.activity", payload, withSource("api.http"), withResource(resource))
		return err
	}
}

func (s *HTTPServer) instrumentationMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := next(c)
		if s.options.Metrics != nil {
			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}
			s.options.Metrics.HTTPRequests.WithLabelValues(c.Path(), c.Request().Method, strconv.Itoa(status)).Inc()
		}
		return err
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
			principal := s.principalFromContext(c)
			if principal == "" {
				return next(c)
			}
			tenantID := s.requestTenantID(c)
			resolution, err := s.rbacResolver.Resolve(c.Request().Context(), principal, rbac.ResolveOptions{TenantID: tenantID})
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("rbac resolution failed", zap.String("principalId", principal), zap.String("tenantId", tenantID), zap.Error(err))
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

func (s *HTTPServer) newSessionResponse(ctx context.Context, user auth.User, tenantID string, session auth.SessionToken, authMethod string) sessionResponse {
	role := normalizeRole(user.Role)
	actorName := strings.TrimSpace(user.DisplayName)
	if actorName == "" {
		actorName = user.Username
	}
	principal := user.PrincipalID()
	tenants := s.resolvePrincipalTenants(ctx, principal)
	if tenantID != "" && !containsTenant(tenants, tenantID) {
		tenants = append(tenants, tenantID)
		slices.Sort(tenants)
	}
	if len(tenants) == 0 {
		tenants = []string{tenantID}
	}
	expires := session.ExpiresAt.UTC().Format(time.RFC3339)
	if strings.TrimSpace(tenantID) == "" && len(tenants) > 0 {
		tenantID = tenants[0]
	}
	return sessionResponse{
		Token:        session.Token,
		RefreshToken: session.Token,
		ExpiresAt:    expires,
		AuthMethod:   authMethod,
		Actor: sessionActor{
			ID:           principal,
			Name:         actorName,
			Role:         role,
			Capabilities: capabilitiesForRole(role),
		},
		Tenants:            buildTenantSummaries(tenants, role),
		ActiveTenantID:     tenantID,
		MustChangePassword: user.MustChangePassword,
	}
}

func (s *HTTPServer) sessionResponseFromLegacy(resp credentialLoginResponse) sessionResponse {
	role := normalizeRole(resp.Role)
	actorName := strings.TrimSpace(resp.DisplayName)
	if actorName == "" {
		actorName = resp.PrincipalID
	}
	tenants := []string{strings.TrimSpace(resp.TenantID)}
	if tenants[0] == "" {
		tenants[0] = "default"
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
		Tenants:            buildTenantSummaries(tenants, role),
		ActiveTenantID:     tenants[0],
		MustChangePassword: resp.MustChangePassword,
	}
}

func buildTenantSummaries(tenants []string, role string) []tenantSummary {
	role = normalizeRole(role)
	summaries := make([]tenantSummary, 0, len(tenants))
	for _, tenantID := range tenants {
		id := strings.TrimSpace(tenantID)
		if id == "" {
			continue
		}
		summaries = append(summaries, tenantSummary{
			ID:        id,
			Name:      id,
			Role:      role,
			IsDefault: id == "default",
		})
	}
	if len(summaries) == 0 {
		summaries = append(summaries, tenantSummary{ID: "default", Name: "default", Role: role, IsDefault: true})
	}
	return summaries
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

func (s *HTTPServer) rememberTenantContext(principal, tenantID string) {
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return
	}
	tenantID = strings.TrimSpace(tenantID)
	s.tenantCtxMu.Lock()
	defer s.tenantCtxMu.Unlock()
	if tenantID == "" {
		if s.tenantContexts != nil {
			delete(s.tenantContexts, principal)
		}
		return
	}
	if s.tenantContexts == nil {
		s.tenantContexts = make(map[string]string)
	}
	s.tenantContexts[principal] = tenantID
}

func (s *HTTPServer) lookupTenantContext(principal string) string {
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return ""
	}
	s.tenantCtxMu.RLock()
	defer s.tenantCtxMu.RUnlock()
	if s.tenantContexts == nil {
		return ""
	}
	return s.tenantContexts[principal]
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
			CapabilitySecurityPolicyRead,
			CapabilitySecurityPolicyManage,
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
			CapabilitySecurityPolicyRead,
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
	if id := strings.TrimSpace(c.Response().Header().Get(echo.HeaderXRequestID)); id != "" {
		return id
	}
	if id := strings.TrimSpace(c.Request().Header.Get(echo.HeaderXRequestID)); id != "" {
		return id
	}
	if id, ok := c.Get(echo.HeaderXRequestID).(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return ""
}

func correlationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(correlationContextKey).(string); ok {
		return value
	}
	return ""
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
	tenantID := c.Param("tenantId")
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
		leaseObj, changed, err = leaseSvc.ReleaseLease(ctx, tenantID, leaseID)
	case "decline":
		leaseObj, changed, err = leaseSvc.DeclineLease(ctx, tenantID, leaseID)
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
	envelope := s.buildOperationResponse(ctx, tenantID, "lease."+action, status, leaseObj, changed, steps)
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
	tenantID := c.Param("tenantId")
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
		leaseObj, changed, err = leaseSvc.ReleasePrefixByID(ctx, tenantID, leaseID)
	case "decline":
		leaseObj, changed, err = leaseSvc.DeclinePrefixByID(ctx, tenantID, leaseID)
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
	envelope := s.buildOperationResponse(ctx, tenantID, "prefix."+action, status, leaseObj, changed, steps)
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
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	overview, err := s.monitor.Overview(ctx, tenantID, limit)
	if err != nil {
		s.logger.Error("monitoring overview failed", zap.String("tenantId", tenantID), zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build monitoring overview")
	}
	return c.JSON(http.StatusOK, overview)
}

func (s *HTTPServer) handleMonitoringPools(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	pools, err := s.monitor.Pools(ctx, tenantID, limit)
	if err != nil {
		s.logger.Error("monitoring pools failed", zap.String("tenantId", tenantID), zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build pool utilization snapshot")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"pools":       pools,
	})
}

func (s *HTTPServer) handleMonitoringRequests(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"requests":    s.monitor.Requests(tenantID),
	})
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
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	snapshot := s.monitor.Security(tenantID)
	return c.JSON(http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC(),
		"security":    snapshot,
	})
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
		if s.logger != nil {
			s.logger.Warn("dashboard health degraded", zap.Error(healthErr))
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
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
	}
	snapshot, kpiErr := svc.KPIs(c.Request().Context(), tenantID, limit)
	if kpiErr != nil {
		if s.logger != nil {
			s.logger.Warn("dashboard kpi degraded", zap.String("tenantId", tenantID), zap.Error(kpiErr))
		}
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard kpis")
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleDashboardStreams(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	limit, err := s.parseLimitQuery(c)
	if err != nil {
		return err
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
			return echo.NewHTTPError(http.StatusBadRequest, "since must be RFC3339 timestamp")
		}
		since = parsed
	}
	snapshot, streamErr := svc.Streams(c.Request().Context(), dashboard.StreamOptions{
		TenantID:          tenantID,
		Limit:             limit,
		IncludeAlerts:     includeAlerts,
		IncludeOperations: includeOps,
		Since:             since,
	})
	if streamErr != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to build dashboard streams")
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleDashboardInsights(c echo.Context) error {
	svc, err := s.requireDashboardService()
	if err != nil {
		return err
	}
	tenantID, err := s.monitoringTenantID(c)
	if err != nil {
		return err
	}
	snapshot, insightErr := svc.Insights(c.Request().Context(), tenantID)
	if insightErr != nil && s.logger != nil {
		s.logger.Warn("dashboard insights degraded", zap.String("tenantId", tenantID), zap.Error(insightErr))
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (s *HTTPServer) handleInsightsOverview(c echo.Context) error {
	return s.handleDashboardInsights(c)
}

func (s *HTTPServer) handleAutomationSnapshot() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		snapshot := s.automation.Snapshot()
		return c.JSON(http.StatusOK, snapshot)
	}
}

func (s *HTTPServer) handleAutomationSchedules() echo.HandlerFunc {
	return func(c echo.Context) error {
		if s.automation == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "automation disabled")
		}
		summaries := s.automation.Schedules()
		response := make([]automationScheduleResponse, 0, len(summaries))
		for _, summary := range summaries {
			response = append(response, mapAutomationSchedule(summary))
		}
		return c.JSON(http.StatusOK, response)
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
			tenantID = s.requestTenantID(c)
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
		tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantID == "" {
			tenantID = s.requestTenantID(c)
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
		var req automationJobRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if strings.TrimSpace(string(req.Type)) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "type is required")
		}
		tenantID := strings.TrimSpace(req.TenantID)
		if tenantID == "" {
			tenantID = s.requestTenantID(c)
		}
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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
		if err := s.automation.Enqueue(c.Request().Context(), job); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusAccepted, map[string]string{"id": job.ID})
	}
}

func (s *HTTPServer) requireWorkflowService() (*workflowsvc.Service, error) {
	if s.workflowSvc == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "workflow service unavailable")
	}
	return s.workflowSvc, nil
}

func (s *HTTPServer) handleWorkflowListDefinitions() echo.HandlerFunc {
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
		return c.JSON(http.StatusOK, defs)
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
			tenantID = s.requestTenantID(c)
		}
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
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

func (s *HTTPServer) handleAlertRules(c echo.Context) error {
	if s.alertController == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "alerting disabled")
	}
	rules := s.alertController.Rules()
	if rules == nil {
		rules = []monitoring.AlertRuleDescriptor{}
	}
	return c.JSON(http.StatusOK, rules)
}

func (s *HTTPServer) handleMonitoringAnalytics(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
	tenantID, err := s.monitoringTenantID(c)
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
	snapshot, err := s.monitor.Analytics(c.Request().Context(), tenantID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
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
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleAlertSuppress(c echo.Context) error {
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
		Channel    string `json:"channel"`
		TTLSeconds int64  `json:"ttlSeconds"`
	}
	var payload request
	if bindErr := c.Bind(&payload); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	entry, updateErr := s.alertFeed.Suppress(tenantID, alertID, payload.Channel)
	if updateErr != nil {
		switch {
		case errors.Is(updateErr, monitoring.ErrAlertNotFound):
			return echo.NewHTTPError(http.StatusNotFound, updateErr.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, updateErr.Error())
		}
	}
	s.applyAlertMute(entry.Fingerprint, payload.TTLSeconds, true)
	return c.JSON(http.StatusOK, entry)
}

func (s *HTTPServer) handleMonitoringCMDBSync(c echo.Context) error {
	if s.monitor == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "monitoring disabled")
	}
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
	ctx := c.Request().Context()
	hotspots, err := s.monitor.Pools(ctx, tenantID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	analytics, err := s.monitor.Analytics(ctx, tenantID, limit)
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

func (s *HTTPServer) handleOpsUpdateSystem() echo.HandlerFunc {
	type request struct {
		Theme             *string `json:"theme"`
		Locale            *string `json:"locale"`
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
			Theme:             payload.Theme,
			Locale:            payload.Locale,
			MaintenanceMode:   payload.MaintenanceMode,
			MaintenanceWindow: payload.MaintenanceWindow,
			Announcement:      payload.Announcement,
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
		s.recordAudit(
			c.Request().Context(),
			s.auditTenantFromContext(c),
			s.actorFromContext(c),
			"ops.transfer.job.start",
			transferJobAuditPayload(job),
			withResource("ops_transfer_job"),
		)
		return c.JSON(http.StatusAccepted, job)
	}
}

func (s *HTTPServer) handleOpsListTransferJobs() echo.HandlerFunc {
	type response struct {
		Items []ops.TransferJob `json:"items"`
		Total int               `json:"total"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 20
		}
		jobs, listErr := s.opsSvc.ListTransferJobs(c.Request().Context(), limit)
		if listErr != nil {
			switch {
			case errors.Is(listErr, ops.ErrImportExportDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "import/export disabled")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
			}
		}
		return c.JSON(http.StatusOK, response{Items: jobs, Total: len(jobs)})
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
	s.recordAudit(
		ctx,
		s.auditTenantFromContext(c),
		actor,
		action,
		settingsAuditPayload(before, after),
		withResource("ops_system"),
	)
	return c.JSON(http.StatusOK, summary)
}

func emptySettingsUpdate(update ops.SystemSettingsUpdate) bool {
	return update.Theme == nil && update.Locale == nil && update.MaintenanceMode == nil && update.MaintenanceWindow == nil && update.Announcement == nil
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
		GeneratedAt time.Time         `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		articles := s.opsSvc.HelpArticles(c.Request().Context())
		return c.JSON(http.StatusOK, response{Items: articles, Total: len(articles), GeneratedAt: time.Now().UTC()})
	}
}

func (s *HTTPServer) handleOpsHelpFAQ() echo.HandlerFunc {
	type response struct {
		Items       []ops.HelpFAQEntry `json:"items"`
		Total       int                `json:"total"`
		GeneratedAt time.Time          `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		entries := s.opsSvc.FAQEntries(c.Request().Context())
		return c.JSON(http.StatusOK, response{Items: entries, Total: len(entries), GeneratedAt: time.Now().UTC()})
	}
}

func (s *HTTPServer) handleOpsHelpReleases() echo.HandlerFunc {
	type response struct {
		Items       []ops.ReleaseNote `json:"items"`
		Total       int               `json:"total"`
		GeneratedAt time.Time         `json:"generatedAt"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		releases := s.opsSvc.ReleaseNotes(c.Request().Context())
		return c.JSON(http.StatusOK, response{Items: releases, Total: len(releases), GeneratedAt: time.Now().UTC()})
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
		return c.JSON(http.StatusAccepted, run)
	}
}

func (s *HTTPServer) handleOpsListScriptRuns() echo.HandlerFunc {
	type response struct {
		Items []ops.ScriptRun `json:"items"`
		Total int             `json:"total"`
	}
	return func(c echo.Context) error {
		if s.opsSvc == nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "ops support unavailable")
		}
		limit, err := s.parseLimitQuery(c)
		if err != nil {
			return err
		}
		if limit == 0 {
			limit = 20
		}
		runs, listErr := s.opsSvc.ListScriptRuns(c.Request().Context(), limit)
		if listErr != nil {
			switch {
			case errors.Is(listErr, ops.ErrScriptsDisabled):
				return echo.NewHTTPError(http.StatusServiceUnavailable, "script runner disabled")
			default:
				return echo.NewHTTPError(http.StatusInternalServerError, listErr.Error())
			}
		}
		return c.JSON(http.StatusOK, response{Items: runs, Total: len(runs)})
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
		tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantID == "" {
			tenantID = s.requestTenantID(c)
		}
		snapshot := diagnosticsSnapshot{
			GeneratedAt: time.Now().UTC(),
			TenantID:    tenantID,
			System:      s.buildSystemHealthResponse(ctx),
		}
		if s.monitor == nil {
			snapshot.Notes = append(snapshot.Notes, "monitoring disabled: analytics unavailable")
		} else if tenantID == "" {
			snapshot.Notes = append(snapshot.Notes, "tenantId not provided: analytics skipped")
		} else if analytics, analyticsErr := s.monitor.Analytics(ctx, tenantID, limit); analyticsErr == nil {
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
		tenantID := strings.TrimSpace(c.QueryParam("tenantId"))
		if tenantID == "" {
			tenantID = s.requestTenantID(c)
		}
		ctx, cancel := context.WithCancel(c.Request().Context())
		defer cancel()
		conn.SetCloseHandler(func(code int, text string) error {
			cancel()
			return nil
		})
		if payload := s.buildStatusPayload(ctx, tenantID); payload != nil {
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
				if payload := s.buildStatusPayload(ctx, tenantID); payload != nil {
					if err := conn.WriteJSON(payload); err != nil {
						return nil
					}
				}
			}
		}
	}
}

func (s *HTTPServer) buildStatusPayload(ctx context.Context, tenantID string) *statusMessage {
	if s.monitor == nil {
		return nil
	}
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
		if overview, err := s.monitor.Overview(ctx, tenantID, 10); err == nil {
			message.Overview = &overview
		} else if s.logger != nil {
			s.logger.Debug("status stream overview failed", zap.String("tenantId", tenantID), zap.Error(err))
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
	Type         automation.JobType `json:"type"`
	Enabled      bool               `json:"enabled"`
	TenantID     string             `json:"tenantId,omitempty"`
	Interval     string             `json:"interval"`
	InitialDelay string             `json:"initialDelay,omitempty"`
	Labels       map[string]string  `json:"labels,omitempty"`
	Channels     []string           `json:"channels,omitempty"`
	Payload      json.RawMessage    `json:"payload,omitempty"`
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
	snapshot, err := s.fetchOperationSnapshot(ctx, tenantID, limit, offset)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("operation log degraded", zap.String("tenantId", tenantID), zap.Error(err))
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
	tenantID := s.requestTenantID(c)
	if tenantID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "tenantId is required")
	}
	return tenantID, nil
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
	report, err := s.reportSvc.MonthlyUsage(c.Request().Context(), c.Param("tenantId"), limit)
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
	report, err := s.reportSvc.SecurityCompliance(c.Request().Context(), c.Param("tenantId"), limit)
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
	report, err := s.reportSvc.CapacityPlanning(c.Request().Context(), c.Param("tenantId"), limit)
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
	report, err := s.reportSvc.AuditTrail(c.Request().Context(), c.Param("tenantId"), resource, correlation, limit)
	if err != nil {
		return s.translateReportingError(err)
	}
	return c.JSON(http.StatusOK, report)
}

func (s *HTTPServer) translateReportingError(err error) error {
	if errors.Is(err, reporting.ErrReportingDisabled) {
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
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
