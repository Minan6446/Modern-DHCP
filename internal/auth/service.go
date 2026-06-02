package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrUserNotFound indicates no matching user row exists.
	ErrUserNotFound = errors.New("auth: user not found")
	// ErrInvalidCredentials indicates username/password mismatch.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrAPIKeyNotFound indicates the token hash does not exist.
	ErrAPIKeyNotFound = errors.New("auth: api key not found")
	// ErrAPIKeyRevoked indicates the token has been revoked or expired.
	ErrAPIKeyRevoked = errors.New("auth: api key revoked")
	// ErrPasswordTooShort indicates the requested password fails the minimum policy.
	ErrPasswordTooShort = errors.New("auth: password too short")
	// ErrUserExists indicates the username already exists in the repository.
	ErrUserExists = errors.New("auth: user already exists")
	// ErrIdentityProviderNotFound signals the identity provider does not exist.
	ErrIdentityProviderNotFound = errors.New("auth: identity provider not found")
	// ErrIdentityProviderExists indicates a duplicate provider identifier.
	ErrIdentityProviderExists = errors.New("auth: identity provider already exists")
)

// User models a console identity stored in auth_users.
type User struct {
	ID                 string         `db:"id"`
	Username           string         `db:"username"`
	DisplayName        string         `db:"display_name"`
	Email              sql.NullString `db:"email"`
	PasswordHash       []byte         `db:"password_hash"`
	Role               string         `db:"role"`
	Status             string         `db:"status"`
	MustChangePassword bool           `db:"must_change_password"`
	LastLoginAt        sql.NullTime   `db:"last_login_at"`
	CreatedAt          time.Time      `db:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at"`
}

// Active returns true when the user can authenticate.
func (u User) Active() bool {
	return strings.EqualFold(u.Status, "active")
}

// PrincipalID composes the RBAC principal identifier.
func (u User) PrincipalID() string {
	return fmt.Sprintf("user:%s", u.ID)
}

// APIKey kinds.
const (
	KeyKindService = "service"
	KeyKindSession = "session"
	KeyKindSystem  = "system"
)

// APIKey mirrors auth_api_keys rows.
type APIKey struct {
	ID          string          `db:"id"`
	Name        string          `db:"name"`
	Prefix      string          `db:"prefix"`
	TokenHash   string          `db:"token_hash"`
	Role        string          `db:"role"`
	PrincipalID string          `db:"principal_id"`
	TenantScope sql.NullString  `db:"tenant_scope"`
	Kind        string          `db:"kind"`
	OwnerUserID sql.NullString  `db:"owner_user_id"`
	Description sql.NullString  `db:"description"`
	CreatedBy   string          `db:"created_by"`
	CreatedAt   time.Time       `db:"created_at"`
	ExpiresAt   sql.NullTime    `db:"expires_at"`
	LastUsedAt  sql.NullTime    `db:"last_used_at"`
	RevokedAt   sql.NullTime    `db:"revoked_at"`
	RevokedBy   sql.NullString  `db:"revoked_by"`
	Metadata    json.RawMessage `db:"metadata"`
}

// Valid returns true when the API key can still be used.
func (k APIKey) Valid(now time.Time) bool {
	if k.RevokedAt.Valid {
		return false
	}
	if k.ExpiresAt.Valid && now.After(k.ExpiresAt.Time) {
		return false
	}
	return true
}

// IdentityProviderRecord mirrors auth_identity_providers rows.
type IdentityProviderRecord struct {
	ID        string          `db:"id"`
	Name      string          `db:"name"`
	Type      string          `db:"type"`
	Endpoint  sql.NullString  `db:"endpoint"`
	Config    json.RawMessage `db:"config"`
	Enabled   bool            `db:"enabled"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

// IdentityProvider describes the management view of an auth provider.
type ManagedIdentityProvider struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Endpoint  string         `json:"endpoint,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	Enabled   bool           `json:"enabled"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// CreateIdentityProviderOptions captures creation payload.
type CreateIdentityProviderOptions struct {
	ID       string
	Name     string
	Type     string
	Endpoint string
	Config   map[string]any
	Enabled  *bool
}

// UpdateIdentityProviderOptions captures partial updates.
type UpdateIdentityProviderOptions struct {
	Name     *string
	Endpoint *string
	Config   *map[string]any
	Enabled  *bool
}

// IdentityProviderUpdate is the storage mutation payload.
type IdentityProviderUpdate struct {
	ID        string
	Name      *string
	Endpoint  *string
	Config    json.RawMessage
	ConfigSet bool
	Enabled   *bool
	UpdatedAt time.Time
}

// Repository defines storage operations required by the service.
type Repository interface {
	GetUserByUsername(ctx context.Context, username string) (User, error)
	GetUserByID(ctx context.Context, id string) (User, error)
	ListUsers(ctx context.Context, filter UserFilter) ([]User, int, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, update UserUpdate) (User, error)
	DeleteUser(ctx context.Context, id string) error
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	UpdatePassword(ctx context.Context, userID string, passwordHash []byte, mustChange bool, updatedAt time.Time) error
	CreateAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error)
	ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error)
	UpdateAPIKey(ctx context.Context, update APIKeyUpdate) (APIKey, error)
	RevokeAPIKey(ctx context.Context, id string, revokedBy string, at time.Time) error
	MarkAPIKeyUsed(ctx context.Context, id string, at time.Time) error
	ListIdentityProviders(ctx context.Context) ([]IdentityProviderRecord, error)
	GetIdentityProvider(ctx context.Context, id string) (IdentityProviderRecord, error)
	CreateIdentityProvider(ctx context.Context, provider IdentityProviderRecord) error
	UpdateIdentityProvider(ctx context.Context, update IdentityProviderUpdate) (IdentityProviderRecord, error)
	ListUserRoles(ctx context.Context, userID string) ([]string, error)
	ReplaceUserRoles(ctx context.Context, userID string, roles []string, now time.Time) error
}

// UserFilter constrains list queries for identities.
type UserFilter struct {
	TenantID *string
	Status   string
	Query    string
	Limit    int
	Offset   int
	SortBy   string
	SortDesc bool
}

