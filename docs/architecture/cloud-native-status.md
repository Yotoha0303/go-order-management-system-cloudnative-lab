# 云原生完成度与缺口

## 结论

当前项目已经完成**微服务核心改造、容器化运行验证和应用可靠性基础收口**，但尚未完成完整的生产级云原生交付。

最准确的定位是：

> 具备独立服务、独立数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、基础熔断、Gateway 限流、Outbox/Saga 运行指标、独立迁移和端到端 CI 的云原生演进实验项目。

不应描述为：

> 已经完成 Kubernetes、完整可观测性、持续部署和生产治理的完整云原生系统。

## 能力矩阵

| 能力域 | 状态 | 当前实现 |
| --- | --- | --- |
| 服务拆分 | 已完成 | Gateway、Identity、Catalog、Inventory、Order、Timeout Worker |
| 容器化 | 已完成 | 每个服务独立镜像，Compose 完整编排 |
| 数据所有权 | 已完成 | 4 个独立逻辑数据库，Order 不直接访问 Catalog/Inventory 表 |
| 分布式一致性 | 已完成基础版 | 库存预占、确认、释放、Order Saga、补偿和对账状态 |
| 异步消息 | 已完成基础版 | RabbitMQ TTL/DLX、Transactional Outbox、Publisher Confirms、手动 ACK |
| Worker 弹性 | 已完成基础版 | 租约抢占、`SKIP LOCKED`、租约回收、多副本启动 |
| HTTP 可靠性 | 已完成基础版 | Request ID、deadline、细分传输超时、有限重试、操作级熔断 |
| 入口流量保护 | 已完成基础版 | Gateway 客户端/全局 Token Bucket、429/Retry-After、状态清理 |
| 运行指标 | 已完成基础版 | Outbox/Saga 两条聚合 SQL、内部 JSON 端点、Worker 周期结构化日志 |
| 数据库迁移 | 已完成 | 4 个服务独立 Goose 目录和一次性 Migration Job |
| 健康检查 | 已完成基础版 | 各服务 `/ping`、`/live`、`/readyz`，Gateway 汇总上游就绪 |
| CI | 已完成 | lint、test、race、build、迁移、镜像、完整拓扑和 Saga 冒烟 |
| 自动对账 | 未完成 | 只有 `reconciliation_required` 状态和只读指标，尚无修复 Worker |
| Kubernetes | 未完成 | 尚无 Deployment、Service、Ingress、Job、HPA 等资源 |
| 完整可观测性 | 未完成 | 尚无 Prometheus、Grafana、OpenTelemetry、集中日志和告警 |
| 高级弹性治理 | 未完成 | 缺少并发隔离、自适应超时、分布式限流和跨副本熔断状态 |
| 持续部署 | 未完成 | 尚无 Registry 发布、环境部署、滚动发布和自动回滚 |
| 生产安全 | 未完成 | 内部静态 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 备份与恢复 | 未完成 | 尚无恢复演练、Runbook、SLO 和压测报告 |

## 已完成的云原生基础

### 独立部署单元与数据边界

```text
api-gateway
identity-service  -> go_order_identity
catalog-service   -> go_order_catalog
inventory-service -> go_order_inventory
order-service     -> go_order_ordering
order-timeout-worker
```

Order Service 通过 HTTP 获取商品快照和操作库存预占，不再通过共享数据库完成跨域事务。

### 分布式一致性

项目实现了：

- `reserving → pending → paid/cancelled` 订单状态流；
- `pending → confirmed/released` 库存预占状态流；
- 失败后的释放补偿；
- 补偿不确定时进入 `reconciliation_required`；
- 超时事件的 Transactional Outbox；
- RabbitMQ 延迟取消和幂等消费。

### 多 Worker 与确认发布

Outbox 使用：

```text
lease_owner
lease_until
next_attempt_at
```

并通过 `FOR UPDATE SKIP LOCKED` 领取批次。Publisher Channel 开启 Confirm Mode，只有 Broker ACK 后才把事件标记为 `published`。系统保持至少一次投递语义。

