import axios from 'axios'
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { UploadFile, UploadRawFile } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  createBackup,
  deleteBackup,
  getBackupList,
  getSettingModuleDataApi,
  restoreBackup,
  saveCommonConfigApi,
  uploadBackup,
} from '../../api/setting'
import { confirmRiskAction } from '../../utils/interaction'
import type { SettingBackup, SettingCommonConfig } from '../../types/modules'

type BackupFormat = 'JSON' | 'Excel'

type BackupScopeOption = {
  value: string
  label: string
}

export interface BackupFilterForm {
  keyword: string
  format: '' | BackupFormat
}

export interface BackupFormModel {
  backupScope: string[]
  backupFormat: BackupFormat
}

export interface RestoreFormModel {
  fileName: string
  availableScope: string[]
  restoreScope: string[]
  fileReady: boolean
  uploadProgress: number
  restoreProgress: number
  format: string
}

export interface BackupPagination {
  page: number
  size: number
  total: number
}

const ALL_FETCH_SIZE = 500

// Only JSON is a valid backup format: the file has to carry every row of
// every selected table, which Excel cannot represent without lossy
// flattening. The legacy "Excel" option produced a metadata-only sheet
// that could never be restored, so it's been removed end-to-end.
const BACKUP_FORMAT_OPTIONS: BackupFormat[] = ['JSON']

const formatDateTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (num: number) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

