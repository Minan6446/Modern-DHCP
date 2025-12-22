export type UserProfile = {
  id: string
  displayName: string
  role: string
  tenantId: string
  avatarUrl?: string
}

export type TenantSummary = {
  id: string
  name: string
  status?: 'active' | 'suspended'
  description?: string
  quotas?: {
    pools?: number
    leases?: number
    staticBindings?: number
  }
}

export type LoginResponse = {
  token: string
  profile: UserProfile
}

export type OperationLogEntry = {
  id: string
  actor: string
  action: string
  resource: string
  scope: string
  status: string
  description: string
  createdAt: string
  metadata?: Record<string, string>
}

export type PoolSummary = {
  id: string
  name: string
  cidr: string
  role: string
  capacity: number
  utilization: number
  status: 'ready' | 'warning' | 'critical'
}

export type PoolUsageSummary = {
  poolId: string
  name: string
  scope?: string
  vlanId?: number
  location?: string
  allocated: number
  capacity: number
  utilization: number
}

export type MonitoringPoolsResponse = {
  generatedAt: string
  pools: PoolUsageSummary[]
}

export type DimensionCount = {
  key: string
  count: number
}

export type ClientDistributionSnapshot = {
  byDeviceType?: DimensionCount[]
  byLocation?: DimensionCount[]
  byVlan?: DimensionCount[]
}

export type LatencyBucket = {
  upperBoundMs: number
  count: number
}

export type RequestPhaseSnapshot = {
  protocol: string
  message: string
  success: number
  failure: number
  averageMs: number
  p95Ms: number
  latencyBuckets?: LatencyBucket[]
}

export type RuntimeSystemHealth = {
  timestamp: string
  cpuPercent: number
  memoryPercent: number
  memoryUsedBytes: number
  diskPercent: number
  networkRxBytes: number
  networkTxBytes: number
  goroutines: number
}

export type RateLimitWindow = {
  tenantId: string
  window: string
  totalHits: number
  uniqueMacs: number
  uniquePorts: number
  lastMac?: string
  lastPort?: string
  lastIp?: string
  lastRetryAfter?: string
  lastHit?: string
}

export type SnoopingWindow = {
  tenantId: string
  window: string
  total: number
  trusted: number
  misses: number
  untrusted: number
  errors: number
  lastResult?: string
  lastPort?: string
  lastMac?: string
  lastVlan?: number
  lastReason?: string
  lastEvent?: string
}

export type SecuritySnapshot = {
  rateLimit?: RateLimitWindow
  snooping?: SnoopingWindow
}

export type OverviewSnapshot = {
  generatedAt: string
  poolUsage: PoolUsageSummary[]
  requestPhases: RequestPhaseSnapshot[]
  clientDistribution: ClientDistributionSnapshot
  systemHealth: RuntimeSystemHealth
  security?: SecuritySnapshot
}

export type AlertSeverity = 'info' | 'warning' | 'critical'
export type AlertLifecycle = 'open' | 'acknowledged' | 'suppressed'

export type AlertFeedEntry = {
  id: string
  summary: string
  details: string
  category: string
  severity: AlertSeverity
  lifecycle: AlertLifecycle
  source: string
  tenantId: string
  assignee?: string
  channel?: string
  fingerprint: string
  tags: string[]
  createdAt: string
  updatedAt: string
}

export type AlertFeedTotals = {
  open: number
  acknowledged: number
  suppressed: number
}

export type AlertFeedSnapshot = {
  generatedAt: string
  totals: AlertFeedTotals
  alerts: AlertFeedEntry[]
}

export type LeaseRecord = {
  id: string
  address: string
  macAddress: string
  hostname?: string
  poolName: string
  tenantId?: string
  state: 'ACTIVE' | 'EXPIRED' | 'RECLAIMED'
  expiresAt: string
  assignedAt?: string
  lastSeenAt?: string
  vlanId?: number
  profile?: string
  relayAgent?: string
  clientVendor?: string
}

export type LeaseHistoryRecord = {
  leaseId: string
  poolId: string
  ipAddress: string
  identifier: string
  clientId: string
  state: string
  securityState: string
  updatedAt: string
  expiresAt: string
  cooldownUntil?: string
}

export type LeaseHistoryResponse = {
  tenantId: string
  items: LeaseHistoryRecord[]
  limit: number
  offset: number
  total: number
}

export type LeaseRiskPool = {
  poolId: string
  poolName: string
  tenantId: string
  utilization: number
  activeLeases: number
  exhaustionEta?: string
}

export type LeaseInsights = {
  generatedAt: string
  pressureIndex: number
  renewalRate: number
  avgLeaseDurationHours: number
  expiringNext24h: number
  declines24h: number
  stateBreakdown: Record<LeaseRecord['state'], number>
  vendorMix: DimensionCount[]
  poolsAtRisk: LeaseRiskPool[]
}

export type AutomationSchedule = {
  id?: string
  name?: string
  type: string
  enabled: boolean
  tenantId: string
  interval: string
  initialDelay?: string
  labels?: Record<string, string>
  channels: string[]
  payload?: Record<string, unknown>
}

export type AutomationSnapshot = {
  pendingJobs: number
  activeWorkers: number
  registeredHandlers: number
  startedAt: string
}

export type AutomationJobStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export type AutomationJobRun = {
  id: string
  tenantId: string
  type: string
  status: AutomationJobStatus
  source: string
  triggeredBy: string
  priority?: number
  attempts?: number
  payloadHash?: string
  labels?: Record<string, string>
  payload?: Record<string, unknown>
  resultSummary?: string
  errorMessage?: string
  notBefore?: string
  queuedAt?: string
  startedAt?: string
  completedAt?: string
  updatedAt?: string
}

