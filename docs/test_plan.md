# 项目测试说明

## 1. 测试目的

本测试方法采用手动测试和自动测试两种不同的方法进行测试，首要的目的除了是验证商品、库存、订单模块在正常流程和错误流程下的数据一致性外，着重项目

核心代码的功能可靠性测试，并且有效缩短测试的流程。

其中，用 REST Client 手动测试进行接口测试，用自动化测试来围绕以订单创建和订单状态机状态流转为主的业务功能可靠性测试。

## 2. 测试类型

本项目当前包含三类测试方式：

1. REST Client 手动接口测试
2. service 层自动化测试
3. Redis 缓存集成测试


## 3. REST Client 接口测试

测试文件位置：

```text
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

### 3.1 商品模块自测

- [x] 创建商品成功
- [x] 创建商品后 `status = 2`
- [x] `price_fen <= 0` 返回参数错误
- [x] `name` 为空返回参数错误
- [x] 查询商品列表成功
- [x] 查询商品详情成功
- [x] 查询不存在商品返回错误
- [x] 商品上架成功
- [x] 商品下架成功

### 3.2 库存模块自测

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

### 3.3 库存流水自测

- [x] 不传 `product_id` 可以查询全部流水
- [x] 传 `product_id` 可以查询指定商品流水
- [x] `product_id` 非法返回参数错误
- [x] 初始化库存后能查到 `biz_type = 1`
- [x] 增加库存后能查到 `biz_type = 2`
- [x] 创建订单后能查到 `biz_type = 3`
- [x] 取消订单后能查到 `biz_type = 4`
- [x] `before_quantity / change_quantity / after_quantity` 正确

### 3.4 订单状态机测试

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

### 3.5 Redis 缓存接口自测

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
go test -v ./...
```

测试内容：

- [x] 商品创建、上下架和查询相关业务规则
- [x] 库存初始化、增加库存和库存异常场景
- [x] 创建订单时库存扣减、库存不足回滚
- [x] 订单支付、完成、取消状态流转
- [x] 已取消订单重复取消不会重复回滚库存
- [x] 关键异常链路返回预期业务错误

## 5. Redis 缓存集成测试


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
