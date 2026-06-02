package dhcpv6

import (
	"context"
	"net"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRelayWhitelistAllowsConfiguredLinkAddr(t *testing.T) {
	srv := NewServer(Options{TenantID: "t1", RelayWhitelist: []string{"2001:db8::10"}}, nil, zap.NewNop())
	if !srv.isRelaySourceAllowed(net.ParseIP("2001:db8::10")) {
		t.Fatalf("expected relay source to be allowed")
	}
	if srv.isRelaySourceAllowed(net.ParseIP("2001:db8::11")) {
		t.Fatalf("expected relay source to be rejected")
	}
}

func TestProcessDatagramDropsIllegalRelaySource(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	srv := NewServer(Options{TenantID: "t1", RelayWhitelist: []string{"2001:db8::1"}}, nil, zap.New(core))
	payload := buildRelayForwardPacket(
		net.ParseIP("2001:db8::99"),
		net.ParseIP("2001:db8::100"),
		map[uint16][][]byte{OptionInterfaceID: {[]byte("uplink-a")}},
	)

	srv.processDatagram(context.Background(), payload, &net.UDPAddr{IP: net.ParseIP("2001:db8::200"), Port: 546})

	entries := logs.FilterMessage("dhcpv6 relay source rejected").All()
	if len(entries) != 1 {
		t.Fatalf("expected one relay rejection log entry, got %d", len(entries))
	}
}

func TestRelayHopChainCapturesAllHops(t *testing.T) {
	hops := []RelayHop{
		{
			HopCount: 1,
			LinkAddr: net.ParseIP("2001:db8:1::1"),
			PeerAddr: net.ParseIP("2001:db8:1::2"),
			Options: map[uint16][][]byte{
				OptionInterfaceID: {[]byte("if-a")},
			},
		},
		{
			HopCount: 2,
			LinkAddr: net.ParseIP("2001:db8:2::1"),
			PeerAddr: net.ParseIP("2001:db8:2::2"),
			Options: map[uint16][][]byte{
				OptionRemoteID: {append([]byte{0, 0, 0, 1}, []byte("relay-b")...)},
			},
		},
	}
	chain := relayHopChain(hops)
	if len(chain) != 2 {
		t.Fatalf("expected hop chain length 2, got %d", len(chain))
	}
	attrs0, _ := chain[0]["attributes"].(map[string]string)
	if attrs0["interface-id"] != "if-a" {
		t.Fatalf("expected first hop interface-id if-a, got %q", attrs0["interface-id"])
	}
	attrs1, _ := chain[1]["attributes"].(map[string]string)
	if attrs1["remote-id"] != "relay-b" {
		t.Fatalf("expected second hop remote-id relay-b, got %q", attrs1["remote-id"])
	}
}

func buildRelayForwardPacket(linkAddr net.IP, peerAddr net.IP, opts map[uint16][][]byte) []byte {
	inner := buildBaseMessage(MessageTypeInformationReq, 0x010203, nil)
	header := make([]byte, 34)
	header[0] = MessageTypeRelayForward
	header[1] = 0
	copy(header[2:18], padIPv6(linkAddr))
	copy(header[18:34], padIPv6(peerAddr))
	options := appendOption(nil, OptionRelayMsg, inner)
	for code, values := range opts {
		for _, value := range values {
			options = appendOption(options, code, value)
		}
	}
	return append(header, options...)
}
