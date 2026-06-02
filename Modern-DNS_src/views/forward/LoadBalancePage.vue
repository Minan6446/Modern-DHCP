<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getLbGroups, createLbGroup, deleteLbGroup, createLbServer, updateLbServer, deleteLbServer, toggleLbServer, lbHealthCheck } from '../../api/forward'
import StatusPill from '../../components/StatusPill.vue'

interface UpstreamServer {
  id: number
  name: string
  address: string
  port: number
  protocol: 'UDP' | 'TCP' | 'DoT' | 'DoH'
  weight: number
  maxConns: number
  latency: number
  status: '健康' | '异常' | '检测中'
  enabled: boolean
  successRate: number
  // Most-recent probe failure reason; empty when last probe was OK.
  // Surfaced via tooltip on the status pill so admins see *why*
  // an upstream is 异常 (TLS / HTTP 4xx / timeout) without log diving.
  lastError?: string
}

interface LbGroup {
  id: number
  name: string
  algorithm: '轮询' | '加权轮询' | '最小延迟' | '随机' | 'IP哈希'
  healthCheckInterval: number
  servers: UpstreamServer[]
  status: '启用' | '禁用'
  qps: number
}

const { t } = useI18n()

const groups = ref<LbGroup[]>([])

const loading = ref(false)
const groupDialogVisible = ref(false)
const serverDialogVisible = ref(false)
const activeGroupId = ref<number | null>(null)
const submitting = ref(false)

const activeGroup = computed(() => groups.value.find(g => g.id === activeGroupId.value) ?? null)

const groupForm = reactive({ name: '', algorithm: '轮询' as LbGroup['algorithm'], healthCheckInterval: 10, status: '启用' as '启用'|'禁用' })
const serverForm = reactive({ name: '', address: '', port: 53, protocol: 'UDP' as UpstreamServer['protocol'], weight: 1, maxConns: 500 })
// editingServerId !== null means the dialog is in edit mode and submit
// will hit PUT /lb/servers/:sid instead of POST /lb/groups/:gid/servers.
// Reusing the same dialog avoids form duplication and keeps validation
// rules co-located.
const editingServerId = ref<number | null>(null)
// Per-row toggle loading guard. Without this, fast double-clicks can
// fire two POSTs that race; backend wins the second but the el-switch
// optimistic v-model would already have flipped twice in the UI.
const togglingServerId = ref<number | null>(null)

const totalQps = computed(() => groups.value.filter(g => g.status === '启用').reduce((s, g) => s + g.qps, 0))
const totalServers = computed(() => groups.value.flatMap(g => g.servers).length)
const healthyServers = computed(() => groups.value.flatMap(g => g.servers).filter(s => s.status === '健康').length)

