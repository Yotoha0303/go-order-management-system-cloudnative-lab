# 安全遗留问题

本文记录**无法自动修复、或需要架构调整与人工决策**的安全问题，供决策后再动手。
可自动修复的问题已直接提交，不在此列。

扫描基线：2026-07-26。使用工具：`govulncheck`、`gosec`、`staticcheck`、`npm audit`。

## 扫描结论快照

| 工具 | 结果 |
|---|---|
| govulncheck | **0 漏洞**（修复前 41 条，其中 19 条代码可达） |
| gosec | **0 issue**（需加 `-exclude-generated`，见下；测试文件中的 16 条为默认口令、子进程调用等噪音） |
| staticcheck | **0 告警**（默认检查集；非默认的 ST1000/ST1003 已被 `.golangci.yml` 排除） |
| npm audit | 9 → **3 条 high**（均为 brace-expansion，见 S-1） |

---

### 关于 gosec 与生成代码

AGENT.md 任务一给出的命令是 `gosec ./...`，它会在 `internal/platform/grpcapi/` 下报 4 条
G103（`unsafe.Slice` / `unsafe.StringData`）。这 4 条全部位于 protoc-gen-go **生成**的
`inventory.pb.go` 中，是 protobuf 运行时读取原始描述符的标准写法，等级 LOW，无法也不应修改。

CI 实际使用的 golangci-lint 因 `.golangci.yml` 的 `exclusions.generated: strict` 已自动跳过
它们（实测 0 issue）。手工运行独立 gosec 时请加 `-exclude-generated`：

```bash
go run github.com/securego/gosec/v2/cmd/gosec@latest -exclude-generated ./...
```

## 待决策

### S-1 · 前端 brace-expansion 三条 high，修复需破坏性降级

**风险等级**：中（仅影响 lint/format 工具链，不进生产 bundle）

`brace-expansion <=5.0.7` 命中 GHSA-mh99-v99m-4gvg（CVSS 7.5，无界展开导致 OOM 崩溃）
与 GHSA-3jxr-9vmj-r5cp（CVSS 5.3，连续空 `{}` 组指数级展开）。

引入链：`@trivago/prettier-plugin-sort-imports` → `minimatch` → `brace-expansion`。

`npm audit fix --force` 会把 `@trivago/prettier-plugin-sort-imports` 从 6.x **降级**到 5.2.2，
npm 明确标记为 breaking change。AGENT.md 禁止自动执行 `--force`，故保留。

**建议方案**（三选一）：

1. 等待 `@trivago/prettier-plugin-sort-imports` 6.x 发布依赖修复版 minimatch 的补丁 —— 推荐，
   风险为零，代价是继续带着这三条告警
2. 加 `overrides` 强制 `brace-expansion` 到 `>=5.0.8`，跳过降级插件本身。需验证 minimatch 与
   插件在新版下行为正常
3. 接受降级到 5.2.2，需要回归验证 `npm run format:check` 与导入排序结果是否变化

### S-3 · 前端 JWT 存放在 JS 可读 cookie

**风险等级**：中（需前后端配合改造）

`fronted/src/lib/cookies.ts` 由前端 JS 通过 `document.cookie` 写入 access token，因此**无法**加
`HttpOnly`，任何 XSS 都能直接读走长期有效令牌。另有两个次要问题：`Secure` 仅在页面本身是
https 时才加；cookie `Max-Age` 为 7 天，而 JWT 有效期 24 小时，令牌过期后 cookie 仍残留。

当前前端源码未发现 XSS 注入点（全量 grep `dangerouslySetInnerHTML` / `v-html` / `eval(` /
`new Function(` / `document.write` 均零命中），所以这是纵深防御问题而非已存在的可利用漏洞。

**建议方案**：由后端下发 `HttpOnly; Secure; SameSite=Strict` 的 cookie 并配套 CSRF 防护，
前端不再接触 token。过渡方案是先把 cookie `Max-Age` 与 token 有效期对齐，并引入 refresh token
缩短 access token 生存期。

### S-4 · 前端缺少 CSP 与安全响应头

**风险等级**：低

`fronted/index.html` 无 `Content-Security-Policy`，`fronted/netlify.toml` 也未配置
`X-Frame-Options`、`X-Content-Type-Options`、`Referrer-Policy`。一旦某个依赖被投毒即无兜底。

**建议方案**：在 netlify.toml 配置响应头。CSP 需按实际内联 style/script 与 API 域名逐条调试，
误配会直接白屏，建议先用 `Content-Security-Policy-Report-Only` 观察一段时间再切正式。

### S-5 · 扫描步骤已加入 CI，但仍是非阻断，且本机 registry 无法 audit

**风险等级**：低（原为中，已部分解决）

已解决部分：`ci.yml` 新增 `govulncheck` 步骤与 `frontend-audit` job（`npm audit --audit-level=high`
加前端构建）。顺带发现前端此前**完全没有 CI** —— `fronted/.github/workflows/` 嵌套在项目目录内，
而 GitHub 只读取仓库根目录的 `.github/workflows/`，那份 workflow 从未运行过。

**仍需决策**：

1. 两个扫描目前都是 `continue-on-error`，只报告不阻断。观察几轮噪音水平后决定是否比照 lint
   增加 Enforce 步骤转为阻断。注意 `npm audit` 在 S-1 解决前必然非空
2. 本机 npm registry 指向 `registry.npmmirror.com`，该镜像未实现 `/-/npm/v1/security/*` 端点，
   本地直接跑 `npm audit` 会 404 退出（本次是显式加 `--registry=https://registry.npmjs.org` 才成功）。
   CI 用默认官方 registry 不受影响。仓库内无 `.npmrc`，该配置来自全局 npm 设置
3. `fronted/.github/` 整个目录现在是死代码（还包含 lint、prettier、测试步骤），需决定是删除，
   还是把其中有价值的步骤合并进根目录 workflow

---

## 已解决

| 编号 | 问题 | 处理方式 |
|---|---|---|
| S-2 | gosec G304：配置文件路径来自变量 | 核实全部 13 个调用点均传编译期字面量，加 `// #nosec G304` 并注明前提；gosec 生产代码归零 |
| S-6 | JWT issuer 仍是旧项目名 | 已随项目重命名统一改为新名，并收敛为 `auth.Issuer` 常量，消除五处字面量漂移的风险。**注意**：该变更使部署前签发的所有 token 失效，需一次性发布并提示用户重新登录 |
| S-7 | K8s 标签仍是旧项目名 | 已改名，同时把废弃的 `commonLabels` 迁移到 `labels`。selector 不再包含 `part-of`，仅保留 `app.kubernetes.io/name`。应用到运行中的命名空间需删除并重建 Deployment 与 Service |
| S-8 | 缺少 `.gitattributes`，Windows 上格式化门禁不可用 | 已新增，全部文本文件统一 `eol=lf`。`gofmt -l .` 从误报 108 个文件变为输出为空，门禁首次真正可用 |
