<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseModal from '@/components/BaseModal.vue'
import {
  listConfigSyncApi,
  syncNodeConfigApi,
  syncAllNodeConfigsApi,
  getConfigDiffApi,
  refreshConfigDiffApi,
  listSyncHistoryApi,
  getClusterStateApi,
  openClusterEventStream,
  type ConfigSyncRow,
  type ConfigDiffItem,
  type SyncHistoryRow,
  type ClusterStatePayload,
  type ClusterStateNode,
} from '@/api/cluster'

const { t } = useI18n()
const loading     = ref(false)
const nodes       = ref<ConfigSyncRow[]>([])
const syncingIds  = ref<Set<number>>(new Set())
const diffVisible = ref(false)
const diffNode    = ref<ConfigSyncRow | null>(null)
const diffItems   = ref<ConfigDiffItem[]>([])
const diffLoading = ref(false)

const syncedCount  = computed(() => nodes.value.filter(n => n.syncStatus === '已同步').length)
const pendingCount = computed(() => nodes.value.filter(n => n.syncStatus === '待同步').length)
const failedCount  = computed(() => nodes.value.filter(n => n.syncStatus === '同步失败').length)
const totalDiffs   = computed(() => nodes.value.reduce((s, n) => s + n.diffCount, 0))

const statusClass = (s: string) =>
  s === '已同步' ? 'mn-badge--success' : s === '待同步' ? 'mn-badge--warning' : s === '同步中' ? 'mn-badge--info' : 'mn-badge--danger'

const roleColor: Record<string, string> = { '主节点': '#F53F3F', '从节点': '#1677FF', '仲裁节点': '#722ED1' }
const masterVersion = ref('—')

// Cluster bootstrap state — when initialized=false we render an empty
// state CTA pointing to /cluster (where ClusterControlPanel lives) instead
// of an empty sync table that looks broken.
const clusterState = ref<ClusterStatePayload | null>(null)
const clusterReady = computed<boolean>(() => clusterState.value?.initialized === true)
const masterNode   = computed<ClusterStateNode | null>(() =>
  clusterState.value?.nodes?.find(n => n.type === 'Primary') ?? null
)
// Banner subtext — shows "name · ip · cluster-domain" pulled from the
// real cluster state, replacing the previously hardcoded sample values.
const masterDescription = computed<string>(() => {
  const m = masterNode.value
  if (!m) return ''
  const ip = m.ipAddresses?.[0] ?? '—'
  const domain = clusterState.value?.clusterDomain ?? ''
  return `${m.name} · ${ip}${domain ? ' · ' + domain : ''}`
})

const loadSync = async () => {
  loading.value = true
  try {
    // Cluster state + sync rows are both read on every refresh so the
    // master-version banner stays in step with reality (e.g. after a
    // failover the primary changes and we must re-render the banner).
    const [state, list] = await Promise.all([
      getClusterStateApi().catch(() => null),
      listConfigSyncApi(),
    ])
    if (state?.data) clusterState.value = state.data
    nodes.value = list.data.list ?? []
    if (list.data.masterVersion) masterVersion.value = list.data.masterVersion
  } finally {
    loading.value = false
  }
}

const syncNode = async (node: ConfigSyncRow) => {
  syncingIds.value.add(node.id)
  node.syncStatus = '同步中'
  try {
    const { data } = await syncNodeConfigApi(node.id)
    Object.assign(node, data)
    ElMessage.success(t('cluster.syncNodeSuccess', { name: node.name }))
  } catch {
    node.syncStatus = '同步失败'
    ElMessage.error(t('cluster.syncNodeFailed', { name: node.name }))
  } finally {
    syncingIds.value.delete(node.id)
  }
}

const syncAll = async () => {
  const pending = nodes.value.filter(n => n.syncStatus !== '已同步')
  if (!pending.length) { ElMessage.info(t('cluster.allSynced')); return }
  await ElMessageBox.confirm(
    t('cluster.syncAllConfirm', { count: pending.length }),
    t('cluster.syncAllTitle'), { type: 'warning', confirmButtonText: t('cluster.confirmSync') }
  )
  await syncAllNodeConfigsApi()
  await loadSync()
  ElMessage.success(t('cluster.syncAllSuccess', { count: pending.length }))
}

