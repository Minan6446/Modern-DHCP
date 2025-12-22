import { useEffect, useMemo, useState } from 'react'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import enUS from 'antd/locale/en_US'
import jaJP from 'antd/locale/ja_JP'
import { RouterProvider } from 'react-router-dom'
import { router } from './router'
import { useSessionStore } from './store/session'
import { usePreferencesStore } from './store/preferences'

const localeMap = {
  'zh-CN': zhCN,
  'en-US': enUS,
  'ja-JP': jaJP,
} as const

function App() {
  const bootstrap = useSessionStore((state) => state.bootstrap)
  const themeMode = usePreferencesStore((state) => state.themeMode)
  const locale = usePreferencesStore((state) => state.locale)
  const fontScale = usePreferencesStore((state) => state.fontScale)
  const highContrast = usePreferencesStore((state) => state.highContrast)
  const reduceMotion = usePreferencesStore((state) => state.reduceMotion)
  const [systemPrefersDark, setSystemPrefersDark] = useState(() => {
    if (typeof window === 'undefined') {
      return true
    }
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  })

  useEffect(() => {
    if (typeof window === 'undefined') {
      return undefined
    }
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = (event: MediaQueryListEvent) => setSystemPrefersDark(event.matches)
    if (typeof media.addEventListener === 'function') {
      media.addEventListener('change', handler)
    } else {
      media.addListener(handler)
    }
    return () => {
      if (typeof media.removeEventListener === 'function') {
        media.removeEventListener('change', handler)
      } else {
        media.removeListener(handler)
      }
    }
  }, [])

  useEffect(() => {
    bootstrap()
  }, [bootstrap])

  const resolvedTheme = themeMode === 'system' ? (systemPrefersDark ? 'dark' : 'light') : themeMode

  useEffect(() => {
    if (typeof document === 'undefined') {
      return
    }
    const root = document.documentElement
    root.dataset.theme = resolvedTheme
    root.dataset.contrast = highContrast ? 'high' : 'normal'
    root.dataset.motion = reduceMotion ? 'reduced' : 'full'
    root.style.setProperty('--app-font-scale', String(fontScale))
  }, [resolvedTheme, highContrast, reduceMotion, fontScale])

  const themeConfig = useMemo(() => {
    const isDark = resolvedTheme === 'dark'
    const primary = highContrast ? '#ffb200' : isDark ? '#5d5bf3' : '#4c46ff'
    const baseBg = isDark ? '#05060a' : '#f3f6ff'
    const textBase = isDark ? '#f8f9ff' : '#0b1220'
    const containerBg = isDark ? '#101225' : '#ffffff'
    return {
      token: {
        colorPrimary: primary,
        colorBgBase: baseBg,
        colorTextBase: textBase,
        colorBgContainer: containerBg,
        borderRadiusLG: highContrast ? 10 : 18,
        fontSize: Math.round(14 * fontScale),
        fontSizeHeading3: Math.round(24 * fontScale),
      },
      components: {
        Layout: {
          headerBg: 'transparent',
          siderBg: 'transparent',
        },
        Menu: {
          itemSelectedBg: isDark ? 'rgba(93,91,243,0.18)' : 'rgba(76,70,255,0.12)',
          itemSelectedColor: isDark ? '#ffffff' : '#1c1f33',
        },
      },
    }
  }, [resolvedTheme, highContrast, fontScale])

  const antdLocale = localeMap[locale] ?? zhCN

  return (
    <ConfigProvider locale={antdLocale} theme={themeConfig}>
      <RouterProvider router={router} />
    </ConfigProvider>
  )
}

export default App
