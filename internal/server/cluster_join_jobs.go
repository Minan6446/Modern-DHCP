package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

const (
	joinJobStateRegistered   = "REGISTERED"
	joinJobStateSnapshotting = "SNAPSHOTTING"
	joinJobStateCatchingUp   = "CATCHING_UP"
	joinJobStateVerifying    = "VERIFYING"
	joinJobStateWarmStandby  = "WARM_STANDBY"
	joinJobStateActive       = "ACTIVE"
	joinJobStateFailed       = "FAILED"
	joinJobStateCanceled     = "CANCELED"
)

const (
	joinPhaseStatusRunning  = "RUNNING"
	joinPhaseStatusSuccess  = "SUCCESS"
	joinPhaseStatusFailed   = "FAILED"
	joinPhaseStatusCanceled = "CANCELED"
)

const (
	joinCheckpointStatusPendingResume = "PENDING_RESUME"
	joinSnapshotStatusImported        = "IMPORTED"
	joinCatchUpStatusPending          = "PENDING"
	joinCatchUpStatusRunning          = "RUNNING"
	joinCatchUpStatusReached          = "HIGH_WATER_REACHED"
)

type clusterJoinPhaseDTO struct {
	Phase      string    `json:"phase"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	DurationMs int64     `json:"durationMs,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type clusterJoinVerificationDTO struct {
	Status                string    `json:"status"`
	CheckedAt             time.Time `json:"checkedAt,omitempty"`
	PoolCount             int       `json:"poolCount,omitempty"`
	BindingCount          int       `json:"bindingCount,omitempty"`
	LeaseCount            int       `json:"leaseCount,omitempty"`
	ActiveLeaseCount      int       `json:"activeLeaseCount,omitempty"`
	ConflictLeaseCount24h int       `json:"conflictLeaseCount24h,omitempty"`
	CreatedLeaseCount24h  int       `json:"createdLeaseCount24h,omitempty"`
	FencingEpoch          string    `json:"fencingEpoch,omitempty"`
	ReplicationHealthy    bool      `json:"replicationHealthy"`
	ReplicationLagMs      int       `json:"replicationLagMs,omitempty"`
	ReplicationMode       string    `json:"replicationMode,omitempty"`
	ReplicationSource     string    `json:"replicationSource,omitempty"`
	ReplicationState      string    `json:"replicationState,omitempty"`
	ReplicationOffset     string    `json:"replicationOffset,omitempty"`
	LastSyncTxID          string    `json:"lastSyncTxId,omitempty"`
	LastSyncType          string    `json:"lastSyncType,omitempty"`
	LastSyncCommitAt      string    `json:"lastSyncCommitAt,omitempty"`
	ConsistencyChecksum   string    `json:"consistencyChecksum,omitempty"`
	Warning               string    `json:"warning,omitempty"`
	FailureReason         string    `json:"failureReason,omitempty"`
}

type clusterJoinSnapshotDTO struct {
	Status              string    `json:"status,omitempty"`
	CapturedAt          time.Time `json:"capturedAt,omitempty"`
	SourceNode          string    `json:"sourceNode,omitempty"`
	SourceAddress       string    `json:"sourceAddress,omitempty"`
	PoolCount           int       `json:"poolCount,omitempty"`
	BindingCount        int       `json:"bindingCount,omitempty"`
	LeaseCount          int       `json:"leaseCount,omitempty"`
	PoolChecksum        string    `json:"poolChecksum,omitempty"`
	BindingChecksum     string    `json:"bindingChecksum,omitempty"`
	LeaseChecksum       string    `json:"leaseChecksum,omitempty"`
	ConsistencyChecksum string    `json:"consistencyChecksum,omitempty"`
	ReplicationOffset   string    `json:"replicationOffset,omitempty"`
	LastSyncTxID        string    `json:"lastSyncTxId,omitempty"`
	LastSyncType        string    `json:"lastSyncType,omitempty"`
	LastSyncCommitAt    string    `json:"lastSyncCommitAt,omitempty"`
}

type clusterJoinCatchUpDTO struct {
	Status               string    `json:"status,omitempty"`
	BaselineOffset       string    `json:"baselineOffset,omitempty"`
	TargetOffset         string    `json:"targetOffset,omitempty"`
	CurrentOffset        string    `json:"currentOffset,omitempty"`
	StartedAt            time.Time `json:"startedAt,omitempty"`
	UpdatedAt            time.Time `json:"updatedAt,omitempty"`
	CompletedAt          time.Time `json:"completedAt,omitempty"`
	HighWatermarkReached bool      `json:"highWatermarkReached,omitempty"`
}

type clusterJoinCheckpointDTO struct {
	Phase              string    `json:"phase,omitempty"`
	Status             string    `json:"status,omitempty"`
	LastCompletedPhase string    `json:"lastCompletedPhase,omitempty"`
	ResumeCount        int       `json:"resumeCount,omitempty"`
	Resumable          bool      `json:"resumable,omitempty"`
	UpdatedAt          time.Time `json:"updatedAt,omitempty"`
	LastError          string    `json:"lastError,omitempty"`
}

type clusterJoinJobDTO struct {
	JobID        string                      `json:"jobId"`
	NodeID       string                      `json:"nodeId"`
	PeerAddress  string                      `json:"peerAddress,omitempty"`
	Status       string                      `json:"status"`
	Progress     int                         `json:"progress"`
	CreatedAt    time.Time                   `json:"createdAt"`
	UpdatedAt    time.Time                   `json:"updatedAt"`
	StartedAt    time.Time                   `json:"startedAt,omitempty"`
	FinishedAt   time.Time                   `json:"finishedAt,omitempty"`
	Error        string                      `json:"error,omitempty"`
	CreatedBy    string                      `json:"createdBy,omitempty"`
	Phases       []clusterJoinPhaseDTO       `json:"phases,omitempty"`
	Snapshot     *clusterJoinSnapshotDTO     `json:"snapshot,omitempty"`
	CatchUp      *clusterJoinCatchUpDTO      `json:"catchUp,omitempty"`
	Checkpoint   *clusterJoinCheckpointDTO   `json:"checkpoint,omitempty"`
	Verification *clusterJoinVerificationDTO `json:"verification,omitempty"`
	RunToken     string                      `json:"-"`
}

type clusterJoinDatasetSummary struct {
	PoolCount           int
	BindingCount        int
	LeaseCount          int
	ActiveLeaseCount    int
	PoolChecksum        string
	BindingChecksum     string
	LeaseChecksum       string
	ConsistencyChecksum string
}

type clusterJoinJobCreateRequest struct {
	NodeID      string `json:"nodeId"`
	PeerAddress string `json:"peerAddress"`
}

func isJoinJobPendingStatus(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case joinJobStateRegistered, joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying:
		return true
	default:
		return false
	}
}

