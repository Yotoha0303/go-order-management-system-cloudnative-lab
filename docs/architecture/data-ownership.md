# 当前数据所有权

## 当前数据库形态

当前阶段：共享 MySQL 数据库。

特点：

- 所有业务模块访问同一个数据库。
- 事务可以跨商品、库存、订单表完成。
- 未来拆分服务时需要重新设计一致性边界。

## 当前表归属建议

| 表 | 当前责任模块 | 未来候选服务 |
|---|---|---|
| users | Identity | Identity Service |
| roles | Identity | Identity Service |
| user_roles | Identity | Identity Service |
| products | Catalog | Catalog Service |
| product_inventories | Inventory | Inventory Service |
| stock_logs | Inventory | Inventory Service |
| orders | Ordering | Order Service |
| order_items | Ordering | Order Service |
| order_idempotency_keys | Ordering | Order Service |
| order_timeout_outbox | Ordering | Order Service / Event Publisher |

## 当前事务边界

订单创建目前是单数据库事务：

1. 创建订单
2. 校验商品
3. 锁定库存
4. 扣减库存
5. 创建库存流水
6. 保存订单幂等状态
7. 创建 Outbox