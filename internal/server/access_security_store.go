package server

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type accessTrustPort struct {
	ID           string `json:"id"`
	Device       string `json:"device"`
	Port         string `json:"port"`
	VLAN         *int   `json:"vlan,omitempty"`
	Trusted      bool   `json:"trusted"`
	RateLimitPps *int   `json:"rateLimitPps,omitempty"`
	LastUpdated  string `json:"lastUpdated,omitempty"`
}

type accessViolationPolicy struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name,omitempty"`
	UpdatedAt            string   `json:"updatedAt,omitempty"`
	Action               string   `json:"action"`
	BlockDurationSeconds *int     `json:"blockDurationSeconds,omitempty"`
	AlertChannels        []string `json:"alertChannels,omitempty"`
}

type accessRateLimitRule struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Target   string `json:"target"`
	LimitPps int    `json:"limitPps"`
	Burst    *int   `json:"burst,omitempty"`
	Dynamic  *bool  `json:"dynamic,omitempty"`
	VLAN     *int   `json:"vlan,omitempty"`
	Status   string `json:"status"`
}

type accessRogueServer struct {
	ID         string   `json:"id"`
	IP         string   `json:"ip"`
	MAC        string   `json:"mac"`
	VLAN       *int     `json:"vlan,omitempty"`
	DetectedAt string   `json:"detectedAt"`
	Severity   string   `json:"severity"`
	Actions    []string `json:"actions,omitempty"`
}

type accessThreatSpoofingRule struct {
	Enabled        bool `json:"enabled"`
	StrictMode     bool `json:"strictMode"`
	BlockThreshold int  `json:"blockThreshold"`
	AutoBlock      bool `json:"autoBlock"`
}

type accessThreatExhaustionRule struct {
	Enabled       bool `json:"enabled"`
	WindowSeconds int  `json:"windowSeconds"`
	AttemptLimit  int  `json:"attemptLimit"`
	AutoRateLimit bool `json:"autoRateLimit"`
}

type accessThreatRogueRule struct {
	Enabled             bool `json:"enabled"`
	DetectInterval      int  `json:"detectInterval"`
	ConfidenceThreshold int  `json:"confidenceThreshold"`
	AutoQuarantine      bool `json:"autoQuarantine"`
}

type accessThreatRuleConfig struct {
	Spoofing   accessThreatSpoofingRule   `json:"spoofing"`
	Exhaustion accessThreatExhaustionRule `json:"exhaustion"`
	Rogue      accessThreatRogueRule      `json:"rogue"`
	UpdatedAt  string                     `json:"updatedAt,omitempty"`
}

type accessSecurityStore struct {
	mu sync.RWMutex

	trustPorts  map[string]map[string]accessTrustPort
	violations  map[string]map[string]accessViolationPolicy
	rateLimits  map[string]map[string]accessRateLimitRule
	rogues      map[string]map[string]accessRogueServer
	threatRules map[string]accessThreatRuleConfig
}

func newAccessSecurityStore() *accessSecurityStore {
	return &accessSecurityStore{
		trustPorts:  make(map[string]map[string]accessTrustPort),
		violations:  make(map[string]map[string]accessViolationPolicy),
		rateLimits:  make(map[string]map[string]accessRateLimitRule),
		rogues:      make(map[string]map[string]accessRogueServer),
		threatRules: make(map[string]accessThreatRuleConfig),
	}
}

func defaultAccessThreatRuleConfig() accessThreatRuleConfig {
	return accessThreatRuleConfig{
		Spoofing: accessThreatSpoofingRule{
			Enabled:        true,
			StrictMode:     true,
			BlockThreshold: 80,
			AutoBlock:      true,
		},
		Exhaustion: accessThreatExhaustionRule{
			Enabled:       true,
			WindowSeconds: 60,
			AttemptLimit:  120,
			AutoRateLimit: true,
		},
		Rogue: accessThreatRogueRule{
			Enabled:             true,
			DetectInterval:      30,
			ConfidenceThreshold: 70,
			AutoQuarantine:      true,
		},
		UpdatedAt: nowRFC3339(),
	}
}

func normalizeThreatRuleConfig(input accessThreatRuleConfig) accessThreatRuleConfig {
	norm := input
	if norm.Spoofing.BlockThreshold <= 0 || norm.Spoofing.BlockThreshold > 100 {
		norm.Spoofing.BlockThreshold = 80
	}
	if norm.Exhaustion.WindowSeconds <= 0 {
		norm.Exhaustion.WindowSeconds = 60
	}
	if norm.Exhaustion.AttemptLimit <= 0 {
		norm.Exhaustion.AttemptLimit = 120
	}
	if norm.Rogue.DetectInterval <= 0 {
		norm.Rogue.DetectInterval = 30
	}
	if norm.Rogue.ConfidenceThreshold <= 0 || norm.Rogue.ConfidenceThreshold > 100 {
		norm.Rogue.ConfidenceThreshold = 70
	}
	norm.UpdatedAt = nowRFC3339()
	return norm
}

