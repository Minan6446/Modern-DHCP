package auth

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

// JWTValidatorOptions configures JWT validation and JWKS rotation.
// This intentionally mirrors OIDCProviderOptions so callers can share config.
type JWTValidatorOptions = OIDCProviderOptions

// JWTIdentity captures normalized fields extracted from a bearer token.
type JWTIdentity struct {
	Subject     string
	Actor       string
	Role        string
	Scopes      []string
	Username    string
	DisplayName string
	Email       string
}

// JWTValidator performs JWT validation with JWKS rotation.
type JWTValidator struct {
	opts JWTValidatorOptions
	http *http.Client
	mu   sync.RWMutex
	jwks map[string]*rsa.PublicKey
	exp  time.Time
	hmac []byte
}

// NewJWTValidator builds a validator with JWKS or HMAC verification.
func NewJWTValidator(opts JWTValidatorOptions) (*JWTValidator, error) {
	opts = opts.normalize()
	v := &JWTValidator{
		opts: opts,
		http: opts.HTTPClient,
		jwks: make(map[string]*rsa.PublicKey),
	}
	if opts.HMACSecret != "" {
		v.hmac = []byte(opts.HMACSecret)
	}
	if strings.TrimSpace(opts.JWKSURL) == "" && len(v.hmac) == 0 {
		return nil, errors.New("jwt: jwksURL or hmacSecret is required")
	}
	if opts.JWKSURL != "" {
		if err := v.refreshJWKS(context.Background(), true); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// Validate parses and verifies a JWT, returning normalized identity and raw claims.
func (v *JWTValidator) Validate(ctx context.Context, token string) (JWTIdentity, map[string]any, error) {
	if v == nil {
		return JWTIdentity{}, nil, errors.New("validator disabled")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return JWTIdentity{}, nil, errors.New("missing token")
	}
	parserOpts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "HS256", "HS384", "HS512"})}
	if v.opts.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(v.opts.Issuer))
	}
	if v.opts.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(v.opts.Audience))
	}
	parserOpts = append(parserOpts, jwt.WithLeeway(v.opts.ClockSkew))
	claims := jwt.MapClaims{}
	parsed, err := jwt.NewParser(parserOpts...).ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if v.hmac != nil && strings.HasPrefix(t.Method.Alg(), "HS") {
			return v.hmac, nil
		}
		return v.lookupKey(ctx, t.Header["kid"])
	})
	if err != nil {
		return JWTIdentity{}, nil, err
	}
	if !parsed.Valid {
		return JWTIdentity{}, nil, errors.New("invalid token")
	}
	scopes := extractScopes(claims)
	if err := requireScopes(v.opts.RequiredScopes, scopes); err != nil {
		return JWTIdentity{}, nil, err
	}
	role := roleFromClaims(claims, v.opts.RoleClaim)
	if role == "" {
		role = roleFromScopeMap(scopes, v.opts.ScopeRoles)
	}
	if role == "" {
		role = normalizeRole(v.opts.DefaultRole)
	}
	identity := JWTIdentity{
		Subject:     stringValue(claims["sub"]),
		Actor:       pickActor(claims),
		Role:        role,
		Scopes:      scopes,
		Username:    pickUsername(claims, v.opts.UsernameClaim),
		DisplayName: stringValue(claims[v.opts.DisplayNameClaim]),
		Email:       stringValue(claims[v.opts.EmailClaim]),
	}
	return identity, claims, nil
}

func (v *JWTValidator) lookupKey(ctx context.Context, kid interface{}) (interface{}, error) {
	if kidStr, ok := kid.(string); ok && kidStr != "" {
		if key := v.cachedKey(kidStr); key != nil {
			return key, nil
		}
	}
	if err := v.refreshJWKS(ctx, true); err != nil {
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
	if v.exp.After(time.Now()) {
		if key, ok := v.jwks[kid]; ok {
			return key
		}
	}
	return nil
}

func (v *JWTValidator) refreshJWKS(ctx context.Context, force bool) error {
	if v.opts.JWKSURL == "" {
		return nil
	}
	if !force && v.exp.After(time.Now()) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.opts.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
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
	v.exp = time.Now().Add(v.opts.JWKSCacheTTL)
	v.mu.Unlock()
	return nil
}

func parseRSAJWK(raw json.RawMessage) (*rsa.PublicKey, string, error) {
	var key struct {
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, "", err
	}
	if !strings.EqualFold(key.Kty, "rsa") || key.N == "" || key.E == "" {
		return nil, "", errors.New("unsupported jwk")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, "", err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, "", err
	}
	if len(eBytes) > 3 {
		return nil, "", errors.New("invalid exponent")
	}
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 + int(b)
	}
	pubKey := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: eInt}
	return pubKey, key.Kid, nil
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

func requireScopes(required, scopes []string) error {
	if len(required) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		have[strings.TrimSpace(s)] = struct{}{}
	}
	for _, scope := range required {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := have[scope]; !ok {
			return fmt.Errorf("missing required scope %s", scope)
		}
	}
	return nil
}

func roleFromScopeMap(scopes []string, mappings map[string]string) string {
	for _, scope := range scopes {
		if role, ok := mappings[scope]; ok {
			return normalizeRole(role)
		}
	}
	return ""
}

func roleFromClaims(claims jwt.MapClaims, key string) string {
	if key == "" {
		return ""
	}
	if raw, ok := claims[key]; ok {
		if str, ok := raw.(string); ok {
			return normalizeRole(str)
		}
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if str, ok := v.(string); ok {
					return normalizeRole(str)
				}
			}
		}
	}
	return ""
}

func pickUsername(claims jwt.MapClaims, claimKey string) string {
	if claimKey != "" {
		if username := stringValue(claims[claimKey]); username != "" {
			return username
		}
	}
	if username := stringValue(claims["preferred_username"]); username != "" {
		return username
	}
	if username := stringValue(claims["email"]); username != "" {
		return username
	}
	return stringValue(claims["sub"])
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
