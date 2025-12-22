package failover

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/metrics"
)

// State captures the failover status derived from ISC DHCP semantics.
type State string

const (
	// StateInit indicates the state machine has not observed peer heartbeats yet.
	StateInit State = "init"
	// StateNormal mirrors the ISC "normal" state where both peers are healthy.
	StateNormal State = "normal"
	// StateCommInterrupted maps to "communication-interrupted" (heartbeat missing but partner not yet down).
	StateCommInterrupted State = "comm_interrupted"
	// StatePartnerDown indicates the partner is considered offline and local node may promote itself.
	StatePartnerDown State = "partner_down"
)

// Role indicates the current serving role of the node within the failover pair or cluster.
type Role string

const (
	// RoleUnknown is the default role before the manager acquires any coordination state.
	RoleUnknown Role = "unknown"
	// RolePrimary indicates the node currently considers itself active/serving.
	RolePrimary Role = "primary"
	// RoleStandby indicates the node is in hot standby and should avoid serving writes.
	RoleStandby Role = "standby"
)

// StatusSnapshot exposes lightweight HA state to other subsystems.
type StatusSnapshot struct {
	Role              Role      `json:"role"`
	State             State     `json:"state"`
	PeerLastSeen      time.Time `json:"peerLastSeen"`
	PeerHealthySince  time.Time `json:"peerHealthySince"`
	ManualFailbackSet bool      `json:"manualFailbackSet"`
}

