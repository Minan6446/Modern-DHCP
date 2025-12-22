package dhcpv4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
)

const (
	dhcpMagicCookie uint32 = 0x63825363

	opBootRequest byte = 1
	opBootReply   byte = 2
)

const (
	MessageTypeDiscover byte = 1
	MessageTypeOffer    byte = 2
	MessageTypeRequest  byte = 3
	MessageTypeDecline  byte = 4
	MessageTypeAck      byte = 5
	MessageTypeNak      byte = 6
	MessageTypeRelease  byte = 7
	MessageTypeInform   byte = 8
)

const (
	OptionSubnetMask           byte = 1
	OptionRouter               byte = 3
	OptionDNSServer            byte = 6
	OptionRequestedIPAddress   byte = 50
	OptionIPAddressLeaseTime   byte = 51
	OptionDHCPMessageType      byte = 53
	OptionServerIdentifier     byte = 54
	OptionParameterRequestList byte = 55
	OptionMessage              byte = 56
	OptionMaximumDHCPSize      byte = 57
	OptionRenewalTime          byte = 58
	OptionRebindingTime        byte = 59
	OptionVendorClass          byte = 60
	OptionClientIdentifier     byte = 61
	OptionUserClass            byte = 77
	OptionRelayAgentInfo       byte = 82
	OptionVendorVIVendorInfo   byte = 125
)

// Message represents a DHCPv4 datagram.
type Message struct {
	Op     byte
	HType  byte
	HLen   byte
	Hops   byte
	XID    uint32
	Secs   uint16
	Flags  uint16
	CIAddr net.IP
	YIAddr net.IP
	SIAddr net.IP
	GIAddr net.IP
	CHAddr net.HardwareAddr
	SName  [64]byte
	File   [128]byte

	Options map[byte][]byte
}

// ParseMessage decodes raw bytes into a Message structure.
func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 240 {
		return nil, errors.New("dhcpv4: packet too short")
	}

	m := &Message{Options: make(map[byte][]byte)}
	m.Op = data[0]
	m.HType = data[1]
	m.HLen = data[2]
	m.Hops = data[3]
	m.XID = binary.BigEndian.Uint32(data[4:8])
	m.Secs = binary.BigEndian.Uint16(data[8:10])
	m.Flags = binary.BigEndian.Uint16(data[10:12])
	m.CIAddr = append(net.IP{}, data[12:16]...)
	m.YIAddr = append(net.IP{}, data[16:20]...)
	m.SIAddr = append(net.IP{}, data[20:24]...)
	m.GIAddr = append(net.IP{}, data[24:28]...)

	chEnd := 28 + int(m.HLen)
	if chEnd > len(data) {
		return nil, errors.New("dhcpv4: invalid chaddr length")
	}
	m.CHAddr = append(net.HardwareAddr{}, data[28:chEnd]...)
	copy(m.SName[:], data[44:108])
	copy(m.File[:], data[108:236])

	if binary.BigEndian.Uint32(data[236:240]) != dhcpMagicCookie {
		return nil, errors.New("dhcpv4: bad magic cookie")
	}

	idx := 240
	for idx < len(data) {
		code := data[idx]
		idx++
		switch code {
		case 0:
			continue
		case 255:
			return m, nil
		}
		if idx >= len(data) {
			return nil, errors.New("dhcpv4: malformed options")
		}
		length := int(data[idx])
		idx++
		if idx+length > len(data) {
			return nil, errors.New("dhcpv4: truncated option")
		}
		m.Options[code] = append([]byte{}, data[idx:idx+length]...)
		idx += length
	}

	return m, nil
}

// SetOption copies the option payload into the message map.
func (m *Message) SetOption(code byte, value []byte) {
	if m.Options == nil {
		m.Options = make(map[byte][]byte)
	}
	m.Options[code] = append([]byte{}, value...)
}

// Option returns the option payload if present.
func (m *Message) Option(code byte) []byte {
	if m == nil || m.Options == nil {
		return nil
	}
	val, ok := m.Options[code]
	if !ok {
		return nil
	}
	return val
}

// MarshalBinary encodes the message into wire format.
func (m *Message) MarshalBinary() ([]byte, error) {
	if len(m.CHAddr) > 16 {
		return nil, errors.New("dhcpv4: chaddr too long")
	}
	if m.HLen == 0 {
		m.HLen = byte(len(m.CHAddr))
	}

	buf := make([]byte, 240)
	buf[0] = m.Op
	buf[1] = m.HType
	buf[2] = m.HLen
	buf[3] = m.Hops
	binary.BigEndian.PutUint32(buf[4:8], m.XID)
	binary.BigEndian.PutUint16(buf[8:10], m.Secs)
	binary.BigEndian.PutUint16(buf[10:12], m.Flags)
	copy(buf[12:16], padIP(m.CIAddr))
	copy(buf[16:20], padIP(m.YIAddr))
	copy(buf[20:24], padIP(m.SIAddr))
	copy(buf[24:28], padIP(m.GIAddr))
	copy(buf[28:28+len(m.CHAddr)], m.CHAddr)
	copy(buf[44:108], m.SName[:])
	copy(buf[108:236], m.File[:])

	binary.BigEndian.PutUint32(buf[236:240], dhcpMagicCookie)

	return append(buf, buildOptions(m.Options)...), nil
}

func buildOptions(opts map[byte][]byte) []byte {
	if len(opts) == 0 {
		return []byte{255}
	}

	codes := make([]int, 0, len(opts))
	for c := range opts {
		codes = append(codes, int(c))
	}
	sort.Ints(codes)

	payload := make([]byte, 0, len(opts)*4)
	for _, c := range codes {
		code := byte(c)
		val := opts[code]
		if len(val) > 255 {
			val = val[:255]
		}
		payload = append(payload, code, byte(len(val)))
		payload = append(payload, val...)
	}
	payload = append(payload, 255)
	return payload
}

func padIP(ip net.IP) []byte {
	if ip == nil {
		return []byte{0, 0, 0, 0}
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return []byte{0, 0, 0, 0}
}

func formatClientID(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", data)
}