func joinJobProgressForPhase(phase string) int {
	switch strings.ToUpper(strings.TrimSpace(phase)) {
	case joinJobStateSnapshotting:
		return 20
	case joinJobStateCatchingUp:
		return 60
	case joinJobStateVerifying:
		return 85
	default:
		return 0
	}
}

func resumableJoinPhase(job clusterJoinJobDTO) string {
	if job.Checkpoint != nil && job.Checkpoint.Resumable {
		phase := strings.ToUpper(strings.TrimSpace(job.Checkpoint.Phase))
		switch phase {
		case joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying:
			return phase
		}
	}
	switch strings.ToUpper(strings.TrimSpace(job.Status)) {
	case joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying:
		return strings.ToUpper(strings.TrimSpace(job.Status))
	case joinJobStateRegistered:
		return joinJobStateSnapshotting
	default:
		return ""
	}
}

func (s *HTTPServer) ensureJoinJobForNode(nodeID, peerAddress, createdBy string) (clusterJoinJobDTO, bool) {
	if s == nil {
		return clusterJoinJobDTO{}, false
	}
	nodeID = strings.TrimSpace(nodeID)
	peerAddress = strings.TrimSpace(peerAddress)
	createdBy = strings.TrimSpace(createdBy)
	if nodeID == "" {
		return clusterJoinJobDTO{}, false
	}
	now := time.Now().UTC()
	job := clusterJoinJobDTO{
		JobID:       uuid.NewString(),
		NodeID:      nodeID,
		PeerAddress: peerAddress,
		Status:      joinJobStateRegistered,
		Progress:    0,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   createdBy,
		Phases:      []clusterJoinPhaseDTO{},
	}

	s.joinJobsMu.Lock()
	if s.joinJobs == nil {
		s.joinJobs = make(map[string]clusterJoinJobDTO)
	}
	for _, existing := range s.joinJobs {
		if existing.NodeID != nodeID {
			continue
		}
		if isJoinJobPendingStatus(existing.Status) {
			s.joinJobsMu.Unlock()
			return existing, false
		}
	}
	s.joinJobs[job.JobID] = job
	s.joinJobsMu.Unlock()
	s.persistJoinJobs(context.Background())

	s.startJoinJobPipeline(job.JobID)
	return job, true
}

func (s *HTTPServer) handleClusterJoinJobsList() echo.HandlerFunc {
	return func(c echo.Context) error {
		s.joinJobsMu.RLock()
		items := make([]clusterJoinJobDTO, 0, len(s.joinJobs))
		for _, job := range s.joinJobs {
			items = append(items, job)
		}
		s.joinJobsMu.RUnlock()
		sort.Slice(items, func(i, j int) bool {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
		return c.JSON(http.StatusOK, map[string]any{"items": items, "count": len(items)})
	}
}

func (s *HTTPServer) handleClusterJoinJobGet() echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := strings.TrimSpace(c.Param("jobId"))
		if jobID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobId is required")
		}
		job, ok := s.getJoinJob(jobID)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "join job not found")
		}
		return c.JSON(http.StatusOK, job)
	}
}

func (s *HTTPServer) handleClusterJoinJobCreate() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req clusterJoinJobCreateRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		req.NodeID = strings.TrimSpace(req.NodeID)
		req.PeerAddress = strings.TrimSpace(req.PeerAddress)
		if req.NodeID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "nodeId is required")
		}
		job, created := s.ensureJoinJobForNode(req.NodeID, req.PeerAddress, s.actorFromContext(c))
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "ha.join_job.create", map[string]any{
			"jobId":       job.JobID,
			"nodeId":      job.NodeID,
			"peerAddress": job.PeerAddress,
			"created":     created,
		}, withResource("ha"))
		return c.JSON(http.StatusAccepted, job)
	}
}

func (s *HTTPServer) handleClusterJoinJobCancel() echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := strings.TrimSpace(c.Param("jobId"))
		if jobID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobId is required")
		}
		s.cancelJoinJobPipeline(jobID)
		job, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
			now := time.Now().UTC()
			finishLastRunningPhase(v, now, joinPhaseStatusCanceled, "")
			v.Status = joinJobStateCanceled
			v.Progress = 0
			v.UpdatedAt = now
			v.FinishedAt = now
			v.Error = ""
			v.Snapshot = nil
			v.CatchUp = nil
			v.Checkpoint = nil
			v.Verification = nil
			v.RunToken = ""
		})
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "join job not found")
		}
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "ha.join_job.cancel", map[string]any{
			"jobId": jobID,
		}, withResource("ha"))
		return c.JSON(http.StatusOK, job)
	}
}

func (s *HTTPServer) handleClusterJoinJobRetry() echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := strings.TrimSpace(c.Param("jobId"))
		if jobID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "jobId is required")
		}
		s.cancelJoinJobPipeline(jobID)
		job, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
			now := time.Now().UTC()
			v.Status = joinJobStateRegistered
			v.Progress = 0
			v.UpdatedAt = now
			v.StartedAt = time.Time{}
			v.FinishedAt = time.Time{}
			v.Error = ""
			v.RunToken = ""
			v.Phases = []clusterJoinPhaseDTO{}
			v.Snapshot = nil
			v.CatchUp = nil
			v.Checkpoint = nil
			v.Verification = nil
		})
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "join job not found")
		}
		s.startJoinJobPipeline(jobID)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), s.actorFromContext(c), "ha.join_job.retry", map[string]any{
			"jobId": jobID,
		}, withResource("ha"))
		return c.JSON(http.StatusAccepted, job)
	}
}

func (s *HTTPServer) getJoinJob(jobID string) (clusterJoinJobDTO, bool) {
	s.joinJobsMu.RLock()
	job, ok := s.joinJobs[jobID]
	s.joinJobsMu.RUnlock()
	return job, ok
}

func (s *HTTPServer) updateJoinJob(jobID string, mutate func(v *clusterJoinJobDTO)) (clusterJoinJobDTO, bool) {
	s.joinJobsMu.Lock()
	job, ok := s.joinJobs[jobID]
	if !ok {
		s.joinJobsMu.Unlock()
		return clusterJoinJobDTO{}, false
	}
	if mutate != nil {
		mutate(&job)
	}
	s.joinJobs[jobID] = job
	s.joinJobsMu.Unlock()
	s.persistJoinJobs(context.Background())
	return job, true
}

func (s *HTTPServer) startJoinJobPipeline(jobID string) {
	s.launchJoinJobPipeline(jobID, joinJobStateSnapshotting, false)
}

func (s *HTTPServer) resumeJoinJobPipeline(jobID string) {
	job, ok := s.getJoinJob(jobID)
	if !ok {
		return
	}
	phase := resumableJoinPhase(job)
	if phase == "" {
		return
	}
	s.launchJoinJobPipeline(jobID, phase, true)
}

