<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BaseChart from '../../components/BaseChart.vue'
import { useDashboard } from '../../composables/dashboard/useDashboard'

const route = useRoute()
const router = useRouter()
const currentView = computed(() => String(route.meta.view || 'overview'))

const {
  dashboardStore,
  loading,
  overviewRefreshing,
  overviewRefreshCooldown,
  overviewAutoRefreshEnabled,
  overviewAutoRefreshCountdown,
  domainDetecting,
  alertBatchProcessing,
  domainSelection,
  alertSelection,
  domainFilters,
  alertFilters,
  domainPager,
  alertPager,
  auditPager,
  resourceChartRange,
  resourceMetricModes,
  overviewRangeOptions,
  qpsViewOptions,
  resourceDimensionOptions,
  overviewMetrics,
  qpsDataset,
  overviewDataset,
  showOverviewSkeleton,
  filteredDomainRows,
  pagedDomainRows,
  domainStatusStats,
  filteredAlertRows,
  pagedAlertRows,
  pagedAuditLogs,
  alertAuditLogs,
  resourceSource,
  hasActiveAlertFilters,
  alertStats,
  unreadAlertCount,
  resourceMetricMap,
  resourceUsageList,
  resourceTrendSummary,
  qpsOption,
  responseOption,
  cacheOption,
  totalOption,
  resourceOption,
  formatDateTime,
  formatNumber,
  metricStatusTagType,
  metricTrendClass,
  usageProgressColor,
  usageStatusTagType,
  usageStatusText,
  rowStatusTag,
  alertLevelTag,
  alertStatusTag,
  domainStatusText,
  alertLevelText,
  alertStatusText,
  availabilityClass,
  domainRowClassName,
  alertRowClassName,
  refreshCurrentView,
  startOverviewAutoRefresh,
  handleOverviewRangeChange,
  handleQpsGranularityChange,
  exportDomainTable,
  openDomainDetail,
  detectAllDomains,
  detectSelectedDomains,
  copyDomainIp,
  applyAlertQuickFilter,
  resetAlertFilters,
  markAlertHandled,
  markSelectedAlertsRead,
  markAllAlertsRead,
  viewAlertDetail,
  resourceExportCommand,
  securityPosture,
  securityPostureLoading,
  fetchSecurityPosture,
  initWatchers,
} = useDashboard()

initWatchers(currentView)
startOverviewAutoRefresh(currentView.value)

</script>

