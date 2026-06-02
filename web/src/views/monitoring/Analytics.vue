<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('monitoring.analytics.title') }}</h3>
      <p class="desc">{{ t('monitoring.analytics.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-select v-model="range" style="width: 130px" @change="loadAnalytics">
          <el-option :label="t('monitoring.analytics.range7d')" value="7d" />
          <el-option :label="t('monitoring.analytics.range30d')" value="30d" />
          <el-option :label="t('monitoring.analytics.range90d')" value="90d" />
        </el-select>
        <el-select v-model="compare" style="width: 130px" @change="loadAnalytics">
          <el-option :label="t('monitoring.analytics.comparePrev')" value="prev" />
          <el-option :label="t('monitoring.analytics.compareLastYear')" value="last-year" />
        </el-select>
      </div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button :loading="loading" @click="loadAnalytics">{{ t('monitoring.analytics.refresh') }}</el-button>
      </div>
    </div>

    <section class="stat-grid">
      <el-card v-for="card in metricCards" :key="card.key" shadow="never" class="stat-card">
        <div class="stat-title">{{ card.title }}</div>
        <div class="stat-value">{{ card.value }}</div>
        <div class="stat-sub">{{ card.desc }}</div>
      </el-card>
    </section>

    <el-card class="surface-card content-card" v-loading="loading">
      <div class="content-grid">
        <div>
          <div class="section-title">{{ t('monitoring.analytics.sectionTypeDistribution') }}</div>
          <div class="pill-list">
            <div class="pill" v-for="item in typeDistribution" :key="item.type" @click="goHistory(item.type)">
              <span class="dot" :style="{ background: item.color }"></span>
              <span class="name">{{ item.type }}</span>
              <span class="value">{{ item.value }}</span>
            </div>
          </div>

          <div class="section-title mt">{{ t('monitoring.analytics.sectionTimeDistribution') }}</div>
          <div class="trend-wrapper">
            <div class="trend-item" v-for="point in timeDistribution" :key="point.hour">
              <div class="bar" :style="{ height: `${Math.max(8, point.value * 6)}px` }"></div>
              <div class="bar-label">{{ point.hour }}h</div>
            </div>
          </div>
        </div>

        <div>
          <div class="section-title">{{ t('monitoring.analytics.sectionTrend') }}</div>
          <div class="compare-grid">
            <div class="compare-card" v-for="item in trendCompareCards" :key="item.label">
              <div class="label">{{ item.label }}</div>
              <div class="value">{{ item.value }}</div>
              <div class="delta" :class="item.delta >= 0 ? 'up' : 'down'">
                {{ item.delta >= 0 ? '+' : '' }}{{ item.delta }}%
              </div>
            </div>
          </div>

          <div class="section-title mt">{{ t('monitoring.analytics.sectionAnomaly') }}</div>
          <div class="anomaly-list">
            <div class="anomaly" v-for="a in anomalies" :key="`${a.time}-${a.desc}`" @click="goConfig">
              <span class="dot red" />
              <span class="time">{{ a.time }}</span>
              <span class="desc-text">{{ a.desc }}</span>
            </div>
            <el-empty v-if="!anomalies.length" :description="t('monitoring.analytics.anomalyEmpty')" :image-size="56" />
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import {
  getAlertAnalyticsSummary,
  getAlertTimeDistribution,
  getAlertTrendCompare,
  getAlertTypeDistribution
} from '@/api/monitoring';
import type {
  AlertAnalyticsSummary,
  AlertTimeDistributionItem,
  AlertTrendCompare,
  AlertTypeDistributionItem
} from '@/types/monitoring';

const { t } = useI18n();
const router = useRouter();
const loading = ref(false);
const range = ref<'7d' | '30d' | '90d'>('30d');
const compare = ref<'prev' | 'last-year'>('prev');

const summary = ref<AlertAnalyticsSummary>({ total: 0, mttr: '-', topType: '-', resolutionRate: 0 });
const typeDistributionRaw = ref<AlertTypeDistributionItem[]>([]);
const timeDistributionRaw = ref<AlertTimeDistributionItem[]>([]);
const trendCompareRaw = ref<AlertTrendCompare | null>(null);

const palette = ['#f56c6c', '#e6a23c', '#409eff', '#909399', '#67c23a', '#8d8d8d'];

const metricCards = computed(() => [
  { key: 'total', title: t('monitoring.analytics.statTotal'), value: summary.value.total, desc: t('monitoring.analytics.statTotalDesc') },
  { key: 'mttr', title: t('monitoring.analytics.statMttr'), value: summary.value.mttr || '-', desc: t('monitoring.analytics.statMttrDesc') },
  { key: 'top', title: t('monitoring.analytics.statTopType'), value: summary.value.topType || '-', desc: t('monitoring.analytics.statTopTypeDesc') },
  { key: 'rate', title: t('monitoring.analytics.statRate'), value: `${summary.value.resolutionRate}%`, desc: t('monitoring.analytics.statRateDesc') }
]);

const typeDistribution = computed(() =>
  typeDistributionRaw.value.map((item, idx) => ({
    type: item.type,
    value: item.count,
    color: palette[idx % palette.length]
  }))
);

const timeDistribution = computed(() =>
  timeDistributionRaw.value.map((item) => ({
    hour: item.hour,
    value: item.count
  }))
);

const trendCompareCards = computed(() => {
  if (!trendCompareRaw.value) return [];
  const currentTotal = trendCompareRaw.value.current.reduce((sum, item) => sum + item.count, 0);
  const previousTotal = trendCompareRaw.value.previous.reduce((sum, item) => sum + item.count, 0) || 1;
  const delta = Math.round(((currentTotal - previousTotal) / previousTotal) * 100);
  return [
    { label: t('monitoring.analytics.trendCountLabel'), value: `${currentTotal} vs ${previousTotal}`, delta },
    { label: t('monitoring.analytics.trendGrowth'), value: `${delta}%`, delta },
    { label: t('monitoring.analytics.trendAnomalyCount'), value: t('monitoring.analytics.trendAnomalyUnit', { count: trendCompareRaw.value.anomalies?.length || 0 }), delta: 0 }
  ];
});

const anomalies = computed(() =>
  (trendCompareRaw.value?.anomalies || []).map((item) => ({
    time: item.ts ? new Date(item.ts).toLocaleString() : '-',
    desc: item.desc
  }))
);

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;

const loadAnalytics = async () => {
  loading.value = true;
  try {
    const [summaryResp, typeResp, timeResp, trendResp] = await Promise.all([
      getAlertAnalyticsSummary({ range: range.value }),
      getAlertTypeDistribution({ range: range.value }),
      getAlertTimeDistribution({ range: '24h', step: '2h' }),
      getAlertTrendCompare({ range: range.value, compare: compare.value })
    ]);
    summary.value = unwrap<AlertAnalyticsSummary>(summaryResp);
    typeDistributionRaw.value = unwrap<AlertTypeDistributionItem[]>(typeResp) || [];
    timeDistributionRaw.value = unwrap<AlertTimeDistributionItem[]>(timeResp) || [];
    trendCompareRaw.value = unwrap<AlertTrendCompare>(trendResp);
  } catch (err) {
    showHttpError(err, t('monitoring.analytics.loadFail'));
  } finally {
    loading.value = false;
  }
};

const goHistory = (type: string) => {
  router.push({ path: '/monitor/history', query: { type } });
};

const goConfig = () => {
  router.push({ path: '/monitor/config', query: { tab: 'thresholds' } });
};

onMounted(loadAnalytics);
</script>

<style scoped>
.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #ebeef5;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  --pill-bg: #ffffff;
  --trend-bar-bg: #dbeafe;
  --danger-bg: #fff1f2;
  --danger-border: #fee2e2;
  --danger-up: #f56c6c;
  --success-down: #67c23a;
  padding: 12px;
  background: var(--page-bg);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.surface-card {
  border: 1px solid var(--surface-border);
  border-radius: 4px;
  background: var(--surface-bg);
  color: var(--text-primary);
}

.page-header {
  padding: 12px 16px;
}

.page-header h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filter-bar {
  height: 48px;
  padding: 8px 16px;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 8px;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button) {
  height: 32px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  border: 1px solid var(--surface-border);
}

.stat-title {
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 700;
}

.stat-sub {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-tertiary);
}

.content-card :deep(.el-card__body) {
  padding: 16px;
}

.content-grid {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 16px;
}

.section-title {
  margin-bottom: 8px;
  font-weight: 600;
}

.mt {
  margin-top: 14px;
}

.pill-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--surface-border);
  border-radius: 999px;
  padding: 6px 10px;
  cursor: pointer;
  background: var(--pill-bg);
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}

.trend-wrapper {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(42px, 1fr));
  align-items: end;
  gap: 8px;
}

.trend-item {
  text-align: center;
  color: var(--text-secondary);
  font-size: 12px;
}

.bar {
  width: 14px;
  margin: 0 auto;
  background: var(--trend-bar-bg);
  border-radius: 6px;
}

.bar-label {
  margin-top: 6px;
}

.compare-grid {
  display: grid;
  gap: 10px;
}

.compare-card {
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  padding: 10px;
  background: var(--surface-bg);
}

.label {
  font-size: 12px;
  color: var(--text-secondary);
}

.value {
  margin-top: 6px;
  font-size: 18px;
  font-weight: 700;
}

.delta {
  margin-top: 6px;
  font-size: 12px;
}

.delta.up {
  color: var(--danger-up);
}

.delta.down {
  color: var(--success-down);
}

.anomaly-list {
  display: grid;
  gap: 6px;
}

.anomaly {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--danger-border);
  border-radius: 6px;
  background: var(--danger-bg);
  cursor: pointer;
}

.dot.red {
  background: #f56c6c;
}

.time {
  color: var(--text-secondary);
  white-space: nowrap;
}

.desc-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
