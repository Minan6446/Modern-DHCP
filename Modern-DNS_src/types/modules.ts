export interface AppUser {
  name: string
  role: string
  avatar: string
}

export interface SystemConfig {
  timezone: string
  language: string
  autoBackup: boolean
  dnssecGlobal: boolean
  maintenanceEnabled?: boolean
}

export interface DashboardModuleData {
  overview: Record<string, any>
  domainStatus: Array<Record<string, any>>
  alerts: {
    rules: DashboardAlertRule[]
    rows: Array<Record<string, any>>
    auditLogs: Array<Record<string, any>>
  }
  resource: Record<string, any>
}

export interface DashboardAlertRule {
  id: number
  ruleId: string
  ruleName: string
  alertType: string
  level: string
  channel: string
  target: string
  threshold: number
  status: string
  remark: string
  createdAt: string
}

export interface DomainZone {
  id: number
  zoneId: string
  domain: string
  type: string
  status: string
  remark: string
  createdAt: string
  upstream: string
  serial: string
}

export interface DomainRecord {
  id: number
  type: string
  host: string
  value: string
  ttl: number
  status: string
  remark: string
  lastUsedAt?: string | null
  updatedAt: string
}

export interface DomainDnssecKey {
  id: number
  keyId: string
  keyTag: number
  algorithm: string
  ds: string
  createdAt: string
  status: string
}

export interface DomainSoa {
  mname: string
  rname: string
  refresh: number
  retry: number
  expire: number
  minimumTtl: number
}

export interface DomainDetail {
  records: DomainRecord[]
  soa: DomainSoa
  dnssec: {
    enabled: boolean
    ksk: DomainDnssecKey[]
    zsk: DomainDnssecKey[]
  }
}

export interface DomainModuleData {
  zones: DomainZone[]
  details: Record<string, DomainDetail>
}

export interface ForwardProvider {
  label: string
  value: string
  ips: string[]
}

export interface ForwardServer {
  id: number
  name: string
  address: string
  port: number
  protocol: string
  priority: number
  status: string
}

export interface ForwardRule {
  id: number
  ruleId: string
  domains: string
  upstreamId: number
  upstreamName: string
  priority: number
  status: string
  remark: string
  createdAt: string
}

export interface ForwardGlobalConfig {
  enabled: boolean
  publicDns: string
  publicDnsEnabled: boolean
  publicDnsCustom: string
  providers: ForwardProvider[]
  timeout: number
  retries: number
  strategy: string
  servers: ForwardServer[]
}

export interface ForwardModuleData {
  global: ForwardGlobalConfig
  condition: {
    rules: ForwardRule[]
  }
}

export interface CacheNode {
  id: number
  cacheId: string
  domain: string
  recordType: string
  recordValue: string
  ttlRemaining: number
  ttlOriginal: number
  source: string
  persisted: string
  cacheTime: string
  updatedAt: string
  children?: CacheNode[]
}

export interface CacheGlobalStrategy {
  ttlMax: number
  minRetain: number
  autoCleanup: boolean
  cleanupCycle: string
}

export interface CacheDomainRule {
  id: number
  domain: string
  customTtl: number
  customRetain: number
  status: string
  createdAt: string
}

export interface CacheModuleData {
  domainCacheTree: CacheNode[]
  strategy: {
    global: CacheGlobalStrategy
    domainRules: CacheDomainRule[]
  }
}

export interface SecurityBlackWhiteRule {
  id: number
  ruleId: string
  type: string
  listType: string
  value: string
  remark: string
  status: string
  createdAt: string
}

export interface SecurityDdosGlobal {
  enabled: boolean
  qpsLimit: number
  currentQps: number
  perIpConnLimit: number
  memSoftMb: number
  memHardMb: number
  perIpQps: number
  perIpBurst: number
}

export interface SecurityDdosDomainRule {
  id: number
  domain: string
  qpsLimit: number
  status: string
  createdAt: string
}

export interface SecurityDnssecRow {
  id: number
  domain: string
  dnssecStatus: string
  signatureStatus: string
  lastCheckAt: string
  ksk: Array<{ keyId: string; createdAt: string; status: string }>
  zsk: Array<{ keyId: string; createdAt: string; status: string }>
}

export interface SecurityModuleData {
  blackWhite: { rules: SecurityBlackWhiteRule[] }
  ddos: {
    global: SecurityDdosGlobal
    domainRules: SecurityDdosDomainRule[]
  }
  dnssec: { rows: SecurityDnssecRow[] }
}

export interface MonitorRuleQps {
  globalThresholdPercent: number
  domainThresholdPercent: number
  periodSec: number
  enabled: boolean
}

export interface MonitorRuleNxdomain {
  thresholdPercent: number
  periodSec: number
  enabled: boolean
}

export interface MonitorRuleLatency {
  thresholdMs: number
  periodSec: number
  enabled: boolean
}

export interface MonitorRuleCacheHit {
  minHitPercent: number
  periodSec: number
  enabled: boolean
}

export interface MonitorRuleHistory {
  id: number
  ruleId: string
  ruleType: string
  triggerAt: string
  content: string
  handleStatus: string
}

export interface MonitorReportData {
  topDomain: Array<{ name: string; value: number }>
  topIp: Array<{ name: string; value: number }>
  heatmap: {
    regions: string[]
    periods: string[]
    values: number[][]
  }
  statusDistribution: Array<{ name: string; value: number }>
}

export interface MonitorRealTimeRow {
  id: number
  responseTime: number
  responseStatus: string
  rcode: string
  time: string
  domain: string
  sourceIp: string
  transactionId?: string
  [key: string]: any
}

