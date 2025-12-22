import type {
  LeaseHistoryExportPayload,
  LeaseHistoryExportResponse,
  LeaseHistorySchedulePayload,
  LeaseHistoryScheduleResponse,
  LeaseHistoryQuery,
  LeaseHistoryResponse,
  LeaseInsights,
  LeaseQuery,
  LeaseRecord,
  PaginatedResponse,
} from '../types/api'
import { getActiveTenantId, http } from './http'

export async function listLeases(
  params?: LeaseQuery & { page?: number; size?: number },
): Promise<PaginatedResponse<LeaseRecord>> {
  const tenantId = getActiveTenantId()
  const { data } = await http().get(`/tenants/${tenantId}/leases`, {
    params: { page: 1, size: 15, ...params },
  })
  return data
}

export async function fetchLeaseInsights(): Promise<LeaseInsights> {
  const tenantId = getActiveTenantId()
  const { data } = await http().get(`/tenants/${tenantId}/leases/insights`)
  return data
}

export async function releaseLease(leaseId: string, reason?: string) {
  const payload = reason ? { reason } : undefined
  const tenantId = getActiveTenantId()
  const { data } = await http().post(`/tenants/${tenantId}/leases/${leaseId}/release`, payload)
  return data
}

export async function fetchLeaseHistory(params: LeaseHistoryQuery): Promise<LeaseHistoryResponse> {
  const tenantId = getActiveTenantId()
  const { data } = await http().get(`/tenants/${tenantId}/leases/history`, {
    params,
  })
  return data
}

export async function exportLeaseHistory(
  payload: LeaseHistoryExportPayload,
): Promise<LeaseHistoryExportResponse> {
  const tenantId = getActiveTenantId()
  const { data } = await http().post(`/tenants/${tenantId}/leases/history/export`, payload)
  return data
}

export async function scheduleLeaseHistory(
  payload: LeaseHistorySchedulePayload,
): Promise<LeaseHistoryScheduleResponse> {
  const tenantId = getActiveTenantId()
  const { data } = await http().post(`/tenants/${tenantId}/reports/lease-daily`, {
    tenantId,
    ...payload,
  })
  return data
}
