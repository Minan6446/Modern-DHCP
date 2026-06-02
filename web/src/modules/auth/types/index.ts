import type { RoleSummary, TenantSummary, User } from '@/types/system';

export enum AuthErrorCode {
  Unauthorized = 401,
  Locked = 423,
  TooManyRequests = 429,
  TokenExpired = 498
}

export interface LoginRequest {
  username: string;
  password: string;
  tenantId?: string;
  remember?: boolean;
  captchaId?: string;
  captchaCode?: string;
}

export interface LoginCaptchaResponse {
  captchaId: string;
  captchaCode: string;
  expiresAt: string;
}

export interface PasswordResetStartRequest {
  username: string;
  email: string;
}

export interface PasswordResetStartResponse {
  challengeId: string;
  expiresAt: string;
}

export interface PasswordResetVerifyRequest {
  challengeId: string;
  code: string;
}

export interface PasswordResetCompleteRequest {
  challengeId: string;
  newPassword: string;
}

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

export interface LoginProfile {
  id: string;
  displayName: string;
  role: string;
  tenantId: string;
}

export interface LoginResponse {
  token?: string;
  refreshToken?: string;
  expiresAt?: string;
  profile?: LoginProfile;
  lastLoginAt?: string;
  abnormalLogin?: boolean;
  // Legacy fields for compatibility
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
  user_info?: User;
}

export const isTokenPair = (value: unknown): value is TokenPair => {
  if (!value || typeof value !== 'object') return false;
  const v = value as Partial<TokenPair>;
  return typeof v.accessToken === 'string' && typeof v.refreshToken === 'string';
};

export type { User, RoleSummary, TenantSummary };
