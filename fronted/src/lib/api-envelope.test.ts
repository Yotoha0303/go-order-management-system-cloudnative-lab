import { AxiosError, AxiosHeaders, type AxiosResponse } from 'axios'
import { describe, expect, it } from 'vitest'
import {
  BusinessApiError,
  apiMessage,
  getErrorMessage,
  unwrap,
  type ApiResponse,
} from './api-client'

/**
 * The backend answers with two different message keys. identity-service uses
 * `message`; catalog, inventory and order-service use `msg`. Reading only
 * `message` made every business error from those three services fall back to a
 * generic string, hiding the real reason from the user.
 */
describe('apiMessage', () => {
  it('reads the message key identity-service uses', () => {
    expect(apiMessage({ code: 40001, message: '用户名或密码错误' })).toBe(
      '用户名或密码错误'
    )
  })

  it('reads the msg key catalog, inventory and order-service use', () => {
    expect(apiMessage({ code: 40903, msg: 'insufficient inventory' })).toBe(
      'insufficient inventory'
    )
  })

  it('prefers message when a response somehow carries both', () => {
    expect(apiMessage({ code: 1, message: 'from message', msg: 'from msg' })).toBe(
      'from message'
    )
  })

  it('ignores empty and non-string values instead of returning them', () => {
    expect(apiMessage({ code: 1, message: '', msg: '' })).toBeUndefined()
    expect(apiMessage({ code: 1, message: 123, msg: null })).toBeUndefined()
    expect(apiMessage(undefined)).toBeUndefined()
    expect(apiMessage('a string body')).toBeUndefined()
  })
})

function response<T>(body: ApiResponse<T>): AxiosResponse<ApiResponse<T>> {
  const config = { headers: new AxiosHeaders() }
  return {
    data: body,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  } as AxiosResponse<ApiResponse<T>>
}

describe('unwrap', () => {
  it('returns the payload when the service reports success', async () => {
    await expect(
      unwrap(Promise.resolve(response({ code: 0, msg: 'success', data: { id: 7 } })))
    ).resolves.toEqual({ id: 7 })
  })

  it('surfaces the reason from a msg envelope rather than a generic fallback', async () => {
    await expect(
      unwrap(Promise.resolve(response({ code: 40903, msg: 'insufficient inventory' })))
    ).rejects.toThrow('insufficient inventory')
  })

  it('surfaces the reason from a message envelope', async () => {
    await expect(
      unwrap(Promise.resolve(response({ code: 40001, message: '订单不存在' })))
    ).rejects.toThrow('订单不存在')
  })

  it('falls back only when neither key carries anything', async () => {
    await expect(
      unwrap(Promise.resolve(response({ code: 50001 })))
    ).rejects.toThrow('请求失败')
  })

  it('keeps the string codes the gateway returns', async () => {
    await expect(
      unwrap(
        Promise.resolve(
          response({ code: 'upstream_unavailable', message: '服务暂不可用' })
        )
      )
    ).rejects.toMatchObject({ code: 'upstream_unavailable' })
  })
})

describe('getErrorMessage', () => {
  it('reads msg out of an axios error body', () => {
    const config = { headers: new AxiosHeaders() }
    const error = new AxiosError('Request failed', 'ERR_BAD_REQUEST', config, undefined, {
      data: { code: 40903, msg: 'insufficient inventory' },
      status: 409,
      statusText: 'Conflict',
      headers: new AxiosHeaders(),
      config,
    } as AxiosResponse)

    expect(getErrorMessage(error)).toBe('insufficient inventory')
  })

  it('passes a BusinessApiError message through untouched', () => {
    expect(getErrorMessage(new BusinessApiError('库存不足', 40903))).toBe('库存不足')
  })
})
