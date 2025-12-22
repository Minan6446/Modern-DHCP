package alerting

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Event represents a normalized alert raised by the platform.
type Event struct {
	ID          string
	Severity    Severity
	TenantID    string
	Category    string
	Summary     string
	Details     string
	Labels      map[string]string
	Resources   []string
	Channels    []string
	OccurredAt  time.Time
	Occurrences int
}

// Notifier delivers alerts to downstream channels.
type Notifier interface {
	Name() string
	Notify(ctx context.Context, event Event) error
}

// ManagerOptions configure the alert routing engine.
type ManagerOptions struct {
	Logger           *zap.Logger
	DedupeWindow     time.Duration
	EscalationWindow time.Duration
}

// Manager fan-outs alerts according to configured routes and notifiers.
type Manager struct {
	logger           *zap.Logger
	mu               sync.Mutex
	notifiers        map[string]Notifier
	routes           []route
	dedupe           map[string]time.Time
	dedupeWindow     time.Duration
	escalationWindow time.Duration
	mute             map[string]time.Time
}

type route struct {
	name       string
	severities map[Severity]struct{}
	channels   []string
}

// NewManager builds a new Manager instance.
func NewManager(opts ManagerOptions) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	window := opts.DedupeWindow
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &Manager{
		logger:           logger,
		notifiers:        make(map[string]Notifier),
		routes:           make([]route, 0),
		dedupe:           make(map[string]time.Time),
		mute:             make(map[string]time.Time),
		dedupeWindow:     window,
		escalationWindow: opts.EscalationWindow,
	}
}

// RegisterNotifier registers a delivery channel by name.
func (m *Manager) RegisterNotifier(name string, notifier Notifier) {
	if notifier == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers[strings.ToLower(name)] = notifier
}

// ConfigureRoutes replaces the current routing table.
func (m *Manager) ConfigureRoutes(routes []route) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = routes
}

// Notify processes an alert event through the routing table.
func (m *Manager) Notify(ctx context.Context, event Event) {
	if event.Severity == "" {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	fingerprint := Fingerprint(event)
	if m.shouldSkip(fingerprint) {
		return
	}
	channels := append([]string(nil), event.Channels...)
	if len(channels) == 0 {
		matched := m.findRoutes(event.Severity)
		for _, rt := range matched {
			channels = append(channels, rt.channels...)
		}
	}
	if len(channels) == 0 {
		m.logger.Debug("alert dropped; no routes", zap.String("severity", string(event.Severity)), zap.String("category", event.Category))
		return
	}
	for _, channel := range channels {
		notifier := m.lookupNotifier(channel)
		if notifier == nil {
			m.logger.Warn("alert channel not registered", zap.String("channel", channel))
			continue
		}
		if err := notifier.Notify(ctx, event); err != nil {
			m.logger.Error("alert delivery failed", zap.String("channel", channel), zap.Error(err))
		}
	}
	m.markProcessed(fingerprint)
}

func (m *Manager) shouldSkip(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if expiry, ok := m.mute[key]; ok {
		if time.Now().Before(expiry) {
			return true
		}
		delete(m.mute, key)
	}
	if last, ok := m.dedupe[key]; ok {
		if time.Since(last) < m.dedupeWindow {
			return true
		}
	}
	return false
}

func (m *Manager) markProcessed(key string) {
	m.mu.Lock()
	m.dedupe[key] = time.Now()
	m.mu.Unlock()
}

// Acknowledge silences the fingerprint for the provided TTL.
func (m *Manager) Acknowledge(fingerprint string, ttl time.Duration) {
	m.addMute(fingerprint, ttl)
}

// Suppress silences the fingerprint for the provided TTL.
func (m *Manager) Suppress(fingerprint string, ttl time.Duration) {
	m.addMute(fingerprint, ttl)
}

func (m *Manager) addMute(key string, ttl time.Duration) {
	if key == "" {
		return
	}
	if ttl <= 0 {
		ttl = m.escalationWindow
		if ttl <= 0 {
			ttl = 10 * time.Minute
		}
	}
	expires := time.Now().Add(ttl)
	m.mu.Lock()
	m.mute[key] = expires
	m.mu.Unlock()
}

func (m *Manager) findRoutes(sev Severity) []route {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]route, 0, len(m.routes))
	for _, rt := range m.routes {
		if _, ok := rt.severities[sev]; ok {
			out = append(out, rt)
		}
	}
	return out
}

func (m *Manager) lookupNotifier(name string) Notifier {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifiers[strings.ToLower(name)]
}

// BuildRoutes constructs route definitions from user configuration.
func BuildRoutes(configs []RouteConfig) []route {
	routes := make([]route, 0, len(configs))
	for _, cfg := range configs {
		sevSet := make(map[Severity]struct{})
		if len(cfg.Severities) == 0 {
			sevSet[SeverityCritical] = struct{}{}
			sevSet[SeverityMajor] = struct{}{}
			sevSet[SeverityWarning] = struct{}{}
			sevSet[SeverityInfo] = struct{}{}
		} else {
			for _, raw := range cfg.Severities {
				if sev, ok := ParseSeverity(raw); ok {
					sevSet[sev] = struct{}{}
				}
			}
		}
		channels := make([]string, 0, len(cfg.Channels))
		for _, ch := range cfg.Channels {
			channels = append(channels, strings.ToLower(strings.TrimSpace(ch)))
		}
		if len(channels) == 0 {
			continue
		}
		routes = append(routes, route{name: cfg.Name, severities: sevSet, channels: channels})
	}
	return routes
}

// RouteConfig mirrors user provided route definitions.
type RouteConfig struct {
	Name       string
	Severities []string
	Channels   []string
}

// ErrChannelExists indicates duplicate registrations.
var ErrChannelExists = fmt.Errorf("alert channel already registered")
