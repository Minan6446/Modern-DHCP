package server

import (
	"context"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type contextKey string

const (
	contextKeyRequestID     contextKey = "requestId"
	contextKeyCorrelationID contextKey = "auditCorrelationId"
	contextKeyTenantID      contextKey = "tenantId"
	contextKeyUserID        contextKey = "userId"
	contextKeyClientIP      contextKey = "clientIp"
	contextKeyUserAgent     contextKey = "userAgent"
	contextKeyEnvironment   contextKey = "environment"
	contextKeyLogger        contextKey = "logger"
)

func setRequestContextValue(c echo.Context, key contextKey, value string) {
	if c == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	req := c.Request()
	if req == nil {
		return
	}
	ctx := context.WithValue(req.Context(), key, value)
	c.SetRequest(req.WithContext(ctx))
}

func requestContextValue(ctx context.Context, key contextKey) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(key).(string); ok {
		return strings.TrimSpace(val)
	}
	return ""
}

// RequestIDFromContext exposes the canonical request correlation identifier.
func RequestIDFromContext(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyRequestID)
}

// GetUserID returns the authenticated principal identifier stored on the context.
func GetUserID(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyUserID)
}

// GetTenantID returns the effective tenant identifier resolved for the request.
func GetTenantID(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyTenantID)
}

// ClientIPFromContext returns the normalized client IP captured from the HTTP request.
func ClientIPFromContext(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyClientIP)
}

// UserAgentFromContext returns the user-agent captured from the HTTP request.
func UserAgentFromContext(ctx context.Context) string {
	return requestContextValue(ctx, contextKeyUserAgent)
}

// EnvironmentFromContext returns the normalized environment label (dev/staging/prod) attached to the request.
func EnvironmentFromContext(ctx context.Context) string {
	return strings.ToLower(requestContextValue(ctx, contextKeyEnvironment))
}

// LoggerFromContext returns a zap logger stored on the context; falls back to the provided logger.
func LoggerFromContext(ctx context.Context, fallback *zap.Logger) *zap.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(contextKeyLogger).(*zap.Logger); ok && l != nil {
			return l
		}
	}
	return fallback
}

func shouldExposeDebugDetails(ctx context.Context) bool {
	switch EnvironmentFromContext(ctx) {
	case "dev", "development", "local", "test", "testing", "staging":
		return true
	default:
		return false
	}
}
