# Go 订单库存一致性管理服务

> 一个面向 Go 后端求职展示的订单库存业务服务，重点演示事务一致性、并发库存扣减、用户级幂等、订单状态机、RabbitMQ 延迟取消、用户数据隔离、Redis 缓存和基础工程化能力。

本项目不是完整商城系统，不刻意堆购物车、支付、物流、优惠券等外围功能；项目重点是把后端开发中最容易被问到、也最能体现工程能力的订单库存主链路做扎实。

## 一眼看懂这个项目

| 维度 | 说明 |
| --- | --- |
| 项目定位 | Go 订单库存一致性管理服务 |
| 适合场景 | Go 后端求职项目、事务/锁/幂等/状态机训练、业务后端项目展示 |
| 核心主链路 | 登录用户创建订单 → 校验幂等 Key → 事务扣减库存 → 写入订单明细与库存流水 → 状态流转 |
| 主要后端能力 | Gin、GORM、MySQL 事务、行锁、条件更新、RabbitMQ、Outbox、JWT、Redis cache-aside、Docker、CI |
| 展示重点 | 不是 CRUD 数量，而是业务一致性、并发安全、数据隔离和工程化完整度 |

## 核心业务闭环

```mermaid
flowchart LR
    A[用户注册 / 登录] --> B[JWT 鉴权]
    B --> C[商品与库存准备]
    C --> D[提交订单]
    D --> E[用户级幂等校验]
    E --> F[开启事务]
    F --> G[锁定库存并条件扣减]
    G --> H[创建订单与订单明细]
    H --> I[写入库存流水]
    I --> J{订单状态机}
    J -->|支付| K[已支付]
    K -->|完成| L[已完成]
    J -->|取消| M[已取消并回滚库存]
```

## 项目重点解决的问题

| 问题 | 解决方式 | 项目价值 |
| --- | --- | --- |
| 重复提交导致重复订单 | `(user_id, idempotency_key)` 唯一索引 + SHA-256 请求摘要 | 避免用户重复点击、客户端重试造成重复下单和重复扣库存 |
| 并发下单导致超卖 | MySQL 事务 + `SELECT ... FOR UPDATE` + `stock_quantity >= quantity` 条件扣减 | 保证库存不会被扣成负数 |
| 多表写入不一致 | 幂等记录、订单、订单项、库存、库存流水放在同一事务 | 任一步失败时整体回滚 |
| 订单越权访问 | 订单查询和状态变更同时匹配当前登录用户 `user_id` | 防止用户访问或操作他人订单 |
| 非法订单状态流转 | service 层状态机 + DAO 条件状态更新 | 限制待支付、已支付、已完成、已取消之间的合法流转 |
| 商品详情重复查询 | Redis cache-aside 缓存 + 商品上下架删除缓存 | 减少热点商品详情对 MySQL 的重复访问 |
| 待支付订单长期占用库存 | 事务 Outbox + RabbitMQ TTL/DLX + 幂等消费者 | 订单创建 30 分钟后仍未支付则自动取消并回补库存 |

## 核心功能

### 用户与鉴权

- 用户注册、登录、查询当前用户信息
- 修改昵称、修改密码
- HS256 JWT 签发、过期校验和 Bearer 鉴权
- 除健康检查、注册和登录外，业务接口统一要求 JWT
- 订单读写接口基于当前登录用户做数据隔离

### 商品模块

- 创建商品
- 查询商品列表和商品详情
- 商品上架、下架
- 商品价格使用 `price_fen`，单位为分，避免浮点精度问题
- 商品创建后默认下架，避免无库存商品直接进入下单链路

### 库存模块

- 初始化商品库存
- 增加商品库存
- 查询商品库存
- 记录库存变更流水
- 通过库存流水追踪初始化、入库、下单扣减、取消回滚等业务来源

### 订单模块

- 使用 `idempotency_key` 幂等创建订单
- 创建订单时扣减库存
- 库存不足时事务回滚
- 查询订单列表和订单详情
- 支付、完成、取消订单
- 取消待支付订单时回滚库存
- 订单状态机限制非法状态流转
- 待支付超过 30 分钟后由 RabbitMQ 触发自动取消和库存回补
- 创建订单与超时 Outbox 同事务提交，发布使用 Publisher Confirm，消费使用手动 ACK

### 前端管理台

- React 管理台已接入商品、库存、库存流水、订单和用户资料接口
- 首页作为项目展示页，突出后端健康状态、订单摘要、库存流水和工程能力点
- 可完整演示商品准备、库存初始化、用户下单、支付、取消、库存回滚等业务流程

## 技术栈

### 后端

- Go 1.25.7
- Gin + GORM
- MySQL 8.4
- Redis 7.2
- RabbitMQ 4.1
- JWT
- Goose 数据库迁移
- YAML + godotenv 配置

### 前端管理台

- React 19 + TypeScript + Vite
- TanStack Router / Query
- Axios + Zustand
- Tailwind CSS + Shadcn UI

### 工程化

