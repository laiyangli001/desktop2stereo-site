import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { KeyRound, Monitor, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import {
  getD2SBalance,
  getD2SLicenses,
  getD2SOrders,
  getD2SCheckoutProviders,
  getD2SWithdrawals,
  changeD2SMode,
  confirmD2SPermanent,
  freeRevokeD2SLicense,
  previewD2SOrder,
  createD2SOrder,
  createD2SOrderCheckout,
  createD2SManualUnbind,
  getD2SManualUnbindRequests,
} from './api'
import { LicenseCard } from './components/license-card'
import { WithdrawalRequestForm } from './components/withdrawal-request-form'
import { formatD2SDate, formatD2SMoney } from './lib/format'
import type { D2SOrder, D2SWalletSummary } from './types'

export { LicenseCard } from './components/license-card'

function submitD2SCheckout(
  checkout: {
    checkout_url: string
    method?: 'GET' | 'POST'
    params?: Record<string, string>
    display_mode?: 'iframe'
    qr_pay_mode?: string
  },
  labels: { title: string; close: string; fallback: string; help: string }
) {
  if (
    checkout.display_mode === 'iframe' &&
    checkout.method === 'POST' &&
    checkout.params
  ) {
    const frameName = `d2s-alipay-${globalThis.crypto.randomUUID()}`
    const overlay = document.createElement('div')
    overlay.className =
      'fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4'
    overlay.setAttribute('role', 'dialog')
    overlay.setAttribute('aria-modal', 'true')
    overlay.setAttribute('aria-label', labels.title)

    const panel = document.createElement('section')
    panel.className =
      'flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl bg-background p-4 shadow-xl'
    const header = document.createElement('div')
    header.className = 'mb-3 flex items-center justify-between gap-3'
    const title = document.createElement('h2')
    title.className = 'text-lg font-semibold'
    title.textContent = labels.title
    const help = document.createElement('p')
    help.className = 'mb-3 text-sm text-muted-foreground'
    help.textContent = labels.help
    const actions = document.createElement('div')
    actions.className = 'flex items-center gap-2'
    const fallback = document.createElement('button')
    fallback.type = 'button'
    fallback.className = 'rounded-md border px-3 py-2 text-sm'
    fallback.textContent = labels.fallback
    const close = document.createElement('button')
    close.type = 'button'
    close.className = 'rounded-md border px-3 py-2 text-sm'
    close.textContent = labels.close
    const frame = document.createElement('iframe')
    frame.name = frameName
    frame.title = labels.title
    let frameMinHeight = 'min-h-[300px]'
    if (checkout.qr_pay_mode === '1') frameMinHeight = 'min-h-[600px]'
    if (checkout.qr_pay_mode === '3') frameMinHeight = 'min-h-[160px]'
    frame.className = `w-full flex-1 rounded-lg border bg-white ${frameMinHeight}`
    frame.setAttribute(
      'sandbox',
      'allow-forms allow-scripts allow-same-origin allow-popups allow-top-navigation-by-user-activation'
    )
    frame.setAttribute('allow', 'payment')
    const form = document.createElement('form')
    form.method = 'POST'
    form.action = checkout.checkout_url
    form.target = frameName
    form.hidden = true
    for (const [name, value] of Object.entries(checkout.params)) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = name
      input.value = value
      form.appendChild(input)
    }
    fallback.addEventListener('click', () => {
      const fallbackForm = form.cloneNode(true) as HTMLFormElement
      fallbackForm.target = '_blank'
      document.body.appendChild(fallbackForm)
      fallbackForm.submit()
      fallbackForm.remove()
    })
    close.addEventListener('click', () => {
      overlay.remove()
      form.remove()
    })
    actions.append(fallback, close)
    header.append(title, actions)
    panel.append(header, help, frame)
    overlay.appendChild(panel)
    document.body.append(overlay, form)
    form.submit()
    return
  }
  if (checkout.method === 'POST' && checkout.params) {
    const form = document.createElement('form')
    form.method = 'POST'
    form.action = checkout.checkout_url
    form.style.display = 'none'
    for (const [name, value] of Object.entries(checkout.params)) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = name
      input.value = value
      form.appendChild(input)
    }
    document.body.appendChild(form)
    form.submit()
    return
  }
  window.location.assign(checkout.checkout_url)
}

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

