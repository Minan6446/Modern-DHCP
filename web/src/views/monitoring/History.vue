<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('monitoring.history.title') }}</h3>
      <p class="desc">{{ t('monitoring.history.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-button-group>
          <el-button :type="quick === '24h' ? 'primary' : 'default'" @click="setQuick('24h')">{{ t('monitoring.history.quick24h') }}</el-button>
          <el-button :type="quick === '7d' ? 'primary' : 'default'" @click="setQuick('7d')">{{ t('monitoring.history.quick7d') }}</el-button>
          <el-button :type="quick === '30d' ? 'primary' : 'default'" @click="setQuick('30d')">{{ t('monitoring.history.quick30d') }}</el-button>
        </el-button-group>
        <el-select v-model="levelFilter" clearable :placeholder="t('monitoring.history.levelPlaceholder')" style="width: 120px">
          <el-option :label="t('monitoring.history.levelEmergency')" value="emergency" />
          <el-option :label="t('monitoring.history.levelCritical')" value="critical" />
          <el-option :label="t('monitoring.history.levelWarning')" value="warning" />
          <el-option :label="t('monitoring.history.levelInfo')" value="info" />
        </el-select>
        <el-input v-model="keyword" clearable :placeholder="t('monitoring.history.keywordPlaceholder')" class="search" />
      </div>
      <div class="bar-middle">
        <el-date-picker
          v-model="customRange"
          type="datetimerange"
          :start-placeholder="t('monitoring.history.dateStart')"
          :end-placeholder="t('monitoring.history.dateEnd')"
          :range-separator="t('monitoring.history.dateSep')"
          @change="applyCustom"
        />
      </div>
      <div class="bar-right">
        <el-button :loading="loading" @click="loadHistory">{{ t('monitoring.history.refresh') }}</el-button>
        <el-button @click="exportCsv">{{ t('monitoring.history.exportAudit') }}</el-button>
      </div>
    </div>

    <section class="stat-grid">
      <el-card v-for="item in statCards" :key="item.key" shadow="never" class="stat-card">
        <div class="stat-title">{{ item.title }}</div>
        <div class="stat-value">{{ item.value }}</div>
        <div class="stat-sub">{{ item.sub }}</div>
      </el-card>
    </section>

    <el-card class="surface-card content-card" v-loading="loading">
      <div class="content-grid">
        <div>
          <div class="section-title">{{ t('monitoring.history.sectionHistory') }}</div>
          <el-table :data="filteredRecords" border stripe row-key="id">
            <el-table-column :label="t('monitoring.history.colLevel')" width="92">
              <template #default="{ row }">
                <el-tag :type="levelTagType(row.level)" effect="light">{{ levelLabel(row.level) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="type" :label="t('monitoring.history.colType')" width="130" />
            <el-table-column prop="message" :label="t('monitoring.history.colMessage')" min-width="220" show-overflow-tooltip />
            <el-table-column prop="start" :label="t('monitoring.history.colStart')" width="170" />
            <el-table-column prop="end" :label="t('monitoring.history.colEnd')" width="170" />
            <el-table-column prop="assignee" :label="t('monitoring.history.colAssignee')" width="140" />
            <el-table-column :label="t('monitoring.history.colStatus')" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'resolved' ? 'success' : 'warning'">{{ row.status === 'resolved' ? t('monitoring.history.statusResolved') : t('monitoring.history.statusProcessing') }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="ip" :label="t('monitoring.history.colIp')" width="130" />
            <el-table-column :label="t('monitoring.history.colActions')" width="120" fixed="right">
              <template #default="{ row }">
                <el-button type="primary" link @click="goConfig(row.level)">{{ t('monitoring.history.goConfig') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div>
          <div class="section-title">{{ t('monitoring.history.sectionCompare') }}</div>
          <div class="compare-grid">
            <div class="compare-card" v-for="item in comparisons" :key="item.label">
              <div class="label">{{ item.label }}</div>
              <div class="value">{{ item.value }}</div>
              <div class="delta" :class="item.delta >= 0 ? 'up' : 'down'">
                {{ item.delta >= 0 ? '+' : '' }}{{ item.delta }}%
              </div>
            </div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { showHttpError } from '@/shared/errors/errorToast';
import { showSuccess } from '@/shared/errors/messageToast';
import { getAlertHistoryCompare, getAlertHistorySummary, listAlertEvents } from '@/api/monitoring';
import type { AlertEvent, AlertHistoryCompare, AlertHistorySummary } from '@/types/monitoring';
import { levelLabel, levelTagType, type AlertLevel } from './alertUi';

interface HistoryRow {
  id: string;
  level: AlertLevel;
  type: string;
  message: string;
  start: string;
  end: string;
  assignee: string;
  status: 'resolved' | 'processing';
  ip: string;
}

const { t } = useI18n();
const router = useRouter();
const route = useRoute();
const quick = ref<'24h' | '7d' | '30d'>('7d');
const customRange = ref<[Date, Date] | null>(null);
const levelFilter = ref<AlertLevel | ''>('');
const keyword = ref('');
const loading = ref(false);

const summary = ref<AlertHistorySummary>({ total: 0, resolutionRate: 0, inProgress: 0 });
const compare = ref<AlertHistoryCompare | null>(null);
const records = ref<HistoryRow[]>([]);

const rangeLabel = computed(() => {
  if (customRange.value) {
    return `${customRange.value[0].toLocaleString()} - ${customRange.value[1].toLocaleString()}`;
  }
  if (quick.value === '24h') return t('monitoring.history.rangeRecent24h');
  if (quick.value === '30d') return t('monitoring.history.rangeRecent30d');
  return t('monitoring.history.rangeRecent7d');
});

const statCards = computed(() => [
  { key: 'range', title: t('monitoring.history.statRange'), value: rangeLabel.value, sub: t('monitoring.history.statRangeSub') },
  { key: 'total', title: t('monitoring.history.statTotal'), value: summary.value.total, sub: t('monitoring.history.statTotalSub') },
  { key: 'rate', title: t('monitoring.history.statRate'), value: `${summary.value.resolutionRate}%`, sub: t('monitoring.history.statRateSub') },
  { key: 'progress', title: t('monitoring.history.statProgress'), value: summary.value.inProgress, sub: t('monitoring.history.statProgressSub') }
]);

const comparisons = computed(() => {
  if (!compare.value) return [];
  const current = compare.value.current;
  const previous = compare.value.previous;
  const previousCount = previous.count || 1;
  const countDelta = Math.round(((current.count - previousCount) / previousCount) * 100);
  const rateDelta = Math.round(current.resolutionRate - previous.resolutionRate);
  const mttrCurrent = current.mttr ?? '-';
  const mttrPrevious = previous.mttr ?? '-';
  return [
    { label: t('monitoring.history.compareCount'), value: `${current.count} vs ${previous.count}`, delta: countDelta },
    { label: t('monitoring.history.compareRate'), value: `${current.resolutionRate}% vs ${previous.resolutionRate}%`, delta: rateDelta },
    {
      label: t('monitoring.history.compareMttr'),
      value: `${mttrCurrent} vs ${mttrPrevious}`,
      delta:
        typeof mttrCurrent === 'number' && typeof mttrPrevious === 'number' && mttrPrevious > 0
          ? Math.round(((mttrCurrent - mttrPrevious) / mttrPrevious) * 100)
          : 0
    }
  ];
});

const filteredRecords = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return records.value.filter((item) => {
    const matchLevel = levelFilter.value ? item.level === levelFilter.value : true;
    const matchKeyword = !kw || [item.type, item.message, item.ip].join(' ').toLowerCase().includes(kw);
    return matchLevel && matchKeyword;
  });
});

const unwrap = <T>(resp: any): T => (resp?.data?.data ?? resp?.data ?? resp) as T;
const formatTime = (value?: string) => (value ? new Date(value).toLocaleString() : '-');

const buildRange = () => {
  const to = customRange.value ? customRange.value[1] : new Date();
  const from = customRange.value
    ? customRange.value[0]
    : (() => {
        const d = new Date();
        if (quick.value === '24h') d.setHours(d.getHours() - 24);
        else if (quick.value === '30d') d.setDate(d.getDate() - 30);
        else d.setDate(d.getDate() - 7);
        return d;
      })();
  return { from: from.toISOString(), to: to.toISOString() };
};

const loadHistory = async () => {
  loading.value = true;
  try {
    const range = buildRange();
    const [summaryResp, compareResp, eventsResp] = await Promise.all([
      getAlertHistorySummary(range),
      getAlertHistoryCompare({ range: quick.value, ...range }),
      listAlertEvents({ page: 1, pageSize: 120, status: 'all', ...range })
    ]);
    summary.value = unwrap<AlertHistorySummary>(summaryResp);
    compare.value = unwrap<AlertHistoryCompare>(compareResp);

    const payload = unwrap<any>(eventsResp);
    const items: AlertEvent[] = Array.isArray(payload?.items)
      ? payload.items
      : Array.isArray(payload?.data?.items)
        ? payload.data.items
        : Array.isArray(payload)
          ? payload
          : [];
    records.value = items.map((item) => ({
      id: item.id,
      level: (item.severity as AlertLevel) || 'info',
      type: item.type || t('monitoring.history.defaultType'),
      message: item.message,
      start: formatTime(item.createdAt),
      end: formatTime(item.resolvedAt),
      assignee: item.assignee || '-',
      status: item.status === 'resolved' ? 'resolved' : 'processing',
      ip: item.ip || '-'
    }));
  } catch (err) {
    showHttpError(err, t('monitoring.history.loadFail'));
  } finally {
    loading.value = false;
  }
};

const setQuick = (val: '24h' | '7d' | '30d') => {
  quick.value = val;
  customRange.value = null;
  loadHistory();
};

const applyCustom = () => {
  loadHistory();
};

const exportCsv = () => {
  const headers = [t('monitoring.history.csvLevel'), t('monitoring.history.csvType'), t('monitoring.history.csvDesc'), t('monitoring.history.csvStart'), t('monitoring.history.csvEnd'), t('monitoring.history.csvAssignee'), t('monitoring.history.csvStatus'), 'IP'];
  const rows = filteredRecords.value.map((r) => [
    levelLabel(r.level),
    r.type,
    r.message,
    r.start,
    r.end,
    r.assignee,
    r.status === 'resolved' ? t('monitoring.history.statusResolved') : t('monitoring.history.statusProcessing'),
    r.ip
  ]);
  const content = [headers, ...rows]
    .map((line) => line.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob([`\uFEFF${content}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `alert_history_${Date.now()}.csv`;
  link.click();
  URL.revokeObjectURL(url);
  showSuccess(t('monitoring.history.exportSuccess'));
};

const goConfig = (level: AlertLevel) => {
  router.push({ path: '/monitor/config', query: { level } });
};

onMounted(loadHistory);

onMounted(() => {
  const level = String(route.query.level || '').trim();
  if (level === 'emergency' || level === 'critical' || level === 'warning' || level === 'info') {
    levelFilter.value = level;
  }
  const type = String(route.query.type || '').trim();
  if (type) {
    keyword.value = type;
  }
});
</script>

<style scoped>
.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #ebeef5;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  --danger-up: #f56c6c;
  --success-down: #67c23a;
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
  align-items: center;
  gap: 8px;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bar-middle {
  display: flex;
  justify-content: center;
}

.search {
  width: 240px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button),
.filter-bar :deep(.el-range-editor) {
  height: 32px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  border: 1px solid var(--surface-border);
}

.stat-title {
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 700;
}

.stat-sub {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-tertiary);
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
  margin-bottom: 8px;
  font-weight: 600;
}

.compare-grid {
  display: grid;
  gap: 10px;
}

.compare-card {
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  padding: 10px;
  background: var(--surface-bg);
}

.label {
  font-size: 12px;
  color: var(--text-secondary);
}

.value {
  margin-top: 6px;
  font-size: 18px;
  font-weight: 700;
}

.delta {
  margin-top: 6px;
  font-size: 12px;
}

.delta.up {
  color: var(--danger-up);
}

.delta.down {
  color: var(--success-down);
}
</style>
