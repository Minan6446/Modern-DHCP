package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/failover"
)

type membershipControllerStub struct {
	nodes         map[string]failover.NodeRegistration
	upsertCalls   int
	removeCalls   int
	allowFailback bool
}

func newMembershipControllerStub() *membershipControllerStub {
	return &membershipControllerStub{nodes: make(map[string]failover.NodeRegistration)}
}

func (m *membershipControllerStub) Snapshot() failover.StatusSnapshot {
	return failover.StatusSnapshot{Role: failover.RolePrimary, State: failover.StateNormal}
}

func (m *membershipControllerStub) AllowManualFailback() {
	m.allowFailback = true
}

func (m *membershipControllerStub) TriggerFailover(context.Context, failover.ManualFailoverRequest) error {
	return nil
}

func (m *membershipControllerStub) UpdateIngressPolicy(context.Context, failover.IngressPolicy) error {
	return nil
}

func (m *membershipControllerStub) Nodes() []failover.NodeStatus {
	items := make([]failover.NodeStatus, 0, len(m.nodes))
	for _, node := range m.nodes {
		items = append(items, failover.NodeStatus{ID: node.ID, Role: node.Role, State: failover.StateNormal, Self: false})
	}
	return items
}

func (m *membershipControllerStub) UpsertNode(_ context.Context, node failover.NodeRegistration) error {
	m.upsertCalls++
	m.nodes[node.ID] = node
	return nil
}

func (m *membershipControllerStub) RemoveNode(_ context.Context, nodeID string) error {
	m.removeCalls++
	delete(m.nodes, nodeID)
	return nil
}

func TestHandleHAMemberUpsertSyncsClusterNode(t *testing.T) {
	controller := newMembershipControllerStub()
	s := &HTTPServer{
		logger:       zap.NewNop(),
		options:      Options{FailoverController: controller},
		clusterNodes: make(map[string]clusterOverviewNode),
	}

	payload := map[string]any{
		"id":      "node-a",
		"address": "10.0.0.10",
		"role":    "standby",
	}
	body, _ := json.Marshal(payload)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ha/members", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.handleHAMemberUpsert()(c); err != nil {
		t.Fatalf("upsert handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if controller.upsertCalls != 1 {
		t.Fatalf("expected 1 upsert call, got %d", controller.upsertCalls)
	}
	if _, ok := s.customClusterNodeByID("node-a"); !ok {
		t.Fatalf("expected cluster node to be synced from ha member upsert")
	}
	events := s.currentMembershipEvents()
	if len(events) == 0 {
		t.Fatalf("expected membership event after upsert")
	}
}

func TestHandleHAMemberRemoveSyncsClusterNode(t *testing.T) {
	controller := newMembershipControllerStub()
	controller.nodes["node-b"] = failover.NodeRegistration{ID: "node-b", Address: "10.0.0.11", Role: failover.RoleStandby}
	s := &HTTPServer{
		logger:       zap.NewNop(),
		options:      Options{FailoverController: controller},
		clusterNodes: map[string]clusterOverviewNode{"node-b": {ID: "node-b", Address: "10.0.0.11", Role: "standby"}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/ha/members/node-b", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("nodeId")
	c.SetParamValues("node-b")

	if err := s.handleHAMemberRemove()(c); err != nil {
		t.Fatalf("remove handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if controller.removeCalls != 1 {
		t.Fatalf("expected 1 remove call, got %d", controller.removeCalls)
	}
	if _, ok := s.customClusterNodeByID("node-b"); ok {
		t.Fatalf("expected cluster node to be removed after ha member remove")
	}
}

func TestResolveClusterNodeMutationIDAcceptsAddress(t *testing.T) {
	s := &HTTPServer{
		clusterNodes: map[string]clusterOverviewNode{
			"node-a": {ID: "node-a", Address: "10.0.0.10", Role: "active"},
			"node-b": {ID: "node-b", Address: "10.0.0.11", Role: "standby"},
		},
	}

	if got := s.resolveClusterNodeMutationID("10.0.0.11", ""); got != "node-b" {
		t.Fatalf("expected address to resolve to node-b, got %q", got)
	}

	if got := s.resolveClusterNodeMutationID("self", ""); got != "node-a" {
		t.Fatalf("expected self to resolve to active node-a, got %q", got)
	}
}

func TestCurrentMembershipEventsFiltersScaleEvents(t *testing.T) {
	s := &HTTPServer{
		scaleEvents: []clusterFailoverEvent{
			{Title: "自动扩容", ErrorCode: ""},
			{Title: "动态成员写入", ErrorCode: ""},
			{Title: "其他事件", ErrorCode: "MEMBERSHIP_REMOVE_FAILED"},
		},
	}

	items := s.currentMembershipEvents()
	if len(items) != 2 {
		t.Fatalf("expected 2 membership events, got %d", len(items))
	}
}

func TestHandleHADrillDryRunCreatesEvent(t *testing.T) {
	controller := newMembershipControllerStub()
	s := &HTTPServer{
		logger:       zap.NewNop(),
		options:      Options{FailoverController: controller},
		clusterNodes: make(map[string]clusterOverviewNode),
	}

	payload := map[string]any{
		"targetRole": "primary",
		"reason":     "scheduled test",
		"force":      false,
	}
	body, _ := json.Marshal(payload)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ha/drills/dry-run", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.handleHADrillDryRun()(c); err != nil {
		t.Fatalf("drill dry-run handler returned error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	events := s.currentDrillEvents()
	if len(events) == 0 {
		t.Fatalf("expected drill event to be recorded")
	}
}
