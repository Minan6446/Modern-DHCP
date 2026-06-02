package server

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/failover"
)

type joinVerifyCoordinatorStub struct {
	mu       sync.RWMutex
	snapshot failover.StatusSnapshot
	sequence []failover.StatusSnapshot
	index    int
}

func (s *joinVerifyCoordinatorStub) Snapshot() failover.StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sequence) > 0 {
		if s.index >= len(s.sequence) {
			return s.sequence[len(s.sequence)-1]
		}
		current := s.sequence[s.index]
		if s.index < len(s.sequence)-1 {
			s.index++
		}
		return current
	}
	return s.snapshot
}

func (s *joinVerifyCoordinatorStub) setSnapshot(snapshot failover.StatusSnapshot) {
	s.mu.Lock()
	s.snapshot = snapshot
	s.sequence = nil
	s.index = 0
	s.mu.Unlock()
}

func (s *joinVerifyCoordinatorStub) setSequence(sequence ...failover.StatusSnapshot) {
	s.mu.Lock()
	s.sequence = append([]failover.StatusSnapshot(nil), sequence...)
	s.index = 0
	if len(sequence) > 0 {
		s.snapshot = sequence[len(sequence)-1]
	}
	s.mu.Unlock()
}

func TestRuntimeJoinJobExecutorVerifyNodeRequiresStableWindow(t *testing.T) {
	coordinator := &joinVerifyCoordinatorStub{}
	coordinator.setSnapshot(failover.StatusSnapshot{
		Role:         failover.RolePrimary,
		State:        failover.StateNormal,
		FencingEpoch: "epoch-1",
		Replication: failover.ReplicationTelemetry{
			Healthy:    true,
			ApplyLagMs: 250,
			Mode:       "binlog",
			Source:     "mysql-primary:3306/mysql-bin.000001",
			State:      "applying",
			LastOffset: "binlog.000001:128",
		},
	})

	s := &HTTPServer{
		options: Options{Coordinator: coordinator},
		clusterNodes: map[string]clusterOverviewNode{
			"node-b": {ID: "node-b", Address: "10.0.0.10", Role: "standby", Disabled: false, Health: "healthy"},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "tcp"},
		joinJobs: map[string]clusterJoinJobDTO{
			"job-1": {JobID: "job-1", NodeID: "node-b", Status: joinJobStateVerifying, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
	}
	s.setHAConfig(config.HAConfig{Partner: config.PartnerConfig{Enabled: true, Address: "10.0.0.10"}})

	executor := &runtimeJoinJobExecutor{
		server:             s,
		verifyStableWindow: 40 * time.Millisecond,
		verifyPollInterval: 5 * time.Millisecond,
		verifyLagThreshold: 2 * time.Second,
	}

	if err := executor.VerifyNode(context.Background(), "job-1", "node-b", "10.0.0.10"); err != nil {
		t.Fatalf("expected verify node success, got %v", err)
	}
	job, ok := s.getJoinJob("job-1")
	if !ok || job.Verification == nil {
		t.Fatalf("expected verification summary to be stored, got %+v", job)
	}
	if job.Verification.Status != "VERIFIED" {
		t.Fatalf("expected verified status, got %+v", job.Verification)
	}
	if job.Verification.ReplicationMode != "binlog" || job.Verification.ReplicationSource != "mysql-primary:3306/mysql-bin.000001" || job.Verification.ReplicationState != "applying" {
		t.Fatalf("expected verification replication semantics, got %+v", job.Verification)
	}
}

func TestRuntimeJoinJobExecutorVerifyNodeRejectsLagAboveThreshold(t *testing.T) {
	coordinator := &joinVerifyCoordinatorStub{}
	coordinator.setSnapshot(failover.StatusSnapshot{
		Role:         failover.RolePrimary,
		State:        failover.StateNormal,
		FencingEpoch: "epoch-2",
		Replication: failover.ReplicationTelemetry{
			Healthy:    true,
			ApplyLagMs: 3200,
			LastOffset: "binlog.000001:256",
		},
	})

	s := &HTTPServer{
		options: Options{Coordinator: coordinator},
		clusterNodes: map[string]clusterOverviewNode{
			"node-b": {ID: "node-b", Address: "10.0.0.10", Role: "standby", Disabled: false, Health: "healthy"},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "tcp"},
		joinJobs: map[string]clusterJoinJobDTO{
			"job-2": {JobID: "job-2", NodeID: "node-b", Status: joinJobStateVerifying, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
	}
	s.setHAConfig(config.HAConfig{Partner: config.PartnerConfig{Enabled: true, Address: "10.0.0.10"}})

	executor := &runtimeJoinJobExecutor{
		server:             s,
		verifyStableWindow: 20 * time.Millisecond,
		verifyPollInterval: 5 * time.Millisecond,
		verifyLagThreshold: 2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	err := executor.VerifyNode(ctx, "job-2", "node-b", "10.0.0.10")
	if err == nil {
		t.Fatalf("expected verify node failure for high lag")
	}
	if !strings.Contains(err.Error(), "replication lag") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected lag-related error, got %v", err)
	}
	job, ok := s.getJoinJob("job-2")
	if !ok || job.Verification == nil {
		t.Fatalf("expected failed verification summary to be stored, got %+v", job)
	}
	if job.Verification.Status != "FAILED" {
		t.Fatalf("expected failed verification status, got %+v", job.Verification)
	}
	if !strings.Contains(job.Verification.FailureReason, "replication lag") {
		t.Fatalf("expected lag failure reason, got %+v", job.Verification)
	}
}

func TestRuntimeJoinJobExecutorWaitForReplicationAdvanceRequiresOffsetAdvance(t *testing.T) {
	coordinator := &joinVerifyCoordinatorStub{}
	baselineTime := time.Now().UTC()
	baseline := failover.StatusSnapshot{
		Role:         failover.RolePrimary,
		State:        failover.StateNormal,
		FencingEpoch: "epoch-3",
		Replication: failover.ReplicationTelemetry{
			Healthy:    true,
			ApplyLagMs: 150,
			LastOffset: "binlog.000001:512",
			UpdatedAt:  baselineTime,
		},
	}
	coordinator.setSequence(
		baseline,
		failover.StatusSnapshot{
			Role:         failover.RolePrimary,
			State:        failover.StateNormal,
			FencingEpoch: "epoch-3",
			Replication: failover.ReplicationTelemetry{
				Healthy:    true,
				ApplyLagMs: 80,
				LastOffset: "binlog.000001:768",
				UpdatedAt:  baselineTime.Add(200 * time.Millisecond),
			},
		},
	)

	s := &HTTPServer{
		options: Options{Coordinator: coordinator},
		clusterNodes: map[string]clusterOverviewNode{
			"node-b": {ID: "node-b", Address: "10.0.0.10", Role: "standby", Disabled: false, Health: "healthy"},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "mock"},
	}
	s.setHAConfig(config.HAConfig{Partner: config.PartnerConfig{Enabled: true, Address: "10.0.0.10"}})

	executor := &runtimeJoinJobExecutor{server: s}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := executor.waitForReplicationAdvance(ctx, baseline); err != nil {
		t.Fatalf("expected replication advance success, got %v", err)
	}
}

func TestRuntimeJoinJobExecutorWaitForReplicationAdvanceFailsWithoutOffsetChange(t *testing.T) {
	coordinator := &joinVerifyCoordinatorStub{}
	baselineTime := time.Now().UTC()
	baseline := failover.StatusSnapshot{
		Role:         failover.RolePrimary,
		State:        failover.StateNormal,
		FencingEpoch: "epoch-4",
		Replication: failover.ReplicationTelemetry{
			Healthy:    true,
			ApplyLagMs: 100,
			LastOffset: "binlog.000001:900",
			UpdatedAt:  baselineTime,
		},
	}
	coordinator.setSnapshot(baseline)

	s := &HTTPServer{
		options: Options{Coordinator: coordinator},
		clusterNodes: map[string]clusterOverviewNode{
			"node-b": {ID: "node-b", Address: "10.0.0.10", Role: "standby", Disabled: false, Health: "healthy"},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "mock"},
	}
	s.setHAConfig(config.HAConfig{Partner: config.PartnerConfig{Enabled: true, Address: "10.0.0.10"}})

	executor := &runtimeJoinJobExecutor{server: s}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	err := executor.waitForReplicationAdvance(ctx, baseline)
	if err == nil {
		t.Fatalf("expected replication advance failure when offset does not advance")
	}
	if !strings.Contains(err.Error(), "offset did not advance") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected offset advance error, got %v", err)
	}
}

func TestRuntimeJoinJobExecutorWaitForReplicationTargetUsesHighWatermark(t *testing.T) {
	coordinator := &joinVerifyCoordinatorStub{}
	baselineTime := time.Now().UTC()
	baseline := failover.StatusSnapshot{
		Role:         failover.RolePrimary,
		State:        failover.StateNormal,
		FencingEpoch: "epoch-5",
		Replication: failover.ReplicationTelemetry{
			Healthy:    true,
			ApplyLagMs: 100,
			LastOffset: "kafka-1:9092/lease-journal|p2|o41",
			UpdatedAt:  baselineTime,
		},
	}
	coordinator.setSequence(
		baseline,
		failover.StatusSnapshot{
			Role:         failover.RolePrimary,
			State:        failover.StateNormal,
			FencingEpoch: "epoch-5",
			Replication: failover.ReplicationTelemetry{
				Healthy:    true,
				ApplyLagMs: 80,
				LastOffset: "kafka-1:9092/lease-journal|p2|o42",
				UpdatedAt:  baselineTime.Add(200 * time.Millisecond),
			},
		},
		failover.StatusSnapshot{
			Role:         failover.RolePrimary,
			State:        failover.StateNormal,
			FencingEpoch: "epoch-5",
			Replication: failover.ReplicationTelemetry{
				Healthy:    true,
				ApplyLagMs: 50,
				LastOffset: "kafka-1:9092/lease-journal|p2|o43",
				UpdatedAt:  baselineTime.Add(400 * time.Millisecond),
			},
		},
	)

	s := &HTTPServer{
		options: Options{Coordinator: coordinator},
		joinJobs: map[string]clusterJoinJobDTO{
			"job-3": {JobID: "job-3", NodeID: "node-b", CatchUp: &clusterJoinCatchUpDTO{TargetOffset: "kafka-1:9092/lease-journal|p2|o43"}},
		},
	}
	executor := &runtimeJoinJobExecutor{server: s}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := executor.waitForReplicationTarget(ctx, "job-3", baseline, "kafka-1:9092/lease-journal|p2|o43"); err != nil {
		t.Fatalf("expected replication target success, got %v", err)
	}
	job, ok := s.getJoinJob("job-3")
	if !ok || job.CatchUp == nil || !job.CatchUp.HighWatermarkReached {
		t.Fatalf("expected catch-up high watermark to be recorded, got %+v", job)
	}
}

func TestCompareReplicationOffsetsForStreamOffsets(t *testing.T) {
	if got := compareReplicationOffsets("kafka-1:9092/lease-journal|p2|o41", "kafka-1:9092/lease-journal|p2|o42"); got >= 0 {
		t.Fatalf("expected offset 42 to be newer than 41, got %d", got)
	}
	if got := compareReplicationOffsets("kafka-1:9092/lease-journal|p2|o42", "kafka-1:9092/lease-journal|p2|o41"); got <= 0 {
		t.Fatalf("expected offset 41 to be older than 42, got %d", got)
	}
	if got := compareReplicationOffsets("kafka-1:9092/lease-journal|p2|o41|lease=a", "kafka-1:9092/lease-journal|p2|o41|lease=b"); got == 0 {
		t.Fatalf("expected lexical fallback for same partition/offset with different suffix")
	}
}

func TestReplicationOffsetAdvancedUnderstandsStreamOffsets(t *testing.T) {
	baselineTime := time.Now().UTC()
	if !replicationOffsetAdvanced(
		"kafka-1:9092/lease-journal|p2|o41",
		baselineTime,
		"kafka-1:9092/lease-journal|p2|o42",
		baselineTime,
	) {
		t.Fatalf("expected stream offset advancement to be detected")
	}
	if replicationOffsetAdvanced(
		"kafka-1:9092/lease-journal|p2|o42",
		baselineTime,
		"kafka-1:9092/lease-journal|p2|o41",
		baselineTime,
	) {
		t.Fatalf("expected stream offset regression to be rejected")
	}
}
