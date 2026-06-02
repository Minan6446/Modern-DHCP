<template>
  <div class="custom-options page-block">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('option.custom.heroEyebrow') }}</p>
        <h2>{{ t('option.custom.heroTitle') }}</h2>
        <p class="desc">{{ t('option.custom.heroDesc') }}</p>
        <div class="hero-stats">
          <div v-for="stat in heroStats" :key="stat.key" class="stat-chip">
            <span class="label">{{ stat.label }}</span>
            <span class="value">{{ stat.value }}</span>
          </div>
        </div>
      </div>
      <div class="hero-actions">
        <el-button type="primary" size="large" :icon="Plus" @click="openCreate">{{
          t('option.custom.createAction')
        }}</el-button>
        <el-button size="large" @click="scrollTester">{{
          t('option.custom.testerAction')
        }}</el-button>
      </div>
    </section>

    <section class="content-grid">
      <el-card shadow="never" class="table-card">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('option.custom.listTitle') }}</p>
              <h3>{{ t('option.custom.listDesc') }}</h3>
            </div>
            <div class="card-actions">
              <el-button
                size="small"
                :disabled="!keyword && category === 'all'"
                @click="resetFilters"
                >{{ t('common.reset') }}</el-button
              >
              <el-button size="small" :icon="Refresh" :loading="loading" @click="fetchOptions">{{
                t('common.refresh')
              }}</el-button>
            </div>
          </div>
        </template>

        <div class="filters">
          <el-radio-group v-model="category" size="small">
            <el-radio-button
              v-for="item in categoryFilters"
              :key="item.value"
              :label="item.value"
              >{{ item.label }}</el-radio-button
            >
          </el-radio-group>
          <el-input
            v-model.trim="keyword"
            :placeholder="t('option.filterKeyword')"
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

        <el-table
          v-loading="loading"
          :data="options"
          stripe
          highlight-current-row
          :empty-text="error || t('option.customEmpty')"
        >
          <el-table-column prop="code" :label="t('option.columnCode')" width="110" sortable />
          <el-table-column prop="name" :label="t('option.columnName')" min-width="180">
            <template #default="{ row }">
              <div class="name-cell">
                <span class="name">{{ displayName(row) }}</span>
                <el-tag v-if="row.version" size="small">{{ row.version }}</el-tag>
              </div>
              <p class="sub">{{ row.description || '--' }}</p>
            </template>
          </el-table-column>
          <el-table-column :label="t('option.columnCategory')" width="140">
            <template #default="{ row }">{{
              categoryLookup[row.category] || row.category
            }}</template>
          </el-table-column>
          <el-table-column prop="dataType" :label="t('option.columnType')" width="120" />
          <el-table-column :label="t('option.columnLength')" width="150">
            <template #default="{ row }">
              <span v-if="row.minLength || row.maxLength"
                >{{ row.minLength || 0 }} - {{ row.maxLength || '∞' }}</span
              >
              <span v-else>--</span>
            </template>
          </el-table-column>
          <el-table-column prop="lastModifiedAt" :label="t('common.updatedAt')" width="160">
            <template #default="{ row }">{{ formatUpdated(row.lastModifiedAt) }}</template>
          </el-table-column>
          <el-table-column width="160">
            <template #header>
              <span>{{ t('common.actions') }}</span>
            </template>
            <template #default="{ row }">
              <el-button type="primary" size="small" plain @click="openEdit(row)">{{
                t('common.edit')
              }}</el-button>
              <el-button type="danger" size="small" plain @click="confirmDelete(row)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="table-footer">
          <el-pagination
            v-model:current-page="pagination.page"
            v-model:page-size="pagination.pageSize"
            background
            layout="total, sizes, prev, pager, next"
            :page-sizes="[10, 20, 50]"
            :total="pagination.total"
            @current-change="fetchOptions"
            @size-change="handleSizeChange"
          />
        </div>
      </el-card>

      <div class="side-stack">
        <option-usage class="side-card" />
        <div ref="testerCard" class="side-card tester-card">
          <el-card shadow="never">
            <template #header>
              <div class="card-header">
                <div>
                  <p class="card-eyebrow">{{ t('option.testerTitle') }}</p>
                  <h3>{{ t('option.custom.testerTitle') }}</h3>
                </div>
                <el-button
                  size="small"
                  type="primary"
                  :loading="testerLoading"
                  @click="runTester"
                  >{{ t('option.testerRun') }}</el-button
                >
              </div>
            </template>
            <el-form
              :model="testerForm"
              label-width="120px"
              label-position="left"
              class="tester-form"
            >
              <el-form-item :label="t('option.custom.testerClientIp')">
                <el-input v-model="testerForm.clientIp" placeholder="192.168.10.20" />
              </el-form-item>
              <el-form-item :label="t('option.custom.testerClientMac')">
                <el-input v-model="testerForm.clientMac" placeholder="AA:BB:CC:DD:EE:FF" />
              </el-form-item>
              <el-form-item :label="t('option.custom.testerRequested')">
                <el-input
                  v-model="testerForm.requested"
                  :placeholder="t('option.custom.testerRequestedHint')"
                />
              </el-form-item>
              <el-form-item :label="t('option.custom.testerTemplate')">
                <el-select
                  v-model="testerForm.templateId"
                  clearable
                  filterable
                  :placeholder="t('option.custom.testerTemplateHint')"
                >
                  <el-option
                    v-for="tpl in templateOptions"
                    :key="tpl.id"
                    :label="`${tpl.name} · v${tpl.version}`"
                    :value="tpl.id"
                  />
                </el-select>
              </el-form-item>
              <el-divider content-position="left">{{ t('option.testerInlineAdd') }}</el-divider>
              <div
                v-for="(inline, index) in testerForm.inlineOptions"
                :key="index"
                class="inline-row"
              >
                <el-input-number
                  v-model="inline.optionCode"
                  :min="1"
                  :max="254"
                  controls-position="right"
                  class="code-input"
                />
                <el-select v-model="inline.scope" class="scope-select">
                  <el-option label="Global" value="global" />
                  <el-option label="Pool" value="pool" />
                  <el-option label="Binding" value="binding" />
                  <el-option label="Profile" value="profile" />
                </el-select>
                <el-input
                  v-model="inline.value"
                  class="value-input"
                  :placeholder="t('option.editorExample')"
                />
                <el-button
                  text
                  type="danger"
                  :disabled="testerForm.inlineOptions.length === 1"
                  @click="removeInline(index)"
                  >-</el-button
                >
              </div>
              <el-button text type="primary" @click="addInline">{{
                t('option.custom.testerAddInline')
              }}</el-button>
            </el-form>
            <div v-if="testerResult" class="tester-result">
              <el-alert type="success" :closable="false" class="mb-12">
                {{ t('option.custom.testerLatency', { ms: testerResult.latencyMs || '--' }) }}
              </el-alert>
              <el-table :data="testerResult.offeredOptions" height="180" size="small" border>
                <el-table-column prop="optionCode" label="Code" width="80" />
                <el-table-column prop="scope" label="Scope" width="90" />
                <el-table-column prop="value" :label="t('option.columnDesc')" />
              </el-table>
              <el-descriptions :column="1" size="small" border class="mt-12">
                <el-descriptions-item :label="t('option.custom.testerRaw')">{{
                  testerResult.rawPacketHex
                }}</el-descriptions-item>
              </el-descriptions>
              <el-timeline class="mt-12">
                <el-timeline-item
                  v-for="(log, idx) in testerResult.logs"
                  :key="idx"
                  :timestamp="idx + 1"
                  >{{ log }}</el-timeline-item
                >
              </el-timeline>
            </div>
          </el-card>
        </div>
      </div>
    </section>

    <el-drawer
      v-model="editorOpen"
      :with-header="false"
      size="520px"
      append-to-body
      destroy-on-close
    >
      <div class="editor-header">
        <h3>{{ editingOption ? t('option.customEdit') : t('option.customCreate') }}</h3>
        <p class="editor-hint">{{ t('option.custom.editorHint') }}</p>
      </div>
      <option-editor
        :model-value="editingOption || undefined"
        :existing-codes="existingCodes"
        @submit="handleSave"
      />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElMessageBox } from 'element-plus';
