<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import EmailTab from '../../components/notification/EmailTab.vue'
import SmsTab from '../../components/notification/SmsTab.vue'
import WebhookTab from '../../components/notification/WebhookTab.vue'
import ChatBotTab from '../../components/notification/ChatBotTab.vue'
import VoiceTab from '../../components/notification/VoiceTab.vue'
import NoticeTemplateDrawer from '../../components/notification/NoticeTemplateDrawer.vue'
import {
  getNotificationConfigApi,
  testNotificationChannelApi,
  updateNotificationChannelApi,
} from '../../api/notification'
import type { NotificationChannel, NotificationConfig, NotificationTabExpose } from '../../types/notification'
import { NOTIFICATION_CHANNELS, createDefaultNotificationConfig } from '../../types/notification'

const { t } = useI18n()
const activeChannel = ref<NotificationChannel>('email')
const pageLoading = ref(false)
const savingChannel = ref<NotificationChannel | null>(null)
const testingChannel = ref<NotificationChannel | null>(null)
const notificationForm = reactive<NotificationConfig>(createDefaultNotificationConfig())

const emailTabRef = ref<NotificationTabExpose>()
const webhookTabRef = ref<NotificationTabExpose>()
const smsTabRef = ref<NotificationTabExpose>()
const dingtalkTabRef = ref<NotificationTabExpose>()
const feishuTabRef = ref<NotificationTabExpose>()
const wecomTabRef = ref<NotificationTabExpose>()
const slackTabRef = ref<NotificationTabExpose>()
const voiceTabRef = ref<NotificationTabExpose>()

const channelTitleMap = computed<Record<NotificationChannel, string>>(() => ({
  email: t('notice.channelEmail'),
  webhook: t('notice.webhook'),
  sms: t('notice.channelSms'),
  dingtalk: t('notice.dingtalk.tabLabel'),
  feishu: t('notice.feishu.tabLabel'),
  wecom: t('notice.wecom.tabLabel'),
  slack: t('notice.slack.tabLabel'),
  voice: t('notice.voice.tabLabel'),
}))

const channelRefMap: Record<NotificationChannel, typeof emailTabRef> = {
  email: emailTabRef,
  webhook: webhookTabRef,
  sms: smsTabRef,
  dingtalk: dingtalkTabRef,
  feishu: feishuTabRef,
  wecom: wecomTabRef,
  slack: slackTabRef,
  voice: voiceTabRef,
}

// assignConfig copies the server-side payload back into the reactive
// form. Iterating the channel list rather than naming fields means a
// future channel only needs an entry in NOTIFICATION_CHANNELS plus the
// matching tab — no risk of forgetting the assign step.
const assignConfig = (payload: NotificationConfig): void => {
  for (const ch of NOTIFICATION_CHANNELS) {
    Object.assign(notificationForm[ch] as object, payload[ch] as object)
  }
}

const getChannelDisabled = (channel: NotificationChannel): boolean => !notificationForm[channel].enabled

const validateChannel = async (channel: NotificationChannel): Promise<boolean> => {
  if (getChannelDisabled(channel)) {
    return true
  }
  return channelRefMap[channel].value?.validate() ?? false
}

const loadNotificationConfig = async (): Promise<void> => {
  pageLoading.value = true
  try {
    const { data } = await getNotificationConfigApi()
    assignConfig(data)
  } catch (_error) {
    ElMessage.error(t('notice.configLoadFailed'))
  } finally {
    pageLoading.value = false
  }
}

const saveChannel = async (channel: NotificationChannel): Promise<void> => {
  if (savingChannel.value || testingChannel.value) {
    return
  }
  const valid = await validateChannel(channel)
  if (!valid) {
    return
  }
  savingChannel.value = channel
  try {
    const { data } = await updateNotificationChannelApi(channel, notificationForm[channel])
    assignConfig(data)
    ElMessage.success(t('notice.configUpdated', { channel: channelTitleMap.value[channel] }))
  } catch (_error) {
    ElMessage.error(t('notice.configUpdateFailed', { channel: channelTitleMap.value[channel] }))
  } finally {
    savingChannel.value = null
  }
}

