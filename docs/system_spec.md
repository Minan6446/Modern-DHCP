# 系统规格说明

本文件聚焦后端控制平面的核心职责、模块边界与数据流，作为架构与实现之间的衔接文档，便于新成员和运维团队快速理解系统。
更新日期：2026-02-10（当前主干）

## 目标

## 核心组件
  - 地址池（internal/pool）：分层池模型、权重/轮询调度、解析元数据（VLAN/端口/SSID/Location）。
  - 租约（internal/lease）：租约分配与历史查询、统计与分布计算、窗口函数聚合。
  - 策略（internal/policy）：匹配与授权钩子，可被 DHCP pipeline 引用。
  - 自动化（internal/automation）：任务调度、审批流、重试与上下文取消。
  - 观测/告警（internal/monitoring, internal/alerting）：指标聚合、分级通知、告警路由。
  - 安全（internal/security）：Rate Limit、Snooping、IPSG 采集与事件发布。
  - 审计（internal/audit）：操作与决策路径记录，覆盖 API 与内部任务。
  - 数据库驱动（internal/db, internal/storage）：MySQL 驱动、连接池、事务包装。
  - 单一 schema/DSN，所有数据为全局资源。
  - 迁移（migrations/）：SQLx 兼容的迁移脚本，随版本升级。

## 关键数据模型（简述）

## 数据流与调用链
1. API 请求由 Gateway 接收，进行认证与上下文注入。
2. 业务服务执行领域逻辑（池解析、租约查询、任务编排等），通过仓储层访问数据库。
3. DHCP pipeline 通过策略钩子与池解析模块决定租约分配，并记录审计与监控指标。
4. 观测层收集指标与事件，Alerting 模块根据全局阈值路由到通知渠道。
5. 自动化任务由调度器触发，执行过程中与审计、监控和通知模块交互。

## 可扩展性与可靠性要点

## 运行与配置接口
  - `go run ./cmd/dbinit` 负责迁移和基础数据。
  - `go run ./cmd/dhcpd` 启动主服务。
  - `go run ./cmd/modern-dhcp` 提供 CLI 操作（租约、池、任务等）。

## 依赖与约束
