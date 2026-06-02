/**
 * Menu definitions use i18n message keys for titles.
 * The `titleKey` field stores the key under `menu.*` namespace.
 * At render time, use `$t('menu.' + item.titleKey)` or `t('menu.' + name)`.
 */
export const menuGroups = [
  {
    path: '/dashboard',
    name: 'dashboard',
    titleKey: 'dashboard',
    icon: 'Odometer',
    children: [
      { path: '/dashboard/overview', name: 'dashboard-overview', titleKey: 'dashboard-overview', view: 'overview' },
      { path: '/dashboard/domain-status', name: 'dashboard-domain-status', titleKey: 'dashboard-domain-status', view: 'domain-status' },
      { path: '/dashboard/resource', name: 'dashboard-resource', titleKey: 'dashboard-resource', view: 'resource' },
    ],
  },
  {
    path: '/domain',
    name: 'domain',
    titleKey: 'domain',
    icon: 'Connection',
    children: [
      { path: '/domain/zone-list', name: 'domain-zone-list', titleKey: 'domain-zone-list', view: 'zone-list' },
    ],
  },
  {
    path: '/forward',
    name: 'forward',
    titleKey: 'forward',
    icon: 'SwitchFilled',
    children: [
      { path: '/forward/rules', name: 'forward-rules', titleKey: 'forward-rules', view: 'rules' },
      { path: '/forward/load-balance', name: 'forward-load-balance', titleKey: 'forward-load-balance', view: 'load-balance' },
      { path: '/forward/cache-domain', name: 'forward-cache-domain', titleKey: 'forward-cache-domain', view: 'cache-domain' },
      { path: '/forward/cache-strategy', name: 'forward-cache-strategy', titleKey: 'forward-cache-strategy', view: 'cache-strategy' },
    ],
  },
  {
    path: '/security',
    name: 'security',
    titleKey: 'security',
    icon: 'Lock',
    children: [
      { path: '/security/domain-access', name: 'security-domain-access', titleKey: 'security-domain-access', view: 'domain-access' },
      { path: '/security/ddos', name: 'security-ddos', titleKey: 'security-ddos', view: 'ddos' },
      { path: '/security/dnssec', name: 'security-dnssec', titleKey: 'security-dnssec', view: 'dnssec' },
      { path: '/security/cert', name: 'security-cert', titleKey: 'security-cert', view: 'cert' },
    ],
  },
  {
    path: '/monitor',
    name: 'monitor',
    titleKey: 'monitor',
    icon: 'Monitor',
    children: [
      { path: '/monitor/alert-notice', name: 'monitor-alert-notice', titleKey: 'monitor-alert-notice', view: 'alert-notice' },
      { path: '/monitor/real-time', name: 'monitor-real-time', titleKey: 'monitor-real-time', view: 'real-time' },
      { path: '/monitor/resolve-log', name: 'monitor-resolve-log', titleKey: 'monitor-resolve-log', view: 'resolve-log' },
      { path: '/monitor/rule', name: 'monitor-rule', titleKey: 'monitor-rule', view: 'rule' },
      { path: '/monitor/report', name: 'monitor-report', titleKey: 'monitor-report', view: 'report' },
      { path: '/monitor/alert-subscribe', name: 'monitor-alert-subscribe', titleKey: 'monitor-alert-subscribe', view: 'alert-subscribe' },
      { path: '/monitor/slow-query', name: 'monitor-slow-query', titleKey: 'monitor-slow-query', view: 'slow-query' },
      { path: '/monitor/client-analysis', name: 'monitor-client-analysis', titleKey: 'monitor-client-analysis', view: 'client-analysis' },
    ],
  },
  {
    path: '/tools',
    name: 'tools',
    titleKey: 'tools',
    icon: 'Tools',
    children: [
      { path: '/tools/dig', name: 'tools-dig', titleKey: 'tools-dig', view: 'dig' },
      { path: '/tools/global-test', name: 'tools-global-test', titleKey: 'tools-global-test', view: 'global-test' },
      { path: '/tools/ip-location', name: 'tools-ip-location', titleKey: 'tools-ip-location', view: 'ip-location' },
    ],
  },
  {
    path: '/cluster',
    name: 'cluster',
    titleKey: 'cluster',
    icon: 'Cpu',
    children: [
      { path: '/cluster/overview', name: 'cluster-overview', titleKey: 'cluster-overview', view: 'overview' },
      { path: '/cluster/nodes', name: 'cluster-nodes', titleKey: 'cluster-nodes', view: 'nodes' },
      { path: '/cluster/config-sync', name: 'cluster-config-sync', titleKey: 'cluster-config-sync', view: 'config-sync' },
    ],
  },
  {
    path: '/setting',
    name: 'setting',
    titleKey: 'setting',
    icon: 'Setting',
    children: [
      { path: '/setting/general', name: 'setting-general', titleKey: 'setting-general', view: 'general' },
      { path: '/setting/users', name: 'setting-users', titleKey: 'setting-users', view: 'users' },
      { path: '/setting/api-keys', name: 'setting-api-keys', titleKey: 'setting-api-keys', view: 'api-keys' },
      { path: '/setting/backup', name: 'setting-backup', titleKey: 'setting-backup', view: 'backup' },
      { path: '/setting/notice', name: 'setting-notice', titleKey: 'setting-notice', view: 'notice' },
      { path: '/setting/audit-log', name: 'setting-audit-log', titleKey: 'setting-audit-log', view: 'audit-log' },
    ],
  },
]

export const flatMenus = menuGroups.flatMap((group) =>
  group.children.map((item) => ({
    ...item,
    parentPath: group.path,
    parentName: group.name,
    parentTitleKey: group.titleKey,
    icon: group.icon,
  })),
)