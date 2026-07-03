# 订单库存管理系统前端

本目录是 `go-order-management-system` 的 React 管理台，基于 Shadcn Admin 模板改造，并已接入项目 Go 后端的认证、用户、商品、库存、库存流水和订单接口。

## 已接入功能

- 用户注册、登录和登录状态保存
- 查询当前用户、修改昵称和修改密码
- 侧边栏与顶部菜单动态展示用户昵称和用户名
- 业务仪表盘与后端 `/ping` 连通状态
- 商品创建、服务端分页、详情、上架和下架
- 通过商品选择器初始化、增加和查询库存
- 库存流水列表与商品选择过滤
- 通过已上架商品选择器创建订单，以及订单详情、支付、完成和取消
- 创建订单网络重试复用同一幂等 Key，避免重复订单和重复扣库存
- Token 过期后统一清理登录状态并跳转登录页

后端尚未实现的 OAuth、找回密码、OTP、Clerk，以及 Users、Tasks、Chats 等模板演示页面均已移除，避免展示不可用入口。

## 技术栈

- React 19 + TypeScript
- Vite
- TanStack Router + TanStack Query
- Axios + Zustand
- Tailwind CSS + Shadcn UI
- React Hook Form + Zod
- Vitest Browser + Playwright

## 后端接口约定

默认 API 根路径为 `/api/v1`。登录成功后，访问令牌保存在认证状态中，Axios 请求拦截器会为受保护接口添加：

```http
Authorization: Bearer <access_token>
```

后端统一响应格式：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

主要 API 适配层：

```text
src/lib/api-client.ts                 Axios 实例、Token 和统一响应解包
src/features/auth/api.ts              注册、登录和当前用户接口
src/features/order-inventory/api.ts   商品、库存、流水、订单和健康检查接口
src/stores/auth-store.ts              用户与访问令牌状态
```

## 本地运行

先确保 Go 后端运行在 `http://localhost:8082`，然后执行：

```powershell
npm install
npm run dev
```

前端默认地址为 `http://127.0.0.1:8880`。开发服务器会将 `/api` 和 `/ping` 代理到后端。

如需修改部署后的 API 前缀，可设置：

```env
VITE_API_BASE_URL=/api/v1
```

生产环境建议通过同源反向代理连接 Go 服务；跨域直连需要后端额外配置 CORS。

## 质量检查

```powershell
npm run lint
npm run build
npm test
npm run format:check
```

## 目录说明

```text
src/components/                  通用 UI 与布局组件
src/features/auth/               登录、注册和认证 API
src/features/order-inventory/    核心业务页面与 API
src/features/settings/           昵称和密码设置页面
src/routes/                      TanStack Router 文件路由
src/stores/                      Zustand 状态
src/lib/                         API 客户端和通用工具
```

项目整体说明、后端启动方式和业务规则请查看根目录 [README](../README.md)。原始 UI 模板遵循本目录 [LICENSE](LICENSE) 中的 MIT License。
