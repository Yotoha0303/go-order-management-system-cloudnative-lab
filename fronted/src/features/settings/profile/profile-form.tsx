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
    .min(1, 'Please enter your nickname.')
    .max(64, 'Nickname must be at most 64 characters long.'),
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
      toast.success('Profile updated successfully.')
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
              <FormLabel>Nickname</FormLabel>
              <FormControl>
                <Input
                  placeholder='Your nickname'
                  disabled={profileQuery.isPending || isSubmitting}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                This is the display name shown in the order management system.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type='submit' disabled={profileQuery.isPending || isSubmitting}>
          Update profile
        </Button>
      </form>
    </Form>
  )
}
