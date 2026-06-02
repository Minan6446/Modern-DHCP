import { type AxiosError, isAxiosError } from 'axios';
import { i18n } from '@/i18n';
import type { ApiErrorDetails, ApiErrorField, ApiErrorPayload } from '@/shared/api-client/types';

const API_ERROR_PROP = '__appError';
const { t } = i18n.global;

const DEFAULT_FALLBACK = 'Request failed, please try again later.';

const BACKEND_MESSAGE_MAP: Record<string, string> = {
  'expiresAt exceeds maximum ttl': '过期时间不能超过 365 天',
  'expiresAt must be in the future': '过期时间必须晚于当前时间',
  'expiresAt must be RFC3339': '过期时间格式无效，请重新选择',
  'ownerUserId must be a valid uuid': '所属用户标识无效，请重新登录后重试',
  'auth: owner user id must be a valid uuid': '所属用户标识无效，请重新登录后重试',
  'auth: api key name is required': '请填写 API Key 名称',
  'auth: api key role is required': '请选择 API Key 角色',
  'auth: expiresAt exceeds maximum ttl': '过期时间不能超过 365 天',
  'auth: expiresAt must be in the future': '过期时间必须晚于当前时间'
};

const CODE_MESSAGES: Record<number, { key: string; fallback: string }> = {
  10001: { key: 'errors.catalog.validationFailed', fallback: 'Validation failed.' },
  20001: { key: 'errors.catalog.businessRule', fallback: 'Request blocked by a business rule.' },
  20004: { key: 'errors.catalog.resourceMissing', fallback: 'Resource not found.' },
  30001: { key: 'errors.catalog.authRequired', fallback: 'Authentication required.' },
  30003: { key: 'errors.catalog.forbidden', fallback: 'Insufficient permissions.' },
  50000: { key: 'errors.catalog.systemError', fallback: 'Service error.' }
};

const TYPE_HINTS: Record<string, { key: string; fallback: string }> = {
  ValidationError: {
    key: 'errors.hints.validation',
    fallback: 'Please fix the highlighted fields and retry.'
  },
  BusinessError: {
    key: 'errors.hints.business',
    fallback: 'Adjust the request or contact an administrator.'
  },
  AuthError: { key: 'errors.hints.auth', fallback: 'Please sign in again and retry.' },
  SystemError: {
    key: 'errors.catalog.systemError',
    fallback: 'Service error, please retry later.'
  },
  NetworkError: {
    key: 'errors.catalog.network',
    fallback: 'Network issue, please check your connection.'
  }
};

export interface ApiErrorDescriptor {
  code?: number;
  status?: number;
  type?: string;
  message: string;
  hint?: string;
  requestId?: string;
  timestamp?: string;
  fields?: ApiErrorField[];
  retryable?: boolean;
  network?: boolean;
}

interface DescriptorOptions {
  fallbackMessage?: string;
  status?: number;
  network?: boolean;
}

const resolveMessage = (code?: number, fallback?: string) => {
  if (code && CODE_MESSAGES[code]) {
    const meta = CODE_MESSAGES[code];
    return t(meta.key, meta.fallback);
  }
  if (fallback && fallback.trim().length > 0) {
    const trimmed = fallback.trim();
    if (BACKEND_MESSAGE_MAP[trimmed]) {
      return BACKEND_MESSAGE_MAP[trimmed];
    }
    return trimmed;
  }
  return t('errors.generic', DEFAULT_FALLBACK);
};

const resolveHint = (details?: ApiErrorDetails, inferredType?: string) => {
  if (details?.hint) return details.hint;
  if (inferredType && TYPE_HINTS[inferredType]) {
    const meta = TYPE_HINTS[inferredType];
    return t(meta.key, meta.fallback);
  }
  return '';
};

const inferType = (details?: ApiErrorDetails, status?: number, network?: boolean) => {
  if (details?.type) return details.type;
  if (network) return 'NetworkError';
  if (status === 401 || status === 423) return 'AuthError';
  if (status === 403) return 'BusinessError';
  if (status === 404) return 'BusinessError';
  if (status && status < 500) return 'BusinessError';
  return 'SystemError';
};

const buildDescriptor = (
  payload: ApiErrorPayload | undefined,
  opts: DescriptorOptions = {}
): ApiErrorDescriptor => {
  const { status, fallbackMessage, network } = opts;
  const details = payload?.details;
  const type = inferType(details, status, network);
  const message = resolveMessage(payload?.code, payload?.message || fallbackMessage);
  const hint = resolveHint(details, type);
  const fields = details?.fields ?? [];
  const retryable = network || !status || status >= 500 || payload?.code === 50000;
  return {
    code: payload?.code,
    status,
    type,
    message,
    hint,
    requestId: payload?.requestId || payload?.traceId || details?.errorId,
    timestamp: payload?.timestamp,
    fields,
    retryable,
    network
  };
};

export const describeAxiosError = (
  error: AxiosError | null,
  fallback?: string
): ApiErrorDescriptor | null => {
  if (!error) return null;
  const payload = (error.response?.data as ApiErrorPayload | undefined) || undefined;
  return buildDescriptor(payload, {
    status: error.response?.status,
    fallbackMessage: fallback || error.message || DEFAULT_FALLBACK,
    network: !error.response
  });
};

export const setApiError = (error: unknown, descriptor: ApiErrorDescriptor | null) => {
  if (!descriptor || !error || typeof error !== 'object') return;
  Object.defineProperty(error, API_ERROR_PROP, {
    value: descriptor,
    configurable: true,
    enumerable: false,
    writable: true
  });
};

export const getApiError = (error: unknown): ApiErrorDescriptor | null => {
  if (!error || typeof error !== 'object') return null;
  const existing = (error as Record<string, unknown>)[API_ERROR_PROP] as
    | ApiErrorDescriptor
    | undefined;
  if (existing) return existing;
  if (isAxiosError(error)) {
    const descriptor = describeAxiosError(error);
    if (descriptor) {
      setApiError(error, descriptor);
      return descriptor;
    }
  }
  return null;
};

export const mapFieldErrors = (
  descriptor?: ApiErrorDescriptor | null,
  fieldMap: Record<string, string> = {}
): Record<string, string> => {
  if (!descriptor?.fields?.length) return {};
  return descriptor.fields.reduce<Record<string, string>>((acc, field) => {
    const key = fieldMap[field.field] || field.field;
    acc[key] = field.message || descriptor.message;
    return acc;
  }, {});
};

export const isValidationError = (descriptor?: ApiErrorDescriptor | null) =>
  descriptor?.type === 'ValidationError';

export const createInlineError = (message: string, hint?: string): ApiErrorDescriptor => ({
  message: message || t('errors.generic', DEFAULT_FALLBACK),
  hint,
  type: 'Inline'
});
