import type {
  ApiResponse,
  PageQuery,
  PageResult,
  TenantDetail,
  TenantPayload,
  TenantQuotaDetail
} from '@/types/system';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

// Backend tenant management endpoints are not exposed; provide no-op stubs to prevent 404s.
export const listTenants = async (params: PageQuery) =>
  resp<PageResult<TenantDetail>>({
    items: [] as TenantDetail[],
    total: 0,
    page: params.page,
    pageSize: params.pageSize,
    hasMore: false
  } as PageResult<TenantDetail>);

export const createTenant = async (payload: TenantPayload) =>
  resp<TenantDetail>(payload as TenantDetail);

export const updateTenant = async (_id: string, payload: TenantPayload) =>
  resp<TenantDetail>(payload as TenantDetail);

export const deleteTenant = async (_id: string) => resp<void>(undefined);

export const getTenantQuotas = async (_id: string) =>
  resp<TenantQuotaDetail>({
    pools: 0,
    leases: 0,
    users: 0,
    dhcpOptions: 0,
    bindings: 0,
    alerts: 0
  });
