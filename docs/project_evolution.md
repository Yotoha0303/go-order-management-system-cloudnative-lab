# 项目演进记录

本文记录项目从业务型 Go 单体到当前微服务实验系统的真实演进过程。

## 阶段总览

```text
业务单体
  ↓
工程化单体
  ↓
独立 API / Worker
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
【当前阶段】
  ↓
可观测性与可靠性
  ↓
Kubernetes
  ↓
持续交付与生产治理
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
- React 管理台；
- Docker、Compose、Goose、Makefile 和 GitHub Actions。

这一阶段的核心一致性依赖单个 MySQL 本地事务：订单、库存、订单项、幂等和 Outbox 同事务提交。

## Phase 1：迁移基线与架构审计

完成：

- 从原仓库复制完整 Git 历史；
- 建立实验仓库；
- 修复 CI 与数据库命名漂移；
- 固化单体依赖、数据所有权、事务边界和风险；
- 建立可重复的 Compose 和 CI 验证入口。

该阶段不改变业务行为，只为后续拆分建立可验证基线。

## Phase 2：Microservices v1——运行单元拆分

完成：

- `api-gateway`；
- `identity-service`；
- `catalog-service`；
- `inventory-service`；
- `order-service`；
- `order-timeout-worker`；
- 每个服务独立二进制和 Docker 镜像；
- Gateway 统一入口和上游就绪检查；
- Worker 与 HTTP API 生命周期分离；
- CI 构建全部服务和镜像。

限制：服务虽然已拆为独立进程，但最初仍共享 MySQL。

## Phase 3：Microservices v2——数据所有权与 Saga

完成：

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
- Worker → Order：超时取消。

### 库存预占

```text
pending -> confirmed
pending -> released
```

### Order Saga

```text
reserving -> pending -> paid / cancelled
reserving -> failed
uncertain -> reconciliation_required
```

### 补偿规则

- 预占失败：订单标记 `failed`；
- 预占成功但订单最终事务失败：释放预占；
- 释放也失败：标记 `reconciliation_required`；
- 取消或超时：释放库存；
- 支付：确认库存。

### 验证

CI 实际启动四数据库拓扑，并验证注册、商品、库存、幂等下单、支付、主动取消和 RabbitMQ 超时补偿。

## Phase 4：独立迁移与 Outbox 多 Worker

完成：

### 服务独立 Goose 迁移

```text
migrations/identity
migrations/catalog
migrations/inventory
migrations/ordering
```

业务进程不再执行 `AutoMigrate`。Compose 使用四个一次性 Migration Job，迁移成功后才启动服务。

### Outbox 租约

新增：

```text
lease_owner
lease_until
next_attempt_at
```

Worker 使用：

```sql
FOR UPDATE SKIP LOCKED
```

实现：

- 多 Worker 不重复拥有同一活跃租约；
- 崩溃后租约可回收；
- 失败事件延迟重试；
- Compose 和 CI 启动两个 Worker 副本；
- 真实 MySQL 集成测试验证领取隔离和租约恢复。

## 当前项目状态

当前已经具备：

- API Gateway 和独立业务服务；
- 服务独立数据库；
- 服务间 HTTP 调用；
- 内部服务认证；
- 库存预占与 Order Saga；
- Transactional Outbox；
- RabbitMQ 延迟取消；
- 多 Worker 租约抢占；
- 服务独立 Goose 迁移；
- 完整 Compose 与端到端 CI。

当前仍不等于完整生产级云原生系统。

## Phase 5：可靠性与可观测性——待完成

计划：

- RabbitMQ Publisher Confirms；
- Prometheus Metrics；
- Grafana Dashboard；
- OpenTelemetry Trace；
- Request ID / Trace ID 跨服务传播；
- Outbox backlog、租约、失败和重试指标；
- Saga 成功率、补偿率和对账状态指标；
- HTTP 客户端超时预算；
- 幂等请求的受控重试；
- 熔断、限流和并发隔离；
- 基础告警规则。

## Phase 6：Kubernetes 本地部署——待完成

计划：

- Deployment；
- Service；
- ConfigMap；
- Secret；
- Ingress；
- Migration Job；
- startup/liveness/readiness probes；
- resource requests/limits；
- HPA；
- PodDisruptionBudget；
- NetworkPolicy；
- kind 或 k3d 本地集群验证。

## Phase 7：持续交付——待完成

计划：

- 镜像推送 GHCR；
- Commit SHA / SemVer 镜像标签；
- 自动部署开发环境；
- Migration Job 发布顺序；
- 滚动更新；
- 部署后 Saga smoke；
- 失败阻断或回滚；
- 生产环境人工审批。

## Phase 8：生产治理——待完成

计划：

- 业务服务和迁移任务独立最小权限数据库账号；
- mTLS 或 Workload Identity；
- MySQL 与 RabbitMQ 备份恢复；
- `reconciliation_required` 自动对账；
- SLI / SLO / 告警；
- 压测、容量评估和慢 SQL 分析；
- 故障注入、恢复演练和 Runbook。

## 当前求职展示重点

项目讲解时应突出：

1. 为什么不能直接把原单体目录切成多个服务；
2. 如何识别商品、库存、订单的数据所有权；
3. 单库 ACID 失效后为什么引入库存预占和 Saga；
4. 为什么补偿失败必须进入 `reconciliation_required`；
5. 为什么 Outbox 仍是至少一次投递；
6. 多 Worker 如何通过租约和 `SKIP LOCKED` 协作；
7. 为什么迁移必须从业务进程生命周期中分离；
8. 为什么当前还不能宣称完成生产级云原生交付。
