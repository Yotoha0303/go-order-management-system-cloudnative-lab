import {
  Bot,
  History,
  LayoutDashboard,
  Package,
  ShoppingCart,
  Warehouse,
} from 'lucide-react'
import { type SidebarData } from '../types'

export function getSidebarData(isAdmin: boolean): SidebarData {
  return {
    navGroups: [
      {
        title: '业务模块',
        items: [
          {
            title: '业务仪表盘',
            url: '/',
            icon: LayoutDashboard,
          },
          ...(isAdmin
            ? [
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
              ]
            : []),
          {
            title: '订单管理',
            url: '/orders',
            icon: ShoppingCart,
          },
          ...(isAdmin
            ? [
                {
                  title: '库存流水',
                  url: '/stock-logs',
                  icon: History,
                },
                {
                  title: 'AI 运营助手',
                  url: '/assistant',
                  icon: Bot,
                },
              ]
            : []),
        ],
      },
    ],
  }
}
