<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules, UploadRawFile } from 'element-plus'
import { CircleCheckFilled, CircleCloseFilled, InfoFilled, RefreshRight, WarningFilled } from '@element-plus/icons-vue'
import { useDomainStore } from '../../stores/domain'
import type { DomainZone } from '../../types/modules'
import BaseModal from '../../components/BaseModal.vue'
import { loadTableState, saveTableState } from '../../utils/tableState'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'

type SortOrder = 'ascending' | 'descending' | null
type BatchAction = 'enable' | 'disable' | 'delete'
type ExportType = 'xlsx' | 'json'

interface ZoneFilters {
  keyword: string
  type: '' | 'Primary' | 'Secondary' | 'Reverse' | 'Stub' | 'Forward'
  status: '' | '正常' | '禁用' | '异常' | '同步中'
  timeRange: string[]
}

interface ZoneSorter {
  prop: keyof DomainZone
  order: Exclude<SortOrder, null>
}

interface ZonePager {
  page: number
  size: number
}

interface ZoneFormModel {
  type: 'Primary' | 'Secondary' | 'Reverse' | 'Stub' | 'Forward'
  domain: string
  zoneId: string
  upstream: string
  remark: string
}

interface ZoneTableState {
  filters: ZoneFilters
  sorter: ZoneSorter
  pager: ZonePager
}

interface EditFormModel extends ZoneFormModel {
  id: number | null
  status: '正常' | '禁用' | '异常' | '同步中'
}

const zoneTypeOptions = computed<Array<{ label: string; value: ZoneFormModel['type'] }>>(() => [
  { label: t('zone.typePrimary'), value: 'Primary' },
  { label: t('zone.typeSecondary'), value: 'Secondary' },
  { label: t('zone.typeReverse'), value: 'Reverse' },
  { label: t('zone.typeStub'), value: 'Stub' },
  { label: t('zone.typeForward'), value: 'Forward' },
])

const router = useRouter()
const { t } = useI18n()
const domainStore = useDomainStore()

const zoneDialogVisible = ref(false)
const editDialogVisible = ref(false)
const uploadRef = ref()
const zoneFormRef = ref<FormInstance>()
const editFormRef = ref<FormInstance>()
const selectedRows = ref<DomainZone[]>([])
const refreshing = ref(false)
const batchLoading = ref(false)
const deletingZoneId = ref<number | null>(null)
const switchingZoneId = ref<number | null>(null)
const exportLoading = ref(false)
const importLoading = ref(false)
const refreshCooldown = ref(false)
let sortDebounceTimer: number | null = null
let refreshCooldownTimer: number | null = null
const tableStateKey = 'modern-dns:zone-list:table-state'

const filters = reactive<ZoneFilters>({ keyword: '', type: '', status: '', timeRange: [] })
const sorter = reactive<ZoneSorter>({ prop: 'createdAt', order: 'descending' })
const pager = reactive<ZonePager>({ page: 1, size: 5 })
const zoneForm = reactive<ZoneFormModel>({ type: 'Primary', domain: '', zoneId: '', upstream: '', remark: '' })
const editForm = reactive<EditFormModel>({ id: null, domain: '', zoneId: '', type: 'Primary', status: '正常', upstream: '', remark: '' })

const cachedState = loadTableState<ZoneTableState>(tableStateKey, {
  filters: { keyword: '', type: '', status: '', timeRange: [] },
  sorter: { prop: 'createdAt', order: 'descending' },
  pager: { page: 1, size: 5 },
})

Object.assign(filters, cachedState.filters)
Object.assign(sorter, cachedState.sorter)
Object.assign(pager, cachedState.pager)

watch(
  () => ({
    filters: { ...filters, timeRange: [...filters.timeRange] },
    sorter: { ...sorter },
    pager: { ...pager },
  }),
  (next) => {
    saveTableState(tableStateKey, next)
  },
  { deep: true },
)

const domainPattern = /^(?:[a-zA-Z0-9_-]+\.)+[a-zA-Z]{2,}|(?:\d+\.){2,}\d+\.in-addr\.arpa$/
const ipv4Pattern = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/

const statusOptions = computed<Array<{ label: string; value: ZoneFilters['status'] }>>(() => [
  { label: t('zone.normal'), value: '正常' },
  { label: t('zone.disabled'), value: '禁用' },
  { label: t('zone.abnormal'), value: '异常' },
  { label: t('zone.syncing'), value: '同步中' },
])

const statusByType: Record<Exclude<ZoneFilters['type'], ''>, ZoneFilters['status'][]> = {
  Primary: ['正常', '禁用', '异常'],
  Secondary: ['正常', '禁用', '异常', '同步中'],
  Reverse: ['正常', '禁用'],
  Stub: ['正常', '禁用', '同步中'],
  Forward: ['正常', '禁用', '同步中'],
}

const availableStatusOptions = computed(() => {
  if (!filters.type) {
    return statusOptions.value
  }
  const allow = statusByType[filters.type]
  return statusOptions.value.filter((item) => allow.includes(item.value))
})

watch(
  () => filters.type,
  () => {
    if (filters.status && !availableStatusOptions.value.some((item) => item.value === filters.status)) {
      filters.status = ''
    }
  },
)

const showCreateUpstreamField = computed(() => ['Forward', 'Stub'].includes(zoneForm.type))
const showEditUpstreamField = computed(() => editForm.type === 'Forward')

const isCreateFormReady = computed(() => {
  const baseOk = Boolean(zoneForm.type && zoneForm.domain.trim()) && domainPattern.test(zoneForm.domain)
  if (!baseOk) {
    return false
  }
  if (showCreateUpstreamField.value && !zoneForm.upstream.trim()) {
    return false
  }
  return true
})

const isEditFormReady = computed(() => {
  const baseOk = Boolean(editForm.type && editForm.domain.trim()) && domainPattern.test(editForm.domain)
  if (!baseOk) {
    return false
  }
  if (showEditUpstreamField.value && !editForm.upstream.trim()) {
    return false
  }
  return true
})

const tableHeaderStyle = () => ({
  textAlign: 'center',
  height: '44px',
})

const tableCellStyle = () => ({
  textAlign: 'center',
  height: '42px',
})

const zoneRules = computed<FormRules>(() => ({
  type: [{ required: true, message: t('zone.valSelectType'), trigger: 'change' }],
  domain: [
    { required: true, message: t('zone.valDomainRequired'), trigger: 'blur' },
    { pattern: domainPattern, message: t('zone.valDomainInvalid'), trigger: 'blur' },
  ],
  upstream: [{ validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
    if (['Forward', 'Stub'].includes(zoneForm.type) && !value) {
      callback(new Error(t('zone.valUpstreamRequired')))
      return
    }
    callback()
  }, trigger: ['blur', 'change'] }],
}))

