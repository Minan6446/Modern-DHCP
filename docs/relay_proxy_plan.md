# 11. 中继与代理实施方案

本节补充系统规范中的「中继与代理」能力，拆解为可实施的服务、配置与观测项，确保 DHCP 核心能够感知并驾驭多跳中继环境、Option 82 元数据以及跨域寻址需求。

## 架构概览
- **RelayCoordinator（新）**：聚合 Option 82 解析、认证、池选择、跨域映射与负载均衡。位于 `internal/relay`，由 DHCPv4/v6 handler 注入。
- **RelayParser**：负责将 Option 82 子选项与厂商自定义 TLV 解码到统一的 `RelayContext`，填充 `circuit-id / remote-id / vlan-id / ssid / vrf / vpn-id / giaddr` 等键。
- **RelayAuthenticator**：基于配置的信任清单、HMAC/PSK 校验、重放窗口判断中继是否合法（与 Guard 协作，实现“中继身份验证”）。
- **RelaySelector**：利用 `RelayContext` + 策略/池服务选择最优地址池，优先级顺序：静态规则（giaddr/VRF/MPLS）→ 策略池 → 元数据解析 → handler fallback。
- **RelayPartitioner**：为“中继负载均衡”提供一致性哈希/加权轮询，将同一路由器/分支绑定到特定 DHCP 节点，避免重复响应；当节点非负责人时，直接丢弃并由 Guard 记录事件。

## 11.1 智能中继支持

### Option 82 解析与处理
- 默认支持 RFC 3046/5107 子选项：1(Circuit-ID)、2(Remote-ID)、5(Link Selection)、6(Subscriber-ID)、9(Vendor-Specific)、151/152(VSS/MPLS VPN)、2.x（厂商扩展）。
- 在 `config.Relay.Option82` 中定义 `subOptionMappings`，允许运营者将特定子选项映射到自定义 metadata key，以及选择 `string/int/hex` 解码方式。
- `RelayContext.Metadata` 会被传入策略引擎与 Guard，并通过 `policy.OptionMetadataBindings` 文档化；同一个键可落入 `policy.Input.Option82`、`pool.MetadataSelector`、`securityguard.Context`。

### 基于 GIAddr 的地址池选择
- `RelaySelector` 引入 `giaddrRules`：`[{ "giaddr": "10.0.0.1", "tenantId": "isp-a", "poolId": "edge-a" }, ...]`。
- 对应 `pool.MetadataSelector` 扩展字段 `GatewayIP`、`VRF`，可直接调用 `pool.Service.ResolvePool` 获取匹配的池；若无匹配，则回退到原先的接口/VLAN/SSID 逻辑。
- 在策略决策阶段新增 `relayHints` 输入，允许规则基于 `giaddr`/`relayId` 做条件判断。

### 中继负载均衡（多个中继指向多个服务器）
- `config.Relay.LoadBalancing`：`mode`(`consistent-hash`/`round-robin`)、`servers`（ID、权重、region/zone）、`hashKeys`（默认 `relayId|giaddr`）。
- DHCPd 在收到报文后调用 `Partitioner.ShouldHandle(ctx, relayId)`；若当前节点不在分配列表中则拒绝并计入 `relay_partition_drops_total`。
- 结合 `HA.NodeID` 实现确定性分片，使跨多中继的大型园区可水平扩容。

### 中继身份验证
- `config.Relay.Authentication`：
  - `allowedAgents`: GIAddr / Remote-ID / Circuit-ID 白名单。
  - `sharedSecrets`: relayId → PSK；当 Option 82 Vendor-Specific 中携带 `auth` TLV 时校验 HMAC。
  - `replayWindow`: 时间窗口，结合 nonce/timestamp 防止重放。
- 验证顺序：共享密钥 → 允许列表 → Snooping Trusted Ports；失败时向 Guard 发 `relay_auth_failure` 事件并拒绝请求。

## 11.2 跨域支持

### 跨 VLAN 分配
- 继承现有 `VLANID` 绑定能力，新增 `vlanRanges`（例如 `100-110`）与 `multiVlanPools` 配置，可让单个中继代理多个 VLAN。
- RelayParser 将 `vlan-id`、`agent.vlan-id` 映射到整数，Selector 支持按范围匹配。

