import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { D2SWalletSection } from '..'

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
      <D2SWalletSection />
    </QueryClientProvider>
  )
}

describe('D2S wallet purchase flow', () => {
  const initialLanguage = i18n.language

  afterEach(async () => {
    await i18n.changeLanguage(initialLanguage)
    vi.restoreAllMocks()
    document
      .querySelector('form[action="https://payments.example/checkout"]')
      ?.remove()
  })

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

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('USD 29.90')).toBeInTheDocument()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

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
          expected_amount_minor: 2990,
          expected_currency: 'USD',
          expected_region: 'INTL',
        })
      )
    })
    expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledWith('order-1')
  })

  it('formats the purchase quote with the zhCN interface locale', async () => {
    await i18n.changeLanguage('zhCN')
    renderWorkspace()

    fireEvent.click(await screen.findByRole('button', { name: /Stripe/ }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/USD 29[.,]90/)).toBeInTheDocument()
  })

  it('previews a balance order and submits the quoted amount without checkout redirect', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['balance'] })
    )
    apiMocks.getD2SBalance.mockResolvedValue(
      response({
        accounts: [
          {
            id: 'balance-cny',
            currency: 'CNY',
            available_minor: 10000,
            reserved_minor: 0,
          },
        ],
      })
    )
    apiMocks.previewD2SOrder.mockResolvedValue(
      response({
        product: 'license',
        provider: 'balance',
        region: 'CN',
        currency: 'CNY',
        amount_minor: 9900,
      })
    )

    renderWorkspace()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Buy with balance' })
    )

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('CNY 99.00')).toBeInTheDocument()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(apiMocks.previewD2SOrder).toHaveBeenCalledWith({
        product: 'license',
        license_id: undefined,
        provider: 'balance',
      })
      expect(apiMocks.createD2SOrder).toHaveBeenCalledWith(
        expect.objectContaining({
          product: 'license',
          provider: 'balance',
          balance_minor: 9900,
          expected_amount_minor: 9900,
          expected_currency: 'CNY',
          expected_region: 'CN',
        })
      )
    })
    expect(apiMocks.createD2SOrderCheckout).not.toHaveBeenCalled()
  })

  it('does not create a balance order when the quoted purchase is canceled', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['balance'] })
    )
    apiMocks.getD2SBalance.mockResolvedValue(
      response({
        accounts: [
          { id: 'balance-cny', currency: 'CNY', available_minor: 10000 },
        ],
      })
    )
    apiMocks.previewD2SOrder.mockResolvedValue(
      response({
        product: 'license',
        provider: 'balance',
        region: 'CN',
        currency: 'CNY',
        amount_minor: 9900,
      })
    )

    renderWorkspace()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Buy with balance' })
    )
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('passes the selected license when starting a paid revoke order', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['creem'] })
    )
    apiMocks.getD2SLicenses.mockResolvedValue(
      response({
        licenses: [
          {
            id: 'license-revoke',
            license_code: 'D2S-REVOKE-1',
            kind: 'paid',
            status: 'active',
            mode: 'online',
            device_hash: 'a'.repeat(64),
            fingerprint_version: 1,
          },
        ],
      })
    )
    apiMocks.previewD2SOrder.mockResolvedValue(
      response({
        product: 'paid_revoke',
        provider: 'creem',
        region: 'INTL',
        currency: 'USD',
        amount_minor: 299,
      })
    )

    renderWorkspace()
    fireEvent.change(screen.getByRole('combobox', { name: 'Purchase type' }), {
      target: { value: 'paid_revoke' },
    })
    await screen.findByRole('option', {
      name: 'D2S-REVOKE-1',
    })
    fireEvent.change(screen.getByRole('combobox', { name: 'License' }), {
      target: { value: 'license-revoke' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Pay with Creem' })
    )

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('D2S-REVOKE-1')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(apiMocks.previewD2SOrder).toHaveBeenCalledWith({
        product: 'paid_revoke',
        license_id: 'license-revoke',
        provider: 'creem',
      })
      expect(apiMocks.createD2SOrder).toHaveBeenCalledWith(
        expect.objectContaining({
          product: 'paid_revoke',
          license_id: 'license-revoke',
          provider: 'creem',
          expected_amount_minor: 299,
          expected_currency: 'USD',
          expected_region: 'INTL',
        })
      )
    })
  })

  it('renders every supported checkout provider returned by the server', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({
        providers: [
          'alipay',
          'paymentfm',
          'wechat',
          'waffo',
          'waffo_pancake',
          'stripe',
          'creem',
        ],
      })
    )

    renderWorkspace()

    for (const name of [
      'Pay with Alipay',
      'Pay with Payment FM',
      'Pay with WeChat',
      'Pay with Waffo',
      'Pay with Waffo Pancake',
      'Pay with Stripe',
      'Pay with Creem',
    ]) {
      expect(await screen.findByRole('button', { name })).toBeEnabled()
    }
  })

  it('restores checkout controls when server price preview fails', async () => {
    apiMocks.previewD2SOrder.mockRejectedValue(new Error('preview unavailable'))

    renderWorkspace()
    const payButton = await screen.findByRole('button', {
      name: 'Pay with Stripe',
    })
    fireEvent.click(payButton)

    await waitFor(() => {
      expect(apiMocks.previewD2SOrder).toHaveBeenCalledOnce()
      expect(
        screen.getByRole('button', { name: 'Pay with Stripe' })
      ).toBeEnabled()
    })
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('never renders unsupported PayPal or Paddle checkout controls', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['paypal', 'paddle', 'stripe'] })
    )

    renderWorkspace()

    expect(
      await screen.findByRole('button', { name: 'Pay with Stripe' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Pay with PayPal/i })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Pay with Paddle/i })
    ).not.toBeInTheDocument()
  })

  it('submits POST checkout parameters through a generated form', async () => {
    const submit = vi
      .spyOn(HTMLFormElement.prototype, 'submit')
      .mockImplementation(() => undefined)
    apiMocks.createD2SOrderCheckout.mockResolvedValue(
      response({
        checkout_url: 'https://payments.example/checkout',
        method: 'POST',
        params: { order_id: 'order-1', signature: 'test-signature' },
      })
    )

    renderWorkspace()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Pay with Stripe' })
    )

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() => expect(submit).toHaveBeenCalledOnce())
    const form = document.querySelector(
      'form[action="https://payments.example/checkout"]'
    )
    expect(form).not.toBeNull()
    expect(form?.getAttribute('method')).toBe('POST')
    expect(
      (form?.querySelector('input[name="order_id"]') as HTMLInputElement)?.value
    ).toBe('order-1')
    expect(
      (form?.querySelector('input[name="signature"]') as HTMLInputElement)
        ?.value
    ).toBe('test-signature')
  })

  it('marks the purchase controls busy and disables competing checkouts while pending', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['balance', 'stripe'] })
    )
    renderWorkspace()

    const balanceButton = await screen.findByRole('button', {
      name: 'Buy with balance',
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Pay with Stripe' })
    )

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledWith('order-1')
    )
    expect(
      within(dialog).getByRole('button', { name: 'Confirm' })
    ).toBeDisabled()
    expect(balanceButton).toBeDisabled()
    expect(
      screen.getByRole('combobox', { name: 'Purchase type', hidden: true })
    ).toBeDisabled()
    expect(balanceButton.closest('[aria-busy="true"]')).not.toBeNull()
  })

  it('disables external checkout while a balance order is being priced', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ providers: ['balance', 'stripe'] })
    )
    apiMocks.previewD2SOrder.mockReturnValue(new Promise(() => {}))
    renderWorkspace()

    const stripeButton = await screen.findByRole('button', {
      name: 'Pay with Stripe',
    })
    fireEvent.click(screen.getByRole('button', { name: 'Buy with balance' }))

    await waitFor(() => expect(apiMocks.previewD2SOrder).toHaveBeenCalledOnce())
    expect(stripeButton).toBeDisabled()
    expect(
      screen.getByRole('combobox', { name: 'Purchase type' })
    ).toBeDisabled()
  })

  it('allows checkout retry with the same order key when checkout creation fails', async () => {
    apiMocks.createD2SOrderCheckout.mockRejectedValue(
      new Error('checkout unavailable')
    )

    renderWorkspace()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Pay with Stripe' })
    )

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() => {
      expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledWith('order-1')
      expect(
        within(dialog).getByRole('button', { name: 'Confirm' })
      ).toBeEnabled()
    })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(apiMocks.createD2SOrder).toHaveBeenCalledTimes(2)
    )
    const firstKey = apiMocks.createD2SOrder.mock.calls[0][0].idempotency_key
    const retryKey = apiMocks.createD2SOrder.mock.calls[1][0].idempotency_key
    expect(firstKey).toBe(retryKey)
  })
})
