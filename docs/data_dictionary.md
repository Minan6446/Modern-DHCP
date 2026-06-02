# 数据字典

> 单租户基线说明：当前数据库以 [migrations/0001_schema_no_tenant.sql](../migrations/0001_schema_no_tenant.sql) 为准，本文件为单租户版本的字段说明。

生成时间：2026-02-26

数据来源：migrations/0001_schema_no_tenant.sql, runtime/cluster_config (server 初始化自动创建)

## address_pools

- 描述：地址池与地址段定义，支持层级池、网络与分配策略。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 地址池唯一标识。 |
| parent_id | VARCHAR(36) | 是 | - | 父级地址池 ID，用于层级继承。 |
| scope | VARCHAR(16) | 否 | GLOBAL | 作用域类型（全局/子池）。 |
| name | VARCHAR(128) | 否 | - | 地址池名称。 |
| cidr | VARCHAR(64) | 否 | - | 地址池 CIDR。 |
| network | VARCHAR(64) | 是 | - | 网络地址（可缓存计算结果）。 |
| netmask | VARCHAR(64) | 是 | - | 子网掩码（可缓存计算结果）。 |
| range_start | VARCHAR(64) | 是 | - | 地址分配起始范围。 |
| range_end | VARCHAR(64) | 是 | - | 地址分配结束范围。 |
| gateway | VARCHAR(64) | 是 | - | 默认网关地址。 |
| option_43 | VARCHAR(255) | 否 | '' | Option 43 的字符串值。 |
| dns | JSON | 是 | - | DNS 服务器列表。 |
| exclusions | JSON | 是 | - | 排除地址区间列表。 |
| vlan_id | INT | 是 | - | VLAN 编号。 |
| interface_id | VARCHAR(64) | 是 | - | 绑定接口 ID。 |
| ssid | VARCHAR(64) | 是 | - | 关联无线 SSID。 |
| location | VARCHAR(128) | 是 | - | 物理或逻辑位置。 |
| geo_code | VARCHAR(128) | 是 | - | 地理编码或区域代码。 |
| device_profile | VARCHAR(64) | 是 | - | 设备画像/类型标签。 |
| tag_fingerprint | CHAR(40) | 是 | - | tags 的 SHA1 指纹，用于索引。 |
| reserve_percent | INT | 是 | 10 | 保留地址比例（百分比）。 |
| lease_profile_id | VARCHAR(36) | 否 | - | 关联租期策略 ID。 |
| tags | JSON | 是 | - | 自定义标签。 |
| allocation_mode | VARCHAR(32) | 否 | ROUND_ROBIN | 地址分配算法。 |
| priority_weight | INT | 否 | 1 | 分配优先级权重。 |
| status | VARCHAR(16) | 否 | active | 地址池状态。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## pool_usage_daily

- 描述：地址池每日使用量快照，用于历史趋势分析。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID。 |
| pool_id | VARCHAR(64) | 否 | - | 地址池 ID。 |
| day | DATE | 否 | - | 统计日期（按天）。 |
| used | BIGINT | 否 | - | 已使用地址数量。 |
| capacity | BIGINT | 否 | - | 地址池总容量。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## audit_events

- 描述：审计事件记录，用于追踪操作行为与请求上下文。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT | 否 | - | 自增主键。 |
| audit_id | VARCHAR(64) | 否 | - | 外部可追踪的审计事件 ID。 |
| actor | VARCHAR(64) | 否 | - | 操作者标识（用户/服务）。 |
| action | VARCHAR(64) | 否 | - | 动作类型。 |
| source | VARCHAR(64) | 否 | api | 事件来源（api/system 等）。 |
| resource | VARCHAR(128) | 是 | - | 资源标识或路径。 |
| correlation_id | VARCHAR(64) | 是 | - | 关联请求/链路 ID。 |
| payload | JSON | 是 | - | 扩展审计上下文。 |
| created_at | DATETIME | 否 | - | 事件时间。 |

## alert_rules

- 描述：自定义告警规则定义。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 规则 ID，主键。 |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID。 |
| name | VARCHAR(255) | 否 | - | 规则名称。 |
| expression | VARCHAR(512) | 否 | - | 规则表达式。 |
| operator | VARCHAR(8) | 否 | - | 比较操作符。 |
| threshold | DOUBLE | 否 | - | 阈值。 |
| duration_sec | INT | 否 | - | 持续时间（秒）。 |
| severity | VARCHAR(32) | 否 | - | 告警级别。 |
| enabled | TINYINT(1) | 否 | 1 | 是否启用。 |
| match_template | TEXT | 是 | - | 匹配模板。 |
| created_at | DATETIME(6) | 否 | - | 创建时间。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |

## alert_routes

- 描述：告警路由与升级策略。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT | 否 | - | 路由 ID，自增主键。 |
| name | VARCHAR(128) | 否 | - | 路由名称，唯一。 |
| description | VARCHAR(512) | 是 | - | 描述。 |
| severities | JSON | 否 | - | 适用告警级别。 |
| channels | JSON | 否 | - | 通知渠道列表。 |
| escalation_minutes | INT | 否 | 0 | 升级等待时间（分钟）。 |
| enabled | TINYINT(1) | 否 | 1 | 是否启用。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | system | 更新者标识。 |

