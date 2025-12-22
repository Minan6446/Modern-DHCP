# Modern-DHCP 企业级规范

## 1. 执行摘要
Modern-DHCP 是采用 Go 编写、以 MySQL 为权威数据存储的企业级双栈 DHCP 平台。其目标客户为需要确定性地址管理、多租户隔离和自动化友好 API 的大型多站点网络。系统兼容 RFC 2131/2132、RFC 3315、RFC 3633，并支持高级策略执行、HA/DR 拓扑以及深度生态集成。

## 2. 高层架构
- **控制平面**：REST/gRPC 管理 API、策略引擎、配置服务、HA 协调器。
- **数据平面**：DHCPv4/v6 报文处理器、感知中继的选择器、租约生命周期服务、安全防护。
- **持久化层**：MySQL 8 主存储（InnoDB）、Redis 集群用于租约热缓存、Kafka 总线承载异步流程、对象存储用于归档。
- **可观测性栈**：Prometheus 导出器、Grafana 仪表板、ELK 日志分析、告警路由服务。
- **集成层**：Webhooks、Terraform/Ansible Provider、IPAM/ITSM 连接器、插件沙箱（Lua/Python 钩子）。

## 3. 组件职责
1. **DHCP 核心（Go）**
   - 实现 DHCPv4/v6 状态机、兼容 BOOTP、支持前缀委派。
   - 支持多层地址池选择（global → subnet → VLAN → interface → port/SSID/location）。
2. **租约服务**
   - 负责租约 CRUD、续约定时器、T1/T2 重算、冷却/隔离处理。
   - 提供批量导入导出（CSV/Excel），保留 1 年在线历史、7 年归档。
3. **策略引擎**
   - 评估设备/用户/位置/时间属性（Option 60/77/82/93、OUI、RADIUS、AP 元数据）。
   - 决策策略：池选择、租期、QoS 标签、安全姿态。
4. **安全防护**
   - 集成 DHCP snooping、按 MAC/IP/端口限速、检测 Rogue 服务器、缓解 DoS/饥饿/中继伪装。
   - 与 NAC、SIEM、Firewall 协同。
5. **HA 与复制**
   - 兼容 ISC DHCP failover，多活集群基于确定性分片。
   - 心跳 <1s，自动故障切换 <3s，可选手动/自动回切。
6. **自动化与 UI**
   - 基于 Go 网关的 React/Vue SPA，多主题、多语言、支持 RBAC。
   - Terraform/Ansible/Python/PowerShell SDK 基于 OpenAPI 3.0。

## 4. 功能需求映射
| 领域 | 关键能力 |
| --- | --- |
| 地址分配 | IPv4 动态/静态、A/B/C 类、BOOTP、IPv6 有状态/无状态、前缀委派、可配置租约模板、续租提醒 |
| 地址池管理 | 分层池、CIDR + 传统掩码、继承/覆盖规则、静态/动态排除、保留百分比、VLAN/端口/SSID/位置绑定 |
| 静态绑定 | 按 MAC/client-id/用户身份/序列号绑定，支持 CSV/Excel + REST 批量导入导出、模板与脚本钩子 |
| 策略引擎 | 感知设备/用户/位置/时间，支持 Option 60/77/82/93、OUI 映射、AP 元数据、中继感知规则、Vendor/User Class、分类缓存 |
| 租约生命周期 | 闲置检测、平滑/强制回收、优先级层级、冷却、冲突历史规避、轮换与偏好排序 |
| 高可用 | 故障切换协议、租约/配置同步、脑裂检测、多站点集群、心跳 + 自动恢复、对客户端透明的迁移 |
| 性能 | 10 万+ 并发客户端、1 万 TPS、P99 <50 ms（目标 10 ms）、线程安全管线、零拷贝解析、优先级队列、UDP 广播/单播优化 |
| 安全 | Snooping 信任域、限速、攻击检测/缓解、Rogue 服务器发现、Option 82 防护、MAC 伪装检测、DAI/IPSG 集成、RADIUS + 802.1X |
| 中继与代理 | Option 82 解析、giaddr 池选择、中继负载均衡、中继认证、跨 VLAN/L3/DC/MPLS 分发 |
| 监控 | 实时看板、DISCOVER/OFFER/REQUEST/ACK 指标、热力图、健康遥测（CPU/内存/磁盘/网络）、诊断（pcap、历史、冲突追踪） |
| 告警 | 分级告警、渠道（Email/SMS/语音/Slack/Teams/钉钉/微信/Webhook/SNMP Trap/Syslog）、可配置阈值、模板化通知 |
| 审计与合规 | 租约/配置/安全/管理操作的不可变日志、差异追踪、合规报告（PCI/HIPAA/等保）、归档与压缩策略 |
| API 与自动化 | REST（OpenAPI 3.0）、OAuth2/JWT、限速/配额、Terraform/Ansible/Python/PowerShell/Go/Java SDK、Webhook 事件、脚本钩子 + 插件框架 |
| 部署 | 裸机、VM、Docker、Kubernetes、公有云、混合/边缘；自动扩缩、跨地域分布、多存储后端、备份/恢复流程 |
| 体验与多租户 | 自适应 UI、主题、多语言、可访问性、拖拽式地址池构建器、拓扑图、3D 数据中心视图、协作、审批、细粒度 RBAC、租户隔离 |
| 特殊场景 | IoT 优化、工业协议感知、行为分析、漫游/移动连续性、云/虚拟化集成、IPAM/ITSM/SIEM 连接器 |
| 指标与容量 | 10 万+ 地址池、100 万+ 租约、99.99% 可用性、线性扩展至 100 节点、存储/内存/带宽容量指导 |

