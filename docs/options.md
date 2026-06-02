# DHCP 选项配置模块文档

涵盖选项管理/模板/测试相关页面组件（`web/src/views/option`）与 API（`web/src/api/options.ts`），并关联通用编辑组件。

## API 概览 — [web/src/api/options.ts](web/src/api/options.ts)
- 标准/自定义：`listStandardOptions(params?)`、`listCustomOptions(params)`、`createCustomOption(payload)`、`updateCustomOption(id,payload)`、`deleteCustomOption(id)`。
- 使用洞察：`getOptionUsage()`。
- 模板：`listOptionTemplates(params)`、`createTemplate(payload)`、`updateTemplate(id,payload)`、`deleteTemplate(id)`、`getTemplateGraph()`。
- 测试：`testOptionDelivery(payload)`（若失败可用 `utils/optionTest.simulateOptionDelivery` 兜底）。

## 页面/组件

### 选项布局 — OptionLayout
路径：web/src/views/option/OptionLayout.vue（未展开）负责路由/标签页容器。

### 标准选项库 — StandardOptions
路径：[web/src/views/option/StandardOptions.vue](web/src/views/option/StandardOptions.vue)
- 功能：浏览内置 RFC 选项；按分类/关键词过滤；查看详情面板。
- 子组件：`QuickOptionConfig`（快捷生成 3/6/15/43）、`OptionUsage`（使用统计）。

### 自定义选项管理 — CustomOptions
路径：[web/src/views/option/CustomOptions.vue](web/src/views/option/CustomOptions.vue)
- 数据：`listCustomOptions`；增删改：`createCustomOption`、`updateCustomOption`、`deleteCustomOption`。
- 功能：表格检索、分页；`OptionEditor` 弹窗创建/编辑。

### 模板管理 — TemplateManager
路径：[web/src/views/option/TemplateManager.vue](web/src/views/option/TemplateManager.vue)
- 数据：`listOptionTemplates`、`createTemplate`、`updateTemplate`、`deleteTemplate`、`getTemplateGraph`。
- 功能：
  - 模板列表 CRUD，显示继承来源与选项数。
  - 继承关系树（fallback 使用 `buildInheritanceGraph`）。
  - 文本编辑器输入 "code:value" 列表并用 `parseTemplateText` 解析；继承时子模板覆盖同码选项。

### 选项测试工具 — OptionTester
路径：[web/src/views/option/OptionTester.vue](web/src/views/option/OptionTester.vue)
- 数据：`testOptionDelivery(payload)`；失败兜底 `simulateOptionDelivery`；模板选择 `listOptionTemplates`。
- 功能：
  - 输入客户端 IP/MAC、请求码列表、模板、内联选项；运行后展示耗时、封包 Hex、下发选项与日志。
  - 右侧校验器：构造 OptionDefinition 并用 `validateOptionValue` 校验值，`HexEditor` 适配 hex 输入。

### 模板/使用可视化
- OptionUsage — [web/src/views/option/components/OptionUsage.vue](web/src/views/option/components/OptionUsage.vue)
  - 数据：`getOptionUsage()`；展示热力柱状图、引用关系表、配置历史时间线。
- QuickOptionConfig — [web/src/views/option/components/QuickOptionConfig.vue](web/src/views/option/components/QuickOptionConfig.vue)
  - 功能：生成常用 Option 3/6/15/43，包含 IP/域名/hex 校验（复用 `validateByType`/`isIPv4`）。

### 编辑组件
- OptionEditor — [web/src/views/option/components/OptionEditor.vue](web/src/views/option/components/OptionEditor.vue)
  - Props: `modelValue?: OptionDefinition`; Emit: `submit(payload)`。
  - 功能：填写代码/名称/类型/分类/长度/正则/允许值，示例值实时校验（`validateOptionValue`）；内嵌 `HexEditor`。
- HexEditor — [web/src/views/option/components/HexEditor.vue](web/src/views/option/components/HexEditor.vue)
  - Props: `modelValue`；Emit: `update:modelValue`；特性：清洗/分组显示/校验十六进制。

## 集成建议
- 表单校验：复用 `utils/validation.optionRule(definition)` 在其他配置页面保证与测试工具一致的校验逻辑。
- 模板继承：保存模板后可用 `utils/optionTemplate.resolveTemplate` 在前端合并链路用于预览/下发模拟。
- 导出/审计：选项变更可结合 `OptionUsage.history` 用于审计时间线展示。
- HEX 场景：使用 `HexEditor` 避免手写校验；提交前可调用 `normalizeValue` 统一格式。
