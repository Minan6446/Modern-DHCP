<template>
  <div class="business-monitor page-block">
    <section class="hero-card business-hero">
      <div>
        <p class="eyebrow">{{ t('monitoring.business.heroEyebrow') }}</p>
        <h2>{{ t('monitoring.business.heroTitle') }}</h2>
        <p class="desc">{{ t('monitoring.business.heroDesc') }}</p>
      </div>
      <div class="hero-stats">
        <div v-for="stat in heroStats" :key="stat.key" class="stat-chip">
          <span class="label">{{ stat.label }}</span>
          <span class="value">{{ stat.value }}</span>
          <span class="hint">{{ stat.hint }}</span>
        </div>
      </div>
      <div class="hero-actions">
        <el-button size="small" text class="hero-refresh" :loading="loading" @click="loadData">{{
          t('common.refresh')
        }}</el-button>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.business.utilizationTitle') }}</p>
              <h3>{{ t('monitoring.business.utilizationSubtitle') }}</h3>
            </div>
          </div>
        </template>
        <div class="chart-wrapper">
          <base-e-chart v-if="utilizationChartOption" :option="utilizationChartOption" />
          <el-empty v-else :description="t('monitoring.business.utilizationEmpty')" />
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.business.leaseFunnelTitle') }}</p>
              <h3>{{ t('monitoring.business.leaseFunnelSubtitle') }}</h3>
            </div>
          </div>
        </template>
        <ul class="funnel-list">
          <li v-for="row in leaseFunnel" :key="row.key">
            <div>
              <p class="funnel-title">{{ row.label }}</p>
              <p class="funnel-subtitle">{{ row.hint }}</p>
            </div>
            <div class="funnel-metric">
              <span>{{ row.value }}</span>
              <el-progress
                :percentage="Math.min(100, row.percent)"
                :status="row.status"
                :stroke-width="8"
              />
            </div>
          </li>
        </ul>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('monitoring.business.poolTableTitle') }}</span>
          </div>
        </template>
        <el-table :data="topPools" size="small" border stripe>
          <template v-if="!topPools.length" #empty>
            <el-empty :description="t('monitoring.business.utilizationEmpty')" />
          </template>
          <el-table-column
            prop="poolName"
            :label="t('monitoring.business.poolTableName')"
            min-width="200"
          />
          <el-table-column
            prop="utilization"
            :label="t('monitoring.business.poolTableUsage')"
            width="140"
          >
            <template #default="{ row }">
              <div class="usage-cell">
                <span>{{ row.utilization.toFixed(1) }}%</span>
                <el-progress
                  :percentage="Math.min(100, Math.round(row.utilization))"
                  :stroke-width="6"
                />
              </div>
            </template>
          </el-table-column>
          <el-table-column
            prop="used"
            :label="t('monitoring.business.poolTableCapacity')"
            width="180"
          >
            <template #default="{ row }"
              >{{ row.used.toLocaleString() }} / {{ row.total.toLocaleString() }}</template
            >
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.business.abnormalTitle') }}</p>
              <h3>{{ t('monitoring.business.abnormalSubtitle') }}</h3>
            </div>
          </div>
        </template>
        <el-table :data="abnormalRows" size="small" border>
          <template v-if="!abnormalRows.length" #empty>
            <el-empty :description="t('monitoring.business.abnormalEmpty')" />
          </template>
          <el-table-column
            prop="type"
            :label="t('monitoring.business.abnormalType')"
            min-width="160"
          >
            <template #default="{ row }">
              <el-tag type="warning">{{ row.type }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="count"
            :label="t('monitoring.business.abnormalCount')"
            width="120"
          />
          <el-table-column
            prop="source"
            :label="t('monitoring.business.abnormalSource')"
            min-width="160"
          />
          <el-table-column
            prop="lastSeenAt"
            :label="t('monitoring.business.abnormalLastSeen')"
            min-width="200"
          >
            <template #default="{ row }">{{ formatTs(row.lastSeenAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('monitoring.business.syntheticTitle') }}</span>
          </div>
        </template>
        <div v-if="synthetics.length" class="synthetic-grid">
          <div
            v-for="check in synthetics"
            :key="check.id"
            class="synthetic-item"
            :class="check.status"
          >
            <div class="synthetic-header">
              <span class="name">{{ check.name }}</span>
              <el-tag :type="syntheticType(check.status)">{{
                syntheticLabel(check.status)
              }}</el-tag>
            </div>
            <div class="synthetic-metrics">
              <span>{{ check.latencyMs }} ms</span>
              <small>{{ formatTs(check.lastRunAt) }}</small>
            </div>
          </div>
        </div>
        <el-empty v-else :description="t('monitoring.business.syntheticEmpty')" />
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('monitoring.business.anomalyTitle') }}</span>
          </div>
        </template>
        <div v-if="!anomalies.length" class="empty-block">
          {{ t('monitoring.business.anomalyEmpty') }}
        </div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in anomalies"
            :key="item.id"
            :timestamp="formatTs(item.occurredAt)"
            :type="item.score >= 80 ? 'danger' : 'warning'"
          >
            <p class="timeline-title">{{ item.type }}</p>
            <p class="timeline-desc">{{ item.message }}</p>
          </el-timeline-item>
        </el-timeline>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import {
  listPoolUsage,
  getLeaseStatus,
  listAnomalies,
  listSyntheticChecks,
  listAbnormalRequests
} from '@/api/monitoring';
import type {
  PoolUsageSummary,
  LeaseStatusSummary,
  AnomalyRecord,
  SyntheticCheck,
  AbnormalRequestStat
} from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();
const loading = ref(false);
const poolUsage = ref<PoolUsageSummary[]>([]);
const leaseStatus = ref<LeaseStatusSummary | null>(null);
const anomalies = ref<AnomalyRecord[]>([]);
const synthetics = ref<SyntheticCheck[]>([]);
const abnormalRequests = ref<AbnormalRequestStat[]>([]);

