# 架构风险登记表

> 本登记表用于约束后续模块化、微服务和云原生改造。风险等级基于当前代码基线，不等同于已发生故障。

## 1. 风险分级

| 等级 | 含义 |
| --- | --- |
| P0 | 已阻塞可信验证，必须在重构前处理 |
| P1 | 进入下一阶段前需要控制，否则会放大故障或返工 |
| P2 | 可在后续阶段处理，但必须持续记录 |

## 2. 风险总表

| ID | 等级 | 风险 | 当前证据 | 主要影响 | 建议处理阶段 |
| --- | --- | --- | --- | --- | --- |
| R-001 | P0 | CI 与测试数据库命名漂移 | CI、Compose、Makefile、env 和测试辅助代码存在连字符、下划线及旧名称混用 | CI 可能在测试初始化前确定性失败，无法作为重构安全网 | Phase 0 |
| R-002 | P1 | 订单创建事务跨越多个未来服务域 | 同一事务操作 Product、Inventory、Ordering、StockLog、Idempotency、Outbox | 直接拆服务会破坏 ACID，产生超卖、重复订单或状态不一致 | 模块化阶段前记录；Inventory 拆分前解决 |
| R-003 | P1 | API 与订单超时 Worker 同进程 | 应用启动同时运行 HTTP Server 和 Worker | API 扩容会复制 Outbox 扫描器和消费者，生命周期和资源无法独立管理 | Phase 2 |
| R-004 | P1 | Outbox 多副本竞争缺少租约机制 | 待发布查询没有 owner、lease、锁定状态或 `SKIP LOCKED` | 多 Worker 可能重复发布，增加消息和数据库负载 | Worker 独立部署前 |
| R-005 | P1 | readiness 只能表达 MySQL 状态 | `/readyz` 只执行数据库 Ping | 无法区分 API 可用、缓存降级、RabbitMQ 失效、Worker 停止 | K8s 前 |
| R-006 | P1 | 业务边界依赖开发者约定 | 代码按 handler/service/dao/model 技术层组织并共享 `*gorm.DB` | 模块之间可任意调用 DAO，重构容易产生循环依赖和数据越权 | Phase 1 |
| R-007 | P1 | RabbitMQ 是启动装配强依赖但未被探针覆盖 | Worker 构造失败会阻止应用启动；运行时状态未进入 readiness | 启动语义、降级语义和探针语义不一致 | Worker 分离前 |
| R-008 | P1 | 结构化观测不足 | 使用 `slog` TextHandler，缺少统一 service/environment/trace/event 字段 | 多服务后难以聚合日志和定位跨服务故障 | Observability 阶段 |
| R-009 | P2 | Redis 降级缺少显式状态指标 | 初始化失败只记录警告并禁用缓存 | 性能退化可能被误判为数据库或业务问题 | Observability 阶段 |
| R-010 | P2 | 前端验证与根 CI 覆盖范围不明确 | 根 CI 主要聚焦 Go、迁移和镜像 | 前端接口契约或构建问题可能未被主流水线发现 | Phase 0/1 |
| R-011 | P2 | 缺少实际容量和恢复基线 | 没有稳定记录 QPS、P95/P99、Outbox 恢复速度 | 无法量化拆分前后收益，也无法定义资源限制 | K8s/可靠性阶段前 |
| R-012 | P2 | 备份、恢复和回滚目标未固化 | 仓库文档未形成 RPO/RTO 和恢复演练记录 | 云部署后故障恢复依赖临时操作 | 上云前 |

## 3. 重点风险说明

### R-001：CI 数据库命名漂移

**状态：开放。**

当前风险不是代码风格问题，而是验证链路失真。任何架构重构都必须建立在可重复运行的测试和 CI 上。

完成标准：

- 开发库统一为 `go_order_management_system`；
- 测试库统一为 `go_order_management_system_test`；
- CI、Compose、Makefile、`.env.example`、测试辅助代码一致；
- CI 服务实际创建的数据库与测试代码读取的数据库一致；
- 记录本地和 CI 验证结果。

### R-002：订单与库存强事务耦合

**状态：接受现状，禁止直接拆分。**

当前事务是项目业务正确性的核心，不应把它视为需要立即消除的“坏设计”。风险来自于未来服务化时错误地认为远程调用可以等价替换本地事务。

控制措施：