// CreateUserOptions captures user creation input.
type CreateUserOptions struct {
	Username           string
	DisplayName        string
	Email              string
	Role               string
	TenantID           string
	Status             string
	Password           string
	MustChangePassword *bool
}

// UpdateUserOptions carries partial updates for an identity.
type UpdateUserOptions struct {
	// Username is not mutated, but can be used to resolve a user when the provided ID is stale.
	Username           *string
	DisplayName        *string
	Email              *string
	Role               *string
	TenantID           *string
	Status             *string
	MustChangePassword *bool
	Password           *string
}

// UserUpdate is the storage-level mutation payload.
type UserUpdate struct {
	ID                 string
	DisplayName        *string
	Email              *string
	Role               *string
	TenantID           *string
	Status             *string
	MustChangePassword *bool
	PasswordHash       []byte
	ResetPassword      bool
	UpdatedAt          time.Time
}

// HasChanges reports whether the update mutates any fields beyond timestamps.
func (u UserUpdate) HasChanges() bool {
	if u.DisplayName != nil || u.Email != nil || u.Role != nil || u.TenantID != nil || u.Status != nil {
		return true
	}
	if u.MustChangePassword != nil {
		return true
	}
	if u.ResetPassword {
		return true
	}
	return false
}

// APIKeyFilter filters list operations.
type APIKeyFilter struct {
	OwnerUserID    *string
	IncludeRevoked bool
	Kinds          []string
}

// APIKeyUpdate carries partial update data for API keys.
type APIKeyUpdate struct {
	ID           string
	Name         *string
	Role         *string
	Description  *string
	ExpiresAt    *time.Time
	ExpiresAtSet bool
	Metadata     json.RawMessage
	MetadataSet  bool
	RevokedAt    *time.Time
	RevokedBy    *string
	RevokeSet    bool
}

// UpdateAPIKeyOptions defines editable API key fields.
type UpdateAPIKeyOptions struct {
	DisplayName    *string
	Role           *string
	Description    *string
	Metadata       *map[string]any
	ExpiresAt      *time.Time
	ClearExpiresAt bool
	Enabled        *bool
}

// TokenMetadata is returned to authenticators once a token is validated.
type TokenMetadata struct {
	DisplayName string
	Role        string
	PrincipalID string
}

// TokenProvider exposes lookup semantics for API credentials.
type TokenProvider interface {
	LookupToken(ctx context.Context, token string) (TokenMetadata, error)
}

// SessionOptions customize login-session issuance.
type SessionOptions struct {
	TenantID   string
	TTL        time.Duration
	AuthMethod string
	ClientIP   string
	UserAgent  string
}

// SessionToken describes the issued token and expiry metadata.
type SessionToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Service bundles authentication helpers.
type Service struct {
	repo            Repository
	logger          *zap.Logger
	sessionTTL      time.Duration
	clock           func() time.Time
	providers       map[string]IdentityProvider
	defaultProvider string
}

// ServiceOption mutates optional wiring.
type ServiceOption func(*Service)

// WithSessionTTL overrides the default session token TTL.
func WithSessionTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
		}
	}
}

