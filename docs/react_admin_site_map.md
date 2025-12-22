# React Admin Site Map & API Coverage

This document captures the planned navigation tree for the Modern DHCP React Admin (React 19 + TypeScript + Ant Design + Vite) and validates each feature group against currently implemented or planned backend endpoints.

## SPA Shell & Shared Capabilities
- **Routing**: `/auth/login` (public) plus `/app/*` protected routes handled by `ProtectedRoute`.
- **State/Session**: Zustand store carries `{ token, profile, tenantId }`; React Query drives data hydration.
- **HTTP Client**: `src/services/http.ts` injects auth headers, tenant context, and audit metadata per [docs/web_ui_modernization.md](web_ui_modernization.md).
- **Metadata-driven nav** (planned): `/api/v1/ui/metadata` will emit capability sets so menus align with RBAC/tenant grants (see Section 6 of web_ui_modernization plan).

## Module Breakdown
Each table lists primary UI concerns, their intended React routes, and backend status.

### 系统管理
| Feature | Proposed Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 用户登录/登出 (用户名/密码) | `/auth/login` | `POST /auth/login`, `POST /auth/logout` (see internal/server/http_server.go) | ✅ Implemented |
| LDAP/AD/SSO 联动 | `/app/system/auth/providers` | Not yet exposed in OpenAPI | ⚪ Not implemented |
| 多租户隔离视图 | `/app/system/tenants`, `/app/system/tenant-switcher` | `/tenants` (planned per docs/system_spec.md Phase 2) | 🟡 Planned |
| RBAC 权限控制 | `/app/system/roles` | `/rbac/roles`, `/rbac/assignments` (not yet surfaced) | ⚪ Missing |
| API 密钥管理 | `/app/system/api-keys` | `POST/GET /auth/api-keys` (not present) | ⚪ Missing |
| 会话监控/强制下线 | `/app/system/sessions` | `/auth/sessions` (not present) | ⚪ Missing |
| 资源配额管理 | `/app/system/tenant-quotas` | `/tenants/:id/quotas` (mentioned in docs/system_spec.md) | 🟡 Planned |
| 租户切换控件 | Header tenant selector | Tenant context REST not yet wired | 🟡 Planned |
| 用户管理 / 操作审计 | `/app/system/users`, `/app/system/audit` | `/users`, `/audit/logs` (pkg/auditpayload references) | 🟡 Partial (backend services exist, HTTP handlers pending) |
| 系统配置 (全局参数/通知/日志) | `/app/system/settings` (mapped to `SettingsPage`) | `/system/config`, `/notifications/channels`, `/logging/config` (not exposed) | ⚪ Missing |

### 仪表盘
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 系统健康概览 | `/app/dashboard` | `GET /monitoring/overview` (docs/api/openapi_v2.yaml) | ✅ Wired in DashboardPage |
| 地址池热点 & 客户端分布 | `/app/dashboard` | `GET /monitoring/pools` | ✅ Wired |
| 告警收件箱 | `/app/dashboard` & `/app/monitoring` | `GET /monitoring/alerts` | ✅ Wired |
| 集群运行状态 / 节点心跳 | `/app/dashboard/cluster` | `/clusters`, `/server/nodes` (not exposed) | ⚪ Missing |
| 服务可用性探针 | `/app/dashboard/availability` | `/monitoring/checks` (not present) | ⚪ Missing |
| 实时事件流 (租约/日志) | `/app/dashboard/streams` | WebSocket/SSE `/events/*` (not implemented) | ⚪ Missing |
| 容量预测 | `/app/dashboard/insights` | `/analytics/capacity` (not present) | ⚪ Missing |

### 地址池管理
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| IPv4 地址池 CRUD | `/app/pools` (existing page) | `/pools` (internal/pool handlers) | 🟡 List implemented, full CRUD not wired |
| 子网/范围/排除配置 | `/app/pools/:id` | `/pools/:id/subnets`, `/pools/:id/ranges` | ⚪ Missing |
| VLAN / 位置 标签 | `/app/pools/:id/tags` | `/pools/:id/tags` | ⚪ Missing |
| 地址池监控 (历史/预警) | `/app/pools/:id/monitoring` | `/monitoring/pools/:id` | ⚪ Missing |
| IPv6/前缀池 | `/app/pools/ipv6` | `/pools/ipv6` (docs mention internal/dhcpv6) | 🟡 Planned |
| 地址池分析报表 | `/app/pools/analytics` | `/analytics/pools` | ⚪ Missing |

### 租约管理
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 活跃租约搜索/筛选 | `/app/leases` (existing) | `GET /leases` | ✅ Basic table wired |
| 租约详情/续期/释放 | `/app/leases/:id` | `POST /leases/:id/release`, `/leases/:id/renew` (not wired) | 🟡 Backend available, UI TBD |
| 历史租约/审计 | `/app/leases/history` | `/leases/history` | ⚪ Missing |
| 租期策略配置 | `/app/leases/policies` | `/lease-policies` | ⚪ Missing |

### 静态绑定管理
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 单条/批量 MAC 绑定 | `/app/static-bindings` | `/bindings/static` (internal/lease binding service) | 🟡 Services exist, HTTP layer TBD |
| 客户端ID/用户绑定 | `/app/static-bindings/identities` | `/bindings/identities` | ⚪ Missing |
| CSV/Excel 导入导出 | `/app/static-bindings/import` | `/bindings/import` | ⚪ Missing |

