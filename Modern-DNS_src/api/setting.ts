import axios from 'axios'
import request from './request'
import type { ApiResponse, IdResult, SuccessResult } from '../types/api'
import type {
  SettingBackup,
  SettingCommonConfig,
  SettingLogItem,
  SettingModuleData,
  SettingNoticeConfig,
  SettingRole,
  SettingUser,
} from '../types/modules'

type RolePermissionPayload = {
  roleId: number
  permissions: string[]
}

type UploadBackupResult = {
  fileName: string
  restoreScope: string[]
  format: string
}

export type BackupListQuery = {
  keyword?: string
  format?: string
  page?: number
  size?: number
}

export type BackupListResult = {
  list: SettingBackup[]
  total: number
}

export type UserListQuery = {
  username?: string
  status?: string
  roleId?: number | ''
  page?: number
  size?: number
}

export type UserListResult = {
  list: SettingUser[]
  total: number
}

export interface ApiKeyRecord {
  id: number
  name: string
  prefix: string
  scope: string[]
  createdBy: string
  createdAt: string
  lastUsedAt: string
  expiresAt: string
  status: string
}

export type ApiKeyListResult = { list: ApiKeyRecord[]; total: number }

export interface AuditLogRow {
  id: string
  operator: string
  operatorRole: string
  clientIp: string
  module: string
  // Backend persists both `actionType` (canonical category) and `action`
  // (free-form description). Older entries / non-HTTP loggers may set
  // only one of the two; the audit page reads both with a fallback.
  actionType?: string
  action: string
  target: string
  result: string
  duration: number
  time: string
  before?: string
  after?: string
}

export type AuditLogResult = { total: number; rows: AuditLogRow[] }

// ── Module data ──────────────────────────────────────────────────────────────

export const getSettingModuleDataApi = (): Promise<ApiResponse<SettingModuleData>> => {
  return Promise.all([
    request.get('/setting/common') as Promise<ApiResponse<any>>,
    request.get('/setting/users') as Promise<ApiResponse<any>>,
    request.get('/setting/roles') as Promise<ApiResponse<any>>,
    request.get('/setting/backups') as Promise<ApiResponse<any>>,
    request.get('/setting/notice') as Promise<ApiResponse<any>>,
    request.get('/setting/logs') as Promise<ApiResponse<any>>,
  ]).then(([common, users, roles, backups, notice, logs]) => ({
    // roles endpoint may return either role array or object payload.
    // Keep both structures compatible to avoid losing permission tree data.
    code: 0, message: 'ok',
    data: {
      common:   common.data,
      users:    (users.data as any)?.list ?? [],
      roles:    Array.isArray(roles.data) ? roles.data : (roles.data as any)?.roles ?? [],
      permissionTree: Array.isArray((roles.data as any)?.permissionTree) ? (roles.data as any).permissionTree : [],
      rolePermissions: (roles.data as any)?.rolePermissions ?? {},
      backups:  (backups.data as any)?.list ?? [],
      notice:   notice.data ?? {},
      logs:     (logs.data as any)?.rows ?? [],
    } as SettingModuleData,
  }))
}

// ── General Config ────────────────────────────────────────────────────────────

export const getCommonConfigApi = (): Promise<ApiResponse<SettingCommonConfig>> => {
  return request.get('/setting/common') as Promise<ApiResponse<SettingCommonConfig>>
}

// saveCommonConfigApi accepts an optional `confirmLockout` flag the
// caller passes after the operator OK'd the "your IP isn't in the new
// whitelist" warning dialog. The backend keys off ?confirmLockout=1 to
// bypass its self-lockout guard, so the round-trip looks like:
//   1st PUT  → backend returns code 4422, frontend pops dialog
//   2nd PUT  → with confirmLockout=true → real save
export const saveCommonConfigApi = (
  payload: SettingCommonConfig,
  opts?: { confirmLockout?: boolean },
): Promise<ApiResponse<SettingCommonConfig & { savedAt: string }>> => {
  const params = opts?.confirmLockout ? { confirmLockout: 1 } : undefined
  return request.put('/setting/common', payload, { params }) as Promise<ApiResponse<SettingCommonConfig & { savedAt: string }>>
}

export const resetCommonConfigApi = (): Promise<ApiResponse<SettingCommonConfig>> => {
  return request.post('/setting/common/reset') as Promise<ApiResponse<SettingCommonConfig>>
}

// Synchronous NTP probe driven by the General Settings → System tab
// "立即测试" button. The backend takes the supplied servers (or the
// persisted list when omitted) and returns either:
//   { success: true,  server: '<host>', driftMs: <int>, checkAt: 'YYYY-...' }
//   { success: false, error: '<message>' }
// The probe deliberately does NOT mutate the persisted ntp_last_*
// columns — those are owned by the background poller, not by this
// ad-hoc test path.
export interface NtpTestResult {
  success: boolean
  server?: string
  driftMs?: number
  checkAt?: string
  error?: string
}