// WithClock supplies a deterministic time source.
func WithClock(clock func() time.Time) ServiceOption {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// NewService constructs a Service instance bound to repo.
func NewService(repo Repository, logger *zap.Logger, opts ...ServiceOption) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	svc := &Service{
		repo:       repo,
		logger:     logger,
		sessionTTL: 24 * time.Hour,
		clock:      time.Now().UTC,
		providers:  make(map[string]IdentityProvider),
	}
	svc.RegisterProvider(newLocalProvider(repo, logger, svc.now), WithProviderDefault())
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Authenticate verifies username/password and returns the user row.
func (s *Service) Authenticate(ctx context.Context, username, password string) (User, error) {
	result, err := s.AuthenticateWithProvider(ctx, "", ProviderRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return User{}, err
	}
	return result.User, nil
}

// AuthenticateWithProvider resolves the requested provider and authenticates the subject.
func (s *Service) AuthenticateWithProvider(ctx context.Context, method string, req ProviderRequest) (ProviderResult, error) {
	provider := s.resolveProvider(method)
	if provider == nil {
		return ProviderResult{}, ErrProviderNotFound
	}
	user, metadata, err := provider.Authenticate(ctx, req)
	if err != nil {
		return ProviderResult{}, err
	}
	result := ProviderResult{User: user, Method: provider.Name()}
	if len(metadata) > 0 {
		result.Metadata = metadata
	}
	return result, nil
}

// RegisterProvider wires a new identity provider into the service.
func (s *Service) RegisterProvider(provider IdentityProvider, opts ...ProviderOption) {
	if s == nil || provider == nil {
		return
	}
	if s.providers == nil {
		s.providers = make(map[string]IdentityProvider)
	}
	name := strings.ToLower(strings.TrimSpace(provider.Name()))
	if name == "" {
		return
	}
	s.providers[name] = provider
	reg := providerRegistration{}
	for _, opt := range opts {
		if opt != nil {
			opt(&reg)
		}
	}
	if reg.makeDefault || s.defaultProvider == "" {
		s.defaultProvider = name
	}
}

// UnregisterProvider removes the named provider from the registry.
func (s *Service) UnregisterProvider(name string) {
	if s == nil || name == "" {
		return
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || len(s.providers) == 0 {
		return
	}
	if _, ok := s.providers[key]; !ok {
		return
	}
	delete(s.providers, key)
	if s.defaultProvider == key {
		s.defaultProvider = ""
		for candidate := range s.providers {
			s.defaultProvider = candidate
			break
		}
	}
}

// ListIdentityProviders returns all configured identity providers for management UI.
func (s *Service) ListIdentityProviders(ctx context.Context) ([]ManagedIdentityProvider, error) {
	if s == nil || s.repo == nil {
		return []ManagedIdentityProvider{}, nil
	}
	records, err := s.repo.ListIdentityProviders(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []ManagedIdentityProvider{}, nil
		}
		return nil, err
	}
	providers := make([]ManagedIdentityProvider, 0, len(records))
	for _, record := range records {
		provider, convErr := recordToProvider(record)
		if convErr != nil {
			return nil, convErr
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

// CreateIdentityProvider registers a new provider configuration.
func (s *Service) CreateIdentityProvider(ctx context.Context, opts CreateIdentityProviderOptions) (ManagedIdentityProvider, error) {
	if s == nil || s.repo == nil {
		return ManagedIdentityProvider{}, errors.New("auth: provider repository unavailable")
	}
	id := normalizeProviderIdentifier(opts.ID)
	if id == "" {
		return ManagedIdentityProvider{}, errors.New("auth: provider id is required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return ManagedIdentityProvider{}, errors.New("auth: provider name is required")
	}
	typ := strings.TrimSpace(strings.ToLower(opts.Type))
	if typ == "" {
		return ManagedIdentityProvider{}, errors.New("auth: provider type is required")
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	configPayload, err := encodeProviderConfig(opts.Config)
	if err != nil {
		return ManagedIdentityProvider{}, err
	}
	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}
	now := s.now()
	record := IdentityProviderRecord{
		ID:        id,
		Name:      name,
		Type:      typ,
		Endpoint:  stringToNull(endpoint),
		Config:    configPayload,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateIdentityProvider(ctx, record); err != nil {
		if isDuplicateEntryError(err) {
			return ManagedIdentityProvider{}, ErrIdentityProviderExists
		}
		return ManagedIdentityProvider{}, err
	}
	return recordToProvider(record)
}

// UpdateIdentityProvider mutates an existing provider configuration.
func (s *Service) UpdateIdentityProvider(ctx context.Context, id string, opts UpdateIdentityProviderOptions) (ManagedIdentityProvider, error) {
	if s == nil || s.repo == nil {
		return ManagedIdentityProvider{}, errors.New("auth: provider repository unavailable")
	}
	trimmedID := normalizeProviderIdentifier(id)
	if trimmedID == "" {
		return ManagedIdentityProvider{}, errors.New("auth: provider id is required")
	}
	update := IdentityProviderUpdate{ID: trimmedID, UpdatedAt: s.now()}
	if opts.Name != nil {
		name := strings.TrimSpace(*opts.Name)
		if name == "" {
			return ManagedIdentityProvider{}, errors.New("auth: provider name cannot be empty")
		}
		update.Name = &name
	}
	if opts.Endpoint != nil {
		endpoint := strings.TrimSpace(*opts.Endpoint)
		update.Endpoint = &endpoint
	}
	if opts.Config != nil {
		payload, err := encodeProviderConfig(*opts.Config)
		if err != nil {
			return ManagedIdentityProvider{}, err
		}
		update.Config = payload
		update.ConfigSet = true
	}
	if opts.Enabled != nil {
		update.Enabled = opts.Enabled
	}
	if update.Name == nil && update.Endpoint == nil && !update.ConfigSet && update.Enabled == nil {
		return ManagedIdentityProvider{}, errors.New("auth: no provider changes supplied")
	}
	record, err := s.repo.UpdateIdentityProvider(ctx, update)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedIdentityProvider{}, ErrIdentityProviderNotFound
		}
		return ManagedIdentityProvider{}, err
	}
	return recordToProvider(record)
}

func (s *Service) resolveProvider(method string) IdentityProvider {
	if s == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(method))
	if name == "" {
		name = s.defaultProvider
	}
	if name == "" {
		return nil
	}
	if provider, ok := s.providers[name]; ok {
		return provider
	}
	return nil
}

// IssueSession issues a short-lived API key for the authenticated user.
func (s *Service) IssueSession(ctx context.Context, user User, opts SessionOptions) (SessionToken, error) {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = s.sessionTTL
	}
	token, prefix, err := generateToken()
	if err != nil {
		return SessionToken{}, err
	}
	now := s.now()
	expiresAt := now.Add(ttl)
	key := &APIKey{
		ID:          uuid.NewString(),
		Name:        fmt.Sprintf("session:%s", user.Username),
		Prefix:      prefix,
		TokenHash:   hashToken(token),
		Role:        normalizeRole(user.Role),
		PrincipalID: user.PrincipalID(),
		Kind:        KeyKindSession,
		CreatedBy:   user.Username,
		CreatedAt:   now,
		ExpiresAt:   sqlNullTime(expiresAt),
	}
	meta := map[string]any{}
	if strings.TrimSpace(opts.AuthMethod) != "" {
		meta["authMethod"] = strings.ToLower(strings.TrimSpace(opts.AuthMethod))
	}
	if ip := strings.TrimSpace(opts.ClientIP); ip != "" {
		meta["clientIp"] = ip
	}
	if ua := strings.TrimSpace(opts.UserAgent); ua != "" {
		meta["userAgent"] = ua
	}
	if len(meta) > 0 {
		if payload, err := json.Marshal(meta); err == nil {
			key.Metadata = payload
		}
	}
	if opts.TenantID != "" {
		key.TenantScope = sql.NullString{String: opts.TenantID, Valid: true}
	}
	if user.ID != "" {
		if _, err := uuid.Parse(user.ID); err == nil {
			key.OwnerUserID = sql.NullString{String: user.ID, Valid: true}
		}
	}
	if err := s.repo.CreateAPIKey(ctx, key); err != nil {
		return SessionToken{}, err
	}
	return SessionToken{Token: token, ExpiresAt: expiresAt}, nil
}

// RefreshSession validates the refresh token and rotates the caller session.
func (s *Service) RefreshSession(ctx context.Context, refreshToken string) (SessionToken, User, string, string, error) {
	trimmed := strings.TrimSpace(refreshToken)
	if trimmed == "" {
		return SessionToken{}, User{}, "", "", ErrAPIKeyNotFound
	}
	key, err := s.repo.GetAPIKeyByHash(ctx, hashToken(trimmed))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionToken{}, User{}, "", "", ErrAPIKeyNotFound
		}
		return SessionToken{}, User{}, "", "", err
	}
	now := s.now()
	if !key.Valid(now) || key.Kind != KeyKindSession {
		return SessionToken{}, User{}, "", "", ErrAPIKeyRevoked
	}
	if !key.OwnerUserID.Valid {
		return SessionToken{}, User{}, "", "", ErrAPIKeyNotFound
	}
	ownerID := strings.TrimSpace(key.OwnerUserID.String)
	if ownerID == "" {
		return SessionToken{}, User{}, "", "", ErrAPIKeyNotFound
	}
	user, err := s.repo.GetUserByID(ctx, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionToken{}, User{}, "", "", ErrUserNotFound
		}
		return SessionToken{}, User{}, "", "", err
	}
	tenantScope := ""
	if key.TenantScope.Valid {
		tenantScope = key.TenantScope.String
	}
	authMethod := authMethodFromMetadata(key.Metadata)
	clientIP, userAgent := sessionClientMetadataFromMetadata(key.Metadata)
	session, err := s.IssueSession(ctx, user, SessionOptions{TenantID: tenantScope, AuthMethod: authMethod, ClientIP: clientIP, UserAgent: userAgent})
	if err != nil {
		return SessionToken{}, User{}, tenantScope, authMethod, err
	}
	if err := s.repo.RevokeAPIKey(ctx, key.ID, user.Username, now); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.Warn("auth: revoke session refresh failed", zap.String("apiKeyId", key.ID), zap.Error(err))
	}
	return session, user, tenantScope, authMethod, nil
}

