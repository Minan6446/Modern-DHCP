# 高可用架构设计

> 版本：2025-12-03（草案）
>
> 范围：DHCP 服务节点、租约数据库、策略/配置存储、事件与可观测性通道。

## 目标
- 兼容 ISC DHCP Failover 协议，实现主从热备及快速切换。
- 支持多活/地理分布式部署，保证租约与配置在多节点之间协调一致。
- 在故障场景下维持客户端透明体验，心跳检测 < 1 秒、切换 < 3 秒。
- 提供可观测性、自动恢复与回切策略，降低运维复杂度。

## 6.1 主从同步（Active/Standby）

### 6.1.1 Failover 协议兼容
- **ISC DHCP 模型**：实现 `Partner Down`, `Communication Interrupted`, `Normal` 等状态机，保留 `MCLT`（Maximum Client Lead Time）约束。
- **复用通道**：在 `internal/failover` 新增 TCP-based 会话，交换 `BNDUPD/BNDACK` 等消息；与租约服务共享序列化格式（JSON Protobuf 均可）。
- **状态持久化**：将 failover 状态写入 `failover_status` 表，节点重启后可恢复现状。
- **互操作**：提供开关允许与传统 ISC 服务器对接，便于渐进迁移。

### 6.1.2 租约数据库实时同步
- **数据库层复制**：推荐 MySQL InnoDB Cluster / Group Replication；若只限两节点，可启用半同步复制 + GTID。
- **CDC 通道**：在 `cmd/failover-relay` 运行 Debezium/Canal，监听 `leases`, `prefix_leases`, `bindings` 变更，实时推送给备节点以驱动本地缓存。
- **冲突化解**：主节点为写入源；备节点仅在 `Partner Down` 状态开启本地写入，并在恢复后通过“补发窗口”与主节点 reconciler 合并。

### 6.1.3 配置同步与一致性检查
- **配置源**：集中托管于 GitOps Repo 或 etcd；所有节点在启动时从同一 commit 拉取。
- **散播**：使用 `internal/config/watcher` 订阅 etcd/kafka，热加载策略和租约生命周期配置。
- **一致性检查**：每次变更附带 `checksum`，节点在接收到配置后将 hash 回传协调器；若 hash 不一致则拒绝启用并报警。

### 6.1.4 脑裂检测与自动恢复
- **仲裁**：引入第三方协调者（etcd/Raft/Consul），所有节点在对外提供写服务前必须获得 lease。
- **心跳**：Failover 对之间以 <500ms 周期互发 `HEARTBEAT`，丢失 3 次即标记为 `Communication Interrupted`。
- **自动化处理**：
  - 双方都看不到对端但仲裁仍在线 → 仅获得仲裁 lease 的节点维持可写，另一方退化为只读。
  - 仲裁也不可达 → 两节点都切换为只读模式，并持续广播“降级状态”，等待人工介入。
- **恢复**：当网络恢复时执行 `resync` 流程，比对 `leases` 版本号并执行双向补偿。

## 6.2 多活集群（Active/Active）

### 6.2.1 入口负载均衡
- **VIP/VRRP**：同城多节点通过 Keepalived/MetalLB 暴露虚拟 IP，结合 BFD/ARP 抢占保证 <3 秒切换。
- **DNS 轮询 or GSLB**：面向客户端/relay 的多 IP RR 记录；监控系统需在节点故障时快速下线对应记录。
- **Anycast+BGP**：在运营商/数据中心使用 Anycast 前缀，流量按最短路径进入最近节点。

### 6.2.2 地理分布与容灾
- **Region Pair**：每个租户指派主/备 Region，主写入，备以异步复制跟随。
- **数据复制**：使用 MySQL 多源复制或基于 binlog 的 Kafka 复制管道，实现跨地域日志回放。
- **容灾演练**：提供 `failover simulate --region a -> b` CLI，在演练期间冻结主区写入，确保 Recovery Time Objective (RTO) < 5 分钟。

### 6.2.3 一致性模型
- **强一致**：对需要严格顺序的租户，使用单主+多数派提交（Group Replication）。
- **最终一致**：普通租户采用事件驱动的最终一致流程；租约冲突通过冲突检测与 `MCLT` 缓冲避免影响客户端。
- **分片**：支持按租户或地理分片分配主节点，减少跨洲延迟。

### 6.2.4 节点自动发现与配置
- **注册表**：Consul/etcd 记录节点元数据（region、AZ、角色、权重、健康状态）。
- **引导流程**：新节点启动时向注册表宣告自身→下载配置→执行自检（数据库、Kafka、failover link）→加入集群旋转。
- **灰度**：可指定 `drain=true` 令节点停止接受新流量，但继续服务现有 session，为滚动升级提供窗口。

## 6.3 故障转移流程

