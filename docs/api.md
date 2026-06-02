# API 概览（后端）

更新日期：2026-02-10（当前主干）

本文件为后端 API 的快速导览，包含主要 REST 路径、WebSocket 流、返回格式与错误规范。详细 schema 参考 `docs/api/openapi_v2.yaml`。

## 基本约定
- Base Path：`/api/v1`（REST 与 WebSocket 共用此前缀）。
- 认证：支持 Bearer Token；前端默认使用 Session Cookie（Secure + HttpOnly）。所有请求会做 RBAC 校验。
- 租户：单租户模式不再需要显式租户；多租户可通过 Header `X-Tenant-Id` 或部分接口的查询参数传递。
- 分页：常见查询支持 `page`, `pageSize`，返回 `total` 与 `items`。
- 错误：统一返回 `{ "code": string, "message": string, "details"?: any }`。

## WebSocket
- `GET /dashboard/streams/live`：实时事件/审计流；支持 `limit`、`types`（alert|ops），消息类型含 `stream.snapshot` 与 `stream.delta`。
- `GET /monitoring/status/stream`：健康与概览状态流，支持 `tenantId`。
- 握手与权限：与 REST 相同（Cookie/Bearer）；反向代理需启用 `proxy_http_version 1.1` 并透传 `Upgrade/Connection` 头。

## 常用资源
- 地址池
  - `GET /pools`：分页列出池，可按 `scope`, `vlan`, `location` 过滤。
  - `POST /pools/resolve`：根据元数据解析最优池，返回选择理由与审计 ID。
- 租约
  - `GET /leases`：按 `state`, `mac`, `ip`, `pool` 检索租约。
  - `POST /leases/release`：释放指定租约（需 RBAC）。
- 集群与监控
  - `GET /cluster/overview`：返回角色、复制延迟、抑制状态、节点健康。
  - `GET /monitoring/overview`：返回池利用率、请求阶段统计、安全事件摘要。
- 自动化
  - `POST /jobs`：创建自动化任务（如池扩容、批量释放）。
  - `GET /jobs/{id}`：查询任务状态、审批与执行日志。
- 通知与告警
  - `GET /notifications`：列出通知配置。
  - `POST /alerts/silence`：创建静默窗口（需审批/权限）。

## 响应示例
- 解析地址池
```json
POST /api/v1/pools/resolve
{
  "vlan": 100,
  "interface": "eth0",
  "location": "hz-01"
}
```
响应：
```json
{
  "poolId": "pool-subnet-1",
  "cidr": "10.0.0.0/24",
  "gateway": "10.0.0.1",
  "weight": 80,
  "auditId": "aud-2025-12-31-001"
}
```

## 版本与兼容性
- 当前文档对应 v1 路由；未来新增字段将保持向后兼容（使用可选字段）。
- 破坏性变更将通过新版本前缀发布（如 `/api/v2`）。

## 开发者提示
- 对接前请确认 RBAC 权限；敏感操作建议走审批。
- 使用 `/cluster/overview` 和 `/monitoring/overview` 进行健康检查与降级判断。
- 建议在客户端添加重试与幂等键，特别是自动化与租约相关操作。
