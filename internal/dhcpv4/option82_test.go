package dhcpv4

import "testing"

func TestDHCPv4Option82ParseRFC3046SubOptions(t *testing.T) {
	raw := []byte{
		1, 6, 'p', 'o', 'r', 't', '-', '1',
		2, 4, 'c', 'o', 'r', 'e',
		6, 3, 'u', 's', 'r',
	}
	parsed, err := dhcpv4Option82Parse(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}
	if parsed.CircuitID != "port-1" {
		t.Fatalf("unexpected circuit-id: %q", parsed.CircuitID)
	}
	if parsed.RemoteID != "core" {
		t.Fatalf("unexpected remote-id: %q", parsed.RemoteID)
	}
	if parsed.SubscriberID != "usr" {
		t.Fatalf("unexpected subscriber-id: %q", parsed.SubscriberID)
	}
}

func TestDHCPv4Option82ParseMalformedTLV(t *testing.T) {
	raw := []byte{1, 5, 'p', 'o'}
	_, err := dhcpv4Option82Parse(raw)
	if err == nil {
		t.Fatalf("expected malformed option82 parse error")
	}
}

func TestDHCPv4Option82WhitelistAllowMatch(t *testing.T) {
	validator := dhcpv4Option82NewValidator(Option82Policy{
		Enabled:        true,
		CircuitIDAllow: []string{"port-1"},
		RemoteIDAllow:  []string{"core"},
	})
	pkt := Packet{
		Option82Present:   true,
		Option82CircuitID: "port-1",
		Option82RemoteID:  "core",
	}
	if err := validator.dhcpv4Option82Validate(pkt); err != nil {
		t.Fatalf("expected whitelist allow, got error: %v", err)
	}
}

func TestDHCPv4Option82WhitelistDenyMatch(t *testing.T) {
	validator := dhcpv4Option82NewValidator(Option82Policy{
		Enabled:       true,
		CircuitIDDeny: []string{"port-9"},
	})
	pkt := Packet{
		Option82Present:   true,
		Option82CircuitID: "port-9",
	}
	if err := validator.dhcpv4Option82Validate(pkt); err == nil {
		t.Fatalf("expected deny match error")
	}
}

func TestDHCPv4Option82ValidateRejectsMalformedPayload(t *testing.T) {
	validator := dhcpv4Option82NewValidator(Option82Policy{Enabled: true})
	pkt := Packet{
		Option82Present: true,
		Option82Error:   "truncated sub-option 1",
	}
	if err := validator.dhcpv4Option82Validate(pkt); err == nil {
		t.Fatalf("expected malformed option82 rejection")
	}
}