// CreateAPIKey persists a long-lived API key for service automation.
func (s *Service) CreateAPIKey(ctx context.Context, name, role, principalID, tenantScope, createdBy, description string, metadata map[string]any, expiresAt *time.Time, ownerUserID *string) (string, *APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, errors.New("auth: api key name is required")
	}
	role = normalizeRole(role)
	if role == "" {
		return "", nil, errors.New("auth: api key role is required")
	}
	var owner string
	if ownerUserID != nil {
		owner = strings.TrimSpace(*ownerUserID)
		if owner != "" {
			if _, err := uuid.Parse(owner); err != nil {
				return "", nil, errors.New("auth: owner user id must be a valid uuid")
			}
		}
	}
	now := s.now()
	var expiry time.Time
	if expiresAt != nil {
		expiry = expiresAt.UTC()
		if expiry.Before(now) {
			return "", nil, errors.New("auth: expiresAt must be in the future")
		}
	}
	token, prefix, err := generateToken()
	if err != nil {
		return "", nil, err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = nil
	}
	key := &APIKey{
		ID:          uuid.NewString(),
		Name:        name,
		Prefix:      prefix,
		TokenHash:   hashToken(token),
		Role:        normalizeRole(role),
		PrincipalID: strings.TrimSpace(principalID),
		Kind:        KeyKindService,
		CreatedBy:   createdBy,
		CreatedAt:   s.now(),
		Metadata:    payload,
	}
	if strings.TrimSpace(description) != "" {
		key.Description = sql.NullString{String: strings.TrimSpace(description), Valid: true}
	}
	if tenantScope != "" {
		key.TenantScope = sql.NullString{String: tenantScope, Valid: true}
	}
	if owner != "" {
		key.OwnerUserID = sql.NullString{String: owner, Valid: true}
	}
	if expiresAt != nil {
		key.ExpiresAt = sqlNullTime(expiry)
	}
	if err := s.repo.CreateAPIKey(ctx, key); err != nil {
		return "", nil, err
	}
	return token, key, nil
}

// LookupToken resolves metadata for the provided API key token.
func (s *Service) LookupToken(ctx context.Context, token string) (TokenMetadata, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return TokenMetadata{}, ErrAPIKeyNotFound
	}
	key, err := s.repo.GetAPIKeyByHash(ctx, hashToken(trimmed))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenMetadata{}, ErrAPIKeyNotFound
		}
		return TokenMetadata{}, err
	}
	now := s.now()
	if !key.Valid(now) {
		return TokenMetadata{}, ErrAPIKeyRevoked
	}
	if err := s.repo.MarkAPIKeyUsed(ctx, key.ID, now); err != nil {
		s.logger.Debug("auth: mark api key used failed", zap.String("apiKeyId", key.ID), zap.Error(err))
	}
	return TokenMetadata{
		DisplayName: key.Name,
		Role:        key.Role,
		PrincipalID: key.PrincipalID,
	}, nil
}

// RevokeAPIKey revokes a key by identifier.
func (s *Service) RevokeAPIKey(ctx context.Context, id, actor string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("auth: api key id required")
	}
	if err := s.repo.RevokeAPIKey(ctx, id, actor, s.now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAPIKeyNotFound
		}
		return err
	}
	return nil
}

// RevokeToken revokes a key referenced by its plaintext token value.
func (s *Service) RevokeToken(ctx context.Context, token, actor string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ErrAPIKeyNotFound
	}
	key, err := s.repo.GetAPIKeyByHash(ctx, hashToken(trimmed))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAPIKeyNotFound
		}
		return err
	}
	return s.RevokeAPIKey(ctx, key.ID, actor)
}

// ListAPIKeys returns keys by owner/kind filters.
func (s *Service) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error) {
	return s.repo.ListAPIKeys(ctx, filter)
}

// UpdateAPIKey updates editable fields and/or enabled state for an API key.
func (s *Service) UpdateAPIKey(ctx context.Context, id, actor string, opts UpdateAPIKeyOptions) (APIKey, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return APIKey{}, errors.New("auth: api key id required")
	}
	update := APIKeyUpdate{ID: trimmedID}
	if opts.DisplayName != nil {
		name := strings.TrimSpace(*opts.DisplayName)
		if name == "" {
			return APIKey{}, errors.New("auth: api key name is required")
		}
		update.Name = &name
	}
	if opts.Role != nil {
		role := normalizeRole(*opts.Role)
		update.Role = &role
	}
	if opts.Description != nil {
		desc := strings.TrimSpace(*opts.Description)
		update.Description = &desc
	}
	if opts.ClearExpiresAt {
		update.ExpiresAtSet = true
		update.ExpiresAt = nil
	} else if opts.ExpiresAt != nil {
		expiry := opts.ExpiresAt.UTC()
		if expiry.Before(s.now()) {
			return APIKey{}, errors.New("auth: expiresAt must be in the future")
		}
		update.ExpiresAtSet = true
		update.ExpiresAt = &expiry
	}
	if opts.Metadata != nil {
		payload, err := json.Marshal(*opts.Metadata)
		if err != nil {
			return APIKey{}, err
		}
		update.MetadataSet = true
		update.Metadata = payload
	}
	if opts.Enabled != nil {
		update.RevokeSet = true
		if *opts.Enabled {
			update.RevokedAt = nil
			update.RevokedBy = nil
		} else {
			now := s.now()
			update.RevokedAt = &now
			by := strings.TrimSpace(actor)
			if by == "" {
				by = "system"
			}
			update.RevokedBy = &by
		}
	}
	if update.Name == nil && update.Role == nil && update.Description == nil && !update.ExpiresAtSet && !update.MetadataSet && !update.RevokeSet {
		return APIKey{}, errors.New("auth: no api key changes supplied")
	}
	updated, err := s.repo.UpdateAPIKey(ctx, update)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrAPIKeyNotFound
		}
		return APIKey{}, err
	}
	return updated, nil
}

