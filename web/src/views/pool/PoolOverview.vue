<template>
  <div class="pool-overview page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('pool.overview.heroEyebrow') }}</p>
        <h2>{{ t('pool.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('pool.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/pool/ipv4')">
            {{ t('pool.overview.actionPrimary') }}
          </el-button>
          <el-button text size="large" @click="goto('/pool/analytics')">
            {{ t('pool.overview.actionSecondary') }}
          </el-button>
        </div>
      </div>
      <div class="hero-metrics">
        <div v-for="card in statsCards" :key="card.key" class="hero-metric">
          <span class="metric-label">{{ card.label }}</span>
          <span class="metric-value">{{ card.value }}</span>
          <span class="metric-hint">{{ card.hint }}</span>
        </div>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('overview.quickActions') }}</span>
            <el-button text size="small" :loading="poolStore.loading" @click="loadPools">{{
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
          >
            {{ action.label }}
          </el-button>
        </div>
        <div class="insight-panel">
          <p class="insight-title">{{ t('overview.insights') }}</p>
          <ul>
            <li v-for="insight in insights" :key="insight.key">{{ insight.text }}</li>
          </ul>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.leaderboard') }}</span>
        </template>
        <div v-if="!leaderboard.length" class="empty-block">
          {{ t('pool.overview.leaderboardEmpty') }}
        </div>
        <div v-else class="leaderboard">
          <div v-for="pool in leaderboard" :key="pool.id" class="leaderboard-row">
            <div>
              <p class="leaderboard-title">{{ pool.name }}</p>
              <p class="leaderboard-desc">{{ pool.cidr }}</p>
            </div>
            <el-progress
              :percentage="pool.utilization"
              :status="pool.utilization >= 80 ? 'exception' : 'success'"
              :stroke-width="12"
            />
          </div>
        </div>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.focus') }}</span>
        </template>
        <el-table :data="focusPools" size="small" border stripe :show-header="!!focusPools.length">
          <template v-if="!focusPools.length" #empty>
            <el-empty :description="t('pool.overview.focusEmpty')" />
          </template>
          <el-table-column prop="name" :label="t('pool.labelName')" min-width="160" />
          <el-table-column prop="cidr" label="CIDR" min-width="160" />
          <el-table-column :label="t('pool.overview.utilization')" width="140">
            <template #default="{ row }">{{ row.utilization }}%</template>
          </el-table-column>
          <el-table-column :label="t('pool.status')" width="120">
            <template #default="{ row }">
              <el-tag :type="row.status === 'warning' ? 'warning' : 'info'">{{
                poolStatus(row.status)
              }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.timeline') }}</span>
        </template>
        <div v-if="!history.length" class="empty-block">{{ t('overview.timelineEmpty') }}</div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in history"
            :key="item.at + item.poolId"
            :timestamp="formatTs(item.at)"
          >
            <p class="timeline-title">{{ item.name }}</p>
            <p class="timeline-desc">{{ poolHistoryLabel(item.action) }}</p>
          </el-timeline-item>
        </el-timeline>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { usePoolStore } from '@/store/pool';
import { useTenantStore } from '@/store/tenant';
import type { Pool } from '@/types/pool';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const router = useRouter();
const poolStore = usePoolStore();
const tenantStore = useTenantStore();

const statsCards = computed(() => {
  const pools = poolStore.pools;
  const total = pools.length;
  const v4 = pools.filter((p) => p.version === 4).length;
  const v6 = total - v4;
  const avg = total
    ? Math.round(pools.reduce((sum, pool) => sum + pool.utilization, 0) / total)
    : 0;
  const saturated = pools.filter((p) => p.utilization >= 80).length;
  return [
    {
      key: 'total',
      label: t('pool.overview.totalPools'),
      value: total,
      hint: t('pool.overview.totalHint')
    },
    {
      key: 'ipv4',
      label: t('pool.overview.ipv4Pools'),
      value: v4,
      hint: t('pool.overview.ipv4Hint')
    },
    {
      key: 'ipv6',
      label: t('pool.overview.ipv6Pools'),
      value: v6,
      hint: t('pool.overview.ipv6Hint')
    },
    {
      key: 'avg',
      label: t('pool.overview.avgUtilization'),
      value: `${avg}%`,
      hint: t('pool.overview.avgHint', { count: saturated })
    }
  ];
});

const quickActions = computed(() => [
  { key: 'v4', label: t('pool.overview.quickV4'), path: '/pool/ipv4' },
  { key: 'v6', label: t('pool.overview.quickV6'), path: '/pool/ipv6' },
  { key: 'analytics', label: t('pool.overview.quickAnalytics'), path: '/pool/analytics' }
]);

const insights = computed(() => {
  const pools = poolStore.pools;
  const highUtil = pools.filter((p) => p.utilization >= 70).length;
  const warnings = pools.filter((p) => p.status === 'warning').length;
  return [
    { key: 'high', text: t('pool.overview.insightHigh', { count: highUtil }) },
    { key: 'warnings', text: t('pool.overview.insightWarnings', { count: warnings }) }
  ];
});

const leaderboard = computed(() =>
  poolStore.pools
    .slice()
    .sort((a, b) => b.utilization - a.utilization)
    .slice(0, 5)
);

const focusPools = computed(() =>
  poolStore.pools.filter((pool) => pool.status === 'warning' || pool.utilization >= 85).slice(0, 6)
);

const history = computed(() =>
  poolStore.history
    .map((event) => ({
      ...event,
      name: poolStore.pools.find((p) => p.id === event.poolId)?.name || 'Pool'
    }))
    .slice(0, 6)
);

const poolHistoryLabel = (action: string) => {
  switch (action) {
    case 'select':
      return t('pool.overview.historySelected');
    case 'create':
      return t('pool.overview.historyCreated');
    case 'update':
      return t('pool.overview.historyUpdated');
    default:
      return action;
  }
};

const poolStatus = (status: Pool['status']) => {
  if (status === 'active') return t('pool.statusActive');
  if (status === 'warning') return t('pool.statusWarning');
  return t('pool.statusDisabled');
};

const goto = (path: string) => router.push(path);

const loadPools = async () => {
  await poolStore.fetch();
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    loadPools();
  }
);

onMounted(() => {
  loadPools();
});
</script>

<style scoped>
.pool-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  padding: 28px;
  border-radius: 20px;
  background: radial-gradient(circle at top left, #ec4899, #8b5cf6 60%, #312e81);
  color: #fff;
  display: flex;
  gap: 24px;
  justify-content: space-between;
}

.hero-actions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  min-width: 320px;
}

.hero-metric {
  background: rgba(15, 23, 42, 0.55);
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
  font-size: 28px;
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
}

.insight-panel {
  margin-top: 16px;
  background: var(--el-fill-color-light);
  border-radius: 12px;
  padding: 12px 16px;
}

.insight-title {
  font-weight: 600;
  margin-bottom: 4px;
}

.leaderboard {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.leaderboard-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.leaderboard-title {
  font-weight: 600;
}

.leaderboard-desc {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

.empty-block {
  padding: 32px;
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

  .hero-metrics {
    width: 100%;
  }
}
</style>
