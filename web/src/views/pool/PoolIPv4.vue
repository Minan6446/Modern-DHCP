<template>
  <div class="pool-page">
    <el-card v-loading="tableLoading" class="surface-card table-card">
      <section class="page-header">
        <h3>{{ t('pool.ipv4Title') }}</h3>
        <p class="desc">{{ t('pool.monitorTitle') }}</p>
      </section>

      <div class="filter-bar surface-card">
        <div class="bar-left">
          <el-form :model="filters" inline class="filters" @submit.prevent>
            <el-form-item :label="t('pool.search')">
              <el-input v-model="filters.keyword" :placeholder="t('pool.search')" clearable size="small" />
            </el-form-item>
            <el-form-item :label="t('pool.status')">
              <el-select v-model="filters.status" clearable :placeholder="t('pool.allStatus')" style="width: 140px" size="small">
                <el-option :label="t('pool.statusActive')" value="active" />
                <el-option :label="t('pool.statusWarning')" value="warning" />
                <el-option :label="t('pool.statusDisabled')" value="disabled" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('pool.usageRangeLabel')">
              <el-select v-model="filters.usageRange" clearable :placeholder="t('pool.all')" size="small" style="width: 150px">
                <el-option label="0% - 30%" value="0-30" />
                <el-option label="30% - 60%" value="30-60" />
                <el-option label="60% - 80%" value="60-80" />
                <el-option label="80% - 100%" value="80-100" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button size="small" type="primary" class="btn-main query-btn" @click="handleSearch">{{ t('common.search') }}</el-button>
              <el-button size="small" class="btn-secondary reset-btn" @click="handleReset">{{ t('common.reset') }}</el-button>
            </el-form-item>
          </el-form>
        </div>
        <div class="bar-right">
          <el-button size="small" class="btn-secondary template-btn" @click="downloadTemplate">{{ t('pool.downloadTemplate') }}</el-button>
          <el-button size="small" type="primary" class="btn-main export-btn" :loading="exporting" @click="exportCsv">{{ t('pool.exportCsv') }}</el-button>
          <el-button size="small" type="primary" class="btn-main import-btn" :loading="importing" :disabled="!canManage" @click="openImport"
            >{{ t('pool.importCsv') }}</el-button
          >
          <el-button size="small" class="btn-secondary refresh-btn" :icon="Refresh" :disabled="tableLoading" @click="fetchPools">{{
            t('common.refresh')
          }}</el-button>
          <el-button size="small" type="primary" class="btn-main create-btn" :icon="Plus" :disabled="!canManage" @click="openCreate">
            {{ t('pool.create') }}
          </el-button>
        </div>
      </div>

      <section class="overview-grid">
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.totalPools') }}</div>
          <div class="overview-value">{{ pagination.total }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.enabledCount') }}</div>
          <div class="overview-value">{{ enabledCount }}</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.avgUsage') }}</div>
          <div class="overview-value">{{ overviewAvgUsage.toFixed(1) }}%</div>
        </el-card>
        <el-card shadow="never" class="overview-card">
          <div class="overview-label">{{ t('pool.highUsageCount') }}</div>
          <div class="overview-value">{{ overviewHighUsageCount }}</div>
        </el-card>
      </section>

      <div v-if="exporting || importing" class="progress-wrap">
        <div v-if="exporting" class="progress-item">
          <span>{{ t('pool.exportProgressLabel') }}</span>
          <el-progress :percentage="exportProgress" />
        </div>
        <div v-if="importing" class="progress-item">
          <span>{{ t('pool.importProgressLabel') }}</span>
          <el-progress :percentage="importProgress" status="success" />
        </div>
      </div>

      <div class="selection-tools">
        <div class="selection-info">{{ t('pool.selectionInfo', { selected: selectedIds.length, total: pools.length }) }}</div>
        <div class="selection-actions">
          <el-button size="small" class="btn-secondary" :disabled="!pools.length" @click="selectAllCurrent"
            >{{ t('pool.selectAll') }}</el-button
          >
          <el-button size="small" class="btn-secondary" :disabled="!pools.length" @click="invertSelection"
            >{{ t('pool.invertSelection') }}</el-button
          >
          <el-button size="small" class="btn-secondary" :disabled="!selectedIds.length" @click="clearSelection"
            >{{ t('pool.clearSelection') }}</el-button
          >
          <el-button
            size="small"
            type="warning"
            class="btn-warning"
            :disabled="!selectedIds.length || !canManage"
            @click="batchSetStatus('disabled')"
            >{{ t('pool.batchDisable') }}</el-button
          >
          <el-button
            size="small"
            type="success"
            class="btn-success"
            :disabled="!selectedIds.length || !canManage"
            @click="batchSetStatus('active')"
            >{{ t('pool.batchEnable') }}</el-button
          >
          <el-button
            size="small"
            type="danger"
            class="btn-danger"
            :disabled="!selectedIds.length || !canManage"
            @click="batchDelete"
            >{{ t('pool.batchDelete') }}</el-button
          >
        </div>
      </div>
      <AppErrorCallout v-if="pageError" :error="pageError" class="mb-12" />
      <PoolTable
        :rows="pools"
        :loading="tableLoading"
        :pagination="pagination"
        :show-actions="true"
        :actions-disabled="!canManage"
        :virtual="false"
        :virtual-threshold="200"
        :table-height="920"
        selectable
        :selected-keys="selectedIds"
        @edit="openEdit"
        @remove="handleDelete"
        @select="handleSelect"
        @selection-change="handleSelectionChange"
        @page-change="handlePageChange"
        @size-change="handleSizeChange"
        @toggle-status="handleToggleStatus"
      />
    </el-card>

    <SubnetWizard
      v-model="wizardVisible"
      :value="wizardDraft"
      :existing-cidrs="existingCidrs"
      :existing-subnets="pools"
      :errors="wizardErrors"
      :submitting="wizardSubmitting"
      @submit="handleWizardSubmit"
    />

    <el-dialog v-model="importDialog" :title="t('pool.importTitle')" width="520px">
      <div class="import-body">
        <p class="tip">{{ t('pool.importTip') }}</p>
        <el-upload
          drag
          :auto-upload="false"
          :show-file-list="false"
          accept=".csv"
          @change="handleImportFile"
        >
          <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
          <div class="el-upload__text">{{ t('pool.uploadText') }}</div>
        </el-upload>

        <el-alert
          v-if="importError"
          type="error"
          show-icon
          :closable="false"
          class="mt-12"
          :title="importError"
        />

        <div v-if="importStats.total > 0" class="mt-12">
          <el-descriptions :column="2" border>
            <el-descriptions-item :label="t('pool.totalRows')">{{ importStats.total }}</el-descriptions-item>
            <el-descriptions-item :label="t('pool.successRows')">{{ importStats.success }}</el-descriptions-item>
            <el-descriptions-item :label="t('pool.failedRows')">{{ importStats.failed }}</el-descriptions-item>
          </el-descriptions>
          <el-scrollbar v-if="failedRows.length" height="160" class="mt-8">
            <div v-for="item in failedRows" :key="item.row" class="fail-list">
              {{ t('pool.failRowDetail', { row: item.row, error: item.error }) }}
            </div>
          </el-scrollbar>
        </div>
      </div>
      <template #footer>
        <el-button :disabled="importing" @click="importDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importFile" @click="submitImport"
          >{{ t('pool.startImport') }}</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Plus, Refresh, UploadFilled } from '@element-plus/icons-vue';