// UpdatePassword rotates the password hash for the given user identifier.
func (s *Service) UpdatePassword(ctx context.Context, userID, newPassword string, mustChange bool) error {
	if s == nil || s.repo == nil {
		return errors.New("auth: repository unavailable")
	}
	trimmedID := strings.TrimSpace(userID)
	if trimmedID == "" {
		return errors.New("auth: user id required")
	}
	password := strings.TrimSpace(newPassword)
	if utf8.RuneCountInString(password) < 8 {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, trimmedID, hash, mustChange, s.now())
}

// FindUserByUsername returns a user by username for account workflows such as password reset.
func (s *Service) FindUserByUsername(ctx context.Context, username string) (User, error) {
	if s == nil || s.repo == nil {
		return User{}, errors.New("auth: repository unavailable")
	}
	trimmed := strings.ToLower(strings.TrimSpace(username))
	if trimmed == "" {
		return User{}, ErrUserNotFound
	}
	user, err := s.repo.GetUserByUsername(ctx, trimmed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}

// GetUserByID returns a user by ID for workflows that need before/after comparisons.
func (s *Service) GetUserByID(ctx context.Context, userID string) (User, error) {
	if s == nil || s.repo == nil {
		return User{}, errors.New("auth: repository unavailable")
	}
	id := strings.TrimSpace(userID)
	if id == "" {
		return User{}, ErrUserNotFound
	}
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}

// ListUsers returns identities matching the provided filter.
func (s *Service) ListUsers(ctx context.Context, filter UserFilter) ([]User, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, errors.New("auth: repository unavailable")
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.SortBy = sanitizeUserSort(filter.SortBy)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Query != "" {
		filter.Query = strings.ToLower(filter.Query)
	}
	if filter.Status != "" {
		if strings.EqualFold(filter.Status, "all") || strings.EqualFold(filter.Status, "any") {
			filter.Status = ""
		} else {
			filter.Status = normalizeUserStatus(filter.Status)
		}
	}
	if filter.TenantID != nil {
		tenant := strings.TrimSpace(*filter.TenantID)
		if tenant == "" {
			tenant = "default"
		}
		filter.TenantID = &tenant
	}
	return s.repo.ListUsers(ctx, filter)
}

// CreateUser provisions a new local identity and optionally generates a password.
func (s *Service) CreateUser(ctx context.Context, opts CreateUserOptions) (User, string, error) {
	if s == nil || s.repo == nil {
		return User{}, "", errors.New("auth: repository unavailable")
	}
	username := strings.TrimSpace(opts.Username)
	displayName := strings.TrimSpace(opts.DisplayName)
	if username == "" || displayName == "" {
		return User{}, "", errors.New("auth: username and display name are required")
	}
	email := strings.TrimSpace(opts.Email)
	role := normalizeRole(opts.Role)
	status := normalizeUserStatus(opts.Status)
	password := strings.TrimSpace(opts.Password)
	mustChange := false
	if opts.MustChangePassword != nil {
		mustChange = *opts.MustChangePassword
	}
	generated := ""
	if password == "" {
		var err error
		password, err = generatePassword(24)
		if err != nil {
			return User{}, "", err
		}
		generated = password
		if opts.MustChangePassword == nil {
			mustChange = true
		}
	}
	if utf8.RuneCountInString(password) < 8 {
		return User{}, "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, "", err
	}
	now := s.now()
	user := User{
		ID:                 uuid.NewString(),
		Username:           username,
		DisplayName:        displayName,
		Role:               role,
		Status:             status,
		MustChangePassword: mustChange,
		PasswordHash:       hash,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if email != "" {
		user.Email = sql.NullString{String: email, Valid: true}
	}
	if err := s.repo.CreateUser(ctx, &user); err != nil {
		if isDuplicateEntryError(err) {
			return User{}, "", ErrUserExists
		}
		return User{}, "", err
	}
	return user, generated, nil
}

// UpdateUser mutates core profile fields for an existing identity.
func (s *Service) UpdateUser(ctx context.Context, userID string, opts UpdateUserOptions) (User, error) {
	if s == nil || s.repo == nil {
		return User{}, errors.New("auth: repository unavailable")
	}
	trimmedID := strings.TrimSpace(userID)
	resolvedID := trimmedID
	// Resolve the authoritative user ID upfront to avoid stale-id 404s.
	if resolvedID != "" {
		user, err := s.repo.GetUserByID(ctx, resolvedID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return User{}, err
			}
			resolvedID = ""
		} else {
			resolvedID = user.ID
		}
	}
	if resolvedID == "" && opts.Username != nil {
		user, err := s.repo.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(*opts.Username)))
		if err == nil {
			resolvedID = user.ID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return User{}, err
		}
	}
	if resolvedID == "" {
		return User{}, ErrUserNotFound
	}
	update := UserUpdate{ID: resolvedID, UpdatedAt: s.now()}
	if opts.DisplayName != nil {
		name := strings.TrimSpace(*opts.DisplayName)
		if name == "" {
			return User{}, errors.New("auth: display name is required")
		}
		update.DisplayName = &name
	}
	if opts.Email != nil {
		email := strings.TrimSpace(*opts.Email)
		update.Email = &email
	}
	if opts.Role != nil {
		role := normalizeRole(*opts.Role)
		update.Role = &role
	}
	if opts.TenantID != nil {
		tenant := strings.TrimSpace(*opts.TenantID)
		if tenant == "" {
			tenant = "default"
		}
		update.TenantID = &tenant
	}
	if opts.Status != nil {
		status := normalizeUserStatus(*opts.Status)
		update.Status = &status
	}
	if opts.MustChangePassword != nil {
		update.MustChangePassword = opts.MustChangePassword
	}
	if opts.Password != nil {
		password := strings.TrimSpace(*opts.Password)
		if utf8.RuneCountInString(password) < 8 {
			return User{}, ErrPasswordTooShort
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, err
		}
		update.ResetPassword = true
		update.PasswordHash = hash
		if opts.MustChangePassword == nil {
			flag := true
			update.MustChangePassword = &flag
		}
	}
	if !update.HasChanges() {
		user, err := s.repo.GetUserByID(ctx, trimmedID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) && opts.Username != nil {
				fallback, lookupErr := s.repo.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(*opts.Username)))
				if lookupErr == nil {
					return fallback, nil
				}
				if !errors.Is(lookupErr, sql.ErrNoRows) {
					return User{}, lookupErr
				}
			}
			if errors.Is(err, sql.ErrNoRows) {
				return User{}, ErrUserNotFound
			}
			return User{}, err
		}
		return user, nil
	}
	user, err := s.repo.UpdateUser(ctx, update)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Retry by username if provided, to handle stale IDs coming from UI caches.
			if opts.Username != nil {
				retryUser, lookupErr := s.repo.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(*opts.Username)))
				if lookupErr == nil {
					update.ID = retryUser.ID
					if user, updateErr := s.repo.UpdateUser(ctx, update); updateErr == nil {
						return user, nil
					}
					// if updateErr isn't nil, fall through to default handling
				} else if !errors.Is(lookupErr, sql.ErrNoRows) {
					return User{}, lookupErr
				}
			}
			return User{}, ErrUserNotFound
		}
		if isDuplicateEntryError(err) {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	return user, nil
}

