<template>
  <div class="lease-overview page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('lease.overview.heroEyebrow') }}</p>
        <h2>{{ t('lease.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('lease.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/lease/active')">{{
            t('lease.overview.actionPrimary')
          }}</el-button>
        </div>
      </div>
      <div class="hero-stats">
        <el-card v-for="card in statsCards" :key="card.key" shadow="hover" class="stat-card">
          <span class="stat-label">{{ card.label }}</span>
          <span class="stat-value">{{ card.value }}</span>
          <span class="stat-hint">{{ card.hint }}</span>
        </el-card>
      </div>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <span>{{ t('overview.quickActions') }}</span>
            <el-button text size="small" :loading="activeLoading" @click="loadData">{{
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
        <div class="health-row">
          <div>
            <p class="health-title">{{ t('lease.overview.healthTitle') }}</p>
            <p class="health-desc">{{ t('lease.overview.healthDesc') }}</p>
          </div>
          <el-progress
            type="circle"
            :percentage="healthScore"
            :stroke-width="10"
            :status="healthScore < 70 ? 'exception' : 'success'"
          />
        </div>
        <div class="pressure-row">
          <span>{{ t('lease.overview.renewPressure') }}</span>
          <el-progress
            :percentage="renewalPressure"
            :status="renewalPressure >= 70 ? 'exception' : 'success'"
            :stroke-width="14"
          />
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('overview.watchlist') }}</span>
        </template>
        <el-table :data="atRisk" border stripe size="small" :show-header="!!atRisk.length">
          <template v-if="!atRisk.length" #empty>
            <el-empty :description="t('lease.overview.watchlistEmpty')" />
          </template>
          <el-table-column prop="ip" label="IP" min-width="140" />
          <el-table-column prop="mac" label="MAC" min-width="160" />
          <el-table-column prop="state" :label="t('lease.state')" width="120">
            <template #default="{ row }">
              <el-tag :type="stateType(row.state)">{{ row.state }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="endsAt" :label="t('lease.detailEnd')" min-width="160">
            <template #default="{ row }">{{ formatTs(row.endsAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('lease.overview.activeSampleTitle') }}</span>
        </template>
        <el-table :data="activeSample" border stripe size="small">
          <template v-if="!activeSample.length" #empty>
            <el-empty :description="t('lease.overview.emptyActive')" />
          </template>
          <el-table-column prop="ip" label="IP" min-width="140" />
          <el-table-column prop="hostname" :label="t('lease.detailHostname')" min-width="160" />
          <el-table-column prop="poolName" :label="t('lease.detailPool')" min-width="140" />
          <el-table-column prop="endsAt" :label="t('lease.detailEnd')" min-width="160">
            <template #default="{ row }">{{ formatTs(row.endsAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never" class="history-card">
        <template #header>
          <span>{{ t('overview.timeline') }}</span>
        </template>
        <div v-if="!historySample.length" class="empty-block">
          {{ t('overview.timelineEmpty') }}
        </div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in historySample"
            :key="item.id"
            :timestamp="formatTs(item.endsAt)"
          >
            <p class="timeline-title">{{ item.ip }}</p>
            <p class="timeline-desc">
              {{ t('lease.overview.timelineReleased', { mac: item.mac }) }}
            </p>
          </el-timeline-item>
        </el-timeline>
      </el-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useLeaseStore } from '@/store/lease';
import { useTenantStore } from '@/store/tenant';
import type { Lease } from '@/types/lease';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const router = useRouter();
const leaseStore = useLeaseStore();
const tenantStore = useTenantStore();
const activeLoading = ref(false);

const statsCards = computed(() => [
  {
    key: 'active',
    label: t('lease.statActive'),
    value: leaseStore.stats.active,
    hint: t('lease.statActiveDesc')
  },
  {
    key: 'pending',
    label: t('lease.statPending'),
    value: leaseStore.stats.pending,
    hint: t('lease.statPendingDesc')
  },
  {
    key: 'failed',
    label: t('lease.statFailed'),
    value: leaseStore.stats.failed,
    hint: t('lease.statFailedDesc')
  },
  {
    key: 'expired',
    label: t('lease.statExpired'),
    value: leaseStore.stats.expired,
    hint: t('lease.statExpiredDesc')
  }
]);

const quickActions = computed(() => [
  { key: 'active', label: t('lease.overview.quickActive'), path: '/lease/active' },
  { key: 'history', label: t('lease.overview.quickHistory'), path: '/lease/history' }
]);

const healthScore = computed(() => {
  const total = leaseStore.stats.active + leaseStore.stats.failed;
  if (!total) return 100;
  const failureRate = (leaseStore.stats.failed / total) * 100;
  return Math.max(40, Math.round(100 - failureRate));
});

const renewalPressure = computed(() => {
  const base = leaseStore.stats.active || 1;
  return Math.min(100, Math.round((leaseStore.stats.pending / base) * 100));
});

const atRisk = computed(() =>
  leaseStore.active
    .filter((lease) => ['DECLINED', 'CONFLICT', 'ABANDONED'].includes(lease.state))
    .slice(0, 5)
);

const activeSample = computed(() => leaseStore.active.slice(0, 6));
const historySample = computed(() => leaseStore.history.slice(0, 6));

const stateType = (state: Lease['state']) => {
  if (['DECLINED', 'CONFLICT', 'ABANDONED'].includes(state)) return 'danger';
  if (['RENEWING', 'REBINDING', 'REQUESTING'].includes(state)) return 'warning';
  return 'success';
};

const goto = (path: string) => router.push(path);

const loadData = async () => {
  activeLoading.value = true;
  try {
    const tenantId = tenantStore.currentTenantId || undefined;
    await Promise.all([
      leaseStore.fetchActive({ tenantId, page: 1, pageSize: 30 }),
      leaseStore.fetchHistory({ tenantId, page: 1, pageSize: 20 })
    ]);
  } finally {
    activeLoading.value = false;
  }
};

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
.lease-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(120deg, #0f766e, #22d3ee);
  color: #fff;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  min-width: 320px;
}

.stat-card {
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.4);
  color: #fff;
}

.stat-label {
  font-size: 12px;
  opacity: 0.8;
}

.stat-value {
  display: block;
  font-size: 28px;
  font-weight: 600;
}

.history-card {
  min-height: 420px;
  padding-bottom: 16px;
}

.stat-hint {
  font-size: 12px;
  opacity: 0.7;
}

.hero-actions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

.split-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 16px;
}

.quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.health-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 12px 0;
}

.health-title {
  font-weight: 600;
}

.health-desc {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.pressure-row {
  margin-top: 12px;
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
}
</style>
