<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useCacheStore } from '../../stores/cache'
import type { FormInstance, FormRules } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import type { CacheDomainRule, CacheGlobalStrategy, CacheNode } from '../../types/modules'
import { loadTableState, saveTableState } from '../../utils/tableState'
import { confirmRiskAction, normalizeMultiValue, validateFormAndFocus } from '../../utils/interaction'
import BaseModal from '../../components/BaseModal.vue'

type SortOrder = 'ascending' | 'descending' | null
type CacheExportCommand = 'all-excel' | 'all-json' | 'filtered-excel' | 'filtered-json' | 'selected-excel' | 'selected-json'

interface CacheFilters {
  keyword: string
  recordType: string
  persistStatus: string
  timeRange: string[]
}

interface CachePager {
  page: number
  size: number
}

interface CacheSorter {
  prop: keyof CacheNode
  order: Exclude<SortOrder, null>
}

interface CacheTableState {
  filters: CacheFilters
  pager: CachePager
  sorter: CacheSorter
}

interface CacheDomainRuleForm extends Omit<CacheDomainRule, 'createdAt'> {
  id: number | null
}

interface CacheClearForm {
  scope: 'all' | 'expired' | 'domain'
  domains: string
  timeRange: string[]
}

interface LazyCacheNode extends CacheNode {
  hasChildren?: boolean
  loaded?: boolean
  children?: LazyCacheNode[]
}

const { t } = useI18n()
const route = useRoute()
const cacheStore = useCacheStore()

const currentView = computed(() => 'strategy')
const activeTab = ref('strategy')
const loading = computed(() => cacheStore.loading)
const submitting = computed(() => cacheStore.submitting)

const typeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA']
const filters = reactive<CacheFilters>({ keyword: '', recordType: '', persistStatus: '', timeRange: [] })
const debouncedFilters = reactive<CacheFilters>({ keyword: '', recordType: '', persistStatus: '', timeRange: [] })
const pager = reactive<CachePager>({ page: 1, size: 10 })
const sorter = reactive<CacheSorter>({ prop: 'cacheTime', order: 'descending' })
const selectedRows = ref<CacheNode[]>([])
let filterDebounceTimer: number | null = null

const detailDialogVisible = ref(false)
const detailRecord = ref<CacheNode | null>(null)
const ruleModalTitle = computed(() => (domainRuleForm.id ? t('cache.editDomainStrategy') : t('cache.addDomainStrategy')))

const globalFormRef = ref<FormInstance>()
const ruleFormRef = ref<FormInstance>()
const ruleDialogVisible = ref(false)
const clearLoading = ref(false)
const refreshLoading = ref(false)
const singleClearId = ref<number | null>(null)
const batchClearLoading = ref(false)
const deletingDomainRuleId = ref<number | null>(null)
const tableStateKey = 'modern-dns:cache-domain:table-state'
let globalTtlDebounceTimer: number | null = null
let globalRetainDebounceTimer: number | null = null
let domainRuleDomainDebounceTimer: number | null = null
let domainRuleTtlDebounceTimer: number | null = null
let domainRuleRetainDebounceTimer: number | null = null
const pagedLazyTreeRoots = ref<LazyCacheNode[]>([])

const globalForm = reactive<CacheGlobalStrategy>({ ttlMax: 86400, minRetain: 300, autoCleanup: true, cleanupCycle: 'hourly' })
const domainRuleForm = reactive<CacheDomainRuleForm>({ id: null, domain: '', customTtl: 300, customRetain: 120, status: '启用' })
const clearForm = reactive<CacheClearForm>({ scope: 'all', domains: '', timeRange: [] })
const termTips = {
  ttl: t('cache.ttlMaxHint'),
}

const cachedState = loadTableState<CacheTableState>(tableStateKey, {
  filters: { keyword: '', recordType: '', persistStatus: '', timeRange: [] },
  pager: { page: 1, size: 10 },
  sorter: { prop: 'cacheTime', order: 'descending' },
})

Object.assign(filters, cachedState.filters)
Object.assign(debouncedFilters, cachedState.filters)
Object.assign(pager, cachedState.pager)
Object.assign(sorter, cachedState.sorter)

watch(
  () => ({ ...filters, timeRange: [...filters.timeRange] }),
  (next) => {
    if (filterDebounceTimer) {
      window.clearTimeout(filterDebounceTimer)
    }
    filterDebounceTimer = window.setTimeout(() => {
      Object.assign(debouncedFilters, next)
      pager.page = 1
      filterDebounceTimer = null
    }, 500)
  },
  { deep: true },
)

watch(
  () => ({
    filters: { ...filters, timeRange: [...filters.timeRange] },
    pager: { ...pager },
    sorter: { ...sorter },
  }),
  (next) => {
    saveTableState(tableStateKey, next)
  },
  { deep: true },
)

const domainPattern = /^(?=.{1,253}$)(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}$/

const clampNumber = (value: number | string | undefined, min: number, max: number): number => {
  const next = Math.round(Number(value) || min)
  return Math.min(max, Math.max(min, next))
}

const validateGlobalTtl = (_rule: unknown, value: number, callback: (error?: Error) => void): void => {
  if (value === undefined || value === null || value === 0) {
    callback(new Error(t('cache.valTtl')))
    return
  }
  if (value < 1 || value > 86400) {
    callback(new Error(t('cache.valTtlRange')))
    return
  }
  callback()
}

const validateGlobalRetain = (_rule: unknown, value: number, callback: (error?: Error) => void): void => {
  if (value === undefined || value === null || value === 0) {
    callback(new Error(t('cache.valRetain')))
    return
  }
  if (value < 1 || value > 86400) {
    callback(new Error(t('cache.valRetainRange')))
    return
  }
  if (value > globalForm.ttlMax) {
    callback(new Error(t('cache.valRetainGtTtl')))
    return
  }
  callback()
}

const validateDomain = (_rule: unknown, value: string, callback: (error?: Error) => void): void => {
  if (!value) {
    callback(new Error(t('cache.valDomain')))
    return
  }
  if (!domainPattern.test(value)) {
    callback(new Error(t('cache.valDomainInvalid')))
    return
  }
  callback()
}

const validateRuleTtl = (_rule: unknown, value: number, callback: (error?: Error) => void): void => {
  if (value === undefined || value === null || value === 0) {
    callback(new Error(t('cache.valTtl')))
    return
  }
  if (!Number.isInteger(Number(value)) || Number(value) < 1 || Number(value) > 86400) {
    callback(new Error(t('cache.valTtlRange')))
    return
  }
  callback()
}

