<template>
  <div class="page-wrap">
    <el-card shadow="never" class="surface-card config-shell" v-loading="pageLoading">
      <div class="title-block">
        <h3>{{ t('system.general.pageTitle') }}</h3>
        <p class="desc">{{ t('system.general.pageDesc') }}</p>
      </div>

      <div class="section-grid">
        <el-card shadow="never" class="section-card">
          <template #header>
            <div class="section-header">
              <span class="section-title">{{ t('system.general.ntpTitle') }}</span>
              <el-tag :type="ntpStatusType" size="small">{{ ntpStatusText }}</el-tag>
            </div>
          </template>

          <el-form label-position="left" label-width="130px" class="config-form">
            <el-form-item :label="t('system.general.ntpAutoSync')">
              <el-switch v-model="ntpForm.enabled" />
            </el-form-item>

            <el-form-item :label="t('system.general.ntpServers')">
              <el-input
                v-model="ntpForm.servers"
                type="textarea"
                :rows="3"
                :placeholder="t('system.general.ntpServersPh')"
              />
              <div class="field-help">{{ t('system.general.ntpServersHelp') }}</div>
            </el-form-item>

            <el-form-item :label="t('system.general.ntpInterval')">
              <el-input-number v-model="ntpForm.intervalMinutes" :min="1" :max="1440" />
            </el-form-item>

            <el-form-item :label="t('system.general.ntpTimeout')">
              <el-input-number v-model="ntpForm.timeoutSeconds" :min="1" :max="30" />
            </el-form-item>

            <el-form-item :label="t('system.general.timezone')">
              <el-select v-model="ntpForm.timezone" style="width: 100%">
                <el-option
                  v-for="tz in timezoneOptions"
                  :key="tz.value"
                  :label="tz.label"
                  :value="tz.value"
                />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('system.general.lastSyncAt')">
              <span>{{ ntpForm.lastSyncAt || '-' }}</span>
            </el-form-item>
          </el-form>

          <div class="action-row">
            <el-button :loading="syncing" :disabled="pageLoading" @click="syncNow">{{ t('system.general.syncNow') }}</el-button>
          </div>
        </el-card>

        <el-card shadow="never" class="section-card">
          <template #header>
            <div class="section-header">
              <span class="section-title">{{ t('system.general.langTitle') }}</span>
            </div>
          </template>

          <el-form label-position="left" label-width="130px" class="config-form">
            <el-form-item :label="t('system.general.systemLanguage')">
              <el-select v-model="languageForm.locale" style="width: 100%">
                <el-option
                  v-for="item in availableLocales"
                  :key="item.code"
                  :label="item.label"
                  :value="item.code"
                />
              </el-select>
            </el-form-item>

            <el-form-item :label="t('system.general.localePreview')">
              <div class="locale-preview">
                <strong>{{ currentLocaleLabel }}</strong>
                <span>{{ t('system.general.currentTag', { tag: languageForm.locale }) }}</span>
              </div>
            </el-form-item>
          </el-form>

        </el-card>

        <el-card shadow="never" class="section-card">
          <template #header>
            <div class="section-header">
              <span class="section-title">{{ t('system.general.sessionTitle') }}</span>
            </div>
          </template>

          <el-form label-position="left" label-width="170px" class="config-form">
            <el-form-item :label="t('system.general.sessionTimeout')">
              <el-input-number v-model="sessionForm.timeoutMinutes" :min="5" :max="1440" />
              <div class="field-help">{{ t('system.general.sessionTimeoutHelp') }}</div>
            </el-form-item>

            <el-form-item :label="t('system.general.autoLogout')">
              <el-switch v-model="sessionForm.autoLogoutEnabled" />
            </el-form-item>
          </el-form>

        </el-card>

        <el-card shadow="never" class="section-card">
          <template #header>
            <div class="section-header">
              <span class="section-title">{{ t('system.general.logTitle') }}</span>
            </div>
          </template>

          <el-form label-position="left" label-width="170px" class="config-form">
            <el-form-item :label="t('system.general.systemLogRetention')">
              <el-input-number v-model="logForm.systemRetentionDays" :min="1" :max="3650" />
            </el-form-item>

            <el-form-item :label="t('system.general.auditLogRetention')">
              <el-input-number v-model="logForm.auditRetentionDays" :min="1" :max="3650" />
            </el-form-item>

            <el-form-item :label="t('system.general.logPushEnabled')">
              <el-switch v-model="logForm.pushEnabled" />
            </el-form-item>

            <el-form-item :label="t('system.general.logPushEndpoint')">
              <el-input v-model="logForm.pushEndpoint" :disabled="!logForm.pushEnabled" :placeholder="t('system.general.logPushEndpointPh')" />
            </el-form-item>

            <el-form-item :label="t('system.general.logPushLevel')">
              <el-select v-model="logForm.pushMinLevel" :disabled="!logForm.pushEnabled" style="width: 100%">
                <el-option v-for="item in logLevelOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
          </el-form>

        </el-card>
      </div>

      <div class="global-action-row">
        <el-button :disabled="pageLoading || savingAll" @click="resetAllSettings">{{ t('common.reset') }}</el-button>
        <el-button type="primary" :loading="savingAll" :disabled="pageLoading" @click="saveAllSettings">{{ t('common.save') }}</el-button>
      </div>

    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useLocale } from '@/i18n/useLocale';
