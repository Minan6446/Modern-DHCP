import request from './request'
import type { ApiResponse } from '../types/api'
import type { DigHistoryItem, DnssecDebugResult, GlobalTestNode, IpLocationRow, ToolsModuleData } from '../types/modules'

type DigPayload = {
  domain: string
  recordType: string
  dnsServer?: string
}

type DigResult = {
  id: number
  output: string
  queriedAt: string
}

type GlobalTestPayload = {
  domain: string
  recordType: string
  nodeGroup: string
}

type GlobalTestResult = {
  nodes: GlobalTestNode[]
  historyId: number
}

type GlobalTestHistoryRow = {
  id: number
  domain: string
  recordType: string
  nodeGroup: string
  testedAt: string
}

// ── Module data ──────────────────────────────────────────────────────────────

export const getToolsModuleData = (): Promise<ApiResponse<ToolsModuleData>> => {
  return (request.get('/tools/dig/history') as Promise<ApiResponse<any>>).then((dig) => ({
    code: 0,
    message: 'ok',
    data: {
      dig: { history: (dig.data as any)?.rows ?? [] },
      dnssecDebug: { samples: {} },
      globalTest: { nodes: [] },
      ipLocation: { samples: {} },
    } as ToolsModuleData,
  }))
}

// ── Dig ───────────────────────────────────────────────────────────────────────

export const runDigQueryApi = (payload: DigPayload): Promise<ApiResponse<DigResult>> => {
  return request.post('/tools/dig', payload) as Promise<ApiResponse<DigResult>>
}

export const getDigHistoryApi = (params?: { domain?: string; page?: number; size?: number }): Promise<ApiResponse<{ total: number; rows: DigHistoryItem[] }>> => {
  return request.get('/tools/dig/history', { params }) as Promise<ApiResponse<{ total: number; rows: DigHistoryItem[] }>>
}

export const deleteDigHistoryApi = (id: number): Promise<ApiResponse<{ id: number }>> => {
  return request.delete(`/tools/dig/history/${id}`) as Promise<ApiResponse<{ id: number }>>
}

export const clearDigHistoryApi = (): Promise<ApiResponse<{ success: boolean }>> => {
  return request.delete('/tools/dig/history') as Promise<ApiResponse<{ success: boolean }>>
}

// ── DNSSEC Debug ──────────────────────────────────────────────────────────────

export const runDnssecDebugApi = (payload: { domain: string }): Promise<ApiResponse<DnssecDebugResult>> => {
  return request.post('/tools/dnssec-debug', payload) as Promise<ApiResponse<DnssecDebugResult>>
}

// ── Global Test ───────────────────────────────────────────────────────────────

export const runGlobalTestApi = (payload: GlobalTestPayload): Promise<ApiResponse<GlobalTestResult>> => {
  return request.post('/tools/global-test', payload, { timeout: 30000 }) as Promise<ApiResponse<GlobalTestResult>>
}

export const getGlobalTestHistoryApi = (params?: { domain?: string; page?: number; size?: number }): Promise<ApiResponse<{ total: number; rows: GlobalTestHistoryRow[] }>> => {
  return request.get('/tools/global-test/history', { params }) as Promise<ApiResponse<{ total: number; rows: GlobalTestHistoryRow[] }>>
}

// ── IP Location ───────────────────────────────────────────────────────────────

export const lookupIpLocationApi = (payload: { ips: string }): Promise<ApiResponse<IpLocationRow[]>> => {
  return request.post('/tools/ip-location', payload) as Promise<ApiResponse<IpLocationRow[]>>
}
