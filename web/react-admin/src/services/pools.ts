import type { PaginatedResponse, PoolQuery, PoolSummary } from '../types/api'
import { getActiveTenantId, http } from './http'

export async function listPools(params?: PoolQuery): Promise<PaginatedResponse<PoolSummary>> {
  const tenantId = getActiveTenantId()
  const { data } = await http().get(`/tenants/${tenantId}/pools`, { params })
  return data
}
