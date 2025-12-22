package failover

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// QuorumState captures the aggregated health of coordinator backends.
type QuorumState struct {
	Healthy      int
	Total        int
	PrimaryVotes int
}

// coordinatorQuorumReporter exposes quorum telemetry for metrics.
type coordinatorQuorumReporter interface {
	CoordinatorQuorum() QuorumState
}

// multiCoordinator fans out to multiple coordinator implementations and
// evaluates quorum/leadership based on majority votes.
type multiCoordinator struct {
	logger       *zap.Logger
	coordinators []Coordinator
	startOnce    sync.Once
	stopOnce     sync.Once
}

func newMultiCoordinator(children []Coordinator, logger *zap.Logger) Coordinator {
	if len(children) == 0 {
		return defaultCoordinator()
	}
	if len(children) == 1 && children[0] != nil {
		return children[0]
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	mc := &multiCoordinator{logger: logger}
	for _, c := range children {
		if c != nil {
			mc.coordinators = append(mc.coordinators, c)
		}
	}
	if len(mc.coordinators) == 0 {
		return defaultCoordinator()
	}
	return mc
}

func (m *multiCoordinator) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		for _, coord := range m.coordinators {
			if coord == nil {
				continue
			}
			coord.Start(ctx)
		}
	})
}

func (m *multiCoordinator) Stop() {
	m.stopOnce.Do(func() {
		for _, coord := range m.coordinators {
			if coord == nil {
				continue
			}
			coord.Stop()
		}
	})
}

func (m *multiCoordinator) Healthy() bool {
	state, _ := m.snapshot()
	if state.Total == 0 {
		return true
	}
	// Require a strict majority of healthy coordinators to stay active.
	return state.Healthy*2 > state.Total
}

func (m *multiCoordinator) ShouldServePrimary() bool {
	state, serve := m.snapshot()
	if state.Total == 0 {
		return true
	}
	if serve {
		return true
	}
	return false
}

func (m *multiCoordinator) snapshot() (QuorumState, bool) {
	var state QuorumState
	for _, coord := range m.coordinators {
		if coord == nil {
			continue
		}
		state.Total++
		if coord.Healthy() {
			state.Healthy++
			if coord.ShouldServePrimary() {
				state.PrimaryVotes++
			}
		}
	}
	if state.Total == 0 {
		return state, true
	}
	if state.Healthy == 0 {
		return state, false
	}
	return state, state.PrimaryVotes*2 > state.Healthy
}

func (m *multiCoordinator) CoordinatorQuorum() QuorumState {
	state, _ := m.snapshot()
	return state
}

// Ensure interface compliance.
var _ Coordinator = (*multiCoordinator)(nil)
var _ coordinatorQuorumReporter = (*multiCoordinator)(nil)
