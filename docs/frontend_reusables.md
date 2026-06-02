# 前端可复用模块

## 错误映射
- 认证错误：`src/shared/errors/authErrors.ts` 提供 `mapAuthError(status, message)`。
- HTTP 错误：`src/shared/errors/httpErrors.ts` 提供 `mapHttpError(status, message)`。
- 业务错误：`src/shared/errors/businessErrors.ts` 提供 `mapBusinessError(code, override)`，可传入自定义映射表。

## Token 存储
- 位置：`src/shared/auth/tokenStorage.ts`。
- 使用：
  - 创建实例：`const storage = createTokenStorage({ session, persistent });`
  - 保存：`storage.saveTokens({ accessToken, refreshToken, expiresIn, remember });`
  - 读取：`storage.loadTokens()` 返回 `{ accessToken, refreshToken, accessExpiresAt, remember, lastUsername }`。
  - 用户信息：`storage.saveUser(user, remember)` / `storage.loadUser<User>()`。
- 内置 `tokenStorage` 默认使用浏览器的 sessionStorage/localStorage，并自动处理 "记住我"。

## 表单 Composable
- 位置：`src/shared/composables/useForm.ts`。
- 核心能力：加载状态、错误状态、防重复提交、校验钩子、成功/失败回调。
- 使用示例：
```ts
const { values, submit, loading, error } = useForm({
  initialValues: () => ({ username: '', password: '' }),
  validate: () => formRef.value?.validate(),
  submit: async (values) => login(values),
  onSuccess: () => toast('ok'),
  onError: (err) => log(err)
});
```

## HTTP 工具
- 重试：`retryAsync(fn, { retries, baseDelay, factor, retryOn })`。
- 缓存：`createRequestCache<T>(ttl)` 创建带 TTL 的内存缓存。
- 队列：
  - 顺序队列：`createTaskQueue()` 将任务串行执行。
  - 单飞队列：`createSingleFlight()` 合并并行请求（用于 token 刷新）。

## 安全
- CSRF：`ensureCsrfCookie()` 在需要时预取 `XSRF-TOKEN`，axios 已配置 `xsrfCookieName` 与 `xsrfHeaderName`。
- 重定向：`sanitizeRedirect(target, extraWhitelist?)` 对登录等入口的跳转参数做白名单校验。
- Token 存储：refresh_token 固定存于 sessionStorage 以降低持久化泄露风险，remember 仅影响用户信息缓存。

## 集成点
- 认证存储：`modules/auth/store` 通过 `tokenStorage` 统一读写 token 和用户信息。
- 登录表单：`modules/auth/views/Login.vue` 使用 `useForm` 统一校验和加载状态。
- HTTP 客户端：`shared/api-client/http.ts` 使用错误映射、重试、单飞队列与 GET 缓存。

## 测试
- 位置：`src/shared/**/__tests__/*.test.ts`，覆盖错误映射、token 存储、表单防重复、重试与队列。
- 运行：在 `web` 目录执行 `npm test` 或 `vitest`。
