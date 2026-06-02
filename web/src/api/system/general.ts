import { httpClient } from '@/shared/api-client/http';

export interface OpsSystemSummary {
  enabled: boolean;
  allowConfigExport: boolean;
  allowConfigImport: boolean;
  backupLocation?: string;
  maintenanceWindow?: string;
  theme: string;
  locale: string;
  maintenanceMode: boolean;
  announcement?: string;
  adminSessionTimeoutMinutes: number;
  autoLogoutEnabled: boolean;
  systemLogRetentionDays: number;
  auditLogRetentionDays: number;
  logPushEnabled: boolean;
  logPushEndpoint?: string;
  logPushMinLevel?: string;
  logPushChannels?: string;
  logPushPhones?: string;
  logPushDingTalkEndpoint?: string;
  logPushFeishuEndpoint?: string;
  logPushWecomEndpoint?: string;
  logPushSlackEndpoint?: string;
  ntpEnabled: boolean;
  ntpServers?: string;
  ntpIntervalMinutes: number;
  ntpTimeoutSeconds: number;
  timezone?: string;
  ntpSyncStatus?: string;
  ntpLastSyncAt?: string;
  updatedAt: string;
  updatedBy: string;
  generatedAt: string;
}

export interface OpsNtpUpdatePayload {
  ntpEnabled?: boolean;
  ntpServers?: string;
  ntpIntervalMinutes?: number;
  ntpTimeoutSeconds?: number;
  timezone?: string;
}

export interface OpsSessionPolicyPayload {
  adminSessionTimeoutMinutes?: number;
  autoLogoutEnabled?: boolean;
}

export interface OpsLogPolicyPayload {
  systemLogRetentionDays?: number;
  auditLogRetentionDays?: number;
  logPushEnabled?: boolean;
  logPushEndpoint?: string;
  logPushMinLevel?: string;
  logPushChannels?: string;
  logPushPhones?: string;
  logPushDingTalkEndpoint?: string;
  logPushFeishuEndpoint?: string;
  logPushWecomEndpoint?: string;
  logPushSlackEndpoint?: string;
}

export interface OpsGeneralSettingsPayload extends OpsNtpUpdatePayload, OpsSessionPolicyPayload, OpsLogPolicyPayload {
  locale?: string;
}

export const getSystemGeneralSettings = () => httpClient.get<OpsSystemSummary>('/ops/system');

export const updateSystemNtpSettings = (payload: OpsNtpUpdatePayload) =>
  httpClient.patch<OpsSystemSummary>('/ops/settings/ntp', payload);

export const updateSystemLocaleSetting = (payload: { locale: string }) =>
  httpClient.patch<OpsSystemSummary>('/ops/settings/locale', payload);

export const updateSystemSessionPolicy = (payload: OpsSessionPolicyPayload) =>
  httpClient.put<OpsSystemSummary>('/ops/system', payload);

export const updateSystemLogPolicy = (payload: OpsLogPolicyPayload) =>
  httpClient.put<OpsSystemSummary>('/ops/system', payload);

export const updateSystemGeneralSettings = (payload: OpsGeneralSettingsPayload) =>
  httpClient.put<OpsSystemSummary>('/ops/system', payload);

export const syncSystemNtpNow = () => httpClient.post<OpsSystemSummary>('/ops/settings/ntp/sync');
