# REST API 补充说明

本文件在核心规范基础上扩展了支持元数据的地址池端点，以及为策略自动化新增的 `poolSelector` 动作片段。以下片段遵循 OpenAPI 3.0 语法，可直接复制到完整规范中。

```yaml
paths:
  /api/v1/tenants/{tenantId}/pools/find:
    post:
      summary: 按元数据筛选地址池
      tags: [Pools]
      parameters:
        - $ref: '#/components/parameters/TenantIdPath'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PoolFindRequest'
      responses:
        '200':
          description: 匹配到的地址池，按特异性从低到高排序
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/AddressPool'
  /api/v1/tenants/{tenantId}/leases/{leaseId}/cooldown/clear:
    post:
      summary: 清除租约的冷却/隔离状态，让 IP 回到地址池
      tags: [Leases]
      parameters:
        - $ref: '#/components/parameters/TenantIdPath'
        - name: leaseId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 冷却状态清除后的租约
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Lease'
        '404':
          description: 未找到对应租约
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
  /api/v1/tenants/{tenantId}/pools/resolve:
    post:
      summary: 基于中继元数据解析最合适的地址池
      tags: [Pools]
      parameters:
        - $ref: '#/components/parameters/TenantIdPath'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PoolSelector'
      responses:
        '200':
          description: 依据 interface/SSID/location/VLAN 优先级匹配成功的地址池
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AddressPool'
        '404':
          description: 未匹配到地址池
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
components:
  schemas:
    PoolFindRequest:
      type: object
      properties:
        scope:
          type: string
          description: 可选的作用域限制（GLOBAL/SUBNET/VLAN/PORT）
        parentId:
          type: string
        vlanId:
          type: integer
          minimum: 1
        interfaceId:
          type: string
        ssid:
          type: string
        location:
          type: string
        limit:
          type: integer
          default: 50
          minimum: 1
          maximum: 500
    PoolSelector:
      type: object
      description: 来自中继或 AP 场景的元数据
      properties:
        interfaceId:
          type: string
        ssid:
          type: string
        location:
          type: string
        vlanId:
          type: integer
          minimum: 1
      minProperties: 1
    PolicyAction:
      type: object
      properties:
        poolId:
          type: string
          format: uuid
        poolSelector:
          $ref: '#/components/schemas/PoolSelector'
        leaseProfile:
          $ref: '#/components/schemas/LeaseProfileOverride'
      anyOf:
        - required: [poolId]
        - required: [poolSelector]
```

已经集成生成版 OpenAPI 的客户端，可将此片段粘贴进原始文档后重新生成 SDK。若需要直接使用的产物，可引用 `docs/openapi.bundle.yaml`，在上游总规范更新前，`openapi-generator-cli`、`oapi-codegen`、`autorest` 等工具都可以指向该文件。对于人工操作，`cmd/modern-dhcp` CLI 现已提供 `pools find`、`pools resolve` 与 `policy selector build`，无需手写 JSON 即可构建元数据过滤条件与选择器。

### 地址池分配元数据
- `allocationMode`（枚举：`SEQUENTIAL`、`ROUND_ROBIN`、`PRIORITY_WEIGHTED`）与 `priorityWeight`（1-100，百分比偏好）是 `AddressPool` 创建/更新载荷中的可选字段。
- `ROUND_ROBIN` 会使用每个地址池的游标轮询所有可用 IPv4 地址；`PRIORITY_WEIGHTED` 则优先使用可用区间内最低 `priorityWeight%` 的地址，若耗尽再回落到剩余部分。
  若忽略这些属性，默认值为 `SEQUENTIAL`/`50`，以保持现有行为。

