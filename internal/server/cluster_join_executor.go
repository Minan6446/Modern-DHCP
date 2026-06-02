package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"modern-dhcp/internal/failover"
)

type runtimeJoinJobExecutor struct {
	server             *HTTPServer
	verifyStableWindow time.Duration
	verifyPollInterval time.Duration
	verifyLagThreshold time.Duration
}

const (
	defaultJoinVerifyStableWindow = 60 * time.Second
	defaultJoinVerifyPollInterval = 1 * time.Second
	defaultJoinVerifyLagThreshold = 2 * time.Second
)

func (e *runtimeJoinJobExecutor) SnapshotNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	if err := e.ensureNodeRegistered(nodeID, peerAddress); err != nil {
		return err
	}
	if e.shouldUseSyncTransaction() {
		if _, err := e.server.runJoinSyncTransaction(ctx, "config", nodeID, peerAddress); err != nil {
			return err
		}
	}
	if err := e.waitForReplicationHealthy(ctx); err != nil {
		return err
	}
	replicationOffset := strings.TrimSpace(e.currentFailoverSnapshot().Replication.LastOffset)
	if replicationOffset == "" {
		return fmt.Errorf("join snapshot: replication offset unavailable")
	}
	return e.server.recordJoinSnapshot(jobID, nodeID, peerAddress, replicationOffset)
}

func (e *runtimeJoinJobExecutor) CatchUpNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	if err := e.ensureNodeRegistered(nodeID, peerAddress); err != nil {
		return err
	}
	baseline := e.currentFailoverSnapshot()
	targetOffset := e.server.joinJobCatchUpTarget(jobID)
	e.server.markJoinCatchUpStarted(jobID, baseline.Replication.LastOffset, targetOffset)
	if e.shouldUseSyncTransaction() {
		if _, err := e.server.runJoinSyncTransaction(ctx, "lease", nodeID, peerAddress); err != nil {
			return err
		}
	}
	if strings.TrimSpace(targetOffset) != "" {
		return e.waitForReplicationTarget(ctx, jobID, baseline, targetOffset)
	}
	if err := e.waitForReplicationAdvance(ctx, baseline); err != nil {
		return err
	}
	e.server.updateJoinCatchUpOffset(jobID, strings.TrimSpace(e.currentFailoverSnapshot().Replication.LastOffset), true)
	return nil
}

func (e *runtimeJoinJobExecutor) VerifyNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	failedVerification := func(reason string) error {
		verification := e.server.buildJoinJobVerification(ctx, nodeID, peerAddress)
		verification.Status = "FAILED"
		verification.FailureReason = joinVerificationMessage(verification.FailureReason, reason)
		e.server.setJoinJobVerification(jobID, &verification)
		return fmt.Errorf(reason)
	}
	if err := e.ensureNodeRegistered(nodeID, peerAddress); err != nil {
		return failedVerification(err.Error())
	}
	if err := e.waitForReplicationHealthy(ctx); err != nil {
		return failedVerification(err.Error())
	}
	if epoch := strings.TrimSpace(e.server.currentClusterFencingEpoch()); epoch == "" {
		return failedVerification("join verify: fencing epoch unavailable")
	}
	if err := e.server.runJoinConsistencyCheck(jobID, nodeID); err != nil {
		return failedVerification(err.Error())
	}
	if err := e.waitForVerifyStableWindow(ctx, nodeID); err != nil {
		return failedVerification(err.Error())
	}
	verification := e.server.buildJoinJobVerification(ctx, nodeID, peerAddress)
	verification.Status = "VERIFIED"
	e.server.setJoinJobVerification(jobID, &verification)
	return nil
}

