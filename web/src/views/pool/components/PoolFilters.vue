<template>
  <div class="toolbar">
    <el-input
      v-model="model.keyword"
      :placeholder="t('pool.search')"
      clearable
      @input="$emit('change')"
    />
    <el-select
      v-model="model.status"
      :placeholder="t('pool.status')"
      clearable
      style="width: 140px"
      @change="$emit('change')"
    >
      <el-option value="active" :label="t('pool.statusActive')" />
      <el-option value="disabled" :label="t('pool.statusDisabled')" />
      <el-option value="warning" :label="t('pool.statusWarning')" />
    </el-select>
    <el-button type="primary" @click="$emit('create')">{{ t('pool.create') }}</el-button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const model = defineModel<{ keyword: string; status: string }>({ required: true });
const { t } = useI18n();

defineEmits<{
  (e: 'change'): void;
  (e: 'create'): void;
}>();
</script>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
}
</style>