func (s *HTTPServer) launchJoinJobPipeline(jobID, phase string, resume bool) {
	s.cancelJoinJobPipeline(jobID)
	phase = strings.ToUpper(strings.TrimSpace(phase))
	if phase == "" {
		phase = joinJobStateSnapshotting
	}
	runToken := uuid.NewString()
	now := time.Now().UTC()
	_, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		v.RunToken = runToken
		v.Status = phase
		v.Progress = joinJobProgressForPhase(phase)
		v.UpdatedAt = now
		if v.StartedAt.IsZero() || !resume {
			v.StartedAt = now
		}
		v.FinishedAt = time.Time{}
		v.Error = ""
		if !resume || phase == joinJobStateSnapshotting {
			v.Phases = []clusterJoinPhaseDTO{}
			v.Snapshot = nil
			v.CatchUp = nil
			v.Verification = nil
		}
		markJoinPhaseRunning(v, phase, now)
		updateJoinCheckpoint(v, phase, joinPhaseStatusRunning, "", "", true, now)
		if resume && v.Checkpoint != nil {
			v.Checkpoint.ResumeCount++
		}
	})
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.setJoinJobCancel(jobID, cancel)
	go s.runJoinJobPipeline(ctx, jobID, runToken, phase)
}

func (s *HTTPServer) runJoinJobPipeline(ctx context.Context, jobID, runToken, startPhase string) {
	defer s.clearJoinJobCancel(jobID)
	phases := []string{joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying}
	started := false
	for idx, phase := range phases {
		if !started {
			if phase != startPhase {
				continue
			}
			started = true
		} else if !s.beginJoinPhase(jobID, runToken, phase, joinJobProgressForPhase(phase)) {
			return
		}
		job, ok := s.getJoinJob(jobID)
		if !ok {
			return
		}
		if err := s.executeJoinPhase(ctx, jobID, job.NodeID, job.PeerAddress, phase); err != nil {
			s.handleJoinPhaseError(jobID, runToken, err)
			return
		}
		if !s.completeJoinPhase(jobID, runToken, phase, joinPhaseStatusSuccess, "") {
			return
		}
		if idx == len(phases)-1 {
			break
		}
	}

	_, _ = s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if strings.TrimSpace(v.RunToken) == "" || v.RunToken != runToken {
			return
		}
		now := time.Now().UTC()
		v.Status = joinJobStateWarmStandby
		v.Progress = 100
		v.UpdatedAt = now
		v.FinishedAt = now
		v.RunToken = ""
		updateJoinCheckpoint(v, joinJobStateVerifying, joinPhaseStatusSuccess, joinJobStateVerifying, "", false, now)
	})
}

func (s *HTTPServer) executeJoinPhase(ctx context.Context, jobID, nodeID, peerAddress, phase string) error {
	executor := s.options.JoinJobExecutor
	switch phase {
	case joinJobStateSnapshotting:
		if executor == nil {
			return s.defaultJoinPhase(ctx, phase)
		}
		return executor.SnapshotNode(ctx, jobID, nodeID, peerAddress)
	case joinJobStateCatchingUp:
		if executor == nil {
			return s.defaultJoinPhase(ctx, phase)
		}
		return executor.CatchUpNode(ctx, jobID, nodeID, peerAddress)
	case joinJobStateVerifying:
		if executor == nil {
			if err := s.defaultJoinPhase(ctx, phase); err != nil {
				return err
			}
		} else if err := executor.VerifyNode(ctx, jobID, nodeID, peerAddress); err != nil {
			return err
		}
		return s.runJoinConsistencyCheck(jobID, nodeID)
	default:
		return nil
	}
}

func (s *HTTPServer) runJoinConsistencyCheck(jobID, nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("join consistency: node id is required")
	}
	node, ok := s.customClusterNodeByID(nodeID)
	if !ok {
		return fmt.Errorf("join consistency: node %s not found", nodeID)
	}
	if node.Disabled {
		return fmt.Errorf("join consistency: node %s is disabled", nodeID)
	}
	role := strings.ToLower(strings.TrimSpace(node.Role))
	if role != "standby" && role != "active" && role != "primary" {
		return fmt.Errorf("join consistency: node %s has unexpected role %s", nodeID, role)
	}
	syncCfg := s.currentSyncConfig()
	if strings.EqualFold(strings.TrimSpace(syncCfg.Transport), "mock") {
		return fmt.Errorf("join consistency: sync transport still mock")
	}
	summary, err := s.collectJoinDatasetSummary(context.Background())
	if err != nil {
		return err
	}
	pools, bindings, leases, err := s.loadJoinDataset(context.Background())
	if err != nil {
		return err
	}
	if err := validateJoinDataset(summary, pools, bindings, leases); err != nil {
		return err
	}
	if jobID = strings.TrimSpace(jobID); jobID != "" {
		if job, ok := s.getJoinJob(jobID); ok && job.CatchUp != nil {
			if target := strings.TrimSpace(job.CatchUp.TargetOffset); target != "" && !job.CatchUp.HighWatermarkReached {
				return fmt.Errorf("join consistency: catch-up high-water mark %s not reached", target)
			}
		}
	}
	if s.logger != nil {
		s.logger.Debug("join consistency verified",
			zap.String("nodeId", nodeID),
			zap.String("checksum", summary.ConsistencyChecksum),
			zap.Int("poolCount", summary.PoolCount),
			zap.Int("bindingCount", summary.BindingCount),
			zap.Int("leaseCount", summary.LeaseCount),
		)
	}
	return nil
}

func (s *HTTPServer) setJoinJobVerification(jobID string, verification *clusterJoinVerificationDTO) {
	if s == nil {
		return
	}
	_, _ = s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		v.Verification = verification
		v.UpdatedAt = time.Now().UTC()
	})
}

func (s *HTTPServer) persistJoinJobs(ctx context.Context) {
	if s == nil {
		return
	}
	s.persistClusterConfigNoAudit(ctx, clusterConfigKeyJoinJobs, s.snapshotJoinJobs())
}

