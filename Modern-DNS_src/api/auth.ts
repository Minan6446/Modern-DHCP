import request from './request'
import type { AppUser } from '../types/modules'

const REFRESH_TOKEN_KEY = 'modern-dns-refresh-token'

export type LoginPayload = {
  username: string
  password: string
  totpCode?: string
}

// Login can resolve to one of two shapes:
//   1. Normal success — token, refreshToken, user (action absent)
//   2. action === 'ENROLL_MFA' — global MFA is required and this account
//      hasn't enrolled yet; the server returns a short-lived enrollToken
//      the client must present to /auth/totp/setup and /auth/totp/verify
//      before being granted real session tokens.
export type LoginResult = {
  code: number
  message: string
  data: ({
    token: string
    refreshToken?: string
    user: AppUser
    action?: undefined
  } | {
    action: 'ENROLL_MFA'
    enrollToken: string
    message?: string
  }) | null
}

export type TotpSetupResult = {
  code: number
  message: string
  data: { secret: string; otpauth: string } | null
}

export type TotpVerifyResult = {
  code: number
  message: string
  // On enrollment-token verify the backend ALSO returns access tokens so the
  // client transitions directly into a logged-in session.
  data: ({
    enabled: true
    token?: string
    refreshToken?: string
    user?: AppUser
  }) | null
}

export type RefreshTokenResult = {
  code: number
  message: string
  data: {
    token: string
  } | null
}

export const loginByPassword = (payload: LoginPayload): Promise<LoginResult> => {
  return (request({
    url: '/auth/login',
    method: 'post',
    data: payload,
    skipAuthRefresh: true,
  } as any) as Promise<LoginResult>).then((res) => {
    // Only persist refresh token when the response is a normal-login success.
    // ENROLL_MFA responses contain enrollToken in `data` but no refreshToken;
    // we deliberately keep that token in memory only so it can't be reused.
    if (res.code === 0 && res.data && 'token' in res.data && res.data.refreshToken) {
      localStorage.setItem(REFRESH_TOKEN_KEY, res.data.refreshToken)
    }
    return res
  })
}

// Both setup and verify accept either an access token (logged-in user
// voluntarily enabling MFA) or a short-lived enrollment token (mandatory
// first-login enrolment). The caller passes whichever it has.
export const totpSetup = (bearer: string): Promise<TotpSetupResult> => {
  return request({
    url: '/auth/totp/setup',
    method: 'post',
    headers: { Authorization: `Bearer ${bearer}` },
    skipAuthRefresh: true,
    useExplicitAuth: true,
  } as any) as Promise<TotpSetupResult>
}

export const totpVerify = (bearer: string, code: string): Promise<TotpVerifyResult> => {
  return (request({
    url: '/auth/totp/verify',
    method: 'post',
    headers: { Authorization: `Bearer ${bearer}` },
    data: { code },
    skipAuthRefresh: true,
    useExplicitAuth: true,
  } as any) as Promise<TotpVerifyResult>).then((res) => {
    if (res.code === 0 && res.data?.refreshToken) {
      localStorage.setItem(REFRESH_TOKEN_KEY, res.data.refreshToken)
    }
    return res
  })
}

export const refreshTokenApi = async (): Promise<RefreshTokenResult> => {
  const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY)
  if (!refreshToken) {
    return { code: 401, message: 'refresh token 不存在', data: null }
  }
  return request({
    url: '/auth/refresh',
    method: 'post',
    headers: { Authorization: `Bearer ${refreshToken}` },
    skipAuthRefresh: true,
  } as any) as Promise<RefreshTokenResult>
}
