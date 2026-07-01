package policy

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/pkg/models"
)

// Input aggregates attributes for rule evaluation.
type Input struct {
	TenantID      string
	MAC           string
	ClientID      string
	UserID        string
	DeviceType    string
	DevicePersona string
	DeviceTags    []string
	MDMManaged    bool
	UserGroups    []string
	Location      string
	VLANID        int
	SSID          string
	AccessPointID string
	ControllerID  string
	GeoZone       string
	RelayInfo     map[string]any
	VendorClass   string
	UserClass     string
	Option60      string
	Option82      map[string]string
	IPv4Class     string
	IsBOOTP       bool
	SupportsSLAAC bool
	Timestamp     time.Time
}

// Decision captures the resulting actions.
type Decision struct {
	PoolID       string
	PoolSelector *PoolSelector
	LeaseProfile models.LeaseProfile
	SecurityTags []string
	Metadata     map[string]any
}

// PoolSelector describes metadata-based pool resolution instructions.
type PoolSelector struct {
	VLANID        int    `json:"vlanId"`
	InterfaceID   string `json:"interfaceId"`
	SSID          string `json:"ssid"`
	Location      string `json:"location"`
	AccessPointID string `json:"accessPointId"`
	ControllerID  string `json:"controllerId"`
	GeoZone       string `json:"geoZone"`
}

const defaultCacheTTL = 60 * time.Second

// Engine evaluates policy rules stored in MySQL.
type Engine struct {
	repo      Repository
	cache     map[string][]models.PolicyRule
	cacheTime map[string]time.Time
	cacheTTL  time.Duration
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewEngine creates a policy engine with naive in-memory cache.
func NewEngine(repo Repository, logger *zap.Logger) *Engine {
	return &Engine{
		repo:      repo,
		cache:     make(map[string][]models.PolicyRule),
		cacheTime: make(map[string]time.Time),
		cacheTTL:  defaultCacheTTL,
		logger:    logger,
	}
}

// Evaluate returns the highest-priority matching decision.
func (e *Engine) Evaluate(ctx context.Context, input Input) (*Decision, error) {
	rules, err := e.fetchRules(ctx, input.TenantID)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		match, actions := e.matchRule(rule, input)
		if !match {
			continue
		}
		var decision Decision
		if err := json.Unmarshal(actions, &decision); err != nil {
			return nil, err
		}
		return &decision, nil
	}

	return nil, errors.New("no matching policy rule")
}

func (e *Engine) fetchRules(ctx context.Context, tenantID string) ([]models.PolicyRule, error) {
	// Fast path: read-lock and check cache
	e.mu.RLock()
	if cached, ok := e.cache[tenantID]; ok {
		if time.Since(e.cacheTime[tenantID]) < e.cacheTTL {
			e.mu.RUnlock()
			return cached, nil
		}
	}
	e.mu.RUnlock()

	// Cache miss or expired: acquire write lock and double-check
	e.mu.Lock()
	if cached, ok := e.cache[tenantID]; ok {
		if time.Since(e.cacheTime[tenantID]) < e.cacheTTL {
			e.mu.Unlock()
			return cached, nil
		}
	}

	rules, err := e.repo.ListRules(ctx, tenantID, 500, 0)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	filtered := make([]models.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			filtered = append(filtered, rule)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Priority < filtered[j].Priority })
	e.cache[tenantID] = filtered
	e.cacheTime[tenantID] = time.Now()
	e.mu.Unlock()
	return filtered, nil
}

func (e *Engine) matchRule(rule models.PolicyRule, input Input) (bool, json.RawMessage) {
	var conditions map[string]any
	if err := json.Unmarshal(rule.Conditions, &conditions); err != nil {
		e.logger.Warn("invalid policy conditions", zap.String("ruleId", rule.ID), zap.Error(err))
		return false, nil
	}

	for k, v := range conditions {
		switch k {
		case "deviceType":
			if str, ok := v.(string); ok && input.DeviceType != str {
				return false, nil
			}
		case "devicePersona":
			if str, ok := v.(string); ok && !strings.EqualFold(input.DevicePersona, str) {
				return false, nil
			}
		case "deviceTag", "deviceTags":
			if !matchSliceCondition(input.DeviceTags, v, true) {
				return false, nil
			}
		case "mdmManaged", "deviceMdmManaged":
			if want, ok := toBool(v); ok && want != input.MDMManaged {
				return false, nil
			}
		case "location":
			if !matchStringCondition(input.Location, v, true) {
				return false, nil
			}
		case "vlanID":
			if vlan, ok := toInt(v); ok && vlan != input.VLANID {
				return false, nil
			}
		case "ssid":
			if ssid, ok := v.(string); ok && input.SSID != ssid {
				return false, nil
			}
		case "accessPointId", "apId":
			if !matchStringCondition(input.AccessPointID, v, true) {
				return false, nil
			}
		case "controllerId":
			if !matchStringCondition(input.ControllerID, v, true) {
				return false, nil
			}
		case "geoZone":
			if !matchStringCondition(input.GeoZone, v, true) {
				return false, nil
			}
		case "userGroup", "userGroups":
			if !matchSliceCondition(input.UserGroups, v, true) {
				return false, nil
			}
		case "timeRange":
			if cfg, ok := v.(map[string]any); ok && !withinTimeRange(input.Timestamp, cfg) {
				return false, nil
			}
		case "supportsSLAAC", "slaac":
			if want, ok := toBool(v); ok && want != input.SupportsSLAAC {
				return false, nil
			}
		case "ipv4Class":
			if !matchStringCondition(input.IPv4Class, v, false) {
				return false, nil
			}
		case "bootp", "isBootp":
			if want, ok := toBool(v); ok && want != input.IsBOOTP {
				return false, nil
			}
		case "vendorClass":
			if !matchStringCondition(input.VendorClass, v, false) {
				return false, nil
			}
		case "userClass":
			if !matchStringCondition(input.UserClass, v, false) {
				return false, nil
			}
		case "clientID":
			if !matchStringCondition(input.ClientID, v, false) {
				return false, nil
			}
		case "mac":
			if !matchStringCondition(input.MAC, v, true) {
				return false, nil
			}
		case "option60":
			if !matchStringCondition(input.Option60, v, false) {
				return false, nil
			}
		default:
			continue
		}
	}

	return true, rule.Actions
}

