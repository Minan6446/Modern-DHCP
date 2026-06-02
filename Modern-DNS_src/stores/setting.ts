import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  createBackupApi,
  deleteBackupApi,
  deleteRoleApi,
  deleteUserApi,
  exportLogsApi,
  getLogsApi,
  getSettingModuleDataApi,
  resetCommonConfigApi,
  resetUserPasswordApi,
  restoreBackupApi,
  saveCommonConfigApi,
  saveNoticeConfigApi,
  saveRoleApi,
  saveRolePermissionsApi,
  saveUserApi,
  testNoticeChannelApi,
  uploadBackupFileApi,
} from '../api/setting'
import type {
  SettingBackup,
  SettingCommonConfig,
  SettingLogItem,
  SettingModuleData,
  SettingNoticeConfig,
  SettingPermissionNode,
  SettingRole,
  SettingUser,
} from '../types/modules'
import { createDefaultGeneralConfigForm } from '../types/setting'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

type LogAppendPayload = Partial<Omit<SettingLogItem, 'id' | 'logId'>>

type SaveRolePermissionPayload = {
  roleId: number
  permissions: string[]
}

const defaultPermissionTreeData = (): SettingPermissionNode[] => ([
  {
    id: 'dashboard',
    label: 'Dashboard',
    children: [
      { id: 'dashboard:view', label: 'View Dashboard' },
      { id: 'dashboard:overview', label: 'View Overview Metrics' },
    ],
  },
  {
    id: 'domain',
    label: 'Domains',
    children: [
      { id: 'domain:zone:list', label: 'View Zone List' },
      { id: 'domain:record:edit', label: 'Edit DNS Records' },
      { id: 'domain:record:delete', label: 'Delete DNS Records' },
    ],
  },
  {
    id: 'forward',
    label: 'Forward & Cache',
    children: [
      { id: 'forward:rules:view', label: 'View Forwarding Rules' },
      { id: 'forward:rules:edit', label: 'Edit Forwarding Rules' },
      { id: 'forward:load-balance:view', label: 'View Load Balancing' },
      { id: 'forward:load-balance:edit', label: 'Edit Load Balancing' },
      { id: 'forward:cache:view', label: 'View Cache Strategy' },
      { id: 'forward:cache:edit', label: 'Edit Cache Strategy' },
    ],
  },
  {
    id: 'security',
    label: 'Security',
    children: [
      { id: 'security:domain-access:view', label: 'View Domain Access Control' },
      { id: 'security:domain-access:edit', label: 'Edit Domain Access Control' },
      { id: 'security:ddos:view', label: 'View DDoS Protection' },
      { id: 'security:ddos:edit', label: 'Edit DDoS Protection' },
      { id: 'security:dnssec:view', label: 'View DNSSEC' },
      { id: 'security:dnssec:edit', label: 'Edit DNSSEC' },
    ],
  },
  {
    id: 'monitor',
    label: 'Monitoring',
    children: [
      { id: 'monitor:realtime:view', label: 'View Real-time Queries' },
      { id: 'monitor:rule:edit', label: 'Edit Alert Rules' },
      { id: 'monitor:report:export', label: 'Export Reports' },
    ],
  },
  {
    id: 'tools',
    label: 'Tools',
    children: [
      { id: 'tools:dig:view', label: 'View Dig Tool' },
      { id: 'tools:dig:edit', label: 'Operate Dig Tool' },
      { id: 'tools:global-test:view', label: 'View Global Test' },
      { id: 'tools:global-test:edit', label: 'Operate Global Test' },
      { id: 'tools:ip-location:view', label: 'View IP Location' },
      { id: 'tools:ip-location:edit', label: 'Operate IP Location' },
    ],
  },
  {
    id: 'cluster',
    label: 'Cluster',
    children: [
      { id: 'cluster:overview:view', label: 'View Cluster Overview' },
      { id: 'cluster:nodes:view', label: 'View Cluster Nodes' },
      { id: 'cluster:nodes:edit', label: 'Edit Cluster Nodes' },
      { id: 'cluster:config-sync:view', label: 'View Config Sync' },
      { id: 'cluster:config-sync:edit', label: 'Edit Config Sync' },
    ],
  },
  {
    // Settings (系统设置). The labels here are English fallbacks — the
    // role-permission tree on the user-management page runs each node
    // through `localizePermissionTree`, which swaps the label for a
    // translated string from `permissionLabelKeyMap` in
    // `views/setting/UserManagePage.vue`. When adding new nodes, both
    // the i18n label key and the map entry need updating in tandem.
    id: 'setting',
    label: 'Settings',
    children: [
      // 通用设置
      { id: 'setting:general:view', label: 'View General Settings' },
      { id: 'setting:general:edit', label: 'Edit General Settings' },
      // 用户管理 + 角色权限
      { id: 'setting:user:view', label: 'View Users' },
      { id: 'setting:user:edit', label: 'Edit Users' },
      { id: 'setting:role:edit', label: 'Edit Roles & Permissions' },
      // API 密钥管理
      { id: 'setting:api-key:view', label: 'View API Keys' },
      { id: 'setting:api-key:edit', label: 'Edit API Keys' },
      // 备份还原（含手动备份 / 还原 / 自动备份调度）
      { id: 'setting:backup:view', label: 'View Backups' },
      { id: 'setting:backup:create', label: 'Create Backup' },
      { id: 'setting:backup:restore', label: 'Restore Backup' },
      { id: 'setting:backup:delete', label: 'Delete Backup' },
      // 通知设置
      { id: 'setting:notice:view', label: 'View Notification Settings' },
      { id: 'setting:notice:edit', label: 'Edit Notification Settings' },
      // 审计日志
      { id: 'setting:log:view', label: 'View Audit Logs' },
      { id: 'setting:log:export', label: 'Export Audit Logs' },
      { id: 'setting:log:purge', label: 'Purge Audit Logs' },
    ],
  },
])

