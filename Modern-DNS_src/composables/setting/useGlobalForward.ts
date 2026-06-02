import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  deleteForwardServerApi,
  getForwardServerList,
  getGlobalForwardConfig,
  saveForwardServerApi,
  saveGlobalForwardConfig,
  updateGlobalForwardSwitchApi,
} from '../../api/forward'
import { useCrud } from '../core/useCrud'
import type { ForwardGlobalConfig, ForwardServer } from '../../types/modules'

type ServerQuery = Record<string, unknown>

export type ForwardServerFormModel = Partial<ForwardServer> & {
  name: string
  address: string
  port: number
  protocol: string
  priority: number
  status: string
}

const createDefaultGlobalForm = (): ForwardGlobalConfig => ({
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

const createServerFormData = (): ForwardServerFormModel => ({
  id: undefined,
  name: '',
  address: '',
  port: 53,
  protocol: 'UDP',
  priority: 10,
  status: '启用',
})

const clampNumber = (value: number | string | undefined, min: number, max: number): number =>
  Math.min(max, Math.max(min, Math.round(Number(value) || min)))

export const useGlobalForward = () => {
  const { t } = useI18n()
  const loadingConfig = ref(false)
  const submittingConfig = ref(false)
  const refreshLoading = ref(false)
  const deletingServerId = ref<number | null>(null)
  const lastUpdated = ref('')
  const globalForm = reactive<ForwardGlobalConfig>(createDefaultGlobalForm())
  const customDnsInputRef = ref<{ focus: () => void } | null>(null)
  const timeoutAdjustTimer = ref<number | null>(null)
  const retryAdjustTimer = ref<number | null>(null)
  const serverPortAdjustTimer = ref<number | null>(null)
  const serverPriorityAdjustTimer = ref<number | null>(null)
  let pollingTimer: number | null = null

  const queryParams = ref<ServerQuery>({})

  const crud = useCrud<ForwardServer, ServerQuery, ForwardServerFormModel, ForwardServerFormModel, number>(
    {
      getList: (params) => getForwardServerList(params),
      create: (payload) => saveForwardServerApi(payload),
      update: (payload) => saveForwardServerApi(payload),
      delete: (id) => deleteForwardServerApi(id),
    },
    queryParams,
    {
      createFormData: createServerFormData,
      getItemId: (item) => item.id as number | undefined,
      pageField: 'page',
      pageSizeField: 'size',
      watchQueryParams: false,
      autoFetch: false,
    },
  )

  const {
    loading: serverLoading,
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

  const isCustomDnsSelected = computed(() => globalForm.publicDns === 'custom')
  const dnsProviderOptions = computed(() => {
    const providers = globalForm.providers || []
    if (providers.some((item) => item.value === 'custom')) {
      return providers
    }
    return [...providers, { label: t('forward.customDnsLabel'), value: 'custom', ips: [] }]
  })

  const loadGlobalConfig = async (): Promise<void> => {
    loadingConfig.value = true
    try {
      const { data } = await getGlobalForwardConfig()
      Object.assign(globalForm, data || createDefaultGlobalForm())
      lastUpdated.value = new Date().toLocaleString('zh-CN', { hour12: false })
    } finally {
      loadingConfig.value = false
    }
  }

  const refreshData = async (options: { silent?: boolean } = {}): Promise<void> => {
    if (refreshLoading.value) {
      return
    }
    refreshLoading.value = true
    try {
      await Promise.all([loadGlobalConfig(), fetchData()])
      if (!options.silent) {
        ElMessage.success(t('forward.globalDataRefreshed'))
      }
    } finally {
      refreshLoading.value = false
    }
  }

  const handlePageChange = async (page: number): Promise<void> => {
    await fetchData({ currentPage: page })
  }

  const handlePageSizeChange = async (size: number): Promise<void> => {
    await fetchData({ currentPage: 1, pageSize: size })
  }

  const saveGlobal = async (): Promise<void> => {
    submittingConfig.value = true
    try {
      await saveGlobalForwardConfig({
        ...globalForm,
        servers: globalForm.servers,
      })
      await refreshData()
      ElMessage.success(t('forward.globalConfigSaved'))
    } finally {
      submittingConfig.value = false
    }
  }

  const resetGlobal = async (): Promise<void> => {
    await refreshData()
    ElMessage.success(t('forward.globalConfigReset'))
  }

  const toggleGlobalEnabled = async (value: boolean): Promise<void> => {
    globalForm.enabled = value
    await updateGlobalForwardSwitchApi({ enabled: value })
    ElMessage.success(t('forward.globalToggled', { status: value ? t('common.enabled') : t('common.disabled') }))
  }

  const togglePublicDnsEnabled = async (value: boolean): Promise<void> => {
    globalForm.publicDnsEnabled = value
    await updateGlobalForwardSwitchApi({ publicDnsEnabled: value })
    ElMessage.success(t('forward.publicDnsToggled', { status: value ? t('common.enabled') : t('common.disabled') }))
  }

  const openCreateDialog = (): void => {
    handleCreate()
    formData.value = createServerFormData()
  }

  const openEditDialog = (row: ForwardServer): void => {
    handleEdit(row)
  }

  const closeDialog = (): void => {
    dialogVisible.value = false
  }

  const resetServerForm = (): void => {
    formData.value = createServerFormData()
  }

  const submitServer = async (): Promise<void> => {
    const isEdit = Boolean(formData.value.id)
    formData.value.port = clampNumber(formData.value.port, 1, 65535)
    formData.value.priority = clampNumber(formData.value.priority, 1, 100)
    await handleSubmit()
    await loadGlobalConfig()
    ElMessage.success(isEdit ? t('forward.serverUpdatedDone') : t('forward.serverCreatedDone'))
  }

  const removeServer = async (row: ForwardServer): Promise<void> => {
    deletingServerId.value = row.id
    try {
      await handleDelete(row.id)
      await loadGlobalConfig()
      ElMessage.success(t('forward.serverDeletedDone'))
    } finally {
      deletingServerId.value = null
    }
  }

  const fillBuiltInProvider = (providerValue: string): void => {
    if (providerValue === 'custom') {
      customDnsInputRef.value?.focus()
      return
    }
    const provider = globalForm.providers.find((item) => item.value === providerValue)
    if (provider) {
      globalForm.publicDnsCustom = provider.ips.join(', ')
    }
  }

  const normalizeCustomDns = (): void => {
    globalForm.publicDnsCustom = String(globalForm.publicDnsCustom || '')
      .split(/[\s,\n]+/)
      .map((item) => item.trim())
      .filter(Boolean)
      .join(',')
  }

  const debounceAdjustTimeout = (value: number | string | undefined): void => {
    if (timeoutAdjustTimer.value) {
      window.clearTimeout(timeoutAdjustTimer.value)
    }
    timeoutAdjustTimer.value = window.setTimeout(() => {
      globalForm.timeout = clampNumber(value, 1, 60)
      timeoutAdjustTimer.value = null
    }, 120)
  }

  const debounceAdjustRetry = (value: number | string | undefined): void => {
    if (retryAdjustTimer.value) {
      window.clearTimeout(retryAdjustTimer.value)
    }
    retryAdjustTimer.value = window.setTimeout(() => {
      globalForm.retries = clampNumber(value, 1, 10)
      retryAdjustTimer.value = null
    }, 120)
  }

  const debounceAdjustServerPort = (value: number | string | undefined): void => {
    if (serverPortAdjustTimer.value) {
      window.clearTimeout(serverPortAdjustTimer.value)
    }
    serverPortAdjustTimer.value = window.setTimeout(() => {
      formData.value.port = clampNumber(value, 1, 65535)
      serverPortAdjustTimer.value = null
    }, 120)
  }

  const debounceAdjustServerPriority = (value: number | string | undefined): void => {
    if (serverPriorityAdjustTimer.value) {
      window.clearTimeout(serverPriorityAdjustTimer.value)
    }
    serverPriorityAdjustTimer.value = window.setTimeout(() => {
      formData.value.priority = clampNumber(value, 1, 100)
      serverPriorityAdjustTimer.value = null
    }, 120)
  }

  const startPolling = (): void => {
    if (pollingTimer) {
      window.clearInterval(pollingTimer)
    }
    pollingTimer = window.setInterval(() => {
      void refreshData({ silent: true })
    }, 30000)
  }

  const stopPolling = (): void => {
    if (pollingTimer) {
      window.clearInterval(pollingTimer)
      pollingTimer = null
    }
  }

  onMounted(async () => {
    await refreshData()
    startPolling()
  })

  onBeforeUnmount(() => {
    if (timeoutAdjustTimer.value) {
      window.clearTimeout(timeoutAdjustTimer.value)
    }
    if (retryAdjustTimer.value) {
      window.clearTimeout(retryAdjustTimer.value)
    }
    if (serverPortAdjustTimer.value) {
      window.clearTimeout(serverPortAdjustTimer.value)
    }
    if (serverPriorityAdjustTimer.value) {
      window.clearTimeout(serverPriorityAdjustTimer.value)
    }
    stopPolling()
  })

  return {
    loadingConfig,
    submittingConfig,
    refreshLoading,
    serverLoading,
    deletingServerId,
    lastUpdated,
    globalForm,
    customDnsInputRef,
    tableData,
    pagination,
    dialogVisible,
    formData,
    isCustomDnsSelected,
    dnsProviderOptions,
    fetchData,
    refreshData,
    handlePageChange,
    handlePageSizeChange,
    saveGlobal,
    resetGlobal,
    toggleGlobalEnabled,
    togglePublicDnsEnabled,
    openCreateDialog,
    openEditDialog,
    closeDialog,
    resetServerForm,
    submitServer,
    removeServer,
    fillBuiltInProvider,
    normalizeCustomDns,
    debounceAdjustTimeout,
    debounceAdjustRetry,
    debounceAdjustServerPort,
    debounceAdjustServerPriority,
  }
}

export default useGlobalForward