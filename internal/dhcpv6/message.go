package dhcpv6

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
)

const (
	MessageTypeSolicit        = 1
	MessageTypeAdvertise      = 2
	MessageTypeRequest        = 3
	MessageTypeConfirm        = 4
	MessageTypeRenew          = 5
	MessageTypeRebind         = 6
	MessageTypeReply          = 7
	MessageTypeRelease        = 8
	MessageTypeDecline        = 9
	MessageTypeReconfigure    = 10
	MessageTypeInformationReq = 11
	MessageTypeRelayForward   = 12
	MessageTypeRelayReply     = 13
)

const (
	OptionClientID    = 1
	OptionServerID    = 2
	OptionIANA        = 3
	OptionIATA        = 4
	OptionIAAddr      = 5
	OptionORO         = 6
	OptionPreference  = 7
	OptionRelayMsg    = 9
	OptionUserClass   = 15
	OptionVendorClass = 16
	OptionInterfaceID = 18
	OptionRemoteID    = 37
	OptionIAPD        = 25
	OptionIAPrefix    = 26
	// OptionDNSRecursiveNameServer defines DNS Recursive Name Server option (RFC 3646 section 3).
	OptionDNSRecursiveNameServer = 23
	// OptionDomainSearchList defines Domain Search List option (RFC 3646 section 4).
	OptionDomainSearchList = 24
	// OptionFQDN defines Client FQDN option (RFC 4704 section 4.1).
	OptionFQDN = 39
	// OptionNTPServer defines NTP Server option (RFC 5908 section 4).
	OptionNTPServer = 56
)

var (
	errShortMessage    = errors.New("dhcpv6: packet too short")
	errMissingRelayMsg = errors.New("dhcpv6: missing relay-message option")
)

// Message represents a decoded DHCPv6 client message (non-relay) along with relay path.
type Message struct {
	MessageType   byte
	TransactionID uint32
	Options       map[uint16][][]byte
	RelayHops     []RelayHop
}

// RelayHop captures a relay-forward hop sequence.
type RelayHop struct {
	HopCount byte
	LinkAddr net.IP
	PeerAddr net.IP
	Options  map[uint16][][]byte
}

// ParseMessage decodes raw bytes into a Message, handling relay encapsulation when present.
func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 4 {
		return nil, errShortMessage
	}
	msgType := data[0]
	if msgType == MessageTypeRelayForward || msgType == MessageTypeRelayReply {
		return parseRelay(data)
	}
	txID := uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	opts, err := parseOptions(data[4:])
	if err != nil {
		return nil, err
	}
	return &Message{MessageType: msgType, TransactionID: txID, Options: opts}, nil
}

func parseRelay(data []byte) (*Message, error) {
	if len(data) < 34 {
		return nil, errShortMessage
	}
	hop := RelayHop{
		HopCount: data[1],
		LinkAddr: append(net.IP(nil), data[2:18]...),
		PeerAddr: append(net.IP(nil), data[18:34]...),
	}
	opts, err := parseOptions(data[34:])
	if err != nil {
		return nil, err
	}
	hop.Options = opts
	relayMsgs := opts[OptionRelayMsg]
	if len(relayMsgs) == 0 {
		return nil, errMissingRelayMsg
	}
	inner, err := ParseMessage(relayMsgs[0])
	if err != nil {
		return nil, err
	}
	inner.RelayHops = append([]RelayHop{hop}, inner.RelayHops...)
	return inner, nil
}

func parseOptions(data []byte) (map[uint16][][]byte, error) {
	opts := make(map[uint16][][]byte)
	for len(data) > 0 {
		if len(data) < 4 {
			return nil, errors.New("dhcpv6: truncated option header")
		}
		code := binary.BigEndian.Uint16(data[0:2])
		length := int(binary.BigEndian.Uint16(data[2:4]))
		data = data[4:]
		if len(data) < length {
			return nil, errors.New("dhcpv6: truncated option data")
		}
		val := make([]byte, length)
		copy(val, data[:length])
		opts[code] = append(opts[code], val)
		data = data[length:]
	}
	return opts, nil
}

