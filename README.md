# Modern-DHCP

Modern-DHCP 是一个采用 Go 编写、以 MySQL 为持久层的企业级双栈 DHCP 平台。此代码库目前包含实现需求所需的核心服务、规范以及数据库结构脚手架。

## 亮点
- 符合 RFC 2131/3315 的 DHCP 引擎骨架，并提供可扩展的策略钩子。
- 基于 MySQL 的分层地址池模型（global → subnet → VLAN → interface/port/SSID → location）。
- 具备 HA 感知的服务运行时、基于 Echo 的 REST 管理 API，以及基于 Zap 的结构化日志。
- 高可用架构涵盖主从 failover、多活集群与自动回切策略，详见 `docs/ha_architecture.md`。
- 通过配置驱动的部署方式支持环境覆盖，并为未来的 Docker/Kubernetes 目标做好准备。
- `docs/deployment_strategy.md` 描述了第 17 章：裸金属/虚拟化/容器/Kubernetes/公有云/混合云的部署方法与弹性伸缩流程。
- `docs/storage_backup_plan.md` 对应第 18 章，涵盖多后端存储矩阵、定时备份/验证、跨区域复制与一键恢复 UI。

## 目录结构
```
cmd/dhcpd             # 主服务入口
internal/config       # 基于 Viper 的配置加载器
internal/db           # 数据库连接器
internal/dhcpv4       # DHCPv4 处理器脚手架
internal/dhcpv6       # DHCPv6 处理器脚手架
internal/lease        # 租约生命周期服务
internal/policy       # 策略引擎接口与求值器
internal/server       # HTTP 管理服务器与健康检查
pkg/models            # 共享数据模型
configs               # 示例配置文件
migrations            # MySQL 迁移脚本
```

## 快速上手
1. 安装 Go 1.22 及以上版本，以及 MySQL 8.0 及以上版本。
2. 将 `configs/config.example.yaml` 复制为 `configs/config.yaml`，并根据需要调整连接串、HA 设置以及特性开关。
3. 执行迁移：运行占位命令 `make migrate`，或直接执行 `migrations/0001_init_schema.sql` 中的 SQL。
4. 启动服务：`go run ./cmd/dhcpd`。
5. 与管理 API 交互：
	- `GET /api/v1/tenants/{tenantId}/pools`：列出分层地址池。
	- `POST /api/v1/tenants/{tenantId}/pools`：创建地址池（提交包含 CIDR、VLAN、租约模板等的 JSON）。
	- 地址池负载支持 `allocationMode`（`SEQUENTIAL`、`ROUND_ROBIN`、`PRIORITY_WEIGHTED`）与 `priorityWeight`（1-100），可让 IPv4 分配在保持兼容旧池的前提下进行轮换或倾斜。
	- `POST /api/v1/tenants/{tenantId}/pools/find`：按元数据（scope、VLAN、interface、SSID、location）筛选并返回最精确的匹配。
	- `POST /api/v1/tenants/{tenantId}/pools/resolve`：基于中继元数据解析唯一最优池，并驱动 DHCP 策略选择器。每次成功调用都会写入 `pool.resolve` 审计记录（以及结构化 Zap 日志），包含 `poolSelector`、解析出的池快照以及 `pkg/auditpayload` 定义的元数据映射，方便 SIEM/审计系统统一处理。
	- `GET|POST|PUT|DELETE /api/v1/tenants/{tenantId}/policies`：管理条件分配策略（条件/动作以 JSON 提交），评估器会自动清缓存。
	- `GET /api/v1/tenants/{tenantId}/bindings`：列出静态绑定；`POST`/`DELETE` 维护保留条目。
	- `GET /api/v1/tenants/{tenantId}/leases?state=ACTIVE&limit=50`：实时查看租约库存。
	- `modern-dhcp leases list --tenant {tenantId} --state ACTIVE --api-key <token>`：CLI 中的同等视图，并提供闲置/即将过期提示。
	- `GET /api/v1/tenants/{tenantId}/prefix-leases?state=ACTIVE&limit=50`（管理员范围）：列出 IPv6 前缀委派，可附加 `state` 过滤，并支持标准的 `limit`/`offset` 分页。
	- `GET /api/v1/tenants/{tenantId}/audit?limit=100`：返回策略/池/绑定变更触发的最新审计事件（操作者来自 API Key 的显示名）。
	- `POST /api/v1/simulate`：用于实验室验证的策略 + 租约流水线模拟。
	以上端点提供最小化脚手架，旨在解锁自动化与 UI 对接；认证、RBAC 与更完善的分页会在后续里程碑中补齐。

