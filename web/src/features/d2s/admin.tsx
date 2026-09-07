import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

import {
  getD2SAdminBalances,
  getD2SAdminLicenses,
  getD2SAdminOrders,
  getD2SAdminPaymentEvents,
  getD2SAdminSigningKeys,
  getD2SAdminReconciliation,
  getD2SAdminUnbindRequests,
  getD2SAdminWithdrawals,
  retireD2SAdminSigningKey,
  reviewD2SUnbind,
  reviewD2SWithdrawal,
  setD2SUserRegion,
} from './api'

function formatPaymentEventDate(timestamp: number): string {
  if (!timestamp) return '—'
  return new Date(timestamp * 1000).toLocaleString()
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
    <div className='flex min-w-52 gap-2'>
      <Input
        aria-label={t('Review note')}
        value={note}
        onChange={(event) => setNote(event.target.value)}
        placeholder={t('Review note')}
      />
      <Button
        type='button'
        size='sm'
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
        disabled={mutation.isPending}
        onClick={() => mutation.mutate('rejected')}
      >
        {t('Reject')}
      </Button>
    </div>
  )
}

export function D2SAdminWorkspace() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [eventFilters, setEventFilters] = useState({
    provider: '',
    order_id: '',
    event_type: '',
  })
  const [activeEventFilters, setActiveEventFilters] = useState(eventFilters)
  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: ['d2s-admin'] })
  const licenses = useQuery({
    queryKey: ['d2s-admin', 'licenses'],
    queryFn: getD2SAdminLicenses,
  })
  const orders = useQuery({
    queryKey: ['d2s-admin', 'orders'],
    queryFn: () => getD2SAdminOrders(),
  })
  const paymentEvents = useQuery({
    queryKey: ['d2s-admin', 'payment-events', activeEventFilters],
    queryFn: () => getD2SAdminPaymentEvents(activeEventFilters),
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

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Desktop2Stereo Admin')}
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
              <CardContent className='space-y-2 text-sm'>
                {(orders.data?.data?.orders || []).map((order) => (
                  <div
                    key={order.id}
                    className='flex flex-wrap justify-between gap-2 border-b pb-2'
                  >
                    <span>
                      {order.id} · user {order.user_id} · {order.provider}
                    </span>
                    <span>
                      {order.status} · {order.currency} {order.amount_minor}
                    </span>
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
                      {account.available_minor}
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
                <CardTitle>{t('Payment events')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3 text-sm'>
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
                      {event.provider} · {event.event_type} · {event.currency}{' '}
                      {event.amount_minor}
                    </div>
                    <div className='text-muted-foreground font-mono text-xs'>
                      {event.provider_event_id} · {event.order_id}
                    </div>
                    <div className='text-muted-foreground text-xs'>
                      {t('Processed at')}:{' '}
                      {formatPaymentEventDate(event.processed_at)}
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
            <CardContent className='space-y-2 text-sm'>
              {(licenses.data?.data?.licenses || []).map((license) => (
                <RegionRow
                  key={license.id}
                  license={license}
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
            <CardContent className='space-y-2 text-sm'>
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
}) {
  const { t } = useTranslation()
  const [region, setRegion] = useState(props.license.region || '')
  useEffect(() => {
    setRegion(props.license.region || '')
  }, [props.license.region])
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
        aria-label={t('Region')}
        value={region}
        onChange={(event) => setRegion(event.target.value)}
        className='h-8 rounded-lg border bg-transparent px-2 text-sm'
      >
        <option value=''>{t('Not locked')}</option>
        <option value='CN'>CN</option>
        <option value='INTL'>INTL</option>
      </select>
      <Button
        type='button'
        size='sm'
        onClick={() => props.onSave(region)}
        disabled={!props.license.user_id || !region}
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
    reason?: string
  }>
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
              {row.amount_minor ?? row.reason}
            </div>
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
