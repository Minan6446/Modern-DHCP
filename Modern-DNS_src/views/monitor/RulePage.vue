<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useMonitorRule, formatDateTime, statusClass } from '../../composables/monitor/useMonitorRule'
import type { MonitorRuleHistory } from '../../types/modules'

const route = useRoute()
const { t } = useI18n()

const {
  loading,
  submitting,
  qpsFormRef,
  nxdomainFormRef,
  latencyFormRef,
  cacheHitFormRef,
  qpsForm,
  nxdomainForm,
  latencyForm,
  cacheHitForm,
  qpsSaving,
  nxdomainSaving,
  latencySaving,
  cacheHitSaving,
  qpsRules,
  nxdomainRules,
  latencyRules,
  cacheHitRules,
  historyPager,
  sortedRuleHistory,
  pagedRuleHistory,
  onHistorySortChange,
  saveQpsRule,
  resetQpsRule,
  saveNxdomainRule,
  resetNxdomainRule,
  saveLatencyRule,
  resetLatencyRule,
  saveCacheHitRule,
  resetCacheHitRule,
} = useMonitorRule()

const detailVisible = ref(false)
const detailRow = ref<MonitorRuleHistory | null>(null)

const openDetail = (row: MonitorRuleHistory) => {
  detailRow.value = row
  detailVisible.value = true
}

const ruleTypeLabel = (value: string): string => {
  const map: Record<string, string> = {
    qps: 'monitor.qpsAlert',
    nxdomain: 'monitor.nxdomainRateAlert',
    latency: 'monitor.latencyAlert',
    cacheHit: 'monitor.cacheHitAlert',
    QPS阈值: 'monitor.qpsAlert',
    NXDOMAIN比率: 'monitor.nxdomainRateAlert',
    响应延迟: 'monitor.latencyAlert',
    缓存命中率: 'monitor.cacheHitAlert',
  }
  const key = map[value]
  return key ? t(key) : value
}

const handleStatusLabel = (value: string): string => {
  const map: Record<string, string> = {
    已处理: 'monitor.handledStatus',
    未处理: 'monitor.unhandledStatus',
    处理中: 'monitor.processingStatus',
  }
  const key = map[value]
  return key ? t(key) : value
}

const triggerContentLabel = (value: string): string => {
  const qpsMatch = value.match(/^更新QPS阈值：全局\s*(\d+)%\s*\/\s*单域名\s*(\d+)%$/)
  if (qpsMatch) {
    return t('monitor.qpsThresholdUpdated', { global: qpsMatch[1], domain: qpsMatch[2] })
  }

  const latencyMatch = value.match(/^更新响应延迟阈值：\s*(\d+)ms$/)
  if (latencyMatch) {
    return t('monitor.latencyThresholdUpdated', { threshold: latencyMatch[1] })
  }

  const nxdomainMatch = value.match(/^更新NXDOMAIN阈值：\s*(\d+)%$/)
  if (nxdomainMatch) {
    return t('monitor.nxdomainThresholdUpdated', { threshold: nxdomainMatch[1] })
  }

  const cacheHitMatch = value.match(/^更新缓存命中率下限：\s*(\d+)%$/)
  if (cacheHitMatch) {
    return t('monitor.cacheHitThresholdUpdated', { threshold: cacheHitMatch[1] })
  }

  return value
}
</script>

