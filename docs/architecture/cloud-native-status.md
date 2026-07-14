# 云原生完成度与生产边界

## 结论

项目既定的 Phase 5–8 工程路线已经完成并通过可重复运行验收：

- Phase 5：应用可靠性收口；
- Phase 6：Kubernetes 基础、失败 rollout 与回滚；
- Phase 7：Prometheus、Grafana、OpenTelemetry 与 Tempo；
- Phase 8：不可变镜像、测试环境自动 CD、备份恢复、故障演练、Runbook 与有界压测。

最准确的定位是：

> 具备明确服务与数据所有权、Order Saga、Transactional Outbox、Publisher Confirms、多 Worker 租约、HTTP 韧性、自动对账、Kubernetes 交付/回滚、应用级可观测性和 GitHub-native 运行保障闭环的 Go 云原生工程实验项目。

仍不应描述为：

> 已完成生产级云平台或能够直接承担真实生产流量的系统。

## 能力矩阵

| 能力域 | 状态 | 当前实现 |
| --- | --- | --- |
| 服务拆分 | 完成 | Gateway、四个业务服务、两个独立 Worker |
| 数据所有权 | 完成 | Identity、Catalog、Inventory、Ordering 四库 |
| 分布式一致性 | 完成基础版 | Inventory Reservation、Order Saga、补偿、自动对账 |
| 异步可靠性 | 完成基础版 | Transactional Outbox、TTL/DLX、Publisher Confirms、手动 ACK |
| Worker 弹性 | 完成基础版 | 多副本、租约、`SKIP LOCKED`、崩溃回收 |
| HTTP 韧性 | 完成基础版 | Deadline、Transport 超时、有限重试、熔断、限流 |
| 数据库迁移 | 完成 | 四套 Goose migration、Compose/Kubernetes Job |
| Compose CI | 完成 | 四库、RabbitMQ、双 Worker、完整 Saga |
| Kubernetes | 完成基础版 | Kustomize、StatefulSet、Deployment、Probe、resources、Ingress、PDB |
| Kubernetes 运行验收 | 完成 | disposable kind、坏 revision、`rollout undo`、恢复后 Saga |
| Prometheus/Grafana | 完成基础版 | 七类 scrape job、recording/alert rules、自动 Dashboard |
| OpenTelemetry | 完成本地版 | W3C HTTP Context、OTLP Collector、Tempo、日志关联 |
| GHCR 发布 | 完成 | 七镜像、完整 Commit SHA 标签、OCI Digest、发布清单、覆盖拒绝 |
| 自动测试环境 CD | 完成 | 精确 Digest、一次性 kind、双 Smoke、坏版本、Last-Known-Good 回滚 |
| 备份恢复 | 完成实验版 | 四库逻辑备份、SHA-256、隔离恢复、数据与迁移验证、源库不变 |
| 故障演练 | 完成实验版 | RabbitMQ、HTTP、Worker 租约、Migration 四类运行演练 |
| Runbook | 完成实验版 | 检测、诊断、缓解、恢复和事故复盘模板 |
| 有界压测 | 完成实验版 | 并发阶段、成功吞吐、P50/P95/P99、资源与容量边界 |
| 消息 Trace 传播 | 未完成 | RabbitMQ 消息尚未携带 W3C Trace Context |
| 告警通知与正式 SLO | 未完成 | 无生产 Alertmanager 路由、SLO 与错误预算 |
| 生产身份与最小权限 | 未完成 | 静态内部 Token、实验数据库账号，无 mTLS/Workload Identity |
| 生产级容灾 | 未完成 | 无跨区、多节点、PITR、托管存储和正式 RPO/RTO 承诺 |

## Phase 8 运行验收

### 8.1 不可变 GHCR 镜像

- 七个服务固定 Commit SHA 标签；
- Registry 返回的 OCI Digest 与发布清单精确一致；
- Digest-qualified Reference 可访问；
- 同一不可变标签禁止覆盖；
- 证据由 Issue #43 和 GitHub Actions Artifact 保存。

### 8.2 测试环境自动 CD 与回滚

