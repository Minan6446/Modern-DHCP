<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, resolveComponent, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useDomainStore } from '../../stores/domain'
import type { FormInstance, FormRules } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import TableColumnManager from '../../components/TableColumnManager.vue'
import RecordImportDialog from '../../components/RecordImportDialog.vue'
import { useZoneEditTable } from '../../composables/dns/useZoneEditTable'
import { useZoneRecord } from '../../composables/dns/useZoneRecord'
import { downloadRecordTemplateApi } from '../../api/zone'
import { useZoneHistory } from '../../composables/dns/useZoneHistory'
import { useZoneHealthChecker } from '../../composables/dns/useZoneHealthChecker'
import type { DomainRecord, DomainSoa } from '../../types/modules'
import type { ImportRow, ConflictItem } from '../../components/RecordImportDialog.vue'
import { loadTableState, saveTableState } from '../../utils/tableState'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const domainStore = useDomainStore()

const zoneId = computed(() => route.params.id)
const zone = computed(() => domainStore.getZoneById(zoneId.value))
const detail = computed(() => domainStore.getDetailById(zoneId.value))
const activeTab = ref('records')
const soaFormRef = ref<FormInstance>()
const rotatingKey = ref<'ksk' | 'zsk' | null>(null)
const dnssecSwitchLoading = ref(false)
const tableStateKey = 'modern-dns:zone-edit:record-table-state'

/* ── useZoneRecord composable ── */
const zoneRecord = useZoneRecord(() => zoneId.value)
const {
  dialogVisible: recordDialogVisible,
  formRef: recordFormRef,
  form: recordForm,
  filter: recordFilter,
  density: tableDensity,
  deletingId: recordDeletingId,
  batchDeleting: recordBatchDeleting,
  statusSwitchingId: recordStatusSwitchingId,
  ttlAdjustingId,
  importing: recordImporting,
  importProgress: recordImportProgress,
  filteredRecords,
  recordValueHint,
  rules: recordRules,
  TTL_PRESETS,
  RECORD_TYPES,
  openDialog: openRecordDialog,
  closeDialog: closeRecordDialog,
  submitRecord,
  clearForm: clearRecordForm,
  deleteRecord,
  batchDelete: batchDeleteRecords,
  adjustTtl: adjustRecordTtl,
  setTtlPreset,
  toggleStatus: toggleRecordStatus,
  resetFilter: resetRecordFilter,
  exportRecordsCsv,
  importRecordsCsv,
  allRecords: allRecordRows,
} = zoneRecord

/* ── table state & virtual mode ── */
const recordColumnOptions = computed(() => [
  { prop: 'type', label: t('record.type'), visible: true },
  { prop: 'host', label: t('record.host'), visible: true },
  { prop: 'value', label: t('record.value'), visible: true },
  { prop: 'ttl', label: t('record.ttl'), visible: true },
  { prop: 'ttlQuick', label: t('record.ttlQuick'), visible: true },
  { prop: 'status', label: t('record.status'), visible: true },
  { prop: 'lastUsedAt', label: t('record.lastUsedAt'), visible: true },
  { prop: 'operate', label: t('record.operation'), visible: true },
] as const)

const visibleRecordColumns = ref<string[]>(recordColumnOptions.value.filter((c) => c.visible).map((c) => c.prop))
const visibleRecordColumnSet = computed(() => new Set(visibleRecordColumns.value))
const isRecordColumnVisible = (prop: string): boolean => visibleRecordColumnSet.value.has(prop)

const cachedState = loadTableState<{ activeTab: string; recordPager: { page: number; size: number } }>(tableStateKey, {
  activeTab: 'records',
  recordPager: { page: 1, size: 10 },
})

const {
  selectedRows: selectedRecordRows,
  selectedRowIds: selectedRecordIds,
  tableData: recordTableData,
  pagination: recordTablePagination,
  isVirtualMode: useVirtualTable,
  handleSelectionChange,
  handleVirtualSelectionChange,
  handlePageChange,
  handleSortChange,
} = useZoneEditTable<DomainRecord>({
  rows: filteredRecords,
  initialPage: cachedState.recordPager.page,
  initialPageSize: cachedState.recordPager.size,
})

activeTab.value = cachedState.activeTab || 'records'

/* ── virtual table width tracking ── */
const virtualWrapRef = ref<HTMLElement | null>(null)
const virtualTableWidth = ref(900)
let _virtualResizeObserver: ResizeObserver | null = null
const tableV2Key = ref(0)
let _tableV2Initialized = false
watch(recordTableData, (rows) => {
  if (rows.length && !_tableV2Initialized) {
    _tableV2Initialized = true
    tableV2Key.value++
  }
  if (!rows.length) _tableV2Initialized = false
})

watch(
  () => ({ activeTab: activeTab.value, recordPager: { page: recordTablePagination.currentPage, size: recordTablePagination.pageSize } }),
  (next) => saveTableState(tableStateKey, next),
  { deep: true },
)

watch(() => recordForm.type, () => recordFormRef.value?.validateField('value'))

/* ── useZoneHistory ── */
const zoneHistory = useZoneHistory(() => zoneId.value)
const { history: recordHistory, canUndo, canRedo, snapshot: takeSnapshot, undo: undoRecord, redo: redoRecord } = zoneHistory

/* ── useZoneHealthChecker ── */
const healthChecker = useZoneHealthChecker(() => zoneId.value)
const { health, checking: healthChecking, healthBadgeClass, healthLabel, check: runHealthCheck } = healthChecker

/* ── history drawer ── */
const historyDrawerVisible = ref(false)

/* ── import dialog (conflict detection) ── */
const importDialogVisible = ref(false)
const handleImportClick = () => { importDialogVisible.value = true }
const handleImportConfirm = async (rows: ImportRow[], conflicts: ConflictItem[]) => {
  const toSave = [
    ...rows.filter((r) => !conflicts.some((c) => c.incoming === r)),
    ...conflicts.filter((c) => c.action === 'overwrite').map((c) => c.incoming),
  ]
  takeSnapshot(t('record.importBackup'))
  recordImporting.value = true
  recordImportProgress.value = 0
  let done = 0
  for (const r of toSave) {
    await domainStore.saveRecord(zoneId.value, r as any)
    done++
    recordImportProgress.value = Math.round((done / toSave.length) * 100)
  }
  recordImporting.value = false
  ElMessage.success(t('record.importedCount', { count: toSave.length }))
}

/* ── batch edit ── */
const batchEditVisible = ref(false)
const batchEditForm = reactive({ ttl: 600, status: '' as '' | '启用' | '禁用' })
const batchEditSubmitting = ref(false)
const openBatchEdit = () => {
  if (!selectedRecordRows.value.length) { ElMessage.warning(t('record.selectRecordFirst')); return }
  Object.assign(batchEditForm, { ttl: 600, status: '' })
  batchEditVisible.value = true
}
const submitBatchEdit = async () => {
  batchEditSubmitting.value = true
  takeSnapshot(t('record.batchEditBackup'))
  try {
    for (const row of selectedRecordRows.value) {
      const updated = { ...row } as DomainRecord & { status: '启用' | '禁用' }
      if (batchEditForm.ttl > 0) updated.ttl = batchEditForm.ttl
      if (batchEditForm.status) updated.status = batchEditForm.status
      await domainStore.saveRecord(zoneId.value, updated)
    }
    ElMessage.success(t('record.batchUpdated', { count: selectedRecordRows.value.length }))
    batchEditVisible.value = false
  } finally {
    batchEditSubmitting.value = false
  }
}


