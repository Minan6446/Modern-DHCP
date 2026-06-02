import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElLoading } from 'element-plus'
import type { FormInstance } from 'element-plus'
import { useDomainStore } from '../../stores/domain'
import type { DomainRecord } from '../../types/modules'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'

export type RecordType = 'A' | 'AAAA' | 'CNAME' | 'TXT' | 'MX' | 'NS' | 'SRV'
export type TableDensity = 'compact' | 'default' | 'relaxed'

export interface RecordFilter {
  keyword: string
  type: string
  status: string
}

export interface RecordFormModel {
  id: number | null
  type: RecordType
  host: string
  value: string
  ttl: number
  status: '启用' | '禁用'
  remark: string
}

const TTL_PRESET_VALUES = [60, 300, 600, 1800, 3600, 21600, 43200, 86400] as const

const RECORD_TYPES: RecordType[] = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'NS', 'SRV']

const EMPTY_FORM: RecordFormModel = {
  id: null,
  type: 'A',
  host: '',
  value: '',
  ttl: 600,
  status: '启用',
  remark: '',
}

const ipv4Pattern = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
const ipv6Pattern = /^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^(([0-9a-fA-F]{1,4}:){1,7}:|:((:[0-9a-fA-F]{1,4}){1,7}|:))$/
const domainPattern = /^(?=.{1,253}$)(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}\.?$/
const hostPattern = /^(@|\*|[a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*)$/

export function useZoneRecord(zoneId: () => string | number | string[]) {
  const domainStore = useDomainStore()
  const { t } = useI18n()

  /* ── dialog ── */
  const dialogVisible = ref(false)
  const formRef = ref<FormInstance>()
  const form = reactive<RecordFormModel>({ ...EMPTY_FORM })

  /* ── filter ── */
  const filter = reactive<RecordFilter>({ keyword: '', type: '', status: '' })

  /* ── density ── */
  const density = ref<TableDensity>('default')

  const TTL_PRESETS = computed(() => [
    { label: `1 ${t('record.minute')}`, value: TTL_PRESET_VALUES[0] },
    { label: `5 ${t('record.minute')}`, value: TTL_PRESET_VALUES[1] },
    { label: `10 ${t('record.minute')}`, value: TTL_PRESET_VALUES[2] },
    { label: `30 ${t('record.minute')}`, value: TTL_PRESET_VALUES[3] },
    { label: `1 ${t('record.hour')}`, value: TTL_PRESET_VALUES[4] },
    { label: `6 ${t('record.hour')}`, value: TTL_PRESET_VALUES[5] },
    { label: `12 ${t('record.hour')}`, value: TTL_PRESET_VALUES[6] },
    { label: `24 ${t('record.hour')}`, value: TTL_PRESET_VALUES[7] },
  ])

  /* ── busy flags ── */
  const deletingId = ref<number | null>(null)
  const batchDeleting = ref(false)
  const statusSwitchingId = ref<number | null>(null)
  const ttlAdjustingId = ref<number | null>(null)

  /* ── derived source rows ── */
  const allRecords = computed<DomainRecord[]>(() => domainStore.getDetailById(zoneId()).records)

  const filteredRecords = computed<DomainRecord[]>(() => {
    let rows = allRecords.value
    const kw = filter.keyword.trim().toLowerCase()
    if (kw) {
      rows = rows.filter(
        (r) =>
          r.host.toLowerCase().includes(kw) ||
          r.value.toLowerCase().includes(kw) ||
          r.type.toLowerCase().includes(kw),
      )
    }
    if (filter.type) {
      rows = rows.filter((r) => r.type === filter.type)
    }
    if (filter.status) {
      rows = rows.filter((r) => r.status === filter.status)
    }
    return rows
  })

  const recordValueHint = computed(() => {
    if (form.type === 'A') return t('record.valueHintA')
    if (form.type === 'AAAA') return t('record.valueHintAAAA')
    if (['CNAME', 'NS', 'MX', 'SRV'].includes(form.type)) return t('record.valueHintDomain', { type: form.type })
    return ''
  })

  /* ── form rules ── */
  const rules = {
    type: [{ required: true, message: t('record.valType'), trigger: 'change' }],
    host: [{
      validator: (_: unknown, value: string, cb: (e?: Error) => void) => {
        const v = String(value || '').trim()
        if (!v) return cb(new Error(t('record.valHostRequired')))
        if (!hostPattern.test(v)) return cb(new Error(t('record.valHostInvalid')))
        cb()
      },
      trigger: ['blur', 'change'],
    }],
    value: [{
      validator: (_: unknown, value: string, cb: (e?: Error) => void) => {
        const v = String(value || '').trim()
        if (!v) return cb(new Error(t('record.valValueRequired')))
        if (form.type === 'A' && !ipv4Pattern.test(v)) return cb(new Error(t('record.valValueA')))
        if (form.type === 'AAAA' && !ipv6Pattern.test(v)) return cb(new Error(t('record.valValueAAAA')))
        if (['CNAME', 'NS', 'MX', 'SRV'].includes(form.type) && !domainPattern.test(v)) return cb(new Error(t('record.valValueDomain')))
        cb()
      },
      trigger: ['blur', 'change'],
    }],
    ttl: [
      { required: true, message: t('record.valTtlRequired'), trigger: 'blur' },
      {
        validator: (_: unknown, value: number, cb: (e?: Error) => void) => {
          const ttl = Number(value)
          if (!Number.isFinite(ttl) || ttl < 1 || ttl > 86400) return cb(new Error(t('record.valTtlRange')))
          cb()
        },
        trigger: ['blur', 'change'],
      },
    ],
  }

  /* ── actions ── */
  const resetForm = () => Object.assign(form, { ...EMPTY_FORM })

  const openDialog = (row?: DomainRecord) => {
    Object.assign(form, row ? { ...row } : { ...EMPTY_FORM })
    dialogVisible.value = true
  }

  const closeDialog = () => {
    dialogVisible.value = false
    resetForm()
    formRef.value?.clearValidate()
  }

  const submitRecord = async () => {
    if (domainStore.submitting) return
    const valid = await validateFormAndFocus(formRef.value)
    if (!valid) return
    await domainStore.saveRecord(zoneId(), form)
    dialogVisible.value = false
    resetForm()
    ElMessage.success(t('record.recordSaved'))
  }

  const clearForm = async () => {
    await confirmRiskAction({ title: t('record.clearFormConfirmTitle'), action: t('record.clearFormConfirmAction'), risk: t('record.clearFormConfirmRisk') })
    resetForm()
    formRef.value?.clearValidate()
  }

  const deleteRecord = async (row: DomainRecord) => {
    if (deletingId.value || batchDeleting.value) return
    deletingId.value = row.id
    try {
      await confirmRiskAction({ title: t('record.deleteConfirmTitle'), action: t('record.deleteConfirmAction'), target: `${row.host} ${row.type}`, risk: t('record.deleteConfirmRisk') })
      domainStore.deleteRecords(zoneId(), [row.id])
      ElMessage.success(t('record.recordDeleted'))
    } finally {
      deletingId.value = null
    }
  }

  const batchDelete = async (selectedRows: DomainRecord[]) => {
    if (batchDeleting.value) return
    if (!selectedRows.length) { ElMessage.warning(t('record.selectRecordFirst')); return }
    batchDeleting.value = true
    try {
      await confirmRiskAction({ title: t('record.batchDeleteConfirmTitle'), action: t('record.batchDeleteConfirmAction'), risk: t('record.batchDeleteConfirmRisk') })
      domainStore.deleteRecords(zoneId(), selectedRows.map((r) => r.id))
      ElMessage.success(t('record.batchDeleted'))
    } finally {
      batchDeleting.value = false
    }
  }

  const adjustTtl = async (row: DomainRecord, delta: number) => {
    if (ttlAdjustingId.value) return
    ttlAdjustingId.value = row.id
    try {
      await domainStore.saveRecord(zoneId(), { ...row, ttl: Math.max(60, Number(row.ttl) + delta) })
    } finally {
      ttlAdjustingId.value = null
    }
  }

  const setTtlPreset = async (row: DomainRecord, preset: number) => {
    if (ttlAdjustingId.value) return
    ttlAdjustingId.value = row.id
    try {
      await domainStore.saveRecord(zoneId(), { ...row, ttl: preset })
    } finally {
      ttlAdjustingId.value = null
    }
  }

  const toggleStatus = async (row: DomainRecord, value: boolean) => {
    if (statusSwitchingId.value) return
    statusSwitchingId.value = row.id
    try {
      domainStore.toggleRecordStatus(zoneId(), row.id, value ? '启用' : '禁用')
      ElMessage.success(t('record.statusToggled', { status: value ? t('record.enabled') : t('record.disabled') }))
    } finally {
      statusSwitchingId.value = null
    }
  }

  /* ── import / export ── */
  const importing = ref(false)
  const importProgress = ref(0)

  const exportRecordsCsv = (rows: DomainRecord[]) => {
    const loading = ElLoading.service({ text: t('record.exportingCsv'), background: 'rgba(0,0,0,0.35)' })
    try {
      const header = 'type,host,value,ttl,status,remark'
      const body = rows
        .map((r) => [r.type, r.host, `"${r.value}"`, r.ttl, r.status, `"${r.remark || ''}"`].join(','))
        .join('\n')
      const blob = new Blob([`${header}\n${body}`], { type: 'text/csv;charset=utf-8;' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `zone-records-${String(zoneId())}.csv`
      a.click()
      URL.revokeObjectURL(url)
      ElMessage.success(t('record.exportedCsv'))
    } finally {
      loading.close()
    }
  }

  const importRecordsCsv = async (file: File): Promise<void> => {
    const text = await file.text()
    const lines = text.trim().split('\n')
    if (lines.length < 2) { ElMessage.error(t('record.csvEmpty')); return }
    const header = lines[0].split(',').map((h) => h.trim())
    const typeIdx = header.indexOf('type')
    const hostIdx = header.indexOf('host')
    const valueIdx = header.indexOf('value')
    const ttlIdx = header.indexOf('ttl')
    const statusIdx = header.indexOf('status')
    const remarkIdx = header.indexOf('remark')
    if (typeIdx < 0 || hostIdx < 0 || valueIdx < 0 || ttlIdx < 0) {
      ElMessage.error(t('record.csvMissingColumns'))
      return
    }
    const dataLines = lines.slice(1).filter((l) => l.trim())
    const total = dataLines.length
    importing.value = true
    importProgress.value = 0
    let imported = 0
    for (let i = 0; i < dataLines.length; i++) {
      const cols = dataLines[i].split(',').map((c) => c.trim().replace(/^"|"$/g, ''))
      const type = cols[typeIdx] as RecordType
      const host = cols[hostIdx]
      const value = cols[valueIdx]
      const ttl = Number(cols[ttlIdx]) || 600
      const rawStatus = cols[statusIdx]
      const status = (rawStatus === '禁用' || rawStatus === t('record.disabled') ? '禁用' : '启用') as '启用' | '禁用'
      const remark = remarkIdx >= 0 ? (cols[remarkIdx] || '') : ''
      if (!type || !host || !value) { importProgress.value = Math.round(((i + 1) / total) * 100); continue }
      await domainStore.saveRecord(zoneId(), { type, host, value, ttl, status, remark })
      imported++
      importProgress.value = Math.round(((i + 1) / total) * 100)
    }
    importing.value = false
    ElMessage.success(t('record.importedCount', { count: imported }))
  }

  const resetFilter = () => Object.assign(filter, { keyword: '', type: '', status: '' })

  return {
    /* state */
    dialogVisible,
    formRef,
    form,
    filter,
    density,
    /* busy */
    deletingId,
    batchDeleting,
    statusSwitchingId,
    ttlAdjustingId,
    importing,
    importProgress,
    /* derived */
    allRecords,
    filteredRecords,
    recordValueHint,
    rules,
    /* constants */
    TTL_PRESETS,
    RECORD_TYPES,
    /* actions */
    openDialog,
    closeDialog,
    submitRecord,
    clearForm,
    deleteRecord,
    batchDelete,
    adjustTtl,
    setTtlPreset,
    toggleStatus,
    resetFilter,
    exportRecordsCsv,
    importRecordsCsv,
  }
}
