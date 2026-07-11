# 当前运行流程

## 启动流程

1. 加载环境变量
2. 加载 config.yml
3. 初始化 MySQL
4. 初始化 Redis
5. 创建业务 Service
6. 创建 Order Timeout Worker
7. 注册 Gin Router
8. 启动 HTTP Server 和 Worker

## 当前运行单元

当前是单进程双职责：

- HTTP API Server
- Order Timeout Worker

Worker 与 API 共享：

- 数据库连接
- Service 实例
- 进程生命周期

## 停止流程

收到 SIGINT/SIGTERM 后：

1. 停止 Worker
2. HTTP Server 优雅退出
3. 等待 Worker 完成关闭

## 未来拆分关注点

需要解决：

- Worker 独立生命周期
- 数据库连接隔离
- 配置隔离
- 日志和指标隔离
- 服务间错误传播