const editRules = computed<FormRules>(() => ({
  type: [{ required: true, message: t('zone.valSelectType'), trigger: 'change' }],
  domain: [
    { required: true, message: t('zone.valDomainRequired'), trigger: ['blur', 'change'] },
    { pattern: domainPattern, message: t('zone.valDomainInvalid'), trigger: ['blur', 'change'] },
  ],
  upstream: [{ validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
    if (editForm.type !== 'Forward') {
      callback()
      return
    }
    const normalized = String(value || '').trim()
    if (!normalized) {
      callback(new Error(t('zone.valForwardUpstreamRequired')))
      return
    }
    if (!ipv4Pattern.test(normalized)) {
      callback(new Error(t('zone.valIpv4Invalid')))
      return
    }
    callback()
  }, trigger: ['blur', 'change'] }],
}))

const statusTagType = (status: string): 'success' | 'danger' | 'warning' | 'info' => {
  const map: Record<string, 'success' | 'danger' | 'warning' | 'info'> = { 正常: 'success', 异常: 'danger', 同步中: 'warning', 禁用: 'info' }
  return map[status] || 'info'
}
const statusTagIcon = (status: string) => ({ 正常: CircleCheckFilled, 异常: CircleCloseFilled, 同步中: WarningFilled, 禁用: InfoFilled }[status] || InfoFilled)
const statusBadgeClass = (status: string): string => {
  const map: Record<string, string> = { 正常: 'mn-badge--success', 异常: 'mn-badge--danger', 同步中: 'mn-badge--warning', 禁用: 'mn-badge--neutral' }
  return map[status] || 'mn-badge--neutral'
}
const zoneTypeLabel = (type: string) => ({ Primary: t('zone.typePrimary'), Secondary: t('zone.typeSecondary'), Reverse: t('zone.typeReverse'), Stub: t('zone.typeStub'), Forward: t('zone.typeForward') }[type] || type)
const zoneStatusText = (status: string) => ({ '正常': t('zone.normal'), '异常': t('zone.abnormal'), '同步中': t('zone.syncing'), '禁用': t('zone.disabled') }[status] || status)

const resetZoneForm = () => {
  Object.assign(zoneForm, {
    type: 'Primary',
    domain: '',
    zoneId: `Z-${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${Math.floor(Math.random() * 900 + 100)}`,
    upstream: '',
    remark: '',
  })
}

const loadData = async () => {
  await domainStore.fetchDomainData()
}

const filteredZones = computed(() =>
  domainStore.sortedZones.filter((item) => {
    const matchKeyword = !filters.keyword || item.domain.includes(filters.keyword) || item.remark.includes(filters.keyword)
    const matchType = !filters.type || item.type === filters.type
    const matchStatus = !filters.status || item.status === filters.status
    const matchTime = !filters.timeRange.length || (item.createdAt >= `${filters.timeRange[0]} 00:00:00` && item.createdAt <= `${filters.timeRange[1]} 23:59:59`)
    return matchKeyword && matchType && matchStatus && matchTime
  }),
)

const sortedFilteredZones = computed(() => {
  const rows = [...filteredZones.value]
  if (!sorter.prop || !sorter.order) {
    return rows
  }
  const factor = sorter.order === 'ascending' ? 1 : -1
  rows.sort((left, right) => {
    const leftValue = left[sorter.prop]
    const rightValue = right[sorter.prop]
    return String(leftValue).localeCompare(String(rightValue)) * factor
  })
  return rows
})

const pagedZones = computed(() => {
  const start = (pager.page - 1) * pager.size
  return sortedFilteredZones.value.slice(start, start + pager.size)
})

const refreshData = async () => {
  if (refreshing.value || refreshCooldown.value) {
    return
  }
  refreshing.value = true
  try {
    await loadData()
    ElMessage.success({
      message: t('zone.zoneRefreshed'),
      duration: 3000,
      customClass: 'dns-success-toast',
    })
  } finally {
    refreshing.value = false
    refreshCooldown.value = true
    if (refreshCooldownTimer) {
      window.clearTimeout(refreshCooldownTimer)
    }
    refreshCooldownTimer = window.setTimeout(() => {
      refreshCooldown.value = false
      refreshCooldownTimer = null
    }, 1000)
  }
}

const openCreateDialog = () => {
  resetZoneForm()
  zoneDialogVisible.value = true
}

const submitZone = async () => {
  if (domainStore.submitting) {
    return
  }
  const valid = await validateFormAndFocus(zoneFormRef.value)
  if (!valid) {
    return
  }
  const zone = await domainStore.createZone(zoneForm)
  resetZoneForm()
  zoneDialogVisible.value = false
  ElMessage.success(t('zone.zoneCreated'))
  router.push({ name: 'domain-zone-edit', params: { id: zone.id } })
}

const openEditDialog = (row: DomainZone) => {
  Object.assign(editForm, row)
  editDialogVisible.value = true
}

const submitEdit = async () => {
  if (domainStore.submitting) {
    return
  }
  const valid = await validateFormAndFocus(editFormRef.value)
  if (!valid) {
    return
  }
  await domainStore.updateZone(editForm.id, {
    domain: editForm.domain,
    zoneId: editForm.zoneId,
    type: editForm.type,
    status: editForm.status,
    upstream: editForm.upstream,
    remark: editForm.remark,
  })
  editDialogVisible.value = false
  ElMessage.success(t('zone.zoneUpdated'))
}

const ensureSelected = () => {
  if (!selectedRows.value.length) {
    ElMessage.warning(t('zone.selectAtLeastOne'))
    return false
  }
  return true
}

