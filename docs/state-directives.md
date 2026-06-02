# 状态管理与指令模块文档

涵盖 Pinia 状态库（`web/src/store/*`）与权限指令（`web/src/directives/permission.ts`），用于 DHCP 控制台的会话、权限、租约、地址池、监控等场景。

## Pinia Stores

### Auth — [web/src/store/auth.ts](web/src/store/auth.ts)
- State: `accessToken`, `refreshToken`, `user`, `permissions`, `sessions`, `loading`, `loggedIn`（持久化到 localStorage）。
- Getters: `isAuthenticated`, `displayName`。
- Actions:
  - `login(payload: LoginRequest)`: 调用登录 API，保存 token。
  - `setUser(user, permissions)`: 设置用户与权限集。
  - `refresh()`: 使用刷新令牌获取新 access token。
  - `fetchSessions()`: 获取会话列表。
  - `logout()`: 退出并清空本地状态。
  - `reset()`: 重置并持久化。
- 用法：登录后调用 `setUser` 载入用户资料与权限。

### Permission — [web/src/store/permission.ts](web/src/store/permission.ts)
- State: `permissions:Set<string>`, `tree`, `loading`。
- Actions:
  - `loadPermissions()`: 获取权限树并展开为 Set。
  - `can(key: string)`: 判断权限。
- 典型配合：权限指令 `v-permission`。

### User — [web/src/store/user.ts](web/src/store/user.ts)
- State: `users`, `total`, `filters`, `bulkSelection`, `loading`（filters 持久化 sessionStorage）。
- Getters: `selectedCount`。
- Actions: `fetch(params?)`（支持分页/状态过滤）、`setSelection(ids)`, `resetFilters()`, `reset()`。

### Session — [web/src/store/session.ts](web/src/store/session.ts)
- State: `sessions`, `total`, `loading`。
- Actions: `fetch(params)`, `kick(id)`（踢出会话并本地移除）。

### Monitor — [web/src/store/monitor.ts](web/src/store/monitor.ts)
- State: `snapshot`, `alerts`, `rules`, `reportTasks`, `config{autoRefresh,intervalMs}`, `loading`（config 持久化 localStorage）。
- Actions: `refreshSnapshot()`, `loadAlerts()`, `loadRules()`, `loadReports()`, `setConfig(config)`, `reset()`。
- 依赖：`normalizeSnapshot` 填充缺省指标。

### Pool — [web/src/store/pool.ts](web/src/store/pool.ts)
- State: `pools`, `currentPoolId`, `usage`, `history`, `loading`（部分持久化 sessionStorage）。
- Getters: `currentPool`。
- Actions: `fetch()`, `select(poolId)`, `setUsage(poolId, utilization)`, `addHistory(action)`, `reset()`。

### Lease — [web/src/store/lease.ts](web/src/store/lease.ts)
- State: `active`, `history`, `filters`, `stats`, `loading`（filters 持久化 sessionStorage）。
- Actions: `fetchActive(params?)`, `fetchHistory(params?)`, `setStats(stats)`, `resetFilters()`, `reset()`。

### System — [web/src/store/system.ts](web/src/store/system.ts)
- State: `globalConfig`, `notificationConfig`, `theme`, `locale`, `params`, `loading`（theme/locale/params 持久化 localStorage）。
- Actions: `loadConfigs()`, `setTheme(theme)`, `setLocale(locale)`, `setParam(key,value)`, `reset()`。

### UI — [web/src/store/ui.ts](web/src/store/ui.ts)
- State: `sidebarCollapsed`, `menuOpenKeys`, `tabs`, `activeTab`, `loading`（持久化 localStorage）。
- Actions: `toggleSidebar(val?)`, `setMenuOpen(keys)`, `openTab(tab)`, `closeTab(key)`, `setLoading(val)`, `reset()`。

## 指令

### 权限指令 — [web/src/directives/permission.ts](web/src/directives/permission.ts)
- 作用：根据权限 key 移除无权限的 DOM 元素。
- 绑定值：`string | string[]`，数组时满足任一即可放行。
- 行为：挂载时读取 `usePermissionStore().can(key)`；未授权则从父节点移除元素。
- 使用示例：
```vue
<el-button v-permission="'pool:create'">新建地址池</el-button>
<el-button v-permission="['pool:update','pool:admin']">编辑</el-button>
```

## 组件/页面集成建议
- 在路由守卫或应用入口加载：调用 `useAuthStore().refresh()` 与 `usePermissionStore().loadPermissions()` 以确保指令可用。
- 需要租约/池筛选时，复用各自 store 的 `filters` 并通过组件事件回填（如 `LeaseFilters`）。
- 多标签页与侧边栏状态通过 `useUiStore` 持久化，页面级只需读写 store 而不必维护本地副本。
