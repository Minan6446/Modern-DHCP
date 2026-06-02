<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BaseChart from '../../components/BaseChart.vue'
import { useRealTimeMonitor } from '../../composables/monitor/useRealTimeMonitor'
import type { MonitorRealTimeRow } from '../../types/modules'

const route = useRoute()
const { t } = useI18n()

const {
  recordTypeOptions,
  regionOptions,
  statusOptions,
  refreshIntervalOptions,
  filterForm,
  chartData,
  chartOptions,
  sortedTableData,
  pagedTableData,
  pagination,
  loading,
  queryLoading,
  refreshing,
  exportLoading,
  autoRefreshEnabled,
  refreshInterval,
  lastUpdated,
  detailVisible,
  detailRow,
  formatDateTime,
  fetchData,
  applyFilters,
  resetFilters,
  handleSortChange,
  setRefreshInterval,
  openDetail,
  closeDetail,
  copyText,
  copyDetail,
  handleExport,
} = useRealTimeMonitor()

const pageTitle = computed(() => route.meta.titleKey ? t('menu.' + route.meta.titleKey) : t('monitor.realTime'))

const statusLabel = (status: string) => {
  const map: Record<string, string> = {
    成功: 'common.success',
    失败: 'common.failed',
  }
  return map[status] ? t(map[status]) : status
}

const regionLabel = (region: string) => {
  const map: Record<string, string> = {
    华东: 'monitor.regionEastChina',
    华北: 'monitor.regionNorthChina',
    华南: 'monitor.regionSouthChina',
    华中: 'monitor.regionCentralChina',
    西南: 'monitor.regionSouthwest',
    海外: 'monitor.regionOverseas',
  }
  return map[region] ? t(map[region]) : region
}

const statusClass = (status: string) => {
  if (status === '成功' || status === 'Success' || status === 'NOERROR') {
    return 'tag-success'
  }
  if (status === 'NXDOMAIN') {
    return 'tag-warning'
  }
  if (status === '失败' || status === 'Failed' || status === 'SERVFAIL' || status === 'REFUSED') {
    return 'tag-danger'
  }
  return 'tag-muted'
}

const realtimeRowClassName = ({ row }: { row: MonitorRealTimeRow }) =>
  row.responseStatus === '失败' || row.responseStatus === 'Failed' || row.responseStatus === 'SERVFAIL' ? 'monitor-row-danger' : ''
</script>

