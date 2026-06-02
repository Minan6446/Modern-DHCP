<script setup lang="ts">
import { ref, computed, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAlertStore, type AlertNotice } from '../../stores/alert'

const alertStore = useAlertStore()
const { t } = useI18n()

type Level = 'critical' | 'warning' | 'info' | string

const filters = reactive({ level: '', status: '', keyword: '' })
const detailVisible = ref(false)
const detailItem = ref<AlertNotice | null>(null)

const statusOf = (n: AlertNotice) => n.status === '已处理' ? 'handled' : n.read ? 'read' : 'unread'

const filtered = computed(() => alertStore.notices.filter(n => {
  const s = statusOf(n)
  const matchK = !filters.keyword || n.content.includes(filters.keyword) || n.type.includes(filters.keyword)
  const levelAliases: Record<string, string[]> = { critical: ['critical', '紧急'], warning: ['warning', '警告'], info: ['info', '提示'] }
  const matchL = !filters.level || (levelAliases[filters.level] || [filters.level]).includes(n.level)
  const matchS = !filters.status || s === filters.status
  return matchK && matchL && matchS
}))

const kpiTotal    = computed(() => alertStore.notices.length)
const kpiUnread   = computed(() => alertStore.notices.filter(n => !n.read && n.status !== '已处理').length)
const kpiCritical = computed(() => alertStore.notices.filter(n => n.level === 'critical' || n.level === '紧急').length)
const kpiHandled  = computed(() => alertStore.notices.filter(n => n.status === '已处理').length)

const levelLabelMap = computed<Record<string, string>>(() => ({ critical: t('monitor.severe'), warning: t('monitor.warningLevel'), info: t('monitor.infoLevel'), '紧急': t('monitor.severe'), '警告': t('monitor.warningLevel'), '提示': t('monitor.infoLevel') }))
const levelClassMap: Record<string, string> = { critical: 'level-critical', warning: 'level-warning', info: 'level-info', '紧急': 'level-critical', '警告': 'level-warning', '提示': 'level-info' }
const levelLabel = (l: string) => levelLabelMap.value[l] || l
const levelClass = (l: string) => levelClassMap[l] || 'level-info'
const statusLabelMap = computed<Record<string, string>>(() => ({ unread: t('monitor.unreadStatus'), read: t('monitor.readStatus'), handled: t('monitor.handledStatus') }))
const statusClassMap: Record<string, string> = { unread: 'mn-badge--danger', read: 'mn-badge--neutral', handled: 'mn-badge--success' }

const openDetail = (row: AlertNotice) => {
  if (!row.read) alertStore.markRead(row.id)
  detailItem.value = row
  detailVisible.value = true
}

const markHandled = async (row: AlertNotice) => {
  try {
    await alertStore.handleAlert(row.id)
    ElMessage.success(t('monitor.markedHandled'))
    detailVisible.value = false
  } catch {
    ElMessage.error(t('monitor.operationFailed'))
  }
}

const markAllRead = async () => {
  try {
    await alertStore.markAllRead()
    ElMessage.success(t('monitor.allMarkedRead'))
  } catch {
    ElMessage.error(t('monitor.operationFailed'))
  }
}

const clearHandled = async () => {
  await ElMessageBox.confirm(t('monitor.clearHandledConfirm'), t('monitor.clearHandledTitle'), { type: 'warning' })
  alertStore.clearHandled()
  ElMessage.success(t('monitor.clearedHandled'))
}

