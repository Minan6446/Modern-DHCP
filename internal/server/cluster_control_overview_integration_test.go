package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestClusterControlJoinHeartbeatReflectsInOverview(t *testing.T) {
	s := newClusterControlTestServer(t.TempDir())
	e := echo.New()
	e.POST("/cluster/initialize", s.handleClusterInitialize())
	e.POST("/cluster/join", s.handleClusterJoin())
	e.GET("/cluster/state", s.handleClusterState())
	e.GET("/cluster/overview", s.handleClusterOverview())

	api := httptest.NewServer(e)
	defer api.Close()

	initializePayload := map[string]any{
		"clusterDomain":          "cluster.example.internal",
		"primaryNodeId":          "node-a",
		"primaryNodeUrl":         "https://10.0.0.1:8443",
		"primaryNodeIpAddresses": []string{"10.0.0.1"},
	}
	initBody, _ := json.Marshal(initializePayload)
	initResp, err := http.Post(api.URL+"/cluster/initialize", echo.MIMEApplicationJSON, bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("initialize request failed: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected initialize status: %d", initResp.StatusCode)
	}
	var initDecoded struct {
		ClusterToken string `json:"clusterToken"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&initDecoded); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if initDecoded.ClusterToken == "" {
		t.Fatalf("expected cluster token from initialize")
	}

	joinPayload := map[string]any{
		"secondaryNodeId":          "node-b",
		"secondaryNodeName":        "node-b",
		"secondaryNodeUrl":         "https://10.0.0.2:8443",
		"secondaryNodeIpAddresses": []string{"10.0.0.2"},
		"clusterToken":             initDecoded.ClusterToken,
	}
	joinBody, _ := json.Marshal(joinPayload)
	joinResp, err := http.Post(api.URL+"/cluster/join", echo.MIMEApplicationJSON, bytes.NewReader(joinBody))
	if err != nil {
		t.Fatalf("join request failed: %v", err)
	}
	defer joinResp.Body.Close()
	if joinResp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected join status: %d", joinResp.StatusCode)
	}
	var joinDecoded clusterJoinResponse
	if err := json.NewDecoder(joinResp.Body).Decode(&joinDecoded); err != nil {
		t.Fatalf("decode join response: %v", err)
	}
	if joinDecoded.NodeToken == "" {
		t.Fatalf("expected node token from join response")
	}
	if joinDecoded.JoinJobID == "" {
		t.Fatalf("expected join job id from join response")
	}

	client := &http.Client{}
	for i := 0; i < 2; i++ {
		stateReq, _ := http.NewRequest(http.MethodGet, api.URL+"/cluster/state", nil)
		stateReq.Header.Set(clusterHeaderNodeID, "node-b")
		stateReq.Header.Set(clusterHeaderNodeToken, joinDecoded.NodeToken)
		stateResp, err := client.Do(stateReq)
		if err != nil {
			t.Fatalf("state request %d failed: %v", i+1, err)
		}
		if stateResp.StatusCode != http.StatusOK {
			stateResp.Body.Close()
			t.Fatalf("unexpected state status on heartbeat %d: %d", i+1, stateResp.StatusCode)
		}
		stateResp.Body.Close()
	}

	overviewResp, err := http.Get(api.URL + "/cluster/overview")
	if err != nil {
		t.Fatalf("overview request failed: %v", err)
	}
	defer overviewResp.Body.Close()
	if overviewResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected overview status: %d", overviewResp.StatusCode)
	}
	var overview clusterOverviewResponse
	if err := json.NewDecoder(overviewResp.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if len(overview.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes in overview, got %d", len(overview.Nodes))
	}

	var secondary clusterOverviewNode
	var found bool
	for _, node := range overview.Nodes {
		if node.ID == "node-b" {
			secondary = node
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected node-b in overview: %+v", overview.Nodes)
	}
	if secondary.NodeType != clusterMemberTypeSecondary {
		t.Fatalf("expected nodeType=secondary, got %+v", secondary)
	}
	if secondary.NodeState != clusterMemberStateConnected {
		t.Fatalf("expected nodeState=connected after repeated heartbeat, got %+v", secondary)
	}
	if secondary.Health != "healthy" {
		t.Fatalf("expected healthy secondary health, got %+v", secondary)
	}
	if secondary.LastHeartbeat == "" {
		t.Fatalf("expected overview heartbeat for secondary")
	}
}
