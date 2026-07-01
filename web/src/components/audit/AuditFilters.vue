<template>
  <div class="filter-bar">
    <div class="bar-left">
      <el-radio-group v-model="timePreset" size="small" @change="onTimePresetChange">
        <el-radio-button label="24h">{{ t('system.audit.preset24h') }}</el-radio-button>
        <el-radio-button label="7d">{{ t('system.audit.preset7d') }}</el-radio-button>
        <el-radio-button label="30d">{{ t('system.audit.preset30d') }}</el-radio-button>
        <el-radio-button label="custom">{{ t('system.audit.presetCustom') }}</el-radio-button>
      </el-radio-group>

      <el-date-picker
        v-if="timePreset === 'custom'"
        v-model="range"
        type="datetimerange"
        range-separator="-"
        :start-placeholder="t('system.audit.startTime')"
        :end-placeholder="t('system.audit.endTime')"
        :unlink-panels="true"
        class="filter-item range-item"
        value-format="YYYY-MM-DD HH:mm:ss"
        @change="onRangeChange"
      />

      <el-input v-model="filters.username" :placeholder="t('system.audit.username')" clearable class="filter-item w-160" />
      <el-input v-model="filters.ip" :placeholder="t('system.audit.ip')" clearable class="filter-item w-150" />
      <el-select v-model="filters.result" clearable class="filter-item w-130" :placeholder="t('system.audit.result')">
        <el-option :label="t('system.audit.success')" value="success" />
        <el-option :label="t('system.audit.failed')" value="failed" />
        <el-option :label="t('system.audit.risk')" value="risk" />
      </el-select>
      <el-select v-model="filters.operationType" clearable class="filter-item w-160" :placeholder="t('system.audit.opType')">
        <el-option v-for="item in operationTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-input v-model="filters.resourceKeyword" :placeholder="t('system.audit.resourceName')" clearable class="filter-item w-180" />
      <el-input v-model="filters.idKeyword" :placeholder="t('system.audit.idSearch')" clearable class="filter-item w-180" />
    </div>

    <div class="bar-right">
      <el-button :disabled="loading" @click="$emit('reset')">{{ t('system.audit.resetBtn') }}</el-button>
      <el-button :loading="loading" @click="$emit('search')">{{ t('system.audit.refreshBtn') }}</el-button>
      <el-button type="primary" :loading="exporting" @click="$emit('export')">{{ t('system.audit.exportBtn') }}</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue';
import { useI18n } from 'vue-i18n';

interface AuditFilters {
  username: string;
  ip: string;
  result: string;
  operationType: string;
  resourceKeyword: string;
  idKeyword: string;
}

const props = withDefaults(defineProps<{
  loading: boolean;
  exporting: boolean;
  operationTypeOptions: { label: string; value: string }[];
  initialTimePreset?: string;
  initialFilters?: Partial<AuditFilters>;
  initialRange?: [Date, Date] | [];
}>(), {
  initialTimePreset: 'custom',
  initialFilters: () => ({}),
  initialRange: () => []
});

const emit = defineEmits<{
  search: [];
  reset: [];
  export: [];
  'update:filters': [filters: AuditFilters];
  'update:timePreset': [preset: string];
  'update:range': [range: [Date, Date] | []];
}>();

const { t } = useI18n();

const timePreset = ref(props.initialTimePreset);
const range = ref<[Date, Date] | []>(props.initialRange as [Date, Date] | []);

const filters = reactive<AuditFilters>({
  username: props.initialFilters?.username || '',
  ip: props.initialFilters?.ip || '',
  result: props.initialFilters?.result || '',
  operationType: props.initialFilters?.operationType || '',
  resourceKeyword: props.initialFilters?.resourceKeyword || '',
  idKeyword: props.initialFilters?.idKeyword || ''
});

const onTimePresetChange = (preset: string) => {
  emit('update:timePreset', preset);
};

const onRangeChange = (val: [Date, Date] | null) => {
  emit('update:range', val || []);
};

watch(filters, () => emit('update:filters', { ...filters }), { deep: true });
</script>

<style scoped>
.filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.bar-left {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}

.bar-right {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.filter-item {
  margin: 0;
}

:deep(.w-130) { width: 130px; }
:deep(.w-150) { width: 150px; }
:deep(.w-160) { width: 160px; }
:deep(.w-180) { width: 180px; }
</style>
