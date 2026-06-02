<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import type { GeneralConfigForm } from '../../types/setting'
import { testNtpApi, type NtpTestResult } from '../../api/setting'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{ model: GeneralConfigForm; loading?: boolean }>(),
  { loading: false },
)

const emit = defineEmits<{
  (e: 'update'): void
  (e: 'reset'): void
}>()

const formRef = ref<FormInstance>()
const activeTab = ref('system')

const timezoneOptions = computed(() => [
  { label: `UTC−12:00 (${t('setting.tz.bakerIsland')})`, value: 'UTC-12' },
  { label: `UTC−8:00 (${t('setting.tz.losAngeles')})`, value: 'UTC-8' },
  { label: `UTC−5:00 (${t('setting.tz.newYork')})`, value: 'UTC-5' },
  { label: `UTC+0:00 (${t('setting.tz.london')})`, value: 'UTC' },
  { label: `UTC+1:00 (${t('setting.tz.berlin')})`, value: 'UTC+1' },
  { label: `UTC+3:00 (${t('setting.tz.moscow')})`, value: 'UTC+3' },
  { label: `UTC+5:30 (${t('setting.tz.newDelhi')})`, value: 'UTC+5:30' },
  { label: `UTC+8:00 (${t('setting.tz.beijingShanghai')})`, value: 'UTC+8' },
  { label: `UTC+9:00 (${t('setting.tz.tokyo')})`, value: 'UTC+9' },
  { label: `UTC+11:00 (${t('setting.tz.sydney')})`, value: 'UTC+11' },
])

const rules = computed<FormRules>(() => ({
  timezone: [{ required: true, message: t('setting.timezoneRequired'), trigger: 'change' }],
  language: [{ required: true, message: t('setting.languageRequired'), trigger: 'change' }],
  // Backup retention validators removed alongside the data-backup
  // panel; the same field is now validated in BackupRestorePage's
  // auto-backup section instead.
  defaultTtl: [{ required: true, message: t('setting.defaultTtlRequired'), trigger: 'blur' }],
  negativeCacheTtl: [{ required: true, message: t('setting.negativeTtlRequired'), trigger: 'blur' }],
  loginTimeoutMinutes: [{ required: true, message: t('setting.sessionTimeoutRequired'), trigger: 'blur' }],
  loginMaxFailures: [{ required: true, message: t('setting.maxFailuresRequired'), trigger: 'blur' }],
  loginLockMinutes: [{ required: true, message: t('setting.lockDurationRequired'), trigger: 'blur' }],
  // 2026-05 cleanup: log-config + globalQpsThreshold rules removed.
  // Log fields (level/retention/format/syslog) are now owned exclusively
  // by the 「审计日志」 page so we don't double-edit the same
  // system_config row. globalQpsThreshold was a dead column — actual
  // QPS limiting reads ddos_global.qps_limit (「安全中心 → DDoS 防护」).
}))

// Used by every <el-input-number @change> on this page to clamp the
// emitted value into the documented range. Element Plus already prevents
// type-in values outside [min,max] but its arrow buttons fire for the
// pre-clamp value, so we run it through here just in case.
const clampInt = (v: number, min: number, max: number) => {
  const n = Number(v)
  return !Number.isFinite(n) ? min : Math.min(max, Math.max(min, Math.round(n)))
}

// ── NTP status panel helpers ──
//
// `ntpStatusLabel` / `ntpStatusClass` drive the badge in the panel
// header. Three states:
//   ok       — last poll succeeded and drift is within +/- 1s
//   warning  — last poll succeeded but drift is large (> 1s)
//   error    — last poll failed (ntpLastError populated)
//   inactive — feature disabled or never polled
//
// We deliberately don't depend on a separate health endpoint — the
// columns are written by the poller itself and read here as part of
// the General Settings GET, so a single network round-trip already
// gives us everything we need to render this section.
const ntpStatusLabel = computed(() => {
  if (!props.model.ntpEnabled) return t('setting.ntpStatusOff')
  if (props.model.ntpLastError) return t('setting.ntpStatusError')
  if (!props.model.ntpLastSync) return t('setting.ntpStatusPending')
  return t('setting.ntpStatusOk')
})

const ntpStatusClass = computed(() => {
  if (!props.model.ntpEnabled) return 'is-inactive'
  if (props.model.ntpLastError) return 'is-warning'
  if (!props.model.ntpLastSync) return 'is-inactive'
  // 1s drift threshold matches what the badge label considers "ok".
  // Above that we still mark green because the poll itself succeeded
  // — operators see the actual number in the disabled text input next
  // to the badge if they want to debug.
  return 'is-active'
})

