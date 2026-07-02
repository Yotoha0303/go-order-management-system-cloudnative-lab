import {
  ClipboardList,
  History,
  LayoutDashboard,
  Package,
  PackageCheck,
  ShoppingCart,
  Warehouse,
} from 'lucide-react'
import { type SidebarData } from '../types'

export const sidebarData: SidebarData = {
  user: {
    name: 'Inventory Admin',
    email: 'ops@example.local',
    avatar: '/avatars/shadcn.jpg',
  },
  teams: [
    {
      name: '订单库存系统',
      logo: PackageCheck,
      plan: 'Go + React',
    },
    {
      name: '业务后台',
      logo: Warehouse,
      plan: '本地环境',
    },
    {
      name: '接口联调',
      logo: ClipboardList,
      plan: 'API v1',
    },
  ],
  navGroups: [
    {
      title: '业务模块',
      items: [
        {
          title: '业务仪表盘',
          url: '/',
          icon: LayoutDashboard,
        },
        {
          title: '商品管理',
          url: '/products',
          icon: Package,
        },
        {
          title: '库存管理',
          url: '/inventory',
          icon: Warehouse,
        },
        {
          title: '订单管理',
          url: '/orders',
          icon: ShoppingCart,
        },
        {
          title: '库存流水',
          url: '/stock-logs',
          icon: History,
        },
      ],
    },
  ],
}
