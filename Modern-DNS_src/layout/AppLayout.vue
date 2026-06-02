<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAppStore } from '../stores/app'
import { useAlertStore } from '../stores/alert'
import BrandLogo from '../components/BrandLogo.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const appStore = useAppStore()
const alertStore = useAlertStore()

const breadcrumbs = computed(() => route.matched.filter((item) => item.meta?.titleKey))

const activeRootMenu = computed(() => {
  const currentPath = typeof route.meta.menuPath === 'string' ? route.meta.menuPath : route.path

  return (
    appStore.menuTree.find(
      (group) => currentPath === group.path || currentPath.startsWith(`${group.path}/`),
    )?.path || ''
  )
})

const openedMenus = computed(() =>
  appStore.collapsed || !activeRootMenu.value ? [] : [activeRootMenu.value],
)

const asideWidth = computed(() =>
  appStore.collapsed ? 'var(--app-aside-collapse-width)' : 'var(--app-aside-width)',
)

const bellPopoverVisible = ref(false)
const unreadCount = computed(() => alertStore.unreadCount)

const bellMarkAllRead = () => { alertStore.markAllRead() }
const bellMarkRead = (id: number) => { alertStore.markRead(id) }
const goAlerts = () => {
  bellPopoverVisible.value = false
  router.push('/monitor/alert-notice')
}

// Fetch alerts on layout mount
alertStore.fetchAlerts()

const handleCommand = (command) => {
  if (command === 'logout') {
    appStore.logout()
    ElMessage.success(t('layout.loggedOut'))
    router.replace('/login')
    return
  }
  if (command === 'profile') {
    router.push('/profile')
  }
}

</script>

<template>
  <el-container class="app-layout">
    <el-aside :width="asideWidth" class="layout-aside">
      <div class="brand-panel" :class="{ collapsed: appStore.collapsed }">
        <BrandLogo :size="36" class="brand-badge" />
        <div v-if="!appStore.collapsed" class="brand-copy">
          <strong>Modern DNS</strong>
          <span>{{ t('layout.brandSubtitle') }}</span>
        </div>
      </div>
      <div class="aside-menu-wrap">
        <el-menu
          :key="`${activeRootMenu}-${appStore.collapsed ? 'collapsed' : 'expanded'}`"
          :default-active="appStore.activeMenu || route.path"
          :default-openeds="openedMenus"
          :collapse="appStore.collapsed"
          :collapse-transition="false"
          :unique-opened="true"
          mode="vertical"
          class="aside-menu"
          router
        >
          <el-sub-menu
            v-for="group in appStore.menuTree"
            :key="group.path"
            :index="group.path"
            :show-timeout="300"
            :hide-timeout="300"
          >
            <template #title>
              <el-icon><component :is="group.icon" /></el-icon>
              <span>{{ $t('menu.' + group.titleKey) }}</span>
            </template>
            <el-menu-item
              v-for="item in group.children"
              :key="item.path"
              :index="item.path"
            >
              {{ $t('menu.' + item.titleKey) }}
            </el-menu-item>
          </el-sub-menu>
        </el-menu>
      </div>
    </el-aside>

    <el-container>
      <el-header class="layout-header">
        <div class="header-left">
          <el-button circle plain @click="appStore.toggleCollapse()">
            <el-icon><Fold v-if="!appStore.collapsed" /><Expand v-else /></el-icon>
          </el-button>
          <div>
            <el-breadcrumb separator="/">
              <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
                {{ $t('menu.' + item.meta.titleKey) }}
              </el-breadcrumb-item>
            </el-breadcrumb>
            <div class="header-route-title">{{ route.meta.titleKey ? $t('menu.' + route.meta.titleKey) : '' }}</div>
          </div>
        </div>

        <div class="header-right">
          <!-- Bell icon -->
          <el-popover
            v-model:visible="bellPopoverVisible"
            placement="bottom-end"
            :width="340"
            trigger="click"
            popper-class="alert-popover"
          >
            <template #reference>
              <div class="bell-btn" :class="{ 'has-unread': unreadCount > 0 }">
                <el-badge :value="unreadCount" :hidden="unreadCount === 0" :max="99">
                  <svg viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="22" height="22"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
                </el-badge>
              </div>
            </template>
            <div class="alert-pop">
              <div class="alert-pop-header">
                <span class="alert-pop-title">{{ $t('monitor.alertNotice') }}</span>
                <el-button v-if="unreadCount > 0" link size="small" @click="bellMarkAllRead">{{ $t('common.selectAll') }}</el-button>
              </div>
              <div class="alert-pop-list">
                <template v-if="alertStore.notices.length">
                  <div
                    v-for="item in alertStore.notices.slice(0, 8)"
                    :key="item.id"
                    class="alert-pop-item"
                    :class="{ 'is-read': item.read }"
                    @click="bellMarkRead(item.id)"
                  >
                    <span class="alert-dot" :class="'alert-dot--' + item.level"></span>
                    <div class="alert-pop-body">
                      <span class="alert-pop-msg">{{ item.content }}</span>
                      <span class="alert-pop-time">{{ item.triggeredAt }}</span>
                    </div>
                  </div>
                </template>
                <div v-else class="alert-pop-empty">{{ $t('common.noData') }}</div>
              </div>
              <div class="alert-pop-footer">
                <el-button link size="small" @click="goAlerts">{{ $t('monitor.alertNotice') }} →</el-button>
              </div>
            </div>
          </el-popover>

          <el-dropdown @command="handleCommand">
            <div class="user-panel">
              <el-avatar size="small">{{ appStore.user.name.slice(0, 1) }}</el-avatar>
              <div class="user-copy">
                <strong>{{ appStore.user.name }}</strong>
                <span>{{ appStore.user.role }}</span>
              </div>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile">{{ t('layout.profile') }}</el-dropdown-item>
                <el-dropdown-item command="logout">{{ t('layout.logout') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="layout-main">
        <div v-show="appStore.loading" class="route-loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>{{ t('layout.pageLoading') }}</span>
        </div>
        <router-view v-slot="{ Component }">
          <keep-alive :max="12">
            <component :is="Component" :key="route.name" />
          </keep-alive>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
  background: transparent;
}

.layout-aside {
  position: sticky;
  top: 0;
  height: 100vh;
  overflow: hidden;
  border-right: 1px solid var(--app-border);
  background: var(--app-bg-secondary);
  box-shadow: 8px 0 24px var(--app-border);
}

.brand-panel {
  display: flex;
  align-items: center;
  gap: 12px;
  height: var(--app-header-height);
  padding: 0 18px;
  border-bottom: 1px solid var(--app-border);
}

.brand-panel.collapsed {
  justify-content: center;
}

.brand-badge {
  /* The component already paints its own dark rounded backdrop and
     accent dot — we only contribute layout sizing here so the badge
     keeps the same 36px footprint as the legacy text version. */
  width: 36px;
  height: 36px;
  border-radius: 12px;
  flex-shrink: 0;
}

.brand-copy {
  display: flex;
  flex-direction: column;
  color: var(--app-accent);
}

.brand-copy span {
  font-size: 12px;
  color: var(--app-text-regular);
}

.aside-menu {
  padding: 12px 10px 24px;
  background: transparent;
  border-right: none;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: var(--app-text-regular);
  --el-menu-hover-bg-color: var(--app-bg);
  --el-menu-active-color: var(--app-accent);
}

.aside-menu-wrap {
  height: calc(100vh - var(--app-header-height));
  overflow: hidden;
}

.layout-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  height: var(--app-header-height);
  padding: 0 24px;
  border-bottom: 1px solid var(--el-color-primary-dark-2);
  background: var(--app-accent);
  color: var(--el-color-white);
}

