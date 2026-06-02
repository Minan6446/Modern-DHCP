<template>
  <el-card shadow="hover" class="panel surface-card">
    <div class="panel-header row">
      <span>{{ t('dashboard.realtimeTitle') }}</span>
      <div class="status">
        <el-tag size="small" :type="statusType">{{ statusText }}</el-tag>
        <el-button size="small" text :loading="connecting" @click="reconnect">{{
          t('dashboard.reconnect')
        }}</el-button>
      </div>
    </div>
    <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12">
      <template #actions>
        <el-button size="small" :loading="connecting" @click="reconnect">{{
          t('common.retry')
        }}</el-button>
      </template>
    </AppErrorCallout>
    <el-empty v-else-if="!canView" :description="t('common.noPermission')" />
    <el-skeleton v-else-if="loading" animated :rows="4" />
    <template v-else>
      <el-empty v-if="isEmpty" :description="t('dashboard.empty')">
        <template #extra>
          <el-button size="small" @click="reconnect">{{ t('common.retry') }}</el-button>
        </template>
      </el-empty>
      <div v-else class="real-grid">
        <div class="real-item list">
          <div class="title">{{ t('dashboard.statusStream') }}</div>
          <el-scrollbar height="200px">
            <ul>
              <li v-for="evt in events" :key="evt.id">
                {{ evt.ip }} → {{ evt.mac }} ({{ evt.state }})
              </li>
            </ul>
          </el-scrollbar>
        </div>
        <div class="real-item">
          <div class="title">{{ t('dashboard.stateFlow') }}</div>
          <BaseEChart :option="stateOption" />
        </div>
        <div class="real-item">
          <div class="title">{{ t('dashboard.alertCenter') }}</div>
          <el-timeline>
            <el-timeline-item v-for="a in alerts" :key="a.id" :type="a.type" :timestamp="a.time">{{
              a.msg
            }}</el-timeline-item>
          </el-timeline>
        </div>
        <div class="real-item list">
          <div class="title">{{ t('dashboard.auditLog') }}</div>
          <el-scrollbar height="200px">
            <ul>
              <li v-for="log in logs" :key="log.id">
                {{ log.user }} {{ log.action }} @ {{ log.time }}
              </li>
            </ul>
          </el-scrollbar>
        </div>
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { useWs } from '@/utils/ws';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { createInlineError, getApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const events = ref<{ id: string; ip: string; mac: string; state: string }[]>([]);
const alerts = ref<
  { id: string; msg: string; type: 'success' | 'warning' | 'danger'; time: string }[]
>([]);
const logs = ref<{ id: string; user: string; action: string; time: string }[]>([]);
const loading = ref(false);
const pageError = ref<ApiErrorDescriptor | null>(null);
const wsState = ref<'idle' | 'connecting' | 'open' | 'closed' | 'error'>('closed');
let disposer: (() => void) | null = null;

const isEmpty = computed(() => !events.value.length && !alerts.value.length && !logs.value.length);
const connecting = computed(() => wsState.value === 'connecting');
const statusType = computed(() =>
  wsState.value === 'open' ? 'success' : wsState.value === 'error' ? 'danger' : 'info'
);
const statusText = computed(() => {
  if (wsState.value === 'open') return 'Live';
  if (wsState.value === 'connecting') return t('dashboard.loading');
  if (wsState.value === 'error') return t('dashboard.wsError');
  return t('dashboard.wsClosed');
});

const stateOption = {
  tooltip: {},
  series: [
    {
      type: 'graph',
      layout: 'circular',
      data: [
        { name: 'DISCOVER' },
        { name: 'OFFER' },
        { name: 'REQUEST' },
        { name: 'ACK' },
        { name: 'BOUND' },
        { name: 'RENEW' },
        { name: 'EXPIRE' }
      ],
      links: [
        { source: 'DISCOVER', target: 'OFFER' },
        { source: 'OFFER', target: 'REQUEST' },
        { source: 'REQUEST', target: 'ACK' },
        { source: 'ACK', target: 'BOUND' },
        { source: 'BOUND', target: 'RENEW' },
        { source: 'BOUND', target: 'EXPIRE' }
      ],
      roam: true,
      label: { show: true }
    }
  ]
};

const maxItems = 50;
const canView = computed(() => permissionStore.can('monitoring.view'));

const toErrorDescriptor = (err: unknown) => {
  const mapped = getApiError(err);
  if (mapped) return mapped;
  if (err instanceof Error && err.message) {
    return createInlineError(err.message);
  }
  return createInlineError(t('common.loadFail'));
};

const mapSeverityTag = (severity?: string) => {
  const value = (severity || '').toLowerCase();
  if (value.includes('critical') || value.includes('error') || value.includes('danger') || value.includes('fail')) {
    return 'danger';
  }
  if (value.includes('warn')) return 'warning';
  return 'success';
};

const applyEntries = (items: any[]) => {
  if (!Array.isArray(items)) return;
  items.forEach((entry) => {
    const time = entry?.occurredAt || entry?.generatedAt || '';
    if (entry?.type === 'alert') {
      alerts.value.unshift({
        id: entry.id,
        msg: entry.summary,
        type: mapSeverityTag(entry.severity),
        time
      });
      if (alerts.value.length > maxItems) alerts.value.length = maxItems;
    } else {
      const fallbackId = entry?.id || `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      logs.value.unshift({
        id: fallbackId,
        user: entry?.source || entry?.metadata?.actor || t('dashboard.unknown'),
        action: entry?.summary || t('dashboard.empty'),
        time
      });
      if (logs.value.length > maxItems) logs.value.length = maxItems;
    }

    const ip = entry?.metadata?.ip || entry?.metadata?.clientIp || entry?.metadata?.address;
    const mac = entry?.metadata?.mac || entry?.metadata?.clientMac;
    if (ip || mac) {
      events.value.unshift({ id: entry.id, ip: ip || '-', mac: mac || '-', state: entry?.severity || entry?.source || entry?.type });
      if (events.value.length > maxItems) events.value.length = maxItems;
    }
  });
};

function bindWs() {
  if (!canView.value) {
    loading.value = false;
    return;
  }
  loading.value = true;
  disposer?.();
  const base = import.meta.env.VITE_WS_BASE ?? 'wss://example.com/api/v1';
  const url = `${base}/dashboard/streams/live?limit=${maxItems}`;
  const ws = useWs({ url, heartbeatMs: 15000, reconnect: true, maxRetry: 5, pauseOnHidden: true });
  wsState.value = 'connecting';
  pageError.value = null;
  ws.onMessage((evt) => {
    wsState.value = 'open';
    loading.value = false;
    try {
      const msg = JSON.parse(evt.data);
      if (msg.type === 'stream.snapshot' && msg.snapshot?.items) {
        events.value = [];
        alerts.value = [];
        logs.value = [];
        applyEntries(msg.snapshot.items);
      } else if (msg.type === 'stream.delta' && msg.items) {
        applyEntries(msg.items);
      }
    } catch (e) {
      // ignore parse errors
    }
  });
  ws.onError((err) => {
    wsState.value = 'error';
    loading.value = false;
    pageError.value = toErrorDescriptor(err);
  });
  disposer = () => ws.close();
}

function reconnect() {
  bindWs();
}

onMounted(bindWs);
onBeforeUnmount(() => disposer?.());

watch(
  () => tenantStore.currentTenantId,
  () => {
    events.value = [];
    alerts.value = [];
    logs.value = [];
    bindWs();
  }
);
</script>

<style scoped>
.panel-header {
  font-weight: 600;
  margin-bottom: 12px;
}

.panel-header.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.real-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.real-item {
  background: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 12px;
  min-height: 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.real-item.list ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.real-item.list li {
  padding: 6px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
}

.title {
  font-weight: 600;
}
</style>