- Docker + Docker Compose
- Makefile
- GitHub Actions
- golangci-lint
- go test / go test -race / go vet / go build

## 项目结构

```text
.github/workflows/ci.yml  持续集成配置
cmd/                      项目启动入口
config/                   YAML 加载、环境变量覆盖和配置校验
docs/                     设计文档、REST Client 请求和验证证据
internal/apperror/        业务错误定义与错误码映射
internal/app/             依赖装配、HTTP Server 和优雅退出
internal/auth/            JWT 签发、校验和认证上下文
internal/bizcache/        Redis 业务缓存
internal/dao/             数据库访问层
internal/handler/         HTTP 接口层
internal/middleware/      请求 ID、日志、超时和恢复中间件
internal/model/           GORM 数据模型
internal/ordertimeout/    RabbitMQ 延迟拓扑、Outbox 发布和超时消费
internal/request/         请求参数和校验规则
internal/response/        统一响应结构
internal/service/         业务规则、状态机和事务
fronted/                  React 管理台、认证状态和后端 API 适配层
migrations/               Goose SQL 迁移
pkg/database/             MySQL 初始化与连接池
pkg/redis/                Redis 客户端初始化
router/                   路由注册
compose.yml               应用、MySQL、Redis、RabbitMQ 编排
Dockerfile                应用镜像多阶段构建
Makefile                  开发、测试、Docker 和迁移命令入口
```

## 分层说明

项目采用常见业务后端分层方式：

| 层 | 职责 |
| --- | --- |
| handler | HTTP 请求处理、参数绑定、错误映射和统一响应 |
| service | 业务规则、状态流转、事务控制和跨表操作 |
| dao | 数据库 CRUD、条件查询和条件更新 |
| model | 数据库表结构映射 |
| request / response | 入参校验与响应结构 |
| bizcache | 业务缓存读写、缓存 key 规则和缓存失效 |
| apperror | 业务错误、错误码和错误信息封装 |

核心原则：**handler 不写业务规则，service 不直接拼 HTTP 响应，dao 不处理业务状态。**

## 核心表设计

| 表 | 作用 | 关键设计 |
| --- | --- | --- |
| `users` | 用户表 | `username` 唯一、密码哈希、用户状态、软删除 |
| `roles` | 角色表 | `role_name` 唯一，内置 `admin` 与 `user` |
| `user_roles` | 用户角色关联表 | `user_id` 唯一，当前每个用户只绑定一个角色 |
| `products` | 商品表 | `price_fen` 金额分、商品状态、默认下架 |
| `product_inventories` | 商品库存表 | `product_id` 唯一，保证一个商品一条库存记录 |
| `stock_logs` | 库存流水表 | 记录变更前、变更量、变更后、业务类型和业务 ID |
| `orders` | 订单主表 | `user_id` 归属隔离、`order_no` 唯一、订单状态机 |
| `order_items` | 订单明细表 | 保存商品名称、价格快照和数量 |
| `order_idempotency_keys` | 订单幂等表 | `(user_id, idempotency_key)` 复合唯一索引 + `request_hash` |
| `order_timeout_outbox` | 订单超时 Outbox | 与订单同事务写入，记录发布时间、重试次数和超时截止点 |

详细表结构见：[docs/table_design.md](docs/table_design.md)

## 核心业务设计

- 订单创建流程见：[docs/order_flow.md](docs/order_flow.md)
- 幂等设计见：[docs/idempotency.md](docs/idempotency.md)
- Redis 缓存设计见：[docs/cache_design.md](docs/cache_design.md)
- 完整业务规则见：[docs/business_rules.md](docs/business_rules.md)
- 接口清单见：[docs/api_list.md](docs/api_list.md)

## 工程化能力

- HTTP Server 设置 ReadTimeout、WriteTimeout、IdleTimeout、ReadHeaderTimeout 和 MaxHeaderBytes
- MySQL 初始化时配置连接池：MaxOpenConns、MaxIdleConns、ConnMaxLifetime、ConnMaxIdleTime
- 启动时使用 PingContext 检查 MySQL 连通性
- 请求层使用 Request ID、Access Log、Recovery 和超时中间件
- Redis 不可用时商品详情缓存自动降级，不影响主流程
- RabbitMQ 使用 TTL + DLX 延迟投递；发布端 Confirm、消费端手动 ACK，重复消息由订单状态条件更新消解
- Dockerfile 使用多阶段构建和非 root 用户运行应用
- Docker Compose 编排应用、MySQL、Redis 和 RabbitMQ，并通过健康检查控制依赖启动顺序
- Goose 管理数据库版本，Makefile 统一封装开发、测试、Docker 和迁移命令
- CI 覆盖 go test、go test -race、go vet、golangci-lint、goose validate 和 go build

## 快速启动

### 前置依赖

- Go 1.25.7
- GNU Make
- Docker 与 Docker Compose
- Goose v3.27.1

```bash
go mod download
go install github.com/pressly/goose/v3/cmd/goose@v3.27.1
```

