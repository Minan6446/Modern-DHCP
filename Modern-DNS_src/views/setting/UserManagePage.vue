<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
  addUser,
  deleteUser,
  editUser,
  getUserList,
  getUserLockStatusApi,
  unlockUserApi,
  kickUserSessionsApi,
  listOnlineSessionsApi,
  revokeOnlineSessionApi,
  type UserLockStatus,
  type OnlineSession,
} from '../../api/setting'
import { useCrud } from '../../composables/core/useCrud'
import { useSettingStore } from '../../stores/setting'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'
import type { SettingRole, SettingUser } from '../../types/modules'

type PermissionTreeRef = {
  setCheckedKeys: (keys: string[]) => void
  getCheckedKeys: () => string[]
}

type UserFormModel = Partial<SettingUser> & {
  password: string
}

const route = useRoute()
const { t } = useI18n()
const settingStore = useSettingStore()

const saving = computed(() => settingStore.saving)
const userActiveTab = ref('users')
const userFormRef = ref<FormInstance>()
const roleFormRef = ref<FormInstance>()
const permissionTreeRef = ref<PermissionTreeRef>()

const resettingPasswordUserId = ref<number | null>(null)
const deletingUserId = ref<number | null>(null)
const deletingRoleId = ref<number | null>(null)
const rolePermissionSaving = ref(false)

const searchForm = ref({
  username: '',
  status: '',
  roleId: '' as number | '',
})

const userStats = computed(() => {
  const list = settingStore.users
  const active = list.filter((item) => item.status === '启用').length
  return {
    total: list.length,
    active,
    disabled: list.length - active,
    roles: settingStore.roles.length,
  }
})

const roleUserCount = computed<Record<number, number>>(() => {
  const map: Record<number, number> = {}
  settingStore.users.forEach((item) => {
    map[item.roleId] = (map[item.roleId] || 0) + 1
  })
  return map
})

const avatarColor = (name: string): string => {
  const palette = ['#165DFF', '#00B42A', '#FF7D00', '#722ED1', '#14C9C9', '#F53F3F', '#FF9A2E']
  let hash = 0
  for (let i = 0; i < name.length; i += 1) hash = (hash * 31 + name.charCodeAt(i)) | 0
  return palette[Math.abs(hash) % palette.length]
}

const avatarText = (row: SettingUser): string => {
  const source = row.realName || row.username || '?'
  return source.trim().charAt(0).toUpperCase()
}

const selectedRoleId = ref<number | null>(1)
const roleDialogVisible = ref(false)
const roleForm = reactive<Partial<SettingRole>>({
  id: undefined,
  name: '',
  remark: '',
})
const copyFromRoleId = ref<number | null>(null)

const roleNameMap = computed(() => settingStore.roleMap)
const selectedRolePermissions = computed(() => settingStore.rolePermissions[selectedRoleId.value] || [])

const permissionLabelKeyMap: Record<string, string> = {
  dashboard: 'user.permissionDashboard',
  'dashboard:view': 'user.permissionDashboardView',
  'dashboard:overview': 'user.permissionOverviewView',
  domain: 'user.permissionDomains',
  'domain:zone:list': 'user.permissionZoneListView',
  'domain:record:edit': 'user.permissionDnsRecordEdit',
  'domain:record:delete': 'user.permissionDnsRecordDelete',
  forward: 'user.permissionForward',
  'forward:rules:view': 'user.permissionForwardRulesView',
  'forward:rules:edit': 'user.permissionForwardRulesEdit',
  'forward:load-balance:view': 'user.permissionForwardLoadBalanceView',
  'forward:load-balance:edit': 'user.permissionForwardLoadBalanceEdit',
  'forward:cache:view': 'user.permissionForwardCacheView',
  'forward:cache:edit': 'user.permissionForwardCacheEdit',
  security: 'user.permissionSecurity',
  'security:domain-access:view': 'user.permissionSecurityDomainAccessView',
  'security:domain-access:edit': 'user.permissionSecurityDomainAccessEdit',
  'security:ddos:view': 'user.permissionSecurityDdosView',
  'security:ddos:edit': 'user.permissionSecurityDdosEdit',
  'security:dnssec:view': 'user.permissionSecurityDnssecView',
  'security:dnssec:edit': 'user.permissionSecurityDnssecEdit',
  monitor: 'user.permissionMonitoring',
  'monitor:realtime:view': 'user.permissionRealtimeView',
  'monitor:rule:edit': 'user.permissionAlertRuleEdit',
  'monitor:report:export': 'user.permissionReportExport',
  tools: 'user.permissionTools',
  'tools:dig:view': 'user.permissionToolsDigView',
  'tools:dig:edit': 'user.permissionToolsDigEdit',
  'tools:global-test:view': 'user.permissionToolsGlobalTestView',
  'tools:global-test:edit': 'user.permissionToolsGlobalTestEdit',
  'tools:ip-location:view': 'user.permissionToolsIpLocationView',
  'tools:ip-location:edit': 'user.permissionToolsIpLocationEdit',
  cluster: 'user.permissionCluster',
  'cluster:overview:view': 'user.permissionClusterOverviewView',
  'cluster:nodes:view': 'user.permissionClusterNodesView',
  'cluster:nodes:edit': 'user.permissionClusterNodesEdit',
  'cluster:config-sync:view': 'user.permissionClusterConfigSyncView',
  'cluster:config-sync:edit': 'user.permissionClusterConfigSyncEdit',
  setting: 'user.permissionSettings',
  'setting:general:view': 'user.permissionGeneralView',
  'setting:general:edit': 'user.permissionGeneralEdit',
  'setting:user:view': 'user.permissionUserView',
  'setting:user:edit': 'user.permissionUserEdit',
  'setting:role:edit': 'user.permissionRolePermissionEdit',
  'setting:api-key:view': 'user.permissionApiKeyView',
  'setting:api-key:edit': 'user.permissionApiKeyEdit',
  'setting:backup:view': 'user.permissionBackupView',
  'setting:backup:create': 'user.permissionBackupCreate',
  'setting:backup:restore': 'user.permissionBackupRestore',
  'setting:backup:delete': 'user.permissionBackupDelete',
  'setting:notice:view': 'user.permissionNoticeView',
  'setting:notice:edit': 'user.permissionNoticeEdit',
  'setting:log:view': 'user.permissionAuditLogView',
  'setting:log:export': 'user.permissionAuditLogExport',
  'setting:log:purge': 'user.permissionAuditLogPurge',
}

const localizePermissionTree = (nodes: SettingRolePermissionNode[]): SettingRolePermissionNode[] =>
  nodes.map((node) => {
    const key = permissionLabelKeyMap[node.id]
    return {
      ...node,
      label: key ? t(key) : node.label,
      children: node.children ? localizePermissionTree(node.children) : undefined,
    }
  })

