<template>
  <div class="pool-page">
    <el-card v-loading="pageLoading" class="surface-card table-card">
      <section class="page-header">
        <h3>{{ t('nav.poolAnalytics') }}</h3>
        <p class="desc">{{ t('pool.analyticsDesc') }}</p>
      </section>

      <div class="filter-bar surface-card">
        <div class="bar-left">
          <div class="filter-item">
            <span class="filter-label">{{ t('pool.analyticsPool') }}</span>
            <el-select
              v-model="selectedPoolId"
              filterable
              clearable
              class="pool-select"
              :placeholder="t('pool.search')"
            >
              <el-option
                v-for="pool in poolOptions"
                :key="pool.id"
                :label="`${pool.name} (${pool.cidr})`"
                :value="pool.id"
              />
            </el-select>
          </div>
          <div class="filter-item">
            <span class="filter-label">{{ t('pool.analyticsTimeWindow') }}</span>
            <el-radio-group v-model="timeWindow" class="time-tabs">
              <el-radio-button label="3d">{{ t('pool.analytics3d') }}</el-radio-button>
              <el-radio-button label="7d">{{ t('pool.analytics7d') }}</el-radio-button>
              <el-radio-button label="30d">{{ t('pool.analytics30d') }}</el-radio-button>
            </el-radio-group>
          </div>
        </div>
        <div class="bar-right">
          <el-button :icon="Refresh" :loading="insightLoading" :disabled="insightLoading || pageLoading" @click="refreshAll">
            {{ t('common.refresh') }}
          </el-button>
          <el-button :loading="exportingReport" :disabled="insightLoading || pageLoading" @click="exportReport">{{ t('pool.analyticsExportReport') }}</el-button>
          <el-button type="primary" :disabled="insightLoading || pageLoading" @click="comparePools">{{ t('pool.analyticsComparePools') }}</el-button>
        </div>
      </div>

      <section class="overview-grid">
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.analyticsTotalCapacity') }}</div>
          <div class="overview-value">{{ overviewTotalCapacity }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.analyticsUsedAndRate') }}</div>
          <div class="overview-value">{{ overviewUsedAndRate }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.analyticsSuccessRate') }}</div>
          <div class="overview-value">{{ allocationSuccessRateLabel }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.analyticsConflictCount') }}</div>
          <div class="overview-value">{{ conflictCount }}</div>
        </el-card>
      </section>

      <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12" />

      <section class="analysis-grid">
        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('pool.analyticsCapacityPlanning') }}</span>
              <span class="card-sub">{{ t('pool.analyticsPeakUtilization', { value: peakUtilization.toFixed(1) }) }}</span>
            </div>
          </template>
          <el-skeleton v-if="insightLoading" animated :rows="5" />
          <el-empty v-else-if="!capacityTrendHasData" :image-size="60" :description="t('pool.analyticsNoCapacityTrend')" />
          <template v-else>
            <BaseEChart :option="capacityPlanningOption" class="chart" />
            <div class="mini-metrics">
              <div>{{ t('pool.analyticsAvgCapacity', { value: avgCapacity.toFixed(1) }) }}</div>
              <div>{{ t('pool.analyticsAvgUsed', { value: avgUsed.toFixed(1) }) }}</div>
              <div>{{ t('pool.analyticsRiskNodes', { value: anomalyCount }) }}</div>
            </div>
          </template>
        </el-card>

        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('pool.analyticsEfficiency') }}</span>
              <span class="card-sub">{{ t('pool.analyticsTotalRequests', { value: efficiencyTotal }) }}</span>
            </div>
          </template>
          <el-skeleton v-if="insightLoading" animated :rows="5" />
          <el-empty v-else-if="!efficiencyTotal" :image-size="60" :description="t('pool.analyticsNoEfficiency')" />
          <template v-else>
            <BaseEChart :option="efficiencyDonutOption" class="chart" />
            <div class="mini-metrics">
              <div>{{ t('pool.analyticsSuccessLabel', { value: efficiencySuccess }) }}</div>
              <div>{{ t('pool.analyticsFailedLabel', { value: efficiencyFailed }) }}</div>
              <div>{{ t('pool.analyticsSuccessRateLabel', { value: allocationSuccessRateLabel }) }}</div>
            </div>
          </template>
        </el-card>

        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('pool.analyticsCapacityTrend') }}</span>
              <span class="card-sub">{{ t('pool.analyticsRecent', { window: windowLabel }) }}</span>
            </div>
          </template>
          <el-skeleton v-if="insightLoading" animated :rows="5" />
          <el-empty v-else-if="!capacityTrendHasData" :image-size="60" :description="t('pool.analyticsNoCapacityTrend')" />
          <BaseEChart v-else :option="capacityTrendBarOption" class="chart" />
        </el-card>

        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('pool.analyticsConflictEvents') }}</span>
              <span class="card-sub">{{ t('pool.analyticsConflictTotal', { value: conflictCount }) }}</span>
            </div>
          </template>
          <el-skeleton v-if="insightLoading" animated :rows="5" />
          <el-empty v-else-if="!conflictTrendHasData" :image-size="60" :description="t('pool.analyticsNoConflict')" />
          <BaseEChart v-else :option="conflictOption" class="chart" />
        </el-card>
      </section>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { Refresh } from '@element-plus/icons-vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { listPools, getUsage, getHistory, getConflicts } from '@/api/pools';
