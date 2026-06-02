<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import type {
  ChatBotBase,
  NotificationDingTalkConfig,
  NotificationFeishuConfig,
  NotificationSlackConfig,
  NotificationTabExpose,
  NotificationWecomConfig,
} from '../../types/notification'
import { notificationAlertTypeOptions } from '../../types/notification'
import { validateFormAndFocus } from '../../utils/interaction'

// ChatBotTab is the shared editor for the four chat-platform channels
// (DingTalk / Feishu / WeCom / Slack). The four share ~70% of their UI
// surface — bot URL, sign secret, alert-type filter, severity floor —
// so collapsing them into one parameterised component avoids 1000 LOC
// of near-duplicate markup. Per-platform extras (DingTalk @-mobiles,
// Feishu interactive-card switch, Slack channel override) are gated on
// the `platform` prop.
//
// Adding a new chat platform: add a case to the platformMeta map below,
// extend NotificationConfig with its config interface, and wire a new
// el-tab-pane in NotificationSettings.vue. The transport layer in
// pkg/notify already exposes a generic chatbotRequest helper.

type ChatBotPlatform = 'dingtalk' | 'feishu' | 'wecom' | 'slack'

type PlatformModel =
  | NotificationDingTalkConfig
  | NotificationFeishuConfig
  | NotificationWecomConfig
  | NotificationSlackConfig

const props = defineProps<{
  platform: ChatBotPlatform
  model: PlatformModel
  loading: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  test: []
  save: []
}>()
const { t } = useI18n()

const formRef = ref<FormInstance>()
const channelEnabled = computed({
  get: () => props.model.enabled,
  set: (value: boolean) => {
    props.model.enabled = value
  },
})

// Each platform has a distinctive icon/colour and a panel title key in
// the i18n bundle. Centralising the metadata here keeps the template
// uncluttered and lets the tab labels in NotificationSettings.vue line
// up visually.
const platformMeta = computed(() => {
  switch (props.platform) {
    case 'dingtalk':
      return {
        iconClass: 'channel-icon-dingtalk',
        iconBg: 'rgba(64, 158, 255, 0.1)',
        iconColor: '#0089FF',
        titleKey: 'notice.dingtalk.panelTitle',
        descKey: 'notice.dingtalk.panelDesc',
        urlPlaceholder: 'https://oapi.dingtalk.com/robot/send?access_token=...',
        testButtonKey: 'notice.dingtalk.testButton',
      }
    case 'feishu':
      return {
        iconClass: 'channel-icon-feishu',
        iconBg: 'rgba(0, 184, 217, 0.1)',
        iconColor: '#00B8D9',
        titleKey: 'notice.feishu.panelTitle',
        descKey: 'notice.feishu.panelDesc',
        urlPlaceholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...',
        testButtonKey: 'notice.feishu.testButton',
      }
    case 'wecom':
      return {
        iconClass: 'channel-icon-wecom',
        iconBg: 'rgba(7, 193, 96, 0.1)',
        iconColor: '#07C160',
        titleKey: 'notice.wecom.panelTitle',
        descKey: 'notice.wecom.panelDesc',
        urlPlaceholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...',
        testButtonKey: 'notice.wecom.testButton',
      }
    case 'slack':
      return {
        iconClass: 'channel-icon-slack',
        iconBg: 'rgba(74, 21, 75, 0.1)',
        iconColor: '#4A154B',
        titleKey: 'notice.slack.panelTitle',
        descKey: 'notice.slack.panelDesc',
        urlPlaceholder: 'https://hooks.slack.com/services/...',
        testButtonKey: 'notice.slack.testButton',
      }
  }
})

// Narrow casts help template type-checking when we know which platform
// we're rendering. The runtime field is always present because the
// model type matches via NotificationConfig.
const dingtalkModel = computed(() => props.model as NotificationDingTalkConfig)
const feishuModel = computed(() => props.model as NotificationFeishuConfig)
const wecomModel = computed(() => props.model as NotificationWecomConfig)
const slackModel = computed(() => props.model as NotificationSlackConfig)

const baseModel = computed(() => props.model as ChatBotBase)