func withinTimeRange(ts time.Time, cfg map[string]any) bool {
	if !matchesAllowedDay(ts, cfg) {
		return false
	}
	start, startOK := cfg["start"].(string)
	end, endOK := cfg["end"].(string)
	if !startOK || !endOK {
		return true
	}
	layout := "15:04"
	startParsed, err := time.Parse(layout, start)
	if err != nil {
		return true
	}
	endParsed, err := time.Parse(layout, end)
	if err != nil {
		return true
	}
	current := time.Date(0, 1, 1, ts.Hour(), ts.Minute(), 0, 0, time.UTC)
	startComparable := time.Date(0, 1, 1, startParsed.Hour(), startParsed.Minute(), 0, 0, time.UTC)
	endComparable := time.Date(0, 1, 1, endParsed.Hour(), endParsed.Minute(), 0, 0, time.UTC)
	if endComparable.Before(startComparable) {
		endComparable = endComparable.Add(24 * time.Hour)
		if current.Before(startComparable) {
			current = current.Add(24 * time.Hour)
		}
	}
	return (current.Equal(startComparable) || current.After(startComparable)) && current.Before(endComparable)
}

func matchesAllowedDay(ts time.Time, cfg map[string]any) bool {
	allowed := buildAllowedDaySet(cfg)
	if len(allowed) == 0 {
		return true
	}
	key := weekdayKey(ts.Weekday())
	_, ok := allowed[key]
	return ok
}

func buildAllowedDaySet(cfg map[string]any) map[string]struct{} {
	var raw any
	if val, ok := cfg["daysOfWeek"]; ok {
		raw = val
	} else if val, ok := cfg["days"]; ok {
		raw = val
	} else {
		return nil
	}
	tokens := extractDayTokens(raw)
	if len(tokens) == 0 {
		return nil
	}
	set := make(map[string]struct{})
	for _, token := range tokens {
		for _, canonical := range expandDayToken(token) {
			set[canonical] = struct{}{}
		}
	}
	return set
}

func extractDayTokens(raw any) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func expandDayToken(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	switch strings.ToLower(raw) {
	case "mon", "monday":
		return []string{"mon"}
	case "tue", "tues", "tuesday":
		return []string{"tue"}
	case "wed", "weds", "wednesday":
		return []string{"wed"}
	case "thu", "thur", "thurs", "thursday":
		return []string{"thu"}
	case "fri", "friday":
		return []string{"fri"}
	case "sat", "saturday":
		return []string{"sat"}
	case "sun", "sunday":
		return []string{"sun"}
	case "weekday", "workday":
		return []string{"mon", "tue", "wed", "thu", "fri"}
	case "weekend":
		return []string{"sat", "sun"}
	default:
		return nil
	}
}

func weekdayKey(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	default:
		return "sun"
	}
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	default:
		return 0, false
	}
}

func toBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		lower := strings.ToLower(val)
		if lower == "true" {
			return true, true
		}
		if lower == "false" {
			return false, true
		}
	}
	return false, false
}

func matchStringCondition(actual string, cond any, ignoreCase bool) bool {
	cmp := func(a, b string) bool {
		if ignoreCase {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	actual = strings.TrimSpace(actual)
	switch v := cond.(type) {
	case string:
		if v == "" {
			return true
		}
		return cmp(actual, v)
	case []any:
		if len(v) == 0 {
			return true
		}
		for _, item := range v {
			if str, ok := item.(string); ok && cmp(actual, str) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func matchSliceCondition(values []string, cond any, ignoreCase bool) bool {
	if len(values) == 0 {
		return false
	}
	cmp := func(a, b string) bool {
		if ignoreCase {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	matches := func(target string) bool {
		for _, value := range values {
			if cmp(strings.TrimSpace(value), target) {
				return true
			}
		}
		return false
	}
	switch v := cond.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return true
		}
		return matches(v)
	case []any:
		if len(v) == 0 {
			return true
		}
		for _, item := range v {
			if str, ok := item.(string); ok && matches(str) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Invalidate clears cache entries for a tenant (or all when empty string).
func (e *Engine) Invalidate(tenantID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if tenantID == "" {
		e.cache = make(map[string][]models.PolicyRule)
		e.cacheTime = make(map[string]time.Time)
		return
	}
	delete(e.cache, tenantID)
	delete(e.cacheTime, tenantID)
}
