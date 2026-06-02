<template>
  <div v-loading="loading" class="table-wrapper">
    <el-skeleton v-if="loading && !rows.length" animated :rows="4" />
    <el-empty v-else-if="!rows.length" :description="emptyText" />
    <template v-else>
      <VirtualTable
        v-if="shouldVirtual"
        :columns="virtualColumns"
        :data="virtualRows"
        row-key="id"
        :height="tableHeight"
        :estimated-row-height="46"
        :row-height="44"
        @row-click="onVirtualRowClick"
      />
      <el-table
        v-else
        ref="tableRef"
        :data="rows"
        border
        stripe
        row-key="id"
        :height="tableHeight"
        @row-click="emitSelect"
        @selection-change="onSelectionChange"
      >
        <el-table-column v-if="selectable" type="selection" width="54" reserve-selection />
        <template v-for="col in orderedColumns" :key="col">
          <el-table-column v-if="col === 'name'" prop="name" :label="t('common.name')" min-width="180">
            <template #header>
              <div class="draggable-header" draggable="true" @dragstart="onDragStart('name')" @dragover.prevent @drop="onDrop('name')">
                {{ t('common.name') }}
              </div>
            </template>
          </el-table-column>

          <el-table-column v-else-if="col === 'cidr'" prop="cidr" :label="t('pool.labelCidr')" min-width="180">
            <template #header>
              <div class="draggable-header" draggable="true" @dragstart="onDragStart('cidr')" @dragover.prevent @drop="onDrop('cidr')">
                {{ t('pool.labelCidr') }}
              </div>
            </template>
          </el-table-column>

          <el-table-column v-else-if="col === 'utilization'" :label="t('pool.usage') + '(%)'" min-width="220">
            <template #header>
              <div class="draggable-header" draggable="true" @dragstart="onDragStart('utilization')" @dragover.prevent @drop="onDrop('utilization')">
                {{ t('pool.usage') + '(%)' }}
              </div>
            </template>
            <template #default="{ row }">
              <div class="usage-cell">
                <span class="usage-text">{{ Number(row.utilization || 0).toFixed(1) }}%</span>
                <el-progress :percentage="Math.min(100, Math.max(0, Number(row.utilization || 0)))" :stroke-width="10" :show-text="false" :color="usageColor(Number(row.utilization || 0))" />
              </div>
            </template>
          </el-table-column>

          <el-table-column v-else-if="col === 'status'" :label="t('pool.status')" width="120">
            <template #header>
              <div class="draggable-header" draggable="true" @dragstart="onDragStart('status')" @dragover.prevent @drop="onDrop('status')">
                {{ t('pool.status') }}
              </div>
            </template>
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>

          <el-table-column v-else-if="col === 'actions' && showActions" :label="t('common.actions')" width="220" fixed="right">
            <template #header>
              <div class="draggable-header" draggable="true" @dragstart="onDragStart('actions')" @dragover.prevent @drop="onDrop('actions')">
                {{ t('common.actions') }}
              </div>
            </template>
            <template #default="{ row }">
              <div class="action-cell">
                <el-button size="small" type="primary" plain class="action-btn action-edit" :disabled="actionsDisabled" @click.stop="handleCommand('edit', row)">
                  {{ t('common.edit') }}
                </el-button>
                <el-button
                  size="small"
                  :type="row.status === 'active' ? 'warning' : 'success'"
                  plain
                  class="action-btn action-toggle"
                  :disabled="actionsDisabled"
                  @click.stop="handleCommand('toggle', row)"
                >
                  {{ row.status === 'active' ? t('pool.statusDisabled') : t('pool.statusActive') }}
                </el-button>
                <el-button size="small" type="danger" plain class="action-btn action-delete" :disabled="actionsDisabled" @click.stop="handleCommand('remove', row)">
                  {{ t('common.delete') }}
                </el-button>
              </div>
            </template>
          </el-table-column>
        </template>
      </el-table>
    </template>
    <div v-if="pagination" class="pager">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        layout="prev, pager, next, sizes, total"
        @current-change="$emit('page-change', $event)"
        @size-change="$emit('size-change', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import VirtualTable from '@/components/virtualized/VirtualTable.vue';
