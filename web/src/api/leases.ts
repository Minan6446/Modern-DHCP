import type { AxiosResponse } from 'axios';
import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type { Lease, LeaseEvent, LeaseFilter, LeaseState, LeaseInsights } from '@/types/lease';
import { httpClient } from '@/shared/api-client/http';
import { normalizeLeaseState } from '@/shared/utils/leaseState';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

const mapLeaseState = (lease: Lease): Lease => ({
  ...lease,
  state: normalizeLeaseState(lease.state as LeaseState)
});

const mapLeasePage = (resp: AxiosResponse<ApiResponse<PageResult<Lease>>>) => {
  const page = (resp.data as any)?.data ?? resp.data;
  const items = page?.items?.map(mapLeaseState) ?? [];
  const total = page?.total ?? 0;
  return { ...resp, data: { ...resp.data, data: { ...page, items, total } } };
};

export const listActiveLeases = async (params: PageQuery & LeaseFilter, _signal?: AbortSignal) =>
  resp<PageResult<Lease>>({
    items: [] as Lease[],
    total: 0,
    page: params.page,
    pageSize: params.pageSize,
    hasMore: false
  } as PageResult<Lease>);

export const listHistoryLeases = async (params: PageQuery & LeaseFilter, _signal?: AbortSignal) =>
  resp<PageResult<Lease>>({
    items: [] as Lease[],
    total: 0,
    page: params.page,
    pageSize: params.pageSize,
    hasMore: false
  } as PageResult<Lease>);

export const getLeaseEvents = async (
  _leaseId: string,
  _params?: { tenantId?: string },
  _signal?: AbortSignal
) => resp<LeaseEvent[]>([]);

export const releaseLease = async (leaseId: string, params?: { tenantId?: string }) =>
  httpClient.post<ApiResponse<void>>(`/core/leases/${leaseId}/release`, undefined, { params });

export const renewLease = async (_leaseId: string, _params?: { tenantId?: string }) =>
  resp<void>(undefined);

export const exportLeases = async (
  _params: LeaseFilter & { format: 'csv' | 'xlsx' | 'pdf' },
  _signal?: AbortSignal
) => new Blob();

export const getLeaseInsights = (params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<LeaseInsights>>('/leases/insights', { params });
