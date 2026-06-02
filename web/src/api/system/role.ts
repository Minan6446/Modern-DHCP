import type { ApiResponse, PageQuery, PageResult, RoleDetail, RolePayload } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';
import {
  capabilitiesToMenuSelection,
  menuSelectionToCapabilities
} from '@/shared/rbacPermissionMap';

// Backend roles are under /rbac/roles; align paths.
const prefix = '/rbac/roles';
const assignmentPrefix = '/rbac/assignments';

export interface RbacAssignmentDTO {
  id: string;
  principalId: string;
  roleName: string;
  createdBy?: string;
  createdAt?: string;
}

const normalizeRole = (raw: any): RoleDetail => ({
  id: String(raw?.id || ''),
  name: String(raw?.name || ''),
  description: raw?.description ? String(raw.description) : undefined,
  scope: (raw?.scope as RoleDetail['scope']) || 'tenant',
  permissions: capabilitiesToMenuSelection(
    (Array.isArray(raw?.capabilities)
      ? raw.capabilities
      : Array.isArray(raw?.permissions)
        ? raw.permissions
        : []) as string[]
  ),
  createdAt: raw?.createdAt
});

export const listRoles = async (params: PageQuery) => {
  const response = await httpClient.get<ApiResponse<PageResult<RoleDetail> | RoleDetail[]>>(
    prefix,
    { params }
  );
  const normalized = normalizePageResult<any>(response.data, params);
  const items = normalized.data.items.map(normalizeRole);
  const total = Number(normalized.data.total ?? items.length);

  return { ...response, data: { ...response.data, data: { items, total } } };
};

export const createRole = (payload: RolePayload) =>
  httpClient.post<ApiResponse<RoleDetail>>(prefix, {
    name: payload.name,
    description: payload.description,
    capabilities: menuSelectionToCapabilities(payload.permissions || [])
  });

export const updateRole = (id: string, payload: RolePayload) =>
  httpClient.put<ApiResponse<RoleDetail>>(`${prefix}/${id}`, {
    name: payload.name,
    description: payload.description,
    capabilities: menuSelectionToCapabilities(payload.permissions || [])
  });

export const deleteRole = (id: string) => httpClient.delete<ApiResponse<void>>(`${prefix}/${id}`);

export const listAssignments = async (
  principalId: string,
  params?: Partial<PageQuery> & { roleName?: string }
) => {
  const response = await httpClient.get<ApiResponse<PageResult<RbacAssignmentDTO> | RbacAssignmentDTO[]>>(
    assignmentPrefix,
    {
      params: {
        principalId,
        page: params?.page ?? 1,
        pageSize: params?.pageSize ?? 200,
        roleName: params?.roleName
      }
    }
  );
  const normalized = normalizePageResult<RbacAssignmentDTO>(response.data, {
    page: Number(params?.page ?? 1),
    pageSize: Number(params?.pageSize ?? 200)
  });
  return { ...response, data: { ...response.data, data: normalized.data } };
};

export const createAssignment = (payload: {
  principalId: string;
  roleName: string;
  orgUnitId?: string;
  resourceType?: string;
  resourceId?: string;
}) => httpClient.post<ApiResponse<RbacAssignmentDTO>>(assignmentPrefix, payload);

export const deleteAssignment = (assignmentId: string) =>
  httpClient.delete<ApiResponse<void>>(`${assignmentPrefix}/${assignmentId}`);

export const assignRoleUsers = async (roleId: string, userIds: string[]) => {
  const roleName = String(roleId || '').trim();
  if (!roleName) return;
  for (const userId of userIds) {
    const principalId = `user:${String(userId || '').trim()}`;
    if (!principalId || principalId === 'user:') continue;
    const { data } = await listAssignments(principalId, { page: 1, pageSize: 200, roleName });
    const exists = (data.data.items || []).some((item) => item.roleName === roleName);
    if (!exists) {
      await createAssignment({ principalId, roleName });
    }
  }
};
