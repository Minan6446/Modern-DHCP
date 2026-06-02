<template>
  <el-card shadow="hover" class="panel surface-card">
    <div class="panel-header">{{ t('pool.monitorTitle') }}</div>
    <el-skeleton v-if="loading" animated :rows="4" />
    <template v-else>
      <el-empty v-if="isEmpty" :description="t('pool.monitorEmpty')" />
      <div v-else class="monitor-grid">
        <div class="chart-card">
          <div class="chart-title">{{ t('pool.usage') }}</div>
          <BaseEChart :option="usageOption" />
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('pool.history') }}</div>
          <el-table :data="history" height="220">
            <el-table-column prop="ts" :label="t('pool.historyTime')" />
            <el-table-column prop="action" :label="t('pool.historyAction')" />
            <el-table-column prop="actor" :label="t('pool.historyActor')" />
          </el-table>
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('pool.conflicts') }}</div>
          <el-alert v-if="conflicts.length === 0" type="success" :title="t('pool.noConflict')" />
          <el-timeline v-else>
            <el-timeline-item
              v-for="c in conflicts"
              :key="c.id"
              type="warning"
              :timestamp="c.identifierType"
            >
              {{ c.poolId }} · {{ c.ipAddress }} · {{ c.identifier }}
            </el-timeline-item>
          </el-timeline>
        </div>
        <div class="chart-card">
          <div class="chart-title">{{ t('pool.alerts') }}</div>
          <el-form label-position="top" class="warn-form">
            <el-form-item :label="t('pool.threshold')">
              <el-slider v-model="threshold" :min="50" :max="95" :step="5" show-input />
            </el-form-item>
            <el-form-item :label="t('pool.notify')">
              <el-switch v-model="notify" />
            </el-form-item>
          </el-form>
        </div>
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { getUsage, getHistory, getConflicts } from '@/api/pools';
import type { BindingConflict } from '@/types/pool';
import { showError } from '@/shared/errors/messageToast';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';

const props = defineProps<{ poolId: string; permissionKey?: string }>();

const usage = ref<{ ts: string[]; used: number[]; capacity: number[] }>({
  ts: [],
  used: [],
  capacity: []
});
const history = ref<{ id: string; action: string; actor: string; ts: string }[]>([]);
const conflicts = ref<BindingConflict[]>([]);
const threshold = ref(80);
const notify = ref(true);
const loading = ref(false);
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const isEmpty = computed(
  () => !usage.value.ts.length && !history.value.length && !conflicts.value.length
);

const usageOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: [t('pool.used'), t('pool.capacity')] },
  xAxis: { type: 'category', data: usage.value.ts },
  yAxis: { type: 'value' },
  series: [
    { type: 'line', name: t('pool.used'), areaStyle: {}, data: usage.value.used },
    { type: 'line', name: t('pool.capacity'), data: usage.value.capacity }
  ]
}));

const canFetch = computed(() => permissionStore.can(props.permissionKey || 'pool.ipv4.view'));

const fetch = async () => {
  if (!props.poolId || !canFetch.value) return;
  loading.value = true;
  try {
    const tenantId = tenantStore.currentTenantId || 'global';
    const [u, h, c] = await Promise.all([
      getUsage(props.poolId, { tenantId }),
      getHistory(props.poolId, { page: 1, pageSize: 20, tenantId }),
      getConflicts(props.poolId, { tenantId })
    ]);
    usage.value = (u as any).data?.data || { ts: [], used: [], capacity: [] };
    history.value = (h as any).data?.data?.items || [];
    conflicts.value = (c as any).data?.data?.items || [];
  } catch (e) {
    showError(t('pool.monitorFail'));
  } finally {
    loading.value = false;
  }
};

onMounted(fetch);

watch(
  () => [props.poolId, tenantStore.currentTenantId, canFetch.value],
  () => fetch()
);
</script>

<style scoped>
.panel-header {
  font-weight: 600;
  margin-bottom: 12px;
}

.monitor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.chart-card {
  background: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 12px;
  min-height: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chart-title {
  font-weight: 600;
}

.warn-form {
  max-width: 320px;
}
</style>
