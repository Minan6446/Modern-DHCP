import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getMonitorModuleData, refreshMonitorRealtime } from '../../api/monitor'
import type { MonitorModuleData, MonitorRealTimeRow } from '../../types/modules'

type SortOrder = 'ascending' | 'descending' | null
type SortState = { prop: string; order: SortOrder }
type SortChangePayload = { prop: string | null; order: SortOrder }
type ExportFormat = 'csv' | 'json' | 'excel'
type ExportScope = 'all' | 'filtered'

type ChartPoint = { name: string; value: number }
type HeatmapData = {
  regions: string[]
  periods: string[]
  values: number[][]
}

const recordTypeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA']
const regionOptions = ['华东', '华北', '华南', '西南', '海外', '华中']
const statusOptions = ['成功', '失败', 'NXDOMAIN', 'SERVFAIL', 'REFUSED']
const refreshIntervalOptions = [5000, 10000, 30000, 60000]

const defaultFilterForm = () => ({
  domain: '',
  recordType: '',
  sourceIp: '',
  region: '',
  status: '',
  timePreset: '1h',
  timeRange: [] as string[],
})

const periodBuckets = ['00-06', '06-12', '12-18', '18-24']

const parseDate = (value: string | number | Date | null | undefined): number => {
  const normalized = String(value || '').trim().replace(' ', 'T')
  const timestamp = Date.parse(normalized)
  return Number.isNaN(timestamp) ? Date.now() : timestamp
}

const formatDateTime = (value: string): string => {
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
  return `${y}-${m}-${d} ${h}:${mm}:${s}`
}

