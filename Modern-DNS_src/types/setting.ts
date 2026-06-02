import type { SettingCommonConfig } from './modules'

export type GeneralConfigForm = SettingCommonConfig

export const createDefaultGeneralConfigForm = (): GeneralConfigForm => ({
  timezone: 'UTC+8',
  language: 'zh-CN',
  autoBackup: true,
  backupCycle: '每日',
  backupRetentionDays: 30,
  backupStorageType: 'local',
  backupStoragePath: '/var/dns/backups',
  dnssecGlobal: true,
  defaultTtl: 3600,
  negativeCacheTtl: 300,
  ecsEnabled: false,
  dohEnabled: false,
  dotEnabled: false,
  // DNS policy extensions (2026-05)
  minTtl: 0,
  maxTtl: 0,
  upstreamProtocolOrder: 'udp,tcp',
  upstreamTimeoutMs: 2000,
  ecsPrefixV4: 24,
  ecsPrefixV6: 56,
  dnsPaddingEnabled: false,
  dnsPaddingBlock: 128,
  dohPreferGet: false,
  loginTimeoutMinutes: 30,
  loginMaxFailures: 5,
  loginLockMinutes: 15,
  mfaRequired: false,
  ipWhitelist: '',
  // Password / session policy (2026-05)
  pwdMinLength: 8,
  pwdRequireUpper: true,
  pwdRequireLower: true,
  pwdRequireDigit: true,
  pwdRequireSymbol: false,
  pwdExpireDays: 0,
  maxConcurrentLogin: 0,
  // DB connection pool (2026-05)
  dbMaxOpenConns: 50,
  dbMaxIdleConns: 25,
  dbConnMaxLifetimeMin: 10,
  dbConnMaxIdleMin: 5,
  // NTP (2026-05)
  ntpEnabled: true,
  ntpServers: 'pool.ntp.org\ntime.cloudflare.com',
  ntpCheckIntervalSec: 300,
  ntpLastSync: '',
  ntpLastDriftMs: 0,
  ntpLastError: '',
  logRetentionDays: 90,
  logLevel: 'INFO',
  logExportFormat: 'csv',
  syslogEnabled: false,
  syslogServer: '',
  // globalQpsThreshold removed in 2026-05 cleanup. The column lingers
  // in system_config for backwards compatibility with old backups but
  // is never read by any backend code path; real QPS limiting lives in
  // ddos_global.qps_limit (「安全中心 → DDoS 防护」).
  maintenanceEnabled: false,
  maintenanceWindow: '02:00-04:00',
})

export const mergeGeneralConfigForm = (
  payload?: Partial<SettingCommonConfig>,
): GeneralConfigForm => ({
  ...createDefaultGeneralConfigForm(),
  ...payload,
})

export const toSettingCommonConfigPayload = (
  form: GeneralConfigForm,
): SettingCommonConfig => ({ ...form })