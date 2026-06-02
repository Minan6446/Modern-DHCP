<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useOperationLog } from '../../composables/setting/useOperationLog'
import type { SettingLogItem } from '../../types/modules'

const route = useRoute()
const { t } = useI18n()

const {
  filterForm,
  logList,
  pagination,
  loading,
  exporting,
  exportFormat,
  detailVisible,
  currentDetail,
  fetchLogs,
  handleSearch,
  handleReset,
  handlePageChange,
  handleSortChange,
  handleExport,
  handleViewDetail,
} = useOperationLog()

const rawLogs = computed<SettingLogItem[]>(() => logList.value)

const actionTypeKey = (value: string): 'create' | 'edit' | 'delete' | 'login' | 'config' => {
  const key = value.toLowerCase()
  if (value === '新增' || key === 'create') return 'create'
  if (value === '编辑' || key === 'edit' || key === 'update') return 'edit'
  if (value === '删除' || key === 'delete' || key === 'remove') return 'delete'
  if (value === '登录' || key === 'login' || key === 'signin') return 'login'
  return 'config'
}

const actionTypeLabel = (value: string) => t(`audit.actionMap.${actionTypeKey(value)}`)

const stats = computed(() => {
  const all = rawLogs.value
  const operators = new Set(all.map((r) => r.operator)).size
  const types = new Set(all.map((r) => r.actionType)).size
  const latest = all.reduce<string>((acc, r) => (r.time > acc ? r.time : acc), '')
  return {
    total: pagination.total,
    operators,
    types,
    latest: latest ? formatDateTime(latest) : t('common.none'),
  }
})

const formatDateTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (num: number) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const formatLogDetail = computed(() => {
  if (!currentDetail.value?.detail) {
    return '{}'
  }
  try {
    return JSON.stringify(JSON.parse(currentDetail.value.detail), null, 2)
  } catch (_error) {
    return currentDetail.value.detail
  }
})