/* ── SOA ── */
const soaForm = reactive<DomainSoa>({ mname: '', rname: '', refresh: 3600, retry: 600, expire: 1209600, minimumTtl: 300 })

const soaSerial = computed(() => zone.value?.serial || '—')

const soaRules = computed<FormRules>(() => ({
  mname: [{ required: true, message: t('record.valMnameRequired'), trigger: 'blur' }],
  rname: [{ required: true, message: t('record.valRnameRequired'), trigger: 'blur' }],
}))

const soaTips = computed(() => ({
  mname: t('record.soaMnameTip'),
  rname: t('record.soaRnameTip'),
  refresh: t('record.soaRefreshTip'),
  retry: t('record.soaRetryTip'),
  expire: t('record.soaExpireTip'),
  minimumTtl: t('record.soaMinTtlTip'),
}))

const termTips = computed(() => ({
  ttl: t('record.ttlTip'),
  ksk: t('record.kskTip'),
  zsk: t('record.zskTip'),
}))

const dnssecKeyStatusLabel = (value: string): string => (value === '生效中' ? t('record.activeNow') : value)

const syncSoaForm = () => Object.assign(soaForm, detail.value.soa)

const saveSoa = async () => {
  if (domainStore.submitting) return
  const valid = await validateFormAndFocus(soaFormRef.value)
  if (!valid) return
  await domainStore.saveSoa(zoneId.value, soaForm)
  ElMessage.success(t('record.soaSaved'))
}

const resetSoa = async () => {
  await confirmRiskAction({ title: t('record.soaResetConfirmTitle'), action: t('record.soaResetAction'), risk: t('record.soaResetRisk') })
  syncSoaForm()
  ElMessage.success(t('record.soaReset'))
}

/* ── DNSSEC ── */
const toggleDnssec = async (value: boolean) => {
  if (dnssecSwitchLoading.value) return
  dnssecSwitchLoading.value = true
  try {
    await confirmRiskAction({ title: t('record.dnssecConfirmTitle'), action: value ? t('record.dnssecEnableAction') : t('record.dnssecDisableAction'), risk: t('record.dnssecChangeRisk') })
    domainStore.updateDnssec(zoneId.value, value)
    ElMessage.success(value ? t('record.dnssecEnabled') : t('record.dnssecDisabled'))
  } finally {
    dnssecSwitchLoading.value = false
  }
}

const rotateKey = async (keyType: 'ksk' | 'zsk') => {
  if (rotatingKey.value) return
  await confirmRiskAction({ title: t('record.rotateConfirmTitle'), action: t('record.rotateAction', { key: keyType.toUpperCase() }), risk: t('record.rotateRisk') })
  rotatingKey.value = keyType
  try {
    domainStore.rotateDnssecKey(zoneId.value, keyType)
    await domainStore.fetchDomainData()
    syncSoaForm()
    ElMessage.success(t('record.rotateDone', { key: keyType.toUpperCase() }))
  } finally {
    rotatingKey.value = null
  }
}

/* ── SOA human-readable formatter ── */
const formatSeconds = (s: number): string => {
  if (s >= 86400 && s % 86400 === 0) return `${s / 86400} ${t('record.day')}`
  if (s >= 3600 && s % 3600 === 0) return `${s / 3600} ${t('record.hour')}`
  if (s >= 60 && s % 60 === 0) return `${s / 60} ${t('record.minute')}`
  return `${s} ${t('record.second')}`
}

/* ── DNSSEC trust chain status ── */
type ChainStatus = 'ok' | 'warn' | 'off'
const dnssecChainStatus = computed((): ChainStatus => {
  if (!detail.value.dnssec.enabled) return 'off'
  const hasKsk = detail.value.dnssec.ksk.some((k) => k.status === '生效中')
  const hasZsk = detail.value.dnssec.zsk.some((k) => k.status === '生效中')
  return hasKsk && hasZsk ? 'ok' : 'warn'
})

/* ── Record type color map ── */
const RECORD_TYPE_COLORS: Record<string, string> = {
  A: '#3b82f6', AAAA: '#8b5cf6', CNAME: '#06b6d4', TXT: '#f59e0b',
  MX: '#10b981', NS: '#6366f1', SRV: '#f97316',
}
const recordTypeBg = (rt: string) => RECORD_TYPE_COLORS[rt] ?? '#6b7280'

const RECORD_TYPE_DESC = computed<Record<string, string>>(() => ({
  A: t('record.ipv4Desc'), AAAA: t('record.ipv6Desc'), CNAME: t('record.cnameDesc'),
  TXT: t('record.txtDesc'), MX: t('record.mxDesc'), NS: t('record.nsDesc'), SRV: t('record.srvDesc'),
}))

const recordValuePlaceholder = computed(() => {
  const map: Record<string, string> = {
    A: 'e.g. 192.168.1.1',
    AAAA: 'e.g. 2001:db8::1',
    CNAME: 'e.g. target.example.com.',
    MX: 'e.g. mail.example.com.',
    NS: 'e.g. ns1.example.com.',
    TXT: 'e.g. v=spf1 include:_spf.example.com ~all',
    SRV: 'e.g. 10 20 443 target.example.com.',
  }
  return map[recordForm.type] ?? t('record.defaultValuePlaceholder')
})

/* DS records from backend (real DNSSEC DS RR text) */
const dsRecords = computed(() =>
  detail.value.dnssec.ksk
    .filter((k) => k.ds)
    .map((k) => ({
      keyId: k.keyId,
      keyTag: k.keyTag,
      algorithm: k.algorithm,
      dsText: k.ds,
    })),
)