import { resolveLocale } from '@/i18n';
import {
  getSystemGeneralSettings,
  syncSystemNtpNow,
  updateSystemGeneralSettings,
  type OpsSystemSummary
} from '@/api/system/general';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

type NtpStatus = 'idle' | 'ok';

interface NtpSettings {
  enabled: boolean;
  servers: string;
  intervalMinutes: number;
  timeoutSeconds: number;
  timezone: string;
  lastSyncAt: string;
  status: NtpStatus;
}

interface SessionPolicySettings {
  timeoutMinutes: number;
  autoLogoutEnabled: boolean;
}

interface LogPolicySettings {
  systemRetentionDays: number;
  auditRetentionDays: number;
  pushEnabled: boolean;
  pushEndpoint: string;
  pushMinLevel: string;
}

const timezoneOptions = [
  { label: 'UTC+08:00 Asia/Shanghai', value: 'Asia/Shanghai' },
  { label: 'UTC+00:00 Europe/London', value: 'Europe/London' },
  { label: 'UTC+00:00 UTC', value: 'UTC' },
  { label: 'UTC-05:00 America/New_York', value: 'America/New_York' },
  { label: 'UTC+09:00 Asia/Tokyo', value: 'Asia/Tokyo' }
];

const logLevelOptions = [
  { label: 'Warning', value: 'warning' },
  { label: 'Error', value: 'error' },
  { label: 'Info', value: 'info' }
];

const { t } = useI18n();
const { current, available: availableLocales, setLocale } = useLocale();

const pageLoading = ref(false);
const savingAll = ref(false);
const syncing = ref(false);

const ntpForm = reactive<NtpSettings>({
  enabled: true,
  servers: 'pool.ntp.org\nntp.aliyun.com\ntime.cloudflare.com',
  intervalMinutes: 30,
  timeoutSeconds: 5,
  timezone: 'Asia/Shanghai',
  lastSyncAt: '',
  status: 'idle'
});

const ntpSnapshot = ref<NtpSettings>({ ...ntpForm });

const languageForm = reactive({ locale: current.value.code });
const languageSnapshot = ref({ locale: current.value.code });

const sessionForm = reactive<SessionPolicySettings>({
  timeoutMinutes: 30,
  autoLogoutEnabled: true
});
const sessionSnapshot = ref<SessionPolicySettings>({ ...sessionForm });

const logForm = reactive<LogPolicySettings>({
  systemRetentionDays: 30,
  auditRetentionDays: 180,
  pushEnabled: false,
  pushEndpoint: '',
  pushMinLevel: 'warning'
});
const logSnapshot = ref<LogPolicySettings>({ ...logForm });

const formatDateTime = (value?: string) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

const ntpStatusType = computed(() => (ntpForm.status === 'ok' ? 'success' : 'info'));
const ntpStatusText = computed(() =>
  ntpForm.status === 'ok' ? t('system.general.syncStatusOk') : t('system.general.syncStatusIdle')
);

const currentLocaleLabel = computed(() => {
  const hit = availableLocales.find((item) => item.code === languageForm.locale);
  return hit?.label || languageForm.locale;
});

