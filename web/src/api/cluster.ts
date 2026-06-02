import type {
  ClusterBackupPlan,
  ClusterBackupRecord,
  ClusterCommand,
  ClusterCommandListResponse,
  ClusterControl,
  ClusterFailoverEvent,
  ClusterFailoverPlan,
  ClusterHaConfig,
  ClusterInitializeRequest,
  ClusterInitializeResponse,
  ClusterJoinJob,
  ClusterJoinJobListResponse,
  ClusterJoinRequest,
  ClusterJoinResponse,
  ClusterMemberDetail,
  ClusterOverview,
  ClusterSyncStats,
  ClusterSyncTransaction,
  ClusterSyncStatus,
  ClusterNode
} from '@/types/cluster';
import { httpClient } from '@/shared/api-client/http';

export const getClusterOverview = () => httpClient.get<ClusterOverview>('/cluster/overview');

export const getClusterControl = () => httpClient.get<ClusterControl>('/cluster/control');

export const getClusterCommands = () => httpClient.get<ClusterCommandListResponse>('/cluster/commands');

export const createClusterCommand = (payload: {
  commandType: string;
  targetNodeId?: string;
  force?: boolean;
  payload?: Record<string, unknown>;
}) => httpClient.post<ClusterCommand>('/cluster/commands', payload);

export const getClusterMemberDetail = (memberId: string) =>
  httpClient.get<ClusterMemberDetail>(`/cluster/members/${encodeURIComponent(memberId)}`);

export const leaveClusterMember = (memberId: string, payload?: { force?: boolean; reason?: string }) =>
  httpClient.post<{ removed: boolean; command: ClusterCommand; cluster: ClusterControl }>(
    `/cluster/members/${encodeURIComponent(memberId)}/leave`,
    payload || {}
  );

export const initializeClusterControl = (payload: ClusterInitializeRequest) =>
  httpClient.post<ClusterInitializeResponse>('/cluster/initialize', payload);

export const joinClusterControl = (payload: ClusterJoinRequest) =>
  httpClient.post<ClusterJoinResponse>('/cluster/join', payload);

export const getClusterJoinJobs = () => httpClient.get<ClusterJoinJobListResponse>('/cluster/join-jobs');
export const getClusterJoinJob = (jobId: string) =>
  httpClient.get<ClusterJoinJob>(`/cluster/join-jobs/${encodeURIComponent(jobId)}`);
export const cancelClusterJoinJob = (jobId: string) =>
  httpClient.post(`/cluster/join-jobs/${encodeURIComponent(jobId)}/cancel`);
export const retryClusterJoinJob = (jobId: string) =>
  httpClient.post<ClusterJoinJob>(`/cluster/join-jobs/${encodeURIComponent(jobId)}/retry`);

export const deleteClusterControl = (payload: { forceDelete: boolean }) =>
  httpClient.post<{ deleted: boolean; cluster: ClusterControl }>('/cluster/delete', payload);

export const getClusterHaConfig = () => httpClient.get<ClusterHaConfig>('/cluster/ha/config');

export const saveClusterHaConfig = (payload: ClusterHaConfig) =>
  httpClient.put('/cluster/ha/config', payload);

export const createClusterNode = (payload: Partial<ClusterNode>) =>
  httpClient.post('/cluster/nodes', payload);

export const updateClusterNode = (id: string, payload: Partial<ClusterNode>) =>
  httpClient.put(`/cluster/nodes/${encodeURIComponent(id)}`, payload);

export const deleteClusterNode = (id: string) =>
  httpClient.delete(`/cluster/nodes/${encodeURIComponent(id)}`);

export const runClusterFailoverTest = () => httpClient.post('/cluster/ha-lb/failover-test');
export const getClusterFailoverHistory = () =>
  httpClient.get<ClusterFailoverEvent[]>('/cluster/ha-lb/failover-events');
export const getClusterFailoverPlans = () =>
  httpClient.get<{ items: ClusterFailoverPlan[]; count: number }>('/cluster/failover/plans');
export const getClusterFailoverPlan = (planId: string) =>
  httpClient.get<ClusterFailoverPlan>(`/cluster/failover/plans/${encodeURIComponent(planId)}`);
export const createClusterFailoverPlan = (payload?: {
  reason?: string;
  sourceNode?: string;
  targetNode?: string;
  dryRun?: boolean;
  simulateFailureStep?: string;
}) => httpClient.post<ClusterFailoverPlan>('/cluster/failover/plans', payload || {});

export const getHAMembershipEvents = () =>
  httpClient.get<{ items: ClusterFailoverEvent[]; count: number }>('/ha/membership-events');
export const upsertHAMember = (payload: {
  id?: string;
  address?: string;
  role?: string;
  region?: string;
  zone?: string;
  weight?: number;
  disabled?: boolean;
}) => httpClient.post('/ha/members', payload);
export const removeHAMember = (nodeId: string) =>
  httpClient.delete(`/ha/members/${encodeURIComponent(nodeId)}`);

export const triggerClusterSync = (payload?: {
  type?: string;
  sourceNode?: string;
  targetNode?: string;
  simulateFailure?: boolean;
}) => httpClient.post<ClusterFailoverEvent>('/cluster/sync/trigger', payload || {});
export const getClusterSyncStatus = () => httpClient.get<ClusterSyncStatus[]>('/cluster/sync/status');
export const getClusterSyncStats = (hours = 24) =>
  httpClient.get<ClusterSyncStats>('/cluster/sync/stats', { params: { hours } });
export const getClusterSyncTransactions = (limit = 20) =>
  httpClient.get<ClusterSyncTransaction[]>('/cluster/sync/transactions', { params: { limit } });
export const getClusterSyncTransaction = (txId: string) =>
  httpClient.get<ClusterSyncTransaction>(`/cluster/sync/transactions/${encodeURIComponent(txId)}`);
export const retryClusterSyncTransaction = (
  txId: string,
  payload?: { strategy?: string; simulateFailure?: boolean }
) => httpClient.post(`/cluster/sync/transactions/${encodeURIComponent(txId)}/retry`, payload || {});
export const getClusterScaleEvents = () => httpClient.get<ClusterFailoverEvent[]>('/cluster/scale/events');

// Backup & DR
export const getClusterBackupPlan = () => httpClient.get<ClusterBackupPlan>('/cluster/backup/plan');
export const saveClusterBackupPlan = (payload: ClusterBackupPlan) =>
  httpClient.put('/cluster/backup/plan', payload);
export const runClusterBackup = () => httpClient.post<ClusterBackupRecord>('/cluster/backup/run');
export const runClusterRestore = (payload: { time: string }) =>
  httpClient.post<ClusterBackupRecord>('/cluster/backup/restore', payload);
export const getClusterBackupHistory = () =>
  httpClient.get<ClusterBackupRecord[]>('/cluster/backup/history');
