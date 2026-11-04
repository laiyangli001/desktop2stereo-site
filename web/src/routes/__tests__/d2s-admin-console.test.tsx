import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  getD2SAdminAlipayPCSettings,
  saveD2SAdminAlipayPCConfig,
} from '../../features/d2s/api'
import { downloadBlob } from '../../features/d2s/download'
import { D2SAdminConsole } from '../d2s-admin-console'

vi.mock('../../features/d2s/download', () => ({ downloadBlob: vi.fn() }))
vi.mock('../../features/d2s/api', () => ({
  getD2SAdminAlipayPCSettings: vi.fn(),
  saveD2SAdminAlipayPCConfig: vi.fn(),
  useD2SAdminAlipayEnvironmentConfig: vi.fn(),
}))

describe('D2SAdminConsole login', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('opens Alipay settings for an independent super admin and saves the selected QR mode without resending stored keys', async () => {
    const settings = {
      qr_pay_mode: '4',
      config_source: 'admin' as const,
      channel_enabled: false,
      app_id: '2026000000000000',
      seller_id: '2088000000000000',
      gateway_url: 'https://openapi.alipay.com/gateway.do',
      notify_url: 'https://d2s.site/api/v1/alipay/d2s/notify',
      return_url: 'https://d2s.site/d2s',
      merchant_private_key_configured: true,
      alipay_public_key_configured: true,
      ready: true,
      updated_at: 1,
    }
    vi.mocked(getD2SAdminAlipayPCSettings).mockResolvedValue({
      success: true,
      data: settings,
    })
    vi.mocked(saveD2SAdminAlipayPCConfig).mockResolvedValue({
      success: true,
      data: settings,
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          ({
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                admin: {
                  id: 1,
                  email: 'admin@example.com',
                  role: 'super_admin',
                },
              },
            }),
          }) as Response
      )
    )
    const user = userEvent.setup()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <D2SAdminConsole />
      </QueryClientProvider>
    )
    await user.click(await screen.findByRole('button', { name: '支付宝支付' }))
    expect(await screen.findByLabelText('Alipay app ID')).toHaveValue(
      settings.app_id
    )
    expect(screen.getByLabelText(/Merchant RSA2 private key/)).toHaveValue('')
    expect(screen.getByLabelText(/Alipay RSA2 public key/)).toHaveValue('')
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'QR display mode' }),
      '1'
    )
    await user.click(
      screen.getByRole('button', { name: 'Save Alipay settings' })
    )
    await waitFor(() =>
      expect(saveD2SAdminAlipayPCConfig).toHaveBeenCalledWith(
        expect.objectContaining({
          app_id: settings.app_id,
          qr_pay_mode: '1',
          merchant_private_key: undefined,
          alipay_public_key: undefined,
        }),
        expect.anything()
      )
    )
  })

  it('announces the pending admin-session check', async () => {
    let resolveSession: ((response: Response) => void) | undefined
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveSession = resolve
        })
    )
    vi.stubGlobal('fetch', fetchMock)

    render(<D2SAdminConsole />)

    expect(screen.getByRole('main')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('正在检查后台会话…')
    await waitFor(() => expect(resolveSession).toBeDefined())
    resolveSession?.({
      ok: false,
      status: 401,
      json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
    } as Response)

    expect(
      await screen.findByRole('textbox', { name: '管理员邮箱' })
    ).toBeInTheDocument()
  })

  it('opens an account operator on an authorized module, not the audit log', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: false,
          status: 401,
          json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
        } as Response
      }
      if (path.endsWith('/api/v1/admin/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 10,
                email: 'operator@example.com',
                role: 'account_operator',
              },
            },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: {} }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await user.type(
      await screen.findByRole('textbox', { name: '管理员邮箱' }),
      'operator@example.com'
    )
    await user.type(screen.getByLabelText('密码'), 'not-a-real-password{Enter}')

    expect(
      await screen.findByRole('heading', { name: '账户紧急限制' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: '用户行为日志' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '行为日志' })).toBeNull()
    expect(screen.queryByRole('button', { name: '支付宝支付' })).toBeNull()
  })

  it('shows every audit field to an audit reader', async () => {
    const auditRecord = {
      id: 21,
      user_id: 8,
      email: 'reader@example.com',
      action_type: 'LOGIN',
      event_time: 1761830400,
      source_ip: '192.0.2.11',
      source_port: 43120,
      destination_ip: '192.0.2.20',
      destination_port: 443,
      user_agent: 'D2S test client',
      device_fingerprint: 'sha256:device',
      request_path: '/api/login',
      result_status: 'SUCCESS',
      record_hash: 'sha256:record',
    }
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 12,
                email: 'reader@example.com',
                role: 'audit_reader',
              },
            },
          }),
        } as Response
      }
      if (path.startsWith('/api/v1/admin/audit/user-events?')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: { items: [auditRecord], total: 1 },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: {} }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '用户行为日志' })
    await user.click(screen.getByRole('button', { name: '查询' }))

    for (const field of [
      'id',
      'user_id',
      'email',
      'action_type',
      'event_time',
      'source_ip',
      'source_port',
      'destination_ip',
      'destination_port',
      'user_agent',
      'device_fingerprint',
      'request_path',
      'result_status',
      'record_hash',
    ]) {
      expect(
        await screen.findByRole('columnheader', { name: field })
      ).toBeInTheDocument()
    }
    expect(screen.getByRole('cell', { name: '192.0.2.11' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: '192.0.2.20' })).toBeInTheDocument()
    expect(
      screen.getByRole('cell', { name: 'sha256:record' })
    ).toBeInTheDocument()
  })

  it('requires confirmation and sends the selected emergency restrictions', async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) => {
        const path = String(input)
        if (path.endsWith('/api/v1/admin/auth/session')) {
          return {
            ok: false,
            status: 401,
            json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
          } as Response
        }
        if (path.endsWith('/api/v1/admin/auth/login')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                admin: {
                  id: 11,
                  email: 'operator@example.com',
                  role: 'account_operator',
                },
              },
            }),
          } as Response
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true, data: {} }),
        } as Response
      }
    )
    const confirmMock = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', confirmMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await user.type(
      await screen.findByRole('textbox', { name: '管理员邮箱' }),
      'operator@example.com'
    )
    await user.type(screen.getByLabelText('密码'), 'not-a-real-password{Enter}')
    await screen.findByRole('heading', { name: '账户紧急限制' })

    await user.type(screen.getByRole('textbox', { name: '账户 ID' }), '42')
    await user.type(
      screen.getByRole('textbox', { name: '处置原因' }),
      '异常交易'
    )
    await user.click(screen.getByRole('checkbox', { name: '禁止登录' }))
    await user.click(screen.getByRole('checkbox', { name: '禁止购买' }))

    const applyButton = screen.getByRole('button', { name: '应用账户限制' })
    await user.click(applyButton)
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).endsWith('/emergency')
      )
    ).toBe(false)

    await user.click(applyButton)
    expect(confirmMock).toHaveBeenCalledTimes(2)
    await waitFor(() => {
      const emergencyCall = fetchMock.mock.calls.find(([input]) =>
        String(input).endsWith('/emergency')
      )
      expect(emergencyCall).toBeDefined()
      expect(emergencyCall?.[1]).toEqual(
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({
            login_blocked: true,
            purchase_blocked: true,
            force_logout: false,
            reason: '异常交易',
          }),
        })
      )
    })
  })

  it('invalidates stale batch previews when criteria change or refresh fails', async () => {
    let previewAttempts = 0
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) => {
        const path = String(input)
        if (path.endsWith('/api/v1/admin/auth/session')) {
          return {
            ok: false,
            status: 401,
            json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
          } as Response
        }
        if (path.endsWith('/api/v1/admin/auth/login')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                admin: {
                  id: 12,
                  email: 'operator@example.com',
                  role: 'account_operator',
                },
              },
            }),
          } as Response
        }
        if (path.endsWith('/api/v1/admin/users/batch/preview')) {
          previewAttempts += 1
          if (previewAttempts > 1) {
            return {
              ok: false,
              status: 503,
              json: async () => ({
                success: false,
                code: 'PREVIEW_UNAVAILABLE',
              }),
            } as Response
          }
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: { preview_id: 'preview-1', match_count: 3 },
            }),
          } as Response
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true, data: {} }),
        } as Response
      }
    )
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await user.type(
      await screen.findByRole('textbox', { name: '管理员邮箱' }),
      'operator@example.com'
    )
    await user.type(screen.getByLabelText('密码'), 'not-a-real-password{Enter}')
    await screen.findByRole('heading', { name: '账户紧急限制' })

    const emailDomain = screen.getByLabelText('邮箱域名')
    await user.type(emailDomain, 'example.com')
    await user.click(screen.getByRole('button', { name: '预览影响账户' }))
    expect(
      await screen.findByRole('button', { name: '确认处置 3 个账户' })
    ).toBeInTheDocument()

    await user.clear(emailDomain)
    await user.type(emailDomain, 'other.example')
    expect(
      screen.queryByRole('button', { name: '确认处置 3 个账户' })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '预览影响账户' }))
    expect(await screen.findByText('PREVIEW_UNAVAILABLE')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: '确认处置 3 个账户' })
    ).not.toBeInTheDocument()

    const previewCalls = fetchMock.mock.calls.filter(([input]) =>
      String(input).endsWith('/api/v1/admin/users/batch/preview')
    )
    expect(JSON.parse(String(previewCalls[1]?.[1]?.body))).toMatchObject({
      selector: { email_domain: 'other.example' },
    })
  })

  it('requests a second-admin export approval before downloading filtered logs', async () => {
    const csv = new Blob(['id,email\\n1,user@example.com'], {
      type: 'text/csv',
    })
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) => {
        const path = String(input)
        if (path.endsWith('/api/v1/admin/auth/session')) {
          return {
            ok: false,
            status: 401,
            json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
          } as Response
        }
        if (path.endsWith('/api/v1/admin/auth/login')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                admin: {
                  id: 13,
                  email: 'auditor@example.com',
                  role: 'super_admin',
                },
              },
            }),
          } as Response
        }
        if (path.includes('/api/v1/admin/audit/export-approvals?')) {
          return {
            ok: true,
            status: 201,
            json: async () => ({
              success: true,
              data: {
                id: 'approval-123',
                export_type: 'USER_ACTION_LOGS',
                status: 'PENDING',
                filter_snapshot: '{"email":"user@example.com"}',
              },
            }),
          } as Response
        }
        if (
          path.endsWith('/api/v1/admin/audit/export-approvals/approval-123')
        ) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                id: 'approval-123',
                export_type: 'USER_ACTION_LOGS',
                status: 'APPROVED',
                filter_snapshot: '{"email":"user@example.com"}',
              },
            }),
          } as Response
        }
        return {
          ok: true,
          status: 200,
          headers: {
            get: (name: string) =>
              name.toLowerCase() === 'content-disposition'
                ? 'attachment; filename="access_log_20261030_120000.csv"'
                : null,
          },
          blob: async () => csv,
        } as Response
      }
    )
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await user.type(
      await screen.findByRole('textbox', { name: '管理员邮箱' }),
      'auditor@example.com'
    )
    await user.type(screen.getByLabelText('密码'), 'not-a-real-password{Enter}')
    await screen.findByRole('heading', { name: '账户审计与治理' })

    await user.type(
      screen.getByLabelText('开始时间（Unix 秒或 RFC3339）'),
      '1793361600'
    )
    await user.type(screen.getByLabelText('邮箱'), 'user@example.com')
    await user.type(screen.getByLabelText('来源 IP'), '203.0.113.7')
    await user.type(screen.getByLabelText('行为类型'), 'LOGIN')
    await user.type(
      screen.getByLabelText('导出用途（审批必填）'),
      '核查用户请求'
    )
    await user.click(screen.getByRole('button', { name: '申请导出审批' }))
    expect(await screen.findByText(/状态：PENDING/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '刷新审批状态' }))
    expect(await screen.findByText(/状态：APPROVED/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '刷新审批状态' })).toHaveClass(
      'focus-visible:ring-2'
    )
    expect(screen.getByRole('button', { name: '下载已批准 CSV' })).toHaveClass(
      'focus-visible:ring-2'
    )
    await user.click(screen.getByRole('button', { name: '下载已批准 CSV' }))

    await waitFor(() => expect(downloadBlob).toHaveBeenCalledOnce())
    expect(downloadBlob).toHaveBeenCalledWith(
      csv,
      'access_log_20261030_120000.csv'
    )

    const exportCall = fetchMock.mock.calls.find(([input]) =>
      String(input).includes('/api/v1/admin/audit/user-events/export.csv?')
    )
    expect(exportCall?.[1]).toEqual(
      expect.objectContaining({ credentials: 'include', cache: 'no-store' })
    )
    const query = new URLSearchParams(String(exportCall?.[0]).split('?')[1])
    expect(Object.fromEntries(query)).toMatchObject({
      start_time: '1793361600',
      email: 'user@example.com',
      source_ip: '203.0.113.7',
      action_type: 'LOGIN',
      approval_id: 'approval-123',
    })
    const approvalCall = fetchMock.mock.calls.find(([input]) =>
      String(input).includes('/api/v1/admin/audit/export-approvals?')
    )
    expect(approvalCall?.[1]).toEqual(
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        cache: 'no-store',
      })
    )
  })

  it('loads later pages for logs, registrations, and purchases', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 1,
                email: 'admin@example.com',
                role: 'super_admin',
              },
            },
          }),
        } as Response
      }
      const url = new URL(path, 'https://local.test')
      const page = Number(url.searchParams.get('page') || 1)
      if (url.pathname.endsWith('/api/v1/admin/audit/user-events')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              items: [{ id: page, action_type: `log-${page}` }],
              total: 51,
            },
          }),
        } as Response
      }
      if (url.pathname.endsWith('/api/v1/admin/audit/registrations')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              items: [{ id: page, email: `registration-${page}@example.com` }],
              total: 51,
            },
          }),
        } as Response
      }
      if (url.pathname.endsWith('/api/v1/admin/purchases')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              items: [{ id: page, order_id: `purchase-${page}` }],
              total: 51,
            },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: {} }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '用户行为日志' })
    await user.click(screen.getByRole('button', { name: '查询' }))
    const logRequest = fetchMock.mock.calls.find(([input]) =>
      String(input).startsWith('/api/v1/admin/audit/user-events?')
    )
    expect(
      new URL(String(logRequest?.[0]), 'https://local.test').searchParams.get(
        'page'
      )
    ).toBe('1')
    expect(
      await screen.findByRole('cell', { name: 'log-1' })
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '下一页' }))
    expect(
      await screen.findByRole('cell', { name: 'log-2' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '注册记录' }))
    await user.click(screen.getByRole('button', { name: '查询' }))
    const registrationRequest = fetchMock.mock.calls.find(([input]) =>
      String(input).startsWith('/api/v1/admin/audit/registrations?')
    )
    const registrationQuery = new URLSearchParams(
      String(registrationRequest?.[0]).split('?')[1]
    )
    expect(registrationQuery.get('page')).toBe('1')
    expect(registrationQuery.get('page_size')).toBe('50')
    expect(
      await screen.findByRole('cell', { name: 'registration-1@example.com' })
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '下一页' }))
    expect(
      await screen.findByRole('cell', { name: 'registration-2@example.com' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '购买记录' }))
    await user.click(screen.getByRole('button', { name: '刷新' }))
    expect(
      await screen.findByRole('cell', { name: 'purchase-1' })
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '下一页' }))
    expect(
      await screen.findByRole('cell', { name: 'purchase-2' })
    ).toBeInTheDocument()
  })

  it('keeps the visible page aligned when a page request fails', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 1,
                email: 'admin@example.com',
                role: 'super_admin',
              },
            },
          }),
        } as Response
      }
      const url = new URL(path, 'https://local.test')
      if (url.pathname.endsWith('/api/v1/admin/audit/user-events')) {
        const page = Number(url.searchParams.get('page') || 1)
        if (page === 2) {
          return {
            ok: false,
            status: 503,
            json: async () => ({ success: false, code: 'PAGE_FAILED' }),
          } as Response
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: { items: [{ id: 1, action_type: 'LOGIN' }], total: 51 },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: {} }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '用户行为日志' })
    await user.click(screen.getByRole('button', { name: '查询' }))
    expect(
      await screen.findByRole('cell', { name: 'LOGIN' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '下一页' }))
    expect(await screen.findByText('PAGE_FAILED')).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'LOGIN' })).toBeInTheDocument()
    expect(screen.getByText(/第 1 页 · 共 51 条/)).toBeInTheDocument()
  })

  it('submits credentials when Enter is pressed in the password field', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: false,
          status: 401,
          json: async () => ({ success: false, code: 'ADMIN_AUTH_REQUIRED' }),
        } as Response
      }
      if (path.endsWith('/api/v1/admin/auth/login')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 9,
                email: 'reviewer@example.com',
                role: 'super_admin',
              },
            },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true, data: {} }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    const email = await screen.findByRole('textbox', { name: '管理员邮箱' })
    await user.type(email, 'reviewer@example.com')
    const password = screen.getByLabelText('密码')
    await user.type(password, 'not-a-real-password{Enter}')

    expect(
      await screen.findByRole('heading', { name: '账户审计与治理' })
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/admin/auth/login',
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  it('bounds audit retention days in the admin form', async () => {
    const fetchMock = vi.fn(
      async () =>
        ({
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 1,
                email: 'admin@example.com',
                role: 'super_admin',
              },
            },
          }),
        }) as Response
    )
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '账户审计与治理' })
    await user.click(screen.getByRole('button', { name: '日志留存' }))

    const retentionInput = screen.getByRole('spinbutton', {
      name: '保留天数（最少 366 天）',
    })
    expect(retentionInput).toHaveAttribute('min', '366')
    expect(retentionInput).toHaveAttribute('max', '2147483647')
    expect(retentionInput).toHaveAttribute('step', '1')

    const executeButton = screen.getByRole('button', { name: '执行受控清理' })
    expect(executeButton).toBeEnabled()
    await user.clear(retentionInput)
    await user.type(retentionInput, '2147483648')
    expect(executeButton).toBeDisabled()
  })

  it('makes audit result tables keyboard-scrollable and gives controls visible states', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path.endsWith('/api/v1/admin/auth/session')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              admin: {
                id: 1,
                email: 'admin@example.com',
                role: 'super_admin',
              },
            },
          }),
        } as Response
      }
      if (
        path.endsWith(
          '/api/v1/admin/audit/export-approvals?page=1&page_size=50'
        )
      ) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            data: {
              items: [
                {
                  id: 'approval-1',
                  requested_by_email: 'requester@example.com',
                  export_type: 'USER_ACTION_LOGS',
                  request_reason: 'Incident review',
                  filter_snapshot: '{}',
                  status: 'PENDING',
                },
              ],
            },
          }),
        } as Response
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: { items: [{ id: 1, order_id: 'order-1' }], total: 1 },
        }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '用户行为日志' })
    const logRegion = screen.getByRole('region', {
      name: '可横向滚动的用户行为日志',
    })
    expect(logRegion).toHaveAttribute('tabindex', '0')
    expect(logRegion).toHaveClass('focus-visible:ring-2')

    const queryButton = screen.getByRole('button', { name: '查询' })
    expect(queryButton).toHaveClass('hover:bg-muted/70')
    expect(queryButton).toHaveClass('focus-visible:ring-2')

    const pendingRefreshButton = screen.getByRole('button', {
      name: '刷新待审批',
    })
    expect(pendingRefreshButton).toHaveClass('focus-visible:ring-2')
    await user.click(pendingRefreshButton)
    for (const name of ['批准', '拒绝']) {
      expect(screen.getByRole('button', { name })).toHaveClass(
        'focus-visible:ring-2'
      )
    }

    await user.click(screen.getByRole('button', { name: '购买记录' }))
    await user.click(screen.getByRole('button', { name: '刷新' }))
    const recordRegion = await screen.findByRole('region', {
      name: '可横向滚动的记录结果',
    })
    expect(recordRegion).toHaveAttribute('tabindex', '0')
  })

  it('lets an account operator review and update a support ticket', async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input)
        if (path.endsWith('/api/v1/admin/auth/session')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                admin: {
                  id: 18,
                  email: 'operator@example.com',
                  role: 'account_operator',
                },
              },
            }),
          } as Response
        }
        if (path.startsWith('/api/v1/admin/support/tickets?')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                items: [
                  {
                    id: 'ticket-123',
                    user_id: 42,
                    category: 'PAYMENT',
                    contact_email: 'customer@example.com',
                    order_id: 'order-456',
                    subject: 'Payment not reflected',
                    message: 'My payment completed but no license is shown.',
                    status: 'OPEN',
                    admin_reply: '',
                    created_at: 1_800_000_000,
                  },
                ],
                total: 1,
              },
            }),
          } as Response
        }
        if (path.endsWith('/api/v1/admin/support/tickets/ticket-123')) {
          expect(init?.method).toBe('PUT')
          return {
            ok: true,
            status: 200,
            json: async () => ({
              success: true,
              data: {
                id: 'ticket-123',
                status: 'RESOLVED',
                admin_reply: 'We restored your license.',
              },
            }),
          } as Response
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true, data: {} }),
        } as Response
      }
    )
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    render(<D2SAdminConsole />)

    await screen.findByRole('heading', { name: '账户紧急限制' })
    await user.click(screen.getByRole('button', { name: '投诉与咨询' }))

    expect(
      await screen.findByRole('heading', { name: 'Payment not reflected' })
    ).toBeInTheDocument()
    expect(screen.getByText('customer@example.com')).toBeInTheDocument()

    await user.type(
      screen.getByRole('textbox', { name: '内部处理备注' }),
      'We restored your license.'
    )
    await user.selectOptions(
      screen.getByRole('combobox', { name: '更新状态' }),
      'RESOLVED'
    )
    await user.click(screen.getByRole('button', { name: '保存处理状态与备注' }))

    await waitFor(() => {
      const updateCall = fetchMock.mock.calls.find(([input]) =>
        String(input).endsWith('/api/v1/admin/support/tickets/ticket-123')
      )
      expect(updateCall?.[1]).toEqual(
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({
            status: 'RESOLVED',
            admin_reply: 'We restored your license.',
          }),
        })
      )
    })
    expect(
      await screen.findByText('工单处理结果已保存并写入审计')
    ).toBeInTheDocument()
  })
})
