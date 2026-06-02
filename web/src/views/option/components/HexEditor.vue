<template>
  <div class="hex-editor">
    <div class="toolbar">
      <span>十六进制编辑器</span>
      <el-tag size="small" type="info">{{ bytes }} bytes</el-tag>
      <el-button size="small" text @click="format">格式化</el-button>
    </div>
    <el-input
      type="textarea"
      :rows="4"
      :model-value="displayValue"
      placeholder="如 0104DEADBEEF"
      @input="onInput"
    />
    <div v-if="!valid" class="error">需要偶数字节的十六进制字符串</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{ modelValue: string }>();
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>();

const clean = computed(() => (props.modelValue || '').replace(/\s+/g, '').toUpperCase());

const bytes = computed(() => Math.ceil(clean.value.length / 2));

const valid = computed(() => /^(?:[0-9A-F]{2})*$/.test(clean.value));

const displayValue = computed(() => clean.value.match(/.{1,2}/g)?.join(' ') || '');

const onInput = (val: string) => {
  const normalized = (val || '').replace(/[^0-9A-Fa-f]/g, '').toUpperCase();
  emit('update:modelValue', normalized);
};

const format = () => emit('update:modelValue', clean.value);
</script>

<style scoped>
.hex-editor {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.error {
  color: var(--el-color-danger);
  font-size: 12px;
}
</style>
