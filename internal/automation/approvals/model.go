package approvals

import (
	"encoding/json"
	"time"
)

// RequestType enumerates automation change categories.
type RequestType string

const (
	RequestTypeScheduleUpdate RequestType = "schedule.update"
	RequestTypeScheduleToggle RequestType = "schedule.toggle"
	RequestTypeJobRun         RequestType = "job.run"
)

// RequestStatus captures the current decision state.
type RequestStatus string

const (
	RequestStatusPending  RequestStatus = "pending"
	RequestStatusApproved RequestStatus = "approved"
	RequestStatusRejected RequestStatus = "rejected"
	RequestStatusApplied  RequestStatus = "applied"
	RequestStatusExpired  RequestStatus = "expired"
)

// ChangeRequest encapsulates a pending automation change awaiting approval.
type ChangeRequest struct {
	ID             string          `json:"id" db:"id"`
	JobType        string          `json:"jobType" db:"job_type"`
	RequestType    RequestType     `json:"requestType" db:"request_type"`
	Status         RequestStatus   `json:"status" db:"status"`
	RequestedBy    string          `json:"requestedBy" db:"requested_by"`
	RequestedAt    time.Time       `json:"requestedAt" db:"requested_at"`
	ApproverID     string          `json:"approverId,omitempty" db:"approver_id"`
	DecidedAt      *time.Time      `json:"decidedAt,omitempty" db:"decided_at"`
	DecisionNote   string          `json:"decisionNote,omitempty" db:"decision_note"`
	OriginalConfig json.RawMessage `json:"originalConfig,omitempty" db:"original_config"`
	ProposedConfig json.RawMessage `json:"proposedConfig,omitempty" db:"proposed_config"`
	Payload        json.RawMessage `json:"payload,omitempty" db:"payload"`
	AutoApplied    bool            `json:"autoApplied" db:"auto_applied"`
	AppliedAt      *time.Time      `json:"appliedAt,omitempty" db:"applied_at"`
	CreatedAt      time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" db:"updated_at"`
	Version        int             `json:"version" db:"version"`
}

// Clone returns a deep copy to avoid mutating internal slices.
func (r ChangeRequest) Clone() ChangeRequest {
	clone := r
	if len(r.OriginalConfig) > 0 {
		clone.OriginalConfig = cloneJSON(r.OriginalConfig)
	}
	if len(r.ProposedConfig) > 0 {
		clone.ProposedConfig = cloneJSON(r.ProposedConfig)
	}
	if len(r.Payload) > 0 {
		clone.Payload = cloneJSON(r.Payload)
	}
	if r.DecidedAt != nil {
		ts := *r.DecidedAt
		clone.DecidedAt = &ts
	}
	if r.AppliedAt != nil {
		ts := *r.AppliedAt
		clone.AppliedAt = &ts
	}
	return clone
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	dup := make([]byte, len(raw))
	copy(dup, raw)
	return dup
}
