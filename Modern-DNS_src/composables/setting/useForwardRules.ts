import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { UploadRawFile } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  batchDeleteConditionRulesApi,
  batchUpdateConditionRuleStatusApi,
  deleteConditionRuleApi,
  getConditionRuleList,
  reorderConditionRulesApi,
  saveConditionRuleApi,
  type ForwardRuleQuery,
  type ForwardRuleSortOrder,
} from '../../api/forward'
import { useCrud } from '../core/useCrud'
import { useForwardStore } from '../../stores/forward'
import type { ForwardRule } from '../../types/modules'
import { confirmRiskAction, normalizeMultiValue } from '../../utils/interaction'
import { loadTableState, saveTableState } from '../../utils/tableState'

type RuleBatchAction = 'enable' | 'disable' | 'delete'

type ForwardRuleFilters = {
  keyword: string
  status: string
  timeRange: string[]
  sortProp: keyof ForwardRule | ''
  sortOrder: ForwardRuleSortOrder
}

type ForwardRuleFormModel = {
  id?: number | null
  domains: string
  upstreamId: number
  priority: number
  remark: string
  status: string
}

type ForwardRuleStateCache = {
  filters: ForwardRuleFilters
  pagination: {
    currentPage: number
    pageSize: number
  }
}

const TABLE_STATE_KEY = 'modern-dns:forward-rule:table-state'

const createForwardRuleFormData = (): ForwardRuleFormModel => ({
  id: null,
  domains: '',
  upstreamId: 0,
  priority: 10,
  remark: '',
  status: '启用',
})

const normalizeDomains = (value: string): string => {
  const tokens = normalizeMultiValue(value)
    .split(',')
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean)

  return Array.from(new Set(tokens)).join(',')
}

const clampPriority = (value: number | string | undefined): number => {
  const next = Number(value || 1)
  if (!Number.isFinite(next)) {
    return 1
  }
  return Math.max(1, Math.min(100, Math.round(next)))
}

