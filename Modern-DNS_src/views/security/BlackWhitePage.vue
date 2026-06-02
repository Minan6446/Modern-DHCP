<script setup lang="ts">
import { UploadFilled, EditPen, Delete } from '@element-plus/icons-vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import { useSecurity, formatDateTime, formatCompactNumber } from '../../composables/security/useSecurity'

const props = withDefaults(defineProps<{ view?: string }>(), { view: '' })

const route = useRoute()
const { t } = useI18n()
const currentView = computed(() => props.view || String(route.meta.view || 'black-white'))

const {
  securityStore,
  loading, submitting, checking,
  bwFilters, bwPager,
  selectedBwRows, bwDialogVisible, bwFormRef, bwForm, bwRules,
  importDialogVisible, refreshLoading, batchRuleLoading,
  deletingRuleId, deletingDdosRuleId, statusSwitchingRuleId,
  exportLoading, importLoading,
  ddosFormRef, ddosGlobalForm, ddosGlobalRules,
  ddosRuleDialogVisible, ddosRuleFormRef, ddosRuleForm, ddosRuleRules,
  dnssecCheckingRowId,
  dnssecFilters, dnssecPager,
  dnssecDetailDialogVisible, dnssecDetail,
  qpsUsagePercent, qpsProgressColor,
  sortedBwRows, pagedBwRows,
  pagedDnssecRows, sortedDnssecRows, statsCards,
  blackWhiteRowClassName, dnssecRowClassName,
  refreshSecurityData,
  openBwDialog, submitBwRule,
  handleRuleStatusChange, removeRule, batchOperateRules,
  exportRules, importRules,
  saveGlobalDdos, resetGlobalDdos,
  openDdosRuleDialog, submitDdosRule, removeDdosRule,
  runCheckAll, runCheckOne,
  openDnssecDetail, copyDnssecKeys,
  handleBwSortChange, handleDnssecSortChange,
} = useSecurity()

const bwStats = computed(() => {
  const rows = securityStore.blackWhiteRules ?? []
  const blackCount = rows.filter((r: { listType: string }) => r.listType === '黑名单').length
  const whiteCount = rows.filter((r: { listType: string }) => r.listType === '白名单').length
  const enabledCount = rows.filter((r: { status: string }) => r.status === '启用').length
  return { total: rows.length, black: blackCount, white: whiteCount, enabled: enabledCount }
})

const bwListTypeLabel = (value: string): string => (value === '白名单' ? t('security.whitelist') : t('security.blacklist'))

const bwStatusLabel = (value: string): string => (value === '启用' ? t('common.enabled') : t('common.disabled'))

const dnssecStatusLabel = (value: string): string => (value === '已开启' ? t('security.opened') : t('security.notOpened'))

const signatureStatusLabel = (value: string): string => {
  if (value === '有效') return t('common.valid')
  if (value === '异常') return t('common.error')
  return value
}
</script>