type SettingRolePermissionNode = {
  id: string
  label: string
  children?: SettingRolePermissionNode[]
}

const localizedPermissionTree = computed<SettingRolePermissionNode[]>(() =>
  localizePermissionTree(settingStore.permissionTree as SettingRolePermissionNode[]),
)

const createUserFormData = (): UserFormModel => ({
  id: undefined,
  username: '',
  password: '',
  realName: '',
  roleId: settingStore.roles[0]?.id,
  roleName: '',
  status: '启用',
})

const crud = useCrud<SettingUser, typeof searchForm.value, UserFormModel, UserFormModel, number>(
  {
    getList: (params) => getUserList(params),
    create: (payload) => addUser(payload),
    update: (payload) => editUser(payload),
    delete: (id) => deleteUser(id),
  },
  searchForm,
  {
    createFormData: createUserFormData,
    getItemId: (item) => item.id as number | undefined,
    pageField: 'page',
    pageSizeField: 'size',
  },
)

const {
  loading: userLoading,
  tableData: userTableData,
  pagination: userPagination,
  dialogVisible: userDialogVisible,
  formData: userForm,
  fetchData,
  handleCreate,
  handleEdit,
  handleDelete,
  handleSubmit,
} = crud

const userRules: FormRules = {
  username: [
    { required: true, message: t('user.enterUsername'), trigger: 'blur' },
    { pattern: /^[a-zA-Z0-9_]{4,20}$/, message: t('user.usernameRule'), trigger: ['blur', 'change'] },
  ],
  password: [
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (userForm.value.id && !String(value || '').trim()) {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error(t('user.enterPassword')))
          return
        }
        if (!/^(?=.*[A-Za-z])(?=.*\d)[A-Za-z\d@$!%*#?&]{8,32}$/.test(value)) {
          callback(new Error(t('user.passwordRule')))
          return
        }
        callback()
      },
      trigger: ['blur', 'change'],
    },
  ],
  realName: [
    { required: true, message: t('user.enterRealName'), trigger: 'blur' },
    { min: 2, max: 20, message: t('user.realNameRule'), trigger: ['blur', 'change'] },
  ],
  roleId: [{ required: true, message: t('user.selectRole'), trigger: 'change' }],
}

const roleRules: FormRules = {
  name: [
    { required: true, message: t('user.enterRoleName'), trigger: 'blur' },
    { pattern: /^[\u4e00-\u9fa5A-Za-z0-9_-]{2,20}$/, message: t('user.roleNameRule'), trigger: ['blur', 'change'] },
  ],
  remark: [{ min: 2, max: 100, message: t('user.roleRemarkRule'), trigger: ['blur', 'change'] }],
}

const statusLabel = (status: string) => (status === '启用' ? t('common.enabled') : t('common.disabled'))

const statusTagType = (status: string) => (status === '启用' ? 'success' : 'info')

const formatDateTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (num: number) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const openCreateUserDialog = (): void => {
  handleCreate()
  userForm.value = createUserFormData()
  userFormRef.value?.clearValidate()
}

const openEditUserDialog = (row: SettingUser): void => {
  handleEdit(row)
  userForm.value = {
    ...userForm.value,
    password: '',
  }
  userFormRef.value?.clearValidate()
}

const submitUser = async (): Promise<void> => {
  const valid = await validateFormAndFocus(userFormRef.value)
  if (!valid) {
    return
  }

  const isEdit = Boolean(userForm.value.id)
  userForm.value = {
    ...userForm.value,
    roleName: roleNameMap.value[Number(userForm.value.roleId)] || '',
  }

  try {
    await handleSubmit()
    ElMessage.success(isEdit ? t('user.userUpdated') : t('user.userCreated'))
  } catch (_error) {
    ElMessage.error(t('user.userSaveFailed'))
  }
}

const confirmDeleteUser = async (row: SettingUser): Promise<void> => {
  if (deletingUserId.value) {
    return
  }
  deletingUserId.value = row.id
  try {
    await confirmRiskAction({
      title: t('common.confirm'),
      action: t('user.deleteUserAction'),
      target: row.username,
      risk: t('user.deleteUserRisk'),
    })
    await handleDelete(row.id)
    ElMessage.success(t('user.userDeleted'))
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('user.userDeleteFailed'))
    }
  } finally {
    deletingUserId.value = null
  }
}

// ── Account lock status ──────────────────────────────────────────────────────
// We hydrate this map once per page load by calling /lock-status for every
// user in the current page. It's cheap (one Redis lookup each) and gives
// the table a "locked" badge so admins can spot wedged accounts without
// waiting for a help-desk ping. Refreshed on every list reload.
const lockStatusMap = ref<Record<number, UserLockStatus>>({})
const unlockingId   = ref<number | null>(null)
// (kickingId removed in 2026-05 — the per-row kick button on the
// user-management tab was relocated to the new 在线用户 tab; spinner
// state for that tab lives in revokingSid / kickingAllId below.)

const loadLockStatuses = async (rows: SettingUser[]): Promise<void> => {
  // Best-effort + parallel; a missing Redis instance leaves the map empty
  // which gracefully degrades to "no badge" for every row.
  const results = await Promise.all(
    rows.map(r =>
      getUserLockStatusApi(r.id)
        .then(({ data }) => [r.id, data] as const)
        .catch(() => null),
    ),
  )
  const next: Record<number, UserLockStatus> = {}
  results.forEach(entry => { if (entry) next[entry[0]] = entry[1] })
  lockStatusMap.value = next
}

const isLocked = (id: number): boolean => lockStatusMap.value[id]?.locked === true

// ── Online-sessions tab ─────────────────────────────────────────────────────
// Lists every live (user, sid) pair in the Redis session table. The
// data is small (one row per active operator session), so a single
// refresh-on-demand pull is enough — no polling, no SSE. The operator
// hits 「刷新」 if they want fresh state after kicking someone.
const onlineSessions = ref<OnlineSession[]>([])
const onlineLoading  = ref(false)
const revokingSid    = ref<string | null>(null)
const kickingAllId   = ref<number | null>(null)

const loadOnlineSessions = async (): Promise<void> => {
  onlineLoading.value = true
  try {
    const { data } = await listOnlineSessionsApi()
    onlineSessions.value = (data ?? []).slice().sort((a, b) => b.issuedAt - a.issuedAt)
  } catch (e) {
    ElMessage.error((e as Error)?.message ?? t('common.requestFailed'))
  } finally {
    onlineLoading.value = false
  }
}

