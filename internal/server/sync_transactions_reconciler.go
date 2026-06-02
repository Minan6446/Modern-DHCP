package server

import (
	"context"
	"fmt"
	"math"
	"time"

	"modern-dhcp/internal/alerting"

	"go.uber.org/zap"
)

const syncTransactionsReconcileInterval = 30 * time.Second
const syncTransactionsReconcileFailureAlertThreshold = 3
const syncTransactionsReconcileAlertCooldown = 5 * time.Minute
const syncTransactionsReconcileBackoffMax = 5 * time.Minute

func (s *HTTPServer) startSyncTransactionsReconciler() {
	if s == nil {
		return
	}
	s.syncTxnReconcileMu.Lock()
	defer s.syncTxnReconcileMu.Unlock()
	if s.syncTxnReconcileCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.syncTxnReconcileCancel = cancel
	s.syncTxnReconcileDone = done
	go func() {
		defer close(done)
		s.runSyncTransactionsReconciler(ctx, s.syncTransactionsReconcileInterval())
	}()
}

func (s *HTTPServer) stopSyncTransactionsReconciler() {
	if s == nil {
		return
	}
	s.syncTxnReconcileMu.Lock()
	cancel := s.syncTxnReconcileCancel
	done := s.syncTxnReconcileDone
	s.syncTxnReconcileCancel = nil
	s.syncTxnReconcileDone = nil
	s.syncTxnReconcileMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *HTTPServer) runSyncTransactionsReconciler(ctx context.Context, interval time.Duration) {
	if s == nil {
		return
	}
	if interval <= 0 {
		interval = syncTransactionsReconcileInterval
	}
	delay := interval
	failureStreak := 0
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
			err := s.persistSyncTransactionsErr(context.Background())
			s.recordSyncTransactionsReconcileResult(time.Now().UTC(), err)
			if err != nil {
				failureStreak++
			} else {
				failureStreak = 0
			}
			delay = s.nextSyncTransactionsReconcileDelayWithBase(interval, failureStreak)
		}
	}
}

func (s *HTTPServer) recordSyncTransactionsReconcileResult(at time.Time, err error) {
	if s == nil {
		return
	}
	s.syncTxnReconcileStateMu.Lock()
	s.syncTxnReconcileLastRun = at.Format(time.RFC3339)
	if err != nil {
		s.syncTxnReconcileLastErr = err.Error()
		s.syncTxnReconcileFailureTotal++
		s.syncTxnReconcileFailureStreak++
	} else {
		s.syncTxnReconcileLastErr = ""
		s.syncTxnReconcileSuccessTotal++
		s.syncTxnReconcileFailureStreak = 0
	}
	failureStreak := s.syncTxnReconcileFailureStreak
	s.syncTxnReconcileStateMu.Unlock()

	if err != nil && failureStreak == s.syncTransactionsReconcileFailureAlertThreshold() {
		if s.shouldEmitSyncTransactionsReconcileAlert(at) {
			detail := fmt.Sprintf("sync transaction snapshot reconcile连续失败%d次: %v", failureStreak, err)
			eventID := fmt.Sprintf("sync-reconcile-alert-%d", at.UnixNano())
			s.appendScaleEvent(clusterFailoverEvent{
				ID:          eventID,
				Time:        at.Format("2006-01-02 15:04"),
				Title:       "同步快照补偿告警",
				Detail:      detail,
				Status:      "failed",
				Type:        "danger",
				StatusLabel: "告警",
			})
			if s.logger != nil {
				s.logger.Error("sync transaction reconcile failure threshold reached",
					zap.Int("consecutiveFailures", failureStreak),
					zap.Error(err),
				)
			}
			event := alerting.Event{
				ID:         eventID,
				Severity:   alerting.SeverityMajor,
				Category:   "cluster.sync.reconcile",
				Summary:    "同步快照补偿连续失败",
				Details:    detail,
				OccurredAt: at,
				Labels: map[string]string{
					"failureStreak": fmt.Sprintf("%d", failureStreak),
					"threshold":     fmt.Sprintf("%d", s.syncTransactionsReconcileFailureAlertThreshold()),
				},
				Resources: []string{"cluster_sync_transactions"},
			}
			if s.alertFeed != nil {
				s.alertFeed.Record(event)
			}
			if s.alertManager != nil {
				s.alertManager.Notify(context.Background(), event)
			}
		}
	}

	if s.options.Metrics == nil || s.options.Metrics.SyncTxnReconcileRuns == nil {
		return
	}
	result := "success"
	if err != nil {
		result = "error"
	}
	s.options.Metrics.SyncTxnReconcileRuns.WithLabelValues(result).Inc()
}

