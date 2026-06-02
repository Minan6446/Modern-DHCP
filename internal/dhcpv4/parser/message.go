package parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"

	"modern-dhcp/internal/dhcpv4/consts"
	dhcpv4errors "modern-dhcp/internal/dhcpv4/errors"
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

func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 240 {
		return nil, dhcpv4errors.Wrap(dhcpv4errors.CodeMalformedPacket, "dhcpv4 packet too short", errors.New("packet too short"))
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
		return nil, dhcpv4errors.Wrap(dhcpv4errors.CodeMalformedPacket, "invalid chaddr length", errors.New("invalid chaddr length"))
	}
	m.CHAddr = append(net.HardwareAddr{}, data[28:chEnd]...)
	copy(m.SName[:], data[44:108])
	copy(m.File[:], data[108:236])

	if binary.BigEndian.Uint32(data[236:240]) != consts.DHCPMagicCookie {
		return nil, dhcpv4errors.Wrap(dhcpv4errors.CodeMalformedPacket, "bad magic cookie", errors.New("bad magic cookie"))
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
			return nil, dhcpv4errors.Wrap(dhcpv4errors.CodeMalformedPacket, "malformed options", errors.New("malformed options"))
		}
		length := int(data[idx])
		idx++
		if idx+length > len(data) {
			return nil, dhcpv4errors.Wrap(dhcpv4errors.CodeMalformedPacket, "truncated option", errors.New("truncated option"))
		}
		m.Options[code] = append([]byte{}, data[idx:idx+length]...)
		idx += length
	}

	return m, nil
}

func (m *Message) SetOption(code byte, value []byte) {
	if m.Options == nil {
		m.Options = make(map[byte][]byte)
	}
	m.Options[code] = append([]byte{}, value...)
}

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
	copy(buf[12:16], PadIP(m.CIAddr))
	copy(buf[16:20], PadIP(m.YIAddr))
	copy(buf[20:24], PadIP(m.SIAddr))
	copy(buf[24:28], PadIP(m.GIAddr))
	copy(buf[28:28+len(m.CHAddr)], m.CHAddr)
	copy(buf[44:108], m.SName[:])
	copy(buf[108:236], m.File[:])

	binary.BigEndian.PutUint32(buf[236:240], consts.DHCPMagicCookie)

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

func PadIP(ip net.IP) []byte {
	if ip == nil {
		return []byte{0, 0, 0, 0}
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return []byte{0, 0, 0, 0}
}

func FormatClientID(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", data)
}