/* ── virtual table columns ── */
const virtualColumns = computed(() => [
  {
    key: 'select',
    title: '',
    width: 56,
    cellRenderer: ({ rowData }: { rowData: DomainRecord }) =>
      h('div', { class: 'virtual-cell-center' }, [
        h(resolveComponent('ElCheckbox') as any, {
          modelValue: selectedRecordIds.value.includes(rowData.id),
          onChange: (checked: boolean) => handleVirtualSelectionChange(rowData.id, checked),
        }),
      ]),
  },
  ...(isRecordColumnVisible('type') ? [{ key: 'type', dataKey: 'type', title: t('record.type'), width: 120 }] : []),
  ...(isRecordColumnVisible('host') ? [{ key: 'host', dataKey: 'host', title: t('record.host'), width: 180 }] : []),
  ...(isRecordColumnVisible('value') ? [{ key: 'value', dataKey: 'value', title: t('record.value'), width: 280 }] : []),
  ...(isRecordColumnVisible('ttl')
    ? [{ key: 'ttl', title: t('record.ttl'), width: 170, cellRenderer: ({ rowData }: { rowData: DomainRecord }) => h('div', { class: 'virtual-ttl-ops' }, [h('span', `${rowData.ttl}`)]) }]
    : []),
  ...(isRecordColumnVisible('ttlQuick')
    ? [{
        key: 'ttlQuick',
        title: t('record.ttlQuick'),
        width: 170,
        cellRenderer: ({ rowData }: { rowData: DomainRecord }) =>
          h('div', { class: 'virtual-ttl-ops' }, [
            h(resolveComponent('ElButton') as any, { size: 'small', link: true, type: 'primary', loading: ttlAdjustingId.value === rowData.id, disabled: Boolean(ttlAdjustingId.value), onClick: () => adjustRecordTtl(rowData, 60) }, () => '+60'),
            h(resolveComponent('ElButton') as any, { size: 'small', link: true, type: 'warning', loading: ttlAdjustingId.value === rowData.id, disabled: Boolean(ttlAdjustingId.value), onClick: () => adjustRecordTtl(rowData, -60) }, () => '-60'),
          ]),
      }]
    : []),
  ...(isRecordColumnVisible('status')
    ? [{
        key: 'status',
        title: t('record.status'),
        width: 150,
        cellRenderer: ({ rowData }: { rowData: DomainRecord }) =>
          h(resolveComponent('ElSwitch') as any, {
            modelValue: rowData.status === '启用',
            loading: recordStatusSwitchingId.value === rowData.id,
            disabled: Boolean(recordStatusSwitchingId.value) || Boolean(recordDeletingId.value) || recordBatchDeleting.value,
            inlinePrompt: true, activeText: t('record.enabled'), inactiveText: t('record.disabled'),
            onChange: (value: boolean) => toggleRecordStatus(rowData, value),
          }),
      }]
    : []),
  ...(isRecordColumnVisible('updatedAt') ? [{ key: 'updatedAt', dataKey: 'updatedAt', title: t('record.operateTime'), width: 190 }] : []),
  ...(isRecordColumnVisible('operate')
    ? [{
        key: 'operate',
        width: 200,
        cellRenderer: ({ rowData }: { rowData: DomainRecord }) =>
          h('div', { class: 'table-operations' }, [
            h(resolveComponent('ElButton') as any, { link: true, type: 'primary', onClick: () => openRecordDialog(rowData) }, () => t('record.edit')),
            h(resolveComponent('ElButton') as any, { link: true, type: 'danger', loading: recordDeletingId.value === rowData.id, disabled: Boolean(recordDeletingId.value) || recordBatchDeleting.value, onClick: () => deleteRecord(rowData) }, () => t('record.delete')),
          ]),
      }]
    : []),
])

const handleRecordCommand = async (row: DomainRecord, command: 'edit' | 'enable' | 'disable' | 'delete') => {
  if (command === 'edit') { openRecordDialog(row); return }
  if (command === 'delete') { await deleteRecord(row); return }
  toggleRecordStatus(row, command === 'enable')
}

// keep-alive in AppLayout reuses this component instance across different
// :id values (the cache key is route.name), so onMounted only fires the
// first time. Watch zoneId so navigating zone-edit/X → zone-edit/Y still
// pulls a fresh detail for Y instead of rendering Y with X's cache (or an
// EMPTY_DETAIL fallback when Y was never fetched).
watch(zoneId, async (id) => {
  if (id == null || id === '') return
  await domainStore.fetchDetail(id)
  syncSoaForm()
})

onMounted(async () => {
  if (!domainStore.zones.length) await domainStore.fetchDomainData()
  await domainStore.fetchDetail(zoneId.value)
  domainStore.startPolling()
  syncSoaForm()

  _virtualResizeObserver = new ResizeObserver((entries) => {
    const w = entries[0]?.contentRect.width
    if (w > 0) virtualTableWidth.value = Math.floor(w)
  })
  watch(virtualWrapRef, (el) => {
    _virtualResizeObserver?.disconnect()
    if (el) {
      _virtualResizeObserver?.observe(el)
      virtualTableWidth.value = Math.floor(el.getBoundingClientRect().width) || 900
    }
  }, { immediate: true })
})

onBeforeUnmount(() => {
  domainStore.stopPolling()
  _virtualResizeObserver?.disconnect()
})
</script>

