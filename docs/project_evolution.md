# 项目演进记录

本文记录项目从业务型 Go 单体到当前云原生演进实验系统的真实过程。

## 阶段总览

```text
业务单体
  ↓
工程化单体
  ↓
多服务运行单元
  ↓
服务独立数据库
  ↓
库存预占 + Order Saga
  ↓
独立 Goose 迁移
  ↓
Outbox 多 Worker 租约
  ↓
应用可靠性收口
  ↓
Kubernetes 基础与交付合同
  ↓
【当前下一阶段：完整可观测性】
  ↓
持续交付与运行保障
```

## Phase 0：原单体业务基本盘

完成：

- 用户注册、登录、JWT 和最小 RBAC；
- 商品创建、查询、上下架；
- 库存初始化、增加、查询和库存流水；
- 用户级幂等订单；
- MySQL 事务、行锁、条件更新和订单状态机；
- Redis 商品详情 cache-aside；
- RabbitMQ TTL/DLX 超时取消；
- Transactional Outbox；
- Docker、Compose、Goose、Makefile 和 GitHub Actions。

这一阶段的一致性依赖单个 MySQL 本地事务。

## Phase 1：迁移基线与架构审计

完成：

- 从原仓库保留完整 Git 历史；
- 建立实验仓库；
- 修复 CI 与数据库命名漂移；
- 固化单体依赖、数据所有权、事务边界和风险；
- 建立可重复的 Compose 和 CI 验证入口。

该阶段不改变业务行为，只为后续演进建立可验证基线。

## Phase 2：Microservices v1——运行单元拆分

拆分为：

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
```

每个运行单元拥有独立二进制和 Docker 镜像。Gateway 提供统一入口，Timeout Worker 与 HTTP 生命周期分离。

初始限制：进程已经拆分，但仍共享 MySQL。

## Phase 3：Microservices v2——数据所有权与 Saga

### 独立数据库

```text
Identity  -> go_order_identity
Catalog   -> go_order_catalog
Inventory -> go_order_inventory
Ordering  -> go_order_ordering
```

### 服务间调用

- Catalog / Inventory → Identity：管理员角色校验；
- Order → Catalog：商品快照；
- Order → Inventory：库存预占、确认和释放；
- Timeout Worker → Order：超时取消。

### 状态与补偿

```text
Inventory reservation:
pending -> confirmed / released

