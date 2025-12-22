package main

import (
	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/monitoring"
)

func setupAlerting(cfg config.AlertingConfig, agg *monitoring.Aggregator, rateTracker monitoring.RateLimitTracker, observer monitoring.AlertObserver, baseLogger *zap.Logger) (*monitoring.AlertController, *alerting.Manager) {
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
	for _, email := range cfg.Email {
		if email.Name == "" || len(email.Recipients) == 0 {
			continue
		}
		manager.RegisterNotifier(email.Name, alerting.NewEmailNotifier(email.Name, email.From, email.Recipients, logger))
	}
	for _, sms := range cfg.SMS {
		if sms.Name == "" || len(sms.Numbers) == 0 {
			continue
		}
		manager.RegisterNotifier(sms.Name, alerting.NewSMSNotifier(sms.Name, sms.Numbers, logger))
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
		manager.RegisterNotifier(webhook.Name, alerting.NewWebhookNotifier(webhook.Name, webhook.URL, webhook.Secret, nil, logger))
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