const openDiff = async (node: ConfigSyncRow) => {
  diffNode.value    = node
  diffVisible.value = true
  diffLoading.value = true
  try {
    const { data } = await getConfigDiffApi(node.id)
    diffItems.value = data ?? []
  } finally {
    diffLoading.value = false
  }
}

// Live diff refresh — pull the secondary's snapshot, recompute the diff
// on the primary, and replace the modal contents. Useful when the cached
// diff is stale (e.g. someone edited records but didn't push yet).
const refreshDiff = async (): Promise<void> => {
  if (!diffNode.value) return
  diffLoading.value = true
  try {
    const { data } = await refreshConfigDiffApi(diffNode.value.id)
    diffItems.value = data ?? []
    ElMessage.success(t('cluster.diffRefreshed'))
  } catch (e: any) {
    ElMessage.error(e?.message ?? t('cluster.diffRefreshFailed'))
  } finally {
    diffLoading.value = false
  }
}

// ── Sync history modal ──
const historyVisible        = ref(false)
const historyRows           = ref<SyncHistoryRow[]>([])
const historyLoading        = ref(false)
const historyTotal          = ref(0)
const historyPage           = ref(1)
const historySize           = 20
const historyTriggerFilter  = ref<string>('')
const historyKeyword        = ref<string>('')

const openHistory = async (): Promise<void> => {
  historyVisible.value = true
  historyPage.value    = 1
  historyTriggerFilter.value = ''
  historyKeyword.value       = ''
  await loadHistory()
}

const loadHistory = async (): Promise<void> => {
  historyLoading.value = true
  try {
    const { data } = await listSyncHistoryApi({
      page:    historyPage.value,
      size:    historySize,
      trigger: historyTriggerFilter.value || undefined,
      keyword: historyKeyword.value || undefined,
    })
    historyRows.value  = data.list ?? []
    historyTotal.value = data.total ?? 0
  } finally {
    historyLoading.value = false
  }
}

const triggerLabel = (trigger: string): string => {
  switch (trigger) {
    case 'manual':     return t('cluster.triggerManual')
    case 'manual-all': return t('cluster.triggerManualAll')
    case 'auto':       return t('cluster.triggerAuto')
    case 'retry':      return t('cluster.triggerRetry')
    default:           return trigger
  }
}

const triggerClass = (trigger: string): string => {
  switch (trigger) {
    case 'manual':     return 'mn-badge--info'
    case 'manual-all': return 'mn-badge--info'
    case 'auto':       return 'mn-badge--success'
    case 'retry':      return 'mn-badge--warning'
    default:           return 'mn-badge--neutral'
  }
}

// SSE — refetch the table when a push finishes anywhere in the cluster,
// and refresh the history modal contents if the operator has it open.
let sse: EventSource | null = null

onMounted(() => {
  loadSync()
  try {
    sse = openClusterEventStream()
    sse.addEventListener('sync-finished', () => {
      if (document.visibilityState !== 'visible') return
      loadSync()
      if (historyVisible.value) loadHistory()
    })
    sse.onerror = () => { /* auto-reconnect */ }
  } catch (e) {
    console.warn('cluster SSE failed', e)
  }
})

