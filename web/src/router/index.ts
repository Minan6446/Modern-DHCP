import {
  createRouter,
  createWebHistory,
  type NavigationGuardNext,
  type RouteLocationNormalized,
  type RouteRecordRaw
} from 'vue-router';
import Layout from '@/layout/Layout.vue';
import { useAuthStore } from '@/modules/auth/store';
import { usePermissionStore } from '@/store/permission';
import {
  ADMIN_ROLE_PERMISSION,
  ensureRoutePermission,
  hasPermission as hasPermissionHelper
} from '@/shared/permission';

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('@/modules/auth/views/Login.vue'),
    meta: { public: true, title: '登录', requiresAuth: false }
  },
  {
    path: '/forbidden',
    component: () => import('@/views/system/Forbidden.vue'),
    meta: { public: true, title: '无权限' }
  },
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        component: () => import('@/views/dashboard/DashboardLayout.vue'),
        meta: { title: '仪表盘', permission: 'dashboard.view', requiresAuth: true }
      },
      {
        path: 'pool',
        redirect: '/pool/ipv4',
        meta: {
          title: '地址池管理',
          permission: ['pool.ipv4.view', 'pool.ipv6.view', 'pool.analytics.view'],
          requiresAuth: true
        }
      },
      {
        path: 'pool/ipv4',
        component: () => import('@/views/pool/PoolIPv4.vue'),
        meta: { title: 'IPv4地址池', permission: 'pool.ipv4.view', requiresAuth: true }
      },
      {
        path: 'pool/ipv6',
        component: () => import('@/views/pool/PoolIPv6.vue'),
        meta: { title: 'IPv6地址池', permission: 'pool.ipv6.view', requiresAuth: true }
      },
      {
        path: 'pool/analytics',
        component: () => import('@/views/pool/PoolAnalytics.vue'),
        meta: { title: '地址池分析', permission: 'pool.analytics.view', requiresAuth: true }
      },
      {
        path: 'lease',
        redirect: '/lease/active',
        meta: { title: '租约管理', permission: ['lease.view', 'lease.manage'], requiresAuth: true }
      },
      {
        path: 'lease/active',
        component: () => import('@/views/lease/LeaseActive.vue'),
        meta: { title: '活跃租约', permission: 'lease.view', requiresAuth: true }
      },
      {
        path: 'lease/history',
        component: () => import('@/views/lease/LeaseHistory.vue'),
        meta: { title: '租约历史', permission: 'lease.view', requiresAuth: true }
      },
      {
        path: 'binding',
        redirect: '/binding/mac',
        meta: {
          title: '绑定管理',
          permission: ['binding.view', 'binding.manage'],
          requiresAuth: true
        }
      },
      {
        path: 'binding/mac',
        component: () => import('@/views/binding/BindingMac.vue'),
        meta: { title: 'MAC地址绑定', permission: 'binding.view', requiresAuth: true }
      },
      {
        path: 'option',
        redirect: '/option/overview',
        meta: { title: '选项管理', permission: 'option.view', requiresAuth: true }
      },
      {
        path: 'option/list',
        component: () => import('@/views/option/OptionList.vue'),
        meta: { title: '选项列表', permission: 'option.view', requiresAuth: true }
      },
      {
        path: 'option/scopes',
        component: () => import('@/views/option/ScopeManagement.vue'),
        meta: { title: '作用域管理', permission: 'option.manage', requiresAuth: true }
      },
      {
        path: 'option/templates',
        component: () => import('@/views/option/TemplateConfig.vue'),
        meta: { title: '配置模板', permission: 'option.view', requiresAuth: true }
      },
      {
        path: 'monitor',
        redirect: '/monitor/overview',
        meta: { title: '监控与告警', permission: 'monitor.view', requiresAuth: true }
      },
      {
        path: 'monitor/overview',
        component: () => import('@/views/monitoring/Overview.vue'),
        meta: { title: '实时监控', permission: 'monitor.view', requiresAuth: true }
      },
      {
        path: 'monitor/history',
        component: () => import('@/views/monitoring/History.vue'),
        meta: { title: '历史告警', permission: 'monitor.view', requiresAuth: true }
      },
      {
        path: 'monitor/alerts',
        redirect: '/monitor/history',
        meta: { title: '告警中心', permission: 'monitor.view', requiresAuth: true }
      },
      {
        path: 'monitor/config',
        component: () => import('@/views/monitoring/Config.vue'),
        meta: { title: '告警配置', permission: 'monitor.manage', requiresAuth: true }
      },
      {
        path: 'monitor/analytics',
        component: () => import('@/views/monitoring/Analytics.vue'),
        meta: { title: '统计分析', permission: 'monitor.view', requiresAuth: true }
      },
      {
        path: 'cluster',
        redirect: '/cluster/overview',
        meta: { title: '集群管理', permission: ['ha.read', 'ha.manage'], requiresAuth: true }
      },
      {
        path: 'cluster/overview',
        component: () => import('@/views/cluster/ClusterRuntimeOverview.vue'),
        meta: { title: '集群运行概览', permission: ['ha.read', 'ha.manage'], requiresAuth: true }
      },
      {
        path: 'cluster/config',
        component: () => import('@/views/cluster/ClusterConfigManagement.vue'),
        meta: { title: '集群配置管理', permission: 'ha.manage', requiresAuth: true }
      },
      {
        path: 'cluster/sync-audit-dr',
        component: () => import('@/views/cluster/ClusterSyncAuditDr.vue'),
        meta: { title: '同步审计与灾备', permission: 'ha.manage', requiresAuth: true }
      },
      {
        path: 'security',
        component: () => import('@/views/security/SecurityOverview.vue'),
        meta: { title: '安全与防护', permission: 'security.view', requiresAuth: true }
      },
      {
        path: 'security/access',
        component: () => import('@/views/security/AccessControl.vue'),
        meta: { title: '接入安全控制', permission: 'security.view', requiresAuth: true }
      },
      {
        path: 'security/mac-lists',
        component: () => import('@/views/security/MacList.vue'),
        meta: { title: 'MAC 名单', permission: 'security.view', requiresAuth: true }
      },
      {
        path: 'security/network',
        component: () => import('@/views/security/NetworkDefense.vue'),
        meta: { title: '网络安全防护', permission: 'security.view', requiresAuth: true }
      },
      {
        path: 'security/threat',
        component: () => import('@/views/security/ThreatProtection.vue'),
        meta: { title: '威胁防护', permission: 'security.view', requiresAuth: true }
      },
      {
        path: 'system/roles',
        component: () => import('@/views/system/RoleManager.vue'),
        meta: {
          title: '用户管理',
          permission: ['rbac.role.read', 'auth.user.read', 'rbac.assignment.read'],
          requiresAuth: true
        }
      },
      {
        path: 'system/apikeys',
        component: () => import('@/views/system/ApiKeyManager.vue'),
        meta: { title: 'API Key', permission: ADMIN_ROLE_PERMISSION, requiresAuth: true }
      },
      {
        path: 'system/sessions',
        component: () => import('@/views/system/SessionManager.vue'),
        meta: { title: '会话管理', permission: ADMIN_ROLE_PERMISSION, requiresAuth: true }
      },
      {
        path: 'system/migration-tools',
        component: () => import('@/views/system/SystemMigrationTools.vue'),
        meta: { title: '数据迁移工具', permission: ADMIN_ROLE_PERMISSION, requiresAuth: true }
      },
      {
        path: 'system/login-audit',
        component: () => import('@/views/system/LoginAudit.vue'),
        meta: {
          title: '日志审计',
          permission: 'audit.read',
          requiresAuth: true,
          requiresTenant: false
        }
      },
      {
        path: 'system/smtp',
        component: () => import('@/views/system/SmtpConfig.vue'),
        meta: { title: '消息服务配置', permission: ADMIN_ROLE_PERMISSION, requiresAuth: true }
      },
      {
        path: 'system/general',
        component: () => import('@/views/system/SystemGeneralSettings.vue'),
        meta: { title: '通用设置', permission: ADMIN_ROLE_PERMISSION, requiresAuth: true }
      },
      {
        path: 'account/settings',
        component: () => import('@/views/account/Settings.vue'),
        meta: { title: '用户设置', requiresAuth: true }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    component: () => import('@/views/system/NotFound.vue'),
    meta: { public: true, title: '未找到' }
  }
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes
});

