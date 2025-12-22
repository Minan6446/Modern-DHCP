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

function clone<T>(payload: T): T {
  return JSON.parse(JSON.stringify(payload))
}

function simulateLatency<T>(payload: T, delay = 280): Promise<T> {
  return new Promise((resolve) => {
    setTimeout(() => resolve(clone(payload)), delay)
  })
}

const systemManagementSummary: SystemManagementSummary = {
  authProviders: [
    { id: 'local', name: '内置账号', type: 'local', status: 'active' },
    { id: 'ldap', name: '企业 LDAP', type: 'ldap', status: 'active' },
    { id: 'sso', name: 'SSO / OAuth2', type: 'sso', status: 'planned' },
  ],
  tenants: [
    {
      tenantId: 'global-net',
      tenantName: 'Global NetOps',
      poolsUsed: 18,
      poolLimit: 32,
      leasesUsed: 12540,
      leaseLimit: 25000,
      staticBindingsUsed: 410,
      staticBindingLimit: 1000,
    },
    {
      tenantId: 'edge-iot',
      tenantName: 'Edge IoT',
      poolsUsed: 9,
      poolLimit: 12,
      leasesUsed: 6420,
      leaseLimit: 8000,
      staticBindingsUsed: 155,
      staticBindingLimit: 400,
    },
  ],
  roles: [
    { id: 'viewer', name: '只读员', description: '可查看所有资源，不允许修改', permissions: 42 },
    { id: 'operator', name: '操作员', description: '执行租约、地址池与自动化任务', permissions: 128 },
    { id: 'admin', name: '平台管理员', description: '管理租户、RBAC 与系统配置', permissions: 256 },
  ],
  apiKeys: [
    { id: 'key_01', label: 'Terraform Runner', scope: 'pools:read automation:run', lastUsedAt: '2025-12-16T02:15:00Z' },
    { id: 'key_02', label: 'CMDB Sync', scope: 'leases:read pools:read', lastUsedAt: '2025-12-15T22:02:00Z' },
  ],
  sessions: [
    {
      id: 'sess_a',
      user: 'alice.z',
      tenant: 'Global NetOps',
      issuedAt: '2025-12-16T01:20:00Z',
      expiresAt: '2025-12-16T05:20:00Z',
      active: true,
    },
    {
      id: 'sess_b',
      user: 'ops.bot',
      tenant: 'Edge IoT',
      issuedAt: '2025-12-15T22:00:00Z',
      expiresAt: '2025-12-16T04:00:00Z',
      active: true,
    },
  ],
}

const staticBindingSummary: StaticBindingSummary = {
  total: 812,
  active: 694,
  pendingImports: 2,
  bindings: [
    {
      id: 'binding-1',
      tenantId: 'global-net',
      device: 'VoIP-Desk-RT01',
      macAddress: '00:16:3e:12:ab:03',
      ipAddress: '10.10.20.35',
      hostname: 'voip-rt01',
      status: 'online',
      lastSeenAt: '2025-12-16T02:40:00Z',
    },
    {
      id: 'binding-2',
      tenantId: 'edge-iot',
      device: 'Factory-Sensor-88',
      macAddress: '00:1c:42:ff:00:88',
      ipAddress: '172.16.5.120',
      hostname: 'sensor-88',
      status: 'offline',
      lastSeenAt: '2025-12-15T17:10:00Z',
    },
  ],
}

const dhcpOptionCatalog: DhcpOptionCatalog = {
  standard: [
    { name: 'Router (Option 3)', code: 3, category: 'IPv4', description: '默认网关列表', usageCount: 128 },
    { name: 'DNS Servers (Option 6)', code: 6, category: 'IPv4', description: '主备 DNS 列表', usageCount: 124 },
    { name: 'Domain Name (Option 15)', code: 15, category: 'Core', description: '客户端加入的域名', usageCount: 98 },
  ],
  custom: [
    { name: 'Vendor Profile ID', code: 224, category: 'Custom', description: '厂商扩展字段', usageCount: 12 },
    { name: 'Edge Sensor Profile', code: 225, category: 'IoT', description: 'IoT 设备特定配置', usageCount: 6 },
  ],
  templates: [
    { name: '标准办公网络', options: 8, usedBy: 24, description: '常规员工网络配置模板' },
    { name: 'VoIP 终端', options: 5, usedBy: 6, description: '含 TFTP/NTP/SIP 配置' },
    { name: 'IoT 设备', options: 6, usedBy: 9, description: '限制选项 + 自定义厂商参数' },
  ],
}

const clusterOverview: ClusterOverview = {
  mode: 'active-active',
  failoverReady: true,
  replicationLagMs: 180,
  pendingActions: 1,
  nodes: [
    {
      id: 'controller-a',
      role: 'active',
      address: '10.0.0.11',
      version: '2.3.0',
      health: 'healthy',
      cpuPercent: 41.5,
      memoryPercent: 62.3,
      syncLagMs: 120,
      lastHeartbeat: '2025-12-16T02:55:00Z',
    },
    {
      id: 'controller-b',
      role: 'active',
      address: '10.0.0.12',
      version: '2.3.0',
      health: 'healthy',
      cpuPercent: 48.2,
      memoryPercent: 59.1,
      syncLagMs: 180,
      lastHeartbeat: '2025-12-16T02:55:04Z',
    },
    {
      id: 'controller-dr',
      role: 'standby',
      address: '10.0.5.20',
      version: '2.3.0',
      health: 'warning',
      cpuPercent: 22.5,
      memoryPercent: 44.2,
      syncLagMs: 640,
      lastHeartbeat: '2025-12-16T02:54:40Z',
    },
  ],
}

