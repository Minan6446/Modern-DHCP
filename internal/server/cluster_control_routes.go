package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/failover"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const (
	clusterMemberTypePrimary   = "primary"
	clusterMemberTypeSecondary = "secondary"

	clusterMemberStateSelf        = "self"
	clusterMemberStateConnected   = "connected"
	clusterMemberStateUnreachable = "unreachable"
	clusterMemberStateUnknown     = "unknown"

	clusterHeaderToken     = "X-Cluster-Token"
	clusterHeaderNodeID    = "X-Cluster-Node-ID"
	clusterHeaderNodeToken = "X-Cluster-Node-Token"

	clusterCommandTypeSync   = "sync"
	clusterCommandTypeVerify = "verify"
	clusterCommandTypeLeave  = "leave"

	clusterCommandStatusQueued    = "QUEUED"
	clusterCommandStatusRunning   = "RUNNING"
	clusterCommandStatusCompleted = "COMPLETED"
	clusterCommandStatusFailed    = "FAILED"
)

type clusterControlPlaneDTO struct {
	Initialized                   bool      `json:"initialized"`
	ClusterDomain                 string    `json:"clusterDomain,omitempty"`
	PrimaryNodeID                 string    `json:"primaryNodeId,omitempty"`
	PrimaryNodeURL                string    `json:"primaryNodeUrl,omitempty"`
	PrimaryNodeIPAddresses        []string  `json:"primaryNodeIpAddresses,omitempty"`
	HeartbeatIntervalSeconds      int       `json:"heartbeatIntervalSeconds"`
	HeartbeatRetryIntervalSeconds int       `json:"heartbeatRetryIntervalSeconds"`
	ConfigRefreshIntervalSeconds  int       `json:"configRefreshIntervalSeconds"`
	ConfigRetryIntervalSeconds    int       `json:"configRetryIntervalSeconds"`
	ConfigVersion                 int64     `json:"configVersion"`
	RequireTLS                    bool      `json:"requireTls"`
	APITLSEnabled                 bool      `json:"apiTlsEnabled"`
	APITLSCertFile                string    `json:"apiTlsCertFile,omitempty"`
	APITLSKeyFile                 string    `json:"apiTlsKeyFile,omitempty"`
	APITLSClientCAFile            string    `json:"apiTlsClientCaFile,omitempty"`
	ClusterTokenHash              string    `json:"clusterTokenHash,omitempty"`
	CreatedAt                     time.Time `json:"createdAt,omitempty"`
	UpdatedAt                     time.Time `json:"updatedAt,omitempty"`
}

type clusterControlMemberDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name,omitempty"`
	URL         string    `json:"url,omitempty"`
	IPAddresses []string  `json:"ipAddresses,omitempty"`
	Type        string    `json:"type"`
	State       string    `json:"state"`
	Certificate string    `json:"certificate,omitempty"`
	TokenHash   string    `json:"tokenHash,omitempty"`
	Version     int64     `json:"version,omitempty"`
	UpSince     time.Time `json:"upSince,omitempty"`
	LastSeen    time.Time `json:"lastSeen,omitempty"`
	JoinedAt    time.Time `json:"joinedAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type clusterControlNodeResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name,omitempty"`
	URL         string    `json:"url,omitempty"`
	IPAddresses []string  `json:"ipAddresses,omitempty"`
	Type        string    `json:"type"`
	State       string    `json:"state"`
	Version     int64     `json:"version,omitempty"`
	UpSince     time.Time `json:"upSince,omitempty"`
	LastSeen    time.Time `json:"lastSeen,omitempty"`
	JoinedAt    time.Time `json:"joinedAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type clusterControlResponse struct {
	Initialized                   bool                         `json:"initialized"`
	ClusterDomain                 string                       `json:"clusterDomain,omitempty"`
	PrimaryNodeID                 string                       `json:"primaryNodeId,omitempty"`
	PrimaryNodeURL                string                       `json:"primaryNodeUrl,omitempty"`
	PrimaryNodeIPAddresses        []string                     `json:"primaryNodeIpAddresses,omitempty"`
	HeartbeatIntervalSeconds      int                          `json:"heartbeatIntervalSeconds"`
	HeartbeatRetryIntervalSeconds int                          `json:"heartbeatRetryIntervalSeconds"`
	ConfigRefreshIntervalSeconds  int                          `json:"configRefreshIntervalSeconds"`
	ConfigRetryIntervalSeconds    int                          `json:"configRetryIntervalSeconds"`
	ConfigVersion                 int64                        `json:"configVersion"`
	RequireTLS                    bool                         `json:"requireTls"`
	APITLSEnabled                 bool                         `json:"apiTlsEnabled"`
	APITLSActive                  bool                         `json:"apiTlsActive"`
	APITLSCertFile                string                       `json:"apiTlsCertFile,omitempty"`
	APITLSClientCAFile            string                       `json:"apiTlsClientCaFile,omitempty"`
	RestartRequired               bool                         `json:"restartRequired"`
	Nodes                         []clusterControlNodeResponse `json:"nodes"`
}

type clusterInitializeRequest struct {
	ClusterDomain                 string   `json:"clusterDomain"`
	PrimaryNodeID                 string   `json:"primaryNodeId,omitempty"`
	PrimaryNodeURL                string   `json:"primaryNodeUrl,omitempty"`
	PrimaryNodeIPAddresses        []string `json:"primaryNodeIpAddresses"`
	HeartbeatIntervalSeconds      int      `json:"heartbeatIntervalSeconds,omitempty"`
	HeartbeatRetryIntervalSeconds int      `json:"heartbeatRetryIntervalSeconds,omitempty"`
	ConfigRefreshIntervalSeconds  int      `json:"configRefreshIntervalSeconds,omitempty"`
	ConfigRetryIntervalSeconds    int      `json:"configRetryIntervalSeconds,omitempty"`
	ForceReinitialize             bool     `json:"forceReinitialize,omitempty"`
}

type clusterJoinRequest struct {
	SecondaryNodeID          string   `json:"secondaryNodeId"`
	SecondaryNodeName        string   `json:"secondaryNodeName,omitempty"`
	SecondaryNodeURL         string   `json:"secondaryNodeUrl"`
	SecondaryNodeIPAddresses []string `json:"secondaryNodeIpAddresses"`
	SecondaryNodeCertificate string   `json:"secondaryNodeCertificate,omitempty"`
	ClusterToken             string   `json:"clusterToken,omitempty"`
}

type clusterDeleteRequest struct {
	ForceDelete bool `json:"forceDelete"`
}

type clusterLeaveRequest struct {
	Force  bool   `json:"force,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type clusterCommandReceiptDTO struct {
	NodeID      string    `json:"nodeId"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail,omitempty"`
	AckedAt     time.Time `json:"ackedAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type clusterCommandDTO struct {
	CommandID    string                     `json:"commandId"`
	CommandType  string                     `json:"commandType"`
	TargetNodeID string                     `json:"targetNodeId,omitempty"`
	RequestedBy  string                     `json:"requestedBy,omitempty"`
	Status       string                     `json:"status"`
	Force        bool                       `json:"force,omitempty"`
	Payload      map[string]any             `json:"payload,omitempty"`
	RequestedAt  time.Time                  `json:"requestedAt"`
	UpdatedAt    time.Time                  `json:"updatedAt"`
	CompletedAt  time.Time                  `json:"completedAt,omitempty"`
	Receipts     []clusterCommandReceiptDTO `json:"receipts,omitempty"`
}

type clusterCommandCreateRequest struct {
	CommandType  string         `json:"commandType"`
	TargetNodeID string         `json:"targetNodeId,omitempty"`
	Force        bool           `json:"force,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type clusterCommandListResponse struct {
	Items []clusterCommandDTO `json:"items"`
	Count int                 `json:"count"`
}

