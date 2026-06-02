<template>
  <div class="binding-mac">
    <section class="stat-grid">
      <el-card
        v-for="card in statCards"
        :key="card.key"
        class="stat-card"
        :class="{ active: activeStatFilter === card.key }"
        @click="handleStatCardClick(card.key)"
      >
        <div class="stat-title">{{ card.label }}</div>
        <div class="stat-value" :style="{ color: card.color }">{{ card.value }}</div>
        <div class="stat-desc">{{ card.desc }}</div>
      </el-card>
    </section>

    <section class="page-header">
      <h1 class="page-title">{{ t('binding.macTitle') }}</h1>
      <p class="page-subtitle">{{ t('binding.macSubtitle') }}</p>
    </section>

    <section class="toolbar surface-card">
      <div class="toolbar-left">
        <el-form :inline="true" :model="filters" class="filter-row">
          <el-form-item :label="t('binding.macKeyword')">
            <el-input
              v-model="filters.keyword"
              clearable
              :placeholder="t('binding.macKeywordPlaceholder')"
              style="width: 260px"
              @keyup.enter="handleSearch"
            />
          </el-form-item>
          <el-form-item :label="t('binding.status')">
            <el-select v-model="filters.status" multiple clearable style="width: 220px">
              <el-option v-for="opt in statusOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="handleSearch">{{ t('common.search') }}</el-button>
            <el-button @click="handleReset">{{ t('common.reset') }}</el-button>
          </el-form-item>
        </el-form>
      </div>

      <div class="toolbar-middle">
        <el-button type="success" plain :disabled="!selectionIds.length || !canManage" @click="handleBulkEnable">{{ t('binding.macBulkEnable') }}</el-button>
        <el-button type="warning" plain :disabled="!selectionIds.length || !canManage" @click="handleBulkDisable">{{ t('binding.macBulkDisable') }}</el-button>
        <el-button type="danger" plain :disabled="!selectionIds.length || !canManage" @click="handleBulkDelete">{{ t('binding.macBulkDelete') }}</el-button>
      </div>

      <div class="toolbar-right">
        <div class="auto-refresh">
          <span>{{ t('binding.macAutoRefresh') }}</span>
          <el-switch v-model="autoRefreshEnabled" />
          <el-select v-model="autoRefreshSeconds" size="small" style="width: 96px" :disabled="!autoRefreshEnabled">
            <el-option :value="15" label="15s" />
            <el-option :value="30" label="30s" />
            <el-option :value="60" label="60s" />
          </el-select>
        </div>
        <div class="action-buttons">
          <el-button class="list-refresh" type="primary" :loading="loading" @click="refresh">
            {{ t('common.refresh') }}
          </el-button>
          <el-button type="primary" plain @click="downloadTemplate">{{ t('binding.macDownloadTemplate') }}</el-button>
          <el-button type="success" plain :disabled="!rows.length" @click="exportCsv">
            {{ t('common.export') }}
          </el-button>
          <el-button type="primary" plain :disabled="!canManage" @click="openImport">
            {{ t('common.import') }}
          </el-button>
          <el-button type="primary" :disabled="!canManage" @click="openCreate">{{
            t('binding.create')
          }}</el-button>
        </div>
      </div>
    </section>

    <section class="table-panel surface-card">
      <el-table
        v-loading="loading"
        :data="displayRows"
        border
        stripe
        row-key="id"
        @selection-change="handleSelectionChange"
      >
        <template #empty>
          <el-empty :description="loading ? t('monitoring.loading') : t('binding.empty')" />
        </template>
        <el-table-column type="selection" width="50" />
        <el-table-column prop="mac" :label="t('binding.formMac')" min-width="170" />
        <el-table-column prop="ip" :label="t('binding.formIp')" min-width="150" />
        <el-table-column prop="poolId" :label="t('pool.labelName')" min-width="180">
          <template #default="{ row }">
            {{ poolNameById[row.poolId] || row.poolId || '--' }}
          </template>
        </el-table-column>
        <el-table-column prop="hostname" :label="t('binding.formHostname')" min-width="160" />
        <el-table-column :label="t('binding.status')" min-width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(displayStatus(row))" disable-transitions effect="light">
              {{ statusLabel(displayStatus(row)) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('binding.macLastSeen')" min-width="180">
          <template #default="{ row }">
            <span>{{ row.lastSeenAt ? formatTs(row.lastSeenAt) : '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" :label="t('binding.createdAt')" min-width="180">
          <template #default="{ row }">{{ formatTs(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column fixed="right" :label="t('common.actions')" width="280">
          <template #default="{ row }">
            <el-button size="small" @click="openDetails(row)">{{ t('binding.macDetail') }}</el-button>
            <el-button type="primary" plain size="small" @click="openEdit(row)">{{
              t('common.edit')
            }}</el-button>
            <el-button
              size="small"
              :type="isBindingEnabled(row) ? 'warning' : 'success'"
              plain
              :disabled="!canManage"
              @click="toggleBindingEnabled(row, !isBindingEnabled(row))"
            >
              {{ isBindingEnabled(row) ? t('binding.macDisable') : t('binding.macEnable') }}
            </el-button>
            <el-button
              type="danger"
              plain
              size="small"
              :disabled="!canManage"
              @click="handleDelete(row)"
            >
              {{ t('common.delete') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
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
    </section>

    <el-drawer v-model="detailVisible" :title="t('binding.macDrawerTitle')" size="460px">
      <template v-if="detailRow">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="MAC">{{ detailRow.mac || '-' }}</el-descriptions-item>
          <el-descriptions-item label="IP">{{ detailRow.ip || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerHostname')">{{ detailRow.hostname || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerPool')">{{ poolNameById[detailRow.poolId || ''] || detailRow.poolId || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerStatus')">{{ statusLabel(displayStatus(detailRow)) }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerCreatedAt')">{{ formatTs(detailRow.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerLastSeen')">{{ detailRow.lastSeenAt ? formatTs(detailRow.lastSeenAt) : '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('binding.macDrawerDesc')">{{ detailRow.description || '-' }}</el-descriptions-item>
        </el-descriptions>
        <div class="audit-box">
          <div class="audit-title">{{ t('binding.macAuditTitle') }}</div>
          <el-empty v-if="!recentAuditLogs.length" :description="t('binding.macAuditEmpty')" :image-size="44" />
          <div v-else class="audit-list">
            <div v-for="item in recentAuditLogs" :key="item.id" class="audit-item">
              <span class="audit-time">{{ formatTs(item.time) }}</span>
              <span class="audit-action">{{ item.action }}</span>
            </div>
          </div>
        </div>
      </template>
    </el-drawer>

    <BindingForm
      v-model="drawerVisible"
      :value="editing"
      :existing-bindings="rows"
      :submitting="saveSubmitting"
      :conflict-checker="checkConflict"
      @submit="handleSave"
    />

    <el-dialog v-model="importDialog" :title="t('binding.macImportTitle')" width="520px">
      <div class="import-body">
        <el-alert
          type="info"
          :closable="false"
          class="mb-12"
          :title="t('binding.macImportPoolHint')"
        />
        <el-upload
          drag
          :auto-upload="false"
          :show-file-list="false"
          accept=".csv"
          @change="handleImportFile"
        >
          <el-icon class="el-icon--upload"><upload-filled /></el-icon>
          <div class="el-upload__text">{{ t('binding.macImportDrag') }}</div>
        </el-upload>

        <el-alert
          v-if="importError"
          type="error"
          show-icon
          :closable="false"
          class="mt-12"
          :title="importError"
        />

        <div v-if="importFailures.length" class="import-failures">
          <div class="import-failures-head">
            <span>{{ t('binding.macImportErrorList', { count: importFailures.length }) }}</span>
            <el-button size="small" @click="downloadImportFailures">{{ t('binding.macImportDownloadErrors') }}</el-button>
          </div>
          <ul>
            <li v-for="(item, index) in importFailures.slice(0, 8)" :key="`${index}-${item.message}`">
              {{ t('binding.macImportRowError', { row: item.row, message: item.message }) }}
            </li>
          </ul>
        </div>
      </div>
      <template #footer>
        <el-button :disabled="importing" @click="importDialog = false">{{ t('binding.macImportCancel') }}</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importFile" @click="submitImport"
          >{{ t('binding.macImportStart') }}</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import { useI18n } from 'vue-i18n';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { useBindingStore } from '@/store/binding';
import BindingForm from './components/BindingForm.vue';
import { listPools } from '@/api/pools';
import { createBinding, updateBinding, deleteBinding, getConflicts } from '@/api/bindings';
import type { Binding, BindingStatus } from '@/types/binding';
import { formatTs } from '@/utils/time';
import { UploadFilled } from '@element-plus/icons-vue';

type DisplayStatus = BindingStatus | 'disabled';

type ImportFailure = {
  row: number;
  message: string;
  raw: string[];
};

type AuditLog = {
  id: string;
  bindingId: string;
  action: string;
  time: number;
};

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();
const bindingStore = useBindingStore();

const loading = computed(() => bindingStore.loading);
const rows = computed(() => bindingStore.rows);
const pagination = reactive({ ...bindingStore.pagination });
const filters = reactive<{
  keyword: string;
  status: DisplayStatus[];
}>({
  keyword: '',
  status: []
});
const drawerVisible = ref(false);
const editing = ref<Binding | null>(null);
const saveSubmitting = ref(false);
const selectionIds = ref<string[]>([]);
const activeStatFilter = ref<string>('');
const detailVisible = ref(false);
const detailRow = ref<Binding | null>(null);
const auditLogs = ref<AuditLog[]>([]);
const importDialog = ref(false);
const importFile = ref<File | null>(null);
const importError = ref('');
const importFailures = ref<ImportFailure[]>([]);
const importing = ref(false);
const autoRefreshEnabled = ref(false);
const autoRefreshSeconds = ref(30);
const poolOptions = ref<Array<{ id: string; name: string; cidr: string }>>([]);
const poolsLoading = ref(false);
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;
const poolNameById = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  poolOptions.value.forEach((pool) => {
    const label = pool.name ? `${pool.name} (${pool.cidr})` : pool.cidr;
    map[pool.id] = label;
  });
  return map;
});
const poolById = computed<Record<string, { id: string; name: string; cidr: string }>>(() => {
  const map: Record<string, { id: string; name: string; cidr: string }> = {};
  poolOptions.value.forEach((pool) => {
    map[pool.id] = pool;
  });
  return map;
});

const canManage = computed(() => permissionStore.can('binding.manage'));
const selectedRows = computed(() => rows.value.filter((row) => selectionIds.value.includes(row.id)));

const displayStatus = (row: Binding): DisplayStatus => {
  const enabled = (row.metadata as Record<string, unknown> | undefined)?.enabled;
  if (enabled === false) return 'disabled';
  return (row.status as BindingStatus) || 'offline';
};

const isBindingEnabled = (row: Binding) => displayStatus(row) !== 'disabled';

const displayRows = computed(() => {
  const keyword = filters.keyword.trim().toLowerCase();
  return rows.value.filter((row) => {
    const status = displayStatus(row);
    if (filters.status.length && !filters.status.includes(status)) {
      return false;
    }
    if (!keyword) return true;
    const poolLabel = row.poolId ? poolNameById.value[row.poolId] || row.poolId : '';
    const bucket = [row.mac, row.ip, row.hostname, row.description, poolLabel]
      .filter(Boolean)
      .map((item) => String(item).toLowerCase());
    return bucket.some((item) => item.includes(keyword));
  });
});

const recentAuditLogs = computed(() => {
  if (!detailRow.value?.id) return [];
  return auditLogs.value
    .filter((log) => log.bindingId === detailRow.value?.id)
    .sort((a, b) => b.time - a.time)
    .slice(0, 8);
});

const statusOptions = [
  { value: 'online', label: t('binding.statusOnline') },
  { value: 'warning', label: t('binding.statusWarning') },
  { value: 'offline', label: t('binding.statusOffline') },
  { value: 'disabled', label: t('binding.macStatusDisabled') }
];

const statCards = computed(() => {
  const stats = bindingStore.stats;
  return [
    {
      key: 'total',
      label: t('binding.statsTotal'),
      value: stats.total,
      color: '#2563eb',
      desc: t('binding.statsTotalDesc')
    },
    {
      key: 'online',
      label: t('binding.statusOnline'),
      value: stats.online,
      color: '#16a34a',
      desc: t('binding.statsOnlineDesc')
    },
    {
      key: 'warning',
      label: t('binding.statusWarning'),
      value: stats.warning,
      color: '#f97316',
      desc: t('binding.statsWarningDesc')
    },
    {
      key: 'offline',
      label: t('binding.statusOffline'),
      value: stats.offline,
      color: '#94a3b8',
      desc: t('binding.statsOfflineDesc')
    },
    {
      key: 'disabled',
      label: t('binding.macStatusDisabled'),
      value: rows.value.filter((row) => displayStatus(row) === 'disabled').length,
      color: '#64748b',
      desc: t('binding.macStatsDisabledDesc')
    }
  ];
});

watch(
  () => bindingStore.pagination,
  (val) => Object.assign(pagination, val),
  { deep: true }
);

watch(
  () => tenantStore.currentTenantId,
  (tenantId) => {
    bindingStore.setFilters({ tenantId: tenantId || undefined, page: 1 });
    pagination.page = 1;
    loadData({ page: 1, tenantId: tenantId || undefined });
    loadPools();
  }
);

const loadData = async (overrides?: Record<string, unknown>) => {
  const keyword = filters.keyword.trim();
  const serverStatuses = filters.status.filter((item) => item !== 'disabled') as BindingStatus[];
  const macPattern = /^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i;
  const ipPattern = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;
  const serverFilter: Record<string, unknown> = {};
  if (keyword) {
    if (macPattern.test(keyword)) {
      serverFilter.mac = keyword;
    } else if (ipPattern.test(keyword)) {
      serverFilter.ip = keyword;
    } else {
      serverFilter.hostname = keyword;
    }
  }
  if (serverStatuses.length) {
    serverFilter.status = serverStatuses;
  }

  await bindingStore.fetchList({
    tenantId: tenantStore.currentTenantId || 'global',
    ...serverFilter,
    ...overrides
  });
};

const refresh = () => loadData();

const handleSearch = () => {
  pagination.page = 1;
  loadData({ page: 1 });
};

const handleReset = () => {
  filters.keyword = '';
  filters.status = [];
  activeStatFilter.value = '';
  bindingStore.resetFilters();
  pagination.page = 1;
  loadData({ page: 1 });
};

const handleStatCardClick = (key: string) => {
  if (activeStatFilter.value === key) {
    activeStatFilter.value = '';
    filters.status = [];
    handleSearch();
    return;
  }
  activeStatFilter.value = key;
  filters.status = key === 'total' ? [] : [key as DisplayStatus];
  handleSearch();
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  loadData({ page });
};

const handleSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  loadData({ pageSize: size, page: 1 });
};

const handleSelectionChange = (rows: Binding[]) => {
  selectionIds.value = rows.map((row) => row.id);
};

const downloadTemplate = () => {
  const header = 'mac,ip,hostname,description,poolId,status';
  const example = ['00:11:22:33:44:55', '10.0.0.10', 'host.example', t('binding.macCsvExample'), 'pool-id', ''];
  const bom = '\ufeff';
  const blob = new Blob([`${bom}${header}\n${example.join(',')}\n`], {
    type: 'text/csv;charset=utf-8;'
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'mac-bindings-template.csv';
  a.click();
  URL.revokeObjectURL(url);
};

const exportCsv = () => {
  if (!rows.value.length) return;
  const header = ['mac', 'ip', 'hostname', 'description', 'poolId', 'status'];
  const csvRows = rows.value.map((r) => [
    r.mac || '',
    r.ip || '',
    r.hostname || '',
    r.description || '',
    r.poolId || '',
    r.status || ''
  ]);
  const csv = [header, ...csvRows]
    .map((r) =>
      r
        .map((cell) => {
          const str = cell === undefined || cell === null ? '' : String(cell);
          return /[",\n]/.test(str) ? `"${str.replace(/"/g, '""')}"` : str;
        })
        .join(',')
    )
    .join('\n');
  const bom = '\ufeff';
  const blob = new Blob([bom + csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'mac-bindings.csv';
  a.click();
  URL.revokeObjectURL(url);
};

const loadPools = async () => {
  poolsLoading.value = true;
  try {
    const { data } = await listPools({
      page: 1,
      pageSize: 200,
      version: 4,
      tenantId: tenantStore.currentTenantId || 'global'
    });
    const payload: any = (data as any)?.data ?? data;
    const items = Array.isArray(payload?.items)
      ? payload.items
      : Array.isArray(payload)
        ? payload
        : [];
    poolOptions.value = items.map((p: any) => ({ id: p.id, name: p.name, cidr: p.cidr }));
  } catch {
    poolOptions.value = [];
  } finally {
    poolsLoading.value = false;
  }
};

const openImport = () => {
  importDialog.value = true;
  importFile.value = null;
  importError.value = '';
  importFailures.value = [];
  loadPools();
};

const splitCsv = (content: string): string[][] => {
  const rows: string[][] = [];
  let current = '';
  let inQuotes = false;
  let row: string[] = [];
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
      if (ch === '\r' && content[i + 1] === '\n') i += 1;
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

const handleImportFile = (file: any) => {
  importError.value = '';
  const picked = file?.raw || file;
  importFile.value = picked instanceof File ? picked : null;
};

const parseIPv4ToInt = (ip: string): number | null => {
  const parts = ip.split('.').map((p) => Number(p));
  if (parts.length !== 4 || parts.some((p) => Number.isNaN(p) || p < 0 || p > 255)) return null;
  return ((parts[0] << 24) >>> 0) + (parts[1] << 16) + (parts[2] << 8) + parts[3];
};

const ipInCidr = (ip: string, cidr: string): boolean => {
  if (!ip || !cidr) return true;
  const [net, prefix] = cidr.split('/');
  const netInt = parseIPv4ToInt(net);
  const ipInt = parseIPv4ToInt(ip);
  const prefixNum = Number(prefix);
  if (netInt === null || ipInt === null || Number.isNaN(prefixNum) || prefixNum < 0 || prefixNum > 32)
    return false;
  const mask = prefixNum === 0 ? 0 : (~0 << (32 - prefixNum)) >>> 0;
  return (ipInt & mask) === (netInt & mask);
};

const submitImport = async () => {
  if (!importFile.value) return;
  importing.value = true;
  importError.value = '';
  importFailures.value = [];
  try {
    const text = await importFile.value.text();
    const rowsCsv = splitCsv(text)
      .map((r) => r.map((cell) => cell.trim()))
      .filter((r) => r.some((cell) => cell !== ''));
    const header = (rowsCsv.shift() || []).map((h) => h.replace(/^\ufeff/, '').toLowerCase());
    const idx = (key: string) => header.indexOf(key);
    const macIdx = idx('mac');
    if (macIdx < 0) throw new Error(t('binding.macCsvMissingMac'));
    const tenantId = tenantStore.currentTenantId || 'global';
    const seenInFile = new Set<string>();
    let successCount = 0;
    let skippedDuplicate = 0;
    const errors: string[] = [];
    const failures: ImportFailure[] = [];
    for (const [index, row] of rowsCsv.entries()) {
      const mac = row[macIdx] || '';
      if (!mac) continue;
      const macKey = mac.toLowerCase();
      if (seenInFile.has(macKey)) {
        failures.push({ row: index + 2, message: t('binding.macCsvDuplicateMac', { mac }), raw: row });
        continue;
      }
      seenInFile.add(macKey);
      const poolId =
        row[idx('poolid')] || row[idx('poolId')] || row[idx('pool_id')] || row[idx('pool')] || '';
      const ip = row[idx('ip')] || '';
      const pool = poolById.value[poolId];
      if (pool && ip && pool.cidr && !ipInCidr(ip, pool.cidr)) {
        failures.push({ row: index + 2, message: t('binding.macCsvIpOutOfRange', { ip, cidr: pool.cidr }), raw: row });
        continue;
      }
      const payload: Partial<Binding> = {
        mac,
        ip: ip || undefined,
        hostname: row[idx('hostname')] || undefined,
        description: row[idx('description')] || undefined,
        status: (row[idx('status')] as BindingStatus) || undefined,
        poolId: poolId || undefined,
        metadata: {
          hostname: row[idx('hostname')] || undefined,
          description: row[idx('description')] || undefined
        }
      };
      try {
        await createBinding({
          ...payload,
          tenantId,
          identifier: payload.mac,
          identifierType: 'mac',
          ipAddress: payload.ip,
          leaseProfileId: 'default-lease-profile'
        });
        successCount += 1;
      } catch (err: any) {
        const resp = err?.response?.data;
        const fieldMsgs = resp?.details?.fields;
        const message = resp?.message || err?.message || t('binding.macImportFail');
        const isDuplicate =
          (Array.isArray(fieldMsgs) && fieldMsgs.some((f: any) => f.field === 'identifier')) ||
          /duplicate|already.?exists/i.test(message || '');
        if (isDuplicate) {
          skippedDuplicate += 1;
          continue;
        }
        errors.push(message);
        failures.push({ row: index + 2, message, raw: row });
      }
    }
    importFailures.value = failures;
    if (errors.length) {
      importError.value = t('binding.macImportError', { count: errors.length });
    }
    if (successCount || skippedDuplicate) {
      showSuccess(t('binding.macImportSuccess', { success: successCount, skipped: skippedDuplicate }));
      appendAudit('bulk-import', t('binding.macImportAudit', { count: successCount }));
      await loadData();
    }
    if (!failures.length) {
      importDialog.value = false;
    }
  } catch (error: any) {
    const resp = error?.response?.data;
    const fields = resp?.details?.fields;
    if (Array.isArray(fields) && fields.length) {
      importError.value = fields.map((f: any) => f.message).join(', ');
    } else {
      importError.value = error?.message || resp?.message || t('binding.macImportFail');
    }
  } finally {
    importing.value = false;
  }
};

const openCreate = () => {
  editing.value = null;
  drawerVisible.value = true;
};

const openEdit = (row: Binding) => {
  editing.value = row;
  drawerVisible.value = true;
};

const openDetails = (row: Binding) => {
  detailRow.value = row;
  detailVisible.value = true;
};

const appendAudit = (bindingId: string, action: string) => {
  auditLogs.value.unshift({
    id: `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    bindingId,
    action,
    time: Date.now()
  });
};

const checkConflict = async (ip: string) => {
  try {
    const { data } = await getConflicts({ ip, tenantId: tenantStore.currentTenantId || 'global' });
    if (!data.data?.items?.length) return false;
    return data.data.items.some((item) => item.id !== editing.value?.id);
  } catch (error) {
    return false;
  }
};

const handleSave = async (payload: Partial<Binding>) => {
  if (!canManage.value) {
    showError(t('binding.noPermission'));
    return;
  }
  if (saveSubmitting.value) return;
  saveSubmitting.value = true;
  try {
    const tenantId = tenantStore.currentTenantId || 'global';
    const baseMetadata =
      payload.metadata && typeof payload.metadata === 'object' ? { ...payload.metadata } : {};
    if (payload.hostname !== undefined) {
      baseMetadata.hostname = payload.hostname;
    }
    if (payload.description !== undefined) {
      baseMetadata.description = payload.description;
    }
    if (payload.options) {
      baseMetadata.options = payload.options;
    }
    const body: Partial<Binding> & {
      tenantId?: string;
      identifier?: string;
      identifierType?: string;
      ipAddress?: string;
      leaseProfileId?: string;
    } = {
      ...payload,
      tenantId,
      status: payload.status,
      metadata: baseMetadata
    };

    // API expects identifier/identifierType/ipAddress; normalize from form inputs.
    body.identifier = payload.mac || payload.identifier;
    body.identifierType = payload.identifierType || 'mac';
    body.ipAddress = payload.ip || payload.ipAddress;
    // Backend requires lease_profile_id FK; use default if none provided.
    body.leaseProfileId = payload.leaseProfileId || 'default-lease-profile';
    if (editing.value?.id) {
      await updateBinding(editing.value.id, body);
      appendAudit(editing.value.id, t('binding.macEditAudit', { mac: payload.mac || editing.value.mac || '' }));
      showSuccess(t('binding.updated'));
    } else {
      await createBinding(body);
      appendAudit(String(payload.mac || payload.identifier || 'new'), t('binding.macCreateAudit', { mac: payload.mac || '' }));
      showSuccess(t('binding.created'));
    }
    drawerVisible.value = false;
    editing.value = null;
    await loadData();
  } catch (error) {
    const apiMsg = (error as any)?.response?.data?.message;
    console.error('binding.save.error', { payload, error: (error as any)?.response?.data });
    showError(apiMsg || t('binding.saveFail'));
  } finally {
    saveSubmitting.value = false;
  }
};

const handleDelete = async (row: Binding) => {
  if (!canManage.value) {
    showError(t('binding.noPermission'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('binding.deleteConfirm'), t('common.confirm'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning'
    });
    await deleteBinding(row.id, { tenantId: tenantStore.currentTenantId || undefined });
    appendAudit(row.id, t('binding.macDeleteAudit', { mac: row.mac || '' }));
    showSuccess(t('binding.deleted'));
    await loadData();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('binding.deleteFail'));
    }
  }
};

const handleBulkDelete = async () => {
  if (!selectionIds.value.length) return;
  try {
    await ElMessageBox.confirm(t('binding.bulkDeleteConfirm'), t('common.confirm'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning'
    });
    await Promise.all(
      selectionIds.value.map((id) =>
        deleteBinding(id, { tenantId: tenantStore.currentTenantId || undefined })
      )
    );
    appendAudit('batch-delete', t('binding.macBulkDeleteAudit', { count: selectionIds.value.length }));
    showSuccess(t('binding.bulkDeleted'));
    await loadData();
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      showError(t('binding.deleteFail'));
    }
  }
};

const downloadImportFailures = () => {
  if (!importFailures.value.length) return;
  const header = ['row', 'message', 'raw'];
  const body = importFailures.value.map((item) => [item.row, item.message, item.raw.join('|')]);
  const csv = [header, ...body]
    .map((line) => line.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `mac-bindings-import-errors-${Date.now()}.csv`;
  link.click();
  URL.revokeObjectURL(url);
};

const toUpdatePayload = (row: Binding, enabled: boolean) => {
  const metadata = {
    ...((row.metadata as Record<string, unknown>) || {}),
    enabled,
    hostname: row.hostname,
    description: row.description
  };
  return {
    tenantId: tenantStore.currentTenantId || 'global',
    identifier: row.mac || row.identifier,
    identifierType: row.identifierType || 'mac',
    ipAddress: row.ip || row.ipAddress,
    poolId: row.poolId,
    hostname: row.hostname,
    description: row.description,
    leaseProfileId: row.leaseProfileId || 'default-lease-profile',
    status: row.status,
    metadata
  };
};

const toggleBindingEnabled = async (row: Binding, enabled: boolean) => {
  if (!canManage.value) return;
  try {
    await updateBinding(row.id, toUpdatePayload(row, enabled));
    appendAudit(row.id, t('binding.macToggleAudit', { action: enabled ? t('binding.macEnable') : t('binding.macDisable'), mac: row.mac || '' }));
    showSuccess(enabled ? t('binding.macToggleEnable') : t('binding.macToggleDisable'));
    await loadData();
  } catch {
    showError(enabled ? t('binding.macToggleEnableFail') : t('binding.macToggleDisableFail'));
  }
};

const handleBulkEnable = async () => {
  if (!selectedRows.value.length || !canManage.value) return;
  try {
    await Promise.all(selectedRows.value.map((row) => updateBinding(row.id, toUpdatePayload(row, true))));
    appendAudit('batch-enable', t('binding.macBulkEnableAudit', { count: selectedRows.value.length }));
    showSuccess(t('binding.macBulkEnableSuccess', { count: selectedRows.value.length }));
    await loadData();
  } catch {
    showError(t('binding.macBulkEnableFail'));
  }
};

const handleBulkDisable = async () => {
  if (!selectedRows.value.length || !canManage.value) return;
  try {
    await Promise.all(
      selectedRows.value.map((row) => updateBinding(row.id, toUpdatePayload(row, false)))
    );
    appendAudit('batch-disable', t('binding.macBulkDisableAudit', { count: selectedRows.value.length }));
    showSuccess(t('binding.macBulkDisableSuccess', { count: selectedRows.value.length }));
    await loadData();
  } catch {
    showError(t('binding.macBulkDisableFail'));
  }
};

const statusTag = (status: DisplayStatus) => {
  if (status === 'online') return 'success';
  if (status === 'warning') return 'warning';
  if (status === 'disabled') return 'danger';
  return 'info';
};

const statusLabel = (status: DisplayStatus) => {
  if (status === 'online') return t('binding.statusOnline');
  if (status === 'warning') return t('binding.statusWarning');
  if (status === 'offline') return t('binding.statusOffline');
  if (status === 'disabled') return t('binding.macStatusDisabled');
  return status;
};

const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
};

const startAutoRefresh = () => {
  stopAutoRefresh();
  if (!autoRefreshEnabled.value) return;
  autoRefreshTimer = setInterval(() => {
    if (loading.value) return;
    loadData({ page: pagination.page, pageSize: pagination.pageSize });
  }, autoRefreshSeconds.value * 1000);
};

watch([autoRefreshEnabled, autoRefreshSeconds], () => {
  startAutoRefresh();
});

onMounted(() => {
  bindingStore.setFilters({ tenantId: tenantStore.currentTenantId || undefined });
  loadData();
  loadPools();
});

onUnmounted(() => {
  stopAutoRefresh();
});
</script>

<style scoped>
.binding-mac {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: calc(100vh - 140px);
  padding: 0 24px 16px;
  box-sizing: border-box;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.page-header {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.page-title {
  margin: 0;
  font-size: 18px;
  line-height: 26px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}

.page-subtitle {
  margin: 0;
  font-size: 14px;
  line-height: 22px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}

.stat-card {
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.stat-card.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--el-color-primary) 45%, transparent);
}

.stat-title {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.stat-value {
  font-size: 30px;
  font-weight: 600;
}

.stat-desc {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

.surface-card {
  background: var(--el-bg-color);
  border-radius: 12px;
  padding: 16px;
  border: 1px solid var(--el-border-color-light);
}

.toolbar {
  display: grid;
  grid-template-columns: minmax(420px, 1fr) auto auto;
  align-items: center;
  gap: 12px;
}

.toolbar-middle {
  display: flex;
  gap: 8px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: flex-end;
}

.auto-refresh {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.list-refresh {
  background: var(--el-color-primary);
  color: var(--el-color-white);
  border: none;
}

.list-refresh:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 1px;
}

.table-panel {
  display: flex;
  flex-direction: column;
  min-height: 520px;
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: auto;
}

.audit-box {
  margin-top: 16px;
}

.audit-title {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 10px;
}

.audit-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.audit-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}

.audit-time {
  color: var(--el-text-color-secondary);
}

.audit-action {
  color: var(--el-text-color-primary);
  font-weight: 500;
}

.import-failures {
  margin-top: 12px;
  padding: 10px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: var(--el-fill-color-light);
}

.import-failures-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.import-failures ul {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}


.import-body .tip {
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}

.import-form {
  margin-bottom: 12px;
}

.field-tip {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
}

.field-list span {
  padding: 2px 8px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  font-size: 12px;
}

@media (max-width: 1360px) {
  .toolbar {
    grid-template-columns: 1fr;
  }

  .toolbar-right {
    justify-content: flex-start;
  }
}

@media (max-width: 1200px) {
  .binding-mac {
    padding-left: 20px;
    padding-right: 20px;
  }
}
</style>
