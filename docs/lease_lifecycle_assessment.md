# 租约生命周期评估（当前状态）

日期：2025-12-02

本说明记录了 Phase 3 增强上线前，代码库中已覆盖的实现情况，重点关注自动回收、冷却处理、冲突规避以及轮换/优先级策略。

## 自动回收 / 过期处理
- `internal/lease/reclaimer.go` 新增基于 ticker 的后台任务，在可配置的宽限期后释放过期租约；`cmd/dhcpd/main.go` 会在 `policy.lifecycle.autoReclaim.enabled` 为 true 时启用该任务。
- 仓储层暴露 `ListExpiredLeases`，支持批量遍历而无需全表扫描。
- 存在的空缺：尚未产生审计/事件，且回收决策未区分租户或优先地址池。

## 冷却 / 隔离
- 分配器现已遵循 `CooldownUntil` 时间戳：`pickIPAddress` 会查询 `ListCooldownIPs` 并跳过仍在冷却或已隔离的地址。
- Decline 操作会向 `conflict_history` 追加结构化条目，保留 reason/signal 元数据；根据策略阈值，租约会进入明确的 `COOLDOWN` 或 `QUARANTINED` 状态。
- 运维可通过 `POST /api/v1/tenants/{tenantId}/leases/{leaseId}/cooldown/clear`（或 `modern-dhcp leases cooldown-clear ...`）清理问题租约，将其重置为 `RELEASED`。
- 尚存空缺：
  - 指标与仪表板仍未展示冷却/隔离计数。
  - 除 `state=COOLDOWN` 过滤外，尚无按生命周期批量列举的 API。

## 冲突规避
- Decline 流程会记录带时间戳/信号的冲突条目，为后续规避策略提供审计轨迹。
- DHCP 处理器在收到客户端的 DHCPDECLINE 时仅将租约标记为 declined；尚未将 ARP/ND 探测或安全信号反馈给分配逻辑。
- 遥测现已加入 `modern_dhcp_lease_conflicts_total` 计数器（标签：tenant、signal）以及 `lease.conflict_detected` 审计事件，包含租约/池元数据，SOC/NOC 可直接追踪波动，无需额外查询表数据。

## 轮换 / 优先策略
- 地址池通过 schema/API/migration 暴露 `allocationMode`（`SEQUENTIAL`、`ROUND_ROBIN`、`PRIORITY_WEIGHTED`）与 `priorityWeight`（1-100），默认保持顺序分配与 50% 中性权重。
- `pickIPAddress` 在 IPv4 场景下支持 `ROUND_ROBIN`（每池内存游标循环）和 `PRIORITY_WEIGHTED`（偏向区间前部，自动回退）；IPv6 仍采用顺序扫描。
- CLI/API 载荷可设置这些开关，运维可在创建/更新地址池时调整行为。
- 尚存空缺：
  - 进程重启会重置轮询游标，持久化存储（DB/Redis）尚未落实。
  - 各模式的利用率指标/可观测性尚未交付。
  - IPv6 分配目前忽略非顺序模式。

## 缺口汇总
| 能力 | 覆盖情况 | 说明 |
| --- | --- | --- |
| 自动回收任务 | ⚠️ 部分 | 后台任务已存在，但缺少审计钩子且未考虑租户公平。 |
| 冷却状态执行 | ⚠️ 部分 | 具备 `COOLDOWN`/`QUARANTINED` 状态与清理端点，但仍缺指标。 |
| 冲突历史利用 | ⚠️ 部分 | Decline 已记录历史，但缺少 UI/指标及主动规避。 |
| 轮换/优先分配 | ⚠️ 部分 | IPv4 支持池级分配模式，但游标仅存内存，IPv6 仍为顺序分配。 |

该评估为 `docs/phase3_lifecycle_security_plan.md` 中跟踪的 Phase 3 任务提供参考。
