import axios, { AxiosHeaders } from 'axios'
import type { AxiosError } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '../types/api'
import i18n from '../i18n'
import {
  handleRefreshToken,
  isTokenRefreshing,
  queueRequestUntilTokenRefreshed,
} from './interceptors/refreshToken'
import type { RetryableRequestConfig } from './interceptors/refreshToken'
import { confirmSensitive, SensitiveCancelled } from './sensitiveConfirm'

const TOKEN_KEY = 'modern-dns-token'

const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

request.interceptors.request.use(
  async (config) => {
    const retryableConfig = config as RetryableRequestConfig

    if (isTokenRefreshing() && !retryableConfig.skipAuthRefresh) {
      return queueRequestUntilTokenRefreshed(retryableConfig)
    }

    const headers = AxiosHeaders.from(config.headers)
    headers.set('X-Demo-Frontend', 'modern-dns')
    if (retryableConfig.useExplicitAuth) {
      // Caller manages Authorization itself (e.g. MFA enrollment flow that
      // presents a short-lived enrollToken not stored in localStorage).
      // Leave whatever they set untouched.
    } else {
      const token = localStorage.getItem(TOKEN_KEY)
      if (token) {
        headers.set('Authorization', `Bearer ${token}`)
      } else {
        headers.delete('Authorization')
      }
    }
    retryableConfig.headers = headers
    return retryableConfig
  },
  (error) => Promise.reject(error),
)

request.interceptors.response.use(
  (response) => response.data,
  async (error: AxiosError<ApiResponse<unknown>>) => {
    let replayedResponse: unknown | null = null
    try {
      replayedResponse = await handleRefreshToken(error, request)
    } catch (refreshError) {
      return Promise.reject(refreshError)
    }
    if (replayedResponse !== null) {
      return replayedResponse
    }

    // ── Sensitive operation step-up auth ──
    // Backend's SensitiveConfirm middleware returns code === 4401 with a
    // `requireConfirm: true` data flag when the request hits a destructive
    // route without the X-Confirm-Password / X-Confirm-TOTP header. Pop a
    // dialog, attach the operator's proof, and retry once. Concurrent
    // failed requests share one dialog (see sensitiveConfirm.ts).
    const body = error.response?.data as ApiResponse<{ requireConfirm?: boolean; mfaEnabled?: boolean }> | undefined
    if (body?.code === 4401 && body.data?.requireConfirm) {
      const config = error.config as RetryableRequestConfig | undefined
      if (config && !config._sensitiveRetried) {
        try {
          const headers = await confirmSensitive({
            mfaEnabled: body.data.mfaEnabled === true,
          })
          config._sensitiveRetried = true
          const retryHeaders = AxiosHeaders.from(config.headers)
          if (headers.password) retryHeaders.set('X-Confirm-Password', headers.password)
          if (headers.totp)     retryHeaders.set('X-Confirm-TOTP', headers.totp)
          config.headers = retryHeaders
          return request(config)
        } catch (cancelErr) {
          if (cancelErr instanceof SensitiveCancelled) {
            // Silent reject — no toast, the operator chose to back out.
            return Promise.reject(cancelErr)
          }
          throw cancelErr
        }
      }
    }

    const t = i18n.global.t
    const responseMessage = (error.response?.data as ApiResponse<unknown> | undefined)?.message

    let msg = ''
    if (responseMessage && !/^ok$/i.test(responseMessage)) {
      msg = responseMessage
    } else if (error.code === 'ECONNABORTED') {
      msg = t('common.requestTimeout')
    } else if (error.message === 'Network Error') {
      msg = t('common.networkError')
    } else {
      switch (error.response?.status) {
        case 401:
          msg = t('common.unauthorized')
          break
        case 403:
          msg = t('common.forbidden')
          break
        case 404:
          msg = t('common.notFound')
          break
        case 500:
        case 502:
        case 503:
        case 504:
          msg = t('common.serverError')
          break
        default:
          msg = error.message || t('common.requestFailed')
          break
      }
    }

    ElMessage.error(msg)
    return Promise.reject(error)
  },
)

export { request }

export default request