package metrics

import (
	"strings"
	"time"

	basemetrics "modern-dhcp/internal/metrics"
)

type Observer interface {
	ObservePacket(packetType string)
	ObserveLeaseOperation(op string, started time.Time)
}

type PromObserver struct {
	collector *basemetrics.Collector
}

func NewObserver(collector *basemetrics.Collector) Observer {
	return &PromObserver{collector: collector}
}

func (o *PromObserver) ObservePacket(packetType string) {
	if o == nil || o.collector == nil || o.collector.DHCPv4Packets == nil {
		return
	}
	if strings.TrimSpace(packetType) == "" {
		return
	}
	o.collector.DHCPv4Packets.WithLabelValues(packetType).Inc()
}

func (o *PromObserver) ObserveLeaseOperation(op string, started time.Time) {
	if o == nil || o.collector == nil || o.collector.DHCPv4LeaseOpDuration == nil {
		return
	}
	if strings.TrimSpace(op) == "" {
		return
	}
	o.collector.DHCPv4LeaseOpDuration.WithLabelValues(op).Observe(time.Since(started).Seconds())
}
