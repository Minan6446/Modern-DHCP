<template>
  <div class="page-wrap">
    <div class="page-header surface-card">
      <h3>配置模板</h3>
      <p class="desc">模板化 DHCP 选项，支持跨作用域同步与引用关系校验</p>
    </div>

    <div class="filter-bar surface-card">
      <div class="bar-left">
        <el-input v-model="keyword" clearable placeholder="模板名称 / 描述" class="search" />
        <el-select v-model="refFilter" clearable placeholder="引用状态" style="width: 130px">
          <el-option label="未引用" value="unused" />
          <el-option label="已引用" value="used" />
        </el-select>
      </div>
      <div class="bar-middle"></div>
      <div class="bar-right">
        <el-button :loading="loading" @click="refresh">刷新</el-button>
        <el-button type="success" :disabled="!selectedTemplate" :loading="syncing" @click="syncTemplate">
          同步模板
        </el-button>
        <el-button type="primary" @click="openCreate">新建模板</el-button>
      </div>
    </div>

    <el-card class="surface-card table-card content-card" v-loading="loading">

      <div class="selection-tools">
        <div class="selection-info">已选 {{ selectedIds.length }} / 当前页 {{ pagedRows.length }}</div>
        <div class="selection-actions">
          <el-button size="small" :disabled="!pagedRows.length" @click="selectAllCurrent">全选当前页</el-button>
          <el-button size="small" :disabled="!pagedRows.length" @click="invertSelection">反选</el-button>
          <el-button size="small" :disabled="!selectedIds.length" @click="clearSelection">清空</el-button>
          <el-button size="small" type="danger" :disabled="!batchDeletable.length" :loading="deleting" @click="batchDelete">
            批量删除
          </el-button>
        </div>
      </div>

      <el-empty v-if="!filtered.length" description="暂无配置模板" :image-size="56" class="table-empty">
        <template #extra>
          <el-button size="small" @click="resetFilters">重置筛选</el-button>
          <el-button size="small" type="primary" @click="openCreate">新建模板</el-button>
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
        @current-change="onCurrentChange"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column type="expand">
          <template #default="{ row }">
            <el-table :data="row.options" size="small" border class="inner-table">
              <el-table-column prop="code" label="代码" width="90" />
              <el-table-column prop="name" label="名称" min-width="150" />
              <el-table-column prop="value" label="默认值" min-width="180" />
              <el-table-column prop="description" label="描述" min-width="180" show-overflow-tooltip />
            </el-table>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="模板名称" min-width="180">
          <template #default="{ row }">
            <div class="name-cell">
              <span class="icon" v-if="row.icon">{{ row.icon }}</span>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
        <el-table-column label="选项数" width="90">
          <template #default="{ row }">{{ row.options.length }}</template>
        </el-table-column>
        <el-table-column label="引用作用域数" width="130">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="goScopes(row)">
              {{ templateUsage(row.id).count }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="最后修改" width="170">
          <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="290" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="success" :loading="syncingId === row.id" @click="syncTemplate(row)">同步</el-button>
            <el-button size="small" @click="openApply(row)">应用</el-button>
            <el-button
              size="small"
              type="danger"
              :disabled="templateUsage(row.id).count > 0"
              @click="handleDelete(row)"
            >
              删除
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
        :before-close="handleTemplateBeforeClose"
        class="standard-form-dialog"
      >
        <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="standard-form">
          <el-form-item label="模板名称" prop="name">
            <el-input v-model="form.name" @input="onTemplateNameInput" />
            <div class="field-hint">模板名称需唯一。</div>
          </el-form-item>
          <el-form-item label="模板描述">
            <el-input v-model="form.description" type="textarea" :autosize="false" />
          </el-form-item>
          <el-form-item label="模板图标">
            <div class="icon-selector-wrap">
              <el-select
                v-model="form.icon"
                filterable
                allow-create
                clearable
                default-first-option
                :filter-method="handleIconFilter"
                placeholder="请选择模板图标"
                @change="onIconChange"
              >
                <template #header>
                  <el-input v-model="iconSearchKeyword" placeholder="搜索图标名称" class="icon-search-input" clearable />
                </template>
                <el-option
                  v-for="item in filteredIconOptions"
                  :key="item.key"
                  :label="item.label"
                  :value="item.value"
                  :disabled="item.disabled"
                >
                  <div class="icon-option" :class="{ 'icon-option-category': item.disabled }">
                    <span v-if="!item.disabled" class="icon-symbol">{{ item.symbol }}</span>
                    <span class="icon-name">{{ item.name }}</span>
                  </div>
                </el-option>
                <el-option label="清空选择" value="__clear__">
                  <div class="icon-option icon-option-clear">
                    <span class="icon-name">清空选择</span>
                  </div>
                </el-option>
              </el-select>
              <div v-if="form.icon" class="icon-preview">当前预览：{{ selectedIconDisplay }}</div>
              <div class="field-hint">可选：选择模板标识图标</div>
            </div>
          </el-form-item>

          <el-form-item label="包含选项" prop="selectedOptionIds">
            <div class="option-import-wrap">
              <el-select
                v-model="form.selectedOptionIds"
                multiple
                filterable
                collapse-tags
                collapse-tags-tooltip
                class="option-import-select"
                placeholder="选择模板包含的 DHCP 选项"
                @change="syncOptionValues"
              >
                <el-option
                  v-for="opt in list"
                  :key="opt.id"
                  :label="`${opt.code} - ${opt.name}`"
                  :value="opt.id"
                />
              </el-select>
              <el-button size="small" @click="optionPickerVisible = true">从选项列表批量导入</el-button>
            </div>
          </el-form-item>

          <el-form-item label="选项默认值">
            <el-table :data="selectedOptions" size="small" border max-height="240">
              <el-table-column prop="code" label="代码" width="80" />
              <el-table-column prop="name" label="名称" width="180" />
              <el-table-column label="默认值">
                <template #default="{ row }">
                  <el-input v-model="form.optionValues[row.id]" />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80">
                <template #default="{ row }">
                  <el-button link type="danger" @click="removeSelectedOption(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="handleTemplateCancel">取消</el-button>
          <el-button :disabled="saving" @click="openTemplatePreview">配置预览</el-button>
          <el-button type="primary" :loading="saving" @click="saveTemplate">{{ saving ? '保存中...' : '保存模板' }}</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="previewVisible" title="配置预览" width="600px">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="模板名称">{{ form.name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="模板描述">{{ form.description || '-' }}</el-descriptions-item>
          <el-descriptions-item label="模板图标">{{ form.icon || '-' }}</el-descriptions-item>
          <el-descriptions-item label="选项列表">
            {{ selectedOptions.map((item) => `${item.code}-${item.name}:${form.optionValues[item.id] ?? item.value}`).join('；') || '-' }}
          </el-descriptions-item>
        </el-descriptions>
      </el-dialog>

      <el-dialog v-model="optionPickerVisible" title="从选项列表批量导入" width="760px">
        <el-input v-model="optionPickerKeyword" size="small" clearable placeholder="搜索代码/名称/描述" class="mb-8" />
        <el-table ref="optionPickerTableRef" :data="pickerRows" row-key="id" border stripe @selection-change="onPickerSelection">
          <el-table-column type="selection" width="46" />
          <el-table-column prop="code" label="代码" width="90" />
          <el-table-column prop="name" label="名称" min-width="150" />
          <el-table-column prop="description" label="描述" min-width="180" />
          <el-table-column prop="value" label="值" min-width="180" />
        </el-table>
        <template #footer>
          <el-button @click="optionPickerVisible = false">取消</el-button>
          <el-button type="primary" @click="importPickedOptions">导入选中项</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="applyVisible" title="应用配置模板" width="620px">
        <el-form :model="applyForm" label-width="120px">
          <el-form-item label="模板">
            <el-select v-model="applyForm.templateId" style="width: 100%" placeholder="请选择模板">
              <el-option v-for="tpl in templates" :key="tpl.id" :label="tpl.name" :value="tpl.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="目标作用域">
            <el-select v-model="applyForm.scopeIds" multiple filterable collapse-tags style="width: 100%">
              <el-option v-for="scope in scopes" :key="scope.id" :label="scope.name" :value="scope.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="应用模式">
            <el-radio-group v-model="applyForm.mode">
              <el-radio label="overwrite">覆盖</el-radio>
              <el-radio label="merge">合并</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="applyForm.mode === 'merge'" label="冲突策略">
            <el-radio-group v-model="applyForm.conflict">
              <el-radio label="template_wins">模板优先</el-radio>
              <el-radio label="keep_scope">保留作用域</el-radio>
            </el-radio-group>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="applyVisible = false">取消</el-button>
          <el-button type="primary" @click="submitApply">执行应用</el-button>
        </template>
      </el-dialog>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';
import type { FormInstance, FormRules } from 'element-plus';
import { showSuccess, showWarning, showError } from '@/shared/errors/messageToast';
import {
  useOptionMemory,
  type DhcpOption,
  type DhcpTemplate,
  type DhcpTemplateOption
} from '@/stores/optionMemory';

const router = useRouter();
const {
  load,
  list,
  scopes,
  templates,
  addTemplateSafe,
  updateTemplateSafe,
  removeTemplates,
  applyTemplate,
  applyTemplateToScopes,
  applyTemplateToGlobal,
  updateScopeSafe,
  templateUsage,
  syncTemplateToReferencedScopes
} = useOptionMemory();

onMounted(() => {
  void load();
});

const loading = ref(false);
const saving = ref(false);
const deleting = ref(false);
const syncing = ref(false);
const syncingId = ref('');

const keyword = ref('');
const refFilter = ref<'used' | 'unused' | ''>('');
const pagination = reactive({ page: 1, pageSize: 8 });

const selectedIds = ref<string[]>([]);
const selectedTemplate = ref<DhcpTemplate | null>(null);
const tableRef = ref<any>();

const dialogVisible = ref(false);
const applyVisible = ref(false);
const optionPickerVisible = ref(false);
const optionPickerKeyword = ref('');
const optionPickerSelected = ref<string[]>([]);
const editingTemplate = ref<DhcpTemplate | null>(null);
const previewVisible = ref(false);
const iconSearchKeyword = ref('');

const formRef = ref<FormInstance>();
const form = reactive({
  name: '',
  description: '',
  icon: '',
  selectedOptionIds: [] as string[],
  optionValues: {} as Record<string, string>
});
const formSnapshot = ref('');

const applyForm = reactive({
  templateId: '',
  scopeIds: [] as string[],
  mode: 'merge' as 'merge' | 'overwrite',
  conflict: 'template_wins' as 'template_wins' | 'keep_scope',
  applyGlobal: false
});

const dialogTitle = computed(() => (editingTemplate.value ? '编辑模板' : '新建模板'));

const iconLibrary = [
  { category: '网络', name: '网络节点', symbol: '🌐' },
  { category: '网络', name: '路由器', symbol: '📡' },
  { category: '网络', name: '交换机', symbol: '🔀' },
  { category: '办公', name: '办公楼', symbol: '🏢' },
  { category: '办公', name: '团队', symbol: '👥' },
  { category: '办公', name: '会议', symbol: '🗂️' },
  { category: '设备', name: '服务器', symbol: '🖥️' },
  { category: '设备', name: '终端', symbol: '💻' },
  { category: '设备', name: 'IoT设备', symbol: '📱' },
  { category: '运维', name: '告警', symbol: '🚨' },
  { category: '运维', name: '监控', symbol: '📈' },
  { category: '运维', name: '工具', symbol: '🛠️' }
];

const filteredIconOptions = computed(() => {
  const keyword = iconSearchKeyword.value.trim().toLowerCase();
  const groups = new Map<string, Array<{ category: string; name: string; symbol: string }>>();
  iconLibrary.forEach((item) => {
    const match = !keyword || item.name.toLowerCase().includes(keyword) || item.category.toLowerCase().includes(keyword);
    if (!match) return;
    if (!groups.has(item.category)) groups.set(item.category, []);
    groups.get(item.category)?.push(item);
  });
  const result: Array<{ key: string; label: string; value: string; disabled: boolean; name: string; symbol: string }> = [];
  groups.forEach((items, category) => {
    result.push({
      key: `cat-${category}`,
      label: category,
      value: `cat-${category}`,
      disabled: true,
      name: `${category}图标`,
      symbol: ''
    });
    items.forEach((item) => {
      result.push({
        key: `${category}-${item.name}`,
        label: `${item.symbol} ${item.name}`,
        value: item.symbol,
        disabled: false,
        name: item.name,
        symbol: item.symbol
      });
    });
  });
  return result;
});

const selectedIconDisplay = computed(() => {
  const current = String(form.icon || '').trim();
  if (!current) return '';
  const matched = iconLibrary.find((item) => item.symbol === current);
  if (matched) return `${matched.symbol} ${matched.name}`;
  return `${current} 自定义图标`;
});

const handleIconFilter = (value: string) => {
  iconSearchKeyword.value = value;
};

const onIconChange = (value: string) => {
  if (value === '__clear__') {
    form.icon = '';
  }
};

const nameExists = (name: string, excludeId?: string) => {
  const value = String(name || '').trim().toLowerCase();
  if (!value) return false;
  return templates.value.some((item) => item.name.trim().toLowerCase() === value && item.id !== excludeId);
};

const selectedOptions = computed<DhcpOption[]>(() =>
  form.selectedOptionIds
    .map((id) => list.value.find((opt) => opt.id === id))
    .filter((item): item is DhcpOption => !!item)
);

const pickerRows = computed(() => {
  const kw = optionPickerKeyword.value.trim().toLowerCase();
  if (!kw) return list.value;
  return list.value.filter((item) =>
    [item.name, item.description, item.value, String(item.code)].join(' ').toLowerCase().includes(kw)
  );
});

const rules: FormRules = {
  name: [
    { required: true, message: '模板名称必填', trigger: ['blur', 'change'] },
    {
      validator: (_: unknown, value: string, callback: (err?: Error) => void) => {
        if (nameExists(value, editingTemplate.value?.id)) {
          callback(new Error('模板名称已存在'));
          return;
        }
        callback();
      },
      trigger: ['blur', 'change']
    }
  ],
  selectedOptionIds: [{ type: 'array', required: true, message: '至少选择一个选项', trigger: 'change' }]
};

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return templates.value.filter((tpl) => {
    const matchKw = kw ? [tpl.name, tpl.description].join(' ').toLowerCase().includes(kw) : true;
    const usage = templateUsage(tpl.id).count;
    const matchRef = refFilter.value === 'used' ? usage > 0 : refFilter.value === 'unused' ? usage === 0 : true;
    return matchKw && matchRef;
  });
});

const pagedRows = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize;
  return filtered.value.slice(start, start + pagination.pageSize);
});

