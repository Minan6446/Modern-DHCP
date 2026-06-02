import { i18n } from '@/i18n';

const { t } = i18n.global;

const isNetworkError = (value?: string) => !!value && value.toLowerCase().includes('network');

export const mapAuthError = (status?: number, message?: string): string => {
  if (status === 401) return t('errors.auth.invalidCredentials');
  if (status === 423) return t('errors.auth.locked');
  if (status === 429) return t('errors.auth.tooMany');
  if (status === 498 || message?.toLowerCase().includes('expired')) return t('errors.auth.expired');
  if (status && status >= 500) return t('errors.catalog.systemError');
  if (isNetworkError(message)) return t('errors.auth.network');
  return t('errors.auth.generic');
};
