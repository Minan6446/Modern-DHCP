import type { LoginResponse, SessionSummary } from '../types/api'
import { http } from './http'

export async function login(payload: { username: string; password: string }): Promise<LoginResponse> {
  const { data } = await http().post('/auth/login', payload)
  return data
}

export async function listSessions(): Promise<SessionSummary[]> {
  const { data } = await http().get('/auth/sessions')
  return data
}

export async function terminateSession(sessionId: string): Promise<void> {
  await http().delete(`/auth/sessions/${sessionId}`)
}