<template>
  <div class="page-shell" v-if="zone">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('record.title') }}</h1>
        <p class="page-subtitle">{{ zone.domain }} / {{ zone.zoneId }}</p>
      </div>
      <div class="page-actions zone-page-actions">
        <el-tooltip :content="`${t('record.healthCheck')}: ${health.checkedAt ? new Date(health.checkedAt).toLocaleTimeString() : t('record.notDetected')}`" placement="bottom">
          <span :class="['mn-badge', healthBadgeClass]" style="cursor:pointer" @click="runHealthCheck">
            <svg style="width:11px;height:11px;margin-right:3px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
            {{ healthLabel }}
          </span>
        </el-tooltip>
        <el-button class="zone-return-btn" @click="router.push('/domain/zone-list')">{{ t('record.returnZoneList') }}</el-button>
      </div>
    </div>

    <el-card class="content-card">
      <el-tabs v-model="activeTab" class="zone-edit-tabs">
        <el-tab-pane :label="t('record.recordsTab')" name="records">
          <div class="mn-toolbar">
            <div class="mn-toolbar-filters">
              <el-input v-model="recordFilter.keyword" clearable :placeholder="t('record.searchHostValue')" style="width:200px">
                <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
              </el-input>
              <el-select v-model="recordFilter.type" clearable :placeholder="t('record.filterType')" style="width:110px">
                <el-option v-for="rt in RECORD_TYPES" :key="rt" :label="rt" :value="rt" />
              </el-select>
              <el-select v-model="recordFilter.status" clearable :placeholder="t('record.filterStatus')" style="width:90px">
                <el-option :label="t('record.enabled')" value="启用" />
                <el-option :label="t('record.disabled')" value="禁用" />
              </el-select>
              <el-button v-if="recordFilter.keyword || recordFilter.type || recordFilter.status" link @click="resetRecordFilter">{{ t('record.clearFilter') }}</el-button>
              <span class="mn-record-count" v-if="filteredRecords.length !== allRecordRows.length">{{ t('record.matched') }} <strong>{{ filteredRecords.length }}</strong> / {{ allRecordRows.length }}</span>
              <span class="mn-record-count" v-else-if="filteredRecords.length">{{ t('record.total') }} <strong>{{ filteredRecords.length }}</strong> {{ t('record.recordUnit') }}</span>
            </div>
            <div class="mn-toolbar-actions">
              <!-- density toggle -->
              <el-tooltip :content="t('record.tableDensity')" placement="top">
                <div class="mn-density-group">
                  <button class="mn-density-btn" :class="{ active: tableDensity === 'compact' }" @click="tableDensity = 'compact'" :title="t('record.compact')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/><line x1="3" y1="14" x2="21" y2="14"/><line x1="3" y1="18" x2="21" y2="18"/></svg>
                  </button>
                  <button class="mn-density-btn" :class="{ active: tableDensity === 'default' }" @click="tableDensity = 'default'" :title="t('record.default')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg>
                  </button>
                  <button class="mn-density-btn" :class="{ active: tableDensity === 'relaxed' }" @click="tableDensity = 'relaxed'" :title="t('record.relaxed')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="3" y1="5" x2="21" y2="5"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="19" x2="21" y2="19"/></svg>
                  </button>
                </div>
              </el-tooltip>
              <TableColumnManager v-model:visible-columns="visibleRecordColumns" :columns="recordColumnOptions" storage-key="zone-edit-record-columns" />
              <el-button :loading="recordBatchDeleting" :disabled="recordBatchDeleting || !selectedRecordRows.length" @click="batchDeleteRecords(selectedRecordRows)">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/></svg></template>
                {{ t('record.batchDelete') }}
              </el-button>
              <el-button :disabled="!canUndo" @click="undoRecord">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 14 4 9 9 4"/><path d="M20 20v-7a4 4 0 0 0-4-4H4"/></svg></template>
              </el-button>
              <el-button :disabled="!canRedo" @click="redoRecord">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 14 20 9 15 4"/><path d="M4 20v-7a4 4 0 0 1 4-4h12"/></svg></template>
              </el-button>
              <el-button :disabled="!selectedRecordRows.length" @click="openBatchEdit">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg></template>
                {{ t('record.batchEdit') }}
              </el-button>
              <el-button @click="downloadRecordTemplateApi">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><polyline points="9 15 12 18 15 15"/></svg></template>
                {{ t('record.importTemplate') }}
              </el-button>
              <el-button @click="handleImportClick">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg></template>
                {{ t('record.importCsv') }}
              </el-button>
              <el-button @click="exportRecordsCsv(filteredRecords)">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
                {{ t('record.exportCsv') }}
              </el-button>
              <el-button @click="historyDrawerVisible = true" :title="t('record.historyTitle')">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg></template>
              </el-button>
              <el-button type="primary" @click="openRecordDialog()">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
                {{ t('record.addNewRecord') }}
              </el-button>
            </div>
          </div>

          <template v-if="detail.records.length">
          <div v-if="useVirtualTable" ref="virtualWrapRef" class="virtual-table-wrap">
            <el-table-v2
              :key="tableV2Key"
              :columns="virtualColumns"
              :data="recordTableData"
              :width="virtualTableWidth"
              :height="560"
              fixed
              :row-height="48"
              :header-height="44"
            />
          </div>

          <el-table v-else :data="recordTableData" class="mn-table" :class="`mn-table--${tableDensity}`" @selection-change="handleSelectionChange" @sort-change="handleSortChange">
            <el-table-column type="selection" width="48" />
            <el-table-column v-if="isRecordColumnVisible('type')" prop="type" :label="t('record.type')" width="110" sortable>
              <template #default="{ row }">
                <span class="mn-type-badge" :style="{ background: recordTypeBg(row.type) }">{{ row.type }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('host')" prop="host" :label="t('record.host')" min-width="150" sortable>
              <template #default="{ row }"><span class="mn-mono">{{ row.host }}</span></template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('value')" prop="value" :label="t('record.value')" min-width="220" show-overflow-tooltip>
              <template #default="{ row }"><span class="mn-mono">{{ row.value }}</span></template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('ttl')" prop="ttl" width="90" sortable>
              <template #header>
                <div class="mn-th-with-tip">
                  <span>{{ t('record.ttl') }}</span>
                  <el-tooltip :content="termTips.ttl" placement="top">
                    <el-icon class="mn-help-icon"><InfoFilled /></el-icon>
                  </el-tooltip>
                </div>
              </template>
              <template #default="{ row }"><span class="mn-mono">{{ row.ttl }}</span></template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('ttlQuick')" :label="t('record.ttlQuick')" width="180">
              <template #default="{ row }">
                <div class="mn-ttl-ops">
                  <el-button link type="primary" size="small" :loading="ttlAdjustingId === row.id" :disabled="Boolean(ttlAdjustingId)" @click="adjustRecordTtl(row, 60)">+60</el-button>
                  <el-button link type="warning" size="small" :loading="ttlAdjustingId === row.id" :disabled="Boolean(ttlAdjustingId)" @click="adjustRecordTtl(row, -60)">-60</el-button>
                  <el-dropdown size="small" :disabled="Boolean(ttlAdjustingId)" @command="(v: number) => setTtlPreset(row, v)">
                    <el-button link size="small">{{ t('record.preset') }}<el-icon class="el-icon--right"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="width:10px;height:10px"><polyline points="6 9 12 15 18 9"/></svg></el-icon></el-button>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item v-for="p in TTL_PRESETS" :key="p.value" :command="p.value">{{ p.label }}</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </div>
              </template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('status')" :label="t('record.status')" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="row.status === '启用' ? 'success' : 'info'" size="small" disable-transitions>{{ row.status === '启用' ? t('record.enabled') : t('record.disabled') }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('lastUsedAt')" prop="lastUsedAt" :label="t('record.lastUsedAt')" min-width="170" sortable>
              <template #default="{ row }"><span class="mn-time">{{ row.lastUsedAt || '—' }}</span></template>
            </el-table-column>
            <el-table-column v-if="isRecordColumnVisible('operate')" :label="t('record.operation')" width="200" fixed="right" align="center">
              <template #default="{ row }">
                <div class="mn-row-ops">
                  <el-button link type="primary" size="small" @click="openRecordDialog(row)">{{ t('record.edit') }}</el-button>
                  <el-button link :type="row.status === '启用' ? 'warning' : 'success'" size="small" :loading="recordStatusSwitchingId === row.id" @click="toggleRecordStatus(row, row.status !== '启用')">{{ row.status === '启用' ? t('record.disabled') : t('record.enabled') }}</el-button>
                  <el-button link type="danger" size="small" :loading="recordDeletingId === row.id" :disabled="Boolean(recordDeletingId) || recordBatchDeleting" @click="deleteRecord(row)">{{ t('record.delete') }}</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>

          <div v-if="!useVirtualTable" class="mn-pagination">
            <el-pagination
              :current-page="recordTablePagination.currentPage"
              :page-size="recordTablePagination.pageSize"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              :total="filteredRecords.length"
              size="small"
              background
              @current-change="(page) => handlePageChange(page)"
              @size-change="(size) => handlePageChange(1, size)"
            />
          </div>
          </template>
          <el-empty v-else :description="t('record.noRecords')">
            <el-button type="primary" @click="openRecordDialog()">{{ t('record.goAdd') }}</el-button>
          </el-empty>
        </el-tab-pane>


        <el-tab-pane :label="t('record.soaTab')" name="soa">
          <div class="mn-panel-header">
            <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/><path d="M4.93 4.93a10 10 0 0 0 0 14.14"/></svg>
            <span>{{ t('record.soaTitle') }}</span>
            <div class="mn-soa-serial-chip">
              <span class="mn-soa-serial-label">Serial</span>
              <span class="mn-mono mn-soa-serial-value">{{ soaSerial }}</span>
            </div>
          </div>
          <!-- SOA human-readable summary chips -->
          <div class="soa-readable-bar">
            <div class="soa-readable-chip">
              <span class="soa-readable-label">Refresh</span>
              <span class="soa-readable-value">{{ formatSeconds(soaForm.refresh) }}</span>
            </div>
            <div class="soa-readable-chip">
              <span class="soa-readable-label">Retry</span>
              <span class="soa-readable-value">{{ formatSeconds(soaForm.retry) }}</span>
            </div>
            <div class="soa-readable-chip">
              <span class="soa-readable-label">Expire</span>
              <span class="soa-readable-value">{{ formatSeconds(soaForm.expire) }}</span>
            </div>
            <div class="soa-readable-chip">
              <span class="soa-readable-label">Min TTL</span>
              <span class="soa-readable-value">{{ formatSeconds(soaForm.minimumTtl) }}</span>
            </div>
          </div>

          <div class="soa-panel">
            <el-form ref="soaFormRef" :model="soaForm" :rules="soaRules" label-position="top" class="soa-form-grid">
              <el-form-item prop="mname" class="soa-col-6">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaMname') }}</span>
                    <el-tooltip :content="soaTips.mname" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <el-input v-model="soaForm.mname" placeholder="ns1.example.com." />
              </el-form-item>
              <el-form-item prop="rname" class="soa-col-6">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaRname') }}</span>
                    <el-tooltip :content="soaTips.rname" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <el-input v-model="soaForm.rname" placeholder="admin.example.com." />
              </el-form-item>
              <el-form-item class="soa-col-3">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaRefresh') }}</span>
                    <el-tooltip :content="soaTips.refresh" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <div class="soa-num-wrap">
                  <el-input-number v-model="soaForm.refresh" :min="60" :max="86400" :step="60" style="width:100%" controls-position="right" />
                  <span class="soa-unit">{{ t('record.second') }}</span>
                </div>
              </el-form-item>
              <el-form-item class="soa-col-3">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaRetry') }}</span>
                    <el-tooltip :content="soaTips.retry" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <div class="soa-num-wrap">
                  <el-input-number v-model="soaForm.retry" :min="60" :max="7200" :step="60" style="width:100%" controls-position="right" />
                  <span class="soa-unit">{{ t('record.second') }}</span>
                </div>
              </el-form-item>
              <el-form-item class="soa-col-3">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaExpire') }}</span>
                    <el-tooltip :content="soaTips.expire" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <div class="soa-num-wrap">
                  <el-input-number v-model="soaForm.expire" :min="86400" :max="2419200" :step="600" style="width:100%" controls-position="right" />
                  <span class="soa-unit">{{ t('record.second') }}</span>
                </div>
              </el-form-item>
              <el-form-item class="soa-col-3">
                <template #label>
                  <div class="mn-form-label-with-tip">
                    <span>{{ t('record.soaMinTtl') }}</span>
                    <el-tooltip :content="soaTips.minimumTtl" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                  </div>
                </template>
                <div class="soa-num-wrap">
                  <el-input-number v-model="soaForm.minimumTtl" :min="60" :max="86400" :step="60" style="width:100%" controls-position="right" />
                  <span class="soa-unit">{{ t('record.second') }}</span>
                </div>
              </el-form-item>
            </el-form>
          </div>
          <div class="soa-actions">
            <el-button type="primary" size="default" :loading="domainStore.submitting" :disabled="domainStore.submitting" @click="saveSoa">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg></template>
              {{ t('record.saveConfig') }}
            </el-button>
            <el-button size="default" :disabled="domainStore.submitting" @click="resetSoa">{{ t('record.reset') }}</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('record.dnssecTab')" name="dnssec">
          <div class="dnssec-panel">
            <div class="mn-panel-header">
              <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
              <span>{{ t('record.dnssecTitle') }}</span>
              <!-- Trust chain indicator -->
              <div class="dnssec-chain">
                <div class="dnssec-chain-step" :class="`chain-${dnssecChainStatus}`">
                  <span class="chain-node">KSK</span>
                  <svg class="chain-arrow" viewBox="0 0 24 6" fill="none"><path d="M0 3 H20 M17 0.5 L20.5 3 L17 5.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  <span class="chain-node">ZSK</span>
                  <svg class="chain-arrow" viewBox="0 0 24 6" fill="none"><path d="M0 3 H20 M17 0.5 L20.5 3 L17 5.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  <span class="chain-node">RR</span>
                </div>
                <span class="chain-label">{{ dnssecChainStatus === 'ok' ? t('record.chainComplete') : dnssecChainStatus === 'warn' ? t('record.chainIncomplete') : t('record.chainNotEnabled') }}</span>
              </div>
              <div style="margin-left:auto;display:flex;align-items:center;gap:8px;">
                <span style="font-size:12px;font-weight:400;color:var(--app-text-regular)">{{ t('record.dnssecSwitch') }}</span>
                <el-switch class="dnssec-switch" :model-value="detail.dnssec.enabled" :loading="dnssecSwitchLoading" :disabled="dnssecSwitchLoading || Boolean(rotatingKey)" inline-prompt :active-text="t('record.dnssecOn')" :inactive-text="t('record.dnssecOff')" @change="toggleDnssec" />
              </div>
            </div>
            <div class="dnssec-key-grid">
              <el-card class="dnssec-key-card">
                <template #header>
                  <div class="dnssec-key-header">
                    <div style="display:flex;align-items:center;gap:6px">
                      <span>{{ t('record.kskKey') }}</span>
                      <el-tooltip :content="termTips.ksk" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                    </div>
                    <el-button class="dnssec-rotate-btn" type="primary" plain size="small" :loading="rotatingKey === 'ksk'" :disabled="Boolean(rotatingKey)" @click="rotateKey('ksk')">{{ t('record.rotateKsk') }}</el-button>
                  </div>
                </template>
                <div v-if="!detail.dnssec.ksk.length" class="dnssec-empty">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                  <span>{{ t('record.autoGenerate') }}</span>
                </div>
                <div v-else class="dnssec-key-list">
                  <div v-for="item in detail.dnssec.ksk" :key="item.id" class="dnssec-key-item">
                    <div class="dnssec-key-row">
                      <span class="section-title">{{ item.keyId }}</span>
                      <span :class="['mn-badge', item.status === '生效中' ? 'mn-badge--success' : 'mn-badge--warning']">{{ dnssecKeyStatusLabel(item.status) }}</span>
                    </div>
                    <div class="stat-label">{{ t('record.algorithm') }}: {{ item.algorithm }} &nbsp;·&nbsp; {{ t('record.created') }}: {{ item.createdAt }}</div>
                  </div>
                </div>
              </el-card>

              <el-card class="dnssec-key-card">
                <template #header>
                  <div class="dnssec-key-header">
                    <div style="display:flex;align-items:center;gap:6px">
                      <span>{{ t('record.zskKey') }}</span>
                      <el-tooltip :content="termTips.zsk" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                    </div>
                    <el-button class="dnssec-rotate-btn" type="primary" plain size="small" :loading="rotatingKey === 'zsk'" :disabled="Boolean(rotatingKey)" @click="rotateKey('zsk')">{{ t('record.rotateZsk') }}</el-button>
                  </div>
                </template>
                <div v-if="!detail.dnssec.zsk.length" class="dnssec-empty">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                  <span>{{ t('record.autoGenerate') }}</span>
                </div>
                <div v-else class="dnssec-key-list">
                  <div v-for="item in detail.dnssec.zsk" :key="item.id" class="dnssec-key-item">
                    <div class="dnssec-key-row">
                      <span class="section-title">{{ item.keyId }}</span>
                      <span :class="['mn-badge', item.status === '生效中' ? 'mn-badge--success' : 'mn-badge--warning']">{{ dnssecKeyStatusLabel(item.status) }}</span>
                    </div>
                    <div class="stat-label">{{ t('record.algorithm') }}: {{ item.algorithm }} &nbsp;·&nbsp; {{ t('record.created') }}: {{ item.createdAt }}</div>
                  </div>
                </div>
              </el-card>

              <!-- DS Records -->
              <el-card v-if="dsRecords.length" class="dnssec-key-card dnssec-ds-card">
                <template #header>
                  <div class="dnssec-key-header">
                    <div style="display:flex;align-items:center;gap:6px">
                      <span>{{ t('record.dsRecord') }}</span>
                      <el-tooltip :content="t('record.dsTip')" placement="top"><el-icon class="mn-help-icon"><InfoFilled /></el-icon></el-tooltip>
                    </div>
                  </div>
                </template>
                <div class="dnssec-key-list">
                  <div v-for="ds in dsRecords" :key="ds.keyId" class="dnssec-key-item">
                    <div class="dnssec-key-row">
                      <span class="section-title">{{ ds.keyId }}</span>
                      <span class="mn-badge mn-badge--neutral">Key Tag {{ ds.keyTag }}</span>
                    </div>
                    <div class="stat-label">{{ t('record.algorithm') }}: {{ ds.algorithm }}</div>
                    <div class="ds-digest-wrap">
                      <span class="ds-digest-label">DS RR</span>
                      <span class="mn-mono ds-digest-value">{{ ds.dsText }}</span>
                    </div>
                  </div>
                </div>
              </el-card>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog
      v-model="recordDialogVisible"
      :title="recordForm.id ? t('record.editRecordTitle') : t('record.addRecordTitle')"
      width="560px"
      destroy-on-close
      @closed="closeRecordDialog"
    >
      <el-form ref="recordFormRef" :model="recordForm" :rules="recordRules" label-position="top" class="mn-modal-form">
        <div class="record-dialog-grid">

          <!-- Row 1: type + host -->
          <el-form-item prop="type">
            <template #label><span class="rdf-label">{{ t('record.type') }}</span></template>
            <el-select v-model="recordForm.type" style="width:100%">
              <el-option v-for="rt in RECORD_TYPES" :key="rt" :label="rt" :value="rt">
                <div class="rdf-type-option">
                  <span class="rdf-type-dot" :style="{ background: recordTypeBg(rt) }"></span>
                  <span>{{ rt }}</span>
                  <span class="rdf-type-desc">{{ RECORD_TYPE_DESC[rt] }}</span>
                </div>
              </el-option>
              <template #prefix>
                <span class="rdf-type-dot" :style="{ background: recordTypeBg(recordForm.type) }"></span>
              </template>
            </el-select>
          </el-form-item>

          <el-form-item prop="host">
            <template #label><span class="rdf-label">{{ t('record.host') }}</span></template>
            <el-input v-model="recordForm.host" placeholder="@ / www / mail" clearable>
              <template #suffix>
                <el-tooltip :content="t('record.hostTip')" placement="top">
                  <svg style="width:13px;height:13px;color:var(--app-text-secondary)" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
                </el-tooltip>
              </template>
            </el-input>
          </el-form-item>

          <!-- Row 2: value full width -->
          <el-form-item prop="value" class="record-col-full">
            <template #label><span class="rdf-label">{{ t('record.value') }}</span></template>
            <el-input
              v-model="recordForm.value"
              :placeholder="recordValuePlaceholder"
              clearable
              :type="recordForm.type === 'TXT' ? 'textarea' : 'text'"
              :rows="3"
            />
            <div v-if="recordValueHint" class="mn-field-hint">
              <svg style="width:11px;height:11px;flex-shrink:0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12.01" y2="8"/><line x1="12" y1="12" x2="12" y2="16"/></svg>
              {{ recordValueHint }}
            </div>
          </el-form-item>

          <!-- Row 3: TTL + status -->
          <el-form-item prop="ttl">
            <template #label><span class="rdf-label">{{ t('record.ttl') }}</span></template>
            <div class="soa-num-wrap">
              <el-input-number v-model="recordForm.ttl" :min="1" :max="86400" :step="60" style="width:100%" controls-position="right" />
              <span class="soa-unit">{{ t('record.second') }}</span>
            </div>
            <div class="rdf-ttl-sub">
              <span class="rdf-ttl-readable">{{ formatSeconds(recordForm.ttl) }}</span>
              <el-dropdown size="small" @command="(v: number) => { recordForm.ttl = v }">
                <span class="rdf-ttl-preset-link">{{ t('record.quickPreset') }}<svg style="width:9px;height:9px;margin-left:2px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg></span>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-for="p in TTL_PRESETS" :key="p.value" :command="p.value">{{ p.label }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </el-form-item>

          <el-form-item prop="status">
            <template #label><span class="rdf-label">{{ t('record.enableStatus') }}</span></template>
            <div class="rdf-status-wrap">
              <el-switch
                :model-value="recordForm.status === '启用'"
                inline-prompt
                :active-text="t('record.enabled')"
                :inactive-text="t('record.disabled')"
                style="--el-switch-on-color:var(--app-success)"
                @change="(v: boolean) => { recordForm.status = v ? '启用' : '禁用' }"
              />
              <span class="rdf-status-label" :class="recordForm.status === '启用' ? 'rdf-status-on' : 'rdf-status-off'">
                {{ recordForm.status === '启用' ? t('record.recordActiveHint') : t('record.recordInactiveHint') }}
              </span>
            </div>
          </el-form-item>

          <!-- Row 4: remark full width -->
          <el-form-item class="record-col-full">
            <template #label><span class="rdf-label">{{ t('record.remark') }}</span></template>
            <el-input v-model="recordForm.remark" :placeholder="t('record.remarkPlaceholder')" clearable maxlength="255" show-word-limit />
          </el-form-item>

        </div>
      </el-form>
      <template #footer>
        <div class="record-dialog-footer">
          <el-button link @click="clearRecordForm" style="color:var(--app-text-secondary)">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.91"/></svg></template>
            {{ t('record.clearForm') }}
          </el-button>
          <div class="record-dialog-footer-right">
            <el-button @click="closeRecordDialog">{{ t('record.cancel') }}</el-button>
            <el-button type="primary" :loading="domainStore.submitting" @click="submitRecord">{{ t('record.saveRecord') }}</el-button>
          </div>
        </div>
      </template>
    </el-dialog>

    <!-- Import dialog with conflict detection -->
    <RecordImportDialog
      v-model="importDialogVisible"
      :existing-records="allRecordRows"
      @confirm="handleImportConfirm"
    />

    <!-- Batch edit dialog -->
    <el-dialog v-model="batchEditVisible" :title="t('record.batchEditTitle')" width="400px" destroy-on-close>
      <el-form :model="batchEditForm" label-position="top" class="mn-modal-form">
        <el-form-item :label="t('record.batchTtlLabel')">
          <div class="soa-num-wrap">
            <el-input-number v-model="batchEditForm.ttl" :min="0" :max="86400" controls-position="right" style="width:100%" />
            <span class="soa-unit">{{ t('record.second') }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('record.batchStatusLabel')">
          <el-select v-model="batchEditForm.status" clearable :placeholder="t('record.noChange')" style="width:100%">
            <el-option :label="t('record.enabled')" value="启用" />
            <el-option :label="t('record.disabled')" value="禁用" />
          </el-select>
        </el-form-item>
        <p style="font-size:12px;color:var(--app-text-regular);margin:0">
          {{ t('record.batchApplyHint', { count: selectedRecordRows.length }) }}
        </p>
      </el-form>
      <template #footer>
        <el-button @click="batchEditVisible = false">{{ t('record.cancel') }}</el-button>
        <el-button type="primary" :loading="batchEditSubmitting" @click="submitBatchEdit">{{ t('record.confirmBatchEdit') }}</el-button>
      </template>
    </el-dialog>

    <!-- History panel drawer -->
    <el-drawer v-model="historyDrawerVisible" :title="t('record.historyTitle')" size="320px" direction="rtl">
      <div v-if="!recordHistory.length" style="padding:24px;text-align:center;color:var(--app-text-secondary);font-size:13px">{{ t('record.noHistory') }}</div>
      <div v-else class="hist-list">
        <div v-for="(snap, i) in [...recordHistory].reverse()" :key="snap.id" class="hist-item">
          <div class="hist-item-label">{{ snap.label }}</div>
          <div class="hist-item-time">{{ new Date(snap.timestamp).toLocaleTimeString() }} · {{ t('record.recordCount', { count: snap.records.length }) }}</div>
          <el-button size="small" link type="primary" @click="zoneHistory.restoreTo(snap.id)">{{ t('record.rollbackTo') }}</el-button>
        </div>
      </div>
    </el-drawer>

    <!-- Import progress dialog -->
    <el-dialog
      :model-value="recordImporting"
      :title="t('record.importingTitle')"
      width="380px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
      align-center
    >
      <div class="import-progress-body">
        <div class="import-progress-pct">{{ recordImportProgress }}%</div>
        <el-progress :percentage="recordImportProgress" :stroke-width="10" striped striped-flow :duration="8" style="width:100%" />
        <div class="import-progress-hint">{{ t('record.importingHint') }}</div>
      </div>
    </el-dialog>
  </div>

  <el-empty v-else :description="t('record.zoneNotFound')">
    <el-button type="primary" @click="router.push('/domain/zone-list')">{{ t('record.returnZoneList') }}</el-button>
  </el-empty>
</template>

<style scoped>
/* ═══════════════ Tabs ═══════════════ */
.zone-edit-tabs { --el-tabs-header-height: 42px; }
.zone-edit-tabs :deep(.el-tabs__header) { padding: 0 20px; margin-bottom: 0; border-bottom: 1px solid var(--app-border); }
.zone-edit-tabs :deep(.el-tabs__content) { padding: 0; }
.zone-edit-tabs :deep(.el-tabs__item) { font-size: 13px; font-weight: 500; color: var(--app-text-regular); }
.zone-edit-tabs :deep(.el-tabs__item.is-active) { color: var(--app-accent); font-weight: 600; }
.zone-edit-tabs :deep(.el-tabs__active-bar) { background-color: var(--app-accent); }

/* ═══════════════ Page actions ═══════════════ */
.zone-page-actions { position: sticky; top: 0; z-index: 3; }

/* ═══════════════ Toolbar ═══════════════ */
.mn-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}
.mn-toolbar-filters { display: flex; align-items: center; gap: 8px; flex: 1; flex-wrap: wrap; }
.mn-toolbar-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.mn-record-count { font-size: 12px; color: var(--app-text-regular); }
.mn-record-count strong { color: var(--app-title); font-weight: 600; }

/* ═══════════════ Table ═══════════════ */
.mn-table :deep(.el-table__header th .cell) {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
}
.mn-table :deep(.el-table__header th) { background: var(--app-bg-secondary); }
.mn-table :deep(.el-table__body td .cell) { font-size: 13px; line-height: 1.5; }
.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) { background: rgba(22,93,255,0.04) !important; }