import type { PoolSummary } from '@/types/pool';
import { ElButton, ElCheckbox } from 'element-plus';
import type { TableV2Column } from 'element-plus';
import { computed, h, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  rows: PoolSummary[];
  loading: boolean;
  pagination?: { page: number; pageSize: number; total: number };
  showActions?: boolean;
  actionsDisabled?: boolean;
  virtual?: boolean;
  virtualThreshold?: number;
  tableHeight?: number;
  selectable?: boolean;
  selectedKeys?: string[];
}>();

const emit = defineEmits<{
  (e: 'edit', row: PoolSummary): void;
  (e: 'remove', row: PoolSummary): void;
  (e: 'select', row: PoolSummary): void;
  (e: 'toggle-status', row: PoolSummary): void;
  (e: 'page-change', page: number): void;
  (e: 'size-change', size: number): void;
  (e: 'selection-change', selection: PoolSummary[]): void;
}>();

const { t } = useI18n();
const emptyText = computed(() => (props.loading ? t('monitoring.loading') : t('monitoring.empty')));
const showActions = computed(() => props.showActions !== false);
const actionsDisabled = computed(() => props.actionsDisabled === true);
const tableHeight = computed(() => props.tableHeight ?? 360);
const shouldVirtual = computed(() =>
  props.virtual !== undefined ? props.virtual : props.rows.length >= (props.virtualThreshold ?? 200)
);
const selectable = computed(() => props.selectable === true);
const selectedKeys = computed(() => props.selectedKeys || []);
const selectedKeySet = computed(() => new Set(selectedKeys.value));
const tableRef = ref<any>();
const dragColumnKey = ref<string | null>(null);
const columnOrder = ref<string[]>(['name', 'cidr', 'utilization', 'status', 'actions']);
const virtualRows = computed(() => props.rows as unknown as Record<string, unknown>[]);

const orderedColumns = computed(() =>
  columnOrder.value.filter((key) => key !== 'actions' || showActions.value)
);

const emitSelect = (row: PoolSummary) => emit('select', row);
const onVirtualRowClick = (row: Record<string, unknown>) => emitSelect(row as unknown as PoolSummary);

const syncingSelection = ref(false);

const emitSelectionChange = (keys: string[]) => {
  const selection = props.rows.filter((item) => keys.includes(item.id));
  if (!syncingSelection.value) emit('selection-change', selection);
};

const syncElTableSelection = () => {
  if (!selectable.value || !tableRef.value) return;
  const set = selectedKeySet.value;
  syncingSelection.value = true;
  tableRef.value.clearSelection();
  props.rows.forEach((row) => {
    if (set.has(row.id)) {
      tableRef.value?.toggleRowSelection(row, true);
    }
  });
  syncingSelection.value = false;
};

const onSelectionChange = (selection: PoolSummary[]) => {
  if (syncingSelection.value) return;
  emit('selection-change', selection);
};

watch(selectedKeys, () => nextTick(syncElTableSelection), { deep: true });
watch(
  () => props.rows,
  () => nextTick(syncElTableSelection),
  { deep: true }
);

const usageColor = (value: number) => {
  if (value >= 85) return '#ef4444';
  if (value >= 60) return '#f59e0b';
  return '#22c55e';
};

const statusTagType = (status?: string) => {
  switch ((status || '').toLowerCase()) {
    case 'active':
      return 'success';
    case 'warning':
      return 'warning';
    case 'disabled':
      return 'info';
    default:
      return 'info';
  }
};

const onDragStart = (key: string) => {
  dragColumnKey.value = key;
};

const onDrop = (targetKey: string) => {
  const sourceKey = dragColumnKey.value;
  dragColumnKey.value = null;
  if (!sourceKey || sourceKey === targetKey) return;
  const order = [...columnOrder.value];
  const sourceIdx = order.indexOf(sourceKey);
  const targetIdx = order.indexOf(targetKey);
  if (sourceIdx < 0 || targetIdx < 0) return;
  order.splice(sourceIdx, 1);
  order.splice(targetIdx, 0, sourceKey);
  columnOrder.value = order;
};

const handleCommand = (command: string, row: PoolSummary) => {
  if (actionsDisabled.value) return;
  if (command === 'edit') {
    emit('edit', row);
  } else if (command === 'toggle') {
    emit('toggle-status', row);
  } else if (command === 'remove') {
    emit('remove', row);
  }
};

