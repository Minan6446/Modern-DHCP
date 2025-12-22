package relay

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"modern-dhcp/internal/config"
)

// Option82Parser normalizes relay agent information options.
type Option82Parser interface {
	Decode(data []byte) (map[string]string, error)
}

type option82Parser struct {
	preserveRaw bool
	mappings    map[byte]subOptionMapping
}

type subOptionMapping struct {
	key     string
	format  string
	aliases []string
}

var _ Option82Parser = (*option82Parser)(nil)

// NewOption82Parser builds a parser using defaults plus user overrides.
func NewOption82Parser(cfg config.RelayOption82Config) Option82Parser {
	parser := &option82Parser{
		preserveRaw: cfg.PreserveRaw,
		mappings:    make(map[byte]subOptionMapping),
	}
	for code, mapping := range defaultSubOptionMappings {
		parser.mappings[code] = mapping
	}
	for codeStr, spec := range cfg.SubOptionMappings {
		code, err := strconv.Atoi(strings.TrimSpace(codeStr))
		if err != nil {
			continue
		}
		if code < 0 || code > 255 {
			continue
		}
		parser.mappings[byte(code)] = subOptionMapping{
			key:     strings.TrimSpace(spec.Key),
			format:  strings.ToLower(strings.TrimSpace(spec.Format)),
			aliases: normalizeAliases(spec.Aliases),
		}
	}
	return parser
}

// NewDefaultOption82Parser returns a parser with built-in defaults.
func NewDefaultOption82Parser() Option82Parser {
	return NewOption82Parser(config.RelayOption82Config{})
}

// Decode parses Option 82 TLVs into metadata keys.
func (p *option82Parser) Decode(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	attrs := make(map[string]string)
	var decodeErr error
	for i := 0; i+1 < len(data); {
		code := data[i]
		length := int(data[i+1])
		i += 2
		if i+length > len(data) {
			decodeErr = fmt.Errorf("option82: truncated sub-option %d", code)
			break
		}
		value := data[i : i+length]
		p.applyMapping(attrs, code, value)
		i += length
	}
	if p.preserveRaw {
		attrs["option82.raw"] = hex.EncodeToString(data)
	}
	return attrs, decodeErr
}

func (p *option82Parser) applyMapping(dest map[string]string, code byte, raw []byte) {
	if len(raw) == 0 {
		return
	}
	mapping, ok := p.mappings[code]
	if !ok || strings.TrimSpace(mapping.key) == "" {
		key := fmt.Sprintf("subopt_%d", code)
		if _, exists := dest[key]; !exists {
			dest[key] = hex.EncodeToString(raw)
		}
		return
	}
	val := decodeValue(raw, mapping.format)
	if val == "" {
		return
	}
	dest[mapping.key] = val
	for _, alias := range mapping.aliases {
		if alias == "" {
			continue
		}
		if _, exists := dest[alias]; !exists {
			dest[alias] = val
		}
	}
}

func decodeValue(raw []byte, format string) string {
	switch format {
	case "", "ascii", "string":
		return sanitizeASCII(raw)
	case "hex":
		return hex.EncodeToString(raw)
	case "ipv4":
		if len(raw) >= 4 {
			if ip := net.IP(raw[:4]).To4(); ip != nil {
				return ip.String()
			}
		}
		return hex.EncodeToString(raw)
	case "int", "uint", "number":
		if len(raw) == 0 {
			return ""
		}
		var val uint64
		for _, b := range raw {
			val = (val << 8) | uint64(b)
		}
		return strconv.FormatUint(val, 10)
	case "vss", "vpn":
		return decodeVSS(raw)
	default:
		return sanitizeASCII(raw)
	}
}

func decodeVSS(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	vssType := raw[0]
	payload := raw[1:]
	switch vssType {
	case 1: // ASCII identifier
		return sanitizeASCII(payload)
	case 2: // 4-octet numeric ID
		if len(payload) >= 4 {
			val := binary.BigEndian.Uint32(payload[:4])
			return strconv.FormatUint(uint64(val), 10)
		}
		return hex.EncodeToString(payload)
	default:
		return hex.EncodeToString(payload)
	}
}

func sanitizeASCII(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range data {
		if c >= 32 && c <= 126 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func normalizeAliases(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, 0, len(src))
	for _, alias := range src {
		alias = strings.TrimSpace(alias)
		if alias != "" {
			out = append(out, alias)
		}
	}
	return out
}

var defaultSubOptionMappings = map[byte]subOptionMapping{
	1:   {key: "circuit-id", format: "string", aliases: []string{"agent.circuit-id", "port-id"}},
	2:   {key: "remote-id", format: "string", aliases: []string{"agent.remote-id", "location"}},
	5:   {key: "link-selection", format: "ipv4", aliases: []string{"giaddr.link"}},
	6:   {key: "subscriber-id", format: "string", aliases: []string{"user-id", "subscriber"}},
	9:   {key: "vendor.raw", format: "hex"},
	151: {key: "vpn-id", format: "vss", aliases: []string{"mpls.vpn-id"}},
	152: {key: "vss-bootstrap", format: "string"},
}
