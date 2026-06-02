import type {
  ApiResponse,
  PageQuery,
  PageResult,
  PermissionNode,
  RoleDetail
} from '@/types/system';
import { httpClient } from '@/shared/api-client/http';
import { buildRolePermissionTree } from '@/shared/rbacPermissionMap';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

export const listRoles = (params: PageQuery) =>
  httpClient.get<ApiResponse<PageResult<RoleDetail>>>('/rbac/roles', { params });

export const getRole = (id: string) => httpClient.get<ApiResponse<RoleDetail>>(`/rbac/roles/${id}`);

export const createRole = (payload: Partial<RoleDetail>) =>
  httpClient.post<ApiResponse<RoleDetail>>('/rbac/roles', payload);

export const updateRole = (id: string, payload: Partial<RoleDetail>) =>
  httpClient.put<ApiResponse<RoleDetail>>(`/rbac/roles/${id}`, payload);

export const deleteRole = (id: string) => httpClient.delete<ApiResponse<void>>(`/rbac/roles/${id}`);

const getCandidateApiBases = async () => {
  const bases = new Set<string>();
  const proxyTarget = (import.meta.env.VITE_API_PROXY_TARGET as string | undefined)?.replace(
    /\/$/,
    ''
  );
  if (proxyTarget) {
    bases.add(`${proxyTarget}/api/v1`);
    bases.add(`${proxyTarget}/api/v2`);
  }

  const envBase = import.meta.env.VITE_API_BASE_URL as string | undefined;
  const normalizedEnv = envBase ? envBase.replace(/\/$/, '') : '';
  if (normalizedEnv) bases.add(normalizedEnv);

  if (normalizedEnv.endsWith('/api/v1')) bases.add('/api/v2');
  if (normalizedEnv.endsWith('/api/v2')) bases.add('/api/v1');

  try {
    const versionsUrl =
      typeof window !== 'undefined'
        ? new URL('/api/versions', window.location.origin).toString()
        : '/api/versions';
    const versionsResp = await httpClient.get<ApiResponse<string[]>>(versionsUrl);
    const versions = (versionsResp.data as any)?.data || (versionsResp.data as any)?.versions || [];
    if (Array.isArray(versions)) {
      versions
        .map((v) => String(v || '').trim())
        .filter(Boolean)
        .forEach((v) => bases.add(`/api/${v}`));
    }
  } catch (_) {
    // ignore
  }

  if (!bases.size) bases.add('/api/v1');
  return Array.from(bases.values());
};

export const getPermissionTree = async () => {
  const parseCaps = (payload: any) => {
    if (Array.isArray(payload?.capabilities)) return payload.capabilities;
    if (Array.isArray(payload?.data?.capabilities)) return payload.data.capabilities;
    return [] as string[];
  };

  const endpoints = ['/rbac/capabilities', '/auth/capabilities'];
  const bases = await getCandidateApiBases();
  let lastError: unknown = null;

  for (const base of bases) {
    for (const endpoint of endpoints) {
      try {
        const response = await httpClient.get<
          { capabilities?: string[] } & ApiResponse<{ capabilities?: string[] }>
        >(endpoint, { baseURL: base });
        return resp<PermissionNode[]>(buildRolePermissionTree(parseCaps(response.data)));
      } catch (err) {
        const status = (err as any)?.response?.status;
        lastError = err;
        if (status && status !== 404) throw err;
      }
    }
  }

  if (lastError) throw lastError;
  return resp<PermissionNode[]>([]);
};

export const assignRolePermissions = async (_id: string, _keys: string[]) => resp<void>(undefined);

export const assignUserRoles = async (_userId: string, _roleIds: string[]) => resp<void>(undefined);