const testChannel = async (channel: NotificationChannel): Promise<void> => {
  if (testingChannel.value || savingChannel.value || getChannelDisabled(channel)) {
    return
  }
  const valid = await validateChannel(channel)
  if (!valid) {
    return
  }
  testingChannel.value = channel
  try {
    const { data } = await testNotificationChannelApi(channel, notificationForm[channel])
    ElMessage.success(data.message)
  } catch (_error) {
    ElMessage.error(t('notice.testFailed', { channel: channelTitleMap.value[channel] }))
  } finally {
    testingChannel.value = null
  }
}

// enabledCount tallies every channel that has its `enabled` switch on.
// Iterating NOTIFICATION_CHANNELS (the canonical list) keeps the count
// honest as channels are added — earlier this was a hard-coded
// ['email','webhook','sms'] which would have under-reported once the
// chat-platform tabs landed.
const enabledCount = computed(() =>
  NOTIFICATION_CHANNELS.filter((ch) => notificationForm[ch].enabled).length,
)
const totalChannels = NOTIFICATION_CHANNELS.length

// Template editor drawer. Lives next to the channel-config tabs but
// has its own opening trigger in the page header so operators don't
// confuse "edit channel credentials" with "edit message template".
const templateDrawerOpen = ref(false)
const openTemplateDrawer = () => {
  templateDrawerOpen.value = true
}

onMounted(() => {
  loadNotificationConfig()
})
</script>

