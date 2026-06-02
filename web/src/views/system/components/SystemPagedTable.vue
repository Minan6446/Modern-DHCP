<template>
  <div class="system-paged-table">
    <el-table v-loading="loading" :data="data" border stripe height="100%">
      <slot />
    </el-table>
    <div class="pager">
      <el-pagination
        :current-page="page"
        :page-size="pageSize"
        :total="total"
        layout="prev, pager, next, jumper"
        @update:current-page="onCurrentPageChange"
        @update:page-size="onPageSizeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts" generic="T extends Record<string, unknown>">
interface Props {
  loading: boolean;
  data: T[];
  page: number;
  pageSize: number;
  total: number;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'update:page', value: number): void;
  (e: 'update:pageSize', value: number): void;
  (e: 'page-change', value: number): void;
  (e: 'size-change', value: number): void;
}>();

const onCurrentPageChange = (value: number) => {
  emit('update:page', value);
  emit('page-change', value);
};

const onPageSizeChange = (value: number) => {
  emit('update:pageSize', value);
  emit('size-change', value);
};
</script>

<style scoped>
.system-paged-table {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}

.pager {
  display: flex;
  justify-content: flex-end;
  padding: 8px 0;
}
</style>