// triggerBackupDownload pulls the actual on-disk backup file from the
// server and saves it to the operator's machine. Previously this helper
// rebuilt a metadata-only JSON locally (just backupId / scope / time /
// size) which produced a file that could **never** be used to restore —
// the screenshot the user reported was that exact bug. The legacy XLSX
// branch is gone for the same reason: an Excel sheet can't carry the
// table rows needed for restore.
//
// We bypass the shared axios `request` instance because its global
// response interceptor unwraps `response.data`, which would discard the
// Content-Disposition header we use to pick a sensible filename. Auth is
// reattached manually so the request is still authenticated.
const triggerBackupDownload = async (payload: SettingBackup): Promise<void> => {
  const token = localStorage.getItem('modern-dns-token') || ''
  const res = await axios.get(`/api/setting/backups/${payload.id}/export`, {
    responseType: 'blob',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    timeout: 120000,
  })

  // The endpoint falls back to JSON metadata if the file is missing on
  // disk; detect that and surface it as an error instead of saving a
  // useless metadata stub.
  const contentType = String(res.headers?.['content-type'] || '')
  if (contentType.includes('application/json')) {
    const text = await (res.data as Blob).text()
    let parsed: { code?: number; message?: string } | undefined
    try { parsed = JSON.parse(text) } catch { /* not json — fall through and save */ }
    if (parsed && (parsed.code !== 0 || !text.includes('"data"'))) {
      throw new Error(parsed.message || '备份文件不存在或已被清理')
    }
  }

  const disposition = String(res.headers?.['content-disposition'] || '')
  const match = disposition.match(/filename="?([^";]+)"?/i)
  const filename = match?.[1] || payload.fileName || `${payload.backupId}.json`

  const url = URL.createObjectURL(res.data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export const useBackupRestore = () => {
  const { t } = useI18n()
  const loading = ref(false)
  const backupCreating = ref(false)
  const backupDeletingId = ref<number | null>(null)
  const uploadLoading = ref(false)
  const restoreLoading = ref(false)
  const rawBackups = ref<SettingBackup[]>([])
  const progressTimer = ref<number | null>(null)

  const backupScopeOptions = computed<BackupScopeOption[]>(() => [
    { value: '系统配置', label: t('backup.scopeSystemConfig') },
    { value: '域名解析', label: t('backup.scopeDnsRecords') },
    { value: '黑白名单', label: t('backup.scopeBlackWhiteList') },
    { value: '安全规则', label: t('backup.scopeSecurityRules') },
  ])

  const filterForm = reactive<BackupFilterForm>({
    keyword: '',
    format: '',
  })
  const pagination = reactive<BackupPagination>({
    page: 1,
    size: 10,
    total: 0,
  })
  const backupForm = reactive<BackupFormModel>({
    backupScope: ['系统配置', '域名解析'],
    backupFormat: 'JSON',
  })
  const restoreForm = reactive<RestoreFormModel>({
    fileName: '',
    availableScope: [],
    restoreScope: [],
    fileReady: false,
    uploadProgress: 0,
    restoreProgress: 0,
    format: '',
  })

  const filteredBackups = computed<SettingBackup[]>(() => {
    const keyword = filterForm.keyword.trim().toLowerCase()
    return rawBackups.value.filter((item) => {
      const hitKeyword =
        !keyword ||
        [item.backupId, item.fileName, item.fileSize, item.format, ...(item.backupScope || [])]
          .some((field) => String(field || '').toLowerCase().includes(keyword))
      const hitFormat = !filterForm.format || item.format === filterForm.format
      return hitKeyword && hitFormat
    })
  })

  const sortedBackups = computed<SettingBackup[]>(() =>
    [...filteredBackups.value].sort((left, right) => right.backupTime.localeCompare(left.backupTime)),
  )

  const backupList = computed<SettingBackup[]>(() => {
    const start = (pagination.page - 1) * pagination.size
    return sortedBackups.value.slice(start, start + pagination.size)
  })

  const historyRecords = computed<SettingBackup[]>(() => sortedBackups.value)
  const recentHistoryRecords = computed<SettingBackup[]>(() => sortedBackups.value.slice(0, 5))

  watch(
    sortedBackups,
    (rows) => {
      pagination.total = rows.length
      const totalPages = Math.max(1, Math.ceil(rows.length / pagination.size))
      if (pagination.page > totalPages) {
        pagination.page = totalPages
      }
    },
    { immediate: true },
  )

  const clearProgressTimer = (): void => {
    if (progressTimer.value !== null) {
      window.clearInterval(progressTimer.value)
      progressTimer.value = null
    }
  }

  const resetRestoreState = (): void => {
    clearProgressTimer()
    restoreForm.fileName = ''
    restoreForm.availableScope = []
    restoreForm.restoreScope = []
    restoreForm.fileReady = false
    restoreForm.uploadProgress = 0
    restoreForm.restoreProgress = 0
    restoreForm.format = ''
  }

  const fetchBackupList = async (): Promise<void> => {
    loading.value = true
    try {
      const { data } = await getBackupList({ page: 1, size: ALL_FETCH_SIZE })
      rawBackups.value = data.list || []
    } catch (_error) {
      ElMessage.error(t('backup.listLoadFailed'))
    } finally {
      loading.value = false
    }
  }

  const handleSearch = (): void => {
    pagination.page = 1
  }

  const handleResetFilter = (): void => {
    filterForm.keyword = ''
    filterForm.format = ''
    pagination.page = 1
  }

  const handlePageChange = (page: number, size = pagination.size): void => {
    pagination.page = page
    pagination.size = size
  }

  const handleCreateBackup = async (): Promise<void> => {
    if (backupCreating.value) {
      return
    }
    if (!backupForm.backupScope.length) {
      ElMessage.warning(t('backup.selectBackupScope'))
      return
    }

    try {
      backupCreating.value = true
      await confirmRiskAction({
        title: t('backup.createConfirmTitle'),
        action: t('backup.createConfirmAction'),
        risk: t('backup.createConfirmRisk'),
      })
      const { data } = await createBackup({ ...backupForm })
      rawBackups.value = [data, ...rawBackups.value.filter((item) => item.id !== data.id)]
      // Pull the *actual* backup file from the server immediately so the
      // operator gets the same artifact that's stored on disk and that a
      // future restore would consume. Failure here doesn't undo the
      // backup itself — it's still in the listing, just not auto-saved.
      try {
        await triggerBackupDownload(data)
      } catch (downloadErr: any) {
        ElMessage.warning(t('backup.autoDownloadFailed', { msg: downloadErr?.message || '' }))
      }

      // Backend now reports per-table row counts and any tables it had
      // to skip due to corrupt JSON columns. Surface both so the
      // operator can tell at a glance whether the backup is "complete"
      // or "complete-ish with warnings". The legacy single-line success
      // toast is preserved when there's nothing notable to flag.
      const warnedCount = data.warnedTables ? Object.keys(data.warnedTables).length : 0
      if (warnedCount > 0) {
        ElMessage.warning(
          t('backup.createSuccessWithWarn', {
            fileName: data.fileName,
            rows: data.totalRows ?? 0,
            warned: warnedCount,
          }),
        )
      } else if (typeof data.totalRows === 'number') {
        ElMessage.success(
          t('backup.createSuccessWithStats', {
            fileName: data.fileName,
            rows: data.totalRows,
          }),
        )
      } else {
        ElMessage.success(t('backup.createSuccess', { fileName: data.fileName }))
      }
    } catch (error: any) {
      // `cancel` is the rejection value from confirmRiskAction when the
      // operator backs out of the confirm dialog — silent treatment.
      if (error === 'cancel') {
        return
      }
      // The shared axios interceptor leaves us a structured error: the
      // body is on `error.response.data.message`. Falling back to
      // `error.message` covers blob-mode requests and network failures.
      const msg = error?.response?.data?.message || error?.message
      ElMessage.error(msg ? `${t('backup.createFailed')}: ${msg}` : t('backup.createFailed'))
    } finally {
      backupCreating.value = false
    }
  }

  const handleDownloadBackup = async (row: SettingBackup): Promise<void> => {
    try {
      await triggerBackupDownload(row)
      ElMessage.success(t('backup.downloadStarted', { fileName: row.fileName }))
    } catch (error: any) {
      const msg = error?.message
      ElMessage.error(msg ? `${t('backup.downloadFailed')}: ${msg}` : t('backup.downloadFailed'))
    }
  }

  const handleDeleteBackup = async (row: SettingBackup): Promise<void> => {
    if (backupDeletingId.value) {
      return
    }

    try {
      backupDeletingId.value = row.id
      await confirmRiskAction({
        title: t('backup.deleteConfirmTitle'),
        action: t('backup.deleteConfirmAction'),
        target: row.backupId,
        risk: t('backup.deleteConfirmRisk'),
      })
      await deleteBackup(row.id)
      rawBackups.value = rawBackups.value.filter((item) => item.id !== row.id)
      ElMessage.success(t('backup.deleted'))
    } catch (_error) {
      if (_error !== 'cancel') {
        ElMessage.error(t('backup.deleteFailed'))
      }
    } finally {
      backupDeletingId.value = null
    }
  }

  const handleBackupFileChange = async (uploadFile: Pick<UploadFile, 'name'>): Promise<void> => {
    const fileName = String(uploadFile.name || '').trim()
    if (!fileName) {
      ElMessage.warning(t('backup.selectFile'))
      return
    }

    clearProgressTimer()
    uploadLoading.value = true
    restoreForm.fileName = fileName
    restoreForm.uploadProgress = 15

    try {
      const { data } = await uploadBackup({ fileName })
      restoreForm.uploadProgress = 100
      restoreForm.fileReady = true
      restoreForm.availableScope = [...(data?.restoreScope || [])]
      restoreForm.restoreScope = [...(data?.restoreScope || [])]
      restoreForm.format = data?.format || ''
      ElMessage.success(t('backup.fileParsed'))
    } catch (_error) {
      resetRestoreState()
      ElMessage.error(t('backup.fileInvalid'))
    } finally {
      uploadLoading.value = false
      window.setTimeout(() => {
        if (!uploadLoading.value && restoreForm.fileReady) {
          restoreForm.uploadProgress = 0
        }
      }, 400)
    }
  }

  const beforeBackupUpload = (rawFile: UploadRawFile): false => {
    void handleBackupFileChange({ name: rawFile.name })
    return false
  }

  const handleRestore = async (): Promise<void> => {
    if (restoreLoading.value) {
      return
    }
    if (!restoreForm.fileReady) {
      ElMessage.warning(t('backup.uploadFirst'))
      return
    }
    if (!restoreForm.restoreScope.length) {
      ElMessage.warning(t('backup.selectRestoreScope'))
      return
    }

    try {
      restoreLoading.value = true
      await confirmRiskAction({
        title: t('backup.restoreConfirmTitle'),
        action: t('backup.restoreConfirmAction'),
        risk: t('backup.restoreConfirmRisk'),
      })
      restoreForm.restoreProgress = 10
      clearProgressTimer()
      progressTimer.value = window.setInterval(() => {
        if (restoreForm.restoreProgress >= 90) {
          clearProgressTimer()
          return
        }
        restoreForm.restoreProgress += 20
      }, 120)

      await restoreBackup({
        fileName: restoreForm.fileName,
        restoreScope: [...restoreForm.restoreScope],
      })

      clearProgressTimer()
      restoreForm.restoreProgress = 100
      ElMessage.success(t('backup.restoreDone'))
      window.setTimeout(() => {
        resetRestoreState()
      }, 500)
    } catch (_error) {
      clearProgressTimer()
      restoreForm.restoreProgress = 0
      if (_error !== 'cancel') {
        ElMessage.error(t('backup.restoreFailed'))
      }
    } finally {
      restoreLoading.value = false
    }
  }

  const scopeLabel = (value: string): string => {
    const map: Record<string, string> = {
      '系统配置': t('backup.scopeSystemConfig'),
      '域名解析': t('backup.scopeDnsRecords'),
      '黑白名单': t('backup.scopeBlackWhiteList'),
      '安全规则': t('backup.scopeSecurityRules'),
    }
    return map[value] || value
  }

  // ─── Auto-backup config ─────────────────────────────────────────────
  //
  // The scheduler at @backend/pkg/scheduler/scheduler.go drives auto
  // backup off the same `system_config` row that powers the system
  // settings page. We surface those four fields right next to the
  // manual-backup controls so the operator never has to navigate away
  // to control the schedule. State is loaded fresh on mount and the
  // "Save" button persists via the existing common-config endpoint —
  // no parallel API surface, so settings stay in sync no matter which
  // page edits them.
  const autoBackupConfig = reactive({
    autoBackup: false,
    backupCycle: 'daily' as 'daily' | 'weekly' | 'monthly',
    backupRetentionDays: 30,
    backupStorageType: 'local' as 'local' | 's3' | 'sftp',
    backupStoragePath: '/var/dns/backups',
  })
  const autoBackupLoading = ref(false)
  const autoBackupSaving = ref(false)
  // Snapshot of the full common config so we can save the backup-related
  // subset without clobbering the other dozen fields the system page
  // also writes to (TTL / DNSSEC / login lockout / theme color etc).
  const commonConfigSnapshot = ref<Partial<SettingCommonConfig> | null>(null)

  // Server stores backupCycle as Chinese ("每日"/"每周"/"每月") in some
  // builds and as English ("daily"/"weekly"/"monthly") in others.
  // Normalize to the English form for the dropdown and translate back
  // when saving so we don't fight existing server-side validation.
  const normalizeCycle = (raw: string): 'daily' | 'weekly' | 'monthly' => {
    const v = String(raw || '').trim().toLowerCase()
    if (v === 'weekly' || raw === '每周') return 'weekly'
    if (v === 'monthly' || raw === '每月') return 'monthly'
    return 'daily'
  }

  const fetchAutoBackupConfig = async (): Promise<void> => {
    autoBackupLoading.value = true
    try {
      const { data } = await getSettingModuleDataApi() as { data: { common: SettingCommonConfig } }
      commonConfigSnapshot.value = { ...data.common }
      autoBackupConfig.autoBackup           = !!data.common.autoBackup
      autoBackupConfig.backupCycle          = normalizeCycle(data.common.backupCycle)
      autoBackupConfig.backupRetentionDays  = Number(data.common.backupRetentionDays) || 30
      autoBackupConfig.backupStorageType    = (data.common.backupStorageType || 'local') as typeof autoBackupConfig.backupStorageType
      autoBackupConfig.backupStoragePath    = data.common.backupStoragePath || '/var/dns/backups'
    } catch (_e) {
      ElMessage.error(t('backup.autoConfigLoadFailed'))
    } finally {
      autoBackupLoading.value = false
    }
  }

  const saveAutoBackupConfig = async (): Promise<void> => {
    // Range guard mirrors the server: retention outside 1–365 days makes
    // the scheduler's prune step either no-op or aggressive in a way the
    // operator probably didn't mean.
    if (autoBackupConfig.backupRetentionDays < 1 || autoBackupConfig.backupRetentionDays > 365) {
      ElMessage.warning(t('backup.autoRetentionRange'))
      return
    }
    if (autoBackupConfig.autoBackup && !autoBackupConfig.backupStoragePath.trim()) {
      ElMessage.warning(t('backup.autoStoragePathRequired'))
      return
    }

    autoBackupSaving.value = true
    try {
      const merged: SettingCommonConfig = {
        ...(commonConfigSnapshot.value as SettingCommonConfig),
        autoBackup:           autoBackupConfig.autoBackup,
        backupCycle:          autoBackupConfig.backupCycle,
        backupRetentionDays:  autoBackupConfig.backupRetentionDays,
        backupStorageType:    autoBackupConfig.backupStorageType,
        backupStoragePath:    autoBackupConfig.backupStoragePath.trim(),
      }
      await saveCommonConfigApi(merged)
      commonConfigSnapshot.value = merged
      ElMessage.success(t('backup.autoConfigSaved'))
    } catch (e: any) {
      const msg = e?.response?.data?.message || e?.message
      ElMessage.error(msg ? `${t('backup.autoConfigSaveFailed')}: ${msg}` : t('backup.autoConfigSaveFailed'))
    } finally {
      autoBackupSaving.value = false
    }
  }

  onScopeDispose(() => {
    clearProgressTimer()
  })

  return {
    backupScopeOptions,
    backupFormatOptions: BACKUP_FORMAT_OPTIONS,
    filterForm,
    backupForm,
    restoreForm,
    pagination,
    loading,
    backupCreating,
    backupDeletingId,
    uploadLoading,
    restoreLoading,
    backupList,
    historyRecords,
    recentHistoryRecords,
    fetchBackupList,
    handleSearch,
    handleResetFilter,
    handlePageChange,
    handleCreateBackup,
    handleDownloadBackup,
    handleDeleteBackup,
    handleBackupFileChange,
    beforeBackupUpload,
    handleRestore,
    resetRestoreState,
    formatDateTime,
    scopeLabel,
    // Auto-backup config
    autoBackupConfig,
    autoBackupLoading,
    autoBackupSaving,
    fetchAutoBackupConfig,
    saveAutoBackupConfig,
  }
}

export default useBackupRestore