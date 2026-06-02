import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import type {
  OptionDefinition,
  OptionTemplate,
  OptionTemplateGraphNode,
  OptionUsageSnapshot,
  OptionTestRequest,
  OptionTestResult
} from '@/types/option';
import { httpClient } from '@/shared/api-client/http';
import { normalizePageResult } from '@/shared/api-client/page';

const resp = <T>(data: T) => ({ data: { code: 0, message: '', data } }) as { data: ApiResponse<T> };

export const listStandardOptions = (params?: {
  category?: string;
  keyword?: string;
  tenantId?: string;
}) => httpClient.get<ApiResponse<OptionDefinition[]>>('/dhcp/options', { params });

export const listCustomOptions = (
  params: (PageQuery & { category?: string }) & { tenantId?: string }
) => httpClient.get<ApiResponse<PageResult<OptionDefinition>>>('/dhcp/options', { params });

export const createCustomOption = (payload: Partial<OptionDefinition> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<OptionDefinition>>('/dhcp/options', payload);

export const updateCustomOption = (
  id: string,
  payload: Partial<OptionDefinition> & { tenantId?: string }
) => httpClient.put<ApiResponse<OptionDefinition>>(`/dhcp/options/${id}`, payload);

export const deleteCustomOption = (id: string, params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/dhcp/options/${id}`, { params });

// Backend lacks these endpoints; return empty fallbacks to avoid 404 noise in UI.
export const getOptionUsage = async (_params?: { tenantId?: string }) =>
  resp<OptionUsageSnapshot>({ heat: [], relations: [], history: [] });

export const listOptionTemplates = async (params: PageQuery & { tenantId?: string }) =>
  httpClient
    .get<ApiResponse<PageResult<OptionTemplate> | OptionTemplate[]>>('/dhcp/templates', {
      params
    })
    .then((response) => {
      const normalized = normalizePageResult<OptionTemplate>(response.data, params);
      const pageData: PageResult<OptionTemplate> = {
        items: normalized.data.items,
        total: Number(normalized.data.total ?? 0),
        page: Number(params.page ?? 1),
        pageSize: Number(params.pageSize ?? 10),
        hasMore: Number(params.page ?? 1) * Number(params.pageSize ?? 10) < Number(normalized.data.total ?? 0)
      };
      return { ...response, data: { ...response.data, data: pageData } };
    });

export const createTemplate = async (payload: Partial<OptionTemplate> & { tenantId?: string }) =>
  httpClient.post<ApiResponse<OptionTemplate>>('/dhcp/templates', payload);

export const updateTemplate = async (
  id: string,
  payload: Partial<OptionTemplate> & { tenantId?: string }
) => httpClient.put<ApiResponse<OptionTemplate>>(`/dhcp/templates/${id}`, payload);

export const deleteTemplate = async (_id: string, _params?: { tenantId?: string }) =>
  httpClient.delete<ApiResponse<void>>(`/dhcp/templates/${_id}`, { params: _params });

export const getTemplateGraph = async (_params?: { tenantId?: string }) =>
  resp<OptionTemplateGraphNode[]>([]);

export const testOptionDelivery = async (_payload: OptionTestRequest & { tenantId?: string }) =>
  resp<OptionTestResult>({ offeredOptions: [], rawPacketHex: '', logs: [], latencyMs: 0 });
