package mdm

import (
	"context"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// buildConnectors instantiates all configured MDM connectors.
func buildConnectors(cfg config.MDMConfig, logger *zap.Logger) []Connector {
	var connectors []Connector
	if c := newIntuneConnector(cfg.Intune, logger); c != nil && c.Enabled() {
		connectors = append(connectors, c)
	}
	if c := newJamfConnector(cfg.Jamf, logger); c != nil && c.Enabled() {
		connectors = append(connectors, c)
	}
	if c := newAirWatchConnector(cfg.AirWatch, logger); c != nil && c.Enabled() {
		connectors = append(connectors, c)
	}
	return connectors
}

type intuneConnector struct {
	cfg    config.IntuneConfig
	logger *zap.Logger
}

func newIntuneConnector(cfg config.IntuneConfig, logger *zap.Logger) Connector {
	if !cfg.Enabled {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &intuneConnector{cfg: cfg, logger: logger.With(zap.String("mdm", "intune"))}
}

func (c *intuneConnector) Name() string { return "intune" }

func (c *intuneConnector) Enabled() bool { return c != nil && c.cfg.Enabled }

func (c *intuneConnector) Sync(ctx context.Context) ([]ComplianceRecord, error) {
	if !c.Enabled() {
		return nil, nil
	}
	c.logger.Debug("intune connector sync skipped (not implemented)")
	return nil, nil
}

type jamfConnector struct {
	cfg    config.JamfConfig
	logger *zap.Logger
}

func newJamfConnector(cfg config.JamfConfig, logger *zap.Logger) Connector {
	if !cfg.Enabled {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &jamfConnector{cfg: cfg, logger: logger.With(zap.String("mdm", "jamf"))}
}

func (c *jamfConnector) Name() string { return "jamf" }

func (c *jamfConnector) Enabled() bool { return c != nil && c.cfg.Enabled }

func (c *jamfConnector) Sync(ctx context.Context) ([]ComplianceRecord, error) {
	if !c.Enabled() {
		return nil, nil
	}
	c.logger.Debug("jamf connector sync skipped (not implemented)")
	return nil, nil
}

type airWatchConnector struct {
	cfg    config.AirWatchConfig
	logger *zap.Logger
}

func newAirWatchConnector(cfg config.AirWatchConfig, logger *zap.Logger) Connector {
	if !cfg.Enabled {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &airWatchConnector{cfg: cfg, logger: logger.With(zap.String("mdm", "airwatch"))}
}

func (c *airWatchConnector) Name() string { return "airwatch" }

func (c *airWatchConnector) Enabled() bool { return c != nil && c.cfg.Enabled }

func (c *airWatchConnector) Sync(ctx context.Context) ([]ComplianceRecord, error) {
	if !c.Enabled() {
		return nil, nil
	}
	c.logger.Debug("airwatch connector sync skipped (not implemented)")
	return nil, nil
}
