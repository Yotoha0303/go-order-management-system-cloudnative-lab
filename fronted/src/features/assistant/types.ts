export type AssistantIntent =
  | 'get_low_stock_products'
  | 'get_order_status_summary'

export type LowStockResult = {
  threshold: number
  count: number
  items: Array<{
    product_id: number
    name: string
    stock: number
  }>
}

export type OrderStatusSummaryResult = {
  days: number
  from: string
  to: string
  total: number
  counts: Array<{
    status: 'pending' | 'paid' | 'finished' | 'cancelled'
    count: number
  }>
}

type AssistantResponseBase = {
  request_id: string
  answer: string
}

export type AssistantChatResponse =
  | (AssistantResponseBase & {
      intent: 'get_low_stock_products'
      data: LowStockResult
    })
  | (AssistantResponseBase & {
      intent: 'get_order_status_summary'
      data: OrderStatusSummaryResult
    })
