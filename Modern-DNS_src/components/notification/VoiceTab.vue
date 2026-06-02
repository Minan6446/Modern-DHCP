<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import type { NotificationTabExpose, NotificationVoiceConfig } from '../../types/notification'
import { normalizeMultiValue, validateFormAndFocus } from '../../utils/interaction'

// VoiceTab edits the Aliyun TTS voice channel config. Voice and SMS
// share an Aliyun signing scheme but the API surface diverges (voice
// only accepts one phone per request, has playTimes/volume controls,
// and uses TtsCode/TtsParam instead of TemplateCode/TemplateParam) so
// they get separate components.

const props = defineProps<{
  model: NotificationVoiceConfig
  loading: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  test: []
  save: []
}>()
const { t } = useI18n()

const formRef = ref<FormInstance>()
const voiceEnabled = computed({
  get: () => props.model.enabled,
  set: (value: boolean) => {
    props.model.enabled = value
  },
})

const mobilePattern = /^1\d{10}$/

const requiredRule = (key: string): FormRules[string] => [
  { required: true, message: t(key), trigger: ['blur', 'change'] },
]

const rules = computed<FormRules>(() => {
  if (!voiceEnabled.value) {
    return {}
  }
  return {
    accessKeyId: requiredRule('notice.validation.accessKeyIdRequired'),
    accessKeySecret: requiredRule('notice.validation.accessKeySecretRequired'),
    ttsCode: requiredRule('notice.voice.validation.ttsCodeRequired'),
    phones: [
      {
        required: true,
        validator: (_rule, value: string, callback) => {
          const phones = String(value || '')
            .split(',')
            .map((p) => p.trim())
            .filter(Boolean)
          if (!phones.length) {
            callback(new Error(t('notice.validation.phonesRequired')))
            return
          }
          if (phones.some((p) => !mobilePattern.test(p))) {
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
  if (!voiceEnabled.value) {
    return true
  }
  return validateFormAndFocus(formRef.value)
}
const clearValidate = (): void => formRef.value?.clearValidate()

watch(voiceEnabled, (value) => {
  if (!value) clearValidate()
})

defineExpose<NotificationTabExpose>({ validate, clearValidate })
</script>

<template>
  <div class="channel-tab">
    <div class="channel-header">
      <div class="channel-header-left">
        <div class="channel-icon channel-icon-voice">
          <!-- Generic phone-receiver glyph; the orange tile maps to the
               warning palette to remind operators that voice calls are
               billed per attempt. -->
          <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M6.62 10.79a15.05 15.05 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.05-.24c1.12.37 2.33.57 3.57.57.55 0 1 .45 1 1V20a1 1 0 0 1-1 1A17 17 0 0 1 3 4a1 1 0 0 1 1-1h3.5c.55 0 1 .45 1 1 0 1.25.2 2.45.57 3.57.11.35.03.74-.25 1.02l-2.2 2.2z"/></svg>
        </div>
        <div>
          <div class="channel-title">{{ $t('notice.voice.panelTitle') }}</div>
          <div class="channel-desc">{{ $t('notice.voice.panelDesc') }}</div>
        </div>
      </div>
      <div class="channel-header-right">
        <span class="channel-status-badge" :class="voiceEnabled ? 'is-on' : 'is-off'">
          {{ voiceEnabled ? $t('notice.enabled') : $t('notice.disabled') }}
        </span>
        <el-switch v-model="voiceEnabled" />
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
          <el-form-item :label="$t('notice.accessKeyId')" prop="accessKeyId" :required="voiceEnabled">
            <el-input v-model="model.accessKeyId" :disabled="disabled" placeholder="LTAI..." />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.accessKeySecret')" prop="accessKeySecret" :required="voiceEnabled">
            <el-input v-model="model.accessKeySecret" show-password :disabled="disabled" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.voice.ttsCode')" prop="ttsCode" :required="voiceEnabled">
            <el-input v-model="model.ttsCode" :disabled="disabled" placeholder="TTS_xxxxxxxxx" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.voice.calledShowNumber')">
            <el-input v-model="model.calledShowNumber" :disabled="disabled" :placeholder="$t('notice.voice.calledShowNumberPlaceholder')" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.regionId')">
            <el-select v-model="model.regionId" :disabled="disabled" style="width: 100%">
              <el-option label="cn-hangzhou (华东1)" value="cn-hangzhou" />
              <el-option label="cn-beijing (华北2)" value="cn-beijing" />
              <el-option label="cn-shenzhen (华南1)" value="cn-shenzhen" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item :label="$t('notice.minLevel')">
            <el-select v-model="model.minLevel" :disabled="disabled" style="width: 100%">
              <el-option :label="$t('notice.minLevelAny')" value="" />
              <el-option label="info" value="info" />
              <el-option label="warning" value="warning" />
              <el-option label="critical" value="critical" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item :label="$t('notice.phones')" prop="phones" :required="voiceEnabled">
            <el-input
              v-model="model.phones"
              :placeholder="$t('notice.phonesPlaceholder')"
              :disabled="disabled"
              @blur="normalizePhones"
            />
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item :label="$t('notice.voice.ttsParam')">
            <el-input
              v-model="model.ttsParam"
              type="textarea"
              :rows="2"
              :disabled="disabled"
              :placeholder="$t('notice.voice.ttsParamPlaceholder')"
            />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item :label="$t('notice.voice.playTimes')">
            <el-input-number v-model="model.playTimes" :min="1" :max="3" :disabled="disabled" style="width: 100%" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item :label="$t('notice.voice.volume')">
            <el-input-number v-model="model.volume" :min="0" :max="100" :step="5" :disabled="disabled" style="width: 100%" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="8">
          <el-form-item :label="$t('notice.voice.maxCalls')">
            <el-input-number v-model="model.maxCalls" :min="1" :max="50" :disabled="disabled" style="width: 100%" />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <div class="channel-actions">
      <el-button type="primary" :loading="loading" :disabled="disabled || loading" @click="emit('test')">
        {{ $t('notice.voice.testButton') }}
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

.channel-icon-voice { background: rgba(245, 108, 108, 0.1); color: #F56C6C; }

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
