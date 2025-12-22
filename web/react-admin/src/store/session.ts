import { create } from 'zustand'
import type { LoginResponse, TenantSummary, UserProfile } from '../types/api'

const storageKey = 'modern-dhcp-session'
const tenantStorageKey = 'modern-dhcp-active-tenant'
const defaultTenantId = import.meta.env.VITE_TENANT_ID ?? 'tenant-default'

function readStoredSession(): LoginResponse | null {
  if (typeof window === 'undefined') {
    return null
  }
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return null
    return JSON.parse(raw) as LoginResponse
  } catch (error) {
    console.warn('[session] failed to parse stored session', error)
    return null
  }
}

export type SessionState = {
  token: string | null
  profile: UserProfile | null
  isAuthenticated: boolean
  bootstrapped: boolean
  activeTenantId: string | null
  availableTenants: TenantSummary[]
  setSession: (payload: LoginResponse) => void
  clearSession: () => void
  bootstrap: () => void
  setActiveTenant: (tenantId: string) => void
  setTenants: (tenants: TenantSummary[]) => void
}

function readStoredTenant(): string | null {
  if (typeof window === 'undefined') {
    return null
  }
  try {
    return window.localStorage.getItem(tenantStorageKey)
  } catch (error) {
    console.warn('[session] failed to read stored tenant', error)
    return null
  }
}

function persistActiveTenant(tenantId: string | null) {
  if (typeof window === 'undefined') {
    return
  }
  try {
    if (tenantId) {
      window.localStorage.setItem(tenantStorageKey, tenantId)
    } else {
      window.localStorage.removeItem(tenantStorageKey)
    }
  } catch (error) {
    console.warn('[session] failed to persist tenant', error)
  }
}

export const useSessionStore = create<SessionState>((set) => ({
  token: null,
  profile: null,
  isAuthenticated: false,
  bootstrapped: false,
  activeTenantId: readStoredTenant() ?? defaultTenantId,
  availableTenants: [],
  setSession: (payload) => {
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(storageKey, JSON.stringify(payload))
    }
    const storedTenant = readStoredTenant()
    const activeTenant = storedTenant ?? payload.profile.tenantId ?? defaultTenantId
    persistActiveTenant(activeTenant)
    set({
      token: payload.token,
      profile: payload.profile,
      isAuthenticated: true,
      bootstrapped: true,
      activeTenantId: activeTenant,
    })
  },
  clearSession: () => {
    if (typeof window !== 'undefined') {
      window.localStorage.removeItem(storageKey)
    }
    set({
      token: null,
      profile: null,
      isAuthenticated: false,
      bootstrapped: true,
    })
  },
  bootstrap: () => {
    const stored = readStoredSession()
    const storedTenant = readStoredTenant()
    const activeTenant = storedTenant ?? stored?.profile?.tenantId ?? defaultTenantId
    if (activeTenant) {
      persistActiveTenant(activeTenant)
    }
    set({
      token: stored?.token ?? null,
      profile: stored?.profile ?? null,
      isAuthenticated: Boolean(stored?.token),
      bootstrapped: true,
      activeTenantId: activeTenant,
    })
  },
  setActiveTenant: (tenantId) => {
    const nextTenant = tenantId || defaultTenantId
    persistActiveTenant(nextTenant)
    set((state) => ({
      activeTenantId: nextTenant,
      availableTenants: state.availableTenants,
    }))
  },
  setTenants: (tenants) => {
    set((state) => {
      const activeExists = tenants.some((tenant) => tenant.id === state.activeTenantId)
      const nextTenantId = activeExists
        ? state.activeTenantId
        : tenants[0]?.id ?? defaultTenantId
      if (nextTenantId) {
        persistActiveTenant(nextTenantId)
      }
      return {
        availableTenants: tenants,
        activeTenantId: nextTenantId,
      }
    })
  },
}))
