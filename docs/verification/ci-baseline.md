# CI 与运行验证

## 1. 目标

当前 CI 不只验证单体编译，而是验证：

- Go 代码质量；
- 单元和集成测试；
- 四套服务迁移；
- 六个服务二进制和镜像；
- 四数据库 Compose 拓扑；
- 两个订单超时 Worker 副本；
- 完整订单 Saga 业务链路。

## 2. 测试数据库

CI 的通用 MySQL 集成测试库为：

```text
go_order_management_system_test
```

命名只使用字母、数字和下划线，避免测试辅助代码和 MySQL 初始化名称不一致。

微服务 Compose 运行时使用四个独立数据库：

```text
go_order_identity
go_order_catalog
go_order_inventory
go_order_ordering
```

## 3. 静态和代码级验证

GitHub Actions 依次执行：

```bash
golangci-lint run ./...
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

其中 `go test ./...` 包含真实 MySQL 的 Outbox 租约领取测试，用于验证：

- Worker A 和 Worker B 不会领取重复事件；
- 多个 Worker 可以拆分同一批待处理事件；
- 租约过期后事件可被其他 Worker 重新领取。

## 4. 数据库迁移验证

### 历史单体迁移

```bash
goose -dir migrations validate
```

根迁移目录只保留用于原单体回归，不参与当前微服务 Compose 运行。

### 当前服务迁移

```bash
for domain in identity catalog inventory ordering; do
  goose -dir "migrations/${domain}" validate
done
```

运行时由以下一次性 Job 执行：

```text
identity-migrate
catalog-migrate
inventory-migrate
ordering-migrate
```

业务服务必须等待所属迁移 Job 成功完成。

## 5. 服务构建验证

CI 构建：

```text
api-gateway
identity-service
catalog-service
inventory-service
order-service
order-timeout-worker
```

二进制构建命令：

```bash
mkdir -p bin
for service in api-gateway identity-service catalog-service inventory-service order-service order-timeout-worker; do
  go build -trimpath -o "bin/${service}" "./cmd/${service}"
done
```

随后执行：

```bash
docker compose config --quiet
docker compose build
```

## 6. 完整拓扑启动

CI 使用：

```bash
docker compose up -d --wait --scale order-timeout-worker=2
```

这会实际启动：

- MySQL；
- RabbitMQ；
- `db-init`；
- 四个迁移 Job；
- API Gateway；
- Identity、Catalog、Inventory、Order；
- 两个 Order Timeout Worker。

启动后检查：

```bash
curl --fail --silent --show-error http://127.0.0.1:8082/readyz
worker_count="$(docker compose ps -q order-timeout-worker | wc -l | tr -d ' ')"
test "${worker_count}" -eq 2
```

## 7. 端到端 Saga 验证

CI 执行：

```bash
sh scripts/smoke/microservices-saga.sh
```

验证流程：

1. 注册管理员；
2. 在 Identity DB 中赋予管理员角色；
3. 创建并上架 Catalog 商品；
4. 初始化 Inventory 库存；
5. 注册买家并创建订单；
6. 重放相同幂等请求，确认不会重复预占；
7. 支付订单，确认 reserved 库存被正式消耗；
8. 创建并主动取消订单，确认库存释放；
9. 创建待支付订单；
10. 等待 RabbitMQ TTL/DLX 超时消息；
11. Worker 调用 Order 超时取消；
12. Inventory 释放预占并恢复可用库存。

## 8. 本地 Compose 验证

准备 `.env`：

```bash
cp .env.example .env
```

至少设置：

```env
MYSQL_PASSWORD=replace_with_a_database_password
JWT_SECRET=replace_with_a_32_plus_chars_random_secret
INTERNAL_SERVICE_TOKEN=replace_with_a_long_random_internal_service_token
```

启动：

```bash
docker compose config --quiet
docker compose up -d --build --wait --scale order-timeout-worker=2
docker compose ps
```

健康检查：

```bash
curl --fail http://127.0.0.1:8082/ping
curl --fail http://127.0.0.1:8082/live
curl --fail http://127.0.0.1:8082/readyz
```

业务冒烟：

```bash
export MYSQL_PASSWORD=replace_with_a_database_password
sh scripts/smoke/microservices-saga.sh
```

清理：

```bash
docker compose down -v --remove-orphans
```

## 9. 当前已通过的验收项

| 检查 | 状态 |
| --- | --- |
| golangci-lint | 已通过 |
| `go test ./...` | 已通过 |
| MySQL Outbox 租约测试 | 已通过 |
| race detector | 已通过 |
| `go vet ./...` | 已通过 |
| `go build ./...` | 已通过 |
| 历史单体迁移 validate | 已通过 |
| 四套服务迁移 validate | 已通过 |
| 六个服务二进制构建 | 已通过 |
| Compose 配置校验 | 已通过 |
| 全部服务镜像构建 | 已通过 |
| 四数据库拓扑启动 | 已通过 |
| 两个 Worker 副本检查 | 已通过 |
| Gateway readiness | 已通过 |
| 完整 Order Saga 冒烟 | 已通过 |
| Compose 正常清理 | 已通过 |

## 10. CI 尚未覆盖

- Kubernetes 资源校验和集群部署；
- Prometheus、Grafana 和 OpenTelemetry；
- Publisher Confirms；
- HTTP 熔断、限流和故障注入；
- Registry 镜像推送和环境持续部署；
- MySQL/RabbitMQ 备份恢复；
- 压测、SLO 和告警验证；
- `reconciliation_required` 自动对账。
