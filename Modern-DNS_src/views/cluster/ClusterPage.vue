<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseModal from '@/components/BaseModal.vue'
import BaseChart from '@/components/BaseChart.vue'
import ClusterControlPanel from './ClusterControlPanel.vue'
import {
  listClusterNodesApi,
  addClusterNodeApi,
  removeClusterNodeApi,
  refreshClusterNodesApi,
  failoverClusterNodeApi,
  drainNodeApi,
  batchRemoveNodesApi,
  getNodeMetricsApi,
  getClusterStateApi,
  openClusterEventStream,
  type ClusterNode,
  type NodeMetricSample,
} from '@/api/cluster'

const { t } = useI18n()

// LATEST_VERSION used to be a hardcoded literal — now we resolve it from
// the live primary's reported version so an upgrade rollout shows up
// automatically without a frontend redeploy. Defaults to "—" until the
// first /cluster/state response lands.
const LATEST_VERSION = ref('—')
const ZONE_OPTIONS = ['华东-1', '华东-2', '华北-1', '华南-1', '西南-1', '华中-1']

// clusterReady — bootstrap flag pulled from /cluster/state. Used to gate
// "Add Node" + the table content; uninitialized clusters get a CTA card
// pointing at the cluster control panel instead of an empty table.
const clusterReady = ref<boolean>(false)

const loading        = ref(false)
const nodes          = ref<ClusterNode[]>([])
const refreshLoading = ref(false)
const submitting     = ref(false)
const addDialog      = ref(false)
const detailNode     = ref<ClusterNode | null>(null)
const detailVisible  = ref(false)

const filterKeyword = ref('')
const filterRole    = ref('')
const filterStatus  = ref('')

const form = reactive({ name: '', ip: '', port: 53, role: '从节点', zone: '' })

type RoleKey = 'master' | 'slave' | 'arbiter' | 'unknown'
type StatusKey = 'online' | 'offline' | 'syncing' | 'degraded' | 'unknown'

const roleKey = (role: string): RoleKey => {
  const normalized = String(role || '').trim().toLowerCase()
  if (normalized === '主节点' || normalized === 'master') return 'master'
  if (normalized === '从节点' || normalized === 'slave') return 'slave'
  if (normalized === '仲裁节点' || normalized === 'arbiter') return 'arbiter'
  return 'unknown'
}

const statusKey = (status: string): StatusKey => {
  const normalized = String(status || '').trim().toLowerCase()
  if (normalized === '在线' || normalized === 'online') return 'online'
  if (normalized === '离线' || normalized === 'offline') return 'offline'
  if (normalized === '同步中' || normalized === 'syncing') return 'syncing'
  if (normalized === '降级' || normalized === 'degraded') return 'degraded'
  return 'unknown'
}

const roleLabel = (role: string): string => {
  const key = roleKey(role)
  if (key === 'master') return t('cluster.masterNode')
  if (key === 'slave') return t('cluster.slaveNode')
  if (key === 'arbiter') return t('cluster.arbiterNode')
  return role || t('common.unknown')
}

const statusLabel = (status: string): string => {
  const key = statusKey(status)
  if (key === 'online') return t('cluster.online')
  if (key === 'offline') return t('cluster.offline')
  if (key === 'syncing') return t('cluster.syncing')
  if (key === 'degraded') return t('cluster.degraded')
  return status || t('common.unknown')
}

const filtered = computed(() => nodes.value.filter(n =>
  (!filterKeyword.value || n.name.includes(filterKeyword.value) || n.ip.includes(filterKeyword.value)) &&
  (!filterRole.value   || roleKey(n.role) === filterRole.value) &&
  (!filterStatus.value || statusKey(n.status) === filterStatus.value)
))

const onlineCount  = computed(() => nodes.value.filter(n => statusKey(n.status) === 'online').length)
const offlineCount = computed(() => nodes.value.filter(n => statusKey(n.status) === 'offline').length)
const totalQps     = computed(() => nodes.value.reduce((s, n) => s + n.qps, 0))
const syncIssues   = computed(() => nodes.value.filter(n => n.syncLag > 100).length)

