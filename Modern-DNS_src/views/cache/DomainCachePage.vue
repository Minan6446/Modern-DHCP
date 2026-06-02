<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute } from 'vue-router'
import { InfoFilled } from '@element-plus/icons-vue'
import { confirmRiskAction } from '../../utils/interaction'
import { useDomainCache } from '../../composables/setting/useDomainCache'
import type { CacheNode } from '../../types/modules'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const route = useRoute()

const {
  loading,
  submitting,
  refreshLoading,
  batchClearLoading,
  singleClearId,
  lastUpdated,
  typeOptions,
  filters,
  selectedRows,
  selectedNodeId,
  selectedTreeNode,
  sortedTree,
  linkedRows,
  pagedRows,
  pager,
  detailDialogVisible,
  detailRecord,
  refreshData,
  selectTreeNode,
  resetSelectedTreeNode,
  handleSortChange,
  handlePageChange,
  handlePageSizeChange,
  handleSelectionChange,
  openDetail,
  copyRecordValue,
  clearSingleCache,
  batchClearCache,
  formatDateTime,
  isTtlWarning,
  sourceTagType,
  typeTagType,
  persistTagType,
} = useDomainCache()

const termTips = {
  ttl: t('cache.ttlTip'),
}

const handleTreeNodeSelect = (data: CacheNode): void => {
  selectTreeNode(data)
}

const handleRefresh = async (): Promise<void> => {
  try {
    await refreshData()
  } catch (_error) {
    ElMessage.error(t('cache.refreshFailed'))
  }
}

const handleClearSingle = async (row: CacheNode): Promise<void> => {
  try {
    await confirmRiskAction({
      title: t('cache.clearConfirmSingle'),
      action: t('cache.clearAction'),
      target: row.cacheId,
      risk: t('cache.clearRisk'),
    })
    await clearSingleCache(row)
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('cache.clearFailed'))
    }
  }
}

const handleBatchClear = async (): Promise<void> => {
  if (!selectedRows.value.length) {
    ElMessage.warning(t('cache.selectRuleFirst'))
    return
  }
  try {
    await confirmRiskAction({
      title: t('cache.batchClearConfirm'),
      action: t('cache.batchClearAction'),
      risk: t('cache.batchClearRisk'),
    })
    await batchClearCache()
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('cache.batchClearFailed'))
    }
  }
}

const kpiTotal = computed(() => linkedRows.value.length)
const kpiExpiringSoon = computed(() => linkedRows.value.filter((r) => Number(r.ttlRemaining) > 0 && Number(r.ttlRemaining) < 30).length)
const kpiPersisted = computed(() => linkedRows.value.filter((r) => r.persisted === t('cache.persisted')).length)
const kpiLocal = computed(() => linkedRows.value.filter((r) => r.source === t('cache.localSource')).length)

const expiringFilterOn = ref(false)
const treeKeyword = ref('')

const ctxVisible = ref(false)
const ctxPos = ref({ x: 0, y: 0 })
const ctxRow = ref<CacheNode | null>(null)

const showContextMenu = (e: MouseEvent, row: CacheNode): void => {
  e.preventDefault()
  ctxRow.value = row
  ctxPos.value = { x: e.clientX, y: e.clientY }
  ctxVisible.value = true
}

const closeCtx = (): void => { ctxVisible.value = false }

const handlePersist = (_row: CacheNode): void => {
  ElMessage.success(t('cache.markedPersist'))
  closeCtx()
}

const ttlPct = (row: CacheNode): number =>
  Math.min(100, Math.round(Number(row.ttlRemaining) / (Number(row.ttlOriginal) || 1) * 100))

const TYPE_COLOR: Record<string, string> = {
  A: '#10b981', AAAA: '#3b82f6', CNAME: '#8b5cf6',
  MX: '#f59e0b', TXT: '#6b7280', NS: '#0ea5e9',
  SRV: '#ec4899', CAA: '#ef4444',
}
</script>

