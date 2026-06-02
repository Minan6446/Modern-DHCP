<template>
  <div class="realtime-monitor page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('monitoring.realtime.heroEyebrow') }}</p>
        <h2>{{ t('monitoring.realtime.heroTitle') }}</h2>
        <p class="desc">{{ t('monitoring.realtime.heroDesc') }}</p>
        <div class="hero-stats">
          <div v-for="metric in heroMetrics" :key="metric.key" class="stat-chip">
            <span class="label">{{ metric.label }}</span>
            <span class="value">{{ metric.value }}</span>
            <span class="hint">{{ metric.hint }}</span>
          </div>
        </div>
      </div>
      <div class="hero-actions">
        <el-select v-model="range" size="small" class="range-select">
          <el-option
            v-for="opt in rangeOptions"
            :key="opt.value"
            :label="opt.label"
            :value="opt.value"
          />
        </el-select>
        <el-button
          size="small"
          text
          class="hero-refresh"
          :loading="packetLoading"
          @click="fetchPacketSeries"
          >{{ t('common.refresh') }}</el-button
        >
        <el-button
          type="primary"
          size="small"
          class="hero-refresh"
          :loading="loading"
          @click="refreshAll"
          >{{ t('monitoring.realtime.syncNow') }}</el-button
        >
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('monitoring.realtime.trafficTitle') }}</p>
              <h3>
                {{
                  chartMode === 'pps'
                    ? t('monitoring.realtime.ppsSubtitle')
                    : t('monitoring.realtime.bandwidthSubtitle')
                }}
              </h3>
            </div>
            <el-radio-group v-model="chartMode" size="small">
              <el-radio-button label="pps">PPS</el-radio-button>
              <el-radio-button label="bandwidth">{{
                t('monitoring.realtime.bandwidth')
              }}</el-radio-button>
            </el-radio-group>
          </div>
        </template>
        <div class="chart-wrapper">
          <el-skeleton v-if="packetLoading" :rows="6" animated />
          <base-e-chart v-else-if="packetChartOption" :option="packetChartOption" />
          <el-empty v-else :description="t('monitoring.realtime.noTraffic')" />
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <p class="card-eyebrow">{{ t('monitoring.realtime.serverTitle') }}</p>
            <el-button text size="small" :loading="loading" @click="refreshServers">{{
              t('common.refresh')
            }}</el-button>
          </div>
        </template>
        <el-table :data="serverPerf" border stripe size="small">
          <template v-if="!serverPerf.length" #empty>
            <el-empty :description="t('monitoring.realtime.noServerData')" />
          </template>
          <el-table-column
            prop="node"
            :label="t('monitoring.realtime.serverNode')"
            min-width="140"
          />
          <el-table-column
            prop="packetsPerSecond"
            :label="t('monitoring.realtime.serverPps')"
            width="140"
          >
            <template #default="{ row }">{{ formatNumber(row.packetsPerSecond) }} PPS</template>
          </el-table-column>
          <el-table-column
            prop="queueDepth"
            :label="t('monitoring.realtime.serverQueue')"
            width="140"
          />
          <el-table-column
            prop="errorRate"
            :label="t('monitoring.realtime.serverErrors')"
            width="140"
          >
            <template #default="{ row }">
              <el-tag
                :type="row.errorRate >= 1 ? 'danger' : row.errorRate >= 0.5 ? 'warning' : 'success'"
                >{{ row.errorRate.toFixed(2) }}%</el-tag
              >
            </template>
          </el-table-column>
          <el-table-column
            prop="threadUtilization"
            :label="t('monitoring.realtime.serverThreads')"
            width="160"
          >
            <template #default="{ row }">
              <el-progress
                :percentage="Math.round(row.threadUtilization)"
                :status="row.threadUtilization >= 80 ? 'warning' : 'success'"
                :stroke-width="8"
              />
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <p class="card-eyebrow">{{ t('monitoring.realtime.latencyTitle') }}</p>
            <span>{{ t('monitoring.realtime.latencySubtitle') }}</span>
          </div>
        </template>
        <div class="chart-wrapper">
          <el-skeleton v-if="loading && !latencyChartOption" :rows="5" animated />
          <base-e-chart v-else-if="latencyChartOption" :option="latencyChartOption" />
          <el-empty v-else :description="t('monitoring.realtime.noLatency')" />
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <p class="card-eyebrow">{{ t('monitoring.realtime.successTitle') }}</p>
            <span>{{ t('monitoring.realtime.successSubtitle') }}</span>
          </div>
        </template>
        <div class="success-grid">
          <div class="chart-wrapper small">
            <base-e-chart v-if="successChartOption" :option="successChartOption" />
            <el-empty v-else :description="t('monitoring.realtime.noSuccessData')" />
          </div>
          <div class="failure-table">
            <p class="section-title">{{ t('monitoring.realtime.failureBreakdown') }}</p>
            <el-table :data="failureRows" size="small" border>
              <el-table-column
                prop="reason"
                :label="t('monitoring.realtime.failureReason')"
                min-width="160"
              />
              <el-table-column
                prop="count"
                :label="t('monitoring.realtime.failureCount')"
                width="120"
              />
              <el-table-column
                prop="ratio"
                :label="t('monitoring.realtime.failureRatio')"
                width="120"
              >
                <template #default="{ row }">{{ row.ratio.toFixed(1) }}%</template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { useTenantStore } from '@/store/tenant';
