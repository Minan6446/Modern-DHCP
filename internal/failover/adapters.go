package failover

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// keepalivedIngress shells out to ip/netsh to toggle VIP ownership when role changes.
type keepalivedIngress struct {
	logger        *zap.Logger
	runner        commandRunner
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex
	cfg           config.IngressConfig
	policy        IngressPolicy
	meta          config.HANodeMetadata
	grace         time.Duration
	lastRole      Role
	lastAdvertise time.Time
	healthy       atomic.Bool
	httpClient    *http.Client
}

type httpCoordinator struct {
	name      string
	backend   string
	endpoints []string
	key       string
	token     string
	logger    *zap.Logger
	client    *http.Client
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
	healthy   atomic.Bool
	allow     atomic.Bool
}

type coordinatorStatusResponse struct {
	Healthy      *bool  `json:"healthy"`
	AllowPrimary *bool  `json:"allowPrimary"`
	Role         string `json:"role"`
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func newKeepalivedIngress(logger *zap.Logger) *keepalivedIngress {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &keepalivedIngress{logger: logger, runner: execRunner{}, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func (k *keepalivedIngress) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.IngressConfig) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.cancel != nil {
		k.cancel()
	}
	k.ctx, k.cancel = context.WithCancel(ctx)
	k.meta = meta
	k.cfg = cfg
	k.grace = cfg.HealthGrace
	if k.grace <= 0 {
		k.grace = 5 * time.Second
	}
	if cfg.VIPInterface == "" || cfg.VIPAddress == "" {
		k.logger.Warn("keepalived ingress missing VIP configuration")
		k.healthy.Store(false)
		return
	}
	k.lastRole = RoleUnknown
	k.lastAdvertise = time.Now()
	k.healthy.Store(true)
	go k.watchdog()
}

func (k *keepalivedIngress) Stop() {
	k.mu.Lock()
	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
		k.ctx = nil
	}
	k.mu.Unlock()
}

func (k *keepalivedIngress) Healthy() bool {
	return k.healthy.Load()
}

func (k *keepalivedIngress) Advertise(role Role) {
	k.mu.Lock()
	ctx := k.ctx
	cfg := k.cfg
	policy := k.policy
	k.lastRole = role
	k.lastAdvertise = time.Now()
	k.mu.Unlock()

	if ctx == nil {
		return
	}
	if cfg.VIPInterface == "" || cfg.VIPAddress == "" {
		return
	}
	err := k.applyIngress(ctx, cfg, role)
	if err != nil {
		k.logger.Warn("failed to update VIP", zap.String("role", string(role)), zap.Error(err))
		k.healthy.Store(false)
		return
	}
	if policy.Strategy != "" {
		k.logger.Debug("ingress policy active", zap.String("strategy", policy.Strategy))
	}
	k.healthy.Store(true)
}

func (k *keepalivedIngress) UpdatePolicy(policy IngressPolicy) error {
	k.mu.Lock()
	k.policy = policy
	logger := k.logger
	k.mu.Unlock()
	if logger != nil && policy.Strategy != "" {
		logger.Info("keepalived ingress policy updated", zap.String("strategy", policy.Strategy))
	}
	return nil
}

func (k *keepalivedIngress) watchdog() {
	ticker := time.NewTicker(k.grace)
	defer ticker.Stop()
	for {
		select {
		case <-k.ctx.Done():
			return
		case <-ticker.C:
			k.mu.RLock()
			since := time.Since(k.lastAdvertise)
			role := k.lastRole
			grace := k.grace
			k.mu.RUnlock()
			if role == RolePrimary && since > 2*grace {
				k.logger.Warn("VIP advertisement stale", zap.Duration("since", since))
				k.healthy.Store(false)
			}
		}
	}
}