const batchDeletable = computed(() =>
  selectedIds.value.filter((id) => templateUsage(id).count === 0)
);

watch([keyword, refFilter], () => {
  pagination.page = 1;
});

const formatTime = (value?: string) => {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(
    date.getDate()
  ).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const onSelectionChange = (rows: DhcpTemplate[]) => {
  selectedIds.value = rows.map((row) => row.id);
};

const onCurrentChange = (row?: DhcpTemplate) => {
  selectedTemplate.value = row || null;
};

const onPageChange = (page: number) => {
  pagination.page = page;
};

const onSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
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

const resetForm = () => {
  formRef.value?.clearValidate();
  form.name = '';
  form.description = '';
  form.icon = '';
  form.selectedOptionIds = [];
  form.optionValues = {};
};

const syncOptionValues = () => {
  const next: Record<string, string> = {};
  for (const option of selectedOptions.value) {
    next[option.id] = form.optionValues[option.id] ?? option.value;
  }
  form.optionValues = next;
};

const openCreate = () => {
  editingTemplate.value = null;
  resetForm();
  formSnapshot.value = JSON.stringify(form);
  dialogVisible.value = true;
};

const openEdit = (tpl: DhcpTemplate) => {
  editingTemplate.value = tpl;
  form.name = tpl.name;
  form.description = tpl.description;
  form.icon = tpl.icon || '';
  form.selectedOptionIds = tpl.options.map((item) => item.optionId);
  form.optionValues = tpl.options.reduce<Record<string, string>>((acc, item) => {
    acc[item.optionId] = item.value;
    return acc;
  }, {});
  formSnapshot.value = JSON.stringify(form);
  dialogVisible.value = true;
};

const onTemplateNameInput = () => formRef.value?.validateField('name');

const removeSelectedOption = (optionId: string) => {
  form.selectedOptionIds = form.selectedOptionIds.filter((id) => id !== optionId);
  const next = { ...form.optionValues };
  delete next[optionId];
  form.optionValues = next;
};

const openTemplatePreview = async () => {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false);
  if (!valid) {
    showWarning('请先修正错误项后再预览');
    return;
  }
  previewVisible.value = true;
};