// ── "立即测试" button state ──
//
// We surface the most recent test result inline (success or error
// strip) and clear it the next time the operator either:
//   (a) edits the server textarea — implicit "I'm trying again"
//   (b) hits the button again — we replace with the new result
//
// We deliberately *don't* persist this into the form model — the
// poller-owned status columns are the source of truth for the badge.
// The test panel is purely a quick-check affordance.
const ntpTesting = ref(false)
const ntpTestResult = ref<NtpTestResult | null>(null)

const handleNtpTest = async () => {
  ntpTesting.value = true
  ntpTestResult.value = null
  try {
    // Send the *current* textarea value, not the persisted one, so
    // operators can validate edits before saving. We split on the
    // same separators the backend's parseServerList accepts.
    const list = String(props.model.ntpServers || '')
      .split(/[\s,;]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    const { data } = await testNtpApi(list)
    ntpTestResult.value = data
    if (data.success) {
      ElMessage.success(t('setting.ntpTestOkToast'))
    } else {
      ElMessage.warning(t('setting.ntpTestFailToast'))
    }
  } catch (err: any) {
    // Network / 5xx — surface the raw message; this is rare since
    // the backend wraps NTP failures into 200 + success:false.
    ntpTestResult.value = { success: false, error: err?.message || String(err) }
    ElMessage.error(t('setting.ntpTestFailToast'))
  } finally {
    ntpTesting.value = false
  }
}

const ntpDriftDisplay = computed(() => {
  if (!props.model.ntpLastSync) return t('setting.ntpDriftPending')
  const ms = props.model.ntpLastDriftMs ?? 0
  const sign = ms >= 0 ? '+' : ''
  const lastSync = String(props.model.ntpLastSync || '').replace('T', ' ').slice(0, 19)
  return `${sign}${ms}ms (${lastSync})`
})

const emitUpdate = async () => {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid) emit('update')
}

// confirmMfaToggle is wired to <el-switch :before-change>. Returning a
// promise that resolves to `true` lets the toggle through; `false` keeps
// the existing state. We surface different copy depending on direction:
//
// ON  → operators are about to force every admin to bind TOTP on their
//       next login (including the *current* one if they haven't bound).
//       Worth a deliberate confirm, plus a hint about the recovery path
//       (admin reset password / DB-side toggle off) so an operator
//       doesn't lock themselves out of the console.
// OFF → relaxes a hardening posture; we still confirm so a stray click
//       doesn't silently weaken the security stance.
const confirmMfaToggle = async (): Promise<boolean> => {
  const turningOn = !props.model.mfaRequired
  try {
    await ElMessageBox.confirm(
      turningOn ? t('setting.mfaEnableConfirmMessage') : t('setting.mfaDisableConfirmMessage'),
      turningOn ? t('setting.mfaEnableConfirmTitle') : t('setting.mfaDisableConfirmTitle'),
      {
        type: turningOn ? 'warning' : 'info',
        confirmButtonText: turningOn ? t('setting.mfaEnableConfirmOk') : t('setting.mfaDisableConfirmOk'),
        cancelButtonText: t('common.cancel'),
        confirmButtonClass: turningOn ? 'el-button--warning' : '',
        dangerouslyUseHTMLString: true,
      },
    )
    return true
  } catch (_e) {
    return false
  }
}
</script>