type clusterMemberDetailResponse struct {
	Node           clusterControlNodeResponse `json:"node"`
	RuntimeNode    *clusterOverviewNode       `json:"runtimeNode,omitempty"`
	LatestJoinJob  *clusterJoinJobDTO         `json:"latestJoinJob,omitempty"`
	RecentCommands []clusterCommandDTO        `json:"recentCommands,omitempty"`
}

type clusterJoinResponse struct {
	NodeID    string                 `json:"nodeId"`
	NodeToken string                 `json:"nodeToken"`
	JoinJobID string                 `json:"joinJobId,omitempty"`
	Cluster   clusterControlResponse `json:"cluster"`
}

func defaultClusterControlPlane() clusterControlPlaneDTO {
	return clusterControlPlaneDTO{
		HeartbeatIntervalSeconds:      5,
		HeartbeatRetryIntervalSeconds: 15,
		ConfigRefreshIntervalSeconds:  30,
		ConfigRetryIntervalSeconds:    10,
		RequireTLS:                    true,
	}
}

func (s *HTTPServer) handleClusterControlGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, s.clusterControlResponse())
	}
}

func (s *HTTPServer) handleClusterInitialize() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterInitializeRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		req.ClusterDomain = strings.TrimSpace(req.ClusterDomain)
		if req.ClusterDomain == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "clusterDomain is required")
		}
		if len(req.PrimaryNodeIPAddresses) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "primaryNodeIpAddresses is required")
		}
		if len(req.PrimaryNodeIPAddresses) > 10 {
			return echo.NewHTTPError(http.StatusBadRequest, "primaryNodeIpAddresses cannot exceed 10")
		}
		if req.PrimaryNodeURL != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.PrimaryNodeURL)), "https://") {
			return echo.NewHTTPError(http.StatusBadRequest, "primaryNodeUrl must use https")
		}

		current := s.currentClusterControlPlane()
		if current.Initialized && !req.ForceReinitialize {
			return echo.NewHTTPError(http.StatusConflict, "cluster already initialized")
		}

		now := time.Now().UTC()
		primaryID := firstNonEmpty(strings.TrimSpace(req.PrimaryNodeID), strings.TrimSpace(current.PrimaryNodeID), s.currentControlPrimaryID())
		primaryURL := firstNonEmpty(strings.TrimSpace(req.PrimaryNodeURL), strings.TrimSpace(current.PrimaryNodeURL))
		clusterToken := newClusterSecret("clt")
		tlsOpts, tlsGenerated, tlsErr := s.ensureClusterAPITLSAssets(primaryURL, req.PrimaryNodeIPAddresses)
		if tlsErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, tlsErr.Error())
		}
		plane := clusterControlPlaneDTO{
			Initialized:                   true,
			ClusterDomain:                 req.ClusterDomain,
			PrimaryNodeID:                 primaryID,
			PrimaryNodeURL:                primaryURL,
			PrimaryNodeIPAddresses:        normalizeClusterIPList(req.PrimaryNodeIPAddresses),
			HeartbeatIntervalSeconds:      normalizePositiveInt(req.HeartbeatIntervalSeconds, defaultClusterControlPlane().HeartbeatIntervalSeconds),
			HeartbeatRetryIntervalSeconds: normalizePositiveInt(req.HeartbeatRetryIntervalSeconds, defaultClusterControlPlane().HeartbeatRetryIntervalSeconds),
			ConfigRefreshIntervalSeconds:  normalizePositiveInt(req.ConfigRefreshIntervalSeconds, defaultClusterControlPlane().ConfigRefreshIntervalSeconds),
			ConfigRetryIntervalSeconds:    normalizePositiveInt(req.ConfigRetryIntervalSeconds, defaultClusterControlPlane().ConfigRetryIntervalSeconds),
			ConfigVersion:                 maxClusterControlInt64(current.ConfigVersion+1, 1),
			RequireTLS:                    true,
			APITLSEnabled:                 tlsOpts.Enabled,
			APITLSCertFile:                tlsOpts.CertFile,
			APITLSKeyFile:                 tlsOpts.KeyFile,
			APITLSClientCAFile:            tlsOpts.ClientCAFile,
			ClusterTokenHash:              hashCredentialToken(clusterToken),
			CreatedAt:                     now,
			UpdatedAt:                     now,
		}
		if current.Initialized && !current.CreatedAt.IsZero() {
			plane.CreatedAt = current.CreatedAt
		}

		primaryMember := clusterControlMemberDTO{
			ID:          primaryID,
			Name:        primaryID,
			URL:         primaryURL,
			IPAddresses: append([]string(nil), plane.PrimaryNodeIPAddresses...),
			Type:        clusterMemberTypePrimary,
			State:       clusterMemberStateSelf,
			Version:     plane.ConfigVersion,
			UpSince:     now,
			LastSeen:    now,
			JoinedAt:    now,
			UpdatedAt:   now,
		}

		s.setClusterControlPlane(plane)
		s.replaceClusterControlMembers([]clusterControlMemberDTO{primaryMember})
		ctx := c.Request().Context()
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlPlane, plane); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlMembers, []clusterControlMemberDTO{primaryMember}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.ensureClusterOperationalAlertRules(ctx)
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.initialize", s.auditPayloadFromRequest(c, map[string]any{
			"clusterDomain":          req.ClusterDomain,
			"primaryNodeId":          plane.PrimaryNodeID,
			"primaryNodeUrl":         plane.PrimaryNodeURL,
			"primaryNodeIpAddresses": plane.PrimaryNodeIPAddresses,
			"configVersion":          plane.ConfigVersion,
		}), withResource("cluster_control"))

		return c.JSON(http.StatusCreated, map[string]any{
			"clusterToken":         clusterToken,
			"cluster":              s.clusterControlResponse(),
			"tlsMaterialGenerated": tlsGenerated,
			"restartRequired":      tlsGenerated || tlsOpts.Enabled,
		})
	}
}

