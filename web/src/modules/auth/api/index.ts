import type { AxiosResponse } from 'axios';
import type { ApiResponse } from '@/types/system';
import type {
  LoginCaptchaResponse,
  LoginRequest,
  LoginResponse,
  PasswordResetCompleteRequest,
  PasswordResetStartRequest,
  PasswordResetStartResponse,
  PasswordResetVerifyRequest
} from '@/modules/auth/types';
import { httpClient } from '@/shared/api-client/http';

const prefix = import.meta.env.VITE_AUTH_PREFIX || '/auth';

// Login + session APIs follow backend shape (token/refreshToken/expiresAt)
export const login = (payload: LoginRequest) =>
  httpClient.post<LoginResponse>(`${prefix}/login`, payload);

export const logout = () => httpClient.post<void>(`${prefix}/logout`);

export const refreshToken = (refreshToken: string) =>
  httpClient.post<LoginResponse>(`${prefix}/session/refresh`, { refreshToken });

export const getLoginCaptcha = (username?: string) =>
  httpClient.get<LoginCaptchaResponse>(`${prefix}/captcha`, {
    params: username ? { username } : undefined
  });

export const startPasswordReset = (payload: PasswordResetStartRequest) =>
  httpClient.post<PasswordResetStartResponse>(`${prefix}/password/reset/start`, payload);

export const verifyPasswordResetCode = (payload: PasswordResetVerifyRequest) =>
  httpClient.post<{ verified: boolean; challengeId: string }>(`${prefix}/password/reset/verify`, payload);

export const completePasswordReset = (payload: PasswordResetCompleteRequest) =>
  httpClient.post<{ success: boolean }>(`${prefix}/password/reset/complete`, payload);

// UI capabilities endpoint (provides granted permission keys)
export const getCapabilities = () =>
  httpClient.get<ApiResponse<{ capabilities: { granted: string[] } }> | { capabilities?: { granted?: string[] } }>(
    '/ui'
  );

export const extractGrantedCapabilities = (
  response: AxiosResponse<
    ApiResponse<{ capabilities: { granted: string[] } }> | { capabilities?: { granted?: string[] } }
  >
) => {
  const payload: any = response?.data;
  const fromEnvelope = payload?.data?.capabilities?.granted;
  if (Array.isArray(fromEnvelope)) {
    return fromEnvelope.map((item) => String(item || '').trim()).filter(Boolean);
  }
  const fromRaw = payload?.capabilities?.granted;
  if (Array.isArray(fromRaw)) {
    return fromRaw.map((item: unknown) => String(item || '').trim()).filter(Boolean);
  }
  return [] as string[];
};
