package relay

import (
	"testing"

	"modern-dhcp/internal/config"
)

func TestOption82ParserDefaults(t *testing.T) {
	parser := NewOption82Parser(config.RelayOption82Config{})
	data := []byte{
		1, 6, 'p', 'o', 'r', 't', '-', '1',
		2, 4, 'c', 'o', 'r', 'e',
		6, 3, 'u', 's', 'r',
	}
	attrs, err := parser.Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := attrs["circuit-id"]; got != "port-1" {
		t.Fatalf("unexpected circuit-id: %s", got)
	}
	if got := attrs["agent.circuit-id"]; got != "port-1" {
		t.Fatalf("alias not populated: %s", got)
	}
	if got := attrs["remote-id"]; got != "core" {
		t.Fatalf("unexpected remote-id: %s", got)
	}
	if got := attrs["user-id"]; got != "usr" {
		t.Fatalf("unexpected user-id: %s", got)
	}
}

func TestOption82ParserCustomMapping(t *testing.T) {
	cfg := config.RelayOption82Config{
		SubOptionMappings: map[string]config.RelaySubOptionSpec{
			"150": {Key: "vlan-id", Format: "int", Aliases: []string{"agent.vlan-id"}},
		},
	}
	parser := NewOption82Parser(cfg)
	data := []byte{150, 2, 0, 10}
	attrs, err := parser.Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := attrs["vlan-id"]; got != "10" {
		t.Fatalf("expected vlan-id 10, got %s", got)
	}
	if got := attrs["agent.vlan-id"]; got != "10" {
		t.Fatalf("alias not populated: %s", got)
	}
}

func TestOption82ParserTruncated(t *testing.T) {
	parser := NewOption82Parser(config.RelayOption82Config{})
	data := []byte{1, 4, 'b', 'a'}
	attrs, err := parser.Decode(data)
	if err == nil {
		t.Fatalf("expected error for truncated payload")
	}
	if len(attrs) != 0 {
		t.Fatalf("expected empty attrs on truncation, got %+v", attrs)
	}
}
