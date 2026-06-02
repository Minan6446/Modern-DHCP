<template>
  <div class="toolbar">
    <el-date-picker
      v-model="model"
      type="datetimerange"
      :start-placeholder="t('lease.startsAt')"
      :end-placeholder="t('lease.endsAt')"
      @change="$emit('change')"
    />
    <el-button v-if="props.showSearch" @click="$emit('search')">{{ t('common.search') }}</el-button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const model = defineModel<[Date, Date] | null>({ default: null });

const props = withDefaults(defineProps<{ showSearch?: boolean }>(), { showSearch: true });

const { t } = useI18n();

defineEmits<{
  (e: 'search'): void;
  (e: 'change'): void;
}>();
</script>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
}
</style>
