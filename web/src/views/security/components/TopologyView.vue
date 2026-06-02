<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.comp.topoTitle') }}</span>
        <el-button size="small" text @click="refresh">{{ t('security.comp.topoRefresh') }}</el-button>
      </div>
    </template>
    <div class="chart-wrapper">
      <base-e-chart v-if="option" :option="option" />
      <el-empty v-else :description="t('security.comp.topoEmpty')" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { getTopology } from '@/api/security';
import type { TopologyLink, TopologyNode } from '@/types/security';
import { buildTopologyOption } from '@/utils/securityAnalysis';

const { t } = useI18n();

const nodes = ref<TopologyNode[]>([]);
const links = ref<TopologyLink[]>([]);

const refresh = async () => {
  try {
    const { data } = await getTopology();
    nodes.value = data.data.nodes;
    links.value = data.data.links;
  } catch (e) {
    nodes.value = [
      { id: 'core1', label: 'Core', type: 'core', status: 'normal' },
      { id: 'dist1', label: 'Dist-1', type: 'distribution', status: 'normal' },
      { id: 'acc1', label: 'Access-1', type: 'access', status: 'warning' },
      { id: 'srv1', label: 'DHCP Server', type: 'server', status: 'normal' },
      { id: 'rogue1', label: 'Rogue', type: 'rogue', status: 'critical' }
    ];
    links.value = [
      { source: 'core1', target: 'dist1', status: 'up' },
      { source: 'dist1', target: 'acc1', status: 'up' },
      { source: 'acc1', target: 'srv1', status: 'up' },
      { source: 'acc1', target: 'rogue1', status: 'degraded' }
    ];
  }
};

const option = computed(() =>
  nodes.value.length ? buildTopologyOption(nodes.value, links.value) : null
);

onMounted(refresh);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.chart-wrapper {
  height: 320px;
}
</style>