### 自动化与任务调度 API
Phase 3 服务要求能够将任务调度状态暴露给 CMDB/监控平台，并允许通过 HTTP 将一次性任务推送进自动化队列。以下三个端点复用现有会话上下文（若 `tenantId` 省略则使用当前租户），并与 `notifications.channels` 中声明的多渠道 webhook 进行对接：
- `/automation/snapshot`：提供调度器当前排队、活跃 Worker 以及注册 Handler 的实时统计，可用于 Prometheus Pull/CMDB 心跳检查；
- `/automation/schedules`：返回经 Sanitized 的静态计划（含 `interval`、`channels`、`payload` 样例），便于对照 CMDB 中的运行手册；
- `POST /automation/jobs`：允许外部系统（变更后台、监控告警）下发一次性任务，并通过 `channels` 字段加挂 CMDB、监控或自定义 webhook；
- `GET /automation/jobs`：支持按租户、类型、状态、来源、触发人过滤的任务历史分页查询；
- `GET /automation/jobs/{jobId}`：查看单次任务的完整上下文、结果摘要以及错误信息。

```yaml
paths:
  /api/v1/automation/snapshot:
    get:
      summary: 获取自动化调度器运行快照
      tags: [Automation]
      responses:
        '200':
          description: 调度器运行时指标
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AutomationSnapshot'
  /api/v1/automation/schedules:
    get:
      summary: 列出启用中的周期任务
      tags: [Automation]
      responses:
        '200':
          description: 配置化任务列表
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/AutomationSchedule'
  /api/v1/automation/jobs:
    get:
      summary: 分页查询自动化任务历史
      tags: [Automation]
      parameters:
        - $ref: '#/components/parameters/TenantQuery'
        - $ref: '#/components/parameters/Limit'
        - $ref: '#/components/parameters/Offset'
        - in: query
          name: types
          schema:
            type: string
          description: 逗号分隔的 JobType 列表
        - in: query
          name: statuses
          schema:
            type: string
          description: 逗号分隔的状态列表（pending/running/succeeded/failed）
        - in: query
          name: sources
          schema:
            type: string
          description: 逗号分隔的来源标识（如 api.manual、automation.schedule）
        - in: query
          name: triggeredBy
          schema:
            type: string
          description: 触发人标识（API Key、用户、workflow service 等）
      responses:
        '200':
          description: 任务列表
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AutomationJobListResponse'
    post:
      summary: 通过 API 投递一次性自动化任务
      tags: [Automation]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AutomationJobRequest'
      responses:
        '202':
          description: 任务已排入队列
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AutomationJobResponse'
  /api/v1/automation/jobs/{jobId}:
    get:
      summary: 查询单个自动化任务详情
      tags: [Automation]
      parameters:
        - name: jobId
          in: path
          required: true
          schema:
            type: string
        - $ref: '#/components/parameters/TenantQuery'
      responses:
        '200':
          description: 任务详情
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AutomationJobRun'
        '404':
          description: 任务不存在
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
components:
  schemas:
    AutomationSnapshot:
      type: object
      properties:
        pendingJobs:
          type: integer
        activeWorkers:
          type: integer
        registeredHandlers:
          type: integer
        startedAt:
          type: string
          format: date-time
    AutomationSchedule:
      type: object
      properties:
        type:
          type: string
        enabled:
          type: boolean
        tenantId:
          type: string
        interval:
          type: string
        initialDelay:
          type: string
        labels:
          type: object
          additionalProperties:
            type: string
        channels:
          type: array
          items:
            type: string
        payload:
          type: object
          description: 根据 Job 类型透传的参数示例
    AutomationJobRequest:
      type: object
      required:
        - type
      properties:
        type:
          type: string
          description: automation.JobType 枚举值
        tenantId:
          type: string
          description: 为空时沿用当前 session 的租户
        labels:
          type: object
          additionalProperties:
            type: string
        payload:
          type: object
          additionalProperties: true
        channels:
          type: array
          items:
            type: string
          description: 对应 `notifications.channels` 中的名称，用于 fan-out
        priority:
          type: integer
          format: int32
        notBefore:
          type: string
          format: date-time
        source:
          type: string
          description: 标识任务来源，未填写默认为 api.manual
        triggeredBy:
          type: string
          description: 触发者（用户名、API Key、workflow service 等）
    AutomationJobResponse:
      type: object
      properties:
        id:
          type: string
          description: 调度器生成的 Job ID
    AutomationJobRun:
      type: object
      properties:
        id:
          type: string
        tenantId:
          type: string
        type:
          type: string
        status:
          type: string
        source:
          type: string
        triggeredBy:
          type: string
        priority:
          type: integer
        attempts:
          type: integer
        payloadHash:
          type: string
        labels:
          type: object
          additionalProperties:
            type: string
        payload:
          type: object
          additionalProperties: true
        resultSummary:
          type: string
        errorMessage:
          type: string
        notBefore:
          type: string
          format: date-time
        queuedAt:
          type: string
          format: date-time
        startedAt:
          type: string
          format: date-time
        completedAt:
          type: string
          format: date-time
        updatedAt:
          type: string
          format: date-time
    AutomationJobListResponse:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/AutomationJobRun'
        total:
          type: integer
        limit:
          type: integer
        offset:
          type: integer
```

