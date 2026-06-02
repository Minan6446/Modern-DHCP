<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>{{ t('security.comp.threatTitle') }}</span>
        <el-space>
          <el-switch v-model="autoProtect" :active-text="t('security.comp.threatAutoProtect')" />
          <el-button size="small" @click="refresh">{{ t('security.comp.threatRefresh') }}</el-button>
        </el-space>
      </div>
    </template>
    <el-row :gutter="12">
      <el-col :span="14">
        <el-table :data="events" size="small" border height="260">
          <el-table-column prop="type" :label="t('security.comp.threatColType')" width="120" />
          <el-table-column prop="sourceIp" :label="t('security.comp.threatColSourceIp')" width="140" />
          <el-table-column prop="port" :label="t('security.comp.threatColPort')" width="100" />
          <el-table-column prop="score" :label="t('security.comp.threatColScore')" width="90" />
          <el-table-column prop="occurredAt" :label="t('security.comp.threatColTime')" />
        </el-table>
      </el-col>
      <el-col :span="10">
        <div class="panel-title">{{ t('security.comp.threatRealtimeTitle') }}</div>
        <div class="chart-wrapper">
          <base-e-chart v-if="trendOption" :option="trendOption" />
        </div>
        <div class="panel-title">{{ t('security.comp.threatPatternTitle') }}</div>
        <el-tag v-for="p in patterns" :key="p.type" type="warning" class="tag"
          >{{ p.type }} x {{ p.count }}</el-tag
        >
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { listThreatEvents } from '@/api/security';
import type { ThreatEvent } from '@/types/security';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { analyzeAttackPattern, scoreThreat } from '@/utils/securityAnalysis';
import { useIntervalFn } from '@vueuse/core';

const { t } = useI18n();

const events = ref<ThreatEvent[]>([]);
const autoProtect = ref(true);

const refresh = async () => {
  try {
    const { data } = await listThreatEvents();
    events.value = data.data.map((e) => ({ ...e, score: scoreThreat(e) }));
  } catch (e) {
    events.value = [
      {
        id: 't1',
        type: 'spoofing',
        sourceIp: '192.168.1.50',
        port: 'Gi0/1',
        score: 65,
        occurredAt: '2024-12-05 10:10'
      },
      {
        id: 't2',
        type: 'exhaustion',
        sourceIp: '192.168.2.30',
        port: 'Gi0/2',
        score: 80,
        occurredAt: '2024-12-05 10:12'
      },
      {
        id: 't3',
        type: 'rogue-server',
        sourceIp: '10.10.10.10',
        port: 'VLAN20',
        score: 90,
        occurredAt: '2024-12-05 10:14'
      }
    ];
  }
};

const patterns = computed(() => analyzeAttackPattern(events.value));

const trendOption = computed(() => {
  if (!events.value.length) return null;
  const categories = events.value.map((e) => e.occurredAt || '').slice(-8);
  const data = events.value.map((e) => e.score).slice(-8);
  return {
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: categories },
    yAxis: { type: 'value', min: 0, max: 100 },
    series: [{ type: 'line', data, smooth: true, areaStyle: {} }]
  };
});

useIntervalFn(
  () => {
    if (autoProtect.value) refresh();
  },
  5000,
  { immediate: true }
);

onMounted(refresh);
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.panel-title {
  font-weight: 600;
  margin: 4px 0;
}

.chart-wrapper {
  height: 160px;
}

.tag {
  margin-right: 6px;
  margin-bottom: 6px;
}
</style>