## 5. 数据模型（MySQL）
- **tenants**：多租户隔离、品牌、配额。
- **dhcp_servers**：节点清单、心跳、角色、集群成员。
- **address_pools**：分层池元数据、继承指针、排除范围、保留策略、分配模式/权重。
- **leases_v4 / leases_v6**：活跃租约、历史状态、冷却标识、冲突元数据。
- **static_bindings**：以 MAC/client-id/用户/序列号为键的保留记录（含导入任务引用）。
- **policies**：条件树（JSON）、优先级、目标动作（租约模板、池、VLAN、QoS）。
- **security_profiles**：限速、信任状态、检测阈值。
- **events & audits**：不可变日志（按日期分区），关联租约/配置/用户。
- **webhooks**：订阅端点、重试/退避状态。
- **backups**：快照目录、位置、校验和、保留计划。

所有表均包含 created_at/updated_at、版本列（用于乐观锁）以及按需软删除以满足审计留存。

## 6. API 接口
1. **管理 REST API**
   - 认证：OAuth2（client credentials + auth code）签发 JWT，支持 LDAP/SAML/ADFS。
   - 命名空间：`/api/v1`（稳定）、`/api/v2`（高级功能），通过 Header 协商版本。
   - 能力：地址池 CRUD、租约查询、策略管理、静态绑定导入导出、HA 控制、集群状态、遥测端点、Webhook 管理。
2. **gRPC 控制平面**
   - DHCP 节点用于配置同步、故障切换协同、流式遥测。
   - Proto 定义编译成 Go/Python/Java SDK。
3. **Terraform/Ansible/Python SDK**
   - 基于 OpenAPI/gRPC IDL 生成，附带示例 Playbook/模块与 CI 友好流水线。
4. **钩子/插件接口**
   - gRPC Sidecar 以及嵌入式 Lua/Python 沙箱，用于前/后/释放钩子、自定义分配策略、第三方集成。

## 7. 高可用与扩展
- **故障切换对**：支持双活或主备，通过 gRPC 流 + binlog 复制租约状态，脑裂防护依赖 etcd/Zookeeper 锁与仲裁。
- **集群**：最多 100 个跨站点节点，按 (tenant, pool, vlan, relay-id) 哈希分片，可对每个池选择强一致或最终一致。
- **自动扩缩**：Kubernetes 使用 HPA 依据 TPS、队列深度、CPU 扩容；裸机环境依靠代理遥测申请额外 VM。

