<template>
  <div class="option-overview page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('option.overview.heroEyebrow') }}</p>
        <h2>{{ t('option.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('option.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/option/standard')">{{
            t('option.overview.actionPrimary')
          }}</el-button>
          <el-button text size="large" @click="goto('/option/templates')">{{
            t('option.overview.actionSecondary')
          }}</el-button>
        </div>
      </div>
      <div class="hero-stats">
        <div v-for="metric in stats" :key="metric.key" class="metric">
          <span class="metric-label">{{ metric.label }}</span>
          <span class="metric-value">{{ metric.value }}</span>
          <span class="metric-hint">{{ metric.hint }}</span>
        </div>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('overview.quickActions') }}</span>
            <el-button text size="small" :loading="loading" @click="loadData">{{
              t('common.refresh')
            }}</el-button>
          </div>
        </template>
        <div class="quick-actions">
          <el-button
            v-for="action in quickActions"
            :key="action.key"
            size="large"
            @click="goto(action.path)"
            >{{ action.label }}</el-button
          >
        </div>
        <div class="heat-list">
          <p class="section-title">{{ t('option.overview.topDemand') }}</p>
          <el-skeleton v-if="loading" animated :rows="4" />
          <template v-else>
            <div v-if="!heat.length" class="empty-block">{{ t('option.overview.heatEmpty') }}</div>
            <div v-else class="heat-items">
              <div v-for="item in heat" :key="item.optionCode" class="heat-item">
                <div>
                  <p class="heat-title">
                    {{ optionLabel(item.optionCode, item.name) }} ({{ item.optionCode }})
                  </p>
                  <p class="heat-desc">
                    {{ t('option.overview.references', { count: item.references }) }}
                  </p>
                </div>
                <el-progress
                  :percentage="Math.min(100, Math.round(item.frequency))"
                  :stroke-width="10"
                />
              </div>
            </div>
          </template>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('option.overview.templatesTitle') }}</span>
        </template>
        <el-table :data="templates" border stripe size="small">
          <template v-if="!templates.length" #empty>
            <el-empty :description="t('option.overview.templatesEmpty')" />
          </template>
          <el-table-column prop="name" :label="t('option.templateName')" min-width="160" />
          <el-table-column prop="version" :label="t('option.version')" width="120" />
          <el-table-column prop="status" :label="t('option.status')" width="120">
            <template #default="{ row }">
              <el-tag :type="templateTag(row.status)">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="updatedAt" :label="t('common.updatedAt')" min-width="160">
            <template #default="{ row }">{{ formatTs(row.updatedAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.timeline') }}</span>
        </template>
        <div v-if="!history.length" class="empty-block">{{ t('overview.timelineEmpty') }}</div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in history"
            :key="item.timestamp + item.optionCode"
            :timestamp="formatTs(item.timestamp)"
            :type="item.action === 'delete' ? 'danger' : 'primary'"
          >
            <p class="timeline-title">
              {{ t('option.overview.historyTitle', { code: item.optionCode }) }}
            </p>
            <p class="timeline-desc">{{ historyDescription(item) }}</p>
          </el-timeline-item>
        </el-timeline>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('option.overview.relationsTitle') }}</span>
        </template>
        <p class="relations-value">{{ relations }}</p>
        <p class="relations-desc">{{ t('option.overview.relationsDesc') }}</p>
        <el-alert type="info" :closable="false">{{ t('option.overview.relationsHint') }}</el-alert>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { getOptionUsage, listOptionTemplates } from '@/api/options';
import type { OptionTemplate, OptionUsageSnapshot } from '@/types/option';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const router = useRouter();
const loading = ref(false);
const snapshot = ref<OptionUsageSnapshot | null>(null);
const templates = ref<OptionTemplate[]>([]);

const stats = computed(() => {
  const heat = snapshot.value?.heat?.length ?? 0;
  const relations = snapshot.value?.relations?.length ?? 0;
  const history = snapshot.value?.history?.length ?? 0;
  return [
    {
      key: 'heat',
      label: t('option.overview.metricHeat'),
      value: heat,
      hint: t('option.overview.metricHeatHint')
    },
    {
      key: 'relations',
      label: t('option.overview.metricRelations'),
      value: relations,
      hint: t('option.overview.metricRelationsHint')
    },
    {
      key: 'history',
      label: t('option.overview.metricHistory'),
      value: history,
      hint: t('option.overview.metricHistoryHint')
    }
  ];
});

const quickActions = computed(() => [
  { key: 'standard', label: t('option.overview.quickStandard'), path: '/option/standard' },
  { key: 'custom', label: t('option.overview.quickCustom'), path: '/option/custom' },
  { key: 'templates', label: t('option.overview.quickTemplates'), path: '/option/templates' }
]);

const heat = computed(() => snapshot.value?.heat?.slice(0, 5) ?? []);
const history = computed(() => snapshot.value?.history?.slice(0, 6) ?? []);
const relations = computed(() => snapshot.value?.relations?.length ?? 0);

const templateTag = (status: OptionTemplate['status']) => {
  if (status === 'active') return 'success';
  if (status === 'draft') return 'warning';
  return 'info';
};

const optionLabel = (code: number, name?: string) => {
  const key = `option.names.${code}`;
  const translated = t(key);
  if (translated !== key) return translated;
  if (name) return name;
  return t('option.optionCodeLabel', { code });
};

const historyDescription = (item: OptionUsageSnapshot['history'][number]) => {
  return t(`option.overview.history.${item.action}` as const, {
    operator: item.operator || t('common.system'),
    value: item.value || '--'
  });
};

const loadData = async () => {
  loading.value = true;
  try {
    const [{ data: usage }, { data: tpl }] = await Promise.all([
      getOptionUsage(),
      listOptionTemplates({ page: 1, pageSize: 5 })
    ]);
    snapshot.value = usage.data;
    templates.value = tpl.data.items || [];
  } finally {
    loading.value = false;
  }
};

const goto = (path: string) => router.push(path);

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.option-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(120deg, #111827, #312e81);
  color: #fff;
  display: flex;
  justify-content: space-between;
  gap: 24px;
}

.hero-actions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  min-width: 320px;
}

.metric {
  background: rgba(15, 23, 42, 0.45);
  border-radius: 14px;
  padding: 14px;
}

.metric-label {
  font-size: 12px;
  text-transform: uppercase;
  opacity: 0.7;
}

.metric-value {
  display: block;
  font-size: 30px;
  font-weight: 600;
}

.metric-hint {
  font-size: 12px;
  opacity: 0.7;
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 16px;
}

.heat-list {
  margin-top: 8px;
}

.section-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.heat-items {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.heat-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
}

.heat-title {
  font-weight: 600;
}

.heat-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.relations-value {
  font-size: 48px;
  font-weight: 600;
  margin: 0;
}

.relations-desc {
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}

.empty-block {
  padding: 24px;
  text-align: center;
  color: var(--el-text-color-secondary);
}

.timeline-title {
  font-weight: 600;
}

.timeline-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

@media (max-width: 960px) {
  .hero-card {
    flex-direction: column;
  }

  .hero-stats {
    width: 100%;
  }
}
</style>