// Aggregate sessions by userId so the "下线该用户全部会话" button can
// show the per-user count next to the row's first appearance.
const sessionCountByUser = computed<Record<number, number>>(() => {
  const map: Record<number, number> = {}
  for (const s of onlineSessions.value) {
    map[s.userId] = (map[s.userId] ?? 0) + 1
  }
  return map
})

// Reason picker for revoke / kick actions. The dialog is shared
// between single-session revoke and full-user kick so the four-item
// enum stays consistent and the audit log gets the same vocabulary
// regardless of which button the operator clicked. The "其他" option
// drops to a free-form input so an operator can paste a ticket
// reference without us adding a fifth radio.
type KickMode = 'one' | 'all'
interface KickDialogState {
  visible: boolean
  mode: KickMode
  row: OnlineSession | null
  reason: string
  customReason: string
  submitting: boolean
}
const kickDialog = reactive<KickDialogState>({
  visible: false,
  mode: 'one',
  row: null,
  reason: '凭据泄露',
  customReason: '',
  submitting: false,
})

// Frozen so reactivity doesn't deep-watch a constant.
const kickReasonPresets = Object.freeze([
  { key: 'leak',      labelKey: 'user.kickReason.leak' },
  { key: 'leaving',   labelKey: 'user.kickReason.leaving' },
  { key: 'anomaly',   labelKey: 'user.kickReason.anomaly' },
  { key: 'other',     labelKey: 'user.kickReason.other' },
])

const openKickDialog = (mode: KickMode, row: OnlineSession): void => {
  if (row.isCurrent && mode === 'one') return
  kickDialog.mode = mode
  kickDialog.row = row
  kickDialog.reason = t('user.kickReason.leak')
  kickDialog.customReason = ''
  kickDialog.visible = true
}

const resolvedKickReason = (): string => {
  const otherLabel = t('user.kickReason.other')
  if (kickDialog.reason === otherLabel) {
    return kickDialog.customReason.trim()
  }
  return kickDialog.reason
}

const submitKickDialog = async (): Promise<void> => {
  const row = kickDialog.row
  if (!row || kickDialog.submitting) return
  const reason = resolvedKickReason()
  // "其他" with empty input — guide the operator instead of letting
  // them submit a useless audit entry.
  if (kickDialog.reason === t('user.kickReason.other') && !reason) {
    ElMessage.warning(t('user.kickReason.otherRequired'))
    return
  }
  kickDialog.submitting = true
  try {
    if (kickDialog.mode === 'one') {
      revokingSid.value = row.sid
      await revokeOnlineSessionApi(row.userId, row.sid, reason)
      ElMessage.success(t('user.revokeSessionSuccess', { name: row.username }))
      onlineSessions.value = onlineSessions.value.filter(s => s.sid !== row.sid)
    } else {
      kickingAllId.value = row.userId
      const { data } = await kickUserSessionsApi(row.userId, reason)
      ElMessage.success(t('user.kickSessionsSuccess', {
        name: row.username,
        count: data.kicked ?? 0,
      }))
      onlineSessions.value = onlineSessions.value.filter(s => s.userId !== row.userId)
    }
    kickDialog.visible = false
  } catch (e) {
    ElMessage.error((e as Error)?.message ?? t('common.requestFailed'))
  } finally {
    kickDialog.submitting = false
    revokingSid.value = null
    kickingAllId.value = null
  }
}

// Thin wrappers preserved as the template's @click handlers so
// renaming the dialog logic later doesn't ripple through markup.
const confirmRevokeOne = (row: OnlineSession): void => openKickDialog('one', row)
const confirmKickAllOf = (row: OnlineSession): void => openKickDialog('all', row)

// Lazy-load: only hit the backend the first time the operator opens
// the tab. Subsequent visits show the last fetched data instantly;
// they can hit refresh for fresh state.
watch(userActiveTab, (tab) => {
  if (tab === 'online' && onlineSessions.value.length === 0 && !onlineLoading.value) {
    void loadOnlineSessions()
  }
})

// Per-user kick on the user-management table was removed in 2026-05.
// Same action is now reachable via "在线用户" tab → row's "下线全部"
// button (confirmKickAllOf above). Keeping the audit verb identical
// across both call sites means historic searches like
// "强制下线" still match.

const confirmUnlockUser = async (row: SettingUser): Promise<void> => {
  if (unlockingId.value) return
  try {
    await confirmRiskAction({
      title: t('cluster.unlockTitle'),
      action: t('cluster.unlockBtn'),
      target: row.username,
      risk: t('cluster.unlockConfirm', { name: row.username }),
    })
    unlockingId.value = row.id
    await unlockUserApi(row.id)
    ElMessage.success(t('cluster.unlockSuccess', { name: row.username }))
    // Refresh just this row's lock status so the badge clears immediately.
    const fresh = await getUserLockStatusApi(row.id).then(r => r.data).catch(() => null)
    if (fresh) lockStatusMap.value = { ...lockStatusMap.value, [row.id]: fresh }
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error)?.message ?? t('common.requestFailed'))
  } finally {
    unlockingId.value = null
  }
}

// Auto-refresh lock badges every time the table re-renders (paging,
// search, post-mutation reload). We don't track a fetch-id to dedupe
// because the call is idempotent and cheap.
watch(userTableData, (rows) => { if (rows?.length) void loadLockStatuses(rows) }, { immediate: false })

// Temp password disclosure dialog state. After the backend auto-generates
// a new password the operator needs to copy it once and hand it off out-of-
// band — we surface it here in a modal that stays open until they
// dismiss it (no auto-toast — that would race with copy attempts).
const tempPasswordVisible = ref(false)
const tempPasswordUser = ref('')
const tempPasswordValue = ref('')

const copyTempPassword = async (): Promise<void> => {
  try {
    await navigator.clipboard.writeText(tempPasswordValue.value)
    ElMessage.success(t('user.passwordCopied'))
  } catch (_e) {
    // Fallback: most modern browsers only deny clipboard when the page
    // isn't focused; the dialog itself still shows the value so manual
    // copy is always possible.
    ElMessage.warning(t('user.passwordCopyFailed'))
  }
}

const confirmResetPassword = async (row: SettingUser): Promise<void> => {
  if (resettingPasswordUserId.value) {
    return
  }
  resettingPasswordUserId.value = row.id
  try {
    await confirmRiskAction({
      title: t('user.resetPasswordConfirmTitle'),
      action: t('user.resetPasswordAction'),
      target: row.username,
      risk: t('user.resetPasswordRisk'),
    })
    const password = await settingStore.resetUserPassword({ id: row.id, username: row.username })
    if (password) {
      tempPasswordUser.value = row.username
      tempPasswordValue.value = password
      tempPasswordVisible.value = true
    } else {
      ElMessage.success(t('user.passwordResetDone'))
    }
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('user.passwordResetFailed'))
    }
  } finally {
    resettingPasswordUserId.value = null
  }
}

