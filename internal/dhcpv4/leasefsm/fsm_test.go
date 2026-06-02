package leasefsm

import "testing"

func TestValidateTransitionAllowed(t *testing.T) {
	if err := ValidateTransition(StateOffered, StateBound); err != nil {
		t.Fatalf("expected Offered -> Bound allowed, got error: %v", err)
	}
}

func TestValidateTransitionRejected(t *testing.T) {
	if err := ValidateTransition(StateOffered, StateReleased); err == nil {
		t.Fatalf("expected Offered -> Released to be rejected")
	}
}

func TestValidateTransitionSelfAllowed(t *testing.T) {
	if err := ValidateTransition(StateBound, StateBound); err != nil {
		t.Fatalf("expected self transition to be allowed, got error: %v", err)
	}
}
