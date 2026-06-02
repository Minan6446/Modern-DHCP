<template>
  <div class="option-templates page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('option.templates.heroEyebrow') }}</p>
        <h2>{{ t('option.templates.heroTitle') }}</h2>
        <p class="desc">{{ t('option.templates.heroDesc') }}</p>
        <div class="hero-metrics">
          <div v-for="metric in heroMetrics" :key="metric.key" class="metric-chip">
            <span class="label">{{ metric.label }}</span>
            <span class="value">{{ metric.value }}</span>
          </div>
        </div>
      </div>
      <div class="hero-actions">
        <el-button type="primary" size="large" :icon="Plus" @click="openCreate">{{
          t('option.templateCreate')
        }}</el-button>
        <el-button size="large" :icon="Refresh" :loading="loading" @click="fetchTemplates">{{
          t('common.refresh')
        }}</el-button>
      </div>
    </section>

    <section class="content-grid">
      <el-card shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('option.templates.listTitle') }}</p>
              <h3>{{ t('option.templates.listDesc') }}</h3>
            </div>
            <div class="card-actions">
              <el-select v-model="status" size="small" class="status-select">
                <el-option :label="t('option.templates.statusAll')" value="all" />
                <el-option :label="t('option.statusDraft')" value="draft" />
                <el-option :label="t('option.statusActive')" value="active" />
                <el-option :label="t('option.statusArchived')" value="archived" />
              </el-select>
              <el-input
                v-model.trim="keyword"
                size="small"
                :placeholder="t('option.templates.search')"
                class="search-input"
                clearable
                :prefix-icon="Search"
                @keyup.enter="handleSearch"
                @clear="handleSearch"
              >
                <template #append>
                  <el-button :icon="Search" @click="handleSearch" />
                </template>
              </el-input>
            </div>
          </div>
        </template>

        <el-table
          v-loading="loading"
          :data="templates"
          :empty-text="error || t('option.templateEmpty')"
          highlight-current-row
          @row-click="openDetail"
        >
          <el-table-column prop="name" :label="t('option.templateName')" min-width="200">
            <template #default="{ row }">
              <div class="name-cell">
                <span class="name">{{ row.name }}</span>
                <el-tag size="small">{{ row.version }}</el-tag>
              </div>
              <p class="sub">{{ row.description || '--' }}</p>
            </template>
          </el-table-column>
          <el-table-column prop="status" :label="t('option.status')" width="140">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.status)" size="small">{{
                statusLabel(row.status)
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="inheritsFrom"
            :label="t('option.templates.columnInherits')"
            width="200"
          >
            <template #default="{ row }">
              {{ inheritName(row.inheritsFrom) }}
            </template>
          </el-table-column>
          <el-table-column prop="updatedAt" :label="t('common.updatedAt')" width="180">
            <template #default="{ row }">{{ formatUpdated(row.updatedAt) }}</template>
          </el-table-column>
          <el-table-column width="180">
            <template #header>{{ t('common.actions') }}</template>
            <template #default="{ row }">
              <el-button text type="primary" size="small" @click.stop="openDetail(row)">{{
                t('common.view')
              }}</el-button>
              <el-button text type="primary" size="small" @click.stop="openEdit(row)">{{
                t('common.edit')
              }}</el-button>
              <el-button text type="danger" size="small" @click.stop="confirmDelete(row)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="table-footer">
          <el-pagination
            v-model:current-page="pagination.page"
            background
            layout="total, prev, pager, next"
            :total="pagination.total"
            :page-size="pagination.pageSize"
            @current-change="fetchTemplates"
          />
        </div>
      </el-card>

      <div class="side-stack">
        <el-card shadow="never" class="side-card">
          <template #header>
            <div class="card-header">
              <p class="card-eyebrow">{{ t('option.templates.detailTitle') }}</p>
              <h3>{{ selectedTemplate ? selectedTemplate.name : t('option.selectHint') }}</h3>
            </div>
          </template>
          <div v-if="selectedTemplate" class="detail-panel">
            <p class="detail-desc">
              {{ selectedTemplate.description || t('option.templateEmpty') }}
            </p>
            <div class="detail-tags">
              <el-tag size="small">v{{ selectedTemplate.version }}</el-tag>
              <el-tag size="small" :type="statusTag(selectedTemplate.status)">{{
                statusLabel(selectedTemplate.status)
              }}</el-tag>
              <el-tag v-if="selectedTemplate.inheritsFrom" size="small" type="info">
                {{
                  t('option.templates.detailInherits', {
                    name: inheritName(selectedTemplate.inheritsFrom)
                  })
                }}
              </el-tag>
            </div>
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item :label="t('option.templates.detailOptions')">{{
                selectedTemplate.options.length
              }}</el-descriptions-item>
              <el-descriptions-item :label="t('option.templates.detailUpdated')">{{
                formatUpdated(selectedTemplate.updatedAt)
              }}</el-descriptions-item>
            </el-descriptions>
            <el-table
              :data="selectedTemplate.options"
              height="220"
              size="small"
              border
              class="mt-12"
            >
              <el-table-column prop="optionCode" label="Code" width="80" />
              <el-table-column prop="scope" label="Scope" width="90" />
              <el-table-column prop="value" :label="t('option.columnDesc')" />
            </el-table>
          </div>
          <el-empty v-else :description="t('option.selectHint')" />
        </el-card>

        <el-card shadow="never" class="side-card">
          <template #header>
            <div class="card-header">
              <p class="card-eyebrow">{{ t('option.templateGraph') }}</p>
              <el-button size="small" text :loading="graphLoading" @click="fetchGraph">{{
                t('common.refresh')
              }}</el-button>
            </div>
          </template>
          <div class="graph-wrapper">
            <el-skeleton v-if="graphLoading" :rows="5" animated />
            <base-e-chart v-else-if="graphOption" :option="graphOption" />
            <el-empty v-else :description="t('option.templateGraphFail')" />
          </div>
        </el-card>
      </div>
    </section>

    <el-dialog
      v-model="dialogOpen"
      :title="editingTemplate ? t('option.templateEdit') : t('option.templateCreate')"
      width="520px"
      append-to-body
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item :label="t('option.templateName')" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('option.version')" prop="version">
          <el-input v-model="form.version" />
        </el-form-item>
        <el-form-item :label="t('option.status')" prop="status">
          <el-select v-model="form.status">
            <el-option :label="t('option.statusDraft')" value="draft" />
            <el-option :label="t('option.statusActive')" value="active" />
            <el-option :label="t('option.statusArchived')" value="archived" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('option.templates.formInherits')">
          <el-select v-model="form.inheritsFrom" clearable filterable>
            <el-option v-for="tpl in templates" :key="tpl.id" :label="tpl.name" :value="tpl.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('option.columnDesc')">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="t('option.templates.formOptions')" prop="optionsText">
          <el-input
            v-model="form.optionsText"
            type="textarea"
            :rows="4"
            :placeholder="t('option.templateOptionsPlaceholder')"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveTemplate">{{
          t('common.save')
        }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { showError, showSuccess, showWarning } from '@/shared/errors/messageToast';
