import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useDashboardStore } from '../../stores/dashboard'
import { useAppStore } from '../../stores/app'
import { confirmRiskAction } from '../../utils/interaction'
import { handleAlertEvent as apiHandleAlert, markAllAlertsRead as apiMarkAllRead, getSecurityPosture as apiGetSecurityPosture } from '../../api/dashboard'
import type { SecurityPosture } from '../../api/dashboard'

export const useDashboard = () => {
  const router = useRouter()
  const { t } = useI18n()
  const appStore = useAppStore()
  const dashboardStore = useDashboardStore()

  const loading = computed(() => dashboardStore.loading)

  const securityPosture = ref<SecurityPosture | null>(null)
  const securityPostureLoading = ref(false)
  const fetchSecurityPosture = async () => {
    if (securityPostureLoading.value) return
    securityPostureLoading.value = true
    try {
      const res = await apiGetSecurityPosture()
      if (res?.data) securityPosture.value = res.data
    } catch { /* silently skip */ }
    finally { securityPostureLoading.value = false }
  }
  const overviewRefreshing = ref(false)
  const overviewRefreshCooldown = ref(false)
  const overviewAutoRefreshEnabled = ref(true)
  const overviewAutoRefreshCountdown = ref(30)
  const domainDetecting = ref(false)
  const alertBatchProcessing = ref(false)

  let overviewRefreshDebounceTimer: number | null = null
  let overviewCountdownTimer: number | null = null
  let domainFilterDebounceTimer: number | null = null
  let alertFilterDebounceTimer: number | null = null

  const domainSelection = ref<any[]>([])
  const alertSelection = ref<any[]>([])

  const domainFilters = reactive({ keyword: '', status: '' })
  const domainAppliedFilters = reactive({ keyword: '', status: '' })
  const alertFilters = reactive({ keyword: '', level: '', type: '', range: [] as string[], quick: 'all' })
  const alertAppliedFilters = reactive({ keyword: '', level: '', type: '', range: [] as string[], quick: 'all' })
  const domainPager = reactive({ page: 1, size: 5 })
  const alertPager = reactive({ page: 1, size: 5 })
  const auditPager = reactive({ page: 1, size: 5 })
  const resourceChartRange = ref('day')
  const resourceMetricModes = reactive({ query: 'day', zone: 'day', record: 'day' })

  const overviewRangeOptions = computed(() => [
    { label: t('dashboard.today') || '今日', value: 'today' },
    { label: t('dashboard.last7d') || '近7天', value: '7d' },
    { label: t('dashboard.last30d') || '近30天', value: '30d' },
    { label: t('dashboard.custom') || '自定义', value: 'custom' },
  ])

  const qpsViewOptions = computed(() => [
    { label: t('dashboard.last24h') || '近24h', value: 'day' },
    { label: t('dashboard.last7d') || '近7天', value: 'week' },
  ])

  const resourceDimensionOptions = computed(() => [
    { label: t('dashboard.dimDay') || '日', value: 'day' },
    { label: t('dashboard.dimMonth') || '月', value: 'month' },
    { label: t('dashboard.dimYear') || '年', value: 'year' },
  ])

  const cssVars = computed(() => {
    appStore.theme
    const style = getComputedStyle(document.documentElement)
    return {
      border: style.getPropertyValue('--app-border').trim(),
      text: style.getPropertyValue('--app-text-regular').trim(),
      bg: style.getPropertyValue('--app-bg-secondary').trim(),
      danger: style.getPropertyValue('--app-danger').trim(),
    }
  })

  const OVERVIEW_DEFAULT = { metrics: [] as any[], response: { xAxis: [] as any[], avg: [] as any[], max: [] as any[], min: [] as any[] }, cache: { total: 0, series: [] as any[] }, total: { xAxis: [] as any[], data: [] as any[] } }
  const overviewDataset = computed(() => {
    const rangeKeyMap: Record<string, string> = { today: 'today', '7d': 'week', '30d': 'month', custom: 'month' }
    const key = rangeKeyMap[dashboardStore.overviewRange] || 'today'
    const v = (dashboardStore.rawData.overview as any)?.[key]
    if (!v || typeof v !== 'object') return OVERVIEW_DEFAULT
    return {
      ...OVERVIEW_DEFAULT, ...v,
      response: { ...OVERVIEW_DEFAULT.response, ...(v.response ?? {}) },
      cache: { ...OVERVIEW_DEFAULT.cache, ...(v.cache ?? {}) },
      total: { ...OVERVIEW_DEFAULT.total, ...(v.total ?? {}) },
    }
  })

  const metricLabelMap: Record<string, string> = {
    dnsQps: 'dashboard.dnsQps',
    avgResponseTime: 'dashboard.avgResponseTime',
    cacheHitRate: 'dashboard.cacheHitRate',
    totalResolves: 'dashboard.totalResolves',
  }
  const metricStatusMap: Record<string, string> = {
    normal: 'dashboard.normal',
    abnormal: 'dashboard.abnormal',
    elevated: 'dashboard.elevated',
  }
  const overviewMetrics = computed(() =>
    (overviewDataset.value.metrics || []).map((m: any) => ({
      ...m,
      label: m.labelKey ? t(metricLabelMap[m.labelKey] || m.labelKey) : m.label,
      status: m.status ? t(metricStatusMap[m.status] || m.status) : undefined,
    })),
  )
  const qpsDataset = computed(() => (dashboardStore.rawData.overview as any)?.qpsTrend?.[dashboardStore.qpsGranularity] || { xAxis: [], data: [] })
  const domainRows = computed(() => { const v = dashboardStore.rawData.domainStatus; return Array.isArray(v) ? v : [] })
  const alertRows = computed(() => { const v = (dashboardStore.rawData.alerts as any)?.rows; return Array.isArray(v) ? v : [] })
  const alertAuditLogs = computed(() => { const v = (dashboardStore.rawData.alerts as any)?.auditLogs; return Array.isArray(v) ? v : [] })
  const RESOURCE_DEFAULT = { metrics: [] as any[], trend: { xAxis: [] as any[], queries: [] as any[], zones: [] as any[], records: [] as any[] }, usage: [] as any[] }
  const resourceSource = computed(() => {
    const v = (dashboardStore.rawData.resource as any)?.[resourceChartRange.value]
    if (!v || typeof v !== 'object') return RESOURCE_DEFAULT
    return { ...RESOURCE_DEFAULT, ...v, trend: { ...RESOURCE_DEFAULT.trend, ...(v.trend ?? {}) } }
  })

  const formatDateTime = (value: string) => {
    if (!value || value === '--') return '--'
    return value.replace('T', ' ').slice(0, 19)
  }

  const formatNumber = (value: number) => new Intl.NumberFormat().format(value)

  const pageSlice = <T>(list: T[], pager: { page: number; size: number }): T[] => {
    const start = (pager.page - 1) * pager.size
    return list.slice(start, start + pager.size)
  }

  const metricStatusTagType = (item: any) => {
    if (item?.key !== 'response') return 'success'
    const value = Number(String(item?.value || '').replace(/[^\d.]/g, ''))
    if (!Number.isFinite(value)) return 'info'
    if (value > 50) return 'warning'
    return 'success'
  }

  const metricTrendClass = (direction: string) => (direction === 'down' ? 'is-down' : 'is-up')
  const showOverviewSkeleton = computed(() => loading.value || overviewRefreshing.value)

  const runOverviewRefresh = async () => {
    if (overviewRefreshing.value) return
    overviewRefreshing.value = true
    try {
      await Promise.all([dashboardStore.fetchData(), fetchSecurityPosture()])
      ElMessage.success(t('dashboard.refreshed'))
    } finally {
      overviewRefreshing.value = false
    }
  }

  const refreshCurrentView = async (currentView: string) => {
    if (currentView !== 'overview') {
      await dashboardStore.fetchData()
      ElMessage.success(t('dashboard.refreshed'))
      return
    }
    if (overviewRefreshCooldown.value) return
    if (overviewRefreshDebounceTimer) window.clearTimeout(overviewRefreshDebounceTimer)
    overviewRefreshDebounceTimer = window.setTimeout(async () => {
      await runOverviewRefresh()
      overviewRefreshCooldown.value = true
      window.setTimeout(() => { overviewRefreshCooldown.value = false }, 300)
      overviewRefreshDebounceTimer = null
    }, 300)
  }

  const stopOverviewAutoRefresh = () => {
    if (overviewCountdownTimer) {
      window.clearInterval(overviewCountdownTimer)
      overviewCountdownTimer = null
    }
  }

  const startOverviewAutoRefresh = (currentView: string) => {
    stopOverviewAutoRefresh()
    if (!overviewAutoRefreshEnabled.value || currentView !== 'overview') return
    overviewAutoRefreshCountdown.value = 30
    overviewCountdownTimer = window.setInterval(async () => {
      if (!overviewAutoRefreshEnabled.value) return
      if (overviewAutoRefreshCountdown.value <= 1) {
        overviewAutoRefreshCountdown.value = 30
        await runOverviewRefresh()
        return
      }
      overviewAutoRefreshCountdown.value -= 1
    }, 1000)
  }

  const handleOverviewRangeChange = (value: string) => {
    dashboardStore.setOverviewRange(value)
    if (overviewRefreshDebounceTimer) window.clearTimeout(overviewRefreshDebounceTimer)
    overviewRefreshDebounceTimer = window.setTimeout(async () => {
      await runOverviewRefresh()
      overviewRefreshDebounceTimer = null
    }, 300)
  }

  const handleQpsGranularityChange = (value: string) => {
    dashboardStore.setQpsGranularity(value)
  }

  const qpsOption = computed(() => ({
    legend: { top: 0, textStyle: { color: cssVars.value.text } },
    tooltip: { trigger: 'axis', confine: true },
    xAxis: { type: 'category', data: qpsDataset.value.xAxis, axisLine: { lineStyle: { color: cssVars.value.border } }, axisLabel: { color: cssVars.value.text } },
    yAxis: { type: 'value', name: 'QPS', splitLine: { show: false }, axisLabel: { color: cssVars.value.text } },
    series: [{ name: t('dashboard.qpsTrend'), type: 'line', smooth: true, areaStyle: {}, symbolSize: 8, data: qpsDataset.value.data }],
  }))

  const responseOption = computed(() => ({
    legend: { top: 0, textStyle: { color: cssVars.value.text } },
    tooltip: { trigger: 'axis', confine: true },
    xAxis: { type: 'category', data: overviewDataset.value.response?.xAxis || [], axisLine: { lineStyle: { color: cssVars.value.border } }, axisLabel: { color: cssVars.value.text } },
    yAxis: { type: 'value', name: 'ms', splitLine: { show: false }, axisLabel: { color: cssVars.value.text } },
    series: [
      { name: t('dashboard.avgResponse'), type: 'line', smooth: true, data: overviewDataset.value.response?.avg || [] },
      { name: t('dashboard.maxResponse'), type: 'line', smooth: true, data: overviewDataset.value.response?.max || [], markLine: { symbol: 'none', lineStyle: { color: cssVars.value.danger, type: 'dashed' }, label: { color: cssVars.value.danger }, data: [{ yAxis: 100, name: '100ms' }] } },
      { name: t('dashboard.minResponse'), type: 'line', smooth: true, data: overviewDataset.value.response?.min || [] },
    ],
  }))

  const cacheNameMap: Record<string, string> = { hit: 'dashboard.cacheHit', miss: 'dashboard.cacheMiss', '命中': 'dashboard.cacheHit', '未命中': 'dashboard.cacheMiss' }
  const cacheOption = computed(() => ({
    tooltip: { trigger: 'item', confine: true },
    legend: { bottom: 0, textStyle: { color: cssVars.value.text } },
    series: [{ name: t('dashboard.cacheHitRatio'), type: 'pie', radius: ['46%', '72%'], label: { formatter: '{b}\n{d}%' }, data: (overviewDataset.value.cache?.series || []).map((s: any) => ({ ...s, name: t(cacheNameMap[s.name] || s.name) })), color: ['#1677FF', '#C9CDD4'] }],
  }))

  const totalOption = computed(() => ({
    legend: { top: 0, textStyle: { color: cssVars.value.text } },
    tooltip: { trigger: 'axis', confine: true },
    xAxis: { type: 'category', data: overviewDataset.value.total?.xAxis || [], axisLine: { lineStyle: { color: cssVars.value.border } }, axisLabel: { color: cssVars.value.text } },
    yAxis: { type: 'value', name: t('dashboard.millions'), splitLine: { show: false }, axisLabel: { color: cssVars.value.text } },
    series: [{ name: t('dashboard.totalTrend'), type: 'bar', barMaxWidth: 32, data: overviewDataset.value.total?.data || [] }],
  }))

  const filteredDomainRows = computed(() =>
    domainRows.value.filter((item: any) => {
      const matchKeyword = !domainAppliedFilters.keyword || item.domain.includes(domainAppliedFilters.keyword)
      const matchStatus = !domainAppliedFilters.status || item.status === domainAppliedFilters.status
      return matchKeyword && matchStatus
    }),
  )

  const pagedDomainRows = computed(() => pageSlice(filteredDomainRows.value, domainPager))

  const domainStatusStats = computed(() => {
    const rows = domainRows.value
    const availableRows = rows.filter((item: any) => item.availability !== '--')
    const avgAvailability = availableRows.length
      ? (availableRows.reduce((sum: number, item: any) => sum + Number(item.availability || 0), 0) / availableRows.length).toFixed(2)
      : '--'
    return [
      { label: t('dashboard.totalDomains'), value: rows.length },
      { label: t('dashboard.normalResolve'), value: rows.filter((item: any) => item.status === 'normal' || item.status === '正常').length },
      { label: t('dashboard.abnormalDomains'), value: rows.filter((item: any) => item.status === 'abnormal' || item.status === '异常').length },
      { label: t('dashboard.avgAvailability'), value: avgAvailability === '--' ? '--' : `${avgAvailability}%` },
    ]
  })

  const filteredAlertRows = computed(() =>
    alertRows.value.filter((item: any) => {
      const matchKeyword = !alertAppliedFilters.keyword || [item.content, item.domain, item.type].some((field: string) => field.includes(alertAppliedFilters.keyword))
      const matchLevel = !alertAppliedFilters.level || item.level === alertAppliedFilters.level
      const matchType = !alertAppliedFilters.type || item.type === alertAppliedFilters.type
      const matchRange = !alertAppliedFilters.range?.length || (item.triggeredAt >= `${alertAppliedFilters.range[0]} 00:00:00` && item.triggeredAt <= `${alertAppliedFilters.range[1]} 23:59:59`)
      const matchQuick =
        alertAppliedFilters.quick === 'all'
        || (alertAppliedFilters.quick === 'unread' && !item.read)
        || (alertAppliedFilters.quick === 'urgent' && (item.level === '紧急' || item.level === 'urgent'))
        || (alertAppliedFilters.quick === 'ddos' && (item.type === 'DDoS攻击' || item.type === 'DDoS'))
        || (alertAppliedFilters.quick === 'dnssec' && (item.type === 'DNSSEC异常' || item.type === 'DNSSEC'))
      return matchKeyword && matchLevel && matchType && matchRange && matchQuick
    }),
  )

  const pagedAlertRows = computed(() => pageSlice(filteredAlertRows.value, alertPager))
  const pagedAuditLogs = computed(() => pageSlice(alertAuditLogs.value, auditPager))

  const hasActiveAlertFilters = computed(() =>
    Boolean(alertFilters.keyword.trim() || alertFilters.level || alertFilters.type || alertFilters.range?.length || alertFilters.quick !== 'all'),
  )

  const alertStats = computed(() => {
    const rows = alertRows.value
    return [
      { key: 'unread', label: t('dashboard.unreadAlerts'), value: rows.filter((item: any) => !item.read).length, emphasize: false },
      { key: 'urgent', label: t('dashboard.urgentAlerts'), value: rows.filter((item: any) => item.level === '紧急' || item.level === 'urgent').length, emphasize: true },
      { key: 'ddos', label: t('dashboard.ddosAlerts'), value: rows.filter((item: any) => item.type === 'DDoS攻击' || item.type === 'DDoS').length, emphasize: false },
      { key: 'dnssec', label: t('dashboard.dnssecAlerts'), value: rows.filter((item: any) => item.type === 'DNSSEC异常' || item.type === 'DNSSEC').length, emphasize: false },
    ]
  })

  const unreadAlertCount = computed(() => alertRows.value.filter((item: any) => !item.read).length)

  const resourceLabelKeyMap: Record<string, string> = {
    totalResolves: 'dashboard.totalResolves',
    hostedZones: 'dashboard.hostedZones',
    totalRecordsCount: 'dashboard.totalRecordsCount',
  }
  const resourceDetailKeyMap: Record<string, string> = {
    quotaUsage: 'dashboard.quotaUsage',
    capacity: 'dashboard.capacity',
    recordPool: 'dashboard.recordPool',
  }
  const usageLabelKeyMap: Record<string, string> = {
    queryQuotaUsage: 'dashboard.queryQuotaUsage',
    zoneCapacityUsage: 'dashboard.zoneCapacityUsage',
    recordPoolUsage: 'dashboard.recordPoolUsage',
  }
  const translateMetric = (item: any) => {
    if (!item) return item
    return {
      ...item,
      label: item.labelKey ? t(resourceLabelKeyMap[item.labelKey] || item.labelKey) : item.label,
      detail: item.detailKey ? `${t(resourceDetailKeyMap[item.detailKey] || item.detailKey)} ${item.detail}` : item.detail,
    }
  }
  const resourceMetricMap = computed(() => ({
    query: translateMetric(((dashboardStore.rawData.resource as any)?.[resourceMetricModes.query]?.metrics || []).find((item: any) => item.key === 'queries')),
    zone: translateMetric(((dashboardStore.rawData.resource as any)?.[resourceMetricModes.zone]?.metrics || []).find((item: any) => item.key === 'zones')),
    record: translateMetric(((dashboardStore.rawData.resource as any)?.[resourceMetricModes.record]?.metrics || []).find((item: any) => item.key === 'records')),
  }))

  const resourceUsageList = computed(() => [...(resourceSource.value.usage || [])].map((item: any) => ({
    ...item,
    label: item.labelKey ? t(usageLabelKeyMap[item.labelKey] || item.labelKey) : item.label,
  })).sort((a: any, b: any) => b.percent - a.percent))

  const resourceTrendSummary = computed(() => {
    const trend = resourceSource.value.trend || { xAxis: [], queries: [], zones: [], records: [] }
    const lastIndex = (trend.xAxis?.length || 1) - 1
    if (lastIndex < 0) return { query: '--', zone: '--', record: '--' }
    return {
      query: trend.queries?.[lastIndex] ?? '--',
      zone: trend.zones?.[lastIndex] ?? '--',
      record: trend.records?.[lastIndex] ?? '--',
    }
  })

  const usageProgressColor = (percent: number) => {
    if (percent >= 90) return '#F53F3F'
    if (percent >= 75) return '#FF7D00'
    return '#00B42A'
  }

  const usageStatusTagType = (percent: number) => {
    if (percent >= 90) return 'danger'
    if (percent >= 75) return 'warning'
    return 'success'
  }

  const usageStatusText = (percent: number) => {
    if (percent >= 90) return t('dashboard.highUsage')
    if (percent >= 75) return t('dashboard.watching')
    return t('dashboard.healthy')
  }

  const resourceOption = computed(() => ({
    legend: { top: 0, textStyle: { color: cssVars.value.text } },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: resourceSource.value.trend?.xAxis || [], axisLine: { lineStyle: { color: cssVars.value.border } }, axisLabel: { color: cssVars.value.text } },
    yAxis: [
      { type: 'value', name: t('dashboard.queryCount'), splitLine: { lineStyle: { color: cssVars.value.border } }, axisLabel: { color: cssVars.value.text } },
      { type: 'value', name: t('dashboard.domainRecord'), splitLine: { show: false }, axisLabel: { color: cssVars.value.text } },
    ],
    series: [
      { name: t('dashboard.queryTrendSeries'), type: 'bar', data: resourceSource.value.trend?.queries || [] },
      { name: t('dashboard.zoneTrendSeries'), type: 'line', smooth: true, yAxisIndex: 1, data: resourceSource.value.trend?.zones || [] },
      { name: t('dashboard.recordTrendSeries'), type: 'line', smooth: true, yAxisIndex: 1, data: resourceSource.value.trend?.records || [] },
    ],
  }))

  const addAlertAuditLog = (action: string, target: string) => {
    ;(dashboardStore.rawData.alerts as any).auditLogs.unshift({ id: Date.now(), operator: appStore.user.name, action, target, result: t('common.success'), time: new Date().toLocaleString(undefined, { hour12: false }) })
  }

  const exportCsv = (fileName: string, rows: any[], columns: { label: string; value: (row: any) => any }[]) => {
    const header = columns.map((item) => item.label).join(',')
    const body = rows.map((row) => columns.map((item) => `"${item.value(row)}"`).join(',')).join('\n')
    const blob = new Blob([`${header}\n${body}`], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = fileName
    link.click()
    URL.revokeObjectURL(link.href)
  }

  const exportDomainTable = () => {
    exportCsv('domain-status.csv', filteredDomainRows.value, [
      { label: t('common.domain'), value: (row) => row.domain },
      { label: t('dashboard.resolveStatus'), value: (row) => row.status },
      { label: t('dashboard.resolveAvailability'), value: (row) => (row.availability === '--' ? '--' : `${row.availability}%`) },
      { label: t('dashboard.checkIp'), value: (row) => row.checkIp },
      { label: t('dashboard.lastCheck'), value: (row) => formatDateTime(row.checkedAt) },
    ])
    ElMessage.success(t('dashboard.exportSuccess'))
  }

  const openDomainDetail = (row: any) => {
    router.push({ path: '/domain/zone-list', query: { domain: row.domain } })
  }

  const detectRows = async (rows: any[]) => {
    if (!rows.length || domainDetecting.value) {
      if (!rows.length) ElMessage.warning(t('dashboard.noDetectableDomains'))
      return
    }
    domainDetecting.value = true
    rows.forEach((row) => {
      row.status = 'detecting'
      row.checkedAt = new Date().toLocaleString(undefined, { hour12: false })
    })
    ElMessage.success(t('dashboard.detectStarted'))
    try {
      await new Promise((resolve) => { window.setTimeout(resolve, 1200) })
      rows.forEach((row, index) => {
        row.status = index % 3 === 0 ? 'abnormal' : 'normal'
        row.availability = index % 3 === 0 ? '96.80' : '99.98'
        row.checkIp = index % 3 === 0 ? '119.29.29.29' : '223.5.5.5'
      })
      ElMessage.success(t('dashboard.detectCompleted'))
    } finally {
      domainDetecting.value = false
    }
  }

  const detectAllDomains = async () => { await detectRows(filteredDomainRows.value) }

  const detectSelectedDomains = async () => {
    if (!domainSelection.value.length) {
      ElMessage.warning(t('dashboard.selectDomainsFirst'))
      return
    }
    await detectRows(domainSelection.value)
  }

  const copyDomainIp = async (ip: string) => {
    if (!ip || ip === '--') {
      ElMessage.warning(t('dashboard.noIpToCopy'))
      return
    }
    try {
      await navigator.clipboard.writeText(ip)
      ElMessage.success(t('common.copied'))
    } catch {
      ElMessage.warning(t('dashboard.copyFailed'))
    }
  }

  const availabilityClass = (availability: string | number) => {
    const value = Number(availability)
    if (!Number.isFinite(value)) return 'is-unknown'
    if (value >= 99) return 'is-good'
    if (value >= 97) return 'is-warning'
    return 'is-danger'
  }

  const rowStatusTag = (status: string) => ({ normal: 'success', '正常': 'success', abnormal: 'danger', '异常': 'danger', detecting: 'warning', '检测中': 'warning', undetected: 'info', '未检测': 'info' }[status] || 'info')
  const alertLevelTag = (level: string) => ({ urgent: 'danger', '紧急': 'danger', warning: 'warning', '警告': 'warning', info: 'primary', '提示': 'primary' }[level] || 'info')
  const alertStatusTag = (status: string) => ({ unhandled: 'danger', '未处理': 'danger', processing: 'warning', '处理中': 'warning', handled: 'success', '已处理': 'success' }[status] || 'info')

  const domainStatusText = (status: string) => {
    const map: Record<string, string> = { normal: 'dashboard.normal', '正常': 'dashboard.normal', abnormal: 'dashboard.abnormal', '异常': 'dashboard.abnormal', detecting: 'dashboard.detecting', '检测中': 'dashboard.detecting', undetected: 'dashboard.undetected', '未检测': 'dashboard.undetected' }
    return map[status] ? t(map[status]) : status
  }
  const alertLevelText = (level: string) => {
    const map: Record<string, string> = { urgent: 'dashboard.urgent', '紧急': 'dashboard.urgent', warning: 'dashboard.warning', '警告': 'dashboard.warning', info: 'dashboard.info', '提示': 'dashboard.info' }
    return map[level] ? t(map[level]) : level
  }
  const alertStatusText = (status: string) => {
    const map: Record<string, string> = { unhandled: 'dashboard.unhandled', '未处理': 'dashboard.unhandled', processing: 'dashboard.processing', '处理中': 'dashboard.processing', handled: 'dashboard.handled', '已处理': 'dashboard.handled' }
    return map[status] ? t(map[status]) : status
  }

  const applyAlertQuickFilter = (key: string) => {
    alertFilters.quick = alertFilters.quick === key ? 'all' : key
  }

  const resetAlertFilters = () => {
    alertFilters.keyword = ''
    alertFilters.level = ''
    alertFilters.type = ''
    alertFilters.range = []
    alertFilters.quick = 'all'
    alertSelection.value = []
    alertPager.page = 1
  }

  const markAlertHandled = async (row: any) => {
    try {
      await apiHandleAlert(row.id)
    } catch {
      // ignore — local update still proceeds
    }
    row.status = 'handled'
    row.read = true
    addAlertAuditLog(t('dashboard.markHandled'), `${row.domain} / ${row.type}`)
    ElMessage.success(t('dashboard.alertHandledSuccess'))
  }

  const markSelectedAlertsRead = async () => {
    if (alertBatchProcessing.value) return
    if (!alertSelection.value.length) {
      ElMessage.warning(t('dashboard.selectAlertsFirst'))
      return
    }
    alertBatchProcessing.value = true
    try {
      await confirmRiskAction({ title: t('dashboard.batchConfirmTitle'), action: t('dashboard.batchRead'), risk: t('dashboard.batchReadRisk') })
      alertSelection.value.forEach((row) => {
        row.read = true
        if (row.status === '未处理' || row.status === 'unhandled') row.status = 'processing'
      })
      addAlertAuditLog(t('dashboard.batchRead'), `${alertSelection.value.length}`)
      ElMessage.success(t('dashboard.batchReadSuccess'))
    } finally {
      alertBatchProcessing.value = false
    }
  }

  const markAllAlertsRead = async () => {
    if (alertBatchProcessing.value || !unreadAlertCount.value) return
    alertBatchProcessing.value = true
    try {
      await confirmRiskAction({ title: t('dashboard.batchConfirmTitle'), action: t('dashboard.allRead'), risk: t('dashboard.allReadRisk') })
      await apiMarkAllRead().catch(() => {})
      alertRows.value.forEach((row: any) => { row.read = true })
      addAlertAuditLog(t('dashboard.allRead'), `${alertRows.value.length}`)
      ElMessage.success(t('dashboard.allReadSuccess'))
    } finally {
      alertBatchProcessing.value = false
    }
  }

  const viewAlertDetail = (row: any) => {
    addAlertAuditLog(t('dashboard.viewAlertDetail'), `${row.domain} / ${row.type}`)
    ElMessage.info(t('dashboard.alertDetailMessage', { domain: row.domain, content: row.content }))
  }

  const exportResourceExcel = () => {
    const trend = resourceSource.value.trend
    exportCsv(
      'resource-trend.csv',
      (trend?.xAxis || []).map((label: string, index: number) => ({ label, queryCount: trend.queries[index], zoneCount: trend.zones[index], recordGrowth: trend.records[index] })),
      [
        { label: t('audit.time'), value: (row) => row.label },
        { label: t('dashboard.queryTrendSeries'), value: (row) => row.queryCount },
        { label: t('dashboard.zoneTrendSeries'), value: (row) => row.zoneCount },
        { label: t('dashboard.recordTrendSeries'), value: (row) => row.recordGrowth },
      ],
    )
    ElMessage.success(t('dashboard.exportSuccess'))
  }

  const exportResourceImage = async () => {
    const container = document.createElement('div')
    container.style.cssText = 'width:1200px;height:520px;position:fixed;left:-9999px'
    document.body.appendChild(container)
    try {
      const { echarts } = await import('../../utils/echarts')
      const chart = echarts.init(container)
      chart.setOption(resourceOption.value)
      const link = document.createElement('a')
      link.href = chart.getDataURL({ pixelRatio: 2, backgroundColor: cssVars.value.bg })
      link.download = 'resource-chart.png'
      link.click()
      chart.dispose()
      ElMessage.success(t('dashboard.exportSuccess'))
    } finally {
      document.body.removeChild(container)
    }
  }

  const resourceExportCommand = (command: string) => {
    if (command === 'excel') { exportResourceExcel(); return }
    exportResourceImage()
  }

  const domainRowClassName = ({ row }: { row: any }) => (row.status === 'abnormal' || row.status === '异常' ? 'dashboard-row-danger' : '')
  const alertRowClassName = ({ row }: { row: any }) => ((row.level === '紧急' || row.level === 'urgent') && row.status !== 'handled' && row.status !== '已处理' ? 'dashboard-row-alert' : '')

  const initWatchers = (currentView: import('vue').Ref<string>) => {
    watch([currentView, overviewAutoRefreshEnabled], () => {
      if (currentView.value !== 'overview') {
        stopOverviewAutoRefresh()
        return
      }
      startOverviewAutoRefresh(currentView.value)
    })

    watch(
      () => [domainFilters.keyword, domainFilters.status],
      () => {
        domainPager.page = 1
        if (domainFilterDebounceTimer) window.clearTimeout(domainFilterDebounceTimer)
        domainFilterDebounceTimer = window.setTimeout(() => {
          domainAppliedFilters.keyword = domainFilters.keyword.trim()
          domainAppliedFilters.status = domainFilters.status
          domainFilterDebounceTimer = null
        }, 300)
      },
      { immediate: true },
    )

    watch(
      () => filteredDomainRows.value.length,
      (length) => {
        const maxPage = Math.max(1, Math.ceil(length / domainPager.size))
        if (domainPager.page > maxPage) domainPager.page = maxPage
      },
    )

    watch(
      () => [alertFilters.keyword, alertFilters.level, alertFilters.type, alertFilters.range, alertFilters.quick],
      () => {
        alertPager.page = 1
        if (alertFilterDebounceTimer) window.clearTimeout(alertFilterDebounceTimer)
        alertFilterDebounceTimer = window.setTimeout(() => {
          alertAppliedFilters.keyword = alertFilters.keyword.trim()
          alertAppliedFilters.level = alertFilters.level
          alertAppliedFilters.type = alertFilters.type
          alertAppliedFilters.range = Array.isArray(alertFilters.range) ? [...alertFilters.range] : []
          alertAppliedFilters.quick = alertFilters.quick
          alertFilterDebounceTimer = null
        }, 300)
      },
      { immediate: true },
    )

    watch(
      () => filteredAlertRows.value.length,
      (length) => {
        const maxPage = Math.max(1, Math.ceil(length / alertPager.size))
        if (alertPager.page > maxPage) alertPager.page = maxPage
        alertSelection.value = []
      },
    )

    watch(
      () => alertAuditLogs.value.length,
      (length) => {
        const maxPage = Math.max(1, Math.ceil(length / auditPager.size))
        if (auditPager.page > maxPage) auditPager.page = maxPage
      },
    )
  }

  onMounted(async () => {
    await Promise.all([dashboardStore.fetchData(), fetchSecurityPosture()])
  })

  onBeforeUnmount(() => {
    stopOverviewAutoRefresh()
    if (overviewRefreshDebounceTimer) { window.clearTimeout(overviewRefreshDebounceTimer); overviewRefreshDebounceTimer = null }
    if (domainFilterDebounceTimer) { window.clearTimeout(domainFilterDebounceTimer); domainFilterDebounceTimer = null }
    if (alertFilterDebounceTimer) { window.clearTimeout(alertFilterDebounceTimer); alertFilterDebounceTimer = null }
  })

  return {
    dashboardStore,
    loading,
    overviewRefreshing,
    overviewRefreshCooldown,
    overviewAutoRefreshEnabled,
    overviewAutoRefreshCountdown,
    domainDetecting,
    alertBatchProcessing,
    domainSelection,
    alertSelection,
    domainFilters,
    alertFilters,
    domainPager,
    alertPager,
    auditPager,
    resourceChartRange,
    resourceMetricModes,
    overviewRangeOptions,
    qpsViewOptions,
    resourceDimensionOptions,
    overviewMetrics,
    qpsDataset,
    overviewDataset,
    showOverviewSkeleton,
    filteredDomainRows,
    pagedDomainRows,
    domainStatusStats,
    filteredAlertRows,
    pagedAlertRows,
    pagedAuditLogs,
    alertAuditLogs,
    resourceSource,
    hasActiveAlertFilters,
    alertStats,
    unreadAlertCount,
    resourceMetricMap,
    resourceUsageList,
    resourceTrendSummary,
    qpsOption,
    responseOption,
    cacheOption,
    totalOption,
    resourceOption,
    formatDateTime,
    formatNumber,
    metricStatusTagType,
    metricTrendClass,
    usageProgressColor,
    usageStatusTagType,
    usageStatusText,
    rowStatusTag,
    alertLevelTag,
    alertStatusTag,
    domainStatusText,
    alertLevelText,
    alertStatusText,
    availabilityClass,
    domainRowClassName,
    alertRowClassName,
    refreshCurrentView,
    startOverviewAutoRefresh,
    handleOverviewRangeChange,
    handleQpsGranularityChange,
    exportDomainTable,
    openDomainDetail,
    detectAllDomains,
    detectSelectedDomains,
    copyDomainIp,
    applyAlertQuickFilter,
    resetAlertFilters,
    markAlertHandled,
    markSelectedAlertsRead,
    markAllAlertsRead,
    viewAlertDetail,
    resourceExportCommand,
    securityPosture,
    securityPostureLoading,
    fetchSecurityPosture,
    initWatchers,
  }
}

export default useDashboard
