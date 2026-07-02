import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import type { Product } from './types'
import { formatDateTime, formatFen, PRODUCT_STATUS } from './format'
import { getErrorMessage, productApi, queryKeys } from './api'
import {
  ApiErrorPanel,
  BusinessPage,
  EmptyRow,
  Field,
  LoadingRow,
  ProductStatusBadge,
} from './components'

export function ProductsPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [priceYuan, setPriceYuan] = useState('')
  const [detailProductId, setDetailProductId] = useState('')
  const [selectedProduct, setSelectedProduct] = useState<Product | null>(null)

  const productsQuery = useQuery({
    queryKey: queryKeys.products,
    queryFn: productApi.list,
  })

  const productDetailMutation = useMutation({
    mutationFn: productApi.detail,
    onSuccess: (product) => {
      setSelectedProduct(product)
      toast.success('商品详情已更新')
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const createProductMutation = useMutation({
    mutationFn: productApi.create,
    onSuccess: async () => {
      toast.success('商品创建成功')
      setName('')
      setDescription('')
      setPriceYuan('')
      await queryClient.invalidateQueries({ queryKey: queryKeys.products })
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  const statusMutation = useMutation({
    mutationFn: async ({
      id,
      action,
    }: {
      id: number
      action: 'on-sale' | 'off-sale'
    }) => {
      if (action === 'on-sale') return productApi.onSale(id)
      return productApi.offSale(id)
    },
    onSuccess: async (_data, variables) => {
      toast.success(variables.action === 'on-sale' ? '商品已上架' : '商品已下架')
      await queryClient.invalidateQueries({ queryKey: queryKeys.products })
      if (selectedProduct?.id === variables.id) {
        productDetailMutation.mutate(variables.id)
      }
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  function handleCreateProduct(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const priceFen = Math.round(Number(priceYuan) * 100)
    if (!name.trim()) {
      toast.error('商品名称不能为空')
      return
    }
    if (!Number.isFinite(priceFen) || priceFen <= 0) {
      toast.error('商品价格必须大于 0')
      return
    }

    createProductMutation.mutate({
      name: name.trim(),
      description: description.trim(),
      price_fen: priceFen,
    })
  }

  function handleSearchProduct(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const id = Number(detailProductId)
    if (!Number.isInteger(id) || id <= 0) {
      toast.error('请输入有效商品 ID')
      return
    }
    productDetailMutation.mutate(id)
  }

  const products = productsQuery.data ?? []

  return (
    <BusinessPage
      title='商品管理'
      description='覆盖创建、列表、详情、上架和下架接口；列表接口当前展示下架商品。'
    >
      <div className='grid gap-4 xl:grid-cols-[minmax(320px,420px)_1fr]'>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle>创建商品</CardTitle>
              <CardDescription>新商品默认下架，完成库存初始化后再上架。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className='space-y-4' onSubmit={handleCreateProduct}>
                <Field label='商品名称'>
                  <Input
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={100}
                    placeholder='例：机械键盘'
                  />
                </Field>
                <Field label='价格（元）'>
                  <Input
                    value={priceYuan}
                    onChange={(event) => setPriceYuan(event.target.value)}
                    inputMode='decimal'
                    placeholder='199.00'
                  />
                </Field>
                <Field label='商品描述'>
                  <Textarea
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    maxLength={500}
                    placeholder='可选，最多 500 字'
                  />
                </Field>
                <Button className='w-full' disabled={createProductMutation.isPending}>
                  创建商品
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>商品详情查询</CardTitle>
              <CardDescription>用于查询已上架商品，或按 ID 执行上下架。</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <form className='flex gap-2' onSubmit={handleSearchProduct}>
                <Input
                  value={detailProductId}
                  onChange={(event) => setDetailProductId(event.target.value)}
                  inputMode='numeric'
                  placeholder='商品 ID'
                />
                <Button
                  type='submit'
                  size='icon'
                  disabled={productDetailMutation.isPending}
                >
                  <Search />
                  <span className='sr-only'>查询商品</span>
                </Button>
              </form>

              {selectedProduct && (
                <div className='rounded-md border p-4 text-sm'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='min-w-0'>
                      <p className='truncate font-medium'>
                        #{selectedProduct.id} {selectedProduct.name}
                      </p>
                      <p className='mt-1 text-muted-foreground'>
                        {formatFen(selectedProduct.price_fen)}
                      </p>
                    </div>
                    <ProductStatusBadge status={selectedProduct.status} />
                  </div>
                  <p className='mt-3 whitespace-pre-wrap text-muted-foreground'>
                    {selectedProduct.description || '暂无描述'}
                  </p>
                  <div className='mt-4 flex flex-wrap gap-2'>
                    <Button
                      size='sm'
                      disabled={
                        selectedProduct.status === PRODUCT_STATUS.ON_SALE ||
                        statusMutation.isPending
                      }
                      onClick={() =>
                        statusMutation.mutate({
                          id: selectedProduct.id,
                          action: 'on-sale',
                        })
                      }
                    >
                      上架
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={
                        selectedProduct.status === PRODUCT_STATUS.OFF_SALE ||
                        statusMutation.isPending
                      }
                      onClick={() =>
                        statusMutation.mutate({
                          id: selectedProduct.id,
                          action: 'off-sale',
                        })
                      }
                    >
                      下架
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>商品列表</CardTitle>
            <CardDescription>后端当前按下架状态查询，可用于库存初始化前准备。</CardDescription>
          </CardHeader>
          <CardContent className='space-y-4'>
            <ApiErrorPanel error={productsQuery.error} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>名称</TableHead>
                  <TableHead>价格</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>更新时间</TableHead>
                  <TableHead className='text-end'>操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {productsQuery.isLoading && <LoadingRow colSpan={6} />}
                {!productsQuery.isLoading &&
                  products.map((product) => (
                    <TableRow key={product.id}>
                      <TableCell>#{product.id}</TableCell>
                      <TableCell className='max-w-[260px] whitespace-normal'>
                        <div className='font-medium'>{product.name}</div>
                        <div className='line-clamp-2 text-muted-foreground'>
                          {product.description || '暂无描述'}
                        </div>
                      </TableCell>
                      <TableCell>{formatFen(product.price_fen)}</TableCell>
                      <TableCell>
                        <ProductStatusBadge status={product.status} />
                      </TableCell>
                      <TableCell>{formatDateTime(product.updated_at)}</TableCell>
                      <TableCell>
                        <div className='flex justify-end gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => {
                              setDetailProductId(String(product.id))
                              productDetailMutation.mutate(product.id)
                            }}
                          >
                            详情
                          </Button>
                          <Button
                            size='sm'
                            disabled={
                              product.status === PRODUCT_STATUS.ON_SALE ||
                              statusMutation.isPending
                            }
                            onClick={() =>
                              statusMutation.mutate({
                                id: product.id,
                                action: 'on-sale',
                              })
                            }
                          >
                            上架
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                {!productsQuery.isLoading && products.length === 0 && (
                  <EmptyRow colSpan={6} message='暂无下架商品' />
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}