const applySummary = (summary: OpsSystemSummary) => {
  ntpForm.enabled = !!summary.ntpEnabled;
  ntpForm.servers = summary.ntpServers || ntpForm.servers;
  ntpForm.intervalMinutes = summary.ntpIntervalMinutes || ntpForm.intervalMinutes;
  ntpForm.timeoutSeconds = summary.ntpTimeoutSeconds || ntpForm.timeoutSeconds;
  ntpForm.timezone = summary.timezone || ntpForm.timezone;
  ntpForm.lastSyncAt = formatDateTime(summary.ntpLastSyncAt);
  ntpForm.status = summary.ntpSyncStatus === 'ok' ? 'ok' : 'idle';

  languageForm.locale = resolveLocale(summary.locale || current.value.code).code;
  sessionForm.timeoutMinutes = summary.adminSessionTimeoutMinutes || sessionForm.timeoutMinutes;
  sessionForm.autoLogoutEnabled = summary.autoLogoutEnabled ?? sessionForm.autoLogoutEnabled;
  logForm.systemRetentionDays = summary.systemLogRetentionDays || logForm.systemRetentionDays;
  logForm.auditRetentionDays = summary.auditLogRetentionDays || logForm.auditRetentionDays;
  logForm.pushEnabled = summary.logPushEnabled ?? logForm.pushEnabled;
  logForm.pushEndpoint = summary.logPushEndpoint || '';
  logForm.pushMinLevel = summary.logPushMinLevel || logForm.pushMinLevel;

  ntpSnapshot.value = { ...ntpForm };
  languageSnapshot.value = { locale: languageForm.locale };
  sessionSnapshot.value = { ...sessionForm };
  logSnapshot.value = { ...logForm };
};

const loadGeneralSettings = async () => {
  pageLoading.value = true;
  try {
    const resp = await getSystemGeneralSettings();
    applySummary(resp.data);
  } catch (error) {
    showHttpError(error, t('system.general.loadFail'));
  } finally {
    pageLoading.value = false;
  }
};

const syncNow = async () => {
  syncing.value = true;
  try {
    const resp = await syncSystemNtpNow();
    applySummary(resp.data);
    showSuccess(t('system.general.syncSuccess'));
  } catch (error) {
    const status = (error as { response?: { status?: number } })?.response?.status;
    if (status === 404) {
      ntpForm.lastSyncAt = new Date().toLocaleString();
      ntpForm.status = 'ok';
      ntpSnapshot.value = { ...ntpForm };
      showWarning(t('system.general.syncUnsupported'));
      return;
    }
    showHttpError(error, t('system.general.syncFail'));
  } finally {
    syncing.value = false;
  }
};

const resetAllSettings = () => {
  Object.assign(ntpForm, ntpSnapshot.value);
  languageForm.locale = languageSnapshot.value.locale;
  Object.assign(sessionForm, sessionSnapshot.value);
  Object.assign(logForm, logSnapshot.value);
  showSuccess(t('system.general.allReset'));
};

const saveAllSettings = async () => {
  savingAll.value = true;
  try {
    const resp = await updateSystemGeneralSettings({
      ntpEnabled: ntpForm.enabled,
      ntpServers: ntpForm.servers,
      ntpIntervalMinutes: ntpForm.intervalMinutes,
      ntpTimeoutSeconds: ntpForm.timeoutSeconds,
      timezone: ntpForm.timezone,
      locale: languageForm.locale,
      adminSessionTimeoutMinutes: sessionForm.timeoutMinutes,
      autoLogoutEnabled: sessionForm.autoLogoutEnabled,
      systemLogRetentionDays: logForm.systemRetentionDays,
      auditLogRetentionDays: logForm.auditRetentionDays,
      logPushEnabled: logForm.pushEnabled,
      logPushEndpoint: logForm.pushEndpoint,
      logPushMinLevel: logForm.pushMinLevel
    });
    applySummary(resp.data);
    setLocale(languageForm.locale);
    showSuccess(t('system.general.allSaved'));
  } catch (error) {
    showHttpError(error, t('system.general.allSaveFail'));
  } finally {
    savingAll.value = false;
  }
};

onMounted(() => {
  loadGeneralSettings();
});
</script>

<style scoped>
.page-wrap {
  width: 100%;
}

.config-shell {
  border-radius: 12px;
}

.title-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 16px;
}

.title-block h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #1f2937;
}

.desc {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
}

.section-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.section-card {
  border-radius: 10px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #111827;
}

.config-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.field-help {
  margin-top: 6px;
  font-size: 12px;
  color: #6b7280;
  line-height: 1.5;
}

.action-row {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.global-action-row {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

.locale-preview {
  display: flex;
  flex-direction: column;
  gap: 2px;
  color: #4b5563;
}

@media (max-width: 1100px) {
  .section-grid {
    grid-template-columns: 1fr;
  }
}
</style>
