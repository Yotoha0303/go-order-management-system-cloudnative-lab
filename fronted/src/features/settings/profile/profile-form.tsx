import { useEffect, useState } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { getErrorMessage } from '@/lib/api-client'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { authApi } from '@/features/auth/api'

const profileFormSchema = z.object({
  nickname: z
    .string()
    .trim()
    .min(1, '请输入昵称')
    .max(64, '昵称最多 64 个字符'),
})

type ProfileFormValues = z.infer<typeof profileFormSchema>

export function ProfileForm() {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const profileQuery = useQuery({
    queryKey: ['current-user'],
    queryFn: authApi.me,
    initialData: user ?? undefined,
    refetchOnMount: 'always',
  })
  const form = useForm<ProfileFormValues>({
    resolver: zodResolver(profileFormSchema),
    defaultValues: { nickname: user?.nickname ?? '' },
  })

  useEffect(() => {
    if (!profileQuery.data) return
    setUser(profileQuery.data)
    form.reset({ nickname: profileQuery.data.nickname })
  }, [form, profileQuery.data, setUser])

  async function onSubmit(data: ProfileFormValues) {
    setIsSubmitting(true)
    try {
      await authApi.updateProfile(data.nickname)
      const currentUser = profileQuery.data ?? user
      if (currentUser) {
        const updatedUser = { ...currentUser, nickname: data.nickname }
        queryClient.setQueryData(['current-user'], updatedUser)
        setUser(updatedUser)
        form.reset({ nickname: updatedUser.nickname })
      }
      toast.success('个人资料更新成功')
    } catch (error) {
      toast.error(getErrorMessage(error))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-8'>
        <FormField
          control={form.control}
          name='nickname'
          render={({ field }) => (
            <FormItem>
              <FormLabel>昵称</FormLabel>
              <FormControl>
                <Input
                  placeholder='请输入昵称'
                  disabled={profileQuery.isPending || isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                该昵称会显示在订单库存管理系统中。
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type='submit' disabled={profileQuery.isPending || isSubmitting}>
          保存资料
        </Button>
      </form>
    </Form>
  )
}
