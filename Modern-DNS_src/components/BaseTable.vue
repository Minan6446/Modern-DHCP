<script setup lang="ts" generic="Row extends { id: string | number }">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    data: Row[]
    total: number
    currentPage: number
    pageSize: number
    loading?: boolean
    density?: 'compact' | 'default' | 'relaxed'
    rowClassName?: (row: Row) => string
    showPagination?: boolean
    pageSizes?: number[]
  }>(),
  {
    loading: false,
    density: 'default',
    showPagination: true,
    pageSizes: () => [5, 10, 20, 50],
    rowClassName: () => '',
  },
)

const emit = defineEmits<{
  (e: 'update:currentPage', page: number): void
  (e: 'update:pageSize', size: number): void
  (e: 'sort-change', payload: { prop: string; order: string | null }): void
  (e: 'selection-change', rows: Row[]): void
}>()

const rowHeight = computed(() => {
  if (props.density === 'compact') return '36px'
  if (props.density === 'relaxed') return '56px'
  return '48px'
})
</script>

<template>
  <div class="base-table-wrap" :class="`base-table-wrap--${density}`">
    <div v-loading="loading">
      <el-table
        class="mn-table"
        :data="data"
        :row-class-name="({ row }: { row: Row }) => rowClassName(row)"
        :style="{ '--row-height': rowHeight }"
        stripe
        table-layout="fixed"
        @sort-change="emit('sort-change', $event)"
        @selection-change="emit('selection-change', $event)"
      >
        <slot />
      </el-table>

      <div v-if="showPagination && total > 0" class="mn-pagination">
        <el-pagination
          :current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          :page-sizes="pageSizes"
          layout="total, sizes, prev, pager, next"
          size="small"
          background
          @current-change="emit('update:currentPage', $event)"
          @size-change="emit('update:pageSize', $event)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.mn-table :deep(.el-table__header th) {
  background: var(--app-bg-secondary);
}
.mn-table :deep(.el-table__header th .cell) {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--app-text-secondary);
}
.mn-table :deep(.el-table__body td .cell) {
  font-size: 13px;
  line-height: 1.5;
}
.mn-table :deep(.el-table__body tr:hover > td.el-table__cell) {
  background: rgba(22, 93, 255, 0.04) !important;
}
.mn-table :deep(.el-table__row) {
  height: var(--row-height, 48px);
}

/* compact */
.base-table-wrap--compact .mn-table :deep(.el-table__body td .cell) {
  font-size: 12px;
  padding-top: 4px;
  padding-bottom: 4px;
}
/* relaxed */
.base-table-wrap--relaxed .mn-table :deep(.el-table__body td .cell) {
  padding-top: 12px;
  padding-bottom: 12px;
}

.mn-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 10px 16px;
  border-top: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
}
</style>