import { Search, Plus, Refresh } from '@element-plus/icons-vue';
import BaseEChart from '@/components/echarts/BaseEChart.vue';
import {
  listOptionTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  getTemplateGraph
} from '@/api/options';
import type { OptionAssignment, OptionTemplate, OptionTemplateGraphNode } from '@/types/option';
import type { PageQuery } from '@/types/system';
import { useTenantStore } from '@/store/tenant';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();

const templates = ref<OptionTemplate[]>([]);
const pagination = reactive({ page: 1, pageSize: 10, total: 0 });
const loading = ref(false);
const error = ref('');
const keyword = ref('');
const status = ref<'all' | OptionTemplate['status']>('all');
const selectedTemplate = ref<OptionTemplate | null>(null);
const dialogOpen = ref(false);
const editingTemplate = ref<OptionTemplate | null>(null);
const saving = ref(false);
const graphLoading = ref(false);
const graphNodes = ref<OptionTemplateGraphNode[]>([]);

const formRef = ref();
const form = reactive({
  name: '',
  version: '',
  status: 'draft' as OptionTemplate['status'],
  inheritsFrom: '',
  description: '',
  optionsText: ''
});

const rules = {
  name: [{ required: true, message: t('option.templateName'), trigger: 'blur' }],
  version: [{ required: true, message: t('option.version'), trigger: 'blur' }],
  optionsText: [
    { required: true, message: t('option.templates.formOptionsRequired'), trigger: 'blur' }
  ]
};