### 跨三层网络分配
- 通过 `giaddr` + `vrf-id`（子选项 151/152 或 Vendor TLV）识别实际三层转发域。
- `CrossDomainConfig.VRFs` 允许将 `vrf`/`route-distinguisher` 匹配到特定地址池或策略标签，实现“一台中继分发多个 L3 网络”。

### 跨数据中心分配
- Option 82 / Vendor TLV 中的 `datacenter`、`region`、`fabric` 键将直接写入 `RelayContext`。
- `pool.MetadataSelector` 扩展 `Region`、`Zone` 字段，允许地址池声明自身所在的 DC，Selector 根据 `relay.region` 决定使用本地或远端池。

### 跨 MPLS VPN 分配
- 支持解析子选项 151(VSS Control) / 152(VSS-Bootfile) 以提取 `vpn-id`、`vrf`，并在配置中设置 `mplsVpnRules`（`vpnId` → Pool）。
- 若 `vpnId` 未命中静态配置，则回落到 `tenantId+userGroup` 组合，以保障多租户 VPN 场景。

## 配置结构（新增）
```yaml
relay:
  enabled: true
  option82:
    preserveRaw: true
    subOptionMappings:
      "1": { key: "circuit-id", format: "string" }
      "2": { key: "remote-id", format: "string" }
      "6": { key: "subscriber-id", format: "string" }
      "151": { key: "vpn-id", format: "hex" }
  authentication:
    required: true
    replayWindow: 60s
    allowedAgents:
      - relayId: "agg-core-1"
        giaddr: "10.1.0.1"
        remoteId: "POP-A"
        vlanRanges: [100-120]
    sharedSecrets:
      agg-core-1: "base64-psk"
  poolSelection:
    giaddrRules:
      - giaddr: "10.1.0.1"
        tenantId: "isp-a"
        poolId: "edge-a"
    rules:
      - match: { vpnId: "65000:100", region: "dc-a" }
        poolId: "mpls-gold"
  loadBalancing:
    mode: "consistent-hash"
    hashKeys: ["relayId", "giaddr"]
    servers:
      - id: "node-a"; weight: 2
      - id: "node-b"; weight: 1
  crossDomain:
    regionFallbacks:
      dc-a: ["edge-a", "edge-b"]
    vrfRules:
      - vrf: "Corp"
        poolId: "corp-vrf"
    mplsVpnRules:
      - vpnId: "100:1"
        poolId: "vpn-gold"
```

## 接口与集成点
- DHCPv4/6 handler 注入 `relay.Provider` 获取 `RelayContext`、`SelectPool`、`ShouldHandle`、`Authenticate` 等方法。
- `securityguard.Context` 增加 `RelayID`、`VPNID`、`VRF` 字段，全量输入检测器。
- HTTP `/api/v1/simulate` 接口新增 `relayContext` 字段，以便离线调试 giaddr/Option82 场景。

## 观测与告警
- 指标：`relay_option82_decode_errors_total`、`relay_auth_failures_total`、`relay_partition_drops_total`、`relay_pool_resolution_latency_seconds`。
- 日志：结构化字段 `relayId`, `giaddr`, `vrf`, `vpnId`, `poolId`。
- 告警：
  - `RelayAuthFailure`：超过阈值时触发。
  - `RelayPoolFallback`：规则未命中而回落默认池次数异常。
  - `CrossDomainMismatch`：中继请求的 region/vpn 与池 region 不匹配时记录。

## 实施里程碑
1. **阶段 A**：落地 `RelayParser` + `RelayContext` + 配置结构；更新 handler、policy 输入。
2. **阶段 B**：实现 `RelaySelector`（giaddr/VRF/MPLS 规则）与扩展 `pool.MetadataSelector`；补充单元测试与模拟器。
3. **阶段 C**：引入 `RelayAuthenticator`、`RelayPartitioner`，连通 Guard/HA 并暴露指标。
4. **阶段 D**：完善文档、API（simulate & config）、编写 e2e 场景用例，开启跨域特性默认值。

该方案为后续代码实现提供蓝图，确保 Section 11 的四大子项具备可验证的落地路径。