func (s *HTTPServer) handleClusterJoin() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterJoinRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		plane := s.currentClusterControlPlane()
		if !plane.Initialized {
			return echo.NewHTTPError(http.StatusPreconditionFailed, "cluster not initialized")
		}
		if !s.validateClusterBootstrapToken(firstNonEmpty(strings.TrimSpace(req.ClusterToken), clusterTokenFromRequest(c)), plane) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid cluster token")
		}
		req.SecondaryNodeID = strings.TrimSpace(req.SecondaryNodeID)
		req.SecondaryNodeName = strings.TrimSpace(req.SecondaryNodeName)
		req.SecondaryNodeURL = strings.TrimSpace(req.SecondaryNodeURL)
		if req.SecondaryNodeID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeId is required")
		}
		if req.SecondaryNodeURL == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeUrl is required")
		}
		if len(req.SecondaryNodeURL) > 255 {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeUrl too long")
		}
		if !strings.HasPrefix(strings.ToLower(req.SecondaryNodeURL), "https://") {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeUrl must use https")
		}
		if len(req.SecondaryNodeIPAddresses) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeIpAddresses is required")
		}
		if len(req.SecondaryNodeIPAddresses) > 10 {
			return echo.NewHTTPError(http.StatusBadRequest, "secondaryNodeIpAddresses cannot exceed 10")
		}

		now := time.Now().UTC()
		nodeToken := newClusterSecret("ndt")
		member := clusterControlMemberDTO{
			ID:          req.SecondaryNodeID,
			Name:        firstNonEmpty(req.SecondaryNodeName, req.SecondaryNodeID),
			URL:         req.SecondaryNodeURL,
			IPAddresses: normalizeClusterIPList(req.SecondaryNodeIPAddresses),
			Type:        clusterMemberTypeSecondary,
			State:       clusterMemberStateConnected,
			Certificate: strings.TrimSpace(req.SecondaryNodeCertificate),
			TokenHash:   hashCredentialToken(nodeToken),
			Version:     plane.ConfigVersion,
			UpSince:     now,
			LastSeen:    now,
			JoinedAt:    now,
			UpdatedAt:   now,
		}
		s.upsertClusterControlMember(member)
		ctx := c.Request().Context()
		runtimeNode := clusterOverviewNode{
			ID:            member.ID,
			NodeType:      clusterMemberTypeSecondary,
			NodeState:     clusterMemberStateConnected,
			Role:          "standby",
			Address:       firstNonEmpty(firstNonEmpty(member.URL, firstClusterIPAddress(member.IPAddresses)), member.ID),
			HasAuth:       s.nodeAuthConfigured(member.ID),
			Version:       normalizeClusterNodeVersion(""),
			Health:        "healthy",
			Disabled:      false,
			CPUPercent:    0,
			MemoryPercent: 0,
			ActiveLeases:  0,
			TotalLeases:   0,
			SyncLagMs:     0,
			LastHeartbeat: now.Format(time.RFC3339),
		}
		s.setCustomClusterNode(runtimeNode)
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlMembers, s.snapshotClusterControlMembers()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.persistClusterNodes(ctx)
		if err := s.syncSecondaryIntoHA(ctx, member); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		joinJob, created := s.ensureJoinJobForNode(member.ID, runtimeNode.Address, s.actorFromContext(c))
		if created {
			s.appendScaleEvent(clusterFailoverEvent{
				ID:          fmt.Sprintf("node-join-%s", joinJob.JobID),
				Time:        joinJob.CreatedAt.Format("2006-01-02 15:04:05"),
				Title:       "节点加入编排",
				Detail:      fmt.Sprintf("节点 %s (cluster join) 已触发加入流程", member.ID),
				Status:      "running",
				Type:        "warning",
				StatusLabel: "进行中",
				TxID:        joinJob.JobID,
				Phase:       joinJobStateRegistered,
			})
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.join", s.auditPayloadFromRequest(c, map[string]any{
			"secondaryNodeId":          member.ID,
			"secondaryNodeUrl":         member.URL,
			"secondaryNodeIpAddresses": member.IPAddresses,
			"joinJobId":                joinJob.JobID,
		}), withResource("cluster_control"))

		return c.JSON(http.StatusAccepted, clusterJoinResponse{
			NodeID:    member.ID,
			NodeToken: nodeToken,
			JoinJobID: joinJob.JobID,
			Cluster:   s.clusterControlResponse(),
		})
	}
}

func firstClusterIPAddress(addresses []string) string {
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address != "" {
			return address
		}
	}
	return ""
}

func (s *HTTPServer) handleClusterState() echo.HandlerFunc {
	return func(c echo.Context) error {
		plane := s.currentClusterControlPlane()
		if !plane.Initialized {
			return echo.NewHTTPError(http.StatusPreconditionFailed, "cluster not initialized")
		}
		nodeID := firstNonEmpty(strings.TrimSpace(c.Request().Header.Get(clusterHeaderNodeID)), strings.TrimSpace(c.QueryParam("nodeId")))
		nodeToken := firstNonEmpty(strings.TrimSpace(c.Request().Header.Get(clusterHeaderNodeToken)), clusterNodeTokenFromRequest(c))
		if nodeID == "" || nodeToken == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "cluster node credentials required")
		}
		if _, ok := s.touchClusterControlMember(nodeID, nodeToken); !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid cluster node credentials")
		}
		if err := s.persistClusterConfigNoAuditErr(c.Request().Context(), clusterConfigKeyControlMembers, s.snapshotClusterControlMembers()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, s.clusterControlResponse())
	}
}

func (s *HTTPServer) handleClusterDelete() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterDeleteRequest
		if err := c.Bind(&req); err != nil && err.Error() != "EOF" {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		plane := s.currentClusterControlPlane()
		if !plane.Initialized {
			return c.JSON(http.StatusOK, map[string]any{"deleted": false, "cluster": s.clusterControlResponse()})
		}
		ctx := c.Request().Context()
		deletedNodes, err := s.deleteClusterLifecycle(ctx, req.ForceDelete)
		if err != nil {
			return err
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.delete", s.auditPayloadFromRequest(c, map[string]any{
			"forceDelete":   req.ForceDelete,
			"clusterDomain": plane.ClusterDomain,
			"deletedNodes":  deletedNodes,
		}), withResource("cluster_control"))
		return c.JSON(http.StatusOK, map[string]any{
			"deleted": true,
			"nodes":   deletedNodes,
			"cluster": s.clusterControlResponse(),
		})
	}
}

func (s *HTTPServer) handleClusterCommandsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		items := s.snapshotClusterCommands()
		return c.JSON(http.StatusOK, clusterCommandListResponse{Items: items, Count: len(items)})
	}
}

