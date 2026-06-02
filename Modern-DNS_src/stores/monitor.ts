import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  getMonitorModuleData,
  getRealTimeLogsApi,
  refreshMonitorRealtime,
  resetNxdomainRuleApi,
  resetQpsRuleApi,
  saveNxdomainRuleApi,
  saveQpsRuleApi,
  saveLatencyRuleApi,
  resetLatencyRuleApi,
  saveCacheHitRuleApi,
  resetCacheHitRuleApi,
  handleRuleHistoryApi,
} from '../api/monitor'
import type {
  MonitorModuleData,
  MonitorRealTimeRow,
  MonitorResolveLogRow,
  MonitorReportData,
  MonitorRuleHistory,
  MonitorRuleNxdomain,
  MonitorRuleQps,
  MonitorRuleLatency,
  MonitorRuleCacheHit,
} from '../types/modules'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

export const useMonitorStore = defineStore('monitor', () => {
  const loading = ref(false)
  const submitting = ref(false)
  const realTimeRows = ref<MonitorRealTimeRow[]>([])
  const resolveLogs = ref<MonitorResolveLogRow[]>([])
  const qpsRule = ref<MonitorRuleQps>({
    globalThresholdPercent: 50,
    domainThresholdPercent: 100,
    periodSec: 60,
    enabled: true,
  })
  const nxdomainRule = ref<MonitorRuleNxdomain>({
    thresholdPercent: 200,
    periodSec: 60,
    enabled: true,
  })
  const latencyRule = ref<MonitorRuleLatency>({
    thresholdMs: 200,
    periodSec: 60,
    enabled: false,
  })
  const cacheHitRule = ref<MonitorRuleCacheHit>({
    minHitPercent: 70,
    periodSec: 300,
    enabled: false,
  })
  const ruleHistory = ref<MonitorRuleHistory[]>([])
  const report = ref<MonitorReportData>({
    topDomain: [],
    topIp: [],
    heatmap: { regions: [], periods: [], values: [] },
    statusDistribution: [],
  })
  const lastUpdated = ref('')

  const qpsRuleDefault = ref<MonitorRuleQps | null>(null)
  const nxdomainRuleDefault = ref<MonitorRuleNxdomain | null>(null)
  const latencyRuleDefault = ref<MonitorRuleLatency | null>(null)
  const cacheHitRuleDefault = ref<MonitorRuleCacheHit | null>(null)

  const fetchMonitorData = async () => {
    loading.value = true
    try {
      const { data } = await getMonitorModuleData() as { data: MonitorModuleData }
      realTimeRows.value = clone(data.realTime || [])
      resolveLogs.value = clone(data.resolveLogs || [])
      qpsRule.value = clone(data.rules?.qps || qpsRule.value)
      nxdomainRule.value = clone(data.rules?.nxdomain || nxdomainRule.value)
      latencyRule.value = clone(data.rules?.latency || latencyRule.value)
      cacheHitRule.value = clone(data.rules?.cacheHit || cacheHitRule.value)
      ruleHistory.value = clone(data.rules?.history || [])
      report.value = clone(data.report || report.value)
      qpsRuleDefault.value = clone(qpsRule.value)
      nxdomainRuleDefault.value = clone(nxdomainRule.value)
      latencyRuleDefault.value = clone(latencyRule.value)
      cacheHitRuleDefault.value = clone(cacheHitRule.value)
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  const refreshRealtime = async () => {
    submitting.value = true
    try {
      await refreshMonitorRealtime()
      const { data } = await getRealTimeLogsApi({ page: 1, size: 50 })
      realTimeRows.value = data.rows ?? []
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      submitting.value = false
    }
  }

  const saveQpsRule = async (payload: MonitorRuleQps): Promise<void> => {
    submitting.value = true
    try {
      await saveQpsRuleApi(payload)
      qpsRule.value = clone(payload)
      await fetchMonitorData()
    } finally {
      submitting.value = false
    }
  }

  const resetQpsRule = async () => {
    submitting.value = true
    try {
      const { data } = await resetQpsRuleApi()
      qpsRule.value = clone(data || qpsRuleDefault.value || qpsRule.value)
    } finally {
      submitting.value = false
    }
  }

  const saveNxdomainRule = async (payload: MonitorRuleNxdomain): Promise<void> => {
    submitting.value = true
    try {
      await saveNxdomainRuleApi(payload)
      nxdomainRule.value = clone(payload)
      await fetchMonitorData()
    } finally {
      submitting.value = false
    }
  }

  const resetNxdomainRule = async () => {
    submitting.value = true
    try {
      const { data } = await resetNxdomainRuleApi()
      nxdomainRule.value = clone(data || nxdomainRuleDefault.value || nxdomainRule.value)
    } finally {
      submitting.value = false
    }
  }

  const saveLatencyRule = async (payload: MonitorRuleLatency): Promise<void> => {
    submitting.value = true
    try {
      await saveLatencyRuleApi(payload)
      latencyRule.value = clone(payload)
      await fetchMonitorData()
    } finally {
      submitting.value = false
    }
  }

  const resetLatencyRule = async () => {
    submitting.value = true
    try {
      const { data } = await resetLatencyRuleApi()
      latencyRule.value = clone(data || latencyRuleDefault.value || latencyRule.value)
    } finally {
      submitting.value = false
    }
  }

  const saveCacheHitRule = async (payload: MonitorRuleCacheHit): Promise<void> => {
    submitting.value = true
    try {
      await saveCacheHitRuleApi(payload)
      cacheHitRule.value = clone(payload)
      await fetchMonitorData()
    } finally {
      submitting.value = false
    }
  }

  const resetCacheHitRule = async () => {
    submitting.value = true
    try {
      const { data } = await resetCacheHitRuleApi()
      cacheHitRule.value = clone(data || cacheHitRuleDefault.value || cacheHitRule.value)
    } finally {
      submitting.value = false
    }
  }

  const handleRuleHistory = async (id: number): Promise<void> => {
    submitting.value = true
    try {
      await handleRuleHistoryApi(id)
      const target = ruleHistory.value.find((item) => item.id === id)
      if (target) target.handleStatus = '已处理'
    } finally {
      submitting.value = false
    }
  }

  const reportStatusTotal = computed(() =>
    report.value.statusDistribution.reduce((sum, item) => sum + Number(item.value || 0), 0),
  )

  return {
    loading,
    submitting,
    realTimeRows,
    resolveLogs,
    qpsRule,
    nxdomainRule,
    latencyRule,
    cacheHitRule,
    ruleHistory,
    report,
    reportStatusTotal,
    lastUpdated,
    fetchMonitorData,
    refreshRealtime,
    saveQpsRule,
    resetQpsRule,
    saveNxdomainRule,
    resetNxdomainRule,
    saveLatencyRule,
    resetLatencyRule,
    saveCacheHitRule,
    resetCacheHitRule,
    handleRuleHistory,
  }
})
