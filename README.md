# Modern-DHCP

Modern-DHCP 是一个以 Go 构建的企业级 DHCP 控制平面，面向多租户与 IPv4/IPv6 双栈场景。项目覆盖后端服务、数据库迁移、审计与观测、自动化编排以及 Vue 3 管理前端，适用于园区网络、数据中心和云边协同环境中的地址分配与治理。

## 核心能力
- 双栈 DHCP 引擎：独立 DHCPv4/DHCPv6 管线，支持策略扩展与高并发处理。
- 多租户与权限治理：租户隔离、RBAC、审计日志与敏感操作可追溯。
- 分层地址池与策略：支持多维元数据匹配、池选择与冲突防护。
- 高可用与复制：内置 HA/Failover、复制状态可观测与切换控制。
- 运维可观测：Prometheus 指标、健康概览、告警路由与通知集成。
- 自动化工作流：任务调度、审批、幂等执行与失败重试。

## 目录结构
```text
cmd/              后端入口程序（dhcpd、modern-dhcp、dbinit、config-check、api-contract-check）
internal/         业务模块与基础设施实现（pool/lease/auth/monitoring/security/...）
pkg/              对外可复用的数据模型与遥测定义
configs/          配置模板与本地运行配置
migrations/       SQL 迁移脚本
deploy/           Docker 与 Kubernetes 部署清单
docs/             架构、接口、部署、安全、观测等设计文档
web/              Vue 3 + Vite 前端控制台
Modern-DNS_src/   附属前端源码目录（按需参考）
```

## 环境要求
- Go 1.23+
- MySQL 8.0+（可选 PostgreSQL，取决于配置）
- Node.js 18+（前端开发）

## 快速开始（本地开发）
1. 准备配置文件

```bash
# Linux/macOS
cp configs/config.example.yaml configs/config.yaml

# Windows PowerShell
Copy-Item configs/config.example.yaml configs/config.yaml
```

2. 根据环境修改 `configs/config.yaml`（数据库 DSN、认证、租户、HA、告警等）。

3. 初始化数据库迁移

```bash
go run ./cmd/dbinit --config configs/config.yaml --driver mysql
```

4. 启动后端服务

```bash
go run ./cmd/dhcpd --config configs/config.yaml
```

5. 校验配置与 CLI

```bash
go run ./cmd/config-check --config configs/config.yaml --scope all
go run ./cmd/modern-dhcp help
```

## 前端开发（Vue 3）
在 `web/` 目录执行：

```bash
npm install
npm run dev
```

常用命令：
- `npm run build`：构建生产包
- `npm run test`：运行前端测试
- `npm run lint`：代码规范检查

## 常用后端命令
- 运行全部测试：`go test ./...`
- 执行基准测试：`go test -bench=. -benchmem ./internal/...`
- API 契约检查：`go run ./cmd/api-contract-check --spec docs/api/openapi_v2.yaml --format markdown`

## 部署说明
- Docker：参见 `deploy/docker/`
- Kubernetes：参见 `deploy/kubernetes/`
- 生产部署与运维策略：参见 `docs/deployment_strategy.md`

## 监控与运维
- 健康与集群状态：`/cluster/overview`
- 监控总览：`/monitoring/overview`
- 指标端点：`/metrics`

更多可观测设计请参考：
- `docs/monitoring_plan.md`
- `docs/observability_dashboard_plan.md`
- `docs/metadata_observability.md`

## 安全与合规
- 访问控制与租户边界：`docs/section20_access_and_tenancy.md`
- 安全子系统设计：`docs/security_subsystem_plan.md`
- 威胁响应流程：`docs/threat_response.md`

## 文档索引
- 系统总览：`docs/system.md`
- 系统规格：`docs/system_spec.md`
- API 说明：`docs/api.md`
- OpenAPI：`docs/api/openapi_v2.yaml`
- 配置参考：`docs/config_reference.md`
- 存储与模式：`docs/storage_and_schema.md`
- 测试策略：`docs/testing_strategy.md`

## License
See `LICENSE`.
