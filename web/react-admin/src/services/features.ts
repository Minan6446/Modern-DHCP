import type {
  ClusterOverview,
  DhcpOptionCatalog,
  HelpCenterSnapshot,
  IntegrationMatrix,
  MaintenanceOverview,
  SecurityOverview,
  StaticBindingSummary,
  SystemManagementSummary,
} from '../types/api'
import { http } from './http'
import {
  mockClusterOverview,
  mockDhcpOptionCatalog,
  mockHelpCenterSnapshot,
  mockIntegrationMatrix,
  mockMaintenanceOverview,
  mockSecurityOverview,
  mockStaticBindingSummary,
  mockSystemManagementSummary,
} from './mockFeatures'

async function withFallback<T>(request: () => Promise<T>, fallback: () => Promise<T>): Promise<T> {
  try {
    return await request()
  } catch (error) {
    console.warn('[feature service] falling back to mock payload', error)
    return fallback()
  }
}

export function fetchSystemManagementSummary(): Promise<SystemManagementSummary> {
  return withFallback(async () => {
    const { data } = await http().get<SystemManagementSummary>('/ui/system/summary')
    return data
  }, mockSystemManagementSummary)
}

export function fetchStaticBindingSummary(): Promise<StaticBindingSummary> {
  return withFallback(async () => {
    const { data } = await http().get<StaticBindingSummary>('/network/static-bindings/summary')
    return data
  }, mockStaticBindingSummary)
}

export function fetchDhcpOptionCatalog(): Promise<DhcpOptionCatalog> {
  return withFallback(async () => {
    const { data } = await http().get<DhcpOptionCatalog>('/dhcp/options/catalog')
    return data
  }, mockDhcpOptionCatalog)
}

export function fetchClusterOverview(): Promise<ClusterOverview> {
  return withFallback(async () => {
    const { data } = await http().get<ClusterOverview>('/cluster/overview')
    return data
  }, mockClusterOverview)
}

export function fetchSecurityOverview(): Promise<SecurityOverview> {
  return withFallback(async () => {
    const { data } = await http().get<SecurityOverview>('/security/overview')
    return data
  }, mockSecurityOverview)
}

export function fetchIntegrationMatrix(): Promise<IntegrationMatrix> {
  return withFallback(async () => {
    const { data } = await http().get<IntegrationMatrix>('/integrations/overview')
    return data
  }, mockIntegrationMatrix)
}

export function fetchMaintenanceOverview(): Promise<MaintenanceOverview> {
  return withFallback(async () => {
    const { data } = await http().get<MaintenanceOverview>('/maintenance/overview')
    return data
  }, mockMaintenanceOverview)
}

export function fetchHelpCenterSnapshot(): Promise<HelpCenterSnapshot> {
  return withFallback(async () => {
    const { data } = await http().get<HelpCenterSnapshot>('/help-center/snapshot')
    return data
  }, mockHelpCenterSnapshot)
}
