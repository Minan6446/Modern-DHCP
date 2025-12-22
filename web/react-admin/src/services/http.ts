import axios from 'axios'
import { useSessionStore } from '../store/session'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
const defaultTenantId = import.meta.env.VITE_TENANT_ID ?? 'tenant-default'
const AUDIT_SOURCE = 'modern-dhcp-react'

export function getActiveTenantId() {
  const state = useSessionStore.getState()
  return state.activeTenantId ?? defaultTenantId
}

const client = axios.create({
  baseURL: API_BASE_URL,
  timeout: 15_000,
})

client.interceptors.request.use((config) => {
  const session = useSessionStore.getState()
  const headers = config.headers ?? {}
  headers['X-Tenant-ID'] = getActiveTenantId()
  headers['X-Audit-Source'] = AUDIT_SOURCE
  if (session.profile) {
    headers['X-Principal-ID'] = session.profile.id
    headers['X-Actor-Name'] = session.profile.displayName
  }
  if (session.token) {
    headers.Authorization = `Bearer ${session.token}`
  }
  config.headers = headers
  return config
})

client.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      useSessionStore.getState().clearSession()
    }
    return Promise.reject(error)
  },
)

export function http() {
  return client
}
