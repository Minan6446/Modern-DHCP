import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import zhCnEl from 'element-plus/es/locale/lang/zh-cn';
import enEl from 'element-plus/es/locale/lang/en';
import type { Language as ElementLocale } from 'element-plus/es/locale';
import {
  SUPPORTED_LOCALES,
  DEFAULT_LOCALE,
  resolveLocale,
  type AppLocale
} from './index';

const STORAGE_KEY = 'mdhcp:locale';

const elementLocales: Record<AppLocale['code'], ElementLocale> = {
  zh: zhCnEl,
  en: enEl
};

const dayjsLocaleMap: Record<AppLocale['code'], string> = {
  zh: 'zh-cn',
  en: 'en'
};

const dayjsLoaders: Record<AppLocale['code'], () => Promise<unknown>> = {
  zh: () => import('dayjs/locale/zh-cn'),
  en: () => import('dayjs/locale/en')
};

const elementLocaleListeners = new Set<(locale: ElementLocale) => void>();

/** Register a callback invoked whenever the active locale changes. Used by main.ts. */
export const onElementLocaleChange = (cb: (locale: ElementLocale) => void) => {
  elementLocaleListeners.add(cb);
  return () => elementLocaleListeners.delete(cb);
};

export const getStoredLocale = (): AppLocale['code'] => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return resolveLocale(raw).code;
  } catch {
    /* ignore storage errors */
  }
  if (typeof navigator !== 'undefined') {
    return resolveLocale(navigator.language).code;
  }
  return DEFAULT_LOCALE;
};

const applySideEffects = (locale: AppLocale) => {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('lang', locale.tag);
  }
  elementLocaleListeners.forEach((cb) => {
    try {
      cb(elementLocales[locale.code]);
    } catch {
      /* swallow listener errors */
    }
  });
  // Lazy-load dayjs locale + activate; only if dayjs is already present in deps.
  dayjsLoaders[locale.code]()
    .then(async () => {
      try {
        const dayjs = (await import('dayjs')).default;
        dayjs.locale(dayjsLocaleMap[locale.code]);
      } catch {
        /* dayjs optional */
      }
    })
    .catch(() => undefined);
};

export const getElementLocale = (code: AppLocale['code']): ElementLocale =>
  elementLocales[code];

export const useLocale = () => {
  const { locale } = useI18n({ useScope: 'global' });

  const current = computed<AppLocale>(() => resolveLocale(locale.value));

  const setLocale = (next: AppLocale['code'] | string) => {
    const target = resolveLocale(next);
    if (locale.value !== target.code) {
      locale.value = target.code;
    }
    try {
      localStorage.setItem(STORAGE_KEY, target.code);
    } catch {
      /* ignore storage errors */
    }
    applySideEffects(target);
  };

  return {
    locale,
    current,
    available: SUPPORTED_LOCALES,
    setLocale
  };
};

/** Apply locale at bootstrap (used by main.ts before app mount). */
export const bootstrapLocale = (i18nLocale: { value: AppLocale['code'] }) => {
  const target = resolveLocale(getStoredLocale());
  i18nLocale.value = target.code;
  applySideEffects(target);
  return target;
};