## 8. 安全与合规
- **身份**：角色制 RBAC（super-admin、net-admin、operator、auditor、read-only），支持带过期时间的临时提权，敏感操作需审批流程。
- **网络安全**：DHCP snooping 信任配置、按端口/模式限速、DAI/IPSG 集成、MAC 漂移检测、Rogue 服务器隔离。
- **加密**：全链路 TLS 1.3，节点间 mTLS，证书轮换自动化，机密托管在安全 vault。
- **审计**：追加式日志、WORM 存储选项，可导出至 SIEM（Splunk、ELK、QRadar），提供合规模板（PCI-DSS、HIPAA、等保）。

## 9. 监控与告警
- **指标**：Prometheus 导出器覆盖 DHCP 报文计数、延迟直方图、地址池利用率、租约 churn、DB/Redis/Kafka 指标、系统健康（CPU/内存/磁盘/网络）。
- **看板**：Grafana 面向运维、容量、安全、地理热力、SLA 视图。
- **诊断**：内置抓包（pcap 导出）、客户端模拟器、冲突检测、端到端租约生命周期 Trace ID。
- **告警路由**：与 Alertmanager 集成，支持 Email/SMS/语音、Slack/Teams/钉钉/微信、SNMP Trap、Syslog、Webhooks。

## 10. 部署、备份与容灾
- **目标环境**：裸机、VMware/Hyper-V/KVM、Docker、Kubernetes、公有云（AWS/Azure/阿里云），支持混合/边缘形态。
- **打包**：Helm Chart、Terraform Module、Ansible Playbook、Docker 镜像。
- **备份**：MySQL 全量 + 增量（mysqldump + Percona XtraBackup）、Redis 快照、Kafka 主题保留策略、跨站复制。
- **恢复**：UI/API 一键恢复、自动化校验任务、定期 DR 演练。

## 11. 用户体验与协作
- 自适应 SPA，提供暗/亮主题，本地化（zh-CN/en-US/ja-JP），符合 WCAG AA。
- 可视化工具：拖拽式池构建器、自动生成拓扑、租约地理地图、3D 数据中心视图。
- 协作：乐观锁的并发编辑、内联评论、变更审批流程、任务分发、通知。

## 12. 路线图与阶段划分
1. **Phase 1 (MVP)**：DHCPv4/v6 核心、MySQL 持久化、基础池层级、静态绑定、REST API、Prometheus 指标、RBAC、HA 双机。
2. **Phase 2**：高级策略引擎、安全防护、监控仪表板、Webhook 生态、Terraform/Ansible 模块、IoT/移动优化。
3. **Phase 3**：多活地理集群、插件市场、与 ITSM/IPAM/SIEM 的深度集成、3D UI、工业协议适配器。

## 13. 实现快照（当前仓库）
- 配置加载器、MySQL 连接器及租约/策略服务均已连接到 REST 模拟端点。
- 地址池与静态绑定仓储/服务实现了 CIDR 与保留策略校验。
- 管理 API 端点：
   - `GET/POST/PUT/DELETE /api/v1/tenants/{tenantId}/pools`，以及 `POST /api/v1/tenants/{tenantId}/pools/find`、`POST /api/v1/tenants/{tenantId}/pools/resolve` 元数据辅助接口
   - `GET/POST/PUT/DELETE /api/v1/tenants/{tenantId}/policies`
   - `GET/POST/DELETE /api/v1/tenants/{tenantId}/bindings`
   - `GET /api/v1/tenants/{tenantId}/leases`
      - `POST /api/v1/tenants/{tenantId}/leases/{leaseId}/cooldown/clear`
   - `POST /api/v1/simulate`（策略/租约演练）
- API 加强：
   - 静态 API Key 认证保护所有 `/api/v1/**` 路由（通过 `auth.apiKeys` 配置）。
   - Prometheus 仪表（请求计数、策略命中、租约生命周期事件）在启用时暴露于 `/metrics`。
- 迁移 `0001` 已创建 tenants、pools、leases（v4/v6）、bindings、policies、audit 等表。
- Go 模块脚手架包含 Echo HTTP 服务器、Zap 日志、sqlx 数据访问。

本规范为实现 Modern-DHCP 平台提供了基线，后续用户故事应针对各功能簇细化验收标准。