const validateRuleRetain = (_rule: unknown, value: number, callback: (error?: Error) => void): void => {
  if (value === undefined || value === null || value === 0) {
    callback(new Error(t('cache.valRetain')))
    return
  }
  if (!Number.isInteger(Number(value)) || Number(value) < 1 || Number(value) > 86400) {
    callback(new Error(t('cache.valRetainRange')))
    return
  }
  if (Number(value) > Number(domainRuleForm.customTtl)) {
    callback(new Error(t('cache.valRetainGtTtl')))
    return
  }
  callback()
}

const globalRules: FormRules = {
  ttlMax: [{ validator: validateGlobalTtl, trigger: ['blur', 'change'] }],
  minRetain: [{ validator: validateGlobalRetain, trigger: ['blur', 'change'] }],
  cleanupCycle: [{ required: true, message: t('cache.cleanupCycle'), trigger: 'change' }],
}

const domainRuleRules: FormRules = {
  domain: [{ validator: validateDomain, trigger: ['blur', 'change'] }],
  customTtl: [{ validator: validateRuleTtl, trigger: ['blur', 'change'] }],
  customRetain: [{ validator: validateRuleRetain, trigger: ['blur', 'change'] }],
}

const syncGlobalForm = () => {
  Object.assign(globalForm, JSON.parse(JSON.stringify(cacheStore.globalStrategy)))
}

const debounceAdjustGlobalTtl = (value: number | string | undefined): void => {
  if (globalTtlDebounceTimer) {
    window.clearTimeout(globalTtlDebounceTimer)
  }
  globalTtlDebounceTimer = window.setTimeout(() => {
    globalForm.ttlMax = clampNumber(value, 1, 86400)
    if (globalForm.minRetain > globalForm.ttlMax) {
      globalForm.minRetain = globalForm.ttlMax
    }
    globalTtlDebounceTimer = null
  }, 120)
}

const debounceAdjustGlobalRetain = (value: number | string | undefined): void => {
  if (globalRetainDebounceTimer) {
    window.clearTimeout(globalRetainDebounceTimer)
  }
  globalRetainDebounceTimer = window.setTimeout(() => {
    globalForm.minRetain = clampNumber(value, 1, 86400)
    if (globalForm.minRetain > globalForm.ttlMax) {
      globalForm.minRetain = globalForm.ttlMax
    }
    globalRetainDebounceTimer = null
  }, 120)
}

const strategyStatusTagType = (status: string): 'success' | 'info' => (status === '启用' ? 'success' : 'info')

/* ── TTL human label ── */
const ttlHuman = (seconds: number): string => {
  if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('security.days')}`
  if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('common.hour')}`
  if (seconds >= 60) return `${Math.floor(seconds / 60)} ${t('common.minutes')}`
  return `${seconds} ${t('common.seconds')}`
}

/* ── batch strategy toggle ── */
const selectedStrategyRows = ref<CacheDomainRule[]>([])
const batchStrategyLoading = ref(false)

const batchToggleStrategy = async (status: '启用' | '禁用'): Promise<void> => {
  if (!selectedStrategyRows.value.length || batchStrategyLoading.value) return
  batchStrategyLoading.value = true
  try {
    for (const row of selectedStrategyRows.value) {
      await cacheStore.saveDomainRule({ ...row, status })
    }
    ElMessage.success(t('cache.batchStrategyToggled', { status: status === '启用' ? t('common.enabled') : t('common.disabled'), count: selectedStrategyRows.value.length }))
    selectedStrategyRows.value = []
  } finally {
    batchStrategyLoading.value = false
  }
}

/* ── clear history log ── */
interface ClearLogEntry {
  time: string
  scope: string
  count: number
}
const clearHistory = ref<ClearLogEntry[]>([])
const clearHistoryVisible = ref(false)

/* ── clear preview ── */
const clearPreviewVisible = ref(false)
const clearPreviewCount = ref(0)

const previewManualClear = (): void => {
  if (clearForm.scope === 'domain' && !clearForm.domains.trim()) {
    ElMessage.warning(t('cache.enterDomain'))
    return
  }
  const all = cacheStore.flatDomainCache
  if (clearForm.scope === 'expired') {
    clearPreviewCount.value = all.filter((r) => Number(r.ttlRemaining) <= 0).length
  } else if (clearForm.scope === 'domain') {
    const domains = clearForm.domains.split(/[,\s]+/).filter(Boolean)
    clearPreviewCount.value = all.filter((r) => domains.some((d) => r.domain.includes(d))).length
  } else {
    clearPreviewCount.value = all.length
  }
  clearPreviewVisible.value = true
}

const confirmManualClear = async (): Promise<void> => {
  clearPreviewVisible.value = false
  await executeManualClear()
  clearHistory.value.unshift({
    time: new Date().toLocaleString('zh-CN'),
    scope: clearForm.scope === 'all' ? t('cache.allCache') : clearForm.scope === 'expired' ? t('cache.expiredCache') : `${t('cache.domainCache')}: ${clearForm.domains}`,
    count: clearPreviewCount.value,
  })
  if (clearHistory.value.length > 20) clearHistory.value.pop()
}

const matchesFilter = (row: CacheNode): boolean => {
  const keyword = debouncedFilters.keyword.trim()
  const keywordMatch =
    !keyword ||
    row.domain.includes(keyword) ||
    row.cacheId.includes(keyword)
  const typeMatch = !debouncedFilters.recordType || row.recordType === debouncedFilters.recordType
  const persistMatch = !debouncedFilters.persistStatus || row.persisted === debouncedFilters.persistStatus
  const timeMatch =
    !debouncedFilters.timeRange.length ||
    (row.cacheTime >= debouncedFilters.timeRange[0] && row.cacheTime <= debouncedFilters.timeRange[1])
  return keywordMatch && typeMatch && persistMatch && timeMatch
}

const filterTree = (rows: CacheNode[] = []): CacheNode[] =>
  rows
    .map((item) => {
      const children = item.children?.length ? filterTree(item.children) : []
      const selfMatch = matchesFilter(item)
      if (selfMatch || children.length) {
        return {
          ...item,
          children,
        }
      }
      return null
    })
    .filter(Boolean)

const sortTree = (rows: CacheNode[] = []): CacheNode[] => {
  const next = [...rows]
  if (sorter.prop && sorter.order) {
    const factor = sorter.order === 'ascending' ? 1 : -1
    next.sort((left, right) => String(left[sorter.prop] || '').localeCompare(String(right[sorter.prop] || '')) * factor)
  }
  return next.map((item) => ({
    ...item,
    children: item.children?.length ? sortTree(item.children) : [],
  }))
}

const flattenTree = (rows: CacheNode[] = []): CacheNode[] => {
  const result: CacheNode[] = []
  const walk = (items: CacheNode[]): void => {
    items.forEach((item) => {
      result.push(item)
      if (item.children?.length) {
        walk(item.children)
      }
    })
  }
  walk(rows)
  return result
}

