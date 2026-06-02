package alerting

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// DutyShift represents an on-call shift definition for a specific rotation.
type DutyShift struct {
	ID                string    `json:"id"`
	Team              string    `json:"team"`
	Rotation          string    `json:"rotation"`
	Primary           []string  `json:"primary"`
	Secondary         []string  `json:"secondary,omitempty"`
	EscalationMinutes int       `json:"escalationMinutes,omitempty"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	HandOffNotes      string    `json:"handOffNotes,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
	UpdatedBy         string    `json:"updatedBy"`
}

// DutyRoster exposes the normalized on-call rotations for the UI.
type DutyRoster struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	UpdatedBy   string      `json:"updatedBy"`
	Shifts      []DutyShift `json:"shifts"`
}

// DutySchedule manages the in-memory duty roster.
type DutySchedule struct {
	mu     sync.RWMutex
	roster DutyRoster
}

// NewDutySchedule constructs a schedule with optional initial shifts.
func NewDutySchedule(shifts []DutyShift, actor string) *DutySchedule {
	sched := &DutySchedule{}
	sched.Replace(shifts, actor)
	return sched
}

// Snapshot returns a copy of the current duty roster.
func (s *DutySchedule) Snapshot() DutyRoster {
	if s == nil {
		return DutyRoster{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyRoster(s.roster)
}

// Replace swaps the duty roster with the provided shifts and notes the actor.
func (s *DutySchedule) Replace(shifts []DutyShift, actor string) DutyRoster {
	if s == nil {
		return DutyRoster{}
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	now := time.Now().UTC()
	cleaned := sanitizeShifts(shifts, actor, now)
	roster := DutyRoster{
		GeneratedAt: time.Now().UTC(),
		UpdatedAt:   now,
		UpdatedBy:   actor,
		Shifts:      cleaned,
	}
	s.mu.Lock()
	s.roster = roster
	s.mu.Unlock()
	return copyRoster(roster)
}

func sanitizeShifts(shifts []DutyShift, actor string, now time.Time) []DutyShift {
	sanitized := make([]DutyShift, 0, len(shifts))
	for _, shift := range shifts {
		start := shift.Start.UTC()
		end := shift.End.UTC()
		if start.IsZero() || end.IsZero() || !end.After(start) {
			continue
		}
		entry := DutyShift{
			ID:                strings.TrimSpace(shift.ID),
			Team:              strings.TrimSpace(shift.Team),
			Rotation:          strings.TrimSpace(shift.Rotation),
			Primary:           normalizeList(shift.Primary),
			Secondary:         normalizeList(shift.Secondary),
			EscalationMinutes: shift.EscalationMinutes,
			Start:             start,
			End:               end,
			HandOffNotes:      strings.TrimSpace(shift.HandOffNotes),
			UpdatedAt:         now,
			UpdatedBy:         actor,
		}
		if entry.ID == "" {
			entry.ID = buildShiftID(entry.Team, entry.Rotation, entry.Start)
		}
		sanitized = append(sanitized, entry)
	}
	sort.SliceStable(sanitized, func(i, j int) bool {
		if sanitized[i].Start.Equal(sanitized[j].Start) {
			return sanitized[i].ID < sanitized[j].ID
		}
		return sanitized[i].Start.Before(sanitized[j].Start)
	})
	return sanitized
}

func copyRoster(src DutyRoster) DutyRoster {
	roster := DutyRoster{
		GeneratedAt: src.GeneratedAt,
		UpdatedAt:   src.UpdatedAt,
		UpdatedBy:   src.UpdatedBy,
	}
	if len(src.Shifts) == 0 {
		roster.Shifts = []DutyShift{}
		return roster
	}
	roster.Shifts = make([]DutyShift, len(src.Shifts))
	for idx, shift := range src.Shifts {
		clone := DutyShift{
			ID:                shift.ID,
			Team:              shift.Team,
			Rotation:          shift.Rotation,
			EscalationMinutes: shift.EscalationMinutes,
			Start:             shift.Start,
			End:               shift.End,
			HandOffNotes:      shift.HandOffNotes,
			UpdatedAt:         shift.UpdatedAt,
			UpdatedBy:         shift.UpdatedBy,
		}
		clone.Primary = append([]string(nil), shift.Primary...)
		clone.Secondary = append([]string(nil), shift.Secondary...)
		roster.Shifts[idx] = clone
	}
	return roster
}

func normalizeList(values []string) []string {
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

func buildShiftID(team, rotation string, start time.Time) string {
	base := strings.ReplaceAll(strings.TrimSpace(team), " ", "-")
	rot := strings.ReplaceAll(strings.TrimSpace(rotation), " ", "-")
	return strings.ToLower(strings.Trim(strings.Join([]string{base, rot, start.Format("20060102T1504Z")}, "-"), "-"))
}