### 配置环境变量

应用启动时先加载 `.env`，再读取 [config.yml](config.yml)。环境变量会覆盖 YAML 中适合按环境变化的连接配置。

```env
MYSQL_PASSWORD=your-password
JWT_SECRET=replace-with-at-least-32-random-characters
JWT_EXPIRE_HOURS=24
REDIS_PASSWORD=
RABBITMQ_URL=amqp://order_app:order_dev_password@127.0.0.1:5672/
ORDER_TIMEOUT_DELAY=30m
```

不要提交真实 `.env`，可从 [.env.example](.env.example) 复制后修改。

### 本地运行后端

PowerShell 示例：

```powershell
Copy-Item .env.example .env
$env:MYSQL_PASSWORD = "your-password"
$env:JWT_SECRET = "replace-with-at-least-32-random-characters"

make infra-up
make migrate-up
make run
```

默认访问地址：`http://localhost:8082`

健康检查：

```bash
curl http://localhost:8082/ping
curl http://localhost:8082/live
curl http://localhost:8082/readyz
```

### Docker 运行完整服务

```powershell
$env:MYSQL_PASSWORD = "your-password"
$env:JWT_SECRET = "replace-with-at-least-32-random-characters"

make docker-up
```

`make docker-up` 会构建应用镜像，启动 MySQL、Redis、RabbitMQ 和应用，并执行数据库迁移。

### 本地运行前端

```powershell
Set-Location fronted
npm install
npm run dev
```

前端默认运行在 `http://127.0.0.1:8880`，Vite 会把 `/api` 和 `/ping` 代理到 `http://localhost:8082`。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `make infra-up` | 仅启动 MySQL、Redis 和 RabbitMQ，并等待健康 |
| `make infra-down` | 停止 Compose 项目 |
| `make migrate-validate` | 静态校验迁移文件 |
| `make migrate-status` | 查看数据库迁移状态 |
| `make migrate-up` | 执行全部待处理迁移 |
| `make migrate-down` | 回滚最近一条迁移 |
| `make docker-build` | 构建应用镜像 |
| `make docker-up` | 构建并启动应用、MySQL、Redis、RabbitMQ |
| `make docker-down` | 停止并移除容器，保留数据卷 |
| `make test` | 运行全部 Go 测试 |
| `make test-service` | 运行 MySQL service 集成测试 |
| `make test-dao` | 运行关键 DAO MySQL 集成测试 |
| `make test-migrations` | 在隔离数据库实跑迁移和回滚 |
| `make test-redis` | 运行 Redis 集成测试 |
| `make test-order-timeout` | 运行 RabbitMQ + MySQL 订单超时端到端测试 |
| `make test-all` | 运行普通测试、MySQL service/DAO/迁移测试和 Redis 集成测试 |
| `make test-race` | 使用 race detector 运行测试 |
| `make check` | 执行格式化、模块校验、vet 和测试 |

## 测试与验证

service 测试会清理所连接数据库中的业务表。必须使用独立测试库，禁止将 `DB_NAME` 指向含有开发数据或生产数据的数据库。

当前测试重点：

- 商品、库存、订单创建、状态机和关键异常分支
- 并发下单防超卖
- 多商品事务回滚
- 创建订单幂等：同 Key 重放、不同请求冲突、并发同 Key、失败回滚后重试
- 订单状态并发：并发支付、并发取消、支付与取消竞争
- Redis 商品详情缓存命中、删除和降级

手动接口测试文件位于 [docs/http](docs/http)，完整业务链路见 [docs/http/demo_flow.http](docs/http/demo_flow.http)。测试计划见 [docs/test_plan.md](docs/test_plan.md)。

## 项目文档

- [docs/api_list.md](docs/api_list.md)：接口清单
- [docs/business_rules.md](docs/business_rules.md)：业务规则
- [docs/table_design.md](docs/table_design.md)：数据表设计
- [docs/order_flow.md](docs/order_flow.md)：订单创建、取消和状态流转说明
- [docs/idempotency.md](docs/idempotency.md)：订单创建幂等设计说明
- [docs/cache_design.md](docs/cache_design.md)：Redis 商品详情缓存设计说明
- [docs/test_plan.md](docs/test_plan.md)：测试计划
- [docs/test_result.md](docs/test_result.md)：测试结果记录
- [docs/project_evolution.md](docs/project_evolution.md)：后续演进
- [docs/interview_guide.md](docs/interview_guide.md)：简历描述、项目讲解和面试追问
- [docs/evidence](docs/evidence)：项目运行、测试与关键业务截图证据

## 当前边界与后续演进

当前项目重点是后端业务基本功，不把系统包装成完整电商平台。

后续可演进方向：

- 增加订单列表状态筛选和时间范围筛选
- 扩展 handler 边界测试和 DAO 集成测试覆盖
- 增加 Prometheus 指标、结构化日志字段规范和链路追踪
- 为幂等记录增加过期清理策略
- 增加操作日志，记录管理员对商品与库存的变更
