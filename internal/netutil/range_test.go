package netutil

import (
	"net/netip"
	"testing"
)

func TestDefaultHostRangeIPv4(t *testing.T) {
	prefix := netip.MustParsePrefix("10.0.0.0/29")
	start, end, err := DefaultHostRange(prefix)
	if err != nil {
		t.Fatalf("DefaultHostRange error: %v", err)
	}
	if start.String() != "10.0.0.1" || end.String() != "10.0.0.6" {
		t.Fatalf("unexpected range %s-%s", start, end)
	}
}
