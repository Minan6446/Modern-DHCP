## 地址池管理模块

本模块覆盖 IPv4/IPv6 地址池的建模、创建/更新/删除、监控、策略与前缀委派工具。前端主要文件：

- API 层： [web/src/api/pools.ts](web/src/api/pools.ts#L1-L19)
- 视图： [web/src/views/pool/PoolLayout.vue](web/src/views/pool/PoolLayout.vue#L1-L28) | [web/src/views/pool/IPv4Pool.vue](web/src/views/pool/IPv4Pool.vue#L1-L129) | [web/src/views/pool/IPv6Pool.vue](web/src/views/pool/IPv6Pool.vue#L1-L83) | [web/src/views/pool/PoolAnalysis.vue](web/src/views/pool/PoolAnalysis.vue#L1-L89)
- 组件： [PoolMonitor](web/src/views/pool/components/PoolMonitor.vue#L1-L92) | [AllocationStrategy](web/src/views/pool/components/AllocationStrategy.vue#L1-L79) | [VlanBinding](web/src/views/pool/components/VlanBinding.vue#L1-L39) | [PrefixDelegation](web/src/views/pool/components/PrefixDelegation.vue#L1-L55) | [PrefixSplitTool](web/src/views/pool/components/PrefixSplitTool.vue#L1-L49) | [SubnetWizard](web/src/views/pool/components/SubnetWizard.vue#L1-L140)

### 数据模型 (types/pool.ts)

- `PoolSummary`: { id, name, version(4|6), cidr, allocated, capacity, utilization, status, vlanId? }
- `SubnetDraft`: 创建/更新载荷，含 cidr、gateway、dns[]、leaseTime/maxLeaseTime、exclude、rangeStart/rangeEnd、strategy、vlan/location/tags。
- `AllocationStrategy`: { mode: round-robin | sequential | random; preferReserved? }
- `PrefixDelegation`: { prefix, length, delegated, capacity }
- `ConflictReport`: { poolId, cidr, conflicts: [{ withPool, cidr, reason }] }

### API 说明

| 方法 | 路径 | 目的 | 关键参数 |
| --- | --- | --- | --- |
| `listPools` | GET /pools | 分页查询地址池 | PageQuery + version(4/6), keyword, status |
| `createPool` | POST /pools | 新建地址池 | `SubnetDraft` |
| `updatePool` | PUT /pools/{id} | 更新地址池 | Partial`<SubnetDraft>` |
| `deletePool` | DELETE /pools/{id} | 删除地址池 | id |
| `getUsage` | GET /pools/{id}/usage | 获取使用率序列 | id |
| `getHistory` | GET /pools/{id}/history | 操作/变更历史 | PageQuery |
| `getConflicts` | GET /pools/{id}/conflicts | CIDR 冲突报告 | id |
| `listPrefixDelegations` | GET /pools/prefixes | 前缀委派列表 | PageQuery |

返回统一包裹在 `ApiResponse`，列表接口为 `PageResult<T>`。

### 组件与页面

- PoolLayout：Tab 切换 IPv4/IPv6/分析。
- IPv4Pool：
  - 筛选：关键词、状态；防抖搜索。
  - 表格：名称、CIDR、利用率、状态；行点击触发池选中。
  - 操作：新建/编辑子网（SubnetWizard），删除；选中池后展示 PoolMonitor；内嵌 AllocationStrategy 与 VlanBinding 以配置分配策略和关联。
  - 数据：`listPools(version=4)`，创建/更新/删除调用对应 API。
- IPv6Pool：
  - 查询 IPv6 池；列表列出名称/前缀/利用率。
  - PrefixDelegation 管理委派列表并触发新增/分割；PrefixSplitTool 提供前缀切分计算。
- PoolAnalysis：静态示例看板，展示使用率趋势、分配效率、容量/性能建议。
- SubnetWizard：四步抽屉创建/编辑子网，支持 CIDR 范围计算、排除/预留批量、策略/租期、VLAN/位置。
- AllocationStrategy：编辑分配算法、优先预留，导入/导出/清空排除列表（CSV 工具），去重后通过 v-model 同步。
- VlanBinding：设置 VLAN、物理端口、SSID 映射与地理位置，保存后通过 `save` 事件回调。
- PoolMonitor：给定 poolId 拉取 usage/history/conflicts，展示折线图、历史表、冲突时间线和容量预警阈值/通知开关。
- PrefixDelegation：显示委派表，校验新增前缀格式（IPv6），触发 `add`/`split` 事件。
- PrefixSplitTool：前缀切分工具，校验父前缀与目标长度，示例生成子前缀列表。

### 典型交互流

1) 查看 IPv4 池：进入页面即 `listPools(version=4)`，行点击保存选中 id 并加载 PoolMonitor（usage/history/conflicts）。
2) 创建池：点击“新建地址池”→ SubnetWizard 输入基础/网络/策略/关联 → `createPool` → 刷新列表。
3) 编辑池：行内“编辑”→ Wizard 预填 → `updatePool(selectedId, draft)` → 刷新。
4) 删除池：行内删除按钮 + Popconfirm → `deletePool(id)`。
5) 配置分配策略/排除：在 AllocationStrategy 中导入/导出排除列表，策略通过 v-model 传回父组件，保存时随 draft 发送。
6) VLAN/端口绑定：在 VlanBinding 填写并触发 `save` 事件（当前示例直接提示，可接 API）。
7) IPv6 前缀池：`listPools(version=6)` 列表；在 PrefixDelegation 添加前缀（`createPool`）或触发分割；可用 PrefixSplitTool 预计算子前缀。

### 集成要点

- API 统一通过 httpClient，需保证鉴权拦截器可用；分页查询默认 page=1/pageSize=50，可按需透传。
- `SubnetDraft` 中 exclude/reserved 建议使用 `dedupeIps` 去重；CIDR 与 IP 校验复用 `validateRangeWithin`、`calcIPv4Range`、`validateIPv6Prefix` 等工具。
- 监控面板的冲突/使用率接口可按需接入实时推送（目前为首次加载调用）。
- CSV 导入导出依赖 utils/csv；下载文件名在组件内固定为 exclude.csv，可视需要增加前缀或时间戳。

### 快速示例

```ts
// 查询并创建 IPv4 池
const { data } = await listPools({ page: 1, pageSize: 20, version: 4, keyword: 'office' });

await createPool({
  name: 'office-1',
  cidr: '10.0.10.0/24',
  gateway: '10.0.10.1',
  dns: ['1.1.1.1'],
  leaseTime: 3600,
  exclude: ['10.0.10.2'],
  reserved: ['10.0.10.3'],
  strategy: { mode: 'round-robin', preferReserved: true }
});
```
