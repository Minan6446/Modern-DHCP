<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('option.scope.title') }}</h3>
      <p class="desc">{{ t('option.scope.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-segmented
          v-model="viewMode"
          :options="[
            { label: t('option.scope.viewCard'), value: 'card' },
            { label: t('option.scope.viewTable'), value: 'table' }
          ]"
        />
        <el-select v-model="statusFilter" clearable :placeholder="t('option.scope.statusPlaceholder')" style="width: 120px">
          <el-option :label="t('option.scope.statusActive')" value="active" />
          <el-option :label="t('option.scope.statusInactive')" value="inactive" />
        </el-select>
        <el-input v-model="keyword" clearable :placeholder="t('option.scope.searchPlaceholder')" class="search" />
      </div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button :loading="loading" @click="refresh">{{ t('option.scope.refresh') }}</el-button>
        <el-button type="primary" @click="startCreate">{{ t('option.scope.create') }}</el-button>
      </div>
    </div>

    <el-card class="surface-card table-card content-card" v-loading="loading">

      <template v-if="viewMode === 'card'">
        <el-empty v-if="!filtered.length" :description="t('option.scope.emptyDesc')" :image-size="56" class="table-empty">
          <template #extra>
            <el-button size="small" @click="resetFilters">{{ t('option.scope.resetFilter') }}</el-button>
            <el-button size="small" type="primary" @click="startCreate">{{ t('option.scope.create') }}</el-button>
          </template>
        </el-empty>
        <div v-else class="cards-grid">
          <el-card v-for="scope in pagedRows" :key="scope.id" class="scope-card" shadow="never">
            <div class="card-header">
              <div class="title-wrap">
                <div class="title">{{ scope.name }}</div>
                <div class="sub">{{ scope.subnet }}</div>
              </div>
              <el-tag :type="scope.status === 'active' ? 'success' : 'info'" effect="light">
                {{ scope.status === 'active' ? t('option.scope.statusActive') : t('option.scope.statusInactive') }}
              </el-tag>
            </div>
            <div class="meta-line"><span>{{ t('option.scope.cardRange') }}</span><b>{{ scope.range }}</b></div>
            <div class="meta-line"><span>{{ t('option.scope.cardGateway') }}</span><b>{{ scope.gateway }}</b></div>
            <div class="meta-line">
              <span>{{ t('option.scope.cardTemplate') }}</span>
              <el-link type="primary" :underline="false" @click="goTemplate(scope)">
                {{ boundTemplateName(scope) || t('option.scope.cardUnbound') }}
              </el-link>
            </div>
            <div class="meta-line"><span>{{ t('option.scope.cardOptions') }}</span><b>{{ scope.options.length }}</b></div>

            <div class="card-actions">
              <el-switch
                :model-value="scope.status === 'active'"
                @change="(value) => toggleStatus(scope, value ? 'active' : 'inactive')"
              />
              <div class="action-buttons">
                <el-button size="small" @click="openDetails(scope)">{{ t('option.scope.details') }}</el-button>
                <el-button size="small" type="primary" @click="startEdit(scope)">{{ t('option.scope.edit') }}</el-button>
                <el-button size="small" type="danger" @click="remove(scope)">{{ t('option.scope.delete') }}</el-button>
              </div>
            </div>
          </el-card>
        </div>
      </template>

      <template v-else>
        <div class="selection-tools">
          <div class="selection-info">{{ t('option.scope.selectionInfo', { sel: selectedIds.length, total: pagedRows.length }) }}</div>
          <div class="selection-actions">
            <el-button size="small" :disabled="!pagedRows.length" @click="selectAllCurrent">{{ t('option.scope.selectAll') }}</el-button>
            <el-button size="small" :disabled="!pagedRows.length" @click="invertSelection">{{ t('option.scope.invertSelection') }}</el-button>
            <el-button size="small" :disabled="!selectedIds.length" @click="clearSelection">{{ t('option.scope.clearSelection') }}</el-button>
            <el-button size="small" type="success" :disabled="!selectedIds.length" @click="batchSetStatus('active')">{{ t('option.scope.batchEnable') }}</el-button>
            <el-button size="small" type="warning" :disabled="!selectedIds.length" @click="batchSetStatus('inactive')">{{ t('option.scope.batchDisable') }}</el-button>
            <el-button size="small" type="danger" :disabled="!selectedIds.length" @click="batchDelete">{{ t('option.scope.batchDelete') }}</el-button>
          </div>
        </div>

        <el-empty v-if="!filtered.length" :description="t('option.scope.emptyDesc')" :image-size="56" class="table-empty">
          <template #extra>
            <el-button size="small" @click="resetFilters">{{ t('option.scope.resetFilter') }}</el-button>
          </template>
        </el-empty>

        <el-table
          v-else
          ref="tableRef"
          :data="pagedRows"
          stripe
          border
          row-key="id"
          class="table"
          @selection-change="onSelectionChange"
        >
          <el-table-column type="selection" width="46" />
          <el-table-column prop="name" :label="t('option.scope.colName')" min-width="160" />
          <el-table-column prop="subnet" :label="t('option.scope.colSubnet')" min-width="160" />
          <el-table-column prop="range" :label="t('option.scope.colRange')" min-width="220" />
          <el-table-column prop="gateway" :label="t('option.scope.colGateway')" min-width="140" />
          <el-table-column :label="t('option.scope.colStatus')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'" effect="light">
                {{ row.status === 'active' ? t('option.scope.statusActive') : t('option.scope.statusInactive') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('option.scope.colTemplate')" min-width="180">
            <template #default="{ row }">
              <el-link type="primary" :underline="false" @click="goTemplate(row)">
                {{ boundTemplateName(row) || t('option.scope.cardUnbound') }}
              </el-link>
            </template>
          </el-table-column>
          <el-table-column :label="t('option.scope.colOptions')" width="90">
            <template #default="{ row }">{{ row.options.length }}</template>
          </el-table-column>
          <el-table-column :label="t('option.scope.colActions')" width="320" fixed="right">
            <template #default="{ row }">
              <div class="table-actions-nowrap">
                <el-button size="small" @click="openDetails(row)">{{ t('option.scope.details') }}</el-button>
                <el-button
                  size="small"
                  :type="row.status === 'active' ? 'warning' : 'success'"
                  @click="toggleStatus(row, row.status === 'active' ? 'inactive' : 'active')"
                >
                  {{ row.status === 'active' ? t('option.scope.statusInactive') : t('option.scope.statusActive') }}
                </el-button>
                <el-button size="small" type="primary" @click="startEdit(row)">{{ t('option.scope.edit') }}</el-button>
                <el-button size="small" type="danger" @click="remove(row)">{{ t('option.scope.delete') }}</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </template>

      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          background
          layout="total, sizes, prev, pager, next, jumper"
          :page-sizes="[8, 16, 32, 64]"
          :total="filtered.length"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>

      <el-dialog
        v-model="dialogVisible"
        :title="dialogTitle"
        width="600px"
        :close-on-click-modal="false"
        :close-on-press-escape="false"
        :before-close="handleDialogBeforeClose"
        class="standard-form-dialog"
      >
        <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="standard-form">
          <el-form-item :label="t('option.scope.formName')" prop="name">
            <el-input v-model="form.name" @input="validateScopeField('name')" />
          </el-form-item>
          <el-form-item :label="t('option.scope.formSubnet')" prop="subnet">
            <el-input v-model="form.subnet" :placeholder="t('option.scope.formSubnetPlaceholder')" @input="validateScopeField('subnet')" />
            <div class="field-hint">{{ t('option.scope.formSubnetHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.scope.formRange')" prop="range">
            <el-input v-model="form.range" :placeholder="t('option.scope.formRangePlaceholder')" @input="validateScopeField('range')" />
            <div class="field-hint">{{ t('option.scope.formRangeHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.scope.formGateway')" prop="gateway">
            <el-input v-model="form.gateway" @input="validateScopeField('gateway')" />
            <div class="field-hint">{{ t('option.scope.formGatewayHint') }}</div>
          </el-form-item>

          <el-form-item :label="t('option.scope.formTemplate')">
            <el-select v-model="form.templateId" clearable filterable :placeholder="t('option.scope.formTemplatePlaceholder')" style="width: 100%" @change="onTemplateChange">
              <el-option v-for="tpl in templates" :key="tpl.id" :label="tpl.name" :value="tpl.id" />
            </el-select>
          </el-form-item>

          <el-form-item :label="t('option.scope.formOptions')" prop="options">
            <el-select
              v-model="form.options"
              multiple
              collapse-tags
              collapse-tags-tooltip
              filterable
              :placeholder="t('option.scope.formOptionsPlaceholder')"
              style="width: 100%"
            >
              <el-option
                v-for="option in availableOptions"
                :key="option.id"
                :label="`${option.code} - ${option.name}`"
                :value="option.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('option.scope.formNotes')" prop="notes"><el-input v-model="form.notes" type="textarea" :autosize="false" /></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="handleDialogCancel">{{ t('option.scope.cancel') }}</el-button>
          <el-button
            type="success"
            :disabled="!form.templateId"
            :loading="saving"
            @click="applyTemplateInDialog"
          >
            {{ t('option.scope.applyTemplate') }}
          </el-button>
          <el-button type="primary" :loading="saving" @click="save">{{ saving ? t('option.scope.saving') : t('option.scope.save') }}</el-button>
        </template>
      </el-dialog>

      <el-drawer v-model="detailVisible" :title="t('option.scope.drawerTitle')" size="42%">
        <template v-if="detailScope">
          <el-descriptions :column="1" border>
            <el-descriptions-item :label="t('option.scope.drawerName')">{{ detailScope.name }}</el-descriptions-item>
            <el-descriptions-item :label="t('option.scope.drawerSubnet')">{{ detailScope.subnet }}</el-descriptions-item>
            <el-descriptions-item :label="t('option.scope.drawerRange')">{{ detailScope.range }}</el-descriptions-item>
            <el-descriptions-item :label="t('option.scope.drawerGateway')">{{ detailScope.gateway }}</el-descriptions-item>
            <el-descriptions-item :label="t('option.scope.drawerStatus')">{{ detailScope.status === 'active' ? t('option.scope.statusActive') : t('option.scope.statusInactive') }}</el-descriptions-item>
            <el-descriptions-item :label="t('option.scope.drawerTemplate')">
              <el-link type="primary" :underline="false" @click="goTemplate(detailScope)">
                {{ boundTemplateName(detailScope) || t('option.scope.cardUnbound') }}
              </el-link>
            </el-descriptions-item>
          </el-descriptions>

          <div class="drawer-section">
            <div class="drawer-title">{{ t('option.scope.drawerOptionsTitle') }}</div>
            <el-table :data="detailOptions" size="small" border max-height="280">
              <el-table-column prop="code" :label="t('option.scope.drawerColCode')" width="80" />
              <el-table-column prop="name" :label="t('option.scope.drawerColName')" min-width="150" />
              <el-table-column prop="value" :label="t('option.scope.drawerColValue')" min-width="160" />
              <el-table-column :label="t('option.scope.drawerColJump')" width="90">
                <template #default>
                  <el-button size="small" link type="primary" @click="goOptionList">{{ t('option.scope.drawerView') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </template>
      </el-drawer>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, onMounted, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { showSuccess, showWarning, showError } from '@/shared/errors/messageToast';
import type { FormInstance, FormRules } from 'element-plus';
import { useOptionMemory, type DhcpScope, type DhcpOption } from '@/stores/optionMemory';
import { isIPv4, validateRangeWithin } from '@/utils/ip';

const { t } = useI18n();
const router = useRouter();
const {
  scopes,
  templates,
  list,
  addScope,
  addScopeSafe,
  updateScope,
  updateScopeSafe,
  removeScope,
  applyTemplateToScopes,
  load
} = useOptionMemory();

onMounted(() => {
  void load();
});

const loading = ref(false);
const saving = ref(false);
const viewMode = ref<'card' | 'table'>('card');
const statusFilter = ref<'active' | 'inactive' | ''>('');
const keyword = ref('');
const pagination = reactive({ page: 1, pageSize: 8 });
const selectedIds = ref<string[]>([]);
const tableRef = ref<any>();

const dialogVisible = ref(false);
const detailVisible = ref(false);
const dialogTitle = computed(() => (currentId.value ? t('option.scope.dialogEditTitle') : t('option.scope.dialogCreateTitle')));
const currentId = ref('');
const formRef = ref<FormInstance>();
const form = reactive({
  name: '',
  subnet: '',
  range: '',
  gateway: '',
  options: [] as string[],
  templateId: '',
  notes: ''
});
const formSnapshot = ref('');

const parseCidr = (cidr: string) => {
  const value = String(cidr || '').trim();
  const [network, prefixText] = value.split('/');
  const prefix = Number(prefixText);
  if (!isIPv4(network) || Number.isNaN(prefix) || prefix < 0 || prefix > 32) return null;
  return value;
};

const parseRange = (range: string) => {
  const match = String(range || '')
    .trim()
    .match(/^([0-9.]+)\s*-\s*([0-9.]+)$/);
  if (!match) return null;
  const start = match[1];
  const end = match[2];
  if (!isIPv4(start) || !isIPv4(end)) return null;
  const toInt = (ip: string) =>
    ip
      .split('.')
      .map((x) => Number(x))
      .reduce((sum, part) => (sum << 8) + part, 0);
  if (toInt(start) > toInt(end)) return null;
  return { start, end };
};

const validateLinkedSubnetRangeGateway = () => {
  const cidr = parseCidr(form.subnet);
  if (!cidr) return t('option.scope.subnetInvalid');
  const range = parseRange(form.range);
  if (!range) return t('option.scope.rangeInvalid');
  if (!validateRangeWithin(cidr, range.start) || !validateRangeWithin(cidr, range.end)) {
    return t('option.scope.rangeNotInSubnet');
  }
  if (!isIPv4(form.gateway) || !validateRangeWithin(cidr, form.gateway)) {
    return t('option.scope.gatewayNotInSubnet');
  }
  return '';
};
const detailScope = ref<DhcpScope | null>(null);

const availableOptions = computed(() => list.value);
const detailOptions = computed(() => {
  const ids = detailScope.value?.options || [];
  return list.value.filter((item) => ids.includes(item.id));
});

const rules: FormRules = {
  name: [{ required: true, message: t('option.scope.required'), trigger: ['blur', 'change'] }],
  subnet: [
    { required: true, message: t('option.scope.required'), trigger: ['blur', 'change'] },
    {
      validator: (_: unknown, _value: string, cb: (err?: Error) => void) => {
        const message = validateLinkedSubnetRangeGateway();
        if (message && message.includes(t('option.scope.subnetInvalid'))) return cb(new Error(message));
        cb();
      },
      trigger: ['blur', 'change']
    }
  ],
  range: [
    { required: true, message: t('option.scope.required'), trigger: ['blur', 'change'] },
    {
      validator: (_: unknown, _value: string, cb: (err?: Error) => void) => {
        const message = validateLinkedSubnetRangeGateway();
        if (message && (message === t('option.scope.rangeInvalid') || message === t('option.scope.rangeNotInSubnet'))) return cb(new Error(message));
        cb();
      },
      trigger: ['blur', 'change']
    }
  ],
  gateway: [
    { required: true, message: t('option.scope.required'), trigger: ['blur', 'change'] },
    {
      validator: (_: unknown, _value: string, cb: (err?: Error) => void) => {
        const message = validateLinkedSubnetRangeGateway();
        if (message && message === t('option.scope.gatewayNotInSubnet')) return cb(new Error(message));
        cb();
      },
      trigger: ['blur', 'change']
    }
  ]
};

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return scopes.value.filter((scope) => {
    const matchStatus = statusFilter.value ? scope.status === statusFilter.value : true;
    const matchKw = kw
      ? [scope.name, scope.subnet, scope.range, scope.gateway, scope.notes || '']
          .join(' ')
          .toLowerCase()
          .includes(kw)
      : true;
    return matchStatus && matchKw;
  });
});

const pagedRows = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize;
  return filtered.value.slice(start, start + pagination.pageSize);
});

watch([statusFilter, keyword, viewMode], () => {
  pagination.page = 1;
});

const boundTemplateName = (scope: DhcpScope) => {
  if (!scope.templateId) return '';
  return templates.value.find((item) => item.id === scope.templateId)?.name || '';
};

const resetForm = () => {
  formRef.value?.resetFields();
  Object.assign(form, {
    name: '',
    subnet: '',
    range: '',
    gateway: '',
    options: [],
    templateId: '',
    notes: ''
  });
};

const startCreate = () => {
  currentId.value = '';
  resetForm();
  formSnapshot.value = JSON.stringify(form);
  dialogVisible.value = true;
};

const startEdit = (scope: DhcpScope) => {
  currentId.value = scope.id;
  Object.assign(form, {
    name: scope.name,
    subnet: scope.subnet,
    range: scope.range,
    gateway: scope.gateway,
    options: [...scope.options],
    templateId: scope.templateId || '',
    notes: scope.notes || ''
  });
  formSnapshot.value = JSON.stringify(form);
  dialogVisible.value = true;
};

const validateScopeField = (field: 'name' | 'subnet' | 'range' | 'gateway') => {
  formRef.value
    ?.validateField(field)
    // Element Plus rejects with field-error objects on invalid input; this is expected during typing.
    .catch(() => undefined);
};

const onTemplateChange = (templateId: string) => {
  if (!templateId) return;
  const template = templates.value.find((item) => item.id === templateId);
  if (!template) return;
  const optionIds = template.options.map((item) => item.optionId);
  form.options = Array.from(new Set([...form.options, ...optionIds]));
  if (!form.notes) {
    form.notes = t('option.scope.templateAutoFill', { name: template.name });
  }
};

const handleDialogCancel = async () => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== formSnapshot.value;
  if (!dirty) {
    dialogVisible.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.scope.unsavedConfirm'), t('option.scope.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.scope.unsavedClose'),
      cancelButtonText: t('option.scope.unsavedContinue')
    });
    dialogVisible.value = false;
  } catch {
    return;
  }
};

const handleDialogBeforeClose = async (done: () => void) => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== formSnapshot.value;
  if (!dirty) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.scope.unsavedConfirm'), t('option.scope.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.scope.unsavedClose'),
      cancelButtonText: t('option.scope.unsavedContinue')
    });
    done();
  } catch {
    return;
  }
};