const handleBatchAction = async (action: BatchAction) => {
  if (batchLoading.value) {
    return
  }
  if (!ensureSelected()) {
    return
  }
  batchLoading.value = true
  const idList = selectedRows.value.map((item) => item.id)
  try {
    if (action === 'enable') {
      await confirmRiskAction({ title: t('zone.batchEnableConfirmTitle'), action: t('zone.batchEnableAction'), risk: t('zone.batchEnableRisk') })
      domainStore.batchUpdateZoneStatus(idList, '正常')
    }
    if (action === 'disable') {
      await confirmRiskAction({ title: t('zone.batchDisableConfirmTitle'), action: t('zone.batchDisableAction'), risk: t('zone.batchDisableRisk') })
      domainStore.batchUpdateZoneStatus(idList, '禁用')
    }
    if (action === 'delete') {
      await confirmRiskAction({ title: t('zone.batchDeleteConfirmTitle'), action: t('zone.batchDeleteAction'), risk: t('zone.batchDeleteRisk') })
      domainStore.deleteZones(idList)
    }
    ElMessage.success({ enable: t('zone.batchEnableDone'), disable: t('zone.batchDisableDone'), delete: t('zone.batchDeleteDone') }[action] || '')
  } finally {
    batchLoading.value = false
  }
}

const toggleZoneStatus = async (row: DomainZone, value: boolean) => {
  if (switchingZoneId.value) {
    return
  }
  switchingZoneId.value = row.id
  try {
    await domainStore.updateZone(row.id, { status: value ? '正常' : '禁用' })
    ElMessage.success(value ? t('zone.zoneEnabled') : t('zone.zoneDisabled'))
  } finally {
    switchingZoneId.value = null
  }
}

const removeZone = async (row: DomainZone) => {
  if (deletingZoneId.value) {
    return
  }
  deletingZoneId.value = row.id
  try {
    await confirmRiskAction({ title: t('zone.deleteConfirmTitle'), action: t('zone.deleteAction'), target: row.domain, risk: t('zone.deleteRisk') })
    domainStore.deleteZones([row.id])
    ElMessage.success(t('zone.zoneDeleted'))
  } finally {
    deletingZoneId.value = null
  }
}

const goToZoneEdit = (row: DomainZone) => {
  router.push({ name: 'domain-zone-edit', params: { id: row.id } })
}

const exportZones = async (fileType: ExportType) => {
  if (exportLoading.value) {
    return
  }
  exportLoading.value = true
  const rows = selectedRows.value.length ? selectedRows.value : sortedFilteredZones.value
  if (!rows.length) {
    ElMessage.warning(t('zone.noExportData'))
    exportLoading.value = false
    return
  }
  try {
    if (fileType === 'json') {
      const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json;charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'zones.json'
      link.click()
      URL.revokeObjectURL(url)
    } else {
      const XLSX = await import('xlsx')
      const worksheet = XLSX.utils.json_to_sheet(rows)
      const workbook = XLSX.utils.book_new()
      XLSX.utils.book_append_sheet(workbook, worksheet, 'Zones')
      XLSX.writeFile(workbook, 'zones.xlsx')
    }
    ElMessage.success(t('zone.exportDone'))
  } finally {
    exportLoading.value = false
  }
}

const handleExportCommand = async (command) => {
  await exportZones(command as ExportType)
}

const parseImportedRows = async (file: UploadRawFile) => {
  if (importLoading.value) {
    return false
  }
  importLoading.value = true
  try {
    const raw = await file.arrayBuffer()
    let rows: Array<Record<string, any>> = []
    if (file.name.endsWith('.json')) {
      rows = JSON.parse(new TextDecoder().decode(raw))
    } else {
      const XLSX = await import('xlsx')
      const workbook = XLSX.read(raw)
      const firstSheet = workbook.Sheets[workbook.SheetNames[0]]
      rows = XLSX.utils.sheet_to_json(firstSheet)
    }
    for (const item of rows) {
      await domainStore.createZone({
        type: item.type || item['Zone类型'] || 'Primary',
        domain: item.domain || item['域名/Zone名称'] || item.zone || 'imported.zone',
        zoneId: item.zoneId || item['Zone ID'],
        upstream: item.upstream || item['上游服务器'] || '',
        remark: item.remark || item['域名备注'] || '',
      })
    }
    ElMessage.success(t('zone.importDone', { count: rows.length }))
  } finally {
    importLoading.value = false
  }
  return false
}

const handleSortChange = ({ prop, order }: { prop: string; order: SortOrder }) => {
  if (sortDebounceTimer) {
    window.clearTimeout(sortDebounceTimer)
  }
  sortDebounceTimer = window.setTimeout(() => {
    sorter.prop = (prop as keyof DomainZone) || 'createdAt'
    sorter.order = order || 'descending'
  }, 180)
}

const tableRowClassName = ({ row }: { row: DomainZone }) => (row.status === '异常' ? 'domain-row-danger' : '')

/* ─────────── Zone Options Dialog ─────────── */
type QueryAccessMode = 'deny' | 'allow' | 'private' | 'ns-only' | 'acl' | 'ns-acl'
type TransferMode    = 'deny' | 'allow' | 'ns-only' | 'acl'
type NotifyMode      = 'none' | 'ns' | 'custom'
type DynUpdateMode   = 'deny' | 'allow' | 'private' | 'acl'

interface ZoneOptions {
  zone: string
  queryAccess: { mode: QueryAccessMode; acl: string }
  transfer:    { mode: TransferMode;    acl: string }
  notify:      { mode: NotifyMode;      targets: string; notifyOnChange: boolean }
  dynUpdate:   { mode: DynUpdateMode;   acl: string; tsigRequired: boolean; allowTypes: string[] }
}

const zoneOptionsVisible  = ref(false)
const zoneOptionsTab      = ref('query')
const zoneOptionsSaving   = ref(false)
const activeZoneOptions   = ref<ZoneOptions>({
  zone: '',
  queryAccess: { mode: 'allow', acl: '' },
  transfer:    { mode: 'deny',  acl: '' },
  notify:      { mode: 'ns',    targets: '', notifyOnChange: true },
  dynUpdate:   { mode: 'deny',  acl: '', tsigRequired: true, allowTypes: ['A','AAAA'] },
})

const openZoneOptions = (row: DomainZone) => {
  activeZoneOptions.value = {
    zone: row.domain,
    queryAccess: { mode: 'allow', acl: '' },
    transfer:    { mode: row.type === 'Primary' ? 'ns-only' : 'deny', acl: '' },
    notify:      { mode: row.type === 'Primary' ? 'ns' : 'none', targets: '', notifyOnChange: true },
    dynUpdate:   { mode: 'deny', acl: '', tsigRequired: true, allowTypes: ['A','AAAA'] },
  }
  zoneOptionsTab.value = 'query'
  zoneOptionsVisible.value = true
}

const saveZoneOptions = async () => {
  zoneOptionsSaving.value = true
  await new Promise(r => window.setTimeout(r, 600))
  zoneOptionsSaving.value = false
  zoneOptionsVisible.value = false
  ElMessage.success(t('zone.zoneOptionsSaved', { zone: activeZoneOptions.value.zone }))
}

