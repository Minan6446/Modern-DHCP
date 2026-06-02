package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/monitoring"
)

type inMemoryClusterConfigStore struct {
	mu   sync.Mutex
	data map[string][]byte
	save int
}

type failingClusterConfigStore struct{}

type captureNotifier struct {
	mu     sync.Mutex
	events []alerting.Event
}

func (n *captureNotifier) Name() string { return "capture" }

func (n *captureNotifier) Notify(_ context.Context, event alerting.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, event)
	return nil
}

func (n *captureNotifier) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.events)
}

func (failingClusterConfigStore) Save(_ context.Context, _ string, _ any) error {
	return fmt.Errorf("boom")
}

func (failingClusterConfigStore) Load(_ context.Context, _ string, _ any) (bool, error) {
	return false, nil
}

func newInMemoryClusterConfigStore() *inMemoryClusterConfigStore {
	return &inMemoryClusterConfigStore{data: map[string][]byte{}}
}

func (s *inMemoryClusterConfigStore) Save(_ context.Context, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.data[key] = payload
	s.save++
	return nil
}

func (s *inMemoryClusterConfigStore) SaveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save
}

func (s *inMemoryClusterConfigStore) Load(_ context.Context, key string, dest any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.data[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func TestRestoreClusterConfigsLoadsSyncTransactions(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	seed := []clusterSyncTransactionDTO{
		{TxID: "tx-restore-1", Type: "lease", Phase: "committed", CreatedAt: "2026-01-01T00:00:00Z"},
		{TxID: "tx-restore-2", Type: "lease", Phase: "failed", CreatedAt: "2026-01-01T00:01:00Z"},
	}
	if err := store.Save(context.Background(), clusterConfigKeySyncTransactions, seed); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &HTTPServer{configStore: store}
	s.restoreClusterConfigs(context.Background())

	got := s.recentSyncTransactions(0)
	if len(got) != len(seed) {
		t.Fatalf("unexpected transaction count: got %d want %d", len(got), len(seed))
	}
	if got[0].TxID != seed[0].TxID || got[1].TxID != seed[1].TxID {
		t.Fatalf("unexpected restored transactions: %+v", got)
	}
}

func TestAppendSyncTransactionPersistsSnapshot(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	s := &HTTPServer{configStore: store}

	tx := clusterSyncTransactionDTO{TxID: "tx-persist-1", Type: "lease", Phase: "prepare", CreatedAt: "2026-01-01T00:00:00Z"}
	s.appendSyncTransaction(tx)

	var persisted []clusterSyncTransactionDTO
	ok, err := store.Load(context.Background(), clusterConfigKeySyncTransactions, &persisted)
	if err != nil {
		t.Fatalf("load persisted snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted snapshot")
	}
	if len(persisted) != 1 || persisted[0].TxID != tx.TxID {
		t.Fatalf("unexpected persisted snapshot: %+v", persisted)
	}
}

func TestCompleteAndFailSyncTransactionPersistSnapshot(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	s := &HTTPServer{configStore: store}

	base := clusterSyncTransactionDTO{TxID: "tx-state-1", Type: "lease", Phase: "prepare", CreatedAt: "2026-01-01T00:00:00Z"}
	s.appendSyncTransaction(base)
	s.completeSyncTransactionWithLatency(base.TxID, 50)

	var persisted []clusterSyncTransactionDTO
	ok, err := store.Load(context.Background(), clusterConfigKeySyncTransactions, &persisted)
	if err != nil || !ok || len(persisted) != 1 {
		t.Fatalf("load committed snapshot failed: ok=%v err=%v len=%d", ok, err, len(persisted))
	}
	if persisted[0].Phase != "committed" {
		t.Fatalf("expected committed phase, got %+v", persisted[0])
	}

	s.failSyncTransaction(base.TxID, "ACK_TIMEOUT", "timeout")
	ok, err = store.Load(context.Background(), clusterConfigKeySyncTransactions, &persisted)
	if err != nil || !ok || len(persisted) != 1 {
		t.Fatalf("load failed snapshot failed: ok=%v err=%v len=%d", ok, err, len(persisted))
	}
	if persisted[0].Phase != "failed" || persisted[0].ErrorCode != "ACK_TIMEOUT" {
		t.Fatalf("expected failed phase and code, got %+v", persisted[0])
	}
}

func TestSyncTransactionsRestoreAfterRestartRoundTrip(t *testing.T) {
	store := newInMemoryClusterConfigStore()

	serverA := &HTTPServer{configStore: store}
	base := clusterSyncTransactionDTO{TxID: "tx-restart-1", Type: "lease", Phase: "prepare", CreatedAt: "2026-01-01T00:00:00Z"}
	serverA.appendSyncTransaction(base)
	serverA.completeSyncTransactionWithLatency(base.TxID, 20)

	serverB := &HTTPServer{configStore: store}
	serverB.restoreClusterConfigs(context.Background())

	restored := serverB.recentSyncTransactions(0)
	if len(restored) != 1 {
		t.Fatalf("unexpected restored count: %d", len(restored))
	}
	if restored[0].TxID != base.TxID || restored[0].Phase != "committed" {
		t.Fatalf("unexpected restored tx after restart: %+v", restored[0])
	}
}

func TestRestoreSyncTransactionsSanitizesInvalidAndDuplicateEntries(t *testing.T) {
	s := &HTTPServer{}
	s.restoreSyncTransactions([]clusterSyncTransactionDTO{
		{TxID: "", Phase: "prepare"},
		{TxID: "tx-dup", Phase: "prepare"},
		{TxID: " tx-dup ", Phase: "committed"},
		{TxID: "tx-ok", Phase: "failed", ErrorCode: "ACK_TIMEOUT"},
	})

	got := s.recentSyncTransactions(0)
	if len(got) != 2 {
		t.Fatalf("unexpected sanitized count: %d", len(got))
	}
	if got[0].TxID != "tx-dup" || got[1].TxID != "tx-ok" {
		t.Fatalf("unexpected sanitized tx order/content: %+v", got)
	}
}

func TestRunSyncTransactionsReconcilerPersistsSnapshot(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	s := &HTTPServer{configStore: store}
	s.restoreSyncTransactions([]clusterSyncTransactionDTO{{TxID: "tx-reconcile-1", Phase: "prepare"}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runSyncTransactionsReconciler(ctx, 10*time.Millisecond)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if store.SaveCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if store.SaveCount() == 0 {
		t.Fatalf("expected reconciler to persist snapshot")
	}
	successTotal, failureTotal, failureStreak := s.syncTransactionsReconcileCounters()
	if successTotal == 0 || failureTotal != 0 || failureStreak != 0 {
		t.Fatalf("unexpected reconcile counters on success path: success=%d failure=%d streak=%d", successTotal, failureTotal, failureStreak)
	}
	lastRun, lastErr := s.syncTransactionsReconcileStatus()
	if lastRun == "" {
		t.Fatalf("expected reconcile last run timestamp")
	}
	if lastErr != "" {
		t.Fatalf("unexpected reconcile error for success path: %s", lastErr)
	}
	var persisted []clusterSyncTransactionDTO
	ok, err := store.Load(context.Background(), clusterConfigKeySyncTransactions, &persisted)
	if err != nil || !ok || len(persisted) != 1 {
		t.Fatalf("unexpected persisted data: ok=%v err=%v len=%d", ok, err, len(persisted))
	}
	if persisted[0].TxID != "tx-reconcile-1" {
		t.Fatalf("unexpected persisted tx: %+v", persisted[0])
	}
}

func TestRunSyncTransactionsReconcilerRecordsErrorStatus(t *testing.T) {
	s := &HTTPServer{configStore: failingClusterConfigStore{}}
	s.restoreSyncTransactions([]clusterSyncTransactionDTO{{TxID: "tx-reconcile-err", Phase: "prepare"}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runSyncTransactionsReconciler(ctx, 10*time.Millisecond)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, lastErr := s.syncTransactionsReconcileStatus()
		if lastErr != "" && len(s.currentScaleEvents()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	lastRun, lastErr := s.syncTransactionsReconcileStatus()
	if lastRun == "" {
		t.Fatalf("expected reconcile last run timestamp on error path")
	}
	if lastErr == "" {
		t.Fatalf("expected reconcile error status")
	}
	successTotal, failureTotal, failureStreak := s.syncTransactionsReconcileCounters()
	if successTotal != 0 || failureTotal == 0 || failureStreak == 0 {
		t.Fatalf("unexpected reconcile counters on error path: success=%d failure=%d streak=%d", successTotal, failureTotal, failureStreak)
	}
	if len(s.currentScaleEvents()) == 0 {
		t.Fatalf("expected threshold alert scale event on repeated reconcile failures")
	}
}

func TestSyncTransactionsReconcileFailureThresholdUsesConfig(t *testing.T) {
	manager := alerting.NewManager(alerting.ManagerOptions{})
	notifier := &captureNotifier{}
	manager.RegisterNotifier("capture", notifier)
	manager.ConfigureRoutes(alerting.BuildRoutes([]alerting.RouteConfig{{
		Name:       "sync-reconcile",
		Severities: []string{string(alerting.SeverityMajor)},
		Channels:   []string{"capture"},
	}}))

	s := &HTTPServer{
		options:      Options{HAConfig: config.HAConfig{Replication: config.ReplicationConfig{SyncTxnReconcileFailureAlertThreshold: 2}}},
		alertManager: manager,
	}
	err := fmt.Errorf("boom")
	now := time.Now().UTC()

	s.recordSyncTransactionsReconcileResult(now, err)
	if len(s.currentScaleEvents()) != 0 {
		t.Fatalf("alert should not fire before configured threshold")
	}
	s.recordSyncTransactionsReconcileResult(now.Add(time.Second), err)
	if len(s.currentScaleEvents()) == 0 {
		t.Fatalf("alert should fire at configured threshold")
	}
	if notifier.Count() == 0 {
		t.Fatalf("expected alert manager notification when threshold reached")
	}
}

func TestSyncTransactionsReconcileAlertCooldownSuppressesDuplicateNotifications(t *testing.T) {
	manager := alerting.NewManager(alerting.ManagerOptions{DedupeWindow: 2 * time.Millisecond})
	notifier := &captureNotifier{}
	manager.RegisterNotifier("capture", notifier)
	manager.ConfigureRoutes(alerting.BuildRoutes([]alerting.RouteConfig{{
		Name:       "sync-reconcile",
		Severities: []string{string(alerting.SeverityMajor)},
		Channels:   []string{"capture"},
	}}))

	s := &HTTPServer{
		options: Options{HAConfig: config.HAConfig{Replication: config.ReplicationConfig{
			SyncTxnReconcileFailureAlertThreshold: 2,
			SyncTxnReconcileAlertCooldown:         2 * time.Minute,
		}}},
		alertManager: manager,
	}
	err := fmt.Errorf("boom")
	now := time.Now().UTC()

	// First threshold hit triggers one notification.
	s.recordSyncTransactionsReconcileResult(now, err)
	s.recordSyncTransactionsReconcileResult(now.Add(time.Second), err)
	if notifier.Count() != 1 {
		t.Fatalf("expected one notification on first threshold hit, got %d", notifier.Count())
	}

	// Reset streak by success, then hit threshold again within cooldown; notification should be suppressed.
	s.recordSyncTransactionsReconcileResult(now.Add(2*time.Second), nil)
	s.recordSyncTransactionsReconcileResult(now.Add(3*time.Second), err)
	s.recordSyncTransactionsReconcileResult(now.Add(4*time.Second), err)
	if notifier.Count() != 1 {
		t.Fatalf("expected duplicate notification suppressed in cooldown, got %d", notifier.Count())
	}
	time.Sleep(5 * time.Millisecond)

	// After cooldown expires, threshold hit should notify again.
	s.recordSyncTransactionsReconcileResult(now.Add(3*time.Minute), nil)
	s.recordSyncTransactionsReconcileResult(now.Add(3*time.Minute+time.Second), err)
	s.recordSyncTransactionsReconcileResult(now.Add(3*time.Minute+2*time.Second), err)
	if notifier.Count() != 2 {
		t.Fatalf("expected notification after cooldown expiry, got %d", notifier.Count())
	}
}

func TestSyncTransactionsReconcileThresholdRecordsAlertFeed(t *testing.T) {
	feed := monitoring.NewAlertFeed(10)
	s := &HTTPServer{
		options:   Options{HAConfig: config.HAConfig{Replication: config.ReplicationConfig{SyncTxnReconcileFailureAlertThreshold: 2}}},
		alertFeed: feed,
	}
	err := fmt.Errorf("boom")
	now := time.Now().UTC()

	s.recordSyncTransactionsReconcileResult(now, err)
	s.recordSyncTransactionsReconcileResult(now.Add(time.Second), err)

	snap := feed.Snapshot("", 10)
	if len(snap.Alerts) == 0 {
		t.Fatalf("expected alert feed entry for reconcile threshold")
	}
	if snap.Alerts[0].Category != "cluster.sync.reconcile" {
		t.Fatalf("unexpected alert feed category: %s", snap.Alerts[0].Category)
	}
}

func TestSyncTransactionsReconcileBackoffDelayIsBounded(t *testing.T) {
	s := &HTTPServer{options: Options{HAConfig: config.HAConfig{Replication: config.ReplicationConfig{
		SyncTxnReconcileInterval:   10 * time.Millisecond,
		SyncTxnReconcileBackoffMax: 80 * time.Millisecond,
	}}}}

	if got := s.nextSyncTransactionsReconcileDelay(0); got < 10*time.Millisecond {
		t.Fatalf("unexpected base delay: %v", got)
	}
	for streak := 1; streak <= 8; streak++ {
		got := s.nextSyncTransactionsReconcileDelay(streak)
		if got < 10*time.Millisecond {
			t.Fatalf("delay below base for streak %d: %v", streak, got)
		}
		if got > 96*time.Millisecond { // max + 20% jitter
			t.Fatalf("delay exceeds max+jitter for streak %d: %v", streak, got)
		}
	}
}

func TestApplySyncTransactionsPolicyRetentionAndArchive(t *testing.T) {
	now := time.Now().UTC()
	s := &HTTPServer{options: Options{HAConfig: config.HAConfig{Replication: config.ReplicationConfig{
		SyncTxnMaxEntries:   2,
		SyncTxnRetention:    time.Hour,
		SyncTxnArchiveLimit: 3,
	}}}}
	s.syncTransactions = []clusterSyncTransactionDTO{
		{TxID: "new-1", CreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339)},
		{TxID: "new-2", CreatedAt: now.Add(-20 * time.Minute).Format(time.RFC3339)},
		{TxID: "new-3", CreatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339)},
		{TxID: "old-1", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339)},
	}

	s.syncTransactionsMu.Lock()
	s.applySyncTransactionsPolicyLocked(now)
	s.syncTransactionsMu.Unlock()

	if len(s.syncTransactions) != 2 {
		t.Fatalf("expected 2 active tx, got %d", len(s.syncTransactions))
	}
	if s.syncTransactions[0].TxID != "new-1" || s.syncTransactions[1].TxID != "new-2" {
		t.Fatalf("unexpected active tx order/content: %+v", s.syncTransactions)
	}
	if len(s.syncTransactionsArchive) != 2 {
		t.Fatalf("expected 2 archived tx, got %d", len(s.syncTransactionsArchive))
	}
	if s.syncTransactionsArchive[0].TxID != "new-3" || s.syncTransactionsArchive[1].TxID != "old-1" {
		t.Fatalf("unexpected archived tx order/content: %+v", s.syncTransactionsArchive)
	}
}

func TestSyncTransactionsReconcileIntervalUsesConfig(t *testing.T) {
	s := &HTTPServer{}
	if got := s.syncTransactionsReconcileInterval(); got != syncTransactionsReconcileInterval {
		t.Fatalf("unexpected default interval: %v", got)
	}
	s.options.HAConfig.Replication.SyncTxnReconcileInterval = 7 * time.Second
	if got := s.syncTransactionsReconcileInterval(); got != 7*time.Second {
		t.Fatalf("unexpected configured interval: %v", got)
	}
}
