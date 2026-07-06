import { createFileRoute } from '@tanstack/react-router'
import { requireAdmin } from '@/features/auth/require-admin'
import { ProductsPage } from '@/features/order-inventory/products'

export const Route = createFileRoute('/_authenticated/products/')({
  beforeLoad: ({ context }) => requireAdmin(context.queryClient),
  component: ProductsPage,
})