.mn-row-ops { display: flex; align-items: center; justify-content: flex-end; gap: 4px; }
.mn-ttl-ops { display: inline-flex; align-items: center; gap: 4px; }
.mn-th-with-tip { display: inline-flex; align-items: center; gap: 4px; }

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 10px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Badges ═══════════════ */
.mn-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 20px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.6;
  white-space: nowrap;
  border: 1px solid transparent;
  letter-spacing: 0.02em;
}
.mn-badge--neutral { color: var(--app-text-secondary); background: rgba(78,89,105,0.08); border-color: rgba(78,89,105,0.18); }

/* ═══════════════ Typography helpers ═══════════════ */
.mn-mono {
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', ui-monospace, monospace;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}
.mn-help-icon { color: var(--app-text-regular); cursor: help; font-size: 13px; }

/* ═══════════════ Panel header ═══════════════ */
.mn-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}
.mn-panel-icon { width: 15px; height: 15px; color: var(--app-accent); flex-shrink: 0; }

/* ═══════════════ Virtual table ═══════════════ */
.virtual-table-wrap { width: 100%; height: 560px; }
.virtual-cell-center { display: flex; align-items: center; justify-content: center; height: 100%; }
.virtual-ttl-ops { display: inline-flex; align-items: center; gap: 6px; }

