<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import type { NotificationEmailConfig, NotificationTabExpose } from '../../types/notification'
import { normalizeMultiValue, validateFormAndFocus } from '../../utils/interaction'

const props = defineProps<{
  model: NotificationEmailConfig
  loading: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  test: []
  save: []
}>()
const { t } = useI18n()

const formRef = ref<FormInstance>()
const emailEnabled = computed({
  get: () => props.model.enabled,
  set: (value: boolean) => {
    props.model.enabled = value
  },
})

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// Why `computed` and explicit `required: true` rules with translated messages?
//
// Element Plus's <el-form-item> auto-injects a `{required: true}` rule when
// you set the `required` prop and no rule already declares `"required" in
// rule`. That auto-injected rule has NO `message`, so async-validator falls
// back to `${prop} is required` — which surfaces as the English
// "smtpHost is required" / "sender is required" toasts the user reported.
//
// Fix: declare each required rule ourselves (with the translated message)
// and switch the form-item to `:required="emailEnabled"` so the asterisk
// (and the "required" semantics) only apply when the channel is actually on.
// Element Plus's matching logic treats our `required: true` rule as the
// canonical one and skips its own injection (see form-item internals).
const rules = computed<FormRules>(() => {
  if (!emailEnabled.value) {
    return {}
  }
  return {
    smtpHost: [
      { required: true, message: t('notice.validation.smtpHostRequired'), trigger: ['blur', 'change'] },
    ],
    smtpPort: [
      {
        required: true,
        validator: (_rule, value: number, callback) => {
          const port = Number(value)
          if (!Number.isInteger(port) || port < 0 || port > 65535) {
            callback(new Error(t('notice.validation.smtpPortRange')))
            return
          }
          callback()
        },
        trigger: ['blur', 'change'],
      },
    ],
    sender: [
      { required: true, message: t('notice.validation.senderRequired'), trigger: ['blur', 'change'] },
      {
        validator: (_rule, value: string, callback) => {
          const email = String(value || '').trim()
          if (!email) {
            // Empty case is already covered by the required: true rule above;
            // pass here so we don't double-report.
            callback()
            return
          }
          if (!emailPattern.test(email)) {
            callback(new Error(t('notice.validation.emailInvalid')))
            return
          }
          callback()
        },
        trigger: ['blur', 'change'],
      },
    ],
    authCode: [
      { required: true, message: t('notice.validation.authCodeRequired'), trigger: ['blur', 'change'] },
    ],
    receivers: [
      {
        // Receivers is optional at *configuration* time — the field
        // exists primarily so operators can target the "send test
        // email" button at a real inbox without polluting the alert
        // routing list. Live alerts populate the To: header from the
        // subscription/route rules, not from this field. We still
        // validate the *format* of any addresses the operator typed
        // so a typo doesn't slip through silently.
        required: false,
        validator: (_rule, value: string, callback) => {
          const receivers = String(value || '')
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean)
          if (!receivers.length) {
            // Empty is fine — saving an SMTP profile without test
            // recipients shouldn't be blocked.
            callback()
            return
          }
          if (receivers.some((item) => !emailPattern.test(item))) {
            callback(new Error(t('notice.validation.receiversInvalid')))
            return
          }
          callback()
        },
        trigger: ['blur', 'change'],
      },
    ],
  }
})

const normalizeReceivers = (): void => {
  props.model.receivers = normalizeMultiValue(props.model.receivers)
}

const validate = async (): Promise<boolean> => {
  if (!emailEnabled.value) {
    return true
  }
  return validateFormAndFocus(formRef.value)
}

const clearValidate = (): void => {
  formRef.value?.clearValidate()
}

watch(emailEnabled, (value) => {
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
        <div class="channel-icon channel-icon-email">
          <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 4-8 5-8-5V6l8 5 8-5v2z" /></svg>
        </div>
        <div>
          <div class="channel-title">{{ $t('notice.emailPanelTitle') }}</div>
          <div class="channel-desc">{{ $t('notice.emailPanelDesc') }}</div>
        </div>
      </div>
      <div class="channel-header-right">
        <span class="channel-status-badge" :class="emailEnabled ? 'is-on' : 'is-off'">{{ emailEnabled ? $t('notice.enabled') : $t('notice.disabled') }}</span>
        <el-switch v-model="emailEnabled" />
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
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.smtpServer')" prop="smtpHost" :required="emailEnabled">
            <el-input v-model="model.smtpHost" placeholder="smtp.example.com" :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('common.port')" prop="smtpPort" :required="emailEnabled">
            <el-input-number v-model="model.smtpPort" :min="0" :max="65535" class="full-width" :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.senderEmail')" prop="sender" :required="emailEnabled">
            <el-input v-model="model.sender" placeholder="alert@example.com" :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.authCode')" prop="authCode" :required="emailEnabled">
            <el-input v-model="model.authCode" show-password :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item :label="$t('notice.recipients')" prop="receivers" :required="false">
            <el-input
              v-model="model.receivers"
              :placeholder="$t('notice.recipientsPlaceholder')"
              :disabled="disabled"
              @blur="normalizeReceivers"
            />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <div class="channel-actions">
      <el-button type="primary" :loading="loading" :disabled="disabled || loading" @click="emit('test')">{{ $t('notice.testEmail') }}</el-button>
      <el-button type="primary" plain :loading="loading" :disabled="disabled || loading" @click="emit('save')">{{ $t('common.save') }}</el-button>
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

.channel-icon-email { background: var(--app-accent-soft); color: var(--app-accent); }

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

.channel-form :deep(.el-input-number) {
  width: 100%;
}

.full-width {
  width: 100%;
}

.channel-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  padding-top: 4px;
  border-top: 1px solid var(--app-border);
  margin-top: 4px;
}

@media (max-width: 767px) {
  .channel-actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>