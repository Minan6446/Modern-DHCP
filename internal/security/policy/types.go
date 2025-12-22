package policy

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

// Effect defines how a rule outcome should be enforced.
type Effect string

const (
	EffectAllow      Effect = "allow"
	EffectMonitor    Effect = "monitor"
	EffectBlock      Effect = "block"
	EffectQuarantine Effect = "quarantine"
)

// ParseEffect normalizes caller input, defaulting to allow.
func ParseEffect(value string) Effect {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(EffectMonitor):
		return EffectMonitor
	case string(EffectBlock):
		return EffectBlock
	case string(EffectQuarantine):
		return EffectQuarantine
	default:
		return EffectAllow
	}
}

// Blocks returns true when the effect should stop packet processing.
func (e Effect) Blocks() bool {
	return e == EffectBlock || e == EffectQuarantine
}

// MatchType enumerates supported attribute matchers.
type MatchType string

const (
	MatchMAC       MatchType = "mac"
	MatchIP        MatchType = "ip"
	MatchCIDR      MatchType = "cidr"
	MatchVLAN      MatchType = "vlan"
	MatchInterface MatchType = "interface"
	MatchPort      MatchType = "port"
)

// Match describes a single attribute comparison.
type Match struct {
	ID        uint64    `json:"id"`
	RuleID    string    `json:"ruleId"`
	TenantID  string    `json:"tenantId"`
	Type      MatchType `json:"type"`
	Value     string    `json:"value"`
	Negate    bool      `json:"negate"`
	CreatedAt time.Time `json:"createdAt"`
}

// Rule defines an ordered policy rule scoped to a tenant.
type Rule struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Priority    int       `json:"priority"`
	Effect      Effect    `json:"effect"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Matches     []Match   `json:"matches"`
}

// RuleSpec captures the mutable fields for create/update operations.
type RuleSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Priority    int         `json:"priority"`
	Effect      Effect      `json:"effect"`
	Enabled     bool        `json:"enabled"`
	Matches     []MatchSpec `json:"matches"`
}

// MatchSpec represents an incoming match request payload.
type MatchSpec struct {
	Type   MatchType `json:"type"`
	Value  string    `json:"value"`
	Negate bool      `json:"negate"`
}

// EvaluationContext mirrors the guard context needed for policy evaluation.
type EvaluationContext struct {
	MAC         string
	IP          string
	VLANID      int
	InterfaceID string
	PortID      string
}

// Decision reports rule matches back to callers.
type Decision struct {
	Matched bool   `json:"matched"`
	RuleID  string `json:"ruleId"`
	Effect  Effect `json:"effect"`
	Name    string `json:"name"`
}

// Evaluator determines whether a context should be allowed, blocked, or monitored.
type Evaluator interface {
	Evaluate(ctx context.Context, tenantID string, input EvaluationContext) Decision
	Invalidate(tenantID string)
}

// compiledMatch aids evaluator performance by caching parsed values.
type compiledMatch struct {
	Match
	macValue  string
	ipValue   netip.Addr
	cidrValue netip.Prefix
	vlanValue int
	iface     string
	port      string
	valid     bool
}
