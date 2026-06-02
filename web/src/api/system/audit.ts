import type { ApiResponse, LoginAuditQuery, LoginAuditRecord, PageResult } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';

export const listLoginAudits = async (params: LoginAuditQuery) => {
  const { page, pageSize, ...rest } = params;
  const limit = pageSize;
  const offset = (page - 1) * pageSize;
  const response = await httpClient.get<ApiResponse<PageResult<LoginAuditRecord>>>('/login-audit', {
    params: { limit, offset, ...rest }
  });
  return response;
};
