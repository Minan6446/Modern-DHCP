import { defineStore } from 'pinia';
import { listUsers } from '@/api/system/user';
import type { PageQuery, User } from '@/types/system';

interface UserState {
  users: User[];
  total: number;
  filters: Partial<PageQuery & { status?: string }>;
  bulkSelection: string[];
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-users';

const loadPersisted = (): Partial<UserState> => {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (e) {
    return {};
  }
};

const persist = (state: UserState) => {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ filters: state.filters }));
};

export const useUserStore = defineStore('user', {
  state: (): UserState => ({
    users: [],
    total: 0,
    filters: { page: 1, pageSize: 20 },
    bulkSelection: [],
    loading: false,
    ...loadPersisted()
  }),
  getters: {
    selectedCount: (state) => state.bulkSelection.length
  },
  actions: {
    async fetch(params?: Partial<PageQuery & { status?: string }>) {
      this.loading = true;
      try {
        if (params) this.filters = { ...this.filters, ...params };
        const { data } = await listUsers(this.filters as PageQuery);
        this.users = data.data.items;
        this.total = data.data.total;
        persist(this);
      } finally {
        this.loading = false;
      }
    },
    setSelection(ids: string[]) {
      this.bulkSelection = ids;
    },
    resetFilters() {
      this.filters = { page: 1, pageSize: 20 };
      persist(this);
    },
    reset() {
      this.$reset();
      persist(this);
    }
  }
});
