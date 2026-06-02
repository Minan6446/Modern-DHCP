package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/auth"
)

const (
	contextActorKey            = "apiActor"
	contextRoleKey             = "apiRole"
	contextCredentialKey       = "apiCredential"
	contextRateKey             = "apiRateKey"
	contextPrincipalKey        = "apiPrincipal"
	contextCapabilitiesKey     = "apiCapabilities"
	contextCapabilityStrictKey = "apiCapabilityStrict"
	contextResolutionKey       = "tenantResolution"
	contextAccessScopeKey      = "apiAccessScope"
	contextPrincipalContextKey = "apiPrincipalContext"
)

// Authenticator validates API keys and/or JWT bearer tokens.
type Authenticator struct {
	requireAuth  bool
	tokens       map[string]APIKeyMetadata
	validator    *JWTValidator
	provider     auth.TokenProvider
	mu           sync.RWMutex
	dynamicCache map[string]dynamicCacheEntry
	dynamicTTL   time.Duration
}

type dynamicCacheEntry struct {
	meta     APIKeyMetadata
	loadedAt time.Time
}

// NewAuthenticator builds a composite auth handler.
func NewAuthenticator(requireAuth bool, tokens map[string]APIKeyMetadata, validator *JWTValidator, provider auth.TokenProvider) *Authenticator {
	safe := make(map[string]APIKeyMetadata, len(tokens))
	for k, v := range tokens {
		safe[k] = v
	}
	return &Authenticator{
		requireAuth:  requireAuth,
		tokens:       safe,
		validator:    validator,
		provider:     provider,
		dynamicCache: make(map[string]dynamicCacheEntry),
		dynamicTTL:   time.Minute,
	}
}

// Middleware enforces configured authentication providers.
func (a *Authenticator) Middleware() echo.MiddlewareFunc {
	if a == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isPublicAuthEndpoint(c.Request().Method, c.Request().URL.Path) {
				return next(c)
			}
			ctx, ok := a.authenticate(c)
			if !ok {
				if a.requireAuth {
					return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
				}
				return next(c)
			}
			applyAuthContext(c, ctx)
			return next(c)
		}
	}
}

func isPublicAuthEndpoint(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	cleanPath := strings.TrimRight(strings.TrimSpace(path), "/")
	if cleanPath == "" {
		cleanPath = "/"
	}
	for _, prefix := range []string{"/api/v1", "/api/v2"} {
		if strings.HasPrefix(cleanPath, prefix+"/") {
			cleanPath = strings.TrimPrefix(cleanPath, prefix)
			break
		}
	}
	if method == http.MethodGet && cleanPath == "/auth/captcha" {
		return true
	}
	if method != http.MethodPost {
		return false
	}
	switch cleanPath {
	case "/auth/login",
		"/auth/session",
		"/auth/session/refresh",
		"/auth/api-keys/exchange",
		"/auth/password/reset/start",
		"/auth/password/reset/verify",
		"/auth/password/reset/complete":
		return true
	default:
		return false
	}
}

func (a *Authenticator) authenticate(c echo.Context) (authContext, bool) {
	if apiCtx, ok := a.authenticateAPIKey(c.Request().Context(), c.Request()); ok {
		return apiCtx, true
	}
	if jwtCtx, ok := a.authenticateJWT(c.Request()); ok {
		return jwtCtx, true
	}
	return authContext{}, false
}

func (a *Authenticator) authenticateAPIKey(ctx context.Context, r *http.Request) (authContext, bool) {
	if len(a.tokens) == 0 && a.provider == nil {
		return authContext{}, false
	}
	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if key == "" {
		if token := bearerToken(r.Header.Get("Authorization")); token != "" {
			key = token
		}
	}
	if key == "" {
		if cookie, err := r.Cookie("auth_token"); err == nil {
			key = strings.TrimSpace(cookie.Value)
		}
	}
	if key == "" {
		if token := strings.TrimSpace(r.URL.Query().Get("access_token")); token != "" {
			key = token
		} else if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
			key = token
		}
	}
	if key == "" {
		if raw := strings.TrimSpace(r.Header.Get("Sec-Websocket-Protocol")); raw != "" {
			for _, candidate := range strings.Split(raw, ",") {
				value := strings.TrimSpace(candidate)
				if value == "" {
					continue
				}
				if strings.HasPrefix(strings.ToLower(value), "bearer ") {
					key = strings.TrimSpace(value[7:])
					break
				}
				if key == "" {
					key = value
				}
			}
		}
	}
	if key == "" {
		return authContext{}, false
	}
	if meta, ok := a.tokens[key]; ok {
		return contextFromMetadata(key, meta), true
	}
	if meta, ok := a.lookupDynamicCache(key); ok {
		return contextFromMetadata(key, meta), true
	}
	if a.provider != nil {
		tokenMeta, err := a.provider.LookupToken(ctx, key)
		if err == nil {
			meta := APIKeyMetadata{
				DisplayName: tokenMeta.DisplayName,
				Role:        tokenMeta.Role,
				PrincipalID: tokenMeta.PrincipalID,
			}
			a.storeDynamicCache(key, meta)
			return contextFromMetadata(key, meta), true
		}
	}
	return authContext{}, false
}

