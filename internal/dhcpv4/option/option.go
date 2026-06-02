package option

import (
	"encoding/binary"
	"fmt"
	"strings"

	"modern-dhcp/internal/dhcpv4/consts"
	dhcpv4errors "modern-dhcp/internal/dhcpv4/errors"
)

const (
	Option82SubOptionCircuitID    byte = 1
	Option82SubOptionRemoteID     byte = 2
	Option82SubOptionSubscriberID byte = 6
)

type Option82Policy struct {
	Enabled        bool
	CircuitIDAllow []string
	CircuitIDDeny  []string
	RemoteIDAllow  []string
	RemoteIDDeny   []string
}

type Option82Parsed struct {
	CircuitID    string
	RemoteID     string
	SubscriberID string
}

type Option82Validator interface {
	Validate(present bool, parseErr, circuitID, remoteID string) error
}

type Validator struct {
	enabled        bool
	circuitIDAllow map[string]struct{}
	circuitIDDeny  map[string]struct{}
	remoteIDAllow  map[string]struct{}
	remoteIDDeny   map[string]struct{}
}

func NewOption82Validator(policy Option82Policy) *Validator {
	return &Validator{
		enabled:        policy.Enabled,
		circuitIDAllow: normalizeSet(policy.CircuitIDAllow),
		circuitIDDeny:  normalizeSet(policy.CircuitIDDeny),
		remoteIDAllow:  normalizeSet(policy.RemoteIDAllow),
		remoteIDDeny:   normalizeSet(policy.RemoteIDDeny),
	}
}

func normalizeSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func (v *Validator) Validate(present bool, parseErr, circuitID, remoteID string) error {
	if present && strings.TrimSpace(parseErr) != "" {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeOption82Invalid, "invalid option82 format", fmt.Errorf("%w: %s", dhcpv4errors.ErrOption82Invalid, parseErr))
	}
	if v == nil || !v.enabled || !present {
		return nil
	}
	if v.matchDeny(circuitID, v.circuitIDDeny) {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeOption82Invalid, "option82 circuit-id denied", fmt.Errorf("circuit-id=%q", circuitID))
	}
	if v.matchDeny(remoteID, v.remoteIDDeny) {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeOption82Invalid, "option82 remote-id denied", fmt.Errorf("remote-id=%q", remoteID))
	}
	if !v.matchAllow(circuitID, v.circuitIDAllow) {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeOption82Invalid, "option82 circuit-id not allowed", fmt.Errorf("circuit-id=%q", circuitID))
	}
	if !v.matchAllow(remoteID, v.remoteIDAllow) {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeOption82Invalid, "option82 remote-id not allowed", fmt.Errorf("remote-id=%q", remoteID))
	}
	return nil
}

func (v *Validator) matchDeny(value string, deny map[string]struct{}) bool {
	if len(deny) == 0 {
		return false
	}
	_, exists := deny[strings.TrimSpace(value)]
	return exists
}

func (v *Validator) matchAllow(value string, allow map[string]struct{}) bool {
	if len(allow) == 0 {
		return true
	}
	_, exists := allow[strings.TrimSpace(value)]
	return exists
}

func ParseOption82(data []byte) (Option82Parsed, error) {
	parsed := Option82Parsed{}
	if len(data) == 0 {
		return parsed, nil
	}
	for i := 0; i < len(data); {
		if i+1 >= len(data) {
			return Option82Parsed{}, fmt.Errorf("truncated sub-option header at offset %d", i)
		}
		code := data[i]
		length := int(data[i+1])
		i += 2
		if i+length > len(data) {
			return Option82Parsed{}, fmt.Errorf("truncated sub-option %d", code)
		}
		value := data[i : i+length]
		i += length
		switch code {
		case Option82SubOptionCircuitID:
			if parsed.CircuitID == "" {
				parsed.CircuitID = SanitizeASCII(value)
			}
		case Option82SubOptionRemoteID:
			if parsed.RemoteID == "" {
				parsed.RemoteID = SanitizeASCII(value)
			}
		case Option82SubOptionSubscriberID:
			if parsed.SubscriberID == "" {
				parsed.SubscriberID = SanitizeASCII(value)
			}
		}
	}
	return parsed, nil
}

func SanitizeASCII(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(value))
	for _, b := range value {
		if b >= 32 && b <= 126 {
			buf = append(buf, b)
		}
	}
	return strings.TrimSpace(string(buf))
}

func DecodeUserClass(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var classes []string
	for i := 0; i < len(data); {
		l := int(data[i])
		i++
		if l == 0 || i+l > len(data) {
			break
		}
		classes = append(classes, SanitizeASCII(data[i:i+l]))
		i += l
	}
	return strings.Join(classes, ",")
}

func EncodeUint32(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return buf
}

func ParseVendorClass(data []byte) string { return SanitizeASCII(data) }

func BuildVendorOption43(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}
	if len(payload) > 255 {
		payload = payload[:255]
	}
	out := make([]byte, len(payload))
	copy(out, payload)
	return out
}

func BuildOption121ClasslessStaticRoutes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func IsDHCPMessageTypeRequest(mt []byte) bool {
	return len(mt) == 1 && mt[0] == consts.MessageTypeRequest
}
