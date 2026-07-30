import type { ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { TableCell, TableRow } from '@/components/ui/table'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { getErrorMessage } from './api'
import {
  ORDER_STATUS,
  PRODUCT_STATUS,
  STOCK_CHANGE_TYPE,
  orderStatusText,
  productStatusText,
  stockChangeTypeText,
} from './format'

type BusinessPageProps = {
  title: string
  description: string
  actions?: ReactNode
  children: ReactNode
}

export function BusinessPage({
  title,
  description,
  actions,
  children,
}: BusinessPageProps) {
  return (
    <>
      <Header>
        <div className='me-auto min-w-0'>
          <p className='truncate text-sm font-medium'>订单库存系统</p>
          <p className='truncate text-xs text-muted-foreground'>{title}</p>
        </div>
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>
      <Main>
        <div className='mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
          <div>
            <h1 className='text-2xl font-bold tracking-tight'>{title}</h1>
            <p className='mt-1 text-sm text-muted-foreground'>{description}</p>
          </div>
          {actions}
        </div>
        {children}
      </Main>
    </>
  )
}

export function Field({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className='grid gap-2 text-sm font-medium'>
      <span>{label}</span>
      {children}
    </label>
  )
}

export function LoadingRow({ colSpan }: { colSpan: number }) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan} className='h-24 text-center'>
        <span className='inline-flex items-center gap-2 text-muted-foreground'>
          <Loader2 className='size-4 animate-spin' />
          加载中
        </span>
      </TableCell>
    </TableRow>
  )
}

export function EmptyRow({
  colSpan,
  message,
}: {
  colSpan: number
  message: string
}) {
  return (
    <TableRow>
      <TableCell
        colSpan={colSpan}
        className='h-24 text-center text-muted-foreground'
      >
        {message}
      </TableCell>
    </TableRow>
  )
}

export function ApiErrorPanel({ error }: { error: unknown }) {
  if (!error) return null

  return (
    <Card className='border-destructive/40 bg-destructive/5'>
      <CardContent className='text-sm text-destructive'>
        {getErrorMessage(error)}
      </CardContent>
    </Card>
  )
}

export function ProductStatusBadge({ status }: { status: number }) {
  const onSale = status === PRODUCT_STATUS.ON_SALE

  return (
    <Badge variant={onSale ? 'default' : 'secondary'}>
      {productStatusText(status)}
    </Badge>
  )
}

export function OrderStatusBadge({ status }: { status: string | number }) {
  if (
    status === ORDER_STATUS.CANCELLED ||
    status === ORDER_STATUS.FAILED ||
    status === ORDER_STATUS.RECONCILIATION_REQUIRED
  ) {
    return <Badge variant='destructive'>{orderStatusText(status)}</Badge>
  }

  if (status === ORDER_STATUS.FINISHED) {
    return <Badge variant='secondary'>{orderStatusText(status)}</Badge>
  }

  if (status === ORDER_STATUS.PAID) {
    return <Badge>{orderStatusText(status)}</Badge>
  }

  return <Badge variant='outline'>{orderStatusText(status)}</Badge>
}

export function StockBizTypeBadge({ type }: { type: string | number }) {
  if (
    type === STOCK_CHANGE_TYPE.RESERVE ||
    type === STOCK_CHANGE_TYPE.CONFIRM
  ) {
    return <Badge variant='destructive'>{stockChangeTypeText(type)}</Badge>
  }

  if (type === STOCK_CHANGE_TYPE.RELEASE) {
    return <Badge variant='secondary'>{stockChangeTypeText(type)}</Badge>
  }

  if (type === STOCK_CHANGE_TYPE.ADD) {
    return <Badge>{stockChangeTypeText(type)}</Badge>
  }

  return <Badge variant='outline'>{stockChangeTypeText(type)}</Badge>
}
