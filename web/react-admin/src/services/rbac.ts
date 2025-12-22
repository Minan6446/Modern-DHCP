import type { RbacRolePayload, RbacRoleSummary } from '../types/api'
import { http } from './http'

export async function listRoles(): Promise<RbacRoleSummary[]> {
  const { data } = await http().get('/rbac/roles')
  return data
}

export async function createRole(payload: RbacRolePayload): Promise<RbacRoleSummary> {
  const { data } = await http().post('/rbac/roles', payload)
  return data
}

export async function updateRole(roleId: string, payload: RbacRolePayload): Promise<RbacRoleSummary> {
  const { data } = await http().put(`/rbac/roles/${roleId}`, payload)
  return data
}

export async function deleteRole(roleId: string): Promise<void> {
  await http().delete(`/rbac/roles/${roleId}`)
}
