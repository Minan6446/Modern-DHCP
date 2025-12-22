# 安全子系统设计草案

## 目标
- 吸收来自 DHCP Snooping/交换机的可信绑定，确保地址分配仅面向受信端口/终端。
- 在 DHCP 处理链前执行速率限制与行为分析，缓解泛洪、饥饿、Option 82 欺骗等攻击。
- 与 IP Source Guard (IP-SG) / Dynamic ARP Inspection (DAI) 双向联动，保持网络设备的实时授权表，并根据网络反馈隔离恶意客户端。
- 集成 RADIUS/802.1X，使用 Access-Accept 属性指导策略决策，并根据 CoA/Disconnect 指令撤销租约。

## 架构概览
```
+---------------------+      +-------------------+      +-----------------+
| Snooping Ingestors  |----->| Trusted Cache     |----->| Guard Interface |
| (gRPC/CSV/syslog)   |      | (Redis + in-mem)  |      | (DHCP pipeline) |
+---------------------+      +-------------------+      +-----------------+
         |                                                   |
         v                                                   v
+---------------------+      +-------------------+      +-----------------+
| Rate Limit Engine   |<---->| Behavior Detector |----->| Metrics/Audit   |
+---------------------+      +-------------------+      +-----------------+
         |
         v
+---------------------+      +-------------------+
| IP-SG/DAI Publisher |<---->| RADIUS/802.1X Hub |
+---------------------+      +-------------------+
```

### 核心包
| 包 | 说明 | 关键接口 |
| --- | --- | --- |
| `internal/security/guard` | 对上层提供统一的 `Guard` 接口，串联 snooping 校验、速率限制、攻击检测、IP-SG/DAI 回调、RADIUS 属性处理 | `Check(ctx, *GuardContext) error`, `OnLeaseChange`, `OnSecurityEvent` |
| `internal/security/snooping` | 负责采集并维护可信绑定（端口/MAC/IP/VLAN）。支持 gRPC 订阅、CSV/JSON 周期拉取、syslog 事件。内部使用 Redis Hash + 本地 LRU 缓存。 | `Loader.Start()`, `Store.Lookup(key)`, `Store.StreamChanges()` |
| `internal/security/ratelimit` | Token Bucket/漏桶实现，支持租户、MAC、端口、IP 四维限速，带突发阈值与自适应退避 | `Limiter.Allow(ctx, key) (bool, Delay)` |
| `internal/security/detector` | 行为打分器，聚合 DHCP 报文序列（DISCOVER/OFFER/REQUEST/DECLINE），识别异常模式。可联动 Option 82/Relay 元数据比对 | `Detector.Observe(event)`, `Detector.Score(key)` |
| `internal/security/ipsgdai` | 向交换机/控制器推送签名租约更新（gRPC/Webhook），并接收 DAI 反馈。封装签名、重试、速率控制。 | `Publisher.Publish(LeaseSnapshot)`, `Callback.Handle(Alert)` |
| `internal/security/radius` | RADIUS 客户端 + CoA/Disconnect 监听。提供属性解码、缓存、策略输入转换 | `Client.Fetch(ctx, identifier)`, `CoAServer.Listen()` |

## 配置与执行流程
1. **配置结构** (`config.SecurityConfig`):
   ```yaml
   security:
     enabled: true
     snooping:
       grpcEndpoint: "collector:7443"
       refreshInterval: 30s
       redis:
         addr: "redis:6379"
         db: 3
       cacheTTL: 2m
     rateLimit:
       default:
         perMacPPS: 50
         perPortPPS: 500
         perIPPPS: 200
         burst: 20
       overrides:
         tenant-acme:
           perMacPPS: 100
     detection:
       declineSpikeThreshold: 5
       starvationWindow: 30s
       option82MismatchTolerance: 2
       discoverBurstThreshold: 40
       discoverToRequestRatio: 0.25
       blockDuration: 2m
       rogueServerAllowlist:
         - 10.99.0.1
         - 10.99.0.2
     macAcl:
       whitelist:
         - "00:11:22:33:44:55"
       blacklist:
         - "de:ad:be:ef:ff:01"
       graylist:
         - "00:aa:00:aa:00:aa"
       graylistAction: monitor
    exhaustion:
      maxLeasesPerMac: 4
      maxLeasesPerUser: 8
      poolWarnPercent: 80
      poolProtectPercent: 95
      isolation:
        temporaryDuration: 10m
        permanentAfter: 3
     ipsgdai:
       publishURL: "https://controller/api/ipsg"
       secret: "<hmac>"
       retry: 5
     radius:
       servers:
         - addr: "10.1.0.10:1812"
           sharedSecret: "secret"
           timeout: 3s
       coa:
         listen: ":3799"
   ```
2. **DHCP Handler Hook**：
   - `dhcpv4.Handler` 注入 `security.Guard`。
   - 执行顺序：
     1. 构造 `GuardContext`（tenant、port、Option 82、MAC、请求类型）。
     2. `Guard.Check`：
        - Snooping：验证 (port, MAC, VLAN) 是否受信。
        - RateLimit：评估速率，必要时返回延迟或拒绝。
        - Detector：评估行为评分，超阈值则拒绝/降级。
     3. 若通过，继续策略/租约流程。
     4. 租约状态变化后调用 `Guard.OnLeaseChange`，推送至 IP-SG/DAI/RADIUS Accounting。

## 数据流与事件
- **Snooping 更新**：
  1. `Loader` 从交换机或集中控制器拉取表项。
  2. 更新 Redis Hash，并广播 Kafka/Redis Stream 事件供其他节点同步。
  3. `Guard` 在本地缓存命中失败时回源 Redis。
