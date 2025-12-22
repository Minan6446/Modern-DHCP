package auth

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ErrProviderNotFound indicates the requested provider is unavailable.
var ErrProviderNotFound = errors.New("auth: identity provider not found")

// ProviderRequest captures the common attributes shared across providers.
type ProviderRequest struct {
	Username   string
	Password   string
	MFACode    string
	TenantHint string
	Attributes map[string]string
	Claims     map[string]any
}

// ProviderMetadata exposes provider-specific attributes.
type ProviderMetadata map[string]any

// ProviderResult returns the provider-authenticated identity.
type ProviderResult struct {
	User     User
	Method   string
	Metadata ProviderMetadata
}

// IdentityProvider authenticates a subject and maps to a console identity.
type IdentityProvider interface {
	Name() string
	Authenticate(ctx context.Context, req ProviderRequest) (User, ProviderMetadata, error)
}

type providerRegistration struct {
	makeDefault bool
}

// ProviderOption mutates provider registration behavior.
type ProviderOption func(*providerRegistration)

// WithProviderDefault marks the provider as the default auth path.
func WithProviderDefault() ProviderOption {
	return func(reg *providerRegistration) {
		if reg != nil {
			reg.makeDefault = true
		}
	}
}

type localProvider struct {
	repo   Repository
	logger *zap.Logger
	clock  func() time.Time
}

func newLocalProvider(repo Repository, logger *zap.Logger, clock func() time.Time) IdentityProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if clock == nil {
		clock = time.Now().UTC
	}
	return &localProvider{repo: repo, logger: logger, clock: clock}
}

func (p *localProvider) Name() string {
	return "password"
}

func (p *localProvider) Authenticate(ctx context.Context, req ProviderRequest) (User, ProviderMetadata, error) {
	if p == nil || p.repo == nil {
		return User{}, nil, ErrInvalidCredentials
	}
	username := strings.TrimSpace(strings.ToLower(req.Username))
	if username == "" || strings.TrimSpace(req.Password) == "" {
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
	if bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(req.Password)) != nil {
		return User{}, nil, ErrInvalidCredentials
	}
	now := p.clock()
	if err := p.repo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		p.logger.Warn("auth: update last login failed", zap.String("userId", user.ID), zap.Error(err))
	}
	return user, ProviderMetadata{"authMethod": p.Name()}, nil
}

// LDAPProviderOptions configures the LDAP identity provider.
type LDAPProviderOptions struct {
	URL                  string
	BindDN               string
	BindPassword         string
	UserBaseDN           string
	UserFilter           string
	UsernameAttribute    string
	DisplayNameAttribute string
	EmailAttribute       string
	UseStartTLS          bool
	SkipTLSVerify        bool
	Timeout              time.Duration
}