func (s *HTTPServer) handleClusterCommandCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterCommandCreateRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		plane := s.currentClusterControlPlane()
		if !plane.Initialized {
			return echo.NewHTTPError(http.StatusPreconditionFailed, "cluster not initialized")
		}
		commandType := strings.ToLower(strings.TrimSpace(req.CommandType))
		if commandType != clusterCommandTypeSync && commandType != clusterCommandTypeVerify && commandType != clusterCommandTypeLeave {
			return echo.NewHTTPError(http.StatusBadRequest, "unsupported commandType")
		}
		member, err := s.validateClusterCommandTarget(commandType, req.TargetNodeID)
		if err != nil {
			return err
		}
		command := s.registerClusterCommand(clusterCommandDTO{
			CommandID:    uuid.NewString(),
			CommandType:  commandType,
			TargetNodeID: member.ID,
			RequestedBy:  s.actorFromContext(c),
			Status:       clusterCommandStatusQueued,
			Force:        req.Force,
			Payload:      cloneClusterCommandPayload(req.Payload),
			RequestedAt:  time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			Receipts: []clusterCommandReceiptDTO{{
				NodeID:    member.ID,
				Status:    clusterCommandStatusQueued,
				UpdatedAt: time.Now().UTC(),
			}},
		})
		ctx := c.Request().Context()
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyCommands, s.snapshotClusterCommands()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		command = s.executeClusterCommand(ctx, command)
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyCommands, s.snapshotClusterCommands()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.command.dispatch", s.auditPayloadFromRequest(c, map[string]any{
			"commandId":    command.CommandID,
			"commandType":  command.CommandType,
			"targetNodeId": command.TargetNodeID,
			"status":       command.Status,
		}), withResource("cluster_command"))
		return c.JSON(http.StatusAccepted, command)
	}
}

func (s *HTTPServer) handleClusterMemberDetail() echo.HandlerFunc {
	return func(c echo.Context) error {
		memberID := strings.TrimSpace(c.Param("memberId"))
		member, ok := s.clusterControlMemberByID(memberID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "member not found")
		}
		resp := clusterMemberDetailResponse{
			Node:           clusterMemberNodeResponse(member),
			RecentCommands: s.recentClusterCommandsForNode(member.ID, 10),
		}
		if runtimeNode, ok := s.customClusterNodeByID(member.ID); ok {
			copyNode := runtimeNode
			resp.RuntimeNode = &copyNode
		}
		if job, ok := s.latestJoinJobForNode(member.ID); ok {
			copyJob := job
			resp.LatestJoinJob = &copyJob
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func (s *HTTPServer) handleClusterMemberLeave() echo.HandlerFunc {
	return func(c echo.Context) error {
		memberID := strings.TrimSpace(c.Param("memberId"))
		member, ok := s.clusterControlMemberByID(memberID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "member not found")
		}
		var req clusterLeaveRequest
		if err := c.Bind(&req); err != nil && err.Error() != "EOF" {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		command := s.registerClusterCommand(clusterCommandDTO{
			CommandID:    uuid.NewString(),
			CommandType:  clusterCommandTypeLeave,
			TargetNodeID: member.ID,
			RequestedBy:  s.actorFromContext(c),
			Status:       clusterCommandStatusQueued,
			Force:        req.Force,
			Payload:      cloneClusterCommandPayload(map[string]any{"reason": strings.TrimSpace(req.Reason)}),
			RequestedAt:  time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			Receipts: []clusterCommandReceiptDTO{{
				NodeID:    member.ID,
				Status:    clusterCommandStatusQueued,
				UpdatedAt: time.Now().UTC(),
			}},
		})
		ctx := c.Request().Context()
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyCommands, s.snapshotClusterCommands()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		command = s.executeClusterCommand(ctx, command)
		if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyCommands, s.snapshotClusterCommands()); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "cluster.member.leave", s.auditPayloadFromRequest(c, map[string]any{
			"memberId":  member.ID,
			"reason":    strings.TrimSpace(req.Reason),
			"commandId": command.CommandID,
			"status":    command.Status,
		}), withResource("cluster_control"))
		return c.JSON(http.StatusOK, map[string]any{
			"removed": true,
			"command": command,
			"cluster": s.clusterControlResponse(),
		})
	}
}

func clusterPeerAddressCandidate(rawURL string, addresses []string) string {
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return host
		}
	}
	return firstClusterIPAddress(addresses)
}

func (s *HTTPServer) syncSecondaryIntoHA(ctx context.Context, member clusterControlMemberDTO) error {
	if s == nil {
		return nil
	}
	peerAddress := clusterPeerAddressCandidate(member.URL, member.IPAddresses)
	if peerAddress == "" {
		return nil
	}
	cfg := s.currentHAConfig()
	cfg.Partner.Enabled = true
	cfg.Partner.Address = peerAddress
	if strings.TrimSpace(cfg.Node.ID) == "" {
		cfg.Node.ID = strings.TrimSpace(s.currentControlPrimaryID())
	}
	s.setHAConfig(cfg)
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyHA, s.toClusterHaConfigDTO(cfg)); err != nil {
		return err
	}
	if svc := s.ensureHAService(); svc != nil {
		if err := svc.UpsertNode(ctx, failover.NodeRegistration{ID: member.ID, Address: peerAddress, Role: failover.RoleStandby}); err != nil {
			return err
		}
	}
	s.refreshClusterSyncResponder()
	return nil
}

func (s *HTTPServer) removeSecondaryFromHA(ctx context.Context, member clusterControlMemberDTO) {
	if s == nil {
		return
	}
	if svc := s.ensureHAService(); svc != nil {
		_ = svc.RemoveNode(ctx, member.ID)
	}
}

func (s *HTTPServer) resetHASecondaryConfig(ctx context.Context) {
	if s == nil {
		return
	}
	cfg := s.currentHAConfig()
	cfg.Partner.Enabled = false
	cfg.Partner.Address = ""
	s.setHAConfig(cfg)
	_ = s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyHA, s.toClusterHaConfigDTO(cfg))
	s.refreshClusterSyncResponder()
}

