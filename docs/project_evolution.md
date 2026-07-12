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
Prometheus 指标基础
  ↓
Grafana Dashboard 与告警规则
  ↓
【当前下一阶段：OpenTelemetry 分布式追踪】
  ↓
持续交付与运行保障
```

## Phase 0：原单体业务基本盘

完成用户、商品、库存、订单、JWT、最小 RBAC、MySQL 本地事务、行锁、Redis cache-aside、RabbitMQ TTL/DLX、Transactional Outbox、Docker、Compose、Goose 和 GitHub Actions。

这一阶段的一致性依赖单个 MySQL 本地事务。

## Phase 1：迁移基线与架构审计

完成：

- 保留原仓库 Git 历史并建立实验仓库；
- 修复 CI 与数据库命名漂移；
- 固化单体依赖、数据所有权、事务边界和风险；
- 建立可重复 Compose 和 CI 验证入口。

该阶段不改变业务行为，只建立后续演进基线。

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

每个运行单元拥有独立二进制和 Docker 镜像。初始限制是服务进程已拆分但仍共享 MySQL。

## Phase 3：Microservices v2——数据所有权与 Saga

### 独立数据库

```text
Identity  -> go_order_identity
Catalog   -> go_order_catalog
Inventory -> go_order_inventory
Ordering  -> go_order_ordering
```

### 服务间协作

- Catalog / Inventory → Identity：角色校验；
- Order → Catalog：商品快照；
- Order → Inventory：预占、确认和释放库存；
- Timeout Worker → Order：超时取消。

### 一致性模型

```text
Inventory reservation:
pending -> confirmed / released

Order:
reserving -> pending -> paid / cancelled
reserving -> failed
uncertain -> reconciliation_required
```

单库 ACID 被替换为本地事务、幂等远程动作、Saga 状态机和补偿。

## Phase 4：独立迁移与 Outbox 多 Worker

### 服务独立 Goose migration

```text
migrations/identity
migrations/catalog
migrations/inventory
migrations/ordering
```

业务进程不再执行 `AutoMigrate`，Compose 和 Kubernetes 使用一次性迁移任务。

### Outbox 租约

```text
lease_owner
lease_until
next_attempt_at
FOR UPDATE SKIP LOCKED
```

实现多副本独占领取、崩溃租约回收、失败延迟重试和真实 MySQL 并发测试。

## Phase 5：应用可靠性收口——已完成

### 5.1 RabbitMQ Publisher Confirms

- 发布与消费 Channel 分离；
- Broker ACK 后才标记 Outbox `published`；
- NACK、确认超时、Channel 关闭和发布错误进入失败重试；
- 保留 at-least-once，不宣称 exactly-once。

### 5.2 HTTP 请求预算与有限重试

```text
X-Request-ID
X-Request-Deadline
```

实现 Transport 细分超时、有限重试、指数退避、剩余预算检查和幂等操作边界。Gateway 不自动重放外部业务请求。

### 5.3 熔断与 Gateway 限流

- 按 `<upstream>/<operation>` 隔离熔断状态；
- 基础设施错误计数，业务 4xx 不打开熔断；
- Gateway 客户端和全局 Token Bucket；
- HTTP 429 与 `Retry-After`。

### 5.4 可靠性聚合指标

Order Service 提供 Outbox 与 Order/Saga 聚合快照、内部只读端点和 Worker 周期结构化日志。

### 5.5 自动 Reconciliation Worker

- 结构化 `order_reconciliation_tasks`；
- 状态变更和任务创建同一事务；
- 租约、`SKIP LOCKED`、多副本、过期租约回收；
- 幂等 confirm/release；
- 有界指数重试；
- `unresolved` 保留未知场景；
- 非变更式 dry-run。

## Phase 6：Kubernetes 基础——已完成

### 6.1 Kustomize 基础

新增 base/local overlay，包含 Namespace、ConfigMap/Secret 合同、MySQL/RabbitMQ StatefulSet、四个 Migration Job、七个 Deployment、Service、Probe、resources、RollingUpdate 和 Schema 等待 initContainer。

### 6.2 真实 kind 部署、恢复与 Saga

CI 从干净 runner 创建 kind 集群，构建并加载七个镜像，等待基础设施、迁移和应用，验证暴露边界与双 Worker，并执行：

```text
unavailable Gateway revision
    ↓
rollout failure detected
    ↓
kubectl rollout undo
    ↓
Gateway readiness recovered
    ↓
