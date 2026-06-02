package rbac

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/resource"
)

// ResolverOptions tune runtime behavior for capability resolution.
type ResolverOptions struct {
	CacheTTL time.Duration
	Logger   *zap.Logger
}

// PrincipalContext represents the authenticated principal and their access scope.
type PrincipalContext struct {
	UserID    string               `json:"userId"`
	SessionID string               `json:"sessionId,omitempty"`
	Scope     resource.AccessScope `json:"scope"`
	IssuedAt  time.Time            `json:"issuedAt"`
}

// PrincipalContextOption customizes a PrincipalContext during creation.
type PrincipalContextOption func(*PrincipalContext)

// NewPrincipalContext constructs a PrincipalContext with sane defaults.
func NewPrincipalContext(userID string, scope resource.AccessScope, opts ...PrincipalContextOption) PrincipalContext {
	ctx := PrincipalContext{UserID: strings.TrimSpace(userID), Scope: scope, IssuedAt: time.Now().UTC()}
	if ctx.Scope.Principal == "" {
		ctx.Scope.Principal = ctx.UserID
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&ctx)
		}
	}
	return ctx
}

// WithPrincipalSession sets the session identifier.
func WithPrincipalSession(sessionID string) PrincipalContextOption {
	return func(ctx *PrincipalContext) {
		ctx.SessionID = strings.TrimSpace(sessionID)
	}
}

// WithPrincipalIssuedAt overrides the issued-at timestamp.
func WithPrincipalIssuedAt(ts time.Time) PrincipalContextOption {
	return func(ctx *PrincipalContext) {
		if ts.IsZero() {
			return
		}
		ctx.IssuedAt = ts
	}
}

// EffectiveScope guarantees the contained AccessScope carries the principal ID.
func (p PrincipalContext) EffectiveScope() resource.AccessScope {
	scope := p.Scope
	if scope.Principal == "" {
		scope.Principal = p.UserID
	}
	return scope
}

// ResolveOptions describes the tenant/org/resource scope of a request.
type ResolveOptions struct {
	OrgUnitID string
	Now       time.Time
	Principal PrincipalContext
}

// Resolver evaluates assignments, inheritance, and temporary grants for a principal.
type Resolver struct {
	repo      Repository
	logger    *zap.Logger
	cacheTTL  time.Duration
	mu        sync.RWMutex
	roleCache map[string]roleCacheEntry
}

type roleCacheEntry struct {
	role     Role
	loadedAt time.Time
}

// NewResolver creates a capability resolver bound to a repository.
func NewResolver(repo Repository, opts ResolverOptions) *Resolver {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = time.Minute
	}
	return &Resolver{
		repo:      repo,
		logger:    logger,
		cacheTTL:  cacheTTL,
		roleCache: make(map[string]roleCacheEntry),
	}
}

// Resolve builds the effective capability set for the principal under the given scope.
func (r *Resolver) Resolve(ctx context.Context, principalID string, opts ResolveOptions) (Resolution, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	assignments, err := r.repo.ListAssignments(ctx, principalID)
	if err != nil {
		return Resolution{}, err
	}
	grants, err := r.repo.ListActiveTempGrants(ctx, principalID, opts.Now)
	if err != nil {
		return Resolution{}, err
	}
	capabilities := make(map[string]struct{})
	grantDetails := make([]GrantEnvelope, 0, len(assignments)+len(grants))
	for _, assignment := range assignments {
		if !r.matchesScope(assignment, opts) {
			continue
		}
		caps, err := r.capabilitiesForRole(ctx, assignment.RoleName)
		if err != nil {
			return Resolution{}, err
		}
		if len(caps) == 0 {
			continue
		}
		for _, capName := range caps {
			capabilities[capName] = struct{}{}
		}
		grantDetails = append(grantDetails, GrantEnvelope{Assignment: assignment, Capabilities: caps})
	}
	for _, grant := range grants {
		assignment, ok := findAssignmentByID(assignments, grant.AssignmentID)
		if !ok {
			continue
		}
		if !r.matchesScope(assignment, opts) {
			continue
		}
		if grant.Status != "approved" && grant.Status != "pending" {
			continue
		}
		caps, err := r.capabilitiesForRole(ctx, assignment.RoleName)
		if err != nil {
			return Resolution{}, err
		}
		for _, capName := range caps {
			capabilities["temp."+capName] = struct{}{}
		}
		grantDetails = append(grantDetails, GrantEnvelope{Assignment: assignment, TempGrant: &grant, Capabilities: caps})
	}
	return Resolution{
		PrincipalID:  principalID,
		OrgUnitID:    opts.OrgUnitID,
		Capabilities: capabilities,
		Grants:       grantDetails,
	}, nil
}

