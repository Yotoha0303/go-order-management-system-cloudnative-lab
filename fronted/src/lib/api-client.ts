import axios, { AxiosError, type AxiosResponse } from 'axios'
import { useAuthStore } from '@/stores/auth-store'

export type ApiResponse<T> = {
  code: number
  message: string
  data?: T
}

const apiBaseURL =
  (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(
    /\/$/,
    ''
  ) || '/api/v1'
const rootBaseURL = apiBaseURL.replace(/\/api\/v1$/, '') || '/'

export const api = axios.create({ baseURL: apiBaseURL, timeout: 10000 })
export const rootApi = axios.create({ baseURL: rootBaseURL, timeout: 10000 })

api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().auth.accessToken
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export class BusinessApiError extends Error {
  code: number

  constructor(message: string, code: number) {
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
    throw new BusinessApiError(data.message || '请求失败', data.code)
  }
  return data.data as T
}

export function getErrorMessage(error: unknown) {
  if (error instanceof BusinessApiError) return error.message
  if (error instanceof AxiosError) {
    const data = error.response?.data
    if (data && typeof data === 'object') {
      if ('message' in data && typeof data.message === 'string') {
        return data.message
      }
      if ('title' in data && typeof data.title === 'string') return data.title
    }
    if (error.message) return error.message
  }
  if (error instanceof Error && error.message) return error.message
  return '请求失败，请稍后重试'
}
