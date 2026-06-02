<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('option.usageTitle') }}</span>
        <el-button
          size="small"
          type="primary"
          class="refresh-btn"
          :disabled="loading"
          @click="refresh"
          >{{ t('option.usageRefresh') }}</el-button
        >
      </div>
    </template>
    <el-row :gutter="12">
      <el-col :span="14">
        <div class="chart-wrapper">
          <el-skeleton v-if="loading && !usage.heat.length" :rows="6" animated />
          <base-e-chart v-else-if="heatOption" :option="heatOption" />
          <el-empty v-else :description="error || t('option.empty')" />
        </div>
      </el-col>
      <el-col :span="10">
        <el-table
          v-if="usage.relations.length"
          :data="usage.relations"
          height="220"
          size="small"
          border
        >
          <el-table-column prop="from" :label="t('option.relationFrom')" width="80" />
          <el-table-column prop="to" :label="t('option.relationTo')" width="80" />
          <el-table-column prop="weight" :label="t('option.columnDesc')" />
        </el-table>
        <el-empty v-else-if="!loading" :description="error || t('option.empty')" />
        <el-divider content-position="left">{{ t('option.columnDesc') }}</el-divider>
        <el-timeline class="history">
          <el-timeline-item
            v-for="item in usage.history"
            :key="item.timestamp + item.optionCode"
            :timestamp="item.timestamp"
          >
            {{ historyText(item) }}
          </el-timeline-item>
        </el-timeline>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { getOptionUsage } from '@/api/options';
import type { OptionUsageSnapshot } from '@/types/option';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import { showError } from '@/shared/errors/messageToast';

const usage = ref<OptionUsageSnapshot>({ heat: [], relations: [], history: [] });
const loading = ref(false);
const error = ref('');
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const fetch = async () => {
  if (!permissionStore.can('option.view')) {
    usage.value = { heat: [], relations: [], history: [] };
    error.value = t('option.noPermission');
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const { data } = await getOptionUsage({ tenantId: tenantStore.currentTenantId });
    usage.value = data.data;
  } catch (e) {
    usage.value = { heat: [], relations: [], history: [] };
    error.value = t('option.usageLoadFail');
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

const refresh = () => fetch();

const optionLabel = (code: number, name?: string) => {
  const key = `option.names.${code}`;
  const translated = t(key);
  if (translated !== key) return translated;
  if (name) return name;
  return t('option.optionCodeLabel', { code });
};

const actionLabel = (action: string) => {
  const key = `option.usageAction.${action}`;
  const translated = t(key);
  return translated === key ? action : translated;
};

const historyText = (item: OptionUsageSnapshot['history'][number]) => {
  const operator = item.operator ? t('option.usageBy', { operator: item.operator }) : '';
  const label = optionLabel(item.optionCode);
  return `${label} ${actionLabel(item.action)}${operator ? ` ${operator}` : ''}`;
};

const heatOption = computed(() => {
  if (!usage.value.heat.length) return null;
  return {
    title: { text: t('option.usageTitle'), left: 'center' },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: usage.value.heat.map((h) => optionLabel(h.optionCode, h.name))
    },
    yAxis: { type: 'value' },
    series: [
      {
        type: 'bar',
        data: usage.value.heat.map((h) => h.frequency),
        itemStyle: {
          color: '#409EFF'
        }
      }
    ]
  };
});

onMounted(fetch);

watch(
  () => tenantStore.currentTenantId,
  () => fetch()
);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.refresh-btn {
  background-color: #5ac8fa;
  border-color: #5ac8fa;
  color: #fff;
}

.refresh-btn:not(.is-disabled):hover,
.refresh-btn:not(.is-disabled):focus {
  background-color: #48b4ef;
  border-color: #48b4ef;
  color: #fff;
}

.chart-wrapper {
  height: 260px;
}

.history {
  max-height: 180px;
  overflow: auto;
}
</style>
