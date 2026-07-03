import { useState } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
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
import { PasswordInput } from '@/components/password-input'
import { authApi } from '@/features/auth/api'

const accountFormSchema = z
  .object({
    oldPassword: z.string().min(1, '请输入当前密码'),
    newPassword: z
      .string()
      .min(6, '新密码至少 6 个字符')
      .max(72, '新密码最多 72 个字符'),
    confirmPassword: z.string().min(1, '请再次输入新密码'),
  })
  .refine((data) => data.oldPassword !== data.newPassword, {
    message: '新密码不能与当前密码相同',
    path: ['newPassword'],
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: '两次输入的新密码不一致',
    path: ['confirmPassword'],
  })

type AccountFormValues = z.infer<typeof accountFormSchema>

export function AccountForm() {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(accountFormSchema),
    defaultValues: {
      oldPassword: '',
      newPassword: '',
      confirmPassword: '',
    },
  })

  async function onSubmit(data: AccountFormValues) {
    setIsSubmitting(true)
    try {
      await authApi.updatePassword({
        oldPassword: data.oldPassword,
        newPassword: data.newPassword,
      })
      form.reset()
      toast.success('密码修改成功')
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
          name='oldPassword'
          render={({ field }) => (
            <FormItem>
              <FormLabel>当前密码</FormLabel>
              <FormControl>
                <PasswordInput
                  autoComplete='current-password'
                  disabled={isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>输入当前密码以确认本次修改。</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='newPassword'
          render={({ field }) => (
            <FormItem>
              <FormLabel>新密码</FormLabel>
              <FormControl>
                <PasswordInput
                  autoComplete='new-password'
                  disabled={isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>密码长度为 6 到 72 个字符。</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='confirmPassword'
          render={({ field }) => (
            <FormItem>
              <FormLabel>确认新密码</FormLabel>
              <FormControl>
                <PasswordInput
                  autoComplete='new-password'
                  disabled={isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type='submit' disabled={isSubmitting}>
          修改密码
        </Button>
      </form>
    </Form>
  )
}