## Web 前端（Vue 3）
- 位置：`web/ui`。
- 技术栈：Vue 3 + TypeScript、Vite、Element Plus、Pinia、Axios、WebSocket 客户端。
- 开发：`npm install && npm run dev`（默认代理 `/api` → `http://localhost:8080`，可通过 `VITE_API_PROXY_TARGET` 覆盖）。
- 构建：`npm run build`，产物位于 `web/ui/dist`，可由 Nginx 等静态服务器托管并通过网关层反向代理 API/WebSocket。

### 认证与指标

### 地址池选择器遥测
- 基于元数据的地址池解析（DHCP 求值器以及 `/pools/resolve`）会以 `modern_dhcp_pool_selector_*` 为前缀输出 Prometheus 数据。核心信号包括 `pool_selector_resolutions_total`（按 scope/VLAN/interface 组合统计）、`pool_selector_resolution_seconds`（直方图）、`pool_selector_errors_total`（按原因标记的失败次数），以及用于覆盖视图的 `pool_metadata_snapshot` 仪表。
- `docs/metadata_observability.md` 记录了经干系人确认的指标标签、采样节奏与看板期望，而 `docs/dashboards/pool_selector_overview.json` 则提供可直接导入的 Grafana 面板。
- 零漫游亲和：DHCPv4 处理器维护 `mobility_affinity_cache`，并通过 `policy.lifecycle.mobilityAffinityTTL` 控制失效时间，命中情况以 `dhcp_mobility_affinity_hit_total{tenant="",result="hit|miss"}` 暴露，便于看板跟踪实际收益。
- Fast ROAM 的定制 Option：当租约带有 Mobility 元数据时，DHCPv4 会在 OFFER/ACK 中写入 Option 125 子选项——`mobility-anchor-id`（文本）和 `session-continuity`（JSON，包含 anchor/controller/geo/pool/lease 片段），供 Wi-Fi 控制器或 SD-WAN API 直接消费。
- 检测到的租约冲突（DECLINE、重复地址报告等）会累计到 `modern_dhcp_lease_conflicts_total`，其标签包含 `tenant` 与 `signal`，方便 NOC 发现问题地址池或接口，同时每次事件都会写入 `lease.conflict_detected` 审计记录，携带租约、地址池、信号、原因以及累计冲突次数，供 SOC 回溯。

- 所有列表型端点（`/policies`、`/pools`、`/bindings`、`/leases`、`/audit`）均接受 `limit`（默认 50，上限 500）与 `offset` 参数。非法输入会返回 400，因此客户端需提供正整数并显式处理分页游标。

## 系统初始化流程
Modern-DHCP 在启动阶段必须严格遵循以下顺序，以确保依赖项、配置与服务稳定就绪：

1. **环境检查与依赖验证**
	- 检查操作系统权限、所需 CAP 能力以及运行用户。
	- 验证网络接口配置（NIC 绑定、VLAN、Anycast/VIP）。
	- 检查端口可用性：DHCP 67/68、API 8080、监控端口等。
	- 验证 Redis/PostgreSQL/其他存储连接可达性与凭据。
	- 检查配置文件与证书/密钥的完整性与权限。

