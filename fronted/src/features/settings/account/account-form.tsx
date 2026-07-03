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
    oldPassword: z.string().min(1, 'Please enter your current password.'),
    newPassword: z
      .string()
      .min(6, 'New password must be at least 6 characters long.')
      .max(72, 'New password must be at most 72 characters long.'),
    confirmPassword: z.string().min(1, 'Please confirm your new password.'),
  })
  .refine((data) => data.oldPassword !== data.newPassword, {
    message: 'New password must be different from the current password.',
    path: ['newPassword'],
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: "Passwords don't match.",
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
      toast.success('Password updated successfully.')
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
              <FormLabel>Current password</FormLabel>
              <FormControl>
                <PasswordInput
                  autoComplete='current-password'
                  disabled={isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                Enter your current password to confirm this change.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='newPassword'
          render={({ field }) => (
            <FormItem>
              <FormLabel>New password</FormLabel>
              <FormControl>
                <PasswordInput
                  autoComplete='new-password'
                  disabled={isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                Use between 6 and 72 characters.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='confirmPassword'
          render={({ field }) => (
            <FormItem>
              <FormLabel>Confirm new password</FormLabel>
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
          Update password
        </Button>
      </form>
    </Form>
  )
}
