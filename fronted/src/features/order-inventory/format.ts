export const PRODUCT_STATUS = {
  ON_SALE: 1,
  OFF_SALE: 2,
} as const

export const ORDER_STATUS = {
  PENDING: 1,
  PAID: 2,
  FINISHED: 3,
  CANCELLED: 4,
} as const

export const STOCK_BIZ_TYPE = {
  INIT: 1,
  MANUAL_ADD: 2,
  ORDER_DEDUCT: 3,
  ORDER_ROLLBACK: 4,
} as const

export function formatFen(fen: number | null | undefined) {
  if (typeof fen !== 'number' || Number.isNaN(fen)) return '-'
  return `¥${(fen / 100).toFixed(2)}`
}

export function formatDateTime(value: string | null | undefined) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'

  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function productStatusText(status: number) {
  if (status === PRODUCT_STATUS.ON_SALE) return '已上架'
  if (status === PRODUCT_STATUS.OFF_SALE) return '已下架'
  return `未知状态 ${status}`
}

export function orderStatusText(status: number) {
  if (status === ORDER_STATUS.PENDING) return '待支付'
  if (status === ORDER_STATUS.PAID) return '已支付'
  if (status === ORDER_STATUS.FINISHED) return '已完成'
  if (status === ORDER_STATUS.CANCELLED) return '已取消'
  return `未知状态 ${status}`
}

export function stockBizTypeText(type: number) {
  if (type === STOCK_BIZ_TYPE.INIT) return '初始化库存'
  if (type === STOCK_BIZ_TYPE.MANUAL_ADD) return '手动入库'
  if (type === STOCK_BIZ_TYPE.ORDER_DEDUCT) return '下单扣减'
  if (type === STOCK_BIZ_TYPE.ORDER_ROLLBACK) return '取消回滚'
  return `未知类型 ${type}`
}