// ListUserRoles returns all roles assigned to a user.
func (s *Service) ListUserRoles(ctx context.Context, userID string) ([]string, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("auth: repository unavailable")
	}
	id := strings.TrimSpace(userID)
	if id == "" {
		return nil, errors.New("auth: user id required")
	}
	if _, err := s.repo.GetUserByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	roles, err := s.repo.ListUserRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// ReplaceUserRoles fully replaces a user's role assignments.
func (s *Service) ReplaceUserRoles(ctx context.Context, userID string, roles []string) ([]string, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("auth: repository unavailable")
	}
	id := strings.TrimSpace(userID)
	if id == "" {
		return nil, errors.New("auth: user id required")
	}
	if _, err := s.repo.GetUserByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	dedup := make(map[string]struct{})
	var normalized []string
	for _, role := range roles {
		candidate := strings.TrimSpace(role)
		if candidate == "" {
			continue
		}
		trimmed := normalizeRole(candidate)
		if trimmed == "" {
			continue
		}
		if _, ok := dedup[trimmed]; ok {
			continue
		}
		dedup[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if err := s.repo.ReplaceUserRoles(ctx, id, normalized, s.now()); err != nil {
		return nil, err
	}
	return normalized, nil
}

// DeleteUser removes an identity from the repository.
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	if s == nil || s.repo == nil {
		return errors.New("auth: repository unavailable")
	}
	trimmedID := strings.TrimSpace(userID)
	if trimmedID == "" {
		return errors.New("auth: user id required")
	}
	if err := s.repo.DeleteUser(ctx, trimmedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}

func authMethodFromMetadata(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(payload, &meta); err != nil {
		return ""
	}
	if method, ok := meta["authMethod"].(string); ok {
		return strings.TrimSpace(method)
	}
	return ""
}

func sessionClientMetadataFromMetadata(payload json.RawMessage) (string, string) {
	if len(payload) == 0 {
		return "", ""
	}
	var meta map[string]any
	if err := json.Unmarshal(payload, &meta); err != nil {
		return "", ""
	}
	clientIP := ""
	if value, ok := meta["clientIp"].(string); ok {
		clientIP = strings.TrimSpace(value)
	}
	userAgent := ""
	if value, ok := meta["userAgent"].(string); ok {
		userAgent = strings.TrimSpace(value)
	}
	return clientIP, userAgent
}

func generateToken() (string, string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := "mdhcp_" + strings.ToLower(hex.EncodeToString(buf))
	prefix := token[:16]
	return token, prefix, nil
}

func generatePassword(length int) (string, error) {
	if length <= 0 {
		length = 24
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sqlNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return "reader"
	}
	switch role {
	case "admin", "reader":
		return role
	default:
		return "reader"
	}
}

func normalizeUserStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "disabled", "locked", "inactive":
		return "disabled"
	case "pending", "invite":
		return "pending"
	default:
		return "active"
	}
}

func sanitizeUserSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "displayname":
		return "display_name"
	case "createdat":
		return "created_at"
	case "updatedat":
		return "updated_at"
	case "lastloginat":
		return "last_login_at"
	case "status":
		return "status"
	default:
		return "username"
	}
}

func isDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate entry")
}

