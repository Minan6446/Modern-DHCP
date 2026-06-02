## 系统管理模块

更新日期：2026-02-10（当前主干）

涵盖登录、角色/权限、API Key、会话管理与用户角色分配。前端主要文件：

- 视图： [web/src/views/system/Login.vue](web/src/views/system/Login.vue#L1-L98) | [ApiKeyManager](web/src/views/system/ApiKeyManager.vue#L1-L126) | [RoleManager](web/src/views/system/RoleManager.vue#L1-L156) | [SessionManager](web/src/views/system/SessionManager.vue#L1-L96) | [UserRoleAssign](web/src/views/system/UserRoleAssign.vue#L1-L62)
- API： [apikeys](web/src/api/apikeys.ts#L1-L10) | [roles](web/src/api/roles.ts#L1-L22) | [sessions](web/src/api/sessions.ts#L1-L9)
- 类型： [web/src/types/system.ts](web/src/types/system.ts#L1-L128)

### 数据模型

- `RoleDetail`: { id, name, description?, permissions: string[], scope: global, createdAt? }
- `PermissionNode`: { key, label, description?, children?, dependsOn? }
- `ApiKey`: { id, name, token, scope: string[], createdAt, expiresAt?, active }
- `SessionInfo`: { id, userId, username, ip, userAgent, createdAt, lastSeenAt }
- `PageQuery/PageResult`、`ApiResponse` 为通用分页/返回包装。

### API 列表

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| `listApiKeys` | GET /apikeys | 分页列出 API Key | PageQuery(keyword) |
| `createApiKey` | POST /apikeys | 新建 API Key | Partial<ApiKey> |
| `revokeApiKey` | POST /apikeys/{id}/revoke | 吊销 Key | id |
| `listRoles` | GET /roles | 角色分页 | PageQuery(keyword) |
| `getRole` | GET /roles/{id} | 角色详情 | id |
| `createRole` | POST /roles | 创建角色 | Partial<RoleDetail> |
| `updateRole` | PUT /roles/{id} | 更新角色 | Partial<RoleDetail> |
| `deleteRole` | DELETE /roles/{id} | 删除角色 | id |
| `getPermissionTree` | GET /permissions/tree | 权限树 | - |
| `assignRolePermissions` | POST /roles/{id}/permissions | 绑定权限 | { keys: string[] } |
| `assignUserRoles` | POST /users/{userId}/roles | 为用户分配角色 | userId, roleIds[] |
| `listSessions` | GET /sessions | 分页查询活跃会话 | PageQuery(keyword,status) |
| `getSession` | GET /sessions/{id} | 会话详情 | id |
| `forceLogout` | POST /sessions/{id}/force-logout | 强制下线 | id |

返回统一包裹在 `ApiResponse`，分页为 `PageResult<T>`。

### 组件与页面

- Login：支持 local/LDAP/SSO 三个 tab；表单校验后调用 `auth.login`（store）；包含验证码刷新钩子。
- ApiKeyManager：
  - 列表：名称/Scope/创建/过期/Token 显隐；分页。
  - 新建：对话框收集 name、scope 多选、expiresAt；提交 `createApiKey`。
  - 吊销：行内 Popconfirm 调用 `revokeApiKey`。
- RoleManager：
  - 列表：角色名、范围、权限数量；操作：编辑、分配用户、删除。
  - 角色抽屉：名称、范围(global)、描述、权限树多选（`getPermissionTree`），保存走 create/update。
  - 分配用户：输入逗号分隔的 userIds，逐个 `assignUserRoles`。
- SessionManager：
  - 使用 Pinia `useSessionStore` 拉取 `listSessions`；筛选 keyword/status；行内“Kick”调用 `forceLogout`。
- UserRoleAssign：
  - 先加载角色列表；输入 userId，勾选角色后批量 `assignUserRoles`。

### 交互流示例

1) 创建角色：RoleManager 新建 → 选择 scope 与权限树 → `createRole` → 刷新。
2) 分配角色：RoleManager 行操作“Assign Users”或 UserRoleAssign 页面 → 输入 userIds → `assignUserRoles`。
3) 下线会话：SessionManager 过滤后点击 Kick → `forceLogout` → store 本地移除该行。
4) 管理 API Key：ApiKeyManager 新建 → `createApiKey`；必要时行内 Revoke。

### 集成要点

- 鉴权：所有接口依赖 httpClient 拦截器携带 Token。
- 表单校验：Role 表单使用 Element Plus 规则；提交前需通过 `formRef.validate`。
- 权限树：`getPermissionTree` 应返回唯一 key 结构，支持 children/dependsOn；使用 `setCheckedKeys` 重置选中。
- 会话分页：SessionManager 默认 pageSize=10，可透传 status=active|stale；kick 后本地过滤掉该 id，避免二次拉取。
- 令牌展示：ApiKey token 默认隐藏，用户点击后在内存中显示；避免持久化明文。

### 快速示例

```ts
// 创建角色并分配给用户
const { data: role } = await createRole({ name: 'ops-admin', scope: 'global', permissions: ['pool.read', 'lease.write'] });
await assignUserRoles('user-123', [role.data.id]);

// 强制下线会话
await forceLogout('session-abc');
```
