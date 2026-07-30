import axios, { AxiosError, type AxiosResponse } from 'axios'
import { useAuthStore } from '@/stores/auth-store'

/**
 * The backend does not speak one envelope. identity-service returns `message`,
 * while catalog, inventory and order-service return `msg`, and the gateway
 * returns `message` alongside a string `code`. Both message keys are optional
 * here so callers have to go through `apiMessage` rather than picking one and
 * silently getting undefined from the other half of the services.
 */
export type ApiResponse<T> = {
  code: number | string
  message?: string
  msg?: string
  data?: T
}

/** Reads whichever message key the responding service used. */
export function apiMessage(payload: unknown): string | undefined {
  if (!payload || typeof payload !== 'object') return undefined
  const body = payload as { message?: unknown; msg?: unknown }
  if (typeof body.message === 'string' && body.message) return body.message
  if (typeof body.msg === 'string' && body.msg) return body.msg
  return undefined
}

const apiBaseURL =
  (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(
    /\/$/,
    ''
  ) || '/api/v1'
const rootBaseURL = apiBaseURL.replace(/\/api\/v1$/, '') || '/'

export const api = axios.create({ baseURL: apiBaseURL, timeout: 10000 })
export const rootApi = axios.create({ baseURL: rootBaseURL, timeout: 10000 })

let unauthorizedHandler: (() => void) | undefined

export function setUnauthorizedHandler(handler?: () => void) {
  unauthorizedHandler = handler
}

api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().auth.accessToken
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (
      error instanceof AxiosError &&
      error.response?.status === 401 &&
      useAuthStore.getState().auth.accessToken
    ) {
      useAuthStore.getState().auth.reset()
      unauthorizedHandler?.()
    }
    return Promise.reject(error)
  }
)

export class BusinessApiError extends Error {
  /** Numeric for the business services, a string for gateway-level failures. */
  code: number | string

  constructor(message: string, code: number | string) {
    super(message)
    this.name = 'BusinessApiError'
    this.code = code
  }
}

export async function unwrap<T>(
  promise: Promise<AxiosResponse<ApiResponse<T>>>
): Promise<T> {
  const { data } = await promise
  if (data.code !== 0) {
    throw new BusinessApiError(apiMessage(data) ?? '请求失败', data.code)
  }
  return data.data as T
}

export function getErrorMessage(error: unknown) {
  if (error instanceof BusinessApiError) return error.message
  if (error instanceof AxiosError) {
    const data = error.response?.data
    const message = apiMessage(data)
    if (message) return message
    if (data && typeof data === 'object') {
      if ('title' in data && typeof data.title === 'string') return data.title
    }
    if (error.message) return error.message
  }
  if (error instanceof Error && error.message) return error.message
  return '请求失败，请稍后重试'
}