const handleTemplateCancel = async () => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== formSnapshot.value;
  if (!dirty) {
    dialogVisible.value = false;
    return;
  }
  try {
    await ElMessageBox.confirm('当前有未保存配置，确认关闭吗？', '未保存提醒', {
      type: 'warning',
      confirmButtonText: '确认关闭',
      cancelButtonText: '继续编辑'
    });
    dialogVisible.value = false;
  } catch {
    return;
  }
};

const handleTemplateBeforeClose = async (done: () => void) => {
  if (saving.value) return;
  const dirty = JSON.stringify(form) !== formSnapshot.value;
  if (!dirty) {
    done();
    return;
  }
  try {
    await ElMessageBox.confirm('当前有未保存配置，确认关闭吗？', '未保存提醒', {
      type: 'warning',
      confirmButtonText: '确认关闭',
      cancelButtonText: '继续编辑'
    });
    done();
  } catch {
    return;
  }
};

const buildTemplateOptions = (): DhcpTemplateOption[] =>
  selectedOptions.value.map((option) => ({
    optionId: option.id,
    code: option.code,
    name: option.name,
    value: form.optionValues[option.id] ?? option.value,
    description: option.description
  }));

const saveTemplate = () => {
  formRef.value?.validate((valid: boolean) => {
    if (!valid) return;
    void (async () => {
      saving.value = true;
      if (editingTemplate.value) {
        const result = await updateTemplateSafe(editingTemplate.value.id, {
          name: form.name.trim(),
          description: form.description.trim(),
          icon: form.icon.trim() || undefined,
          options: buildTemplateOptions()
        });
        if (!result.ok) {
          showError(result.message || '模板更新失败');
          saving.value = false;
          return;
        }
        showSuccess('模板已更新');
      } else {
        const result = await addTemplateSafe({
          name: form.name.trim(),
          description: form.description.trim(),
          icon: form.icon.trim() || undefined,
          options: buildTemplateOptions()
        });
        if (!result.ok) {
          showError(result.message || '模板创建失败');
          saving.value = false;
          return;
        }
        showSuccess('模板已创建');
      }
      saving.value = false;
      dialogVisible.value = false;
    })();
  });
};

