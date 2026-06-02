# 测试策略（后端）

本文件说明后端的测试分类与建议覆盖面，便于持续集成与回归。

## 分类
- 单元测试：聚焦纯函数与轻量服务逻辑，位于 `*_test.go`。
- 组件测试：带数据库或外部依赖的子系统（pool/lease/automation）使用 test DSN 或内存替身。
- 集成测试：在 CI 中可用 Docker Compose/Kubernetes 启动依赖后运行 `go test ./...`。

## 覆盖重点
- DHCP 编解码与 pipeline：消息解析、阶段状态机、策略/池钩子分支。
- 池解析：权重/轮询、按 VLAN/Location 解析、fallback 行为。
- 租约服务：分配/续约/释放、窗口函数统计、幂等与冲突场景。
- 自动化：调度、审批超时、重试、幂等键。
- HA：角色判断、复制延迟降级、Draining 行为。
- 安全：Rate Limit/Snooping 事件聚合与动作。

## 数据与夹具
- 使用 migration 初始化测试数据库；可为每个测试生成独立 schema 以隔离数据。
- 通过工厂/fixture 创建池、策略、租约样本，便于重用。

## 性能与基准
- 在 `perf/benchmarks/` 或包内 `*_test.go` 使用 `testing.B` 编写基准，关注池解析、序列化、策略匹配。
- 基准运行：`go test -bench=. -benchmem ./internal/...`。

## CI 建议
- 执行 `go test ./...` 作为最小门槛。
- 对接数据库的测试可在 CI 中通过 docker-compose 启动 MySQL；如耗时长，可分层运行。
- 对关键路径启用 `-race` 检查（成本较高，可在 nightly 执行）。

## 调试技巧
- 使用 `t.Parallel()` 提升测试速度，但注意共享状态。
- 在失败输出中包含池/接口等上下文，便于定位。
- 对外部调用使用 mock 或内存实现，避免不稳定的网络依赖。
