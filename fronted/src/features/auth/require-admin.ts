import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { currentUserQueryOptions } from './api'
import { isAdminUser } from './permissions'

export async function requireAdmin(queryClient: QueryClient): Promise<void> {
  const user = await queryClient.fetchQuery({
    ...currentUserQueryOptions(),
    staleTime: 0,
  })
  useAuthStore.getState().auth.setUser(user)

  if (!isAdminUser(user)) {
    throw redirect({ to: '/403' })
  }
}
