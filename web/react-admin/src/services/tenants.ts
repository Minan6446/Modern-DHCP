import type { TenantPayload, TenantSummary } from '../types/api'
import { http } from './http'

export async function listTenants(): Promise<TenantSummary[]> {
  const { data } = await http().get('/tenants')
  return data
}

export async function createTenant(payload: TenantPayload): Promise<TenantSummary> {
  const { data } = await http().post('/tenants', payload)
  return data
}

export async function updateTenant(tenantId: string, payload: TenantPayload): Promise<TenantSummary> {
  const { data } = await http().patch(`/tenants/${tenantId}`, payload)
  return data
}

export async function deleteTenant(tenantId: string): Promise<void> {
  await http().delete(`/tenants/${tenantId}`)
}
