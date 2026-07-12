# 云原生完成度与缺口

## 结论

当前项目已经完成：

- 微服务核心改造与四库数据所有权；
- 应用可靠性收口；
- Compose 完整业务验证；
- Kubernetes base/local/test overlays；
- disposable kind 部署、失败 revision 检测、`rollout undo` 和完整 Kubernetes Saga；
- Gateway Ingress 与多副本 PDB 交付合同；
- Prometheus 应用指标基础；
- 七个应用 target 的真实抓取和关键时序自动验收；
- Kubernetes Pod scrape annotations 与 Worker metrics ports。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、熔断、限流、自动对账、可重复 Kubernetes 部署/回滚，以及 Prometheus 应用指标和自动抓取验证的云原生演进实验项目。

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
| Worker 弹性 | 已完成基础版 | 租约、`SKIP LOCKED`、多副本、租约恢复 |
| 自动对账 | 已完成基础版 | 结构化任务、事务触发器、已知修复动作、多副本、dry-run |
| HTTP 可靠性 | 已完成基础版 | Request ID、deadline、细分超时、有限重试、操作级熔断 |
| 入口保护 | 已完成基础版 | Gateway 客户端/全局 Token Bucket、429/Retry-After |
| 数据库迁移 | 已完成 | 服务独立 Goose migration、Compose Job 和 Kubernetes Job |
| Compose CI | 已完成 | lint、test、race、build、迁移、镜像、四库、四个 Worker 副本、完整 Saga |
| Kubernetes 基础 | 已完成基础版 | StatefulSet、Deployment、Service、Migration Job、Probe、resources、Ingress、PDB、local/test overlays |
| Kubernetes 运行验收 | 已完成基础版 | kind 部署、暴露面、双 Worker、失败 revision、undo 与完整 Saga |
| Prometheus endpoints | 已完成基础版 | 5 个 HTTP 服务 `/metrics`，2 个 Worker 独立 metrics listener |
| HTTP 指标 | 已完成基础版 | server count/bytes/duration、client attempt/outcome/duration，有限标签 |
| Saga/Outbox 指标 | 已完成基础版 | 状态、积压、租约、超时、年龄、失败尝试、对账和卡住状态 Gauge |
| Worker/RabbitMQ 指标 | 已完成基础版 | Worker up/listener、Publisher Confirm outcome/duration |
| Prometheus 运行验收 | 已完成基础版 | Compose Prometheus、7 targets up、Saga 后关键 metric query |
| Kubernetes scrape 合同 | 已完成基础版 | Pod annotations 与 Worker metrics ports，可由 local/test overlay 渲染 |
| Grafana 与告警 | 未完成 | 无 Dashboard、recording rule 和正式 alert rule |
| 分布式追踪 | 未完成 | 无 OpenTelemetry、W3C Trace Context、trace/span 日志字段 |
| 持续部署 | 未完成 | 无 GHCR、自动测试环境部署和不可变版本推广 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无正式备份恢复演练、Runbook、压测报告和故障演练 |

## Prometheus 已验证范围

### Scrape targets

```text
api-gateway                  :8082/metrics
identity-service             :8083/metrics
catalog-service              :8084/metrics
inventory-service            :8085/metrics
order-service                :8086/metrics
order-timeout-worker         :9091/metrics
order-reconciliation-worker  :9092/metrics
```

Prometheus Metrics workflow 会：

1. 合并默认 Compose 与 observability overlay；
2. 启动完整应用拓扑、两个双副本 Worker 和 Prometheus；
3. 运行完整 Order Saga；
4. 验证七个 application targets 全部 `up`；
5. 查询 HTTP server count/duration、Order、Outbox 和 Worker 指标；
6. 失败时上传 targets、query 和 Compose diagnostics。

### 指标域

```text
go_order_http_server_requests_total
go_order_http_server_response_bytes_total
go_order_http_server_request_duration_seconds
go_order_http_client_attempts_total
go_order_http_client_attempt_duration_seconds
go_order_orders{status}
go_order_outbox_events{status}
go_order_reconciliation_required
go_order_saga_stuck_transient
go_order_worker_up{worker}
go_order_rabbitmq_publish_total{outcome}
go_order_rabbitmq_publish_duration_seconds{outcome}
```

标签明确禁止使用：request ID、trace ID、用户/订单/预占/事件 ID、Worker 实例 ID、原始 URL、查询字符串和错误消息。

### 边界

当前指标 registry 使用标准库输出 Prometheus text format，足以支持本项目的固定指标合同和并发测试。当前没有：

- Grafana；
- alert rules；
- Prometheus Operator / ServiceMonitor；
- Kubernetes 内 Prometheus Server；
- RabbitMQ exporter、MySQL exporter、kube-state-metrics；
- consumer delivery 的细粒度计数；
- 长期保留、HA Prometheus 或远程存储。

## Kubernetes 已验证范围

主 CI 会创建干净 kind 集群，运行基础设施、四个 Migration Job、七个 Deployment，验证 Service 暴露和双 Worker，执行失败 rollout、`kubectl rollout undo` 和完整 Kubernetes Saga。

Kubernetes Contracts workflow 会验证 local/test overlays、Ingress、七个 PDB、副本数、ClusterIP/NodePort 边界，以及 Prometheus annotations/ports 可正确渲染。

Ingress Controller、TLS、HPA、NetworkPolicy、多节点故障和托管云集成仍属于增强项。

## 阶段判断

### Phase 5：可靠性收口

已完成。

### Phase 6：Kubernetes 基础

已完成原始跟踪范围。

### Phase 7：完整可观测性

部分完成：

- [x] Prometheus 应用指标；
- [x] Compose Prometheus 抓取；
- [x] 七 target 与关键指标自动验收；
- [x] Kubernetes scrape 合同；
- [ ] Grafana Dashboard；
- [ ] Prometheus alert rules；
- [ ] OpenTelemetry；
- [ ] W3C Trace Context；
- [ ] `trace_id` / `span_id` 日志字段；
- [ ] RabbitMQ consumer 与基础设施 exporter。

### Phase 8：持续交付与运行保障

尚未完成：GHCR、测试环境 CD、部署后 Smoke、环境级回滚、MySQL 备份恢复、故障演练、Runbook、压测与最小权限身份体系。

## 完成度估算

以下为工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 94% |
| 容器化应用开发 | 约 94% |
| 应用可靠性基础 | 约 95% |
| Kubernetes 基础部署与合同 | 约 88%–92% |
| 完整可观测性 | 约 45%–55% |
| 云原生应用基础 | 约 92%–94% |
| 生产级云原生交付 | 约 68%–75% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、熔断、限流、自动对账和独立迁移；建立 Compose/kind 双环境端到端 CI，并为七个运行单元实现有限基数 Prometheus 指标、Outbox/Saga 业务 Gauge、RabbitMQ Confirm 指标及七 target 自动抓取验收。
