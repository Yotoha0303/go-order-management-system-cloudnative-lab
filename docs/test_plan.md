# 项目测试说明

## 1. 测试目的

本测试方法采用手动测试和自动测试两种不同的方法进行测试，首要的目的除了是验证商品、库存、订单模块在正常流程和错误流程下的数据一致性外，着重项目

核心代码的功能可靠性测试，并且有效缩短测试的流程。

其中，用 REST Client 手动测试进行接口测试，用自动化测试来围绕以订单创建和订单状态机状态流转为主的业务功能可靠性测试。

## 2. 测试类型

本项目当前包含四类测试方式：

1. REST Client 手动接口测试
2. service 层自动化测试
3. Redis 缓存集成测试
4. React 前端自动化测试


## 3. REST Client 接口测试

测试文件位置：

```text
docs/http/auth.http
docs/http/demo_flow.http
docs/http/products.http
docs/http/inventory.http
docs/http/stock_logs.http
docs/http/orders.http
docs/http/redis.http
```

执行方式：

1. 安装 VS Code REST Client 插件
2. 启动项目：`go run cmd/main.go`
3. 打开对应 `.http` 文件
4. 点击每个请求上方的 `Send Request`
5. 对比响应结果和数据库变化

### 3.1 用户与鉴权模块自测

- [x] 用户注册成功
- [x] 用户登录成功并返回 access_token
- [x] 携带 Bearer Token 可以查询当前用户
- [ ] 修改当前用户昵称
- [ ] 修改当前用户密码
- [ ] 未携带 Token 访问受保护接口返回 401
- [ ] 使用两个账号手动验证订单数据隔离

### 3.2 商品模块自测

- [x] 创建商品成功
- [x] 创建商品后 `status = 2`
- [x] `price_fen <= 0` 返回参数错误
- [x] `name` 为空返回参数错误
- [x] 查询商品列表成功
- [x] 按上架、下架和全部状态筛选商品
- [x] 商品列表分页及分页参数边界
- [x] 查询商品详情成功
- [x] 查询不存在商品返回错误
- [x] 商品上架成功
- [x] 商品下架成功

### 3.3 库存模块自测

- [x] 存在商品可以初始化库存
- [x] 不存在商品不能初始化库存
- [x] 重复初始化库存失败
- [x] `stock_quantity = 0` 可以初始化
- [x] 初始化库存后 `product_inventories` 有记录
- [x] 初始化库存后 `stock_logs` 有 `biz_type = 1` 记录
- [x] 已初始化库存的商品可以增加库存
- [x] 未初始化库存的商品不能增加库存
- [x] `quantity <= 0` 返回参数错误
- [x] 增加库存后 `stock_quantity` 正确变化
- [x] 增加库存后 `stock_logs` 有 `biz_type = 2` 记录

### 3.4 库存流水自测

- [x] 不传 `product_id` 可以查询全部流水
- [x] 传 `product_id` 可以查询指定商品流水
- [x] `product_id` 非法返回参数错误
- [x] 初始化库存后能查到 `biz_type = 1`
- [x] 增加库存后能查到 `biz_type = 2`
- [x] 创建订单后能查到 `biz_type = 3`
- [x] 取消订单后能查到 `biz_type = 4`
- [x] `before_quantity / change_quantity / after_quantity` 正确

### 3.5 订单状态机测试

创建订单

- [x] 正常创建订单成功
- [x] 商品不存在时创建订单失败
- [x] 商品下架时创建订单失败
- [x] 库存不存在时创建订单失败
- [x] 库存不足时创建订单失败
- [x] 创建订单成功后 `orders` 有记录
- [x] 创建订单成功后 `order_items` 有记录
- [x] 创建订单成功后 `product_inventories` 库存扣减
- [x] 创建订单成功后 `stock_logs` 有 `biz_type = 3` 记录
- [x] 相同 idempotency_key 和相同请求返回同一订单
- [x] 相同 idempotency_key 和不同请求返回冲突
- [x] 并发使用相同 idempotency_key 只创建一笔订单
- [x] 创建失败时幂等记录随事务回滚，允许重试

支付订单

- [x] 待支付订单可以支付
- [x] 已支付订单重复支付失败
- [x] 已取消订单支付失败
- [x] 已完成订单支付失败
- [x] 不存在订单支付失败

完成订单

- [x] 已支付订单可以完成
- [x] 未支付订单完成失败
- [x] 已取消订单完成失败
- [x] 已完成订单重复完成失败
- [x] 不存在订单完成失败

取消订单