/* ═══════════════ Record dialog ═══════════════ */
.mn-modal-form {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 16px 20px 4px;
}
.mn-modal-form :deep(.el-form-item__label) {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-secondary);
  padding-bottom: 5px;
}
.mn-modal-form :deep(.el-form-item__error) { font-size: 12px; }

.record-dialog-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px 20px;
}
.record-col-full { grid-column: span 2; }
.record-dialog-grid :deep(.el-form-item) { margin-bottom: 0; }
.record-dialog-grid :deep(.el-form-item__label) { padding-bottom: 0; margin-bottom: 4px; }

/* Label */
.rdf-label { font-size: 13px; font-weight: 500; color: var(--app-text-secondary); }

/* Type select option */
.rdf-type-option {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rdf-type-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  display: inline-block;
}
.rdf-type-desc { margin-left: auto; font-size: 11px; color: var(--app-text-secondary); }

/* TTL */
.rdf-ttl-preset-link {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 500;
  color: var(--app-accent);
  cursor: pointer;
  user-select: none;
}
.rdf-ttl-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 5px;
}
.rdf-ttl-readable {
  font-size: 12px;
  color: var(--app-accent);
  font-weight: 500;
}

/* Status */
.rdf-status-wrap { display: flex; align-items: center; gap: 10px; }
.rdf-status-label { font-size: 12px; }
.rdf-status-on { color: var(--app-success); }
.rdf-status-off { color: var(--app-text-secondary); }

