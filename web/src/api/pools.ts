import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type {
  PoolSummary,
  PoolOverviewStats,
  SubnetDraft,
  PrefixDelegation,
  BindingConflictResponse
} from '@/types/pool';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';

const DEFAULT_LEASE_PROFILE_ID = 'default-lease-profile';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

const toPoolSummary = (raw: any): PoolSummary => {
  const cidr = String(raw?.cidr || raw?.CIDR || '');
  const version = cidr.includes(':') ? 6 : 4;
  return {
    id: String(raw?.id || ''),
    name: String(raw?.name || ''),
    version,
    cidr,
    allocated: Number(raw?.allocated ?? 0),
    capacity: Number(raw?.capacity ?? 0),
    utilization: Number(raw?.utilization ?? 0),
    status: (raw?.status as PoolSummary['status']) || 'active',
    vlanId: raw?.vlanId ?? raw?.VLANID,
    tenantId: raw?.tenantId
  };
};

const mapAllocationMode = (value?: string) => {
  const normalized = String(value || '').toUpperCase();
  if (normalized === 'SEQUENTIAL') return 'sequential';
  if (normalized === 'ROUND_ROBIN') return 'round-robin';
  if (normalized === 'PRIORITY_WEIGHTED') return 'random';
  return 'round-robin';
};

const normalizeExclusions = (exclusions: any): string[] => {
  if (!Array.isArray(exclusions)) return [];
  return exclusions
    .map((entry) => {
      const start = String(entry?.start || '').trim();
      const end = String(entry?.end || '').trim();
      if (!start && !end) return '';
      if (!end || start === end) return start || end;
      return `${start}-${end}`;
    })
    .filter(Boolean);
};

const toSubnetDraft = (raw: any): SubnetDraft => ({
  name: String(raw?.name || ''),
  cidr: String(raw?.cidr || raw?.CIDR || ''),
  gateway: raw?.gateway,
  option43: raw?.option43,
  dns: Array.isArray(raw?.dns) ? raw?.dns : [],
  leaseTime: Number(raw?.leaseTime ?? 3600),
  maxLeaseTime: Number(raw?.maxLeaseTime ?? 7200),
  exclude: normalizeExclusions(raw?.exclusions),
  rangeStart: raw?.rangeStart ?? raw?.range_start,
  rangeEnd: raw?.rangeEnd ?? raw?.range_end,
  strategy: {
    mode: mapAllocationMode(raw?.allocationMode || raw?.allocation_mode)
  },
  vlanId: raw?.vlanId ?? raw?.VLANID,
  location: raw?.location,
  tags: Array.isArray(raw?.tags) ? raw?.tags : []
});

const normalizePoolResponse = (data: any): ApiResponse<PoolSummary> => {
  if (data && typeof data === 'object' && 'code' in data && 'data' in data) {
    const payload = (data as ApiResponse<any>).data;
    return {
      ...(data as ApiResponse<any>),
      data: toPoolSummary(payload)
    } as ApiResponse<PoolSummary>;
  }
  return { code: 0, message: '', data: toPoolSummary(data) } as ApiResponse<PoolSummary>;
};

const toAllocationMode = (mode?: string) => {
  if (mode === 'sequential') return 'SEQUENTIAL';
  if (mode === 'round-robin') return 'ROUND_ROBIN';
  if (mode === 'random') return 'PRIORITY_WEIGHTED';
  return 'SEQUENTIAL';
};

const toExclusions = (exclude: string[] = []) =>
  exclude
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => {
      const [start, end] = entry.split('-').map((part) => part.trim());
      return { start: start || end, end: end || start };
    })
    .filter((item) => item.start && item.end);

export const listPools = async (
  params: PageQuery & { version?: 4 | 6; tenantId?: string },
  signal?: AbortSignal
) => {
  // Use relative path so baseURL (e.g., /api/v1) prefixes correctly
  const response = await httpClient.get<
    ApiResponse<PageResult<PoolSummary>> | PageResult<PoolSummary> | any[]
  >('pools', { params, signal });
  const normalized = normalizePageResult<any>(response.data, params);
  const mapped = Array.isArray(normalized.data.items)
    ? normalized.data.items.map(toPoolSummary)
    : [];
  const items = params.version ? mapped.filter((item) => item.version === params.version) : mapped;
  // Preserve server-provided total so pagination reflects full result set instead of current page length.
  const total = normalized.data.total;
  return { ...response, data: { ...normalized, data: { ...normalized.data, items, total } } };
};

const toPoolOverviewStats = (raw: any): PoolOverviewStats => ({
  total: Number(raw?.total ?? 0),
  enabledCount: Number(raw?.enabledCount ?? 0),
  avgUsage: Number(raw?.avgUsage ?? 0),
  highUsageCount: Number(raw?.highUsageCount ?? 0)
});

export const getPoolStats = async (
  params: { version?: 4 | 6; keyword?: string; status?: string; tenantId?: string },
  signal?: AbortSignal
) => {
  const response = await httpClient.get<ApiResponse<PoolOverviewStats> | PoolOverviewStats>('pools/stats', {
    params,
    signal
  });
  const payload = (response.data as any)?.data ?? response.data;
  const data = toPoolOverviewStats(payload);
  return {
    ...response,
    data: {
      code: 0,
      message: '',
      data
    } as ApiResponse<PoolOverviewStats>
  };
};