func (a *Authenticator) authenticateJWT(r *http.Request) (authContext, bool) {
	if a.validator == nil {
		return authContext{}, false
	}
	bearer := bearerToken(r.Header.Get("Authorization"))
	if bearer == "" {
		return authContext{}, false
	}
	identity, _, err := a.validator.Validate(r.Context(), bearer)
	if err != nil {
		return authContext{}, false
	}
	return authContext{
		Actor:       identity.Actor,
		Role:        identity.Role,
		Credential:  identity.Subject,
		RateKey:     identity.Subject,
		PrincipalID: identity.Subject,
		Scheme:      "oauth2",
	}, true
}

type authContext struct {
	Actor       string
	Role        string
	Credential  string
	RateKey     string
	PrincipalID string
	Scheme      string
}

func applyAuthContext(c echo.Context, ctx authContext) {
	if ctx.Actor != "" {
		c.Set(contextActorKey, ctx.Actor)
	}
	if ctx.Role != "" {
		c.Set(contextRoleKey, ctx.Role)
	}
	if ctx.Credential != "" {
		c.Set(contextCredentialKey, ctx.Credential)
	}
	if ctx.RateKey != "" {
		c.Set(contextRateKey, ctx.RateKey)
	}
	if ctx.PrincipalID != "" {
		c.Set(contextPrincipalKey, ctx.PrincipalID)
		setRequestContextValue(c, contextKeyUserID, ctx.PrincipalID)
	}
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func (a *Authenticator) lookupDynamicCache(token string) (APIKeyMetadata, bool) {
	if token == "" || a.dynamicCache == nil {
		return APIKeyMetadata{}, false
	}
	a.mu.RLock()
	entry, ok := a.dynamicCache[token]
	ttl := a.dynamicTTL
	a.mu.RUnlock()
	if !ok {
		return APIKeyMetadata{}, false
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	if time.Since(entry.loadedAt) > ttl {
		a.mu.Lock()
		delete(a.dynamicCache, token)
		a.mu.Unlock()
		return APIKeyMetadata{}, false
	}
	return entry.meta, true
}

func (a *Authenticator) storeDynamicCache(token string, meta APIKeyMetadata) {
	if token == "" || a.dynamicCache == nil {
		return
	}
	a.mu.Lock()
	a.dynamicCache[token] = dynamicCacheEntry{meta: meta, loadedAt: time.Now().UTC()}
	a.mu.Unlock()
}

func contextFromMetadata(token string, meta APIKeyMetadata) authContext {
	actor := meta.DisplayName
	if actor == "" {
		actor = token
	}
	principal := strings.TrimSpace(meta.PrincipalID)
	if principal == "" {
		principal = token
	}
	return authContext{
		Actor:       actor,
		Role:        normalizeRole(meta.Role),
		Credential:  token,
		RateKey:     token,
		PrincipalID: principal,
		Scheme:      "apiKey",
	}
}

// ResolveAPIKey returns metadata for the provided API key or an error if invalid.
func (a *Authenticator) ResolveAPIKey(ctx context.Context, token string) (APIKeyMetadata, error) {
	if a == nil {
		return APIKeyMetadata{}, auth.ErrAPIKeyNotFound
	}
	key := strings.TrimSpace(token)
	if key == "" {
		return APIKeyMetadata{}, auth.ErrAPIKeyNotFound
	}
	if meta, ok := a.tokens[key]; ok {
		return meta, nil
	}
	if meta, ok := a.lookupDynamicCache(key); ok {
		return meta, nil
	}
	if a.provider == nil {
		return APIKeyMetadata{}, auth.ErrAPIKeyNotFound
	}
	tokenMeta, err := a.provider.LookupToken(ctx, key)
	if err != nil {
		return APIKeyMetadata{}, err
	}
	meta := APIKeyMetadata{
		DisplayName: tokenMeta.DisplayName,
		Role:        tokenMeta.Role,
		PrincipalID: tokenMeta.PrincipalID,
	}
	a.storeDynamicCache(key, meta)
	return meta, nil
}
