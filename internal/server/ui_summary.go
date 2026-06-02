package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"modern-dhcp/internal/auth"
	"modern-dhcp/internal/pool"
)

type systemAuthProviderResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type systemTenantQuotaSummary struct {
	TenantID           string `json:"tenantId"`
	TenantName         string `json:"tenantName"`
	PoolsUsed          int    `json:"poolsUsed"`
	PoolLimit          int    `json:"poolLimit"`
	LeasesUsed         int    `json:"leasesUsed"`
	LeaseLimit         int    `json:"leaseLimit"`
	StaticBindingsUsed int    `json:"staticBindingsUsed"`
	StaticBindingLimit int    `json:"staticBindingLimit"`
}

type systemManagementSummaryResponse struct {
	AuthProviders []systemAuthProviderResponse `json:"authProviders"`
	Tenants       []systemTenantQuotaSummary   `json:"tenants"`
	Roles         []rbacRoleResponse           `json:"roles"`
	APIKeys       []apiKeyResponse             `json:"apiKeys"`
}

type cachedUISystemSummary struct {
	payload   []byte
	expiresAt time.Time
}

var (
	uiSystemSummaryTTL   = 10 * time.Second
	uiSystemSummaryCache sync.Map
	uiSystemSummaryGroup singleflight.Group
)

func (s *HTTPServer) handleUISystemSummary() echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		cacheKey := strings.Join([]string{
			strings.TrimSpace(GetTenantID(ctx)),
			strings.TrimSpace(GetUserID(ctx)),
			strings.TrimSpace(s.principalFromContext(c)),
		}, "|")
		if raw, ok := uiSystemSummaryCache.Load(cacheKey); ok {
			if cached, ok := raw.(cachedUISystemSummary); ok && time.Now().Before(cached.expiresAt) {
				return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, cached.payload)
			}
		}
		result, err, _ := uiSystemSummaryGroup.Do(cacheKey, func() (any, error) {
			summary := systemManagementSummaryResponse{
				AuthProviders: []systemAuthProviderResponse{},
				Tenants:       []systemTenantQuotaSummary{},
				Roles:         []rbacRoleResponse{},
				APIKeys:       []apiKeyResponse{},
			}

			if err := s.populateAuthProviders(ctx, &summary); err != nil {
				return nil, err
			}
			if err := s.populateRoles(ctx, &summary); err != nil {
				return nil, err
			}
			s.populateTenantSummaries(ctx, c, &summary)

			payload, err := json.Marshal(summary)
			if err != nil {
				return nil, err
			}
			uiSystemSummaryCache.Store(cacheKey, cachedUISystemSummary{payload: payload, expiresAt: time.Now().Add(uiSystemSummaryTTL)})
			return payload, nil
		})
		if err != nil {
			return err
		}
		payload, _ := result.([]byte)
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, payload)
	}
}

