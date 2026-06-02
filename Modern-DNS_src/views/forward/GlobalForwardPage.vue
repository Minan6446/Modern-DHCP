<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import BaseModal from '../../components/BaseModal.vue'
import { useGlobalForward } from '../../composables/setting/useGlobalForward'
import { useUpstreamHealthChecker } from '../../composables/setting/useUpstreamHealthChecker'
import { confirmRiskAction, validateFormAndFocus } from '../../utils/interaction'
import type { ForwardServer } from '../../types/modules'
import { runLatencyTestApi, getTrafficStatsApi, type TrafficBucket } from '../../api/forward'

const route = useRoute()
const { t } = useI18n()
const globalFormRef = ref<FormInstance>()
const serverFormRef = ref<FormInstance>()

const {
  loadingConfig,
  submittingConfig,
  refreshLoading,
  serverLoading,
  deletingServerId,
  lastUpdated,
  globalForm,
  customDnsInputRef,
  tableData,
  pagination,
  dialogVisible,
  formData,
  isCustomDnsSelected,
  dnsProviderOptions,
  refreshData,
  handlePageChange,
  handlePageSizeChange,
  saveGlobal,
  resetGlobal,
  toggleGlobalEnabled,
  togglePublicDnsEnabled,
  openCreateDialog,
  openEditDialog,
  closeDialog,
  resetServerForm,
  submitServer,
  removeServer,
  fillBuiltInProvider,
  normalizeCustomDns,
  debounceAdjustTimeout,
  debounceAdjustRetry,
  debounceAdjustServerPort,
  debounceAdjustServerPriority,
} = useGlobalForward()

const ipPattern = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
const domainPattern = /^(?=.{1,253}$)(?!-)(?:[a-zA-Z0-9-]{1,63}\.)+[a-zA-Z]{2,63}$/
const serverNamePattern = /^[\u4e00-\u9fa5A-Za-z0-9_]{2,32}$/

const validateIpOrDomain = (_rule: unknown, value: string, callback: (error?: Error) => void): void => {
  if (!value) {
    callback(new Error(t('forward.valIpOrDomain')))
    return
  }
  if (!ipPattern.test(value) && !domainPattern.test(value)) {
    callback(new Error(t('forward.valIpOrDomainInvalid')))
    return
  }
  callback()
}

const validateDnsList = (_rule: unknown, value: string, callback: (error?: Error) => void): void => {
  if (!isCustomDnsSelected.value) {
    callback()
    return
  }
  const items = String(value || '')
    .split(/[\s,\n]+/)
    .map((item) => item.trim())
    .filter(Boolean)

  if (!items.length) {
    callback(new Error(t('forward.valCustomDnsRequired')))
    return
  }
  if (items.some((item) => !ipPattern.test(item))) {
    callback(new Error(t('forward.valCustomDnsInvalid')))
    return
  }
  callback()
}

const globalRules = computed<FormRules>(() => ({
  publicDns: [{ required: true, message: t('forward.valPublicDns'), trigger: 'change' }],
  publicDnsCustom: [{ validator: validateDnsList, trigger: ['blur', 'change'] }],
  timeout: [{ required: true, message: t('forward.valTimeout'), trigger: ['blur', 'change'] }],
  retries: [{ required: true, message: t('forward.valRetries'), trigger: ['blur', 'change'] }],
  strategy: [{ required: true, message: t('forward.valStrategy'), trigger: 'change' }],
}))

const serverRules = computed<FormRules>(() => ({
  name: [
    { required: true, message: t('forward.valServerName'), trigger: ['blur', 'change'] },
    { pattern: serverNamePattern, message: t('forward.valServerNamePattern'), trigger: ['blur', 'change'] },
  ],
  address: [{ validator: validateIpOrDomain, trigger: ['blur', 'change'] }],
  port: [
    { required: true, message: t('forward.valPort'), trigger: ['blur', 'change'] },
    {
      validator: (_rule: unknown, value: number, callback: (error?: Error) => void) => {
        const port = Number(value)
        if (!Number.isInteger(port) || port < 1 || port > 65535) {
          callback(new Error(t('forward.valPortRange')))
          return
        }
        callback()
      },
      trigger: ['blur', 'change'],
    },
  ],
  protocol: [{ required: true, message: t('forward.valProtocol'), trigger: 'change' }],
  priority: [
    { required: true, message: t('forward.valPriority'), trigger: ['blur', 'change'] },
    {
      validator: (_rule: unknown, value: number, callback: (error?: Error) => void) => {
        const priority = Number(value)
        if (!Number.isInteger(priority) || priority < 1 || priority > 100) {
          callback(new Error(t('forward.valPriorityRange')))
          return
        }
        callback()
      },
      trigger: ['blur', 'change'],
    },
  ],
}))

const activeTab = ref('dns')

const statusTagType = (status: string): 'success' | 'info' => (status === '启用' ? 'success' : 'info')