export interface MonitorResolveLogRow {
  domain: string
  transactionId: string
  sourceIp: string
  [key: string]: any
}

export interface MonitorModuleData {
  realTime: MonitorRealTimeRow[]
  resolveLogs: MonitorResolveLogRow[]
  rules: {
    qps: MonitorRuleQps
    nxdomain: MonitorRuleNxdomain
    latency: MonitorRuleLatency
    cacheHit: MonitorRuleCacheHit
    history: MonitorRuleHistory[]
  }
  report: MonitorReportData
}

export interface DigHistoryItem {
  id: number
  domain: string
  recordType: string
  dnsServer: string
  queriedAt: string
}

export interface DnssecDebugResult {
  domain: string
  overallStatus: string
  dnssecEnabled: boolean
  checkedAt: string
  details: Array<{
    item: string
    status: string
    result: string
    detail: string
  }>
}

export interface GlobalTestNode {
  id: number
  node: string
  operator: string
  category: string
  result: string
  latency: number
  consistency: string
  detail: string
}

export interface IpLocationRow {
  ip: string
  country: string
  province: string
  city: string
  isp: string
  asn: string
  location: string
  lineType: string
  geoDnsSuggestion: string
}

export interface ToolsModuleData {
  dig: {
    history: DigHistoryItem[]
  }
  dnssecDebug: {
    samples: Record<string, Omit<DnssecDebugResult, 'domain'>>
  }
  globalTest: {
    nodes: GlobalTestNode[]
  }
  ipLocation: {
    samples: Record<string, IpLocationRow>
  }
}

export interface SettingCommonConfig {
  timezone: string
  language: string
  autoBackup: boolean
  backupCycle: string
  backupRetentionDays: number
  backupStorageType: 'local' | 's3' | 'sftp'
  backupStoragePath: string
  dnssecGlobal: boolean
  defaultTtl: number
  negativeCacheTtl: number
  ecsEnabled: boolean
  dohEnabled: boolean
  dotEnabled: boolean

  // ── DNS policy extensions (2026-05) ─────────────────
  // TTL clamp & upstream / ECS / padding. Backend defaults
  // documented in internal/model/model.go::SystemConfig.
  minTtl: number
  maxTtl: number
  upstreamProtocolOrder: string
  upstreamTimeoutMs: number
  ecsPrefixV4: number
  ecsPrefixV6: number
  dnsPaddingEnabled: boolean
  dnsPaddingBlock: number
  dohPreferGet: boolean

  loginTimeoutMinutes: number
  loginMaxFailures: number
  loginLockMinutes: number
  mfaRequired: boolean
  ipWhitelist: string

  // ── Password & session policy (2026-05) ─────────────
  pwdMinLength: number
  pwdRequireUpper: boolean
  pwdRequireLower: boolean
  pwdRequireDigit: boolean
  pwdRequireSymbol: boolean
  pwdExpireDays: number
  maxConcurrentLogin: number

  // ── DB connection pool (2026-05) ────────────────────
  dbMaxOpenConns: number
  dbMaxIdleConns: number
  dbConnMaxLifetimeMin: number
  dbConnMaxIdleMin: number

  // ── NTP time sync (2026-05) ─────────────────────────
  ntpEnabled: boolean
  ntpServers: string
  ntpCheckIntervalSec: number
  ntpLastSync?: string
  ntpLastDriftMs?: number
  ntpLastError?: string

  logRetentionDays: number
  logLevel: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'
  logExportFormat: 'csv' | 'json' | 'syslog'
  syslogEnabled: boolean
  syslogServer: string
  // globalQpsThreshold dropped in 2026-05 cleanup; see types/setting.ts
  // for the rationale. Old backup payloads may still carry it; it is
  // ignored on import (Partial<SettingCommonConfig> spread).
  maintenanceEnabled: boolean
  maintenanceWindow: string
}

export interface SettingUser {
  id: number
  username: string
  password?: string
  realName: string
  roleId: number
  roleName: string
  status: string
  createdAt: string
}

export interface SettingRole {
  id: number
  name: string
  remark: string
  createdAt: string
}

export interface SettingPermissionNode {
  id: string
  label: string
  children?: SettingPermissionNode[]
}

export interface SettingBackup {
  id: number
  backupId: string
  backupScope: string[]
  backupTime: string
  fileSize: string
  format: string
  fileName: string
  // Optional stats only present on the create-backup response (the
  // listing endpoint doesn't compute them on demand). The audit page
  // and the post-create toast both consume these to tell the operator
  // exactly what made it into the file.
  totalRows?: number
  tables?: Record<string, number>
  warnedTables?: Record<string, string>
}

export interface SettingNoticeConfig {
  email: {
    enabled: boolean
    smtpHost: string
    smtpPort: number
    sender: string
    authCode: string
    receivers: string
  }
  webhook: {
    enabled: boolean
    url: string
    method: string
    secret: string
    alertTypes: string[]
  }
  sms: {
    enabled: boolean
    apiKey: string
    templateId: string
    phones: string
  }
}

export interface SettingLogItem {
  id: number
  logId: string
  operator: string
  actionType: string
  module: string
  content: string
  ip: string
  time: string
  detail: string
}

export interface SettingModuleData {
  common: SettingCommonConfig
  users: SettingUser[]
  roles: SettingRole[]
  permissionTree: SettingPermissionNode[]
  rolePermissions: Record<number, string[]>
  backups: SettingBackup[]
  notice: SettingNoticeConfig
  logs: SettingLogItem[]
}
