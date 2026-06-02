<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  getAuditLogsApi,
  exportAuditLogsApi,
  purgeExpiredAuditLogsApi,
  getSettingModuleDataApi,
  saveLogConfigApi,
  type AuditLogRow,
} from '@/api/setting'
import {
  createDefaultGeneralConfigForm,
  mergeGeneralConfigForm,
} from '@/types/setting'
import type { GeneralConfigForm } from '@/types/setting'
import { confirmRiskAction } from '@/utils/interaction'

const { t } = useI18n()

const loading    = ref(false)
const exporting  = ref(false)

const filterKeyword = ref('')
const filterModule  = ref('')
const filterAction  = ref('')
const filterResult  = ref('')
const filterRange   = ref<string[]>([])

const currentDetail = ref<AuditLogRow | null>(null)
const detailVisible = ref(false)

const auditLogs = ref<AuditLogRow[]>([])
const total     = ref(0)

const filtered = computed(() => auditLogs.value)

const totalCount  = computed(() => total.value)
const resultKey = (result: string): 'success' | 'failed' | 'rejected' => {
  const value = result.toLowerCase()
  if (result === '成功' || value === 'success') {
    return 'success'
  }
  if (result === '失败' || value === 'failed' || value === 'failure') {
    return 'failed'
  }
  return 'rejected'
}

// rowAction prefers the `actionType` field (the canonical category like
// "编辑" / "登录" set by writeOpLog) and falls back to the legacy `action`
// field so older log rows or external integrations that only populate one
// of the two still render correctly. Without this fallback every row
// shows "吊销" because the frontend used to read the empty `action`
// column and hit the catch-all.
const rowAction = (row: AuditLogRow): string => row.actionType || row.action || ''

type ActionKey =
  | 'create' | 'edit' | 'delete' | 'login' | 'logout' | 'config' | 'sync'
  | 'revoke' | 'test' | 'export' | 'backup' | 'restore' | 'upload' | 'unlock'
  | 'unknown'

const actionKey = (row: AuditLogRow | string): ActionKey => {
  const action = typeof row === 'string' ? row : rowAction(row)
  const value = action.toLowerCase()
  if (action === '新增' || value === 'create') return 'create'
  if (action === '编辑' || action === '修改' || action === '修改密码' || value === 'edit' || value === 'update') return 'edit'
  if (action === '删除' || value === 'delete' || value === 'remove') return 'delete'
  if (action === '登录' || value === 'login' || value === 'signin') return 'login'
  if (action === '登出' || value === 'logout' || value === 'signout') return 'logout'
  if (action === '配置' || value === 'config') return 'config'
  if (action === '同步' || action === '自动同步' || value === 'sync') return 'sync'
  if (action === '吊销' || value === 'revoke') return 'revoke'
  if (action === '测试' || value === 'test') return 'test'
  if (action === '导出' || value === 'export') return 'export'
  if (action === '备份' || value === 'backup') return 'backup'
  if (action === '还原' || value === 'restore') return 'restore'
  if (action === '上传' || value === 'upload') return 'upload'
  if (action === '解锁账号' || action === '解锁' || value === 'unlock') return 'unlock'
  return 'unknown'
}

// moduleLabel translates the Chinese module name persisted by the
// backend into the active locale. Unknown modules pass through verbatim
// so partner/external integrations (or future modules added before the
// frontend ships) still render meaningfully instead of becoming
// "unknown" placeholders.
const moduleI18nKeyByCN: Record<string, string> = {
  '区域管理': 'audit.moduleMap.zone',
  '转发管理': 'audit.moduleMap.forward',
  '缓存管理': 'audit.moduleMap.cache',
  '安全中心': 'audit.moduleMap.security',
  '集群管理': 'audit.moduleMap.cluster',
  '用户管理': 'audit.moduleMap.user',
  '系统设置': 'audit.moduleMap.setting',
  'API密钥':  'audit.moduleMap.apiKey',
  '认证中心': 'audit.moduleMap.auth',
  '身份认证': 'audit.moduleMap.auth', // legacy rows
  '备份还原': 'audit.moduleMap.backup',
  '通知设置': 'audit.moduleMap.notice',
  '审计日志': 'audit.moduleMap.auditLog',
}
const moduleLabel = (m: string): string => {
  const key = moduleI18nKeyByCN[m]
  return key ? t(key) : (m || '')
}

// channelLabel maps backend's Chinese channel names (used inside
// composed Target strings) to localized display text. Falls back to the
// raw text so any future channel that hasn't been added here still
// shows something sensible.
// The backend's channelDisplayName helper writes these specific
// Chinese strings into the operation_logs target column. Keep this
// table in sync with backend/internal/handler/setting.go::channelDisplayName
// (and chatPlatformLabel) — diverging keys would silently fall back to
// raw Chinese strings on the audit page, which is what triggered the
// original "English page shows Chinese" report.
const channelI18nKeyByCN: Record<string, string> = {
  '邮件':   'audit.channelMap.email',
  '邮箱':   'audit.channelMap.email', // legacy rows from earlier builds
  '钉钉':   'audit.channelMap.dingtalk',
  '企业微信': 'audit.channelMap.wechat',
  '飞书':   'audit.channelMap.feishu',
  'Slack':  'audit.channelMap.slack',
  '电话':   'audit.channelMap.voice',
  'Webhook': 'audit.channelMap.webhook',
  '短信':   'audit.channelMap.sms',
}
const localizeChannel = (cn: string): string => {
  const key = channelI18nKeyByCN[cn]
  return key ? t(key) : cn
}

