package main

import (
	"sort"
	"strings"

	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/monitoring"
)

func setupAlerting(cfg config.AlertingConfig, agg *monitoring.Aggregator, rateTracker monitoring.RateLimitTracker, observer monitoring.AlertObserver, configStore monitoring.RuntimeAlertConfigStore, baseLogger *zap.Logger) (*monitoring.AlertController, *alerting.Manager) {
	if !cfg.Enabled || agg == nil {
		return nil, nil
	}
	var logger *zap.Logger
	if baseLogger != nil {
		logger = baseLogger.Named("alerting")
	} else {
		logger = zap.NewNop()
	}
	manager := alerting.NewManager(alerting.ManagerOptions{
		Logger:           logger,
		DedupeWindow:     cfg.DedupeWindow,
		EscalationWindow: cfg.EscalationWindow,
	})
	registerAlertNotifiers(manager, cfg.Notifiers, logger)
	if len(cfg.Routes) > 0 {
		manager.ConfigureRoutes(alerting.BuildRoutes(convertRoutes(cfg.Routes)))
	}
	controller := monitoring.NewAlertController(monitoring.AlertControllerOptions{
		Config:      cfg,
		Aggregator:  agg,
		RateTracker: rateTracker,
		ConfigStore: configStore,
		Manager:     manager,
		Observer:    observer,
		Logger:      logger,
	})
	return controller, manager
}

func registerAlertNotifiers(manager *alerting.Manager, cfg config.AlertNotifierConfig, logger *zap.Logger) {
	if manager == nil {
		return
	}
	var firstEmail alerting.Notifier
	var firstSMS alerting.Notifier
	var firstWebhook alerting.Notifier
	for _, email := range cfg.Email {
		if email.Name == "" || len(email.Recipients) == 0 {
			continue
		}
		notifier := alerting.NewEmailNotifier(email.Name, email.From, email.Recipients, logger)
		manager.RegisterNotifier(email.Name, notifier)
		if firstEmail == nil {
			firstEmail = notifier
		}
	}
	for _, sms := range cfg.SMS {
		if sms.Name == "" || len(sms.Numbers) == 0 {
			continue
		}
		notifier := alerting.NewSMSNotifier(sms.Name, sms.Numbers, logger)
		manager.RegisterNotifier(sms.Name, notifier)
		if firstSMS == nil {
			firstSMS = notifier
		}
	}
	for _, voice := range cfg.Voice {
		if voice.Name == "" || len(voice.Numbers) == 0 {
			continue
		}
		manager.RegisterNotifier(voice.Name, alerting.NewVoiceNotifier(voice.Name, voice.Numbers, logger))
	}
	for _, chat := range cfg.Chat {
		if chat.Name == "" || chat.Webhook == "" {
			continue
		}
		manager.RegisterNotifier(chat.Name, alerting.NewChatNotifier(chat.Name, chat.Channel, chat.Webhook, nil, logger))
	}
	for _, webhook := range cfg.Webhook {
		if webhook.Name == "" || webhook.URL == "" {
			continue
		}
		notifier := alerting.NewWebhookNotifier(webhook.Name, webhook.URL, webhook.Secret, nil, logger)
		manager.RegisterNotifier(webhook.Name, notifier)
		if firstWebhook == nil {
			firstWebhook = notifier
		}
	}
	for _, snmp := range cfg.SNMP {
		if snmp.Name == "" || snmp.Target == "" {
			continue
		}
		manager.RegisterNotifier(snmp.Name, alerting.NewSNMPNotifier(snmp.Name, snmp.Target, snmp.Community, logger))
	}
	for _, syslog := range cfg.Syslog {
		if syslog.Name == "" || syslog.Address == "" {
			continue
		}
		manager.RegisterNotifier(syslog.Name, alerting.NewSyslogNotifier(syslog.Name, syslog.Address, syslog.Network, logger))
	}
	if firstEmail != nil {
		manager.RegisterNotifier("email", firstEmail)
	}
	if firstSMS != nil {
		manager.RegisterNotifier("sms", firstSMS)
	}
	if firstWebhook != nil {
		manager.RegisterNotifier("webhook", firstWebhook)
	}
}

func notifierChannels(cfg config.AlertNotifierConfig) []string {
	set := make(map[string]struct{})
	add := func(name string) {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			set[name] = struct{}{}
		}
	}
	for _, email := range cfg.Email {
		add(email.Name)
	}
	for _, sms := range cfg.SMS {
		add(sms.Name)
	}
	for _, voice := range cfg.Voice {
		add(voice.Name)
	}
	for _, chat := range cfg.Chat {
		add(chat.Name)
	}
	for _, webhook := range cfg.Webhook {
		add(webhook.Name)
	}
	for _, snmp := range cfg.SNMP {
		add(snmp.Name)
	}
	for _, syslog := range cfg.Syslog {
		add(syslog.Name)
	}
	channels := make([]string, 0, len(set))
	for name := range set {
		channels = append(channels, name)
	}
	sort.Strings(channels)
	return channels
}

func defaultRoutesFromChannels(channels []string) []config.AlertRouteConfig {
	if len(channels) == 0 {
		return nil
	}
	return []config.AlertRouteConfig{
		{
			Name:       "default",
			Severities: []string{string(alerting.SeverityCritical), string(alerting.SeverityMajor), string(alerting.SeverityWarning), string(alerting.SeverityInfo)},
			Channels:   channels,
		},
	}
}

func convertRoutes(routes []config.AlertRouteConfig) []alerting.RouteConfig {
	converted := make([]alerting.RouteConfig, 0, len(routes))
	for _, rt := range routes {
		converted = append(converted, alerting.RouteConfig{
			Name:       rt.Name,
			Severities: append([]string(nil), rt.Severities...),
			Channels:   append([]string(nil), rt.Channels...),
		})
	}
	return converted
}

func convertRouteRules(routes []config.AlertRouteConfig) []alerting.RoutingRule {
	converted := make([]alerting.RoutingRule, 0, len(routes))
	for _, rt := range routes {
		converted = append(converted, alerting.RoutingRule{
			Name:       rt.Name,
			Severities: append([]string(nil), rt.Severities...),
			Channels:   append([]string(nil), rt.Channels...),
			Enabled:    true,
		})
	}
	return converted
}

func routingRulesToConfigs(rules []alerting.RoutingRule) []alerting.RouteConfig {
	converted := make([]alerting.RouteConfig, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, alerting.RouteConfig{
			Name:       rule.Name,
			Severities: append([]string(nil), rule.Severities...),
			Channels:   append([]string(nil), rule.Channels...),
		})
	}
	return converted
}
