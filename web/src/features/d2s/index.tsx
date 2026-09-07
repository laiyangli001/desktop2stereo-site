import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
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
  getD2SBalanceTransactions,
  getD2SInvite,
  getD2SInviteRecords,
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
import type { D2SLicense, D2SOrder } from './types'

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

function formatMoney(amount: number, currency: string): string {
  return `${currency} ${(amount / 100).toFixed(2)}`
}

function formatDate(timestamp: number): string {
  if (!timestamp) return '—'
  return new Date(timestamp * 1000).toLocaleString()
}

type D2SPurchaseProduct = 'license' | 'paid_revoke' | 'offline_extension'

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
  const { t } = useTranslation()
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
      <CardContent className='grid gap-2 sm:grid-cols-2'>
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
          {t('Expires')}: {formatDate(props.license.expires_at)}
        </div>
        <div>
          {t('Offline access until')}:{' '}
          {formatDate(props.license.offline_valid_until)}
        </div>
        {props.license.status === 'active' &&
          props.license.device_hash &&
          props.license.mode !== 'permanent' && (
            <div className='flex flex-wrap gap-2 sm:col-span-2'>
              <Button
                type='button'
                size='sm'
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
                  className='h-8 rounded-lg border bg-transparent px-2 text-sm'
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
                  className='h-9 rounded-lg border bg-transparent px-3'
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
  return (
    <div className='grid gap-2 border-b py-3 text-sm last:border-0 sm:grid-cols-[1.4fr_0.8fr_0.8fr_1.2fr] sm:items-center'>
      <div className='font-mono text-xs'>{props.order.id}</div>
      <div>{props.order.product}</div>
      <div>
        <StatusBadge value={props.order.status} />
      </div>
      <div className='sm:text-right'>
        {formatMoney(props.order.amount_minor, props.order.currency)}
        <div className='text-muted-foreground text-xs'>
          {formatDate(props.order.created_at)}
        </div>
      </div>
    </div>
  )
}

export function D2SWorkspace() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [purchaseProduct, setPurchaseProduct] =
    useState<D2SPurchaseProduct>('license')
  const [purchaseLicenseID, setPurchaseLicenseID] = useState('')
  const licenses = useQuery({
    queryKey: ['d2s', 'licenses'],
    queryFn: getD2SLicenses,
  })
  const orders = useQuery({
    queryKey: ['d2s', 'orders'],
    queryFn: getD2SOrders,
  })
  const checkoutProviders = useQuery({
    queryKey: ['d2s', 'checkout-providers'],
    queryFn: getD2SCheckoutProviders,
  })
  const balance = useQuery({
    queryKey: ['d2s', 'balance'],
    queryFn: getD2SBalance,
  })
  const balanceTransactions = useQuery({
    queryKey: ['d2s', 'balance-transactions'],
    queryFn: getD2SBalanceTransactions,
  })
  const invite = useQuery({
    queryKey: ['d2s', 'invite'],
    queryFn: getD2SInvite,
  })
  const inviteRecords = useQuery({
    queryKey: ['d2s', 'invite-records'],
    queryFn: getD2SInviteRecords,
  })
  const withdrawals = useQuery({
    queryKey: ['d2s', 'withdrawals'],
    queryFn: getD2SWithdrawals,
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
  const d2sQueries = [
    licenses,
    orders,
    checkoutProviders,
    balance,
    balanceTransactions,
    invite,
    inviteRecords,
    withdrawals,
    manualUnbinds,
  ]
  const d2sDataLoading = d2sQueries.some((query) => query.isPending)
  const d2sDataFailed = d2sQueries.some((query) => query.isError)

  const accountRows = balance.data?.data?.accounts ?? []
  const transactionRows = balanceTransactions.data?.data?.transactions ?? []
  const orderRows = orders.data?.data?.orders ?? []
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
  const enabledCheckoutProviders = checkoutProviders.data?.data?.providers ?? []
  const balancePurchaseMutation = useMutation({
    mutationFn: async () => {
      const preview = await previewD2SOrder({
        product: purchaseProduct,
        license_id:
          purchaseProduct === 'license' ? undefined : purchaseLicenseID,
        provider: 'balance',
      })
      if (!preview.success || !preview.data) {
        throw new Error(preview.error?.message || 'preview failed')
      }
      const account = accountRows.find(
        (row) => row.currency === preview.data?.currency
      )
      if (!account || account.available_minor < preview.data.amount_minor) {
        throw new Error('insufficient balance')
      }
      return createD2SOrder({
        product: purchaseProduct,
        license_id:
          purchaseProduct === 'license' ? undefined : purchaseLicenseID,
        provider: 'balance',
        balance_minor: preview.data.amount_minor,
        idempotency_key: `d2s-balance-${globalThis.crypto.randomUUID()}`,
      })
    },
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Unable to purchase license'))
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('License purchased'))
    },
    onError: () => toast.error(t('Unable to purchase license')),
  })
  const externalPurchaseMutation = useMutation({
    mutationFn: async (request: {
      provider:
        | 'stripe'
        | 'creem'
        | 'waffo_pancake'
        | 'waffo'
        | 'paymentfm'
        | 'alipay'
        | 'wechat'
      product: D2SPurchaseProduct
      licenseID?: string
    }) => {
      const preview = await previewD2SOrder({
        product: request.product,
        license_id: request.licenseID,
        provider: request.provider,
      })
      if (!preview.success || !preview.data) {
        throw new Error(preview.error?.message || 'preview failed')
      }
      const created = await createD2SOrder({
        product: request.product,
        license_id: request.licenseID,
        provider: request.provider,
        balance_minor: 0,
        idempotency_key: `d2s-${request.product}-${request.provider}-${globalThis.crypto.randomUUID()}`,
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
    },
    onError: () => toast.error(t('Unable to start checkout')),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Desktop2Stereo')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
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
          <Card>
            <CardHeader>
              <CardTitle>{t('Balance')}</CardTitle>
              <CardDescription>
                {t('Balances are separated by currency.')}
              </CardDescription>
            </CardHeader>
            <CardContent className='grid gap-3 sm:grid-cols-2'>
              {accountRows.length === 0 && (
                <p className='text-muted-foreground'>
                  {t('No balance accounts')}
                </p>
              )}
              {accountRows.map((account) => (
                <div key={account.id} className='rounded-lg border p-3'>
                  <div className='font-medium'>{account.currency}</div>
                  <div>
                    {formatMoney(account.available_minor, account.currency)}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Reserved')}:{' '}
                    {formatMoney(account.reserved_minor, account.currency)}
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Balance transactions')}</CardTitle>
              <CardDescription>
                {t('Recent currency ledger entries.')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {transactionRows.length === 0 ? (
                <p className='text-muted-foreground'>
                  {t('No balance transactions')}
                </p>
              ) : (
                transactionRows.map((transaction) => (
                  <div
                    key={transaction.id}
                    className='flex flex-wrap justify-between gap-2 border-b py-3 text-sm last:border-0'
                  >
                    <span>
                      {transaction.kind}
                      {transaction.order_id && ` · ${transaction.order_id}`}
                    </span>
                    <span>
                      {transaction.amount_minor > 0 ? '+' : ''}
                      {formatMoney(
                        transaction.amount_minor,
                        transaction.currency
                      )}{' '}
                      <span className='text-muted-foreground text-xs'>
                        {formatDate(transaction.created_at)}
                      </span>
                    </span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Purchase a license')}</CardTitle>
              <CardDescription>
                {t('Use your locked-region balance for a full payment.')}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex flex-wrap items-center gap-3'>
              <label className='flex items-center gap-2 text-sm'>
                <span>{t('Purchase type')}</span>
                <select
                  aria-label={t('Purchase type')}
                  className='h-8 rounded-lg border bg-transparent px-2 text-sm'
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
                    className='h-8 max-w-56 rounded-lg border bg-transparent px-2 text-sm'
                    value={purchaseLicenseID}
                    onChange={(event) =>
                      setPurchaseLicenseID(event.target.value)
                    }
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
                    balancePurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() => balancePurchaseMutation.mutate()}
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'alipay',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'paymentfm',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'wechat',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'waffo',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'waffo_pancake',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'stripe',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
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
                  disabled={
                    externalPurchaseMutation.isPending || purchaseTargetMissing
                  }
                  onClick={() =>
                    externalPurchaseMutation.mutate({
                      provider: 'creem',
                      product: purchaseProduct,
                      licenseID: purchaseLicenseID || undefined,
                    })
                  }
                >
                  {externalPurchaseMutation.isPending
                    ? t('Creating checkout…')
                    : t('Pay with Creem')}
                </Button>
              )}
              <span className='text-muted-foreground text-sm'>
                {t('The server will quote the price and verify your currency.')}
              </span>
            </CardContent>
          </Card>

          <div className='grid gap-4 xl:grid-cols-2'>
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
                  <p className='text-muted-foreground'>{t('No licenses')}</p>
                )}
            </section>
            <section className='space-y-3' aria-labelledby='d2s-invite-title'>
              <h2 id='d2s-invite-title' className='text-lg font-semibold'>
                {t('Invitations')}
              </h2>
              <Card>
                <CardContent className='space-y-2 pt-4'>
                  <div>
                    {t('Invite code')}:{' '}
                    <span className='font-mono'>
                      {invite.data?.data?.invite_code ?? '—'}
                    </span>
                  </div>
                  <div>
                    {t('Rewarded accounts')}:{' '}
                    {invite.data?.data?.rewarded_invitees ?? 0}
                  </div>
                  <div className='border-t pt-2'>
                    <div className='font-medium'>{t('Reward history')}</div>
                    {(inviteRecords.data?.data?.records?.length ?? 0) === 0 ? (
                      <p className='text-muted-foreground text-sm'>
                        {t('No invite rewards')}
                      </p>
                    ) : (
                      <div className='space-y-1 text-sm'>
                        {inviteRecords.data?.data?.records.map((record) => (
                          <div
                            key={record.id}
                            className='flex flex-wrap justify-between gap-2'
                          >
                            <span>
                              {t('Invitee')} #{record.invitee_user_id} ·{' '}
                              <StatusBadge value={record.status} />
                            </span>
                            <span>
                              +
                              {formatMoney(
                                record.amount_minor,
                                record.currency
                              )}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            </section>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Orders')}</CardTitle>
              <CardDescription>{t('Recent D2S orders')}</CardDescription>
            </CardHeader>
            <CardContent>
              {orderRows.length === 0 ? (
                <p className='text-muted-foreground'>{t('No orders')}</p>
              ) : (
                orderRows.map((order) => (
                  <OrderRow key={order.id} order={order} />
                ))
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Withdrawals')}</CardTitle>
              <CardDescription>{t('CN balance withdrawals')}</CardDescription>
            </CardHeader>
            <CardContent className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'>
              <WithdrawalRequestForm
                availableMinor={cnyBalance?.available_minor ?? 0}
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
                  <p className='text-muted-foreground'>{t('No withdrawals')}</p>
                ) : (
                  withdrawalRows.map((withdrawal) => (
                    <div
                      key={withdrawal.id}
                      className='flex flex-wrap justify-between gap-2 border-b py-3 last:border-0'
                    >
                      <span>
                        {formatMoney(
                          withdrawal.amount_minor,
                          withdrawal.currency
                        )}
                      </span>
                      <span>
                        <StatusBadge value={withdrawal.status} />{' '}
                        <span className='text-muted-foreground text-xs'>
                          {formatDate(withdrawal.created_at)}
                        </span>
                      </span>
                    </div>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