import {
  getRealtimeSnapshot,
  getMetricSeries,
  getLatencyDistribution,
  listServerPerformance,
  getAllocationBreakdown
} from '@/api/monitoring';
import type {
  MetricPoint,
  PerfSnapshot,
  ServerPerformanceMetric,
  AllocationBreakdown
} from '@/types/monitoring';

const PACKET_METRICS = [
  {
    key: 'discover',
    metric: 'dhcp.discover.pps',
    bandwidthMetric: 'dhcp.discover.bandwidth',
    labelKey: 'monitoring.realtime.metricDiscover',
    color: '#38bdf8'
  },
  {
    key: 'offer',
    metric: 'dhcp.offer.pps',
    bandwidthMetric: 'dhcp.offer.bandwidth',
    labelKey: 'monitoring.realtime.metricOffer',
    color: '#a855f7'
  },
  {
    key: 'request',
    metric: 'dhcp.request.pps',
    bandwidthMetric: 'dhcp.request.bandwidth',
    labelKey: 'monitoring.realtime.metricRequest',
    color: '#f97316'
  },
  {
    key: 'ack',
    metric: 'dhcp.ack.pps',
    bandwidthMetric: 'dhcp.ack.bandwidth',
    labelKey: 'monitoring.realtime.metricAck',
    color: '#22c55e'
  },
  {
    key: 'nak',
    metric: 'dhcp.nak.pps',
    bandwidthMetric: 'dhcp.nak.bandwidth',
    labelKey: 'monitoring.realtime.metricNak',
    color: '#ef4444'
  }
] as const;

const range = ref<'1m' | '5m' | '15m'>('5m');
const chartMode = ref<'pps' | 'bandwidth'>('pps');
const loading = ref(false);
const packetLoading = ref(false);
const snapshot = ref<PerfSnapshot | null>(null);
const packetSeries = ref<Record<string, MetricPoint[]>>({});
const bandwidthSeries = ref<Record<string, MetricPoint[]>>({});
const latencyBuckets = ref<Array<{ bucket: string; value: number }>>([]);
const allocation = ref<AllocationBreakdown | null>(null);
const serverPerf = ref<ServerPerformanceMetric[]>([]);

const tenantStore = useTenantStore();
const { t } = useI18n();

const rangeOptions = [
  { label: '1 min', value: '1m' },
  { label: '5 min', value: '5m' },
  { label: '15 min', value: '15m' }
];

const heroMetrics = computed(() => [
  {
    key: 'pps',
    label: t('monitoring.realtime.metricPps'),
    value: snapshot.value ? `${snapshot.value.pps.toLocaleString()} PPS` : '--',
    hint: t('monitoring.realtime.metricPpsHint')
  },
  {
    key: 'latency',
    label: t('monitoring.realtime.metricLatency'),
    value: snapshot.value ? `${snapshot.value.latencyP95} ms` : '--',
    hint: t('monitoring.realtime.metricLatencyHint')
  },
  {
    key: 'success',
    label: t('monitoring.realtime.metricSuccess'),
    value: snapshot.value ? `${snapshot.value.successRate}%` : '--',
    hint: t('monitoring.realtime.metricSuccessHint')
  }
]);

const packetChartOption = computed(() => {
  const seriesSource = chartMode.value === 'pps' ? packetSeries.value : bandwidthSeries.value;
  const datasets = PACKET_METRICS.filter((cfg) => seriesSource[cfg.key]?.length).map((cfg) => ({
    name: t(cfg.labelKey as never),
    type: 'line',
    smooth: true,
    showSymbol: false,
    areaStyle: {
      opacity: 0.15
    },
    emphasis: { focus: 'series' },
    itemStyle: { color: cfg.color },
    data: (seriesSource[cfg.key] || []).map((point) => [point.timestamp, point.value])
  }));
  if (!datasets.length) return null;
  return {
    tooltip: { trigger: 'axis' },
    legend: { top: 0 },
    grid: { top: 30, left: 40, right: 20, bottom: 30 },
    xAxis: { type: 'time' },
    yAxis: {
      type: 'value',
      name: chartMode.value === 'pps' ? 'pps' : 'Mbps',
      axisLabel: { formatter: (val: number) => (chartMode.value === 'pps' ? val : `${val} Mbps`) }
    },
    series: datasets
  };
});

const latencyChartOption = computed(() => {
  if (!latencyBuckets.value.length) return null;
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 20, bottom: 30, top: 20 },
    xAxis: { type: 'category', data: latencyBuckets.value.map((item) => item.bucket) },
    yAxis: { type: 'value', name: 'ms' },
    series: [
      {
        type: 'bar',
        data: latencyBuckets.value.map((item) => item.value),
        itemStyle: { color: '#0ea5e9' }
      }
    ]
  };
});