const dynUpdateRecordTypes = ['A','AAAA','CNAME','MX','TXT','SRV','PTR','NS']

const onZoneSelectionChange = (rows: DomainZone[]) => {
  selectedRows.value = rows
}

const handleRowCommand = async (row: DomainZone, command: 'edit' | 'records') => {
  if (command === 'edit') {
    openEditDialog(row)
    return
  }
  goToZoneEdit(row)
}

const closeCreateDialog = async () => {
  if (!zoneForm.domain.trim() && !zoneForm.upstream.trim() && !zoneForm.remark.trim()) {
    zoneDialogVisible.value = false
    return
  }
  await confirmRiskAction({ title: t('zone.abandonEditTitle'), action: t('zone.abandonEditAction'), risk: t('zone.abandonEditRisk') })
  zoneDialogVisible.value = false
}

onMounted(async () => {
  await loadData()
  domainStore.startPolling()
})

onBeforeUnmount(() => {
  if (sortDebounceTimer) {
    window.clearTimeout(sortDebounceTimer)
    sortDebounceTimer = null
  }
  if (refreshCooldownTimer) {
    window.clearTimeout(refreshCooldownTimer)
    refreshCooldownTimer = null
  }
  domainStore.stopPolling()
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('zone.title') }}</h1>
        <p class="page-subtitle">{{ $t('zone.subtitle') }}</p>
      </div>
      <div class="zone-last-updated">
        {{ t('zone.lastRefresh') }}{{ domainStore.lastUpdated || t('zone.loading') }}
      </div>
    </div>

    <!-- Stats strip -->
    <div class="mn-stats-strip">
      <div class="mn-stat-tile">
        <span class="mn-stat-value">{{ domainStore.zones.length }}</span>
        <span class="mn-stat-label">{{ t('zone.totalZones') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--success">
        <span class="mn-stat-value">{{ domainStore.zones.filter(z => z.status === '正常').length }}</span>
        <span class="mn-stat-label">{{ t('zone.normalRunning') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--danger">
        <span class="mn-stat-value">{{ domainStore.zones.filter(z => z.status === '异常').length }}</span>
        <span class="mn-stat-label">{{ t('zone.abnormal') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--neutral">
        <span class="mn-stat-value">{{ domainStore.zones.filter(z => z.status === '禁用').length }}</span>
        <span class="mn-stat-label">{{ t('zone.disabled') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--warning">
        <span class="mn-stat-value">{{ domainStore.zones.filter(z => z.status === '同步中').length }}</span>
        <span class="mn-stat-label">{{ t('zone.syncing') }}</span>
      </div>
    </div>

    <el-card class="mn-card">
      <!-- Toolbar -->
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="filters.keyword" clearable :placeholder="t('zone.searchPlaceholderDomain')" style="width:220px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filters.type" clearable :placeholder="t('zone.zoneTypePlaceholder')" style="width:130px">
            <el-option v-for="item in zoneTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="filters.status" clearable :placeholder="t('zone.statusPlaceholder')" style="width:120px">
            <el-option v-for="item in availableStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-date-picker
            v-model="filters.timeRange"
            type="daterange"
            value-format="YYYY-MM-DD"
            :range-separator="t('zone.dateRangeSep')"
            :start-placeholder="t('zone.dateStart')"
            :end-placeholder="t('zone.dateEnd')"
            style="width:260px"
          />
          <el-button :loading="refreshing" :disabled="refreshing || refreshCooldown || batchLoading" @click="refreshData">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ t('zone.refresh') }}
          </el-button>
        </div>
        <div class="mn-toolbar-actions">
          <el-button :loading="batchLoading" :disabled="batchLoading || !selectedRows.length" @click="handleBatchAction('enable')">{{ t('zone.batchEnable') }}</el-button>
          <el-button :loading="batchLoading" :disabled="batchLoading || !selectedRows.length" @click="handleBatchAction('disable')">{{ t('zone.batchDisable') }}</el-button>
          <el-button type="danger" plain :loading="batchLoading" :disabled="batchLoading || !selectedRows.length" @click="handleBatchAction('delete')">{{ t('zone.batchDelete') }}</el-button>
          <el-upload ref="uploadRef" :show-file-list="false" :auto-upload="false" :before-upload="parseImportedRows" accept=".xlsx,.xls,.json">
            <el-button :loading="importLoading" :disabled="importLoading">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg></template>
              {{ t('zone.import') }}
            </el-button>
          </el-upload>
          <el-dropdown @command="handleExportCommand">
            <el-button :loading="exportLoading" :disabled="exportLoading">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
              {{ t('zone.export') }}
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="xlsx">{{ t('zone.exportExcel') }}</el-dropdown-item>
                <el-dropdown-item command="json">{{ t('zone.exportJson') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button type="primary" @click="openCreateDialog">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
            {{ t('zone.addZone') }}
          </el-button>
        </div>
      </div>

      <!-- Selection hint -->
      <div v-if="selectedRows.length" class="mn-selection-hint">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
        {{ t('zone.selected') }} <strong>{{ selectedRows.length }}</strong> {{ t('zone.zoneItems') }}
      </div>

      <!-- Table -->
      <div v-loading="domainStore.loading">
        <el-table
          v-if="sortedFilteredZones.length"
          class="mn-table"
          :data="pagedZones"
          stripe
          table-layout="fixed"
          @selection-change="onZoneSelectionChange"
          @sort-change="handleSortChange"
          :row-class-name="tableRowClassName"
        >
          <el-table-column type="selection" width="48" />
          <el-table-column prop="domain" :label="t('zone.domainOrZone')" min-width="260" sortable="custom">
            <template #default="{ row }">
              <el-button link type="primary" class="mn-domain-link" @click="goToZoneEdit(row)">
                <svg class="mn-domain-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
                <span class="mn-mono">{{ row.domain }}</span>
              </el-button>
            </template>
          </el-table-column>
          <el-table-column :label="t('zone.zoneType')" width="130" sortable align="center">
            <template #default="{ row }">
              <span class="mn-badge mn-badge--neutral">{{ zoneTypeLabel(row.type) }}</span>
            </template> 
          </el-table-column>
          <el-table-column :label="t('zone.runStatus')" width="120" sortable="custom" align="center">
            <template #default="{ row }">
              <span :class="['mn-badge', statusBadgeClass(row.status)]">{{ zoneStatusText(row.status) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="remark" :label="t('zone.remark')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }"><span class="mn-remark">{{ row.remark || '—' }}</span></template>
          </el-table-column>
          <el-table-column prop="createdAt" :label="t('zone.createdAt')" min-width="180" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ row.createdAt }}</span></template>
          </el-table-column>
          <el-table-column :label="t('zone.operation')" width="300" fixed="right" align="center">
            <template #default="{ row }">
              <div class="mn-row-ops">
                <el-button link type="primary" @click="goToZoneEdit(row)">{{ t('zone.recordManage') }}</el-button>
                <el-button link type="primary" @click="openEditDialog(row)">{{ t('zone.edit') }}</el-button>
                <el-button link type="primary" @click="openZoneOptions(row)">{{ t('zone.zoneOptions') }}</el-button>
                <el-switch
                  :model-value="row.status !== '禁用'"
                  :loading="switchingZoneId === row.id"
                  :disabled="Boolean(switchingZoneId) || Boolean(deletingZoneId) || batchLoading"
                  inline-prompt
                  :active-text="t('zone.enableSwitch')"
                  :inactive-text="t('zone.disableSwitch')"
                  @change="toggleZoneStatus(row, $event)"
                />
                <el-button link type="danger" :loading="deletingZoneId === row.id" :disabled="Boolean(deletingZoneId) || batchLoading" @click="removeZone(row)">{{ t('zone.delete') }}</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('zone.noZoneData')" :image-size="80">
          <el-button type="primary" @click="openCreateDialog">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
            {{ t('zone.addZone') }}
          </el-button>
        </el-empty>
      </div>

      <div v-if="sortedFilteredZones.length" class="mn-pagination">
        <el-pagination
          v-model:current-page="pager.page"
          v-model:page-size="pager.size"
          size="small"
          background
          layout="total, sizes, prev, pager, next"
          :total="sortedFilteredZones.length"
          :page-sizes="[5, 10, 20]"
        />
      </div>
    </el-card>

    <BaseModal
      v-model="zoneDialogVisible"
      :title="t('zone.addZone')"
      :width="560"
      :loading="domainStore.submitting"
      :confirm-disabled="!isCreateFormReady || domainStore.submitting"
      :confirm-text="t('zone.createZone')"
      :cancel-text="t('zone.cancel')"
      @confirm="submitZone"
      @cancel="closeCreateDialog"
      @close="closeCreateDialog"
    >
      <el-form ref="zoneFormRef" :model="zoneForm" :rules="zoneRules" label-position="top" class="mn-modal-form">
        <el-form-item :label="t('zone.zoneType')" prop="type">
          <el-select v-model="zoneForm.type" :placeholder="t('zone.selectType')" style="width:100%">
            <el-option v-for="item in zoneTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('zone.domainLabel')" prop="domain">
          <el-input v-model="zoneForm.domain" :placeholder="t('zone.domainPlaceholder')" />
        </el-form-item>
        <el-form-item v-if="showCreateUpstreamField" :label="t('zone.upstream')" prop="upstream">
          <el-input v-model="zoneForm.upstream" :placeholder="t('zone.upstreamPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('zone.remark')">
          <el-input v-model="zoneForm.remark" type="textarea" :rows="3" :placeholder="t('zone.remarkPlaceholder')" />
        </el-form-item>
      </el-form>
    </BaseModal>

    <BaseModal
      v-model="zoneOptionsVisible"
      :title="`${t('zone.zoneOptionsTitle')} — ${activeZoneOptions.zone}`"
      :width="680"
      :loading="zoneOptionsSaving"
      :confirm-text="t('zone.save')"
      :cancel-text="t('zone.cancel')"
      @confirm="saveZoneOptions"
      class="zone-opts-modal"
    >
      <el-tabs v-model="zoneOptionsTab" class="zone-opts-tabs">

        <el-tab-pane name="query">
          <template #label><span class="zo-tab-label"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="6"/><path d="M8 5v3l2 1"/></svg>{{ t('zone.queryAccessTab') }}</span></template>
          <div class="zo-pane">
            <p class="zo-desc">{{ t('zone.queryAccessDesc') }}</p>
            <div class="zo-option-list">
              <label v-for="opt in [
                { value:'deny',    label:t('zone.queryDenyAll'),    sub:t('zone.queryDenyAllSub'),    variant:'danger' },
                { value:'allow',   label:t('zone.queryAllowAll'),   sub:t('zone.queryAllowAllSub'),   variant:'success' },
                { value:'private', label:t('zone.queryPrivate'),    sub:t('zone.queryPrivateSub'),    variant:'warning' },
                { value:'ns-only', label:t('zone.queryNsOnly'),     sub:t('zone.queryNsOnlySub'),     variant:'info' },
                { value:'acl',     label:t('zone.queryAcl'),        sub:t('zone.queryAclSub'),        variant:'info' },
                { value:'ns-acl',  label:t('zone.queryNsAcl'),      sub:t('zone.queryNsAclSub'),      variant:'info' },
              ]" :key="opt.value"
                :class="['zo-option', `zo-option--${opt.variant}`, { 'is-active': activeZoneOptions.queryAccess.mode === opt.value }]"
                @click="activeZoneOptions.queryAccess.mode = opt.value as QueryAccessMode"
              >
                <span class="zo-option-radio"><span class="zo-option-radio-dot" /></span>
                <span class="zo-option-body">
                  <span class="zo-option-label">{{ opt.label }}</span>
                  <span class="zo-option-sub">{{ opt.sub }}</span>
                </span>
              </label>
            </div>
            <transition name="zo-slide">
              <div v-if="['acl','ns-acl'].includes(activeZoneOptions.queryAccess.mode)" class="zo-sub-panel">
                <div class="zo-field-label">{{ t('zone.aclWhitelist') }} <span class="zo-field-hint">{{ t('zone.aclWhitelistHint') }}</span></div>
                <el-input v-model="activeZoneOptions.queryAccess.acl" type="textarea" :rows="3" placeholder="10.0.0.0/8&#10;192.168.1.0/24" class="zo-textarea" />
              </div>
            </transition>
          </div>
        </el-tab-pane>

        <el-tab-pane name="transfer">
          <template #label><span class="zo-tab-label"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2 8h12M10 5l3 3-3 3"/></svg>{{ t('zone.transferTab') }}</span></template>
          <div class="zo-pane">
            <p class="zo-desc">{{ t('zone.transferDesc') }}</p>
            <div class="zo-option-list">
              <label v-for="opt in [
                { value:'deny',    label:t('zone.transferDeny'),    sub:t('zone.transferDenySub'),    variant:'danger' },
                { value:'allow',   label:t('zone.transferAllow'),   sub:t('zone.transferAllowSub'),   variant:'warning' },
                { value:'ns-only', label:t('zone.transferNsOnly'),  sub:t('zone.transferNsOnlySub'),  variant:'success' },
                { value:'acl',     label:t('zone.transferAcl'),     sub:t('zone.transferAclSub'),     variant:'info' },
              ]" :key="opt.value"
                :class="['zo-option', `zo-option--${opt.variant}`, { 'is-active': activeZoneOptions.transfer.mode === opt.value }]"
                @click="activeZoneOptions.transfer.mode = opt.value as TransferMode"
              >
                <span class="zo-option-radio"><span class="zo-option-radio-dot" /></span>
                <span class="zo-option-body">
                  <span class="zo-option-label">{{ opt.label }}</span>
                  <span class="zo-option-sub">{{ opt.sub }}</span>
                </span>
              </label>
            </div>
            <transition name="zo-slide">
              <div v-if="activeZoneOptions.transfer.mode === 'acl'" class="zo-sub-panel">
                <div class="zo-field-label">{{ t('zone.transferAclLabel') }} <span class="zo-field-hint">{{ t('zone.transferAclHint') }}</span></div>
                <el-input v-model="activeZoneOptions.transfer.acl" type="textarea" :rows="3" placeholder="192.168.1.10&#10;10.0.0.0/8" class="zo-textarea" />
              </div>
            </transition>
          </div>
        </el-tab-pane>

        <el-tab-pane name="notify">
          <template #label><span class="zo-tab-label"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2a4 4 0 0 1 4 4c0 4 1.5 5 1.5 5h-11S4 10 4 6a4 4 0 0 1 4-4zm-1 10h2"/></svg>{{ t('zone.notifyTab') }}</span></template>
          <div class="zo-pane">
            <p class="zo-desc">{{ t('zone.notifyDesc') }}</p>
            <div class="zo-option-list">
              <label v-for="opt in [
                { value:'none',   label:t('zone.notifyNone'),    sub:t('zone.notifyNoneSub'),    variant:'danger' },
                { value:'ns',     label:t('zone.notifyNs'),      sub:t('zone.notifyNsSub'),      variant:'success' },
                { value:'custom', label:t('zone.notifyCustom'),  sub:t('zone.notifyCustomSub'),  variant:'info' },
              ]" :key="opt.value"
                :class="['zo-option', `zo-option--${opt.variant}`, { 'is-active': activeZoneOptions.notify.mode === opt.value }]"
                @click="activeZoneOptions.notify.mode = opt.value as NotifyMode"
              >
                <span class="zo-option-radio"><span class="zo-option-radio-dot" /></span>
                <span class="zo-option-body">
                  <span class="zo-option-label">{{ opt.label }}</span>
                  <span class="zo-option-sub">{{ opt.sub }}</span>
                </span>
              </label>
            </div>
            <transition name="zo-slide">
              <div v-if="activeZoneOptions.notify.mode === 'custom'" class="zo-sub-panel">
                <div class="zo-field-label">{{ t('zone.notifyTargetLabel') }} <span class="zo-field-hint">{{ t('zone.notifyTargetHint') }}</span></div>
                <el-input v-model="activeZoneOptions.notify.targets" type="textarea" :rows="3" placeholder="192.168.1.20&#10;10.0.1.5" class="zo-textarea" />
              </div>
            </transition>
            <transition name="zo-slide">
              <div v-if="activeZoneOptions.notify.mode !== 'none'" class="zo-toggle-row">
                <div class="zo-toggle-text">
                  <span class="zo-toggle-label">{{ t('zone.notifyOnChange') }}</span>
                  <span class="zo-toggle-sub">{{ t('zone.notifyOnChangeSub') }}</span>
                </div>
                <el-switch v-model="activeZoneOptions.notify.notifyOnChange" />
              </div>
            </transition>
          </div>
        </el-tab-pane>

        <el-tab-pane name="dynupdate">
          <template #label><span class="zo-tab-label"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M13 8A5 5 0 1 1 8 3"/><polyline points="11 1 13 3 11 5"/></svg>{{ t('zone.dynUpdateTab') }}</span></template>
          <div class="zo-pane">
            <p class="zo-desc">{{ t('zone.dynUpdateDesc') }}</p>
            <div class="zo-option-list">
              <label v-for="opt in [
                { value:'deny',    label:t('zone.dynDeny'),    sub:t('zone.dynDenySub'),    variant:'danger' },
                { value:'allow',   label:t('zone.dynAllow'),   sub:t('zone.dynAllowSub'),   variant:'warning' },
                { value:'private', label:t('zone.dynPrivate'), sub:t('zone.dynPrivateSub'), variant:'info' },
                { value:'acl',     label:t('zone.dynAcl'),     sub:t('zone.dynAclSub'),     variant:'info' },
              ]" :key="opt.value"
                :class="['zo-option', `zo-option--${opt.variant}`, { 'is-active': activeZoneOptions.dynUpdate.mode === opt.value }]"
                @click="activeZoneOptions.dynUpdate.mode = opt.value as DynUpdateMode"
              >
                <span class="zo-option-radio"><span class="zo-option-radio-dot" /></span>
                <span class="zo-option-body">
                  <span class="zo-option-label">{{ opt.label }}</span>
                  <span class="zo-option-sub">{{ opt.sub }}</span>
                </span>
              </label>
            </div>
            <transition name="zo-slide">
              <div v-if="activeZoneOptions.dynUpdate.mode !== 'deny'" class="zo-extra">
                <div v-if="activeZoneOptions.dynUpdate.mode === 'acl'" class="zo-sub-panel">
                  <div class="zo-field-label">{{ t('zone.dynAclLabel') }} <span class="zo-field-hint">{{ t('zone.dynAclHint') }}</span></div>
                  <el-input v-model="activeZoneOptions.dynUpdate.acl" type="textarea" :rows="3" placeholder="10.0.0.0/8&#10;192.168.1.0/24" class="zo-textarea" />
                </div>
                <div class="zo-toggle-row">
                  <div class="zo-toggle-text">
                    <span class="zo-toggle-label">{{ t('zone.dynTsig') }}</span>
                    <span class="zo-toggle-sub">{{ t('zone.dynTsigSub') }}</span>
                  </div>
                  <el-switch v-model="activeZoneOptions.dynUpdate.tsigRequired" />
                </div>
                <div class="zo-rtype-block">
                  <div class="zo-field-label">{{ t('zone.dynAllowTypes') }}</div>
                  <div class="zo-rtype-chips">
                    <button
                      v-for="rt in dynUpdateRecordTypes" :key="rt" type="button"
                      :class="['zo-rtype-chip', { 'is-on': activeZoneOptions.dynUpdate.allowTypes.includes(rt) }]"
                      @click="activeZoneOptions.dynUpdate.allowTypes.includes(rt)
                        ? activeZoneOptions.dynUpdate.allowTypes.splice(activeZoneOptions.dynUpdate.allowTypes.indexOf(rt), 1)
                        : activeZoneOptions.dynUpdate.allowTypes.push(rt)"
                    >{{ rt }}</button>
                  </div>
                </div>
              </div>
            </transition>
          </div>
        </el-tab-pane>

      </el-tabs>
    </BaseModal>

    <BaseModal
      v-model="editDialogVisible"
      :title="t('zone.editZoneTitle')"
      :width="640"
      :loading="domainStore.submitting"
      :confirm-disabled="!isEditFormReady || domainStore.submitting"
      :confirm-text="t('zone.save')"
      :cancel-text="t('zone.cancel')"
      @confirm="submitEdit"
    >
      <el-form
        ref="editFormRef"
        :model="editForm"
        :rules="editRules"
        label-position="top"
        class="mn-modal-form"
      >
        <el-form-item :label="t('zone.zoneType')" prop="type">
          <el-select v-model="editForm.type" style="width:100%">
            <el-option v-for="item in zoneTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('zone.domainLabel')" prop="domain">
          <el-input v-model="editForm.domain" />
        </el-form-item>
        <el-form-item :label="t('zone.runStatus')">
          <el-select v-model="editForm.status" style="width:100%">
            <el-option :label="t('zone.normal')" value="正常" />
            <el-option :label="t('zone.disabled')" value="禁用" />
            <el-option :label="t('zone.abnormal')" value="异常" />
            <el-option :label="t('zone.syncing')" value="同步中" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="showEditUpstreamField" :label="t('zone.upstream')" prop="upstream">
          <el-input v-model="editForm.upstream" :placeholder="t('zone.ipv4Placeholder')" />
        </el-form-item>
        <el-form-item :label="t('zone.remark')">
          <el-input v-model="editForm.remark" type="textarea" :rows="3" :placeholder="t('zone.remarkPlaceholder')" />
        </el-form-item>
      </el-form>
    </BaseModal>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.zone-last-updated {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Typography — enterprise B-side ═══════════════ */

/* Page header */
:deep(.page-title) {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--app-title);
}
:deep(.page-subtitle) {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
}

/* Stat strip numbers */
.mn-stat-value {
  font-size: 28px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.03em;
  line-height: 1;
}
.mn-stat-label {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.01em;
  color: var(--app-text-regular);
  line-height: 1.4;
}

/* Toolbar inputs / selects */
.mn-toolbar :deep(.el-input__inner),
.mn-toolbar :deep(.el-select__placeholder),
.mn-toolbar :deep(.el-select__selected-item) {
  font-size: 13px;
}
.mn-toolbar :deep(.el-button) {
  font-size: 13px;
  font-weight: 500;
}

/* Table */
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  padding: 10px 0;
}
.mn-table :deep(.el-table__header th .cell) {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--app-text-secondary);
  padding: 0 20px;
}
.mn-table :deep(.el-table__body td) {
  padding: 12px 0;
}
.mn-table :deep(.el-table__body td .cell) {
  font-size: 13px;
  color: var(--app-text);
  line-height: 1.55;
  padding: 0 20px;
}
.mn-table :deep(.el-button.is-link) {
  font-size: 13px;
  font-weight: 500;
}

/* Domain link — slightly larger + medium weight */
.mn-domain-link {
  font-size: 13px;
  font-weight: 600;
  letter-spacing: -0.01em;
}
.mn-domain-link .mn-mono {
  font-size: 13px;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', ui-monospace, monospace;
  font-weight: 500;
  letter-spacing: 0;
}

/* Badge text */
.mn-badge {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

/* Remark / time — subdued secondary */
.mn-remark,
.mn-time {
  font-size: 12px;
  font-weight: 400;
  color: var(--app-text-regular);
}

/* Row ops link buttons */
.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.mn-row-ops :deep(.el-button.is-link) {
  font-size: 12px;
  font-weight: 500;
}

/* Pagination text */
.mn-pagination :deep(.el-pagination__total),
.mn-pagination :deep(.el-pagination__jump) {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* Modal form */
.mn-modal-form :deep(.el-form-item__label) {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-secondary);
  letter-spacing: 0.01em;
}
.mn-modal-form :deep(.el-input__inner),
.mn-modal-form :deep(.el-textarea__inner),
.mn-modal-form :deep(.el-select__placeholder),
.mn-modal-form :deep(.el-select__selected-item) {
  font-size: 13px;
}

/* ═══════════════ Stats strip ═══════════════ */
.mn-stats-strip {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
}

.mn-stat-tile {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 14px;
  padding: 14px 24px;
  border-radius: 10px;
  border: 1px solid var(--app-border);
  background: var(--app-bg);
  min-width: 120px;
  flex: 1;
  box-shadow: var(--app-shadow-soft);
}

.mn-stat-tile--success .mn-stat-value { color: var(--app-success); }
.mn-stat-tile--danger  .mn-stat-value { color: var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color: var(--app-warning); }
.mn-stat-tile--neutral .mn-stat-value { color: var(--app-text-secondary); }

/* ═══════════════ Card ═══════════════ */
.mn-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.mn-card :deep(.el-card__body) { padding: 0; }

/* ═══════════════ Toolbar ═══════════════ */
.mn-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.mn-toolbar-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}

.mn-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.mn-input-icon {
  width: 13px;
  height: 13px;
  color: var(--app-text-regular);
}

/* ═══════════════ Selection hint ═══════════════ */
.mn-selection-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 16px;
  font-size: 12px;
  color: var(--app-accent);
  background: rgba(22, 93, 255, 0.04);
  border-bottom: 1px solid var(--app-border);
}

.mn-selection-hint svg {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
}

/* ═══════════════ Table ═══════════════ */
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  white-space: nowrap;
}

