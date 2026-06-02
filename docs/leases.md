# 租约管理模块文档

涵盖租约查询、历史、详情等前端组件（`web/src/views/lease`）及相关 API（`web/src/api/leases.ts`）。

## API 概览 — [web/src/api/leases.ts](web/src/api/leases.ts)
- 查询：`listActiveLeases(params)`、`listHistoryLeases(params)`。
- 事件：`getLeaseEvents(leaseId)`。
- 操作：`releaseLease(leaseId)`、`renewLease(leaseId)`。
- 导出：`exportLeases(params)`（支持 `csv|xlsx|pdf`，`responseType: 'blob'`）。
- 租期来源：租约实际时长由地址池/子网的 `leaseProfileId` 映射到 `lease_profiles.default_duration`。

## 页面/组件

### 租约布局 — LeaseLayout
路径：web/src/views/lease/LeaseLayout.vue（未展开）负责标签页容器。

### 活跃租约 — ActiveLeases
路径：[web/src/views/lease/ActiveLeases.vue](web/src/views/lease/ActiveLeases.vue)
- 功能：
  - 过滤：`LeaseFilters` 复用；表格支持选择、双击查看详情。
  - 操作：单条续期 (`renewLease`)、释放 (`releaseLease`)、批量释放；导出 CSV/XLSX/PDF（`exportLeases` + `exportBlob`).
  - 实时：接入 WS `wss://example.com/leases/active`，收到 `lease-update` 消息时防抖刷新。
- 依赖：`listActiveLeases`、`renewLease`、`releaseLease`、`exportLeases`、`LeaseDetail`。

### 历史租约 — LeaseHistory
路径：[web/src/views/lease/LeaseHistory.vue](web/src/views/lease/LeaseHistory.vue)
- 功能：时间范围过滤（`datetimerange`），分页查询历史租约。
- 依赖：`listHistoryLeases`。

### 过滤组件 — LeaseFilters
路径：[web/src/views/lease/components/LeaseFilters.vue](web/src/views/lease/components/LeaseFilters.vue)
- Props: `modelValue: LeaseFilter`; Emits: `update:modelValue`, `search`。
- 功能：IP/MAC/ClientID/Pool/状态筛选；保存/加载本地条件（localStorage）；重置清空后触发查询。

### 详情抽屉 — LeaseDetail
路径：[web/src/views/lease/components/LeaseDetail.vue](web/src/views/lease/components/LeaseDetail.vue)
- Props: `modelValue: boolean`, `lease: Lease | null`；Emit: `update:modelValue`。
- 功能：显示租约字段、状态标签、T1/T2、时间线；加载事件 `getLeaseEvents(leaseId)`。

## 集成建议
- 过滤同步：在父级使用 `v-model` 与 `LeaseFilters` 保持 store/路由 query 同步，便于刷新或分享链接。
- 批量操作：批量释放前可提示确认；长耗时操作考虑加入进度/结果 toast。
- 导出：对 `exportLeases` 的 Blob 结果使用 `exportBlob` 或 `downloadCsv`；可在请求参数附上当前 filters 保证一致性。
- 实时更新：WS 不可用时回退为定时轮询 `listActiveLeases`，并复用防抖函数降低频率。
- 租期调整：在地址池/子网调整 `leaseProfileId` 后，新分配租约将按对应 profile 生效。
