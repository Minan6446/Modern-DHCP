<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import type { NotificationSmsConfig, NotificationTabExpose } from '../../types/notification'
import { normalizeMultiValue, validateFormAndFocus } from '../../utils/interaction'

const props = defineProps<{
  model: NotificationSmsConfig
  loading: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  test: []
  save: []
}>()
const { t } = useI18n()

const formRef = ref<FormInstance>()
const smsEnabled = computed({
  get: () => props.model.enabled,
  set: (value: boolean) => {
    props.model.enabled = value
  },
})

const mobilePattern = /^1\d{10}$/

// See EmailTab.vue for the rationale; we declare required: true with the
// translated message ourselves so el-form-item doesn't fall back to its
// default English "<prop> is required" message.
const requiredRule = (field: 'accessKeyId' | 'accessKeySecret' | 'signName' | 'templateCode'): FormRules[string] => [
  { required: true, message: t(`notice.validation.${field}Required`), trigger: ['blur', 'change'] },
]

const rules = computed<FormRules>(() => {
  if (!smsEnabled.value) {
    return {}
  }
  return {
    accessKeyId: requiredRule('accessKeyId'),
    accessKeySecret: requiredRule('accessKeySecret'),
    signName: requiredRule('signName'),
    templateCode: requiredRule('templateCode'),
    phones: [
      {
        required: true,
        validator: (_rule, value: string, callback) => {
          const phones = String(value || '')
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean)
          if (!phones.length) {
            callback(new Error(t('notice.validation.phonesRequired')))
            return
          }
          if (phones.some((item) => !mobilePattern.test(item))) {
            callback(new Error(t('notice.validation.phonesInvalid')))
            return
          }
          callback()
        },
        trigger: ['blur', 'change'],
      },
    ],
  }
})

const normalizePhones = (): void => {
  props.model.phones = normalizeMultiValue(props.model.phones)
}

const validate = async (): Promise<boolean> => {
  if (!smsEnabled.value) {
    return true
  }
  return validateFormAndFocus(formRef.value)
}

const clearValidate = (): void => {
  formRef.value?.clearValidate()
}

watch(smsEnabled, (value) => {
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
        <div class="channel-icon channel-icon-sms">
          <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-2 12H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z" /></svg>
        </div>
        <div>
          <div class="channel-title">{{ $t('notice.smsPanelTitle') }}</div>
          <div class="channel-desc">{{ $t('notice.smsPanelDesc') }}</div>
        </div>
      </div>
      <div class="channel-header-right">
        <span class="channel-status-badge" :class="smsEnabled ? 'is-on' : 'is-off'">{{ smsEnabled ? $t('notice.enabled') : $t('notice.disabled') }}</span>
        <el-switch v-model="smsEnabled" />
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
          <el-form-item :label="$t('notice.accessKeyId')" prop="accessKeyId" :required="smsEnabled">
            <el-input v-model="model.accessKeyId" :disabled="disabled" placeholder="LTAI..." />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.accessKeySecret')" prop="accessKeySecret" :required="smsEnabled">
            <el-input v-model="model.accessKeySecret" show-password :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.signName')" prop="signName" :required="smsEnabled">
            <el-input v-model="model.signName" :disabled="disabled" :placeholder="$t('notice.signNamePlaceholder')" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.templateCode')" prop="templateCode" :required="smsEnabled">
            <el-input v-model="model.templateCode" :disabled="disabled" placeholder="SMS_xxxxxxxxx" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.regionId')">
            <el-select v-model="model.regionId" :disabled="disabled" style="width: 100%">
              <el-option label="cn-hangzhou (华东1)" value="cn-hangzhou" />
              <el-option label="cn-beijing (华北2)" value="cn-beijing" />
              <el-option label="cn-shenzhen (华南1)" value="cn-shenzhen" />
              <el-option label="ap-southeast-1 (新加坡)" value="ap-southeast-1" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item :label="$t('notice.phones')" prop="phones" :required="smsEnabled">
            <el-input
              v-model="model.phones"
              :placeholder="$t('notice.phonesPlaceholder')"
              :disabled="disabled"
              @blur="normalizePhones"
            />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <div class="channel-actions">
      <el-button type="primary" :loading="loading" :disabled="disabled || loading" @click="emit('test')">{{ $t('notice.testSms') }}</el-button>
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

.channel-icon-sms { background: rgba(255, 125, 0, 0.1); color: var(--app-warning); }

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