.record-dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.record-dialog-footer-right { display: flex; gap: 8px; }

.mn-field-hint {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  margin-top: 6px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* ═══════════════ SOA panel ═══════════════ */
.soa-panel {
  padding: 20px 20px 6px;
}
.soa-form-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}
.soa-form-grid :deep(.el-form-item) { margin-bottom: 0; }
.soa-col-6 { grid-column: span 6; }
.soa-col-3 { grid-column: span 3; }
.mn-form-label-with-tip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-secondary);
  padding-bottom: 5px;
}
.soa-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 20px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* SOA number input + unit */
.soa-num-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}
.soa-num-wrap .el-input-number { flex: 1; }
.soa-unit {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  font-weight: 500;
}

/* ═══════════════ DNSSEC panel ═══════════════ */
.dnssec-panel { display: flex; flex-direction: column; gap: 0; }
.dnssec-key-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  padding: 16px;
  align-items: start;
}
.dnssec-key-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  overflow: hidden;
  box-shadow: var(--app-shadow-soft);
}
.dnssec-key-card :deep(.el-card__body) { padding: 0; }
.dnssec-key-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}
.dnssec-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 20px 16px;
  color: var(--app-text-secondary);
  font-size: 12px;
}
.dnssec-empty svg { width: 18px; height: 18px; flex-shrink: 0; opacity: 0.45; }
.dnssec-key-list { display: flex; flex-direction: column; gap: 10px; padding: 16px; }
.dnssec-key-item {
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--app-bg-secondary);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.dnssec-key-item .section-title { font-size: 12px; font-weight: 600; font-family: 'JetBrains Mono','Consolas',monospace; color: var(--app-title); }
.dnssec-key-item .stat-label { font-size: 12px; color: var(--app-text-regular); }
.dnssec-rotate-btn { font-size: 12px; }
.dnssec-switch :deep(.el-switch__core) { border-radius: 999px; }

