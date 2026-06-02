package dhcpv6

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	ping "github.com/go-ping/ping"
)

// DeriveIPv6FromEUI64 derives an IPv6 address from a /64 prefix and MAC using EUI-64 embedding.
func DeriveIPv6FromEUI64(prefix netip.Prefix, mac net.HardwareAddr) (netip.Addr, error) {
	if !prefix.IsValid() || !prefix.Addr().Is6() {
		return netip.Addr{}, fmt.Errorf("invalid ipv6 prefix: %q", prefix.String())
	}
	if len(mac) != 6 {
		return netip.Addr{}, errors.New("eui64 requires 48-bit mac")
	}
	base := prefix.Masked().Addr().As16()
	iface := [8]byte{mac[0] ^ 0x02, mac[1], mac[2], 0xff, 0xfe, mac[3], mac[4], mac[5]}
	for i := 0; i < len(iface); i++ {
		base[8+i] = iface[i]
	}
	return netip.AddrFrom16(base), nil
}

func probeIPv6Conflict(ctx context.Context, targetIP string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	addr := net.ParseIP(targetIP)
	if addr == nil || addr.To16() == nil || addr.To4() != nil {
		return false, fmt.Errorf("invalid ipv6 address: %q", targetIP)
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pinger, err := ping.NewPinger(targetIP)
	if err != nil {
		return false, err
	}
	pinger.SetPrivileged(true)
	pinger.SetNetwork("ip6")
	pinger.Count = 1
	pinger.Interval = 200 * time.Millisecond
	pinger.Timeout = timeout

	done := make(chan error, 1)
	go func() {
		done <- pinger.Run()
	}()

	select {
	case <-probeCtx.Done():
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return false, nil
		}
		return false, probeCtx.Err()
	case runErr := <-done:
		if runErr != nil {
			return false, runErr
		}
		stats := pinger.Statistics()
		if stats == nil {
			return false, nil
		}
		return stats.PacketsRecv > 0, nil
	}
}
