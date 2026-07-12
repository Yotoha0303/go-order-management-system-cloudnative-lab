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
Prometheus 指标
  ↓
Grafana Dashboard 与告警规则
  ↓
OpenTelemetry 分布式追踪
  ↓
【当前下一阶段：持续交付与运行保障】
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

## Phase 7：可观测性——应用级基础已完成

### 7.1 Prometheus 指标基础

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
- Order、Outbox、Reconciliation 和卡住的 Saga；
- Worker 进程与 metrics listener；
- RabbitMQ Publisher Confirm outcome。

标签禁止 request/trace/user/order/reservation/event/Worker 实例 ID、原始 URL、查询字符串和错误消息。

Observability workflow 会运行完整 Order Saga，验证七个 targets 全部 `up` 并查询关键时序。

### 7.2 Grafana Dashboard 与告警规则

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

Dashboard 包含 16 个面板，覆盖 targets、服务请求率/5xx/p95、内部 HTTP、Order/Saga、Outbox、Reconciliation、RabbitMQ Confirm 和 Worker availability。

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

每条告警都包含显式 `for` 窗口，并由 promtool 触发/不触发夹具验证。当前规则不是生产 SLO，尚未接入 Alertmanager receiver。

### 7.3 OpenTelemetry 分布式追踪——已完成基础版

新增：

- OpenTelemetry SDK 和可选 OTLP/HTTP exporter；
- 无 exporter 时仍有效的本地 Trace Context；
- W3C `traceparent` / `tracestate` 提取与注入；
- 有界 HTTP server/client span；
- `X-Trace-ID` / `X-Span-ID` 响应诊断头；
- `trace_id` / `span_id` 结构化日志关联；
- `order.create_saga` 与 Worker batch/task/consume span；
- OpenTelemetry Collector、Tempo 和 Grafana Tempo datasource；
- 固定 Trace ID 的五服务跨服务 Trace 自动验收。

运行链路：

```text
Client W3C context
  -> API Gateway
  -> Identity / Catalog / Inventory / Order
  -> OTLP/HTTP Collector
  -> OTLP/gRPC Tempo
  -> Grafana Tempo datasource
```

Observability Stack workflow 会：

1. 校验 Compose、Dashboard、Provisioning、Prometheus 规则和高基数合同；
2. 启动完整应用、Prometheus、Grafana、Collector 和 Tempo；
3. 使用固定有效 W3C context 执行完整 Order Saga；
4. 验证 Prometheus 与 Grafana；
5. 通过 Tempo API 验证 Gateway、Identity、Catalog、Inventory、Order 位于同一 Trace；
6. 验证 `order.create_saga` 和有界 HTTP span name；
7. 拒绝 span name 中的数字资源 ID 或 UUID。

边界：RabbitMQ 消息尚未传播 W3C Context；无 baggage、tail sampling、生产采样策略、Kubernetes Trace backend 或 SQL statement tracing。

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
- Grafana 自动数据源、应用总览 Dashboard 和 API 自动验收；
- OpenTelemetry W3C HTTP Trace、Collector/Tempo、日志关联和跨服务 Trace 验收。

当前仍不等于完整生产级云原生系统。

## Phase 8：持续交付与运行保障——下一主阶段

- GHCR 版本化、不可变镜像；
- 测试环境自动部署和部署后 Smoke Test；
- 环境级错误版本回滚；
- MySQL 备份恢复；
- 故障演练、Runbook、压测和容量报告；
- 最小权限数据库账户、mTLS/Workload Identity。

## 后续增强

- RabbitMQ 消息 Trace Context；
- Alertmanager 通知、生产 SLO 和错误预算；
- tail sampling 与生产 Trace retention；
- MySQL/RabbitMQ/Kubernetes exporter；
- Kubernetes 内 Prometheus/Grafana/Collector/Tempo；
- Ingress Controller 真实流量与 TLS；
- HPA、NetworkPolicy、多节点故障；
- 托管云存储、LoadBalancer 和 Workload Identity。
