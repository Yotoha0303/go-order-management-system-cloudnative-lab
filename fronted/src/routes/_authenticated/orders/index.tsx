import { createFileRoute } from '@tanstack/react-router'
import { OrdersPage } from '@/features/order-inventory/orders'

export const Route = createFileRoute('/_authenticated/orders/')({
  component: OrdersPage,
})
