# 租约生命周期设计

> 版本：2025-12-03（草案）
>
> 适用范围：`internal/lease` 服务、DHCPv4/v6 处理器、策略引擎与外部可观测性组件。

## 1. 生命周期阶段

| 状态 | 描述 | 触发组件 |
| --- | --- | --- |
| `PROVISIONING` | 刚写入或待审批的租约；仅持久化元数据，不向客户端发放 | API/控制台、模拟器 |
| `ACTIVE` | 正常租约，记录 `expiresAt`、`leaseProfile` 等信息 | DHCPv4/v6 处理链 |
| `IDLE_PENDING` | 检测到长时间无流量/心跳；等待优雅通知窗口结束 | Idle Watcher、Guard 遥测 |
| `GRACEFUL_RECLAIM` | 已向上游通知，等待 `graceExpiresAt` | Reclaimer 协程 |
| `RELEASED` | 正常释放，等待冷却或立即可复用 | 租约服务、API |
| `COOLDOWN` | 因冲突/Decline 进入冷却，`cooldownUntil` 前不可复用 | Decline、Guard |
| `QUARANTINED` | 判定异常（安全/冲突频发），需手工解除 | Security Guard、SOC |
| `EXPIRED` | 超时未续租，等待回收 | Reclaimer |
| `FORCE_RECYCLED` | 强制回收完成 | Reclaimer |

状态机需要允许从 `ACTIVE` → `IDLE_PENDING` → `GRACEFUL_RECLAIM` → `RELEASED`/`FORCE_RECYCLED` 等分支，也支持 `COOLDOWN` / `QUARANTINED` 的循环。

### 1.1 状态机与转换细节

```
PROVISIONING
  │(DHCP 承认 / 管理员批准)
  ▼
   ACTIVE ──(闲置信号)──▶ IDLE_PENDING ──(超时/通知)──▶ GRACEFUL_RECLAIM
  │  │                                         │
  │  └─(Decline/冲突)→ COOLDOWN ──(冷却完)──────┘
  │                                        │
  │─(安全告警)→ QUARANTINED ──(手动解封)──────┘
  │
  ├─(客户端释放)→ RELEASED ──(冷却≥0)→ 可复用
  └─(expiresAt 到期)→ EXPIRED ─→ FORCE_RECYCLED
```

| 起始状态 | 事件 | 目标状态 | 备注 |
| --- | --- | --- | --- |
| `PROVISIONING` | 管理端批准/首次 DHCP 交互 | `ACTIVE` | 写入 `lease.Service.AllocateOrReuse` |
| `ACTIVE` | `MarkIdle` 被调用且未续租 | `IDLE_PENDING` | 记录 `idleReason` |
| `IDLE_PENDING` | 在 `idleGrace` 内收到续租/心跳 | `ACTIVE` | 清除 `idle` 标记 |
| `IDLE_PENDING` | 超过 `idleGrace` | `GRACEFUL_RECLAIM` | 触发通知 |
| `GRACEFUL_RECLAIM` | `graceExpiresAt` 到期 | `FORCE_RECYCLED` | 若回收失败则重试 |
| `ACTIVE` | Decline/冲突 | `COOLDOWN` | 写入 `cooldownUntil` |
| `COOLDOWN` | 冷却到期且未再触发冲突 | `RELEASED` | 供分配器复用 |
| `ACTIVE`/`COOLDOWN` | Guard 告警（欺骗/威胁） | `QUARANTINED` | 需管理员解除 |
| `QUARANTINED` | Admin 批准 | `RELEASED` | 可重新分配 |
| 任意 | 手动释放 | `RELEASED` | 记录 `releasedBy` |
| `RELEASED` | 满足复用条件 | `ACTIVE` | 新分配触发 |

## 2. 自动回收机制

### 2.1 闲置检测
- **信号源**：
  - `security.Guard` 采集的端口/ACL 事件（如 NAS-PORT 断开）。
  - NetFlow/心跳（未来 `observability/liveness` 服务）。
  - DHCP 汇报：长时间未收到 RENEW/REBIND。
- **实现**：`internal/lease/idlewatcher`（新组件）订阅上述信号，调用 `lease.Service.MarkIdle(leaseID)`，写入 `lastIdleSignal` 与 `idleReason`。
- **策略**：`policy.lifecycle.idleGrace` 控制 `IDLE_PENDING` → `GRACEFUL_RECLAIM` 的窗口（默认 30 分钟）。

### 2.2 优雅回收
- `lease.Service.BeginGracefulReclaim`：
  1. 将状态置为 `GRACEFUL_RECLAIM`，记录 `graceExpiresAt = now + policy.lifecycle.gracePeriod`。
  2. 通过 `events`（Kafka/Webhook）与 `auditpayload.LeaseLifecycleEvent` 通知上游。
  3. 指标：`modern_dhcp_lease_graceful_pending_total{tenant}`。
