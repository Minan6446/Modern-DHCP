export const loadTableState = <T>(key: string, fallback: T): T => {
  if (typeof window === 'undefined') {
    return fallback
  }

  try {
    const raw = window.sessionStorage.getItem(key)
    if (!raw) {
      return fallback
    }
    return { ...fallback, ...JSON.parse(raw) }
  } catch (_error) {
    return fallback
  }
}

export const saveTableState = <T>(key: string, state: T): void => {
  if (typeof window === 'undefined') {
    return
  }

  try {
    window.sessionStorage.setItem(key, JSON.stringify(state))
  } catch (_error) {
    // Ignore storage quota and serialization issues silently.
  }
}
