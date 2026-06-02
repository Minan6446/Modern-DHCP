<template>
  <div class="monitor-overview page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('monitoring.overview.heroEyebrow') }}</p>
        <h2>{{ t('monitoring.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('monitoring.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/monitoring/realtime')">{{
            t('monitoring.overview.actionPrimary')
          }}</el-button>
          <el-button text size="large" @click="goto('/monitoring/alerts')">{{
            t('monitoring.overview.actionSecondary')
          }}</el-button>
        </div>
      </div>
      <div class="hero-metrics">
        <div v-for="metric in heroMetrics" :key="metric.key" class="metric">
          <span class="metric-label">{{ metric.label }}</span>
          <span class="metric-value">{{ metric.value }}</span>
          <span class="metric-hint">{{ metric.hint }}</span>
        </div>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('monitoring.overview.performanceTitle') }}</span>
            <el-button text size="small" :loading="loading" @click="loadData">{{
              t('common.refresh')
            }}</el-button>
          </div>
        </template>
        <div class="perf-grid">
          <div v-for="row in perfRows" :key="row.key" class="perf-item">
            <div class="perf-header">
              <span>{{ row.label }}</span>
              <span>{{ row.value }}</span>
            </div>
            <el-progress :percentage="row.percent" :status="row.status" :stroke-width="10" />
          </div>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('monitoring.overview.syntheticTitle') }}</span>
        </template>
        <el-table :data="synthetics" size="small" border stripe>
          <template v-if="!synthetics.length" #empty>
            <el-empty :description="t('monitoring.overview.syntheticEmpty')" />
          </template>
          <el-table-column
            prop="name"
            :label="t('monitoring.overview.checkName')"
            min-width="200"
          />
          <el-table-column prop="status" :label="t('monitoring.overview.checkStatus')" width="140">
            <template #default="{ row }">
              <el-tag :type="checkType(row.status)">{{ checkLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="latencyMs" :label="t('monitoring.overview.latency')" width="140" />
          <el-table-column
            prop="lastRunAt"
            :label="t('monitoring.overview.lastRun')"
            min-width="200"
          >
            <template #default="{ row }">{{ formatTs(row.lastRunAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('monitoring.overview.alertsTitle') }}</span>
        </template>
        <el-table :data="alerts" size="small" border stripe>
          <template v-if="!alerts.length" #empty>
            <el-empty :description="t('monitoring.overview.alertsEmpty')" />
          </template>
          <el-table-column prop="severity" :label="t('monitoring.overview.severity')" width="120">
            <template #default="{ row }">
              <el-tag :type="severityType(row.severity)">{{ row.severity }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="message"
            :label="t('monitoring.overview.alertMessage')"
            min-width="240"
          />
          <el-table-column
            prop="status"
            :label="t('monitoring.overview.alertStatus')"
            width="120"
          />
          <el-table-column
            prop="createdAt"
            :label="t('monitoring.overview.detectedAt')"
            min-width="200"
          >
            <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('monitoring.overview.anomaliesTitle') }}</span>
        </template>
        <div v-if="!anomalies.length" class="empty-block">
          {{ t('monitoring.overview.anomaliesEmpty') }}
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
import { useRouter } from 'vue-router';
import {
  getRealtimeSnapshot,
  listAlertEvents,
  listAnomalies,
  listSyntheticChecks
} from '@/api/monitoring';
import type { AlertEvent, AnomalyRecord, PerfSnapshot, SyntheticCheck } from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const router = useRouter();
const tenantStore = useTenantStore();
const loading = ref(false);
const snapshot = ref<PerfSnapshot | null>(null);
const alerts = ref<AlertEvent[]>([]);
const anomalies = ref<AnomalyRecord[]>([]);
const synthetics = ref<SyntheticCheck[]>([]);

const heroMetrics = computed(() => [
  {
    key: 'pps',
    label: t('monitoring.overview.metricPps'),
    value: snapshot.value?.pps ? `${snapshot.value.pps.toLocaleString()} pps` : '--',
    hint: t('monitoring.overview.metricPpsHint')
  },
  {
    key: 'latency',
    label: t('monitoring.overview.metricLatency'),
    value: snapshot.value ? `${snapshot.value.latencyP95} ms` : '--',
    hint: t('monitoring.overview.metricLatencyHint')
  },
  {
    key: 'success',
    label: t('monitoring.overview.metricSuccess'),
    value: snapshot.value ? `${snapshot.value.successRate}%` : '--',
    hint: t('monitoring.overview.metricSuccessHint')
  }
]);

const perfRows = computed(() => {
  if (!snapshot.value) return [];
  return [
    {
      key: 'cpu',
      label: 'CPU',
      value: `${snapshot.value.cpu}%`,
      percent: snapshot.value.cpu,
      status: snapshot.value.cpu >= 80 ? 'exception' : 'success'
    },
    {
      key: 'memory',
      label: t('monitoring.overview.memory'),
      value: `${snapshot.value.memory}%`,
      percent: snapshot.value.memory,
      status: snapshot.value.memory >= 80 ? 'warning' : 'success'
    },
    {
      key: 'latency',
      label: t('monitoring.overview.latencyP99'),
      value: `${snapshot.value.latencyP99} ms`,
      percent: Math.min(100, Math.round((snapshot.value.latencyP99 / 10) * 100)),
      status: snapshot.value.latencyP99 >= 12 ? 'exception' : 'success'
    },
    {
      key: 'success',
      label: t('monitoring.overview.successRate'),
      value: `${snapshot.value.successRate}%`,
      percent: snapshot.value.successRate,
      status: snapshot.value.successRate < 98 ? 'exception' : 'success'
    }
  ];
});

const severityType = (severity: AlertEvent['severity']) => {
  if (severity === 'critical') return 'danger';
  if (severity === 'warning') return 'warning';
  return 'info';
};

const checkType = (status: SyntheticCheck['status']) => {
  if (status === 'fail') return 'danger';
  if (status === 'degraded') return 'warning';
  return 'success';
};

const checkLabel = (status: SyntheticCheck['status']) => {
  if (status === 'pass') return t('monitoring.overview.checkPass');
  if (status === 'degraded') return t('monitoring.overview.checkDegraded');
  return t('monitoring.overview.checkFail');
};

const goto = (path: string) => router.push(path);

const loadData = async () => {
  loading.value = true;
  try {
    const tenantId = tenantStore.currentTenantId || undefined;
    const [snapshotRes, alertRes, anomalyRes, syntheticRes] = await Promise.all([
      getRealtimeSnapshot({ tenantId }),
      listAlertEvents({ tenantId, page: 1, pageSize: 5 }),
      listAnomalies({ tenantId, severity: 'warning' }),
      listSyntheticChecks({ tenantId })
    ]);
    snapshot.value = snapshotRes.data.data;
    const alertData = alertRes.data.data as any;
    alerts.value = Array.isArray(alertData.items) ? alertData.items : alertData;
    anomalies.value = anomalyRes.data.data?.slice(0, 6) || [];
    synthetics.value = syntheticRes.data.data?.slice(0, 6) || [];
  } finally {
    loading.value = false;
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => loadData()
);

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.monitor-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(120deg, #1e293b, #0ea5e9);
  color: #fff;
  display: flex;
  justify-content: space-between;
  gap: 24px;
}

.hero-actions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  min-width: 320px;
}

.metric {
  background: rgba(15, 23, 42, 0.5);
  border-radius: 14px;
  padding: 14px;
}

.metric-label {
  font-size: 12px;
  opacity: 0.7;
  text-transform: uppercase;
}

.metric-value {
  display: block;
  font-size: 28px;
  font-weight: 600;
}

.metric-hint {
  font-size: 12px;
  opacity: 0.7;
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.perf-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.perf-item {
  background: var(--el-fill-color-light);
  border-radius: 12px;
  padding: 12px 16px;
}

.perf-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-weight: 600;
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
  .hero-card {
    flex-direction: column;
  }
}
</style>
