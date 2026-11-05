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

import { D2SStore } from '../store'

const apiMocks = vi.hoisted(() => ({
  getD2SBalance: vi.fn(),
  getD2SLicenses: vi.fn(),
  getD2SCheckoutProviders: vi.fn(),
  getD2SStoreProducts: vi.fn(),
  previewD2SOrder: vi.fn(),
  createD2SOrder: vi.fn(),
  createD2SOrderCheckout: vi.fn(),
}))
vi.mock('../api', () => apiMocks)
function response<T>(data: T) {
  return { success: true, data }
}
function renderStore() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <D2SStore />
    </QueryClientProvider>
  )
}
async function reviewOrder() {
  const button = await screen.findByRole('button', { name: 'Review order' })
  await waitFor(() => expect(button).toBeEnabled())
  fireEvent.click(button)
  await screen.findByRole('button', { name: 'Confirm and pay' })
  return screen.getByRole('complementary', { name: 'Order summary' })
}
const catalog = {
  products: [
    { id: 'license', prices: { CNY: 9900, USD: 2990 }, starting_at: false },
    { id: 'paid_revoke', prices: { CNY: 1990, USD: 299 }, starting_at: true },
    {
      id: 'offline_extension',
      prices: { CNY: 1990, USD: 299 },
      starting_at: false,
    },
  ],
}