onMounted(() => {
  alertStore.fetchAlerts()
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('monitor.alertNotice') }}</h1>
        <p class="page-subtitle">{{ $t('monitor.alertSubtitle') }}</p>
      </div>
      <div class="header-actions">
        <el-button size="small" @click="markAllRead" :disabled="kpiUnread === 0">{{ $t('monitor.allRead') }}</el-button>
        <el-button size="small" type="danger" plain @click="clearHandled">{{ $t('monitor.clearHandled') }}</el-button>
      </div>
    </div>

    <!-- KPI -->
    <div class="mn-stats-strip">
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ kpiTotal }}</span><span class="mn-stat-label">{{ $t('monitor.totalAlerts') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ kpiUnread }}</span><span class="mn-stat-label">{{ $t('monitor.unread') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--danger"><span class="mn-stat-value">{{ kpiCritical }}</span><span class="mn-stat-label">{{ $t('monitor.severe') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ kpiHandled }}</span><span class="mn-stat-label">{{ $t('monitor.handled') }}</span></div>
    </div>

    <!-- Table card -->
    <el-card class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="filters.keyword" clearable :placeholder="$t('monitor.searchAlert')" style="width:240px">
            <template #prefix>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
            </template>
          </el-input>
          <el-select v-model="filters.level" clearable :placeholder="$t('monitor.levelLabel')" style="width:120px">
            <el-option :label="$t('monitor.criticalUrgent')" value="critical" />
            <el-option :label="$t('monitor.warningLevel')" value="warning" />
            <el-option :label="$t('monitor.infoLevel')" value="info" />
          </el-select>
          <el-select v-model="filters.status" clearable :placeholder="$t('common.status')" style="width:120px">
            <el-option :label="$t('monitor.unreadStatus')" value="unread" />
            <el-option :label="$t('monitor.readStatus')" value="read" />
            <el-option :label="$t('monitor.handledStatus')" value="handled" />
          </el-select>
        </div>
      </div>

      <el-table :data="filtered" stripe class="mn-table" v-loading="alertStore.loading" table-layout="auto" :row-class-name="({ row }) => !row.read && row.status !== '已处理' ? 'row-unread' : ''">
        <el-table-column :label="$t('monitor.levelLabel')" width="90" align="center">
          <template #default="{ row }">
            <span :class="['level-badge', levelClass(row.level)]">{{ levelLabel(row.level) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="content" :label="$t('monitor.alertContentLabel')" min-width="260" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="['notice-title', { 'is-unread': !row.read && row.status !== '已处理' }]">{{ row.content }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" :label="$t('monitor.source')" width="120" align="center">
          <template #default="{ row }">
            <span class="mn-badge mn-badge--neutral">{{ row.type || '--' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="triggeredAt" :label="$t('monitor.time')" width="170">
          <template #default="{ row }"><span class="mn-time">{{ row.triggeredAt }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.status')" width="100" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', statusClassMap[statusOf(row)]]">{{ statusLabelMap[statusOf(row)] }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="140">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openDetail(row)">{{ $t('common.detail') }}</el-button>
              <el-button link type="success" :disabled="row.status === '已处理'" @click="markHandled(row)">{{ $t('monitor.handle') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('monitor.noAlertNotice')" /></template>
      </el-table>
    </el-card>

    <!-- Detail drawer -->
    <el-dialog v-model="detailVisible" :title="$t('monitor.alertDetail')" width="480px" append-to-body>
      <template v-if="detailItem">
        <div class="detail-header">
          <span :class="['level-badge', 'level-badge--lg', levelClass(detailItem.level)]">{{ levelLabel(detailItem.level) }}</span>
          <span class="detail-title">{{ detailItem.content }}</span>
        </div>
        <el-descriptions :column="2" border class="detail-meta">
          <el-descriptions-item :label="$t('monitor.source')">{{ detailItem.type || '--' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('monitor.time')">{{ detailItem.triggeredAt }}</el-descriptions-item>
          <el-descriptions-item :label="$t('common.domain')">{{ detailItem.domain || '--' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('common.status')">
            <span :class="['mn-badge', statusClassMap[statusOf(detailItem)]]">{{ statusLabelMap[statusOf(detailItem)] }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>
      <template #footer>
        <el-button @click="detailVisible = false">{{ $t('common.close') }}</el-button>
        <el-button type="primary" :disabled="detailItem?.status === '已处理'" @click="detailItem && markHandled(detailItem)">{{ $t('monitor.markHandled') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-header { display:flex; align-items:flex-start; justify-content:space-between; }
.header-actions { display:flex; gap:8px; flex-shrink:0; margin-top:4px; }

.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }

.level-badge {
  display: inline-flex; align-items: center; justify-content: center;
  padding: 2px 8px; border-radius: 4px;
  font-size: 11px; font-weight: 700; color: #fff;
}
.level-badge--lg { font-size: 13px; padding: 3px 12px; }
.level-critical { background: #ef4444; }
.level-warning  { background: #f59e0b; }
.level-info     { background: #3b82f6; }

.notice-title { font-size: 13px; color: var(--app-text-primary,#1d2129); }
.notice-title.is-unread { font-weight: 700; }

:deep(.row-unread td) { background: #fffbf0 !important; }

.detail-header { display:flex; align-items:center; gap:10px; margin-bottom:16px; }
.detail-title { font-size:15px; font-weight:700; color:var(--app-text-primary,#1d2129); line-height:1.4; }
.detail-meta { margin-top:0; }
</style>