## 17. 多样化部署与弹性伸缩
- **交付形态**：覆盖裸金属（iPXE + Ansible + Redfish）、虚拟化（VMware/KVM/Hyper-V 模板 + cloud-init）、Docker 单机（Compose/Host 网络）、Kubernetes（Helm + HPA + GitOps）、公有云（AWS/Azure/阿里云托管数据库/缓存/负载均衡）与边缘/混合云场景，详见 `docs/deployment_strategy.md`。
- **配置入口**：`deployment.*`、`deployment.scaling.*`、`deployment.geo.*`、`deployment.hybrid.*` 字段用于声明部署模式、扩缩阈值、资源区间、地域/边缘/混合云拓扑；示例配置在 `configs/config.example.yaml`。
- **部署工件**：`deploy/docker/docker-compose.yaml` 提供 PoC/边缘快速启动，`deploy/kubernetes/values.yaml` 供 Helm/ArgoCD 复用，未来将扩展 Terraform/Ansible 模块。
- **弹性扩展**：通过 Prometheus Adapter + HPA 或裸机/VM 扩缩控制器，依据 `dhcp_lease_tps`、CPU、队列深度自动扩缩；Resource Adjustment 可按需提升 vCPU/RAM。
- **地理与混合云**：`deployment.geo.regions` 描述 Active/Standby/Edge 布局与 `latencyBudget`，HA 协调器按区域进行仲裁；`deployment.hybrid` 统一 VPN Mesh/Transit/流量策略，支持 prefer-onprem/cloud-first 等模式。

## 18. 存储与备份
- **多后端矩阵**：`storage.relational` 记录 MySQL/PostgreSQL/RDS 拓扑与主备顺序，`storage.nosql` `storage.distributed` `storage.cloud` 描述 Redis/Mongo、etcd/Consul/ZK 以及对象存储目标，便于环境切换与 IaC 自动化。
- **备份策略**：`backup.schedule/fullInterval/incrementalInterval` 统一定义全量+增量、`retentionDays` 默认保留 30 天，`crossRegion` 负责异地复制，`verification` 周期性在沙箱执行恢复演练。
- **一键恢复**：`backup.restoreUi` 钩住图形化恢复门户（SSO + 审计），支持从 Catalog 选择时间点并自动执行验证/切换。
- **文档与工件**：详见 `docs/storage_backup_plan.md`；未来 CLI/API 会读取上述配置生成 Backup Catalog 以及 Terraform/Ansible 模块。

## 19. 现代化 Web 界面
- **响应式体验**：单一 SPA 适配 PC/平板/手机，布局系统遵循 CSS Grid/Flexbox，窗口事件驱动图表自适应；基于系统/浏览器首选项自动切换暗/亮主题，可在租户级覆盖。
- **全局化与可访问性**：内置 zh-CN/en-US/ja-JP 语言包，动态加载翻译词典，遵循 WCAG 2.1 AA（键盘导航、ARIA、对比度、可配置字体大小、屏幕阅读器标签）。
- **可视化操作套件**：提供拖拽式地址池设计器、拓扑构建器（自动发现 + 手工节点）、租约地理热力图、3D 机房/机架视图，依赖 WebGL/Three.js 与实时遥测接口。
- **协作工作流**：实现租户范围的乐观锁并发编辑、评论/批注线程、变更审批（多级门槛 + 审批模板）、任务/待办分派并与审计事件联动。
- **参考设计**：详见 `docs/web_ui_modernization.md`，定义前端技术栈、API 需求、数据契约以及里程碑分解。

## 22. 移动场景优化
移动终端在跨 AP、跨子网漫游时需要低时延地址切换、会话连续性以及策略差异化。本节定义租约、策略、集成能力的新增范围，指导控制平面与数据平面协同演进。

### 22.1 漫游设备支持
1. **跨子网租约保持（快速漫游）**
   - 在租约仓储中引入 *Mobility Anchor* 概念：同一设备在多个 L3 子网中的活跃租约共享一个锚 ID，允许 DHCP 服务器在收到新的 DISCOVER/REQUEST 时直接复用上一子网的 IP 或在同一地址池族内切换。
   - 服务器在 OFFER 阶段附带 `mobility-anchor-id` Option（自定义 125 子选项），以便终端/控制器缓存并在下一跳请求时回传，减少数据库查询。
   - 为 failover 复制、Kafka 事件增加 “mobility_change” 类型，保证伙伴节点提前同步当前锚状态，目标是 <50ms 的决策延迟。

