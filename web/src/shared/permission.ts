import type { RouteLocationNormalized } from 'vue-router';
import type { User } from '@/modules/auth/types';
import type { usePermissionStore } from '@/store/permission';

const SUPER_ADMIN_NAME = 'admin';
export const ADMIN_ROLE_PERMISSION = '__role_admin__';

export const isSuperAdminUser = (user?: Pick<User, 'username' | 'roles'> | null): boolean => {
  if (!user) return false;
  const username = user.username?.trim().toLowerCase();
  if (username === SUPER_ADMIN_NAME) return true;
  return !!user.roles?.some((role) => role.name?.trim().toLowerCase() === SUPER_ADMIN_NAME);
};

export const isAdminRoleUser = (user?: Pick<User, 'roles'> | null): boolean => {
  if (!user) return false;
  return !!user.roles?.some((role) => role.name?.trim().toLowerCase() === SUPER_ADMIN_NAME);
};

export const hasPermission = (
  permission: string | string[] | undefined,
  permissionStore: ReturnType<typeof usePermissionStore>,
  user?: Pick<User, 'username' | 'roles'> | null
) => {
  if (!permission) return true;
  if (isSuperAdminUser(user)) return true;
  if (permission === ADMIN_ROLE_PERMISSION) return isAdminRoleUser(user);
  if (Array.isArray(permission)) return permission.some((p) => permissionStore.can(p));
  return permissionStore.can(permission);
};

export const ensureRoutePermission = (
  _to: RouteLocationNormalized,
  permissionStore: ReturnType<typeof usePermissionStore>,
  user?: Pick<User, 'username' | 'roles'> | null
) => {
  const permission = _to.meta?.permission as string | string[] | undefined;
  if (hasPermission(permission, permissionStore, user)) return null;
  return { path: '/forbidden' };
};
