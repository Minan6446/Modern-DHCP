package policy

import (
	"testing"
	"time"

	"modern-dhcp/pkg/models"
)

func TestMatchRuleLocationSelectors(t *testing.T) {
	engine := &Engine{}
	rule := models.PolicyRule{
		Conditions: []byte(`{"location":["HQ","Lab"]}`),
		Actions:    []byte(`{"poolId":"pool-1"}`),
	}
	input := Input{Location: "lab"}
	match, _ := engine.matchRule(rule, input)
	if !match {
		t.Fatalf("expected case-insensitive match for location array")
	}
	input = Input{Location: "branch"}
	match, _ = engine.matchRule(rule, input)
	if match {
		t.Fatalf("expected mismatch for unmatched location")
	}
}

func TestMatchRuleUserGroupSelectors(t *testing.T) {
	engine := &Engine{}
	rule := models.PolicyRule{
		Conditions: []byte(`{"userGroup":"staff"}`),
		Actions:    []byte(`{"poolId":"pool-2"}`),
	}
	input := Input{UserGroups: []string{"Guest", "Staff"}}
	match, _ := engine.matchRule(rule, input)
	if !match {
		t.Fatalf("expected userGroup string to match regardless of case")
	}

	rule.Conditions = []byte(`{"userGroups":["vip","partner"]}`)
	input = Input{UserGroups: []string{"VIP"}}
	match, _ = engine.matchRule(rule, input)
	if !match {
		t.Fatalf("expected userGroups array to match when any entry aligns")
	}
}

func TestMatchStringConditionDefaults(t *testing.T) {
	if !matchStringCondition("", "", true) {
		t.Fatalf("empty selector should default to match")
	}
	if !matchStringCondition("branch", []any{}, true) {
		t.Fatalf("empty array selector should default to match")
	}
}

func TestMatchSliceConditionDefaults(t *testing.T) {
	groups := []string{"alpha"}
	if !matchSliceCondition(groups, "", true) {
		t.Fatalf("empty string selector should default to match")
	}
	if !matchSliceCondition(groups, []any{}, true) {
		t.Fatalf("empty slice selector should default to match")
	}
}

func TestWithinTimeRangeDayFilters(t *testing.T) {
	ts := time.Date(2025, time.January, 6, 9, 0, 0, 0, time.UTC) // Monday
	cfg := map[string]any{"start": "08:00", "end": "18:00"}
	if !withinTimeRange(ts, cfg) {
		t.Fatalf("missing days should allow all weekdays")
	}

	cfg["days"] = []any{"weekday"}
	saturday := time.Date(2025, time.January, 4, 9, 0, 0, 0, time.UTC)
	if withinTimeRange(saturday, cfg) {
		t.Fatalf("weekday token should exclude weekends")
	}
}
