import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { ElMessage, ElLoading } from 'element-plus'
import * as XLSX from 'xlsx'
import { useI18n } from 'vue-i18n'
import { useMonitorStore } from '../../stores/monitor'
import type { MonitorResolveLogRow } from '../../types/modules'
import { loadTableState, saveTableState } from '../../utils/tableState'

type SortOrder = 'ascending' | 'descending' | null

type SortState = {
  prop: string
  order: SortOrder
}

type ResolveLogViewState = {
  filters: ResolveLogFilterForm
  pagination: ResolveLogPagination
  sorter: SortState
}

export interface ResolveLogFilterForm {
  keyword: string
  recordType: string
  rcode: string
  timePreset: string
  timeRange: string[]
}

export interface ResolveLogPagination {
  page: number
  size: number
  total: number
}

const RESOLVE_LOG_STATE_KEY = 'modern-dns:resolve-log:view-state'

const DEFAULT_FILTERS: ResolveLogFilterForm = {
  keyword: '',
  recordType: '',
  rcode: '',
  timePreset: '24h',
  timeRange: [],
}

const parseDate = (value: string | number | Date | null | undefined): number => {
  const normalized = String(value || '').trim().replace(' ', 'T')
  const timestamp = Date.parse(normalized)
  return Number.isNaN(timestamp) ? Date.now() : timestamp
}

const isWithinTime = (value: string, preset: string, range: string[]): boolean => {
  const ts = parseDate(value)
  const now = Date.now()
  const presetMap: Record<string, number> = {
    '1h': 60 * 60 * 1000,
    '6h': 6 * 60 * 60 * 1000,
    '24h': 24 * 60 * 60 * 1000,
    '7d': 7 * 24 * 60 * 60 * 1000,
  }

  if (preset !== 'custom') {
    return now - ts <= (presetMap[preset] || presetMap['24h'])
  }

  if (range.length === 2) {
    const start = parseDate(range[0])
    const end = parseDate(range[1])
    return ts >= start && ts <= end
  }

  return true
}

const copyText = async (text: string, success: string): Promise<void> => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(success)
  } catch (_error) {
    const textarea = document.createElement('textarea')
    textarea.value = text
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    ElMessage.success(success)
  }
}

const exportRows = (
  rows: Array<Record<string, unknown>>,
  baseName: string,
  format: 'excel' | 'csv' | 'json',
  t: (key: string, params?: Record<string, unknown>) => string,
): void => {
  if (!rows.length) {
    ElMessage.warning(t('monitor.noExportData'))
    return
  }
  const loading = ElLoading.service({ text: t('monitor.resolveLogExporting', { format: format.toUpperCase() }), background: 'rgba(0,0,0,0.35)' })
  try {
    if (format === 'json') {
      const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json;charset=utf-8' })
      const href = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = href
      anchor.download = `${baseName}.json`
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(href)
      return
    }

    if (format === 'csv') {
      const columns = Object.keys(rows[0])
      const csv = [
        columns.join(','),
        ...rows.map((row) => columns.map((key) => `"${String(row[key] ?? '').replace(/"/g, '""')}"`).join(',')),
      ].join('\n')
      const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' })
      const href = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = href
      anchor.download = `${baseName}.csv`
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(href)
      return
    }

    const worksheet = XLSX.utils.json_to_sheet(rows)
    const workbook = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(workbook, worksheet, 'Export')
    XLSX.writeFile(workbook, `${baseName}.xlsx`)
  } finally {
    loading.close()
  }
}