// Shared rule: bot URL is the only hard-required field. Sign secret is
// optional (DingTalk's keyword mode and WeCom/Slack don't sign).
// Severity floor / alert-type filter validate at the backend layer too,
// so we keep the front-end check lightweight.
const rules = computed<FormRules>(() => {
  if (!channelEnabled.value) {
    return {}
  }
  return {
    url: [
      { required: true, message: t('notice.validation.webhookUrlRequired'), trigger: ['blur', 'change'] },
      {
        validator: (_rule, value: string, callback) => {
          const url = String(value || '').trim()
          if (!url) {
            callback()
            return
          }
          try {
            const parsed = new URL(url)
            if (!['http:', 'https:'].includes(parsed.protocol)) {
              callback(new Error(t('notice.validation.webhookProtocolInvalid')))
              return
            }
            callback()
          } catch (_err) {
            callback(new Error(t('notice.validation.webhookUrlInvalid')))
          }
        },
        trigger: ['blur', 'change'],
      },
    ],
  }
})

const alertTypeLabelMap: Record<string, string> = {
  DDoS防护: 'notice.alertTypes.ddos',
  解析失败: 'notice.alertTypes.resolveFailed',
  DNSSEC异常: 'notice.alertTypes.dnssec',
  配置变更: 'notice.alertTypes.configChanged',
}
const alertTypeLabel = (value: string) => t(alertTypeLabelMap[value] || 'common.unknown')

const validate = async (): Promise<boolean> => {
  if (!channelEnabled.value) {
    return true
  }
  return validateFormAndFocus(formRef.value)
}

const clearValidate = (): void => {
  formRef.value?.clearValidate()
}

watch(channelEnabled, (value) => {
  if (!value) {
    clearValidate()
  }
})

defineExpose<NotificationTabExpose>({
  validate,
  clearValidate,
})
</script>

