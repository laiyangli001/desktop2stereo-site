import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { D2SMySupportTickets } from '../../features/d2s/my-support-tickets'
import { D2SSupportFloatingButton } from '../../features/d2s/support-floating-button'
import zhCN from '../../i18n/locales/zh.json'
import { D2SSupportPage, Route as supportRoute } from '../support'

const supportMocks = vi.hoisted(() => ({
  submitTicket: vi.fn(),
  getMyTickets: vi.fn(),
  redirect: vi.fn(() => new Error('support route redirect')),
  auth: {
    user: { id: 7, email: 'user@example.com' } as {
      id: number
      email: string
    } | null,
    accessToken: 'test-access-token' as string | null,
  },
  pathname: '/',
}))

vi.mock('@/features/d2s/api', () => ({
  submitD2SSupportTicket: supportMocks.submitTicket,
  getD2SMySupportTickets: supportMocks.getMyTickets,
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: Object.assign(
    (selector: (state: unknown) => unknown) =>
      selector({ auth: supportMocks.auth }),
    { getState: () => ({ auth: supportMocks.auth }) }
  ),
}))

vi.mock('@/lib/auth-session', () => ({
  resolveAuthentication: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (options: unknown) => options,
  Link: ({
    children,
    onClick,
    to,
  }: {
    children: React.ReactNode
    onClick?: (event: React.MouseEvent<HTMLAnchorElement>) => void
    to: string
  }) => (
    <a
      href={to}
      onClick={(event) => {
        event.preventDefault()
        onClick?.(event)
      }}
    >
      {children}
    </a>
  ),
  redirect: supportMocks.redirect,
  useRouterState: ({
    select,
  }: {
    select: (state: { location: { pathname: string } }) => unknown
  }) => select({ location: { pathname: supportMocks.pathname } }),
}))

