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
- Grafana Prometheus 数据源和应用总览 Dashboard 自动 Provisioning；
- `promtool` 规则夹具、Prometheus API 与 Grafana API 自动验收。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、熔断、限流、自动对账、可重复 Kubernetes 部署/回滚，以及 Prometheus/Grafana 应用级监控与基础告警规则的云原生演进实验项目。

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
| Kubernetes scrape 合同 | 已完成基础版 | Pod annotations 与 Worker metrics ports，可由 local/test overlay 渲染 |
| 分布式追踪 | 未完成 | 无 OpenTelemetry、W3C Trace Context、trace/span 日志字段 |
| 告警通知与 SLO | 未完成 | 无 Alertmanager receiver、通知路由、生产 SLO 和错误预算 |
| 基础设施监控 | 未完成 | 无 MySQL/RabbitMQ exporter、kube-state-metrics 和 Kubernetes 监控栈 |
| 持续部署 | 未完成 | 无 GHCR、自动测试环境部署和不可变版本推广 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无正式备份恢复演练、Runbook、压测报告和故障演练 |

## Prometheus 与 Grafana 已验证范围

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

### Observability Stack workflow

该 workflow 会：

1. 合并默认 Compose 与 observability overlay；
2. 校验 Dashboard、Provisioning、规则名称和禁止标签合同；
3. 运行 `promtool check config`；
4. 运行 target down 的触发/不触发和 Outbox overdue 触发夹具；
5. 启动完整应用、两个双副本 Worker、Prometheus 和 Grafana；
6. 运行完整 Order Saga；
7. 验证七个 application targets 全部 `up`；
8. 验证四个规则组和九条 alert rules 健康；
9. 查询基础时序与六类 recording series；
10. 通过 Grafana API 验证默认 Prometheus 数据源与文件 Provisioning Dashboard；
11. 失败时上传 Compose、Prometheus 和 Grafana diagnostics。

### Recording rules

```text
service:http_requests:rate5m
service:http_server_errors:rate5m
service:http_server_error_ratio:rate5m
service:http_server_request_duration_seconds:p95
service:http_client_attempts:rate5m
worker:up:max
```

健康服务没有 5xx 时，错误率规则会输出明确的零值序列，避免 Dashboard 和告警出现无数据状态。

### Alert rules

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

每条告警都有显式 `for` 窗口，以降低瞬时抖动造成的噪声。当前阈值是实验项目默认值，不代表生产 SLO。

### Grafana Provisioning

```text
Datasource UID: prometheus
Dashboard UID:  go-order-overview
Dashboard:      Go Order Management Overview
```

Dashboard 通过仓库文件管理并自动加载，无需手动导入。面板覆盖 target、HTTP RED 信号、内部调用、Order/Saga、Outbox、Reconciliation、RabbitMQ Publisher Confirm 和 Worker availability。

## 当前观测边界

尚未完成：

- Alertmanager receiver、通知渠道和升级策略；
- 生产 SLO、错误预算和容量阈值；
- OpenTelemetry SDK/exporter；
- W3C Trace Context 和跨服务 Trace；
- `trace_id` / `span_id` 日志关联；
- RabbitMQ consumer delivery 细粒度计数；
- MySQL、RabbitMQ、Node 和 Kubernetes exporter；
- Prometheus Operator / ServiceMonitor；
- Kubernetes 内 Prometheus、Grafana 和长期存储；
- HA Prometheus 或远程存储。

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
- [x] Grafana 自动 Provisioning Dashboard；
- [x] Prometheus recording rules；
- [x] Prometheus alert rules 与 promtool 夹具；
- [ ] OpenTelemetry；
- [ ] W3C Trace Context；
- [ ] `trace_id` / `span_id` 日志字段；
- [ ] Alertmanager 通知与生产 SLO；
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
| 完整可观测性 | 约 68%–75% |
| 云原生应用基础 | 约 93%–95% |
| 生产级云原生交付 | 约 70%–77% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、熔断、限流、自动对账和独立迁移；建立 Compose/kind 双环境端到端 CI，为七个运行单元实现有限基数 Prometheus 指标与 Outbox/Saga 业务 Gauge，并构建自动 Provisioning 的 Grafana 总览 Dashboard、recording rules、九条带持续窗口的 Prometheus 告警及 promtool/Grafana API 自动验收。
