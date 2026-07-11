# 云原生完成度与缺口

## 结论

当前项目已经完成**微服务核心改造和容器化运行验证**，但尚未完成完整的生产级云原生交付。

最准确的定位是：

> 具备独立服务、独立数据库、Saga、Outbox、多 Worker、独立迁移和端到端 CI 的云原生演进实验项目。

不应描述为：

> 已经完成 Kubernetes、可观测性、持续部署和生产治理的完整云原生系统。

## 能力矩阵

| 能力域 | 状态 | 当前实现 |
| --- | --- | --- |
| 服务拆分 | 已完成 | Gateway、Identity、Catalog、Inventory、Order、Timeout Worker |
| 容器化 | 已完成 | 每个服务独立镜像，Compose 完整编排 |
| 数据所有权 | 已完成 | 4 个独立逻辑数据库，Order 不直接访问 Catalog/Inventory 表 |
| 分布式一致性 | 已完成基础版 | 库存预占、确认、释放、Order Saga、补偿和对账状态 |
| 异步消息 | 已完成基础版 | RabbitMQ TTL/DLX、Transactional Outbox、手动 ACK |
| Worker 弹性 | 已完成基础版 | 租约抢占、`SKIP LOCKED`、崩溃后租约回收、多副本启动 |
| 数据库迁移 | 已完成 | 4 个服务独立 Goose 目录和一次性迁移 Job |
| 健康检查 | 已完成基础版 | 各服务 `/ping`、`/live`、`/readyz`，Gateway 汇总上游就绪 |
| CI | 已完成 | lint、test、race、vet、build、迁移、镜像、完整拓扑和 Saga 冒烟 |
| Kubernetes | 未完成 | 尚无 Deployment、Service、Ingress、Job、HPA 等资源 |
| 可观测性 | 未完成 | 尚无 Prometheus、Grafana、OpenTelemetry、集中日志和告警 |
| 服务弹性治理 | 未完成 | 缺少标准重试、熔断、限流、隔离和统一超时预算 |
| 持续部署 | 未完成 | 尚无 Registry 发布、环境部署、滚动发布和自动回滚 |
| 生产安全 | 未完成 | 内部静态 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 备份与故障恢复 | 未完成 | 尚无恢复演练、对账任务、Runbook、SLO 和压测报告 |

## 已完成的云原生基础

### 独立部署单元

每个服务拥有独立入口和镜像：

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
```

服务可以独立启动、重启和构建。Timeout Worker 已验证两个副本同时运行。

### 独立数据边界

```text
Identity  -> go_order_identity
Catalog   -> go_order_catalog
Inventory -> go_order_inventory
Ordering  -> go_order_ordering
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

### 多 Worker Outbox

Outbox 使用：

```text
lease_owner
lease_until
next_attempt_at
```

并通过 `FOR UPDATE SKIP LOCKED` 领取批次。它避免多个活跃 Worker 同时拥有同一事件，但保持至少一次投递语义。

### 版本化 Schema

业务进程不执行 `AutoMigrate`。Compose 使用独立迁移 Job，在服务启动前执行 Goose migration。

## 尚未完成的关键阶段

### Phase 5：可靠性与可观测性

优先事项：

1. RabbitMQ Publisher Confirms；
2. Prometheus 指标；
3. Outbox 积压、租约、失败和重试指标；
4. Order Saga 成功率、补偿率和对账状态指标；
5. OpenTelemetry Trace 和跨服务上下文传播；
6. Grafana Dashboard 与基础告警；
7. HTTP 客户端超时、受控重试和熔断。

### Phase 6：Kubernetes 本地部署

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

所有服务必须配置：

- resource requests/limits；
- startupProbe；
- livenessProbe；
- readinessProbe；
- 滚动更新策略；
- 非 root 和只读文件系统等安全上下文。

建议先使用 kind 或 k3d，本阶段不要求购买托管 Kubernetes。

### Phase 7：持续交付

目标：

- 构建并推送版本化镜像到 GHCR；
- 对开发环境执行自动部署；
- Migration Job 与应用发布顺序明确；
- 部署后执行 readiness 和 Saga smoke；
- 失败时停止发布或回滚；
- 生产环境保留人工审批。

### Phase 8：生产治理

目标：

- 业务服务和迁移任务使用独立最小权限数据库账户；
- 内部 Token 替换为 mTLS 或 Workload Identity；
- MySQL/RabbitMQ 备份恢复演练；
- `reconciliation_required` 自动对账；
- SLI/SLO、告警、容量评估、压测和 Runbook；
- 故障注入和恢复验证。

## 阶段完成度估算

以下数字是工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 85% |
| 容器化应用开发 | 约 80% |
| 云原生应用基础 | 约 65%–70% |
| Kubernetes 部署 | 约 10% |
| 可观测性 | 约 15% |
| 生产级云原生交付 | 约 40%–50% |

## 简历和项目讲解表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ 超时补偿、Worker 多副本租约抢占、独立数据库迁移和端到端 CI 验证；继续建设 Kubernetes、可观测性与持续交付体系。

不推荐：

> 已完成生产级云原生系统。
