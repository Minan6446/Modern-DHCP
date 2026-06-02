package consts

import "time"

const DHCPMagicCookie uint32 = 0x63825363

const (
	OpBootRequest byte = 1
	OpBootReply   byte = 2
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
	OptionVendorSpecificInfo   byte = 43
	OptionClasslessStaticRoute byte = 121
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

const FlagBroadcast uint16 = 1 << 15

const (
	DefaultServerPort   = 67
	DefaultReadTimeout  = 5 * time.Second
	DefaultLeaseSeconds = 3600
)