const copyLogDetail = async (): Promise<void> => {
  const text = formatLogDetail.value
  try {
    await navigator.clipboard.writeText(text)
  } catch (_error) {
    const textarea = document.createElement('textarea')
    textarea.value = text
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
  ElMessage.success(t('audit.requestCopied'))
}

onMounted(async () => {
  await fetchLogs()
})
</script>

<template>
  <div class="ol-page page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('audit.subtitle') }}</p>
      </div>
    </div>

    <!-- ═══ Stats strip ═══ -->
    <div class="ol-stats">
      <div class="ol-stat">
        <div class="ol-stat-icon ol-stat-icon-total">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6zm-1 7V3.5L18.5 9H13zm-2 8H7v-2h4v2zm4-4H7v-2h8v2zm0-4H7V7h8v2z"/></svg>
        </div>
        <div class="ol-stat-body">
          <div class="ol-stat-label">{{ $t('audit.kpi.totalLogs') }}</div>
          <div class="ol-stat-value ol-stat-value-num">{{ stats.total }}</div>
        </div>
      </div>
      <div class="ol-stat">
        <div class="ol-stat-icon ol-stat-icon-operators">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
        </div>
        <div class="ol-stat-body">
          <div class="ol-stat-label">{{ $t('audit.kpi.operators') }}</div>
          <div class="ol-stat-value ol-stat-value-num">{{ stats.operators }}</div>
        </div>
      </div>
      <div class="ol-stat">
        <div class="ol-stat-icon ol-stat-icon-types">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-7 3c1.93 0 3.5 1.57 3.5 3.5S13.93 13 12 13s-3.5-1.57-3.5-3.5S10.07 6 12 6zm7 13H5v-.23c0-.62.28-1.2.76-1.58C7.47 15.82 9.64 15 12 15s4.53.82 6.24 2.19c.48.38.76.97.76 1.58V19z"/></svg>
        </div>
        <div class="ol-stat-body">
          <div class="ol-stat-label">{{ $t('audit.kpi.actionTypes') }}</div>
          <div class="ol-stat-value ol-stat-value-num">{{ stats.types }}</div>
        </div>
      </div>
      <div class="ol-stat">
        <div class="ol-stat-icon ol-stat-icon-latest">
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20"><path d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zm4.24 16L11 13V7h1.5v5.25l4.5 2.67L16.23 18z"/></svg>
        </div>
        <div class="ol-stat-body">
          <div class="ol-stat-label">{{ $t('audit.kpi.latestTime') }}</div>
          <div class="ol-stat-value ol-stat-value-time">{{ stats.latest }}</div>
        </div>
      </div>
    </div>

    <!-- ═══ Main table card ═══ -->
    <el-card class="ol-card" shadow="never">
      <!-- Toolbar -->
      <div class="ol-toolbar">
        <div class="ol-toolbar-filters">
          <el-input
            v-model="filterForm.keyword"
            clearable
            :placeholder="$t('audit.searchPlaceholderSimple')"
            class="ol-search-input"
          >
            <template #prefix>
              <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14" style="color:var(--app-text-placeholder)"><path d="M15.5 14h-.79l-.28-.27A6.471 6.471 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg>
            </template>
          </el-input>
          <el-date-picker
            v-model="filterForm.range"
            type="datetimerange"
            :range-separator="$t('common.to')"
            :start-placeholder="$t('common.startTime')"
            :end-placeholder="$t('common.endTime')"
            value-format="YYYY-MM-DD HH:mm:ss"
            class="ol-date-picker"
          />
          <el-select v-model="filterForm.type" clearable :placeholder="$t('audit.action')" class="ol-type-select">
            <el-option :label="$t('audit.actionMap.create')" value="新增" />
            <el-option :label="$t('audit.actionMap.edit')" value="编辑" />
            <el-option :label="$t('audit.actionMap.delete')" value="删除" />
            <el-option :label="$t('audit.actionMap.login')" value="登录" />
            <el-option :label="$t('audit.actionMap.config')" value="配置" />
          </el-select>
          <el-button type="primary" plain @click="handleSearch">
            <template #icon><svg viewBox="0 0 24 24" fill="currentColor" width="13" height="13"><path d="M15.5 14h-.79l-.28-.27A6.471 6.471 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg></template>
            {{ $t('common.search') }}
          </el-button>
          <el-button @click="handleReset">{{ $t('common.reset') }}</el-button>
        </div>
        <div class="ol-toolbar-actions">
          <el-select v-model="exportFormat" class="ol-export-fmt">
            <el-option label="CSV" value="CSV" />
            <el-option label="Excel" value="Excel" />
          </el-select>
          <el-button :loading="loading" :disabled="loading" @click="fetchLogs">
            <template #icon><svg viewBox="0 0 24 24" fill="currentColor" width="13" height="13"><path d="M17.65 6.35A7.958 7.958 0 0 0 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08A5.99 5.99 0 0 1 12 18c-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/></svg></template>
            {{ $t('common.refresh') }}
          </el-button>
          <el-button type="primary" :loading="exporting" :disabled="exporting" @click="handleExport">
            <template #icon><svg viewBox="0 0 24 24" fill="currentColor" width="13" height="13"><path d="M19 9h-4V3H9v6H5l7 7 7-7zm-8 2V5h2v6h1.17L12 13.17 9.83 11H11zm-6 7h14v2H5v-2z"/></svg></template>
            {{ $t('common.export') }}
          </el-button>
        </div>
      </div>

      <!-- Table -->
      <el-empty v-if="!logList.length && !loading" :description="$t('audit.empty')" class="ol-empty" />
      <el-table
        v-else
        v-loading="loading"
        :data="logList"
        stripe
        class="ol-table"
        @sort-change="handleSortChange"
      >
        <el-table-column prop="logId" :label="$t('audit.logId')" width="180" align="left" sortable="custom" />
        <el-table-column prop="operator" :label="$t('audit.operator')" width="120" align="left" sortable="custom">
          <template #default="{ row }">
            <span class="ol-operator">
              <span class="ol-operator-avatar">{{ row.operator?.charAt(0)?.toUpperCase() }}</span>
              {{ row.operator }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="actionType" :label="$t('audit.action')" width="110" align="left" sortable="custom">
          <template #default="{ row }">
            <span class="ol-type-badge" :class="`ol-type-${actionTypeKey(row.actionType)}`">{{ actionTypeLabel(row.actionType) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="module" :label="$t('audit.module')" width="140" align="left" sortable="custom" />
        <el-table-column prop="content" :label="$t('audit.detail')" min-width="220" align="left" show-overflow-tooltip />
        <el-table-column prop="ip" :label="$t('audit.ipAddress')" width="150" align="left" sortable="custom">
          <template #default="{ row }">
            <span class="ol-ip">{{ row.ip }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="time" :label="$t('audit.time')" width="180" align="left" sortable="custom">
          <template #default="{ row }">
            <span class="ol-time">{{ formatDateTime(row.time) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="80" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link type="primary" class="ol-detail-btn" @click="handleViewDetail(row)">{{ $t('common.detail') }}</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="ol-pagination">
        <el-pagination
          :current-page="pagination.page"
          :page-size="pagination.size"
          layout="total, sizes, prev, pager, next"
          small
          :page-sizes="[10, 20, 50]"
          :total="pagination.total"
          @current-change="(page) => handlePageChange(page)"
          @size-change="(size) => handlePageChange(1, size)"
        />
      </div>
    </el-card>

    <!-- ═══ Detail dialog ═══ -->
    <el-dialog
      v-model="detailVisible"
      :title="$t('audit.operationDetailTitle')"
      width="520px"
      destroy-on-close
      :close-on-press-escape="true"
      :close-on-click-modal="true"
      class="ol-dialog"
    >
      <div v-if="currentDetail" class="ol-detail">
        <div class="ol-detail-header">
          <span class="ol-type-badge" :class="`ol-type-${actionTypeKey(currentDetail.actionType)}`">{{ actionTypeLabel(currentDetail.actionType) }}</span>
          <span class="ol-detail-module">{{ currentDetail.module }}</span>
          <span class="ol-detail-time">{{ formatDateTime(currentDetail.time) }}</span>
        </div>
        <el-descriptions :column="2" border size="small" class="ol-detail-desc">
          <el-descriptions-item :label="$t('audit.logId')" :span="2">{{ currentDetail.logId }}</el-descriptions-item>
          <el-descriptions-item :label="$t('audit.operator')">{{ currentDetail.operator }}</el-descriptions-item>
          <el-descriptions-item :label="$t('audit.ipAddress')">{{ currentDetail.ip }}</el-descriptions-item>
          <el-descriptions-item :label="$t('audit.module')">{{ currentDetail.module }}</el-descriptions-item>
          <el-descriptions-item :label="$t('audit.action')">{{ actionTypeLabel(currentDetail.actionType) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('audit.detail')" :span="2">{{ currentDetail.content }}</el-descriptions-item>
        </el-descriptions>
        <div class="ol-detail-params">
          <div class="ol-detail-params-header">
            <span class="ol-detail-params-title">{{ $t('audit.requestParams') }}</span>
            <el-button link type="primary" size="small" @click="copyLogDetail">
              <template #icon><svg viewBox="0 0 24 24" fill="currentColor" width="12" height="12"><path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/></svg></template>
              {{ $t('common.copy') }}
            </el-button>
          </div>
          <pre class="ol-detail-pre">{{ formatLogDetail }}</pre>
        </div>
      </div>
      <template #footer>
        <div class="ol-dialog-footer">
          <el-button @click="detailVisible = false">{{ $t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.ol-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ol-page :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Stats strip ═══════════════ */
.ol-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.ol-stat {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 20px;
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  transition: box-shadow 0.2s, transform 0.2s;
}

.ol-stat:hover {
  box-shadow: 0 6px 18px rgba(22, 93, 255, 0.12);
  transform: translateY(-1px);
}

.ol-stat-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}

.ol-stat-icon-total     { background: var(--app-accent-soft); color: var(--app-accent); }
.ol-stat-icon-operators { background: rgba(114, 46, 209, 0.1); color: #722ED1; }
.ol-stat-icon-types     { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }
.ol-stat-icon-latest    { background: rgba(255, 125, 0, 0.1); color: var(--app-warning); }

.ol-stat-body {
  flex: 1;
  min-width: 0;
}

.ol-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-bottom: 4px;
}

.ol-stat-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
}

.ol-stat-value-num {
  font-size: 24px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.ol-stat-value-time {
  font-size: 13px;
  font-weight: 500;
}

/* ═══════════════ Card ═══════════════ */
.ol-card {
  overflow: hidden;
  border: 1px solid var(--app-border);
  border-radius: 10px;
}

/* ═══════════════ Toolbar ═══════════════ */
.ol-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 14px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.ol-toolbar-filters {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  flex: 1;
}

.ol-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.ol-search-input {
  width: 240px;
}

.ol-date-picker {
  width: 320px;
}

.ol-type-select {
  width: 130px;
}

.ol-export-fmt {
  width: 90px;
}

/* ═══════════════ Table ═══════════════ */
.ol-table {
  width: 100%;
}

.ol-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  border-bottom: 1px solid var(--app-border);
}

.ol-table :deep(.el-table__row:hover > td) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.ol-empty {
  padding: 48px 0;
}

/* ═══════════════ Operator cell ═══════════════ */
.ol-operator {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
}

.ol-operator-avatar {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--app-accent-soft);
  color: var(--app-accent);
  font-size: 11px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

/* ═══════════════ Action-type badges ═══════════════ */
.ol-type-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid transparent;
  white-space: nowrap;
}

.ol-type-create { background: rgba(0, 180, 42, 0.08); color: var(--app-success); border-color: rgba(0, 180, 42, 0.22); }
.ol-type-edit { background: rgba(22, 93, 255, 0.08); color: var(--app-accent); border-color: rgba(22, 93, 255, 0.22); }
.ol-type-delete { background: rgba(245, 63, 63, 0.08); color: var(--app-danger); border-color: rgba(245, 63, 63, 0.22); }
.ol-type-login { background: rgba(114, 46, 209, 0.08); color: #722ED1; border-color: rgba(114, 46, 209, 0.2); }
.ol-type-config { background: rgba(255, 125, 0, 0.08); color: var(--app-warning); border-color: rgba(255, 125, 0, 0.22); }

/* ═══════════════ IP / Time cells ═══════════════ */
.ol-ip {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
  color: var(--app-text-secondary);
}

.ol-time {
  font-size: 12px;
  color: var(--app-text-regular);
  font-variant-numeric: tabular-nums;
}

.ol-detail-btn {
  font-size: 12px;
}

/* ═══════════════ Pagination ═══════════════ */
.ol-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Detail dialog ═══════════════ */
.ol-detail {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ol-detail-header {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.ol-detail-module {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-title);
}

.ol-detail-time {
  font-size: 12px;
  color: var(--app-text-regular);
  margin-left: auto;
}

.ol-detail-desc {
  font-size: 13px;
}

.ol-detail-params {
  border: 1px solid var(--app-border);
  border-radius: 8px;
  overflow: hidden;
}

.ol-detail-params-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.ol-detail-params-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.ol-detail-pre {
  margin: 0;
  padding: 12px;
  font-size: 12px;
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  background: #fafbfc;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--app-title);
  min-height: 80px;
}

.ol-dialog-footer {
  display: flex;
  justify-content: flex-end;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .ol-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .ol-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .ol-toolbar-actions {
    align-self: flex-end;
  }

  .ol-date-picker {
    width: 100%;
  }

  .ol-search-input {
    width: 100%;
  }
}

@media (max-width: 640px) {
  .ol-stats {
    grid-template-columns: 1fr;
  }
}
</style>
