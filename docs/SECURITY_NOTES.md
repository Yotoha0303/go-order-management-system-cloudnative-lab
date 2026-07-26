# 安全遗留问题

本文记录**无法自动修复、或需要架构调整与人工决策**的安全问题，供决策后再动手。
可自动修复的问题已直接提交，不在此列。

扫描基线：`b3652b4` 之后的 main，2026-07-26。
使用工具：`govulncheck`、`gosec`、`staticcheck`、`npm audit`。

## 扫描结论快照

| 工具 | 结果 |
|---|---|
| govulncheck | **0 漏洞**（修复前 41 条，其中 19 条代码可达） |
| staticcheck | **0 告警**（默认检查集；非默认的 ST1000/ST1003 已被 `.golangci.yml` 排除，CI 不报） |
| gosec | 生产代码 1 条低危（见 S-2），其余 16 条均在 `*_test.go` |
| npm audit | 9 → **3 条 high**（均为 brace-expansion，见 S-1） |

---

## S-1 · 前端 brace-expansion 三条 high，修复需破坏性降级

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

---

## S-2 · gosec G304：配置文件路径来自变量

**风险等级**：低

`config/config.go:66` 以变量路径调用文件读取，gosec 判定为潜在路径穿越。经核对，全部调用点
传入的都是硬编码字面量 `"config.yml"`，当前不可利用。

**建议方案**：若确认该函数永远只读项目内配置，加 `// #nosec G304` 并注明理由；
若未来可能接受外部路径，改为白名单或 `filepath.Clean` + 前缀校验。

---

## S-3 · 前端 JWT 存放在 JS 可读 cookie

**风险等级**：中（需前后端配合改造）

`fronted/src/lib/cookies.ts` 由前端 JS 通过 `document.cookie` 写入 access token，因此**无法**加
`HttpOnly`，任何 XSS 都能直接读走长期有效令牌。另有两个次要问题：`Secure` 仅在页面本身是
https 时才加；cookie `Max-Age` 为 7 天，而 JWT 有效期 24 小时，令牌过期后 cookie 仍残留。

当前前端源码未发现 XSS 注入点（全量 grep `dangerouslySetInnerHTML` / `v-html` / `eval(` /
`new Function(` / `document.write` 均零命中），所以这是纵深防御问题而非已存在的可利用漏洞。

**建议方案**：由后端下发 `HttpOnly; Secure; SameSite=Strict` 的 cookie 并配套 CSRF 防护，
前端不再接触 token。过渡方案是先把 cookie `Max-Age` 与 token 有效期对齐，并引入 refresh token
缩短 access token 生存期。

---

## S-4 · 前端缺少 CSP 与安全响应头

**风险等级**：低

`fronted/index.html` 无 `Content-Security-Policy`，`fronted/netlify.toml` 也未配置
`X-Frame-Options`、`X-Content-Type-Options`、`Referrer-Policy`。一旦某个依赖被投毒即无兜底。

**建议方案**：在 netlify.toml 配置响应头。CSP 需按实际内联 style/script 与 API 域名逐条调试，
误配会直接白屏，建议先用 `Content-Security-Policy-Report-Only` 观察一段时间再切正式。

---

## S-5 · 依赖漏洞扫描在流程上是失效的

**风险等级**：中（流程问题，会让上述所有依赖漏洞无人发现）

两个独立原因叠加：

1. `.github/workflows/ci.yml` 中没有任何 audit 步骤，前端依赖漏洞不会被流水线拦截；
   Go 侧同样没有 `govulncheck` 步骤
2. 本机 npm registry 指向 `registry.npmmirror.com`，该镜像未实现 `/-/npm/v1/security/*` 端点，
   直接运行 `npm audit` 会 404 报错退出。本次扫描是显式加 `--registry=https://registry.npmjs.org`
   才成功。仓库内无 `.npmrc`，说明该配置来自全局 npm 设置

**建议方案**：CI 增加 `govulncheck ./...` 与 `npm audit --audit-level=high`（后者显式指定官方
registry）。是否让它们阻断构建需要决策 —— 建议先设为非阻断并观察噪音水平。

---

## S-6 · JWT issuer 仍是旧项目名（有意保留）

**风险等级**：无（记录以免被误当作重命名遗漏）

`cmd/{catalog,identity,inventory,order}-service/main.go` 与 `internal/app/deps.go` 中传给
`auth.NewTokenManager` 的 issuer 字符串仍是 `"go-order-management-system"`。

这不是重命名遗漏。issuer 在解析时由 `jwt.WithIssuer` 校验，一旦修改：所有已签发的 token
立即失效（24 小时内全部用户被 401），且五处必须严格一致，否则服务间鉴权互相不认。

**建议方案**：如确需改名，配合一次计划内的令牌轮换窗口进行，五处同步修改并提前通知用户重新登录。

---

## S-7 · Kubernetes 标签仍是旧项目名（有意保留）

**风险等级**：无（一致性问题）

`deploy/kubernetes/base/kustomization.yaml:18` 的 `app.kubernetes.io/part-of` 与
`namespace.yaml:6` 的 `app.kubernetes.io/name` 仍为 `go-order-management-system`。

`part-of` 位于 `commonLabels` 下，kustomize 会把它同时写入 Deployment 与 Service 的
**selector**，而 selector 在已有集群上不可变，修改后 `kubectl apply` 会直接报错，需要先删除
再重建。本项目 CI 使用一次性 kind 集群，影响有限，但对长期运行的环境是破坏性变更。

**建议方案**：与下一次需要重建集群的变更合并执行；顺带把已废弃的 `commonLabels` 迁移到
`labels`（kustomize 已在构建时给出 deprecation 警告）。

---

## S-8 · 缺少 .gitattributes，Windows 上格式化门禁不可用

**风险等级**：无（工程效率问题，但会导致门禁被绕过）

仓库 `core.autocrlf=true` 且没有 `.gitattributes`，工作区所有 `.go` 文件都是 CRLF，
导致 `gofmt -l .` 把**每一个**文件都报为未格式化（本次扫描时是 108 个，其中绝大多数根本没被
改动过）。AGENT.md 的提交前门禁要求该命令输出为空，在 Windows 上永远无法满足，实际效果是
这道门禁被跳过。

本次是改用「去掉 CR 后再判断」才得到真实结果：全仓库实际只有 1 个文件需要格式化，已修复。

**建议方案**：新增 `.gitattributes`，至少包含 `*.go text eol=lf`（建议再加 `* text=auto`）。
落地时会一次性重写行尾，应单独成一个提交，避免与业务改动混在一起。
