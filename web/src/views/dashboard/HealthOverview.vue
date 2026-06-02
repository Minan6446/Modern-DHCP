<template>
  <el-card shadow="hover" class="panel surface-card">
    <div class="panel-header">{{ t('dashboard.serverHealthTitle') }}</div>
    <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12">
      <template #actions>
        <el-button size="small" :loading="loading" @click="fetchData">{{
          t('common.retry')
        }}</el-button>
      </template>
    </AppErrorCallout>
    <el-empty v-else-if="!canView" :description="t('common.noPermission')" />
    <el-skeleton v-else-if="loading" animated :rows="4" />
    <el-empty v-else-if="!snapshot" :description="t('dashboard.empty')">
      <template #extra>
        <el-button size="small" @click="fetchData">{{ t('common.retry') }}</el-button>
      </template>
    </el-empty>
    <div v-else class="health-grid">
      <div class="health-card">
        <div class="label">{{ t('dashboard.cpuUsage') }}</div>
        <el-progress
          type="dashboard"
          :percentage="cpuUsage"
          :color="progressColor(cpuUsage)"
          :format="formatCpu"
        />
        <div class="value total">{{ t('dashboard.cpuTotalSub', { value: snapshot?.cpuCores ?? '--' }) }}</div>
      </div>
      <div class="health-card">
        <div class="label">{{ t('dashboard.memoryUsage') }}</div>
        <el-progress
          type="dashboard"
          :percentage="memoryUsage"
          :color="progressColor(memoryUsage)"
          :format="formatMemory"
        />
        <div class="value total">{{ t('dashboard.memoryTotalSub', { value: formatBytes(memoryTotal) }) }}</div>
      </div>
      <div class="health-card">
        <div class="label">{{ t('dashboard.diskUsage') }}</div>
        <el-progress
          type="dashboard"
          :percentage="diskUsage"
          :color="progressColor(diskUsage)"
          :format="formatDisk"
        />
        <div class="value total">{{ t('dashboard.diskTotalSub', { value: formatBytes(diskTotal) }) }}</div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue';
import { getRealtimeSnapshot } from '@/api/monitoring';
import type { PerfSnapshot } from '@/types/monitoring';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { createInlineError, getApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';

const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const loading = ref(false);
const snapshot = ref<PerfSnapshot | null>(null);
const pageError = ref<ApiErrorDescriptor | null>(null);
const canView = computed(() => permissionStore.can('monitoring.view'));
const clampPercent = (value: number) => Math.min(100, Math.max(0, value));
const cpuUsage = computed(() => clampPercent(snapshot.value?.cpu ?? 0));
const memoryUsage = computed(() => clampPercent(snapshot.value?.memory ?? 0));
const diskUsage = computed(() => clampPercent(snapshot.value?.disk ?? 0));

const formatCpu = () => `${cpuUsage.value.toFixed(1)}%`;
const formatMemory = () => `${memoryUsage.value.toFixed(1)}%`;
const formatDisk = () => `${diskUsage.value.toFixed(1)}%`;

const formatBytes = (bytes: number | undefined) => {
  if (!bytes || bytes <= 0) return '--';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx += 1;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[idx]}`;
};

const memoryTotal = computed(() => {
  const total = snapshot.value?.memoryTotalBytes;
  if (total && total > 0) return total;
  const used = snapshot.value?.memoryUsedBytes;
  const percent = snapshot.value?.memory ?? 0;
  if (used && percent > 0) return Math.round((used * 100) / percent);
  return undefined;
});

const diskTotal = computed(() => {
  const total = snapshot.value?.diskTotalBytes;
  if (total && total > 0) return total;
  const used = snapshot.value?.diskUsedBytes;
  const percent = snapshot.value?.disk ?? 0;
  if (used && percent > 0) return Math.round((used * 100) / percent);
  return undefined;
});

const progressColor = (value: number) => {
  if (value >= 85) return '#ef4444';
  if (value >= 70) return '#f59e0b';
  return '#22c55e';
};

const toErrorDescriptor = (err: unknown) => {
  const mapped = getApiError(err);
  if (mapped) return mapped;
  if (err instanceof Error && err.message) {
    return createInlineError(err.message);
  }
  return createInlineError(t('common.loadFail'));
};

const fetchData = async () => {
  if (!canView.value) return;
  loading.value = true;
  pageError.value = null;
  try {
    const { data } = await getRealtimeSnapshot({ tenantId: tenantStore.currentTenantId });
    snapshot.value = data.data || null;
  } catch (err: unknown) {
    pageError.value = toErrorDescriptor(err);
  } finally {
    loading.value = false;
  }
};

onMounted(fetchData);
watch(
  () => tenantStore.currentTenantId,
  () => fetchData()
);
</script>

<style scoped>
.panel-header {
  font-weight: 600;
  margin-bottom: 12px;
}

.health-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.health-card {
  background: var(--el-fill-color-light);
  padding: 12px;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: center;
  justify-content: center;
  min-height: 220px;
}

.label {
  color: var(--el-text-color-secondary);
}

.value {
  font-size: 20px;
  font-weight: 700;
}

.total {
  font-size: 16px;
  font-weight: 600;
}

:deep(.el-progress__text) {
  font-size: 16px;
  font-weight: 600;
}

@media (max-width: 1200px) {
  .health-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .health-grid {
    grid-template-columns: 1fr;
  }
}
</style>