2. **位置感知分配（最近网关返回）**
   - 扩展 pool.MetadataSelector，接受 `apId`、`controllerId`、`geoZone` 等字段；调度器根据最新的 Wi-Fi 控制器/交换机场景映射确定最近网关所属地址池。
   - 新增缓存：`mobility_affinity_cache`（Redis），按 `(tenantId, deviceId)` 记录最近一次成功租约的网关坐标，命中后直接指向对应池，失效时间 30s，可被策略引擎覆盖。
   - 配置文件 `policy.lifecycle.mobilityAffinityTTL` 控制缓存时间，并暴露观察指标 `dhcp_mobility_affinity_hit_total`。

3. **会话连续性保证**
   - DHCP 服务在检测到漫游需求时，会在 ACK 中回传 `session-continuity` 元数据（JSON，含 anchor、租期、粘滞池等），供上层 SD-WAN/5G UPF API 消费。
   - 引入 “漫游保护窗口” 参数：在 `lease_profiles` 中追加 `mobility_grace_period`，在该窗口内的释放请求将转为 COOLDOWN 状态但保留会话上下文，以便终端短期断连后无需重新认证。
   - 通过 Webhook 推送 `mobility.session` 事件至会话控制器（SASE、SD-WAN、WLC），使其更新流表或 QoS 策略。

4. **移动 IP 支持**
   - 支持与 Proxy Mobile IP（PMIPv6）或企业 SD-WAN 控制器协作：当策略定义 `mobileIp:true` 时，DHCP 服务器会分配一个 “归属地址” 与 “会话地址” 对，归属地址持久化在租约表中，会话地址根据当前接入点从临时池获取。
   - 在 `leases_v6` 中记录 `home_address`、`care_of_address` 字段，并在审计事件中输出两者的映射关系，方便链路追踪。
   - 需要在 HA 复制与导出中包含新的字段，确保热切换后仍能恢复相同的移动 IP 状态。

### 22.2 移动设备识别
1. **iOS/Android 特征识别**
   - 策略引擎扩展 OUI/OUI36、Option 55、Option 60/77 指纹库，结合 DHCP Option 161/162（移动设备信息）以及自研 UA 指纹，判断终端是否为 iOS、Android、HarmonyOS 等。
   - 新增 `mobileProfiles` 配置块，定义特征 → 标签映射（如 `ios.17`, `android.enterprise`），供策略/可视化直接引用。

2. **移动设备策略（BYOD）**
   - 在 Policy DSL 中加入 `device.persona == "BYOD"`、`device.mdmManaged == true` 等条件（具体字段为 `devicePersona`、`deviceTags`、`mdmManaged`）；动作可下发专用池、缩短租期、强制安全模板。
   - RBAC 增加 `byod.policy.manage` 能力，以限制谁可以修改 BYOD 相关规则。
   - 审计事件需记录策略命中时的终端类型、用户身份、漫游状态，便于合规追溯。

3. **移动设备管理（MDM）集成**
   - 引入 MDM 连接器框架（初期支持 Intune、Jamf、AirWatch），通过 OAuth2 Client Credentials 拉取设备合规信息，将 `complianceState` 回写至租约扩展字段。
   - 当 MDM 报告设备不合规时，DHCP 可触发策略将终端迁移至隔离 VLAN，或在 `security_state` 中标记 `SUSPECT`。
   - 连接器需支持增量同步、退避重试、以及在 `monitoring/alerting` 中暴露健康指标。

4. **推送通知集成（租期提醒）**
   - 结合租约通知调度器，在检测到移动设备且其租期即将到期（如 T1 内 10%）时，通过 Kafka 事件驱动通知服务向 APNs/FCM/企业 IM 发送提醒。
   - 消息体包含租期结束时间、推荐操作（如重新开启 Wi-Fi）、租户自定义文案，并支持 i18n。
   - 失败告警通过 Alerting 子系统的 `ops-chat` 路由上报，确保 BYOD 支持团队可见。
