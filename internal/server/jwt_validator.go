package server

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator validates OAuth2/OIDC bearer tokens.
type JWTValidator struct {
	opts       OAuthOptions
	hmacKey    []byte
	httpClient *http.Client
	mu         sync.RWMutex
	jwks       map[string]*rsa.PublicKey
	expiresAt  time.Time
}

// JWTIdentity captures the normalized subject of a bearer token.
type JWTIdentity struct {
	Subject string
	Actor   string
	Role    string
	Scopes  []string
}

// NewJWTValidator constructs a validator if OAuth is enabled.
func NewJWTValidator(opts OAuthOptions) (*JWTValidator, error) {
	if !opts.Enabled {
		return nil, nil
	}
	v := &JWTValidator{
		opts: opts,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		jwks: make(map[string]*rsa.PublicKey),
	}
	if opts.HMACSecret != "" {
		v.hmacKey = []byte(opts.HMACSecret)
	}
	if opts.JWKSURL != "" {
		if err := v.refreshJWKS(context.Background()); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// Validate parses and verifies a bearer token.
func (v *JWTValidator) Validate(ctx context.Context, token string) (JWTIdentity, error) {
	if v == nil {
		return JWTIdentity{}, errors.New("jwt validator disabled")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return JWTIdentity{}, errors.New("missing token")
	}
	parserOpts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256", "HS256"})}
	if v.opts.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(v.opts.Issuer))
	}
	if v.opts.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(v.opts.Audience))
	}
	leeway := v.opts.ClockSkew
	if leeway <= 0 {
		leeway = 30 * time.Second
	}
	parserOpts = append(parserOpts, jwt.WithLeeway(leeway))
	parser := jwt.NewParser(parserOpts...)
	claims := jwt.MapClaims{}
	jwtToken, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if v.hmacKey != nil && strings.HasPrefix(t.Method.Alg(), "HS") {
			return v.hmacKey, nil
		}
		return v.lookupKey(ctx, t.Header["kid"])
	})
	if err != nil {
		return JWTIdentity{}, err
	}
	if !jwtToken.Valid {
		return JWTIdentity{}, errors.New("invalid token")
	}
	scopes := extractScopes(claims)
	if err := v.requireScopes(scopes); err != nil {
		return JWTIdentity{}, err
	}
	identity := JWTIdentity{
		Subject: stringValue(claims["sub"]),
		Actor:   pickActor(claims),
		Scopes:  scopes,
		Role:    v.roleFromScopes(scopes),
	}
	if identity.Role == "" {
		identity.Role = normalizeRole(v.opts.DefaultRole)
	}
	return identity, nil
}

func (v *JWTValidator) requireScopes(scopes []string) error {
	if len(v.opts.RequiredScopes) == 0 {
		return nil
	}
	want := make(map[string]struct{}, len(v.opts.RequiredScopes))
	for _, scope := range v.opts.RequiredScopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		want[scope] = struct{}{}
	}
	have := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		have[s] = struct{}{}
	}
	for scope := range want {
		if _, ok := have[scope]; !ok {
			return fmt.Errorf("missing required scope %s", scope)
		}
	}
	return nil
}

func (v *JWTValidator) roleFromScopes(scopes []string) string {
	for _, scope := range scopes {
		if role, ok := v.opts.ScopeRoles[scope]; ok {
			return normalizeRole(role)
		}
	}
	return ""
}

func pickActor(claims jwt.MapClaims) string {
	if actor := stringValue(claims["preferred_username"]); actor != "" {
		return actor
	}
	if actor := stringValue(claims["email"]); actor != "" {
		return actor
	}
	return stringValue(claims["sub"])
}

func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return ""
	}
}

func extractScopes(claims jwt.MapClaims) []string {
	if raw, ok := claims["scope"].(string); ok {
		return strings.Fields(raw)
	}
	if rawSlice, ok := claims["scp"].([]interface{}); ok {
		scopes := make([]string, 0, len(rawSlice))
		for _, item := range rawSlice {
			if str, ok := item.(string); ok {
				scopes = append(scopes, str)
			}
		}
		return scopes
	}
	return nil
}

func (v *JWTValidator) lookupKey(ctx context.Context, kid interface{}) (interface{}, error) {
	if kidStr, ok := kid.(string); ok && kidStr != "" {
		if key := v.cachedKey(kidStr); key != nil {
			return key, nil
		}
	}
	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}
	if kidStr, ok := kid.(string); ok && kidStr != "" {
		if key := v.cachedKey(kidStr); key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("jwks key %s not found", kidStr)
	}
	for _, key := range v.jwks {
		return key, nil
	}
	return nil, errors.New("jwks empty")
}

func (v *JWTValidator) cachedKey(kid string) interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.expiresAt.After(time.Now()) {
		if key, ok := v.jwks[kid]; ok {
			return key
		}
	}
	return nil
}

func (v *JWTValidator) refreshJWKS(ctx context.Context) error {
	if v.opts.JWKSURL == "" {
		return nil
	}
	if v.expiresAt.After(time.Now()) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.opts.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("jwks fetch failed: %d", resp.StatusCode)
	}
	var payload struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	parsed := make(map[string]*rsa.PublicKey)
	for _, rawKey := range payload.Keys {
		key, kid, err := parseRSAJWK(rawKey)
		if err != nil {
			continue
		}
		parsed[kid] = key
	}
	if len(parsed) == 0 {
		return errors.New("jwks contains no rsa keys")
	}
	v.mu.Lock()
	v.jwks = parsed
	ttl := v.opts.JWKSCacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	v.expiresAt = time.Now().Add(ttl)
	v.mu.Unlock()
	return nil
}

func parseRSAJWK(raw json.RawMessage) (*rsa.PublicKey, string, error) {
	var jwk struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	if err := json.Unmarshal(raw, &jwk); err != nil {
		return nil, "", err
	}
	if jwk.Kty != "RSA" {
		return nil, "", fmt.Errorf("unsupported kty %s", jwk.Kty)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, "", err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, "", err
	}
	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		e = 65537
	}
	pubKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}
	return pubKey, jwk.Kid, nil
}