import { showError, showInfo, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import {
  listPools,
  getPoolStats,
  createPool,
  updatePool,
  deletePool,
  getPool,
  importPoolsCsv,
  exportPoolsCsv
} from '@/api/pools';
import type { PoolSummary, SubnetDraft } from '@/types/pool';
import PoolTable from './components/PoolTable.vue';
import SubnetWizard from './components/SubnetWizard.vue';
import AppErrorCallout from '@/components/common/AppErrorCallout.vue';
import { getApiError, createInlineError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { usePoolStore } from '@/store/pool';
import { usePoolApiGuard } from './usePoolApiGuard';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const poolStore = usePoolStore();

const filters = reactive<{
  keyword: string;
  status: '' | 'active' | 'warning' | 'disabled';
  usageRange: '' | '0-30' | '30-60' | '60-80' | '80-100';
}>({ keyword: '', status: '', usageRange: '' });
const pagination = reactive({ page: 1, pageSize: 10, total: 0 });
const pools = ref<PoolSummary[]>([]);
const tableLoading = ref(false);
const pageError = ref<ApiErrorDescriptor | null>(null);
const selectedPoolId = ref('');
const selectedIds = ref<string[]>([]);
const wizardVisible = ref(false);
const wizardDraft = ref<SubnetDraft | null>(null);
const wizardErrors = ref<ApiErrorDescriptor | null>(null);
const wizardSubmitting = ref(false);
const editingPoolId = ref<string | null>(null);
const canManage = computed(() => permissionStore.can('pool.ipv4.manage'));
const { wrapApi } = usePoolApiGuard();

type ImportRow = Partial<
  SubnetDraft & {
    scope?: string;
    parentId?: string;
    interfaceId?: string;
    ssid?: string;
    reservePercent?: string;
    leaseProfileId?: string;
    tags?: string;
    vlanId?: string;
    location?: string;
    gateway?: string;
    dns?: string;
    exclusions?: string;
    allocationMode?: string;
    priorityWeight?: string;
    rangeStart?: string;
    rangeEnd?: string;
    network?: string;
    netmask?: string;
  }
>;

const exporting = ref(false);
const importing = ref(false);
const importDialog = ref(false);
const importFile = ref<File | null>(null);
const importError = ref('');
const importStats = reactive({ total: 0, success: 0, failed: 0 });
const failedRows = ref<{ row: number; error: string }[]>([]);
const exportProgress = ref(0);
const importProgress = ref(0);

const existingCidrs = computed(() =>
  pools.value
    .filter((p) => p.id !== editingPoolId.value)
    .map((p) => p.cidr)
    .filter(Boolean)
);

const enabledCount = ref(0);
const overviewAvgUsage = ref(0);
const overviewHighUsageCount = ref(0);

let controller: AbortController | null = null;
let requestToken = 0;

const getTenantId = () => tenantStore.currentTenantId || 'global';

const inUsageRange = (usage: number, range: '' | '0-30' | '30-60' | '60-80' | '80-100') => {
  if (!range) return true;
  const value = Math.max(0, Math.min(100, Number(usage || 0)));
  if (range === '0-30') return value >= 0 && value < 30;
  if (range === '30-60') return value >= 30 && value < 60;
  if (range === '60-80') return value >= 60 && value < 80;
  return value >= 80 && value <= 100;
};

const applyUsageFilter = (rows: PoolSummary[]) =>
  rows.filter((item) => inUsageRange(Number(item.utilization || 0), filters.usageRange));

const fetchAllPoolsForExport = async () => {
  const tenantId = getTenantId();
  const pageSize = 500;
  let page = 1;
  const all: PoolSummary[] = [];
  const seen = new Set<string>();
  const maxPages = 200; // tighter safety guard to avoid runaway loops
  while (true) {
    const { data } = await wrapApi(
      () =>
        listPools(
          {
            page,
            pageSize,
            keyword: filters.keyword || undefined,
            status: filters.status || undefined,
            version: 4,
            tenantId
          },
          undefined
        ),
      t('pool.loadFail')
    );

    const items = data.data.items || [];
    const total = data.data.total || items.length;

    const before = all.length;
    items.forEach((item) => {
      if (item && item.id && !seen.has(item.id)) {
        seen.add(item.id);
        all.push(item);
      }
    });
    const added = all.length - before;

    if (
      !items.length ||
      items.length < pageSize ||
      all.length >= total ||
      page >= maxPages ||
      added === 0
    )
      break;
    page += 1;
  }

  const detailed: any[] = [];
  for (const pool of all) {
    try {
      const { data } = await getPool(pool.id, { tenantId });
      detailed.push(data.data || pool);
    } catch (err) {
      // fallback to summary if detail fetch fails
      detailed.push(pool);
    }
  }

  return detailed;
};

const fetchPools = async () => {
  if (!permissionStore.can('pool.ipv4.view')) return;
  const token = ++requestToken;
  const tenantId = getTenantId();
  controller?.abort();
  controller = new AbortController();
  tableLoading.value = true;
  pageError.value = null;
  try {
    const localUsageFilterEnabled = !!filters.usageRange;
    if (!localUsageFilterEnabled) {
      const [pageResponse, statsResponse] = await Promise.allSettled([
        wrapApi(
          () =>
            listPools(
              {
                page: pagination.page,
                pageSize: pagination.pageSize,
                keyword: filters.keyword || undefined,
                status: filters.status || undefined,
                version: 4,
                tenantId
              },
              controller?.signal
            ),
          t('pool.loadFail')
        ),
        getPoolStats(
          {
            version: 4,
            keyword: filters.keyword || undefined,
            status: filters.status || undefined,
            tenantId
          },
          controller?.signal
        )
      ]);
      if (pageResponse.status !== 'fulfilled') {
        throw pageResponse.reason;
      }
      const data = pageResponse.value.data;
      if (token !== requestToken) return;
      pools.value = (data.data.items || []).slice(0, pagination.pageSize);
      poolStore.applyList(data.data.items);
      pagination.total = data.data.total;
      if (statsResponse.status === 'fulfilled') {
        enabledCount.value = Number(statsResponse.value.data?.data?.enabledCount ?? 0);
        overviewAvgUsage.value = Number(statsResponse.value.data?.data?.avgUsage ?? 0);
        overviewHighUsageCount.value = Number(statsResponse.value.data?.data?.highUsageCount ?? 0);
      }
    } else {
      let page = 1;
      const pageSize = 500;
      const all: PoolSummary[] = [];
      const seen = new Set<string>();
      while (true) {
        const { data } = await wrapApi(
          () =>
            listPools(
              {
                page,
                pageSize,
                keyword: filters.keyword || undefined,
                status: filters.status || undefined,
                version: 4,
                tenantId
              },
              controller?.signal
            ),
          t('pool.loadFail')
        );
        const items = data.data.items || [];
        const total = data.data.total || items.length;
        items.forEach((item) => {
          if (item?.id && !seen.has(item.id)) {
            seen.add(item.id);
            all.push(item);
          }
        });
        if (!items.length || items.length < pageSize || all.length >= total || page >= 200) break;
        page += 1;
      }

      const filtered = applyUsageFilter(all);
      const offset = (pagination.page - 1) * pagination.pageSize;
      if (offset >= filtered.length && pagination.page > 1) {
        pagination.page = 1;
      }
      const start = (pagination.page - 1) * pagination.pageSize;
      pools.value = filtered.slice(start, start + pagination.pageSize);
      poolStore.applyList(filtered);
      pagination.total = filtered.length;
      enabledCount.value = filtered.filter((item) => item.status === 'active').length;
      const sumUsage = filtered.reduce((sum, item) => sum + Number(item.utilization || 0), 0);
      overviewAvgUsage.value = filtered.length ? sumUsage / filtered.length : 0;
      overviewHighUsageCount.value = filtered.filter((item) => Number(item.utilization || 0) >= 80).length;
    }
    selectedIds.value = selectedIds.value.filter((id) => pools.value.some((p) => p.id === id));
    if (!pools.value.length) {
      selectedPoolId.value = '';
      return;
    }
    const preferredId = poolStore.currentPoolId;
    if (preferredId && pools.value.some((p) => p.id === preferredId)) {
      selectedPoolId.value = preferredId;
    } else if (!selectedPoolId.value || !pools.value.some((p) => p.id === selectedPoolId.value)) {
      selectedPoolId.value = pools.value[0].id;
      poolStore.select(selectedPoolId.value);
    }
  } catch (error: any) {
    if (error?.code === 'ERR_CANCELED') return;
    if (error?.name === 'CanceledError' || error?.name === 'AbortError') return;
    pageError.value = getApiError(error) || createInlineError(t('pool.loadFail'));
  } finally {
    if (token === requestToken) {
      tableLoading.value = false;
    }
    controller = null;
  }
};

const downloadTemplate = () => {
  const header =
    'name,cidr,scope,parentId,vlanId,interfaceId,ssid,location,reservePercent,leaseProfileId,tags,gateway,option43,dns,rangeStart,rangeEnd,exclusions,allocationMode,priorityWeight';
  const comment = [
    t('pool.csvColName'),
    t('pool.csvColCidr'),
    t('pool.csvColScope'),
    t('pool.csvColParentId'),
    t('pool.csvColVlanId'),
    t('pool.csvColInterfaceId'),
    t('pool.csvColSsid'),
    t('pool.csvColLocation'),
    t('pool.csvColReservePercent'),
    t('pool.csvColLeaseProfileId'),
    t('pool.csvColTags'),
    t('pool.csvColGateway'),
    t('pool.csvColOption43'),
    t('pool.csvColDns'),
    t('pool.csvColRangeStart'),
    t('pool.csvColRangeEnd'),
    t('pool.csvColExclusions'),
    t('pool.csvColAllocationMode'),
    t('pool.csvColPriorityWeight')
  ].join(',');
  const example = [
    t('pool.csvExampleName'),
    '192.168.10.0/24',
    'GLOBAL',
    '',
    '10',
    '',
    t('pool.csvExampleLocation'),
    '',
    '10',
    'default-lease-profile',
    'role=core;env=prod',
    '192.168.10.1',
    'serverip=1.1.1.1',
    '8.8.8.8;8.8.4.4',
    '192.168.10.10',
    '192.168.10.200',
    '192.168.10.50-192.168.10.60',
    'ROUND_ROBIN',
    '1'
  ].join(',');
  const bom = '\ufeff';
  const blob = new Blob([`${bom}${header}\n${comment}\n${example}\n`], {
    type: 'text/csv;charset=utf-8;'
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'ipv4-pool-template.csv';
  a.click();
  URL.revokeObjectURL(url);
};

const exportCsv = async () => {
  try {
    exporting.value = true;
    exportProgress.value = 10;
    const response = await wrapApi(() => exportPoolsCsv({ version: 4 }), t('pool.loadFail'));
    exportProgress.value = 70;
    const blob =
      response.data instanceof Blob
        ? response.data
        : new Blob([response.data as any], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'ipv4-pools-export.csv';
    a.click();
    exportProgress.value = 100;
    URL.revokeObjectURL(url);
    showSuccess(t('pool.exportSuccess'));
  } catch (error: any) {
    if (error?.code === 'ERR_CANCELED' || error?.name === 'AbortError') return;
    // wrapApi already shows error message
  } finally {
    exporting.value = false;
    window.setTimeout(() => {
      exportProgress.value = 0;
    }, 300);
  }
};

const openImport = () => {
  importDialog.value = true;
  importFile.value = null;
  importError.value = '';
  failedRows.value = [];
  importStats.total = 0;
  importStats.success = 0;
  importStats.failed = 0;
};

const splitCsv = (content: string): string[][] => {
  const rows: string[][] = [];
  let current = '';
  let row: string[] = [];
  let inQuotes = false;
  for (let i = 0; i < content.length; i += 1) {
    const ch = content[i];
    if (ch === '"') {
      const next = content[i + 1];
      if (inQuotes && next === '"') {
        current += '"';
        i += 1;
        continue;
      }
      inQuotes = !inQuotes;
      continue;
    }
    if (ch === ',' && !inQuotes) {
      row.push(current);
      current = '';
      continue;
    }
    if ((ch === '\n' || ch === '\r') && !inQuotes) {
      if (ch === '\r' && content[i + 1] === '\n') {
        i += 1;
      }
      row.push(current);
      rows.push(row);
      row = [];
      current = '';
      continue;
    }
    current += ch;
  }
  row.push(current);
  rows.push(row);
  return rows;
};

const parseCsv = (content: string): ImportRow[] => {
  const rawRows = splitCsv(content)
    .map((r) => r.map((cell) => cell.trim()))
    .filter((r) => r.some((cell) => cell !== ''))
    .filter((r) => !(r[0] || '').startsWith('#'));
  if (rawRows.length < 2) throw new Error(t('pool.csvInsufficient'));
  const header = rawRows[0].map((h) =>
    h
      .replace(/^\ufeff/, '')
      .split('(')[0]
      .trim()
      .toLowerCase()
  );
  const fieldMap: Record<string, keyof ImportRow> = {
    name: 'name',
    cidr: 'cidr',
    scope: 'scope',
    parentid: 'parentId',
    vlanid: 'vlanId',
    interfaceid: 'interfaceId',
    ssid: 'ssid',
    location: 'location',
    reservepercent: 'reservePercent',
    leaseprofileid: 'leaseProfileId',
    tags: 'tags',
    gateway: 'gateway',
    dns: 'dns',
    rangestart: 'rangeStart',
    rangeend: 'rangeEnd',
    exclusions: 'exclusions',
    allocationmode: 'allocationMode',
    priorityweight: 'priorityWeight',
    network: 'network',
    netmask: 'netmask'
  };
  ['name', 'cidr'].forEach((key) => {
    if (!header.includes(key)) throw new Error(t('pool.csvMissingField', { field: key }));
  });
  const headerKeys = header.map((key) => fieldMap[key] || null);
  const rows: ImportRow[] = [];
  for (let i = 1; i < rawRows.length; i += 1) {
    const cols = rawRows[i];
    const record: ImportRow = {};
    headerKeys.forEach((mapped, idx) => {
      if (!mapped) return;
      (record as Record<string, string>)[mapped as string] = (cols[idx] || '').trim();
    });
    if (!record.name && !record.cidr) continue;
    rows.push(record);
  }
  return rows;
};

const handleImportFile = (file: any) => {
  importError.value = '';
  failedRows.value = [];
  const picked = file?.raw || file;
  importFile.value = picked instanceof File ? picked : null;
  const reader = new FileReader();
  reader.onload = () => {
    try {
      const text = String(reader.result || '');
      const parsed = parseCsv(text);
      importStats.total = parsed.length;
      importStats.success = 0;
      importStats.failed = 0;
    } catch (error: any) {
      importError.value = error?.message || t('pool.parseFail');
      importFile.value = null;
      importStats.total = 0;
    }
  };
  reader.readAsText(picked);
};

const normalizeImportResult = (payload: any) => {
  const data = payload?.data ?? payload;
  const inner = data?.data ?? data;
  return {
    total: Number(inner?.total ?? 0),
    success: Number(inner?.success ?? 0),
    failed: Number(inner?.failed ?? 0),
    errors: Array.isArray(inner?.errors) ? inner.errors : []
  } as {
    total: number;
    success: number;
    failed: number;
    errors: { row?: number; message?: string; error?: string }[];
  };
};

const submitImport = async () => {
  if (!importFile.value) return;
  importing.value = true;
  importProgress.value = 10;
  const timer = window.setInterval(() => {
    if (importProgress.value < 90) {
      importProgress.value += 8;
    }
  }, 250);
  importError.value = '';
  failedRows.value = [];
  importStats.success = 0;
  importStats.failed = 0;
  try {
    const response = await wrapApi(
      () => importPoolsCsv(importFile.value as File, { version: 4 }),
      t('pool.saveFail')
    );
    const result = normalizeImportResult(response?.data);
    importStats.total = result.total;
    importStats.success = result.success;
    importStats.failed = result.failed;
    failedRows.value = (result.errors || []).map((item) => {
      const rowNumber = Number((item as any).row ?? (item as any).Row ?? 0);
      return { row: rowNumber, error: item.message || (item as any).error || t('pool.importFail') };
    });
    if (!importStats.failed) {
      showSuccess(t('pool.importDone'));
    }
    importProgress.value = 100;
    await fetchPools();
  } catch (error: any) {
    importError.value = getApiError(error)?.message || error?.message || t('pool.importFail');
  } finally {
    window.clearInterval(timer);
    importing.value = false;
    window.setTimeout(() => {
      importProgress.value = 0;
    }, 300);
  }
};

const handleSearch = () => {
  pagination.page = 1;
  selectedIds.value = [];
  fetchPools();
};

const handleReset = () => {
  filters.keyword = '';
  filters.status = '';
  filters.usageRange = '';
  handleSearch();
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  fetchPools();
};

const handleSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  fetchPools();
};

const handleSelectionChange = (selection: PoolSummary[]) => {
  selectedIds.value = selection.map((item) => item.id);
};

const selectAllCurrent = () => {
  selectedIds.value = pools.value.map((p) => p.id);
};

const invertSelection = () => {
  const set = new Set(selectedIds.value);
  selectedIds.value = pools.value.filter((p) => !set.has(p.id)).map((p) => p.id);
};

const clearSelection = () => {
  selectedIds.value = [];
};

const selectedRows = computed(() => pools.value.filter((row) => selectedIds.value.includes(row.id)));

const batchSetStatus = async (status: 'active' | 'disabled') => {
  if (!selectedRows.value.length || !canManage.value) return;
  const actionText = status === 'active' ? t('pool.statusActive') : t('pool.statusDisabled');
  try {
    await ElMessageBox.confirm(t('pool.batchConfirm', { action: actionText, count: selectedRows.value.length }), t('pool.batchConfirmTitle'), {
      type: 'warning'
    });
    const tenantId = getTenantId();
    for (const row of selectedRows.value) {
      await wrapApi(() => updatePool(row.id, { status, tenantId }), t('pool.saveFail'));
    }
    showSuccess(t('pool.batchDone', { action: actionText }));
    await fetchPools();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('pool.batchFailed', { action: actionText }));
    }
  }
};

const batchDelete = async () => {
  if (!selectedRows.value.length || !canManage.value) return;
  try {
    await ElMessageBox.confirm(
      t('pool.batchDeleteConfirm', { count: selectedRows.value.length }),
      t('pool.batchDeleteTitle'),
      {
        type: 'warning'
      }
    );
    const tenantId = getTenantId();
    for (const row of selectedRows.value) {
      await wrapApi(() => deletePool(row.id, { tenantId }), t('pool.deleteFail'));
    }
    showSuccess(t('pool.batchDeleteDone'));
    selectedIds.value = [];
    await fetchPools();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('pool.batchDeleteFailed'));
    }
  }
};

