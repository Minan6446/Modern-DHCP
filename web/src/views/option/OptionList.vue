<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>{{ t('option.list.title') }}</h3>
      <p class="desc">{{ t('option.list.desc') }}</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-select v-model="category" clearable :placeholder="t('option.list.categoryPlaceholder')" style="width: 120px">
          <el-option :label="t('option.list.categoryStandard')" value="standard" />
          <el-option :label="t('option.list.categoryCustom')" value="custom" />
          <el-option :label="t('option.list.categoryVendor')" value="vendor" />
          <el-option :label="t('option.list.categoryTemplate')" value="template" />
        </el-select>
        <el-select v-model="status" clearable :placeholder="t('option.list.statusPlaceholder')" style="width: 120px">
          <el-option :label="t('option.list.statusPersisted')" value="persisted" />
          <el-option :label="t('option.list.statusDraft')" value="draft" />
        </el-select>
        <el-input
          v-model="keyword"
          :placeholder="t('option.list.searchPlaceholder')"
          clearable
          class="search"
        />
      </div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button type="primary" @click="openCreate">{{ t('option.list.addOption') }}</el-button>
      </div>
    </div>

    <el-card class="surface-card table-card content-card" v-loading="loading">

      <div class="selection-tools">
        <div class="selection-info">{{ t('option.list.selectionInfo', { sel: selectedIds.length, total: pagedRows.length }) }}</div>
        <div class="selection-actions">
          <el-button size="small" :disabled="!pagedRows.length" @click="selectAllCurrent">{{ t('option.list.selectAll') }}</el-button>
          <el-button size="small" :disabled="!pagedRows.length" @click="invertSelection">{{ t('option.list.invertSelection') }}</el-button>
          <el-button size="small" :disabled="!selectedIds.length" @click="clearSelection">{{ t('option.list.clearSelection') }}</el-button>
          <el-button size="small" type="danger" :disabled="!batchDeletable.length" :loading="deleting" @click="batchDelete">
            {{ t('option.list.batchDelete') }}
          </el-button>
        </div>
      </div>

      <el-empty v-if="!loading && !pagedRows.length" :description="t('option.list.emptyDesc')" :image-size="56" class="table-empty">
        <template #extra>
          <el-button size="small" @click="handleReset">{{ t('option.list.resetFilter') }}</el-button>
          <el-button size="small" type="primary" @click="openCreate">{{ t('option.list.createOption') }}</el-button>
        </template>
      </el-empty>

      <el-table
        v-else
        ref="tableRef"
        :data="pagedRows"
        stripe
        border
        class="table"
        row-key="id"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column prop="code" :label="t('option.list.colCode')" width="90" />
        <el-table-column prop="name" :label="t('option.list.colName')" min-width="150" />
        <el-table-column prop="description" :label="t('option.list.colDesc')" min-width="220" show-overflow-tooltip />
        <el-table-column prop="value" :label="t('option.list.colValue')" min-width="180" show-overflow-tooltip />
        <el-table-column prop="category" :label="t('option.list.colCategory')" width="110">
          <template #default="{ row }">
            <el-tag :type="tagType(row.category)" effect="light">{{ categoryLabel(row.category) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('option.list.colPersistStatus')" width="110">
          <template #default="{ row }">
            <el-tag :type="row.persisted ? 'success' : 'warning'" effect="light">{{ row.persisted ? t('option.list.statusPersisted') : t('option.list.statusDraft') }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('option.list.colRefCount')" width="190">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="goReference(row)">
              {{ t('option.list.refTemplate', { tpl: optionRefs(row.id).templateCount, scope: optionRefs(row.id).scopeCount }) }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column :label="t('option.list.colActions')" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="startEdit(row)">{{ t('option.list.edit') }}</el-button>
            <el-button
              size="small"
              type="danger"
              :disabled="optionRefs(row.id).total > 0"
              :loading="deletingId === row.id"
              @click="remove(row)"
            >
              {{ t('option.list.delete') }}
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
          :total="filtered.length"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>

      <el-dialog
        v-model="creating"
        :title="t('option.list.createDialogTitle')"
        width="600px"
        :close-on-click-modal="false"
        :close-on-press-escape="false"
        :before-close="handleCreateBeforeClose"
        class="standard-form-dialog"
      >
        <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="110px" class="standard-form">
          <el-form-item :label="t('option.list.formCode')" prop="code">
            <el-input-number v-model="createForm.code" :min="1" :max="254" :controls="false" @change="onCreateCodeChange" />
            <div class="field-hint">{{ t('option.list.formCodeHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.list.formName')" prop="name">
            <el-input v-model="createForm.name" :placeholder="t('option.list.formNamePlaceholder')" @input="onCreateNameInput" />
            <div class="field-hint">{{ t('option.list.formNameHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.list.formDesc')" prop="description">
            <el-input v-model="createForm.description" type="textarea" :autosize="false" />
          </el-form-item>
          <el-form-item :label="t('option.list.formValue')" prop="value">
            <el-input v-model="createForm.value" :placeholder="t('option.list.formValuePlaceholder')" @input="onCreateValueInput" />
            <div class="field-hint">{{ t('option.list.formValueHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.list.formCategory')" prop="category">
            <el-select v-model="createForm.category">
              <el-option :label="t('option.list.categoryStandard')" value="standard" />
              <el-option :label="t('option.list.categoryCustom')" value="custom" />
              <el-option :label="t('option.list.categoryVendor')" value="vendor" />
            </el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="handleCreateCancel">{{ t('option.list.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="handleCreate">{{ saving ? t('option.list.saving') : t('option.list.save') }}</el-button>
        </template>
      </el-dialog>

      <el-dialog
        v-model="editing"
        :title="t('option.list.editDialogTitle')"
        width="600px"
        :close-on-click-modal="false"
        :close-on-press-escape="false"
        :before-close="handleEditBeforeClose"
        class="standard-form-dialog"
      >
        <el-form :model="form" :rules="rules" ref="formRef" label-width="110px" class="standard-form">
          <el-form-item :label="t('option.list.formCode')" prop="code">
            <el-input-number v-model="form.code" :min="1" :max="254" :controls="false" @change="onEditCodeChange" />
            <div class="field-hint">{{ t('option.list.formCodeEditHint') }}</div>
          </el-form-item>
          <el-form-item :label="t('option.list.formName')" prop="name">
            <el-input v-model="form.name" @input="onEditNameInput" />
          </el-form-item>
          <el-form-item :label="t('option.list.formDesc')" prop="description">
            <el-input v-model="form.description" type="textarea" :autosize="false" />
          </el-form-item>
          <el-form-item :label="t('option.list.formValue')" prop="value">
            <el-input v-model="form.value" @input="onEditValueInput" />
          </el-form-item>
          <el-form-item :label="t('option.list.formCategory')" prop="category">
            <el-select v-model="form.category">
              <el-option :label="t('option.list.categoryStandard')" value="standard" />
              <el-option :label="t('option.list.categoryCustom')" value="custom" />
              <el-option :label="t('option.list.categoryVendor')" value="vendor" />
              <el-option :label="t('option.list.categoryTemplate')" value="template" />
            </el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="handleEditCancel">{{ t('option.list.cancel') }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveEdit">{{ saving ? t('option.list.saving') : t('option.list.save') }}</el-button>
        </template>
      </el-dialog>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, onMounted, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';
import type { FormInstance, FormRules } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { showSuccess, showWarning, showError } from '@/shared/errors/messageToast';
import { useOptionMemory, type DhcpOption, type OptionCategory } from '@/stores/optionMemory';

const { t } = useI18n();

const router = useRouter();
const {
  list,
  addOption,
  updateOption,
  removeOption,
  codeExists,
  load,
  optionReferenceCount
} = useOptionMemory();

onMounted(() => {
  void load();
});

const loading = ref(false);
const deleting = ref(false);
const saving = ref(false);
const deletingId = ref('');

const keyword = ref('');
const category = ref('');
const status = ref('');
const pagination = reactive({ page: 1, pageSize: 10 });

const creating = ref(false);
const editing = ref(false);
const currentId = ref('');
const createFormRef = ref<FormInstance>();
const formRef = ref<FormInstance>();
const tableRef = ref<any>();
const selectedIds = ref<string[]>([]);

const createForm = reactive<{ code: number; name: string; description: string; value: string; category: OptionCategory }>({
  code: 3,
  name: '',
  description: '',
  value: '',
  category: 'custom'
});

const form = reactive<{ code: number; name: string; description: string; value: string; category: OptionCategory }>({
  code: 0,
  name: '',
  description: '',
  value: '',
  category: 'custom'
});

const createSnapshot = ref('');
const editSnapshot = ref('');

const normalizeName = (value: string) => value.trim().toLowerCase();
const isNameDuplicate = (name: string, excludeId?: string) => {
  const normalized = normalizeName(name);
  if (!normalized) return false;
  return list.value.some((item) => normalizeName(item.name) === normalized && item.id !== excludeId);
};

const validateOptionValueFormat = (value: string) => {
  const text = value.trim();
  if (!text) return false;
  if (text.length > 255) return false;
  if (/^0x[0-9a-fA-F]+$/.test(text)) return true;
  if (/^\d+$/.test(text)) return true;
  if (/^[0-9a-fA-F:.\-_,\s]+$/.test(text)) return true;
  return !/[\u0000-\u001f]/.test(text);
};

const validateCreateCode = (_: unknown, value: number, callback: (err?: Error) => void) => {
  if (!value) return callback(new Error(t('option.list.required')));
  if (value < 1 || value > 254) return callback(new Error(t('option.list.codeRange')));
  if (codeExists(value)) return callback(new Error(t('option.list.codeExists')));
  callback();
};

const validateCreateName = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error(t('option.list.required')));
  if (isNameDuplicate(text)) return callback(new Error(t('option.list.nameExists')));
  callback();
};

const validateCreateValue = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error(t('option.list.required')));
  if (!validateOptionValueFormat(text)) return callback(new Error(t('option.list.valueInvalid')));
  callback();
};

const createRules: FormRules = {
  code: [{ validator: validateCreateCode, trigger: ['change', 'blur'] }],
  name: [{ validator: validateCreateName, trigger: ['input', 'blur'] }],
  value: [{ validator: validateCreateValue, trigger: ['input', 'blur'] }]
};

const validateEditCode = (_: unknown, value: number, callback: (err?: Error) => void) => {
  if (!value) return callback(new Error(t('option.list.required')));
  if (value < 1 || value > 254) return callback(new Error(t('option.list.codeRange')));
  const conflict = list.value.some((item) => item.code === value && item.id !== currentId.value);
  if (conflict) return callback(new Error(t('option.list.codeExists')));
  callback();
};

const validateEditName = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error(t('option.list.required')));
  if (isNameDuplicate(text, currentId.value)) return callback(new Error(t('option.list.nameExists')));
  callback();
};

const validateEditValue = (_: unknown, value: string, callback: (err?: Error) => void) => {
  const text = String(value || '').trim();
  if (!text) return callback(new Error(t('option.list.required')));
  if (!validateOptionValueFormat(text)) return callback(new Error(t('option.list.valueInvalid')));
  callback();
};

const rules: FormRules = {
  code: [{ validator: validateEditCode, trigger: ['change', 'blur'] }],
  name: [{ validator: validateEditName, trigger: ['input', 'blur'] }],
  value: [{ validator: validateEditValue, trigger: ['input', 'blur'] }]
};

const optionRefs = (id: string) => optionReferenceCount(id);

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return list.value.filter((item) => {
    const matchKw = kw
      ? [item.name, item.description, item.value, String(item.code)].some((field) =>
          String(field || '').toLowerCase().includes(kw)
        )
      : true;
    const matchCat = category.value ? item.category === category.value : true;
    const matchStatus =
      status.value === 'persisted'
        ? !!item.persisted
        : status.value === 'draft'
          ? !item.persisted
          : true;
    return matchKw && matchCat && matchStatus;
  });
});

const pagedRows = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize;
  return filtered.value.slice(start, start + pagination.pageSize);
});

const batchDeletable = computed(() =>
  selectedIds.value.filter((id) => optionRefs(id).total === 0)
);

watch([keyword, category, status], () => {
  pagination.page = 1;
});

const tagType = (cat: DhcpOption['category']) => {
  if (cat === 'standard') return 'success';
  if (cat === 'custom') return 'warning';
  if (cat === 'vendor') return 'info';
  return 'primary';
};

const categoryLabel = (cat: DhcpOption['category']) => {
  if (cat === 'standard') return t('option.list.categoryStandard');
  if (cat === 'custom') return t('option.list.categoryCustom');
  if (cat === 'vendor') return t('option.list.categoryVendor');
  return t('option.list.categoryTemplate');
};

const goReference = (row: DhcpOption) => {
  const refs = optionRefs(row.id);
  if (refs.templateCount > 0) {
    router.push('/option/templates');
    return;
  }
  if (refs.scopeCount > 0) {
    router.push('/option/scopes');
  }
};

const onPageChange = (page: number) => {
  pagination.page = page;
};

const onSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
};

const handleSelectionChange = (rows: DhcpOption[]) => {
  selectedIds.value = rows.map((item) => item.id);
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

const openCreate = () => {
  resetCreateForm();
  createSnapshot.value = JSON.stringify(createForm);
  creating.value = true;
};

const resetCreateForm = () => {
  createFormRef.value?.resetFields();
  createForm.code = Math.max(1, Math.floor(Math.random() * 200));
  createForm.name = '';
  createForm.description = '';
  createForm.value = '';
  createForm.category = 'custom';
};

const onCreateCodeChange = () => createFormRef.value?.validateField('code');
const onCreateNameInput = () => createFormRef.value?.validateField('name');
const onCreateValueInput = () => createFormRef.value?.validateField('value');
const onEditCodeChange = () => formRef.value?.validateField('code');
const onEditNameInput = () => formRef.value?.validateField('name');
const onEditValueInput = () => formRef.value?.validateField('value');

const handleCreateCancel = async () => {
  if (saving.value) return;
  const dirty = JSON.stringify(createForm) !== createSnapshot.value;
  if (!dirty) {
    creating.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.list.unsavedConfirm'), t('option.list.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.list.unsavedClose'),
      cancelButtonText: t('option.list.unsavedContinue')
    });
    creating.value = false;
  } catch {
    return;
  }
};

const handleCreateBeforeClose = async (done: () => void) => {
  if (saving.value) return;
  const dirty = JSON.stringify(createForm) !== createSnapshot.value;
  if (!dirty) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.list.unsavedConfirm'), t('option.list.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.list.unsavedClose'),
      cancelButtonText: t('option.list.unsavedContinue')
    });
    done();
  } catch {
    return;
  }
};