// API-key status names are localized via apiKeyStatusMap. Backend
// writes `正常` / `禁用` literally for the toggle target.
const localizeApiKeyStatus = (cn: string): string => {
  if (cn === '正常') return t('audit.apiKeyStatusMap.active')
  if (cn === '禁用') return t('audit.apiKeyStatusMap.disabled')
  return cn
}

// Pattern table for the Target column. Each entry pairs a regex (built
// against the canonical Chinese phrase the backend writes) with a
// renderer that produces the localized string. Order matters — the
// first match wins, so more specific patterns must precede generic
// ones (e.g. "重命名角色" before "保存角色"). Patterns missing here
// fall through to the raw target text, which keeps unknown audit rows
// from breaking the page.
const targetPatterns: Array<{ re: RegExp; render: (m: RegExpMatchArray) => string }> = [
  // ─── Settings / general ─────────────────────────────────────────
  { re: /^保存常规配置$/,           render: () => t('audit.targetMap.saveCommonConfig') },
  { re: /^保存通知配置$/,           render: () => t('audit.targetMap.saveNoticeConfig') },
  { re: /^手动清理过期日志$/,       render: () => t('audit.targetMap.purgeExpiredLogs') },
  // ─── Notification channels ──────────────────────────────────────
  { re: /^保存\s+(.+?)\s+通道配置$/,
    render: (m) => t('audit.targetMap.saveChannelConfig', { channel: localizeChannel(m[1]) }) },
  { re: /^(.+?)\s+通道测试成功$/,
    render: (m) => t('audit.targetMap.channelTestSuccess', { channel: localizeChannel(m[1]) }) },
  { re: /^(.+?)\s+通道测试失败：(.*)$/,
    render: (m) => t('audit.targetMap.channelTestFailed', { channel: localizeChannel(m[1]), reason: m[2] }) },
  // ─── Auth ───────────────────────────────────────────────────────
  { re: /^控制台登录成功$/,         render: () => t('audit.targetMap.consoleLogin') },
  { re: /^进入 MFA 强制绑定流程$/,  render: () => t('audit.targetMap.mfaEnroll') },
  { re: /^用户修改了登录密码$/,     render: () => t('audit.targetMap.changePassword') },
  { re: /^退出登录$/,               render: () => t('audit.targetMap.logout') },
  // ─── Users ──────────────────────────────────────────────────────
  { re: /^创建用户\s+(.+)$/,        render: (m) => t('audit.targetMap.createUser',  { name: m[1] }) },
  { re: /^保存用户\s+(.+)$/,        render: (m) => t('audit.targetMap.saveUser',    { name: m[1] }) },
  { re: /^删除用户\s+(.+)$/,        render: (m) => t('audit.targetMap.deleteUser',  { name: m[1] }) },
  { re: /^重置用户\s+(.+?)\s+的密码$/,
    render: (m) => t('audit.targetMap.resetUserPassword', { name: m[1] }) },
  // ─── Roles ──────────────────────────────────────────────────────
  // 重命名/配置 must come before the generic 创建/保存/删除 角色 patterns.
  { re: /^重命名角色\s+(.+?)\s*→\s*(.+)$/,
    render: (m) => t('audit.targetMap.renameRole', { from: m[1], to: m[2] }) },
  { re: /^配置角色\s+(.+?)\s+权限（共\s*(\d+)\s*项）$/,
    render: (m) => t('audit.targetMap.configRolePermissions', { name: m[1], count: m[2] }) },
  { re: /^创建角色\s+(.+)$/,        render: (m) => t('audit.targetMap.createRole', { name: m[1] }) },
  { re: /^保存角色\s+(.+)$/,        render: (m) => t('audit.targetMap.saveRole',   { name: m[1] }) },
  { re: /^删除角色\s+(.+)$/,        render: (m) => t('audit.targetMap.deleteRole', { name: m[1] }) },
  // ─── Backups ────────────────────────────────────────────────────
  { re: /^上传备份文件\s+(.+?)（(.+?)）$/,
    render: (m) => t('audit.targetMap.uploadBackup', { name: m[1], size: m[2] }) },
  { re: /^从文件还原\s+(.+)$/,      render: (m) => t('audit.targetMap.restoreFromFile', { name: m[1] }) },
  { re: /^还原备份\s+(.+)$/,        render: (m) => t('audit.targetMap.restoreBackup',   { name: m[1] }) },
  // 创建备份 may include a parenthesized scope summary; capture the
  // whole tail as the backup identifier+context.
  { re: /^创建备份\s+(.+)$/,        render: (m) => t('audit.targetMap.createBackup', { name: m[1] }) },
  { re: /^删除备份\s+(.+)$/,        render: (m) => t('audit.targetMap.deleteBackup', { name: m[1] }) },
  // ─── Audit log export ───────────────────────────────────────────
  { re: /^导出\s+(\S+)\s+格式\s*·\s*共\s*(\d+)\s*条（已截断）$/,
    render: (m) => t('audit.targetMap.exportLogsTruncated', { format: m[1], count: m[2] }) },
  { re: /^导出\s+(\S+)\s+格式\s*·\s*共\s*(\d+)\s*条$/,
    render: (m) => t('audit.targetMap.exportLogs', { format: m[1], count: m[2] }) },
  // ─── API keys ───────────────────────────────────────────────────
  // Toggle target is "{name} → {正常|禁用}". Anchor on the arrow with
  // the trailing token in the known set so plain key names (which can
  // contain arrows) don't get misinterpreted.
  { re: /^(.+?)\s+→\s+(正常|禁用)$/,
    render: (m) => t('audit.targetMap.apiKeyToggle', { name: m[1], status: localizeApiKeyStatus(m[2]) }) },
]