export const testNtpApi = (servers?: string[]): Promise<ApiResponse<NtpTestResult>> => {
  return request.post('/setting/common/ntp/test', { servers: servers ?? [] }) as Promise<ApiResponse<NtpTestResult>>
}

// Scoped writer for the audit-log dialog. Backed by PUT /setting/common/log
// which only touches the log-* / syslog-* columns; the rest of the
// system_config row is left untouched.
//
// We deliberately accept the same field names the backend reads so the
// audit dialog can pass `{ ...configForm }` slices without renaming.
// Compared to the previous saveCommonConfigApi round-trip, this:
//   - avoids the last-writer-wins race against the General Settings
//     panel (only 5 columns get written)
//   - avoids accidentally clobbering NTP poller-owned status columns
//   - returns immediately with `{ savedAt }` like the other config writers
export interface LogConfigPayload {
  logLevel?: string
  logRetentionDays: number
  logExportFormat: 'csv' | 'json' | 'syslog'
  syslogEnabled: boolean
  syslogServer: string
}

export const saveLogConfigApi = (
  payload: LogConfigPayload,
): Promise<ApiResponse<{ savedAt: string }>> => {
  return request.put('/setting/common/log', payload) as Promise<ApiResponse<{ savedAt: string }>>
}

// ── Users ─────────────────────────────────────────────────────────────────────

export const getUserList = (params: UserListQuery): Promise<ApiResponse<UserListResult>> => {
  return request.get('/setting/users', { params }) as Promise<ApiResponse<UserListResult>>
}

export const saveUserApi = (payload: Partial<SettingUser>): Promise<ApiResponse<Partial<SettingUser>>> => {
  return request.post('/setting/users', payload) as Promise<ApiResponse<Partial<SettingUser>>>
}

export const addUser = saveUserApi
export const editUser = saveUserApi

export const deleteUserApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/setting/users/${id}`) as Promise<ApiResponse<IdResult>>
}

export const deleteUser = deleteUserApi

export const resetUserPasswordApi = (payload: Pick<SettingUser, 'id' | 'username'> & { password?: string }): Promise<ApiResponse<SuccessResult & { password?: string }>> => {
  return request.post(`/setting/users/${payload.id}/reset-password`, { password: payload.password }) as Promise<ApiResponse<SuccessResult & { password?: string }>>
}

// Lock-status / unlock — paired with the Redis-backed login-failure
// counter. Lock-status is cheap (one Redis GET + TTL); we call it from
// the user-management table to surface a "locked" badge so admins can
// see who's locked without waiting for them to log in. Unlock clears
// the counter so the user can retry immediately.
export interface UserLockStatus {
  username: string
  failCount: number
  failMax: number
  locked: boolean
  lockTTLSec: number
}

export const getUserLockStatusApi = (id: number): Promise<ApiResponse<UserLockStatus>> => {
  return request.get(`/setting/users/${id}/lock-status`) as Promise<ApiResponse<UserLockStatus>>
}

export const unlockUserApi = (id: number): Promise<ApiResponse<{ username: string; unlocked: boolean }>> => {
  return request.post(`/setting/users/${id}/unlock`) as Promise<ApiResponse<{ username: string; unlocked: boolean }>>
}

// One row per (user, sid) pair. The shape is intentionally flat so
// the online-users table can render one row per session with its own
// "logout this session" button, while a "logout all" UX collapses
// rows by userId on the frontend.
export interface OnlineSession {
  userId: number
  username: string
  realName: string
  roleName: string
  sid: string
  // unix-millis timestamp of when the sid was first registered (login
  // moment, or last refresh-token rotation, whichever came later).
  issuedAt: number
  // True when this row is the caller's own current session — the UI
  // grays out the per-row revoke button to match the backend's
  // self-protection check.
  isCurrent: boolean
}

export const listOnlineSessionsApi = (): Promise<ApiResponse<OnlineSession[]>> => {
  return request.get('/setting/online-sessions') as Promise<ApiResponse<OnlineSession[]>>
}

// reason is propagated to the audit log. Operators are nudged into
// picking from a four-item enum on the UI but the API accepts free
// text so an integration script can paste a ticket reference verbatim.
export const revokeOnlineSessionApi = (
  userId: number,
  sid: string,
  reason: string = '',
): Promise<ApiResponse<{ success: boolean }>> => {
  return request.post('/setting/online-sessions/revoke', { userId, sid, reason }) as Promise<
    ApiResponse<{ success: boolean }>
  >
}

// Force-revoke every active session of the target user. The number
// returned is the count of sessions that *were* active just before
// the kick — useful for audit display ("3 个会话已被踢出"). After
// this call lands, every browser tab still holding an old JWT will
// 401 on its next request and bounce to the login screen.
export const kickUserSessionsApi = (
  id: number,
  reason: string = '',
): Promise<ApiResponse<{ username: string; kicked: number }>> => {
  return request.post(`/setting/users/${id}/kick-sessions`, { reason }) as Promise<
    ApiResponse<{ username: string; kicked: number }>
  >
}

// ── Roles ─────────────────────────────────────────────────────────────────────

export const saveRoleApi = (payload: Partial<SettingRole>): Promise<ApiResponse<Partial<SettingRole>>> => {
  return request.post('/setting/roles', payload) as Promise<ApiResponse<Partial<SettingRole>>>
}

export const deleteRoleApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/setting/roles/${id}`) as Promise<ApiResponse<IdResult>>
}