import { showError, showSuccess, showWarning } from '@/shared/errors/messageToast';
import { Search, Plus, Refresh } from '@element-plus/icons-vue';
import OptionEditor from './components/OptionEditor.vue';
import OptionUsage from './components/OptionUsage.vue';
import {
  listCustomOptions,
  createCustomOption,
  updateCustomOption,
  deleteCustomOption,
  listOptionTemplates,
  testOptionDelivery
} from '@/api/options';
import type { PageQuery } from '@/types/system';
import type { OptionDefinition, OptionTemplate, OptionTestResult } from '@/types/option';
import { useTenantStore } from '@/store/tenant';
import { formatTs } from '@/utils/time';
import { simulateOptionDelivery } from '@/utils/optionTest';

const { t } = useI18n();
const tenantStore = useTenantStore();

const options = ref<OptionDefinition[]>([]);
const pagination = reactive({ page: 1, pageSize: 10, total: 0 });
const loading = ref(false);
const error = ref('');
const keyword = ref('');
const category = ref<'all' | OptionDefinition['category']>('all');

const editorOpen = ref(false);
const editingOption = ref<OptionDefinition | null>(null);

const templateOptions = ref<OptionTemplate[]>([]);
const testerForm = reactive({
  clientIp: '',
  clientMac: '',
  requested: '',
  templateId: '',
  inlineOptions: [{ optionCode: 3, scope: 'global' as const, value: '' }]
});
const testerResult = ref<OptionTestResult | null>(null);
const testerLoading = ref(false);
const testerCard = ref<HTMLElement | null>(null);

