package quarantine

import (
	"context"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/security/detector"
	"modern-dhcp/internal/security/guard"
	"modern-dhcp/pkg/models"
)

// LeaseSink persists security posture back into the lease service.
type LeaseSink struct {
	svc    *lease.Service
	logger *zap.Logger
}

// NewLeaseSink builds a sink backed by the lease service.
func NewLeaseSink(svc *lease.Service, logger *zap.Logger) *LeaseSink {
	return &LeaseSink{svc: svc, logger: logger}
}

// Apply updates the security state for the matching client.
func (l *LeaseSink) Apply(ctx context.Context, signal guard.QuarantineSignal) {
	if l == nil || l.svc == nil {
		return
	}
	identifier := firstNonEmpty(signal.MAC, signal.ClientID)
	if identifier == "" || signal.TenantID == "" {
		return
	}
	scopeRef := lease.NewResourceScope(signal.TenantID, signal.TenantID)
	state := mapState(signal.State)
	if err := l.svc.UpdateSecurityState(ctx, scopeRef, identifier, state); err != nil {
		if l.logger != nil {
			l.logger.Warn("update security state failed", zap.Error(err), zap.String("tenantId", signal.TenantID), zap.String("identifier", identifier))
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func mapState(state detector.SecurityState) string {
	switch state {
	case detector.SecurityStateBlocked:
		return models.SecurityStateBlocked
	case detector.SecurityStateSuspect:
		return models.SecurityStateSuspect
	default:
		return models.SecurityStateOK
	}
}