const heroMetrics = computed(() => {
  const total = pagination.total;
  const active = templates.value.filter((tpl) => tpl.status === 'active').length;
  const inherits = templates.value.filter((tpl) => !!tpl.inheritsFrom).length;
  return [
    { key: 'total', label: t('option.templates.statTotal'), value: total },
    { key: 'active', label: t('option.templates.statActive'), value: active },
    { key: 'inherits', label: t('option.templates.statInherited'), value: inherits }
  ];
});

const graphOption = computed(() => {
  if (!graphNodes.value.length) return null;
  const nodes = graphNodes.value.map((node) => ({
    id: node.id,
    name: `${node.name}\nv${node.version}`,
    value: node.name,
    symbolSize: node.inheritsFrom ? 56 : 64
  }));
  const links = graphNodes.value
    .filter((node) => node.inheritsFrom)
    .map((node) => ({ source: node.id, target: node.inheritsFrom as string }));
  return {
    tooltip: { trigger: 'item' },
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        label: { show: true, fontSize: 12 },
        force: { repulsion: 160, edgeLength: 90 },
        data: nodes,
        links
      }
    ]
  };
});

const formatUpdated = (ts?: string) => (ts ? formatTs(ts) : '--');

const statusTag = (state: OptionTemplate['status']) => {
  if (state === 'active') return 'success';
  if (state === 'draft') return 'warning';
  return 'info';
};

const statusLabel = (state: OptionTemplate['status']) => {
  if (state === 'active') return t('option.statusActive');
  if (state === 'draft') return t('option.statusDraft');
  return t('option.statusArchived');
};

const inheritName = (id?: string) => {
  if (!id) return '--';
  return templates.value.find((tpl) => tpl.id === id)?.name || id;
};

const handleSearch = () => {
  pagination.page = 1;
  fetchTemplates();
};

const fetchTemplates = async () => {
  loading.value = true;
  error.value = '';
  try {
    const params: PageQuery & {
      keyword?: string;
      status?: OptionTemplate['status'] | 'all';
      tenantId?: string;
    } = {
      page: pagination.page,
      pageSize: pagination.pageSize,
      keyword: keyword.value || undefined,
      status: status.value === 'all' ? undefined : status.value,
      tenantId: tenantStore.currentTenantId
    };
    const { data } = await listOptionTemplates(params);
    templates.value = data.data?.items || [];
    pagination.total = data.data?.total || 0;
    if (templates.value.length) {
      if (!templates.value.find((tpl) => tpl.id === selectedTemplate.value?.id)) {
        selectedTemplate.value = templates.value[0];
      }
    } else {
      selectedTemplate.value = null;
    }
  } catch (e) {
    templates.value = [];
    pagination.total = 0;
    error.value = t('option.templateLoadFail');
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

const fetchGraph = async () => {
  graphLoading.value = true;
  try {
    const { data } = await getTemplateGraph({ tenantId: tenantStore.currentTenantId });
    graphNodes.value = data.data || [];
  } catch (e) {
    graphNodes.value = [];
  } finally {
    graphLoading.value = false;
  }
};

const openCreate = () => {
  editingTemplate.value = null;
  resetForm();
  dialogOpen.value = true;
};

const openEdit = (tpl: OptionTemplate) => {
  editingTemplate.value = tpl;
  form.name = tpl.name;
  form.version = tpl.version;
  form.status = tpl.status;
  form.inheritsFrom = tpl.inheritsFrom || '';
  form.description = tpl.description || '';
  form.optionsText = tpl.options.map((opt) => `${opt.optionCode}:${opt.value}`).join('\n');
  dialogOpen.value = true;
};

const openDetail = (tpl: OptionTemplate) => {
  selectedTemplate.value = tpl;
};

const resetForm = () => {
  form.name = '';
  form.version = '';
  form.status = 'draft';
  form.inheritsFrom = '';
  form.description = '';
  form.optionsText = '';
};

const parseOptions = (): OptionAssignment[] =>
  form.optionsText
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [code, value] = line.split(':');
      return {
        optionCode: Number(code),
        value: (value || '').trim(),
        scope: 'global' as const
      };
    })
    .filter((entry) => !Number.isNaN(entry.optionCode) && entry.value);