2. **配置加载与解析**
	- 加载主配置（如 `configs/config.yaml`）。
	- 解析命令行参数并覆盖配置；若无显式值再读取环境变量，最终 fallback 到配置文件。
	- 注入环境变量（优先级：命令行 > 环境变量 > 配置文件）。
	- 执行配置验证与规范化（默认值补全、路径校验、单位换算）。

3. **存储系统初始化**
	- 建立 Redis 连接池（支持主从/集群模式）。
	- 建立 PostgreSQL/MySQL 连接并执行健康检查。
	- 根据需要触发数据库迁移或 schema 校验。
	- 初始化内置地址池、策略、租户模板。
	- 加载静态绑定、保留地址与其它离线配置。

4. **核心服务初始化**
	- 初始化 DHCP 协议栈（IPv4/IPv6）、报文缓冲与 hook。
	- 启动地址池管理器、前缀分配器。
	- 启动策略引擎及缓存。
	- 初始化高可用管理器（failover/coordinator/replicator）。
	- 初始化安全子系统（Guard、Snooping、Rate Limit 等）。

5. **网络服务启动**
	- 绑定 DHCP sockets（67/68）并开启监听。
	- 启动 Web 管理与 REST API（HTTP/HTTPS）。
	- 初始化 API 网关/反向代理（若启用）。
	- 启动监控指标采集器与 Prometheus exporter。
	- 启动健康检查/探针端点（liveness/readiness）。

6. **高可用选举与同步**
	- 加入协调器进行主从/多活选举，确定角色。
	- 执行租约、状态数据的主备同步。
	- 同步配置版本与运行期覆盖项。
	- 启用心跳与故障检测，记录监控指标。

7. **外部系统集成初始化**
	- 建立与网络设备（Cisco、华为等）的接口连接或 API session。
	- 初始化无线控制器（WLC）或 NAC 集成。
	- 建立 SDN/控制面北向接口。
	- 向监控/告警系统注册所需通道与 webhooks。
	- 加载插件系统或扩展模块。

8. **系统就绪状态**
	- 更新系统状态为 `Running`，写入状态存储或 etcd。
	- 触发 readiness 探针供编排系统（K8s 等）感知。
	- 发送启动完成通知（Webhook、Kafka、邮件等）。
	- 开放客户端请求通路并进入稳态运行。
### 前缀租约可视化
- 管理员可通过 `GET /api/v1/tenants/{tenantId}/prefix-leases` 查看 IPv6 前缀委派，该路由复用了分页辅助函数，并支持 `state`（`ACTIVE`、`RELEASED`、`DECLINED` 等）过滤；由于继承 `tenantAdminGroup` 中间件，因此必须使用 `admin` 角色的 API Key。
- CLI：`cmd/modern-dhcp` 提供 `modern-dhcp prefix-leases list --tenant <id> [--state ACTIVE] [--limit 100 --offset 0] --url http://manager:8080 --api-key <token>`，输出包含 prefix、IAPD、pool、client、expiresAt 的表格，同时标记即将过期或非 `ACTIVE` 的条目。
- `modern-dhcp pools find|resolve` 子命令帮助运维人员无需手写 JSON 即可调用新接口，例如 `modern-dhcp pools resolve --tenant tenant-1 --vlan 200 --interface Gi1/0/48` 会打印解析到的地址池 ID 及 scope 元数据。
- 策略作者可运行 `modern-dhcp policy selector build --vlan 200 --interface gi1/0/48 --wrap`，生成可直接写入策略动作体的 JSON 片段。
- 喜欢 GUI？访问管理节点上的 `/console/policy-selector`，即可打开与 CLI 保持一致的轻量级构建器，实时渲染 JSON，并可选调用 `/pools/resolve` 进行预览。
- 面向运维的 UI：浏览管理节点的 `/console/prefix-delegations`，即可使用同一套 API 的轻量控制台。输入租户 ID 与 API Key 后，可按 state/limit 过滤，并突出显示 “即将过期”（30 分钟内）及非活跃前缀。