<template>
  <div class="page-shell realtime-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ pageTitle }}</h1>
        <p class="page-subtitle">{{ $t('monitor.realTimeSubtitle') }} {{ $t('common.lastRefresh') }}{{ lastUpdated || $t('common.loading') }}</p>
      </div>
    </div>

    <!-- ── 查询 + 表格 ── -->
    <el-card class="monitor-card" shadow="never">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="filterForm.domain" clearable :placeholder="$t('monitor.filterByDomain')" style="width: 180px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterForm.recordType" clearable :placeholder="$t('monitor.recordType')" style="width: 120px">
            <el-option v-for="item in recordTypeOptions" :key="item" :label="item" :value="item" />
          </el-select>
          <el-input v-model="filterForm.sourceIp" clearable :placeholder="$t('monitor.sourceIp')" style="width: 140px" />
          <el-select v-model="filterForm.region" clearable :placeholder="$t('monitor.region')" style="width: 120px">
            <el-option v-for="item in regionOptions" :key="item" :label="regionLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="filterForm.status" clearable :placeholder="$t('monitor.resolveStatusLabel')" style="width: 130px">
            <el-option v-for="item in statusOptions" :key="item" :label="statusLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="filterForm.timePreset" style="width: 120px">
            <el-option :label="$t('monitor.last1h')" value="1h" />
            <el-option :label="$t('monitor.last6h')" value="6h" />
            <el-option :label="$t('monitor.last24h')" value="24h" />
            <el-option :label="$t('monitor.custom')" value="custom" />
          </el-select>
          <el-date-picker
            v-if="filterForm.timePreset === 'custom'"
            v-model="filterForm.timeRange"
            type="datetimerange"
            value-format="YYYY-MM-DD HH:mm:ss"
            :range-separator="$t('monitor.rangeSeparator')"
            :start-placeholder="$t('monitor.startTime')"
            :end-placeholder="$t('monitor.endTime')"
          />
          <el-button type="primary" :loading="queryLoading" @click="applyFilters">{{ $t('monitor.query') }}</el-button>
          <el-button @click="resetFilters">{{ $t('common.reset') }}</el-button>
          <el-button :loading="refreshing" :disabled="refreshing" @click="fetchData({ refresh: true })">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ $t('common.refresh') }}
          </el-button>
        </div>
        <div class="mn-toolbar-actions">
          <el-switch v-model="autoRefreshEnabled" inline-prompt :active-text="$t('dashboard.autoRefresh')" :inactive-text="$t('dashboard.paused')" />
          <el-select :model-value="refreshInterval" style="width: 120px" @change="setRefreshInterval">
            <el-option v-for="item in refreshIntervalOptions" :key="item" :label="`${item / 1000} ${$t('monitor.seconds')}`" :value="item" />
          </el-select>
          <el-dropdown @command="(command) => { const [scope, format] = String(command).split('-'); handleExport(format as 'csv' | 'json' | 'excel', scope as 'all' | 'filtered') }">
            <el-button :loading="exportLoading" :disabled="exportLoading">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
              {{ $t('common.export') }}
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="filtered-excel">{{ $t('monitor.filteredExcel') }}</el-dropdown-item>
                <el-dropdown-item command="filtered-csv">{{ $t('monitor.filteredCsv') }}</el-dropdown-item>
                <el-dropdown-item command="filtered-json">{{ $t('monitor.filteredJson') }}</el-dropdown-item>
                <el-dropdown-item divided command="all-excel">{{ $t('monitor.allExcel') }}</el-dropdown-item>
                <el-dropdown-item command="all-csv">{{ $t('monitor.allCsv') }}</el-dropdown-item>
                <el-dropdown-item command="all-json">{{ $t('monitor.allJson') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <template v-if="sortedTableData.length">
        <el-table
          :data="pagedTableData"
          stripe
          highlight-current-row
          v-loading="loading"
          class="mn-table"
          :row-class-name="realtimeRowClassName"
          @sort-change="handleSortChange"
        >
          <el-table-column prop="queryId" :label="$t('monitor.queryId')" width="180" sortable="custom" />
          <el-table-column prop="time" :label="$t('monitor.time')" width="180" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ formatDateTime(row.time) }}</span></template>
          </el-table-column>
          <el-table-column prop="domain" :label="$t('monitor.queryDomain')" min-width="220" sortable="custom" show-overflow-tooltip />
          <el-table-column prop="recordType" :label="$t('monitor.recordType')" width="100" sortable="custom">
            <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.recordType }}</span></template>
          </el-table-column>
          <el-table-column prop="sourceIp" :label="$t('monitor.sourceIp')" width="150" sortable="custom">
            <template #default="{ row }"><span class="mn-mono">{{ row.sourceIp }}</span></template>
          </el-table-column>
          <el-table-column prop="region" :label="$t('monitor.region')" width="100" sortable="custom">
            <template #default="{ row }">{{ regionLabel(row.region) }}</template>
          </el-table-column>
          <el-table-column prop="responseStatus" :label="$t('monitor.resolveStatusLabel')" width="120" sortable="custom">
            <template #default="{ row }">
              <span :class="[
                'mn-badge',
                statusClass(row.responseStatus) === 'tag-success' ? 'mn-badge--success' :
                statusClass(row.responseStatus) === 'tag-warning' ? 'mn-badge--warning' :
                statusClass(row.responseStatus) === 'tag-danger'  ? 'mn-badge--danger'  : 'mn-badge--neutral'
              ]">{{ statusLabel(row.responseStatus) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="responseTime" :label="$t('monitor.responseMs')" width="110" align="right" sortable="custom" />
          <el-table-column :label="$t('common.operation')" width="100" fixed="right">
            <template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">{{ $t('common.detail') }}</el-button></template>
          </el-table-column>
        </el-table>
        <div class="mn-pagination">
          <el-pagination
            v-model:current-page="pagination.page"
            v-model:page-size="pagination.size"
            layout="total, sizes, prev, pager, next"
            :page-sizes="[10, 20, 50]"
            :total="sortedTableData.length"
            small
            background
          />
        </div>
      </template>
      <el-empty v-else :description="$t('monitor.noRealTimeData')" />
    </el-card>

    <!-- ── 图表区 ── -->
    <div class="mn-chart-grid">
      <el-card class="monitor-card chart-span-6" shadow="never">
        <template #header>
          <div class="mn-chart-header">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
            <span>{{ $t('monitor.topDomainStats') }}</span>
          </div>
        </template>
        <el-skeleton v-if="loading" animated :rows="7" />
        <el-empty v-else-if="!chartData.topDomain.length" :description="$t('monitor.noReportData')" />
        <BaseChart v-else :option="chartOptions.topDomain" :loading="false" :empty="false" height="320px" />
      </el-card>

      <el-card class="monitor-card chart-span-6" shadow="never">
        <template #header>
          <div class="mn-chart-header">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
            <span>{{ $t('monitor.topIpStats') }}</span>
          </div>
        </template>
        <el-skeleton v-if="loading" animated :rows="7" />
        <el-empty v-else-if="!chartData.topIp.length" :description="$t('monitor.noReportData')" />
        <BaseChart v-else :option="chartOptions.topIp" :loading="false" :empty="false" height="320px" />
      </el-card>

      <el-card class="monitor-card chart-span-6" shadow="never">
        <template #header>
          <div class="mn-chart-header">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
            <span>{{ $t('monitor.geoHeatmap') }}</span>
          </div>
        </template>
        <el-skeleton v-if="loading" animated :rows="7" />
        <el-empty v-else-if="!chartData.heatmap.values.length" :description="$t('monitor.noReportData')" />
        <BaseChart v-else :option="chartOptions.heatmap" :loading="false" :empty="false" height="320px" />
      </el-card>

      <el-card class="monitor-card chart-span-6" shadow="never">
        <template #header>
          <div class="mn-chart-header">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21.21 15.89A10 10 0 1 1 8 2.83"/><path d="M22 12A10 10 0 0 0 12 2v10z"/></svg>
            <span>{{ $t('monitor.statusDist') }}</span>
          </div>
        </template>
        <el-skeleton v-if="loading" animated :rows="7" />
        <el-empty v-else-if="!chartData.statusDistribution.length" :description="$t('monitor.noReportData')" />
        <BaseChart v-else :option="chartOptions.status" :loading="false" :empty="false" height="320px" />
      </el-card>
    </div>

    <!-- ── 详情 Dialog ── -->
    <el-dialog v-model="detailVisible" :title="$t('monitor.realTimeDetail')" width="520px" destroy-on-close @closed="closeDetail">
      <div v-if="detailRow" class="detail-grid">
        <div class="detail-item"><span>{{ $t('monitor.queryId') }}</span><strong>{{ detailRow.queryId }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.time') }}</span><strong>{{ formatDateTime(detailRow.time) }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.queryDomain') }}</span><strong>{{ detailRow.domain }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.recordType') }}</span><strong>{{ detailRow.recordType }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.sourceIp') }}</span><strong class="mn-mono">{{ detailRow.sourceIp }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.region') }}</span><strong>{{ regionLabel(detailRow.region) }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.resolveStatusLabel') }}</span><strong>{{ statusLabel(detailRow.responseStatus) }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.responseTime') }}</span><strong>{{ detailRow.responseTime }} ms</strong></div>
        <div class="detail-item detail-item-block">
          <span>{{ $t('monitor.requestContent') }}</span>
          <strong>{{ detailRow.requestPayload }}</strong>
          <el-button text type="primary" @click="copyText(detailRow.requestPayload || '', $t('monitor.requestCopied'))">{{ $t('common.copy') }}</el-button>
        </div>
        <div class="detail-item detail-item-block">
          <span>{{ $t('monitor.responseContent') }}</span>
          <strong>{{ detailRow.responsePayload }}</strong>
          <el-button text type="primary" @click="copyText(detailRow.responsePayload || '', $t('monitor.responseCopied'))">{{ $t('common.copy') }}</el-button>
        </div>
      </div>
      <template #footer>
        <el-button @click="closeDetail">{{ $t('common.close') }}</el-button>
        <el-button type="primary" @click="copyDetail">{{ $t('monitor.copyKeyInfo') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.realtime-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ═══════════════ Cards ═══════════════ */
.monitor-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.monitor-card :deep(.el-card__body) {
  padding: 0;
}

.monitor-card :deep(.el-card__header) {
  padding: 13px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

/* ═══════════════ Toolbar ═══════════════ */
.mn-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.mn-toolbar-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}

.mn-toolbar-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}

.mn-input-icon {
  width: 13px;
  height: 13px;
  color: var(--app-text-regular);
}

/* ═══════════════ Table ═══════════════ */
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.mn-table :deep(.el-table__body tr.current-row > td.el-table__cell),
.mn-table :deep(.el-table__body tr.el-table__row--selected > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.08) !important;
}

:deep(.monitor-row-danger > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.04);
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Badges ═══════════════ */
.mn-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 9px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.6;
  white-space: nowrap;
  border: 1px solid transparent;
}

.mn-badge--neutral {
  color: var(--app-text-secondary);
  background: rgba(78, 89, 105, 0.08);
  border-color: rgba(78, 89, 105, 0.18);
}

.mn-badge--success {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

.mn-badge--warning {
  color: var(--app-warning);
  background: rgba(255, 125, 0, 0.08);
  border-color: rgba(255, 125, 0, 0.22);
}

.mn-badge--danger {
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.22);
}

/* ═══════════════ Mono / time ═══════════════ */
.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}

.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}

/* ═══════════════ Charts grid ═══════════════ */
.mn-chart-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.chart-span-6 {
  /* no-op: grid already 2-col */
}

.mn-chart-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}

.mn-chart-icon {
  width: 15px;
  height: 15px;
  color: var(--app-accent);
  flex-shrink: 0;
}

/* ═══════════════ Detail dialog ═══════════════ */
.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 11px 13px;
  border-radius: 8px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
}

.detail-item span {
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--app-text-regular);
}

.detail-item strong {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  word-break: break-all;
}

.detail-item-block {
  grid-column: 1 / -1;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1100px) {
  .mn-chart-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .mn-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>
