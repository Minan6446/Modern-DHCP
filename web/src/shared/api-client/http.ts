import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig
} from 'axios';
import { useAuthStore } from '@/modules/auth/store';
import { useTenantStore } from '@/store/tenant';
import type { ApiResponse } from './types';
import { AuthErrorCode } from '@/modules/auth/types';
import { describeAxiosError, setApiError } from '@/shared/errors/apiError';
import type { ApiErrorDescriptor } from '@/shared/errors/apiError';
import { createRequestCache } from '@/shared/http/cache';
import { retryAsync } from '@/shared/http/retry';
import { createSingleFlight } from '@/shared/http/queue';
import { i18n } from '@/i18n';
import router from '@/router';
import { showError } from '@/shared/errors/messageToast';

type CacheableRequestConfig = InternalAxiosRequestConfig & { _cacheKey?: string };

const inFlightCache = createRequestCache<AxiosResponse>(2000);
const refreshQueue = createSingleFlight<string | null>();
const { t } = i18n.global;
let clusterFencingEpoch = '';

const isClusterWriteRequest = (config: AxiosRequestConfig) => {
  const method = (config.method || 'get').toLowerCase();
  if (!['post', 'put', 'patch', 'delete'].includes(method)) return false;
  const rawUrl = String(config.url || '').trim();
  if (!rawUrl) return false;
  const withoutOrigin = rawUrl.replace(/^https?:\/\/[^/]+/i, '');
  const path = withoutOrigin.split('?')[0].replace(/^\/+/, '');
  return (
    path.startsWith('cluster') ||
    path.startsWith('ha') ||
    path.startsWith('ops/system') ||
    path.startsWith('ops/settings') ||
    path.startsWith('pools') ||
    path.startsWith('core/pools') ||
    path.startsWith('api/v1/ops/system') ||
    path.startsWith('api/v1/ops/settings') ||
    path.startsWith('api/v1/pools') ||
    path.startsWith('api/v1/core/pools')
  );
};

const extractFencingEpoch = (payload: unknown): string => {
  if (!payload || typeof payload !== 'object') return '';
  const obj = payload as Record<string, unknown>;
  const direct = String(obj.fencingEpoch || '').trim();
  if (direct) return direct;
  const nested = obj.data;
  if (nested && typeof nested === 'object') {
    const nestedEpoch = String((nested as Record<string, unknown>).fencingEpoch || '').trim();
    if (nestedEpoch) return nestedEpoch;
  }
  return '';
};

const extractExpectedEpoch = (payload: unknown, fallbackMessage = ''): string => {
  const fromText = (text: string) => {
    const match = /expectedEpoch\s*[:=]\s*([0-9]+)/i.exec(text || '');
    return match?.[1] || '';
  };

  if (payload && typeof payload === 'object') {
    const obj = payload as Record<string, unknown>;
    const direct = String(obj.expectedEpoch || '').trim();
    if (direct) return direct;
    const fromMessage = fromText(String(obj.message || '').trim());
    if (fromMessage) return fromMessage;
    const fromError = fromText(String(obj.error || '').trim());
    if (fromError) return fromError;
    const nested = obj.data;
    if (nested && typeof nested === 'object') {
      const nestedEpoch = String((nested as Record<string, unknown>).expectedEpoch || '').trim();
      if (nestedEpoch) return nestedEpoch;
      const nestedMessage = fromText(String((nested as Record<string, unknown>).message || '').trim());
      if (nestedMessage) return nestedMessage;
    }
  }
  const message = fallbackMessage || (typeof payload === 'string' ? payload : '');
  return fromText(message);
};

const isStaleFencingEpoch = (payload: unknown, message = '') => {
  const textParts: string[] = [message];
  if (typeof payload === 'string') {
    textParts.push(payload);
  } else if (payload && typeof payload === 'object') {
    const obj = payload as Record<string, unknown>;
    textParts.push(String(obj.message || ''));
    textParts.push(String(obj.error || ''));
    const nested = obj.data;
    if (nested && typeof nested === 'object') {
      const nestedObj = nested as Record<string, unknown>;
      textParts.push(String(nestedObj.message || ''));
      textParts.push(String(nestedObj.error || ''));
      textParts.push(String(nestedObj.code || ''));
    }
  }
  const text = textParts.join(' ').toLowerCase();
  if (text.includes('stale_fencing_epoch')) return true;
  if (payload && typeof payload === 'object') {
    const obj = payload as Record<string, unknown>;
    const code = String(obj.code || '').toLowerCase();
    if (code === 'stale_fencing_epoch') return true;
    const nested = obj.data;
    if (nested && typeof nested === 'object') {
      const nestedCode = String((nested as Record<string, unknown>).code || '').toLowerCase();
      if (nestedCode === 'stale_fencing_epoch') return true;
    }
  }
  return false;
};

