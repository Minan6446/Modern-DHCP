import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { jsPDF } from 'jspdf'
import * as XLSX from 'xlsx'
import { useMonitorStore } from '../../stores/monitor'
import type { MonitorReportData } from '../../types/modules'
import { loadTableState, saveTableState } from '../../utils/tableState'

export type ReportChartType = 'domain' | 'ip' | 'heatmap' | 'status'
export type ReportType = 'overview' | 'traffic' | 'geo' | 'status'
export type AggregateDimension = 'domain' | 'ip' | 'region' | 'status'

export interface ReportFilterForm {
  reportType: ReportType
  aggregateDimension: AggregateDimension
  timePreset: string
  domain: string
  timeRange: string[]
}

export interface ReportTableRow {
  rank: number
  label: string
  value: number
  category: string
}

interface ReportViewState {
  reportFilters: ReportFilterForm
  chartType: ReportChartType
}

const REPORT_STATE_KEY = 'modern-dns:report:view-state'

const DEFAULT_FILTERS: ReportFilterForm = {
  reportType: 'overview',
  aggregateDimension: 'domain',
  timePreset: '7d',
  domain: '',
  timeRange: [],
}

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

export const useReport = () => {
  const { t } = useI18n()
  const monitorStore = useMonitorStore()
  const cachedState = loadTableState<ReportViewState>(REPORT_STATE_KEY, {
    reportFilters: DEFAULT_FILTERS,
    chartType: 'status',
  })

  const filterForm = reactive<ReportFilterForm>({
    ...DEFAULT_FILTERS,
    ...cachedState.reportFilters,
    timeRange: [...(cachedState.reportFilters.timeRange || [])],
  })
  const appliedFilters = reactive<ReportFilterForm>({
    ...filterForm,
    timeRange: [...filterForm.timeRange],
  })
  const querying = ref(false)
  const exporting = ref(false)
  const fullscreenVisible = ref(false)
  const chartType = ref<ReportChartType>(cachedState.chartType || 'status')

  const loading = computed(() => monitorStore.loading)
  const lastUpdated = computed(() => monitorStore.lastUpdated)

  const reportTypeOptions = computed<Array<{ label: string; value: ReportType }>>(() => [
    { label: t('monitor.reportTypeOverview'), value: 'overview' },
    { label: t('monitor.reportTypeTraffic'), value: 'traffic' },
    { label: t('monitor.reportTypeGeo'), value: 'geo' },
    { label: t('monitor.reportTypeStatus'), value: 'status' },
  ])

  const aggregateOptions = computed<Array<{ label: string; value: AggregateDimension }>>(() => [
    { label: t('monitor.aggregateByDomain'), value: 'domain' },
    { label: t('monitor.aggregateByClientIp'), value: 'ip' },
    { label: t('monitor.aggregateByRegion'), value: 'region' },
    { label: t('monitor.aggregateByStatus'), value: 'status' },
  ])

  watch(
    () => ({
      reportFilters: { ...appliedFilters, timeRange: [...appliedFilters.timeRange] },
      chartType: chartType.value,
    }),
    (next) => {
      saveTableState(REPORT_STATE_KEY, next)
    },
    { deep: true },
  )

  const reportScale = computed<number>(() => {
    const preset = appliedFilters.timePreset
    if (preset === '1d') {
      return 0.3
    }
    if (preset === '7d') {
      return 1
    }
    if (preset === '30d') {
      return 4.2
    }
    if (appliedFilters.timeRange.length === 2) {
      const days = Math.max(1, (parseDate(appliedFilters.timeRange[1]) - parseDate(appliedFilters.timeRange[0])) / (24 * 60 * 60 * 1000))
      return Math.max(0.2, Math.min(4.2, days / 7))
    }
    return 1
  })

  const reportData = computed<MonitorReportData>(() => {
    const scale = reportScale.value
    const domainKeyword = appliedFilters.domain.trim()
    const topDomain = monitorStore.report.topDomain
      .filter((item) => !domainKeyword || item.name.includes(domainKeyword))
      .map((item) => ({ ...item, value: Math.round(item.value * scale) }))
    const topIp = monitorStore.report.topIp.map((item) => ({ ...item, value: Math.round(item.value * scale) }))
    const heatmap = {
      ...monitorStore.report.heatmap,
      values: monitorStore.report.heatmap.values.map((item) => [item[0], item[1], Math.round(item[2] * scale)]),
    }
    const statusDistribution = monitorStore.report.statusDistribution.map((item) => ({ ...item, value: Math.round(item.value * scale) }))
    return { topDomain, topIp, heatmap, statusDistribution }
  })

  const tableData = computed<ReportTableRow[]>(() => {
    if (appliedFilters.aggregateDimension === 'ip') {
      return reportData.value.topIp.map((item, index) => ({ rank: index + 1, label: item.name, value: item.value, category: t('monitor.clientIp') }))
    }
    if (appliedFilters.aggregateDimension === 'region') {
      const rows = reportData.value.heatmap.regions.map((region, regionIndex) => {
        const total = reportData.value.heatmap.values
          .filter((item) => Number(item[1]) === regionIndex)
          .reduce((sum, item) => sum + Number(item[2] || 0), 0)
        return { rank: 0, label: region, value: total, category: t('monitor.region') }
      })
      return rows
        .sort((left, right) => right.value - left.value)
        .map((item, index) => ({ ...item, rank: index + 1 }))
    }
    if (appliedFilters.aggregateDimension === 'status') {
      return reportData.value.statusDistribution.map((item, index) => ({ rank: index + 1, label: item.name, value: item.value, category: t('common.status') }))
    }
    return reportData.value.topDomain.map((item, index) => ({ rank: index + 1, label: item.name, value: item.value, category: t('common.domain') }))
  })

  const visibleCharts = computed<ReportChartType[]>(() => {
    if (appliedFilters.reportType === 'traffic') {
      return ['domain', 'ip']
    }
    if (appliedFilters.reportType === 'geo') {
      return ['heatmap']
    }
    if (appliedFilters.reportType === 'status') {
      return ['status']
    }
    return ['domain', 'ip', 'heatmap', 'status']
  })

  const reportIsEmpty = computed<boolean>(
    () => !reportData.value.topDomain.length && !reportData.value.topIp.length && !reportData.value.heatmap.values.length && !reportData.value.statusDistribution.length,
  )

  const fetchReportData = async (): Promise<void> => {
    try {
      await monitorStore.fetchMonitorData()
    } catch (_error) {
      ElMessage.error(t('monitor.reportLoadFailed'))
    }
  }

  const handleSearch = (): void => {
    querying.value = true
    Object.assign(appliedFilters, {
      ...filterForm,
      timeRange: [...filterForm.timeRange],
    })
    querying.value = false
    ElMessage.success(t('monitor.reportFilterApplied'))
  }

  const handleReset = (): void => {
    Object.assign(filterForm, { ...DEFAULT_FILTERS, timeRange: [] })
    Object.assign(appliedFilters, { ...DEFAULT_FILTERS, timeRange: [] })
    ElMessage.success(t('monitor.reportFilterReset'))
  }

  const exportReport = async (command: string): Promise<void> => {
    if (exporting.value) {
      return
    }

    try {
      exporting.value = true
      if (command === 'excel') {
        const workbook = XLSX.utils.book_new()
        XLSX.utils.book_append_sheet(workbook, XLSX.utils.json_to_sheet(reportData.value.topDomain), 'TopDomain')
        XLSX.utils.book_append_sheet(workbook, XLSX.utils.json_to_sheet(reportData.value.topIp), 'TopIP')
        XLSX.utils.book_append_sheet(workbook, XLSX.utils.json_to_sheet(tableData.value), 'SummaryTable')
        XLSX.utils.book_append_sheet(workbook, XLSX.utils.json_to_sheet(reportData.value.statusDistribution), 'StatusDistribution')
        XLSX.writeFile(workbook, 'monitor-report.xlsx')
      } else {
        const pdf = new jsPDF({ unit: 'pt', format: 'a4' })
        pdf.setFontSize(14)
        pdf.text(t('monitor.pdfTitle'), 40, 44)
        pdf.setFontSize(10)
        pdf.text(`${t('monitor.generatedAt')}: ${formatDateTime(new Date().toISOString())}`, 40, 64)
        let y = 92
        const sections = [
          { title: t('monitor.topDomainStats'), rows: reportData.value.topDomain.map((item) => `${item.name}: ${item.value}`) },
          { title: t('monitor.topIpStats'), rows: reportData.value.topIp.map((item) => `${item.name}: ${item.value}`) },
          { title: t('monitor.aggregateTable'), rows: tableData.value.map((item) => `${item.label}: ${item.value}`) },
          { title: t('monitor.statusDist'), rows: reportData.value.statusDistribution.map((item) => `${item.name}: ${item.value}`) },
        ]
        sections.forEach((section) => {
          pdf.setFontSize(12)
          pdf.text(section.title, 40, y)
          y += 18
          pdf.setFontSize(10)
          section.rows.slice(0, 12).forEach((row) => {
            pdf.text(`- ${row}`, 48, y)
            y += 14
          })
          y += 10
        })
        pdf.save('monitor-report.pdf')
      }
      ElMessage.success(t('monitor.reportExportSuccess'))
    } catch (_error) {
      ElMessage.error(t('monitor.reportExportFailed'))
    } finally {
      exporting.value = false
    }
  }

  const openChartFullscreen = (type: ReportChartType): void => {
    chartType.value = type
    fullscreenVisible.value = true
  }

  const closeFullscreenDialog = (): void => {
    fullscreenVisible.value = false
  }

  const onFullscreenOpened = (): void => {
    window.dispatchEvent(new Event('resize'))
  }

  return {
    filterForm,
    reportTypeOptions,
    aggregateOptions,
    querying,
    exporting,
    fullscreenVisible,
    chartType,
    loading,
    lastUpdated,
    reportData,
    tableData,
    visibleCharts,
    reportIsEmpty,
    fetchReportData,
    handleSearch,
    handleReset,
    exportReport,
    openChartFullscreen,
    closeFullscreenDialog,
    onFullscreenOpened,
  }
}

export default useReport