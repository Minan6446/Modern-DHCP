package netutil

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const icmpProtocolNumber = 1

// ICMPProber sends ICMP echo requests to determine if an IPv4 address is active.
type ICMPProber struct {
	timeout time.Duration
	payload []byte
	seq     uint32
}

// NewICMPProber builds a prober with the provided timeout.
func NewICMPProber(timeout time.Duration) *ICMPProber {
	if timeout <= 0 {
		timeout = time.Second
	}
	return &ICMPProber{timeout: timeout, payload: []byte("modern-dhcp-conflict-probe")}
}

// Probe returns true when the target responds to an ICMP echo request before the deadline.
func (p *ICMPProber) Probe(ctx context.Context, addr netip.Addr) (bool, time.Duration, error) {
	if !addr.IsValid() {
		return false, 0, errors.New("icmp: invalid address")
	}
	ip := addr.Unmap()
	if !ip.Is4() {
		return false, 0, errors.New("icmp: only ipv4 addresses supported")
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return false, 0, err
	}
	defer conn.Close()

	deadline := time.Now().Add(p.timeout)
	if ctx != nil {
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return false, 0, err
	}

	seq := atomic.AddUint32(&p.seq, 1)
	echo := &icmp.Echo{
		ID:   int(seq & 0xffff),
		Seq:  int((seq >> 16) & 0xffff),
		Data: p.payload,
	}
	msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: echo}
	packet, err := msg.Marshal(nil)
	if err != nil {
		return false, 0, err
	}

	target := &net.IPAddr{IP: ip.AsSlice()}
	started := time.Now()
	if _, err := conn.WriteTo(packet, target); err != nil {
		return false, 0, err
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return false, time.Since(started), nil
			}
			return false, 0, err
		}
		peerIP, _ := peer.(*net.IPAddr)
		if peerIP == nil || !peerIP.IP.Equal(target.IP) {
			continue
		}
		response, err := icmp.ParseMessage(icmpProtocolNumber, buf[:n])
		if err != nil {
			continue
		}
		if response.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		reply, ok := response.Body.(*icmp.Echo)
		if !ok {
			continue
		}
		if reply.ID == echo.ID && reply.Seq == echo.Seq {
			return true, time.Since(started), nil
		}
	}
}