### 6.3.1 心跳检测
- **探针**：
  - L2：VRRP/BFD 监测本地端口。
  - L4：gRPC/HTTP `/healthz` (含数据库/消息队列依赖)
  - 应用：Failover TCP 链路传递序列号。
  - 集群：Serf/Consul Agent 或自研 UDP Ping，每个节点都会以 `<1s` 周期探测对端或仲裁者，刚性依赖在 server 层以健康钩子暴露。
- **频率与阈值**：默认 250~500ms 间隔，连续 3 次失败 (<3s) 即判定为失联并触发协调器更新 VIP/DNS/Anycast；探针成功即刷新 failover 协议心跳，保持租约共享状态。

### 6.3.2 自动切换
- **阈值**：探针确认失效后立即向协调器汇报；协调器更新 VIP/BGP/DNS 目标。
- **租约同步**：备节点先执行 `catchup` 拉取最新租约，再开始响应客户端；全流程目标 < 3 秒。
- **可观测性**：`modern_dhcp_ha_failovers_total{role}` 计数 + `ha_failover_duration_seconds` 直方图。

### 6.3.3 客户端无感知迁移
- 复用 failover 协议共享租约与事务 ID，确保客户端在重发 REQUEST/OFFER 时不会收到 `NAK`。
- Option 82/relay 元数据保持一致：多节点共用同一策略缓存与池解析，防止迁移后选取不同地址。
- 对支持 Rapid Commit 的客户端，可让备节点继续发送相同 Server ID，避免 DHCP 重新绑定。

### 6.3.4 回切策略
- **自动回切**：主节点恢复后，探针需连续 `ha.failback.stablePeriod`（默认 60s）保持成功，协调器允许运行且状态为 `Normal` 时自动切回；整个流程由 failover manager 根据 `ha.failback.mode=auto` 自动触发。
- **手动回切**：`ha.failback.mode=manual` 时保持当前活跃角色不变，直到 SRE 通过 CLI/API（如 `POST /api/v1/ha/allow-failback`）审批 `AllowFailback` 动作后才会释放 VIP 并切回。
- **审计**：所有 failover/failback 事件写入 `ha.events` 表 + Kafka 主题，并在 `/healthz` 中暴露当前角色与最后一次心跳时间，供 SOC/NOC 追溯。

## 7. 性能优化

### 7.1 万级并发支持
- **连接池管理**：
   - MySQL：在 `internal/db/mysql.go` 启用 `sql.DB` 级连接池监控，结合 `go-sql-driver/mysql` 的 `maxLifetime/jitter` 与 `db.SetConnMaxLifetime`，并为主/备节点分别设置 `MaxOpen=500`, `MaxIdle=200`。提供 `ha.performance.dbPoolOverrides`，支持按角色动态扩容连接池。
   - 网络：重构 `internal/server/http_server.go` 与 DHCP 监听器以共享 `netpoll`/`reuseport`，并将 UDP socket 池化（每 CPU 绑核）。
- **请求队列优化**：在 `internal/server` 和 `internal/dhcpv4` 增加 `PriorityQueue`（基于 `container/heap`），优先处理续租/关键租约；交互通过 `metrics` 导出队列长度与等待时间。
- **响应时间 <10ms (P99)**：引入 `adaptive batching`（例如把多条策略查询在单个 SQL 中完成）、`fast-path` 缓存（内存中 caching tenant profile+pool 元数据），通过 `modern_dhcp_request_latency_seconds{quantile="0.99"}` 追踪。
- **支持 10 万+ 并发客户端**：
   - 使用 `SO_REUSEPORT` + 每核 goroutine acceptor，提高 UDP fan-out；
   - 在 `internal/dhcpv4/server.go` 引入 `ring buffer` 驱动的无锁 channel，减少调度开销；
   - 扩展 `redis` 作为租约热缓存，命中即返回，降低数据库压力。

### 7.2 内存与存储优化
- **内存数据库缓存**：
   - 读路径：使用 Redis/Memcached 缓存 `lease.Result`、策略评估结果与 pool selector；失效策略由 `LeaseProfile` TTL 驱动。
   - 写路径：在 failover 模式下共享 redis 集群，通过复制延迟指标确保热备一致。
- **数据库连接复用**：复用 `sql.Tx`，通过 `sync.Pool` 缓存 `lease.Repository` 查询参数对象，减少 GC。
- **租约数据压缩**：在 `lease.Repository` 写入前对历史租约/日志字段使用 `zstd` 压缩（store as BLOB），并提供 `lease.compressThresholdBytes` 开关。
- **批量写入优化**：汇聚多条续租/释放操作，批量执行 `INSERT ... ON DUPLICATE KEY UPDATE`；新增 `lease.batchFlushInterval` 与 `batchSize` 配置，默认 2ms/64 条，保障写放大受控。