export const hasPermission = hasPermissionHelper;

export const ensureAuth = async (
  to: RouteLocationNormalized,
  auth: ReturnType<typeof useAuthStore>
) => {
  const isPublic = to.meta?.public === true;
  const requiresAuth = to.meta?.requiresAuth !== false;

  // Only run auth check for non-public routes to avoid needless calls on assets/login.
  if (!isPublic && !auth.isAuthenticated) {
    await auth.checkAuth();
  }

  if (to.path === '/login' && auth.isAuthenticated) {
    const redirect = auth.consumePostLoginRedirect() || '/';
    return { path: redirect };
  }

  if (requiresAuth && !auth.isAuthenticated) {
    auth.setPostLoginRedirect(to.fullPath);
    return { path: '/login' };
  }

  return null;
};

export const ensurePermission = (
  to: RouteLocationNormalized,
  permissionStore: ReturnType<typeof usePermissionStore>,
  auth: ReturnType<typeof useAuthStore>
) => {
  return ensureRoutePermission(to, permissionStore, auth.user);
};

router.beforeEach(
  async (
    to: RouteLocationNormalized,
    _from: RouteLocationNormalized,
    next: NavigationGuardNext
  ) => {
    const auth = useAuthStore();
    const permissionStore = usePermissionStore();

    const steps = [await ensureAuth(to, auth), ensurePermission(to, permissionStore, auth)];

    const redirect = steps.find(Boolean);
    if (redirect) return next(redirect as any);
    next();
  }
);

export default router;