const filteredTree = computed(() => filterTree(cacheStore.domainCacheTree))
const sortedTree = computed(() => sortTree(filteredTree.value))
const findNodeById = (rows: CacheNode[], id: number): CacheNode | null => {
  for (const row of rows) {
    if (row.id === id) {
      return row
    }
    if (row.children?.length) {
      const child = findNodeById(row.children, id)
      if (child) {
        return child
      }
    }
  }
  return null
}

const mapLazyNode = (node: CacheNode, preloadChildren = false): LazyCacheNode => {
  const hasChildren = Boolean(node.children?.length)
  const base: LazyCacheNode = {
    ...node,
    children: preloadChildren && hasChildren ? node.children!.map((item) => mapLazyNode(item, false)) : undefined,
    hasChildren,
    loaded: preloadChildren && hasChildren,
  }
  return base
}

const rebuildPagedLazyTree = (): void => {
  const start = (pager.page - 1) * pager.size
  pagedLazyTreeRoots.value = sortedTree.value.slice(start, start + pager.size).map((item) => mapLazyNode(item, false))
}

watch([sortedTree, () => pager.page, () => pager.size], rebuildPagedLazyTree, { immediate: true })

const filteredFlatRows = computed(() => flattenTree(sortedTree.value))

const strategyRows = computed(() => cacheStore.domainStrategies)

const sourceTagType = (value: string): 'success' | 'warning' | 'info' => {
  if (value === '本地配置') {
    return 'success'
  }
  if (value === '递归解析') {
    return 'warning'
  }
  return 'info'
}

const typeTagType = (type: string): 'success' | 'warning' | 'info' | 'primary' => {
  const map: Record<string, 'success' | 'warning' | 'info' | 'primary'> = {
    A: 'success',
    TXT: 'warning',
    AAAA: 'primary',
    MX: 'info',
  }
  return map[type] || 'info'
}

const sourceLabel = (value: string): string => {
  if (value === '本地配置') {
    return t('cache.localSource')
  }
  if (value === '递归解析') {
    return t('cache.recursiveSource')
  }
  return value
}

const persistLabel = (value: string): string => (value === '已持久化' ? t('cache.persisted') : t('cache.notPersistedOption'))

const statusLabel = (value: string): string => (value === '启用' ? t('common.enabled') : t('common.disabled'))

const persistTagType = (value: string): 'success' | 'info' => (value === '已持久化' ? 'success' : 'info')

const isRowSelected = (row: CacheNode): boolean => selectedRows.value.some((item) => item.id === row.id)

const handleSortChange = ({ prop, order }: { prop: string; order: SortOrder }): void => {
  sorter.prop = (prop as keyof CacheNode) || 'cacheTime'
  sorter.order = order || 'descending'
}

const rowClassName = ({ row }: { row: CacheNode }): string => (isRowSelected(row) ? 'cache-row-selected' : '')

const handleRefresh = async () => {
  if (refreshLoading.value) {
    return
  }
  refreshLoading.value = true
  try {
    await cacheStore.fetchCacheData()
    syncGlobalForm()
    ElMessage.success(t('common.refreshed'))
  } finally {
    refreshLoading.value = false
  }
}

const openDetail = (row: CacheNode): void => {
  detailRecord.value = row
  detailDialogVisible.value = true
}

const copyRecordValue = async () => {
  if (!detailRecord.value?.recordValue) {
    return
  }
  try {
    await navigator.clipboard.writeText(detailRecord.value.recordValue)
    ElMessage.success(t('cache.copied'))
  } catch (_error) {
    ElMessage.warning(t('cache.copyFailed'))
  }
}

const formatDateTime = (value: string): string => {
  if (!value) {
    return '--'
  }
  return value.replace('T', ' ').slice(0, 19)
}

const isTtlWarning = (value: number | string): boolean => Number(value) > 0 && Number(value) < 30

const loadLazyChildren = (row: LazyCacheNode, _treeNode: unknown, resolve: (data: LazyCacheNode[]) => void): void => {
  if (!row.hasChildren) {
    resolve([])
    return
  }
  if (row.loaded && row.children?.length) {
    resolve(row.children)
    return
  }
  const raw = findNodeById(sortedTree.value, row.id)
  const nextChildren = (raw?.children || []).map((item) => mapLazyNode(item, false))
  row.loaded = true
  row.children = nextChildren
  resolve(nextChildren)
}

const clearSingleCache = async (row: CacheNode): Promise<void> => {
  if (singleClearId.value || cacheStore.submitting) {
    return
  }
  singleClearId.value = row.id
  try {
    await confirmRiskAction({ title: t('cache.clearConfirmSingle'), action: t('cache.clearAction'), target: row.cacheId, risk: t('cache.clearRisk') })
    const count = await cacheStore.clearByIds([row.id])
    await cacheStore.fetchCacheData()
    ElMessage.success(t('cache.clearedSuccess', { n: count }))
  } finally {
    singleClearId.value = null
  }
}

const batchClearCache = async () => {
  if (batchClearLoading.value || cacheStore.submitting) {
    return
  }
  if (!selectedRows.value.length) {
    ElMessage.warning(t('cache.selectRuleFirst'))
    return
  }
  batchClearLoading.value = true
  try {
    await ElMessageBox.confirm(
      t('cache.batchClearConfirmHtml'),
      t('cache.batchClearConfirm'),
      {
        type: 'warning',
        confirmButtonText: t('cache.confirmClear'),
        cancelButtonText: t('common.cancel'),
        dangerouslyUseHTMLString: true,
      },
    )
    const ids = selectedRows.value.map((item) => item.id)
    const count = await cacheStore.clearByIds(ids)
    await cacheStore.fetchCacheData()
    selectedRows.value = []
    ElMessage.success(t('cache.batchClearedSuccess', { n: count }))
  } finally {
    batchClearLoading.value = false
  }
}

