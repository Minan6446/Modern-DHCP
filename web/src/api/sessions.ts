import type { ApiResponse, PageQuery, PageResult, SessionInfo } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';

export const listSessions = async (params: PageQuery) => {
  const page = Math.max(1, Number(params.page || 1));
  const pageSize = Math.max(1, Number(params.pageSize || 20));
  const response = await httpClient.get<ApiResponse<PageResult<SessionInfo>> | PageResult<SessionInfo>>(
    '/auth/sessions',
    {
      params: {
        limit: pageSize,
        offset: (page - 1) * pageSize,
        q: params.keyword,
        status: params.status
      }
    }
  );
  return { data: normalizePageResult<SessionInfo>(response.data, { page, pageSize }) };
};

export const getSession = async (_id: string) => {
  const response = await httpClient.get<ApiResponse<PageResult<SessionInfo>> | PageResult<SessionInfo>>(
    '/auth/sessions',
    { params: { limit: 200, offset: 0 } }
  );
  const normalized = normalizePageResult<SessionInfo>(response.data, { page: 1, pageSize: 200 });
  const found = normalized.data.items.find((item) => item.id === _id);
  return {
    data: {
      code: 0,
      message: '',
      data: found as SessionInfo
    }
  } as { data: ApiResponse<SessionInfo> };
};

export const forceLogout = async (id: string) =>
  httpClient.delete<ApiResponse<void>>(`/auth/sessions/${encodeURIComponent(id)}`);
