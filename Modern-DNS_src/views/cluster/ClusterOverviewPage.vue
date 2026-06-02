<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseChart from '../../components/BaseChart.vue'
import { getClusterOverviewApi, getClusterStateApi, type ClusterNode, type TrendPoint } from '@/api/cluster'

const { t } = useI18n()

const timeRange = ref('1d')
const loading   = ref(false)

const nodes     = ref<ClusterNode[]>([])
const trendRaw  = ref<TrendPoint[]>([])

const onlineCount  = computed(() => nodes.value.filter(n => n.status === '在线').length)
const offlineCount = computed(() => nodes.value.filter(n => n.status === '离线').length)
const syncIssues   = computed(() => nodes.value.filter(n => n.syncLag > 100).length)
const totalQps     = computed(() => nodes.value.reduce((s, n) => s + n.qps, 0))
const avgCpu       = computed(() => {
  const active = nodes.value.filter(n => n.status !== '离线')
  return active.length ? Math.round(active.reduce((s, n) => s + n.cpuUsage, 0) / active.length) : 0
})
const avgMem       = computed(() => {
  const active = nodes.value.filter(n => n.status !== '离线')
  return active.length ? Math.round(active.reduce((s, n) => s + n.memUsage, 0) / active.length) : 0
})

const roleColor = computed<Record<string, string>>(() => ({ [t('cluster.masterNode')]: '#F53F3F', [t('cluster.slaveNode')]: '#1677FF', [t('cluster.arbiterNode')]: '#722ED1' }))
const statusClass = (s: string) => s === '在线' ? 'mn-badge--success' : s === '离线' ? 'mn-badge--danger' : s === '同步中' ? 'mn-badge--warning' : 'mn-badge--neutral'
const usageColor = (v: number) => v >= 80 ? '#F53F3F' : v >= 60 ? '#FF7D00' : '#00B42A'

const trendData = computed(() => ({
  times:   trendRaw.value.map(t => t.time),
  qps:     trendRaw.value.map(t => t.qps),
  latency: trendRaw.value.map(t => t.latency),
}))

// Cluster bootstrap flag — used to render an init-prompt card instead of
// stale-looking empty charts when the cluster hasn't been bootstrapped.
// We refresh it alongside overview so a freshly-initialized cluster goes
// from "Init" CTA to live charts within one tick.
const clusterReady = ref<boolean>(false)

// loadOverview honours the current timeRange selector so the trend window
// matches what the user picked. Default '1d' = 24h with 13 buckets ≈ 2h
// each, identical to the legacy behaviour.
const loadOverview = async (): Promise<void> => {
  loading.value = true
  try {
    const [overview, state] = await Promise.all([
      getClusterOverviewApi(timeRange.value),
      getClusterStateApi().catch(() => null),
    ])
    nodes.value    = overview.data.nodes ?? []
    trendRaw.value = overview.data.trend ?? []
    clusterReady.value = state?.data?.initialized === true
  } finally {
    loading.value = false
  }
}

// ── Auto-refresh ──
// Poll every 10s, but only when the tab is visible so a backgrounded
// dashboard doesn't keep hammering the API.
const refreshIntervalMs = 10_000
let refreshTimer: number | null = null

const startAutoRefresh = (): void => {
  stopAutoRefresh()
  refreshTimer = window.setInterval(() => {
    if (document.visibilityState === 'visible') {
      loadOverview()
    }
  }, refreshIntervalMs)
}
const stopAutoRefresh = (): void => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}
const handleVisibility = (): void => {
  if (document.visibilityState === 'visible') {
    loadOverview()
  }
}

watch(timeRange, () => {
  loadOverview()
})

onMounted(() => {
  loadOverview()
  startAutoRefresh()
  document.addEventListener('visibilitychange', handleVisibility)
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  document.removeEventListener('visibilitychange', handleVisibility)
})

/* ── 公共 tooltip 样式（亮色主题） ── */
const tooltipStyle = {
  trigger: 'axis' as const,
  confine: true,
  backgroundColor: '#ffffff',
  borderColor: '#E5E6EB',
  borderWidth: 1,
  borderRadius: 8,
  padding: [10, 14],
  textStyle: { color: '#1D2129', fontSize: 12 },
  extraCssText: 'box-shadow:0 4px 16px rgba(0,0,0,0.10);',
  axisPointer: {
    type: 'line' as const,
    lineStyle: { color: '#165DFF', width: 1, type: 'dashed' as const },
  },
}

