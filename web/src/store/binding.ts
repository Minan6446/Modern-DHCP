import { defineStore } from 'pinia';
import { listBindings } from '@/api/bindings';
import type { Binding, BindingFilter, BindingStatus } from '@/types/binding';
import type { PageQuery } from '@/types/system';
import { showError } from '@/shared/errors/messageToast';

interface PaginationState {
  page: number;
  pageSize: number;
  total: number;
}

type BindingFiltersInput = Partial<
  Omit<PageQuery, 'status'> & BindingFilter & { status?: BindingStatus[]; tenantId?: string }
>;

interface BindingState {
  rows: Binding[];
  loading: boolean;
  filters: BindingFiltersInput;
  pagination: PaginationState;
  stats: { total: number; online: number; warning: number; offline: number };
  selection: string[];
}

const STORAGE_KEY = 'mdhcp-binding';

const loadPersisted = (): Partial<BindingState> => {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (error) {
    return {};
  }
};

const persist = (state: BindingState) => {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ filters: state.filters }));
};

let lastErrorAt = 0;
const notifyOnce = (message: string, cooldownMs = 3000) => {
  const now = Date.now();
  if (now - lastErrorAt < cooldownMs) return;
  lastErrorAt = now;
  showError(message);
};

export const useBindingStore = defineStore('binding', {
  state: (): BindingState => ({
    rows: [],
    loading: false,
    filters: { page: 1, pageSize: 20 },
    pagination: { page: 1, pageSize: 20, total: 0 },
    stats: { total: 0, online: 0, warning: 0, offline: 0 },
    selection: [],
    ...loadPersisted()
  }),
  actions: {
    async fetchList(params?: BindingFiltersInput) {
      this.loading = true;
      try {
        if (params) this.filters = { ...this.filters, ...params };
        const { status, ...rest } = this.filters;
        const normalizedStatus =
          Array.isArray(status) && status.length ? status.join(',') : undefined;
        const normalizeEmpty = (v: any) => (v === '' ? undefined : v);
        const query = {
          page: rest.page ?? 1,
          pageSize: rest.pageSize ?? 20,
          mac: normalizeEmpty(rest.mac),
          ip: normalizeEmpty(rest.ip),
          hostname: normalizeEmpty(rest.hostname),
          status: normalizedStatus,
          tenantId: rest.tenantId || 'global'
        } as PageQuery & BindingFilter & { tenantId?: string };
        const { data } = await listBindings(query);
        // Normalize possible response shapes: {data:{items}}, {items}, or raw array
        const payload: any = (data as any)?.data ?? data;
        let items: Binding[] = [];
        if (Array.isArray(payload)) {
          items = payload as Binding[];
        } else if (Array.isArray(payload?.items)) {
          items = payload.items as Binding[];
        } else if (payload && typeof payload === 'object' && 'id' in payload) {
          // Backend might return a single object instead of a list; wrap it.
          items = [payload as Binding];
        }

        const parseMetadata = (raw: any): Record<string, unknown> => {
          if (!raw) return {};
          if (typeof raw === 'string') {
            try {
              return JSON.parse(raw) as Record<string, unknown>;
            } catch {
              return {};
            }
          }
          if (typeof raw === 'object') return raw as Record<string, unknown>;
          return {};
        };

        const mapped = items.map((item: any) => {
          const metadata = parseMetadata(item.metadata);
          const hostname =
            (metadata.hostname as string) ||
            (metadata.host as string) ||
            (metadata.fqdn as string) ||
            item.hostname ||
            '';
          const description =
            (metadata.description as string) ||
            (metadata.desc as string) ||
            item.description ||
            '';
          const options =
            (metadata.options as Record<string, string>) ||
            (metadata.dhcpOptions as Record<string, string>) ||
            item.options ||
            undefined;

          return {
            ...item,
            metadata,
            hostname,
            description,
            options,
            // Backend may return identifier/ipAddress instead of mac/ip; normalize for UI usage.
            mac: item.mac || (item as any).identifier || '',
            ip: item.ip || (item as any).ipAddress || '',
            lastSeenAt: item.lastSeenAt || (item as any).last_seen_at || undefined,
            statusSource: item.statusSource || (item as any).status_source || undefined,
            poolId: item.poolId || (item as any).poolID || ''
          };
        });

        // If response is just a health payload like {status:'ok',version:'v1'}, keep existing rows.
        if (mapped.length === 0 && payload && payload.status === 'ok' && payload.version) {
          this.rows = this.rows.length ? [...this.rows] : [];
        } else {
          this.rows = mapped;
        }
        const total =
          payload && typeof payload === 'object' && 'total' in payload
            ? Number((payload as any).total) || 0
            : this.rows.length;
        this.pagination = {
          page: query.page ?? 1,
          pageSize: query.pageSize ?? 20,
          total
        };
        const statsFromApi =
          payload && typeof payload === 'object' && 'stats' in payload
            ? (payload as any).stats || {}
            : {};
        const stats = { total, online: 0, warning: 0, offline: 0 } as Record<string, number>;
        // Prefer backend stats when available.
        ['online', 'warning', 'offline', 'total'].forEach((k) => {
          const v = Number((statsFromApi as any)?.[k]) || 0;
          if (v > 0) stats[k] = v;
        });
        // Fallback to page-local aggregation when backend omitted specific counts.
        if (!statsFromApi || typeof statsFromApi !== 'object') {
          this.rows.forEach((row) => {
            if (row.status === 'online') stats.online += 1;
            else if (row.status === 'warning') stats.warning += 1;
            else stats.offline += 1;
          });
        }
        this.stats = {
          total: stats.total,
          online: stats.online,
          warning: stats.warning,
          offline: stats.offline
        };
        persist(this);
      } catch (error) {
        this.rows = [];
        this.pagination = { page: 1, pageSize: this.pagination.pageSize, total: 0 };
        this.stats = { total: 0, online: 0, warning: 0, offline: 0 };
        notifyOnce('请求失败，请稍后重试');
      } finally {
        this.loading = false;
      }
    },
    setFilters(payload: BindingFiltersInput) {
      this.filters = { ...this.filters, ...payload };
      persist(this);
    },
    resetFilters() {
      this.filters = { page: 1, pageSize: 20, tenantId: this.filters.tenantId };
      this.pagination = { page: 1, pageSize: 20, total: 0 };
      persist(this);
    },
    setSelection(ids: string[]) {
      this.selection = ids;
    },
    reset() {
      this.rows = [];
      this.loading = false;
      this.filters = { page: 1, pageSize: 20 };
      this.pagination = { page: 1, pageSize: 20, total: 0 };
      this.stats = { total: 0, online: 0, warning: 0, offline: 0 };
      this.selection = [];
      persist(this as BindingState);
    }
  }
});
