import { defineStore } from 'pinia';
import type { PermissionNode } from '@/types/system';
import { getCapabilities, extractGrantedCapabilities } from '@/modules/auth/api';
import { useAuthStore } from '@/modules/auth/store';
import { ADMIN_ROLE_PERMISSION, isAdminRoleUser, isSuperAdminUser } from '@/shared/permission';
import { buildRolePermissionTree } from '@/shared/rbacPermissionMap';

interface PermissionState {
  permissions: Set<string>;
  tree: PermissionNode[];
  loading: boolean;
}

const permissionAliases: Record<string, string[]> = {
  'dashboard.view': [
    'pool.read',
    'lease.read',
    'binding.read',
    'policy.read',
    'report.read',
    'security.view',
    'ha.read'
  ],
  'binding.view': ['binding.read'],
  'option.view': ['policy.read'],
  'option.manage': ['policy.write'],
  'pool.ipv4.view': ['pool.read'],
  'pool.ipv6.view': ['pool.read'],
  'pool.analytics.view': ['pool.read'],
  'pool.ipv4.manage': ['pool.write'],
  'pool.ipv6.manage': ['pool.write'],
  'lease.view': ['lease.read'],
  'monitor.view': ['report.read'],
  'monitor.manage': ['report.read'],
  'monitoring.view': ['report.read']
};

export const usePermissionStore = defineStore('permission', {
  state: (): PermissionState => ({
    permissions: new Set<string>(),
    tree: [],
    loading: false
  }),
  actions: {
    async loadPermissions() {
      this.loading = true;
      try {
        const auth = useAuthStore();
        if (isSuperAdminUser(auth.user)) {
          this.permissions = new Set(['*']);
          this.tree = [];
          return;
        }
        // Authorize strictly from granted capabilities
        const caps = await getCapabilities();
        const granted = extractGrantedCapabilities(caps);
        this.permissions = new Set(granted);
        this.tree = buildRolePermissionTree(granted);
      } finally {
        this.loading = false;
      }
    },
    setPermissions(keys: string[]) {
      const auth = useAuthStore();
      if (isSuperAdminUser(auth.user)) {
        this.permissions = new Set(['*']);
      } else {
        this.permissions = new Set(keys);
      }
    },
    can(key?: string) {
      const auth = useAuthStore();
      if (isSuperAdminUser(auth.user)) return true;
      if (!key) return true;
      if (key === ADMIN_ROLE_PERMISSION) return isAdminRoleUser(auth.user);
      if (this.permissions.has('*')) return true;
      if (this.permissions.has(key)) return true;
      const aliases = permissionAliases[key] || [];
      return aliases.some((alias) => this.permissions.has(alias));
    },
    hasAny(keys: string[]) {
      const auth = useAuthStore();
      if (isSuperAdminUser(auth.user)) return true;
      if (!keys.length) return true;
      if (this.permissions.has('*')) return true;
      return keys.some((k) => this.permissions.has(k));
    },
    reset() {
      this.permissions = new Set();
      this.tree = [];
      this.loading = false;
    }
  }
});
