<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseChart from '../../components/BaseChart.vue'
import { getSlowQueries } from '../../api/monitor'

const { t } = useI18n()

interface SlowQuery {
  id: number
  domain: string
  queryType: string
  avgLatency: number
  maxLatency: number
  p95Latency: number
  count: number
  errorRate: number
  clientIp: string
  lastSeen: string
  trend: 'up' | 'down' | 'stable'
}

const rows = ref<SlowQuery[]>([])

const threshold = ref(100)
const filterType = ref('')
const keyword = ref('')
const timeRange = ref('1h')
const loading = ref(false)

const trendData = ref<{ times: string[]; avg: number[]; p95: number[] }>({ times: [], avg: [], p95: [] })

const fetchData = async () => {
  loading.value = true
  try {
    const { data } = await getSlowQueries({ threshold: threshold.value, keyword: keyword.value, queryType: filterType.value, range: timeRange.value })
    rows.value = (data?.rows ?? []).map((r: any, i: number) => ({ ...r, id: r.id ?? i + 1 }))
    const trend = data?.trend ?? []
    trendData.value = {
      times: trend.map((t: any) => t.label),
      avg: trend.map((t: any) => t.avg),
      p95: trend.map((t: any) => t.p95),
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchData())
watch([timeRange, threshold, filterType, keyword], () => fetchData(), { debounce: 300 } as any)

const filteredRows = computed(() => rows.value)

const totalSlowCount = computed(() => filteredRows.value.reduce((s, r) => s + r.count, 0))
const avgLatency = computed(() => {
  if (!filteredRows.value.length) return 0
  return Math.round(filteredRows.value.reduce((s, r) => s + r.avgLatency, 0) / filteredRows.value.length)
})
const maxLatency = computed(() => Math.max(...filteredRows.value.map(r => r.maxLatency), 0))

const trendChartOption = computed(() => {
  const d = trendData.value
  return {
    backgroundColor: 'transparent',
    grid: { left: 16, right: 24, top: 56, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(15,23,42,0.88)',
      borderWidth: 0,
      textStyle: { color: '#f1f5f9', fontSize: 12 },
      formatter: (params: any[]) => {
        const time = params[0]?.axisValue ?? ''
        return [
          `<div style="font-weight:700;margin-bottom:6px;color:#94a3b8">${time}</div>`,
          ...params.map((p: any) =>
            `<div style="display:flex;align-items:center;gap:8px;margin:2px 0">
              <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
              <span style="flex:1">${p.seriesName}</span>
              <strong style="color:#fff">${p.value} ms</strong>
            </div>`
          ),
        ].join('')
      },
    },
    legend: {
      top: 8,
      right: 12,
      itemWidth: 12,
      itemHeight: 12,
      itemGap: 20,
      textStyle: { color: '#64748b', fontSize: 12 },
      icon: 'circle',
    },
    xAxis: {
      type: 'category',
      data: d.times,
      boundaryGap: false,
      axisLine: { lineStyle: { color: '#e2e8f0' } },
      axisTick: { show: false },
      axisLabel: { color: '#94a3b8', fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      name: 'ms',
      nameTextStyle: { color: '#94a3b8', fontSize: 11 },
      splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } },
      axisLabel: { color: '#94a3b8', fontSize: 11 },
      axisLine: { show: false },
      axisTick: { show: false },
    },
    series: [
      {
        name: t('monitor.avgLatencyLabel'),
        type: 'line',
        smooth: 0.4,
        data: d.avg,
        symbol: 'circle',
        symbolSize: 5,
        itemStyle: { color: '#f59e0b', borderWidth: 2, borderColor: '#fff' },
        lineStyle: { color: '#f59e0b', width: 2.5 },
        areaStyle: {
          color: {
            type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(245,158,11,0.25)' },
              { offset: 1, color: 'rgba(245,158,11,0.02)' },
            ],
          },
        },
      },
      {
        name: t('monitor.p95Latency'),
        type: 'line',
        smooth: 0.4,
        data: d.p95,
        symbol: 'circle',
        symbolSize: 5,
        itemStyle: { color: '#ef4444', borderWidth: 2, borderColor: '#fff' },
        lineStyle: { color: '#ef4444', width: 2.5 },
        areaStyle: {
          color: {
            type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(239,68,68,0.18)' },
              { offset: 1, color: 'rgba(239,68,68,0.01)' },
            ],
          },
        },
      },
    ],
  }
})