export const getRolePermissionsApi = (roleId: number): Promise<ApiResponse<string[]>> => {
  return request.get(`/setting/roles/${roleId}/permissions`) as Promise<ApiResponse<string[]>>
}

export const saveRolePermissionsApi = (payload: RolePermissionPayload): Promise<ApiResponse<RolePermissionPayload>> => {
  return request.put(`/setting/roles/${payload.roleId}/permissions`, { permissions: payload.permissions }) as Promise<ApiResponse<RolePermissionPayload>>
}

// ── Backup ────────────────────────────────────────────────────────────────────

export const getBackupList = (params: BackupListQuery): Promise<ApiResponse<BackupListResult>> => {
  return request.get('/setting/backups', { params }) as Promise<ApiResponse<BackupListResult>>
}

export const createBackupApi = (payload: { backupScope: string[]; backupFormat: string }): Promise<ApiResponse<SettingBackup>> => {
  return request.post('/setting/backups', payload) as Promise<ApiResponse<SettingBackup>>
}

export const restoreBackupApi = (payload: { id?: number; fileName?: string; restoreScope?: string[]; backupId?: string }): Promise<ApiResponse<SuccessResult>> => {
  if (payload.id) {
    return request.post(`/setting/backups/${payload.id}/restore`, payload) as Promise<ApiResponse<SuccessResult>>
  }
  return request.post('/setting/backups/restore', payload) as Promise<ApiResponse<SuccessResult>>
}

