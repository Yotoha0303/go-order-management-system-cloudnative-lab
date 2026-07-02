import {
  api,
  BusinessApiError,
  getErrorMessage,
  rootApi,
  unwrap,
  type ApiResponse,
} from '@/lib/api-client'
import type {
  AddInventoryPayload,
  CreateOrderPayload,
  CreateProductPayload,
  InitInventoryPayload,
  Inventory,
  Order,
  OrderDetail,
  Product,
  StockLog,
} from './types'

export { BusinessApiError, getErrorMessage }

export const queryKeys = {
  health: ['health'] as const,
  products: ['products'] as const,
  product: (id: number) => ['products', id] as const,
  inventory: (productId: number) => ['inventory', productId] as const,
  stockLogsRoot: ['stock-logs'] as const,
  stockLogs: (productId?: number) =>
    ['stock-logs', { productId: productId ?? null }] as const,
  orders: ['orders'] as const,
  order: (id: number) => ['orders', id] as const,
}

export const healthApi = {
  ping: () =>
    unwrap<{ message: string }>(
      rootApi.get<ApiResponse<{ message: string }>>('/ping')
    ),
}

export const productApi = {
  list: () => unwrap<Product[]>(api.get<ApiResponse<Product[]>>('/products')),
  create: (payload: CreateProductPayload) =>
    unwrap<Product>(api.post<ApiResponse<Product>>('/products', payload)),
  detail: (id: number) =>
    unwrap<Product>(api.get<ApiResponse<Product>>(`/products/${id}`)),
  onSale: (id: number) =>
    unwrap<void>(api.patch<ApiResponse<void>>(`/products/${id}/on-sale`)),
  offSale: (id: number) =>
    unwrap<void>(api.patch<ApiResponse<void>>(`/products/${id}/off-sale`)),
}

export const inventoryApi = {
  init: (payload: InitInventoryPayload) =>
    unwrap<void>(api.post<ApiResponse<void>>('/inventory/init', payload)),
  add: (payload: AddInventoryPayload) =>
    unwrap<void>(api.post<ApiResponse<void>>('/inventory/add', payload)),
  detailByProductId: (productId: number) =>
    unwrap<Inventory>(
      api.get<ApiResponse<Inventory>>(`/inventory/products/${productId}`)
    ),
}

export const stockLogApi = {
  list: (productId?: number) =>
    unwrap<StockLog[]>(
      api.get<ApiResponse<StockLog[]>>('/stock-logs', {
        params: productId ? { product_id: productId } : undefined,
      })
    ),
}

export const orderApi = {
  create: (payload: CreateOrderPayload) =>
    unwrap<Order>(
      api.post<ApiResponse<Order>>('/orders', {
        ...payload,
        idempotency_key: crypto.randomUUID(),
      })
    ),
  list: () => unwrap<Order[]>(api.get<ApiResponse<Order[]>>('/orders')),
  detail: (id: number) =>
    unwrap<OrderDetail>(api.get<ApiResponse<OrderDetail>>(`/orders/${id}`)),
  pay: (id: number) =>
    unwrap<void>(api.patch<ApiResponse<void>>(`/orders/${id}/pay`)),
  finish: (id: number) =>
    unwrap<void>(api.patch<ApiResponse<void>>(`/orders/${id}/finish`)),
  cancel: (id: number) =>
    unwrap<void>(api.patch<ApiResponse<void>>(`/orders/${id}/cancel`)),
}
