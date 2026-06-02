# 静态绑定管理模块文档

涵盖静态绑定（MAC/IP/Client ID）管理相关 API 与前端组件。API 位于 `web/src/api/bindings.ts`，组件主要位于 `web/src/views/binding/components`。

## API 概览 — [web/src/api/bindings.ts](web/src/api/bindings.ts)
- `listBindings(params: PageQuery & BindingFilter)`: 分页查询绑定。
- `getBinding(id)`: 获取单条绑定详情。
- `createBinding(payload)` / `updateBinding(id, payload)` / `deleteBinding(id)`: CRUD。
- `batchImportBindings(items)`: 批量导入，返回成功/失败计数与错误列表。
- `batchDeleteBindings(filter)`: 按过滤条件批量删除。
- `getConflicts(params)`: 检测绑定冲突，返回冲突列表与数量。

建议：
- 批量导入前先前端校验 IP/MAC；导入成功后触发列表刷新。
- 删除/批删前可调用 `getConflicts` 预估影响。

## 页面与组件

### 绑定布局 — [web/src/views/binding/BindingLayout.vue](web/src/views/binding/BindingLayout.vue)
- 通过标签页切换绑定类型与批量工具（占位导向至具体子页面/组件）。

### 绑定表单 — [web/src/views/binding/components/BindingForm.vue](web/src/views/binding/components/BindingForm.vue)
- Props:
  - `modelValue: boolean` 抽屉开关。
  - `value: Binding | null` 当前编辑数据。
  - `conflictChecker?: (ip: string) => Promise<boolean>` 可选 IP 冲突检查。
- Emits: `update:modelValue(boolean)`, `submit(Partial<Binding>)`。
- 行为：
  - MAC 自动格式化/校验（`utils/mac`）；错误时展示提示。
  - IP 失焦可触发冲突检查并提示。
  - 支持动态添加/移除 DHCP 选项键值对。

### 批量导入 — [web/src/views/binding/components/BatchImport.vue](web/src/views/binding/components/BatchImport.vue)
- 功能：
  - 下载 CSV/Excel 模板（Excel 以 CSV 另存为提示）。
  - 上传 CSV -> `parseCsv` 解析，逐行校验 MAC/IP，生成预览表；显示进度。
  - 提交时过滤有效行，调用 `batchImportBindings`，提示成功/失败数量。
- 依赖：`utils/csv`、`utils/mac`；调用 `batchImportBindings` API。

### 绑定监控 — [web/src/views/binding/components/BindingMonitor.vue](web/src/views/binding/components/BindingMonitor.vue)
- 功能：展示最近绑定状态（MAC/IP/状态/租约关联）；带刷新按钮（防抖）。
- 数据源：`listBindings({ page:1, pageSize:20 })`。

## 集成建议
- 表单校验：可复用 `utils/validation`（如 `macRule`、`ipRule`）在新增/编辑页面减少重复逻辑。
- 冲突处理：在保存前通过 `getConflicts` 或组件 `conflictChecker` 实现前置校验，避免下发失败。
- 批量导入：导入成功后触发列表/监控刷新；失败记录可与通知/审计联动。
- 性能：列表/监控请求可统一走 `useDebounceFn` 或轮询，以降低接口压力并保持实时性。
