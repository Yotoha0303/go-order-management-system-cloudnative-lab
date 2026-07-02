import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import type { Inventory } from './types'
import { formatDateTime } from './format'
import { getErrorMessage, inventoryApi, queryKeys } from './api'
import { BusinessPage, Field } from './components'

export function InventoryPage() {
  const queryClient = useQueryClient()
  const [initProductId, setInitProductId] = useState('')
  const [initQuantity, setInitQuantity] = useState('')
  const [addProductId, setAddProductId] = useState('')
  const [addQuantity, setAddQuantity] = useState('')
  const [lookupProductId, setLookupProductId] = useState('')
  const [inventory, setInventory] = useState<Inventory | null>(null)

  const lookupMutation = useMutation({
    mutationFn: inventoryApi.detailByProductId,
    onSuccess: (data) => {
      setInventory(data)
      toast.success('库存信息已更新')
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const initMutation = useMutation({
    mutationFn: inventoryApi.init,
    onSuccess: async (_data, variables) => {
      toast.success('库存初始化成功')
      setInitQuantity('')
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.inventory(variables.product_id),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.stockLogsRoot }),
      ])
      lookupMutation.mutate(variables.product_id)
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const addMutation = useMutation({
    mutationFn: inventoryApi.add,
    onSuccess: async (_data, variables) => {
      toast.success('库存增加成功')
      setAddQuantity('')
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.inventory(variables.product_id),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.stockLogsRoot }),
      ])
      lookupMutation.mutate(variables.product_id)
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  function parsePositiveInteger(value: string, label: string) {
    const number = Number(value)
    if (!Number.isInteger(number) || number <= 0) {
      toast.error(`${label} 必须是大于 0 的整数`)
      return null
    }
    return number
  }

  function handleInitInventory(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const productId = parsePositiveInteger(initProductId, '商品 ID')
    const stockQuantity = Number(initQuantity)

    if (productId === null) return
    if (!Number.isInteger(stockQuantity) || stockQuantity < 0) {
      toast.error('初始库存必须是大于等于 0 的整数')
      return
    }

    initMutation.mutate({
      product_id: productId,
      stock_quantity: stockQuantity,
    })
  }

  function handleAddInventory(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const productId = parsePositiveInteger(addProductId, '商品 ID')
    const quantity = parsePositiveInteger(addQuantity, '入库数量')

    if (productId === null || quantity === null) return

    addMutation.mutate({
      product_id: productId,
      quantity,
    })
  }

  function handleLookupInventory(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const productId = parsePositiveInteger(lookupProductId, '商品 ID')
    if (productId === null) return
    lookupMutation.mutate(productId)
  }

  return (
    <BusinessPage
      title='库存管理'
      description='覆盖库存初始化、手动入库和按商品查询库存接口。'
    >
      <div className='grid gap-4 xl:grid-cols-[minmax(320px,420px)_1fr]'>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle>初始化库存</CardTitle>
              <CardDescription>每个商品只能初始化一次库存。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className='space-y-4' onSubmit={handleInitInventory}>
                <Field label='商品 ID'>
                  <Input
                    value={initProductId}
                    onChange={(event) => setInitProductId(event.target.value)}
                    inputMode='numeric'
                    placeholder='1'
                  />
                </Field>
                <Field label='初始库存'>
                  <Input
                    value={initQuantity}
                    onChange={(event) => setInitQuantity(event.target.value)}
                    inputMode='numeric'
                    placeholder='100'
                  />
                </Field>
                <Button className='w-full' disabled={initMutation.isPending}>
                  初始化
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>手动入库</CardTitle>
              <CardDescription>库存记录已存在时才能增加库存。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className='space-y-4' onSubmit={handleAddInventory}>
                <Field label='商品 ID'>
                  <Input
                    value={addProductId}
                    onChange={(event) => setAddProductId(event.target.value)}
                    inputMode='numeric'
                    placeholder='1'
                  />
                </Field>
                <Field label='入库数量'>
                  <Input
                    value={addQuantity}
                    onChange={(event) => setAddQuantity(event.target.value)}
                    inputMode='numeric'
                    placeholder='20'
                  />
                </Field>
                <Button className='w-full' disabled={addMutation.isPending}>
                  增加入库
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>库存查询</CardTitle>
            <CardDescription>后端当前提供按商品 ID 查询库存。</CardDescription>
          </CardHeader>
          <CardContent className='space-y-4'>
            <form className='flex gap-2' onSubmit={handleLookupInventory}>
              <Input
                value={lookupProductId}
                onChange={(event) => setLookupProductId(event.target.value)}
                inputMode='numeric'
                placeholder='商品 ID'
              />
              <Button type='submit' size='icon' disabled={lookupMutation.isPending}>
                <Search />
                <span className='sr-only'>查询库存</span>
              </Button>
            </form>

            {inventory ? (
              <div className='grid gap-4 sm:grid-cols-3'>
                <div className='rounded-md border p-4'>
                  <p className='text-sm text-muted-foreground'>商品 ID</p>
                  <p className='mt-2 text-2xl font-bold'>#{inventory.product_id}</p>
                </div>
                <div className='rounded-md border p-4'>
                  <p className='text-sm text-muted-foreground'>当前库存</p>
                  <p className='mt-2 text-2xl font-bold'>
                    {inventory.stock_quantity}
                  </p>
                </div>
                <div className='rounded-md border p-4'>
                  <p className='text-sm text-muted-foreground'>更新时间</p>
                  <p className='mt-2 text-lg font-semibold'>
                    {formatDateTime(inventory.updated_at)}
                  </p>
                </div>
              </div>
            ) : (
              <div className='rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground'>
                输入商品 ID 后查询库存。
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}
