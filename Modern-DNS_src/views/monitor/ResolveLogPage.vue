<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useResolveLog } from '../../composables/monitor/useResolveLog'

const route = useRoute()

const {
  filterForm,
  pagination,
  loading,
  querying,
  refreshing,
  exporting,
  detailVisible,
  currentDetail,
  lastUpdated,
  resolveRows,
  totalRows,
  fetchResolveLogs,
  handleSearch,
  handleReset,
  handleRefresh,
  handlePageChange,
  handleSortChange,
  handleExport,
  openDetail,
  copyDetailSummary,
  copyDetailText,
  formatDateTimeMs,
  statusClass,
  rowClassName,
} = useResolveLog()

const recordTypeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA']
const rcodeOptions = ['NOERROR', 'NXDOMAIN', 'SERVFAIL', 'REFUSED']

onMounted(async () => {
  await fetchResolveLogs()
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('monitor.resolveLogSubtitle') }}{{ $t('common.lastRefresh') }}{{ lastUpdated || $t('common.loading') }}</p>
      </div>
    </div>

    <el-card class="monitor-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="filterForm.keyword" clearable :placeholder="$t('monitor.searchDomainTxidIp')" style="width: 280px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterForm.recordType" clearable :placeholder="$t('monitor.recordType')" style="width: 120px">
            <el-option v-for="item in recordTypeOptions" :key="item" :label="item" :value="item" />
          </el-select>
          <el-select v-model="filterForm.rcode" clearable :placeholder="$t('monitor.returnCode')" style="width: 130px">
            <el-option v-for="item in rcodeOptions" :key="item" :label="item" :value="item" />
          </el-select>
          <el-select v-model="filterForm.timePreset" style="width: 120px">
            <el-option :label="$t('monitor.last1h')" value="1h" />
            <el-option :label="$t('monitor.last6h')" value="6h" />
            <el-option :label="$t('monitor.last24h')" value="24h" />
            <el-option :label="$t('monitor.last7d')" value="7d" />
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
          <el-button type="primary" :loading="querying" @click="handleSearch">{{ $t('monitor.query') }}</el-button>
          <el-button @click="handleReset">{{ $t('common.reset') }}</el-button>
          <el-button :loading="refreshing" :disabled="refreshing" @click="handleRefresh">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ $t('common.refresh') }}
          </el-button>
        </div>
        <div class="mn-toolbar-actions">
          <el-dropdown @command="handleExport">
            <el-button :loading="exporting" :disabled="exporting">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
              {{ $t('monitor.exportLog') }}
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="all-excel">{{ $t('monitor.allExcel') }}</el-dropdown-item>
                <el-dropdown-item command="all-csv">{{ $t('monitor.allCsv') }}</el-dropdown-item>
                <el-dropdown-item command="all-json">{{ $t('monitor.allJson') }}</el-dropdown-item>
                <el-dropdown-item divided command="filtered-excel">{{ $t('monitor.filteredExcel') }}</el-dropdown-item>
                <el-dropdown-item command="filtered-csv">{{ $t('monitor.filteredCsv') }}</el-dropdown-item>
                <el-dropdown-item command="filtered-json">{{ $t('monitor.filteredJson') }}</el-dropdown-item>
                <el-dropdown-item command="filtered-batch">{{ $t('monitor.filteredBatch') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <el-empty v-if="!totalRows && !loading" :description="$t('monitor.noResolveLog')" />
      <template v-else>
        <el-table
          v-loading="loading"
          :data="resolveRows"
          stripe
          highlight-current-row
          class="mn-table"
          :row-class-name="rowClassName"
          @sort-change="handleSortChange"
        >
          <el-table-column prop="logId" :label="$t('monitor.logId')" width="170" sortable="custom" />
          <el-table-column prop="time" :label="$t('monitor.time')" width="210" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ formatDateTimeMs(row.time) }}</span></template>
          </el-table-column>
          <el-table-column prop="domain" :label="$t('common.domain')" min-width="220" sortable="custom" show-overflow-tooltip />
          <el-table-column prop="recordType" :label="$t('monitor.recordType')" width="100" sortable="custom">
            <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.recordType }}</span></template>
          </el-table-column>
          <el-table-column prop="transactionId" label="Transaction ID" min-width="140" sortable="custom">
            <template #default="{ row }"><span class="mn-mono">{{ row.transactionId }}</span></template>
          </el-table-column>
          <el-table-column prop="rcode" :label="$t('monitor.returnCode')" width="120" sortable="custom">
            <template #default="{ row }">
              <span :class="[
                'mn-badge',
                statusClass(row.rcode) === 'tag-success' ? 'mn-badge--success' :
                statusClass(row.rcode) === 'tag-warning' ? 'mn-badge--warning' :
                statusClass(row.rcode) === 'tag-danger'  ? 'mn-badge--danger'  : 'mn-badge--neutral'
              ]">{{ row.rcode }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="sourceIp" :label="$t('monitor.sourceIp')" min-width="130" sortable="custom">
            <template #default="{ row }"><span class="mn-mono">{{ row.sourceIp }}</span></template>
          </el-table-column>
          <el-table-column prop="responseTime" :label="$t('monitor.responseMs')" width="110" align="right" sortable="custom" />
          <el-table-column :label="$t('common.operation')" width="100" fixed="right">
            <template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">{{ $t('common.detail') }}</el-button></template>
          </el-table-column>
        </el-table>
        <div class="mn-pagination">
          <el-pagination
            :current-page="pagination.page"
            :page-size="pagination.size"
            layout="total, sizes, prev, pager, next"
            :page-sizes="[10, 20, 50]"
            :total="pagination.total"
            small
            background
            @current-change="(page) => handlePageChange(page)"
            @size-change="(size) => handlePageChange(1, size)"
          />
        </div>
      </template>
    </el-card>

    <el-dialog v-model="detailVisible" :title="$t('monitor.resolveLogDetail')" width="500px" destroy-on-close>
      <div v-if="currentDetail" class="detail-grid">
        <div class="detail-item"><span>{{ $t('monitor.logId') }}</span><strong>{{ currentDetail.logId }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.time') }}</span><strong>{{ formatDateTimeMs(currentDetail.time) }}</strong></div>
        <div class="detail-item"><span>{{ $t('common.domain') }}</span><strong>{{ currentDetail.domain }}</strong></div>
        <div class="detail-item"><span>Transaction ID</span><strong class="mn-mono">{{ currentDetail.transactionId }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.returnCode') }}</span><strong>{{ currentDetail.rcode }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.responseTime') }}</span><strong>{{ currentDetail.responseTime }} ms</strong></div>
        <div class="detail-item detail-item-block"><span>{{ $t('monitor.requestHeaders') }}</span><strong>{{ currentDetail.requestHeaders }}</strong></div>
        <div class="detail-item detail-item-block"><span>{{ $t('monitor.responseHeaders') }}</span><strong>{{ currentDetail.responseHeaders }}</strong></div>
        <div class="detail-item detail-item-block">
          <span>{{ $t('monitor.requestContent') }}</span>
          <strong>{{ currentDetail.requestPayload }}</strong>
          <el-button text type="primary" @click="copyDetailText(currentDetail.requestPayload || '', $t('monitor.requestCopied'))">{{ $t('common.copy') }}</el-button>
        </div>
        <div class="detail-item detail-item-block">
          <span>{{ $t('monitor.responseContent') }}</span>
          <strong>{{ currentDetail.responsePayload }}</strong>
          <el-button text type="primary" @click="copyDetailText(currentDetail.responsePayload || '', $t('monitor.responseCopied'))">{{ $t('common.copy') }}</el-button>
        </div>
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">{{ $t('common.close') }}</el-button>
        <el-button type="primary" @click="copyDetailSummary">{{ $t('monitor.copyKeyInfo') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.page-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ═══════════════ Card ═══════════════ */
.monitor-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.monitor-card :deep(.el-card__body) {
  padding: 0;
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
  word-break: break-word;
}

.detail-item-block {
  grid-column: span 2;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1100px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }

  .detail-item-block {
    grid-column: span 1;
  }
}

@media (max-width: 768px) {
  .mn-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>