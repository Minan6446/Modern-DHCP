# 性能优化执行清单

> 本文将 7. 性能优化拆解为可实施的代码工作项，并给出优先级/里程碑。

## 优先级阶段
1. **Phase 0 – Baseline & Benchmarks**
  - 建立 `perf/benchmarks`，记录初始 QPS / 延迟（DHCPv4 解析、策略评估、租约写入）。
  - 为每个基准保留 `BENCHMARK.md`/Grafana 记录，提交时附上 `benchstat` 结果。
2. **Phase 1 – 连接池 + UDP 批处理（P0）**
  - 动态 DB 连接池（根据主/备角色调整）。
  - UDP `reuseport + recvmmsg` 批处理，目标是单节点 10 万+ 并发的输入带宽。
3. **Phase 2 – 缓存 + 批量写入（P1）**
  - 引入 Redis/Memcached 缓存、BatchWriter、sync.Pool 减 GC。
4. **Phase 3 – 零拷贝及协议优化（P1/P2）**
  - Packet cursor、广播/单播智能转换、多 worker 进程。
5. **Phase 4 – SLA 监控/优化闭环（P2）**
  - hdrhistogram、自动告警、持续基准验证。

## 1. 连接与并发
- [ ] **DB 连接池动态配置** (`internal/db/mysql.go`)
  - 将 `MaxOpenConns/MaxIdleConns` 暴露给 `ha.performance.dbPoolOverrides`，并根据 `failover.Manager.Role()` 在 runtime 调整。
  - **依赖**：Phase 1；需要新的配置结构、metrics（连接池耗尽）。
- [ ] **网络连接池与 netpoll** (`internal/dhcpv4/server.go`, `internal/dhcpv6/server.go`)
  - 引入 `reuseport` 监听；结合 `golang.org/x/sys/unix` 使用 `Recvmmsg`。
  - **验证**：`perf/benchmarks/dhcpv4_bench_test.go` 记录批量接收 QPS；以 P99 < 10ms 为目标。
- [ ] **优先级请求队列** (`internal/server`, `internal/dhcpv4`)
  - 新建 `pkg/prioqueue`，根据租约类型/Option82 分级。
  - 导出 `modern_dhcp_request_queue_length` 指标。

## 2. 内存/存储
- [ ] **Redis/Memcached 缓存** (`internal/lease/cache.go`)
  - 设计统一 Cache 接口，支持 multi-get；与 failover 同步。
  - **实施顺序**：优先缓存策略/池解析 → 再缓存租约；每步都需命中率指标。
- [ ] **批量写入与压缩** (`internal/lease/repository.go`)
  - 增加 `BatchWriter`，在 goroutine 中 flush。
  - 对历史字段使用 `zstd` （通过 `github.com/klauspost/compress/zstd`）。
- [ ] **连接复用**
  - 将 `lease`/`policy` 查询参数结构放入 `sync.Pool`，减少 GC。

## 3. 协议与 UDP 处理
- [ ] **零拷贝解析** (`internal/dhcpv4/message.go`, `internal/dhcpv6/message.go`)
  - 引入 `cursor` 解析器和 `PacketView`，避免 `copy`。
  - **步骤**：
    1. 添加 `PacketView` + 基准。
    2. 迁移 DHCPv4 parser。
    3. 迁移 DHCPv6 parser。
    4. 在基准中验证分配次数下降。
- [ ] **多 worker 进程**
  - CLI 参数 `--workers`、`--processes`，在 `cmd/dhcpd/main.go` fork 多进程或启动 goroutine group。
- [ ] **广播/单播转换逻辑** (`internal/dhcpv4/handler.go`)
  - 根据 Option82 + Relay 染色做决策，失败 fallback broadcast。

## 4. 可观测性 & 基准测试
- [ ] **perf/benchmarks** 目录
  - 添加 `go test -bench` + 真实 PCAP 回放脚本。
- [ ] **P99 SLA 监控**
  - 在 `metrics` 模块记录 `hdrhistogram`，并将 P50/P95/P99 上报。

每个任务都需要：
1. 设计文档或代码注释更新。
2. 单元/基准测试。
3. 仪表板或告警调整。

## 增量实施路线
| Step | 范围 | 代码触点 | 验证手段 |
| --- | --- | --- | --- |
| 0 | 建立基线 | `perf/benchmarks`, `docs/perf/BENCHMARK.md` | `go test -bench ./perf/...`, `benchstat` |
| 1 | DB 池 / UDP 批处理 | `internal/db/mysql.go`, `internal/dhcpv4/server.go`, `cmd/dhcpd/main.go` | 基准 + `mysql_pool_in_use`, `udp_batch_size` 指标 |
| 2 | Cache + Batch Writer | `internal/lease/cache.go`, `internal/lease/repository.go`, Redis config | 命中率、批量 flush 耗时、单元测试 |
| 3 | 零拷贝 + worker | `internal/dhcp{v4,v6}/message.go`, `cmd/dhcpd` worker CLI | 新/旧 parser 基准、pprof 分配 |
| 4 | SLA/监控 | `internal/metrics`, Grafana 面板 | hdrhistogram/P99 告警 |