Order:
reserving -> pending -> paid / cancelled
reserving -> failed
uncertain -> reconciliation_required
```

- 预占失败：订单标记 `failed`；
- 预占成功但订单最终事务失败：释放预占；
- 释放也失败：标记 `reconciliation_required`；
- 取消或超时：释放库存；
- 支付：确认库存。

CI 实际启动四数据库拓扑，并验证注册、商品、库存、幂等下单、支付、主动取消和 RabbitMQ 超时补偿。

## Phase 4：独立迁移与 Outbox 多 Worker

### 服务独立 Goose 迁移

```text
migrations/identity
migrations/catalog
migrations/inventory
migrations/ordering
```

业务进程不再执行 `AutoMigrate`。Compose 使用一次性 Migration Job，迁移成功后才启动服务。

### Outbox 租约

新增：

```text
lease_owner
lease_until
next_attempt_at
```

Worker 使用 `FOR UPDATE SKIP LOCKED`，实现：

- 多副本不重复拥有同一活跃租约；
- 崩溃后租约回收；
- 失败事件延迟重试；
- 真实 MySQL 集成测试；
- Compose 与 CI 双副本验证。

## Phase 5：应用可靠性收口——已完成

### 5.1 RabbitMQ Publisher Confirms

- 发布与消费 Channel 分离；
- Broker ACK 后才把 Outbox 标记为 `published`；
- NACK、确认超时、通道关闭和直接发布错误进入失败重试；
- 保留 at-least-once，不宣称 exactly-once。

### 5.2 HTTP 请求预算与有限重试

```text
X-Request-ID
X-Request-Deadline
```

实现：

- TCP、TLS、响应头和单次调用总超时；
- 只重试选定网络错误和 502/503/504；
- 指数退避与有限抖动；
- 剩余预算不足时停止；
- 4xx、业务错误和调用方 deadline 不重试；
- 库存预占复用稳定 reservation ID；
- Gateway 不重放外部业务请求。

### 5.3 基础熔断与 Gateway 限流

- 按 `<upstream>/<operation>` 隔离的 Closed/Open/Half-open 熔断器；
- 选定基础设施错误计数，4xx 不打开熔断；
- Open 状态在网络调用前快速失败；
- Gateway 客户端与全局 Token Bucket；
- HTTP 429、`Retry-After` 和 Request ID。

### 5.4 Outbox 与 Saga 运行指标

- Outbox 与 Order 聚合 SQL；
- 内部 Token 保护的只读 JSON 端点；
- Outbox 状态、租约、可重试、超时、年龄和尝试次数；
- Order 状态、对账数量、最老年龄和卡住的瞬态状态；
- Worker 周期结构化日志。

该能力是 Prometheus Collector 的数据源，不等于完整时序监控。

### 5.5 自动 Order Reconciliation Worker

新增：

```text
order_reconciliation_tasks
trg_orders_v2_create_reconciliation_task
order-reconciliation-worker
```

实现：

- 订单状态与任务创建同一 MySQL 事务；
- 任务租约与 `FOR UPDATE SKIP LOCKED`；
- 多副本和过期租约恢复；
- Inventory release/confirm 幂等重试；
- 本地最终订单状态、Outbox 完成和任务完成同事务；
- 有界指数重试；
- 未知动作和状态不匹配保持 `unresolved`；
- 非变更式 dry-run。

## Phase 6：Kubernetes 基础——已完成

### 6.1 Kustomize 交付基础

新增：

```text
deploy/kubernetes/base
deploy/kubernetes/overlays/local
scripts/k8s/deploy-local.sh
```

基础清单包括：

- Namespace、ConfigMap 和 Secret 合同；
- MySQL、RabbitMQ StatefulSet 与 PVC；
- 四个服务独立 Migration Job；
- Gateway、四个业务服务和两个 Worker Deployment；
- HTTP Service 与 Gateway NodePort；
- startup/liveness/readiness Probe；
- Worker 进程探针；
- CPU/内存 requests 和 limits；
- RollingUpdate；
- Schema 等待 initContainer。

### 6.2 真实 kind 部署、恢复与 Saga

独立 Kubernetes CI Job 会：

1. 从干净 runner 创建 disposable kind 集群；
2. 构建并加载七个应用镜像；
3. 应用 local overlay；
4. 等待 MySQL、RabbitMQ 和四个 Migration Job；
5. 等待七个 Deployment；
6. 验证只有 Gateway 使用 NodePort；
7. 验证两个 Timeout Worker 和两个 Reconciliation Worker；
8. 发布不可用 Gateway revision，确认 rollout 失败；
9. 执行 `kubectl rollout undo`；
10. 验证原镜像与 Gateway readiness 恢复；
11. 执行完整 Kubernetes Order Saga；
12. 无论成功失败都删除集群。

实际验收发现并修复：

- MySQL Pod 内隐式 Unix Socket 与 TCP 行为差异；
- Deployment Ready 后 Gateway 聚合 readiness 短暂抖动造成的一次性检查脆弱性。

### 6.3 Ingress、test overlay 与 PDB

新增：

```text
deploy/kubernetes/overlays/test
.github/workflows/kubernetes-contracts.yml
```

Test overlay 定义：

- Gateway、四个业务服务和两个 Worker 类型均为 2 副本；
- 七个 `minAvailable: 1` PodDisruptionBudget；
- 一个 `nginx` Gateway Ingress；
- 测试主机名 `go-order.test.local`；
- MySQL、RabbitMQ 和业务 Service 继续使用 ClusterIP；
- test Secret 仅使用占位值。

独立 Kubernetes Contracts workflow 验证：

- local 和 test overlays 都能渲染；
- test overlay 只有一个 Ingress；
- 七个 Deployment 均为 2 副本；
- 七个 PDB selector 与工作负载标签一致；
- test overlay 不包含 NodePort；
- MySQL/RabbitMQ 不会被错误应用 PDB；
- 渲染 YAML 作为 artifact 保存。

边界：当前验证的是 Ingress/PDB 资源合同，不是 Ingress Controller 安装、TLS 或真实 Ingress 流量。

## 当前项目状态

当前已经具备：

- API Gateway、四个业务服务和两个独立 Worker 类型；
- 服务独立数据库与数据所有权；
- Inventory Reservation 和 Order Saga；
- Transactional Outbox、Publisher Confirms 和超时补偿；
- HTTP deadline、有限重试、熔断和限流；
- Outbox/Saga 运行指标；
- 自动分类、修复和 dry-run；
- 服务独立迁移；
- Compose 四库、四个 Worker 副本和完整 Saga；
- Kustomize base/local/test overlays；
- kind 真实部署、失败 rollout 检测、`rollout undo` 和完整 Kubernetes Saga；
- Gateway Ingress 和多副本 PDB 合同。

当前仍不等于完整生产级云原生系统。

## Phase 7：完整可观测性——下一主阶段

计划：

- Prometheus；
- Grafana；
- OpenTelemetry；
- W3C Trace Context；
- `trace_id` / `span_id` 日志字段；
- Saga、Outbox、RabbitMQ 和 Reconciliation 指标；
- 基础告警规则。

## Phase 8：持续交付与运行保障——待完成

计划：

- GHCR 版本化镜像；
- 测试环境自动部署；
- 部署后 Smoke Test；
- 环境级错误版本回滚；
- MySQL 备份恢复；
- 故障演练；
- Runbook；
- 压测和容量报告；
- 最小权限数据库账户与 Workload Identity。

## Phase 6 之后的 Kubernetes 增强

这些不属于当前基础阶段的完成门槛：

- Ingress Controller 安装与真实流量验收；
- TLS；
- HPA；
- NetworkPolicy；
- 多节点与节点失效验证；
- 不可变 Registry 镜像；
- 托管云存储、LoadBalancer 和 Workload Identity。
