package failover

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// ProbeResult captures the outcome of a single health probe cycle.
type ProbeResult struct {
	Healthy   bool
	CheckedAt time.Time
	Failures  int
}

// HealthProbe performs active health checks against peers or coordination agents.
type HealthProbe interface {
	Start(ctx context.Context, cfg config.ProbeConfig, cb func(ProbeResult))
	Stop()
}

func buildHealthProbe(cfg config.HAConfig, logger *zap.Logger) HealthProbe {
	backend := strings.ToLower(strings.TrimSpace(cfg.Probe.Backend))
	if backend == "" {
		backend = "udp"
	}
	switch backend {
	case "udp", "serf":
		return newUDPHealthProbe(logger)
	case "http", "consul":
		return newHTTPHealthProbe(logger)
	default:
		return noopProbe{}
	}
}

type udpHealthProbe struct {
	logger *zap.Logger
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

func newUDPHealthProbe(logger *zap.Logger) *udpHealthProbe {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &udpHealthProbe{logger: logger}
}

func (p *udpHealthProbe) Start(ctx context.Context, cfg config.ProbeConfig, cb func(ProbeResult)) {
	if cb == nil {
		return
	}
	target := resolveProbeTarget(cfg.Target, cfg.Port)
	if target == "" {
		p.logger.Warn("udp probe missing target")
		return
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	timeout := cfg.Timeout
	if timeout <= 0 || timeout >= interval {
		timeout = interval / 2
		if timeout <= 0 {
			timeout = 200 * time.Millisecond
		}
	}
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}
	payload := []byte(cfg.Payload)
	if len(payload) == 0 {
		payload = []byte("PING")
	}

	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}
	probeCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-probeCtx.Done():
				return
			case ts := <-ticker.C:
				err := pingUDP(target, payload, timeout)
				if err != nil {
					failures++
					if failures > threshold {
						failures = threshold
					}
					p.logger.Debug("udp probe failed", zap.String("target", target), zap.Error(err))
					cb(ProbeResult{Healthy: false, CheckedAt: ts, Failures: failures})
					continue
				}
				failures = 0
				cb(ProbeResult{Healthy: true, CheckedAt: ts, Failures: failures})
			}
		}
	}()
}

func (p *udpHealthProbe) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func pingUDP(target string, payload []byte, timeout time.Duration) error {
	conn, err := net.DialTimeout("udp", target, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(payload); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	buf := make([]byte, len(payload))
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	return nil
}

type httpHealthProbe struct {
	logger *zap.Logger
	client *http.Client
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

func newHTTPHealthProbe(logger *zap.Logger) *httpHealthProbe {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &httpHealthProbe{
		logger: logger,
		client: &http.Client{},
	}
}

func (p *httpHealthProbe) Start(ctx context.Context, cfg config.ProbeConfig, cb func(ProbeResult)) {
	if cb == nil {
		return
	}
	target := strings.TrimSpace(cfg.Target)
	if target == "" {
		p.logger.Warn("http probe missing target")
		return
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = interval / 2
		if timeout <= 0 {
			timeout = 200 * time.Millisecond
		}
	}
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}

	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}
	probeCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-probeCtx.Done():
				return
			case ts := <-ticker.C:
				ctx, cancel := context.WithTimeout(probeCtx, timeout)
				err := p.request(ctx, target, cfg)
				cancel()
				if err != nil {
					failures++
					if failures > threshold {
						failures = threshold
					}
					p.logger.Debug("http probe failed", zap.String("target", target), zap.Error(err))
					cb(ProbeResult{Healthy: false, CheckedAt: ts, Failures: failures})
					continue
				}
				failures = 0
				cb(ProbeResult{Healthy: true, CheckedAt: ts, Failures: failures})
			}
		}
	}()
}

func (p *httpHealthProbe) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *httpHealthProbe) request(ctx context.Context, target string, cfg config.ProbeConfig) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if cfg.ConsulToken != "" {
		req.Header.Set("X-Consul-Token", cfg.ConsulToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

type noopProbe struct{}

func (noopProbe) Start(ctx context.Context, cfg config.ProbeConfig, cb func(ProbeResult)) {}

func (noopProbe) Stop() {}

func resolveProbeTarget(host string, port int) string {
	host = strings.TrimSpace(host)
	switch {
	case host == "" && port == 0:
		return ""
	case host == "" && port != 0:
		return fmt.Sprintf("127.0.0.1:%d", port)
	case port == 0:
		return host
	default:
		if strings.Contains(host, ":") {
			return host
		}
		return fmt.Sprintf("%s:%d", host, port)
	}
}
