# 订单状态机说明

## 状态

| 状态 | 数值 | 允许的后续状态 |
|---|---:|---|
| pending | 1 | paid、cancelled |
| paid | 2 | finished |
| finished | 3 | 无 |
| cancelled | 4 | 无 |

## 写入规则

- 支付只允许 `pending -> paid`。
- 完成只允许 `paid -> finished`。
- 主动取消和超时取消只允许 `pending -> cancelled`。
- 状态更新同时匹配 `id + user_id + old_status`，并检查受影响行数，避免并发覆盖。
- 取消时订单状态、库存回补和库存流水必须处于同一事务。
- 已取消订单重复取消返回成功，但不能重复回补库存。

完整流程与异常处理见 [order_flow.md](order_flow.md) 和 [business_rules.md](business_rules.md)。