/* ── 公共轴样式 ── */
const axisLabel = { color: '#86909C', fontSize: 11 }
const splitLine  = { lineStyle: { color: '#F0F1F5', type: 'dashed' as const } }
const axisLine   = { lineStyle: { color: '#E5E6EB' } }

const qpsTrendOption = computed(() => {
  const d = trendData.value
  return {
    backgroundColor: 'transparent',
    grid: { left: 12, right: 16, top: 52, bottom: 4, containLabel: true },
    tooltip: {
      ...tooltipStyle,
      formatter: (params: any[]) => {
        const t = params[0]?.axisValueLabel ?? ''
        return `<div style="font-weight:600;margin-bottom:6px;color:#4E5969">${t}</div>` +
          params.map(p =>
            `<div style="display:flex;align-items:center;gap:8px;line-height:22px">${p.marker}<span style="flex:1">${p.seriesName}</span><b>${p.seriesName.includes('QPS') ? p.value.toLocaleString() : p.value + ' ms'}</b></div>`
          ).join('')
      },
    },
    legend: {
      top: 10, right: 16,
      itemWidth: 20, itemHeight: 3, itemGap: 20,
      textStyle: { color: '#4E5969', fontSize: 12 },
      icon: 'roundRect',
    },
    xAxis: {
      type: 'category', data: d.times, boundaryGap: false,
      axisLine: axisLine, axisTick: { show: false }, axisLabel,
    },
    yAxis: [
      {
        type: 'value', name: 'QPS',
        nameLocation: 'end' as const,
        nameTextStyle: { color: '#86909C', fontSize: 11, padding: [0, 32, 0, 0] },
        splitLine, axisLabel, axisLine: { show: false }, axisTick: { show: false },
        min: (v: { min: number }) => Math.floor(v.min * 0.85 / 1000) * 1000,
      },
      {
        type: 'value', name: 'ms',
        nameLocation: 'end' as const,
        nameTextStyle: { color: '#86909C', fontSize: 11 },
        splitLine: { show: false }, axisLabel, axisLine: { show: false }, axisTick: { show: false },
        min: 0,
      },
    ],
    series: [
      {
        name: t('cluster.totalQps'), type: 'line', smooth: 0.5, data: d.qps, yAxisIndex: 0,
        symbol: 'circle', symbolSize: 6, showSymbol: false,
        emphasis: { scale: true, focus: 'series' as const },
        itemStyle: { color: '#165DFF', borderWidth: 2, borderColor: '#fff' },
        lineStyle: { color: '#165DFF', width: 2.5 },
        areaStyle: {
          color: {
            type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(22,93,255,0.18)' },
              { offset: 0.7, color: 'rgba(22,93,255,0.05)' },
              { offset: 1, color: 'rgba(22,93,255,0)' },
            ],
          },
        },
      },
      {
        name: t('dashboard.avgLatency'), type: 'line', smooth: 0.5, data: d.latency, yAxisIndex: 1,
        symbol: 'circle', symbolSize: 6, showSymbol: false,
        emphasis: { scale: true, focus: 'series' as const },
        itemStyle: { color: '#FF7D00', borderWidth: 2, borderColor: '#fff' },
        lineStyle: { color: '#FF7D00', width: 2, type: 'dashed' as const },
      },
    ],
  }
})

