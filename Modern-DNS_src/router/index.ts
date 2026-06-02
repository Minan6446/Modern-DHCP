import { h } from 'vue'
import { createRouter, createWebHistory, RouterView } from 'vue-router'
import i18n from '../i18n'
import AppLayout from '../layout/AppLayout.vue'
import { flatMenus } from '../constants/menu'
import { useAppStore } from '../stores/app'

const SettingRouteView = {
  render: () => h(RouterView),
}

const menuComponentMap = {
  'dashboard-overview': () => import('../views/dashboard/OverviewPage.vue'),
  'dashboard-domain-status': () => import('../views/dashboard/OverviewPage.vue'),
  'dashboard-resource': () => import('../views/dashboard/OverviewPage.vue'),

  'domain-zone-list': () => import('../views/domain/ZoneListPage.vue'),

  'forward-rules': () => import('../views/forward/ForwardRulesPage.vue'),
  'forward-load-balance': () => import('../views/forward/LoadBalancePage.vue'),
  'forward-cache-domain': () => import('../views/cache/DomainCachePage.vue'),
  'forward-cache-strategy': () => import('../views/cache/CacheStrategyPage.vue'),

  'security-domain-access': () => import('../views/security/DomainAccessPage.vue'),
  'security-ddos': () => import('../views/security/BlackWhitePage.vue'),
  'security-dnssec': () => import('../views/security/DnssecPage.vue'),
  'security-cert': () => import('../views/security/CertManagePage.vue'),

  'monitor-alert-notice': () => import('../views/monitor/AlertNoticePage.vue'),
  'monitor-real-time': () => import('../views/monitor/RealTimePage.vue'),
  'monitor-resolve-log': () => import('../views/monitor/ResolveLogPage.vue'),
  'monitor-rule': () => import('../views/monitor/RulePage.vue'),
  'monitor-report': () => import('../views/monitor/ReportPage.vue'),
  'monitor-alert-subscribe': () => import('../views/monitor/AlertSubscribePage.vue'),
  'monitor-slow-query': () => import('../views/monitor/SlowQueryPage.vue'),
  'monitor-client-analysis': () => import('../views/monitor/ClientAnalysisPage.vue'),

  'tools-dig': () => import('../views/tools/DigPage.vue'),
  'tools-global-test': () => import('../views/tools/DigPage.vue'),
  'tools-ip-location': () => import('../views/tools/DigPage.vue'),

  'cluster-overview': () => import('../views/cluster/ClusterOverviewPage.vue'),
  'cluster-nodes': () => import('../views/cluster/ClusterPage.vue'),
  'cluster-config-sync': () => import('../views/cluster/ClusterConfigSyncPage.vue'),

} as Record<string, () => Promise<unknown>>

