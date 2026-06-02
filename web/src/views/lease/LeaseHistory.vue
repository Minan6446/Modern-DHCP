<template>
  <div class="lease-page">
    <el-card v-loading="loading" class="surface-card table-card">
      <div class="top-ops-bar">
        <div class="ops-left">
          <h3>{{ t('lease.historyTitle') }}</h3>
          <p class="desc">{{ t('lease.historyDesc') }}</p>
        </div>
        <div class="ops-middle" />
        <div class="ops-right">
          <el-button size="small" :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
        </div>
      </div>

      <section class="overview-grid four-cols">
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('lease.historyCardTotal') }}</div>
          <div class="overview-value" style="color: #2563eb">{{ stats.total }}</div>
          <div class="overview-desc">{{ t('lease.historyCardTotalDesc') }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('lease.historyCardNormal') }}</div>
          <div class="overview-value" style="color: #16a34a">{{ stats.normalEnd }}</div>
          <div class="overview-desc">{{ t('lease.historyCardNormalDesc') }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card" :class="stats.abnormalEnd > 0 ? 'is-alert' : ''">
          <div class="overview-label">{{ t('lease.historyCardAbnormal') }}</div>
          <div class="overview-value" style="color: #dc2626">{{ stats.abnormalEnd }}</div>
          <div class="overview-desc">{{ t('lease.historyCardAbnormalDesc') }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('lease.historyCardAvgDuration') }}</div>
          <div class="overview-value" style="color: #7c3aed">{{ stats.avgLeaseDuration }}</div>
          <div class="overview-desc">{{ t('lease.historyCardAvgDurationDesc') }}</div>
        </el-card>
      </section>

      <section class="analysis-grid">
        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('lease.historyTrendTitle') }}</span>
              <span class="card-sub">{{ t('lease.historyTrendSub') }}</span>
            </div>
          </template>
          <el-empty v-if="!leaseTrendOption" :image-size="60" :description="t('lease.historyTrendEmpty')" />
          <BaseEChart v-else :option="leaseTrendOption" class="chart" />
        </el-card>

        <el-card shadow="never" class="analysis-card">
          <template #header>
            <div class="card-title-row">
              <span class="card-title">{{ t('lease.historyAbnormalTitle') }}</span>
              <span class="card-sub">{{ t('lease.historyAbnormalSub') }}</span>
            </div>
          </template>
          <el-empty v-if="!abnormalRateOption" :image-size="60" :description="t('lease.historyAbnormalEmpty')" />
          <BaseEChart v-else :option="abnormalRateOption" class="chart" />
        </el-card>
      </section>

      <section class="filters-panel">
        <div class="quick-filters">
          <el-tag
            v-for="item in quickTimeItems"
            :key="item.key"
            :type="activeQuickTime === item.key ? 'primary' : 'info'"
            :effect="activeQuickTime === item.key ? 'dark' : 'plain'"
            class="quick-tag"
            @click="applyQuickTime(item.key)"
          >
            {{ item.label }}
          </el-tag>
        </div>

        <div class="audit-filters-grid">
          <div class="filter-group">
            <div class="group-title">{{ t('lease.historyGroupBase') }}</div>
            <el-form :model="filters" inline class="filters" @submit.prevent>
              <el-form-item label="IP">
                <el-input v-model="filters.ip" size="small" placeholder="IP" clearable />
              </el-form-item>
              <el-form-item label="MAC">
                <el-input v-model="filters.mac" size="small" placeholder="MAC" clearable />
              </el-form-item>
              <el-form-item label="Client ID">
                <el-input v-model="filters.clientId" size="small" placeholder="Client ID" clearable />
              </el-form-item>
              <el-form-item :label="t('lease.activeFilterPool')">
                <el-input v-model="filters.poolId" size="small" :placeholder="t('lease.activeFilterPoolPlaceholder')" clearable />
              </el-form-item>
            </el-form>
          </div>

          <div class="filter-group">
            <div class="group-title">{{ t('lease.historyGroupAudit') }}</div>
            <el-form :model="filters" inline class="filters" @submit.prevent>
              <el-form-item :label="t('lease.activeFilterState')">
                <el-select
                  v-model="filters.state"
                  multiple
                  clearable
                  collapse-tags
                  collapse-tags-tooltip
                  size="small"
                  :placeholder="t('lease.activeFilterStatePlaceholder')"
                  style="width: 190px"
                >
                  <el-option
                    v-for="state in stateOptions"
                    :key="state.value"
                    :label="state.label"
                    :value="state.value"
                  />
                </el-select>
              </el-form-item>
              <el-form-item :label="t('lease.historyFilterVersion')">
                <el-select v-model="version" clearable size="small" style="width: 120px">
                  <el-option label="IPv4" value="4" />
                  <el-option label="IPv6" value="6" />
                </el-select>
              </el-form-item>
              <el-form-item :label="t('lease.historyFilterLeaseRisk')">
                <el-select v-model="leaseRisk" clearable size="small" style="width: 140px" :placeholder="t('lease.historyFilterLeaseRiskAll')">
                  <el-option :label="t('lease.historyFilterLeaseRiskNormal')" value="normal" />
                  <el-option :label="t('lease.historyFilterLeaseRiskShort')" value="short" />
                  <el-option :label="t('lease.historyFilterLeaseRiskLong')" value="long" />
                </el-select>
              </el-form-item>
            </el-form>
          </div>

          <div class="filter-group">
            <div class="group-title">{{ t('lease.historyGroupAnomaly') }}</div>
            <el-form inline class="filters" @submit.prevent>
              <el-form-item :label="t('lease.historyFilterAnomalyType')">
                <el-select
                  v-model="anomalyType"
                  clearable
                  size="small"
                  style="width: 160px"
                  :placeholder="t('lease.historyFilterAnomalyAll')"
                >
                  <el-option :label="t('lease.historyAnomalyConflict')" value="CONFLICT" />
                  <el-option :label="t('lease.historyAnomalyDeclined')" value="DECLINED" />
                  <el-option :label="t('lease.historyAnomalyAbandoned')" value="ABANDONED" />
                  <el-option :label="t('lease.historyAnomalyOffline')" value="OFFLINE" />
                </el-select>
              </el-form-item>
              <el-form-item :label="t('lease.historyFilterTimeRange')">
                <el-date-picker
                  v-model="dateRange"
                  type="datetimerange"
                  :start-placeholder="t('lease.historyFilterStartTime')"
                  :end-placeholder="t('lease.historyFilterEndTime')"
                  size="small"
                  value-format="x"
                  style="width: 310px"
                />
              </el-form-item>
            </el-form>
          </div>

          <div class="actions-fixed">
            <el-button class="audit-action-btn" size="small" type="primary" :loading="loading" @click="handleSearch">
              {{ t('common.search') }}
            </el-button>
            <el-button class="audit-action-btn" size="small" :disabled="loading" @click="handleReset">{{ t('common.reset') }}</el-button>

            <el-popover placement="left" :width="340" trigger="click">
              <template #reference>
                <el-button class="audit-action-btn" size="small" :loading="exporting">{{ t('lease.historyExportAudit') }}</el-button>
              </template>
              <div class="export-panel">
                <div class="export-title">{{ t('lease.historyExportFieldTitle') }}</div>
                <el-checkbox-group v-model="exportFields" class="field-grid">
                  <el-checkbox v-for="field in exportFieldOptions" :key="field.key" :label="field.key">
                    {{ field.label }}
                  </el-checkbox>
                </el-checkbox-group>
                <div class="export-actions">
                  <el-button size="small" @click="resetExportFields">{{ t('lease.historyExportDefault') }}</el-button>
                  <el-button
                    size="small"
                    type="primary"
                    :loading="exporting"
                    :disabled="!exportFields.length"
                    @click="exportAuditReport"
                  >
                    {{ t('lease.historyExportBtn') }}
                  </el-button>
                </div>
              </div>
            </el-popover>
          </div>
        </div>
      </section>

      <el-empty v-if="!loading && !displayRows.length" :description="t('lease.historyEmpty')" :image-size="56" class="table-empty">
        <template #extra>
          <el-button size="small" @click="handleReset">{{ t('lease.historyResetFilter') }}</el-button>
        </template>
      </el-empty>

      <el-table v-else v-loading="loading" class="lease-table" :data="displayRows" border stripe row-key="id">
        <el-table-column :label="t('lease.ip')" min-width="170">
          <template #default="{ row }">
            <div class="ip-cell">
              <span class="ip">{{ row.ip }}</span>
              <el-tag type="info" size="small">IPv{{ row.version }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="mac" :label="t('lease.mac')" min-width="150" />
        <el-table-column prop="clientId" label="Client ID" min-width="170" show-overflow-tooltip />
        <el-table-column prop="poolName" :label="t('lease.pool')" min-width="150" show-overflow-tooltip />
        <el-table-column :label="t('lease.state')" min-width="130">
          <template #default="{ row }">
            <el-tag :type="stateMeta(row).color || 'info'" effect="light">{{ stateMeta(row).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.historyColAbnormal')" min-width="130">
          <template #default="{ row }">
            <el-tag v-if="abnormalType(row)" type="danger" effect="light">{{ abnormalType(row) }}</el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.startsAt')" min-width="180">
          <template #default="{ row }">{{ formatTs(row.startsAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('lease.endsAt')" min-width="220">
          <template #default="{ row }">
            <div class="expires" :class="expiryClass(row)">
              <span>{{ formatTs(row.endsAt) }}</span>
              <el-tag v-if="expiryTag(row)" size="small" :type="expiryTag(row)?.type" effect="plain">
                {{ expiryTag(row)?.label }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.historyColDuration')" min-width="140">
          <template #default="{ row }">
            <span :class="durationClass(row)">{{ durationText(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.historyColActions')" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDetail(row)">{{ t('lease.historyColDetail') }}</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          background
          layout="total, sizes, prev, pager, next, jumper"
          :page-sizes="[10, 20, 50, 100]"
          :total="pagination.total"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>

    <el-drawer v-model="detailVisible" :title="t('lease.historyDrawerTitle')" size="44%" destroy-on-close>
      <template v-if="detailLease">
        <div class="drawer-block">
          <div class="drawer-title">{{ t('lease.historyDrawerBase') }}</div>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="IP">{{ detailLease.ip }}</el-descriptions-item>
            <el-descriptions-item label="MAC">{{ detailLease.mac }}</el-descriptions-item>
            <el-descriptions-item label="Client ID">{{ detailLease.clientId || '-' }}</el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerHostname')">{{ detailLease.hostname || '-' }}</el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerPool')">{{ detailLease.poolName || detailLease.poolId }}</el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerState')">
              <el-tag :type="stateMeta(detailLease).color || 'info'" effect="light">{{ stateMeta(detailLease).label }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerStartsAt')">{{ formatTs(detailLease.startsAt) }}</el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerEndsAt')">{{ formatTs(detailLease.endsAt) }}</el-descriptions-item>
            <el-descriptions-item :label="t('lease.historyDrawerDuration')">{{ durationText(detailLease) }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="drawer-block">
          <div class="drawer-title">{{ t('lease.historyDrawerTimeline') }}</div>
          <el-timeline>
            <el-timeline-item :timestamp="formatTs(detailLease.startsAt)" type="primary">{{ t('lease.historyDrawerCreated') }}</el-timeline-item>
            <el-timeline-item v-if="detailLease.t1" :timestamp="detailLease.t1" type="success">{{ t('lease.historyDrawerT1') }}</el-timeline-item>
            <el-timeline-item v-if="detailLease.t2" :timestamp="detailLease.t2" type="warning">{{ t('lease.historyDrawerT2') }}</el-timeline-item>
            <el-timeline-item :timestamp="formatTs(detailLease.endsAt)" :type="abnormalType(detailLease) ? 'danger' : 'info'">
              {{ abnormalType(detailLease) ? t('lease.historyDrawerAbnormalEnd') : t('lease.historyDrawerNormalEnd') }}
            </el-timeline-item>
          </el-timeline>
        </div>

        <div class="drawer-block">
          <div class="drawer-title">{{ t('lease.historyDrawerAnomalyTitle') }}</div>
          <el-empty v-if="!abnormalType(detailLease)" :description="t('lease.historyDrawerNoAnomaly')" :image-size="48" />
          <el-alert
            v-else
            type="error"
            :closable="false"
            :title="t('lease.historyDrawerAnomalyAlert', { type: abnormalType(detailLease) })"
            show-icon
          />
        </div>

        <div class="drawer-block">
          <div class="drawer-title">{{ t('lease.historyDrawerManualTitle') }}</div>
          <el-empty v-if="!manualAuditRecords(detailLease).length" :description="t('lease.historyDrawerNoManual')" :image-size="48" />
          <el-timeline v-else>
            <el-timeline-item
              v-for="record in manualAuditRecords(detailLease)"
              :key="record.id"
              :timestamp="record.ts"
              type="info"
            >
              {{ record.action }}（{{ record.actor }}）
            </el-timeline-item>
          </el-timeline>
        </div>

        <div class="drawer-actions">
          <el-button size="small" :loading="detailExporting" @click="exportDetail(detailLease)">{{ t('lease.historyDrawerExportDetail') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import { useLeaseStore } from '@/store/lease';
import { useTenantStore } from '@/store/tenant';
import type { Lease, LeaseFilter, LeaseState } from '@/types/lease';
import { formatTs, timeLeft } from '@/utils/time';
import { groupedLeaseStates, leaseStateMeta } from '@/shared/utils/leaseState';
import { showError, showSuccess } from '@/shared/errors/messageToast';

type DateQuickKey = '24h' | '7d' | '30d' | '90d' | '1y' | 'custom';

interface SavedHistoryFilter {
  ip?: string;
  mac?: string;
  clientId?: string;
  poolId?: string;
  state?: LeaseState[];
  version?: '4' | '6' | '';
  dateRange?: [number, number];
  anomalyType?: string;
  leaseRisk?: string;
}

interface ExportFieldOption {
  key: string;
  label: string;
}

const { t } = useI18n();
const leaseStore = useLeaseStore();
const tenantStore = useTenantStore();

const rows = computed(() => leaseStore.history);
const loading = computed(() => leaseStore.loadingHistory);
const pagination = reactive({ ...leaseStore.historyPagination });
const filters = reactive<LeaseFilter>({ ...leaseStore.historyFilters });
const dateRange = ref<[number, number] | null>(null);
const version = ref<'4' | '6' | ''>('');
const anomalyType = ref('');
const leaseRisk = ref<'normal' | 'short' | 'long' | ''>('');
const activeQuickTime = ref<DateQuickKey>('7d');

const detailVisible = ref(false);
const detailLease = ref<Lease | null>(null);
const detailExporting = ref(false);

const savingFilter = ref(false);
const exporting = ref(false);
const savedFilterKey = ref('');
const storageKey = computed(() => `lease-history-audit-filters-${tenantStore.currentTenantId || 'global'}`);
const savedFilters = ref<Record<string, SavedHistoryFilter>>(loadSavedFilters());

const exportFieldOptions = computed<ExportFieldOption[]>(() => [
  { key: 'ip', label: 'IP' },
  { key: 'mac', label: 'MAC' },
  { key: 'clientId', label: 'Client ID' },
  { key: 'hostname', label: t('lease.historyFieldHostname') },
  { key: 'poolName', label: t('lease.historyFieldPool') },
  { key: 'state', label: t('lease.historyFieldState') },
  { key: 'abnormal', label: t('lease.historyFieldAbnormal') },
  { key: 'version', label: t('lease.historyFieldVersion') },
  { key: 'startsAt', label: t('lease.historyFieldStartsAt') },
  { key: 'endsAt', label: t('lease.historyFieldEndsAt') },
  { key: 'duration', label: t('lease.historyFieldDuration') }
]);
const defaultExportFields = ['ip', 'mac', 'clientId', 'poolName', 'state', 'abnormal', 'startsAt', 'endsAt', 'duration'];
const exportFields = ref<string[]>([...defaultExportFields]);

const quickTimeItems = computed<Array<{ key: DateQuickKey; label: string }>>(() => [
  { key: '24h', label: t('lease.historyQuick24h') },
  { key: '7d', label: t('lease.historyQuick7d') },
  { key: '30d', label: t('lease.historyQuick30d') },
  { key: '90d', label: t('lease.historyQuick90d') },
  { key: '1y', label: t('lease.historyQuick1y') },
  { key: 'custom', label: t('lease.historyQuickCustom') }
]);

const allLeaseStates = groupedLeaseStates();
const stateOptions = [
  ...allLeaseStates.active,
  ...allLeaseStates.transient,
  ...allLeaseStates.error,
  ...allLeaseStates.other
].map((state) => ({
  value: state,
  label: leaseStateMeta(state).label
}));

const abnormalLabelMap = computed<Record<string, string>>(() => ({
  CONFLICT: t('lease.historyAnomalyConflict'),
  DECLINED: t('lease.historyAnomalyDeclined'),
  ABANDONED: t('lease.historyAnomalyAbandoned'),
  OFFLINE: t('lease.historyAnomalyOffline')
}));

const abnormalType = (lease: Lease) => {
  if (['CONFLICT', 'DECLINED', 'ABANDONED', 'OFFLINE'].includes(lease.state)) {
    return abnormalLabelMap.value[lease.state] || lease.state;
  }
  return '';
};

const durationHours = (lease: Lease) => {
  const start = new Date(lease.startsAt).getTime();
  const end = new Date(lease.endsAt).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0;
  return (end - start) / 3600000;
};

const durationText = (lease: Lease) => {
  const value = durationHours(lease);
  if (!value) return '-';
  if (value >= 24) return t('lease.historyDurationDay', { value: (value / 24).toFixed(1) });
  return t('lease.historyDurationHour', { value: value.toFixed(1) });
};

const durationClass = (lease: Lease) => {
  const value = durationHours(lease);
  if (!value) return '';
  if (value < 1) return 'duration-short';
  if (value > 168) return 'duration-long';
  return '';
};

const isNormalEnd = (lease: Lease) => ['EXPIRED', 'RELEASED'].includes(lease.state) || !abnormalType(lease);

const displayRows = computed(() => {
  const stateSet = filters.state?.length ? new Set(filters.state) : null;
  return rows.value.filter((item) => {
    if (filters.ip && !item.ip?.toLowerCase().includes(filters.ip.toLowerCase())) return false;
    if (filters.mac && !item.mac?.toLowerCase().includes(filters.mac.toLowerCase())) return false;
    if (filters.clientId && !item.clientId?.toLowerCase().includes(filters.clientId.toLowerCase())) return false;
    if (filters.poolId) {
      const poolText = `${item.poolName || ''} ${item.poolId || ''}`.toLowerCase();
      if (!poolText.includes(filters.poolId.toLowerCase())) return false;
    }
    if (stateSet && !stateSet.has(item.state)) return false;
    if (version.value && item.version !== Number(version.value)) return false;
    if (anomalyType.value && item.state !== anomalyType.value) return false;

    const hours = durationHours(item);
    if (leaseRisk.value === 'short' && !(hours > 0 && hours < 1)) return false;
    if (leaseRisk.value === 'long' && hours <= 168) return false;
    if (leaseRisk.value === 'normal' && (hours > 0 && (hours < 1 || hours > 168))) return false;

    if (dateRange.value?.length === 2) {
      const [start, end] = dateRange.value;
      const ts = new Date(item.endsAt).getTime();
      if (Number.isFinite(ts) && (ts < start || ts > end)) return false;
    }
    return true;
  });
});

const stats = computed(() => {
  const list = displayRows.value;
  const total = list.length;
  const normalEnd = list.filter((item) => isNormalEnd(item)).length;
  const abnormalEnd = list.filter((item) => !isNormalEnd(item)).length;
  const avg = list.length
    ? list.reduce((sum, item) => sum + durationHours(item), 0) / list.length
    : 0;
  return {
    total,
    normalEnd,
    abnormalEnd,
    avgLeaseDuration: avg >= 24 ? t('lease.historyDurationDay', { value: (avg / 24).toFixed(1) }) : t('lease.historyDurationHour', { value: avg.toFixed(1) })
  };
});

const groupedByDay = computed(() => {
  const map = new Map<string, { total: number; abnormal: number }>();
  displayRows.value.forEach((item) => {
    const key = (item.endsAt || '').slice(0, 10) || 'N/A';
    const current = map.get(key) || { total: 0, abnormal: 0 };
    current.total += 1;
    if (abnormalType(item)) current.abnormal += 1;
    map.set(key, current);
  });
  return Array.from(map.entries())
    .map(([day, value]) => ({ day, ...value }))
    .sort((a, b) => (a.day > b.day ? 1 : -1));
});

const leaseTrendOption = computed(() => {
  if (!groupedByDay.value.length) return null;
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 20, top: 24, bottom: 28 },
    xAxis: { type: 'category', data: groupedByDay.value.map((item) => item.day) },
    yAxis: { type: 'value', name: t('lease.historyChartCount') },
    series: [
      {
        type: 'line',
        smooth: true,
        data: groupedByDay.value.map((item) => item.total),
        lineStyle: { color: '#2563eb', width: 2 },
        areaStyle: { color: 'rgba(37, 99, 235, 0.14)' }
      }
    ]
  };
});

const abnormalRateOption = computed(() => {
  if (!groupedByDay.value.length) return null;
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 20, top: 24, bottom: 28 },
    xAxis: { type: 'category', data: groupedByDay.value.map((item) => item.day) },
    yAxis: { type: 'value', name: t('lease.historyChartPercent'), min: 0, max: 100 },
    series: [
      {
        type: 'line',
        smooth: true,
        data: groupedByDay.value.map((item) => Number(((item.abnormal / Math.max(item.total, 1)) * 100).toFixed(2))),
        lineStyle: { color: '#dc2626', width: 2 },
        areaStyle: { color: 'rgba(220, 38, 38, 0.14)' }
      }
    ]
  };
});

watch(
  () => leaseStore.historyFilters,
  (val) => {
    Object.assign(filters, val || {});
    version.value = val.version ? (String(val.version) as '4' | '6') : '';
    if (val.startTime && val.endTime) {
      dateRange.value = [new Date(val.startTime).getTime(), new Date(val.endTime).getTime()];
    } else {
      dateRange.value = null;
    }
  },
  { deep: true, immediate: true }
);

watch(
  () => leaseStore.historyPagination,
  (val) => {
    Object.assign(pagination, val);
  },
  { deep: true }
);

watch(storageKey, () => {
  savedFilters.value = loadSavedFilters();
  savedFilterKey.value = '';
});

watch(
  () => tenantStore.currentTenantId,
  () => {
    pagination.page = 1;
    void loadHistory({ page: 1 });
  }
);

const buildQuery = (overrides?: Partial<LeaseFilter>) => {
  const startTime = dateRange.value?.[0] ? new Date(dateRange.value[0]).toISOString() : undefined;
  const endTime = dateRange.value?.[1] ? new Date(dateRange.value[1]).toISOString() : undefined;
  const base: LeaseFilter = {
    ...filters,
    version: version.value ? (Number(version.value) as 4 | 6) : undefined,
    startTime,
    endTime,
    tenantId: tenantStore.currentTenantId || undefined,
    page: overrides?.page ?? pagination.page,
    pageSize: overrides?.pageSize ?? pagination.pageSize
  };
  return { ...base, ...overrides };
};

const loadHistory = async (overrides?: Partial<LeaseFilter>) => {
  try {
    await leaseStore.fetchHistory(buildQuery(overrides));
  } catch {
    showError(t('lease.historyLoadFail'));
  }
};

const handleSearch = async () => {
  pagination.page = 1;
  await loadHistory({ page: 1 });
  showSuccess(t('lease.historySearchApplied'));
};

const handleReset = async () => {
  activeQuickTime.value = '7d';
  applyQuickTime('7d');
  version.value = '';
  anomalyType.value = '';
  leaseRisk.value = '';
  Object.assign(filters, {
    page: 1,
    pageSize: pagination.pageSize,
    ip: '',
    mac: '',
    clientId: '',
    poolId: '',
    state: [],
    startTime: undefined,
    endTime: undefined,
    keyword: undefined,
    version: undefined
  });
  pagination.page = 1;
  await loadHistory({ page: 1, startTime: undefined, endTime: undefined, version: undefined });
  showSuccess(t('lease.historySearchReset'));
};

const handlePageChange = async (page: number) => {
  pagination.page = page;
  await loadHistory({ page });
};

const handleSizeChange = async (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  await loadHistory({ pageSize: size, page: 1 });
};

const refresh = async () => {
  await loadHistory();
};

const applyQuickTime = (key: DateQuickKey) => {
  activeQuickTime.value = key;
  if (key === 'custom') return;
  const now = Date.now();
  const durationMap: Record<Exclude<DateQuickKey, 'custom'>, number> = {
    '24h': 24 * 60 * 60 * 1000,
    '7d': 7 * 24 * 60 * 60 * 1000,
    '30d': 30 * 24 * 60 * 60 * 1000,
    '90d': 90 * 24 * 60 * 60 * 1000,
    '1y': 365 * 24 * 60 * 60 * 1000
  };
  dateRange.value = [now - durationMap[key], now];
};

const stateMeta = (lease: Lease) => leaseStateMeta(lease.state);

const expiryTag = (lease: Lease): { type: 'danger' | 'warning'; label: string } | null => {
  const end = new Date(lease.endsAt).getTime();
  if (!Number.isFinite(end)) return null;
  if (end <= Date.now()) return { type: 'danger', label: t('lease.historyEnded') };
  const left = timeLeft(lease.endsAt);
  return left ? { type: 'warning', label: left } : null;
};

const expiryClass = (lease: Lease) => {
  const end = new Date(lease.endsAt).getTime();
  if (!Number.isFinite(end)) return '';
  if (end <= Date.now()) return 'is-expired';
  if (end - Date.now() <= 24 * 60 * 60 * 1000) return 'is-expiring';
  return '';
};

const openDetail = (lease: Lease) => {
  detailLease.value = lease;
  detailVisible.value = true;
};

const manualAuditRecords = (lease: Lease) => {
  const records: Array<{ id: string; ts: string; actor: string; action: string }> = [];
  if (lease.renewCount && lease.renewCount > 0) {
    records.push({
      id: `${lease.id}-renew`,
      ts: lease.t1 || lease.startsAt,
      actor: 'system',
      action: t('lease.historyDrawerAutoRenew', { count: lease.renewCount })
    });
  }
  if (lease.relayAgent) {
    records.push({
      id: `${lease.id}-relay`,
      ts: lease.endsAt,
      actor: 'relay',
      action: t('lease.historyDrawerRelayReport', { agent: lease.relayAgent })
    });
  }
  return records;
};

const csvEscape = (value: unknown) => {
  const text = String(value ?? '');
  if (text.includes(',') || text.includes('"') || text.includes('\n')) {
    return `"${text.replace(/"/g, '""')}"`;
  }
  return text;
};

const mapFieldValue = (row: Lease, key: string) => {
  switch (key) {
    case 'state':
      return leaseStateMeta(row.state).label;
    case 'abnormal':
      return abnormalType(row) || '—';
    case 'startsAt':
      return formatTs(row.startsAt);
    case 'endsAt':
      return formatTs(row.endsAt);
    case 'poolName':
      return row.poolName || row.poolId;
    case 'duration':
      return durationText(row);
    default:
      return (row as Record<string, unknown>)[key] ?? '';
  }
};

const downloadCsv = (filename: string, lines: string[]) => {
  const blob = new Blob([`\uFEFF${lines.join('\n')}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
};

const exportRows = (dataset: Lease[], filenamePrefix: string) => {
  if (!dataset.length) {
    showError(t('lease.historyNoExportData'));
    return;
  }
  if (!exportFields.value.length) {
    showError(t('lease.historyNoExportField'));
    return;
  }
  const fields = exportFieldOptions.value.filter((item) => exportFields.value.includes(item.key));
  const header = fields.map((item) => csvEscape(item.label)).join(',');
  const rowsCsv = dataset.map((item) => fields.map((field) => csvEscape(mapFieldValue(item, field.key))).join(','));
  const filterSnapshot = [
    `${t('lease.historyCsvExportTime')},${csvEscape(new Date().toLocaleString())}`,
    `${t('lease.historyCsvTimeRange')},${csvEscape(dateRange.value ? `${new Date(dateRange.value[0]).toLocaleString()} ~ ${new Date(dateRange.value[1]).toLocaleString()}` : t('lease.historyCsvAll'))}`,
    `${t('lease.historyCsvVersion')},${csvEscape(version.value || t('lease.historyCsvAll'))}`,
    `${t('lease.historyCsvAbnormal')},${csvEscape(anomalyType.value || t('lease.historyCsvAll'))}`
  ];
  downloadCsv(`${filenamePrefix}-${Date.now()}.csv`, [...filterSnapshot, '', header, ...rowsCsv]);
  showSuccess(t('lease.historyExportSuccess'));
};

const resetExportFields = () => {
  exportFields.value = [...defaultExportFields];
};

const exportAuditReport = async () => {
  exporting.value = true;
  try {
    exportRows(displayRows.value, 'lease-audit-report');
  } catch {
    showError(t('lease.historyExportFail'));
  } finally {
    exporting.value = false;
  }
};

const exportDetail = async (lease: Lease) => {
  detailExporting.value = true;
  try {
    const details = [
      `${t('lease.historyCsvField')},${t('lease.historyCsvValue')}`,
      `IP,${csvEscape(lease.ip)}`,
      `MAC,${csvEscape(lease.mac)}`,
      `Client ID,${csvEscape(lease.clientId || '-')}`,
      `${t('lease.historyCsvPool')},${csvEscape(lease.poolName || lease.poolId)}`,
      `${t('lease.historyCsvState')},${csvEscape(leaseStateMeta(lease.state).label)}`,
      `${t('lease.historyCsvAbnormal')},${csvEscape(abnormalType(lease) || '—')}`,
      `${t('lease.historyCsvStartsAt')},${csvEscape(formatTs(lease.startsAt))}`,
      `${t('lease.historyCsvEndsAt')},${csvEscape(formatTs(lease.endsAt))}`,
      `${t('lease.historyCsvDuration')},${csvEscape(durationText(lease))}`
    ];
    downloadCsv(`lease-lifecycle-${lease.ip}-${Date.now()}.csv`, details);
    showSuccess(t('lease.historyDetailExportSuccess'));
  } catch {
    showError(t('lease.historyDetailExportFail'));
  } finally {
    detailExporting.value = false;
  }
};

function loadSavedFilters() {
  try {
    return JSON.parse(localStorage.getItem(storageKey.value) || '{}') as Record<string, SavedHistoryFilter>;
  } catch {
    return {} as Record<string, SavedHistoryFilter>;
  }
}

const saveFilterCondition = async () => {
  savingFilter.value = true;
  try {
    const key = `${t('lease.historySaveFilterPrefix')}${new Date().toLocaleString()}`;
    savedFilters.value = {
      ...savedFilters.value,
      [key]: {
        ip: filters.ip,
        mac: filters.mac,
        clientId: filters.clientId,
        poolId: filters.poolId,
        state: filters.state,
        version: version.value,
        dateRange: dateRange.value || undefined,
        anomalyType: anomalyType.value || undefined,
        leaseRisk: leaseRisk.value || undefined
      }
    };
    localStorage.setItem(storageKey.value, JSON.stringify(savedFilters.value));
    showSuccess(t('lease.historySaveFilterSuccess'));
  } catch {
    showError(t('lease.historySaveFilterFail'));
  } finally {
    savingFilter.value = false;
  }
};

const loadFilterCondition = async (key?: string) => {
  if (!key) return;
  const saved = savedFilters.value[key];
  if (!saved) return;

  Object.assign(filters, saved, { page: 1 });
  version.value = saved.version || '';
  dateRange.value = saved.dateRange || null;
  anomalyType.value = saved.anomalyType || '';
  leaseRisk.value = (saved.leaseRisk as 'normal' | 'short' | 'long' | '') || '';
  pagination.page = 1;

  await loadHistory({
    ...saved,
    page: 1,
    version: saved.version ? (Number(saved.version) as 4 | 6) : undefined,
    startTime: saved.dateRange?.[0] ? new Date(saved.dateRange[0]).toISOString() : undefined,
    endTime: saved.dateRange?.[1] ? new Date(saved.dateRange[1]).toISOString() : undefined
  });
  showSuccess(t('lease.historyLoadFilterSuccess'));
};

onMounted(() => {
  applyQuickTime('7d');
  void loadHistory();
});
</script>

<style scoped>
.lease-page {
  padding: 12px;
  background: #f5f7fa;
  min-height: 100%;
}

.table-card {
  width: 100%;
  min-height: calc(100vh - 140px);
}

.table-card :deep(.el-card__body) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.top-ops-bar {
  display: grid;
  grid-template-columns: auto minmax(460px, 1fr) auto;
  justify-content: start;
  gap: 16px;
  align-items: center;
  margin-bottom: 12px;
  padding: 12px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
}

.ops-left h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: #6b7280;
}

.ops-right {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.overview-grid {
  display: grid;
  gap: 12px;
  margin-bottom: 12px;
}

.overview-grid.four-cols {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.overview-card {
  border-radius: 10px;
  min-height: 118px;
}

.overview-card.is-alert {
  background: #fff7ed;
}

.overview-label {
  color: #6b7280;
  font-size: 13px;
}

.overview-value {
  margin-top: 8px;
  font-size: 30px;
  font-weight: 800;
  line-height: 1.15;
}

.overview-desc {
  margin-top: 8px;
  color: #9ca3af;
  font-size: 12px;
}

.analysis-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.analysis-card {
  border-radius: 10px;
}

.card-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-title {
  font-weight: 600;
}

.card-sub {
  color: #6b7280;
  font-size: 12px;
}

.chart {
  min-height: 250px;
}

.filters-panel {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  background: #fff;
  padding: 12px;
  margin-bottom: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.quick-filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.quick-tag {
  cursor: pointer;
}

.audit-filters-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr auto;
  gap: 10px;
  align-items: start;
}

.filter-group {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 8px;
  min-height: 108px;
}

.group-title {
  font-size: 12px;
  font-weight: 600;
  color: #4b5563;
  margin-bottom: 8px;
}

.filters {
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  row-gap: 8px;
}

.filters :deep(.el-form-item) {
  margin-bottom: 0;
}

.actions-fixed {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: stretch;
}

.audit-action-btn {
  width: 120px;
}

.actions-fixed :deep(.el-button + .el-button) {
  margin-left: 0;
}

.export-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.export-title {
  font-size: 13px;
  color: #4b5563;
  font-weight: 600;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 10px;
}

.export-actions {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.lease-table {
  width: 100%;
}

.ip-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ip {
  font-weight: 600;
  color: #111827;
}

.expires {
  display: flex;
  align-items: center;
  gap: 8px;
}

.expires.is-expiring {
  color: #b45309;
  font-weight: 600;
}

.expires.is-expired {
  color: #dc2626;
  font-weight: 700;
}

.duration-short {
  color: #f59e0b;
  font-weight: 600;
}

.duration-long {
  color: #7c3aed;
  font-weight: 600;
}

.table-empty {
  margin: 18px 0 10px;
  border: 1px dashed #d1d5db;
  border-radius: 10px;
  background: #fff;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid #f0f2f5;
}

.drawer-block {
  margin-bottom: 16px;
}

.drawer-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 8px;
}

.drawer-actions {
  margin-top: 8px;
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 1600px) {
  .audit-filters-grid {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 1400px) {
  .overview-grid.four-cols {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .analysis-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 1200px) {
  .top-ops-bar {
    grid-template-columns: 1fr;
    align-items: flex-start;
  }

  .audit-filters-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .overview-grid.four-cols {
    grid-template-columns: 1fr;
  }
}
</style>