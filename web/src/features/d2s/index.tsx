import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { KeyRound, Monitor, ShieldCheck } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
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
  getD2SWithdrawals,
  changeD2SMode,
  confirmD2SPermanent,
  freeRevokeD2SLicense,
  createD2SManualUnbind,
  getD2SManualUnbindRequests,
} from './api'
import { LicenseCard } from './components/license-card'
import { WithdrawalRequestForm } from './components/withdrawal-request-form'
import { formatD2SDate, formatD2SMoney } from './lib/format'
import type { D2SOrder, D2SWalletSummary } from './types'

export { LicenseCard } from './components/license-card'

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
    enabled: view === 'authorization',
  })
  const d2sQueries =
    view === 'wallet'
      ? [orders, balance, withdrawals]
      : [licenses, manualUnbinds]
  const d2sDataLoading = d2sQueries.some((query) => query.isPending)
  const d2sDataFailed = d2sQueries.some(
    (query) => query.isError || query.data?.success === false
  )
  const licenseRows = licenses.data?.data?.licenses ?? []

  const accountRows = balance.data?.data?.accounts ?? []
  const orderRows = orders.data?.data?.orders ?? EMPTY_D2S_ORDERS
  const withdrawalRows = withdrawals.data?.data?.withdrawals ?? []
  const cnyBalance = accountRows.find((account) => account.currency === 'CNY')
  useEffect(() => {
    if (view !== 'wallet' || !onWalletSummaryChange) return
    onWalletSummaryChange({
      orders: orderRows,
    })
  }, [orderRows, onWalletSummaryChange, view])
  const content = (
    <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
      {view === 'authorization' && (
        <div className='space-y-5'>
          <p className='text-muted-foreground text-sm'>
            {t('Manage your licenses and devices.')}
          </p>
          <Link
            to='/d2s/store'
            className='text-primary inline-flex items-center gap-2 text-sm font-medium underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-4'
          >
            {t('Visit store')} <span aria-hidden='true'>→</span>
          </Link>
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
