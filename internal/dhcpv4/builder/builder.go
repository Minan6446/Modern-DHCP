package builder

import (
	"errors"
	"net"
	"time"

	"modern-dhcp/internal/dhcpv4/consts"
	"modern-dhcp/internal/dhcpv4/option"
	"modern-dhcp/internal/dhcpv4/parser"
	"modern-dhcp/internal/lease"
)

type ResponseBuilder interface {
	BuildResponse(req *parser.Message, result *lease.Result, msgType byte) (*parser.Message, error)
	BuildNAK(req *parser.Message, reason string) *parser.Message
	PacketTypeName(messageType byte) string
}

type DefaultBuilder struct {
	serverIP     net.IP
	vendorEncode func(*lease.Result) []byte
}

func NewResponseBuilder(serverIP net.IP, vendorEncode func(*lease.Result) []byte) ResponseBuilder {
	return &DefaultBuilder{serverIP: serverIP, vendorEncode: vendorEncode}
}

func (b *DefaultBuilder) BuildResponse(req *parser.Message, result *lease.Result, msgType byte) (*parser.Message, error) {
	if result == nil || result.Lease == nil {
		return nil, errors.New("dhcpv4: missing lease result")
	}
	leaseIP := net.ParseIP(result.Lease.IPAddress)
	if leaseIP == nil {
		return nil, errors.New("dhcpv4: invalid lease ip")
	}
	resp := NewReplyFromRequest(req)
	resp.YIAddr = leaseIP.To4()
	resp.SIAddr = parser.PadIP(b.serverIP)
	resp.SetOption(consts.OptionDHCPMessageType, []byte{msgType})
	resp.SetOption(consts.OptionServerIdentifier, parser.PadIP(b.serverIP))
	leaseTime := result.Profile.DefaultDuration
	if leaseTime <= 0 {
		leaseTime = time.Duration(consts.DefaultLeaseSeconds) * time.Second
	}
	resp.SetOption(consts.OptionIPAddressLeaseTime, option.EncodeUint32(uint32(leaseTime.Seconds())))
	if result.RenewalTime > 0 {
		resp.SetOption(consts.OptionRenewalTime, option.EncodeUint32(uint32(result.RenewalTime.Seconds())))
	}
	if result.RebindingTime > 0 {
		resp.SetOption(consts.OptionRebindingTime, option.EncodeUint32(uint32(result.RebindingTime.Seconds())))
	}
	if b.vendorEncode != nil {
		if payload := b.vendorEncode(result); len(payload) > 0 {
			resp.SetOption(consts.OptionVendorVIVendorInfo, payload)
		}
	}
	return resp, nil
}

func (b *DefaultBuilder) BuildNAK(req *parser.Message, reason string) *parser.Message {
	resp := NewReplyFromRequest(req)
	resp.YIAddr = net.IPv4zero
	resp.SetOption(consts.OptionDHCPMessageType, []byte{consts.MessageTypeNak})
	resp.SIAddr = parser.PadIP(b.serverIP)
	resp.SetOption(consts.OptionServerIdentifier, parser.PadIP(b.serverIP))
	if reason != "" {
		if len(reason) > 255 {
			reason = reason[:255]
		}
		resp.SetOption(consts.OptionMessage, []byte(reason))
	}
	return resp
}

func NewReplyFromRequest(req *parser.Message) *parser.Message {
	return &parser.Message{
		Op:      consts.OpBootReply,
		HType:   req.HType,
		HLen:    req.HLen,
		XID:     req.XID,
		Secs:    req.Secs,
		Flags:   req.Flags,
		SIAddr:  make([]byte, len(req.SIAddr)),
		GIAddr:  append(net.IP{}, req.GIAddr...),
		CHAddr:  append(net.HardwareAddr{}, req.CHAddr...),
		Options: make(map[byte][]byte),
	}
}

func (b *DefaultBuilder) PacketTypeName(messageType byte) string {
	switch messageType {
	case consts.MessageTypeDiscover:
		return "DISCOVER"
	case consts.MessageTypeOffer:
		return "OFFER"
	case consts.MessageTypeRequest:
		return "REQUEST"
	case consts.MessageTypeAck:
		return "ACK"
	case consts.MessageTypeNak:
		return "NAK"
	default:
		return ""
	}
}
