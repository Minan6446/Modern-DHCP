import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useToolsStore } from '../../stores/tools'
import type { DigHistoryItem, GlobalTestNode, IpLocationRow } from '../../types/modules'
import { confirmRiskAction, normalizeMultiValue, validateFormAndFocus } from '../../utils/interaction'

type SortOrder = 'ascending' | 'descending' | null
type SortChangePayload = { prop: string | null; order: SortOrder }

const domainPattern = /^(?=.{1,253}$)(\*\.)?(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}$/
const ipv4Pattern = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
const ipv6Pattern = /^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|::1|([0-9a-fA-F]{1,4}:){1,7}:|:([0-9a-fA-F]{1,4}:){1,7})$/

export const formatDateTime = (value: string): string => {
  const date = new Date(String(value || '').replace(' ', 'T'))
  if (Number.isNaN(date.getTime())) return String(value || '--')
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`
}

export const saveFile = (name: string, content: string, type: string) => {
  const url = URL.createObjectURL(new Blob([content], { type }))
  const anchor = document.createElement('a')
  anchor.href = url; anchor.download = name; anchor.click(); URL.revokeObjectURL(url)
}

export const useTools = () => {
  const { t } = useI18n()
  const toolsStore = useToolsStore()
  const loading = computed(() => toolsStore.loading)

  const digFormRef = ref<FormInstance>()
  const dnssecFormRef = ref<FormInstance>()
  const globalFormRef = ref<FormInstance>()
  const ipFormRef = ref<FormInstance>()

  const digForm = reactive({ domain: '', recordType: 'A', dnsServer: '' })
  const dnssecForm = reactive({ domain: '' })
  const globalTestForm = reactive({ domain: '', recordType: 'A', nodeGroup: 'all' })
  const ipForm = reactive({ ips: '' })

  const digLoading = ref(false)
  const digCopying = ref(false)
  const digHistoryDeletingId = ref<number | null>(null)
  const dnssecLoading = ref(false)
  const dnssecExporting = ref(false)
  const globalLoading = ref(false)
  const globalExporting = ref(false)
  const ipQueryLoading = ref(false)
  const ipExporting = ref(false)
  const ipCopyingAll = ref(false)

  const digHistoryPager = reactive({ page: 1, size: 6 })
  const globalPager = reactive({ page: 1, size: 10 })
  const globalSorter = reactive<{ prop: keyof GlobalTestNode | string; order: SortOrder }>({ prop: 'node', order: 'ascending' })
  const ipPager = reactive({ page: 1, size: 10 })

  const globalDetailVisible = ref(false)
  const globalDetailRow = ref<GlobalTestNode | null>(null)

  let actionDebounceTimer: number | null = null

  const runDebounced = (callback: () => Promise<void> | void) => {
    if (actionDebounceTimer) window.clearTimeout(actionDebounceTimer)
    actionDebounceTimer = window.setTimeout(async () => { await callback(); actionDebounceTimer = null }, 300)
  }

  const ipValidationMessage = ref('')

  const digRules: FormRules = {
    domain: [
      { required: true, message: t('tools.enterTargetDomain'), trigger: ['blur', 'change'] },
      { validator: (_r, v, cb) => { if (!v) { cb(); return } if (!domainPattern.test(v)) { cb(new Error(t('tools.invalidDomain'))); return } cb() }, trigger: ['blur', 'change'] },
    ],
    recordType: [{ required: true, message: t('tools.selectRecordType'), trigger: 'change' }],
    dnsServer: [
      { validator: (_r, v, cb) => { if (!v) { cb(); return } if (!ipv4Pattern.test(v) && !domainPattern.test(v)) { cb(new Error(t('tools.dnsServerIpOrDomainOnly'))); return } cb() }, trigger: ['blur', 'change'] },
    ],
  }

  const dnssecRules: FormRules = {
    domain: [
      { required: true, message: t('tools.enterTargetDomain'), trigger: ['blur', 'change'] },
      { validator: (_r, v, cb) => { if (!v) { cb(); return } if (!domainPattern.test(v)) { cb(new Error(t('tools.invalidDomainSupportsWildcard'))); return } cb() }, trigger: ['blur', 'change'] },
    ],
  }

  const globalRules: FormRules = {
    domain: [
      { required: true, message: t('tools.enterTargetDomain'), trigger: ['blur', 'change'] },
      { validator: (_r, v, cb) => { if (!v) { cb(); return } if (!domainPattern.test(v)) { cb(new Error(t('tools.invalidDomain'))); return } cb() }, trigger: ['blur', 'change'] },
    ],
    recordType: [{ required: true, message: t('tools.selectRecordType'), trigger: 'change' }],
  }

  const ipRules: FormRules = {
    ips: [
      { required: true, message: t('tools.enterIpAddress'), trigger: ['blur', 'change'] },
      {
        validator: (_r, v, cb) => {
          const normalized = normalizeMultiValue(String(v || ''))
          if (!normalized) { cb(new Error(t('tools.enterIpAddress'))); return }
          const invalid = normalized.split(',').map((s) => s.trim()).filter(Boolean).find((s) => !ipv4Pattern.test(s) && !ipv6Pattern.test(s))
          if (invalid) { cb(new Error(t('tools.invalidIpFormat', { ip: invalid }))); return }
          cb()
        },
        trigger: ['blur', 'change'],
      },
    ],
  }

  const tagClass = (status: string) => {
    if (status === '成功' || status === 'Success' || status === '一致' || status === 'Consistent' || status === '通过' || status === 'Passed') return 'tag-success'
    if (status === '不一致' || status === 'Inconsistent' || status === '异常' || status === 'Abnormal' || status === '失败' || status === 'Failed') return 'tag-danger'
    if (status === '超时' || status === 'Timeout' || status === '警告' || status === 'Warning') return 'tag-warning'
    return 'tag-muted'
  }

  const riskLevel = (row: IpLocationRow): 'high' | 'medium' | 'low' => {
    const line = String(row.lineType || '')
    if (line.includes('高风险') || line.includes('代理') || line.includes('匿名')) return 'high'
    if (line.includes('企业') || line.includes('数据中心')) return 'medium'
    return 'low'
  }

  const riskClass = (level: 'high' | 'medium' | 'low') => {
    if (level === 'high') return 'tag-danger'
    if (level === 'medium') return 'tag-warning'
    return 'tag-success'
  }

  const riskLabel = (level: 'high' | 'medium' | 'low') => {
    if (level === 'high') return t('tools.riskHigh')
    if (level === 'medium') return t('tools.riskMedium')
    return t('tools.riskLow')
  }

  const sortedGlobalRows = computed(() => {
    const rows = [...toolsStore.globalTestNodes]
    if (!globalSorter.prop || !globalSorter.order) return rows
    const factor = globalSorter.order === 'ascending' ? 1 : -1
    return rows.sort((a, b) => String(a[globalSorter.prop] || '').localeCompare(String(b[globalSorter.prop] || '')) * factor)
  })

  const pagedGlobalRows = computed(() => sortedGlobalRows.value.slice((globalPager.page - 1) * globalPager.size, globalPager.page * globalPager.size))
  const pagedBatchIpRows = computed(() => toolsStore.batchIpResults.slice((ipPager.page - 1) * ipPager.size, ipPager.page * ipPager.size))
  const pagedDigHistory = computed(() => toolsStore.digHistory.slice((digHistoryPager.page - 1) * digHistoryPager.size, digHistoryPager.page * digHistoryPager.size))

  const dnssecStatsCards = computed(() => [
    { key: 'opened', label: t('tools.dnssecOpenedCount'), value: toolsStore.dnssecStats.opened },
    { key: 'valid', label: t('tools.dnssecValidCount'), value: toolsStore.dnssecStats.valid },
    { key: 'abnormal', label: t('tools.dnssecAbnormalCount'), value: toolsStore.dnssecStats.abnormal },
    { key: 'unopened', label: t('tools.dnssecUnopenedCount'), value: toolsStore.dnssecStats.unopened },
  ])

  const dnssecDomainRows = computed(() => {
    if (!toolsStore.dnssecResult) return []
    return [{
      domain: toolsStore.dnssecResult.domain,
      dnssecStatus: toolsStore.dnssecResult.dnssecEnabled ? '已开启' : '未开启',
      signatureStatus: toolsStore.dnssecResult.overallStatus,
      checkedAt: formatDateTime(toolsStore.dnssecResult.checkedAt),
    }]
  })

  const executeDig = async () => {
    if (digLoading.value) return
    if (!await validateFormAndFocus(digFormRef.value)) return
    digLoading.value = true
    try {
      await toolsStore.runDigQuery({ ...digForm, dnsServer: digForm.dnsServer.trim() })
      ElMessage.success(t('tools.digQuerySuccess'))
    } catch { ElMessage.error(t('tools.digQueryFailed')) }
    finally { digLoading.value = false }
  }

  const runDig = () => runDebounced(executeDig)

  const copyDigResult = async () => {
    if (!toolsStore.digOutput || digCopying.value) { if (!toolsStore.digOutput) ElMessage.warning(t('tools.noCopyContent')); return }
    digCopying.value = true
    try { await navigator.clipboard.writeText(toolsStore.digOutput); ElMessage.success(t('tools.resultCopied')) }
    catch { ElMessage.error(t('tools.copyFailedManual')) }
    finally { digCopying.value = false }
  }

  const clearDigOutput = async () => {
    await confirmRiskAction({ title: t('tools.clearConfirmTitle'), action: t('tools.clearDigOutputAction'), risk: t('tools.clearDigOutputRisk') })
    toolsStore.clearDigOutput()
    ElMessage.success(t('tools.outputCleared'))
  }

  const reuseDigHistory = (row: DigHistoryItem) => {
    digForm.domain = row.domain
    digForm.recordType = row.recordType
    digForm.dnsServer = row.dnsServer === '系统默认DNS' ? '' : row.dnsServer
  }

  const removeDigHistory = async (row: DigHistoryItem) => {
    if (digHistoryDeletingId.value) return
    digHistoryDeletingId.value = row.id
    try {
      await confirmRiskAction({ title: t('common.confirm'), action: t('tools.deleteDigHistoryAction'), target: row.domain, risk: t('tools.deleteDigHistoryRisk') })
      toolsStore.deleteDigHistory(row.id)
      ElMessage.success(t('tools.historyDeleted'))
    } catch (e) { if (e !== 'cancel') ElMessage.error(t('tools.deleteFailedRetry')) }
    finally { digHistoryDeletingId.value = null }
  }

  const runDnssecDetect = async () => {
    if (dnssecLoading.value) return
    if (!await validateFormAndFocus(dnssecFormRef.value)) return
    dnssecLoading.value = true
    try { await toolsStore.runDnssecDebug({ domain: dnssecForm.domain.trim() }); ElMessage.success(t('tools.dnssecCheckFinished')) }
    catch { ElMessage.error(t('tools.dnssecCheckFailed')) }
    finally { dnssecLoading.value = false }
  }

  const runDnssec = () => runDebounced(runDnssecDetect)

  const buildDnssecReport = () => {
    if (!toolsStore.dnssecResult) return ''
    const r = toolsStore.dnssecResult
    return [`${t('tools.targetDomain')}: ${r.domain}`, `${t('tools.checkStatus')}: ${r.overallStatus}`, `${t('tools.checkTime')}: ${formatDateTime(r.checkedAt)}`, `${t('tools.dnssecState')}: ${r.dnssecEnabled ? t('security.opened') : t('security.notOpened')}`, `${t('tools.checkDetails')}:`, ...r.details.map((item: any) => `- ${item.item}: ${item.status} / ${item.result} / ${item.detail}`)].join('\n')
  }

  const copyDnssecReport = async () => {
    const text = buildDnssecReport()
    if (!text) { ElMessage.warning(t('tools.noCheckReport')); return }
    try { await navigator.clipboard.writeText(text); ElMessage.success(t('tools.checkReportCopied')) }
    catch { ElMessage.error(t('tools.copyFailedManual')) }
  }

  const exportDnssecReport = async (format: string) => {
    if (!toolsStore.dnssecResult || dnssecExporting.value) return
    dnssecExporting.value = true
    try {
      if (format === 'text') saveFile('tools-dnssec-report.txt', buildDnssecReport(), 'text/plain;charset=utf-8')
      else saveFile('tools-dnssec-report.json', JSON.stringify(toolsStore.dnssecResult, null, 2), 'application/json;charset=utf-8')
      ElMessage.success(t('tools.checkReportExported'))
    } catch { ElMessage.error(t('tools.exportFailedRetry')) }
    finally { dnssecExporting.value = false }
  }

  const runGlobalTest = async () => {
    if (globalLoading.value) return
    if (!await validateFormAndFocus(globalFormRef.value)) return
    globalLoading.value = true
    try { await toolsStore.runGlobalTest({ ...globalTestForm }); globalPager.page = 1; ElMessage.success(t('tools.globalTestFinished')) }
    catch { ElMessage.error(t('tools.globalTestFailed')) }
    finally { globalLoading.value = false }
  }

  const runGlobal = () => runDebounced(runGlobalTest)

  const openGlobalDetail = (row: GlobalTestNode) => { globalDetailRow.value = row; globalDetailVisible.value = true }

  const onGlobalSortChange = ({ prop, order }: SortChangePayload) => {
    globalSorter.prop = (prop as keyof GlobalTestNode) || 'node'
    globalSorter.order = order || 'ascending'
  }

  const exportGlobalRows = async () => {
    const rows = sortedGlobalRows.value
    if (!rows.length || globalExporting.value) { if (!rows.length) ElMessage.warning(t('tools.noGlobalExportData')); return }
    globalExporting.value = true
    try {
      const XLSX = await import('xlsx')
      const wb = XLSX.utils.book_new()
      XLSX.utils.book_append_sheet(wb, XLSX.utils.json_to_sheet(rows.map((item) => ({ [t('tools.nodeName')]: item.node, [t('tools.operator')]: item.operator, [t('tools.resolveResult')]: item.result, [t('tools.latencyMs')]: item.latency, [t('tools.consistencyStatus')]: item.consistency, [t('common.detail')]: item.detail }))), 'GlobalTest')
      XLSX.writeFile(wb, 'tools-global-test.xlsx')
      ElMessage.success(t('tools.testResultExported'))
    } catch { ElMessage.error(t('tools.exportFailedRetry')) }
    finally { globalExporting.value = false }
  }

  const queryIpLocation = async () => {
    if (ipQueryLoading.value) return
    if (!await validateFormAndFocus(ipFormRef.value)) return
    ipQueryLoading.value = true
    try {
      ipForm.ips = normalizeMultiValue(ipForm.ips)
      await toolsStore.lookupIpLocation({ ips: ipForm.ips })
      ipPager.page = 1
      ElMessage.success(t('tools.ipLookupSuccess'))
    } catch { ElMessage.error(t('tools.ipLookupFailed')) }
    finally { ipQueryLoading.value = false }
  }

  const runIpQuery = () => runDebounced(queryIpLocation)

  const copyIpInfo = async (row: IpLocationRow) => {
    const level = riskLevel(row)
    const lines = [`${t('tools.ipAddress')}: ${row.ip}`, `${t('tools.country')}: ${row.country}-${row.province}-${row.city}`, `${t('tools.isp')}: ${row.isp}`, `${t('tools.location')}: ${row.location}`, `${t('tools.riskLevel')}: ${riskLabel(level)}`]
    try { await navigator.clipboard.writeText(lines.join('\n')); ElMessage.success(t('tools.ipInfoCopied')) }
    catch { ElMessage.error(t('tools.copyFailedManual')) }
  }

  const copyAllIpInfo = async () => {
    const rows = toolsStore.ipResults
    if (!rows.length || ipCopyingAll.value) { if (!rows.length) ElMessage.warning(t('tools.noIpCopyData')); return }
    ipCopyingAll.value = true
    try {
      await navigator.clipboard.writeText(rows.map((row) => `${row.ip} | ${row.country}-${row.province}-${row.city} | ${row.isp} | ${row.location} | ${t('tools.riskLevel')}:${riskLabel(riskLevel(row))}`).join('\n'))
      ElMessage.success(t('tools.allIpCopied'))
    } catch { ElMessage.error(t('tools.copyFailedManual')) }
    finally { ipCopyingAll.value = false }
  }

  const exportIpRows = async () => {
    const rows = toolsStore.ipResults
    if (!rows.length || ipExporting.value) { if (!rows.length) ElMessage.warning(t('tools.noIpExportData')); return }
    ipExporting.value = true
    try {
      const csv = [
        [t('tools.ipAddress'), t('tools.country'), t('tools.province'), t('tools.city'), t('tools.isp'), t('tools.location'), t('tools.riskLevel')].join(','),
        ...rows.map((item) => [item.ip, item.country, item.province, item.city, item.isp, item.location, riskLabel(riskLevel(item))].map((cell) => `"${String(cell || '').replace(/"/g, '""')}"`).join(',')),
      ].join('\n')
      saveFile('tools-ip-location.csv', `\uFEFF${csv}`, 'text/csv;charset=utf-8')
      ElMessage.success(t('tools.ipExportSuccess'))
    } catch { ElMessage.error(t('tools.exportFailedRetry')) }
    finally { ipExporting.value = false }
  }

  watch(() => ipForm.ips, (value) => {
    const normalized = normalizeMultiValue(String(value || ''))
    if (!normalized) { ipValidationMessage.value = ''; return }
    const invalid = normalized.split(',').map((s) => s.trim()).filter(Boolean).find((s) => !ipv4Pattern.test(s) && !ipv6Pattern.test(s))
    ipValidationMessage.value = invalid ? t('tools.invalidIpFormat', { ip: invalid }) : ''
  })

  watch(() => toolsStore.digHistory.length, (length) => {
    const maxPage = Math.max(1, Math.ceil(length / digHistoryPager.size))
    if (digHistoryPager.page > maxPage) digHistoryPager.page = maxPage
  })

  watch(() => sortedGlobalRows.value.length, (length) => {
    const maxPage = Math.max(1, Math.ceil(length / globalPager.size))
    if (globalPager.page > maxPage) globalPager.page = maxPage
  })

  watch(() => toolsStore.batchIpResults.length, (length) => {
    const maxPage = Math.max(1, Math.ceil(length / ipPager.size))
    if (ipPager.page > maxPage) ipPager.page = maxPage
  })

  onMounted(async () => {
    try { await toolsStore.fetchToolsData() }
    catch { ElMessage.error(t('tools.toolsDataLoadFailed')) }
  })

  onBeforeUnmount(() => {
    if (actionDebounceTimer) { window.clearTimeout(actionDebounceTimer); actionDebounceTimer = null }
  })

  return {
    toolsStore, loading,
    digFormRef, dnssecFormRef, globalFormRef, ipFormRef,
    digForm, dnssecForm, globalTestForm, ipForm,
    digLoading, digCopying, digHistoryDeletingId,
    dnssecLoading, dnssecExporting,
    globalLoading, globalExporting,
    ipQueryLoading, ipExporting, ipCopyingAll,
    digHistoryPager, globalPager, globalSorter, ipPager,
    globalDetailVisible, globalDetailRow,
    ipValidationMessage,
    digRules, dnssecRules, globalRules, ipRules,
    sortedGlobalRows, pagedGlobalRows, pagedBatchIpRows, pagedDigHistory,
    dnssecStatsCards, dnssecDomainRows,
    tagClass, riskClass, riskLevel, riskLabel,
    runDig, copyDigResult, clearDigOutput, reuseDigHistory, removeDigHistory,
    runDnssec, copyDnssecReport, exportDnssecReport,
    runGlobal, openGlobalDetail, onGlobalSortChange, exportGlobalRows,
    runIpQuery, copyIpInfo, copyAllIpInfo, exportIpRows,
  }
}

export default useTools