const handleDelete = async (tpl: DhcpTemplate) => {
  const usage = templateUsage(tpl.id);
  if (usage.count > 0) {
    showWarning('该模板已被作用域引用，禁止删除');
    return;
  }
  try {
    await ElMessageBox.confirm(`确认删除模板 ${tpl.name}？`, '删除确认', { type: 'warning' });
  } catch {
    return;
  }
  const result = await removeTemplates([tpl.id]);
  if (result.failed.length) {
    showError(result.failed[0].message || '模板删除失败');
    return;
  }
  if (result.blocked.length) {
    showWarning('该模板已被作用域引用，禁止删除');
    return;
  }
  showSuccess('模板已删除');
};

const batchDelete = async () => {
  if (!batchDeletable.value.length) return;
  try {
    await ElMessageBox.confirm(`确认删除 ${batchDeletable.value.length} 个未引用模板？`, '批量删除确认', {
      type: 'warning'
    });
  } catch {
    return;
  }
  deleting.value = true;
  const result = await removeTemplates(batchDeletable.value);
  deleting.value = false;
  clearSelection();
  if (result.failed.length) {
    showWarning(`批量删除完成，${result.failed.length} 项失败：${result.failed[0].message}`);
    return;
  }
  showSuccess('批量删除完成');
};

