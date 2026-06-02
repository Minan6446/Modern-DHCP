import type { ApiKey, ApiResponse, PageQuery, PageResult } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';

export const listApiKeys = (params: PageQuery) =>
  httpClient.get<ApiResponse<PageResult<ApiKey>>>('/auth/api-keys', { params });

export const createApiKey = (payload: Partial<ApiKey>) =>
  httpClient.post<ApiResponse<ApiKey>>('/auth/api-keys', payload);

export const revokeApiKey = (id: string) =>
  httpClient.delete<ApiResponse<void>>(`/auth/api-keys/${id}`);