## 24. 量化性能标准
针对 Phase 3 之后的规模化目标，新增 24.x 系列工作包来确保“10 万+ 并发客户端 / 1 万 TPS / P99 < 50ms / 1,000+ 地址池” 等硬性指标。每个子条目都需要明确代码触点、验收指标和回归基准。

### 24.1 并发能力
| 需求 | 交付项 | 代码触点 | 验收方式 |
| --- | --- | --- | --- |
| 10 万+ 并发客户端 | **PERF-24.1A UDP 多队列+reuseport**：在 `internal/dhcpv4/server.go` 与 `internal/dhcpv6/server.go` 中为每块网卡启用 `SO_REUSEPORT`，结合 `runtime.NumCPU()` 动态生成 `listenerShard`，每个 shard 内部维护 `recvmmsg` 循环与 `netpoll` 事件；`cmd/dhcpd` 新增 `--listeners`/`--rx-batch` CLI。 | 同上 + `cmd/dhcpd/main.go` | 回放 120k 并发的 PCAP（`perf/benchmarks/traffic_120k.pcap`），验证 `dhcp_rx_backlog` < 5%。 |
| 10,000+ TPS | **PERF-24.1B pipeline 并行化**：拆分报文处理为 `parse → policy → lease → respond` 四段流水线，使用无锁 `ringbuffer`（`pkg/ringqueue`）串联；对策略缓存命中路径使用 `sync.Pool` 重用 `policy.Input`；租约写入交给 `lease.WriterPool`，通过 `internal/lease/service.go` 的 `Submit` API 吸收突发流量。 | `internal/dhcpv4/handler.go`, `internal/dhcpv6/handler.go`, `internal/lease/service.go`, `pkg/ringqueue` | `go test ./perf/... -bench=Pipeline` 达到 ≥10k TPS，Grafana `dhcp_request_tps` P95≥10k。 |
| P99 < 50ms | **PERF-24.1C SLA 监控闭环**：在 `internal/metrics` 引入 HDRHistogram，记录 `dhcp_request_latency_seconds` 的 P50/P95/P99；`cmd/dhcpd` 增加 `--sla.maxP99`，当 P99>配置值时触发告警事件（`events.NewPolicyEvent`），同时在 `monitoring/dashboard` 中新增延迟热力图。 | `internal/metrics/metrics.go`, `internal/monitoring/aggregator.go`, Grafana JSON | 压测 15 分钟，`PromQL: histogram_quantile(0.99, rate(dhcp_request_latency_seconds_bucket[5m])) < 0.05`。 |
| 1,000+ 地址池 | **PERF-24.1D 池索引与缓存**：在 `internal/pool/service.go` 添加基于 `sync.Map` 的热索引，并对 `PoolSelector` 结果写入 `mobility.AffinityCache` 的派生实例；引入 `pool.MetadataDigest`，将 `1,000+` 池的匹配逻辑压缩成 Bloom 过滤器，减少锁冲突；配合 `internal/policy/engine.go` 的规则缓存失效策略。 | `internal/pool/service.go`, `internal/policy/engine.go`, `internal/mobility/affinity.go` | 通过 `perf/benchmarks/pool_select_bench_test.go` 验证 1,200 池情况下命中耗时 < 5µs，实际 TP99 维持 < 50ms。 |

#### 运行手册补充
- `docs/perf/BENCHMARK.md` 新增 “PERF-24.1” 章节，记录压测命令、TPS/延迟曲线与硬件配置。
- `deploy/kubernetes/values.yaml` 暴露 `dhcpd.listenerShards`, `dhcpd.pipelineWorkers`, `dhcpd.slaMaxP99`，确保集群与裸机一致。
- `docs/monitoring_plan.md` 添加 `dhcp_request_latency_seconds` 仪表与 Alertmanager 规则样例。

> 注：24.2（可靠性）与 24.3（可扩展性）会在后续补充表格及 Runbook，本次优先完成 24.1 的并发能力条目，并与 Phase 1/2 的代码变更保持一致，避免重复实现。

