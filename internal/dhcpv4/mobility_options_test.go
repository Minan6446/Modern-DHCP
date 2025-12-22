package dhcpv4

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"

	leasepkg "modern-dhcp/internal/lease"
	"modern-dhcp/pkg/models"
)

func TestEncodeMobilityVendorOptionAnchorOnly(t *testing.T) {
	leaseObj := &models.Lease{
		PoolID:           "pool-1",
		MobilityAnchorID: "anchor-123",
		ExpiresAt:        time.Now().UTC(),
	}
	result := &leasepkg.Result{Lease: leaseObj, Profile: models.LeaseProfile{DefaultDuration: 5 * time.Minute}}
	payload := encodeMobilityVendorOption(result)
	if len(payload) == 0 {
		t.Fatalf("expected payload for anchor only")
	}
	enterprise := binary.BigEndian.Uint32(payload[:4])
	if enterprise != vendorEnterpriseID {
		t.Fatalf("enterprise mismatch: got %d", enterprise)
	}
	vendorDataLen := int(payload[4])
	vendorData := payload[5:]
	if vendorDataLen != len(vendorData) {
		t.Fatalf("length mismatch: header=%d actual=%d", vendorDataLen, len(vendorData))
	}
	if vendorData[0] != vendorSubOptionMobilityAnchorID {
		t.Fatalf("expected anchor sub-option, got %d", vendorData[0])
	}
	anchorLen := int(vendorData[1])
	anchor := string(vendorData[2 : 2+anchorLen])
	if anchor != "anchor-123" {
		t.Fatalf("unexpected anchor payload: %s", anchor)
	}
}

func TestEncodeMobilityVendorOptionSessionFallback(t *testing.T) {
	leaseObj := &models.Lease{
		PoolID:               "pool-2",
		LastControllerID:     "controller-7",
		MobilityLocationHint: "lobby-east",
		ExpiresAt:            time.Date(2024, 7, 12, 9, 0, 0, 0, time.UTC),
	}
	result := &leasepkg.Result{Lease: leaseObj, Profile: models.LeaseProfile{DefaultDuration: 90 * time.Second}}
	payload := encodeMobilityVendorOption(result)
	if len(payload) == 0 {
		t.Fatalf("expected payload for session fallback")
	}
	vendorData := payload[5:]
	if len(vendorData) == 0 {
		t.Fatalf("expected session sub-option payload")
	}
	if vendorData[0] != vendorSubOptionSessionContinuity {
		t.Fatalf("expected session sub-option, got %d", vendorData[0])
	}
	sessionLen := int(vendorData[1])
	sessionBytes := vendorData[2 : 2+sessionLen]
	var session map[string]any
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		t.Fatalf("session continuity json: %v", err)
	}
	if session["poolId"] != "pool-2" {
		t.Fatalf("expected poolId fallback, got %v", session["poolId"])
	}
	if session["controllerId"] != "controller-7" {
		t.Fatalf("expected controllerId in payload")
	}
}

func TestEncodeMobilityVendorOptionBudget(t *testing.T) {
	hugeAnchor := strings.Repeat("a", 200)
	hugeSession := json.RawMessage(strings.Repeat("b", 400))
	leaseObj := &models.Lease{
		PoolID:            "pool-3",
		MobilityAnchorID:  hugeAnchor,
		SessionContinuity: hugeSession,
		ExpiresAt:         time.Now().UTC(),
	}
	result := &leasepkg.Result{Lease: leaseObj}
	payload := encodeMobilityVendorOption(result)
	if len(payload) == 0 {
		t.Fatalf("expected payload for oversized inputs")
	}
	vendorDataLen := int(payload[4])
	if vendorDataLen > 255 {
		t.Fatalf("vendor data exceeds single-byte length: %d", vendorDataLen)
	}
}