import type { PoolSummary, BindingConflict } from '@/types/pool';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { showInfo, showSuccess } from '@/shared/errors/messageToast';
import { createInlineError, getApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();

const pools = ref<PoolSummary[]>([]);
const selectedPoolId = ref('');
const timeWindow = ref<'3d' | '7d' | '30d'>('7d');

const usageSeries = ref<{ ts: string[]; used: number[]; capacity: number[] }>({
  ts: [],
  used: [],
  capacity: []
});
const allocationStats = ref<{ success: number; failed: number }>({ success: 0, failed: 0 });
const conflictEntries = ref<BindingConflict[]>([]);
const conflictTotalByWindow = ref(0);

const pageLoading = ref(false);
const insightLoading = ref(false);
const exportingReport = ref(false);
const pageError = ref<ApiErrorDescriptor | null>(null);

const poolOptions = computed(() => pools.value);
const currentPool = computed(() => pools.value.find((item) => item.id === selectedPoolId.value) || null);

const windowLabel = computed(() => {
  if (timeWindow.value === '3d') return t('pool.analyticsWindow3d');
  if (timeWindow.value === '30d') return t('pool.analyticsWindow30d');
  return t('pool.analyticsWindow7d');
});

const maxArrayValue = (arr: number[]) => (arr.length ? Math.max(...arr.map((value) => Number(value || 0))) : 0);
const latestArrayValue = (arr: number[]) => (arr.length ? Number(arr[arr.length - 1] || 0) : 0);

const deriveIPv4CapacityFromCIDR = (cidr: string): number => {
  const text = String(cidr || '').trim();
  const parts = text.split('/');
  if (parts.length !== 2) return 0;
  const prefix = Number(parts[1]);
  if (!Number.isFinite(prefix) || prefix < 0 || prefix > 32) return 0;
  if (prefix >= 31) return 0;
  return 2 ** (32 - prefix) - 2;
};

const resolveWindowRange = (window: '3d' | '7d' | '30d') => {
  const days = window === '3d' ? 3 : window === '30d' ? 30 : 7;
  const now = new Date();
  const end = new Date(now);
  end.setHours(23, 59, 59, 999);
  const start = new Date(now);
  start.setDate(start.getDate() - (days - 1));
  start.setHours(0, 0, 0, 0);
  return {
    from: start.toISOString(),
    to: end.toISOString()
  };
};

const overviewTotalCapacity = computed(() => {
  const totalIPv4 = pools.value
    .filter((item) => item.version === 4)
    .reduce((sum, item) => {
      const declared = Number(item.capacity || 0);
      if (declared > 0) return sum + declared;
      return sum + deriveIPv4CapacityFromCIDR(item.cidr);
    }, 0);
  if (totalIPv4 > 0) return totalIPv4;

  const fromSeries = usageSeries.value.capacity.length
    ? latestArrayValue(usageSeries.value.capacity)
    : maxArrayValue(usageSeries.value.capacity);
  if (fromSeries > 0) return fromSeries;
  return Number(currentPool.value?.capacity || 0);
});

const selectedPoolCapacity = computed(() => {
  const fromSeries = usageSeries.value.capacity.length
    ? latestArrayValue(usageSeries.value.capacity)
    : maxArrayValue(usageSeries.value.capacity);
  if (fromSeries > 0) return fromSeries;
  return Number(currentPool.value?.capacity || 0);
});

const overviewUsedAndRate = computed(() => {
  const used = usageSeries.value.used.length
    ? latestArrayValue(usageSeries.value.used)
    : Number(currentPool.value?.allocated || 0);
  const total = selectedPoolCapacity.value;
  if (!total) return '0 / 0.0%';
  const rate = (used / total) * 100;
  return `${used} / ${rate.toFixed(1)}%`;
});

const efficiencyFallbackSuccess = computed(() => {
  if (usageSeries.value.used.length) {
    return latestArrayValue(usageSeries.value.used);
  }
  return Number(currentPool.value?.allocated || 0);
});

const efficiencySuccess = computed(() => {
  if (allocationStats.value.success > 0 || allocationStats.value.failed > 0) {
    return allocationStats.value.success;
  }
  return efficiencyFallbackSuccess.value;
});

const efficiencyFailed = computed(() => {
  if (allocationStats.value.success > 0 || allocationStats.value.failed > 0) {
    return allocationStats.value.failed;
  }
  return conflictTotalByWindow.value;
});

const efficiencyTotal = computed(() => efficiencySuccess.value + efficiencyFailed.value);

const allocationSuccessRate = computed(() => {
  if (!efficiencyTotal.value) return null;
  return (efficiencySuccess.value / efficiencyTotal.value) * 100;
});

const allocationSuccessRateLabel = computed(() => {
  if (allocationSuccessRate.value === null) return '--';
  return `${allocationSuccessRate.value.toFixed(1)}%`;
});

const conflictCount = computed(() => {
  if (conflictTotalByWindow.value > 0) return conflictTotalByWindow.value;
  return conflictEntries.value.length;
});

const peakUtilization = computed(() => {
  const cap = usageSeries.value.capacity;
  const used = usageSeries.value.used;
  if (!cap.length || !used.length) return Number(currentPool.value?.utilization || 0);
  let peak = 0;
  for (let index = 0; index < Math.min(cap.length, used.length); index += 1) {
    const capacity = Number(cap[index] || 0);
    const usedValue = Number(used[index] || 0);
    if (capacity <= 0) continue;
    const rate = (usedValue / capacity) * 100;
    if (rate > peak) peak = rate;
  }
  return peak;
});

const avgCapacity = computed(() => {
  if (!usageSeries.value.capacity.length) return 0;
  const total = usageSeries.value.capacity.reduce((sum, item) => sum + Number(item || 0), 0);
  return total / usageSeries.value.capacity.length;
});

const avgUsed = computed(() => {
  if (!usageSeries.value.used.length) return 0;
  const total = usageSeries.value.used.reduce((sum, item) => sum + Number(item || 0), 0);
  return total / usageSeries.value.used.length;
});

const anomalyIndexes = computed(() => {
  const result: number[] = [];
  for (let index = 0; index < usageSeries.value.used.length; index += 1) {
    const used = Number(usageSeries.value.used[index] || 0);
    const cap = Number(usageSeries.value.capacity[index] || 0);
    if (cap > 0 && used / cap >= 0.85) {
      result.push(index);
    }
  }
  return result;
});

const anomalyCount = computed(() => anomalyIndexes.value.length);
const capacityTrendHasData = computed(() => usageSeries.value.ts.length > 0);

const conflictByDay = computed(() => {
  const map = new Map<string, number>();
  conflictEntries.value.forEach((entry) => {
    const raw = String(entry.updatedAt || '').trim();
    const key = raw ? raw.slice(0, 10) : t('pool.analyticsUnknown');
    map.set(key, (map.get(key) || 0) + 1);
  });
  const rows = Array.from(map.entries())
    .map(([day, value]) => ({ day, value }))
    .sort((a, b) => (a.day > b.day ? 1 : -1));
  return rows;
});

const conflictTrendHasData = computed(() => conflictByDay.value.length > 0);

const capacityPlanningOption = computed(() => {
  const usedSeries = usageSeries.value.used.map((value, index) => {
    const isAnomaly = anomalyIndexes.value.includes(index);
    return {
      value: Number(value || 0),
      symbolSize: isAnomaly ? 10 : 6,
      itemStyle: isAnomaly ? { color: '#ef4444' } : undefined
    };
  });

  return {
    tooltip: { trigger: 'axis' },
    legend: { data: [t('pool.analyticsChartUsed'), t('pool.analyticsChartCapacity')] },
    grid: { left: 24, right: 18, top: 34, bottom: 24 },
    xAxis: { type: 'category', data: usageSeries.value.ts },
    yAxis: { type: 'value' },
    series: [
      {
        type: 'line',
        name: t('pool.analyticsChartUsed'),
        data: usedSeries,
        smooth: true,
        lineStyle: { width: 2, color: '#3b82f6' }
      },
      {
        type: 'line',
        name: t('pool.analyticsChartCapacity'),
        data: usageSeries.value.capacity,
        smooth: true,
        lineStyle: { width: 2, color: '#10b981' }
      }
    ]
  };
});

const efficiencyDonutOption = computed(() => ({
  tooltip: { trigger: 'item' },
  legend: { bottom: 0 },
  series: [
    {
      type: 'pie',
      radius: ['52%', '72%'],
      center: ['50%', '45%'],
      avoidLabelOverlap: false,
      label: { formatter: '{b}: {d}%' },
      data: [
        { name: t('pool.analyticsChartSuccess'), value: efficiencySuccess.value, itemStyle: { color: '#22c55e' } },
        { name: t('pool.analyticsChartFailed'), value: efficiencyFailed.value, itemStyle: { color: '#ef4444' } }
      ]
    }
  ]
}));

const capacityTrendBarOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: 24, right: 18, top: 34, bottom: 24 },
  xAxis: { type: 'category', data: usageSeries.value.ts },
  yAxis: { type: 'value' },
  series: [
    {
      type: 'bar',
      name: t('pool.analyticsChartUsageRate'),
      data: usageSeries.value.used.map((used, index) => {
        const cap = Number(usageSeries.value.capacity[index] || 0);
        const rate = cap > 0 ? (Number(used || 0) / cap) * 100 : 0;
        return {
          value: Number(rate.toFixed(1)),
          itemStyle: { color: rate >= 85 ? '#ef4444' : rate >= 60 ? '#f59e0b' : '#22c55e' }
        };
      })
    }
  ]
}));

const conflictOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: 24, right: 18, top: 34, bottom: 24 },
  xAxis: { type: 'category', data: conflictByDay.value.map((item) => item.day) },
  yAxis: { type: 'value', minInterval: 1 },
  series: [
    {
      type: 'line',
      smooth: true,
      name: t('pool.analyticsChartConflictCount'),
      data: conflictByDay.value.map((item) => ({
        value: item.value,
        symbolSize: item.value >= 3 ? 10 : 6,
        itemStyle: item.value >= 3 ? { color: '#ef4444' } : undefined
      })),
      lineStyle: { width: 2, color: '#f97316' }
    }
  ]
}));

const fetchPools = async () => {
  if (!permissionStore.can('pool.analytics.view')) return;
  try {
    const { data } = await listPools({
      page: 1,
      pageSize: 300,
      tenantId: tenantStore.currentTenantId || 'global'
    });
    pools.value = data.data.items || [];
    if (!pools.value.length) {
      selectedPoolId.value = '';
      usageSeries.value = { ts: [], used: [], capacity: [] };
      allocationStats.value = { success: 0, failed: 0 };
      conflictEntries.value = [];
      conflictTotalByWindow.value = 0;
      return;
    }
    if (!selectedPoolId.value || !pools.value.some((pool) => pool.id === selectedPoolId.value)) {
      selectedPoolId.value = pools.value[0].id;
    }
  } catch (error) {
    pageError.value = getApiError(error) || createInlineError(t('pool.loadFail'));
  }
};

