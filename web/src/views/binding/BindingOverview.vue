<template>
  <div class="binding-overview page-block">
    <section class="hero-card">
      <div class="hero-text">
        <p class="eyebrow">{{ t('binding.overview.heroEyebrow') }}</p>
        <h2>{{ t('binding.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('binding.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/binding/mac')">
            {{ t('binding.overview.actionPrimary') }}
          </el-button>
        </div>
      </div>
      <div class="hero-metrics">
        <div v-for="metric in heroMetrics" :key="metric.key" class="hero-metric">
          <span class="metric-label">{{ metric.label }}</span>
          <span class="metric-value">{{ metric.value }}</span>
          <span class="metric-hint">{{ metric.hint }}</span>
        </div>
      </div>
    </section>

    <section class="stat-grid">
      <el-card v-for="card in statsCards" :key="card.key" class="stat-card">
        <div class="stat-label">{{ card.label }}</div>
        <div class="stat-value" :style="{ color: card.color }">{{ card.value }}</div>
        <div class="stat-desc">{{ card.desc }}</div>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('overview.quickActions') }}</span>
            <el-button text size="small" :loading="bindingStore.loading" @click="refresh">
              {{ t('common.refresh') }}
            </el-button>
          </div>
        </template>
        <div class="quick-actions">
          <el-button
            v-for="action in quickActions"
            :key="action.key"
            size="large"
            @click="goto(action.path)"
          >
            {{ action.label }}
          </el-button>
        </div>
        <el-alert type="info" :closable="false">
          {{ t('binding.overview.watchersHint') }}
        </el-alert>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.watchlist') }}</span>
        </template>
        <el-table :data="watchlist" border stripe size="small" :show-header="!!watchlist.length">
          <template v-if="!watchlist.length" #empty>
            <el-empty :description="t('binding.overview.watchlistEmpty')" />
          </template>
          <el-table-column prop="mac" :label="t('binding.formMac')" min-width="160" />
          <el-table-column prop="ip" :label="t('binding.formIp')" min-width="140" />
          <el-table-column :label="t('binding.status')" width="120">
            <template #default="{ row }">
              <el-tag :type="row.status === 'warning' ? 'warning' : 'info'">{{
                statusLabel(row.status)
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="createdAt" :label="t('binding.createdAt')" min-width="180">
            <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.timeline') }}</span>
        </template>
        <div v-if="!timeline.length" class="empty-block">{{ t('overview.timelineEmpty') }}</div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in timeline"
            :key="item.id"
            :timestamp="item.time"
            :type="item.type"
          >
            <p class="timeline-title">{{ item.title }}</p>
            <p class="timeline-desc">{{ item.desc }}</p>
          </el-timeline-item>
        </el-timeline>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.playbooks') }}</span>
        </template>
        <div class="playbook-list">
          <div v-for="playbook in playbooks" :key="playbook.key" class="playbook-item">
            <div>
              <p class="playbook-title">{{ playbook.title }}</p>
              <p class="playbook-desc">{{ playbook.desc }}</p>
            </div>
            <el-tag size="small" effect="dark" type="info">{{ playbook.tag }}</el-tag>
          </div>
        </div>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useBindingStore } from '@/store/binding';
import { useTenantStore } from '@/store/tenant';
import type { Binding, BindingStatus } from '@/types/binding';
import { formatTs } from '@/utils/time';

const router = useRouter();
const { t } = useI18n();
const bindingStore = useBindingStore();
const tenantStore = useTenantStore();

const heroMetrics = computed(() => [
  {
    key: 'total',
    label: t('binding.statsTotal'),
    value: bindingStore.stats.total,
    hint: t('binding.statsTotalDesc')
  },
  {
    key: 'online',
    label: t('binding.statusOnline'),
    value: bindingStore.stats.online,
    hint: t('binding.statsOnlineDesc')
  },
  {
    key: 'warning',
    label: t('binding.statusWarning'),
    value: bindingStore.stats.warning,
    hint: t('binding.statsWarningDesc')
  }
]);

const statsCards = computed(() => [
  {
    key: 'total',
    label: t('binding.statsTotal'),
    value: bindingStore.stats.total,
    desc: t('binding.statsTotalDesc'),
    color: '#2563eb'
  },
  {
    key: 'online',
    label: t('binding.statusOnline'),
    value: bindingStore.stats.online,
    desc: t('binding.statsOnlineDesc'),
    color: '#16a34a'
  },
  {
    key: 'warning',
    label: t('binding.statusWarning'),
    value: bindingStore.stats.warning,
    desc: t('binding.statsWarningDesc'),
    color: '#f97316'
  },
  {
    key: 'offline',
    label: t('binding.statusOffline'),
    value: bindingStore.stats.offline,
    desc: t('binding.statsOfflineDesc'),
    color: '#475569'
  }
]);

const quickActions = computed(() => [
  { key: 'mac', label: t('binding.overview.quickMac'), path: '/binding/mac' },
  { key: 'batch', label: t('binding.overview.quickBatch'), path: '/binding/batch' }
]);

const watchlist = computed(() =>
  bindingStore.rows
    .filter((row) => row.status === 'warning' || row.status === 'offline')
    .slice(0, 5)
);

const timeline = computed(() =>
  bindingStore.rows
    .slice()
    .sort((a, b) => (b.createdAt || '').localeCompare(a.createdAt || ''))
    .slice(0, 6)
    .map((row) => ({
      id: row.id,
      time: formatTs(row.createdAt),
      type: row.status === 'warning' ? 'warning' : 'primary',
      title: row.hostname || row.mac,
      desc: t('binding.overview.timelineCreated', { ip: row.ip, mac: row.mac })
    }))
);

const playbooks = computed(() => [
  {
    key: 'compliance',
    title: t('binding.overview.playbooks.compliance.title'),
    desc: t('binding.overview.playbooks.compliance.desc'),
    tag: t('binding.overview.playbooks.compliance.tag')
  },
  {
    key: 'migration',
    title: t('binding.overview.playbooks.migration.title'),
    desc: t('binding.overview.playbooks.migration.desc'),
    tag: t('binding.overview.playbooks.migration.tag')
  },
  {
    key: 'onboarding',
    title: t('binding.overview.playbooks.onboarding.title'),
    desc: t('binding.overview.playbooks.onboarding.desc'),
    tag: t('binding.overview.playbooks.onboarding.tag')
  }
]);

const statusLabel = (status: BindingStatus) => {
  if (status === 'online') return t('binding.statusOnline');
  if (status === 'warning') return t('binding.statusWarning');
  if (status === 'offline') return t('binding.statusOffline');
  return status;
};

const goto = (path: string) => {
  router.push(path);
};

const loadData = async () => {
  await bindingStore.fetchList({
    tenantId: tenantStore.currentTenantId || undefined,
    page: 1,
    pageSize: 40
  });
};

const refresh = () => loadData();

watch(
  () => tenantStore.currentTenantId,
  () => {
    loadData();
  }
);

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.binding-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 32px;
  padding: 32px;
  border-radius: 20px;
  background: linear-gradient(135deg, #0f172a, #1d4ed8);
  color: var(--el-color-white);
}

.hero-text h2 {
  font-size: 30px;
  margin: 4px 0 12px;
}

.hero-text .desc {
  opacity: 0.9;
  max-width: 540px;
}

.eyebrow {
  text-transform: uppercase;
  letter-spacing: 0.2em;
  font-size: 12px;
  opacity: 0.8;
}

.hero-actions {
  margin-top: 20px;
  display: flex;
  gap: 12px;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 16px;
  min-width: 320px;
}

.hero-metric {
  background: rgba(15, 23, 42, 0.45);
  border-radius: 16px;
  padding: 16px;
}

.metric-label {
  font-size: 13px;
  opacity: 0.8;
}

.metric-value {
  display: block;
  font-size: 32px;
  font-weight: 600;
}

.metric-hint {
  font-size: 12px;
  opacity: 0.65;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.stat-card {
  border-radius: 16px;
}

.stat-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.stat-value {
  font-size: 34px;
  font-weight: 600;
}

.stat-desc {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
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
  gap: 12px;
  margin-bottom: 12px;
}

.empty-block {
  padding: 32px;
  text-align: center;
  color: var(--el-text-color-secondary);
}

.playbook-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.playbook-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.playbook-item:last-child {
  border-bottom: none;
}

.playbook-title {
  font-weight: 600;
}

.playbook-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

@media (max-width: 960px) {
  .hero-card {
    flex-direction: column;
  }

  .hero-metrics {
    width: 100%;
  }
}
</style>