### DHCPv4 并发调优

DHCPv4 UDP 监听器的并发度可以通过配置或 CLI 覆盖：

```yaml
service:
	dhcpv4:
		workerCount: 0      # 0=auto, 默认 runtime.NumCPU()
		queueDepth: 0       # 0=auto, 默认 workerCount*4
		listenerFanout: 0   # 0=auto, 默认按 CPU（上限 16）
		rxBatchSize: 4      # 单 listener 每次读取的报文数
		reusePort: true     # 仅 Linux/*BSD 支持，自动回退
```

- CLI 旗标：`--dhcpv4-workers`、`--dhcpv4-queue`、`--listeners`、`--rx-batch`、`--reuseport=true|false`。
- 配置优先，CLI 可在不改动配置文件的情况下按环境覆盖；传入 `0` 则保留自动推导。
- `--listeners>1` 时默认启用 `SO_REUSEPORT` 并开启 listener 分片；`--rx-batch` 控制批量读取以降低系统调用开销。

### 元数据可观测性计划
- `docs/metadata_observability.md` 给出了地址池元数据覆盖及选择器成功率的 Prometheus + Grafana 合同（指标名、标签、抓取周期）。在实现导出器前，可将此草案分享给可观测性团队，确保仪表板、日志、审计记录复用同一字段名。
- 租约生命周期指标（`modern_dhcp_lease_*`、`modern_dhcp_allocator_*`）的命名、标签策略详见 `docs/lease_lifecycle.md`，并可与 `docs/dashboards/pool_selector_overview.json` 扩展的 Grafana 面板联动。

### 跨域访问外部控制台
- 将 `service.cors.enabled` 设为 `true`，并在 `service.cors.allowedOrigins`（逗号分隔或 YAML 列表）中填写外部控制台/SPA 所在域名，即可为管理 API 启用 CORS。
- 启用后，HTTP 服务器会尊重配置的来源，并在允许列表中包含 `Authorization` 与 `X-API-Key`，这样内嵌的 `/console/prefix-delegations` 页面或任意自定义 UI 都能在其他域访问。
- 单域部署可保持默认禁用（或留空）以维持当前的锁定行为。

### CLI 写操作
- `POST /api/v1/tenants/{tenantId}/leases/{leaseId}/release`（以及 `/decline`）允许管理员立即释放或隔离地址；该路由要求 `admin` 角色 API Key，并记录携带可选 `reason` 字段的审计事件。
- `modern-dhcp leases release --tenant {tenantId} --lease {leaseId} [--reason ...] --api-key <token>` 及其 `decline` 变体封装了上述端点，方便自动化工作流，并输出最新状态供脚本分支。
- 可通过 `POST /api/v1/tenants/{tenantId}/leases/{leaseId}/cooldown/clear` 或 `modern-dhcp leases cooldown-clear --tenant {tenantId} --lease {leaseId}` 清理冷却/隔离积压，将租约重置为 `RELEASED` 并写入审计事件。
- 前缀委派同样支持 `POST /api/v1/tenants/{tenantId}/prefix-leases/{prefixLeaseId}/release|decline` 以及 CLI `modern-dhcp prefix-leases release|decline --tenant <id> --id <prefixLeaseId>`。
- 所有写操作共享新的 JSON 辅助函数，会自动附带 API/Bearer 凭据，并原样返回非 2xx 响应，便于排障。

