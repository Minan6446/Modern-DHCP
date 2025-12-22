# 18. 存储与备份蓝图

本计划落实系统规范第 18 章，说明 Modern-DHCP 如何在多种持久化后端之间切换，并实现端到端的备份、验证与一键恢复能力。对应配置位于 `configs/config.example.yaml` 中的 `storage.*` 与 `backup.*` 节点。

## 18.1 多后端存储

| 类别 | 方案 | 说明 |
| --- | --- | --- |
| 关系数据库 | MySQL 8.0+、PostgreSQL 12+、AWS RDS/Aurora、Azure SQL | `storage.relational` 定义主/备优先级、连接池参数与 TLS；MySQL 作为当前默认实现，PostgreSQL 由迁移工具同步 schema，RDS/SQL Database 提供托管变体。 |
| NoSQL / Cache | Redis Cluster、MongoDB | `storage.nosql.redis` 描述三节点集群或 Elasticache，配合租约热缓存；`storage.nosql.mongo` 预留事件文档或审计归档用途。 |
| 分布式协调 | Etcd、Consul、Zookeeper | `storage.distributed.*` 统一定义 endpoint/TLS/namespace，供 HA 协调器与配置同步组件切换。 |
| 云存储 | S3、Azure Blob、OSS | `storage.cloud.objectStores` 指定备份与归档所在 bucket/region/kms。 |

### 切换策略
1. **主数据库**：`storage.relational.primary` 控制当前权威库（默认 `mysql`）。`failover` 字段预设灾备接管顺序。
2. **热备同步**：通过 MySQL async replication 或 PostgreSQL logical replication，同步至对侧后端；事件流由 Kafka CDC 推送。
3. **配置引用**：HTTP/租约服务仍使用现有 `mysql.*` 字段建立连接，`storage.*` 作为描述性元数据及日后多后端自动化所需的清单。迁移计划：当 `primary=postgres` 时，CLI/运维剧本读取 `storage.relational.postgres.dsn` 生成新的 DSN。
4. **NoSQL 冗余**：Redis cluster 模式 + 哨兵，Mongo 仅在启用时写入（event store / 审计归档）。
5. **协调服务**：默认 etcd，Consul/Zookeeper 作为兼容层，用于边缘/混合云或已有栈。

## 18.2 备份恢复

### 18.2.1 自动定时备份
- `backup.schedule` 使用 cron 表达式（示例：每 4 小时捕获增量）。
- `backup.fullInterval` 与 `incrementalInterval` 约定全量（24h）与增量（4h）任务；执行顺序：全量 → 多个增量 → 验证。
- 通过调度器触发 `mysqldump`/`xtrabackup`/`pg_basebackup`、Redis RDB/AOF、Kafka topic retention snapshots。

### 18.2.2 保留策略
- `backup.retentionDays: 30` 表示至少保留近 30 天的可恢复点；超过 30 天的备份转入 Glacier/Archive，并在 `storage.cloud.objectStores` 中标记 `prefix=archive/`。
- 支持按租户/区域细分保留策略（未来扩展 `backup.policies`）。

### 18.2.3 一键恢复
- `backup.restoreUi.enabled` + `url` 指向 Web 控制台，允许拥有审批的管理员从备份集中选择时间点，系统自动：
  1. 在隔离命名空间创建临时数据库/缓存。
  2. 回放备份并执行一致性校验。
  3. 切换 HA 控制器或生成迁移脚本用于生产回滚。
- `authProvider` 和 `auditSink` 确保操作可追溯（Okta 登录、Kafka 审计事件）。

### 18.2.4 异地与跨区域
- `backup.locations` 同时写入本地 NAS 与 S3；`crossRegion` 启用 bucket replication，`lagTarget` 控制最大跨区延迟。
- 对于 PostgreSQL/Aurora，可利用数据库原生跨区副本；对象存储用于冷备。

### 18.2.5 备份验证与测试恢复
- `backup.verification` 定期（示例：每周日 02:30）在 `sandboxEnv` 恢复全量 + 最近增量，运行租约一致性测试与 API 冒烟。
- 验证结果推送至 `alertChannel`（例如 ops-chat/Slack），失败会触发 `CRITICAL` 告警。

### 流程示意
1. **捕获**：按照计划执行全量/增量，生成带 metadata 的 manifest（commit hash、schema 版本、HA role）。
2. **复制**：同步到二级存储及跨区域 object store。
3. **验证**：在隔离环境拉起容器，执行 `modern-dhcp verify-backup --manifest ...`。
4. **发布**：更新 Catalog/API，供恢复 UI 选择；将 manifest 写入 `storage.cloud.objectStores.prefix/manifest.json`。

## 18.3 集成点与未来工作
- 编排层（Terraform/Ansible）可读取 `storage.*` 与 `backup.*`，根据目标环境渲染适配脚本。
- 计划加入 PostgreSQL 驱动与 Mongo 审计写入路径，届时配置将直接驱动运行时行为。
- 备份 Catalog API 将与管理 UI 合并，支持 RBAC/审批、自动生成演练报告。