const makeKey = (config: AxiosRequestConfig) => {
  const { method = 'get', url = '', params, data } = config;
  if ((method || 'get').toLowerCase() !== 'get') return '';
  return `${method}:${url}:${JSON.stringify(params || {})}:${JSON.stringify(data || {})}`;
};

const addAuthHeader = <TConfig extends AxiosRequestConfig>(config: TConfig, token: string) => {
  if (!token) return config;
  return {
    ...config,
    headers: {
      ...(config.headers || {}),
      Authorization: `Bearer ${token}`
    }
  } satisfies AxiosRequestConfig;
};

const shouldRetry = (
  descriptor: ApiErrorDescriptor | null,
  original: CacheableRequestConfig & { _retry?: boolean }
) => {
  const retriable = !!descriptor?.retryable && !original._retry;
  if (retriable) {
    original._retry = true;
    return true;
  }
  return false;
};

const reissueWithToken = (
  instance: AxiosInstance,
  original: CacheableRequestConfig,
  token: string
) => {
  const next = addAuthHeader(original, token);
  return instance(next);
};

const handleUnauthorized = async (
  error: AxiosError,
  instance: AxiosInstance,
  original: CacheableRequestConfig & { _retry?: boolean }
) => {
  const auth = useAuthStore();
  if (!auth.refreshToken || original._retry) return null;
  if (original.url?.includes('/auth/refresh')) {
    await auth.logout();
    showError(t('errors.auth.sessionExpired'));
    const current = router.currentRoute.value;
    const redirect = current?.path && current.path !== '/login' ? current.fullPath : undefined;
    await router.push({ path: '/login', query: redirect ? { redirect } : undefined });
    return Promise.reject(error);
  }

  original._retry = true;
  const token = await refreshQueue.run(async () => {
    await auth.refresh();
    return auth.accessToken || null;
  });

  if (!token) {
    await auth.logout();
    showError(t('errors.auth.sessionExpired'));
    const current = router.currentRoute.value;
    const redirect = current?.path && current.path !== '/login' ? current.fullPath : undefined;
    await router.push({ path: '/login', query: redirect ? { redirect } : undefined });
    return Promise.reject(error);
  }

  return reissueWithToken(instance, original, token);
};

const hasCsrfCookie = () => {
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some((part) => part.trim().startsWith('csrf_token='));
};

const handleSuccess = (instance: AxiosInstance, resp: AxiosResponse) => {
  // Some auth responses return a CSRF token in the body; set header and a non-HttpOnly cookie fallback for dev HTTP
  const csrfToken = (resp.data as { csrfToken?: string } | undefined)?.csrfToken;
  if (csrfToken) {
    instance.defaults.headers.common['X-CSRF-Token'] = csrfToken;
    if (!hasCsrfCookie() && typeof document !== 'undefined') {
      document.cookie = `csrf_token=${csrfToken}; path=/; SameSite=Lax`;
    }
  }

  const auditStatus = (resp.headers?.['x-audit-status'] || resp.headers?.['X-Audit-Status']) as
    | string
    | undefined;
  if (auditStatus && auditStatus.toLowerCase() === 'failed') {
    showError(t('system.audit.recordFailed', '审计记录写入失败，已降级为非阻断。'));
  }

  const withCache = resp.config as CacheableRequestConfig;
  const fromHeader =
    String(resp.headers?.['x-fencing-epoch'] || resp.headers?.['X-Fencing-Epoch'] || '').trim();
  const fromBody = extractFencingEpoch(resp.data);
  const nextEpoch = fromHeader || fromBody;
  if (nextEpoch) {
    clusterFencingEpoch = nextEpoch;
  }
  const key = makeKey(withCache) || withCache._cacheKey;
  if (key && (resp.config.method || 'get').toLowerCase() === 'get') {
    inFlightCache.set(key, resp);
  }
  return resp;
};

