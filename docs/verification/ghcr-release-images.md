# GHCR 不可变镜像与发布清单

## 目标

本阶段只解决一个问题：把仓库中的七个可部署进程构建为与单一 Git commit 绑定的 GHCR 镜像，并产出包含精确 OCI digest 的机器可读发布清单。

本阶段**不执行 Kubernetes 部署**，不修改运行环境，不自动迁移数据库，也不把镜像发布成功等同于测试环境或生产环境发布成功。

## 发布对象

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
order-reconciliation-worker
```

每个镜像继续使用共享的 `deploy/docker/Dockerfile.service`，通过 `SERVICE` build argument 选择 `cmd/<service>` 构建目标。

## 触发边界

`.github/workflows/publish-images.yml` 仅接受：

1. 在 `main` 分支上手动执行 `workflow_dispatch`；
2. 推送匹配 `release-*` 的 Git tag。

该工作流没有 `pull_request` 触发器。普通 PR 只运行 `.github/workflows/release-contracts.yml`，后者只有 `contents: read`，不会获得 `packages: write`，也不会登录或写入 GHCR。

## 权限边界

- 工作流顶层：`contents: read`；
- 预检与清单校验：`packages: read`；
- 只有镜像发布矩阵：`packages: write`；
- Registry 登录使用当前运行的 `GITHUB_TOKEN`，不保存长期 GHCR 密钥；
- 构建参数不接收业务 Secret、数据库密码或部署凭据。

## 不可变标签规则

镜像名称格式：

```text
ghcr.io/<lowercase-owner>/go-order-management-system-cloudnative-lab-<service>
```

标签格式：

```text
sha-<40-character-commit-sha>
```

同一 commit-SHA 标签一旦存在，预检直接失败；发布矩阵在 push 前还会再次检查，拒绝覆盖。工作流不生成 `latest`，部署系统也不应以分支别名或其他可变标签作为事实来源。

这一策略的代价是：Registry 不是事务系统，七个镜像无法原子提交。若运行过程中出现**部分发布**，已经写入的 commit 标签不会被覆盖，当前 commit 的整组发布应判定为失败。恢复方式是：

1. 保留失败运行和已发布 digest 作为诊断证据；
2. 修复构建或 Registry 问题；
3. 产生新的提交并从新的 commit-SHA 重新发布；
4. 不删除或重写已有 commit 标签来伪造完整发布。

## 发布流程

### 1. 全局预检

在任何构建开始前，工作流检查七个 commit-SHA 标签均不存在。任意一个已存在都会阻止本次运行。

### 2. 并行构建和推送

每个服务：

- 使用相同 source commit；
- 使用独立 BuildKit cache scope；
- 写入 OCI source、revision 和 title labels；
- 启用 provenance 与 SBOM；
- 推送 commit-SHA 标签；
- 读取 `docker/build-push-action` 返回的 OCI digest；
- 立即通过 digest-qualified reference 执行 `imagetools inspect`。

### 3. 服务清单片段

每个矩阵任务生成一个 JSON 片段：

```json
{
  "commit_sha": "<40-character-commit-sha>",
  "digest": "sha256:<64-hex>",
  "image": "ghcr.io/<owner>/<package>",
  "reference": "ghcr.io/<owner>/<package>@sha256:<64-hex>",
  "service": "api-gateway",
  "tag": "sha-<40-character-commit-sha>"
}
```

片段必须满足：服务名属于固定七项、镜像路径小写、tag 与 commit 完全一致、digest 为合法 SHA-256，且 `reference` 必须由 image 与 digest 精确组成。

### 4. 聚合发布清单

`scripts/release/manifest.py` 只在收齐七个唯一服务片段后生成：

```text
release-manifest.json
release-references.txt
```

发布清单包含：

- schema version；
- GitHub `owner/repository`；
- 精确 source commit；
- source commit 的提交时间；
- 按固定顺序排列的七个镜像、tag、digest 和 digest-qualified reference。

`created_at` 使用 Git commit 时间，而不是工作流当前时间，避免同一 source commit 因重试产生不同的清单元数据。

### 5. Registry 终检

聚合任务再次读取清单，并逐个对七个 digest-qualified references 执行 `docker buildx imagetools inspect`。只有全部可查询时，才上传 90 天保留的发布清单 artifact。

## 本地和 PR 验证

不需要 Registry 凭据：

```bash
python3 -m compileall -q scripts/release scripts/verify/release-contracts.py
python3 -m unittest discover -s scripts/release -p "test_*.py" -v
python3 scripts/verify/release-contracts.py
```

验证范围：

- 固定七服务集合和顺序；
- 缺失、重复、非法 digest、可变 tag、大小写错误均失败；
- 发布工作流没有 PR 触发器；
- `packages: write` 只存在于镜像发布任务；
- commit-SHA 标签、覆盖拒绝、digest 提取、片段聚合和 Registry 终检合同存在；
- 共享 Dockerfile 仍构建选定服务并以非 root 用户运行。

## 首次实际验收

代码合并到 `main` 后，需要在 GitHub Actions 中从 `main` 手动运行 **Publish Immutable Images**。验收证据应包括：

1. 七个矩阵任务全部成功；
2. GHCR 中存在七个 `sha-<commit>` 标签；
3. 每个任务的 digest inspect 成功；
4. `release-manifest-<commit>` artifact 存在；
5. 清单内七个 digest-qualified references 均可重新 inspect；
6. 对同一 commit 再次运行时，预检因已有标签而失败，不覆盖内容。

在这次真实 GHCR 运行完成前，只能声称“发布工作流和清单合同已经实现并通过 PR 静态/单元验证”，不能声称“镜像已经实际发布”。

## 回滚边界

本阶段没有部署动作，因此不存在应用流量回滚。其回滚仅包括：

- 停止使用某份发布清单；
- 后续部署选择上一份已验证清单中的 digest；
- 禁止通过重写同一 commit 标签实现所谓回滚。

后续测试环境 CD 必须消费 `release-manifest.json` 中的 digest-qualified references，并单独实现部署、烟雾测试和失败回滚。
