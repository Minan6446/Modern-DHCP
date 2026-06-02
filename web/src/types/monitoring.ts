export interface MetricPoint {
  timestamp: string;
  value: number;
}

export interface PerfSnapshot {
  pps: number;
  cpu: number;
  cpuCores?: number;
  memory: number;
  memoryUsedBytes?: number;
  memoryTotalBytes?: number;
  disk: number;
  diskUsedBytes?: number;
  diskTotalBytes?: number;
  latencyP50: number;
  latencyP95: number;
  latencyP99: number;
  successRate: number;
}

export interface ServerPerformanceMetric {
  node: string;
  region?: string;
  packetsPerSecond: number;
  bandwidthMbps: number;
  queueDepth: number;
  errorRate: number;
  threadUtilization: number;
}

export interface AllocationBreakdown {
  total: number;
  success: number;
  failures: Array<{ reason: string; count: number }>;
}

export interface PoolUsageSummary {
  poolId: string;
  poolName: string;
  used: number;
  total: number;
  utilization: number;
}

export interface PoolUsageSummarySnapshot {
  totalPools: number;
  ipv4Pools: number;
  ipv6Pools: number;
  totalIps: number;
  allocatedIps: number;
  availableIps: number;
}

export interface LeaseStatusSummary {
  active: number;
  expired: number;
  pending: number;
  failed: number;
}

export interface AnomalyRecord {
  id: string;
  type: string;
  message: string;
  score: number;
  occurredAt: string;
}

export interface SyntheticCheck {
  id: string;
  name: string;
  status: 'pass' | 'fail' | 'degraded';
  latencyMs: number;
  lastRunAt: string;
}

export interface AbnormalRequestStat {
  id: string;
  type: string;
  count: number;
  source?: string;
  lastSeenAt: string;
}

export interface AlertRule {
  id: string;
  name: string;
  metric: string;
  operator: '>' | '>=' | '<' | '<=';
  threshold: number;
  duration: number;
  severity: 'info' | 'warning' | 'critical';
  enabled: boolean;
  match?: string;
}

export interface AlertEvent {
  id: string;
  ruleId: string;
  severity: 'info' | 'warning' | 'critical' | 'emergency';
  message: string;
  status: 'firing' | 'resolved' | 'ack';
  assignee?: string;
  createdAt: string;
  resolvedAt?: string;
  ip?: string;
  type?: string;
}

export interface NotificationChannel {
  name: string;
  id: string;
  type: 'email' | 'webhook' | 'sms' | 'pagerduty';
  target: string;
  enabled: boolean;
}

export interface AlertActiveSummary {
  active: number;
  emergency: number;
  critical: number;
  warning: number;
  info: number;
  servers?: { healthy: number; total: number };
}

export interface AlertTrendPoint {
  ts: string;
  emergency: number;
  critical: number;
  warning: number;
  info: number;
}

export interface AlertHistorySummary {
  total: number;
  resolutionRate: number;
  inProgress: number;
}

export interface AlertHistoryCompare {
  current: { count: number; resolutionRate: number; mttr?: number | string };
  previous: { count: number; resolutionRate: number; mttr?: number | string };
}

export interface AlertThresholdConfig {
  resource: {
    poolUsage: number;
    leaseUsage: number;
    renewFail: number;
    leaseTimeDrift?: number;
    failedRequestRatio?: number;
    subnetImbalance?: number;
    logErrorThreshold?: number;
  };
  server: {
    responseTimeout: number;
    responseTimeMs?: number;
    cpuUsage?: number;
    memoryUsage?: number;
    processCheck: boolean;
  };
  network: {
    conflictSensitivity: 'high' | 'medium' | 'low';
    abnormalQps: number;
    duplicateIpDetection?: boolean;
    unauthorizedServerDetection?: boolean;
  };
}

export interface AlertNotifyConfig {
  channels: { email: boolean; sms: boolean; webhook: boolean; webhookUrl?: string };
  policies: {
    emergency: Array<'email' | 'sms' | 'webhook'>;
    critical: Array<'email' | 'webhook'>;
    info: Array<'email'>;
  };
}

export interface AlertTemplate {
  id: string;
  name: string;
  lang: string;
  channel: 'email' | 'sms' | 'webhook';
  subject?: string;
  body?: string;
  variables?: string[];
  updatedAt?: string;
}

export interface AlertReceiver {
  id: string;
  name: string;
  email: string;
  phone?: string;
  levels: Array<'emergency' | 'critical' | 'info' | 'all'>;
  department?: string;
  schedule?: string;
  serverGroups?: string[];
}

export interface AlertAnalyticsSummary {
  total: number;
  mttr: string | number;
  topType: string;
  resolutionRate: number;
}

export interface AlertTypeDistributionItem {
  type: string;
  count: number;
}

export interface AlertTimeDistributionItem {
  hour: number;
  count: number;
}

export interface AlertTrendCompare {
  current: Array<{ ts: string; count: number }>;
  previous: Array<{ ts: string; count: number }>;
  anomalies?: Array<{ ts: string; desc: string }>;
}

export interface ReportRequest {
  type: 'ops-daily' | 'performance' | 'capacity' | 'security';
  range: { from: string; to: string };
  format: 'pdf' | 'csv';
}

export interface ReportTask {
  id: string;
  type: ReportRequest['type'];
  status: 'pending' | 'running' | 'done' | 'failed';
  downloadUrl?: string;
  createdAt: string;
}
