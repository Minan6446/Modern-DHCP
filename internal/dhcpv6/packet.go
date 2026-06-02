package dhcpv6

import (
	"net"

	"modern-dhcp/internal/lease"
)

// Packet carries minimal DHCPv6 data for evaluation.
type Packet struct {
	TransactionID       uint32
	DUID                string
	IAID                uint32
	IAAddr              net.IP
	DNSRecursiveServers []net.IP
	DomainSearchList    []string
	FQDN                string
	NTPServers          []net.IP
	LinkAddr            net.IP
	PeerAddr            net.IP
	VendorClass         string
	UserClass           string
	RelayInfo           map[string]string
	ClientMAC           net.HardwareAddr
	SupportsSLAAC       bool
	PrefixRequests      []lease.PrefixRequest
}