export const useResolveLog = () => {
  const { t } = useI18n()
  const monitorStore = useMonitorStore()
  const cachedState = loadTableState<ResolveLogViewState>(RESOLVE_LOG_STATE_KEY, {
    filters: DEFAULT_FILTERS,
    pagination: { page: 1, size: 10, total: 0 },
    sorter: { prop: 'time', order: 'descending' },
  })

  const loading = computed(() => monitorStore.loading)
  const lastUpdated = computed(() => monitorStore.lastUpdated)
  const filterForm = reactive<ResolveLogFilterForm>({ ...DEFAULT_FILTERS, ...cachedState.filters, timeRange: [...(cachedState.filters.timeRange || [])] })
  const appliedFilters = reactive<ResolveLogFilterForm>({ ...filterForm, timeRange: [...filterForm.timeRange] })
  const pagination = reactive<ResolveLogPagination>({
    page: cachedState.pagination.page || 1,
    size: cachedState.pagination.size || 10,
    total: 0,
  })
  const sortState = reactive<SortState>({
    prop: cachedState.sorter.prop || 'time',
    order: cachedState.sorter.order || 'descending',
  })
  const querying = ref(false)
  const refreshing = ref(false)
  const exporting = ref(false)
  const detailVisible = ref(false)
  const currentDetail = ref<MonitorResolveLogRow | null>(null)

  watch(
    () => ({
      filters: { ...appliedFilters, timeRange: [...appliedFilters.timeRange] },
      pagination: { page: pagination.page, size: pagination.size, total: pagination.total },
      sorter: { ...sortState },
    }),
    (next) => {
      saveTableState(RESOLVE_LOG_STATE_KEY, next)
    },
    { deep: true },
  )

  const filteredRows = computed<MonitorResolveLogRow[]>(() =>
    monitorStore.resolveLogs.filter((item) => {
      const keyword = appliedFilters.keyword.trim().toLowerCase()
      const keywordMatch =
        !keyword ||
        [item.domain, item.transactionId, item.sourceIp]
          .some((field) => String(field || '').toLowerCase().includes(keyword))
      const typeMatch = !appliedFilters.recordType || item.recordType === appliedFilters.recordType
      const codeMatch = !appliedFilters.rcode || item.rcode === appliedFilters.rcode
      const timeMatch = isWithinTime(String(item.time || ''), appliedFilters.timePreset, appliedFilters.timeRange)
      return keywordMatch && typeMatch && codeMatch && timeMatch
    }),
  )

  const sortedRows = computed<MonitorResolveLogRow[]>(() => {
    const rows = [...filteredRows.value]
    if (!sortState.prop || !sortState.order) {
      return rows
    }
    const factor = sortState.order === 'ascending' ? 1 : -1
    return rows.sort((left, right) => String(left[sortState.prop] || '').localeCompare(String(right[sortState.prop] || '')) * factor)
  })

  const resolveRows = computed<MonitorResolveLogRow[]>(() => {
    const start = (pagination.page - 1) * pagination.size
    return sortedRows.value.slice(start, start + pagination.size)
  })

  watch(
    sortedRows,
    (rows) => {
      pagination.total = rows.length
      const totalPages = Math.max(1, Math.ceil(rows.length / pagination.size))
      if (pagination.page > totalPages) {
        pagination.page = totalPages
      }
    },
    { immediate: true },
  )

  const fetchResolveLogs = async (): Promise<void> => {
    try {
      await monitorStore.fetchMonitorData()
    } catch (_error) {
      ElMessage.error(t('monitor.resolveLogLoadFailed'))
    }
  }

  const handleSearch = (): void => {
    querying.value = true
    Object.assign(appliedFilters, {
      ...filterForm,
      timeRange: [...filterForm.timeRange],
    })
    pagination.page = 1
    querying.value = false
    ElMessage.success(t('monitor.resolveLogSearchApplied'))
  }

  const handleReset = (): void => {
    Object.assign(filterForm, { ...DEFAULT_FILTERS, timeRange: [] })
    Object.assign(appliedFilters, { ...DEFAULT_FILTERS, timeRange: [] })
    pagination.page = 1
    ElMessage.success(t('monitor.resolveLogResetApplied'))
  }

  const handleRefresh = async (): Promise<void> => {
    if (refreshing.value) {
      return
    }
    try {
      refreshing.value = true
      await monitorStore.fetchMonitorData()
      ElMessage.success(t('monitor.resolveLogRefreshed'))
    } catch (_error) {
      ElMessage.error(t('monitor.refreshFailedRetry'))
    } finally {
      refreshing.value = false
    }
  }

  const handlePageChange = (page: number, size = pagination.size): void => {
    pagination.page = page
    pagination.size = size
  }

  const handleSortChange = ({ prop, order }: { prop: string | null; order: SortOrder }): void => {
    sortState.prop = prop || 'time'
    sortState.order = order || 'descending'
  }

  const openDetail = (row: MonitorResolveLogRow): void => {
    currentDetail.value = row
    detailVisible.value = true
  }

  const copyDetailSummary = async (): Promise<void> => {
    if (!currentDetail.value) {
      return
    }
    const lines = [
      `${t('monitor.resolveLogFieldLogId')}: ${currentDetail.value.logId}`,
      `${t('monitor.resolveLogFieldDomain')}: ${currentDetail.value.domain}`,
      `Transaction ID: ${currentDetail.value.transactionId}`,
      `${t('monitor.resolveLogFieldRcode')}: ${currentDetail.value.rcode}`,
      `${t('monitor.resolveLogFieldRequestHeaders')}: ${currentDetail.value.requestHeaders}`,
      `${t('monitor.resolveLogFieldResponseHeaders')}: ${currentDetail.value.responseHeaders}`,
      `${t('monitor.resolveLogFieldRequestContent')}: ${currentDetail.value.requestPayload}`,
      `${t('monitor.resolveLogFieldResponseContent')}: ${currentDetail.value.responsePayload}`,
    ]
    await copyText(lines.join('\n'), t('monitor.resolveLogSummaryCopied'))
  }

  const copyDetailText = async (text: string, success = t('common.copied')): Promise<void> => {
    await copyText(text || '', success)
  }

  const handleExport = async (command: string): Promise<void> => {
    if (exporting.value) {
      return
    }

    try {
      exporting.value = true
      const [scope, format] = command.split('-') as [string, string]
      const sourceRows = scope === 'all' ? monitorStore.resolveLogs : filteredRows.value
      const rows = sourceRows.map((item) => ({
        [t('monitor.resolveLogFieldLogId')]: item.logId,
        [t('monitor.resolveLogFieldTime')]: formatDateTimeMs(String(item.time || '')),
        [t('monitor.resolveLogFieldDomain')]: item.domain,
        [t('monitor.resolveLogFieldRecordType')]: item.recordType,
        TransactionID: item.transactionId,
        [t('monitor.resolveLogFieldRcode')]: item.rcode,
        [t('monitor.resolveLogFieldSourceIp')]: item.sourceIp,
        [t('monitor.resolveLogFieldResponseTimeMs')]: item.responseTime,
        [t('monitor.resolveLogFieldRequestHeaders')]: item.requestHeaders,
        [t('monitor.resolveLogFieldResponseHeaders')]: item.responseHeaders,
      }))

      if (format === 'batch') {
        exportRows(rows, 'monitor-resolve-logs', 'excel', t)
        exportRows(rows, 'monitor-resolve-logs', 'csv', t)
        ElMessage.success(t('monitor.resolveLogExportBatch'))
        return
      }

      exportRows(rows, 'monitor-resolve-logs', format as 'excel' | 'csv' | 'json', t)
      ElMessage.success(t('monitor.resolveLogExportSuccess'))
    } catch (_error) {
      ElMessage.error(t('monitor.resolveLogExportFailed'))
    } finally {
      exporting.value = false
    }
  }

  const formatDateTimeMs = (value: string): string => {
    const date = new Date(String(value || '').replace(' ', 'T'))
    if (Number.isNaN(date.getTime())) {
      return String(value || '--')
    }
    const y = date.getFullYear()
    const m = String(date.getMonth() + 1).padStart(2, '0')
    const d = String(date.getDate()).padStart(2, '0')
    const h = String(date.getHours()).padStart(2, '0')
    const mm = String(date.getMinutes()).padStart(2, '0')
    const s = String(date.getSeconds()).padStart(2, '0')
    const ms = String(date.getMilliseconds()).padStart(3, '0')
    return `${y}-${m}-${d} ${h}:${mm}:${s}.${ms}`
  }

  const statusClass = (status: string): string => {
    if (status === 'NOERROR' || status === '成功' || status === '已处理') {
      return 'tag-success'
    }
    if (status === 'NXDOMAIN' || status === '警告' || status === '处理中') {
      return 'tag-warning'
    }
    if (status === 'SERVFAIL' || status === 'REFUSED' || status === '失败' || status === '未处理') {
      return 'tag-danger'
    }
    return 'tag-muted'
  }

  const rowClassName = ({ row }: { row: MonitorResolveLogRow }): string =>
    row.rcode === 'SERVFAIL' || row.rcode === 'REFUSED' ? 'monitor-row-danger' : ''

  onScopeDispose(() => {
    querying.value = false
  })

  return {
    filterForm,
    pagination,
    loading,
    querying,
    refreshing,
    exporting,
    detailVisible,
    currentDetail,
    lastUpdated,
    resolveRows,
    totalRows: computed(() => sortedRows.value.length),
    fetchResolveLogs,
    handleSearch,
    handleReset,
    handleRefresh,
    handlePageChange,
    handleSortChange,
    handleExport,
    openDetail,
    copyDetailSummary,
    copyDetailText,
    formatDateTimeMs,
    statusClass,
    rowClassName,
  }
}

export default useResolveLog