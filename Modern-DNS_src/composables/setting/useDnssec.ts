import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useSecurityStore } from '../../stores/security'
import type { SecurityDnssecRow } from '../../types/modules'
import { confirmRiskAction } from '../../utils/interaction'

type SortOrder = 'ascending' | 'descending' | null
type SortChangePayload = { prop: string | null; order: SortOrder }
type DnssecKeyType = 'ksk' | 'zsk'
type DnssecKey = SecurityDnssecRow['ksk'][number]

const RAW_DNSSEC_OPENED = '已开启'
const RAW_DNSSEC_UNOPENED = '未开启'
const RAW_SIG_VALID = '有效'
const RAW_SIG_ABNORMAL = '异常'
const RAW_SIG_UNCHECKED = '未检查'
const RAW_SIG_UNTESTED = '未检测'

const formatDateTime = (value: string): string => {
  if (!value) {
    return '--'
  }

  const normalized = String(value).replace('T', ' ').replace(/\//g, '-').trim()
  const date = new Date(normalized)
  if (Number.isNaN(date.getTime())) {
    return normalized.length >= 19 ? normalized.slice(0, 19) : normalized
  }

  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  const second = String(date.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`
}

const formatCompactNumber = (value: number): string => {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return String(value)
}

const copyText = async (text: string): Promise<void> => {
  if (!text.trim()) {
    throw new Error('EMPTY_TEXT')
    return
  }
  await navigator.clipboard.writeText(text)
}

export const useDnssec = () => {
  const { t } = useI18n()
  const securityStore = useSecurityStore()

  const filters = reactive({
    keyword: '',
    dnssecStatus: '',
    signatureStatus: '',
  })
  const appliedFilters = reactive({
    keyword: '',
    dnssecStatus: '',
    signatureStatus: '',
  })
  const pager = reactive({ page: 1, size: 10 })
  const sorter = reactive<{ prop: string; order: SortOrder }>({ prop: 'lastCheckAt', order: 'descending' })

  const detailVisible = ref(false)
  const detailRowId = ref<number | null>(null)
  const rowActionId = ref<number | null>(null)
  const bulkChecking = ref(false)

  let filterTimer: number | null = null

  const loading = computed(() => securityStore.loading)
  const submitting = computed(() => securityStore.submitting)
  const checking = computed(() => securityStore.checking)
  const rows = computed(() => securityStore.dnssecRows)
  const detail = computed(() => rows.value.find((item) => item.id === detailRowId.value) || null)

  const filteredRows = computed(() =>
    rows.value.filter((item) => {
      const keyword = appliedFilters.keyword.trim().toLowerCase()
      const matchKeyword = !keyword || item.domain.toLowerCase().includes(keyword)
      const matchDnssecStatus = !appliedFilters.dnssecStatus || item.dnssecStatus === appliedFilters.dnssecStatus
      const matchSignatureStatus = !appliedFilters.signatureStatus
        || item.signatureStatus === appliedFilters.signatureStatus
        || (appliedFilters.signatureStatus === RAW_SIG_UNCHECKED && item.signatureStatus === RAW_SIG_UNTESTED)
      return matchKeyword && matchDnssecStatus && matchSignatureStatus
    }),
  )

  const sortedRows = computed(() => {
    const list = [...filteredRows.value]
    if (!sorter.prop || !sorter.order) {
      return list
    }

    const factor = sorter.order === 'ascending' ? 1 : -1
    return list.sort((left, right) => {
      const leftValue = left[sorter.prop as keyof SecurityDnssecRow]
      const rightValue = right[sorter.prop as keyof SecurityDnssecRow]
      return String(leftValue ?? '').localeCompare(String(rightValue ?? '')) * factor
    })
  })

  const pagedRows = computed(() => {
    const start = (pager.page - 1) * pager.size
    return sortedRows.value.slice(start, start + pager.size)
  })

  const statsCards = computed(() => [
    { label: t('security.dnssecOpenedDomains'), key: 'opened', value: formatCompactNumber(securityStore.dnssecStats.opened) },
    { label: t('security.dnssecValidDomains'), key: 'valid', value: formatCompactNumber(securityStore.dnssecStats.valid) },
    { label: t('security.dnssecAbnormalDomains'), key: 'abnormal', value: formatCompactNumber(securityStore.dnssecStats.abnormal) },
    { label: t('security.dnssecUnopenedDomains'), key: 'unopened', value: formatCompactNumber(securityStore.dnssecStats.unopened) },
  ])

  const keySummary = (row: SecurityDnssecRow): string => `KSK ${row.ksk.length} / ZSK ${row.zsk.length}`
  const totalKeys = (row: SecurityDnssecRow): number => row.ksk.length + row.zsk.length
  const rowClassName = ({ row }: { row: SecurityDnssecRow }): string => (row.signatureStatus === RAW_SIG_ABNORMAL ? 'dnssec-row-abnormal' : '')

  const dnssecStatusLabel = (value: string): string => {
    if (value === RAW_DNSSEC_OPENED) return t('security.opened')
    if (value === RAW_DNSSEC_UNOPENED) return t('security.notOpened')
    return value
  }

  const signatureStatusLabel = (value: string): string => {
    if (value === RAW_SIG_VALID) return t('security.valid')
    if (value === RAW_SIG_ABNORMAL) return t('security.invalid')
    if (value === RAW_SIG_UNCHECKED) return t('security.notChecked')
    if (value === RAW_SIG_UNTESTED) return t('security.notChecked')
    return value
  }

  const applyFilters = () => {
    appliedFilters.keyword = filters.keyword.trim()
    appliedFilters.dnssecStatus = filters.dnssecStatus
    appliedFilters.signatureStatus = filters.signatureStatus
    pager.page = 1
  }

  const resetFilters = () => {
    filters.keyword = ''
    filters.dnssecStatus = ''
    filters.signatureStatus = ''
    applyFilters()
  }

  const refreshData = async (silent = false) => {
    try {
      await securityStore.fetchSecurityData()
      if (!silent) {
        ElMessage.success(t('security.refreshSuccess'))
      }
    } catch (_error) {
      if (!silent) {
        ElMessage.error(t('security.refreshFailed'))
      }
    }
  }

  const handleSortChange = ({ prop, order }: SortChangePayload) => {
    sorter.prop = prop || 'lastCheckAt'
    sorter.order = order
  }

  const openDetail = (row: SecurityDnssecRow) => {
    detailRowId.value = row.id
    detailVisible.value = true
  }

  const closeDetail = () => {
    detailVisible.value = false
    detailRowId.value = null
  }

  const runCheckAll = async () => {
    if (bulkChecking.value || rowActionId.value !== null) {
      return
    }
    bulkChecking.value = true
    try {
      await securityStore.checkAllDnssec()
      ElMessage.success(t('security.checkAllDone'))
    } catch (_error) {
      ElMessage.error(t('security.checkFailed'))
    } finally {
      bulkChecking.value = false
    }
  }

  const runCheckOne = async (row: SecurityDnssecRow) => {
    if (rowActionId.value !== null || bulkChecking.value) {
      return
    }
    rowActionId.value = row.id
    try {
      await securityStore.checkDnssecById(row.id)
      ElMessage.success(t('security.recheckDone', { domain: row.domain }))
    } catch (_error) {
      ElMessage.error(t('security.recheckFailed'))
    } finally {
      rowActionId.value = null
    }
  }

  const toggleEnabled = async (row: SecurityDnssecRow, enabled: boolean) => {
    if (rowActionId.value !== null) {
      return
    }
    rowActionId.value = row.id
    try {
      await securityStore.toggleDnssec(row.id, enabled)
      ElMessage.success(t('security.dnssecToggled', {
        domain: row.domain,
        status: enabled ? t('security.opened') : t('security.notOpened'),
      }))
    } catch (_error) {
      ElMessage.error(t('security.toggleFailed'))
    } finally {
      rowActionId.value = null
    }
  }

  const generateKey = async (row: SecurityDnssecRow, keyType: DnssecKeyType) => {
    if (rowActionId.value !== null) {
      return
    }
    rowActionId.value = row.id
    try {
      await securityStore.generateDnssecKey(row.id, keyType)
      ElMessage.success(t('security.keyGenerated', { domain: row.domain, type: keyType.toUpperCase() }))
    } catch (_error) {
      ElMessage.error(t('security.keyGenerateFailed'))
    } finally {
      rowActionId.value = null
    }
  }

  const deleteKey = async (row: SecurityDnssecRow, keyType: DnssecKeyType, key: DnssecKey) => {
    await confirmRiskAction({
      action: t('security.deleteKeyAction', { type: keyType.toUpperCase() }),
      target: `${row.domain} / ${key.keyId}`,
      risk: t('security.deleteKeyRisk'),
    })

    rowActionId.value = row.id
    try {
      await securityStore.deleteDnssecKey(row.id, keyType, key.keyId)
      ElMessage.success(t('security.keyDeleted', { keyId: key.keyId }))
    } catch (_error) {
      ElMessage.error(t('security.keyDeleteFailed'))
    } finally {
      rowActionId.value = null
    }
  }

  const copyKeys = async (row: SecurityDnssecRow) => {
    if (!row.ksk.length && !row.zsk.length) {
      ElMessage.warning(t('security.noCopyContent'))
      return
    }

    const keyText = [
      `${t('cache.domain')}: ${row.domain}`,
      ...row.ksk.map((item) => `KSK ${item.keyId} ${item.status} ${formatDateTime(item.createdAt)}`),
      ...row.zsk.map((item) => `ZSK ${item.keyId} ${item.status} ${formatDateTime(item.createdAt)}`),
    ].join('\n')

    try {
      await copyText(keyText)
      ElMessage.success(t('security.keyCopied'))
    } catch (_error) {
      ElMessage.error(t('security.copyFailed'))
    }
  }

  watch(
    () => [filters.keyword, filters.dnssecStatus, filters.signatureStatus],
    () => {
      if (filterTimer !== null) {
        window.clearTimeout(filterTimer)
      }
      filterTimer = window.setTimeout(() => {
        applyFilters()
      }, 240)
    },
  )

  watch(
    () => filteredRows.value.length,
    (length) => {
      const pageCount = Math.max(1, Math.ceil(length / pager.size))
      if (pager.page > pageCount) {
        pager.page = pageCount
      }
    },
  )

  watch(
    () => pager.size,
    () => {
      pager.page = 1
    },
  )

  onMounted(() => {
    if (!rows.value.length) {
      void refreshData(true)
    }
    applyFilters()
  })

  return {
    loading,
    submitting,
    checking,
    filters,
    pager,
    sorter,
    detailVisible,
    detail,
    rowActionId,
    bulkChecking,
    rows,
    sortedRows,
    pagedRows,
    statsCards,
    formatDateTime,
    keySummary,
    totalKeys,
    rowClassName,
    dnssecStatusLabel,
    signatureStatusLabel,
    applyFilters,
    resetFilters,
    refreshData,
    handleSortChange,
    openDetail,
    closeDetail,
    runCheckAll,
    runCheckOne,
    toggleEnabled,
    generateKey,
    deleteKey,
    copyKeys,
  }
}