func (k *keepalivedIngress) applyIngress(ctx context.Context, cfg config.IngressConfig, role Role) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		if role == RolePrimary {
			err = k.ensureVIP(ctx, cfg)
		} else {
			err = k.withdrawVIP(ctx, cfg)
		}
		if err != nil {
			errCh <- err
		}
	}()
	if cfg.DNS.Provider != "" && cfg.DNSEntry != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := k.updateDNS(ctx, cfg, role); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *keepalivedIngress) ensureVIP(ctx context.Context, cfg config.IngressConfig) error {
	ip := net.ParseIP(cfg.VIPAddress)
	if ip == nil {
		return fmt.Errorf("invalid vip address: %s", cfg.VIPAddress)
	}
	if runtime.GOOS == "windows" {
		mask, err := windowsMask(ip)
		if err != nil {
			return err
		}
		return k.runner.Run(ctx, "netsh", "interface", "ip", "add", "address", cfg.VIPInterface, cfg.VIPAddress, mask)
	}
	prefix := "/32"
	if ip.To4() == nil {
		prefix = "/128"
	}
	return k.runner.Run(ctx, "ip", "addr", "replace", fmt.Sprintf("%s%s", cfg.VIPAddress, prefix), "dev", cfg.VIPInterface)
}

func (k *keepalivedIngress) withdrawVIP(ctx context.Context, cfg config.IngressConfig) error {
	ip := net.ParseIP(cfg.VIPAddress)
	if ip == nil {
		return fmt.Errorf("invalid vip address: %s", cfg.VIPAddress)
	}
	if runtime.GOOS == "windows" {
		return k.runner.Run(ctx, "netsh", "interface", "ip", "delete", "address", cfg.VIPInterface, cfg.VIPAddress)
	}
	prefix := "/32"
	if ip.To4() == nil {
		prefix = "/128"
	}
	return k.runner.Run(ctx, "ip", "addr", "del", fmt.Sprintf("%s%s", cfg.VIPAddress, prefix), "dev", cfg.VIPInterface)
}

func (k *keepalivedIngress) updateDNS(ctx context.Context, cfg config.IngressConfig, role Role) error {
	dns := cfg.DNS
	provider := strings.ToLower(strings.TrimSpace(dns.Provider))
	if provider == "" || dns.Endpoint == "" {
		return nil
	}
	switch provider {
	case "http", "rest":
		return k.postDNSUpdate(ctx, cfg, role)
	default:
		return fmt.Errorf("dns provider %s not supported", provider)
	}
}

func (k *keepalivedIngress) postDNSUpdate(ctx context.Context, cfg config.IngressConfig, role Role) error {
	dns := cfg.DNS
	if dns.Endpoint == "" || cfg.DNSEntry == "" {
		return nil
	}
	ttl := dns.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	payload := map[string]any{
		"fqdn":       cfg.DNSEntry,
		"address":    "",
		"ttlSeconds": int(ttl / time.Second),
		"role":       role,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"active":     role == RolePrimary,
	}
	if role == RolePrimary {
		payload["address"] = cfg.VIPAddress
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dns.Endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if dns.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", dns.Token))
	}
	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dns update failed with status %d", resp.StatusCode)
	}
	return nil
}

func newHTTPCoordinator(def config.NamedCoordinator, logger *zap.Logger) (Coordinator, error) {
	endpoints := append([]string(nil), def.Endpoints...)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("coordinator %s missing endpoints", def.Name)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	interval := def.Interval
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	tlsCfg, err := buildTLSClientConfig(def.TLS)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment},
	}
	name := def.Name
	if name == "" {
		name = def.Backend
	}
	coord := &httpCoordinator{
		name:      name,
		backend:   strings.ToLower(def.Backend),
		endpoints: endpoints,
		key:       def.Key,
		token:     def.Token,
		logger:    logger,
		client:    client,
		interval:  interval,
	}
	coord.allow.Store(true)
	return coord, nil
}

