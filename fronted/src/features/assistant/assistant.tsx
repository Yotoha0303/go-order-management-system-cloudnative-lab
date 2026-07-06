import { useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Bot, Loader2, Send, UserRound } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { BusinessPage } from '@/features/order-inventory/components'
import { formatDateTime } from '@/features/order-inventory/format'
import { assistantApi, getErrorMessage } from './api'
import type { AssistantChatResponse } from './types'

type ChatEntry = {
  id: number
  role: 'user' | 'assistant'
  content: string
  response?: AssistantChatResponse
  error?: boolean
}

const suggestions = ['查询库存低于 10 的商品', '统计最近 7 天订单状态']

const orderStatusLabels = {
  pending: '待支付',
  paid: '已支付',
  finished: '已完成',
  cancelled: '已取消',
} as const

export function AssistantPage() {
  const [message, setMessage] = useState('')
  const [entries, setEntries] = useState<ChatEntry[]>([])
  const nextID = useRef(0)

  const chatMutation = useMutation({
    mutationFn: assistantApi.chat,
    onSuccess: (response) => {
      setEntries((current) => [
        ...current,
        {
          id: ++nextID.current,
          role: 'assistant',
          content: response.answer,
          response,
        },
      ])
    },
    onError: (error) => {
      setEntries((current) => [
        ...current,
        {
          id: ++nextID.current,
          role: 'assistant',
          content: getErrorMessage(error),
          error: true,
        },
      ])
    },
  })

  const submitMessage = (rawMessage: string) => {
    const normalized = rawMessage.trim()
    if (!normalized || chatMutation.isPending) return

    setEntries((current) => [
      ...current,
      { id: ++nextID.current, role: 'user', content: normalized },
    ])
    setMessage('')
    chatMutation.mutate(normalized)
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    submitMessage(message)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submitMessage(message)
    }
  }

  return (
    <BusinessPage
      title='AI 运营助手'
      description='通过自然语言查询低库存商品和订单状态统计。'
    >
      <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]'>
        <Card className='min-h-[34rem]'>
          <CardHeader className='border-b'>
            <CardTitle className='flex items-center gap-2'>
              <Bot className='size-5' />
              对话查询
            </CardTitle>
            <CardDescription>
              助手只执行预先定义的只读查询，不会直接修改订单或库存。
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-1 flex-col gap-4'>
            <div
              className='flex min-h-72 flex-1 flex-col gap-4 overflow-y-auto rounded-lg bg-muted/30 p-4'
              aria-live='polite'
            >
              {entries.length === 0 && (
                <div className='m-auto max-w-md text-center text-sm text-muted-foreground'>
                  <Bot className='mx-auto mb-3 size-9' />
                  输入运营问题开始查询。当前支持低库存商品和订单状态统计。
                </div>
              )}
              {entries.map((entry) => (
                <ChatBubble key={entry.id} entry={entry} />
              ))}
              {chatMutation.isPending && (
                <div className='flex items-center gap-2 text-sm text-muted-foreground'>
                  <Loader2 className='size-4 animate-spin' />
                  正在分析并查询业务数据…
                </div>
              )}
            </div>

            <form className='space-y-3' onSubmit={handleSubmit}>
              <Textarea
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='例如：查询库存低于 5 的商品'
                maxLength={2000}
                rows={3}
                disabled={chatMutation.isPending}
                aria-label='发送给 AI 运营助手的消息'
              />
              <div className='flex items-center justify-between gap-3'>
                <span className='text-xs text-muted-foreground'>
                  Enter 发送，Shift + Enter 换行
                </span>
                <Button
                  type='submit'
                  disabled={!message.trim() || chatMutation.isPending}
                >
                  {chatMutation.isPending ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <Send />
                  )}
                  发送
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>

        <Card className='h-fit'>
          <CardHeader>
            <CardTitle className='text-base'>快捷问题</CardTitle>
            <CardDescription>点击后直接发送查询</CardDescription>
          </CardHeader>
          <CardContent className='grid gap-2'>
            {suggestions.map((suggestion) => (
              <Button
                key={suggestion}
                type='button'
                variant='outline'
                className='h-auto justify-start py-3 text-left whitespace-normal'
                disabled={chatMutation.isPending}
                onClick={() => submitMessage(suggestion)}
              >
                {suggestion}
              </Button>
            ))}
            <p className='mt-2 text-xs leading-5 text-muted-foreground'>
              页面仅对管理员开放。每次提问都会生成审计记录，但不会保存问题正文。
            </p>
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}

function ChatBubble({ entry }: { entry: ChatEntry }) {
  const isUser = entry.role === 'user'

  return (
    <div className={cn('flex gap-3', isUser && 'justify-end')}>
      {!isUser && (
        <div className='flex size-8 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground'>
          <Bot className='size-4' />
        </div>
      )}
      <div
        className={cn(
          'max-w-[85%] rounded-xl px-4 py-3 text-sm',
          isUser
            ? 'bg-primary text-primary-foreground'
            : 'border bg-background',
          entry.error && 'border-destructive text-destructive'
        )}
      >
        <p className='whitespace-pre-wrap'>{entry.content}</p>
        {entry.response && <AssistantResult response={entry.response} />}
      </div>
      {isUser && (
        <div className='flex size-8 shrink-0 items-center justify-center rounded-full border bg-background'>
          <UserRound className='size-4' />
        </div>
      )}
    </div>
  )
}

function AssistantResult({ response }: { response: AssistantChatResponse }) {
  if (response.intent === 'get_low_stock_products') {
    return (
      <div className='mt-3 space-y-2 border-t pt-3'>
        <div className='flex flex-wrap gap-2'>
          <Badge variant='secondary'>阈值 ≤ {response.data.threshold}</Badge>
          <Badge variant='outline'>{response.data.count} 个商品</Badge>
        </div>
        {response.data.items.map((item) => (
          <div
            key={item.product_id}
            className='flex items-center justify-between gap-4 rounded-md bg-muted/60 px-3 py-2'
          >
            <span className='min-w-0 truncate'>
              #{item.product_id} {item.name}
            </span>
            <span className='shrink-0 font-medium'>库存 {item.stock}</span>
          </div>
        ))}
        <RequestID value={response.request_id} />
      </div>
    )
  }

  return (
    <div className='mt-3 space-y-2 border-t pt-3'>
      <div className='flex flex-wrap gap-2'>
        <Badge variant='secondary'>最近 {response.data.days} 天</Badge>
        <Badge variant='outline'>共 {response.data.total} 单</Badge>
      </div>
      <p className='text-xs text-muted-foreground'>
        {formatDateTime(response.data.from)} 至{' '}
        {formatDateTime(response.data.to)}
      </p>
      <div className='grid grid-cols-2 gap-2'>
        {response.data.counts.map((item) => (
          <div key={item.status} className='rounded-md bg-muted/60 px-3 py-2'>
            <p className='text-xs text-muted-foreground'>
              {orderStatusLabels[item.status]}
            </p>
            <p className='mt-1 font-semibold'>{item.count}</p>
          </div>
        ))}
      </div>
      <RequestID value={response.request_id} />
    </div>
  )
}

function RequestID({ value }: { value: string }) {
  return (
    <p className='pt-1 text-[11px] break-all text-muted-foreground'>
      Request ID: {value}
    </p>
  )
}