const upsertLocalPool = (pool: PoolSummary, select = false) => {
  const idx = pools.value.findIndex((p) => p.id === pool.id);
  if (idx >= 0) {
    pools.value.splice(idx, 1, pool);
  } else {
    pools.value.unshift(pool);
    if (pools.value.length > pagination.pageSize) {
      pools.value = pools.value.slice(0, pagination.pageSize);
    }
  }
  pools.value = [...pools.value];
  poolStore.upsert(pool);
  if (select) {
    selectedPoolId.value = pool.id;
  }
};

const ensurePoolVisible = (pool: PoolSummary | null) => {
  if (!pool) return;
  upsertLocalPool(pool, true);
};

const openCreate = () => {
  if (!canManage.value) return;
  wizardDraft.value = null;
  editingPoolId.value = null;
  wizardVisible.value = true;
  wizardErrors.value = null;
};

const openEdit = async (row: PoolSummary) => {
  if (!canManage.value) return;
  wizardDraft.value = {
    name: row.name,
    cidr: row.cidr,
    leaseTime: 3600,
    maxLeaseTime: 7200,
    exclude: [],
    strategy: { mode: 'round-robin' },
    vlanId: row.vlanId,
    tags: [],
    gateway: undefined,
    dns: [],
    location: undefined
  };
  editingPoolId.value = row.id;
  wizardVisible.value = true;
  wizardErrors.value = null;
  try {
    const { data } = await wrapApi(
      () => getPool(row.id, { tenantId: getTenantId() }),
      t('pool.loadFail')
    );
    wizardDraft.value = data.data;
  } catch (error) {
    console.error(error);
  }
};

