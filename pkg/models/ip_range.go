package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// IPRange represents a contiguous inclusive range of IP addresses.
type IPRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// IPRangeList wraps a slice of IPRange with DB helpers.
type IPRangeList []IPRange

// Value implements driver.Valuer for persisting JSON.
func (r IPRangeList) Value() (driver.Value, error) {
	if len(r) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// Scan implements sql.Scanner for reading JSON.
func (r *IPRangeList) Scan(value any) error {
	if r == nil {
		return errors.New("IPRangeList: nil receiver")
	}
	if value == nil {
		*r = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("IPRangeList: unsupported type %T", value)
	}
	if len(raw) == 0 {
		*r = nil
		return nil
	}
	var decoded []IPRange
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*r = decoded
	return nil
}

// MarshalJSON ensures empty lists serialize as '[]'.
func (r IPRangeList) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("[]"), nil
	}
	type alias IPRangeList
	return json.Marshal(alias(r))
}

// UnmarshalJSON accepts either null or array payloads.
func (r *IPRangeList) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("IPRangeList: nil receiver")
	}
	if string(data) == "null" {
		*r = nil
		return nil
	}
	var decoded []IPRange
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = decoded
	return nil
}
