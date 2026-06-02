import { computed, reactive, ref } from 'vue';
import { useDebouncedFetch } from './useDebouncedFetch';

export interface TableFetcherParams<F> {
  page: number;
  pageSize: number;
  filters: F;
  signal?: AbortSignal;
}

export interface TableFetcherResult<T> {
  items: T[];
  total: number;
}

export interface UseTableFetchOptions<F> {
  initialPage?: number;
  initialPageSize?: number;
  filters: F;
  delay?: number;
}

export const useTableFetch = <T, F extends Record<string, any>>(
  fetcher: (params: TableFetcherParams<F>) => Promise<TableFetcherResult<T>>,
  options: UseTableFetchOptions<F>
) => {
  const page = ref(options.initialPage ?? 1);
  const pageSize = ref(options.initialPageSize ?? 20);
  const filters = reactive({ ...(options.filters as F) });
  const rows = ref<T[]>([]);
  const total = ref(0);

  const { run, runNow, loading, error, cancel } = useDebouncedFetch<[], TableFetcherResult<T>>(
    async (_args, signal) => {
      const result = await fetcher({
        page: page.value,
        pageSize: pageSize.value,
        filters: filters as F,
        signal
      });
      rows.value = result.items;
      total.value = result.total;
      return result;
    },
    { delay: options.delay ?? 200 }
  );

  const refresh = () => run();
  const refreshNow = () => runNow();

  const pagination = computed(() => ({
    page: page.value,
    pageSize: pageSize.value,
    total: total.value
  }));

  const setPage = (value: number) => {
    page.value = value;
    refreshNow();
  };

  const setPageSize = (value: number) => {
    pageSize.value = value;
    page.value = 1;
    refreshNow();
  };

  return {
    rows,
    total,
    page,
    pageSize,
    filters,
    loading,
    error,
    refresh,
    refreshNow,
    setPage,
    setPageSize,
    pagination,
    cancel
  };
};
