package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

func newClusterControlTestServer(tempDir string) *HTTPServer {
	return &HTTPServer{
		logger:                zap.NewNop(),
		options:               Options{APITLS: APITLSOptions{SelfSignedDir: tempDir}},
		configStore:           newInMemoryClusterConfigStore(),
		clusterControlPlane:   defaultClusterControlPlane(),
		clusterControlMembers: make(map[string]clusterControlMemberDTO),
		clusterCommands:       make([]clusterCommandDTO, 0),
		clusterNodes:          make(map[string]clusterOverviewNode),
		clusterNodeAuth:       make(map[string]string),
		alertRuleStore:        make(map[string][]alertRuleDTO),
	}
}

func TestHandleClusterInitializeCreatesTLSAssets(t *testing.T) {
	tempDir := t.TempDir()
	s := newClusterControlTestServer(tempDir)

	payload := map[string]any{
		"clusterDomain":          "cluster.example.internal",
		"primaryNodeId":          "node-a",
		"primaryNodeUrl":         "https://node-a.example.internal:8443",
		"primaryNodeIpAddresses": []string{"10.0.0.10"},
	}
	body, _ := json.Marshal(payload)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/cluster/initialize", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.handleClusterInitialize()(c); err != nil {
		t.Fatalf("initialize handler returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ClusterToken         string                 `json:"clusterToken"`
		Cluster              clusterControlResponse `json:"cluster"`
		TLSMaterialGenerated bool                   `json:"tlsMaterialGenerated"`
		RestartRequired      bool                   `json:"restartRequired"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ClusterToken == "" {
		t.Fatalf("expected cluster token")
	}
	if !resp.TLSMaterialGenerated {
		t.Fatalf("expected tls material to be generated")
	}
	if !resp.RestartRequired || !resp.Cluster.RestartRequired {
		t.Fatalf("expected restart required flags to be true")
	}
	if !resp.Cluster.APITLSEnabled || resp.Cluster.APITLSCertFile == "" {
		t.Fatalf("expected api tls metadata in response: %+v", resp.Cluster)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "api-server-cert.pem")); err != nil {
		t.Fatalf("expected cert file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "api-server-key.pem")); err != nil {
		t.Fatalf("expected key file to exist: %v", err)
	}

	store := s.configStore.(*inMemoryClusterConfigStore)
	var plane clusterControlPlaneDTO
	ok, err := store.Load(context.Background(), clusterConfigKeyControlPlane, &plane)
	if err != nil || !ok {
		t.Fatalf("expected persisted control plane: ok=%v err=%v", ok, err)
	}
	if plane.ClusterDomain != "cluster.example.internal" {
		t.Fatalf("unexpected persisted control plane: %+v", plane)
	}
}

func TestHandleClusterJoinAndStateHeartbeat(t *testing.T) {
	s := newClusterControlTestServer(t.TempDir())
	bootstrapToken := "clt_test_token"
	s.setClusterControlPlane(clusterControlPlaneDTO{
		Initialized:                   true,
		ClusterDomain:                 "cluster.example.internal",
		PrimaryNodeID:                 "node-a",
		PrimaryNodeURL:                "https://node-a.example.internal:8443",
		PrimaryNodeIPAddresses:        []string{"10.0.0.10"},
		HeartbeatIntervalSeconds:      5,
		HeartbeatRetryIntervalSeconds: 15,
		ConfigRefreshIntervalSeconds:  30,
		ConfigRetryIntervalSeconds:    10,
		ConfigVersion:                 1,
		RequireTLS:                    true,
		APITLSEnabled:                 true,
		APITLSCertFile:                "data/cluster/pki/api-server-cert.pem",
		APITLSKeyFile:                 "data/cluster/pki/api-server-key.pem",
		ClusterTokenHash:              hashCredentialToken(bootstrapToken),
	})
	primary := clusterControlMemberDTO{ID: "node-a", Type: clusterMemberTypePrimary, State: clusterMemberStateSelf}
	s.replaceClusterControlMembers([]clusterControlMemberDTO{primary})

	joinPayload := map[string]any{
		"secondaryNodeId":          "node-b",
		"secondaryNodeUrl":         "https://node-b.example.internal:8443",
		"secondaryNodeIpAddresses": []string{"10.0.0.11"},
		"clusterToken":             bootstrapToken,
	}
	body, _ := json.Marshal(joinPayload)
	e := echo.New()
	joinReq := httptest.NewRequest(http.MethodPost, "/cluster/join", bytes.NewReader(body))
	joinReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	joinRec := httptest.NewRecorder()
	joinCtx := e.NewContext(joinReq, joinRec)

	if err := s.handleClusterJoin()(joinCtx); err != nil {
		t.Fatalf("join handler returned error: %v", err)
	}
	if joinRec.Code != http.StatusAccepted {
		t.Fatalf("unexpected join status: %d body=%s", joinRec.Code, joinRec.Body.String())
	}
	var joinResp clusterJoinResponse
	if err := json.Unmarshal(joinRec.Body.Bytes(), &joinResp); err != nil {
		t.Fatalf("decode join response: %v", err)
	}
	if joinResp.NodeToken == "" {
		t.Fatalf("expected node token")
	}
	if joinResp.JoinJobID == "" {
		t.Fatalf("expected join job id")
	}
	if _, ok := s.getJoinJob(joinResp.JoinJobID); !ok {
		t.Fatalf("expected join job to be created")
	}
	runtimeNode, ok := s.customClusterNodeByID("node-b")
	if !ok {
		t.Fatalf("expected runtime node to be created for joined secondary")
	}
	if runtimeNode.Role != "standby" {
		t.Fatalf("expected standby runtime node, got %+v", runtimeNode)
	}
	haCfg := s.currentHAConfig()
	if !haCfg.Partner.Enabled || haCfg.Partner.Address != "node-b.example.internal" {
		t.Fatalf("expected ha partner to be wired to joined secondary, got %+v", haCfg.Partner)
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/cluster/state", nil)
	stateReq.Header.Set(clusterHeaderNodeID, "node-b")
	stateReq.Header.Set(clusterHeaderNodeToken, joinResp.NodeToken)
	stateRec := httptest.NewRecorder()
	stateCtx := e.NewContext(stateReq, stateRec)

	if err := s.handleClusterState()(stateCtx); err != nil {
		t.Fatalf("state handler returned error: %v", err)
	}
	if stateRec.Code != http.StatusOK {
		t.Fatalf("unexpected state status: %d body=%s", stateRec.Code, stateRec.Body.String())
	}
	var stateResp clusterControlResponse
	if err := json.Unmarshal(stateRec.Body.Bytes(), &stateResp); err != nil {
		t.Fatalf("decode state response: %v", err)
	}
	if len(stateResp.Nodes) != 2 {
		t.Fatalf("expected 2 cluster members, got %d", len(stateResp.Nodes))
	}
	var found bool
	for _, node := range stateResp.Nodes {
		if node.ID == "node-b" {
			found = true
			if node.State != clusterMemberStateConnected {
				t.Fatalf("expected connected state, got %+v", node)
			}
		}
	}
	if !found {
		t.Fatalf("expected node-b in state response")
	}
}

func TestHandleClusterDeleteRequiresForce(t *testing.T) {
	s := newClusterControlTestServer(t.TempDir())
	s.setClusterControlPlane(clusterControlPlaneDTO{Initialized: true, ClusterDomain: "cluster.example.internal", APITLSEnabled: true})
	s.setHAConfig(config.HAConfig{Partner: config.PartnerConfig{Enabled: true, Address: "node-b.example.internal"}})
	s.upsertNodeAuth("node-b", "secret")
	s.restoreJoinJobs([]clusterJoinJobDTO{{JobID: "job-1", NodeID: "node-b", Status: joinJobStateRegistered, Progress: 10, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}})
	s.registerClusterCommand(clusterCommandDTO{CommandID: "cmd-1", CommandType: clusterCommandTypeVerify, TargetNodeID: "node-b", Status: clusterCommandStatusCompleted, RequestedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	s.replaceClusterControlMembers([]clusterControlMemberDTO{
		{ID: "node-a", Type: clusterMemberTypePrimary, State: clusterMemberStateSelf},
		{ID: "node-b", Type: clusterMemberTypeSecondary, State: clusterMemberStateConnected},
	})

	e := echo.New()
	deleteReq := httptest.NewRequest(http.MethodPost, "/cluster/delete", bytes.NewReader([]byte(`{"forceDelete":false}`)))
	deleteReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	deleteRec := httptest.NewRecorder()
	deleteCtx := e.NewContext(deleteReq, deleteRec)

	err := s.handleClusterDelete()(deleteCtx)
	if err == nil {
		t.Fatalf("expected precondition error")
	}

	forceReq := httptest.NewRequest(http.MethodPost, "/cluster/delete", bytes.NewReader([]byte(`{"forceDelete":true}`)))
	forceReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	forceRec := httptest.NewRecorder()
	forceCtx := e.NewContext(forceReq, forceRec)
	if err := s.handleClusterDelete()(forceCtx); err != nil {
		t.Fatalf("force delete returned error: %v", err)
	}
	if forceRec.Code != http.StatusOK {
		t.Fatalf("unexpected force delete status: %d body=%s", forceRec.Code, forceRec.Body.String())
	}
	if len(s.snapshotClusterControlMembers()) != 0 {
		t.Fatalf("expected members to be cleared")
	}
	if len(s.snapshotClusterCommands()) != 0 {
		t.Fatalf("expected commands to be cleared")
	}
	if len(s.snapshotJoinJobs()) != 0 {
		t.Fatalf("expected join jobs to be cleared")
	}
	if len(s.snapshotNodeAuth()) != 0 {
		t.Fatalf("expected node auth to be cleared")
	}
	haCfg := s.currentHAConfig()
	if haCfg.Partner.Enabled || haCfg.Partner.Address != "" {
		t.Fatalf("expected ha partner to be cleared, got %+v", haCfg.Partner)
	}
}

func TestHandleClusterCommandCreateAndLeave(t *testing.T) {
	s := newClusterControlTestServer(t.TempDir())
	s.setClusterControlPlane(clusterControlPlaneDTO{
		Initialized:                   true,
		ClusterDomain:                 "cluster.example.internal",
		PrimaryNodeID:                 "node-a",
		HeartbeatIntervalSeconds:      5,
		HeartbeatRetryIntervalSeconds: 15,
		ConfigRefreshIntervalSeconds:  30,
		ConfigRetryIntervalSeconds:    10,
		ConfigVersion:                 1,
		RequireTLS:                    true,
	})
	s.replaceClusterControlMembers([]clusterControlMemberDTO{
		{ID: "node-a", Type: clusterMemberTypePrimary, State: clusterMemberStateSelf, LastSeen: time.Now().UTC()},
		{ID: "node-b", Type: clusterMemberTypeSecondary, State: clusterMemberStateConnected, LastSeen: time.Now().UTC(), URL: "https://node-b.example.internal:8443", IPAddresses: []string{"10.0.0.11"}},
	})
	s.setCustomClusterNode(clusterOverviewNode{ID: "node-b", Role: "standby", Address: "https://node-b.example.internal:8443", HasAuth: true, Version: "test", Health: "healthy", LastHeartbeat: time.Now().UTC().Format(time.RFC3339)})

	e := echo.New()
	cmdReq := httptest.NewRequest(http.MethodPost, "/cluster/commands", bytes.NewReader([]byte(`{"commandType":"verify","targetNodeId":"node-b"}`)))
	cmdReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	cmdRec := httptest.NewRecorder()
	cmdCtx := e.NewContext(cmdReq, cmdRec)
	if err := s.handleClusterCommandCreate()(cmdCtx); err != nil {
		t.Fatalf("command handler returned error: %v", err)
	}
	if cmdRec.Code != http.StatusAccepted {
		t.Fatalf("unexpected command status: %d body=%s", cmdRec.Code, cmdRec.Body.String())
	}
	var command clusterCommandDTO
	if err := json.Unmarshal(cmdRec.Body.Bytes(), &command); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	if command.Status != clusterCommandStatusCompleted || len(command.Receipts) != 1 || command.Receipts[0].Status != clusterCommandStatusCompleted {
		t.Fatalf("expected completed command receipt, got %+v", command)
	}

	leaveReq := httptest.NewRequest(http.MethodPost, "/cluster/members/node-b/leave", bytes.NewReader([]byte(`{"reason":"maintenance"}`)))
	leaveReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	leaveRec := httptest.NewRecorder()
	leaveCtx := e.NewContext(leaveReq, leaveRec)
	leaveCtx.SetParamNames("memberId")
	leaveCtx.SetParamValues("node-b")
	if err := s.handleClusterMemberLeave()(leaveCtx); err != nil {
		t.Fatalf("leave handler returned error: %v", err)
	}
	if leaveRec.Code != http.StatusOK {
		t.Fatalf("unexpected leave status: %d body=%s", leaveRec.Code, leaveRec.Body.String())
	}
	if _, ok := s.clusterControlMemberByID("node-b"); ok {
		t.Fatalf("expected member to be removed after leave")
	}
	if _, ok := s.customClusterNodeByID("node-b"); ok {
		t.Fatalf("expected runtime node to be removed after leave")
	}
	commands := s.snapshotClusterCommands()
	if len(commands) < 2 || commands[0].CommandType != clusterCommandTypeLeave || commands[0].Status != clusterCommandStatusCompleted {
		t.Fatalf("expected leave command recorded, got %+v", commands)
	}
	if len(commands[0].Receipts) != 1 || commands[0].Receipts[0].Status != clusterCommandStatusCompleted {
		t.Fatalf("expected leave receipt completed, got %+v", commands[0])
	}
}

func TestClusterControlResponseRestartFlagClearsWhenTLSActive(t *testing.T) {
	s := newClusterControlTestServer(t.TempDir())
	s.apiTLSRuntimeActive = true
	s.setClusterControlPlane(clusterControlPlaneDTO{
		Initialized:    true,
		APITLSEnabled:  true,
		APITLSCertFile: "data/cluster/pki/api-server-cert.pem",
	})

	resp := s.clusterControlResponse()
	if !resp.APITLSActive {
		t.Fatalf("expected api tls active")
	}
	if resp.RestartRequired {
		t.Fatalf("expected restartRequired to be false when tls runtime is active")
	}
}