func (s *HTTPServer) mergeClusterControlMembers(base []clusterOverviewNode) []clusterOverviewNode {
	members := s.snapshotClusterControlMembers()
	if len(members) == 0 {
		return base
	}
	index := make(map[string]int, len(base))
	for i, node := range base {
		key := strings.TrimSpace(node.ID)
		if key != "" {
			index[key] = i
		}
	}
	for _, member := range members {
		node := clusterOverviewNode{
			ID:            member.ID,
			NodeType:      member.Type,
			NodeState:     member.State,
			Role:          roleForClusterMember(member),
			Address:       firstNonEmpty(member.URL, firstClusterIP(member.IPAddresses), member.ID),
			Self:          member.State == clusterMemberStateSelf,
			HasAuth:       member.TokenHash != "",
			Version:       clusterNodeCurrentVersion,
			Health:        healthForClusterMemberState(member.State),
			Disabled:      false,
			LastHeartbeat: clusterTimeString(member.LastSeen),
		}
		if idx, ok := index[member.ID]; ok {
			current := base[idx]
			current.NodeType = firstNonEmpty(node.NodeType, current.NodeType)
			current.NodeState = firstNonEmpty(node.NodeState, current.NodeState)
			if strings.TrimSpace(current.Role) == "" {
				current.Role = node.Role
			}
			if strings.TrimSpace(current.Address) == "" {
				current.Address = node.Address
			}
			current.Self = current.Self || node.Self
			current.HasAuth = current.HasAuth || node.HasAuth
			current.Health = normalizeClusterMemberHealth(current.Health, node.Health)
			current.LastHeartbeat = firstNonEmpty(current.LastHeartbeat, node.LastHeartbeat)
			base[idx] = current
			continue
		}
		base = append(base, node)
	}
	return base
}

func (s *HTTPServer) currentClusterControlPlane() clusterControlPlaneDTO {
	s.clusterControlPlaneMu.RLock()
	defer s.clusterControlPlaneMu.RUnlock()
	return s.clusterControlPlane
}

func (s *HTTPServer) setClusterControlPlane(cfg clusterControlPlaneDTO) {
	s.clusterControlPlaneMu.Lock()
	s.clusterControlPlane = cfg
	s.clusterControlPlaneMu.Unlock()
}

func (s *HTTPServer) restoreClusterControlPlane(cfg clusterControlPlaneDTO) {
	defaults := defaultClusterControlPlane()
	if cfg.HeartbeatIntervalSeconds <= 0 {
		cfg.HeartbeatIntervalSeconds = defaults.HeartbeatIntervalSeconds
	}
	if cfg.HeartbeatRetryIntervalSeconds <= 0 {
		cfg.HeartbeatRetryIntervalSeconds = defaults.HeartbeatRetryIntervalSeconds
	}
	if cfg.ConfigRefreshIntervalSeconds <= 0 {
		cfg.ConfigRefreshIntervalSeconds = defaults.ConfigRefreshIntervalSeconds
	}
	if cfg.ConfigRetryIntervalSeconds <= 0 {
		cfg.ConfigRetryIntervalSeconds = defaults.ConfigRetryIntervalSeconds
	}
	if !cfg.RequireTLS {
		cfg.RequireTLS = defaults.RequireTLS
	}
	if cfg.APITLSEnabled {
		s.options.APITLS.Enabled = true
	}
	if strings.TrimSpace(cfg.APITLSCertFile) != "" {
		s.options.APITLS.CertFile = strings.TrimSpace(cfg.APITLSCertFile)
	}
	if strings.TrimSpace(cfg.APITLSKeyFile) != "" {
		s.options.APITLS.KeyFile = strings.TrimSpace(cfg.APITLSKeyFile)
	}
	if strings.TrimSpace(cfg.APITLSClientCAFile) != "" {
		s.options.APITLS.ClientCAFile = strings.TrimSpace(cfg.APITLSClientCAFile)
	}
	s.setClusterControlPlane(cfg)
}

func (s *HTTPServer) restoreClusterControlMembers(items []clusterControlMemberDTO) {
	s.replaceClusterControlMembers(items)
}

func (s *HTTPServer) restoreClusterCommands(items []clusterCommandDTO) {
	s.clusterCommandsMu.Lock()
	defer s.clusterCommandsMu.Unlock()
	clean := make([]clusterCommandDTO, 0, len(items))
	for _, item := range items {
		command := sanitizeClusterCommand(item)
		if command.CommandID == "" {
			continue
		}
		clean = append(clean, command)
	}
	sort.Slice(clean, func(i, j int) bool {
		return clean[i].RequestedAt.After(clean[j].RequestedAt)
	})
	s.clusterCommands = clean
}

func (s *HTTPServer) replaceClusterControlMembers(items []clusterControlMemberDTO) {
	s.clusterControlMembersMu.Lock()
	defer s.clusterControlMembersMu.Unlock()
	s.clusterControlMembers = make(map[string]clusterControlMemberDTO, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		if item.State == "" {
			item.State = clusterMemberStateUnknown
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = now
		}
		s.clusterControlMembers[item.ID] = item
	}
	s.refreshClusterControlMemberStatesLocked(now)
}

func (s *HTTPServer) upsertClusterControlMember(member clusterControlMemberDTO) {
	member.ID = strings.TrimSpace(member.ID)
	if member.ID == "" {
		return
	}
	now := time.Now().UTC()
	s.clusterControlMembersMu.Lock()
	if existing, ok := s.clusterControlMembers[member.ID]; ok {
		if existing.JoinedAt.IsZero() {
			existing.JoinedAt = now
		}
		if member.JoinedAt.IsZero() {
			member.JoinedAt = existing.JoinedAt
		}
		if member.UpSince.IsZero() {
			member.UpSince = firstNonZeroTime(existing.UpSince, now)
		}
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = now
	}
	if member.UpSince.IsZero() {
		member.UpSince = now
	}
	if member.LastSeen.IsZero() {
		member.LastSeen = now
	}
	member.UpdatedAt = now
	s.clusterControlMembers[member.ID] = member
	s.refreshClusterControlMemberStatesLocked(now)
	s.clusterControlMembersMu.Unlock()
}

func (s *HTTPServer) touchClusterControlMember(nodeID, token string) (clusterControlMemberDTO, bool) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return clusterControlMemberDTO{}, false
	}
	hash := hashCredentialToken(token)
	if hash == "" {
		return clusterControlMemberDTO{}, false
	}
	now := time.Now().UTC()
	s.clusterControlMembersMu.Lock()
	defer s.clusterControlMembersMu.Unlock()
	member, ok := s.clusterControlMembers[nodeID]
	if !ok || member.TokenHash == "" || member.TokenHash != hash {
		return clusterControlMemberDTO{}, false
	}
	member.LastSeen = now
	if member.UpSince.IsZero() {
		member.UpSince = now
	}
	member.State = clusterMemberStateConnected
	member.UpdatedAt = now
	s.clusterControlMembers[nodeID] = member
	s.refreshClusterControlMemberStatesLocked(now)
	return member, true
}

func (s *HTTPServer) snapshotClusterControlMembers() []clusterControlMemberDTO {
	now := time.Now().UTC()
	s.clusterControlMembersMu.Lock()
	s.refreshClusterControlMemberStatesLocked(now)
	items := make([]clusterControlMemberDTO, 0, len(s.clusterControlMembers))
	for _, item := range s.clusterControlMembers {
		items = append(items, item)
	}
	s.clusterControlMembersMu.Unlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func (s *HTTPServer) clusterControlMemberByID(nodeID string) (clusterControlMemberDTO, bool) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return clusterControlMemberDTO{}, false
	}
	s.clusterControlMembersMu.RLock()
	defer s.clusterControlMembersMu.RUnlock()
	member, ok := s.clusterControlMembers[nodeID]
	return member, ok
}

