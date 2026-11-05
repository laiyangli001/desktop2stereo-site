import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { ShoppingBag } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import {
  getD2SBalance,
  getD2SLicenses,
  getD2SCheckoutProviders,
  getD2SStoreProducts,
  previewD2SOrder,
  createD2SOrder,
  createD2SOrderCheckout,
} from './api'
import { StoreProductCard } from './components/store-product-card'
import { STORE_PRODUCT_CONTENT } from './lib/store-products'
import { submitD2SCheckout } from './lib/checkout'
import { formatD2SMoney } from './lib/format'

type D2SPurchaseProduct = 'license' | 'paid_revoke' | 'offline_extension'
type D2SPurchaseProvider =
  | 'balance'
  | 'stripe'
  | 'creem'
  | 'waffo_pancake'
  | 'waffo'
  | 'paymentfm'
  | 'alipay'
  | 'alipay_pc'
  | 'wechat'

type D2SQuotedPurchase = {
  product: D2SPurchaseProduct
  provider: D2SPurchaseProvider
  licenseID?: string
  amountMinor: number
  currency: string
  region: string
  idempotencyKey: string
}

function isD2SQuoteChanged(error: unknown): boolean {
  return (
    axios.isAxiosError<{ error?: { code?: string } }>(error) &&
    error.response?.data?.error?.code === 'quote_changed'
  )
}

const D2S_PROVIDER_NAMES: Record<D2SPurchaseProvider, string> = {
  balance: 'Balance',
  alipay: 'Alipay',
  alipay_pc: 'Alipay (PC)',
  paymentfm: 'Payment FM',
  wechat: 'WeChat',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
  stripe: 'Stripe',
  creem: 'Creem',
}

const supportedProviders = Object.keys(D2S_PROVIDER_NAMES)