<template>
  <div class="channel-tab">
    <div class="channel-header">
      <div class="channel-header-left">
        <div
          class="channel-icon"
          :class="platformMeta.iconClass"
          :style="{ background: platformMeta.iconBg, color: platformMeta.iconColor }"
        >
          <!-- Generic chat-bubble icon; the surrounding tile colour
               disambiguates between platforms. Avoids shipping four
               vendor logos that would risk trademark issues. -->
          <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
        </div>
        <div>
          <div class="channel-title">{{ t(platformMeta.titleKey) }}</div>
          <div class="channel-desc">{{ t(platformMeta.descKey) }}</div>
        </div>
      </div>
      <div class="channel-header-right">
        <span class="channel-status-badge" :class="channelEnabled ? 'is-on' : 'is-off'">
          {{ channelEnabled ? $t('notice.enabled') : $t('notice.disabled') }}
        </span>
        <el-switch v-model="channelEnabled" />
      </div>
    </div>

    <el-form
      ref="formRef"
      :model="model"
      :rules="rules"
      label-position="top"
      require-asterisk-position="left"
      class="channel-form"
    >
      <el-row :gutter="20">
        <!-- Common: bot URL -->
        <el-col :xs="24" :md="16">
          <el-form-item :label="$t('notice.botUrl')" prop="url" :required="channelEnabled">
            <el-input v-model="baseModel.url" :placeholder="platformMeta.urlPlaceholder" :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item :label="$t('notice.minLevel')">
            <el-select v-model="baseModel.minLevel" :disabled="disabled" style="width: 100%">
              <el-option :label="$t('notice.minLevelAny')" value="" />
              <el-option label="info" value="info" />
              <el-option label="warning" value="warning" />
              <el-option label="critical" value="critical" />
            </el-select>
          </el-form-item>
        </el-col>

        <!-- Common: sign secret (DingTalk/Feishu use it; WeCom/Slack ignore it but the field stays for parity) -->
        <el-col v-if="platform === 'dingtalk' || platform === 'feishu'" :xs="24" :md="12">
          <el-form-item :label="$t('notice.signSecret')">
            <el-input
              v-model="baseModel.secret"
              show-password
              :disabled="disabled"
              :placeholder="$t('notice.signSecretPlaceholder')"
            />
          </el-form-item>
        </el-col>

        <!-- Feishu-only: interactive card switch -->
        <el-col v-if="platform === 'feishu'" :xs="24" :md="12">
          <el-form-item :label="$t('notice.feishu.useCard')">
            <el-switch v-model="feishuModel.useCard" :disabled="disabled" />
            <span class="inline-hint">{{ $t('notice.feishu.useCardHint') }}</span>
          </el-form-item>
        </el-col>

        <!-- DingTalk @-mentions -->
        <template v-if="platform === 'dingtalk'">
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.dingtalk.atMobiles')">
              <el-select
                v-model="dingtalkModel.atMobiles"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                :placeholder="$t('notice.dingtalk.atMobilesPlaceholder')"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.dingtalk.atUserIds')">
              <el-select
                v-model="dingtalkModel.atUserIds"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.dingtalk.atAll')">
              <el-switch v-model="dingtalkModel.atAll" :disabled="disabled" />
            </el-form-item>
          </el-col>
        </template>

        <!-- Feishu @-mentions -->
        <template v-if="platform === 'feishu'">
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.feishu.atUsers')">
              <el-select
                v-model="feishuModel.atUsers"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                :placeholder="$t('notice.feishu.atUsersPlaceholder')"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.feishu.atAll')">
              <el-switch v-model="feishuModel.atAll" :disabled="disabled" />
            </el-form-item>
          </el-col>
        </template>

        <!-- WeCom @-mentions -->
        <template v-if="platform === 'wecom'">
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.wecom.atMobiles')">
              <el-select
                v-model="wecomModel.atMobiles"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                :placeholder="$t('notice.wecom.atMobilesPlaceholder')"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item :label="$t('notice.wecom.atUsers')">
              <el-select
                v-model="wecomModel.atUsers"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                :placeholder="$t('notice.wecom.atUsersPlaceholder')"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </template>

        <!-- Slack channel/username/icon overrides + mentions -->
        <template v-if="platform === 'slack'">
          <el-col :xs="24" :md="8">
            <el-form-item :label="$t('notice.slack.channel')">
              <el-input v-model="slackModel.channel" :placeholder="$t('notice.slack.channelPlaceholder')" :disabled="disabled" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item :label="$t('notice.slack.username')">
              <el-input v-model="slackModel.username" :placeholder="$t('notice.slack.usernamePlaceholder')" :disabled="disabled" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="8">
            <el-form-item :label="$t('notice.slack.iconEmoji')">
              <el-input v-model="slackModel.iconEmoji" placeholder=":rotating_light:" :disabled="disabled" />
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item :label="$t('notice.slack.mentions')">
              <el-select
                v-model="slackModel.mentions"
                multiple
                filterable
                allow-create
                default-first-option
                :disabled="disabled"
                :placeholder="$t('notice.slack.mentionsPlaceholder')"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </template>

        <!-- Common: alert-type filter -->
        <el-col :span="24">
          <el-form-item :label="$t('notice.alertType')">
            <el-checkbox-group v-model="baseModel.alertTypes" :disabled="disabled" class="channel-checkbox-group">
              <el-checkbox v-for="item in notificationAlertTypeOptions" :key="item" :label="item" :value="item">
                {{ alertTypeLabel(item) }}
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>
        </el-col>

        <!-- Common: SSRF opt-in for self-hosted IM -->
        <el-col :span="24">
          <el-form-item :label="$t('notice.allowPrivate')">
            <el-switch v-model="baseModel.allowPrivateNetwork" :disabled="disabled" />
            <span class="inline-hint">{{ $t('notice.allowPrivateHint') }}</span>
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <div class="channel-actions">
      <el-button type="primary" :loading="loading" :disabled="disabled || loading" @click="emit('test')">
        {{ t(platformMeta.testButtonKey) }}
      </el-button>
      <el-button type="primary" plain :loading="loading" :disabled="disabled || loading" @click="emit('save')">
        {{ $t('common.save') }}
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.channel-tab {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.channel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: linear-gradient(180deg, #FAFBFC 0%, #FFFFFF 100%);
}

.channel-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.channel-icon {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  flex-shrink: 0;
}

.channel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
  line-height: 1.4;
}

.channel-desc {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-top: 2px;
}

.channel-header-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.channel-status-badge {
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid transparent;
}

.channel-status-badge.is-on {
  background: rgba(0, 180, 42, 0.1);
  color: var(--app-success);
  border-color: rgba(0, 180, 42, 0.25);
}

.channel-status-badge.is-off {
  background: rgba(134, 144, 156, 0.08);
  color: var(--app-disabled);
  border-color: var(--app-border);
}

.channel-checkbox-group {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 20px;
}

.channel-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  padding-top: 4px;
  border-top: 1px solid var(--app-border);
  margin-top: 4px;
}

.inline-hint {
  margin-left: 12px;
  font-size: 12px;
  color: var(--app-text-regular);
}

@media (max-width: 767px) {
  .channel-actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
