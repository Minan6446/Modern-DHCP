<template>
  <div class="lease-page">
    <el-card v-loading="loading" class="surface-card table-card">
      <div class="top-ops-bar">
        <div class="ops-left">
          <h3>{{ t('lease.tabActive') }}</h3>
          <p class="desc">{{ t('lease.activeHint') }}</p>
        </div>
        <div class="ops-middle" />
        <div class="ops-right">
          <el-button size="small" :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
        </div>
      </div>

      <section class="overview-grid">
        <el-card
          v-for="card in statCards"
          :key="card.key"
          shadow="never"
          class="overview-card"
          :class="[
            activeQuickFilter === card.key ? 'is-active' : '',
            card.alert ? 'is-alert' : '',
            card.clickable ? 'is-clickable' : ''
          ]"
          @click="card.clickable ? applyQuickFilter(card.key) : undefined"
        >
          <div class="overview-label">{{ card.label }}</div>
          <div class="overview-value" :style="{ color: card.color }">{{ card.value }}</div>
          <div class="overview-desc">{{ card.desc }}</div>
        </el-card>
      </section>

      <section class="filters-panel">
        <div class="quick-filters">
          <el-tag
            v-for="item in quickFilterItems"
            :key="item.key"
            :type="activeQuickFilter === item.key ? 'primary' : 'info'"
            :effect="activeQuickFilter === item.key ? 'dark' : 'plain'"
            class="quick-tag"
            @click="applyQuickFilter(item.key)"
          >
            {{ item.label }}
          </el-tag>
        </div>

        <div class="filter-main">
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
            <el-form-item :label="t('lease.activeFilterState')">
              <el-select
                v-model="filters.state"
                multiple
                clearable
                collapse-tags
                collapse-tags-tooltip
                size="small"
                :placeholder="t('lease.activeFilterStatePlaceholder')"
                style="width: 200px"
              >
                <el-option
                  v-for="state in stateOptions"
                  :key="state.value"
                  :label="state.label"
                  :value="state.value"
                />
              </el-select>
            </el-form-item>
          </el-form>

          <div class="filter-actions-right">
            <el-button size="small" type="primary" :loading="loading" @click="handleSearch">
              {{ t('common.search') }}
            </el-button>
            <el-button size="small" :disabled="loading" @click="handleReset">{{ t('common.reset') }}</el-button>
          </div>
        </div>

        <div class="ops-row">
          <div class="ops-row-left">
            <el-button
              size="small"
              type="danger"
              :disabled="!selection.length || !canManage"
              :loading="bulkReleasing"
              @click="handleBulkRelease"
            >
              {{ t('lease.activeBulkRelease') }}
            </el-button>
          </div>
          <div class="ops-row-right">
            <el-button size="small" :loading="savingFilter" @click="saveFilterCondition">{{ t('lease.activeSaveFilter') }}</el-button>
            <el-select
              v-model="savedFilterKey"
              clearable
              size="small"
              :placeholder="t('lease.activeLoadFilter')"
              style="width: 170px"
              @change="loadFilterCondition"
            >
              <el-option v-for="(item, key) in savedFilters" :key="key" :label="key" :value="key" />
            </el-select>
            <el-button size="small" :loading="exporting" @click="exportCsv">{{ t('lease.activeExport') }}</el-button>
            <el-button size="small" :icon="Refresh" :loading="loading" @click="refresh">{{ t('lease.activeRefresh') }}</el-button>
          </div>
        </div>
      </section>

      <div class="selection-tools">
        <div class="selection-info">{{ t('lease.activeSelectionInfo', { sel: selection.length, total: displayRows.length }) }}</div>
        <div class="selection-actions">
          <el-button size="small" :disabled="!displayRows.length" @click="selectAllCurrent">{{ t('lease.activeSelectAll') }}</el-button>
          <el-button size="small" :disabled="!displayRows.length" @click="invertSelection">{{ t('lease.activeInvert') }}</el-button>
          <el-button size="small" :disabled="!selection.length" @click="clearSelection">{{ t('lease.activeClear') }}</el-button>
        </div>
      </div>

      <el-empty v-if="!loading && !displayRows.length" :description="t('lease.activeEmpty')" :image-size="56" class="table-empty">
        <template #extra>
          <el-button size="small" @click="handleReset">{{ t('lease.activeResetFilter') }}</el-button>
          <el-button size="small" type="primary" :loading="loading" @click="refresh">{{ t('lease.activeRefreshData') }}</el-button>
        </template>
      </el-empty>

      <el-table
        v-else
        ref="tableRef"
        v-loading="loading"
        class="lease-table"
        :data="displayRows"
        border
        stripe
        row-key="id"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column :label="t('lease.ip')" min-width="170">
          <template #default="{ row }">
            <div class="ip-cell">
              <span class="ip">{{ row.ip }}</span>
              <el-tag type="info" size="small">IPv{{ row.version }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="mac" :label="t('lease.mac')" min-width="150" />
        <el-table-column prop="clientId" label="Client ID" min-width="160" show-overflow-tooltip />
        <el-table-column prop="poolName" :label="t('lease.pool')" min-width="150" show-overflow-tooltip />
        <el-table-column :label="t('lease.state')" min-width="130">
          <template #default="{ row }">
            <el-tag :type="stateMeta(row).color || 'info'" effect="light">{{ stateMeta(row).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.endsAt')" min-width="210">
          <template #default="{ row }">
            <div class="expires" :class="expiryClass(row)">
              <span>{{ formatTs(row.endsAt) }}</span>
              <el-tag v-if="expiryTag(row)" size="small" :type="expiryTag(row)?.type" effect="plain">
                {{ expiryTag(row)?.label }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('lease.startsAt')" min-width="180">
          <template #default="{ row }">{{ formatTs(row.startsAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('lease.activeActions')" width="250" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDetail(row)">{{ t('lease.activeDetail') }}</el-button>
            <el-button
              size="small"
              type="success"
              :disabled="!canManage || releasingId === row.id"
              :loading="renewingId === row.id"
              @click="handleRenew(row)"
            >
              {{ t('lease.activeRenew') }}
            </el-button>
            <el-button
              size="small"
              type="danger"
              :disabled="!canManage || renewingId === row.id"
              :loading="releasingId === row.id"
              @click="handleRelease(row)"
            >
              {{ t('lease.activeRelease') }}
            </el-button>
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

    <LeaseDetail v-model="detailVisible" :lease="detailLease" />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import LeaseDetail from './components/LeaseDetail.vue';
import { useLeaseStore } from '@/store/lease';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import type { Lease, LeaseFilter, LeaseState } from '@/types/lease';
import { releaseLease, renewLease, exportLeases } from '@/api/leases';
import { formatTs, timeLeft } from '@/utils/time';
import { groupedLeaseStates, leaseStateMeta } from '@/shared/utils/leaseState';
import { showError, showSuccess } from '@/shared/errors/messageToast';

type QuickFilterKey = 'all' | 'in-use' | 'expiring' | 'expired' | 'anomaly';

const leaseStore = useLeaseStore();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const { t } = useI18n();

const loading = computed(() => leaseStore.loadingActive);
const rows = computed(() => leaseStore.active);
const pagination = reactive({ ...leaseStore.activePagination });
const filters = reactive<LeaseFilter>({ ...leaseStore.filters });

const detailVisible = ref(false);
const detailLease = ref<Lease | null>(null);
const selection = ref<Lease[]>([]);
const tableRef = ref<any>();

const activeQuickFilter = ref<QuickFilterKey>('all');
const exporting = ref(false);
const savingFilter = ref(false);
const bulkReleasing = ref(false);
const renewingId = ref<string | null>(null);
const releasingId = ref<string | null>(null);

const savedFilterKey = ref('');
const storageKey = computed(() => `lease-active-filters-${tenantStore.currentTenantId || 'global'}`);
const savedFilters = ref<Record<string, LeaseFilter>>(loadSavedFilters());

const canManage = computed(() => permissionStore.can('lease.manage'));

const quickFilterItems = computed<Array<{ key: QuickFilterKey; label: string }>>(() => [
  { key: 'all', label: t('lease.activeQuickAll') },
  { key: 'in-use', label: t('lease.activeQuickInUse') },
  { key: 'expiring', label: t('lease.activeQuickExpiring') },
  { key: 'expired', label: t('lease.activeQuickExpired') },
  { key: 'anomaly', label: t('lease.activeQuickAnomaly') }
]);

const activeStates = new Set<LeaseState>(['BOUND', 'RENEWING', 'REBINDING']);
const expiredStates = new Set<LeaseState>(['EXPIRED', 'RELEASED']);
const anomalyStates = new Set<LeaseState>(['DECLINED', 'CONFLICT', 'ABANDONED']);
const allLeaseStates = groupedLeaseStates();
const stateOptions = [...allLeaseStates.active, ...allLeaseStates.transient, ...allLeaseStates.error, ...allLeaseStates.other].map((state) => ({
  value: state,
  label: leaseStateMeta(state).label
}));

const isExpiringSoon = (lease: Lease) => {
  const endsAt = new Date(lease.endsAt).getTime();
  if (!Number.isFinite(endsAt)) return false;
  const now = Date.now();
  const diff = endsAt - now;
  return diff > 0 && diff <= 24 * 60 * 60 * 1000;
};

const isExpiredByTime = (lease: Lease) => {
  const endsAt = new Date(lease.endsAt).getTime();
  return Number.isFinite(endsAt) && endsAt <= Date.now();
};

const expiresIn24hCount = computed(() => rows.value.filter((item) => isExpiringSoon(item)).length);
const inUseCount = computed(() => rows.value.filter((item) => activeStates.has(item.state)).length);
const expiredCount = computed(
  () => rows.value.filter((item) => expiredStates.has(item.state) || isExpiredByTime(item)).length
);
const anomalyCount = computed(
  () => rows.value.filter((item) => anomalyStates.has(item.state) || leaseStateMeta(item.state).group === 'error').length
);

const statCards = computed(() => [
  {
    key: 'all' as QuickFilterKey,
    label: t('lease.activeCardAll'),
    value: pagination.total,
    color: '#2563eb',
    desc: t('lease.activeCardAllDesc'),
    alert: false,
    clickable: true
  },
  {
    key: 'in-use' as QuickFilterKey,
    label: t('lease.activeCardInUse'),
    value: inUseCount.value,
    color: '#16a34a',
    desc: t('lease.activeCardInUseDesc'),
    alert: false,
    clickable: true
  },
  {
    key: 'expiring' as QuickFilterKey,
    label: t('lease.activeCardExpiring'),
    value: expiresIn24hCount.value,
    color: '#f59e0b',
    desc: t('lease.activeCardExpiringDesc'),
    alert: expiresIn24hCount.value > 0,
    clickable: true
  },
  {
    key: 'expired' as QuickFilterKey,
    label: t('lease.activeCardExpired'),
    value: expiredCount.value,
    color: '#6b7280',
    desc: t('lease.activeCardExpiredDesc'),
    alert: expiredCount.value > 0,
    clickable: true
  },
  {
    key: 'anomaly' as QuickFilterKey,
    label: t('lease.activeCardAnomaly'),
    value: anomalyCount.value,
    color: '#dc2626',
    desc: t('lease.activeCardAnomalyDesc'),
    alert: anomalyCount.value > 0,
    clickable: true
  }
]);

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

    if (activeQuickFilter.value === 'in-use' && !activeStates.has(item.state)) return false;
    if (activeQuickFilter.value === 'expiring' && !isExpiringSoon(item)) return false;
    if (activeQuickFilter.value === 'expired' && !(expiredStates.has(item.state) || isExpiredByTime(item))) return false;
    if (
      activeQuickFilter.value === 'anomaly' &&
      !(anomalyStates.has(item.state) || leaseStateMeta(item.state).group === 'error')
    ) {
      return false;
    }
    return true;
  });
});

