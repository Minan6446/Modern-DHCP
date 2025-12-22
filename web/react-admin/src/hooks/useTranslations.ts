import { usePreferencesStore } from '../store/preferences'
import { messages, type SupportedLocale } from '../i18n/messages'

export function useTranslations() {
  const locale = usePreferencesStore((state) => state.locale)
  const bundle = messages[locale as SupportedLocale] ?? messages['zh-CN']
  return bundle
}
