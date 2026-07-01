package lease

import (
	"strings"
	"time"

	"go.uber.org/zap"
)

func (s *Service) checkIsolation(tenantID, identifier string) error {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return nil
	}
	s.isolationMu.Lock()
	defer s.isolationMu.Unlock()
	entry, ok := s.isolation[s.isolationKey(tenantID, identifier)]
	if !ok {
		return nil
	}
	if !entry.Permanent && !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt) {
		delete(s.isolation, s.isolationKey(tenantID, identifier))
		return nil
	}
	return ErrClientIsolated
}

func (s *Service) applyIsolationPolicy(tenantID, identifier, reason string) {
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if identifier == "" {
		return
	}
	isoCfg := s.exhaustion.Isolation
	if isoCfg.TemporaryDuration <= 0 && isoCfg.PermanentAfter <= 0 {
		return
	}
	key := s.isolationKey(tenantID, identifier)
	s.isolationMu.Lock()
	entry := s.isolation[key]
	if entry == nil {
		entry = &isolationEntry{}
		s.isolation[key] = entry
	}
	entry.Count++
	entry.Reason = reason
	permanent := isoCfg.PermanentAfter > 0 && entry.Count >= isoCfg.PermanentAfter
	if permanent {
		entry.Permanent = true
		entry.ExpiresAt = time.Time{}
	} else {
		dur := isoCfg.TemporaryDuration
		if dur <= 0 {
			dur = 5 * time.Minute
		}
		entry.ExpiresAt = time.Now().UTC().Add(dur)
		entry.Permanent = false
	}
	s.isolationMu.Unlock()
	if s.logger != nil {
		fields := []zap.Field{
			zap.String("tenantId", tenantID),
			zap.String("identifier", identifier),
			zap.String("reason", reason),
			zap.Bool("permanent", permanent),
			zap.Int("violations", entry.Count),
		}
		if !permanent {
			fields = append(fields, zap.Time("expiresAt", entry.ExpiresAt))
		}
		s.logger.Warn("client isolated due to exhaustion policy", fields...)
	}
	s.recordLeaseMetric("client_isolated")
}

func (s *Service) isolationKey(tenantID, identifier string) string {
	return tenantID + "|" + identifier
}
