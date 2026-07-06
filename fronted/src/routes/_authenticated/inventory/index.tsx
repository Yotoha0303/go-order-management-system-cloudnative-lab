import { createFileRoute } from '@tanstack/react-router'
import { requireAdmin } from '@/features/auth/require-admin'
import { InventoryPage } from '@/features/order-inventory/inventory'

export const Route = createFileRoute('/_authenticated/inventory/')({
  beforeLoad: ({ context }) => requireAdmin(context.queryClient),
  component: InventoryPage,
})
