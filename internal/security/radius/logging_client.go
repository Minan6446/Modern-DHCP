package radius

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ServerEndpoint describes a RADIUS endpoint.
type ServerEndpoint struct {
	Address      string
	SharedSecret string
	Timeout      time.Duration
}

// ClientOptions configures the logging client.
type ClientOptions struct {
	Servers []ServerEndpoint
	Logger  *zap.Logger
}

type loggingClient struct {
	servers []ServerEndpoint
	logger  *zap.Logger
}

// NewLoggingClient returns a client that simply logs lookup/accounting requests.
func NewLoggingClient(opts ClientOptions) Client {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &loggingClient{servers: opts.Servers, logger: logger}
}

func (c *loggingClient) Lookup(ctx context.Context, req LookupRequest) (*Attributes, error) {
	if len(c.servers) == 0 {
		return nil, nil
	}
	c.logger.Debug("radius lookup", zap.String("tenantId", req.TenantID), zap.String("mac", req.MAC), zap.String("clientId", req.ClientID))
	return nil, nil
}

func (c *loggingClient) Accounting(ctx context.Context, report AccountingReport) error {
	if len(c.servers) == 0 {
		return nil
	}
	c.logger.Info("radius accounting", zap.String("tenantId", report.TenantID), zap.String("mac", report.MACAddress), zap.String("ip", report.IPAddress), zap.String("action", report.Action))
	return nil
}

// LoggingCoAServer exposes a stub Change-of-Authorization listener.
type LoggingCoAServer struct {
	ListenAddr string
	Logger     *zap.Logger
}

// NewLoggingCoAServer builds a stub that only logs start/stop.
func NewLoggingCoAServer(addr string, logger *zap.Logger) CoAServer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LoggingCoAServer{ListenAddr: addr, Logger: logger}
}

// Listen logs that a CoA listener would start; returns immediately for now.
func (s *LoggingCoAServer) Listen(ctx context.Context) error {
	if addr := strings.TrimSpace(s.ListenAddr); addr != "" {
		s.Logger.Info("radius coa listener configured", zap.String("listen", addr))
	}
	return nil
}
