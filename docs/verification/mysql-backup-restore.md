# MySQL 四库备份与隔离恢复

## 目标

Phase 8.3 对以下四个服务数据库执行可重复的逻辑备份与隔离恢复：

```text
go_order_identity
go_order_catalog
go_order_inventory
go_order_ordering
```

本阶段验证备份完整性、恢复可用性、代表性业务数据和迁移状态，并证明恢复操作不会修改源数据库。所有运行都发生在 GitHub Actions 的一次性 Docker 环境中，证据只保存在 GitHub Actions 与 GitHub Issue。

## 数据边界

工作流先运行完整 Order Saga，生成管理员、买家、商品、库存、支付订单、取消订单和超时订单等合成数据。Artifact 仅允许合成测试数据；不得将生产数据、真实用户数据或任何凭据放入 SQL Dump、Manifest、日志或验证报告。

密码通过容器进程环境变量 `MYSQL_PWD` 传入 MySQL 客户端，不作为命令行参数，也不写入 Dump 或 Manifest。

## 一致性与静止窗口

创建合成数据后，工作流停止所有应用服务、两类 Worker 与 RabbitMQ，只保留源 MySQL。这样可以在备份、恢复和源库复核期间建立受控的写入静止窗口。

每个数据库使用 MySQL 8.4 `mysqldump` 独立导出，并启用：

- `--single-transaction`；
- `--quick`；
- `--routines` 与 `--triggers`；
- `--hex-blob`；
- `--order-by-primary`；
- `--skip-dump-date` 与 `--skip-comments`。

这提供当前项目规模下可重复的逻辑快照，但不代表跨四库的全局分布式时间点一致性。四库之间没有由 MySQL 提供的单一全局事务快照；工作流依靠停止写入来缩小并消除测试环境中的跨库变化窗口。

## Backup Manifest

`scripts/backup/manifest.py` 固定要求四个数据库，记录：

- Schema Version；
- Repository；
- Source Commit；
- UTC 创建时间；
- MySQL Client Version；
- Backup Duration；
- Restore Duration；
- 每个 Dump 文件名、字节数和 SHA-256；
- 总字节数。

缺失、空文件、重复数据库、顺序漂移、额外数据库、大小不符或 SHA-256 不符都会失败。工作流还会复制并故意破坏一个 Dump，证明完整性验证能够拒绝损坏输入。

## 隔离恢复

备份完成后，脚本创建一个独立的 MySQL 8.4 容器。该容器：

- 不复用源 MySQL 数据卷；
- 不覆盖源数据库；
- 不对公网开放端口；
- 使用独立临时密码；
- 在脚本退出时删除。

四个 Dump 导入后，工作流验证：

- 每个数据库都存在已应用的 `goose_db_version`；
- Identity 中存在合成用户；
- Catalog 中存在已创建商品；
- Inventory 中存在库存和库存变更记录；
- Ordering 中存在至少三类业务订单及订单项。

随后从恢复容器重新导出四个数据库，并与原始 Dump 逐字节比较。恢复后的规范化 Dump 必须完全一致。

## 源数据库保持不变

在恢复验证完成后，脚本再次从源 MySQL 导出四个数据库。每个 Source-After Dump 必须与备份时的 Source-Before Dump 完全一致。这证明隔离恢复没有执行源库 DROP、IMPORT、UPDATE 或其他修改。

## 证据

GitHub Actions Artifact 包含：

- 四个合成数据 SQL Dump；
- `backup-manifest.json`；
- 验证后的 Dump 列表；
- Source-Before、Source-After 与 Restored SHA-256；
- 恢复数据计数；
- Backup/Restore Duration；
- 损坏 Dump 被拒绝的负测试结果；
- 源 Saga 日志；
- 失败时的 Compose 诊断。

Artifact 保留 30 天。Phase 8.3 成功后，GitHub Actions 会在 Issue #50 写入 Commit、Run 和 Artifact 名称，不包含数据库密码。

## RPO 与 RTO 解释

本工作流测得的 Backup Duration 和 Restore Duration 是一次 GitHub Hosted Runner 上的小型合成数据结果：

- **RPO**：测试运行在写入静止后创建逻辑快照，因此该受控运行的目标数据丢失窗口接近零；这不等于生产环境的持续 RPO 承诺。
- **RTO**：记录的 Restore Duration 只覆盖四个小型 SQL Dump 导入，不包括生产审批、对象存储下载、DNS、流量切换、缓存预热或人工诊断。

## 限制

该验收不等同于生产级灾难恢复。它未覆盖：

- MySQL 物理热备；
- Binlog 与 Point-in-Time Recovery；
- 加密备份和密钥轮换；
- 异地对象存储与长期保留；
- 大数据量恢复性能；
- 跨区域故障；
- 生产 RPO/RTO 合规；
- 数据库 Schema Down Migration。

## 回滚边界

删除 Backup Workflow、Manifest Tool 和恢复脚本只会移除 GitHub 内的备份验证能力，不会改变应用 API、数据库 Schema、Compose 正常运行、Kubernetes 部署、GHCR 镜像或业务 Saga。
