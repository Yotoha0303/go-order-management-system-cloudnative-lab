# 接口清单

## 1. 健康检查

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /ping | 基础连通性检查 |
| GET | /live | 进程存活检查 |
| GET | /readyz | 数据库就绪检查；未就绪时返回 503 / 5001 |

除健康检查、注册和登录外，以下业务接口均要求请求头：`Authorization: Bearer <access_token>`。

## 2. 商品模块（需要鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/v1/products | 创建商品 |
| GET | /api/v1/products | 查询商品列表，当前默认查询下架商品 |
| GET | /api/v1/products/:id | 查询商品详情 |
| PATCH | /api/v1/products/:id/on-sale | 商品上架 |
| PATCH | /api/v1/products/:id/off-sale | 商品下架 |

## 3. 库存模块（需要鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/v1/inventory/init | 初始化库存 |
| POST | /api/v1/inventory/add | 增加库存 |
| GET | /api/v1/inventory/products/:product_id | 查询商品库存 |

## 4. 库存流水模块（需要鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/v1/stock-logs | 查询库存流水，product_id 可选 |

## 5. 用户与鉴权模块

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/v1/auth/register | 否 | 用户注册 |
| POST | /api/v1/auth/login | 否 | 登录并返回 access_token |
| GET | /api/v1/users/me | Bearer JWT | 查询当前用户 |
| PUT | /api/v1/users/me/profile | Bearer JWT | 修改当前用户昵称 |
| PATCH | /api/v1/users/me/password | Bearer JWT | 修改当前用户密码 |

## 6. 订单模块

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/v1/orders | Bearer JWT | 创建当前用户订单（idempotency_key 幂等） |
| GET | /api/v1/orders?page=1&page_size=20 | Bearer JWT | 分页查询当前用户订单列表；默认 1/20，page_size 最大 100 |
| GET | /api/v1/orders/:id | Bearer JWT | 查询当前用户订单详情 |
| PATCH | /api/v1/orders/:id/pay | Bearer JWT | 支付当前用户订单 |
| PATCH | /api/v1/orders/:id/finish | Bearer JWT | 完成当前用户订单 |
| PATCH | /api/v1/orders/:id/cancel | Bearer JWT | 取消当前用户订单 |

订单列表响应的 `data` 结构为：`{"orders":[],"total":0,"page":1,"page_size":20}`。`total` 仅统计当前登录用户的订单。

鉴权请求头：`Authorization: Bearer <access_token>`。未登录返回 401；访问其他用户订单返回 404，避免暴露订单是否存在。
