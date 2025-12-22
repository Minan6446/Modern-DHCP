import type { OperationLogEntry, OverviewSnapshot } from '../types/api'
import { http } from './http'

export async function fetchOverviewSnapshot(): Promise<OverviewSnapshot> {
  const { data } = await http().get('/monitoring/overview')
  return data
}

export async function fetchOperationsLog(): Promise<OperationLogEntry[]> {
  const { data } = await http().get('/monitoring/operations')
  return data
}
