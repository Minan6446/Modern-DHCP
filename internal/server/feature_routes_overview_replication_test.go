package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/failover"
)

type overviewCoordinatorStub struct {
	snapshot failover.StatusSnapshot
}

func (o overviewCoordinatorStub) Snapshot() failover.StatusSnapshot {
	return o.snapshot
}

func TestHandleClusterOverviewIncludesReplicationTelemetry(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	s := &HTTPServer{
		logger: zap.NewNop(),
		options: Options{Coordinator: overviewCoordinatorStub{snapshot: failover.StatusSnapshot{
			Role:  failover.RolePrimary,
			State: failover.StateNormal,
			Replication: failover.ReplicationTelemetry{
				ApplyLagMs: 22,
				LastOffset: "mysql-bin.001:99",
				UpdatedAt:  ts,
				Healthy:    true,
				Mode:       "binlog",
				Source:     "mysql-primary:3306/mysql-bin.001",
				State:      "applying",
				MySQLRole:  "primary",
				RedisRole:  "leader",
			},
		}}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/overview", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.handleClusterOverview()(c); err != nil {
		t.Fatalf("cluster overview handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload clusterOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.ReplicationLagMs != 22 {
		t.Fatalf("expected replication lag from telemetry, got %d", payload.ReplicationLagMs)
	}
	if payload.ReplicationHealth != "healthy" {
		t.Fatalf("expected replication health=healthy, got %s", payload.ReplicationHealth)
	}
	if payload.ReplicationTx != "mysql-bin.001:99" {
		t.Fatalf("unexpected replication tx: %s", payload.ReplicationTx)
	}
	if payload.ReplicationMode != "binlog" || payload.ReplicationSource != "mysql-primary:3306/mysql-bin.001" || payload.ReplicationState != "applying" {
		t.Fatalf("unexpected replication semantics: %+v", payload)
	}
	if payload.ReplicationUpdatedAt != ts.Format(time.RFC3339) {
		t.Fatalf("unexpected replication updatedAt: %s", payload.ReplicationUpdatedAt)
	}
	if payload.DHCPRole != "primary" {
		t.Fatalf("expected dhcp role=primary, got %s", payload.DHCPRole)
	}
	if payload.MySQLRole != "primary" {
		t.Fatalf("expected mysql role=primary, got %s", payload.MySQLRole)
	}
	if payload.RedisRole != "primary" {
		t.Fatalf("expected redis role=primary, got %s", payload.RedisRole)
	}
	if !payload.StorageRoleAligned {
		t.Fatalf("expected storage role aligned=true")
	}
	if !payload.WriteGateOpen {
		t.Fatalf("expected write gate open on primary")
	}
}

func TestHandleClusterOverviewStorageRolesUnknownWithoutServiceReport(t *testing.T) {
	s := &HTTPServer{
		logger: zap.NewNop(),
		options: Options{Coordinator: overviewCoordinatorStub{snapshot: failover.StatusSnapshot{
			Role:  failover.RolePrimary,
			State: failover.StateNormal,
			Replication: failover.ReplicationTelemetry{
				ApplyLagMs: 22,
				Healthy:    true,
			},
		}}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/overview", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.handleClusterOverview()(c); err != nil {
		t.Fatalf("cluster overview handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload clusterOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.MySQLRole != "unknown" {
		t.Fatalf("expected mysql role=unknown without service report, got %s", payload.MySQLRole)
	}
	if payload.RedisRole != "unknown" {
		t.Fatalf("expected redis role=unknown without service report, got %s", payload.RedisRole)
	}
}

func TestClusterReplicationHealth(t *testing.T) {
	if got := clusterReplicationHealth(failover.ReplicationTelemetry{Healthy: true}); got != "healthy" {
		t.Fatalf("expected healthy, got %s", got)
	}
	if got := clusterReplicationHealth(failover.ReplicationTelemetry{Healthy: false, ApplyLagMs: 5}); got != "degraded" {
		t.Fatalf("expected degraded, got %s", got)
	}
	if got := clusterReplicationHealth(failover.ReplicationTelemetry{Healthy: false, LastError: "stream failed"}); got != "error" {
		t.Fatalf("expected error, got %s", got)
	}
	if got := clusterReplicationHealth(failover.ReplicationTelemetry{}); got != "unknown" {
		t.Fatalf("expected unknown, got %s", got)
	}
}

func TestResolveClusterDHCPRolePrefersSelfNodeWhenSnapshotUnknown(t *testing.T) {
	got := resolveClusterDHCPRole(failover.RoleUnknown, []clusterOverviewNode{{
		ID:       "10.0.11.22",
		Role:     "active",
		Address:  "10.0.11.22",
		Self:     true,
		Disabled: false,
	}})
	if got != "primary" {
		t.Fatalf("expected primary, got %s", got)
	}
}

func TestResolveClusterDHCPRoleSingleNodeFallback(t *testing.T) {
	got := resolveClusterDHCPRole(failover.RoleUnknown, []clusterOverviewNode{{
		ID:       "10.0.11.22",
		Role:     "active",
		Address:  "10.0.11.22",
		Disabled: false,
	}})
	if got != "primary" {
		t.Fatalf("expected primary, got %s", got)
	}
}

func TestResolveClusterDHCPRoleKeepsUnknownOnAmbiguousTopology(t *testing.T) {
	got := resolveClusterDHCPRole(failover.RoleUnknown, []clusterOverviewNode{
		{ID: "10.0.11.22", Role: "active", Address: "10.0.11.22", Disabled: false},
		{ID: "10.0.11.23", Role: "standby", Address: "10.0.11.23", Disabled: false},
	})
	if got != "unknown" {
		t.Fatalf("expected unknown, got %s", got)
	}
}

func TestResolveClusterDHCPHealthPrefersSelfWhenSnapshotUnknown(t *testing.T) {
	got := resolveClusterDHCPHealth(failover.StateCommInterrupted, failover.RoleUnknown, []clusterOverviewNode{{
		ID:       "10.0.11.22",
		Role:     "active",
		Health:   "healthy",
		Self:     true,
		Disabled: false,
	}})
	if got != "healthy" {
		t.Fatalf("expected healthy, got %s", got)
	}
}

func TestResolveClusterDHCPHealthSingleNodeFallback(t *testing.T) {
	got := resolveClusterDHCPHealth(failover.StateCommInterrupted, failover.RoleUnknown, []clusterOverviewNode{{
		ID:       "10.0.11.22",
		Role:     "active",
		Health:   "critical",
		Disabled: false,
	}})
	if got != "critical" {
		t.Fatalf("expected critical, got %s", got)
	}
}

func TestResolveClusterDHCPHealthKeepsSnapshotBaseOnAmbiguousTopology(t *testing.T) {
	got := resolveClusterDHCPHealth(failover.StateCommInterrupted, failover.RoleUnknown, []clusterOverviewNode{
		{ID: "10.0.11.22", Role: "active", Health: "healthy", Disabled: false},
		{ID: "10.0.11.23", Role: "standby", Health: "healthy", Disabled: false},
	})
	if got != "warning" {
		t.Fatalf("expected warning, got %s", got)
	}
}

func TestCurrentSyncStatusWithJoinIncludesMetricGrades(t *testing.T) {
	s := &HTTPServer{
		logger: zap.NewNop(),
		options: Options{Coordinator: overviewCoordinatorStub{snapshot: failover.StatusSnapshot{
			Role: failover.RolePrimary,
			Replication: failover.ReplicationTelemetry{
				ApplyLagMs: 1200,
			},
		}}},
		syncStatus: []clusterSyncStatusDTO{
			{Type: "租约同步", Latency: "25 ms", Last: "10 秒前", Health: "健康"},
		},
		syncTransactions: []clusterSyncTransactionDTO{
			{TxID: "tx-1", Type: "lease", Phase: "committed", CreatedAt: time.Now().UTC().Format(time.RFC3339), LatencyMs: 2600},
		},
	}

	items := s.currentSyncStatusWithJoin()
	if len(items) == 0 {
		t.Fatalf("expected sync status items")
	}
	got := items[0]

	if got.MySQLLagMs != 1200 || got.RedisOffsetLag != 1200 {
		t.Fatalf("unexpected storage metrics: mysql=%d redis=%d", got.MySQLLagMs, got.RedisOffsetLag)
	}
	if got.MySQLLagWarnMs <= 0 || got.MySQLLagCriticalMs <= 0 || got.RedisOffsetLagWarn <= 0 || got.RedisOffsetLagCritical <= 0 {
		t.Fatalf("expected positive storage thresholds: %+v", got)
	}
	if got.MySQLLagLevel != "critical" || got.RedisOffsetLagLevel != "critical" {
		t.Fatalf("expected critical storage levels, got mysql=%s redis=%s", got.MySQLLagLevel, got.RedisOffsetLagLevel)
	}

	if got.Rfc6853AckLatencyMs != 2600 {
		t.Fatalf("expected ack latency from latest tx, got %d", got.Rfc6853AckLatencyMs)
	}
	if got.Rfc6853AckWarnMs <= 0 || got.Rfc6853AckCriticalMs <= 0 {
		t.Fatalf("expected positive ack thresholds: warn=%d critical=%d", got.Rfc6853AckWarnMs, got.Rfc6853AckCriticalMs)
	}
	if got.Rfc6853AckLatencyLevel != "critical" {
		t.Fatalf("expected critical ack level, got %s", got.Rfc6853AckLatencyLevel)
	}
}