<template>
  <div class="page-shell security-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('security.accessControlSubtitle') }}</p>
      </div>
    </div>

    <!-- ═══════════ 黑白名单 ═══════════ -->
    <template v-if="currentView === 'black-white'">
      <!-- Stats strip -->
      <div class="bw-stats">
        <div class="bw-stat-card">
          <div class="bw-stat-icon bw-stat-icon--total">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 12l2 2 4-4"/><path d="M21 12c0 4.97-4.03 9-9 9s-9-4.03-9-9 4.03-9 9-9 9 4.03 9 9z"/></svg>
          </div>
          <div class="bw-stat-body">
            <span class="bw-stat-label">{{ $t('security.totalRules') }}</span>
            <span class="bw-stat-value">{{ bwStats.total }}</span>
          </div>
        </div>
        <div class="bw-stat-card">
          <div class="bw-stat-icon bw-stat-icon--black">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>
          </div>
          <div class="bw-stat-body">
            <span class="bw-stat-label">{{ $t('security.blacklist') }}</span>
            <span class="bw-stat-value">{{ bwStats.black }}</span>
          </div>
        </div>
        <div class="bw-stat-card">
          <div class="bw-stat-icon bw-stat-icon--white">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
          </div>
          <div class="bw-stat-body">
            <span class="bw-stat-label">{{ $t('security.whitelist') }}</span>
            <span class="bw-stat-value">{{ bwStats.white }}</span>
          </div>
        </div>
        <div class="bw-stat-card">
          <div class="bw-stat-icon bw-stat-icon--enabled">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64A9 9 0 1 1 5.64 6.64"/><line x1="12" y1="2" x2="12" y2="12"/></svg>
          </div>
          <div class="bw-stat-body">
            <span class="bw-stat-label">{{ $t('security.enabledCount') }}</span>
            <span class="bw-stat-value">{{ bwStats.enabled }}</span>
          </div>
        </div>
      </div>

      <!-- Table card -->
      <el-card class="security-card bw-table-card">
        <!-- Toolbar -->
        <div class="sec-toolbar">
          <div class="sec-toolbar-filters">
            <el-input v-model="bwFilters.keyword" clearable :placeholder="$t('security.searchBwPlaceholder')" style="width: 240px">
              <template #prefix>
                <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
              </template>
            </el-input>
            <el-select v-model="bwFilters.type" clearable :placeholder="$t('common.type')" style="width: 110px">
              <el-option :label="$t('common.all')" value="" />
              <el-option label="IP" value="IP" />
              <el-option :label="$t('common.domain')" value="域名" />
            </el-select>
            <el-select v-model="bwFilters.listType" clearable :placeholder="$t('security.listType')" style="width: 130px">
              <el-option :label="$t('common.all')" value="" />
              <el-option :label="$t('security.blacklist')" value="黑名单" />
              <el-option :label="$t('security.whitelist')" value="白名单" />
            </el-select>
            <el-select v-model="bwFilters.status" clearable :placeholder="$t('common.status')" style="width: 110px">
              <el-option :label="$t('common.all')" value="" />
              <el-option :label="$t('common.enabled')" value="启用" />
              <el-option :label="$t('common.disabled')" value="禁用" />
            </el-select>
          </div>
          <div class="sec-toolbar-actions">
            <el-button
              :loading="refreshLoading"
              :disabled="refreshLoading || submitting"
              @click="refreshSecurityData"
            >
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
              </template>
              {{ $t('common.refresh') }}
            </el-button>
            <el-button
              :loading="batchRuleLoading"
              :disabled="!selectedBwRows.length || batchRuleLoading"
              @click="batchOperateRules('enable')"
            >{{ $t('security.batchEnable') }}</el-button>
            <el-button
              :loading="batchRuleLoading"
              :disabled="!selectedBwRows.length || batchRuleLoading"
              @click="batchOperateRules('disable')"
            >{{ $t('security.batchDisable') }}</el-button>
            <el-button
              type="danger"
              plain
              :loading="batchRuleLoading"
              :disabled="!selectedBwRows.length || batchRuleLoading"
              @click="batchOperateRules('delete')"
            >{{ $t('common.batchDelete') }}</el-button>
            <el-button
              :loading="importLoading"
              :disabled="importLoading"
              @click="importDialogVisible = true"
            >
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
              </template>
              {{ $t('security.importList') }}
            </el-button>
            <el-dropdown @command="exportRules">
              <el-button :loading="exportLoading" :disabled="exportLoading">
                <template #icon>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                </template>
                {{ $t('security.exportList') }}
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="all-excel">{{ $t('security.exportAllExcel') }}</el-dropdown-item>
                  <el-dropdown-item command="all-json">{{ $t('security.exportAllJson') }}</el-dropdown-item>
                  <el-dropdown-item command="selected-excel">{{ $t('security.exportSelectedExcel') }}</el-dropdown-item>
                  <el-dropdown-item command="selected-json">{{ $t('security.exportSelectedJson') }}</el-dropdown-item>
                  <el-dropdown-item command="filtered-excel">{{ $t('security.exportFilteredExcel') }}</el-dropdown-item>
                  <el-dropdown-item command="filtered-json">{{ $t('security.exportFilteredJson') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button type="primary" @click="openBwDialog()">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              </template>
              {{ $t('security.addRule') }}
            </el-button>
          </div>
        </div>

        <!-- Table -->
        <template v-if="sortedBwRows.length">
          <el-table
            :data="pagedBwRows"
            stripe
            v-loading="loading"
            :row-class-name="blackWhiteRowClassName"
            highlight-current-row
            class="bw-table"
            @selection-change="selectedBwRows = $event"
            @sort-change="handleBwSortChange"
          >
            <el-table-column type="selection" width="48" />
            <el-table-column prop="ruleId" :label="$t('security.ruleId')" width="180" sortable="custom" />
            <el-table-column prop="type" :label="$t('common.type')" width="90" sortable="custom">
              <template #default="{ row }">
                <span class="bw-badge bw-badge--neutral">{{ row.type }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="listType" :label="$t('security.listType')" width="100" sortable="custom">
              <template #default="{ row }">
                <span :class="row.listType === '白名单' ? 'bw-badge bw-badge--white' : 'bw-badge bw-badge--black'">{{ bwListTypeLabel(row.listType) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="value" :label="$t('common.value')" min-width="240" sortable="custom" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="bw-mono">{{ row.value }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="remark" :label="$t('common.remark')" min-width="180" show-overflow-tooltip />
            <el-table-column prop="status" :label="$t('common.status')" width="140" sortable="custom">
              <template #default="{ row }">
                <div class="status-cell">
                  <el-switch
                    :model-value="row.status === '启用'"
                    size="small"
                    :disabled="batchRuleLoading || Boolean(deletingRuleId) || Boolean(statusSwitchingRuleId)"
                    @change="(enabled: string | number | boolean) => handleRuleStatusChange(row, Boolean(enabled))"
                  />
                  <span :class="row.status === '启用' ? 'bw-badge bw-badge--on' : 'bw-badge bw-badge--off'">{{ bwStatusLabel(row.status) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.createdAt')" width="170" sortable="custom">
              <template #default="{ row }">
                <span class="bw-time">{{ formatDateTime(row.createdAt) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="200" fixed="right" align="center" class-name="operation-column">
              <template #default="{ row }">
                <div class="row-op-group">
                  <el-button link type="primary" class="row-op-btn row-op-btn--edit" @click="openBwDialog(row)">
                    <el-icon><EditPen /></el-icon>
                    <span>{{ $t('common.edit') }}</span>
                  </el-button>
                  <span class="row-op-divider" aria-hidden="true"></span>
                  <el-button
                    link
                    type="danger"
                    class="row-op-btn row-op-btn--delete"
                    :loading="deletingRuleId === row.id"
                    :disabled="batchRuleLoading || Boolean(statusSwitchingRuleId)"
                    @click="removeRule(row)"
                  >
                    <el-icon><Delete /></el-icon>
                    <span>{{ $t('common.delete') }}</span>
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <div class="sec-pagination">
            <el-pagination
              v-model:current-page="bwPager.page"
              v-model:page-size="bwPager.size"
              layout="total, sizes, prev, pager, next"
              small
              background
              :page-sizes="[10, 20, 50]"
              :total="sortedBwRows.length"
            />
          </div>
        </template>
        <el-empty v-else :description="$t('security.noBwRules')" />
      </el-card>
    </template>

    <!-- ═══════════ DDoS防护 ═══════════ -->
    <div v-else-if="currentView === 'ddos'" class="stack-card">
      <el-card class="security-card">
        <template #header>
          <div class="sec-panel-header">
            <svg class="sec-panel-icon sec-panel-icon--warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            <span>{{ $t('security.globalQpsLimit') }}</span>
          </div>
        </template>
        <div class="ddos-global-grid">
          <el-form ref="ddosFormRef" :model="ddosGlobalForm" :rules="ddosGlobalRules" label-width="100px" class="ddos-global-left">
            <el-form-item :label="$t('security.totalQpsLimit')" prop="qpsLimit">
              <el-input-number v-model="ddosGlobalForm.qpsLimit" :min="1" :max="100000" style="width: 100%" />
            </el-form-item>
            <el-form-item :label="$t('security.currentQps')">
              <div class="ddos-progress-block">
                <el-progress :percentage="qpsUsagePercent" :color="qpsProgressColor" />
                <div class="ddos-progress-label">
                  {{ $t('security.qpsUsage', { current: formatCompactNumber(ddosGlobalForm.currentQps), limit: formatCompactNumber(ddosGlobalForm.qpsLimit), percent: qpsUsagePercent }) }}
                </div>
              </div>
            </el-form-item>
            <!-- New abuse-mitigation knobs. 0 = disabled, so operators
                 can roll out gradually without committing to a number
                 they have not yet sized for their traffic profile. -->
            <el-form-item :label="$t('security.perIpConnLimit')" prop="perIpConnLimit">
              <el-input-number v-model="ddosGlobalForm.perIpConnLimit" :min="0" :max="100000" style="width: 100%" />
              <div class="ddos-field-hint">{{ $t('security.perIpConnLimitHint') }}</div>
            </el-form-item>
            <el-form-item :label="$t('security.memSoftMb')" prop="memSoftMb">
              <el-input-number v-model="ddosGlobalForm.memSoftMb" :min="0" :max="65536" style="width: 100%" />
              <div class="ddos-field-hint">{{ $t('security.memSoftMbHint') }}</div>
            </el-form-item>
            <el-form-item :label="$t('security.memHardMb')" prop="memHardMb">
              <el-input-number v-model="ddosGlobalForm.memHardMb" :min="0" :max="65536" style="width: 100%" />
              <div class="ddos-field-hint">{{ $t('security.memHardMbHint') }}</div>
            </el-form-item>
          </el-form>
          <div class="ddos-global-right">
            <span class="ddos-switch-label">{{ $t('security.enableProtection') }}</span>
            <el-switch v-model="ddosGlobalForm.enabled" />
          </div>
        </div>
        <div class="ddos-action-row">
          <el-button type="primary" :loading="submitting" :disabled="submitting" @click="saveGlobalDdos">{{ $t('security.saveConfig') }}</el-button>
          <el-button :disabled="submitting" @click="resetGlobalDdos">{{ $t('security.resetConfig') }}</el-button>
        </div>
      </el-card>

      <el-card class="security-card">
        <template #header>
          <div class="sec-panel-header">
            <svg class="sec-panel-icon sec-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
            <span>{{ $t('security.domainRateLimit') }}</span>
            <el-button type="primary" class="sec-panel-btn" @click="openDdosRuleDialog()">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              </template>
              {{ $t('security.addRateLimitRule') }}
            </el-button>
          </div>
        </template>
        <template v-if="securityStore.ddosDomainRules.length">
          <el-table :data="securityStore.ddosDomainRules" stripe v-loading="loading" class="bw-table">
            <el-table-column prop="domain" :label="$t('cache.domain')" min-width="240" sortable />
            <el-table-column prop="qpsLimit" :label="$t('security.qpsLimit')" width="160" sortable align="right">
              <template #default="{ row }">{{ formatCompactNumber(row.qpsLimit) }}</template>
            </el-table-column>
            <el-table-column prop="status" :label="$t('common.status')" width="120" sortable>
              <template #default="{ row }">
                <span :class="row.status === '启用' ? 'bw-badge bw-badge--on' : 'bw-badge bw-badge--off'">{{ bwStatusLabel(row.status) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.createdAt')" min-width="180" sortable>
              <template #default="{ row }">
                <span class="bw-time">{{ formatDateTime(row.createdAt) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" width="170" fixed="right" align="center" class-name="operation-column">
              <template #default="{ row }">
                <div class="row-op-group">
                  <el-button link type="primary" class="row-op-btn" @click="openDdosRuleDialog(row)">{{ $t('common.edit') }}</el-button>
                  <el-button
                    link
                    type="danger"
                    class="row-op-btn"
                    :loading="deletingDdosRuleId === row.id"
                    :disabled="submitting"
                    @click="removeDdosRule(row)"
                  >{{ $t('common.delete') }}</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </template>
        <el-empty v-else :description="$t('security.noRateLimitRules')" />
      </el-card>
    </div>

    <!-- ═══════════ DNSSEC ═══════════ -->
    <div v-else-if="currentView === 'dnssec'" class="stack-card">
      <!-- Stats overview -->
      <el-card class="security-card">
        <template #header>
          <div class="sec-panel-header">
            <svg class="sec-panel-icon sec-panel-icon--success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
            <span>{{ $t('security.dnssecOverview') }}</span>
            <el-button type="primary" class="sec-panel-btn" :loading="checking" :disabled="checking || Boolean(dnssecCheckingRowId)" @click="runCheckAll">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
              </template>
              {{ $t('security.checkAllSignatures') }}
            </el-button>
          </div>
        </template>
        <div class="dnssec-stats-grid">
          <div v-for="item in statsCards" :key="item.label" class="dnssec-stat-card">
            <span class="dnssec-stat-label">{{ item.label }}</span>
            <span class="dnssec-stat-value" :class="item.key === 'abnormal' ? 'dnssec-stat-value--danger' : ''">{{ item.value }}</span>
          </div>
        </div>
      </el-card>

      <!-- Domain table -->
      <el-card class="security-card">
        <template #header>
          <div class="sec-panel-header">
            <svg class="sec-panel-icon sec-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
            <span>{{ $t('security.domainSignatureDetails') }}</span>
            <el-input v-model="dnssecFilters.keyword" clearable :placeholder="$t('common.searchDomain')" style="width: 220px; margin-left: auto">
              <template #prefix>
                <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
              </template>
            </el-input>
          </div>
        </template>
        <template v-if="sortedDnssecRows.length">
          <el-table
            :data="pagedDnssecRows"
            stripe
            v-loading="loading"
            :row-class-name="dnssecRowClassName"
            class="bw-table"
            @sort-change="handleDnssecSortChange"
          >
            <el-table-column prop="domain" :label="$t('cache.domain')" min-width="220" sortable="custom" />
            <el-table-column prop="dnssecStatus" :label="$t('security.dnssecStatus')" width="140" sortable="custom">
              <template #default="{ row }">
                <span :class="row.dnssecStatus === '已开启' ? 'bw-badge bw-badge--on' : 'bw-badge bw-badge--off'">{{ dnssecStatusLabel(row.dnssecStatus) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="signatureStatus" :label="$t('security.signatureStatus')" width="130" sortable="custom">
              <template #default="{ row }">
                <span :class="row.signatureStatus === '有效' ? 'bw-badge bw-badge--on' : row.signatureStatus === '异常' ? 'bw-badge bw-badge--danger' : 'bw-badge bw-badge--off'">
                  {{ signatureStatusLabel(row.signatureStatus) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('security.lastCheckTime')" min-width="180" sortable="custom">
              <template #default="{ row }">
                <span class="bw-time">{{ formatDateTime(row.lastCheckAt) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="$t('common.operation')" min-width="200" fixed="right">
              <template #default="{ row }">
                <div class="table-operations">
                  <el-button link type="primary" @click="openDnssecDetail(row)">{{ $t('cache.viewDetail') }}</el-button>
                  <el-button
                    v-if="row.dnssecStatus === '已开启'"
                    link
                    type="warning"
                    :loading="dnssecCheckingRowId === row.id"
                    :disabled="checking || Boolean(dnssecCheckingRowId)"
                    @click="runCheckOne(row)"
                  >{{ $t('security.recheck') }}</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <div class="sec-pagination">
            <el-pagination
              v-model:current-page="dnssecPager.page"
              v-model:page-size="dnssecPager.size"
              layout="total, sizes, prev, pager, next"
              small
              background
              :page-sizes="[10, 20, 50]"
              :total="sortedDnssecRows.length"
            />
          </div>
        </template>
        <el-empty v-else :description="$t('security.noDnssecData')" />
      </el-card>
    </div>

    <!-- ═══════════ Dialogs ═══════════ -->
    <el-dialog v-model="bwDialogVisible" :title="bwForm.id ? $t('security.editRule') : $t('security.addRule')" width="500px" destroy-on-close>
      <el-form ref="bwFormRef" :model="bwForm" :rules="bwRules" label-width="100px" class="bw-rule-form">
        <el-form-item :label="$t('common.type')" prop="type">
          <el-select v-model="bwForm.type" style="width: 100%">
            <el-option label="IP" value="IP" />
            <el-option :label="$t('common.domain')" value="域名" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('security.listType')" prop="listType">
          <el-select v-model="bwForm.listType" style="width: 100%">
            <el-option :label="$t('security.blacklist')" value="黑名单" />
            <el-option :label="$t('security.whitelist')" value="白名单" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('common.value')" prop="value">
          <el-input v-model="bwForm.value" :placeholder="bwForm.type === 'IP' ? $t('security.ipPlaceholder') : $t('security.domainWildcardPlaceholder')" />
        </el-form-item>
        <el-form-item :label="$t('common.remark')">
          <el-input v-model="bwForm.remark" type="textarea" :rows="3" resize="none" :placeholder="$t('common.remarkOptional')" />
        </el-form-item>
        <el-form-item :label="$t('common.status')">
          <el-switch v-model="bwForm.status" active-value="启用" inactive-value="禁用" :active-text="$t('common.enabled')" :inactive-text="$t('common.disabled')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bwDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" :disabled="submitting" @click="submitBwRule">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialogVisible" :title="$t('security.importList')" width="520px" destroy-on-close>
      <el-upload drag :show-file-list="false" :auto-upload="false" :before-upload="importRules" accept=".xlsx,.xls,.json">
        <el-icon><UploadFilled /></el-icon>
        <div>{{ $t('security.dragOrClickUpload') }}</div>
        <template #tip><div class="el-upload__tip">{{ $t('security.supportExcelJson') }}</div></template>
      </el-upload>
    </el-dialog>

    <el-dialog v-model="ddosRuleDialogVisible" :title="ddosRuleForm.id ? $t('security.editRateLimitRule') : $t('security.addRateLimitRule')" width="500px" destroy-on-close>
      <el-form ref="ddosRuleFormRef" :model="ddosRuleForm" :rules="ddosRuleRules" label-width="110px">
        <el-form-item :label="$t('cache.domain')" prop="domain">
          <el-input v-model="ddosRuleForm.domain" :placeholder="$t('security.domainPlaceholder')" />
        </el-form-item>
        <el-form-item :label="$t('security.qpsLimit')" prop="qpsLimit">
          <el-input-number v-model="ddosRuleForm.qpsLimit" :min="1" :max="100000" style="width: 100%" />
        </el-form-item>
        <el-form-item :label="$t('common.status')">
          <el-switch v-model="ddosRuleForm.status" active-value="启用" inactive-value="禁用" :active-text="$t('common.enabled')" :inactive-text="$t('common.disabled')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ddosRuleDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" :disabled="submitting" @click="submitDdosRule">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dnssecDetailDialogVisible" :title="$t('security.dnssecKeyDetail')" width="640px" destroy-on-close>
      <el-descriptions v-if="dnssecDetail" :column="1" border>
        <el-descriptions-item :label="$t('cache.domain')">{{ dnssecDetail.domain }}</el-descriptions-item>
        <el-descriptions-item :label="$t('security.dnssecStatus')">{{ dnssecDetail.dnssecStatus }}</el-descriptions-item>
        <el-descriptions-item :label="$t('security.signatureStatus')">{{ dnssecDetail.signatureStatus }}</el-descriptions-item>
        <el-descriptions-item :label="$t('security.kskKeyInfo')">
          <div v-if="dnssecDetail.ksk?.length">
            <div v-for="item in dnssecDetail.ksk" :key="item.keyId" class="key-row">{{ item.keyId }} / {{ item.createdAt }} / {{ item.status }}</div>
          </div>
          <span v-else class="key-empty">{{ $t('common.none') }}</span>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('security.zskKeyInfo')">
          <div v-if="dnssecDetail.zsk?.length">
            <div v-for="item in dnssecDetail.zsk" :key="item.keyId" class="key-row">{{ item.keyId }} / {{ item.createdAt }} / {{ item.status }}</div>
          </div>
          <span v-else class="key-empty">{{ $t('common.none') }}</span>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('security.lastCheckTime')">{{ dnssecDetail.lastCheckAt }}</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="dnssecDetailDialogVisible = false">{{ $t('cache.close') }}</el-button>
        <el-button type="primary" @click="copyDnssecKeys">{{ $t('security.copyKeyInfo') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page shell ═══════════════ */
.security-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ═══════════════ Cards ═══════════════ */
.security-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.security-card :deep(.el-card__body) {
  padding: 0;
}

.security-card :deep(.el-card__header) {
  padding: 13px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

/* ═══════════════ Stats strip ═══════════════ */
.bw-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.bw-stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 18px;
  background: #fff;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
}

.bw-stat-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.bw-stat-icon svg {
  width: 20px;
  height: 20px;
}

.bw-stat-icon--total  { background: rgba(22, 93, 255, 0.10); color: var(--app-accent); }
.bw-stat-icon--black  { background: rgba(245, 63, 63, 0.10); color: var(--app-danger); }
.bw-stat-icon--white  { background: rgba(0, 180, 42, 0.10);  color: var(--app-success); }
.bw-stat-icon--enabled { background: rgba(255, 125, 0, 0.10); color: var(--app-warning); }

.bw-stat-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.bw-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
  font-weight: 400;
}

.bw-stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--app-title);
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}

/* ═══════════════ Table card (no inner padding) ═══════════════ */
.bw-table-card :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Toolbar ═══════════════ */
.sec-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.sec-toolbar-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}

.sec-toolbar-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}

.input-icon {
  width: 14px;
  height: 14px;
  color: var(--app-text-regular);
}

/* ═══════════════ Panel header (DDoS / DNSSEC cards) ═══════════════ */
.sec-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
}

.sec-panel-icon {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.sec-panel-icon--accent  { color: var(--app-accent); }
.sec-panel-icon--success { color: var(--app-success); }
.sec-panel-icon--warning { color: var(--app-warning); }
.sec-panel-icon--danger  { color: var(--app-danger); }

.sec-panel-btn {
  margin-left: auto;
  flex-shrink: 0;
}

/* ═══════════════ Table ═══════════════ */
.bw-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.bw-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.bw-table :deep(.el-table__body tr.current-row > td.el-table__cell),
.bw-table :deep(.el-table__body tr.el-table__row--selected > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.08) !important;
}

.bw-table :deep(.operation-column .cell) {
  overflow: visible;
}

.status-cell {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.row-op-group {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(78, 89, 105, 0.06);
}

.row-op-btn {
  padding: 0;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.row-op-btn :deep(.el-icon) {
  font-size: 13px;
}

.row-op-btn--edit {
  color: var(--app-accent);
}

.row-op-btn--delete {
  color: var(--app-danger);
}

.row-op-divider {
  width: 1px;
  height: 12px;
  background: var(--app-border);
}

@media (max-width: 1360px) {
  .row-op-group {
    gap: 6px;
    padding: 2px 6px;
  }

  .row-op-btn span {
    font-size: 12px;
  }
}

/* Black-list row tint */
.security-shell :deep(.bw-row-black > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.03);
}

.security-shell :deep(.bw-row-white > td.el-table__cell) {
  background: transparent;
}

.security-shell :deep(.dnssec-row-abnormal > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.04);
}

/* ═══════════════ Custom badges ═══════════════ */
.bw-badge {
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

.bw-badge--neutral {
  color: var(--app-text-secondary);
  background: rgba(78, 89, 105, 0.08);
  border-color: rgba(78, 89, 105, 0.18);
}

.bw-badge--black {
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.22);
}

.bw-badge--white {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

.bw-badge--on {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

.bw-badge--off {
  color: var(--app-text-regular);
  background: rgba(78, 89, 105, 0.06);
  border-color: rgba(78, 89, 105, 0.15);
}

.bw-badge--danger {
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.22);
}

/* ═══════════════ Mono value & time ═══════════════ */
.bw-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
  color: var(--app-text-main, var(--app-title));
}

.bw-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-regular);
}

/* ═══════════════ Pagination ═══════════════ */
.sec-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ DDoS form panel ═══════════════ */
.ddos-global-grid {
  display: grid;
  grid-template-columns: 1fr 200px;
  gap: 16px;
  padding: 20px;
}

.ddos-global-left :deep(.el-form-item) {
  margin-bottom: 16px;
}

.ddos-global-right {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg-secondary);
  padding: 16px;
}

.ddos-switch-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-regular);
}

.ddos-progress-block {
  width: 100%;
}

.ddos-progress-label {
  margin-top: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
}

.ddos-field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
  line-height: 1.5;
}

.ddos-action-row {
  display: flex;
  gap: 8px;
  padding: 0 20px 20px;
}

/* ═══════════════ DNSSEC stats grid ═══════════════ */
.dnssec-stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  padding: 16px;
}

.dnssec-stat-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg-secondary);
}

.dnssec-stat-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

.dnssec-stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--app-title);
  font-variant-numeric: tabular-nums;
}

.dnssec-stat-value--danger {
  color: var(--app-danger);
}

/* ═══════════════ Dialog details ═══════════════ */
.bw-rule-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.bw-rule-form :deep(.el-textarea__inner) {
  min-height: 60px;
}

.key-row {
  font-size: 12px;
  font-family: var(--app-font-mono, 'Consolas', monospace);
  color: var(--app-title);
  padding: 2px 0;
}

.key-empty {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .bw-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 1100px) {
  .ddos-global-grid {
    grid-template-columns: 1fr;
  }

  .dnssec-stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .bw-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .sec-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .sec-toolbar-filters,
  .sec-toolbar-actions {
    width: 100%;
  }
}

@media (max-width: 480px) {
  .bw-stats {
    grid-template-columns: 1fr;
  }
}
</style>