watch(
  () => leaseStore.filters,
  (val) => {
    Object.assign(filters, val || {});
  },
  { deep: true }
);

watch(
  () => leaseStore.activePagination,
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
    void loadData({ page: 1 });
  }
);

const buildQuery = (overrides?: Partial<LeaseFilter>) => ({
  ...filters,
  ...overrides,
  tenantId: tenantStore.currentTenantId || undefined,
  page: overrides?.page ?? pagination.page,
  pageSize: overrides?.pageSize ?? pagination.pageSize
});

const loadData = async (overrides?: Partial<LeaseFilter>) => {
  try {
    await leaseStore.fetchActive(buildQuery(overrides));
    selection.value = [];
  } catch {
    showError(t('lease.loadFail'));
  }
};

const refresh = async () => {
  await loadData();
};

const handleSearch = async () => {
  pagination.page = 1;
  await loadData({ page: 1 });
  showSuccess(t('lease.activeFilterApplied'));
};

const handleReset = async () => {
  activeQuickFilter.value = 'all';
  Object.assign(filters, {
    page: 1,
    pageSize: pagination.pageSize,
    ip: '',
    mac: '',
    clientId: '',
    poolId: '',
    state: []
  });
  pagination.page = 1;
  await loadData({ ...filters, page: 1 });
  showSuccess(t('lease.activeFilterReset'));
};

