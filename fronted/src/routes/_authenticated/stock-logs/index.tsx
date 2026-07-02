import { createFileRoute } from '@tanstack/react-router'
import { StockLogsPage } from '@/features/order-inventory/stock-logs'

export const Route = createFileRoute('/_authenticated/stock-logs/')({
  component: StockLogsPage,
})
