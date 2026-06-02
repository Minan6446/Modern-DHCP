<template>
  <div class="dashboard-page">
    <section class="alert-strip">
      <el-alert
        v-if="!alertItems.length"
        type="success"
        :closable="false"
        show-icon
        :title="t('dashboard.noHighAlert')"
      />
      <div v-else class="alert-list">
        <div v-for="alert in alertItems" :key="alert.id" class="alert-item" :class="alert.levelClass">
          <div class="alert-main">
            <div class="alert-title">
              <el-tag size="small" :type="alert.tagType">{{ alert.levelText }}</el-tag>
              <span>{{ alert.title }}</span>
            </div>
            <div class="alert-desc">{{ alert.message }}</div>
          </div>
          <div class="alert-actions">
            <el-button size="small" @click="goAlertDetail(alert)">{{ t('dashboard.viewDetail') }}</el-button>
            <el-button size="small" type="primary" @click="handleAlert(alert)">{{ t('dashboard.quickHandle') }}</el-button>
          </div>
        </div>
      </div>
    </section>

    <section class="core-status-grid">
      <el-card shadow="never" class="section-card equal-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.serverHealthTitle') }}</span>
            <el-button text size="small" @click="goTo('/monitor/overview')">{{ t('dashboard.detail') }}</el-button>
          </div>
        </template>
        <div class="server-grid">
          <div v-for="item in serverStatusCards" :key="item.key" class="server-card">
            <div class="server-label">{{ item.label }}</div>
            <div class="server-value" :class="{ danger: item.value >= 85 }">{{ item.value.toFixed(1) }}%</div>
            <div class="server-sub">{{ item.sub }}</div>
            <el-progress
              :percentage="Math.min(100, Math.max(0, item.value))"
              :stroke-width="8"
              :show-text="false"
              :color="progressColor(item.value)"
            />
          </div>
        </div>
      </el-card>

      <el-card shadow="never" class="section-card equal-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.kpiTitle') }}</span>
            <el-button text size="small" @click="goTo('/pool/analytics')">{{ t('dashboard.detail') }}</el-button>
          </div>
        </template>
        <div class="kpi-grid">
          <div v-for="kpi in kpiCards" :key="kpi.key" class="kpi-card" @click="goTo(kpi.route)">
            <div class="kpi-label">{{ kpi.label }}</div>
            <div class="kpi-value" :class="{ danger: kpi.danger }">{{ kpi.value }}</div>
            <div class="kpi-sub">{{ kpi.sub }}</div>
          </div>
        </div>
      </el-card>
    </section>

    <section class="trend-grid">
      <el-card shadow="never" class="section-card equal-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.ipUsageTrend') }}</span>
            <el-radio-group v-model="timeWindow" size="small">
              <el-radio-button label="3d">{{ t('dashboard.days3') }}</el-radio-button>
              <el-radio-button label="7d">{{ t('dashboard.days7') }}</el-radio-button>
              <el-radio-button label="30d">{{ t('dashboard.days30') }}</el-radio-button>
            </el-radio-group>
          </div>
        </template>
        <el-empty v-if="!ipUsageTrend.length" :image-size="60" :description="t('dashboard.noTrendData')" />
        <BaseEChart v-else :option="ipUsageOption" class="trend-chart" />
      </el-card>

      <el-card shadow="never" class="section-card equal-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.leaseReqTrend') }}</span>
            <el-radio-group v-model="timeWindow" size="small">
              <el-radio-button label="3d">{{ t('dashboard.days3') }}</el-radio-button>
              <el-radio-button label="7d">{{ t('dashboard.days7') }}</el-radio-button>
              <el-radio-button label="30d">{{ t('dashboard.days30') }}</el-radio-button>
            </el-radio-group>
          </div>
        </template>
        <el-empty v-if="!leaseReqTrend.length" :image-size="60" :description="t('dashboard.noLeaseReqTrend')" />
        <BaseEChart v-else :option="leaseReqOption" class="trend-chart" />
      </el-card>
    </section>

    <section class="insight-grid">
      <el-card shadow="never" class="section-card insight-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.hotPoolTitle') }}</span>
            <el-button text size="small" @click="goTo('/pool/analytics')">{{ t('dashboard.viewMore') }}</el-button>
          </div>
        </template>
        <el-empty v-if="!topPools.length" :image-size="60" :description="t('dashboard.noHotPool')">
          <template #extra>
            <el-button size="small" @click="refreshAll">{{ t('dashboard.refresh') }}</el-button>
          </template>
        </el-empty>
        <BaseEChart v-else :option="topPoolOption" class="insight-chart" />
      </el-card>

      <el-card shadow="never" class="section-card insight-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.leaseStatusDist') }}</span>
            <el-button text size="small" @click="goTo('/lease/active')">{{ t('dashboard.viewMore') }}</el-button>
          </div>
        </template>
        <el-empty v-if="!leaseStatusData.length" :image-size="60" :description="t('dashboard.noLeaseStatusData')">
          <template #extra>
            <el-button size="small" @click="goTo('/lease/active')">{{ t('dashboard.quickGo') }}</el-button>
          </template>
        </el-empty>
        <BaseEChart v-else :option="leaseStatusOption" class="insight-chart" />
      </el-card>

      <el-card shadow="never" class="section-card insight-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.anomalyTypeTitle') }}</span>
            <el-button text size="small" @click="goTo('/monitor/history')">{{ t('dashboard.viewMore') }}</el-button>
          </div>
        </template>
        <el-empty v-if="!anomalyTypeData.length" :image-size="60" :description="t('dashboard.noAnomalyData')">
          <template #extra>
            <el-button size="small" @click="goTo('/monitor/config')">{{ t('dashboard.goConfigAlert') }}</el-button>
          </template>
        </el-empty>
        <BaseEChart v-else :option="anomalyOption" class="insight-chart" />
      </el-card>

      <el-card shadow="never" class="section-card insight-card">
        <template #header>
          <div class="section-head">
            <span>{{ t('dashboard.severityTitle') }}</span>
            <el-button text size="small" @click="goTo('/monitor/history')">{{ t('dashboard.viewMore') }}</el-button>
          </div>
        </template>
        <el-empty v-if="!severityData.length" :image-size="60" :description="t('dashboard.noSeverityData')">
          <template #extra>
            <el-button size="small" @click="goTo('/monitor/overview')">{{ t('dashboard.quickGo') }}</el-button>
          </template>
        </el-empty>
        <BaseEChart v-else :option="severityOption" class="insight-chart" />
      </el-card>
    </section>

    <AppErrorCallout v-if="pageError" :error="pageError" class="mt-12">
      <template #actions>
        <el-button size="small" :loading="loading" @click="refreshAll">{{ t('dashboard.retry') }}</el-button>
      </template>
    </AppErrorCallout>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessageBox } from 'element-plus';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import {
  getRealtimeSnapshot,
  getPoolUsageSummary,
  getLeaseStatus,
  listPoolUsage,
  listAnomalies,
  listAlertEvents,
  getMetricSeries
} from '@/api/monitoring';
import type { AlertEvent, AnomalyRecord, LeaseStatusSummary, MetricPoint, PerfSnapshot, PoolUsageSummary, PoolUsageSummarySnapshot } from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { useI18n } from 'vue-i18n';
import { showSuccess } from '@/shared/errors/messageToast';
import { createInlineError, getApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const router = useRouter();
const tenantStore = useTenantStore();
const { t } = useI18n();

const loading = ref(false);
const pageError = ref<ApiErrorDescriptor | null>(null);

const alerts = ref<AlertEvent[]>([]);
const snapshot = ref<PerfSnapshot | null>(null);
const poolSummary = ref<PoolUsageSummarySnapshot | null>(null);
const leaseStatus = ref<LeaseStatusSummary | null>(null);
const poolUsage = ref<PoolUsageSummary[]>([]);
const anomalies = ref<AnomalyRecord[]>([]);

const timeWindow = ref<'3d' | '7d' | '30d'>('7d');
const ipUsageTrend = ref<MetricPoint[]>([]);
const leaseReqTrend = ref<MetricPoint[]>([]);

const palette = computed(() => ({
  textPrimary: '#1D2129',
  textRegular: '#4E5969',
  textSecondary: '#86909C',
  border: '#E5E6EB',
  splitLine: '#EEF1F6',
  cardBg: '#FFFFFF',
  pageBg: '#F5F7FA',
  primary: '#165DFF',
  success: '#00B42A',
  warning: '#FF7D00',
  danger: '#F53F3F',
  info: '#86909C'
}));

const progressColor = (value: number) => {
  if (value >= 85) return palette.value.danger;
  if (value >= 70) return palette.value.warning;
  return palette.value.success;
};

const formatBytes = (bytes?: number) => {
  if (!bytes || bytes <= 0) return '--';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx += 1;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[idx]}`;
};

const memoryTotal = computed(() => {
  const total = snapshot.value?.memoryTotalBytes;
  if (total && total > 0) return total;
  const used = snapshot.value?.memoryUsedBytes;
  const percent = snapshot.value?.memory || 0;
  if (used && percent > 0) return Math.round((used * 100) / percent);
  return undefined;
});

const diskTotal = computed(() => {
  const total = snapshot.value?.diskTotalBytes;
  if (total && total > 0) return total;
  const used = snapshot.value?.diskUsedBytes;
  const percent = snapshot.value?.disk || 0;
  if (used && percent > 0) return Math.round((used * 100) / percent);
  return undefined;
});

const serverStatusCards = computed(() => [
  {
    key: 'cpu',
    label: t('dashboard.cpuUsageLabel'),
    value: Number(snapshot.value?.cpu || 0),
    sub: t('dashboard.cpuCoresSub', { value: snapshot.value?.cpuCores ?? '--' })
  },
  {
    key: 'memory',
    label: t('dashboard.memoryUsageLabel'),
    value: Number(snapshot.value?.memory || 0),
    sub: t('dashboard.totalSub', { value: formatBytes(memoryTotal.value) })
  },
  {
    key: 'disk',
    label: t('dashboard.diskUsageLabel'),
    value: Number(snapshot.value?.disk || 0),
    sub: t('dashboard.totalSub', { value: formatBytes(diskTotal.value) })
  }
]);

const kpiCards = computed(() => {
  const successRate = Number((snapshot.value?.successRate ?? 0) * 100);
  const latencyP95 = Number(snapshot.value?.latencyP95 ?? 0);
  const activeLeases = Number(leaseStatus.value?.active ?? 0);
  const availableIps = Number(poolSummary.value?.availableIps ?? 0);
  return [
    {
      key: 'successRate',
      label: t('dashboard.successRateLabel'),
      value: `${successRate.toFixed(2)}%`,
      sub: t('dashboard.goToLease'),
      route: '/lease/active',
      danger: successRate < 85
    },
    {
      key: 'latency',
      label: t('dashboard.latencyLabel'),
      value: `${latencyP95.toFixed(1)} ms`,
      sub: t('dashboard.goToMonitor'),
      route: '/monitor/overview',
      danger: latencyP95 > 120
    },
    {
      key: 'active',
      label: t('dashboard.activeLeaseLabel'),
      value: activeLeases.toLocaleString(),
      sub: t('dashboard.goToLeaseHistory'),
      route: '/lease/history',
      danger: false
    },
    {
      key: 'available',
      label: t('dashboard.availableIpLabel'),
      value: availableIps.toLocaleString(),
      sub: t('dashboard.goToPool'),
      route: '/pool/ipv4',
      danger: availableIps <= 100
    }
  ];
});

const topPools = computed(() => [...poolUsage.value].sort((a, b) => b.utilization - a.utilization).slice(0, 8));

const leaseStatusData = computed(() => {
  const source = leaseStatus.value;
  if (!source) return [];
  return [
    { name: t('dashboard.stateActive'), value: Number(source.active || 0) },
    { name: t('dashboard.statePending'), value: Number(source.pending || 0) },
    { name: t('dashboard.stateExpired'), value: Number(source.expired || 0) },
    { name: t('dashboard.stateFailed'), value: Number(source.failed || 0) }
  ].filter((item) => item.value > 0);
});

const anomalyTypeData = computed(() => {
  const buckets: Record<string, number> = {};
  anomalies.value.forEach((item) => {
    const key = item.type || 'unknown';
    buckets[key] = (buckets[key] || 0) + 1;
  });
  return Object.entries(buckets).map(([name, value]) => ({ name, value }));
});

const severityData = computed(() => {
  const buckets = {
    emergency: 0,
    critical: 0,
    warning: 0,
    info: 0
  };
  alerts.value.forEach((item) => {
    const level = String(item.severity || '').toLowerCase();
    if (level in buckets) {
      (buckets as Record<string, number>)[level] += 1;
    }
  });
  return Object.entries(buckets)
    .map(([name, value]) => ({ name, value }))
    .filter((item) => item.value > 0);
});

const alertItems = computed(() =>
  alerts.value.slice(0, 4).map((item) => {
    const severity = String(item.severity || 'info').toLowerCase();
    const isHigh = severity === 'emergency' || severity === 'critical';
    return {
      id: item.id,
      title: isHigh ? t('dashboard.alertHigh') : t('dashboard.alertMid'),
      message: item.message,
      levelText: isHigh ? t('dashboard.levelHigh') : t('dashboard.levelMid'),
      levelClass: isHigh ? 'high' : 'mid',
      tagType: isHigh ? 'danger' : 'warning',
      severity,
      raw: item
    };
  })
);

const toXAxisLabel = (value: string) => {
  const text = String(value || '');
  return text.length > 16 ? text.slice(5, 16) : text;
};

const ipUsageOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  textStyle: { color: palette.value.textRegular },
  grid: { left: 24, right: 18, top: 32, bottom: 24 },
  xAxis: {
    type: 'category',
    data: ipUsageTrend.value.map((item) => toXAxisLabel(item.timestamp)),
    axisLine: { lineStyle: { color: palette.value.border } },
    axisLabel: { color: palette.value.textSecondary }
  },
  yAxis: {
    type: 'value',
    axisLabel: { formatter: '{value}%', color: palette.value.textSecondary },
    splitLine: { lineStyle: { color: palette.value.splitLine } }
  },
  series: [
    {
      name: t('dashboard.seriesUsage'),
      type: 'line',
      smooth: true,
      data: ipUsageTrend.value.map((item) => ({
        value: Number(item.value || 0).toFixed(1),
        symbolSize: Number(item.value || 0) >= 85 ? 10 : 6,
        itemStyle: Number(item.value || 0) >= 85 ? { color: palette.value.danger } : undefined
      })),
      lineStyle: { width: 2, color: palette.value.primary }
    }
  ]
}));

const leaseReqOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  textStyle: { color: palette.value.textRegular },
  grid: { left: 24, right: 18, top: 32, bottom: 24 },
  xAxis: {
    type: 'category',
    data: leaseReqTrend.value.map((item) => toXAxisLabel(item.timestamp)),
    axisLine: { lineStyle: { color: palette.value.border } },
    axisLabel: { color: palette.value.textSecondary }
  },
  yAxis: {
    type: 'value',
    axisLabel: { color: palette.value.textSecondary },
    splitLine: { lineStyle: { color: palette.value.splitLine } }
  },
  series: [
    {
      name: t('dashboard.seriesRequests'),
      type: 'line',
      smooth: true,
      data: leaseReqTrend.value.map((item) => ({
        value: Number(item.value || 0),
        symbolSize: Number(item.value || 0) >= 1000 ? 10 : 6,
        itemStyle: Number(item.value || 0) >= 1000 ? { color: palette.value.danger } : undefined
      })),
      lineStyle: { width: 2, color: palette.value.success }
    }
  ]
}));

const topPoolOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  textStyle: { color: palette.value.textRegular },
  legend: { data: [t('dashboard.seriesUsage')], textStyle: { color: palette.value.textSecondary } },
  grid: { left: 28, right: 18, top: 32, bottom: 28 },
  xAxis: {
    type: 'category',
    data: topPools.value.map((item) => item.poolName),
    axisLabel: { interval: 0, rotate: 20, color: palette.value.textSecondary },
    axisLine: { lineStyle: { color: palette.value.border } }
  },
  yAxis: {
    type: 'value',
    axisLabel: { formatter: '{value}%', color: palette.value.textSecondary },
    splitLine: { lineStyle: { color: palette.value.splitLine } }
  },
  series: [
    {
      type: 'bar',
      name: t('dashboard.seriesUsage'),
      label: { show: true, position: 'top', formatter: '{c}%' },
      data: topPools.value.map((item) => ({
        value: Number(item.utilization || 0).toFixed(1),
        itemStyle: { color: Number(item.utilization || 0) >= 85 ? palette.value.danger : palette.value.primary }
      }))
    }
  ]
}));

const leaseStatusOption = computed(() => ({
  tooltip: { trigger: 'item' },
  textStyle: { color: palette.value.textRegular },
  legend: { bottom: 0, textStyle: { color: palette.value.textSecondary } },
  color: [palette.value.primary, palette.value.success, palette.value.warning, palette.value.danger],
  series: [
    {
      type: 'pie',
      radius: ['45%', '68%'],
      center: ['50%', '44%'],
      label: { formatter: '{b}: {d}%' },
      data: leaseStatusData.value
    }
  ]
}));

const anomalyOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  textStyle: { color: palette.value.textRegular },
  legend: { data: [t('dashboard.seriesAnomalies')], textStyle: { color: palette.value.textSecondary } },
  grid: { left: 26, right: 18, top: 32, bottom: 26 },
  xAxis: {
    type: 'category',
    data: anomalyTypeData.value.map((item) => item.name),
    axisLabel: { interval: 0, rotate: 20, color: palette.value.textSecondary },
    axisLine: { lineStyle: { color: palette.value.border } }
  },
  yAxis: {
    type: 'value',
    minInterval: 1,
    axisLabel: { color: palette.value.textSecondary },
    splitLine: { lineStyle: { color: palette.value.splitLine } }
  },
  series: [
    {
      type: 'bar',
      name: t('dashboard.seriesAnomalies'),
      label: { show: true, position: 'top' },
      data: anomalyTypeData.value.map((item) => ({
        value: item.value,
        itemStyle: { color: item.value >= 5 ? palette.value.danger : palette.value.warning }
      }))
    }
  ]
}));

const severityOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  textStyle: { color: palette.value.textRegular },
  legend: { data: [t('dashboard.seriesAlertCount')], textStyle: { color: palette.value.textSecondary } },
  grid: { left: 26, right: 18, top: 32, bottom: 26 },
  xAxis: {
    type: 'category',
    data: severityData.value.map((item) => item.name),
    axisLine: { lineStyle: { color: palette.value.border } },
    axisLabel: { color: palette.value.textSecondary }
  },
  yAxis: {
    type: 'value',
    minInterval: 1,
    axisLabel: { color: palette.value.textSecondary },
    splitLine: { lineStyle: { color: palette.value.splitLine } }
  },
  series: [
    {
      type: 'line',
      smooth: true,
      name: t('dashboard.seriesAlertCount'),
      label: { show: true },
      data: severityData.value.map((item) => ({
        value: item.value,
        symbolSize: item.name === 'critical' || item.name === 'emergency' ? 10 : 6,
        itemStyle:
          item.name === 'critical' || item.name === 'emergency' ? { color: palette.value.danger } : { color: palette.value.primary }
      })),
      lineStyle: { width: 2, color: palette.value.warning }
    }
  ]
}));

const goTo = (path: string) => {
  router.push(path);
};

const goAlertDetail = (alert: { raw: AlertEvent }) => {
  router.push({ path: '/monitor/history', query: { id: alert.raw.id } });
};

const handleAlert = async (alert: { raw: AlertEvent }) => {
  await ElMessageBox.confirm(t('dashboard.handleAlertConfirm'), t('dashboard.handleAlertTitle'), {
    type: 'warning'
  });
  router.push({ path: '/monitor/config', query: { eventId: alert.raw.id } });
  showSuccess(t('dashboard.handleAlertSuccess'));
};

const normalizeMetric = (items: MetricPoint[]) =>
  (items || []).map((item) => ({
    timestamp: item.timestamp,
    value: Number(item.value || 0)
  }));

const fetchTrendData = async () => {
  const tenantId = tenantStore.currentTenantId;
  const params = {
    page: 1,
    pageSize: 120,
    range: timeWindow.value,
    tenantId
  };
  const [ipRes, reqRes] = await Promise.allSettled([
    getMetricSeries('ip-usage-rate', params),
    getMetricSeries('dhcp-lease-requests', params)
  ]);

  if (ipRes.status === 'fulfilled') {
    ipUsageTrend.value = normalizeMetric(ipRes.value.data.data || []);
  } else {
    ipUsageTrend.value = [];
  }

  if (reqRes.status === 'fulfilled') {
    leaseReqTrend.value = normalizeMetric(reqRes.value.data.data || []);
  } else {
    leaseReqTrend.value = [];
  }
};

const refreshAll = async () => {
  loading.value = true;
  pageError.value = null;
  const tenantId = tenantStore.currentTenantId;
  try {
    const [alertRes, perfRes, poolSumRes, leaseRes, poolUsageRes, anomalyRes] = await Promise.all([
      listAlertEvents({ page: 1, pageSize: 20, status: 'firing', tenantId }),
      getRealtimeSnapshot({ tenantId }),
      getPoolUsageSummary({ tenantId }),
      getLeaseStatus({ tenantId }),
      listPoolUsage({ tenantId }),
      listAnomalies({ tenantId })
    ]);

    alerts.value = alertRes.data.data.items || [];
    snapshot.value = perfRes.data.data || null;
    poolSummary.value = poolSumRes.data.data?.summary || null;
    leaseStatus.value = leaseRes.data.data || null;
    poolUsage.value = poolUsageRes.data.data || [];
    anomalies.value = anomalyRes.data.data || [];

    await fetchTrendData();
  } catch (error) {
    pageError.value = getApiError(error) || createInlineError(t('dashboard.loadFail'));
  } finally {
    loading.value = false;
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => refreshAll()
);

watch(timeWindow, () => {
  fetchTrendData().catch(() => undefined);
});

onMounted(() => {
  refreshAll();
});
</script>

<style scoped>
.dashboard-page {
  --dash-page-bg: #f5f7fa;
  --dash-surface-bg: #ffffff;
  --dash-surface-strong: #0f172a;
  --dash-border: #e5e7eb;
  --dash-border-soft: #eef1f6;
  --dash-text-primary: #111827;
  --dash-text-regular: #4b5563;
  --dash-text-secondary: #6b7280;
  --dash-alert-bg: #ffffff;
  --dash-alert-high-bg: #fef2f2;
  --dash-alert-high-border: #fecaca;
  --dash-alert-mid-bg: #fffbeb;
  --dash-alert-mid-border: #fde68a;
  --dash-danger: #f53f3f;
  --dash-primary: #165dff;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 100%;
  padding: 16px;
  background: var(--dash-page-bg);
}

.alert-strip {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.alert-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.alert-item {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 12px;
  align-items: center;
  border-radius: 10px;
  padding: 10px 12px;
  border: 1px solid var(--dash-border);
  background: var(--dash-alert-bg);
}

.alert-item.high {
  background: var(--dash-alert-high-bg);
  border-color: var(--dash-alert-high-border);
}

.alert-item.mid {
  background: var(--dash-alert-mid-bg);
  border-color: var(--dash-alert-mid-border);
}

.alert-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.alert-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: var(--dash-text-primary);
}

.alert-desc {
  color: var(--dash-text-regular);
  font-size: 13px;
}

.alert-actions {
  display: flex;
  gap: 8px;
}

.core-status-grid,
.trend-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.section-card {
  border: 1px solid var(--dash-border);
  border-radius: 10px;
}

.section-card :deep(.el-card__header) {
  padding: 12px 14px;
  border-bottom: 1px solid var(--dash-border-soft);
}

.section-card :deep(.el-card__body) {
  padding: 14px;
}

.equal-card {
  min-height: 300px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-weight: 600;
  color: var(--dash-text-primary);
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.server-card {
  border: 1px solid var(--dash-border-soft);
  background: var(--dash-surface-bg);
  border-radius: 8px;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.server-label {
  color: var(--dash-text-secondary);
  font-size: 13px;
}

.server-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--dash-text-primary);
}

.server-value.danger {
  color: var(--dash-danger);
}

.server-sub {
  color: var(--dash-text-regular);
  font-size: 12px;
}

.kpi-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.kpi-card {
  border: 1px solid var(--dash-border-soft);
  background: var(--dash-surface-bg);
  border-radius: 8px;
  padding: 10px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.kpi-label {
  color: var(--dash-text-secondary);
  font-size: 13px;
}

.kpi-value {
  color: var(--dash-text-primary);
  font-size: 22px;
  font-weight: 700;
}

.kpi-value.danger {
  color: var(--dash-danger);
}

.kpi-sub {
  color: var(--dash-text-regular);
  font-size: 12px;
}

.trend-chart,
.insight-chart {
  height: 260px;
}

.insight-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.insight-card {
  min-height: 320px;
}

.mt-12 {
  margin-top: 12px;
}

@media (max-width: 1200px) {
  .core-status-grid,
  .trend-grid,
  .insight-grid {
    grid-template-columns: 1fr;
  }

  .server-grid {
    grid-template-columns: 1fr;
  }

  .alert-item {
    grid-template-columns: 1fr;
  }

  .alert-actions {
    justify-content: flex-start;
  }
}
</style>