func (s *HTTPServer) snapshotJoinJobs() []clusterJoinJobDTO {
	s.joinJobsMu.RLock()
	items := make([]clusterJoinJobDTO, 0, len(s.joinJobs))
	for _, job := range s.joinJobs {
		items = append(items, sanitizeJoinJobForPersist(job))
	}
	s.joinJobsMu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (s *HTTPServer) restoreJoinJobs(items []clusterJoinJobDTO) {
	resumableJobIDs := make([]string, 0)
	s.joinJobsMu.Lock()
	if s.joinJobs == nil {
		s.joinJobs = make(map[string]clusterJoinJobDTO)
	}
	for key := range s.joinJobs {
		delete(s.joinJobs, key)
	}
	for _, item := range items {
		job, ok := sanitizeJoinJobForRestore(item)
		if !ok {
			continue
		}
		s.joinJobs[job.JobID] = job
		if isJoinJobPendingStatus(job.Status) && resumableJoinPhase(job) != "" {
			resumableJobIDs = append(resumableJobIDs, job.JobID)
		}
	}
	s.joinJobsMu.Unlock()
	s.joinCancelsMu.Lock()
	for key := range s.joinCancels {
		delete(s.joinCancels, key)
	}
	s.joinCancelsMu.Unlock()
	for _, jobID := range resumableJobIDs {
		s.resumeJoinJobPipeline(jobID)
	}
}

func sanitizeJoinJobForPersist(job clusterJoinJobDTO) clusterJoinJobDTO {
	job.JobID = strings.TrimSpace(job.JobID)
	job.NodeID = strings.TrimSpace(job.NodeID)
	job.PeerAddress = strings.TrimSpace(job.PeerAddress)
	job.Status = strings.ToUpper(strings.TrimSpace(job.Status))
	job.Error = strings.TrimSpace(job.Error)
	job.CreatedBy = strings.TrimSpace(job.CreatedBy)
	job.RunToken = ""
	if job.Progress < 0 {
		job.Progress = 0
	}
	if job.Progress > 100 {
		job.Progress = 100
	}
	phases := make([]clusterJoinPhaseDTO, 0, len(job.Phases))
	for _, phase := range job.Phases {
		phase.Phase = strings.ToUpper(strings.TrimSpace(phase.Phase))
		phase.Status = strings.ToUpper(strings.TrimSpace(phase.Status))
		phase.Error = strings.TrimSpace(phase.Error)
		phases = append(phases, phase)
	}
	job.Phases = phases
	if job.Verification != nil {
		verification := *job.Verification
		verification.Status = strings.ToUpper(strings.TrimSpace(verification.Status))
		verification.FencingEpoch = strings.TrimSpace(verification.FencingEpoch)
		verification.ReplicationMode = strings.TrimSpace(verification.ReplicationMode)
		verification.ReplicationSource = strings.TrimSpace(verification.ReplicationSource)
		verification.ReplicationState = strings.TrimSpace(verification.ReplicationState)
		verification.ReplicationOffset = strings.TrimSpace(verification.ReplicationOffset)
		verification.LastSyncTxID = strings.TrimSpace(verification.LastSyncTxID)
		verification.LastSyncType = strings.TrimSpace(verification.LastSyncType)
		verification.LastSyncCommitAt = strings.TrimSpace(verification.LastSyncCommitAt)
		verification.ConsistencyChecksum = strings.TrimSpace(verification.ConsistencyChecksum)
		verification.Warning = strings.TrimSpace(verification.Warning)
		verification.FailureReason = strings.TrimSpace(verification.FailureReason)
		job.Verification = &verification
	}
	if job.Snapshot != nil {
		snapshot := *job.Snapshot
		snapshot.Status = strings.ToUpper(strings.TrimSpace(snapshot.Status))
		snapshot.SourceNode = strings.TrimSpace(snapshot.SourceNode)
		snapshot.SourceAddress = strings.TrimSpace(snapshot.SourceAddress)
		snapshot.PoolChecksum = strings.TrimSpace(snapshot.PoolChecksum)
		snapshot.BindingChecksum = strings.TrimSpace(snapshot.BindingChecksum)
		snapshot.LeaseChecksum = strings.TrimSpace(snapshot.LeaseChecksum)
		snapshot.ConsistencyChecksum = strings.TrimSpace(snapshot.ConsistencyChecksum)
		snapshot.ReplicationOffset = strings.TrimSpace(snapshot.ReplicationOffset)
		snapshot.LastSyncTxID = strings.TrimSpace(snapshot.LastSyncTxID)
		snapshot.LastSyncType = strings.TrimSpace(snapshot.LastSyncType)
		snapshot.LastSyncCommitAt = strings.TrimSpace(snapshot.LastSyncCommitAt)
		job.Snapshot = &snapshot
	}
	if job.CatchUp != nil {
		catchUp := *job.CatchUp
		catchUp.Status = strings.ToUpper(strings.TrimSpace(catchUp.Status))
		catchUp.BaselineOffset = strings.TrimSpace(catchUp.BaselineOffset)
		catchUp.TargetOffset = strings.TrimSpace(catchUp.TargetOffset)
		catchUp.CurrentOffset = strings.TrimSpace(catchUp.CurrentOffset)
		job.CatchUp = &catchUp
	}
	if job.Checkpoint != nil {
		checkpoint := *job.Checkpoint
		checkpoint.Phase = strings.ToUpper(strings.TrimSpace(checkpoint.Phase))
		checkpoint.Status = strings.ToUpper(strings.TrimSpace(checkpoint.Status))
		checkpoint.LastCompletedPhase = strings.ToUpper(strings.TrimSpace(checkpoint.LastCompletedPhase))
		checkpoint.LastError = strings.TrimSpace(checkpoint.LastError)
		job.Checkpoint = &checkpoint
	}
	return job
}

func sanitizeJoinJobForRestore(job clusterJoinJobDTO) (clusterJoinJobDTO, bool) {
	job = sanitizeJoinJobForPersist(job)
	if job.JobID == "" || job.NodeID == "" {
		return clusterJoinJobDTO{}, false
	}
	if isJoinJobPendingStatus(job.Status) {
		now := time.Now().UTC()
		resumePhase := resumableJoinPhase(job)
		finishLastRunningPhase(&job, now, joinPhaseStatusFailed, "join job interrupted by server restart; resume scheduled")
		job.UpdatedAt = now
		job.RunToken = ""
		if job.Checkpoint == nil {
			job.Checkpoint = &clusterJoinCheckpointDTO{}
		}
		job.Checkpoint.Phase = resumePhase
		job.Checkpoint.Status = joinCheckpointStatusPendingResume
		job.Checkpoint.Resumable = resumePhase != ""
		job.Checkpoint.UpdatedAt = now
		job.Checkpoint.LastError = ""
		job.Error = joinVerificationMessage(job.Error, "join job interrupted by server restart; resume scheduled")
		if job.Verification != nil && strings.TrimSpace(job.Verification.Status) == "" {
			job.Verification.Status = "FAILED"
		}
	}
	return job, true
}

func (s *HTTPServer) buildJoinJobVerification(ctx context.Context, nodeID, peerAddress string) clusterJoinVerificationDTO {
	verification := clusterJoinVerificationDTO{
		Status:    "VERIFIED",
		CheckedAt: time.Now().UTC(),
	}
	if ctx == nil {
		ctx = context.Background()
	}

	snap := failover.StatusSnapshot{}
	if svc := s.ensureHAService(); svc != nil {
		snap = svc.Snapshot()
	} else if s.options.Coordinator != nil {
		snap = s.options.Coordinator.Snapshot()
	}
	verification.ReplicationHealthy = snap.Replication.Healthy
	verification.ReplicationLagMs = snap.Replication.ApplyLagMs
	verification.ReplicationMode = strings.TrimSpace(snap.Replication.Mode)
	verification.ReplicationSource = strings.TrimSpace(snap.Replication.Source)
	verification.ReplicationState = strings.TrimSpace(snap.Replication.State)
	if verification.ReplicationSource == "" {
		verification.ReplicationSource = strings.TrimSpace(peerAddress)
	}
	verification.ReplicationOffset = strings.TrimSpace(snap.Replication.LastOffset)
	verification.FencingEpoch = strings.TrimSpace(snap.FencingEpoch)
	if summary, err := s.collectJoinDatasetSummary(ctx); err == nil {
		verification.PoolCount = summary.PoolCount
		verification.BindingCount = summary.BindingCount
		verification.LeaseCount = summary.LeaseCount
		verification.ActiveLeaseCount = summary.ActiveLeaseCount
		verification.ConsistencyChecksum = summary.ConsistencyChecksum
	} else {
		verification.Warning = joinVerificationMessage(verification.Warning, err.Error())
	}

	poolScope := pool.NewResourceScope(defaultAccessScopeID, systemTenantID)
	if s.poolSvc != nil {
		if _, err := s.poolSvc.CountPools(ctx, poolScope); err != nil {
			verification.Warning = joinVerificationMessage(verification.Warning, fmt.Sprintf("pool count unavailable: %v", err))
		}
		if _, err := s.poolSvc.CountBindings(ctx, poolScope, pool.BindingFilter{}); err != nil {
			verification.Warning = joinVerificationMessage(verification.Warning, fmt.Sprintf("binding count unavailable: %v", err))
		}
	}

	leaseScope := lease.NewResourceScope(defaultAccessScopeID, systemTenantID)
	windowStart := verification.CheckedAt.Add(-24 * time.Hour)
	if s.leaseSvc != nil {
		if count, err := s.leaseSvc.CountActiveLeases(ctx, leaseScope); err == nil {
			verification.ActiveLeaseCount = count
		} else {
			verification.Warning = joinVerificationMessage(verification.Warning, fmt.Sprintf("active lease count unavailable: %v", err))
		}
		if count, err := s.leaseSvc.CountConflictLeasesSince(ctx, leaseScope, windowStart); err == nil {
			verification.ConflictLeaseCount24h = count
		} else {
			verification.Warning = joinVerificationMessage(verification.Warning, fmt.Sprintf("conflict lease count unavailable: %v", err))
		}
		if count, err := s.leaseSvc.CountLeasesCreatedSince(ctx, leaseScope, windowStart); err == nil {
			verification.CreatedLeaseCount24h = count
		} else {
			verification.Warning = joinVerificationMessage(verification.Warning, fmt.Sprintf("created lease count unavailable: %v", err))
		}
	}

	if checksum, err := s.joinConsistencyChecksum(nodeID); err == nil {
		verification.ConsistencyChecksum = checksum
	} else {
		verification.Warning = joinVerificationMessage(verification.Warning, err.Error())
	}

	if tx, ok := s.latestCommittedJoinSync(nodeID, peerAddress); ok {
		verification.LastSyncTxID = tx.TxID
		verification.LastSyncType = tx.Type
		verification.LastSyncCommitAt = tx.CommitAt
	}

	if !verification.ReplicationHealthy {
		verification.Status = "FAILED"
		verification.FailureReason = joinVerificationMessage(verification.FailureReason, "replication unhealthy")
	}
	if verification.FencingEpoch == "" {
		verification.Status = "FAILED"
		verification.FailureReason = joinVerificationMessage(verification.FailureReason, "fencing epoch unavailable")
	}

	return verification
}

func (s *HTTPServer) latestCommittedJoinSync(nodeID, peerAddress string) (clusterSyncTransactionDTO, bool) {
	node, _ := s.customClusterNodeByID(strings.TrimSpace(nodeID))
	peerCandidates := []string{
		strings.TrimSpace(peerAddress),
		strings.TrimSpace(node.Address),
		strings.TrimSpace(node.ID),
	}
	for _, tx := range s.recentSyncTransactions(0) {
		if !strings.EqualFold(strings.TrimSpace(tx.Phase), "committed") {
			continue
		}
		for _, candidate := range peerCandidates {
			if candidate == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(tx.TargetNode), candidate) || strings.EqualFold(strings.TrimSpace(tx.SourceNode), candidate) {
				return tx, true
			}
		}
	}
	return clusterSyncTransactionDTO{}, false
}

