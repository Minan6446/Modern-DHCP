# 前端 API 层文档

概述前端 API 封装（`web/src/api`）的客户端拦截、各业务模块方法与常见集成方式。函数返回值默认 `Promise<AxiosResponse<ApiResponse<T>>>`，除导出接口外。

## HTTP 客户端
- 位置：[web/src/api/http/client.ts](web/src/api/http/client.ts)
- 特性
  - 自动附加 `Authorization: Bearer <token>`（来自 `useAuthStore`）。
  - 401 且存在 `refreshToken` 时尝试 `auth.refresh()` 后重放原请求；失败时调用 `auth.logout()`。
  - 默认 `timeout=15000ms`，`baseURL` 取 `VITE_API_BASE_URL`。

## 业务模块

### 认证与密钥
- [web/src/api/auth.ts](web/src/api/auth.ts)：`login(payload)`、`logout()`、`refreshToken(token)`，返回 token 对与用户信息。
- [web/src/api/apikeys.ts](web/src/api/apikeys.ts)：`listApiKeys(params)`、`createApiKey(payload)`、`revokeApiKey(id)`。
- 系统版前缀 `/api/v1`：
  - [web/src/api/system/auth.ts](web/src/api/system/auth.ts)：`login`、`logout`、`refreshToken`、`listSessions`。

### 会话
- [web/src/api/sessions.ts](web/src/api/sessions.ts)：`listSessions(params)`、`getSession(id)`、`forceLogout(id)`。

### 用户与角色
- [web/src/api/system/user.ts](web/src/api/system/user.ts)：`listUsers(params)`、`createUser`、`updateUser`、`updateUserStatus`、`getUserPermissions`。
- [web/src/api/roles.ts](web/src/api/roles.ts)：`listRoles`、`getRole`、`createRole`、`updateRole`、`deleteRole`、`getPermissionTree`、`assignRolePermissions(id, keys)`、`assignUserRoles(userId, roleIds)`。
- [web/src/api/system/role.ts](web/src/api/system/role.ts)：`listRoles`、`createRole`、`updateRole`、`deleteRole`（`/api/v1/roles` 前缀）。

### 系统配置
- [web/src/api/system/config.ts](web/src/api/system/config.ts)：`getGlobalConfig`、`updateGlobalConfig`、`getNotificationConfig`、`updateNotificationConfig`。
- [web/src/api/system/index.ts](web/src/api/system/index.ts)：系统 API 聚合导出。

### 地址池 / 前缀
- [web/src/api/pools.ts](web/src/api/pools.ts)：
  - 列表与 CRUD：`listPools(params)`、`createPool(payload)`、`updatePool(id,payload)`、`deletePool(id)`。
  - 监控：`getUsage(id)`、`getHistory(id, params)`、`getConflicts(id)`。
  - 委派：`listPrefixDelegations(params)`。

### 绑定
- [web/src/api/bindings.ts](web/src/api/bindings.ts)：`listBindings(params)`、`createBinding`、`updateBinding`、`deleteBinding`、`batchImportBindings(items)`、`batchDeleteBindings(filter)`、`getConflicts(params)`、`getBinding(id)`。

### 租约
- [web/src/api/leases.ts](web/src/api/leases.ts)：
  - 查询：`listActiveLeases(params)`、`listHistoryLeases(params)`、`getLeaseEvents(leaseId)`。
  - 操作：`releaseLease(leaseId)`、`renewLease(leaseId)`。
  - 导出：`exportLeases(params)`（`responseType: 'blob'`）。

### DHCP 选项
- [web/src/api/options.ts](web/src/api/options.ts)：
  - 标准/自定义：`listStandardOptions`、`listCustomOptions`、`createCustomOption`、`updateCustomOption`、`deleteCustomOption`。
  - 使用洞察：`getOptionUsage()`。
  - 模板：`listOptionTemplates`、`createTemplate`、`updateTemplate`、`deleteTemplate`、`getTemplateGraph()`。
  - 测试：`testOptionDelivery(payload)`。

### 监控与告警
- [web/src/api/monitoring.ts](web/src/api/monitoring.ts)：
  - 监控：`getRealtimeSnapshot()`、`getMetricSeries(metric, params)`、`getLatencyDistribution()`。
  - 池/租约概览：`listPoolUsage()`、`getLeaseStatus()`。
  - 异常/合成监测：`listAnomalies()`、`listSyntheticChecks()`。
  - 告警：`listAlertRules()`、`saveAlertRule(payload)`、`deleteAlertRule(id)`、`listAlertEvents(params?)`。
  - 通道：`listChannels()`、`saveChannel(payload)`。
  - 报表：`createReportTask(payload)`、`listReportTasks()`。

### 安全
- [web/src/api/security.ts](web/src/api/security.ts)：
  - Snooping：`listTrustPorts()`、`saveTrustPorts(ports)`、`listSnoopingBindings(params)`、`saveViolationPolicy(policy)`。
  - 速率限制：`listRateLimits(params)`、`saveRateLimit(payload)`、`deleteRateLimit(id)`。
  - Rogue 服务器：`listRogueServers()`、`quarantineRogue(id)`。
  - DAI/SourceGuard：`getDAIConfig()`、`updateDAIConfig(payload)`、`getSourceGuard()`、`updateSourceGuard(payload)`。
  - 端口/认证：`listPortProfiles()`、`savePortProfile(payload)`、`listDot1xProfiles()`、`saveDot1xProfile(payload)`、`listMacAuthProfiles()`、`saveMacAuthProfile(payload)`。
  - Radius 映射：`listRadiusMappings()`、`saveRadiusMapping(payload)`、`deleteRadiusMapping(id)`。
  - 威胁/拓扑：`listThreatEvents()`、`getTopology()`。

### API Key
- [web/src/api/apikeys.ts](web/src/api/apikeys.ts)：见“认证与密钥”部分。

## 常见集成示例
- 统一下载：对 `exportLeases` 等返回 `Blob` 的接口可用 `exportBlob`/`downloadCsv` 触发浏览器下载。
- 权限控制：`listRoles`/`getPermissionTree` 结果可写入 `usePermissionStore`，配合 `v-permission` 指令控制按钮显示。
- 监控/实时：`getRealtimeSnapshot` 与 `getMetricSeries` 可结合 WebSocket 推送与 `rollingAppend` 维护前端折线数据。

## 注意
- 存在两套路径：根路径（如 `/pools`、`/bindings`）与 `/api/v1/*` 系统路径，使用时注意选用一致的后端版本。
- 所有请求均经过拦截器自动注入 token 与 401 刷新逻辑，调用层无需重复处理。
