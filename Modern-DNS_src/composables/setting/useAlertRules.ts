import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { addAlertRule, deleteAlertRule, editAlertRule, getAlertRuleList, testAlertRuleApi } from '../../api/dashboard'
import { useCrud } from '../core/useCrud'
import { useDashboardStore } from '../../stores/dashboard'
import type { DashboardAlertRule } from '../../types/modules'

export type AlertRuleFormModel = Partial<DashboardAlertRule>

const createAlertRuleFormData = (): AlertRuleFormModel => ({
  id: undefined,
  ruleId: '',
  ruleName: '',
  alertType: '解析失败',
  level: '警告',
  channel: '邮件',
  target: '',
  threshold: 1,
  status: '启用',
  remark: '',
  createdAt: '',
})

export const useAlertRules = () => {
  const { t } = useI18n()
  const dashboardStore = useDashboardStore()
  const searchForm = ref({
    keyword: '',
    alertType: '',
    status: '',
  })
  const deletingRuleId = ref<number | null>(null)
  const testingRuleId = ref<number | null>(null)

  const crud = useCrud<DashboardAlertRule, typeof searchForm.value, AlertRuleFormModel, AlertRuleFormModel, number>(
    {
      getList: (params) => getAlertRuleList(params),
      create: (payload) => addAlertRule(payload),
      update: (payload) => editAlertRule(payload),
      delete: (id) => deleteAlertRule(id),
    },
    searchForm,
    {
      createFormData: createAlertRuleFormData,
      getItemId: (item) => item.id as number | undefined,
      pageField: 'page',
      pageSizeField: 'size',
      watchQueryParams: false,
    },
  )

  const {
    loading,
    tableData,
    pagination,
    dialogVisible,
    formData,
    fetchData,
    handleCreate,
    handleEdit,
    handleDelete,
    handleSubmit,
  } = crud

  const auditLogs = computed(() => dashboardStore.rawData.alerts?.auditLogs || [])
  const stats = computed(() => {
    const rows = tableData.value
    return [
      { key: 'total', label: t('monitor.alertRuleStatTotal'), value: pagination.total },
      { key: 'enabled', label: t('monitor.alertRuleStatEnabled'), value: rows.filter((item) => item.status === '启用').length },
      { key: 'disabled', label: t('monitor.alertRuleStatDisabled'), value: rows.filter((item) => item.status === '禁用').length },
      { key: 'mail', label: t('monitor.alertRuleStatEmailChannel'), value: rows.filter((item) => item.channel === '邮件').length },
    ]
  })

  const refreshAuditLogs = async (): Promise<void> => {
    await dashboardStore.fetchData()
  }

  const openCreateDialog = (): void => {
    handleCreate()
    formData.value = createAlertRuleFormData()
  }

  const openEditDialog = (row: DashboardAlertRule): void => {
    handleEdit(row)
  }

  const submitRule = async (): Promise<void> => {
    const isEdit = Boolean(formData.value.id)
    await handleSubmit()
    await refreshAuditLogs()
    ElMessage.success(isEdit ? t('monitor.alertRuleEditSuccess') : t('monitor.alertRuleCreateSuccess'))
  }

  const removeRule = async (row: DashboardAlertRule): Promise<void> => {
    deletingRuleId.value = row.id
    try {
      await handleDelete(row.id)
      await refreshAuditLogs()
      ElMessage.success(t('monitor.alertRuleDeleteSuccess'))
    } finally {
      deletingRuleId.value = null
    }
  }

  const testRule = async (row: DashboardAlertRule): Promise<void> => {
    if (testingRuleId.value) {
      return
    }
    testingRuleId.value = row.id
    try {
      await testAlertRuleApi({ id: row.id })
      await refreshAuditLogs()
      ElMessage.success(t('monitor.alertRuleTestSuccess', { name: row.ruleName }))
    } catch (_error) {
      ElMessage.error(t('monitor.alertRuleTestFailed'))
    } finally {
      testingRuleId.value = null
    }
  }

  onMounted(() => {
    refreshAuditLogs()
  })

  return {
    searchForm,
    loading,
    tableData,
    pagination,
    dialogVisible,
    formData,
    deletingRuleId,
    testingRuleId,
    auditLogs,
    stats,
    fetchData,
    openCreateDialog,
    openEditDialog,
    submitRule,
    removeRule,
    testRule,
  }
}

export default useAlertRules