const fetchUsageSeries = async () => {
  if (!selectedPoolId.value) {
    usageSeries.value = { ts: [], used: [], capacity: [] };
    return;
  }
  const { data } = await getUsage(selectedPoolId.value, {
    tenantId: tenantStore.currentTenantId || 'global',
    window: timeWindow.value
  });
  usageSeries.value = data?.data || { ts: [], used: [], capacity: [] };
};

const fetchAllocationStats = async () => {
  if (!selectedPoolId.value) {
    allocationStats.value = { success: 0, failed: 0 };
    conflictTotalByWindow.value = 0;
    return;
  }

  const range = resolveWindowRange(timeWindow.value);
  const baseParams = {
    page: 1,
    pageSize: 1,
    tenantId: tenantStore.currentTenantId || 'global',
    from: range.from,
    to: range.to
  };

  const [successResp, conflictResp, declinedResp] = await Promise.all([
    getHistory(selectedPoolId.value, { ...baseParams, state: 'active' }),
    getHistory(selectedPoolId.value, { ...baseParams, state: 'conflict' }),
    getHistory(selectedPoolId.value, { ...baseParams, state: 'declined' })
  ]);

  const toTotal = (resp: any) => Number(resp?.data?.data?.total || 0);
  const success = toTotal(successResp);
  const conflict = toTotal(conflictResp);
  const declined = toTotal(declinedResp);
  const failed = conflict + declined;

  allocationStats.value = {
    success,
    failed
  };
  conflictTotalByWindow.value = conflict;
};

