import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { D2SWorkspace, D2SWalletSection } from '..'

const apiMocks = vi.hoisted(() => ({
  changeD2SMode: vi.fn(),
  confirmD2SPermanent: vi.fn(),
  createD2SManualUnbind: vi.fn(),
  createD2SOrder: vi.fn(),
  createD2SOrderCheckout: vi.fn(),
  createD2SWithdrawal: vi.fn(),
  freeRevokeD2SLicense: vi.fn(),
  getD2SBalance: vi.fn(),
  getD2SBalanceTransactions: vi.fn(),
  getD2SCheckoutProviders: vi.fn(),
  getD2SInvite: vi.fn(),
  getD2SInviteRecords: vi.fn(),
  getD2SLicenses: vi.fn(),
  getD2SManualUnbindRequests: vi.fn(),
  getD2SOrders: vi.fn(),
  getD2SWithdrawals: vi.fn(),
  previewD2SOrder: vi.fn(),
}))

vi.mock('../api', () => apiMocks)

describe('D2SWorkspace data status', () => {
  it('announces unavailable data instead of showing an empty account state', async () => {
    for (const query of Object.values(apiMocks)) {
      query.mockRejectedValue(new Error('D2S API unavailable'))
    }

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <D2SWorkspace />
      </QueryClientProvider>
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load Desktop2Stereo data'
    )
  })

  it('renders configured Payment FM and balance ledger entries', async () => {
    apiMocks.confirmD2SPermanent.mockResolvedValue({
      success: true,
      data: { changed: true },
    })
    apiMocks.getD2SBalance.mockResolvedValue({
      success: true,
      data: { accounts: [] },
    })
    apiMocks.getD2SBalanceTransactions.mockResolvedValue({
      success: true,
      data: {
        transactions: [
          {
            id: 'ledger-1',
            currency: 'USD',
            kind: 'reserve',
            amount_minor: -1000,
            created_at: 1,
          },
        ],
      },
    })
    apiMocks.getD2SCheckoutProviders.mockResolvedValue({
      success: true,
      data: { providers: ['paymentfm'] },
    })
    apiMocks.getD2SInvite.mockResolvedValue({
      success: true,
      data: { invite_code: 'invite-code', rewarded_invitees: 0 },
    })
    apiMocks.getD2SInviteRecords.mockResolvedValue({
      success: true,
      data: {
        records: [
          {
            id: 'reward-1',
            invitee_user_id: 55,
            currency: 'USD',
            amount_minor: 140,
            status: 'credited',
            created_at: 1,
          },
        ],
      },
    })
    apiMocks.getD2SLicenses.mockResolvedValue({
      success: true,
      data: {
        licenses: [
          {
            id: 'license-1',
            license_code: 'D2S-TEST-1',
            kind: 'paid',
            status: 'active',
            mode: 'online',
            device_hash: 'a'.repeat(64),
            fingerprint_version: 1,
            offline_period_days: 7,
            offline_valid_until: 0,
            expires_at: 0,
          },
        ],
      },
    })
    apiMocks.getD2SManualUnbindRequests.mockResolvedValue({
      success: true,
      data: { requests: [] },
    })
    apiMocks.getD2SOrders.mockResolvedValue({
      success: true,
      data: { orders: [] },
    })
    apiMocks.getD2SWithdrawals.mockResolvedValue({
      success: true,
      data: { withdrawals: [] },
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <D2SWalletSection />
        <D2SWorkspace />
      </QueryClientProvider>
    )

    expect(
      await screen.findByRole('button', { name: 'Pay with Payment FM' })
    ).toBeInTheDocument()

    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    fireEvent.click(
      screen.getByRole('button', { name: 'Make permanent D2S-TEST-1' })
    )
    await waitFor(() => {
      expect(apiMocks.confirmD2SPermanent).toHaveBeenCalledWith({
        license_id: 'license-1',
        device_hash: 'a'.repeat(64),
        confirmation: 'PERMANENT',
      })
    })
    expect(confirmSpy).toHaveBeenCalledWith('Confirm permanent binding')
    confirmSpy.mockRestore()
  })

  it('keeps commerce controls and invitations out of authorization management', async () => {
    apiMocks.getD2SLicenses.mockResolvedValue({
      success: true,
      data: { licenses: [] },
    })
    apiMocks.getD2SManualUnbindRequests.mockResolvedValue({
      success: true,
      data: { requests: [] },
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <D2SWorkspace />
      </QueryClientProvider>
    )

    expect(
      await screen.findByText('Authorization management')
    ).toBeInTheDocument()
    expect(screen.queryByText('Purchase a license')).not.toBeInTheDocument()
    expect(screen.queryByText('Invitations')).not.toBeInTheDocument()
    expect(screen.queryByText('Withdrawals')).not.toBeInTheDocument()
  })
})
