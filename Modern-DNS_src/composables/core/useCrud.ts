import { onMounted, reactive, ref, shallowRef, watch } from 'vue'
import type { Ref, ShallowRef } from 'vue'

type MaybePromise<T> = T | Promise<T>

export interface CrudPagination {
  currentPage: number
  pageSize: number
  total: number
}

export interface CrudApi<Item, Query extends Record<string, unknown>, CreatePayload = Partial<Item>, UpdatePayload = Partial<Item>, DeleteId = number | string> {
  getList: (params: Query & Record<string, unknown>) => MaybePromise<unknown>
  create: (payload: CreatePayload) => MaybePromise<unknown>
  update: (payload: UpdatePayload) => MaybePromise<unknown>
  delete: (id: DeleteId) => MaybePromise<unknown>
}

export interface UseCrudOptions<Item, DeleteId = number | string> {
  initialPage?: number
  initialPageSize?: number
  autoFetch?: boolean
  watchQueryParams?: boolean
  pageField?: string
  pageSizeField?: string
  createFormData?: () => Partial<Item>
  getItemId?: (item: Partial<Item>) => DeleteId | null | undefined
}

export interface UseCrudResult<Item, Query extends Record<string, unknown>> {
  loading: Ref<boolean>
  tableData: ShallowRef<Item[]>
  pagination: CrudPagination
  dialogVisible: Ref<boolean>
  formData: ShallowRef<Partial<Item>>
  fetchData: (overrides?: Partial<Query> & { currentPage?: number; pageSize?: number }) => Promise<void>
  handleCreate: () => void
  handleEdit: (item: Item) => void
  handleDelete: (item: Item | number | string) => Promise<void>
  handleSubmit: () => Promise<void>
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null

const cloneValue = <T>(value: T): T => {
  return JSON.parse(JSON.stringify(value)) as T
}

const unwrapPayload = (value: unknown): unknown => {
  if (!isRecord(value) || !('data' in value)) {
    return value
  }
  return unwrapPayload(value.data)
}

const extractListResult = <Item>(value: unknown): { list: Item[]; total: number } => {
  const payload = unwrapPayload(value)

  if (Array.isArray(payload)) {
    return {
      list: payload as Item[],
      total: payload.length,
    }
  }

  if (!isRecord(payload)) {
    return {
      list: [],
      total: 0,
    }
  }

  const candidateList = payload.list ?? payload.items ?? payload.rows ?? payload.records
  const list = Array.isArray(candidateList) ? (candidateList as Item[]) : []
  const candidateTotal = payload.total ?? payload.count ?? payload.totalCount ?? list.length
  const total = Number(candidateTotal)

  return {
    list,
    total: Number.isFinite(total) ? total : list.length,
  }
}

export const useCrud = <
  Item extends object,
  Query extends Record<string, unknown>,
  CreatePayload = Partial<Item>,
  UpdatePayload = Partial<Item>,
  DeleteId = number | string,
>(
  api: CrudApi<Item, Query, CreatePayload, UpdatePayload, DeleteId>,
  queryParams: Ref<Query>,
  options: UseCrudOptions<Item, DeleteId> = {},
): UseCrudResult<Item, Query> => {
  const loading = ref(false)
  const tableData = shallowRef<Item[]>([])
  const dialogVisible = ref(false)
  const formData = ref<Partial<Item>>(options.createFormData?.() ?? {}) as ShallowRef<Partial<Item>>
  const pagination = reactive<CrudPagination>({
    currentPage: options.initialPage ?? 1,
    pageSize: options.initialPageSize ?? 10,
    total: 0,
  })

  const pageField = options.pageField ?? 'page'
  const pageSizeField = options.pageSizeField ?? 'size'
  const getItemId = options.getItemId ?? ((item: Partial<Item>) => (item as { id?: DeleteId }).id)

  const editingId = ref<DeleteId | null>(null)

  const resetFormData = (): void => {
    formData.value = cloneValue(options.createFormData?.() ?? {})
    editingId.value = null
  }

  const buildListParams = (
    overrides?: Partial<Query> & { currentPage?: number; pageSize?: number },
  ): Query & Record<string, unknown> => {
    const nextPage = overrides?.currentPage ?? pagination.currentPage
    const nextPageSize = overrides?.pageSize ?? pagination.pageSize

    pagination.currentPage = nextPage
    pagination.pageSize = nextPageSize

    return {
      ...queryParams.value,
      ...overrides,
      [pageField]: nextPage,
      [pageSizeField]: nextPageSize,
    }
  }

  const fetchData = async (
    overrides?: Partial<Query> & { currentPage?: number; pageSize?: number },
  ): Promise<void> => {
    loading.value = true
    try {
      const result = await api.getList(buildListParams(overrides))
      const { list, total } = extractListResult<Item>(result)
      tableData.value = list
      pagination.total = total
    } finally {
      loading.value = false
    }
  }

  const handleCreate = (): void => {
    resetFormData()
    dialogVisible.value = true
  }

  const handleEdit = (item: Item): void => {
    formData.value = cloneValue(item)
    editingId.value = getItemId(item)
    dialogVisible.value = true
  }

  const handleDelete = async (item: Item | number | string): Promise<void> => {
    loading.value = true
    try {
      const deleteId = (typeof item === 'object'
        ? getItemId(item as Partial<Item>)
        : item) as DeleteId | null | undefined

      if (deleteId == null) {
        throw new Error('Missing delete id')
      }

      await api.delete(deleteId)

      const totalPages = Math.max(1, Math.ceil(Math.max(0, pagination.total - 1) / pagination.pageSize))
      if (pagination.currentPage > totalPages) {
        pagination.currentPage = totalPages
      }

      await fetchData()
    } finally {
      loading.value = false
    }
  }

  const handleSubmit = async (): Promise<void> => {
    loading.value = true
    try {
      if (editingId.value != null) {
        await api.update(formData.value as UpdatePayload)
      } else {
        await api.create(formData.value as CreatePayload)
      }

      dialogVisible.value = false
      resetFormData()
      await fetchData()
    } finally {
      loading.value = false
    }
  }

  if (options.watchQueryParams !== false) {
    watch(
      queryParams,
      async () => {
        pagination.currentPage = options.initialPage ?? 1
        await fetchData()
      },
      { deep: true },
    )
  }

  if (options.autoFetch !== false) {
    onMounted(() => {
      fetchData()
    })
  }

  return {
    loading,
    tableData,
    pagination,
    dialogVisible,
    formData,
    fetchData,
    handleCreate,
    handleEdit,
    handleDelete,
    handleSubmit,
  }
}

export default useCrud