### DHCP 选项配置
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 标准选项库 | `/app/dhcp-options` | `/dhcp/options` (internal/dhcp_options_store.go mentions Phase 2) | 🟡 Planned |
| 自定义选项 & 校验 | `/app/dhcp-options/custom` | `/dhcp/options/custom` | ⚪ Missing |
| 模板系统 | `/app/dhcp-options/templates` | `/dhcp/options/templates` | ⚪ Missing |

### 策略管理
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 策略工作台 (当前 `PolicyPage`) | `/app/policy` | `/policy/rules` (internal/policy) | 🟡 Backend packages exist, UI placeholders |
| 设备/用户/位置策略 | `/app/policy/rules/*` | `/policy/matchers` | ⚪ Missing |
| 策略部署/回滚 | `/app/policy/deployments` | `/policy/deployments` | ⚪ Missing |
| 策略测试工具 | `/app/policy/test` | `/policy/dry-run` | ⚪ Missing |

### 服务器集群管理
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 节点列表/状态 | `/app/cluster/nodes` | `/cluster/nodes` | ⚪ Missing |
| 高可用配置 | `/app/cluster/ha` | `/cluster/ha` | ⚪ Missing |
| 服务配置/启停 | `/app/cluster/services` | `/cluster/services` | ⚪ Missing |
| 集群监控 | `/app/cluster/monitoring` | `/monitoring/cluster` | ⚪ Missing |

### 安全与防护
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| DHCP Snooping & 绑定表 | `/app/security/snooping` | `/security/snooping` | ⚪ Missing |
| 速率限制 & 违规处理 | `/app/security/rate-limit` | `/security/rate-limit` (partial snapshot via `/monitoring/overview.security`) | 🟡 Snapshot only |
| 非法服务器检测 | `/app/security/rogue` | `/security/rogue` | ⚪ Missing |
| 802.1x / RADIUS 集成 | `/app/security/integrations` | `/security/integrations` | ⚪ Missing |

### 监控与告警
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 实时性能监控 | `/app/monitoring` (current page) | `/monitoring/overview`, `/monitoring/pools` | ✅ Partial |
| 业务监控 (租约/异常) | `/app/monitoring/business` | `/monitoring/requests`, `/monitoring/anomalies` (not exposed) | ⚪ Missing |
| 告警规则配置 | `/app/monitoring/rules` | `/monitoring/alerts/rules` (planned) | 🟡 Planned |
| 告警处理流程 | `/app/monitoring/inbox` | `/monitoring/alerts` + POST actions (not implemented) | 🟡 Partial |
| 报表与分析 | `/app/monitoring/reports` | `/reports/monitoring` | ⚪ Missing |

### 运维自动化 (AutomationPage in place)
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 定时任务/工作流 | `/app/automation` | `/automation/schedules`, `/automation/jobs` | ✅ Implemented (list, snapshots, job detail) |
| 配置同步/变更流程 | `/app/automation/workflows` | `/automation/workflows` (docs/automation_workflows_plan.md) | 🟡 Planned |
| 脚本库/调度 | `/app/automation/scripts` | `/automation/scripts` | ⚪ Missing |
| 故障诊断工具 | `/app/automation/diagnostics` | `/automation/diagnostics` | ⚪ Missing |

### API 与集成
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| REST API 控制台 & Tokens | `/app/integrations/api` | `/auth/api-keys`, `/openapi` | 🟡 Tokens missing, OpenAPI docs available |
| 第三方系统集成 | `/app/integrations/external` | `/integrations/*` (not present) | ⚪ Missing |
| 数据导入导出 | `/app/integrations/import-export` | `/export/*`, `/import/*` | ⚪ Missing |
| Webhook 事件订阅 | `/app/integrations/webhooks` | `/webhooks/subscriptions`, `/webhooks/deliveries` | ⚪ Missing |

### 系统维护
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 备份/恢复 | `/app/maintenance/backups` | `/maintenance/backups` | ⚪ Missing |
| 系统升级管理 | `/app/maintenance/upgrades` | `/maintenance/upgrades` | ⚪ Missing |
| 性能优化 & 容量管理 | `/app/maintenance/perf` | `/maintenance/perf`, `/maintenance/capacity` | ⚪ Missing |

### 帮助与支持
| Feature Cluster | Route | Backend Endpoint(s) | Status |
| --- | --- | --- | --- |
| 在线文档/版本信息 | `/app/help/docs` | Static markdown + `/system/info` | 🟡 Partial (docs exist, need endpoint) |
| 故障诊断工具 | `/app/help/diagnostics` | `/support/bundles` | ⚪ Missing |
| 技术支持请求 | `/app/help/support` | `/support/tickets` | ⚪ Missing |

## Next Steps
1. **Extend Router**: add nested route definitions for the planned sections above; gate unfinished modules behind feature flags to avoid broken nav.
2. **OpenAPI Alignment**: capture missing endpoints in `docs/api/openapi_v2.yaml`, tagging each with RBAC scopes so frontend permissions can be enforced.
3. **Service Clients**: scaffold `src/services/<module>.ts` files per endpoint, even if currently mocked, to standardize React Query usage.
4. **Tenant-aware data**: ensure each request includes tenant headers as described in Section 9 of web_ui_modernization.md, especially for dashboards and automation.
5. **Real-time channels**: plan SSE/WebSocket endpoints for event streams (租约事件、操作日志、告警推送) so React components can subscribe without polling.
