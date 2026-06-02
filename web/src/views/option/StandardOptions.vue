<template>
  <div class="standard-options page-block">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">{{ t('option.standard.heroEyebrow') }}</p>
        <h2>{{ t('option.standard.heroTitle') }}</h2>
        <p class="desc">{{ t('option.standard.heroDesc') }}</p>
        <div class="stat-grid">
          <div v-for="stat in stats" :key="stat.key" class="stat-chip">
            <span class="label">{{ stat.label }}</span>
            <span class="value">{{ stat.value }}</span>
          </div>
        </div>
      </div>
      <div class="hero-highlights">
        <div class="hero-header">
          <span>{{ t('option.standard.highlightTitle') }}</span>
          <el-button text size="small" :loading="loading" @click="fetchOptions">{{
            t('common.refresh')
          }}</el-button>
        </div>
        <div v-if="!highlightOptions.length" class="highlight-empty">
          {{ t('option.standard.highlightEmpty') }}
        </div>
        <div v-else class="highlight-list">
          <div
            v-for="item in highlightOptions"
            :key="item.code"
            class="highlight-item"
            @click="openDetail(item)"
          >
            <div class="highlight-meta">
              <span class="code">Opt {{ item.code }}</span>
              <el-tag v-if="item.standard" size="small" type="success">RFC</el-tag>
              <el-tag v-else-if="item.vendorId" size="small" type="info">Vendor</el-tag>
            </div>
            <p class="highlight-name">{{ item.name }}</p>
            <p class="highlight-updated">{{ formatUpdated(item.lastModifiedAt) }}</p>
          </div>
        </div>
      </div>
    </section>

    <section class="content-grid">
      <el-card class="library-card" shadow="never">
        <template #header>
          <div class="card-header">
            <div>
              <p class="card-eyebrow">{{ t('option.standard.libraryTitle') }}</p>
              <h3>{{ t('option.standard.libraryDesc') }}</h3>
            </div>
            <el-button-group>
              <el-button
                size="small"
                :disabled="!keyword && category === 'all'"
                @click="resetFilters"
                >{{ t('common.reset') }}</el-button
              >
              <el-button size="small" type="primary" :loading="loading" @click="fetchOptions">{{
                t('option.standard.refresh')
              }}</el-button>
            </el-button-group>
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
            clearable
            class="search-input"
            :prefix-icon="Search"
            @keyup.enter="fetchOptions"
            @clear="fetchOptions"
          >
            <template #append>
              <el-button :icon="Search" @click="fetchOptions" />
            </template>
          </el-input>
        </div>
        <el-alert v-if="error" type="warning" :closable="false" class="mb-12">
          {{ error }}
        </el-alert>
        <el-table
          v-loading="loading"
          :data="filteredOptions"
          stripe
          highlight-current-row
          :border="false"
          :empty-text="error || t('option.standard.empty')"
          @row-click="openDetail"
        >
          <el-table-column prop="code" :label="t('option.columnCode')" width="110" sortable />
          <el-table-column prop="name" :label="t('option.columnName')" min-width="180">
            <template #default="{ row }">
              <div class="name-cell">
                <span class="name">{{ row.name }}</span>
                <el-tag v-if="row.version" size="small" type="info">{{ row.version }}</el-tag>
              </div>
              <p class="sub">{{ row.description }}</p>
            </template>
          </el-table-column>
          <el-table-column :label="t('option.columnCategory')" width="150">
            <template #default="{ row }">
              {{ categoryLookup[row.category] || row.category }}
            </template>
          </el-table-column>
          <el-table-column prop="dataType" :label="t('option.columnType')" width="120" />
          <el-table-column :label="t('option.columnLength')" width="160">
            <template #default="{ row }">
              <span v-if="row.minLength || row.maxLength"
                >{{ row.minLength || 0 }} - {{ row.maxLength || '∞' }}</span
              >
              <span v-else>--</span>
            </template>
          </el-table-column>
          <el-table-column width="140">
            <template #header>
              <span>{{ t('common.actions') }}</span>
            </template>
            <template #default="{ row }">
              <el-button text size="small" type="primary" @click.stop="openDetail(row)">{{
                t('common.view')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <div class="insights">
        <quick-option-config class="insight-card" />
        <option-usage class="insight-card" />
      </div>
    </section>

    <el-drawer
      v-model="detailOpen"
      :title="selectedDetail ? t('option.standard.drawerTitle', { code: selectedDetail.code }) : ''"
      size="420px"
      append-to-body
    >
      <div v-if="selectedDetail" class="detail-panel">
        <p class="detail-eyebrow">{{ selectedDetail.version || t('option.standard.detailRfc') }}</p>
        <h3>{{ selectedDetail.name }}</h3>
        <p class="detail-desc">{{ selectedDetail.description || t('option.columnDesc') }}</p>
        <div class="detail-tags">
          <el-tag size="small">{{
            categoryLookup[selectedDetail.category] || selectedDetail.category
          }}</el-tag>
          <el-tag v-if="selectedDetail.standard" size="small" type="success">RFC</el-tag>
          <el-tag v-else-if="selectedDetail.vendorId" size="small" type="info"
            >Vendor {{ selectedDetail.vendorId }}</el-tag
          >
        </div>
        <el-descriptions :column="1" border size="small" class="detail-desc-list">
          <el-descriptions-item :label="t('option.columnType')">{{
            selectedDetail.dataType
          }}</el-descriptions-item>
          <el-descriptions-item :label="t('option.standard.drawerSpec')">{{
            selectedDetail.version || '--'
          }}</el-descriptions-item>
          <el-descriptions-item :label="t('option.standard.drawerLength')">
            <span v-if="selectedDetail.minLength || selectedDetail.maxLength"
              >{{ selectedDetail.minLength || 0 }} - {{ selectedDetail.maxLength || '∞' }}</span
            >
            <span v-else>--</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('option.standard.drawerAllowed')">
            <div v-if="selectedDetail.allowedValues?.length" class="chip-list">
              <el-tag v-for="val in selectedDetail.allowedValues" :key="val" size="small">{{
                val
              }}</el-tag>
            </div>
            <span v-else> -- </span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('option.standard.drawerPattern')">{{
            selectedDetail.pattern || '--'
          }}</el-descriptions-item>
        </el-descriptions>
        <div class="detail-section">
          <p class="section-title">{{ t('option.standard.drawerSample') }}</p>
          <div v-if="selectedDetail.valueExample" class="sample-block">
            {{ selectedDetail.valueExample }}
          </div>
          <el-empty v-else :description="t('option.standard.drawerNoSample')" />
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { showError } from '@/shared/errors/messageToast';
import { Search } from '@element-plus/icons-vue';
import { listStandardOptions } from '@/api/options';
import type { OptionCategory, OptionDefinition } from '@/types/option';
import OptionUsage from './components/OptionUsage.vue';
import QuickOptionConfig from './components/QuickOptionConfig.vue';
import { useTenantStore } from '@/store/tenant';
import { usePermissionStore } from '@/store/permission';
import { formatTs } from '@/utils/time';