const applyQuickFilter = (key: QuickFilterKey) => {
  activeQuickFilter.value = key;
};

const handlePageChange = async (page: number) => {
  pagination.page = page;
  await loadData({ page });
};

const handleSizeChange = async (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  await loadData({ pageSize: size, page: 1 });
};

const handleSelectionChange = (items: Lease[]) => {
  selection.value = items;
};

const selectAllCurrent = () => {
  const table = tableRef.value;
  if (!table) return;
  table.clearSelection();
  displayRows.value.forEach((item) => table.toggleRowSelection(item, true));
};

const invertSelection = () => {
  const table = tableRef.value;
  if (!table) return;
  const selected = new Set(selection.value.map((item) => item.id));
  displayRows.value.forEach((item) => table.toggleRowSelection(item, !selected.has(item.id)));
};

const clearSelection = () => {
  tableRef.value?.clearSelection();
  selection.value = [];
};

const openDetail = (lease: Lease) => {
  detailLease.value = lease;
  detailVisible.value = true;
};

const handleRenew = async (lease: Lease) => {
  if (!canManage.value) {
    showError(t('lease.noPermission'));
    return;
  }
  renewingId.value = lease.id;
  try {
    await renewLease(lease.id, { tenantId: tenantStore.currentTenantId || undefined });
    showSuccess(t('lease.renewSuccess'));
    await loadData();
  } catch {
    showError(t('lease.renewFail'));
  } finally {
    renewingId.value = null;
  }
};

