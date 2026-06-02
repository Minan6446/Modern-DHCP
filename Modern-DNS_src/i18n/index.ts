import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import enUS from './en-US'

const LANG_KEY = 'modern-dns-lang'

function getSavedLocale(): 'zh-CN' | 'en-US' {
  const saved = localStorage.getItem(LANG_KEY)
  if (saved === 'en-US') return 'en-US'
  return 'zh-CN'
}

const i18n = createI18n({
  legacy: false,
  locale: getSavedLocale(),
  fallbackLocale: 'zh-CN',
  globalInjection: true,
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export const setI18nLanguage = (language: string): 'zh-CN' | 'en-US' => {
  const nextLocale = language === 'en-US' ? 'en-US' : 'zh-CN'

  i18n.global.locale.value = nextLocale
  document.documentElement.lang = nextLocale
  localStorage.setItem(LANG_KEY, nextLocale)

  return nextLocale
}

export default i18n