<template>
  <el-form ref="formRef" :model="model" :rules="rules" label-position="top" require-asterisk-position="left" v-loading="loading">

    <!-- Status overview strip -->
    <div class="gs-stats">
      <div class="gs-stat">
        <div class="gs-stat-icon gs-stat-icon-dnssec">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5z" /></svg>
        </div>
        <div class="gs-stat-body">
          <div class="gs-stat-label">DNSSEC</div>
          <div class="gs-stat-value">
            <span class="gs-stat-dot" :class="model.dnssecGlobal ? 'is-on' : 'is-off'" />
            {{ model.dnssecGlobal ? $t('setting.on') : $t('setting.off') }}
          </div>
        </div>
      </div>
      <div class="gs-stat">
        <div class="gs-stat-icon gs-stat-icon-mfa">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M18 8h-1V6a5 5 0 0 0-10 0v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zm-6 9a2 2 0 1 1 2-2 2 2 0 0 1-2 2zm3.1-9H8.9V6a3.1 3.1 0 0 1 6.2 0z" /></svg>
        </div>
        <div class="gs-stat-body">
          <div class="gs-stat-label">{{ $t('setting.mfaLabel') }}</div>
          <div class="gs-stat-value">
            <span class="gs-stat-dot" :class="model.mfaRequired ? 'is-on' : 'is-off'" />
            {{ model.mfaRequired ? $t('setting.forced') : $t('setting.notEnabled') }}
          </div>
        </div>
      </div>
      <div class="gs-stat">
        <div class="gs-stat-icon gs-stat-icon-maint" :class="{ 'is-warning': model.maintenanceEnabled }">
          <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18"><path d="M22.7 19 13.6 9.9a5.89 5.89 0 0 0-6.5-7.8l3.6 3.6L8.1 8.3 4.5 4.7a5.89 5.89 0 0 0 7.8 6.5l9.1 9.1z" /></svg>
        </div>
        <div class="gs-stat-body">
          <div class="gs-stat-label">{{ $t('setting.maintLabel') }}</div>
          <div class="gs-stat-value">
            <span class="gs-stat-dot" :class="model.maintenanceEnabled ? 'is-warn' : 'is-off'" />
            <!-- Off-state copy aligned with the DNSSEC / MFA cards
                 above so all three read consistently. The previous
                 wording "运行中" was attached to the "维护模式" label,
                 which scanned as "maintenance mode currently running"
                 instead of "system running normally (not in
                 maintenance)". setting.notEnabled is the same key the
                 MFA card uses for the equivalent off state. -->
            {{ model.maintenanceEnabled ? $t('setting.maintMode') : $t('setting.notEnabled') }}
          </div>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="gs-tabs">

      <!-- ══ Tab 1: 系统 ══ -->
      <el-tab-pane name="system">
        <template #label>
          <span class="gs-tab-label">
            <svg class="gs-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M4 6h16v2H4zm0 5h16v2H4zm0 5h16v2H4z" /></svg>
            {{ $t('setting.tabSystem') }}
          </span>
        </template>

        <!-- 本地化 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-locale">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm-1 17.93A8 8 0 0 1 4.07 13H8a15 15 0 0 0 .59 3.78A8 8 0 0 1 11 19.93zM11 11H6.06A8 8 0 0 1 11 4.07zm2 0V4.07A8 8 0 0 1 17.94 11zm0 2h4.94A8 8 0 0 1 13 19.93z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.systemLocale') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.systemLocaleDesc') }}</div>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.timezone')" prop="timezone" required>
                <el-select v-model="model.timezone" class="full-width" :placeholder="$t('setting.selectTimezone')">
                  <el-option v-for="tz in timezoneOptions" :key="tz.value" :label="tz.label" :value="tz.value" />
                </el-select>
              </el-form-item>
              <el-form-item :label="$t('setting.language')" prop="language" required>
                <el-select v-model="model.language" class="full-width">
                  <el-option :label="$t('setting.langZhCN')" value="zh-CN" />
                  <el-option :label="$t('setting.langEnUS')" value="en-US" />
                </el-select>
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- 系统品牌 / 数据备份 面板已在 2026-05 清理中移除：
             - 系统品牌（systemTitle/logoUrl/themeColor）整体下线，前后端字段一并删除。
             - 数据备份相关配置改由「备份还原 → 自动备份」标签页统一管理，避免双入口编辑同一份 system_config 互相覆盖。 -->

        <!-- NTP 时间同步 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-locale">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8zm.5-13H11v6l5.2 3.2.8-1.3-4.5-2.7z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.ntpTitle') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.ntpDesc') }}</div>
            </div>
            <div class="gs-ntp-header-actions">
              <div class="gs-panel-badge" :class="ntpStatusClass">
                {{ ntpStatusLabel }}
              </div>
              <el-button
                size="small"
                type="primary"
                plain
                :loading="ntpTesting"
                :disabled="!model.ntpEnabled"
                @click="handleNtpTest"
              >
                {{ $t('setting.ntpTestNow') }}
              </el-button>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-switch-row">
              <div class="gs-switch-info">
                <div class="gs-switch-label">{{ $t('setting.ntpEnabled') }}</div>
                <div class="gs-switch-desc">{{ $t('setting.ntpEnabledDesc') }}</div>
              </div>
              <el-switch v-model="model.ntpEnabled" />
            </div>
            <el-form-item :label="$t('setting.ntpServers')" style="margin-top: 4px">
              <el-input
                v-model="model.ntpServers"
                type="textarea"
                :rows="3"
                :placeholder="$t('setting.ntpServersPlaceholder')"
                :disabled="!model.ntpEnabled"
              />
            </el-form-item>
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.ntpInterval')">
                <el-input-number
                  v-model="model.ntpCheckIntervalSec"
                  :min="60" :max="3600" :step="60"
                  controls-position="right" class="full-width"
                  :disabled="!model.ntpEnabled"
                  @change="(v: number) => { model.ntpCheckIntervalSec = clampInt(v, 60, 3600) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.ntpDrift')">
                <el-input :model-value="ntpDriftDisplay" disabled />
              </el-form-item>
            </div>
            <div v-if="ntpTestResult && ntpTestResult.success" class="gs-inline-success">
              {{ $t('setting.ntpTestOk', {
                server: ntpTestResult.server || '-',
                drift: ntpTestResult.driftMs ?? 0,
                at: ntpTestResult.checkAt || '-',
              }) }}
            </div>
            <div v-else-if="ntpTestResult && !ntpTestResult.success" class="gs-inline-error">
              {{ $t('setting.ntpTestFail', { error: ntpTestResult.error || '-' }) }}
            </div>
            <div v-else-if="model.ntpLastError" class="gs-inline-error">
              {{ $t('setting.ntpLastError', { error: model.ntpLastError }) }}
            </div>
          </div>
        </div>

        <!-- DB 连接池 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-backup">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 3c-4.97 0-9 1.34-9 3v12c0 1.66 4.03 3 9 3s9-1.34 9-3V6c0-1.66-4.03-3-9-3zm0 2c4.42 0 7 1.06 7 1s-2.58 1-7 1-7-1.06-7-1 2.58-1 7-1z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.dbPoolTitle') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.dbPoolDesc') }}</div>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row gs-fields-row-3">
              <el-form-item :label="$t('setting.dbMaxOpen')">
                <el-input-number
                  v-model="model.dbMaxOpenConns"
                  :min="1" :max="500" :step="5"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.dbMaxOpenConns = clampInt(v, 1, 500) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.dbMaxIdle')">
                <el-input-number
                  v-model="model.dbMaxIdleConns"
                  :min="0" :max="500" :step="5"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.dbMaxIdleConns = clampInt(v, 0, 500) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.dbConnLifetime')">
                <el-input-number
                  v-model="model.dbConnMaxLifetimeMin"
                  :min="1" :max="240" :step="1"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.dbConnMaxLifetimeMin = clampInt(v, 1, 240) }"
                />
              </el-form-item>
            </div>
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.dbConnIdle')">
                <el-input-number
                  v-model="model.dbConnMaxIdleMin"
                  :min="1" :max="120" :step="1"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.dbConnMaxIdleMin = clampInt(v, 1, 120) }"
                />
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- 维护模式 —— 从原「日志与维护」tab 搬过来。为什么在这？
             「从业务上」维护模式是个运行时开关，和时区、NTP、DB 连接池
             同属「系统运行参数」范畴；绑定在原 tab 里与「日志」同台反而导致
             tab 名不伦不类。globalQpsThreshold 是死字段 (后端从未读取)，
             真实限流走「安全中心 → DDoS 防护」下的 ddos_global.qps_limit。 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-maint" :class="{ 'is-warning': model.maintenanceEnabled }">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M22.7 19 13.6 9.9a5.89 5.89 0 0 0-6.5-7.8l3.6 3.6L8.1 8.3 4.5 4.7a5.89 5.89 0 0 0 7.8 6.5l9.1 9.1z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.systemMaint') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.systemMaintDesc') }}</div>
            </div>
            <div v-if="model.maintenanceEnabled" class="gs-panel-badge is-warning">{{ $t('setting.maintMode') }}</div>
          </div>
          <div class="gs-panel-body">
            <el-form-item :label="$t('setting.maintWindow')">
              <el-input v-model="model.maintenanceWindow" :placeholder="$t('setting.maintWindowPlaceholder')" :disabled="!model.maintenanceEnabled" />
            </el-form-item>
            <div class="gs-switch-row">
              <div class="gs-switch-info">
                <div class="gs-switch-label">{{ $t('setting.enableMaint') }}</div>
                <div class="gs-switch-desc">{{ $t('setting.enableMaintDesc') }}</div>
              </div>
              <el-switch v-model="model.maintenanceEnabled" />
            </div>
            <!-- Cross-page hint: prevent operators from hunting for QPS / log
                 settings under 「维护」. Each link points to the single source
                 of truth for the formerly-duplicated knob. -->
            <div class="gs-cross-link">
              {{ $t('setting.maintCrossLinkPrefix') }}
              <router-link to="/security/black-white?view=ddos">{{ $t('setting.maintCrossLinkDdos') }}</router-link>
              <span class="gs-cross-link-sep">·</span>
              <router-link to="/setting/audit-log">{{ $t('setting.maintCrossLinkAudit') }}</router-link>
            </div>
          </div>
        </div>
      </el-tab-pane>

      <!-- ══ Tab 2: DNS 策略 ══ -->
      <el-tab-pane name="dns">
        <template #label>
          <span class="gs-tab-label">
            <svg class="gs-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm-1 17.93A8 8 0 0 1 4.07 13H8a15 15 0 0 0 .59 3.78A8 8 0 0 1 11 19.93zM11 11H6.06A8 8 0 0 1 11 4.07zm2 0V4.07A8 8 0 0 1 17.94 11zm0 2h4.94A8 8 0 0 1 13 19.93z" /></svg>
            {{ $t('setting.dnsPolicy') }}
          </span>
        </template>

        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-dns">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.dnsGlobalPolicy') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.dnsGlobalPolicyDesc') }}</div>
            </div>
            <div class="gs-panel-badge" :class="model.dnssecGlobal ? 'is-active' : 'is-inactive'">
              DNSSEC {{ model.dnssecGlobal ? $t('setting.on') : $t('setting.off') }}
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.defaultTtl')" prop="defaultTtl" required>
                <el-input-number v-model="model.defaultTtl" :min="60" :max="86400" :step="60" controls-position="right" class="full-width" @change="(v: number) => { model.defaultTtl = clampInt(v, 60, 86400) }" />
              </el-form-item>
              <el-form-item :label="$t('setting.negativeCacheTtl')" prop="negativeCacheTtl" required>
                <el-input-number v-model="model.negativeCacheTtl" :min="0" :max="3600" :step="30" controls-position="right" class="full-width" @change="(v: number) => { model.negativeCacheTtl = clampInt(v, 0, 3600) }" />
              </el-form-item>
            </div>
            <div class="gs-switch-grid">
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.dnssecGlobal') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.dnssecGlobalDesc') }}</div>
                </div>
                <el-switch v-model="model.dnssecGlobal" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.ecsEnabled') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.ecsEnabledDesc') }}</div>
                </div>
                <el-switch v-model="model.ecsEnabled" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.dohEnabled') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.dohEnabledDesc') }}</div>
                </div>
                <el-switch v-model="model.dohEnabled" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.dotEnabled') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.dotEnabledDesc') }}</div>
                </div>
                <el-switch v-model="model.dotEnabled" />
              </div>
            </div>
          </div>
        </div>

        <!-- TTL 钳制 + 上游协议偏好 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-dns">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M11 7h2v6h-2zm0 8h2v2h-2zm1-13C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.ttlClampTitle') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.ttlClampDesc') }}</div>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.minTtl')">
                <el-input-number
                  v-model="model.minTtl"
                  :min="0" :max="86400" :step="30"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.minTtl = clampInt(v, 0, 86400) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.maxTtl')">
                <el-input-number
                  v-model="model.maxTtl"
                  :min="0" :max="604800" :step="60"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.maxTtl = clampInt(v, 0, 604800) }"
                />
              </el-form-item>
            </div>
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.upstreamOrder')">
                <el-select v-model="model.upstreamProtocolOrder" class="full-width">
                  <el-option label="UDP → TCP" value="udp,tcp" />
                  <el-option label="TCP → UDP" value="tcp,udp" />
                  <el-option label="DoT → UDP → TCP" value="dot,udp,tcp" />
                  <el-option label="DoH → DoT → UDP" value="doh,dot,udp" />
                  <el-option label="DoH → UDP → TCP" value="doh,udp,tcp" />
                </el-select>
              </el-form-item>
              <el-form-item :label="$t('setting.upstreamTimeout')">
                <el-input-number
                  v-model="model.upstreamTimeoutMs"
                  :min="200" :max="30000" :step="100"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.upstreamTimeoutMs = clampInt(v, 200, 30000) }"
                />
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- ECS prefix + RFC 8467 padding -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-dns">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm-1 16.93A8 8 0 0 1 4.07 13H11zm0-7.93H4.07A8 8 0 0 1 11 5.07zm2-3.93A8 8 0 0 1 19.93 11H13zm0 11.93V13h6.93A8 8 0 0 1 13 18.93z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.ecsPaddingTitle') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.ecsPaddingDesc') }}</div>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.ecsPrefixV4')">
                <el-input-number
                  v-model="model.ecsPrefixV4"
                  :min="0" :max="32" :step="1"
                  controls-position="right" class="full-width"
                  :disabled="!model.ecsEnabled"
                  @change="(v: number) => { model.ecsPrefixV4 = clampInt(v, 0, 32) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.ecsPrefixV6')">
                <el-input-number
                  v-model="model.ecsPrefixV6"
                  :min="0" :max="128" :step="1"
                  controls-position="right" class="full-width"
                  :disabled="!model.ecsEnabled"
                  @change="(v: number) => { model.ecsPrefixV6 = clampInt(v, 0, 128) }"
                />
              </el-form-item>
            </div>
            <div class="gs-switch-row">
              <div class="gs-switch-info">
                <div class="gs-switch-label">{{ $t('setting.dnsPaddingEnabled') }}</div>
                <div class="gs-switch-desc">{{ $t('setting.dnsPaddingDesc') }}</div>
              </div>
              <el-switch v-model="model.dnsPaddingEnabled" />
            </div>
            <div class="gs-fields-row">
              <el-form-item :label="$t('setting.dnsPaddingBlock')">
                <el-input-number
                  v-model="model.dnsPaddingBlock"
                  :min="32" :max="1024" :step="32"
                  controls-position="right" class="full-width"
                  :disabled="!model.dnsPaddingEnabled"
                  @change="(v: number) => { model.dnsPaddingBlock = clampInt(v, 32, 1024) }"
                />
              </el-form-item>
            </div>
            <div class="gs-switch-row">
              <div class="gs-switch-info">
                <div class="gs-switch-label">{{ $t('setting.dohPreferGet') }}</div>
                <div class="gs-switch-desc">{{ $t('setting.dohPreferGetDesc') }}</div>
              </div>
              <el-switch v-model="model.dohPreferGet" :disabled="!model.dohEnabled" />
            </div>
          </div>
        </div>
      </el-tab-pane>

      <!-- ══ Tab 3: 安全 ══ -->
      <el-tab-pane name="security">
        <template #label>
          <span class="gs-tab-label">
            <svg class="gs-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5z" /></svg>
            {{ $t('setting.securityTab') }}
          </span>
        </template>

        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-security">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M18 8h-1V6a5 5 0 0 0-10 0v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zm-6 9a2 2 0 1 1 2-2 2 2 0 0 1-2 2zm3.1-9H8.9V6a3.1 3.1 0 0 1 6.2 0z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.securityAccess') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.securityAccessDesc') }}</div>
            </div>
            <div class="gs-panel-badge" :class="model.mfaRequired ? 'is-active' : 'is-inactive'">
              {{ model.mfaRequired ? $t('setting.mfaLabel') + ' ' + $t('setting.on') : $t('setting.mfaLabel') + ' ' + $t('setting.off') }}
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row gs-fields-row-3">
              <el-form-item :label="$t('setting.sessionTimeout')" prop="loginTimeoutMinutes" required>
                <el-input-number v-model="model.loginTimeoutMinutes" :min="5" :max="1440" :step="5" controls-position="right" class="full-width" @change="(v: number) => { model.loginTimeoutMinutes = clampInt(v, 5, 1440) }" />
              </el-form-item>
              <el-form-item :label="$t('setting.maxLoginFailures')" prop="loginMaxFailures" required>
                <el-input-number v-model="model.loginMaxFailures" :min="1" :max="20" :step="1" controls-position="right" class="full-width" @change="(v: number) => { model.loginMaxFailures = clampInt(v, 1, 20) }" />
              </el-form-item>
              <el-form-item :label="$t('setting.lockDuration')" prop="loginLockMinutes" required>
                <el-input-number v-model="model.loginLockMinutes" :min="1" :max="1440" :step="5" controls-position="right" class="full-width" @change="(v: number) => { model.loginLockMinutes = clampInt(v, 1, 1440) }" />
              </el-form-item>
            </div>
            <div class="gs-switch-row">
              <div class="gs-switch-info">
                <div class="gs-switch-label">{{ $t('setting.mfaRequired') }}</div>
                <div class="gs-switch-desc">{{ $t('setting.mfaRequiredDesc') }}</div>
              </div>
              <el-switch v-model="model.mfaRequired" :before-change="confirmMfaToggle" />
            </div>
            <el-form-item :label="$t('setting.ipWhitelist')" style="margin-top: 4px">
              <el-input v-model="model.ipWhitelist" type="textarea" :rows="3" :placeholder="$t('setting.ipWhitelistPlaceholder')" />
            </el-form-item>
          </div>
        </div>

        <!-- 密码策略 -->
        <div class="gs-panel">
          <div class="gs-panel-header">
            <div class="gs-section-icon gs-section-icon-security">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 1 3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5z" /></svg>
            </div>
            <div class="gs-panel-title-block">
              <div class="gs-panel-title">{{ $t('setting.pwdPolicyTitle') }}</div>
              <div class="gs-panel-desc">{{ $t('setting.pwdPolicyDesc') }}</div>
            </div>
          </div>
          <div class="gs-panel-body">
            <div class="gs-fields-row gs-fields-row-3">
              <el-form-item :label="$t('setting.pwdMinLength')">
                <el-input-number
                  v-model="model.pwdMinLength"
                  :min="8" :max="64" :step="1"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.pwdMinLength = clampInt(v, 8, 64) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.pwdExpireDays')">
                <el-input-number
                  v-model="model.pwdExpireDays"
                  :min="0" :max="365" :step="30"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.pwdExpireDays = clampInt(v, 0, 365) }"
                />
              </el-form-item>
              <el-form-item :label="$t('setting.maxConcurrentLogin')">
                <el-input-number
                  v-model="model.maxConcurrentLogin"
                  :min="0" :max="20" :step="1"
                  controls-position="right" class="full-width"
                  @change="(v: number) => { model.maxConcurrentLogin = clampInt(v, 0, 20) }"
                />
              </el-form-item>
            </div>
            <div class="gs-switch-grid">
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.pwdRequireUpper') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.pwdRequireUpperDesc') }}</div>
                </div>
                <el-switch v-model="model.pwdRequireUpper" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.pwdRequireLower') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.pwdRequireLowerDesc') }}</div>
                </div>
                <el-switch v-model="model.pwdRequireLower" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.pwdRequireDigit') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.pwdRequireDigitDesc') }}</div>
                </div>
                <el-switch v-model="model.pwdRequireDigit" />
              </div>
              <div class="gs-switch-row gs-switch-compact">
                <div class="gs-switch-info">
                  <div class="gs-switch-label">{{ $t('setting.pwdRequireSymbol') }}</div>
                  <div class="gs-switch-desc">{{ $t('setting.pwdRequireSymbolDesc') }}</div>
                </div>
                <el-switch v-model="model.pwdRequireSymbol" />
              </div>
            </div>
          </div>
        </div>
      </el-tab-pane>

      <!-- ══ Tab 4: 日志与维护 ══ -->
      <!-- 2026-05 cleanup: the 「日志与维护」 tab was removed.
           ─ Log fields (level / retention / export format / syslog) are now
             edited exclusively from the 「审计日志」 page — having two
             entry points caused last-writer-wins overwrites of the same
             system_config row.
           ─ Maintenance mode + window moved to the 「系统」 tab above
             (it's an operational toggle alongside timezone / NTP / DB
             pool, not a 「log」 setting).
           ─ globalQpsThreshold was unread by any backend code path; the
             real limiter reads ddos_global.qps_limit owned by
             「安全中心 → DDoS 防护」. -->
    </el-tabs>

    <!-- Footer — persistent across all tabs -->
    <div class="gs-footer">
      <span class="gs-footer-hint">{{ $t('setting.saveTip') }}</span>
      <div class="gs-footer-actions">
        <el-button :disabled="loading" @click="emit('reset')">{{ $t('setting.resetDefault') }}</el-button>
        <el-button type="primary" :loading="loading" :disabled="loading" @click="emitUpdate">{{ $t('setting.saveConfig') }}</el-button>
      </div>
    </div>

  </el-form>
