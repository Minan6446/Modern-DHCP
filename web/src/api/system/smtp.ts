import type { ApiResponse } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';

export type SmtpConfig = {
  host: string;
  port: number;
  username: string;
  password: string;
  from: string;
  useTLS: boolean;
  testRecipient?: string;
};

export type SmsConfig = {
  provider: string;
  accessKeyId: string;
  accessKeySecret: string;
  signName: string;
  templateCode: string;
  testPhone: string;
};

export type WebhookConfig = {
  url: string;
  method: string;
  token: string;
  headerKey: string;
  payload: string;
  headers?: Record<string, string>;
  channels?: string[];
  dingTalkUrl?: string;
  feishuUrl?: string;
  wecomUrl?: string;
  slackUrl?: string;
  phoneNumbers?: string;
};

export const getSmtpConfig = () =>
  httpClient.get<ApiResponse<SmtpConfig>>('/core/notifications/smtp');

export const saveSmtpConfig = (payload: SmtpConfig) =>
  httpClient.post<ApiResponse<void>>('/core/notifications/smtp', payload);

export const sendSmtpTest = (payload: SmtpConfig & { to: string }) =>
  httpClient.post<ApiResponse<void>>('/core/notifications/smtp/test', payload);

export const getSmsConfig = () => httpClient.get<ApiResponse<SmsConfig>>('/core/notifications/sms');

export const saveSmsConfig = (payload: SmsConfig) =>
  httpClient.post<ApiResponse<void>>('/core/notifications/sms', payload);

export const sendSmsTest = () => httpClient.post<ApiResponse<void>>('/core/notifications/sms/test');

export const getWebhookConfig = () =>
  httpClient.get<ApiResponse<WebhookConfig>>('/core/notifications/webhook');

export const saveWebhookConfig = (payload: WebhookConfig) =>
  httpClient.post<ApiResponse<void>>('/core/notifications/webhook', payload);

export const sendWebhookTest = () =>
  httpClient.post<ApiResponse<void>>('/core/notifications/webhook/test');