func (e *runtimeJoinJobExecutor) ensureNodeRegistered(nodeID, peerAddress string) error {
	if e == nil || e.server == nil {
		return fmt.Errorf("join executor unavailable")
	}
	node, ok := e.server.customClusterNodeByID(strings.TrimSpace(nodeID))
	if !ok {
		return fmt.Errorf("join executor: node %s not found", strings.TrimSpace(nodeID))
	}
	if !strings.EqualFold(strings.TrimSpace(node.Role), "standby") {
		return fmt.Errorf("join executor: node %s is not standby", strings.TrimSpace(nodeID))
	}
	if node.Disabled {
		return fmt.Errorf("join executor: node %s is disabled", strings.TrimSpace(nodeID))
	}
	if peer := clusterPeerAddressCandidate(strings.TrimSpace(peerAddress), []string{node.Address}); peer != "" {
		cfg := e.server.currentHAConfig()
		if !strings.EqualFold(strings.TrimSpace(cfg.Partner.Address), peer) {
			return fmt.Errorf("join executor: standby peer %s not wired into ha config", peer)
		}
	}
	return nil
}

func (e *runtimeJoinJobExecutor) shouldUseSyncTransaction() bool {
	if e == nil || e.server == nil {
		return false
	}
	return e.server.currentSyncTransportMode() == "tcp" && strings.TrimSpace(e.server.clusterSyncPeerAddress()) != ""
}