const nodeQpsOption = computed(() => ({
  backgroundColor: 'transparent',
  grid: { left: 12, right: 16, top: 20, bottom: 4, containLabel: true },
  tooltip: {
    ...tooltipStyle,
    formatter: (params: { name: string; value: number; marker: string }[]) => {
      const p = params[0]
      return `${p.marker} <b>${p.name}</b><br/>QPS: <b style="color:#165DFF">${p.value.toLocaleString()}</b>`
    },
  },
  xAxis: {
    type: 'category',
    data: nodes.value.filter(n => n.qps > 0).map(n => n.name.replace('dns-', '')),
    axisLine: axisLine, axisTick: { show: false },
    axisLabel: { ...axisLabel, interval: 0 },
  },
  yAxis: {
    type: 'value', name: 'QPS',
    nameTextStyle: { color: '#86909C', fontSize: 11 },
    splitLine, axisLabel, axisLine: { show: false }, axisTick: { show: false },
  },
  series: [{
    name: 'QPS', type: 'bar', barMaxWidth: 40,
    data: nodes.value.filter(n => n.qps > 0).map(n => n.qps),
    itemStyle: {
      color: {
        type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1,
        colorStops: [
          { offset: 0, color: '#165DFF' },
          { offset: 1, color: 'rgba(22,93,255,0.25)' },
        ],
      },
      borderRadius: [4, 4, 0, 0],
    },
    emphasis: {
      itemStyle: {
        color: {
          type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: '#3C7EFF' },
            { offset: 1, color: 'rgba(60,126,255,0.35)' },
          ],
        },
      },
    },
  }],
}))