<template>
  <div class="notification-page page-shell" v-loading="pageLoading">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('notice.title') }}</h1>
        <p class="page-subtitle">{{ $t('notice.subtitle') }}</p>
      </div>
      <div class="page-header-actions">
        <el-button type="primary" plain @click="openTemplateDrawer">
          <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14" style="margin-right:6px"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6zm2 16H8v-2h8v2zm0-4H8v-2h8v2zm-3-5V3.5L18.5 9H13z" /></svg>
          {{ $t('noticeTemplate.openButton') }}
        </el-button>
      </div>
    </div>

    <!-- ═══ Stats strip ═══ -->
    <div class="nt-stats">
      <div class="nt-stat">
        <div class="nt-stat-icon nt-stat-icon-email">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 4-8 5-8-5V6l8 5 8-5v2z" /></svg>
        </div>
        <div class="nt-stat-body">
          <div class="nt-stat-label">{{ $t('notice.emailAlert') }}</div>
          <div class="nt-stat-value">
            <span class="nt-dot" :class="notificationForm.email.enabled ? 'is-on' : 'is-off'" />
            {{ notificationForm.email.enabled ? $t('notice.enabled') : $t('notice.disabled') }}
          </div>
        </div>
      </div>
      <div class="nt-stat">
        <div class="nt-stat-icon nt-stat-icon-webhook">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M13.5 2c-5.62 0-10.19 4.27-10.48 9.82L1 9.8v4.49l5.26-3.6-5.26-3.64v2.52C1.26 6.88 6.89 2.75 13.5 2.75c5.52 0 10.16 3.57 11.89 8.6l.85-.61A12.73 12.73 0 0 0 13.5 2zM22 9.71l-5.26 3.6 5.26 3.64v-2.52C21.74 17.12 16.11 21.25 9.5 21.25c-5.52 0-10.16-3.57-11.89-8.6l-.85.61A12.73 12.73 0 0 0 9.5 22c5.62 0 10.19-4.27 10.48-9.82L22 14.2V9.71z" /></svg>
        </div>
        <div class="nt-stat-body">
          <div class="nt-stat-label">{{ $t('notice.webhook') }}</div>
          <div class="nt-stat-value">
            <span class="nt-dot" :class="notificationForm.webhook.enabled ? 'is-on' : 'is-off'" />
            {{ notificationForm.webhook.enabled ? $t('notice.enabled') : $t('notice.disabled') }}
          </div>
        </div>
      </div>
      <div class="nt-stat">
        <div class="nt-stat-icon nt-stat-icon-sms">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
        </div>
        <div class="nt-stat-body">
          <div class="nt-stat-label">{{ $t('notice.smsAlert') }}</div>
          <div class="nt-stat-value">
            <span class="nt-dot" :class="notificationForm.sms.enabled ? 'is-on' : 'is-off'" />
            {{ notificationForm.sms.enabled ? $t('notice.enabled') : $t('notice.disabled') }}
          </div>
        </div>
      </div>
      <div class="nt-stat">
        <div class="nt-stat-icon nt-stat-icon-active">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M12 22a2 2 0 0 0 2-2h-4a2 2 0 0 0 2 2zm6-6V11c0-3.07-1.64-5.64-4.5-6.32V4a1.5 1.5 0 0 0-3 0v.68C7.63 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z" /></svg>
        </div>
        <div class="nt-stat-body">
          <div class="nt-stat-label">{{ $t('notice.enabledChannels') }}</div>
          <div class="nt-stat-value nt-stat-value-num">{{ enabledCount }} <span class="nt-stat-value-unit">/ {{ totalChannels }}</span></div>
        </div>
      </div>
    </div>

    <!-- ═══ Tabs card ═══ -->
    <el-card class="nt-main-card" shadow="never">
      <el-tabs v-model="activeChannel" class="nt-tabs">

        <el-tab-pane name="email">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 4-8 5-8-5V6l8 5 8-5v2z" /></svg>
              {{ $t('notice.emailAlert') }}
              <span class="nt-tab-dot" :class="notificationForm.email.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <EmailTab
            ref="emailTabRef"
            :model="notificationForm.email"
            :loading="savingChannel === 'email' || testingChannel === 'email'"
            :disabled="!notificationForm.email.enabled"
            @test="testChannel('email')"
            @save="saveChannel('email')"
          />
        </el-tab-pane>

        <el-tab-pane name="webhook">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M13.5 2c-5.62 0-10.19 4.27-10.48 9.82L1 9.8v4.49l5.26-3.6-5.26-3.64v2.52C1.26 6.88 6.89 2.75 13.5 2.75c5.52 0 10.16 3.57 11.89 8.6l.85-.61A12.73 12.73 0 0 0 13.5 2zM22 9.71l-5.26 3.6 5.26 3.64v-2.52C21.74 17.12 16.11 21.25 9.5 21.25c-5.52 0-10.16-3.57-11.89-8.6l-.85.61A12.73 12.73 0 0 0 9.5 22c5.62 0 10.19-4.27 10.48-9.82L22 14.2V9.71z" /></svg>
              {{ $t('notice.webhook') }}
              <span class="nt-tab-dot" :class="notificationForm.webhook.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <WebhookTab
            ref="webhookTabRef"
            :model="notificationForm.webhook"
            :loading="savingChannel === 'webhook' || testingChannel === 'webhook'"
            :disabled="!notificationForm.webhook.enabled"
            @test="testChannel('webhook')"
            @save="saveChannel('webhook')"
          />
        </el-tab-pane>

        <el-tab-pane name="sms">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
              {{ $t('notice.smsAlert') }}
              <span class="nt-tab-dot" :class="notificationForm.sms.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <SmsTab
            ref="smsTabRef"
            :model="notificationForm.sms"
            :loading="savingChannel === 'sms' || testingChannel === 'sms'"
            :disabled="!notificationForm.sms.enabled"
            @test="testChannel('sms')"
            @save="saveChannel('sms')"
          />
        </el-tab-pane>

        <!-- DingTalk / Feishu / WeCom / Slack share ChatBotTab through
             the `platform` prop. Each tab still owns its own ref so
             form-level validation triggers in isolation. -->
        <el-tab-pane name="dingtalk">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
              {{ $t('notice.dingtalk.tabLabel') }}
              <span class="nt-tab-dot" :class="notificationForm.dingtalk.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <ChatBotTab
            ref="dingtalkTabRef"
            platform="dingtalk"
            :model="notificationForm.dingtalk"
            :loading="savingChannel === 'dingtalk' || testingChannel === 'dingtalk'"
            :disabled="!notificationForm.dingtalk.enabled"
            @test="testChannel('dingtalk')"
            @save="saveChannel('dingtalk')"
          />
        </el-tab-pane>

        <el-tab-pane name="feishu">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
              {{ $t('notice.feishu.tabLabel') }}
              <span class="nt-tab-dot" :class="notificationForm.feishu.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <ChatBotTab
            ref="feishuTabRef"
            platform="feishu"
            :model="notificationForm.feishu"
            :loading="savingChannel === 'feishu' || testingChannel === 'feishu'"
            :disabled="!notificationForm.feishu.enabled"
            @test="testChannel('feishu')"
            @save="saveChannel('feishu')"
          />
        </el-tab-pane>

        <el-tab-pane name="wecom">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
              {{ $t('notice.wecom.tabLabel') }}
              <span class="nt-tab-dot" :class="notificationForm.wecom.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <ChatBotTab
            ref="wecomTabRef"
            platform="wecom"
            :model="notificationForm.wecom"
            :loading="savingChannel === 'wecom' || testingChannel === 'wecom'"
            :disabled="!notificationForm.wecom.enabled"
            @test="testChannel('wecom')"
            @save="saveChannel('wecom')"
          />
        </el-tab-pane>

        <el-tab-pane name="slack">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
              {{ $t('notice.slack.tabLabel') }}
              <span class="nt-tab-dot" :class="notificationForm.slack.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <ChatBotTab
            ref="slackTabRef"
            platform="slack"
            :model="notificationForm.slack"
            :loading="savingChannel === 'slack' || testingChannel === 'slack'"
            :disabled="!notificationForm.slack.enabled"
            @test="testChannel('slack')"
            @save="saveChannel('slack')"
          />
        </el-tab-pane>

        <el-tab-pane name="voice">
          <template #label>
            <span class="nt-tab-label">
              <svg class="nt-tab-icon" viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M6.62 10.79a15.05 15.05 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.05-.24c1.12.37 2.33.57 3.57.57.55 0 1 .45 1 1V20a1 1 0 0 1-1 1A17 17 0 0 1 3 4a1 1 0 0 1 1-1h3.5c.55 0 1 .45 1 1 0 1.25.2 2.45.57 3.57.11.35.03.74-.25 1.02l-2.2 2.2z"/></svg>
              {{ $t('notice.voice.tabLabel') }}
              <span class="nt-tab-dot" :class="notificationForm.voice.enabled ? 'is-on' : 'is-off'" />
            </span>
          </template>
          <VoiceTab
            ref="voiceTabRef"
            :model="notificationForm.voice"
            :loading="savingChannel === 'voice' || testingChannel === 'voice'"
            :disabled="!notificationForm.voice.enabled"
            @test="testChannel('voice')"
            @save="saveChannel('voice')"
          />
        </el-tab-pane>

      </el-tabs>
    </el-card>

    <NoticeTemplateDrawer v-model="templateDrawerOpen" />
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.notification-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.notification-page :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Stats strip ═══════════════ */
.nt-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.nt-stat {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 20px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow 0.2s, transform 0.2s;
}

