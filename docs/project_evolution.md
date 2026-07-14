# 项目演进记录

本文记录项目从业务型 Go 单体到云原生工程实验系统的真实演进过程。

## 阶段总览

```text
业务单体
  ↓
工程化单体
  ↓
多服务运行单元
  ↓
四库数据所有权
  ↓
Inventory Reservation + Order Saga
  ↓
独立 Goose Migration
  ↓
Transactional Outbox + 多 Worker 租约
  ↓
应用可靠性收口
  ↓
Kubernetes 基础与回滚
  ↓
Prometheus + Grafana + OpenTelemetry
  ↓
不可变镜像与自动测试环境 CD
  ↓
备份恢复 + 故障演练 + Runbook + 有界压测
  ↓
【既定云原生实验路线完成】
```

## Phase 0：业务单体

完成用户、商品、库存、订单、JWT、最小 RBAC、MySQL 本地事务、行锁、Redis cache-aside、RabbitMQ TTL/DLX、Transactional Outbox、Docker、Compose、Goose 和 GitHub Actions。

这一阶段的一致性主要依赖单个 MySQL 本地事务。

## Phase 1：迁移基线与架构审计

- 保留原仓库历史并建立实验仓库；
- 修复 CI、数据库命名和迁移漂移；
- 固化依赖、数据所有权、事务边界和风险；
- 建立可重复 Compose 与 CI 验证入口。

## Phase 2：Microservices v1——运行单元拆分

拆分出 API Gateway、Identity、Catalog、Inventory、Order 和 Timeout Worker。每个运行单元拥有独立二进制与镜像，但初期仍共享 MySQL。

## Phase 3：Microservices v2——四库与 Saga

```text
Identity  -> go_order_identity
Catalog   -> go_order_catalog
Inventory -> go_order_inventory
Ordering  -> go_order_ordering
```

跨域协作改为内部 HTTP API，单库 ACID 改为：

- 本地事务；
- 库存预占状态机；
- Order Saga；
- 幂等远程动作；
- 补偿；
- `reconciliation_required` 明确不确定状态。

## Phase 4：独立迁移与多 Worker

- 四套服务独立 Goose migration；
- 业务进程移除 `AutoMigrate`；
- Compose/Kubernetes 一次性 Migration Job；
- Outbox 增加 `lease_owner`、`lease_until`、`next_attempt_at`；
- `FOR UPDATE SKIP LOCKED` 支持多副本领取；
- Worker 崩溃后租约可过期回收。

## Phase 5：应用可靠性收口——完成

### Publisher Confirms

Broker ACK 后才把 Outbox 标记为 `published`；NACK、确认超时、Channel 关闭和连接错误进入可重试失败。系统保留 at-least-once，不宣称 exactly-once。

### HTTP 请求预算

实现：

```text
X-Request-ID
X-Request-Deadline
Connect/TLS/Header/Total Timeout
Bounded Retry
Exponential Backoff
Remaining Budget Check
```

### 熔断与限流

- 按 `<upstream>/<operation>` 隔离熔断状态；
- 基础设施错误计数，业务 4xx 不开路；
- Gateway 客户端/全局 Token Bucket；
- HTTP 429 与 `Retry-After`。

### 自动对账

新增 Reconciliation Worker、结构化任务、租约、多副本、幂等修复、有界重试和非变更式 dry-run。

## Phase 6：Kubernetes 基础——完成

### Kustomize 与工作负载

建立 base/local/test overlays，包含：

- Namespace、ConfigMap、Secret 合同；
- MySQL/RabbitMQ StatefulSet；
- 四个 Migration Job；
- 七个 Deployment 与 Service；
- Startup/Readiness/Liveness Probe；
- resources、RollingUpdate、Ingress 和 PDB。

### 实机验收

CI 在干净 Runner 创建 kind 集群：

```text
部署完整拓扑
  ↓
验证双 Worker 与暴露边界
  ↓
注入不可用 Gateway revision
  ↓
检测 rollout failure
  ↓
kubectl rollout undo
  ↓
Gateway 恢复
  ↓
完整 Kubernetes Saga
```