<template>
  <div class="page-shell cache-domain-shell" @click="closeCtx">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('cache.domainCache') }}</h1>
        <p class="page-subtitle">{{ $t('cache.domainCacheSubtitle') }}</p>
      </div>
      <span class="cache-last-updated">{{ $t('common.updatedAt') }}: {{ formatDateTime(lastUpdated) }}</span>
    </div>

    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-select v-model="filters.recordType" clearable :placeholder="$t('cache.recordTypePlaceholder')" style="width: 120px">
            <el-option v-for="type in typeOptions" :key="type" :label="type" :value="type" />
          </el-select>
          <el-select v-model="filters.persistStatus" clearable :placeholder="$t('cache.persistStatus')" style="width: 120px">
            <el-option :label="$t('cache.persistedOption')" value="已持久化" />
            <el-option :label="$t('cache.notPersistedOption')" value="未持久化" />
          </el-select>
          <el-date-picker
            v-model="filters.timeRange"
            type="datetimerange"
            value-format="YYYY-MM-DD HH:mm:ss"
            :range-separator="$t('common.to')"
            :start-placeholder="$t('cache.startTime')"
            :end-placeholder="$t('cache.endTime')"
          />
          <button
            type="button"
            :class="['quick-chip', { 'quick-chip--on': expiringFilterOn }]"
            @click="expiringFilterOn = !expiringFilterOn"
          >{{ $t('cache.expiringFilter') }}</button>
          <el-button :loading="refreshLoading" :disabled="refreshLoading || submitting" @click="handleRefresh">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ $t('cache.refresh') }}
          </el-button>
        </div>
        <div class="mn-toolbar-actions">
          <span v-if="selectedTreeNode" class="mn-node-tag">{{ $t('cache.currentNode') }}：{{ selectedTreeNode.domain }}</span>
          <el-button v-if="selectedNodeId" @click="resetSelectedTreeNode">{{ $t('cache.viewAll') }}</el-button>
          <el-button type="danger" plain :loading="batchClearLoading" :disabled="batchClearLoading || submitting || !selectedRows.length" @click="handleBatchClear">{{ $t('cache.batchClearCache') }}</el-button>
        </div>
      </div>

      <div class="mn-stats-strip" style="padding: 12px 16px 0;">
        <div class="mn-stat-tile mn-stat-tile--neutral">
          <div class="mn-stat-value">{{ kpiTotal }}</div>
          <div class="mn-stat-label">{{ $t('cache.totalRecords') }}</div>
        </div>
        <div class="mn-stat-tile mn-stat-tile--warning">
          <div class="mn-stat-value">{{ kpiExpiringSoon }}</div>
          <div class="mn-stat-label">{{ $t('cache.expiringSoon') }}</div>
        </div>
        <div class="mn-stat-tile mn-stat-tile--success">
          <div class="mn-stat-value">{{ kpiPersisted }}</div>
          <div class="mn-stat-label">{{ $t('cache.persisted') }}</div>
        </div>
        <div class="mn-stat-tile mn-stat-tile--accent">
          <div class="mn-stat-value">{{ kpiLocal }}</div>
          <div class="mn-stat-label">{{ $t('cache.localSource') }}</div>
        </div>
      </div>

      <div class="cache-layout">
        <div class="cache-tree-panel">
      <div class="mn-sub-header">
        <svg class="mn-sub-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
        <span>{{ $t('cache.cacheTree') }}</span>
        <span class="mn-sub-meta">{{ sortedTree.length }} {{ $t('common.rootNodes') }}</span>
      </div>
      <div class="tree-search-wrap">
        <el-input v-model="treeKeyword" clearable :placeholder="$t('common.searchDomain')" size="small">
          <template #prefix><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
        </el-input>
      </div>
      <el-scrollbar max-height="520px">
        <el-tree
          :data="sortedTree"
          node-key="id"
          highlight-current
          :current-node-key="selectedNodeId || undefined"
          :props="{ label: 'domain', children: 'children' }"
          :filter-node-method="(val: string, data: { domain: string }) => !val || data.domain.includes(val)"
          :default-expand-all="Boolean(treeKeyword)"
          :empty-text="$t('cache.noCacheTreeData')"
          @node-click="handleTreeNodeSelect"
        >
          <template #default="{ data }">
            <div class="cache-tree-node">
              <span class="cache-tree-label">{{ data.domain }}</span>
              <span class="cache-tree-meta">{{ data.recordType }}</span>
            </div>
          </template>
        </el-tree>
      </el-scrollbar>
    </div>

    <div class="cache-list-panel">
      <div class="mn-sub-header">
        <svg class="mn-sub-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="3" y1="15" x2="21" y2="15"/><line x1="9" y1="9" x2="9" y2="21"/></svg>
        <span>{{ $t('cache.cacheList') }}</span>
        <span class="mn-sub-meta">{{ linkedRows.length }} {{ $t('common.records') }}</span>
      </div>
      <el-table
        class="mn-table"
        :data="expiringFilterOn ? pagedRows.filter(r => Number(r.ttlRemaining) > 0 && Number(r.ttlRemaining) < 30) : pagedRows"
        row-key="id"
        stripe
        height="520"
        table-layout="auto"
        v-loading="loading"
        @selection-change="handleSelectionChange"
        @sort-change="handleSortChange"
        @row-contextmenu="showContextMenu"
      >
        <el-table-column type="selection" width="50" />
        <el-table-column prop="cacheId" :label="$t('cache.cacheId')" min-width="140" sortable="custom">
          <template #default="{ row }"><span class="mn-mono">{{ row.cacheId }}</span></template>
        </el-table-column>
        <el-table-column prop="domain" :label="$t('cache.domain')" min-width="200" sortable="custom" show-overflow-tooltip>
          <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
        </el-table-column>
        <el-table-column prop="recordType" :label="$t('common.type')" width="80" sortable="custom" align="center">
          <template #default="{ row }">
            <span class="type-badge" :style="{ background: TYPE_COLOR[row.recordType] ?? '#6b7280' }">{{ row.recordType }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="recordValue" :label="$t('cache.recordValue')" min-width="200" show-overflow-tooltip />
        <el-table-column width="200" sortable="custom" prop="ttlRemaining">
          <template #header>
            <div class="inline-actions">
              <span>{{ $t('cache.cacheTtl') }}</span>
              <el-tooltip :content="termTips.ttl" placement="top">
                <el-icon class="term-help-icon"><InfoFilled /></el-icon>
              </el-tooltip>
            </div>
          </template>
          <template #default="{ row }">
            <div class="cache-ttl-cell">
              <div class="ttl-nums">
                <span class="ttl-line" :class="{ 'ttl-line--warn': Number(row.ttlRemaining) < 30 }">{{ row.ttlRemaining }}s</span>
                <span class="ttl-line ttl-line--secondary">/ {{ row.ttlOriginal }}s</span>
              </div>
              <div class="ttl-mini-bar-wrap">
                <div
                  class="ttl-mini-bar"
                  :class="{ 'ttl-mini-bar--warn': Number(row.ttlRemaining) < 30 }"
                  :style="{ width: ttlPct(row) + '%' }"
                ></div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="source" :label="$t('cache.source')" width="100" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', sourceTagType(row.source) === 'success' ? 'mn-badge--success' : sourceTagType(row.source) === 'warning' ? 'mn-badge--warning' : 'mn-badge--neutral']">{{ row.source }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="persisted" :label="$t('common.persist')" width="100" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', row.persisted === $t('cache.persisted') ? 'mn-badge--success' : 'mn-badge--neutral']">{{ row.persisted }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="cacheTime" :label="$t('cache.cachedTime')" min-width="160" sortable="custom">
          <template #default="{ row }"><span class="mn-time">{{ row.cacheTime }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="120" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openDetail(row)">{{ $t('cache.detail') }}</el-button>
              <el-button link type="danger" :loading="singleClearId === row.id" :disabled="Boolean(singleClearId) || submitting" @click="handleClearSingle(row)">{{ $t('cache.clear') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('cache.noCacheRecords')" /></template>
      </el-table>
      <div v-if="linkedRows.length" class="mn-pagination">
        <el-pagination
          :current-page="pager.page"
          :page-size="pager.size"
          size="small"
          background
          layout="total, sizes, prev, pager, next"
          :page-sizes="[5, 10, 20]"
          :total="linkedRows.length"
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
        />
      </div>
    </div>
  </div>
</el-card>

    <el-dialog v-model="detailDialogVisible" width="520px" class="cache-detail-dialog" append-to-body :show-close="true">
      <template #header>
        <div class="cd-header">
          <div class="cd-header-main">
            <span class="cd-domain">{{ detailRecord?.domain ?? '--' }}</span>
            <span v-if="detailRecord" :class="['cd-type-badge', `cd-type-badge--${detailRecord.recordType.toLowerCase()}`]">{{ detailRecord.recordType }}</span>
          </div>
          <div class="cd-header-sub mn-mono">{{ detailRecord?.cacheId }}</div>
        </div>
      </template>
      <div v-if="detailRecord" class="cd-body" aria-live="polite">
        <div class="cd-section-title">{{ $t('cache.recordInfo') }}</div>
        <div class="cd-grid">
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.recordValue') }}</span>
            <div class="cd-val cd-val--with-copy">
              <span class="cd-record mn-mono">{{ detailRecord.recordValue }}</span>
              <button type="button" class="cd-copy-btn" @click="copyRecordValue">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="12" height="12"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                {{ $t('common.copy') }}
              </button>
            </div>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.source') }}</span>
            <span :class="['mn-badge', detailRecord.source === $t('cache.localSource') ? 'mn-badge--success' : detailRecord.source === $t('cache.recursiveSource') ? 'mn-badge--warning' : 'mn-badge--neutral']">{{ detailRecord.source }}</span>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.persistStatus') }}</span>
            <span :class="['mn-badge', detailRecord.persisted === $t('cache.persisted') ? 'mn-badge--success' : 'mn-badge--neutral']">{{ detailRecord.persisted }}</span>
          </div>
        </div>
        <div class="cd-section-title" style="margin-top:16px">{{ $t('cache.ttlInfo') }}</div>
        <div class="cd-ttl-block">
          <div class="cd-ttl-nums">
            <div class="cd-ttl-item">
              <span class="cd-ttl-num" :class="{ 'cd-ttl-warn': isTtlWarning(detailRecord.ttlRemaining) }">{{ detailRecord.ttlRemaining }}<span class="cd-ttl-unit">{{ $t('cache.secondsUnit') }}</span></span>
              <span class="cd-ttl-label">{{ $t('cache.ttlRemaining') }}</span>
            </div>
            <div class="cd-ttl-divider"></div>
            <div class="cd-ttl-item">
              <span class="cd-ttl-num cd-ttl-num--muted">{{ detailRecord.ttlOriginal }}<span class="cd-ttl-unit">{{ $t('cache.secondsUnit') }}</span></span>
              <span class="cd-ttl-label">{{ $t('cache.ttlOriginal') }}</span>
            </div>
          </div>
          <div class="cd-ttl-bar-wrap">
            <div
              class="cd-ttl-bar"
              :class="{ 'cd-ttl-bar--warn': isTtlWarning(detailRecord.ttlRemaining) }"
              :style="{ width: `${Math.min(100, Math.round(detailRecord.ttlRemaining / (detailRecord.ttlOriginal || 1) * 100))}%` }"
            ></div>
          </div>
          <div class="cd-ttl-bar-label">{{ $t('cache.remainingPct', { n: Math.min(100, Math.round(detailRecord.ttlRemaining / (detailRecord.ttlOriginal || 1) * 100)) }) }}</div>
        </div>
        <div class="cd-section-title" style="margin-top:16px">{{ $t('cache.timeInfo') }}</div>
        <div class="cd-grid">
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.cachedTime') }}</span>
            <span class="mn-mono cd-val">{{ formatDateTime(detailRecord.cacheTime) }}</span>
          </div>
          <div class="cd-row">
            <span class="cd-label">{{ $t('cache.updatedTime') }}</span>
            <span class="mn-mono cd-val">{{ formatDateTime(detailRecord.updatedAt) }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cd-footer">
          <el-button type="danger" plain size="small" :loading="singleClearId === detailRecord?.id" :disabled="Boolean(singleClearId)" @click="detailRecord && handleClearSingle(detailRecord)">{{ $t('cache.clear') }}</el-button>
          <el-button size="small" @click="detailDialogVisible = false">{{ $t('cache.close') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Right-click context menu -->
    <teleport to="body">
      <transition name="ctx-fade">
        <div
          v-if="ctxVisible"
          class="ctx-menu"
          :style="{ top: ctxPos.y + 'px', left: ctxPos.x + 'px' }"
          @click.stop
        >
          <button class="ctx-item" @click="ctxRow && openDetail(ctxRow); closeCtx()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            {{ $t('cache.viewDetail') }}
          </button>
          <button class="ctx-item" @click="ctxRow && handlePersist(ctxRow)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>
            {{ $t('cache.markPersist') }}
          </button>
          <div class="ctx-divider"></div>
          <button class="ctx-item ctx-item--danger" @click="ctxRow && handleClearSingle(ctxRow); closeCtx()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>
            {{ $t('cache.clear') }}
          </button>
        </div>
      </transition>
    </teleport>
  </div>
</template>

<style scoped>
/* ═══════════════ Stats strip ═══════════════ */
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }

/* ═══════════════ Page ═══════════════ */
.cache-domain-shell { gap: 16px; }

.cache-last-updated {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Card ═══════════════ */
.mn-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.mn-card :deep(.el-card__body) { padding: 0; }

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
  flex-wrap: wrap;
}

.mn-input-icon {
  width: 13px;
  height: 13px;
  color: var(--app-text-regular);
}

.mn-node-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 20px;
  font-size: 12px;
  background: rgba(22, 93, 255, 0.08);
  color: var(--app-accent);
  border: 1px solid rgba(22, 93, 255, 0.2);
}

/* ═══════════════ Layout panels ═══════════════ */
.cache-layout {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 0;
  padding: 16px;
  gap: 16px;
}

.cache-tree-panel,
.cache-list-panel {
  border: 1px solid var(--app-border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--app-bg);
}

/* ═══════════════ Sub-panel headers ═══════════════ */
.mn-sub-header {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 10px 12px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
  font-size: 12px;
  font-weight: 600;
  color: var(--app-title);
}

.mn-sub-icon {
  width: 13px;
  height: 13px;
  color: var(--app-accent);
  flex-shrink: 0;
}

.mn-sub-meta {
  margin-left: auto;
  font-weight: 400;
  color: var(--app-text-regular);
  font-size: 12px;
}

/* ═══════════════ Tree ═══════════════ */
.cache-tree-panel :deep(.el-scrollbar) {
  padding: 8px 4px;
}

.cache-tree-node {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
}

.cache-tree-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.cache-tree-meta {
  color: var(--app-text-regular);
  font-size: 11px;
  flex-shrink: 0;
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

.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.inline-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.term-help-icon {
  color: var(--app-text-regular);
  cursor: help;
  font-size: 13px;
}

.cache-ttl-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ttl-line {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.ttl-line--warn {
  color: var(--app-warning);
  font-weight: 600;
}

.ttl-line--secondary {
  color: var(--app-text-regular);
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 10px 12px;
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
:deep(.cache-detail-dialog .el-dialog) {
  border-radius: 12px;
  overflow: hidden;
}

:deep(.cache-detail-dialog .el-dialog__header) {
  margin-right: 0;
  padding: 0;
  border-bottom: 1px solid var(--app-border);
}

:deep(.cache-detail-dialog .el-dialog__body) { padding: 0; }

:deep(.cache-detail-dialog .el-dialog__footer) {
  padding: 10px 20px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

.cd-header {
  padding: 16px 20px 14px;
  background: linear-gradient(135deg, rgba(22,93,255,0.04) 0%, transparent 60%);
}

.cd-header-main {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.cd-domain {
  font-size: 16px;
  font-weight: 700;
  color: var(--app-text-primary, #1d2129);
  word-break: break-all;
}

.cd-type-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.04em;
  flex-shrink: 0;
}
.cd-type-badge--a    { background: #10b981; }
.cd-type-badge--aaaa { background: #3b82f6; }
.cd-type-badge--cname{ background: #8b5cf6; }
.cd-type-badge--mx   { background: #f59e0b; }
.cd-type-badge--txt  { background: #6b7280; }
.cd-type-badge--ns   { background: #0ea5e9; }
.cd-type-badge--srv  { background: #ec4899; }
.cd-type-badge--caa  { background: #ef4444; }

.cd-header-sub {
  margin-top: 4px;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.cd-body { padding: 16px 20px 20px; }

.cd-section-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border);
}

.cd-grid { display: flex; flex-direction: column; gap: 0; }

.cd-row {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  align-items: center;
  min-height: 36px;
  gap: 12px;
  border-bottom: 1px solid rgba(0,0,0,0.04);
}

.cd-row:last-child { border-bottom: none; }

.cd-label {
  font-size: 12px;
  color: var(--app-text-secondary);
  flex-shrink: 0;
}

.cd-val {
  font-size: 13px;
  color: var(--app-text-primary, #1d2129);
  word-break: break-all;
}

.cd-val--with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cd-record {
  flex: 1;
  min-width: 0;
  word-break: break-all;
  font-size: 12px;
}

.cd-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid rgba(22,93,255,0.25);
  background: rgba(22,93,255,0.05);
  color: var(--app-accent);
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 11px;
  white-space: nowrap;
  flex-shrink: 0;
  transition: background 0.15s;
}

.cd-copy-btn:hover { background: rgba(22,93,255,0.12); }

/* TTL block */
.cd-ttl-block {
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 12px 16px;
}

.cd-ttl-nums {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 10px;
}

.cd-ttl-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.cd-ttl-num {
  font-size: 22px;
  font-weight: 700;
  font-family: var(--app-font-mono, monospace);
  color: var(--app-accent);
  line-height: 1;
}

.cd-ttl-unit {
  font-size: 11px;
  font-weight: 400;
  margin-left: 2px;
  color: var(--app-text-secondary);
}

.cd-ttl-num--muted { color: var(--app-text-secondary); }

.cd-ttl-warn { color: var(--app-danger) !important; }

.cd-ttl-label {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.cd-ttl-divider {
  width: 1px;
  height: 32px;
  background: var(--app-border);
  flex-shrink: 0;
}

.cd-ttl-bar-wrap {
  height: 6px;
  border-radius: 3px;
  background: var(--app-border);
  overflow: hidden;
}

.cd-ttl-bar {
  height: 100%;
  border-radius: 3px;
  background: var(--app-accent);
  transition: width 0.4s ease;
}

.cd-ttl-bar--warn { background: var(--app-danger); }

.cd-ttl-bar-label {
  margin-top: 5px;
  font-size: 11px;
  color: var(--app-text-secondary);
  text-align: right;
}

/* Footer */
.cd-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

/* ═══════════════ KPI strip ═══════════════ */
.kpi-strip {
  display: flex;
  align-items: center;
  gap: 0;
  padding: 10px 20px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
}

.kpi-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  gap: 2px;
}

.kpi-num {
  font-size: 22px;
  font-weight: 700;
  font-family: var(--app-font-mono, monospace);
  color: var(--app-text-primary, #1d2129);
  line-height: 1;
}

.kpi-num--ok   { color: var(--app-success); }

.kpi-label {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.kpi-divider {
  width: 1px;
  height: 32px;
  background: var(--app-border);
  flex-shrink: 0;
}

/* ═══════════════ Quick filter chip ═══════════════ */
.quick-chip {
  display: inline-flex;
  align-items: center;
  padding: 3px 10px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid var(--app-border);
  background: var(--app-bg);
  color: var(--app-text-secondary);
  transition: all 0.15s;
  white-space: nowrap;
}

.quick-chip--on {
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.35);
  color: var(--app-danger);
}

/* ═══════════════ Tree search ═══════════════ */
.tree-search-wrap {
  padding: 8px 10px;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Type badge ═══════════════ */
.type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.04em;
}

/* ═══════════════ TTL mini bar ═══════════════ */
.ttl-nums {
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.ttl-mini-bar-wrap {
  margin-top: 3px;
  height: 3px;
  border-radius: 2px;
  background: var(--app-border);
  overflow: hidden;
}

.ttl-mini-bar {
  height: 100%;
  border-radius: 2px;
  background: var(--app-accent);
  transition: width 0.3s;
}

.ttl-mini-bar--warn { background: var(--app-danger); }

/* ═══════════════ Context menu ═══════════════ */
.ctx-menu {
  position: fixed;
  z-index: 9999;
  min-width: 148px;
  background: var(--app-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.12);
  padding: 4px 0;
  overflow: hidden;
}

.ctx-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 7px 14px;
  font-size: 13px;
  color: var(--app-text-primary, #1d2129);
  background: none;
  border: none;
  cursor: pointer;
  text-align: left;
  transition: background 0.12s;
}

.ctx-item:hover { background: var(--app-bg-secondary); }

.ctx-item--danger { color: var(--app-danger); }
.ctx-item--danger:hover { background: rgba(245,63,63,0.06); }

.ctx-divider { height: 1px; background: var(--app-border); margin: 3px 0; }

.ctx-fade-enter-active,
.ctx-fade-leave-active { transition: opacity 0.1s, transform 0.1s; }
.ctx-fade-enter-from,
.ctx-fade-leave-to { opacity: 0; transform: scale(0.95); }

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1200px) {
  .cache-layout { grid-template-columns: 1fr; }
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { justify-content: flex-start; }
  .kpi-strip { flex-wrap: wrap; gap: 12px; }
}
</style>