<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useTools, formatDateTime } from '../../composables/tools/useTools'
import { normalizeMultiValue } from '../../utils/interaction'

const route = useRoute()
const { t } = useI18n()
const currentView = computed(() => String(route.meta.view || 'dig'))

const {
  toolsStore, loading,
  digFormRef, dnssecFormRef, globalFormRef, ipFormRef,
  digForm, dnssecForm, globalTestForm, ipForm,
  digLoading, digCopying, digHistoryDeletingId,
  dnssecLoading, dnssecExporting,
  globalLoading, globalExporting,
  ipQueryLoading, ipExporting, ipCopyingAll,
  digHistoryPager, globalPager, ipPager,
  globalDetailVisible, globalDetailRow,
  ipValidationMessage,
  digRules, dnssecRules, globalRules, ipRules,
  sortedGlobalRows, pagedGlobalRows, pagedBatchIpRows, pagedDigHistory,
  dnssecStatsCards, dnssecDomainRows,
  tagClass, riskClass, riskLevel, riskLabel,
  runDig, copyDigResult, clearDigOutput, reuseDigHistory, removeDigHistory,
  runDnssec, copyDnssecReport, exportDnssecReport,
  runGlobal, openGlobalDetail, onGlobalSortChange, exportGlobalRows,
  runIpQuery, copyIpInfo, copyAllIpInfo, exportIpRows,
} = useTools()

const consistencyLabel = (value: string) => {
  const map: Record<string, string> = {
    一致: t('tools.consistent'),
    不一致: t('tools.inconsistent'),
    超时: t('tools.timeout'),
  }
  return map[value] || value
}

