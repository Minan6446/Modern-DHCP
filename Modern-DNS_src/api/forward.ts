import request from './request'
import type { ApiResponse, IdResult, SuccessResult } from '../types/api'
import type { ForwardGlobalConfig, ForwardModuleData, ForwardRule, ForwardServer } from '../types/modules'

export type ForwardRuleSortOrder = 'ascending' | 'descending' | null

export interface ForwardRuleQuery {
  keyword?: string
  status?: string
  timeRange?: string[]
  sortProp?: keyof ForwardRule | ''
  sortOrder?: ForwardRuleSortOrder
  page?: number
  size?: number
}

export interface ForwardRuleListResult {
  list: ForwardRule[]
  total: number
}

export type ForwardRulePayload = Partial<ForwardRule> & {
  domains: string
  upstreamId: number
  priority: number
  remark: string
  status: string
}

type BatchStatusPayload = {
  ids: number[]
  status: string
}

type ReorderPayload = {
  orderedIds: number[]
}

export interface ForwardServerQuery {
  page?: number
  size?: number
}

export interface ForwardServerListResult {
  list: ForwardServer[]
  total: number
}

export type ForwardServerPayload = Partial<ForwardServer> & Omit<ForwardServer, 'id'>

type ForwardSwitchPayload = {
  enabled?: boolean
  publicDnsEnabled?: boolean
}

const compareRuleValue = (left: ForwardRule, right: ForwardRule, prop: keyof ForwardRule, order: Exclude<ForwardRuleSortOrder, null>): number => {
  const factor = order === 'ascending' ? 1 : -1
  if (prop === 'priority' || prop === 'upstreamId' || prop === 'id') {
    return (Number(left[prop] || 0) - Number(right[prop] || 0)) * factor
  }
  return String(left[prop] || '').localeCompare(String(right[prop] || '')) * factor
}

// ── Global config ─────────────────────────────────────────────────────────────

export const getForwardModuleData = (): Promise<ApiResponse<ForwardModuleData>> => {
  return request.get('/forward/global') as Promise<ApiResponse<ForwardModuleData>>
}

export const getGlobalForwardConfig = (): Promise<ApiResponse<ForwardGlobalConfig>> => {
  return request.get('/forward/global') as Promise<ApiResponse<ForwardGlobalConfig>>
}

export const saveGlobalForwardConfig = (payload: ForwardGlobalConfig): Promise<ApiResponse<ForwardGlobalConfig>> => {
  return request.put('/forward/global', payload) as Promise<ApiResponse<ForwardGlobalConfig>>
}

export const updateGlobalForwardSwitchApi = (payload: ForwardSwitchPayload): Promise<ApiResponse<SuccessResult & ForwardSwitchPayload>> => {
  return request.post('/forward/global/toggle', payload) as Promise<ApiResponse<SuccessResult & ForwardSwitchPayload>>
}

// ── Upstream servers ──────────────────────────────────────────────────────────

export const getForwardServerList = (params: ForwardServerQuery): Promise<ApiResponse<ForwardServerListResult>> => {
  return request.get('/forward/servers', { params }) as Promise<ApiResponse<ForwardServerListResult>>
}

export const saveForwardServerApi = (payload: ForwardServerPayload): Promise<ApiResponse<ForwardServer>> => {
  if (payload.id) {
    return request.put(`/forward/servers/${payload.id}`, payload) as Promise<ApiResponse<ForwardServer>>
  }
  return request.post('/forward/servers', payload) as Promise<ApiResponse<ForwardServer>>
}

export const deleteForwardServerApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/forward/servers/${id}`) as Promise<ApiResponse<IdResult>>
}

// ── Condition rules ───────────────────────────────────────────────────────────

export const getConditionRuleList = (params: ForwardRuleQuery): Promise<ApiResponse<ForwardRuleListResult>> => {
  return request.get('/forward/condition/rules', { params }) as Promise<ApiResponse<ForwardRuleListResult>>
}

export const saveConditionRuleApi = (payload: ForwardRulePayload): Promise<ApiResponse<ForwardRule>> => {
  if (payload.id) {
    return request.put(`/forward/condition/rules/${payload.id}`, payload) as Promise<ApiResponse<ForwardRule>>
  }
  return request.post('/forward/condition/rules', payload) as Promise<ApiResponse<ForwardRule>>
}

export const deleteConditionRuleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/forward/condition/rules/${id}`) as Promise<ApiResponse<IdResult>>
}