describe('D2S support form', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
    supportMocks.auth = {
      user: { id: 7, email: 'user@example.com' },
      accessToken: 'test-access-token',
    }
    supportMocks.pathname = '/'
  })

  it('redirects unauthenticated visitors to sign-in before showing ticket submission', async () => {
    supportMocks.auth = { user: null, accessToken: null }
    const { beforeLoad } = supportRoute as unknown as {
      beforeLoad: () => Promise<void>
    }

    await expect(beforeLoad()).rejects.toThrow('support route redirect')

    expect(supportMocks.redirect).toHaveBeenCalledWith({
      to: '/sign-in',
      search: { redirect: '/support' },
    })
  })

  it('submits an authenticated user issue and shows its reference number', async () => {
    supportMocks.submitTicket.mockResolvedValue({
      success: true,
      data: { ticket_id: 'ticket-123' },
    })

    const user = userEvent.setup()
    render(<D2SSupportPage />)

    expect(screen.getByLabelText('Contact email')).toHaveAttribute(
      'spellcheck',
      'false'
    )
    await user.type(screen.getByLabelText('Subject'), 'Payment not reflected')
    await user.type(
      screen.getByLabelText('Describe the issue', { exact: false }),
      'My payment succeeded but my account does not show the purchase.'
    )
    await user.click(screen.getByRole('button', { name: 'Submit message' }))

    await waitFor(() => {
      expect(supportMocks.submitTicket).toHaveBeenCalledWith(
        expect.objectContaining({
          category: 'PAYMENT',
          contact_email: 'user@example.com',
          subject: 'Payment not reflected',
          message:
            'My payment succeeded but my account does not show the purchase.',
        })
      )
    })
    expect(await screen.findByRole('status')).toHaveTextContent('ticket-123')
    expect(screen.getByText('Message received')).toBeInTheDocument()
  })

  it('rejects a short issue description before submitting', async () => {
    const user = userEvent.setup()
    render(<D2SSupportPage />)

    await user.type(screen.getByLabelText('Subject'), 'Help')
    fireEvent.change(
      screen.getByLabelText('Describe the issue', { exact: false }),
      {
        target: { value: 'short' },
      }
    )
    await user.click(screen.getByRole('button', { name: 'Submit message' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Describe the issue in 10 to 5000 characters.'
    )
    expect(supportMocks.submitTicket).not.toHaveBeenCalled()
  })

  it('opens complaint and secure Enterprise WeChat contact links', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn().mockResolvedValue({
      json: async () => ({
        success: true,
        data: {
          enterprise_wechat_url:
            'https://work.weixin.qq.com/ca/cawcde40faa1d08998',
        },
      }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { rerender } = render(<D2SSupportFloatingButton />)

    const toggle = screen.getByRole('button', {
      name: 'Complaints and support',
    })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(toggle)

    const complaintLink = screen.getByRole('link', {
      name: 'Submit a complaint or question',
    })
    expect(complaintLink).toHaveAttribute('href', '/support')
    expect(
      screen.getByText(
        'Sign in with a registered account to submit a ticket. Enterprise WeChat is also available.'
      )
    ).toBeInTheDocument()
    const wechatLink = await screen.findByRole('link', {
      name: 'Add enterprise WeChat support',
    })
    expect(wechatLink).toHaveAttribute(
      'href',
      'https://work.weixin.qq.com/ca/cawcde40faa1d08998'
    )
    expect(wechatLink).toHaveAttribute('target', '_blank')
    expect(wechatLink).toHaveAttribute('rel', 'noopener noreferrer')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/support/config', {
      cache: 'no-store',
    })

    await user.click(complaintLink)
    supportMocks.pathname = '/support'
    rerender(<D2SSupportFloatingButton />)
    expect(
      screen.getByRole('button', {
        name: 'Complaints and support',
      })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('link', { name: 'Submit a complaint or question' })
    ).not.toBeInTheDocument()
  })

  it('explains the registered-account requirement in Simplified Chinese', async () => {
    const previousLanguage = i18next.language
    try {
      i18next.addResourceBundle('zhCN', 'translation', zhCN.translation)
      await i18next.changeLanguage('zhCN')
      const user = userEvent.setup()
      render(<D2SSupportFloatingButton />)
      await user.click(screen.getByRole('button', { name: '投诉与咨询' }))

      expect(
        screen.getByText(
          '站内提交问题需先注册并登录；也可通过企业微信联系人工客服。'
        )
      ).toBeInTheDocument()
    } finally {
      await i18next.changeLanguage(previousLanguage)
      i18next.removeResourceBundle('zhCN', 'translation')
    }
  })

  it('hides the floating support contact from the independent admin console', () => {
    supportMocks.pathname = '/d2s-admin-console'
    render(<D2SSupportFloatingButton />)

    expect(
      screen.queryByRole('button', { name: 'Complaints and support' })
    ).not.toBeInTheDocument()
  })

  it('shows the signed-in user their own ticket list', async () => {
    const longSubject = 'A'.repeat(180)
    supportMocks.getMyTickets.mockResolvedValue({
      success: true,
      data: {
        items: [
          {
            id: 'ticket-123',
            category: 'PAYMENT',
            order_id: 'order-456',
            subject: longSubject,
            message: 'My payment succeeded but my account does not show it.',
            status: 'IN_PROGRESS',
            admin_reply: 'We are checking the payment with the provider.',
            created_at: 1_800_000_000,
            updated_at: 1_800_000_100,
          },
        ],
        total: 1,
      },
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <D2SMySupportTickets />
      </QueryClientProvider>
    )

    const subject = await screen.findByRole('heading', { name: longSubject })
    expect(subject).toHaveClass('break-words')
    expect(screen.getByText('ticket-123')).toBeInTheDocument()
    expect(screen.getByText('In progress')).toBeInTheDocument()
    expect(screen.getByText('Response from support')).toBeInTheDocument()
    expect(
      screen.getByText('We are checking the payment with the provider.')
    ).toBeInTheDocument()
    expect(supportMocks.getMyTickets).toHaveBeenCalledWith(1)
  })

  it('renders ticket timestamps for the zhCN interface locale', async () => {
    const previousLanguage = i18next.language
    try {
      await i18next.changeLanguage('zhCN')
      supportMocks.getMyTickets.mockResolvedValue({
        success: true,
        data: {
          items: [
            {
              id: 'ticket-zh-cn',
              category: 'PAYMENT',
              order_id: '',
              subject: '付款成功但账户未显示',
              message: '付款后账户中尚未显示购买记录。',
              status: 'OPEN',
              admin_reply: '',
              created_at: 1_800_000_000,
              updated_at: 1_800_000_000,
            },
          ],
          total: 1,
        },
      })
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })

      render(
        <QueryClientProvider client={queryClient}>
          <D2SMySupportTickets />
        </QueryClientProvider>
      )

      expect(
        await screen.findByRole('heading', {
          name: '付款成功但账户未显示',
        })
      ).toBeInTheDocument()
      expect(screen.getByText(/UTC/)).toBeInTheDocument()
    } finally {
      await i18next.changeLanguage(previousLanguage)
    }
  })
})