func stringToNull(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func encodeProviderConfig(config map[string]any) (json.RawMessage, error) {
	if config == nil {
		return nil, nil
	}
	if len(config) == 0 {
		return json.RawMessage([]byte("{}")), nil
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func decodeProviderConfig(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if len(cfg) == 0 {
		return nil, nil
	}
	return cfg, nil
}

func recordToProvider(record IdentityProviderRecord) (ManagedIdentityProvider, error) {
	cfg, err := decodeProviderConfig(record.Config)
	if err != nil {
		return ManagedIdentityProvider{}, err
	}
	provider := ManagedIdentityProvider{
		ID:        record.ID,
		Name:      record.Name,
		Type:      record.Type,
		Enabled:   record.Enabled,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
	if record.Endpoint.Valid {
		provider.Endpoint = record.Endpoint.String
	}
	if cfg != nil {
		provider.Config = cfg
	}
	return provider, nil
}

func normalizeProviderIdentifier(id string) string {
	lower := strings.ToLower(strings.TrimSpace(id))
	if lower == "" {
		return ""
	}
	var builder strings.Builder
	lastHyphen := false
	for _, r := range lower {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if valid {
			if r == '-' {
				if lastHyphen {
					continue
				}
				lastHyphen = true
			} else {
				lastHyphen = false
			}
			builder.WriteRune(r)
			continue
		}
		if r == ' ' || r == '.' {
			if lastHyphen {
				continue
			}
			builder.WriteRune('-')
			lastHyphen = true
		}
	}
	result := strings.Trim(builder.String(), "-_")
	return result
}

// SQLRepository implements Repository with sqlx.
type SQLRepository struct {
	db *sqlx.DB
}

// NewRepository constructs a SQL repository.
func NewRepository(db *sqlx.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) GetUserByUsername(ctx context.Context, username string) (User, error) {
	const query = `SELECT id, username, display_name, email, password_hash, role, status, must_change_password, last_login_at, created_at, updated_at FROM auth_users WHERE LOWER(username) = ? LIMIT 1`
	var user User
	if err := r.db.GetContext(ctx, &user, query, strings.ToLower(username)); err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *SQLRepository) GetUserByID(ctx context.Context, id string) (User, error) {
	const query = `SELECT id, username, display_name, email, password_hash, role, status, must_change_password, last_login_at, created_at, updated_at FROM auth_users WHERE id = ? LIMIT 1`
	var user User
	if err := r.db.GetContext(ctx, &user, query, id); err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *SQLRepository) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	const statement = `UPDATE auth_users SET last_login_at = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, statement, at, at, userID)
	return err
}

func (r *SQLRepository) UpdatePassword(ctx context.Context, userID string, passwordHash []byte, mustChange bool, updatedAt time.Time) error {
	const statement = `UPDATE auth_users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, statement, passwordHash, mustChange, updatedAt, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLRepository) CreateAPIKey(ctx context.Context, key *APIKey) error {
	const statement = `INSERT INTO auth_api_keys (id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, metadata) VALUES (:id, :name, :prefix, :token_hash, :role, :principal_id, :tenant_scope, :kind, :owner_user_id, :description, :created_by, :created_at, :expires_at, :last_used_at, :revoked_at, :revoked_by, :metadata)`
	_, err := r.db.NamedExecContext(ctx, statement, key)
	return err
}

func (r *SQLRepository) GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	const query = `SELECT id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, COALESCE(metadata, '{}') AS metadata FROM auth_api_keys WHERE token_hash = ? LIMIT 1`
	var key APIKey
	if err := r.db.GetContext(ctx, &key, query, hash); err != nil {
		return APIKey{}, err
	}
	return key, nil
}

func (r *SQLRepository) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error) {
	builder := strings.Builder{}
	builder.WriteString(`SELECT id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, COALESCE(metadata, '{}') AS metadata FROM auth_api_keys`)
	var clauses []string
	var args []any
	if filter.OwnerUserID != nil {
		clauses = append(clauses, "owner_user_id = ?")
		args = append(args, *filter.OwnerUserID)
	}
	if len(filter.Kinds) > 0 {
		placeholders := make([]string, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			placeholders = append(placeholders, "?")
			args = append(args, kind)
		}
		clauses = append(clauses, fmt.Sprintf("kind IN (%s)", strings.Join(placeholders, ",")))
	}
	if !filter.IncludeRevoked {
		clauses = append(clauses, "revoked_at IS NULL")
	}
	if len(clauses) > 0 {
		builder.WriteString(" WHERE ")
		builder.WriteString(strings.Join(clauses, " AND "))
	}
	builder.WriteString(" ORDER BY created_at DESC")
	query := builder.String()
	var keys []APIKey
	if err := r.db.SelectContext(ctx, &keys, query, args...); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *SQLRepository) UpdateAPIKey(ctx context.Context, update APIKeyUpdate) (APIKey, error) {
	sets := make([]string, 0, 8)
	args := make([]any, 0, 12)
	if update.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *update.Name)
	}
	if update.Role != nil {
		sets = append(sets, "role = ?")
		args = append(args, *update.Role)
	}
	if update.Description != nil {
		if strings.TrimSpace(*update.Description) == "" {
			sets = append(sets, "description = NULL")
		} else {
			sets = append(sets, "description = ?")
			args = append(args, strings.TrimSpace(*update.Description))
		}
	}
	if update.ExpiresAtSet {
		if update.ExpiresAt == nil {
			sets = append(sets, "expires_at = NULL")
		} else {
			sets = append(sets, "expires_at = ?")
			args = append(args, update.ExpiresAt.UTC())
		}
	}
	if update.MetadataSet {
		sets = append(sets, "metadata = ?")
		args = append(args, update.Metadata)
	}
	if update.RevokeSet {
		if update.RevokedAt == nil {
			sets = append(sets, "revoked_at = NULL", "revoked_by = NULL")
		} else {
			sets = append(sets, "revoked_at = ?", "revoked_by = ?")
			args = append(args, update.RevokedAt.UTC())
			if update.RevokedBy != nil {
				args = append(args, strings.TrimSpace(*update.RevokedBy))
			} else {
				args = append(args, "")
			}
		}
	}
	if len(sets) == 0 {
		return APIKey{}, errors.New("auth: no api key changes supplied")
	}
	query := "UPDATE auth_api_keys SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	args = append(args, update.ID)
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return APIKey{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return APIKey{}, err
	}
	if rows == 0 {
		return APIKey{}, sql.ErrNoRows
	}
	const getByID = `SELECT id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, COALESCE(metadata, '{}') AS metadata FROM auth_api_keys WHERE id = ? LIMIT 1`
	var key APIKey
	if err := r.db.GetContext(ctx, &key, getByID, update.ID); err != nil {
		return APIKey{}, err
	}
	return key, nil
}

func (r *SQLRepository) RevokeAPIKey(ctx context.Context, id string, revokedBy string, at time.Time) error {
	const statement = `UPDATE auth_api_keys SET revoked_at = ?, revoked_by = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, statement, at, revokedBy, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLRepository) MarkAPIKeyUsed(ctx context.Context, id string, at time.Time) error {
	const statement = `UPDATE auth_api_keys SET last_used_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, statement, at, id)
	return err
}

