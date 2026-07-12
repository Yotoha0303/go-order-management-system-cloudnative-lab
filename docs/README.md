# 项目文档导航

本文档区分**当前实现**、**验证证据**、**历史单体基线**和**后续规划**，避免把旧设计或尚未运行验收的能力误认为当前状态。

## 当前实现：优先阅读

| 文档 | 内容 |
| --- | --- |
| [../README.md](../README.md) | 当前项目定位、运行拓扑、Compose/kind/Prometheus 启动方式和能力边界 |
| [architecture/microservices-v2-data-ownership.md](architecture/microservices-v2-data-ownership.md) | 四库数据所有权、服务调用、Inventory Reservation 和 Order Saga |
| [architecture/migrations-outbox-leasing.md](architecture/migrations-outbox-leasing.md) | 独立 Goose 迁移、Outbox 租约、多 Worker 和 Publisher Confirms |
| [architecture/http-timeout-retry.md](architecture/http-timeout-retry.md) | 请求预算、Transport 超时、有限重试和安全边界 |
| [architecture/circuit-breaker-rate-limit.md](architecture/circuit-breaker-rate-limit.md) | 操作级熔断与 Gateway Token Bucket 限流 |
| [architecture/reliability-indicators.md](architecture/reliability-indicators.md) | Outbox/Saga 聚合指标、内部端点和周期日志 |
| [architecture/reconciliation-worker.md](architecture/reconciliation-worker.md) | 对账任务、事务触发器、租约 Worker、修复动作和 dry-run |
| [architecture/kubernetes-foundation.md](architecture/kubernetes-foundation.md) | Kustomize base/local/test、StatefulSet、Migration Job、Deployment、Ingress、PDB、kind 部署和回滚验收 |
| [architecture/prometheus-metrics.md](architecture/prometheus-metrics.md) | Prometheus registry、scrape endpoints、HTTP/Order/Outbox/Worker/RabbitMQ 指标、标签基数和部署合同 |
| [architecture/cloud-native-status.md](architecture/cloud-native-status.md) | 云原生完成度、已完成能力和生产级缺口 |
| [project_evolution.md](project_evolution.md) | 从单体到微服务、可靠性、Kubernetes 和可观测性基础的演进记录 |

## 验证资料

| 文档或脚本 | 验证内容 |
| --- | --- |
| [verification/ci-baseline.md](verification/ci-baseline.md) | 当前 Go、迁移、镜像、Compose 和 Saga 质量门禁 |
| [verification/publisher-confirms.md](verification/publisher-confirms.md) | RabbitMQ Broker ACK 与 Outbox 状态 |
| [verification/http-timeout-retry.md](verification/http-timeout-retry.md) | 请求预算和有限重试 |
| [verification/circuit-breaker-rate-limit.md](verification/circuit-breaker-rate-limit.md) | 熔断状态、Token Bucket 和 HTTP 429 |
| [verification/reliability-indicators.md](verification/reliability-indicators.md) | 聚合查询、内部鉴权和年龄边界 |
| [verification/reconciliation-worker.md](verification/reconciliation-worker.md) | 对账映射、事务回滚、租约和修复动作 |
| `scripts/smoke/microservices-saga.sh` | Compose 与 Kubernetes 共用的微服务业务断言 |
| `scripts/smoke/microservices-saga-kubernetes.sh` | Kubernetes Saga 包装入口 |
| `scripts/smoke/prometheus-metrics.py` | Prometheus readiness、七 target 健康和关键时序查询 |
| `scripts/k8s/deploy-local.sh` | 已进入 CI 实机验收的 kind 部署流程 |
| `deploy/kubernetes/overlays/test/README.md` | test overlay 的 Ingress、PDB、DNS、Secret 和控制器前置条件 |

## 当前代码与交付路径

```text
cmd/
├── api-gateway
├── identity-service
├── catalog-service
├── inventory-service
├── order-service
├── order-timeout-worker
└── order-reconciliation-worker

internal/
├── catalogsvc
├── inventorysvc
├── ordersvc
└── platform
    └── metrics

migrations/
├── identity
├── catalog
├── inventory
└── ordering

deploy/
├── docker
├── prometheus
└── kubernetes
    ├── base
    └── overlays
        ├── local
        └── test
```

## 当前验证边界

### 主 CI 已通过

- lint、unit/integration test、race、vet 和 build；
- 单体历史迁移与四套服务迁移校验；
- Docker 镜像构建；
- Compose 四库、RabbitMQ、双类 Worker 副本和完整 Order Saga；
- disposable kind 集群、StatefulSet、Migration Job 和七个 Deployment；
- Gateway NodePort 与内部 ClusterIP 边界；
- 不可用 Gateway revision、`kubectl rollout undo` 和恢复后的 Kubernetes Saga。

### Kubernetes Contracts workflow 已通过

- local/test overlay 渲染；
- Gateway Ingress、七个 PDB、七个 2 副本 Deployment；
- test overlay 无 NodePort；
- MySQL/RabbitMQ 无错误 PDB；
- Prometheus scrape annotations 和 Worker metrics ports 可渲染；
- 渲染 YAML 作为 artifact 保存。

### Prometheus Metrics workflow 已通过

- 默认 Compose 与 observability overlay 合并校验；
- 完整应用、两个双副本 Worker 和 Prometheus 启动；
- 完整 Order Saga；
- 七个 application scrape target 全部 `up`；
- HTTP server duration/count、Order、Outbox 和 Worker 指标可由 Prometheus API 查询；
- 失败时保存 Compose、target 和 query diagnostics。

### 尚未完成

- Grafana Dashboard 和正式 Prometheus alert rules；
- OpenTelemetry、W3C Trace Context 与 `trace_id` / `span_id` 日志字段；
- RabbitMQ consumer 细粒度计数和基础设施 exporter；
- Ingress Controller 真实流量、TLS、HPA、NetworkPolicy 和多节点故障；
- Registry 不可变镜像、测试环境 CD 和正式环境 overlay；
- 托管云存储、负载均衡和 Workload Identity。

## 历史基线文档

以下内容保留为微服务改造前的演进证据，不代表当前运行形态：

- [architecture/current-state.md](architecture/current-state.md)
- [architecture/dependency-map.md](architecture/dependency-map.md)
- [architecture/data-ownership.md](architecture/data-ownership.md)
- [architecture/runtime-flow.md](architecture/runtime-flow.md)
- [architecture/transaction-boundaries.md](architecture/transaction-boundaries.md)
- [architecture/risk-register.md](architecture/risk-register.md)
- [architecture/microservices-v1.md](architecture/microservices-v1.md)
- [table_design.md](table_design.md)
- [order_flow.md](order_flow.md)
- [idempotency.md](idempotency.md)
- [cache_design.md](cache_design.md)
- [business_rules.md](business_rules.md)
- [api_list.md](api_list.md)

## 文档维护规则

1. 根 README 只描述 `main` 的真实状态。
2. “代码存在”“能够渲染”“组件已启动”“业务或指标已验收”必须分开表述。
3. 架构变化必须同步更新当前架构、验证边界和演进记录。
4. 历史文档不删除，但必须明确阶段。
5. 未通过自动或可复现验收的能力不能写成“已完成”。
