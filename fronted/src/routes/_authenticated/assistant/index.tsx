import { createFileRoute } from '@tanstack/react-router'
import { AssistantPage } from '@/features/assistant/assistant'
import { requireAdmin } from '@/features/auth/require-admin'

export const Route = createFileRoute('/_authenticated/assistant/')({
  beforeLoad: ({ context }) => requireAdmin(context.queryClient),
  component: AssistantPage,
})
