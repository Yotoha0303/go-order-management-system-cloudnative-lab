export type ApiResponse<T> = {
  code: number
  message: string
  data?: T
}

export type Product = {
  id: number
  name: string
  description: string
  price_fen: number
  status: number
  created_at: string
  updated_at: string
}

export type Inventory = {
  id: number
  product_id: number
  stock_quantity: number
  created_at: string
  updated_at: string
}

export type StockLog = {
  id: number
  product_id: number
  change_quantity: number
  before_quantity: number
  after_quantity: number
  biz_type: number
  biz_id?: number | null
  remark: string
  created_at: string
}

export type Order = {
  id: number
  order_no: string
  total_amount_fen: number
  status: number
  paid_at?: string | null
  completed_at?: string | null
  cancelled_at?: string | null
  created_at: string
  updated_at: string
}

export type OrderItem = {
  id: number
  order_id: number
  product_id: number
  product_name: string
  product_price_fen: number
  quantity: number
  subtotal_fen: number
  created_at: string
}

export type OrderDetail = {
  order: Order
  items: OrderItem[]
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
