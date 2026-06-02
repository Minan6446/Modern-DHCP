import request from './request'
import type { ApiResponse, IdResult, SuccessResult } from '../types/api'
import type {
  SecurityDdosDomainRule,
  SecurityDdosGlobal,
  SecurityModuleData,
  SecurityBlackWhiteRule,
} from '../types/modules'

type BatchStatusPayload = {
  ids: number[]
  status: string
}

type DnssecTogglePayload = {
  id: number
  enabled: boolean
}

type DnssecKeyPayload = {
  id: number
  keyType: 'ksk' | 'zsk'
}

type DnssecDeleteKeyPayload = DnssecKeyPayload & {
  keyId: string
}

export interface TLSCertRecord {
  id: number
  domain: string
  type: 'DoT' | 'DoH' | 'mTLS'
  issuer: string
  expireAt: string
  daysLeft: number
  status: string
  fingerprint: string
  uploadedAt: string
}

export type TLSCertListResult = { list: TLSCertRecord[]; total: number }

// ── Module data ───────────────────────────────────────────────────────────────

export const getSecurityModuleData = (): Promise<ApiResponse<SecurityModuleData>> => {
  return Promise.all([
    request.get('/security/bw-rules') as Promise<ApiResponse<any>>,
    request.get('/security/ddos') as Promise<ApiResponse<any>>,
    request.get('/security/dnssec') as Promise<ApiResponse<any>>,
  ]).then(([bwRes, ddosRes, dnssecRes]) => ({
    code: 0,
    message: 'ok',
    data: {
      blackWhite: { rules: (bwRes.data as any)?.list ?? [] },
      ddos: {
        global: (ddosRes.data as any)?.global ?? {},
        domainRules: (ddosRes.data as any)?.domainRules ?? [],
      },
      dnssec: { rows: dnssecRes.data ?? [] },
    },
  })) as Promise<ApiResponse<SecurityModuleData>>
}

// ── Black/White Rules ─────────────────────────────────────────────────────────

export const saveBlackWhiteRuleApi = (payload: Partial<SecurityBlackWhiteRule> & Pick<SecurityBlackWhiteRule, 'type' | 'listType' | 'value' | 'remark' | 'status'>): Promise<ApiResponse<Partial<SecurityBlackWhiteRule>>> => {
  if (payload.id) {
    return request.put(`/security/bw-rules/${payload.id}`, payload) as Promise<ApiResponse<Partial<SecurityBlackWhiteRule>>>
  }
  return request.post('/security/bw-rules', payload) as Promise<ApiResponse<Partial<SecurityBlackWhiteRule>>>
}

export const deleteBlackWhiteRuleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/security/bw-rules/${id}`) as Promise<ApiResponse<IdResult>>
}

export const batchUpdateBlackWhiteStatusApi = (payload: BatchStatusPayload): Promise<ApiResponse<BatchStatusPayload>> => {
  return request.put('/security/bw-rules/batch-status', payload) as Promise<ApiResponse<BatchStatusPayload>>
}

export const batchDeleteBlackWhiteRulesApi = (ids: number[]): Promise<ApiResponse<{ ids: number[] }>> => {
  return request.delete('/security/bw-rules/batch', { data: { ids } }) as Promise<ApiResponse<{ ids: number[] }>>
}

export const importBlackWhiteRulesApi = (rules: Array<{ type: string; listType: string; value: string; remark: string; status: string }>): Promise<ApiResponse<{ imported: number }>> => {
  return request.post('/security/bw-rules/import', rules) as Promise<ApiResponse<{ imported: number }>>
}

// ── DDoS ──────────────────────────────────────────────────────────────────────

export const saveDdosGlobalConfigApi = (payload: SecurityDdosGlobal): Promise<ApiResponse<SecurityDdosGlobal>> => {
  return request.put('/security/ddos/global', payload) as Promise<ApiResponse<SecurityDdosGlobal>>
}

export const saveDdosDomainRuleApi = (payload: Partial<SecurityDdosDomainRule> & Pick<SecurityDdosDomainRule, 'domain' | 'qpsLimit' | 'status'>): Promise<ApiResponse<Partial<SecurityDdosDomainRule>>> => {
  if (payload.id) {
    return request.put(`/security/ddos/domain-rules/${payload.id}`, payload) as Promise<ApiResponse<Partial<SecurityDdosDomainRule>>>
  }
  return request.post('/security/ddos/domain-rules', payload) as Promise<ApiResponse<Partial<SecurityDdosDomainRule>>>
}

export const deleteDdosDomainRuleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/security/ddos/domain-rules/${id}`) as Promise<ApiResponse<IdResult>>
}

