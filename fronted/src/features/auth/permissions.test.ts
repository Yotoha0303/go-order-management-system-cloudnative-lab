import { describe, expect, it } from 'vitest'
import type { AuthUser } from './api'
import { isAdminUser } from './permissions'

const user = (roles: string[]): AuthUser => ({
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  status: 1,
  roles,
})

describe('isAdminUser', () => {
  it('accepts an admin user', () => {
    expect(isAdminUser(user(['admin']))).toBe(true)
  })

  it('rejects a regular or missing user', () => {
    expect(isAdminUser(user(['user']))).toBe(false)
    expect(isAdminUser(null)).toBe(false)
  })
})
