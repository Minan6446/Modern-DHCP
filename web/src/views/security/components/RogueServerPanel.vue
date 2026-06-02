<template>
  <el-card v-loading="loading" shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.rogueTitle') }}</span>
        <el-button size="small" :disabled="!canView || loading" @click="refresh">{{
          t('security.refresh')
        }}</el-button>
      </div>
    </template>
    <el-alert v-if="error" type="error" :closable="false" :title="error" class="mb-2" />
    <el-row :gutter="12">
      <el-col :span="14">
        <el-table :data="records" size="small" border height="240">
          <el-table-column prop="ip" label="IP" width="140" />
          <el-table-column prop="mac" label="MAC" width="160" />
          <el-table-column prop="vlan" label="VLAN" width="80" />
          <el-table-column prop="severity" :label="t('security.comp.rogueColSeverity')" width="100" />
          <el-table-column :label="t('security.action')" width="160">
            <template #default="{ row }">
              <el-button link type="primary" :disabled="!canManage" @click="quarantine(row)">{{
                t('security.quarantine')
              }}</el-button>
              <el-button link type="danger" :disabled="!canManage" @click="block(row)">{{
                t('security.block')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-col>
      <el-col :span="10">
        <div class="panel-title">{{ t('security.attackAnalysis') }}</div>
        <div class="chart-wrapper">
          <base-e-chart v-if="chartOption" :option="chartOption" />
        </div>
        <div class="panel-title">{{ t('security.actions') }}</div>
        <el-space wrap>
          <el-button type="primary" size="small" :disabled="!canManage">{{
            t('security.blockMac')
          }}</el-button>
          <el-button type="danger" size="small" :disabled="!canManage">{{
            t('security.pushAcl')
          }}</el-button>
          <el-button size="small" :disabled="!canManage">{{ t('security.sendAlert') }}</el-button>
        </el-space>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { blockRogue, listRogueServers, quarantineRogue } from '@/api/security';
import type { RogueServerRecord } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { ElMessageBox } from 'element-plus';
import { showHttpError } from '@/shared/errors/errorToast';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';

const records = ref<RogueServerRecord[]>([]);
const loading = ref(false);
const error = ref('');
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();
const canView = computed(() => permissionStore.can('security.view'));
const canManage = computed(() => permissionStore.can('security.manage'));

const refresh = async () => {
  if (!canView.value) {
    error.value = t('security.noPermissionView');
    records.value = [];
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const { data } = await listRogueServers({ tenantId: tenantStore.currentTenantId });
    records.value = data.data;
  } catch (e) {
    error.value = t('security.loadFailRogue');
    records.value = [];
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

const quarantine = async (row: RogueServerRecord) => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('security.quarantineConfirm'), t('common.confirm'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  try {
    await quarantineRogue(row.id, { tenantId: tenantStore.currentTenantId });
    showSuccess(t('security.quarantined'));
    refresh();
  } catch (e) {
    showHttpError(error, t('security.saveFail'));
  }
};

const block = (row: RogueServerRecord) => {
  if (!canManage.value) {
    showError(t('security.noPermissionSave'));
    return;
  }
  ElMessageBox.confirm(t('security.blockConfirm'), t('common.confirm'), { type: 'warning' })
    .then(async () => {
      try {
        await blockRogue(row.id, { tenantId: tenantStore.currentTenantId });
        showSuccess(t('security.blocked'));
        refresh();
      } catch (e) {
        showHttpError(error, t('security.saveFail'));
      }
    })
    .catch(() => {});
};

const chartOption = computed(() => {
  if (!records.value.length) return null;
  return {
    title: { text: t('security.comp.rogueChartTitle'), left: 'center' },
    tooltip: {},
    radar: {
      indicator: [
        { name: t('security.comp.rogueRadarOffer'), max: 100 },
        { name: t('security.comp.rogueRadarCrossVlan'), max: 100 },
        { name: t('security.comp.rogueRadarSpoofMac'), max: 100 },
        { name: t('security.comp.rogueRadarTrafficSpike'), max: 100 },
        { name: t('security.comp.rogueRadarAnomalyOpt'), max: 100 }
      ]
    },
    series: [
      {
        type: 'radar',
        data: [{ value: [80, 60, 90, 70, 65], name: t('security.comp.rogueRadarScore') }]
      }
    ]
  };
});

onMounted(refresh);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.mb-2 {
  margin-bottom: 8px;
}

.panel-title {
  font-weight: 600;
  margin: 4px 0;
}

.chart-wrapper {
  height: 180px;
}
</style>