.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

:deep(.domain-row-danger > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.04);
}

.mn-domain-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.mn-domain-icon {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
}

.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: center;
  padding: 10px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Badges ═══════════════ */
.mn-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 9px;
  border-radius: 20px;
  line-height: 1.6;
  white-space: nowrap;
  border: 1px solid transparent;
}

.mn-badge--neutral {
  color: var(--app-text-secondary);
  background: rgba(78, 89, 105, 0.08);
  border-color: rgba(78, 89, 105, 0.18);
}

.mn-badge--success {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

.mn-badge--danger {
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.22);
}

.mn-badge--warning {
  color: var(--app-warning);
  background: rgba(255, 125, 0, 0.08);
  border-color: rgba(255, 125, 0, 0.22);
}

/* ═══════════════ Mono / time / remark (layout only — font governed by typography block) ═══════════════ */
.mn-mono {
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', ui-monospace, monospace;
  font-variant-numeric: tabular-nums;
}

/* ═══════════════ Modal form ═══════════════ */
.mn-modal-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
}

.mn-modal-form :deep(.el-form-item) { margin-bottom: 0; }

.mn-modal-form :deep(.el-form-item.is-required .el-form-item__label::before) {
  color: var(--app-danger);
}

.mn-modal-form :deep(.el-form-item__error) {
  font-size: 12px;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1200px) {
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { justify-content: flex-start; }
  .mn-stats-strip { gap: 8px; }
}

