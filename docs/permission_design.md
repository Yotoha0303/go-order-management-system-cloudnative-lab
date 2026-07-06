# 权限设计

## 1. 当前范围

项目采用最小 RBAC：每个用户当前只绑定一个角色，内置角色为 `admin` 和 `user`。数据库保留 `roles` 与 `user_roles` 两张表，`user_roles.user_id` 唯一，以避免现阶段引入多角色合并规则。

| 能力 | user | admin |
|---|---:|---:|
| 查看个人资料、修改个人资料 | 是 | 是 |
| 创建、查看、支付、完成、取消自己的订单 | 是 | 是 |
| 查看商品和商品库存 | 是 | 是 |
| 创建、上下架商品 | 否 | 是 |
| 初始化、增加库存 | 否 | 是 |
| 查看库存流水 | 否 | 是 |

## 2. 校验方式

JWT 只保存用户身份，不保存角色。管理员中间件收到请求后，通过一次 `EXISTS + JOIN` 查询实时确认角色：

```sql
SELECT EXISTS (
    SELECT 1
    FROM user_roles AS ur
    INNER JOIN roles AS r ON r.id = ur.role_id
    WHERE ur.user_id = ? AND r.role_name = 'admin'
);
```

这样角色变更会在下一次请求立即生效，也没有缓存失效问题。当前管理接口访问量较小，不需要为角色查询引入 Redis。若后续性能数据证明这里是瓶颈，再增加短 TTL 缓存和主动失效机制。

## 3. 用户注册与存量数据

- 注册时，用户记录和默认 `user` 角色绑定在同一数据库事务中创建，任一步失败都会回滚。
- `00011_user_roles.sql` 会把迁移前已有用户回填为 `user`。
- 前端只负责隐藏入口和改善体验，不能代替后端鉴权。

## 4. 设置管理员

项目暂不提供公开的角色管理 API，避免普通用户自行提权。首次部署后由数据库管理员执行以下 SQL，将指定账号提升为管理员：

```sql
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users AS u
INNER JOIN roles AS r ON r.role_name = 'admin'
WHERE u.username = 'replace_with_username'
ON DUPLICATE KEY UPDATE role_id = VALUES(role_id);
```

执行后应确认影响的用户名，并重新调用 `/api/v1/users/me` 检查返回的 `roles`。由于角色不写入 JWT，不需要重新签发令牌。
