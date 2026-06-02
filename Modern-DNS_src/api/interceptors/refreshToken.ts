import { AxiosHeaders } from 'axios'
import type { AxiosError, AxiosInstance, InternalAxiosRequestConfig } from 'axios'
import router from '../../router'
import i18n from '../../i18n'
import type { ApiResponse } from '../../types/api'

const TOKEN_KEY = 'modern-dns-token'
const USER_KEY = 'modern-dns-user'
const LOGIN_PATH = '/login'

export interface RetryableRequestConfig<Data = unknown> extends InternalAxiosRequestConfig<Data> {
  _retry?: boolean
  skipAuthRefresh?: boolean
  // When true, the request interceptor must keep the Authorization header
  // exactly as the caller set it (e.g. for the MFA enrollment flow that
  // presents a short-lived enrollToken instead of the stored access token).
  useExplicitAuth?: boolean
  // Set by the response interceptor after a 4401 step-up confirmation has
  // been replayed once, so a wrong password/TOTP returned 4401 a second
  // time doesn't loop forever — we surface the error normally instead.
  _sensitiveRetried?: boolean
}

type PendingRequest = {
  onSuccess: (token: string) => void
  onError: (error: unknown) => void
}

let isRefreshing = false
const pendingRequests: PendingRequest[] = []

const normalizeHeaders = <Data = unknown>(config: RetryableRequestConfig<Data>, token: string): RetryableRequestConfig<Data> => {
  const headers = AxiosHeaders.from(config.headers)
  headers.set('X-Demo-Frontend', 'modern-dns')
  headers.set('Authorization', `Bearer ${token}`)
  config.headers = headers
  return config
}

const flushPendingRequests = (token: string): void => {
  pendingRequests.splice(0).forEach((request) => request.onSuccess(token))
}

const rejectPendingRequests = (error: unknown): void => {
  pendingRequests.splice(0).forEach((request) => request.onError(error))
}

const clearAuthState = (): void => {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

const redirectToLogin = async (): Promise<void> => {
  clearAuthState()
  if (router.currentRoute.value.path !== LOGIN_PATH) {
    await router.push(LOGIN_PATH)
  }
}

const requestNewToken = async (): Promise<string> => {
  const authModule = await import('../auth')
  const response = await authModule.refreshTokenApi()

  if (response.code !== 0 || !response.data?.token) {
    throw new Error(i18n.global.t('common.tokenRefreshFailed'))
  }

  const nextToken = response.data.token
  localStorage.setItem(TOKEN_KEY, nextToken)
  return nextToken
}

/**
 * 返回当前是否处于 Token 刷新中，供 request 拦截器决定是否挂起后续请求。
 */
export const isTokenRefreshing = (): boolean => isRefreshing

/**
 * 在刷新期间挂起请求，待新 Token 获取成功后继续发送原请求。
 */
export const queueRequestUntilTokenRefreshed = <Data = unknown>(
  config: RetryableRequestConfig<Data>,
): Promise<RetryableRequestConfig<Data>> =>
  new Promise((resolve, reject) => {
    pendingRequests.push({
      onSuccess: (token) => resolve(normalizeHeaders(config, token)),
      onError: reject,
    })
  })

/**
 * 处理 401 响应。首次命中时发起刷新；刷新期间到达的 401 请求进入队列等待重放。
 */
export const handleRefreshToken = async (
  error: AxiosError<ApiResponse<unknown>>,
  instance: AxiosInstance,
): Promise<unknown | null> => {
  const config = error.config as RetryableRequestConfig | undefined

  if (!config || config.skipAuthRefresh || config._retry || error.response?.status !== 401) {
    return null
  }

  if (isRefreshing) {
    return new Promise((resolve, reject) => {
      pendingRequests.push({
        onSuccess: (token) => {
          const retryConfig = normalizeHeaders({ ...config, _retry: true }, token)
          resolve(instance(retryConfig))
        },
        onError: reject,
      })
    })
  }

  config._retry = true
  isRefreshing = true

  try {
    const nextToken = await requestNewToken()
    isRefreshing = false
    flushPendingRequests(nextToken)
    return instance(normalizeHeaders(config, nextToken))
  } catch (refreshError) {
    isRefreshing = false
    rejectPendingRequests(refreshError)
    await redirectToLogin()
    throw refreshError
  }
}
