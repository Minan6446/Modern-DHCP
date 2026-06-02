import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElLoading } from 'element-plus'
import * as XLSX from 'xlsx'
import { useI18n } from 'vue-i18n'
import { getLogsApi } from '../../api/setting'
import type { SettingLogItem } from '../../types/modules'

type SortOrder = 'ascending' | 'descending' | null

export interface OperationLogFilterForm {
  keyword: string
  type: string
  range: string[]
}

export interface OperationLogPagination {
  page: number
  size: number
  total: number
}

const formatDateTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (num: number) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export const useOperationLog = () => {
  const { t } = useI18n()
  const filterForm = reactive<OperationLogFilterForm>({
    keyword: '',
    type: '',
    range: [],
  })
  const pagination = reactive<OperationLogPagination>({
    page: 1,
    size: 10,
    total: 0,
  })
  const loading = ref(false)
  const exporting = ref(false)
  const exportFormat = ref<'CSV' | 'Excel'>('CSV')
  const detailVisible = ref(false)
  const currentDetail = ref<SettingLogItem | null>(null)
  const rawLogs = ref<SettingLogItem[]>([])
  const sortState = reactive<{ prop: keyof SettingLogItem | ''; order: SortOrder }>({
    prop: 'time',
    order: 'descending',
  })

  const filteredLogs = computed<SettingLogItem[]>(() => {
    const keyword = filterForm.keyword.trim().toLowerCase()

    return rawLogs.value.filter((item) => {
      const hitKeyword =
        !keyword ||
        [item.operator, item.content, item.ip]
          .some((field) => String(field || '').toLowerCase().includes(keyword))
      const hitType = !filterForm.type || item.actionType === filterForm.type
      const hitRange =
        !filterForm.range.length ||
        (new Date(item.time).getTime() >= new Date(filterForm.range[0]).getTime() &&
          new Date(item.time).getTime() <= new Date(filterForm.range[1]).getTime())

      return hitKeyword && hitType && hitRange
    })
  })

  const sortedLogs = computed<SettingLogItem[]>(() => {
    const rows = [...filteredLogs.value]
    if (!sortState.prop) {
      return rows
    }

    const factor = sortState.order === 'ascending' ? 1 : -1
    return rows.sort((left, right) =>
      String(left[sortState.prop] || '').localeCompare(String(right[sortState.prop] || '')) * factor,
    )
  })

  const logList = computed<SettingLogItem[]>(() => {
    const start = (pagination.page - 1) * pagination.size
    return sortedLogs.value.slice(start, start + pagination.size)
  })

  watch(
    sortedLogs,
    (rows) => {
      pagination.total = rows.length
      const totalPages = Math.max(1, Math.ceil(rows.length / pagination.size))
      if (pagination.page > totalPages) {
        pagination.page = totalPages
      }
    },
    { immediate: true },
  )

  const fetchLogs = async (): Promise<void> => {
    loading.value = true
    try {
      const { data } = await getLogsApi()
      rawLogs.value = data || []
    } catch (_error) {
      ElMessage.error(t('audit.loadFailed'))
    } finally {
      loading.value = false
    }
  }

  const handleSearch = (): void => {
    pagination.page = 1
  }

  const handleReset = (): void => {
    filterForm.keyword = ''
    filterForm.type = ''
    filterForm.range = []
    pagination.page = 1
    sortState.prop = 'time'
    sortState.order = 'descending'
  }

  const handlePageChange = (page: number, size = pagination.size): void => {
    pagination.page = page
    pagination.size = size
  }

  const handleSortChange = (payload: { prop: string | null; order: SortOrder }): void => {
    sortState.prop = (payload.prop || 'time') as keyof SettingLogItem
    sortState.order = payload.order || 'descending'
    pagination.page = 1
  }

  const handleViewDetail = (row: SettingLogItem): void => {
    currentDetail.value = row
    detailVisible.value = true
  }

  const handleExport = async (): Promise<void> => {
    if (exporting.value) {
      return
    }
    const loadingInstance = ElLoading.service({ text: t('audit.exporting'), background: 'rgba(0,0,0,0.35)' })
    try {
      exporting.value = true
      const rows = sortedLogs.value.map((item) => ({
        [t('audit.logId')]: item.logId,
        [t('audit.operator')]: item.operator,
        [t('audit.action')]: item.actionType,
        [t('audit.module')]: item.module,
        [t('audit.detail')]: item.content,
        [t('audit.ipAddress')]: item.ip,
        [t('audit.time')]: formatDateTime(item.time),
      }))

      // Note: the audit-page server-side export endpoint
      // (/setting/logs/export) now returns a binary file rather than a
      // metadata ack, so we can't call it here as a "log this happened"
      // ping like we used to. The export is audited via the local
      // appendLog call inside the legacy SettingsStore consumer
      // (`stores/setting.ts -> exportLogs`) and the client-generated
      // file is what the operator sees. No round-trip needed.

      if (exportFormat.value === 'Excel') {
        const worksheet = XLSX.utils.json_to_sheet(rows)
        const workbook = XLSX.utils.book_new()
        XLSX.utils.book_append_sheet(workbook, worksheet, 'logs')
        XLSX.writeFile(workbook, `operation-logs-${Date.now()}.xlsx`)
      } else {
        const headers = [
          t('audit.logId'),
          t('audit.operator'),
          t('audit.action'),
          t('audit.module'),
          t('audit.detail'),
          t('audit.ipAddress'),
          t('audit.time'),
        ]
        const csvRows = [
          headers.join(','),
          ...rows.map((row) =>
            headers
              .map((key) => `"${String((row as Record<string, string>)[key] || '').replace(/"/g, '""')}"`)
              .join(','),
          ),
        ]
        const blob = new Blob([`\uFEFF${csvRows.join('\n')}`], { type: 'text/csv;charset=utf-8;' })
        const href = URL.createObjectURL(blob)
        const anchor = document.createElement('a')
        anchor.href = href
        anchor.download = `operation-logs-${Date.now()}.csv`
        document.body.appendChild(anchor)
        anchor.click()
        document.body.removeChild(anchor)
        URL.revokeObjectURL(href)
      }

      ElMessage.success(t('audit.exportSuccess'))
    } catch (_error) {
      ElMessage.error(t('audit.exportFailed'))
    } finally {
      exporting.value = false
      loadingInstance.close()
    }
  }

  return {
    filterForm,
    logList,
    pagination,
    loading,
    exporting,
    exportFormat,
    detailVisible,
    currentDetail,
    fetchLogs,
    handleSearch,
    handleReset,
    handlePageChange,
    handleSortChange,
    handleExport,
    handleViewDetail,
  }
}

export default useOperationLog