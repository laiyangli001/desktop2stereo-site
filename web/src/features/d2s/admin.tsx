import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'

import {
  getD2SAdminBalances,
  getD2SAdminLicenses,
  getD2SAdminOrders,
  getD2SAdminPaymentEvents,
  getD2SAdminAlipayPCSettings,
  setD2SAdminAlipayQRPayMode,
  closeD2SAdminAlipayOrder,
  createD2SAdminAlipayRefund,
  downloadD2SAdminAlipayBill,
  getD2SAdminAlipayRefunds,
  reconcileD2SAdminAlipayRefund,
  getD2SAdminSigningKeys,
  getD2SAdminReconciliation,
  getD2SAdminUnbindRequests,
  getD2SAdminWithdrawalPayout,
  getD2SAdminWithdrawals,
  retireD2SAdminSigningKey,
  reviewD2SUnbind,
  reviewD2SWithdrawal,
  setD2SUserRegion,
} from './api'
import { formatD2SDate, formatD2SMoney } from './lib/format'
import { AlipayPCConfigForm } from './alipay-config-form'

function AlipayOrderActions(props: {
  orderID: string
  status: string
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const refunds = useQuery({
    queryKey: ['d2s-admin', 'alipay-refunds', props.orderID],
    queryFn: () => getD2SAdminAlipayRefunds(props.orderID),
  })
  const refundMutation = useMutation({
    mutationFn: (input: {
      amount_minor: number
      reason: string
      idempotency_key: string
    }) => createD2SAdminAlipayRefund(props.orderID, input),
    onSuccess: (result) => {
      void refunds.refetch()
      props.onDone()
      if (result.success && result.data?.status === 'pending') {
        toast.info(t('Refund is pending provider confirmation'))
      } else if (result.success) {
        toast.success(t('Refund submitted'))
      } else {
        toast.error(result.error?.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const closeMutation = useMutation({
    mutationFn: () => closeD2SAdminAlipayOrder(props.orderID),
    onSuccess: (result) => {
      props.onDone()
      if (result.success) toast.success(t('Order closed'))
      else toast.error(result.error?.message || t('Operation failed'))
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const reconcileMutation = useMutation({
    mutationFn: reconcileD2SAdminAlipayRefund,
    onSuccess: (result) => {
      void refunds.refetch()
      props.onDone()
      if (result.success && result.data?.status === 'completed') {
        toast.success(t('Refund confirmed'))
      } else {
        toast.info(t('Refund is still pending provider confirmation'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const hasPendingRefund = (refunds.data?.data?.refunds || []).some(
    (refund) => refund.status === 'pending'
  )

  return (
    <div className='w-full space-y-2 border-t pt-2'>
      {props.status === 'paid' && (
        <form
          className='flex flex-wrap items-end gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            const amountMinor = Math.round(Number(amount) * 100)
            if (!Number.isSafeInteger(amountMinor) || amountMinor <= 0) return
            if (!window.confirm(t('Confirm this Alipay refund?'))) return
            refundMutation.mutate({
              amount_minor: amountMinor,
              reason: reason.trim(),
              idempotency_key: `d2s-refund-${globalThis.crypto.randomUUID()}`,
            })
          }}
        >
          <Input
            aria-label={t('Refund amount in CNY')}
            type='number'
            min='0.01'
            step='0.01'
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            placeholder={t('Amount CNY')}
            disabled={refundMutation.isPending || hasPendingRefund}
          />
          <Input
            aria-label={t('Refund reason')}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('Reason')}
            disabled={refundMutation.isPending || hasPendingRefund}
          />
          <Button
            type='submit'
            size='sm'
            disabled={refundMutation.isPending || hasPendingRefund}
          >
            {t('Refund')}
          </Button>
        </form>
      )}
      {props.status === 'pending' && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={closeMutation.isPending}
          onClick={() => {
            if (window.confirm(t('Confirm closing this Alipay order?'))) {
              closeMutation.mutate()
            }
          }}
        >
          {t('Close Alipay order')}
        </Button>
      )}
      {(refunds.data?.data?.refunds || []).map((refund) => (
        <div
          key={refund.id}
          className='flex flex-wrap items-center gap-2 text-xs'
        >
          <span>
            {formatD2SMoney(refund.amount_minor, 'CNY', 'zh-CN')} ·{' '}
            {refund.status} · {refund.out_request_no}
          </span>
          {refund.status === 'pending' && (
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={reconcileMutation.isPending}
              onClick={() => reconcileMutation.mutate(refund.out_request_no)}
            >
              {t('Query refund status')}
            </Button>
          )}
        </div>
      ))}
    </div>
  )
}

function AlipayBillDownload() {
  const { t } = useTranslation()
  const [date, setDate] = useState(() => {
    const today = new Date()
    const month = String(today.getMonth() + 1).padStart(2, '0')
    const day = String(today.getDate()).padStart(2, '0')
    return `${today.getFullYear()}-${month}-${day}`
  })
  const mutation = useMutation({
    mutationFn: downloadD2SAdminAlipayBill,
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `alipay_trade_bill_${date.replaceAll('-', '')}.zip`
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    },
    onError: () => toast.error(t('Unable to download bill')),
  })
  return (
    <form
      className='flex flex-wrap items-center gap-2 border-t pt-3'
      onSubmit={(event) => {
        event.preventDefault()
        mutation.mutate(date)
      }}
    >
      <Input
        aria-label={t('Bill date')}
        type='date'
        value={date}
        onChange={(event) => setDate(event.target.value)}
      />
      <Button type='submit' size='sm' disabled={mutation.isPending}>
        {t('Download Alipay bill')}
      </Button>
    </form>
  )
}

function ReviewActions(props: {
  id: string
  kind: 'unbind' | 'withdrawal'
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [note, setNote] = useState('')
  const mutation = useMutation({
    mutationFn: (status: string) =>
      props.kind === 'unbind'
        ? reviewD2SUnbind(props.id, status, note)
        : reviewD2SWithdrawal(props.id, status, note),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      setNote('')
      props.onDone()
      toast.success(t('Review updated'))
    },
    onError: () => toast.error(t('Operation failed')),
  })

  return (
    <div className='flex min-w-52 gap-2' aria-busy={mutation.isPending}>
      <Input
        aria-label={`${t('Review note')} ${props.id}`}
        autoComplete='off'
        name={`review_note_${props.id}`}
        value={note}
        onChange={(event) => setNote(event.target.value)}
      />
      <Button
        type='button'
        size='sm'
        aria-label={`${t('Approve')} ${props.id}`}
        disabled={mutation.isPending}
        onClick={() =>
          mutation.mutate(props.kind === 'withdrawal' ? 'paid' : 'approved')
        }
      >
        {t('Approve')}
      </Button>
      <Button
        type='button'
        size='sm'
        variant='destructive'
        aria-label={`${t('Reject')} ${props.id}`}
        disabled={mutation.isPending}
        onClick={() => mutation.mutate('rejected')}
      >
        {t('Reject')}
      </Button>
    </div>
  )
}

function WithdrawalPayoutReveal(props: { withdrawalID: string }) {
  const { t } = useTranslation()
  const [details, setDetails] = useState<{
    alipay_account: string
    real_name: string
  } | null>(null)
  const payout = useMutation({
    mutationFn: () => getD2SAdminWithdrawalPayout(props.withdrawalID),
    onSuccess: (result) => {
      if (result.success && result.data) {
        setDetails(result.data)
      }
    },
  })

  return (
    <div className='space-y-2'>
      {details ? (
        <>
          <dl className='grid gap-x-2 sm:grid-cols-[auto_1fr]'>
            <dt className='text-muted-foreground'>{t('Alipay account')}</dt>
            <dd className='break-all'>{details.alipay_account}</dd>
            <dt className='text-muted-foreground'>{t('Real name')}</dt>
            <dd className='break-words'>{details.real_name}</dd>
          </dl>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => setDetails(null)}
          >
            {t('Hide payout details')}
          </Button>
        </>
      ) : (
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={payout.isPending}
          onClick={() => payout.mutate()}
        >
          {payout.isPending
            ? t('Loading payout details')
            : t('View payout details')}
        </Button>
      )}
      {payout.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load payout details')}
        </p>
      )}
      {payout.isSuccess && !payout.data?.success && !details && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load payout details')}
        </p>
      )}
    </div>
  )
}

export function D2SAdminWorkspace() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [eventFilters, setEventFilters] = useState({
    provider: '',
    order_id: '',
    event_type: '',
  })
  const [activeEventFilters, setActiveEventFilters] = useState(eventFilters)
  const [orderFilters, setOrderFilters] = useState({
    status: '',
    user_id: '',
  })
  const [activeOrderFilters, setActiveOrderFilters] = useState(orderFilters)
  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: ['d2s-admin'] })
  const licenses = useQuery({
    queryKey: ['d2s-admin', 'licenses'],
    queryFn: getD2SAdminLicenses,
  })
  const orders = useQuery({
    queryKey: ['d2s-admin', 'orders', activeOrderFilters],
    queryFn: () => getD2SAdminOrders(activeOrderFilters),
  })
  const paymentEvents = useQuery({
    queryKey: ['d2s-admin', 'payment-events', activeEventFilters],
    queryFn: () => getD2SAdminPaymentEvents(activeEventFilters),
  })
  const alipaySettings = useQuery({
    queryKey: ['d2s-admin', 'alipay-settings'],
    queryFn: getD2SAdminAlipayPCSettings,
  })
  const balances = useQuery({
    queryKey: ['d2s-admin', 'balances'],
    queryFn: () => getD2SAdminBalances(),
  })
  const withdrawals = useQuery({
    queryKey: ['d2s-admin', 'withdrawals'],
    queryFn: getD2SAdminWithdrawals,
  })
  const unbinds = useQuery({
    queryKey: ['d2s-admin', 'unbinds'],
    queryFn: getD2SAdminUnbindRequests,
  })
  const keys = useQuery({
    queryKey: ['d2s-admin', 'keys'],
    queryFn: getD2SAdminSigningKeys,
  })
  const reconciliation = useQuery({
    queryKey: ['d2s-admin', 'reconciliation'],
    queryFn: getD2SAdminReconciliation,
  })
  const adminQueries = [
    licenses,
    orders,
    paymentEvents,
    alipaySettings,
    balances,
    withdrawals,
    unbinds,
    keys,
    reconciliation,
  ]
  const adminDataLoading = adminQueries.some((query) => query.isPending)
  const adminDataFailed = adminQueries.some((query) => query.isError)
  const regionMutation = useMutation({
    mutationFn: ({ userID, region }: { userID: number; region: string }) =>
      setD2SUserRegion(userID, region),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
      } else {
        void queryClient.invalidateQueries({
          queryKey: ['d2s-admin', 'licenses'],
        })
        toast.success(t('Region updated'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const signingKeyMutation = useMutation({
    mutationFn: (keyID: string) => retireD2SAdminSigningKey(keyID),
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      toast.success(t('Signing key retired'))
      void queryClient.invalidateQueries({ queryKey: ['d2s-admin', 'keys'] })
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const alipayQRModeMutation = useMutation({
    mutationFn: setD2SAdminAlipayQRPayMode,
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      void queryClient.invalidateQueries({
        queryKey: ['d2s-admin', 'alipay-settings'],
      })
      toast.success(t('Alipay QR display mode updated'))
    },
    onError: () => toast.error(t('Operation failed')),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Authorization management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          {adminDataLoading && (
            <p role='status' className='text-muted-foreground text-sm'>
              {t('Loading Desktop2Stereo admin data')}
            </p>
          )}
          {adminDataFailed && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Unable to load Desktop2Stereo admin data')}
            </p>
          )}
          <div className='grid gap-4 lg:grid-cols-2'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Orders and chargebacks')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3 text-sm'>
                <form
                  className='grid gap-2 sm:grid-cols-[1fr_1fr_auto_auto]'
                  onSubmit={(event) => {
                    event.preventDefault()
                    setActiveOrderFilters({
                      status: orderFilters.status.trim(),
                      user_id: orderFilters.user_id.trim(),
                    })
                  }}
                >
                  <select
                    aria-label={t('Order status')}
                    autoComplete='off'
                    name='order_status'
                    value={orderFilters.status}
                    onChange={(event) =>
                      setOrderFilters((current) => ({
                        ...current,
                        status: event.target.value,
                      }))
                    }
                    className='h-9 rounded-lg border bg-transparent px-3 text-sm'
                  >
                    <option value=''>{t('All statuses')}</option>
                    <option value='pending'>{t('Pending')}</option>
                    <option value='paid'>{t('Paid')}</option>
                    <option value='canceled'>{t('Canceled')}</option>
                    <option value='chargeback'>{t('Chargeback')}</option>
                  </select>
                  <Input
                    aria-label={t('User ID')}
                    autoComplete='off'
                    name='user_id'
                    placeholder={t('User ID')}
                    inputMode='numeric'
                    value={orderFilters.user_id}
                    onChange={(event) =>
                      setOrderFilters((current) => ({
                        ...current,
                        user_id: event.target.value,
                      }))
                    }
                  />
                  <Button type='submit' size='sm'>
                    {t('Filter orders')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    onClick={() => {
                      const emptyFilters = { status: '', user_id: '' }
                      setOrderFilters(emptyFilters)
                      setActiveOrderFilters(emptyFilters)
                    }}
                  >
                    {t('Clear order filter')}
                  </Button>
                </form>
                {(orders.data?.data?.orders || []).map((order) => (
                  <div key={order.id} className='border-b pb-2'>
                    <div className='flex flex-wrap justify-between gap-2'>
                      <span>
                        {order.id} · user {order.user_id} · {order.provider}
                      </span>
                      <span>
                        {order.status} ·{' '}
                        {formatD2SMoney(
                          order.amount_minor,
                          order.currency,
                          locale
                        )}
                      </span>
                    </div>
                    {order.provider === 'alipay_pc' && (
                      <AlipayOrderActions
                        orderID={order.id}
                        status={order.status}
                        onDone={refresh}
                      />
                    )}
                  </div>
                ))}
                {!orders.data?.data?.orders?.length && (
                  <p className='text-muted-foreground'>{t('No records')}</p>
                )}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>{t('Negative balances')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-2 text-sm'>
                {(balances.data?.data?.accounts || []).map((account) => (
                  <div
                    key={account.id}
                    className='flex justify-between border-b pb-2'
                  >
                    <span>
                      user {account.user_id} · {account.currency}
                    </span>
                    <span className='text-destructive'>
                      {formatD2SMoney(
                        account.available_minor,
                        account.currency,
                        locale
                      )}
                    </span>
                  </div>
                ))}
                {!balances.data?.data?.accounts?.length && (
                  <p className='text-muted-foreground'>
                    {t('No negative balances')}
                  </p>
                )}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>{t('Alipay PC QR display')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3 text-sm'>
                <label className='flex flex-wrap items-center gap-3'>
                  <span>{t('QR display mode')}</span>
                  <select
                    aria-label={t('QR display mode')}
                    className='bg-background h-9 rounded-md border px-3'
                    disabled={
                      alipayQRModeMutation.isPending || alipaySettings.isPending
                    }
                    value={alipaySettings.data?.data?.qr_pay_mode || '4'}
                    onChange={(event) =>
                      alipayQRModeMutation.mutate(event.target.value)
                    }
                  >
                    <option value='0'>0 · {t('Compact landscape')}</option>
                    <option value='1'>1 · {t('Portrait')}</option>
                    <option value='3'>3 · {t('Mini')}</option>
                    <option value='4'>4 · {t('Embedded QR')}</option>
                  </select>
                </label>
                <p className='text-muted-foreground'>
                  {t('This setting applies to all Alipay PC checkouts.')}
                </p>
              </CardContent>
            </Card>
            <AlipayPCConfigForm
              key={JSON.stringify(alipaySettings.data?.data ?? null)}
              settings={alipaySettings.data?.data}
              showQRMode={false}
              onSaved={() =>
                void queryClient.invalidateQueries({
                  queryKey: ['d2s-admin', 'alipay-settings'],
                })
              }
            />
            <Card>
              <CardHeader>
                <CardTitle>{t('Payment events')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3 text-sm'>
                <AlipayBillDownload />
                <form
                  className='grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto_auto]'
                  onSubmit={(event) => {
                    event.preventDefault()
                    setActiveEventFilters({
                      provider: eventFilters.provider.trim(),
                      order_id: eventFilters.order_id.trim(),
                      event_type: eventFilters.event_type.trim(),
                    })
                  }}
                >
                  <Input
                    aria-label={t('Provider')}
                    autoComplete='off'
                    name='provider'
                    placeholder={t('Provider')}
                    value={eventFilters.provider}
                    onChange={(event) =>
                      setEventFilters((current) => ({
                        ...current,
                        provider: event.target.value,
                      }))
                    }
                  />
                  <Input
                    aria-label={t('Event type')}
                    autoComplete='off'
                    name='event_type'
                    placeholder={t('Event type')}
                    value={eventFilters.event_type}
                    onChange={(event) =>
                      setEventFilters((current) => ({
                        ...current,
                        event_type: event.target.value,
                      }))
                    }
                  />
                  <Input
                    aria-label={t('Order ID')}
                    autoComplete='off'
                    name='order_id'
                    placeholder={t('Order ID')}
                    value={eventFilters.order_id}
                    onChange={(event) =>
                      setEventFilters((current) => ({
                        ...current,
                        order_id: event.target.value,
                      }))
                    }
                  />
                  <Button type='submit' size='sm'>
                    {t('Filter')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    onClick={() => {
                      const emptyFilters = {
                        provider: '',
                        order_id: '',
                        event_type: '',
                      }
                      setEventFilters(emptyFilters)
                      setActiveEventFilters(emptyFilters)
                    }}
                  >
                    {t('Clear')}
                  </Button>
                </form>
                {(paymentEvents.data?.data?.events || []).map((event) => (
                  <div
                    key={event.id}
                    className='space-y-1 border-b pb-2 last:border-0'
                  >
                    <div>
                      {event.provider} · {event.event_type} ·{' '}
                      {formatD2SMoney(
                        event.amount_minor,
                        event.currency,
                        locale
                      )}
                    </div>
                    <div className='text-muted-foreground font-mono text-xs'>
                      {event.provider_event_id} · {event.order_id}
                    </div>
                    <div className='text-muted-foreground text-xs'>
                      {t('Processed at')}:{' '}
                      {formatD2SDate(event.processed_at, locale)}
                    </div>
                  </div>
                ))}
                {!paymentEvents.data?.data?.events?.length && (
                  <p className='text-muted-foreground'>{t('No records')}</p>
                )}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('License region controls')}</CardTitle>
            </CardHeader>
            <CardContent
              className='space-y-2 text-sm'
              aria-busy={regionMutation.isPending}
            >
              {(licenses.data?.data?.licenses || []).map((license) => (
                <RegionRow
                  key={`${license.id}-${license.region || ''}`}
                  license={license}
                  saving={regionMutation.isPending}
                  onSave={(region) =>
                    license.user_id &&
                    regionMutation.mutate({ userID: license.user_id, region })
                  }
                />
              ))}
            </CardContent>
          </Card>

          <div className='grid gap-4 lg:grid-cols-2'>
            <ReviewCard
              title={t('Withdrawal review')}
              rows={withdrawals.data?.data?.withdrawals || []}
              locale={locale}
              kind='withdrawal'
              onDone={refresh}
            />
            <ReviewCard
              title={t('Manual unbind review')}
              rows={unbinds.data?.data?.requests || []}
              kind='unbind'
              onDone={refresh}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Signing public keys')}</CardTitle>
            </CardHeader>
            <CardContent
              className='space-y-2 text-sm'
              aria-busy={signingKeyMutation.isPending}
            >
              {(keys.data?.data?.keys || []).map((key) => (
                <div
                  key={key.key_id}
                  className='flex flex-wrap items-center justify-between gap-2 border-b pb-2'
                >
                  <span>
                    {key.key_id} · {key.algorithm} · {key.status}
                  </span>
                  {key.is_current && (
                    <span className='text-muted-foreground'>
                      {t('Current')}
                    </span>
                  )}
                  {!key.is_current && key.status !== 'retired' && (
                    <Button
                      type='button'
                      size='sm'
                      variant='destructive'
                      disabled={signingKeyMutation.isPending}
                      onClick={() => {
                        if (window.confirm(t('Confirm retire signing key'))) {
                          signingKeyMutation.mutate(key.key_id)
                        }
                      }}
                    >
                      {t('Retire')}
                    </Button>
                  )}
                </div>
              ))}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('Daily reconciliation')}</CardTitle>
            </CardHeader>
            <CardContent className='grid gap-2 text-sm sm:grid-cols-3 lg:grid-cols-6'>
              <span>
                {t('Orders')}: {reconciliation.data?.data?.orders ?? 0}
              </span>
              <span>
                {t('Paid orders')}:{' '}
                {reconciliation.data?.data?.paid_orders ?? 0}
              </span>
              <span>
                {t('Payment events')}:{' '}
                {reconciliation.data?.data?.payment_events ?? 0}
              </span>
              <span>
                {t('Chargebacks')}:{' '}
                {reconciliation.data?.data?.chargeback_orders ?? 0}
              </span>
              <span className='text-destructive'>
                {t('Mismatches')}:{' '}
                {reconciliation.data?.data?.mismatches.length ?? 0}
              </span>
              <span className='text-destructive'>
                {t('Pending expired')}:{' '}
                {reconciliation.data?.data?.pending_expired ?? 0}
              </span>
              {(reconciliation.data?.data?.mismatches.length ?? 0) > 0 && (
                <div className='space-y-2 border-t pt-3 sm:col-span-3 lg:col-span-6'>
                  <div className='font-medium'>
                    {t('Reconciliation details')}
                  </div>
                  {reconciliation.data?.data?.mismatches.map((mismatch) => (
                    <div
                      key={`${mismatch.kind}-${mismatch.order_id || 'unknown'}-${mismatch.provider_event_id || 'unknown'}`}
                      className='rounded border p-2 text-xs'
                    >
                      <span className='font-medium'>{mismatch.kind}</span>
                      {mismatch.order_id && (
                        <span> · order {mismatch.order_id}</span>
                      )}
                      {mismatch.provider_event_id && (
                        <span> · event {mismatch.provider_event_id}</span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function RegionRow(props: {
  license: {
    id: string
    user_id?: number
    license_code: string
    status?: string
    mode?: string
    device_hash?: string
    region?: string
  }
  onSave: (region: string) => void
  saving: boolean
}) {
  const { t } = useTranslation()
  const [region, setRegion] = useState(props.license.region || '')
  return (
    <div className='flex flex-wrap items-center gap-2 border-b pb-2'>
      <div
        className='min-w-64'
        role='group'
        aria-label={props.license.license_code}
      >
        <div className='font-medium'>
          {props.license.license_code} · user {props.license.user_id}
        </div>
        <div className='text-muted-foreground text-xs'>
          {t('Status')}: {props.license.status || '—'} · {t('Mode')}:{' '}
          {props.license.mode || '—'}
        </div>
        <div className='text-muted-foreground font-mono text-xs'>
          {t('Device')}:{' '}
          {props.license.device_hash
            ? `${props.license.device_hash.slice(0, 12)}…`
            : t('Unbound')}
        </div>
      </div>
      <select
        aria-label={`${t('Region')} ${props.license.license_code}`}
        autoComplete='off'
        name={`region_${props.license.id}`}
        value={region}
        onChange={(event) => setRegion(event.target.value)}
        disabled={props.saving}
        className='h-8 rounded-lg border bg-transparent px-2 text-sm'
      >
        <option value=''>{t('Not locked')}</option>
        <option value='CN'>CN</option>
        <option value='INTL'>INTL</option>
      </select>
      <Button
        type='button'
        size='sm'
        aria-label={`${t('Save region')} ${props.license.license_code}`}
        onClick={() => props.onSave(region)}
        disabled={!props.license.user_id || !region || props.saving}
      >
        {t('Save region')}
      </Button>
    </div>
  )
}

function ReviewCard(props: {
  title: string
  rows: Array<{
    id: string
    user_id?: number
    status: string
    amount_minor?: number
    currency?: string
    reason?: string
  }>
  locale?: string
  kind: 'unbind' | 'withdrawal'
  onDone: () => void
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{props.title}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3 text-sm'>
        {props.rows.map((row) => (
          <div key={row.id} className='space-y-2 border-b pb-3'>
            <div>
              {row.id} · user {row.user_id} · {row.status}{' '}
              {row.amount_minor !== undefined && row.currency
                ? formatD2SMoney(row.amount_minor, row.currency, props.locale)
                : (row.amount_minor ?? row.reason)}
            </div>
            {props.kind === 'withdrawal' && row.status === 'pending' && (
              <WithdrawalPayoutReveal withdrawalID={row.id} />
            )}
            {row.status === 'pending' && (
              <ReviewActions
                id={row.id}
                kind={props.kind}
                onDone={props.onDone}
              />
            )}
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
