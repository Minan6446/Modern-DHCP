<template>
  <el-card shadow="hover" class="panel surface-card">
    <div class="panel-header">{{ t('dashboard.kpiTitle') }}</div>

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
      <div class="kpi-grid">
        <div v-for="card in kpiCards" :key="card.key" class="kpi-card">
          <div class="kpi-label">{{ card.label }}</div>
          <div class="kpi-value">{{ card.value }}</div>
          <div v-if="card.hint" class="kpi-hint">{{ card.hint }}</div>
        </div>
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { getRealtimeSnapshot, getLeaseStatus, getPoolUsageSummary } from '@/api/monitoring';
import { getLeaseInsights } from '@/api/leases';
import type { PerfSnapshot, PoolUsageSummarySnapshot, LeaseStatusSummary } from '@/types/monitoring';
import type { LeaseInsights } from '@/types/lease';
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
const poolSummary = ref<PoolUsageSummarySnapshot | null>(null);
const leaseStatus = ref<LeaseStatusSummary | null>(null);
const insights = ref<LeaseInsights | null>(null);
const snapshot = ref<PerfSnapshot | null>(null);
const pageError = ref<ApiErrorDescriptor | null>(null);
const canView = computed(() => permissionStore.can('monitoring.view'));

interface KPICard {
  key: string;
  label: string;
  value: string;
  hint?: string;
}

const totalIps = computed(() => poolSummary.value?.totalIps ?? 0);
const assignedIps = computed(() => poolSummary.value?.allocatedIps ?? 0);
const availableIps = computed(() => poolSummary.value?.availableIps ?? 0);
const subnetCount = computed(() => poolSummary.value?.totalPools ?? 0);
const avgLeaseHours = computed(() => insights.value?.avgLeaseDurationHours ?? 0);
const todayNewLeases = computed(() => insights.value?.todayNewLeases ?? 0);
const leaseSuccessRate = computed(() => (snapshot.value?.successRate ?? 0) * 100);
const conflictIps = computed(() => insights.value?.conflictIps ?? 0);

const formatNumber = (value: number, maximumFractionDigits = 0) =>
  value.toLocaleString(undefined, { maximumFractionDigits });

const formatHours = (value: number) => `${value.toFixed(1)}h`;

const kpiCards = computed<KPICard[]>(() => [
  {
    key: 'totalIp',
    label: t('dashboard.totalIp'),
    value: formatNumber(totalIps.value)
  },
  {
    key: 'assignedIp',
    label: t('dashboard.assignedIp'),
    value: formatNumber(assignedIps.value)
  },
  {
    key: 'availableIp',
    label: t('dashboard.availableIp'),
    value: formatNumber(availableIps.value)
  },
  {
    key: 'conflictIp',
    label: t('dashboard.conflictIp'),
    value: formatNumber(conflictIps.value)
  },
  {
    key: 'subnetCount',
    label: t('dashboard.subnetCount'),
    value: formatNumber(subnetCount.value)
  },
  {
    key: 'avgLeaseTime',
    label: t('dashboard.avgLeaseTime'),
    value: formatHours(avgLeaseHours.value)
  },
  {
    key: 'todayNewLease',
    label: t('dashboard.todayNewLease'),
    value: formatNumber(todayNewLeases.value)
  },
  {
    key: 'leaseSuccessRate',
    label: t('dashboard.leaseSuccessRate'),
    value: `${leaseSuccessRate.value.toFixed(2)}%`
  }
]);

const toErrorDescriptor = (err: unknown) => {
  const mapped = getApiError(err);
  if (mapped) return mapped;
  if (err instanceof Error && err.message) {
    return createInlineError(err.message);
  }
  return createInlineError(t('common.loadFail'));
};

const fetchData = async () => {
  if (!canView.value) return;
  loading.value = true;
  pageError.value = null;
  const tenantId = tenantStore.currentTenantId;
  try {
    const [poolRes, leaseRes, insightsRes, snapRes] = await Promise.all([
      getPoolUsageSummary({ tenantId }),
      getLeaseStatus({ tenantId }),
      getLeaseInsights({ tenantId }),
      getRealtimeSnapshot({ tenantId })
    ]);
    poolSummary.value = poolRes.data.data?.summary || null;
    leaseStatus.value = leaseRes.data.data || null;
    insights.value = insightsRes.data.data || null;
    snapshot.value = snapRes.data.data || null;
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

.kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.kpi-card {
  background: var(--el-fill-color-light);
  border-radius: 10px;
  padding: 14px;
  min-height: 110px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.kpi-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.kpi-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}

.kpi-hint {
  color: var(--el-text-color-placeholder);
  font-size: 12px;
}

@media (max-width: 1200px) {
  .kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .kpi-grid {
    grid-template-columns: 1fr;
  }
}
</style>