func (s *HTTPServer) joinConsistencyChecksum(nodeID string) (string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return "", fmt.Errorf("join consistency: node id is required")
	}
	if _, ok := s.customClusterNodeByID(nodeID); !ok {
		return "", fmt.Errorf("join consistency: node %s not found", nodeID)
	}
	summary, err := s.collectJoinDatasetSummary(context.Background())
	if err != nil {
		return "", err
	}
	return summary.ConsistencyChecksum, nil
}

func joinVerificationMessage(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	return current + "; " + next
}

func (s *HTTPServer) defaultJoinPhase(ctx context.Context, phase string) error {
	delay := 1500 * time.Millisecond
	switch phase {
	case joinJobStateCatchingUp, joinJobStateVerifying:
		delay = 2 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *HTTPServer) handleJoinPhaseError(jobID, runToken string, err error) {
	if err == nil || err == context.Canceled {
		return
	}
	now := time.Now().UTC()
	_, _ = s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if strings.TrimSpace(v.RunToken) == "" || v.RunToken != runToken {
			return
		}
		finishLastRunningPhase(v, now, joinPhaseStatusFailed, err.Error())
		v.Status = joinJobStateFailed
		v.UpdatedAt = now
		v.FinishedAt = now
		v.Error = err.Error()
		v.RunToken = ""
		updateJoinCheckpoint(v, v.Status, joinPhaseStatusFailed, "", err.Error(), false, now)
	})
	eventID := fmt.Sprintf("join-failed-%s", jobID)
	s.appendScaleEvent(clusterFailoverEvent{
		ID:          eventID,
		Time:        now.Format("2006-01-02 15:04:05"),
		Title:       "节点加入失败",
		Detail:      err.Error(),
		Status:      "failed",
		Type:        "danger",
		StatusLabel: "失败",
		TxID:        jobID,
		Phase:       joinJobStateFailed,
		ErrorCode:   "JOIN_FAILED",
	})
	alertEvent := alerting.Event{
		ID:         eventID,
		Severity:   alerting.SeverityMajor,
		Category:   "cluster.join",
		Summary:    "节点加入流程失败",
		Details:    err.Error(),
		OccurredAt: now,
		Labels: map[string]string{
			"jobId": jobID,
			"phase": joinJobStateFailed,
		},
		Resources: []string{"cluster_join_job"},
	}
	if s.alertFeed != nil {
		s.alertFeed.Record(alertEvent)
	}
	if s.alertManager != nil {
		s.alertManager.Notify(context.Background(), alertEvent)
	}
}

