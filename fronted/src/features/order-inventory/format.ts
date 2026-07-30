export const PRODUCT_STATUS = {
  ON_SALE: 1,
  OFF_SALE: 2,
} as const

export const ORDER_STATUS = {
  RESERVING: 'reserving',
  PENDING: 'pending',
  PAYING: 'paying',
  PAID: 'paid',
  CANCELLING: 'cancelling',
  CANCELLED: 'cancelled',
  FINISHED: 'finished',
  FAILED: 'failed',
  RECONCILIATION_REQUIRED: 'reconciliation_required',
} as const

// inventory-service change_type strings (replaces monolith biz_type codes).
export const STOCK_CHANGE_TYPE = {
  INITIALIZE: 'initialize',
  ADD: 'add',
  RESERVE: 'reserve',
  CONFIRM: 'confirm',
  RELEASE: 'release',
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

export function orderStatusText(status: string | number) {
  if (status === ORDER_STATUS.PENDING) return '待支付'
  if (status === ORDER_STATUS.PAYING) return '支付中'
  if (status === ORDER_STATUS.PAID) return '已支付'
  if (status === ORDER_STATUS.FINISHED) return '已完成'
  if (status === ORDER_STATUS.CANCELLING) return '取消中'
  if (status === ORDER_STATUS.CANCELLED) return '已取消'
  if (status === ORDER_STATUS.RESERVING) return '预占库存中'
  if (status === ORDER_STATUS.FAILED) return '失败'
  if (status === ORDER_STATUS.RECONCILIATION_REQUIRED) return '待对账'
  return `未知状态 ${status}`
}

export function stockChangeTypeText(type: string | number) {
  if (type === STOCK_CHANGE_TYPE.INITIALIZE) return '初始化库存'
  if (type === STOCK_CHANGE_TYPE.ADD) return '手动入库'
  if (type === STOCK_CHANGE_TYPE.RESERVE) return '预占库存'
  if (type === STOCK_CHANGE_TYPE.CONFIRM) return '确认扣减'
  if (type === STOCK_CHANGE_TYPE.RELEASE) return '释放回滚'
  return `未知类型 ${type}`
}

/** @deprecated use stockChangeTypeText — kept for any leftover imports */
export function stockBizTypeText(type: string | number) {
  return stockChangeTypeText(type)
}
