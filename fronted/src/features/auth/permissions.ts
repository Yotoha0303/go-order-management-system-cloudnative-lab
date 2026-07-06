import type { AuthUser } from './api'

export const ADMIN_ROLE = 'admin'

export function isAdminUser(user: AuthUser | null | undefined): boolean {
  return Array.isArray(user?.roles) && user.roles.includes(ADMIN_ROLE)
}
