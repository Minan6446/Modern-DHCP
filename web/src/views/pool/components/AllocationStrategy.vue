<template>
  <el-card shadow="hover" class="panel">
    <div class="panel-header">{{ t('pool.allocationTitle') }}</div>
    <el-form label-width="140px" :model="strategy">
      <el-form-item :label="t('pool.allocationAlgorithm')">
        <el-select v-model="strategy.mode">
          <el-option value="round-robin" :label="t('pool.allocationRoundRobin')" />
          <el-option value="sequential" :label="t('pool.allocationSequential')" />
          <el-option value="random" :label="t('pool.allocationRandom')" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('pool.excludeBatch')">
        <el-input v-model="excludeText" type="textarea" :autosize="{ minRows: 4 }" />
        <div class="actions">
          <el-button size="small" @click="importExclude">{{ t('pool.importBtn') }}</el-button>
          <el-button size="small" @click="exportExclude">{{ t('pool.exportBtn') }}</el-button>
          <el-button size="small" type="danger" @click="clearExclude">{{ t('pool.clearBtn') }}</el-button>
        </div>
        <div class="tags">
          <el-tag v-for="ip in exclude" :key="ip" closable @close="remove(ip)">{{ ip }}</el-tag>
        </div>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AllocationStrategy } from '@/types/pool';
import { dedupeIps } from '@/utils/ip';
import { parseCsv, toCsv, downloadCsv } from '@/utils/csv';
import { showSuccess } from '@/shared/errors/messageToast';

const { t } = useI18n();
const props = defineProps<{ modelValue: AllocationStrategy; excludeList: string[] }>();
const emits = defineEmits<{
  (e: 'update:modelValue', v: AllocationStrategy): void;
  (e: 'update:excludeList', v: string[]): void;
}>();

const strategy = ref<AllocationStrategy>({ ...props.modelValue });
watch(strategy, (v) => emits('update:modelValue', v), { deep: true });

const exclude = ref<string[]>([...props.excludeList]);
watch(exclude, (v) => emits('update:excludeList', v), { deep: true });

const excludeText = ref(props.excludeList.join('\n'));

const importExclude = () => {
  const rows = parseCsv(excludeText.value);
  const flat = rows.flat();
  exclude.value = dedupeIps([...exclude.value, ...flat]);
  showSuccess(t('pool.imported'));
};

const exportExclude = () => {
  downloadCsv('exclude.csv', toCsv(exclude.value.map((i) => [i])));
};

const clearExclude = () => {
  exclude.value = [];
};

const remove = (ip: string) => {
  exclude.value = exclude.value.filter((x) => x !== ip);
};
</script>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
  margin: 8px 0;
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
</style>