- 下载已验收发布清单；
- 七个应用和 Migration Job 使用精确 Digest；
- 部署到一次性 GitHub Actions kind 集群；
- 初始 Smoke Test 通过；
- 注入不存在的坏版本并证明 rollout 失败；
- 恢复完整 Last-Known-Good Digest 集合；
- 回滚后再次执行完整 Saga；
- 证据由 Issue #48 保存。

### 8.3 四库备份与隔离恢复

- 四个服务数据库独立逻辑备份；
- 记录 Repository、Commit、UTC 时间、MySQL 版本、大小和 SHA-256；
- 拒绝缺失、额外、损坏或绑定错误的 Dump；
- 恢复到独立 MySQL 8.4 容器；
- 验证迁移状态和代表性业务数据；
- 恢复后导出与源备份逐字节一致；
- 恢复过程不修改源数据库；
- 证据由 Issue #50 保存。

### 8.4 可重复故障演练

受保护主分支运行 `29323288284` 已通过：

- RabbitMQ Session 1→0→1 与 Worker 自动重连；
- 慢 HTTP 上游触发有界超时、开路拒绝、半开恢复和闭合成功；
- Worker 子进程持有 Outbox 租约后被终止，第二实例过期回收且仅完成一次；
- 非法 SQL 在独立 MySQL 中失败，发布继续条件保持阻断；
- 全部演练后完整 Order Saga 通过；
- 证据 Artifact：`runtime-fault-drills-756aa56121e13a091db4b9195bc596fc14c39de4-29323288284`。

### 8.5 Runbook 与有界压测

受保护主分支运行 `29321080192` 已通过：

```text
健康持续阶段最佳成功吞吐：177.989 requests/second
健康持续阶段最高 P95：31.812 ms
健康持续阶段数：2
健康阶段错误数：0
首个边界：concurrency 8 的 throughput_plateau_with_tail_growth
```

测量限制：

- 单个 GitHub-hosted Runner；
- 合成订单创建流量；
- 并发 1/4/8/16/32；
- 每阶段最多 3000 请求；
- 总测量请求硬上限 15000；
- 边界阶段及后续阶段不进入健康容量摘要；
- 原始全阶段请求、错误与资源数据仍保留在 Artifact；
- 该结果不是生产 SLO 或容量保证。

## 当前生产级缺口

### 安全与身份

- 内部服务认证仍是静态 Token；
- Migration/Runtime 数据库账号尚未完全最小权限拆分；
- 无 mTLS、Workload Identity、Secret Rotation 和正式审计体系。

### 平台与网络

- 无长期运行的多节点或跨可用区集群；
- 无正式 TLS、NetworkPolicy、HPA、Pod Security 和生产 Ingress 流量验收；
- 无托管数据库、消息队列或负载均衡器故障切换验证。

### 数据保护

- 当前是合成数据逻辑备份实验；
- 无加密外部备份仓库、PITR、增量备份、保留策略和定期恢复值班；
- RPO/RTO 只记录测量方法，不做生产承诺。

### 可观测性与运营

- 无正式 Alertmanager receiver、Pager/升级链路；
- 无生产 SLO、错误预算与容量预测；
- 缺少 MySQL、RabbitMQ、Node 等完整基础设施 Exporter；
- Trace backend、Prometheus 与备份没有生产级 HA 和长期存储。

## 阶段状态

```text
Phase 5 应用可靠性        完成
Phase 6 Kubernetes 基础   完成
Phase 7 应用级可观测性    完成
Phase 8 交付与运行保障    完成既定实验范围
```

后续工作应作为**新的生产化路线**单独立项，而不是继续扩张已完成的 Phase 8。

## 推荐简历表述

> 将 Go 订单库存系统演进为七运行单元、四服务数据库的容器化微服务架构，完成库存预占 Saga、Transactional Outbox、RabbitMQ Publisher Confirms、多 Worker 租约、请求预算/重试/熔断/限流和自动对账；建立 Compose/kind 双环境 CI、Prometheus/Grafana/OpenTelemetry 可观测性、七镜像 GHCR Digest 发布、一次性测试环境自动 CD 与回滚、四库备份恢复、四类故障演练和有界压测闭环。明确区分实验验收与生产级安全、容灾、SLO 和长期运行能力。
