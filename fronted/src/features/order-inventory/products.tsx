import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import { getErrorMessage, productApi, queryKeys } from './api'
import {
  ApiErrorPanel,
  BusinessPage,
  EmptyRow,
  Field,
  LoadingRow,
  ProductStatusBadge,
} from './components'
import { formatDateTime, formatFen, PRODUCT_STATUS } from './format'
import { parseYuanToFen } from './money'
import type { Product } from './types'

export function ProductsPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [priceYuan, setPriceYuan] = useState('')
  const [selectedProduct, setSelectedProduct] = useState<Product | null>(null)
  const [page, setPage] = useState(1)
  const pageSize = 10

  const productsQuery = useQuery({
    queryKey: queryKeys.products('all', page, pageSize),
    queryFn: () => productApi.list('all', page, pageSize),
    placeholderData: (previousData) => previousData,
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
      setPage(1)
      await queryClient.invalidateQueries({ queryKey: queryKeys.productsRoot })
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
      toast.success(
        variables.action === 'on-sale' ? '商品已上架' : '商品已下架'
      )
      await queryClient.invalidateQueries({ queryKey: queryKeys.productsRoot })
      if (selectedProduct?.id === variables.id) {
        productDetailMutation.mutate(variables.id)
      }
    },
    onError: (error) => toast.error(getErrorMessage(error)),
  })

  function handleCreateProduct(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!name.trim()) {
      toast.error('商品名称不能为空')
      return
    }
    const priceFen = parseYuanToFen(priceYuan)
    if (priceFen === null) {
      toast.error('商品价格必须是最多两位小数的正数')
      return
    }

    createProductMutation.mutate({
      name: name.trim(),
      description: description.trim(),
      price_fen: priceFen,
    })
  }

  const products = productsQuery.data?.products ?? []
  const total = productsQuery.data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  return (
    <BusinessPage
      title='商品管理'
      description='创建商品并完成库存准备、上架和下架管理。'
    >
      <div className='grid gap-4 xl:grid-cols-[minmax(320px,420px)_1fr]'>
        <div className='space-y-4'>
          <Card>
            <CardHeader>
              <CardTitle>创建商品</CardTitle>
              <CardDescription>
                新商品默认下架，完成库存初始化后再上架。
              </CardDescription>
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
                <Button
                  className='w-full'
                  disabled={createProductMutation.isPending}
                >
                  创建商品
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>商品详情</CardTitle>
              <CardDescription>
                从右侧列表选择商品后查看并执行状态操作。
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
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
                        (statusMutation.isPending &&
                          statusMutation.variables?.id === selectedProduct.id)
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
                        (statusMutation.isPending &&
                          statusMutation.variables?.id === selectedProduct.id)
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
              {!selectedProduct && (
                <div className='rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground'>
                  请从商品列表选择一项。
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>商品列表</CardTitle>
            <CardDescription>
              展示全部商品，可直接查看详情或调整上下架状态。
            </CardDescription>
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
                      <TableCell>
                        {formatDateTime(product.updated_at)}
                      </TableCell>
                      <TableCell>
                        <div className='flex justify-end gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => {
                              productDetailMutation.mutate(product.id)
                            }}
                          >
                            详情
                          </Button>
                          <Button
                            size='sm'
                            disabled={
                              product.status === PRODUCT_STATUS.ON_SALE ||
                              (statusMutation.isPending &&
                                statusMutation.variables?.id === product.id)
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
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={
                              product.status === PRODUCT_STATUS.OFF_SALE ||
                              (statusMutation.isPending &&
                                statusMutation.variables?.id === product.id)
                            }
                            onClick={() =>
                              statusMutation.mutate({
                                id: product.id,
                                action: 'off-sale',
                              })
                            }
                          >
                            下架
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                {!productsQuery.isLoading && products.length === 0 && (
                  <EmptyRow colSpan={6} message='暂无商品' />
                )}
              </TableBody>
            </Table>
            <div className='flex items-center justify-between gap-3'>
              <p className='text-sm text-muted-foreground'>
                共 {total} 条，第 {page} / {Math.max(totalPages, 1)} 页
              </p>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page === 1 || productsQuery.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  上一页
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages || productsQuery.isFetching}
                  onClick={() => setPage((current) => current + 1)}
                >
                  下一页
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </BusinessPage>
  )
}