const PROTOCOL_COLORS: Record<string, string> = {
  UDP: '#3b82f6', TCP: '#10b981', DoT: '#8b5cf6', DoH: '#f59e0b',
}
const protocolBg = (p: string) => PROTOCOL_COLORS[p] ?? '#6b7280'

const PROTOCOL_DESC = computed<Record<string, string>>(() => ({
  UDP: t('forward.protocolUdp'), TCP: t('forward.protocolTcp'), DoT: t('forward.protocolDot'), DoH: t('forward.protocolDoh'),
}))

const resolvedIps = computed(() => {
  if (!globalForm.publicDns || globalForm.publicDns === 'custom') {
    return (globalForm.publicDnsCustom || '').split(/[,\s]+/).filter(Boolean)
  }
  const provider = globalForm.providers.find((p) => p.value === globalForm.publicDns)
  return provider?.ips ?? []
})

/* ── Health checker ── */
const {
  checking,
  summary,
  allRecords,
  getRecord,
  runCheck,
  startPolling,
  statusLabel,
  statusColor,
  latencyText,
} = useUpstreamHealthChecker()

const allServers = computed<ForwardServer[]>(() => (tableData.value as unknown as ForwardServer[]))

// Auto-poll every 5 minutes. Public recursive resolvers (114/119/223…)
// don't change reachability minute-to-minute, and a tighter cadence just
// burned probe budget + UI flicker. Operators who want a fresh result
// click "立即检测" — that always runs immediately regardless of timer.
onMounted(() => startPolling(() => allServers.value, 5 * 60_000))

/* ── Traffic stats (real backend data) ── */
const trafficBuckets = ref<TrafficBucket[]>([])
const trafficLoading = ref(false)

const loadTrafficStats = async () => {
  trafficLoading.value = true
  try {
    const { data } = await getTrafficStatsApi()
    trafficBuckets.value = data?.buckets ?? []
  } finally {
    trafficLoading.value = false
  }
}

onMounted(() => { loadTrafficStats() })

const STAT_HOURS = computed(() => trafficBuckets.value.map((b) => b.hour))

const TRAFFIC_SERIES = [
  { name: t('forward.successQueries'), color: '#10b981' },
  { name: t('forward.failQueries'), color: '#F53F3F' },
]

const trafficRows = computed(() => {
  const buckets = trafficBuckets.value
  return TRAFFIC_SERIES.map((s, idx) => {
    const values = buckets.map((b) => (idx === 0 ? b.success : b.fail))
    return { ...s, values, total: values.reduce((a, b) => a + b, 0) }
  })
})
const maxTraffic = computed(() => Math.max(...trafficRows.value.flatMap((r) => r.values), 1))
const barH = (v: number) => `${Math.round((v / maxTraffic.value) * 64)}px`

/* ── DNS latency test ── */
type LatencyEntry = { label: string; ip: string; latencyMs: number | null; status: 'idle' | 'testing' | 'done' | 'fail' }
const latencyTesting = ref(false)
const latencyResults = ref<LatencyEntry[]>([])

const buildLatencyTargets = (): LatencyEntry[] => {
  const targets: LatencyEntry[] = []
  for (const p of globalForm.providers) {
    if (p.value === 'custom') continue
    for (const ip of (p.ips ?? [])) {
      targets.push({ label: p.label, ip, latencyMs: null, status: 'idle' })
    }
  }
  return targets
}

const runLatencyTest = async (): Promise<void> => {
  if (latencyTesting.value) return
  latencyTesting.value = true
  latencyResults.value = buildLatencyTargets().map((e) => ({ ...e, status: 'testing' as const }))
  try {
    const targets = latencyResults.value.map((e) => ({ label: e.label, ip: e.ip }))
    const { data } = await runLatencyTestApi(targets)
    latencyResults.value = (data as any).results.map((r: any) => ({
      label: r.label,
      ip: r.ip,
      latencyMs: r.latencyMs,
      status: r.status,
    }))
    latencyResults.value.sort((a, b) => {
      if (a.latencyMs === null) return 1
      if (b.latencyMs === null) return -1
      return a.latencyMs! - b.latencyMs!
    })
    ElMessage.success(t('forward.latencyDone'))
  } catch {
    ElMessage.error(t('common.loadFailed'))
  } finally {
    latencyTesting.value = false
  }
}

const latencyColor = (ms: number | null): string => {
  if (ms === null) return 'var(--app-danger)'
  if (ms < 50) return 'var(--app-success)'
  if (ms < 100) return '#f59e0b'
  return 'var(--app-danger)'
}

const handleOpenServerDialog = (row?: ForwardServer): void => {
  if (row) {
    openEditDialog(row)
  } else {
    openCreateDialog()
  }
  serverFormRef.value?.clearValidate()
}

const handleSaveGlobal = async (): Promise<void> => {
  const valid = await validateFormAndFocus(globalFormRef.value)
  if (!valid) {
    return
  }
  if (globalForm.timeout < 1 || globalForm.timeout > 60) {
    ElMessage.warning(t('forward.timeoutWarn'))
    return
  }
  if (globalForm.retries < 1 || globalForm.retries > 10) {
    ElMessage.warning(t('forward.retriesWarn'))
    return
  }

  try {
    await confirmRiskAction({
      title: t('forward.saveConfirmTitle'),
      action: t('forward.saveConfirmAction'),
      risk: t('forward.saveConfirmRisk'),
    })
    await saveGlobal()
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('forward.saveFailed'))
    }
  }
}