func (s *HTTPServer) beginJoinPhase(jobID, runToken, phase string, progress int) bool {
	_, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if strings.TrimSpace(v.RunToken) == "" || v.RunToken != runToken {
			return
		}
		if v.Status == joinJobStateCanceled || v.Status == joinJobStateFailed {
			return
		}
		now := time.Now().UTC()
		v.Status = phase
		v.Progress = progress
		v.UpdatedAt = now
		markJoinPhaseRunning(v, phase, now)
		updateJoinCheckpoint(v, phase, joinPhaseStatusRunning, "", "", true, now)
	})
	if !ok {
		return false
	}
	job, exists := s.getJoinJob(jobID)
	if !exists {
		return false
	}
	if job.RunToken != runToken {
		return false
	}
	if job.Status == joinJobStateCanceled || job.Status == joinJobStateFailed {
		return false
	}
	return true
}

func (s *HTTPServer) completeJoinPhase(jobID, runToken, phase, phaseStatus, phaseErr string) bool {
	_, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if strings.TrimSpace(v.RunToken) == "" || v.RunToken != runToken {
			return
		}
		now := time.Now().UTC()
		completeLatestPhase(v, phase, now, phaseStatus, phaseErr)
		v.UpdatedAt = now
		lastCompleted := ""
		if strings.EqualFold(phaseStatus, joinPhaseStatusSuccess) {
			lastCompleted = phase
		}
		updateJoinCheckpoint(v, phase, phaseStatus, lastCompleted, phaseErr, !strings.EqualFold(phaseStatus, joinPhaseStatusSuccess) || phase != joinJobStateVerifying, now)
	})
	if !ok {
		return false
	}
	job, exists := s.getJoinJob(jobID)
	if !exists {
		return false
	}
	if job.RunToken != runToken {
		return false
	}
	if job.Status == joinJobStateCanceled || job.Status == joinJobStateFailed {
		return false
	}
	return true
}

func completeLatestPhase(job *clusterJoinJobDTO, phase string, finishedAt time.Time, status, errMsg string) {
	if job == nil {
		return
	}
	for i := len(job.Phases) - 1; i >= 0; i-- {
		rec := &job.Phases[i]
		if rec.Phase != phase {
			continue
		}
		if !rec.FinishedAt.IsZero() {
			continue
		}
		rec.FinishedAt = finishedAt
		rec.Status = status
		rec.DurationMs = maxInt64(0, finishedAt.Sub(rec.StartedAt).Milliseconds())
		rec.Error = errMsg
		return
	}
}

func finishLastRunningPhase(job *clusterJoinJobDTO, finishedAt time.Time, status, errMsg string) {
	if job == nil {
		return
	}
	for i := len(job.Phases) - 1; i >= 0; i-- {
		rec := &job.Phases[i]
		if rec.Status != joinPhaseStatusRunning {
			continue
		}
		if !rec.FinishedAt.IsZero() {
			continue
		}
		rec.FinishedAt = finishedAt
		rec.Status = status
		rec.DurationMs = maxInt64(0, finishedAt.Sub(rec.StartedAt).Milliseconds())
		rec.Error = errMsg
		return
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func markJoinPhaseRunning(job *clusterJoinJobDTO, phase string, startedAt time.Time) {
	if job == nil {
		return
	}
	phase = strings.ToUpper(strings.TrimSpace(phase))
	for i := len(job.Phases) - 1; i >= 0; i-- {
		rec := &job.Phases[i]
		if rec.Phase != phase {
			continue
		}
		if rec.FinishedAt.IsZero() {
			rec.Status = joinPhaseStatusRunning
			rec.StartedAt = startedAt
			rec.FinishedAt = time.Time{}
			rec.DurationMs = 0
			rec.Error = ""
			return
		}
		break
	}
	job.Phases = append(job.Phases, clusterJoinPhaseDTO{Phase: phase, Status: joinPhaseStatusRunning, StartedAt: startedAt})
}

func updateJoinCheckpoint(job *clusterJoinJobDTO, phase, status, lastCompleted, lastError string, resumable bool, updatedAt time.Time) {
	if job == nil {
		return
	}
	if job.Checkpoint == nil {
		job.Checkpoint = &clusterJoinCheckpointDTO{}
	}
	job.Checkpoint.Phase = strings.ToUpper(strings.TrimSpace(phase))
	job.Checkpoint.Status = strings.ToUpper(strings.TrimSpace(status))
	if strings.TrimSpace(lastCompleted) != "" {
		job.Checkpoint.LastCompletedPhase = strings.ToUpper(strings.TrimSpace(lastCompleted))
	}
	job.Checkpoint.Resumable = resumable
	job.Checkpoint.UpdatedAt = updatedAt
	job.Checkpoint.LastError = strings.TrimSpace(lastError)
}

func (s *HTTPServer) loadJoinDataset(ctx context.Context) ([]models.AddressPool, []models.StaticBinding, []models.Lease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	poolScope := pool.NewResourceScope(defaultAccessScopeID, systemTenantID)
	leaseScope := lease.NewResourceScope(defaultAccessScopeID, systemTenantID)
	pools, err := listAllPools(ctx, s.poolSvc, poolScope, 200)
	if err != nil {
		return nil, nil, nil, err
	}
	bindings, err := listAllBindings(ctx, s.poolSvc, poolScope, 200)
	if err != nil {
		return nil, nil, nil, err
	}
	leases, err := listAllLeases(ctx, s.leaseSvc, leaseScope, 200)
	if err != nil {
		return nil, nil, nil, err
	}
	return pools, bindings, leases, nil
}

func (s *HTTPServer) collectJoinDatasetSummary(ctx context.Context) (clusterJoinDatasetSummary, error) {
	pools, bindings, leases, err := s.loadJoinDataset(ctx)
	if err != nil {
		return clusterJoinDatasetSummary{}, err
	}
	if err := validateJoinDataset(clusterJoinDatasetSummary{}, pools, bindings, leases); err != nil {
		return clusterJoinDatasetSummary{}, err
	}
	return buildJoinDatasetSummary(pools, bindings, leases), nil
}

func buildJoinDatasetSummary(pools []models.AddressPool, bindings []models.StaticBinding, leases []models.Lease) clusterJoinDatasetSummary {
	sort.Slice(pools, func(i, j int) bool { return strings.Compare(pools[i].ID, pools[j].ID) < 0 })
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].PoolID != bindings[j].PoolID {
			return strings.Compare(bindings[i].PoolID, bindings[j].PoolID) < 0
		}
		if bindings[i].IPAddress != bindings[j].IPAddress {
			return strings.Compare(bindings[i].IPAddress, bindings[j].IPAddress) < 0
		}
		return strings.Compare(bindings[i].ID, bindings[j].ID) < 0
	})
	sort.Slice(leases, func(i, j int) bool {
		if leases[i].PoolID != leases[j].PoolID {
			return strings.Compare(leases[i].PoolID, leases[j].PoolID) < 0
		}
		if leases[i].IPAddress != leases[j].IPAddress {
			return strings.Compare(leases[i].IPAddress, leases[j].IPAddress) < 0
		}
		return strings.Compare(leases[i].ID, leases[j].ID) < 0
	})
	poolRows := make([]string, 0, len(pools))
	for _, item := range pools {
		poolRows = append(poolRows, strings.Join([]string{
			item.ID,
			item.Scope,
			ptrString(item.ParentID),
			item.CIDR,
			item.RangeStart,
			item.RangeEnd,
			item.Gateway,
			item.LeaseProfileID,
			item.AllocationMode,
			strconv.Itoa(item.PriorityWeight),
			item.Status,
			strconv.FormatInt(item.UpdatedAt.UTC().UnixMilli(), 10),
		}, "|"))
	}
	bindingRows := make([]string, 0, len(bindings))
	for _, item := range bindings {
		bindingRows = append(bindingRows, strings.Join([]string{
			item.ID,
			item.Identifier,
			item.IdentifierType,
			item.PoolID,
			item.IPAddress,
			item.LeaseProfileID,
			item.Status,
			ptrString(item.StatusSource),
			strconv.FormatInt(ptrTimeUnixMilli(item.LastSeenAt), 10),
			strconv.FormatInt(item.UpdatedAt.UTC().UnixMilli(), 10),
		}, "|"))
	}
	leaseRows := make([]string, 0, len(leases))
	activeLeaseCount := 0
	for _, item := range leases {
		if strings.EqualFold(strings.TrimSpace(item.State), "ACTIVE") {
			activeLeaseCount++
		}
		leaseRows = append(leaseRows, strings.Join([]string{
			item.ID,
			item.PoolID,
			item.IPAddress,
			item.HardwareAddr,
			item.ClientID,
			item.UserID,
			item.State,
			item.SecurityState,
			strconv.FormatInt(item.ExpiresAt.UTC().UnixMilli(), 10),
			strconv.FormatInt(item.UpdatedAt.UTC().UnixMilli(), 10),
		}, "|"))
	}
	poolChecksum := checksumRows(poolRows)
	bindingChecksum := checksumRows(bindingRows)
	leaseChecksum := checksumRows(leaseRows)
	return clusterJoinDatasetSummary{
		PoolCount:           len(pools),
		BindingCount:        len(bindings),
		LeaseCount:          len(leases),
		ActiveLeaseCount:    activeLeaseCount,
		PoolChecksum:        poolChecksum,
		BindingChecksum:     bindingChecksum,
		LeaseChecksum:       leaseChecksum,
		ConsistencyChecksum: checksumRows([]string{poolChecksum, bindingChecksum, leaseChecksum}),
	}
}

