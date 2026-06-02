# Web 工具函数库 API/使用文档

面向 DHCP 前端的常用工具函数与组合式模块，涵盖 IP/MAC/时间、校验、CSV/导出、WebSocket、监控指标、选项模板处理与安全分析等。路径均位于 `web/src/utils/`。

## IP 工具（ip.ts）
- `isIPv4(ip)` / `isIPv6(ip)`: 校验格式。
- `formatIPv4(ip)` / `formatIPv6(ip)`: 合法时返回规范化字符串。
- `ipv4ToInt(ip)` / `intToIpv4(num)`: IPv4 与整型转换。
- `cidrMask(prefix)`: 生成掩码整数。
- `calcIPv4Range(cidr)`: 计算网段范围与主机容量。
- `validateRangeWithin(cidr, ip)`: IP 是否落在网段内。
- `dedupeIps(ips)`: 去重并清洗空白。
- `validateIPv6Prefix(prefix)`: 校验 IPv6 前缀表达。
- `hasConflict(ips)`: 是否存在重复 IP。
- `summarizeSubnets(cidrs)`: 汇总子网数量与总主机数。

使用示例：
```ts
import { calcIPv4Range, hasConflict } from '@/utils/ip';
const range = calcIPv4Range('10.0.0.0/24');
const duplicated = hasConflict(['10.0.0.2', '10.0.0.2']);
```

## MAC 工具（mac.ts）
- `isMac(mac)`: 校验 MAC。
- `formatMac(mac, separator=':')`: 规范化 MAC。
- `normalizeMac(mac)`: 冒号分隔标准格式。
- `conflictMac(macs)`: 检测重复 MAC。

## 时间工具（time.ts）
- `formatTs(ts?)`: 格式化时间。
- `calcT1T2(leaseTime)`: DHCP T1/T2 计算。
- `timeLeft(ts?)`: 剩余时间友好显示。
- `formatRelative(ts)`: 相对时间文本。
- `formatRange(from, to)`: 范围显示。
- `isHoliday(ts)`: 是否周末。
- `nextBusinessDay(ts)`: 下一个工作日 ISO 字符串。

## 校验与选项工具
- `validation.ts`（Element Plus 规则）：`requiredRule`、`ipRule`、`ipv6Rule`、`macRule`、`optionRule(def)`、`lengthRule`、`numberRangeRule`、`businessRule`。
- `optionValidator.ts`: 基于 `OptionDefinition` 的值校验/规范化
  - `validateByType(type, value)`
  - `validateOptionValue(definition, value)`
  - `normalizeValue(definition, value)`
  - `maskClientId(mac)`

## CSV/导出
- `csv.ts`: `parseCsv(text)`、`toCsv(rows)`、`downloadCsv(filename, content)`。
- `exporter.ts`: `exportBlob(blob, filename)` 通用下载；`exportCsvRows(rows, filename)` 便捷导出 CSV。

## WebSocket 组合式 — useWs（ws.ts）
- Options: `url`、`protocols?`、`heartbeatMs?`、`reconnect?`(default true)、`maxRetry?`(default 5)。
- 返回：`state`(`idle|connecting|open|closed|error`)、`send(data)`、`close()`、`onMessage(handler)`。
- 特性：指数退避重连；心跳定时发送 `{type:'ping'}`；调用 `close()` 将关闭并禁用重连。

## 监控工具（monitoring.ts）
- `rollingAppend(series, point, max=60)`: 维护固定长度序列。
- `toMetricPoint(value)`: 构造带时间戳的点。
- `normalizeSnapshot(partial)`: 填充缺省的性能快照。

## 选项模板工具（optionTemplate.ts）
- `buildInheritanceGraph(templates)`: 转换为图节点。
- `resolveTemplate(templates, id)`: 递归合并继承链，后者覆盖父级。
- `diffTemplates(base, next)`: 计算新增/移除/变更的 `OptionAssignment`。
- `parseTemplateText(text)`: 解析 "code:value" 文本为 assignments。

## 选项测试工具（optionTest.ts）
- `simulateOptionDelivery(request)`: 模拟 DHCP OFFER 选项下发，返回 `OptionTestResult`（包含 offeredOptions/rawPacketHex/logs/latencyMs）。

## 安全分析工具（securityAnalysis.ts）
- `scoreThreat(event)`: 基于事件类型/端口加权得分。
- `analyzeAttackPattern(events)`: 统计类型 Top3。
- `buildTopologyOption(nodes, links)`: 生成 ECharts graph 配置，按节点状态着色。

## 在组件中的常见使用模式
- 表单校验：在 `el-form` 的 rules 中复用 `validation.ts` 与 `optionValidator`，减少自定义闭包。
- 批量导入/导出：`parseCsv` + `downloadCsv` 处理模板与预览；`exportBlob` 处理后端返回文件。
- 实时监控：结合 `useWs` 订阅事件，配合 `rollingAppend` 维护折线图数据。
- 选项配置：`resolveTemplate` 展示继承后配置；`diffTemplates` 用于变更审计或对比。
