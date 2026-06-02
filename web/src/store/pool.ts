import { defineStore } from 'pinia';
import { listPools } from '@/api/pools';
import type { Pool } from '@/types/pool';

interface PoolState {
  pools: Pool[];
  currentPoolId: string;
  usage: Record<string, number>;
  history: Array<{ poolId: string; action: string; at: string }>;
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-pool';

const loadPersisted = (): Partial<PoolState> => {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (e) {
    return {};
  }
};

const persist = (state: PoolState) => {
  sessionStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ currentPoolId: state.currentPoolId, history: state.history })
  );
};

export const usePoolStore = defineStore('pool', {
  state: (): PoolState => ({
    pools: [],
    currentPoolId: '',
    usage: {},
    history: [],
    loading: false,
    ...loadPersisted()
  }),
  getters: {
    currentPool: (state: PoolState): Pool | null =>
      state.pools.find((p) => p.id === state.currentPoolId) || null
  },
  actions: {
    async fetch() {
      this.loading = true;
      try {
        const { data } = await listPools({ page: 1, pageSize: 200 });
        this.applyList(data.data.items);
      } finally {
        this.loading = false;
      }
    },
    applyList(pools: Pool[]) {
      this.pools = pools;
      if (!this.pools.find((p) => p.id === this.currentPoolId)) {
        this.currentPoolId = this.pools.length ? this.pools[0].id : '';
      }
      persist(this);
    },
    upsert(pool: Pool) {
      const idx = this.pools.findIndex((p) => p.id === pool.id);
      if (idx >= 0) {
        this.pools[idx] = pool;
      } else {
        this.pools = [pool, ...this.pools];
      }
      if (!this.currentPoolId) {
        this.currentPoolId = pool.id;
      }
      persist(this);
    },
    select(poolId: string) {
      this.currentPoolId = poolId;
      this.history = [
        { poolId, action: 'select', at: new Date().toISOString() },
        ...this.history
      ].slice(0, 20);
      persist(this);
    },
    setUsage(poolId: string, utilization: number) {
      this.usage[poolId] = utilization;
    },
    addHistory(action: string) {
      if (!this.currentPoolId) return;
      this.history = [
        { poolId: this.currentPoolId, action, at: new Date().toISOString() },
        ...this.history
      ].slice(0, 20);
      persist(this);
    },
    reset() {
      this.$reset();
      persist(this);
    }
  }
});
