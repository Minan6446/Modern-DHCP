export interface ClusterNode {
  id: string;
  role: 'active' | 'standby' | 'worker' | string;
  address: string;
  self?: boolean;
  managed?: boolean;
  auth?: string;
  hasAuth?: boolean;
  disabled?: boolean;
  version: string;
  health: 'healthy' | 'warning' | 'critical' | string;
  cpuPercent: number;
  memoryPercent: number;
  activeLeasesRedis: number;
  totalLeasesMySQL: number;
  syncLagMs: number;
  lastHeartbeat: string;
}

export interface ClusterOverview {
  mode: 'active-active' | 'active-passive' | string;
  dhcpRole?: 'primary' | 'standby' | 'unknown' | string;
  dhcpHealth?: 'healthy' | 'warning' | 'critical' | string;
  failoverReady: boolean;
  replicationLagMs: number;
  replicationHealth?: 'healthy' | 'degraded' | 'error' | 'unknown' | string;
  replicationError?: string;
  replicationTx?: string;
  replicationMode?: string;
  replicationSource?: string;
  replicationState?: string;
  replicationUpdatedAt?: string;
  storageRoleAligned?: boolean;
  mysqlRole?: 'primary' | 'replica' | 'unknown' | string;
  mysqlHealth?: 'healthy' | 'warning' | 'critical' | 'degraded' | 'error' | 'unknown' | string;
  mysqlLagMs?: number;
  redisRole?: 'primary' | 'replica' | 'unknown' | string;
  redisHealth?: 'healthy' | 'warning' | 'critical' | 'degraded' | 'error' | 'unknown' | string;
  redisOffsetLag?: number;
  fencingEpoch?: string;
  writeGateOpen?: boolean;
  pendingActions: number;
  joinPending?: number;
  joinFailed?: number;
  joinCompleted?: number;
  nodes: ClusterNode[];
}

export interface ClusterControlNode {
  id: string;
  name?: string;
  url?: string;
  ipAddresses: string[];
  type: 'primary' | 'secondary' | string;
  state: 'self' | 'connected' | 'unreachable' | 'unknown' | string;
  version?: number;
  upSince?: string;
  lastSeen?: string;
  joinedAt?: string;
  updatedAt?: string;
}

export interface ClusterControl {
  initialized: boolean;
  clusterDomain?: string;
  primaryNodeId?: string;
  primaryNodeUrl?: string;
  primaryNodeIpAddresses: string[];
  heartbeatIntervalSeconds: number;
  heartbeatRetryIntervalSeconds: number;
  configRefreshIntervalSeconds: number;
  configRetryIntervalSeconds: number;
  configVersion: number;
  requireTls: boolean;
  apiTlsEnabled: boolean;
  apiTlsActive: boolean;
  apiTlsCertFile?: string;
  apiTlsClientCaFile?: string;
  restartRequired: boolean;
  nodes: ClusterControlNode[];
}

export interface ClusterCommandReceipt {
  nodeId: string;
  status: 'QUEUED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | string;
  detail?: string;
  ackedAt?: string;
  updatedAt?: string;
  completedAt?: string;
  error?: string;
}

export interface ClusterCommand {
  commandId: string;
  commandType: 'sync' | 'verify' | 'leave' | string;
  targetNodeId?: string;
  requestedBy?: string;
  status: 'QUEUED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | string;
  force?: boolean;
  payload?: Record<string, unknown>;
  requestedAt: string;
  updatedAt: string;
  completedAt?: string;
  receipts: ClusterCommandReceipt[];
}

export interface ClusterCommandListResponse {
  items: ClusterCommand[];
  count: number;
}

export interface ClusterMemberDetail {
  node: ClusterControlNode;
  runtimeNode?: ClusterNode;
  latestJoinJob?: ClusterJoinJob;
  recentCommands: ClusterCommand[];
}

export interface ClusterInitializeRequest {
  clusterDomain: string;
  primaryNodeId?: string;
  primaryNodeUrl?: string;
  primaryNodeIpAddresses: string[];
  heartbeatIntervalSeconds?: number;
  heartbeatRetryIntervalSeconds?: number;
  configRefreshIntervalSeconds?: number;
  configRetryIntervalSeconds?: number;
  forceReinitialize?: boolean;
}

export interface ClusterInitializeResponse {
  clusterToken: string;
  cluster: ClusterControl;
  tlsMaterialGenerated: boolean;
  restartRequired: boolean;
}

export interface ClusterJoinRequest {
  secondaryNodeId: string;
  secondaryNodeName?: string;
  secondaryNodeUrl: string;
  secondaryNodeIpAddresses: string[];
  secondaryNodeCertificate?: string;
  clusterToken: string;
}

export interface ClusterJoinResponse {
  nodeId: string;
  nodeToken: string;
  joinJobId?: string;
  cluster: ClusterControl;
}

export interface ClusterJoinPhase {
  phase: string;
  status: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'CANCELED' | string;
  startedAt: string;
  finishedAt?: string;
  durationMs?: number;
  error?: string;
}

export interface ClusterJoinVerification {
  status: 'VERIFIED' | 'FAILED' | string;
  checkedAt?: string;
  poolCount?: number;
  bindingCount?: number;
  leaseCount?: number;
  activeLeaseCount?: number;
  conflictLeaseCount24h?: number;
  createdLeaseCount24h?: number;
  fencingEpoch?: string;
  replicationHealthy: boolean;
  replicationLagMs?: number;
  replicationMode?: string;
  replicationSource?: string;
  replicationState?: string;
  replicationOffset?: string;
  lastSyncTxId?: string;
  lastSyncType?: string;
  lastSyncCommitAt?: string;
  consistencyChecksum?: string;
  warning?: string;
  failureReason?: string;
}