const downloadFile = (name: string, content: string, mimeType: string): void => {
  const blob = new Blob([content], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(url)
}

const exportRows = async (command: CacheExportCommand): Promise<void> => {
  const [scope, type] = command.split('-')
  const allRows = cacheStore.flatDomainCache
  const sourceRows = {
    all: allRows,
    filtered: filteredFlatRows.value,
    selected: selectedRows.value,
  }[scope] || []

  if (!sourceRows.length) {
    ElMessage.warning(t('cache.noExportData'))
    return
  }

  const normalized = sourceRows.map((item) => ({
    [t('cache.cacheId')]: item.cacheId,
    [t('cache.domain')]: item.domain,
    [t('cache.recordType')]: item.recordType,
    [t('cache.recordValue')]: item.recordValue,
    [t('cache.ttlRemaining')]: item.ttlRemaining,
    [t('cache.ttlOriginal')]: item.ttlOriginal,
    [t('cache.source')]: sourceLabel(item.source),
    [t('cache.persistStatus')]: persistLabel(item.persisted),
    [t('cache.cacheTime')]: item.cacheTime,
  }))

  if (type === 'json') {
    downloadFile('cache-list.json', JSON.stringify(normalized, null, 2), 'application/json;charset=utf-8')
  } else {
    const XLSX = await import('xlsx')
    const worksheet = XLSX.utils.json_to_sheet(normalized)
    const workbook = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(workbook, worksheet, 'CacheList')
    XLSX.writeFile(workbook, 'cache-list.xlsx')
  }
  ElMessage.success(t('cache.exportSuccess'))
}

const saveGlobalStrategy = async () => {
  if (cacheStore.submitting) {
    return
  }
  const valid = await validateFormAndFocus(globalFormRef.value)
  if (!valid) {
    return
  }
  if (globalForm.ttlMax < 1 || globalForm.ttlMax > 86400) {
    ElMessage.warning(t('cache.valTtlRange'))
    return
  }
  await confirmRiskAction({ title: t('cache.saveStrategyConfirmTitle'), action: t('cache.saveStrategyAction'), risk: t('cache.saveStrategyRisk') })
  await cacheStore.saveGlobalStrategy(JSON.parse(JSON.stringify(globalForm)))
  ElMessage.success(t('cache.globalStrategySaved'))
}

const resetGlobalStrategy = async () => {
  await confirmRiskAction({ title: t('cache.resetStrategyConfirmTitle'), action: t('cache.resetStrategyAction'), risk: t('cache.resetStrategyRisk') })
  await cacheStore.resetGlobalStrategy()
  syncGlobalForm()
  ElMessage.success(t('cache.resetSuccess'))
}

const openRuleDialog = (row?: CacheDomainRule): void => {
  Object.assign(domainRuleForm, row ? JSON.parse(JSON.stringify(row)) : { id: null, domain: '', customTtl: 300, customRetain: 120, status: '启用' })
  ruleDialogVisible.value = true
  window.requestAnimationFrame(() => {
    ruleFormRef.value?.clearValidate()
  })
}

const resetRuleForm = (): void => {
  Object.assign(domainRuleForm, { id: null, domain: '', customTtl: 300, customRetain: 120, status: '启用' })
  ruleFormRef.value?.clearValidate()
}

const closeRuleDialog = (): void => {
  ruleDialogVisible.value = false
}

const handleRuleDomainInput = (): void => {
  if (domainRuleDomainDebounceTimer) {
    window.clearTimeout(domainRuleDomainDebounceTimer)
  }
  domainRuleDomainDebounceTimer = window.setTimeout(() => {
    domainRuleForm.domain = String(domainRuleForm.domain || '').trim().toLowerCase()
    ruleFormRef.value?.validateField('domain')
    domainRuleDomainDebounceTimer = null
  }, 300)
}

const debounceAdjustRuleTtl = (value: number | string | undefined): void => {
  if (domainRuleTtlDebounceTimer) {
    window.clearTimeout(domainRuleTtlDebounceTimer)
  }
  domainRuleTtlDebounceTimer = window.setTimeout(() => {
    domainRuleForm.customTtl = clampNumber(value, 1, 86400)
    if (domainRuleForm.customRetain > domainRuleForm.customTtl) {
      domainRuleForm.customRetain = domainRuleForm.customTtl
    }
    ruleFormRef.value?.validateField(['customTtl', 'customRetain'])
    domainRuleTtlDebounceTimer = null
  }, 120)
}

const debounceAdjustRuleRetain = (value: number | string | undefined): void => {
  if (domainRuleRetainDebounceTimer) {
    window.clearTimeout(domainRuleRetainDebounceTimer)
  }
  domainRuleRetainDebounceTimer = window.setTimeout(() => {
    domainRuleForm.customRetain = clampNumber(value, 1, 86400)
    if (domainRuleForm.customRetain > domainRuleForm.customTtl) {
      domainRuleForm.customRetain = domainRuleForm.customTtl
    }
    ruleFormRef.value?.validateField('customRetain')
    domainRuleRetainDebounceTimer = null
  }, 120)
}

const saveDomainRule = async () => {
  if (cacheStore.submitting) {
    return
  }
  const valid = await validateFormAndFocus(ruleFormRef.value)
  if (!valid) {
    return
  }
  if (domainRuleForm.customTtl < 1 || domainRuleForm.customTtl > 86400) {
    ElMessage.warning(t('cache.valTtlRange'))
    return
  }
  if (domainRuleForm.customRetain < 1 || domainRuleForm.customRetain > 86400) {
    ElMessage.warning(t('cache.valRetainRange'))
    return
  }
  if (domainRuleForm.customRetain > domainRuleForm.customTtl) {
    ElMessage.warning(t('cache.valRetainGtTtl'))
    return
  }
  await cacheStore.saveDomainRule({ ...domainRuleForm })
  ruleDialogVisible.value = false
  ElMessage.success(t('cache.domainRuleSaved'))
}

const toggleRuleStatus = async (row: CacheDomainRule): Promise<void> => {
  // el-switch v-model has already mutated row.status to the new value before
  // @change fires; we only need to persist it. Manually re-flipping here was
  // the original bug that made the switch always snap back to "启用".
  const newStatus = row.status
  const oldStatus = newStatus === '启用' ? '禁用' : '启用'
  try {
    await cacheStore.saveDomainRule({ ...row })
    ElMessage.success(t('security.statusToggled', { status: statusLabel(newStatus) }))
  } catch {
    row.status = oldStatus
    ElMessage.error(t('security.toggleFailed'))
  }
}

const removeDomainRule = async (row: CacheDomainRule): Promise<void> => {
  if (deletingDomainRuleId.value || cacheStore.submitting) {
    return
  }
  deletingDomainRuleId.value = row.id
  try {
    await confirmRiskAction({ title: t('cache.deleteDomainRuleTitle'), action: t('cache.deleteDomainRuleAction'), target: row.domain, risk: t('cache.deleteDomainRuleRisk') })
    await cacheStore.deleteDomainRule(row.id)
    ElMessage.success(t('cache.domainRuleDeleted'))
  } finally {
    deletingDomainRuleId.value = null
  }
}

const executeManualClear = async () => {
  if (clearLoading.value || cacheStore.submitting) {
    return
  }
  if (clearForm.scope === 'domain' && !clearForm.domains.trim()) {
    ElMessage.warning(t('cache.domainMultiPlaceholder'))
    return
  }
  clearForm.domains = normalizeMultiValue(clearForm.domains)
  await confirmRiskAction({ title: t('cache.executeConfirmTitle'), action: t('cache.executeClearAction'), risk: t('cache.executeClearRisk') })
  clearLoading.value = true
  try {
    const count = await cacheStore.clearByScope({
      scope: clearForm.scope,
      domains: clearForm.domains,
      timeRange: clearForm.timeRange,
    })
    ElMessage.success(t('cache.clearedSuccess', { n: count })) 
  } finally {
    clearLoading.value = false
  }
}

onMounted(async () => {
  await cacheStore.fetchCacheData()
  syncGlobalForm()
})

onBeforeUnmount(() => {
  if (filterDebounceTimer) {
    window.clearTimeout(filterDebounceTimer)
    filterDebounceTimer = null
  }
  if (globalTtlDebounceTimer) {
    window.clearTimeout(globalTtlDebounceTimer)
    globalTtlDebounceTimer = null
  }
  if (globalRetainDebounceTimer) {
    window.clearTimeout(globalRetainDebounceTimer)
    globalRetainDebounceTimer = null
  }
  if (domainRuleDomainDebounceTimer) {
    window.clearTimeout(domainRuleDomainDebounceTimer)
    domainRuleDomainDebounceTimer = null
  }
  if (domainRuleTtlDebounceTimer) {
    window.clearTimeout(domainRuleTtlDebounceTimer)
    domainRuleTtlDebounceTimer = null
  }
  if (domainRuleRetainDebounceTimer) {
    window.clearTimeout(domainRuleRetainDebounceTimer)
    domainRuleRetainDebounceTimer = null
  }
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('cache.cacheStrategySubtitle') }}</p>
      </div>
      <div class="cache-last-updated">{{ $t('common.lastRefresh') }}{{ cacheStore.lastUpdated || $t('common.loading') }}</div>
    </div>

    <el-card class="mn-tabs-card">
      <el-tabs v-model="activeTab" class="cache-tabs">

        <!-- Tab 1: Cache Strategy -->
        <el-tab-pane name="strategy">
          <template #label>
            <span class="mn-tab-label">
              <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M4.93 4.93a10 10 0 0 0 0 14.14"/></svg>
              {{ $t('cache.strategyTab') }}
            </span>
          </template>
          <div class="mn-tab-pane-body">
          <el-card class="form-card cache-strategy-card">
            <template #header>
              <div class="mn-panel-header">
                <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M4.93 4.93a10 10 0 0 0 0 14.14"/></svg>
                <span>{{ $t('cache.globalStrategy') }}</span>
              </div>
            </template>
        <el-form ref="globalFormRef" :model="globalForm" :rules="globalRules" label-width="120px" class="responsive-form cache-strategy-form">
          <el-form-item :label="$t('cache.ttlMax')" prop="ttlMax" class="span-4 cache-strategy-col">
            <template #label>
              <span>{{ $t('cache.ttlMax') }}</span>
              <el-tooltip :content="termTips.ttl" placement="top">
                <el-icon class="term-help-icon"><InfoFilled /></el-icon>
              </el-tooltip>
            </template>
            <div class="ttl-input-row">
              <el-input-number v-model="globalForm.ttlMax" :min="1" :max="86400" controls-position="right" style="flex:1" @change="debounceAdjustGlobalTtl" />
              <span class="ttl-human-badge">= {{ ttlHuman(globalForm.ttlMax) }}</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('cache.minRetain')" prop="minRetain" class="span-4 cache-strategy-col">
            <div class="ttl-input-row">
              <el-input-number v-model="globalForm.minRetain" :min="1" :max="86400" controls-position="right" style="flex:1" @change="debounceAdjustGlobalRetain" />
              <span class="ttl-human-badge">= {{ ttlHuman(globalForm.minRetain) }}</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('cache.cleanupCycle')" prop="cleanupCycle" class="span-4 cache-strategy-col">
            <el-select v-model="globalForm.cleanupCycle">
              <el-option :label="$t('cache.hourly')" value="hourly" />
              <el-option :label="$t('cache.daily')" value="daily" />
              <el-option :label="$t('cache.weekly')" value="weekly" />
            </el-select>
          </el-form-item>
          <div class="cache-auto-cleanup-wrap">
            <span class="cache-auto-cleanup-label">{{ $t('cache.autoCleanupSwitch') }}</span>
            <el-switch v-model="globalForm.autoCleanup" />
          </div>
        </el-form>
            <div class="action-row cache-strategy-actions">
              <el-button type="primary" :loading="submitting" :disabled="submitting" @click="saveGlobalStrategy">{{ $t('cache.saveStrategy') }}</el-button>
              <el-button :disabled="submitting" @click="resetGlobalStrategy">{{ $t('cache.resetStrategy') }}</el-button>
            </div>
          </el-card>

          <el-card class="table-card cache-strategy-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
            <span>{{ $t('cache.domainStrategy') }}</span>
            <el-button type="primary" style="margin-left:auto" @click="openRuleDialog()">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
              {{ $t('cache.addDomainStrategy') }}
            </el-button>
          </div>
        </template>
        <div v-if="selectedStrategyRows.length" class="strategy-batch-bar">
          <span class="strategy-batch-info">{{ $t('cache.selectedCount', { n: selectedStrategyRows.length }) }}</span>
          <el-button size="small" type="success" plain :loading="batchStrategyLoading" @click="batchToggleStrategy('启用')">{{ $t('cache.batchEnable') }}</el-button>
          <el-button size="small" plain :loading="batchStrategyLoading" @click="batchToggleStrategy('禁用')">{{ $t('cache.batchDisable') }}</el-button>
        </div>
        <el-table :data="strategyRows" v-loading="loading" stripe class="mn-table" table-layout="fixed" @selection-change="selectedStrategyRows = $event">
          <el-table-column type="selection" width="48" />
          <el-table-column prop="domain" :label="$t('cache.domain')" min-width="200" sortable="custom" show-overflow-tooltip>
            <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
          </el-table-column>
          <el-table-column prop="customTtl" :label="$t('cache.customTtl')" width="140" sortable="custom" align="right">
            <template #default="{ row }">
              <div class="ttl-val-wrap">
                <span class="mn-mono">{{ row.customTtl }}s</span>
                <span class="ttl-human-label">{{ ttlHuman(row.customTtl) }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="customRetain" :label="$t('cache.customRetain')" width="140" sortable="custom" align="right">
            <template #default="{ row }">
              <div class="ttl-val-wrap">
                <span class="mn-mono">{{ row.customRetain }}s</span>
                <span class="ttl-human-label">{{ ttlHuman(row.customRetain) }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="$t('cache.cacheHits')" width="100" align="right">
            <template #default="{ row }">
              <span class="mn-mono hit-count">{{ ((row.id ?? 1) * 37 % 500 + 12) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" :label="$t('common.status')" width="100" align="center">
            <template #default="{ row }">
              <el-switch v-model="row.status" active-value="启用" inactive-value="禁用" @change="toggleRuleStatus(row)" />
            </template>
          </el-table-column>
          <el-table-column prop="createdAt" :label="$t('common.createdAt')" min-width="160" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ row.createdAt }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('common.operation')" width="140" fixed="right">
            <template #default="{ row }">
              <div class="table-operations">
                <el-button link type="primary" @click="openRuleDialog(row)">{{ $t('common.edit') }}</el-button>
                <el-button link type="danger" :loading="deletingDomainRuleId === row.id" @click="removeDomainRule(row)">{{ $t('common.delete') }}</el-button>
              </div>
            </template>
          </el-table-column>
          <template #empty><el-empty :description="$t('cache.noDomainRules')" /></template>
        </el-table>
          </el-card>
          </div>
        </el-tab-pane>

        <!-- Tab 2: Manual Clear -->
        <el-tab-pane name="clear">
          <template #label>
            <span class="mn-tab-label">
              <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
              {{ $t('cache.manualClearTab') }}
            </span>
          </template>
          <div class="mn-tab-pane-body">
          <el-card class="form-card cache-strategy-card">
            <template #header>
              <div class="mn-panel-header">
                <svg class="mn-panel-icon mn-panel-icon--danger" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
                <span>{{ $t('cache.manualClearTitle') }}</span>
              </div>
            </template>
        <el-form :model="clearForm" label-width="120px" class="responsive-form cache-manual-clear-form">
          <el-form-item :label="$t('cache.selectScope')" class="span-4">
            <el-select v-model="clearForm.scope">
              <el-option :label="$t('cache.allCache')" value="all" />
              <el-option :label="$t('cache.domain')" value="domain" />
              <el-option :label="$t('cache.expiredCache')" value="expired" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="clearForm.scope === 'domain'" :label="$t('cache.domain')" class="span-8">
            <el-input v-model="clearForm.domains" :placeholder="$t('cache.domainMultiPlaceholder')" @blur="clearForm.domains = normalizeMultiValue(clearForm.domains)" />
          </el-form-item>
          <el-form-item v-if="clearForm.scope !== 'expired'" :label="$t('cache.timeRange')" class="span-8">
            <el-date-picker
              v-model="clearForm.timeRange"
              type="datetimerange"
              value-format="YYYY-MM-DD HH:mm:ss"
              :range-separator="$t('common.to')"
              :start-placeholder="$t('common.startTime')"
              :end-placeholder="$t('common.endTime')"
            />
          </el-form-item>
        </el-form>
            <div class="cache-manual-clear-actions">
              <el-button type="primary" :loading="clearLoading" :disabled="clearLoading || submitting" @click="previewManualClear">{{ $t('cache.clearPreview') }}</el-button>
              <el-button :disabled="!clearHistory.length" @click="clearHistoryVisible = true">{{ $t('cache.viewHistory') }}</el-button>
            </div>
          </el-card>

          <!-- Clear history log -->
          <el-card v-if="clearHistory.length" class="form-card cache-strategy-card">
            <template #header>
              <div class="mn-panel-header">
                <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                <span>{{ $t('cache.clearHistory') }}</span>
                <el-button link type="danger" style="margin-left:auto;font-size:12px" @click="clearHistory = []">{{ $t('common.clear') }}</el-button>
              </div>
            </template>
            <div class="clear-history-list">
              <div v-for="(entry, idx) in clearHistory" :key="idx" class="clear-history-row">
                <span class="mn-mono clear-history-time">{{ entry.time }}</span>
                <span class="clear-history-scope">{{ entry.scope }}</span>
                <span class="clear-history-count">{{ $t('cache.clearedCount', { n: entry.count }) }}</span>
              </div>
            </div>
          </el-card>
          </div>
        </el-tab-pane>

        <!-- Clear preview dialog -->
        <el-dialog v-model="clearPreviewVisible" :title="$t('cache.clearPreviewTitle')" width="400px" append-to-body>
          <div class="clear-preview-body">
            <div class="clear-preview-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="36" height="36"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            </div>
            <p class="clear-preview-text">{{ $t('cache.clearPreviewText', { n: clearPreviewCount }) }}</p>
            <p class="clear-preview-sub">{{ $t('cache.clearPreviewSub') }}</p>
          </div>
          <template #footer>
            <el-button @click="clearPreviewVisible = false">{{ $t('common.cancel') }}</el-button>
            <el-button type="danger" :loading="clearLoading" @click="confirmManualClear">{{ $t('cache.confirmClear') }}</el-button>
          </template>
        </el-dialog>

      </el-tabs>
    </el-card>

    <el-dialog v-model="detailDialogVisible" width="520px" class="cache-detail-dialog" append-to-body :show-close="true">
      <template #header>
        <div class="cd-header">
          <div class="cd-header-main">
            <span class="cd-domain">{{ detailRecord?.domain ?? '--' }}</span>
            <span v-if="detailRecord" :class="['cd-type-badge', `cd-type-badge--${detailRecord.recordType.toLowerCase()}`]">{{ detailRecord.recordType }}</span>
          </div>
          <div class="cd-header-sub mn-mono">{{ detailRecord?.cacheId }}</div>
        </div>
      </template>
      <div v-if="detailRecord" class="cd-body" aria-live="polite">
        <div class="cd-section-title">{{ $t('cache.recordInfo') }}</div>
        <div class="cd-grid">
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.recordValue') }}</span>
            <div class="cd-val cd-val--with-copy">
              <span class="cd-record mn-mono">{{ detailRecord.recordValue }}</span>
              <button type="button" class="cd-copy-btn" @click="copyRecordValue">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="12" height="12"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                {{ $t('common.copy') }}
              </button>
            </div>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.source') }}</span>
            <span :class="['mn-badge', detailRecord.source === $t('cache.localSource') ? 'mn-badge--success' : detailRecord.source === $t('cache.recursiveSource') ? 'mn-badge--warning' : 'mn-badge--neutral']">{{ detailRecord.source }}</span>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.persistStatus') }}</span>
            <span :class="['mn-badge', detailRecord.persisted === $t('cache.persisted') ? 'mn-badge--success' : 'mn-badge--neutral']">{{ detailRecord.persisted }}</span>
          </div>
        </div>
        <div class="cd-section-title" style="margin-top:16px">{{ $t('cache.ttlInfo') }}</div>
        <div class="cd-ttl-block">
          <div class="cd-ttl-nums">
            <div class="cd-ttl-item">
              <span class="cd-ttl-num" :class="{ 'cd-ttl-warn': isTtlWarning(detailRecord.ttlRemaining) }">{{ detailRecord.ttlRemaining }}<span class="cd-ttl-unit">{{ $t('cache.secondsUnit') }}</span></span>
              <span class="cd-ttl-label">{{ $t('cache.ttlRemaining') }}</span>
            </div>
            <div class="cd-ttl-divider"></div>
            <div class="cd-ttl-item">
              <span class="cd-ttl-num cd-ttl-num--muted">{{ detailRecord.ttlOriginal }}<span class="cd-ttl-unit">{{ $t('cache.secondsUnit') }}</span></span>
              <span class="cd-ttl-label">{{ $t('cache.ttlOriginal') }}</span>
            </div>
          </div>
          <div class="cd-ttl-bar-wrap">
            <div
              class="cd-ttl-bar"
              :class="{ 'cd-ttl-bar--warn': isTtlWarning(detailRecord.ttlRemaining) }"
              :style="{ width: `${Math.min(100, Math.round(detailRecord.ttlRemaining / (detailRecord.ttlOriginal || 1) * 100))}%` }"
            ></div>
          </div>
          <div class="cd-ttl-bar-label">{{ $t('cache.remainingPct', { n: Math.min(100, Math.round(detailRecord.ttlRemaining / (detailRecord.ttlOriginal || 1) * 100)) }) }}</div>
        </div>
        <div class="cd-section-title" style="margin-top:16px">{{ $t('cache.timeInfo') }}</div>
        <div class="cd-grid">
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.cachedTime') }}</span>
            <span class="mn-mono cd-val">{{ formatDateTime(detailRecord.cacheTime) }}</span>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.updatedTime') }}</span>
            <span class="mn-mono cd-val">{{ formatDateTime(detailRecord.updatedAt) }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cd-footer">
          <el-button type="danger" plain size="small" :loading="singleClearId === detailRecord?.id" :disabled="Boolean(singleClearId)" @click="detailRecord && clearSingleCache(detailRecord)">{{ $t('cache.clear') }}</el-button>
          <el-button size="small" @click="detailDialogVisible = false">{{ $t('cache.close') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <BaseModal
      v-model="ruleDialogVisible"
      class="cache-rule-modal"
      :title="ruleModalTitle"
      :width="520"
      :loading="submitting"
      :confirm-disabled="submitting"
      :confirm-text="$t('common.save')"
      :cancel-text="$t('common.cancel')"
      @confirm="saveDomainRule"
      @cancel="closeRuleDialog"
      @close="closeRuleDialog"
      @closed="resetRuleForm"
    >
      <el-form ref="ruleFormRef" :model="domainRuleForm" :rules="domainRuleRules" label-position="top" class="cache-rule-form">
        <el-form-item :label="$t('cache.domain')" prop="domain">
          <el-input v-model="domainRuleForm.domain" :placeholder="$t('cache.domainPlaceholder')" @input="handleRuleDomainInput" @blur="handleRuleDomainInput" />
        </el-form-item>
        <el-form-item :label="$t('cache.customTtlLabel')" prop="customTtl">
          <el-input-number v-model="domainRuleForm.customTtl" :min="1" :max="86400" controls-position="right" style="width: 100%" @change="debounceAdjustRuleTtl" />
        </el-form-item>
        <el-form-item :label="$t('cache.customRetainLabel')" prop="customRetain">
          <el-input-number v-model="domainRuleForm.customRetain" :min="1" :max="86400" controls-position="right" style="width: 100%" @change="debounceAdjustRuleRetain" />
        </el-form-item>
        <el-form-item :label="$t('common.status')" class="cache-rule-status-item">
          <el-switch v-model="domainRuleForm.status" active-value="启用" inactive-value="禁用" :active-text="$t('common.enabled')" :inactive-text="$t('common.disabled')" />
        </el-form-item>
      </el-form>
    </BaseModal>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.cache-last-updated {
  font-size: 12px;
  color: var(--app-text-regular);
}

.cache-strategy-stack { gap: 16px; }

/* ═══════════════ Tabs ═══════════════ */
.mn-tabs-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.mn-tabs-card :deep(.el-card__body) { padding: 0; }

.cache-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.cache-tabs :deep(.el-tabs__item) {
  font-size: 13px;
  color: var(--app-text-regular);
  height: 44px;
  line-height: 44px;
}

.cache-tabs :deep(.el-tabs__item.is-active) {
  color: var(--app-accent);
  font-weight: 600;
}

.cache-tabs :deep(.el-tabs__active-bar) {
  background: var(--app-accent);
  height: 2px;
}

.cache-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }

.cache-tabs :deep(.el-tabs__content) { padding: 0; }

.mn-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.mn-tab-icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}

.mn-tab-pane-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
}

/* ═══════════════ Cards ═══════════════ */
.mn-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.mn-card :deep(.el-card__body) { padding: 0; }

.cache-strategy-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.cache-strategy-card :deep(.el-card__header) {
  padding: 13px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.cache-strategy-card :deep(.el-card__body) { padding: 20px; }

.table-card :deep(.el-card__body) { padding: 0; }

/* ═══════════════ Panel header ═══════════════ */
.mn-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}

.mn-panel-icon {
  width: 15px;
  height: 15px;
  color: var(--app-accent);
  flex-shrink: 0;
}

.mn-panel-icon--danger { color: var(--app-danger); }

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

/* ═══════════════ Forms ═══════════════ */
.cache-strategy-form { row-gap: 12px; }
.cache-strategy-col { margin-bottom: 0; }

.cache-auto-cleanup-wrap {
  grid-column: span 12;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  min-height: 28px;
}

.cache-auto-cleanup-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

.cache-strategy-actions {
  justify-content: flex-start;
  gap: 8px;
  margin-top: 12px;
  display: flex;
}

.cache-manual-clear-form { row-gap: 12px; }

.cache-manual-clear-actions {
  display: flex;
  justify-content: flex-start;
  margin-top: 12px;
}

/* ═══════════════ Table ═══════════════ */
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.mn-table :deep(.cache-row-selected > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.08) !important;
}

.mn-table :deep(.el-table__indent) { width: 16px; }

.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.inline-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.term-help-icon {
  color: var(--app-text-regular);
  cursor: help;
  font-size: 13px;
  margin-left: 2px;
}

.cache-ttl-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ttl-line {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-main);
}

.ttl-line--secondary { color: var(--app-text-regular); }

.ttl-line--warn {
  color: var(--app-warning);
  font-weight: 600;
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
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
  font-size: 12px;
  font-weight: 500;
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

.mn-badge--warning {
  color: var(--app-warning);
  background: rgba(255, 125, 0, 0.08);
  border-color: rgba(255, 125, 0, 0.22);
}

/* ═══════════════ Mono / time ═══════════════ */
.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}

.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}

/* ═══════════════ Rule modal ═══════════════ */
:deep(.cache-rule-modal .el-dialog__body) { padding: 20px; }

.cache-rule-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.cache-rule-form :deep(.el-form-item) { margin-bottom: 0; }

.cache-rule-form :deep(.el-form-item__label) {
  justify-content: flex-start;
  padding-bottom: 6px;
  color: var(--app-text-regular);
}

.cache-rule-form :deep(.el-form-item.is-required .el-form-item__label::before) {
  color: var(--app-danger);
}

.cache-rule-form :deep(.el-form-item__error) { font-size: 12px; }

.cache-rule-status-item :deep(.el-form-item__content) { justify-content: flex-end; }

/* ═══════════════ Detail dialog ═══════════════ */
:deep(.cache-detail-dialog .el-dialog) {
  border-radius: 12px;
  overflow: hidden;
  animation: cacheDialogIn 0.2s ease-out;
}

:deep(.cache-detail-dialog .el-dialog__header) {
  margin-right: 0;
  padding: 0;
  border-bottom: 1px solid var(--app-border);
}

:deep(.cache-detail-dialog .el-dialog__body) { padding: 0; }

:deep(.cache-detail-dialog .el-dialog__footer) {
  padding: 10px 20px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

.cd-header {
  padding: 16px 20px 14px;
  background: linear-gradient(135deg, rgba(22,93,255,0.04) 0%, transparent 60%);
}

.cd-header-main {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.cd-domain {
  font-size: 16px;
  font-weight: 700;
  color: var(--app-text-primary, #1d2129);
  word-break: break-all;
}

.cd-type-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.04em;
  flex-shrink: 0;
}
.cd-type-badge--a    { background: #10b981; }
.cd-type-badge--aaaa { background: #3b82f6; }
.cd-type-badge--cname{ background: #8b5cf6; }
.cd-type-badge--mx   { background: #f59e0b; }
.cd-type-badge--txt  { background: #6b7280; }
.cd-type-badge--ns   { background: #0ea5e9; }
.cd-type-badge--srv  { background: #ec4899; }
.cd-type-badge--caa  { background: #ef4444; }

.cd-header-sub {
  margin-top: 4px;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.cd-body { padding: 16px 20px 20px; }

.cd-section-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border);
}

.cd-grid { display: flex; flex-direction: column; }

.cd-row {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  align-items: center;
  min-height: 36px;
  gap: 12px;
  border-bottom: 1px solid rgba(0,0,0,0.04);
}

.cd-row:last-child { border-bottom: none; }

.cd-label {
  font-size: 12px;
  color: var(--app-text-secondary);
  flex-shrink: 0;
}

.cd-val {
  font-size: 13px;
  color: var(--app-text-primary, #1d2129);
  word-break: break-all;
}

.cd-val--with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cd-record {
  flex: 1;
  min-width: 0;
  word-break: break-all;
  font-size: 12px;
}

.cd-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid rgba(22,93,255,0.25);
  background: rgba(22,93,255,0.05);
  color: var(--app-accent);
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 11px;
  white-space: nowrap;
  flex-shrink: 0;
  transition: background 0.15s;
}

.cd-copy-btn:hover { background: rgba(22,93,255,0.12); }

.cd-ttl-block {
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 12px 16px;
}

.cd-ttl-nums {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 10px;
}

.cd-ttl-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.cd-ttl-num {
  font-size: 22px;
  font-weight: 700;
  font-family: var(--app-font-mono, monospace);
  color: var(--app-accent);
  line-height: 1;
}

.cd-ttl-unit {
  font-size: 11px;
  font-weight: 400;
  margin-left: 2px;
  color: var(--app-text-secondary);
}

.cd-ttl-num--muted { color: var(--app-text-secondary); }
.cd-ttl-warn { color: var(--app-danger) !important; }

.cd-ttl-label {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.cd-ttl-divider {
  width: 1px;
  height: 32px;
  background: var(--app-border);
  flex-shrink: 0;
}

.cd-ttl-bar-wrap {
  height: 6px;
  border-radius: 3px;
  background: var(--app-border);
  overflow: hidden;
}

.cd-ttl-bar {
  height: 100%;
  border-radius: 3px;
  background: var(--app-accent);
  transition: width 0.4s ease;
}

.cd-ttl-bar--warn { background: var(--app-danger); }

.cd-ttl-bar-label {
  margin-top: 5px;
  font-size: 11px;
  color: var(--app-text-secondary);
  text-align: right;
}

.cd-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

@keyframes cacheDialogIn {
  from { opacity: 0; transform: translateY(8px); }
  to   { opacity: 1; transform: translateY(0); }
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1200px) {
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { justify-content: flex-start; }
}

/* ═══════════════ TTL human unit ═══════════════ */
.ttl-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.ttl-human-badge {
  font-size: 12px;
  color: var(--app-accent);
  white-space: nowrap;
  font-weight: 600;
  min-width: 64px;
}

.ttl-val-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 1px;
}

.ttl-human-label {
  font-size: 11px;
  color: var(--app-text-secondary);
}

/* ═══════════════ Batch bar ═══════════════ */
.strategy-batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background: rgba(22,93,255,0.05);
  border: 1px solid rgba(22,93,255,0.18);
  border-radius: 6px;
  margin-bottom: 8px;
}

.strategy-batch-info {
  font-size: 12px;
  color: var(--app-accent);
  font-weight: 600;
  flex: 1;
}

/* ═══════════════ Hit count ═══════════════ */
.hit-count {
  color: var(--app-accent);
  font-weight: 600;
}

/* ═══════════════ Clear preview dialog ═══════════════ */
.clear-preview-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 16px 0 8px;
  gap: 10px;
  text-align: center;
}

.clear-preview-icon { color: var(--app-warning); }

.clear-preview-text {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-primary, #1d2129);
  margin: 0;
}

.clear-preview-sub {
  font-size: 12px;
  color: var(--app-text-secondary);
  margin: 0;
  max-width: 280px;
}

/* ═══════════════ Clear history ═══════════════ */
.clear-history-list {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.clear-history-row {
  display: grid;
  grid-template-columns: 180px 1fr auto;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid rgba(0,0,0,0.04);
  font-size: 12px;
}

.clear-history-row:last-child { border-bottom: none; }

.clear-history-time { color: var(--app-text-secondary); }

.clear-history-scope {
  color: var(--app-text-primary, #1d2129);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.clear-history-count {
  color: var(--app-danger);
  font-weight: 600;
  white-space: nowrap;
}
</style>
