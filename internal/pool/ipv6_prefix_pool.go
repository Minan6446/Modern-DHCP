package pool

import (
	"fmt"
	"math/big"
	"net/netip"

	"modern-dhcp/internal/netutil"
)

// IPv6PrefixPool provides deterministic delegated-prefix allocation with conflict detection.
type IPv6PrefixPool struct {
	PoolPrefix            netip.Prefix
	DelegatedPrefixLength int
	usedPrefixes          map[string]struct{}
}

// NewIPv6PrefixPool builds an IPv6PrefixPool allocator for PD workflows.
func NewIPv6PrefixPool(poolPrefix netip.Prefix, delegatedPrefixLength int, used map[string]struct{}) (*IPv6PrefixPool, error) {
	if !poolPrefix.IsValid() || !poolPrefix.Addr().Is6() {
		return nil, fmt.Errorf("pool: invalid IPv6 pool prefix %q", poolPrefix.String())
	}
	if delegatedPrefixLength <= 0 {
		delegatedPrefixLength = poolPrefix.Bits()
	}
	if delegatedPrefixLength < poolPrefix.Bits() {
		delegatedPrefixLength = poolPrefix.Bits()
	}
	if delegatedPrefixLength > 128 {
		delegatedPrefixLength = 128
	}
	usedSet := make(map[string]struct{}, len(used))
	for k := range used {
		usedSet[k] = struct{}{}
	}
	return &IPv6PrefixPool{PoolPrefix: poolPrefix.Masked(), DelegatedPrefixLength: delegatedPrefixLength, usedPrefixes: usedSet}, nil
}

// IsPrefixInUse reports whether a prefix has already been allocated.
func (p *IPv6PrefixPool) IsPrefixInUse(prefix netip.Prefix) bool {
	if p == nil {
		return false
	}
	_, exists := p.usedPrefixes[prefix.Masked().String()]
	return exists
}

// MarkUsed records a delegated prefix in the in-use set.
func (p *IPv6PrefixPool) MarkUsed(prefix netip.Prefix) {
	if p == nil {
		return
	}
	p.usedPrefixes[prefix.Masked().String()] = struct{}{}
}

// AllocateAvailablePrefix chooses a candidate prefix if free, otherwise scans for the next available one.
func (p *IPv6PrefixPool) AllocateAvailablePrefix(candidate netip.Prefix) (netip.Prefix, bool) {
	if p == nil {
		return netip.Prefix{}, false
	}
	if candidate.IsValid() {
		if allocated, ok := p.AllocateSpecificPrefix(candidate); ok {
			return allocated, true
		}
	}
	diff := p.DelegatedPrefixLength - p.PoolPrefix.Bits()
	if diff < 0 {
		diff = 0
	}
	maxCandidates := 1 << 12
	baseInt := netutil.AddrToBig(p.PoolPrefix.Masked().Addr())
	step := new(big.Int).Lsh(big.NewInt(1), uint(128-p.DelegatedPrefixLength))
	total := new(big.Int).Lsh(big.NewInt(1), uint(diff))
	for i := 0; i < maxCandidates; i++ {
		idx := big.NewInt(int64(i))
		if idx.Cmp(total) >= 0 {
			break
		}
		offset := new(big.Int).Mul(step, idx)
		candidateInt := new(big.Int).Add(baseInt, offset)
		addr, ok := netutil.BigToAddr(candidateInt)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(addr, p.DelegatedPrefixLength).Masked()
		if p.IsPrefixInUse(prefix) {
			continue
		}
		p.MarkUsed(prefix)
		return prefix, true
	}
	return netip.Prefix{}, false
}

// AllocateSpecificPrefix allocates only the requested prefix and rejects on conflict.
func (p *IPv6PrefixPool) AllocateSpecificPrefix(candidate netip.Prefix) (netip.Prefix, bool) {
	if p == nil || !candidate.IsValid() {
		return netip.Prefix{}, false
	}
	maskedCandidate := netip.PrefixFrom(candidate.Masked().Addr(), p.DelegatedPrefixLength).Masked()
	if !p.PoolPrefix.Contains(maskedCandidate.Masked().Addr()) {
		return netip.Prefix{}, false
	}
	if p.IsPrefixInUse(maskedCandidate) {
		return netip.Prefix{}, false
	}
	p.MarkUsed(maskedCandidate)
	return maskedCandidate, true
}
