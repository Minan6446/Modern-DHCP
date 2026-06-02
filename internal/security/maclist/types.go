package maclist

import (
	"context"
	"time"
)

// ListType enumerates supported MAC list categories.
type ListType string

const (
	ListTypeWhitelist ListType = "whitelist"
	ListTypeBlacklist ListType = "blacklist"
	ListTypeGraylist  ListType = "graylist"
)

// Action describes the enforcement for a list entry.
type Action string

const (
	ActionAllow   Action = "allow"
	ActionBlock   Action = "block"
	ActionMonitor Action = "monitor"
)

// Entry represents a MAC list record.
type Entry struct {
	ID          string     `db:"id"`
	MAC         string     `db:"mac"`
	Type        ListType   `db:"list_type"`
	Action      Action     `db:"action"`
	Description string     `db:"description"`
	Source      string     `db:"source"`
	Priority    int        `db:"priority"`
	Enabled     bool       `db:"enabled"`
	ValidFrom   *time.Time `db:"valid_from"`
	ValidUntil  *time.Time `db:"valid_until"`
	Metadata    []byte     `db:"metadata"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// Result conveys an evaluation decision.
type Result struct {
	Entry *Entry
}

// Evaluator returns the list decision for a given MAC.
type Evaluator interface {
	Evaluate(ctx context.Context, tenantID, mac string) (Result, error)
}
