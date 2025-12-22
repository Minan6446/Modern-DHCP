import type { UiMetadata } from '../types/api'
import { http } from './http'

const fallbackMetadata: UiMetadata = {
  catalog: [
    'view:dashboard',
    'manage:system',
    'view:pools',
    'view:leases',
    'manage:static-bindings',
    'manage:dhcp-options',
    'view:automation',
    'view:monitoring',
    'manage:policy',
    'view:cluster',
    'view:security',
    'manage:integrations',
    'view:maintenance',
    'manage:settings',
    'view:help',
  ],
  granted: [
    'view:dashboard',
    'manage:system',
    'view:pools',
    'view:leases',
    'manage:static-bindings',
    'manage:dhcp-options',
    'view:automation',
    'view:monitoring',
    'manage:policy',
    'view:cluster',
    'view:security',
    'manage:integrations',
    'view:maintenance',
    'manage:settings',
    'view:help',
  ],
}

export async function fetchUiMetadata(): Promise<UiMetadata> {
  try {
    const { data } = await http().get<UiMetadata>('/ui/metadata')
    if (data?.granted?.length) {
      return data
    }
    return fallbackMetadata
  } catch (error) {
    console.warn('[ui metadata] falling back to defaults', error)
    return fallbackMetadata
  }
}