const EMPTY_D2S_ORDERS: D2SOrder[] = []

function StatusBadge(props: { value: string }) {
  const destructive = ['suspended', 'revoked', 'chargeback', 'failed'].includes(
    props.value
  )
  return (
    <Badge variant={destructive ? 'destructive' : 'secondary'}>
      {props.value}
    </Badge>
  )
}

function OrderRow(props: { order: D2SOrder }) {
  const { i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  return (
    <div className='grid gap-2 border-b py-3 text-sm last:border-0 sm:grid-cols-[1.4fr_0.8fr_0.8fr_1.2fr] sm:items-center'>
      <div className='font-mono text-xs'>{props.order.id}</div>
      <div>{props.order.product}</div>
      <div>
        <StatusBadge value={props.order.status} />
      </div>
      <div className='sm:text-right'>
        {formatD2SMoney(props.order.amount_minor, props.order.currency, locale)}
        <div className='text-muted-foreground text-xs'>
          {formatD2SDate(props.order.created_at, locale)}
        </div>
      </div>
    </div>
  )
}

export function D2SWorkspace(
  props: {
    view?: 'authorization' | 'wallet'
    embeddedWallet?: boolean
    onWalletSummaryChange?: (summary: D2SWalletSummary) => void
  } = {}
) {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const view = props.view ?? 'authorization'
  const onWalletSummaryChange = props.onWalletSummaryChange
  const queryClient = useQueryClient()
  const [purchaseProduct, setPurchaseProduct] =
    useState<D2SPurchaseProduct>('license')
  const [purchaseLicenseID, setPurchaseLicenseID] = useState('')
  const [pendingPurchase, setPendingPurchase] =
    useState<D2SQuotedPurchase | null>(null)
  const licenses = useQuery({
    queryKey: ['d2s', 'licenses'],
    queryFn: getD2SLicenses,
    enabled: view === 'authorization',
  })
  const orders = useQuery({
    queryKey: ['d2s', 'orders'],
    queryFn: getD2SOrders,
    enabled: view === 'wallet',
  })
  const checkoutProviders = useQuery({
    queryKey: ['d2s', 'checkout-providers'],
    queryFn: getD2SCheckoutProviders,
    enabled: view === 'authorization',
  })
  const balance = useQuery({
    queryKey: ['d2s', 'balance'],
    queryFn: getD2SBalance,
  })
  const withdrawals = useQuery({
    queryKey: ['d2s', 'withdrawals'],
    queryFn: getD2SWithdrawals,
    enabled: view === 'wallet',
  })
  const modeMutation = useMutation({
    mutationFn: (request: Parameters<typeof changeD2SMode>[0]) =>
      changeD2SMode(request),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Unable to update license mode'))
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('License mode updated'))
    },
    onError: () => toast.error(t('Unable to update license mode')),
  })
  const permanentMutation = useMutation({
    mutationFn: (request: { license_id: string; device_hash: string }) =>
      confirmD2SPermanent({
        ...request,
        confirmation: 'PERMANENT',
      }),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(
          result.error?.message || t('Unable to make license permanent')
        )
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('License permanently bound'))
    },
    onError: () => toast.error(t('Unable to make license permanent')),
  })
  const revokeMutation = useMutation({
    mutationFn: (request: Parameters<typeof freeRevokeD2SLicense>[0]) =>
      freeRevokeD2SLicense(request),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Unable to revoke license'))
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('License revoked'))
    },
    onError: () => toast.error(t('Unable to revoke license')),
  })
  const manualUnbindMutation = useMutation({
    mutationFn: (request: { license_id: string; reason: string }) =>
      createD2SManualUnbind(request),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(
          result.error?.message || t('Unable to request manual unbind')
        )
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('Manual unbind requested'))
    },
    onError: () => toast.error(t('Unable to request manual unbind')),
  })
  const manualUnbinds = useQuery({
    queryKey: ['d2s', 'manual-unbind'],
    queryFn: getD2SManualUnbindRequests,
    enabled: view === 'authorization',
  })
  const d2sQueries =
    view === 'wallet'
      ? [orders, balance, withdrawals]
      : [licenses, checkoutProviders, balance, manualUnbinds]
  const d2sDataLoading = d2sQueries.some((query) => query.isPending)
  const d2sDataFailed = d2sQueries.some(
    (query) => query.isError || query.data?.success === false
  )
  const licenseRows = licenses.data?.data?.licenses ?? []

  const accountRows = balance.data?.data?.accounts ?? []
  const orderRows = orders.data?.data?.orders ?? EMPTY_D2S_ORDERS
  const withdrawalRows = withdrawals.data?.data?.withdrawals ?? []
  const purchasableLicenses = (licenses.data?.data?.licenses ?? []).filter(
    (license) => {
      if (license.status !== 'active' || license.mode === 'permanent') {
        return false
      }
      if (purchaseProduct === 'paid_revoke') {
        return Boolean(license.device_hash)
      }
      return true
    }
  )
  const purchaseTargetMissing =
    purchaseProduct !== 'license' &&
    !purchasableLicenses.some((license) => license.id === purchaseLicenseID)
  const cnyBalance = accountRows.find((account) => account.currency === 'CNY')
  useEffect(() => {
    if (view !== 'wallet' || !onWalletSummaryChange) return
    onWalletSummaryChange({
      orders: orderRows,
    })
  }, [orderRows, onWalletSummaryChange, view])
  const purchaseEnabled = checkoutProviders.data?.data?.enabled === true
  const enabledCheckoutProviders = purchaseEnabled
    ? (checkoutProviders.data?.data?.providers ?? [])
    : []
  const quoteMutation = useMutation({
    mutationFn: async (request: {
      provider: D2SPurchaseProvider
      product: D2SPurchaseProduct
      licenseID?: string
    }): Promise<D2SQuotedPurchase> => {
      const preview = await previewD2SOrder({
        product: request.product,
        license_id: request.licenseID,
        provider: request.provider,
      })
      if (
        !preview.success ||
        !preview.data ||
        preview.data.product !== request.product ||
        preview.data.provider !== request.provider ||
        !Number.isSafeInteger(preview.data.amount_minor) ||
        preview.data.amount_minor <= 0
      ) {
        throw new Error('quote unavailable')
      }
      if (request.provider === 'balance') {
        const account = accountRows.find(
          (row) => row.currency === preview.data?.currency
        )
        if (!account || account.available_minor < preview.data.amount_minor) {
          throw new Error('insufficient balance')
        }
      }
      return {
        ...request,
        amountMinor: preview.data.amount_minor,
        currency: preview.data.currency,
        region: preview.data.region,
        idempotencyKey: `d2s-${globalThis.crypto.randomUUID()}`,
      }
    },
    onSuccess: setPendingPurchase,
    onError: (error, request) => {
      let message: string
      if (error.message === 'insufficient balance') {
        message = t('Insufficient balance')
      } else if (request.provider === 'balance') {
        message = t('Unable to purchase license')
      } else {
        message = t('Unable to start checkout')
      }
      toast.error(message)
    },
  })
  const balancePurchaseMutation = useMutation({
    mutationFn: (quote: D2SQuotedPurchase) =>
      createD2SOrder({
        product: quote.product,
        license_id: quote.licenseID,
        provider: 'balance',
        balance_minor: quote.amountMinor,
        expected_amount_minor: quote.amountMinor,
        expected_currency: quote.currency,
        expected_region: quote.region,
        idempotency_key: quote.idempotencyKey,
      }),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Unable to purchase license'))
        return
      }
      setPendingPurchase(null)
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('License purchased'))
    },
    onError: (error) => {
      if (isD2SQuoteChanged(error)) {
        setPendingPurchase(null)
        toast.error(
          t('Price changed; review the latest quote before purchasing.')
        )
        return
      }
      toast.error(t('Unable to purchase license'))
    },
  })
  const externalPurchaseMutation = useMutation({
    mutationFn: async (request: D2SQuotedPurchase) => {
      const created = await createD2SOrder({
        product: request.product,
        license_id: request.licenseID,
        provider: request.provider,
        balance_minor: 0,
        expected_amount_minor: request.amountMinor,
        expected_currency: request.currency,
        expected_region: request.region,
        idempotency_key: request.idempotencyKey,
      })
      if (!created.success || !created.data) {
        throw new Error(created.error?.message || 'order creation failed')
      }
      return createD2SOrderCheckout(created.data.id)
    },
    onSuccess: (result) => {
      if (!result.success || !result.data?.checkout_url) {
        toast.error(result.error?.message || t('Unable to start checkout'))
        return
      }
      submitD2SCheckout(result.data, {
        title: t('Pay with Alipay'),
        close: t('Close'),
        fallback: t('Open Alipay in a new tab'),
        help: t(
          'Scan the Alipay QR code here. If it does not appear, open Alipay in a new tab.'
        ),
      })
      setPendingPurchase(null)
    },
    onError: (error) => {
      if (isD2SQuoteChanged(error)) {
        setPendingPurchase(null)
        toast.error(
          t('Price changed; review the latest quote before purchasing.')
        )
        return
      }
      toast.error(t('Unable to start checkout'))
    },
  })
  const purchasePending =
    quoteMutation.isPending ||
    balancePurchaseMutation.isPending ||
    externalPurchaseMutation.isPending
  let purchaseProductLabel = ''
  if (pendingPurchase?.product === 'license') {
    purchaseProductLabel = t('New license')
  } else if (pendingPurchase?.product === 'paid_revoke') {
    purchaseProductLabel = t('Paid revoke')
  } else if (pendingPurchase?.product === 'offline_extension') {
    purchaseProductLabel = t('Offline extension')
  }
  const requestPurchaseQuote = (provider: D2SPurchaseProvider) => {
    quoteMutation.mutate({
      provider,
      product: purchaseProduct,
      licenseID: purchaseProduct === 'license' ? undefined : purchaseLicenseID,
    })
  }

  const content = (
    <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
      {view === 'authorization' && (
        <div className='space-y-5'>
          <p className='text-muted-foreground text-sm'>
            {t('Manage your licenses, devices and purchases in one place.')}
          </p>
          {licenses.isSuccess && licenses.data?.success && (
            <dl className='grid grid-cols-3 gap-2 sm:gap-3'>
              {[
                {
                  label: t('Total licenses'),
                  value: licenseRows.length,
                  icon: KeyRound,
                },
                {
                  label: t('Active licenses'),
                  value: licenseRows.filter(
                    (license) => license.status === 'active'
                  ).length,
                  icon: ShieldCheck,
                },
                {
                  label: t('Bound devices'),
                  value: new Set(
                    licenseRows.flatMap((license) =>
                      license.device_hash ? [license.device_hash] : []
                    )
                  ).size,
                  icon: Monitor,
                },
              ].map((item) => (
                <div
                  key={item.label}
                  className='flex min-w-0 items-center justify-between rounded-xl border bg-card p-3 sm:p-4'
                >
                  <div>
                    <dt className='text-muted-foreground text-xs sm:text-sm'>
                      {item.label}
                    </dt>
                    <dd className='mt-1 text-xl font-semibold tabular-nums sm:text-2xl'>
                      {item.value}
                    </dd>
                  </div>
                  <item.icon
                    className='text-muted-foreground hidden size-5 sm:block'
                    aria-hidden='true'
                  />
                </div>
              ))}
            </dl>
          )}
        </div>
      )}
      {d2sDataLoading && (
        <p role='status' className='text-muted-foreground text-sm'>
          {t('Loading Desktop2Stereo data')}
        </p>
      )}
      {d2sDataFailed && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load Desktop2Stereo data')}
        </p>
      )}
      {view === 'authorization' && (
        <AlertDialog
          open={pendingPurchase !== null}
          onOpenChange={(open) => {
            if (!open && !purchasePending) setPendingPurchase(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('Confirm Payment')}</AlertDialogTitle>
              <AlertDialogDescription>
                {t('Amount to pay:')}{' '}
                <strong className='text-foreground'>
                  {pendingPurchase &&
                    formatD2SMoney(
                      pendingPurchase.amountMinor,
                      pendingPurchase.currency,
                      locale
                    )}
                </strong>
              </AlertDialogDescription>
            </AlertDialogHeader>
            {pendingPurchase && (
              <dl className='grid gap-2 text-sm'>
                <div className='flex justify-between gap-4'>
                  <dt>{t('Purchase type')}</dt>
                  <dd>{purchaseProductLabel}</dd>
                </div>
                {pendingPurchase.licenseID && (
                  <div className='flex justify-between gap-4'>
                    <dt>{t('License')}</dt>
                    <dd>
                      {licenses.data?.data?.licenses?.find(
                        (license) => license.id === pendingPurchase.licenseID
                      )?.license_code ?? pendingPurchase.licenseID}
                    </dd>
                  </div>
                )}
                <div className='flex justify-between gap-4'>
                  <dt>{t('Payment')}</dt>
                  <dd>
                    {pendingPurchase.provider === 'balance'
                      ? t('Balance')
                      : D2S_PROVIDER_NAMES[pendingPurchase.provider]}
                  </dd>
                </div>
                <div className='flex justify-between gap-4'>
                  <dt>{t('Region')}</dt>
                  <dd>{pendingPurchase.region}</dd>
                </div>
              </dl>
            )}
            <AlertDialogFooter>
              <AlertDialogCancel disabled={purchasePending}>
                {t('Cancel')}
              </AlertDialogCancel>
              <AlertDialogAction
                type='button'
                disabled={purchasePending}
                onClick={() => {
                  if (!pendingPurchase) return
                  if (pendingPurchase.provider === 'balance') {
                    balancePurchaseMutation.mutate(pendingPurchase)
                  } else {
                    externalPurchaseMutation.mutate(pendingPurchase)
                  }
                }}
              >
                {t('Confirm')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}

      <div className='w-full min-w-0'>
        {view === 'authorization' && (
          <section className='space-y-4' aria-labelledby='d2s-licenses-title'>
            <h2 id='d2s-licenses-title' className='text-lg font-semibold'>
              {t('Licenses and devices')}
            </h2>
            <div className='grid min-w-0 gap-4 lg:grid-cols-2'>
              {licenseRows.map((license) => (
                <LicenseCard
                  key={license.id}
                  license={license}
                  actionPending={
                    modeMutation.isPending ||
                    permanentMutation.isPending ||
                    revokeMutation.isPending ||
                    manualUnbindMutation.isPending
                  }
                  onChangeMode={(mode, offlineDays) => {
                    if (!license.device_hash) return
                    modeMutation.mutate({
                      license_id: license.id,
                      device_hash: license.device_hash,
                      mode,
                      offline_period_days: offlineDays,
                    })
                  }}
                  onConfirmPermanent={() => {
                    if (
                      !license.device_hash ||
                      !window.confirm(t('Confirm permanent binding'))
                    ) {
                      return
                    }
                    permanentMutation.mutate({
                      license_id: license.id,
                      device_hash: license.device_hash,
                    })
                  }}
                  onFreeRevoke={() => {
                    if (
                      !license.device_hash ||
                      !window.confirm(t('Confirm free revoke'))
                    ) {
                      return
                    }
                    revokeMutation.mutate({
                      license_id: license.id,
                      device_hash: license.device_hash,
                      fingerprint_version: license.fingerprint_version,
                    })
                  }}
                  onManualUnbind={(reason) => {
                    manualUnbindMutation.mutate({
                      license_id: license.id,
                      reason,
                    })
                  }}
                  manualUnbindStatus={
                    manualUnbinds.data?.data?.requests
                      ?.filter((request) => request.license_id === license.id)
                      .sort((a, b) => b.created_at - a.created_at)[0]?.status
                  }
                />
              ))}
            </div>
            {licenses.isSuccess &&
              licenses.data?.success &&
              licenseRows.length === 0 && (
                <p
                  aria-live='polite'
                  className='text-muted-foreground flex min-h-40 flex-col items-center justify-center gap-3 rounded-xl border border-dashed bg-muted/20 px-5 py-8 text-center text-sm'
                  role='status'
                >
                  <KeyRound className='size-7' aria-hidden='true' />
                  <span className='text-foreground font-medium'>
                    {t('No licenses')}
                  </span>
                  <span>{t('Your licenses will appear here.')}</span>
                </p>
              )}
          </section>
        )}
      </div>

      {view === 'authorization' && (
        <Card id='d2s-purchase' className='min-w-0 overflow-hidden'>
          <CardHeader>
            <CardTitle>{t('Purchase a license')}</CardTitle>
            <CardDescription>
              {t('Choose a license product and an available payment method.')}
            </CardDescription>
            {!purchaseEnabled && (
              <p role='status' className='text-muted-foreground text-sm'>
                {t('Purchases are currently unavailable.')}
              </p>
            )}
          </CardHeader>
          <CardContent
            className='flex flex-wrap items-center gap-3'
            aria-busy={purchasePending}
          >
            <label className='flex items-center gap-2 text-sm'>
              <span>{t('Purchase type')}</span>
              <select
                aria-label={t('Purchase type')}
                autoComplete='off'
                className='h-8 rounded-lg border bg-transparent px-2 text-sm'
                name='purchase_type'
                disabled={!purchaseEnabled || purchasePending}
                value={purchaseProduct}
                onChange={(event) =>
                  setPurchaseProduct(event.target.value as D2SPurchaseProduct)
                }
              >
                <option value='license'>{t('New license')}</option>
                <option value='paid_revoke'>{t('Paid revoke')}</option>
                <option value='offline_extension'>
                  {t('Offline extension')}
                </option>
              </select>
            </label>
            {purchaseProduct !== 'license' && (
              <label className='flex items-center gap-2 text-sm'>
                <span>{t('License')}</span>
                <select
                  aria-label={t('License')}
                  autoComplete='off'
                  className='h-8 max-w-56 rounded-lg border bg-transparent px-2 text-sm'
                  name='license_id'
                  disabled={!purchaseEnabled || purchasePending}
                  value={purchaseLicenseID}
                  onChange={(event) => setPurchaseLicenseID(event.target.value)}
                >
                  <option value=''>{t('Select a license')}</option>
                  {purchasableLicenses.map((license) => (
                    <option key={license.id} value={license.id}>
                      {license.license_code}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {enabledCheckoutProviders.includes('alipay_pc') && (
              <Button
                type='button'
                variant='outline'
                className='border-blue-600 bg-blue-50 text-blue-950 hover:bg-blue-100'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('alipay_pc')}
              >
                <img
                  src='/alipay-logo-square.png'
                  alt=''
                  aria-hidden='true'
                  className='size-6 rounded-full'
                />
                <span>
                  {externalPurchaseMutation.isPending
                    ? t('Creating checkout…')
                    : t('Pay on Alipay (PC)')}
                </span>
                <Badge className='bg-orange-600 text-white hover:bg-orange-600'>
                  {t('Recommended')}
                </Badge>
              </Button>
            )}
            {enabledCheckoutProviders.includes('balance') && (
              <Button
                type='button'
                disabled={
                  purchasePending ||
                  purchaseTargetMissing ||
                  balance.isPending ||
                  balance.isError
                }
                onClick={() => requestPurchaseQuote('balance')}
              >
                {balancePurchaseMutation.isPending
                  ? t('Creating order…')
                  : t('Buy with balance')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('alipay') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('alipay')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Alipay')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('paymentfm') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('paymentfm')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Payment FM')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('wechat') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('wechat')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with WeChat')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('waffo') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('waffo')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Waffo')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('waffo_pancake') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('waffo_pancake')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Waffo Pancake')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('stripe') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('stripe')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Stripe')}
              </Button>
            )}
            {enabledCheckoutProviders.includes('creem') && (
              <Button
                type='button'
                variant='outline'
                disabled={purchasePending || purchaseTargetMissing}
                onClick={() => requestPurchaseQuote('creem')}
              >
                {externalPurchaseMutation.isPending
                  ? t('Creating checkout…')
                  : t('Pay with Creem')}
              </Button>
            )}
            <span className='text-muted-foreground text-sm'>
              {t('The server will quote the price and verify your currency.')}
            </span>
            {quoteMutation.isPending && (
              <p role='status' aria-live='polite' className='text-sm'>
                {t('Getting price quote…')}
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {view === 'wallet' && (
        <>
          {!props.embeddedWallet && (
            <Card>
              <CardHeader>
                <CardTitle>{t('Orders')}</CardTitle>
                <CardDescription>{t('Recent D2S orders')}</CardDescription>
              </CardHeader>
              <CardContent>
                {orderRows.length === 0 ? (
                  <p
                    aria-live='polite'
                    className='text-muted-foreground'
                    role='status'
                  >
                    {t('No orders')}
                  </p>
                ) : (
                  orderRows.map((order) => (
                    <OrderRow key={order.id} order={order} />
                  ))
                )}
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle>{t('Withdrawals')}</CardTitle>
              <CardDescription>{t('CN balance withdrawals')}</CardDescription>
            </CardHeader>
            <CardContent className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'>
              <WithdrawalRequestForm
                availableMinor={cnyBalance?.available_minor ?? 0}
                minWithdrawalMinor={
                  balance.data?.data?.min_withdrawal_minor ?? 5000
                }
                onSuccess={() => {
                  void queryClient.invalidateQueries({
                    queryKey: ['d2s', 'withdrawals'],
                  })
                  void queryClient.invalidateQueries({
                    queryKey: ['d2s', 'balance'],
                  })
                }}
              />
              <div>
                {withdrawalRows.length === 0 ? (
                  <p
                    aria-live='polite'
                    className='text-muted-foreground'
                    role='status'
                  >
                    {t('No withdrawals')}
                  </p>
                ) : (
                  withdrawalRows.map((withdrawal) => (
                    <div
                      key={withdrawal.id}
                      className='flex flex-wrap justify-between gap-2 border-b py-3 last:border-0'
                    >
                      <span>
                        {formatD2SMoney(
                          withdrawal.amount_minor,
                          withdrawal.currency,
                          locale
                        )}
                      </span>
                      <span>
                        <StatusBadge value={withdrawal.status} />{' '}
                        <span className='text-muted-foreground text-xs'>
                          {formatD2SDate(withdrawal.created_at, locale)}
                        </span>
                      </span>
                    </div>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )

  if (view === 'wallet') return content

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Authorization management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export function D2SWalletSection(
  props: {
    embeddedWallet?: boolean
    onWalletSummaryChange?: (summary: D2SWalletSummary) => void
  } = {}
) {
  return <D2SWorkspace view='wallet' {...props} />
}
