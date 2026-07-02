import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Search, Trash2 } from 'lucide-react'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { CreateOrderPayload, Order } from './types'
import { formatDateTime, formatFen, ORDER_STATUS } from './format'
import { getErrorMessage, orderApi, queryKeys } from './api'
import {
  ApiErrorPanel,
  BusinessPage,
  EmptyRow,
  Field,
  LoadingRow,
  OrderStatusBadge,
} from './components'

type DraftOrderItem = {
  product_id: string
  quantity: string
}

type OrderAction = 'pay' | 'finish' | 'cancel'

const actionText: Record<OrderAction, string> = {
  pay: '支付',
  finish: '完成',
  cancel: '取消',
}

export function OrdersPage() {
  const queryClient = useQueryClient()
  const [items, setItems] = useState<DraftOrderItem[]>([
    { product_id: '', quantity: '1' },
  ])
  const [detailOrderId, setDetailOrderId] = useState('')
  const [selectedOrderId, setSelectedOrderId] = useState<number | null>(null)

  const ordersQuery = useQuery({
    queryKey: queryKeys.orders,
    queryFn: orderApi.list,
  })

  const orderDetailQuery = useQuery({
    queryKey: queryKeys.order(selectedOrderId ?? 0),
    queryFn: () => orderApi.detail(selectedOrderId ?? 0),
    enabled: selectedOrderId !== null,
  })

  const createOrderMutation = useMutation({
    mutationFn: orderApi.create,
    onSuccess: async (order) => {
      toast.success(`订单创建成功：${order.order_no}`)
      setItems([{ product_id: '', quantity: '1' }])
      setSelectedOrderId(order.id)
      setDetailOrderId(String(order.id))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.orders }),
        queryClient.invalidateQueries({ queryKey: queryKeys.stockLogsRoot }),
      ])
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const orderActionMutation = useMutation({
    mutationFn: async ({
      id,
      action,
    }: {
      id: number
      action: OrderAction
    }) => {
      if (action === 'pay') return orderApi.pay(id)
      if (action === 'finish') return orderApi.finish(id)
      return orderApi.cancel(id)
    },
    onSuccess: async (_data, variables) => {
      toast.success(`订单${actionText[variables.action]}成功`)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.orders }),
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
      toast.error('商品 ID 和数量都必须是大于 0 的整数')
      return null
    }

    return { items: parsed }
  }

  function handleCreateOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const payload = parseCreateOrderPayload()
    if (!payload) return
    createOrderMutation.mutate(payload)
  }

  function handleSearchOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const id = Number(detailOrderId)
    if (!Number.isInteger(id) || id <= 0) {
      toast.error('请输入有效订单 ID')
      return
    }
    setSelectedOrderId(id)
  }

  const orders = ordersQuery.data ?? []
  const orderDetail = orderDetailQuery.data

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
                商品必须已上架且库存充足；商品 ID 可从商品详情查询获得。
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
                      <Field label={index === 0 ? '商品 ID' : ' '}>
                        <Input
                          value={item.product_id}
                          onChange={(event) =>
                            updateItem(index, {
                              product_id: event.target.value,
                            })
                          }
                          inputMode='numeric'
                          placeholder='1'
                        />
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
              <CardDescription>按订单 ID 查询明细和订单项。</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <form className='flex gap-2' onSubmit={handleSearchOrder}>
                <Input
                  value={detailOrderId}
                  onChange={(event) => setDetailOrderId(event.target.value)}
                  inputMode='numeric'
                  placeholder='订单 ID'
                />
                <Button type='submit' size='icon' disabled={orderDetailQuery.isFetching}>
                  <Search />
                  <span className='sr-only'>查询订单</span>
                </Button>
              </form>

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
                          <TableCell>{formatFen(item.product_price_fen)}</TableCell>
                          <TableCell>{item.quantity}</TableCell>
                          <TableCell>{formatFen(item.subtotal_fen)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>

                  <OrderActions
                    order={orderDetail.order}
                    pending={orderActionMutation.isPending}
                    onAction={(action) =>
                      orderActionMutation.mutate({
                        id: orderDetail.order.id,
                        action,
                      })
                    }
                  />
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>订单列表</CardTitle>
            <CardDescription>按后端返回顺序展示订单，支持状态流转操作。</CardDescription>
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
                      <TableCell className='font-medium'>{order.order_no}</TableCell>
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
                              setDetailOrderId(String(order.id))
                            }}
                          >
                            详情
                          </Button>
                          <OrderActions
                            compact
                            order={order}
                            pending={orderActionMutation.isPending}
                            onAction={(action) =>
                              orderActionMutation.mutate({
                                id: order.id,
                                action,
                              })
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
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}

function OrderActions({
  order,
  pending,
  compact,
  onAction,
}: {
  order: Order
  pending: boolean
  compact?: boolean
  onAction: (action: OrderAction) => void
}) {
  const size = compact ? 'sm' : 'default'

  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        size={size}
        disabled={pending || order.status !== ORDER_STATUS.PENDING}
        onClick={() => onAction('pay')}
      >
        支付
      </Button>
      <Button
        size={size}
        variant='outline'
        disabled={pending || order.status !== ORDER_STATUS.PAID}
        onClick={() => onAction('finish')}
      >
        完成
      </Button>
      <Button
        size={size}
        variant='outline'
        disabled={pending || order.status !== ORDER_STATUS.PENDING}
        onClick={() => onAction('cancel')}
      >
        取消
      </Button>
    </div>
  )
}