const normalizePercent = (value: number) => (value <= 1 ? value * 100 : value);

const heroStats = computed(() => {
  const status = leaseStatus.value;
  return [
    {
      key: 'active',
      label: t('monitoring.business.statActive'),
      value: status ? status.active.toLocaleString() : '--',
      hint: t('monitoring.business.statActiveHint')
    },
    {
      key: 'pending',
      label: t('monitoring.business.statPending'),
      value: status ? status.pending.toLocaleString() : '--',
      hint: t('monitoring.business.statPendingHint')
    },
    {
      key: 'expired',
      label: t('monitoring.business.statExpired'),
      value: status ? status.expired.toLocaleString() : '--',
      hint: t('monitoring.business.statExpiredHint')
    },
    {
      key: 'failed',
      label: t('monitoring.business.statFailed'),
      value: status ? status.failed.toLocaleString() : '--',
      hint: t('monitoring.business.statFailedHint')
    }
  ];
});

const utilizationChartOption = computed(() => {
  if (!poolUsage.value.length) return null;
  const top = [...poolUsage.value]
    .sort((a, b) => normalizePercent(b.utilization) - normalizePercent(a.utilization))
    .slice(0, 8);
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 80, right: 20, bottom: 20, top: 20 },
    xAxis: { type: 'value', max: 100, axisLabel: { formatter: '{value}%' } },
    yAxis: { type: 'category', data: top.map((item) => item.poolName) },
    series: [
      {
        type: 'bar',
        data: top.map((item) => Number(normalizePercent(item.utilization).toFixed(2))),
        itemStyle: { color: '#6366f1', borderRadius: [4, 4, 4, 4] }
      }
    ]
  };
});

const topPools = computed(() =>
  [...poolUsage.value]
    .sort((a, b) => normalizePercent(b.utilization) - normalizePercent(a.utilization))
    .slice(0, 10)
    .map((item) => ({ ...item, utilization: normalizePercent(item.utilization) }))
);