const toggleUserStatus = async (row: SettingUser): Promise<void> => {
  try {
    await editUser({
      ...row,
      status: row.status === '启用' ? '禁用' : '启用',
    })
    await fetchData()
    ElMessage.success(t('user.userStatusUpdated'))
  } catch (_error) {
    ElMessage.error(t('user.userStatusUpdateFailed'))
  }
}

const openRoleDialog = (row?: SettingRole): void => {
  Object.assign(roleForm, {
    id: row?.id || null,
    name: row?.name || '',
    remark: row?.remark || '',
  })
  roleDialogVisible.value = true
  roleFormRef.value?.clearValidate()
}

const submitRole = async (): Promise<void> => {
  if (settingStore.saving) {
    return
  }
  const valid = await validateFormAndFocus(roleFormRef.value)
  if (!valid) {
    return
  }
  await settingStore.saveRole({ ...roleForm })
  roleDialogVisible.value = false
  ElMessage.success(t('user.roleSaved'))
}

const confirmDeleteRole = async (row: SettingRole): Promise<void> => {
  if (deletingRoleId.value) {
    return
  }
  deletingRoleId.value = row.id
  try {
    await confirmRiskAction({ title: t('common.confirm'), action: t('user.deleteRoleAction'), target: row.name, risk: t('user.deleteRoleRisk') })
    await settingStore.deleteRole(row.id)
    if (selectedRoleId.value === row.id) {
      selectedRoleId.value = settingStore.roles[0]?.id || null
    }
    ElMessage.success(t('user.roleDeleted'))
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('user.roleDeleteFailed'))
    }
  } finally {
    deletingRoleId.value = null
  }
}

const onRoleChange = (roleId: number): void => {
  selectedRoleId.value = roleId
  permissionTreeRef.value?.setCheckedKeys(selectedRolePermissions.value)
}

const selectAllPermission = (): void => {
  const allKeys = settingStore.permissionTree.flatMap((parent) =>
    (parent.children || []).map((child) => child.id),
  )
  permissionTreeRef.value?.setCheckedKeys(allKeys)
}

const inversePermission = (): void => {
  const allKeys = settingStore.permissionTree.flatMap((parent) =>
    (parent.children || []).map((child) => child.id),
  )
  const current = permissionTreeRef.value?.getCheckedKeys() || []
  permissionTreeRef.value?.setCheckedKeys(allKeys.filter((key) => !current.includes(key)))
}

const copyPermissionsFromRole = (): void => {
  if (!copyFromRoleId.value) {
    ElMessage.warning(t('user.selectSourceRole'))
    return
  }
  permissionTreeRef.value?.setCheckedKeys(settingStore.rolePermissions[copyFromRoleId.value] || [])
  ElMessage.success(t('user.permissionCopied'))
}

const saveRolePermission = async (): Promise<void> => {
  if (settingStore.saving || rolePermissionSaving.value) {
    return
  }
  if (!selectedRoleId.value) {
    ElMessage.warning(t('user.selectRoleFirst'))
    return
  }
  rolePermissionSaving.value = true
  try {
    const permissions = permissionTreeRef.value?.getCheckedKeys() || []
    await settingStore.saveRolePermissions({ roleId: selectedRoleId.value, permissions })
    ElMessage.success(t('user.rolePermissionSaved'))
  } finally {
    rolePermissionSaving.value = false
  }
}

onMounted(async () => {
  try {
    await settingStore.fetchSettingData()
    if (!selectedRoleId.value && settingStore.roles.length) {
      selectedRoleId.value = settingStore.roles[0].id
    }
    permissionTreeRef.value?.setCheckedKeys(selectedRolePermissions.value)
  } catch (_error) {
    ElMessage.error(t('user.settingDataLoadFailed'))
  }
  // Eager-load online sessions so the stat card up top reflects
  // reality on first render. Cheap (one Redis SCAN + one batch SQL
  // join) and the data feeds both the card and the 在线用户 tab,
  // so doing it once at mount avoids a flash of "0" when the
  // operator hasn't opened that tab yet.
  void loadOnlineSessions()
})
</script>