/* ═══════════════ Zone Options Modal ═══════════════ */
.zone-opts-modal :deep(.dns-base-modal__body) { padding: 0; }

/* Tab header */
.zone-opts-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 20px;
  background: var(--dns-bg-card);
  border-bottom: 1px solid var(--dns-border-color);
}
.zone-opts-tabs :deep(.el-tabs__item) {
  font-size: 13px;
  font-weight: 500;
  color: var(--dns-text-body-color);
  height: 42px;
  padding: 0 14px;
}
.zone-opts-tabs :deep(.el-tabs__item.is-active) {
  color: var(--dns-color-primary);
  font-weight: 600;
}
.zone-opts-tabs :deep(.el-tabs__active-bar) {
  background: var(--dns-color-primary);
  height: 2px;
}
.zone-opts-tabs :deep(.el-tabs__content) { padding: 0; }

/* Tab label with SVG icon */
.zo-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.zo-tab-label svg {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  opacity: 0.7;
}

/* Scrollable pane */
.zo-pane {
  padding: 18px 20px 20px;
  max-height: 400px;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* Description banner */
.zo-desc {
  margin: 0 0 14px;
  padding: 8px 12px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--dns-text-body-color);
  background: var(--app-bg-tertiary);
  border-left: 3px solid var(--dns-color-primary);
  border-radius: 0 4px 4px 0;
}