## alert_thresholds

- 描述：告警阈值配置，控制各资源的触发敏感度。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID，主键。 |
| resource_pool_usage | INT | 否 | 0 | 地址池使用率阈值。 |
| resource_lease_usage | INT | 否 | 0 | 租约使用率阈值。 |
| resource_renew_fail | INT | 否 | 0 | 租约续约失败阈值。 |
| resource_lease_time_drift | INT | 否 | 0 | 租约时间漂移阈值。 |
| resource_failed_request_ratio | INT | 否 | 0 | 请求失败比例阈值。 |
| resource_subnet_imbalance | INT | 否 | 0 | 子网不平衡阈值。 |
| resource_log_error_threshold | INT | 否 | 0 | 日志错误计数阈值。 |
| server_response_timeout | INT | 否 | 0 | 服务器响应超时次数阈值。 |
| server_response_time_ms | INT | 否 | 0 | 服务器响应时间阈值（毫秒）。 |
| server_cpu_usage | INT | 否 | 0 | CPU 使用率阈值。 |
| server_memory_usage | INT | 否 | 0 | 内存使用率阈值。 |
| server_process_check | TINYINT(1) | 否 | 1 | 是否开启进程存活检查。 |
| network_conflict_sensitivity | VARCHAR(16) | 否 | medium | IP 冲突检测灵敏度。 |
| network_abnormal_qps | INT | 否 | 0 | 异常 QPS 阈值。 |
| network_duplicate_ip_detection | TINYINT(1) | 否 | 0 | 是否检测重复 IP。 |
| network_unauthorized_server_detection | TINYINT(1) | 否 | 0 | 是否检测未授权服务器。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | system | 更新者标识。 |

## alert_notify_configs

- 描述：告警通知渠道与策略配置。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID，主键。 |
| channels_email | TINYINT(1) | 否 | 1 | 邮件渠道开关。 |
| channels_sms | TINYINT(1) | 否 | 0 | 短信渠道开关。 |
| channels_webhook | TINYINT(1) | 否 | 0 | Webhook 渠道开关。 |
| webhook_url | VARCHAR(512) | 是 | - | Webhook 地址。 |
| policies_emergency | JSON | 否 | - | 紧急级别策略。 |
| policies_critical | JSON | 否 | - | 严重级别策略。 |
| policies_info | JSON | 否 | - | 信息级别策略。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | system | 更新者标识。 |

## alert_templates

- 描述：告警通知模板，支持多语言多渠道内容。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 模板 ID，主键。 |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID。 |
| name | VARCHAR(255) | 否 | - | 模板名称。 |
| lang | VARCHAR(32) | 否 | - | 语言。 |
| channel | VARCHAR(32) | 否 | - | 通知渠道（email/sms/webhook 等）。 |
| subject | VARCHAR(255) | 是 | - | 主题。 |
| body | TEXT | 是 | - | 正文。 |
| variables | JSON | 是 | - | 可用变量列表。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | system | 更新者标识。 |

## alert_receivers

- 描述：告警接收人及其偏好。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 接收人 ID，主键。 |
| tenant_id | VARCHAR(64) | 否 | - | 租户 ID。 |
| name | VARCHAR(128) | 否 | - | 姓名/称呼。 |
| email | VARCHAR(160) | 否 | - | 邮箱。 |
| phone | VARCHAR(64) | 是 | - | 手机号。 |
| levels | JSON | 否 | - | 关注的告警级别。 |
| department | VARCHAR(128) | 是 | - | 部门。 |
| schedule | VARCHAR(128) | 是 | - | 排班/值班计划。 |
| server_groups | JSON | 是 | - | 负责的服务器或分组。 |
| created_at | DATETIME(6) | 否 | - | 创建时间。 |
| updated_at | DATETIME(6) | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | system | 更新者标识。 |

## auth_api_keys

- 描述：API 密钥表，存储服务/用户使用的访问令牌。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | CHAR(36) | 否 | - | 密钥 ID。 |
| name | VARCHAR(128) | 否 | - | 密钥名称。 |
| prefix | VARCHAR(16) | 否 | - | 密钥前缀，用于快速查找。 |
| token_hash | CHAR(64) | 否 | - | 完整密钥哈希。 |
| role | VARCHAR(32) | 否 | - | 绑定角色。 |
| principal_id | VARCHAR(128) | 否 | - | 关联主体（用户/服务）ID。 |
| tenant_scope | VARCHAR(64) | 是 | - | 会话令牌可见租户范围。 |
| kind | VARCHAR(16) | 否 | service | 密钥类型（service/user）。 |
| owner_user_id | CHAR(36) | 是 | - | 所属用户 ID。 |
| description | TEXT | 是 | - | 密钥用途说明。 |
| created_by | VARCHAR(128) | 否 | - | 创建者标识。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| expires_at | DATETIME | 是 | - | 过期时间。 |
| last_used_at | DATETIME | 是 | - | 最近使用时间。 |
| revoked_at | DATETIME | 是 | - | 吊销时间。 |
| revoked_by | VARCHAR(128) | 是 | - | 吊销操作者。 |
| metadata | JSON | 是 | - | 扩展元数据。 |

## auth_user_roles

