package dhcpv6

import (
	"net"
	"testing"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/mobility"
)

func TestDeviceClassificationUsesFingerprinter(t *testing.T) {
	detector := mobility.NewDetector(config.MobileProfilesConfig{
		Enabled: true,
		Profiles: []config.MobileProfileSpec{
			{
				Name:                "ios",
				Platform:            "ios",
				Persona:             "byod",
				Tags:                []string{"mobile", "ios"},
				VendorClassContains: []string{"iphone"},
			},
		},
	})
	if detector == nil {
		t.Fatalf("expected detector")
	}
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	handler := &Handler{fingerprinter: detector}
	deviceType, persona, tags := handler.deviceClassification(Packet{VendorClass: "iPhone-17", ClientMAC: mac})
	if deviceType != "ios" {
		t.Fatalf("expected ios deviceType, got %q", deviceType)
	}
	if persona != "byod" {
		t.Fatalf("expected persona propagated, got %q", persona)
	}
	if len(tags) != 2 {
		t.Fatalf("expected tags propagated")
	}
}