/* Option list */
.zo-option-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.zo-option {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-radius: var(--dns-radius-base);
  border: 1px solid var(--dns-border-color);
  background: var(--dns-bg-card);
  cursor: pointer;
  transition: border-color 0.14s, background 0.14s;
  user-select: none;
}
.zo-option:hover {
  border-color: #93BBFF;
  background: #F5F8FF;
}
.zo-option.is-active {
  border-color: var(--dns-color-primary);
  background: #EEF4FF;
}
/* variant accent on active left border */
.zo-option--danger.is-active  { border-left: 3px solid var(--dns-color-danger); }
.zo-option--warning.is-active { border-left: 3px solid var(--dns-color-warning); }
.zo-option--success.is-active { border-left: 3px solid var(--dns-color-success); }
.zo-option--info.is-active    { border-left: 3px solid var(--dns-color-primary); }

/* Custom radio dot */
.zo-option-radio {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 1.5px solid var(--dns-border-color);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: border-color 0.14s;
}
.zo-option.is-active .zo-option-radio {
  border-color: var(--dns-color-primary);
}
.zo-option-radio-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: transparent;
  transition: background 0.14s;
}
.zo-option.is-active .zo-option-radio-dot {
  background: var(--dns-color-primary);
}

.zo-option-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}
.zo-option-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--dns-text-title-color);
  line-height: 1.4;
}
.zo-option.is-active .zo-option-label {
  color: var(--dns-color-primary);
}
.zo-option-sub {
  font-size: 12px;
  color: var(--dns-text-body-color);
  line-height: 1.5;
}