- 描述：用户与角色的多对多映射。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | CHAR(36) | 否 | - | 记录 ID，主键。 |
| user_id | CHAR(36) | 否 | - | 用户 ID。 |
| role | VARCHAR(64) | 否 | - | 角色标识。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## auth_identity_providers

- 描述：身份提供商配置（OIDC/SAML 等）。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 提供商 ID。 |
| name | VARCHAR(128) | 否 | - | 显示名称。 |
| type | VARCHAR(32) | 否 | - | 提供商类型。 |
| endpoint | VARCHAR(512) | 是 | - | 提供商端点 URL。 |
| config | JSON | 是 | - | 认证配置参数。 |
| enabled | TINYINT(1) | 否 | 1 | 是否启用。 |
| created_at | TIMESTAMP | 否 | CURRENT_TIMESTAMP | 创建时间。 |
| updated_at | TIMESTAMP | 否 | CURRENT_TIMESTAMP | 更新时间。 |

## auth_users

- 描述：认证用户表，保存控制台账户与状态。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | CHAR(36) | 否 | - | 用户 ID。 |
| username | VARCHAR(64) | 否 | - | 登录用户名。 |
| display_name | VARCHAR(128) | 否 | - | 显示名称。 |
| email | VARCHAR(160) | 是 | - | 邮箱。 |
| password_hash | VARBINARY(255) | 否 | - | 密码哈希。 |
| role | VARCHAR(32) | 否 | reader | 默认角色标识。 |
| status | VARCHAR(16) | 否 | active | 账号状态。 |
| must_change_password | TINYINT(1) | 否 | 0 | 是否首次登录强制改密。 |
| last_login_at | DATETIME | 是 | - | 最近登录时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## cluster_config

- 描述：集群级配置存储（HA、HA-LB、同步、节点列表等），由后端启动时自动建表并用于持久化控制面共享配置。
- 定义位置：运行时自动创建（`internal/server/cluster_config_store.go`）。

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| ckey | VARCHAR(64) | 否 | - | 配置键（例如 `cluster_ha_config`、`cluster_nodes`）。 |
| cvalue | JSON | 否 | - | 配置值（JSON 对象或数组，随键定义）。 |
| updated_at | TIMESTAMP | 否 | CURRENT_TIMESTAMP | 最近更新时间，自动随写入更新。 |

## automation_change_requests

- 描述：自动化变更的审批请求与结果。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 变更请求 ID。 |
| job_type | VARCHAR(64) | 否 | - | 关联作业类型。 |
| request_type | VARCHAR(32) | 否 | - | 请求类型（create/update 等）。 |
| status | VARCHAR(32) | 否 | pending | 审批状态。 |
| requested_by | VARCHAR(128) | 否 | - | 发起人标识。 |
| requested_at | DATETIME | 否 | - | 发起时间。 |
| approver_id | VARCHAR(128) | 是 | - | 审批人标识。 |
| decided_at | DATETIME | 是 | - | 决策时间。 |
| decision_note | TEXT | 是 | - | 审批意见。 |
| original_config | JSON | 是 | - | 原配置快照。 |
| proposed_config | JSON | 是 | - | 目标配置快照。 |
| payload | JSON | 是 | - | 变更载荷。 |
| auto_applied | TINYINT(1) | 否 | 0 | 是否自动应用。 |
| applied_at | DATETIME | 是 | - | 应用时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| version | INT | 否 | 1 | 版本号。 |

## automation_jobs

- 描述：自动化作业队列表。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 作业 ID。 |
| job_type | VARCHAR(64) | 否 | - | 作业类型。 |
| status | VARCHAR(32) | 否 | pending | 作业状态。 |
| source | VARCHAR(32) | 否 | '' | 触发来源。 |
| triggered_by | VARCHAR(128) | 否 | '' | 触发者标识。 |
| priority | INT | 否 | 0 | 优先级。 |
| attempts | INT | 否 | 0 | 重试次数。 |
| payload_hash | CHAR(64) | 否 | '' | 载荷哈希。 |
| labels | JSON | 是 | - | 作业标签。 |
| payload | JSON | 是 | - | 作业载荷。 |
| result_summary | TEXT | 是 | - | 结果摘要。 |
| error_message | TEXT | 是 | - | 错误信息。 |
| not_before | DATETIME | 是 | - | 最早执行时间。 |
| queued_at | DATETIME | 否 | - | 入队时间。 |
| started_at | DATETIME | 是 | - | 开始时间。 |
| completed_at | DATETIME | 是 | - | 完成时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## collab_approvals

- 描述：协作审批记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 审批记录 ID。 |
| workflow_id | VARCHAR(64) | 否 | - | 审批流程 ID。 |
| stage | INT | 否 | - | 审批阶段。 |
| approver_id | VARCHAR(64) | 否 | - | 审批人 ID。 |
| decision | VARCHAR(16) | 否 | pending | 审批结果。 |
| decided_at | DATETIME | 是 | - | 审批时间。 |
| comment | TEXT | 是 | - | 备注。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| deleted_at | DATETIME | 是 | - | 删除时间。 |
| version | INT | 否 | 0 | 乐观锁版本。 |

## collab_comments