const handleResetGlobal = async (): Promise<void> => {
  try {
    await confirmRiskAction({
      title: t('forward.resetConfirmTitle'),
      action: t('forward.resetConfirmAction'),
      risk: t('forward.resetConfirmRisk'),
    })
    await resetGlobal()
    globalFormRef.value?.clearValidate()
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('forward.resetFailed'))
    }
  }
}

const handleSubmitServer = async (): Promise<void> => {
  const valid = await validateFormAndFocus(serverFormRef.value)
  if (!valid) {
    return
  }
  try {
    await submitServer()
  } catch (_error) {
    ElMessage.error(t('forward.serverSaveFailed'))
  }
}

const handleDeleteServer = async (row: ForwardServer): Promise<void> => {
  try {
    await confirmRiskAction({
      title: t('forward.deleteConfirmTitle'),
      action: t('forward.deleteConfirmAction'),
      target: row.name,
      risk: t('forward.deleteConfirmRisk'),
    })
    await removeServer(row)
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error(t('forward.deleteFailed'))
    }
  }
}

const handleGlobalEnabledChange = async (value: string | number | boolean): Promise<void> => {
  try {
    await toggleGlobalEnabled(Boolean(value))
  } catch (_error) {
    globalForm.enabled = !Boolean(value)
    ElMessage.error(t('forward.globalEnabledFailed'))
  }
}

const handlePublicDnsEnabledChange = async (value: string | number | boolean): Promise<void> => {
  try {
    await togglePublicDnsEnabled(Boolean(value))
  } catch (_error) {
    globalForm.publicDnsEnabled = !Boolean(value)
    ElMessage.error(t('forward.publicDnsEnabledFailed'))
  }
}
</script>

