# 项目文档导航

本文档用于区分**当前微服务实现**、**历史单体基线**和**后续规划**，避免把旧设计误认为当前运行状态。

## 当前实现：优先阅读

| 文档 | 内容 |
| --- | --- |
| [../README.md](../README.md) | 当前项目定位、运行拓扑、快速启动和能力边界 |
| [architecture/microservices-v2-data-ownership.md](architecture/microservices-v2-data-ownership.md) | 四库数据所有权、服务间调用、库存预占和订单 Saga |
| [architecture/migrations-outbox-leasing.md](architecture/migrations-outbox-leasing.md) | 服务独立 Goose 迁移、Outbox 租约、多 Worker 和 Publisher Confirms |
| [architecture/http-timeout-retry.md](architecture/http-timeout-retry.md) | 端到端请求预算、传输超时、有限重试和安全边界 |
| [architecture/circuit-breaker-rate-limit.md](architecture/circuit-breaker-rate-limit.md) | 上游熔断状态机、Gateway Token Bucket 与运行边界 |
| [architecture/reliability-indicators.md](architecture/reliability-indicators.md) | Outbox/Saga 聚合指标、内部端点和周期结构化日志 |
| [architecture/cloud-native-status.md](architecture/cloud-native-status.md) | 当前云原生完成度、已完成能力和生产级缺口 |
| [verification/ci-baseline.md](verification/ci-baseline.md) | 当前 CI、Compose、双 Worker 和端到端 Saga 验证 |
| [verification/publisher-confirms.md](verification/publisher-confirms.md) | RabbitMQ Broker ACK 与 Outbox 状态验证 |
| [verification/http-timeout-retry.md](verification/http-timeout-retry.md) | 请求预算和有限重试验证计划 |
| [verification/circuit-breaker-rate-limit.md](verification/circuit-breaker-rate-limit.md) | 熔断状态、限流补充/清理和 HTTP 429 合约验证 |
| [verification/reliability-indicators.md](verification/reliability-indicators.md) | 两条聚合查询、内部鉴权和年龄边界验证 |
| [project_evolution.md](project_evolution.md) | 从原单体到当前微服务阶段的演进记录 |

## 当前运行路径

当前微服务实现主要位于：

```text
cmd/api-gateway
cmd/identity-service
cmd/catalog-service
cmd/inventory-service
cmd/order-service
cmd/order-timeout-worker

internal/catalogsvc
internal/inventorysvc
internal/ordersvc
internal/platform

migrations/identity
migrations/catalog
migrations/inventory
migrations/ordering
```

## 历史基线文档

以下文档用于说明项目在微服务改造前的单体设计，保留作为演进证据，但不代表当前运行形态：

- [architecture/current-state.md](architecture/current-state.md)
- [architecture/dependency-map.md](architecture/dependency-map.md)
- [architecture/data-ownership.md](architecture/data-ownership.md)
- [architecture/runtime-flow.md](architecture/runtime-flow.md)
- [architecture/transaction-boundaries.md](architecture/transaction-boundaries.md)
- [architecture/risk-register.md](architecture/risk-register.md)
- [architecture/microservices-v1.md](architecture/microservices-v1.md)

旧单体业务设计文档同样作为历史资料保留：

- [table_design.md](table_design.md)
- [order_flow.md](order_flow.md)
- [idempotency.md](idempotency.md)
- [cache_design.md](cache_design.md)
- [business_rules.md](business_rules.md)
- [api_list.md](api_list.md)

这些文件主要描述旧的 `handler → service → dao → model` 单体路径、共享数据库事务和 Redis 商品缓存。阅读当前实现时，应以微服务 v2 文档及代码为准。

## 测试和演示资料

- [test_plan.md](test_plan.md)：原单体和部分业务测试计划
- [test_result.md](test_result.md)：历史测试记录
- [http](http)：手动 HTTP 请求样例
- [evidence](evidence)：项目运行与测试证据
- `scripts/smoke/microservices-saga.sh`：当前完整微服务业务冒烟测试

## 文档维护规则

1. 根 README 只描述 `main` 当前可运行状态。
2. 当前架构变化必须同步更新微服务 v2、可靠性、迁移/Outbox 和云原生状态文档。
3. 历史文档不删除，但必须明确标记其对应阶段。
4. 不能把未通过 CI 或未在 Compose/Kubernetes 中实际运行的能力写成“已完成”。
5. Kubernetes、可观测性和持续部署完成后，应新增独立文档并更新云原生状态矩阵。