<template>
  <div class="page-shell user-manage-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('user.subtitle') }}</p>
      </div>
    </div>

    <!-- Stats strip -->
    <div class="um-stats">
      <div class="um-stat">
        <div class="um-stat-icon um-stat-icon-total"><el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M12 12a5 5 0 1 0-5-5 5 5 0 0 0 5 5zm0 2c-3.33 0-10 1.67-10 5v3h20v-3c0-3.33-6.67-5-10-5z" /></svg></el-icon></div>
        <div class="um-stat-body">
          <div class="um-stat-label">{{ $t('user.totalUsers') }}</div>
          <div class="um-stat-value">{{ userStats.total }}</div>
        </div>
      </div>
      <div class="um-stat">
        <div class="um-stat-icon um-stat-icon-active"><el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" /></svg></el-icon></div>
        <div class="um-stat-body">
          <div class="um-stat-label">{{ $t('user.activeUsers') }}</div>
          <div class="um-stat-value">{{ userStats.active }}</div>
        </div>
      </div>
      <!-- Stat card was "已禁用 (disabled users)" until 2026-05; an
           operator with disabled accounts to triage already filters
           by status in the table, so the dashboard slot was wasted on
           a near-always-zero number. Online-user count is more
           actionable (it tells the operator at a glance how many
           people are actively using the system right now) and ties
           directly to the new 在线用户 tab below. -->
      <div class="um-stat">
        <div class="um-stat-icon um-stat-icon-online"><el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 4a3 3 0 1 1-3 3 3 3 0 0 1 3-3zm0 14a8 8 0 0 1-6.3-3.06A6.55 6.55 0 0 1 12 14a6.55 6.55 0 0 1 6.3 2.94A8 8 0 0 1 12 20z" /></svg></el-icon></div>
        <div class="um-stat-body">
          <div class="um-stat-label">{{ $t('user.onlineUsers') }}</div>
          <div class="um-stat-value">{{ Object.keys(sessionCountByUser).length }}</div>
        </div>
      </div>
      <div class="um-stat">
        <div class="um-stat-icon um-stat-icon-role"><el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5z" /></svg></el-icon></div>
        <div class="um-stat-body">
          <div class="um-stat-label">{{ $t('user.roleCount') }}</div>
          <div class="um-stat-value">{{ userStats.roles }}</div>
        </div>
      </div>
    </div>

    <el-card class="content-card um-main-card">
      <el-tabs v-model="userActiveTab" class="um-tabs">

        <!-- ═══════════════ 用户列表 ═══════════════ -->
        <el-tab-pane :label="$t('user.userListTab')" name="users">
          <div class="um-toolbar">
            <div class="um-filter-row">
              <el-input
                v-model="searchForm.username"
                :placeholder="$t('user.searchUserPlaceholder')"
                clearable
                class="um-search"
                @keyup.enter="fetchData({ currentPage: 1 })"
              >
                <template #prefix>
                  <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 5 1.5-1.5-5-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z" /></svg></el-icon>
                </template>
              </el-input>
              <el-select v-model="searchForm.status" :placeholder="$t('user.allStatus')" clearable class="um-filter" @change="fetchData({ currentPage: 1 })">
                <el-option :label="$t('user.allStatus')" value="" />
                <el-option :label="$t('common.enabled')" value="启用" />
                <el-option :label="$t('common.disabled')" value="禁用" />
              </el-select>
              <el-select v-model="searchForm.roleId" :placeholder="$t('user.allRoles')" clearable class="um-filter" @change="fetchData({ currentPage: 1 })">
                <el-option :label="$t('user.allRoles')" :value="''" />
                <el-option v-for="r in settingStore.roles" :key="r.id" :label="r.name" :value="r.id" />
              </el-select>
              <el-button :disabled="userLoading" @click="fetchData({ currentPage: 1 })">
                <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M17.65 6.35A8 8 0 1 0 19.73 14h-2.08A6 6 0 1 1 12 6a5.9 5.9 0 0 1 4.22 1.78L13 11h7V4z" /></svg></el-icon>
                {{ $t('common.refresh') }}
              </el-button>
            </div>
            <el-button type="primary" @click="openCreateUserDialog">
              <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" /></svg></el-icon>
              {{ $t('user.addUser') }}
            </el-button>
          </div>

          <el-table :data="userTableData" v-loading="userLoading" class="um-table" row-key="id">
            <el-table-column :label="$t('user.username')" min-width="220">
              <template #default="{ row }">
                <div class="um-user-cell">
                  <span class="um-avatar" :style="{ background: avatarColor(row.username || '') }">{{ avatarText(row) }}</span>
                  <div class="um-user-info">
                    <div class="um-user-name">
                      {{ row.username }}<span class="um-user-id">#{{ row.id }}</span>
                      <el-tooltip
                        v-if="isLocked(row.id)"
                        :content="$t('cluster.accountLockedTip', { sec: lockStatusMap[row.id]?.lockTTLSec ?? 0 })"
                        placement="top"
                      >
                        <span class="um-locked-badge">{{ $t('cluster.accountLocked') }}</span>
                      </el-tooltip>
                    </div>
                    <div class="um-user-realname">{{ row.realName }}</div>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('user.role')" width="160">
              <template #default="{ row }">
                <span class="um-role-chip">{{ row.roleName }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('user.status')" width="110" align="center">
              <template #default="{ row }">
                <span class="um-status" :class="row.status === '启用' ? 'is-active' : 'is-inactive'">
                  <i class="um-status-dot" />{{ statusLabel(row.status) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('user.createdAt')" width="180">
              <template #default="{ row }">
                <span class="um-datetime">{{ formatDateTime(row.createdAt) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="280" fixed="right">
              <template #default="{ row }">
                <div class="um-actions">
                  <el-button link type="primary" @click="openEditUserDialog(row)">{{ $t('common.edit') }}</el-button>
                  <el-divider direction="vertical" />
                  <el-button link type="warning" :loading="resettingPasswordUserId === row.id" :disabled="Boolean(resettingPasswordUserId) || Boolean(deletingUserId) || saving" @click="confirmResetPassword(row)">{{ $t('user.resetPassword') }}</el-button>
                  <el-divider direction="vertical" />
                  <el-button
                    v-if="isLocked(row.id)"
                    link
                    type="danger"
                    :loading="unlockingId === row.id"
                    :disabled="Boolean(unlockingId) || saving"
                    @click="confirmUnlockUser(row)"
                  >{{ $t('cluster.unlockBtn') }}</el-button>
                  <!-- "强制下线" was removed from this row in 2026-05;
                       force-logout is a session-management action, not
                       a user-record action. The new "在线用户" tab is
                       where it lives — listing actual live sessions
                       gives operators the right context (issued-at
                       time, single-session vs all-sessions) instead of
                       a context-free button on every row. -->
                  <el-divider v-if="isLocked(row.id)" direction="vertical" />
                  <el-button link :type="row.status === '启用' ? 'info' : 'success'" @click="toggleUserStatus(row)">{{ row.status === '启用' ? $t('common.disable') : $t('common.enable') }}</el-button>
                  <el-divider direction="vertical" />
                  <el-button link type="danger" :loading="deletingUserId === row.id" :disabled="Boolean(deletingUserId) || saving" @click="confirmDeleteUser(row)">{{ $t('common.delete') }}</el-button>
                </div>
              </template>
            </el-table-column>
            <template #empty>
              <div class="um-empty">
                <el-empty :image-size="80" :description="$t('user.noUserData')" />
              </div>
            </template>
          </el-table>

          <div class="um-pagination">
            <el-pagination
              :current-page="userPagination.currentPage"
              :page-size="userPagination.pageSize"
              layout="total, sizes, prev, pager, next, jumper"
              background
              :page-sizes="[10, 20, 50, 100]"
              :total="userPagination.total"
              @current-change="(page) => fetchData({ currentPage: page })"
              @size-change="(pageSize) => fetchData({ currentPage: 1, pageSize })"
            />
          </div>
        </el-tab-pane>

        <!-- ═══════════════ 在线用户 ═══════════════ -->
        <!-- Lists every live access-token sid the Redis session table
             knows about. One row per (user, sid) pair so an operator
             with three concurrent browser sessions shows up three
             times — letting admins kick a single rogue tab without
             nuking the user's other (legitimate) sessions. The
             user-level "下线全部" button beside each row collapses
             that down to the existing /users/:id/kick-sessions
             endpoint when a wholesale logout is needed. -->
        <el-tab-pane :label="$t('user.onlineSessionsTab')" name="online">
          <div class="um-toolbar">
            <div class="um-filter-row">
              <div class="um-online-summary">
                <span>{{ $t('user.onlineSummary', {
                  sessions: onlineSessions.length,
                  users: Object.keys(sessionCountByUser).length,
                }) }}</span>
              </div>
              <div class="um-filter-actions">
                <el-button :icon="undefined" :loading="onlineLoading" @click="loadOnlineSessions">
                  {{ $t('common.refresh') }}
                </el-button>
              </div>
            </div>
          </div>

          <el-table
            :data="onlineSessions"
            v-loading="onlineLoading"
            class="um-table"
            stripe
            row-key="sid"
          >
            <el-table-column prop="username" :label="$t('user.username')" min-width="180">
              <template #default="{ row }">
                <div class="um-user-cell">
                  <div class="um-user-name">
                    {{ row.username }}<span class="um-user-id">#{{ row.userId }}</span>
                    <el-tag v-if="row.isCurrent" size="small" type="success" class="um-online-self">
                      {{ $t('user.currentSessionTag') }}
                    </el-tag>
                  </div>
                  <div v-if="row.realName" class="um-user-real">{{ row.realName }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="roleName" :label="$t('user.role')" width="160" />
            <el-table-column :label="$t('user.sessionId')" width="160">
              <template #default="{ row }">
                <code class="um-online-sid">{{ row.sid.slice(0, 12) }}…</code>
              </template>
            </el-table-column>
            <el-table-column :label="$t('user.issuedAt')" width="200">
              <template #default="{ row }">
                {{ row.issuedAt ? new Date(row.issuedAt).toLocaleString() : '—' }}
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="280" fixed="right">
              <template #default="{ row }">
                <div class="um-actions">
                  <el-button
                    link
                    type="danger"
                    :disabled="row.isCurrent || revokingSid === row.sid"
                    :loading="revokingSid === row.sid"
                    @click="confirmRevokeOne(row)"
                  >{{ $t('user.revokeSessionBtn') }}</el-button>
                  <el-divider direction="vertical" />
                  <el-button
                    link
                    type="danger"
                    :disabled="kickingAllId === row.userId"
                    :loading="kickingAllId === row.userId"
                    @click="confirmKickAllOf(row)"
                  >{{ $t('user.kickAllBtn', { count: sessionCountByUser[row.userId] ?? 0 }) }}</el-button>
                </div>
              </template>
            </el-table-column>
            <template #empty>
              <div class="um-empty">
                <el-empty :image-size="80" :description="$t('user.noOnlineUser')" />
              </div>
            </template>
          </el-table>
        </el-tab-pane>

        <!-- ═══════════════ 角色权限 ═══════════════ -->
        <el-tab-pane :label="$t('user.rolePermissionTab')" name="roles">
          <div class="um-rbac">
            <!-- Left: Role list -->
            <div class="um-rbac-col">
              <div class="um-panel">
                <div class="um-panel-header">
                  <div class="um-panel-title">
                    {{ $t('user.roleList') }}
                    <span class="um-count-badge">{{ settingStore.roles.length }}</span>
                  </div>
                  <el-button type="primary" size="small" @click="openRoleDialog()">
                    <el-icon><svg viewBox="0 0 24 24" fill="currentColor" width="12" height="12"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" /></svg></el-icon>
                    {{ $t('user.addRole') }}
                  </el-button>
                </div>
                <div class="um-panel-body">
                  <el-empty v-if="!settingStore.roles.length" :image-size="60" :description="$t('user.noRoleData')" />
                  <div v-else class="um-role-list">
                    <div
                      v-for="item in settingStore.roles"
                      :key="item.id"
                      class="um-role-item"
                      :class="{ 'is-active': selectedRoleId === item.id }"
                      @click="onRoleChange(item.id)"
                    >
                      <div class="um-role-main">
                        <div class="um-role-name">
                          {{ item.name }}
                          <span class="um-role-count">{{ roleUserCount[item.id] || 0 }} {{ $t('user.people') }}</span>
                        </div>
                        <div class="um-role-remark">{{ item.remark || $t('common.none') }}</div>
                      </div>
                      <div class="um-role-actions">
                        <el-button link type="primary" size="small" @click.stop="openRoleDialog(item)">{{ $t('common.edit') }}</el-button>
                        <el-button link type="danger" size="small" :loading="deletingRoleId === item.id" :disabled="Boolean(deletingRoleId) || saving" @click.stop="confirmDeleteRole(item)">{{ $t('common.delete') }}</el-button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Right: Permission tree -->
            <div class="um-rbac-col">
              <div class="um-panel">
                <div class="um-panel-header">
                  <div class="um-panel-title">
                    {{ $t('user.permissionConfig') }}
                    <span v-if="selectedRoleId" class="um-panel-subtitle">- {{ roleNameMap[selectedRoleId] }}</span>
                  </div>
                  <div class="um-panel-actions">
                    <el-button size="small" @click="selectAllPermission">{{ $t('common.selectAll') }}</el-button>
                    <el-button size="small" @click="inversePermission">{{ $t('user.inverseSelect') }}</el-button>
                    <el-select v-model="copyFromRoleId" :placeholder="$t('user.copyFromRole')" size="small" class="um-copy-select">
                      <el-option
                        v-for="item in settingStore.roles.filter((role) => role.id !== selectedRoleId)"
                        :key="item.id"
                        :label="item.name"
                        :value="item.id"
                      />
                    </el-select>
                    <el-button size="small" @click="copyPermissionsFromRole">{{ $t('user.copyNow') }}</el-button>
                    <el-button size="small" type="primary" :loading="rolePermissionSaving" :disabled="rolePermissionSaving" @click="saveRolePermission">{{ $t('user.savePermissionConfig') }}</el-button>
                  </div>
                </div>
                <div class="um-panel-body">
                  <el-empty
                    v-if="!settingStore.permissionTree.length"
                    :image-size="70"
                    :description="$t('user.noPermissionTree')"
                  />
                  <el-tree
                    v-else
                    ref="permissionTreeRef"
                    node-key="id"
                    :data="localizedPermissionTree"
                    show-checkbox
                    default-expand-all
                    :default-checked-keys="selectedRolePermissions"
                    :props="{ label: 'label', children: 'children' }"
                    class="um-permission-tree"
                  />
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="userDialogVisible" :title="userForm.id ? $t('user.editUser') : $t('user.addUser')" width="500px" destroy-on-close>
      <el-form
        ref="userFormRef"
        :model="userForm"
        :rules="userRules"
        label-position="top"
        require-asterisk-position="left"
        class="single-column-dialog-form"
      >
        <el-form-item :label="$t('user.username')" prop="username" required><el-input v-model="userForm.username" /></el-form-item>
        <!--
          Password is only collected on user creation. For edits (id present)
          we hide the field entirely so changing a role / display name doesn't
          force the operator to re-type the password — that workflow goes
          through the dedicated "重置密码" action instead. Hiding (instead of
          merely making it optional) also sidesteps Element Plus's auto-
          injected required rule whose default English message ("password is
          required") was leaking through under the zh-CN locale.
        -->
        <el-form-item v-if="!userForm.id" :label="$t('login.password')" prop="password" required><el-input v-model="userForm.password" show-password /></el-form-item>
        <el-form-item :label="$t('user.displayName')" prop="realName" required><el-input v-model="userForm.realName" /></el-form-item>
        <el-form-item :label="$t('user.role')" prop="roleId" required>
          <el-select v-model="userForm.roleId" class="full-width">
            <el-option v-for="item in settingStore.roles" :key="item.id" :label="item.name" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('user.status')" class="switch-right-item">
          <el-switch v-model="userForm.status" active-value="启用" inactive-value="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer-right">
          <el-button @click="userDialogVisible = false">{{ $t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="userLoading" @click="submitUser">{{ $t('common.save') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Kick / revoke confirmation dialog with mandatory reason picker.
         Shared between single-session revoke and full-user kick: the
         dialog title and risk copy switch on kickDialog.mode while the
         reason enum + audit-log path stay identical. -->
    <el-dialog
      v-model="kickDialog.visible"
      :title="kickDialog.mode === 'one' ? $t('user.revokeSessionTitle') : $t('user.kickSessionsTitle')"
      width="460px"
      destroy-on-close
      :close-on-click-modal="false"
    >
      <div v-if="kickDialog.row" class="kick-dialog-body">
        <div class="kick-dialog-target">
          <div class="kick-dialog-target-label">{{ $t('user.kickDialogTarget') }}</div>
          <div class="kick-dialog-target-value">
            {{ kickDialog.row.username }}
            <span v-if="kickDialog.mode === 'one'" class="kick-dialog-sid">· {{ kickDialog.row.sid.slice(0, 8) }}…</span>
            <span v-else class="kick-dialog-sid">· {{ $t('user.kickAllBtn', { count: sessionCountByUser[kickDialog.row.userId] ?? 0 }) }}</span>
          </div>
        </div>
        <el-form label-position="top">
          <el-form-item :label="$t('user.kickReasonLabel')" required>
            <el-radio-group v-model="kickDialog.reason" class="kick-dialog-reasons">
              <el-radio
                v-for="opt in kickReasonPresets"
                :key="opt.key"
                :value="$t(opt.labelKey)"
              >{{ $t(opt.labelKey) }}</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="kickDialog.reason === $t('user.kickReason.other')" :label="$t('user.kickReasonCustom')">
            <el-input
              v-model="kickDialog.customReason"
              maxlength="200"
              show-word-limit
              :placeholder="$t('user.kickReasonCustomPlaceholder')"
            />
          </el-form-item>
        </el-form>
        <el-alert
          type="warning"
          :closable="false"
          :title="kickDialog.mode === 'one'
            ? $t('user.revokeSessionConfirm', { name: kickDialog.row.username })
            : $t('user.kickSessionsConfirm', { name: kickDialog.row.username })"
          show-icon
        />
      </div>
      <template #footer>
        <div class="dialog-footer-right">
          <el-button @click="kickDialog.visible = false">{{ $t('common.cancel') }}</el-button>
          <el-button type="danger" :loading="kickDialog.submitting" @click="submitKickDialog">
            {{ kickDialog.mode === 'one' ? $t('user.revokeSessionBtn') : $t('user.kickSessionsBtn') }}
          </el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="roleDialogVisible" :title="roleForm.id ? $t('user.editRole') : $t('user.addRole')" width="500px" destroy-on-close>
      <el-form
        ref="roleFormRef"
        :model="roleForm"
        :rules="roleRules"
        label-position="top"
        require-asterisk-position="left"
        class="single-column-dialog-form"
      >
        <el-form-item :label="$t('user.role')" prop="name" required><el-input v-model="roleForm.name" /></el-form-item>
        <el-form-item :label="$t('common.remark')" prop="remark"><el-input v-model="roleForm.remark" type="textarea" :rows="3" class="role-remark-input" /></el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer-right">
          <el-button @click="roleDialogVisible = false">{{ $t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="submitRole">{{ $t('common.save') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!--
      Temp password disclosure modal. Shown once after a successful reset;
      the password isn't recoverable later so the dialog deliberately
      uses :close-on-click-modal=false / :show-close=false on the toast
      and forces the operator through the explicit "我已记录" button.
    -->
    <el-dialog
      v-model="tempPasswordVisible"
      :title="$t('user.tempPasswordTitle')"
      width="460px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      align-center
    >
      <div class="temp-pw-body">
        <p class="temp-pw-desc">{{ $t('user.tempPasswordDesc', { user: tempPasswordUser }) }}</p>
        <div class="temp-pw-box">
          <code class="temp-pw-value">{{ tempPasswordValue }}</code>
          <el-button type="primary" link @click="copyTempPassword">{{ $t('common.copy') }}</el-button>
        </div>
        <p class="temp-pw-hint">{{ $t('user.tempPasswordHint') }}</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="tempPasswordVisible = false">{{ $t('user.tempPasswordAcknowledge') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page shell ═══════════════ */
.user-manage-page {
  font-size: 13px;
}

.user-manage-page :deep(.el-card__body) {
  padding: 0;
}

.um-main-card {
  overflow: hidden;
}

/* ═══════════════ Stats strip ═══════════════ */
.um-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.um-stat {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow 0.2s, transform 0.2s;
}

.um-stat:hover {
  box-shadow: 0 6px 18px rgba(22, 93, 255, 0.12);
  transform: translateY(-1px);
}

.um-stat-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}

.um-stat-icon-total { background: var(--app-accent-soft); color: var(--app-accent); }
.um-stat-icon-active { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }
.um-stat-icon-disabled { background: rgba(134, 144, 156, 0.12); color: var(--app-disabled); }
.um-stat-icon-online { background: rgba(255, 145, 0, 0.12); color: #FF9100; }
.um-stat-icon-role { background: rgba(114, 46, 209, 0.1); color: #722ED1; }

.um-stat-body {
  flex: 1;
  min-width: 0;
}

.um-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-bottom: 4px;
}

.um-stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--app-title);
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

/* ═══════════════ Tabs ═══════════════ */
.um-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 20px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.um-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.um-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  height: 48px;
  line-height: 48px;
  padding: 0 20px !important;
}

.um-tabs :deep(.el-tabs__content) {
  padding: 20px;
}

/* Online-sessions tab — small visual additions only; the table reuses
   .um-table from the user-list tab. */
.um-online-summary {
  font-size: 13px;
  color: var(--app-text-secondary);
  font-variant-numeric: tabular-nums;
}
.um-online-sid {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  background: var(--app-bg-tertiary, #f4f5f7);
  padding: 2px 6px;
  border-radius: 4px;
}
.um-online-self {
  margin-left: 8px;
}

/* Kick / revoke dialog. The reasons radio group spans full width and
   stacks on narrow viewports so the four-item enum stays scannable. */
.kick-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.kick-dialog-target {
  background: var(--app-bg-tertiary, #f4f5f7);
  border-radius: 6px;
  padding: 10px 12px;
}
.kick-dialog-target-label {
  font-size: 12px;
  color: var(--app-text-secondary);
  margin-bottom: 4px;
}
.kick-dialog-target-value {
  font-size: 14px;
  font-weight: 500;
}
.kick-dialog-sid {
  margin-left: 6px;
  color: var(--app-text-secondary);
  font-weight: 400;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.kick-dialog-reasons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}

/* ═══════════════ Toolbar ═══════════════ */
.um-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.um-filter-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  flex-wrap: wrap;
}

.um-search {
  width: 280px;
  min-width: 200px;
}

.um-filter {
  width: 140px;
}

/* ═══════════════ User table ═══════════════ */
.um-table {
  --el-table-border-color: var(--app-border);
}

.um-table :deep(.el-table__header-wrapper th) {
  background: var(--app-table-header) !important;
  color: var(--app-text-regular);
  font-weight: 600;
  font-size: 13px;
}

.um-table :deep(.el-table__row) {
  transition: background 0.2s;
}

.um-table :deep(.el-table__row:hover > td) {
  background: var(--app-table-hover) !important;
}

.um-user-cell {
  display: flex;
  align-items: center;
  gap: 12px;
}

.um-avatar {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  font-weight: 600;
  font-size: 14px;
  flex-shrink: 0;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.08);
}

.um-user-info {
  min-width: 0;
  line-height: 1.45;
}

.um-user-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  display: flex;
  align-items: center;
  gap: 6px;
}

.um-user-id {
  font-size: 11px;
  font-weight: 400;
  color: var(--app-disabled);
  font-variant-numeric: tabular-nums;
}

.um-locked-badge {
  display: inline-block;
  margin-left: 6px;
  padding: 1px 6px;
  font-size: 11px;
  font-weight: 600;
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.1);
  border: 1px solid rgba(245, 63, 63, 0.3);
  border-radius: 3px;
  cursor: help;
}

.um-user-realname {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
}

.um-role-chip {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  background: var(--app-accent-soft);
  color: var(--app-accent);
  border: 1px solid rgba(22, 93, 255, 0.2);
}

.um-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
}

.um-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}

.um-status.is-active {
  color: var(--app-success);
}

.um-status.is-active .um-status-dot {
  background: var(--app-success);
  box-shadow: 0 0 0 3px rgba(0, 180, 42, 0.15);
}

.um-status.is-inactive {
  color: var(--app-disabled);
}

.um-status.is-inactive .um-status-dot {
  background: var(--app-disabled);
}

.um-datetime {
  font-size: 13px;
  color: var(--app-text-regular);
  font-variant-numeric: tabular-nums;
}

.um-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.um-actions :deep(.el-button.is-link) {
  padding: 0 4px;
  height: auto;
}

.um-actions :deep(.el-divider--vertical) {
  margin: 0 2px;
  border-color: var(--app-border);
}

.um-empty {
  padding: 40px 0;
}

.um-pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

/* ═══════════════ RBAC layout ═══════════════ */
.um-rbac {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
}

.um-rbac-col {
  min-width: 0;
}

/* ═══════════════ Panel (inner card) ═══════════════ */
.um-panel {
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 480px;
}

.um-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border);
  background: linear-gradient(180deg, #FAFBFC 0%, #FFFFFF 100%);
  flex-wrap: wrap;
}

.um-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  display: flex;
  align-items: center;
  gap: 8px;
}

.um-panel-subtitle {
  font-size: 12px;
  font-weight: 400;
  color: var(--app-text-regular);
}

.um-count-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 8px;
  border-radius: 11px;
  background: var(--app-accent-soft);
  color: var(--app-accent);
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.um-panel-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.um-panel-body {
  padding: 12px 16px;
  flex: 1;
  overflow: auto;
}

.um-copy-select {
  width: 160px;
}

/* ═══════════════ Role list items ═══════════════ */
.um-role-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.um-role-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  cursor: pointer;
  background: var(--app-bg-secondary);
  transition: all 0.2s;
}

.um-role-item:hover {
  border-color: var(--app-accent-muted);
  background: #FBFDFF;
}

.um-role-item.is-active {
  border-color: var(--app-accent);
  background: var(--app-accent-soft);
  box-shadow: 0 0 0 3px rgba(22, 93, 255, 0.08);
}

.um-role-main {
  min-width: 0;
  flex: 1;
}

.um-role-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  display: flex;
  align-items: center;
  gap: 8px;
}

.um-role-count {
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: 10px;
  background: rgba(22, 93, 255, 0.08);
  color: var(--app-accent);
}

.um-role-item.is-active .um-role-count {
  background: #fff;
}

.um-role-remark {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 3px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.um-role-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

/* ═══════════════ Permission tree ═══════════════ */
.um-permission-tree {
  background: transparent;
}

.um-permission-tree :deep(.el-tree-node__content) {
  height: 34px;
  border-radius: 6px;
  transition: background 0.2s;
}

.um-permission-tree :deep(.el-tree-node__content:hover) {
  background: var(--app-accent-soft);
}

.um-permission-tree :deep(.el-checkbox) {
  margin-right: 8px;
}

.um-permission-tree :deep(.el-tree-node__label) {
  font-size: 13px;
  color: var(--app-text-main);
}

.um-permission-tree :deep(.el-tree-node.is-current > .el-tree-node__content) {
  background: var(--app-accent-soft);
}

/* ═══════════════ Dialog ═══════════════ */
.single-column-dialog-form {
  padding: 20px 4px 4px;
}

.single-column-dialog-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.role-remark-input :deep(.el-textarea__inner) {
  min-height: 60px;
}

.switch-right-item :deep(.el-form-item__content) {
  justify-content: flex-end;
}

.dialog-footer-right {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .um-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .um-rbac {
    grid-template-columns: 1fr;
  }

  .um-search {
    width: 100%;
  }
}

@media (max-width: 768px) {
  .um-stats {
    grid-template-columns: 1fr;
  }

  .um-filter-row {
    width: 100%;
  }

  .um-filter {
    flex: 1;
    min-width: 120px;
  }

  .um-panel-actions {
    width: 100%;
  }

  .um-copy-select {
    flex: 1;
  }
}

/* ═══════════════ Temp password disclosure dialog ═══════════════ */
.temp-pw-body { display: flex; flex-direction: column; gap: 12px; }
.temp-pw-desc { font-size: 13px; color: var(--app-text-regular); margin: 0; line-height: 1.6; }
.temp-pw-box {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 12px;
  background: var(--app-accent-soft);
  border: 1px dashed var(--app-accent);
  border-radius: 8px;
}
.temp-pw-value {
  flex: 1;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--app-title);
  user-select: all;
  word-break: break-all;
}
.temp-pw-hint { font-size: 12px; color: var(--app-warning); margin: 0; }
</style>
