package lease

import "net"

// PrefixRequest captures IA_PD details from DHCPv6 clients.
type PrefixRequest struct {
	IAPDID            uint32
	Prefix            net.IP
	PrefixLength      byte
	PreferredLifetime uint32
	ValidLifetime     uint32
}
