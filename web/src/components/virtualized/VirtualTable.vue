<template>
  <ElTableV2
    :columns="columns"
    :data="data"
    :row-key="rowKey"
    :height="height"
    :width="width"
    :row-height="rowHeight"
    :estimated-row-height="estimatedRowHeight"
    :header-height="headerHeight"
    fixed
    v-bind="$attrs"
    @row-click="handleRowClick"
  />
</template>

<script setup lang="ts">
import type { TableV2Column } from 'element-plus';

interface Props<Row extends Record<string, unknown>> {
  columns: TableV2Column<Row>[];
  data: Row[];
  rowKey: string;
  height?: number;
  width?: number | string;
  rowHeight?: number;
  estimatedRowHeight?: number;
  headerHeight?: number;
}

const emit = defineEmits<{
  (e: 'row-click', row: Record<string, unknown>): void;
}>();

withDefaults(defineProps<Props<Record<string, unknown>>>(), {
  height: 480,
  width: '100%',
  rowHeight: 44,
  estimatedRowHeight: 46,
  headerHeight: 48
});

const handleRowClick = ({ rowData }: { rowData: Record<string, unknown> }) => {
  emit('row-click', rowData);
};
</script>