const securityOverview: SecurityOverview = {
  rateLimitHits: 184,
  snoopingViolations: 6,
  rogueServers: 1,
  controls: [
    { id: 'snooping', name: 'DHCP Snooping', status: 'enabled', lastEventAt: '2025-12-16T02:10:00Z' },
    { id: 'rate-limit', name: '端口级速率限制', status: 'enabled', severity: 'warning', lastEventAt: '2025-12-16T02:32:00Z' },
    { id: 'rogue-scan', name: '非法服务器扫描', status: 'degraded', severity: 'critical', lastEventAt: '2025-12-16T01:48:00Z' },
  ],
  recentFindings: [
    {
      id: 'alert-rogue-ap',
      summary: '检测到可疑 DHCP 应答',
      details: '交换机 SW-18：非信任端口收到 OFFER',
      category: 'security',
      severity: 'critical',
      lifecycle: 'open',
      source: 'snooping',
      tenantId: 'edge-iot',
      assignee: 'secops',
      channel: 'webhook',
      fingerprint: 'rogue::SW-18',
      tags: ['rogue', 'edge'],
      createdAt: '2025-12-16T02:41:00Z',
      updatedAt: '2025-12-16T02:41:00Z',
    },
    {
      id: 'alert-rate',
      summary: '租户 Edge IoT 触发速率限制',
      details: 'MAC 00:1c:42:ff:00:88 被临时限制 15 分钟',
      category: 'security',
      severity: 'warning',
      lifecycle: 'acknowledged',
      source: 'rate-limit',
      tenantId: 'edge-iot',
      fingerprint: 'ratelimit::001c42ff0088',
      tags: ['rate-limit'],
      createdAt: '2025-12-16T02:05:00Z',
      updatedAt: '2025-12-16T02:10:00Z',
    },
  ],
}

const integrationMatrix: IntegrationMatrix = {
  adapters: [
    {
      id: 'grafana',
      name: 'Grafana / Prometheus',
      type: 'monitoring',
      status: 'connected',
      endpoint: 'https://grafana.example.com',
      lastSyncAt: '2025-12-16T02:50:00Z',
    },
    {
      id: 'servicenow',
      name: 'ServiceNow ITSM',
      type: 'itsm',
      status: 'connected',
      endpoint: 'https://servicenow.example.com',
      lastSyncAt: '2025-12-16T02:20:00Z',
    },
    {
      id: 'cmdb',
      name: 'CMDB 双向同步',
      type: 'cmdb',
      status: 'warning',
      endpoint: 'https://cmdb.example.com',
      lastSyncAt: '2025-12-16T01:10:00Z',
    },
    {
      id: 'webhook-edge',
      name: 'Edge Webhook',
      type: 'webhook',
      status: 'disconnected',
      endpoint: 'https://hooks.edge.example.com',
    },
  ],
  webhookDeliveries24h: 182,
  apiCalls24h: 4260,
}

const maintenanceOverview: MaintenanceOverview = {
  backupsEnabled: true,
  backupWindow: '02:00-03:00 UTC',
  lastBackupAt: '2025-12-16T02:15:00Z',
  tasks: [
    { id: 'task-backup', title: '租约数据库全量备份', owner: 'db-team', window: '02:00-03:00', status: 'completed' },
    { id: 'task-upgrade', title: '2.3.1 预生产升级演练', owner: 'platform', window: 'Dec 20 01:00-03:00', status: 'scheduled' },
    { id: 'task-audit', title: '安全基线审计', owner: 'secops', window: 'Dec 22 04:00-05:00', status: 'scheduled' },
  ],
  forecasts: [
    { metric: '数据库存储', current: 68, limit: 100, projection30d: 82 },
    { metric: '对象存储备份', current: 45, limit: 80, projection30d: 57 },
    { metric: '消息队列', current: 52, limit: 90, projection30d: 70 },
  ],
}

const helpCenterSnapshot: HelpCenterSnapshot = {
  docs: [
    {
      id: 'doc-user-guide',
      title: '用户操作手册 v2.3',
      type: 'doc',
      updatedAt: '2025-12-10T09:00:00Z',
      link: '/docs/user-guide',
    },
    {
      id: 'doc-api',
      title: 'OpenAPI v2 参考',
      type: 'api',
      updatedAt: '2025-12-08T12:00:00Z',
      link: '/docs/api/openapi_v2.yaml',
    },
    {
      id: 'doc-faq',
      title: '常见故障排查 FAQ',
      type: 'faq',
      updatedAt: '2025-12-12T15:30:00Z',
      link: '/docs/troubleshooting',
    },
  ],
  openTickets: 3,
  latestVersion: '2.3.0',
  releaseHighlights: ['新增自动化作业抽屉', '监控仪表盘容量热点卡片', 'API 密钥粒度权限设计草案'],
}

export function mockSystemManagementSummary() {
  return simulateLatency(systemManagementSummary)
}

export function mockStaticBindingSummary() {
  return simulateLatency(staticBindingSummary)
}

export function mockDhcpOptionCatalog() {
  return simulateLatency(dhcpOptionCatalog)
}

export function mockClusterOverview() {
  return simulateLatency(clusterOverview)
}

export function mockSecurityOverview() {
  return simulateLatency(securityOverview)
}

export function mockIntegrationMatrix() {
  return simulateLatency(integrationMatrix)
}

export function mockMaintenanceOverview() {
  return simulateLatency(maintenanceOverview)
}

export function mockHelpCenterSnapshot() {
  return simulateLatency(helpCenterSnapshot)
}
