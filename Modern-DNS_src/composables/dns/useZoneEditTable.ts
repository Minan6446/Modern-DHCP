import { computed, reactive, shallowRef, toValue, watch } from 'vue'
import type { ComputedRef, MaybeRefOrGetter, Ref } from 'vue'

export type ZoneEditTableSortOrder = 'ascending' | 'descending' | null

export interface ZoneEditTableSortState<Row> {
  prop: keyof Row | ''
  order: ZoneEditTableSortOrder
}

export interface ZoneEditTablePagination {
  currentPage: number
  pageSize: number
}

export interface UseZoneEditTableOptions<Row extends { id: string | number }> {
  rows: MaybeRefOrGetter<Row[]>
  initialPage?: number
  initialPageSize?: number
  virtualThreshold?: number
  initialSort?: Partial<ZoneEditTableSortState<Row>>
}

export interface UseZoneEditTableResult<Row extends { id: string | number }> {
  selectedRows: Ref<Row[]>
  selectedRowIds: Ref<Array<Row['id']>>
  tableData: ComputedRef<Row[]>
  pagination: ZoneEditTablePagination
  sortState: ZoneEditTableSortState<Row>
  isVirtualMode: ComputedRef<boolean>
  handleSelectionChange: (rows: Row[]) => void
  handleVirtualSelectionChange: (id: Row['id'], checked: boolean) => void
  handlePageChange: (currentPage: number, pageSize?: number) => void
  handleSortChange: (payload: { prop: keyof Row | '' | null; order: ZoneEditTableSortOrder }) => void
  syncSelectedRowsByIds: () => void
}

const compareValues = (left: unknown, right: unknown): number => {
  if (left == null && right == null) {
    return 0
  }
  if (left == null) {
    return -1
  }
  if (right == null) {
    return 1
  }

  if (typeof left === 'number' && typeof right === 'number') {
    return left - right
  }

  const leftDate = Date.parse(String(left))
  const rightDate = Date.parse(String(right))
  if (!Number.isNaN(leftDate) && !Number.isNaN(rightDate)) {
    return leftDate - rightDate
  }

  return String(left).localeCompare(String(right), 'zh-CN', { numeric: true, sensitivity: 'base' })
}

const sortRows = <Row extends Record<string, unknown>>(
  rows: Row[],
  sortState: ZoneEditTableSortState<Row>,
): Row[] => {
  if (!sortState.prop || !sortState.order) {
    return rows.slice()
  }

  const direction = sortState.order === 'ascending' ? 1 : -1

  return rows.slice().sort((left, right) => {
    const leftValue = left[sortState.prop]
    const rightValue = right[sortState.prop]
    return compareValues(leftValue, rightValue) * direction
  })
}

/**
 * 管理 Zone 编辑页表格的勾选、分页、排序与虚拟滚动切换状态。
 *
 * 该 composable 设计为可直接替换 ZoneEditPage 中当前分散的表格交互逻辑。
 * 在组件中可通过将 isVirtualMode 重命名为 useVirtualTable 的方式，保留原有模板调用位置。
 */
export const useZoneEditTable = <Row extends { id: string | number }>(
  options: UseZoneEditTableOptions<Row>,
): UseZoneEditTableResult<Row> => {
  const selectedRows = shallowRef<Row[]>([])
  const selectedRowIds = shallowRef<Array<Row['id']>>([])
  const pagination = reactive<ZoneEditTablePagination>({
    currentPage: options.initialPage ?? 1,
    pageSize: options.initialPageSize ?? 5,
  })
  const sortState = reactive({
    prop: (options.initialSort?.prop ?? '') as keyof Row | '',
    order: options.initialSort?.order ?? null,
  }) as ZoneEditTableSortState<Row>

  const virtualThreshold = options.virtualThreshold ?? Infinity
  const sourceRows = computed<Row[]>(() => toValue(options.rows) ?? [])
  const sortedRows = computed<Row[]>(() => sortRows(sourceRows.value, sortState as ZoneEditTableSortState<Row>))
  const isVirtualMode = computed<boolean>(() => sortedRows.value.length > virtualThreshold)
  const tableData = computed<Row[]>(() => {
    if (isVirtualMode.value) {
      return sortedRows.value
    }

    const start = (pagination.currentPage - 1) * pagination.pageSize
    return sortedRows.value.slice(start, start + pagination.pageSize)
  })

  /**
   * 根据当前选中的行 ID 回填完整行数据，供虚拟表格和普通表格共享。
   */
  const syncSelectedRowsByIds = (): void => {
    const rowMap = new Map(sourceRows.value.map((row) => [row.id, row]))
    selectedRows.value = selectedRowIds.value
      .map((id) => rowMap.get(id))
      .filter((row): row is Row => Boolean(row))
  }

  /**
   * 处理 el-table 的 selection-change 事件。
   */
  const handleSelectionChange = (rows: Row[]): void => {
    selectedRows.value = rows.slice()
    selectedRowIds.value = rows.map((row) => row.id)
  }

  /**
   * 处理虚拟表格中的勾选状态变更。
   */
  const handleVirtualSelectionChange = (id: Row['id'], checked: boolean): void => {
    const selectedIdSet = new Set<Row['id']>(selectedRowIds.value)
    if (checked) {
      selectedIdSet.add(id)
    } else {
      selectedIdSet.delete(id)
    }
    selectedRowIds.value = Array.from(selectedIdSet)
    syncSelectedRowsByIds()
  }

  /**
   * 统一处理分页切换。传入 pageSize 时会同步更新每页条数。
   */
  const handlePageChange = (currentPage: number, pageSize = pagination.pageSize): void => {
    pagination.currentPage = currentPage
    pagination.pageSize = pageSize
  }

  /**
   * 记录表格排序状态，便于普通表格和虚拟表格共用同一套数据源。
   */
  const handleSortChange = (payload: { prop: keyof Row | '' | null; order: ZoneEditTableSortOrder }): void => {
    sortState.prop = (payload.prop ?? '') as ZoneEditTableSortState<Row>['prop']
    sortState.order = payload.order ?? null
    pagination.currentPage = 1
  }

  watch(
    sourceRows,
    (rows) => {
      syncSelectedRowsByIds()

      const totalPages = Math.max(1, Math.ceil(rows.length / pagination.pageSize))
      if (!isVirtualMode.value && pagination.currentPage > totalPages) {
        pagination.currentPage = totalPages
      }
      if (!rows.length) {
        pagination.currentPage = 1
      }
    },
    { deep: true },
  )

  return {
    selectedRows,
    selectedRowIds,
    tableData,
    pagination,
    sortState,
    isVirtualMode,
    handleSelectionChange,
    handleVirtualSelectionChange,
    handlePageChange,
    handleSortChange,
    syncSelectedRowsByIds,
  }
}

export default useZoneEditTable