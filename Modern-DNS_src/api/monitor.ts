import request from './request'
import type { ApiResponse, SuccessResult } from '../types/api'
import type {
  MonitorModuleData,
  MonitorRealTimeRow,
  MonitorResolveLogRow,
  MonitorReportData,
  MonitorRuleNxdomain,
  MonitorRuleQps,
  MonitorRuleLatency,
  MonitorRuleCacheHit,
} from '../types/modules'

// ── Module data ──────────────────────────────────────────────────────────────

export const getMonitorModuleData = (): Promise<ApiResponse<MonitorModuleData>> => {
  return Promise.all([
    request.get('/monitor/realtime') as Promise<ApiResponse<any>>,
    request.get('/monitor/resolve-logs') as Promise<ApiResponse<any>>,
    request.get('/monitor/rules') as Promise<ApiResponse<any>>,
    request.get('/monitor/report') as Promise<ApiResponse<any>>,
  ]).then(([rt, rl, rules, rep]) => ({
    code: 0,
    message: 'ok',
    data: {
      realTime: (rt.data as any)?.rows ?? [],
      resolveLogs: (rl.data as any)?.rows ?? [],
      rules: {
        qps: (rules.data as any)?.qps ?? {},
        nxdomain: (rules.data as any)?.nxdomain ?? {},
        latency: (rules.data as any)?.latency ?? {},
        cacheHit: (rules.data as any)?.cacheHit ?? {},
        history: (rules.data as any)?.history ?? [],
      },
      report: rep.data ?? {},
    } as MonitorModuleData,
  }))
}

// ── Real-time ─────────────────────────────────────────────────────────────────

export const getRealTimeLogsApi = (params?: {
  domain?: string; sourceIp?: string; status?: string; recordType?: string; page?: number; size?: number
}): Promise<ApiResponse<{ total: number; rows: MonitorRealTimeRow[] }>> => {
  return request.get('/monitor/realtime', { params }) as Promise<ApiResponse<{ total: number; rows: MonitorRealTimeRow[] }>>
}

export const refreshMonitorRealtime = (): Promise<ApiResponse<SuccessResult>> => {
  return request.post('/monitor/realtime/refresh') as Promise<ApiResponse<SuccessResult>>
}

// ── Resolve Logs ──────────────────────────────────────────────────────────────

export const getResolveLogsApi = (params?: {
  page?: number; size?: number; domain?: string; sourceIp?: string; status?: string
  recordType?: string; startTime?: string; endTime?: string; keyword?: string
}): Promise<ApiResponse<{ total: number; rows: MonitorResolveLogRow[] }>> => {
  return request.get('/monitor/resolve-logs', { params }) as Promise<ApiResponse<{ total: number; rows: MonitorResolveLogRow[] }>>
}

export const exportResolveLogsApi = (payload: { format?: string; scope?: string; domain?: string; timeRange?: string[] }): Promise<ApiResponse<{ success: boolean; exportedAt: string }>> => {
  return request.post('/monitor/resolve-logs/export', payload) as Promise<ApiResponse<{ success: boolean; exportedAt: string }>>
}

// ── Rules ─────────────────────────────────────────────────────────────────────

export const saveQpsRuleApi = (payload: MonitorRuleQps): Promise<ApiResponse<MonitorRuleQps>> => {
  return request.put('/monitor/rules/qps', payload) as Promise<ApiResponse<MonitorRuleQps>>
}

export const saveNxdomainRuleApi = (payload: MonitorRuleNxdomain): Promise<ApiResponse<MonitorRuleNxdomain>> => {
  return request.put('/monitor/rules/nxdomain', payload) as Promise<ApiResponse<MonitorRuleNxdomain>>
}

export const resetQpsRuleApi = (): Promise<ApiResponse<MonitorRuleQps>> => {
  return request.post('/monitor/rules/qps/reset') as Promise<ApiResponse<MonitorRuleQps>>
}

export const resetNxdomainRuleApi = (): Promise<ApiResponse<MonitorRuleNxdomain>> => {
  return request.post('/monitor/rules/nxdomain/reset') as Promise<ApiResponse<MonitorRuleNxdomain>>
}

export const handleRuleHistoryApi = (id: number): Promise<ApiResponse<{ id: number; success: boolean }>> => {
  return request.patch(`/monitor/rule-history/${id}/handle`) as Promise<ApiResponse<{ id: number; success: boolean }>>
}

export const saveLatencyRuleApi = (payload: MonitorRuleLatency): Promise<ApiResponse<MonitorRuleLatency>> => {
  return request.put('/monitor/rules/latency', payload) as Promise<ApiResponse<MonitorRuleLatency>>
}

export const resetLatencyRuleApi = (): Promise<ApiResponse<MonitorRuleLatency>> => {
  return request.post('/monitor/rules/latency/reset') as Promise<ApiResponse<MonitorRuleLatency>>
}

export const saveCacheHitRuleApi = (payload: MonitorRuleCacheHit): Promise<ApiResponse<MonitorRuleCacheHit>> => {
  return request.put('/monitor/rules/cache-hit', payload) as Promise<ApiResponse<MonitorRuleCacheHit>>
}

export const resetCacheHitRuleApi = (): Promise<ApiResponse<MonitorRuleCacheHit>> => {
  return request.post('/monitor/rules/cache-hit/reset') as Promise<ApiResponse<MonitorRuleCacheHit>>
}

// ── Report ────────────────────────────────────────────────────────────────────

export const getMonitorReportApi = (params?: { startTime?: string; endTime?: string; domain?: string }): Promise<ApiResponse<MonitorReportData>> => {
  return request.get('/monitor/report', { params }) as Promise<ApiResponse<MonitorReportData>>
}

// ── Alert Subscribe ──────────────────────────────────────────────────────────

export const getAlertSubscribeRules = (): Promise<ApiResponse<any[]>> => {
  return request.get('/monitor/alert-subscribe/rules') as Promise<ApiResponse<any[]>>
}

export const createAlertSubscribeRule = (payload: any): Promise<ApiResponse<any>> => {
  return request.post('/monitor/alert-subscribe/rules', payload) as Promise<ApiResponse<any>>
}

export const updateAlertSubscribeRule = (id: number, payload: any): Promise<ApiResponse<any>> => {
  return request.put(`/monitor/alert-subscribe/rules/${id}`, payload) as Promise<ApiResponse<any>>
}

export const deleteAlertSubscribeRule = (id: number): Promise<ApiResponse<any>> => {
  return request.delete(`/monitor/alert-subscribe/rules/${id}`) as Promise<ApiResponse<any>>
}

export const toggleAlertSubscribeRule = (id: number, status: string): Promise<ApiResponse<any>> => {
  return request.patch(`/monitor/alert-subscribe/rules/${id}/toggle`, { status }) as Promise<ApiResponse<any>>
}

// ── Slow Query ───────────────────────────────────────────────────────────────

export const getSlowQueries = (params?: { threshold?: number; keyword?: string; queryType?: string; range?: string }): Promise<ApiResponse<any>> => {
  return request.get('/monitor/slow-query', { params }) as Promise<ApiResponse<any>>
}

// ── Client Analysis ──────────────────────────────────────────────────────────

export const getClientAnalysis = (params?: { range?: string; keyword?: string; region?: string }): Promise<ApiResponse<any>> => {
  return request.get('/monitor/client-analysis', { params }) as Promise<ApiResponse<any>>
}
