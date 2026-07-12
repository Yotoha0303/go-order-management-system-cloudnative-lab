# 云原生完成度与缺口

## 结论

当前项目已经完成：

- 微服务核心改造；
- 容器化与四库数据所有权；
- 应用可靠性收口；
- Docker Compose 完整业务验证；
- Kubernetes Kustomize base、local 和 test overlays；
- disposable kind 集群自动部署；
- 失败 revision 检测、`kubectl rollout undo` 和完整 Kubernetes Saga；
- Gateway Ingress 交付合同；
- 多副本应用 PodDisruptionBudget 合同；
- local/test overlay 自动渲染与资源边界验证。

最准确的定位是：

> 具备独立服务与数据库、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker、请求预算、有限重试、基础熔断、Gateway 限流、运行指标、自动对账，以及可重复 Kubernetes 部署、回滚和交付合同验证的云原生演进实验项目。

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
| 自动对账 | 已完成基础版 | 结构化任务、事务触发器、已知修复动作、租约 Worker、多副本、dry-run |
| HTTP 可靠性 | 已完成基础版 | Request ID、deadline、细分超时、有限重试、操作级熔断 |
| 入口保护 | 已完成基础版 | Gateway 客户端/全局 Token Bucket、429/Retry-After |
| 运行指标 | 已完成基础版 | Outbox/Saga 聚合 SQL、内部端点、周期结构化日志 |
| 数据库迁移 | 已完成 | 服务独立 Goose migration、Compose Job 和 Kubernetes Job |
| Compose CI | 已完成 | lint、test、race、build、迁移、镜像、四库、四个 Worker 副本、完整 Saga |
| Kubernetes 清单 | 已完成基础版 | Namespace、ConfigMap/Secret、StatefulSet、Deployment、Service、Migration Job、Probe、资源限制、Ingress、PDB |
| Kubernetes 运行验收 | 已完成基础版 | kind 部署、迁移、rollout、暴露面、双 Worker、失败 revision、undo 与完整 Saga |
| Kubernetes 环境合同 | 已完成基础版 | local NodePort overlay、test Ingress/PDB overlay、独立契约 CI |
| Kubernetes 弹性治理 | 部分完成 | 有 RollingUpdate 和 PDB；无 HPA、NetworkPolicy、多节点故障验证 |
| 完整可观测性 | 未完成 | 无 Prometheus、Grafana、OpenTelemetry 和正式告警 |
| 持续部署 | 未完成 | 无 GHCR 发布、自动测试环境部署和不可变版本推广 |
| 生产安全 | 未完成 | 静态内部 Token、数据库 root 账号、无 mTLS/Workload Identity |
| 运行保障 | 未完成 | 无正式备份恢复演练、Runbook、压测报告和故障演练 |

## Kubernetes 已验证范围

### Local runtime

CI 会：

1. 创建干净 kind 集群；
2. 构建并加载 7 个应用镜像；
3. 应用 local overlay；
4. 等待 MySQL、RabbitMQ 和 4 个 Migration Job；
5. 等待 7 个 Deployment；
6. 验证只有 Gateway 使用 NodePort；
7. 验证两个 Timeout Worker 和两个 Reconciliation Worker；
8. 发布不可用 Gateway revision 并确认 rollout 失败；
9. 执行 `kubectl rollout undo`；
10. 确认原镜像和 Gateway readiness 恢复；
11. 执行完整 Kubernetes Order Saga；
12. 清理集群，失败时上传诊断。

### Test delivery contract

独立 Kubernetes Contracts workflow 会验证：

- local 和 test overlay 都能由 Kustomize 渲染；
- test overlay 只有一个 Gateway Ingress；
- Ingress 使用 `nginx` class 和 `go-order.test.local`；
- Gateway、四个业务服务和两个 Worker 类型均为 2 副本；
- 七个 PDB 均为 `minAvailable: 1`；
- test overlay 不包含 NodePort；
- MySQL 和 RabbitMQ 不会被错误应用 PDB；
- 渲染结果作为 CI artifact 保存。

该验证证明清单和资源合同正确，但不等于真实 Ingress Controller 已安装，也不等于 Ingress 流量已在 CI 中通过。

## 应用可靠性链

### 异步发布

```text
Order local transaction
  -> timeout Outbox
  -> lease claim
  -> RabbitMQ publish
  -> Broker ACK
  -> published
```

NACK、确认超时、通道关闭和直接发布错误都会进入失败重试。语义仍是 at-least-once。

### 同步调用

内部客户端具备：

- Request ID 和绝对 deadline 传播；
- TCP、TLS、响应头和总超时；
- 502/503/504 与选定网络错误的有限重试；
- 指数退避与有限抖动；
- 剩余预算判断；
- 按 `<upstream>/<operation>` 隔离的熔断器。

Gateway 不重放外部业务请求。

### 自动对账

Reconciliation Worker：

- 使用租约和 `FOR UPDATE SKIP LOCKED`；
- 支持多个副本和过期租约恢复；
- 复用 deadline、重试和熔断；
- 重复执行幂等 release/confirm；
- 本地订单最终状态、Outbox 完成和任务完成同事务；
- 未知动作保持 `unresolved`；
- 支持非变更式 dry-run。

## 阶段判断

### Phase 5：可靠性收口

已完成。

### Phase 6：Kubernetes 基础

按原始跟踪范围已完成：

- Deployment；
- Service；
- ConfigMap；
- Secret contract；
- Migration Job；
- Ingress contract；
- startup/liveness/readiness Probe；
- requests/limits；
- RollingUpdate；
- rollout undo；
- PDB；
- local/test overlays；
- kind 实际部署和完整 Saga。

仍属于后续增强而不是 Phase 6 基础验收的内容：

- Ingress Controller 安装与真实流量验收；
- TLS；
- HPA；
- NetworkPolicy；
- 多节点和节点故障验证；
- Registry 不可变镜像；
- 托管云存储、负载均衡和 Workload Identity。

### Phase 7：完整可观测性

尚未完成：

- Prometheus；
- Grafana；
- OpenTelemetry；
- W3C Trace Context；
- `trace_id` / `span_id` 日志字段；
- Saga、Outbox、RabbitMQ 和 Reconciliation 指标；
- 基础告警。

### Phase 8：持续交付与运行保障

尚未完成：

- GHCR 镜像；
- 测试环境自动部署；
- 部署后 Smoke Test；
- 环境级自动回滚；
- MySQL 备份恢复；
- 故障演练；
- Runbook；
- 压测与容量报告；
- 最小权限账号和 Workload Identity。

## 完成度估算

以下为工程阶段估算，不是严格测量值：

| 目标 | 估算完成度 |
| --- | ---: |
| 微服务核心改造 | 约 94% |
| 容器化应用开发 | 约 93% |
| 应用可靠性基础 | 约 94% |
| 云原生应用基础 | 约 90%–93% |
| Kubernetes 基础部署与合同 | 约 85%–90% |
| 完整可观测性 | 约 25% |
| 生产级云原生交付 | 约 65%–72% |

## 简历表述

推荐：

> 将 Go 订单库存单体系统演进为容器化微服务架构，完成服务与数据库边界拆分、库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、Timeout/Reconciliation Worker 多副本租约、请求预算、有限重试、操作级熔断、Gateway 限流、自动对账、独立迁移，以及 Compose/kind 双环境端到端 CI；在 Kubernetes 中验证 Migration Job、探针、RollingUpdate、失败 revision 检测、`rollout undo`、Ingress/PDB 交付合同和完整订单 Saga。
