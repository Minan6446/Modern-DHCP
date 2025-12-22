package failover

import (
	"modern-dhcp/internal/metrics"
)

type metricsRecorder interface {
	RecordRoleTransition(from, to Role)
	RecordStateTransition(from, to State)
	RecordPeerGap(seconds float64)
	RecordCoordinatorQuorum(state QuorumState)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) RecordRoleTransition(from, to Role)        {}
func (noopMetricsRecorder) RecordStateTransition(from, to State)      {}
func (noopMetricsRecorder) RecordPeerGap(seconds float64)             {}
func (noopMetricsRecorder) RecordCoordinatorQuorum(state QuorumState) {}

type promMetricsRecorder struct {
	collector *metrics.Collector
}

func newMetricsRecorder(collector *metrics.Collector) metricsRecorder {
	if collector == nil {
		return noopMetricsRecorder{}
	}
	return &promMetricsRecorder{collector: collector}
}

func (p *promMetricsRecorder) RecordRoleTransition(from, to Role) {
	if p == nil || p.collector == nil || p.collector.HARole == nil {
		return
	}
	if from != "" {
		p.collector.HARole.WithLabelValues(string(from)).Set(0)
	}
	if to != "" {
		p.collector.HARole.WithLabelValues(string(to)).Set(1)
	}
	if p.collector.HAFailoverTransitions != nil && from != "" && to != "" && from != to {
		p.collector.HAFailoverTransitions.WithLabelValues(string(from), string(to)).Inc()
	}
}

func (p *promMetricsRecorder) RecordStateTransition(from, to State) {
	if p == nil || p.collector == nil || p.collector.HAState == nil {
		return
	}
	if from != "" {
		p.collector.HAState.WithLabelValues(string(from)).Set(0)
	}
	if to != "" {
		p.collector.HAState.WithLabelValues(string(to)).Set(1)
	}
}

func (p *promMetricsRecorder) RecordPeerGap(seconds float64) {
	if p == nil || p.collector == nil || p.collector.HAPeerGapSeconds == nil {
		return
	}
	p.collector.HAPeerGapSeconds.Set(seconds)
}

func (p *promMetricsRecorder) RecordCoordinatorQuorum(state QuorumState) {
	if p == nil || p.collector == nil || p.collector.HACoordinatorQuorum == nil {
		return
	}
	p.collector.HACoordinatorQuorum.WithLabelValues("healthy").Set(float64(state.Healthy))
	p.collector.HACoordinatorQuorum.WithLabelValues("total").Set(float64(state.Total))
	p.collector.HACoordinatorQuorum.WithLabelValues("primary_votes").Set(float64(state.PrimaryVotes))
}
