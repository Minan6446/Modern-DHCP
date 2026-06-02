import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as forwardApiModule from '../api/forward'
import type { ForwardGlobalConfig, ForwardRule, ForwardServer } from '../types/modules'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

export const useForwardStore = defineStore('forward', () => {
  const loading = ref(false)
  const submitting = ref(false)
  const globalConfig = ref<ForwardGlobalConfig>({
    enabled: false,
    publicDns: '',
    publicDnsEnabled: true,
    publicDnsCustom: '',
    providers: [],
    timeout: 5,
    retries: 2,
    strategy: 'priority',
    servers: [],
  })
  const conditionRules = ref<ForwardRule[]>([])
  const lastUpdated = ref('')
  let timerId: number | null = null

  const fetchForwardData = async () => {
    loading.value = true
    try {
      const [globalRes, rulesRes, serversRes] = await Promise.all([
        forwardApiModule.getGlobalForwardConfig(),
        forwardApiModule.getConditionRuleList({ page: 1, size: 999 }),
        forwardApiModule.getForwardServerList({ page: 1, size: 999 }),
      ])
      const rawServers = (serversRes.data as any)
      const serverList = Array.isArray(rawServers) ? rawServers : (rawServers?.list ?? [])
      globalConfig.value = clone({ ...(globalRes.data as any), servers: serverList })
      const rawRules = (rulesRes.data as any)
      conditionRules.value = clone(Array.isArray(rawRules) ? rawRules : (rawRules?.list ?? []))
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  const startPolling = () => {
    stopPolling()
    timerId = window.setInterval(fetchForwardData, 30000)
  }

  const stopPolling = () => {
    if (timerId) {
      window.clearInterval(timerId)
      timerId = null
    }
  }

  const upstreamOptions = computed(() =>
    globalConfig.value.servers.map((item) => ({
      label: `${item.name} / ${item.address}`,
      value: item.id,
    })),
  )

  const saveGlobalConfig = async (payload: ForwardGlobalConfig): Promise<void> => {
    submitting.value = true
    try {
      globalConfig.value = clone(payload)
    } finally {
      submitting.value = false
    }
  }

  const resetGlobalConfig = async () => {
    await fetchForwardData()
  }

  const saveServer = async (payload: Partial<ForwardServer> & Omit<ForwardServer, 'id'>): Promise<void> => {
    submitting.value = true
    try {
      if (payload.id) {
        const target = globalConfig.value.servers.find((item) => item.id === payload.id)
        if (target) {
          Object.assign(target, payload)
        }
      } else {
        globalConfig.value.servers.unshift({ ...payload, id: Date.now() })
      }
    } finally {
      submitting.value = false
    }
  }

  const deleteServer = (id: number): void => {
    globalConfig.value.servers = globalConfig.value.servers.filter((item) => item.id !== id)
    conditionRules.value = conditionRules.value.filter((item) => item.upstreamId !== id)
  }

  const saveConditionRule = async (payload: Partial<ForwardRule> & Omit<ForwardRule, 'id' | 'ruleId' | 'upstreamName' | 'createdAt'>): Promise<void> => {
    submitting.value = true
    try {
      const upstream = globalConfig.value.servers.find((item) => item.id === payload.upstreamId)
      const normalized = {
        ...payload,
        upstreamName: upstream ? `${upstream.name} / ${upstream.address}` : '',
      }
      if (payload.id) {
        const target = conditionRules.value.find((item) => item.id === payload.id)
        if (target) {
          Object.assign(target, normalized)
        }
      } else {
        conditionRules.value.unshift({
          ...normalized,
          id: Date.now(),
          ruleId: `FWD-RULE-${String(Date.now()).slice(-6)}`,
          createdAt: new Date().toLocaleString('zh-CN', { hour12: false }),
        })
      }
    } finally {
      submitting.value = false
    }
  }

  const batchUpdateRuleStatus = (ids: number[], status: string): void => {
    const set = new Set(ids.map(Number))
    conditionRules.value.forEach((item) => {
      if (set.has(item.id)) {
        item.status = status
      }
    })
  }

  const deleteRules = (ids: number[]): void => {
    const set = new Set(ids.map(Number))
    conditionRules.value = conditionRules.value.filter((item) => !set.has(item.id))
  }

  return {
    loading,
    submitting,
    globalConfig,
    conditionRules,
    lastUpdated,
    upstreamOptions,
    fetchForwardData,
    startPolling,
    stopPolling,
    saveGlobalConfig,
    resetGlobalConfig,
    saveServer,
    deleteServer,
    saveConditionRule,
    batchUpdateRuleStatus,
    deleteRules,
  }
})