export type AutomationJobListResponse = {
  items: AutomationJobRun[]
  total: number
  limit: number
  offset: number
}

export type AutomationJobRequest = {
  type: string
  tenantId?: string
  labels?: Record<string, string>
  payload?: Record<string, unknown>
  channels?: string[]
  priority?: number
  notBefore?: string
  source?: string
  triggeredBy?: string
}

export type PaginatedResponse<T> = {
  items: T[]
  total: number
  page: number
  size: number
}

export type LeaseQuery = {
  query?: string
  state?: LeaseRecord['state']
  poolName?: string
  ipRange?: string
  macPrefix?: string
  lastSeenMinutes?: number
}

export type LeaseHistoryQuery = {
  state?: string
  identifier?: string
  ip?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export type ReportingArtifact = {
  name: string
  path: string
  format: 'csv' | 'pdf'
  contentType: string
  size: number
  generatedAt: string
}

export type LeaseHistoryExportPayload = {
  format: 'csv' | 'pdf'
  destination: string
  state?: string
  identifier?: string
  ipAddress?: string
  from?: string
  to?: string
  limit?: number
}

export type LeaseHistoryExportResponse = {
  status: string
  artifact: ReportingArtifact
}

export type ExportJobSummary = {
  jobId: string
  report: string
  tenantId: string
  format: 'csv' | 'pdf'
  interval: string
  nextRun: string
}

export type LeaseHistorySchedulePayload = {
  format: 'csv' | 'pdf'
  destination: string
  intervalHours: number
  state?: string
  identifier?: string
  ipAddress?: string
  from?: string
  to?: string
  limit?: number
}

export type LeaseHistoryScheduleResponse = {
  status: string
  job: ExportJobSummary
}

export type PoolQuery = {
  query?: string
  status?: 'warning' | 'critical'
  role?: string
}

export type SystemAuthProvider = {
  id: string
  name: string
  type: 'local' | 'ldap' | 'sso'
  status: 'active' | 'planned' | 'disabled'
}

export type TenantQuotaSummary = {
  tenantId: string
  tenantName: string
  poolsUsed: number
  poolLimit: number
  leasesUsed: number
  leaseLimit: number
  staticBindingsUsed: number
  staticBindingLimit: number
}

export type RbacRoleSummary = {
  id: string
  name: string
  description: string
  permissions: number
}

export type RbacRolePayload = {
  name: string
  description?: string
  permissions: number
}

export type ApiKeySummary = {
  id: string
  label: string
  scope: string
  lastUsedAt?: string
}

export type SessionSummary = {
  id: string
  user: string
  tenant: string
  issuedAt: string
  expiresAt: string
  active: boolean
}

export type SystemManagementSummary = {
  authProviders: SystemAuthProvider[]
  tenants: TenantQuotaSummary[]
  roles: RbacRoleSummary[]
  apiKeys: ApiKeySummary[]
  sessions: SessionSummary[]
}

export type StaticBindingRecord = {
  id: string
  tenantId: string
  device: string
  macAddress: string
  ipAddress: string
  hostname?: string
  status: 'online' | 'offline' | 'pending'
  lastSeenAt?: string
}

export type StaticBindingSummary = {
  total: number
  active: number
  pendingImports: number
  bindings: StaticBindingRecord[]
}

export type DhcpOptionDefinition = {
  name: string
  code: number
  category: string
  description: string
  usageCount?: number
}

export type DhcpOptionTemplate = {
  name: string
  options: number
  usedBy: number
  description: string
}

export type DhcpOptionCatalog = {
  standard: DhcpOptionDefinition[]
  custom: DhcpOptionDefinition[]
  templates: DhcpOptionTemplate[]
}

export type ClusterNodeStatus = {
  id: string
  role: 'active' | 'standby' | 'worker'
  address: string
  version: string
  health: 'healthy' | 'warning' | 'critical'
  cpuPercent: number
  memoryPercent: number
  syncLagMs: number
  lastHeartbeat: string
}

export type ClusterOverview = {
  mode: 'active-passive' | 'active-active'
  failoverReady: boolean
  replicationLagMs: number
  nodes: ClusterNodeStatus[]
  pendingActions: number
}

export type SecurityControlStatus = {
  id: string
  name: string
  status: 'enabled' | 'degraded' | 'disabled'
  severity?: AlertSeverity
  lastEventAt?: string
}

export type SecurityOverview = {
  rateLimitHits: number
  snoopingViolations: number
  rogueServers: number
  controls: SecurityControlStatus[]
  recentFindings: AlertFeedEntry[]
}

export type IntegrationAdapter = {
  id: string
  name: string
  type: 'monitoring' | 'cmdb' | 'itsm' | 'webhook'
  status: 'connected' | 'warning' | 'disconnected'
  endpoint: string
  lastSyncAt?: string
}

export type IntegrationMatrix = {
  adapters: IntegrationAdapter[]
  webhookDeliveries24h: number
  apiCalls24h: number
}

export type MaintenanceTask = {
  id: string
  title: string
  owner: string
  window: string
  status: 'scheduled' | 'running' | 'completed'
}

export type CapacityForecast = {
  metric: string
  current: number
  limit: number
  projection30d: number
}

export type MaintenanceOverview = {
  backupsEnabled: boolean
  backupWindow: string
  lastBackupAt: string
  tasks: MaintenanceTask[]
  forecasts: CapacityForecast[]
}

export type SupportResource = {
  id: string
  title: string
  type: 'doc' | 'guide' | 'api' | 'faq'
  updatedAt: string
  link: string
}

export type HelpCenterSnapshot = {
  docs: SupportResource[]
  openTickets: number
  latestVersion: string
  releaseHighlights: string[]
}

export type UiMetadata = {
  catalog: string[]
  granted: string[]
  temporary?: string[]
}
