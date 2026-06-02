# 配置参考（后端）

更新日期：2026-02-10（当前主干）

本文件梳理 `configs/config.yaml` 的主要字段与建议取值，便于部署与运维快速对照。优先级：CLI 旗标 > 环境变量 > YAML。

## server
- `listen`: HTTP 监听地址，默认 `0.0.0.0:8080`。
- `cors`: 允许的来源、方法、头。
- `gracefulTimeout`: 优雅退出超时，单位秒。

## dhcpv4 / dhcpv6
- `interfaces`: 监听的网络接口列表。
- `port`: DHCP 端口，v4 默认 67，v6 默认 547。
- `retry`: 重试次数/退避参数。
- `leaseDefaultTTL`: 默认租约时长。
- `options`: 额外 DHCP 选项或自定义供应配置。

## storage
- `primary`: 主库 DSN。
- `readOnly`: 只读 DSN（可选，用于读写分离）。
- `maxOpenConns` / `maxIdleConns`: 连接池参数。
- `connMaxLifetime`: 连接生命周期。

## monitoring
- `enabled`: 是否开启指标导出。
- `listen`: 指标监听地址（如与 API 分离）。
- `sampling`: 日志/事件抽样率。
- `snapshotInterval`: `/monitoring/overview` 快照刷新间隔。

## alerting
- `enabled`: 是否启用告警聚合。
- `thresholds`: 全局阈值（池容量、租约失败率等）。
- `silence`: 静默窗口配置。

## notifications
- `channels`: 邮件、聊天、Webhook 配置。
- `routing`: 按严重级别路由到不同渠道。

## security
- `rateLimit`: 每接口/VLAN 的速率阈值、窗口与动作。
- `snooping`: 是否采集/上报；事件采样率。
- `ipsg`: 源保护相关参数（如启用）。

## automation
- `scheduler`: 并发度、队列深度、限速。
- `approval`: 审批超时、升级策略、审批人列表来源。
- `retry`: 默认重试次数与退避。

## lease
- `historyRetention`: 历史租约保留时长。
- `statsWindow`: 统计窗口大小。

## pool
- `resolution`: 解析策略（权重、轮询、优先级）。
- `fallback`: 解析失败时的降级策略（拒绝/指定池）。

## ha/failover
- `enabled`: 是否启用 HA 模式。
- `role`: 固定角色或自动。
- `healthProbe`: 心跳周期与超时。
- `autoFailback`: 是否自动回切及窗口。

### ha.replication（同步与补偿）
- `ackTimeout`: ACK 门控超时。
- `enforceAck`: 是否在写路径强制 ACK 门控。
- `ackFailurePolicy`: ACK 失败策略，推荐 `strict|degraded`。
- `syncTxnReconcileInterval`: `syncTransactions` 内存快照补偿写入 `cluster_config` 的周期，默认 `30s`。
- `syncTxnReconcileBackoffMax`: 补偿失败后指数退避的最大间隔，默认 `5m`。
- `syncTxnReconcileFailureAlertThreshold`: 补偿连续失败告警阈值，默认 `3`。
- `syncTxnReconcileAlertCooldown`: 补偿阈值告警冷却窗口，冷却期内抑制重复外送，默认 `5m`。
- `syncTxnMaxEntries`: 活跃事务保留上限，默认 `100`。
- `syncTxnRetention`: 活跃事务按创建时间保留期，默认 `168h`（7天）。
- `syncTxnArchiveLimit`: 被淘汰事务的归档上限（`cluster_sync_transactions_archive`），默认 `500`。

## ops
- `drainOnShutdown`: 退出时是否将节点置为 Draining。
- `logLevel`: 日志级别。
- `audit`: 审计开关与存储保留期。

## 前端环境变量（运维常用）
- `VITE_API_BASE_URL`: REST API 前缀，默认 `/api/v1`。
- `VITE_AUTH_PREFIX`: 登录前缀，默认 `/auth`（与后端 `/api/v1/auth` 对应）。
- `VITE_WS_BASE`: WebSocket 基址，示例 `wss://<域名>/api/v1`。
- `VITE_TENANT_HEADER`: 租户头，默认 `X-Tenant-Id`。
- `VITE_DEFAULT_TENANT`: 缺省租户 ID，默认 `global`。

## 透传与网关
- 如经反向代理，请确保 WebSocket 连接启用 `proxy_http_version 1.1`，并透传 `Upgrade` 与 `Connection` 头。
- 建议将机密（DB 密码、SMTP、Webhook Token）放入外部 Secret，通过环境变量注入。

## 建议
- 生产环境将机密通过环境变量或外部 Secret 提供。
- 单租户模式无需配置 `storage.tenants`。
- 变更前备份配置并记录审计，确保可回滚。
