<template>
  <div class="system-filter-bar">
    <el-input
      :model-value="modelValue"
      :placeholder="placeholder"
      clearable
      @update:model-value="handleInput"
    />
    <slot />
    <el-button v-if="actionLabel" type="primary" @click="$emit('action')">{{ actionLabel }}</el-button>
  </div>
</template>

<script setup lang="ts">
interface Props {
  modelValue: string;
  placeholder?: string;
  actionLabel?: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
  (e: 'search', value: string): void;
  (e: 'action'): void;
}>();

const handleInput = (value: string | number) => {
  const text = String(value ?? '');
  emit('update:modelValue', text);
  emit('search', text);
};
</script>

<style scoped>
.system-filter-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