结合以上 schema，可以通过 `docs/openapi.bundle.yaml` 重新生成 SDK，或在 CMDB/监控系统中直接引用以驱动 webhook 下发流程。

### 监控与集成 API（Phase 8）
Phase 8 引入的监控聚合器会周期性生成 `AlertFeedSnapshot`、`AnalyticsSnapshot` 以及新的 `CMDBSyncPayload`，配合规则 DSL 允许控制面实时自愈。以下端点均已在 `docs/api/openapi_v1.yaml` 与 `docs/openapi.bundle.yaml` 中建模，可直接用于 SDK 生成：
- `/monitoring/alerts`：获取当前租户的告警快照，并返回 `AlertFeedTotals`；
- `/monitoring/alerts/{alertId}/ack`：在分配责任人的同时确认告警，可选 `ttlSeconds` 自动创建静默窗口；
- `/monitoring/alerts/{alertId}/suppress`：指定渠道或持续时间的静默操作，复用 mute window；
- `/monitoring/analytics`：返回容量规划 (`CapacityInsight`) 与异常检测 (`AnomalyInsight`)；
- `/monitoring/integrations/cmdb`：一次性整合热点地址池、容量/异常洞察与告警合计，便于 CMDB/ITSM 入库。

```yaml
paths:
  /api/v1/monitoring/alerts:
    get:
      summary: Alert lifecycle feed for the active tenant
      tags: [Monitoring]
      parameters:
        - $ref: '#/components/parameters/TenantQuery'
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AlertFeedSnapshot'
  /api/v1/monitoring/alerts/{alertId}/ack:
    post:
      summary: Acknowledge an alert and optionally assign ownership
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AlertAcknowledgeRequest'
  /api/v1/monitoring/alerts/{alertId}/suppress:
    post:
      summary: Suppress an alert for a duration or channel
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AlertSuppressRequest'
  /api/v1/monitoring/analytics:
    get:
      summary: Advanced analytics snapshot for dashboards
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AnalyticsSnapshot'
  /api/v1/monitoring/integrations/cmdb:
    get:
      summary: CMDB-friendly payload containing hotspots and alert totals
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CMDBSyncPayload'
```

常见调用示例：

```bash
# 拉取指定租户的告警并分页
curl -sS -H "Authorization: Bearer $TOKEN" \
  "${API_ROOT}/api/v1/monitoring/alerts?tenantId=demo&limit=50"

# 将告警标记为已确认并静默 30 分钟
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"assignee":"noc@corp","ttlSeconds":1800}' \
  "${API_ROOT}/api/v1/monitoring/alerts/ALERT-123/ack"

# 导出 CMDB/ITSM 友好的整合载荷
curl -sS -H "Authorization: Bearer $TOKEN" \
  "${API_ROOT}/api/v1/monitoring/integrations/cmdb?tenantId=demo"
```

#### CMDB/ITSM 集成工具包
- 结合 `PoolUsageSummary` 与 `CapacityInsight` 可以直接驱动 CMDB 热点面板或 UCMDB 自定义视图；
- `AlertFeedTotals` 与 `AnomalyInsight` 提供按严重级别的 SLA 遵循数据，可映射到 ITSM 工单优先级；
- `Source` 字段默认为 `modern-dhcp.monitoring`, 可在 `config.notification.integrations.cmdb.source` 中覆盖，方便 CMDB 与其他来源去重；
- 建议在 CMDB Connector 侧缓存 `generatedAt`，避免重复写入。若需要增量同步，可存储上次成功的 `fingerprint` 列表，对应的 `lifecycle` 字段即可用于判断是否关闭。