const mergePermissionNodes = (base: SettingPermissionNode[], incoming: SettingPermissionNode[]): SettingPermissionNode[] => {
  const incomingMap = new Map(incoming.map((node) => [node.id, node]))
  const merged = base.map((baseNode) => {
    const hit = incomingMap.get(baseNode.id)
    if (!hit) {
      return clone(baseNode)
    }
    const baseChildren = baseNode.children || []
    const hitChildren = hit.children || []
    return {
      ...clone(baseNode),
      ...clone(hit),
      children: baseChildren.length ? mergePermissionNodes(baseChildren, hitChildren) : clone(hitChildren),
    }
  })

  const baseIds = new Set(base.map((node) => node.id))
  const extra = incoming.filter((node) => !baseIds.has(node.id)).map((node) => clone(node))
  return [...merged, ...extra]
}

const collectPermissionKeys = (nodes: SettingPermissionNode[]): string[] =>
  nodes.flatMap((node) => [node.id, ...(node.children ? collectPermissionKeys(node.children) : [])])

export const useSettingStore = defineStore('setting', () => {
  const loading = ref(false)
  const saving = ref(false)
  // Initialize via the canonical factory so newly-added fields (e.g.
  // the 2026-05 password / NTP / DB-pool / TTL-clamp knobs) auto-flow
  // here without forgetting to update this duplicate. The factory also
  // owns the documented defaults — keeping a second copy in sync by
  // hand is exactly the kind of bug we hit when adding new fields.
  const commonConfig = ref<SettingCommonConfig>(createDefaultGeneralConfigForm())

  const users = ref<SettingUser[]>([])
  const roles = ref<SettingRole[]>([])
  const permissionTree = ref<SettingPermissionNode[]>([])
  const rolePermissions = ref<Record<number, string[]>>({})
  const backups = ref<SettingBackup[]>([])
  const noticeConfig = ref<SettingNoticeConfig>({
    email: { enabled: false, smtpHost: '', smtpPort: 465, sender: '', authCode: '', receivers: '' },
    webhook: { enabled: false, url: '', method: 'POST', secret: '', alertTypes: [] },
    sms: { enabled: false, apiKey: '', templateId: '', phones: '' },
  })
  const logs = ref<SettingLogItem[]>([])

  const roleMap = computed<Record<number, string>>(() => {
    const map: Record<number, string> = {}
    roles.value.forEach((item) => {
      map[item.id] = item.name
    })
    return map
  })

  const appendLog = (payload: LogAppendPayload): void => {
    const next: SettingLogItem = {
      id: Date.now(),
      logId: `LOG-${Date.now()}`,
      operator: payload.operator || 'admin',
      actionType: payload.actionType || '配置',
      module: payload.module || '系统设置',
      content: payload.content || '执行配置操作',
      ip: payload.ip || '10.10.1.8',
      time: payload.time || new Date().toLocaleString('zh-CN', { hour12: false }),
      detail: payload.detail || '{}',
    }
    logs.value.unshift(next)
  }

  const fetchSettingData = async (): Promise<void> => {
    loading.value = true
    try {
      const { data } = await getSettingModuleDataApi() as { data: SettingModuleData }
      commonConfig.value = clone(data.common || commonConfig.value)
      users.value = clone(data.users || [])
      roles.value = clone(data.roles || [])
      const tree = clone(data.permissionTree || [])
      const defaultTree = defaultPermissionTreeData()
      permissionTree.value = mergePermissionNodes(defaultTree, tree)

      const serverRolePermissions = clone(data.rolePermissions || {})
      if (Object.keys(serverRolePermissions).length) {
        const allPermissionKeys = collectPermissionKeys(permissionTree.value)
        const adminPermissions = new Set([...(serverRolePermissions[1] || []), ...allPermissionKeys])
        serverRolePermissions[1] = Array.from(adminPermissions)
        rolePermissions.value = serverRolePermissions
      } else {
        const allPermissionKeys = collectPermissionKeys(permissionTree.value)
        const fallback: Record<number, string[]> = {}
        roles.value.forEach((role) => {
          fallback[role.id] = role.id === 1 ? allPermissionKeys : []
        })
        rolePermissions.value = fallback
      }
      backups.value = clone(data.backups || [])
      noticeConfig.value = clone(data.notice || noticeConfig.value)
      logs.value = clone(data.logs || [])
    } finally {
      loading.value = false
    }
  }

  const saveCommonConfig = async (payload: SettingCommonConfig): Promise<void> => {
    saving.value = true
    try {
      await saveCommonConfigApi(payload)
      commonConfig.value = { ...commonConfig.value, ...clone(payload) }
      appendLog({
        actionType: '配置',
        module: '常规设置',
        content: '保存常规配置',
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const resetCommonConfig = async (): Promise<SettingCommonConfig> => {
    saving.value = true
    try {
      const { data } = await resetCommonConfigApi()
      commonConfig.value = clone(data)
      appendLog({
        actionType: '配置',
        module: '常规设置',
        content: '重置默认配置',
        detail: JSON.stringify(data),
      })
      return data
    } finally {
      saving.value = false
    }
  }

  const saveUser = async (payload: Partial<SettingUser>): Promise<void> => {
    saving.value = true
    try {
      const { data } = await saveUserApi(payload)
      const roleName = roleMap.value[data.roleId] || data.roleName || ''
      const normalized: SettingUser = {
        id: Number(data.id || payload.id || Date.now()),
        username: String(data.username || payload.username || ''),
        password: data.password || payload.password,
        realName: String(data.realName || payload.realName || ''),
        roleId: Number(data.roleId || payload.roleId || 0),
        roleName: String(roleName),
        status: String(data.status || payload.status || '启用'),
        createdAt: String(data.createdAt || payload.createdAt || new Date().toLocaleString('zh-CN', { hour12: false })),
      }
      const index = users.value.findIndex((item) => item.id === normalized.id)
      if (index >= 0) {
        users.value[index] = normalized
      } else {
        users.value.unshift(normalized)
      }
      appendLog({
        actionType: payload.id ? '编辑' : '新增',
        module: '用户管理',
        content: `${payload.id ? '编辑' : '新增'}用户 ${payload.username}`,
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const toggleUserStatus = async (userId: number): Promise<void> => {
    const target = users.value.find((item) => item.id === userId)
    if (!target) {
      return
    }
    const nextStatus = target.status === '启用' ? '禁用' : '启用'
    await saveUser({ ...target, status: nextStatus })
  }

  const resetUserPassword = async (payload: Pick<SettingUser, 'id' | 'username'>): Promise<string | undefined> => {
    saving.value = true
    try {
      // Backend now auto-generates a strong temp password when the body's
      // `password` field is empty and returns it in the response so the
      // operator can copy it once and hand it to the user out-of-band.
      // We surface that string here so the page can pop a "copy this
      // password" dialog instead of just a "reset successful" toast.
      const res = await resetUserPasswordApi(payload)
      appendLog({
        actionType: '编辑',
        module: '用户管理',
        content: `重置用户 ${payload.username} 密码`,
        detail: JSON.stringify({ userId: payload.id }),
      })
      return res.data?.password
    } finally {
      saving.value = false
    }
  }

  const deleteUser = async (id: number): Promise<void> => {
    saving.value = true
    try {
      const target = users.value.find((item) => item.id === id)
      await deleteUserApi(id)
      users.value = users.value.filter((item) => item.id !== id)
      appendLog({
        actionType: '删除',
        module: '用户管理',
        content: `删除用户 ${target?.username || id}`,
        detail: JSON.stringify({ id }),
      })
    } finally {
      saving.value = false
    }
  }

  const saveRole = async (payload: Partial<SettingRole>): Promise<void> => {
    saving.value = true
    try {
      const { data } = await saveRoleApi(payload)
      const normalized: SettingRole = {
        id: Number(data.id || payload.id || Date.now()),
        name: String(data.name || payload.name || ''),
        remark: String(data.remark || payload.remark || ''),
        createdAt: String(data.createdAt || payload.createdAt || new Date().toLocaleString('zh-CN', { hour12: false })),
      }
      const index = roles.value.findIndex((item) => item.id === normalized.id)
      if (index >= 0) {
        roles.value[index] = normalized
      } else {
        const newRoleId = normalized.id || Date.now()
        roles.value.push({ ...normalized, id: newRoleId })
        rolePermissions.value[newRoleId] = []
      }
      appendLog({
        actionType: payload.id ? '编辑' : '新增',
        module: '角色权限',
        content: `${payload.id ? '编辑' : '新增'}角色 ${payload.name}`,
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const deleteRole = async (id: number): Promise<void> => {
    saving.value = true
    try {
      const target = roles.value.find((item) => item.id === id)
      await deleteRoleApi(id)
      roles.value = roles.value.filter((item) => item.id !== id)
      delete rolePermissions.value[id]
      appendLog({
        actionType: '删除',
        module: '角色权限',
        content: `删除角色 ${target?.name || id}`,
        detail: JSON.stringify({ id }),
      })
    } finally {
      saving.value = false
    }
  }

  const saveRolePermissions = async (payload: SaveRolePermissionPayload): Promise<void> => {
    saving.value = true
    try {
      await saveRolePermissionsApi(payload)
      rolePermissions.value[payload.roleId] = clone(payload.permissions || [])
      appendLog({
        actionType: '配置',
        module: '角色权限',
        content: `保存角色权限 roleId=${payload.roleId}`,
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const createBackup = async (payload: { backupScope: string[]; backupFormat: string }): Promise<SettingBackup> => {
    saving.value = true
    try {
      const { data } = await createBackupApi(payload)
      backups.value.unshift(clone(data))
      appendLog({
        actionType: '新增',
        module: '备份还原',
        content: `创建备份 ${data.backupId}`,
        detail: JSON.stringify(data),
      })
      return data
    } finally {
      saving.value = false
    }
  }

  const restoreBackup = async (payload: { fileName: string; restoreScope: string[]; backupId?: string }): Promise<void> => {
    saving.value = true
    try {
      await restoreBackupApi(payload)
      appendLog({
        actionType: '配置',
        module: '备份还原',
        content: `执行还原 ${payload.fileName || payload.backupId || ''}`,
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const deleteBackup = async (id: number): Promise<void> => {
    saving.value = true
    try {
      const target = backups.value.find((item) => item.id === id)
      await deleteBackupApi(id)
      backups.value = backups.value.filter((item) => item.id !== id)
      appendLog({
        actionType: '删除',
        module: '备份还原',
        content: `删除备份 ${target?.backupId || id}`,
        detail: JSON.stringify({ id }),
      })
    } finally {
      saving.value = false
    }
  }

  const uploadBackupFile = async (payload: { fileName: string }): Promise<{ fileName: string; restoreScope: string[]; format: string } | null> => {
    saving.value = true
    try {
      const { data } = await uploadBackupFileApi(payload)
      appendLog({
        actionType: '配置',
        module: '备份还原',
        content: `上传备份文件 ${payload.fileName}`,
        detail: JSON.stringify(payload),
      })
      return data
    } finally {
      saving.value = false
    }
  }

  const saveNoticeConfig = async (payload: SettingNoticeConfig): Promise<void> => {
    saving.value = true
    try {
      await saveNoticeConfigApi(payload)
      noticeConfig.value = clone(payload)
      appendLog({
        actionType: '配置',
        module: '通知设置',
        content: '保存通知配置',
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const testNoticeChannel = async (payload: { channel: string }): Promise<void> => {
    saving.value = true
    try {
      await testNoticeChannelApi(payload)
      appendLog({
        actionType: '配置',
        module: '通知设置',
        content: `测试通知通道 ${payload.channel}`,
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  const fetchLogs = async (): Promise<void> => {
    loading.value = true
    try {
      const { data } = await getLogsApi()
      logs.value = clone(data || logs.value)
    } finally {
      loading.value = false
    }
  }

  const exportLogs = async (payload: { count: number }): Promise<void> => {
    saving.value = true
    try {
      await exportLogsApi(payload)
      appendLog({
        actionType: '配置',
        module: '操作日志',
        content: '导出操作日志',
        detail: JSON.stringify(payload),
      })
    } finally {
      saving.value = false
    }
  }

  return {
    loading,
    saving,
    commonConfig,
    users,
    roles,
    permissionTree,
    rolePermissions,
    backups,
    noticeConfig,
    logs,
    roleMap,
    fetchSettingData,
    saveCommonConfig,
    resetCommonConfig,
    saveUser,
    toggleUserStatus,
    resetUserPassword,
    deleteUser,
    saveRole,
    deleteRole,
    saveRolePermissions,
    createBackup,
    restoreBackup,
    deleteBackup,
    uploadBackupFile,
    saveNoticeConfig,
    testNoticeChannel,
    fetchLogs,
    exportLogs,
  }
})
