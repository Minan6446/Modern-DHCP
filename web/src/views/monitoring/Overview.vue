<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('monitoring.liveAlert.title') }}</h3>
      <p class="desc">{{ t('monitoring.liveAlert.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-select v-model="severityFilter" clearable :placeholder="t('monitoring.liveAlert.severityPlaceholder')" style="width: 130px">
          <el-option :label="t('monitoring.liveAlert.severityEmergency')" value="emergency" />
          <el-option :label="t('monitoring.liveAlert.severityCritical')" value="critical" />
          <el-option :label="t('monitoring.liveAlert.severityWarning')" value="warning" />
          <el-option :label="t('monitoring.liveAlert.severityInfo')" value="info" />
        </el-select>
        <el-select v-model="trendWindow" :placeholder="t('monitoring.liveAlert.trendWindowPlaceholder')" style="width: 140px" @change="loadData">
          <el-option :label="t('monitoring.liveAlert.trendWindow24h')" value="24h" />
          <el-option :label="t('monitoring.liveAlert.trendWindow7d')" value="7d" />
          <el-option :label="t('monitoring.liveAlert.trendWindow30d')" value="30d" />
        </el-select>
        <el-input v-model="keyword" clearable :placeholder="t('monitoring.liveAlert.keywordPlaceholder')" class="search" />
      </div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button :loading="loading" @click="loadData">{{ t('monitoring.liveAlert.refresh') }}</el-button>
        <el-button type="primary" @click="goHistory">{{ t('monitoring.liveAlert.goHistory') }}</el-button>
      </div>
    </div>

    <section class="stat-grid">
      <el-card
        v-for="card in statCards"
        :key="card.key"
        shadow="never"
        class="stat-card"
        :class="{ active: activeCard === card.key }"
        @click="clickStatCard(card.key)"
      >
        <div class="stat-title">{{ card.title }}</div>
        <div class="stat-value">{{ card.value }}</div>
        <div class="stat-sub">{{ card.sub }}</div>
      </el-card>
    </section>

    <el-card class="surface-card content-card" v-loading="loading || actionLoading">
      <div class="content-grid">
        <div class="left-pane">
          <div class="section-title">{{ t('monitoring.liveAlert.sectionAlertList') }}</div>
          <el-table :data="filteredAlerts" border stripe row-key="id" class="table">
            <el-table-column :label="t('monitoring.liveAlert.colLevel')" width="92">
              <template #default="{ row }">
                <el-tag :type="levelTagType(row.level)" effect="light">{{ levelLabel(row.level) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="type" :label="t('monitoring.liveAlert.colType')" width="130" />
            <el-table-column prop="message" :label="t('monitoring.liveAlert.colMessage')" min-width="220" show-overflow-tooltip />
            <el-table-column prop="time" :label="t('monitoring.liveAlert.colTime')" width="170" />
            <el-table-column prop="ip" :label="t('monitoring.liveAlert.colIp')" width="140" />
            <el-table-column :label="t('monitoring.liveAlert.colActions')" width="210" fixed="right">
              <template #default="{ row }">
                <el-button size="small" type="primary" link @click="ack(row)">{{ t('monitoring.liveAlert.actionAck') }}</el-button>
                <el-button size="small" type="success" link @click="resolve(row)">{{ t('monitoring.liveAlert.actionResolve') }}</el-button>
                <el-button size="small" link @click="goConfigByLevel(row.level)">{{ t('monitoring.liveAlert.actionGoConfig') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div class="right-pane">
          <div class="section-title">{{ t('monitoring.liveAlert.sectionTrend') }}</div>
          <div class="trend-switch">
            <el-radio-group v-model="trendStep" @change="loadData">
              <el-radio-button label="1h">{{ t('monitoring.liveAlert.trendStep1h') }}</el-radio-button>
              <el-radio-button label="2h">{{ t('monitoring.liveAlert.trendStep2h') }}</el-radio-button>
              <el-radio-button label="6h">{{ t('monitoring.liveAlert.trendStep6h') }}</el-radio-button>
            </el-radio-group>
          </div>
          <div class="trend-wrapper">
            <div class="trend-item" v-for="point in trend" :key="point.label" @click="goAnalytics(point.label)">
              <div class="bar" :style="{ height: `${Math.max(8, point.total * 6)}px` }">
                <span class="bar-part emergency" :style="{ height: `${point.emergency * 6}px` }" />
                <span class="bar-part critical" :style="{ height: `${point.critical * 6}px` }" />
                <span class="bar-part warning" :style="{ height: `${point.warning * 6}px` }" />
                <span class="bar-part info" :style="{ height: `${point.info * 6}px` }" />
              </div>
              <div class="bar-label">{{ point.label }}</div>
            </div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';
import { useAuthStore } from '@/modules/auth/store';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import {
  acknowledgeAlertEvent,
  getAlertActiveSummary,
  getAlertActiveTrend,
  listAlertEvents,
  resolveAlertEvent
} from '@/api/monitoring';
import type { AlertActiveSummary, AlertEvent, AlertTrendPoint } from '@/types/monitoring';
import { levelLabel, levelTagType, type AlertLevel } from './alertUi';

interface AlertRow {
  id: string;
  level: AlertLevel;
  type: string;
  message: string;
  time: string;
  ip: string;
}

const { t } = useI18n();
const router = useRouter();
const authStore = useAuthStore();
const loading = ref(false);
const actionLoading = ref(false);
const keyword = ref('');
const severityFilter = ref<AlertLevel | ''>('');
const trendWindow = ref<'24h' | '7d' | '30d'>('24h');
const trendStep = ref<'1h' | '2h' | '6h'>('2h');
const activeCard = ref<'all' | AlertLevel>('all');

const summary = ref<AlertActiveSummary>({ active: 0, emergency: 0, critical: 0, warning: 0, info: 0 });
const alerts = ref<AlertEvent[]>([]);
const trendPoints = ref<AlertTrendPoint[]>([]);

const statCards = computed(() => [
  { key: 'all' as const, title: t('monitoring.liveAlert.statActive'), value: summary.value.active, sub: t('monitoring.liveAlert.statActiveSub') },
  { key: 'emergency' as const, title: t('monitoring.liveAlert.statEmergency'), value: summary.value.emergency, sub: t('monitoring.liveAlert.statEmergencySub') },
  { key: 'critical' as const, title: t('monitoring.liveAlert.statCritical'), value: summary.value.critical, sub: t('monitoring.liveAlert.statCriticalSub') },
  { key: 'warning' as const, title: t('monitoring.liveAlert.statWarning'), value: summary.value.warning, sub: t('monitoring.liveAlert.statWarningSub') }
]);

const alertRows = computed<AlertRow[]>(() =>
  alerts.value.map((item) => ({
    id: item.id,
    level: (item.severity as AlertLevel) || 'info',
    type: item.type || t('monitoring.liveAlert.defaultType'),
    message: item.message,
    time: item.createdAt ? new Date(item.createdAt).toLocaleString() : '-',
    ip: item.ip || '-'
  }))
);

const filteredAlerts = computed(() =>
  alertRows.value.filter((row) => {
    const currentLevel = activeCard.value === 'all' ? severityFilter.value : activeCard.value;
    const matchLevel = currentLevel ? row.level === currentLevel : true;
    const kw = keyword.value.trim();
    const matchKeyword =
      !kw || [row.type, row.message, row.ip].join(' ').toLowerCase().includes(kw.toLowerCase());
    return matchLevel && matchKeyword;
  })
);

const trend = computed(() =>
  trendPoints.value.map((point) => {
    const d = point.ts ? new Date(point.ts) : new Date();
    const label = trendStep.value === '1h' ? `${d.getHours()}:00` : `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}h`;
    return {
      label,
      emergency: point.emergency,
      critical: point.critical,
      warning: point.warning,
      info: point.info,
      total: point.emergency + point.critical + point.warning + point.info
    };
  })
);

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;
const currentOperator = computed(
  () => authStore.user?.username || authStore.user?.displayName || authStore.lastUsername || 'unknown'
);

const loadData = async () => {
  loading.value = true;
  try {
    const [summaryResp, trendResp, eventsResp] = await Promise.all([
      getAlertActiveSummary(),
      getAlertActiveTrend({ window: trendWindow.value, step: trendStep.value }),
      listAlertEvents({ page: 1, pageSize: 80, status: 'firing', severity: severityFilter.value || undefined })
    ]);

    summary.value = unwrap<AlertActiveSummary>(summaryResp);
    trendPoints.value = unwrap<AlertTrendPoint[]>(trendResp) || [];

    const payload = unwrap<any>(eventsResp);
    const items: AlertEvent[] = Array.isArray(payload?.items)
      ? payload.items
      : Array.isArray(payload?.data?.items)
        ? payload.data.items
        : Array.isArray(payload)
          ? payload
          : [];
    alerts.value = items;
  } catch (err) {
    showHttpError(err, t('monitoring.liveAlert.loadFail'));
  } finally {
    loading.value = false;
  }
};

const clickStatCard = (key: 'all' | AlertLevel) => {
  activeCard.value = key;
};

const ack = async (row: AlertRow) => {
  await ElMessageBox.confirm(t('monitoring.liveAlert.ackConfirm', { msg: row.message }), t('monitoring.liveAlert.ackTitle'), { type: 'warning' });
  actionLoading.value = true;
  try {
    await acknowledgeAlertEvent(row.id, { assignee: currentOperator.value });
    showSuccess(t('monitoring.liveAlert.acked'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('monitoring.liveAlert.ackFail'));
  } finally {
    actionLoading.value = false;
  }
};

const resolve = async (row: AlertRow) => {
  await ElMessageBox.confirm(t('monitoring.liveAlert.resolveConfirm', { msg: row.message }), t('monitoring.liveAlert.resolveTitle'), { type: 'warning' });
  actionLoading.value = true;
  try {
    await resolveAlertEvent(row.id, { channel: 'dashboard', assignee: currentOperator.value });
    showSuccess(t('monitoring.liveAlert.resolved'));
    await loadData();
  } catch (err) {
    showHttpError(err, t('monitoring.liveAlert.resolveFail'));
  } finally {
    actionLoading.value = false;
  }
};

const goHistory = () => {
  router.push({ path: '/monitor/history', query: { level: activeCard.value !== 'all' ? activeCard.value : undefined } });
};

const goConfigByLevel = (level: AlertLevel) => {
  router.push({ path: '/monitor/config', query: { level } });
};

const goAnalytics = (slot: string) => {
  router.push({ path: '/monitor/analytics', query: { slot, level: activeCard.value } });
};

onMounted(loadData);
</script>

<style scoped>
.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #ebeef5;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  --bar-bg: #f3f4f6;
  --accent-border: #409eff;
  padding: 12px;
  background: var(--page-bg);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.surface-card {
  border: 1px solid var(--surface-border);
  border-radius: 4px;
  background: var(--surface-bg);
  color: var(--text-primary);
}

.page-header {
  padding: 12px 16px;
}

.page-header h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filter-bar {
  height: 48px;
  padding: 8px 16px;
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 8px;
  align-items: center;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search {
  width: 240px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button),
.filter-bar :deep(.el-radio-button__inner) {
  height: 32px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  cursor: pointer;
  border: 1px solid var(--surface-border);
}

.stat-card.active {
  border-color: var(--accent-border);
}

.stat-title {
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  font-size: 26px;
  font-weight: 700;
  margin-top: 6px;
}

.stat-sub {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 6px;
}

.content-card :deep(.el-card__body) {
  padding: 16px;
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 16px;
}

.section-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.trend-switch {
  margin-bottom: 10px;
}

.trend-wrapper {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(42px, 1fr));
  align-items: end;
  gap: 8px;
}

.trend-item {
  text-align: center;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
}

.bar {
  width: 14px;
  margin: 0 auto;
  border-radius: 6px;
  background: var(--bar-bg);
  overflow: hidden;
  display: flex;
  flex-direction: column-reverse;
}

.bar-part {
  display: block;
  width: 100%;
}

.bar-part.emergency {
  background: #f56c6c;
}

.bar-part.critical {
  background: #e6a23c;
}

.bar-part.warning {
  background: #409eff;
}

.bar-part.info {
  background: #909399;
}

.bar-label {
  margin-top: 6px;
}
</style>