<template>
  <div class="page-shell forward-global-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('menu.' + route.meta.titleKey) }}</h1>
        <p class="page-subtitle">{{ $t('forward.globalForwardSubtitle') }}</p>
      </div>
      <div class="forward-header-right">
        <div class="fwd-global-switch">
          <span class="fwd-switch-label">{{ t('forward.globalSwitch') }}</span>
          <el-switch
            :model-value="globalForm.enabled"
            inline-prompt :active-text="t('forward.switchOn')" :inactive-text="t('forward.switchOff')"
            style="--el-switch-on-color:var(--app-success)"
            @change="handleGlobalEnabledChange"
          />
          <span :class="['mn-badge', globalForm.enabled ? 'mn-badge--success' : 'mn-badge--neutral']" style="font-size:10px">
            {{ globalForm.enabled ? t('forward.enabled') : t('forward.disabled') }}
          </span>
        </div>
        <span class="forward-last-updated">{{ t('forward.refreshedAt') }}{{ lastUpdated || t('forward.loading') }}</span>
      </div>
    </div>

    <el-form ref="globalFormRef" :model="globalForm" :rules="globalRules" label-width="120px" label-position="left" v-loading="loadingConfig || submittingConfig">
      <el-card class="forward-tabs-card">
        <el-tabs v-model="activeTab" class="forward-tabs">

          <!-- Tab 1: 公共 DNS -->
          <el-tab-pane name="dns">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
                {{ t('forward.publicDnsTab') }}
              </span>
            </template>
            <div class="mn-tab-body">
              <div class="forward-dns-grid">
                <div class="dns-left">
                  <el-form-item :label="t('forward.publicDnsLabel')" prop="publicDns" style="margin-bottom:12px">
                    <el-select v-model="globalForm.publicDns" style="width:100%" @change="fillBuiltInProvider">
                      <el-option v-for="item in dnsProviderOptions" :key="item.value" :label="item.label" :value="item.value" />
                    </el-select>
                  </el-form-item>
                  <div v-if="resolvedIps.length" class="dns-ip-chips">
                    <span v-for="ip in resolvedIps" :key="ip" class="dns-ip-chip">{{ ip }}</span>
                  </div>
                  <div class="dns-switch-inline" style="margin-top:14px">
                    <el-switch
                      :model-value="globalForm.publicDnsEnabled"
                      inline-prompt :active-text="t('forward.switchOn')" :inactive-text="t('forward.switchOff')"
                      style="--el-switch-on-color:var(--app-success)"
                      @change="handlePublicDnsEnabledChange"
                    />
                    <span class="switch-label">{{ t('forward.enablePublicDns') }}</span>
                    <span :class="['mn-badge', globalForm.publicDnsEnabled ? 'mn-badge--success' : 'mn-badge--neutral']" style="font-size:10px">
                      {{ globalForm.publicDnsEnabled ? t('forward.publicDnsOn') : t('forward.publicDnsOff') }}
                    </span>
                  </div>
                </div>
                <el-form-item :label="t('forward.customDnsLabel')" prop="publicDnsCustom" style="margin-bottom:0">
                  <el-input
                    ref="customDnsInputRef"
                    v-model="globalForm.publicDnsCustom"
                    type="textarea" :rows="5"
                    :disabled="!isCustomDnsSelected"
                    :placeholder="t('forward.customDnsPlaceholder')"
                    @blur="normalizeCustomDns"
                  />
                  <div v-if="!isCustomDnsSelected" class="dns-custom-hint">{{ t('forward.customDnsHint') }}</div>
                </el-form-item>
              </div>
            </div>
            <div class="tab-action-bar">
              <span class="mn-action-desc">{{ t('forward.saveConfigDesc') }}</span>
              <div class="mn-action-buttons">
                <el-button type="primary" :loading="submittingConfig" :disabled="submittingConfig" @click="handleSaveGlobal">{{ t('forward.saveConfig') }}</el-button>
                <el-button :disabled="submittingConfig" @click="handleResetGlobal">{{ t('forward.resetConfig') }}</el-button>
              </div>
            </div>
          </el-tab-pane>

          <!-- Tab 2: 上游服务器 -->
          <el-tab-pane name="servers">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
                {{ t('forward.upstreamTab') }}
              </span>
            </template>
            <div class="mn-tab-toolbar">
              <el-button :loading="refreshLoading" :disabled="refreshLoading || submittingConfig" @click="refreshData">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
                {{ t('forward.refresh') }}
              </el-button>
              <el-button type="primary" @click="handleOpenServerDialog()">
                <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
                {{ t('forward.addUpstream') }}
              </el-button>
            </div>
            <el-table class="mn-table" :data="tableData" v-loading="serverLoading" stripe table-layout="fixed">
              <el-table-column prop="name" :label="t('forward.serverName')" width="180" sortable />
              <el-table-column prop="address" :label="t('forward.serverAddress')" min-width="220" sortable show-overflow-tooltip>
                <template #default="{ row }"><span class="mn-mono">{{ row.address }}</span></template>
              </el-table-column>
              <el-table-column prop="port" :label="t('forward.port')" width="90" sortable>
                <template #default="{ row }"><span class="mn-mono">{{ row.port }}</span></template>
              </el-table-column>
              <el-table-column prop="protocol" :label="t('forward.protocol')" width="90" sortable align="center">
                <template #default="{ row }">
                  <span class="fwd-proto-badge" :style="{ background: protocolBg(row.protocol) }">{{ row.protocol }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="priority" :label="t('forward.priorityLabel')" width="140" sortable>
                <template #default="{ row }">
                  <div class="fwd-priority-cell">
                    <span class="mn-mono fwd-priority-num">{{ row.priority }}</span>
                    <div class="fwd-priority-bar-wrap">
                      <div class="fwd-priority-bar" :style="{ width: `${Math.min(100,(1-(row.priority-1)/99)*100)}%` }"></div>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('forward.statusLabel')" width="100" align="center">
                <template #default="{ row }">
                  <span :class="['mn-badge', row.status === '启用' ? 'mn-badge--success' : 'mn-badge--neutral']">{{ row.status === '启用' ? t('forward.enabled') : t('forward.disabled') }}</span>
                </template>
              </el-table-column>
              <el-table-column :label="t('forward.operation')" width="140" align="right">
                <template #default="{ row }">
                  <div class="mn-row-ops">
                    <el-button link type="primary" @click="handleOpenServerDialog(row)">{{ t('forward.edit') }}</el-button>
                    <el-button link type="danger" :loading="deletingServerId === row.id" :disabled="Boolean(deletingServerId) || submittingConfig" @click="handleDeleteServer(row)">{{ t('forward.delete') }}</el-button>
                  </div>
                </template>
              </el-table-column>
              <template #empty><el-empty :description="t('forward.noUpstream')" /></template>
            </el-table>
            <div class="mn-pagination">
              <el-pagination
                :current-page="pagination.currentPage"
                :page-size="pagination.pageSize"
                size="small"
                background
                layout="total, sizes, prev, pager, next"
                :page-sizes="[10, 20, 50]"
                :total="pagination.total"
                @current-change="handlePageChange"
                @size-change="handlePageSizeChange"
              />
            </div>
          </el-tab-pane>

          <!-- Tab 3: 全局参数 -->
          <el-tab-pane name="params">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M4.93 4.93a10 10 0 0 0 0 14.14"/></svg>
                {{ t('forward.globalParamsTab') }}
              </span>
            </template>
            <div class="mn-tab-body">
              <div class="forward-params-grid">
                <div class="param-card">
                  <div class="param-card-label">{{ t('forward.timeoutLabel') }}</div>
                  <div class="param-card-body">
                    <el-form-item prop="timeout" style="margin-bottom:0">
                      <div class="soa-num-wrap">
                        <el-input-number v-model="globalForm.timeout" :min="1" :max="60" controls-position="right" style="width:100%" @change="debounceAdjustTimeout" />
                        <span class="soa-unit">{{ t('forward.second') }}</span>
                      </div>
                    </el-form-item>
                  </div>
                  <div class="param-card-desc">{{ t('forward.timeoutDesc') }}</div>
                </div>
                <div class="param-card">
                  <div class="param-card-label">{{ t('forward.retriesLabel') }}</div>
                  <div class="param-card-body">
                    <el-form-item prop="retries" style="margin-bottom:0">
                      <div class="soa-num-wrap">
                        <el-input-number v-model="globalForm.retries" :min="1" :max="10" controls-position="right" style="width:100%" @change="debounceAdjustRetry" />
                        <span class="soa-unit">{{ t('forward.times') }}</span>
                      </div>
                    </el-form-item>
                  </div>
                  <div class="param-card-desc">{{ t('forward.retriesDesc') }}</div>
                </div>
                <div class="param-card">
                  <div class="param-card-label">{{ t('forward.strategyLabel') }}</div>
                  <div class="param-card-body">
                    <el-form-item prop="strategy" style="margin-bottom:0">
                      <el-select v-model="globalForm.strategy" style="width:100%">
                        <el-option :label="t('forward.strategyPriority')" value="priority" />
                        <el-option :label="t('forward.strategyRoundRobin')" value="round-robin" />
                        <el-option :label="t('forward.strategyRandom')" value="random" />
                      </el-select>
                    </el-form-item>
                  </div>
                  <div class="param-card-desc">{{ t('forward.strategyDesc') }}</div>
                </div>
              </div>
            </div>
            <div class="tab-action-bar">
              <span class="mn-action-desc">{{ t('forward.saveParamsDesc') }}</span>
              <div class="mn-action-buttons">
                <el-button type="primary" :loading="submittingConfig" :disabled="submittingConfig" @click="handleSaveGlobal">{{ t('forward.saveParams') }}</el-button>
                <el-button :disabled="submittingConfig" @click="handleResetGlobal">{{ t('forward.resetConfig') }}</el-button>
              </div>
            </div>
          </el-tab-pane>

          <!-- Tab 4: 健康检测 -->
          <el-tab-pane name="health">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
                {{ t('forward.healthTab') }}
              </span>
            </template>
            <div class="mn-tab-toolbar">
              <div class="health-summary-strip">
                <span class="health-kpi health-kpi--ok">{{ t('forward.online') }} {{ summary.online }}</span>
                <span class="health-kpi health-kpi--warn">{{ t('forward.slowResp') }} {{ summary.slow }}</span>
                <span class="health-kpi health-kpi--err">{{ t('forward.timeout') }} {{ summary.timeout }}</span>
                <span class="health-kpi">{{ t('forward.avgLatency') }} {{ summary.avgLatency !== null ? `${summary.avgLatency} ms` : '--' }}</span>
              </div>
              <el-button :loading="checking" @click="runCheck(allServers)">{{ t('forward.checkNow') }}</el-button>
            </div>
            <div v-if="!allServers.length" style="padding:32px 0">
              <el-empty :description="t('forward.noUpstreamHint')" />
            </div>
            <div v-else class="health-grid">
              <div v-for="row in allServers" :key="row.id" class="health-card">
                <div class="health-card-header">
                  <span class="health-dot" :style="{ background: statusColor(row.id) }"></span>
                  <span class="health-card-name">{{ row.name }}</span>
                  <span class="fwd-proto-badge" :style="{ background: protocolBg(row.protocol) }">{{ row.protocol }}</span>
                </div>
                <div class="health-card-addr mn-mono">{{ row.address }}:{{ row.port }}</div>
                <div class="health-card-meta">
                  <span class="health-status-label" :style="{ color: statusColor(row.id) }">{{ statusLabel(row.id) }}</span>
                  <span class="health-latency" :style="{ color: statusColor(row.id) }">{{ latencyText(row.id) }}</span>
                </div>
                <div v-if="getRecord(row.id).checkedAt" class="health-card-time">{{ t('forward.checkedAt') }}{{ getRecord(row.id).checkedAt }}</div>
                <div v-if="getRecord(row.id).errorCount > 0" class="health-card-err">{{ t('forward.failCount', { count: getRecord(row.id).errorCount }) }}</div>
              </div>
            </div>
          </el-tab-pane>

          <!-- Tab 5: 流量统计 -->
          <el-tab-pane name="stats">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
                {{ t('forward.trafficTab') }}
              </span>
            </template>
            <div class="mn-tab-body">
              <div class="stats-legend">
                <span v-for="s in TRAFFIC_SERIES" :key="s.name" class="stats-legend-item">
                  <span class="stats-legend-dot" :style="{ background: s.color }"></span>{{ s.name }}
                </span>
              </div>
              <div class="stats-chart">
                <div v-for="(hour, hi) in STAT_HOURS" :key="hour" class="stats-col">
                  <div class="stats-bars">
                    <div
                      v-for="row in trafficRows" :key="row.name"
                      class="stats-bar"
                      :style="{ height: barH(row.values[hi]), background: row.color }"
                      :title="`${row.name} ${hour}: ${row.values[hi]} qps`"
                    ></div>
                  </div>
                  <div class="stats-hour">{{ hi % 4 === 0 ? hour : '' }}</div>
                </div>
              </div>
              <div class="stats-table">
                <div class="stats-table-head">
                  <span>{{ t('forward.upstream') }}</span><span>{{ t('forward.totalReqs') }}</span><span>{{ t('forward.share') }}</span>
                </div>
                <div v-for="row in trafficRows" :key="row.name" class="stats-table-row">
                  <span class="stats-name"><span class="stats-dot" :style="{ background: row.color }"></span>{{ row.name }}</span>
                  <span class="mn-mono">{{ row.total.toLocaleString() }}</span>
                  <div class="stats-share-wrap">
                    <div class="stats-share-bar" :style="{ width: `${Math.round(row.total / (trafficRows.reduce((a,b)=>a+b.total,0)||1)*100)}%`, background: row.color }"></div>
                    <span class="stats-share-pct">{{ Math.round(row.total / (trafficRows.reduce((a,b)=>a+b.total,0)||1)*100) }}%</span>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- Tab 6: DNS 延迟测试 -->
          <el-tab-pane name="latency">
            <template #label>
              <span class="mn-tab-label">
                <svg class="mn-tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                {{ t('forward.latencyTab') }}
              </span>
            </template>
            <div class="mn-tab-body">
              <div class="latency-header">
                <p class="latency-desc">{{ t('forward.latencyDesc') }}</p>
                <el-button type="primary" :loading="latencyTesting" @click="runLatencyTest">
                  {{ latencyTesting ? t('forward.testing') : t('forward.startTest') }}
                </el-button>
              </div>
              <div v-if="!latencyResults.length" class="latency-empty">
                <el-empty :description="t('forward.latencyEmpty')" />
              </div>
              <div v-else class="latency-list">
                <div v-for="(entry, i) in latencyResults" :key="entry.ip" class="latency-row">
                  <span class="latency-rank">{{ i + 1 }}</span>
                  <span class="latency-ip mn-mono">{{ entry.ip }}</span>
                  <span class="latency-provider">{{ entry.label }}</span>
                  <div class="latency-bar-wrap">
                    <div
                      v-if="entry.status === 'done'"
                      class="latency-bar"
                      :style="{ width: `${Math.min(100, Math.round((entry.latencyMs ?? 0) / 130 * 100))}%`, background: latencyColor(entry.latencyMs) }"
                    ></div>
                    <div v-else-if="entry.status === 'testing'" class="latency-bar latency-bar--pulse" style="width:40%"></div>
                  </div>
                  <span class="latency-val" :style="{ color: latencyColor(entry.latencyMs) }">
                    <template v-if="entry.status === 'idle'">--</template>
                    <template v-else-if="entry.status === 'testing'">…</template>
                    <template v-else-if="entry.status === 'fail'">{{ t('forward.latencyTimeout') }}</template>
                    <template v-else>{{ entry.latencyMs }} ms</template>
                  </span>
                </div>
              </div>
            </div>
          </el-tab-pane>

        </el-tabs>
      </el-card>
    </el-form>

    <BaseModal
      v-model="dialogVisible"
      :title="formData.id ? t('forward.editUpstream') : t('forward.addUpstream')"
      :width="480"
      :loading="submittingConfig"
      :confirm-disabled="submittingConfig"
      :confirm-text="t('forward.save')"
      :cancel-text="t('forward.cancel')"
      @confirm="handleSubmitServer"
      @cancel="closeDialog"
      @close="closeDialog"
      @closed="resetServerForm"
    >
      <el-form ref="serverFormRef" :model="formData" :rules="serverRules" label-position="top" class="forward-server-modal-form">
        <div class="smodal-grid">
          <el-form-item :label="t('forward.serverName')" prop="name">
            <el-input v-model="formData.name" maxlength="32" show-word-limit :placeholder="t('forward.serverNamePlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('forward.serverAddress')" prop="address">
            <el-input v-model="formData.address" :placeholder="t('forward.serverAddrPlaceholder')" />
          </el-form-item>
        </div>
        <div class="smodal-grid">
          <el-form-item :label="t('forward.port')" prop="port">
            <el-input-number v-model="formData.port" :min="1" :max="65535" controls-position="right" style="width:100%" @change="debounceAdjustServerPort" />
          </el-form-item>
          <el-form-item :label="t('forward.protocol')" prop="protocol">
            <el-select v-model="formData.protocol" style="width:100%">
              <el-option v-for="p in ['UDP','TCP','DoT','DoH']" :key="p" :label="p" :value="p">
                <div class="smodal-proto-option">
                  <span class="fwd-proto-badge" :style="{ background: protocolBg(p) }">{{ p }}</span>
                  <span class="smodal-proto-desc">{{ PROTOCOL_DESC[p] }}</span>
                </div>
              </el-option>
            </el-select>
          </el-form-item>
        </div>
        <div class="smodal-grid">
          <el-form-item :label="t('forward.priorityLabel')" prop="priority">
            <div class="soa-num-wrap">
              <el-input-number v-model="formData.priority" :min="1" :max="100" controls-position="right" style="width:100%" @change="debounceAdjustServerPriority" />
            </div>
            <div class="fwd-priority-preview">
              <div class="fwd-priority-bar-wrap" style="flex:1">
                <div class="fwd-priority-bar" :style="{ width: `${Math.min(100,(1-(Number(formData.priority||1)-1)/99)*100)}%` }"></div>
              </div>
              <span class="priority-tip">{{ t('forward.priorityTip') }}</span>
            </div>
          </el-form-item>
          <el-form-item :label="t('forward.enableStatusLabel')">
            <div class="rdf-status-wrap" style="height:32px">
              <el-switch
                v-model="formData.status"
                active-value="启用" inactive-value="禁用"
                inline-prompt :active-text="t('forward.switchOn')" :inactive-text="t('forward.switchOff')"
                style="--el-switch-on-color:var(--app-success)"
              />
              <span class="rdf-status-label" :class="formData.status === '启用' ? 'rdf-status-on' : 'rdf-status-off'">
                {{ formData.status === '启用' ? t('forward.joinPool') : t('forward.notJoinPool') }}
              </span>
            </div>
          </el-form-item>
        </div>
      </el-form>
    </BaseModal>
  </div>
</template>

<style scoped>
/* ═══════════════ Page ═══════════════ */
.forward-global-shell {
  gap: 16px;
}

.forward-last-updated {
  font-size: 12px;
  color: var(--app-text-regular);
}

.forward-global-form {
  gap: 16px;
}

/* ═══════════════ Cards ═══════════════ */
.forward-tabs-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.forward-tabs-card :deep(.el-card__body) {
  padding: 0;
}

.forward-card {
  border: 1px solid var(--app-border);
  border-radius: 10px;
  box-shadow: var(--app-shadow-soft);
  overflow: hidden;
}

.forward-action-card :deep(.el-card__body) {
  padding: 12px 16px;
}

/* ═══════════════ Tabs ═══════════════ */
.forward-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 16px;
  background: var(--app-bg-secondary);
  border-bottom: 1px solid var(--app-border);
}

