# 项目文档导航

本文档区分**当前实现**、**验证证据**、**历史单体基线**和**后续规划**，避免把旧设计或尚未运行验收的能力误认为当前状态。

## 当前实现：优先阅读

| 文档 | 内容 |
| --- | --- |
| [../README.md](../README.md) | 当前项目定位、运行拓扑、启动方式和能力边界 |
| [architecture/microservices-v2-data-ownership.md](architecture/microservices-v2-data-ownership.md) | 四库数据所有权、服务调用、Inventory Reservation 和 Order Saga |
| [architecture/migrations-outbox-leasing.md](architecture/migrations-outbox-leasing.md) | 独立 Goose 迁移、Outbox 租约、多 Worker 和 Publisher Confirms |
| [architecture/http-timeout-retry.md](architecture/http-timeout-retry.md) | 请求预算、Transport 超时、有限重试和安全边界 |
| [architecture/circuit-breaker-rate-limit.md](architecture/circuit-breaker-rate-limit.md) | 操作级熔断与 Gateway Token Bucket 限流 |
| [architecture/reliability-indicators.md](architecture/reliability-indicators.md) | Outbox/Saga 聚合指标、内部端点和周期日志 |
| [architecture/reconciliation-worker.md](architecture/reconciliation-worker.md) | 对账任务、事务触发器、租约 Worker、修复动作和 dry-run |
| [architecture/kubernetes-foundation.md](architecture/kubernetes-foundation.md) | Kustomize base/local overlay、StatefulSet、Migration Job、Deployment、探针和 kind 脚本 |
| [architecture/cloud-native-status.md](architecture/cloud-native-status.md) | 云原生完成度、已完成能力和生产级缺口 |
| [project_evolution.md](project_evolution.md) | 从单体到微服务、可靠性收口和 Kubernetes 基础的演进记录 |

## 验证资料

| 文档或脚本 | 验证内容 |
| --- | --- |
| [verification/ci-baseline.md](verification/ci-baseline.md) | 当前 Go、迁移、镜像、Compose 和 Saga 质量门禁 |
| [verification/publisher-confirms.md](verification/publisher-confirms.md) | RabbitMQ Broker ACK 与 Outbox 状态 |
| [verification/http-timeout-retry.md](verification/http-timeout-retry.md) | 请求预算和有限重试 |
| [verification/circuit-breaker-rate-limit.md](verification/circuit-breaker-rate-limit.md) | 熔断状态、Token Bucket 和 HTTP 429 |
| [verification/reliability-indicators.md](verification/reliability-indicators.md) | 聚合查询、内部鉴权和年龄边界 |
| [verification/reconciliation-worker.md](verification/reconciliation-worker.md) | 对账映射、事务回滚、租约和修复动作 |
| `scripts/smoke/microservices-saga.sh` | Compose 完整微服务业务闭环 |
| `scripts/k8s/deploy-local.sh` | kind 本地部署流程；当前尚未进入 CI 实机验收 |

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

migrations/
├── identity
├── catalog
├── inventory
└── ordering

deploy/
├── docker
└── kubernetes
    ├── base
    └── overlays/local
```

## 当前验证边界

已实际通过 CI：

- lint、unit/integration test、race、vet、build；
- 单体历史迁移与四套服务迁移校验；
- Docker 镜像构建；
- Compose 四库、RabbitMQ、双类 Worker 副本和完整 Order Saga；
- Kubernetes local overlay 的 Kustomize 渲染。

尚未在 CI 实际完成：

- kind/k3d 集群部署；
- Kubernetes 上的完整 Saga；
- Ingress、PDB、HPA、NetworkPolicy；
- 错误版本发布与 `rollout undo` 演练。

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
2. “清单存在”“能够渲染”“集群已部署”“业务已验收”必须分开表述。
3. 架构变化必须同步更新当前架构、验证边界和演进记录。
4. 历史文档不删除，但必须明确阶段。
5. 未通过自动或可复现验收的能力不能写成“已完成”。
