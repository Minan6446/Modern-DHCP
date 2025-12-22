package dhcpv6

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"

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

	if len(msg.RelayHops) > 0 {
		hop := msg.RelayHops[0]
		pkt.LinkAddr = copyIP(hop.LinkAddr)
		pkt.PeerAddr = copyIP(hop.PeerAddr)
		if attrs := relayHopAttributes(hop); len(attrs) > 0 {
			pkt.RelayInfo = attrs
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
		}
	}
	if vals := hop.Options[OptionRemoteID]; len(vals) > 0 {
		if decoded := sanitizeASCII(vals[0]); decoded != "" {
			attrs["remote-id"] = decoded
			attrs["agent.remote-id"] = decoded
			if _, exists := attrs["location"]; !exists {
				attrs["location"] = decoded
			}
		} else {
			attrs["remote-id"] = hex.EncodeToString(vals[0])
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