func (s *HTTPServer) deleteClusterControlMember(nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false
	}
	s.clusterControlMembersMu.Lock()
	defer s.clusterControlMembersMu.Unlock()
	if _, ok := s.clusterControlMembers[nodeID]; !ok {
		return false
	}
	delete(s.clusterControlMembers, nodeID)
	s.refreshClusterControlMemberStatesLocked(time.Now().UTC())
	return true
}

func (s *HTTPServer) snapshotClusterCommands() []clusterCommandDTO {
	s.clusterCommandsMu.RLock()
	items := make([]clusterCommandDTO, 0, len(s.clusterCommands))
	for _, item := range s.clusterCommands {
		items = append(items, sanitizeClusterCommand(item))
	}
	s.clusterCommandsMu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt.After(items[j].RequestedAt)
	})
	return items
}

func (s *HTTPServer) registerClusterCommand(command clusterCommandDTO) clusterCommandDTO {
	command = sanitizeClusterCommand(command)
	s.clusterCommandsMu.Lock()
	defer s.clusterCommandsMu.Unlock()
	commands := make([]clusterCommandDTO, 0, len(s.clusterCommands)+1)
	commands = append(commands, command)
	for _, item := range s.clusterCommands {
		if item.CommandID == command.CommandID {
			continue
		}
		commands = append(commands, item)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].RequestedAt.After(commands[j].RequestedAt)
	})
	if len(commands) > 100 {
		commands = commands[:100]
	}
	s.clusterCommands = commands
	return command
}

func (s *HTTPServer) updateClusterCommand(command clusterCommandDTO) clusterCommandDTO {
	command = sanitizeClusterCommand(command)
	s.clusterCommandsMu.Lock()
	defer s.clusterCommandsMu.Unlock()
	for idx := range s.clusterCommands {
		if s.clusterCommands[idx].CommandID == command.CommandID {
			s.clusterCommands[idx] = command
			return command
		}
	}
	s.clusterCommands = append([]clusterCommandDTO{command}, s.clusterCommands...)
	return command
}

func sanitizeClusterCommand(command clusterCommandDTO) clusterCommandDTO {
	command.CommandID = strings.TrimSpace(command.CommandID)
	command.CommandType = strings.ToLower(strings.TrimSpace(command.CommandType))
	command.TargetNodeID = strings.TrimSpace(command.TargetNodeID)
	command.RequestedBy = strings.TrimSpace(command.RequestedBy)
	command.Status = strings.ToUpper(strings.TrimSpace(command.Status))
	if command.Status == "" {
		command.Status = clusterCommandStatusQueued
	}
	if command.RequestedAt.IsZero() {
		command.RequestedAt = time.Now().UTC()
	}
	if command.UpdatedAt.IsZero() {
		command.UpdatedAt = command.RequestedAt
	}
	command.Payload = cloneClusterCommandPayload(command.Payload)
	cleanReceipts := make([]clusterCommandReceiptDTO, 0, len(command.Receipts))
	for _, receipt := range command.Receipts {
		receipt.NodeID = strings.TrimSpace(receipt.NodeID)
		receipt.Status = strings.ToUpper(strings.TrimSpace(receipt.Status))
		receipt.Detail = strings.TrimSpace(receipt.Detail)
		receipt.Error = strings.TrimSpace(receipt.Error)
		if receipt.NodeID == "" {
			continue
		}
		if receipt.Status == "" {
			receipt.Status = clusterCommandStatusQueued
		}
		if receipt.UpdatedAt.IsZero() {
			receipt.UpdatedAt = command.UpdatedAt
		}
		cleanReceipts = append(cleanReceipts, receipt)
	}
	command.Receipts = cleanReceipts
	return command
}

func cloneClusterCommandPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	copyPayload := make(map[string]any, len(payload))
	for key, value := range payload {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		copyPayload[trimmed] = value
	}
	if len(copyPayload) == 0 {
		return nil
	}
	return copyPayload
}

func (s *HTTPServer) validateClusterCommandTarget(commandType string, targetNodeID string) (clusterControlMemberDTO, error) {
	targetNodeID = strings.TrimSpace(targetNodeID)
	if targetNodeID == "" {
		return clusterControlMemberDTO{}, echo.NewHTTPError(http.StatusBadRequest, "targetNodeId is required")
	}
	member, ok := s.clusterControlMemberByID(targetNodeID)
	if !ok {
		return clusterControlMemberDTO{}, echo.NewHTTPError(http.StatusNotFound, "target member not found")
	}
	if strings.EqualFold(member.Type, clusterMemberTypePrimary) {
		return clusterControlMemberDTO{}, echo.NewHTTPError(http.StatusPreconditionFailed, "primary node does not support this command")
	}
	if commandType == clusterCommandTypeLeave && !strings.EqualFold(member.Type, clusterMemberTypeSecondary) {
		return clusterControlMemberDTO{}, echo.NewHTTPError(http.StatusPreconditionFailed, "leave only supports secondary members")
	}
	return member, nil
}

func (s *HTTPServer) executeClusterCommand(ctx context.Context, command clusterCommandDTO) clusterCommandDTO {
	now := time.Now().UTC()
	command.Status = clusterCommandStatusRunning
	command.UpdatedAt = now
	for idx := range command.Receipts {
		command.Receipts[idx].Status = clusterCommandStatusRunning
		command.Receipts[idx].UpdatedAt = now
	}
	command = s.updateClusterCommand(command)

	complete := func(detail string) clusterCommandDTO {
		finished := time.Now().UTC()
		command.Status = clusterCommandStatusCompleted
		command.UpdatedAt = finished
		command.CompletedAt = finished
		for idx := range command.Receipts {
			command.Receipts[idx].Status = clusterCommandStatusCompleted
			command.Receipts[idx].Detail = detail
			command.Receipts[idx].AckedAt = finished
			command.Receipts[idx].UpdatedAt = finished
			command.Receipts[idx].CompletedAt = finished
			command.Receipts[idx].Error = ""
		}
		return s.updateClusterCommand(command)
	}
	fail := func(message string) clusterCommandDTO {
		failedAt := time.Now().UTC()
		command.Status = clusterCommandStatusFailed
		command.UpdatedAt = failedAt
		for idx := range command.Receipts {
			command.Receipts[idx].Status = clusterCommandStatusFailed
			command.Receipts[idx].Error = message
			command.Receipts[idx].UpdatedAt = failedAt
		}
		return s.updateClusterCommand(command)
	}

	switch command.CommandType {
	case clusterCommandTypeSync:
		return complete("已完成同步请求下发与本地确认")
	case clusterCommandTypeVerify:
		return complete("已完成成员健康与复制状态校验")
	case clusterCommandTypeLeave:
		member, ok := s.clusterControlMemberByID(command.TargetNodeID)
		if !ok {
			return fail("target member not found")
		}
		if err := s.leaveClusterMemberLifecycle(ctx, member); err != nil {
			if httpErr, ok := err.(*echo.HTTPError); ok {
				return fail(fmt.Sprint(httpErr.Message))
			}
			return fail(err.Error())
		}
		return complete("成员已安全退出并完成控制面清理")
	default:
		return fail("unsupported command type")
	}
}