const save = () => {
  formRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    void (async () => {
      saving.value = true;
      if (currentId.value) {
        const result = await updateScopeSafe(currentId.value, { ...form, options: [...form.options] });
        if (!result.ok) {
          showError(result.message || t('option.scope.saveFail'));
          saving.value = false;
          return;
        }
        showSuccess(t('option.scope.updated'));
      } else {
        const createdResult = await addScopeSafe({ ...form, options: [...form.options], status: 'active' });
        if (!createdResult.ok || !createdResult.scope) {
          showError(createdResult.message || t('option.scope.createFail'));
          saving.value = false;
          return;
        }
        const created = createdResult.scope;
        if (form.templateId) {
          applyTemplateToScopes(form.templateId, [created.id], 'merge', 'template_wins');
          const latest = scopes.value.find((item) => item.id === created.id);
          const nextOptions = [...(latest?.options || form.options)];
          const syncResult = await updateScopeSafe(created.id, {
            templateId: form.templateId,
            options: nextOptions
          });
          if (!syncResult.ok) {
            showWarning(t('option.scope.templateBindFail', { msg: syncResult.message || '' }));
            saving.value = false;
            dialogVisible.value = false;
            return;
          }
        }
        showSuccess(t('option.scope.created'));
      }
      saving.value = false;
      dialogVisible.value = false;
    })();
  });
};