</template>

<style scoped>
/* ═══════════════ Status strip ═══════════════ */
.gs-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}

.gs-stat {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow 0.2s, transform 0.2s;
}

.gs-stat:hover {
  box-shadow: 0 6px 18px rgba(22, 93, 255, 0.12);
  transform: translateY(-1px);
}

.gs-stat-icon {
  width: 38px;
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}

.gs-stat-icon-dnssec { background: var(--app-accent-soft); color: var(--app-accent); }
.gs-stat-icon-backup { background: rgba(114, 46, 209, 0.1); color: #722ED1; }
.gs-stat-icon-mfa { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }
.gs-stat-icon-maint { background: rgba(134, 144, 156, 0.12); color: var(--app-disabled); }
.gs-stat-icon-maint.is-warning { background: rgba(255, 125, 0, 0.12); color: var(--app-warning); }

.gs-stat-body {
  flex: 1;
  min-width: 0;
}

.gs-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-bottom: 2px;
}

.gs-stat-value {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
}

.gs-stat-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.gs-stat-dot.is-on {
  background: var(--app-success);
  box-shadow: 0 0 0 3px rgba(0, 180, 42, 0.15);
}

.gs-stat-dot.is-off {
  background: var(--app-disabled);
}

.gs-stat-dot.is-warn {
  background: var(--app-warning);
  box-shadow: 0 0 0 3px rgba(255, 125, 0, 0.18);
}

