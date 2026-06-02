package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRefreshSessionRotatesAndRevokes(t *testing.T) {
	repo := newMemoryRepo()
	user := User{ID: "11111111-1111-1111-1111-111111111111", Username: "alice", Role: "reader", Status: "active"}
	repo.users[user.ID] = user
	svc := NewService(repo, nil, WithSessionTTL(time.Hour))

	session, err := svc.IssueSession(context.Background(), user, SessionOptions{TenantID: "t1", AuthMethod: "oidc", ClientIP: "10.0.0.8", UserAgent: "Mozilla/5.0"})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	rotated, refreshedUser, tenantID, authMethod, err := svc.RefreshSession(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if rotated.Token == session.Token {
		t.Fatalf("expected new token after refresh")
	}
	if refreshedUser.ID != user.ID {
		t.Fatalf("unexpected user returned: %s", refreshedUser.ID)
	}
	if tenantID != "t1" {
		t.Fatalf("tenant scope should persist, got %s", tenantID)
	}
	if authMethod != "oidc" {
		t.Fatalf("auth method should persist, got %s", authMethod)
	}

	rotatedKey, err := repo.GetAPIKeyByHash(context.Background(), hashToken(rotated.Token))
	if err != nil {
		t.Fatalf("load rotated key: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(rotatedKey.Metadata, &meta); err != nil {
		t.Fatalf("decode rotated metadata: %v", err)
	}
	if got, _ := meta["clientIp"].(string); got != "10.0.0.8" {
		t.Fatalf("clientIp should persist, got %q", got)
	}
	if got, _ := meta["userAgent"].(string); got != "Mozilla/5.0" {
		t.Fatalf("userAgent should persist, got %q", got)
	}

	// Old token should now be revoked for lookups.
	if _, err := svc.LookupToken(context.Background(), session.Token); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("expected revoked error for old token, got %v", err)
	}
}

func TestRevokeTokenByPlaintext(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo, nil)
	expiry := time.Now().Add(24 * time.Hour)
	owner := "11111111-1111-1111-1111-111111111111"
	// Seed a key manually.
	keyToken, key, err := svc.CreateAPIKey(context.Background(), "svc", "admin", "principal", "", "system", "", nil, &expiry, &owner)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if keyToken == "" || key.ID == "" {
		t.Fatalf("expected token and id")
	}
	if err := svc.RevokeToken(context.Background(), keyToken, "tester"); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if _, err := svc.LookupToken(context.Background(), keyToken); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("expected revoked after revoke, got %v", err)
	}
}

// memoryRepo is a minimal in-memory Repository for tests.
type memoryRepo struct {
	users     map[string]User
	keys      map[string]APIKey
	userRoles map[string][]string
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		users:     make(map[string]User),
		keys:      make(map[string]APIKey),
		userRoles: make(map[string][]string),
	}
}

func (r *memoryRepo) GetUserByUsername(ctx context.Context, username string) (User, error) {
	for _, u := range r.users {
		if strings.EqualFold(u.Username, username) {
			return u, nil
		}
	}
	return User{}, sql.ErrNoRows
}

func (r *memoryRepo) GetUserByID(ctx context.Context, id string) (User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return User{}, sql.ErrNoRows
}

func (r *memoryRepo) ListUsers(ctx context.Context, filter UserFilter) ([]User, int, error) {
	list := make([]User, 0, len(r.users))
	for _, u := range r.users {
		list = append(list, u)
	}
	return list, len(list), nil
}

func (r *memoryRepo) CreateUser(ctx context.Context, user *User) error {
	for _, u := range r.users {
		if strings.EqualFold(u.Username, user.Username) {
			return ErrUserExists
		}
	}
	r.users[user.ID] = *user
	return nil
}

func (r *memoryRepo) UpdateUser(ctx context.Context, update UserUpdate) (User, error) {
	user, ok := r.users[update.ID]
	if !ok {
		return User{}, sql.ErrNoRows
	}
	if update.DisplayName != nil {
		user.DisplayName = *update.DisplayName
	}
	if update.Email != nil {
		user.Email = stringToNull(*update.Email)
	}
	if update.Role != nil {
		user.Role = *update.Role
	}
	if update.Status != nil {
		user.Status = *update.Status
	}
	if update.MustChangePassword != nil {
		user.MustChangePassword = *update.MustChangePassword
	}
	if update.ResetPassword {
		user.PasswordHash = update.PasswordHash
	}
	r.users[user.ID] = user
	return user, nil
}

