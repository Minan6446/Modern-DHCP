## 仪表盘模块

更新日期：2026-02-10（当前主干）

仪表盘包含系统健康、KPI、实时监控与快速洞察面板，用于总体运行态势可视化。前端主要文件：

- 布局： [web/src/views/dashboard/DashboardLayout.vue](web/src/views/dashboard/DashboardLayout.vue#L1-L32)
- 面板： [HealthOverview](web/src/views/dashboard/HealthOverview.vue#L1-L83) | [KPIPanel](web/src/views/dashboard/KPIPanel.vue#L1-L96) | [RealtimePanel](web/src/views/dashboard/RealtimePanel.vue#L1-L105) | [QuickInsights](web/src/views/dashboard/QuickInsights.vue#L1-L93)

### 数据与 API

- 当前示例数据均为前端 mock。实际接入时建议提供：
  - 健康概览：服务器在线数/总数、集群状态、可用性百分比、拓扑连通性图数据。
  - KPI：地址池利用率、活跃租约数、请求量趋势、成功率与响应时间序列。
  - 实时：`/api/v1/dashboard/streams/live` 事件/审计流（WebSocket），包含 `stream.snapshot` 与 `stream.delta`。
  - 洞察：热点池热力数据、设备类型分布、异常请求雷达、容量预测序列。
- WebSocket：RealtimePanel 使用 `useWs` 建立 WS，基址 `wss://<域名>/api/v1`。握手后处理：
  - `stream.snapshot`：携带 `items`，重置事件/告警/日志列表。
  - `stream.delta`：携带 `items`，追加增量；各列表长度>50 时截断。
  需反向代理透传 `Upgrade/Connection`，前端配置 `VITE_WS_BASE`。

### 组件说明

- DashboardLayout：两行双列栅格；包含 HealthOverview + KPIPanel、RealtimePanel + QuickInsights。
- HealthOverview：
  - 指标：服务器在线/总数、集群健康标签。
  - 图表：可用性仪表盘、拓扑力导图（节点=DHCP/Relay）。
  - 数据源：`health` 响应式对象（mock）；实际可从健康检查 API/拓扑 API 获取。
- KPIPanel：
  - 图表：地址池利用率（环图）、24h 请求量（折线）、成功率/响应时间（双轴折线+柱）；活跃租约计数。
  - 数据源：本地随机；建议接入监控时序数据库或统计 API。
- RealtimePanel：
  - 区块：事件/告警/操作日志列表（WS 推送，50 条截断）、状态流转图（ECharts graph）。
  - 行为：onMounted 建立 ws，处理 `stream.snapshot`/`stream.delta`；需配置真实 ws 地址与鉴权。
- QuickInsights：
  - 图表：热点池热力图、设备类型分布饼图、异常请求雷达、容量预测折线。
  - 数据源：示例静态；建议后端提供热点池利用率矩阵、设备类型统计、异常分类计数、容量预测序列。

### 集成要点

- 数据契约：为各图表提供结构化接口（时间序列数组或矩阵），避免在前端做随机生成。
- WS 鉴权：`useWs` 支持心跳；需传入带 token 的 url 或在 onOpen 发送认证帧；注意断线重连与最大列表长度控制。
- 自适应：BaseEChart 均启用自适应；容器需有固定高度（组件已设）。
- 性能：实时面板列表截断 50 条避免内存/渲染膨胀；ECharts 数据量大时可使用 sampling 或 dataZoom。

### 快速接入示例（伪代码）

```ts
// 拉取 KPI
const { data } = await httpClient.get('/metrics/kpi');
poolOption.series[0].data = data.utilization;
reqTrendOption.series[0].data = data.req24h;

// 建立 WS
const ws = useWs({ url: `wss://api.example.com/stream?token=${token}` });
ws.onMessage(handleMsg);
```
