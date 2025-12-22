import { useCallback } from 'react'
import { messages, type MessageKey, type SupportedLocale } from '../i18n/messages'
import { usePreferencesStore } from '../store/preferences'

type MessageValues = Record<string, string | number | undefined>

export function useI18n() {
  const locale = usePreferencesStore((state) => state.locale)
  const bundle = messages[locale as SupportedLocale] ?? messages['zh-CN']

  const t = useCallback(
    (key: MessageKey, values?: MessageValues) => {
      const template = bundle[key] ?? key
      if (!values) return template
      return Object.entries(values).reduce((acc, [token, value]) => {
        const safeValue = value ?? ''
        return acc.replace(new RegExp(`{{${token}}}`, 'g'), String(safeValue))
      }, template)
    },
    [bundle],
  )

  return { t, locale }
}
