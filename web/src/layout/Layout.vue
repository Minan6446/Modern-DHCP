<template>
  <div class="app-layout">
    <header class="app-header">
      <div class="header-left">
        <div class="brand" @click="goHome">
          <img class="brand-logo" :src="logo" :alt="t('app.title')" />
          <span class="brand-name">{{ t('app.title') }}</span>
        </div>
        <el-breadcrumb v-if="breadcrumbs.length" separator="/" class="breadcrumb">
          <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
            <span class="breadcrumb-text" @click="handleBreadcrumb(item.path)">{{
              item.label
            }}</span>
          </el-breadcrumb-item>
        </el-breadcrumb>
      </div>
      <div class="header-right">
        <div class="action-group">
          <el-button text class="icon-btn" @click="toggleSidebar">
            <component :is="isCollapse ? Expand : Fold" />
          </el-button>
        </div>
        <div class="action-group"><NotificationBell /></div>
        <div class="action-group">
          <el-dropdown trigger="click">
            <span class="user-info">
              <el-avatar :size="32" :src="userAvatar" icon="UserFilled" />
              <span class="username">{{ authStore.user?.displayName || authStore.user?.username || t('layout.defaultUserName') }}</span>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="goSettings">{{ t('layout.settings') }}</el-dropdown-item>
                <el-dropdown-item divided @click="logout">{{ t('layout.logout') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
    </header>

    <div class="app-body">
      <aside class="sider" :class="{ collapsed: isCollapse }">
        <el-scrollbar class="sider-scroll">
          <el-menu
            :default-active="activeMenu"
            class="menu"
            :collapse="isCollapse"
            unique-opened
            router
          >
            <template v-for="item in filteredMenus" :key="item.key">
              <el-sub-menu
                v-if="item.children && item.children.length"
                :index="item.path || item.key"
              >
                <template #title>
                  <component :is="item.icon" class="menu-icon" />
                  <span>{{ item.label }}</span>
                </template>
                <template v-for="child in item.children" :key="child.key">
                  <el-sub-menu
                    v-if="child.children && child.children.length"
                    :index="child.path || child.key"
                  >
                    <template #title>
                      <component :is="child.icon" class="menu-icon" />
                      <span>{{ child.label }}</span>
                    </template>
                    <el-menu-item
                      v-for="grand in child.children"
                      :key="grand.key"
                      :index="grand.path || grand.key"
                      @click="handleMenuClick(grand)"
                    >
                      <component :is="grand.icon" class="menu-icon" />
                      <span>{{ grand.label }}</span>
                    </el-menu-item>
                  </el-sub-menu>
                  <el-menu-item
                    v-else
                    :index="child.path || child.key"
                    @click="handleMenuClick(child)"
                  >
                    <component :is="child.icon" class="menu-icon" />
                    <span>{{ child.label }}</span>
                  </el-menu-item>
                </template>
              </el-sub-menu>
              <el-menu-item v-else :index="item.path || item.key" @click="handleMenuClick(item)">
                <component :is="item.icon" class="menu-icon" />
                <span>{{ item.label }}</span>
              </el-menu-item>
            </template>
          </el-menu>
        </el-scrollbar>
      </aside>

      <main class="content">
        <div class="view-wrapper">
          <router-view v-slot="{ Component, route: slotRoute }">
            <keep-alive :include="cacheViews">
              <component :is="Component" :key="slotRoute.fullPath" />
            </keep-alive>
          </router-view>
        </div>
        <footer class="footer">{{ footerSlogan }}</footer>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue';
import logo from '@/assets/logo.svg';
import { useRoute, useRouter } from 'vue-router';
import type { Component } from 'vue';
import { usePermissionStore } from '@/store/permission';
import { useAuthStore } from '@/modules/auth/store';
import { useI18n } from 'vue-i18n';
import { showError, showSuccess } from '@/shared/errors/messageToast';
import {
  ADMIN_ROLE_PERMISSION,
  hasPermission as hasPermissionHelper
} from '@/shared/permission';
import { loadLocaleModule } from '@/i18n';
import NotificationBell from './NotificationBell.vue';
import {
  Fold,
  Expand,
  Bell,
  UserFilled,
  HomeFilled,
  List,
  Tickets,
  Key,
  Lock,
  Setting,
  Monitor,
  Guide,
  Connection,
  Grid,
  Document,
  Histogram,
  TrendCharts,
  Collection,
  Message
} from '@element-plus/icons-vue';

interface MenuItem {
  key: string;
  label: string;
  icon?: Component;
  path?: string;
  children?: MenuItem[];
  permission?: string | string[];
}

const { t } = useI18n();
const router = useRouter();
const route = useRoute();

const footerSlogan = computed(() => String(t('layout.footerSlogan') || ''));

const permissionStore = usePermissionStore();
const authStore = useAuthStore();

const diceBearAvatar = (seed: string) =>
  `https://api.dicebear.com/9.x/thumbs/svg?seed=${encodeURIComponent(seed || 'user')}`;

onMounted(async () => {
  await permissionStore.loadPermissions();
  // Preload sidebar/global i18n modules so all nav items render in the active locale.
  const sidebarModules = ['system', 'cluster', 'security', 'settings', 'login'];
  await Promise.allSettled(sidebarModules.map((m) => loadLocaleModule(m)));
});

const isCollapse = ref(false);

const userAvatar = computed(() => {
  const name = authStore.user?.username || t('layout.defaultUserName');
  return diceBearAvatar(name || 'user');
});

const hasPermission = (key?: string | string[]) => {
  return hasPermissionHelper(key, permissionStore, authStore.user);
};

const menus = computed<MenuItem[]>(() => [
  {
    key: 'dashboard',
    label: t('nav.dashboard'),
    icon: HomeFilled,
    path: '/dashboard',
    permission: 'dashboard.view'
  },
  {
    key: 'pool',
    label: t('nav.pool'),
    icon: Grid,
    path: '/pool/ipv4',
    children: [
      {
        key: 'pool-ipv4',
        label: t('nav.poolIPv4'),
        icon: Histogram,
        path: '/pool/ipv4',
        permission: 'pool.ipv4.view'
      },
      {
        key: 'pool-ipv6',
        label: t('nav.poolIPv6'),
        icon: Connection,
        path: '/pool/ipv6',
        permission: 'pool.ipv6.view'
      },
      {
        key: 'pool-analytics',
        label: t('nav.poolAnalytics'),
        icon: TrendCharts,
        path: '/pool/analytics',
        permission: 'pool.analytics.view'
      }
    ]
  },
  {
    key: 'lease',
    label: t('nav.lease'),
    icon: List,
    path: '/lease/active',
    children: [
      {
        key: 'lease-active',
        label: t('nav.leaseActive'),
        icon: List,
        path: '/lease/active',
        permission: 'lease.view'
      },
      {
        key: 'lease-history',
        label: t('nav.leaseHistory'),
        icon: Document,
        path: '/lease/history',
        permission: 'lease.view'
      }
    ]
  },
  {
    key: 'binding',
    label: t('nav.binding'),
    icon: Connection,
    path: '/binding/mac',
    children: [
      {
        key: 'binding-mac',
        label: t('nav.bindingMac'),
        icon: Connection,
        path: '/binding/mac',
        permission: 'binding.view'
      }
    ]
  },
  {
    key: 'option',
    label: t('nav.option'),
    icon: Tickets,
    path: '/option/list',
    children: [
      {
        key: 'option-list',
        label: t('nav.optionList'),
        icon: Collection,
        path: '/option/list',
        permission: 'option.view'
      },
      {
        key: 'option-scopes',
        label: t('nav.optionScopes'),
        icon: Tickets,
        path: '/option/scopes',
        permission: 'option.manage'
      },
      {
        key: 'option-template-config',
        label: t('nav.optionTemplateConfig'),
        icon: Collection,
        path: '/option/templates',
        permission: 'option.view'
      }
    ]
  },
  {
    key: 'monitor',
    label: t('nav.monitor'),
    icon: Guide,
    path: '/monitor/realtime',
    permission: 'monitor.view',
    children: [
      {
        key: 'monitor-overview',
        label: t('nav.monitorOverview'),
        icon: TrendCharts,
        path: '/monitor/overview',
        permission: 'monitor.view'
      },
      {
        key: 'monitor-history',
        label: t('nav.monitorHistory'),
        icon: Grid,
        path: '/monitor/history',
        permission: 'monitor.view'
      },
      {
        key: 'monitor-config',
        label: t('nav.monitorConfig'),
        icon: Bell,
        path: '/monitor/config',
        permission: 'monitor.manage'
      },
      {
        key: 'monitor-analytics',
        label: t('nav.monitorAnalytics'),
        icon: Document,
        path: '/monitor/analytics',
        permission: 'monitor.view'
      }
    ]
  },
  {
    key: 'cluster',
    label: t('nav.cluster'),
    icon: Monitor,
    path: '/cluster/overview',
    permission: ['ha.read', 'ha.manage'],
    children: [
      {
        key: 'cluster-overview',
        label: t('nav.clusterOverview'),
        icon: Connection,
        path: '/cluster/overview',
        permission: ['ha.read', 'ha.manage']
      },
      {
        key: 'cluster-config',
        label: t('nav.clusterConfig'),
        icon: Guide,
        path: '/cluster/config',
        permission: 'ha.manage'
      },
      {
        key: 'cluster-audit-dr',
        label: t('nav.clusterAuditDr'),
        icon: Monitor,
        path: '/cluster/sync-audit-dr',
        permission: 'ha.manage'
      }
    ]
  },
  {
    key: 'security',
    label: t('nav.security'),
    icon: Lock,
    permission: 'security.view',
    path: '/security/access',
    children: [
      {
        key: 'security-access',
        label: t('nav.securityAccess'),
        icon: Guide,
        path: '/security/access',
        permission: 'security.view'
      },
      {
        key: 'security-mac-list',
        label: t('nav.securityMacList'),
        icon: Monitor,
        path: '/security/mac-lists',
        permission: 'security.view'
      },
      {
        key: 'security-network',
        label: t('nav.securityNetwork'),
        icon: Monitor,
        path: '/security/network',
        permission: 'security.view'
      },
      {
        key: 'security-threat',
        label: t('nav.securityThreat'),
        icon: Bell,
        path: '/security/threat',
        permission: 'security.view'
      }
    ]
  },
  {
    key: 'system',
    label: t('nav.system'),
    icon: Setting,
    path: '/system',
    permission: ADMIN_ROLE_PERMISSION,
    children: [
      {
        key: 'system-general',
        label: t('nav.systemGeneral'),
        icon: Setting,
        path: '/system/general',
        permission: ADMIN_ROLE_PERMISSION
      },
      {
        key: 'system-roles',
        label: t('nav.systemRoles'),
        icon: UserFilled,
        path: '/system/roles',
        permission: ['rbac.role.read', 'auth.user.read', 'rbac.assignment.read']
      },
      {
        key: 'system-apikeys',
        label: t('system.apikey.title'),
        icon: Key,
        path: '/system/apikeys',
        permission: ADMIN_ROLE_PERMISSION
      },
      {
        key: 'system-sessions',
        label: t('nav.systemSessions'),
        icon: Monitor,
        path: '/system/sessions',
        permission: ADMIN_ROLE_PERMISSION
      },
      {
        key: 'system-migration-tools',
        label: t('nav.systemMigrationTools'),
        icon: Histogram,
        path: '/system/migration-tools',
        permission: ADMIN_ROLE_PERMISSION
      },
      {
        key: 'system-smtp',
        label: t('nav.systemSmtp'),
        icon: Message,
        path: '/system/smtp',
        permission: ADMIN_ROLE_PERMISSION
      },
      {
        key: 'system-login-audit',
        label: t('nav.loginAudit'),
        icon: Histogram,
        path: '/system/login-audit',
        permission: 'audit.read'
      }
    ]
  }
]);

const filterMenu = (items: MenuItem[]): MenuItem[] =>
  items
    .map((m) => {
      if (m.children && m.children.length) {
        const kids = filterMenu(m.children);
        return { ...m, children: kids };
      }
      return m;
    })
    .filter((m) => (m.children ? m.children.length > 0 : hasPermission(m.permission)));

const filteredMenus = computed(() => filterMenu(menus.value));

const activeMenu = ref(route.path);
const tabs = ref<{ title: string; path: string; closable: boolean }[]>([
  { title: t('nav.dashboard'), path: '/dashboard', closable: false }
]);
const activeTab = ref(tabs.value[0].path);
const cacheViews = ref<string[]>(['/dashboard']);

const breadcrumbs = computed(() => {
  return route.matched.map((r) => ({
    label: (r.meta?.title as string) || (r.name as string) || r.path,
    path: r.path
  }));
});

watch(
  () => route.path,
  (path) => {
    activeMenu.value = path;
    addTab(path, (route.meta?.title as string) || (route.name as string) || t('nav.defaultPage'));
  },
  { immediate: true }
);

function addTab(path: string, title: string) {
  if (!tabs.value.find((t) => t.path === path)) {
    tabs.value.push({ title, path, closable: true });
    cacheViews.value.push(path);
  }
  activeTab.value = path;
}

function removeTab(target: string) {
  const idx = tabs.value.findIndex((t) => t.path === target);
  if (idx === -1) return;
  const isActive = activeTab.value === target;
  tabs.value.splice(idx, 1);
  cacheViews.value = cacheViews.value.filter((p) => p !== target);
  if (isActive && tabs.value.length) {
    const next = tabs.value[Math.max(idx - 1, 0)].path;
    activeTab.value = next;
    router.push(next);
  }
}

function handleMenuClick(item: MenuItem) {
  if (!item.path) return;
  if (item.path === route.path) return;
  router.push(item.path);
}

function handleBreadcrumb(path: string) {
  router.push(path);
}

function toggleSidebar() {
  isCollapse.value = !isCollapse.value;
}

function goHome() {
  router.push('/dashboard');
}

function goSettings() {
  router.push('/account/settings');
}

async function logout() {
  try {
    await authStore.logout();
    permissionStore.reset();
    tabs.value = [{ title: t('nav.dashboard'), path: '/dashboard', closable: false }];
    cacheViews.value = ['/dashboard'];
    activeTab.value = '/dashboard';
    activeMenu.value = '/login';
    await router.replace('/login');
    showSuccess(t('layout.logoutSuccess'));
  } catch (err) {
    showError(t('nav.logoutFail'));
  }
}
</script>

<style scoped>
.app-layout {
  --el-color-primary: #165dff;
  --el-color-primary-light-3: #4080ff;
  --el-color-primary-light-5: #69a1ff;
  --el-color-primary-light-7: #91c3ff;
  --el-color-primary-light-8: #abcfff;
  --el-color-primary-light-9: #e8f3ff;
  --el-color-primary-dark-2: #0e42d2;
  --el-color-success: #00b42a;
  --el-color-warning: #ff7d00;
  --el-color-danger: #f53f3f;

  width: 100vw;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
  color: var(--el-text-color-primary);
}

.app-header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-bottom: 1px solid var(--el-border-color-light);
  background: #ffffff;
  position: sticky;
  top: 0;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.brand-logo {
  width: 28px;
  height: 28px;
}

.brand-name {
  font-weight: 600;
  font-size: 16px;
}

.breadcrumb {
  margin-left: 8px;
}

.breadcrumb-text {
  cursor: pointer;
  color: #4e5969;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-group {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding-left: 8px;
}

.action-group + .action-group {
  border-left: 1px solid var(--el-border-color-lighter);
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.icon-btn :deep(svg) {
  width: 18px;
  height: 18px;
  color: #4e5969;
}

.user-info {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.username {
  font-size: 14px;
  color: #1d2129;
}

.app-body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.sider {
  width: 240px;
  transition: width 0.2s ease;
  background: #001529;
  color: #ffffff;
  border-right: 1px solid rgba(255, 255, 255, 0.12);
}

.sider.collapsed {
  width: 64px;
}

.sider-scroll {
  height: calc(100vh - 56px);
}

.sider-scroll :deep(.el-scrollbar__wrap) {
  scrollbar-width: none; /* Firefox */
}

.sider-scroll :deep(.el-scrollbar__wrap::-webkit-scrollbar) {
  width: 6px;
}

.sider-scroll :deep(.el-scrollbar__wrap::-webkit-scrollbar-track) {
  background: transparent;
}

.sider-scroll :deep(.el-scrollbar__wrap::-webkit-scrollbar-thumb) {
  background: rgba(255, 255, 255, 0.16);
  border-radius: 8px;
}

.sider-scroll :deep(.el-scrollbar__bar) {
  width: 0;
}

.menu-icon {
  margin-right: 6px;
  width: 18px;
  height: 18px;
  font-size: 18px;
  flex-shrink: 0;
  color: inherit;
}

:deep(.el-menu) {
  --el-menu-icon-width: 22px;
  font-size: 14px;
  background-color: #001529;
  border-right: none;
}

:deep(.el-menu-item),
:deep(.el-sub-menu__title) {
  height: 44px;
  line-height: 44px;
  color: rgba(255, 255, 255, 0.85);
  font-size: 14px;
}

:deep(.el-menu-item.is-active) {
  background-color: #165dff;
  color: #ffffff;
}

:deep(.el-menu-item:hover),
:deep(.el-sub-menu__title:hover) {
  background-color: rgba(22, 93, 255, 0.18) !important;
  color: #ffffff !important;
}

:deep(.el-sub-menu .el-menu-item) {
  padding-left: 52px !important;
  font-size: 13px;
}

:deep(.el-sub-menu__icon-arrow) {
  right: 16px;
  color: rgba(255, 255, 255, 0.7);
}

:deep(.el-menu--popup) {
  background: #001529 !important;
  border-color: rgba(255, 255, 255, 0.16);
}

:deep(.el-menu--popup .el-menu-item),
:deep(.el-menu--popup .el-sub-menu__title) {
  color: rgba(255, 255, 255, 0.85) !important;
}

:deep(.el-menu--popup .el-menu-item.is-active),
:deep(.el-menu--popup .el-menu-item:hover),
:deep(.el-menu--popup .el-sub-menu__title:hover) {
  background: rgba(22, 93, 255, 0.18) !important;
  color: #ffffff !important;
}

.content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: #f5f7fa;
}

.view-wrapper {
  flex: 1;
  padding: 16px;
  overflow: auto;
  background: #f5f7fa;
}

.footer {
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
  border-top: 1px solid var(--el-border-color-light);
}

@media (max-width: 1024px) {
  .sider {
    position: fixed;
    left: 0;
    top: 56px;
    height: calc(100vh - 56px);
    z-index: 9;
  }

  .content {
    margin-left: 0;
  }
}
</style>
