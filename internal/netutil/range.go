package netutil

import (
	"errors"
	"math/big"
	"net/netip"
)

// MaxAddress returns the highest address contained in the prefix.
func MaxAddress(prefix netip.Prefix) (netip.Addr, error) {
	base := AddrToBig(prefix.Masked().Addr())
	bits := 128
	if prefix.Addr().Is4() {
		bits = 32
	}
	hostBits := bits - prefix.Bits()
	if hostBits < 0 {
		hostBits = 0
	}
	span := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))
	span.Sub(span, big.NewInt(1))
	maxInt := new(big.Int).Add(base, span)
	addr, ok := BigToAddr(maxInt)
	if !ok {
		return netip.Addr{}, ErrInvalidAddress
	}
	if prefix.Addr().Is4() {
		if v4, ok := ipv4FromCompat(addr); ok {
			addr = v4
		} else {
			addr = addr.Unmap()
		}
	}
	return addr, nil
}

// DefaultHostRange returns the first and last allocatable addresses for the prefix.
func DefaultHostRange(prefix netip.Prefix) (netip.Addr, netip.Addr, error) {
	start := prefix.Masked().Addr()
	end, err := MaxAddress(prefix)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	if prefix.Addr().Is4() {
		hostBits := 32 - prefix.Bits()
		if hostBits > 1 {
			if inc, ok := AddToAddr(start, 1); ok {
				start = inc
			}
			if dec, ok := AddToAddr(end, -1); ok {
				end = dec
			}
		}
	}
	return start, end, nil
}

// AddToAddr returns a new address offset by delta (positive or negative).
func AddToAddr(addr netip.Addr, delta int64) (netip.Addr, bool) {
	base := AddrToBig(addr)
	result := new(big.Int).Add(base, big.NewInt(delta))
	if result.Sign() < 0 {
		return netip.Addr{}, false
	}
	out, ok := BigToAddr(result)
	if !ok {
		return netip.Addr{}, false
	}
	if addr.Is4() {
		if v4, ok := ipv4FromCompat(out); ok {
			out = v4
		} else {
			out = out.Unmap()
		}
	}
	return out, true
}

// ErrInvalidAddress indicates an address could not be derived.
var ErrInvalidAddress = errors.New("netutil: invalid address")

func ipv4FromCompat(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() {
		return netip.Addr{}, false
	}
	b := addr.As16()
	for i := 0; i < 12; i++ {
		if b[i] != 0 {
			return netip.Addr{}, false
		}
	}
	return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
}