const saveTemplate = () => {
  formRef.value?.validate(async (valid: boolean) => {
    if (!valid) return;
    saving.value = true;
    try {
      const payload = {
        name: form.name,
        version: form.version,
        status: form.status,
        inheritsFrom: form.inheritsFrom || undefined,
        description: form.description || undefined,
        options: parseOptions(),
        tenantId: tenantStore.currentTenantId
      };
      if (!payload.options.length) {
        showWarning(t('option.templates.formOptionsRequired'));
        saving.value = false;
        return;
      }
      if (editingTemplate.value?.id) {
        await updateTemplate(editingTemplate.value.id, payload);
        showSuccess(t('option.updated'));
      } else {
        await createTemplate(payload);
        showSuccess(t('option.created'));
      }
      dialogOpen.value = false;
      await fetchTemplates();
      fetchGraph();
    } catch (e) {
      showError(t('option.saveFail'));
    } finally {
      saving.value = false;
    }
  });
};

const confirmDelete = async (tpl: OptionTemplate) => {
  try {
    await ElMessageBox.confirm(t('option.templateDeleteConfirm'), t('common.delete'), {
      type: 'warning'
    });
    await deleteTemplate(tpl.id, { tenantId: tenantStore.currentTenantId });
    showSuccess(t('option.deleted'));
    fetchTemplates();
    fetchGraph();
  } catch (e) {
    if (e === 'cancel' || e === 'close') return;
    showError(t('option.deleteFail'));
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    pagination.page = 1;
    fetchTemplates();
    fetchGraph();
  }
);

watch(status, () => {
  pagination.page = 1;
  fetchTemplates();
});

onMounted(() => {
  fetchTemplates();
  fetchGraph();
});
</script>

<style scoped>
.option-templates {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px;
  border-radius: 18px;
  background: linear-gradient(120deg, #0f172a, #1e1b4b);
  color: #fff;
}

.hero-card h2 {
  font-size: 28px;
  margin: 6px 0 12px;
}

.eyebrow {
  text-transform: uppercase;
  font-size: 12px;
  letter-spacing: 1px;
  opacity: 0.75;
}

.hero-metrics {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.metric-chip {
  background: rgba(255, 255, 255, 0.08);
  padding: 12px 14px;
  border-radius: 12px;
  min-width: 140px;
}

.metric-chip .label {
  font-size: 12px;
  opacity: 0.8;
}

.metric-chip .value {
  display: block;
  font-size: 22px;
  font-weight: 600;
}

.hero-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
  justify-content: center;
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-eyebrow {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.card-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.search-input {
  width: 220px;
}

.table-footer {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.side-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.side-card {
  width: 100%;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.name {
  font-weight: 600;
}

.sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.graph-wrapper {
  height: 320px;
}

.mt-12 {
  margin-top: 12px;
}

@media (max-width: 1200px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
}
</style>
