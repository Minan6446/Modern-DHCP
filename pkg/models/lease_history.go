package models

import "time"

// LeaseHistoryFilter constrains historical lease lookups.
type LeaseHistoryFilter struct {
	State      string    `json:"state,omitempty"`
	Identifier string    `json:"identifier,omitempty"`
	IPAddress  string    `json:"ipAddress,omitempty"`
	From       time.Time `json:"from,omitempty"`
	To         time.Time `json:"to,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}
