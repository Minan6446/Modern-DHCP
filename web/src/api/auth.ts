import type { ApiResponse, User } from '@/types/system';
import { httpClient } from '@/shared/api-client/http';
import { useAuthStore } from '@/modules/auth/store';

export interface LoginPayload {
  username: string;
  password: string;
  type: 'local' | 'ldap' | 'sso';
  captcha?: string;
}

export interface LoginResult {
  accessToken: string;
  refreshToken: string;
  user: User;
}

export const login = (payload: LoginPayload) =>
  httpClient.post<ApiResponse<LoginResult>>('/auth/login', payload);

export const logout = () => httpClient.post<ApiResponse<void>>('/auth/logout');

export const refreshToken = (token: string) =>
  httpClient.post<ApiResponse<{ accessToken: string }>>('/auth/session/refresh', {
    refreshToken: token
  });

export const changePassword = (payload: {
  currentPassword: string;
  newPassword: string;
  username?: string;
}) => {
  const auth = useAuthStore();
  const username = payload.username || auth.user?.username || auth.lastUsername || '';
  return httpClient.post<ApiResponse<void>>('/auth/password', { ...payload, username });
};
