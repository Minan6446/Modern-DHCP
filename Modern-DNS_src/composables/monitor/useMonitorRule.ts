import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useMonitorStore } from '../../stores/monitor'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'
import { loadTableState, saveTableState } from '../../utils/tableState'

type SortOrder = 'ascending' | 'descending' | null
type SortState = { prop: string; order: SortOrder }
type SortChangePayload = { prop: string | null; order: SortOrder }

interface RuleViewState {
  historyPager: { page: number; size: number }
  historySorter: SortState
}

const RULE_STATE_KEY = 'modern-dns:monitor:rule-state'

const sortedRows = <T extends Record<string, any>>(rows: T[], sorter: SortState): T[] => {
  const next = [...rows]
  if (!sorter.prop || !sorter.order) return next
  const factor = sorter.order === 'ascending' ? 1 : -1
  return next.sort((a, b) => String(a[sorter.prop] || '').localeCompare(String(b[sorter.prop] || '')) * factor)
}

const pageRows = <T>(rows: T[], pager: { page: number; size: number }): T[] => {
  const start = (pager.page - 1) * pager.size
  return rows.slice(start, start + pager.size)
}

export const formatDateTime = (value: string): string => {
  const date = new Date(String(value || '').replace(' ', 'T'))
  if (Number.isNaN(date.getTime())) return String(value || '--')
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  const h = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  const s = String(date.getSeconds()).padStart(2, '0')
  return `${y}-${m}-${d} ${h}:${mm}:${s}`
}

export const statusClass = (status: string): string => {
  if (status === '成功' || status === 'NOERROR' || status === '已处理') return 'tag-success'
  if (status === 'NXDOMAIN' || status === '警告' || status === '处理中') return 'tag-warning'
  if (status === '失败' || status === 'SERVFAIL' || status === 'REFUSED' || status === '未处理') return 'tag-danger'
  return 'tag-muted'
}

