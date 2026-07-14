# Phase 8 持续交付与运行保障收口

## 目的

本文汇总 Phase 8.1–8.5 的最终实现、真实运行证据和能力边界。只有存在可重复 GitHub Actions 运行或明确 Artifact 的能力才标记为完成。

项目内容与运行证据仅保存在 GitHub、GitHub Actions、GitHub Issues 和 GHCR，没有部署到 GitHub 之外的公开网站、长期集群或第三方 SaaS 平台。

## 验收总表

| 阶段 | 状态 | 跟踪 | 验收摘要 |
| --- | --- | --- | --- |
| 8.1 不可变镜像 | 完成 | #43 | 七个 GHCR 镜像、完整 SHA 标签、OCI Digest、发布清单、覆盖拒绝 |
| 8.2 测试环境自动 CD | 完成 | #48 | 精确 Digest、一次性 kind、双 Smoke、坏版本检测、完整 Digest 回滚 |
| 8.3 备份恢复 | 完成 | #50 | 四库 Dump、SHA-256、隔离恢复、业务/迁移验证、源库不变、损坏拒绝 |
| 8.4 故障演练 | 完成 | #51 | RabbitMQ、HTTP、Worker 租约、Migration 四类演练及最终 Saga |
| 8.5 Runbook 与压测 | 完成 | #52 | Operator Runbook、P50/P95/P99、成功吞吐、资源证据、容量边界 |

## 8.1 不可变 GHCR 镜像

发布工作流：

- 固定七个服务；
- 使用完整 Commit SHA 标签；
- 生成 SBOM/Provenance；
- 获取 Registry 返回的 OCI Digest；
- 聚合 `release-manifest.json`；
- 验证标签指向的真实 Digest 与清单一致；
- 验证所有 Digest-qualified Reference 可 inspect；
- 已存在的 SHA 标签拒绝覆盖。

首次已接受运行：

```text
commit: 5f25a7497983420ea065bb6d874cc3774e5fd52e
run:    29200087041
```

## 8.2 精确 Digest 的自动测试环境

触发条件：受信任的镜像发布成功后，或主分支手动执行。

流程：

```text
下载 release-manifest.json
  ↓
验证 Repository / Commit / 七镜像集合
  ↓
渲染七个应用和四个 Migration Job 的精确 Digest
  ↓
创建 disposable kind
  ↓
Readiness + 完整 Order Saga
  ↓
注入不存在的坏镜像
  ↓
证明 rollout 失败
  ↓
恢复完整 Last-Known-Good Digest 集合
  ↓
再次执行完整 Order Saga
  ↓
删除集群
```

已接受运行：

```text
commit:             c6686ea57c6113ea657d0a2aeb0d171c6e8a4421
release source run: 29231695000
delivery run:       29231831538
artifact:           test-release-c6686ea57c6113ea657d0a2aeb0d171c6e8a4421-29231831538
```

不使用长期云凭据，不部署到生产或外部平台。

## 8.3 四库备份与隔离恢复

已验证数据库：

```text
go_order_identity
go_order_catalog
go_order_inventory
go_order_ordering
```

Manifest 记录：

- Schema Version；
- Repository；
- Source Commit；
- UTC 创建时间；
- MySQL Client Version；
- Backup/Restore Duration；
- 每库文件名、大小和 SHA-256；
- 总字节数。

恢复环境是独立 MySQL 8.4 容器，不复用源卷，不暴露公网端口，退出时连同匿名数据卷删除。

恢复正确性通过源库与恢复库的逻辑指纹一致性验证：比较规范化后的 Schema 和按稳定顺序计算的业务数据指纹。Dump 文件 SHA-256 用于文件完整性与损坏检测，不把 SQL 文本的逐字节稳定序列化视为恢复正确性的证明。

已接受运行：

```text
commit: 92214bddd8492f9d92634ff70e0fe6231127d1e6
run:    29240788890
```

该实验不等同于生产 PITR、跨区备份或正式 RPO/RTO。

## 8.4 四类故障演练

已接受运行：

```text
commit: 756aa56121e13a091db4b9195bc596fc14c39de4
run:    29323288284
artifact: runtime-fault-drills-756aa56121e13a091db4b9195bc596fc14c39de4-29323288284
```

### RabbitMQ

- 停止 RabbitMQ；
- 观察应用 Session Gauge 变为 0；
- 恢复 RabbitMQ；
- Worker 无进程重启完成自动重连；
- Session 恢复为 1；
- 超时驱动订单 Saga 再次通过。

### HTTP 超时与熔断

- 使用真实本地慢响应服务器；
- 触发 Response Header Timeout；
- 达到阈值后熔断开路；
- 开路请求不发生额外网络 I/O；
- 上游恢复后半开探测成功；
- 熔断回到 Closed。

### Worker 租约

- 子进程调用生产 `claimPending` 持有 Outbox Lease；
- 强制终止子进程；
- 等待租约过期；
- 第二 Worker 回收同一事件；
- 事件只完成一次；
- 无重复行和残留租约。

### Migration 失败

- 独立 MySQL 8.4 容器与数据库；
- 执行故意非法 SQL；
- Migration 必须失败；
- Promotion 条件保持阻断；
- 非法表不存在；
- 正常迁移目录继续通过 validate。

## 8.5 Runbook 与有界压测

已接受运行：

```text
commit: 1d9b9bda1e808a45260efa422cdc4252c84145e7
run:    29321080192
artifact: bounded-load-test-1d9b9bda1e808a45260efa422cdc4252c84145e7-29321080192
```

### 测量边界

```text
并发：1 / 4 / 8 / 16 / 32
每阶段请求上限：3000
总测量请求上限：15000
请求：POST /api/v1/orders
数据：合成用户、商品、库存和唯一 Idempotency Key
环境：单个 GitHub-hosted Runner
```

### 已接受结果

```text
健康持续阶段最佳成功吞吐：177.989 requests/second
健康持续阶段最高 P95：31.812 ms
健康持续阶段数：2
健康阶段错误数：0
首个观测边界：concurrency 8
分类：throughput_plateau_with_tail_growth
```

边界阶段及后续阶段不参与“健康容量”吞吐、P95 和错误摘要；原始全阶段请求、错误与资源证据仍保存在 Artifact，用于诊断。

### Runbook

`docs/runbooks/operations.md` 包含：

- 架构快速参考；
- 证据优先处理规则；
- 发布、备份、RabbitMQ、HTTP、Worker、Migration 和高延迟事件处理；
- 检测、诊断、缓解、恢复与验证；
- 事故复盘模板；
- 明确的生产增强边界。

## 最终交付链路

```text
代码
  → lint/test/race/vet/build
  → Compose/Kubernetes Saga
  → 七个不可变 GHCR 镜像
  → Digest 发布清单
  → 一次性 kind 自动 CD
  → 部署后 Smoke
  → 坏版本检测与 Digest 回滚
  → 四库备份恢复
  → 四类故障演练
  → Operator Runbook
  → 有界负载与容量边界证据
```

## 不在本阶段宣称的能力

- 生产 SLO、错误预算和告警值班；
- 多节点、跨可用区或跨区域容灾；
- 托管数据库、消息队列和负载均衡器故障切换；
- MySQL PITR、增量备份和正式 RPO/RTO；
- mTLS、Workload Identity、数据库运行账号最小权限；
- 长期 HA Prometheus、Tempo 和 Backup Storage；
- 真实生产流量下的容量保证。

Phase 8 的实验工程目标已经完成。上述生产化能力应作为新的、独立范围管理。
