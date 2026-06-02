<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  getClusterStateApi,
  initializeClusterApi,
  deleteClusterApi,
  resyncClusterApi,
  dispatchClusterCommandApi,
  rotateClusterTokenApi,
  type ClusterStatePayload,
  type InitializeClusterResult,
} from '@/api/cluster'

const { t } = useI18n()

const state = ref<ClusterStatePayload | null>(null)
const loading = ref(false)
const submitting = ref(false)

const fetchState = async (): Promise<void> => {
  loading.value = true
  try {
    const res = await getClusterStateApi()
    state.value = res.data ?? null
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(fetchState)

const initialized = computed(() => Boolean(state.value?.initialized))

// ── Initialize dialog ──
const initVisible = ref(false)
const initForm = ref({
  clusterDomain: '',
  primaryNodeIpAddresses: '',
  heartbeatIntervalSec: 5,
  configRefreshSec: 30,
})
const initResult = ref<InitializeClusterResult | null>(null)

const openInit = (): void => {
  initForm.value = {
    clusterDomain: '',
    primaryNodeIpAddresses: '',
    heartbeatIntervalSec: 5,
    configRefreshSec: 30,
  }
  initResult.value = null
  initVisible.value = true
}

const submitInit = async (): Promise<void> => {
  const ips = initForm.value.primaryNodeIpAddresses
    .split(/[,\s]+/)
    .map((v) => v.trim())
    .filter(Boolean)
  if (!initForm.value.clusterDomain || ips.length === 0) {
    ElMessage.warning(t('cluster.bootstrap.requireDomainAndIps'))
    return
  }
  submitting.value = true
  try {
    const res = await initializeClusterApi({
      clusterDomain: initForm.value.clusterDomain,
      primaryNodeIpAddresses: ips,
      heartbeatIntervalSec: initForm.value.heartbeatIntervalSec,
      configRefreshSec: initForm.value.configRefreshSec,
    })
    initResult.value = res.data ?? null
    await fetchState()
    ElMessage.success(t('cluster.bootstrap.initialized'))
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const copyToken = async (): Promise<void> => {
  if (!initResult.value?.apiToken) return
  try {
    await navigator.clipboard.writeText(initResult.value.apiToken)
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.warning(t('common.copyFailed'))
  }
}

// ── Resync ──
const resync = async (): Promise<void> => {
  await ElMessageBox.confirm(t('cluster.bootstrap.resyncConfirm'), t('cluster.bootstrap.resyncTitle'), {
    type: 'warning',
  })
  submitting.value = true
  try {
    const res = await resyncClusterApi()
    const failed = (res.data?.results ?? []).filter((r) => !r.ok).length
    if (failed === 0) {
      ElMessage.success(t('cluster.bootstrap.resyncOk', { v: res.data?.configVersion ?? '' }))
    } else {
      ElMessage.warning(t('cluster.bootstrap.resyncPartial', { failed }))
    }
    await fetchState()
  } catch (err: unknown) {
    if (err !== 'cancel') ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// ── Rotate cluster API token ──
//
// Generates a new cluster token and shows it in a one-time alert. The
// previous token keeps verifying for graceSec (default 60s) so existing
// secondaries can pull the fresh value during their next heartbeat
// without dropping the cluster. Operators must propagate the new token
// to any peer that doesn't auto-refresh.
const rotateToken = async (): Promise<void> => {
  await ElMessageBox.confirm(
    t('cluster.bootstrap.rotateTokenConfirm'),
    t('cluster.bootstrap.rotateTokenTitle'),
    { type: 'warning', confirmButtonText: t('cluster.bootstrap.rotateTokenConfirmBtn'), cancelButtonText: t('common.cancel') }
  )
  submitting.value = true
  try {
    const res = await rotateClusterTokenApi(60)
    const tok = res.data?.apiToken ?? ''
    const grace = res.data?.graceSec ?? 60
    await ElMessageBox.alert(
      `<div style="word-break:break-all;font-family:monospace;background:rgba(22,93,255,0.06);padding:12px;border-radius:6px;margin-bottom:12px">${tok}</div>
       <p style="font-size:13px;color:#4E5969">${t('cluster.bootstrap.rotateTokenReminder', { grace })}</p>`,
      t('cluster.bootstrap.rotateTokenDoneTitle'),
      { dangerouslyUseHTMLString: true, confirmButtonText: t('common.ok') }
    )
  } catch (err: unknown) {
    if (err !== 'cancel') ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// ── Delete cluster ──
const teardown = async (): Promise<void> => {
  await ElMessageBox.confirm(t('cluster.bootstrap.deleteConfirm'), t('cluster.bootstrap.deleteTitle'), {
    type: 'warning',
    confirmButtonText: t('common.confirm'),
    cancelButtonText: t('common.cancel'),
  })
  submitting.value = true
  try {
    await deleteClusterApi(true)
    ElMessage.success(t('cluster.bootstrap.deleted'))
    await fetchState()
  } catch (err: unknown) {
    if (err !== 'cancel') ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// ── Cluster commands ──
const forceUpdate = async (): Promise<void> => {
  submitting.value = true
  try {
    const res = await dispatchClusterCommandApi('forceUpdateBlockLists')
    ElMessage.success(t('cluster.bootstrap.commandResult', { ok: res.data?.success ?? 0, fail: res.data?.failed ?? 0 }))
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const tempDisableMinutes = ref(5)
const tempDisable = async (): Promise<void> => {
  if (tempDisableMinutes.value <= 0 || tempDisableMinutes.value > 1440) {
    ElMessage.warning(t('cluster.bootstrap.invalidMinutes'))
    return
  }
  submitting.value = true
  try {
    const res = await dispatchClusterCommandApi('temporaryDisableBlocking', { minutes: tempDisableMinutes.value })
    ElMessage.success(t('cluster.bootstrap.commandResult', { ok: res.data?.success ?? 0, fail: res.data?.failed ?? 0 }))
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || t('common.saveFailed'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-card class="cluster-control" v-loading="loading">
    <div class="cc-header">
      <div>
        <h2 class="cc-title">{{ t('cluster.bootstrap.title') }}</h2>
        <p class="cc-subtitle" v-if="initialized">
          <span class="cc-tag cc-tag--ok">{{ t('cluster.bootstrap.online') }}</span>
          {{ state?.clusterDomain }} · {{ t('cluster.bootstrap.version') }} {{ state?.configVersion }}
        </p>
        <p class="cc-subtitle" v-else>
          <span class="cc-tag cc-tag--off">{{ t('cluster.bootstrap.offline') }}</span>
          {{ t('cluster.bootstrap.notInitializedHint') }}
        </p>
      </div>
      <div class="cc-actions">
        <template v-if="!initialized">
          <el-button type="primary" @click="openInit">{{ t('cluster.bootstrap.initBtn') }}</el-button>
        </template>
        <template v-else>
          <el-button @click="resync" :loading="submitting">{{ t('cluster.bootstrap.resyncBtn') }}</el-button>
          <el-button @click="forceUpdate" :loading="submitting">{{ t('cluster.bootstrap.forceUpdateBtn') }}</el-button>
          <el-input-number v-model="tempDisableMinutes" :min="1" :max="1440" size="small" class="cc-min-input" />
          <el-button @click="tempDisable" :loading="submitting">{{ t('cluster.bootstrap.tempDisableBtn') }}</el-button>
          <el-button @click="rotateToken" :loading="submitting">{{ t('cluster.bootstrap.rotateTokenBtn') }}</el-button>
          <el-button type="danger" plain @click="teardown" :loading="submitting">{{ t('cluster.bootstrap.deleteBtn') }}</el-button>
        </template>
      </div>
    </div>

    <div v-if="initialized && state" class="cc-stat-grid">
      <div class="cc-stat">
        <div class="cc-stat-label">{{ t('cluster.bootstrap.heartbeat') }}</div>
        <div class="cc-stat-value">{{ state.heartbeatIntervalSec }}s</div>
      </div>
      <div class="cc-stat">
        <div class="cc-stat-label">{{ t('cluster.bootstrap.refresh') }}</div>
        <div class="cc-stat-value">{{ state.configRefreshSec }}s</div>
      </div>
      <div class="cc-stat">
        <div class="cc-stat-label">{{ t('cluster.bootstrap.nodeCount') }}</div>
        <div class="cc-stat-value">{{ state.nodes.length }}</div>
      </div>
      <div class="cc-stat">
        <div class="cc-stat-label">{{ t('cluster.bootstrap.connected') }}</div>
        <div class="cc-stat-value">{{ state.nodes.filter((n) => n.state === 'Connected' || n.state === 'Self').length }}</div>
      </div>
    </div>

    <!-- Init dialog -->
    <el-dialog v-model="initVisible" :title="t('cluster.bootstrap.initTitle')" width="540px" append-to-body>
      <template v-if="!initResult">
        <el-form label-position="top">
          <el-form-item :label="t('cluster.bootstrap.clusterDomain')" required>
            <el-input v-model="initForm.clusterDomain" placeholder="cluster.example.com" />
          </el-form-item>
          <el-form-item :label="t('cluster.bootstrap.primaryIps')" required>
            <el-input v-model="initForm.primaryNodeIpAddresses" type="textarea" :rows="3"
              placeholder="10.0.0.10, 10.0.0.11" />
            <div class="cc-form-hint">{{ t('cluster.bootstrap.primaryIpsHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('cluster.bootstrap.heartbeat')">
            <el-input-number v-model="initForm.heartbeatIntervalSec" :min="1" :max="600" />
          </el-form-item>
          <el-form-item :label="t('cluster.bootstrap.refresh')">
            <el-input-number v-model="initForm.configRefreshSec" :min="5" :max="3600" />
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <el-alert :title="t('cluster.bootstrap.tokenWarning')" type="warning" :closable="false" show-icon
          style="margin-bottom: 12px" />
        <div class="cc-token-block">
          <div class="cc-token-label">{{ t('cluster.bootstrap.apiToken') }}</div>
          <div class="cc-token-value">{{ initResult.apiToken }}</div>
        </div>
        <el-button size="small" @click="copyToken">{{ t('common.copy') }}</el-button>
      </template>
      <template #footer>
        <el-button @click="initVisible = false">{{ initResult ? t('common.close') : t('common.cancel') }}</el-button>
        <el-button v-if="!initResult" type="primary" :loading="submitting" @click="submitInit">{{ t('cluster.bootstrap.confirmInit') }}</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<style scoped>
.cluster-control {
  margin-bottom: 16px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
}
.cc-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; flex-wrap: wrap; }
.cc-title { margin: 0; font-size: 16px; font-weight: 600; color: var(--app-title); }
.cc-subtitle { margin: 4px 0 0 0; font-size: 12px; color: var(--app-text-regular); display: flex; align-items: center; gap: 8px; }
.cc-tag { padding: 1px 8px; border-radius: 10px; font-size: 11px; font-weight: 500; }
.cc-tag--ok { background: rgba(0, 180, 42, 0.1); color: var(--app-success); }
.cc-tag--off { background: rgba(78, 89, 105, 0.08); color: var(--app-text-secondary); }
.cc-actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.cc-min-input { width: 100px; }
.cc-stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-top: 16px; }
.cc-stat { padding: 10px 14px; border: 1px solid var(--app-border); border-radius: 8px; background: var(--app-bg-secondary); }
.cc-stat-label { font-size: 11px; color: var(--app-text-regular); }
.cc-stat-value { font-size: 20px; font-weight: 700; color: var(--app-title); margin-top: 4px; font-variant-numeric: tabular-nums; }
.cc-form-hint { font-size: 11px; color: var(--app-text-regular); margin-top: 4px; }
.cc-token-block { padding: 12px; background: var(--app-bg-secondary); border: 1px dashed var(--app-border); border-radius: 6px; margin-bottom: 8px; }
.cc-token-label { font-size: 11px; color: var(--app-text-regular); }
.cc-token-value { font-family: var(--app-font-mono, 'Consolas', monospace); font-size: 12px; word-break: break-all; margin-top: 6px; }
</style>