- **速率限制**：
  - Key = `tenant|port|mac`。Token Bucket 数据结构存于本地（高频）+ Redis（跨节点共享）。
  - `Limiter.Allow` 返回 `Allow=false` 时触发 `security_events_total{type="ratelimit"}` 与审计事件。
- **攻击检测**：
  - `Detector` 订阅 DHCP 报文事件（轻量结构），维护滑动窗口。
  - 当检测到 DISCOVER 泛洪或 DECLINE 激增时，触发 `Guard` 决策（可自动 quarantine）。
- **IP-SG/DAI**：
  - `OnLeaseChange` 将租约快照（tenant、MAC、IP、VLAN、port、expiresAt、signature）推送给控制器。
  - 控制器反馈的违规（DAI reject）通过 Webhook 发送至 `Callback`，调用 `leaseSvc.MarkQuarantined`。
- **RADIUS/802.1X**：
  - `Client.Fetch` 在策略阶段可读取 Access-Accept 中的 VLAN/ACL/策略标签并注入 `policy.Input`。
  - `CoAServer` 接收到 Disconnect/CoA 时，通知 lease 服务释放或变更租约，并产生日志/审计。

## 威胁防护策略落地

- **Rogue 服务器检测**：`guard` 现会将 `GIAddr` 输入行为检测器，若发现来自未授权中继/服务器的报文即触发 `rogue_server` 拒绝，并记录 `security_events_total{type="detector", result="deny"}`。授权列表可通过 `detection.rogueServerAllowlist` 配置。
- **耗尽/饥饿防护**：内置检测器维护滑动窗口，默认当 `discoverBurstThreshold` 内的 REQUEST 与 DISCOVER 比例低于 `discoverToRequestRatio` 时触发 `starvation_pattern`，并将客户端拉黑 `blockDuration`。
- **恶意客户端隔离**：一旦命中上述检测或 DECLINE 激增阈值，客户端状态被标记为 `blocked`，同一窗口内的请求都会立即拒绝；窗口恢复后状态降级为 `ok`。检测结果会同步到 `lease.securityState` 字段，方便 API/控制台查询并为后续 CLI/API 解封提供依据。
- **租约数量限制**：`security.exhaustion.maxLeasesPerMac` 和 `maxLeasesPerUser` 会在 `lease.Service` 中统计当前活跃租约数量，超额即拒绝新的分配并触发审计/metrics。超限的 MAC/用户会被放入隔离名单，防止其继续消耗地址。
- **地址池保护阈值**：`security.exhaustion.poolWarnPercent` (默认 80%) 触发 Warning 日志，`poolProtectPercent` (默认 95%) 进入保护模式并拒绝新的分配，指标 `lease_events_total{action="pool_protection"}` 同步暴露，便于 SRE 自动化扩容。
- **恶意客户端隔离（临时/永久）**：`security.exhaustion.isolation` 定义隔离持续时间与永久封禁阈值。每次触发超限或异常耗尽时都会调用隔离策略，临时隔离默认 10 分钟，可配置 `permanentAfter`（例如 3 次）后转为永久封禁。

## 指标与审计
| 指标 | 说明 |
| --- | --- |
| `security_events_total{type="snooping"|"ratelimit"|"detector"|"ipsg"|"radius"}` | 安全部署触发的事件计数 |
| `security_guard_latency_seconds` | `Guard.Check` 延迟直方图 |
| `snooping_bindings_cache_hits_total` / `misses_total` | 可信绑定缓存命中情况 |
| `rate_limit_block_duration_seconds` | 速率限制施加的退避时间 |

审计事件示例：
- `security.snooping_violation`: `{ "tenantId": "t1", "mac": ..., "port": "Gi1/0/24", "reason": "untrusted_port" }`
- `security.ratelimit_block`: `{ "tenantId": "t1", "mac": ..., "pps": 800 }`
- `security.dai_alert`: `{ "tenantId": "t2", "mac": ..., "ip": "10.0.0.5", "action": "quarantine" }`

## 下一步
1. 在 `internal/security` 下建立上述包与接口，提交最小可运行骨架。（已完成：新增 Guard、Snooping、RateLimit、Detector、IP-SG/DAI、RADIUS 包，并在 DHCPv4 handler 中注入管线）。
2. 扩展配置解析与 `dhcpd` 注入逻辑，加入 feature flag 以灰度启用。（已完成：`config.SecurityConfig` 现支持 snooping/rateLimit/detection/IP-SG/DAI/RADIUS，并通过 `securityguard.NewFromConfig` 在进程启动时装配；Guard 现输出 `security_events_total`、`security_guard_latency_seconds`，Snooping Store 也会刷新 `snooping_cache_events_total` 命中/未命中数据）。
3. 编写单元测试（速率限制、snooping 查找、Detector 判定、IP-SG 发布重试、MAC ACL）。——已新增针对 Token Bucket 限流与 HTTP Publisher 签名/重试的基础测试，本轮将覆盖 per-IP 限速与 MAC ACL 灰度路径。
4. 实现真实的 snooping ingest、IP-SG/DAI 推送、RADIUS Client，并在 README/系统规范/Phase 3 计划中补充运维指南（当前提供 HTTP Publisher 与 Logging RADIUS Client 占位实现，仅负责签名+日志，尚未做高可用/回调处理）。随后补上 detector/snooping 更完善的测试矩阵，并打通 MAC 漂移告警到交换机 API 的自动化闭环。