### 策略架构校验
- 通过 REST API 提交的策略条件/动作会根据 `docs/policy_conditions_schema.json` 与 `docs/policy_actions_schema.json`（并已镜像到 `internal/policy/schema` 便于嵌入）进行校验；未通过者会在进入求值器之前被拒，并返回详细错误。
- `poolSelector`（VLAN/interface/SSID/location）现已纳入策略动作架构，当静态 `poolId` 不合适时，可请求基于元数据的解析。具体载荷可参考 `docs/api.md`。
- BYOD 策略如今可直接在条件体中使用 `devicePersona`/`deviceTags` 字段，并通过布尔 `mdmManaged` 标识筛选受管终端。所有新字段已同步至 `docs/policy_conditions_schema.json` 及嵌入式校验器，提交时即可获得语义校验；若需编辑此类规则，请为用户授予 `byod.policy.manage` 能力。

### 策略元数据映射
`internal/policy/metadata.go` 维护了 DHCP Option 与策略元数据键的一致映射，方便策略作者与网络控制器保持字段对齐。常用映射如下：

| DHCP Option | 写入的元数据键 | 用途 |
| --- | --- | --- |
| Option 60 (DHCPv4) / Option 16 (DHCPv6) Vendor Class | `vendorClass`、`option60`、`deviceType` | 直接用于 `vendorClass` 条件匹配，并驱动 `ClassifyDeviceType` 推断终端类别。 |
| Option 77 (DHCPv4) / Option 15 (DHCPv6) User Class | `userClass`、`userGroups` | `userClass` 可精确匹配；`DeriveUserGroups` 会将其与中继 hint 组合成逻辑标签集合。 |
| Option 82 sub-option 1 / DHCPv6 Option 18 Interface-ID | `circuit-id`、`agent.circuit-id`、`port-id`、`interface-id` | 标识接入口，既可作为 Guard 输入，也能为元数据选择器提供接口/端口范围。 |
| Option 82 sub-option 2 / DHCPv6 Option 37 Remote-ID | `remote-id`、`agent.remote-id`、`location`、`site`、`room`、`zone`、`building` | 将中继/控制器提供的站点或机房信息标准化，供 `location` 条件与池选择器使用。 |
| Option 82 vendor SSID/BSSID 扩展 | `ssid`、`wifi-ssid`、`wireless-ssid`、`essid`、`ap-ssid` | 将 AP/控制器暴露的无线标识映射到 `input.SSID`，以便编写 SSID/BSSID 定向策略。 |
| Option 82 VLAN 或控制器拓展字段 | `vlan-id`、`agent.vlan-id` | 统一 VLAN 元数据，既可与策略条件匹配，也为 `poolSelector` 注入 Layer-2 作用域。 |

### 租约生命周期与回收
- `docs/lease_lifecycle.md` 描述了各租约状态（`ACTIVE`、`IDLE_PENDING`、`GRACEFUL_RECLAIM` 等）的状态机、自动回收策略以及冷却/冲突规避流程。
- 管理 API/CLI 和观察性指标正在按阶段集成：请参阅该文档的里程碑表获取最新进展与配置片段。
- 如需规划主从/多活部署与故障转移，请结合 `docs/ha_architecture.md` 中的阶段性路线图实施。

### 策略变更事件
- Modern-DHCP 可实时向下游 DHCP 节点广播策略变更。通过 `kafka.topics.policyEvents` 与 brokers 列表配置 Kafka 主题，或使用 `events.policyWebhooks` 发布 webhook，亦可同时启用。每次创建/更新/删除都会发送结构化载荷（`action`、`tenantId`、`ruleId`、序列化规则体、时间戳），让远端节点无需轮询 REST API 也能刷新缓存。

## 13. 智能告警系统
Modern-DHCP 的告警层基于监控聚合器、策略引擎与外部通知适配器协同工作：`internal/monitoring` 负责产出池利用率、请求延迟、系统健康等指标，告警管理器根据阈值/策略生成事件，并通过多渠道发送。所有告警配置均由 `monitoring.alerting` 节点驱动，可在 `configs/config.yaml` 为每个租户或全局定义阈值、收件人以及静默时间。

