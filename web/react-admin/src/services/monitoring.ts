import type {
  AlertFeedSnapshot,
  AlertSeverity,
  MonitoringPoolsResponse,
} from '../types/api'
import { http } from './http'

export async function fetchAlertFeed(params?: {
  severity?: AlertSeverity
  limit?: number
}): Promise<AlertFeedSnapshot> {
  const { data } = await http().get('/monitoring/alerts', { params })
  return data
}

export async function fetchMonitoredPools(limit?: number): Promise<MonitoringPoolsResponse> {
  const { data } = await http().get('/monitoring/pools', { params: limit ? { limit } : undefined })
  return data
}
