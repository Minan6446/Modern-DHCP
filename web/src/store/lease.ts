import { defineStore } from 'pinia';
import { listActiveLeases, listHistoryLeases } from '@/api/leases';
import type { Lease, LeaseFilter } from '@/types/lease';

interface LeaseState {
  active: Lease[];
  history: Lease[];
  filters: LeaseFilter;
  historyFilters: LeaseFilter;
  stats: { active: number; expired: number; pending: number; failed: number };
  loadingActive: boolean;
  loadingHistory: boolean;
  activePagination: { page: number; pageSize: number; total: number };
  historyPagination: { page: number; pageSize: number; total: number };
}

const STORAGE_KEY = 'mdhcp-lease';

const loadPersisted = (): Partial<LeaseState> => {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (e) {
    return {};
  }
};

const persist = (state: LeaseState) => {
  sessionStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ filters: state.filters, historyFilters: state.historyFilters })
  );
};

const summarize = (rows: Lease[]) => {
  const result = { expired: 0, pending: 0, failed: 0 };
  rows.forEach((lease) => {
    switch (lease.state) {
      case 'EXPIRED':
      case 'RELEASED':
        result.expired += 1;
        break;
      case 'INIT':
      case 'SELECTING':
      case 'REQUESTING':
        result.pending += 1;
        break;
      case 'DECLINED':
      case 'CONFLICT':
      case 'ABANDONED':
        result.failed += 1;
        break;
      default:
        break;
    }
  });
  return result;
};

export const useLeaseStore = defineStore('lease', {
  state: (): LeaseState => ({
    active: [],
    history: [],
    filters: { page: 1, pageSize: 20 },
    historyFilters: { page: 1, pageSize: 20 },
    stats: { active: 0, expired: 0, pending: 0, failed: 0 },
    loadingActive: false,
    loadingHistory: false,
    activePagination: { page: 1, pageSize: 20, total: 0 },
    historyPagination: { page: 1, pageSize: 20, total: 0 },
    ...loadPersisted()
  }),
  actions: {
    async fetchActive(params?: Partial<LeaseFilter>) {
      this.loadingActive = true;
      try {
        if (params) this.filters = { ...this.filters, ...params } as LeaseFilter;
        const query = { ...this.filters } as LeaseFilter;
        const { data } = await listActiveLeases(query);
        const page = data.data;
        this.active = page.items;
        this.activePagination = {
          page: page.page ?? query.page ?? 1,
          pageSize: page.pageSize ?? query.pageSize ?? 20,
          total: page.total ?? 0
        };
        const breakdown = summarize(page.items);
        this.stats = {
          active: page.total ?? 0,
          expired: breakdown.expired,
          pending: breakdown.pending,
          failed: breakdown.failed
        };
        persist(this);
      } finally {
        this.loadingActive = false;
      }
    },
    async fetchHistory(params?: Partial<LeaseFilter>) {
      this.loadingHistory = true;
      try {
        if (params) this.historyFilters = { ...this.historyFilters, ...params } as LeaseFilter;
        const query = { ...this.historyFilters } as LeaseFilter;
        const { data } = await listHistoryLeases(query);
        const page = data.data;
        this.history = page.items;
        this.historyPagination = {
          page: page.page ?? query.page ?? 1,
          pageSize: page.pageSize ?? query.pageSize ?? 20,
          total: page.total ?? 0
        };
        persist(this);
      } finally {
        this.loadingHistory = false;
      }
    },
    resetFilters() {
      this.filters = { page: 1, pageSize: 20 } as LeaseFilter;
      this.historyFilters = { page: 1, pageSize: 20 } as LeaseFilter;
      this.activePagination = { page: 1, pageSize: 20, total: 0 };
      this.historyPagination = { page: 1, pageSize: 20, total: 0 };
      persist(this);
    },
    reset() {
      this.$reset();
      persist(this);
    }
  }
});
