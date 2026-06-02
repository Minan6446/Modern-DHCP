package dhcpv4

import (
	"context"
	"fmt"
	"net"

	dhcpv4security "modern-dhcp/internal/dhcpv4/security"

	"go.uber.org/zap"
)

type RogueDetectorConfig struct {
	Enabled      bool
	Interface    string
	ClusterNodes []string
}

type SecurityOptions struct {
	MACRateLimitPPS int
	RelayWhitelist  []string
	RogueDetector   RogueDetectorConfig
}

type dhcpv4MACRateLimiter struct {
	delegate dhcpv4security.MACLimiter
}

func dhcpv4NewMACRateLimiter(pps int) *dhcpv4MACRateLimiter {
	return &dhcpv4MACRateLimiter{delegate: dhcpv4security.NewMACRateLimiter(pps)}
}

func (l *dhcpv4MACRateLimiter) allow(mac string) bool {
	if l == nil {
		return true
	}
	if l.delegate == nil {
		return true
	}
	return l.delegate.Allow(mac)
}

func normalizeIPSet(values []string) map[string]struct{} {
	return dhcpv4security.NormalizeIPSet(values)
}

func (s *Server) dhcpv4AllowByMAC(msg *Message, remote *net.UDPAddr) bool {
	if s.macLimiter == nil {
		return true
	}
	if msg == nil {
		return true
	}
	mac := net.HardwareAddr(msg.CHAddr).String()
	if s.macLimiter.allow(mac) {
		return true
	}
	if s.logger != nil {
		s.logger.Warn("dhcpv4 mac rate limit exceeded; dropping packet",
			zap.String("mac", mac),
			zap.String("remote", udpAddrString(remote)),
			zap.Uint32("xid", msg.XID),
		)
	}
	return false
}

func (s *Server) dhcpv4RelayValidateOrReject(ctx context.Context, msg *Message, pkt Packet, remote *net.UDPAddr) bool {
	if len(s.relayIPWhitelist) == 0 {
		return true
	}
	relayWhitelist := s.opts.Security.RelayWhitelist
	if len(relayWhitelist) == 0 {
		relayWhitelist = make([]string, 0, len(s.relayIPWhitelist))
		for ip := range s.relayIPWhitelist {
			relayWhitelist = append(relayWhitelist, ip)
		}
	}
	validator := dhcpv4security.NewRelayValidator(relayWhitelist)
	err := validator.Validate(dhcpv4security.RelayRequest{
		Option82Present: pkt.Option82Present,
		GIAddr:          pkt.GIAddr,
		XID:             pkt.XID,
	}, remote)
	if err == nil {
		return true
	}
	if s.logger != nil {
		remoteIP := ""
		if remote != nil && remote.IP != nil {
			remoteIP = remote.IP.String()
		}
		dhcpv4Logger(ctx, s.logger).Error("dhcpv4 relay source rejected",
			zap.String("remote", remoteIP),
			zap.Uint32("xid", pkt.XID),
			zap.Error(err),
		)
	}
	reason := "relay source not allowed"
	if remote == nil || remote.IP == nil {
		reason = "relay source validation failed"
	}
	if wrapped := fmt.Sprintf("%v", err); wrapped != "" {
		reason = wrapped
	}
	s.sendNak(ctx, msg, remote, reason)
	return false
}

func udpAddrString(addr *net.UDPAddr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}

func (s *Server) startRogueDetector(ctx context.Context) {
	if !s.rogueConfig.Enabled {
		return
	}
	go s.dhcpv4StartRogueDetector(ctx)
}

func (s *Server) rogueAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	_, ok := s.rogueClusterNodes[ip.String()]
	return ok
}