const statusClass = (s: string) => {
  const key = statusKey(s)
  return key === 'online'
    ? 'mn-badge--success'
    : key === 'offline'
      ? 'mn-badge--danger'
      : key === 'syncing'
        ? 'mn-badge--warning'
        : 'mn-badge--neutral'
}
const roleColorByKey: Record<RoleKey, string> = {
  master: '#F53F3F',
  slave: '#1677FF',
  arbiter: '#722ED1',
  unknown: '#86909C',
}
const usageColor = (v: number) => v >= 80 ? 'var(--app-danger)' : v >= 60 ? 'var(--app-warning)' : 'var(--app-success)'
const isOutdated = (v: string) => LATEST_VERSION.value !== '—' && v !== LATEST_VERSION.value

// `0001-01-01T00:00:00Z` is Go's zero time. The backend marshals it
// verbatim when a node has never reported a heartbeat, so we treat it
// the same as the explicit "—" sentinel here. Without this guard the
// drawer renders the literal `0001-01-01T00:00:00Z` (image 4).
const isZeroTime = (ts: string): boolean => !ts || ts === '—' || ts.startsWith('0001-01-01')

const relativeTime = (ts: string) => {
  if (isZeroTime(ts)) return '—'
  const diff = Math.floor((Date.now() - new Date(ts).getTime()) / 1000)
  if (diff < 0)     return formatAbsoluteTime(ts) // future timestamp — fall back to absolute
  if (diff < 60)    return `${diff}s`
  if (diff < 3600)  return `${Math.floor(diff / 60)}m`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`
  return `${Math.floor(diff / 86400)}d`
}

// formatAbsoluteTime trims RFC3339 to "YYYY-MM-DD HH:mm:ss" so the
// detail drawer can show a readable timestamp without timezone noise.
// Used by the side drawer for joinedAt + lastHeartbeat.
const formatAbsoluteTime = (ts: string): string => {
  if (isZeroTime(ts)) return '—'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ts
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

const rowClassName = ({ row }: { row: ClusterNode }) =>
  statusKey(row.status) === 'offline' ? 'row-offline' : ''

const openDetail = (node: ClusterNode) => { detailNode.value = { ...node }; detailVisible.value = true }

const loadNodes = async () => {
  loading.value = true
  try {
    // Cluster state piggybacks on every list refresh: it gives us the
    // bootstrap flag (clusterReady) and the live primary's version which
    // we surface as LATEST_VERSION for the upgrade-needed badges.
    const [list, state] = await Promise.all([
      listClusterNodesApi(),
      getClusterStateApi().catch(() => null),
    ])
    nodes.value = list.data.list ?? []
    if (state?.data) {
      clusterReady.value = state.data.initialized === true
      const primary = state.data.nodes?.find(n => n.type === 'Primary')
      if (primary?.version) LATEST_VERSION.value = primary.version
    } else {
      clusterReady.value = false
    }
  } finally {
    loading.value = false
  }
}

const checkAll = async () => {
  refreshLoading.value = true
  try {
    await refreshClusterNodesApi()
    await loadNodes()
    ElMessage.success(t('cluster.refreshSuccess'))
  } finally {
    refreshLoading.value = false
  }
}

const failover = async (node: ClusterNode) => {
  await ElMessageBox.confirm(
    t('cluster.failoverConfirm', { name: node.name }),
    t('cluster.failoverTitle'),
    { type: 'warning', confirmButtonText: t('cluster.confirmSwitch'), cancelButtonText: t('common.cancel') }
  )
  const { data } = await failoverClusterNodeApi(node.id)
  detailVisible.value = false
  await loadNodes()
  ElMessage.success(t('cluster.switchSuccess', { name: node.name }))
  // The backend tags this response with a warning explaining that role
  // flip alone doesn't re-issue tokens / migrate scheduler ownership /
  // update VIPs. Show it loudly so operators don't assume a full failover.
  const warn = (data as any)?.warning
  if (warn) {
    ElMessageBox.alert(warn, t('cluster.failoverWarningTitle'), {
      confirmButtonText: t('common.ok'),
      type: 'warning',
    }).catch(() => {})
  }
}

const removeNode = async (node: ClusterNode) => {
  await ElMessageBox.confirm(t('cluster.removeConfirm', { name: node.name }), t('cluster.removeTitle'), { type: 'warning' })
  await removeClusterNodeApi(node.id)
  detailVisible.value = false
  await loadNodes()
  ElMessage.success(t('cluster.removeSuccess'))
}

const submitNode = async () => {
  if (!form.ip || !form.name || !form.zone) { ElMessage.warning(t('cluster.fillRequired')); return }
  submitting.value = true
  try {
    const { data } = await addClusterNodeApi({ ...form })
    await loadNodes()
    addDialog.value = false
    Object.assign(form, { name: '', ip: '', port: 53, role: '从节点', zone: '' })
    ElMessage.success(t('cluster.joinSuccess'))
    // The backend returns a join hint explaining that adding the row is
    // only step 1 — the operator still has to start the secondary process
    // on the target host. Show it as a sticky info notification.
    const hint = (data as any)?.joinHint
    if (hint) {
      ElMessageBox.alert(hint, t('cluster.joinHintTitle'), {
        confirmButtonText: t('common.ok'),
        type: 'info',
      }).catch(() => {})
    }
  } finally {
    submitting.value = false
  }
}

// ── Multi-select + batch ops ──
// Element Plus el-table emits selection-change with the full row array.
// We only need the IDs for the API; we keep the full rows so we can
// confirm against names in the modal.
const selectedRows = ref<ClusterNode[]>([])
const onSelectionChange = (rows: ClusterNode[]): void => { selectedRows.value = rows }

const batchRemove = async (): Promise<void> => {
  const rows = selectedRows.value.filter(r => roleKey(r.role) !== 'master')
  if (!rows.length) {
    ElMessage.warning(t('cluster.batchSelectAtLeastOne'))
    return
  }
  await ElMessageBox.confirm(
    t('cluster.batchRemoveConfirm', { count: rows.length, names: rows.slice(0, 5).map(r => r.name).join(', ') + (rows.length > 5 ? '...' : '') }),
    t('cluster.batchRemoveTitle'),
    { type: 'warning' }
  )
  await batchRemoveNodesApi(rows.map(r => r.id))
  selectedRows.value = []
  await loadNodes()
  ElMessage.success(t('cluster.batchRemoveDone', { count: rows.length }))
}

const toggleDrain = async (node: ClusterNode): Promise<void> => {
  const next = !node.drained
  await drainNodeApi(node.id, next)
  await loadNodes()
  ElMessage.success(next ? t('cluster.drainOn', { name: node.name }) : t('cluster.drainOff', { name: node.name }))
}

// ── Node detail live chart ──
// When the drawer opens for a node, fetch its 5-minute ring buffer once
// and re-fetch every 5s while the drawer stays open. The buffer is small
// (≤60 samples) so this is cheap.
const detailSamples = ref<NodeMetricSample[]>([])
let detailTimer: number | null = null

const loadDetailSamples = async (id: number): Promise<void> => {
  try {
    const { data } = await getNodeMetricsApi(id)
    detailSamples.value = data.samples ?? []
  } catch {
    detailSamples.value = []
  }
}

watch(detailVisible, (open) => {
  if (open && detailNode.value) {
    loadDetailSamples(detailNode.value.id)
    detailTimer = window.setInterval(() => {
      if (detailNode.value) loadDetailSamples(detailNode.value.id)
    }, 5_000)
  } else if (detailTimer !== null) {
    window.clearInterval(detailTimer)
    detailTimer = null
    detailSamples.value = []
  }
})

const detailChart = computed(() => ({
  times: detailSamples.value.map(s => s.at.slice(11, 19)),
  cpu:   detailSamples.value.map(s => s.cpuUsage),
  mem:   detailSamples.value.map(s => s.memUsage),
  qps:   detailSamples.value.map(s => s.qps),
}))

const detailChartOption = computed(() => ({
  grid: { top: 24, left: 36, right: 32, bottom: 28 },
  legend: { top: 0, textStyle: { fontSize: 11 }, itemHeight: 8, itemGap: 12 },
  tooltip: { trigger: 'axis' as const, confine: true },
  xAxis: { type: 'category' as const, data: detailChart.value.times, axisLabel: { fontSize: 10 } },
  yAxis: [
    { type: 'value' as const, name: '%', max: 100, axisLabel: { fontSize: 10 } },
    { type: 'value' as const, name: 'QPS', axisLabel: { fontSize: 10 } },
  ],
  series: [
    { name: 'CPU', type: 'line', smooth: true, data: detailChart.value.cpu, yAxisIndex: 0, lineStyle: { color: '#F53F3F', width: 2 }, itemStyle: { color: '#F53F3F' }, showSymbol: false },
    { name: 'MEM', type: 'line', smooth: true, data: detailChart.value.mem, yAxisIndex: 0, lineStyle: { color: '#FF7D00', width: 2 }, itemStyle: { color: '#FF7D00' }, showSymbol: false },
    { name: 'QPS', type: 'line', smooth: true, data: detailChart.value.qps, yAxisIndex: 1, lineStyle: { color: '#1677FF', width: 2 }, itemStyle: { color: '#1677FF' }, showSymbol: false },
  ],
}))

// ── SSE subscription ──
// Reload the table when a node joins / leaves / a state transition fires.
// We don't reload on heartbeat — that's what auto-poll is for elsewhere.
let sse: EventSource | null = null
const startSSE = (): void => {
  try {
    sse = openClusterEventStream()
    const refresh = (): void => { if (document.visibilityState === 'visible') loadNodes() }
    sse.addEventListener('node-joined', refresh)
    sse.addEventListener('node-left', refresh)
    sse.addEventListener('node-state', refresh)
    sse.addEventListener('sync-finished', refresh)
    sse.onerror = () => { /* EventSource auto-reconnects */ }
  } catch (e) {
    // SSE optional — page still works on manual refresh
    console.warn('cluster SSE failed', e)
  }
}

onMounted(() => {
  loadNodes()
  startSSE()
})

onBeforeUnmount(() => {
  if (sse) { sse.close(); sse = null }
  if (detailTimer !== null) { window.clearInterval(detailTimer); detailTimer = null }
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('cluster.nodeManage') }}</h1>
        <p class="page-subtitle">{{ $t('cluster.nodeManageSubtitle') }}</p>
      </div>
    </div>

    <ClusterControlPanel />

    <!-- KPI — only meaningful after the cluster is initialized; keeping
         them visible at zero on a fresh deploy made the page look broken. -->
    <div v-if="clusterReady" class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--success">
        <span class="mn-stat-value">{{ onlineCount }}</span>
        <span class="mn-stat-label">{{ $t('cluster.onlineNodes') }}</span>
      </div>
      <div :class="['mn-stat-tile', offlineCount > 0 ? 'mn-stat-tile--danger' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ offlineCount }}</span>
        <span class="mn-stat-label">{{ $t('cluster.offlineNodes') }}</span>
      </div>
      <div :class="['mn-stat-tile', syncIssues > 0 ? 'mn-stat-tile--warning' : 'mn-stat-tile--neutral']">
        <span class="mn-stat-value">{{ syncIssues }}</span>
        <span class="mn-stat-label">{{ $t('cluster.syncIssues') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--accent">
        <span class="mn-stat-value">{{ totalQps.toLocaleString() }}</span>
        <span class="mn-stat-label">{{ $t('cluster.totalQps') }}</span>
      </div>
      <div class="mn-stat-tile mn-stat-tile--accent">
        <span class="mn-stat-value">{{ nodes.length }}</span>
        <span class="mn-stat-label">{{ $t('cluster.nodeTotal') }}</span>
      </div>
    </div>

    <!-- Table card -->
    <el-card v-if="clusterReady" class="mn-card">
      <div class="mn-toolbar">
        <div class="mn-toolbar-filters">
          <el-input v-model="filterKeyword" clearable :placeholder="$t('cluster.searchNodePlaceholder')" style="width:220px">
            <template #prefix><svg class="mn-input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></template>
          </el-input>
          <el-select v-model="filterRole" clearable :placeholder="$t('cluster.role')" style="width:120px">
            <el-option :label="$t('cluster.masterNode')" value="master" />
            <el-option :label="$t('cluster.slaveNode')" value="slave" />
            <el-option :label="$t('cluster.arbiterNode')" value="arbiter" />
          </el-select>
          <el-select v-model="filterStatus" clearable :placeholder="$t('cluster.status')" style="width:120px">
            <el-option :label="$t('cluster.online')" value="online" />
            <el-option :label="$t('cluster.offline')" value="offline" />
            <el-option :label="$t('cluster.syncing')" value="syncing" />
            <el-option :label="$t('cluster.degraded')" value="degraded" />
          </el-select>
        </div>
        <div class="mn-toolbar-actions">
          <el-button v-if="selectedRows.length > 0" type="danger" plain @click="batchRemove">
            {{ $t('cluster.batchRemoveBtn', { count: selectedRows.length }) }}
          </el-button>
          <el-button :loading="refreshLoading" @click="checkAll">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
            {{ $t('cluster.refreshStatus') }}
          </el-button>
          <el-button type="primary" @click="addDialog = true">
            <template #icon><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
            {{ $t('cluster.addNode') }}
          </el-button>
        </div>
      </div>

      <el-table
        :data="filtered" stripe class="mn-table" table-layout="auto"
        :row-class-name="rowClassName"
        :default-sort="{ prop: 'qps', order: 'descending' }"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="42" :selectable="(row: ClusterNode) => roleKey(row.role) !== 'master'" />
        <el-table-column prop="name" :label="$t('cluster.nodeName')" min-width="160" sortable>
          <template #default="{ row }">
            <div class="node-name-cell">
              <span class="node-dot" :class="'node-dot--' + (statusKey(row.status) === 'online' ? 'ok' : statusKey(row.status) === 'offline' ? 'off' : 'warn')"></span>
              <span class="mn-mono node-link" @click="openDetail(row)">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="ip" :label="$t('cluster.address')" min-width="160">
          <template #default="{ row }"><span class="mn-mono">{{ row.ip }}:{{ row.port }}</span></template>
        </el-table-column>
        <el-table-column prop="role" :label="$t('cluster.role')" width="110" align="center">
          <template #default="{ row }">
            <span class="type-badge" :style="{ background: roleColorByKey[roleKey(row.role)] }">{{ roleLabel(row.role) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="zone" :label="$t('cluster.zone')" width="90" align="center">
          <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.zone }}</span></template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('cluster.status')" width="110" align="center">
          <template #default="{ row }">
            <div class="status-stack">
              <span :class="['mn-badge', statusClass(row.status)]">{{ statusLabel(row.status) }}</span>
              <span v-if="row.drained" class="mn-badge mn-badge--neutral drain-tag">{{ $t('cluster.drained') }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="cpuUsage" label="CPU" width="130" sortable>
          <template #default="{ row }">
            <div class="usage-cell">
              <el-progress :percentage="row.cpuUsage" :color="usageColor(row.cpuUsage)" :stroke-width="5" :show-text="false" style="flex:1" />
              <span class="usage-pct mn-mono">{{ row.cpuUsage }}%</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="memUsage" :label="$t('cluster.memory')" width="130" sortable>
          <template #default="{ row }">
            <div class="usage-cell">
              <el-progress :percentage="row.memUsage" :color="usageColor(row.memUsage)" :stroke-width="5" :show-text="false" style="flex:1" />
              <span class="usage-pct mn-mono">{{ row.memUsage }}%</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="qps" label="QPS" width="90" align="right" sortable>
          <template #default="{ row }"><span class="mn-mono">{{ row.qps.toLocaleString() }}</span></template>
        </el-table-column>
        <el-table-column prop="syncLag" :label="$t('cluster.syncDelay')" width="105" align="right" sortable>
          <template #default="{ row }">
            <span class="mn-mono" :style="{ color: row.syncLag > 100 ? 'var(--app-danger)' : row.syncLag > 30 ? 'var(--app-warning)' : 'var(--app-success)' }">
              {{ row.syncLag === 99999 ? '—' : row.syncLag + ' ms' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="version" :label="$t('cluster.version')" width="110" align="center">
          <template #default="{ row }">
            <div class="version-cell">
              <span class="mn-mono" style="font-size:12px">{{ row.version }}</span>
              <el-tooltip v-if="isOutdated(row.version)" :content="`${$t('cluster.latestVersion')} ${LATEST_VERSION}`" placement="top">
                <span class="outdated-dot">{{ $t('cluster.needUpgrade') }}</span>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="lastHeartbeat" :label="$t('cluster.lastHeartbeat')" width="110" align="right">
          <template #default="{ row }">
            <el-tooltip :content="formatAbsoluteTime(row.lastHeartbeat)" placement="top" :disabled="isZeroTime(row.lastHeartbeat)">
              <span class="mn-time heartbeat-rel">{{ relativeTime(row.lastHeartbeat) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="160" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button link type="primary" @click="openDetail(row)">{{ $t('cluster.detail') }}</el-button>
              <el-tooltip
                v-if="roleKey(row.role) === 'slave' && statusKey(row.status) === 'online'"
                :content="$t('cluster.failoverTooltip')"
                placement="top"
              >
                <el-button link type="warning" @click="failover(row)">{{ $t('cluster.failover') }}</el-button>
              </el-tooltip>
              <el-button
                v-if="roleKey(row.role) === 'slave'"
                link :type="row.drained ? 'success' : 'info'"
                @click="toggleDrain(row)"
              >{{ row.drained ? $t('cluster.undrainBtn') : $t('cluster.drainBtn') }}</el-button>
              <el-button link type="danger" @click="removeNode(row)">{{ $t('cluster.removeNode') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('cluster.noNodes')" /></template>
      </el-table>
    </el-card>

    <!-- Detail drawer -->
    <el-drawer v-model="detailVisible" :title="detailNode?.name ?? $t('cluster.detail')" size="400px" append-to-body>
      <template v-if="detailNode">
        <div class="drawer-section">
          <div class="drawer-kv-grid">
            <span class="dk-label">{{ $t('cluster.role') }}</span>
            <span class="type-badge" :style="{ background: roleColorByKey[roleKey(detailNode.role)] }">{{ roleLabel(detailNode.role) }}</span>
            <span class="dk-label">{{ $t('cluster.status') }}</span>
            <span :class="['mn-badge', statusClass(detailNode.status)]">{{ statusLabel(detailNode.status) }}</span>
            <span class="dk-label">{{ $t('cluster.address') }}</span>
            <span class="mn-mono dk-val">{{ detailNode.ip }}:{{ detailNode.port }}</span>
            <span class="dk-label">{{ $t('cluster.zone') }}</span>
            <span class="dk-val">{{ detailNode.zone }}</span>
            <span class="dk-label">{{ $t('cluster.version') }}</span>
            <span class="mn-mono dk-val">{{ detailNode.version }}</span>
            <span class="dk-label">{{ $t('cluster.joinedAt') }}</span>
            <span class="mn-time dk-val">{{ formatAbsoluteTime(detailNode.joinedAt) }}</span>
            <span class="dk-label">{{ $t('cluster.lastHeartbeat') }}</span>
            <span class="mn-time dk-val">{{ formatAbsoluteTime(detailNode.lastHeartbeat) }}</span>
          </div>
        </div>
        <div class="drawer-section">
          <div class="drawer-section-title">{{ $t('cluster.resourceUsage') }}</div>
          <div class="drawer-usage-list">
            <div class="du-row">
              <span class="du-label">CPU</span>
              <el-progress :percentage="detailNode.cpuUsage" :color="usageColor(detailNode.cpuUsage)" :stroke-width="8" style="flex:1" />
            </div>
            <div class="du-row">
              <span class="du-label">{{ $t('cluster.memory') }}</span>
              <el-progress :percentage="detailNode.memUsage" :color="usageColor(detailNode.memUsage)" :stroke-width="8" style="flex:1" />
            </div>
          </div>
        </div>
        <div class="drawer-section">
          <div class="drawer-section-title">{{ $t('cluster.performance') }}</div>
          <div class="drawer-kv-grid">
            <span class="dk-label">QPS</span>
            <span class="mn-mono dk-val" style="font-size:16px;font-weight:700;color:var(--app-accent)">{{ detailNode.qps.toLocaleString() }}</span>
            <span class="dk-label">{{ $t('cluster.syncDelay') }}</span>
            <span class="mn-mono dk-val" :style="{ fontSize:'16px', fontWeight:'700', color: detailNode.syncLag > 100 ? 'var(--app-danger)' : detailNode.syncLag > 30 ? 'var(--app-warning)' : 'var(--app-success)' }">
              {{ detailNode.syncLag === 99999 ? $t('cluster.offline') : detailNode.syncLag + ' ms' }}
            </span>
          </div>
        </div>
        <div class="drawer-section">
          <div class="drawer-section-title">{{ $t('cluster.liveTrend') }}</div>
          <div v-if="detailSamples.length === 0" class="drawer-trend-empty">{{ $t('cluster.liveTrendEmpty') }}</div>
          <BaseChart v-else :option="detailChartOption" style="height:180px" />
        </div>
        <div class="drawer-actions">
          <el-button v-if="roleKey(detailNode.role) === 'slave' && statusKey(detailNode.status) === 'online'" type="warning" @click="failover(detailNode)">{{ $t('cluster.failover') }}</el-button>
          <el-button v-if="roleKey(detailNode.role) === 'slave'" :type="detailNode.drained ? 'success' : 'info'" plain @click="toggleDrain(detailNode)">
            {{ detailNode.drained ? $t('cluster.undrainBtn') : $t('cluster.drainBtn') }}
          </el-button>
          <el-button type="danger" plain @click="removeNode(detailNode)">{{ $t('cluster.removeNode') }}</el-button>
        </div>
      </template>
    </el-drawer>

    <!-- Add node dialog -->
    <BaseModal
      v-model="addDialog"
      :title="$t('cluster.addNodeTitle')"
      :width="460"
      :loading="submitting"
      :confirm-text="$t('cluster.joinCluster')"
      :cancel-text="$t('common.cancel')"
      @confirm="submitNode"
    >
      <el-form :model="form" label-position="top" class="add-node-form">
        <div class="form-grid">
          <el-form-item :label="$t('cluster.nodeName')" required>
            <el-input v-model="form.name" :placeholder="$t('cluster.nodePlaceholder')" />
          </el-form-item>
          <el-form-item :label="$t('cluster.role')" required>
            <el-select v-model="form.role" style="width:100%">
              <el-option :label="$t('cluster.slaveNode')" value="从节点" />
              <el-option :label="$t('cluster.arbiterNode')" value="仲裁节点" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('common.ip')" required>
            <el-input v-model="form.ip" :placeholder="$t('cluster.ipPlaceholder')" />
          </el-form-item>
          <el-form-item :label="$t('common.port')" required>
            <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" style="width:100%" />
          </el-form-item>
          <el-form-item :label="$t('cluster.zone')" style="grid-column:1/-1" required>
            <el-select v-model="form.zone" :placeholder="$t('cluster.selectZone')" style="width:100%" filterable allow-create default-first-option>
              <el-option v-for="z in ZONE_OPTIONS" :key="z" :label="z" :value="z" />
            </el-select>
          </el-form-item>
        </div>
      </el-form>
    </BaseModal>
  </div>
</template>

<style scoped>
.mn-stats-strip { display:flex; gap:14px; flex-wrap:wrap; }
.mn-stat-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--app-border); background:var(--app-bg); min-width:100px; flex:1; box-shadow:var(--app-shadow-soft); }
.mn-stat-value { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; }
.mn-stat-label { font-size:12px; font-weight:500; color:var(--app-text-regular); line-height:1.4; }
.mn-stat-tile--success .mn-stat-value { color:var(--app-success); }
.mn-stat-tile--danger .mn-stat-value  { color:var(--app-danger); }
.mn-stat-tile--warning .mn-stat-value { color:var(--app-warning); }
.mn-stat-tile--accent .mn-stat-value  { color:var(--app-accent); }
.mn-stat-tile--neutral .mn-stat-value { color:var(--app-text-secondary); }

.mn-toolbar { display:flex; align-items:center; justify-content:space-between; gap:10px; flex-wrap:wrap; padding:12px 16px; background:var(--app-bg-secondary); border-bottom:1px solid var(--app-border); }
.mn-toolbar-filters { display:flex; align-items:center; flex-wrap:wrap; gap:8px; flex:1; }
.mn-toolbar-actions { display:flex; align-items:center; gap:8px; flex-shrink:0; }
.mn-input-icon { width:13px; height:13px; color:var(--app-text-regular); }

.node-name-cell { display:flex; align-items:center; gap:7px; }
.node-dot { width:8px; height:8px; border-radius:50%; flex-shrink:0; }
.node-dot--ok   { background:#10b981; box-shadow:0 0 0 3px rgba(16,185,129,0.15); }
.node-dot--off  { background:#94a3b8; }
.node-dot--warn { background:#f59e0b; box-shadow:0 0 0 3px rgba(245,158,11,0.15); animation:pulse 1.5s ease-in-out infinite; }
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.5} }
.node-link { cursor:pointer; color:var(--app-accent); font-weight:600; }
.node-link:hover { text-decoration:underline; }

.usage-cell { display:flex; align-items:center; gap:8px; }
.usage-pct  { font-size:12px; width:34px; text-align:right; flex-shrink:0; color:var(--app-text-secondary); }

.type-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; color:#fff; }

.mn-row-ops { display:flex; align-items:center; justify-content:flex-end; gap:6px; }

.form-grid { display:grid; grid-template-columns:1fr 1fr; gap:0 16px; }

/* Offline row dim */
:deep(.row-offline) { opacity: 0.52; }
:deep(.row-offline:hover) { opacity: 0.72; }

/* Version outdated badge */
.version-cell { display:flex; align-items:center; gap:5px; justify-content:center; }
.outdated-dot {
  display: inline-flex;
  align-items: center;
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 700;
  background: rgba(255,125,0,0.12);
  color: var(--app-warning);
  cursor: default;
  white-space: nowrap;
}

/* Relative heartbeat */
.heartbeat-rel { font-size:12px; color:var(--app-text-regular); cursor:default; }

/* Status column stack with drained tag */
.status-stack { display:flex; flex-direction:column; align-items:center; gap:3px; }
.drain-tag { font-size:10px; padding:1px 6px; }

/* Drawer live trend empty state */
.drawer-trend-empty {
  padding:20px; text-align:center; font-size:12px;
  color:var(--dns-text-assist-color);
  background:var(--dns-bg-page); border-radius:6px;
}

/* Add node form */
.add-node-form :deep(.el-form-item__label) { font-size:13px; font-weight:500; padding-bottom:4px; }
.add-node-form :deep(.el-form-item) { margin-bottom:16px; }

/* Drawer */
.drawer-section { padding: 16px 0; border-bottom: 1px solid var(--app-border); }
.drawer-section:last-child { border-bottom: none; }
.drawer-section-title { font-size:12px; font-weight:700; text-transform:uppercase; letter-spacing:0.06em; color:var(--app-text-secondary); margin-bottom:12px; }
.drawer-kv-grid { display:grid; grid-template-columns:80px 1fr; gap:10px 12px; align-items:center; }
.dk-label { font-size:12px; color:var(--app-text-secondary); }
.dk-val { font-size:13px; color:var(--app-text-primary,#1d2129); }
.drawer-usage-list { display:flex; flex-direction:column; gap:10px; }
.du-row { display:flex; align-items:center; gap:12px; }
.du-label { font-size:12px; color:var(--app-text-secondary); width:28px; flex-shrink:0; }
.drawer-actions { display:flex; gap:10px; padding-top:16px; }
</style>
