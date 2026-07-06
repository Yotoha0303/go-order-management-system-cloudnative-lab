import {
  api,
  getErrorMessage,
  unwrap,
  type ApiResponse,
} from '@/lib/api-client'
import type { AssistantChatResponse } from './types'

export { getErrorMessage }

export const assistantApi = {
  chat: (message: string) =>
    unwrap<AssistantChatResponse>(
      api.post<ApiResponse<AssistantChatResponse>>('/admin/assistant/chat', {
        message,
      })
    ),
}
