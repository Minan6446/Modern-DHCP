<template>
  <el-alert
    v-if="error"
    :title="error.message"
    type="error"
    show-icon
    :closable="closable"
    class="app-error-callout"
  >
    <template #default>
      <p v-if="error.hint" class="hint">{{ error.hint }}</p>
      <ul v-if="error.fields?.length" class="field-list">
        <li v-for="field in error.fields" :key="field.field">
          <strong>{{ formatField(field.field) }}</strong>
          <span>{{ field.message }}</span>
        </li>
      </ul>
      <div v-if="error.requestId || error.timestamp" class="meta">
        <span v-if="error.requestId">{{ t('errors.requestId', { id: error.requestId }) }}</span>
        <span v-if="error.timestamp">{{ t('errors.timestamp', { ts: error.timestamp }) }}</span>
      </div>
    </template>
  </el-alert>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const props = defineProps<{ error?: ApiErrorDescriptor | null; closable?: boolean }>();
const { t } = useI18n();

const formatField = (name: string) => name?.split('.')?.pop() || name;
const error = computed(() => props.error || null);
const closable = computed(() => props.closable ?? false);
</script>

<style scoped>
.app-error-callout {
  margin-bottom: 12px;
}

.hint {
  margin: 4px 0 0;
}

.field-list {
  margin: 8px 0 0;
  padding-left: 18px;
}

.field-list li {
  list-style: disc;
  margin-bottom: 4px;
}

.field-list strong {
  margin-right: 4px;
}

.meta {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  color: #6b7280;
}
</style>
