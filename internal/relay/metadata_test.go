package relay

import (
	"net"
	"testing"
)

func TestBuildMetadata(t *testing.T) {
	attrs := map[string]string{
		"circuit-id": "eth1/1",
		"remote-id":  "agg-core",
		"vlan-id":    "200",
		"vpn-id":     "65000:100",
		"region":     "dc-east",
		"datacenter": "rack-12",
	}
	meta := BuildMetadata("tenant-a", net.ParseIP("10.0.0.1"), attrs, 0, []string{"gold"})
	if meta.RelayID != "agg-core" {
		t.Fatalf("expected relay id agg-core, got %s", meta.RelayID)
	}
	if meta.VLANID != 200 {
		t.Fatalf("expected vlan 200, got %d", meta.VLANID)
	}
	if meta.VPNID != "65000:100" {
		t.Fatalf("expected vpn 65000:100, got %s", meta.VPNID)
	}
	if meta.Region != "dc-east" {
		t.Fatalf("expected region dc-east, got %s", meta.Region)
	}
	if meta.DataCenter != "rack-12" {
		t.Fatalf("expected data center rack-12, got %s", meta.DataCenter)
	}
	if len(meta.UserGroups) != 1 || meta.UserGroups[0] != "gold" {
		t.Fatalf("expected user group gold, got %+v", meta.UserGroups)
	}
}
