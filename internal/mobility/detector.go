package mobility

import (
	"net"
	"strings"

	"modern-dhcp/internal/config"
)

// DeviceSignals captures raw hints used for fingerprinting.
type DeviceSignals struct {
	VendorClass string
	UserClass   string
	RelayInfo   map[string]string
	MAC         net.HardwareAddr
	Option55    []int
}

// ProfileMatch captures the semantic result of a fingerprint lookup.
type ProfileMatch struct {
	Name     string
	Platform string
	Persona  string
	Tags     []string
}

// Detector evaluates configured heuristics to classify mobile devices.
type Detector struct {
	enabled  bool
	profiles []compiledProfile
}

// NewDetector builds a detector from config. Returns nil if disabled or no profiles.
func NewDetector(cfg config.MobileProfilesConfig) *Detector {
	if !cfg.Enabled || len(cfg.Profiles) == 0 {
		return nil
	}
	compiled := make([]compiledProfile, 0, len(cfg.Profiles))
	for _, spec := range cfg.Profiles {
		profile := compiledProfile{
			name:      strings.TrimSpace(spec.Name),
			platform:  strings.TrimSpace(strings.ToLower(spec.Platform)),
			persona:   strings.TrimSpace(strings.ToLower(spec.Persona)),
			tags:      normalizeList(spec.Tags),
			vendorSet: normalizeList(spec.VendorClassContains),
			userSet:   normalizeList(spec.UserClassContains),
			relaySet:  normalizeList(spec.RelayKeywords),
			ouis:      normalizeList(spec.OUIs),
		}
		if len(spec.Option55Contains) > 0 {
			profile.option55 = make(map[int]struct{}, len(spec.Option55Contains))
			for _, code := range spec.Option55Contains {
				if code <= 0 {
					continue
				}
				profile.option55[code] = struct{}{}
			}
		}
		if profile.name == "" {
			profile.name = profile.platform
		}
		compiled = append(compiled, profile)
	}
	if len(compiled) == 0 {
		return nil
	}
	return &Detector{enabled: true, profiles: compiled}
}

// Match returns the first profile that satisfies all configured criteria.
func (d *Detector) Match(sig DeviceSignals) (ProfileMatch, bool) {
	var empty ProfileMatch
	if d == nil || !d.enabled {
		return empty, false
	}
	vendor := strings.ToLower(sig.VendorClass)
	user := strings.ToLower(sig.UserClass)
	relayBlob := flattenRelay(sig.RelayInfo)
	oui := deriveOUI(sig.MAC)

	for _, profile := range d.profiles {
		if !containsAllTokens(vendor, profile.vendorSet) {
			continue
		}
		if !containsAllTokens(user, profile.userSet) {
			continue
		}
		if !relayHasKeywords(relayBlob, profile.relaySet) {
			continue
		}
		if !ouiMatches(oui, profile.ouis) {
			continue
		}
		if len(profile.option55) > 0 && !option55Superset(sig.Option55, profile.option55) {
			continue
		}
		match := ProfileMatch{
			Name:     profile.name,
			Platform: profile.platform,
			Persona:  profile.persona,
			Tags:     append([]string(nil), profile.tags...),
		}
		return match, true
	}
	return empty, false
}

type compiledProfile struct {
	name     string
	platform string
	persona  string
	tags     []string

	vendorSet []string
	userSet   []string
	relaySet  []string
	ouis      []string
	option55  map[int]struct{}
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, val := range values {
		trimmed := strings.TrimSpace(strings.ToLower(val))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func flattenRelay(relay map[string]string) string {
	if len(relay) == 0 {
		return ""
	}
	builder := strings.Builder{}
	for _, val := range relay {
		if val == "" {
			continue
		}
		builder.WriteString(strings.ToLower(val))
		builder.WriteRune(' ')
	}
	return builder.String()
}

func deriveOUI(mac net.HardwareAddr) string {
	if len(mac) < 3 {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(mac.String()[0:8], ":", ""))
}

func containsAllTokens(source string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		if !strings.Contains(source, token) {
			return false
		}
	}
	return true
}

func relayHasKeywords(relay string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		if !strings.Contains(relay, token) {
			return false
		}
	}
	return true
}

func ouiMatches(actual string, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	if actual == "" {
		return false
	}
	for _, candidate := range expected {
		normalized := strings.ReplaceAll(candidate, ":", "")
		if normalized == "" {
			continue
		}
		if strings.HasPrefix(actual, strings.ToLower(normalized)) {
			return true
		}
	}
	return false
}

func option55Superset(codes []int, required map[int]struct{}) bool {
	if len(required) == 0 {
		return true
	}
	have := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		have[code] = struct{}{}
	}
	for code := range required {
		if _, ok := have[code]; !ok {
			return false
		}
	}
	return true
}
