<template>
  <el-card shadow="hover" class="panel surface-card">
    <div class="panel-header">{{ t('dashboard.quickTitle') }}</div>
    <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12">
      <template #actions>
        <el-button size="small" :loading="loading" @click="fetchData">{{
          t('common.retry')
        }}</el-button>
      </template>
    </AppErrorCallout>
    <el-empty v-else-if="!canView" :description="t('common.noPermission')" />
    <el-skeleton v-else-if="loading" animated :rows="4" />
    <template v-else>
      <el-empty v-if="isEmpty" :description="t('dashboard.empty')">
        <template #extra>
          <el-button size="small" @click="fetchData">{{ t('common.retry') }}</el-button>
        </template>
      </el-empty>
      <div v-else class="insight-grid">
        <div class="chart-card">
          <div class="chart-title">{{ t('dashboard.hotPools') }}</div>
          <BaseEChart v-if="heatOption" :option="heatOption" />
          <el-empty v-else :description="t('dashboard.empty')" />
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('dashboard.leaseStatus') }}</div>
          <BaseEChart v-if="deviceOption" :option="deviceOption" />
          <el-empty v-else :description="t('dashboard.empty')" />
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('dashboard.anomalies') }}</div>
          <BaseEChart v-if="radarOption" :option="radarOption" />
          <el-empty v-else :description="t('dashboard.empty')" />
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('dashboard.capacity') }}</div>
          <BaseEChart v-if="forecastOption" :option="forecastOption" />
          <el-empty v-else :description="t('dashboard.empty')" />
        </div>
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { listPoolUsage, getLeaseStatus, listAnomalies } from '@/api/monitoring';
import type { PoolUsageSummary, LeaseStatusSummary, AnomalyRecord } from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { createInlineError, getApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const loading = ref(false);
const poolUsage = ref<PoolUsageSummary[]>([]);
const leaseStatus = ref<LeaseStatusSummary | null>(null);
const anomalies = ref<AnomalyRecord[]>([]);
const pageError = ref<ApiErrorDescriptor | null>(null);
const canView = computed(() => permissionStore.can('monitoring.view'));

const toErrorDescriptor = (err: unknown) => {
  const mapped = getApiError(err);
  if (mapped) return mapped;
  if (err instanceof Error && err.message) {
    return createInlineError(err.message);
  }
  return createInlineError(t('common.loadFail'));
};

const isEmpty = computed(
  () => !poolUsage.value.length && !leaseStatus.value && !anomalies.value.length
);

const heatOption = computed(() => {
  if (!poolUsage.value.length) return null;
  const sorted = [...poolUsage.value].sort((a, b) => b.utilization - a.utilization).slice(0, 6);
  return {
    tooltip: {},
    visualMap: { min: 0, max: 100, calculable: true, orient: 'horizontal' },
    series: [
      {
        type: 'heatmap',
        data: sorted.map((p, idx) => [idx, 0, Math.round(p.utilization)]),
        label: { show: true }
      }
    ],
    xAxis: { type: 'category', data: sorted.map((p) => p.poolName) },
    yAxis: { type: 'category', data: [t('dashboard.utilizationPercent')] }
  };
});

const deviceOption = computed(() => {
  if (!leaseStatus.value) return null;
  const data = [
    { name: t('dashboard.stateActive'), value: leaseStatus.value.active },
    { name: t('dashboard.stateExpired'), value: leaseStatus.value.expired },
    { name: t('dashboard.statePending'), value: leaseStatus.value.pending },
    { name: t('dashboard.stateFailed'), value: leaseStatus.value.failed }
  ].filter((d) => d.value > 0);
  if (!data.length) return null;
  return {
    tooltip: { trigger: 'item' },
    series: [
      {
        type: 'pie',
        radius: '60%',
        data
      }
    ]
  };
});

const radarOption = computed(() => {
  if (!anomalies.value.length) return null;
  const counts = anomalies.value.reduce<Record<string, number>>((acc, cur) => {
    acc[cur.type] = (acc[cur.type] || 0) + 1;
    return acc;
  }, {});
  const indicators = Object.keys(counts).map((k) => ({
    name: k,
    max: Math.max(...Object.values(counts), 5)
  }));
  return {
    tooltip: {},
    legend: { data: [t('dashboard.anomalyFrequency')] },
    radar: { indicator: indicators },
    series: [{ type: 'radar', data: [{ value: Object.values(counts), name: t('dashboard.anomalyFrequency') }] }]
  };
});

const forecastOption = computed(() => {
  if (!poolUsage.value.length) return null;
  const sorted = [...poolUsage.value].sort((a, b) => b.used - a.used);
  return {
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: sorted.map((p) => p.poolName) },
    yAxis: { type: 'value', name: t('dashboard.chartUsed') },
    series: [{ type: 'bar', data: sorted.map((p) => p.used) }]
  };
});

const fetchData = async () => {
  if (!canView.value) return;
  loading.value = true;
  pageError.value = null;
  const tenantId = tenantStore.currentTenantId;
  try {
    const [poolRes, leaseRes, anomalyRes] = await Promise.all([
      listPoolUsage({ tenantId }),
      getLeaseStatus({ tenantId }),
      listAnomalies()
    ]);
    poolUsage.value = poolRes.data.data || [];
    leaseStatus.value = leaseRes.data.data || null;
    anomalies.value = anomalyRes.data.data || [];
  } catch (err: unknown) {
    pageError.value = toErrorDescriptor(err);
  } finally {
    loading.value = false;
  }
};

onMounted(fetchData);
watch(
  () => tenantStore.currentTenantId,
  () => fetchData()
);
</script>

<style scoped>
.panel-header {
  font-weight: 600;
  margin-bottom: 12px;
}

.insight-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.chart-card {
  background: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 12px;
  min-height: 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chart-title {
  font-weight: 600;
}
</style>