func (s *HTTPServer) populateAuthProviders(ctx context.Context, summary *systemManagementSummaryResponse) error {
	if summary == nil {
		return nil
	}
	summary.AuthProviders = summary.AuthProviders[:0]
	summary.APIKeys = summary.APIKeys[:0]
	if s.authService == nil {
		return nil
	}

	providers, err := s.authService.ListIdentityProviders(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, provider := range providers {
		summary.AuthProviders = append(summary.AuthProviders, systemAuthProviderResponse{
			ID:     provider.ID,
			Name:   provider.Name,
			Type:   normalizeAuthProviderType(provider.Type),
			Status: authProviderStatus(provider.Enabled),
		})
	}
	sort.Slice(summary.AuthProviders, func(i, j int) bool {
		return strings.ToLower(summary.AuthProviders[i].Name) < strings.ToLower(summary.AuthProviders[j].Name)
	})

	filter := auth.APIKeyFilter{
		IncludeRevoked: false,
		Kinds:          []string{auth.KeyKindService},
	}
	keys, err := s.authService.ListAPIKeys(ctx, filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	now := time.Now().UTC()
	for _, key := range keys {
		if !key.Valid(now) {
			continue
		}
		summary.APIKeys = append(summary.APIKeys, newAPIKeyResponse(key))
	}
	sort.Slice(summary.APIKeys, func(i, j int) bool {
		if summary.APIKeys[i].CreatedAt.Equal(summary.APIKeys[j].CreatedAt) {
			return summary.APIKeys[i].ID < summary.APIKeys[j].ID
		}
		return summary.APIKeys[i].CreatedAt.After(summary.APIKeys[j].CreatedAt)
	})
	return nil
}

func (s *HTTPServer) populateRoles(ctx context.Context, summary *systemManagementSummaryResponse) error {
	if summary == nil {
		return nil
	}
	summary.Roles = summary.Roles[:0]
	if s.rbacRepo == nil {
		return nil
	}
	roles, err := s.rbacRepo.ListRoles(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, role := range roles {
		summary.Roles = append(summary.Roles, newRBACRoleResponse(role))
	}
	sort.Slice(summary.Roles, func(i, j int) bool {
		return strings.ToLower(summary.Roles[i].Name) < strings.ToLower(summary.Roles[j].Name)
	})
	return nil
}

func (s *HTTPServer) populateTenantSummaries(ctx context.Context, c echo.Context, summary *systemManagementSummaryResponse) {
	if summary == nil {
		return
	}
	logger := LoggerFromContext(ctx, s.logger)
	summary.Tenants = summary.Tenants[:0]
	principal := s.principalFromContext(c)
	tenantIDs := s.resolvePrincipalTenants(ctx, principal)
	if len(tenantIDs) == 0 {
		tenantIDs = []string{"default"}
	}
	poolScope := s.poolScopeRef(c)
	leaseScope := s.leaseScopeRef(c)
	for _, tenantID := range tenantIDs {
		id := strings.TrimSpace(tenantID)
		if id == "" {
			continue
		}
		entry := systemTenantQuotaSummary{
			TenantID:   id,
			TenantName: id,
		}
		poolRef := poolScope.WithTenantOverride(id)
		leaseRef := leaseScope.WithTenantOverride(id)
		if s.poolSvc != nil {
			if pools, err := listAllPools(ctx, s.poolSvc, poolRef, 200); err == nil {
				entry.PoolsUsed = len(pools)
			} else if logger != nil {
				logger.Warn("ui summary: pool count failed", zap.String("tenantId", id), zap.Error(err))
			}
		}
		if s.leaseSvc != nil {
			if leaseCount, err := s.leaseSvc.CountActiveLeases(ctx, leaseRef); err == nil {
				entry.LeasesUsed = leaseCount
			} else if logger != nil {
				logger.Warn("ui summary: lease count failed", zap.String("tenantId", id), zap.Error(err))
			}
		}
		if s.poolSvc != nil {
			if bindingCount, err := countStaticBindings(ctx, s.poolSvc, poolRef, 200); err == nil {
				entry.StaticBindingsUsed = bindingCount
			} else if logger != nil {
				logger.Warn("ui summary: static binding count failed", zap.String("tenantId", id), zap.Error(err))
			}
		}
		if s.tenantQuota != nil {
			if quota, err := s.tenantQuota.GetQuota(ctx, id); err == nil {
				entry.PoolLimit = quota.PoolLimit
				entry.LeaseLimit = quota.LeaseLimit
				entry.StaticBindingLimit = quota.ClientLimit
			} else if logger != nil {
				logger.Warn("ui summary: quota lookup failed", zap.String("tenantId", id), zap.Error(err))
			}
		}
		summary.Tenants = append(summary.Tenants, entry)
	}
	sort.Slice(summary.Tenants, func(i, j int) bool {
		return strings.ToLower(summary.Tenants[i].TenantID) < strings.ToLower(summary.Tenants[j].TenantID)
	})
}

func normalizeAuthProviderType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "local", "password":
		return "local"
	case "ldap":
		return "ldap"
	case "sso", "oidc", "oauth", "oauth2", "saml":
		return "sso"
	default:
		return value
	}
}

func authProviderStatus(enabled bool) string {
	if enabled {
		return "active"
	}
	return "disabled"
}

func countStaticBindings(ctx context.Context, svc *pool.Service, scopeRef pool.ResourceScope, pageSize int) (int, error) {
	if svc == nil {
		return 0, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	total := 0
	offset := 0
	for {
		batch, err := svc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, pageSize, offset)
		if err != nil {
			return 0, err
		}
		total += len(batch)
		if len(batch) < pageSize {
			break
		}
		offset += len(batch)
		if offset >= 5000 {
			break
		}
	}
	return total, nil
}
