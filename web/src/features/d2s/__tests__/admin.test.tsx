import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { D2SAdminWorkspace } from '../admin'

const apiMocks = vi.hoisted(() => ({
  getD2SAdminBalances: vi.fn(),
  getD2SAdminLicenses: vi.fn(),
  getD2SAdminOrders: vi.fn(),
  getD2SAdminPaymentEvents: vi.fn(),
  getD2SAdminReconciliation: vi.fn(),
  getD2SAdminSigningKeys: vi.fn(),
  getD2SAdminUnbindRequests: vi.fn(),
  getD2SAdminWithdrawals: vi.fn(),
  retireD2SAdminSigningKey: vi.fn(),
  reviewD2SUnbind: vi.fn(),
  reviewD2SWithdrawal: vi.fn(),
  setD2SUserRegion: vi.fn(),
}))

vi.mock('../api', () => apiMocks)

function response<T>(data: T) {
  return { success: true, data }
}

function renderAdminWorkspace() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <D2SAdminWorkspace />
    </QueryClientProvider>
  )
}

describe('D2SAdminWorkspace', () => {
  it('renders operational review and reconciliation data with accessible controls', async () => {
    apiMocks.getD2SAdminLicenses.mockResolvedValue(
      response({
        licenses: [
          {
            id: 'license-1',
            user_id: 42,
            license_code: 'D2S-ADMIN-1',
            status: 'active',
            mode: 'online',
            device_hash:
              '1234567890123456789012345678901234567890123456789012345678901234',
            region: 'INTL',
          },
        ],
      })
    )
    apiMocks.getD2SAdminOrders.mockResolvedValue(
      response({
        orders: [
          {
            id: 'order-chargeback',
            user_id: 42,
            status: 'chargeback',
            provider: 'stripe',
            currency: 'USD',
            amount_minor: 2990,
          },
        ],
      })
    )
    apiMocks.getD2SAdminPaymentEvents.mockResolvedValue(
      response({
        events: [
          {
            id: 'payment-event-1',
            provider: 'stripe',
            provider_event_id: 'stripe-chargeback-1',
            order_id: 'order-chargeback',
            event_type: 'chargeback',
            amount_minor: 2990,
            currency: 'USD',
            processed_at: 1,
          },
        ],
      })
    )
    apiMocks.getD2SAdminBalances.mockResolvedValue(
      response({
        accounts: [
          {
            id: 'balance-1',
            user_id: 42,
            currency: 'USD',
            available_minor: -140,
            reserved_minor: 0,
          },
        ],
      })
    )
    apiMocks.getD2SAdminWithdrawals.mockResolvedValue(
      response({
        withdrawals: [
          {
            id: 'withdrawal-1',
            user_id: 42,
            status: 'pending',
            amount_minor: 5000,
          },
        ],
      })
    )
    apiMocks.getD2SAdminUnbindRequests.mockResolvedValue(
      response({
        requests: [
          {
            id: 'unbind-1',
            user_id: 42,
            status: 'pending',
            reason: 'hardware replacement',
          },
        ],
      })
    )
    apiMocks.getD2SAdminSigningKeys.mockResolvedValue(
      response({
        keys: [
          {
            key_id: 'old-key',
            algorithm: 'ES256',
            status: 'active',
            is_current: false,
          },
        ],
      })
    )
    apiMocks.getD2SAdminReconciliation.mockResolvedValue(
      response({
        orders: 1,
        payment_events: 1,
        paid_orders: 0,
        chargeback_orders: 1,
        pending_expired: 0,
        mismatches: [
          {
            order_id: 'order-chargeback',
            provider_event_id: 'refund-1',
            kind: 'reversal_not_settled',
          },
        ],
      })
    )

    renderAdminWorkspace()

    await waitFor(() => {
      expect(screen.getAllByText(/order-chargeback/).length).toBeGreaterThan(0)
    })
    expect(screen.getByText(/Negative balances/)).toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Order status' })
    ).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'User ID' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Filter orders' })
    ).toBeInTheDocument()
    expect(screen.getByText(/Paid orders: 0/)).toBeInTheDocument()
    expect(screen.getByText(/Pending expired: 0/)).toBeInTheDocument()
    expect(
      screen.getByText(/stripe · chargeback · USD 2990/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/stripe-chargeback-1 · order-chargeback/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Processed at:/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Filter' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: 'Event type' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: 'Order ID' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Status: active · Mode: online/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Device: 123456789012…/)).toBeInTheDocument()
    expect(screen.getByText(/reversal_not_settled/)).toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Region D2S-ADMIN-1' })
    ).toHaveValue('INTL')
    const approveButtons = screen.getAllByRole('button', {
      name: /^Approve /,
    })
    const rejectButtons = screen.getAllByRole('button', {
      name: /^Reject /,
    })
    expect(approveButtons).toHaveLength(2)
    expect(rejectButtons).toHaveLength(2)
    expect(approveButtons).toEqual(
      expect.arrayContaining([expect.objectContaining({ type: 'button' })])
    )
    expect(rejectButtons).toEqual(
      expect.arrayContaining([expect.objectContaining({ type: 'button' })])
    )
    expect(
      screen.getByRole('button', { name: 'Approve withdrawal-1' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Reject unbind-1' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retire' })).toHaveAttribute(
      'type',
      'button'
    )
    expect(
      screen.getByRole('button', { name: 'Save region D2S-ADMIN-1' })
    ).toHaveAttribute('type', 'button')
    expect(
      screen.getAllByRole('textbox', { name: /^Review note / })
    ).toHaveLength(2)

    fireEvent.change(screen.getByRole('combobox', { name: 'Order status' }), {
      target: { value: 'paid' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: 'User ID' }), {
      target: { value: '42' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Filter orders' }))
    await waitFor(() => {
      expect(apiMocks.getD2SAdminOrders).toHaveBeenCalledWith({
        status: 'paid',
        user_id: '42',
      })
    })

    apiMocks.setD2SUserRegion.mockResolvedValue(response({}))
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Region D2S-ADMIN-1' }),
      {
        target: { value: 'CN' },
      }
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Save region D2S-ADMIN-1' })
    )
    await waitFor(() => {
      expect(apiMocks.setD2SUserRegion).toHaveBeenCalledWith(42, 'CN')
      expect(apiMocks.getD2SAdminLicenses).toHaveBeenCalledTimes(2)
    })
  })

  it('announces unavailable admin data instead of presenting an empty dashboard', async () => {
    for (const query of Object.values(apiMocks)) {
      query.mockRejectedValue(new Error('admin API unavailable'))
    }

    renderAdminWorkspace()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load Desktop2Stereo admin data'
    )
  })
})
