import { httpClient } from '@/shared/api-client/http';
import type { ApiResponse, PageQuery, PageResult } from '@/types/system';
import { normalizePageResult } from '@/shared/api-client/page';

export interface DhcpOptionDTO {
  id?: string;
  name: string;
  code: number;
  scope?: string;
  format?: string;
  dataType?: string;
  value: string;
  valueExample?: string;
  allowedValues?: string[];
  sampleValue?: string;
  description?: string;
  tags?: string[];
  updatedAt?: string;
}

export interface DhcpOptionPayload
  extends Partial<Omit<DhcpOptionDTO, 'id' | 'updatedAt' | 'allowedValues'>> {
  allowedValues?: string[];
}

export interface DhcpTemplateDTO {
  id: string;
  name: string;
  description?: string;
  icon?: string;
  options: DhcpOptionDTO[];
  updatedAt?: string;
}

export interface DhcpTemplatePayload {
  name: string;
  description?: string;
  icon?: string;
  options: DhcpOptionDTO[];
}

export interface DhcpScopeDTO {
  id?: string;
  name: string;
  subnet?: string;
  range?: string;
  gateway?: string;
  status?: 'active' | 'inactive';
  scopeType?: string;
  target?: string;
  templateId?: string;
  optionIds?: string[];
  description?: string;
  notes?: string;
  updatedAt?: string;
}

export interface DhcpScopePayload
  extends Partial<Omit<DhcpScopeDTO, 'id' | 'updatedAt' | 'optionIds'>> {
  optionIds?: string[];
}

export const fetchDhcpOptions = (params?: Partial<PageQuery>) =>
  httpClient
    .get<ApiResponse<PageResult<DhcpOptionDTO> | DhcpOptionDTO[]>>('/dhcp/options', {
      params
    })
    .then((response) => {
      const normalized = normalizePageResult<DhcpOptionDTO>(response.data, params);
      return { ...response, data: { ...response.data, data: normalized.data } };
    });

export const createDhcpOption = (payload: DhcpOptionPayload) =>
  httpClient.post<ApiResponse<DhcpOptionDTO>>('/dhcp/options', payload);

export const updateDhcpOption = (id: string, payload: DhcpOptionPayload) =>
  httpClient.put<ApiResponse<DhcpOptionDTO>>(`/dhcp/options/${id}`, payload);

export const deleteDhcpOption = (id: string) =>
  httpClient.delete<ApiResponse<void>>(`/dhcp/options/${id}`);

export const fetchDhcpTemplates = (params?: Partial<PageQuery> & { keyword?: string }) =>
  httpClient
    .get<ApiResponse<PageResult<DhcpTemplateDTO> | DhcpTemplateDTO[]>>('/dhcp/templates', {
      params
    })
    .then((response) => {
      const normalized = normalizePageResult<DhcpTemplateDTO>(response.data, params);
      return { ...response, data: { ...response.data, data: normalized.data } };
    });

export const createDhcpTemplate = (payload: DhcpTemplatePayload) =>
  httpClient.post<ApiResponse<DhcpTemplateDTO>>('/dhcp/templates', payload);

export const updateDhcpTemplate = (id: string, payload: DhcpTemplatePayload) =>
  httpClient.put<ApiResponse<DhcpTemplateDTO>>(`/dhcp/templates/${id}`, payload);

export const deleteDhcpTemplate = (id: string) =>
  httpClient.delete<ApiResponse<void>>(`/dhcp/templates/${id}`);

export const applyDhcpTemplate = (id: string) =>
  httpClient.post<ApiResponse<void | DhcpOptionDTO[]>>(`/dhcp/templates/${id}/apply`);

export const syncDhcpTemplate = (id: string, mode: 'replace' | 'merge' = 'replace') =>
  httpClient.post<ApiResponse<{ templateId: string; mode: string; updatedScopes: number }>>(
    `/dhcp/templates/${id}/sync`,
    { mode }
  );

export const fetchDhcpScopes = (
  params?: Partial<PageQuery> & { keyword?: string; scopeType?: string; templateId?: string }
) =>
  httpClient
    .get<ApiResponse<PageResult<DhcpScopeDTO> | DhcpScopeDTO[]>>('/dhcp/scopes', {
      params
    })
    .then((response) => {
      const normalized = normalizePageResult<DhcpScopeDTO>(response.data, params);
      return { ...response, data: { ...response.data, data: normalized.data } };
    });

export const createDhcpScope = (payload: DhcpScopePayload) =>
  httpClient.post<ApiResponse<DhcpScopeDTO>>('/dhcp/scopes', payload);

export const updateDhcpScope = (id: string, payload: DhcpScopePayload) =>
  httpClient.put<ApiResponse<DhcpScopeDTO>>(`/dhcp/scopes/${id}`, payload);

export const deleteDhcpScope = (id: string) =>
  httpClient.delete<ApiResponse<void>>(`/dhcp/scopes/${id}`);
