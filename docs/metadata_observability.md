# 元数据观测策略

更新日期：2026-02-10（当前主干）

本文件阐述如何在监控与日志中携带池/接口/位置等元数据，便于定位问题与容量规划。

## 目标
- 将池、VLAN、接口、SSID、Location 等信息作为标签输出到指标与日志。
- 支持按维度聚合：热点池、热点接口、区域性故障、特定策略命中。

## 标签建议
- 地址池：`pool`, `pool_scope`（global/subnet/vlan/port/ssid/location）
- 网络属性：`vlan`, `interface`, `location`, `ssid`
- 协议阶段：`phase`（discover/offer/request/ack/renew/rebind/release/decline）、`family`（v4/v6）
- 策略：`policy_id`, `policy_action`

## 指标携带
- DHCP 请求计数/耗时：携带 `pool, family, phase`。
- 租约失败计数：携带 `pool, reason`。
- 池容量：携带 `pool_scope, vlan/location`。
- 安全事件：`interface, vlan, event_type`。

## 日志与审计
- 在结构化日志中附带 `request_id, pool, interface, vlan, location`，并记录策略决策与告警抑制信息。
- 审计记录操作对象的元数据（池/租约/策略），便于追踪变更。

## 数据保真与采样
- 高频事件（如 Discover）可抽样记录日志，但指标应完整；抽样率在配置中可调。
- 安全事件避免过度抽样，改用聚合计数和速率阈值。

## 排错示例
- 按 VLAN 聚合租约失败率，定位交换机或 AP 侧问题。
- 按 Location 聚合池容量，识别区域性容量瓶颈。
- 按策略 ID 聚合拒绝/放行比率，验证策略更新影响。
