import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  getToolsModuleData,
  lookupIpLocationApi,
  runDigQueryApi,
  runDnssecDebugApi,
  runGlobalTestApi,
  deleteDigHistoryApi,
  clearDigHistoryApi,
} from '../api/tools'
import type {
  DigHistoryItem,
  DnssecDebugResult,
  GlobalTestNode,
  IpLocationRow,
  ToolsModuleData,
} from '../types/modules'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

type DigQueryPayload = {
  domain: string
  recordType: string
  dnsServer?: string
}

type GlobalTestPayload = {
  domain: string
  recordType: string
  nodeGroup: string
}

type IpLookupPayload = {
  ips: string
}

export const useToolsStore = defineStore('tools', () => {
  const loading = ref(false)
  const submitting = ref(false)

  const digHistory = ref<DigHistoryItem[]>([])
  const digOutput = ref('')

  const dnssecResult = ref<DnssecDebugResult | null>(null)
  const globalTestNodes = ref<GlobalTestNode[]>([])
  const ipResults = ref<IpLocationRow[]>([])

  const lastUpdated = ref('')

  const fetchToolsData = async () => {
    loading.value = true
    try {
      const { data } = await getToolsModuleData() as { data: ToolsModuleData }
      digHistory.value = clone(data.dig?.history || [])
      globalTestNodes.value = clone(data.globalTest?.nodes || [])
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  const runDigQuery = async (payload: DigQueryPayload): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await runDigQueryApi(payload)
      digOutput.value = data.output || ''
      digHistory.value.unshift({
        id: Date.now(),
        domain: payload.domain,
        recordType: payload.recordType,
        dnsServer: payload.dnsServer || '系统默认DNS',
        queriedAt: data.queriedAt || new Date().toLocaleString('zh-CN', { hour12: false }),
      })
      digHistory.value = digHistory.value.slice(0, 10)
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      submitting.value = false
    }
  }

  const clearDigOutput = () => {
    digOutput.value = ''
  }

  const deleteDigHistory = async (id: number): Promise<void> => {
    await deleteDigHistoryApi(id)
    digHistory.value = digHistory.value.filter((item) => item.id !== id)
  }

  const clearDigHistory = async (): Promise<void> => {
    submitting.value = true
    try {
      await clearDigHistoryApi()
      digHistory.value = []
    } finally {
      submitting.value = false
    }
  }

  const runDnssecDebug = async (payload: { domain: string }): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await runDnssecDebugApi(payload)
      dnssecResult.value = clone(data)
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      submitting.value = false
    }
  }

  const runGlobalTest = async (payload: GlobalTestPayload): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await runGlobalTestApi(payload)
      const raw: GlobalTestNode[] = (data as any)?.nodes ?? (Array.isArray(data) ? data : [])
      const filtered = raw.filter((item) => {
        if (payload.nodeGroup === 'all') return true
        return item.category === payload.nodeGroup
      })
      globalTestNodes.value = filtered.map((item) => {
        const baseResult = filtered.find((row) => row.result !== '--')?.result || item.result
        if (item.result === '--') return { ...item, consistency: '超时' }
        return { ...item, consistency: item.result === baseResult ? '一致' : '不一致' }
      })
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      submitting.value = false
    }
  }

  const lookupIpLocation = async (payload: IpLookupPayload): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await lookupIpLocationApi(payload)
      ipResults.value = clone(data || [])
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      submitting.value = false
    }
  }

  const singleIpResult = computed(() => (ipResults.value.length === 1 ? ipResults.value[0] : null))
  const batchIpResults = computed(() => (ipResults.value.length > 1 ? ipResults.value : []))
  const dnssecStats = computed(() => {
    if (!dnssecResult.value) {
      return { opened: 0, valid: 0, abnormal: 0, unopened: 0 }
    }
    const opened = dnssecResult.value.dnssecEnabled ? 1 : 0
    const unopened = dnssecResult.value.dnssecEnabled ? 0 : 1
    const hasAbnormal = dnssecResult.value.details.some((item) => item.status === '失败' || item.status === '警告')
    return {
      opened,
      valid: dnssecResult.value.dnssecEnabled && !hasAbnormal ? 1 : 0,
      abnormal: hasAbnormal ? 1 : 0,
      unopened,
    }
  })

  return {
    loading,
    submitting,
    digHistory,
    digOutput,
    dnssecResult,
    globalTestNodes,
    ipResults,
    singleIpResult,
    batchIpResults,
    dnssecStats,
    lastUpdated,
    fetchToolsData,
    runDigQuery,
    clearDigOutput,
    deleteDigHistory,
    clearDigHistory,
    runDnssecDebug,
    runGlobalTest,
    lookupIpLocation,
  }
})
