import { ElMessage } from 'element-plus';
import { getApiError } from '@/shared/errors/apiError';
import { mapAuthError } from '@/shared/errors/mapAuthError';
import { mapHttpError } from '@/shared/errors/httpErrors';

interface ErrorToastOptions {
  fallback?: string;
  auth?: boolean;
}

const resolveMessage = (error: unknown, options?: ErrorToastOptions) => {
  const descriptor = getApiError(error);
  const status = descriptor?.status;
  const sourceMessage = descriptor?.message;
  if (options?.auth) {
    return mapAuthError(status, sourceMessage) || options?.fallback || sourceMessage || mapHttpError(status, sourceMessage);
  }
  return mapHttpError(status, sourceMessage) || options?.fallback || sourceMessage || mapHttpError();
};

export const showHttpError = (error: unknown, fallback?: string) => {
  ElMessage.error(resolveMessage(error, { fallback }));
};

export const showAuthError = (error: unknown, fallback?: string) => {
  ElMessage.error(resolveMessage(error, { fallback, auth: true }));
};
