<template>
  <section class="overview-grid">
    <el-card shadow="never" class="overview-card">
      <div class="overview-label">{{ t('system.audit.statTotal') }}</div>
      <div class="overview-value">{{ statTotalToday }}</div>
    </el-card>
    <el-card shadow="never" class="overview-card">
      <div class="overview-label">{{ t('system.audit.statSuccess') }}</div>
      <div class="overview-value">{{ statSuccessToday }}</div>
    </el-card>
    <el-card shadow="never" class="overview-card">
      <div class="overview-label">{{ t('system.audit.statFailed') }}</div>
      <div class="overview-value">{{ statFailedToday }}</div>
    </el-card>
    <el-card shadow="never" class="overview-card">
      <div class="overview-label">{{ t('system.audit.statRisk') }}</div>
      <div class="overview-value">{{ statRiskToday }}</div>
    </el-card>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import dayjs from 'dayjs';
import type { LoginAuditRecord } from '@/types/system';

const props = withDefaults(defineProps<{
  rows: LoginAuditRecord[];
  isRiskRow?: (row: LoginAuditRecord) => boolean;
}>(), {
  isRiskRow: () => false
});

const { t } = useI18n();

const today = dayjs().format('YYYY-MM-DD');

const todayRows = computed(() => {
  return props.rows.filter((item: LoginAuditRecord) => dayjs(item.createdAt).format('YYYY-MM-DD') === today);
});

const statTotalToday = computed(() => todayRows.value.length);
const statSuccessToday = computed(() => todayRows.value.filter((item: LoginAuditRecord) => item.result === 'success' && !props.isRiskRow(item)).length);
const statFailedToday = computed(() => todayRows.value.filter((item: LoginAuditRecord) => item.result === 'failed').length);
const statRiskToday = computed(() => todayRows.value.filter((item: LoginAuditRecord) => props.isRiskRow(item)).length);
</script>

<style scoped>
.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.overview-card {
  text-align: center;
}

.overview-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
}

.overview-value {
  font-size: 28px;
  font-weight: 700;
}
</style>
