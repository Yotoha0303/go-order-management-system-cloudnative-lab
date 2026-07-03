import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, type RenderResult } from 'vitest-browser-react'
import { type Locator, userEvent } from 'vitest/browser'
import { SignUpForm } from './sign-up-form'

const FORM_MESSAGES = {
  usernameEmpty: '请输入用户名',
  passwordEmpty: '请输入密码',
  confirmPasswordEmpty: '请再次输入密码',
  passwordMismatch: '两次输入的密码不一致',
} as const

const registerMock = vi.hoisted(() => vi.fn())
const navigate = vi.hoisted(() => vi.fn())

vi.mock('@/features/auth/api', () => ({
  authApi: { register: registerMock },
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

describe('SignUpForm', () => {
  let screen: RenderResult
  let usernameInput: Locator
  let passwordInput: Locator
  let confirmPasswordInput: Locator
  let submitButton: Locator

  beforeEach(async () => {
    vi.clearAllMocks()
    registerMock.mockResolvedValue(undefined)

    screen = await render(<SignUpForm />)
    usernameInput = screen.getByRole('textbox', { name: '用户名' })
    passwordInput = screen.getByLabelText('密码', { exact: true })
    confirmPasswordInput = screen.getByLabelText('确认密码')
    submitButton = screen.getByRole('button', { name: '创建账号' })
  })

  it('renders fields and submit button', async () => {
    await expect.element(usernameInput).toBeInTheDocument()
    await expect.element(passwordInput).toBeInTheDocument()
    await expect.element(confirmPasswordInput).toBeInTheDocument()
    await expect.element(submitButton).toBeInTheDocument()
  })

  it('shows validation messages when submitting empty form', async () => {
    await userEvent.click(submitButton)

    await expect
      .element(screen.getByText(FORM_MESSAGES.usernameEmpty))
      .toBeInTheDocument()
    await expect
      .element(screen.getByText(FORM_MESSAGES.passwordEmpty))
      .toBeInTheDocument()
    await expect
      .element(screen.getByText(FORM_MESSAGES.confirmPasswordEmpty))
      .toBeInTheDocument()
  })

  it('shows a mismatch error when passwords do not match', async () => {
    await userEvent.fill(usernameInput, 'alice')
    await userEvent.fill(passwordInput, '1234567')
    await userEvent.fill(confirmPasswordInput, '7654321')

    await userEvent.click(submitButton)
    await expect
      .element(screen.getByText(FORM_MESSAGES.passwordMismatch))
      .toBeInTheDocument()
  })

  it('registers and redirects to sign in', async () => {
    await userEvent.fill(usernameInput, 'alice')
    await userEvent.fill(passwordInput, '1234567')
    await userEvent.fill(confirmPasswordInput, '1234567')

    await userEvent.click(submitButton)
    await vi.waitFor(() =>
      expect(registerMock).toHaveBeenCalledWith('alice', '1234567')
    )
    expect(navigate).toHaveBeenCalledWith({ to: '/sign-in', replace: true })
  })
})