func (c *httpCoordinator) Start(ctx context.Context) {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		c.ctx, c.cancel = context.WithCancel(ctx)
		c.wg.Add(1)
		go c.pollLoop()
	})
}

func (c *httpCoordinator) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.wg.Wait()
		if transport, ok := c.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	})
}

func (c *httpCoordinator) Healthy() bool {
	if c == nil {
		return true
	}
	return c.healthy.Load()
}

func (c *httpCoordinator) ShouldServePrimary() bool {
	if c == nil {
		return true
	}
	return c.allow.Load()
}

func (c *httpCoordinator) pollLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	c.sampleOnce()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.sampleOnce()
		}
	}
}

func (c *httpCoordinator) sampleOnce() {
	ctx := c.ctx
	if ctx == nil {
		return
	}
	var lastErr error
	for _, ep := range c.endpoints {
		healthy, allow, err := c.checkEndpoint(ctx, ep)
		if err != nil {
			lastErr = err
			continue
		}
		c.updateState(healthy, allow, nil, ep)
		return
	}
	c.updateState(false, false, lastErr, "")
}

func (c *httpCoordinator) checkEndpoint(ctx context.Context, endpoint string) (bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, false, err
	}
	c.applyHeaders(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return false, false, fmt.Errorf("coordinator endpoint %s returned %d", endpoint, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNoContent {
		return true, true, nil
	}
	var payload coordinatorStatusResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return false, false, err
	}
	healthy := true
	if payload.Healthy != nil {
		healthy = *payload.Healthy
	}
	allow := true
	if payload.AllowPrimary != nil {
		allow = *payload.AllowPrimary
	} else if payload.Role != "" {
		allow = strings.EqualFold(payload.Role, "primary") || strings.EqualFold(payload.Role, "leader")
	}
	if !healthy {
		return false, allow, fmt.Errorf("endpoint %s marked unhealthy", endpoint)
	}
	return healthy, allow, nil
}

func (c *httpCoordinator) applyHeaders(req *http.Request) {
	if c.key != "" {
		req.Header.Set("X-Coordinator-Key", c.key)
	}
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}
}

func (c *httpCoordinator) updateState(healthy, allow bool, err error, endpoint string) {
	prevHealthy := c.healthy.Load()
	prevAllow := c.allow.Load()
	c.healthy.Store(healthy)
	c.allow.Store(allow)
	if healthy != prevHealthy || allow != prevAllow {
		fields := []zap.Field{
			zap.String("coordinator", c.name),
			zap.String("backend", c.backend),
			zap.Bool("healthy", healthy),
			zap.Bool("allowPrimary", allow),
		}
		if endpoint != "" {
			fields = append(fields, zap.String("endpoint", endpoint))
		}
		c.logger.Info("coordinator state updated", fields...)
		return
	}
	if err != nil && !healthy {
		c.logger.Warn("coordinator still unhealthy", zap.String("coordinator", c.name), zap.Error(err))
	}
}

func buildCoordinator(cfg config.HAConfig, logger *zap.Logger) Coordinator {
	definitions := make([]config.NamedCoordinator, 0, len(cfg.Coordinators)+1)
	if hasLegacyCoordinator(cfg.Coordinator) {
		definitions = append(definitions, config.NamedCoordinator{
			Name:              "primary",
			CoordinatorConfig: cfg.Coordinator,
		})
	}
	definitions = append(definitions, cfg.Coordinators...)
	if len(definitions) == 0 {
		return defaultCoordinator()
	}
	children := make([]Coordinator, 0, len(definitions))
	for _, def := range definitions {
		coord := buildCoordinatorDriver(def, logger)
		if coord == nil {
			continue
		}
		children = append(children, coord)
	}
	return newMultiCoordinator(children, logger)
}

func hasLegacyCoordinator(cfg config.CoordinatorConfig) bool {
	if cfg.Backend != "" {
		return true
	}
	return len(cfg.Endpoints) > 0
}