describe('D2S unified store purchase flow', () => {
  const initialLanguage = i18n.language
  beforeEach(() => {
    vi.resetAllMocks()
    apiMocks.getD2SStoreProducts.mockResolvedValue(response(catalog))
    apiMocks.getD2SBalance.mockResolvedValue(
      response({
        accounts: [
          { id: 'usd', currency: 'USD', available_minor: 0, reserved_minor: 0 },
        ],
      })
    )
    apiMocks.getD2SLicenses.mockResolvedValue(response({ licenses: [] }))
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ enabled: true, providers: ['stripe'] })
    )
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
  afterEach(async () => {
    await i18n.changeLanguage(initialLanguage)
    vi.restoreAllMocks()
    document
      .querySelectorAll('form[action="https://payments.example/checkout"]')
      .forEach((form) => form.remove())
    document
      .querySelectorAll('[role="dialog"][aria-label="Pay with Alipay"]')
      .forEach((dialog) => dialog.remove())
  })

  it('reviews the exact price and confirms payment on the same page without creating an order while browsing', async () => {
    renderStore()
    expect(
      await screen.findByRole('radio', { name: 'Full license' })
    ).toBeChecked()
    expect(apiMocks.previewD2SOrder).not.toHaveBeenCalled()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
    const summary = await reviewOrder()
    expect(within(summary).getByText('USD 29.90')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
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
    )
    expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledWith('order-1')
  })

  it('formats exact prices with the zhCN interface locale', async () => {
    await i18n.changeLanguage('zhCN')
    renderStore()
    const summary = await reviewOrder()
    expect(within(summary).getByText(/USD 29[.,]90/)).toBeInTheDocument()
  })

  it('uses the full quoted balance amount without starting an external checkout', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ enabled: true, providers: ['balance'] })
    )
    apiMocks.getD2SBalance.mockResolvedValue(
      response({ accounts: [{ currency: 'CNY', available_minor: 10000 }] })
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
    renderStore()
    const summary = await reviewOrder()
    expect(within(summary).getByText('CNY 99.00')).toBeInTheDocument()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
      expect(apiMocks.createD2SOrder).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: 'balance',
          balance_minor: 9900,
          expected_amount_minor: 9900,
          expected_currency: 'CNY',
          expected_region: 'CN',
        })
      )
    )
    expect(apiMocks.createD2SOrderCheckout).not.toHaveBeenCalled()
  })

  it('cancels a reviewed order without creating a transaction', async () => {
    renderStore()
    const summary = await reviewOrder()
    fireEvent.click(within(summary).getByRole('button', { name: 'Cancel' }))
    expect(
      within(summary).getByRole('button', { name: 'Review order' })
    ).toBeInTheDocument()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('passes the chosen eligible license when confirming paid revoke', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ enabled: true, providers: ['creem'] })
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
            expires_at: 0,
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
    renderStore()
    fireEvent.click(await screen.findByRole('radio', { name: 'Paid revoke' }))
    await screen.findByRole('option', { name: 'D2S-REVOKE-1' })
    fireEvent.change(screen.getByRole('combobox', { name: 'License' }), {
      target: { value: 'license-revoke' },
    })
    const summary = await reviewOrder()
    expect(within(summary).getByText('D2S-REVOKE-1')).toBeInTheDocument()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
      expect(apiMocks.createD2SOrder).toHaveBeenCalledWith(
        expect.objectContaining({
          product: 'paid_revoke',
          license_id: 'license-revoke',
          provider: 'creem',
          expected_amount_minor: 299,
        })
      )
    )
  })

  it('prioritizes and defaults to Alipay while allowing the other supported payment methods', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({
        enabled: true,
        providers: [
          'paypal',
          'stripe',
          'alipay',
          'alipay_pc',
          'paymentfm',
          'wechat',
          'waffo',
          'waffo_pancake',
          'creem',
          'paddle',
        ],
      })
    )
    renderStore()
    const group = await screen.findByRole('group', { name: 'Payment method' })
    await screen.findByRole('radio', { name: 'Alipay (PC)' })
    const radios = within(group).getAllByRole('radio')
    expect(radios[0]).toHaveAccessibleName('Alipay (PC)')
    expect(radios[0]).toBeChecked()
    for (const name of [
      'Alipay',
      'Payment FM',
      'WeChat',
      'Waffo',
      'Waffo Pancake',
      'Stripe',
      'Creem',
    ]) {
      expect(within(group).getByRole('radio', { name })).toBeEnabled()
    }
    expect(
      screen.queryByRole('radio', { name: /PayPal|Paddle/i })
    ).not.toBeInTheDocument()
    fireEvent.click(within(group).getByRole('radio', { name: 'Stripe' }))
    expect(within(group).getByRole('radio', { name: 'Stripe' })).toBeChecked()
  })

  it('restores review controls when quoting fails without creating an order', async () => {
    apiMocks.previewD2SOrder.mockRejectedValue(new Error('unavailable'))
    renderStore()
    const button = await screen.findByRole('button', { name: 'Review order' })
    await waitFor(() => expect(button).toBeEnabled())
    fireEvent.click(button)
    await waitFor(() => {
      expect(apiMocks.previewD2SOrder).toHaveBeenCalledOnce()
      expect(button).toBeEnabled()
    })
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('submits signed POST checkout fields through the existing checkout helper', async () => {
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
    renderStore()
    const summary = await reviewOrder()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() => expect(submit).toHaveBeenCalledOnce())
    const form = document.querySelector(
      'form[action="https://payments.example/checkout"]'
    )
    expect(form).toHaveAttribute('method', 'POST')
    expect(form?.querySelector('input[name="order_id"]')).toHaveValue('order-1')
    expect(form?.querySelector('input[name="signature"]')).toHaveValue(
      'test-signature'
    )
  })

  it('prevents competing selections and duplicate payment requests during checkout', async () => {
    renderStore()
    const summary = await reviewOrder()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
      expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledOnce()
    )
    for (const radio of screen.getAllByRole('radio'))
      {expect(radio).toBeDisabled()}
    expect(
      within(summary).getByRole('button', { name: 'Processing…' })
    ).toBeDisabled()
  })

  it('discards an expired price snapshot and requires a fresh review', async () => {
    apiMocks.createD2SOrder.mockRejectedValue({
      isAxiosError: true,
      response: { data: { error: { code: 'quote_changed' } } },
    })
    renderStore()
    const summary = await reviewOrder()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
      expect(
        within(summary).getByRole('button', { name: 'Review order' })
      ).toBeEnabled()
    )
    expect(apiMocks.createD2SOrderCheckout).not.toHaveBeenCalled()
  })

  it('keeps price cards visible while purchases are closed and blocks all ordering', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ enabled: false, providers: [] })
    )
    renderStore()
    expect(
      await screen.findByRole('radio', { name: 'Full license' })
    ).toBeEnabled()
    await screen.findByText('Purchases are currently unavailable.')
    expect(screen.getByRole('button', { name: 'Review order' })).toBeDisabled()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('requires a new review after changing payment method', async () => {
    apiMocks.getD2SCheckoutProviders.mockResolvedValue(
      response({ enabled: true, providers: ['stripe', 'creem'] })
    )
    renderStore()
    await reviewOrder()
    fireEvent.click(screen.getByRole('radio', { name: 'Creem' }))
    expect(
      screen.getByRole('button', { name: 'Review order' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Confirm and pay' })
    ).not.toBeInTheDocument()
  })

  it('does not allow checkout when product prices cannot be loaded', async () => {
    apiMocks.getD2SStoreProducts.mockRejectedValue(
      new Error('catalog unavailable')
    )
    renderStore()
    await screen.findByRole('alert')
    expect(screen.getByRole('button', { name: 'Review order' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
  it('retries checkout with the same idempotency key after a network error', async () => {
    apiMocks.createD2SOrderCheckout.mockRejectedValue(
      new Error('checkout unavailable')
    )
    renderStore()
    const summary = await reviewOrder()
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() => {
      expect(apiMocks.createD2SOrderCheckout).toHaveBeenCalledOnce()
      expect(
        within(summary).getByRole('button', { name: 'Confirm and pay' })
      ).toBeEnabled()
    })
    fireEvent.click(
      within(summary).getByRole('button', { name: 'Confirm and pay' })
    )
    await waitFor(() =>
      expect(apiMocks.createD2SOrder).toHaveBeenCalledTimes(2)
    )
    expect(apiMocks.createD2SOrder.mock.calls[0][0].idempotency_key).toBe(
      apiMocks.createD2SOrder.mock.calls[1][0].idempotency_key
    )
  })

  it('disables competing choices while a price quote is pending', async () => {
    apiMocks.previewD2SOrder.mockReturnValue(new Promise(() => {}))
    renderStore()
    const review = await screen.findByRole('button', { name: 'Review order' })
    await waitFor(() => expect(review).toBeEnabled())
    fireEvent.click(review)
    await waitFor(() => expect(apiMocks.previewD2SOrder).toHaveBeenCalledOnce())
    for (const radio of screen.getAllByRole('radio'))
      {expect(radio).toBeDisabled()}
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })

  it('discards the previous quote when selecting a different product', async () => {
    renderStore()
    await reviewOrder()
    fireEvent.click(screen.getByRole('radio', { name: 'Offline extension' }))
    expect(
      screen.queryByRole('button', { name: 'Confirm and pay' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Review order' })).toBeDisabled()
    expect(apiMocks.createD2SOrder).not.toHaveBeenCalled()
  })
})
