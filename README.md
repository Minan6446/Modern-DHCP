# Modern-DHCP

Modern-DHCP 是一个以 Go 编写、面向多租户和 IPv4/IPv6 双栈的企业级 DHCP 控制平面。工程提供后端服务、数据库迁移、自动化/观测模块与 React 管理前端，可用于构建高可用、可审计、具备自动化能力的 DHCP 平台。

## 核心特性
- 双栈引擎：RFC 2131/3315 基线，独立的 DHCPv4/v6 管线与可插拔策略钩子。
- 多租户与 RBAC：租户上下文、最小权限控制、全链路审计。
- 分层地址池：Global → Subnet → VLAN → Port/SSID → Location，支持权重/轮询与元数据解析。
- 高可用与灾备：Failover 管理、角色/复制延迟可观测、支持手动/自动回切。
- 观测与告警：Prometheus 指标、聚合快照、分级告警与通知路由。
- 自动化与审批：任务调度、审批流、幂等重试，可触发外部集成。
- 前端控制台：React Admin 实时监控与池/租约管理。

## 代码结构速览
```
cmd/              # 后端入口：dhcpd、modern-dhcp CLI、dbinit 迁移工具、config-check 校验工具
internal/         # 业务与基础设施模块（pool/lease/automation/monitoring/security/...）
pkg/              # 共享模型与遥测定义
configs/          # 配置示例与运行配置
deploy/           # Docker Compose 与 Helm values 模板
docs/             # 架构、运维、观测、安全与 API 文档
migrations/       # 数据库迁移脚本
web/react-admin/  # 管理前端
```

## 快速开始（开发）
1) 环境：Go 1.22+、MySQL 8.0+、Node 18+（如需前端）。
2) 配置：复制 `configs/config.example.yaml` 为 `configs/config.yaml` 并填好 DSN/租户/HA/告警。
3) 迁移：
```
go run ./cmd/dbinit --driver mysql --dsn "user:pass@tcp(127.0.0.1:3306)/modern_dhcp"
```
4) 启动后端：`go run ./cmd/dhcpd`
5) CLI 验证：`go run ./cmd/modern-dhcp --help`
6) 配置校验：`go run ./cmd/config-check --config configs/config.yaml --scope auth`

## 生产部署指引
- Docker Compose：`deploy/docker/docker-compose.yaml` 适合 PoC 或单节点。
- Kubernetes：使用 `deploy/kubernetes/values.yaml` 配置副本、资源与存储；建议以 Job/InitContainer 运行迁移。
- 配置与机密：优先使用环境变量或外部 Secret；参见 docs/config_reference.md。
- HA：通过 `/cluster/overview` 暴露角色与复制延迟；详细见 docs/ha_architecture.md。

## 运行与运维
- 健康检查：`/cluster/overview`（角色、延迟、抑制）、`/monitoring/overview`（池/租约/安全概览）。
- 指标：默认暴露 Prometheus `/metrics`；仪表盘规划见 docs/observability_dashboard_plan.md。
- 日志与审计：结构化输出；关键变更、HA 切换、审批与告警抑制写入审计。
- 备份与恢复：MySQL 定期全量/增量备份并校验，配置文件版本化；细节见 docs/deployment_strategy.md。

## 安全与合规
- 多租户隔离与 RBAC，敏感操作可绑定审批；参考 docs/section20_access_and_tenancy.md。
- Rate Limit、Snooping、IPSG 等安全事件的路由与封禁策略见 docs/security_subsystem_plan.md。
- 常见事件响应步骤见 docs/threat_response.md。

## 自动化与工作流
- 调度、审批、通知与幂等策略描述见 docs/automation_workflows_plan.md。
- CLI/任务示例可按租户发起池扩容、租约批量释放等操作。

## API 与前端
- API 概览与 OpenAPI 草稿：见 docs/api.md 与 docs/api/openapi_v2.yaml。
- 前端：进入 `web/react-admin` 执行 `pnpm install && pnpm dev`（默认代理 8080）；生产构建 `pnpm build`。

## 开发与测试
- 运行测试：`go test ./...`
- 基准测试：`go test -bench=. -benchmem ./internal/...`
- 更详细的覆盖范围与建议见 docs/testing_strategy.md。

## API 契约校验
- 快速对照 OpenAPI 与运行时路由：`go run ./cmd/api-contract-check --spec docs/api/openapi_v2.yaml --format markdown`
- 输出为 Markdown 表格（默认）或纯文本（`--format text`），任何缺失/多余的接口都会使命令以退出码 2 失败。
- CI 中会自动执行同一命令，保持 docs/api/openapi_v2.yaml 与真实 API 同步。

## 更多文档
- 架构与系统规格：docs/system_spec.md
- 部署与运维：docs/deployment_strategy.md
- 观测与告警：docs/monitoring_plan.md、docs/metadata_observability.md
- 存储与迁移：docs/storage_and_schema.md

## 许可证
本项目遵循仓库附带的 LICENSE。