func (s *accessSecurityStore) getThreatRules(tenantID string) accessThreatRuleConfig {
	tenantID = normalizeTenantID(tenantID)
	s.mu.RLock()
	item, ok := s.threatRules[tenantID]
	s.mu.RUnlock()
	if ok {
		return item
	}
	return defaultAccessThreatRuleConfig()
}

func (s *accessSecurityStore) saveThreatRules(tenantID string, item accessThreatRuleConfig) accessThreatRuleConfig {
	tenantID = normalizeTenantID(tenantID)
	norm := normalizeThreatRuleConfig(item)
	s.mu.Lock()
	s.threatRules[tenantID] = norm
	s.mu.Unlock()
	return norm
}

func normalizeTenantID(tenantID string) string {
	id := strings.TrimSpace(tenantID)
	if id == "" {
		return systemTenantID
	}
	return id
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func ensureID(id string, prefix string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed != "" {
		return trimmed
	}
	return prefix + "-" + uuid.NewString()
}

func (s *accessSecurityStore) listTrustPorts(tenantID string) []accessTrustPort {
	tenantID = normalizeTenantID(tenantID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.trustPorts[tenantID]
	out := make([]accessTrustPort, 0, len(bucket))
	for _, item := range bucket {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastUpdated > out[j].LastUpdated
	})
	return out
}

func (s *accessSecurityStore) replaceTrustPorts(tenantID string, items []accessTrustPort) []accessTrustPort {
	tenantID = normalizeTenantID(tenantID)
	next := make(map[string]accessTrustPort, len(items))
	for _, item := range items {
		norm := item
		norm.ID = ensureID(norm.ID, "trust")
		norm.Device = strings.TrimSpace(norm.Device)
		norm.Port = strings.TrimSpace(norm.Port)
		norm.LastUpdated = nowRFC3339()
		next[norm.ID] = norm
	}
	s.mu.Lock()
	s.trustPorts[tenantID] = next
	s.mu.Unlock()
	return s.listTrustPorts(tenantID)
}

func (s *accessSecurityStore) listViolationPolicies(tenantID string) []accessViolationPolicy {
	tenantID = normalizeTenantID(tenantID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.violations[tenantID]
	out := make([]accessViolationPolicy, 0, len(bucket))
	for _, item := range bucket {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func (s *accessSecurityStore) upsertViolationPolicy(tenantID string, item accessViolationPolicy) accessViolationPolicy {
	tenantID = normalizeTenantID(tenantID)
	norm := item
	norm.ID = ensureID(norm.ID, "violation")
	norm.Name = strings.TrimSpace(norm.Name)
	if norm.Name == "" {
		norm.Name = "默认违规策略"
	}
	norm.Action = strings.TrimSpace(strings.ToLower(norm.Action))
	if norm.Action == "" {
		norm.Action = "alert"
	}
	norm.UpdatedAt = nowRFC3339()
	if norm.AlertChannels == nil {
		norm.AlertChannels = []string{}
	}
	s.mu.Lock()
	if _, ok := s.violations[tenantID]; !ok {
		s.violations[tenantID] = make(map[string]accessViolationPolicy)
	}
	s.violations[tenantID][norm.ID] = norm
	s.mu.Unlock()
	return norm
}

func (s *accessSecurityStore) deleteViolationPolicy(tenantID string, id string) bool {
	tenantID = normalizeTenantID(tenantID)
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.violations[tenantID]
	if !ok {
		return false
	}
	if _, exists := bucket[id]; !exists {
		return false
	}
	delete(bucket, id)
	return true
}

func (s *accessSecurityStore) listRateLimits(tenantID string) []accessRateLimitRule {
	tenantID = normalizeTenantID(tenantID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.rateLimits[tenantID]
	out := make([]accessRateLimitRule, 0, len(bucket))
	for _, item := range bucket {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Target) < strings.ToLower(out[j].Target)
	})
	return out
}

func (s *accessSecurityStore) upsertRateLimit(tenantID string, item accessRateLimitRule) accessRateLimitRule {
	tenantID = normalizeTenantID(tenantID)
	norm := item
	norm.ID = ensureID(norm.ID, "rate")
	norm.Scope = strings.TrimSpace(strings.ToLower(norm.Scope))
	if norm.Scope == "" {
		norm.Scope = "port"
	}
	norm.Target = strings.TrimSpace(norm.Target)
	if norm.Target == "" {
		norm.Target = "-"
	}
	if norm.LimitPps < 0 {
		norm.LimitPps = 0
	}
	norm.Status = strings.TrimSpace(strings.ToLower(norm.Status))
	if norm.Status == "" {
		norm.Status = "active"
	}
	s.mu.Lock()
	if _, ok := s.rateLimits[tenantID]; !ok {
		s.rateLimits[tenantID] = make(map[string]accessRateLimitRule)
	}
	s.rateLimits[tenantID][norm.ID] = norm
	s.mu.Unlock()
	return norm
}

func (s *accessSecurityStore) deleteRateLimit(tenantID string, id string) bool {
	tenantID = normalizeTenantID(tenantID)
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.rateLimits[tenantID]
	if !ok {
		return false
	}
	if _, exists := bucket[id]; !exists {
		return false
	}
	delete(bucket, id)
	return true
}

func (s *accessSecurityStore) listRogueServers(tenantID string) []accessRogueServer {
	tenantID = normalizeTenantID(tenantID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.rogues[tenantID]
	out := make([]accessRogueServer, 0, len(bucket))
	for _, item := range bucket {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].DetectedAt > out[j].DetectedAt
	})
	return out
}

func (s *accessSecurityStore) createRogueServer(tenantID string, item accessRogueServer) accessRogueServer {
	tenantID = normalizeTenantID(tenantID)
	norm := item
	norm.ID = ensureID(norm.ID, "rogue")
	norm.IP = strings.TrimSpace(norm.IP)
	norm.MAC = strings.TrimSpace(norm.MAC)
	norm.Severity = strings.TrimSpace(strings.ToLower(norm.Severity))
	if norm.Severity == "" {
		norm.Severity = "medium"
	}
	norm.DetectedAt = nowRFC3339()
	if norm.Actions == nil {
		norm.Actions = []string{}
	}
	s.mu.Lock()
	if _, ok := s.rogues[tenantID]; !ok {
		s.rogues[tenantID] = make(map[string]accessRogueServer)
	}
	s.rogues[tenantID][norm.ID] = norm
	s.mu.Unlock()
	return norm
}

func (s *accessSecurityStore) updateRogueServer(tenantID string, id string, item accessRogueServer) (accessRogueServer, bool) {
	tenantID = normalizeTenantID(tenantID)
	id = strings.TrimSpace(id)
	if id == "" {
		return accessRogueServer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.rogues[tenantID]
	if !ok {
		return accessRogueServer{}, false
	}
	current, exists := bucket[id]
	if !exists {
		return accessRogueServer{}, false
	}
	if value := strings.TrimSpace(item.IP); value != "" {
		current.IP = value
	}
	if value := strings.TrimSpace(item.MAC); value != "" {
		current.MAC = value
	}
	if item.VLAN != nil {
		current.VLAN = item.VLAN
	}
	if value := strings.TrimSpace(strings.ToLower(item.Severity)); value != "" {
		current.Severity = value
	}
	if item.Actions != nil {
		current.Actions = item.Actions
	}
	current.DetectedAt = nowRFC3339()
	bucket[id] = current
	return current, true
}

func (s *accessSecurityStore) deleteRogueServer(tenantID string, id string) bool {
	tenantID = normalizeTenantID(tenantID)
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.rogues[tenantID]
	if !ok {
		return false
	}
	if _, exists := bucket[id]; !exists {
		return false
	}
	delete(bucket, id)
	return true
}

func (s *accessSecurityStore) markRogueAction(tenantID string, id string, action string) (accessRogueServer, bool) {
	tenantID = normalizeTenantID(tenantID)
	id = strings.TrimSpace(id)
	action = strings.TrimSpace(strings.ToLower(action))
	if id == "" || action == "" {
		return accessRogueServer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.rogues[tenantID]
	if !ok {
		return accessRogueServer{}, false
	}
	record, exists := bucket[id]
	if !exists {
		return accessRogueServer{}, false
	}
	has := false
	for _, item := range record.Actions {
		if strings.EqualFold(item, action) {
			has = true
			break
		}
	}
	if !has {
		record.Actions = append(record.Actions, action)
	}
	if action == "block" {
		record.Severity = "high"
	} else if action == "quarantine" && strings.TrimSpace(record.Severity) == "low" {
		record.Severity = "medium"
	}
	record.DetectedAt = nowRFC3339()
	bucket[id] = record
	return record, true
}
