import { create } from 'zustand'

type ThemeMode = 'dark' | 'light' | 'system'
type LocaleOption = 'zh-CN' | 'en-US' | 'ja-JP'

export type PreferencesSnapshot = {
  themeMode: ThemeMode
  locale: LocaleOption
  fontScale: number
  highContrast: boolean
  reduceMotion: boolean
}

type PreferencesState = PreferencesSnapshot & {
  setThemeMode: (value: ThemeMode) => void
  setLocale: (value: LocaleOption) => void
  setFontScale: (value: number) => void
  setHighContrast: (value: boolean) => void
  setReduceMotion: (value: boolean) => void
  resetPreferences: () => void
}

const storageKey = 'modern-dhcp-preferences'

const defaultPreferences: PreferencesSnapshot = {
  themeMode: 'dark',
  locale: 'zh-CN',
  fontScale: 1,
  highContrast: false,
  reduceMotion: false,
}

function clampFontScale(value: number) {
  if (Number.isNaN(value)) return 1
  return Math.min(1.3, Math.max(0.9, Number(value)))
}

function readStoredPreferences(): Partial<PreferencesSnapshot> | null {
  if (typeof window === 'undefined') {
    return null
  }
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PreferencesSnapshot>
    if (!parsed || typeof parsed !== 'object') {
      return null
    }
    const normalized: Partial<PreferencesSnapshot> = {}
    if (parsed.themeMode === 'dark' || parsed.themeMode === 'light' || parsed.themeMode === 'system') {
      normalized.themeMode = parsed.themeMode
    }
    if (parsed.locale === 'zh-CN' || parsed.locale === 'en-US' || parsed.locale === 'ja-JP') {
      normalized.locale = parsed.locale
    }
    if (typeof parsed.fontScale === 'number') {
      normalized.fontScale = clampFontScale(parsed.fontScale)
    }
    if (typeof parsed.highContrast === 'boolean') {
      normalized.highContrast = parsed.highContrast
    }
    if (typeof parsed.reduceMotion === 'boolean') {
      normalized.reduceMotion = parsed.reduceMotion
    }
    return normalized
  } catch (error) {
    console.warn('[preferences] failed to parse stored values', error)
    return null
  }
}

function persistPreferences(snapshot: PreferencesSnapshot) {
  if (typeof window === 'undefined') {
    return
  }
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(snapshot))
  } catch (error) {
    console.warn('[preferences] failed to persist state', error)
  }
}

function extractSnapshot(state: PreferencesState): PreferencesSnapshot {
  return {
    themeMode: state.themeMode,
    locale: state.locale,
    fontScale: state.fontScale,
    highContrast: state.highContrast,
    reduceMotion: state.reduceMotion,
  }
}

export const usePreferencesStore = create<PreferencesState>((set) => ({
  ...defaultPreferences,
  ...readStoredPreferences(),
  setThemeMode: (value) =>
    set((state) => {
      const snapshot = { ...extractSnapshot(state), themeMode: value }
      persistPreferences(snapshot)
      return { themeMode: value }
    }),
  setLocale: (value) =>
    set((state) => {
      const snapshot = { ...extractSnapshot(state), locale: value }
      persistPreferences(snapshot)
      return { locale: value }
    }),
  setFontScale: (value) =>
    set((state) => {
      const nextValue = clampFontScale(value)
      const snapshot = { ...extractSnapshot(state), fontScale: nextValue }
      persistPreferences(snapshot)
      return { fontScale: nextValue }
    }),
  setHighContrast: (value) =>
    set((state) => {
      const snapshot = { ...extractSnapshot(state), highContrast: value }
      persistPreferences(snapshot)
      return { highContrast: value }
    }),
  setReduceMotion: (value) =>
    set((state) => {
      const snapshot = { ...extractSnapshot(state), reduceMotion: value }
      persistPreferences(snapshot)
      return { reduceMotion: value }
    }),
  resetPreferences: () =>
    set(() => {
      persistPreferences(defaultPreferences)
      return defaultPreferences
    }),
}))