const applyTemplateInDialog = () => {
  if (!form.templateId) return;
  if (currentId.value) {
    void (async () => {
      applyTemplateToScopes(form.templateId, [currentId.value], 'merge', 'template_wins');
      const latest = scopes.value.find((item) => item.id === currentId.value);
      const nextOptions = [...(latest?.options || form.options)];
      const result = await updateScopeSafe(currentId.value, {
        templateId: form.templateId,
        options: nextOptions
      });
      if (!result.ok) {
        showError(result.message || t('option.scope.templateApplyFail'));
        return;
      }
      form.options = nextOptions;
      showSuccess(t('option.scope.templateApplied'));
    })();
    return;
  }
  showWarning(t('option.scope.templateApplyHint'));
};

const openDetails = (scope: DhcpScope) => {
  detailScope.value = scope;
  detailVisible.value = true;
};

const toggleStatus = (scope: DhcpScope, status: DhcpScope['status']) => {
  void (async () => {
    const result = await updateScopeSafe(scope.id, { status });
    if (!result.ok) {
      showError(result.message || t('option.scope.statusUpdateFail'));
      return;
    }
    showSuccess(status === 'active' ? t('option.scope.enabled') : t('option.scope.disabled'));
  })();
};

const remove = async (scope: DhcpScope) => {
  try {
    await ElMessageBox.confirm(t('option.scope.deleteConfirm', { name: scope.name }), t('option.scope.deleteTitle'), { type: 'warning' });
  } catch {
    return;
  }
  const result = await removeScope(scope.id);
  if (!result.ok) {
    showError(result.message || t('option.scope.deleteFail'));
    return;
  }
  showSuccess(t('option.scope.deleted'));
};