const syncTemplate = async (row?: DhcpTemplate) => {
  const target = row || selectedTemplate.value;
  if (!target) {
    showWarning('请先选择模板');
    return;
  }
  const usage = templateUsage(target.id);
  if (!usage.count) {
    showWarning('该模板暂无引用作用域');
    return;
  }
  syncing.value = !row;
  syncingId.value = row ? row.id : '';
  const result = await syncTemplateToReferencedScopes(target.id, 'merge', 'template_wins');
  syncing.value = false;
  syncingId.value = '';
  if (!result.ok) {
    showError(result.message || '模板同步失败');
    return;
  }
  showSuccess(`模板同步完成，已更新 ${result.applied} 个作用域`);
};

const openApply = (tpl?: DhcpTemplate) => {
  applyForm.templateId = tpl?.id || selectedTemplate.value?.id || templates.value[0]?.id || '';
  applyForm.scopeIds = [];
  applyForm.mode = 'merge';
  applyForm.conflict = 'template_wins';
  applyVisible.value = true;
};

const submitApply = async () => {
  if (!applyForm.templateId) {
    showWarning('请先选择模板');
    return;
  }
  const scopeResult = applyTemplateToScopes(
    applyForm.templateId,
    applyForm.scopeIds,
    applyForm.mode,
    applyForm.conflict
  );
  if (!applyForm.scopeIds.length) {
    applyTemplateToGlobal(applyForm.templateId, applyForm.mode, applyForm.conflict);
    const applyResult = await applyTemplate(applyForm.templateId);
    if (!applyResult.ok) {
      showError(applyResult.message || '模板应用失败');
      return;
    }
    applyVisible.value = false;
    showSuccess(`应用完成，已更新 ${scopeResult.applied} 个作用域`);
    return;
  } else {
    let failedCount = 0;
    let firstMessage = '';
    for (const scopeID of applyForm.scopeIds) {
      const scope = scopes.value.find((item) => item.id === scopeID);
      if (!scope) continue;
      const result = await updateScopeSafe(scopeID, {
        templateId: applyForm.templateId,
        options: [...scope.options]
      });
      if (!result.ok) {
        failedCount++;
        if (!firstMessage) firstMessage = result.message || '作用域更新失败';
      }
    }
    if (failedCount > 0) {
      applyVisible.value = false;
      showWarning(`已应用到部分作用域，${failedCount} 项失败：${firstMessage}`);
      return;
    }
  }
  applyVisible.value = false;
  showSuccess(`应用完成，已更新 ${scopeResult.applied} 个作用域`);
};

