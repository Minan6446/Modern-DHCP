import request from './request'
import type { ApiResponse } from '../types/api'

// ── Types ─────────────────────────────────────────────────────────────────────

// ClusterNode is the merged view returned by the backend: static identity
// columns come from the cluster_nodes table; live metrics (status / cpu /
// mem / qps / syncLag / lastHeartbeat) are resolved server-side from the
// in-memory cluster runtime registry and are NOT persisted in MySQL.
export interface ClusterNode {
  // ── persisted (cluster_nodes) ──
  id: number
  nodeId?: string
  name: string
  ip: string
  ipAddresses?: string
  url?: string
  port: number
  role: string
  zone: string
  version: string
  joinedAt: string
  // ── runtime (pkg/cluster.Runtime; refreshed on each request) ──
  status: string
  state?: 'Self' | 'Connected' | 'Unreachable' | 'Unknown' | string
  cpuUsage: number
  memUsage: number
  qps: number
  syncLag: number
  upSince?: string
  lastHeartbeat: string
  lastSeen?: string
  // ── operational toggle ── true when the node is in maintenance and
  // intentionally excluded from sync rotation (see drainNodeApi).
  drained?: boolean
}

export interface ClusterStats {
  online: number
  offline: number
  syncIssues: number
  totalQps: number
  avgCpu: number
  avgMem: number
  total: number
}

export interface TrendPoint {
  time: string
  qps: number
  latency: number
}

export interface ClusterOverview {
  nodes: ClusterNode[]
  stats: ClusterStats
  trend: TrendPoint[]
}

export interface ConfigSyncRow {
  id: number
  nodeId: number
  name: string
  ip: string
  role: string
  zone: string
  configVersion: string
  masterVersion: string
  syncStatus: string
  lastSyncAt: string
  diffCount: number
}

export interface ConfigDiffItem {
  key: string
  label: string
  masterValue: string
  nodeValue: string
  changed: boolean
}

// ── Overview ──────────────────────────────────────────────────────────────────

export const getClusterOverviewApi = (range?: string): Promise<ApiResponse<ClusterOverview>> => {
  return request.get('/cluster/overview', { params: range ? { range } : undefined }) as Promise<ApiResponse<ClusterOverview>>
}

// ── Node Management ───────────────────────────────────────────────────────────

export const listClusterNodesApi = (params?: {
  keyword?: string; role?: string; status?: string; page?: number; size?: number
}): Promise<ApiResponse<{ total: number; list: ClusterNode[] }>> => {
  return request.get('/cluster/nodes', { params }) as Promise<ApiResponse<{ total: number; list: ClusterNode[] }>>
}

export interface AddClusterNodeResult {
  node: ClusterNode
  joinHint: string
}

export const addClusterNodeApi = (payload: { name: string; ip: string; port?: number; role?: string; zone: string }): Promise<ApiResponse<AddClusterNodeResult>> => {
  return request.post('/cluster/nodes', payload) as Promise<ApiResponse<AddClusterNodeResult>>
}

export const updateClusterNodeApi = (id: number, payload: Partial<ClusterNode>): Promise<ApiResponse<ClusterNode>> => {
  return request.put(`/cluster/nodes/${id}`, payload) as Promise<ApiResponse<ClusterNode>>
}

export const removeClusterNodeApi = (id: number): Promise<ApiResponse<{ id: number }>> => {
  return request.delete(`/cluster/nodes/${id}`) as Promise<ApiResponse<{ id: number }>>
}

export const refreshClusterNodesApi = (): Promise<ApiResponse<{ success: boolean; refreshedAt: string }>> => {
  return request.post('/cluster/nodes/refresh') as Promise<ApiResponse<{ success: boolean; refreshedAt: string }>>
}

export const failoverClusterNodeApi = (id: number): Promise<ApiResponse<ClusterNode>> => {
  return request.post(`/cluster/nodes/${id}/failover`) as Promise<ApiResponse<ClusterNode>>
}

// ── Config Sync ───────────────────────────────────────────────────────────────

export const listConfigSyncApi = (params?: {
  keyword?: string; syncStatus?: string; page?: number; size?: number
}): Promise<ApiResponse<{ total: number; list: ConfigSyncRow[]; masterVersion: string }>> => {
  return request.get('/cluster/config-sync', { params }) as Promise<ApiResponse<{ total: number; list: ConfigSyncRow[]; masterVersion: string }>>
}

export const syncNodeConfigApi = (id: number): Promise<ApiResponse<ConfigSyncRow>> => {
  return request.post(`/cluster/config-sync/${id}/sync`) as Promise<ApiResponse<ConfigSyncRow>>
}

export const syncAllNodeConfigsApi = (): Promise<ApiResponse<{ success: boolean; syncedAt: string }>> => {
  return request.post('/cluster/config-sync/sync-all') as Promise<ApiResponse<{ success: boolean; syncedAt: string }>>
}

export const getConfigDiffApi = (id: number): Promise<ApiResponse<ConfigDiffItem[]>> => {
  return request.get(`/cluster/config-sync/${id}/diff`) as Promise<ApiResponse<ConfigDiffItem[]>>
}

// Live diff: makes the primary pull the secondary's current snapshot,
// recompute the per-table comparison, and persist it. The cached GET
// /diff endpoint above only returns whatever was last computed; call
// this when the operator clicks the "刷新差异" button.
export const refreshConfigDiffApi = (id: number): Promise<ApiResponse<ConfigDiffItem[]>> => {
  return request.post(`/cluster/config-sync/${id}/diff/refresh`) as Promise<ApiResponse<ConfigDiffItem[]>>
}

