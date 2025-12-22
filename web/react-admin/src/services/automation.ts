import type {
  AutomationJobListResponse,
  AutomationJobRequest,
  AutomationJobRun,
  AutomationSchedule,
  AutomationSnapshot,
} from '../types/api'
import { http } from './http'

export type AutomationJobQuery = {
  limit?: number
  offset?: number
  types?: string
  statuses?: string
  sources?: string
  triggeredBy?: string
}

export async function fetchSchedules(): Promise<AutomationSchedule[]> {
  const { data } = await http().get('/automation/schedules')
  return data
}

export async function fetchSnapshot(): Promise<AutomationSnapshot> {
  const { data } = await http().get('/automation/snapshot')
  return data
}

export async function listJobs(params?: AutomationJobQuery): Promise<AutomationJobListResponse> {
  const { data } = await http().get('/automation/jobs', { params })
  return data
}

export async function createJob(payload: AutomationJobRequest): Promise<AutomationJobRun> {
  const { data } = await http().post('/automation/jobs', payload)
  return data
}

export async function fetchJob(jobId: string): Promise<AutomationJobRun> {
  const { data } = await http().get(`/automation/jobs/${jobId}`)
  return data
}