const menuRoutes = flatMenus.filter((item) => item.parentPath !== '/setting').map((item) => ({
  path: item.path.replace(/^\//, ''),
  name: item.name,
  component: menuComponentMap[item.name],
  meta: {
    titleKey: item.titleKey,
    parentTitleKey: item.parentTitleKey,
    menuPath: item.path,
    view: item.view,
  },
}))

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/system/LoginPage.vue'),
      meta: { titleKey: 'login-title' },
    },
    {
      path: '/',
      component: AppLayout,
      meta: { titleKey: 'app-console' },
      children: [
        { path: '', redirect: '/login' },
        {
          path: 'profile',
          name: 'profile',
          component: () => import('../views/system/ProfilePage.vue'),
          meta: { titleKey: 'profile-title' },
        },
        { path: 'monitor/alert', redirect: '/monitor/alert-notice' },
        { path: 'forward/global', redirect: '/forward/rules' },
        { path: 'forward/condition', redirect: '/forward/rules' },
        { path: 'security/black-white', redirect: '/security/domain-access' },
        { path: 'security/acl', redirect: '/security/domain-access' },
        { path: 'security/rpz', redirect: '/security/domain-access' },
        { path: 'tools/dnssec-debug', redirect: '/security/dnssec' },
        { path: 'cache', redirect: '/forward/cache-domain' },
        { path: 'cache/domain', redirect: '/forward/cache-domain' },
        { path: 'cache/strategy', redirect: '/forward/cache-strategy' },
        { path: 'forward', redirect: '/forward/rules' },
        { path: 'cluster', redirect: '/cluster/overview' },
        { path: 'security', redirect: '/security/domain-access' },
        { path: 'monitor', redirect: '/monitor/alert-notice' },
        { path: 'tools', redirect: '/tools/dig' },
        {
          path: 'setting',
          component: SettingRouteView,
          redirect: '/setting/general',
          meta: { titleKey: 'setting' },
          children: [
            {
              path: 'general',
              name: 'setting-general',
              component: () => import('@/views/setting/GeneralSettingPage.vue'),
              meta: {
                titleKey: 'setting-general',
                parentTitleKey: 'setting',
                menuPath: '/setting/general',
                view: 'general',
                icon: 'Setting',
              },
            },
            {
              path: 'users',
              name: 'setting-users',
              component: () => import('@/views/setting/UserManagePage.vue'),
              meta: {
                titleKey: 'setting-users',
                parentTitleKey: 'setting',
                menuPath: '/setting/users',
                view: 'users',
                icon: 'Setting',
              },
            },
            {
              path: 'backup',
              name: 'setting-backup',
              component: () => import('@/views/setting/BackupRestorePage.vue'),
              meta: {
                titleKey: 'setting-backup',
                parentTitleKey: 'setting',
                menuPath: '/setting/backup',
                view: 'backup',
                icon: 'Setting',
              },
            },
            {
              path: 'notice',
              name: 'setting-notice',
              component: () => import('@/views/setting/NotificationSettings.vue'),
              meta: {
                titleKey: 'setting-notice',
                parentTitleKey: 'setting',
                menuPath: '/setting/notice',
                view: 'notice',
                icon: 'Setting',
              },
            },
            {
              path: 'api-keys',
              name: 'setting-api-keys',
              component: () => import('@/views/setting/ApiKeysPage.vue'),
              meta: {
                titleKey: 'setting-api-keys',
                parentTitleKey: 'setting',
                menuPath: '/setting/api-keys',
                view: 'api-keys',
                icon: 'Setting',
              },
            },
            {
              path: 'audit-log',
              name: 'setting-audit-log',
              component: () => import('@/views/setting/AuditLogPage.vue'),
              meta: {
                titleKey: 'setting-audit-log',
                parentTitleKey: 'setting',
                menuPath: '/setting/audit-log',
                view: 'audit-log',
                icon: 'Setting',
              },
            },
            {
              path: 'logs',
              name: 'setting-logs',
              redirect: '/setting/audit-log',
            },
          ],
        },
        ...menuRoutes,
        {
          path: 'domain/zone-edit/:id',
          name: 'domain-zone-edit',
          component: () => import('../views/domain/ZoneEditPage.vue'),
          meta: {
            titleKey: 'domain-zone-list',
            parentTitleKey: 'domain',
            menuPath: '/domain/zone-list',
            view: 'zone-edit',
          },
        },
      ],
    },
    {
      path: '/403',
      name: 'no-permission',
      component: () => import('../views/system/NoPermission.vue'),
      meta: { titleKey: 'no-permission' },
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/system/NotFound.vue'),
      meta: { titleKey: 'not-found' },
    },
  ],
})

router.beforeEach((to, from, next) => {
  const appStore = useAppStore()
  const isLoginRoute = to.path === '/login'

  if (!appStore.isAuthenticated && !isLoginRoute) {
    const DEFAULT_PATH = '/dashboard/overview'
    const query = to.fullPath !== DEFAULT_PATH ? { redirect: to.fullPath } : {}
    next({ path: '/login', query })
    return
  }

  if (appStore.isAuthenticated && isLoginRoute) {
    next('/dashboard/overview')
    return
  }

  appStore.setLoading(!isLoginRoute && from.path !== '/login')
  if (to.meta.menuPath) {
    appStore.setActiveMenu(String(to.meta.menuPath))
  }
  const titleKey = to.meta.titleKey as string | undefined
  const pageTitle = titleKey ? (i18n.global.t(`menu.${titleKey}` as string) !== `menu.${titleKey}` ? i18n.global.t(`menu.${titleKey}` as string) : i18n.global.t('app.defaultPageTitle')) : i18n.global.t('app.defaultPageTitle')
  document.title = `${pageTitle} - ${i18n.global.t('app.title')}`
  next()
})

router.afterEach(() => {
  const appStore = useAppStore()
  appStore.setLoading(false)
})

export default router