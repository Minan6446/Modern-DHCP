package leasefsm

import "testing"

func TestValidateTransitionAllowed(t *testing.T) {
	if err := ValidateTransition(LeaseStateOffered, LeaseStateBound); err != nil {
		t.Fatalf("expected transition offered->bound to be allowed: %v", err)
	}
	if err := ValidateTransition(LeaseStateBound, LeaseStateRenewing); err != nil {
		t.Fatalf("expected transition bound->renewing to be allowed: %v", err)
	}
}

func TestValidateTransitionRejected(t *testing.T) {
	if err := ValidateTransition(LeaseStateOffered, LeaseStateReleased); err == nil {
		t.Fatalf("expected transition offered->released to be rejected")
	}
}

func TestValidateTransitionSelfAllowed(t *testing.T) {
	if err := ValidateTransition(LeaseStateBound, LeaseStateBound); err != nil {
		t.Fatalf("expected self-transition bound->bound to be allowed: %v", err)
	}
}
