# HA 节点加入与集群治理实施方案

更新日期：2026-02-27

本方案用于回答并落地以下问题：
1. 新增备节点后如何同步地址池、静态绑定、租约。
2. 主备同时访问同一 MySQL、Redis 的风险与规避。
3. 当前集群管理如何优化与可扩展方向。

---

## 1. 新增备节点的同步流程（推荐三阶段）

### 1.1 设计目标
- **零双写**：新增节点在追平前不得发号、不得执行写操作。
- **可验证**：每阶段有可观测指标和明确完成条件。
- **可回滚**：任意阶段失败都能回到安全 standby/draining 状态。

### 1.2 节点状态机（Join FSM）
- `REGISTERED`：节点注册完成，但未参与业务。
- `SNAPSHOTTING`：执行全量快照导入（地址池/静态绑定/租约）。
- `CATCHING_UP`：按事件增量追平（binlog/CDC/Kafka）。
- `VERIFYING`：执行 checksum 与样本对账。
- `WARM_STANDBY`：追平完成，可接管但不主动写。
- `ACTIVE`：故障切换后成为主节点。
- `FAILED`：同步失败或校验失败，等待人工处理。

### 1.3 同步对象与顺序
建议顺序（依赖从上到下）：
1) `lease_profiles`
2) `address_pools`
3) `static_bindings`
4) `leases_v4` / `leases_v6` / `prefix_leases_v6`

说明：
- 地址池与静态绑定属于配置平面，先同步可避免租约回放时引用缺失。
- 租约属于运行态，最后做全量 + 增量追平。

### 1.4 时序（推荐）
1. **Join 请求**：控制面创建 `join_job`，目标节点进入 `REGISTERED`。
2. **写门禁开启**：目标节点强制 `standby + draining`，拒绝所有写 API 与 DHCP ACK。
3. **记录高水位**：主节点记录 binlog 位点（或 CDC offset）。
4. **全量快照导入**：按对象顺序导入，启用分批与断点续传。
5. **增量追平**：从高水位消费变更流，持续更新 lag。
6. **一致性校验**：表级行数 + 哈希校验 + 抽样校验。
7. **进入 WARM_STANDBY**：允许健康探针、复制探针，仍禁止写。
8. **可选接管演练**：执行 dry-run failover，确认接管路径。

### 1.5 完成判据（SLO）
- 复制延迟 `lag <= 2s`（连续 60s）。
- 校验通过率 100%。
- 节点状态 `StateNormal` 且探针连续成功。
- `pending_actions == 0`（可参考 `/cluster/overview` 聚合字段）。

---

## 2. 主备共用 MySQL/Redis 的问题与规避

## 2.1 MySQL 共用的主要问题
1. **脑裂双写**：主备同时写入租约，出现地址冲突。
2. **唯一约束不足**：现有唯一约束更偏标识符维度，不足以覆盖“同池同地址活跃唯一”。
3. **切换窗口误写**：角色切换期间旧主仍可写，导致回切冲突。
4. **长事务与锁竞争**：切换时如果存在慢事务，会扩大不一致窗口。

### 2.2 MySQL 规避建议
- **单写门禁**：只有 `RolePrimary` 可写（API + DHCP 分配双重门禁）。
- **fencing token**：写请求必须携带当前 epoch/token，过期节点写入直接拒绝。
- **数据库兜底约束**：
  - 建议为租约增加“池+IP+活跃态”唯一性策略（可用生成列/部分唯一实现）。
  - 建议为静态绑定增加“pool_id + ip_address”唯一约束。
- **幂等写入**：所有写路径带 `request_id`/`dedupe_key`，重复提交可安全重放。
- **切换保护**：旧主先 `draining` 后摘流量，再执行角色变更。

## 2.3 Redis 共用的主要问题
1. **L1/L2 缓存不一致**：每节点有本地 L1，切换后可能读旧值。
2. **失效风暴**：大量 key 前缀删除会造成抖动。
3. **把 Redis 当锁但缺 fencing**：网络分区时锁语义不可靠。

### 2.4 Redis 规避建议
- **缓存用途限定**：Redis 只做缓存与加速，不作为最终一致性来源。
- **统一 key 命名空间**：包含 `tenant/pool/epoch`，切换后自然隔离旧缓存。
- **失效广播**：写后发布失效事件，所有节点同步清理 L1。
- **锁最小化**：关键一致性以 DB 约束与 coordinator 为准，Redis 锁只做辅助。

---

## 3. 对当前集群管理功能的优化建议

## 3.1 P0（必须优先）
1. **落地真实 replicator**
   - 现状：failover replicator 仍为 noop。
   - 目标：接入 binlog/CDC 消费器，输出 `apply_lag`、`apply_error`、`last_offset`。
2. **写门禁统一中间件**
   - 所有配置写接口、租约写接口、迁移入口统一校验 `role == primary`。
3. **Join Job 编排器**
   - 提供 `create/get/cancel/retry` API；前端显示每阶段进度与错误。

## 3.2 P1（稳定性增强）
1. **一致性校验器**：表级 checksum + 采样对账。
2. **故障演练自动化**：周期性 dry-run，验证 RTO/RPO。
3. **审计闭环**：切换原因、fencing 结果、延迟曲线入审计。
4. **告警联动**：`lag`、`split-brain risk`、`join failed` 触发 P1/P2。

## 3.3 P2（能力扩展）
1. 多备节点（N+1）与优先级接管。
2. 按 pool/tenant 粒度故障域隔离。
3. 跨 AZ 异地容灾与延迟分级策略。
4. 基于历史稳定性的自适应回切窗口。

---

## 4. 与现有代码的对接建议

### 4.1 复用点
- 角色/状态机基础：`internal/failover/manager.go`
- HA 配置入口：`internal/config/config.go` 中 `HAConfig/ReplicationConfig`
- 集群视图接口：`/cluster/overview`（`internal/server/feature_routes.go`）
- 租约 CDC 确认：`internal/replication/journal.go` 与 `internal/lease/service.go`

### 4.2 需要补齐的关键实现
- `internal/failover` 下新增真实 `Replicator` 实现（替换 noop）。
- 新增 `internal/ha/join`（建议）负责节点加入编排。
- 在 server 写接口处统一接入 `PrimaryWriteGuard`。

---

## 5. 建议参数（初始值）

- `heartbeatInterval`: 200ms
- `failoverTimeout`: 1500ms
- `probe.failureThreshold`: 3
- `replication.ackTimeout`: 2s
- `join.verifyStableWindow`: 60s
- `maxAllowedLagForPromotion`: 2000ms

> 参数需结合生产网络抖动、数据库负载与业务峰值压测后再收敛。

---

## 6. 里程碑（两周示例）

### Week 1
- 完成 Join Job 状态机与 API。
- 完成写门禁中间件接入。
- 接入复制 lag 指标并展示到 `/cluster/overview`。

### Week 2
- 完成一致性校验器。
- 完成 failover 演练脚本（dry-run + 回滚）。
- 接入告警规则并做一次演练复盘。

---

## 7. 验收标准

1. 新增备节点可在 30 分钟内进入 `WARM_STANDBY`（中型数据规模）。
2. 主节点故障后 10 秒内完成接管，且无重复分配。
3. 故障演练后审计链路完整（事件、原因、操作者、指标快照）。
4. 发生网络抖动时不会出现双主可写。