### 13.1 分级告警
| 级别 | 触发示例 | 默认路径 |
| --- | --- | --- |
| **紧急 (Critical)** | 管理 API 无法访问、关键节点宕机、地址池容量=0、DHCP 服务线程崩溃 | 电话+SMS 轮询通知、紧急告警群 (Slack/Teams)、PagerDuty/Call Tree |
| **重要 (Major)** | 单个地址池利用率 ≥ 90%、Guard 检测到攻击/伪装、配置校验失败导致策略失效 | 邮件、即时通信群、Webhook (CMDB/ITSM) |
| **警告 (Minor/Warning)** | 地址池利用率 ≥ 80%、请求超时/失败率攀升、CPU/内存持续高于阈值 | Slack/Teams/钉钉、Syslog、SNMP Trap |
| **信息 (Info)** | 新设备上线、租约批量到期、策略/配置变更、HA 角色切换 | Webhook 回调、操作日志、日常广播频道 |

每条告警都包含租户、来源、触发阈值、关联资源（池/租约/节点）以及推荐操作。分级策略支持：
- **继承**：平台提供默认策略，租户可覆盖阈值或禁用某些通道。
- **抑制/合并**：对同类事件在静默窗口内自动合并，避免风暴。
- **自动升级**：在指定时间内未确认的 `Major` 将升级为 `Critical` 并走电话链路。

### 13.2 多渠道通知
告警分发采用插件式适配器，支持并发推送至多个渠道：
- **邮件 / SMS / 语音电话**：通过 SMTP 及第三方短信、语音网关触达值班人员，支持模板化内容与本地化语言。
- **聊天协作 (Slack、Teams、钉钉、企业微信)**：配置 Bot Token 或 Webhook URL 后，即可将告警推送至团队频道，并附带一键确认/静默按钮。
- **SNMP Trap**：向 NOC/NMS 发送 v2/v3 Trap，OID 映射在 `docs/alerting/snmp_mib.md`，方便集成现有网管视图。
- **Webhook 回调**：可将 JSON 事件投递到 ITSM、工单或自动化平台；失败会重试并记录状态。
- **Syslog 转发**：以 RFC 5424/3164 格式输出到集中日志平台，便于统一审计与 SOC 关联分析。

所有通道都可配置重试、超时、速率限制与 HMAC 签名。后续里程碑中会补充 UI/CLI 用于管理收件人、值班表与静默窗口，并提供 Grafana/Prometheus Alertmanager 的桥接器以与现有监控体系互通。

### 13.3 运行时配置
`cmd/dhcpd` 在启动阶段会调用内部的 `setupAlerting`，将 `monitoring.NewAggregator`、`internal/alerting.Manager` 以及新增加的 `AlertController` 绑定到同一个 runtime context。`configs/config.example.yaml` 提供了完整的 `monitoring.alerting` 示例，可直接复制并按租户自定义阈值、静默窗口与通道。

```yaml
monitoring:
	alerting:
		enabled: true
		evaluateInterval: 30s          # AlertController 周期性读取聚合数据的频率
		dedupeWindow: 2m               # 相同资源的重复告警抑制时间
		escalationWindow: 10m          # 预留给电话链路升级的窗口
		routes:
			- name: critical-default
				severities: [CRITICAL, MAJOR]
				channels: [noc-email, ops-chat]
		notifiers:
			email:
				- name: noc-email
					from: dhcp-alerts@example.com
					recipients: [noc@example.com]
			chat:
				- name: ops-chat
					channel: "#dhcp-alerts"
					webhook: https://hooks.slack.example.com/services/T000/B000/KEY
		defaultPolicy:
			silenceWindow: 5m
			poolThresholds:
				warning: 80
				major: 90
				critical: 95
			latency:
				major: 200
				critical: 500
			errorRate:
				warning: 2
				major: 5
				critical: 10
		policies:
			tenant-acme:
				poolSampleSize: 25
				poolThresholds:
					warning: 70
					major: 85
					critical: 92
```

