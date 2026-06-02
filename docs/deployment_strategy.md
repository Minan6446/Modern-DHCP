# 部署与运维策略

更新日期：2026-02-10（当前主干）

本指南覆盖从开发到生产的后端部署路径、配置要点与运维日常流程。

## 环境与依赖
- 语言与运行时：Go 1.22+。
- 数据库：MySQL 8.0+，需开启二进制日志以支持复制与审计。
- 前端（如需）：Node 18+ 用于管理控制台。
- 容器：推荐使用 Docker / containerd；Kubernetes 部署可用 Helm values（deploy/kubernetes/values.yaml）。

## 配置约定
- 配置来源优先级：CLI 旗标 > 环境变量 > YAML（configs/config.yaml）。
- 常见字段：
  - `server.listen`：API 监听地址，默认 0.0.0.0:8080。
  - `dhcpv4` / `dhcpv6`：协议端口、接口、租约策略。
  - `storage`：主库 DSN、读库 DSN。
  - `monitoring` / `alerting` / `security`：观测、告警、安全模块开关与阈值。
- 机密管理：生产环境建议使用环境变量或外部 secret 挂载，不在仓库内存放凭据。

## 部署模式
- 本地开发：
  - `go run ./cmd/dbinit` 执行迁移与引导数据。
  - `go run ./cmd/dhcpd` 启动后端；前端可单独运行。
- Docker Compose（deploy/docker/docker-compose.yaml）：
  - 适合 PoC 或单节点环境；提供 MySQL、DHCPD、前端服务。
- Kubernetes：
  - 使用 `deploy/kubernetes/values.yaml` 自定义副本数、资源、存储类与服务类型。
  - 通过 ConfigMap/Secret 提供配置，使用 InitContainer 运行迁移（或 Job 形式）。
  - Ingress/反向代理需开启 WebSocket 升级以支持 `/api/v1/dashboard/streams/live` 等实时流。

## 反向代理（Nginx 示例，443 同域）
```nginx
location /api/ {
  proxy_pass http://10.0.11.22:8080/api/;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
  proxy_set_header Host $host;
}
```
前端 WebSocket 需走 `wss://<域名>/api/v1/...`，不要直连后端 8080 明文端口。

## 迁移与版本管理
- 初始迁移：`go run ./cmd/dbinit --driver mysql --dsn ...`。
- 升级流程：
  1. 停止写流量或将节点置为 Draining。
  2. 在 Primary 节点执行迁移。
  3. 验证复制与健康后恢复流量。
- 回滚：如迁移可逆，可执行相应 down 脚本；否则需基于备份恢复。

## 运行与健康
- 健康检查：
  - Liveness/Readiness：`/cluster/overview`（角色、延迟）与 `/monitoring/overview`（核心指标）。
  - DB：连接池错误率、慢查询、复制延迟。
  - 实时流：`/dashboard/streams/live` 连接失败通常是反代未透传 Upgrade/Connection。
- 日志：
  - 结构化输出至 stdout，便于容器收集；关键事件写入审计。
- 资源：
  - CPU/内存：池解析与租约统计依赖数据库；应用层 CPU 主要来自编解码与 JSON 序列化。
  - FD/端口：DHCP 监听端口需特权；注意系统 ulimit 设置。

## 备份与恢复
- 数据库：
  - 定期全量 + 增量备份，验证恢复窗口。
  - 备份后执行一致性校验（checksum、对账查询）。
- 配置：
  - 将 YAML/Helm values 版本化；敏感配置使用外部密钥存储。

## 运营与日常检查
- 每日：审计异常、告警抑制窗口、复制延迟、活跃租约统计。
- 每周：索引与慢查询审计、备份可用性演练、容量评估（池利用率）。
- 变更前：将节点置为 Draining，确认监控基线与回滚预案。
