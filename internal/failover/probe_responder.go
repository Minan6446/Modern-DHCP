package failover

import (
	"context"
	"net"
	"sync"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// ProbeResponder accepts UDP or HTTP probe requests and returns acknowledgements.
type ProbeResponder interface {
	Start(ctx context.Context, cfg config.ProbeConfig)
	Stop()
}

type udpProbeResponder struct {
	logger *zap.Logger
	conn   *net.UDPConn
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func buildProbeResponder(cfg config.HAConfig, logger *zap.Logger) ProbeResponder {
	backend := cfg.Probe.Backend
	if backend == "" || backend == "udp" || backend == "serf" {
		if logger == nil {
			logger = zap.NewNop()
		}
		return &udpProbeResponder{logger: logger}
	}
	return noopResponder{}
}

func (r *udpProbeResponder) Start(ctx context.Context, cfg config.ProbeConfig) {
	if r == nil {
		return
	}
	if cfg.Port == 0 {
		if r.logger != nil {
			r.logger.Warn("probe responder disabled: no port configured")
		}
		return
	}
	addr := &net.UDPAddr{Port: cfg.Port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("probe responder failed to listen", zap.Error(err))
		}
		return
	}
	r.conn = conn
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		buf := make([]byte, 512)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if r.logger != nil {
					r.logger.Debug("probe responder read error", zap.Error(err))
				}
				continue
			}
			if n > 0 {
				if _, err := conn.WriteToUDP(buf[:n], remote); err != nil && r.logger != nil {
					r.logger.Debug("probe responder failed to reply", zap.Error(err))
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
}

func (r *udpProbeResponder) Stop() {
	if r == nil {
		return
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
	r.wg.Wait()
}

type noopResponder struct{}

func (noopResponder) Start(ctx context.Context, cfg config.ProbeConfig) {}
func (noopResponder) Stop()                                             {}
