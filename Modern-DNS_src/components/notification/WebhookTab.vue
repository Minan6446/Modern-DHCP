<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import type { NotificationTabExpose, NotificationWebhookConfig } from '../../types/notification'
import { notificationAlertTypeOptions } from '../../types/notification'
import { validateFormAndFocus } from '../../utils/interaction'

const props = defineProps<{
  model: NotificationWebhookConfig
  loading: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  test: []
  save: []
}>()
const { t } = useI18n()

const formRef = ref<FormInstance>()
const webhookEnabled = computed({
  get: () => props.model.enabled,
  set: (value: boolean) => {
    props.model.enabled = value
  },
})

// See EmailTab.vue for the full rationale: el-form-item's `required` prop
// auto-injects a message-less rule, surfacing "url is required" in English.
// We declare required: true ourselves with a translated message and gate
// the whole rule object on webhookEnabled so disabling the channel quiets
// the form.
const rules = computed<FormRules>(() => {
  if (!webhookEnabled.value) {
    return {}
  }
  return {
    url: [
      { required: true, message: t('notice.validation.webhookUrlRequired'), trigger: ['blur', 'change'] },
      {
        validator: (_rule, value: string, callback) => {
          const url = String(value || '').trim()
          if (!url) {
            // Required check above already reported this — stay quiet.
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
          } catch (_error) {
            callback(new Error(t('notice.validation.webhookUrlInvalid')))
          }
        },
        trigger: ['blur', 'change'],
      },
    ],
    method: [{ required: true, message: t('notice.validation.requestMethodRequired'), trigger: 'change' }],
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
  if (!webhookEnabled.value) {
    return true
  }
  return validateFormAndFocus(formRef.value)
}

const clearValidate = (): void => {
  formRef.value?.clearValidate()
}

watch(webhookEnabled, (value) => {
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
        <div class="channel-icon channel-icon-webhook">
          <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M13.5 2c-5.62 0-10.19 4.27-10.48 9.82L1 9.8v4.49l5.26-3.6-5.26-3.64v2.52C1.26 6.88 6.89 2.75 13.5 2.75c5.52 0 10.16 3.57 11.89 8.6l.85-.61A12.73 12.73 0 0 0 13.5 2zM22 9.71l-5.26 3.6 5.26 3.64v-2.52C21.74 17.12 16.11 21.25 9.5 21.25c-5.52 0-10.16-3.57-11.89-8.6l-.85.61A12.73 12.73 0 0 0 9.5 22c5.62 0 10.19-4.27 10.48-9.82L22 14.2V9.71z" /></svg>
        </div>
        <div>
          <div class="channel-title">{{ $t('notice.webhookPanelTitle') }}</div>
          <div class="channel-desc">{{ $t('notice.webhookPanelDesc') }}</div>
        </div>
      </div>
      <div class="channel-header-right">
        <span class="channel-status-badge" :class="webhookEnabled ? 'is-on' : 'is-off'">{{ webhookEnabled ? $t('notice.enabled') : $t('notice.disabled') }}</span>
        <el-switch v-model="webhookEnabled" />
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
          <el-form-item :label="$t('notice.webhookUrl')" prop="url" :required="webhookEnabled">
            <el-input v-model="model.url" placeholder="https://hooks.example.com" :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.requestMethod')" prop="method" :required="webhookEnabled">
            <el-select v-model="model.method" class="full-width" :disabled="disabled">
              <el-option label="GET" value="GET" />
              <el-option label="POST" value="POST" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.secret')">
            <el-input v-model="model.secret" show-password :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.alertType')">
            <el-checkbox-group v-model="model.alertTypes" :disabled="disabled" class="channel-checkbox-group">
              <el-checkbox v-for="item in notificationAlertTypeOptions" :key="item" :label="item" :value="item">
                {{ alertTypeLabel(item) }}
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <div class="channel-actions">
      <el-button type="primary" :loading="loading" :disabled="disabled || loading" @click="emit('test')">{{ $t('notice.testRequest') }}</el-button>
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

.channel-icon-webhook { background: rgba(114, 46, 209, 0.1); color: #722ED1; }

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