func (r *Resolver) capabilitiesForRole(ctx context.Context, roleName string) ([]string, error) {
	role, err := r.loadRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	capSet := make(map[string]struct{})
	r.collectCapabilities(ctx, role, capSet)
	augmentUserCapabilities(capSet)
	result := make([]string, 0, len(capSet))
	for capName := range capSet {
		result = append(result, capName)
	}
	return result, nil
}

func (r *Resolver) collectCapabilities(ctx context.Context, role Role, dst map[string]struct{}) {
	for _, capName := range role.Capabilities {
		dst[capName] = struct{}{}
	}
	if role.InheritsFrom == nil || *role.InheritsFrom == "" {
		return
	}
	parent, err := r.loadRole(ctx, *role.InheritsFrom)
	if err != nil {
		r.logger.Warn("rbac resolver: inherited role lookup failed", zap.String("role", role.Name), zap.Error(err))
		return
	}
	r.collectCapabilities(ctx, parent, dst)
}

func (r *Resolver) loadRole(ctx context.Context, roleName string) (Role, error) {
	r.mu.RLock()
	entry, ok := r.roleCache[roleName]
	r.mu.RUnlock()
	if ok && time.Since(entry.loadedAt) < r.cacheTTL {
		return entry.role, nil
	}
	role, err := r.repo.GetRole(ctx, roleName)
	if err != nil {
		return Role{}, err
	}
	r.mu.Lock()
	r.roleCache[roleName] = roleCacheEntry{role: role, loadedAt: time.Now().UTC()}
	r.mu.Unlock()
	return role, nil
}

func (r *Resolver) matchesScope(assignment Assignment, opts ResolveOptions) bool {
	assignment.HydrateScope()
	scope := opts.Principal.EffectiveScope()
	if !assignment.Scope.MatchesAccessScope(scope) {
		return false
	}
	return matchesOrgScope(assignment, opts)
}

func matchesOrgScope(assignment Assignment, opts ResolveOptions) bool {
	org := strings.TrimSpace(opts.OrgUnitID)
	if org == "" {
		return true
	}
	if assignment.OrgUnitID == nil || strings.TrimSpace(*assignment.OrgUnitID) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*assignment.OrgUnitID), org)
}

func findAssignmentByID(assignments []Assignment, id string) (Assignment, bool) {
	for _, assignment := range assignments {
		if assignment.ID == id {
			return assignment, true
		}
	}
	return Assignment{}, false
}

func augmentUserCapabilities(capabilities map[string]struct{}) {
	if capabilities == nil {
		return
	}
	if _, ok := capabilities[CapabilityRBACAssignmentRead]; ok {
		capabilities[CapabilityRBACRoleRead] = struct{}{}
		capabilities[CapabilityUserRead] = struct{}{}
	}
	if _, ok := capabilities[CapabilityRBACAssignmentWrite]; ok {
		capabilities[CapabilityRBACRoleManage] = struct{}{}
		capabilities[CapabilityRBACRoleRead] = struct{}{}
		capabilities[CapabilityUserManage] = struct{}{}
		capabilities[CapabilityUserRead] = struct{}{}
	}
	if _, ok := capabilities[CapabilityUserRead]; ok {
		capabilities[CapabilityAuthProviderRead] = struct{}{}
	}
	if _, ok := capabilities[CapabilityUserManage]; ok {
		capabilities[CapabilityAuthProviderManage] = struct{}{}
		capabilities[CapabilityAuthProviderRead] = struct{}{}
	}
	if _, ok := capabilities[CapabilityRBACRoleManage]; ok {
		capabilities[CapabilityRBACRoleRead] = struct{}{}
	}
}
