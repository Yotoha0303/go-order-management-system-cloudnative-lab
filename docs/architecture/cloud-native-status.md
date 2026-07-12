# 云原生完成度与缺口

## 结论

当前项目已经完成：

- 微服务核心改造与四库数据所有权；
- 应用可靠性收口；
- Compose 完整业务验证；
- Kubernetes base/local/test overlays；
- disposable kind 部署、失败 revision 检测、`rollout undo` 和完整 Kubernetes Saga；
- Gateway Ingress 与多副本 PDB 交付合同；
- 七个运行单元的 Prometheus 指标和真实抓取验收；
- 六条 recording rules 与九条基础 alert rules；
- Grafana Prometheus/Tempo 数据源和应用总览 Dashboard 自动 Provisioning；
- OpenTelemetry SDK、W3C HTTP Trace Context、OTLP Collector、Tempo 和 Trace/Log 关联；
- Prometheus、Grafana 和 Tempo API 自动验收。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、熔断、限流、自动对账、可重复 Kubernetes 部署/回滚，以及 Prometheus/Grafana/OpenTelemetry 应用级可观测性的云原生演进实验项目。

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
| Prometheus 运行验收 | 已完成基础版 | Compose Prometheus、7 targets up、规则健康、Saga 后关键 metric query |
| Recording rules | 已完成基础版 | 请求率、5xx、错误率、p95、上游尝试和 Worker availability |
| Prometheus alert rules | 已完成基础版 | 9 条规则、显式 `for` 窗口、promtool 触发/不触发夹具 |
| Grafana Dashboard | 已完成基础版 | 自动数据源、稳定 UID、16 个应用/Saga/Outbox/Worker 面板、API 验收 |
| OpenTelemetry SDK | 已完成基础版 | 所有 HTTP 服务和 Worker；无 exporter 时仍生成本地 Trace Context |
| W3C HTTP 传播 | 已完成基础版 | `traceparent` / `tracestate` 提取与注入、响应 Trace/Span ID |
| OTLP 与 Trace backend | 已完成本地版 | OTLP/HTTP Collector、OTLP/gRPC Tempo、Grafana Tempo datasource |
| Trace/Log 关联 | 已完成基础版 | Context-aware `slog` 自动附加 `trace_id` / `span_id` |
| 跨服务 Trace 验收 | 已完成基础版 | 固定 Trace ID 下验证 Gateway、Identity、Catalog、Inventory、Order 和 `order.create_saga` |
| Kubernetes scrape 合同 | 已完成基础版 | Pod annotations 与 Worker metrics ports，可由 local/test overlay 渲染 |
| 消息 Trace 传播 | 未完成 | RabbitMQ 消息尚未携带 W3C Trace Context |
| 告警通知与 SLO | 未完成 | 无 Alertmanager receiver、通知路由、生产 SLO 和错误预算 |
| 基础设施监控 | 未完成 | 无 MySQL/RabbitMQ exporter、kube-state-metrics 和 Kubernetes 监控栈 |
| 持续部署 | 未完成 | 无 GHCR、自动测试环境部署和不可变版本推广 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无正式备份恢复演练、Runbook、压测报告和故障演练 |

## Observability Stack 已验证范围

### Prometheus 与 Grafana

```text
api-gateway                  :8082/metrics
identity-service             :8083/metrics
catalog-service              :8084/metrics
inventory-service            :8085/metrics
order-service                :8086/metrics
order-timeout-worker         :9091/metrics
order-reconciliation-worker  :9092/metrics
```

Workflow 会验证：

- Compose overlay、Dashboard、Provisioning、规则和禁止标签合同；
- `promtool check config`；
- target down 触发/不触发和 Outbox overdue 规则夹具；
- 七个 targets 全部 `up`；
- 四个规则组、九条 alert rules 和六类 recording series；
- Grafana Prometheus/Tempo 数据源；
- `go-order-overview` Dashboard 自动 Provisioning。

### OpenTelemetry 与 Tempo

运行链路：

```text
Client W3C context
  -> API Gateway
  -> Identity / Catalog / Inventory / Order
  -> OTLP/HTTP Collector
  -> OTLP/gRPC Tempo
  -> Grafana Tempo datasource
```

自动验收使用固定有效 Trace ID 运行完整 Order Saga，并要求：

- Gateway、Identity、Catalog、Inventory、Order 位于同一 Trace；
- 至少十个 spans；
- 存在 `order.create_saga`；
- 存在有界 `POST api_orders` HTTP span；
- span name 不包含数字资源 ID 或 UUID。

无 OTLP endpoint 时，服务仍安装 SDK provider 并生成有效 Trace/Span ID，只是不向 backend 导出。

## 当前观测边界

尚未完成：

- RabbitMQ 消息头中的 W3C Trace Context；
- baggage、tail sampling 和生产采样策略；
- 生产 Trace retention、HA backend 或托管平台；
- Alertmanager receiver、通知渠道和升级策略；
- 生产 SLO、错误预算和容量阈值；
- RabbitMQ consumer delivery 细粒度计数；
- MySQL、RabbitMQ、Node 和 Kubernetes exporter；
- Prometheus Operator / ServiceMonitor；
- Kubernetes 内 Prometheus、Grafana、Collector、Tempo 和长期存储；
- SQL statement tracing。

## Kubernetes 已验证范围

主 CI 会创建干净 kind 集群，运行基础设施、四个 Migration Job、七个 Deployment，验证 Service 暴露和双 Worker，执行失败 rollout、`kubectl rollout undo` 和完整 Kubernetes Saga。

Kubernetes Contracts workflow 会验证 local/test overlays、Ingress、七个 PDB、副本数、ClusterIP/NodePort 边界，以及 Prometheus annotations/ports 可正确渲染。

Ingress Controller、TLS、HPA、NetworkPolicy、多节点故障和托管云集成仍属于增强项。

## 阶段判断

### Phase 5：可靠性收口

已完成。

### Phase 6：Kubernetes 基础

已完成原始跟踪范围。

### Phase 7：应用级可观测性基础

已完成原始核心范围：

- [x] Prometheus 应用指标与七 target 抓取；
- [x] Kubernetes scrape 合同；
- [x] Grafana 自动 Provisioning Dashboard；
- [x] Prometheus recording/alert rules 与 promtool 夹具；
- [x] OpenTelemetry SDK 和 OTLP export；
- [x] W3C HTTP Trace Context；
- [x] `trace_id` / `span_id` 日志字段；
- [x] Collector/Tempo 与五服务 Trace 自动验收；
- [ ] RabbitMQ 消息 Trace Context；
- [ ] Alertmanager 通知与生产 SLO；
- [ ] 基础设施 exporter 与 Kubernetes 观测栈。

后面三项属于生产增强，不阻止应用级 Phase 7 基础收口。

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
| 应用级可观测性 | 约 88%–92% |
| 云原生应用基础 | 约 95%–97% |
| 生产级云原生交付 | 约 74%–80% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、熔断、限流、自动对账和独立迁移；建立 Compose/kind 双环境端到端 CI，为七个运行单元实现有限基数 Prometheus 指标、Grafana Dashboard、recording/alert rules，并通过 OpenTelemetry W3C Context、OTLP Collector、Tempo 和结构化日志关联完成五服务跨服务 Trace 自动验收。