func (r *memoryRepo) DeleteUser(ctx context.Context, id string) error {
	if _, ok := r.users[id]; !ok {
		return sql.ErrNoRows
	}
	delete(r.users, id)
	return nil
}

func (r *memoryRepo) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	user, ok := r.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	user.LastLoginAt = sql.NullTime{Time: at, Valid: true}
	r.users[userID] = user
	return nil
}

func (r *memoryRepo) UpdatePassword(ctx context.Context, userID string, passwordHash []byte, mustChange bool, updatedAt time.Time) error {
	user, ok := r.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	user.PasswordHash = passwordHash
	user.MustChangePassword = mustChange
	r.users[userID] = user
	return nil
}

func (r *memoryRepo) CreateAPIKey(ctx context.Context, key *APIKey) error {
	r.keys[key.ID] = *key
	return nil
}

func (r *memoryRepo) GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	for _, key := range r.keys {
		if key.TokenHash == hash {
			return key, nil
		}
	}
	return APIKey{}, sql.ErrNoRows
}

func (r *memoryRepo) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]APIKey, error) {
	list := make([]APIKey, 0, len(r.keys))
	for _, key := range r.keys {
		list = append(list, key)
	}
	return list, nil
}

func (r *memoryRepo) RevokeAPIKey(ctx context.Context, id string, revokedBy string, at time.Time) error {
	key, ok := r.keys[id]
	if !ok {
		return sql.ErrNoRows
	}
	key.RevokedAt = sql.NullTime{Time: at, Valid: true}
	key.RevokedBy = sql.NullString{String: revokedBy, Valid: revokedBy != ""}
	r.keys[id] = key
	return nil
}

func (r *memoryRepo) UpdateAPIKey(ctx context.Context, update APIKeyUpdate) (APIKey, error) {
	key, ok := r.keys[update.ID]
	if !ok {
		return APIKey{}, sql.ErrNoRows
	}
	if update.Name != nil {
		key.Name = *update.Name
	}
	if update.Role != nil {
		key.Role = *update.Role
	}
	if update.Description != nil {
		desc := strings.TrimSpace(*update.Description)
		if desc == "" {
			key.Description = sql.NullString{}
		} else {
			key.Description = sql.NullString{String: desc, Valid: true}
		}
	}
	if update.ExpiresAtSet {
		if update.ExpiresAt == nil {
			key.ExpiresAt = sql.NullTime{}
		} else {
			key.ExpiresAt = sql.NullTime{Time: update.ExpiresAt.UTC(), Valid: true}
		}
	}
	if update.MetadataSet {
		key.Metadata = append([]byte(nil), update.Metadata...)
	}
	if update.RevokeSet {
		if update.RevokedAt == nil {
			key.RevokedAt = sql.NullTime{}
			key.RevokedBy = sql.NullString{}
		} else {
			key.RevokedAt = sql.NullTime{Time: update.RevokedAt.UTC(), Valid: true}
			if update.RevokedBy != nil {
				by := strings.TrimSpace(*update.RevokedBy)
				key.RevokedBy = sql.NullString{String: by, Valid: by != ""}
			} else {
				key.RevokedBy = sql.NullString{}
			}
		}
	}
	r.keys[update.ID] = key
	return key, nil
}

func (r *memoryRepo) MarkAPIKeyUsed(ctx context.Context, id string, at time.Time) error {
	key, ok := r.keys[id]
	if !ok {
		return sql.ErrNoRows
	}
	key.LastUsedAt = sql.NullTime{Time: at, Valid: true}
	r.keys[id] = key
	return nil
}

func (r *memoryRepo) ListIdentityProviders(ctx context.Context) ([]IdentityProviderRecord, error) {
	return nil, nil
}

func (r *memoryRepo) GetIdentityProvider(ctx context.Context, id string) (IdentityProviderRecord, error) {
	return IdentityProviderRecord{}, sql.ErrNoRows
}

func (r *memoryRepo) CreateIdentityProvider(ctx context.Context, provider IdentityProviderRecord) error {
	return nil
}

func (r *memoryRepo) UpdateIdentityProvider(ctx context.Context, update IdentityProviderUpdate) (IdentityProviderRecord, error) {
	return IdentityProviderRecord{}, sql.ErrNoRows
}

func (r *memoryRepo) ListUserRoles(ctx context.Context, userID string) ([]string, error) {
	roles := r.userRoles[userID]
	return append([]string(nil), roles...), nil
}

func (r *memoryRepo) ReplaceUserRoles(ctx context.Context, userID string, roles []string, now time.Time) error {
	r.userRoles[userID] = append([]string(nil), roles...)
	return nil
}
