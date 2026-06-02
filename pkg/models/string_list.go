package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// StringList wraps a slice of strings with DB helpers.
type StringList []string

// Value implements driver.Valuer for persisting JSON.
func (s StringList) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// Scan implements sql.Scanner for reading JSON.
func (s *StringList) Scan(value any) error {
	if s == nil {
		return errors.New("StringList: nil receiver")
	}
	if value == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("StringList: unsupported type %T", value)
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	var decoded []string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*s = decoded
	return nil
}

// MarshalJSON ensures empty lists serialize as '[]'.
func (s StringList) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	type alias StringList
	return json.Marshal(alias(s))
}

// UnmarshalJSON accepts either null or array payloads.
func (s *StringList) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("StringList: nil receiver")
	}
	if string(data) == "null" {
		*s = nil
		return nil
	}
	var decoded []string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = decoded
	return nil
}
