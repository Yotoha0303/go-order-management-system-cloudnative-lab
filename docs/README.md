# 项目文档导航

本文档区分**当前实现**、**运行证据**、**历史基线**和**生产增强项**，避免把代码存在、配置可渲染和真实运行验收混为一谈。

## 当前实现

| 文档 | 内容 |
| --- | --- |
| [../README.md](../README.md) | 当前项目定位、能力矩阵、运行入口和生产边界 |
| [architecture/microservices-v2-data-ownership.md](architecture/microservices-v2-data-ownership.md) | 四库数据所有权、Inventory Reservation 和 Order Saga |
| [architecture/migrations-outbox-leasing.md](architecture/migrations-outbox-leasing.md) | Goose migration、Outbox 租约、多 Worker 与 Publisher Confirms |
| [architecture/http-timeout-retry.md](architecture/http-timeout-retry.md) | Request Deadline、Transport 超时和有限重试 |
| [architecture/circuit-breaker-rate-limit.md](architecture/circuit-breaker-rate-limit.md) | 操作级熔断与 Gateway Token Bucket |
| [architecture/reconciliation-worker.md](architecture/reconciliation-worker.md) | 自动对账任务、租约、修复动作与 dry-run |
| [architecture/kubernetes-foundation.md](architecture/kubernetes-foundation.md) | Kustomize、Migration Job、Deployment、Ingress、PDB、kind 与回滚 |
| [architecture/prometheus-metrics.md](architecture/prometheus-metrics.md) | HTTP、Saga、Outbox、Worker 与 RabbitMQ 指标 |
| [architecture/grafana-alerts.md](architecture/grafana-alerts.md) | Dashboard、recording rules 和 alert rules |
| [architecture/opentelemetry-tracing.md](architecture/opentelemetry-tracing.md) | W3C Context、OTLP、Tempo 和日志关联 |
| [architecture/cloud-native-status.md](architecture/cloud-native-status.md) | Phase 5–8 完成状态与生产级缺口 |
| [project_evolution.md](project_evolution.md) | 从单体到运行保障闭环的演进记录 |

## Phase 8 验收与运营资料

| 文档 | 内容 |
| --- | --- |
| [verification/phase-08-closure.md](verification/phase-08-closure.md) | Phase 8.1–8.5 最终验收总表、运行编号和边界 |
| [verification/ghcr-release-images.md](verification/ghcr-release-images.md) | 不可变 GHCR 镜像、OCI Digest 和发布清单 |
| [verification/test-environment-deployment.md](verification/test-environment-deployment.md) | 精确 Digest 测试环境部署、坏版本和回滚 |
| [verification/mysql-backup-restore.md](verification/mysql-backup-restore.md) | 四库逻辑备份、SHA-256 与隔离恢复 |
| [verification/runtime-fault-drills.md](verification/runtime-fault-drills.md) | RabbitMQ、HTTP、Worker 和 Migration 故障演练 |
| [runbooks/operations.md](runbooks/operations.md) | Operator Runbook：检测、诊断、缓解、恢复和复盘 |
| [verification/load-test.md](verification/load-test.md) | 有界压测方法、数据边界和容量解释规则 |

## 主要验证脚本

```text
scripts/smoke/microservices-saga.sh
scripts/smoke/microservices-saga-kubernetes.sh
scripts/smoke/prometheus-metrics.py
scripts/smoke/grafana-provisioning.py
scripts/smoke/tempo-trace.py
scripts/release/manifest.py
scripts/deployment/deployment.py
scripts/backup/manifest.py
scripts/backup/run-backup-restore.sh
scripts/fault-drills/migration-failure.sh
scripts/load/order_create_load.py
scripts/load/resource_sampler.py
scripts/load/analyze_load.py
```

## 主要 GitHub Actions 工作流

```text
ci.yml
kubernetes-contracts.yml
observability.yml
release-contracts.yml
publish-images.yml
test-environment-deployment.yml
backup-contracts.yml
mysql-backup-restore.yml
fault-drill-contracts.yml
fault-drills.yml
operations-contracts.yml
load-test.yml
```

执行边界：

- PR 合同工作流只读、非破坏；
- GHCR 发布、真实自动 CD、备份恢复、故障演练和压测只在受信任的 `main` 或手动入口执行；
- 所有一次性环境都在 `if: always()` 清理；
- 项目与证据仅保存在 GitHub、GitHub Actions、GitHub Issues 和 GHCR。

## 已接受的 Phase 8 证据

| Issue | 验收 |
| --- | --- |
| #43 | 七个不可变 GHCR 镜像和发布清单 |
| #48 | 一次性 kind 自动 CD、Smoke、坏版本和回滚 |
| #50 | 四库备份、隔离恢复与损坏输入拒绝 |
| #51 | 四类真实故障演练和最终 Saga |
| #52 | Operator Runbook 与有界压测 |

## 当前验证边界

### 已完成

- lint、unit/integration、race、vet 和 build；
- 四套服务迁移与历史迁移校验；
- 七服务镜像、Compose 四库和完整 Saga；
- kind 部署、坏 revision、undo 与恢复后 Saga；
- Prometheus/Grafana/Tempo 运行验收；
- 七个 GHCR Digest 镜像和发布清单；
- 精确 Digest 的测试环境自动部署和回滚；
- 四库逻辑备份与隔离恢复；
- RabbitMQ、HTTP、Worker 租约与 Migration 故障演练；
- 有界负载、P50/P95/P99、成功吞吐和资源证据。

### 生产增强项

- RabbitMQ 消息 W3C Trace Context；
- Alertmanager 通知、正式 SLO/错误预算；
- 基础设施 Exporter 与 Kubernetes 内长期监控栈；
- mTLS、Workload Identity 和数据库最小权限账号；
- 多节点/跨区、PITR、托管存储、正式 RPO/RTO；
- TLS、HPA、NetworkPolicy 和真实生产流量规划。

这些内容不再属于已完成的 Phase 8，应由新的生产化路线单独管理。

## 历史基线

以下文档保留为演进证据，不代表当前运行形态：

- [architecture/current-state.md](architecture/current-state.md)
- [architecture/dependency-map.md](architecture/dependency-map.md)
- [architecture/data-ownership.md](architecture/data-ownership.md)
- [architecture/runtime-flow.md](architecture/runtime-flow.md)
- [architecture/transaction-boundaries.md](architecture/transaction-boundaries.md)
- [architecture/risk-register.md](architecture/risk-register.md)
- [architecture/microservices-v1.md](architecture/microservices-v1.md)

## 文档维护规则

1. 根 README 只描述 `main` 的真实状态。
2. “代码存在”“可渲染”“已启动”“通过业务验收”必须分开表述。
3. 只有存在自动或可重复证据的能力才标记为完成。
4. 生产增强不得倒灌为 Phase 8 未完成项。
5. 新阶段必须使用独立 Issue、PR 和验收边界。
