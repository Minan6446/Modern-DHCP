# 威胁防护与响应手册

## 9. 威胁防护

### 9.1 欺骗防护
- **虚假服务器检测**：`detector.SimpleDetector` 会对比 `GIAddr` 与 `detection.rogueServerAllowlist`，命中后直接拒绝并通知 Guard/Quarantine。
- **恶意中继检测**：Snooping 信任端口 + Relay 允许列表联合管控，未受信的中继直接在 Guard 层阻断，同时写入 `security_events_total{type="snooping"}`。
- **Option 82 欺骗防护**：Guard/Detector 将 Circuit-ID、Remote-ID 与 Snooping 绑定比对，超过 `option82MismatchTolerance` 即打入灰名单并触发速率降级。
- **MAC 地址伪造检测**：`detection.macDriftThreshold` 监测同一 MAC 在多端口漂移的行为，命中后将客户端标记为 `mac_drift` 并可触发交换机侧端口安全策略。

### 9.2 耗尽攻击防护
- **租约数量限制**：`security.exhaustion.maxLeasesPerMac/User` 在 `lease.Service` 中实时统计活跃租约，超过阈值即拒绝新分配并将终端加入隔离名单。
- **地址池保护阈值**：当池使用率达到 `poolWarnPercent`（默认 80%）时发出 Warning，超过 `poolProtectPercent`（默认 95%）则进入保护模式并拒绝新的 Discover/Request。
- **恶意客户端隔离**：`security.exhaustion.isolation` 支持临时隔离（默认 10m）与多次违规后的永久封禁，Guard 入口会优先检查隔离表以阻断请求。
- **智能速率限制**：RateLimit Token Bucket 已支持 Port/MAC/IP 多维限速，并可通过 Guard 灰名单下探将阈值降级，用于抑制 Discover Storm。

Modern-DHCP 在安全管道中加入了针对 Rogue 服务器、地址耗尽以及恶意客户端的内置检测器。本手册描述如何启用相关功能、观察指标并在需要时手动干预。

## 1. 配置与阈值

```yaml
security:
  detection:
    declineSpikeThreshold: 5          # DECLINE 激增视为异常
    starvationWindow: 30s             # 行为窗口大小
    discoverBurstThreshold: 40        # 多长的 DISCOVER 突发触发检测
    discoverToRequestRatio: 0.25      # REQUEST/ DISCOVER 低于该比例判定为饥饿攻击
    blockDuration: 2m                 # 命中威胁后隔离持续时间
    rogueServerAllowlist:             # 允许的 GIAddr/中继地址
      - 10.99.0.1
      - 10.99.0.2
```

- **Rogue 服务器检测**：只要 `GIAddr` 不在 `rogueServerAllowlist` 且字段非空，检测器即返回 `rogue_server`，DHCP 请求会被拒绝，后续窗口内的请求将继续被隔离。
- **耗尽/饥饿防护**：在 `starvationWindow` 内，若 `discoverBurstThreshold` 被达到但 `REQUEST` 数量不足（低于 `discoverToRequestRatio`），检测器会产生 `starvation_pattern`，对客户端实施拉黑直到 `blockDuration` 结束。
- **恶意客户端隔离**：`declineSpikeThreshold` 或以上两个策略被触发时，客户端状态会变为 `blocked`。状态会在窗口重置后自动恢复为 `ok`，也可以通过计划中的 CLI/API 人工解除。

## 2. 监控与告警

| 指标 | 说明 |
| --- | --- |
| `security_events_total{type="detector", result="deny"}` | 检测器触发的拒绝次数（rogue / starvation / decline_spike）。|
| `security_guard_latency_seconds{result="deny"}` | 拒绝路径的耗时，定位性能问题。|
| `rogue_dhcp_alerts_total` *(规划中)* | 结合网络嗅探/遥测的 Rogue 告警。|

推荐在 Prometheus / Alertmanager 中创建告警：

- **Rogue DHCP Server Alert**：`increase(security_events_total{type="detector",result="deny"}[5m]) > 0` 且 `reason="rogue_server"`。
- **Exhaustion Attack Alert**：`increase(security_events_total{type="detector",result="deny"}[5m]) >= 5` 且 `reason="starvation_pattern"`。

## 3. 安全状态持久化

检测器的隔离决策会通过 `guard.QuarantineSignal` 回写到 `leases_v4/leases_v6` 的 `security_state` 列（由迁移 `migrations/0003_security_state.sql` 创建，默认为 `OK`）。这使得安全事件不仅存在内存窗口，还能在 REST API、UI 或外部审计工具中查询。需要注意：

- `SUSPECT` 表示短期观察名单，可继续为客户端发放租约但会降低优先级；`BLOCKED` 会立即拒绝 DHCP 请求。
- 手动解封当前可通过 SQL 或维护脚本执行，例如：

  ```sql
  UPDATE leases_v4
     SET security_state = 'OK'
   WHERE tenant_id = 't1'
     AND hardware_addr = 'aa:bb:cc:dd:ee:ff';
  ```

- 如果集群早期版本缺少该列，需要运行 `goose up` 以应用 `0003_security_state.sql`。迁移脚本会为历史数据填充 `OK`，确保升级过程幂等。
- 管理员可通过新 API `POST /tenants/{tenantId}/leases/{leaseId}/security-state` 设置状态：

  ```http
  POST /tenants/t1/leases/lease-123/security-state
  Content-Type: application/json

  { "state": "SUSPECT" }
  ```

  仅允许 `OK` / `SUSPECT` / `BLOCKED`，并会在审计日志中记录原状态与新状态。
- Web 控制台入口：`/console/lease-security` 提供图形界面，可检索租约、按 `securityState` 过滤并触发“Mark OK / Suspect / Block”动作，无需直接调用 API。

## 4. 响应流程

1. **确认触发原因**：通过管理 API 查询最新的安全事件，或在日志中搜索 `detector rejection`。
2. **排查 Rogue**：核对 `GIAddr` 与交换机/relay 列表，若为合法设备，更新 `rogueServerAllowlist` 并重新加载配置。
3. **处理耗尽攻击**：
   - 查看 `policy/pools` 可用地址，必要时扩容或切换备用池。
   - 在交换机上定位源端口（利用 snooping 表），手动隔离或 Rate-Limit。
4. **解除隔离**：待攻击排除后，可在未来 CLI/API 中调用 `security.quarantine.clear`（计划中）。临时方案是将 `blockDuration` 调低并重启进程。

## 5. 后续路线

- 与 NAC/防火墙集成：将 `blocked` 客户端推送至外部控制器，实现二/三层隔离。
- 地址耗尽联动：`security.exhaustion` 自动对超出 `maxLeasesPerMac/User` 的终端实施隔离，同时在池使用率达到 `poolWarnPercent/poolProtectPercent` 时写入审计与指标，SRE 可据此扩容或强制保护模式。
- CLI / API：提供安全管理员查看/解除隔离、导出历史的接口。
- 与 syslog/sniffer 的集成：将主动探测的 Rogue 事件统一注入 `OnSecurityEvent`，与本检测器的输出合并。