func (opts LDAPProviderOptions) normalize() LDAPProviderOptions {
	opts.UserBaseDN = strings.TrimSpace(opts.UserBaseDN)
	if strings.TrimSpace(opts.UserFilter) == "" {
		opts.UserFilter = "(uid={username})"
	}
	if strings.TrimSpace(opts.UsernameAttribute) == "" {
		opts.UsernameAttribute = "uid"
	}
	if strings.TrimSpace(opts.DisplayNameAttribute) == "" {
		opts.DisplayNameAttribute = "cn"
	}
	if strings.TrimSpace(opts.EmailAttribute) == "" {
		opts.EmailAttribute = "mail"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	return opts
}

// NewLDAPProvider builds an LDAP-backed identity provider.
func NewLDAPProvider(repo Repository, logger *zap.Logger, opts LDAPProviderOptions) (IdentityProvider, error) {
	if repo == nil {
		return nil, errors.New("auth: ldap provider requires repository")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	opts = opts.normalize()
	if strings.TrimSpace(opts.URL) == "" {
		return nil, errors.New("auth: ldap url is required")
	}
	if opts.UserBaseDN == "" {
		return nil, errors.New("auth: ldap userBaseDN is required")
	}
	return &ldapProvider{
		repo:   repo,
		logger: logger,
		opts:   opts,
		clock:  time.Now().UTC,
	}, nil
}

type ldapProvider struct {
	repo   Repository
	logger *zap.Logger
	opts   LDAPProviderOptions
	clock  func() time.Time
}

func (p *ldapProvider) Name() string {
	return "ldap"
}

func (p *ldapProvider) Authenticate(ctx context.Context, req ProviderRequest) (User, ProviderMetadata, error) {
	if p == nil || p.repo == nil {
		return User{}, nil, ErrInvalidCredentials
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return User{}, nil, err
	}
	username := strings.TrimSpace(strings.ToLower(req.Username))
	if username == "" || req.Password == "" {
		return User{}, nil, ErrInvalidCredentials
	}
	conn, err := p.dial(ctx)
	if err != nil {
		p.logger.Warn("auth: ldap dial failed", zap.Error(err))
		return User{}, nil, ErrInvalidCredentials
	}
	defer conn.Close()
	if strings.TrimSpace(p.opts.BindDN) != "" {
		if err := conn.Bind(p.opts.BindDN, p.opts.BindPassword); err != nil {
			p.logger.Warn("auth: ldap bind failed", zap.Error(err))
			return User{}, nil, ErrInvalidCredentials
		}
	}
	searchReq := ldap.NewSearchRequest(
		p.opts.UserBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,
		0,
		false,
		p.filter(username),
		p.requestedAttributes(),
		nil,
	)
	searchRes, err := conn.Search(searchReq)
	if err != nil || len(searchRes.Entries) == 0 {
		return User{}, nil, ErrInvalidCredentials
	}
	entry := searchRes.Entries[0]
	if err := conn.Bind(entry.DN, req.Password); err != nil {
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
	if display := strings.TrimSpace(entry.GetAttributeValue(p.opts.DisplayNameAttribute)); display != "" {
		user.DisplayName = display
	}
	now := p.clock()
	if err := p.repo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		p.logger.Warn("auth: ldap update last login failed", zap.String("userId", user.ID), zap.Error(err))
	}
	metadata := ProviderMetadata{
		"authMethod":  p.Name(),
		"directoryDN": entry.DN,
	}
	if mail := strings.TrimSpace(entry.GetAttributeValue(p.opts.EmailAttribute)); mail != "" {
		metadata["email"] = mail
	}
	return user, metadata, nil
}

func (p *ldapProvider) requestTimeout(ctx context.Context) time.Duration {
	deadlineTimeout := p.opts.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && (deadlineTimeout <= 0 || remaining < deadlineTimeout) {
			deadlineTimeout = remaining
		}
	}
	if deadlineTimeout <= 0 {
		deadlineTimeout = 5 * time.Second
	}
	return deadlineTimeout
}

func (p *ldapProvider) dial(ctx context.Context) (*ldap.Conn, error) {
	timeout := p.requestTimeout(ctx)
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := ldap.DialURL(p.opts.URL,
		ldap.DialWithDialer(dialer),
		ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: p.opts.SkipTLSVerify}))
	if err != nil {
		return nil, err
	}
	if p.opts.UseStartTLS {
		tlsConfig := &tls.Config{InsecureSkipVerify: p.opts.SkipTLSVerify}
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (p *ldapProvider) filter(username string) string {
	escaped := ldap.EscapeFilter(username)
	return strings.ReplaceAll(p.opts.UserFilter, "{username}", escaped)
}

func (p *ldapProvider) requestedAttributes() []string {
	attrs := map[string]struct{}{
		p.opts.DisplayNameAttribute: {},
		p.opts.EmailAttribute:       {},
	}
	list := make([]string, 0, len(attrs))
	for attr := range attrs {
		trimmed := strings.TrimSpace(attr)
		if trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}