const onSelectionChange = (rows: DhcpScope[]) => {
  selectedIds.value = rows.map((row) => row.id);
};

const selectAllCurrent = () => {
  const table = tableRef.value;
  if (!table) return;
  table.clearSelection();
  pagedRows.value.forEach((row) => table.toggleRowSelection(row, true));
};

const invertSelection = () => {
  const table = tableRef.value;
  if (!table) return;
  const selectedSet = new Set(selectedIds.value);
  pagedRows.value.forEach((row) => table.toggleRowSelection(row, !selectedSet.has(row.id)));
};

const clearSelection = () => {
  tableRef.value?.clearSelection();
  selectedIds.value = [];
};

const batchSetStatus = (status: DhcpScope['status']) => {
  void (async () => {
    const failed = [] as string[];
    for (const id of selectedIds.value) {
      const result = await updateScopeSafe(id, { status });
      if (!result.ok) failed.push(result.message || t('option.scope.itemUpdateFail', { id }));
    }
    if (failed.length) {
      showWarning(t('option.scope.batchStatusFail', { count: failed.length, msg: failed[0] }));
      return;
    }
    showSuccess(status === 'active' ? t('option.scope.batchEnableSuccess') : t('option.scope.batchDisableSuccess'));
  })();
};

const batchDelete = async () => {
  if (!selectedIds.value.length) return;
  try {
    await ElMessageBox.confirm(t('option.scope.batchDeleteConfirm', { count: selectedIds.value.length }), t('option.scope.batchDeleteTitle'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  const failed = [] as string[];
  for (const id of selectedIds.value) {
    const result = await removeScope(id);
    if (!result.ok) failed.push(result.message || t('option.scope.itemDeleteFail', { id }));
  }
  clearSelection();
  if (failed.length) {
    showWarning(t('option.scope.batchDeleteFail', { count: failed.length, msg: failed[0] }));
    return;
  }
  showSuccess(t('option.scope.batchDeleteSuccess'));
};

const resetFilters = () => {
  statusFilter.value = '';
  keyword.value = '';
  pagination.page = 1;
};

const refresh = async () => {
  loading.value = true;
  await load();
  loading.value = false;
};

const onPageChange = (page: number) => {
  pagination.page = page;
};

const onSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
};

const goTemplate = (scope: DhcpScope) => {
  if (!scope.templateId) {
    router.push('/option/templates');
    return;
  }
  router.push('/option/templates');
};

const goOptionList = () => {
  router.push('/option/list');
};
</script>

<style scoped>
.standard-form-dialog :deep(.el-dialog) {
  border-radius: 8px;
  min-height: 680px;
}

.standard-form-dialog :deep(.el-dialog__header) {
  padding: 32px 32px 0;
}

.standard-form-dialog :deep(.el-dialog__body) {
  padding: 32px;
}

.standard-form-dialog :deep(.el-dialog__footer) {
  padding: 0 32px 32px;
}

.standard-form :deep(.el-form-item) {
  margin-bottom: 18px;
}

.standard-form :deep(.el-form-item__label) {
  width: 110px !important;
  justify-content: flex-end;
  padding-right: 12px;
  white-space: nowrap;
}

.standard-form :deep(.el-form-item__content) {
  margin-left: 0 !important;
}

.standard-form :deep(.el-input),
.standard-form :deep(.el-select),
.standard-form :deep(.el-input-number),
.standard-form :deep(.el-textarea) {
  width: 440px;
}

.standard-form :deep(.el-input__wrapper),
.standard-form :deep(.el-select__wrapper),
.standard-form :deep(.el-input-number),
.standard-form :deep(.el-textarea__inner) {
  min-height: 36px;
  border-radius: 4px;
}

.standard-form :deep(.el-textarea__inner) {
  height: 80px;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-muted: #4b5563;
  --subtle-bg: #f9fafb;
  --empty-border: #d1d5db;
  --divider: #f0f2f5;
  padding: 12px;
  background: var(--page-bg);
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  padding: 12px 16px;
  border-radius: 4px;
  background: var(--surface-bg);
}

.table-card :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.page-header h3 {
  margin: 0;
}

.filter-bar {
  height: 48px;
  padding: 8px 16px;
  border-radius: 4px;
  background: var(--surface-bg);
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

.bar-middle {
  min-width: 24px;
}

.desc {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.search {
  width: 240px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button),
.filter-bar :deep(.el-segmented) {
  height: 32px;
}

.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}

.scope-card {
  border-radius: 10px;
  min-height: 232px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 8px;
}

.title {
  font-size: 16px;
  font-weight: 700;
}

.sub {
  color: var(--text-secondary);
  font-size: 12px;
}

.meta-line {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 8px 0;
  color: var(--text-muted);
}

.meta-line b {
  color: var(--text-primary);
  margin-left: 8px;
  font-weight: 600;
}

.card-actions {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.selection-tools {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border: 1px solid var(--surface-border);
  border-radius: 8px;
  background: var(--subtle-bg);
}

.selection-actions {
  display: flex;
  gap: 8px;
}

.table-actions-nowrap {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  white-space: nowrap;
  gap: 6px;
}

.table-actions-nowrap :deep(.el-button) {
  margin-left: 0;
}

.table-empty {
  border: 1px dashed var(--empty-border);
  border-radius: 10px;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--divider);
}

.drawer-section {
  margin-top: 14px;
}

.drawer-title {
  font-weight: 600;
  margin-bottom: 8px;
}
</style>