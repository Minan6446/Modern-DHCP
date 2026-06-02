//go:build !dhcpv4_pcap

package dhcpv4

import (
	"context"
	"time"
)

func (s *Server) dhcpv4StartRogueDetector(ctx context.Context) {
	if s.logger != nil {
		s.logger.Warn("dhcpv4 rogue detector requires cgo+pcap; running without passive rogue detection")
	}
	<-ctx.Done()
}

func (s *Server) rogueDetectorReadTimeout() time.Duration {
	return 2 * time.Second
}
