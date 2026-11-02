import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
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
import { toIntlLocale } from '@/i18n/languages'

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
import { WithdrawalRequestForm } from './components/withdrawal-request-form'
import type { D2SLicense, D2SOrder, D2SWalletSummary } from './types'

function submitD2SCheckout(checkout: {
  checkout_url: string
  method?: 'GET' | 'POST'
  params?: Record<string, string>
}) {
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

function formatMoney(
  amountMinor: number,
  currency: string,
  locale?: string
): string {
  const parts = new Intl.NumberFormat(toIntlLocale(locale), {
    style: 'currency',
    currency,
    currencyDisplay: 'code',
  }).formatToParts(amountMinor / 100)
  return parts
    .map((part) => (part.type === 'literal' ? ' ' : part.value))
    .join('')
}

function formatDate(timestamp: number, locale?: string): string {
  if (!timestamp) return '—'
  return new Intl.DateTimeFormat(toIntlLocale(locale), {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp * 1000))
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

export function LicenseCard(props: {
  license: D2SLicense
  actionPending: boolean
  onChangeMode: (mode: 'online' | 'offline', offlineDays: number) => void
  onConfirmPermanent: () => void
  onFreeRevoke: () => void
  onManualUnbind: (reason: string) => void
  manualUnbindStatus?: string
}) {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const configuredDays = props.license.offline_period_days
  const [offlineDays, setOfflineDays] = useState<7 | 14 | 30>(
    configuredDays === 14 || configuredDays === 30 ? configuredDays : 7
  )
  const [manualReason, setManualReason] = useState('')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{props.license.license_code}</CardTitle>
        <CardDescription>{props.license.kind}</CardDescription>
      </CardHeader>
      <CardContent
        className='grid gap-2 sm:grid-cols-2'
        aria-busy={props.actionPending}
      >
        <div>
          {t('Status')}: <StatusBadge value={props.license.status} />
        </div>
        <div>
          {t('Mode')}: <span className='font-medium'>{props.license.mode}</span>
        </div>
        <div>
          {t('Device')}:{' '}
          <span className='font-mono text-xs'>
            {props.license.device_hash
              ? `${props.license.device_hash.slice(0, 12)}…`
              : t('Unbound')}
          </span>
        </div>
        <div>
          {t('Expires')}: {formatDate(props.license.expires_at, locale)}
        </div>
        <div>
          {t('Offline access until')}:{' '}
          {formatDate(props.license.offline_valid_until, locale)}
        </div>
        {props.license.status === 'active' &&
          props.license.device_hash &&
          props.license.mode !== 'permanent' && (
            <div className='flex flex-wrap gap-2 sm:col-span-2'>
              <Button
                type='button'
                size='sm'
                aria-label={`${t('Online mode')} ${props.license.license_code}`}
                variant={
                  props.license.mode === 'online' ? 'default' : 'outline'
                }
                disabled={props.actionPending}
                onClick={() => props.onChangeMode('online', offlineDays)}
              >
                {t('Online mode')}
              </Button>
              <Button
                type='button'
                size='sm'
                aria-label={`${t('Offline mode')} ${
                  props.license.license_code
                }`}
                variant={
                  props.license.mode === 'offline' ? 'default' : 'outline'
                }
                disabled={props.actionPending}
                onClick={() => props.onChangeMode('offline', offlineDays)}
              >
                {t('Offline mode')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='outline'
                aria-label={`${t('Make permanent')} ${
                  props.license.license_code
                }`}
                disabled={props.actionPending}
                onClick={props.onConfirmPermanent}
              >
                {t('Make permanent')}
              </Button>
              <label className='flex items-center gap-2 text-sm'>
                <span>{t('Offline period')}</span>
                <select
                  aria-label={`${t('Offline period')} ${
                    props.license.license_code
                  }`}
                  autoComplete='off'
                  className='h-8 rounded-lg border bg-transparent px-2 text-sm'
                  name={`offline_period_days_${props.license.id}`}
                  disabled={props.actionPending}
                  value={offlineDays}
                  onChange={(event) =>
                    setOfflineDays(Number(event.target.value) as 7 | 14 | 30)
                  }
                >
                  <option value={7}>{t('7 days')}</option>
                  <option value={14}>{t('14 days')}</option>
                  <option value={30}>{t('30 days')}</option>
                </select>
              </label>
              <Button
                type='button'
                size='sm'
                variant='destructive'
                aria-label={`${t('Free revoke')} ${props.license.license_code}`}
                disabled={props.actionPending}
                onClick={props.onFreeRevoke}
              >
                {t('Free revoke')}
              </Button>
            </div>
          )}
        {props.license.status === 'active' &&
          props.license.mode === 'permanent' &&
          props.license.device_hash && (
            <div className='grid gap-2 sm:col-span-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
              {props.manualUnbindStatus && (
                <div className='text-sm sm:col-span-2'>
                  {t('Manual unbind status')}:{' '}
                  <StatusBadge value={props.manualUnbindStatus} />
                </div>
              )}
              <label className='grid gap-1 text-sm'>
                <span>{t('Manual unbind reason')}</span>
                <input
                  aria-label={`${t('Manual unbind reason')} ${
                    props.license.license_code
                  }`}
                  autoComplete='off'
                  className='h-9 rounded-lg border bg-transparent px-3'
                  name={`manual_unbind_reason_${props.license.id}`}
                  spellCheck={false}
                  value={manualReason}
                  onChange={(event) => setManualReason(event.target.value)}
                  placeholder={t('Manual unbind reason')}
                  disabled={props.actionPending}
                />
              </label>
              <Button
                type='button'
                variant='outline'
                className='self-end'
                disabled={
                  props.actionPending ||
                  props.manualUnbindStatus === 'pending' ||
                  !manualReason.trim()
                }
                onClick={() => {
                  props.onManualUnbind(manualReason.trim())
                  setManualReason('')
                }}
              >
                {t('Request manual unbind')}
              </Button>
            </div>
          )}
      </CardContent>
    </Card>
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
        {formatMoney(props.order.amount_minor, props.order.currency, locale)}
        <div className='text-muted-foreground text-xs'>
          {formatDate(props.order.created_at, locale)}
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
  })
  const orders = useQuery({
    queryKey: ['d2s', 'orders'],
    queryFn: getD2SOrders,
    enabled: view === 'wallet',
  })
  const checkoutProviders = useQuery({
    queryKey: ['d2s', 'checkout-providers'],
    queryFn: getD2SCheckoutProviders,
    enabled: view === 'wallet',
  })
  const balance = useQuery({
    queryKey: ['d2s', 'balance'],
    queryFn: getD2SBalance,
    enabled: view === 'wallet',
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
  })
  const d2sQueries =
    view === 'wallet'
      ? [
          licenses,
          orders,
          checkoutProviders,
          balance,
          withdrawals,
          manualUnbinds,
        ]
      : [licenses, manualUnbinds]
  const d2sDataLoading = d2sQueries.some((query) => query.isPending)
  const d2sDataFailed = d2sQueries.some((query) => query.isError)

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
  const enabledCheckoutProviders = checkoutProviders.data?.data?.providers ?? []
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
      submitD2SCheckout(result.data)
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
      {view === 'wallet' && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Purchase a license')}</CardTitle>
            <CardDescription>
              {t('Use your locked-region balance for a full payment.')}
            </CardDescription>
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
                disabled={purchasePending}
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
                  disabled={purchasePending}
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
                    formatMoney(
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

      <div className='grid gap-4 xl:grid-cols-2'>
        {view === 'authorization' && (
          <section className='space-y-3' aria-labelledby='d2s-licenses-title'>
            <h2 id='d2s-licenses-title' className='text-lg font-semibold'>
              {t('Licenses and devices')}
            </h2>
            {licenses.data?.data?.licenses?.map((license) => (
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
            {!licenses.isLoading &&
              (licenses.data?.data?.licenses?.length ?? 0) === 0 && (
                <p
                  aria-live='polite'
                  className='text-muted-foreground'
                  role='status'
                >
                  {t('No licenses')}
                </p>
              )}
          </section>
        )}
      </div>

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
                        {formatMoney(
                          withdrawal.amount_minor,
                          withdrawal.currency,
                          locale
                        )}
                      </span>
                      <span>
                        <StatusBadge value={withdrawal.status} />{' '}
                        <span className='text-muted-foreground text-xs'>
                          {formatDate(withdrawal.created_at, locale)}
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
