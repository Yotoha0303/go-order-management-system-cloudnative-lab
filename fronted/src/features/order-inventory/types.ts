export type Product = {
  id: number
  name: string
  description: string
  price_fen: number
  status: number
  created_at: string
  updated_at: string
}

/**
 * Omit the status entirely to list every product. The catalog service parses
 * this value as an integer, so the literal 'all' the monolith accepted is now
 * rejected with a 400.
 */
export type ProductListStatus = 1 | 2

export type ProductList = {
  list: Product[]
  total: number
  page: number
  page_size: number
}

export type Inventory = {
  id: number
  product_id: number
  stock_quantity: number
  created_at: string
  updated_at: string
}

// inventory-service stock logs (inventory_stock_logs), not the monolith shape.
export type StockChangeType =
  | 'initialize'
  | 'add'
  | 'reserve'
  | 'confirm'
  | 'release'
  | string

export type StockLog = {
  id: number
  product_id: number
  change_type: StockChangeType
  quantity: number
  reference_id?: string
  created_at: string
}

// inventory-service listLogs returns a page envelope, not a bare array.
export type StockLogList = {
  list: StockLog[]
  total: number
  page: number
  page_size: number
}

// order-service statuses are strings (orders_v2), not the monolith's tinyint codes.
export type OrderStatus =
  | 'reserving'
  | 'pending'
  | 'paying'
  | 'paid'
  | 'cancelling'
  | 'cancelled'
  | 'finished'
  | 'failed'
  | 'reconciliation_required'

export type OrderItem = {
  id: number
  order_id: number
  product_id: number
  product_name: string
  price_fen: number
  quantity: number
}

// create/get return the order itself (items nested). There is no order_no field.
export type Order = {
  id: number
  user_id: number
  status: OrderStatus | string
  total_fen: number
  reservation_id: string
  idempotency_key: string
  failure_reason?: string
  created_at: string
  updated_at: string
  items?: OrderItem[]
}

export type OrderList = {
  list: Order[]
  total: number
  page: number
  page_size: number
}

export type CreateProductPayload = {
  name: string
  description: string
  price_fen: number
}

export type InitInventoryPayload = {
  product_id: number
  stock_quantity: number
}

export type AddInventoryPayload = {
  product_id: number
  quantity: number
}

export type CreateOrderPayload = {
  items: {
    product_id: number
    quantity: number
  }[]
}
