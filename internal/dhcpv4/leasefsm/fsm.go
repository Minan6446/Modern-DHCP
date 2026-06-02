package leasefsm

import "fmt"

// State defines DHCPv4 lease state-machine nodes.
type State string

const (
	StateInit      State = "Init"
	StateOffered   State = "Offered"
	StateBound     State = "Bound"
	StateRenewing  State = "Renewing"
	StateRebinding State = "Rebinding"
	StateExpired   State = "Expired"
	StateReleased  State = "Released"
)

var allowedTransitions = map[State]map[State]struct{}{
	StateInit: {
		StateOffered: {},
	},
	StateOffered: {
		StateBound:   {},
		StateExpired: {},
	},
	StateBound: {
		StateRenewing:  {},
		StateRebinding: {},
		StateExpired:   {},
		StateReleased:  {},
	},
	StateRenewing: {
		StateBound:     {},
		StateRebinding: {},
		StateExpired:   {},
		StateReleased:  {},
	},
	StateRebinding: {
		StateBound:    {},
		StateExpired:  {},
		StateReleased: {},
	},
	StateExpired: {
		StateInit:     {},
		StateReleased: {},
	},
	StateReleased: {
		StateInit: {},
	},
}

// IsKnownState reports whether the state is part of the DHCPv4 FSM.
func IsKnownState(state State) bool {
	_, ok := allowedTransitions[state]
	return ok
}

// ValidateTransition enforces legal finite-state transitions.
// Example: Offered -> Released is rejected, Offered -> Bound is allowed.
func ValidateTransition(from, to State) error {
	if !IsKnownState(from) {
		return fmt.Errorf("leasefsm: unknown from-state %q", from)
	}
	if !IsKnownState(to) {
		return fmt.Errorf("leasefsm: unknown to-state %q", to)
	}
	if from == to {
		return nil
	}
	next := allowedTransitions[from]
	if _, ok := next[to]; !ok {
		return fmt.Errorf("leasefsm: invalid transition %q -> %q", from, to)
	}
	return nil
}

// Next returns the target state if transition validation succeeds.
func Next(from, to State) (State, error) {
	if err := ValidateTransition(from, to); err != nil {
		return "", err
	}
	return to, nil
}