func buildCoordinatorDriver(def config.NamedCoordinator, logger *zap.Logger) Coordinator {
	if logger == nil {
		logger = zap.NewNop()
	}
	backend := strings.ToLower(strings.TrimSpace(def.Backend))
	switch backend {
	case "", "disabled":
		return nil
	case "noop":
		return defaultCoordinator()
	default:
		coord, err := newHTTPCoordinator(def, logger)
		if err != nil {
			logger.Warn("failed to initialize coordinator backend", zap.String("backend", backend), zap.Error(err))
			return defaultCoordinator()
		}
		return coord
	}
}

func buildTLSClientConfig(cfg config.TLSConfig) (*tls.Config, error) {
	if cfg.CAFile == "" && cfg.CertFile == "" && cfg.KeyFile == "" {
		return nil, nil
	}
	tlsCfg := &tls.Config{}
	if cfg.CAFile != "" {
		caBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("failed to append CA certs from %s", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	} else if cfg.CertFile != "" || cfg.KeyFile != "" {
		return nil, fmt.Errorf("both certFile and keyFile must be provided for mutual TLS")
	}
	return tlsCfg, nil
}

var _ Coordinator = (*httpCoordinator)(nil)

func windowsMask(ip net.IP) (string, error) {
	if ip.To4() == nil {
		return "", fmt.Errorf("ipv6 VIPs not supported on windows")
	}
	return "255.255.255.255", nil
}

// kafkaBroadcast pushes HA state transitions to Kafka.
type kafkaBroadcast struct {
	logger  *zap.Logger
	writer  *kafka.Writer
	started atomic.Bool
	mu      sync.Mutex
}

func newKafkaBroadcast(logger *zap.Logger) *kafkaBroadcast {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &kafkaBroadcast{logger: logger}
}

func (k *kafkaBroadcast) Start(ctx context.Context, cfg config.BroadcastConfig) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if strings.ToLower(cfg.Backend) != "kafka" {
		return
	}
	if len(cfg.Endpoints) == 0 || cfg.Topic == "" {
		k.logger.Warn("kafka broadcast missing endpoints or topic")
		return
	}
	k.writer = &kafka.Writer{
		Addr:         kafka.TCP(cfg.Endpoints...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		BatchTimeout: 10 * time.Millisecond,
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	k.started.Store(true)
}

func (k *kafkaBroadcast) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.writer != nil {
		_ = k.writer.Close()
		k.writer = nil
	}
	k.started.Store(false)
}

func (k *kafkaBroadcast) Publish(role Role, state State) {
	if !k.started.Load() || k.writer == nil {
		return
	}
	payload := map[string]any{
		"role":      role,
		"state":     state,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		k.logger.Warn("failed to marshal broadcast payload", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := k.writer.WriteMessages(ctx, kafka.Message{Value: data}); err != nil {
		k.logger.Warn("failed to publish HA state", zap.Error(err))
	}
}

// httpDiscoveryRegistry sends registration heartbeats to HTTP endpoints.
type httpDiscoveryRegistry struct {
	logger    *zap.Logger
	client    *http.Client
	endpoints []string
	cfg       config.DiscoveryConfig
	meta      config.HANodeMetadata
	ctx       context.Context
	cancel    context.CancelFunc
	trigger   chan struct{}
	healthy   atomic.Bool
	mu        sync.RWMutex
	lastRole  Role
}

func newHTTPDiscoveryRegistry(logger *zap.Logger) *httpDiscoveryRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &httpDiscoveryRegistry{
		logger: logger,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (h *httpDiscoveryRegistry) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.DiscoveryConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
	}
	if cfg.RegisterInterval <= 0 {
		cfg.RegisterInterval = maxDuration(cfg.TTL/2, 5*time.Second)
	}
	h.meta = meta
	h.cfg = cfg
	h.endpoints = append([]string(nil), cfg.Endpoints...)
	h.ctx, h.cancel = context.WithCancel(ctx)
	h.trigger = make(chan struct{}, 1)
	h.lastRole = RoleUnknown
	if len(h.endpoints) == 0 {
		h.logger.Warn("discovery registry missing endpoints")
		h.healthy.Store(false)
		return
	}
	go h.loop()
}

func (h *httpDiscoveryRegistry) Stop() {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		h.ctx = nil
	}
	h.mu.Unlock()
}

func (h *httpDiscoveryRegistry) Healthy() bool {
	return h.healthy.Load()
}

func (h *httpDiscoveryRegistry) Refresh(role Role) {
	h.mu.Lock()
	h.lastRole = role
	trigger := h.trigger
	h.mu.Unlock()
	if trigger == nil {
		return
	}
	select {
	case trigger <- struct{}{}:
	default:
	}
}

func (h *httpDiscoveryRegistry) loop() {
	cfg := h.cfg
	interval := cfg.RegisterInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	h.pushRegistration()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.pushRegistration()
		case <-h.trigger:
			h.pushRegistration()
		}
	}
}

