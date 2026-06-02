import type { Component } from 'vue';

export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
  requestId?: string;
  traceId?: string;
  timestamp?: string;
}

export type { LoginRequest, LoginResponse, TokenPair, AuthErrorCode } from '@/modules/auth/types';

export interface PageQuery {
  page: number;
  pageSize: number;
  keyword?: string;
  status?: string;
}

export interface PageResult<T> {
  items: T[];
  total: number;
}

export interface User {
  id: string;
  username: string;
  displayName: string;
  email?: string;
  phone?: string;
  status: 'active' | 'disabled';
  roles: RoleSummary[];
  tenants: TenantSummary[];
  lastLoginAt?: string;
}

export interface RoleSummary {
  id: string;
  name: string;
}

export interface TenantSummary {
  id: string;
  name: string;
}

export interface PermissionNode {
  key: string;
  label: string;
  description?: string;
  children?: PermissionNode[];
  dependsOn?: string[];
}

export interface RoleDetail extends RoleSummary {
  description?: string;
  permissions: string[];
  scope: 'global' | 'tenant';
  createdAt?: string;
}

export interface TenantDetail {
  id: string;
  name: string;
  code: string;
  status: 'active' | 'disabled';
  quota: TenantQuota;
  createdAt?: string;
}

export interface TenantQuota {
  pools: number;
  leases: number;
  users: number;
}

export interface TenantQuotaDetail extends TenantQuota {
  dhcpOptions?: number;
  bindings?: number;
  alerts?: number;
}

export interface TenantPayload {
  name: string;
  code: string;
  status?: 'active' | 'disabled';
}

export interface ApiKey {
  id: string;
  displayName: string;
  role: string;
  capabilities: string[];
  description?: string;
  ipWhitelist?: string[];
  createdAt: string;
  createdBy?: string;
  expiresAt?: string;
  lastUsedAt?: string;
  ownerUserId?: string;
  revokedAt?: string;
  status: 'active' | 'revoked' | 'expired';
  token?: string;
}

export interface ApiKeyPayload {
  displayName: string;
  role: string;
  capabilities?: string[];
  description?: string;
  ipWhitelist?: string[];
  expiresAt: string; // RFC3339
  ownerUserId?: string;
}

export interface SessionInfo {
  id: string;
  userId: string;
  username: string;
  tenantId: string;
  ip: string;
  userAgent: string;
  status?: 'active' | 'stale';
  current?: boolean;
  createdAt: string;
  lastSeenAt: string;
}

export interface AuthSession extends SessionInfo {
  deviceId?: string;
  location?: string;
}

export interface LoginAuditRecord {
  id: string;
  username: string;
  operationType?: string;
  resourceType?: string;
  resourceId?: string;
  resourceName?: string;
  resourceLink?: string;
  resourceSummary?: string;
  operationSummary?: string;
  ip: string;
  result: 'success' | 'failed';
  reason?: string;
  sessionId?: string;
  requestId?: string;
  userAgent?: string;
  location?: string;
  before?: Record<string, any>;
  after?: Record<string, any>;
  diff?: Record<string, any>;
  rawPayload?: Record<string, any>;
  createdAt: string;
  traceId?: string;
}

export interface LoginAuditQuery extends PageQuery {
  username?: string;
  ip?: string;
  result?: 'success' | 'failed' | '';
  operationType?: string;
  resourceKeyword?: string;
  requestId?: string;
  sessionId?: string;
  startAt?: string;
  endAt?: string;
}

export interface MenuItem {
  key: string;
  label: string;
  icon?: Component;
  path: string;
  children?: MenuItem[];
  permission?: string;
}

export interface UserPayload {
  username: string;
  displayName: string;
  email?: string;
  phone?: string;
  password?: string;
  mustChangePassword?: boolean;
  roles?: string[];
  tenants?: string[];
  status?: 'active' | 'disabled';
}

export interface UserStatusPayload {
  status: 'active' | 'disabled';
}

export interface RolePayload {
  name: string;
  description?: string;
  permissions: string[];
  scope?: 'global' | 'tenant';
}

export interface GlobalConfig {
  maintenanceMode: boolean;
  sessionTimeoutMinutes: number;
  passwordPolicy?: string;
  auditEnabled?: boolean;
}

export interface NotificationChannelConfig {
  type: 'email' | 'webhook' | 'sms' | 'pagerduty';
  target: string;
  enabled: boolean;
  severity?: Array<'info' | 'warning' | 'critical'>;
}

export interface NotificationConfig {
  channels: NotificationChannelConfig[];
  defaultSeverity?: Array<'info' | 'warning' | 'critical'>;
}

export interface BackupSnapshot {
  id: string;
  label: string;
  type: 'full' | 'incremental';
  scope: 'global' | 'tenant';
  tenantId?: string;
  tenantName?: string;
  sizeGb: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  storedAt: string;
  createdAt: string;
  expiresAt?: string;
  createdBy: string;
  checksum?: string;
  includesSecrets?: boolean;
}

export interface BackupPolicy {
  id: string;
  name: string;
  cadence: string;
  retentionDays: number;
  scope: 'global' | 'tenant';
  enabled: boolean;
  nextRunAt?: string;
  lastRunAt?: string;
}

export interface BackupGuardrail {
  key: string;
  title: string;
  status: 'ok' | 'warning' | 'failed';
  detail?: string;
  lastCheckedAt?: string;
}

export interface BackupRestoreJob {
  id: string;
  snapshotId: string;
  snapshotLabel: string;
  mode: 'validate' | 'replace';
  status: 'planned' | 'running' | 'completed' | 'failed';
  initiatedBy: string;
  targetTenantId?: string;
  targetTenantName?: string;
  startedAt?: string;
  completedAt?: string;
  notes?: string;
}

export interface BackupOverview {
  latestSnapshot?: BackupSnapshot;
  snapshots: BackupSnapshot[];
  policies: BackupPolicy[];
  restoreJobs: BackupRestoreJob[];
  guardrails: BackupGuardrail[];
}

export interface SnapshotCreatePayload {
  label: string;
  scope: 'global' | 'tenant';
  tenantId?: string;
  includeSecrets?: boolean;
  expedite?: boolean;
}

export interface SnapshotRestorePayload {
  snapshotId: string;
  mode: 'validate' | 'replace';
  targetTenantId?: string;
  approveInstantly?: boolean;
  notes?: string;
}