const successChartOption = computed(() => {
  if (!allocation.value) return null;
  const total = allocation.value.total || 1;
  const successRatio = allocation.value.success / total;
  return {
    tooltip: { trigger: 'item' },
    series: [
      {
        name: t('monitoring.realtime.successTitle'),
        type: 'pie',
        radius: ['55%', '80%'],
        avoidLabelOverlap: false,
        label: { show: true, formatter: '{b}: {d}%' },
        data: [
          {
            value: allocation.value.success,
            name: t('monitoring.realtime.successLabel'),
            itemStyle: { color: '#22c55e' }
          },
          {
            value: total - allocation.value.success,
            name: t('monitoring.realtime.failureLabel'),
            itemStyle: { color: '#ef4444' }
          }
        ]
      }
    ]
  };
});

const failureRows = computed(() => {
  if (!allocation.value) return [];
  const totalFailures = allocation.value.failures.reduce((sum, item) => sum + item.count, 0) || 1;
  return allocation.value.failures.map((item) => ({
    reason: t(`monitoring.realtime.failure.${item.reason}` as never, item.reason),
    count: item.count,
    ratio: (item.count / totalFailures) * 100
  }));
});

const formatNumber = (value: number) => value.toLocaleString();

const fetchPacketSeries = async () => {
  packetLoading.value = true;
  const tenantId = tenantStore.currentTenantId || undefined;
  try {
    const responses = await Promise.all(
      PACKET_METRICS.map(async (cfg) => {
        const [ppsRes, bwRes] = await Promise.all([
          getMetricSeries(cfg.metric, { page: 1, pageSize: 60, range: range.value, tenantId }),
          getMetricSeries(cfg.bandwidthMetric, {
            page: 1,
            pageSize: 60,
            range: range.value,
            tenantId
          })
        ]);
        return {
          key: cfg.key,
          pps: ppsRes.data.data || [],
          bandwidth: bwRes.data.data || []
        };
      })
    );
    const nextPps: Record<string, MetricPoint[]> = {};
    const nextBw: Record<string, MetricPoint[]> = {};
    responses.forEach((entry) => {
      nextPps[entry.key] = entry.pps;
      nextBw[entry.key] = entry.bandwidth;
    });
    packetSeries.value = nextPps;
    bandwidthSeries.value = nextBw;
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    packetLoading.value = false;
  }
};

const fetchSnapshot = async () => {
  loading.value = true;
  const tenantId = tenantStore.currentTenantId || undefined;
  try {
    const [snapshotRes, latencyRes, allocationRes, serverRes] = await Promise.all([
      getRealtimeSnapshot({ tenantId }),
      getLatencyDistribution({ tenantId }),
      getAllocationBreakdown({ tenantId }),
      listServerPerformance({ tenantId })
    ]);
    snapshot.value = snapshotRes.data.data;
    latencyBuckets.value = latencyRes.data.data || [];
    allocation.value = allocationRes.data.data || null;
    serverPerf.value = serverRes.data.data || [];
  } catch (error) {
    showHttpError(error, t('monitoring.loadFail'));
  } finally {
    loading.value = false;
  }
};

const refreshServers = () => {
  fetchSnapshot();
};

const refreshAll = async () => {
  await Promise.all([fetchSnapshot(), fetchPacketSeries()]);
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    refreshAll();
  }
);

watch(range, () => {
  fetchPacketSeries();
});

onMounted(() => {
  refreshAll();
});
</script>

<style scoped>
.realtime-monitor {
  --hero-grad-start: #0f172a;
  --hero-grad-end: #0369a1;
  --hero-text: #ffffff;
  --hero-chip-bg: rgba(15, 23, 42, 0.35);
  --hero-btn-bg: #38bdf8;
  --hero-btn-hover: #0ea5e9;
  --text-secondary: #6b7280;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-refresh {
  color: var(--hero-text);
  background-color: var(--hero-btn-bg);
  border-color: var(--hero-btn-bg);
}

.hero-refresh:hover {
  color: var(--hero-text);
  background-color: var(--hero-btn-hover);
  border-color: var(--hero-btn-hover);
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(120deg, var(--hero-grad-start), var(--hero-grad-end));
  color: var(--hero-text);
}

.eyebrow {
  text-transform: uppercase;
  font-size: 12px;
  letter-spacing: 1px;
  opacity: 0.8;
}

.hero-stats {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 16px;
}

.stat-chip {
  padding: 12px 16px;
  border-radius: 12px;
  background: var(--hero-chip-bg);
  min-width: 160px;
}

.stat-chip .label {
  font-size: 12px;
  opacity: 0.9;
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
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: flex-end;
}

.range-select {
  min-width: 120px;
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
  color: var(--text-secondary);
}

.chart-wrapper {
  min-height: 260px;
}

.chart-wrapper.small {
  min-height: 220px;
}

.success-grid {
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: 16px;
  align-items: flex-start;
}

.failure-table {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-title {
  font-weight: 600;
}

@media (max-width: 960px) {
  .hero-card {
    flex-direction: column;
  }

  .success-grid {
    grid-template-columns: 1fr;
  }
}
</style>
