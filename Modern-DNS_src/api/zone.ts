import request from './request'
import type { ApiResponse, IdResult, SuccessResult } from '../types/api'
import type {
  DomainZone,
  DomainRecord,
  DomainSoa,
  DomainDetail,
} from '../types/modules'

/* ─────────────────────────────────────────────
   Payload types
───────────────────────────────────────────── */

export interface ZoneListParams {
  page?: number
  pageSize?: number
  keyword?: string
  type?: string
  status?: string
}

export interface ZoneListResult {
  total: number
  list: DomainZone[]
}

export interface ZoneCreatePayload {
  zoneId?: string
  domain: string
  type: string
  remark?: string
  upstream?: string
}

export interface ZoneUpdatePayload {
  type?: string
  remark?: string
  upstream?: string
  status?: string
  serial?: string
}

export interface RecordUpsertPayload {
  type: string
  host: string
  value: string
  ttl: number
  status: '启用' | '禁用'
  remark?: string
}

export interface RecordStatusPayload {
  status: '启用' | '禁用'
}

export interface BatchDeletePayload {
  ids: number[]
}

export interface DnssecTogglePayload {
  enabled: boolean
}

/* ─────────────────────────────────────────────
   Zone CRUD
───────────────────────────────────────────── */

/** 获取区域列表（分页 + 过滤） */
export const getZones = (params?: ZoneListParams): Promise<ApiResponse<ZoneListResult>> =>
  request.get('/domain/zones', { params })

/** 获取单个区域详情（含 records / SOA / DNSSEC） */
export const getZoneDetail = (zoneId: number | string): Promise<ApiResponse<DomainDetail>> =>
  request.get(`/domain/zones/${zoneId}`)

/** 新增区域 */
export const createZone = (payload: ZoneCreatePayload): Promise<ApiResponse<DomainZone>> =>
  request.post('/domain/zones', payload)

/** 编辑区域基本信息 */
export const updateZone = (
  zoneId: number | string,
  payload: ZoneUpdatePayload,
): Promise<ApiResponse<DomainZone>> =>
  request.put(`/domain/zones/${zoneId}`, payload)

/** 批量删除区域 */
export const deleteZones = (payload: BatchDeletePayload): Promise<ApiResponse<SuccessResult>> =>
  request.delete('/domain/zones-batch', { data: payload })

/** 批量启用 / 禁用区域 */
export const batchUpdateZoneStatus = (
  ids: Array<number | string>,
  status: string,
): Promise<ApiResponse<SuccessResult>> =>
  request.patch('/domain/zones-batch-status', { ids, status })

/* ─────────────────────────────────────────────
   DNS Records
───────────────────────────────────────────── */

/** 获取区域下所有解析记录 */
export const getRecords = (zoneId: number | string): Promise<ApiResponse<DomainRecord[]>> =>
  request.get(`/domain/zones/${zoneId}/records`)

/** 新增解析记录 */
export const createRecord = (
  zoneId: number | string,
  payload: RecordUpsertPayload,
): Promise<ApiResponse<DomainRecord>> =>
  request.post(`/domain/zones/${zoneId}/records`, payload)

/** 编辑解析记录 */
export const updateRecord = (
  zoneId: number | string,
  recordId: number,
  payload: RecordUpsertPayload,
): Promise<ApiResponse<DomainRecord>> =>
  request.put(`/domain/zones/${zoneId}/records/${recordId}`, payload)

/** 批量删除解析记录 */
export const deleteRecords = (
  zoneId: number | string,
  payload: BatchDeletePayload,
): Promise<ApiResponse<SuccessResult>> =>
  request.delete(`/domain/zones/${zoneId}/records-batch`, { data: payload })

/** 导出区域记录为 CSV（直接触发浏览器下载） */
export const exportRecordsCsvApi = (zoneId: number | string): void => {
  request.get(`/domain/zones/${zoneId}/records/export`, { responseType: 'blob' })
    .then((res: any) => {
      const blob = res instanceof Blob ? res : new Blob([res])
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `zone-${zoneId}-records.csv`
      a.click()
      URL.revokeObjectURL(url)
    })
}

/** 下载导入 CSV 模版 */
export const downloadRecordTemplateApi = (): void => {
  request.get('/domain/records/template', { responseType: 'blob' })
    .then((res: any) => {
      const blob = res instanceof Blob ? res : new Blob([res])
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'records-import-template.csv'
      a.click()
      URL.revokeObjectURL(url)
    })
}

/** 批量导入 CSV 记录（通过后端接口，支持冲突检测） */
export const importRecordsCsvApi = (
  zoneId: number | string,
  file: File,
): Promise<{ data: { data: { imported: number; skipped: number } } }> => {
  const form = new FormData()
  form.append('file', file)
  return request.post(`/domain/zones/${zoneId}/records/import`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

/** 切换单条记录启用状态 */
export const updateRecordStatus = (
  zoneId: number | string,
  recordId: number,
  payload: RecordStatusPayload,
): Promise<ApiResponse<DomainRecord>> =>
  request.patch(`/domain/zones/${zoneId}/records/${recordId}/status`, payload)

/* ─────────────────────────────────────────────
   SOA
───────────────────────────────────────────── */

/** 获取 SOA */
export const getSoa = (zoneId: number | string): Promise<ApiResponse<DomainSoa>> =>
  request.get(`/domain/zones/${zoneId}/soa`)

/** 更新 SOA */
export const updateSoa = (
  zoneId: number | string,
  payload: DomainSoa,
): Promise<ApiResponse<DomainSoa>> =>
  request.put(`/domain/zones/${zoneId}/soa`, payload)

/* ─────────────────────────────────────────────
   DNSSEC
───────────────────────────────────────────── */

/** 开启 / 关闭 DNSSEC */
export const toggleDnssec = (
  zoneId: number | string,
  payload: DnssecTogglePayload,
): Promise<ApiResponse<SuccessResult>> =>
  request.put(`/domain/zones/${zoneId}/dnssec/toggle`, payload)

/** 触发密钥轮转（ksk | zsk） */
export const rotateDnssecKey = (
  zoneId: number | string,
  keyType: 'ksk' | 'zsk',
): Promise<ApiResponse<IdResult>> =>
  request.post(`/domain/zones/${zoneId}/dnssec/generate`, { keyType })