const handleEditCancel = async () => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== editSnapshot.value;
  if (!dirty) {
    editing.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.list.unsavedConfirm'), t('option.list.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.list.unsavedClose'),
      cancelButtonText: t('option.list.unsavedContinue')
    });
    editing.value = false;
  } catch {
    return;
  }
};

const handleEditBeforeClose = async (done: () => void) => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== editSnapshot.value;
  if (!dirty) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.list.unsavedConfirm'), t('option.list.unsavedTitle'), {
      type: 'warning',
      confirmButtonText: t('option.list.unsavedClose'),
      cancelButtonText: t('option.list.unsavedContinue')
    });
    done();
  } catch {
    return;
  }
};

const handleCreate = () => {
  createFormRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    void (async () => {
      saving.value = true;
      const saved = await addOption({ ...createForm, description: createForm.description.trim() });
      creating.value = false;
      saving.value = false;
      if (saved) {
        showSuccess(t('option.list.saved'));
      } else {
        showWarning(t('option.list.saveFail'));
      }
      resetCreateForm();
    })();
  });
};

const startEdit = (row: DhcpOption) => {
  currentId.value = row.id;
  Object.assign(form, row);
  editSnapshot.value = JSON.stringify(form);
  editing.value = true;
};

const saveEdit = () => {
  formRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    saving.value = true;
    updateOption(currentId.value, { ...form });
    saving.value = false;
    editing.value = false;
    showSuccess(t('option.list.saved'));
  });
};

