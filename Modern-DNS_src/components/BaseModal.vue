<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title: string
    width?: string | number
    loading?: boolean
    showFooter?: boolean
    showCancel?: boolean
    showConfirm?: boolean
    confirmDisabled?: boolean
    confirmText?: string
    cancelText?: string
    draggable?: boolean
    closeOnClickModal?: boolean
    closeOnPressEscape?: boolean
    destroyOnClose?: boolean
  }>(),
  {
    width: 640,
    loading: false,
    showFooter: true,
    showCancel: true,
    showConfirm: true,
    confirmDisabled: false,
    draggable: true,
    closeOnClickModal: false,
    closeOnPressEscape: true,
    destroyOnClose: true,
  },
)

const { t } = useI18n()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'open'): void
  (e: 'opened'): void
  (e: 'close'): void
  (e: 'closed'): void
  (e: 'cancel'): void
  (e: 'confirm'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
})

const confirmLabel = computed(() => props.confirmText ?? t('common.confirm'))
const cancelLabel = computed(() => props.cancelText ?? t('common.cancel'))

const handleCancel = () => {
  emit('cancel')
  emit('update:modelValue', false)
}

const handleConfirm = () => {
  emit('confirm')
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    :width="width"
    :draggable="draggable"
    :close-on-click-modal="closeOnClickModal"
    :close-on-press-escape="closeOnPressEscape"
    :destroy-on-close="destroyOnClose"
    class="dns-base-modal"
    @open="emit('open')"
    @opened="emit('opened')"
    @close="emit('close')"
    @closed="emit('closed')"
  >
    <div class="dns-base-modal__body">
      <slot />
    </div>

    <template v-if="showFooter" #footer>
      <div class="dns-base-modal__footer">
        <slot name="footer-prefix" />

        <el-button v-if="showCancel" @click="handleCancel">{{ cancelLabel }}</el-button>
        <el-button
          v-if="showConfirm"
          type="primary"
          :loading="loading"
          :disabled="confirmDisabled"
          @click="handleConfirm"
        >
          {{ confirmLabel }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.dns-base-modal :deep(.el-dialog) {
  border-radius: var(--dns-radius-base, 8px);
}

.dns-base-modal :deep(.el-dialog__header) {
  margin-right: 0;
  padding: 18px 20px 12px;
  border-bottom: 1px solid var(--dns-border-color, var(--el-border-color));
}

.dns-base-modal :deep(.el-dialog__title) {
  color: var(--dns-text-title-color, var(--el-text-color-primary));
  font-size: var(--dns-text-title-size, 16px);
  font-weight: var(--dns-text-title-weight, 600);
}

.dns-base-modal :deep(.el-dialog__body) {
  padding: 16px 20px;
}

.dns-base-modal :deep(.el-dialog__footer) {
  padding: 12px 20px 18px;
  border-top: 1px solid var(--dns-border-color, var(--el-border-color));
}

.dns-base-modal__body {
  color: var(--dns-text-body-color, var(--el-text-color-regular));
  font-size: var(--dns-text-body-size, 14px);
  font-weight: var(--dns-text-body-weight, 400);
}

.dns-base-modal__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}
</style>