- 描述：协作评论。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 评论 ID。 |
| resource_type | VARCHAR(64) | 否 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 否 | - | 资源 ID。 |
| author_id | VARCHAR(64) | 否 | - | 作者 ID。 |
| parent_id | VARCHAR(64) | 是 | - | 父评论 ID。 |
| body | TEXT | 否 | - | 评论内容。 |
| status | VARCHAR(32) | 否 | open | 评论状态。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| deleted_at | DATETIME | 是 | - | 删除时间。 |
| version | INT | 否 | 0 | 乐观锁版本。 |

## collab_events

- 描述：协作事件流水。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT | 否 | - | 自增事件 ID。 |
| session_id | VARCHAR(64) | 是 | - | 协作会话 ID。 |
| resource_type | VARCHAR(64) | 是 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 是 | - | 资源 ID。 |
| event_type | VARCHAR(64) | 否 | - | 事件类型。 |
| payload | JSON | 是 | - | 事件载荷。 |
| created_at | DATETIME | 否 | - | 事件时间。 |

## collab_locks

- 描述：协作资源锁。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 锁 ID。 |
| resource_type | VARCHAR(64) | 否 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 否 | - | 资源 ID。 |
| session_id | VARCHAR(64) | 否 | - | 关联会话 ID。 |
| status | VARCHAR(32) | 否 | active | 锁状态。 |
| acquired_at | DATETIME | 否 | - | 获取时间。 |
| expires_at | DATETIME | 否 | - | 过期时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| deleted_at | DATETIME | 是 | - | 删除时间。 |
| version | INT | 否 | 0 | 乐观锁版本。 |

## collab_sessions

- 描述：协作会话。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 会话 ID。 |
| resource_type | VARCHAR(64) | 否 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 否 | - | 资源 ID。 |
| user_id | VARCHAR(64) | 否 | - | 参与用户 ID。 |
| status | VARCHAR(32) | 否 | active | 会话状态。 |
| lock_version | INT | 否 | 0 | 资源锁版本。 |
| expires_at | DATETIME | 否 | - | 过期时间。 |
| metadata | JSON | 是 | - | 会话元数据。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| deleted_at | DATETIME | 是 | - | 删除时间。 |
| version | INT | 否 | 0 | 乐观锁版本。 |

## collab_tasks

- 描述：协作任务与待办。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 任务 ID。 |
| title | VARCHAR(256) | 否 | - | 任务标题。 |
| assignee_id | VARCHAR(64) | 是 | - | 指派用户 ID。 |
| resource_type | VARCHAR(64) | 否 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 否 | - | 资源 ID。 |
| resource_ref | JSON | 是 | - | 资源引用信息。 |
| state | VARCHAR(32) | 否 | todo | 任务状态。 |
| priority | INT | 否 | 3 | 优先级。 |
| due_at | DATETIME | 是 | - | 截止时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| deleted_at | DATETIME | 是 | - | 删除时间。 |
| version | INT | 否 | 0 | 乐观锁版本。 |

## dhcp_custom_options

- 描述：自定义 DHCP 选项定义。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 自定义选项 ID。 |
| code | INT | 否 | - | DHCP 选项编号。 |
| name | VARCHAR(255) | 否 | - | 选项名称。 |
| scope | VARCHAR(24) | 否 | - | 作用域（global/pool/subnet）。 |
| format | VARCHAR(64) | 否 | - | 选项格式（字符串/数值等）。 |
| data_type | VARCHAR(64) | 否 | - | 数据类型。 |
| value | TEXT | 否 | - | 当前配置值。 |
| value_example | TEXT | 是 | - | 示例值。 |
| allowed_values | TEXT | 是 | - | 可选值集合。 |
| sample_value | TEXT | 是 | - | 采样值。 |
| description | TEXT | 是 | - | 选项描述。 |
| tags | TEXT | 是 | - | 标签或分类。 |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 创建时间。 |
| updated_at | DATETIME | 否 | CURRENT_TIMESTAMP | 更新时间。 |

## dhcp_option_templates

- 描述：DHCP 选项配置模板定义，支持模板图标与模板选项快照，避免页面内存/文件丢失风险。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 模板 ID。 |
| name | VARCHAR(255) | 否 | - | 模板名称。 |
| description | TEXT | 是 | - | 模板描述。 |
| icon | VARCHAR(32) | 是 | - | 模板图标（emoji）。 |
| options | LONGTEXT | 否 | - | 模板选项快照（JSON 序列化数组）。 |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 创建时间。 |
| updated_at | DATETIME | 否 | CURRENT_TIMESTAMP | 更新时间。 |

## dhcp_option_template_apply_history

- 描述：DHCP 配置模板应用历史，记录操作人、应用目标与模板快照，用于审计与回滚追踪。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT UNSIGNED | 否 | - | 历史记录主键（自增）。 |
| template_id | VARCHAR(64) | 否 | - | 模板 ID。 |
| template_name | VARCHAR(255) | 否 | '' | 模板名称快照。 |
| action | VARCHAR(32) | 否 | apply | 操作类型（当前为 apply）。 |
| operator | VARCHAR(128) | 否 | system | 操作人（用户/服务主体）。 |
| target_scope | VARCHAR(64) | 否 | global | 应用目标范围。 |
| applied_option_count | INT | 否 | 0 | 本次应用的选项数量。 |
| template_snapshot | LONGTEXT | 是 | - | 应用时模板完整快照（JSON）。 |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 应用时间。 |

## dhcp_option_scopes