- 定时器：`internal/lease/reclaimer` 轮询到 `graceExpiresAt <= now` 的租约后调用 `Release`，若回收失败则重试并记录 `lease.lifecycle.reclaim_failed` 日志。

### 2.3 强制回收
- 条件：`expiresAt <= now` 且策略 `policy.lifecycle.forceOnExpire = true`。
- 动作：直接调用 `lease.Service.Release`，状态转 `FORCE_RECYCLED`，写审计。
- 补充：对于 `Critical` 设备可配置 `policy.lifecycle.forceDelay[profile]` 延后回收。

### 2.4 分级策略
- 在 `models.LeaseProfile` 引入：
  - `ReclaimTier int`（0=最高优先级，数值越大越滞后）。
  - `Critical bool`（必须优雅回收）。
- 回收器遍历租约时按 tier 排序；若超出 `maxConcurrentReclaimsPerTenant` 则保留队列。

## 3. 地址再利用策略

### 3.1 冷却期
- `CooldownUntil` 字段由 Decline/冲突检测写入。
- `allocator.pickIPAddress` 需加载 `ListCooldownIPs(poolID)` 并跳过 `cooldownUntil > now` 的地址。
- 配置：`policy.lifecycle.cooldown.defaultMinutes` + 每个信号的 override（例如 DHCPDECLINE=10 分钟、Rogue detection=4 小时）。

### 3.2 冲突历史避让
- `conflict_history` 为 JSON 数组：`[{"ts":...,"signal":"dhcpdecline","mac":"...","notes":"..."]}`。
- 重新分配时：
  - 若某地址在过去 `policy.lifecycle.conflictWindow` 内冲突次数超过阈值 `conflictThreshold`，则跳过。
  - 指标：`modern_dhcp_allocator_conflict_skips_total{tenant, poolId}`。
  - 审计：`lease.conflict_avoidance`。

### 3.3 轮转/负载均衡
- `models.AddressPool` 字段：`allocationMode`, `priorityWeight` 已存在。
- 策略函数：
  - `SEQUENTIAL`: 原行为。
  - `ROUND_ROBIN`: 使用 `pool_alloc_cursor` 表持久化游标。
  - `PRIORITY_WEIGHTED`: 结合 `priorityWeight` 生成加权区间。
  - `HIGH_FIRST` / `LOW_FIRST`: 新增模式，优先选择高/低地址段。
- IPv6 支持：`internal/lease/allocator_v6.go` 需要相同逻辑，避免仅顺序扫描。

### 3.4 优先分配策略
- 可在地址池配置 `preferredRanges`（JSON 数组，包含 `start/end/tag`）。
- 分配器根据 `preferredRanges` + `DeviceType/UserGroups` 匹配合适范围，如存量不足再回退至全池。
- 对应审计：记录最终选择的范围及原因。

## 4. 可观测性与审计

| 信号 | 指标 | 描述 |
| --- | --- | --- |
| 自动回收触发 | `modern_dhcp_lease_reclaims_total{tenant,result}` | result=`graceful`,`forced`,`failed` |
| 冷却跳过 | `modern_dhcp_allocator_cooldown_skips_total{tenant,poolId}` | 每次跳过计数 |
| 冲突避让 | `modern_dhcp_allocator_conflict_skips_total{tenant,poolId,signal}` | 区分信号来源 |
| 回收延迟 | `modern_dhcp_lease_reclaim_latency_seconds` | 从 `expiresAt` 到释放的直方图 |
| 生命周期状态 | `modern_dhcp_lease_lifecycle_gauge{tenant,state}` | Gauge 代表当前租约数量 |
| 优雅回收积压 | `modern_dhcp_lease_graceful_pending_total{tenant}` | 当前等待回收的租约数量 |
| 空闲检测事件 | `modern_dhcp_lease_idle_events_total{tenant,reason}` | IdleWatcher/Guard 标记次数 |
| 分配模式覆盖率 | `modern_dhcp_allocator_mode_assignments_total{tenant,mode}` | 每种 `allocationMode` 的命中次数 |

审计事件：新增 `LeaseLifecycleEvent`，包含：
- `action`：`IDLE_MARKED`、`GRACEFUL_NOTIFY`、`FORCE_RECYCLE`、`COOLDOWN_ENTER`、`COOLDOWN_EXIT`、`CONFLICT_AVOIDED` 等。
- `context`：租约、地址池、信号、触发组件。

## 5. 接口与配置

