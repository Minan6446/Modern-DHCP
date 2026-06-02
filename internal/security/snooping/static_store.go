package snooping

import (
	"context"
	"strings"
	"time"

	"modern-dhcp/internal/metrics"
)

// StaticOptions builds a simple in-memory trusted port map.
type StaticOptions struct {
	TrustedPorts []string
	TTL          time.Duration
	Metrics      *metrics.Collector
}

type staticStore struct {
	trusted map[string]struct{}
	ttl     time.Duration
	metrics *metrics.Collector
}

// NewStaticStore returns a store that trusts a static list of port identifiers.
func NewStaticStore(opts StaticOptions) Store {
	normalized := make(map[string]struct{}, len(opts.TrustedPorts))
	for _, port := range opts.TrustedPorts {
		port = strings.TrimSpace(strings.ToLower(port))
		if port == "" {
			continue
		}
		normalized[port] = struct{}{}
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &staticStore{trusted: normalized, ttl: ttl, metrics: opts.Metrics}
}

func (s *staticStore) Lookup(ctx context.Context, key LookupKey) (*Binding, error) {
	if len(s.trusted) == 0 {
		s.recordMetric("miss")
		return nil, ErrBindingNotFound
	}
	portID := strings.TrimSpace(strings.ToLower(key.PortID))
	if portID == "" {
		s.recordMetric("miss")
		return nil, ErrBindingNotFound
	}
	if _, ok := s.trusted[portID]; !ok {
		s.recordMetric("untrusted")
		return nil, ErrUntrusted
	}
	binding := &Binding{
		TenantID:   key.TenantID,
		MACAddress: key.MAC,
		PortID:     key.PortID,
		VLANID:     key.VLANID,
		Trusted:    true,
		ExpiresAt:  time.Now().Add(s.ttl),
	}
	s.recordMetric("hit")
	return binding, nil
}

func (s *staticStore) StreamChanges(ctx context.Context) (<-chan Binding, error) {
	ch := make(chan Binding)
	close(ch)
	return ch, nil
}

func (s *staticStore) recordMetric(result string) {
	if s.metrics == nil || s.metrics.SnoopingCache == nil {
		return
	}
	s.metrics.SnoopingCache.WithLabelValues(result).Inc()
}