func validateJoinDataset(_ clusterJoinDatasetSummary, pools []models.AddressPool, bindings []models.StaticBinding, leases []models.Lease) error {
	poolIDs := make(map[string]struct{}, len(pools))
	for _, item := range pools {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("join consistency: pool id is empty")
		}
		if _, exists := poolIDs[id]; exists {
			return fmt.Errorf("join consistency: duplicate pool id %s", id)
		}
		poolIDs[id] = struct{}{}
	}
	bindingIDs := make(map[string]struct{}, len(bindings))
	for _, item := range bindings {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("join consistency: binding id is empty")
		}
		if _, exists := bindingIDs[id]; exists {
			return fmt.Errorf("join consistency: duplicate binding id %s", id)
		}
		bindingIDs[id] = struct{}{}
		if len(poolIDs) > 0 {
			if _, ok := poolIDs[strings.TrimSpace(item.PoolID)]; !ok {
				return fmt.Errorf("join consistency: binding %s references missing pool %s", id, strings.TrimSpace(item.PoolID))
			}
		}
	}
	activeLeaseKeys := make(map[string]struct{})
	for _, item := range leases {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("join consistency: lease id is empty")
		}
		if len(poolIDs) > 0 {
			if _, ok := poolIDs[strings.TrimSpace(item.PoolID)]; !ok {
				return fmt.Errorf("join consistency: lease %s references missing pool %s", id, strings.TrimSpace(item.PoolID))
			}
		}
		if strings.EqualFold(strings.TrimSpace(item.State), "ACTIVE") {
			key := strings.TrimSpace(item.PoolID) + "|" + strings.TrimSpace(item.IPAddress)
			if _, exists := activeLeaseKeys[key]; exists {
				return fmt.Errorf("join consistency: duplicate active lease for %s", strings.TrimSpace(item.IPAddress))
			}
			activeLeaseKeys[key] = struct{}{}
		}
	}
	return nil
}

func checksumRows(rows []string) string {
	hash := sha256.New()
	for _, row := range rows {
		hash.Write([]byte(row))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil)[:8])
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ptrTimeUnixMilli(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UTC().UnixMilli()
}

func listAllBindings(ctx context.Context, poolSvc *pool.Service, scopeRef pool.ResourceScope, pageSize int) ([]models.StaticBinding, error) {
	if poolSvc == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	offset := 0
	bindings := make([]models.StaticBinding, 0)
	for {
		batch, err := poolSvc.ListBindings(ctx, scopeRef, pool.BindingFilter{}, pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("join dataset bindings: %w", err)
		}
		bindings = append(bindings, batch...)
		if len(batch) < pageSize {
			break
		}
		offset += len(batch)
		if offset >= 5000 {
			break
		}
	}
	return bindings, nil
}

func listAllLeases(ctx context.Context, leaseSvc *lease.Service, scopeRef lease.ResourceScope, pageSize int) ([]models.Lease, error) {
	if leaseSvc == nil {
		return nil, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	offset := 0
	leases := make([]models.Lease, 0)
	for {
		batch, err := leaseSvc.ListLeases(ctx, scopeRef, "", pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("join dataset leases: %w", err)
		}
		leases = append(leases, batch...)
		if len(batch) < pageSize {
			break
		}
		offset += len(batch)
		if offset >= 5000 {
			break
		}
	}
	return leases, nil
}

func (s *HTTPServer) recordJoinSnapshot(jobID, nodeID, peerAddress, replicationOffset string) error {
	if s == nil {
		return fmt.Errorf("join snapshot: server unavailable")
	}
	summary, err := s.collectJoinDatasetSummary(context.Background())
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, ok := s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		snapshot := &clusterJoinSnapshotDTO{
			Status:              joinSnapshotStatusImported,
			CapturedAt:          now,
			SourceNode:          strings.TrimSpace(nodeID),
			SourceAddress:       strings.TrimSpace(peerAddress),
			PoolCount:           summary.PoolCount,
			BindingCount:        summary.BindingCount,
			LeaseCount:          summary.LeaseCount,
			PoolChecksum:        summary.PoolChecksum,
			BindingChecksum:     summary.BindingChecksum,
			LeaseChecksum:       summary.LeaseChecksum,
			ConsistencyChecksum: summary.ConsistencyChecksum,
			ReplicationOffset:   strings.TrimSpace(replicationOffset),
		}
		if tx, found := s.latestCommittedJoinSync(nodeID, peerAddress); found {
			snapshot.LastSyncTxID = tx.TxID
			snapshot.LastSyncType = tx.Type
			snapshot.LastSyncCommitAt = tx.CommitAt
		}
		v.Snapshot = snapshot
		if v.CatchUp == nil {
			v.CatchUp = &clusterJoinCatchUpDTO{}
		}
		v.CatchUp.Status = joinCatchUpStatusPending
		v.CatchUp.TargetOffset = strings.TrimSpace(replicationOffset)
		v.CatchUp.UpdatedAt = now
		v.UpdatedAt = now
	})
	if !ok {
		return fmt.Errorf("join snapshot: job %s not found", strings.TrimSpace(jobID))
	}
	return nil
}