- [x] 待支付订单可以取消
- [x] 取消订单后库存回滚
- [x] 取消订单后 `stock_logs` 有 `biz_type = 4` 记录
- [x] 已支付订单取消失败
- [x] 已完成订单取消失败
- [x] 不存在订单取消失败
- [x] 已取消订单再次取消直接成功
- [x] 已取消订单再次取消不会重复回滚库存

### 3.6 Redis 缓存接口自测

- [x] 商品详情缓存 key 是否正确
- [x] Redis 为空时，缓存函数不会影响主流程
- [x] SetProductDetail 后能 GetProductDetail
- [x] DeleteProductDetailCache 后再次 Get 应该 miss
- [x] 缓存 TTL 是否存在

## 4. service 层自动化测试（包含订单状态机测试）

测试文件位置：

```text
internal/service/*_test.go
```

执行方式：

```bash
make test-service
```

测试内容：

- [x] 商品创建、上下架和查询相关业务规则
- [x] 库存初始化、增加库存和库存异常场景
- [x] 创建订单时库存扣减、库存不足回滚
- [x] 订单支付、完成、取消状态流转
- [x] 已取消订单重复取消不会重复回滚库存
- [x] 关键异常链路返回预期业务错误
- [x] 注册用户和默认角色在同一事务内成功或回滚
- [x] 修改密码后旧密码失效、新密码可登录

## 5. Handler 业务接口测试

测试文件位置：

```text
internal/handler/*_test.go
```

执行方式：

```bash
go test -v ./internal/handler
```

最小测试内容：

- [x] 创建订单时正确传递当前用户、幂等键和商品明细
- [x] 非法请求在调用 service 前返回 400
- [x] service 业务错误正确映射为 HTTP 状态码和业务错误码

## 6. DAO MySQL 集成测试

测试文件位置：

```text
internal/dao/dao_integration_test.go
```

执行方式：

```bash
make test-dao
```

测试内容：

- [x] 用户角色变更后查询立即生效
- [x] 条件扣库存不会把库存扣成负数
- [x] 订单查询和状态修改强制校验用户归属
- [x] AI 助手低库存查询使用真实 MySQL JOIN、阈值、排序和 limit
- [x] AI 助手订单状态使用数据库分组和半开时间区间
- [x] AI 调用日志写入真实 MySQL

## 7. 数据库迁移集成测试

执行方式：

```bash
make test-migrations
```

测试内容：

- [x] 在隔离数据库执行全部迁移和回滚
- [x] 存量用户自动回填 `user` 角色
- [x] `user_roles` 外键完整创建
- [x] `ai_call_logs` 字段和三个审计索引完整创建
- [x] `order_timeout_outbox` 字段和订单外键完整创建

## 8. RabbitMQ 订单超时取消测试

执行方式：

```bash
make test-order-timeout
```

- [x] 创建订单与超时 Outbox 同事务提交，失败整体回滚
- [x] 超时截止时间为订单创建时间加 30 分钟
- [x] 超时取消回补库存并写入一条回滚流水
- [x] 重复超时消息不重复回补库存
- [x] 已支付订单收到超时消息保持已支付
- [x] MySQL Outbox 截止时间作为事实源，提前投递不能取消订单
- [x] RabbitMQ 消息 expiration 使用剩余毫秒，已过期事件最小为 1ms
- [x] 非法或带未知字段的消息进入失败队列

## 9. Redis 缓存集成测试


测试文件位置：

```text
internal/bizcache/product_cache_test.go
```

执行方式：

```bash
RUN_REDIS_TEST=1 go test -v ./internal/bizcache
```

测试内容：

- [x] key 正确  
- [x] Redis nil 不 panic  
- [x] Set/Get/Delete 正常  
- [x] TTL 正常  
- [x] 接口层手动验证上下架删除缓存

## 10. AI 助手自动化测试

测试内容：

- [x] Structured Output 严格拒绝未知、重复和非对象字段
- [x] ToolRegistry 拒绝未知意图和重名工具
- [x] 两个只读工具覆盖默认值、边界、空数据、错误和取消
- [x] 401/403 在 LLM 调用之前终止
- [x] Handler 覆盖 400/401/403/422/502/504 映射
- [x] 调用日志不保存 message、prompt、tool arguments 或 tool result

## 11. React 前端自动化测试

测试文件位置：

```text
fronted/src/**/*.{test,spec}.{ts,tsx}
```

执行方式：

```bash
cd fronted
npm test
```

测试内容：

- [x] Token 过期后清除认证状态并触发登录跳转
- [x] 相同订单请求重试复用幂等 Key，请求内容改变后生成新 Key
- [x] 订单状态对应的支付、完成和取消操作权限
- [x] 完成和取消订单要求二次确认
- [x] 商品元转分转换及非法小数位校验
- [x] 登录、注册和账号退出交互
