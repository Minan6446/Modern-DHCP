import request from './request'
import type { ApiResponse, IdResult, SuccessResult } from '../types/api'
import type { DashboardAlertRule, DashboardModuleData } from '../types/modules'

export type DashboardAlertRuleQuery = {
  keyword?: string
  alertType?: string
  status?: string
  page?: number
  size?: number
}

export type DashboardAlertRuleListResult = {
  list: DashboardAlertRule[]
  total: number
}

// ── Overview ──────────────────────────────────────────────────────────────────

export const getOverview = (range = 'today'): Promise<ApiResponse<DashboardModuleData['overview']['today']>> => {
  return request.get('/dashboard/overview', { params: { range } }) as any
}

// ── Domain Status ─────────────────────────────────────────────────────────────

export const getDomainStatus = (): Promise<ApiResponse<DashboardModuleData['domainStatus']>> => {
  return request.get('/dashboard/domain-status') as any
}

// ── Alerts ────────────────────────────────────────────────────────────────────

export const getAlerts = (): Promise<ApiResponse<DashboardModuleData['alerts']>> => {
  return request.get('/dashboard/alerts') as any
}

// ── Resource ──────────────────────────────────────────────────────────────────

export const getResource = (dim = 'day'): Promise<ApiResponse<DashboardModuleData['resource']>> => {
  return request.get('/dashboard/resource', { params: { dim } }) as any
}

// ── Security Posture ─────────────────────────────────────────────────────────
//
// Aggregator endpoint that powers the dashboard "security at a glance"
// card: locked accounts, online sessions, kicks in last 24h,
// password-overdue users, EDNS padding counters. One round-trip.
export interface SecurityPosture {
  lockedAccounts: number
  activeSessions: number
  onlineUsers: number
  kickedLast24h: number
  overduePasswordUsers: number
  paddingInbound: number
  paddingOutbound: number
  sampledAt: string
}

export const getSecurityPosture = (): Promise<ApiResponse<SecurityPosture>> => {
  return request.get('/dashboard/security-posture') as Promise<ApiResponse<SecurityPosture>>
}

// ── Alert Rules ───────────────────────────────────────────────────────────────

export const getAlertRuleList = (params: DashboardAlertRuleQuery): Promise<ApiResponse<DashboardAlertRuleListResult>> => {
  return request.get('/dashboard/alert-rules', { params }) as Promise<ApiResponse<DashboardAlertRuleListResult>>
}

export const addAlertRule = (payload: Partial<DashboardAlertRule>): Promise<ApiResponse<DashboardAlertRule>> => {
  return request.post('/dashboard/alert-rules', payload) as Promise<ApiResponse<DashboardAlertRule>>
}

export const editAlertRule = (payload: Partial<DashboardAlertRule>): Promise<ApiResponse<DashboardAlertRule>> => {
  return request.put(`/dashboard/alert-rules/${payload.id}`, payload) as Promise<ApiResponse<DashboardAlertRule>>
}

export const deleteAlertRule = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/dashboard/alert-rules/${id}`) as Promise<ApiResponse<IdResult>>
}

export const testAlertRuleApi = (payload: { id: number }): Promise<ApiResponse<SuccessResult & { id: number; sentAt: string }>> => {
  return request.post(`/dashboard/alert-rules/${payload.id}/test`, payload) as Promise<ApiResponse<SuccessResult & { id: number; sentAt: string }>>
}

// ── Alert Events ──────────────────────────────────────────────────────────────

export const handleAlertEvent = (id: number): Promise<ApiResponse<SuccessResult>> => {
  return request.patch(`/dashboard/alerts/${id}/handle`) as Promise<ApiResponse<SuccessResult>>
}

export const markAllAlertsRead = (): Promise<ApiResponse<SuccessResult>> => {
  return request.patch('/dashboard/alerts/read-all') as Promise<ApiResponse<SuccessResult>>
}