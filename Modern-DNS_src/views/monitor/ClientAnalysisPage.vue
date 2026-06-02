<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import BaseChart from '../../components/BaseChart.vue'
import { getClientAnalysis } from '../../api/monitor'

const { t } = useI18n()
const timeRange = ref('1d')
const keyword   = ref('')
const filterRegion = ref('')
const loading = ref(false)

interface ClientRow {
  ip: string
  region: string
  city: string
  isp: string
  queryCount: number
  nxdomainCount: number
  errorRate: number
  topDomain: string
  lastSeen: string
  blocked: boolean
}

const clients = ref<ClientRow[]>([])
const trendPoints = ref<{ label: string; count: number }[]>([])
const geoData = ref<{ name: string; value: number }[]>([])
const recordTypeData = ref<{ name: string; value: number }[]>([])

const fetchData = async () => {
  loading.value = true
  try {
    const { data } = await getClientAnalysis({ range: timeRange.value, keyword: keyword.value, region: filterRegion.value })
    clients.value = (data?.clients ?? []).map((c: any) => ({ ...c, city: c.city ?? '', isp: c.isp ?? '', blocked: c.blocked ?? false }))
    trendPoints.value = data?.trend ?? []
    geoData.value = data?.geoDist ?? []
    recordTypeData.value = data?.typeDist ?? []
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchData())
watch([timeRange, keyword, filterRegion], () => fetchData())

const filtered = computed(() => clients.value)

const totalClients   = computed(() => clients.value.length)
const blockedClients = computed(() => clients.value.filter(c => c.blocked).length)
const totalQueries   = computed(() => clients.value.reduce((s, c) => s + c.queryCount, 0))
const highRiskCount  = computed(() => clients.value.filter(c => c.errorRate > 2).length)
const avgErrorRate   = computed(() => {
  if (!clients.value.length) return '0.00'
  return (clients.value.reduce((s, c) => s + c.errorRate, 0) / clients.value.length).toFixed(2)
})

const toggleBlock = (row: ClientRow) => {
  row.blocked = !row.blocked
  ElMessage.success(t('monitor.blockToggleSuccess', {
    ip: row.ip,
    status: row.blocked ? t('monitor.blockedStatus') : t('monitor.unblockedStatus'),
  }))
}

const trendOption = computed(() => {
  const x = trendPoints.value.map(t => t.label)
  const d = trendPoints.value.map(t => t.count)
  return {
    grid: { left: 16, right: 16, top: 32, bottom: 28, containLabel: true },
    tooltip: { trigger: 'axis', confine: true },
    xAxis: { type: 'category', data: x, axisLabel: { color: '#888', fontSize: 11 }, axisLine: { lineStyle: { color: '#e5e7eb' } } },
    yAxis: { type: 'value', name: t('monitor.queryCount'), splitLine: { lineStyle: { color: '#f0f0f0' } }, axisLabel: { color: '#888' } },
    series: [{
      name: t('monitor.clientQueryVolume'),
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: d,
      areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(22,93,255,0.18)' }, { offset: 1, color: 'rgba(22,93,255,0.01)' }] } },
      lineStyle: { color: '#165dff', width: 2 },
    }],
  }
})

const geoOption = computed(() => ({
  tooltip: { trigger: 'item' },
  legend: { bottom: 0, textStyle: { color: '#888', fontSize: 11 } },
  series: [{
    name: t('monitor.geoDist'), type: 'pie', radius: ['40%', '68%'],
    label: { formatter: '{b}\n{d}%', fontSize: 11 },
    data: geoData.value,
    color: ['#165dff','#36cfc9','#ff7d00','#00b42a','#9254de','#f5222d'],
  }],
}))

const recordTypeOption = computed(() => ({
  tooltip: { trigger: 'axis', confine: true },
  grid: { left: 16, right: 16, top: 20, bottom: 20, containLabel: true },
  xAxis: { type: 'value', axisLabel: { color: '#888' }, splitLine: { lineStyle: { color: '#f0f0f0' } } },
  yAxis: { type: 'category', data: recordTypeData.value.map(d => d.name), axisLabel: { color: '#888' } },
  series: [{
    name: t('monitor.queryCount'), type: 'bar', barMaxWidth: 32,
    data: recordTypeData.value.map(d => d.value),
    itemStyle: { color: '#36cfc9', borderRadius: [0, 4, 4, 0] },
  }],
}))

const ispOption = computed(() => ({
  tooltip: { trigger: 'item' },
  legend: { bottom: 0, textStyle: { color: '#888', fontSize: 11 } },
  series: [{
    name: t('monitor.ispDist'), type: 'pie', radius: ['40%', '68%'],
    label: { formatter: '{b}\n{d}%', fontSize: 11 },
    data: [],
    color: ['#165dff','#ff7d00','#00b42a'],
  }],
}))

const errorRateColor = (v: number) => v > 3 ? 'var(--app-danger)' : v > 1 ? 'var(--app-warning)' : 'var(--app-success)'
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('monitor.clientAnalysis') }}</h1>
        <p class="page-subtitle">{{ $t('monitor.clientAnalysisSubtitle') }}</p>
      </div>
    </div>

    <!-- KPI -->
    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--accent">
        <span class="mn-stat-value">{{ totalClients }}</span>
        <span class="mn-stat-label">{{ $t('monitor.activeClients') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--accent">
        <span class="mn-stat-value">{{ totalQueries.toLocaleString() }}</span>
        <span class="mn-stat-label">{{ $t('monitor.todayTotalQueries') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--danger">
        <span class="mn-stat-value">{{ highRiskCount }}</span>
        <span class="mn-stat-label">{{ $t('monitor.highRiskClients') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--danger">
        <span class="mn-stat-value">{{ blockedClients }}</span>
        <span class="mn-stat-label">{{ $t('monitor.blocked') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--warning">
        <span class="mn-stat-value">{{ avgErrorRate }}%</span>
        <span class="mn-stat-label">{{ $t('monitor.avgErrorRate') }}</span>
      </div>
    </div>

    <!-- Trend chart -->
    <el-card class="mn-card">
      <div class="mn-chart-header">
        <div class="mn-chart-title">
          <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
          {{ $t('monitor.clientQueryTrend') }}
        </div>
        <el-radio-group v-model="timeRange" size="small">
          <el-radio-button label="1h">{{ $t('monitor.last1h') }}</el-radio-button>
          <el-radio-button label="1d">{{ $t('dashboard.today') }}</el-radio-button>
          <el-radio-button label="7d">{{ $t('dashboard.last7d') }}</el-radio-button>
        </el-radio-group>
      </div>
      <div style="height:240px">
        <BaseChart :option="trendOption" :loading="false" :empty="false" />
      </div>
    </el-card>

    <!-- Distribution charts row -->
    <div class="chart-row">
      <el-card class="mn-card chart-row-item">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
            {{ $t('monitor.geoDist') }}
          </div>
        </div>
        <div style="height:240px">
          <BaseChart :option="geoOption" :loading="false" :empty="false" />
        </div>
      </el-card>

      <el-card class="mn-card chart-row-item">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
            {{ $t('monitor.queryTypeDist') }}
          </div>
        </div>
        <div style="height:240px">
          <BaseChart :option="recordTypeOption" :loading="false" :empty="false" />
        </div>
      </el-card>

      <el-card class="mn-card chart-row-item">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
            {{ $t('monitor.ispDist') }}
          </div>
        </div>
        <div style="height:240px">
          <BaseChart :option="ispOption" :loading="false" :empty="false" />
        </div>
      </el-card>
    </div>

    <!-- Top clients table -->
    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="keyword" clearable :placeholder="$t('monitor.searchIpCityDomain')" style="width:240px">
            <template #prefix>
              <svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
            </template>
          </el-input>
          <el-select v-model="filterRegion" clearable :placeholder="$t('monitor.region')" style="width:110px">
            <el-option v-for="r in ['华东','华北','华南','华中','西南','西北']" :key="r" :label="r" :value="r" />
          </el-select>
        </div>
      </div>

      <el-table :data="filtered" stripe class="mn-table" table-layout="auto" :default-sort="{ prop: 'queryCount', order: 'descending' }">
        <el-table-column type="index" label="#" width="48" align="center" />
        <el-table-column prop="ip" :label="$t('monitor.clientIp')" min-width="150" sortable>
          <template #default="{ row }">
            <span class="mn-mono client-ip" :class="{ 'is-blocked': row.blocked }">{{ row.ip }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="region" :label="$t('monitor.region')" width="72" align="center">
          <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.region }}</span></template>
        </el-table-column>
        <el-table-column prop="city" :label="$t('monitor.city')" width="80" align="center" />
        <el-table-column prop="isp" label="ISP" width="72" align="center">
          <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.isp }}</span></template>
        </el-table-column>
        <el-table-column prop="queryCount" :label="$t('monitor.queryCount')" width="110" align="right" sortable>
          <template #default="{ row }"><span class="mn-mono">{{ row.queryCount.toLocaleString() }}</span></template>
        </el-table-column>
        <el-table-column prop="nxdomainCount" label="NXDOMAIN" width="115" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: row.nxdomainCount > 200 ? 'var(--app-warning)' : 'inherit' }">{{ row.nxdomainCount.toLocaleString() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="errorRate" :label="$t('monitor.errorRate')" width="90" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: errorRateColor(row.errorRate), fontWeight: row.errorRate > 2 ? 700 : 400 }">{{ row.errorRate }}%</span>
          </template>
        </el-table-column>
        <el-table-column prop="topDomain" :label="$t('monitor.topDomain')" min-width="180" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.topDomain }}</span></template>
        </el-table-column>
        <el-table-column prop="lastSeen" :label="$t('monitor.lastActive')" min-width="160">
          <template #default="{ row }"><span class="mn-time">{{ row.lastSeen }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.status')" width="80" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', row.blocked ? 'mn-badge--danger' : 'mn-badge--success']">{{ row.blocked ? $t('monitor.blocked') : $t('monitor.normal') }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="90" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link :type="row.blocked ? 'success' : 'danger'" @click="toggleBlock(row)">
              {{ row.blocked ? $t('monitor.unblock') : $t('monitor.block') }}
            </el-button>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('monitor.noClientData')" /></template>
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:column; gap:4px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }

.mn-card { border:1px solid var(--app-border); border-radius:10px; box-shadow:var(--app-shadow-soft); overflow:hidden; }
.mn-card :deep(.el-card__body) { padding:0; }

.mn-chart-header { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:12px 16px; background:var(--app-bg-secondary); border-bottom:1px solid var(--app-border); }
.mn-chart-title  { display:flex; align-items:center; gap:7px; font-size:13px; font-weight:600; color:var(--app-title); }
.mn-chart-icon   { width:15px; height:15px; color:var(--app-accent); flex-shrink:0; }

.chart-row { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; }
@media (max-width:1100px) { .chart-row { grid-template-columns:1fr 1fr; } }
@media (max-width:700px)  { .chart-row { grid-template-columns:1fr; } }

.mn-toolbar { display:flex; align-items:center; justify-content:space-between; gap:10px; flex-wrap:wrap; padding:12px 16px; background:var(--app-bg-secondary); border-bottom:1px solid var(--app-border); }
.mn-toolbar-filters { display:flex; align-items:center; flex-wrap:wrap; gap:8px; flex:1; }
.mn-input-icon { width:13px; height:13px; color:var(--app-text-regular); }

.mn-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 8px; border-radius:20px; font-size:11px; font-weight:500; line-height:1.6; border:1px solid transparent; white-space:nowrap; }
.mn-badge--neutral { color:var(--app-text-secondary); background:rgba(78,89,105,0.08); border-color:rgba(78,89,105,0.18); }
.mn-badge--success { color:var(--app-success); background:rgba(0,180,42,0.08); border-color:rgba(0,180,42,0.22); }
.mn-badge--danger  { color:var(--app-danger);  background:rgba(245,63,63,0.08); border-color:rgba(245,63,63,0.22); }

.mn-mono  { font-family:var(--app-font-mono,'JetBrains Mono','Consolas',monospace); font-size:12px; }
.mn-time  { font-size:12px; font-variant-numeric:tabular-nums; color:var(--app-text-regular); }

.client-ip { font-weight:600; }
.client-ip.is-blocked { color:var(--app-danger); text-decoration:line-through; opacity:0.7; }

.mn-table :deep(.el-table__header th) { background:var(--app-bg-secondary); color:var(--app-text-secondary); font-size:12px; font-weight:600; text-transform:uppercase; letter-spacing:0.04em; }
.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) { background:rgba(22,93,255,0.04) !important; }
</style>
