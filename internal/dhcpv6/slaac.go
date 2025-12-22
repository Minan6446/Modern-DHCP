package dhcpv6

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"net/netip"
	"strings"

	"modern-dhcp/internal/netutil"
)

// buildSLAACSeed concatenates deterministic identifiers used for SLAAC host derivation.
func buildSLAACSeed(pkt Packet) string {
	var parts []string
	if pkt.DUID != "" {
		parts = append(parts, pkt.DUID)
	}
	if pkt.ClientMAC != nil {
		parts = append(parts, pkt.ClientMAC.String())
	}
	if pkt.LinkAddr != nil {
		parts = append(parts, pkt.LinkAddr.String())
	}
	if pkt.PeerAddr != nil {
		parts = append(parts, pkt.PeerAddr.String())
	}
	if pkt.IAID != 0 {
		parts = append(parts, fmt.Sprintf("%d", pkt.IAID))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "|")
}

func deriveSLAACAddress(prefix netip.Prefix, seed string) (string, bool) {
	if seed == "" {
		return "", false
	}
	hostBits := 128 - prefix.Bits()
	if hostBits <= 0 {
		return "", false
	}
	sum := sha256.Sum256([]byte(seed))
	hostInt := new(big.Int).SetBytes(sum[:])
	maxHosts := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))
	hostInt.Mod(hostInt, maxHosts)
	network := netutil.AddrToBig(prefix.Masked().Addr())
	candidate := new(big.Int).Add(network, hostInt)
	addr, ok := netutil.BigToAddr(candidate)
	if !ok {
		return "", false
	}
	return addr.String(), true
}