export const batchUpdateConditionRuleStatusApi = (payload: BatchStatusPayload): Promise<ApiResponse<BatchStatusPayload>> => {
  return request.put('/forward/condition/rules/batch-status', payload) as Promise<ApiResponse<BatchStatusPayload>>
}

export const batchDeleteConditionRulesApi = (ids: number[]): Promise<ApiResponse<{ ids: number[] }>> => {
  return request.delete('/forward/condition/rules/batch', { data: { ids } }) as Promise<ApiResponse<{ ids: number[] }>>
}

export const reorderConditionRulesApi = (payload: ReorderPayload): Promise<ApiResponse<ReorderPayload>> => {
  return request.put('/forward/condition/rules/reorder', payload) as Promise<ApiResponse<ReorderPayload>>
}

// ── Load Balance ─────────────────────────────────────────────────────────────

export const getLbGroups = (): Promise<ApiResponse<any[]>> => {
  return request.get('/forward/lb/groups') as Promise<ApiResponse<any[]>>
}

export const createLbGroup = (payload: { name: string; algorithm: string; healthCheckInterval: number; status?: string }): Promise<ApiResponse<any>> => {
  return request.post('/forward/lb/groups', payload) as Promise<ApiResponse<any>>
}

export const deleteLbGroup = (id: number): Promise<ApiResponse<SuccessResult>> => {
  return request.delete(`/forward/lb/groups/${id}`) as Promise<ApiResponse<SuccessResult>>
}

export const createLbServer = (groupId: number, payload: { name: string; address: string; port: number; protocol: string; weight: number; maxConns: number }): Promise<ApiResponse<any>> => {
  return request.post(`/forward/lb/groups/${groupId}/servers`, payload) as Promise<ApiResponse<any>>
}

export const updateLbServer = (sid: number, payload: { name: string; address: string; port: number; protocol: string; weight: number; maxConns: number }): Promise<ApiResponse<any>> => {
  return request.put(`/forward/lb/servers/${sid}`, payload) as Promise<ApiResponse<any>>
}

export const deleteLbServer = (sid: number): Promise<ApiResponse<SuccessResult>> => {
  return request.delete(`/forward/lb/servers/${sid}`) as Promise<ApiResponse<SuccessResult>>
}

export const toggleLbServer = (sid: number, enabled: boolean): Promise<ApiResponse<SuccessResult>> => {
  return request.put(`/forward/lb/servers/${sid}/toggle`, { enabled }) as Promise<ApiResponse<SuccessResult>>
}

export const lbHealthCheck = (groupId: number): Promise<ApiResponse<any[]>> => {
  return request.post(`/forward/lb/groups/${groupId}/health-check`) as Promise<ApiResponse<any[]>>
}

// ── Traffic stats ──────────────────────────────────────────────────────────

export interface TrafficBucket {
  hour: string
  total: number
  success: number
  fail: number
}

export interface TrafficStatsData {
  buckets: TrafficBucket[]
  total: number
  success: number
  fail: number
  successRate: number
}

export const getTrafficStatsApi = (): Promise<ApiResponse<TrafficStatsData>> => {
  return request.get('/forward/traffic-stats') as Promise<ApiResponse<TrafficStatsData>>
}

// ── Latency test ────────────────────────────────────────────────────────────

export interface LatencyTarget {
  id?: number
  label: string
  ip: string
  port?: number
  protocol?: string
}

export interface LatencyResult {
  id?: number
  label: string
  ip: string
  latencyMs: number | null
  status: 'done' | 'fail'
  error?: string
}

export interface LatencySummary {
  total: number
  online: number
  slow: number
  timeout: number
  avgLatency: number | null
}

export interface LatencyTestResponse {
  results: LatencyResult[]
  summary: LatencySummary
}

export const runLatencyTestApi = (
  targets: LatencyTarget[],
): Promise<ApiResponse<LatencyTestResponse>> => {
  return request.post('/forward/latency-test', { targets }) as Promise<ApiResponse<LatencyTestResponse>>
}