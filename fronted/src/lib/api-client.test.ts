import { AxiosError, AxiosHeaders } from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import { api, setUnauthorizedHandler } from './api-client'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
  setUnauthorizedHandler(undefined)
  useAuthStore.getState().auth.reset()
})

describe('api unauthorized handling', () => {
  it('clears the session and invokes the registered handler on 401', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    useAuthStore.getState().auth.setAccessToken('expired-token')
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'alice',
      nickname: 'Alice',
      status: 1,
    })
    api.defaults.adapter = async (config) => {
      throw new AxiosError(
        'Unauthorized',
        'ERR_BAD_REQUEST',
        config,
        undefined,
        {
          data: { code: 7004, message: '访问令牌已过期' },
          status: 401,
          statusText: 'Unauthorized',
          headers: new AxiosHeaders(),
          config,
        }
      )
    }

    await expect(api.get('/protected')).rejects.toBeInstanceOf(AxiosError)

    expect(useAuthStore.getState().auth.accessToken).toBe('')
    expect(useAuthStore.getState().auth.user).toBeNull()
    expect(onUnauthorized).toHaveBeenCalledOnce()
  })
})