export const deleteBackupApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/setting/backups/${id}`) as Promise<ApiResponse<IdResult>>
}

export const uploadBackupFileApi = (payload: FormData | { fileName: string }): Promise<ApiResponse<UploadBackupResult>> => {
  if (payload instanceof FormData) {
    return request.post('/setting/backups/upload', payload, { headers: { 'Content-Type': 'multipart/form-data' } }) as Promise<ApiResponse<UploadBackupResult>>
  }
  return request.post('/setting/backups/upload', payload) as Promise<ApiResponse<UploadBackupResult>>
}

export const exportBackup = (id: number): Promise<ApiResponse<SettingBackup | null>> => {
  return request.get(`/setting/backups/${id}/export`) as Promise<ApiResponse<SettingBackup | null>>
}

export const createBackup = createBackupApi
export const restoreBackup = restoreBackupApi
export const deleteBackup = deleteBackupApi
export const uploadBackup = uploadBackupFileApi

// ── Notice ────────────────────────────────────────────────────────────────────

export const saveNoticeConfigApi = (payload: SettingNoticeConfig): Promise<ApiResponse<SettingNoticeConfig & { savedAt: string }>> => {
  return request.put('/setting/notice', payload) as Promise<ApiResponse<SettingNoticeConfig & { savedAt: string }>>
}

export const testNoticeChannelApi = (payload: { channel: string }): Promise<ApiResponse<SuccessResult & { channel: string }>> => {
  return request.post('/setting/notice/test', payload) as Promise<ApiResponse<SuccessResult & { channel: string }>>
}

// ── API Keys ──────────────────────────────────────────────────────────────

export const listApiKeysApi = (params?: { keyword?: string; status?: string; page?: number; size?: number }): Promise<ApiResponse<ApiKeyListResult>> => {
  return request.get('/setting/api-keys', { params }) as Promise<ApiResponse<ApiKeyListResult>>
}

export const createApiKeyApi = (payload: { name: string; scope: string[]; expiresAt?: string; noExpiry: boolean }): Promise<ApiResponse<ApiKeyRecord & { secret?: string }>> => {
  return request.post('/setting/api-keys', payload) as Promise<ApiResponse<ApiKeyRecord & { secret?: string }>>
}

export const updateApiKeyApi = (id: number, payload: { name: string; scope: string[]; expiresAt?: string; noExpiry: boolean }): Promise<ApiResponse<ApiKeyRecord>> => {
  return request.put(`/setting/api-keys/${id}`, payload) as Promise<ApiResponse<ApiKeyRecord>>
}

export const toggleApiKeyApi = (id: number): Promise<ApiResponse<{ id: number; status: string }>> => {
  return request.patch(`/setting/api-keys/${id}/toggle`) as Promise<ApiResponse<{ id: number; status: string }>>
}

export const revokeApiKeyApi = (id: number): Promise<ApiResponse<IdResult>> => {
  return request.delete(`/setting/api-keys/${id}`) as Promise<ApiResponse<IdResult>>
}

// ── Audit / Operation Logs ──────────────────────────────────────────────────────────────

export const getAuditLogsApi = (params?: { keyword?: string; module?: string; action?: string; result?: string; startTime?: string; endTime?: string; page?: number; size?: number }): Promise<ApiResponse<AuditLogResult>> => {
  return request.get('/setting/logs', { params }) as Promise<ApiResponse<AuditLogResult>>
}

export interface AuditLogExportPayload {
  format?: 'csv' | 'json'
  keyword?: string
  module?: string
  action?: string
  result?: string
  startTime?: string
  endTime?: string
}

export interface AuditLogExportResult {
  filename: string
  count: number
  truncated: boolean
  format: 'csv' | 'json'
}

// exportAuditLogsApi triggers a real file download via a blob response.
// We bypass the shared axios `request` instance because its global
// response interceptor unwraps `response.data`, which would strip the
// Content-Disposition / X-Export-Count headers we need to (a) name the
// downloaded file after the server's stamp and (b) tell the operator how
// many rows actually got included (in particular when the 100k cap was
// hit). The auth header is still attached manually so the request goes
// out under the same identity as every other call.
export const exportAuditLogsApi = async (
  payload?: AuditLogExportPayload,
): Promise<AuditLogExportResult> => {
  const format = payload?.format || 'csv'
  const token = localStorage.getItem('modern-dns-token') || ''
  const res = await axios.post('/api/setting/logs/export', { ...payload, format }, {
    responseType: 'blob',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    // Long-running export shouldn't share the 10s default timeout the
    // shared instance imposes on regular API calls.
    timeout: 120000,
  })

  // Server may still return a JSON error body even when we asked for a
  // blob (e.g. format validation failure). Detect by content-type and
  // re-parse so the caller sees a usable error message instead of an
  // opaque blob being saved as a file.
  const contentType = String(res.headers?.['content-type'] || '')
  if (contentType.includes('application/json') && (res.data as Blob).size < 4096) {
    const text = await (res.data as Blob).text()
    try {
      const parsed = JSON.parse(text) as { code?: number; message?: string }
      if (parsed && typeof parsed.code === 'number' && parsed.code !== 0) {
        throw new Error(parsed.message || '导出失败')
      }
    } catch (_e) {
      // Fall through — looked JSON-ish but didn't parse, treat as data.
    }
  }

  const disposition = String(res.headers?.['content-disposition'] || '')
  const match = disposition.match(/filename="?([^"]+)"?/i)
  const filename = match?.[1] || `audit-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.${format}`
  const count = Number(res.headers?.['x-export-count'] || 0)
  const truncated = res.headers?.['x-export-truncated'] === '1'

  // Drive the download by clicking a transient anchor — works in every
  // evergreen browser and avoids opening a new tab (which Safari blocks
  // when not triggered directly by a user gesture chain).
  const url = window.URL.createObjectURL(res.data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Defer the revoke a tick so Firefox finishes the download navigation
  // before the URL is invalidated.
  setTimeout(() => window.URL.revokeObjectURL(url), 1000)

  return { filename, count, truncated, format }
}

export interface PurgeExpiredLogsResult {
  deleted: number
  retentionDays: number
  cutoff: string
}

// Manually triggers the operation-log retention sweep on the backend.
// Mirrors the once-per-minute scheduler tick so the operator can reclaim
// space immediately after lowering the retention window.
export const purgeExpiredAuditLogsApi = (): Promise<ApiResponse<PurgeExpiredLogsResult>> => {
  return request.post('/setting/logs/purge-expired') as Promise<ApiResponse<PurgeExpiredLogsResult>>
}

export const getLogsApi = (): Promise<ApiResponse<SettingLogItem[]>> => {
  return request.get('/setting/logs') as Promise<ApiResponse<SettingLogItem[]>>
}

export const exportLogsApi = (payload: { count: number }): Promise<ApiResponse<SuccessResult & { count: number }>> => {
  return request.post('/setting/logs/export', payload) as Promise<ApiResponse<SuccessResult & { count: number }>>
}
