import { createI18n } from 'vue-i18n';
import en from './locales/en';
import zh from './locales/zh';

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

const messages = { en, zh } as const;

export type MessageSchema = typeof zh;

export const i18n = createI18n<[MessageSchema], AppLocale['code']>({
  legacy: false,
  globalInjection: true,
  locale: DEFAULT_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  missingWarn: false,
  fallbackWarn: false,
  messages: messages as any
});

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