// NodeStatus summarizes HA membership details for observability surfaces.
type NodeStatus struct {
	ID            string    `json:"id"`
	Role          Role      `json:"role"`
	State         State     `json:"state"`
	Region        string    `json:"region,omitempty"`
	Zone          string    `json:"zone,omitempty"`
	Weight        int       `json:"weight,omitempty"`
	Self          bool      `json:"self"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	PeerLastSeen  time.Time `json:"peerLastSeen"`
	Warnings      []string  `json:"warnings,omitempty"`
}

// ManualFailoverRequest describes a manual failover/failback operation.
type ManualFailoverRequest struct {
	TargetRole Role
	Reason     string
	DryRun     bool
	Force      bool
}

// IngressPolicy captures runtime load-balancer preferences.
type IngressPolicy struct {
	Strategy       string
	Weights        map[string]int
	StickyDuration time.Duration
}

var (
	// ErrManualFailoverDisabled indicates manual failover is unavailable.
	ErrManualFailoverDisabled = errors.New("failover: manual control disabled")
	// ErrManualFailoverPeerHealthy indicates failover was rejected because the peer is healthy.
	ErrManualFailoverPeerHealthy = errors.New("failover: peer still healthy")
	// ErrManualFailoverInvalid indicates the target role was invalid.
	ErrManualFailoverInvalid = errors.New("failover: invalid target role")
	// ErrIngressControllerUnavailable indicates no ingress controller is wired.
	ErrIngressControllerUnavailable = errors.New("failover: ingress controller unavailable")
)

// StatusReporter exposes HA snapshots to other packages (e.g. HTTP server healthz).
type StatusReporter interface {
	Snapshot() StatusSnapshot
}

// Manager is a placeholder high-availability coordinator responsible for
// heartbeats, role negotiation, and future failover orchestration.
type Manager struct {
	cfg    config.HAConfig
	logger *zap.Logger

	mu       sync.RWMutex
	role     Role
	state    State
	ticker   *time.Ticker
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	replicator            Replicator
	configWatcher         ConfigWatcher
	coordinator           Coordinator
	heartbeat             HeartbeatTransport
	ingress               IngressController
	broadcaster           BroadcastBus
	discovery             DiscoveryRegistry
	probe                 HealthProbe
	responder             ProbeResponder
	peerLastSeen          time.Time
	peerHealthySince      time.Time
	failTimeout           time.Duration
	mclt                  time.Duration
	expectedSum           string
	probeFailureThreshold int
	failbackMode          string
	failbackStable        time.Duration
	manualFailbackAllowed atomic.Bool
	metrics               metricsRecorder
}

// NewManager constructs a HA manager for the provided configuration. The
// manager is inert until Start is invoked.
func NewManager(cfg config.HAConfig, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	mgr := &Manager{
		cfg:                   cfg,
		logger:                logger,
		role:                  RoleUnknown,
		state:                 StateInit,
		replicator:            defaultReplicator(),
		configWatcher:         defaultConfigWatcher(),
		coordinator:           buildCoordinator(cfg, logger),
		heartbeat:             defaultHeartbeat(),
		ingress:               buildIngressController(cfg, logger),
		broadcaster:           buildBroadcastBus(cfg, logger),
		discovery:             buildDiscoveryRegistry(cfg, logger),
		probe:                 buildHealthProbe(cfg, logger),
		failTimeout:           durationOrDefault(cfg.FailoverTimeout, 450*time.Millisecond),
		mclt:                  cfg.Partner.MCLT,
		expectedSum:           cfg.ConfigSync.ExpectedChecksum,
		failbackMode:          strings.ToLower(strings.TrimSpace(cfg.Failback.Mode)),
		failbackStable:        cfg.Failback.StablePeriod,
		probeFailureThreshold: cfg.Probe.FailureThreshold,
		responder:             buildProbeResponder(cfg, logger),
		metrics:               noopMetricsRecorder{},
	}
	if mgr.probeFailureThreshold <= 0 {
		mgr.probeFailureThreshold = 3
	}
	if mgr.mclt <= 0 {
		mgr.mclt = 2 * time.Minute
	}
	if mgr.failbackMode == "" {
		mgr.failbackMode = "auto"
	}
	if mgr.failbackStable <= 0 {
		mgr.failbackStable = time.Minute
	}
	return mgr
}

// WithMetricsCollector attaches a metrics collector to report HA telemetry.
func (m *Manager) WithMetricsCollector(collector *metrics.Collector) *Manager {
	if m == nil {
		return m
	}
	m.metrics = newMetricsRecorder(collector)
	m.metrics.RecordRoleTransition("", m.role)
	m.metrics.RecordStateTransition("", m.state)
	m.metrics.RecordPeerGap(0)
	return m
}

// Start begins emitting heartbeat placeholders. A disabled or empty HA mode
// results in a no-op so callers need not branch.
func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	if strings.EqualFold(m.cfg.Mode, "disabled") || m.cfg.Mode == "" {
		m.logger.Info("failover manager disabled; skipping startup")
		return
	}
	if m.ticker != nil {
		return
	}
	interval := m.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 125 * time.Millisecond
	}
	m.ticker = time.NewTicker(interval)
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	m.replicator.Start(ctx)
	m.configWatcher.Start(ctx)
	m.coordinator.Start(ctx)
	m.heartbeat.Start(ctx, func() {
		m.MarkPeerHeartbeat(time.Now())
	})
	m.ingress.Start(ctx, m.cfg.Node, m.cfg.Ingress)
	m.broadcaster.Start(ctx, m.cfg.Broadcast)
	m.discovery.Start(ctx, m.cfg.Node, m.cfg.Discovery)
	if m.responder != nil {
		m.responder.Start(ctx, m.cfg.Probe)
	}
	if m.probe != nil {
		probeCfg := m.cfg.Probe
		if probeCfg.Target == "" {
			probeCfg.Target = m.cfg.Partner.Address
		}
		if probeCfg.Port == 0 {
			probeCfg.Port = m.cfg.Partner.Port
		}
		m.probe.Start(ctx, probeCfg, m.handleProbeResult)
	}
	m.updateIngress()
	m.refreshDiscovery()
	m.publishState()
	go m.loop(ctx)
}

// Stop halts the heartbeat loop if it is running.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.ticker != nil {
			m.ticker.Stop()
		}
		if m.stopCh != nil {
			close(m.stopCh)
		}
		if m.doneCh != nil {
			<-m.doneCh
		}
		m.replicator.Stop()
		m.configWatcher.Stop()
		m.coordinator.Stop()
		m.heartbeat.Stop()
		m.ingress.Stop()
		m.broadcaster.Stop()
		m.discovery.Stop()
		if m.responder != nil {
			m.responder.Stop()
		}
		if m.probe != nil {
			m.probe.Stop()
		}
		m.ticker = nil
		m.stopCh = nil
		m.doneCh = nil
	})
}

// Role returns the currently advertised node role.
func (m *Manager) Role() Role {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.role
}

func (m *Manager) loop(ctx context.Context) {
	defer close(m.doneCh)
	m.logger.Info("failover manager started", zap.String("mode", m.cfg.Mode))
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("failover manager stopping: context cancelled")
			return
		case <-m.stopCh:
			m.logger.Info("failover manager stopping: stop requested")
			return
		case ts := <-m.ticker.C:
			m.emitHeartbeat(ts)
			m.evaluatePeer(ts)
			m.validateConfig()
			m.reconcileCoordinator()
			m.observeReplication()
			m.refreshDiscovery()
		}
	}
}

// emitHeartbeat currently logs a placeholder entry so we can wire metrics and
// integration tests later.
func (m *Manager) emitHeartbeat(ts time.Time) {
	m.logger.Debug("failover heartbeat tick",
		zap.Time("ts", ts),
		zap.String("mode", m.cfg.Mode),
		zap.String("role", string(m.Role())),
		zap.String("state", string(m.State())),
	)
}

// MarkPeerHeartbeat should be invoked by the heartbeat transport or TCP failover handler.
func (m *Manager) MarkPeerHeartbeat(ts time.Time) {
	m.mu.Lock()
	m.peerLastSeen = ts
	m.mu.Unlock()
	m.setState(StateNormal)
	m.metrics.RecordPeerGap(0)
}

// State returns the current failover state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) evaluatePeer(now time.Time) {
	m.mu.RLock()
	last := m.peerLastSeen
	current := m.state
	m.mu.RUnlock()
	deadline := now.Sub(last)
	m.metrics.RecordPeerGap(deadline.Seconds())
	var next State
	switch {
	case last.IsZero():
		next = StateInit
	case deadline > m.failTimeout:
		if current != StatePartnerDown {
			m.logger.Warn("failover entering partner-down window", zap.Duration("since", deadline), zap.Duration("mclt", m.mclt))
		}
		next = StatePartnerDown
	case deadline > m.failTimeout/2:
		if current != StateCommInterrupted {
			m.logger.Warn("failover communication interrupted", zap.Duration("since", deadline))
		}
		next = StateCommInterrupted
	default:
		next = StateNormal
	}
	m.setState(next)
}

func (m *Manager) validateConfig() {
	if !m.cfg.ConfigSync.Enforce || m.expectedSum == "" {
		return
	}
	if watcherSum := m.configWatcher.LastChecksum(); watcherSum != "" && watcherSum != m.expectedSum {
		m.logger.Warn("config checksum mismatch; demoting to standby", zap.String("expected", m.expectedSum), zap.String("actual", watcherSum))
		m.setRole(RoleStandby)
	}
}

func (m *Manager) reconcileCoordinator() {
	m.recordCoordinatorMetrics()
	if !m.coordinator.Healthy() {
		m.logger.Warn("coordinator unhealthy; forcing standby")
		m.setRole(RoleStandby)
		return
	}
	shouldServePrimary := m.coordinator.ShouldServePrimary()
	if shouldServePrimary && m.State() != StatePartnerDown {
		m.setRole(RolePrimary)
		return
	}
	if shouldServePrimary {
		return
	}
	currentRole := m.Role()
	if strings.EqualFold(m.failbackMode, "manual") && currentRole == RolePrimary && !m.manualFailbackAllowed.Load() {
		m.logger.Debug("manual failback pending approval; remaining primary")
		return
	}
	if strings.EqualFold(m.failbackMode, "auto") && currentRole == RolePrimary && m.failbackStable > 0 {
		var healthyFor time.Duration
		if !m.peerHealthySince.IsZero() {
			healthyFor = time.Since(m.peerHealthySince)
		}
		if healthyFor < m.failbackStable {
			m.logger.Debug("waiting for peer stability before auto-failback", zap.Duration("remaining", m.failbackStable-healthyFor))
			return
		}
	}
	if currentRole == RolePrimary && m.manualFailbackAllowed.Load() {
		m.logger.Info("manual failback approved; yielding VIP")
	}
	m.setRole(RoleStandby)
}

func (m *Manager) recordCoordinatorMetrics() {
	var state QuorumState
	if reporter, ok := m.coordinator.(coordinatorQuorumReporter); ok {
		state = reporter.CoordinatorQuorum()
	} else if m.coordinator != nil {
		state.Total = 1
		if m.coordinator.Healthy() {
			state.Healthy = 1
			if m.coordinator.ShouldServePrimary() {
				state.PrimaryVotes = 1
			}
		}
	}
	m.metrics.RecordCoordinatorQuorum(state)
}

func (m *Manager) observeReplication() {
	if m.replicator == nil {
		return
	}
	if !m.replicator.Healthy() {
		m.logger.Warn("replicator unhealthy; standby sync may lag")
	}
}

func (m *Manager) setRole(role Role) {
	m.mu.Lock()
	prev := m.role
	changed := prev != role
	if changed {
		m.logger.Info("failover role change", zap.String("from", string(prev)), zap.String("to", string(role)))
		m.role = role
		if role == RoleStandby {
			m.manualFailbackAllowed.Store(false)
		}
	}
	m.mu.Unlock()
	if changed {
		m.metrics.RecordRoleTransition(prev, role)
		m.publishState()
		m.updateIngress()
	}
}

func (m *Manager) setState(next State) {
	m.mu.Lock()
	prev := m.state
	changed := prev != next
	if changed {
		m.state = next
		if next == StateNormal {
			m.peerHealthySince = time.Now()
		}
	}
	m.mu.Unlock()
	if changed {
		m.metrics.RecordStateTransition(prev, next)
		m.publishState()
	}
}

func (m *Manager) publishState() {
	if m.broadcaster == nil {
		return
	}
	m.broadcaster.Publish(m.Role(), m.State())
}

func (m *Manager) updateIngress() {
	if m.ingress == nil {
		return
	}
	if !m.ingress.Healthy() {
		m.logger.Warn("ingress controller unhealthy; advertisements may be stale")
	}
	m.ingress.Advertise(m.Role())
}

func (m *Manager) refreshDiscovery() {
	if m.discovery == nil {
		return
	}
	if !m.discovery.Healthy() {
		m.logger.Warn("discovery registry unhealthy; entries may expire")
	}
	m.discovery.Refresh(m.Role())
}

// AllowManualFailback approves a pending manual failback request.
func (m *Manager) AllowManualFailback() {
	if m == nil {
		return
	}
	m.manualFailbackAllowed.Store(true)
}

// TriggerFailover enforces a manual failover/failback request.
func (m *Manager) TriggerFailover(ctx context.Context, req ManualFailoverRequest) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if m == nil {
		return ErrManualFailoverDisabled
	}
	target := req.TargetRole
	if target != RoleStandby {
		target = RolePrimary
	}
	if req.DryRun {
		m.logger.Info("failover dry-run", zap.String("targetRole", string(target)), zap.String("reason", req.Reason), zap.Bool("force", req.Force))
		return nil
	}
	switch target {
	case RolePrimary:
		if m.State() == StateNormal && !req.Force {
			return ErrManualFailoverPeerHealthy
		}
		m.setRole(RolePrimary)
	case RoleStandby:
		m.setRole(RoleStandby)
	default:
		return ErrManualFailoverInvalid
	}
	m.logger.Info("manual failover applied", zap.String("targetRole", string(target)), zap.String("reason", req.Reason), zap.Bool("force", req.Force))
	m.updateIngress()
	return nil
}

// UpdateIngressPolicy pushes runtime load-balancer adjustments.
func (m *Manager) UpdateIngressPolicy(ctx context.Context, policy IngressPolicy) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if m == nil {
		return ErrManualFailoverDisabled
	}
	if m.ingress == nil {
		return ErrIngressControllerUnavailable
	}
	if err := m.ingress.UpdatePolicy(policy); err != nil {
		return err
	}
	m.logger.Info("ingress policy updated", zap.String("strategy", policy.Strategy))
	return nil
}

// Nodes returns a coarse snapshot of HA membership state.
func (m *Manager) Nodes() []NodeStatus {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	self := NodeStatus{
		ID:            m.cfg.Node.ID,
		Role:          m.role,
		State:         m.state,
		Region:        m.cfg.Node.Region,
		Zone:          m.cfg.Node.Zone,
		Weight:        m.cfg.Node.Weight,
		Self:          true,
		LastHeartbeat: time.Now().UTC(),
		PeerLastSeen:  m.peerLastSeen,
	}
	if warn := m.nodeWarningsLocked(); len(warn) > 0 {
		self.Warnings = warn
	}
	nodes := []NodeStatus{self}
	if !m.peerLastSeen.IsZero() {
		nodes = append(nodes, NodeStatus{
			ID:            "peer",
			Role:          oppositeRole(m.role),
			State:         m.state,
			LastHeartbeat: m.peerLastSeen,
			PeerLastSeen:  m.peerLastSeen,
		})
	}
	return nodes
}

func (m *Manager) nodeWarningsLocked() []string {
	var warnings []string
	switch m.state {
	case StateCommInterrupted:
		warnings = append(warnings, "peer unreachable")
	case StatePartnerDown:
		warnings = append(warnings, "peer down")
	}
	return warnings
}

func oppositeRole(role Role) Role {
	if role == RolePrimary {
		return RoleStandby
	}
	return RolePrimary
}

// Snapshot captures current HA role/state for health endpoints.
func (m *Manager) Snapshot() StatusSnapshot {
	if m == nil {
		return StatusSnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return StatusSnapshot{
		Role:              m.role,
		State:             m.state,
		PeerLastSeen:      m.peerLastSeen,
		PeerHealthySince:  m.peerHealthySince,
		ManualFailbackSet: m.manualFailbackAllowed.Load(),
	}
}

func (m *Manager) handleProbeResult(res ProbeResult) {
	if res.Healthy {
		m.MarkPeerHeartbeat(res.CheckedAt)
		return
	}
	if res.Failures >= m.probeFailureThreshold {
		m.logger.Warn("active probe marked partner down", zap.Int("failures", res.Failures))
		m.setState(StatePartnerDown)
		return
	}
	m.setState(StateCommInterrupted)
}