func (h *httpDiscoveryRegistry) pushRegistration() {
	h.mu.RLock()
	ctx := h.ctx
	endpoints := append([]string(nil), h.endpoints...)
	cfg := h.cfg
	meta := h.meta
	role := h.lastRole
	h.mu.RUnlock()
	if ctx == nil || len(endpoints) == 0 {
		return
	}
	payload := map[string]any{
		"id":         meta.ID,
		"region":     meta.Region,
		"zone":       meta.Zone,
		"weight":     meta.Weight,
		"role":       role,
		"ttlSeconds": int(cfg.TTL / time.Second),
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Warn("failed to marshal discovery payload", zap.Error(err))
		return
	}
	var success bool
	for _, ep := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/nodes/%s", strings.TrimRight(ep, "/"), meta.ID), bytes.NewReader(data))
		if err != nil {
			h.logger.Warn("failed to build discovery request", zap.String("endpoint", ep), zap.Error(err))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.client.Do(req)
		if err != nil {
			h.logger.Warn("discovery heartbeat failed", zap.String("endpoint", ep), zap.Error(err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			success = true
		} else {
			h.logger.Warn("discovery heartbeat rejected", zap.Int("status", resp.StatusCode))
		}
	}
	h.healthy.Store(success)
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func buildIngressController(cfg config.HAConfig, logger *zap.Logger) IngressController {
	mode := strings.ToLower(strings.TrimSpace(cfg.Ingress.Mode))
	switch mode {
	case "keepalived", "vip":
		return newKeepalivedIngress(logger)
	case "", "disabled":
		return noopIngress{}
	default:
		if logger != nil {
			logger.Warn("unsupported ingress backend; falling back to noop", zap.String("mode", mode))
		}
		return noopIngress{}
	}
}

func buildBroadcastBus(cfg config.HAConfig, logger *zap.Logger) BroadcastBus {
	backend := strings.ToLower(strings.TrimSpace(cfg.Broadcast.Backend))
	switch backend {
	case "kafka":
		return newKafkaBroadcast(logger)
	case "", "disabled":
		return noopBroadcast{}
	default:
		if logger != nil {
			logger.Warn("unsupported broadcast backend; using noop", zap.String("backend", backend))
		}
		return noopBroadcast{}
	}
}

func buildDiscoveryRegistry(cfg config.HAConfig, logger *zap.Logger) DiscoveryRegistry {
	backend := strings.ToLower(strings.TrimSpace(cfg.Discovery.Backend))
	switch backend {
	case "http", "rest", "consul":
		return newHTTPDiscoveryRegistry(logger)
	case "", "disabled":
		return noopDiscovery{}
	default:
		if logger != nil {
			logger.Warn("unsupported discovery backend; using noop", zap.String("backend", backend))
		}
		return noopDiscovery{}
	}
}