// ── Drain / batch ops / token rotation / per-node metrics ─────────────────────

export const drainNodeApi = (id: number, drained: boolean): Promise<ApiResponse<{ id: number; drained: boolean }>> => {
  const path = drained ? `/cluster/nodes/${id}/drain` : `/cluster/nodes/${id}/undrain`
  return request.post(path) as Promise<ApiResponse<{ id: number; drained: boolean }>>
}

export const batchRemoveNodesApi = (ids: number[]): Promise<ApiResponse<{ removed: number; names: string[] }>> => {
  return request.post('/cluster/nodes/batch-remove', { ids }) as Promise<ApiResponse<{ removed: number; names: string[] }>>
}

export interface RotateTokenResp {
  apiToken: string
  graceSec: number
  rotatedAt: string
  reminder: string
}
export const rotateClusterTokenApi = (graceSec = 60): Promise<ApiResponse<RotateTokenResp>> => {
  return request.post('/cluster/rotate-token', { graceSec }) as Promise<ApiResponse<RotateTokenResp>>
}

export interface NodeMetricSample {
  at: string
  cpuUsage: number
  memUsage: number
  qps: number
  syncLag: number
}
export const getNodeMetricsApi = (id: number): Promise<ApiResponse<{ nodeId: string; name: string; samples: NodeMetricSample[] }>> => {
  return request.get(`/cluster/nodes/${id}/metrics`) as Promise<ApiResponse<{ nodeId: string; name: string; samples: NodeMetricSample[] }>>
}

// ── SSE event stream ──────────────────────────────────────────────────────────
//
// Returns a live EventSource subscribed to /api/cluster/stream. Pages can
// listen for "node-state" / "node-joined" / "node-left" / "sync-finished"
// and refetch their own data on demand instead of polling. Caller must
// close() the EventSource on unmount.
export const openClusterEventStream = (): EventSource => {
  // Cookies are not used for auth; we need to pass JWT as query param
  // because EventSource doesn't allow custom headers. The backend admits
  // the same JWT either as Authorization header or `?token=` query.
  const token = localStorage.getItem('modern-dns-token') ?? ''
  const url = `/api/cluster/stream${token ? `?token=${encodeURIComponent(token)}` : ''}`
  return new EventSource(url, { withCredentials: false })
}

// ── Sync history ──────────────────────────────────────────────────────────────

export interface SyncHistoryRow {
  id: number
  version: string
  startedAt: string
  finishedAt: string
  totalNodes: number
  successCount: number
  failedCount: number
  trigger: 'manual' | 'manual-all' | 'auto' | 'retry'
  triggeredBy: string
  notes: string
  detail: Array<{ nodeId: string; name: string; url: string; ok: boolean; message?: string }>
}

export const listSyncHistoryApi = (params?: {
  keyword?: string; trigger?: string; page?: number; size?: number
}): Promise<ApiResponse<{ total: number; list: SyncHistoryRow[] }>> => {
  return request.get('/cluster/sync-history', { params }) as Promise<ApiResponse<{ total: number; list: SyncHistoryRow[] }>>
}

// ── Cluster bootstrap & control plane ─────────────────────────────────────────

export interface ClusterStateNode {
  id: string
  name: string
  url: string
  ipAddresses: string[]
  type: 'Primary' | 'Secondary'
  state: 'Self' | 'Connected' | 'Unreachable' | 'Unknown'
  version: string
  upSince: string
  lastSeen: string
}

export interface ClusterStatePayload {
  initialized: boolean
  clusterDomain: string
  heartbeatIntervalSec: number
  configRefreshSec: number
  configVersion: string
  nodes: ClusterStateNode[]
}

export interface InitializeClusterPayload {
  clusterDomain: string
  primaryNodeIpAddresses: string[]
  heartbeatIntervalSec?: number
  configRefreshSec?: number
}

export interface InitializeClusterResult {
  clusterDomain: string
  apiToken: string
  heartbeatIntervalSec: number
  configRefreshSec: number
  configVersion: string
}

export interface ResyncResult {
  configVersion: string
  syncedAt: string
  results: { nodeId: string; name: string; url: string; ok: boolean; message?: string }[]
}

export interface ClusterCommandResult {
  action: string
  success: number
  failed: number
  results: { nodeId: string; name: string; url: string; ok: boolean; message?: string }[]
}

export const getClusterStateApi = (): Promise<ApiResponse<ClusterStatePayload>> => {
  return request.get('/cluster/state') as Promise<ApiResponse<ClusterStatePayload>>
}

export const initializeClusterApi = (
  payload: InitializeClusterPayload,
): Promise<ApiResponse<InitializeClusterResult>> => {
  return request.post('/cluster/initialize', payload) as Promise<ApiResponse<InitializeClusterResult>>
}

export const deleteClusterApi = (force = false): Promise<ApiResponse<{ deleted: boolean; force: boolean }>> => {
  return request.delete('/cluster', { params: { force: force ? 1 : 0 } }) as Promise<ApiResponse<{ deleted: boolean; force: boolean }>>
}

export const resyncClusterApi = (): Promise<ApiResponse<ResyncResult>> => {
  return request.post('/cluster/resync') as Promise<ApiResponse<ResyncResult>>
}

export const dispatchClusterCommandApi = (
  action: 'forceUpdateBlockLists' | 'temporaryDisableBlocking' | 'notify',
  args?: Record<string, unknown>,
): Promise<ApiResponse<ClusterCommandResult>> => {
  return request.post('/cluster/command', { action, args }) as Promise<ApiResponse<ClusterCommandResult>>
}
