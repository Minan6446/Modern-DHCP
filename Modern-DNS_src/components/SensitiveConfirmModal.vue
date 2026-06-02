<script setup lang="ts">
// SensitiveConfirmModal — step-up auth dialog for destructive operations.
//
// Mounted imperatively by `confirmSensitive()` (see sensitiveConfirm.ts):
// the caller awaits a Promise that resolves to either the headers we
// should attach on retry, or `null` when the operator cancels. Once the
// user picks a method (TOTP if MFA is bound, otherwise password) we emit
// `confirm` with the chosen header pair and the parent imperatively
// unmounts us.
//
// We deliberately don't import the auth store — the password/TOTP code
// the operator types here is short-circuited straight into the headers
// of a single retry by the axios interceptor; nothing is persisted.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  mfaEnabled: boolean
  message?: string
}>()

const emit = defineEmits<{
  (e: 'confirm', payload: { password?: string; totp?: string }): void
  (e: 'cancel'): void
}>()

const visible = ref(true)
// Default to TOTP when the user has it bound — MFA is the more
// phishing-resistant proof and we should nudge the operator toward it.
// They can still flip to password via the toggle if they prefer.
const method = ref<'totp' | 'password'>(props.mfaEnabled ? 'totp' : 'password')
const password = ref('')
const totp = ref('')
const submitting = ref(false)

const valid = computed(() => {
  if (method.value === 'totp') return /^\d{6}$/.test(totp.value.trim())
  return password.value.length > 0
})

const onConfirm = (): void => {
  if (!valid.value) return
  submitting.value = true
  if (method.value === 'totp') {
    emit('confirm', { totp: totp.value.trim() })
  } else {
    emit('confirm', { password: password.value })
  }
}

const onCancel = (): void => {
  visible.value = false
  emit('cancel')
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="t('sensitive.title')"
    width="420px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    @close="onCancel"
  >
    <div class="sc-body">
      <p class="sc-msg">{{ props.message || t('sensitive.defaultMessage') }}</p>

      <el-radio-group v-if="props.mfaEnabled" v-model="method" size="small" class="sc-method">
        <el-radio-button label="totp">{{ t('sensitive.methodTotp') }}</el-radio-button>
        <el-radio-button label="password">{{ t('sensitive.methodPassword') }}</el-radio-button>
      </el-radio-group>

      <el-input
        v-if="method === 'totp'"
        v-model="totp"
        maxlength="6"
        :placeholder="t('sensitive.totpPlaceholder')"
        class="sc-input mn-mono"
        autofocus
        @keyup.enter="onConfirm"
      />
      <el-input
        v-else
        v-model="password"
        type="password"
        show-password
        :placeholder="t('sensitive.passwordPlaceholder')"
        class="sc-input"
        autofocus
        @keyup.enter="onConfirm"
      />
    </div>
    <template #footer>
      <el-button @click="onCancel">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :disabled="!valid" :loading="submitting" @click="onConfirm">
        {{ t('sensitive.confirmBtn') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.sc-body { display:flex; flex-direction:column; gap:14px; }
.sc-msg { margin:0; font-size:13px; color:var(--dns-text-body-color); line-height:1.6; }
.sc-method { align-self:flex-start; }
.sc-input { width:100%; }
</style>
