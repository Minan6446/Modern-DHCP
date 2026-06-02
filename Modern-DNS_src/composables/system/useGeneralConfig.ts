import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  getSettingModuleDataApi,
  resetCommonConfigApi,
  saveCommonConfigApi,
} from '../../api/setting'
import { useAppStore } from '../../stores/app'
import type { GeneralConfigForm } from '../../types/setting'
import {
  createDefaultGeneralConfigForm,
  mergeGeneralConfigForm,
  toSettingCommonConfigPayload,
} from '../../types/setting'
import { confirmRiskAction } from '../../utils/interaction'
import { setI18nLanguage } from '../../i18n'

const normalizeRetentionDays = (value: number): number => {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    return 1
  }
  return Math.min(365, Math.max(1, Math.round(numeric)))
}

const normalizeBackupCycle = (value?: string): 'daily' | 'weekly' | 'monthly' => {
  const normalized = String(value || '').trim().toLowerCase()
  if (normalized === 'daily' || normalized === '每日') return 'daily'
  if (normalized === 'weekly' || normalized === '每周') return 'weekly'
  if (normalized === 'monthly' || normalized === '每月') return 'monthly'
  return 'daily'
}

export const useGeneralConfig = () => {
  const { t } = useI18n()
  const appStore = useAppStore()
  const loading = ref(false)
  const configForm = reactive<GeneralConfigForm>(createDefaultGeneralConfigForm())

  const syncAppSystemConfig = (): void => {
    appStore.updateSystemConfig({
      timezone: configForm.timezone,
      language: configForm.language,
      autoBackup: configForm.autoBackup,
      dnssecGlobal: configForm.dnssecGlobal,
      maintenanceEnabled: configForm.maintenanceEnabled,
    })
  }

  const assignConfigForm = (payload?: Partial<GeneralConfigForm>): void => {
    Object.assign(configForm, mergeGeneralConfigForm(payload))
    configForm.backupCycle = normalizeBackupCycle(configForm.backupCycle)
    configForm.backupRetentionDays = normalizeRetentionDays(configForm.backupRetentionDays)
  }

  const loadConfig = async (): Promise<void> => {
    loading.value = true
    try {
      const { data } = await getSettingModuleDataApi()
      assignConfigForm(data.common)
      const savedLang = localStorage.getItem('modern-dns-lang')
      if (savedLang === 'en-US' || savedLang === 'zh-CN') {
        configForm.language = savedLang
      }
      syncAppSystemConfig()
    } catch (_error) {
      ElMessage.error(t('setting.generalConfigLoadFailed'))
    } finally {
      loading.value = false
    }
  }

  const saveConfig = async (): Promise<void> => {
    if (loading.value) {
      return
    }
    try {
      loading.value = true
      configForm.backupRetentionDays = normalizeRetentionDays(configForm.backupRetentionDays)
      await confirmRiskAction({
        title: t('setting.generalSaveConfirmTitle'),
        action: t('setting.generalSaveConfirmAction'),
        risk: t('setting.generalSaveConfirmRisk'),
      })
      const payload = toSettingCommonConfigPayload(configForm)
      // First attempt — backend may refuse with code 4422 if the new
      // IPWhitelist would lock the operator out. We surface that as a
      // confirm dialog rather than a silent error so the operator sees
      // the consequence before committing.
      const first = await saveCommonConfigApi(payload)
      if (first?.code === 4422) {
        await ElMessageBox.confirm(
          (first.message as string) || t('cluster.ipLockoutMessage'),
          t('cluster.ipLockoutTitle'),
          {
            type: 'warning',
            confirmButtonText: t('cluster.ipLockoutConfirm'),
            cancelButtonText: t('common.cancel'),
          },
        )
        await saveCommonConfigApi(payload, { confirmLockout: true })
      }
      syncAppSystemConfig()
      ElMessage.success(t('setting.generalSaveSuccess'))
    } catch (_error) {
      if (_error !== 'cancel') {
        ElMessage.error(t('setting.generalSaveFailed'))
      }
    } finally {
      loading.value = false
    }
  }

  const resetConfig = async (): Promise<void> => {
    if (loading.value) {
      return
    }
    try {
      loading.value = true
      await confirmRiskAction({
        title: t('setting.generalResetConfirmTitle'),
        action: t('setting.generalResetConfirmAction'),
        risk: t('setting.generalResetConfirmRisk'),
      })
      const { data } = await resetCommonConfigApi()
      assignConfigForm(data)
      syncAppSystemConfig()
      ElMessage.success(t('setting.generalResetSuccess'))
    } catch (_error) {
      if (_error !== 'cancel') {
        ElMessage.error(t('setting.generalResetFailed'))
      }
    } finally {
      loading.value = false
    }
  }

  watch(
    () => [
      configForm.timezone,
      configForm.language,
      configForm.autoBackup,
      configForm.dnssecGlobal,
      configForm.maintenanceEnabled,
    ],
    () => {
      syncAppSystemConfig()
    },
  )

  watch(
    () => configForm.language,
    (lang) => {
      if (lang) setI18nLanguage(lang)
    },
  )

  onMounted(() => {
    loadConfig()
  })

  return {
    configForm,
    loading,
    saveConfig,
    resetConfig,
  }
}

export default useGeneralConfig