complete Kubernetes Order Saga
```

实际验收发现并修复 MySQL Pod Socket/TCP 差异和 Gateway 聚合 readiness 短暂抖动。

### 6.3 Test overlay、Ingress 与 PDB

新增：

- `deploy/kubernetes/overlays/test`；
- 一个 `nginx` Gateway Ingress；
- 七个 2 副本应用 Deployment；
- 七个 `minAvailable: 1` PDB；
- 独立 Kubernetes Contracts workflow。

当前验证的是交付合同，不等于已安装 Ingress Controller、TLS 或完成真实 Ingress 流量验收。

## Phase 7：可观测性

### 7.1 Prometheus 指标基础——已完成

新增标准库实现的 Prometheus text registry，支持 Counter、Gauge、固定桶 Histogram 和 scrape-time Collector。

Scrape endpoints：

```text
api-gateway                  :8082/metrics
identity-service             :8083/metrics
catalog-service              :8084/metrics
inventory-service            :8085/metrics
order-service                :8086/metrics
order-timeout-worker         :9091/metrics
order-reconciliation-worker  :9092/metrics
```

指标覆盖：

- HTTP server 请求数、响应字节和耗时；
- HTTP client 每次真实网络尝试、结果、状态类别和耗时；
- Order 状态、Outbox 状态、积压、租约、最老年龄和失败尝试；
- Reconciliation 数量和卡住的 Saga；
- Worker 进程与 metrics listener；
- RabbitMQ Publisher Confirm 的 ACK/NACK/timeout/channel-close/publish-error。

标签基数规则明确禁止 request/trace/user/order/reservation/event/Worker 实例 ID、原始 URL、查询字符串和错误消息。

Compose 与 Kubernetes 合同：

- `compose.observability.yml` 可选叠加 Prometheus；
- `deploy/prometheus/prometheus.yml` 定义七个应用 scrape target；
- Kubernetes base 为七个应用 Pod 添加 scrape annotations；
- 两个 Worker 声明 metrics container port。

Phase 7.1 CI 会启动完整业务拓扑和 Prometheus，运行完整 Order Saga，验证七个 targets 全部 `up` 并查询关键时序。

### 7.2 Grafana Dashboard 与告警规则——已完成

新增自动 Provisioning：

```text
deploy/grafana/provisioning/datasources/prometheus.yml
deploy/grafana/provisioning/dashboards/dashboards.yml
deploy/grafana/dashboards/go-order-overview.json
```

稳定合同：

```text
Grafana datasource UID: prometheus
Dashboard UID:           go-order-overview
Dashboard title:         Go Order Management Overview
```

Dashboard 包含 16 个面板，覆盖：

- 七个应用 target 可用性；
- 服务请求率、5xx 比例和 p95 延迟；
- 内部 HTTP 调用；
- Order 状态与 Saga 卡住状态；
- Outbox 状态、overdue 和最老可执行年龄；
- Reconciliation backlog；
- RabbitMQ Publisher Confirm outcome；
- Worker availability。

新增六条 recording rules：

```text
service:http_requests:rate5m
service:http_server_errors:rate5m
service:http_server_error_ratio:rate5m
service:http_server_request_duration_seconds:p95
service:http_client_attempts:rate5m
worker:up:max
```

新增九条基础 alert rules：

```text
GoOrderTargetDown
GoOrderWorkerDown
GoOrderElevatedHTTP5xxRatio
GoOrderHighP95Latency
GoOrderOutboxOverdue
GoOrderOutboxActionableAgeHigh
GoOrderReconciliationBacklog
GoOrderSagaStuck
GoOrderMetricsCollectionFailing
```

每条告警都包含显式 `for` 窗口。健康服务无 5xx 时会输出明确的零错误率序列，而不是缺失时序。

Observability Stack workflow 会：

1. 校验 Compose、Dashboard、Provisioning、规则名称和高基数标签合同；
2. 运行 `promtool check config`；
3. 运行 target down 触发/不触发与 Outbox overdue 触发夹具；
4. 启动完整应用、Prometheus 和 Grafana；
5. 执行完整 Order Saga；
6. 验证七个 targets、四个规则组、九条告警和 recording series；
7. 通过 Grafana API 验证数据源和文件 Provisioning Dashboard。

边界：当前规则是实验项目默认阈值，不是生产 SLO；尚未接入 Alertmanager receiver 或通知渠道。

### 7.3 OpenTelemetry 分布式追踪——待完成

- OpenTelemetry SDK 和 OTLP exporter；
- W3C Trace Context；
- HTTP server/client span；
- Gateway 到业务服务的跨服务 Trace；
- `trace_id` / `span_id` 结构化日志关联；
- Worker、RabbitMQ 和 Saga span/event 设计；
- Collector 与 Trace backend；
- Trace 合同和端到端验收。

其他可观测性增强：

- Alertmanager receiver 和通知路由；
- 生产 SLO、错误预算和容量阈值；
- RabbitMQ consumer 细粒度指标；
- MySQL、RabbitMQ、Kubernetes 基础设施 exporter；
- Kubernetes 内 Prometheus/Grafana 部署。

## 当前项目状态

当前已经具备：

- 服务和数据所有权拆分；
- Inventory Reservation、Order Saga、补偿和自动对账；
- Outbox、Publisher Confirms、多 Worker 租约；
- 请求预算、有限重试、熔断和限流；
- Compose 与 Kubernetes 双环境完整 Saga；
- kind 失败 rollout 与 undo；
- Ingress/PDB/test overlay 合同；
- Prometheus 应用指标、七 target 抓取和 recording/alert rules；
- Grafana 自动数据源、应用总览 Dashboard 和 API 自动验收。

当前仍不等于完整生产级云原生系统。

## Phase 8：持续交付与运行保障——待完成

- GHCR 版本化镜像；
- 测试环境自动部署和部署后 Smoke Test；
- 环境级错误版本回滚；
- MySQL 备份恢复；
- 故障演练、Runbook、压测和容量报告；
- 最小权限数据库账户、mTLS/Workload Identity。

## Kubernetes 后续增强

- Ingress Controller 真实流量与 TLS；
- HPA；
- NetworkPolicy；
- 多节点和节点失效验证；
- 不可变 Registry 镜像；
- 托管云存储、LoadBalancer 和 Workload Identity。
