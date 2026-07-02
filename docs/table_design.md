# 数据表设计说明

## 1. products 商品表

用途：保存商品基础信息。

核心字段：

- id：商品 ID
- name：商品名称
- description：商品描述
- price_fen：商品价格，单位为分
- status：商品状态，1 表示上架，2 表示下架
- created_at / updated_at：创建和更新时间

设计说明：

1. 商品价格使用 price_fen，避免浮点数精度问题
2. 商品创建后默认下架，避免未准备库存的商品直接被下单
3. status 加索引，方便按商品状态查询

## 2. product_inventories 商品库存表

用途：保存每个商品当前库存。

核心字段：

- id：库存记录 ID
- product_id：商品 ID
- stock_quantity：当前库存数量
- created_at / updated_at：创建和更新时间

设计说明：

1. product_id 使用唯一索引，保证一个商品只有一条库存记录
2. stock_quantity 使用 check 约束，保证库存不能小于 0
3. 当前库存以 product_inventories 为准，stock_logs 只作为变更记录

## 3. stock_logs 库存流水表

用途：记录每次库存变化。

核心字段：

- product_id：商品 ID
- change_quantity：本次变化数量
- before_quantity：变化前库存
- after_quantity：变化后库存
- biz_type：业务类型
- biz_id：业务 ID
- remark：备注
- created_at：创建时间

biz_type 说明：

- 1：初始化库存
- 2：手动入库
- 3：下单扣减库存
- 4：取消订单回滚库存

设计说明：

1. 库存流水用于追踪库存变化来源
2. before_quantity 和 after_quantity 便于排查库存异常
3. biz_id 可关联订单 ID，方便追踪业务来源

## 4. users 用户表

用途：保存登录账号、密码哈希、状态和登录审计时间。

核心字段：

- id：用户 ID
- username：唯一登录名
- password_hash：bcrypt 密码哈希，不通过 API 返回
- nickname：展示昵称
- status：1 正常，2 禁用
- last_login_at / deleted_at：登录审计与软删除字段

## 5. orders 订单表

用途：保存订单主信息。

核心字段：

- id：订单 ID
- user_id：订单所有者用户 ID
- order_no：订单号
- total_amount_fen：订单总金额，单位为分
- status：订单状态
- paid_at：支付时间
- completed_at：完成时间
- cancelled_at：取消时间
- created_at / updated_at：创建和更新时间

订单状态：

- 1：待支付
- 2：已支付
- 3：已完成
- 4：已取消

设计说明：

1. order_no 设置唯一索引，保证订单号唯一
2. total_amount_fen 使用分为单位，避免金额精度问题
3. 订单状态通过状态机控制，避免非法流转
4. `(user_id, created_at)` 索引支持当前用户订单列表
5. 所有订单读取和状态更新都同时匹配 user_id，防止越权访问

## 6. order_items 订单明细表

用途：保存订单中的商品明细。

核心字段：

- order_id：订单 ID
- product_id：商品 ID
- product_name：下单时商品名称
- product_price_fen：下单时商品价格
- quantity：购买数量
- subtotal_fen：小计金额

设计说明：

1. 保存商品名称和价格快照，避免商品后续改名或改价影响历史订单
2. order_id 加索引，方便查询订单详情
3. product_id 加索引，方便分析商品销售情况

## 7. order_idempotency_keys 订单幂等表

用途：仲裁同一用户的并发创建订单请求。

设计说明：

1. `(user_id, idempotency_key)` 为复合唯一索引，不同用户可使用相同 Key
2. request_hash 用于识别同一 Key 是否被不同请求内容复用
3. order_id 关联成功创建的订单，status 区分创建中和已创建
