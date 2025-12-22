package guard

import (
	"strings"

	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/internal/security/detector"
	"modern-dhcp/internal/security/ipsgdai"
	"modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/security/radius"
	"modern-dhcp/internal/security/ratelimit"
	"modern-dhcp/internal/security/snooping"
)

// NewFromConfig wires a guard based on security configuration.
func NewFromConfig(cfg config.SecurityConfig, collector *metrics.Collector, logger *zap.Logger, quarantine QuarantineSink, auditSvc *audit.Service, policyEval policy.Evaluator, rateObserver ratelimit.Observer, snoopObserver snooping.Observer) Guard {
	if !cfg.Enabled {
		return NewNoop()
	}

	deps := Dependencies{Logger: logger, Metrics: collector, Quarantine: quarantine, Audit: auditSvc, RateObserver: rateObserver, SnoopingObserver: snoopObserver}
	if cfg.Policy.Enabled && policyEval != nil {
		deps.Policy = policyEval
	}

	if cfg.Snooping.Enabled && len(cfg.Snooping.TrustedPorts) > 0 {
		deps.Snooping = snooping.NewStaticStore(snooping.StaticOptions{
			TrustedPorts: cfg.Snooping.TrustedPorts,
			TTL:          cfg.Snooping.CacheTTL,
			Metrics:      collector,
		})
	}

	limiter := buildLimiter(cfg)
	if limiter != nil {
		deps.Limiter = limiter
	}

	if shouldEnableDetector(cfg.Detection) {
		deps.Detector = detector.NewSimpleDetector(detector.Config{
			DeclineSpikeThreshold:     cfg.Detection.DeclineSpikeThreshold,
			StarvationWindow:          cfg.Detection.StarvationWindow,
			Option82MismatchTolerance: cfg.Detection.Option82MismatchTolerance,
			DiscoverBurstThreshold:    cfg.Detection.DiscoverBurstThreshold,
			DiscoverToRequestRatio:    cfg.Detection.DiscoverToRequestRatio,
			BlockDuration:             cfg.Detection.BlockDuration,
			RogueServerAllowlist:      cfg.Detection.RogueServerAllowlist,
			MACDriftThreshold:         cfg.Detection.MACDriftThreshold,
		})
	}

	if cfg.IPSGDAI.PublishURL != "" {
		publisher, err := ipsgdai.NewHTTPPublisher(ipsgdai.HTTPPublisherOptions{
			URL:    cfg.IPSGDAI.PublishURL,
			Secret: cfg.IPSGDAI.Secret,
			Retry:  cfg.IPSGDAI.Retry,
			Logger: logger,
		})
		if err != nil {
			logger.Warn("ipsg publisher init failed", zap.Error(err))
		} else {
			deps.Publisher = publisher
		}
	}

	if radiusClient := buildRadiusClient(cfg, logger); radiusClient != nil {
		deps.Radius = radiusClient
	}

	if acl := buildMACACL(cfg.MACACL); acl != nil {
		deps.MACACL = acl
	}

	return New(deps)
}

func buildLimiter(cfg config.SecurityConfig) ratelimit.Limiter {
	defaultProfile := convertProfile(cfg.RateLimit.Default)
	hasLimits := defaultProfile.PerMacPPS > 0 || defaultProfile.PerPortPPS > 0 || defaultProfile.PerIPPPS > 0

	overrides := make(map[string]ratelimit.Profile, len(cfg.RateLimit.Overrides))
	for tenant, profile := range cfg.RateLimit.Overrides {
		converted := convertProfile(profile)
		overrides[tenant] = converted
		if converted.PerMacPPS > 0 || converted.PerPortPPS > 0 || converted.PerIPPPS > 0 {
			hasLimits = true
		}
	}

	if !hasLimits {
		return nil
	}

	return ratelimit.NewTokenBucketLimiter(ratelimit.Options{
		Default:   defaultProfile,
		Overrides: overrides,
	})
}

func convertProfile(profile config.RateLimitProfile) ratelimit.Profile {
	return ratelimit.Profile{
		PerMacPPS:  profile.PerMacPPS,
		PerPortPPS: profile.PerPortPPS,
		PerIPPPS:   profile.PerIPPPS,
		Burst:      profile.Burst,
	}
}

func buildRadiusClient(cfg config.SecurityConfig, logger *zap.Logger) radius.Client {
	if len(cfg.Radius.Servers) == 0 {
		return nil
	}
	endpoints := make([]radius.ServerEndpoint, 0, len(cfg.Radius.Servers))
	for _, srv := range cfg.Radius.Servers {
		endpoints = append(endpoints, radius.ServerEndpoint{
			Address:      srv.Address,
			SharedSecret: srv.SharedSecret,
			Timeout:      srv.Timeout,
		})
	}
	return radius.NewLoggingClient(radius.ClientOptions{Servers: endpoints, Logger: logger})
}

func buildMACACL(cfg config.MACACLConfig) *MACACL {
	enforce := cfg.EnforceWhitelist
	if !enforce && len(cfg.Whitelist) > 0 {
		enforce = true
	}
	hasRules := enforce || len(cfg.Whitelist) > 0 || len(cfg.Blacklist) > 0 || len(cfg.Graylist) > 0
	if !hasRules {
		return nil
	}
	action := strings.ToLower(strings.TrimSpace(cfg.GraylistAction))
	switch action {
	case string(GraylistActionBlock):
		// keep block
	case "", string(GraylistActionMonitor):
		action = string(GraylistActionMonitor)
	default:
		action = string(GraylistActionMonitor)
	}
	return &MACACL{
		Whitelist:        cfg.Whitelist,
		Blacklist:        cfg.Blacklist,
		Graylist:         cfg.Graylist,
		GraylistAction:   GraylistAction(action),
		EnforceWhitelist: enforce,
	}
}

func shouldEnableDetector(cfg config.DetectionConfig) bool {
	return cfg.DeclineSpikeThreshold > 0 || cfg.DiscoverBurstThreshold > 0 || cfg.MACDriftThreshold > 0 || len(cfg.RogueServerAllowlist) > 0
}
