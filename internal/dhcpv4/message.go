package dhcpv4

import (
	"net"

	"modern-dhcp/internal/dhcpv4/consts"
	"modern-dhcp/internal/dhcpv4/parser"
)

const (
	dhcpMagicCookie = consts.DHCPMagicCookie
	opBootRequest   = consts.OpBootRequest
	opBootReply     = consts.OpBootReply
)

const (
	MessageTypeDiscover = consts.MessageTypeDiscover
	MessageTypeOffer    = consts.MessageTypeOffer
	MessageTypeRequest  = consts.MessageTypeRequest
	MessageTypeDecline  = consts.MessageTypeDecline
	MessageTypeAck      = consts.MessageTypeAck
	MessageTypeNak      = consts.MessageTypeNak
	MessageTypeRelease  = consts.MessageTypeRelease
	MessageTypeInform   = consts.MessageTypeInform
)

const (
	OptionSubnetMask           = consts.OptionSubnetMask
	OptionRouter               = consts.OptionRouter
	OptionDNSServer            = consts.OptionDNSServer
	OptionRequestedIPAddress   = consts.OptionRequestedIPAddress
	OptionIPAddressLeaseTime   = consts.OptionIPAddressLeaseTime
	OptionDHCPMessageType      = consts.OptionDHCPMessageType
	OptionServerIdentifier     = consts.OptionServerIdentifier
	OptionParameterRequestList = consts.OptionParameterRequestList
	OptionMessage              = consts.OptionMessage
	OptionMaximumDHCPSize      = consts.OptionMaximumDHCPSize
	OptionRenewalTime          = consts.OptionRenewalTime
	OptionRebindingTime        = consts.OptionRebindingTime
	OptionVendorClass          = consts.OptionVendorClass
	OptionClientIdentifier     = consts.OptionClientIdentifier
	OptionUserClass            = consts.OptionUserClass
	OptionRelayAgentInfo       = consts.OptionRelayAgentInfo
	OptionVendorVIVendorInfo   = consts.OptionVendorVIVendorInfo
)

type Message = parser.Message

func ParseMessage(data []byte) (*Message, error) { return parser.ParseMessage(data) }

func padIP(ip net.IP) []byte { return parser.PadIP(ip) }

func formatClientID(data []byte) string { return parser.FormatClientID(data) }
