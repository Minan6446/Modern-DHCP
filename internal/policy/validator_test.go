package policy

import "testing"

func TestValidateActionsRequiresPoolTarget(t *testing.T) {
	payload := []byte(`{"leaseProfile":{"defaultDuration":3600}}`)
	if err := ValidateActions(payload); err == nil {
		t.Fatalf("expected error for missing pool target")
	}
}

func TestValidateActionsAcceptsPoolSelector(t *testing.T) {
	payload := []byte(`{"poolSelector":{"vlanId":200}}`)
	if err := ValidateActions(payload); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateActionsRejectsEmptySelector(t *testing.T) {
	payload := []byte(`{"poolSelector":{}}`)
	if err := ValidateActions(payload); err == nil {
		t.Fatalf("expected error for empty poolSelector")
	}
}

func TestValidateConditionsStringSelectorVariants(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		payload := []byte(`{"location":"HQ-West"}`)
		if err := ValidateConditions(payload); err != nil {
			t.Fatalf("location string should be valid: %v", err)
		}
	})
	t.Run("array", func(t *testing.T) {
		payload := []byte(`{"userGroups":["guest","Partner"]}`)
		if err := ValidateConditions(payload); err != nil {
			t.Fatalf("userGroups array should be valid: %v", err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		payload := []byte(`{"location":123}`)
		if err := ValidateConditions(payload); err == nil {
			t.Fatalf("expected error for non-string selector")
		}
	})
}

func TestValidateConditionsTimeRangeDayList(t *testing.T) {
	valid := []byte(`{"timeRange":{"start":"08:00","end":"18:00","days":["mon","Weekday"]}}`)
	if err := ValidateConditions(valid); err != nil {
		t.Fatalf("timeRange with valid day tokens should pass: %v", err)
	}
	invalid := []byte(`{"timeRange":{"start":"08:00","end":"18:00","days":["monday","holiday"]}}`)
	if err := ValidateConditions(invalid); err == nil {
		t.Fatalf("expected error for invalid day token")
	}
}

func TestValidateConditionsBYODFields(t *testing.T) {
	valid := []byte(`{"devicePersona":"byod","deviceTags":["ios","mobile"],"mdmManaged":true}`)
	if err := ValidateConditions(valid); err != nil {
		t.Fatalf("BYOD fields should be accepted: %v", err)
	}
	invalid := []byte(`{"mdmManaged":"true"}`)
	if err := ValidateConditions(invalid); err == nil {
		t.Fatalf("mdmManaged must be boolean")
	}
}