const isCancelError = (error: unknown) => error === 'cancel' || error === 'close';

const handleRelease = async (lease: Lease) => {
  if (!canManage.value) {
    showError(t('lease.noPermission'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('lease.activeReleaseConfirm', { ip: lease.ip }), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning'
    });
    releasingId.value = lease.id;
    await releaseLease(lease.id, { tenantId: tenantStore.currentTenantId || undefined });
    showSuccess(t('lease.releaseSuccess'));
    await loadData();
  } catch (error) {
    if (!isCancelError(error)) {
      showError(t('lease.releaseFail'));
    }
  } finally {
    releasingId.value = null;
  }
};

const handleBulkRelease = async () => {
  if (!selection.value.length || bulkReleasing.value) return;
  try {
    await ElMessageBox.confirm(t('lease.activeBulkReleaseConfirm', { count: selection.value.length }), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning'
    });
    bulkReleasing.value = true;
    await Promise.all(
      selection.value.map((item) =>
        releaseLease(item.id, { tenantId: tenantStore.currentTenantId || undefined })
      )
    );
    showSuccess(t('lease.bulkReleaseSuccess'));
    await loadData();
  } catch (error) {
    if (!isCancelError(error)) {
      showError(t('lease.releaseFail'));
    }
  } finally {
    bulkReleasing.value = false;
  }
};