### 7.3 协议处理优化
- **报文解析零拷贝**：在 `internal/dhcpv4/message.go` / `internal/dhcpv6/message.go` 引入 `bytecursor` 结构，从 `[]byte` slice 直接引用 option，不重新分配；对需要持久化的字段再 lazy copy。
- **多线程/多进程架构**：
   - 每个绑定 CPU 的 worker goroutine 负责：接收 UDP → 解析 → 策略 → 缓存查找 → 响应；
   - 当单节点不足时，提供 `dhcpd --processes=N` 多进程模式，通过共享内存或 Redis 协调租约；
   - 通过 `internal/failover` 监控 worker 健康，防止热点倾斜。
- **UDP 报文优化**：开启 `recvmmsg/sendmmsg`（Go 1.23 的 `x/sys/unix` 支持），批量读写；在网卡侧调优 `SO_RCVBUF`、`SO_BUSY_POLL`。
- **广播/单播智能转换**：根据 Client Capabilities + Option 82 推断是否转单播；在同一交换机内的续租请求转为单播减少广播风暴，若探测失败自动回退广播。

性能路线的实施通过 `perf/benchmarks`（将新增）验证，要求部署前提供 QPS vs 延迟图、内存曲线与容量预估。

## 8. 多层安全防护
- **多维速率限制**：`security.rateLimit` Token Bucket 现支持 per port / per MAC / per IP 三级键控，灰名单客户会被强制切换到每秒 5pps 的保护配置，超限结果通过 Prometheus 与审计事件可视化。
- **DoS / 饥饿攻击识别**：`detector.SimpleDetector` 维护滑动窗口，监测 Discover 热度、Decline 激增、请求/Discover 比例倒挂等模式，命中后自动隔离 `blockDuration` 并可透传至租约服务执行 Quarantine。

### 8.2 网络层安全
- **DAI 联动**：`guard.OnLeaseChange` 通过 `ipsgdai.Publisher` 推送签名租约快照（MAC/IP/VLAN/端口/过期时间），网络控制器可以据此更新 Dynamic ARP Inspection 表项；当控制器侧发现 ARP 欺诈时，通过 `OnSecurityEvent` 回传并触发租约隔离。
- **IP Source Guard 集成**：Lease 变更同样喂给 IP-SG，将 DHCP 实例视为授权源；配置项 `security.ipsgdai.publishURL/secret` 控制推送端点与签名。

### 8.3 认证与授权
- **802.1X / EAP**：RADIUS 客户端支持与 EAP-TLS/PEAP/EAP-MD5 认证器对接。`radius.Client.Lookup` 可在 DHCP 流程中按 `MAC/ClientID` 查询 Access-Accept，解析 VLAN/ACL/QoS 属性并注入策略引擎；CoA/Disconnect 则由 `radius.COA` 监听触发租约撤销。
- **RADIUS 属性映射**：Access-Accept 中的 `Tunnel-Private-Group-ID`、`Filter-Id`、`Cisco-AVPair` 等会被转为 `policy.Input` 的 `VLANID`、`ACL`、`QoSProfile` 字段，确保租约/策略与上游 NAC 一致。
- **MAC 白/黑/灰名单**：新增 `security.macAcl` 配置，Guard 在 Snooping 之前先执行 MAC ACL。白名单模式用于零信任设备接入，黑名单直接拒绝，灰名单可选择 monitor / block / 降速等动作。
- **证书认证**：管理 API 与 RADIUS 客户端均支持 mTLS，`security.radius.servers[].sharedSecret` 仅作为后备；DHCP 节点对外暴露的 gRPC/Syslog 采集端同样启用 TLS 校验证书链，防止伪造 Snooping Feed。

多层安全策略统一在 `security_subsystem_plan.md` 维护实施路线，灰度时可通过 `security.enabled=false` 或单独关闭某子模块安全回退。

## 9. 可观测性与运维

| 指标 | 说明 |
| --- | --- |
| `modern_dhcp_ha_heartbeat_latency_seconds{peer}` | failover TCP 心跳延迟直方图 |
| `modern_dhcp_ha_link_state` | Gauge，0=down,1=up |
| `modern_dhcp_ha_failovers_total{reason}` | 自动/手动切换计数 |
| `modern_dhcp_db_replication_lag_seconds{source}` | 数据库复制延迟 |
| `modern_dhcp_config_drift_total{component}` | 配置不一致事件 |

告警建议：
- 30 秒内连续发生 3 次 failover。
- replication lag > SLA。
- 配置 checksum 不一致。

## 10. 实施路线图

1. **Milestone H1 – 主从基础**
   - 实现 failover TCP 通道、心跳、MCLT 状态机。
   - 打通租约 CDC，完成主备同步与 split-brain 仲裁。
2. **Milestone H2 – 多活 & 负载均衡**
   - 引入注册表/发现模块，支持 VIP/DNS/Anycast。
   - 完成事件驱动的最终一致性同步与滚动升级策略。
3. **Milestone H3 – 全局容灾**
   - Geo Replication、Region Failover 演练、跨洲 SLA。
   - Runbook、仪表板、自动回切策略完善。

每个里程碑均需同步更新 `README.md`、Helm/配置示例与 Runbook，并附带回归测试（模拟 failover、断网、split-brain）。
