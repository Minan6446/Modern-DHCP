package leasefsm

import "fmt"

// State defines DHCPv6 lease state-machine nodes.
type State string

const (
	LeaseStateInit      State = "Init"
	LeaseStateOffered   State = "Offered"
	LeaseStateBound     State = "Bound"
	LeaseStateRenewing  State = "Renewing"
	LeaseStateRebinding State = "Rebinding"
	LeaseStateExpired   State = "Expired"
	LeaseStateReleased  State = "Released"
)

var allowedTransitions = map[State]map[State]struct{}{
	LeaseStateInit: {
		LeaseStateOffered: {},
	},
	LeaseStateOffered: {
		LeaseStateBound:   {},
		LeaseStateExpired: {},
	},
	LeaseStateBound: {
		LeaseStateRenewing:  {},
		LeaseStateRebinding: {},
		LeaseStateExpired:   {},
		LeaseStateReleased:  {},
	},
	LeaseStateRenewing: {
		LeaseStateBound:     {},
		LeaseStateRebinding: {},
		LeaseStateExpired:   {},
		LeaseStateReleased:  {},
	},
	LeaseStateRebinding: {
		LeaseStateBound:    {},
		LeaseStateExpired:  {},
		LeaseStateReleased: {},
	},
	LeaseStateExpired: {
		LeaseStateInit:     {},
		LeaseStateReleased: {},
	},
	LeaseStateReleased: {
		LeaseStateInit: {},
	},
}

// IsKnownState reports whether the state is part of the DHCPv6 FSM.
func IsKnownState(state State) bool {
	_, ok := allowedTransitions[state]
	return ok
}

// ValidateTransition enforces legal finite-state transitions.
func ValidateTransition(from, to State) error {
	if !IsKnownState(from) {
		return fmt.Errorf("dhcpv6 leasefsm: unknown from-state %q", from)
	}
	if !IsKnownState(to) {
		return fmt.Errorf("dhcpv6 leasefsm: unknown to-state %q", to)
	}
	if from == to {
		return nil
	}
	next := allowedTransitions[from]
	if _, ok := next[to]; !ok {
		return fmt.Errorf("dhcpv6 leasefsm: invalid transition %q -> %q", from, to)
	}
	return nil
}

// Next validates a state transition and returns the target state on success.
func Next(from, to State) (State, error) {
	if err := ValidateTransition(from, to); err != nil {
		return "", err
	}
	return to, nil
}