const syncLagOption = computed(() => ({
  backgroundColor: 'transparent',
  grid: { left: 12, right: 16, top: 20, bottom: 4, containLabel: true },
  tooltip: {
    ...tooltipStyle,
    formatter: (params: { name: string; value: number; marker: string }[]) => {
      const p = params[0]
      const color = p.value > 100 ? '#F53F3F' : p.value > 30 ? '#FF7D00' : '#00B42A'
      return `${p.marker} <b>${p.name}</b><br/>${t('cluster.syncDelay')}: <b style="color:${color}">${p.value} ms</b>`
    },
  },
  xAxis: {
    type: 'category',
    data: nodes.value.filter(n => n.role !== '仲裁节点' && n.syncLag !== 99999).map(n => n.name.replace('dns-', '')),
    axisLine: axisLine, axisTick: { show: false },
    axisLabel: { ...axisLabel, interval: 0 },
  },
  yAxis: {
    type: 'value', name: 'ms',
    nameTextStyle: { color: '#86909C', fontSize: 11 },
    splitLine, axisLabel, axisLine: { show: false }, axisTick: { show: false },
    min: 0,
  },
  series: [{
    name: t('cluster.syncDelay'), type: 'bar', barMaxWidth: 40,
    data: nodes.value.filter(n => n.role !== '仲裁节点' && n.syncLag !== 99999).map(n => ({
      value: n.syncLag,
      itemStyle: {
        color: n.syncLag > 100
          ? { type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: '#F53F3F' }, { offset: 1, color: 'rgba(245,63,63,0.25)' }] }
          : n.syncLag > 30
            ? { type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: '#FF7D00' }, { offset: 1, color: 'rgba(255,125,0,0.25)' }] }
            : { type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: '#00B42A' }, { offset: 1, color: 'rgba(0,180,42,0.25)' }] },
      },
    })),
    itemStyle: { borderRadius: [4, 4, 0, 0] },
  }],
}))
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('cluster.overview') }}</h1>
        <p class="page-subtitle">{{ $t('cluster.overviewSubtitle') }}</p>
      </div>
      <el-radio-group v-if="clusterReady" v-model="timeRange" size="small">
        <el-radio-button v-for="r in [{ label: $t('cluster.oneHour'), value:'1h' },{ label: $t('dashboard.today'), value:'1d' },{ label: $t('dashboard.last7d'), value:'7d' }]" :key="r.value" :label="r.value">{{ r.label }}</el-radio-button>
      </el-radio-group>
    </div>

    <!-- Cluster-not-initialized placeholder. Replaces the dashboard so an
         operator who lands here on a fresh deploy gets a clear next step. -->
    <div v-if="!loading && !clusterReady" class="empty-bootstrap">
      <div class="empty-bootstrap-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      </div>
      <div class="empty-bootstrap-body">
        <h3>{{ $t('cluster.notInitTitle') }}</h3>
        <p>{{ $t('cluster.notInitOverviewDesc') }}</p>
      </div>
      <router-link to="/cluster" class="empty-bootstrap-link">
        <el-button type="primary">{{ $t('cluster.goToInit') }}</el-button>
      </router-link>
    </div>

    <!-- KPI -->
    <div v-if="clusterReady" class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success">
        <span class="mn-stat-value">{{ onlineCount }}</span><span class="mn-stat-label">{{ $t('cluster.onlineNodes') }}</span>
      </div>
      <div :class="['mn-stat-tile', offlineCount > 0 ? 'mn-stat-tile--danger' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ offlineCount }}</span><span class="mn-stat-label">{{ $t('cluster.offlineNodes') }}</span>
      </div>
      <div :class="['mn-stat-tile', syncIssues > 0 ? 'mn-stat-tile--warning' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ syncIssues }}</span><span class="mn-stat-label">{{ $t('cluster.syncIssues') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--accent">
        <span class="mn-stat-value">{{ totalQps.toLocaleString() }}</span><span class="mn-stat-label">{{ $t('cluster.totalQps') }}</span>
      </div>
      <div :class="['mn-stat-tile', avgCpu >= 80 ? 'mn-stat-tile--danger' : avgCpu >= 60 ? 'mn-stat-tile--warning' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ avgCpu }}%</span><span class="mn-stat-label">{{ $t('cluster.avgCpu') }}</span>
      </div>
      <div :class="['mn-stat-tile', avgMem >= 80 ? 'mn-stat-tile--danger' : avgMem >= 60 ? 'mn-stat-tile--warning' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ avgMem }}%</span><span class="mn-stat-label">{{ $t('cluster.avgMem') }}</span>
      </div>
    </div>

    <!-- QPS + Latency trend -->
    <el-card v-if="clusterReady" class="mn-card">
      <div class="mn-chart-header">
        <div class="mn-chart-title">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14" style="color:var(--app-accent)"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
          {{ $t('cluster.qpsTrend') }}
        </div>
      </div>
      <div style="height:240px">
        <BaseChart :option="qpsTrendOption" :loading="false" :empty="false" />
      </div>
    </el-card>

    <!-- Node cards -->
    <el-empty v-if="clusterReady && !nodes.length && !loading" :description="$t('cluster.noNodes')" style="margin:24px 0" />
    <div v-else-if="clusterReady" class="node-grid">
      <div v-for="node in nodes" :key="node.name" class="node-card" :class="{ 'node-card--offline': node.status === '离线', 'node-card--warn': node.status === '同步中' }">
        <div class="node-card-header">
          <div class="node-name-row">
            <span class="node-dot" :class="'node-dot--' + (node.status === '在线' ? 'ok' : node.status === '离线' ? 'off' : 'warn')"></span>
            <span class="node-name mn-mono">{{ node.name }}</span>
          </div>
          <span class="type-badge" :style="{ background: roleColor[node.role] }">{{ node.role }}</span>
        </div>
        <div class="node-meta">
          <span class="meta-item">{{ node.ip }}</span>
          <span class="meta-sep">·</span>
          <span class="meta-item">{{ node.zone }}</span>
          <span class="meta-sep">·</span>
          <span class="meta-item">{{ node.version }}</span>
        </div>
        <div class="node-metrics">
          <div class="node-metric">
            <span class="nm-label">QPS</span>
            <span class="nm-value" :style="{ color: 'var(--app-accent)' }">{{ node.qps.toLocaleString() }}</span>
          </div>
          <div class="node-metric">
            <span class="nm-label">{{ $t('cluster.syncDelay') }}</span>
            <span class="nm-value" :style="{ color: node.syncLag > 100 ? 'var(--app-danger)' : node.syncLag > 30 ? 'var(--app-warning)' : 'var(--app-success)' }">
              {{ node.syncLag === 99999 ? '—' : node.syncLag + ' ms' }}
            </span>
          </div>
        </div>
        <div class="node-usage">
          <div class="usage-row">
            <span class="usage-label">CPU</span>
            <el-progress :percentage="node.cpuUsage" :color="usageColor(node.cpuUsage)" :stroke-width="5" :show-text="false" style="flex:1" />
            <span class="usage-pct">{{ node.cpuUsage }}%</span>
          </div>
          <div class="usage-row">
            <span class="usage-label">{{ $t('cluster.memory') }}</span>
            <el-progress :percentage="node.memUsage" :color="usageColor(node.memUsage)" :stroke-width="5" :show-text="false" style="flex:1" />
            <span class="usage-pct">{{ node.memUsage }}%</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom charts row -->
    <div v-if="clusterReady" class="chart-row">
      <el-card class="mn-card chart-col">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14" style="color:var(--app-accent)"><rect x="3" y="3" width="4" height="18"/><rect x="10" y="8" width="4" height="13"/><rect x="17" y="13" width="4" height="8"/></svg>
            {{ $t('cluster.nodeQpsDistribution') }}
          </div>
        </div>
        <div style="height:200px">
          <BaseChart :option="nodeQpsOption" :loading="false" :empty="false" />
        </div>
      </el-card>
      <el-card class="mn-card chart-col">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14" style="color:var(--app-accent)"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
            {{ $t('cluster.syncLag') }}
          </div>
        </div>
        <div style="height:200px">
          <BaseChart :option="syncLagOption" :loading="false" :empty="false" />
        </div>
      </el-card>
    </div>
  </div>