.forward-tabs :deep(.el-tabs__item) {
  font-size: 13px;
  color: var(--app-text-regular);
  height: 44px;
  line-height: 44px;
}

.forward-tabs :deep(.el-tabs__item.is-active) {
  color: var(--app-accent);
  font-weight: 600;
}

.forward-tabs :deep(.el-tabs__active-bar) {
  background: var(--app-accent);
  height: 2px;
}

.forward-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.forward-tabs :deep(.el-tabs__content) {
  padding: 0;
}

.mn-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.mn-tab-icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}

.mn-tab-body {
  padding: 20px;
}

.mn-tab-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

/* ═══════════════ Form layouts ═══════════════ */
.forward-global-form :deep(.el-form-item__label) {
  justify-content: flex-start;
  text-align: left;
}

.forward-global-form :deep(.el-form-item.is-required .el-form-item__label::before) {
  color: var(--app-danger);
}

.forward-dns-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
  align-items: start;
}

.dns-left { display: flex; flex-direction: column; }

.dns-custom-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.dns-ip-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.dns-ip-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-family: var(--app-font-mono, monospace);
  background: rgba(22, 93, 255, 0.07);
  color: var(--app-accent);
  border: 1px solid rgba(22, 93, 255, 0.18);
}

.dns-switch-inline {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.switch-label,
.priority-tip {
  font-size: 12px;
  color: var(--app-text-regular);
}

.forward-params-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.param-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg);
}