<template>
  <div class="page-shell monitor-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('monitor.alertRuleSubtitle') }}</p>
      </div>
    </div>

    <div class="stack-card">
      <el-card class="form-card monitor-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon mn-panel-icon--warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
            <span>{{ $t('monitor.qpsSpikeRule') }}</span>
            <el-switch v-model="qpsForm.enabled" style="margin-left:auto" />
          </div>
        </template>
        <el-form ref="qpsFormRef" :model="qpsForm" :rules="qpsRules" label-width="160px" class="responsive-form rule-form-grid" :class="{ 'rule-form-disabled': !qpsForm.enabled }">
          <el-form-item :label="$t('monitor.globalQpsThreshold')" prop="globalThresholdPercent" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="qpsForm.globalThresholdPercent" :min="1" :max="1000" :disabled="!qpsForm.enabled" style="width: 100%" />
              <span class="rule-unit">%</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('monitor.domainQpsThreshold')" prop="domainThresholdPercent" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="qpsForm.domainThresholdPercent" :min="1" :max="1000" :disabled="!qpsForm.enabled" style="width: 100%" />
              <span class="rule-unit">%</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('monitor.checkPeriod')" prop="periodSec" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="qpsForm.periodSec" :min="1" :max="1000" :disabled="!qpsForm.enabled" style="width: 100%" />
              <span class="rule-unit">{{ $t('monitor.seconds') }}</span>
            </div>
          </el-form-item>
        </el-form>
        <div class="rule-footer">
          <div class="action-row monitor-action-row">
            <el-button type="primary" :loading="qpsSaving" :disabled="qpsSaving || submitting" @click="saveQpsRule">{{ $t('monitor.saveRule') }}</el-button>
            <el-button :disabled="qpsSaving || submitting" @click="resetQpsRule">{{ $t('monitor.resetRule') }}</el-button>
          </div>
          <span class="helper-text rule-helper">{{ $t('monitor.ruleChangeEffective') }}</span>
        </div>
      </el-card>

      <el-card class="form-card monitor-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon mn-panel-icon--danger" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            <span>{{ $t('monitor.nxdomainSpikeRule') }}</span>
            <el-switch v-model="nxdomainForm.enabled" style="margin-left:auto" />
          </div>
        </template>
        <el-form ref="nxdomainFormRef" :model="nxdomainForm" :rules="nxdomainRules" label-width="160px" class="responsive-form rule-form-grid" :class="{ 'rule-form-disabled': !nxdomainForm.enabled }">
          <el-form-item :label="$t('monitor.nxdomainThreshold')" prop="thresholdPercent" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="nxdomainForm.thresholdPercent" :min="1" :max="1000" :disabled="!nxdomainForm.enabled" style="width: 100%" />
              <span class="rule-unit">%</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('monitor.checkPeriod')" prop="periodSec" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="nxdomainForm.periodSec" :min="1" :max="1000" :disabled="!nxdomainForm.enabled" style="width: 100%" />
              <span class="rule-unit">{{ $t('monitor.seconds') }}</span>
            </div>
          </el-form-item>
          <el-form-item class="span-4" />
        </el-form>
        <div class="rule-footer">
          <div class="action-row monitor-action-row">
            <el-button type="primary" :loading="nxdomainSaving" :disabled="nxdomainSaving || submitting" @click="saveNxdomainRule">{{ $t('monitor.saveRule') }}</el-button>
            <el-button :disabled="nxdomainSaving || submitting" @click="resetNxdomainRule">{{ $t('monitor.resetRule') }}</el-button>
          </div>
          <span class="helper-text rule-helper">{{ $t('monitor.ruleChangeEffective') }}</span>
        </div>
      </el-card>

      <el-card class="form-card monitor-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon mn-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
            <span>{{ $t('monitor.latencyAlertRule') }}</span>
            <el-switch v-model="latencyForm.enabled" style="margin-left:auto" />
          </div>
        </template>
        <el-form ref="latencyFormRef" :model="latencyForm" :rules="latencyRules" label-width="160px" class="responsive-form rule-form-grid" :class="{ 'rule-form-disabled': !latencyForm.enabled }">
          <el-form-item :label="$t('monitor.latencyThresholdMs')" prop="thresholdMs" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="latencyForm.thresholdMs" :min="1" :max="10000" :disabled="!latencyForm.enabled" style="width: 100%" />
              <span class="rule-unit">ms</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('monitor.checkPeriod')" prop="periodSec" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="latencyForm.periodSec" :min="1" :max="1000" :disabled="!latencyForm.enabled" style="width: 100%" />
              <span class="rule-unit">{{ $t('monitor.seconds') }}</span>
            </div>
          </el-form-item>
          <el-form-item class="span-4" />
        </el-form>
        <div class="rule-footer">
          <div class="action-row monitor-action-row">
            <el-button type="primary" :loading="latencySaving" :disabled="latencySaving || submitting" @click="saveLatencyRule">{{ $t('monitor.saveRule') }}</el-button>
            <el-button :disabled="latencySaving || submitting" @click="resetLatencyRule">{{ $t('monitor.resetRule') }}</el-button>
          </div>
          <span class="helper-text rule-helper">{{ $t('monitor.ruleChangeEffective') }}</span>
        </div>
      </el-card>

      <el-card class="form-card monitor-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon mn-panel-icon--warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
            <span>{{ $t('monitor.cacheHitAlertRule') }}</span>
            <el-switch v-model="cacheHitForm.enabled" style="margin-left:auto" />
          </div>
        </template>
        <el-form ref="cacheHitFormRef" :model="cacheHitForm" :rules="cacheHitRules" label-width="160px" class="responsive-form rule-form-grid" :class="{ 'rule-form-disabled': !cacheHitForm.enabled }">
          <el-form-item :label="$t('monitor.minHitRate')" prop="minHitPercent" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="cacheHitForm.minHitPercent" :min="1" :max="100" :disabled="!cacheHitForm.enabled" style="width: 100%" />
              <span class="rule-unit">%</span>
            </div>
          </el-form-item>
          <el-form-item :label="$t('monitor.checkPeriod')" prop="periodSec" class="span-4">
            <div class="rule-input-inline">
              <el-input-number v-model="cacheHitForm.periodSec" :min="1" :max="3600" :disabled="!cacheHitForm.enabled" style="width: 100%" />
              <span class="rule-unit">{{ $t('monitor.seconds') }}</span>
            </div>
          </el-form-item>
          <el-form-item class="span-4" />
        </el-form>
        <div class="rule-footer">
          <div class="action-row monitor-action-row">
            <el-button type="primary" :loading="cacheHitSaving" :disabled="cacheHitSaving || submitting" @click="saveCacheHitRule">{{ $t('monitor.saveRule') }}</el-button>
            <el-button :disabled="cacheHitSaving || submitting" @click="resetCacheHitRule">{{ $t('monitor.resetRule') }}</el-button>
          </div>
          <span class="helper-text rule-helper">{{ $t('monitor.ruleChangeEffective') }}</span>
        </div>
      </el-card>

      <el-card class="table-card monitor-card">
        <template #header>
          <div class="mn-panel-header">
            <svg class="mn-panel-icon mn-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
            <span>{{ $t('monitor.ruleHistory') }}</span>
          </div>
        </template>
        <el-table :data="pagedRuleHistory" stripe v-loading="loading" class="mn-table" @sort-change="onHistorySortChange">
          <el-table-column prop="ruleId" :label="$t('monitor.ruleId')" min-width="140" sortable="custom" />
          <el-table-column prop="ruleType" :label="$t('monitor.ruleType')" min-width="140" sortable="custom">
            <template #default="{ row }">{{ ruleTypeLabel(row.ruleType) }}</template>
          </el-table-column>
          <el-table-column :label="$t('monitor.triggerTime')" min-width="180" sortable="custom">
            <template #default="{ row }"><span class="mn-time">{{ formatDateTime(row.triggerAt) }}</span></template>
          </el-table-column>
          <el-table-column prop="content" :label="$t('monitor.triggerContent')" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">{{ triggerContentLabel(row.content) }}</template>
          </el-table-column>
          <el-table-column prop="handleStatus" :label="$t('monitor.handleStatus')" width="120">
            <template #default="{ row }">
              <span :class="[
                'mn-badge',
                statusClass(row.handleStatus) === 'tag-success' ? 'mn-badge--success' :
                statusClass(row.handleStatus) === 'tag-danger'  ? 'mn-badge--danger'  :
                statusClass(row.handleStatus) === 'tag-warning' ? 'mn-badge--warning' : 'mn-badge--neutral'
              ]">{{ handleStatusLabel(row.handleStatus) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="$t('common.operation')" width="100">
            <template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">{{ $t('common.detail') }}</el-button></template>
          </el-table-column>
        </el-table>
        <div class="mn-pagination">
          <el-pagination v-model:current-page="historyPager.page" v-model:page-size="historyPager.size" layout="total, sizes, prev, pager, next" :page-sizes="[8, 16, 32]" :total="sortedRuleHistory.length" small background />
        </div>
      </el-card>
    </div>

    <el-dialog v-model="detailVisible" :title="$t('monitor.ruleDetail')" width="480px" append-to-body>
      <template v-if="detailRow">
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item :label="$t('monitor.ruleId')">{{ detailRow.ruleId }}</el-descriptions-item>
          <el-descriptions-item :label="$t('monitor.ruleType')">{{ ruleTypeLabel(detailRow.ruleType) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('monitor.triggerTime')">{{ formatDateTime(detailRow.triggerAt) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('monitor.triggerContent')">{{ triggerContentLabel(detailRow.content) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('monitor.handleStatus')">
            <span :class="[
              'mn-badge',
              statusClass(detailRow.handleStatus) === 'tag-success' ? 'mn-badge--success' :
              statusClass(detailRow.handleStatus) === 'tag-danger'  ? 'mn-badge--danger'  :
              statusClass(detailRow.handleStatus) === 'tag-warning' ? 'mn-badge--warning' : 'mn-badge--neutral'
            ]">{{ handleStatusLabel(detailRow.handleStatus) }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>
      <template #footer>
        <el-button @click="detailVisible = false">{{ $t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.monitor-shell {
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

.monitor-card :deep(.el-card__header) {
  padding: 13px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.monitor-card :deep(.el-card__body) {
  padding: 20px;
}

.table-card :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Panel header ═══════════════ */
.mn-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}

.mn-panel-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.mn-panel-icon--accent  { color: var(--app-accent); }
.mn-panel-icon--warning { color: var(--app-warning); }
.mn-panel-icon--danger  { color: var(--app-danger); }

/* ═══════════════ Rule form ═══════════════ */
.rule-form-disabled {
  opacity: 0.45;
  pointer-events: none;
}

.rule-form-grid :deep(.el-form-item) {
  margin-bottom: 16px;
}

.rule-form-grid :deep(.el-form-item__label) {
  display: inline-flex;
  align-items: center;
}

.rule-input-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.rule-unit {
  width: 26px;
  text-align: center;
  font-size: 12px;
  color: var(--app-text-regular);
  flex-shrink: 0;
}

.rule-footer {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  padding-top: 4px;
  border-top: 1px solid var(--app-border);
  margin-top: 4px;
}

.rule-helper,
.helper-text {
  font-size: 12px;
  color: var(--app-text-regular);
  text-align: right;
}

.monitor-action-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* ═══════════════ History table ═══════════════ */
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

.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 768px) {
  .rule-footer {
    flex-direction: column;
    align-items: flex-start;
  }

  .rule-helper {
    text-align: left;
  }
}
</style>




