import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { D2SWorkspace } from '..'

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

function response<T>(data: T) {
  return { success: true, data }
}

function renderWorkspace() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <D2SWorkspace />
    </QueryClientProvider>
  )
}

describe('D2SWorkspace purchase flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getD2SBalance.mockResolvedValue(
      response({
        accounts: [
          {
            id: 'balance-1',
            currency: 'USD',
            available_minor: 0,
            reserved_minor: 0,
          },
        ],
      })
    )
    apiMocks.getD2SBalanceTransactions.mockResolvedValue(
      response({ transactions: [] })
    )
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['stripe'] })
    )
    apiMocks.getD2SInvite.mockResolvedValue(
      response({ invite_code: 'invite-code', rewarded_invitees: 0 })
    )
    apiMocks.getD2SInviteRecords.mockResolvedValue(response({ records: [] }))
    apiMocks.getD2SLicenses.mockResolvedValue(response({ licenses: [] }))
    apiMocks.getD2SManualUnbindRequests.mockResolvedValue(
      response({ requests: [] })
    )
    apiMocks.getD2SOrders.mockResolvedValue(response({ orders: [] }))
    apiMocks.getD2SWithdrawals.mockResolvedValue(response({ withdrawals: [] }))
    apiMocks.previewD2SOrder.mockResolvedValue(
      response({
        product: 'license',
        provider: 'stripe',
        region: 'INTL',
        currency: 'USD',
        amount_minor: 2990,
      })
    )
    apiMocks.createD2SOrder.mockResolvedValue(
      response({ id: 'order-1', product: 'license', provider: 'stripe' })
    )
    apiMocks.createD2SOrderCheckout.mockReturnValue(new Promise(() => {}))
  })

  it('previews and creates a server-priced external order from an accessible control', async () => {
    renderWorkspace()

    const payButton = await screen.findByRole('button', {
      name: 'Pay with Stripe',
    })
    fireEvent.click(payButton)

    await waitFor(() => {
      expect(apiMocks.previewD2SOrder).toHaveBeenCalledWith({
        product: 'license',
        license_id: undefined,
        provider: 'stripe',
      })
      expect(apiMocks.createD2SOrder).toHaveBeenCalledWith(
        expect.objectContaining({
          product: 'license',
          license_id: undefined,
          provider: 'stripe',
          balance_minor: 0,
        })
      )
    })
    expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledWith('order-1')
  })
})
