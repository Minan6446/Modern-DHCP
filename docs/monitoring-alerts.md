# 监控与告警模块文档

更新日期：2026-02-10（当前主干）

涵盖前端监控/告警页面组件及相关 API 调用。组件路径位于 `web/src/views/monitoring`，API 位于 `web/src/api/monitoring.ts`。

## API 概览 — [web/src/api/monitoring.ts](web/src/api/monitoring.ts)
- 实时/指标：`getRealtimeSnapshot()`、`getMetricSeries(metric, params)`、`getLatencyDistribution()`。
- 池/租约状态：`listPoolUsage()`、`getLeaseStatus()`。
- 异常与合成监测：`listAnomalies()`、`listSyntheticChecks()`。
- 告警：`listAlertRules()`、`saveAlertRule(payload)`、`deleteAlertRule(id)`、`listAlertEvents(params?)`。
- 通知渠道：`listChannels()`、`saveChannel(payload)`。
- 报表：`createReportTask(payload)`、`listReportTasks()`。

## 页面/组件

### 监控布局 — MonitoringLayout
路径：[web/src/views/monitoring/MonitoringLayout.vue](web/src/views/monitoring/MonitoringLayout.vue)
- 通过 `el-tabs` 切换四个子面板：实时监控、业务监控、告警中心、报表分析。

### 实时监控 — RealtimePanel
路径：[web/src/views/dashboard/RealtimePanel.vue](web/src/views/dashboard/RealtimePanel.vue)
- 数据源：
  - WebSocket：`wss://<域名>/api/v1/dashboard/streams/live?limit=50`，消息类型 `stream.snapshot` / `stream.delta`。
  - 备选：如 WS 不可用，可定时调用 `/api/v1/dashboard/streams` 获取快照。
- 展示：事件/告警/操作日志列表（各 50 条截断）、状态流转图（ECharts graph）。
- 关键逻辑：`useWs` 心跳+重连；snapshot 重置列表，delta 追加增量；需配置 `VITE_WS_BASE` 并在反代透传 `Upgrade/Connection`。

### 业务监控 — BusinessMonitor
路径：[web/src/views/monitoring/BusinessMonitor.vue](web/src/views/monitoring/BusinessMonitor.vue)
- 数据源：`listPoolUsage()`（池利用率）、`getLeaseStatus()`（租约状态）、`listAnomalies()`（异常记录）、`listSyntheticChecks()`（合成交易健康）。
- 展示：
  - 地址池利用率柱状图。
  - 租约状态概览（Active/Expired/Pending/Failed）。
  - 异常请求表格、合成检查表格（含状态标签）。

### 告警中心 — AlertCenter
路径：[web/src/views/monitoring/AlertCenter.vue](web/src/views/monitoring/AlertCenter.vue)
- 数据源：`listAlertRules()`、`listAlertEvents()`、`listChannels()`；规则保存/删除使用 `saveAlertRule`、`deleteAlertRule`。
- 功能：
  - 规则列表：指标、阈值、级别、启用状态，支持编辑/删除。
  - 规则编辑对话框：指标、阈值、比较符、持续时长、级别、日志匹配、启用开关。
  - 当前告警表格、通知渠道列表（邮箱/Webhook 等）。

### 报表中心 — ReportCenter
路径：[web/src/views/monitoring/ReportCenter.vue](web/src/views/monitoring/ReportCenter.vue)
- 数据源：`createReportTask(form)` 生成报表，`listReportTasks()` 拉取任务。
- 功能：
  - 表单：报表类型、时间范围、格式（PDF/CSV）。
  - 任务表：展示状态、创建时间、下载链接。

## 集成建议
- WebSocket 与轮询：实时面板默认用 `/dashboard/streams/live` 增量；不可用时退化为定时快照。
- 数据回放与占位：组件已在异常捕获中提供示例数据占位，便于开发时无后端的静态展示。
- 告警规则与权限：可结合 `usePermissionStore().can('alert:write')` 控制“新增/删除”按钮显示，配合 `v-permission` 使用。
- 报表下载：若后端返回下载 URL，可与 `exportBlob` 组合；当前组件直接使用 `downloadUrl` 渲染外链。
- 性能曲线：前端只做轻量绘图，重采样/聚合建议在后端完成后再返回。
