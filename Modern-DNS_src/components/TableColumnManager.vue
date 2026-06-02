<script setup lang="ts">
import { computed, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { Operation, RefreshLeft } from '@element-plus/icons-vue'

export type TableColumnManagerItem = {
  prop: string
  label: string
  visible: boolean
}

const props = defineProps<{
  columns: readonly TableColumnManagerItem[]
  storageKey: string
  visibleColumns?: string[]
}>()

const emit = defineEmits<{
  (event: 'update:visibleColumns', value: string[]): void
}>()

const getDefaultVisibleColumns = (columns: readonly TableColumnManagerItem[]): string[] =>
  columns.filter((column) => column.visible).map((column) => column.prop)

const normalizeVisibleColumns = (value: string[] | undefined, columns: readonly TableColumnManagerItem[]): string[] => {
  const validColumnMap = new Map(columns.map((column) => [column.prop, column]))
  const incoming = Array.isArray(value) ? value : []
  const incomingSet = new Set(incoming.filter((prop) => validColumnMap.has(prop)))

  columns.forEach((column) => {
    if (column.visible && !incomingSet.has(column.prop) && !incoming.length) {
      incomingSet.add(column.prop)
    }
  })

  const normalized = columns
    .map((column) => column.prop)
    .filter((prop) => incomingSet.has(prop))

  return normalized.length ? normalized : getDefaultVisibleColumns(columns)
}

const storageValue = useStorage<string[]>(
  `table-column-manager:${props.storageKey}`,
  normalizeVisibleColumns(props.visibleColumns, props.columns),
  localStorage,
)

const availableColumns = computed(() => props.columns)

const visibleColumnsModel = computed<string[]>({
  get: () => normalizeVisibleColumns(storageValue.value, availableColumns.value),
  set: (value) => {
    storageValue.value = normalizeVisibleColumns(value, availableColumns.value)
  },
})

const visibleCountText = computed(() => `${visibleColumnsModel.value.length}/${availableColumns.value.length}`)

const resetVisibleColumns = (): void => {
  visibleColumnsModel.value = getDefaultVisibleColumns(availableColumns.value)
}

watch(
  () => props.visibleColumns,
  (value) => {
    if (!value?.length) {
      return
    }
    const normalized = normalizeVisibleColumns(value, availableColumns.value)
    if (normalized.join('|') !== visibleColumnsModel.value.join('|')) {
      storageValue.value = normalized
    }
  },
)

watch(
  availableColumns,
  (columns) => {
    storageValue.value = normalizeVisibleColumns(storageValue.value, columns)
  },
  { deep: true, immediate: true },
)

watch(
  visibleColumnsModel,
  (value) => {
    emit('update:visibleColumns', value)
  },
  { immediate: true },
)
</script>

<template>
  <el-popover placement="bottom-end" trigger="click" :width="260" popper-class="table-column-manager-popper">
    <template #reference>
      <el-button plain>
        <el-icon><Operation /></el-icon>
        <span>{{ $t('common.columnManage') }}</span>
        <span class="column-count">{{ visibleCountText }}</span>
      </el-button>
    </template>

    <div class="table-column-manager">
      <div class="manager-header">
        <div class="manager-title">{{ $t('common.showColumns') }}</div>
        <el-button link type="primary" @click="resetVisibleColumns">
          <el-icon><RefreshLeft /></el-icon>
          <span>{{ $t('common.resetDefault') }}</span>
        </el-button>
      </div>

      <el-checkbox-group v-model="visibleColumnsModel" class="manager-checkbox-group">
        <el-checkbox
          v-for="column in availableColumns"
          :key="column.prop"
          :label="column.prop"
          class="manager-checkbox"
        >
          {{ column.label }}
        </el-checkbox>
      </el-checkbox-group>
    </div>
  </el-popover>
</template>

<style scoped>
.table-column-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.manager-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.manager-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--dns-text-title-color, #1d2129);
}

.manager-checkbox-group {
  display: grid;
  gap: 8px;
}

.manager-checkbox {
  margin-right: 0;
}

.column-count {
  color: var(--dns-text-assist-color, #86909c);
}
</style>