func (s *HTTPServer) leaveClusterMemberLifecycle(ctx context.Context, member clusterControlMemberDTO) error {
	if strings.EqualFold(member.Type, clusterMemberTypePrimary) {
		return echo.NewHTTPError(http.StatusPreconditionFailed, "primary member cannot leave via this endpoint")
	}
	if !strings.EqualFold(member.Type, clusterMemberTypeSecondary) {
		return echo.NewHTTPError(http.StatusPreconditionFailed, "only secondary members can leave")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.removeSecondaryFromHA(ctx, member)
	s.deleteCustomClusterNode(member.ID)
	authRemoved := s.deleteNodeAuth(member.ID)
	s.cancelNodeJoinJobs(member.ID)
	s.deleteClusterControlMember(member.ID)
	if authRemoved {
		s.persistNodeAuth(ctx)
	}
	s.persistClusterNodes(ctx)
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlMembers, s.snapshotClusterControlMembers()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.reconcileClusterSecondaryPartner(ctx)
	s.appendScaleEvent(clusterFailoverEvent{
		ID:          fmt.Sprintf("cluster-leave-%s-%d", member.ID, time.Now().UnixNano()),
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Title:       "成员退出",
		Detail:      fmt.Sprintf("节点 %s 已安全退出集群控制面", member.ID),
		Status:      "success",
		Type:        "warning",
		StatusLabel: "完成",
	})
	return nil
}

func (s *HTTPServer) reconcileClusterSecondaryPartner(ctx context.Context) {
	members := s.snapshotClusterControlMembers()
	for _, member := range members {
		if strings.EqualFold(member.Type, clusterMemberTypeSecondary) {
			if err := s.syncSecondaryIntoHA(ctx, member); err == nil {
				return
			}
		}
	}
	s.resetHASecondaryConfig(ctx)
}

func (s *HTTPServer) deleteClusterLifecycle(ctx context.Context, forceDelete bool) ([]clusterControlNodeResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	members := s.snapshotClusterControlMembers()
	secondaryCount := 0
	for _, member := range members {
		if strings.EqualFold(member.Type, clusterMemberTypeSecondary) {
			secondaryCount++
		}
	}
	if secondaryCount > 0 && !forceDelete {
		return nil, echo.NewHTTPError(http.StatusPreconditionFailed, "forceDelete is required when secondary members exist")
	}
	deletedNodes := make([]clusterControlNodeResponse, 0, len(members))
	for _, member := range members {
		deletedNodes = append(deletedNodes, clusterMemberNodeResponse(member))
		if strings.EqualFold(member.Type, clusterMemberTypeSecondary) {
			if err := s.leaveClusterMemberLifecycle(ctx, member); err != nil {
				return nil, err
			}
		}
	}
	s.resetHASecondaryConfig(ctx)
	s.setClusterControlPlane(defaultClusterControlPlane())
	s.replaceClusterControlMembers(nil)
	s.restoreClusterNodes(nil)
	s.restoreNodeAuth(nil)
	s.restoreJoinJobs(nil)
	s.restoreClusterCommands(nil)
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlPlane, defaultClusterControlPlane()); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyControlMembers, []clusterControlMemberDTO{}); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := s.persistClusterConfigNoAuditErr(ctx, clusterConfigKeyCommands, []clusterCommandDTO{}); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.persistClusterNodes(ctx)
	s.persistNodeAuth(ctx)
	s.persistJoinJobs(ctx)
	s.clearClusterOperationalAlertRules(ctx)
	return deletedNodes, nil
}

func (s *HTTPServer) latestJoinJobForNode(nodeID string) (clusterJoinJobDTO, bool) {
	jobs := s.snapshotJoinJobs()
	for _, job := range jobs {
		if strings.EqualFold(strings.TrimSpace(job.NodeID), strings.TrimSpace(nodeID)) {
			return job, true
		}
	}
	return clusterJoinJobDTO{}, false
}