.nt-stat:hover {
  box-shadow: 0 6px 18px rgba(22, 93, 255, 0.12);
  transform: translateY(-1px);
}

.nt-stat-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}

.nt-stat-icon-email   { background: var(--app-accent-soft); color: var(--app-accent); }
.nt-stat-icon-webhook { background: rgba(114, 46, 209, 0.1); color: #722ED1; }
.nt-stat-icon-sms     { background: rgba(255, 125, 0, 0.1); color: var(--app-warning); }
.nt-stat-icon-active  { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }

.nt-stat-body {
  flex: 1;
  min-width: 0;
}

.nt-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-bottom: 4px;
}

.nt-stat-value {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
}

.nt-stat-value-num {
  font-size: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.nt-stat-value-unit {
  font-size: 13px;
  font-weight: 400;
  color: var(--app-text-regular);
}

.nt-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.nt-dot.is-on {
  background: var(--app-success);
  box-shadow: 0 0 0 3px rgba(0, 180, 42, 0.18);
}

.nt-dot.is-off {
  background: var(--app-disabled);
}

/* ═══════════════ Main card & tabs ═══════════════ */
.nt-main-card {
  overflow: hidden;
}

.nt-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 20px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.nt-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.nt-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  height: 48px;
  line-height: 48px;
  padding: 0 20px !important;
}

.nt-tabs :deep(.el-tabs__content) {
  padding: 24px;
}

.nt-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
}

.nt-tab-icon {
  flex-shrink: 0;
}

.nt-tab-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.nt-tab-dot.is-on {
  background: var(--app-success);
  box-shadow: 0 0 0 2px rgba(0, 180, 42, 0.2);
}

.nt-tab-dot.is-off {
  background: var(--app-disabled);
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .nt-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .nt-stats {
    grid-template-columns: 1fr;
  }

  .nt-tabs :deep(.el-tabs__item) {
    padding: 0 14px !important;
  }

  .nt-tabs :deep(.el-tabs__content) {
    padding: 16px;
  }
}
</style>