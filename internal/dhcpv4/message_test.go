package dhcpv4

import (
	"bytes"
	"net"
	"testing"
)

func TestMessageMarshalRoundTrip(t *testing.T) {
	msg := &Message{
		Op:    opBootRequest,
		HType: 1,
		HLen:  byte(len([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})),
		XID:   0xdeadbeef,
		CHAddr: net.HardwareAddr{
			0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
		},
		CIAddr: net.IPv4(10, 0, 0, 10),
		Options: map[byte][]byte{
			OptionDHCPMessageType:  {MessageTypeDiscover},
			OptionClientIdentifier: {1, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
	}

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	if parsed.XID != msg.XID {
		t.Fatalf("XID mismatch: got %x want %x", parsed.XID, msg.XID)
	}
	if !parsed.CIAddr.Equal(msg.CIAddr) {
		t.Fatalf("CIAddr mismatch: got %v want %v", parsed.CIAddr, msg.CIAddr)
	}
	if !bytes.Equal(parsed.Options[OptionClientIdentifier], msg.Options[OptionClientIdentifier]) {
		t.Fatalf("client identifier mismatch")
	}
}

func TestDecodeUserClass(t *testing.T) {
	payload := []byte{2, 'I', 'o', 3, 'I', 'o', 'T'}
	val := decodeUserClass(payload)
	if val != "Io,IoT" {
		t.Fatalf("unexpected user class: %s", val)
	}
}
