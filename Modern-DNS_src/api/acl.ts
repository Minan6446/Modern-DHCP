import request from './request'
import type { ApiResponse, IdResult } from '../types/api'

export interface AclRule {
  id: number
  name: string
  cidr: string
  type: '允许' | '拒绝'
  priority: number
  queryTypes: string[]
  zones: string
  hitCount: number
  status: '启用' | '禁用'
  remark: string
  createdAt: string
}

interface AclRuleDTO extends Omit<AclRule, 'queryTypes'> {
  queryTypes: string // backend returns comma-joined string
}

const fromDTO = (r: AclRuleDTO): AclRule => ({
  ...r,
  queryTypes: typeof r.queryTypes === 'string'
    ? r.queryTypes.split(',').map((s) => s.trim()).filter(Boolean)
    : [],
})

export const listAclRulesApi = async (
  params?: { keyword?: string; type?: string; status?: string },
): Promise<ApiResponse<{ list: AclRule[]; total: number }>> => {
  const res = (await request.get('/security/acl', { params })) as ApiResponse<{ list: AclRuleDTO[]; total: number }>
  return {
    ...res,
    data: { list: (res.data?.list ?? []).map(fromDTO), total: res.data?.total ?? 0 },
  }
}

export const createAclRuleApi = async (payload: Partial<AclRule>): Promise<ApiResponse<AclRule>> => {
  const res = (await request.post('/security/acl', payload)) as ApiResponse<AclRuleDTO>
  return { ...res, data: res.data ? fromDTO(res.data) : (res.data as unknown as AclRule) }
}

export const updateAclRuleApi = async (id: number, payload: Partial<AclRule>): Promise<ApiResponse<AclRule>> => {
  const res = (await request.put(`/security/acl/${id}`, payload)) as ApiResponse<AclRuleDTO>
  return { ...res, data: res.data ? fromDTO(res.data) : (res.data as unknown as AclRule) }
}

export const deleteAclRuleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/security/acl/${id}`) as Promise<ApiResponse<IdResult>>
}

export const toggleAclRuleApi = (
  id: number,
  status: '启用' | '禁用',
): Promise<ApiResponse<{ id: number; status: string }>> => {
  return request.patch(`/security/acl/${id}/toggle`, { status }) as Promise<ApiResponse<{ id: number; status: string }>>
}