.dnssec-key-card :deep(.el-empty__description) {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Table density ═══════════════ */
.mn-table--compact :deep(.el-table__body td .cell) { padding-top: 4px; padding-bottom: 4px; font-size: 12px; }
.mn-table--compact :deep(.el-table__row) { height: 36px; }
.mn-table--relaxed :deep(.el-table__body td .cell) { padding-top: 12px; padding-bottom: 12px; }
.mn-table--relaxed :deep(.el-table__row) { height: 56px; }

/* ═══════════════ Density toggle ═══════════════ */
.mn-density-group {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--app-border);
  border-radius: 6px;
  overflow: hidden;
  background: var(--app-bg-secondary);
}
.mn-density-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--app-text-regular);
  transition: background 0.15s, color 0.15s;
  padding: 0;
}
.mn-density-btn svg { width: 13px; height: 13px; }
.mn-density-btn:hover { background: var(--app-border); color: var(--app-title); }
.mn-density-btn.active { background: var(--app-accent); color: #fff; }

/* ═══════════════ SOA serial chip ═══════════════ */
.mn-soa-serial-chip {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border: 1px solid var(--app-border);
  border-radius: 20px;
  background: var(--app-bg);
}
.mn-soa-serial-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
}
.mn-soa-serial-value { font-size: 12px; color: var(--app-title); }

/* ═══════════════ DNSSEC key row + DS records ═══════════════ */
.dnssec-key-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.dnssec-ds-card { grid-column: span 2; }
.ds-digest-wrap {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 4px;
  padding: 6px 10px;
  background: var(--app-bg);
  border: 1px solid var(--app-border);
  border-radius: 6px;
  overflow: hidden;
}
.ds-digest-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
  flex-shrink: 0;
  padding-top: 1px;
}
.ds-digest-value {
  font-size: 11px;
  word-break: break-all;
  color: var(--app-text);
  line-height: 1.5;
}

/* ═══════════════ Badge extra variants ═══════════════ */
.mn-badge--success { color: var(--app-success); background: rgba(0,180,42,0.08); border-color: rgba(0,180,42,0.22); }
.mn-badge--warning { color: var(--app-warning); background: rgba(255,125,0,0.08); border-color: rgba(255,125,0,0.22); }

/* ═══════════════ Toolbar input icon ═══════════════ */
.mn-input-icon { width: 13px; height: 13px; color: var(--app-text-secondary); }

/* ═══════════════ Colored record type badge ═══════════════ */
.mn-type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.05em;
  white-space: nowrap;
}

/* ═══════════════ SOA human-readable bar ═══════════════ */
.soa-readable-bar {
  display: flex;
  gap: 0;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-bg);
}
.soa-readable-chip {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 12px;
  border-right: 1px solid var(--app-border);
}
.soa-readable-chip:last-child { border-right: none; }
.soa-readable-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
}
.soa-readable-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-accent);
  margin-top: 2px;
}

/* ═══════════════ DNSSEC trust chain ═══════════════ */
.dnssec-chain {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: 16px;
}
.dnssec-chain-step {
  display: flex;
  align-items: center;
  gap: 4px;
}
.chain-node {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  border: 1px solid currentColor;
}
.chain-ok .chain-node { color: var(--app-success); background: rgba(0,180,42,0.08); }
.chain-warn .chain-node { color: var(--app-warning); background: rgba(255,125,0,0.08); }
.chain-off .chain-node { color: var(--app-text-secondary); background: rgba(78,89,105,0.08); }
.chain-arrow { width: 28px; height: 8px; }
.chain-ok .chain-arrow { color: var(--app-success); }
.chain-warn .chain-arrow { color: var(--app-warning); }
.chain-off .chain-arrow { color: var(--app-text-secondary); }
.chain-label {
  font-size: 11px;
  font-weight: 500;
  color: var(--app-text-secondary);
}

/* ═══════════════ History drawer ═══════════════ */
.hist-list { display: flex; flex-direction: column; }
.hist-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--app-border);
}
.hist-item-label { font-size: 13px; font-weight: 500; color: var(--app-title); }
.hist-item-time { font-size: 11px; color: var(--app-text-secondary); }

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 900px) {
  .record-dialog-grid { grid-template-columns: 1fr; }
  .record-col-full { grid-column: auto; }
  .soa-col-6, .soa-col-3 { grid-column: span 12; }
  .dnssec-key-grid { grid-template-columns: 1fr; }
  .dnssec-ds-card { grid-column: auto; }
  .record-dialog-footer { flex-direction: column; align-items: stretch; }
  .record-dialog-footer-right { justify-content: flex-end; }
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { flex-wrap: wrap; }
}

/* ═══════════════ Import progress ═══════════════ */
.import-progress-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 8px 4px 4px;
}
.import-progress-pct {
  font-size: 32px;
  font-weight: 700;
  color: var(--app-accent);
  line-height: 1;
}
.import-progress-hint {
  font-size: 12px;
  color: var(--app-text-secondary);
}
</style>




