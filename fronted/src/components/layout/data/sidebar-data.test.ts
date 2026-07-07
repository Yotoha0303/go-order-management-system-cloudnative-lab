import { describe, expect, it } from 'vitest'
import { getSidebarData } from './sidebar-data'

const titles = (isAdmin: boolean) =>
  getSidebarData(isAdmin).navGroups.flatMap((group) =>
    group.items.map((item) => item.title)
  )

describe('getSidebarData', () => {
  it('shows management modules to administrators', () => {
    expect(titles(true)).toEqual([
      '业务仪表盘',
      '商品管理',
      '库存管理',
      '订单管理',
      '库存流水',
    ])
  })

  it('hides management modules from regular users', () => {
    expect(titles(false)).toEqual(['业务仪表盘', '订单管理'])
  })
})