export function D2SStore() {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const queryClient = useQueryClient()
  const [checkedAt] = useState(() => Date.now() / 1000)
  const [product, setProduct] = useState<D2SPurchaseProduct>('license')
  const [licenseID, setLicenseID] = useState('')
  const [providerChoice, setProviderChoice] = useState('')
  const [pendingQuote, setPendingQuote] = useState<D2SQuotedPurchase | null>(
    null
  )
  const catalog = useQuery({
    queryKey: ['d2s', 'store-products'],
    queryFn: getD2SStoreProducts,
  })
  const licenses = useQuery({
    queryKey: ['d2s', 'licenses'],
    queryFn: getD2SLicenses,
  })
  const checkoutProviders = useQuery({
    queryKey: ['d2s', 'checkout-providers'],
    queryFn: getD2SCheckoutProviders,
  })
  const balance = useQuery({
    queryKey: ['d2s', 'balance'],
    queryFn: getD2SBalance,
  })
  const enabled =
    checkoutProviders.data?.success === true &&
    checkoutProviders.data.data?.enabled === true
  const providers = (
    enabled ? (checkoutProviders.data?.data?.providers ?? []) : []
  )
    .filter(
      (value, index, values) =>
        supportedProviders.includes(value) && values.indexOf(value) === index
    )
    .sort((a, b) => Number(b === 'alipay_pc') - Number(a === 'alipay_pc'))
  const provider = providers.includes(providerChoice)
    ? providerChoice
    : (providers[0] ?? '')
  const accounts = balance.data?.data?.accounts ?? []
  let currency: 'CNY' | 'USD' = 'CNY'
  if (['stripe', 'creem', 'waffo_pancake'].includes(provider)) currency = 'USD'
  if (
    provider === 'balance' &&
    accounts.length === 1 &&
    accounts[0].currency === 'USD'
  ) {
    currency = 'USD'
  }
  const eligibleLicenses = (licenses.data?.data?.licenses ?? []).filter(
    (license) =>
      license.status === 'active' &&
      license.mode !== 'permanent' &&
      (!license.expires_at || license.expires_at > checkedAt) &&
      (product !== 'paid_revoke' || Boolean(license.device_hash))
  )
  const targetMissing =
    product !== 'license' &&
    !eligibleLicenses.some((license) => license.id === licenseID)
  const quote =
    pendingQuote?.product === product &&
    pendingQuote.provider === provider &&
    (product === 'license' || pendingQuote.licenseID === licenseID)
      ? pendingQuote
      : null
  const products = catalog.data?.data?.products ?? []
  const selectedProduct = products.find((row) => row.id === product)
  const productContent = STORE_PRODUCT_CONTENT[product]
  const catalogReady =
    catalog.isSuccess && catalog.data?.success === true && products.length > 0

  const quoteMutation = useMutation({
    mutationFn: async (request: {
      product: D2SPurchaseProduct
      provider: D2SPurchaseProvider
      licenseID?: string
    }): Promise<D2SQuotedPurchase> => {
      const response = await previewD2SOrder({
        product: request.product,
        provider: request.provider,
        license_id: request.licenseID,
      })
      const data = response.data
      if (
        !response.success ||
        !data ||
        data.product !== request.product ||
        data.provider !== request.provider ||
        !Number.isSafeInteger(data.amount_minor) ||
        data.amount_minor <= 0 ||
        !['CNY', 'USD'].includes(data.currency) ||
        !['CN', 'INTL'].includes(data.region)
      ) {
        throw new Error('quote unavailable')
      }
      if (request.provider === 'balance') {
        const account = accounts.find((row) => row.currency === data.currency)
        if (!account || account.available_minor < data.amount_minor) {
          throw new Error('insufficient balance')
        }
      }
      return {
        ...request,
        amountMinor: data.amount_minor,
        currency: data.currency,
        region: data.region,
        idempotencyKey: `d2s-${globalThis.crypto.randomUUID()}`,
      }
    },
    onSuccess: setPendingQuote,
    onError: (error) =>
      toast.error(
        t(
          error.message === 'insufficient balance'
            ? 'Insufficient balance'
            : 'Unable to start checkout'
        )
      ),
  })
  const payment = useMutation({
    mutationFn: async (request: D2SQuotedPurchase) => {
      const created = await createD2SOrder({
        product: request.product,
        license_id: request.licenseID,
        provider: request.provider,
        balance_minor: request.provider === 'balance' ? request.amountMinor : 0,
        expected_amount_minor: request.amountMinor,
        expected_currency: request.currency,
        expected_region: request.region,
        idempotency_key: request.idempotencyKey,
      })
      if (!created.success || !created.data) {
        throw new Error(created.error?.message || 'order creation failed')
      }
      if (request.provider === 'balance') return null
      const checkout = await createD2SOrderCheckout(created.data.id)
      if (!checkout.success || !checkout.data?.checkout_url) {
        throw new Error('checkout unavailable')
      }
      return checkout.data
    },
    onSuccess: (checkout) => {
      if (checkout) {
        submitD2SCheckout(checkout, {
          title: t('Pay with Alipay'),
          close: t('Close'),
          fallback: t('Open Alipay in a new tab'),
          help: t(
            'Scan the Alipay QR code here. If it does not appear, open Alipay in a new tab.'
          ),
        })
      } else toast.success(t('License purchased'))
      setPendingQuote(null)
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
    },
    onError: (error) => {
      if (isD2SQuoteChanged(error)) {
        setPendingQuote(null)
        toast.error(
          t('Price changed; review the latest quote before purchasing.')
        )
      } else toast.error(t('Unable to start checkout'))
    },
  })
  const busy = quoteMutation.isPending || payment.isPending
  const reviewDisabled =
    !enabled ||
    !provider ||
    targetMissing ||
    !catalogReady ||
    busy ||
    (provider === 'balance' && (balance.isPending || balance.isError))
  const estimated = selectedProduct?.prices[currency]
  let exactOrEstimated = '—'
  if (quote) {
    exactOrEstimated = formatD2SMoney(quote.amountMinor, quote.currency, locale)
  } else if (Number.isSafeInteger(estimated) && (estimated ?? 0) > 0) {
    exactOrEstimated = formatD2SMoney(estimated ?? 0, currency, locale)
  }
  let paymentButtonLabel = t('Review order')
  if (quote) paymentButtonLabel = t('Confirm and pay')
  if (busy) paymentButtonLabel = t('Processing…')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Store')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-7xl space-y-6 pb-20'>
          <header className='rounded-xl border bg-gradient-to-br from-primary/10 via-background to-background p-5 sm:p-6'>
            <div className='flex items-center gap-3'>
              <span className='bg-primary/10 text-primary flex size-10 items-center justify-center rounded-xl'>
                <ShoppingBag className='size-5' aria-hidden='true' />
              </span>
              <div>
                <h2 className='text-lg font-semibold'>
                  {t('Desktop2Stereo Store')}
                </h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(
                    'Select a product, review your order and pay on this page.'
                  )}
                </p>
              </div>
            </div>
          </header>

          {catalog.isPending && <p role='status'>{t('Loading')}</p>}
          {!catalog.isPending && !catalogReady && (
            <div
              role='alert'
              className='space-y-3 rounded-lg border border-destructive/30 p-4'
            >
              <p>{t('Unable to load store products')}</p>
              <Button variant='outline' onClick={() => void catalog.refetch()}>
                {t('Retry')}
              </Button>
            </div>
          )}
          {catalogReady && (
            <fieldset disabled={busy} className='min-w-0'>
              <legend className='mb-4 text-base font-semibold'>
                {t('Choose a product')}
              </legend>
              <div className='grid min-w-0 gap-4 lg:grid-cols-3'>
                {products
                  .filter((row) => Object.hasOwn(STORE_PRODUCT_CONTENT, row.id))
                  .map((row) => (
                    <StoreProductCard
                      key={row.id}
                      product={row}
                      selected={row.id === product}
                      disabled={busy}
                      currency={currency}
                      onSelect={() => {
                        setProduct(row.id)
                        setPendingQuote(null)
                      }}
                    />
                  ))}
              </div>
            </fieldset>
          )}

          <div className='grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]'>
            <Card className='min-w-0'>
              <CardHeader>
                <CardTitle>{t('Purchase details')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-6' aria-busy={busy}>
                {product !== 'license' && (
                  <div className='space-y-3'>
                    <label
                      htmlFor='store-license'
                      className='text-sm font-medium'
                    >
                      {t('License')}
                    </label>
                    <select
                      id='store-license'
                      className='h-10 w-full min-w-0 rounded-md border bg-background px-3 text-sm focus-visible:ring-2 focus-visible:ring-ring'
                      value={licenseID}
                      disabled={busy || licenses.isPending || licenses.isError}
                      onChange={(event) => {
                        setLicenseID(event.target.value)
                        setPendingQuote(null)
                      }}
                    >
                      <option value=''>{t('Select a license')}</option>
                      {eligibleLicenses.map((license) => (
                        <option key={license.id} value={license.id}>
                          {license.license_code}
                        </option>
                      ))}
                    </select>
                    {!licenses.isPending && eligibleLicenses.length === 0 && (
                      <p
                        role='status'
                        className='text-muted-foreground text-sm'
                      >
                        {t('No eligible license for this product.')}
                      </p>
                    )}
                  </div>
                )}
                {licenses.isError && (
                  <p role='alert' className='text-destructive text-sm'>
                    {t('Unable to load Desktop2Stereo data')}
                  </p>
                )}
                <fieldset disabled={busy} className='min-w-0 space-y-3'>
                  <legend className='mb-3 text-sm font-medium'>
                    {t('Payment method')}
                  </legend>
                  {checkoutProviders.isPending && (
                    <p role='status'>{t('Loading')}</p>
                  )}
                  {checkoutProviders.isError && (
                    <p role='alert' className='text-destructive text-sm'>
                      {t('Unable to load payment methods')}
                    </p>
                  )}
                  {!checkoutProviders.isPending && !enabled && (
                    <p
                      role='status'
                      className='text-muted-foreground rounded-lg bg-muted/40 p-4 text-sm'
                    >
                      {t('Purchases are currently unavailable.')}
                    </p>
                  )}
                  {enabled && providers.length === 0 && (
                    <p role='status' className='text-muted-foreground text-sm'>
                      {t('No payment method is currently available.')}
                    </p>
                  )}
                  <div className='grid gap-3 sm:grid-cols-2'>
                    {providers.map((value) => (
                      <label
                        key={value}
                        className={`flex cursor-pointer items-center gap-3 rounded-lg border p-4 focus-within:ring-2 focus-within:ring-ring ${provider === value ? 'border-primary bg-primary/5' : 'hover:bg-muted/30'}`}
                      >
                        <input
                          type='radio'
                          name='store-payment'
                          value={value}
                          checked={provider === value}
                          aria-label={t(
                            D2S_PROVIDER_NAMES[value as D2SPurchaseProvider]
                          )}
                          onChange={() => {
                            setProviderChoice(value)
                            setPendingQuote(null)
                          }}
                          className='size-4 accent-primary'
                          disabled={busy}
                        />
                        {value === 'alipay_pc' && (
                          <img
                            src='/alipay-logo-square.png'
                            alt=''
                            className='size-7 rounded-md'
                          />
                        )}
                        <span className='text-sm font-medium'>
                          {t(D2S_PROVIDER_NAMES[value as D2SPurchaseProvider])}
                        </span>
                        {value === 'alipay_pc' && (
                          <Badge variant='secondary' className='ml-auto'>
                            {t('Recommended')}
                          </Badge>
                        )}
                      </label>
                    ))}
                  </div>
                  {providers.includes('balance') && (
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Balance pays the full amount and cannot be combined with another payment method.'
                      )}
                    </p>
                  )}
                </fieldset>
                <div className='space-y-2 rounded-lg bg-muted/30 p-4 text-sm'>
                  <h3 className='font-medium'>{t('Purchase notes')}</h3>
                  <p className='text-muted-foreground leading-6'>
                    {t(productContent.description)}
                  </p>
                  <p className='text-muted-foreground leading-6'>
                    {t(
                      'Final price is confirmed before payment. No order is created until you confirm.'
                    )}
                  </p>
                </div>
              </CardContent>
            </Card>

            <aside
              aria-label={t('Order summary')}
              className='min-w-0 xl:sticky xl:top-4'
            >
              <Card className='min-w-0 overflow-hidden border-primary/20'>
                <CardHeader className='border-b bg-muted/30'>
                  <CardTitle>{t('Order summary')}</CardTitle>
                </CardHeader>
                <CardContent className='space-y-5 pt-5'>
                  <dl className='space-y-4 text-sm'>
                    <div className='space-y-1'>
                      <dt className='text-muted-foreground'>{t('Product')}</dt>
                      <dd className='font-medium'>{t(productContent.title)}</dd>
                    </div>
                    {product !== 'license' && (
                      <div className='space-y-1'>
                        <dt className='text-muted-foreground'>
                          {t('License')}
                        </dt>
                        <dd className='break-all font-mono text-xs'>
                          {eligibleLicenses.find(
                            (license) => license.id === licenseID
                          )?.license_code || t('Select a license')}
                        </dd>
                      </div>
                    )}
                    <div className='flex justify-between gap-3'>
                      <dt className='text-muted-foreground'>{t('Payment')}</dt>
                      <dd>
                        {provider
                          ? t(
                              D2S_PROVIDER_NAMES[
                                provider as D2SPurchaseProvider
                              ]
                            )
                          : '—'}
                      </dd>
                    </div>
                  </dl>
                  <div className='space-y-2 border-t pt-5' aria-live='polite'>
                    <p className='text-muted-foreground text-sm'>
                      {quote ? t('Amount to pay:') : t('Estimated price')}
                    </p>
                    <p className='break-words text-3xl font-semibold tracking-tight tabular-nums'>
                      {exactOrEstimated}
                    </p>
                    {!quote && selectedProduct?.starting_at && (
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Tiered pricing; review the exact quote before payment'
                        )}
                      </p>
                    )}
                    {quote && (
                      <p
                        role='status'
                        className='text-muted-foreground text-xs'
                      >
                        {t('Review this amount before confirming payment.')}
                      </p>
                    )}
                  </div>
                  <Button
                    type='button'
                    className='w-full'
                    disabled={reviewDisabled}
                    onClick={() => {
                      if (quote) payment.mutate(quote)
                      else if (provider) {
                        quoteMutation.mutate({
                          product,
                          provider: provider as D2SPurchaseProvider,
                          licenseID:
                            product === 'license' ? undefined : licenseID,
                        })
                      }
                    }}
                  >
                    {paymentButtonLabel}
                  </Button>
                  {quote && !busy && (
                    <Button
                      type='button'
                      variant='ghost'
                      className='w-full'
                      onClick={() => setPendingQuote(null)}
                    >
                      {t('Cancel')}
                    </Button>
                  )}
                </CardContent>
              </Card>
            </aside>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