export const useMonitorRule = () => {
  const { t } = useI18n()
  const monitorStore = useMonitorStore()

  const submitting = computed(() => monitorStore.submitting)
  const loading = computed(() => monitorStore.loading)

  const cachedState = loadTableState<RuleViewState>(RULE_STATE_KEY, {
    historyPager: { page: 1, size: 8 },
    historySorter: { prop: 'triggerAt', order: 'descending' },
  })

  const historyPager = reactive({ ...cachedState.historyPager })
  const historySorter = reactive<SortState>({ ...cachedState.historySorter })

  const qpsFormRef = ref<FormInstance>()
  const nxdomainFormRef = ref<FormInstance>()
  const qpsForm = reactive({ globalThresholdPercent: 50, domainThresholdPercent: 100, periodSec: 60, enabled: true })
  const nxdomainForm = reactive({ thresholdPercent: 200, periodSec: 60, enabled: true })
  const latencyForm = reactive({ thresholdMs: 200, periodSec: 60, enabled: false })
  const cacheHitForm = reactive({ minHitPercent: 70, periodSec: 300, enabled: false })
  const qpsSaving = ref(false)
  const nxdomainSaving = ref(false)
  const latencySaving = ref(false)
  const cacheHitSaving = ref(false)
  const latencyFormRef = ref<FormInstance>()
  const cacheHitFormRef = ref<FormInstance>()

  let ruleDebounceTimer: number | null = null

  const sortedRuleHistory = computed(() => sortedRows(monitorStore.ruleHistory, historySorter))
  const pagedRuleHistory = computed(() => pageRows(sortedRuleHistory.value, historyPager))

  const qpsRules = {
    globalThresholdPercent: [{ required: true, message: t('monitor.enterGlobalQpsThreshold'), trigger: ['blur', 'change'] }],
    domainThresholdPercent: [{ required: true, message: t('monitor.enterDomainQpsThreshold'), trigger: ['blur', 'change'] }],
    periodSec: [{ required: true, message: t('monitor.enterCheckPeriod'), trigger: ['blur', 'change'] }],
  }

  const nxdomainRules = {
    thresholdPercent: [{ required: true, message: t('monitor.enterNxdomainThreshold'), trigger: ['blur', 'change'] }],
    periodSec: [{ required: true, message: t('monitor.enterCheckPeriod'), trigger: ['blur', 'change'] }],
  }

  const latencyRules = {
    thresholdMs: [{ required: true, message: t('monitor.enterLatencyThreshold'), trigger: ['blur', 'change'] }],
    periodSec: [{ required: true, message: t('monitor.enterCheckPeriod'), trigger: ['blur', 'change'] }],
  }

  const cacheHitRules = {
    minHitPercent: [{ required: true, message: t('monitor.enterMinHitRate'), trigger: ['blur', 'change'] }],
    periodSec: [{ required: true, message: t('monitor.enterCheckPeriod'), trigger: ['blur', 'change'] }],
  }

  const syncRuleForms = () => {
    Object.assign(qpsForm, JSON.parse(JSON.stringify(monitorStore.qpsRule)))
    Object.assign(nxdomainForm, JSON.parse(JSON.stringify(monitorStore.nxdomainRule)))
    Object.assign(latencyForm, JSON.parse(JSON.stringify(monitorStore.latencyRule)))
    Object.assign(cacheHitForm, JSON.parse(JSON.stringify(monitorStore.cacheHitRule)))
  }

  const clampRuleInputs = () => {
    if (ruleDebounceTimer) window.clearTimeout(ruleDebounceTimer)
    ruleDebounceTimer = window.setTimeout(() => {
      qpsForm.globalThresholdPercent = Math.min(1000, Math.max(1, Number(qpsForm.globalThresholdPercent) || 1))
      qpsForm.domainThresholdPercent = Math.min(1000, Math.max(1, Number(qpsForm.domainThresholdPercent) || 1))
      qpsForm.periodSec = Math.min(1000, Math.max(1, Number(qpsForm.periodSec) || 1))
      nxdomainForm.thresholdPercent = Math.min(1000, Math.max(1, Number(nxdomainForm.thresholdPercent) || 1))
      nxdomainForm.periodSec = Math.min(1000, Math.max(1, Number(nxdomainForm.periodSec) || 1))
      latencyForm.thresholdMs = Math.min(10000, Math.max(1, Number(latencyForm.thresholdMs) || 1))
      latencyForm.periodSec = Math.min(1000, Math.max(1, Number(latencyForm.periodSec) || 1))
      cacheHitForm.minHitPercent = Math.min(100, Math.max(1, Number(cacheHitForm.minHitPercent) || 1))
      cacheHitForm.periodSec = Math.min(3600, Math.max(1, Number(cacheHitForm.periodSec) || 1))
      ruleDebounceTimer = null
    }, 300)
  }

  watch(
    () => [qpsForm.globalThresholdPercent, qpsForm.domainThresholdPercent, qpsForm.periodSec, nxdomainForm.thresholdPercent, nxdomainForm.periodSec, latencyForm.thresholdMs, latencyForm.periodSec, cacheHitForm.minHitPercent, cacheHitForm.periodSec],
    clampRuleInputs,
  )

  watch(
    () => ({ historyPager: { ...historyPager }, historySorter: { ...historySorter } }),
    (next) => saveTableState(RULE_STATE_KEY, next),
    { deep: true },
  )

  watch(
    () => sortedRuleHistory.value.length,
    (length) => {
      const maxPage = Math.max(1, Math.ceil(length / historyPager.size))
      if (historyPager.page > maxPage) historyPager.page = maxPage
    },
  )

  const onHistorySortChange = ({ prop, order }: SortChangePayload) => {
    historySorter.prop = prop || 'triggerAt'
    historySorter.order = order || 'descending'
  }

  const saveQpsRule = async () => {
    if (qpsSaving.value) return
    qpsSaving.value = true
    try {
      const valid = await validateFormAndFocus(qpsFormRef.value)
      if (!valid) return
      await monitorStore.saveQpsRule({ ...qpsForm })
      ElMessage.success(t('monitor.qpsRuleSaved'))
    } catch (_error) {
      ElMessage.error(t('common.saveFailed'))
    } finally {
      qpsSaving.value = false
    }
  }

  const resetQpsRule = async () => {
    try {
      await confirmRiskAction({ title: t('common.resetConfirmTitle'), action: t('monitor.resetQpsRuleAction'), risk: t('monitor.resetRuleRisk') })
      await monitorStore.resetQpsRule()
      syncRuleForms()
      ElMessage.success(t('monitor.qpsRuleReset'))
    } catch (_error) {
      if (_error !== 'cancel') ElMessage.error(t('common.resetFailed'))
    }
  }

  const saveNxdomainRule = async () => {
    if (nxdomainSaving.value) return
    nxdomainSaving.value = true
    try {
      const valid = await validateFormAndFocus(nxdomainFormRef.value)
      if (!valid) return
      await monitorStore.saveNxdomainRule({ ...nxdomainForm })
      ElMessage.success(t('monitor.nxdomainRuleSaved'))
    } catch (_error) {
      ElMessage.error(t('common.saveFailed'))
    } finally {
      nxdomainSaving.value = false
    }
  }

  const resetNxdomainRule = async () => {
    try {
      await confirmRiskAction({ title: t('common.resetConfirmTitle'), action: t('monitor.resetNxdomainRuleAction'), risk: t('monitor.resetRuleRisk') })
      await monitorStore.resetNxdomainRule()
      syncRuleForms()
      ElMessage.success(t('monitor.nxdomainRuleReset'))
    } catch (_error) {
      if (_error !== 'cancel') ElMessage.error(t('common.resetFailed'))
    }
  }

  const saveLatencyRule = async () => {
    if (latencySaving.value) return
    latencySaving.value = true
    try {
      const valid = await validateFormAndFocus(latencyFormRef.value)
      if (!valid) return
      await monitorStore.saveLatencyRule({ ...latencyForm })
      ElMessage.success(t('monitor.latencyRuleSaved'))
    } catch (_error) {
      ElMessage.error(t('common.saveFailed'))
    } finally {
      latencySaving.value = false
    }
  }

  const resetLatencyRule = async () => {
    try {
      await confirmRiskAction({ title: t('common.resetConfirmTitle'), action: t('monitor.resetLatencyRuleAction'), risk: t('monitor.resetRuleRisk') })
      await monitorStore.resetLatencyRule()
      syncRuleForms()
      ElMessage.success(t('monitor.latencyRuleReset'))
    } catch (_error) {
      if (_error !== 'cancel') ElMessage.error(t('common.resetFailed'))
    }
  }

  const saveCacheHitRule = async () => {
    if (cacheHitSaving.value) return
    cacheHitSaving.value = true
    try {
      const valid = await validateFormAndFocus(cacheHitFormRef.value)
      if (!valid) return
      await monitorStore.saveCacheHitRule({ ...cacheHitForm })
      ElMessage.success(t('monitor.cacheHitRuleSaved'))
    } catch (_error) {
      ElMessage.error(t('common.saveFailed'))
    } finally {
      cacheHitSaving.value = false
    }
  }

  const resetCacheHitRule = async () => {
    try {
      await confirmRiskAction({ title: t('common.resetConfirmTitle'), action: t('monitor.resetCacheHitRuleAction'), risk: t('monitor.resetRuleRisk') })
      await monitorStore.resetCacheHitRule()
      syncRuleForms()
      ElMessage.success(t('monitor.cacheHitRuleReset'))
    } catch (_error) {
      if (_error !== 'cancel') ElMessage.error(t('common.resetFailed'))
    }
  }

  onMounted(async () => {
    try {
      await monitorStore.fetchMonitorData()
      syncRuleForms()
    } catch (_error) {
      ElMessage.error(t('monitor.ruleDataLoadFailed'))
    }
  })

  onBeforeUnmount(() => {
    if (ruleDebounceTimer) {
      window.clearTimeout(ruleDebounceTimer)
      ruleDebounceTimer = null
    }
  })

  return {
    loading,
    submitting,
    qpsFormRef,
    nxdomainFormRef,
    latencyFormRef,
    cacheHitFormRef,
    qpsForm,
    nxdomainForm,
    latencyForm,
    cacheHitForm,
    qpsSaving,
    nxdomainSaving,
    latencySaving,
    cacheHitSaving,
    qpsRules,
    nxdomainRules,
    latencyRules,
    cacheHitRules,
    historyPager,
    historySorter,
    sortedRuleHistory,
    pagedRuleHistory,
    formatDateTime,
    statusClass,
    syncRuleForms,
    onHistorySortChange,
    saveQpsRule,
    resetQpsRule,
    saveNxdomainRule,
    resetNxdomainRule,
    saveLatencyRule,
    resetLatencyRule,
    saveCacheHitRule,
    resetCacheHitRule,
  }
}

export default useMonitorRule