const remove = async (row: DhcpOption) => {
  const refs = optionRefs(row.id);
  if (refs.total > 0) {
    showWarning(t('option.list.refWarning'));
    return;
  }
  try {
    await ElMessageBox.confirm(t('option.list.deleteConfirm', { name: row.name, code: row.code }), t('option.list.deleteTitle'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  deletingId.value = row.id;
  const result = await removeOption(row.id);
  deletingId.value = '';
  if (!result.ok) {
    showError(result.message || t('option.list.deleteFail'));
    return;
  }
  showSuccess(t('option.list.deleted'));
};

const batchDelete = async () => {
  if (!batchDeletable.value.length) return;
  try {
    await ElMessageBox.confirm(t('option.list.batchDeleteConfirm', { count: batchDeletable.value.length }), t('option.list.batchDeleteTitle'), {
      type: 'warning'
    });
  } catch {
    return;
  }
  deleting.value = true;
  const failed: string[] = [];
  for (const id of batchDeletable.value) {
    const result = await removeOption(id);
    if (!result.ok) failed.push(result.message || t('option.list.itemDeleteFail', { id }));
  }
  deleting.value = false;
  clearSelection();
  if (failed.length) {
    showWarning(t('option.list.batchDeleteFail', { count: failed.length, msg: failed[0] }));
    return;
  }
  showSuccess(t('option.list.batchDeleteSuccess'));
};

const handleReset = () => {
  keyword.value = '';
  category.value = '';
  status.value = '';
  pagination.page = 1;
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

.table-card {
  width: 100%;
}

.table-card :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.filter-bar :deep(.el-input__wrapper),
.filter-bar :deep(.el-select__wrapper) {
  min-height: 32px;
}

.filter-bar :deep(.el-button) {
  height: 32px;
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

.selection-info {
  color: var(--text-muted);
}

.table-empty {
  margin: 8px 0;
  border: 1px dashed var(--empty-border);
  border-radius: 10px;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--divider);
}
</style>