### 24.2 可靠性（Availability & Durability）
| 需求 | 交付项 | 代码触点 | 验收方式 |
| --- | --- | --- | --- |
| 99.99% 可用性（全年不可用 < 53 分钟） | **PERF-24.2A HA 遥测与 SLO 跟踪**：为 `failover.Manager` 注入 Prometheus 指标（`ha_role`, `ha_state`, `ha_peer_lag_seconds`, `ha_failover_transitions_total`），并在 `docs/monitoring_plan.md`/Grafana 面板中设定 5m SLO 视图；`cmd/dhcpd` 默认上报。 | `internal/failover/manager.go`, `internal/metrics/metrics.go`, `docs/monitoring_plan.md` | PromQL `max_over_time(ha_peer_lag_seconds[5m]) < 0.5`，并通过 Chaos/kill -9 演练捕获一次 failover 事件。 |
| 数据零丢失（HA 模式） | **PERF-24.2B 双写日志 + 冲洗确认**：在 `lease.Service` 的写路径中添加 `replication.Journal` 钩子，只有在主库与 CDC 队列同时确认后才 ACK 客户端；为 Standby 补充 `binlog catch-up` 指标。 | `internal/lease/service.go`, `internal/failover/replicator.go`, `docs/ha_architecture.md` | 故障注入时对比主/备租约表与 CDC 队列，确认零差异；`ha_replication_lag_seconds` < 0.2s。 |
| 亚秒级故障切换（<500ms） | **PERF-24.2C 快速心跳+Ingress 切换**：将默认 `heartbeatInterval` 降至 `125ms`，`failoverTimeout` 设为 `450ms`，并在 `IngressController` 中引入并行 VIP/DNS 撤销。 | `internal/failover/manager.go`, `internal/failover/ingress.go`, `configs/config.example.yaml`, `deploy/*` | 利用 tc/netem 注入主节点故障，捕捉 `ha_failover_transitions_total` 的增长并测量客户端中断时间 < 500ms。 |
| 无单点故障 | **PERF-24.2D 多协调器与探测**：允许同时配置 `etcd + consul` 或 `raft + serf`，引入 `probe` 粘性得分，在任一控制面故障时自动降级但持续服务；补齐 `docs/ha_architecture.md` 的冗余矩阵。 | `internal/failover/manager.go`, `internal/failover/coordinator.go`, `docs/ha_architecture.md`, `deploy/kubernetes/values.yaml` | 断开任一协调器或探针时，`ha_state{state="comm_interrupted"}` 至多持续 5s，且 `lease_events_total` 无下降。 |

#### 24.2 Runbook
- 监控：新增 Grafana “HA SLO” 面板，将 `ha_role`, `ha_state`, `ha_peer_lag_seconds` 与 `lease_events_total` 叠加，Alertmanager 设置 `ha_peer_lag_seconds > 0.5` 或 `ha_state="partner_down"` 超过 30s 告警。
- 配置：`configs/config.example.yaml`/Helm 值文件提供 `heartbeatInterval: 125ms`、`failoverTimeout: 450ms`、`probe.interval: 200ms` 的基线，并允许每区域覆盖。
- 演练：季度进行 kill-primary、断网、协调器下线、双节点分裂四类演练，记录 `ha_failover_transitions_total` 与业务中断时间，将结果写入 `docs/ha_architecture.md` 的附录。 

## 25. 容量规划
面向 Phase 3 后的规模扩展，建立初始容量基线，并在压测/运营中动态回填实际数据。

### 25.1 存储容量
- **租约记录**：平均 1 KB/条，100 万条≈1 GB；结合增量增长建议预留 10 GB。 
- **日志记录**：按 1 GB/天、保留 30 天计算，至少 30 GB，可结合集中式日志压缩。 
- **配置数据**：当前 <100 MB，包含多租户策略/池定义。 
- **总量建议**：以 50 GB 为起步（本地 SSD + 对象存储备份），并按月评估扩容阈值。 

### 25.2 内存需求
- **基础服务**：控制面、API、监控常驻 2 GB。 
- **租约负载**：每增加 10 万活动租约，额外分配 1 GB 用于缓存/索引。 
- **缓存策略**：依据 Option82/策略命中率动态配置 Redis/Memcached，占用独立内存资源池。 
- **规划方式**：按预期峰值租约数量加 30% buffer，结合 GC/heap profile 调整。 

### 25.3 网络带宽
- **单次请求体积**：平均 300 字节（请求+响应+元数据）。 
- **吞吐估算**：1 万 TPS ≈ 3 MB/s（≈24 Mbps），维持 3 倍冗余以承受突发。 
- **链路标准**：建议节点与上游交换机采用至少 1 Gbps 接口，万兆链路用于多活集群/镜像。 
- **峰值策略**：压测采集 P95 带宽，若连续 7 天利用率>60% 则扩容（链路聚合或多节点水平扩展）。