- 描述：DHCP 选项作用域配置与模板绑定信息。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 作用域 ID。 |
| name | VARCHAR(255) | 否 | - | 作用域名称。 |
| subnet | VARCHAR(64) | 否 | '' | 子网。 |
| range_value | VARCHAR(128) | 否 | '' | 地址范围。 |
| gateway | VARCHAR(64) | 否 | '' | 网关地址。 |
| status | VARCHAR(16) | 否 | active | 状态。 |
| scope_type | VARCHAR(32) | 否 | GLOBAL | 作用域类型。 |
| target | VARCHAR(128) | 否 | global | 目标对象。 |
| template_id | VARCHAR(64) | 是 | - | 绑定模板 ID。 |
| option_ids | LONGTEXT | 否 | - | 绑定选项 ID 列表（JSON 序列化）。 |
| description | TEXT | 是 | - | 描述。 |
| notes | TEXT | 是 | - | 备注。 |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 创建时间。 |
| updated_at | DATETIME | 否 | CURRENT_TIMESTAMP | 更新时间。 |

## iot_device_profiles

- 描述：IoT 设备档案与休眠策略。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 设备档案 ID。 |
| name | VARCHAR(128) | 否 | - | 档案名称。 |
| description | TEXT | 是 | - | 档案描述。 |
| sleep_class | VARCHAR(16) | 否 | NORMAL | 休眠等级。 |
| sleep_interval | BIGINT | 否 | 0 | 休眠间隔（秒）。 |
| offline_window | BIGINT | 否 | 0 | 允许离线窗口（秒）。 |
| lease_profile_id | VARCHAR(36) | 是 | - | 关联租期策略。 |
| sleepy_capable | TINYINT(1) | 否 | 0 | 是否支持休眠。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## iot_devices

- 描述：IoT 设备注册信息与状态。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 设备记录 ID。 |
| device_id | VARCHAR(128) | 否 | - | 设备唯一标识。 |
| display_name | VARCHAR(128) | 是 | - | 设备显示名。 |
| hardware_addr | VARCHAR(64) | 是 | - | 硬件地址。 |
| profile_id | VARCHAR(36) | 是 | - | 关联档案 ID。 |
| lease_profile_id | VARCHAR(36) | 是 | - | 关联租期策略。 |
| sleep_class | VARCHAR(16) | 否 | NORMAL | 休眠等级。 |
| sleep_interval | BIGINT | 否 | 0 | 休眠间隔（秒）。 |
| offline_window | BIGINT | 否 | 0 | 允许离线窗口（秒）。 |
| sleepy_hint | TINYINT(1) | 否 | 0 | 是否建议休眠。 |
| status | VARCHAR(16) | 否 | ACTIVE | 设备状态。 |
| firmware_version | VARCHAR(64) | 是 | - | 固件版本。 |
| labels | JSON | 是 | - | 标签集合。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| last_seen | DATETIME | 是 | - | 最近在线时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## lease_profiles

- 描述：租期策略定义。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 租期策略 ID。 |
| name | VARCHAR(128) | 否 | - | 策略名称。 |
| default_duration | BIGINT | 否 | - | 默认租期（秒）。 |
| min_duration | BIGINT | 否 | - | 最小租期（秒）。 |
| max_duration | BIGINT | 否 | - | 最大租期（秒）。 |
| renewal_time | BIGINT | 否 | 0 | 续租时间（T1）。 |
| rebinding_time | BIGINT | 否 | 0 | 重新绑定时间（T2）。 |
| notification_lead | BIGINT | 否 | - | 到期提醒提前量（秒）。 |
| infinite | TINYINT(1) | 否 | 0 | 是否为永久租期。 |
| device_type | VARCHAR(64) | 是 | - | 适用设备类型。 |
| sleepy_capable | TINYINT(1) | 否 | 0 | 是否支持休眠。 |
| sleepy_offline_window | BIGINT | 否 | 0 | 休眠离线窗口（秒）。 |
| sleepy_hold_duration | BIGINT | 否 | 0 | 休眠保留时长（秒）。 |
| mobility_grace_period | BIGINT | 否 | 0 | 漫游宽限期（秒）。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## lease_history_v4

