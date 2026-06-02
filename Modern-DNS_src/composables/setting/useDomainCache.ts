import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useCacheStore } from '../../stores/cache'
import type { CacheNode } from '../../types/modules'
import { loadTableState, saveTableState } from '../../utils/tableState'

type SortOrder = 'ascending' | 'descending' | null

type CacheFilters = {
  keyword: string
  recordType: string
  persistStatus: string
  timeRange: string[]
}

type CachePager = {
  page: number
  size: number
}

type CacheSorter = {
  prop: keyof CacheNode
  order: Exclude<SortOrder, null>
}

type CacheViewState = {
  filters: CacheFilters
  pager: CachePager
  sorter: CacheSorter
  selectedNodeId: number | null
}

const TABLE_STATE_KEY = 'modern-dns:cache-domain:table-state'

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

const sortTree = (rows: CacheNode[], sorter: CacheSorter): CacheNode[] => {
  const next = [...rows]
  if (sorter.prop && sorter.order) {
    const factor = sorter.order === 'ascending' ? 1 : -1
    next.sort((left, right) => String(left[sorter.prop] || '').localeCompare(String(right[sorter.prop] || '')) * factor)
  }
  return next.map((item) => ({
    ...item,
    children: item.children?.length ? sortTree(item.children, sorter) : [],
  }))
}

const isCacheNode = (value: CacheNode | null): value is CacheNode => value !== null