func (e *runtimeJoinJobExecutor) waitForReplicationHealthy(ctx context.Context) error {
	if e == nil || e.server == nil {
		return fmt.Errorf("join executor unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !e.server.currentHAConfig().Partner.Enabled || strings.TrimSpace(e.server.currentHAConfig().Partner.Address) == "" {
		return nil
	}
	timeout := 6 * time.Second
	if interval := e.server.currentHAConfig().Replication.SnapshotInterval; interval > 0 {
		candidate := interval * 2
		if candidate > timeout {
			timeout = candidate
		}
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		snap := e.currentFailoverSnapshot()
		if snap.Replication.Healthy && strings.TrimSpace(snap.Replication.LastError) == "" && strings.TrimSpace(snap.Replication.LastOffset) != "" {
			return nil
		}
		if time.Now().After(deadline) {
			if strings.TrimSpace(snap.Replication.LastError) != "" {
				return fmt.Errorf("join replication unhealthy: %s", strings.TrimSpace(snap.Replication.LastError))
			}
			return fmt.Errorf("join replication telemetry not ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (e *runtimeJoinJobExecutor) waitForReplicationAdvance(ctx context.Context, baseline failover.StatusSnapshot) error {
	if err := e.waitForReplicationHealthy(ctx); err != nil {
		return err
	}
	if e == nil || e.server == nil {
		return fmt.Errorf("join executor unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	beforeOffset := strings.TrimSpace(baseline.Replication.LastOffset)
	beforeUpdatedAt := baseline.Replication.UpdatedAt
	timeout := 6 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		snap := e.currentFailoverSnapshot()
		if replicationOffsetAdvanced(beforeOffset, beforeUpdatedAt, snap.Replication.LastOffset, snap.Replication.UpdatedAt) {
			return nil
		}
		if time.Now().After(deadline) {
			currentOffset := strings.TrimSpace(snap.Replication.LastOffset)
			if currentOffset == "" {
				return fmt.Errorf("join catch-up: replication offset unavailable")
			}
			return fmt.Errorf("join catch-up: replication offset did not advance from %s", beforeOffset)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (e *runtimeJoinJobExecutor) waitForReplicationTarget(ctx context.Context, jobID string, baseline failover.StatusSnapshot, targetOffset string) error {
	if err := e.waitForReplicationHealthy(ctx); err != nil {
		return err
	}
	if e == nil || e.server == nil {
		return fmt.Errorf("join executor unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	targetOffset = strings.TrimSpace(targetOffset)
	if targetOffset == "" {
		return e.waitForReplicationAdvance(ctx, baseline)
	}
	timeout := 6 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		snap := e.currentFailoverSnapshot()
		currentOffset := strings.TrimSpace(snap.Replication.LastOffset)
		if currentOffset != "" {
			reached := compareReplicationOffsets(targetOffset, currentOffset) <= 0
			e.server.updateJoinCatchUpOffset(jobID, currentOffset, reached)
			if reached {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if currentOffset == "" {
				return fmt.Errorf("join catch-up: replication offset unavailable")
			}
			return fmt.Errorf("join catch-up: replication offset %s did not reach target %s", currentOffset, targetOffset)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (e *runtimeJoinJobExecutor) currentFailoverSnapshot() failover.StatusSnapshot {
	if e == nil || e.server == nil {
		return failover.StatusSnapshot{}
	}
	if svc := e.server.ensureHAService(); svc != nil {
		return svc.Snapshot()
	}
	if e.server.options.Coordinator != nil {
		return e.server.options.Coordinator.Snapshot()
	}
	return failover.StatusSnapshot{}
}

func (e *runtimeJoinJobExecutor) waitForVerifyStableWindow(ctx context.Context, nodeID string) error {
	if e == nil || e.server == nil {
		return fmt.Errorf("join executor unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	window := e.joinVerifyStableWindow()
	interval := e.joinVerifyPollInterval()
	var stableSince time.Time
	lastReason := ""

	for {
		if err := ctx.Err(); err != nil {
			if lastReason != "" {
				return fmt.Errorf(lastReason)
			}
			return err
		}
		if err := e.evaluateVerifyStability(nodeID); err != nil {
			stableSince = time.Time{}
			lastReason = err.Error()
		} else {
			now := time.Now().UTC()
			if stableSince.IsZero() {
				stableSince = now
			}
			if now.Sub(stableSince) >= window {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			if lastReason != "" {
				return fmt.Errorf(lastReason)
			}
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (e *runtimeJoinJobExecutor) evaluateVerifyStability(nodeID string) error {
	snapshot := e.currentFailoverSnapshot()
	if !clusterFailoverReady(snapshot) {
		return fmt.Errorf("join verify: failover state not stable")
	}
	if strings.TrimSpace(clusterFencingEpoch(snapshot)) == "" {
		return fmt.Errorf("join verify: fencing epoch unavailable")
	}
	if !snapshot.Replication.Healthy {
		return fmt.Errorf("join verify: replication unhealthy")
	}
	if errText := strings.TrimSpace(snapshot.Replication.LastError); errText != "" {
		return fmt.Errorf("join verify: replication error: %s", errText)
	}
	if strings.TrimSpace(snapshot.Replication.LastOffset) == "" {
		return fmt.Errorf("join verify: replication offset unavailable")
	}
	if lag := time.Duration(snapshot.Replication.ApplyLagMs) * time.Millisecond; lag > e.joinVerifyLagThreshold() {
		return fmt.Errorf("join verify: replication lag %dms exceeds threshold %dms", snapshot.Replication.ApplyLagMs, e.joinVerifyLagThreshold().Milliseconds())
	}
	nodes := e.currentVerificationNodes(snapshot)
	if pending := countClusterPendingActions(snapshot, nodes); pending > 0 {
		return fmt.Errorf("join verify: %d pending actions remain", pending)
	}
	if err := e.server.runJoinConsistencyCheck("", nodeID); err != nil {
		return err
	}
	return nil
}

func (e *runtimeJoinJobExecutor) currentVerificationNodes(snapshot failover.StatusSnapshot) []clusterOverviewNode {
	if e == nil || e.server == nil {
		return nil
	}
	ctx := context.Background()
	nodes := e.server.customClusterNodes()
	if svc := e.server.ensureHAService(); svc != nil {
		nodes = e.server.mergeClusterNodes(buildClusterNodes(svc.Nodes(ctx), snapshot, nil))
	}
	if len(nodes) == 0 {
		nodes = buildClusterNodes(nil, snapshot, nil)
	}
	return nodes
}

func (e *runtimeJoinJobExecutor) joinVerifyStableWindow() time.Duration {
	if e != nil && e.verifyStableWindow > 0 {
		return e.verifyStableWindow
	}
	return defaultJoinVerifyStableWindow
}

func (e *runtimeJoinJobExecutor) joinVerifyPollInterval() time.Duration {
	if e != nil && e.verifyPollInterval > 0 {
		return e.verifyPollInterval
	}
	return defaultJoinVerifyPollInterval
}

func (e *runtimeJoinJobExecutor) joinVerifyLagThreshold() time.Duration {
	if e != nil && e.verifyLagThreshold > 0 {
		return e.verifyLagThreshold
	}
	return defaultJoinVerifyLagThreshold
}

func replicationOffsetAdvanced(beforeOffset string, beforeUpdatedAt time.Time, afterOffset string, afterUpdatedAt time.Time) bool {
	beforeOffset = strings.TrimSpace(beforeOffset)
	afterOffset = strings.TrimSpace(afterOffset)
	if afterOffset == "" {
		return false
	}
	if beforeOffset == "" {
		return !afterUpdatedAt.IsZero()
	}
	if compareReplicationOffsets(beforeOffset, afterOffset) < 0 {
		return true
	}
	return afterUpdatedAt.After(beforeUpdatedAt)
}

func compareReplicationOffsets(beforeOffset string, afterOffset string) int {
	beforeOffset = strings.TrimSpace(beforeOffset)
	afterOffset = strings.TrimSpace(afterOffset)
	if beforeOffset == afterOffset {
		return 0
	}
	beforeStream, beforeOK := parseStreamReplicationOffset(beforeOffset)
	afterStream, afterOK := parseStreamReplicationOffset(afterOffset)
	if beforeOK && afterOK {
		if !strings.EqualFold(beforeStream.source, afterStream.source) {
			return strings.Compare(strings.ToLower(beforeStream.source), strings.ToLower(afterStream.source))
		}
		if beforeStream.partition != afterStream.partition {
			return beforeStream.partition - afterStream.partition
		}
		if beforeStream.offset < afterStream.offset {
			return -1
		}
		if beforeStream.offset > afterStream.offset {
			return 1
		}
		return strings.Compare(beforeOffset, afterOffset)
	}
	return strings.Compare(strings.ToLower(beforeOffset), strings.ToLower(afterOffset))
}

type streamReplicationOffset struct {
	source    string
	partition int
	offset    int64
}

func parseStreamReplicationOffset(value string) (streamReplicationOffset, bool) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, "|")
	if len(parts) < 3 {
		return streamReplicationOffset{}, false
	}
	source := strings.TrimSpace(parts[0])
	partitionText := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parts[1])), "p")
	offsetText := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parts[2])), "o")
	partition, partErr := strconv.Atoi(partitionText)
	offset, offsetErr := strconv.ParseInt(offsetText, 10, 64)
	if source == "" || partErr != nil || offsetErr != nil {
		return streamReplicationOffset{}, false
	}
	return streamReplicationOffset{source: source, partition: partition, offset: offset}, true
}

func (s *HTTPServer) runJoinSyncTransaction(ctx context.Context, syncType, nodeID, peerAddress string) (clusterSyncTransactionDTO, error) {
	if s == nil {
		return clusterSyncTransactionDTO{}, fmt.Errorf("join sync unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sourceNode, targetNode := s.resolveSyncEndpoints("", strings.TrimSpace(peerAddress))
	event := s.startSyncTransaction(syncType, sourceNode, targetNode, "", "new_tx", false)
	txID := strings.TrimSpace(event.TxID)
	if txID == "" {
		return clusterSyncTransactionDTO{}, fmt.Errorf("join sync %s for node %s did not produce transaction id", syncType, strings.TrimSpace(nodeID))
	}
	ackTimeout := s.currentHAConfig().Replication.AckTimeout
	if ackTimeout <= 0 {
		ackTimeout = 2 * time.Second
	}
	timeout := ackTimeout * 3
	if timeout < 4*time.Second {
		timeout = 4 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return clusterSyncTransactionDTO{}, err
			}
		}
		tx, ok := s.syncTransactionByID(txID)
		if ok {
			switch strings.ToLower(strings.TrimSpace(tx.Phase)) {
			case "committed":
				return tx, nil
			case "failed":
				message := strings.TrimSpace(tx.ErrorMessage)
				if message == "" {
					message = strings.TrimSpace(tx.ErrorCode)
				}
				if message == "" {
					message = fmt.Sprintf("join sync %s failed", syncType)
				}
				return tx, fmt.Errorf(message)
			}
		}
		if time.Now().After(deadline) {
			return clusterSyncTransactionDTO{}, fmt.Errorf("join sync %s timed out", syncType)
		}
		select {
		case <-ctx.Done():
			return clusterSyncTransactionDTO{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