- 先完成模块所有权；
- Catalog 可先做只读/校验类远程契约实验；
- Inventory 拆分前先实现库存预占；
- 引入补偿、幂等、对账和故障测试后再移动数据所有权。

### R-003/R-004：Worker 与多副本

**状态：进入 Kubernetes 前必须处理。**

当前 API 和 Worker 同进程时，增加 API 副本会同步增加：

- Outbox 扫描循环；
- RabbitMQ 发布者；
- RabbitMQ 消费者。

即使消费业务具有一定幂等性，重复发布仍会增加消息、数据库和排障成本。

控制措施：

1. 建立独立 `cmd/api` 和 `cmd/order-timeout-worker`；
2. API Deployment 与 Worker Deployment 独立；
3. Worker 初期固定单副本；
4. 多副本前引入数据库租约、悲观锁或 `SKIP LOCKED` 方案；
5. 增加 Outbox backlog、oldest event age、publish failures 指标。

### R-005/R-007：健康语义不完整

**状态：Kubernetes 前处理。**

建议拆分后定义：

| 运行单元 | liveness | readiness |
| --- | --- | --- |
| API | 进程/事件循环存活 | MySQL 可用；必要配置有效 |
| Worker | Worker 主循环存活 | MySQL、RabbitMQ 会话和拓扑可用 |

Redis 当前是可降级缓存，不应简单地把 Redis 故障等同于 API not-ready，但应暴露降级状态和指标。

### R-006：模块所有权缺少代码约束

**状态：Phase 1 处理。**

目标不是机械套用复杂 DDD，而是建立最小规则：

- 模块拥有自己的 application/domain/infrastructure/transport 边界；
- 其他模块不能直接调用其 DAO；
- 跨模块调用通过接口；
- 表所有权唯一；
- 共享平台代码不得反向依赖业务模块。

### R-008/R-009：可观测性不足

**状态：设计已立项，实施后置。**

最小日志字段建议：

- `timestamp`
- `level`
- `service`
- `environment`
- `request_id`
- `trace_id`
- `user_id`
- `order_id`
- `event_id`
- `operation`
- `duration_ms`
- `error`

最小指标建议：

- HTTP 请求数、错误率、P95/P99；
- MySQL 连接池使用情况；
- Redis 命中/失败；
- Outbox pending 数量和最老事件年龄；
- RabbitMQ 发布失败和消费重试；
- 订单创建成功、库存不足和幂等冲突次数。

## 4. 不确定项登记

以下不是已确认缺陷，在获得运行数据前不得写成事实：

| ID | 待验证问题 | 验证方式 |
| --- | --- | --- |
| U-001 | 当前最大稳定 QPS 与瓶颈位置 | 固定数据集压测并记录 CPU、内存、DB、P95/P99 |
| U-002 | Outbox 积压恢复吞吐 | 暂停 RabbitMQ、累积事件、恢复后测清空速度 |
| U-003 | 多 API 副本的重复发布频率 | 在隔离环境运行多副本并对 event_id/消息数量对账 |
| U-004 | Redis 降级后的延迟增幅 | 比较启用和禁用 Redis 的商品详情延迟与 DB QPS |
| U-005 | RabbitMQ 长时中断对 API 的实际影响 | 故障注入并观察创建订单、Outbox 和日志 |
| U-006 | 当前恢复时间和数据丢失窗口 | 执行数据库备份恢复演练并记录 RPO/RTO |

## 5. 阶段闸门

### 进入模块化单体前

- R-001 已关闭；
- Compose smoke test 可重复；
- 当前依赖和数据所有权已记录。

### 进入 Kubernetes 前

- API 与 Worker 已拆分；
- R-003、R-005、R-007 已控制；
- 每个运行单元有独立探针和资源配置；
- 有基础日志和指标。

### 进入 Inventory Service 拆分前

- R-002 有明确目标方案；
- 库存预占模型完成；
- Reserve/Confirm/Release 幂等完成；
- 补偿与对账流程完成；
- 故障测试通过。

## 6. 维护规则

- 每个架构 PR 必须检查是否新增、降低或关闭风险。
- 风险关闭需要代码、测试或运行证据，不以“已讨论”作为关闭依据。
- 未验证推断必须放入“不确定项”，不能升级为已确认事实。
- 任何为了展示技术数量而扩大系统复杂度的改造，都应先说明它解决的具体风险或业务问题。