export const useDomainCache = () => {
  const { t } = useI18n()
  const cacheStore = useCacheStore()
  const cachedState = loadTableState<CacheViewState>(TABLE_STATE_KEY, {
    filters: { keyword: '', recordType: '', persistStatus: '', timeRange: [] },
    pager: { page: 1, size: 10 },
    sorter: { prop: 'cacheTime', order: 'descending' },
    selectedNodeId: null,
  })

  const filters = reactive<CacheFilters>({
    keyword: cachedState.filters.keyword,
    recordType: cachedState.filters.recordType,
    persistStatus: cachedState.filters.persistStatus,
    timeRange: [...cachedState.filters.timeRange],
  })
  const pager = reactive<CachePager>({
    page: cachedState.pager.page,
    size: cachedState.pager.size,
  })
  const sorter = reactive<CacheSorter>({
    prop: cachedState.sorter.prop,
    order: cachedState.sorter.order,
  })
  const selectedNodeId = ref<number | null>(cachedState.selectedNodeId)
  const selectedRows = ref<CacheNode[]>([])
  const detailDialogVisible = ref(false)
  const detailRecord = ref<CacheNode | null>(null)
  const refreshLoading = ref(false)
  const batchClearLoading = ref(false)
  const singleClearId = ref<number | null>(null)
  const lastUpdated = computed(() => cacheStore.lastUpdated)
  const loading = computed(() => cacheStore.loading)
  const submitting = computed(() => cacheStore.submitting)

  const typeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA']

  const matchesFilter = (row: CacheNode): boolean => {
    const keyword = filters.keyword.trim()
    const keywordMatch = !keyword || row.domain.includes(keyword) || row.cacheId.includes(keyword)
    const typeMatch = !filters.recordType || row.recordType === filters.recordType
    const persistMatch = !filters.persistStatus || row.persisted === filters.persistStatus
    const timeMatch =
      !filters.timeRange.length ||
      (row.cacheTime >= filters.timeRange[0] && row.cacheTime <= filters.timeRange[1])
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
        .filter(isCacheNode)

  const filteredTree = computed<CacheNode[]>(() => filterTree(cacheStore.domainCacheTree))
  const sortedTree = computed<CacheNode[]>(() => sortTree(filteredTree.value, sorter))
  const selectedTreeNode = computed<CacheNode | null>(() => {
    if (!selectedNodeId.value) {
      return null
    }
    return findNodeById(sortedTree.value, selectedNodeId.value)
  })
  const linkedRows = computed<CacheNode[]>(() => {
    if (!selectedTreeNode.value) {
      return flattenTree(sortedTree.value)
    }
    return flattenTree([selectedTreeNode.value])
  })
  const pagedRows = computed<CacheNode[]>(() => {
    const start = (pager.page - 1) * pager.size
    return linkedRows.value.slice(start, start + pager.size)
  })

  watch(
    () => ({
      filters: { ...filters, timeRange: [...filters.timeRange] },
      pager: { ...pager },
      sorter: { ...sorter },
      selectedNodeId: selectedNodeId.value,
    }),
    (next) => saveTableState(TABLE_STATE_KEY, next),
    { deep: true },
  )

  watch(
    () => filters,
    () => {
      pager.page = 1
    },
    { deep: true },
  )

  watch(sortedTree, (rows) => {
    if (selectedNodeId.value && !findNodeById(rows, selectedNodeId.value)) {
      selectedNodeId.value = null
    }
  })

  const refreshData = async (): Promise<void> => {
    if (refreshLoading.value) {
      return
    }
    refreshLoading.value = true
    try {
      await cacheStore.fetchCacheData()
      ElMessage.success(t('cache.refreshed'))
    } finally {
      refreshLoading.value = false
    }
  }

  const selectTreeNode = (node: CacheNode | null): void => {
    selectedNodeId.value = node?.id ?? null
    pager.page = 1
  }

  const resetSelectedTreeNode = (): void => {
    selectedNodeId.value = null
    pager.page = 1
  }

  const handleSortChange = ({ prop, order }: { prop: string | null; order: SortOrder }): void => {
    sorter.prop = (prop as keyof CacheNode) || 'cacheTime'
    sorter.order = order || 'descending'
  }

  const handlePageChange = (page: number): void => {
    pager.page = page
  }

  const handlePageSizeChange = (size: number): void => {
    pager.page = 1
    pager.size = size
  }

  const handleSelectionChange = (rows: CacheNode[]): void => {
    selectedRows.value = rows
  }

  const openDetail = (row: CacheNode): void => {
    detailRecord.value = row
    detailDialogVisible.value = true
  }

  const copyRecordValue = async (): Promise<void> => {
    if (!detailRecord.value?.recordValue) {
      return
    }
    try {
      await navigator.clipboard.writeText(detailRecord.value.recordValue)
      ElMessage.success(t('cache.recordCopied'))
    } catch (_error) {
      ElMessage.warning(t('cache.copyFailed'))
    }
  }

  const clearSingleCache = async (row: CacheNode): Promise<void> => {
    if (singleClearId.value || cacheStore.submitting) {
      return
    }
    singleClearId.value = row.id
    try {
      const count = await cacheStore.clearByIds([row.id])
      await cacheStore.fetchCacheData()
      selectedRows.value = selectedRows.value.filter((item) => item.id !== row.id)
      ElMessage.success(t('cache.clearedSuccess', { n: count }))
    } finally {
      singleClearId.value = null
    }
  }

  const batchClearCache = async (): Promise<void> => {
    if (batchClearLoading.value || cacheStore.submitting || !selectedRows.value.length) {
      return
    }
    batchClearLoading.value = true
    try {
      const count = await cacheStore.clearByIds(selectedRows.value.map((item) => item.id))
      await cacheStore.fetchCacheData()
      selectedRows.value = []
      ElMessage.success(t('cache.batchClearedSuccess', { n: count }))
    } finally {
      batchClearLoading.value = false
    }
  }

  const formatDateTime = (value: string): string => {
    if (!value) {
      return t('common.none')
    }
    return value.replace('T', ' ').slice(0, 19)
  }

  const isTtlWarning = (value: number | string): boolean => Number(value) > 0 && Number(value) < 30

  const sourceTagType = (value: string): 'success' | 'warning' | 'info' => {
    if (value === '本地配置' || value.toLowerCase() === 'local config') {
      return 'success'
    }
    if (value === '递归解析' || value.toLowerCase() === 'recursive resolve') {
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

  const persistTagType = (value: string): 'success' | 'info' =>
    (value === '已持久化' || value.toLowerCase() === 'persisted' ? 'success' : 'info')

  onMounted(async () => {
    await cacheStore.fetchCacheData()
  })

  return {
    loading,
    submitting,
    refreshLoading,
    batchClearLoading,
    singleClearId,
    lastUpdated,
    typeOptions,
    filters,
    sorter,
    pager,
    selectedRows,
    selectedNodeId,
    selectedTreeNode,
    sortedTree,
    linkedRows,
    pagedRows,
    detailDialogVisible,
    detailRecord,
    refreshData,
    selectTreeNode,
    resetSelectedTreeNode,
    handleSortChange,
    handlePageChange,
    handlePageSizeChange,
    handleSelectionChange,
    openDetail,
    copyRecordValue,
    clearSingleCache,
    batchClearCache,
    formatDateTime,
    isTtlWarning,
    sourceTagType,
    typeTagType,
    persistTagType,
  }
}

export default useDomainCache