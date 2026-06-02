import type { ApiResponse, PageQuery, PageResult, TenantDetail } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';

// Backend does not expose tenant CRUD endpoints; provide stubs to avoid 404s.
const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

export const listTenants = async (params: PageQuery) =>
  resp<PageResult<TenantDetail>>({
    items: [] as TenantDetail[],
    total: 0,
    page: params.page,
    pageSize: params.pageSize,
    hasMore: false
  } as PageResult<TenantDetail>);

export const getTenant = async (_id: string) => resp<TenantDetail>({} as TenantDetail);

export const createTenant = async (_payload: Partial<TenantDetail>) =>
  resp<TenantDetail>({} as TenantDetail);

export const updateTenant = async (_id: string, _payload: Partial<TenantDetail>) =>
  resp<TenantDetail>({} as TenantDetail);

export const deleteTenant = async (_id: string) => resp<void>(undefined);