</template>

<style scoped>
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-disabled); }

.mn-chart-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:8px; padding:14px 16px 0; }
.mn-chart-title { display:flex; align-items:center; gap:6px; font-size:14px; font-weight:600; color:var(--app-text-primary,#1d2129); }

.node-grid { display:grid; grid-template-columns:repeat(auto-fill, minmax(280px, 1fr)); gap:14px; }

.node-card {
  padding: 16px;
  border-radius: 10px;
  border: 1px solid var(--app-border);
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: box-shadow 0.2s;
}
.node-card:hover { box-shadow: 0 4px 16px rgba(0,0,0,0.1); }
.node-card--offline { opacity: 0.6; }
.node-card--warn { border-color: #fde68a; }

.node-card-header { display:flex; align-items:center; justify-content:space-between; }
.node-name-row { display:flex; align-items:center; gap:7px; }
.node-dot { width:8px; height:8px; border-radius:50%; flex-shrink:0; }
.node-dot--ok  { background:#10b981; box-shadow:0 0 0 3px rgba(16,185,129,0.15); }
.node-dot--off { background:#ef4444; }
.node-dot--warn { background:#f59e0b; box-shadow:0 0 0 3px rgba(245,158,11,0.15); animation:pulse 1.5s ease-in-out infinite; }
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.5} }
.node-name { font-size:13px; font-weight:600; color:var(--app-text-primary,#1d2129); }
.type-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; color:#fff; }

.node-meta { display:flex; align-items:center; gap:4px; font-size:11px; color:var(--app-text-secondary); }
.meta-sep { color:var(--app-border); }

.node-metrics { display:flex; gap:16px; }
.node-metric { display:flex; flex-direction:column; gap:2px; }
.nm-label { font-size:11px; color:var(--app-text-secondary); }
.nm-value { font-size:16px; font-weight:700; font-variant-numeric:tabular-nums; }

.node-usage { display:flex; flex-direction:column; gap:5px; }
.usage-row { display:flex; align-items:center; gap:8px; }
.usage-label { font-size:11px; color:var(--app-text-secondary); width:28px; flex-shrink:0; }
.usage-pct { font-size:11px; font-variant-numeric:tabular-nums; color:var(--app-text-secondary); width:32px; text-align:right; flex-shrink:0; }

.chart-row { display:grid; grid-template-columns:1fr 1fr; gap:14px; }
.chart-col :deep(.el-card__body) { padding: 0 0 14px; }

@media (max-width: 900px) {
  .chart-row { grid-template-columns:1fr; }
}

/* Cluster-not-initialized empty state */
.empty-bootstrap {
  display:flex; align-items:center; gap:18px;
  padding:24px 28px; border-radius:12px;
  border:1px dashed var(--app-accent-muted);
  background:var(--app-accent-soft);
}
.empty-bootstrap-icon { width:48px; height:48px; flex-shrink:0; display:flex; align-items:center; justify-content:center; border-radius:50%; background:#fff; color:var(--app-accent); }
.empty-bootstrap-icon svg { width:24px; height:24px; }
.empty-bootstrap-body { flex:1; min-width:0; }
.empty-bootstrap-body h3 { margin:0 0 4px; font-size:15px; color:var(--dns-text-title-color); }
.empty-bootstrap-body p { margin:0; font-size:13px; color:var(--dns-text-body-color); line-height:1.6; }
.empty-bootstrap-link { flex-shrink:0; }
</style>
