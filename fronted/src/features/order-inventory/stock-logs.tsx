import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { productApi, queryKeys, stockLogApi } from './api'
import {
  ApiErrorPanel,
  BusinessPage,
  EmptyRow,
  LoadingRow,
  StockBizTypeBadge,
} from './components'
import { formatDateTime } from './format'

export function StockLogsPage() {
  const [activeProductId, setActiveProductId] = useState<number | undefined>()

  const productsQuery = useQuery({
    queryKey: queryKeys.products('all'),
    queryFn: () => productApi.list('all'),
  })

  const stockLogsQuery = useQuery({
    queryKey: queryKeys.stockLogs(activeProductId),
    queryFn: () => stockLogApi.list(activeProductId),
  })

  const stockLogs = stockLogsQuery.data ?? []

  return (
    <BusinessPage
      title='库存流水'
      description='查看初始化、手动入库、下单扣减和取消回滚产生的库存流水。'
    >
      <Card>
        <CardHeader>
          <CardTitle>流水列表</CardTitle>
          <CardDescription>不输入商品 ID 时查询全部库存流水。</CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <Select
            value={activeProductId ? String(activeProductId) : 'all'}
            onValueChange={(value) =>
              setActiveProductId(value === 'all' ? undefined : Number(value))
            }
          >
            <SelectTrigger
              className='sm:max-w-sm'
              disabled={productsQuery.isPending}
            >
              <SelectValue placeholder='按商品筛选' />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>全部商品</SelectItem>
              {(productsQuery.data?.products ?? []).map((product) => (
                <SelectItem key={product.id} value={String(product.id)}>
                  #{product.id} {product.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <ApiErrorPanel error={stockLogsQuery.error} />
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>商品 ID</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>变化</TableHead>
                <TableHead>变更前</TableHead>
                <TableHead>变更后</TableHead>
                <TableHead>业务 ID</TableHead>
                <TableHead>备注</TableHead>
                <TableHead>时间</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {stockLogsQuery.isLoading && <LoadingRow colSpan={9} />}
              {!stockLogsQuery.isLoading &&
                stockLogs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>#{log.id}</TableCell>
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
                    <TableCell>{log.before_quantity}</TableCell>
                    <TableCell>{log.after_quantity}</TableCell>
                    <TableCell>{log.biz_id ? `#${log.biz_id}` : '-'}</TableCell>
                    <TableCell className='max-w-[280px] whitespace-normal'>
                      {log.remark || '-'}
                    </TableCell>
                    <TableCell>{formatDateTime(log.created_at)}</TableCell>
                  </TableRow>
                ))}
              {!stockLogsQuery.isLoading && stockLogs.length === 0 && (
                <EmptyRow colSpan={9} message='暂无库存流水' />
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </BusinessPage>
  )
}