const handleWizardSubmit = async (draft: SubnetDraft) => {
  if (!canManage.value || wizardSubmitting.value) return;
  wizardSubmitting.value = true;
  wizardErrors.value = null;
  let changedPool: PoolSummary | null = null;
  try {
    const tenantId = getTenantId();
    const currentPoolId = editingPoolId.value;
    const payload: SubnetDraft & { tenantId?: string } = {
      ...draft,
      dns: draft.dns || [],
      tenantId
    };
    console.warn('[pool] submit payload', JSON.parse(JSON.stringify(payload)));
    if (currentPoolId) {
      const { data } = await wrapApi(() => updatePool(currentPoolId, payload), t('pool.saveFail'));
      changedPool = data.data;
      upsertLocalPool(changedPool, selectedPoolId.value === currentPoolId);
      showSuccess(t('pool.updated'));
    } else {
      const { data } = await wrapApi(
        () => createPool({ ...payload, version: 4 }),
        t('pool.saveFail')
      );
      changedPool = data.data;
      upsertLocalPool(changedPool, true);
      showSuccess(t('pool.created'));
    }
    wizardVisible.value = false;
    pagination.page = 1; // force refresh to include newest data
    await fetchPools();
    ensurePoolVisible(changedPool);
  } catch (error) {
    wizardErrors.value = getApiError(error);
    console.error(error);
  } finally {
    wizardSubmitting.value = false;
  }
};

