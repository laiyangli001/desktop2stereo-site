import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { PaymentSettingsSection } from '../integrations/payment-settings-section'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
afterEach(() => vi.clearAllMocks())

it('manages Alipay alongside other payment gateways using the system settings API and a separate form', async () => {
  const settings = {
    qr_pay_mode: '4',
    config_source: 'admin',
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
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: settings },
  })
  vi.mocked(api.put).mockResolvedValue({
    data: { success: true, data: settings },
  })
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const { container } = render(
    <QueryClientProvider client={queryClient}>
      <PaymentSettingsSection
        defaultValues={{
          PayAddress: '',
          EpayId: '',
          EpayKey: '',
          Price: 1,
          MinTopUp: 1,
          CustomCallbackAddress: '',
          PayMethods: '[]',
          AmountOptions: '[]',
          AmountDiscount: '{}',
          StripeApiSecret: '',
          StripeWebhookSecret: '',
          StripePriceId: '',
          StripeUnitPrice: 1,
          StripeMinTopUp: 1,
          StripePromotionCodesEnabled: false,
          CreemApiKey: '',
          CreemWebhookSecret: '',
          CreemTestMode: false,
          CreemProducts: '[]',
        }}
        waffoDefaultValues={{
          WaffoEnabled: false,
          WaffoApiKey: '',
          WaffoPrivateKey: '',
          WaffoPublicCert: '',
          WaffoSandboxPublicCert: '',
          WaffoSandboxApiKey: '',
          WaffoSandboxPrivateKey: '',
          WaffoSandbox: false,
          WaffoMerchantId: '',
          WaffoCurrency: 'USD',
          WaffoUnitPrice: 1,
          WaffoMinTopUp: 1,
          WaffoNotifyUrl: '',
          WaffoReturnUrl: '',
          WaffoPayMethods: '[]',
        }}
        waffoPancakeDefaultValues={{
          WaffoPancakeMerchantID: '',
          WaffoPancakePrivateKey: '',
          WaffoPancakeReturnURL: '',
        }}
        complianceDefaults={{
          confirmed: true,
          termsVersion: 'v1',
          confirmedAt: 1,
          confirmedBy: 1,
        }}
      />
    </QueryClientProvider>
  )
  expect(screen.getByRole('tab', { name: 'Epay' })).toBeInTheDocument()
  expect(screen.getByRole('tab', { name: 'Stripe' })).toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Epay' }))
  await user.type(screen.getByLabelText('Epay merchant ID'), 'merchant-draft')
  await user.click(screen.getByRole('tab', { name: 'Alipay' }))
  expect(await screen.findByLabelText('Alipay app ID')).toHaveValue(
    settings.app_id
  )
  expect(api.get).toHaveBeenCalledWith('/api/option/alipay/settings')
  expect(container.querySelector('form form')).toBeNull()
  expect(screen.getByLabelText(/Merchant RSA2 private key/)).toHaveValue('')
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'QR display mode' }),
    '1'
  )
  await user.click(screen.getByRole('button', { name: 'Save Alipay settings' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      '/api/option/alipay/config',
      expect.objectContaining({
        qr_pay_mode: '1',
        app_id: settings.app_id,
        merchant_private_key: undefined,
        alipay_public_key: undefined,
      })
    )
  )
  await user.click(screen.getByRole('tab', { name: 'Epay' }))
  expect(screen.getByLabelText('Epay merchant ID')).toHaveValue(
    'merchant-draft'
  )
})
