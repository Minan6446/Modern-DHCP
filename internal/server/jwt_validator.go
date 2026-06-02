package server

import "modern-dhcp/internal/auth"

// Re-export identity for consumers in server package.
type JWTIdentity = auth.JWTIdentity
type JWTValidator = auth.JWTValidator

// NewJWTValidator constructs a validator for bearer authentication.
func NewJWTValidator(opts OAuthOptions) (*auth.JWTValidator, error) {
	if !opts.Enabled {
		return nil, nil
	}
	return auth.NewJWTValidator(auth.OIDCProviderOptions{
		Issuer:           opts.Issuer,
		Audience:         opts.Audience,
		JWKSURL:          opts.JWKSURL,
		JWKSCacheTTL:     opts.JWKSCacheTTL,
		HMACSecret:       opts.HMACSecret,
		RequiredScopes:   opts.RequiredScopes,
		ScopeRoles:       opts.ScopeRoles,
		DefaultRole:      opts.DefaultRole,
		ClockSkew:        opts.ClockSkew,
		UsernameClaim:    "preferred_username",
		DisplayNameClaim: "name",
		EmailClaim:       "email",
		RoleClaim:        "role",
	})
}
