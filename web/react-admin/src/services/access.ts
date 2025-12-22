import type { AccessApiKey, CreateApiKeyPayload, CreateApiKeyResponse } from '../types/api'
import { http } from './http'

export async function listApiKeys(): Promise<AccessApiKey[]> {
  const { data } = await http().get('/access/api-keys')
  return data
}

export async function createApiKey(payload: CreateApiKeyPayload): Promise<CreateApiKeyResponse> {
  const { data } = await http().post('/access/api-keys', payload)
  return data
}

export async function revokeApiKey(keyId: string): Promise<void> {
  await http().delete(`/access/api-keys/${keyId}`)
}