- 描述：归档的 DHCPv4 租约历史记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT UNSIGNED | 否 | - | 历史记录主键（自增）。 |
| lease_id | VARCHAR(36) | 否 | - | 原租约 ID。 |
| tenant_id | VARCHAR(64) | 否 | global | 租户标识。 |
| pool_id | VARCHAR(36) | 否 | - | 地址池 ID。 |
| ip_address | VARCHAR(64) | 否 | - | IP 地址（字符串）。 |
| hardware_addr | VARCHAR(64) | 否 | - | MAC 地址。 |
| client_id | VARCHAR(128) | 是 | - | 客户端 ID。 |
| user_id | VARCHAR(64) | 是 | - | 用户标识。 |
| mobility_anchor_id | VARCHAR(128) | 否 | '' | 漫游锚点 ID。 |
| device_type | VARCHAR(64) | 是 | - | 设备类型。 |
| last_access_point_id | VARCHAR(128) | 否 | '' | 最近接入点 ID。 |
| last_controller_id | VARCHAR(128) | 否 | '' | 最近控制器 ID。 |
| last_geo_zone | VARCHAR(128) | 否 | '' | 最近地理区域。 |
| mobility_location_hint | VARCHAR(255) | 否 | '' | 位置提示。 |
| mdm_managed | TINYINT(1) | 否 | 0 | 是否纳管（MDM）。 |
| mdm_source | VARCHAR(64) | 是 | - | MDM 来源。 |
| mdm_tags | JSON | 是 | - | MDM 标签。 |
| mdm_observed_at | DATETIME | 是 | - | MDM 最近观察时间。 |
| relay_info | JSON | 是 | - | 中继信息。 |
| session_continuity | JSON | 是 | - | 会话连续性信息。 |
| expires_at | DATETIME | 否 | - | 到期时间。 |
| final_state | VARCHAR(16) | 否 | - | 归档时状态。 |
| security_state | VARCHAR(16) | 否 | OK | 安全状态。 |
| cooldown_until | DATETIME | 是 | - | 冷却到期时间。 |
| conflict_history | JSON | 是 | - | 冲突历史。 |
| created_at | DATETIME | 否 | - | 原记录创建时间。 |
| updated_at | DATETIME | 否 | - | 原记录更新时间。 |
| archived_at | DATETIME | 否 | - | 归档时间。 |

## leases_v4

- 描述：IPv4 租约记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 租约 ID。 |
| pool_id | VARCHAR(36) | 否 | - | 关联地址池 ID。 |
| ip_address | VARBINARY(16) | 否 | - | IPv4 地址（二进制）。 |
| hardware_addr | VARCHAR(64) | 否 | - | MAC 地址。 |
| client_id | VARCHAR(128) | 是 | - | 客户端 ID。 |
| user_id | VARCHAR(64) | 是 | - | 用户标识。 |
| mobility_anchor_id | VARCHAR(128) | 否 | '' | 漫游锚点 ID。 |
| device_type | VARCHAR(64) | 是 | - | 设备类型。 |
| last_access_point_id | VARCHAR(128) | 否 | '' | 最近接入点 ID。 |
| last_controller_id | VARCHAR(128) | 否 | '' | 最近控制器 ID。 |
| last_geo_zone | VARCHAR(128) | 否 | '' | 最近地理区域。 |
| mobility_location_hint | VARCHAR(255) | 否 | '' | 位置提示。 |
| relay_info | JSON | 是 | - | 中继信息。 |
| session_continuity | JSON | 是 | - | 会话连续性信息。 |
| expires_at | DATETIME | 否 | - | 租约到期时间。 |
| state | VARCHAR(16) | 否 | - | 租约状态。 |
| security_state | VARCHAR(16) | 否 | OK | 安全状态。 |
| cooldown_until | DATETIME | 是 | - | 冷却到期时间。 |
| conflict_history | JSON | 是 | - | 冲突历史。 |
| mdm_managed | TINYINT(1) | 否 | 0 | 是否纳管（MDM）。 |
| mdm_source | VARCHAR(64) | 是 | - | MDM 来源。 |
| mdm_tags | JSON | 是 | - | MDM 标签。 |
| mdm_observed_at | DATETIME | 是 | - | MDM 最近观察时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## leases_v6

- 描述：IPv6 租约记录（字段与 leases_v4 一致）。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 租约 ID。 |
| pool_id | VARCHAR(36) | 否 | - | 关联地址池 ID。 |
| ip_address | VARBINARY(16) | 否 | - | IPv6 地址（二进制）。 |
| hardware_addr | VARCHAR(64) | 否 | - | MAC 地址。 |
| client_id | VARCHAR(128) | 是 | - | 客户端 ID。 |
| user_id | VARCHAR(64) | 是 | - | 用户标识。 |
| mobility_anchor_id | VARCHAR(128) | 否 | '' | 漫游锚点 ID。 |
| device_type | VARCHAR(64) | 是 | - | 设备类型。 |
| last_access_point_id | VARCHAR(128) | 否 | '' | 最近接入点 ID。 |
| last_controller_id | VARCHAR(128) | 否 | '' | 最近控制器 ID。 |
| last_geo_zone | VARCHAR(128) | 否 | '' | 最近地理区域。 |
| mobility_location_hint | VARCHAR(255) | 否 | '' | 位置提示。 |
| relay_info | JSON | 是 | - | 中继信息。 |
| session_continuity | JSON | 是 | - | 会话连续性信息。 |
| expires_at | DATETIME | 否 | - | 租约到期时间。 |
| state | VARCHAR(16) | 否 | - | 租约状态。 |
| security_state | VARCHAR(16) | 否 | OK | 安全状态。 |
| cooldown_until | DATETIME | 是 | - | 冷却到期时间。 |
| conflict_history | JSON | 是 | - | 冲突历史。 |
| mdm_managed | TINYINT(1) | 否 | 0 | 是否纳管（MDM）。 |
| mdm_source | VARCHAR(64) | 是 | - | MDM 来源。 |
| mdm_tags | JSON | 是 | - | MDM 标签。 |
| mdm_observed_at | DATETIME | 是 | - | MDM 最近观察时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## ops_system_settings