#### SDK 重新生成与分发
1. 确认 `docs/api/openapi_v1.yaml` 与 `docs/openapi.bundle.yaml` 版本一致（`git diff` 应为空），再选择所需语言；
2. TypeScript（Axios）：
   ```bash
   openapi-generator-cli generate ^
     -i docs/openapi.bundle.yaml ^
     -g typescript-axios ^
     -o sdk/typescript-monitoring
   ```
3. Go（chi/oapi-codegen）：
   ```bash
   oapi-codegen --config tools/sdk/go-monitoring.yaml \
     docs/openapi.bundle.yaml > internal/api/monitoring.gen.go
   ```
4. PowerShell（AutoRest）：
   ```bash
   autorest --input-file=docs/openapi.bundle.yaml \
     --powershell --output-folder=sdk/powershell-monitoring
   ```
5. 重新生成后，运行各语言 SDK 的 `fmt`/`lint`，并在变更说明中引用 Phase 8 监控特性，提醒用户更新依赖。

### 租户配额与角色自助（Phase 9）
Tenant Console 需要开放租户管理员的配额调整与角色管理接口，默认仅拥有 `tenant.quota.*` 能力的主体可访问。

```yaml
paths:
  /api/v1/tenants/{tenantId}/quotas:
    get:
      summary: Retrieve tenant quota limits
      tags: [Tenants]
      parameters:
        - $ref: '#/components/parameters/TenantIdPath'
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TenantQuota'
    put:
      summary: Update tenant quota limits
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TenantQuotaUpdateRequest'
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TenantQuota'
```

`TenantQuota` 当前包含以下字段：
- `poolLimit` / `leaseLimit` / `clientLimit`：地址池、租约、客户端数量上限；
- `apiRequestLimit`：每小时可用 API 请求额度（0 表示无限）；
- `automationJobLimit`：可并发运行的自动化任务数量；
- `updatedAt`：UTC 时间戳，用于 UI 缓存与提示。

示例调用：

```bash
# 查看默认租户配额
curl -sS -H "Authorization: Bearer $TOKEN" \
  "${API_ROOT}/api/v1/tenants/demo/quotas"

# 调整 API 请求与自动化任务限制
curl -sS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"apiRequestLimit":100000,"automationJobLimit":50}' \
  "${API_ROOT}/api/v1/tenants/demo/quotas"
```

更新会记录 `tenant.quota.update` 审计事件，便于在自助门户回溯责任人。若租户超出配额，配额服务会提前触发 Phase 8 告警渠道，Tenant Console 可直接订阅这些告警来提醒管理员减载或申请提升。