.param-card-label {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
}

.param-card-body { flex: 1; }

.param-card-desc {
  font-size: 11px;
  color: var(--app-text-secondary);
  line-height: 1.5;
}

.tab-action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}

.fwd-proto-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.04em;
  white-space: nowrap;
}

.fwd-priority-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.fwd-priority-num { min-width: 24px; }

.fwd-priority-bar-wrap {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--app-border);
  overflow: hidden;
}

.fwd-priority-bar {
  height: 100%;
  border-radius: 2px;
  background: var(--app-accent);
  transition: width 0.3s;
}

.fwd-priority-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.smodal-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
}

.smodal-proto-option {
  display: flex;
  align-items: center;
  gap: 8px;
}

.smodal-proto-desc {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.soa-num-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
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
  gap: 8px;
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

/* ═══════════════ Mono ═══════════════ */
.mn-mono {
  font-family: var(--app-font-mono, 'JetBrains Mono', 'Consolas', monospace);
  font-size: 12px;
}

/* ═══════════════ Action bar ═══════════════ */
.mn-action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.mn-action-desc {
  font-size: 12px;
  color: var(--app-text-regular);
}

.mn-action-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
}

.soa-unit {
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
  flex-shrink: 0;
}

.rdf-status-wrap { display: flex; align-items: center; gap: 10px; }
.rdf-status-label { font-size: 12px; }
.rdf-status-on { color: var(--app-success); }
.rdf-status-off { color: var(--app-text-secondary); }

