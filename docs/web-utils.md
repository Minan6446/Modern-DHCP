# Web 工具函数与表单组件文档

本文档覆盖近期新增/增强的前端工具模块：IP 工具、MAC 工具、时间工具、表单校验规则。包含 API 说明与在组件中的使用示例。

## 目录
- IP 工具（web/src/utils/ip.ts）
- MAC 工具（web/src/utils/mac.ts）
- 时间工具（web/src/utils/time.ts）
- 校验规则（web/src/utils/validation.ts）
- 组件示例：与 Element Plus 表单联动

## IP 工具
文件：web/src/utils/ip.ts

| 函数 | 说明 | 参数 | 返回 |
| --- | --- | --- | --- |
| isIPv4(ip) | 校验 IPv4 格式 | ip:string | boolean |
| isIPv6(ip) | 校验 IPv6 格式 | ip:string | boolean |
| formatIPv4(ip) | 合法时返回去空格 IPv4 | ip:string | string |
| formatIPv6(ip) | 合法时返回小写 IPv6 | ip:string | string |
| ipv4ToInt(ip) | IPv4 转无符号整型 | ip:string | number |
| intToIpv4(num) | 整型转 IPv4 | num:number | string |
| cidrMask(prefix) | 根据前缀生成掩码 | prefix:number | number |
| calcIPv4Range(cidr) | 计算网段范围、广播、主机数 | cidr:string | {network,broadcast,firstHost,lastHost,hostCount}\|null |
| validateRangeWithin(cidr, ip) | IP 是否落在网段内 | cidr:string, ip:string | boolean |
| dedupeIps(ips) | 去重/清洗 IP 列表 | ips:string[] | string[] |
| validateIPv6Prefix(prefix) | 校验 IPv6 前缀表达 | prefix:string | boolean |
| hasConflict(ips) | IP 是否存在重复 | ips:string[] | boolean |
| summarizeSubnets(cidrs) | 汇总网段数量与总主机数 | cidrs:string[] | {count:number,totalHosts:number} |

示例：
```ts
import { calcIPv4Range, hasConflict } from '@/utils/ip';

const range = calcIPv4Range('10.0.0.0/24');
const conflict = hasConflict(['10.0.0.2', '10.0.0.2']);
```

## MAC 工具
文件：web/src/utils/mac.ts

| 函数 | 说明 | 参数 | 返回 |
| --- | --- | --- | --- |
| isMac(mac) | 校验 MAC 格式 | mac:string | boolean |
| formatMac(mac, separator=':') | 规范化 MAC，指定分隔符 | mac:string, separator?:string | string |
| normalizeMac(mac) | 使用冒号分隔的标准化输出 | mac:string | string |
| conflictMac(macs) | 检测 MAC 是否重复 | macs:string[] | boolean |

示例：
```ts
import { formatMac, conflictMac } from '@/utils/mac';

const normalized = formatMac('aa-bb-cc-dd-ee-ff');
const hasDup = conflictMac(['AA:BB:CC:DD:EE:FF', 'aa-bb-cc-dd-ee-ff']);
```

## 时间工具
文件：web/src/utils/time.ts

| 函数 | 说明 | 参数 | 返回 |
| --- | --- | --- | --- |
| formatTs(ts?) | 时间戳/ISO 转标准格式 | ts?:string | string |
| calcT1T2(leaseTime) | DHCP 租约计算 T1/T2 | leaseTime:number | {t1:number,t2:number} |
| timeLeft(ts?) | 距离 ts 剩余时间（s/m/h 或 expired） | ts?:string | string |
| formatRelative(ts) | 相对时间文本 | ts:string | string |
| formatRange(from, to) | 时间范围格式化 | from:string, to:string | string |
| isHoliday(ts) | 是否周末 | ts:string | boolean |
| nextBusinessDay(ts) | 获取下一个工作日 ISO 字符串 | ts:string | string |

示例：
```ts
import { timeLeft, nextBusinessDay } from '@/utils/time';

const remaining = timeLeft('2026-02-01T12:00:00Z');
const nextBiz = nextBusinessDay('2026-01-03T00:00:00Z');
```

## 校验规则
文件：web/src/utils/validation.ts

依赖 Element Plus `FormItemRule` 与选项定义类型。

| 规则/函数 | 说明 | 参数 | 触发 |
| --- | --- | --- | --- |
| requiredRule(message?) | 必填校验 | message?:string | blur |
| ipRule(message?) | IPv4 校验 | message?:string | blur |
| ipv6Rule(message?) | IPv6 校验 | message?:string | blur |
| macRule(message?) | MAC 校验 | message?:string | blur |
| optionRule(definition) | 按 OptionDefinition 校验值/范围/枚举 | definition:OptionDefinition | blur |
| lengthRule(min,max,message?) | 字符串长度区间 | min:number,max:number,message?:string | blur |
| numberRangeRule(min,max,message?) | 数值范围 | min:number,max:number,message?:string | change |
| businessRule(predicate,message) | 自定义业务断言 | predicate:()=>boolean,message:string | blur |

示例：
```ts
import { requiredRule, ipRule, macRule, optionRule } from '@/utils/validation';
import type { OptionDefinition } from '@/types/option';

const optionDef: OptionDefinition = { code: 3, name: 'router', dataType: 'ip' , category: 'network' };
const rules = {
  ip: [requiredRule(), ipRule()],
  mac: [requiredRule(), macRule('MAC 格式错误')],
  optionValue: [optionRule(optionDef)]
};
```

## 组件示例：与 Element Plus 表单联动

以下示例展示如何在表单中使用校验规则，同时复用 IP/MAC/时间工具：

```vue
<script setup lang="ts">
import { ref } from 'vue';
import { requiredRule, ipRule, macRule, numberRangeRule } from '@/utils/validation';
import { timeLeft } from '@/utils/time';

const formModel = ref({ ip: '', mac: '', lease: 3600 });
const rules = {
  ip: [requiredRule(), ipRule()],
  mac: [requiredRule(), macRule()],
  lease: [numberRangeRule(60, 86400, '租约需在 1 分钟到 24 小时间')]
};

const leaseHint = () => timeLeft(new Date(Date.now() + formModel.value.lease * 1000).toISOString());
</script>

<template>
  <el-form :model="formModel" :rules="rules" label-width="120px">
    <el-form-item label="客户端 IP" prop="ip">
      <el-input v-model="formModel.ip" placeholder="例如 192.168.1.10" />
    </el-form-item>
    <el-form-item label="客户端 MAC" prop="mac">
      <el-input v-model="formModel.mac" placeholder="AA:BB:CC:DD:EE:FF" />
    </el-form-item>
    <el-form-item label="租约时长" prop="lease">
      <el-input-number v-model="formModel.lease" :min="60" :max="86400" />
      <span class="hint">剩余: {{ leaseHint() }}</span>
    </el-form-item>
  </el-form>
</template>
```

使用建议：
- 复用工具函数完成输入预处理（如 `formatMac`）后再提交后端。
- 在表格/列表中展示 IP/MAC 时调用格式化函数统一展示风格。
- 如果需要高级校验（如租约与业务状态联动），可通过 `businessRule` 传入布尔断言。
