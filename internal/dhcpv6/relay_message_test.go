package dhcpv6

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestRelayHopAttributesInterfaceAndRemoteID(t *testing.T) {
	remote := make([]byte, 4)
	binary.BigEndian.PutUint32(remote, 64512)
	remote = append(remote, []byte("edge-relay-1")...)

	msg := &Message{
		MessageType:   MessageTypeRequest,
		TransactionID: 0x010203,
		RelayHops: []RelayHop{
			{
				HopCount: 1,
				LinkAddr: net.ParseIP("2001:db8:1::1"),
				PeerAddr: net.ParseIP("2001:db8:ffff::10"),
				Options: map[uint16][][]byte{
					OptionInterfaceID: {[]byte("ge-0/0/1.100")},
					OptionRemoteID:    {remote},
				},
			},
		},
	}

	pkt, err := packetFromMessage(msg)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if got := pkt.RelayInfo["interface-id"]; got != "ge-0/0/1.100" {
		t.Fatalf("expected interface-id ge-0/0/1.100, got %q", got)
	}
	if got := pkt.RelayInfo["agent.circuit-id"]; got != "ge-0/0/1.100" {
		t.Fatalf("expected agent.circuit-id ge-0/0/1.100, got %q", got)
	}
	if got := pkt.RelayInfo["remote-id"]; got != "edge-relay-1" {
		t.Fatalf("expected remote-id edge-relay-1, got %q", got)
	}
	if got := pkt.RelayInfo["remote-id-enterprise"]; got != "64512" {
		t.Fatalf("expected remote-id-enterprise 64512, got %q", got)
	}
	if got := pkt.RelayInfo["location"]; got != "edge-relay-1" {
		t.Fatalf("expected location fallback to remote-id, got %q", got)
	}
}

func TestPacketFromMessageRelayInfoIncludesHopChain(t *testing.T) {
	msg := &Message{
		MessageType:   MessageTypeRequest,
		TransactionID: 0x0a0b0c,
		RelayHops: []RelayHop{
			{
				HopCount: 1,
				LinkAddr: net.ParseIP("2001:db8:1::1"),
				PeerAddr: net.ParseIP("2001:db8:1::2"),
				Options: map[uint16][][]byte{
					OptionInterfaceID: {[]byte("uplink-1")},
				},
			},
			{
				HopCount: 2,
				LinkAddr: net.ParseIP("2001:db8:2::1"),
				PeerAddr: net.ParseIP("2001:db8:2::2"),
				Options: map[uint16][][]byte{
					OptionRemoteID: {append([]byte{0, 0, 0, 1}, []byte("branch-22")...)},
				},
			},
		},
	}

	pkt, err := packetFromMessage(msg)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if got := pkt.RelayInfo["relay-hop-count"]; got != "2" {
		t.Fatalf("expected relay-hop-count 2, got %q", got)
	}
	if got := pkt.RelayInfo["relay-hop.0.interface-id"]; got != "uplink-1" {
		t.Fatalf("expected hop 0 interface-id uplink-1, got %q", got)
	}
	if got := pkt.RelayInfo["relay-hop.1.remote-id"]; got != "branch-22" {
		t.Fatalf("expected hop 1 remote-id branch-22, got %q", got)
	}
}
