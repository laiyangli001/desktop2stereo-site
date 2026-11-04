import { KeyRound, Monitor, ShieldCheck, Unplug } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { formatD2SDate } from '../lib/format'
import type { D2SLicense } from '../types'

const statusLabels: Record<string, string> = {
  active: 'Active',
  expired: 'Expired',
  suspended: 'Suspended',
  revoked: 'Revoked',
  pending: 'Pending',
  approved: 'Approved',
  rejected: 'Rejected',
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
  const bound = Boolean(props.license.device_hash)
  const active = props.license.status === 'active'
  const permanent = props.license.mode === 'permanent'
  const title =
    props.license.kind === 'trial' ? t('Trial license') : t('Full license')
  const modeLabels: Record<string, string> = {
    online: t('Online mode'),
    offline: t('Offline mode'),
    permanent: t('Permanent binding'),
  }

  return (
    <Card className='min-w-0 gap-0 overflow-hidden py-0'>
      <CardHeader className='gap-4 border-b bg-muted/30 px-5 py-5'>
        <div className='flex items-start justify-between gap-3'>
          <div className='flex min-w-0 items-center gap-3'>
            <div className='bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-xl'>
              <KeyRound className='size-5' aria-hidden='true' />
            </div>
            <div className='min-w-0'>
              <CardTitle className='text-base'>{title}</CardTitle>
              <p className='text-muted-foreground mt-1 break-all font-mono text-xs'>
                {props.license.license_code}
              </p>
            </div>
          </div>
          <Badge
            variant={active ? 'secondary' : 'outline'}
            className={
              active
                ? 'shrink-0 border-emerald-600/20 bg-emerald-500/10 text-emerald-800 dark:text-emerald-300'
                : 'shrink-0'
            }
          >
            {t(statusLabels[props.license.status] ?? props.license.status)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className='space-y-5 p-5' aria-busy={props.actionPending}>
        <dl className='grid grid-cols-2 gap-x-4 gap-y-4 text-sm'>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{t('Usage mode')}</dt>
            <dd className='mt-1 font-medium'>
              {modeLabels[props.license.mode] ?? props.license.mode}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{t('Expires')}</dt>
            <dd className='mt-1 break-words font-medium'>
              {props.license.expires_at
                ? formatD2SDate(props.license.expires_at, locale)
                : t('No expiry')}
            </dd>
          </div>
          <div className='col-span-2 min-w-0'>
            <dt className='text-muted-foreground text-xs'>
              {t('Offline access until')}
            </dt>
            <dd className='mt-1 break-words font-medium'>
              {formatD2SDate(props.license.offline_valid_until, locale)}
            </dd>
          </div>
        </dl>

        <div className='flex items-start gap-3 rounded-lg border bg-muted/20 p-3'>
          <Monitor
            className='text-muted-foreground mt-0.5 size-4 shrink-0'
            aria-hidden='true'
          />
          <div className='min-w-0 text-sm'>
            <p className='font-medium'>{t('Device binding')}</p>
            <p
              className='text-muted-foreground mt-1 break-all font-mono text-xs'
              title={props.license.device_hash}
            >
              {props.license.device_hash || t('Unbound')}
            </p>
            {!bound && (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('No device is currently linked to this license.')}
              </p>
            )}
          </div>
        </div>

        {active && bound && !permanent && (
          <div className='space-y-4 border-t pt-4'>
            <fieldset className='space-y-3' disabled={props.actionPending}>
              <legend className='mb-2 text-sm font-medium'>
                {t('Usage mode')}
              </legend>
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  size='sm'
                  aria-label={`${t('Online mode')} ${props.license.license_code}`}
                  aria-pressed={props.license.mode === 'online'}
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
                  aria-label={`${t('Offline mode')} ${props.license.license_code}`}
                  aria-pressed={props.license.mode === 'offline'}
                  variant={
                    props.license.mode === 'offline' ? 'default' : 'outline'
                  }
                  disabled={props.actionPending}
                  onClick={() => props.onChangeMode('offline', offlineDays)}
                >
                  {t('Offline mode')}
                </Button>
              </div>
              <label className='flex flex-wrap items-center justify-between gap-2 text-sm'>
                <span className='text-muted-foreground'>
                  {t('Offline period')}
                </span>
                <select
                  aria-label={`${t('Offline period')} ${props.license.license_code}`}
                  autoComplete='off'
                  className='focus-visible:ring-ring h-9 rounded-md border bg-background px-3 text-sm focus-visible:ring-2'
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
            </fieldset>
            <div className='space-y-3 border-t pt-4'>
              <p className='text-sm font-medium'>{t('Device actions')}</p>
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  aria-label={`${t('Make permanent')} ${props.license.license_code}`}
                  disabled={props.actionPending}
                  onClick={props.onConfirmPermanent}
                >
                  <ShieldCheck className='size-4' aria-hidden='true' />
                  {t('Make permanent')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  className='text-destructive hover:bg-destructive/10 hover:text-destructive'
                  aria-label={`${t('Free revoke')} ${props.license.license_code}`}
                  disabled={props.actionPending}
                  onClick={props.onFreeRevoke}
                >
                  <Unplug className='size-4' aria-hidden='true' />
                  {t('Free revoke')}
                </Button>
              </div>
            </div>
          </div>
        )}

        {active && bound && permanent && (
          <div className='space-y-3 border-t pt-4'>
            {props.manualUnbindStatus && (
              <p className='text-muted-foreground flex flex-wrap items-center gap-2 text-sm'>
                <span>{t('Manual unbind status')}:</span>
                <Badge variant='outline'>
                  {t(
                    statusLabels[props.manualUnbindStatus] ??
                      props.manualUnbindStatus
                  )}
                </Badge>
              </p>
            )}
            <label className='grid gap-2 text-sm'>
              <span className='font-medium'>{t('Manual unbind reason')}</span>
              <input
                aria-label={`${t('Manual unbind reason')} ${props.license.license_code}`}
                autoComplete='off'
                className='focus-visible:ring-ring h-10 w-full min-w-0 rounded-md border bg-background px-3 focus-visible:ring-2'
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
              className='w-full sm:w-auto'
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
