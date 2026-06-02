package dhcpv6

import (
	"net"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/pkg/models"
)

func TestPacketFromMessage_EnterpriseOptions(t *testing.T) {
	dnsPayload := encodeIPv6AddressListOption([]net.IP{net.ParseIP("2001:db8::53"), net.ParseIP("2001:db8::54")})
	domainPayload, err := encodeDomainSearchListOption([]string{"example.com", "corp.local"})
	if err != nil {
		t.Fatalf("encode domain search list failed: %v", err)
	}
	fqdnPayload, err := encodeFQDNOption("host01.example.com")
	if err != nil {
		t.Fatalf("encode fqdn failed: %v", err)
	}
	ntpPayload := encodeNTPServerOption([]net.IP{net.ParseIP("2001:db8::123")})

	msg := &Message{
		MessageType:   MessageTypeInformationReq,
		TransactionID: 0x100001,
		Options: map[uint16][][]byte{
			OptionDNSRecursiveNameServer: {dnsPayload},
			OptionDomainSearchList:       {domainPayload},
			OptionFQDN:                   {fqdnPayload},
			OptionNTPServer:              {ntpPayload},
		},
	}

	pkt, err := packetFromMessage(msg)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if len(pkt.DNSRecursiveServers) != 2 {
		t.Fatalf("expected 2 dns recursive servers, got %d", len(pkt.DNSRecursiveServers))
	}
	if len(pkt.DomainSearchList) != 2 {
		t.Fatalf("expected 2 domain search entries, got %d", len(pkt.DomainSearchList))
	}
	if pkt.FQDN != "host01.example.com" {
		t.Fatalf("expected fqdn host01.example.com, got %q", pkt.FQDN)
	}
	if len(pkt.NTPServers) != 1 || !pkt.NTPServers[0].Equal(net.ParseIP("2001:db8::123")) {
		t.Fatalf("expected one ntp server 2001:db8::123")
	}
}

func TestPacketFromMessage_EnterpriseOptionsMalformed(t *testing.T) {
	msg := &Message{
		MessageType:   MessageTypeInformationReq,
		TransactionID: 0x100002,
		Options: map[uint16][][]byte{
			OptionDNSRecursiveNameServer: {{0x20, 0x01, 0x0d}},
			OptionDomainSearchList:       {{64, 'a', 0}},
			OptionFQDN:                   {{0, 0, 0, 64, 'b', 0}},
			OptionNTPServer:              {{0, 1, 0, 16, 0, 1}},
		},
	}

	pkt, err := packetFromMessage(msg)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if len(pkt.DNSRecursiveServers) != 0 {
		t.Fatalf("expected malformed dns option to be ignored")
	}
	if len(pkt.DomainSearchList) != 0 {
		t.Fatalf("expected malformed domain search option to be ignored")
	}
	if pkt.FQDN != "" {
		t.Fatalf("expected malformed fqdn option to be ignored")
	}
	if len(pkt.NTPServers) != 0 {
		t.Fatalf("expected malformed ntp option to be ignored")
	}
}

func TestBuildResponse_EnterpriseOptions(t *testing.T) {
	srv := NewServer(Options{TenantID: "t1", ServerID: []byte{0, 3, 0, 1, 1, 2, 3, 4}}, nil, zap.NewNop())
	req := &Message{
		MessageType:   MessageTypeRequest,
		TransactionID: 0x223344,
		Options: map[uint16][][]byte{
			OptionClientID: {{0, 3, 0, 1, 1, 2, 3, 4, 5, 6}},
		},
	}
	pkt := Packet{
		DomainSearchList: []string{"example.com", "corp.local"},
		FQDN:             "host02.example.com",
		NTPServers:       []net.IP{net.ParseIP("2001:db8::200")},
		DNSRecursiveServers: []net.IP{
			net.ParseIP("2001:db8::fallback"),
		},
	}
	result := &lease.Result{
		Lease: &models.Lease{IPAddress: "2001:db8:1::100"},
		Pool:  &models.AddressPool{DNS: models.StringList{"2001:db8::53", "2001:db8::54"}},
		Profile: models.LeaseProfile{
			DefaultDuration: time.Hour,
			MaxDuration:     2 * time.Hour,
		},
	}

	resp, err := srv.buildResponse(req, pkt, result, MessageTypeReply, MessageTypeRequest)
	if err != nil {
		t.Fatalf("buildResponse failed: %v", err)
	}
	parsed, err := ParseMessage(resp)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}
	replyPkt, err := packetFromMessage(parsed)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if len(replyPkt.DNSRecursiveServers) != 2 {
		t.Fatalf("expected pool dns servers in response, got %d", len(replyPkt.DNSRecursiveServers))
	}
	if len(replyPkt.DomainSearchList) != 2 {
		t.Fatalf("expected domain search list in response")
	}
	if replyPkt.FQDN != "host02.example.com" {
		t.Fatalf("expected fqdn in response, got %q", replyPkt.FQDN)
	}
	if len(replyPkt.NTPServers) != 1 {
		t.Fatalf("expected ntp server option in response")
	}
}

func TestBuildResponse_EnterpriseOptionsInvalidConfig(t *testing.T) {
	srv := NewServer(Options{TenantID: "t1", ServerID: []byte{0, 3, 0, 1, 1, 2, 3, 4}}, nil, zap.NewNop())
	req := &Message{
		MessageType:   MessageTypeRequest,
		TransactionID: 0x223345,
		Options: map[uint16][][]byte{
			OptionClientID: {{0, 3, 0, 1, 1, 2, 3, 4, 5, 6}},
		},
	}
	pkt := Packet{
		DomainSearchList:    []string{"bad..example.com"},
		DNSRecursiveServers: []net.IP{net.ParseIP("2001:db8::999")},
	}
	result := &lease.Result{
		Lease: &models.Lease{IPAddress: "2001:db8:1::101"},
		Pool:  &models.AddressPool{DNS: models.StringList{"not-an-ip"}},
		Profile: models.LeaseProfile{
			DefaultDuration: time.Hour,
			MaxDuration:     2 * time.Hour,
		},
	}

	resp, err := srv.buildResponse(req, pkt, result, MessageTypeReply, MessageTypeRequest)
	if err != nil {
		t.Fatalf("buildResponse failed: %v", err)
	}
	parsed, err := ParseMessage(resp)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}
	if parsed.Option(OptionDomainSearchList) != nil {
		t.Fatalf("expected invalid domain search list to be omitted")
	}
	replyPkt, err := packetFromMessage(parsed)
	if err != nil {
		t.Fatalf("packetFromMessage failed: %v", err)
	}
	if len(replyPkt.DNSRecursiveServers) != 1 || !replyPkt.DNSRecursiveServers[0].Equal(net.ParseIP("2001:db8::999")) {
		t.Fatalf("expected fallback dns server from packet")
	}
}