/* Sub panel (ACL input) */
.zo-sub-panel {
  margin-top: 10px;
  padding: 12px 14px;
  background: var(--dns-bg-page);
  border: 1px solid var(--dns-border-color);
  border-radius: var(--dns-radius-base);
}
.zo-field-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--dns-text-title-color);
  margin-bottom: 6px;
}
.zo-field-hint {
  font-weight: 400;
  color: var(--dns-text-assist-color);
  margin-left: 4px;
}
.zo-textarea :deep(.el-textarea__inner) {
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.7;
  background: var(--dns-bg-card);
}

/* Toggle row */
.zo-toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 10px;
  padding: 11px 14px;
  background: var(--dns-bg-page);
  border: 1px solid var(--dns-border-color);
  border-radius: var(--dns-radius-base);
}
.zo-toggle-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.zo-toggle-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--dns-text-title-color);
}
.zo-toggle-sub {
  font-size: 11px;
  color: var(--dns-text-assist-color);
}

/* Extra block for dyn-update */
.zo-extra {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
}

/* Record type block */
.zo-rtype-block {
  padding: 12px 14px;
  background: var(--dns-bg-page);
  border: 1px solid var(--dns-border-color);
  border-radius: var(--dns-radius-base);
}
.zo-rtype-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.zo-rtype-chip {
  display: inline-flex;
  align-items: center;
  padding: 3px 12px;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-weight: 500;
  border: 1px solid var(--dns-border-color);
  background: var(--dns-bg-card);
  color: var(--dns-text-body-color);
  cursor: pointer;
  transition: border-color 0.13s, background 0.13s, color 0.13s;
  outline: none;
}
.zo-rtype-chip:hover {
  border-color: var(--dns-color-primary);
  color: var(--dns-color-primary);
}
.zo-rtype-chip.is-on {
  border-color: var(--dns-color-primary);
  background: #EEF4FF;
  color: var(--dns-color-primary);
  font-weight: 700;
}

/* Slide transition */
.zo-slide-enter-active { transition: opacity 0.18s ease, max-height 0.22s ease; max-height: 400px; overflow: hidden; }
.zo-slide-leave-active { transition: opacity 0.15s ease, max-height 0.18s ease; max-height: 400px; overflow: hidden; }
.zo-slide-enter-from, .zo-slide-leave-to { opacity: 0; max-height: 0; }
</style>




