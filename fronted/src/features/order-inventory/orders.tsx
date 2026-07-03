import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { getErrorMessage, orderApi, productApi, queryKeys } from './api'
import {
  ApiErrorPanel,
  BusinessPage,
  EmptyRow,
  Field,
  LoadingRow,
  OrderStatusBadge,
} from './components'
import { formatDateTime, formatFen } from './format'
import {
  isOrderActionAllowed,
  requiresOrderActionConfirmation,
  type OrderAction,
} from './order-actions'
import {
  prepareOrderSubmission,
  type PendingOrderSubmission,
} from './order-submission'
import type { CreateOrderPayload, Order } from './types'

type DraftOrderItem = {
  product_id: string
  quantity: string
}

const actionText: Record<OrderAction, string> = {
  pay: '支付',
  finish: '完成',
  cancel: '取消',
}

const orderPageSize = 10

export function OrdersPage() {
  const queryClient = useQueryClient()
  const [items, setItems] = useState<DraftOrderItem[]>([
    { product_id: '', quantity: '1' },
  ])
  const [selectedOrderId, setSelectedOrderId] = useState<number | null>(null)
  const [page, setPage] = useState(1)
  const [pendingSubmission, setPendingSubmission] =
    useState<PendingOrderSubmission | null>(null)
  const [actionConfirmation, setActionConfirmation] = useState<{
    order: Order
    action: Exclude<OrderAction, 'pay'>
  } | null>(null)

  const ordersQuery = useQuery({
    queryKey: queryKeys.orders(page, orderPageSize),
    queryFn: () => orderApi.list(page, orderPageSize),
    placeholderData: (previousData) => previousData,
  })
  const productsQuery = useQuery({
    queryKey: queryKeys.products(1),
    queryFn: () => productApi.list(1),
  })

  const orderDetailQuery = useQuery({
    queryKey: queryKeys.order(selectedOrderId ?? 0),
    queryFn: () => orderApi.detail(selectedOrderId ?? 0),
    enabled: selectedOrderId !== null,
  })

  const createOrderMutation = useMutation({
    mutationFn: ({
      payload,
      idempotencyKey,
    }: {
      payload: CreateOrderPayload
      idempotencyKey: string
    }) => orderApi.create(payload, idempotencyKey),
    onSuccess: async (order) => {
      toast.success(`订单创建成功：${order.order_no}`)
      setItems([{ product_id: '', quantity: '1' }])
      setPendingSubmission(null)
      setSelectedOrderId(order.id)
      setPage(1)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ordersRoot }),
        queryClient.invalidateQueries({ queryKey: queryKeys.stockLogsRoot }),
      ])
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const orderActionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: OrderAction }) => {
      if (action === 'pay') return orderApi.pay(id)
      if (action === 'finish') return orderApi.finish(id)
      return orderApi.cancel(id)
    },
    onSuccess: async (_data, variables) => {
      toast.success(`订单${actionText[variables.action]}成功`)
      setActionConfirmation(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ordersRoot }),
        queryClient.invalidateQueries({ queryKey: queryKeys.stockLogsRoot }),
      ])
      if (selectedOrderId) {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.order(selectedOrderId),
        })
      }
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  function updateItem(index: number, patch: Partial<DraftOrderItem>) {
    setItems((current) =>
      current.map((item, itemIndex) =>
        itemIndex === index ? { ...item, ...patch } : item
      )
    )
  }

  function requestOrderAction(order: Order, action: OrderAction) {
    if (!requiresOrderActionConfirmation(action)) {
      orderActionMutation.mutate({ id: order.id, action })
      return
    }
    setActionConfirmation({ order, action })
  }

  function removeItem(index: number) {
    setItems((current) => current.filter((_, itemIndex) => itemIndex !== index))
  }

  function parseCreateOrderPayload(): CreateOrderPayload | null {
    const parsed = items.map((item) => ({
      product_id: Number(item.product_id),
      quantity: Number(item.quantity),
    }))

    const hasInvalidItem = parsed.some(
      (item) =>
        !Number.isInteger(item.product_id) ||
        item.product_id <= 0 ||
        !Number.isInteger(item.quantity) ||
        item.quantity <= 0
    )

    if (hasInvalidItem) {
      toast.error('请选择商品并填写正整数数量')
      return null
    }

    if (new Set(parsed.map((item) => item.product_id)).size !== parsed.length) {
      toast.error('同一商品请合并为一行')
      return null
    }

    return { items: parsed }
  }

  function handleCreateOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const payload = parseCreateOrderPayload()
    if (!payload) return
    const submission = prepareOrderSubmission(payload, pendingSubmission)
    setPendingSubmission(submission)
    createOrderMutation.mutate({
      payload,
      idempotencyKey: submission.idempotencyKey,
    })
  }

  const orders = ordersQuery.data?.orders ?? []
  const total = ordersQuery.data?.total ?? 0
  const totalPages = Math.ceil(total / orderPageSize)
  const orderDetail = orderDetailQuery.data
  const onSaleProducts = productsQuery.data?.products ?? []
  const estimatedAmountFen = items.reduce((total, item) => {
    const product = onSaleProducts.find(
      (candidate) => candidate.id === Number(item.product_id)
    )
    const quantity = Number(item.quantity)
    if (!product || !Number.isInteger(quantity) || quantity <= 0) return total
    return total + product.price_fen * quantity
  }, 0)

  return (
    <BusinessPage
      title='订单管理'
      description='覆盖创建订单、订单列表、详情、支付、完成和取消接口。'
    >
      <div className='grid gap-4 xl:grid-cols-[minmax(340px,460px)_1fr]'>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle>创建订单</CardTitle>
              <CardDescription>
                仅可选择已上架商品，最终金额和库存以服务端校验为准。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form className='space-y-4' onSubmit={handleCreateOrder}>
                <div className='space-y-3'>
                  {items.map((item, index) => (
                    <div
                      key={index}
                      className='grid grid-cols-[1fr_110px_auto] items-end gap-2'
                    >
                      <Field label={index === 0 ? '商品' : ' '}>
                        <Select
                          value={item.product_id}
                          onValueChange={(value) =>
                            updateItem(index, {
                              product_id: value,
                            })
                          }
                        >
                          <SelectTrigger>
                            <SelectValue placeholder='请选择商品' />
                          </SelectTrigger>
                          <SelectContent>
                            {onSaleProducts.map((product) => (
                              <SelectItem
                                key={product.id}
                                value={String(product.id)}
                              >
                                {product.name} · {formatFen(product.price_fen)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Field>
                      <Field label={index === 0 ? '数量' : ' '}>
                        <Input
                          value={item.quantity}
                          onChange={(event) =>
                            updateItem(index, { quantity: event.target.value })
                          }
                          inputMode='numeric'
                          placeholder='1'
                        />
                      </Field>
                      <Button
                        type='button'
                        size='icon'
                        variant='outline'
                        disabled={items.length === 1}
                        onClick={() => removeItem(index)}
                      >
                        <Trash2 />
                        <span className='sr-only'>删除商品行</span>
                      </Button>
                    </div>
                  ))}
                </div>
                <p className='text-end text-sm text-muted-foreground'>
                  预计金额：
                  <span className='ms-1 font-medium text-foreground'>
                    {formatFen(estimatedAmountFen)}
                  </span>
                </p>
                <div className='flex flex-col gap-2 sm:flex-row'>
                  <Button
                    type='button'
                    variant='outline'
                    className='flex-1'
                    onClick={() =>
                      setItems((current) => [
                        ...current,
                        { product_id: '', quantity: '1' },
                      ])
                    }
                  >
                    <Plus />
                    添加商品
                  </Button>
                  <Button
                    className='flex-1'
                    disabled={createOrderMutation.isPending}
                  >
                    提交订单
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>订单详情</CardTitle>
              <CardDescription>
                从订单列表选择后查看商品快照和状态。
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              {orderDetail && (
                <div className='space-y-4 rounded-md border p-4'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <p className='truncate font-medium'>
                        #{orderDetail.order.id} {orderDetail.order.order_no}
                      </p>
                      <p className='mt-1 text-sm text-muted-foreground'>
                        {formatFen(orderDetail.order.total_amount_fen)}
                      </p>
                    </div>
                    <OrderStatusBadge status={orderDetail.order.status} />
                  </div>

                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>商品</TableHead>
                        <TableHead>单价</TableHead>
                        <TableHead>数量</TableHead>
                        <TableHead>小计</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {orderDetail.items.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell className='max-w-[180px] whitespace-normal'>
                            #{item.product_id} {item.product_name}
                          </TableCell>
                          <TableCell>
                            {formatFen(item.product_price_fen)}
                          </TableCell>
                          <TableCell>{item.quantity}</TableCell>
                          <TableCell>{formatFen(item.subtotal_fen)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>

                  <OrderActions
                    order={orderDetail.order}
                    pendingAction={
                      orderActionMutation.isPending &&
                      orderActionMutation.variables?.id === orderDetail.order.id
                        ? orderActionMutation.variables.action
                        : undefined
                    }
                    onAction={(action) =>
                      requestOrderAction(orderDetail.order, action)
                    }
                  />
                </div>
              )}
              {!orderDetail && (
                <div className='rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground'>
                  {orderDetailQuery.isFetching
                    ? '正在加载订单详情…'
                    : '请从订单列表选择订单。'}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>订单列表</CardTitle>
            <CardDescription>
              按后端返回顺序展示订单，支持状态流转操作。
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-4'>
            <ApiErrorPanel error={ordersQuery.error} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>订单号</TableHead>
                  <TableHead>金额</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead className='text-end'>操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ordersQuery.isLoading && <LoadingRow colSpan={6} />}
                {!ordersQuery.isLoading &&
                  orders.map((order) => (
                    <TableRow key={order.id}>
                      <TableCell>#{order.id}</TableCell>
                      <TableCell className='font-medium'>
                        {order.order_no}
                      </TableCell>
                      <TableCell>{formatFen(order.total_amount_fen)}</TableCell>
                      <TableCell>
                        <OrderStatusBadge status={order.status} />
                      </TableCell>
                      <TableCell>{formatDateTime(order.created_at)}</TableCell>
                      <TableCell>
                        <div className='flex flex-wrap justify-end gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => {
                              setSelectedOrderId(order.id)
                            }}
                          >
                            详情
                          </Button>
                          <OrderActions
                            compact
                            order={order}
                            pendingAction={
                              orderActionMutation.isPending &&
                              orderActionMutation.variables?.id === order.id
                                ? orderActionMutation.variables.action
                                : undefined
                            }
                            onAction={(action) =>
                              requestOrderAction(order, action)
                            }
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                {!ordersQuery.isLoading && orders.length === 0 && (
                  <EmptyRow colSpan={6} message='暂无订单数据' />
                )}
              </TableBody>
            </Table>
            <div className='flex items-center justify-between gap-3'>
              <p className='text-sm text-muted-foreground'>
                共 {total} 条，第 {page} / {Math.max(totalPages, 1)} 页
              </p>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page === 1 || ordersQuery.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  上一页
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages || ordersQuery.isFetching}
                  onClick={() => setPage((current) => current + 1)}
                >
                  下一页
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
      <ConfirmDialog
        open={actionConfirmation !== null}
        onOpenChange={(open) => !open && setActionConfirmation(null)}
        title={
          actionConfirmation?.action === 'cancel'
            ? '确认取消订单'
            : '确认完成订单'
        }
        desc={
          actionConfirmation?.action === 'cancel'
            ? `取消订单 ${actionConfirmation.order.order_no} 后将回滚对应库存。`
            : `订单 ${actionConfirmation?.order.order_no ?? ''} 完成后不能再变更状态。`
        }
        cancelBtnText='返回'
        confirmText={
          actionConfirmation?.action === 'cancel' ? '确认取消' : '确认完成'
        }
        destructive={actionConfirmation?.action === 'cancel'}
        isLoading={orderActionMutation.isPending}
        handleConfirm={() => {
          if (!actionConfirmation) return
          orderActionMutation.mutate({
            id: actionConfirmation.order.id,
            action: actionConfirmation.action,
          })
        }}
      />
    </BusinessPage>
  )
}

function OrderActions({
  order,
  pendingAction,
  compact,
  onAction,
}: {
  order: Order
  pendingAction?: OrderAction
  compact?: boolean
  onAction: (action: OrderAction) => void
}) {
  const size = compact ? 'sm' : 'default'
  const pending = pendingAction !== undefined

  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        size={size}
        disabled={pending || !isOrderActionAllowed(order.status, 'pay')}
        onClick={() => onAction('pay')}
      >
        {pendingAction === 'pay' && <Loader2 className='animate-spin' />}
        支付
      </Button>
      <Button
        size={size}
        variant='outline'
        disabled={pending || !isOrderActionAllowed(order.status, 'finish')}
        onClick={() => onAction('finish')}
      >
        {pendingAction === 'finish' && <Loader2 className='animate-spin' />}
        完成
      </Button>
      <Button
        size={size}
        variant='outline'
        disabled={pending || !isOrderActionAllowed(order.status, 'cancel')}
        onClick={() => onAction('cancel')}
      >
        {pendingAction === 'cancel' && <Loader2 className='animate-spin' />}
        取消
      </Button>
    </div>
  )
}
