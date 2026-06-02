<template>
  <div class="page surface-card">
    <el-card shadow="never" class="module-card">
      <template #header>
        <div class="module-header">
          <div>
            <div class="module-title">{{ t('system.apikey.pageTitle') }}</div>
            <div class="module-subtitle">{{ t('system.apikey.pageSubtitle') }}</div>
          </div>
          <div class="header-actions">
            <el-input
              v-model="keyword"
              :placeholder="t('system.apikey.searchPh')"
              clearable
              class="w-300"
              @input="onSearch"
            />
            <el-button :loading="loading" @click="fetch">{{ t('system.apikey.refresh') }}</el-button>
            <el-button v-if="canManageApiKeys" type="primary" @click="openCreateDialog"
              >{{ t('system.apikey.createBtn') }}</el-button
            >
          </div>
        </div>
      </template>

      <div class="filter-bulk-row">
        <div class="left-filters">
          <el-select
            v-model="roleFilter"
            :placeholder="t('system.apikey.filterRole')"
            clearable
            class="w-140"
            @change="onFilterChange"
          >
            <el-option value="reader" :label="t('system.apikey.roleReader')" />
            <el-option value="admin" :label="t('system.apikey.roleAdmin')" />
          </el-select>
          <el-select
            v-model="statusFilter"
            :placeholder="t('system.apikey.filterStatus')"
            clearable
            class="w-140"
            @change="onFilterChange"
          >
            <el-option value="active" :label="t('system.apikey.statusActive')" />
            <el-option value="expired" :label="t('system.apikey.statusExpired')" />
            <el-option value="revoked" :label="t('system.apikey.statusRevoked')" />
          </el-select>
          <el-select
            v-model="ownerFilter"
            :placeholder="t('system.apikey.filterOwner')"
            clearable
            filterable
            class="w-180"
            @change="onFilterChange"
          >
            <el-option v-for="owner in ownerOptions" :key="owner" :label="owner" :value="owner" />
          </el-select>
        </div>

        <div class="right-bulk">
          <el-button @click="selectAllCurrentPage">{{ t('system.apikey.selectAll') }}</el-button>
          <el-button @click="inverseSelectionCurrentPage">{{ t('system.apikey.invertSelection') }}</el-button>
          <el-button
            :disabled="!selectedRows.length"
            :loading="actionLoading"
            @click="batchEnable"
            >{{ t('system.apikey.batchEnable') }}</el-button
          >
          <el-button
            :disabled="!selectedRows.length"
            :loading="actionLoading"
            @click="batchDisable"
            >{{ t('system.apikey.batchDisable') }}</el-button
          >
          <el-button
            type="danger"
            plain
            :disabled="!selectedRows.length"
            :loading="actionLoading"
            @click="batchDelete"
            >{{ t('system.apikey.batchDelete') }}</el-button
          >
        </div>
      </div>

      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="pagedRows"
        border
        stripe
        class="data-table"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="52" />
        <el-table-column prop="displayName" :label="t('system.apikey.colName')" min-width="180" />
        <el-table-column :label="t('system.apikey.colKey')" min-width="220">
          <template #default="{ row }">
            <div class="key-cell">
              <span class="key-text">{{ maskedKey(row) }}</span>
              <el-button text type="primary" @click="copyKey(row)">{{ t('system.apikey.copy') }}</el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colRole')" width="120">
          <template #default="{ row }">{{ permissionLabel(row.role) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colStatus')" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTagType(resolveStatus(row))">{{ statusLabel(resolveStatus(row)) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colOwner')" min-width="150">
          <template #default="{ row }">{{ ownerLabel(row) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colLastUsed')" min-width="160">
          <template #default="{ row }">{{ formatTime(row.lastUsedAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colExpiry')" min-width="180">
          <template #default="{ row }">
            <div :class="['expire-cell', expiryClass(row)]">{{ formatTime(row.expiresAt) }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colCreated')" min-width="160">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="t('system.apikey.colAction')" width="380" fixed="right" class-name="action-column">
          <template #default="{ row }">
            <div class="action-group">
              <el-button type="primary" plain size="small" @click="openDetailDrawer(row)">{{ t('system.apikey.detail') }}</el-button>
              <el-button type="primary" plain size="small" :disabled="!canManageApiKeys" @click="openEditDialog(row)">{{ t('system.apikey.edit') }}</el-button>
              <el-button type="primary" plain size="small" @click="copyKey(row)">{{ t('system.apikey.copy') }}</el-button>
              <el-popconfirm
                :title="resolveStatus(row) === 'revoked' ? t('system.apikey.confirmEnable') : t('system.apikey.confirmDisable')"
                @confirm="toggleKeyStatus(row)"
              >
                <template #reference>
                  <el-button
                    :type="resolveStatus(row) === 'revoked' ? 'success' : 'warning'"
                    plain
                    size="small"
                    :disabled="resolveStatus(row) === 'expired'"
                    >{{ resolveStatus(row) === 'revoked' ? t('system.apikey.enable') : t('system.apikey.disable') }}</el-button
                  >
                </template>
              </el-popconfirm>
              <el-popconfirm
                :title="t('system.apikey.confirmDelete')"
                @confirm="revokeSingle(row, 'delete')"
              >
                <template #reference>
                  <el-button type="danger" plain size="small" :disabled="isBuiltInKey(row)">{{ t('system.apikey.deleteBtn') }}</el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && !pagedRows.length" :description="t('system.apikey.emptyDesc')">
        <el-button v-if="canManageApiKeys" type="primary" @click="openCreateDialog">{{ t('system.apikey.createBtn') }}</el-button>
      </el-empty>

      <div class="pager-row">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="filteredRows.length"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>

    <el-dialog v-model="createDialog.visible" :title="editingKeyId ? t('system.apikey.editTitle') : t('system.apikey.createTitle')" width="620px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item :label="t('system.apikey.formName')" prop="displayName">
          <el-input v-model="form.displayName" :placeholder="t('system.apikey.formNamePh')" />
        </el-form-item>
        <el-form-item :label="t('system.apikey.formRole')" prop="role">
          <el-select v-model="form.role" class="w-220">
            <el-option value="reader" :label="t('system.apikey.roleReader')" />
            <el-option value="admin" :label="t('system.apikey.roleAdmin')" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('system.apikey.formCapabilities')">
          <el-select v-model="form.capabilities" multiple filterable collapse-tags class="w-420">
            <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('system.apikey.formExpiry')" prop="expiresAt">
          <el-date-picker
            v-model="form.expiresAt"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ss[Z]"
            :disabled-date="isExpiryDateDisabled"
            :placeholder="t('system.apikey.formExpiryPh')"
          />
        </el-form-item>
        <el-form-item :label="t('system.apikey.formOwner')">
          <el-select v-model="form.ownerUserId" filterable clearable class="w-300" :placeholder="t('system.apikey.formOwnerPh')">
            <el-option v-for="user in userOptions" :key="user.id" :label="user.label" :value="user.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('system.apikey.formIpWhitelist')">
          <el-input
            v-model="ipWhitelistText"
            type="textarea"
            :rows="4"
            :placeholder="t('system.apikey.formIpWhitelistPh')"
          />
        </el-form-item>
        <el-form-item :label="t('system.apikey.formDesc')">
          <el-input v-model="form.description" type="textarea" :rows="2" :placeholder="t('system.apikey.formDescPh')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialog.visible = false">{{ t('system.apikey.btnCancel') }}</el-button>
        <el-button type="primary" :loading="actionLoading" :disabled="actionLoading" @click="submitCreate"
          >{{ editingKeyId ? t('system.apikey.btnSave') : t('system.apikey.btnCreate') }}</el-button
        >
      </template>
    </el-dialog>

    <el-dialog v-model="secretDialog.visible" :title="t('system.apikey.secretTitle')" width="560px">
      <div class="secret-tip">{{ t('system.apikey.secretTip') }}</div>
      <el-form label-width="90px">
        <el-form-item label="Key ID">
          <el-input :model-value="secretDialog.keyId" readonly />
        </el-form-item>
        <el-form-item :label="t('system.apikey.secretKey')">
          <div class="secret-row">
            <el-input :model-value="secretDialog.secret" readonly />
            <el-button type="primary" plain @click="copyText(secretDialog.secret)">{{ t('system.apikey.copy') }}</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="secretDialog.visible = false">{{ t('system.apikey.secretClose') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailDrawer.visible" :title="t('system.apikey.drawerTitle')" size="520px">
      <div v-if="detailDrawer.row" class="detail-wrap">
        <el-descriptions :column="1" border>
          <el-descriptions-item :label="t('system.apikey.descName')">{{ detailDrawer.row.displayName }}</el-descriptions-item>
          <el-descriptions-item label="Key ID">{{ detailDrawer.row.id }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descRole')">{{ permissionLabel(detailDrawer.row.role) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descStatus')">
            <el-tag :type="statusTagType(resolveStatus(detailDrawer.row))">
              {{ statusLabel(resolveStatus(detailDrawer.row)) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descOwner')">{{ ownerLabel(detailDrawer.row) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descCreator')">{{ creatorLabel(detailDrawer.row.createdBy) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descCreated')">{{ formatTime(detailDrawer.row.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descLastUsed')">{{ formatTime(detailDrawer.row.lastUsedAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descExpiry')">{{ formatTime(detailDrawer.row.expiresAt) }}</el-descriptions-item>
          <el-descriptions-item :label="t('system.apikey.descIpWhitelist')">{{ detailDrawer.row.ipWhitelist?.join(', ') || '-' }}</el-descriptions-item>
        </el-descriptions>

        <div class="stat-grid">
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('system.apikey.statCalls') }}</div>
            <div class="stat-value">{{ detailStats.callCount }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('system.apikey.statLastInvoke') }}</div>
            <div class="stat-value">{{ detailStats.lastInvoke }}</div>
          </el-card>
          <el-card shadow="never" class="stat-card">
            <div class="stat-label">{{ t('system.apikey.statTtl') }}</div>
            <div class="stat-value">{{ detailStats.ttl }}</div>
          </el-card>
        </div>

        <div class="record-head">{{ t('system.apikey.recordTitle') }}</div>
        <el-timeline v-if="detailRecords.length">
          <el-timeline-item
            v-for="item in detailRecords"
            :key="`${item.time}-${item.action}`"
            :timestamp="item.time"
            placement="top"
          >
            {{ item.action }}：{{ item.detail }}
          </el-timeline-item>
        </el-timeline>
        <el-empty v-else :description="t('system.apikey.recordEmpty')">
          <el-button plain @click="fetch">{{ t('system.apikey.recordRefresh') }}</el-button>
        </el-empty>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessageBox, type FormInstance, type FormRules } from 'element-plus';
import { listApiKeys, createApiKey, revokeApiKey, updateApiKey } from '@/api/system/apikey';
import { listUsers } from '@/api/system/user';
import type { ApiKey, ApiKeyPayload } from '@/types/system';
import { useSystemViewStore } from '@/store/systemView';
import { usePermissionStore } from '@/store/permission';
import { useI18n } from 'vue-i18n';
import { showAuthError } from '@/shared/errors/errorToast';
import { showSuccess, showWarning } from '@/shared/errors/messageToast';

const { t } = useI18n();

interface UserOption {
  id: string;
  label: string;
}

const permissionStore = usePermissionStore();
const systemView = useSystemViewStore();

const canManageApiKeys = computed(() => permissionStore.can('auth.apikey.manage'));

const tableRef = ref<any>();
const loading = ref(false);
const actionLoading = ref(false);

const allRows = ref<ApiKey[]>([]);
const selectedRows = ref<ApiKey[]>([]);

const keyword = ref(systemView.apikey.keyword);
const page = ref(systemView.apikey.page || 1);
const pageSize = ref(systemView.apikey.pageSize || 10);

const roleFilter = ref('');
const statusFilter = ref('');
const ownerFilter = ref('');

const userOptions = ref<UserOption[]>([]);

const capabilityOptions = [
  { value: 'pool.read', label: t('system.apikey.capPoolRead') },
  { value: 'pool.write', label: t('system.apikey.capPoolWrite') },
  { value: 'lease.read', label: t('system.apikey.capLeaseRead') },
  { value: 'lease.manage', label: t('system.apikey.capLeaseManage') },
  { value: 'report.read', label: t('system.apikey.capReportRead') },
  { value: 'audit.read', label: t('system.apikey.capAuditRead') },
  { value: 'security.policy.read', label: t('system.apikey.capSecPolicyRead') },
  { value: 'security.policy.manage', label: t('system.apikey.capSecPolicyManage') }
];

const createDialog = reactive({ visible: false });
const editingKeyId = ref('');
const formRef = ref<FormInstance>();
const form = reactive<ApiKeyPayload>({
  displayName: '',
  role: 'reader',
  capabilities: [],
  description: '',
  expiresAt: '',
  ownerUserId: ''
});
const ipWhitelistText = ref('');

const secretDialog = reactive({ visible: false, keyId: '', secret: '' });

const detailDrawer = reactive<{ visible: boolean; row: ApiKey | null }>({
  visible: false,
  row: null
});

const rules: FormRules = {
  displayName: [
    { required: true, message: t('system.apikey.valRequired'), trigger: 'blur' },
    { min: 2, max: 50, message: t('system.apikey.valLength'), trigger: 'blur' }
  ],
  role: [{ required: true, message: t('system.apikey.valRequired'), trigger: 'change' }],
  expiresAt: [
    {
      trigger: ['change', 'blur'],
      validator: (_: unknown, value: string, callback: (error?: Error) => void) => {
        if (!value) {
          callback();
          return;
        }
        const parsed = Date.parse(value);
        if (Number.isNaN(parsed)) {
          callback(new Error(t('system.apikey.valExpiryInvalid')));
          return;
        }
        if (parsed <= Date.now()) {
          callback(new Error(t('system.apikey.valExpiryFuture')));
          return;
        }
        callback();
      }
    }
  ]
};

const ownerOptions = computed(() => {
  const set = new Set<string>();
  allRows.value.forEach((row) => {
    const owner = ownerLabel(row);
    if (owner && owner !== '-') set.add(owner);
  });
  return Array.from(set.values()).sort((a, b) => a.localeCompare(b));
});

const filteredRows = computed(() => {
  let list = [...allRows.value];
  if (keyword.value.trim()) {
    const q = keyword.value.trim().toLowerCase();
    list = list.filter((row) => {
      const fields = [row.displayName, row.id, ownerLabel(row), creatorLabel(row.createdBy)]
        .map((item) => String(item || '').toLowerCase())
        .join(' ');
      return fields.includes(q);
    });
  }
  if (roleFilter.value) {
    list = list.filter((row) => String(row.role || '') === roleFilter.value);
  }
  if (statusFilter.value) {
    list = list.filter((row) => resolveStatus(row) === statusFilter.value);
  }
  if (ownerFilter.value) {
    list = list.filter((row) => ownerLabel(row) === ownerFilter.value);
  }
  return list;
});

const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

const detailStats = computed(() => {
  const row = detailDrawer.row;
  if (!row) {
    return { callCount: '-', lastInvoke: '-', ttl: '-' };
  }
  const lastInvoke = formatTime(row.lastUsedAt);
  return {
    callCount: row.lastUsedAt ? t('system.apikey.callsHas') : t('system.apikey.callsNone'),
    lastInvoke,
    ttl: ttlText(row)
  };
});

const detailRecords = computed(() => {
  const row = detailDrawer.row;
  if (!row) return [] as Array<{ time: string; action: string; detail: string }>;
  const records: Array<{ time: string; action: string; detail: string }> = [];
  if (row.createdAt) {
    records.push({ time: formatTime(row.createdAt), action: t('system.apikey.recordCreate'), detail: creatorLabel(row.createdBy) });
  }
  if (row.lastUsedAt) {
    records.push({ time: formatTime(row.lastUsedAt), action: t('system.apikey.recordLastCall'), detail: t('system.apikey.recordLastCallDetail') });
  }
  if (row.revokedAt) {
    records.push({ time: formatTime(row.revokedAt), action: t('system.apikey.recordDisable'), detail: t('system.apikey.recordDisableDetail') });
  }
  return records;
});

const fetchUsers = async () => {
  try {
    const { data } = await listUsers({ page: 1, pageSize: 200, keyword: '' });
    userOptions.value = (data.data.items || []).map((user) => ({
      id: user.id,
      label: `${user.username} / ${user.displayName || '-'}`
    }));
  } catch {
    userOptions.value = [];
  }
};

const fetch = async () => {
  if (!canManageApiKeys.value) return;
  loading.value = true;
  try {
    systemView.setApiKey({ page: page.value, pageSize: pageSize.value, keyword: keyword.value });
    const { data } = await listApiKeys(true);
    const rows = (((data as any)?.data || data) as ApiKey[]) || [];
    allRows.value = [...rows].sort((a, b) => {
      const ta = Date.parse(a.createdAt || '') || 0;
      const tb = Date.parse(b.createdAt || '') || 0;
      return tb - ta;
    });
    if ((page.value - 1) * pageSize.value >= filteredRows.value.length) {
      page.value = 1;
    }
  } catch (e) {
    showAuthError(e, t('system.apikey.loadFail'));
  } finally {
    loading.value = false;
  }
};

const onSearch = () => {
  page.value = 1;
  systemView.setApiKey({ page: page.value, pageSize: pageSize.value, keyword: keyword.value });
};

const onFilterChange = () => {
  page.value = 1;
  selectedRows.value = [];
};

const handlePageChange = (value: number) => {
  page.value = value;
  systemView.setApiKey({ page: value, pageSize: pageSize.value, keyword: keyword.value });
};

const handleSizeChange = (value: number) => {
  pageSize.value = value;
  page.value = 1;
  systemView.setApiKey({ page: 1, pageSize: value, keyword: keyword.value });
};

const onSelectionChange = (rows: ApiKey[]) => {
  selectedRows.value = rows;
};

const selectAllCurrentPage = () => {
  if (!tableRef.value) return;
  tableRef.value.clearSelection();
  pagedRows.value.forEach((row) => tableRef.value.toggleRowSelection(row, true));
};

const inverseSelectionCurrentPage = () => {
  if (!tableRef.value) return;
  const selectedIdSet = new Set(selectedRows.value.map((row) => row.id));
  tableRef.value.clearSelection();
  pagedRows.value.forEach((row) => {
    const shouldSelect = !selectedIdSet.has(row.id);
    tableRef.value.toggleRowSelection(row, shouldSelect);
  });
};

const revokeRows = async (rows: ApiKey[], successText: string) => {
  if (!rows.length) return;
  actionLoading.value = true;
  try {
    for (const row of rows) {
      await revokeApiKey(row.id);
    }
    showSuccess(successText);
    selectedRows.value = [];
    await fetch();
  } catch (e) {
    showAuthError(e, t('system.apikey.actionFail'));
  } finally {
    actionLoading.value = false;
  }
};

const batchDisable = async () => {
  const candidates = selectedRows.value.filter((row) => resolveStatus(row) === 'active');
  if (!candidates.length) {
    showWarning(t('system.apikey.batchDisableWarn'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('system.apikey.batchDisableConfirm', { count: candidates.length }), t('system.apikey.batchDisableTitle'), {
      type: 'warning'
    });
    await revokeRows(candidates, t('system.apikey.batchDisableOk'));
  } catch (e: any) {
    if (e === 'cancel') return;
  }
};

const batchEnable = async () => {
  const candidates = selectedRows.value.filter((row) => resolveStatus(row) === 'revoked');
  if (!candidates.length) {
    showWarning(t('system.apikey.batchEnableWarn'));
    return;
  }
  actionLoading.value = true;
  try {
    for (const row of candidates) {
      await updateApiKey(row.id, { enabled: true });
    }
    showSuccess(t('system.apikey.batchEnableOk'));
    selectedRows.value = [];
    await fetch();
  } catch (e) {
    showAuthError(e, t('system.apikey.batchEnableFail'));
  } finally {
    actionLoading.value = false;
  }
};

const batchDelete = async () => {
  const candidates = selectedRows.value.filter((row) => !isBuiltInKey(row));
  if (!candidates.length) {
    showWarning(t('system.apikey.batchDeleteWarn'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('system.apikey.batchDeleteConfirm', { count: candidates.length }), t('system.apikey.batchDeleteTitle'), {
      type: 'warning'
    });
    await revokeRows(candidates, t('system.apikey.batchDeleteOk'));
  } catch (e: any) {
    if (e === 'cancel') return;
  }
};

const revokeSingle = async (row: ApiKey, mode: 'disable' | 'delete') => {
  if (mode === 'delete' && isBuiltInKey(row)) return;
  await revokeRows([row], mode === 'disable' ? t('system.apikey.singleDisabled') : t('system.apikey.singleDeleted'));
};

const toggleKeyStatus = async (row: ApiKey) => {
  const status = resolveStatus(row);
  if (status === 'expired') {
    showWarning(t('system.apikey.expiredEditHint'));
    return;
  }
  actionLoading.value = true;
  try {
    if (status === 'revoked') {
      await updateApiKey(row.id, { enabled: true });
      showSuccess(t('system.apikey.singleEnabled'));
    } else {
      await updateApiKey(row.id, { enabled: false });
      showSuccess(t('system.apikey.singleDisabled'));
    }
    await fetch();
  } catch (e) {
    showAuthError(e, t('system.apikey.actionFail'));
  } finally {
    actionLoading.value = false;
  }
};

const isExpiryDateDisabled = (date: Date) => {
  const now = new Date();
  now.setHours(0, 0, 0, 0);
  return date.getTime() < now.getTime();
};

const normalizePrincipalToUsername = (value?: string) => {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const parts = raw.split(':').map((item) => item.trim()).filter(Boolean);
  if (parts.length >= 2) return parts[parts.length - 1];
  return raw;
};

const ownerLabel = (row: ApiKey) => {
  const ownerFromOwner = normalizePrincipalToUsername(row.ownerUserId);
  if (ownerFromOwner) return ownerFromOwner;
  const ownerFromCreator = normalizePrincipalToUsername(row.createdBy);
  if (ownerFromCreator) return ownerFromCreator;
  return '-';
};

const creatorLabel = (value?: string) => normalizePrincipalToUsername(value) || '-';

const permissionLabel = (role?: string) => {
  if (role === 'reader') return t('system.apikey.roleReader');
  if (role === 'admin') return t('system.apikey.roleAdmin');
  return role || '-';
};

const resolveStatus = (row: ApiKey) => {
  if (row.revokedAt) return 'revoked';
  if (row.status === 'revoked') return 'revoked';
  if (row.expiresAt) {
    const expiresMs = Date.parse(row.expiresAt);
    if (!Number.isNaN(expiresMs) && expiresMs <= Date.now()) return 'expired';
  }
  if (row.status === 'expired') return 'expired';
  return 'active';
};

const statusLabel = (status: string) => {
  if (status === 'active') return t('system.apikey.statusActive');
  if (status === 'expired') return t('system.apikey.statusExpired');
  if (status === 'revoked') return t('system.apikey.statusRevoked');
  return status;
};

const statusTagType = (status: string) => {
  if (status === 'active') return 'success';
  if (status === 'expired') return 'warning';
  if (status === 'revoked') return 'danger';
  return 'info';
};

const isExpiringSoon = (row: ApiKey) => {
  if (!row.expiresAt) return false;
  const expiresMs = Date.parse(row.expiresAt);
  if (Number.isNaN(expiresMs) || expiresMs <= Date.now()) return false;
  return expiresMs - Date.now() <= 72 * 60 * 60 * 1000;
};

const expiryClass = (row: ApiKey) => {
  const status = resolveStatus(row);
  if (status === 'expired') return 'is-expired';
  if (isExpiringSoon(row)) return 'is-warning';
  return '';
};

const ttlText = (row: ApiKey) => {
  if (!row.expiresAt) return t('system.apikey.ttlPermanent');
  const expiresMs = Date.parse(row.expiresAt);
  if (Number.isNaN(expiresMs)) return '-';
  const diff = expiresMs - Date.now();
  if (diff <= 0) return t('system.apikey.ttlExpired');
  const days = Math.floor(diff / (24 * 60 * 60 * 1000));
  const hours = Math.floor((diff % (24 * 60 * 60 * 1000)) / (60 * 60 * 1000));
  return t('system.apikey.ttlDays', { d: days, h: hours });
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const maskedKey = (row: ApiKey) => {
  const raw = String(row.token || row.id || '');
  if (!raw) return '-';
  if (raw.length <= 8) return `${raw.slice(0, 2)}****`;
  return `${raw.slice(0, 4)}****${raw.slice(-4)}`;
};

const isBuiltInKey = (row: ApiKey) => {
  const id = String(row.id || '').toLowerCase();
  return id.startsWith('builtin') || id.startsWith('system_') || id.includes('super_admin_default');
};

const copyText = async (value: string) => {
  const text = String(value || '').trim();
  if (!text) {
    showWarning(t('system.apikey.nothingToCopy'));
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    showSuccess(t('system.apikey.copied'));
  } catch {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.left = '-9999px';
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    document.execCommand('copy');
    document.body.removeChild(textarea);
    showSuccess(t('system.apikey.copied'));
  }
};

const copyKey = async (row: ApiKey) => {
  const raw = String(row.token || row.id || '').trim();
  await copyText(raw);
};

const resetCreateForm = () => {
  Object.assign(form, {
    displayName: '',
    role: 'reader',
    capabilities: [],
    description: '',
    expiresAt: '',
    ownerUserId: ''
  });
  ipWhitelistText.value = '';
  formRef.value?.clearValidate();
};

const openCreateDialog = () => {
  editingKeyId.value = '';
  resetCreateForm();
  createDialog.visible = true;
};

const openEditDialog = (row: ApiKey) => {
  editingKeyId.value = row.id;
  form.displayName = row.displayName || '';
  form.role = row.role || 'reader';
  form.capabilities = Array.isArray(row.capabilities) ? [...row.capabilities] : [];
  form.description = row.description || '';
  form.expiresAt = row.expiresAt || '';
  form.ownerUserId = row.ownerUserId || '';
  ipWhitelistText.value = Array.isArray(row.ipWhitelist) ? row.ipWhitelist.join('\n') : '';
  formRef.value?.clearValidate();
  createDialog.visible = true;
};

const ipWhitelistLines = () =>
  ipWhitelistText.value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

const submitCreate = () => {
  formRef.value?.validate(async (valid: boolean) => {
    if (!valid) return;
    actionLoading.value = true;
    try {
      const ips = ipWhitelistLines();
      const payload: ApiKeyPayload = {
        displayName: form.displayName,
        role: form.role,
        capabilities: form.capabilities,
        expiresAt: form.expiresAt,
        ownerUserId: form.ownerUserId,
        description: String(form.description || '').trim(),
        ipWhitelist: ips
      };

      if (editingKeyId.value) {
        await updateApiKey(editingKeyId.value, {
          displayName: payload.displayName,
          role: payload.role,
          capabilities: payload.capabilities || [],
          description: payload.description,
          expiresAt: payload.expiresAt || '',
          ipWhitelist: payload.ipWhitelist || []
        });
        createDialog.visible = false;
        showSuccess(t('system.apikey.updateOk'));
      } else {
        const { data } = await createApiKey(payload);
        const created = ((data as any)?.data || data) as ApiKey;
        createDialog.visible = false;
        if (created?.token) {
          secretDialog.keyId = created.id || '-';
          secretDialog.secret = created.token;
          secretDialog.visible = true;
        } else {
          showSuccess(t('system.apikey.createOk'));
        }
      }
      await fetch();
    } catch (e) {
      showAuthError(e, editingKeyId.value ? t('system.apikey.updateFail') : t('system.apikey.createFail'));
    } finally {
      actionLoading.value = false;
    }
  });
};

const openDetailDrawer = (row: ApiKey) => {
  detailDrawer.row = row;
  detailDrawer.visible = true;
};

onMounted(async () => {
  await Promise.all([fetch(), fetchUsers()]);
});
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

.module-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.module-title {
  font-weight: 600;
  font-size: 16px;
  line-height: 24px;
}

.module-subtitle {
  margin-top: 2px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.filter-bulk-row {
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.left-filters,
.right-bulk {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.data-table :deep(.el-table__cell .cell) {
  white-space: nowrap;
}

.data-table :deep(.action-column .cell) {
  white-space: normal;
  overflow: visible;
}

.action-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: nowrap;
}

.key-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.key-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

.expire-cell.is-warning {
  color: var(--el-color-warning);
  font-weight: 600;
}

.expire-cell.is-expired {
  color: var(--el-color-danger);
  font-weight: 600;
}

.pager-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.secret-tip {
  margin-bottom: 12px;
  color: var(--el-color-warning-dark-2);
}

.secret-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
}

.detail-wrap {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}

.stat-card {
  border-radius: 8px;
}

.stat-label {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-bottom: 6px;
}

.stat-value {
  font-weight: 600;
}

.record-head {
  font-weight: 600;
}

.w-140 {
  width: 140px;
}

.w-180 {
  width: 180px;
}

.w-220 {
  width: 220px;
}

.w-300 {
  width: 300px;
}

.w-420 {
  width: 420px;
}
</style>