<template>
  <div class="page-shell dashboard-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('common.lastUpdate') }}{{ dashboardStore.lastUpdated || $t('common.loading') }}</p>
      </div>
      <div class="page-actions">
        <el-button :loading="overviewRefreshing" :disabled="overviewRefreshing || overviewRefreshCooldown" @click="refreshCurrentView(currentView)">
          <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
          {{ $t('common.refreshNow') }}
        </el-button>
      </div>
    </div>

    <template v-if="currentView === 'overview'">
      <!-- Time range toolbar -->
      <el-card class="mn-card">
        <div class="mn-toolbar">
          <div class="mn-toolbar-filters">
            <el-radio-group size="small" :model-value="dashboardStore.overviewRange" @change="handleOverviewRangeChange">
              <el-radio-button v-for="item in overviewRangeOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
            <el-date-picker
              v-if="dashboardStore.overviewRange === 'custom'"
              :model-value="dashboardStore.overviewCustomRange"
              type="daterange"
              value-format="YYYY-MM-DD"
              range-separator="-"
              :start-placeholder="$t('common.startDate')"
              :end-placeholder="$t('common.endDate')"
              @update:model-value="dashboardStore.setOverviewCustomRange"
            />
          </div>
          <div class="mn-toolbar-actions overview-auto-refresh">
            <el-switch v-model="overviewAutoRefreshEnabled" inline-prompt :active-text="$t('dashboard.autoRefresh')" :inactive-text="$t('dashboard.paused')" />
            <span :class="overviewAutoRefreshEnabled ? 'auto-refresh-on' : 'auto-refresh-off'">
              {{ overviewAutoRefreshEnabled ? `${$t('dashboard.running')} ${overviewAutoRefreshCountdown}s` : $t('dashboard.paused') }}
            </span>
          </div>
        </div>
      </el-card>

      <!-- Metric stat tiles -->
      <div class="mn-metric-grid">
        <div v-for="item in overviewMetrics" :key="item.key" class="mn-metric-tile">
          <div class="mn-metric-head">
            <span class="mn-metric-label">{{ item.label }}</span>
            <span v-if="item.key === 'response'" :class="['mn-badge', metricStatusTagType(item) === 'success' ? 'mn-badge--success' : 'mn-badge--warning']">{{ item.status }}</span>
          </div>
          <div class="mn-metric-value">{{ item.value }}</div>
          <div class="mn-metric-trend" :class="metricTrendClass(item.trendDirection)">
            <svg class="mn-trend-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline v-if="item.trendDirection === 'up'" points="18 15 12 9 6 15"/>
              <polyline v-else points="6 9 12 15 18 9"/>
            </svg>
            <span>{{ $t('dashboard.trendCompare') }}{{ item.trendDirection === 'down' ? $t('dashboard.trendDown') : $t('dashboard.trendUp') }}</span>
            <strong>{{ item.trend }}</strong>
          </div>
          <el-progress v-if="item.key === 'cache'" :percentage="item.percent" :stroke-width="6" :color="item.percent >= 80 ? 'var(--app-success)' : 'var(--app-accent)'" :show-text="false" class="mn-metric-progress" />
        </div>
      </div>

      <!-- Charts 2x2 -->
      <div class="mn-chart-grid">
        <el-card class="mn-chart-card">
          <div class="mn-chart-header">
            <div class="mn-chart-title">
              <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
              {{ $t('dashboard.qpsTrend') }}
            </div>
            <el-radio-group :model-value="dashboardStore.qpsGranularity" size="small" @change="handleQpsGranularityChange">
              <el-radio-button v-for="item in qpsViewOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-chart-body">
            <el-skeleton v-if="showOverviewSkeleton" animated :rows="7" />
            <BaseChart v-else :option="qpsOption" :loading="false" :empty="!qpsDataset.data?.length" />
          </div>
        </el-card>
        <el-card class="mn-chart-card">
          <div class="mn-chart-header">
            <div class="mn-chart-title">
              <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              {{ $t('dashboard.responseTrend') }}
            </div>
            <el-radio-group :model-value="dashboardStore.qpsGranularity" size="small" @change="handleQpsGranularityChange">
              <el-radio-button v-for="item in qpsViewOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-chart-body">
            <el-skeleton v-if="showOverviewSkeleton" animated :rows="7" />
            <BaseChart v-else :option="responseOption" :loading="false" :empty="!overviewDataset.response?.xAxis?.length" />
          </div>
        </el-card>
        <el-card class="mn-chart-card">
          <div class="mn-chart-header">
            <div class="mn-chart-title">
              <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
              {{ $t('dashboard.cacheHitRatio') }}
            </div>
            <el-radio-group :model-value="dashboardStore.qpsGranularity" size="small" @change="handleQpsGranularityChange">
              <el-radio-button v-for="item in qpsViewOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-chart-body">
            <el-skeleton v-if="showOverviewSkeleton" animated :rows="7" />
            <BaseChart v-else :option="cacheOption" :loading="false" :empty="!overviewDataset.cache?.series?.length" />
          </div>
          <div class="mn-cache-summary">
            <div class="mn-cache-stat">
              <span class="mn-cache-dot mn-cache-dot--hit"></span>
              <span class="mn-cache-stat-label">{{ $t('dashboard.cacheHit') }}</span>
              <strong>{{ formatNumber(overviewDataset.cache?.series?.[0]?.value || 0) }}</strong>
            </div>
            <div class="mn-cache-stat">
              <span class="mn-cache-dot mn-cache-dot--miss"></span>
              <span class="mn-cache-stat-label">{{ $t('dashboard.cacheMiss') }}</span>
              <strong>{{ formatNumber(overviewDataset.cache?.series?.[1]?.value || 0) }}</strong>
            </div>
          </div>
        </el-card>
        <el-card class="mn-chart-card">
          <div class="mn-chart-header">
            <div class="mn-chart-title">
              <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
              {{ $t('dashboard.totalTrend') }}
            </div>
            <el-radio-group :model-value="dashboardStore.qpsGranularity" size="small" @change="handleQpsGranularityChange">
              <el-radio-button v-for="item in qpsViewOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-chart-body">
            <el-skeleton v-if="showOverviewSkeleton" animated :rows="7" />
            <BaseChart v-else :option="totalOption" :loading="false" :empty="!overviewDataset.total?.xAxis?.length" />
          </div>
        </el-card>
      </div>

      <!-- Security Posture Card -->
      <el-card class="mn-card mn-security-card">
        <div class="mn-panel-header">
          <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
          <span>{{ $t('dashboard.securityPosture') }}</span>
          <span v-if="securityPosture?.sampledAt" class="mn-security-sampled">{{ $t('dashboard.sampledAt') }}{{ formatDateTime(securityPosture.sampledAt) }}</span>
        </div>
        <div v-loading="securityPostureLoading" class="mn-security-grid">
          <div class="mn-security-item">
            <div class="mn-security-value" :class="{ 'is-danger': (securityPosture?.lockedAccounts ?? 0) > 0 }">{{ securityPosture?.lockedAccounts ?? '--' }}</div>
            <div class="mn-security-label">{{ $t('dashboard.lockedAccounts') }}</div>
          </div>
          <div class="mn-security-item">
            <div class="mn-security-value">{{ securityPosture?.activeSessions ?? '--' }}</div>
            <div class="mn-security-label">{{ $t('dashboard.activeSessions') }}</div>
            <div class="mn-security-detail" v-if="securityPosture">{{ securityPosture.onlineUsers }} {{ $t('dashboard.onlineUsers') }}</div>
          </div>
          <div class="mn-security-item">
            <div class="mn-security-value" :class="{ 'is-warning': (securityPosture?.kickedLast24h ?? 0) > 0 }">{{ securityPosture?.kickedLast24h ?? '--' }}</div>
            <div class="mn-security-label">{{ $t('dashboard.kickedLast24h') }}</div>
          </div>
          <div class="mn-security-item">
            <div class="mn-security-value" :class="{ 'is-warning': (securityPosture?.overduePasswordUsers ?? 0) > 0 }">{{ securityPosture?.overduePasswordUsers ?? '--' }}</div>
            <div class="mn-security-label">{{ $t('dashboard.overduePasswords') }}</div>
          </div>
          <div class="mn-security-item">
            <div class="mn-security-value is-accent">{{ formatNumber(securityPosture?.paddingInbound ?? 0) }}</div>
            <div class="mn-security-label">{{ $t('dashboard.paddingInbound') }}</div>
          </div>
          <div class="mn-security-item">
            <div class="mn-security-value is-accent">{{ formatNumber(securityPosture?.paddingOutbound ?? 0) }}</div>
            <div class="mn-security-label">{{ $t('dashboard.paddingOutbound') }}</div>
          </div>
        </div>
      </el-card>
    </template>

    <template v-else-if="currentView === 'domain-status'">
      <!-- Domain status stat tiles -->
      <div class="mn-stats-strip">
        <div v-for="item in domainStatusStats" :key="item.label" class="mn-stat-tile">
          <span class="mn-stat-value">{{ item.value }}</span>
          <span class="mn-stat-label">{{ item.label }}</span>
        </div>
      </div>

      <el-card class="mn-card">
        <div class="mn-toolbar">
          <div class="mn-toolbar-filters">
            <el-input v-model="domainFilters.keyword" clearable :placeholder="$t('dashboard.domainSearch')" style="width:240px">
              <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
            </el-input>
            <el-select v-model="domainFilters.status" clearable :placeholder="$t('dashboard.resolveStatus')" style="width:140px">
              <el-option :label="$t('dashboard.normal')" value="正常" />
              <el-option :label="$t('dashboard.abnormal')" value="异常" />
              <el-option :label="$t('dashboard.undetected')" value="未检测" />
            </el-select>
          </div>
          <div class="mn-toolbar-actions">
            <el-button :loading="loading" @click="refreshCurrentView">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
              {{ $t('common.refresh') }}
            </el-button>
            <el-button @click="exportDomainTable">
              <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
              {{ $t('common.export') }}
            </el-button>
            <el-button :disabled="!domainSelection.length || domainDetecting" :loading="domainDetecting" @click="detectSelectedDomains">{{ $t('dashboard.detectSelected') }}</el-button>
            <el-button type="primary" :disabled="domainDetecting" :loading="domainDetecting" @click="detectAllDomains">{{ $t('dashboard.detectAll') }}</el-button>
          </div>
        </div>

        <div v-loading="loading || domainDetecting">
          <el-table v-if="filteredDomainRows.length" :data="pagedDomainRows" class="mn-table" @selection-change="domainSelection = $event" :row-class-name="domainRowClassName">
            <el-table-column type="selection" width="48" />
            <el-table-column prop="domain" :label="$t('common.domain')" min-width="220" sortable>
              <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
            </el-table-column>
            <el-table-column :label="$t('dashboard.resolveStatus')" width="110" sortable align="center">
              <template #default="{ row }">
                <span :class="['mn-badge', rowStatusTag(row.status) === 'success' ? 'mn-badge--success' : rowStatusTag(row.status) === 'danger' ? 'mn-badge--danger' : 'mn-badge--neutral']">{{ domainStatusText(row.status) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('dashboard.resolveAvailability')" width="130" sortable align="center">
              <template #default="{ row }">
                <span class="mn-availability" :class="availabilityClass(row.availability)">
                  {{ row.availability === '--' ? '—' : `${row.availability}%` }}
                </span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('dashboard.checkIp')" width="180" sortable>
              <template #default="{ row }">
                <div class="mn-ip-cell">
                  <span class="mn-mono">{{ row.checkIp || '—' }}</span>
                  <el-button v-if="row.checkIp" link type="primary" size="small" @click="copyDomainIp(row.checkIp)">{{ $t('common.copy') }}</el-button>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('dashboard.lastCheck')" min-width="170" sortable>
              <template #default="{ row }"><span class="mn-time">{{ formatDateTime(row.checkedAt) }}</span></template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="100" fixed="right" align="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openDomainDetail(row)">{{ $t('common.detail') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else :description="$t('dashboard.noMatchDomain')" :image-size="80">
            <el-button type="primary" @click="router.push('/domain/zone-list')">{{ $t('dashboard.addDomain') }}</el-button>
          </el-empty>
        </div>

        <div v-if="filteredDomainRows.length" class="mn-pagination">
          <el-pagination v-model:current-page="domainPager.page" v-model:page-size="domainPager.size" layout="total, sizes, prev, pager, next" :page-sizes="[10, 20, 50]" :total="filteredDomainRows.length" size="small" background />
        </div>
      </el-card>
    </template>

    <template v-else-if="currentView === 'alert'">
      <!-- Alert quick-filter stat tiles -->
      <div class="mn-stats-strip">
        <div
          v-for="item in alertStats"
          :key="item.key"
          class="mn-stat-tile mn-stat-tile--clickable"
          :class="{ 'mn-stat-tile--active': alertFilters.quick === item.key, 'mn-stat-tile--danger': item.emphasize }"
          @click="applyAlertQuickFilter(item.key)"
        >
          <span class="mn-stat-value">{{ item.value }}</span>
          <span class="mn-stat-label">{{ item.label }}</span>
        </div>
      </div>

      <!-- Alert table -->
      <el-card class="mn-card">
        <div class="mn-toolbar">
          <div class="mn-toolbar-filters">
            <el-input v-model="alertFilters.keyword" clearable :placeholder="$t('dashboard.alertSearch')" style="width:240px">
              <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
            </el-input>
            <el-select v-model="alertFilters.level" clearable :placeholder="$t('dashboard.alertLevel')" style="width:120px">
              <el-option :label="$t('dashboard.urgent')" value="紧急" />
              <el-option :label="$t('dashboard.warning')" value="警告" />
              <el-option :label="$t('dashboard.info')" value="提示" />
            </el-select>
            <el-select v-model="alertFilters.type" clearable :placeholder="$t('dashboard.alertType')" style="width:150px">
              <el-option label="DDoS" value="DDoS攻击" />
              <el-option label="QPS Spike" value="QPS突增" />
              <el-option label="DNSSEC" value="DNSSEC异常" />
              <el-option label="Resolve Fail" value="解析失败" />
            </el-select>
            <el-date-picker v-model="alertFilters.range" type="daterange" value-format="YYYY-MM-DD" range-separator="-" :start-placeholder="$t('common.start')" :end-placeholder="$t('common.end')" style="width:240px" />
            <el-button text :disabled="!hasActiveAlertFilters" @click="resetAlertFilters">{{ $t('common.reset') }}</el-button>
          </div>
          <div class="mn-toolbar-actions">
            <span class="mn-selection-hint-inline" v-if="alertSelection.length">{{ $t('dashboard.selected') }} <strong>{{ alertSelection.length }}</strong> {{ $t('dashboard.items') }}</span>
            <el-button :disabled="!alertSelection.length || alertBatchProcessing" :loading="alertBatchProcessing" @click="markSelectedAlertsRead">{{ $t('dashboard.batchRead') }}</el-button>
            <el-button type="primary" :disabled="!unreadAlertCount || alertBatchProcessing" :loading="alertBatchProcessing" @click="markAllAlertsRead">{{ $t('dashboard.allRead') }}</el-button>
          </div>
        </div>

        <div v-loading="loading || alertBatchProcessing">
          <el-table v-if="filteredAlertRows.length" :data="pagedAlertRows" class="mn-table" @selection-change="alertSelection = $event" :row-class-name="alertRowClassName">
            <el-table-column type="selection" width="48" />
            <el-table-column :label="$t('dashboard.level')" width="90" sortable align="center">
              <template #default="{ row }">
                <span :class="['mn-badge', alertLevelTag(row.level) === 'danger' ? 'mn-badge--danger' : alertLevelTag(row.level) === 'warning' ? 'mn-badge--warning' : 'mn-badge--neutral']">{{ alertLevelText(row.level) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="type" :label="$t('dashboard.alertType')" width="130" sortable />
            <el-table-column prop="domain" :label="$t('dashboard.relatedDomain')" min-width="180" sortable>
              <template #default="{ row }"><span class="mn-mono">{{ row.domain }}</span></template>
            </el-table-column>
            <el-table-column prop="content" :label="$t('dashboard.alertContent')" min-width="260" show-overflow-tooltip />
            <el-table-column :label="$t('dashboard.triggerTime')" min-width="170" sortable>
              <template #default="{ row }"><span class="mn-time">{{ formatDateTime(row.triggeredAt) }}</span></template>
            </el-table-column>
            <el-table-column :label="$t('dashboard.processStatus')" width="110" sortable align="center">
              <template #default="{ row }">
                <span :class="['mn-badge', alertStatusTag(row.status) === 'success' ? 'mn-badge--success' : 'mn-badge--neutral']">{{ alertStatusText(row.status) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="170" fixed="right" align="right">
              <template #default="{ row }">
                <div class="mn-row-ops">
                  <el-button link type="primary" :disabled="row.status === 'handled' || row.status === '已处理'" @click="markAlertHandled(row)">{{ $t('dashboard.markHandled') }}</el-button>
                  <el-button link type="primary" @click="viewAlertDetail(row)">{{ $t('common.detail') }}</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else :description="$t('dashboard.noMatchAlert')" :image-size="80">
            <el-button type="primary" @click="resetAlertFilters">{{ $t('dashboard.resetFilter') }}</el-button>
          </el-empty>
        </div>

        <div v-if="filteredAlertRows.length" class="mn-pagination">
          <el-pagination v-model:current-page="alertPager.page" v-model:page-size="alertPager.size" layout="total, sizes, prev, pager, next" :page-sizes="[10, 20, 50]" :total="filteredAlertRows.length" size="small" background />
        </div>
      </el-card>

      <!-- Alert audit log -->
      <el-card class="mn-card">
        <div class="mn-panel-header">
          <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
          <span>{{ $t('dashboard.alertAuditLog') }}</span>
        </div>
        <el-table :data="pagedAuditLogs" class="mn-table">
          <el-table-column prop="operator" :label="$t('audit.operator')" width="140" />
          <el-table-column prop="action" :label="$t('audit.action')" width="180" />
          <el-table-column prop="target" :label="$t('audit.target')" min-width="220" />
          <el-table-column prop="result" :label="$t('audit.result')" width="100" />
          <el-table-column :label="$t('audit.time')" min-width="180">
            <template #default="{ row }"><span class="mn-time">{{ formatDateTime(row.time) }}</span></template>
          </el-table-column>
        </el-table>
        <div class="mn-pagination">
          <el-pagination v-model:current-page="auditPager.page" v-model:page-size="auditPager.size" layout="total, prev, pager, next" :total="alertAuditLogs.length" size="small" background />
        </div>
      </el-card>
    </template>

    <template v-else-if="currentView === 'resource'">
      <!-- Resource metric tiles -->
      <div class="mn-resource-grid">
        <div class="mn-resource-tile">
          <div class="mn-resource-tile-head">
            <span class="mn-metric-label">{{ $t('dashboard.todayQueries') }}</span>
            <el-radio-group v-model="resourceMetricModes.query" size="small">
              <el-radio-button v-for="item in resourceDimensionOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-metric-value">{{ resourceMetricMap.query?.value }}</div>
          <div class="mn-metric-detail">{{ resourceMetricMap.query?.detail }}</div>
        </div>
        <div class="mn-resource-tile">
          <div class="mn-resource-tile-head">
            <span class="mn-metric-label">{{ $t('dashboard.totalZones') }}</span>
            <el-radio-group v-model="resourceMetricModes.zone" size="small">
              <el-radio-button v-for="item in resourceDimensionOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-metric-value">{{ resourceMetricMap.zone?.value }}</div>
          <div class="mn-metric-detail">{{ resourceMetricMap.zone?.detail }}</div>
        </div>
        <div class="mn-resource-tile">
          <div class="mn-resource-tile-head">
            <span class="mn-metric-label">{{ $t('dashboard.totalRecords') }}</span>
            <el-radio-group v-model="resourceMetricModes.record" size="small">
              <el-radio-button v-for="item in resourceDimensionOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="mn-metric-value">{{ resourceMetricMap.record?.value }}</div>
          <div class="mn-metric-detail">{{ resourceMetricMap.record?.detail }}</div>
        </div>
      </div>

      <!-- Resource chart -->
      <el-card class="mn-card">
        <div class="mn-chart-header">
          <div class="mn-chart-title">
            <svg class="mn-chart-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
            {{ $t('dashboard.resourceUsage') }}
          </div>
          <div class="mn-chart-actions">
            <div class="mn-resource-summary">
              <span class="mn-summary-chip">{{ $t('dashboard.todayQueries') }} {{ resourceTrendSummary.query }}</span>
              <span class="mn-summary-chip">{{ $t('dashboard.totalZones') }} {{ resourceTrendSummary.zone }}</span>
              <span class="mn-summary-chip">{{ $t('dashboard.totalRecords') }} {{ resourceTrendSummary.record }}</span>
            </div>
            <el-radio-group v-model="resourceChartRange" size="small">
              <el-radio-button v-for="item in resourceDimensionOptions" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
            </el-radio-group>
            <el-dropdown @command="resourceExportCommand">
              <el-button type="primary">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></template>
                {{ $t('common.export') }}
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="excel">{{ $t('common.exportExcel') }}</el-dropdown-item>
                  <el-dropdown-item command="image">{{ $t('common.exportImage') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
        <div class="mn-chart-body">
          <BaseChart :option="resourceOption" :loading="loading" :empty="!resourceSource.trend?.xAxis?.length" height="360px" />
        </div>
      </el-card>

      <!-- Usage rates -->
      <el-card class="mn-card">
        <div class="mn-panel-header">
          <svg class="mn-panel-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
          <span>{{ $t('dashboard.resourceUsage') }}</span>
        </div>
        <div class="mn-usage-grid">
          <div v-for="item in resourceUsageList" :key="item.label" class="mn-usage-item" :class="{ 'mn-usage-item--high': item.percent >= 90 }">
            <div class="mn-usage-head">
              <span class="mn-usage-label">{{ item.label }}</span>
              <div class="mn-usage-right">
                <span :class="['mn-badge', usageStatusTagType(item.percent) === 'success' ? 'mn-badge--success' : usageStatusTagType(item.percent) === 'warning' ? 'mn-badge--warning' : usageStatusTagType(item.percent) === 'danger' ? 'mn-badge--danger' : 'mn-badge--neutral']">{{ usageStatusText(item.percent) }}</span>
                <strong class="mn-usage-pct">{{ item.percent }}%</strong>
              </div>
            </div>
            <el-progress :percentage="item.percent" :stroke-width="8" :show-text="false" :color="usageProgressColor(item.percent)" />
            <span class="mn-usage-detail">{{ item.detail }}</span>
          </div>
        </div>
      </el-card>
    </template>
  </div>
</template>

<style scoped>
/* ═══════════════ Shell ═══════════════ */
.dashboard-shell { gap: 16px; }

/* ═══════════════ Card base ═══════════════ */
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
.overview-auto-refresh { gap: 8px; }
.auto-refresh-on  { font-size: 12px; color: var(--app-success); }
.auto-refresh-off { font-size: 12px; color: var(--app-text-regular); }

/* ═══════════════ Overview metric tiles ═══════════════ */
.mn-metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.mn-metric-tile {
  padding: 20px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mn-metric-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.mn-metric-label {
  font-size: 12px;
  color: var(--app-text-regular);
  font-weight: 500;
}
.mn-metric-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--app-title);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.mn-metric-detail {
  font-size: 12px;
  color: var(--app-text-regular);
}
.mn-metric-trend {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  margin-top: 2px;
}
.mn-metric-trend.is-up   { color: var(--app-success); }
.mn-metric-trend.is-down { color: var(--app-danger); }
.mn-trend-icon { width: 13px; height: 13px; flex-shrink: 0; }
.mn-metric-progress { margin-top: 8px; }

/* ═══════════════ Chart grid ═══════════════ */
.mn-chart-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.mn-chart-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}
.mn-chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px 12px;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
  flex-wrap: wrap;
}
.mn-chart-title {
  display: flex;
  align-items: center;
  gap: 7px;
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
.mn-chart-body { padding: 16px; }
.mn-chart-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* Cache summary */
.mn-cache-summary {
  display: flex;
  gap: 16px;
  padding: 10px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}
.mn-cache-stat {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
}
.mn-cache-stat strong { color: var(--app-title); }
.mn-cache-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.mn-cache-dot--hit  { background: var(--app-accent); }
.mn-cache-dot--miss { background: var(--app-border); }
.mn-cache-stat-label { color: var(--app-text-regular); }

/* ═══════════════ Stats strip (domain / alert) ═══════════════ */
.mn-stats-strip {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.mn-stat-tile {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 14px 20px;
  border-radius: 10px;
  border: 1px solid var(--app-border);
  background: var(--app-bg);
  min-width: 110px;
  flex: 1;
  box-shadow: var(--app-shadow-soft);
}
.mn-stat-tile--clickable {
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
}
.mn-stat-tile--clickable:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.09);
}
.mn-stat-tile--active {
  outline: 2px solid var(--app-accent);
  outline-offset: -1px;
}
.mn-stat-tile--danger { background: rgba(245,63,63,0.04); border-color: rgba(245,63,63,0.3); }
.mn-stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--app-title);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.mn-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Panel header ═══════════════ */
.mn-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}
.mn-panel-icon {
  width: 15px;
  height: 15px;
  color: var(--app-accent);
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
:deep(.dashboard-row-danger > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.04);
}
:deep(.dashboard-row-alert > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.06);
  animation: alert-pulse 1.4s ease-in-out infinite;
}
@keyframes alert-pulse {
  0%, 100% { filter: brightness(1); }
  50%       { filter: brightness(0.95); }
}

.mn-row-ops {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}
.mn-ip-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

/* ═══════════════ Pagination ═══════════════ */
.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 10px 16px;
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
.mn-badge--neutral { color: var(--app-text-secondary); background: rgba(78,89,105,0.08); border-color: rgba(78,89,105,0.18); }
.mn-badge--success { color: var(--app-success); background: rgba(0,180,42,0.08); border-color: rgba(0,180,42,0.22); }
.mn-badge--danger  { color: var(--app-danger);  background: rgba(245,63,63,0.08); border-color: rgba(245,63,63,0.22); }
.mn-badge--warning { color: var(--app-warning); background: rgba(255,125,0,0.08); border-color: rgba(255,125,0,0.22); }

/* ═══════════════ Text helpers ═══════════════ */
.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}
.mn-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}
.mn-availability {
  font-weight: 600;
  font-size: 13px;
}
.mn-availability.is-good    { color: var(--app-success); }
.mn-availability.is-warning { color: var(--app-warning); }
.mn-availability.is-danger  { color: var(--app-danger); }
.mn-availability.is-unknown { color: var(--app-text-secondary); }

.mn-selection-hint-inline {
  font-size: 12px;
  color: var(--app-accent);
}

/* ═══════════════ Resource view ═══════════════ */
.mn-resource-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.mn-resource-tile {
  padding: 20px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  box-shadow: var(--app-shadow-soft);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mn-resource-tile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}
.mn-resource-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.mn-summary-chip {
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 12px;
  color: var(--app-text-secondary);
  background: var(--app-bg-secondary);
  border: 1px solid var(--app-border);
}

/* Usage grid */
.mn-usage-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  padding: 16px;
}
.mn-usage-item {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg-secondary);
}
.mn-usage-item--high {
  border-color: rgba(245,63,63,0.35);
  background: rgba(245,63,63,0.03);
}
.mn-usage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.mn-usage-label { font-size: 13px; font-weight: 500; color: var(--app-text); }
.mn-usage-right { display: flex; align-items: center; gap: 8px; }
.mn-usage-pct   { font-size: 14px; font-weight: 700; color: var(--app-title); }
.mn-usage-detail { font-size: 12px; color: var(--app-text-regular); }