const latencyColor = (v: number) => v >= 300 ? 'var(--app-danger)' : v >= 150 ? 'var(--app-warning)' : 'var(--app-success)'
const trendIcon = (t: string) => t === 'up' ? '↑' : t === 'down' ? '↓' : '→'
const trendColor = (t: string) => t === 'up' ? 'var(--app-danger)' : t === 'down' ? 'var(--app-success)' : 'var(--app-text-secondary)'
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('monitor.slowQuery') }}</h1>
        <p class="page-subtitle">{{ $t('monitor.slowQuerySubtitle') }}</p>
      </div>
    </div>

    <!-- KPI -->
    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ filteredRows.length }}</span><span class="mn-stat-label">{{ $t('monitor.slowQueryDomains') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--warning"><span class="mn-stat-value">{{ avgLatency }}</span><span class="mn-stat-label">{{ $t('monitor.avgLatencyMs') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ maxLatency }}</span><span class="mn-stat-label">{{ $t('monitor.maxLatencyMs') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--accent"><span class="mn-stat-value">{{ totalSlowCount.toLocaleString() }}</span><span class="mn-stat-label">{{ $t('monitor.slowQueryTotal') }}</span></div>
    </div>

    <!-- Trend chart -->
    <el-card class="mn-card">
      <div class="mn-chart-header">
        <div class="mn-chart-title">
          <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
          {{ $t('monitor.latencyTrend') }}（{{ timeRange === '1h' ? $t('monitor.last1h') : timeRange === '1d' ? $t('dashboard.today') : $t('dashboard.last7d') }}）
        </div>
        <el-radio-group v-model="timeRange" size="small">
          <el-radio-button v-for="r in [{ label: $t('cluster.oneHour'), value:'1h' },{ label: $t('dashboard.today'), value:'1d' },{ label: $t('dashboard.last7d'), value:'7d' }]" :key="r.value" :label="r.value">{{ r.label }}</el-radio-button>
        </el-radio-group>
      </div>
      <div style="height:260px">
        <BaseChart :option="trendChartOption" :loading="false" :empty="false" />
      </div>
    </el-card>

    <!-- Table -->
    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="keyword" clearable :placeholder="$t('monitor.searchDomain')" style="width:220px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterType" clearable :placeholder="$t('monitor.recordType')" style="width:120px">
            <el-option v-for="t in ['A','AAAA','MX','CNAME','TXT']" :key="t" :label="t" :value="t" />
          </el-select>
          <div class="threshold-filter">
            <span class="threshold-label">{{ $t('monitor.latencyThreshold') }}</span>
            <el-slider v-model="threshold" :min="50" :max="500" :step="50" show-input style="width:280px" />
            <span class="threshold-unit">ms</span>
          </div>
        </div>
      </div>

      <el-table :data="filteredRows" stripe class="mn-table" table-layout="auto" :default-sort="{ prop: 'avgLatency', order: 'descending' }">
        <el-table-column type="index" label="#" width="50" align="center" />
        <el-table-column prop="domain" :label="$t('common.domain')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
        </el-table-column>
        <el-table-column prop="queryType" :label="$t('common.type')" width="80" align="center">
          <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.queryType }}</span></template>
        </el-table-column>
        <el-table-column prop="avgLatency" :label="$t('monitor.avgLatencyLabel')" width="120" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono latency-val" :style="{ color: latencyColor(row.avgLatency) }">{{ row.avgLatency }} ms</span>
          </template>
        </el-table-column>
        <el-table-column prop="p95Latency" :label="$t('monitor.p95Latency')" width="120" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: latencyColor(row.p95Latency) }">{{ row.p95Latency }} ms</span>
          </template>
        </el-table-column>
        <el-table-column prop="maxLatency" :label="$t('monitor.maxLatencyLabel')" width="120" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: latencyColor(row.maxLatency) }">{{ row.maxLatency }} ms</span>
          </template>
        </el-table-column>
        <el-table-column prop="count" :label="$t('monitor.queryCount')" width="110" align="right" sortable>
          <template #default="{ row }"><span class="mn-mono">{{ row.count.toLocaleString() }}</span></template>
        </el-table-column>
        <el-table-column prop="errorRate" :label="$t('monitor.errorRate')" width="90" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: row.errorRate > 2 ? 'var(--app-danger)' : 'var(--app-text-primary)' }">{{ row.errorRate }}%</span>
          </template>
        </el-table-column>
        <el-table-column prop="trend" :label="$t('monitor.trend')" width="70" align="center">
          <template #default="{ row }">
            <span class="trend-icon" :style="{ color: trendColor(row.trend) }">{{ trendIcon(row.trend) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="lastSeen" :label="$t('monitor.lastSeen')" min-width="160">
          <template #default="{ row }"><span class="mn-time">{{ row.lastSeen }}</span></template>
        </el-table-column>
        <template #empty><el-empty :description="$t('monitor.noSlowQueries')" /></template>
      </el-table>
    </el-card>
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
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }
.mn-chart-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:12px; }
.mn-chart-title { display:flex; align-items:center; gap:6px; font-size:14px; font-weight:600; color:var(--app-text-primary,#1d2129); }
.mn-chart-icon { color:var(--app-accent); }
.threshold-filter { display:flex; align-items:center; gap:10px; }
.threshold-label { font-size:12px; color:var(--app-text-secondary); white-space:nowrap; }
.threshold-unit { font-size:12px; color:var(--app-text-secondary); }
.latency-val { font-weight:700; }
.trend-icon { font-size:16px; font-weight:700; }
</style>