const handleDelete = async (row: PoolSummary) => {
  if (!canManage.value) return;
  try {
    const tenantId = getTenantId();
    await wrapApi(() => deletePool(row.id, { tenantId }), t('pool.deleteFail'));
    showSuccess(t('pool.deleted'));
    if (selectedPoolId.value === row.id) selectedPoolId.value = '';
    selectedIds.value = selectedIds.value.filter((id) => id !== row.id);
    if (pagination.page > 1 && pools.value.length <= 1) {
      pagination.page -= 1;
    }
    await fetchPools();
  } catch (error) {
    console.error(error);
    // wrapApi already emitted message
  }
};

const handleToggleStatus = async (row: PoolSummary) => {
  if (!canManage.value) return;
  const tenantId = getTenantId();
  const nextStatus: PoolSummary['status'] = row.status === 'active' ? 'disabled' : 'active';
  try {
    const { data } = await wrapApi(
      () => updatePool(row.id, { status: nextStatus, tenantId }),
      t('pool.saveFail')
    );
    const updated = data?.data || { ...row, status: nextStatus };
    const finalStatus: PoolSummary['status'] = updated.status || nextStatus;
    // Optimistically reflect the status change so the table updates immediately.
    upsertLocalPool({ ...updated, status: finalStatus }, selectedPoolId.value === row.id);
    // If a status filter is applied, remove rows that no longer match.
    if (filters.status && finalStatus !== filters.status) {
      pools.value = pools.value.filter((p) => p.id !== updated.id);
      selectedIds.value = selectedIds.value.filter((id) => id !== updated.id);
      if (selectedPoolId.value === updated.id) {
        selectedPoolId.value = pools.value[0]?.id || '';
        if (selectedPoolId.value) {
          poolStore.select(selectedPoolId.value);
        }
      }
    }
    showSuccess(finalStatus === 'active' ? t('pool.enabled') : t('pool.statusDisabledDone'));
    await fetchPools();
    if (!filters.status || finalStatus === filters.status) {
      upsertLocalPool({ ...updated, status: finalStatus }, selectedPoolId.value === row.id);
    }
  } catch (error) {
    console.error(error);
  }
};

