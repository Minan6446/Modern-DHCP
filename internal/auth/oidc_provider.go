package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// OIDCProviderOptions configures the OIDC/OAuth2 JWT provider.
// Tokens are validated via JWKS (RS256) or optional HMAC secret.
type OIDCProviderOptions struct {
	Issuer           string
	Audience         string
	JWKSURL          string
	JWKSCacheTTL     time.Duration
	HMACSecret       string
	RequiredScopes   []string
	ScopeRoles       map[string]string
	DefaultRole      string
	ClockSkew        time.Duration
	UsernameClaim    string
	DisplayNameClaim string
	EmailClaim       string
	RoleClaim        string
	HTTPClient       *http.Client
}

func (opts OIDCProviderOptions) normalize() OIDCProviderOptions {
	if opts.ClockSkew <= 0 {
		opts.ClockSkew = 30 * time.Second
	}
	if opts.JWKSCacheTTL <= 0 {
		opts.JWKSCacheTTL = 5 * time.Minute
	}
	if opts.UsernameClaim == "" {
		opts.UsernameClaim = "preferred_username"
	}
	if opts.DisplayNameClaim == "" {
		opts.DisplayNameClaim = "name"
	}
	if opts.EmailClaim == "" {
		opts.EmailClaim = "email"
	}
	if opts.RoleClaim == "" {
		opts.RoleClaim = "role"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	return opts
}

// oidcProvider authenticates a user by validating a JWT (id_token/access token).
type oidcProvider struct {
	repo      Repository
	logger    *zap.Logger
	validator *JWTValidator
	opts      OIDCProviderOptions
}

// NewOIDCProvider builds an OIDC-backed identity provider.
func NewOIDCProvider(repo Repository, logger *zap.Logger, opts OIDCProviderOptions) (IdentityProvider, error) {
	if repo == nil {
		return nil, errors.New("auth: oidc provider requires repository")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	opts = opts.normalize()
	if strings.TrimSpace(opts.Issuer) == "" {
		return nil, errors.New("auth: oidc issuer is required")
	}
	if strings.TrimSpace(opts.JWKSURL) == "" && strings.TrimSpace(opts.HMACSecret) == "" {
		return nil, errors.New("auth: jwksURL or hmacSecret is required")
	}
	validator, err := NewJWTValidator(opts)
	if err != nil {
		return nil, err
	}
	return &oidcProvider{
		repo:      repo,
		logger:    logger,
		validator: validator,
		opts:      opts,
	}, nil
}

func (p *oidcProvider) Name() string {
	return "oidc"
}

func (p *oidcProvider) Authenticate(ctx context.Context, req ProviderRequest) (User, ProviderMetadata, error) {
	if p == nil || p.validator == nil || p.repo == nil {
		return User{}, nil, ErrInvalidCredentials
	}
	token := pickOIDCToken(req)
	if token == "" {
		return User{}, nil, ErrInvalidCredentials
	}
	identity, claims, err := p.validator.Validate(ctx, token)
	if err != nil {
		return User{}, nil, ErrInvalidCredentials
	}
	username := strings.TrimSpace(strings.ToLower(identity.Username))
	if username == "" {
		username = strings.TrimSpace(strings.ToLower(identity.Subject))
	}
	if username == "" {
		return User{}, nil, ErrInvalidCredentials
	}
	user, err := p.repo.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, nil, ErrInvalidCredentials
		}
		return User{}, nil, err
	}
	if !user.Active() {
		return User{}, nil, ErrInvalidCredentials
	}
	// Optionally refresh display/email from claims if present.
	if name := strings.TrimSpace(identity.DisplayName); name != "" {
		user.DisplayName = name
	}
	if email := strings.TrimSpace(identity.Email); email != "" {
		user.Email = sql.NullString{String: email, Valid: true}
	}
	if err := p.repo.UpdateLastLogin(ctx, user.ID, time.Now().UTC()); err != nil {
		p.logger.Warn("auth: oidc update last login failed", zap.String("userId", user.ID), zap.Error(err))
	}
	meta := ProviderMetadata{
		"authMethod": p.Name(),
		"issuer":     p.opts.Issuer,
		"subject":    identity.Subject,
		"scopes":     identity.Scopes,
	}
	if identity.Actor != "" {
		meta["actor"] = identity.Actor
	}
	if identity.Role != "" {
		meta["role"] = identity.Role
	}
	meta["claims"] = claims
	return user, meta, nil
}

func pickOIDCToken(req ProviderRequest) string {
	if req.Claims != nil {
		if v, ok := req.Claims["idToken"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := req.Claims["token"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := req.Claims["accessToken"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if strings.TrimSpace(req.Password) != "" {
		return strings.TrimSpace(req.Password)
	}
	return ""
}