const onPickerSelection = (rows: DhcpOption[]) => {
  optionPickerSelected.value = rows.map((row) => row.id);
};

const importPickedOptions = () => {
  const merged = Array.from(new Set([...form.selectedOptionIds, ...optionPickerSelected.value]));
  form.selectedOptionIds = merged;
  syncOptionValues();
  optionPickerVisible.value = false;
  showSuccess(`已导入 ${optionPickerSelected.value.length} 个选项`);
};

const resetFilters = () => {
  keyword.value = '';
  refFilter.value = '';
  pagination.page = 1;
};

const refresh = async () => {
  loading.value = true;
  await load();
  loading.value = false;
};

const goScopes = (_tpl: DhcpTemplate) => {
  router.push('/option/scopes');
};
</script>

<style scoped>
.page-wrap {
  --page-bg: #f5f7fa;
  --surface-bg: #ffffff;
  --surface-border: #e5e7eb;
  --text-secondary: #6b7280;
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

.table-empty {
  border: 1px dashed var(--empty-border);
  border-radius: 10px;
}

.inner-table {
  background: var(--surface-bg);
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.icon {
  font-size: 16px;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--divider);
}

.option-import-wrap {
  width: 440px;
  display: flex;
  gap: 8px;
}

.icon-selector-wrap {
  width: 440px;
}

.icon-search-input {
  margin-bottom: 8px;
}

.icon-option {
  display: flex;
  align-items: center;
  gap: 8px;
}

.icon-option-category {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 500;
}

.icon-option-clear {
  color: var(--el-color-danger);
}

.icon-symbol {
  width: 18px;
  text-align: center;
}

.icon-name {
  flex: 1;
}

.icon-preview {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.option-import-select {
  width: 352px;
}

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
.standard-form :deep(.el-textarea) {
  width: 440px;
}

.standard-form :deep(.el-input__wrapper),
.standard-form :deep(.el-select__wrapper),
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

.mb-8 {
  margin-bottom: 8px;
}
</style>