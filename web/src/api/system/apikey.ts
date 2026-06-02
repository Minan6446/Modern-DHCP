import type { ApiKey, ApiResponse, ApiKeyPayload } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';

// Backend API keys live under /auth/api-keys; align paths.
const prefix = '/auth/api-keys';

export const listApiKeys = (includeRevoked = false) =>
  httpClient.get<ApiResponse<ApiKey[]>>(prefix, { params: { includeRevoked } });

export const createApiKey = (payload: ApiKeyPayload) =>
  httpClient.post<ApiResponse<ApiKey>>(prefix, payload);

export interface ApiKeyUpdatePayload {
  displayName?: string;
  role?: string;
  capabilities?: string[];
  description?: string;
  ipWhitelist?: string[];
  expiresAt?: string;
  enabled?: boolean;
}

export const updateApiKey = (id: string, payload: ApiKeyUpdatePayload) =>
  httpClient.patch<ApiResponse<ApiKey>>(`${prefix}/${id}`, payload);

export const revokeApiKey = (id: string) => httpClient.delete<ApiResponse<void>>(`${prefix}/${id}`);
