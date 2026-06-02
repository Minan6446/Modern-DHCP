package alerting

import (
	"strings"
	"sync"
	"time"
)

// RoutingRule captures a single alert routing definition persisted by the API.
type RoutingRule struct {
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	Severities        []string  `json:"severities"`
	Channels          []string  `json:"channels"`
	EscalationMinutes int       `json:"escalationMinutes,omitempty"`
	Enabled           bool      `json:"enabled"`
	UpdatedAt         time.Time `json:"updatedAt"`
	UpdatedBy         string    `json:"updatedBy"`
}

// RoutingSnapshot exposes the routing table for UI consumption.
type RoutingSnapshot struct {
	UpdatedAt time.Time     `json:"updatedAt"`
	UpdatedBy string        `json:"updatedBy"`
	Rules     []RoutingRule `json:"rules"`
}

// RoutingStore persists routing rules in memory with thread safety.
type RoutingStore struct {
	mu       sync.RWMutex
	snapshot RoutingSnapshot
}

// NewRoutingStore constructs a routing store with optional rules.
func NewRoutingStore(rules []RoutingRule, actor string) *RoutingStore {
	store := &RoutingStore{}
	store.Replace(rules, actor)
	return store
}

// Snapshot returns a deep copy of the routing snapshot for external use.
func (s *RoutingStore) Snapshot() RoutingSnapshot {
	if s == nil {
		return RoutingSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyRoutingSnapshot(s.snapshot)
}

// Replace swaps the current routing table with the provided rules and records the actor.
func (s *RoutingStore) Replace(rules []RoutingRule, actor string) RoutingSnapshot {
	if s == nil {
		return RoutingSnapshot{}
	}
	cleaned := make([]RoutingRule, 0, len(rules))
	now := time.Now().UTC()
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			continue
		}
		normalized := RoutingRule{
			Name:              strings.TrimSpace(rule.Name),
			Description:       strings.TrimSpace(rule.Description),
			EscalationMinutes: rule.EscalationMinutes,
			Enabled:           rule.Enabled,
			UpdatedAt:         now,
			UpdatedBy:         actor,
		}
		normalized.Severities = normalizeValues(rule.Severities)
		normalized.Channels = normalizeValues(rule.Channels)
		cleaned = append(cleaned, normalized)
	}
	snapshot := RoutingSnapshot{
		UpdatedAt: now,
		UpdatedBy: actor,
		Rules:     make([]RoutingRule, len(cleaned)),
	}
	copy(snapshot.Rules, cleaned)
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
	return copyRoutingSnapshot(snapshot)
}

func copyRoutingSnapshot(src RoutingSnapshot) RoutingSnapshot {
	if len(src.Rules) == 0 {
		return RoutingSnapshot{UpdatedAt: src.UpdatedAt, UpdatedBy: src.UpdatedBy, Rules: []RoutingRule{}}
	}
	clone := RoutingSnapshot{
		UpdatedAt: src.UpdatedAt,
		UpdatedBy: src.UpdatedBy,
		Rules:     make([]RoutingRule, len(src.Rules)),
	}
	for idx, rule := range src.Rules {
		dup := RoutingRule{
			Name:              rule.Name,
			Description:       rule.Description,
			EscalationMinutes: rule.EscalationMinutes,
			Enabled:           rule.Enabled,
			UpdatedAt:         rule.UpdatedAt,
			UpdatedBy:         rule.UpdatedBy,
		}
		if len(rule.Severities) > 0 {
			dup.Severities = append([]string(nil), rule.Severities...)
		} else {
			dup.Severities = []string{}
		}
		if len(rule.Channels) > 0 {
			dup.Channels = append([]string(nil), rule.Channels...)
		} else {
			dup.Channels = []string{}
		}
		clone.Rules[idx] = dup
	}
	return clone
}

func normalizeValues(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	ordered := make([]string, 0, len(values))
	for _, raw := range values {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		ordered = append(ordered, normalized)
	}
	return ordered
}