func (m *Message) Option(code uint16) []byte {
	if m == nil || m.Options == nil {
		return nil
	}
	values := m.Options[code]
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func (m *Message) AllOptions(code uint16) [][]byte {
	if m == nil || m.Options == nil {
		return nil
	}
	return m.Options[code]
}

// packetFromMessage maps a DHCPv6 message to the handler Packet structure.
func packetFromMessage(msg *Message) (Packet, error) {
	var pkt Packet
	if msg == nil {
		return pkt, errors.New("dhcpv6: nil message")
	}
	pkt.TransactionID = msg.TransactionID

	if cid := msg.Option(OptionClientID); cid != nil {
		duidStr, mac := parseDUID(cid)
		pkt.DUID = duidStr
		pkt.ClientMAC = mac
	}

	if ia := msg.Option(OptionIANA); ia != nil {
		iaid, addr := parseIANA(ia)
		pkt.IAID = iaid
		if addr != nil {
			pkt.IAAddr = addr
		}
	}

	if pdOpts := msg.AllOptions(OptionIAPD); len(pdOpts) > 0 {
		for _, raw := range pdOpts {
			if pd, ok := parseIAPD(raw); ok {
				pkt.PrefixRequests = append(pkt.PrefixRequests, pd)
			}
		}
	}

	if pkt.IAAddr == nil && len(msg.AllOptions(OptionIANA)) == 0 {
		pkt.SupportsSLAAC = true
	}
	if !pkt.SupportsSLAAC && len(msg.AllOptions(OptionIATA)) > 0 {
		pkt.SupportsSLAAC = true
	}
	if !pkt.SupportsSLAAC && msg.MessageType == MessageTypeInformationReq {
		pkt.SupportsSLAAC = true
	}

	if uc := msg.Option(OptionUserClass); uc != nil {
		pkt.UserClass = parseUserClass(uc)
	}
	if vc := msg.Option(OptionVendorClass); vc != nil {
		pkt.VendorClass = parseVendorClass(vc)
	}
	if dnsRaw := msg.Option(OptionDNSRecursiveNameServer); len(dnsRaw) > 0 {
		pkt.DNSRecursiveServers = parseIPv6AddressListOption(dnsRaw)
	}
	if dslRaw := msg.Option(OptionDomainSearchList); len(dslRaw) > 0 {
		pkt.DomainSearchList = parseDomainSearchListOption(dslRaw)
	}
	if fqdnRaw := msg.Option(OptionFQDN); len(fqdnRaw) > 0 {
		pkt.FQDN = parseFQDNOption(fqdnRaw)
	}
	if ntpRaw := msg.Option(OptionNTPServer); len(ntpRaw) > 0 {
		pkt.NTPServers = parseNTPServerOption(ntpRaw)
	}

	if len(msg.RelayHops) > 0 {
		hop := msg.RelayHops[0]
		pkt.LinkAddr = copyIP(hop.LinkAddr)
		pkt.PeerAddr = copyIP(hop.PeerAddr)

		relayAttrs := make(map[string]string)
		relayAttrs["relay-hop-count"] = strconv.Itoa(len(msg.RelayHops))
		for idx, relayHop := range msg.RelayHops {
			if link := strings.TrimSpace(net.IP(relayHop.LinkAddr).String()); link != "" {
				relayAttrs[fmt.Sprintf("relay-hop.%d.link-addr", idx)] = link
			}
			if peer := strings.TrimSpace(net.IP(relayHop.PeerAddr).String()); peer != "" {
				relayAttrs[fmt.Sprintf("relay-hop.%d.peer-addr", idx)] = peer
			}
			attrs := relayHopAttributes(relayHop)
			for key, value := range attrs {
				if strings.TrimSpace(value) == "" {
					continue
				}
				relayAttrs[fmt.Sprintf("relay-hop.%d.%s", idx, key)] = value
				if _, exists := relayAttrs[key]; !exists {
					relayAttrs[key] = value
				}
			}
		}
		if len(relayAttrs) > 0 {
			pkt.RelayInfo = relayAttrs
		}
	}

	return pkt, nil
}

func parseDUID(data []byte) (string, net.HardwareAddr) {
	duid := hex.EncodeToString(data)
	if len(data) < 4 {
		return duid, nil
	}
	duidType := binary.BigEndian.Uint16(data[0:2])
	switch duidType {
	case 1: // LLT
		if len(data) < 10 {
			return duid, nil
		}
		mac := net.HardwareAddr(make([]byte, len(data)-8))
		copy(mac, data[8:])
		return duid, mac
	case 3: // LL
		if len(data) < 8 {
			return duid, nil
		}
		mac := net.HardwareAddr(make([]byte, len(data)-4))
		copy(mac, data[4:])
		return duid, mac
	default:
		return duid, nil
	}
}

func parseIANA(data []byte) (uint32, net.IP) {
	if len(data) < 12 {
		return 0, nil
	}
	iaid := binary.BigEndian.Uint32(data[0:4])
	opts, err := parseOptions(data[12:])
	if err != nil {
		return iaid, nil
	}
	iaaddrs := opts[OptionIAAddr]
	if len(iaaddrs) == 0 {
		return iaid, nil
	}
	entry := iaaddrs[0]
	if len(entry) < 24 {
		return iaid, nil
	}
	ip := net.IP(entry[0:16])
	return iaid, append(net.IP(nil), ip...)
}

func parseIAPD(data []byte) (lease.PrefixRequest, bool) {
	var req lease.PrefixRequest
	if len(data) < 12 {
		return req, false
	}
	req.IAPDID = binary.BigEndian.Uint32(data[0:4])
	opts, err := parseOptions(data[12:])
	if err != nil {
		return req, false
	}
	prefixes := opts[OptionIAPrefix]
	if len(prefixes) == 0 {
		return req, true
	}
	entry := prefixes[0]
	if len(entry) < 25 {
		return req, true
	}
	req.PreferredLifetime = binary.BigEndian.Uint32(entry[0:4])
	req.ValidLifetime = binary.BigEndian.Uint32(entry[4:8])
	req.PrefixLength = entry[8]
	ip := net.IP(entry[9:25])
	req.Prefix = append(net.IP(nil), ip...)
	return req, true
}

func parseUserClass(data []byte) string {
	out := ""
	offset := 0
	for offset+2 <= len(data) {
		l := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+l > len(data) {
			break
		}
		if out != "" {
			out += ","
		}
		out += sanitizeASCII(data[offset : offset+l])
		offset += l
	}
	return out
}

func parseVendorClass(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	enterprise := binary.BigEndian.Uint32(data[0:4])
	offset := 4
	parts := []string{fmt.Sprintf("%d", enterprise)}
	for offset+2 <= len(data) {
		l := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+l > len(data) {
			break
		}
		parts = append(parts, sanitizeASCII(data[offset:offset+l]))
		offset += l
	}
	return strings.Join(parts, ":")
}

func relayHopAttributes(hop RelayHop) map[string]string {
	if hop.Options == nil {
		return nil
	}
	attrs := make(map[string]string)
	if vals := hop.Options[OptionInterfaceID]; len(vals) > 0 {
		if decoded := sanitizeASCII(vals[0]); decoded != "" {
			attrs["interface-id"] = decoded
			attrs["agent.circuit-id"] = decoded
		} else {
			attrs["interface-id"] = hex.EncodeToString(vals[0])
			attrs["agent.circuit-id"] = attrs["interface-id"]
		}
		attrs["interface-id-raw"] = hex.EncodeToString(vals[0])
	}
	if vals := hop.Options[OptionRemoteID]; len(vals) > 0 {
		remoteID, remoteEnterprise, remoteRaw := parseRelayRemoteID(vals[0])
		attrs["remote-id"] = remoteID
		attrs["agent.remote-id"] = remoteID
		attrs["remote-id-enterprise"] = remoteEnterprise
		attrs["remote-id-raw"] = remoteRaw
		if _, exists := attrs["location"]; !exists {
			attrs["location"] = remoteID
		}
	}
	for code, vals := range hop.Options {
		if code == OptionInterfaceID || code == OptionRemoteID || code == OptionRelayMsg {
			continue
		}
		if len(vals) == 0 {
			continue
		}
		key := fmt.Sprintf("opt_%d", code)
		if _, exists := attrs[key]; !exists {
			attrs[key] = hex.EncodeToString(vals[0])
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func parseRelayRemoteID(data []byte) (id string, enterprise string, raw string) {
	raw = hex.EncodeToString(data)
	if len(data) == 0 {
		return "", "", raw
	}
	if len(data) < 4 {
		decoded := sanitizeASCII(data)
		if decoded != "" {
			return decoded, "", raw
		}
		return raw, "", raw
	}
	enterprise = strconv.FormatUint(uint64(binary.BigEndian.Uint32(data[0:4])), 10)
	identifier := data[4:]
	decoded := sanitizeASCII(identifier)
	if decoded != "" {
		return decoded, enterprise, raw
	}
	if len(identifier) > 0 {
		return hex.EncodeToString(identifier), enterprise, raw
	}
	return raw, enterprise, raw
}

func sanitizeASCII(data []byte) string {
	runes := make([]byte, 0, len(data))
	for _, b := range data {
		if b >= 32 && b <= 126 {
			runes = append(runes, b)
		}
	}
	return string(runes)
}

func copyIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	return append(net.IP(nil), ip...)
}

func parseIPv6AddressListOption(data []byte) []net.IP {
	if len(data) == 0 {
		return nil
	}
	if len(data)%16 != 0 {
		zap.L().Debug("dhcpv6 option IPv6 address list has invalid length", zap.Int("length", len(data)))
	}
	servers := make([]net.IP, 0, len(data)/16)
	for offset := 0; offset+16 <= len(data); offset += 16 {
		addr := append(net.IP(nil), data[offset:offset+16]...)
		if addr.To16() == nil || addr.To4() != nil {
			continue
		}
		servers = append(servers, addr)
	}
	if len(servers) == 0 {
		return nil
	}
	return servers
}

func parseDomainSearchListOption(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	entries := make([]string, 0)
	offset := 0
	for offset < len(data) {
		domain, next, ok := decodeDomainName(data, offset)
		if !ok {
			zap.L().Debug("dhcpv6 domain search list decode failed", zap.Int("offset", offset), zap.Int("length", len(data)))
			break
		}
		if domain != "" {
			entries = append(entries, domain)
		}
		offset = next
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func parseFQDNOption(data []byte) string {
	if len(data) < 3 {
		return ""
	}
	domain, _, ok := decodeDomainName(data, 3)
	if ok {
		return domain
	}
	zap.L().Debug("dhcpv6 fqdn decode failed", zap.Int("length", len(data)))
	return ""
}

func parseNTPServerOption(data []byte) []net.IP {
	if len(data) < 4 {
		return nil
	}
	servers := make([]net.IP, 0)
	offset := 0
	for offset+4 <= len(data) {
		subCode := binary.BigEndian.Uint16(data[offset : offset+2])
		subLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		offset += 4
		if offset+subLen > len(data) {
			zap.L().Debug("dhcpv6 ntp server suboption truncated", zap.Int("offset", offset), zap.Int("sub_len", subLen), zap.Int("length", len(data)))
			break
		}
		subData := data[offset : offset+subLen]
		offset += subLen
		if subCode != 1 {
			continue
		}
		if len(subData)%16 != 0 {
			zap.L().Debug("dhcpv6 ntp server address suboption invalid length", zap.Int("length", len(subData)))
		}
		for i := 0; i+16 <= len(subData); i += 16 {
			ip := append(net.IP(nil), subData[i:i+16]...)
			if ip.To16() == nil || ip.To4() != nil {
				continue
			}
			servers = append(servers, ip)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	return servers
}

func decodeDomainName(data []byte, start int) (string, int, bool) {
	if start < 0 || start >= len(data) {
		return "", start, false
	}
	parts := make([]string, 0)
	offset := start
	for {
		if offset >= len(data) {
			return "", start, false
		}
		labelLen := int(data[offset])
		offset++
		if labelLen == 0 {
			break
		}
		if labelLen > 63 || offset+labelLen > len(data) {
			return "", start, false
		}
		label := strings.ToLower(sanitizeASCII(data[offset : offset+labelLen]))
		if label == "" {
			return "", start, false
		}
		parts = append(parts, label)
		offset += labelLen
	}
	return strings.Join(parts, "."), offset, true
}

func encodeIPv6AddressListOption(servers []net.IP) []byte {
	if len(servers) == 0 {
		return nil
	}
	payload := make([]byte, 0, len(servers)*16)
	for _, server := range servers {
		if server == nil {
			continue
		}
		ip := server.To16()
		if ip == nil || server.To4() != nil {
			continue
		}
		payload = append(payload, ip...)
	}
	if len(payload) == 0 {
		return nil
	}
	return payload
}

func encodeDomainSearchListOption(domains []string) ([]byte, error) {
	if len(domains) == 0 {
		return nil, nil
	}
	payload := make([]byte, 0)
	for _, domain := range domains {
		encoded, err := encodeDomainName(domain)
		if err != nil {
			return nil, err
		}
		if len(encoded) == 0 {
			continue
		}
		payload = append(payload, encoded...)
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return payload, nil
}

func encodeFQDNOption(fqdn string) ([]byte, error) {
	name, err := encodeDomainName(fqdn)
	if err != nil {
		return nil, err
	}
	if len(name) == 0 {
		return nil, nil
	}
	payload := make([]byte, 3, 3+len(name))
	payload = append(payload, name...)
	return payload, nil
}

func encodeNTPServerOption(servers []net.IP) []byte {
	if len(servers) == 0 {
		return nil
	}
	payload := make([]byte, 0, len(servers)*20)
	for _, server := range servers {
		if server == nil {
			continue
		}
		ip := server.To16()
		if ip == nil || server.To4() != nil {
			continue
		}
		sub := make([]byte, 20)
		binary.BigEndian.PutUint16(sub[0:2], 1)
		binary.BigEndian.PutUint16(sub[2:4], 16)
		copy(sub[4:20], ip)
		payload = append(payload, sub...)
	}
	if len(payload) == 0 {
		return nil
	}
	return payload
}

func encodeDomainName(domain string) ([]byte, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(domain, "."))
	if trimmed == "" {
		return nil, nil
	}
	labels := strings.Split(trimmed, ".")
	payload := make([]byte, 0, len(trimmed)+2)
	for _, label := range labels {
		part := strings.TrimSpace(label)
		if part == "" {
			return nil, fmt.Errorf("dhcpv6: invalid domain label in %q", domain)
		}
		if len(part) > 63 {
			return nil, fmt.Errorf("dhcpv6: domain label exceeds 63 bytes in %q", domain)
		}
		payload = append(payload, byte(len(part)))
		payload = append(payload, []byte(strings.ToLower(part))...)
	}
	payload = append(payload, 0)
	return payload, nil
}
