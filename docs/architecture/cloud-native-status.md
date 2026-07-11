# 云原生完成度与缺口

## 结论

当前项目已经完成：

- 微服务核心改造；
- 容器化与四库数据所有权；
- 应用可靠性收口；
- Docker Compose 完整业务验证；
- Kubernetes Kustomize 基础；
- disposable kind 集群自动部署；
- 失败 revision 检测、`kubectl rollout undo` 和完整 Kubernetes Saga 验收。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、基础熔断、Gateway 限流、运行指标、自动对账，以及可重复 kind 部署与回滚验收的云原生演进实验项目。

仍不应描述为：

> 已完成生产级云原生系统。

## 能力矩阵

| 能力域 | 状态 | 当前实现 |
| --- | --- | --- |
| 服务拆分 | 已完成 | Gateway、Identity、Catalog、Inventory、Order、Timeout Worker、Reconciliation Worker |
| 容器化 | 已完成 | 每个运行单元独立镜像，Compose 与 kind 均可运行 |
| 数据所有权 | 已完成 | 4 个独立逻辑数据库 |
| 分布式一致性 | 已完成基础版 | Inventory Reservation、Order Saga、补偿和明确对账状态 |
| 异步消息 | 已完成基础版 | RabbitMQ TTL/DLX、Transactional Outbox、Publisher Confirms、手动 ACK |
| Timeout Worker 弹性 | 已完成基础版 | 租约、`SKIP LOCKED`、多副本、租约恢复 |
| 自动对账 | 已完成基础版 | 结构化任务、事务触发器、已知修复动作、租约 Worker、多副本、dry-run |
| HTTP 可靠性 | 已完成基础版 | Request ID、deadline、细分超时、有限重试、操作级熔断 |
| 入口保护 | 已完成基础版 | Gateway 客户端/全局 Token Bucket、429/Retry-After |
| 运行指标 | 已完成基础版 | Outbox/Saga 聚合 SQL、内部端点、周期结构化日志 |
| 数据库迁移 | 已完成 | 服务独立 Goose migration、Compose Job 和 Kubernetes Job |
| Compose CI | 已完成 | lint、test、race、build、迁移、镜像、四库、四个 Worker 副本、完整 Saga |
| Kubernetes 基础 | 已完成基础版 | Kustomize、Namespace、ConfigMap/Secret 合同、StatefulSet、Deployment、Service、Migration Job、探针和资源限制 |
| Kubernetes 运行验收 | 已完成基础版 | CI 创建 kind 集群，完成迁移、rollout、暴露面、双 Worker、失败 revision、undo 与完整 Saga |
| Kubernetes 高可用治理 | 未完成 | 无 Ingress、PDB、HPA、NetworkPolicy、多节点失效验证 |
| 完整可观测性 | 未完成 | 无 Prometheus、Grafana、OpenTelemetry 和正式告警 |
| 持续部署 | 未完成 | 无 GHCR 发布、测试环境自动部署和不可变版本推广 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无正式备份恢复演练、Runbook、压测报告和故障演练 |

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

Ordering migration 包含：

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

Reconciliation Worker：

- 使用租约和 `FOR UPDATE SKIP LOCKED`；
- 支持多个副本和过期租约恢复；
- 通过现有 InventoryClient 复用 deadline、重试和熔断；
- 重复执行幂等 release/confirm；
- 本地订单最终状态、Outbox 完成和任务完成同事务；
- 失败进入有界指数重试；
- 未知动作或状态不匹配保持 `unresolved`；
- 支持非变更式 dry-run。

## Kubernetes 交付基础

### 已实现清单

```text
Namespace
ConfigMap
Secret contract
MySQL StatefulSet + PVC
RabbitMQ StatefulSet + PVC
4 Migration Jobs
5 HTTP Services
5 HTTP Deployments
2 Worker Deployments
startup/liveness/readiness probes
resource requests/limits
RollingUpdate
local NodePort overlay
kind cluster configuration
```

只有 Gateway 使用 NodePort；业务服务、MySQL 和 RabbitMQ 保持集群内部访问。

### 自动化运行验收

独立 CI Job 会：

1. 创建干净 kind 集群；
2. 构建并加载 7 个应用镜像；
3. 应用 local overlay；
4. 等待 MySQL、RabbitMQ 和 4 个 Migration Job；
5. 等待 7 个 Deployment；
6. 验证 Gateway/内部 Service 暴露边界；
7. 验证两个 Timeout Worker 和两个 Reconciliation Worker；
8. 发布不可用 Gateway revision 并确认 rollout 失败；
9. 执行 `kubectl rollout undo`；
10. 确认原镜像和 Gateway readiness 恢复；
11. 执行完整 Kubernetes Order Saga；
12. 清理集群，失败时上传完整诊断。

该验收已经真实发现并修复：

- Kubernetes MySQL 客户端隐式 Unix Socket 与显式 TCP 的差异；
- Deployment 已 Ready 后 Gateway 聚合 readiness 短暂抖动导致的一次性检查脆弱性。

## CI 证据

当前流水线包含两条独立链路。

### Go 与 Compose

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
完整 Compose Order Saga
```

### Kubernetes Runtime

```text
Kustomize render
kind cluster creation
2 StatefulSets
4 Migration Jobs
7 Deployments
Service exposure validation
2 + 2 Worker replicas
failed rollout detection
kubectl rollout undo
complete Kubernetes Order Saga
failure diagnostics
cluster cleanup
```

## 下一阶段

### Phase 6 剩余治理

仍需完成：

- Ingress Controller 和域名入口；
- PodDisruptionBudget；
- HorizontalPodAutoscaler；
- NetworkPolicy；
- 多节点和节点故障验证；
- Registry 不可变镜像；
- dev/test/prod overlays；
- 托管云存储、负载均衡和 Workload Identity 集成。

### Phase 7：完整可观测性

需要完成：

- Prometheus；
- Grafana；
- OpenTelemetry；
- W3C Trace Context；
- `trace_id` / `span_id` 日志字段；
- Saga、Outbox、RabbitMQ 和 Reconciliation 指标；
- 基础告警。

### Phase 8：持续交付与运行保障

需要完成：

- GHCR 镜像；
- 测试环境自动部署；
- 部署后 Smoke Test；
- 环境级自动回滚；
- MySQL 备份恢复；
- 故障演练；
- Runbook；
- 压测与容量报告；
- 最小权限账号和 Workload Identity。

## 完成度估算

以下为工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 94% |
| 容器化应用开发 | 约 92% |
| 应用可靠性基础 | 约 94% |
| 云原生应用基础 | 约 88%–91% |
| Kubernetes 基础部署与验收 | 约 65%–72% |
| 完整可观测性 | 约 25% |
| 生产级云原生交付 | 约 62%–70% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、操作级熔断、Gateway Token Bucket 限流、自动对账、独立迁移，以及 Compose/kind 双环境端到端 CI；在 Kubernetes 中验证 Migration Job、探针、RollingUpdate、失败 revision 检测、`rollout undo` 和完整订单 Saga。