func (r *SQLRepository) ListUsers(ctx context.Context, filter UserFilter) ([]User, int, error) {
	base := `FROM auth_users`
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if filter.Status != "" {
		clauses = append(clauses, "LOWER(status) = ?")
		args = append(args, filter.Status)
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		clauses = append(clauses, "(LOWER(username) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(email) LIKE ?)")
		args = append(args, like, like, like)
	}
	var where string
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	countQuery := "SELECT COUNT(*) " + base + where
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []User{}, 0, nil
	}
	column := filter.SortBy
	if column == "" {
		column = "username"
	}
	order := "ASC"
	if filter.SortDesc {
		order = "DESC"
	}
	selectQuery := "SELECT id, username, display_name, email, password_hash, role, status, must_change_password, last_login_at, created_at, updated_at " + base + where + " ORDER BY " + column + " " + order + " LIMIT ? OFFSET ?"
	argsList := append([]any{}, args...)
	argsList = append(argsList, filter.Limit, filter.Offset)
	var users []User
	if err := r.db.SelectContext(ctx, &users, selectQuery, argsList...); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *SQLRepository) CreateUser(ctx context.Context, user *User) error {
	const statement = `INSERT INTO auth_users (id, username, display_name, email, password_hash, role, status, must_change_password, last_login_at, created_at, updated_at) VALUES (:id, :username, :display_name, :email, :password_hash, :role, :status, :must_change_password, :last_login_at, :created_at, :updated_at)`
	_, err := r.db.NamedExecContext(ctx, statement, user)
	return err
}

func (r *SQLRepository) ListUserRoles(ctx context.Context, userID string) ([]string, error) {
	const query = `SELECT role FROM auth_user_roles WHERE user_id = ? ORDER BY role`
	var roles []string
	if err := r.db.SelectContext(ctx, &roles, query, userID); err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *SQLRepository) ReplaceUserRoles(ctx context.Context, userID string, roles []string, now time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM auth_user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if len(roles) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO auth_user_roles (id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, role := range roles {
		if _, err = stmt.ExecContext(ctx, uuid.NewString(), userID, role, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLRepository) UpdateUser(ctx context.Context, update UserUpdate) (User, error) {
	if strings.TrimSpace(update.ID) == "" {
		return User{}, errors.New("auth: user id required")
	}
	setClauses := []string{"updated_at = ?"}
	args := []any{update.UpdatedAt}
	if update.DisplayName != nil {
		setClauses = append(setClauses, "display_name = ?")
		args = append(args, *update.DisplayName)
	}
	if update.Email != nil {
		email := strings.TrimSpace(*update.Email)
		if email == "" {
			setClauses = append(setClauses, "email = NULL")
		} else {
			setClauses = append(setClauses, "email = ?")
			args = append(args, email)
		}
	}
	if update.Role != nil {
		setClauses = append(setClauses, "role = ?")
		args = append(args, *update.Role)
	}
	if update.Status != nil {
		setClauses = append(setClauses, "status = ?")
		args = append(args, *update.Status)
	}
	if update.MustChangePassword != nil {
		setClauses = append(setClauses, "must_change_password = ?")
		args = append(args, *update.MustChangePassword)
	}
	if update.ResetPassword {
		setClauses = append(setClauses, "password_hash = ?")
		args = append(args, update.PasswordHash)
	}
	statement := "UPDATE auth_users SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	args = append(args, update.ID)
	result, err := r.db.ExecContext(ctx, statement, args...)
	if err != nil {
		return User{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if rows == 0 {
		return User{}, sql.ErrNoRows
	}
	return r.GetUserByID(ctx, update.ID)
}

func (r *SQLRepository) DeleteUser(ctx context.Context, id string) error {
	const statement = `DELETE FROM auth_users WHERE id = ?`
	result, err := r.db.ExecContext(ctx, statement, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLRepository) ListIdentityProviders(ctx context.Context) ([]IdentityProviderRecord, error) {
	const query = `SELECT id, name, type, endpoint, config, enabled, created_at, updated_at FROM auth_identity_providers ORDER BY created_at DESC`
	var records []IdentityProviderRecord
	if err := r.db.SelectContext(ctx, &records, query); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *SQLRepository) GetIdentityProvider(ctx context.Context, id string) (IdentityProviderRecord, error) {
	const query = `SELECT id, name, type, endpoint, config, enabled, created_at, updated_at FROM auth_identity_providers WHERE id = ? LIMIT 1`
	var record IdentityProviderRecord
	if err := r.db.GetContext(ctx, &record, query, id); err != nil {
		return IdentityProviderRecord{}, err
	}
	return record, nil
}

func (r *SQLRepository) CreateIdentityProvider(ctx context.Context, provider IdentityProviderRecord) error {
	const statement = `INSERT INTO auth_identity_providers (id, name, type, endpoint, config, enabled, created_at, updated_at) VALUES (:id, :name, :type, :endpoint, :config, :enabled, :created_at, :updated_at)`
	_, err := r.db.NamedExecContext(ctx, statement, provider)
	return err
}

func (r *SQLRepository) UpdateIdentityProvider(ctx context.Context, update IdentityProviderUpdate) (IdentityProviderRecord, error) {
	if strings.TrimSpace(update.ID) == "" {
		return IdentityProviderRecord{}, errors.New("auth: provider id required")
	}
	setClauses := []string{"updated_at = ?"}
	args := []any{update.UpdatedAt}
	if update.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *update.Name)
	}
	if update.Endpoint != nil {
		trimmed := strings.TrimSpace(*update.Endpoint)
		if trimmed == "" {
			setClauses = append(setClauses, "endpoint = NULL")
		} else {
			setClauses = append(setClauses, "endpoint = ?")
			args = append(args, trimmed)
		}
	}
	if update.ConfigSet {
		if len(update.Config) == 0 {
			setClauses = append(setClauses, "config = NULL")
		} else {
			setClauses = append(setClauses, "config = ?")
			args = append(args, update.Config)
		}
	}
	if update.Enabled != nil {
		setClauses = append(setClauses, "enabled = ?")
		args = append(args, *update.Enabled)
	}
	statement := "UPDATE auth_identity_providers SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	args = append(args, update.ID)
	result, err := r.db.ExecContext(ctx, statement, args...)
	if err != nil {
		return IdentityProviderRecord{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return IdentityProviderRecord{}, err
	}
	if rows == 0 {
		return IdentityProviderRecord{}, sql.ErrNoRows
	}
	return r.GetIdentityProvider(ctx, update.ID)
}
