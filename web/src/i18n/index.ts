import { createI18n } from 'vue-i18n';

// Eagerly-loaded core modules (needed on every page)
import commonEn from './locales/en/common';
import commonZh from './locales/zh/common';
import appEn from './locales/en/app';
import appZh from './locales/zh/app';
import navEn from './locales/en/nav';
import navZh from './locales/zh/nav';
import errorsEn from './locales/en/errors';
import errorsZh from './locales/zh/errors';

/**
 * Application-level supported locales.
 * Internal i18n locale codes stay short (en/zh) to match existing messages,
 * but we expose BCP-47 tags via `tag` for <html lang> / ElementPlus / dayjs.
 */
export interface AppLocale {
  /** Internal code used as vue-i18n locale key. */
  code: 'zh' | 'en';
  /** BCP-47 tag (used for <html lang>, dayjs, etc.). */
  tag: 'zh-CN' | 'en-US';
  /** Human label shown in the language switcher. */
  label: string;
  /** Short label (2 chars) for compact UI. */
  short: string;
}

export const SUPPORTED_LOCALES: AppLocale[] = [
  { code: 'zh', tag: 'zh-CN', label: '简体中文', short: '中' },
  { code: 'en', tag: 'en-US', label: 'English', short: 'EN' }
];

export const DEFAULT_LOCALE: AppLocale['code'] = 'zh';
export const FALLBACK_LOCALE: AppLocale['code'] = 'zh';

// Core messages loaded at boot time
const bootMessages: Record<string, any> = {
  en: { ...commonEn, ...appEn, ...navEn, ...errorsEn },
  zh: { ...commonZh, ...appZh, ...navZh, ...errorsZh }
};

export type MessageSchema = typeof bootMessages['zh'];

export const i18n = createI18n<[MessageSchema], AppLocale['code']>({
  legacy: false,
  globalInjection: true,
  locale: DEFAULT_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  missingWarn: false,
  fallbackWarn: false,
  messages: bootMessages as any
});

/**
 * Lazy-load a route-level i18n module (e.g. "pool", "lease", "cluster").
 * Registers it via setLocaleMessage so the UI reactively picks it up.
 * Call this in route guards or on page entry.
 */
const localeModuleCache = new Set<string>();

export async function loadLocaleModule(moduleName: string): Promise<void> {
  if (localeModuleCache.has(moduleName)) return; // already loaded
  try {
    let enMod: any, zhMod: any;
    switch (moduleName) {
      case 'login': enMod = (await import('./locales/en/login')).default; zhMod = (await import('./locales/zh/login')).default; break;
      case 'layout': enMod = (await import('./locales/en/layout')).default; zhMod = (await import('./locales/zh/layout')).default; break;
      case 'notifications': enMod = (await import('./locales/en/notifications')).default; zhMod = (await import('./locales/zh/notifications')).default; break;
      case 'overview': enMod = (await import('./locales/en/overview')).default; zhMod = (await import('./locales/zh/overview')).default; break;
      case 'tenant': enMod = (await import('./locales/en/tenant')).default; zhMod = (await import('./locales/zh/tenant')).default; break;
      case 'dashboard': enMod = (await import('./locales/en/dashboard')).default; zhMod = (await import('./locales/zh/dashboard')).default; break;
      case 'monitoring': enMod = (await import('./locales/en/monitoring')).default; zhMod = (await import('./locales/zh/monitoring')).default; break;
      case 'cluster': enMod = (await import('./locales/en/cluster')).default; zhMod = (await import('./locales/zh/cluster')).default; break;
      case 'pool': enMod = (await import('./locales/en/pool')).default; zhMod = (await import('./locales/zh/pool')).default; break;
      case 'lease': enMod = (await import('./locales/en/lease')).default; zhMod = (await import('./locales/zh/lease')).default; break;
      case 'binding': enMod = (await import('./locales/en/binding')).default; zhMod = (await import('./locales/zh/binding')).default; break;
      case 'option': enMod = (await import('./locales/en/option')).default; zhMod = (await import('./locales/zh/option')).default; break;
      case 'security': enMod = (await import('./locales/en/security')).default; zhMod = (await import('./locales/zh/security')).default; break;
      case 'system': enMod = (await import('./locales/en/system')).default; zhMod = (await import('./locales/zh/system')).default; break;
      case 'settings': enMod = (await import('./locales/en/settings')).default; zhMod = (await import('./locales/zh/settings')).default; break;
      default: return; // unknown module
    }
    const mergedEn = { ...i18n.global.getLocaleMessage('en'), ...enMod };
    const mergedZh = { ...i18n.global.getLocaleMessage('zh'), ...zhMod };
    i18n.global.setLocaleMessage('en', mergedEn);
    i18n.global.setLocaleMessage('zh', mergedZh);
    localeModuleCache.add(moduleName);
  } catch {
    // Module not found or load failed — ignore gracefully
  }
}

export type AppI18n = typeof i18n;

export const isSupportedLocale = (value: unknown): value is AppLocale['code'] =>
  typeof value === 'string' && SUPPORTED_LOCALES.some((l) => l.code === value);

export const resolveLocale = (input: unknown): AppLocale => {
  const fallback = SUPPORTED_LOCALES.find((l) => l.code === DEFAULT_LOCALE)!;
  if (typeof input !== 'string') return fallback;
  const lowered = input.toLowerCase();
  return (
    SUPPORTED_LOCALES.find(
      (l) => l.code === lowered || l.tag.toLowerCase() === lowered
    ) ||
    SUPPORTED_LOCALES.find((l) => lowered.startsWith(l.code)) ||
    fallback
  );
};
