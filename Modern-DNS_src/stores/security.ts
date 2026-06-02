import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  batchDeleteBlackWhiteRulesApi,
  batchUpdateBlackWhiteStatusApi,
  checkDnssecAllApi,
  checkDnssecByIdApi,
  deleteDnssecKeyApi,
  deleteBlackWhiteRuleApi,
  deleteDdosDomainRuleApi,
  generateDnssecKeyApi,
  getSecurityModuleData,
  importBlackWhiteRulesApi,
  saveBlackWhiteRuleApi,
  saveDdosDomainRuleApi,
  saveDdosGlobalConfigApi,
  toggleDnssecApi,
} from '../api/security'
import type {
  SecurityBlackWhiteRule,
  SecurityDdosDomainRule,
  SecurityDdosGlobal,
  SecurityDnssecRow,
  SecurityModuleData,
} from '../types/modules'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

export const useSecurityStore = defineStore('security', () => {
  const loading = ref(false)
  const submitting = ref(false)
  const checking = ref(false)

  const blackWhiteRules = ref<SecurityBlackWhiteRule[]>([])
  const ddosGlobal = ref<SecurityDdosGlobal>({
    enabled: true,
    qpsLimit: 10000,
    currentQps: 0,
    perIpConnLimit: 0,
    memSoftMb: 0,
    memHardMb: 0,
    perIpQps: 0,
    perIpBurst: 0,
  })
  const ddosDomainRules = ref<SecurityDdosDomainRule[]>([])
  const dnssecRows = ref<SecurityDnssecRow[]>([])
  const lastUpdated = ref('')

  const fetchSecurityData = async () => {
    loading.value = true
    try {
      const { data } = await getSecurityModuleData() as { data: SecurityModuleData }
      blackWhiteRules.value = clone(data.blackWhite?.rules || [])
      ddosGlobal.value = clone(data.ddos?.global || ddosGlobal.value)
      ddosDomainRules.value = clone(data.ddos?.domainRules || [])
      dnssecRows.value = clone(data.dnssec?.rows || [])
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loading.value = false
    }
  }

  const saveBlackWhiteRule = async (payload: Partial<SecurityBlackWhiteRule> & Pick<SecurityBlackWhiteRule, 'type' | 'listType' | 'value' | 'remark' | 'status'>): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await saveBlackWhiteRuleApi(payload) as { data: any }
      if (payload.id) {
        const target = blackWhiteRules.value.find((item) => item.id === payload.id)
        if (target) {
          Object.assign(target, data)
        }
      } else {
        blackWhiteRules.value.unshift(data as SecurityBlackWhiteRule)
      }
    } finally {
      submitting.value = false
    }
  }

  const deleteBlackWhiteRule = async (id: number): Promise<void> => {
    submitting.value = true
    try {
      await deleteBlackWhiteRuleApi(id)
      blackWhiteRules.value = blackWhiteRules.value.filter((item) => item.id !== id)
    } finally {
      submitting.value = false
    }
  }

  const batchUpdateBlackWhiteStatus = async (ids: number[], status: string): Promise<void> => {
    submitting.value = true
    try {
      await batchUpdateBlackWhiteStatusApi({ ids, status })
      const set = new Set(ids.map(Number))
      blackWhiteRules.value.forEach((item) => {
        if (set.has(item.id)) {
          item.status = status
        }
      })
    } finally {
      submitting.value = false
    }
  }

  const batchDeleteBlackWhiteRules = async (ids: number[]): Promise<void> => {
    submitting.value = true
    try {
      await batchDeleteBlackWhiteRulesApi(ids)
      const set = new Set(ids.map(Number))
      blackWhiteRules.value = blackWhiteRules.value.filter((item) => !set.has(item.id))
    } finally {
      submitting.value = false
    }
  }

  const importBlackWhiteRules = async (rows: Array<Record<string, unknown>>): Promise<void> => {
    submitting.value = true
    try {
      const rules = rows.map((row) => ({
        type: String(row.type || row['类型'] || 'IP'),
        listType: String(row.listType || row['名单类型'] || '黑名单'),
        value: String(row.value || row['值'] || ''),
        remark: String(row.remark || row['备注'] || ''),
        status: String(row.status || row['状态'] || '启用'),
      }))
      await importBlackWhiteRulesApi(rules)
      await fetchSecurityData()
    } finally {
      submitting.value = false
    }
  }

  const saveDdosGlobalConfig = async (payload: SecurityDdosGlobal): Promise<void> => {
    submitting.value = true
    try {
      await saveDdosGlobalConfigApi(payload)
      ddosGlobal.value = clone(payload)
    } finally {
      submitting.value = false
    }
  }

  const saveDdosDomainRule = async (payload: Partial<SecurityDdosDomainRule> & Pick<SecurityDdosDomainRule, 'domain' | 'qpsLimit' | 'status'>): Promise<void> => {
    submitting.value = true
    try {
      const { data } = await saveDdosDomainRuleApi(payload) as { data: any }
      if (payload.id) {
        const target = ddosDomainRules.value.find((item) => item.id === payload.id)
        if (target) {
          Object.assign(target, data)
        }
      } else {
        ddosDomainRules.value.unshift(data as SecurityDdosDomainRule)
      }
    } finally {
      submitting.value = false
    }
  }

  const deleteDdosDomainRule = async (id: number): Promise<void> => {
    submitting.value = true
    try {
      await deleteDdosDomainRuleApi(id)
      ddosDomainRules.value = ddosDomainRules.value.filter((item) => item.id !== id)
    } finally {
      submitting.value = false
    }
  }

  const checkAllDnssec = async () => {
    checking.value = true
    try {
      await checkDnssecAllApi()
      await fetchSecurityData()
    } finally {
      checking.value = false
    }
  }

  const checkDnssecById = async (id: number): Promise<void> => {
    checking.value = true
    try {
      await checkDnssecByIdApi(id)
      await fetchSecurityData()
    } finally {
      checking.value = false
    }
  }

  const toggleDnssec = async (id: number, enabled: boolean): Promise<void> => {
    submitting.value = true
    try {
      await toggleDnssecApi({ id, enabled })
      await fetchSecurityData()
    } finally {
      submitting.value = false
    }
  }

  const generateDnssecKey = async (id: number, keyType: 'ksk' | 'zsk'): Promise<void> => {
    submitting.value = true
    try {
      await generateDnssecKeyApi({ id, keyType })
      await fetchSecurityData()
    } finally {
      submitting.value = false
    }
  }

  const deleteDnssecKey = async (id: number, keyType: 'ksk' | 'zsk', keyId: string): Promise<void> => {
    submitting.value = true
    try {
      await deleteDnssecKeyApi({ id, keyType, keyId })
      await fetchSecurityData()
    } finally {
      submitting.value = false
    }
  }

  const dnssecStats = computed(() => {
    const opened = dnssecRows.value.filter((item) => item.dnssecStatus === '已开启').length
    const valid = dnssecRows.value.filter((item) => item.signatureStatus === '有效').length
    const abnormal = dnssecRows.value.filter((item) => item.signatureStatus === '异常').length
    const unopened = dnssecRows.value.filter((item) => item.dnssecStatus === '未开启').length
    return { opened, valid, abnormal, unopened }
  })

  return {
    loading,
    submitting,
    checking,
    blackWhiteRules,
    ddosGlobal,
    ddosDomainRules,
    dnssecRows,
    dnssecStats,
    lastUpdated,
    fetchSecurityData,
    saveBlackWhiteRule,
    deleteBlackWhiteRule,
    batchUpdateBlackWhiteStatus,
    batchDeleteBlackWhiteRules,
    importBlackWhiteRules,
    saveDdosGlobalConfig,
    saveDdosDomainRule,
    deleteDdosDomainRule,
    checkAllDnssec,
    checkDnssecById,
    toggleDnssec,
    generateDnssecKey,
    deleteDnssecKey,
  }
})
