# 观测仪表盘计划

更新日期：2026-02-10（当前主干）

本计划描述建议的 Grafana / 前端仪表盘布局，支持运维与安全视角的快速洞察。

## 概览（Ops Home）
- 请求量与成功率（按协议族）。
- P95/P99 延迟（按入口、节点）。
- 活跃租约计数与变化趋势。
- 池利用率 Top N / 底部 N。
 - 实时事件/审计流：`/api/v1/dashboard/streams/live` 连接状态与最近增量。

## DHCP Pipeline 页
- 各阶段请求/失败率：discover/offer/request/ack/renew/rebind。
- 拒绝原因分布：策略拒绝、池耗尽、校验失败。
- 速率限制命中趋势。

## Pool Capacity 页
- 按层级（global/subnet/vlan/port/ssid/location）容量占用与增长速率。
- 预测剩余时间（基于线性回归或简单移动平均）。
- 池切换事件与配置变更时间线叠加。

## Cluster/HA 页
- 节点角色、复制延迟、切换次数。
- 连接池使用率、错误率。
- 抑制状态、维护窗口标记。

## Security 页
- Rate Limit/Snooping 事件速率与分布（按 interface/vlan）。
- Rogue/异常 DHCP 请求计数（如实现）。
- 告警升级与静默状态。

## 数据库页
- QPS、慢查询计数、耗时分布。
- 连接池饱和度、等待时间。
- 复制延迟与主从延迟。

## 自动化与任务页
- 任务成功率、重试次数、平均执行时长。
- 审批等待时间与拒绝比例。
- 热门任务类型与触发来源。

## 落地建议
- 数据源：Prometheus；必要时接入 Loki/ELK 进行日志查询。
- 模板：在 `docs/dashboards/` 中存放 JSON 模板（可后续补充）。
- 权限：仪表盘按角色控制访问。
 - WebSocket：前端实时卡片需反代透传 `Upgrade/Connection`，基址示例 `wss://<域名>/api/v1`。
