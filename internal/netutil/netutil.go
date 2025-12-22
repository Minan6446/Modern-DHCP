package netutil

import (
	"crypto/rand"
	"math/big"
	"net/netip"
)

// AddrToBig converts a netip.Addr into a big integer for arithmetic operations.
func AddrToBig(addr netip.Addr) *big.Int {
	if !addr.IsValid() {
		return big.NewInt(0)
	}
	if addr.Is4() {
		ipv4 := addr.As4()
		buf := make([]byte, 16)
		copy(buf[12:], ipv4[:])
		return new(big.Int).SetBytes(buf)
	}
	ipv6 := addr.As16()
	return new(big.Int).SetBytes(ipv6[:])
}

// BigToAddr converts a big integer back into a 128-bit IPv6 address.
func BigToAddr(v *big.Int) (netip.Addr, bool) {
	if v == nil || v.Sign() < 0 {
		return netip.Addr{}, false
	}
	buf := v.Bytes()
	if len(buf) > 16 {
		return netip.Addr{}, false
	}
	tmp := make([]byte, 16)
	copy(tmp[16-len(buf):], buf)
	addr, ok := netip.AddrFromSlice(tmp)
	return addr, ok
}

// RandomBigInt returns a uniformly distributed random integer in [0, 2^bitSize).
func RandomBigInt(bitSize int) (*big.Int, error) {
	if bitSize <= 0 {
		return big.NewInt(0), nil
	}
	max := new(big.Int).Lsh(big.NewInt(1), uint(bitSize))
	return rand.Int(rand.Reader, max)
}