## Phase 7：应用级可观测性——完成

### Prometheus

五个 HTTP 服务和两个 Worker 提供 `/metrics`，覆盖 HTTP server/client、Order、Saga、Outbox、Reconciliation、Worker、RabbitMQ 和 Migration 信号。

### Grafana 与规则

- 自动 Provisioning Prometheus/Tempo 数据源；
- 应用、Saga、Outbox、Reconciliation 与 Worker 总览 Dashboard；
- recording rules；
- alert rules 与 promtool firing/non-firing 测试。

### OpenTelemetry

- W3C `traceparent` / `tracestate`；
- HTTP server/client 与业务 span；
- OTLP Collector 与 Tempo；
- `trace_id` / `span_id` 日志关联；
- 固定 Trace ID 的五服务跨服务 Trace 自动验收。

## Phase 8：持续交付与运行保障——完成既定实验范围

### 8.1 不可变 GHCR 发布

七个服务镜像使用完整 Commit SHA 标签，记录 OCI Digest 并生成发布清单。工作流拒绝覆盖已存在的不可变标签，并验证所有 Digest Reference 可访问。

验收：Issue #43。

### 8.2 自动测试环境 CD 与回滚

从已验收 Artifact 下载发布清单，把精确 Digest 渲染到 disposable kind：

```text
部署精确 Digest
  ↓
Readiness + Saga Smoke
  ↓
注入坏镜像
  ↓
证明 rollout 失败
  ↓
恢复 Last-Known-Good Digest 集合
  ↓
再次执行 Saga
```

验收：Issue #48。

### 8.3 四库备份恢复

- 合成业务数据；
- 停止写入建立静止窗口；
- 四库独立 `mysqldump`；
- Repository/Commit/UTC/MySQL Version/Size/SHA-256 清单；
- 独立 MySQL 8.4 容器恢复；
- 迁移状态和代表性业务数据验证；
- 恢复库逻辑 Schema 与主键有序数据指纹和源库恢复前指纹精确一致；
- 源库恢复后逻辑指纹与恢复前保持一致；
- 损坏 Dump 必须被拒绝。

验收：Issue #50。

### 8.4 四类故障演练

主分支运行 `29323288284` 通过：

1. RabbitMQ 停止与恢复，Worker Session 1→0→1；
2. 真实慢 HTTP 上游触发超时、开路、无网络拒绝、半开恢复；
3. Worker 持有租约后被终止，第二实例过期回收且仅完成一次；
4. 非法 Migration 在独立 MySQL 中失败，发布保持阻断；
5. 全部演练后完整 Order Saga 通过。

验收：Issue #51。

### 8.5 Runbook 与有界压测

Operator Runbook 固化：

- 证据优先原则；
- 常见事件检测与诊断；
- RabbitMQ、HTTP、Worker、Migration、备份与发布恢复步骤；
- 事故复盘模板；
- 实验能力与生产能力边界。

受保护主分支压测运行 `29321080192`：

```text
并发阶段：1 / 4 / 8 / 16 / 32
每阶段请求上限：3000
总请求硬上限：15000
健康阶段最佳成功吞吐：177.989 requests/second
健康阶段最高 P95：31.812 ms
健康阶段错误数：0
首个观测边界：concurrency 8 的吞吐平台与尾延迟增长
```

验收：Issue #52。

## 当前项目状态

既定路线已经闭环：

```text
代码
  → 质量门禁
  → 不可变镜像
  → Digest 发布清单
  → 一次性测试环境
  → Smoke Test
  → 坏版本检测与回滚
  → 数据备份恢复
  → 故障演练
  → Runbook
  → 有界容量证据
```

当前系统仍不是生产级平台。未覆盖内容包括正式身份与最小权限、跨区容灾、PITR、生产 SLO、告警升级、长期监控/Trace/Backup 存储和真实生产流量容量规划。

这些内容应进入新的生产化路线，不再作为 Phase 8 遗留项。
