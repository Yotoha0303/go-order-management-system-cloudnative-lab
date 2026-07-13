# Digest-pinned disposable test delivery

## 目标

Phase 8.2 把 Phase 8.1 已验证的 `release-manifest.json` 作为唯一发布来源，在 **GitHub Actions 内的一次性 kind 集群**中完成部署、Smoke Test、错误版本验证和回滚证明。

本阶段不会发布到 GitHub 之外的网站、SaaS 托管平台、长期公开集群或生产环境。项目代码、镜像、清单、运行记录和验收证据只保存在 GitHub 仓库、GitHub Actions、GitHub Issues 与 GHCR。

## 触发边界

`.github/workflows/deploy-test-release.yml` 只接受两类受信任入口：

1. `Publish Immutable Images` 在 `main` 上成功完成后的 `workflow_run`；
2. 由具备仓库写权限的操作者从 `main` 显式执行 `workflow_dispatch`，并提供源 Run ID 与完整 Commit SHA。

工作流没有 `pull_request` 触发器。PR 只运行只读的 `Delivery Contracts`，不会获得 `actions: read`、`packages: read`、`issues: write`，不会下载私有发布产物，也不会创建集群。

## 发布来源

工作流通过 GitHub Actions Run ID 下载：

```text
release-manifest-<40-character-commit-sha>
```

下载后执行两层校验：

- `manifest.py verify` 检查仓库、Commit、固定七服务、不可变标签和合法 `sha256`；
- `deployment.py render` 要求 Kustomize 输出中每一个应用镜像恰好出现预期次数，并全部替换为清单中的精确 OCI digest。

任何 `latest`、分支标签、Commit 标签或本地 `:local` 镜像都不能成为测试环境部署源。

## 一次性环境

运行流程：

```text
verified release artifact
  -> validate exact seven-image manifest
  -> render local Kustomize topology with digest references
  -> create runner-local kind cluster
  -> create temporary GHCR pull secret
  -> apply infrastructure, migrations and applications
  -> wait for MySQL, RabbitMQ, four migration Jobs and seven Deployments
  -> verify deployed image inventory
  -> run complete Order Saga
```

Gateway 仅通过 runner 本地端口 `127.0.0.1:8082` 访问。集群不接入公网 Ingress、外部负载均衡器或云提供商。GHCR 凭据来自当前 Run 的短期 `GITHUB_TOKEN`，只写入临时集群 Secret，不保存进 Artifact。

## 部署后验收

首次部署必须同时满足：

- 七个 Deployment 使用发布清单中的精确 OCI digest；
- 四个 Migration Job 使用所属服务的同一精确 Digest；
- MySQL、RabbitMQ、迁移和应用全部 Ready/Complete；
- Gateway `/readyz` 成功；
- 完整 Order Saga 成功，包括注册、权限、商品、库存、订单、幂等、支付、取消、超时与补偿。

`deployment.py verify-inventory` 会读取 Kubernetes API 返回的 Deployment/Job JSON，不接受本地标签、可变 GHCR 标签或缺失工作负载。

## 错误版本与回滚

工作流把 API Gateway 临时设置为一个不存在但格式合法的 Digest，并要求 rollout 在限定时间内失败。若错误版本意外成功，工作流立即失败。

随后工作流不是只执行单个 `rollout undo`，而是根据已验收发布清单重新设置**全部七个 Deployment**，恢复完整 Last-Known-Good Digest 集合。恢复后必须：

1. 七个 Deployment 全部完成 rollout；
2. Gateway readiness 恢复；
3. 再次验证 Deployment 与 Migration Job 镜像清单；
4. 再次执行完整 Order Saga。

本阶段只证明应用镜像集合回滚，不证明数据库 schema 回滚。Migration Job 已执行后不会因应用镜像回滚而自动执行 down migration。

## 证据

成功或失败都会上传 GitHub Actions Artifact。成功证据包括：

- 原始 `release-manifest.json`；
- Digest 引用列表；
- Digest 化 Kubernetes 渲染结果；
- 回滚前和回滚后的 Deployment/Job JSON；
- 两次已验证镜像 Inventory；
- 两次 Saga Smoke 日志；
- 错误 rollout 和恢复结果；
- 最终资源与事件快照。

失败时额外保存 Pod 描述、日志、事件、资源状态和 kind 节点日志。无论成功或失败，集群都通过 `if: always()` 删除。

成功运行会在 Issue #48 写入 GitHub 内验收证据。该评论只包含 Commit、Actions Run、Artifact 和已验证结论，不包含凭据。

## 权限

- 工作流顶层：`contents: read`；
- 部署任务：`actions: read`、`contents: read`、`packages: read`；
- 证据任务：`contents: read`、`issues: write`；
- 不使用云账号、长期 GHCR PAT、外部部署 Secret 或生产凭据。

## 回滚边界

删除本工作流和部署工具不会修改业务 API、数据库结构、消息合同、Compose 环境或现有 kind CI。它只移除从已发布 Digest 集合创建一次性测试环境的交付验证能力。
