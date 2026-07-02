import { createFileRoute } from '@tanstack/react-router'
import { ProductsPage } from '@/features/order-inventory/products'

export const Route = createFileRoute('/_authenticated/products/')({
  component: ProductsPage,
})
