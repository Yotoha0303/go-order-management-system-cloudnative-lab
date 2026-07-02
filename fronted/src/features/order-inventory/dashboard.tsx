import type { ElementType } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  ClipboardList,
  History,
  Package,
  ShoppingCart,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  BusinessPage,
  EmptyRow,
  OrderStatusBadge,
  StockBizTypeBadge,
} from './components'
import { formatDateTime, formatFen, ORDER_STATUS } from './format'
import { healthApi, orderApi, productApi, queryKeys, stockLogApi } from './api'

export function OrderInventoryDashboard() {
  const healthQuery = useQuery({
    queryKey: queryKeys.health,
    queryFn: healthApi.ping,
    retry: false,
  })
  const productsQuery = useQuery({
    queryKey: queryKeys.products,
    queryFn: productApi.list,
  })
  const ordersQuery = useQuery({
    queryKey: queryKeys.orders,
    queryFn: orderApi.list,
  })
  const stockLogsQuery = useQuery({
    queryKey: queryKeys.stockLogs(),
    queryFn: () => stockLogApi.list(),
  })

  const products = productsQuery.data ?? []
  const orders = ordersQuery.data ?? []
  const stockLogs = stockLogsQuery.data ?? []
  const pendingOrders = orders.filter(
    (order) => order.status === ORDER_STATUS.PENDING
  )
  const paidOrders = orders.filter(
    (order) =>
      order.status === ORDER_STATUS.PAID ||
      order.status === ORDER_STATUS.FINISHED
  )
  const paidAmountFen = paidOrders.reduce(
    (total, order) => total + order.total_amount_fen,
    0
  )

  return (
    <BusinessPage
      title='业务仪表盘'
      description='汇总商品、库存流水和订单状态，快速判断系统是否可用。'
    >
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          title='后端健康'
          value={healthQuery.isSuccess ? '正常' : '待确认'}
          description={
            healthQuery.isError
              ? '请确认 Go 服务是否运行在 8082'
              : '来自 /ping 接口'
          }
          icon={Activity}
        />
        <MetricCard
          title='待上架商品'
          value={products.length.toString()}
          description='当前商品列表接口返回下架商品'
          icon={Package}
        />
        <MetricCard
          title='待支付订单'
          value={pendingOrders.length.toString()}
          description={`订单总数 ${orders.length}`}
          icon={ShoppingCart}
        />
        <MetricCard
          title='已支付金额'
          value={formatFen(paidAmountFen)}
          description='统计已支付和已完成订单'
          icon={ClipboardList}
        />
      </div>

      <div className='mt-4 grid gap-4 xl:grid-cols-7'>
        <Card className='xl:col-span-4'>
          <CardHeader>
            <CardTitle>最近订单</CardTitle>
            <CardDescription>按后端订单列表返回顺序展示前 5 条</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>订单号</TableHead>
                  <TableHead>金额</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>创建时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {orders.slice(0, 5).map((order) => (
                  <TableRow key={order.id}>
                    <TableCell className='font-medium'>{order.order_no}</TableCell>
                    <TableCell>{formatFen(order.total_amount_fen)}</TableCell>
                    <TableCell>
                      <OrderStatusBadge status={order.status} />
                    </TableCell>
                    <TableCell>{formatDateTime(order.created_at)}</TableCell>
                  </TableRow>
                ))}
                {orders.length === 0 && (
                  <EmptyRow colSpan={4} message='暂无订单数据' />
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card className='xl:col-span-3'>
          <CardHeader>
            <CardTitle className='flex items-center gap-2'>
              <History className='size-4' />
              最近库存流水
            </CardTitle>
            <CardDescription>用于确认入库、扣减和回滚是否生效</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>商品</TableHead>
                  <TableHead>类型</TableHead>
                  <TableHead>变化</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {stockLogs.slice(0, 6).map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>#{log.product_id}</TableCell>
                    <TableCell>
                      <StockBizTypeBadge type={log.biz_type} />
                    </TableCell>
                    <TableCell
                      className={
                        log.change_quantity < 0
                          ? 'text-destructive'
                          : 'text-emerald-600 dark:text-emerald-400'
                      }
                    >
                      {log.change_quantity > 0 ? '+' : ''}
                      {log.change_quantity}
                    </TableCell>
                  </TableRow>
                ))}
                {stockLogs.length === 0 && (
                  <EmptyRow colSpan={3} message='暂无库存流水' />
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}

type MetricCardProps = {
  title: string
  value: string
  description: string
  icon: ElementType
}

function MetricCard({ title, value, description, icon: Icon }: MetricCardProps) {
  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 pb-2'>
        <CardTitle className='text-sm font-medium'>{title}</CardTitle>
        <Icon className='size-4 text-muted-foreground' />
      </CardHeader>
      <CardContent>
        <div className='text-2xl font-bold'>{value}</div>
        <p className='mt-1 text-xs text-muted-foreground'>{description}</p>
      </CardContent>
    </Card>
  )
}
