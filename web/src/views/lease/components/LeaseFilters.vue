<template>
  <div class="filters">
    <el-form :inline="true" :model="model">
      <el-form-item :label="t('lease.filterIp')"><el-input v-model="model.ip" /></el-form-item>
      <el-form-item :label="t('lease.filterMac')"><el-input v-model="model.mac" /></el-form-item>
      <el-form-item :label="t('lease.filterClient')"
        ><el-input v-model="model.clientId"
      /></el-form-item>
      <el-form-item :label="t('lease.filterPool')"
        ><el-input v-model="model.poolId"
      /></el-form-item>
      <el-form-item :label="t('lease.filterState')">
        <el-select v-model="model.state" multiple clearable style="width: 220px">
          <el-option-group v-for="group in grouped" :key="group.key" :label="group.label">
            <el-option
              v-for="s in group.states"
              :key="s"
              :label="stateLabel(s)"
              :value="s"
              :title="stateDesc(s)"
            />
          </el-option-group>
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="emitSearch">{{ t('common.search') }}</el-button>
        <el-button @click="reset">{{ t('common.reset') }}</el-button>
        <el-button text @click="save">{{ t('lease.filtersSave') }}</el-button>
        <el-select
          v-model="savedKey"
          :placeholder="t('lease.filtersLoad')"
          clearable
          style="width: 160px"
          @change="load"
        >
          <el-option v-for="(cond, key) in saved" :key="key" :label="key" :value="key" />
        </el-select>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { LeaseFilter, LeaseState } from '@/types/lease';
import { useTenantStore } from '@/store/tenant';
import { useI18n } from 'vue-i18n';
import { showSuccess } from '@/shared/errors/messageToast';
import { groupedLeaseStates, leaseStateMeta } from '@/shared/utils/leaseState';

const props = defineProps<{ modelValue: LeaseFilter }>();
const emits = defineEmits<{
  (e: 'update:modelValue', v: LeaseFilter): void;
  (e: 'search'): void;
}>();

const model = reactive<LeaseFilter>({ ...props.modelValue });
const grouped = computed(() => [
  { key: 'active', label: '活跃', states: groupedLeaseStates().active },
  { key: 'transient', label: '过渡', states: groupedLeaseStates().transient },
  { key: 'error', label: '异常', states: groupedLeaseStates().error },
  { key: 'other', label: '其他', states: groupedLeaseStates().other }
]);
const tenantStore = useTenantStore();
const { t } = useI18n();
const storageKey = computed(() => `leaseFilters-${tenantStore.currentTenantId || 'global'}`);
const loadSaved = () => JSON.parse(localStorage.getItem(storageKey.value) || '{}');
const saved = ref<Record<string, LeaseFilter>>(loadSaved());
const savedKey = ref('');

watch(
  () => props.modelValue,
  (val) => {
    Object.assign(model, val || {});
  },
  { deep: true }
);

watch(storageKey, () => {
  saved.value = loadSaved();
  savedKey.value = '';
});

const emitSearch = () => {
  emits('update:modelValue', { ...model });
  emits('search');
};

const reset = () => {
  Object.keys(model).forEach((k) => delete (model as any)[k]);
  emits('update:modelValue', {} as LeaseFilter);
  emits('search');
  showSuccess(t('lease.filtersResetTip'));
};

const save = () => {
  const key = `filter-${Date.now()}`;
  saved.value[key] = { ...model };
  localStorage.setItem(storageKey.value, JSON.stringify(saved.value));
  showSuccess(t('lease.filtersSaved'));
};

const load = (key?: string) => {
  if (!key || !saved.value[key]) return;
  Object.assign(model, saved.value[key]);
  emitSearch();
};

const stateLabel = (s: LeaseState) => leaseStateMeta(s).label;
const stateDesc = (s: LeaseState) => leaseStateMeta(s).desc;
</script>

<style scoped>
.filters {
  padding: 8px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
}
</style>
