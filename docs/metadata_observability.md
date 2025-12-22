# 地址池元数据可观测性约定

该约定已经与可观测性及网络运维干系人评审通过。他们的要求包括：租户标签保持简短（`tenant` 而非 `tenantId`）、延迟直方图遵循 Prometheus 命名规则（追加 `_seconds` 后缀）、在缓存上线前暂缓为元数据仪表执行后台全量扫描。下表已体现这些调整并获得签字确认。

## 目标
- 统计地址池选择器的解析频次，以及最常见的元数据组合。
- 暴露解析结果的上下文（scope、VLAN、interface、SSID、location），让 Grafana 面板无需调用管理 API 即可按任意维度透视。
- 提供轻量级健康信号（成功/失败计数、最近一次更新的时间跨度），便于 Prometheus 抓取且不增加控制平面压力。
- 与新的审计结构保持字段一致，使日志、指标与 UI 面板能够共享相同字段名。

## 指标提案
| 指标 | 类型 | 标签 | 更新频率 | 说明 |
| --- | --- | --- | --- | --- |
| `modern_dhcp_pool_selector_resolutions_total` | Counter | `tenant`, `scope`, `vlan`, `interface`, `ssid`, `location`, `result`（`resolved` / `miss`） | 每次解析尝试递增（包括 DHCP 处理器与 `/pools/resolve`）。 | 若有解析结果则使用地址池元数据，否则回退到选择器输入。 |
| `modern_dhcp_pool_selector_resolution_seconds` | Histogram | `tenant`, `result` | 每次执行元数据选择器时，将 `poolSvc.ResolvePool` 的耗时写入直方图。 | 桶：0.001、0.005、0.01、0.025、0.05、0.1、0.25。 |
| `modern_dhcp_pool_metadata_snapshot` | Gauge | `tenant`, `poolId`, `scope`, `vlan`, `interface`, `ssid`, `location` | 地址池参与解析时即刻更新，指针值固定为 `1`。 | 池级缓存落地后，SRE 将重新评估是否需要定期抓取。 |
| `modern_dhcp_pool_selector_errors_total` | Counter | `tenant`, `reason`（`not_found`、`validation`、`repository`） | 每当解析失败且无法返回地址池时递增。 | 作为告警信号（例如 5 分钟内快速上涨 → 告警）。 |

## 可视化约定
- **Selector Outcome Board（Grafana）**
  - 展示 `modern_dhcp_pool_selector_resolutions_total` 的堆叠面积图，并按 `result` 拆分以突显失败峰值。
  - 通过相同标签构建 VLAN/interface/SSID 维度的 Top-N 表格，定位热点。
- **Latency + Error SLO Panel**
  - 针对每个租户，基于 `resolution_latency_ms`（由 histogram 派生）绘制延迟百分位（P50/P95/P99）。
  - `pool_selector_errors_total` 在最近 5 分钟的单值图，并结合阈值标记。
- **Coverage Matrix**
  - 由 `pool_metadata_snapshot` 驱动的表格，运维可按站点、VLAN 或接口过滤，并可与 `/console/policy-selector` 中的审计轨迹联动校验实时行为。若某单元 24 小时未更新，则在仪表停止刷新后显示漂移标记。
- **Drift Watch**
  - 将审计载荷流（日志采集）与仪表指标进行关联，标记引用已删除或元数据陈旧地址池的选择器。仪表板会对超过 24 小时的条目标红。

## 验证计划
1. **Stakeholder Review**：向可观测性团队讲解此约定（指标、标签、采样频率），确保满足看板与待办需求。目标受众：NOC 负责人 + DHCP SRE。
2. **Sample Dashboards**：使用合成数据（Prometheus 回放或 JSON 模型）制作 Grafana 示例面板，验证标签设计支持所需下钻。
3. **Feedback Loop**：2025 年 12 月 2 日已与 NOC + Observability 完成，调整内容已体现在上表。
4. **Sign-Off**：✅（记录于 Jira 工单 NET-4821）。

在完成审批后，`internal/metrics` 中的 Prometheus 采集器已与现有 HTTP/策略指标一同输出这些计数器、直方图与仪表。