onBeforeUnmount(() => {
  if (sse) { sse.close(); sse = null }
})
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('cluster.configSync') }}</h1>
        <p class="page-subtitle">{{ $t('cluster.configSyncSubtitle') }}</p>
      </div>
      <div v-if="clusterReady" class="header-actions">
        <el-button @click="openHistory">{{ $t('cluster.syncHistory') }}</el-button>
        <el-button type="primary" @click="syncAll">
          <template #icon>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </template>
          {{ $t('cluster.syncAll') }}
        </el-button>
      </div>
    </div>

    <!-- KPI -->
    <div v-if="clusterReady" class="kpi-strip">
      <div class="kpi-tile kpi-tile--success">
        <span class="kpi-val">{{ syncedCount }}</span>
        <span class="kpi-lbl">{{ $t('cluster.syncedNodes') }}</span>
      </div>
      <div :class="['kpi-tile', pendingCount > 0 ? 'kpi-tile--warning' : 'kpi-tile--neutral']">
        <span class="kpi-val">{{ pendingCount }}</span>
        <span class="kpi-lbl">{{ $t('cluster.pendingNodes') }}</span>
      </div>
      <div :class="['kpi-tile', failedCount > 0 ? 'kpi-tile--danger' : 'kpi-tile--neutral']">
        <span class="kpi-val">{{ failedCount }}</span>
        <span class="kpi-lbl">{{ $t('cluster.failedNodes') }}</span>
      </div>
      <div :class="['kpi-tile', totalDiffs > 0 ? 'kpi-tile--warning' : 'kpi-tile--neutral']">
        <span class="kpi-val">{{ totalDiffs }}</span>
        <span class="kpi-lbl">{{ $t('cluster.pendingConfigs') }}</span>
      </div>
    </div>

    <!-- Cluster-not-initialized empty state — replaces all KPI/table content
         until the operator boots the cluster from the node management page. -->
    <div v-if="!loading && !clusterReady" class="empty-bootstrap">
      <div class="empty-bootstrap-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      </div>
      <div class="empty-bootstrap-body">
        <h3>{{ $t('cluster.notInitTitle') }}</h3>
        <p>{{ $t('cluster.notInitSyncDesc') }}</p>
      </div>
      <router-link to="/cluster" class="empty-bootstrap-link">
        <el-button type="primary">{{ $t('cluster.goToInit') }}</el-button>
      </router-link>
    </div>

    <!-- Master version banner — uses real cluster state instead of hardcoded
         sample values; rendered only after cluster is initialized. -->
    <div v-if="clusterReady" class="master-banner">
      <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" class="banner-icon"><circle cx="8" cy="8" r="6"/><path d="M8 5v3l2 1"/></svg>
      <span>{{ $t('cluster.masterConfigVersion') }}:</span>
      <code class="banner-version">{{ masterVersion }}</code>
      <span v-if="masterDescription" class="banner-node">{{ masterDescription }}</span>
    </div>

    <!-- Table -->
    <el-card v-if="clusterReady" class="mn-card">
      <el-table :data="nodes" stripe class="mn-table" table-layout="auto">
        <el-table-column prop="name" :label="$t('cluster.nodeName')" min-width="160">
          <template #default="{ row }">
            <div class="name-cell">
              <span class="name-text mn-mono">{{ row.name }}</span>
              <span class="type-badge" :style="{ background: roleColor[row.role] }">{{ row.role }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="ip" :label="$t('cluster.address')" width="140">
          <template #default="{ row }"><span class="mn-mono">{{ row.ip }}</span></template>
        </el-table-column>
        <el-table-column prop="zone" :label="$t('cluster.zone')" width="90" align="center">
          <template #default="{ row }"><span class="mn-badge mn-badge--neutral">{{ row.zone }}</span></template>
        </el-table-column>
        <el-table-column prop="configVersion" :label="$t('cluster.currentConfigVersion')" min-width="200">
          <template #default="{ row }">
            <div class="version-cell">
              <code class="ver-code" :class="{ 'ver-outdated': row.configVersion !== masterVersion }">{{ row.configVersion }}</code>
              <span v-if="row.configVersion !== masterVersion" class="ver-diff-badge">{{ row.diffCount }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="syncStatus" :label="$t('cluster.syncStatus')" width="110" align="center">
          <template #default="{ row }">
            <span :class="['mn-badge', statusClass(row.syncStatus)]">{{ row.syncStatus }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="lastSyncAt" :label="$t('cluster.lastSyncAt')" min-width="170">
          <template #default="{ row }"><span class="mn-time">{{ row.lastSyncAt }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('common.operation')" width="180" fixed="right">
          <template #default="{ row }">
            <div class="mn-row-ops">
              <el-button
                v-if="row.diffCount > 0"
                link type="primary"
                @click="openDiff(row)"
              >{{ $t('cluster.viewDiff') }}</el-button>
              <el-button
                link
                :type="row.syncStatus === '同步失败' ? 'danger' : 'primary'"
                :loading="syncingIds.has(row.id)"
                :disabled="row.syncStatus === '已同步'"
                @click="syncNode(row)"
              >{{ row.syncStatus === '已同步' ? $t('cluster.upToDate') : $t('cluster.syncNow') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="$t('cluster.noNodeData')" /></template>
      </el-table>
    </el-card>

    <!-- Diff dialog -->
    <BaseModal
      v-model="diffVisible"
      :title="`${$t('cluster.configDiff')} — ${diffNode?.name}`"
      :width="680"
      :show-confirm="false"
      :cancel-text="$t('common.close')"
    >
      <div v-if="diffNode" v-loading="diffLoading" class="diff-wrap">
        <div class="diff-meta">
          <span>{{ $t('cluster.diffCompare', { version: masterVersion, count: diffNode.diffCount }) }}</span>
          <el-button size="small" link type="primary" :loading="diffLoading" @click="refreshDiff">
            {{ $t('cluster.diffRefresh') }}
          </el-button>
        </div>
        <div class="diff-table">
          <div class="diff-thead">
            <span>{{ $t('cluster.configItem') }}</span>
            <span>{{ $t('cluster.masterValue') }}</span>
            <span>{{ $t('cluster.nodeValue') }}</span>
          </div>
          <div
            v-for="item in diffItems" :key="item.key"
            :class="['diff-row', { 'diff-row--changed': item.changed }]"
          >
            <span class="diff-key">{{ item.label }}</span>
            <code class="diff-val diff-val--master">{{ item.masterValue }}</code>
            <code class="diff-val" :class="item.changed ? 'diff-val--changed' : 'diff-val--same'">{{ item.nodeValue }}</code>
          </div>
          <div v-if="!diffItems.length && !diffLoading" class="diff-empty">
            {{ $t('cluster.diffEmpty') }}
          </div>
        </div>
      </div>
    </BaseModal>

    <!-- Sync history dialog -->
    <BaseModal
      v-model="historyVisible"
      :title="$t('cluster.syncHistoryTitle')"
      :width="980"
      :show-confirm="false"
      :cancel-text="$t('common.close')"
    >
      <div v-loading="historyLoading">
        <!-- Trigger filter row — wires up the keyword + trigger params the
             backend already supports but the UI didn't surface before. -->
        <div class="history-toolbar">
          <el-select
            v-model="historyTriggerFilter"
            clearable
            size="small"
            :placeholder="$t('cluster.trigger')"
            style="width:160px"
            @change="() => { historyPage = 1; loadHistory() }"
          >
            <el-option :label="$t('cluster.triggerManual')"    value="manual" />
            <el-option :label="$t('cluster.triggerManualAll')" value="manual-all" />
            <el-option :label="$t('cluster.triggerAuto')"      value="auto" />
            <el-option :label="$t('cluster.triggerRetry')"     value="retry" />
          </el-select>
          <el-input
            v-model="historyKeyword"
            clearable
            size="small"
            :placeholder="$t('cluster.searchVersionOrUser')"
            style="width:220px"
            @keyup.enter="() => { historyPage = 1; loadHistory() }"
            @clear="() => { historyPage = 1; loadHistory() }"
          />
          <el-button size="small" @click="() => { historyPage = 1; loadHistory() }">{{ $t('common.search') }}</el-button>
        </div>
        <el-table :data="historyRows" stripe class="mn-table" max-height="520">
          <!-- Expand row = failure details on demand. Replaces the previous
               cramped "failureDetail" column that overflowed the modal width. -->
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="history-expand">
                <div class="history-expand-section">
                  <h4>{{ $t('cluster.successCount') }} ({{ row.successCount }})</h4>
                  <ul v-if="row.detail.filter((x: any) => x.ok).length" class="history-detail history-detail--ok">
                    <li v-for="d in row.detail.filter((x: any) => x.ok)" :key="d.nodeId">
                      <strong>{{ d.name }}</strong> · <span class="mn-mono">{{ d.url }}</span>
                    </li>
                  </ul>
                  <p v-else class="history-empty">—</p>
                </div>
                <div class="history-expand-section">
                  <h4>{{ $t('cluster.failedCount') }} ({{ row.failedCount }})</h4>
                  <ul v-if="row.detail.filter((x: any) => !x.ok).length" class="history-detail history-detail--err">
                    <li v-for="d in row.detail.filter((x: any) => !x.ok)" :key="d.nodeId">
                      <strong>{{ d.name }}</strong>: {{ d.message }}
                    </li>
                  </ul>
                  <p v-else class="history-empty">—</p>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="version" :label="$t('cluster.syncVersion')" min-width="180">
            <template #default="{ row }"><code class="ver-code">{{ row.version || '—' }}</code></template>
          </el-table-column>
          <el-table-column prop="trigger" :label="$t('cluster.trigger')" width="110" align="center">
            <template #default="{ row }">
              <span :class="['mn-badge', triggerClass(row.trigger)]">{{ triggerLabel(row.trigger) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="triggeredBy" :label="$t('cluster.triggeredBy')" width="110" />
          <el-table-column :label="$t('cluster.startedAt')" width="170">
            <template #default="{ row }"><span class="mn-time">{{ row.startedAt }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('cluster.totalNodes')" width="80" align="center">
            <template #default="{ row }">{{ row.totalNodes }}</template>
          </el-table-column>
          <el-table-column :label="$t('cluster.successCount')" width="80" align="center">
            <template #default="{ row }"><span class="mn-badge mn-badge--success">{{ row.successCount }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('cluster.failedCount')" width="80" align="center">
            <template #default="{ row }">
              <span :class="['mn-badge', row.failedCount > 0 ? 'mn-badge--danger' : 'mn-badge--neutral']">{{ row.failedCount }}</span>
            </template>
          </el-table-column>
          <template #empty><el-empty :description="$t('cluster.noHistoryData')" /></template>
        </el-table>
        <el-pagination
          v-if="historyTotal > historySize"
          class="history-pager"
          background
          layout="prev, pager, next"
          :page-size="historySize"
          :total="historyTotal"
          :current-page="historyPage"
          @current-change="(p: number) => { historyPage = p; loadHistory() }"
        />
      </div>
    </BaseModal>
  </div>
</template>

<style scoped>
/* KPI */
.kpi-strip { display:flex; gap:14px; flex-wrap:wrap; }
.kpi-tile { display:flex; flex-direction:row; align-items:center; gap:14px; padding:14px 24px; border-radius:10px; border:1px solid var(--dns-border-color); background:var(--dns-bg-card); flex:1; min-width:100px; box-shadow:var(--app-shadow-soft); }
.kpi-val  { font-size:28px; font-weight:700; font-variant-numeric:tabular-nums; letter-spacing:-0.03em; line-height:1; color:var(--dns-text-title-color); }
.kpi-lbl  { font-size:12px; font-weight:500; color:var(--dns-text-body-color); }
.kpi-tile--success .kpi-val { color:var(--app-success); }
.kpi-tile--warning .kpi-val { color:var(--app-warning); }
.kpi-tile--danger  .kpi-val { color:var(--app-danger); }
.kpi-tile--neutral .kpi-val { color:var(--app-disabled); }

/* Master banner */
.master-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: var(--app-accent-soft);
  border: 1px solid var(--app-accent-muted);
  border-radius: 8px;
  font-size: 13px;
  color: var(--dns-text-body-color);
}
.banner-icon { width:15px; height:15px; color:var(--app-accent); flex-shrink:0; }
.banner-version { font-family:'JetBrains Mono','Consolas',monospace; font-size:12px; color:var(--app-accent); background:rgba(22,93,255,0.08); padding:2px 7px; border-radius:4px; }
.banner-node { margin-left:auto; font-size:12px; color:var(--dns-text-assist-color); }

/* Table cells */
.name-cell { display:flex; align-items:center; gap:8px; }
.name-text { font-size:13px; font-weight:600; color:var(--dns-text-title-color); }
.type-badge { display:inline-flex; align-items:center; justify-content:center; padding:2px 7px; border-radius:4px; font-size:11px; font-weight:700; color:#fff; flex-shrink:0; }

.version-cell { display:flex; align-items:center; gap:8px; }
.ver-code { font-family:'JetBrains Mono','Consolas',monospace; font-size:11px; padding:2px 7px; border-radius:4px; background:var(--dns-bg-page); color:var(--dns-text-body-color); }
.ver-outdated { background:rgba(255,125,0,0.08); color:var(--app-warning); }
.ver-diff-badge { font-size:11px; font-weight:600; color:var(--app-warning); background:rgba(255,125,0,0.1); padding:1px 6px; border-radius:3px; }

.mn-row-ops { display:flex; align-items:center; justify-content:flex-end; gap:6px; }

/* Diff dialog */
.diff-wrap { display:flex; flex-direction:column; gap:12px; }
.diff-meta {
  display:flex; align-items:center; justify-content:space-between; gap:12px;
  font-size:13px; color:var(--dns-text-body-color);
  padding:8px 12px; background:var(--dns-bg-page); border-radius:6px;
}
.diff-meta code { font-family:'JetBrains Mono','Consolas',monospace; font-size:11px; color:var(--app-accent); background:rgba(22,93,255,0.08); padding:1px 6px; border-radius:3px; }
.diff-empty { padding:24px; text-align:center; color:var(--dns-text-assist-color); font-size:13px; }

/* Header actions */
.header-actions { display:flex; gap:8px; align-items:center; }

/* Sync history modal */
.history-toolbar { display:flex; gap:10px; align-items:center; margin-bottom:14px; }
.history-detail { margin:6px 0 0; padding-left:18px; font-size:12px; }
.history-detail li { line-height:1.7; word-break:break-all; }
.history-detail strong { color:var(--dns-text-title-color); margin-right:4px; }
.history-detail--ok li { color:var(--app-text-secondary); }
.history-detail--err li { color:var(--app-danger); }
.history-expand { display:flex; gap:24px; padding:10px 24px 14px; background:var(--dns-bg-page); }
.history-expand-section { flex:1; min-width:0; }
.history-expand-section h4 { font-size:12px; font-weight:700; text-transform:uppercase; letter-spacing:0.05em; color:var(--app-text-secondary); margin:0; }
.history-empty { color:var(--dns-text-assist-color); margin:6px 0 0; font-size:12px; }
.history-pager { display:flex; justify-content:flex-end; margin-top:14px; }

/* Cluster-not-initialized empty state */
.empty-bootstrap {
  display:flex; align-items:center; gap:18px;
  padding:24px 28px; border-radius:12px;
  border:1px dashed var(--app-accent-muted);
  background:var(--app-accent-soft);
}
.empty-bootstrap-icon { width:48px; height:48px; flex-shrink:0; display:flex; align-items:center; justify-content:center; border-radius:50%; background:#fff; color:var(--app-accent); }
.empty-bootstrap-icon svg { width:24px; height:24px; }
.empty-bootstrap-body { flex:1; min-width:0; }
.empty-bootstrap-body h3 { margin:0 0 4px; font-size:15px; color:var(--dns-text-title-color); }
.empty-bootstrap-body p { margin:0; font-size:13px; color:var(--dns-text-body-color); line-height:1.6; }
.empty-bootstrap-link { flex-shrink:0; }

.diff-table { border:1px solid var(--dns-border-color); border-radius:8px; overflow:hidden; font-size:13px; }
.diff-thead {
  display:grid; grid-template-columns:1fr 1.2fr 1.2fr;
  padding:8px 12px; background:var(--dns-bg-page);
  font-size:11px; font-weight:700; text-transform:uppercase;
  letter-spacing:0.05em; color:var(--dns-text-assist-color);
  border-bottom:1px solid var(--dns-border-color);
}
.diff-row {
  display:grid; grid-template-columns:1fr 1.2fr 1.2fr;
  padding:9px 12px; border-bottom:1px solid var(--dns-border-color);
  align-items:center; gap:8px;
}
.diff-row:last-child { border-bottom:none; }
.diff-row--changed { background:rgba(255,125,0,0.04); }
.diff-key { font-size:12px; font-weight:600; color:var(--dns-text-title-color); }
.diff-val { font-family:'JetBrains Mono','Consolas',monospace; font-size:11px; padding:2px 8px; border-radius:4px; }
.diff-val--master  { background:rgba(0,180,42,0.08); color:var(--app-success); }
.diff-val--same    { background:rgba(0,180,42,0.08); color:var(--app-success); }
.diff-val--changed { background:rgba(245,63,63,0.08); color:var(--app-danger); }
</style>
