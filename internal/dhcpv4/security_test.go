package dhcpv4

import (
	"context"
	"net"
	"testing"

	"go.uber.org/zap"
)

func TestDHCPv4AllowByMACRateLimit(t *testing.T) {
	s := &Server{
		logger:     zap.NewNop(),
		macLimiter: dhcpv4NewMACRateLimiter(1),
	}
	msg := &Message{XID: 1, CHAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, Options: map[byte][]byte{}}
	remote := &net.UDPAddr{IP: net.ParseIP("10.1.1.1"), Port: 68}
	if !s.dhcpv4AllowByMAC(msg, remote) {
		t.Fatalf("first packet should pass limiter")
	}
	if s.dhcpv4AllowByMAC(msg, remote) {
		t.Fatalf("second immediate packet should be rate-limited")
	}
}

func TestDHCPv4RelayValidateRejectsNonWhitelistedRelay(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
		opts:   Options{ServerIP: net.IPv4(10, 0, 0, 1)},
		relayIPWhitelist: map[string]struct{}{
			"192.168.10.1": {},
		},
	}
	msg := &Message{XID: 99, CHAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, Options: map[byte][]byte{}}
	pkt := Packet{GIAddr: net.ParseIP("10.0.0.2"), Option82Present: true}
	remote := &net.UDPAddr{IP: net.ParseIP("10.10.10.10"), Port: 67}
	if s.dhcpv4RelayValidateOrReject(context.Background(), msg, pkt, remote) {
		t.Fatalf("non-whitelisted relay should be rejected")
	}
}

func TestDHCPv4RelayValidateAllowsWhitelistedRelay(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
		relayIPWhitelist: map[string]struct{}{
			"10.10.10.10": {},
		},
	}
	msg := &Message{XID: 100, CHAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01}, Options: map[byte][]byte{}}
	pkt := Packet{GIAddr: net.ParseIP("10.0.0.2"), Option82Present: true}
	remote := &net.UDPAddr{IP: net.ParseIP("10.10.10.10"), Port: 67}
	if !s.dhcpv4RelayValidateOrReject(context.Background(), msg, pkt, remote) {
		t.Fatalf("whitelisted relay should be allowed")
	}
}
