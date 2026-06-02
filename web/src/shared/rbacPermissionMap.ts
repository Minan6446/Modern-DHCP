import type { PermissionNode } from '@/types/system';

interface MenuPermissionLeaf {
  key: string;
  label: string;
  capability: string;
}

interface MenuPermissionGroup {
  key: string;
  label: string;
  children: MenuPermissionLeaf[];
}

const MENU_PERMISSION_TREE: MenuPermissionGroup[] = [
  {
    key: 'menu.pool',
    label: '地址池',
    children: [
      { key: 'menu.pool.ipv4', label: 'IPv4地址池（查看）', capability: 'pool.read' },
      { key: 'menu.pool.ipv6', label: 'IPv6地址池（查看）', capability: 'pool.read' },
      { key: 'menu.pool.analytics', label: '地址池分析（查看）', capability: 'pool.read' },
      { key: 'menu.pool.manage', label: '地址池（管理）', capability: 'pool.write' }
    ]
  },
  {
    key: 'menu.lease',
    label: '租约管理',
    children: [
      { key: 'menu.lease.active', label: '活跃租约（查看）', capability: 'lease.read' },
      { key: 'menu.lease.history', label: '租约历史（查看）', capability: 'lease.read' },
      { key: 'menu.lease.policy', label: '租约策略（管理）', capability: 'lease.manage' }
    ]
  },
  {
    key: 'menu.binding',
    label: '静态绑定',
    children: [
      { key: 'menu.binding.mac', label: 'MAC地址绑定（查看）', capability: 'binding.read' },
      { key: 'menu.binding.manage', label: '静态绑定（管理）', capability: 'binding.manage' }
    ]
  },
  {
    key: 'menu.option',
    label: 'DHCP 选项',
    children: [
      { key: 'menu.option.list', label: '选项列表（查看）', capability: 'policy.read' },
      { key: 'menu.option.templates', label: '配置模板（查看）', capability: 'policy.read' },
      { key: 'menu.option.scopes', label: '作用域管理（管理）', capability: 'policy.write' }
    ]
  },
  {
    key: 'menu.monitor',
    label: '监控告警',
    children: [
      { key: 'menu.monitor.overview', label: '实时监控（查看）', capability: 'report.read' },
      { key: 'menu.monitor.history', label: '历史告警（查看）', capability: 'report.read' },
      { key: 'menu.monitor.analytics', label: '统计分析（查看）', capability: 'report.read' },
      { key: 'menu.monitor.config', label: '告警配置（管理）', capability: 'report.read' }
    ]
  },
  {
    key: 'menu.cluster',
    label: '集群管理',
    children: [
      { key: 'menu.cluster.overview', label: '集群运行概览（查看）', capability: 'ha.read' },
      { key: 'menu.cluster.config', label: '集群配置管理（管理）', capability: 'ha.manage' },
      { key: 'menu.cluster.auditdr', label: '同步审计与灾备（管理）', capability: 'ha.manage' }
    ]
  },
  {
    key: 'menu.security',
    label: '安全防护',
    children: [
      { key: 'menu.security.overview', label: '安全与防护（查看）', capability: 'security.view' },
      { key: 'menu.security.access', label: '接入安全控制（查看）', capability: 'security.view' },
      { key: 'menu.security.mac', label: 'MAC 名单（查看）', capability: 'security.view' },
      { key: 'menu.security.network', label: '网络安全防护（查看）', capability: 'security.view' },
      { key: 'menu.security.threat', label: '威胁防护（查看）', capability: 'security.view' },
      { key: 'menu.security.policy.read', label: '安全策略（查看）', capability: 'security.policy.read' },
      {
        key: 'menu.security.policy.manage',
        label: '安全策略（管理）',
        capability: 'security.policy.manage'
      },
      { key: 'menu.security.manage', label: '安全功能（管理）', capability: 'security.manage' }
    ]
  },
  {
    key: 'menu.system',
    label: '系统管理',
    children: [
      { key: 'menu.system.users', label: '用户管理（查看）', capability: 'auth.user.read' },
      { key: 'menu.system.users.manage', label: '用户管理（管理）', capability: 'auth.user.manage' },
      { key: 'menu.system.apikey', label: 'API Key（管理）', capability: 'auth.apikey.manage' },
      { key: 'menu.system.roles', label: '角色与权限（查看）', capability: 'rbac.role.read' },
      { key: 'menu.system.roles.manage', label: '角色与权限（管理）', capability: 'rbac.role.manage' },
      {
        key: 'menu.system.assignments.read',
        label: '角色分配（查看）',
        capability: 'rbac.assignment.read'
      },
      {
        key: 'menu.system.assignments.manage',
        label: '角色分配（管理）',
        capability: 'rbac.assignment.write'
      },
      { key: 'menu.system.quota', label: '租户配额（查看）', capability: 'tenant.quota.read' },
      { key: 'menu.system.quota.manage', label: '租户配额（管理）', capability: 'tenant.quota.write' },
      { key: 'menu.system.audit', label: '日志审计（查看）', capability: 'audit.read' }
    ]
  }
];

const menuKeyToCapability = new Map<string, string>();
for (const group of MENU_PERMISSION_TREE) {
  for (const item of group.children) {
    menuKeyToCapability.set(item.key, item.capability);
  }
}

const capabilityToMenuKeys = new Map<string, string[]>();
for (const [menuKey, capability] of menuKeyToCapability) {
  const list = capabilityToMenuKeys.get(capability) || [];
  list.push(menuKey);
  capabilityToMenuKeys.set(capability, list);
}

export const buildRolePermissionTree = (availableCapabilities: string[]): PermissionNode[] => {
  const available = new Set((availableCapabilities || []).map((item) => String(item || '').trim()));
  return MENU_PERMISSION_TREE.map((group) => {
    const children = group.children
      .filter((item) => available.size === 0 || available.has(item.capability))
      .map((item) => ({ key: item.key, label: item.label } as PermissionNode));
    return {
      key: group.key,
      label: group.label,
      children
    } as PermissionNode;
  }).filter((group) => (group.children || []).length > 0);
};

export const menuSelectionToCapabilities = (keys: string[]): string[] => {
  const set = new Set<string>();
  for (const raw of keys || []) {
    const key = String(raw || '').trim();
    if (!key) continue;
    const mapped = menuKeyToCapability.get(key);
    if (mapped) {
      set.add(mapped);
      continue;
    }
    if (key.includes('.') && !key.startsWith('menu.')) {
      set.add(key);
    }
  }
  return Array.from(set.values());
};

export const capabilitiesToMenuSelection = (capabilities: string[]): string[] => {
  const selected = new Set<string>();
  for (const raw of capabilities || []) {
    const capability = String(raw || '').trim();
    if (!capability) continue;
    const menuKeys = capabilityToMenuKeys.get(capability) || [];
    if (menuKeys.length) {
      for (const key of menuKeys) {
        selected.add(key);
      }
    } else {
      selected.add(capability);
    }
  }
  return Array.from(selected.values());
};
