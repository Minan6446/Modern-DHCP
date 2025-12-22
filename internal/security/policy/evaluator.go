package policy

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EvaluatorOptions tune caching behavior for the policy evaluator.
type EvaluatorOptions struct {
	CacheTTL time.Duration
	Logger   *zap.Logger
}

// RuleEvaluator caches compiled policy rules per tenant.
type RuleEvaluator struct {
	repo   Repository
	logger *zap.Logger
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

var _ Evaluator = (*RuleEvaluator)(nil)

// NewEvaluator constructs a caching evaluator backed by the repository.
func NewEvaluator(repo Repository, opts EvaluatorOptions) *RuleEvaluator {
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RuleEvaluator{
		repo:   repo,
		logger: logger,
		ttl:    ttl,
		cache:  make(map[string]cacheEntry),
	}
}

// Evaluate finds the highest priority rule that matches the context.
func (e *RuleEvaluator) Evaluate(ctx context.Context, tenantID string, input EvaluationContext) Decision {
	if tenantID == "" {
		return Decision{Effect: EffectAllow}
	}
	rules := e.loadRules(ctx, tenantID)
	if len(rules) == 0 {
		return Decision{Effect: EffectAllow}
	}
	compiledCtx := compileEvaluationContext(input)
	for _, rule := range rules {
		if !rule.data.Enabled {
			continue
		}
		if rule.matchesContext(compiledCtx) {
			return Decision{Matched: true, RuleID: rule.data.ID, Effect: rule.data.Effect, Name: rule.data.Name}
		}
	}
	return Decision{Effect: EffectAllow}
}

// Invalidate clears cached rules for the tenant.
func (e *RuleEvaluator) Invalidate(tenantID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if tenantID == "" {
		e.cache = make(map[string]cacheEntry)
		return
	}
	delete(e.cache, tenantID)
}

func (e *RuleEvaluator) loadRules(ctx context.Context, tenantID string) []compiledRule {
	now := time.Now()
	e.mu.RLock()
	entry, ok := e.cache[tenantID]
	e.mu.RUnlock()
	if ok && now.Before(entry.expires) {
		return entry.rules
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if entry, ok := e.cache[tenantID]; ok && time.Now().Before(entry.expires) {
		return entry.rules
	}
	records, err := e.repo.ListRules(ctx, tenantID)
	if err != nil {
		e.logger.Warn("security policy: list rules failed", zap.String("tenantId", tenantID), zap.Error(err))
		return nil
	}
	compiled := make([]compiledRule, 0, len(records))
	for _, rule := range records {
		compiled = append(compiled, compileRule(rule, e.logger))
	}
	e.cache[tenantID] = cacheEntry{rules: compiled, expires: time.Now().Add(e.ttl)}
	return compiled
}

// cacheEntry tracks compiled rules per tenant.
type cacheEntry struct {
	rules   []compiledRule
	expires time.Time
}

// compiledRule stores parsed match metadata.
type compiledRule struct {
	data    Rule
	matches []compiledMatch
}

func (r compiledRule) matchesContext(ctx compiledEvaluationContext) bool {
	if len(r.matches) == 0 {
		return true
	}
	for _, match := range r.matches {
		if !match.evaluate(ctx) {
			return false
		}
	}
	return true
}

func compileRule(rule Rule, logger *zap.Logger) compiledRule {
	compiled := compiledRule{data: rule}
	for _, match := range rule.Matches {
		cm := compiledMatch{Match: match, valid: true}
		switch match.Type {
		case MatchMAC:
			cm.macValue = strings.ToLower(strings.TrimSpace(match.Value))
			cm.valid = cm.macValue != ""
		case MatchIP:
			addr, err := netip.ParseAddr(strings.TrimSpace(match.Value))
			if err != nil {
				logger.Warn("security policy: invalid ip match", zap.String("value", match.Value), zap.String("ruleId", rule.ID), zap.Error(err))
				cm.valid = false
			} else {
				cm.ipValue = addr
			}
		case MatchCIDR:
			prefix, err := netip.ParsePrefix(strings.TrimSpace(match.Value))
			if err != nil {
				logger.Warn("security policy: invalid cidr match", zap.String("value", match.Value), zap.String("ruleId", rule.ID), zap.Error(err))
				cm.valid = false
			} else {
				cm.cidrValue = prefix
			}
		case MatchVLAN:
			val, err := strconv.Atoi(strings.TrimSpace(match.Value))
			if err != nil {
				logger.Warn("security policy: invalid vlan match", zap.String("value", match.Value), zap.String("ruleId", rule.ID), zap.Error(err))
				cm.valid = false
			} else {
				cm.vlanValue = val
			}
		case MatchInterface:
			cm.iface = strings.ToLower(strings.TrimSpace(match.Value))
			cm.valid = cm.iface != ""
		case MatchPort:
			cm.port = strings.ToLower(strings.TrimSpace(match.Value))
			cm.valid = cm.port != ""
		default:
			logger.Warn("security policy: unsupported match type", zap.String("type", string(match.Type)), zap.String("ruleId", rule.ID))
			cm.valid = false
		}
		compiled.matches = append(compiled.matches, cm)
	}
	return compiled
}

// compiledEvaluationContext caches normalized context values per evaluation.
type compiledEvaluationContext struct {
	mac       string
	ip        netip.Addr
	ipValid   bool
	vlanValue int
	iface     string
	port      string
}

func compileEvaluationContext(ctx EvaluationContext) compiledEvaluationContext {
	compiled := compiledEvaluationContext{
		mac:       strings.ToLower(strings.TrimSpace(ctx.MAC)),
		vlanValue: ctx.VLANID,
		iface:     strings.ToLower(strings.TrimSpace(ctx.InterfaceID)),
		port:      strings.ToLower(strings.TrimSpace(ctx.PortID)),
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(ctx.IP)); err == nil {
		compiled.ip = addr
		compiled.ipValid = true
	}
	return compiled
}

func (m compiledMatch) evaluate(ctx compiledEvaluationContext) bool {
	if !m.valid {
		return false
	}
	var matched bool
	switch m.Type {
	case MatchMAC:
		matched = ctx.mac != "" && ctx.mac == m.macValue
	case MatchIP:
		matched = ctx.ipValid && ctx.ip == m.ipValue
	case MatchCIDR:
		matched = ctx.ipValid && m.cidrValue.Contains(ctx.ip)
	case MatchVLAN:
		matched = ctx.vlanValue == m.vlanValue
	case MatchInterface:
		matched = ctx.iface != "" && ctx.iface == m.iface
	case MatchPort:
		matched = ctx.port != "" && ctx.port == m.port
	default:
		matched = false
	}
	if m.Negate {
		return !matched
	}
	return matched
}
