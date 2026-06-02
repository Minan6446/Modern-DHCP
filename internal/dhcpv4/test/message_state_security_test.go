package test

import (
	"testing"

	dhcpv4 "modern-dhcp/internal/dhcpv4"
	"modern-dhcp/internal/dhcpv4/leasefsm"
)

func TestMessageParseAndMarshalRoundTrip(t *testing.T) {
	msg := &dhcpv4.Message{
		Op:     1,
		HType:  1,
		HLen:   6,
		XID:    0x12345678,
		CHAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Options: map[byte][]byte{
			dhcpv4.OptionDHCPMessageType: {dhcpv4.MessageTypeDiscover},
		},
	}
	wire, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := dhcpv4.ParseMessage(wire)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if decoded.XID != msg.XID {
		t.Fatalf("xid mismatch: got=%d want=%d", decoded.XID, msg.XID)
	}
	if got := decoded.Option(dhcpv4.OptionDHCPMessageType); len(got) != 1 || got[0] != dhcpv4.MessageTypeDiscover {
		t.Fatalf("message type mismatch: %v", got)
	}
}

func TestLeaseFSMTransitions(t *testing.T) {
	if err := leasefsm.ValidateTransition(leasefsm.StateOffered, leasefsm.StateBound); err != nil {
		t.Fatalf("expected valid transition: %v", err)
	}
	if err := leasefsm.ValidateTransition(leasefsm.StateOffered, leasefsm.StateReleased); err == nil {
		t.Fatalf("expected invalid transition Offered->Released")
	}
}