- 描述：系统级运行设置（主题、维护窗口等）。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | TINYINT | 否 | 1 | 固定主键。 |
| theme | VARCHAR(64) | 否 | '' | UI 主题。 |
| locale | VARCHAR(32) | 否 | '' | 默认语言。 |
| maintenance_mode | TINYINT(1) | 否 | 0 | 是否维护模式。 |
| maintenance_window | VARCHAR(128) | 是 | - | 维护窗口说明。 |
| announcement | TEXT | 是 | - | 公告内容。 |
| admin_session_timeout_minutes | INT | 否 | 30 | 管理端会话超时（分钟）。 |
| auto_logout_enabled | TINYINT(1) | 否 | 1 | 是否启用自动登出。 |
| system_log_retention_days | INT | 否 | 30 | 系统日志保留天数。 |
| audit_log_retention_days | INT | 否 | 180 | 审计日志保留天数。 |
| log_push_enabled | TINYINT(1) | 否 | 0 | 是否启用日志推送。 |
| log_push_endpoint | VARCHAR(512) | 是 | - | 日志推送地址（Webhook）。 |
| log_push_min_level | VARCHAR(16) | 否 | warning | 日志推送最小级别。 |
| log_push_channels | VARCHAR(256) | 否 | '' | 日志推送渠道列表（逗号分隔：dingtalk/feishu/wecom/slack/phone）。 |
| log_push_phones | VARCHAR(512) | 否 | '' | 电话告警号码列表（逗号分隔）。 |
| log_push_dingtalk_endpoint | VARCHAR(512) | 否 | '' | 钉钉推送地址。 |
| log_push_feishu_endpoint | VARCHAR(512) | 否 | '' | 飞书推送地址。 |
| log_push_wecom_endpoint | VARCHAR(512) | 否 | '' | 企业微信推送地址。 |
| log_push_slack_endpoint | VARCHAR(512) | 否 | '' | Slack 推送地址。 |
| ntp_enabled | TINYINT(1) | 否 | 1 | 是否启用 NTP 自动同步。 |
| ntp_servers | TEXT | 是 | - | NTP 服务器列表（换行分隔）。 |
| ntp_interval_minutes | INT | 否 | 30 | NTP 同步周期（分钟）。 |
| ntp_timeout_seconds | INT | 否 | 5 | NTP 同步超时时间（秒）。 |
| timezone | VARCHAR(64) | 否 | Asia/Shanghai | 系统时区。 |
| ntp_sync_status | VARCHAR(16) | 否 | idle | 最近同步状态。 |
| ntp_last_sync_at | DATETIME | 是 | - | 最近一次 NTP 同步时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| updated_by | VARCHAR(128) | 否 | - | 更新人标识。 |

## policy_drafts

- 描述：策略草稿定义。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 草稿 ID。 |
| name | VARCHAR(128) | 否 | - | 草稿名称。 |
| description | TEXT | 是 | - | 草稿描述。 |
| status | VARCHAR(32) | 否 | draft | 草稿状态。 |
| rules | JSON | 否 | - | 规则定义集合。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| created_by | VARCHAR(128) | 否 | - | 创建者标识。 |
| updated_by | VARCHAR(128) | 否 | - | 更新者标识。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |
| published_at | DATETIME | 是 | - | 发布时间。 |

## policy_rules

- 描述：已发布的策略规则集合。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 规则 ID。 |
| priority | INT | 否 | - | 规则优先级（数值越小优先级越高）。 |
| conditions | JSON | 否 | - | 匹配条件。 |
| actions | JSON | 否 | - | 执行动作。 |
| enabled | TINYINT | 否 | 1 | 是否启用。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## policy_versions

- 描述：策略版本历史。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 版本 ID。 |
| version | INT | 否 | - | 版本号。 |
| derived_from | VARCHAR(64) | 是 | - | 派生自的版本 ID。 |
| changelog | TEXT | 是 | - | 版本说明。 |
| rules | JSON | 否 | - | 规则定义集合。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| published_by | VARCHAR(128) | 否 | - | 发布人标识。 |
| published_at | DATETIME | 否 | - | 发布时间。 |
| rollback_of | VARCHAR(64) | 是 | - | 回滚目标版本 ID。 |
| created_at | DATETIME | 否 | - | 创建时间。 |

## prefix_leases_v6

- 描述：IPv6 前缀租约记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 前缀租约 ID。 |
| pool_id | VARCHAR(36) | 否 | - | 地址池 ID。 |
| client_id | VARCHAR(128) | 否 | - | 客户端标识。 |
| iapd_id | INT | 否 | - | IAPD 标识。 |
| prefix | VARCHAR(64) | 否 | - | IPv6 前缀。 |
| prefix_length | INT | 否 | - | 前缀长度。 |
| state | VARCHAR(16) | 否 | - | 租约状态。 |
| expires_at | DATETIME | 否 | - | 到期时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## rbac_approval_rules

- 描述：RBAC 审批规则配置。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 规则 ID。 |
| role_name | VARCHAR(64) | 否 | - | 目标角色。 |
| scope | VARCHAR(32) | 否 | - | 作用域类型。 |
| min_approvers | INT | 否 | 1 | 最少审批人数量。 |
| approver_role | VARCHAR(64) | 否 | - | 审批角色。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## rbac_assignments