const leaseFunnel = computed(() => {
  const status = leaseStatus.value;
  if (!status) return [];
  const total = status.active + status.pending + status.expired + status.failed || 1;
  const makeRow = (
    key: string,
    label: string,
    hint: string,
    value: number,
    warnThreshold = 80
  ) => ({
    key,
    label,
    hint,
    value: value.toLocaleString(),
    percent: (value / total) * 100,
    status: (value / total) * 100 >= warnThreshold ? 'warning' : 'success'
  });
  return [
    makeRow(
      'active',
      t('monitoring.business.funnelActive'),
      t('monitoring.business.funnelActiveHint'),
      status.active,
      60
    ),
    makeRow(
      'pending',
      t('monitoring.business.funnelPending'),
      t('monitoring.business.funnelPendingHint'),
      status.pending,
      30
    ),
    makeRow(
      'expired',
      t('monitoring.business.funnelExpired'),
      t('monitoring.business.funnelExpiredHint'),
      status.expired,
      20
    ),
    makeRow(
      'failed',
      t('monitoring.business.funnelFailed'),
      t('monitoring.business.funnelFailedHint'),
      status.failed,
      10
    )
  ];
});

const abnormalRows = computed(() => abnormalRequests.value.slice(0, 8));

const syntheticType = (status: SyntheticCheck['status']) => {
  if (status === 'fail') return 'danger';
  if (status === 'degraded') return 'warning';
  return 'success';
};
const syntheticLabel = (status: SyntheticCheck['status']) => {
  if (status === 'pass') return t('monitoring.business.syntheticPass');
  if (status === 'degraded') return t('monitoring.business.syntheticDegraded');
  return t('monitoring.business.syntheticFail');
};

const loadData = async () => {
  loading.value = true;
  const tenantId = tenantStore.currentTenantId || undefined;
  try {
    const [poolRes, leaseRes, anomalyRes, syntheticRes, abnormalRes] = await Promise.all([
      listPoolUsage({ tenantId }),
      getLeaseStatus({ tenantId }),
      listAnomalies({ tenantId, severity: 'warning' }),
      listSyntheticChecks({ tenantId }),
      listAbnormalRequests({ tenantId })
    ]);
    poolUsage.value = poolRes.data.data || [];
    leaseStatus.value = leaseRes.data.data || null;
    anomalies.value = anomalyRes.data.data?.slice(0, 6) || [];
    synthetics.value = syntheticRes.data.data?.slice(0, 6) || [];
    abnormalRequests.value = abnormalRes.data.data || [];
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    loading.value = false;
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => loadData()
);

onMounted(() => loadData());
</script>

<style scoped>
.business-monitor {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.business-hero {
  background: linear-gradient(120deg, #0f172a, #4338ca);
  color: #fff;
  position: relative;
  padding: 24px 200px 24px 24px;
  border-radius: 20px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.stat-chip {
  background: rgba(15, 23, 42, 0.45);
  border-radius: 12px;
  padding: 12px 14px;
}

.stat-chip .label {
  font-size: 12px;
  opacity: 0.9;
  text-transform: uppercase;
}

.stat-chip .value {
  display: block;
  font-size: 26px;
  font-weight: 600;
}

.stat-chip .hint {
  font-size: 12px;
  opacity: 0.8;
}

.hero-actions {
  position: absolute;
  right: 24px;
  top: 24px;
}

.hero-refresh {
  color: #fff;
  background-color: #38bdf8;
  border-color: #38bdf8;
}

.hero-refresh:hover {
  color: #fff;
  background-color: #0ea5e9;
  border-color: #0ea5e9;
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 18px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-eyebrow {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.chart-wrapper {
  min-height: 280px;
}

.funnel-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.funnel-list li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--el-border-color-light);
}

.funnel-title {
  font-weight: 600;
}

.funnel-subtitle {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.funnel-metric {
  min-width: 200px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-end;
}

.usage-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.synthetic-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
}

.synthetic-item {
  border: 1px solid var(--el-border-color);
  border-radius: 12px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.synthetic-item.fail {
  border-color: var(--el-color-danger);
}

.synthetic-item.degraded {
  border-color: var(--el-color-warning);
}

.synthetic-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.synthetic-metrics {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.synthetic-metrics small {
  color: var(--el-text-color-secondary);
}

.empty-block {
  padding: 24px;
  text-align: center;
  color: var(--el-text-color-secondary);
}

.timeline-title {
  font-weight: 600;
}

.timeline-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

@media (max-width: 960px) {
  .business-hero {
    padding-right: 24px;
  }

  .hero-actions {
    position: static;
    margin-top: 12px;
  }
}
</style>
