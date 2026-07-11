# CI 基线验证

## 1. 目标

在模块化单体和微服务改造前，建立统一、可重复的数据库命名与验证入口。

## 2. 规范数据库名称

| 用途 | 名称 |
| --- | --- |
| 开发/Compose 数据库 | `go_order_management_system` |
| MySQL 集成测试数据库 | `go_order_management_system_test` |

命名规则：

- 只使用小写字母、数字和下划线；
- 不使用连字符；
- 不继续使用旧名称 `go_order_inventory_test`。

## 3. 配置映射

以下位置必须保持一致：

- `config.yml` 的 `mysql.database`；
- `Makefile` 的 `DB_NAME`；
- `.env.example` 的 `DB_NAME`；
- `compose.yml` 中 app、mysql 和 migrate 的默认 `DB_NAME`；
- `.env.example` 的 `MYSQL_TEST_DATABASE`；
- `.github/workflows/ci.yml` 的 `MYSQL_TEST_DATABASE`；
- CI MySQL service 的 `MYSQL_DATABASE`；
- 测试辅助代码读取的 `MYSQL_TEST_DATABASE`。

## 4. 静态验证

当前修复已确认：

- CI 设置 `MYSQL_TEST_DATABASE=go_order_management_system_test`；
- CI MySQL service 创建 `go_order_management_system_test`；
- 测试数据库名称符合测试辅助代码的 `^[a-zA-Z0-9_]+$` 约束；
- Compose 的 app、mysql 和 migrate 默认使用 `go_order_management_system`；
- `config.yml`、Makefile 和 `.env.example` 使用相同开发库名。

## 5. 本地验证命令

### 不依赖基础设施

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
go build ./...
goose -dir migrations validate
docker compose config --quiet
```

### MySQL/Redis/RabbitMQ 集成验证

准备环境变量：

```bash
export MYSQL_PASSWORD=your_password
export MYSQL_TEST_PASSWORD=your_password
export MYSQL_TEST_DATABASE=go_order_management_system_test
export JWT_SECRET=replace_with_a_32_plus_chars_random_secret
```

启动基础设施：

```bash
make infra-up
```

运行集成测试：

```bash
make test-service
make test-dao
make test-migrations
make test-redis
make test-order-timeout
```

完整验证：

```bash
make ci
```

## 6. Compose 验证

```bash
export MYSQL_PASSWORD=your_password
export JWT_SECRET=replace_with_a_32_plus_chars_random_secret

docker compose config --quiet
docker compose up -d --build --wait
docker compose ps
```

健康检查：

```bash
curl http://127.0.0.1:8082/ping
curl http://127.0.0.1:8082/live
curl http://127.0.0.1:8082/readyz
```

## 7. CI 验证标准

GitHub Actions 应完成：

- golangci-lint；
- `go test ./...`；
- `go test -race ./...`；
- `go vet ./...`；
- `go build ./...`；
- Goose migration validate；
- 应用二进制构建；
- Docker image 构建。

## 8. 当前验证状态

| 检查 | 状态 |
| --- | --- |
| 配置名称静态一致性 | 已完成 |
| 测试名称正则兼容性 | 已完成 |
| 分支差异范围检查 | 已完成 |
| 本地 Go 测试 | 待执行 |
| Compose 实际启动 | 待执行 |
| GitHub Actions 实际运行 | 待创建 PR 或推送到受监听分支后执行 |

未执行的运行验证不得写成“已通过”。