import type {
  ApiResponse,
  PageQuery,
  PageResult,
  User,
  UserPayload,
  UserStatusPayload
} from '@/types/system';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';

// Backend user management lives under /auth/users; align paths.
const prefix = '/auth/users';

export const listUsers = async (params: PageQuery & { status?: string }) => {
  const response = await httpClient.get<ApiResponse<PageResult<User> | User[]>>(prefix, {
    params: {
      limit: params.pageSize,
      offset: (params.page - 1) * params.pageSize,
      q: params.keyword,
      status: params.status
    }
  });
  const normalized = normalizePageResult<User>(response.data, params);
  const items = normalized.data.items;
  const total = Number(normalized.data.total ?? items.length);

  return { ...response, data: { ...response.data, data: { items, total } } };
};

export const createUser = (payload: UserPayload) =>
  httpClient.post<ApiResponse<User>>(prefix, payload);

export const updateUser = (id: string, payload: UserPayload) =>
  httpClient.put<ApiResponse<User>>(`${prefix}/${id}`, payload);

export const getUserRoles = (id: string) =>
  httpClient.get<ApiResponse<string[]>>(`${prefix}/${id}/roles`);

export const replaceUserRoles = (id: string, roles: string[]) =>
  httpClient.put<ApiResponse<string[]>>(`${prefix}/${id}/roles`, { roles });

export const updateUserStatus = (id: string, payload: UserStatusPayload) =>
  httpClient.put<ApiResponse<User>>(`${prefix}/${id}`, payload);

export const getUserPermissions = (id: string) =>
  httpClient.get<ApiResponse<string[]>>(`${prefix}/${id}/permissions`);

export const deleteUser = (id: string) => httpClient.delete<ApiResponse<void>>(`${prefix}/${id}`);