const targetLabel = (row: AuditLogRow): string => {
  const raw = row.target || ''
  for (const p of targetPatterns) {
    const m = raw.match(p.re)
    if (m) return p.render(m)
  }
  return raw
}

const actionLabel = (row: AuditLogRow) => {
  const key = actionKey(row)
  // For unmapped categories fall back to the raw text so non-standard
  // actionTypes (e.g. "重置") still render meaningfully instead of the
  // catch-all "吊销" badge that was leaking through previously.
  if (key === 'unknown') return rowAction(row) || t('common.unknown')
  return t(`audit.actionMap.${key}`)
}
const resultLabel = (result: string) => t(`audit.resultMap.${resultKey(result)}`)

// formatTime renders the backend's RFC3339 timestamp (e.g.
// 2026-05-06T10:44:52.415+08:00) into a compact "YYYY-MM-DD HH:mm:ss"
// form so the Audit table column doesn't have to grow extra wide. The
// backend keeps the precise stamp for export.
const formatTime = (raw: string): string => {
  if (!raw) return '—'
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

const failedCount = computed(() => auditLogs.value.filter(r => resultKey(r.result) !== 'success').length)
const operators   = computed(() => new Set(auditLogs.value.map(r => r.operator)).size)
const modules     = computed(() => new Set(auditLogs.value.map(r => r.module)).size)

const actionClass = (row: AuditLogRow | string) => {
  const m: Record<string, string> = {
    create: 'act-add',
    edit: 'act-edit',
    delete: 'act-del',
    login: 'act-login',
    logout: 'act-logout',
    config: 'act-config',
    sync: 'act-sync',
    revoke: 'act-revoke',
    test: 'act-config',
    export: 'act-sync',
  }
  return m[actionKey(row)] ?? ''
}
const resultClass = (r: string) =>
  resultKey(r) === 'success' ? 'mn-badge--success' : resultKey(r) === 'failed' ? 'mn-badge--danger' : 'mn-badge--warning'

// The `value` here is the canonical Chinese string the backend writes
// to the operation_logs.module column — the filter posts it back
// verbatim and the WHERE clause does an exact string compare. Adding
// or renaming a value without updating the backend write side would
// silently filter to zero rows.
const moduleOptions = computed(() => [
  { value: '区域管理', label: t('audit.moduleMap.zone') },
  { value: '转发管理', label: t('audit.moduleMap.forward') },
  { value: '缓存管理', label: t('audit.moduleMap.cache') },
  { value: '安全中心', label: t('audit.moduleMap.security') },
  { value: '集群管理', label: t('audit.moduleMap.cluster') },
  { value: '用户管理', label: t('audit.moduleMap.user') },
  { value: '系统设置', label: t('audit.moduleMap.setting') },
  { value: 'API密钥',  label: t('audit.moduleMap.apiKey') },
  { value: '认证中心', label: t('audit.moduleMap.auth') },
  { value: '备份还原', label: t('audit.moduleMap.backup') },
  { value: '通知设置', label: t('audit.moduleMap.notice') },
  { value: '审计日志', label: t('audit.moduleMap.auditLog') },
])

const actionOptions = computed(() => [
  { value: '新增', label: t('audit.actionMap.create') },
  { value: '编辑', label: t('audit.actionMap.edit') },
  { value: '删除', label: t('audit.actionMap.delete') },
  { value: '登录', label: t('audit.actionMap.login') },
  { value: '登出', label: t('audit.actionMap.logout') },
  { value: '配置', label: t('audit.actionMap.config') },
  { value: '同步', label: t('audit.actionMap.sync') },
  { value: '吊销', label: t('audit.actionMap.revoke') },
  { value: '备份', label: t('audit.actionMap.backup') },
  { value: '还原', label: t('audit.actionMap.restore') },
  { value: '上传', label: t('audit.actionMap.upload') },
  { value: '导出', label: t('audit.actionMap.export') },
])

const resultOptions = computed(() => [
  { value: '成功', label: t('audit.resultMap.success') },
  { value: '失败', label: t('audit.resultMap.failed') },
  { value: '拒绝', label: t('audit.resultMap.rejected') },
])

const parseDiff = (before?: string, after?: string) => {
  if (!before && !after) return []
  const b = before ? JSON.parse(before) : {}
  const a = after  ? JSON.parse(after)  : {}
  const keys = new Set([...Object.keys(b), ...Object.keys(a)])
  return Array.from(keys).map(k => ({
    key: k,
    before: b[k] !== undefined ? JSON.stringify(b[k]) : '—',
    after:  a[k] !== undefined ? JSON.stringify(a[k]) : '—',
    changed: JSON.stringify(b[k]) !== JSON.stringify(a[k]),
  }))
}

const loadLogs = async () => {
  loading.value = true
  try {
    const { data } = await getAuditLogsApi({
      keyword:   filterKeyword.value || undefined,
      module:    filterModule.value  || undefined,
      action:    filterAction.value  || undefined,
      result:    filterResult.value  || undefined,
      startTime: filterRange.value?.[0] || undefined,
      endTime:   filterRange.value?.[1] ? filterRange.value[1] + ' 23:59:59' : undefined,
    })
    auditLogs.value = data.rows ?? []
    total.value     = data.total ?? 0
  } finally {
    loading.value = false
  }
}

const openDetail = (row: AuditLogRow) => {
  currentDetail.value = row
  detailVisible.value = true
}

const handleExport = async () => {
  // Send the same filter state that the table is rendering with so the
  // exported file matches "what you see". The backend re-runs the query
  // server-side rather than trusting the in-memory `filtered` slice,
  // which means we get up to 100k rows instead of just the current page.
  exporting.value = true
  try {
    const [startTime, endTime] = filterRange.value || []
    const result = await exportAuditLogsApi({
      format: 'csv',
      keyword:   filterKeyword.value || undefined,
      module:    filterModule.value  || undefined,
      action:    filterAction.value  || undefined,
      result:    filterResult.value  || undefined,
      startTime: startTime || undefined,
      endTime:   endTime   || undefined,
    })
    if (result.truncated) {
      ElMessage.warning(t('audit.exportTruncated', { count: result.count }))
    } else {
      ElMessage.success(t('audit.exportDone', { count: result.count }))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || t('audit.exportFailed'))
  } finally {
    exporting.value = false
  }
}

const handleRefresh = () => loadLogs()

const handleReset = () => {
  filterKeyword.value = ''
  filterModule.value  = ''
  filterAction.value  = ''
  filterResult.value  = ''
  filterRange.value   = []
  loadLogs()
}

// ── Log configuration dialog ──────────────────────────────────────────
//
// The audit page itself only reads logs; the knobs that govern *how* logs
// are kept (level, retention window, export format, syslog forwarder)
// live on SystemConfig and are also reachable from the General Settings
// page. We surface them again here in a focused modal so an operator
// triaging the audit table doesn't have to navigate away to extend the
// retention window or trigger an immediate purge.
//
// State is local: we hydrate on dialog open from the same /setting/common
// endpoint and persist via the same SaveCommonConfig handler — no
// separate API surface, no risk of drift with the General Settings panel.
const configDialogVisible = ref(false)
const configLoading       = ref(false)
const configSaving        = ref(false)
const purging             = ref(false)
const configForm = reactive<GeneralConfigForm>(createDefaultGeneralConfigForm())

const openLogConfig = async () => {
  configDialogVisible.value = true
  configLoading.value = true
  try {
    const { data } = await getSettingModuleDataApi()
    Object.assign(configForm, mergeGeneralConfigForm(data.common))
  } catch (_e) {
    ElMessage.error(t('audit.logConfigLoadFailed'))
  } finally {
    configLoading.value = false
  }
}

const saveLogConfig = async () => {
  // Range-clamp here mirrors the General Settings panel so the audit-page
  // entry can't accidentally save out-of-range values. Backend would
  // reject these too, but we'd rather fail fast in the UI.
  const days = Number(configForm.logRetentionDays)
  if (!Number.isFinite(days) || days < 1 || days > 730) {
    ElMessage.error(t('audit.logRetentionRange'))
    return
  }
  if (configForm.syslogEnabled && !String(configForm.syslogServer || '').trim()) {
    ElMessage.error(t('audit.syslogServerRequired'))
    return
  }
  configSaving.value = true
  try {
    // Scoped writer: only sends the 5 log-related columns. Replaces the
    // earlier saveCommonConfigApi round-trip which posted the *entire*
    // GeneralConfigForm and silently reverted any other-tab edits the
    // operator had made between dialog-open and dialog-save (a real
    // last-writer-wins bug, not a hypothetical one).
    await saveLogConfigApi({
      logLevel: configForm.logLevel,
      logRetentionDays: configForm.logRetentionDays,
      logExportFormat: configForm.logExportFormat,
      syslogEnabled: configForm.syslogEnabled,
      syslogServer: configForm.syslogServer,
    })
    ElMessage.success(t('audit.logConfigSaved'))
    configDialogVisible.value = false
  } catch (_e) {
    ElMessage.error(t('audit.logConfigSaveFailed'))
  } finally {
    configSaving.value = false
  }
}

const purgeExpiredNow = async () => {
  try {
    await confirmRiskAction({
      title: t('audit.purgeConfirmTitle'),
      action: t('audit.purgeConfirmAction'),
      risk: t('audit.purgeConfirmRisk', { days: configForm.logRetentionDays }),
    })
  } catch (_e) {
    return
  }
  purging.value = true
  try {
    const { data } = await purgeExpiredAuditLogsApi()
    ElMessage.success(t('audit.purgeDone', { count: data?.deleted ?? 0 }))
    // Refresh the underlying table so the now-empty pre-cutoff rows
    // disappear without forcing the operator to hit the refresh button.
    void loadLogs()
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || t('audit.purgeFailed')
    ElMessageBox.alert(msg, t('audit.purgeFailed'), { type: 'error' })
  } finally {
    purging.value = false
  }
}

onMounted(loadLogs)
</script>

<template>
  <div class="page-shell al-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('audit.title') }}</h1>
        <p class="page-subtitle">{{ $t('audit.subtitle') }}</p>
      </div>
      <div style="display:flex;gap:8px">
        <el-button :loading="loading" @click="handleRefresh">
          <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
          {{ $t('common.refresh') }}
        </el-button>
        <el-button @click="openLogConfig">
          <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg></template>
          {{ $t('audit.logConfig') }}
        </el-button>
        <el-button type="primary" plain :loading="exporting" @click="handleExport">
          <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
          {{ $t('common.export') }}
        </el-button>
      </div>
    </div>

    <!-- KPI -->
    <div class="al-kpi">
      <div class="al-kpi-tile">
        <div class="al-kpi-icon al-kpi-icon--blue">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6zm-1 7V3.5L18.5 9H13z"/></svg>
        </div>
        <div><div class="al-kpi-val">{{ totalCount }}</div><div class="al-kpi-lbl">{{ $t('audit.kpi.totalLogs') }}</div></div>
      </div>
      <div class="al-kpi-tile">
        <div class="al-kpi-icon al-kpi-icon--purple">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
        </div>
        <div><div class="al-kpi-val">{{ operators }}</div><div class="al-kpi-lbl">{{ $t('audit.kpi.operators') }}</div></div>
      </div>
      <div class="al-kpi-tile">
        <div class="al-kpi-icon al-kpi-icon--green">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-5 14H7v-2h7v2zm3-4H7v-2h10v2zm0-4H7V7h10v2z"/></svg>
        </div>
        <div><div class="al-kpi-val">{{ modules }}</div><div class="al-kpi-lbl">{{ $t('audit.kpi.modules') }}</div></div>
      </div>
      <div class="al-kpi-tile" :class="{ 'al-kpi-tile--warn': failedCount > 0 }">
        <div class="al-kpi-icon" :class="failedCount > 0 ? 'al-kpi-icon--red' : 'al-kpi-icon--gray'">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>
        </div>
        <div><div class="al-kpi-val" :style="{ color: failedCount > 0 ? 'var(--app-danger)' : 'var(--app-disabled)' }">{{ failedCount }}</div><div class="al-kpi-lbl">{{ $t('audit.kpi.failedOrRejected') }}</div></div>
      </div>
    </div>

    <!-- Filters -->
    <el-card class="mn-card">
      <div class="al-toolbar">
        <div class="al-filters">
          <el-input v-model="filterKeyword" clearable :placeholder="$t('audit.searchPlaceholder')" style="width:260px">
            <template #prefix><svg viewBox="0 0 24 24" fill="currentColor" width="13" height="13" style="color:var(--app-text-placeholder)"><path d="M15.5 14h-.79l-.28-.27A6.471 6.471 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg></template>
          </el-input>
          <el-select v-model="filterModule" clearable :placeholder="$t('audit.module')" style="width:110px">
            <el-option v-for="m in moduleOptions" :key="m.value" :label="m.label" :value="m.value" />
          </el-select>
          <el-select v-model="filterAction" clearable :placeholder="$t('audit.action')" style="width:110px">
            <el-option v-for="a in actionOptions" :key="a.value" :label="a.label" :value="a.value" />
          </el-select>
          <el-select v-model="filterResult" clearable :placeholder="$t('audit.result')" style="width:100px">
            <el-option v-for="r in resultOptions" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
          <el-date-picker v-model="filterRange" type="daterange" :range-separator="$t('common.to')" :start-placeholder="$t('common.startDate')" :end-placeholder="$t('common.endDate')" value-format="YYYY-MM-DD" style="width:240px" />
          <el-button @click="handleReset">{{ $t('common.reset') }}</el-button>
        </div>
        <span class="al-result-count">{{ $t('common.total') }} {{ filtered.length }} {{ $t('common.items') }}</span>
      </div>

      <!-- Table -->
      <el-table :data="filtered" v-loading="loading" stripe class="mn-table al-table" table-layout="auto">
        <el-table-column prop="id" :label="$t('audit.logId')" width="180">
          <template #default="{ row }"><span class="mn-mono al-id">{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column prop="operator" :label="$t('audit.operator')" width="130">
          <template #default="{ row }">
            <div class="op-cell">
              <span class="op-avatar">{{ row.operator.charAt(0).toUpperCase() }}</span>
              <div class="op-info">
                <span class="op-name">{{ row.operator }}</span>
                <span class="op-role">{{ row.operatorRole }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="clientIp" :label="$t('audit.sourceIp')" width="145">
          <template #default="{ row }"><span class="mn-mono al-ip">{{ row.clientIp }}</span></template>
        </el-table-column>
        <el-table-column prop="module" :label="$t('audit.module')" width="120">
          <template #default="{ row }">{{ moduleLabel(row.module) }}</template>
        </el-table-column>
        <el-table-column prop="action" :label="$t('audit.action')" width="80" align="center">
          <template #default="{ row }">
            <span :class="['al-action', actionClass(row)]">{{ actionLabel(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="target" :label="$t('audit.target')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ targetLabel(row) }}</template>
        </el-table-column>
        <el-table-column prop="result" :label="$t('audit.result')" width="80" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', resultClass(row.result)]">{{ resultLabel(row.result) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="duration" :label="$t('audit.duration')" width="80" align="right">
          <template #default="{ row }"><span class="mn-mono al-dur">{{ row.duration }} ms</span></template>
        </el-table-column>
        <el-table-column prop="time" :label="$t('audit.time')" width="175" sortable>
          <template #default="{ row }"><span class="mn-time" :title="row.time">{{ formatTime(row.time) }}</span></template>
        </el-table-column>
        <el-table-column label="" width="80" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">
              <template v-if="row.before || row.after">Diff</template>
              <template v-else>{{ $t('common.detail') }}</template>
            </el-button>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('audit.empty')" /></template>
      </el-table>
    </el-card>

    <!-- Detail / Diff drawer -->
    <el-drawer
      v-model="detailVisible"
      :title="$t('audit.detailTitle', { id: currentDetail?.id ?? '' })"
      size="520px"
      append-to-body
    >
      <template v-if="currentDetail" #default>
        <div class="detail-body">
          <!-- Header badges -->
          <div class="detail-header">
            <span :class="['al-action', actionClass(currentDetail)]">{{ actionLabel(currentDetail) }}</span>
            <span :class="['mn-badge', resultClass(currentDetail.result)]">{{ resultLabel(currentDetail.result) }}</span>
            <span class="detail-module">{{ moduleLabel(currentDetail.module) }}</span>
            <span class="mn-time" style="margin-left:auto" :title="currentDetail.time">{{ formatTime(currentDetail.time) }}</span>
          </div>

          <!-- KV grid -->
          <div class="detail-section">
            <div class="detail-section-title">{{ $t('audit.detailOperationInfo') }}</div>
            <div class="detail-kv">
              <span class="kv-lbl">{{ $t('audit.logId') }}</span>    <code class="kv-val mn-mono">{{ currentDetail.id }}</code>
              <span class="kv-lbl">{{ $t('audit.operator') }}</span>      <span class="kv-val">{{ currentDetail.operator }}（{{ currentDetail.operatorRole }}）</span>
              <span class="kv-lbl">{{ $t('audit.sourceIp') }}</span>     <code class="kv-val mn-mono">{{ currentDetail.clientIp }}</code>
              <span class="kv-lbl">{{ $t('audit.target') }}</span>    <span class="kv-val">{{ targetLabel(currentDetail) }}</span>
              <span class="kv-lbl">{{ $t('audit.duration') }}</span>    <span class="kv-val mn-mono">{{ currentDetail.duration }} ms</span>
            </div>
          </div>

          <!-- Diff section -->
          <div v-if="currentDetail.before || currentDetail.after" class="detail-section">
            <div class="detail-section-title">{{ $t('audit.diffTitle') }}</div>
            <div class="diff-header-row">
              <span>{{ $t('audit.diffField') }}</span>
              <span>{{ $t('audit.diffBefore') }}</span>
              <span>{{ $t('audit.diffAfter') }}</span>
            </div>
            <div
              v-for="item in parseDiff(currentDetail.before, currentDetail.after)"
              :key="item.key"
              :class="['diff-row', { 'diff-row--changed': item.changed }]"
            >
              <span class="diff-key">{{ item.key }}</span>
              <code class="diff-val diff-val--before">{{ item.before }}</code>
              <code class="diff-val" :class="item.changed ? 'diff-val--after-changed' : 'diff-val--after-same'">{{ item.after }}</code>
            </div>
          </div>

          <div v-else class="no-diff">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><circle cx="12" cy="12" r="10"/><path d="M12 8v4m0 4h.01"/></svg>
            {{ $t('audit.noDiff') }}
          </div>
        </div>
      </template>
    </el-drawer>

    <!--
      Log configuration dialog. Backed by the same /setting/common
      endpoint that General Settings uses, but scoped to the log knobs
      so an operator can adjust retention / forwarder without leaving
      the audit page. The "立即清理" button calls the dedicated purge
      endpoint and refreshes the table on success.
    -->
    <el-dialog
      v-model="configDialogVisible"
      :title="$t('audit.logConfigTitle')"
      width="540px"
      destroy-on-close
    >
      <el-form
        v-loading="configLoading"
        :model="configForm"
        label-position="top"
        class="al-cfg-form"
      >
        <!-- LogLevel dropdown intentionally removed in 2026-05.
             Background: the backend never read system_config.log_level
             — logging goes through stdlib `log.Printf` at a single
             implicit level. Surfacing the dropdown was misleading
             operators (“I set it to ERROR but DEBUG entries still
             appear”). The DB column is kept for backwards compat; the
             UI knob is gone until we wire structured logging (slog).
             Retention is the only knob in this row that actually
             ticks the scheduler, so it gets the full width. -->
        <el-form-item :label="$t('setting.logRetention')">
          <el-input-number
            v-model="configForm.logRetentionDays"
            :min="1"
            :max="730"
            :step="1"
            controls-position="right"
            class="full-width"
          />
        </el-form-item>

        <el-form-item :label="$t('setting.logExportFormat')">
          <el-select v-model="configForm.logExportFormat" class="full-width">
            <el-option label="CSV" value="csv" />
            <el-option label="JSON" value="json" />
            <el-option :label="$t('setting.syslogPush')" value="syslog" />
          </el-select>
        </el-form-item>

        <div class="al-cfg-divider">
          <span>{{ $t('audit.syslogSection') }}</span>
        </div>

        <div class="al-cfg-switch-row">
          <div>
            <div class="al-cfg-switch-label">{{ $t('audit.syslogEnabled') }}</div>
            <div class="al-cfg-switch-desc">{{ $t('audit.syslogEnabledDesc') }}</div>
          </div>
          <el-switch v-model="configForm.syslogEnabled" />
        </div>

        <el-form-item :label="$t('audit.syslogServer')">
          <el-input
            v-model="configForm.syslogServer"
            :disabled="!configForm.syslogEnabled"
            :placeholder="$t('audit.syslogServerPlaceholder')"
            clearable
          />
        </el-form-item>

        <div class="al-cfg-divider">
          <span>{{ $t('audit.purgeSection') }}</span>
        </div>

        <div class="al-cfg-purge-row">
          <div class="al-cfg-purge-info">
            <div class="al-cfg-purge-label">{{ $t('audit.purgeNow') }}</div>
            <div class="al-cfg-purge-desc">
              {{ $t('audit.purgeDesc', { days: configForm.logRetentionDays }) }}
            </div>
          </div>
          <el-button
            type="danger"
            plain
            :loading="purging"
            :disabled="configLoading || configForm.logRetentionDays < 1"
            @click="purgeExpiredNow"
          >
            {{ $t('audit.purgeNowAction') }}
          </el-button>
        </div>
      </el-form>

      <template #footer>
        <div style="display:flex;justify-content:flex-end;gap:8px">
          <el-button @click="configDialogVisible = false">{{ $t('common.cancel') }}</el-button>
          <el-button
            type="primary"
            :loading="configSaving"
            :disabled="configLoading"
            @click="saveLogConfig"
          >
            {{ $t('common.save') }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.al-page :deep(.el-card__body) { padding: 0; }

/* KPI */
.al-kpi { display:grid; grid-template-columns:repeat(4,1fr); gap:14px; }
.al-kpi-tile {
  display: flex; align-items: center; gap: 14px;
  padding: 14px 20px; border-radius: 10px;
  border: 1px solid var(--dns-border-color);
  background: var(--dns-bg-card);
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow .2s;
}
.al-kpi-tile:hover { box-shadow: 0 4px 16px rgba(0,0,0,0.08); }
.al-kpi-tile--warn { border-color: rgba(245,63,63,0.3); }
.al-kpi-icon { width:40px; height:40px; display:flex; align-items:center; justify-content:center; border-radius:8px; flex-shrink:0; }
.al-kpi-icon--blue   { background:rgba(22,93,255,0.1);  color:var(--app-accent); }
.al-kpi-icon--purple { background:rgba(114,46,209,0.1); color:#722ED1; }
.al-kpi-icon--green  { background:rgba(0,180,42,0.1);   color:var(--app-success); }
.al-kpi-icon--red    { background:rgba(245,63,63,0.1);  color:var(--app-danger); }
.al-kpi-icon--gray   { background:var(--dns-bg-page);   color:var(--app-disabled); }
.al-kpi-val { font-size:26px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; color:var(--dns-text-title-color); }
.al-kpi-lbl { font-size:12px; color:var(--dns-text-body-color); margin-top:3px; }

/* Toolbar */
.al-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  flex-wrap: wrap; gap: 10px;
  padding: 12px 16px;
  background: var(--dns-bg-page);
  border-bottom: 1px solid var(--dns-border-color);
}
.al-filters { display:flex; align-items:center; gap:8px; flex-wrap:wrap; flex:1; }
.al-result-count { font-size:12px; color:var(--dns-text-assist-color); flex-shrink:0; }

/* Table */
.al-table { width:100%; }
.al-id   { font-size:11px; color:var(--dns-text-assist-color); }
.al-ip   { font-size:12px; }
.al-dur  { font-size:12px; color:var(--dns-text-assist-color); }

.op-cell { display:flex; align-items:center; gap:7px; }
.op-avatar { width:24px; height:24px; border-radius:50%; background:var(--app-accent-soft); color:var(--app-accent); font-size:11px; font-weight:700; display:flex; align-items:center; justify-content:center; flex-shrink:0; }
.op-info { display:flex; flex-direction:column; gap:1px; }
.op-name { font-size:13px; font-weight:500; color:var(--dns-text-title-color); line-height:1.2; }
.op-role { font-size:11px; color:var(--dns-text-assist-color); line-height:1.2; }

/* Action badges */
.al-action { display:inline-block; padding:2px 7px; border-radius:4px; font-size:11px; font-weight:600; border:1px solid transparent; white-space:nowrap; }
.act-add    { background:rgba(0,180,42,0.08);   color:var(--app-success); border-color:rgba(0,180,42,0.22); }
.act-edit   { background:rgba(22,93,255,0.08);  color:var(--app-accent);  border-color:rgba(22,93,255,0.22); }
.act-del    { background:rgba(245,63,63,0.08);  color:var(--app-danger);  border-color:rgba(245,63,63,0.22); }
.act-login  { background:rgba(114,46,209,0.08); color:#722ED1;             border-color:rgba(114,46,209,0.2); }
.act-logout { background:rgba(134,144,156,0.1); color:var(--app-disabled); border-color:rgba(134,144,156,0.2); }
.act-config { background:rgba(255,125,0,0.08);  color:var(--app-warning); border-color:rgba(255,125,0,0.22); }
.act-sync   { background:rgba(22,93,255,0.08);  color:var(--app-accent);  border-color:rgba(22,93,255,0.22); }
.act-revoke { background:rgba(245,63,63,0.08);  color:var(--app-danger);  border-color:rgba(245,63,63,0.22); }

/* Detail drawer */
.detail-body { display:flex; flex-direction:column; gap:16px; padding:4px 0; }
.detail-header { display:flex; align-items:center; gap:8px; flex-wrap:wrap; padding-bottom:4px; border-bottom:1px solid var(--dns-border-color); }
.detail-module { font-size:13px; font-weight:600; color:var(--dns-text-title-color); }

.detail-section { display:flex; flex-direction:column; gap:8px; }
.detail-section-title { font-size:11px; font-weight:700; text-transform:uppercase; letter-spacing:0.06em; color:var(--dns-text-assist-color); }
.detail-kv { display:grid; grid-template-columns:80px 1fr; gap:8px 12px; font-size:13px; align-items:start; }
.kv-lbl { color:var(--dns-text-assist-color); font-size:12px; }
.kv-val { color:var(--dns-text-title-color); word-break:break-all; }

/* Diff */
.diff-header-row {
  display:grid; grid-template-columns:100px 1fr 1fr;
  padding:6px 10px; background:var(--dns-bg-page);
  font-size:11px; font-weight:700; text-transform:uppercase;
  letter-spacing:0.05em; color:var(--dns-text-assist-color);
  border:1px solid var(--dns-border-color); border-radius:6px 6px 0 0;
  border-bottom:none;
}
.diff-row {
  display:grid; grid-template-columns:100px 1fr 1fr;
  padding:8px 10px; border:1px solid var(--dns-border-color);
  border-top: none; align-items:center; gap:8px;
}
.diff-row:last-child { border-radius:0 0 6px 6px; }
.diff-row--changed { background:rgba(255,125,0,0.03); }
.diff-key { font-size:12px; font-weight:600; color:var(--dns-text-title-color); }
.diff-val { font-family:'JetBrains Mono','Consolas',monospace; font-size:11px; padding:2px 7px; border-radius:4px; word-break:break-all; }
.diff-val--before       { background:rgba(134,144,156,0.1); color:var(--dns-text-body-color); }
.diff-val--after-same   { background:rgba(0,180,42,0.08); color:var(--app-success); }
.diff-val--after-changed{ background:rgba(22,93,255,0.08); color:var(--app-accent); }

.no-diff { display:flex; align-items:center; gap:8px; padding:16px; color:var(--dns-text-assist-color); font-size:13px; background:var(--dns-bg-page); border-radius:8px; border:1px solid var(--dns-border-color); }

@media (max-width: 1200px) { .al-kpi { grid-template-columns:repeat(2,1fr); } }
@media (max-width: 700px)  { .al-kpi { grid-template-columns:1fr; } }

/* ── Log config dialog ─────────────────────────────────── */
.al-cfg-form .full-width { width: 100%; }
.al-cfg-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}
.al-cfg-divider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 8px 0 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-regular);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.al-cfg-divider::before,
.al-cfg-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--app-border);
}
.al-cfg-switch-row,
.al-cfg-purge-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  margin-bottom: 14px;
  background: var(--app-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
.al-cfg-switch-label,
.al-cfg-purge-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}
.al-cfg-switch-desc,
.al-cfg-purge-desc {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
  line-height: 1.5;
}
.al-cfg-purge-info { flex: 1; min-width: 0; }
</style>
