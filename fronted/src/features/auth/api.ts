import { api, unwrap } from '@/lib/api-client'

export type AuthUser = {
  id: number
  username: string
  nickname: string
  status: number
  last_login_at?: string
}

export type LoginResult = {
  access_token: string
  user: AuthUser
}

export type UpdatePasswordParams = {
  oldPassword: string
  newPassword: string
}

export const authApi = {
  register: (username: string, password: string) =>
    unwrap<void>(api.post('/auth/register', { username, password })),
  login: (username: string, password: string) =>
    unwrap<LoginResult>(api.post('/auth/login', { username, password })),
  me: () => unwrap<AuthUser>(api.get('/users/me')),
  updateProfile: (nickname: string) =>
    unwrap<void>(api.put('/users/me/profile', { nickname })),
  updatePassword: ({ oldPassword, newPassword }: UpdatePasswordParams) =>
    unwrap<void>(
      api.patch('/users/me/password', {
        old_password: oldPassword,
        new_password: newPassword,
      })
    ),
}