.forward-header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.fwd-global-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  background: var(--app-bg);
}

.fwd-switch-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--app-text-secondary);
}

@media (max-width: 1200px) {
  .forward-dns-grid,
  .forward-params-grid {
    grid-template-columns: 1fr;
  }

  .smodal-grid {
    grid-template-columns: 1fr;
  }

  .mn-action-bar,
  .tab-action-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}

/* ═══════════════ Health check ═══════════════ */
.health-summary-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.health-kpi {
  padding: 3px 10px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 500;
  background: rgba(78, 89, 105, 0.08);
  border: 1px solid rgba(78, 89, 105, 0.18);
  color: var(--app-text-secondary);
}

.health-kpi--ok { background: rgba(0,180,42,0.08); border-color: rgba(0,180,42,0.22); color: var(--app-success); }
.health-kpi--warn { background: rgba(245,158,11,0.08); border-color: rgba(245,158,11,0.22); color: #f59e0b; }
.health-kpi--err { background: rgba(245,63,63,0.08); border-color: rgba(245,63,63,0.22); color: var(--app-danger); }

.health-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
  padding: 16px;
}

.health-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-bg);
  transition: box-shadow 0.2s;
}

.health-card:hover { box-shadow: var(--app-shadow-soft); }

.health-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.health-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  transition: background 0.3s;
}

