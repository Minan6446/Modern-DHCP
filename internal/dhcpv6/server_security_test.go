package dhcpv6

import (
	"context"
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRateLimitMiddlewareDropsExceededIdentity(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	srv := NewServer(Options{TenantID: "t1", RateLimitPPS: 1, ServerID: []byte{0, 1, 2, 3}}, nil, zap.New(core))

	before := testutil.ToFloat64(dhcpv6PacketRateLimitedTotal)
	payload := buildInformationRequestWithClientID([]byte{0, 1, 0, 1, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	remote := &net.UDPAddr{IP: net.ParseIP("2001:db8::10"), Port: 546}

	srv.processDatagram(context.Background(), payload, remote)
	srv.processDatagram(context.Background(), payload, remote)

	after := testutil.ToFloat64(dhcpv6PacketRateLimitedTotal)
	if after-before < 1 {
		t.Fatalf("expected rate-limited counter to increase, before=%v after=%v", before, after)
	}
	entries := logs.FilterMessage("dhcpv6 packet rate limit exceeded; dropping packet").All()
	if len(entries) == 0 {
		t.Fatalf("expected rate limit warning log")
	}
}

func TestRogueAdvertiseDetectionInProcessDatagram(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	srv := NewServer(Options{TenantID: "t1", ServerID: []byte{0, 1, 2, 3}, ClusterServerIDs: []string{"00010203"}}, nil, zap.New(core))

	before := testutil.ToFloat64(dhcpv6RogueServerDetectedTotal)
	rogue := buildAdvertiseWithServerID([]byte{0xaa, 0xbb, 0xcc, 0xdd})
	srv.processDatagram(context.Background(), rogue, &net.UDPAddr{IP: net.ParseIP("2001:db8::66"), Port: 547})

	after := testutil.ToFloat64(dhcpv6RogueServerDetectedTotal)
	if after-before < 1 {
		t.Fatalf("expected rogue counter to increase, before=%v after=%v", before, after)
	}
	entries := logs.FilterMessage("rogue dhcpv6 advertise detected").All()
	if len(entries) == 0 {
		t.Fatalf("expected rogue detection error log")
	}
}

func buildInformationRequestWithClientID(clientID []byte) []byte {
	options := appendOption(nil, OptionClientID, clientID)
	return buildBaseMessage(MessageTypeInformationReq, 0x112233, options)
}

func buildAdvertiseWithServerID(serverID []byte) []byte {
	options := appendOption(nil, OptionServerID, serverID)
	return buildBaseMessage(MessageTypeAdvertise, 0x010203, options)
}
