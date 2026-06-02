<script setup lang="ts">
import { ref } from 'vue'
import GlobalForwardPage from './GlobalForwardPage.vue'
import ConditionForwardPage from './ConditionForwardPage.vue'

const activeTab = ref('global')
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('forward.title') }}</h1>
        <p class="page-subtitle">{{ $t('forward.subtitle') }}</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="forward-tabs">
      <el-tab-pane :label="$t('forward.globalForward')" name="global">
        <div class="tab-desc">{{ $t('forward.globalForwardTabDesc') }}</div>
        <GlobalForwardPage />
      </el-tab-pane>
      <el-tab-pane :label="$t('forward.conditionForward')" name="condition">
        <div class="tab-desc">{{ $t('forward.conditionForwardTabDesc') }}</div>
        <ConditionForwardPage />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.forward-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
  padding: 0 4px;
  background: var(--app-bg);
  border-bottom: 1px solid var(--app-border);
}
.forward-tabs :deep(.el-tabs__content) {
  padding: 0;
}
.tab-desc {
  font-size: 12px;
  color: var(--app-text-secondary);
  padding: 10px 2px 14px;
}
/* Hide inner page-shell / page-header inside embedded pages */
.forward-tabs :deep(.page-shell) {
  padding: 0;
  gap: 12px;
}
.forward-tabs :deep(.page-header) {
  display: none;
}
</style>