/* ═══════════════ Tabs ═══════════════ */
.gs-tabs {
  --el-tabs-header-height: 48px;
}

.gs-tabs :deep(.el-tabs__header) {
  margin: 0 0 20px;
  border-bottom: 1px solid var(--app-border);
}

.gs-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.gs-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  height: 48px;
  line-height: 48px;
  padding: 0 20px !important;
}

.gs-tabs :deep(.el-tabs__content) {
  padding: 0;
}

.gs-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
}

.gs-tab-icon {
  flex-shrink: 0;
}

/* ═══════════════ Panel (section card) ═══════════════ */
.gs-panel {
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 16px;
}

.gs-panel:last-child {
  margin-bottom: 0;
}

.gs-panel-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border);
  background: linear-gradient(180deg, #FAFBFC 0%, #FFFFFF 100%);
}

.gs-section-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  flex-shrink: 0;
}

.gs-section-icon-locale { background: var(--app-accent-soft); color: var(--app-accent); }
.gs-section-icon-brand { background: rgba(255, 125, 0, 0.1); color: var(--app-warning); }
.gs-section-icon-backup { background: rgba(114, 46, 209, 0.1); color: #722ED1; }
.gs-section-icon-dns { background: var(--app-accent-soft); color: var(--app-accent); }
.gs-section-icon-security { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }
.gs-section-icon-log { background: rgba(20, 201, 201, 0.1); color: #14C9C9; }
.gs-section-icon-maint { background: rgba(134, 144, 156, 0.12); color: var(--app-disabled); }
.gs-section-icon-maint.is-warning { background: rgba(255, 125, 0, 0.12); color: var(--app-warning); }

.gs-panel-title-block {
  flex: 1;
  min-width: 0;
}

.gs-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  line-height: 1.4;
}

.gs-panel-desc {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
}

.gs-panel-badge {
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  flex-shrink: 0;
  border: 1px solid transparent;
}

.gs-panel-badge.is-active {
  background: rgba(0, 180, 42, 0.1);
  color: var(--app-success);
  border-color: rgba(0, 180, 42, 0.25);
}

.gs-panel-badge.is-inactive {
  background: rgba(134, 144, 156, 0.08);
  color: var(--app-disabled);
  border-color: var(--app-border);
}

.gs-panel-badge.is-warning {
  background: rgba(255, 125, 0, 0.1);
  color: var(--app-warning);
  border-color: rgba(255, 125, 0, 0.25);
}

.gs-panel-body {
  padding: 18px 20px;
}

/* ═══════════════ Form rows ═══════════════ */
.gs-fields-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 20px;
  transition: opacity 0.2s;
}