const handleSelect = (row: PoolSummary) => {
  selectedPoolId.value = row.id;
  poolStore.select(row.id);
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    pagination.page = 1;
    selectedIds.value = [];
    fetchPools();
  }
);

onMounted(() => {
  fetchPools();
});

onBeforeUnmount(() => {
  controller?.abort();
  controller = null;
});
</script>

<style scoped>
.pool-page {
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-muted: #4b5563;
  --subtle-bg: #f9fafb;
  --chip-bg: #f3f4f6;
  --danger-text: #b91c1c;
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 2400px;
  margin: 0 auto;
  min-height: calc(100vh - 24px);
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

.page-header {
  margin-bottom: 12px;
}

.page-header h3 {
  margin: 0;
}

.filter-bar {
  min-height: 56px;
  margin-bottom: 12px;
  padding: 12px 16px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  background: #f5f7fa;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bar-left {
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}

.bar-right {
  justify-content: flex-end;
  white-space: nowrap;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.filters {
  margin: 0;
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 16px;
  white-space: nowrap;
}

.filters :deep(.el-form-item) {
  margin-bottom: 0;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper),
.filter-bar :deep(.el-button) {
  min-height: 32px;
  height: 32px;
  border-radius: 8px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.overview-card {
  border-radius: 10px;
}

.overview-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.overview-value {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
}

.progress-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.progress-item {
  display: grid;
  grid-template-columns: 80px 1fr;
  align-items: center;
  gap: 10px;
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

.mb-12 {
  margin-bottom: 12px;
}

.mt-12 {
  margin-top: 12px;
}

.mt-8 {
  margin-top: 8px;
}

.import-body .tip {
  color: var(--text-muted);
  margin-bottom: 12px;
}

.fail-list {
  padding: 4px 0;
  color: var(--danger-text);
  font-size: 13px;
}

@media (max-width: 1200px) {
  .filter-bar {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .bar-right {
    justify-content: flex-start;
  }

  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
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