### 同步调用可靠性

Gateway 和业务服务传播：

```text
X-Request-ID
X-Request-Deadline
```

内部客户端具备：

- TCP 连接、TLS、响应头和单次请求总超时；
- 仅针对选定传输错误和 502/503/504 的有限重试；
- 指数退避和有限抖动；
- 剩余预算不足时停止重试；
- 按 `upstream/operation` 隔离的 Closed/Open/Half-open 熔断器。

Gateway 不重放外部业务请求。

### 入口限流

Gateway 在反向代理前检查客户端与全局 Token Bucket。超限返回 HTTP 429、`Retry-After` 和 Request ID。`/live` 与 `/readyz` 不受限流影响。

### Outbox/Saga 运行指标

Order Service 提供受内部 Token 保护的只读端点：

```http
GET /internal/v1/operations/reliability
```

每个快照固定执行两条聚合 SQL：

```text
1 条 Outbox 聚合
1 条 Order 聚合
```

当前可观测内容包括：

- Outbox 各状态数量；
- 有效租约、当前可重试和超时未完成数量；
- 最老可处理事件年龄、最大和累计失败尝试；
- 所有订单状态数量；
- `reconciliation_required` 数量与最老年龄；
- 超时停留在 `reserving`、`paying`、`cancelling` 的订单数；
- 快照时间和查询耗时。

Timeout Worker 周期输出相同快照的结构化日志。该能力不是 Prometheus 时序指标，也不会自动修改任何订单状态。

### 版本化 Schema 与 CI

业务进程不执行 `AutoMigrate`。Compose 使用独立 Migration Job，在服务启动前执行 Goose migration。

CI 验证：

- lint、普通测试、race、vet、全部构建；
- 单体和四套服务迁移；
- 全部服务镜像；
- 四数据库完整拓扑；
- 两个 Timeout Worker；
- Gateway readiness；
- 完整 Order Saga 冒烟。

指标阶段还验证真实 MySQL 聚合、空数据库、固定时钟年龄、内部鉴权、错误不泄露和恰好两条 SQL。

## 后续关键阶段

### Phase 5：可靠性收口

已完成：

1. RabbitMQ Publisher Confirms；
2. HTTP 统一超时预算；
3. 有限重试和指数退避；
4. 基础熔断；
5. Gateway 请求限流；
6. Outbox 与 Order Saga 只读运行指标。

剩余：

1. 自动 `reconciliation` Worker；
2. 明确的对账任务数据模型和修复动作；
3. Prometheus 指标与基础告警；
4. OpenTelemetry Trace；
5. Grafana Dashboard。

### Phase 6：Kubernetes 基础

最低资源集合：

```text
Deployment
Service
ConfigMap
Secret
Ingress
Migration Job
HorizontalPodAutoscaler
PodDisruptionBudget
NetworkPolicy
```

还需配置 requests/limits、startup/liveness/readiness Probe、RollingUpdate、安全上下文和回滚验证。

### Phase 7：持续交付

目标包括 GHCR 版本化镜像、测试环境部署、Migration Job 发布顺序、部署后 smoke、失败停止或回滚，以及生产人工审批。

### Phase 8：生产治理

目标包括最小权限数据库账户、mTLS/Workload Identity、备份恢复、SLO、告警、容量评估、压测、Runbook 和故障演练。

## 阶段完成度估算

以下数字是工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 90% |
| 容器化应用开发 | 约 84% |
| 应用可靠性基础 | 约 80% |
| 云原生应用基础 | 约 75%–79% |
| Kubernetes 部署 | 约 10% |
| 完整可观测性 | 约 25% |
| 生产级云原生交付 | 约 48%–58% |

## 简历和项目讲解表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Worker 多副本租约、请求预算、有限重试、操作级熔断、Gateway Token Bucket 限流、Outbox/Saga 聚合运行指标、独立数据库迁移和端到端 CI；继续建设自动对账、Kubernetes、Prometheus/OpenTelemetry 与持续交付体系。

不推荐：

> 已完成生产级云原生系统。
