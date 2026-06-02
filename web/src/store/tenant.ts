import { defineStore } from 'pinia';
import { getTenantQuotas } from '@/api/system/tenant';
import type { TenantDetail, TenantQuotaDetail } from '@/types/system';
import { loadState, saveState, removeState } from './persist';

interface TenantState {
  currentTenantId: string;
  tenants: TenantDetail[];
  quota: TenantQuotaDetail | null;
  history: string[];
  loading: boolean;
}

const STORAGE_KEY = 'mdhcp-tenant';

const normalizeTenantId = (value?: string) => {
  const trimmed = value?.trim();
  if (!trimmed || trimmed === 'default') return 'global';
  return trimmed;
};

const loadPersisted = (): Partial<TenantState> => {
  const persisted = loadState<TenantState>(STORAGE_KEY);
  if (!persisted) return persisted;
  const current = normalizeTenantId(persisted.currentTenantId);
  const history = (persisted.history || []).map((id) => normalizeTenantId(id));
  return { ...persisted, currentTenantId: current, history };
};

const persist = (state: TenantState) => {
  if (state.currentTenantId || state.history.length) {
    saveState(STORAGE_KEY, { currentTenantId: state.currentTenantId, history: state.history });
  } else {
    removeState(STORAGE_KEY);
  }
};

const SYSTEM_TENANT = {
  id: 'global',
  name: '全局空间'
} as TenantDetail;

export const useTenantStore = defineStore('tenant', {
  state: (): TenantState => ({
    currentTenantId: SYSTEM_TENANT.id,
    tenants: [SYSTEM_TENANT],
    quota: null,
    history: [],
    loading: false,
    ...loadPersisted()
  }),
  getters: {
    currentTenant: (state) => state.tenants.find((t) => t.id === state.currentTenantId) || null
  },
  actions: {
    async loadTenants() {
      this.loading = true;
      try {
        this.tenants = [SYSTEM_TENANT];
        this.currentTenantId = normalizeTenantId(SYSTEM_TENANT.id);
        this.history = [this.currentTenantId];
        persist(this);
      } finally {
        this.loading = false;
      }
    },
    async loadQuota(id?: string) {
      const tid = normalizeTenantId(id || this.currentTenantId || SYSTEM_TENANT.id);
      if (!tid) return;
      const { data } = await getTenantQuotas(tid);
      this.quota = data.data;
    },
    switchTenant(id: string) {
      this.currentTenantId = normalizeTenantId(id || SYSTEM_TENANT.id);
      this.history = [this.currentTenantId];
      persist(this);
    },
    reset() {
      this.$reset();
      this.currentTenantId = normalizeTenantId(SYSTEM_TENANT.id);
      this.tenants = [SYSTEM_TENANT];
      removeState(STORAGE_KEY);
    }
  }
});