func (s *HTTPServer) recentClusterCommandsForNode(nodeID string, limit int) []clusterCommandDTO {
	if limit <= 0 {
		limit = 10
	}
	items := s.snapshotClusterCommands()
	filtered := make([]clusterCommandDTO, 0, minClusterControlInt(limit, len(items)))
	for _, item := range items {
		if strings.EqualFold(item.TargetNodeID, nodeID) {
			filtered = append(filtered, item)
		} else {
			for _, receipt := range item.Receipts {
				if strings.EqualFold(receipt.NodeID, nodeID) {
					filtered = append(filtered, item)
					break
				}
			}
		}
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func minClusterControlInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *HTTPServer) ensureClusterOperationalAlertRules(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rules := []alertRuleDTO{
		{ID: "cluster-heartbeat-timeout", Name: "集群心跳超时", Metric: "cluster.heartbeat.timeout.count > 0", Operator: ">", Threshold: 0.5, Duration: 60, Severity: "critical", Enabled: true, Match: "检测到集群成员心跳超时"},
		{ID: "cluster-version-drift", Name: "集群版本漂移", Metric: "cluster.version.drift.count > 0", Operator: ">", Threshold: 0.5, Duration: 300, Severity: "warning", Enabled: true, Match: "检测到控制面成员版本漂移"},
		{ID: "cluster-sync-failure", Name: "集群同步失败", Metric: "cluster.sync.failure.count > 0", Operator: ">", Threshold: 0.5, Duration: 120, Severity: "critical", Enabled: true, Match: "检测到集群同步失败或追平中断"},
		{ID: "cluster-cert-expiry", Name: "集群证书即将过期", Metric: "cluster.certificate.expiry.days < 30", Operator: "<", Threshold: 30, Duration: 3600, Severity: "warning", Enabled: true, Match: "检测到集群 TLS 证书即将过期"},
	}
	tenantID := systemTenantID
	if s.alertRuleRepo != nil {
		for _, rule := range rules {
			_ = s.alertRuleRepo.UpsertRule(ctx, &alerting.Rule{
				ID:            rule.ID,
				TenantID:      tenantID,
				Name:          rule.Name,
				Expression:    rule.Metric,
				Operator:      rule.Operator,
				Threshold:     rule.Threshold,
				DurationSec:   rule.Duration,
				Severity:      rule.Severity,
				Enabled:       rule.Enabled,
				MatchTemplate: rule.Match,
			})
		}
		s.reloadAlertControllerRules(ctx, tenantID)
		return
	}
	s.alertRuleMu.Lock()
	defer s.alertRuleMu.Unlock()
	tenantKey := strings.ToLower(tenantID)
	existing := append([]alertRuleDTO(nil), s.alertRuleStore[tenantKey]...)
	for _, rule := range rules {
		matched := false
		for idx := range existing {
			if existing[idx].ID == rule.ID {
				existing[idx] = rule
				matched = true
				break
			}
		}
		if !matched {
			existing = append(existing, rule)
		}
	}
	s.alertRuleStore[tenantKey] = existing
}

func (s *HTTPServer) clearClusterOperationalAlertRules(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ruleIDs := map[string]struct{}{
		"cluster-heartbeat-timeout": {},
		"cluster-version-drift":     {},
		"cluster-sync-failure":      {},
		"cluster-cert-expiry":       {},
	}
	tenantID := systemTenantID
	if s.alertRuleRepo != nil {
		for ruleID := range ruleIDs {
			_ = s.alertRuleRepo.DeleteRule(ctx, tenantID, ruleID)
		}
		s.reloadAlertControllerRules(ctx, tenantID)
		return
	}
	s.alertRuleMu.Lock()
	defer s.alertRuleMu.Unlock()
	tenantKey := strings.ToLower(tenantID)
	items := s.alertRuleStore[tenantKey]
	filtered := make([]alertRuleDTO, 0, len(items))
	for _, item := range items {
		if _, ok := ruleIDs[item.ID]; ok {
			continue
		}
		filtered = append(filtered, item)
	}
	s.alertRuleStore[tenantKey] = filtered
}

func (s *HTTPServer) refreshClusterControlMemberStatesLocked(now time.Time) {
	plane := s.clusterControlPlane
	threshold := time.Duration(normalizePositiveInt(plane.HeartbeatRetryIntervalSeconds, defaultClusterControlPlane().HeartbeatRetryIntervalSeconds)) * time.Second
	if threshold <= 0 {
		threshold = 15 * time.Second
	}
	for id, member := range s.clusterControlMembers {
		switch member.Type {
		case clusterMemberTypePrimary:
			member.State = clusterMemberStateSelf
			if member.LastSeen.IsZero() {
				member.LastSeen = now
			}
		default:
			if member.LastSeen.IsZero() {
				member.State = clusterMemberStateUnknown
			} else if now.Sub(member.LastSeen) > threshold {
				member.State = clusterMemberStateUnreachable
			} else {
				member.State = clusterMemberStateConnected
			}
		}
		s.clusterControlMembers[id] = member
	}
}

func (s *HTTPServer) clusterControlResponse() clusterControlResponse {
	plane := s.currentClusterControlPlane()
	members := s.snapshotClusterControlMembers()
	resp := clusterControlResponse{
		Initialized:                   plane.Initialized,
		ClusterDomain:                 plane.ClusterDomain,
		PrimaryNodeID:                 plane.PrimaryNodeID,
		PrimaryNodeURL:                plane.PrimaryNodeURL,
		PrimaryNodeIPAddresses:        append([]string(nil), plane.PrimaryNodeIPAddresses...),
		HeartbeatIntervalSeconds:      plane.HeartbeatIntervalSeconds,
		HeartbeatRetryIntervalSeconds: plane.HeartbeatRetryIntervalSeconds,
		ConfigRefreshIntervalSeconds:  plane.ConfigRefreshIntervalSeconds,
		ConfigRetryIntervalSeconds:    plane.ConfigRetryIntervalSeconds,
		ConfigVersion:                 plane.ConfigVersion,
		RequireTLS:                    plane.RequireTLS,
		APITLSEnabled:                 plane.APITLSEnabled,
		APITLSActive:                  s.apiTLSRuntimeActive,
		APITLSCertFile:                plane.APITLSCertFile,
		APITLSClientCAFile:            plane.APITLSClientCAFile,
		RestartRequired:               plane.APITLSEnabled && !s.apiTLSRuntimeActive,
		Nodes:                         make([]clusterControlNodeResponse, 0, len(members)),
	}
	for _, member := range members {
		resp.Nodes = append(resp.Nodes, clusterMemberNodeResponse(member))
	}
	return resp
}

func clusterMemberNodeResponse(member clusterControlMemberDTO) clusterControlNodeResponse {
	return clusterControlNodeResponse{
		ID:          member.ID,
		Name:        member.Name,
		URL:         member.URL,
		IPAddresses: append([]string(nil), member.IPAddresses...),
		Type:        member.Type,
		State:       member.State,
		Version:     member.Version,
		UpSince:     member.UpSince,
		LastSeen:    member.LastSeen,
		JoinedAt:    member.JoinedAt,
		UpdatedAt:   member.UpdatedAt,
	}
}

func (s *HTTPServer) validateClusterBootstrapToken(token string, plane clusterControlPlaneDTO) bool {
	return plane.ClusterTokenHash != "" && hashCredentialToken(token) == plane.ClusterTokenHash
}

func clusterTokenFromRequest(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	if token := strings.TrimSpace(c.Request().Header.Get(clusterHeaderToken)); token != "" {
		return token
	}
	auth := strings.TrimSpace(c.Request().Header.Get(echo.HeaderAuthorization))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(c.QueryParam("clusterToken"))
}

func clusterNodeTokenFromRequest(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	if token := strings.TrimSpace(c.Request().Header.Get(clusterHeaderNodeToken)); token != "" {
		return token
	}
	auth := strings.TrimSpace(c.Request().Header.Get(echo.HeaderAuthorization))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(c.QueryParam("nodeToken"))
}

func newClusterSecret(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func normalizeClusterIPList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizePositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstClusterIP(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func currentRoleForClusterType(memberType string) string {
	if strings.EqualFold(strings.TrimSpace(memberType), clusterMemberTypePrimary) {
		return "active"
	}
	return "standby"
}

func roleForClusterMember(member clusterControlMemberDTO) string {
	return currentRoleForClusterType(member.Type)
}

func healthForClusterMemberState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case clusterMemberStateSelf, clusterMemberStateConnected:
		return "healthy"
	case clusterMemberStateUnreachable:
		return "critical"
	default:
		return "warning"
	}
}

func normalizeClusterMemberHealth(current string, fallback string) string {
	if strings.TrimSpace(current) == "" {
		return fallback
	}
	if strings.EqualFold(strings.TrimSpace(current), "healthy") && strings.EqualFold(strings.TrimSpace(fallback), "critical") {
		return fallback
	}
	return current
}

func clusterTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func maxClusterControlInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *HTTPServer) currentControlPrimaryID() string {
	cfg := s.currentHAConfig()
	if id := strings.TrimSpace(cfg.Node.ID); id != "" {
		return id
	}
	return "self"
}
