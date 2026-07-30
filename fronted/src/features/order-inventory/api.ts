import {
  api,
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
  OrderList,
  Product,
  ProductList,
  ProductListStatus,
  StockLogList,
} from './types'

export { getErrorMessage }

export const queryKeys = {
  health: ['health'] as const,
  productsRoot: ['products'] as const,
  // null rather than a status value: an absent status means every status, so
  // defaulting it to 2 here would collide with an explicit off-sale query.
  products: (status?: ProductListStatus, page = 1, pageSize = 100) =>
    ['products', { status: status ?? null, page, pageSize }] as const,
  product: (id: number) => ['products', id] as const,
  inventory: (productId: number) => ['inventory', productId] as const,
  stockLogsRoot: ['stock-logs'] as const,
  stockLogs: (productId?: number) =>
    ['stock-logs', { productId: productId ?? null }] as const,
  ordersRoot: ['orders'] as const,
  orders: (page: number, pageSize: number) =>
    ['orders', { page, pageSize }] as const,
  order: (id: number) => ['orders', id] as const,
}

export const healthApi = {
  // Gateway /ping returns a plain body {"message":"success"} without the
  // business envelope {code,msg,data} that unwrap() expects.
  ping: async () => {
    const { data, status } = await rootApi.get<{ message?: string }>('/ping')
    if (status >= 200 && status < 300) {
      return { message: data?.message ?? 'success' }
    }
    throw new Error('health check failed')
  },
}

export const productApi = {
  list: (status?: ProductListStatus, page = 1, pageSize = 100) =>
    unwrap<ProductList>(
      api.get<ApiResponse<ProductList>>('/products', {
        params: {
          status,
          page,
          page_size: pageSize,
        },
      })
    ),
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
  list: (productId?: number, page = 1, pageSize = 100) =>
    unwrap<StockLogList>(
      api.get<ApiResponse<StockLogList>>('/stock-logs', {
        params: {
          ...(productId ? { product_id: productId } : {}),
          page,
          page_size: pageSize,
        },
      })
    ),
}

export const orderApi = {
  create: (payload: CreateOrderPayload, idempotencyKey: string) =>
    unwrap<Order>(
      api.post<ApiResponse<Order>>('/orders', {
        ...payload,
        idempotency_key: idempotencyKey,
      })
    ),
  list: (page: number, pageSize: number) =>
    unwrap<OrderList>(
      api.get<ApiResponse<OrderList>>('/orders', {
        params: { page, page_size: pageSize },
      })
    ),
  detail: (id: number) =>
    unwrap<Order>(api.get<ApiResponse<Order>>(`/orders/${id}`)),
  pay: (id: number) =>
    unwrap<Order>(api.patch<ApiResponse<Order>>(`/orders/${id}/pay`)),
  finish: (id: number) =>
    unwrap<Order>(api.patch<ApiResponse<Order>>(`/orders/${id}/finish`)),
  cancel: (id: number) =>
    unwrap<Order>(api.patch<ApiResponse<Order>>(`/orders/${id}/cancel`)),
}