const exportCsv = async () => {
  exporting.value = true;
  try {
    const blob = await exportLeases({ ...buildQuery(), format: 'csv' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `leases-${Date.now()}.csv`;
    link.click();
    URL.revokeObjectURL(url);
    showSuccess(t('lease.activeExportSuccess'));
  } catch {
    showError(t('lease.exportFail'));
  } finally {
    exporting.value = false;
  }
};

function loadSavedFilters() {
  try {
    return JSON.parse(localStorage.getItem(storageKey.value) || '{}') as Record<string, LeaseFilter>;
  } catch {
    return {};
  }
}

const saveFilterCondition = async () => {
  savingFilter.value = true;
  try {
    const key = `${t('lease.activeFilterPrefix')}${new Date().toLocaleString()}`;
    savedFilters.value = {
      ...savedFilters.value,
      [key]: {
        ip: filters.ip,
        mac: filters.mac,
        clientId: filters.clientId,
        poolId: filters.poolId,
        state: filters.state,
        pageSize: pagination.pageSize,
        page: 1
      }
    };
    localStorage.setItem(storageKey.value, JSON.stringify(savedFilters.value));
    showSuccess(t('lease.activeFilterSaved'));
  } catch {
    showError(t('lease.activeFilterSaveFail'));
  } finally {
    savingFilter.value = false;
  }
};

const loadFilterCondition = async (key?: string) => {
  if (!key) return;
  const saved = savedFilters.value[key];
  if (!saved) return;
  Object.assign(filters, saved, { page: 1 });
  pagination.page = 1;
  await loadData({ ...saved, page: 1 });
  showSuccess(t('lease.activeFilterLoaded'));
};

const stateMeta = (lease: Lease) => leaseStateMeta(lease.state);

const expiryTag = (lease: Lease): { type: 'danger' | 'warning'; label: string } | null => {
  if (isExpiredByTime(lease) || expiredStates.has(lease.state)) return { type: 'danger', label: t('lease.activeExpired') };
  if (isExpiringSoon(lease)) return { type: 'warning', label: t('lease.activeExpiring') };
  const left = timeLeft(lease.endsAt);
  return left ? { type: 'warning', label: left } : null;
};

const expiryClass = (lease: Lease) => {
  if (isExpiredByTime(lease) || expiredStates.has(lease.state)) return 'is-expired';
  if (isExpiringSoon(lease)) return 'is-expiring';
  return '';
};

onMounted(() => {
  void loadData();
});
</script>

<style scoped>
.lease-page {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
  --text-muted: #4b5563;
  --subtle-bg: #f9fafb;
  --alert-bg: #fff7ed;
  --hover-border: #93c5fd;
  --active-border: #3b82f6;
  --active-shadow: rgba(59, 130, 246, 0.12);
  --warn-text: #b45309;
  --danger-text: #dc2626;
  --empty-border: #d1d5db;
  --divider: #f0f2f5;
  padding: 12px;
  background: var(--page-bg);
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
  background: var(--surface-bg);
  border: 1px solid var(--surface-border);
  border-radius: 10px;
}

.ops-left h3 {
  margin: 0;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.ops-right {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.overview-card {
  border-radius: 10px;
  min-height: 120px;
  cursor: default;
  transition: all 0.2s ease;
}

.overview-card.is-clickable {
  cursor: pointer;
}

.overview-card.is-clickable:hover {
  border-color: var(--hover-border);
  transform: translateY(-1px);
}

.overview-card.is-active {
  border-color: var(--active-border);
  box-shadow: 0 0 0 1px var(--active-shadow);
}

.overview-card.is-alert {
  background: var(--alert-bg);
}

.overview-label {
  color: var(--text-secondary);
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
  color: var(--text-tertiary);
  font-size: 12px;
}

.filters-panel {
  border: 1px solid var(--surface-border);
  border-radius: 10px;
  background: var(--surface-bg);
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

.filter-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
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

.filter-actions-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.ops-row {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 12px;
  align-items: center;
}

.ops-row-left {
  display: flex;
  align-items: center;
}

.ops-row-right {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.selection-tools {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background: var(--subtle-bg);
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  margin-bottom: 12px;
}

.selection-actions {
  display: flex;
  gap: 8px;
}

.selection-info {
  color: var(--text-muted);
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
  color: var(--text-primary);
}

.expires {
  display: flex;
  align-items: center;
  gap: 8px;
}

.expires.is-expiring {
  color: var(--warn-text);
  font-weight: 600;
}

.expires.is-expired {
  color: var(--danger-text);
  font-weight: 700;
}

.table-empty {
  margin: 18px 0 10px;
  border: 1px dashed var(--empty-border);
  border-radius: 10px;
  background: var(--surface-bg);
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--divider);
}

@media (max-width: 1400px) {
  .overview-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 1200px) {
  .top-ops-bar {
    grid-template-columns: 1fr;
    align-items: flex-start;
  }

  .filter-main {
    grid-template-columns: 1fr;
  }

  .ops-row {
    grid-template-columns: 1fr;
  }

  .ops-row-right {
    justify-content: flex-start;
  }

  .selection-tools {
    flex-direction: column;
    align-items: flex-start;
  }
}

@media (max-width: 768px) {
  .overview-grid {
    grid-template-columns: 1fr;
  }
}
</style>