const fetchGroups = async () => {
  loading.value = true
  try {
    const { data } = await getLbGroups()
    groups.value = data ?? []
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchGroups())

const openGroupDialog = () => {
  Object.assign(groupForm, { name: '', algorithm: '轮询', healthCheckInterval: 10, status: '启用' })
  groupDialogVisible.value = true
}

const submitGroup = async () => {
  if (!groupForm.name) { ElMessage.warning(t('forward.requireGroupName')); return }
  submitting.value = true
  try {
    await createLbGroup({ ...groupForm })
    ElMessage.success(t('forward.groupCreated'))
    groupDialogVisible.value = false
    await fetchGroups()
  } finally {
    submitting.value = false
  }
}

const removeGroup = async (group: LbGroup) => {
  await ElMessageBox.confirm(t('forward.deleteConfirm', { name: group.name }), t('forward.deleteConfirmTitle'), { type: 'warning' })
  await deleteLbGroup(group.id)
  if (activeGroupId.value === group.id) activeGroupId.value = null
  ElMessage.success(t('forward.groupDeleted'))
  await fetchGroups()
}

const openServerDialog = (groupId: number) => {
  activeGroupId.value = groupId
  editingServerId.value = null
  Object.assign(serverForm, { name: '', address: '', port: 53, protocol: 'UDP', weight: 1, maxConns: 500 })
  serverDialogVisible.value = true
}

const openEditServerDialog = (groupId: number, server: UpstreamServer) => {
  activeGroupId.value = groupId
  editingServerId.value = server.id
  Object.assign(serverForm, {
    name: server.name,
    address: server.address,
    port: server.port,
    protocol: server.protocol,
    weight: server.weight,
    maxConns: server.maxConns,
  })
  serverDialogVisible.value = true
}

const submitServer = async () => {
  if (!serverForm.address) { ElMessage.warning(t('forward.requireUpstream')); return }
  const group = activeGroup.value
  if (!group) return
  submitting.value = true
  try {
    if (editingServerId.value !== null) {
      await updateLbServer(editingServerId.value, { ...serverForm })
      ElMessage.success(t('forward.nodeUpdated'))
    } else {
      await createLbServer(group.id, { ...serverForm })
      ElMessage.success(t('forward.nodeAdded'))
    }
    serverDialogVisible.value = false
    editingServerId.value = null
    await fetchGroups()
  } finally {
    submitting.value = false
  }
}

const onToggleServer = async (server: UpstreamServer, val: boolean) => {
  // The el-switch v-model has already mutated row.enabled to the new
  // value before this handler runs. We persist that intent; on failure
  // we roll back so the toggle reflects truth instead of misleading
  // the operator into thinking the change took effect.
  if (togglingServerId.value !== null) return
  togglingServerId.value = server.id
  try {
    await toggleLbServer(server.id, val)
    ElMessage.success(val ? t('common.enabled') : t('common.disabled'))
  } catch {
    server.enabled = !val
    ElMessage.error(t('forward.saveFailed'))
  } finally {
    togglingServerId.value = null
  }
}

const removeServer = async (group: LbGroup, server: UpstreamServer) => {
  await ElMessageBox.confirm(t('forward.removeConfirm', { address: server.address }), t('forward.removeConfirmTitle'), { type: 'warning' })
  await deleteLbServer(server.id)
  ElMessage.success(t('forward.nodeRemoved'))
  await fetchGroups()
}

const checkHealth = async (group: LbGroup) => {
  loading.value = true
  try {
    const { data } = await lbHealthCheck(group.id)
    const g = groups.value.find(g => g.id === group.id)
    if (g && data) g.servers = data as any
    ElMessage.success(t('forward.healthCheckDone'))
  } finally {
    loading.value = false
  }
}

const algorithmColor: Record<string, string> = { '轮询': '#3b82f6', '加权轮询': '#8b5cf6', '最小延迟': '#10b981', '随机': '#f59e0b', 'IP哈希': '#6b7280' }
// Map the 中文 health status enum to the StatusPill intent vocabulary.
// Centralised so callers don't repeat the ternary inline (which would
// drift over time as new states like "未检测" or "超时" get added).
const healthIntent = (s: string): 'success' | 'warning' | 'danger' | 'neutral' => {
  if (s === '健康') return 'success'
  if (s === '检测中') return 'warning'
  if (s === '异常') return 'danger'
  return 'neutral'
}

const protocolLabelMap: Record<string, string> = {
  UDP: 'forward.lbProtocolUdp',
  TCP: 'forward.lbProtocolTcp',
  DoT: 'forward.lbProtocolDot',
  DoH: 'forward.lbProtocolDoh',
}

const protocolLabel = (protocol: string): string => {
  const key = protocolLabelMap[String(protocol || '')]
  return key ? t(key) : protocol
}

const formatProbeError = (raw: string | undefined): string => {
  const message = String(raw || '').trim()
  if (!message) {
    return ''
  }
  const lower = message.toLowerCase()

  if (lower.includes('i/o timeout') || lower.includes('timeout')) return t('forward.lbProbeTimeout')
  if (lower.includes('connection refused')) return t('forward.lbProbeConnRefused')
  if (lower.includes('no such host')) return t('forward.lbProbeNoSuchHost')
  if (lower.includes('network is unreachable')) return t('forward.lbProbeNetworkUnreachable')
  if (lower.includes('tls')) return t('forward.lbProbeTlsFailed')

  const httpMatch = message.match(/http\s*(\d{3})/i)
  if (httpMatch?.[1]) {
    return t('forward.lbProbeHttpError', { code: httpMatch[1] })
  }
  if (lower.includes('read udp')) return t('forward.lbProbeReadUdpFailed')

  return t('forward.lbProbeUnknown')
}
</script>

<template>
  <div class="page-shell">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ $t('forward.loadBalance') }}</h1>
        <p class="page-subtitle">{{ $t('forward.loadBalanceSubtitle') }}</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" @click="openGroupDialog">{{ $t('forward.newLbGroup') }}</el-button>
      </div>
    </div>

    <div class="mn-stats-strip">
      <div class="mn-stat-tile mn-stat-tile--accent"><span class="mn-stat-value">{{ totalQps.toLocaleString() }}</span><span class="mn-stat-label">{{ $t('forward.totalQps') }}</span></div>
      <div class="mn-stat-tile mn-stat-tile--success"><span class="mn-stat-value">{{ healthyServers }}</span><span class="mn-stat-label">{{ $t('forward.healthyNodes') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ totalServers }}</span><span class="mn-stat-label">{{ $t('forward.totalNodes') }}</span></div>
      <div class="mn-stat-tile"><span class="mn-stat-value">{{ groups.length }}</span><span class="mn-stat-label">{{ $t('forward.groupCount') }}</span></div>
    </div>

    <div class="lb-groups">
      <el-empty v-if="!loading && groups.length === 0" :description="$t('forward.noLbGroups')" />
      <el-card v-for="group in groups" :key="group.id" class="mn-card lb-group-card">
        <div class="lb-group-header">
          <div class="lb-group-title">
            <span class="lb-group-name">{{ group.name }}</span>
            <!-- Algorithm badge keeps a per-algorithm hue, so we still
                 inline a background color via the legacy .type-badge
                 class — StatusPill is for semantic status only. -->
            <span class="type-badge" :style="{ background: algorithmColor[group.algorithm] }">{{ group.algorithm }}</span>
            <StatusPill :intent="group.status === '启用' ? 'success' : 'neutral'">{{ group.status === '启用' ? $t('common.enabled') : $t('common.disabled') }}</StatusPill>
          </div>
          <div class="lb-group-actions">
            <span class="lb-group-qps">{{ group.qps.toLocaleString() }} QPS</span>
            <el-button size="small" :loading="loading" @click="checkHealth(group)">{{ $t('forward.healthCheck') }}</el-button>
            <el-button size="small" type="primary" @click="openServerDialog(group.id)">{{ $t('forward.addNode') }}</el-button>
            <el-button size="small" type="danger" plain @click="removeGroup(group)">{{ $t('forward.deleteGroup') }}</el-button>
          </div>
        </div>

        <el-table :data="group.servers" size="small" class="lb-server-table mn-table--compact">
          <el-table-column prop="name" :label="$t('forward.nodeName')" min-width="120" />
          <el-table-column prop="address" :label="$t('forward.address')" min-width="180">
            <template #default="{ row }"><span class="mn-mono">{{ row.address }}:{{ row.port }}</span></template>
          </el-table-column>
          <el-table-column prop="protocol" :label="$t('forward.protocol')" width="70" align="center">
            <template #default="{ row }"><StatusPill intent="neutral" size="xs">{{ protocolLabel(row.protocol) }}</StatusPill></template>
          </el-table-column>
          <el-table-column prop="weight" :label="$t('forward.weight')" width="70" align="center">
            <template #default="{ row }"><span class="mn-mono">{{ row.weight }}</span></template>
          </el-table-column>
          <el-table-column prop="latency" :label="$t('forward.latency')" width="90" align="right">
            <template #default="{ row }"><span class="mn-mono" :style="{ color: row.latency > 50 ? 'var(--app-warning)' : 'var(--app-success)' }">{{ row.latency }} ms</span></template>
          </el-table-column>
          <el-table-column prop="successRate" :label="$t('forward.successRate')" width="90" align="right">
            <template #default="{ row }"><span class="mn-mono" :style="{ color: row.successRate < 90 ? 'var(--app-danger)' : 'var(--app-success)' }">{{ row.successRate }}%</span></template>
          </el-table-column>
          <el-table-column prop="status" :label="$t('forward.status')" width="90" align="center">
            <template #default="{ row }">
              <!-- Show the probe failure reason on hover when the row
                   is in 异常. The tooltip stays out of the way for
                   healthy rows (lastError is empty) so the table
                   doesn't grow flicker-prone tooltip triggers. -->
              <el-tooltip v-if="row.lastError" :content="formatProbeError(row.lastError)" placement="top" effect="dark">
                <StatusPill :intent="healthIntent(row.status)" dot>{{ row.status === '健康' ? $t('forward.healthyNodes') : row.status === '异常' ? $t('common.error') : $t('forward.healthCheck') }}</StatusPill>
              </el-tooltip>
              <StatusPill v-else :intent="healthIntent(row.status)" dot>{{ row.status === '健康' ? $t('forward.healthyNodes') : row.status === '异常' ? $t('common.error') : $t('forward.healthCheck') }}</StatusPill>
            </template>
          </el-table-column>
          <el-table-column prop="enabled" :label="$t('forward.enabled')" width="70" align="center">
            <template #default="{ row }"><el-switch v-model="row.enabled" size="small" :loading="togglingServerId === row.id" :disabled="togglingServerId === row.id" @change="(val: boolean) => onToggleServer(row, val)" /></template>
          </el-table-column>
          <el-table-column :label="$t('common.operation')" width="140" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openEditServerDialog(group.id, row)">{{ $t('forward.edit') }}</el-button>
              <el-button link type="danger" size="small" @click="removeServer(group, row)">{{ $t('forward.remove') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>

    <!-- Group dialog -->
    <el-dialog v-model="groupDialogVisible" :title="$t('forward.newLbGroup')" width="440px" append-to-body>
      <el-form :model="groupForm" label-position="top">
        <el-form-item :label="$t('forward.groupName')"><el-input v-model="groupForm.name" :placeholder="$t('forward.lbGroup')" /></el-form-item>
        <el-form-item :label="$t('forward.lbAlgorithm')">
          <el-select v-model="groupForm.algorithm" style="width:100%">
            <el-option :label="$t('forward.roundRobin')" value="轮询" />
            <el-option :label="$t('forward.weightedRoundRobin')" value="加权轮询" />
            <el-option :label="$t('forward.leastLatency')" value="最小延迟" />
            <el-option :label="$t('forward.random')" value="随机" />
            <el-option :label="$t('forward.ipHash')" value="IP哈希" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('forward.healthCheckInterval')">
          <el-input-number v-model="groupForm.healthCheckInterval" :min="5" :max="300" controls-position="right" style="width:100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupDialogVisible = false">{{ $t('forward.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitGroup">{{ $t('forward.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- Server dialog: add or edit, driven by editingServerId -->
    <el-dialog v-model="serverDialogVisible" :title="editingServerId !== null ? $t('forward.editNode') : $t('forward.addNode')" width="440px" append-to-body>
      <el-form :model="serverForm" label-position="top">
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0 16px">
          <el-form-item :label="$t('forward.nodeName')"><el-input v-model="serverForm.name" :placeholder="$t('forward.nodeNamePlaceholder')" /></el-form-item>
          <el-form-item :label="$t('forward.protocol')">
            <el-select v-model="serverForm.protocol" style="width:100%">
              <el-option v-for="p in ['UDP','TCP','DoT','DoH']" :key="p" :label="protocolLabel(p)" :value="p" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('forward.upstreamAddress')"><el-input v-model="serverForm.address" :placeholder="$t('forward.upstreamPlaceholder')" /></el-form-item>
          <el-form-item :label="$t('forward.port')"><el-input-number v-model="serverForm.port" :min="1" :max="65535" controls-position="right" style="width:100%" /></el-form-item>
          <el-form-item :label="$t('forward.weightForWrr')"><el-input-number v-model="serverForm.weight" :min="1" :max="100" controls-position="right" style="width:100%" /></el-form-item>
          <el-form-item :label="$t('forward.maxConns')"><el-input-number v-model="serverForm.maxConns" :min="10" :max="10000" controls-position="right" style="width:100%" /></el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="serverDialogVisible = false">{{ $t('forward.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitServer">{{ editingServerId !== null ? $t('forward.save') : $t('forward.add') }}</el-button>
      </template>
    </el-dialog>
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
.lb-groups { display:flex; flex-direction:column; gap:16px; }
.lb-group-card { }
.lb-group-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:12px; flex-wrap:wrap; gap:8px; }
.lb-group-title { display:flex; align-items:center; gap:8px; }
.lb-group-name { font-size:15px; font-weight:700; color:var(--app-text-primary,#1d2129); }
.lb-group-actions { display:flex; align-items:center; gap:8px; }
.lb-group-qps { font-size:12px; color:var(--app-accent); font-weight:600; font-family:var(--app-font-mono,monospace); }
.lb-server-table { border:1px solid var(--app-border); border-radius:6px; overflow:hidden; }
/* .type-badge / .status-dot now provided globally in style.css. The
   former local declarations were removed when LoadBalance was migrated
   to <StatusPill>. */
</style>
