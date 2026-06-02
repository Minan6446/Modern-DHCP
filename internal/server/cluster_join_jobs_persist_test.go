package server

import (
	"context"
	"testing"
	"time"
)

type instantJoinExecutor struct{}

func (instantJoinExecutor) SnapshotNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	return nil
}
func (instantJoinExecutor) CatchUpNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	return nil
}
func (instantJoinExecutor) VerifyNode(ctx context.Context, jobID, nodeID, peerAddress string) error {
	return nil
}

func TestUpdateJoinJobPersistsSnapshot(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	now := time.Now().UTC()
	s := &HTTPServer{
		configStore: store,
		joinJobs: map[string]clusterJoinJobDTO{
			"job-1": {
				JobID:     "job-1",
				NodeID:    "node-b",
				Status:    joinJobStateRegistered,
				Progress:  0,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	job, ok := s.updateJoinJob("job-1", func(v *clusterJoinJobDTO) {
		v.Status = joinJobStateVerifying
		v.Progress = 85
		v.Snapshot = &clusterJoinSnapshotDTO{Status: joinSnapshotStatusImported, ConsistencyChecksum: "snap-1"}
		v.CatchUp = &clusterJoinCatchUpDTO{Status: joinCatchUpStatusReached, TargetOffset: "offset-12", CurrentOffset: "offset-12", HighWatermarkReached: true}
		v.Checkpoint = &clusterJoinCheckpointDTO{Phase: joinJobStateVerifying, Status: joinPhaseStatusRunning, Resumable: true}
		v.Verification = &clusterJoinVerificationDTO{Status: "VERIFIED", ReplicationHealthy: true}
	})
	if !ok {
		t.Fatalf("expected join job update to succeed")
	}
	if job.Status != joinJobStateVerifying {
		t.Fatalf("unexpected updated job: %+v", job)
	}

	var persisted []clusterJoinJobDTO
	ok, err := store.Load(context.Background(), clusterConfigKeyJoinJobs, &persisted)
	if err != nil {
		t.Fatalf("load persisted join jobs: %v", err)
	}
	if !ok || len(persisted) != 1 {
		t.Fatalf("expected persisted join jobs, ok=%v len=%d", ok, len(persisted))
	}
	if persisted[0].JobID != "job-1" || persisted[0].Verification == nil {
		t.Fatalf("unexpected persisted job: %+v", persisted[0])
	}
	if persisted[0].RunToken != "" {
		t.Fatalf("expected run token to be stripped during persistence")
	}
	if persisted[0].Snapshot == nil || persisted[0].CatchUp == nil || persisted[0].Checkpoint == nil {
		t.Fatalf("expected snapshot/catch-up/checkpoint to be persisted, got %+v", persisted[0])
	}
}

func TestRestoreJoinJobsResumesPendingJob(t *testing.T) {
	s := &HTTPServer{
		options: Options{JoinJobExecutor: instantJoinExecutor{}},
		clusterNodes: map[string]clusterOverviewNode{
			"node-b": {ID: "node-b", Address: "10.0.0.11", Role: "standby", Disabled: false},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "tcp"},
	}
	now := time.Now().UTC().Add(-time.Minute)
	s.restoreJoinJobs([]clusterJoinJobDTO{
		{
			JobID:     "job-pending",
			NodeID:    "node-b",
			Status:    joinJobStateSnapshotting,
			Progress:  20,
			CreatedAt: now,
			UpdatedAt: now,
			RunToken:  "run-token",
			Checkpoint: &clusterJoinCheckpointDTO{
				Phase:     joinJobStateSnapshotting,
				Status:    joinPhaseStatusRunning,
				Resumable: true,
			},
			Phases: []clusterJoinPhaseDTO{{
				Phase:     joinJobStateSnapshotting,
				Status:    joinPhaseStatusRunning,
				StartedAt: now,
			}},
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		job, ok := s.getJoinJob("job-pending")
		if ok && job.Status == joinJobStateWarmStandby {
			if job.Checkpoint == nil || job.Checkpoint.ResumeCount == 0 {
				t.Fatalf("expected resumed checkpoint metadata, got %+v", job)
			}
			return
		}
		if time.Now().After(deadline) {
			if ok {
				t.Fatalf("expected resumed job to complete, got %+v", job)
			}
			t.Fatalf("expected restored join job")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRestoreClusterConfigsLoadsJoinJobs(t *testing.T) {
	store := newInMemoryClusterConfigStore()
	seed := []clusterJoinJobDTO{{
		JobID:     "job-restore-1",
		NodeID:    "node-c",
		Status:    joinJobStateWarmStandby,
		Progress:  100,
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
		UpdatedAt: time.Now().UTC().Add(-time.Minute),
		Verification: &clusterJoinVerificationDTO{
			Status:             "VERIFIED",
			ReplicationHealthy: true,
		},
	}}
	if err := store.Save(context.Background(), clusterConfigKeyJoinJobs, seed); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &HTTPServer{configStore: store, joinJobs: make(map[string]clusterJoinJobDTO)}
	s.restoreClusterConfigs(context.Background())

	job, ok := s.getJoinJob("job-restore-1")
	if !ok {
		t.Fatalf("expected restored join job")
	}
	if job.Status != joinJobStateWarmStandby || job.Verification == nil {
		t.Fatalf("unexpected restored job: %+v", job)
	}
}
