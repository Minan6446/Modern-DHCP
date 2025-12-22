package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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
)

// User models a console identity stored in auth_users.
type User struct {
	ID                 string         `db:"id"`
	Username           string         `db:"username"`
	DisplayName        string         `db:"display_name"`
	Email              sql.NullString `db:"email"`
	PasswordHash       []byte         `db:"password_hash"`
	Role               string         `db:"role"`
	TenantID           string         `db:"tenant_id"`
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

// Repository defines storage operations required by the service.
type Repository interface {
	GetUserByUsername(ctx context.Context, username string) (User, error)
	GetUserByID(ctx context.Context, id string) (User, error)
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	UpdatePassword(ctx context.Context, userID string, passwordHash []byte, mustChange bool, updatedAt time.Time) error
	CreateAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error)
	ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error)
	RevokeAPIKey(ctx context.Context, id string, revokedBy string, at time.Time) error
	MarkAPIKeyUsed(ctx context.Context, id string, at time.Time) error
}

// APIKeyFilter filters list operations.
type APIKeyFilter struct {
	OwnerUserID    *string
	IncludeRevoked bool
	Kinds          []string
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
	if strings.TrimSpace(opts.AuthMethod) != "" {
		meta := map[string]any{"authMethod": strings.ToLower(strings.TrimSpace(opts.AuthMethod))}
		if payload, err := json.Marshal(meta); err == nil {
			key.Metadata = payload
		}
	}
	if opts.TenantID != "" {
		key.TenantScope = sql.NullString{String: opts.TenantID, Valid: true}
	}
	if user.ID != "" {
		key.OwnerUserID = sql.NullString{String: user.ID, Valid: true}
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
	session, err := s.IssueSession(ctx, user, SessionOptions{TenantID: tenantScope, AuthMethod: authMethod})
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
	if ownerUserID != nil && *ownerUserID != "" {
		key.OwnerUserID = sql.NullString{String: *ownerUserID, Valid: true}
	}
	if expiresAt != nil {
		key.ExpiresAt = sqlNullTime(*expiresAt)
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

func generateToken() (string, string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := "mdhcp_" + strings.ToLower(hex.EncodeToString(buf))
	prefix := token[:16]
	return token, prefix, nil
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

// SQLRepository implements Repository with sqlx.
type SQLRepository struct {
	db *sqlx.DB
}

// NewRepository constructs a SQL repository.
func NewRepository(db *sqlx.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) GetUserByUsername(ctx context.Context, username string) (User, error) {
	const query = `SELECT id, username, display_name, email, password_hash, role, tenant_id, status, must_change_password, last_login_at, created_at, updated_at FROM auth_users WHERE LOWER(username) = ? LIMIT 1`
	var user User
	if err := r.db.GetContext(ctx, &user, query, strings.ToLower(username)); err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *SQLRepository) GetUserByID(ctx context.Context, id string) (User, error) {
	const query = `SELECT id, username, display_name, email, password_hash, role, tenant_id, status, must_change_password, last_login_at, created_at, updated_at FROM auth_users WHERE id = ? LIMIT 1`
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
	const query = `SELECT id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, metadata FROM auth_api_keys WHERE token_hash = ? LIMIT 1`
	var key APIKey
	if err := r.db.GetContext(ctx, &key, query, hash); err != nil {
		return APIKey{}, err
	}
	return key, nil
}

func (r *SQLRepository) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error) {
	builder := strings.Builder{}
	builder.WriteString(`SELECT id, name, prefix, token_hash, role, principal_id, tenant_scope, kind, owner_user_id, description, created_by, created_at, expires_at, last_used_at, revoked_at, revoked_by, metadata FROM auth_api_keys`)
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