const fetchConflictsReport = async () => {
  if (!selectedPoolId.value) {
    conflictEntries.value = [];
    return;
  }
  const { data } = await getConflicts(selectedPoolId.value, {
    tenantId: tenantStore.currentTenantId || 'global',
    page: 1,
    pageSize: 200
  });
  conflictEntries.value = data?.data?.items || [];
};

const fetchInsights = async () => {
  if (!selectedPoolId.value) return;
  insightLoading.value = true;
  pageError.value = null;
  try {
    await Promise.all([fetchUsageSeries(), fetchAllocationStats(), fetchConflictsReport()]);
  } catch (error) {
    pageError.value = getApiError(error) || createInlineError(t('pool.monitorFail'));
  } finally {
    insightLoading.value = false;
  }
};

const refreshAll = async () => {
  if (pageLoading.value || insightLoading.value) return;
  pageLoading.value = true;
  pageError.value = null;
  try {
    await fetchPools();
    await fetchInsights();
  } finally {
    pageLoading.value = false;
  }
};

const exportReport = async () => {
  if (!selectedPoolId.value) {
    showInfo(t('pool.analyticsSelectPool'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('pool.analyticsExportConfirm'), t('pool.analyticsExportTitle'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  exportingReport.value = true;
  try {
    const poolName = currentPool.value?.name || 'pool';
    const escapeCell = (value: unknown) => `"${String(value ?? '').replace(/"/g, '""')}"`;

    const sections: string[] = [];

    const overviewLines = [
      [t('pool.analyticsReportTitle'), new Date().toISOString()],
      [t('pool.analyticsReportPool'), `${currentPool.value?.name || '-'} (${currentPool.value?.cidr || '-'})`],
      [t('pool.analyticsReportTimeWindow'), windowLabel.value],
      [t('pool.analyticsReportCapacity'), String(overviewTotalCapacity.value)],
      [t('pool.analyticsReportUsedRate'), overviewUsedAndRate.value],
      [t('pool.analyticsReportSuccessRate'), allocationSuccessRateLabel.value],
      [t('pool.analyticsReportConflict'), String(conflictCount.value)]
    ];
    sections.push(overviewLines.map((row) => row.map(escapeCell).join(',')).join('\n'));

    const trendRows = usageSeries.value.ts.map((ts, index) => {
      const used = Number(usageSeries.value.used[index] || 0);
      const capacity = Number(usageSeries.value.capacity[index] || 0);
      const utilization = capacity > 0 ? Number(((used / capacity) * 100).toFixed(1)) : 0;
      return [ts, String(used), String(capacity), `${utilization}%`];
    });
    sections.push(
      [
        [t('pool.analyticsReportTrendDetail')],
        [t('pool.analyticsReportTime'), t('pool.analyticsReportUsedCount'), t('pool.analyticsReportTotalCapacity'), t('pool.analyticsReportUtilization')],
        ...trendRows
      ]
        .map((row) => row.map(escapeCell).join(','))
        .join('\n')
    );

    const conflictRows = conflictEntries.value.map((item) => [
      item.id || '',
      item.poolId || '',
      item.ipAddress || '',
      item.identifier || '',
      item.identifierType || '',
      item.updatedAt || ''
    ]);
    sections.push(
      [
        [t('pool.analyticsReportConflictDetail')],
        ['ID', t('pool.analyticsReportPool') + 'ID', t('pool.analyticsReportIp'), t('pool.analyticsReportIdentifier'), t('pool.analyticsReportIdentifierType'), t('pool.analyticsReportUpdateTime')],
        ...conflictRows
      ]
        .map((row) => row.map(escapeCell).join(','))
        .join('\n')
    );

    const csv = sections.join('\n\n');
    const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${poolName}-analysis-report.csv`;
    link.click();
    URL.revokeObjectURL(url);
    showSuccess(t('pool.analyticsExportDone'));
  } finally {
    exportingReport.value = false;
  }
};

const comparePools = async () => {
  if (pools.value.length < 2) {
    showInfo(t('pool.analyticsCompareMin'));
    return;
  }
  await ElMessageBox.confirm(t('pool.analyticsCompareConfirm'), t('pool.analyticsCompareTitle'), {
    type: 'warning'
  });
  const sorted = [...pools.value].sort((a, b) => b.utilization - a.utilization);
  const top = sorted.slice(0, 10);
  const summary = top
    .map((item, index) => `${index + 1}. ${item.name} (${item.cidr}) - ${Number(item.utilization || 0).toFixed(1)}%`)
    .join('\n');

  const escapeCell = (value: unknown) => `"${String(value ?? '').replace(/"/g, '""')}"`;
  const rows = [
    [t('pool.analyticsCompareRank'), t('pool.analyticsComparePoolName'), 'CIDR', t('pool.analyticsCompareStatus'), t('pool.analyticsCompareUtilization'), t('pool.analyticsCompareAllocated'), t('pool.analyticsCompareCapacity')],
    ...top.map((item, index) => [
      index + 1,
      item.name,
      item.cidr,
      item.status,
      Number(item.utilization || 0).toFixed(1),
      Number(item.allocated || 0),
      Number(item.capacity || 0)
    ])
  ];
  const csv = rows.map((row) => row.map(escapeCell).join(',')).join('\n');
  const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `pool-compare-${new Date().toISOString().slice(0, 10)}.csv`;
  link.click();
  URL.revokeObjectURL(url);
  showSuccess(t('pool.analyticsCompareDone'));

  await ElMessageBox.alert(summary || t('pool.analyticsCompareEmpty'), t('pool.analyticsCompareResult'), {
    confirmButtonText: t('common.confirm')
  });
};

watch(
  () => tenantStore.currentTenantId,
  async () => {
    await refreshAll();
  }
);

watch([selectedPoolId, timeWindow], async () => {
  await fetchInsights();
});

onMounted(async () => {
  await refreshAll();
});
</script>

<style scoped>
.pool-page {
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-muted: #4b5563;
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 2400px;
  margin: 0 auto;
  min-height: calc(100vh - 24px);
}

.table-card {
  width: 100%;
  min-height: calc(100vh - 140px);
}

.table-card :deep(.el-card__body) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-header {
  margin-bottom: 12px;
}

.page-header h3 {
  margin: 0;
}

.filter-bar {
  height: 56px;
  padding: 12px 16px;
  margin-bottom: 12px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  background: #f5f7fa;
}

.bar-left,
.bar-right {
  display: flex;
  gap: 8px;
  align-items: center;
}

.bar-left {
  min-width: 0;
  gap: 16px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}

.bar-right {
  justify-content: flex-end;
  white-space: nowrap;
}

.filter-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.filter-label {
  width: 56px;
  font-size: 13px;
  line-height: 20px;
  font-weight: 500;
  color: var(--text-muted);
}

.pool-select {
  width: 300px;
}

.time-tabs {
  display: inline-flex;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filter-bar :deep(.el-select__wrapper),
.filter-bar :deep(.el-radio-button__inner),
.filter-bar :deep(.el-button) {
  height: 32px;
  min-height: 32px;
  border-radius: 8px;
}

.filter-bar :deep(.el-select__wrapper) {
  box-shadow: 0 0 0 1px var(--el-border-color) inset;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.overview-card {
  border-radius: 10px;
}

.overview-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.overview-value {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
}

.analysis-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.analysis-card {
  border-radius: 10px;
  min-height: 360px;
}

.card-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.card-title {
  font-weight: 600;
  color: var(--text-primary);
}

.card-sub {
  color: var(--text-secondary);
  font-size: 12px;
}

.chart {
  height: 260px;
}

.mini-metrics {
  margin-top: 8px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  color: var(--text-muted);
  font-size: 13px;
}

.mb-12 {
  margin-bottom: 12px;
}

@media (max-width: 1200px) {
  .filter-bar {
    grid-template-columns: 1fr;
    height: auto;
    gap: 12px;
  }

  .bar-right {
    justify-content: flex-start;
  }

  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .analysis-grid {
    grid-template-columns: 1fr;
  }

  .mini-metrics {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .overview-grid {
    grid-template-columns: 1fr;
  }
}
</style>