.gs-fields-row-3 {
  grid-template-columns: 1fr 1fr 1fr;
}

.gs-fields-row.is-disabled-section {
  opacity: 0.5;
  pointer-events: none;
}

/* ═══════════════ Switch rows ═══════════════ */
.gs-switch-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 8px;
}

.gs-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  background: #FAFBFC;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  margin-bottom: 12px;
  gap: 16px;
  transition: border-color 0.2s, background 0.2s;
}

.gs-switch-row:hover {
  border-color: var(--app-accent-muted);
  background: #FBFDFF;
}

.gs-switch-compact {
  margin-bottom: 0;
}

.gs-switch-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-title);
}

.gs-switch-desc {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 3px;
  line-height: 1.4;
}

/* ═══════════════ Footer ═══════════════ */
.gs-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
  margin-top: 20px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  flex-wrap: wrap;
}

.gs-footer-hint {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  color: var(--app-text-regular);
}

.gs-footer-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

/* ═══════════════ Inputs / Upload ═══════════════ */
.full-width {
  width: 100%;
}

.color-picker-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.color-input {
  flex: 1;
}

.logo-uploader {
  width: 100%;
}

.logo-uploader :deep(.el-upload) {
  width: 100%;
  height: 40px;
  border: 1px dashed var(--app-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}

.logo-uploader :deep(.el-upload:hover) {
  border-color: var(--app-accent);
  background: var(--app-accent-soft);
}

.logo-preview {
  height: 32px;
  max-width: 120px;
  object-fit: contain;
}

.logo-placeholder {
  font-size: 12px;
  color: var(--app-disabled);
}

/* Inline error strip for sub-panels (e.g. NTP last-error). Sized for a
   single-line message; multi-line errors wrap naturally. */
.gs-inline-error {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  background: rgba(255, 77, 79, 0.08);
  color: #cf1322;
  border: 1px solid rgba(255, 77, 79, 0.3);
  font-size: 12px;
  line-height: 1.5;
}

/* Inline success strip — same shape as gs-inline-error, green palette.
   Used for the "立即测试" button's success result. */
.gs-inline-success {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  background: rgba(82, 196, 26, 0.08);
  color: #389e0d;
  border: 1px solid rgba(82, 196, 26, 0.3);
  font-size: 12px;
  line-height: 1.5;
}

/* Header-side cluster of badge + button, right-aligned, gap matches
   the panel-header padding so the row visually balances with the
   icon + title block on the left. */
.gs-ntp-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

/* Cross-page navigation hint shown at the bottom of the maintenance
   panel. Soft-tone, secondary text — operators looking for the old
   QPS / log fields land here and immediately see the right page. */
.gs-cross-link {
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--app-bg-secondary, #f5f7fa);
  font-size: 12px;
  color: var(--app-text-secondary, #909399);
  line-height: 1.5;
}

.gs-cross-link a {
  color: var(--el-color-primary);
  text-decoration: none;
}

.gs-cross-link a:hover {
  text-decoration: underline;
}

.gs-cross-link-sep {
  margin: 0 6px;
  color: var(--app-disabled, #c0c4cc);
}

/* legacy divider no longer used between gs-panel blocks */
.gs-divider {
  display: none;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .gs-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .gs-fields-row-3 {
    grid-template-columns: 1fr 1fr;
  }

  .gs-switch-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .gs-stats {
    grid-template-columns: 1fr;
  }

  .gs-fields-row,
  .gs-fields-row-3 {
    grid-template-columns: 1fr;
  }

  .gs-footer {
    flex-direction: column;
    align-items: flex-start;
  }

  .gs-footer-actions {
    margin-left: 0;
    width: 100%;
    justify-content: flex-end;
  }
}
</style>