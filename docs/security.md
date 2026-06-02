# 安全与防护模块文档

涵盖前端安全/防护页面组件（`web/src/views/security`）与相关 API（`web/src/api/security.ts`）。

## API 概览 — [web/src/api/security.ts](web/src/api/security.ts)
- Snooping：`listTrustPorts()`、`saveTrustPorts(ports)`、`listSnoopingBindings(params)`、`saveViolationPolicy(policy)`。
- 速率限制：`listRateLimits(params)`、`saveRateLimit(payload)`、`deleteRateLimit(id)`。
- Rogue 服务器：`listRogueServers()`、`quarantineRogue(id)`。
- ARP/源防护：`getDAIConfig()`、`updateDAIConfig(payload)`、`getSourceGuard()`、`updateSourceGuard(payload)`。
- 端口安全：`listPortProfiles()`、`savePortProfile(payload)`。
- 认证集成：`listDot1xProfiles()`、`saveDot1xProfile(payload)`、`listMacAuthProfiles()`、`saveMacAuthProfile(payload)`、`listRadiusMappings()`、`saveRadiusMapping(payload)`、`deleteRadiusMapping(id)`。
- 威胁/拓扑：`listThreatEvents()`、`getTopology()`。

## 页面/组件

### 安全总览 — SecurityLayout
路径：[web/src/views/security/SecurityLayout.vue](web/src/views/security/SecurityLayout.vue)；通过标签页在接入安全、网络防护、认证与授权、威胁中心之间切换。

### 接入安全 — AccessSecurity
路径：[web/src/views/security/AccessSecurity.vue](web/src/views/security/AccessSecurity.vue)
- 子组件：`SnoopingConfig`、`RateLimiter`、`RogueServerPanel`。

#### SnoopingConfig — DHCP Snooping 配置
路径：[web/src/views/security/components/SnoopingConfig.vue](web/src/views/security/components/SnoopingConfig.vue)
- 数据：`listTrustPorts`、`listSnoopingBindings`；保存：`saveTrustPorts`、`saveViolationPolicy`。
- 功能：信任端口开关与限速、绑定表分页查看、违规策略（丢弃/关断/告警/限速，含告警渠道、封禁时长）。

#### RateLimiter — 速率限制
路径：[web/src/views/security/components/RateLimiter.vue](web/src/views/security/components/RateLimiter.vue)
- 数据：`listRateLimits`；保存/删除：`saveRateLimit`、`deleteRateLimit`。
- 功能：端口/MAC/动态策略分栏展示；新增规则弹窗，支持范围、PPS、突发、VLAN、状态配置。

#### RogueServerPanel — 非法服务器检测
路径：[web/src/views/security/components/RogueServerPanel.vue](web/src/views/security/components/RogueServerPanel.vue)
- 数据：`listRogueServers`；隔离：`quarantineRogue`。
- 展示：疑似记录表、攻击模式雷达图；提供隔离/阻断操作位（阻断为占位）。

### 网络防护 — ProtectionCenter
路径：[web/src/views/security/ProtectionCenter.vue](web/src/views/security/ProtectionCenter.vue)
- 子组件：`ProtectionPolicies`。

#### ProtectionPolicies — DAI/Source Guard/端口安全
路径：[web/src/views/security/components/ProtectionPolicies.vue](web/src/views/security/components/ProtectionPolicies.vue)
- 数据：`getDAIConfig`/`updateDAIConfig`，`getSourceGuard`/`updateSourceGuard`，`listPortProfiles`。
- 功能：
  - DAI：启用、MAC/IP 校验、PPS 限制。
  - Source Guard：默认动作、例外列表。
  - 端口安全：最大 MAC、粘性、违规关断等只读表。

### 认证与授权 — AuthCenter
路径：[web/src/views/security/AuthCenter.vue](web/src/views/security/AuthCenter.vue)
- 子组件：`AuthIntegration`。

#### AuthIntegration — 802.1x / MAC Auth / RADIUS 映射
路径：[web/src/views/security/components/AuthIntegration.vue](web/src/views/security/components/AuthIntegration.vue)
- 数据：`listDot1xProfiles`、`listMacAuthProfiles`、`listRadiusMappings`；保存：`saveDot1xProfile`、`saveMacAuthProfile`、`saveRadiusMapping`、`deleteRadiusMapping`。
- 功能：展示并保存各认证配置；支持添加/删除 Radius 映射行。

### 威胁中心 — ThreatCenter
路径：[web/src/views/security/ThreatCenter.vue](web/src/views/security/ThreatCenter.vue)
- 子组件：`ThreatMonitor`、`TopologyView`。

#### ThreatMonitor — 威胁防护监控
路径：[web/src/views/security/components/ThreatMonitor.vue](web/src/views/security/components/ThreatMonitor.vue)
- 数据：`listThreatEvents`；分析：`scoreThreat`、`analyzeAttackPattern`（来自 `utils/securityAnalysis`）。
- 功能：威胁事件表、趋势折线、Top 攻击模式标签；`useIntervalFn` 支持 5s 自动刷新，开关 `autoProtect` 控制。

#### TopologyView — 拓扑可视化
路径：[web/src/views/security/components/TopologyView.vue](web/src/views/security/components/TopologyView.vue)
- 数据：`getTopology`；渲染：`buildTopologyOption`（ECharts graph）。
- 功能：显示核心/汇聚/接入/服务器/rogue 节点与链路状态，支持刷新。

### 安全态势 — ThreatCenter 入口页面
同上，组合 ThreatMonitor 与 TopologyView；布局在 [web/src/views/security/ThreatCenter.vue](web/src/views/security/ThreatCenter.vue)。

## 集成建议
- 权限控制：对“新增规则”“保存配置”等按钮可绑定 `v-permission`（如 `security:write`）。
- 异常占位：组件已在 catch 分支提供示例数据，便于无后端时的静态联调。
- 轮询/推送：ThreatMonitor 默认 5s 刷新，可接入 WebSocket 替换或补充；防护策略更新后可触发告警/拓扑刷新保持一致性。
- 配置落盘：保存操作可结合全局通知/日志（如审计）记录变更。