export const useForwardRules = () => {
  const { t } = useI18n()
  const forwardStore = useForwardStore()
  const cachedState = loadTableState<ForwardRuleStateCache>(TABLE_STATE_KEY, {
    filters: {
      keyword: '',
      status: '',
      timeRange: [],
      sortProp: 'priority',
      sortOrder: 'ascending',
    },
    pagination: {
      currentPage: 1,
      pageSize: 10,
    },
  })

  const searchForm = ref<ForwardRuleFilters>({
    keyword: cachedState.filters.keyword,
    status: cachedState.filters.status,
    timeRange: [...cachedState.filters.timeRange],
    sortProp: cachedState.filters.sortProp,
    sortOrder: cachedState.filters.sortOrder,
  })
  const selectedRules = ref<ForwardRule[]>([])
  const refreshLoading = ref(false)
  const deletingRuleId = ref<number | null>(null)
  const switchingRuleId = ref<number | null>(null)
  const batchRuleLoading = ref(false)
  const ruleExportLoading = ref(false)
  const ruleImportLoading = ref(false)
  const reorderLoading = ref(false)
  const domainInputTimer = ref<number | null>(null)
  const priorityInputTimer = ref<number | null>(null)
  const remarkInputTimer = ref<number | null>(null)

  const crud = useCrud<ForwardRule, ForwardRuleFilters, ForwardRuleFormModel, ForwardRuleFormModel, number>(
    {
      getList: (params) => getConditionRuleList(params as ForwardRuleQuery),
      create: (payload) => saveConditionRuleApi({
        domains: normalizeDomains(payload.domains),
        upstreamId: Number(payload.upstreamId),
        priority: clampPriority(payload.priority),
        remark: String(payload.remark || ''),
        status: String(payload.status || '启用'),
      }),
      update: (payload) => saveConditionRuleApi({
        id: Number(payload.id),
        domains: normalizeDomains(payload.domains),
        upstreamId: Number(payload.upstreamId),
        priority: clampPriority(payload.priority),
        remark: String(payload.remark || ''),
        status: String(payload.status || '启用'),
      }),
      delete: (id) => deleteConditionRuleApi(id),
    },
    searchForm,
    {
      initialPage: cachedState.pagination.currentPage,
      initialPageSize: cachedState.pagination.pageSize,
      createFormData: createForwardRuleFormData,
      getItemId: (item) => item.id as number | undefined,
      pageField: 'page',
      pageSizeField: 'size',
      autoFetch: false,
      watchQueryParams: false,
    },
  )

  const {
    loading,
    tableData,
    pagination,
    dialogVisible,
    formData,
    fetchData,
    handleCreate,
    handleEdit,
    handleDelete,
    handleSubmit,
  } = crud

  const lastUpdated = computed(() => forwardStore.lastUpdated)
  const upstreamOptions = computed(() =>
    forwardStore.globalConfig.servers.map((item) => ({
      value: item.id,
      label: `${item.name} / ${item.address}:${item.port}`,
      disabled: item.status !== '启用',
    })),
  )
  const ruleModalTitle = computed(() => (formData.value.id ? t('forward.editRule') : t('forward.addRule')))
  const canReorder = computed(() => tableData.value.length > 1)

  watch(
    () => ({
      filters: {
        ...searchForm.value,
        timeRange: [...searchForm.value.timeRange],
      },
      pagination: {
        currentPage: pagination.currentPage,
        pageSize: pagination.pageSize,
      },
    }),
    (next) => saveTableState(TABLE_STATE_KEY, next),
    { deep: true },
  )

  const refreshContext = async (): Promise<void> => {
    if (refreshLoading.value) {
      return
    }
    refreshLoading.value = true
    try {
      await forwardStore.fetchForwardData()
      await fetchData()
      selectedRules.value = []
    } finally {
      refreshLoading.value = false
    }
  }

  const getAllFilteredRules = async (): Promise<ForwardRule[]> => {
    const result = await getConditionRuleList({
      ...searchForm.value,
      page: 1,
      size: Math.max(200, pagination.total || 50),
    })
    return result.data.list || []
  }

  const openCreateDialog = (): void => {
    handleCreate()
    formData.value = createForwardRuleFormData()
  }

  const openEditDialog = (row: ForwardRule): void => {
    handleEdit(row)
    formData.value = {
      id: row.id,
      domains: normalizeDomains(row.domains),
      upstreamId: row.upstreamId,
      priority: row.priority,
      remark: row.remark,
      status: row.status,
    }
  }

  const closeDialog = (): void => {
    dialogVisible.value = false
  }

  const resetForm = (): void => {
    formData.value = createForwardRuleFormData()
  }

  const handleSearch = async (): Promise<void> => {
    pagination.currentPage = 1
    await fetchData({ currentPage: 1 })
  }

  const resetFilters = async (): Promise<void> => {
    searchForm.value.keyword = ''
    searchForm.value.status = ''
    searchForm.value.timeRange = []
    searchForm.value.sortProp = 'priority'
    searchForm.value.sortOrder = 'ascending'
    pagination.currentPage = 1
    await fetchData({ currentPage: 1 })
  }

  const handlePageChange = async (page: number): Promise<void> => {
    await fetchData({ currentPage: page })
  }

  const handlePageSizeChange = async (size: number): Promise<void> => {
    await fetchData({ currentPage: 1, pageSize: size })
  }

  const handleSelectionChange = (rows: ForwardRule[]): void => {
    selectedRules.value = rows
  }

  const handleSortChange = async ({ prop, order }: { prop: string | null; order: ForwardRuleSortOrder }): Promise<void> => {
    searchForm.value.sortProp = ((prop || 'priority') as keyof ForwardRule | '')
    searchForm.value.sortOrder = order || 'ascending'
    await fetchData({ currentPage: 1 })
  }

  const submitRule = async (): Promise<void> => {
    const isEdit = Boolean(formData.value.id)
    formData.value.domains = normalizeDomains(String(formData.value.domains || ''))
    formData.value.priority = clampPriority(formData.value.priority)
    await handleSubmit()
    await forwardStore.fetchForwardData()
    ElMessage.success(isEdit ? t('forward.ruleUpdatedDone') : t('forward.ruleCreatedDone'))
  }

  const removeRule = async (row: ForwardRule): Promise<void> => {
    deletingRuleId.value = row.id
    try {
      await handleDelete(row.id)
      await forwardStore.fetchForwardData()
      selectedRules.value = selectedRules.value.filter((item) => item.id !== row.id)
      ElMessage.success(t('forward.ruleDeletedDone'))
    } finally {
      deletingRuleId.value = null
    }
  }

  const toggleRuleStatus = async (row: ForwardRule, enabled: boolean): Promise<void> => {
    switchingRuleId.value = row.id
    try {
      await batchUpdateConditionRuleStatusApi({ ids: [row.id], status: enabled ? '启用' : '禁用' })
      await fetchData()
      ElMessage.success(t('forward.ruleStatusToggled', { status: enabled ? t('common.enabled') : t('common.disabled') }))
    } finally {
      switchingRuleId.value = null
    }
  }

  const handleBatchRuleAction = async (action: RuleBatchAction): Promise<void> => {
    if (!selectedRules.value.length) {
      ElMessage.warning(t('forward.selectAtLeastOneRule'))
      return
    }

    batchRuleLoading.value = true
    const ids = selectedRules.value.map((item) => item.id)
    try {
      if (action === 'enable') {
        await confirmRiskAction({ title: t('forward.batchEnableTitle'), action: t('forward.batchEnableAction'), risk: t('forward.batchRuleRisk') })
        await batchUpdateConditionRuleStatusApi({ ids, status: '启用' })
      }
      if (action === 'disable') {
        await confirmRiskAction({ title: t('forward.batchDisableTitle'), action: t('forward.batchDisableAction'), risk: t('forward.batchDisableRisk') })
        await batchUpdateConditionRuleStatusApi({ ids, status: '禁用' })
      }
      if (action === 'delete') {
        await confirmRiskAction({ title: t('forward.batchDeleteTitle'), action: t('forward.batchDeleteAction'), risk: t('forward.batchDeleteRisk') })
        await batchDeleteConditionRulesApi(ids)
      }
      selectedRules.value = []
      await forwardStore.fetchForwardData()
      await fetchData()
      const doneMap: Record<RuleBatchAction, string> = {
        enable: t('forward.batchEnableDone'),
        disable: t('forward.batchDisableDone'),
        delete: t('forward.batchDeleteDone'),
      }
      ElMessage.success(doneMap[action])
    } finally {
      batchRuleLoading.value = false
    }
  }

  const exportRules = async (type: 'json' | 'xlsx'): Promise<void> => {
    if (ruleExportLoading.value) {
      return
    }

    ruleExportLoading.value = true
    try {
      const rows = selectedRules.value.length ? selectedRules.value : await getAllFilteredRules()
      if (!rows.length) {
        ElMessage.warning(t('forward.noExportRules'))
        return
      }

      if (type === 'json') {
        const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json;charset=utf-8' })
        const url = URL.createObjectURL(blob)
        const link = document.createElement('a')
        link.href = url
        link.download = 'forward-rules.json'
        link.click()
        URL.revokeObjectURL(url)
      } else {
        const XLSX = await import('xlsx')
        const worksheet = XLSX.utils.json_to_sheet(rows)
        const workbook = XLSX.utils.book_new()
        XLSX.utils.book_append_sheet(workbook, worksheet, 'ForwardRules')
        XLSX.writeFile(workbook, 'forward-rules.xlsx')
      }
      ElMessage.success(t('forward.ruleExportDone'))
    } finally {
      ruleExportLoading.value = false
    }
  }

  const parseImportedRules = async (file: UploadRawFile): Promise<boolean> => {
    if (ruleImportLoading.value) {
      return false
    }

    ruleImportLoading.value = true
    try {
      const raw = await file.arrayBuffer()
      let rows: Array<Record<string, unknown>> = []

      if (file.name.endsWith('.json')) {
        rows = JSON.parse(new TextDecoder().decode(raw))
      } else {
        const XLSX = await import('xlsx')
        const workbook = XLSX.read(raw)
        const firstSheet = workbook.Sheets[workbook.SheetNames[0]]
        rows = XLSX.utils.sheet_to_json(firstSheet)
      }

      for (const item of rows) {
        const fallbackUpstreamId = forwardStore.globalConfig.servers[0]?.id || 0
        await saveConditionRuleApi({
          domains: normalizeDomains(String(item.domains || item['指定域名'] || '')),
          upstreamId: Number(item.upstreamId || item['上游服务器ID'] || fallbackUpstreamId),
          priority: clampPriority(Number(item.priority || item['优先级'] || 10)),
          remark: String(item.remark || item['备注'] || ''),
          status: String(item.status || item['状态'] || '启用'),
        })
      }

      await forwardStore.fetchForwardData()
      await fetchData({ currentPage: 1 })
      ElMessage.success(t('forward.ruleImportDone', { count: rows.length }))
    } finally {
      ruleImportLoading.value = false
    }

    return false
  }

  const reorderByIds = async (orderedIds: number[]): Promise<void> => {
    if (!orderedIds.length) {
      return
    }

    reorderLoading.value = true
    try {
      await reorderConditionRulesApi({ orderedIds })
      await forwardStore.fetchForwardData()
      await fetchData()
      ElMessage.success(t('forward.priorityOrderUpdated'))
    } finally {
      reorderLoading.value = false
    }
  }

  const moveRulePriority = async (row: ForwardRule, direction: -1 | 1): Promise<void> => {
    const rows = await getAllFilteredRules()
    const currentIndex = rows.findIndex((item) => item.id === row.id)
    const targetIndex = currentIndex + direction
    if (currentIndex < 0 || targetIndex < 0 || targetIndex >= rows.length) {
      return
    }
    const nextRows = rows.slice()
    const [current] = nextRows.splice(currentIndex, 1)
    nextRows.splice(targetIndex, 0, current)
    await reorderByIds(nextRows.map((item) => item.id))
  }

  const handlePriorityDragEnd = async (payload: { oldIndex?: number; newIndex?: number }): Promise<void> => {
    const oldIndex = Number(payload.oldIndex)
    const newIndex = Number(payload.newIndex)
    if (!Number.isInteger(oldIndex) || !Number.isInteger(newIndex) || oldIndex === newIndex) {
      return
    }

    const rows = await getAllFilteredRules()
    const globalStart = (pagination.currentPage - 1) * pagination.pageSize
    const sourceIndex = globalStart + oldIndex
    const targetIndex = globalStart + newIndex
    if (sourceIndex < 0 || targetIndex < 0 || sourceIndex >= rows.length || targetIndex >= rows.length) {
      return
    }
    const nextRows = rows.slice()
    const [current] = nextRows.splice(sourceIndex, 1)
    nextRows.splice(targetIndex, 0, current)
    await reorderByIds(nextRows.map((item) => item.id))
  }

  const handleRuleDomainsInput = (): void => {
    if (domainInputTimer.value) {
      window.clearTimeout(domainInputTimer.value)
    }
    domainInputTimer.value = window.setTimeout(() => {
      formData.value.domains = normalizeDomains(String(formData.value.domains || ''))
      domainInputTimer.value = null
    }, 300)
  }

  const handleRuleRemarkInput = (): void => {
    if (remarkInputTimer.value) {
      window.clearTimeout(remarkInputTimer.value)
    }
    remarkInputTimer.value = window.setTimeout(() => {
      formData.value.remark = String(formData.value.remark || '').slice(0, 100)
      remarkInputTimer.value = null
    }, 300)
  }

  const debounceAdjustRulePriority = (value: number | string | undefined): void => {
    if (priorityInputTimer.value) {
      window.clearTimeout(priorityInputTimer.value)
    }
    priorityInputTimer.value = window.setTimeout(() => {
      formData.value.priority = clampPriority(value)
      priorityInputTimer.value = null
    }, 200)
  }

  onMounted(async () => {
    await refreshContext()
  })

  onBeforeUnmount(() => {
    if (domainInputTimer.value) {
      window.clearTimeout(domainInputTimer.value)
    }
    if (priorityInputTimer.value) {
      window.clearTimeout(priorityInputTimer.value)
    }
    if (remarkInputTimer.value) {
      window.clearTimeout(remarkInputTimer.value)
    }
  })

  return {
    searchForm,
    loading,
    pagination,
    tableData,
    dialogVisible,
    formData,
    selectedRules,
    refreshLoading,
    deletingRuleId,
    switchingRuleId,
    batchRuleLoading,
    ruleExportLoading,
    ruleImportLoading,
    reorderLoading,
    canReorder,
    lastUpdated,
    upstreamOptions,
    ruleModalTitle,
    fetchData,
    refreshContext,
    openCreateDialog,
    openEditDialog,
    closeDialog,
    resetForm,
    handleSearch,
    resetFilters,
    handlePageChange,
    handlePageSizeChange,
    handleSelectionChange,
    handleSortChange,
    submitRule,
    removeRule,
    toggleRuleStatus,
    handleBatchRuleAction,
    exportRules,
    parseImportedRules,
    moveRulePriority,
    handlePriorityDragEnd,
    handleRuleDomainsInput,
    handleRuleRemarkInput,
    debounceAdjustRulePriority,
  }
}

export default useForwardRules