.header-left,
.header-right,
.user-panel {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-route-title {
  margin-top: 6px;
  font-size: 16px;
  font-weight: 700;
  color: var(--el-color-white);
}

.user-panel {
  padding: 8px 12px;
  border: 1px solid var(--el-color-primary-dark-2);
  border-radius: 14px;
  background: var(--el-color-primary-dark-2);
  color: var(--el-color-white);
  cursor: pointer;
}

.user-panel:hover {
  background: var(--app-accent-soft);
  border-color: var(--app-accent-soft);
  color: var(--app-accent);
}

.user-copy {
  display: flex;
  flex-direction: column;
}

.user-copy span {
  font-size: 12px;
  color: var(--app-accent-muted);
}

:deep(.el-breadcrumb__item .el-breadcrumb__inner) {
  color: var(--app-accent-muted);
}

:deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--el-color-white);
}

.layout-header :deep(.el-breadcrumb__inner),
.layout-header :deep(.el-breadcrumb__separator),
.layout-header :deep(.el-icon),
.layout-header :deep(.el-button),
.layout-header :deep(.el-switch__label) {
  color: var(--el-color-white);
}

.layout-header :deep(.el-button.is-plain) {
  border-color: var(--el-color-primary-dark-2);
  background: var(--el-color-primary-dark-2);
}

.layout-header :deep(.el-button.is-plain:hover) {
  border-color: var(--app-accent-soft);
  background: var(--app-accent-soft);
  color: var(--app-accent);
}

.layout-main {
  position: relative;
  padding: 24px;
}

.route-loading {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: var(--app-bg);
  font-size: 16px;
  color: var(--app-text-regular);
}

/* Bell */
.bell-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  border: none;
  background: var(--app-accent, #165dff);
  cursor: pointer;
  transition: background 0.2s;
}

.bell-btn:hover {
  background: var(--el-color-primary-dark-2, #1040cc);
}

.bell-btn :deep(.el-badge__content) {
  font-size: 10px;
  height: 16px;
  line-height: 16px;
  padding: 0 4px;
  min-width: 16px;
  transform: translateY(-4px) translateX(4px);
}

/* Alert popover content */
.alert-pop {
  padding: 0;
}

.alert-pop-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px 8px;
  border-bottom: 1px solid #f0f0f0;
}

.alert-pop-title {
  font-size: 14px;
  font-weight: 700;
  color: #1d2129;
}

.alert-pop-list {
  max-height: 280px;
  overflow-y: auto;
}

.alert-pop-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  border-bottom: 1px solid #f5f5f5;
  transition: background 0.15s;
}

.alert-pop-item:hover {
  background: #f8faff;
}

.alert-pop-item.is-read {
  opacity: 0.5;
}

.alert-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-top: 5px;
}

.alert-dot--critical { background: #ef4444; }
.alert-dot--warning  { background: #f59e0b; }
.alert-dot--info     { background: #3b82f6; }

.alert-pop-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.alert-pop-msg {
  font-size: 13px;
  color: #1d2129;
  line-height: 1.4;
  word-break: break-all;
}

.alert-pop-time {
  font-size: 11px;
  color: #94a3b8;
}

.alert-pop-empty {
  padding: 24px 14px;
  text-align: center;
  font-size: 13px;
  color: #94a3b8;
}

.alert-pop-footer {
  padding: 8px 14px;
  text-align: center;
  border-top: 1px solid #f0f0f0;
}

@media (max-width: 900px) {
  .layout-header,
  .layout-main {
    padding: 16px;
  }

  .user-copy {
    display: none;
  }
}
</style>