控制面会按照上述配置注册邮件、聊天、SNMP、Syslog 等通知器，然后根据路由表将不同等级的告警 fan-out 至对应通道。每个租户均可在 `policies` 字典中覆盖阈值或开关，AlertController 会自动合并默认值并尊重静默窗口，避免震荡。

## 14. 审计与合规
Modern-DHCP 通过 `internal/audit`、`pkg/auditpayload` 以及数据库中的审计表维持全链路记录，并提供面向稽核/合规的报告与数据保留策略。

### 14.1 完整审计日志
- **租约分配 / 释放**：`lease.allocate`、`lease.release`、`lease.cooldown_clear` 等事件会记录租户、池、MAC、IP、时间戳、调用来源（API/CLI/guard），以及触发该操作的 API Key 显示名。
- **配置变更**：地址池、策略、ACL、告警路由等写操作都会生成 `config.change` 事件，包含变更前后 JSON diff、提交人、审批链（若启用外部工单）。审计服务支持压缩存储 diff，并可将快照同步到对象存储。
- **安全事件**：Guard、Snooping、Exhaustion 等子系统会以 `security.alert` 或 `security.violation` 推送事件，载荷含信号类型（DoS、伪装、Rogue DHCP 等）、置信度、处置动作以及关联的接口/端口。
- **管理员操作**：登录、令牌校验、RBAC 决策、敏感查询或写操作全部写入 `admin.activity`，字段包括 IP、User-Agent、API Key、角色、请求路径、结果码，满足等保/PCI 对管理员行为留痕的要求。
- 所有事件都带有全局唯一的 `auditId` 与 `correlationId`，便于跨系统追踪；默认保存在 MySQL，并可通过 Kafka 推送到 SIEM。

### 14.2 合规报告
- **月度地址使用报告**：监控聚合器与审计事件生成每月租约使用快照，可按租户、部门（基于标签）、VLAN/位置统计分配率与空闲量，导出为 CSV/PDF 供财务和资产管理。
- **安全合规报告**：整合 Guard 告警、登录审计与补丁状态，输出 PCI-DSS、HIPAA、国密等模版化章节（帐号管理、访问控制、日志留存、异常响应），并附带原始事件引用。
- **容量规划报告**：利用 `internal/monitoring` 的历史数据产生趋势线（90/95/99 百分位利用率、租约峰值、请求时延），结合业务增长预测给出未来季度所需地址池/节点容量。
- **审计追踪报告**：针对任意资源（租约、策略、配置）可以生成时间线式报告，列出所有相关审计记录、操作者与结果，满足外部审计抽查要求；CLI/REST 提供导出 JSON/CSV 接口。

### 14.3 数据保留策略
- **租约历史**：生产库保留最近 12 个月的完整租约与事件，超过 12 个月的数据自动写入对象存储或 HDFS，并保留 7 年可检索归档，支持按租户、IP、MAC 查询。
- **审计日志保留**：默认遵循合规要求（PCI ≥ 1 年在线、等保 ≥ 6 个月），并可在配置中为不同租户/区域设定更长周期；在线数据按月分表，归档时自动压缩（ZSTD）并附带索引元数据。
- **配置变更历史**：所有配置快照写入 `config_snapshots` 表并推送至 Git/S3，支持一键回滚；保留策略与审计日志一致，确保在 7 年窗口内可追溯。
- **压缩归档**：平台提供周期性任务打包旧数据（租约、审计、配置 diff），采用 `tar+zstd` 或对象存储生命周期策略，支持校验哈希与加密（KMS/SM4），方便离线存储或提交监管机构。

## 后续规划
- 完善 DHCP 报文处理管线，并接入原始套接字或 AF_PACKET。
- 实现 Redis 缓存、Kafka 事件总线与 HA 协调层。
- 构建 CI/CD 工作流、容器镜像，以及 Terraform/Ansible 集成。

完整需求覆盖请参阅 `docs/system_spec.md`。
