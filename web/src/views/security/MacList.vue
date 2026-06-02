<template>
  <div class="page surface-card mac-list-page">
    <el-card shadow="never" class="module-card">
      <div class="stats-grid">
        <button
          v-for="card in statCards"
          :key="card.type"
          class="stat-card"
          :class="[`type-${card.type}`, { active: activeTypeCard === card.type }]"
          type="button"
          @click="toggleTypeCard(card.type)"
        >
          <div class="stat-title">{{ card.label }}</div>
          <div class="stat-value">{{ card.value }}</div>
        </button>
      </div>

      <div class="filter-bulk-row">
        <div class="left-filters">
          <el-select v-model="query.type" clearable :placeholder="t('security.macList.filterTypePlaceholder')" class="w-140">
            <el-option :label="t('security.macList.ruleWhitelist')" value="whitelist" />
            <el-option :label="t('security.macList.ruleBlacklist')" value="blacklist" />
            <el-option :label="t('security.macList.ruleGraylist')" value="graylist" />
          </el-select>
          <el-select v-model="query.action" clearable :placeholder="t('security.macList.filterActionPlaceholder')" class="w-140">
            <el-option :label="t('security.macList.actionAllow')" value="allow" />
            <el-option :label="t('security.macList.actionBlock')" value="block" />
            <el-option :label="t('security.macList.actionMonitor')" value="monitor" />
          </el-select>
          <el-select v-model="query.enabled" clearable :placeholder="t('security.macList.filterEnabledPlaceholder')" class="w-140">
            <el-option :label="t('security.macList.detailEnabledOn')" value="true" />
            <el-option :label="t('security.macList.detailEnabledOff')" value="false" />
          </el-select>
          <el-input
            v-model="query.keyword"
            clearable
            class="w-220"
            :placeholder="t('security.macList.filterKeywordPlaceholder')"
            @keyup.enter="handleSearch"
          />
        </div>

        <div class="right-actions">
          <el-button :loading="loading" @click="handleSearch">{{ t('security.macList.btnSearch') }}</el-button>
          <el-button @click="downloadTemplate">{{ t('security.macList.btnDownloadTemplate') }}</el-button>
          <el-button :disabled="!filteredRows.length" @click="exportCsv">{{ t('security.macList.btnExport') }}</el-button>
          <el-button type="primary" plain :disabled="!canManage" @click="openImport">{{ t('security.macList.btnImport') }}</el-button>
          <el-button type="primary" :disabled="!canManage" @click="openCreate">{{ t('security.macList.btnCreate') }}</el-button>
        </div>
      </div>

      <div class="batch-bar">
        <span class="batch-tip">{{ t('security.macList.selectedCount', { n: selectedRows.length }) }}</span>
        <el-button size="small" :disabled="!hasSelection || !canManage" @click="batchSetEnabled(true)">
          {{ t('security.macList.batchEnable') }}
        </el-button>
        <el-button size="small" :disabled="!hasSelection || !canManage" @click="batchSetEnabled(false)">
          {{ t('security.macList.batchDisable') }}
        </el-button>
        <el-button size="small" :disabled="!hasSelection || !canManage" @click="batchRenewVisible = true">
          {{ t('security.macList.batchRenew') }}
        </el-button>
        <el-button size="small" :disabled="!hasSelection || !canManage" @click="batchPriorityVisible = true">
          {{ t('security.macList.batchPriority') }}
        </el-button>
        <el-popconfirm
          :title="t('security.macList.batchDeleteConfirm')"
          width="260"
          @confirm="batchDelete"
        >
          <template #reference>
            <el-button size="small" type="danger" plain :disabled="!hasSelection || !canManage">{{ t('security.macList.batchDelete') }}</el-button>
          </template>
        </el-popconfirm>
      </div>

      <el-table
        v-loading="loading"
        :data="pagedRows"
        border
        stripe
        class="data-table"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column prop="mac" :label="t('security.macList.colMac')" min-width="170" />
        <el-table-column :label="t('security.macList.colType')" width="120">
          <template #default="{ row }">
            <el-tag :type="typeTag(row.type)">{{ renderType(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('security.macList.colAction')" width="100">
          <template #default="{ row }">
            <el-tag :type="actionTag(row.action)">{{ renderAction(row.action) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('security.macList.colEnabled')" width="120">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :disabled="!canManage"
              @change="handleEnabledSwitch(row, $event)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="priority" :label="t('security.macList.colPriority')" width="100" />
        <el-table-column :label="t('security.macList.colValidity')" min-width="220">
          <template #default="{ row }">
            <span>{{ formatValidity(row.validFrom, row.validUntil) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('security.macList.colSource')" width="120">
          <template #default="{ row }">
            <span>{{ renderSource(row.source) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('security.macList.colUpdated')" width="170">
          <template #default="{ row }">{{ formatTs(row.updatedAt || row.createdAt || '') }}</template>
        </el-table-column>
        <el-table-column :label="t('security.macList.colActions')" width="260" fixed="right" class-name="action-column">
          <template #default="{ row }">
            <div class="action-group">
              <el-button type="primary" plain size="small" @click="openDetail(row)">{{ t('security.macList.btnDetail') }}</el-button>
              <el-button size="small" @click="openEdit(row)" :disabled="!canManage">{{ t('security.macList.btnEdit') }}</el-button>
              <el-popconfirm :title="t('security.macList.deleteRuleConfirm')" width="220" @confirm="removeEntry(row)">
                <template #reference>
                  <el-button type="danger" plain size="small" :disabled="!canManage">{{ t('security.macList.btnDelete') }}</el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && !pagedRows.length" :description="t('security.macList.emptyData')" />

      <div class="pager-row">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.pageSize"
          :total="filteredRows.length"
          layout="total, sizes, prev, pager, next, jumper"
          :page-sizes="[10, 20, 50, 100]"
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
        />
      </div>
    </el-card>

    <el-dialog v-model="editorVisible" :title="editorTitle" width="560px" destroy-on-close>
      <el-form ref="editorFormRef" :model="editorForm" :rules="editorRules" label-width="120px">
        <el-form-item :label="t('security.macList.formMac')" prop="mac">
          <el-input v-model="editorForm.mac" placeholder="00:11:22:33:44:55" @input="onMacInput" />
          <div v-if="macConflictHint" class="conflict-hint">{{ macConflictHint }}</div>
        </el-form-item>
        <el-form-item :label="t('security.macList.formType')" prop="type">
          <el-select v-model="editorForm.type">
            <el-option :label="t('security.macList.ruleWhitelist')" value="whitelist" />
            <el-option :label="t('security.macList.ruleBlacklist')" value="blacklist" />
            <el-option :label="t('security.macList.ruleGraylist')" value="graylist" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('security.macList.formAction')" prop="action">
          <el-select v-model="editorForm.action">
            <el-option :label="t('security.macList.actionAllow')" value="allow" />
            <el-option :label="t('security.macList.actionBlock')" value="block" />
            <el-option :label="t('security.macList.actionMonitor')" value="monitor" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('security.macList.formPriority')" prop="priority">
          <el-input-number v-model="editorForm.priority" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item :label="t('security.macList.formSource')" prop="source">
          <el-select v-model="editorForm.source">
            <el-option v-for="item in sourceOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('security.macList.formEnabled')">
          <el-switch v-model="editorForm.enabled" />
        </el-form-item>
        <el-form-item :label="t('security.macList.formValidity')">
          <el-date-picker
            v-model="editorForm.validityRange"
            type="datetimerange"
            :start-placeholder="t('security.macList.formStartTime')"
            :end-placeholder="t('security.macList.formEndTime')"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item :label="t('security.macList.formDescription')">
          <el-input v-model="editorForm.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('security.macList.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" :disabled="!canManage" @click="saveEntry">{{ t('security.macList.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" :title="t('security.macList.importTitle')" width="620px" destroy-on-close>
      <el-upload drag :auto-upload="false" :show-file-list="false" accept=".csv" @change="handleImportFile">
        <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
        <div class="el-upload__text">{{ t('security.macList.importUploadHint') }}</div>
        <template #tip>
          <div class="el-upload__tip">{{ t('security.macList.importFieldHint') }}</div>
        </template>
      </el-upload>

      <el-alert
        v-if="importErrorSummary"
        type="error"
        :title="importErrorSummary"
        :closable="false"
        show-icon
        class="mt-12"
      />
      <el-scrollbar v-if="importErrors.length" max-height="160" class="import-errors">
        <div v-for="err in importErrors" :key="err" class="import-error-item">{{ err }}</div>
      </el-scrollbar>

      <template #footer>
        <el-button :disabled="importing" @click="importVisible = false">{{ t('security.macList.importCancel') }}</el-button>
        <el-button type="primary" :loading="importing" :disabled="!importFile" @click="submitImport">{{ t('security.macList.importSubmit') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchRenewVisible" :title="t('security.macList.renewTitle')" width="420px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item :label="t('security.macList.renewDays')">
          <el-input-number v-model="batchRenewDays" :min="1" :max="3650" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchRenewVisible = false">{{ t('security.macList.cancel') }}</el-button>
        <el-button type="primary" :disabled="!hasSelection || !canManage" @click="batchRenew">{{ t('security.macList.renewConfirm') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchPriorityVisible" :title="t('security.macList.priorityTitle')" width="460px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item :label="t('security.macList.priorityMode')">
          <el-radio-group v-model="batchPriorityMode">
            <el-radio label="set">{{ t('security.macList.priorityModeSet') }}</el-radio>
            <el-radio label="delta">{{ t('security.macList.priorityModeDelta') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="batchPriorityMode === 'set' ? t('security.macList.priorityTargetLabel') : t('security.macList.priorityDeltaLabel')">
          <el-input-number v-model="batchPriorityValue" :min="-10000" :max="10000" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchPriorityVisible = false">{{ t('security.macList.cancel') }}</el-button>
        <el-button type="primary" :disabled="!hasSelection || !canManage" @click="batchAdjustPriority">{{ t('security.macList.priorityConfirm') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" :title="t('security.macList.detailTitle')" size="540px">
      <div v-if="detailRow" class="detail-wrap">
        <el-descriptions :column="1" border>
          <el-descriptions-item :label="t('security.macList.detailMac')">{{ detailRow.mac }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailType')">
            <el-tag :type="typeTag(detailRow.type)">{{ renderType(detailRow.type) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailAction')">
            <el-tag :type="actionTag(detailRow.action)">{{ renderAction(detailRow.action) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailEnabled')">
            <el-tag :type="detailRow.enabled ? 'success' : 'info'">{{ detailRow.enabled ? t('security.macList.detailEnabledOn') : t('security.macList.detailEnabledOff') }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailPriority')">{{ detailRow.priority || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailSource')">{{ renderSource(detailRow.source) }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailValidity')">{{ formatValidity(detailRow.validFrom, detailRow.validUntil) }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailDescription')">{{ detailRow.description || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailCreated')">{{ formatTs(detailRow.createdAt || '') }}</el-descriptions-item>
          <el-descriptions-item :label="t('security.macList.detailUpdated')">{{ formatTs(detailRow.updatedAt || '') }}</el-descriptions-item>
        </el-descriptions>

        <div class="record-head">{{ t('security.macList.logTitle') }}</div>
        <el-timeline>
          <el-timeline-item v-for="item in operationLogs" :key="item.id" :timestamp="item.time" placement="top">
            {{ item.text }}
          </el-timeline-item>
        </el-timeline>

        <div class="record-head">{{ t('security.macList.matchTitle') }}</div>
        <el-empty v-if="!matchEvents.length" :description="t('security.macList.matchEmpty')" :image-size="52" />
        <el-timeline v-else>
          <el-timeline-item v-for="evt in matchEvents" :key="evt.id" :timestamp="evt.time" placement="top">
            {{ evt.text }}
          </el-timeline-item>
        </el-timeline>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { FormInstance, FormRules } from 'element-plus';
import { UploadFilled } from '@element-plus/icons-vue';
import { createMacList, deleteMacList, listMacLists, recordMacListAuditAction, updateMacList } from '@/api/security';
import type { MacListAction, MacListEntry, MacListType } from '@/types/security';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { showHttpError } from '@/shared/errors/errorToast';
import { showError, showSuccess, showWarning } from '@/shared/errors/messageToast';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();

const loading = ref(false);
const saving = ref(false);
const importing = ref(false);
const allEntries = ref<MacListEntry[]>([]);
const selectedRows = ref<MacListEntry[]>([]);

const activeTypeCard = ref<MacListType | ''>('');

const query = reactive({
  type: '' as MacListType | '',
  action: '' as MacListAction | '',
  enabled: '' as '' | 'true' | 'false',
  keyword: '',
  page: 1,
  pageSize: 20
});

const editorVisible = ref(false);
const editorFormRef = ref<FormInstance>();
const editorForm = reactive({
  id: '',
  mac: '',
  type: 'whitelist' as MacListType,
  action: 'allow' as MacListAction,
  priority: 100,
  source: 'manual',
  enabled: true,
  description: '',
  validityRange: [] as [Date, Date] | []
});

const importVisible = ref(false);
const importFile = ref<File | null>(null);
const importErrors = ref<string[]>([]);
const importErrorSummary = ref('');

const batchRenewVisible = ref(false);
const batchRenewDays = ref(30);
const batchPriorityVisible = ref(false);
const batchPriorityMode = ref<'set' | 'delta'>('set');
const batchPriorityValue = ref(100);

const detailVisible = ref(false);
const detailRow = ref<MacListEntry | null>(null);

const canView = computed(() => permissionStore.can('security.view'));
const canManage = computed(() => permissionStore.can('security.manage'));
const hasSelection = computed(() => selectedRows.value.length > 0);

const sourceOptions = [
  { value: 'manual', label: t('security.macList.sourceManual') },
  { value: 'import', label: t('security.macList.sourceImport') },
  { value: 'sync-dhcp', label: t('security.macList.sourceSyncDhcp') },
  { value: 'sync-nac', label: t('security.macList.sourceSyncNac') },
  { value: 'sync-cmdb', label: t('security.macList.sourceSyncCmdb') },
  { value: 'api', label: t('security.macList.sourceApi') },
  { value: 'automation', label: t('security.macList.sourceAutomation') }
];

const normalizeMac = (value?: string) => {
  const raw = String(value || '').trim().replace(/-/g, ':').toUpperCase();
  return raw;
};

const isValidMac = (value?: string) => /^([0-9A-F]{2}:){5}[0-9A-F]{2}$/.test(normalizeMac(value));

const statCards = computed(() => {
  const stats = allEntries.value.reduce(
    (acc, item) => {
      if (item.type === 'whitelist') acc.whitelist += 1;
      if (item.type === 'blacklist') acc.blacklist += 1;
      if (item.type === 'graylist') acc.graylist += 1;
      return acc;
    },
    { whitelist: 0, blacklist: 0, graylist: 0 }
  );
  return [
    { type: 'whitelist' as MacListType, label: t('security.macList.statWhitelist'), value: stats.whitelist },
    { type: 'blacklist' as MacListType, label: t('security.macList.statBlacklist'), value: stats.blacklist },
    { type: 'graylist' as MacListType, label: t('security.macList.statGraylist'), value: stats.graylist }
  ];
});

const filteredRows = computed(() => {
  const keyword = query.keyword.trim().toLowerCase();
  return allEntries.value.filter((item) => {
    if (activeTypeCard.value && item.type !== activeTypeCard.value) return false;
    if (query.type && item.type !== query.type) return false;
    if (query.action && item.action !== query.action) return false;
    if (query.enabled) {
      const enabled = query.enabled === 'true';
      if (item.enabled !== enabled) return false;
    }
    if (keyword) {
      const text = [item.mac, item.description, item.source].join(' ').toLowerCase();
      if (!text.includes(keyword)) return false;
    }
    return true;
  });
});

const pagedRows = computed(() => {
  const start = (query.page - 1) * query.pageSize;
  return filteredRows.value.slice(start, start + query.pageSize);
});

const findConflict = (mac: string, type: MacListType, excludeId?: string) => {
  const normalized = normalizeMac(mac);
  if (!normalized) return null;
  return allEntries.value.find((item) => {
    if (excludeId && item.id === excludeId) return false;
    return normalizeMac(item.mac) === normalized && item.type !== type;
  });
};

const macConflictHint = computed(() => {
  if (!editorForm.mac) return '';
  const conflict = findConflict(editorForm.mac, editorForm.type, editorForm.id || undefined);
  if (!conflict) return '';
  return t('security.macList.conflictHint', { type: renderType(conflict.type), action: renderAction(conflict.action) });
});

const editorRules: FormRules = {
  mac: [
    { required: true, message: t('security.macList.macRequired'), trigger: 'blur' },
    {
      validator: (
        _rule: unknown,
        value: string,
        callback: (error?: Error) => void
      ) => {
        if (!isValidMac(value)) {
          callback(new Error(t('security.macList.macInvalid')));
          return;
        }
        callback();
      },
      trigger: 'blur'
    }
  ],
  type: [{ required: true, message: t('security.macList.typeRequired'), trigger: 'change' }],
  action: [{ required: true, message: t('security.macList.actionRequired'), trigger: 'change' }]
};

const editorTitle = computed(() => (editorForm.id ? t('security.macList.editorTitleEdit') : t('security.macList.editorTitleCreate')));

const operationLogs = computed(() => {
  const row = detailRow.value;
  if (!row) return [] as Array<{ id: string; time: string; text: string }>;
  const metadataLogs = Array.isArray((row.metadata as any)?.operationLogs)
    ? (row.metadata as any).operationLogs
    : [];
  if (metadataLogs.length > 0) {
    return metadataLogs.map((item: any, index: number) => ({
      id: `meta-log-${index}`,
      time: formatTs(String(item.time || item.ts || row.updatedAt || row.createdAt || '')),
      text: String(item.text || item.action || t('security.macList.logOperated'))
    }));
  }
  return [
    {
      id: 'created',
      time: formatTs(row.createdAt || ''),
      text: t('security.macList.logCreated')
    },
    {
      id: 'updated',
      time: formatTs(row.updatedAt || row.createdAt || ''),
      text: row.enabled ? t('security.macList.logEnabledState') : t('security.macList.logDisabledState')
    }
  ];
});

const matchEvents = computed(() => {
  const row = detailRow.value;
  if (!row) return [] as Array<{ id: string; time: string; text: string }>;
  const metadataEvents = Array.isArray((row.metadata as any)?.matchEvents)
    ? (row.metadata as any).matchEvents
    : [];
  return metadataEvents.map((item: any, index: number) => ({
    id: `match-${index}`,
    time: formatTs(String(item.time || item.ts || row.updatedAt || row.createdAt || '')),
    text: String(item.text || item.summary || t('security.macList.logMatched'))
  }));
});

const typeTag = (type: MacListType) => {
  if (type === 'blacklist') return 'danger';
  if (type === 'graylist') return 'warning';
  return 'success';
};

const actionTag = (action: MacListAction) => {
  if (action === 'block') return 'danger';
  if (action === 'monitor') return 'warning';
  return 'success';
};

const renderType = (type: MacListType) => {
  if (type === 'blacklist') return t('security.macList.ruleBlacklist');
  if (type === 'graylist') return t('security.macList.ruleGraylist');
  return t('security.macList.ruleWhitelist');
};

const renderAction = (action: MacListAction) => {
  if (action === 'block') return t('security.macList.actionBlock');
  if (action === 'monitor') return t('security.macList.actionMonitor');
  return t('security.macList.actionAllow');
};

const renderSource = (source?: string) => {
  if (!source) return '-';
  const found = sourceOptions.find((item) => item.value === source);
  return found?.label || source;
};

const formatValidity = (from?: string, to?: string) => {
  if (!from && !to) return '-';
  if (from && to) return `${formatTs(from)} ~ ${formatTs(to)}`;
  if (from) return `${formatTs(from)} ~`;
  return `~ ${formatTs(to || '')}`;
};

const buildPayloadFromEntry = (entry: MacListEntry, overrides?: Partial<MacListEntry>) => {
  const next = { ...entry, ...overrides };
  return {
    mac: normalizeMac(next.mac),
    type: next.type,
    action: next.action,
    description: next.description || '',
    source: next.source || 'manual',
    priority: Number(next.priority || 100),
    enabled: Boolean(next.enabled),
    validFrom: next.validFrom,
    validUntil: next.validUntil,
    metadata: next.metadata,
    tenantId: tenantStore.currentTenantId || undefined
  };
};

const resetEditorForm = () => {
  editorForm.id = '';
  editorForm.mac = '';
  editorForm.type = 'whitelist';
  editorForm.action = 'allow';
  editorForm.priority = 100;
  editorForm.source = 'manual';
  editorForm.enabled = true;
  editorForm.description = '';
  editorForm.validityRange = [];
};

const loadAllEntries = async () => {
  if (!canView.value) {
    allEntries.value = [];
    return;
  }
  loading.value = true;
  try {
    const pageSize = 200;
    let page = 1;
    let total = 0;
    const result: MacListEntry[] = [];
    do {
      const { data } = await listMacLists({
        page,
        pageSize,
        tenantId: tenantStore.currentTenantId || undefined
      } as any);
      const payload = data?.data;
      const items = payload?.items || [];
      total = Number(payload?.total || 0);
      result.push(...items);
      page += 1;
      if (items.length === 0) break;
    } while (result.length < total);

    allEntries.value = result
      .map((item) => ({ ...item, mac: normalizeMac(item.mac) }))
      .sort((a, b) => {
        const ta = Date.parse(b.updatedAt || b.createdAt || '') || 0;
        const tb = Date.parse(a.updatedAt || a.createdAt || '') || 0;
        return ta - tb;
      });

    if ((query.page - 1) * query.pageSize >= filteredRows.value.length) {
      query.page = 1;
    }
  } catch (error) {
    allEntries.value = [];
    showHttpError(error, t('security.macList.loadFail'));
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  query.page = 1;
};

const handlePageChange = (value: number) => {
  query.page = value;
};

const handlePageSizeChange = (value: number) => {
  query.pageSize = value;
  query.page = 1;
};

const onSelectionChange = (rows: MacListEntry[]) => {
  selectedRows.value = rows;
};

const toggleTypeCard = (type: MacListType) => {
  activeTypeCard.value = activeTypeCard.value === type ? '' : type;
  query.page = 1;
};

const openCreate = () => {
  if (!canManage.value) {
    showWarning(t('security.macList.noPermission'));
    return;
  }
  resetEditorForm();
  editorVisible.value = true;
};

const openEdit = (row: MacListEntry) => {
  if (!canManage.value) {
    showWarning(t('security.macList.noPermission'));
    return;
  }
  editorForm.id = row.id;
  editorForm.mac = normalizeMac(row.mac);
  editorForm.type = row.type;
  editorForm.action = row.action;
  editorForm.priority = Number(row.priority || 100);
  editorForm.source = row.source || 'manual';
  editorForm.enabled = Boolean(row.enabled);
  editorForm.description = row.description || '';
  editorForm.validityRange =
    row.validFrom || row.validUntil
      ? [new Date(row.validFrom || Date.now()), new Date(row.validUntil || Date.now())]
      : [];
  editorVisible.value = true;
};

const onMacInput = (value: string) => {
  editorForm.mac = normalizeMac(value);
};

const saveEntry = async () => {
  if (!canManage.value) return;
  const formRef = editorFormRef.value;
  if (!formRef) return;
  const valid = await formRef.validate().catch(() => false);
  if (!valid) return;

  const conflict = findConflict(editorForm.mac, editorForm.type, editorForm.id || undefined);
  if (conflict) {
    showError(t('security.macList.conflictHint', { type: renderType(conflict.type), action: renderAction(conflict.action) }));
    return;
  }

  const payload: Partial<MacListEntry> & { tenantId?: string } = {
    mac: normalizeMac(editorForm.mac),
    type: editorForm.type,
    action: editorForm.action,
    priority: Number(editorForm.priority || 100),
    source: editorForm.source,
    enabled: editorForm.enabled,
    description: editorForm.description,
    validFrom: editorForm.validityRange[0]?.toISOString(),
    validUntil: editorForm.validityRange[1]?.toISOString(),
    tenantId: tenantStore.currentTenantId || undefined
  };

  saving.value = true;
  try {
    if (editorForm.id) {
      await updateMacList(editorForm.id, payload);
      showSuccess(t('security.macList.ruleUpdated'));
    } else {
      await createMacList(payload);
      showSuccess(t('security.macList.ruleCreated'));
    }
    editorVisible.value = false;
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.saveFail'));
  } finally {
    saving.value = false;
  }
};

const removeEntry = async (row: MacListEntry) => {
  try {
    await deleteMacList(row.id, { tenantId: tenantStore.currentTenantId || undefined });
    showSuccess(t('security.macList.ruleDeleted'));
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.deleteFail'));
  }
};

const toggleEnabled = async (row: MacListEntry, enabled: boolean) => {
  if (!canManage.value) return;
  try {
    await updateMacList(row.id, buildPayloadFromEntry(row, { enabled }));
    row.enabled = enabled;
    showSuccess(enabled ? t('security.macList.ruleEnabled') : t('security.macList.ruleDisabled'));
  } catch (error) {
    showHttpError(error, t('security.macList.statusUpdateFail'));
  }
};

const handleEnabledSwitch = (row: MacListEntry, value: string | number | boolean) => {
  toggleEnabled(row, Boolean(value));
};

const batchSetEnabled = async (enabled: boolean) => {
  if (!hasSelection.value || !canManage.value) return;
  loading.value = true;
  try {
    for (const row of selectedRows.value) {
      await updateMacList(row.id, buildPayloadFromEntry(row, { enabled }));
    }
    showSuccess(enabled ? t('security.macList.batchEnableSuccess') : t('security.macList.batchDisableSuccess'));
    selectedRows.value = [];
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.batchOpFail'));
  } finally {
    loading.value = false;
  }
};

const batchRenew = async () => {
  if (!hasSelection.value || !canManage.value) return;
  const days = Number(batchRenewDays.value || 0);
  if (days <= 0) {
    showWarning(t('security.macList.renewDaysMinError'));
    return;
  }
  loading.value = true;
  try {
    const now = Date.now();
    const until = new Date(now + days * 24 * 3600 * 1000).toISOString();
    for (const row of selectedRows.value) {
      const validFrom = row.validFrom || new Date(now).toISOString();
      await updateMacList(row.id, buildPayloadFromEntry(row, { validFrom, validUntil: until }));
    }
    showSuccess(t('security.macList.batchRenewSuccess'));
    batchRenewVisible.value = false;
    selectedRows.value = [];
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.batchRenewFail'));
  } finally {
    loading.value = false;
  }
};

const batchAdjustPriority = async () => {
  if (!hasSelection.value || !canManage.value) return;
  loading.value = true;
  try {
    for (const row of selectedRows.value) {
      const current = Number(row.priority || 100);
      const next =
        batchPriorityMode.value === 'set'
          ? Number(batchPriorityValue.value || current)
          : current + Number(batchPriorityValue.value || 0);
      const safePriority = Math.min(10000, Math.max(1, next));
      await updateMacList(row.id, buildPayloadFromEntry(row, { priority: safePriority }));
    }
    showSuccess(t('security.macList.batchPrioritySuccess'));
    batchPriorityVisible.value = false;
    selectedRows.value = [];
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.batchPriorityFail'));
  } finally {
    loading.value = false;
  }
};

const batchDelete = async () => {
  if (!hasSelection.value || !canManage.value) return;
  loading.value = true;
  try {
    for (const row of selectedRows.value) {
      await deleteMacList(row.id, { tenantId: tenantStore.currentTenantId || undefined });
    }
    showSuccess(t('security.macList.batchDeleteSuccess'));
    selectedRows.value = [];
    await loadAllEntries();
  } catch (error) {
    showHttpError(error, t('security.macList.batchDeleteFail'));
  } finally {
    loading.value = false;
  }
};

const openDetail = (row: MacListEntry) => {
  detailRow.value = row;
  detailVisible.value = true;
};

const splitCsv = (content: string) => {
  const rows: string[][] = [];
  let current = '';
  let inQuotes = false;
  let row: string[] = [];

  for (let index = 0; index < content.length; index += 1) {
    const ch = content[index];
    if (ch === '"') {
      if (inQuotes && content[index + 1] === '"') {
        current += '"';
        index += 1;
      } else {
        inQuotes = !inQuotes;
      }
      continue;
    }
    if (ch === ',' && !inQuotes) {
      row.push(current);
      current = '';
      continue;
    }
    if ((ch === '\n' || ch === '\r') && !inQuotes) {
      if (ch === '\r' && content[index + 1] === '\n') index += 1;
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
  importFile.value = file?.raw instanceof File ? file.raw : null;
  importErrors.value = [];
  importErrorSummary.value = '';
};

const submitImport = async () => {
  if (!importFile.value) return;
  importing.value = true;
  importErrors.value = [];
  importErrorSummary.value = '';

  try {
    const content = await importFile.value.text();
    const rows = splitCsv(content)
      .map((row) => row.map((cell) => cell.trim()))
      .filter((row) => row.some(Boolean));

    if (!rows.length) throw new Error(t('security.macList.importEmpty'));

    const header = rows.shift()!.map((item) => item.replace(/^\ufeff/, '').toLowerCase());
    const required = ['mac', 'type', 'action'];
    const missing = required.filter((field) => !header.includes(field));
    if (missing.length > 0) {
      throw new Error(t('security.macList.importMissingFields', { fields: missing.join(', ') }));
    }

    const idx = (field: string) => header.indexOf(field);
    const existingMap = new Map(allEntries.value.map((item) => [normalizeMac(item.mac), item]));
    const staged = new Map<string, MacListType>();
    const entriesToImport: Array<Partial<MacListEntry>> = [];

    rows.forEach((row, rowIndex) => {
      const line = rowIndex + 2;
      const mac = normalizeMac(row[idx('mac')]);
      const type = (row[idx('type')] || 'whitelist') as MacListType;
      const action = (row[idx('action')] || 'allow') as MacListAction;
      const priority = Number(row[idx('priority')] || 100);
      const enabled = String(row[idx('enabled')] || 'true').toLowerCase() !== 'false';

      if (!isValidMac(mac)) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importMacError', { mac: row[idx('mac')] || '' }) }));
        return;
      }
      if (!['whitelist', 'blacklist', 'graylist'].includes(type)) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importTypeError') }));
        return;
      }
      if (!['allow', 'block', 'monitor'].includes(action)) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importActionError') }));
        return;
      }
      if (!Number.isFinite(priority) || priority < 1 || priority > 10000) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importPriorityError') }));
        return;
      }

      const existing = existingMap.get(mac);
      if (existing && existing.type !== type) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importConflictExisting', { type: renderType(existing.type), mac }) }));
        return;
      }

      const stagedType = staged.get(mac);
      if (stagedType && stagedType !== type) {
        importErrors.value.push(t('security.macList.importLineError', { line, msg: t('security.macList.importConflictFile', { mac }) }));
        return;
      }
      staged.set(mac, type);

      entriesToImport.push({
        mac,
        type,
        action,
        priority,
        enabled,
        source: row[idx('source')] || 'import',
        description: row[idx('description')] || '',
        validFrom: row[idx('validfrom')] || undefined,
        validUntil: row[idx('validuntil')] || undefined,
        tenantId: tenantStore.currentTenantId || undefined
      });
    });

    if (importErrors.value.length > 0) {
      importErrorSummary.value = t('security.macList.importValidationFail', { count: importErrors.value.length });
      return;
    }

    for (const entry of entriesToImport) {
      await createMacList(entry as any);
    }

    await recordMacListAuditAction({
      action: 'security.mac_list.import',
      count: entriesToImport.length,
      tenantId: tenantStore.currentTenantId || undefined
    });

    showSuccess(t('security.macList.importSuccess', { count: entriesToImport.length }));
    importVisible.value = false;
    importFile.value = null;
    await loadAllEntries();
  } catch (error: any) {
    importErrorSummary.value = error?.message || t('security.macList.importFail');
  } finally {
    importing.value = false;
  }
};

const downloadTemplate = () => {
  const header = ['mac', 'type', 'action', 'priority', 'source', 'description', 'enabled', 'validFrom', 'validUntil'];
  const example = ['00:11:22:33:44:55', 'whitelist', 'allow', '100', 'manual', t('security.macList.csvExampleDesc'), 'true', '', ''];
  const csv = `\ufeff${header.join(',')}\n${example.join(',')}\n`;
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'mac-list-template.csv';
  anchor.click();
  URL.revokeObjectURL(url);
};

const exportCsv = () => {
  const rows = filteredRows.value;
  if (!rows.length) return;
  void recordMacListAuditAction({
    action: 'security.mac_list.export',
    count: rows.length,
    tenantId: tenantStore.currentTenantId || undefined
  });
  const header = ['mac', 'type', 'action', 'priority', 'source', 'description', 'enabled', 'validFrom', 'validUntil'];
  const body = rows.map((item) => [
    item.mac,
    item.type,
    item.action,
    item.priority ?? '',
    item.source || '',
    item.description || '',
    item.enabled ? 'true' : 'false',
    item.validFrom || '',
    item.validUntil || ''
  ]);
  const csv = [header, ...body]
    .map((line) => line.map((cell) => (String(cell).includes(',') ? `"${String(cell).replace(/"/g, '""')}"` : String(cell))).join(','))
    .join('\n');
  const blob = new Blob([`\ufeff${csv}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'mac-list-export.csv';
  anchor.click();
  URL.revokeObjectURL(url);
};

const openImport = () => {
  importVisible.value = true;
  importErrors.value = [];
  importErrorSummary.value = '';
  importFile.value = null;
};

onMounted(loadAllEntries);
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  box-sizing: border-box;
}

.module-card {
  border-radius: 10px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.stat-card {
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  padding: 12px;
  background: var(--el-fill-color-extra-light);
  min-height: 84px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.2s ease, transform 0.2s ease;
}

.stat-card:hover {
  transform: translateY(-1px);
}

.stat-card.active {
  border-width: 2px;
}

.stat-card.type-whitelist.active {
  border-color: var(--el-color-success);
}

.stat-card.type-blacklist.active {
  border-color: var(--el-color-danger);
}

.stat-card.type-graylist.active {
  border-color: var(--el-color-warning);
}

.stat-title {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  line-height: 1;
}

.filter-bulk-row {
  margin-bottom: 10px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.left-filters,
.right-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  border: 1px solid var(--el-border-color-light);
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 10px;
}

.batch-tip {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-right: 2px;
}

.data-table :deep(.el-table__cell .cell) {
  white-space: nowrap;
}

.data-table :deep(.action-column .cell) {
  white-space: normal;
}

.action-group {
  display: flex;
  gap: 6px;
  align-items: center;
}

.pager-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.detail-wrap {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.record-head {
  font-weight: 600;
}

.conflict-hint {
  font-size: 12px;
  color: var(--el-color-danger);
  margin-top: 4px;
}

.import-errors {
  margin-top: 10px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 8px;
}

.import-error-item {
  font-size: 12px;
  color: var(--el-color-danger);
  line-height: 1.6;
}

.w-140 {
  width: 140px;
}

.w-220 {
  width: 220px;
}

@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: repeat(1, minmax(0, 1fr));
  }
}
</style>
