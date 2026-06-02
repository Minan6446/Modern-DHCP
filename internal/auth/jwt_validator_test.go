package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTValidatorRS256WithScopes(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kid := "k1"
	jwk := marshalRSAJWK(t, &priv.PublicKey, kid)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []json.RawMessage{jwk}})
	}))
	defer server.Close()

	claims := jwt.MapClaims{
		"iss":                "https://issuer.example.com",
		"aud":                "api",
		"sub":                "user-1",
		"scope":              "read write",
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Add(-time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	validator, err := NewJWTValidator(OIDCProviderOptions{
		Issuer:         "https://issuer.example.com",
		Audience:       "api",
		JWKSURL:        server.URL,
		RequiredScopes: []string{"read"},
		ScopeRoles:     map[string]string{"write": "admin"},
		DefaultRole:    "reader",
	})
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}

	identity, _, err := validator.Validate(context.Background(), signed)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if identity.Subject != "user-1" {
		t.Fatalf("unexpected subject: %s", identity.Subject)
	}
	if identity.Actor != "alice" {
		t.Fatalf("unexpected actor: %s", identity.Actor)
	}
	if identity.Role != "admin" {
		t.Fatalf("role should map from scope to admin, got %s", identity.Role)
	}
}

func TestJWTValidatorRejectsMissingScope(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := marshalRSAJWK(t, &priv.PublicKey, "scope")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []json.RawMessage{jwk}})
	}))
	defer server.Close()
	claims := jwt.MapClaims{
		"iss":   "issuer",
		"aud":   "aud",
		"sub":   "user-2",
		"scope": "basic",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "scope"
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	validator, err := NewJWTValidator(OIDCProviderOptions{Issuer: "issuer", Audience: "aud", JWKSURL: server.URL, RequiredScopes: []string{"write"}})
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}
	if _, _, err := validator.Validate(context.Background(), signed); err == nil {
		t.Fatalf("expected missing scope error")
	}
}

func TestJWTValidatorJWKSRotationOnKidMiss(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 1024)
	key2, _ := rsa.GenerateKey(rand.Reader, 1024)
	jwk1 := marshalRSAJWK(t, &key1.PublicKey, "k1")
	jwk2 := marshalRSAJWK(t, &key2.PublicKey, "k2")
	var call int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{"keys": []json.RawMessage{jwk1}}
		if call > 0 {
			payload = map[string]any{"keys": []json.RawMessage{jwk2}}
		}
		call++
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()
	validator, err := NewJWTValidator(OIDCProviderOptions{Issuer: "issuer", Audience: "aud", JWKSURL: server.URL, JWKSCacheTTL: time.Minute})
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}
	claims := jwt.MapClaims{
		"iss": "issuer",
		"aud": "aud",
		"sub": "user-rotate",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "k2"
	signed, err := token.SignedString(key2)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	identity, _, err := validator.Validate(context.Background(), signed)
	if err != nil {
		t.Fatalf("validate after rotation: %v", err)
	}
	if identity.Subject != "user-rotate" {
		t.Fatalf("unexpected subject %s", identity.Subject)
	}
}

func TestJWTValidatorHMAC(t *testing.T) {
	claims := jwt.MapClaims{
		"iss":   "issuer",
		"aud":   "aud",
		"sub":   "user-hmac",
		"scope": "s1",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	validator, err := NewJWTValidator(OIDCProviderOptions{Issuer: "issuer", Audience: "aud", HMACSecret: "secret", RequiredScopes: []string{"s1"}, DefaultRole: "reader"})
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}
	identity, _, err := validator.Validate(context.Background(), signed)
	if err != nil {
		t.Fatalf("validate hmac: %v", err)
	}
	if identity.Role != "reader" {
		t.Fatalf("expected default role reader, got %s", identity.Role)
	}
}

func marshalRSAJWK(t *testing.T, pub *rsa.PublicKey, kid string) json.RawMessage {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(bigIntToBytes(pub.E))
	data := map[string]string{
		"kty": "RSA",
		"kid": kid,
		"n":   n,
		"e":   e,
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal jwk: %v", err)
	}
	return raw
}

func bigIntToBytes(e int) []byte {
	if e == 0 {
		return []byte{1}
	}
	buf := []byte{}
	for e > 0 {
		buf = append([]byte{byte(e % 256)}, buf...)
		e = e / 256
	}
	return buf
}
