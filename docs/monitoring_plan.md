# 观测与告警计划

更新日期：2026-02-10（当前主干）

本计划说明系统的指标、日志、告警与告警路由策略，帮助运维持续观察运行健康。

## 指标暴露
- Prometheus：默认 `/metrics` 端点，覆盖池解析、租约分配、策略命中、HTTP 请求、DB 连接池等指标。
- 聚合快照（REST）：
  - `/api/v1/monitoring/overview`：池利用率、租约状态分布、请求阶段统计、系统健康。
  - `/api/v1/cluster/overview`：角色、复制延迟、抑制状态，供前端与 LB 使用。
- 实时流（WebSocket）：
  - `/api/v1/dashboard/streams/live`：事件/审计增量流；`limit`、`types`（alert|ops）。
  - `/api/v1/monitoring/status/stream`：健康/概览流，支持 `tenantId`。
- 代理要求：反代需开启 `proxy_http_version 1.1`，并透传 `Upgrade`/`Connection` 头以支持 WebSocket。
- 自定义标签：池、VLAN、接口、Location 等元数据作为标签，便于分组。

## 告警原则
- 三级分级：
  - P1：服务不可用、写入失败、复制中断。
  - P2：延迟升高、池容量高水位、安全事件频繁。
  - P3：趋势预警、配置偏差、慢查询。
- 静默与抑制：
  - 维护窗口可配置静默；HA 切换期间可抑制级联告警。
  - Rate Limit 与 Snooping 事件在短时间内去重，避免风暴。

## 指标与阈值示例
- 可用性
  - HTTP 5xx 率（按入口）；阈值 > 1% 触发 P1。
  - `/cluster/overview` 延迟 > 2s 或角色未知触发 P1。
- 池与租约
  - 池可用容量 < 15% 触发 P1；< 30% 触发 P2。
  - 租约失败率 > 5% 触发 P1；> 2% 触发 P2。
- 数据库
  - 连接池耗尽、慢查询数量、复制延迟 > 3s。
- 安全
  - Rate Limit 或 Snooping 事件在 5 分钟内超过全局阈值触发 P1/P2。

## 日志与审计
- 结构化日志：包含请求 ID、角色、接口、阶段、耗时。
- 审计：
  - 记录变更类操作（池、策略、租约操作、审批决策）。
  - 记录 HA 切换、告警抑制、自动化任务决策。

## 告警路由
- 配置来源：`configs/config.yaml` 的 `alerting` 与 `notifications` 部分。
- 渠道：邮件、聊天、Webhook；可按严重级别分别路由。
- 聚合：按池、事件类型聚合，减少重复。
- 升级策略：未恢复的 P1 在 15 分钟内升级到值班/管理组。

## 运维仪表盘（概览）
- Overview：请求量、成功率、P99 延迟、活跃租约、池利用率。
- Cluster：角色、复制延迟、节点健康、切换次数。
- Security：Rate Limit/Snooping 事件趋势、封禁计数。
- DB：QPS、慢查询、连接池使用率、复制延迟。
 - Realtime：`/dashboard/streams/live` 连接状态与最近增量。

## 运维操作手册挂钩
- 告警触发后关联对应 Runbook（如 threat_response.md）。
- 通过前端或 CLI 查看 `/cluster/overview` 与 `/monitoring/overview`，确认是否需要抑制或切换。