.health-card-name {
  font-size: 13px;
  font-weight: 600;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.health-card-addr {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.health-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 4px;
}

.health-status-label { font-size: 12px; font-weight: 500; }
.health-latency { font-size: 13px; font-weight: 700; font-family: var(--app-font-mono, monospace); }

.health-card-time {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.health-card-err {
  font-size: 11px;
  color: var(--app-danger);
}

/* ═══════════════ Traffic stats ═══════════════ */
.stats-legend {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.stats-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
}

.stats-legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  flex-shrink: 0;
}

.stats-chart {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 96px;
  padding-bottom: 20px;
  border-bottom: 1px solid var(--app-border);
  margin-bottom: 20px;
  overflow-x: auto;
}

.stats-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  flex: 1;
  min-width: 20px;
}

.stats-bars {
  display: flex;
  align-items: flex-end;
  gap: 1px;
}

.stats-bar {
  width: 4px;
  border-radius: 2px 2px 0 0;
  transition: height 0.4s;
  cursor: pointer;
  opacity: 0.85;
}

.stats-bar:hover { opacity: 1; }

.stats-hour {
  font-size: 10px;
  color: var(--app-text-secondary);
  white-space: nowrap;
  height: 14px;
}

.stats-table { display: flex; flex-direction: column; gap: 8px; }

.stats-table-head {
  display: grid;
  grid-template-columns: 120px 100px 1fr;
  gap: 12px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border);
}

.stats-table-row {
  display: grid;
  grid-template-columns: 120px 100px 1fr;
  gap: 12px;
  align-items: center;
  font-size: 13px;
}

.stats-name {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stats-dot {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  flex-shrink: 0;
}

.stats-share-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stats-share-bar {
  height: 6px;
  border-radius: 3px;
  max-width: 160px;
  min-width: 4px;
  transition: width 0.4s;
}

.stats-share-pct {
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
}

/* ═══════════════ Latency test ═══════════════ */
.latency-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.latency-desc {
  font-size: 13px;
  color: var(--app-text-secondary);
  margin: 0;
}

.latency-empty { padding: 24px 0; }

.latency-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.latency-row {
  display: grid;
  grid-template-columns: 28px 120px 120px 1fr 64px;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--app-border);
  background: var(--app-bg);
  font-size: 13px;
}

.latency-row:hover { background: var(--app-bg-secondary); }

.latency-rank {
  font-size: 11px;
  font-weight: 700;
  color: var(--app-text-secondary);
  text-align: center;
}

.latency-ip { font-size: 12px; }
.latency-provider { font-size: 12px; color: var(--app-text-secondary); }

.latency-bar-wrap {
  height: 6px;
  border-radius: 3px;
  background: var(--app-border);
  overflow: hidden;
}

.latency-bar {
  height: 100%;
  border-radius: 3px;
  transition: width 0.5s;
}

.latency-bar--pulse {
  background: var(--app-accent);
  animation: latency-pulse 1s ease-in-out infinite;
}

@keyframes latency-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.latency-val {
  font-size: 12px;
  font-weight: 700;
  font-family: var(--app-font-mono, monospace);
  text-align: right;
}

@media (max-width: 1200px) {
  .forward-dns-grid,
  .forward-params-grid {
    grid-template-columns: 1fr;
  }

  .smodal-grid {
    grid-template-columns: 1fr;
  }

  .mn-action-bar,
  .tab-action-bar {
    flex-direction: column;
    align-items: flex-start;
  }

  .latency-row {
    grid-template-columns: 28px 1fr 1fr;
    grid-template-rows: auto auto;
  }

  .latency-bar-wrap,
  .latency-val {
    grid-column: span 2;
  }
}
</style>