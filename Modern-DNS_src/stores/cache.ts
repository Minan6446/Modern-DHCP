import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  clearCacheRecords,
  deleteDomainCacheRule,
  getCacheModuleData,
  saveDomainCacheRule,
  saveGlobalCacheStrategy,
} from '../api/cache'
import type { CacheDomainRule, CacheGlobalStrategy, CacheModuleData, CacheNode } from '../types/modules'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

const flattenTree = (rows: CacheNode[] = []): CacheNode[] => {
  const result: CacheNode[] = []
  const walk = (items: CacheNode[]): void => {
    items.forEach((item) => {
      result.push(item)
      if (item.children?.length) {
        walk(item.children)
      }
    })
  }
  walk(rows)
  return result
}

const removeNodesByIds = (rows: CacheNode[], idsSet: Set<number>) =>
  rows
    .filter((item) => !idsSet.has(item.id))
    .map((item) => ({
      ...item,
      children: item.children?.length ? removeNodesByIds(item.children, idsSet) : undefined,
    }))

export const useCacheStore = defineStore('cache', () => {
  const loading = ref(false)
  const submitting = ref(false)
  const domainCacheTree = ref<CacheNode[]>([])
  const globalStrategy = ref<CacheGlobalStrategy>({
    ttlMax: 86400,
    minRetain: 300,
    autoCleanup: true,
    cleanupCycle: 'hourly',
  })
  const domainStrategies = ref<CacheDomainRule[]>([])
  const lastUpdated = ref('')

  const fetchCacheData = async () => {
    loading.value = true
    try {
      const { data } = await getCacheModuleData() as { data: CacheModuleData }
      domainCacheTree.value = clone(data.domainCacheTree || [])
      globalStrategy.value = clone(data.strategy?.global || globalStrategy.value)
      domainStrategies.value = clone(data.strategy?.domainRules || [])
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  const flatDomainCache = computed(() => flattenTree(domainCacheTree.value))

  const clearByIds = async (ids: number[]): Promise<number> => {
    submitting.value = true
    try {
      const normalized = ids.map(Number)
      await clearCacheRecords({ ids: normalized })
      const idsSet = new Set(normalized)
      domainCacheTree.value = removeNodesByIds(domainCacheTree.value, idsSet)
      return normalized.length
    } finally {
      submitting.value = false
    }
  }

  const saveGlobalStrategy = async (payload: CacheGlobalStrategy): Promise<void> => {
    submitting.value = true
    try {
      await saveGlobalCacheStrategy(payload)
      globalStrategy.value = clone(payload)
    } finally {
      submitting.value = false
    }
  }

  const resetGlobalStrategy = async () => {
    await fetchCacheData()
  }

  const saveDomainRule = async (payload: Partial<CacheDomainRule> & Pick<CacheDomainRule, 'domain' | 'customTtl' | 'customRetain' | 'status'>): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await saveDomainCacheRule(payload) as { data: any }
      if (payload.id) {
        const target = domainStrategies.value.find((item) => item.id === payload.id)
        if (target) {
          Object.assign(target, data)
        }
      } else {
        domainStrategies.value.unshift(data as CacheDomainRule)
      }
    } finally {
      submitting.value = false
    }
  }

  const deleteDomainRule = async (id: number): Promise<void> => {
    submitting.value = true
    try {
      await deleteDomainCacheRule(id)
      domainStrategies.value = domainStrategies.value.filter((item) => item.id !== id)
    } finally {
      submitting.value = false
    }
  }

  const clearByScope = async (payload: { scope: string; domains?: string; timeRange?: string[] }): Promise<number> => {
    submitting.value = true
    try {
      const allRows = flatDomainCache.value
      let targetIds = []

      if (payload.scope === 'all') {
        targetIds = allRows.map((item) => item.id)
      }

      if (payload.scope === 'expired') {
        targetIds = allRows.filter((item) => Number(item.ttlRemaining) <= 0).map((item) => item.id)
      }

      if (payload.scope === 'domain') {
        const domains = String(payload.domains || '')
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean)
        targetIds = allRows
          .filter((item) => domains.some((domain) => item.domain.includes(domain)))
          .map((item) => item.id)
      }

      const timeRange = payload.timeRange || []
      if (timeRange.length === 2) {
        const start = new Date(timeRange[0]).getTime()
        const end = new Date(timeRange[1]).getTime()
        const set = new Set(targetIds)
        targetIds = allRows
          .filter((item) => {
            if (!set.has(item.id)) {
              return false
            }
            const ts = new Date(item.cacheTime).getTime()
            return ts >= start && ts <= end
          })
          .map((item) => item.id)
      }

      if (!targetIds.length) {
        return 0
      }

      await clearCacheRecords({ ids: targetIds })
      const idsSet = new Set(targetIds)
      domainCacheTree.value = removeNodesByIds(domainCacheTree.value, idsSet)
      return targetIds.length
    } finally {
      submitting.value = false
    }
  }

  return {
    loading,
    submitting,
    domainCacheTree,
    flatDomainCache,
    globalStrategy,
    domainStrategies,
    lastUpdated,
    fetchCacheData,
    clearByIds,
    clearByScope,
    saveGlobalStrategy,
    resetGlobalStrategy,
    saveDomainRule,
    deleteDomainRule,
  }
})