export const batchDeleteDdosDomainRulesApi = (ids: number[]): Promise<ApiResponse<{ ids: number[] }>> => {
  return request.delete('/security/ddos/domain-rules/batch', { data: { ids } }) as Promise<ApiResponse<{ ids: number[] }>>
}

// ── DNSSEC ────────────────────────────────────────────────────────────────────

export const checkDnssecAllApi = (): Promise<ApiResponse<SuccessResult>> => {
  return request.post('/security/dnssec/check-all') as Promise<ApiResponse<SuccessResult>>
}

export const checkDnssecByIdApi = (id: number): Promise<ApiResponse<SuccessResult & IdResult>> => {
  return request.post(`/security/dnssec/${id}/check`) as Promise<ApiResponse<SuccessResult & IdResult>>
}

export const toggleDnssecApi = (payload: DnssecTogglePayload): Promise<ApiResponse<SuccessResult & DnssecTogglePayload>> => {
  return request.put(`/security/dnssec/${payload.id}/toggle`, payload) as Promise<ApiResponse<SuccessResult & DnssecTogglePayload>>
}

export const generateDnssecKeyApi = (payload: DnssecKeyPayload): Promise<ApiResponse<SuccessResult & DnssecKeyPayload & { key: { keyId: string; createdAt: string; status: string } }>> => {
  return request.post(`/security/dnssec/${payload.id}/generate-key`, payload) as Promise<ApiResponse<SuccessResult & DnssecKeyPayload & { key: { keyId: string; createdAt: string; status: string } }>>
}

export const deleteDnssecKeyApi = (payload: DnssecDeleteKeyPayload): Promise<ApiResponse<SuccessResult & DnssecDeleteKeyPayload>> => {
  return request.delete(`/security/dnssec/${payload.id}/keys/${payload.keyId}`, { params: { keyType: payload.keyType } }) as Promise<ApiResponse<SuccessResult & DnssecDeleteKeyPayload>>
}

// ── TLS Certificates ──────────────────────────────────────────────────────────

export const listCertsApi = (params?: { keyword?: string; status?: string; page?: number; size?: number }): Promise<ApiResponse<TLSCertListResult>> => {
  return request.get('/security/certs', { params }) as Promise<ApiResponse<TLSCertListResult>>
}

export const uploadCertApi = (payload: { domain: string; type: string; certContent: string; keyContent?: string; expireAt?: string }): Promise<ApiResponse<TLSCertRecord>> => {
  return request.post('/security/certs', payload) as Promise<ApiResponse<TLSCertRecord>>
}

export const deleteCertApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/security/certs/${id}`) as Promise<ApiResponse<IdResult>>
}

export const renewCertApi = (id: number): Promise<ApiResponse<TLSCertRecord>> => {
  return request.post(`/security/certs/${id}/renew`) as Promise<ApiResponse<TLSCertRecord>>
}

// ── RPZ Rules ────────────────────────────────────────────────────────────────

export interface RpzRule {
  id: number
  name: string
  category: string
  type: string
  pattern: string
  action: string
  redirectTo: string
  hitCount: number
  status: string
  createdAt: string
  updatedAt: string
}

export const listRpzRulesApi = (params?: { keyword?: string; action?: string; status?: string }): Promise<ApiResponse<{ list: RpzRule[]; total: number }>> => {
  return request.get('/security/rpz', { params }) as Promise<ApiResponse<{ list: RpzRule[]; total: number }>>
}

export const createRpzRuleApi = (payload: Partial<RpzRule>): Promise<ApiResponse<RpzRule>> => {
  return request.post('/security/rpz', payload) as Promise<ApiResponse<RpzRule>>
}

export const updateRpzRuleApi = (payload: Partial<RpzRule>): Promise<ApiResponse<RpzRule>> => {
  return request.put(`/security/rpz/${payload.id}`, payload) as Promise<ApiResponse<RpzRule>>
}

export const deleteRpzRuleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/security/rpz/${id}`) as Promise<ApiResponse<IdResult>>
}

export const toggleRpzRuleApi = (id: number, status: string): Promise<ApiResponse<{ id: number; status: string }>> => {
  return request.put(`/security/rpz/${id}/toggle`, { status }) as Promise<ApiResponse<{ id: number; status: string }>>
}