### 5.1 配置项（`configs/config.yaml`）
```yaml
policy:
  lifecycle:
    autoReclaim:
      enabled: true
      interval: 60s
      batchSize: 200
    idleGrace: 30m
    gracePeriod: 10m
    forceOnExpire: true
    forceDelay:
      critical: 15m
    cooldown:
      defaultMinutes: 10
      signals:
        dhcpDecline: 10
        guardSpoof: 240
    conflictWindow: 24h
    conflictThreshold: 2
    maxConcurrentReclaimsPerTenant: 20
```

### 5.2 API / CLI
- `GET /api/v1/tenants/{tenantId}/leases?state=IDLE_PENDING`：新增状态过滤。
- `POST /api/v1/tenants/{tenantId}/leases/{leaseId}/graceful-reclaim`：人工触发。
- CLI：`modern-dhcp leases lifecycle --tenant <id> --state GRACEFUL_RECLAIM`。
- Web UI：在 `/console/lease-security` 增加生命周期筛选及批量操作按钮。

## 6. 实施计划

1. **Schema & 数据层**（Sprint N+1）
   - 扩展 `leases` 表（`state`, `cooldown_until`, `conflict_history`, `grace_expires_at`, `idle_reason`）。
   - 新建 `pool_alloc_cursor` 表。
2. **服务层改造**
   - `lease.Service`: 引入 `IdleWatcher`, `Reclaimer`, 新状态转换逻辑。
   - `allocator`: 支持 IPv6 轮转、优先区间。
3. **可观测性**
   - 在 `internal/observability` 输出上述指标；更新 `docs/dashboards` 示例。
4. **管理接口**
   - REST+CLI+UI 追加生命周期过滤/操作。
5. **测试计划**
   - 单元：状态机转换、冷却/冲突过滤、游标持久化。
   - 集成：Simulate 调用 + fake Guard/Idle 信号。
   - 负载：大规模租约回收压测，验证批处理与指标可观测性。

## 7. 组件职责矩阵

| 组件 | 职责 | 关键接口 / 文件 |
| --- | --- | --- |
| `internal/lease.Service` | 状态机持久化、回收/冷却计算、分配器协调 | `lease/service.go`, `lease/allocator*.go` |
| `internal/security/guard` | 采集接入事件、防护信号，调用 `MarkIdle`、`EnterQuarantine` | `security/guard/context.go` |
| DHCPv4/v6 处理器 | 将客户端/relay 元数据映射到策略与租约上下文，触发 `Release/Decline` | `internal/dhcpv4/handler.go`, `internal/dhcpv6/handler.go` |
| 策略引擎/配置 | 通过 `policy.lifecycle.*` 调整各窗口、tier、冷却策略 | `configs/config.yaml`, `internal/policy` |
| 可观测性子系统 | 暴露指标、仪表盘、审计串联 | `internal/observability`, `docs/dashboards/*` |
| 事件/审计流水线 | Kafka/Webhook + `auditpayload` 记录生命周期操作 | `internal/events`, `pkg/auditpayload` |

跨组件协作示例：Guard 报告端口下线 → IdleWatcher 调用 `MarkIdle` → Lease Service 进入 `IDLE_PENDING` → Observability 打点 → 若未恢复则触发 Reclaimer → Events 推送 `GRACEFUL_NOTIFY`。

## 8. 里程碑与依赖

| 里程碑 | 内容 | 依赖 | 输出 |
| --- | --- | --- | --- |
| M1：Schema 落地 | 扩展 `leases` 表、建 `pool_alloc_cursor` | DBA 审批、迁移工具 | `migrations/00xx_lease_lifecycle.sql` |
| M2：Service/Allocator | 实现 IdleWatcher、Reclaimer、IPv6 分配策略 | M1 数据结构 | 新的 `internal/lease` 包逻辑 + 单测 |
| M3：Telemetry & 审计 | 指标、事件、Grafana 面板更新 | M2 指标钩子 | `internal/observability`, `docs/dashboards/lifecycle.json` |
| M4：API/UI | 生命周期过滤、批量操作 | M2 状态机 | REST/CLI/Console 增量发布 |
| M5：SRE 验收 | 压测 + Runbook | 前序所有里程碑 | `docs/runbooks/lease_lifecycle.md`, SOP |

关联文档：
- `docs/lease_lifecycle_assessment.md`（现状评估）
- `docs/phase3_lifecycle_security_plan.md`（路线图）
- `docs/threat_response.md`（与 `QUARANTINED` 相关的安全动作）
- `docs/dashboards/pool_selector_overview.json`（示例可扩展为生命周期面板）

---

此设计与 `docs/lease_lifecycle_assessment.md`、`docs/phase3_lifecycle_security_plan.md` 相互参照，为 Phase 3 交付奠定实施蓝图。后续若配置字段或状态机变更，请同步更新此文档与 README。