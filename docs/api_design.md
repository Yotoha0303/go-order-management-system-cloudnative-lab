# API 设计说明

## 路径与响应

- 业务接口统一使用 `/api/v1` 前缀，健康检查使用 `/ping`、`/live`、`/readyz`。
- 成功与失败均使用统一响应结构：`{ "code": 0, "message": "success", "data": ... }`。
- Handler 负责参数绑定和错误映射，业务规则放在 Service，数据库操作放在 DAO。

## 认证与授权

- 注册、登录无需 Token；其余业务接口使用 Bearer JWT。
- JWT 只表达用户身份，角色通过数据库实时查询。
- 商品写操作、库存写操作和库存流水查询仅允许 `admin`；普通用户可查看商品、库存并操作自己的订单。
- 跨用户订单访问按不存在处理，避免暴露资源信息。

## 输入边界

- 列表接口限制页码与 `page_size`，默认值由 Handler 明确设置。
- 金额使用整数分，库存与购买数量必须为正数或非负数。
- 创建订单要求客户端提供 `idempotency_key`，相同 Key 的不同请求内容返回冲突。

完整接口见 [api_list.md](api_list.md)，权限规则见 [permission_design.md](permission_design.md)。