const categoryFilters = computed(() => [
  { value: 'all', label: t('option.categoryAll') },
  { value: 'basic', label: t('option.categoryBasic') },
  { value: 'network', label: t('option.categoryNetwork') },
  { value: 'time', label: t('option.categoryTime') },
  { value: 'security', label: t('option.categorySecurity') },
  { value: 'vendor', label: t('option.categoryVendor') },
  { value: 'custom', label: t('option.tabCustom') }
]);

const categoryLookup = computed<Record<string, string>>(() => ({
  basic: t('option.categoryBasic'),
  network: t('option.categoryNetwork'),
  time: t('option.categoryTime'),
  security: t('option.categorySecurity'),
  vendor: t('option.categoryVendor'),
  custom: t('option.tabCustom')
}));

const displayName = (row: OptionDefinition) => {
  const key = `option.names.${row.code}`;
  const translated = t(key);
  if (translated !== key) return translated;
  return row.name;
};

const heroStats = computed(() => {
  const categoryCount = new Set(options.value.map((opt) => opt.category)).size;
  const recent = [...options.value]
    .filter((opt) => !!opt.lastModifiedAt)
    .sort(
      (a, b) =>
        new Date(b.lastModifiedAt || '').getTime() - new Date(a.lastModifiedAt || '').getTime()
    )[0];
  return [
    { key: 'total', label: t('option.custom.statTotal'), value: pagination.total },
    { key: 'categories', label: t('option.custom.statCategories'), value: categoryCount },
    {
      key: 'recent',
      label: t('option.custom.statRecent'),
      value: recent ? formatUpdated(recent.lastModifiedAt) : '--'
    }
  ];
});

const existingCodes = computed(() => options.value.map((opt) => opt.code));

const formatUpdated = (ts?: string) => (ts ? formatTs(ts) : '--');

const handleSizeChange = (size: number) => {
  pagination.pageSize = size;
  pagination.page = 1;
  fetchOptions();
};

const handleSearch = () => {
  pagination.page = 1;
  fetchOptions();
};

const resetFilters = () => {
  category.value = 'all';
  keyword.value = '';
  pagination.page = 1;
  fetchOptions();
};

