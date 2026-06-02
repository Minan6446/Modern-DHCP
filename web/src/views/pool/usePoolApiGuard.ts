import { showError } from '@/shared/errors/messageToast';
import { i18n } from '@/i18n';
import { getApiError } from '@/shared/errors/apiError';

const isAbortError = (error: any) =>
  error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError' || error?.name === 'AbortError';
const { t } = i18n.global;

const extractMessage = (error: unknown, fallback?: string) => {
  const descriptor = getApiError(error);
  if (descriptor?.message) return descriptor.message;
  return fallback || t('errors.generic');
};

export const usePoolApiGuard = () => {
  const wrapApi = async <T>(fn: () => Promise<T>, fallback?: string): Promise<T> => {
    try {
      return await fn();
    } catch (error) {
      if (!isAbortError(error)) {
        const msg = extractMessage(error, fallback);
        showError(msg);
      }
      throw error;
    }
  };

  return { wrapApi };
};