export interface ClusterJoinSnapshot {
  status?: string;
  capturedAt?: string;
  sourceNode?: string;
  sourceAddress?: string;
  poolCount?: number;
  bindingCount?: number;
  leaseCount?: number;
  poolChecksum?: string;
  bindingChecksum?: string;
  leaseChecksum?: string;
  consistencyChecksum?: string;
  replicationOffset?: string;
  lastSyncTxId?: string;
  lastSyncType?: string;
  lastSyncCommitAt?: string;
}

export interface ClusterJoinCatchUp {
  status?: string;
  baselineOffset?: string;
  targetOffset?: string;
  currentOffset?: string;
  startedAt?: string;
  updatedAt?: string;
  completedAt?: string;
  highWatermarkReached?: boolean;
}

export interface ClusterJoinCheckpoint {
  phase?: string;
  status?: string;
  lastCompletedPhase?: string;
  resumeCount?: number;
  resumable?: boolean;
  updatedAt?: string;
  lastError?: string;
}

export interface ClusterJoinJob {
  jobId: string;
  nodeId: string;
  peerAddress?: string;
  status: 'REGISTERED' | 'SNAPSHOTTING' | 'CATCHING_UP' | 'VERIFYING' | 'WARM_STANDBY' | 'ACTIVE' | 'FAILED' | 'CANCELED' | string;
  progress: number;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
  error?: string;
  createdBy?: string;
  phases: ClusterJoinPhase[];
  snapshot?: ClusterJoinSnapshot;
  catchUp?: ClusterJoinCatchUp;
  checkpoint?: ClusterJoinCheckpoint;
  verification?: ClusterJoinVerification;
}

export interface ClusterJoinJobListResponse {
  items: ClusterJoinJob[];
  count: number;
}

export interface ClusterHaConfig {
  mode: 'active-passive' | 'active-active' | string;
  primary: string;
  standbyNodes: string[];
  loadBalancing: {
    enabled: boolean;
    virtualIp?: string;
    balancer?: string;
    method?: string;
  };
  failover: {
    intervalMs: number;
    timeoutMs: number;
    failureThreshold: number;
  };
  replication: {
    mechanism: string;
    snapshotIntervalSec: number;
    lagAlertMs: number;
    syncMode: string;
  };
  splitBrain: {
    strategy: string;
    arbiter?: string;
    fenceScript?: string;
    sharedLock?: string;
  };
  recovery: {
    manualAction?: string;
  };
}

export interface ClusterFailoverEvent {
  id: string;
  time: string;
  title: string;
  detail: string;
  status: 'success' | 'failed' | 'running';
  type: 'success' | 'warning' | 'info' | 'danger';
  statusLabel?: string;
  txId?: string;
  phase?: string;
  ackAt?: string;
  errorCode?: string;
}

export interface ClusterFailoverPlanStep {
  id: string;
  title: string;
  status: 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED' | string;
  detail?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
  error?: string;
  rollback?: boolean;
}

export interface ClusterFailoverPlan {
  planId: string;
  status: 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'ROLLED_BACK' | string;
  reason: string;
  dryRun: boolean;
  sourceNode?: string;
  targetNode?: string;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
  failedStep?: string;
  rollbackTriggered: boolean;
  rollbackStatus?: string;
  steps: ClusterFailoverPlanStep[];
}

export interface ClusterSyncStatus {
  type: string;
  latency: string;
  last: string;
  health: string;
  mysqlLagMs?: number;
  mysqlLagLevel?: 'healthy' | 'warning' | 'critical' | string;
  mysqlLagWarnMs?: number;
  mysqlLagCriticalMs?: number;
  redisOffsetLag?: number;
  redisOffsetLagLevel?: 'healthy' | 'warning' | 'critical' | string;
  redisOffsetLagWarn?: number;
  redisOffsetLagCritical?: number;
  rfc6853AckLatencyMs?: number;
  rfc6853AckLatencyLevel?: 'healthy' | 'warning' | 'critical' | string;
  rfc6853AckWarnMs?: number;
  rfc6853AckCriticalMs?: number;
  transport?: string;
  peerAddress?: string;
  responderUp?: boolean;
  responderErr?: string;
  lastTxId?: string;
  phase?: string;
  sourceNode?: string;
  targetNode?: string;
  bndupdAt?: string;
  bndackAt?: string;
  ackAt?: string;
  errorCode?: string;
  errorMessage?: string;
  reconcileLastRun?: string;
  reconcileLastErr?: string;
  reconcileSuccessTotal?: number;
  reconcileFailureTotal?: number;
  reconcileFailureStreak?: number;
  reconcileIntervalSec?: number;
  reconcileAlertThreshold?: number;
}

export interface ClusterSyncTransaction {
  txId: string;
  type: string;
  sourceNode: string;
  targetNode: string;
  retryOf?: string;
  retryStrategy?: string;
  phase: string;
  createdAt: string;
  bndupdAt?: string;
  bndackAt?: string;
  ackAt?: string;
  commitAt?: string;
  latencyMs?: number;
  errorCode?: string;
  errorMessage?: string;
}

export interface ClusterSyncStats {
  windowHours: number;
  total: number;
  committed: number;
  failed: number;
  ackTimeoutCount: number;
  successRatePercent: number;
  avgAckLatencyMs: number;
  p95AckLatencyMs: number;
}

export interface ClusterBackupPlan {
  freq: string;
  retain: string;
  storage: string;
  contents: string[];
}

export interface ClusterBackupRecord {
  time: string;
  type: string;
  size: string;
  status: string;
  txId?: string;
  phase?: string;
  ackAt?: string;
  errorCode?: string;
}