- 描述：RBAC 角色分配关系。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 分配 ID。 |
| principal_id | VARCHAR(128) | 否 | - | 主体 ID。 |
| role_name | VARCHAR(64) | 否 | - | 角色名称。 |
| org_unit_id | VARCHAR(64) | 是 | - | 组织单元 ID。 |
| resource_type | VARCHAR(64) | 是 | - | 资源类型。 |
| resource_id | VARCHAR(64) | 是 | - | 资源 ID。 |
| created_by | VARCHAR(128) | 否 | - | 创建人标识。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| expires_at | DATETIME | 是 | - | 过期时间。 |
| attributes | JSON | 是 | - | 自定义属性。 |
| group_ids | JSON | 是 | - | 关联用户组。 |
| labels | JSON | 是 | - | 关联标签。 |
| org_scope | VARCHAR(64) | 否 | - | 组织作用域（生成列）。 |
| resource_scope | VARCHAR(64) | 否 | - | 资源作用域（生成列）。 |
| resource_key | VARCHAR(64) | 否 | - | 资源键（生成列）。 |
| group_scope | VARCHAR(191) | 否 | '' | 组作用域索引字段。 |
| label_scope | VARCHAR(191) | 否 | '' | 标签作用域索引字段。 |

## rbac_org_units

- 描述：RBAC 组织单元。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 组织单元 ID。 |
| parent_id | VARCHAR(64) | 是 | - | 父级组织单元 ID。 |
| name | VARCHAR(128) | 否 | - | 组织单元名称。 |
| path | VARCHAR(512) | 是 | - | 组织路径。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## rbac_roles

- 描述：RBAC 角色定义。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| name | VARCHAR(64) | 否 | - | 角色名称。 |
| inherits_from | VARCHAR(64) | 是 | - | 继承的角色名称。 |
| description | TEXT | 是 | - | 角色描述。 |
| capabilities | JSON | 是 | - | 能力集合。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## rbac_temp_grants

- 描述：临时授权与审批记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(64) | 否 | - | 临时授权 ID。 |
| assignment_id | VARCHAR(64) | 否 | - | 关联分配 ID。 |
| requested_by | VARCHAR(128) | 否 | - | 申请人标识。 |
| reason | TEXT | 是 | - | 申请原因。 |
| status | VARCHAR(16) | 否 | pending | 审批状态。 |
| expires_at | DATETIME | 否 | - | 失效时间。 |
| approved_by | VARCHAR(128) | 是 | - | 审批人标识。 |
| approved_at | DATETIME | 是 | - | 审批时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## security_mac_lists

- 描述：MAC 黑白名单与监控列表。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 列表项 ID。 |
| mac | VARCHAR(64) | 否 | - | MAC 地址。 |
| list_type | VARCHAR(16) | 否 | - | 列表类型（allow/deny/monitor）。 |
| action | VARCHAR(16) | 否 | monitor | 匹配后的动作。 |
| description | VARCHAR(255) | 是 | - | 描述信息。 |
| source | VARCHAR(64) | 是 | - | 录入来源。 |
| priority | INT | 否 | 100 | 优先级。 |
| enabled | TINYINT | 否 | 1 | 是否启用。 |
| valid_from | DATETIME | 是 | - | 生效时间。 |
| valid_until | DATETIME | 是 | - | 失效时间。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## security_policy_matches

- 描述：安全策略的匹配条件。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT | 否 | - | 自增主键。 |
| rule_id | CHAR(36) | 否 | - | 关联策略规则 ID。 |
| match_type | VARCHAR(32) | 否 | - | 匹配类型。 |
| match_value | VARCHAR(255) | 否 | - | 匹配值。 |
| negate | TINYINT(1) | 否 | 0 | 是否取反。 |
| created_at | DATETIME | 否 | - | 创建时间。 |

## security_policy_rules

- 描述：安全策略规则。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | CHAR(36) | 否 | - | 规则 ID。 |
| name | VARCHAR(128) | 否 | - | 规则名称。 |
| description | TEXT | 是 | - | 规则描述。 |
| priority | INT | 否 | 100 | 规则优先级。 |
| effect | VARCHAR(16) | 否 | allow | 执行效果（allow/deny）。 |
| enabled | TINYINT(1) | 否 | 1 | 是否启用。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

## static_bindings

- 描述：静态绑定记录。
- 定义位置：0001_schema_no_tenant.sql

| 字段 | 类型 | 允许为空 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | VARCHAR(36) | 否 | - | 绑定 ID。 |
| identifier | VARCHAR(128) | 否 | - | 绑定标识（MAC/ClientID）。 |
| identifier_type | VARCHAR(32) | 否 | - | 标识类型。 |
| pool_id | VARCHAR(36) | 否 | - | 地址池 ID。 |
| ip_address | VARBINARY(16) | 否 | - | 绑定 IP（二进制）。 |
| lease_profile_id | VARCHAR(36) | 否 | - | 租期策略 ID。 |
| metadata | JSON | 是 | - | 扩展元数据。 |
| status | VARCHAR(16) | 否 | offline | 绑定状态。 |
| status_source | VARCHAR(32) | 是 | - | 状态来源。 |
| last_seen_at | DATETIME | 是 | - | 最近上报时间。 |
| status_updated_at | DATETIME | 是 | - | 状态更新时间。 |
| created_at | DATETIME | 否 | - | 创建时间。 |
| updated_at | DATETIME | 否 | - | 更新时间。 |

