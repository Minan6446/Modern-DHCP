<template>
  <div class="security-overview page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('security.overview.heroEyebrow') }}</p>
        <h2>{{ t('security.overview.heroTitle') }}</h2>
        <p class="desc">{{ t('security.overview.heroDesc') }}</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="goto('/security/rogue')">{{
            t('security.overview.actionPrimary')
          }}</el-button>
          <el-button text size="large" @click="goto('/security/policy')">{{
            t('security.overview.actionSecondary')
          }}</el-button>
        </div>
      </div>
      <div class="hero-metrics">
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
          <div class="card-header quick-actions-header">
            <span>{{ t('overview.quickActions') }}</span>
            <el-button
              text
              size="small"
              class="security-refresh"
              :loading="loading"
              @click="loadData"
              >{{ t('common.refresh') }}</el-button
            >
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
        <div class="coverage-card">
          <div>
            <p class="coverage-title">{{ t('security.overview.coverageTitle') }}</p>
            <p class="coverage-desc">{{ t('security.overview.coverageDesc') }}</p>
          </div>
          <el-progress
            type="dashboard"
            :percentage="trustCoverage"
            :color="trustCoverage >= 80 ? '#16a34a' : '#f97316'"
          />
        </div>
      </el-card>

      <el-card shadow="never" class="access-guard-card">
        <template #header>
          <span>{{ t('security.overview.controlsTitle') }}</span>
        </template>
        <ul class="control-list">
          <li v-for="control in controls" :key="control.key">
            <p class="control-title">{{ control.title }}</p>
            <p class="control-desc">{{ control.desc }}</p>
          </li>
        </ul>
      </el-card>
    </section>

    <section class="split-grid">
      <el-card shadow="never">
        <template #header>
          <span>{{ t('security.overview.rogueTitle') }}</span>
        </template>
        <el-table :data="rogues" size="small" border stripe>
          <template v-if="!rogues.length" #empty>
            <el-empty :description="t('security.overview.rogueEmpty')" />
          </template>
          <el-table-column prop="ip" label="IP" min-width="150" />
          <el-table-column prop="mac" label="MAC" min-width="160" />
          <el-table-column prop="vlan" label="VLAN" width="100" />
          <el-table-column prop="severity" :label="t('security.overview.severity')" width="120">
            <template #default="{ row }">
              <el-tag :type="rogueSeverity(row.severity)">{{ row.severity }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="detectedAt"
            :label="t('security.overview.detected')"
            min-width="180"
          >
            <template #default="{ row }">{{ formatTs(row.detectedAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span>{{ t('security.overview.threatTitle') }}</span>
        </template>
        <div v-if="!threats.length" class="empty-block">
          {{ t('security.overview.threatEmpty') }}
        </div>
        <el-timeline v-else>
          <el-timeline-item
            v-for="event in threats"
            :key="event.id"
            :timestamp="formatTs(event.occurredAt)"
            :type="event.score >= 80 ? 'danger' : 'warning'"
          >
            <p class="timeline-title">{{ threatLabel(event.type) }}</p>
            <p class="timeline-desc">{{ event.description || event.sourceIp }}</p>
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
import { listRogueServers, listThreatEvents, listTrustPorts } from '@/api/security';
import type { RogueServerRecord, ThreatEvent, TrustPort } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const router = useRouter();
const tenantStore = useTenantStore();
const loading = ref(false);
const trustPorts = ref<TrustPort[]>([]);
const rogues = ref<RogueServerRecord[]>([]);
const threats = ref<ThreatEvent[]>([]);

const stats = computed(() => {
  const trusted = trustPorts.value.filter((port) => port.trusted).length;
  const total = trustPorts.value.length || 1;
  return [
    {
      key: 'trust',
      label: t('security.overview.metricTrust'),
      value: trusted,
      hint: t('security.overview.metricTrustHint')
    },
    {
      key: 'rogue',
      label: t('security.overview.metricRogue'),
      value: rogues.value.length,
      hint: t('security.overview.metricRogueHint')
    },
    {
      key: 'threat',
      label: t('security.overview.metricThreat'),
      value: threats.value.length,
      hint: t('security.overview.metricThreatHint')
    }
  ];
});

const quickActions = computed(() => [
  { key: 'snooping', label: t('security.overview.quickSnooping'), path: '/security/snooping' },
  { key: 'rate', label: t('security.overview.quickRateLimit'), path: '/security/rate-limit' },
  { key: 'topology', label: t('security.overview.quickTopology'), path: '/security/topology' }
]);

const controls = computed(() => [
  {
    key: 'dai',
    title: t('security.overview.controlDai.title'),
    desc: t('security.overview.controlDai.desc')
  },
  {
    key: 'sg',
    title: t('security.overview.controlGuard.title'),
    desc: t('security.overview.controlGuard.desc')
  },
  {
    key: 'profiles',
    title: t('security.overview.controlProfiles.title'),
    desc: t('security.overview.controlProfiles.desc')
  }
]);

const trustCoverage = computed(() => {
  if (!trustPorts.value.length) return 0;
  return Math.round(
    (trustPorts.value.filter((port) => port.trusted).length / trustPorts.value.length) * 100
  );
});

const rogueSeverity = (severity: RogueServerRecord['severity']) => {
  if (severity === 'high') return 'danger';
  if (severity === 'medium') return 'warning';
  return 'info';
};

const threatLabel = (type: ThreatEvent['type']) => {
  switch (type) {
    case 'spoofing':
      return t('security.overview.threatSpoofing');
    case 'exhaustion':
      return t('security.overview.threatExhaustion');
    case 'rogue-server':
      return t('security.overview.threatRogue');
    default:
      return t('security.overview.threatAnomaly');
  }
};

const goto = (path: string) => router.push(path);

const loadData = async () => {
  loading.value = true;
  try {
    const tenantId = tenantStore.currentTenantId || undefined;
    const [trustRes, rogueRes, threatRes] = await Promise.all([
      listTrustPorts({ tenantId }),
      listRogueServers({ tenantId }),
      listThreatEvents({ tenantId })
    ]);
    trustPorts.value = trustRes.data.data || [];
    rogues.value = (rogueRes.data.data || []).slice(0, 6);
    threats.value = (threatRes.data.data || []).slice(0, 6);
  } finally {
    loading.value = false;
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => loadData()
);

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.security-overview {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(120deg, #1b1f3a, #9333ea);
  color: var(--el-color-white);
  display: flex;
  justify-content: space-between;
  gap: 24px;
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

.metric {
  background: rgba(15, 23, 42, 0.5);
  border-radius: 14px;
  padding: 14px;
}

.metric-label {
  font-size: 12px;
  opacity: 0.7;
  text-transform: uppercase;
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

.quick-actions-header {
  position: relative;
  align-items: flex-start;
  padding-right: 120px;
}

.security-refresh {
  position: absolute;
  top: 0;
  right: 0;
  color: var(--el-color-white);
  background-color: #38bdf8;
  border-color: #38bdf8;
}

.security-refresh:hover {
  color: var(--el-color-white);
  background-color: #0ea5e9;
  border-color: #0ea5e9;
}

.quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;
}

.coverage-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  background: var(--el-fill-color-light);
  border-radius: 14px;
  padding: 12px 16px;
}

.access-guard-card {
  border-radius: 16px;
}

.access-guard-card :deep(.el-card__header) {
  padding: 18px 24px;
}

.access-guard-card :deep(.el-card__body) {
  padding: 24px;
}

.coverage-title {
  font-weight: 600;
}

.coverage-desc {
  color: var(--el-text-color-secondary);
}

.control-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.control-title {
  font-weight: 600;
}

.control-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
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
