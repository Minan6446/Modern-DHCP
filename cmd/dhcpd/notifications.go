package main

import (
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/notifications"
)

func setupNotifications(cfg config.NotificationsConfig, baseLogger *zap.Logger) *notifications.Dispatcher {
	if !cfg.Enabled {
		return nil
	}
	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	logger := baseLogger
	if logger != nil {
		logger = baseLogger.Named("notifications")
	} else {
		logger = zap.NewNop()
	}
	dispatcher := notifications.NewDispatcher(timeout, logger)
	for _, channel := range cfg.Channels {
		name := strings.TrimSpace(channel.Name)
		if name == "" || strings.TrimSpace(channel.Endpoint) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(channel.Type)) {
		case "", "webhook":
			sender := notifications.NewWebhookSender(channel.Endpoint, notifications.WebhookOptions{
				Secret:  channel.Secret,
				Method:  channel.Method,
				Headers: channel.Headers,
				Timeout: timeout,
			}, logger)
			dispatcher.RegisterChannel(name, sender)
		default:
			logger.Warn("notification channel type not supported", zap.String("channel", name), zap.String("type", channel.Type))
		}
	}
	return dispatcher
}
