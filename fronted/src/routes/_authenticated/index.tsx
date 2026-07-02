import { createFileRoute } from '@tanstack/react-router'
import { OrderInventoryDashboard } from '@/features/order-inventory/dashboard'

export const Route = createFileRoute('/_authenticated/')({
  component: OrderInventoryDashboard,
})
