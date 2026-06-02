# DHCP 前端专用组件文档

更新日期：2026-02-10（当前主干）

涵盖 DHCP 相关的核心组件及其 API/交互约定，便于集成、复用与测试。组件路径以 `web/src/views/...` 为准。

## 子网向导 — SubnetWizard
路径：web/src/views/pool/components/SubnetWizard.vue

- Props
  - `modelValue: boolean` 抽屉显示控制。
  - `value: SubnetDraft | null` 编辑时传入原始草稿。
- Emits
  - `update:modelValue(boolean)` 控制显示隐藏。
  - `submit(SubnetDraft)` 完成四步后提交草稿。
- 功能要点
  - 4 步流程：基础信息 → 网络配置 → 策略参数 → 关联信息。
  - 内置 IPv4 计算与去重：`calcIPv4Range` 显示首尾主机与容量；`validateRangeWithin` 过滤排除/预留；`dedupeIps` 去重。
  - 必填校验：名称、CIDR 为空时阻止提交。
- 使用示例
```vue
<SubnetWizard v-model="drawer" :value="editing" @submit="saveSubnet" />
```

## 前缀委派 — PrefixDelegation
路径：web/src/views/pool/components/PrefixDelegation.vue

- Props: `list: Prefix[]`
- Emits: `add(prefix: string)`, `split(prefix: Prefix)`
- 功能：展示现有委派表格，校验 IPv6 前缀后触发新增；支持对行执行分割操作。
- 依赖：`validateIPv6Prefix` 校验格式。

## 前缀分割工具 — PrefixSplitTool
路径：web/src/views/pool/components/PrefixSplitTool.vue

- 内部状态：`parent` 父前缀、`length` 目标长度、`results` 结果列表。
- 行为：校验父前缀与长度关系，示例生成 4 个子前缀供复制/参考（演示版，可扩展为真实算法）。

## 分配策略面板 — AllocationStrategy
路径：web/src/views/pool/components/AllocationStrategy.vue

- Props
  - `modelValue: AllocationStrategy`
  - `excludeList: string[]` 批量排除地址。
- Emits
  - `update:modelValue(AllocationStrategy)`
  - `update:excludeList(string[])`
- 功能要点
  - 配置分配算法与“优先预留”开关。
  - 批量导入/导出排除地址：`parseCsv` 读取、`toCsv` + `downloadCsv` 导出；`dedupeIps` 去重。
  - 清空/逐条移除排除列表。

## 地址池监控 — PoolMonitor
路径：web/src/views/pool/components/PoolMonitor.vue

- Props: `poolId: string`
- 行为：挂载后并行请求池使用率、历史、冲突数据（`getUsage`/`getHistory`/`getConflicts`）。
- 展示：
  - ECharts 折线图显示已用/容量曲线。
  - 历史表格（时间/动作/操作者）。
  - 冲突时间线；无冲突时展示成功提示。
  - 容量预警滑块与通知开关（本地状态，可对接告警模块）。

## 选项编辑器 — OptionEditor
路径：web/src/views/option/components/OptionEditor.vue

- Props: `modelValue?: OptionDefinition`
- Emits: `submit(payload: Partial<OptionDefinition>)`
- 功能要点
  - 支持代码、名称、类型、分类、长度范围、允许值、正则、自定义示例值。
  - 示例值实时校验：`validateOptionValue` 生成 OptionDefinition 后校验，并通过表单校验器反馈。
  - `HexEditor` 内嵌用于 hex 类型输入。
- 规则：内部使用必填与自定义 validator；`defFromForm()` 构建提交载荷。

## 十六进制编辑器 — HexEditor
路径：web/src/views/option/components/HexEditor.vue

- Props: `modelValue: string`
- Emits: `update:modelValue(string)`
- 行为：
  - 仅保留 0-9A-F，自动大写；展示分组视图与字节数。
  - `format()` 将分组去空格回写；非法输入时显示错误提示。

## 绑定表单 — BindingForm
路径：web/src/views/binding/components/BindingForm.vue

- Props
  - `modelValue: boolean` 抽屉开关。
  - `value: Binding | null` 现有绑定。
  - `conflictChecker?: (ip: string) => Promise<boolean>` 可选 IP 冲突检查器。
- Emits
  - `update:modelValue(boolean)`
  - `submit(Partial<Binding>)`
- 功能要点
  - MAC 输入自动格式化与校验；若格式错误显示错误消息。
  - IP 失焦触发可选冲突检查，显示警告提示。
  - 选项键值对动态增删。

## 批量导入绑定 — BatchImport
路径：web/src/views/binding/components/BatchImport.vue

- 功能：下载 CSV/Excel 模板（提示 Excel 需另存）、上传 CSV，解析为预览并逐行校验。
- 校验：`isMac`/`formatMac` 处理 MAC，IP 为空则标记错误；预览表格展示结果。
- 提交：过滤有效行，调用 `batchImportBindings`，弹出成功/失败统计。

## 租约筛选 — LeaseFilters
路径：web/src/views/lease/components/LeaseFilters.vue

- Props: `modelValue: LeaseFilter`
- Emits: `update:modelValue(LeaseFilter)`, `search`
- 功能：IP/MAC/ClientID/Pool/状态过滤；支持保存/加载本地筛选条件（localStorage）。重置时清空模型并触发查询。

## 快速使用指引
- 表单类组件（SubnetWizard、AllocationStrategy、BindingForm、LeaseFilters）均使用 `v-model` 双向绑定，与 emits 同步外部状态。
- 校验与格式化已内置基础逻辑；如需更强校验，可组合 `@/utils/validation` 自定义规则。
- 数据获取型组件（PoolMonitor）需要提供有效 `poolId` 并保证相关 API 可用。
- Hex 类型输入场景优先复用 `HexEditor`，避免手写校验。