const dnssecStatusLabel = (value: string) => {
  const map: Record<string, string> = {
    已开启: t('security.opened'),
    未开启: t('security.notOpened'),
  }
  return map[value] || value
}
</script>
<template>
  <div class="page-shell tools-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('tools.subtitle') }}{{ $t('common.lastUpdate') }}{{ toolsStore.lastUpdated || $t('common.loading') }}</p>
      </div>
    </div>

    <!-- ══════════════ Dig 检测 ══════════════ -->
    <template v-if="currentView === 'dig'">
      <el-card class="tools-card tools-form-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
            <span>{{ $t('tools.digQueryParams') }}</span>
          </div>
        </template>
        <el-form ref="digFormRef" :model="digForm" :rules="digRules" inline class="tools-inline-form" @keyup.enter="runDig">
          <el-form-item :label="$t('tools.targetDomain')" prop="domain" required>
            <el-input v-model="digForm.domain" clearable placeholder="example.com" style="width: 240px" />
          </el-form-item>
          <el-form-item :label="$t('tools.recordType')" prop="recordType" required>
            <el-select v-model="digForm.recordType" style="width: 130px">
              <el-option v-for="item in ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SOA', 'SRV', 'CAA']" :key="item" :label="item" :value="item" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('tools.dnsServer')" prop="dnsServer">
            <el-input v-model="digForm.dnsServer" clearable :placeholder="$t('tools.defaultSystemDns')" style="width: 200px" />
          </el-form-item>
          <el-form-item class="form-action-right">
            <el-button type="primary" :loading="digLoading" :disabled="digLoading" @click="runDig">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
              </template>
              {{ $t('tools.executeQuery') }}
            </el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <div class="tools-split-layout">
        <!-- Terminal output -->
        <el-card class="tools-card tools-output-card">
          <template #header>
            <div class="tl-panel-header">
              <svg class="tl-panel-icon tl-panel-icon--mono" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
              <span>{{ $t('tools.queryResult') }}</span>
              <div class="tl-panel-actions">
                <el-button size="small" :loading="digCopying" :disabled="digCopying" @click="copyDigResult">{{ $t('tools.copyAllInOne') }}</el-button>
                <el-button size="small" @click="clearDigOutput">{{ $t('tools.clear') }}</el-button>
              </div>
            </div>
          </template>
          <el-empty v-if="!toolsStore.digOutput" :description="$t('tools.enterDomainToRunDig')" />
          <pre v-else class="tools-terminal">{{ toolsStore.digOutput }}</pre>
        </el-card>

        <!-- Query history -->
        <el-card class="tools-card tools-history-card">
          <template #header>
            <div class="tl-panel-header">
              <svg class="tl-panel-icon tl-panel-icon--muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              <span>{{ $t('tools.queryHistory') }}</span>
            </div>
          </template>
          <el-empty v-if="!toolsStore.digHistory.length" :description="$t('tools.noHistory')" />
          <template v-else>
            <div v-for="item in pagedDigHistory" :key="item.id" class="history-item" @click="reuseDigHistory(item)">
              <div class="history-main">
                <span class="history-domain">{{ item.domain }}</span>
                <span class="history-meta">
                  <span class="tl-badge tl-badge--neutral">{{ item.recordType }}</span>
                  {{ formatDateTime(item.queriedAt) }}
                </span>
              </div>
              <div class="history-actions">
                <el-button link type="primary" size="small" @click.stop="reuseDigHistory(item)">{{ $t('tools.refill') }}</el-button>
                <el-button link type="danger" size="small" :loading="digHistoryDeletingId === item.id" :disabled="Boolean(digHistoryDeletingId)" @click.stop="removeDigHistory(item)">{{ $t('common.delete') }}</el-button>
              </div>
            </div>
            <div class="tools-pagination-wrap">
              <el-pagination v-model:current-page="digHistoryPager.page" v-model:page-size="digHistoryPager.size" layout="total, prev, pager, next" :total="toolsStore.digHistory.length" small background />
            </div>
          </template>
        </el-card>
      </div>
    </template>

    <!-- ══════════════ DNSSEC 调试 ══════════════ -->
    <div v-else-if="currentView === 'dnssec-debug'" class="stack-card">
      <el-card class="tools-card tools-form-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
            <span>{{ $t('tools.dnssecDebugParams') }}</span>
          </div>
        </template>
        <el-form ref="dnssecFormRef" :model="dnssecForm" :rules="dnssecRules" inline class="tools-inline-form" @keyup.enter="runDnssec">
          <el-form-item :label="$t('tools.targetDomain')" prop="domain" required>
            <el-input v-model="dnssecForm.domain" clearable :placeholder="$t('tools.domainWildcardPlaceholder')" style="width: 320px" />
          </el-form-item>
          <el-form-item class="form-action-right">
            <el-button type="primary" :loading="dnssecLoading" :disabled="dnssecLoading" @click="runDnssec">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
              </template>
              {{ $t('tools.oneClickCheck') }}
            </el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <!-- Stats strip -->
      <div class="tl-stats-strip">
        <div v-for="item in dnssecStatsCards" :key="item.key" class="tl-stat-tile" :class="item.key === 'abnormal' ? 'tl-stat-tile--danger' : ''">
          <span class="tl-stat-tile-label">{{ item.label }}</span>
          <span class="tl-stat-tile-value">{{ item.value }}</span>
        </div>
      </div>

      <!-- Domain overview table -->
      <el-card v-if="dnssecDomainRows.length" class="tools-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
            <span>{{ $t('tools.domainStatusOverview') }}</span>
          </div>
        </template>
        <el-table :data="dnssecDomainRows" class="tl-table" size="small">
          <el-table-column prop="domain" :label="$t('common.domain')" min-width="260" />
          <el-table-column prop="dnssecStatus" :label="$t('security.dnssecStatus')" width="140">
            <template #default="{ row }">
              <span :class="row.dnssecStatus === '已开启' ? 'tl-badge tl-badge--on' : 'tl-badge tl-badge--off'">{{ dnssecStatusLabel(row.dnssecStatus) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="signatureStatus" :label="$t('security.signatureStatus')" width="140">
            <template #default="{ row }">
              <span :class="tagClass(row.signatureStatus) === 'tag-success' ? 'tl-badge tl-badge--on' : tagClass(row.signatureStatus) === 'tag-danger' ? 'tl-badge tl-badge--danger' : 'tl-badge tl-badge--warning'">{{ row.signatureStatus }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="checkedAt" :label="$t('tools.checkTime')" min-width="180" />
        </el-table>
      </el-card>

      <!-- Detail table -->
      <el-card class="tools-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
            <span>{{ $t('tools.checkDetails') }}</span>
            <div class="tl-panel-actions">
              <el-button size="small" :disabled="!toolsStore.dnssecResult" @click="copyDnssecReport">{{ $t('tools.copyReport') }}</el-button>
              <el-dropdown @command="exportDnssecReport">
                <el-button size="small" :loading="dnssecExporting" :disabled="dnssecExporting || !toolsStore.dnssecResult">{{ $t('tools.exportReport') }}</el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="text">{{ $t('tools.exportText') }}</el-dropdown-item>
                    <el-dropdown-item command="json">{{ $t('tools.exportJson') }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </div>
        </template>
        <el-table v-if="toolsStore.dnssecResult" :data="toolsStore.dnssecResult.details" stripe class="tl-table">
          <el-table-column prop="item" :label="$t('tools.checkItem')" min-width="180" />
          <el-table-column prop="status" :label="$t('common.status')" width="110">
            <template #default="{ row }">
              <span :class="tagClass(row.status) === 'tag-success' ? 'tl-badge tl-badge--on' : tagClass(row.status) === 'tag-danger' ? 'tl-badge tl-badge--danger' : 'tl-badge tl-badge--warning'">{{ row.status }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="result" :label="$t('tools.checkResult')" min-width="220" />
          <el-table-column prop="detail" :label="$t('common.detail')" min-width="260" show-overflow-tooltip />
        </el-table>
        <el-empty v-else :description="$t('tools.runDnssecFirst')" />
      </el-card>
    </div>

    <!-- ══════════════ 全球测试 ══════════════ -->
    <div v-else-if="currentView === 'global-test'" class="stack-card">
      <el-card class="tools-card tools-form-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
            <span>{{ $t('tools.globalTestParams') }}</span>
            <div class="tl-panel-actions">
              <el-button size="small" :loading="globalExporting" :disabled="globalExporting" @click="exportGlobalRows">
                <template #icon>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                </template>
                {{ $t('tools.exportExcel') }}
              </el-button>
            </div>
          </div>
        </template>
        <el-form ref="globalFormRef" :model="globalTestForm" :rules="globalRules" inline class="tools-inline-form" @keyup.enter="runGlobal">
          <el-form-item :label="$t('tools.targetDomain')" prop="domain" required>
            <el-input v-model="globalTestForm.domain" clearable :placeholder="$t('tools.enterDomain')" style="width: 240px" />
          </el-form-item>
          <el-form-item :label="$t('tools.recordType')" prop="recordType" required>
            <el-select v-model="globalTestForm.recordType" style="width: 130px">
              <el-option label="A" value="A" />
              <el-option label="AAAA" value="AAAA" />
              <el-option label="CNAME" value="CNAME" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('tools.nodeSelection')">
            <el-radio-group v-model="globalTestForm.nodeGroup" class="node-group">
              <el-radio-button :label="$t('tools.domesticNodes')" value="国内" />
              <el-radio-button :label="$t('tools.overseasNodes')" value="海外" />
              <el-radio-button :label="$t('tools.allNodes')" value="all" />
            </el-radio-group>
          </el-form-item>
          <el-form-item class="form-action-right">
            <el-button type="primary" :loading="globalLoading" :disabled="globalLoading" @click="runGlobal">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
              </template>
              {{ $t('tools.startTest') }}
            </el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <el-card class="tools-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
            <span>{{ $t('tools.testResult') }} <span v-if="sortedGlobalRows.length" class="tl-count-chip">{{ sortedGlobalRows.length }} {{ $t('tools.nodes') }}</span></span>
          </div>
        </template>
        <el-table
          :data="pagedGlobalRows"
          stripe
          v-loading="loading || globalLoading"
          class="tl-table"
          :row-class-name="({ row }) => !['一致', 'Consistent'].includes(row.consistency) ? 'tl-row-danger' : ''"
          @sort-change="onGlobalSortChange"
        >
          <el-table-column prop="node" :label="$t('tools.nodeName')" width="180" sortable="custom" />
          <el-table-column prop="operator" :label="$t('tools.operator')" width="140" sortable="custom" />
          <el-table-column prop="result" :label="$t('tools.resolveResult')" min-width="220" sortable="custom" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="tl-mono">{{ row.result }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="latency" :label="$t('monitor.responseMs')" width="110" align="right" sortable="custom" />
          <el-table-column prop="consistency" :label="$t('tools.consistency')" width="110" sortable="custom">
            <template #default="{ row }">
              <span :class="tagClass(row.consistency) === 'tag-success' ? 'tl-badge tl-badge--on' : tagClass(row.consistency) === 'tag-danger' ? 'tl-badge tl-badge--danger' : 'tl-badge tl-badge--warning'">{{ consistencyLabel(row.consistency) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="$t('common.operation')" width="100" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openGlobalDetail(row)">{{ $t('common.detail') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="tl-pagination">
          <el-pagination v-model:current-page="globalPager.page" v-model:page-size="globalPager.size" layout="total, sizes, prev, pager, next" :page-sizes="[10, 20, 50]" :total="sortedGlobalRows.length" small background />
        </div>
      </el-card>
    </div>

    <!-- ══════════════ IP 归属地 ══════════════ -->
    <div v-else class="stack-card">
      <el-card class="tools-card tools-form-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>
            <span>{{ $t('tools.ipLocationQuery') }}</span>
            <div class="tl-panel-actions">
              <el-button size="small" :loading="ipExporting" :disabled="ipExporting" @click="exportIpRows">{{ $t('tools.exportResult') }}</el-button>
              <el-button size="small" :loading="ipCopyingAll" :disabled="ipCopyingAll" @click="copyAllIpInfo">{{ $t('tools.copyAll') }}</el-button>
            </div>
          </div>
        </template>
        <el-form ref="ipFormRef" :model="ipForm" :rules="ipRules" inline class="tools-inline-form" @keyup.enter="runIpQuery">
          <el-form-item :label="$t('tools.ipAddress')" prop="ips" required>
            <el-input v-model="ipForm.ips" clearable :placeholder="$t('tools.multiIpPlaceholder')" style="width: 420px" @blur="ipForm.ips = normalizeMultiValue(ipForm.ips)" />
          </el-form-item>
          <el-form-item class="form-action-right">
            <el-button type="primary" :loading="ipQueryLoading" :disabled="ipQueryLoading" @click="runIpQuery">
              <template #icon>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
              </template>
              {{ $t('tools.startQuery') }}
            </el-button>
          </el-form-item>
        </el-form>
        <p v-if="ipValidationMessage" class="ip-error-tip">{{ ipValidationMessage }}</p>
      </el-card>

      <!-- Single IP result -->
      <el-card v-if="toolsStore.singleIpResult" class="tools-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
            <span>{{ $t('tools.locationResult') }}</span>
            <div class="tl-panel-actions">
              <el-button size="small" @click="copyIpInfo(toolsStore.singleIpResult)">{{ $t('tools.copyResult') }}</el-button>
            </div>
          </div>
        </template>
        <div class="tl-ip-grid">
          <div class="tl-ip-tile">
            <span class="tl-ip-tile-label">{{ $t('tools.ipAddress') }}</span>
            <span class="tl-ip-tile-value tl-mono">{{ toolsStore.singleIpResult.ip }}</span>
          </div>
          <div class="tl-ip-tile">
            <span class="tl-ip-tile-label">{{ $t('tools.country') }}</span>
            <span class="tl-ip-tile-value">{{ toolsStore.singleIpResult.country }} · {{ toolsStore.singleIpResult.province }} · {{ toolsStore.singleIpResult.city }}</span>
          </div>
          <div class="tl-ip-tile">
            <span class="tl-ip-tile-label">{{ $t('tools.isp') }}</span>
            <span class="tl-ip-tile-value">{{ toolsStore.singleIpResult.isp }}</span>
          </div>
          <div class="tl-ip-tile">
            <span class="tl-ip-tile-label">{{ $t('tools.location') }}</span>
            <span class="tl-ip-tile-value tl-mono">{{ toolsStore.singleIpResult.location }}</span>
          </div>
          <div class="tl-ip-tile">
            <span class="tl-ip-tile-label">{{ $t('tools.riskLevel') }}</span>
            <span class="tl-ip-tile-value">
              <span :class="riskClass(riskLevel(toolsStore.singleIpResult)) === 'tag-danger' ? 'tl-badge tl-badge--danger' : riskClass(riskLevel(toolsStore.singleIpResult)) === 'tag-warning' ? 'tl-badge tl-badge--warning' : 'tl-badge tl-badge--on'">{{ riskLabel(riskLevel(toolsStore.singleIpResult)) }}</span>
            </span>
          </div>
        </div>
      </el-card>

      <!-- Batch IP results -->
      <el-card v-else-if="toolsStore.batchIpResults.length" class="tools-card">
        <template #header>
          <div class="tl-panel-header">
            <svg class="tl-panel-icon tl-panel-icon--accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
            <span>{{ $t('tools.batchLocationResult') }} <span class="tl-count-chip">{{ toolsStore.batchIpResults.length }} {{ $t('common.items') }}</span></span>
          </div>
        </template>
        <el-table :data="pagedBatchIpRows" stripe class="tl-table">
          <el-table-column prop="ip" :label="$t('tools.ipAddress')" min-width="150">
            <template #default="{ row }"><span class="tl-mono">{{ row.ip }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('tools.country')" min-width="170">
            <template #default="{ row }">{{ row.country }} · {{ row.province }} · {{ row.city }}</template>
          </el-table-column>
          <el-table-column prop="isp" :label="$t('tools.isp')" min-width="140" />
          <el-table-column prop="location" :label="$t('tools.location')" min-width="140">
            <template #default="{ row }"><span class="tl-mono">{{ row.location }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('tools.riskLevel')" width="110">
            <template #default="{ row }">
              <span :class="riskClass(riskLevel(row)) === 'tag-danger' ? 'tl-badge tl-badge--danger' : riskClass(riskLevel(row)) === 'tag-warning' ? 'tl-badge tl-badge--warning' : 'tl-badge tl-badge--on'">{{ riskLabel(riskLevel(row)) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="$t('common.operation')" width="80" fixed="right">
            <template #default="{ row }"><el-button link type="primary" @click="copyIpInfo(row)">{{ $t('common.copy') }}</el-button></template>
          </el-table-column>
        </el-table>
        <div class="tl-pagination">
          <el-pagination v-model:current-page="ipPager.page" v-model:page-size="ipPager.size" layout="total, sizes, prev, pager, next" :page-sizes="[10, 20, 50]" :total="toolsStore.batchIpResults.length" small background />
        </div>
      </el-card>

      <el-empty v-else :description="$t('tools.enterIpToQuery')" />
    </div>

    <!-- ══════════════ 全球测试节点详情 Dialog ══════════════ -->
    <el-dialog v-model="globalDetailVisible" :title="$t('tools.nodeDetail')" width="500px" destroy-on-close>
      <div v-if="globalDetailRow" class="detail-grid">
        <div class="detail-item"><span>{{ $t('tools.nodeName') }}</span><strong>{{ globalDetailRow.node }}</strong></div>
        <div class="detail-item"><span>{{ $t('tools.operator') }}</span><strong>{{ globalDetailRow.operator }}</strong></div>
        <div class="detail-item"><span>{{ $t('tools.resolveResult') }}</span><strong class="tl-mono">{{ globalDetailRow.result }}</strong></div>
        <div class="detail-item"><span>{{ $t('monitor.responseTime') }}</span><strong>{{ globalDetailRow.latency }} ms</strong></div>
        <div class="detail-item">
          <span>{{ $t('tools.consistencyStatus') }}</span>
          <strong>
            <span :class="tagClass(globalDetailRow.consistency) === 'tag-success' ? 'tl-badge tl-badge--on' : tagClass(globalDetailRow.consistency) === 'tag-danger' ? 'tl-badge tl-badge--danger' : 'tl-badge tl-badge--warning'">{{ consistencyLabel(globalDetailRow.consistency) }}</span>
          </strong>
        </div>
        <div class="detail-item detail-item-block"><span>{{ $t('common.detail') }}</span><strong>{{ globalDetailRow.detail }}</strong></div>
      </div>
      <template #footer>
        <el-button @click="globalDetailVisible = false">{{ $t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.tools-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ═══════════════ Cards ═══════════════ */
.tools-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.tools-card :deep(.el-card__header) {
  padding: 13px 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.tools-card :deep(.el-card__body) {
  padding: 16px;
}

.tools-form-card :deep(.el-card__body) {
  padding: 16px 16px 8px;
}

/* ═══════════════ Panel header ═══════════════ */
.tl-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  min-height: 24px;
}

.tl-panel-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.tl-panel-icon--accent  { color: var(--app-accent); }
.tl-panel-icon--success { color: var(--app-success); }
.tl-panel-icon--warning { color: var(--app-warning); }
.tl-panel-icon--mono    { color: #64748b; }
.tl-panel-icon--muted   { color: var(--app-text-regular); }

.tl-panel-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

/* ═══════════════ Count chip ═══════════════ */
.tl-count-chip {
  display: inline-flex;
  align-items: center;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 500;
  background: rgba(22, 93, 255, 0.08);
  color: var(--app-accent);
  border: 1px solid rgba(22, 93, 255, 0.18);
  margin-left: 6px;
}

/* ═══════════════ Inline query form ═══════════════ */
.tools-inline-form {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  flex-wrap: wrap;
}

.tools-inline-form :deep(.el-form-item) {
  margin-bottom: 10px;
}

.form-action-right {
  margin-left: auto;
}

/* ═══════════════ Dig split layout ═══════════════ */
.tools-split-layout {
  display: grid;
  grid-template-columns: 1fr 340px;
  gap: 16px;
}

.tools-output-card :deep(.el-card__body),
.tools-history-card :deep(.el-card__body) {
  padding: 0;
}

/* ═══════════════ Terminal output ═══════════════ */
.tools-terminal {
  min-height: 380px;
  margin: 0;
  border-radius: 0;
  background: #1a1f2e;
  color: #c9d1d9;
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
  line-height: 1.75;
  padding: 16px 18px;
  white-space: pre-wrap;
  word-break: break-all;
}

/* ═══════════════ Query history ═══════════════ */
.history-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
  transition: background 0.12s;
}

.history-item:last-child {
  border-bottom: none;
}

.history-item:hover {
  background: rgba(22, 93, 255, 0.04);
}

.history-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.history-domain {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--app-text-regular);
}

.history-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.tools-pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding: 10px 14px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ DNSSEC stats strip ═══════════════ */
.tl-stats-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.tl-stat-tile {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  background: #fff;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
}

.tl-stat-tile--danger .tl-stat-tile-value {
  color: var(--app-danger);
}

.tl-stat-tile-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

.tl-stat-tile-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--app-title);
  font-variant-numeric: tabular-nums;
}

/* ═══════════════ IP single result grid ═══════════════ */
.tl-ip-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.tl-ip-tile {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 12px 14px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg-secondary);
}

.tl-ip-tile-label {
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--app-text-regular);
}

.tl-ip-tile-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-title);
  word-break: break-all;
}

/* ═══════════════ Error tip ═══════════════ */
.ip-error-tip {
  margin: 0 0 6px;
  font-size: 12px;
  color: var(--app-danger);
}

/* ═══════════════ Tables ═══════════════ */
.tl-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
  color: var(--app-text-secondary);
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.tl-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}

.tl-table :deep(.el-table__body tr.current-row > td.el-table__cell),
.tl-table :deep(.el-table__body tr.el-table__row--selected > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.08) !important;
}

.tools-shell :deep(.tl-row-danger > td.el-table__cell) {
  background: rgba(245, 63, 63, 0.04);
}

/* ═══════════════ Pagination footer ═══════════════ */
.tl-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Mono text ═══════════════ */
.tl-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}

/* ═══════════════ Badges ═══════════════ */
.tl-badge {
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

.tl-badge--neutral {
  color: var(--app-text-secondary);
  background: rgba(78, 89, 105, 0.08);
  border-color: rgba(78, 89, 105, 0.18);
}

.tl-badge--on {
  color: var(--app-success);
  background: rgba(0, 180, 42, 0.08);
  border-color: rgba(0, 180, 42, 0.22);
}

.tl-badge--off {
  color: var(--app-text-regular);
  background: rgba(78, 89, 105, 0.06);
  border-color: rgba(78, 89, 105, 0.15);
}

.tl-badge--danger {
  color: var(--app-danger);
  background: rgba(245, 63, 63, 0.08);
  border-color: rgba(245, 63, 63, 0.22);
}

.tl-badge--warning {
  color: var(--app-warning);
  background: rgba(255, 125, 0, 0.08);
  border-color: rgba(255, 125, 0, 0.22);
}

/* ═══════════════ Node group radio ═══════════════ */
.node-group :deep(.el-radio-button__inner) {
  border-radius: 6px;
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
  grid-column: span 2;
}

/* ═══════════════ Responsive ═══════════════ */
@media (max-width: 1280px) {
  .tools-split-layout {
    grid-template-columns: 1fr;
  }

  .tl-stats-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .tl-ip-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .tl-stats-strip,
  .tl-ip-grid,
  .detail-grid {
    grid-template-columns: 1fr;
  }

  .detail-item-block {
    grid-column: span 1;
  }

  .form-action-right {
    margin-left: 0;
  }
}
</style>
