//go:build dhcpv4_pcap && (linux || windows) && cgo

package dhcpv4

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"go.uber.org/zap"
)

func (s *Server) dhcpv4StartRogueDetector(ctx context.Context) {
	iface := strings.TrimSpace(s.rogueConfig.Interface)
	if iface == "" {
		if s.logger != nil {
			s.logger.Warn("dhcpv4 rogue detector disabled: interface is empty")
		}
		return
	}
	handle, err := pcap.OpenLive(iface, 1600, true, s.rogueDetectorReadTimeout())
	if err != nil {
		if s.logger != nil {
			s.logger.Error("dhcpv4 rogue detector start failed", zap.Error(err), zap.String("interface", iface))
		}
		return
	}
	defer handle.Close()
	if err := handle.SetBPFFilter("udp and src port 67 and dst port 68"); err != nil {
		if s.logger != nil {
			s.logger.Error("dhcpv4 rogue detector bpf failed", zap.Error(err))
		}
		return
	}
	if s.logger != nil {
		s.logger.Info("dhcpv4 rogue detector started", zap.String("interface", iface))
	}
	packets := gopacket.NewPacketSource(handle, handle.LinkType()).Packets()
	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-packets:
			if !ok || packet == nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			s.inspectRogueOffer(packet)
		}
	}
}

func (s *Server) inspectRogueOffer(packet gopacket.Packet) {
	ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
	udpLayer := packet.Layer(layers.LayerTypeUDP)
	dhcpLayer := packet.Layer(layers.LayerTypeDHCPv4)
	if ipv4Layer == nil || udpLayer == nil || dhcpLayer == nil {
		return
	}
	ipv4, ok := ipv4Layer.(*layers.IPv4)
	if !ok {
		return
	}
	dhcp, ok := dhcpLayer.(*layers.DHCPv4)
	if !ok || dhcp.Operation != layers.DHCPOpReply {
		return
	}
	msgType := byte(0)
	for _, opt := range dhcp.Options {
		if opt.Type == layers.DHCPOptMessageType && len(opt.Data) > 0 {
			msgType = opt.Data[0]
			break
		}
	}
	if msgType != MessageTypeOffer {
		return
	}
	sourceIP := net.IP(ipv4.SrcIP)
	if s.rogueAllowed(sourceIP) {
		return
	}
	if s.logger != nil {
		s.logger.Error("rogue dhcp offer detected",
			zap.String("sourceIp", sourceIP.String()),
			zap.String("messageType", "OFFER"),
		)
	}
}

func (s *Server) rogueDetectorReadTimeout() time.Duration {
	return 2 * time.Second
}
