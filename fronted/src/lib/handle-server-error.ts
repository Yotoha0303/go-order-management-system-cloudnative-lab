import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { apiMessage } from '@/lib/api-client'

export function handleServerError(error: unknown) {
  if (import.meta.env.DEV) {
    // eslint-disable-next-line no-console
    console.log(error)
  }

  let errMsg = 'Something went wrong!'

  if (
    error &&
    typeof error === 'object' &&
    'status' in error &&
    Number(error.status) === 204
  ) {
    errMsg = 'No content.'
  }

  if (error instanceof AxiosError) {
    const data = error.response?.data

    const message = apiMessage(data)

    if (message?.trim()) {
      errMsg = message
    } else if (
      data &&
      typeof data === 'object' &&
      'title' in data &&
      typeof data.title === 'string' &&
      data.title.trim()
    ) {
      errMsg = data.title
    }
  }

  toast.error(errMsg)
}
