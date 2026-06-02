<template>
  <div v-loading="loading" class="table-wrapper">
    <el-skeleton v-if="loading && !rows.length" animated :rows="4" />
    <el-empty v-else-if="!rows.length" :description="emptyText" />
    <el-table
      v-else
      :data="rows"
      border
      stripe
      height="100%"
      row-key="id"
      @row-dblclick="$emit('row-dblclick', $event)"
    >
      <el-table-column prop="ip" :label="t('lease.ip')" />
      <el-table-column prop="mac" :label="t('lease.mac')" />
      <el-table-column prop="state" :label="t('lease.state')" />
      <el-table-column prop="startsAt" :label="t('lease.startsAt')" />
      <el-table-column prop="endsAt" :label="t('lease.endsAt')" />
      <el-table-column prop="poolName" :label="t('lease.pool')" />
    </el-table>
    <div class="pager">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        layout="prev, pager, next, jumper"
        @current-change="$emit('page-change', $event)"
        @size-change="$emit('size-change', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Lease } from '@/types/lease';
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  rows: Lease[];
  loading: boolean;
  pagination: { page: number; pageSize: number; total: number };
}>();

const { t } = useI18n();
const emptyText = computed(() => (props.loading ? t('monitoring.loading') : t('monitoring.empty')));

defineEmits<{
  (e: 'page-change', page: number): void;
  (e: 'size-change', size: number): void;
  (e: 'row-dblclick', row: Lease): void;
}>();
</script>

<style scoped>
.table-wrapper {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 20px 0 24px;
  min-height: 950px;
  height: 100%;
  flex: 1;
}

.pager {
  display: flex;
  justify-content: flex-end;
  align-self: flex-end;
  margin-top: auto;
  padding: 8px 0;
}
</style>
