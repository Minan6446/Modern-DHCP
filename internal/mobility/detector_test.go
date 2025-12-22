package mobility

import (
	"net"
	"testing"

	"modern-dhcp/internal/config"
)

func TestDetectorMatch(t *testing.T) {
	cfg := config.MobileProfilesConfig{
		Enabled: true,
		Profiles: []config.MobileProfileSpec{
			{
				Name:                "ios-default",
				Platform:            "ios",
				Persona:             "byod",
				Tags:                []string{"ios", "mobile"},
				VendorClassContains: []string{"iphone"},
				Option55Contains:    []int{1, 3},
			},
		},
	}
	detector := NewDetector(cfg)
	if detector == nil {
		t.Fatalf("expected detector")
	}
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	match, ok := detector.Match(DeviceSignals{
		VendorClass: "iPhone-17",
		Option55:    []int{1, 3, 6},
		MAC:         mac,
	})
	if !ok {
		t.Fatalf("expected match")
	}
	if match.Platform != "ios" || match.Persona != "byod" {
		t.Fatalf("unexpected match payload: %+v", match)
	}
	if len(match.Tags) != 2 {
		t.Fatalf("expected tags copied")
	}
}

func TestDetectorMismatch(t *testing.T) {
	cfg := config.MobileProfilesConfig{Enabled: true, Profiles: []config.MobileProfileSpec{{Name: "android", VendorClassContains: []string{"android"}}}}
	detector := NewDetector(cfg)
	match, ok := detector.Match(DeviceSignals{VendorClass: "windows"})
	if ok {
		t.Fatalf("unexpected match: %+v", match)
	}
}