func (s *HTTPServer) nextSyncTransactionsReconcileDelay(failureStreak int) time.Duration {
	return s.nextSyncTransactionsReconcileDelayWithBase(s.syncTransactionsReconcileInterval(), failureStreak)
}

func (s *HTTPServer) nextSyncTransactionsReconcileDelayWithBase(base time.Duration, failureStreak int) time.Duration {
	if failureStreak <= 0 {
		return base
	}
	maxDelay := s.syncTransactionsReconcileBackoffMax()
	exp := math.Pow(2, float64(failureStreak))
	delay := time.Duration(float64(base) * exp)
	if delay > maxDelay {
		delay = maxDelay
	}
	// Add up to 20% jitter to avoid synchronized retries across nodes.
	jitterRange := delay / 5
	if jitterRange <= 0 {
		return delay
	}
	jitter := time.Duration(time.Now().UnixNano() % int64(jitterRange+1))
	return delay + jitter
}

func (s *HTTPServer) syncTransactionsReconcileInterval() time.Duration {
	if s == nil {
		return syncTransactionsReconcileInterval
	}
	interval := s.options.HAConfig.Replication.SyncTxnReconcileInterval
	if interval <= 0 {
		return syncTransactionsReconcileInterval
	}
	return interval
}

func (s *HTTPServer) syncTransactionsReconcileFailureAlertThreshold() int {
	if s == nil {
		return syncTransactionsReconcileFailureAlertThreshold
	}
	threshold := s.options.HAConfig.Replication.SyncTxnReconcileFailureAlertThreshold
	if threshold <= 0 {
		return syncTransactionsReconcileFailureAlertThreshold
	}
	return threshold
}

func (s *HTTPServer) syncTransactionsReconcileAlertCooldown() time.Duration {
	if s == nil {
		return syncTransactionsReconcileAlertCooldown
	}
	cooldown := s.options.HAConfig.Replication.SyncTxnReconcileAlertCooldown
	if cooldown <= 0 {
		return syncTransactionsReconcileAlertCooldown
	}
	return cooldown
}

func (s *HTTPServer) syncTransactionsReconcileBackoffMax() time.Duration {
	if s == nil {
		return syncTransactionsReconcileBackoffMax
	}
	maxDelay := s.options.HAConfig.Replication.SyncTxnReconcileBackoffMax
	if maxDelay <= 0 {
		return syncTransactionsReconcileBackoffMax
	}
	if maxDelay < s.syncTransactionsReconcileInterval() {
		return s.syncTransactionsReconcileInterval()
	}
	return maxDelay
}

func (s *HTTPServer) shouldEmitSyncTransactionsReconcileAlert(at time.Time) bool {
	if s == nil {
		return false
	}
	s.syncTxnReconcileStateMu.Lock()
	defer s.syncTxnReconcileStateMu.Unlock()
	cooldown := s.syncTransactionsReconcileAlertCooldown()
	if !s.syncTxnReconcileLastAlertAt.IsZero() && at.Sub(s.syncTxnReconcileLastAlertAt) < cooldown {
		return false
	}
	s.syncTxnReconcileLastAlertAt = at
	return true
}

func (s *HTTPServer) syncTransactionsReconcileStatus() (lastRun, lastErr string) {
	if s == nil {
		return "", ""
	}
	s.syncTxnReconcileStateMu.RLock()
	defer s.syncTxnReconcileStateMu.RUnlock()
	return s.syncTxnReconcileLastRun, s.syncTxnReconcileLastErr
}

func (s *HTTPServer) syncTransactionsReconcileCounters() (successTotal, failureTotal, failureStreak int) {
	if s == nil {
		return 0, 0, 0
	}
	s.syncTxnReconcileStateMu.RLock()
	defer s.syncTxnReconcileStateMu.RUnlock()
	return s.syncTxnReconcileSuccessTotal, s.syncTxnReconcileFailureTotal, s.syncTxnReconcileFailureStreak
}
