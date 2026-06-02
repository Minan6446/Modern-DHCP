package netutil

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ACDProber defines ARP + ICMP probe primitives used by DHCPv4 address conflict detection.
type ACDProber interface {
	ProbeARP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error)
	ProbeICMP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error)
}

// DHCPv4ACDProber provides ARP-then-ICMP probing support for IPv4 private ranges.
type DHCPv4ACDProber struct {
	arpTimeout time.Duration
	icmp       *ICMPProber
}

// NewDHCPv4ACDProber creates a prober with independent ARP/ICMP probe timeouts.
func NewDHCPv4ACDProber(arpTimeout, icmpTimeout time.Duration) *DHCPv4ACDProber {
	if arpTimeout <= 0 {
		arpTimeout = time.Second
	}
	return &DHCPv4ACDProber{arpTimeout: arpTimeout, icmp: NewICMPProber(icmpTimeout)}
}

// ProbeARP performs a best-effort ARP reachability probe by triggering ARP resolution
// and checking OS neighbor cache for the requested IPv4 address.
func (p *DHCPv4ACDProber) ProbeARP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error) {
	if !addr.IsValid() {
		return false, 0, errors.New("acd arp: invalid address")
	}
	address := addr.Unmap()
	if !address.Is4() {
		return false, 0, errors.New("acd arp: only ipv4 addresses supported")
	}
	if !address.IsPrivate() {
		return false, 0, nil
	}
	start := time.Now()
	deadline := start.Add(p.arpTimeout)
	if ctx != nil {
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
	}
	if err := p.primeNeighborCache(address, deadline); err != nil {
		return false, time.Since(start), err
	}
	alive, err := p.lookupNeighbor(address, deadline)
	return alive, time.Since(start), err
}

// ProbeICMP runs ICMP Echo probing.
func (p *DHCPv4ACDProber) ProbeICMP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error) {
	if p == nil || p.icmp == nil {
		return false, 0, errors.New("acd icmp: prober unavailable")
	}
	return p.icmp.Probe(ctx, addr)
}

func (p *DHCPv4ACDProber) primeNeighborCache(addr netip.Addr, deadline time.Time) error {
	d := net.Dialer{Deadline: deadline, Timeout: p.arpTimeout}
	conn, err := d.Dial("udp4", net.JoinHostPort(addr.String(), "9"))
	if err != nil {
		return nil
	}
	defer conn.Close()
	_, _ = conn.Write([]byte{0x1})
	return nil
}

func (p *DHCPv4ACDProber) lookupNeighbor(addr netip.Addr, deadline time.Time) (bool, error) {
	command, args := arpLookupCommand(addr.String())
	cmd := exec.Command(command, args...)
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, context.DeadlineExceeded
		}
		ctx, cancel := context.WithTimeout(context.Background(), remaining)
		defer cancel()
		cmd = exec.CommandContext(ctx, command, args...)
	}
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	target := addr.String()
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if !strings.Contains(line, strings.ToLower(target)) {
			continue
		}
		if strings.Contains(line, "incomplete") {
			continue
		}
		return true, nil
	}
	return false, nil
}

func arpLookupCommand(ip string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "arp", []string{"-a", ip}
	}
	return "arp", []string{"-n", ip}
}
