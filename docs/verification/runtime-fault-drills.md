# 运行时故障演练

## 目标与边界

Phase 8.4 将现有可靠性机制转化为四类可重复故障演练。所有故障只作用于一次性 GitHub Actions Docker Compose 或独立 MySQL 容器；工作流有 35 分钟上限，失败时仍上传诊断并删除容器、网络和数据卷。

这些演练证明当前项目在单个 GitHub 托管 Runner 上能够检测、隔离和恢复指定故障，不等同于生产级混沌工程，也不证明多节点网络分区、可用区故障、云负载均衡、托管数据库切换或正式值班体系。

## 1. RabbitMQ 断连与恢复

### 故障注入

停止 Compose 中的 RabbitMQ 容器，不重启两类 Worker。

### 检测信号

Prometheus 指标 `go_order_rabbitmq_session_up` 必须从 1 下降为 0；工作流保留下降时的指标响应和检测耗时。

### 预期影响

依赖 RabbitMQ 的延迟取消发布和消费暂时不可用，但应用进程仍存在，数据库和 HTTP 基础服务不被销毁。

### 恢复动作

重新启动原 RabbitMQ 服务，等待 Worker 自身重连循环恢复会话。

### 恢复验证

`go_order_rabbitmq_session_up` 必须重新达到 1，且无需重启 Worker。随后运行完整 Order Saga，其中包含超时驱动的订单取消，证明恢复后的发布、延迟队列、消费和业务补偿链路可用。

## 2. HTTP 超时与熔断

### 故障注入

`TestFaultDrillHTTPTimeoutCircuitRecovery` 启动真实本地 HTTP Server。Server 在前两个调用中延迟响应头，超过生产 `resiliencehttp` Client 的响应头与总调用预算。

### 检测信号

前两个请求必须以网络超时结束，并记录总耗时和实际上游调用数。

### 预期影响

达到失败阈值后，同一上游和操作的 Circuit 进入 Open。下一请求必须立即得到 `ErrCircuitOpen`，实际 HTTP Server 调用数不得增加。

### 恢复动作

恢复 Server 的正常响应并等待 Open Interval 到期。

### 恢复验证

第一个 Half-Open 探针必须成功，随后普通请求继续成功，证明 Circuit 从 Half-Open 返回 Closed。证据记录 Open 拒绝耗时、Open 期间网络调用数和最终调用数。

## 3. Worker 进程崩溃与租约回收

### 故障注入

工作流先停止正常 Timeout Worker。Go 测试父进程启动一个独立测试子进程；子进程使用生产 `Worker.claimPending` 获取一条真实 `order_timeout_outbox_v2` 租约，并写出就绪证据。父进程确认数据库中的 `lease_owner=fault-owner` 后强制终止该子进程，不执行正常租约释放。

### 检测信号

数据库中保留原 Owner 和 `lease_until`，证明进程终止发生在租约持有期间。

### 预期影响

在租约到期前，其他 Worker 不得处理同一 Outbox；该任务在短暂窗口内处于等待状态，但不会永久丢失。

### 恢复动作

等待受控的短租约到期，创建 `recovery-worker` 并调用同一生产 Claim 路径。

### 恢复验证

Recovery Worker 必须回收同一个 Event ID，完成一次状态更新，最终满足：

- `status=published`；
- `attempts=1`；
- `lease_owner` 和 `lease_until` 已清空；
- 相同 Event ID 只有一行；
- 没有重复 Outbox 记录或永久卡住的租约。

该演练关注租约所有权和任务回收，不声称模拟操作系统、容器运行时或整机故障的全部行为。

## 4. 迁移失败隔离

### 故障注入

启动独立 MySQL 8.4 容器，数据目录使用 Tmpfs，不复用应用数据库卷。使用固定版本 Goose 对独立数据库执行包含非法 SQL 的迁移目录。

### 检测信号

Goose 必须返回非零状态，日志保留具体数据库错误和故障检测耗时。

### 预期影响

非法迁移不得创建目标业务表，且工作流不得生成 `promotion-approved` 标记；应用发布链路不会继续。

### 恢复动作

删除一次性迁移容器及其 Tmpfs 数据，并保持仓库正常迁移目录不变。

### 恢复验证

验证五个正常迁移目录仍可通过 `goose validate`，非法表不存在，Promotion 保持阻断。

## 综合验收与证据

四类演练完成后再次检查 Gateway Readiness 并运行完整 Order Saga。Artifact 包含：

- 每类演练的 JSON 结果；
- RabbitMQ Session 指标与恢复 Saga；
- HTTP 超时、Open、Half-Open 和 Closed 证据；
- Worker Owner 就绪、进程终止与租约回收证据；
- 非法迁移日志与正常迁移验证；
- 最终 Saga；
- 失败时的 Compose 状态、日志和 RabbitMQ 指标；
- 汇总文件 `fault-drill-summary.json`。

Artifact 仅包含合成数据和运行证据，不保存生产数据、真实凭据或外部基础设施信息。