const handleError = async (error: AxiosError, instance: AxiosInstance) => {
  const auth = useAuthStore();
  const status = error.response?.status;
  const message =
    (error.response?.data as ApiResponse<unknown> | undefined)?.message || error.message;
  const original = (error.config || {}) as CacheableRequestConfig & {
    _retry?: boolean;
    _fencingRetryCount?: number;
  };
  if (original._cacheKey) inFlightCache.delete(original._cacheKey as string);

  const descriptor = describeAxiosError(error, message);
  if (descriptor) {
    setApiError(error, descriptor);
  }

  if (
    status === 409 &&
    (original._fencingRetryCount || 0) < 2 &&
    isClusterWriteRequest(original) &&
    isStaleFencingEpoch(error.response?.data, message)
  ) {
    const expectedEpoch = extractExpectedEpoch(error.response?.data, message);
    if (expectedEpoch) {
      clusterFencingEpoch = expectedEpoch;
      const headers = { ...(original.headers as Record<string, string> | undefined) };
      headers['X-Fencing-Epoch'] = expectedEpoch;
      original.headers = headers as InternalAxiosRequestConfig['headers'];
      original._fencingRetryCount = (original._fencingRetryCount || 0) + 1;
      return instance(original);
    }
  }

  if (shouldRetry(descriptor, original)) {
    return retryAsync(() => instance(original), {
      retries: 1,
      baseDelay: 300,
      retryOn: (err) => {
        const s = (err as AxiosError)?.response?.status;
        return !s || s >= 500;
      }
    });
  }

  if (status === AuthErrorCode.Unauthorized) {
    const retry = await handleUnauthorized(error, instance, original);
    if (retry) return retry;
    if (auth.accessToken) {
      await auth.logout();
      const current = router.currentRoute.value;
      const redirect = current?.path && current.path !== '/login' ? current.fullPath : undefined;
      await router.push({ path: '/login', query: redirect ? { redirect } : undefined });
    }
  }

  if (status === 404) {
    const base = (original.baseURL || import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '');
    const path = original.url || '';
    const full = base && path ? `${base}${path.startsWith('/') ? '' : '/'}${path}` : path || base;
    const normalizedPath = String(path).split('?')[0].replace(/^https?:\/\/[^/]+/i, '');
    const isOptionalSyncEndpoint =
      normalizedPath === '/ops/settings/ntp/sync' ||
      normalizedPath.endsWith('/ops/settings/ntp/sync') ||
      normalizedPath === '/api/v1/ops/settings/ntp/sync';
    if (!isOptionalSyncEndpoint) {
      const hint = 'API not found. Check base URL (/api/v1) and path (e.g. /core/pools).';
      const detail = full ? `${hint} URL=${full}` : hint;
      showError(detail);
    }
    return Promise.reject(error);
  }

  const isAuthLogin = (original.url || '').includes('/auth/login');
  const shouldToast = !isAuthLogin && (status !== AuthErrorCode.Unauthorized || !auth.accessToken);
  if (shouldToast) {
    const fallback = message || t('errors.generic', 'Request failed, please try again later.');
    showError(descriptor?.message || fallback);
  }

  return Promise.reject(error);
};

export const createHttpClient = (): AxiosInstance => {
  const instance = axios.create({
    baseURL: import.meta.env.VITE_API_BASE_URL,
    timeout: 15000,
    withCredentials: true,
    // Backend issues csrf_token cookie and expects X-CSRF-Token header
    xsrfCookieName: 'csrf_token',
    xsrfHeaderName: 'X-CSRF-Token'
  });

  instance.interceptors.request.use((config) => {
    const auth = useAuthStore();
    const tenant = useTenantStore();
    const headers = (config.headers || {}) as Record<string, string>;
    if (auth.accessToken) headers.Authorization = `Bearer ${auth.accessToken}`;
    if (tenant.currentTenantId) {
      const tenantHeader = import.meta.env.VITE_TENANT_HEADER || 'X-Tenant-Id';
      headers[tenantHeader] = tenant.currentTenantId;
    }
    if (isClusterWriteRequest(config) && clusterFencingEpoch && !headers['X-Fencing-Epoch']) {
      headers['X-Fencing-Epoch'] = clusterFencingEpoch;
    }
    config.headers = headers as InternalAxiosRequestConfig['headers'];
    // Avoid returning cached AxiosResponses as configs to prevent stray baseURL-only requests (e.g., hitting /api/v1).
    // If needed, lightweight response caching can be reintroduced safely at the adapter layer.
    return config;
  });

  instance.interceptors.response.use(
    (resp) => handleSuccess(instance, resp),
    (error) => handleError(error, instance)
  );

  return instance;
};

export const httpClient = createHttpClient();