/* ═══════════════ Security Posture ═══════════════ */
.mn-security-card { margin-top: 0; }
.mn-security-sampled {
  margin-left: auto;
  font-size: 11px;
  font-weight: 400;
  color: var(--app-text-regular);
}
.mn-security-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 0;
  min-height: 90px;
}
.mn-security-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 18px 12px;
  border-right: 1px solid var(--app-border);
}
.mn-security-item:last-child { border-right: none; }
.mn-security-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--app-title);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.mn-security-value.is-danger  { color: var(--app-danger); }
.mn-security-value.is-warning { color: var(--app-warning); }
.mn-security-value.is-accent  { color: var(--app-accent); }
.mn-security-label {
  font-size: 12px;
  color: var(--app-text-regular);
  font-weight: 500;
  text-align: center;
}
.mn-security-detail {
  font-size: 11px;
  color: var(--app-text-secondary);
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1200px) {
  .mn-metric-grid,
  .mn-chart-grid,
  .mn-resource-grid,
  .mn-usage-grid,
  .mn-stats-strip { grid-template-columns: 1fr; }
  .mn-security-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .mn-toolbar { flex-direction: column; align-items: flex-start; }
  .mn-toolbar-actions { justify-content: flex-start; }
  .mn-chart-header { flex-direction: column; align-items: flex-start; }
  .mn-resource-summary { width: 100%; }
}
</style>