const fetchOptions = async () => {
  loading.value = true;
  error.value = '';
  try {
    const params: PageQuery & { category?: string; keyword?: string; tenantId?: string } = {
      page: pagination.page,
      pageSize: pagination.pageSize,
      keyword: keyword.value || undefined,
      tenantId: tenantStore.currentTenantId
    };
    if (category.value !== 'all') params.category = category.value;
    const { data } = await listCustomOptions(params);
    options.value = data.data?.items || [];
    pagination.total = data.data?.total || 0;
  } catch (e) {
    options.value = [];
    pagination.total = 0;
    error.value = t('option.loadFail');
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

const fetchTemplates = async () => {
  try {
    const { data } = await listOptionTemplates({
      page: 1,
      pageSize: 50,
      tenantId: tenantStore.currentTenantId
    });
    templateOptions.value = data.data?.items || [];
  } catch (e) {
    templateOptions.value = [];
  }
};

const openCreate = () => {
  editingOption.value = null;
  editorOpen.value = true;
};

const openEdit = (row: OptionDefinition) => {
  editingOption.value = row;
  editorOpen.value = true;
};

const handleSave = async (payload: Partial<OptionDefinition>) => {
  try {
    const body = { ...payload, tenantId: tenantStore.currentTenantId };
    if (editingOption.value?.id) {
      await updateCustomOption(editingOption.value.id, body);
      showSuccess(t('option.updated'));
    } else {
      await createCustomOption(body);
      showSuccess(t('option.created'));
    }
    editorOpen.value = false;
    await fetchOptions();
  } catch (e) {
    showError(t('option.saveFail'));
  }
};

const confirmDelete = async (row: OptionDefinition) => {
  if (!row.id) return;
  try {
    await ElMessageBox.confirm(t('option.customDeleteConfirm'), t('common.delete'), {
      type: 'warning'
    });
    await deleteCustomOption(row.id, { tenantId: tenantStore.currentTenantId });
    showSuccess(t('option.deleted'));
    fetchOptions();
  } catch (e) {
    if (e === 'cancel' || e === 'close') return;
    showError(t('option.deleteFail'));
  }
};

const addInline = () => {
  testerForm.inlineOptions.push({ optionCode: 15, scope: 'global', value: '' });
};

const removeInline = (index: number) => {
  if (testerForm.inlineOptions.length === 1) return;
  testerForm.inlineOptions.splice(index, 1);
};

const buildTesterRequest = () => {
  const requestedOptions = testerForm.requested
    .split(',')
    .map((v) => Number(v.trim()))
    .filter((v) => !Number.isNaN(v));
  const inlineOptions = testerForm.inlineOptions
    .filter((item) => item.optionCode && item.value)
    .map((item) => ({
      optionCode: item.optionCode,
      scope: item.scope,
      value: item.value
    }));
  const payload: any = {
    clientIp: testerForm.clientIp || undefined,
    clientMac: testerForm.clientMac || undefined,
    requestedOptions: requestedOptions.length ? requestedOptions : undefined,
    templateId: testerForm.templateId || undefined,
    inlineOptions
  };
  if (tenantStore.currentTenantId) payload.tenantId = tenantStore.currentTenantId;
  return payload;
};

const runTester = async () => {
  testerLoading.value = true;
  testerResult.value = null;
  const payload = buildTesterRequest();
  try {
    const { data } = await testOptionDelivery(payload);
    testerResult.value = data.data;
  } catch (e) {
    testerResult.value = simulateOptionDelivery(payload);
    showWarning(t('option.testerSimulated'));
  } finally {
    testerLoading.value = false;
  }
};

const scrollTester = () => {
  nextTick(() => {
    testerCard.value?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  });
};

watch(
  () => tenantStore.currentTenantId,
  () => {
    pagination.page = 1;
    fetchOptions();
    fetchTemplates();
  }
);

watch(category, () => {
  pagination.page = 1;
  fetchOptions();
});

onMounted(() => {
  fetchOptions();
  fetchTemplates();
});
</script>

<style scoped>
.custom-options {
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
  background: linear-gradient(120deg, #111827, #312e81);
  color: #fff;
}

.hero-card h2 {
  font-size: 28px;
  margin: 4px 0 12px;
}

.eyebrow {
  text-transform: uppercase;
  font-size: 12px;
  letter-spacing: 1px;
  opacity: 0.75;
}

.hero-stats {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 16px;
}

.stat-chip {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  padding: 12px 16px;
  min-width: 150px;
}

.stat-chip .label {
  font-size: 12px;
  opacity: 0.8;
}

.stat-chip .value {
  display: block;
  font-size: 24px;
  font-weight: 600;
}

.hero-actions {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 20px;
}

.filters {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.search-input {
  min-width: 240px;
  flex: 1;
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
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.name {
  font-weight: 600;
}

.sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
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

.inline-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

.code-input {
  width: 110px;
}

.scope-select {
  width: 120px;
}

.value-input {
  flex: 1;
}

.tester-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tester-result {
  margin-top: 12px;
}

.mt-12 {
  margin-top: 12px;
}

.mb-12 {
  margin-bottom: 12px;
}

.editor-header {
  margin-bottom: 12px;
}

.editor-hint {
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

@media (max-width: 1200px) {
  .content-grid {
    grid-template-columns: 1fr;
  }

  .hero-card {
    flex-direction: column;
  }
}
</style>
