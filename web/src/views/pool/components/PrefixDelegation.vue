<template>
  <el-card shadow="hover" class="panel">
    <div class="panel-header">{{ t('pool.pdTitle') }}</div>
    <el-table :data="delegations" height="240" border>
      <el-table-column prop="prefix" :label="t('pool.pdColPrefix')" />
      <el-table-column prop="delegated" :label="t('pool.pdColDelegated')" />
      <el-table-column prop="capacity" :label="t('pool.pdColCapacity')" />
      <el-table-column :label="t('pool.pdColUsage')">
        <template #default="{ row }"
          >{{ ((row.delegated / row.capacity) * 100).toFixed(1) }}%</template
        >
      </el-table-column>
      <el-table-column :label="t('pool.pdColAction')" width="120">
        <template #default="{ row }">
          <el-button size="small" text @click="split(row)">{{ t('pool.pdSplit') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-form label-width="120px" class="mt">
      <el-form-item :label="t('pool.pdNewPrefix')">
        <el-input v-model="newPrefix" placeholder="2001:db8::/56" />
      </el-form-item>
      <el-button type="primary" @click="add">{{ t('pool.pdAdd') }}</el-button>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PrefixDelegation as Prefix } from '@/types/pool';
import { validateIPv6Prefix } from '@/utils/ip';
import { showError } from '@/shared/errors/messageToast';

const { t } = useI18n();
const props = defineProps<{ list: Prefix[] }>();
const emits = defineEmits<{
  (e: 'add', prefix: string): void;
  (e: 'split', prefix: Prefix): void;
}>();

const delegations = computed<Prefix[]>(() => props.list || []);
const newPrefix = ref('');

const add = () => {
  if (!validateIPv6Prefix(newPrefix.value)) {
    showError(t('pool.pdInvalidPrefix'));
    return;
  }
  emits('add', newPrefix.value);
  newPrefix.value = '';
};

const split = (p: Prefix) => emits('split', p);
</script>

<style scoped>
.mt {
  margin-top: 12px;
}
</style>