export const getPool = async (id: string, params?: { tenantId?: string }) => {
  const response = await httpClient.get<ApiResponse<SubnetDraft> | any>(`pools/${id}`, { params });
  const data = response.data;
  const normalized =
    data && typeof data === 'object' && 'code' in data && 'data' in data
      ? (data as ApiResponse<SubnetDraft>)
      : ({ code: 0, message: '', data: toSubnetDraft(data) } as ApiResponse<SubnetDraft>);
  return { ...response, data: normalized };
};

export const createPool = (
  payload: SubnetDraft & { tenantId?: string; version?: 4 | 6; leaseProfileId?: string }
) =>
  (() => {
    const body = {
      name: payload.name,
      cidr: payload.cidr,
      gateway: payload.gateway,
      option43: payload.option43,
      dns: payload.dns ?? [],
      rangeStart: payload.rangeStart,
      rangeEnd: payload.rangeEnd,
      vlanId: payload.vlanId,
      location: payload.location,
      exclusions: toExclusions(payload.exclude),
      allocationMode: toAllocationMode(payload.strategy?.mode),
      leaseTime: Number(payload.leaseTime ?? 3600),
      maxLeaseTime: Number(payload.maxLeaseTime ?? 7200),
      leaseProfileId: payload.leaseProfileId || DEFAULT_LEASE_PROFILE_ID
    };
    console.warn('[pool] create body', JSON.parse(JSON.stringify(body)));
    return httpClient.post<ApiResponse<PoolSummary> | any>('pools', body);
  })().then((response) => ({ ...response, data: normalizePoolResponse(response.data) }));

export const updatePool = (
  id: string,
  payload: Partial<SubnetDraft> & { tenantId?: string; version?: 4 | 6; status?: PoolSummary['status'] }
) =>
  (() => {
    const body = {
      name: payload.name,
      cidr: payload.cidr,
      gateway: payload.gateway,
      option43: payload.option43,
      dns: payload.dns ?? [],
      rangeStart: payload.rangeStart,
      rangeEnd: payload.rangeEnd,
      vlanId: payload.vlanId,
      location: payload.location,
      exclusions: toExclusions(payload.exclude || []),
      allocationMode: toAllocationMode(payload.strategy?.mode),
      leaseTime: payload.leaseTime != null ? Number(payload.leaseTime) : undefined,
      maxLeaseTime: payload.maxLeaseTime != null ? Number(payload.maxLeaseTime) : undefined,
      leaseProfileId: payload.leaseProfileId || DEFAULT_LEASE_PROFILE_ID,
      status: payload.status
    };
    console.warn('[pool] update body', JSON.parse(JSON.stringify(body)));
    const params = payload.tenantId ? { params: { tenantId: payload.tenantId } } : undefined;
    return httpClient.patch<ApiResponse<PoolSummary> | any>(`pools/${id}`, body, params);
  })().then((response) => ({ ...response, data: normalizePoolResponse(response.data) }));

export const deletePool = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`pools/${id}`, { params });

// Backend currently exposes no usage/history endpoints for pools; return empty shapes to avoid 404 noise.
export const getUsage = async (
  id: string,
  params?: { tenantId?: string; window?: '3d' | '7d' | '30d' }
) => httpClient.get<ApiResponse<{ ts: string[]; used: number[]; capacity: number[] }>>(`pools/${id}/usage`, { params });

export const getHistory = async (
  id: string,
  params: PageQuery & {
    tenantId?: string;
    state?: string;
    identifier?: string;
    ip?: string;
    from?: string;
    to?: string;
  }
) =>
  httpClient.get<ApiResponse<PageResult<{ id: string; action: string; actor: string; ts: string }>>>(
    `pools/${id}/history`,
    { params }
  );

// Backend lacks a bindings conflict endpoint; return empty conflicts to keep UI stable.
export const getConflicts = async (id: string, params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<BindingConflictResponse>>(`pools/${id}/conflicts`, { params });

// Prefix delegation endpoint is not available server-side; return empty list to keep UI stable.
export const listPrefixDelegations = async (params: PageQuery & { tenantId?: string }) =>
  resp<PageResult<PrefixDelegation>>({
    items: [] as PrefixDelegation[],
    total: 0,
    page: params.page,
    pageSize: params.pageSize,
    hasMore: false
  } as PageResult<PrefixDelegation>);

export type PoolImportResult = {
  total: number;
  success: number;
  failed: number;
  errors?: { row?: number; message?: string; error?: string }[];
};

export const exportPoolsCsv = (params?: { version?: 4 | 6 }) =>
  httpClient.get<Blob>('pools/export', { params, responseType: 'blob' });

export const importPoolsCsv = (file: File, params?: { version?: 4 | 6 }) => {
  const form = new FormData();
  form.append('file', file);
  return httpClient.post<ApiResponse<PoolImportResult> | PoolImportResult>('pools/import', form, {
    params,
    headers: { 'Content-Type': 'multipart/form-data' }
  });
};