租户审计接口新增过滤能力，可按 `actor`、`actions`、`resource`、`correlationId` 查询：

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "${API_ROOT}/api/v1/tenants/demo/audit?actor=user:alice&actions=tenant.quota.update,rbac.assignment.create&limit=20"
```

当提供过滤条件时，后端自动调用 `ListEventsFiltered` 并限制分页上限，满足 Tenant Console 的快速查找体验。

### 运维与支持 API
运维控制台需要在无额外数据库查询的情况下快速了解系统管理开关、导入导出资产、知识库内容以及脚本目录。本阶段新增的 `/api/v1/ops/*` 端点全部要求管理员角色，并返回只读的元数据快照：
- `/ops/system`：系统管理开关、备份位置与维护窗口，便于 CMDB 展示系统就绪度；
- `PUT /ops/system`：一次性更新主题、默认语言、维护模式/窗口与平台公告（写操作会自动记录审计）；
- `PATCH /ops/settings/theme|locale|maintenance`：针对单项配置的轻量更新接口，方便前端分步骤保存；
- `/ops/import-export`：最近导入/导出产物及允许的文件格式；
- `/ops/import-export/jobs`：提交和查询批量导入/导出任务队列；
- `/ops/import-export/jobs/{jobId}`：查看单个导入/导出任务的状态与产物；
- `/ops/help-center`：本地缓存的知识库摘要（标题、概述、更新时间）以及外链；
- `/ops/help-center/articles|faq|releases`：分别返回文章列表、FAQ 以及发布说明时间轴；
- `/ops/support`：外部客服渠道及升级联系人列表；
- `/ops/scripts`：允许运维在 UI 内直接执行的脚本白名单及默认超时配置。
- `/ops/scripts/{scriptName}/runs`：触发某个白名单脚本执行并记录审计信息；
- `/ops/scripts/runs`：查询最近的脚本执行记录，以便排障和回溯；
- `/ops/scripts/runs/{runId}`：查看单次脚本执行的实时状态与输出。
- `/ops/scripts/runs/{runId}/approve`：由具备权限的管理员审批并放行脚本执行；
- `/ops/scripts/runs/{runId}/reject`：拒绝危险或未经授权的脚本请求并写入审计。
- `/diagnostics/snapshot`：汇总系统健康、分析洞察、自动化运行状况与脚本目录，支持 `format=zip` 下载支援包。

```yaml
paths:
  /api/v1/ops/system:
    get:
      summary: 查询系统管理配置
      tags: [OpsSupport]
      responses:
        '200':
          description: 当前系统维度的开关状态
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OpsSystemSummary'
    put:
      summary: 更新平台主题/语言/维护模式
      tags: [OpsSupport]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/OpsSystemUpdate'
      responses:
        '200':
          description: 保存后的系统配置
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OpsSystemSummary'
  /api/v1/ops/settings/theme:
    patch:
      summary: 单独更新控制台主题
      tags: [OpsSupport]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [theme]
              properties:
                theme:
                  type: string
      responses:
        '200':
          description: 更新后的系统配置
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OpsSystemSummary'
  /api/v1/ops/settings/locale:
    patch:
      summary: 单独更新默认语言
      tags: [OpsSupport]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [locale]
              properties:
                locale:
                  type: string
      responses:
        '200':
          description: 更新后的系统配置
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OpsSystemSummary'
  /api/v1/ops/settings/maintenance:
    patch:
      summary: 更新维护模式/窗口/公告
      tags: [OpsSupport]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                maintenanceMode:
                  type: boolean
                maintenanceWindow:
                  type: string
                announcement:
                  type: string
      responses:
        '200':
          description: 更新后的系统配置
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OpsSystemSummary'

示例：

```bash
# 快速进入维护模式并展示公告
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"maintenanceMode":true,"maintenanceWindow":"23:00-02:00 UTC","announcement":"Control plane upgrade"}' \
  "${API_ROOT}/api/v1/ops/settings/maintenance"
```
  /api/v1/ops/import-export:
    get:
      summary: 导入导出通道及最近任务
      tags: [OpsSupport]
      responses:
        '200':
          description: 存储路径与最近 10 条传输记录
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ImportExportSummary'
  /api/v1/ops/import-export/jobs:
    post:
      summary: 提交导入/导出任务
      tags: [OpsSupport]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TransferJobRequest'
      responses:
        '202':
          description: 已进入批处理队列
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TransferJob'
    get:
      summary: 查询导入/导出任务
      tags: [OpsSupport]
      parameters:
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: 最近的导入/导出任务
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TransferJobListResponse'
  /api/v1/ops/import-export/jobs/{jobId}:
    get:
      summary: 查询导入/导出任务详情
      tags: [OpsSupport]
      parameters:
        - name: jobId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 单个任务的运行状态
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TransferJob'
  /api/v1/ops/help-center:
    get:
      summary: 帮助中心与知识库元数据
      tags: [OpsSupport]
      responses:
        '200':
          description: 帮助文章索引
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HelpCenterInfo'
  /api/v1/ops/help-center/articles:
    get:
      summary: 帮助中心文章列表
      tags: [OpsSupport]
      responses:
        '200':
          description: 文章清单
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HelpArticleList'
  /api/v1/ops/help-center/faq:
    get:
      summary: 常见问题列表
      tags: [OpsSupport]
      responses:
        '200':
          description: FAQ 条目
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HelpFAQList'
  /api/v1/ops/help-center/releases:
    get:
      summary: 发布说明时间轴
      tags: [OpsSupport]
      responses:
        '200':
          description: 最近的版本记录
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReleaseNoteList'
  /api/v1/ops/support:
    get:
      summary: 支持渠道目录
      tags: [OpsSupport]
      responses:
        '200':
          description: 联系客服/升级的可用方式
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SupportDirectory'
  /api/v1/ops/scripts:
    get:
      summary: 运维脚本目录
      tags: [OpsSupport]
      responses:
        '200':
          description: 允许在控制台中执行的脚本列表
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptCatalog'
  /api/v1/ops/scripts/{scriptName}/runs:
    post:
      summary: 启动脚本执行
      tags: [OpsSupport]
      parameters:
        - name: scriptName
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ScriptRunRequest'
      responses:
        '202':
          description: 已接受执行请求
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptRun'
  /api/v1/ops/scripts/runs:
    get:
      summary: 最近脚本执行
      tags: [OpsSupport]
      parameters:
        - $ref: '#/components/parameters/Limit'
      responses:
        '200':
          description: 脚本执行记录
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptRunListResponse'
  /api/v1/ops/scripts/runs/{runId}:
    get:
      summary: 查询脚本执行详情
      tags: [OpsSupport]
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 单次脚本运行的完整状态
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptRun'
  /api/v1/ops/scripts/runs/{runId}/approve:
    post:
      summary: 审批脚本执行
      tags: [OpsSupport]
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ScriptApprovalRequest'
      responses:
        '200':
          description: 执行已放行并进入队列
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptRun'
  /api/v1/ops/scripts/runs/{runId}/reject:
    post:
      summary: 拒绝脚本执行
        /api/v1/diagnostics/snapshot:
          get:
            summary: 诊断快照（可选 ZIP）
            tags: [Diagnostics]
            parameters:
              - name: tenantId
                in: query
                schema:
                  type: string
                description: 当需要聚合特定租户的分析/容量信息时指定
              - $ref: '#/components/parameters/Limit'
              - name: format
                in: query
                schema:
                  type: string
                  enum: [json, zip]
                description: 设置为 zip 即可下载支援包
            responses:
              '200':
                description: JSON 诊断快照或 ZIP 支援包
                content:
                  application/json:
                    schema:
                      $ref: '#/components/schemas/DiagnosticsSnapshot'
                  application/zip:
                    schema:
                      type: string
                      format: binary
      tags: [OpsSupport]
      parameters:
        - name: runId
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ScriptApprovalRequest'
      responses:
        '200':
          description: 执行被拒绝并记录审计
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScriptRun'
components:
  schemas:
    OpsSystemSummary:
      type: object
      properties:
        enabled:
          type: boolean
        allowConfigExport:
          type: boolean
        allowConfigImport:
          type: boolean
        backupLocation:
          type: string
        maintenanceWindow:
          type: string
        theme:
          type: string
        locale:
          type: string
        maintenanceMode:
          type: boolean
        announcement:
          type: string
        updatedAt:
          type: string
          format: date-time
        updatedBy:
          type: string
        generatedAt:
          type: string
          format: date-time
    OpsSystemUpdate:
      type: object
      properties:
        theme:
          type: string
        locale:
          type: string
        maintenanceMode:
          type: boolean
        maintenanceWindow:
          type: string
        announcement:
          type: string
    ImportExportSummary:
      type: object
      properties:
        enabled:
          type: boolean
        storagePath:
          type: string
        objectStore:
          type: string
        formats:
          type: array
          items:
            type: string
        maxFileSize:
          type: integer
          format: int64
        recentTransfers:
          type: array
          items:
            $ref: '#/components/schemas/TransferMetadata'
        generatedAt:
          type: string
          format: date-time
    TransferMetadata:
      type: object
      properties:
        name:
          type: string
        sizeBytes:
          type: integer
          format: int64
        modifiedAt:
          type: string
          format: date-time
    TransferJob:
      type: object
      properties:
        id:
          type: string
        kind:
          type: string
          enum: [import, export]
        resource:
          type: string
        format:
          type: string
        status:
          type: string
          enum: [pending, running, succeeded, failed]
        requestedBy:
          type: string
        reason:
          type: string
        sourceUri:
          type: string
        targetUri:
          type: string
        artifactPath:
          type: string
        metadata:
          type: object
          additionalProperties:
            type: string
        sizeBytes:
          type: integer
          format: int64
        error:
          type: string
        createdAt:
          type: string
          format: date-time
        startedAt:
          type: string
          format: date-time
        completedAt:
          type: string
          format: date-time
    TransferJobRequest:
      type: object
      properties:
        kind:
          type: string
          enum: [import, export]
        resource:
          type: string
        format:
          type: string
        sourceUri:
          type: string
        targetUri:
          type: string
        reason:
          type: string
        metadata:
          type: object
          additionalProperties:
            type: string
        sizeBytes:
          type: integer
          format: int64
    TransferJobListResponse:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/TransferJob'
        total:
          type: integer
    HelpCenterInfo:
      type: object
      properties:
        enabled:
          type: boolean
        baseUrl:
          type: string
        featuredTopics:
          type: array
          items:
            type: string
        externalSearchUrl:
          type: string
        articles:
          type: array
          items:
            $ref: '#/components/schemas/HelpArticle'
        generatedAt:
          type: string
          format: date-time
    HelpArticle:
      type: object
      properties:
        id:
          type: string
        title:
          type: string
        summary:
          type: string
        url:
          type: string
        path:
          type: string
        updatedAt:
          type: string
          format: date-time
    SupportDirectory:
      type: object
      properties:
        enabled:
          type: boolean
        ticketUrl:
          type: string
        chatUrl:
          type: string
        phone:
          type: string
        escalationPolicy:
          type: string
        contacts:
          type: array
          items:
            type: string
        generatedAt:
          type: string
          format: date-time
    ScriptCatalog:
      type: object
      properties:
        enabled:
          type: boolean
        defaultTimeout:
          type: string
          description: Go duration 字符串（如 "30s"）
        maxConcurrent:
          type: integer
        sandboxImage:
          type: string
        entries:
          type: array
          items:
            $ref: '#/components/schemas/ScriptDescriptor'
    ScriptDescriptor:
      type: object
      properties:
        name:
          type: string
        description:
          type: string
        command:
          type: string
        args:
          type: array
          items:
            type: string
        allowedRoles:
          type: array
          items:
            type: string
        requiresApproval:
          type: boolean
          description: 是否需要二次审批
    ScriptRunRequest:
      type: object
      properties:
        reason:
          type: string
        args:
          type: array
          items:
            type: string
        env:
          type: object
          additionalProperties:
            type: string
        timeoutSeconds:
          type: integer
          format: int64
          minimum: 0
    ScriptRun:
      type: object
      properties:
        id:
          type: string
        scriptName:
          type: string
        description:
          type: string
        requestedBy:
          type: string
        role:
          type: string
        reason:
          type: string
        requiresApproval:
          type: boolean
        approvedBy:
          type: string
        approvedAt:
          type: string
          format: date-time
        approvalNote:
          type: string
        rejectedBy:
          type: string
        rejectedAt:
          type: string
          format: date-time
        rejectionNote:
          type: string
        args:
          type: array
          items:
            type: string
        extraArgs:
          type: array
          items:
            type: string
        env:
          type: object
          additionalProperties:
            type: string
        status:
          type: string
        stdout:
          type: string
        stderr:
          type: string
        error:
          type: string
        createdAt:
          type: string
          format: date-time
        startedAt:
          type: string
          format: date-time
        completedAt:
          type: string
          format: date-time
        duration:
          type: string
          description: Go duration 字符串（如 "1.5s"）
        timeout:
          type: string
          description: Runner 应用的超时时间 (Go duration)
    ScriptRunListResponse:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/ScriptRun'
        total:
          type: integer
    ScriptApprovalRequest:
      type: object
      properties:
        note:
          type: string
          description: 审批备注或拒绝理由
```

如同自动化接口，可直接引用 `docs/openapi.bundle.yaml` 或通过 `openapi_v1.yaml` 将上述 schema 集成到 SDK/监控中的类型系统里，帮助控制台渲染支持目录与脚本目录。