const { t } = useI18n();
const tenantStore = useTenantStore();
const permissionStore = usePermissionStore();

const options = ref<OptionDefinition[]>([]);
const loading = ref(false);
const error = ref('');
const keyword = ref('');
const category = ref<'all' | OptionCategory>('all');
const detailOpen = ref(false);
const selectedDetail = ref<OptionDefinition | null>(null);

const categoryFilters = computed(() => [
  { value: 'all', label: t('option.categoryAll') },
  { value: 'basic', label: t('option.categoryBasic') },
  { value: 'network', label: t('option.categoryNetwork') },
  { value: 'time', label: t('option.categoryTime') },
  { value: 'security', label: t('option.categorySecurity') },
  { value: 'vendor', label: t('option.categoryVendor') }
]);

const categoryLookup = computed<Record<string, string>>(() => ({
  basic: t('option.categoryBasic'),
  network: t('option.categoryNetwork'),
  time: t('option.categoryTime'),
  security: t('option.categorySecurity'),
  vendor: t('option.categoryVendor'),
  custom: t('option.tabCustom')
}));

const filteredOptions = computed(() => {
  const kw = keyword.value.toLowerCase();
  return options.value.filter((opt) => {
    const byCategory = category.value === 'all' || opt.category === category.value;
    const byKeyword =
      !kw ||
      `${opt.code}`.includes(kw) ||
      opt.name.toLowerCase().includes(kw) ||
      (opt.description || '').toLowerCase().includes(kw);
    return byCategory && byKeyword;
  });
});

