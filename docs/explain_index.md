# 索引设计说明

## 唯一性索引

- `users.username`：保证登录名唯一。
- `roles.role_name`：保证角色名唯一。
- `user_roles.user_id`：当前阶段限制每个用户只绑定一个角色。
- `product_inventories.product_id`：保证一个商品只有一条库存记录。
- `orders.order_no`：保证订单号唯一。
- `order_idempotency_keys(user_id, idempotency_key)`：隔离不同用户的幂等 Key。
- `order_timeout_outbox.event_id/order_id`：保证事件和订单超时记录唯一。

## 查询索引

- `products.status`：支持按上下架状态筛选。
- `stock_logs.product_id/biz_type/biz_id`：支持库存变更追踪。
- `orders(user_id, created_at)`：支持当前用户订单分页。
- `orders.status`：支持状态条件更新与排查。
- `order_timeout_outbox(published_at, next_attempt_at)`：支持 Worker 扫描待发布事件。
- `user_roles.role_id`：支持按角色反查用户。

索引只针对当前查询模式建立。新增状态、时间范围或运营统计查询前，应先使用 `EXPLAIN` 验证执行计划，避免为展示目的堆叠重复索引。
