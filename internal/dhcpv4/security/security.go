package security

import (
	"fmt"
	"net"
	"strings"
	"sync"

	dhcpv4errors "modern-dhcp/internal/dhcpv4/errors"

	"github.com/juju/ratelimit"
)

type MACLimiter interface {
	Allow(mac string) bool
}

type macRateLimiter struct {
	pps     int64
	buckets map[string]*ratelimit.Bucket
	mu      sync.Mutex
}

func NewMACRateLimiter(pps int) MACLimiter {
	if pps <= 0 {
		pps = 10
	}
	return &macRateLimiter{pps: int64(pps), buckets: make(map[string]*ratelimit.Bucket, 256)}
}

func (l *macRateLimiter) Allow(mac string) bool {
	if l == nil {
		return true
	}
	key := strings.ToLower(strings.TrimSpace(mac))
	if key == "" {
		return true
	}
	l.mu.Lock()
	bucket, ok := l.buckets[key]
	if !ok {
		bucket = ratelimit.NewBucketWithRate(float64(l.pps), l.pps)
		l.buckets[key] = bucket
	}
	l.mu.Unlock()
	return bucket.TakeAvailable(1) == 1
}

type RelayRequest struct {
	Option82Present bool
	GIAddr          net.IP
	XID             uint32
}

type RelayValidator interface {
	Validate(req RelayRequest, remote *net.UDPAddr) error
}

type WhitelistRelayValidator struct {
	whitelist map[string]struct{}
}

func NewRelayValidator(ips []string) RelayValidator {
	return &WhitelistRelayValidator{whitelist: NormalizeIPSet(ips)}
}

func NormalizeIPSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		ip := net.ParseIP(strings.TrimSpace(v))
		if ip == nil {
			continue
		}
		set[ip.String()] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func (v *WhitelistRelayValidator) Validate(req RelayRequest, remote *net.UDPAddr) error {
	if v == nil || len(v.whitelist) == 0 {
		return nil
	}
	isRelay := req.Option82Present || firstIPv4(req.GIAddr) != nil
	if !isRelay {
		return nil
	}
	if remote == nil || remote.IP == nil {
		return dhcpv4errors.Wrap(dhcpv4errors.CodeRelayRejected, "relay source validation failed", fmt.Errorf("xid=%d", req.XID))
	}
	if _, ok := v.whitelist[remote.IP.String()]; ok {
		return nil
	}
	return dhcpv4errors.Wrap(dhcpv4errors.CodeRelayRejected, "relay source not allowed", fmt.Errorf("remote=%s xid=%d", remote.IP.String(), req.XID))
}

func firstIPv4(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	v4 := net.IP(ip).To4()
	if v4 == nil {
		return nil
	}
	if v4.Equal(net.IPv4zero) {
		return nil
	}
	return v4
}