func (s *HTTPServer) joinJobCatchUpTarget(jobID string) string {
	job, ok := s.getJoinJob(strings.TrimSpace(jobID))
	if !ok || job.CatchUp == nil {
		return ""
	}
	return strings.TrimSpace(job.CatchUp.TargetOffset)
}

func (s *HTTPServer) markJoinCatchUpStarted(jobID, baselineOffset, targetOffset string) {
	now := time.Now().UTC()
	_, _ = s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if v.CatchUp == nil {
			v.CatchUp = &clusterJoinCatchUpDTO{}
		}
		v.CatchUp.Status = joinCatchUpStatusRunning
		v.CatchUp.BaselineOffset = strings.TrimSpace(baselineOffset)
		if strings.TrimSpace(targetOffset) != "" {
			v.CatchUp.TargetOffset = strings.TrimSpace(targetOffset)
		}
		v.CatchUp.StartedAt = now
		v.CatchUp.UpdatedAt = now
		v.CatchUp.CompletedAt = time.Time{}
		v.CatchUp.HighWatermarkReached = false
		v.UpdatedAt = now
	})
}

func (s *HTTPServer) updateJoinCatchUpOffset(jobID, currentOffset string, reached bool) {
	now := time.Now().UTC()
	_, _ = s.updateJoinJob(jobID, func(v *clusterJoinJobDTO) {
		if v.CatchUp == nil {
			v.CatchUp = &clusterJoinCatchUpDTO{}
		}
		v.CatchUp.CurrentOffset = strings.TrimSpace(currentOffset)
		v.CatchUp.UpdatedAt = now
		if reached {
			v.CatchUp.Status = joinCatchUpStatusReached
			v.CatchUp.CompletedAt = now
			v.CatchUp.HighWatermarkReached = true
		}
		v.UpdatedAt = now
	})
}

func (s *HTTPServer) setJoinJobCancel(jobID string, cancel context.CancelFunc) {
	if cancel == nil {
		return
	}
	s.joinCancelsMu.Lock()
	if s.joinCancels == nil {
		s.joinCancels = make(map[string]context.CancelFunc)
	}
	s.joinCancels[jobID] = cancel
	s.joinCancelsMu.Unlock()
}

func (s *HTTPServer) clearJoinJobCancel(jobID string) {
	s.joinCancelsMu.Lock()
	delete(s.joinCancels, jobID)
	s.joinCancelsMu.Unlock()
}

func (s *HTTPServer) cancelJoinJobPipeline(jobID string) {
	s.joinCancelsMu.Lock()
	cancel, ok := s.joinCancels[jobID]
	if ok {
		delete(s.joinCancels, jobID)
	}
	s.joinCancelsMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

func (s *HTTPServer) countPendingJoinJobs() int {
	pending, _, _ := s.joinJobCounters()
	return pending
}

func (s *HTTPServer) joinJobCounters() (pending int, failed int, completed int) {
	s.joinJobsMu.RLock()
	defer s.joinJobsMu.RUnlock()
	pending = 0
	failed = 0
	completed = 0
	for _, job := range s.joinJobs {
		switch job.Status {
		case joinJobStateRegistered, joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying:
			pending++
		case joinJobStateFailed:
			failed++
		case joinJobStateWarmStandby, joinJobStateActive:
			completed++
		}
	}
	return pending, failed, completed
}

func (s *HTTPServer) joinSyncStatus() (clusterSyncStatusDTO, bool) {
	s.joinJobsMu.RLock()
	defer s.joinJobsMu.RUnlock()
	if len(s.joinJobs) == 0 {
		return clusterSyncStatusDTO{}, false
	}
	runningCount := 0
	failedCount := 0
	canceledCount := 0
	completedCount := 0
	var latestUpdate time.Time
	latencyMs := int64(0)

	for _, item := range s.joinJobs {
		switch item.Status {
		case joinJobStateRegistered, joinJobStateSnapshotting, joinJobStateCatchingUp, joinJobStateVerifying:
			runningCount++
		case joinJobStateFailed:
			failedCount++
		case joinJobStateCanceled:
			canceledCount++
		case joinJobStateWarmStandby, joinJobStateActive:
			completedCount++
		}
		if item.UpdatedAt.After(latestUpdate) {
			latestUpdate = item.UpdatedAt
		}
		if rec, ok := latestPhaseRecord(item); ok {
			candidate := rec.DurationMs
			if rec.Status == joinPhaseStatusRunning && !rec.StartedAt.IsZero() {
				candidate = time.Since(rec.StartedAt).Milliseconds()
			}
			if candidate > latencyMs {
				latencyMs = candidate
			}
		}
	}

	health := "健康"
	switch {
	case runningCount > 0:
		health = "同步中"
	case failedCount > 0:
		health = "异常"
	case canceledCount > 0 && completedCount == 0:
		health = "已取消"
	}

	latency := "-"
	if latencyMs > 0 {
		latency = formatMs(latencyMs)
	}

	last := "-"
	if !latestUpdate.IsZero() {
		last = humanizeAgo(latestUpdate)
	}
	if runningCount > 0 || failedCount > 0 || canceledCount > 0 || completedCount > 0 {
		last = last + " · 运行中:" + strconv.Itoa(runningCount) + " 失败:" + strconv.Itoa(failedCount) + " 完成:" + strconv.Itoa(completedCount)
	}

	return clusterSyncStatusDTO{
		Type:    "节点加入同步",
		Latency: latency,
		Last:    last,
		Health:  health,
	}, true
}

func latestPhaseRecord(job clusterJoinJobDTO) (clusterJoinPhaseDTO, bool) {
	if len(job.Phases) == 0 {
		return clusterJoinPhaseDTO{}, false
	}
	return job.Phases[len(job.Phases)-1], true
}

func humanizeAgo(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	d := time.Since(ts)
	if d < 0 {
		d = 0
	}
	sec := int64(d / time.Second)
	if sec < 60 {
		return "刚刚"
	}
	if sec < 3600 {
		return strconv.FormatInt(sec/60, 10) + " 分钟前"
	}
	if sec < 86400 {
		return strconv.FormatInt(sec/3600, 10) + " 小时前"
	}
	return strconv.FormatInt(sec/86400, 10) + " 天前"
}

func formatMs(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return strconv.FormatInt(ms, 10) + " ms"
}
