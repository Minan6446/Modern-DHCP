package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

func TestClusterFailoverPlanFailureTriggersRollback(t *testing.T) {
	s := &HTTPServer{
		logger: zap.NewNop(),
		haConfig: config.HAConfig{
			Mode: "active-passive",
			Node: config.HANodeMetadata{ID: "node-a"},
		},
		clusterNodes: map[string]clusterOverviewNode{
			"node-a": {ID: "node-a", Address: "10.0.0.1", Role: "active", Health: "healthy"},
			"node-b": {ID: "node-b", Address: "10.0.0.2", Role: "standby", Health: "healthy"},
		},
		failoverPlans: make(map[string]clusterFailoverPlanDTO),
	}

	e := echo.New()
	body := []byte(`{"reason":"test","simulateFailureStep":"promote_storage"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/failover/plans", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	if err := s.handleClusterFailoverPlanCreate()(ctx); err != nil {
		t.Fatalf("create failover plan returned error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected %d got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	var created clusterFailoverPlanDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if created.PlanID == "" {
		t.Fatalf("expected non-empty plan id")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		plan, ok := s.getFailoverPlan(created.PlanID)
		if !ok {
			t.Fatalf("plan not found after creation")
		}
		if plan.Status != failoverPlanStatusPending && plan.Status != failoverPlanStatusRunning {
			if plan.Status == failoverPlanStatusFailed && !plan.RollbackTriggered {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			if plan.Status != failoverPlanStatusRolledBack {
				t.Fatalf("expected rolled back status, got %s", plan.Status)
			}
			if plan.FailedStep != "promote_storage" {
				t.Fatalf("expected failed step promote_storage, got %s", plan.FailedStep)
			}
			if !plan.RollbackTriggered {
				t.Fatalf("expected rollbackTriggered=true")
			}
			if plan.RollbackStatus == "" {
				t.Fatalf("expected rollback status to be visible")
			}
			failedVisible := false
			for _, step := range plan.Steps {
				if step.ID == "promote_storage" && step.Status == failoverPlanStepFailed {
					failedVisible = true
					break
				}
			}
			if !failedVisible {
				t.Fatalf("expected failed step detail to be visible in steps")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for plan execution")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