const formatCompactNumber = (value: number): string => {
  const abs = Math.abs(value)
  if (abs >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (abs >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return `${value}`
}

const isWithinTime = (value: string, preset: string, range: string[]): boolean => {
  const ts = parseDate(value)
  const now = Date.now()
  const presetMap: Record<string, number> = {
    '1h': 60 * 60 * 1000,
    '6h': 6 * 60 * 60 * 1000,
    '24h': 24 * 60 * 60 * 1000,
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

const compareValues = (left: unknown, right: unknown): number => {
  const leftNumber = Number(left)
  const rightNumber = Number(right)
  if (Number.isFinite(leftNumber) && Number.isFinite(rightNumber)) {
    return leftNumber - rightNumber
  }

  const leftTime = parseDate(String(left || ''))
  const rightTime = parseDate(String(right || ''))
  if (!Number.isNaN(leftTime) && !Number.isNaN(rightTime) && String(left || '').includes('-')) {
    return leftTime - rightTime
  }

  return String(left || '').localeCompare(String(right || ''))
}

const saveFile = (name: string, content: string, type: string): void => {
  const blob = new Blob([content], { type })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(url)
}

const toExportRows = (rows: MonitorRealTimeRow[]) =>
  rows.map((item) => ({
    queryId: item.queryId,
    time: item.time,
    domain: item.domain,
    recordType: item.recordType,
    sourceIp: item.sourceIp,
    region: item.region,
    responseStatus: item.responseStatus,
    rcode: item.rcode,
    responseTime: item.responseTime,
    requestPayload: item.requestPayload,
    responsePayload: item.responsePayload,
  }))

export const useRealTimeMonitor = () => {
  const { t, locale } = useI18n()
  const filterForm = reactive(defaultFilterForm())
  const appliedFilters = reactive(defaultFilterForm())
  const pagination = reactive({ page: 1, size: 10 })
  const sortState = reactive<SortState>({ prop: 'time', order: 'descending' })

  const loading = ref(false)
  const queryLoading = ref(false)
  const refreshing = ref(false)
  const exportLoading = ref(false)
  const autoRefreshEnabled = ref(true)
  const refreshInterval = ref(5000)
  const lastUpdated = ref('')

  const detailVisible = ref(false)
  const detailRow = ref<MonitorRealTimeRow | null>(null)

  const rawTableData = ref<MonitorRealTimeRow[]>([])
  const timerId = ref<number | null>(null)
  let queryTimer: number | null = null

  const tableData = computed(() =>
    rawTableData.value.filter((item) => {
      const domainMatch = !appliedFilters.domain || item.domain.includes(appliedFilters.domain)
      const typeMatch = !appliedFilters.recordType || item.recordType === appliedFilters.recordType
      const ipMatch = !appliedFilters.sourceIp || item.sourceIp.includes(appliedFilters.sourceIp)
      const regionMatch = !appliedFilters.region || item.region === appliedFilters.region
      const statusMatch = !appliedFilters.status || item.responseStatus === appliedFilters.status
      const timeMatch = isWithinTime(item.time, appliedFilters.timePreset, appliedFilters.timeRange)
      return domainMatch && typeMatch && ipMatch && regionMatch && statusMatch && timeMatch
    }),
  )

  const sortedTableData = computed(() => {
    const rows = [...tableData.value]
    if (!sortState.prop || !sortState.order) {
      return rows
    }
    const factor = sortState.order === 'ascending' ? 1 : -1
    return rows.sort((left, right) => compareValues(left[sortState.prop], right[sortState.prop]) * factor)
  })

  const pagedTableData = computed(() => {
    const start = (pagination.page - 1) * pagination.size
    return sortedTableData.value.slice(start, start + pagination.size)
  })

  const chartData = computed(() => {
    const domainMap = new Map<string, number>()
    const ipMap = new Map<string, number>()
    const statusMap = new Map<string, number>()
    const heatmapMatrix = new Map<string, number>()
    const regions = new Set<string>()

    tableData.value.forEach((item) => {
      domainMap.set(item.domain, (domainMap.get(item.domain) || 0) + 1)
      ipMap.set(item.sourceIp, (ipMap.get(item.sourceIp) || 0) + 1)
      statusMap.set(item.responseStatus, (statusMap.get(item.responseStatus) || 0) + 1)
      regions.add(item.region)

      const hour = new Date(parseDate(item.time)).getHours()
      const periodIndex = hour < 6 ? 0 : hour < 12 ? 1 : hour < 18 ? 2 : 3
      const key = `${item.region}::${periodIndex}`
      heatmapMatrix.set(key, (heatmapMatrix.get(key) || 0) + 1)
    })

    const topDomain: ChartPoint[] = [...domainMap.entries()]
      .sort((left, right) => right[1] - left[1])
      .slice(0, 8)
      .map(([name, value]) => ({ name, value }))

    const topIp: ChartPoint[] = [...ipMap.entries()]
      .sort((left, right) => right[1] - left[1])
      .slice(0, 8)
      .map(([name, value]) => ({ name, value }))

    const statusDistribution: ChartPoint[] = [...statusMap.entries()]
      .sort((left, right) => right[1] - left[1])
      .map(([name, value]) => ({ name, value }))

    const regionList = [...regions]
    const heatmap: HeatmapData = {
      regions: regionList,
      periods: periodBuckets,
      values: regionList.flatMap((region, regionIndex) =>
        periodBuckets.map((_, periodIndex) => [periodIndex, regionIndex, heatmapMatrix.get(`${region}::${periodIndex}`) || 0]),
      ),
    }

    return {
      topDomain,
      topIp,
      statusDistribution,
      heatmap,
    }
  })

  const chartOptions = computed(() => ({
    topDomain: {
      grid: { left: 12, right: 12, top: 22, bottom: 12, containLabel: true },
      tooltip: { trigger: 'axis', confine: true, axisPointer: { type: 'shadow' } },
      xAxis: { type: 'value', splitLine: { show: false }, axisLabel: { formatter: (v: number) => formatCompactNumber(v) } },
      yAxis: { type: 'category', data: chartData.value.topDomain.map((item) => item.name), inverse: true, splitLine: { show: false } },
      series: [{ name: t('monitor.queryCountLabel'), type: 'bar', barMaxWidth: 22, data: chartData.value.topDomain.map((item) => item.value) }],
    },
    topIp: {
      grid: { left: 12, right: 12, top: 22, bottom: 12, containLabel: true },
      tooltip: { trigger: 'axis', confine: true, axisPointer: { type: 'shadow' } },
      xAxis: { type: 'value', splitLine: { show: false }, axisLabel: { formatter: (v: number) => formatCompactNumber(v) } },
      yAxis: { type: 'category', data: chartData.value.topIp.map((item) => item.name), inverse: true, splitLine: { show: false } },
      series: [{ name: t('monitor.visitCountLabel'), type: 'bar', barMaxWidth: 22, data: chartData.value.topIp.map((item) => item.value) }],
    },
    heatmap: {
      grid: { left: 12, right: 12, top: 22, bottom: 38, containLabel: true },
      tooltip: { position: 'top', confine: true },
      xAxis: { type: 'category', data: chartData.value.heatmap.periods, splitLine: { show: false } },
      yAxis: { type: 'category', data: chartData.value.heatmap.regions, splitLine: { show: false } },
      visualMap: {
        min: 0,
        max: Math.max(1, ...chartData.value.heatmap.values.map((item) => Number(item[2] || 0))),
        calculable: true,
        orient: 'horizontal',
        left: 'center',
        bottom: 0,
        inRange: { color: ['#E6F4FF', '#91CAFF', '#1677FF'] },
      },
      series: [{ type: 'heatmap', data: chartData.value.heatmap.values }],
    },
    status: {
      tooltip: {
        trigger: 'item',
        confine: true,
        formatter: (params: { name?: string; value?: number }) => `${params?.name || ''}<br/>${t('monitor.quantityLabel')}: ${formatCompactNumber(Number(params?.value || 0))}`,
      },
      legend: { bottom: 0, left: 'center' },
      series: [{ name: t('monitor.resolveStatusLabel'), type: 'pie', radius: ['46%', '70%'], label: { show: true, formatter: '{b} {d}%' }, data: chartData.value.statusDistribution }],
    },
  }))

  const fetchData = async (options?: { silent?: boolean; refresh?: boolean }) => {
    const { silent = false, refresh = false } = options || {}
    if (refresh) {
      refreshing.value = true
    } else {
      loading.value = true
    }

    try {
      if (refresh) {
        await refreshMonitorRealtime()
      }
      const { data } = await getMonitorModuleData() as { data: MonitorModuleData }
      rawTableData.value = JSON.parse(JSON.stringify(data.realTime || []))
      lastUpdated.value = new Date().toLocaleString(locale.value === 'en-US' ? 'en-US' : 'zh-CN', { hour12: false })
      if (!silent) {
        ElMessage.success(refresh ? t('monitor.realTimeRefreshed') : t('monitor.realTimeLoaded'))
      }
    } catch (_error) {
      if (!silent) {
        ElMessage.error(refresh ? t('monitor.refreshFailedRetry') : t('monitor.realTimeLoadFailed'))
      }
    } finally {
      loading.value = false
      refreshing.value = false
    }
  }

  const applyFilters = () => {
    queryLoading.value = true
    if (queryTimer !== null) {
      window.clearTimeout(queryTimer)
    }
    queryTimer = window.setTimeout(() => {
      Object.assign(appliedFilters, JSON.parse(JSON.stringify(filterForm)))
      pagination.page = 1
      queryLoading.value = false
      ElMessage.success(t('monitor.filterApplied'))
      queryTimer = null
    }, 240)
  }

  const resetFilters = () => {
    Object.assign(filterForm, defaultFilterForm())
    Object.assign(appliedFilters, defaultFilterForm())
    pagination.page = 1
  }

  const handleSortChange = ({ prop, order }: SortChangePayload) => {
    sortState.prop = prop || 'time'
    sortState.order = order || 'descending'
  }

  const stopAutoRefresh = () => {
    if (timerId.value !== null) {
      window.clearInterval(timerId.value)
      timerId.value = null
    }
  }

  const startAutoRefresh = () => {
    stopAutoRefresh()
    if (!autoRefreshEnabled.value) {
      return
    }
    timerId.value = window.setInterval(() => {
      void fetchData({ silent: true, refresh: true })
    }, refreshInterval.value)
  }

  const setRefreshInterval = (value: number) => {
    refreshInterval.value = value
  }

  const openDetail = (row: MonitorRealTimeRow) => {
    detailRow.value = row
    detailVisible.value = true
  }

  const closeDetail = () => {
    detailVisible.value = false
    detailRow.value = null
  }

  const copyText = async (text: string, successMessage: string) => {
    try {
      await navigator.clipboard.writeText(text)
      ElMessage.success(successMessage)
    } catch (_error) {
      ElMessage.error(t('monitor.copyFailedManual'))
    }
  }

  const copyDetail = async () => {
    if (!detailRow.value) {
      return
    }
    const lines = [
      `${t('monitor.queryId')}: ${detailRow.value.queryId}`,
      `${t('monitor.time')}: ${detailRow.value.time}`,
      `${t('monitor.queryDomain')}: ${detailRow.value.domain}`,
      `${t('monitor.recordType')}: ${detailRow.value.recordType}`,
      `${t('monitor.sourceIp')}: ${detailRow.value.sourceIp}`,
      `${t('monitor.region')}: ${detailRow.value.region}`,
      `${t('monitor.resolveStatusLabel')}: ${detailRow.value.responseStatus}`,
      `${t('monitor.returnCode')}: ${detailRow.value.rcode}`,
      `${t('monitor.responseTime')}: ${detailRow.value.responseTime} ms`,
    ]
    await copyText(lines.join('\n'), t('monitor.keyInfoCopied'))
  }

  const handleExport = async (format: ExportFormat, scope: ExportScope = 'filtered') => {
    if (exportLoading.value) {
      return
    }
    exportLoading.value = true
    try {
      const rows = toExportRows(scope === 'all' ? rawTableData.value : tableData.value)
      if (!rows.length) {
        ElMessage.warning(t('monitor.noExportData'))
        return
      }
      if (format === 'json') {
        saveFile('monitor-real-time.json', JSON.stringify(rows, null, 2), 'application/json;charset=utf-8')
      } else if (format === 'csv') {
        const columns = Object.keys(rows[0])
        const csv = [
          columns.join(','),
          ...rows.map((row) => columns.map((key) => `"${String(row[key as keyof typeof row] ?? '').replace(/"/g, '""')}"`).join(',')),
        ].join('\n')
        saveFile('monitor-real-time.csv', `\uFEFF${csv}`, 'text/csv;charset=utf-8')
      } else {
        const XLSX = await import('xlsx')
        const worksheet = XLSX.utils.json_to_sheet(rows)
        const workbook = XLSX.utils.book_new()
        XLSX.utils.book_append_sheet(workbook, worksheet, 'Realtime')
        XLSX.writeFile(workbook, 'monitor-real-time.xlsx')
      }
      ElMessage.success(t('monitor.realTimeExportSuccess'))
    } catch (_error) {
      ElMessage.error(t('monitor.exportFailedRetry'))
    } finally {
      exportLoading.value = false
    }
  }

  watch(
    () => sortedTableData.value.length,
    (length) => {
      const maxPage = Math.max(1, Math.ceil(length / pagination.size))
      if (pagination.page > maxPage) {
        pagination.page = maxPage
      }
    },
  )

  watch([autoRefreshEnabled, refreshInterval], () => {
    startAutoRefresh()
  })

  onMounted(() => {
    void fetchData({ silent: true })
    startAutoRefresh()
  })

  onBeforeUnmount(() => {
    stopAutoRefresh()
    if (queryTimer !== null) {
      window.clearTimeout(queryTimer)
      queryTimer = null
    }
  })

  return {
    recordTypeOptions,
    regionOptions,
    statusOptions,
    refreshIntervalOptions,
    filterForm,
    chartData,
    chartOptions,
    tableData,
    sortedTableData,
    pagedTableData,
    pagination,
    sortState,
    loading,
    queryLoading,
    refreshing,
    exportLoading,
    autoRefreshEnabled,
    refreshInterval,
    lastUpdated,
    detailVisible,
    detailRow,
    formatDateTime,
    formatCompactNumber,
    fetchData,
    applyFilters,
    resetFilters,
    handleSortChange,
    startAutoRefresh,
    stopAutoRefresh,
    setRefreshInterval,
    openDetail,
    closeDetail,
    copyText,
    copyDetail,
    handleExport,
  }
}

export default useRealTimeMonitor