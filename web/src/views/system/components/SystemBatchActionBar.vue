<template>
  <div class="system-batch-bar">
    <div class="summary">
      <span class="label">{{ label }}</span>
      <span class="count">{{ selectedCount }}</span>
    </div>
    <slot />
    <el-button type="primary" :disabled="disabled" :loading="loading" @click="$emit('action')">
      {{ actionLabel }}
    </el-button>
  </div>
</template>

<script setup lang="ts">
interface Props {
  selectedCount: number;
  actionLabel: string;
  disabled?: boolean;
  loading?: boolean;
  label?: string;
}

withDefaults(defineProps<Props>(), {
  disabled: false,
  loading: false,
  label: 'Selected'
});

defineEmits<{ (e: 'action'): void }>();
</script>

<style scoped>
.system-batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.summary {
  display: flex;
  align-items: center;
  gap: 4px;
}

.count {
  font-weight: 600;
}
</style>
