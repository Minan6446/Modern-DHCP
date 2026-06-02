import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type { Binding, BindingFilter } from '@/types/binding';
import { httpClient } from '@/shared/api-client/http';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

export const listBindings = (params: (PageQuery & BindingFilter) & { tenantId?: string }) =>
  httpClient.get<ApiResponse<PageResult<Binding>>>('/core/bindings', { params });

export const createBinding = (payload: Partial<Binding> & { tenantId?: string }) => {
  const { tenantId, ...body } = payload;
  return httpClient.post<ApiResponse<Binding>>(
    '/core/bindings',
    { ...body, tenantId },
    { params: { tenantId } }
  );
};

export const updateBinding = (id: string, payload: Partial<Binding> & { tenantId?: string }) => {
  const { tenantId, ...body } = payload;
  return httpClient.patch<ApiResponse<Binding>>(
    `/core/bindings/${id}`,
    { ...body, tenantId },
    { params: { tenantId } }
  );
};

export const deleteBinding = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/core/bindings/${id}`, { params });

export const batchImportBindings = (
  items: Array<Partial<Binding>>,
  params?: { tenantId?: string }
) =>
  httpClient.post<ApiResponse<{ success: number; failed: number; errors?: string[] }>>(
    '/core/bindings/batch',
    { items },
    { params }
  );

export const batchDeleteBindings = (filter: BindingFilter & { tenantId?: string }) =>
  httpClient.post<ApiResponse<{ deleted: number }>>('/core/bindings/batch-delete', filter);

// Backend has no conflicts endpoint; provide empty result to avoid 404s.
export const getConflicts = async (_params: BindingFilter & { tenantId?: string }) =>
  resp<{ count: number; items: Binding[] }>({ count: 0, items: [] });

export const getBinding = (id: string, params?: { tenantId?: string }) =>
  httpClient.get<ApiResponse<Binding>>(`/core/bindings/${id}`, { params });
