package leasefsm

import "fmt"

func ExampleValidateTransition() {
	fmt.Println(ValidateTransition(StateInit, StateOffered) == nil)
	fmt.Println(ValidateTransition(StateOffered, StateReleased) == nil)
	// Output:
	// true
	// false
}