const toggleSelection = (row: PoolSummary) => {
  const set = new Set(selectedKeySet.value);
  if (set.has(row.id)) {
    set.delete(row.id);
  } else {
    set.add(row.id);
  }
  emitSelectionChange(Array.from(set));
};

const renderSelect = ({ rowData }: { rowData: PoolSummary }) =>
  h(ElCheckbox, {
    modelValue: selectedKeySet.value.has(rowData.id),
    'onUpdate:modelValue': () => toggleSelection(rowData),
    onClick: (event: Event) => event.stopPropagation()
  });

const virtualColumns = computed<TableV2Column<PoolSummary>[]>(() => {
  const columns: TableV2Column<PoolSummary>[] = [
    ...(selectable.value
      ? [
          {
            key: 'select',
            dataKey: 'id',
            title: '',
            width: 56,
            fixed: 'left',
            cellRenderer: renderSelect
          } as TableV2Column<PoolSummary>
        ]
      : []),
    { key: 'name', dataKey: 'name', title: t('common.name'), width: 180 },
    { key: 'cidr', dataKey: 'cidr', title: t('pool.labelCidr'), width: 180 },
    {
      key: 'utilization',
      dataKey: 'utilization',
      title: `${t('pool.usage')}(%)`,
      width: 180,
      cellRenderer: ({ cellData }: { cellData: unknown }) => `${Number(cellData ?? 0).toFixed(1)}%`
    },
    {
      key: 'status',
      dataKey: 'status',
      title: t('pool.status'),
      width: 140,
      cellRenderer: ({ cellData }: { cellData: unknown }) => statusLabel(String(cellData ?? ''))
    }
  ];

  if (showActions.value) {
    columns.push({
      key: 'actions',
      dataKey: 'id',
      title: t('common.actions'),
      width: 220,
      fixed: 'right',
      cellRenderer: ({ rowData }: { rowData: PoolSummary }) =>
        h('div', { class: 'action-cell' }, [
          h(
            ElButton,
            {
              size: 'small',
              type: 'primary',
              plain: true,
              disabled: actionsDisabled.value,
              onClick: (e: Event) => {
                e.stopPropagation();
                handleCommand('edit', rowData);
              }
            },
            () => t('common.edit')
          ),
          h(
            ElButton,
            {
              size: 'small',
              type: rowData.status === 'active' ? 'warning' : 'success',
              plain: true,
              disabled: actionsDisabled.value,
              onClick: (e: Event) => {
                e.stopPropagation();
                handleCommand('toggle', rowData);
              }
            },
            () => (rowData.status === 'active' ? t('pool.statusDisabled') : t('pool.statusActive'))
          ),
          h(
            ElButton,
            {
              size: 'small',
              type: 'danger',
              plain: true,
              disabled: actionsDisabled.value,
              onClick: (e: Event) => {
                e.stopPropagation();
                handleCommand('remove', rowData);
              }
            },
            () => t('common.delete')
          )
        ])
    });
  }

  return columns;
});

const statusLabel = (status?: string) => {
  switch ((status || '').toLowerCase()) {
    case 'active':
      return t('pool.statusActive');
    case 'warning':
      return t('pool.statusWarning');
    case 'disabled':
      return t('pool.statusDisabled');
    default:
      return status || '-';
  }
};
</script>

<style scoped>
.table-wrapper {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  min-height: 0;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: auto;
}

.action-cell {
  display: flex;
  gap: 8px;
}

.draggable-header {
  user-select: none;
  cursor: move;
}

.usage-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}


.usage-text {
  font-size: 12px;
  color: #374151;
}

.sky-btn {
  background-color: #38bdf8;
  border-color: #38bdf8;
  color: #fff;
}

.sky-btn:not(.is-disabled):hover,
.sky-btn:not(.is-disabled):focus {
  background-color: #0ea5e9;
  border-color: #0ea5e9;
  color: #fff;
}

.danger-btn {
  background-color: #ef4444;
  border-color: #ef4444;
  color: #fff;
}

.danger-btn:not(.is-disabled):hover,
.danger-btn:not(.is-disabled):focus {
  background-color: #dc2626;
  border-color: #dc2626;
  color: #fff;
}
</style>