const stats = computed(() => {
  const total = options.value.length;
  const categories = new Set(options.value.map((opt) => opt.category));
  const vendorCount = options.value.filter(
    (opt) => opt.vendorId || opt.category === 'vendor'
  ).length;
  return [
    { key: 'defined', label: t('option.standard.statDefined'), value: total },
    { key: 'categories', label: t('option.standard.statCategories'), value: categories.size },
    { key: 'vendor', label: t('option.standard.statVendors'), value: vendorCount }
  ];
});

const highlightOptions = computed(() =>
  [...options.value]
    .filter((opt) => !!opt.lastModifiedAt)
    .sort(
      (a, b) =>
        new Date(b.lastModifiedAt || '').getTime() - new Date(a.lastModifiedAt || '').getTime()
    )
    .slice(0, 4)
);

const formatUpdated = (ts?: string) => (ts ? formatTs(ts) : '--');

const openDetail = (row: OptionDefinition) => {
  selectedDetail.value = row;
  detailOpen.value = true;
};

const resetFilters = () => {
  keyword.value = '';
  category.value = 'all';
  fetchOptions();
};

const fetchOptions = async () => {
  if (!permissionStore.can('option.view')) {
    error.value = t('option.noPermission');
    options.value = [];
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const params: { category?: string; keyword?: string; tenantId?: string } = {};
    if (category.value !== 'all') params.category = category.value;
    if (keyword.value) params.keyword = keyword.value;
    if (tenantStore.currentTenantId) params.tenantId = tenantStore.currentTenantId;
    const { data } = await listStandardOptions(params);
    options.value = data.data || [];
  } catch (e) {
    error.value = t('option.loadFail');
    options.value = [];
    showError(error.value);
  } finally {
    loading.value = false;
  }
};

watch(
  () => tenantStore.currentTenantId,
  () => fetchOptions()
);

watch(category, () => fetchOptions());

watch(detailOpen, (open) => {
  if (!open) selectedDetail.value = null;
});

onMounted(() => {
  fetchOptions();
});
</script>

<style scoped>
.standard-options {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 28px;
  padding: 28px;
  border-radius: 20px;
  background: linear-gradient(110deg, #0f172a, #1e1b4b);
  color: #f1f5f9;
}

.hero-copy h2 {
  font-size: 28px;
  margin: 6px 0 10px;
}

.eyebrow {
  text-transform: uppercase;
  font-size: 12px;
  letter-spacing: 1px;
  opacity: 0.7;
}

.desc {
  opacity: 0.8;
  max-width: 520px;
}

.stat-grid {
  margin-top: 18px;
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.stat-chip {
  padding: 12px 16px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.08);
  min-width: 140px;
}

.stat-chip .label {
  font-size: 12px;
  text-transform: uppercase;
  opacity: 0.7;
}

.stat-chip .value {
  display: block;
  font-size: 24px;
  font-weight: 600;
}

.hero-highlights {
  flex: 1;
  background: rgba(15, 23, 42, 0.45);
  border-radius: 16px;
  padding: 18px;
}

.hero-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.highlight-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.highlight-item {
  padding: 12px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.05);
  cursor: pointer;
  transition: border 0.2s ease;
  border: 1px solid transparent;
}

.highlight-item:hover {
  border-color: rgba(255, 255, 255, 0.2);
}

.highlight-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  opacity: 0.8;
}

.highlight-name {
  margin: 4px 0;
  font-weight: 600;
}

.highlight-empty {
  opacity: 0.7;
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 20px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.search-input {
  flex: 1;
  min-width: 200px;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.name-cell .name {
  font-weight: 600;
}

.name-cell .sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.sub {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.insights {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.insight-card :deep(.el-card) {
  width: 100%;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-eyebrow {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  text-transform: uppercase;
}

.detail-tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.detail-desc-list {
  margin-top: 12px;
}

.chip-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.detail-section {
  margin-top: 12px;
}

.sample-block {
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
    monospace;
}

.mb-12 {
  margin-bottom: 12px;
}

@media (max-width: 1200px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
}
</style>
