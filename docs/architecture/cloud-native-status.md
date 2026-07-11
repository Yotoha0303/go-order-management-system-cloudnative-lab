# 云原生完成度与缺口

## 结论

当前项目已经完成**微服务核心改造、容器化验证和应用可靠性收口**，但尚未完成 Kubernetes、完整可观测性、持续交付和生产运行保障。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、基础熔断、Gateway 限流、运行指标和自动对账 Worker 的云原生演进实验项目。

不应描述为：

> 已完成生产级云原生系统。

## 能力矩阵

| 能力域 | 状态 | 当前实现 |
| --- | --- | --- |
| 服务拆分 | 已完成 | Gateway、Identity、Catalog、Inventory、Order、Timeout Worker、Reconciliation Worker |
| 容器化 | 已完成 | 每个运行单元独立镜像，Compose 完整编排 |
| 数据所有权 | 已完成 | 4 个独立逻辑数据库 |
| 分布式一致性 | 已完成基础版 | Inventory Reservation、Order Saga、补偿和明确对账状态 |
| 异步消息 | 已完成基础版 | RabbitMQ TTL/DLX、Transactional Outbox、Publisher Confirms、手动 ACK |
| Timeout Worker 弹性 | 已完成基础版 | 租约、`SKIP LOCKED`、多副本、租约恢复 |
| 自动对账 | 已完成基础版 | 结构化任务、事务触发器、三种已知修复动作、租约 Worker、多副本 |
| HTTP 可靠性 | 已完成基础版 | Request ID、deadline、细分超时、有限重试、操作级熔断 |
| 入口保护 | 已完成基础版 | Gateway 客户端/全局 Token Bucket、429/Retry-After |
| 运行指标 | 已完成基础版 | Outbox/Saga 两条聚合 SQL、内部端点、周期结构化日志 |
| 数据库迁移 | 已完成 | 服务独立 Goose migration 和一次性 Job |
| CI | 已完成 | lint、test、race、build、迁移、镜像、四库、四个 Worker 副本、Saga 冒烟 |
| Kubernetes | 未完成 | 无 Deployment、Service、Ingress、Migration Job、HPA 等清单 |
| 完整可观测性 | 未完成 | 无 Prometheus、Grafana、OpenTelemetry 和正式告警 |
| 持续部署 | 未完成 | 无 GHCR 发布、测试环境自动部署、滚动更新和自动回滚 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无备份恢复演练、Runbook、压测报告和故障演练 |

## 当前运行单元

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker x N
order-reconciliation-worker x N
```

数据边界：

```text
Identity  -> go_order_identity
Catalog   -> go_order_catalog
Inventory -> go_order_inventory
Ordering  -> go_order_ordering
```

## 已完成的应用可靠性链

### 异步发布

```text
Order local transaction
  -> timeout Outbox
  -> lease claim
  -> RabbitMQ publish
  -> Broker ACK
  -> published
```

NACK、确认超时、通道关闭和直接发布错误都会进入失败重试。语义仍是 at-least-once。

### 同步调用

```text
X-Request-ID
X-Request-Deadline
```

内部客户端具备：

- TCP、TLS、响应头和总超时；
- 502/503/504 与选定网络错误的有限重试；
- 指数退避与有限抖动；
- 剩余预算判断；
- 按 `<upstream>/<operation>` 隔离的熔断器。

Gateway 不重放外部业务请求。

### 入口保护

Gateway 使用客户端与全局 Token Bucket。超限返回 HTTP 429、`Retry-After` 和 Request ID。健康端点不受业务限流阻断。

### 运行指标

Order Service 提供：

```http
GET /internal/v1/operations/reliability
```

每个快照执行两条聚合 SQL，覆盖 Outbox 状态、租约、重试、超时、年龄、尝试次数，以及 Order 状态、`reconciliation_required` 和卡住的瞬态状态。

这仍不是 Prometheus 时序监控。

### 自动对账

Ordering migration 新增：

```text
order_reconciliation_tasks
trg_orders_v2_create_reconciliation_task
```

订单进入 `reconciliation_required` 时，根据原状态创建明确动作：

| 原状态 | 动作 |
| --- | --- |
| `reserving` | `release_inventory_and_fail` |
| `cancelling` | `finalize_cancel` |
| `paying` | `finalize_payment` |
| 其他 | `unresolved` |

该映射不读取 `failure_reason`。触发器与订单状态更新处于同一事务，回滚时任务同时回滚。

Reconciliation Worker：

- 使用租约和 `FOR UPDATE SKIP LOCKED`；
- 支持多个副本和过期租约恢复；
- 通过现有 InventoryClient 复用 deadline、重试和熔断；
- 重复执行幂等 release/confirm；
- 本地订单最终状态、Outbox 完成和任务完成同事务；
- 失败进入有界指数重试；
- 未知动作或状态不匹配保持 `unresolved`；
- 保留原始失败历史。

## CI 证据

当前流水线验证：

```text
golangci-lint
go test ./...
go test -race ./...
go vet ./...
go build ./...
单体和四套服务 migration validate
7 个服务/Worker 二进制
全部 Compose 镜像
4 个数据库
2 个 Timeout Worker
2 个 Reconciliation Worker
Gateway readiness
完整 Order Saga smoke
```

Reconciliation 阶段还验证：

- 触发器动作映射；
- 状态更新与任务创建事务回滚；
- Worker 独占租约和租约恢复；
- release/fail、cancel、payment 三种修复；
- 远程失败保持可重试；
- 未知动作保持 `unresolved`。

## 下一阶段

### Phase 6：Kubernetes 基础

需要完成：

```text
Deployment
Service
ConfigMap
Secret
Migration Job
Ingress
startupProbe
livenessProbe
readinessProbe
requests / limits
RollingUpdate
rollout undo
HPA
PodDisruptionBudget
NetworkPolicy
```

推荐先在 kind 或 k3d 验证，不要求立即购买托管集群。

### Phase 7：完整可观测性

需要完成：

- Prometheus；
- Grafana；
- OpenTelemetry；
- W3C Trace Context；
- Saga、Outbox、RabbitMQ 和 Reconciliation 指标；
- 基础告警。

### Phase 8：持续交付与运行保障

需要完成：

- GHCR 镜像；
- 测试环境自动部署；
- Smoke Test；
- 错误版本回滚；
- MySQL 备份恢复；
- 故障演练；
- Runbook；
- 压测与容量报告；
- 最小权限账号和 Workload Identity。

## 完成度估算

以下为工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 92% |
| 容器化应用开发 | 约 87% |
| 应用可靠性基础 | 约 92% |
| 云原生应用基础 | 约 80%–84% |
| Kubernetes 部署 | 约 10% |
| 完整可观测性 | 约 25% |
| 生产级云原生交付 | 约 52%–62% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、操作级熔断、Gateway Token Bucket 限流、Outbox/Saga 运行指标、结构化自动对账、独立迁移和端到端 CI；继续建设 Kubernetes、Prometheus/OpenTelemetry 与持续交付体系。
