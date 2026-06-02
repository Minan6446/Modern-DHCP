package server

import (
	"context"
	"strings"
	"testing"
)

func TestRunJoinConsistencyCheck(t *testing.T) {
	s := &HTTPServer{
		clusterNodes: map[string]clusterOverviewNode{
			"node-a": {ID: "node-a", Address: "10.0.0.10", Role: "standby", Disabled: false},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "tcp"},
	}
	if err := s.runJoinConsistencyCheck("", "node-a"); err != nil {
		t.Fatalf("expected consistency check success, got %v", err)
	}
}

func TestRunJoinConsistencyCheckRejectsMockTransport(t *testing.T) {
	s := &HTTPServer{
		clusterNodes: map[string]clusterOverviewNode{
			"node-a": {ID: "node-a", Address: "10.0.0.10", Role: "standby", Disabled: false},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "mock"},
	}
	if err := s.runJoinConsistencyCheck("", "node-a"); err == nil {
		t.Fatalf("expected consistency check failure for mock transport")
	}
}

func TestBuildJoinJobVerificationIncludesChecksumAndLastSync(t *testing.T) {
	s := &HTTPServer{
		clusterNodes: map[string]clusterOverviewNode{
			"node-a": {ID: "node-a", Address: "10.0.0.10", Role: "standby", Disabled: false},
		},
		syncConfig: clusterSyncConfigDTO{Transport: "tcp"},
		syncTransactions: []clusterSyncTransactionDTO{
			{TxID: "tx-1", Type: "lease", TargetNode: "10.0.0.10", Phase: "committed", CommitAt: "2026-04-10T00:03:00Z"},
		},
	}

	verification := s.buildJoinJobVerification(context.Background(), "node-a", "10.0.0.10")
	if verification.LastSyncTxID != "tx-1" {
		t.Fatalf("expected last sync tx id, got %+v", verification)
	}
	if verification.ConsistencyChecksum == "" {
		t.Fatalf("expected consistency checksum, got %+v", verification)
	}
	if verification.Status != "FAILED" {
		t.Fatalf("expected failed verification without ha telemetry, got %+v", verification)
	}
	if !strings.Contains(verification.FailureReason, "fencing epoch unavailable") {
		t.Fatalf("expected fencing failure reason, got %+v", verification)
	}
}
