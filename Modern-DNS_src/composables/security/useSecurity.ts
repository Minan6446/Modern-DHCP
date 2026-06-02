import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElLoading } from 'element-plus'
import type { FormInstance, UploadFile } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useSecurityStore } from '../../stores/security'
import type { SecurityBlackWhiteRule, SecurityDdosDomainRule, SecurityDnssecRow } from '../../types/modules'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'

type SortOrder = 'ascending' | 'descending' | null
type SortChangePayload = { prop: string | null; order: SortOrder }
type RuleAction = 'enable' | 'disable' | 'delete'

const ipPattern = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}(\/(3[0-2]|[12]?\d))?$/
const domainPattern = /^(?=.{1,253}$)(\*\.)?(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}$/

export const formatDateTime = (value: string): string => {
  if (!value) return '--'
  const normalized = String(value).replace('T', ' ').replace(/\//g, '-').trim()
  const date = new Date(normalized)
  if (!Number.isNaN(date.getTime())) {
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`
  }
  return normalized.length >= 19 ? normalized.slice(0, 19) : normalized
}

export const formatCompactNumber = (value: number): string => {
  const abs = Math.abs(value)
  if (abs >= 100_000_000) return `${(value / 100_000_000).toFixed(2)}亿`
  if (abs >= 10_000) return `${(value / 10_000).toFixed(1)}万`
  return `${value}`
}

export const useSecurity = () => {
  const { t } = useI18n()
  const securityStore = useSecurityStore()

  const loading = computed(() => securityStore.loading)
  const submitting = computed(() => securityStore.submitting)
  const checking = computed(() => securityStore.checking)

  const bwFilters = reactive({ keyword: '', type: '', listType: '', status: '' })
  const bwAppliedFilters = reactive({ keyword: '', type: '', listType: '', status: '' })
  const bwSorter = reactive<{ prop: string; order: SortOrder }>({ prop: 'createdAt', order: 'descending' })
  const bwPager = reactive({ page: 1, size: 10 })
  const selectedBwRows = ref<SecurityBlackWhiteRule[]>([])
  const bwDialogVisible = ref(false)
  const bwFormRef = ref<FormInstance>()
  const bwForm = reactive({ id: null as number | null, type: 'IP', listType: '黑名单', value: '', remark: '', status: '启用' })

  const importDialogVisible = ref(false)
  const refreshLoading = ref(false)
  const batchRuleLoading = ref(false)
  const deletingRuleId = ref<number | null>(null)
  const deletingDdosRuleId = ref<number | null>(null)
  const statusSwitchingRuleId = ref<number | null>(null)
  const exportLoading = ref(false)
  const importLoading = ref(false)

  const ddosFormRef = ref<FormInstance>()
  const ddosGlobalForm = reactive({
    enabled: true,
    qpsLimit: 10000,
    currentQps: 0,
    perIpConnLimit: 0,
    memSoftMb: 0,
    memHardMb: 0,
    perIpQps: 0,
    perIpBurst: 0,
  })
  const ddosRuleDialogVisible = ref(false)
  const ddosRuleFormRef = ref<FormInstance>()
  const ddosRuleForm = reactive({ id: null as number | null, domain: '', qpsLimit: 1000, status: '启用' })
  const dnssecCheckingRowId = ref<number | null>(null)

  const dnssecFilters = reactive({ keyword: '' })
  const dnssecAppliedFilters = reactive({ keyword: '' })
  const dnssecPager = reactive({ page: 1, size: 10 })
  const dnssecSorter = reactive<{ prop: string; order: SortOrder }>({ prop: 'lastCheckAt', order: 'descending' })
  const dnssecDetailDialogVisible = ref(false)
  const dnssecDetail = ref<SecurityDnssecRow | null>(null)

  let bwFilterDebounceTimer: number | null = null
  let dnssecFilterDebounceTimer: number | null = null
  let qpsStepDebounceTimer: number | null = null

  const validateRuleValue = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
    if (!value) { callback(new Error(t('security.ruleValueRequired'))); return }
    if (bwForm.type === 'IP' && !ipPattern.test(value)) { callback(new Error(t('security.invalidIpOrCidr'))); return }
    if (bwForm.type === '域名' && !domainPattern.test(value)) { callback(new Error(t('security.invalidDomainWildcard'))); return }
    callback()
  }

  const validateDdosDomain = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
    if (!value) { callback(new Error(t('security.domainRequired'))); return }
    if (!domainPattern.test(value)) { callback(new Error(t('security.invalidDomain'))); return }
    callback()
  }

  const bwRules = {
    type: [{ required: true, message: t('security.selectType'), trigger: 'change' }],
    listType: [{ required: true, message: t('security.selectListType'), trigger: 'change' }],
    value: [{ required: true, message: t('security.ruleValueRequired'), trigger: ['blur', 'change'] }, { validator: validateRuleValue, trigger: ['blur', 'change'] }],
  }

  const ddosGlobalRules = { qpsLimit: [{ required: true, message: t('security.totalQpsLimitRequired'), trigger: ['blur', 'change'] }] }
  const ddosRuleRules = {
    domain: [{ validator: validateDdosDomain, trigger: ['blur', 'change'] }],
    qpsLimit: [{ required: true, message: t('security.qpsLimitRequired'), trigger: ['blur', 'change'] }],
  }

  const syncDdosGlobal = () => { Object.assign(ddosGlobalForm, JSON.parse(JSON.stringify(securityStore.ddosGlobal))) }

  const qpsUsagePercent = computed(() => Math.min(100, Math.round((ddosGlobalForm.currentQps / Math.max(ddosGlobalForm.qpsLimit, 1)) * 100)))
  const qpsProgressColor = computed(() => {
    if (qpsUsagePercent.value > 90) return '#F53F3F'
    if (qpsUsagePercent.value >= 70) return '#FF7D00'
    return '#1677FF'
  })

  const filteredBwRows = computed(() =>
    securityStore.blackWhiteRules.filter((item) => {
      const keyword = bwAppliedFilters.keyword.trim()
      return (!keyword || item.ruleId.includes(keyword) || item.value.includes(keyword))
        && (!bwAppliedFilters.type || item.type === bwAppliedFilters.type)
        && (!bwAppliedFilters.listType || item.listType === bwAppliedFilters.listType)
        && (!bwAppliedFilters.status || item.status === bwAppliedFilters.status)
    }),
  )

  const sortedBwRows = computed(() => {
    const rows = [...filteredBwRows.value]
    if (!bwSorter.prop || !bwSorter.order) return rows
    const factor = bwSorter.order === 'ascending' ? 1 : -1
    return rows.sort((a, b) => String(a[bwSorter.prop] || '').localeCompare(String(b[bwSorter.prop] || '')) * factor)
  })

  const pagedBwRows = computed(() => sortedBwRows.value.slice((bwPager.page - 1) * bwPager.size, bwPager.page * bwPager.size))

  const sortedDnssecRows = computed(() => {
    const rows = securityStore.dnssecRows.filter((item) => !dnssecAppliedFilters.keyword || item.domain.includes(dnssecAppliedFilters.keyword))
    if (!dnssecSorter.prop || !dnssecSorter.order) return rows
    const factor = dnssecSorter.order === 'ascending' ? 1 : -1
    return [...rows].sort((a, b) => String(a[dnssecSorter.prop] || '').localeCompare(String(b[dnssecSorter.prop] || '')) * factor)
  })

  const pagedDnssecRows = computed(() => sortedDnssecRows.value.slice((dnssecPager.page - 1) * dnssecPager.size, dnssecPager.page * dnssecPager.size))

  const statsCards = computed(() => [
    { label: t('security.dnssecOpenedDomains'), key: 'opened', value: formatCompactNumber(securityStore.dnssecStats.opened) },
    { label: t('security.dnssecValidDomains'), key: 'valid', value: formatCompactNumber(securityStore.dnssecStats.valid) },
    { label: t('security.dnssecAbnormalDomains'), key: 'abnormal', value: formatCompactNumber(securityStore.dnssecStats.abnormal) },
    { label: t('security.dnssecUnopenedDomains'), key: 'unopened', value: formatCompactNumber(securityStore.dnssecStats.unopened) },
  ])

  const blackWhiteRowClassName = ({ row }: { row: SecurityBlackWhiteRule }) => (row.listType === '黑名单' ? 'bw-row-black' : 'bw-row-white')
  const dnssecRowClassName = ({ row }: { row: SecurityDnssecRow }) => (row.signatureStatus === '异常' ? 'dnssec-row-abnormal' : '')

  const refreshSecurityData = async () => {
    if (refreshLoading.value) return
    refreshLoading.value = true
    try {
      await securityStore.fetchSecurityData()
      syncDdosGlobal()
      ElMessage.success(t('security.refreshSuccess'))
    } catch {
      ElMessage.error(t('security.refreshFailed'))
    } finally {
      refreshLoading.value = false
    }
  }

  const openBwDialog = (row?: SecurityBlackWhiteRule) => {
    Object.assign(bwForm, row || { id: null, type: 'IP', listType: '黑名单', value: '', remark: '', status: '启用' })
    bwDialogVisible.value = true
    nextTick(() => { bwFormRef.value?.clearValidate() })
  }

  const submitBwRule = async () => {
    if (securityStore.submitting) return
    try {
      if (!await validateFormAndFocus(bwFormRef.value)) return
      await securityStore.saveBlackWhiteRule({ ...bwForm })
      bwDialogVisible.value = false
      ElMessage.success(t('security.ruleSaved'))
    } catch {
      ElMessage.error(t('security.ruleSaveFailed'))
    }
  }

  const handleRuleStatusChange = async (row: SecurityBlackWhiteRule, enabled: boolean) => {
    if (statusSwitchingRuleId.value) return
    statusSwitchingRuleId.value = row.id
    try {
      await securityStore.batchUpdateBlackWhiteStatus([row.id], enabled ? '启用' : '禁用')
      ElMessage.success(t('security.ruleStatusChanged', { status: enabled ? t('common.enabled') : t('common.disabled') }))
    } catch {
      ElMessage.error(t('security.statusUpdateFailed'))
    } finally {
      statusSwitchingRuleId.value = null
    }
  }

  const removeRule = async (row: SecurityBlackWhiteRule) => {
    if (deletingRuleId.value || batchRuleLoading.value) return
    deletingRuleId.value = row.id
    try {
      await confirmRiskAction({ title: t('security.deleteConfirmTitle'), action: t('security.deleteBwRuleAction'), target: row.ruleId, risk: t('security.deleteBwRuleRisk') })
      await securityStore.deleteBlackWhiteRule(row.id)
      ElMessage.success(t('security.ruleDeleted'))
    } catch (e) {
      if (e !== 'cancel') ElMessage.error(t('security.deleteFailedRetry'))
    } finally {
      deletingRuleId.value = null
    }
  }

  const batchOperateRules = async (action: RuleAction) => {
    if (batchRuleLoading.value) return
    if (!selectedBwRows.value.length) { ElMessage.warning(t('security.selectRuleFirst')); return }
    batchRuleLoading.value = true
    const actionMap = {
      enable: { title: t('security.batchEnableTitle'), message: t('security.batchEnableAction'), status: '启用', success: t('security.batchEnableDone') },
      disable: { title: t('security.batchDisableTitle'), message: t('security.batchDisableAction'), status: '禁用', success: t('security.batchDisableDone') },
      delete: { title: t('security.batchDeleteTitle'), message: t('security.batchDeleteAction'), status: '', success: t('security.batchDeleteDone') },
    }
    const config = actionMap[action]
    const ids = selectedBwRows.value.map((item) => item.id)
    try {
      await confirmRiskAction({ title: config.title, action: config.message, risk: t('security.batchOperateRisk') })
      if (action === 'delete') await securityStore.batchDeleteBlackWhiteRules(ids)
      else await securityStore.batchUpdateBlackWhiteStatus(ids, config.status)
      selectedBwRows.value = []
      ElMessage.success(config.success)
    } catch (e) {
      if (e !== 'cancel') ElMessage.error(t('security.batchOperateFailed'))
    } finally {
      batchRuleLoading.value = false
    }
  }

  const exportRules = async (command: string) => {
    if (exportLoading.value) return
    exportLoading.value = true
    const [scope, format] = command.split('-')
    const sourceRows = ({ all: securityStore.blackWhiteRules, selected: selectedBwRows.value, filtered: filteredBwRows.value } as Record<string, SecurityBlackWhiteRule[]>)[scope] || []
    if (!sourceRows.length) { ElMessage.warning(t('security.noExportData')); exportLoading.value = false; return }
    const normalized = sourceRows.map((item) => ({
      [t('security.ruleId')]: item.ruleId,
      [t('common.type')]: item.type,
      [t('security.listType')]: item.listType,
      [t('common.value')]: item.value,
      [t('common.remark')]: item.remark,
      [t('common.status')]: item.status,
      [t('common.createdAt')]: formatDateTime(item.createdAt),
    }))
    const downloadFile = (name: string, content: string, type: string) => {
      const url = URL.createObjectURL(new Blob([content], { type }))
      const link = document.createElement('a')
      link.href = url; link.download = name; link.click(); URL.revokeObjectURL(url)
    }
    const loadingInstance = ElLoading.service({ text: t('security.exporting'), background: 'rgba(0,0,0,0.35)' })
    try {
      if (format === 'json') {
        downloadFile('security-rules.json', JSON.stringify(normalized, null, 2), 'application/json;charset=utf-8')
      } else {
        const XLSX = await import('xlsx')
        const wb = XLSX.utils.book_new()
        XLSX.utils.book_append_sheet(wb, XLSX.utils.json_to_sheet(normalized), 'SecurityRules')
        XLSX.writeFile(wb, 'security-rules.xlsx')
      }
      ElMessage.success(t('security.exportSuccess'))
    } catch {
      ElMessage.error(t('security.exportFailed'))
    } finally {
      exportLoading.value = false
      loadingInstance.close()
    }
  }

  const importRules = async (file: UploadFile) => {
    if (importLoading.value) return false
    importLoading.value = true
    const loadingInstance = ElLoading.service({ text: t('security.importing'), background: 'rgba(0,0,0,0.35)' })
    try {
      const raw = await (file.raw as File).arrayBuffer()
      let rows: any[] = []
      if (file.name.toLowerCase().endsWith('.json')) {
        rows = JSON.parse(new TextDecoder().decode(raw))
      } else {
        const XLSX = await import('xlsx')
        const wb = XLSX.read(raw)
        rows = XLSX.utils.sheet_to_json(wb.Sheets[wb.SheetNames[0]])
      }
      await securityStore.importBlackWhiteRules(rows)
      importDialogVisible.value = false
      ElMessage.success(t('security.importSuccess', { count: rows.length }))
    } catch {
      ElMessage.error(t('security.importFailed'))
    } finally {
      importLoading.value = false
      loadingInstance.close()
    }
    return false
  }

  const saveGlobalDdos = async () => {
    if (securityStore.submitting) return
    try {
      if (!await validateFormAndFocus(ddosFormRef.value)) return
      if (ddosGlobalForm.qpsLimit < 1 || ddosGlobalForm.qpsLimit > 100000) { ElMessage.warning(t('security.totalQpsLimitRange')); return }
      await confirmRiskAction({ title: t('security.saveConfirmTitle'), action: t('security.saveGlobalQpsAction'), risk: t('security.saveGlobalQpsRisk') })
      await securityStore.saveDdosGlobalConfig({ ...ddosGlobalForm })
      ElMessage.success(t('security.globalQpsSaved'))
    } catch (e) {
      if (e !== 'cancel') ElMessage.error(t('security.saveFailedRetry'))
    }
  }

  const resetGlobalDdos = async () => {
    try {
      await confirmRiskAction({ title: t('security.resetConfirmTitle'), action: t('security.resetGlobalQpsAction'), risk: t('security.resetGlobalQpsRisk') })
      await securityStore.fetchSecurityData()
      syncDdosGlobal()
      ElMessage.success(t('security.globalQpsReset'))
    } catch (e) {
      if (e !== 'cancel') ElMessage.error(t('security.resetFailed'))
    }
  }

  const openDdosRuleDialog = (row?: SecurityDdosDomainRule) => {
    Object.assign(ddosRuleForm, row || { id: null, domain: '', qpsLimit: 1000, status: '启用' })
    ddosRuleDialogVisible.value = true
    nextTick(() => { ddosRuleFormRef.value?.clearValidate() })
  }

  const submitDdosRule = async () => {
    if (securityStore.submitting) return
    try {
      if (!await validateFormAndFocus(ddosRuleFormRef.value)) return
      if (ddosRuleForm.qpsLimit < 1 || ddosRuleForm.qpsLimit > 100000) { ElMessage.warning(t('security.domainQpsLimitRange')); return }
      await securityStore.saveDdosDomainRule({ ...ddosRuleForm })
      ddosRuleDialogVisible.value = false
      ElMessage.success(t('security.rateLimitSaved'))
    } catch {
      ElMessage.error(t('security.rateLimitSaveFailed'))
    }
  }

  const removeDdosRule = async (row: SecurityDdosDomainRule) => {
    if (deletingDdosRuleId.value) return
    deletingDdosRuleId.value = row.id
    try {
      await confirmRiskAction({ title: t('security.deleteConfirmTitle'), action: t('security.deleteRateLimitAction'), target: row.domain, risk: t('security.deleteRateLimitRisk') })
      await securityStore.deleteDdosDomainRule(row.id)
      ElMessage.success(t('security.rateLimitDeleted'))
    } catch (e) {
      if (e !== 'cancel') ElMessage.error(t('security.deleteFailedRetry'))
    } finally {
      deletingDdosRuleId.value = null
    }
  }

  const runCheckAll = async () => {
    try {
      await securityStore.checkAllDnssec()
      ElMessage.success(t('security.checkAllDone'))
    } catch {
      ElMessage.error(t('security.checkFailed'))
    }
  }

  const runCheckOne = async (row: SecurityDnssecRow) => {
    if (dnssecCheckingRowId.value || checking.value) return
    dnssecCheckingRowId.value = row.id
    try {
      await securityStore.checkDnssecById(row.id)
      ElMessage.success(t('security.recheckDone', { domain: row.domain }))
    } catch {
      ElMessage.error(t('security.recheckFailed'))
    } finally {
      dnssecCheckingRowId.value = null
    }
  }

  const openDnssecDetail = (row: SecurityDnssecRow) => {
    dnssecDetail.value = row
    dnssecDetailDialogVisible.value = true
  }

  const copyDnssecKeys = async () => {
    if (!dnssecDetail.value) return
    const lines = [
      `${t('cache.domain')}: ${dnssecDetail.value.domain}`,
      `${t('security.kskKeyInfo')}:`, ...(dnssecDetail.value.ksk || []).map((item: any) => `- ${item.keyId} / ${item.createdAt} / ${item.status}`),
      `${t('security.zskKeyInfo')}:`, ...(dnssecDetail.value.zsk || []).map((item: any) => `- ${item.keyId} / ${item.createdAt} / ${item.status}`),
    ]
    try {
      await navigator.clipboard.writeText(lines.join('\n'))
      ElMessage.success(t('security.keyCopied'))
    } catch {
      ElMessage.error(t('security.copyFailed'))
    }
  }

  const handleBwSortChange = ({ prop, order }: SortChangePayload) => {
    bwSorter.prop = prop || 'createdAt'
    bwSorter.order = order || 'descending'
  }

  const handleDnssecSortChange = ({ prop, order }: SortChangePayload) => {
    dnssecSorter.prop = prop || 'lastCheckAt'
    dnssecSorter.order = order || 'descending'
  }

  watch(
    () => [bwFilters.keyword, bwFilters.type, bwFilters.listType, bwFilters.status],
    () => {
      bwPager.page = 1
      if (bwFilterDebounceTimer) window.clearTimeout(bwFilterDebounceTimer)
      bwFilterDebounceTimer = window.setTimeout(() => {
        Object.assign(bwAppliedFilters, { keyword: bwFilters.keyword.trim(), type: bwFilters.type, listType: bwFilters.listType, status: bwFilters.status })
        bwFilterDebounceTimer = null
      }, 300)
    },
    { immediate: true },
  )

  watch(
    () => dnssecFilters.keyword,
    () => {
      dnssecPager.page = 1
      if (dnssecFilterDebounceTimer) window.clearTimeout(dnssecFilterDebounceTimer)
      dnssecFilterDebounceTimer = window.setTimeout(() => {
        dnssecAppliedFilters.keyword = dnssecFilters.keyword.trim()
        dnssecFilterDebounceTimer = null
      }, 300)
    },
    { immediate: true },
  )

  watch(
    () => ddosGlobalForm.qpsLimit,
    (value) => {
      if (qpsStepDebounceTimer) window.clearTimeout(qpsStepDebounceTimer)
      qpsStepDebounceTimer = window.setTimeout(() => {
        const normalized = Math.min(100000, Math.max(1, Number(value) || 1))
        if (normalized !== ddosGlobalForm.qpsLimit) ddosGlobalForm.qpsLimit = normalized
        qpsStepDebounceTimer = null
      }, 300)
    },
  )

  watch(() => sortedBwRows.value.length, (length) => {
    const maxPage = Math.max(1, Math.ceil(length / bwPager.size))
    if (bwPager.page > maxPage) bwPager.page = maxPage
    selectedBwRows.value = []
  })

  watch(() => sortedDnssecRows.value.length, (length) => {
    const maxPage = Math.max(1, Math.ceil(length / dnssecPager.size))
    if (dnssecPager.page > maxPage) dnssecPager.page = maxPage
  })

  watch(() => bwForm.type, () => { bwFormRef.value?.validateField('value', () => undefined) })

  onMounted(async () => {
    try {
      await securityStore.fetchSecurityData()
      syncDdosGlobal()
    } catch {
      ElMessage.error(t('security.initFailed'))
    }
  })

  onBeforeUnmount(() => {
    if (bwFilterDebounceTimer) { window.clearTimeout(bwFilterDebounceTimer); bwFilterDebounceTimer = null }
    if (dnssecFilterDebounceTimer) { window.clearTimeout(dnssecFilterDebounceTimer); dnssecFilterDebounceTimer = null }
    if (qpsStepDebounceTimer) { window.clearTimeout(qpsStepDebounceTimer); qpsStepDebounceTimer = null }
  })

  return {
    securityStore,
    loading, submitting, checking,
    bwFilters, bwPager, bwSorter,
    selectedBwRows, bwDialogVisible, bwFormRef, bwForm, bwRules,
    importDialogVisible, refreshLoading, batchRuleLoading,
    deletingRuleId, deletingDdosRuleId, statusSwitchingRuleId,
    exportLoading, importLoading,
    ddosFormRef, ddosGlobalForm, ddosGlobalRules,
    ddosRuleDialogVisible, ddosRuleFormRef, ddosRuleForm, ddosRuleRules,
    dnssecCheckingRowId,
    dnssecFilters, dnssecPager,
    dnssecDetailDialogVisible, dnssecDetail,
    qpsUsagePercent, qpsProgressColor,
    filteredBwRows, sortedBwRows, pagedBwRows,
    sortedDnssecRows, pagedDnssecRows, statsCards,
    blackWhiteRowClassName, dnssecRowClassName,
    formatDateTime, formatCompactNumber,
    refreshSecurityData,
    openBwDialog, submitBwRule,
    handleRuleStatusChange, removeRule, batchOperateRules,
    exportRules, importRules,
    saveGlobalDdos, resetGlobalDdos,
    openDdosRuleDialog, submitDdosRule, removeDdosRule,
    runCheckAll, runCheckOne,
    openDnssecDetail, copyDnssecKeys,
    handleBwSortChange, handleDnssecSortChange,
  }
}

export default useSecurity
