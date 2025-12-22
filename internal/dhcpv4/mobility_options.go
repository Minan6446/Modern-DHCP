package dhcpv4

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"

	"modern-dhcp/internal/lease"
	"modern-dhcp/pkg/models"
)

const (
	vendorEnterpriseID               uint32 = 32473
	vendorSubOptionMobilityAnchorID  byte   = 1
	vendorSubOptionSessionContinuity byte   = 2
)

func encodeMobilityVendorOption(result *lease.Result) []byte {
	if result == nil || result.Lease == nil {
		return nil
	}
	anchor := strings.TrimSpace(result.Lease.MobilityAnchorID)
	session := sessionContinuityPayload(result.Lease, result.Profile)
	if anchor == "" && len(session) == 0 {
		return nil
	}
	var suboptions []vendorSubOption
	if anchor != "" {
		suboptions = append(suboptions, vendorSubOption{Code: vendorSubOptionMobilityAnchorID, Value: []byte(anchor)})
	}
	if len(session) > 0 {
		suboptions = append(suboptions, vendorSubOption{Code: vendorSubOptionSessionContinuity, Value: session})
	}
	payload := buildVendorData(suboptions...)
	if len(payload) == 0 {
		return nil
	}
	buf := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], vendorEnterpriseID)
	buf[4] = byte(len(payload))
	copy(buf[5:], payload)
	return buf
}

type vendorSubOption struct {
	Code  byte
	Value []byte
}

func buildVendorData(subs ...vendorSubOption) []byte {
	remaining := 255
	data := make([]byte, 0, len(subs)*8)
	for _, sub := range subs {
		if remaining <= 2 || len(sub.Value) == 0 {
			continue
		}
		chunk := truncateForBudget(sub.Value, remaining-2)
		if len(chunk) == 0 {
			continue
		}
		data = append(data, sub.Code, byte(len(chunk)))
		data = append(data, chunk...)
		remaining -= 2 + len(chunk)
	}
	return data
}

func truncateForBudget(value []byte, budget int) []byte {
	if budget <= 0 {
		return nil
	}
	if len(value) > budget {
		trimmed := make([]byte, budget)
		copy(trimmed, value[:budget])
		return trimmed
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}

func sessionContinuityPayload(lease *models.Lease, profile models.LeaseProfile) []byte {
	if lease == nil {
		return nil
	}
	if len(lease.SessionContinuity) > 0 {
		payload := make([]byte, len(lease.SessionContinuity))
		copy(payload, lease.SessionContinuity)
		return payload
	}
	data := map[string]any{
		"poolId":         lease.PoolID,
		"leaseExpiresAt": lease.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if profile.DefaultDuration > 0 {
		data["leaseSeconds"] = int(profile.DefaultDuration.Seconds())
	}
	if lease.MobilityAnchorID != "" {
		data["anchorId"] = lease.MobilityAnchorID
	}
	if lease.LastControllerID != "" {
		data["controllerId"] = lease.LastControllerID
	}
	if lease.LastGeoZone != "" {
		data["geoZone"] = lease.LastGeoZone
	}
	if lease.MobilityLocationHint != "" {
		data["location"] = lease.MobilityLocationHint
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return payload
}
