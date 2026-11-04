import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

import {
  saveD2SAdminAlipayPCConfig,
  useD2SAdminAlipayEnvironmentConfig,
  type D2SAdminAlipayPCSettings,
} from './api'

type AlipayPCConfigFormState = {
  qr_pay_mode: string
  channel_enabled: boolean
  app_id: string
  seller_id: string
  gateway_url: string
  notify_url: string
  return_url: string
  merchant_private_key: string
  alipay_public_key: string
}

export function AlipayPCConfigForm(props: {
  settings?: D2SAdminAlipayPCSettings
  showQRMode?: boolean
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const config = props.settings
  const [form, setForm] = useState<AlipayPCConfigFormState>(() => ({
    qr_pay_mode: config?.qr_pay_mode || '4',
    channel_enabled: config?.channel_enabled ?? false,
    app_id: config?.app_id ?? '',
    seller_id: config?.seller_id ?? '',
    gateway_url: config?.gateway_url ?? 'https://openapi.alipay.com/gateway.do',
    notify_url:
      config?.notify_url ?? 'https://d2s.site/api/v1/alipay/d2s/notify',
    return_url: config?.return_url ?? 'https://d2s.site/d2s',
    merchant_private_key: '',
    alipay_public_key: '',
  }))

  const saveMutation = useMutation({
    mutationFn: saveD2SAdminAlipayPCConfig,
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      setForm((current) => ({
        ...current,
        merchant_private_key: '',
        alipay_public_key: '',
      }))
      props.onSaved()
      toast.success(t('Alipay settings saved'))
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const environmentMutation = useMutation({
    mutationFn: useD2SAdminAlipayEnvironmentConfig,
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      props.onSaved()
      toast.success(t('Using server environment settings'))
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const busy = saveMutation.isPending || environmentMutation.isPending
  const update = (
    name: keyof AlipayPCConfigFormState,
    value: string | boolean
  ) => setForm((current) => ({ ...current, [name]: value }))

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Alipay PC payment configuration')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4 text-sm'>
        <div className='rounded-md border p-3'>
          <p>
            {t('Configuration source')}: {config?.config_source || t('Loading')}
          </p>
          <p>
            {t('Configuration status')}:{' '}
            {config?.ready ? t('Ready') : t('Incomplete')}
          </p>
          <p>
            {t('Merchant private key')}:{' '}
            {config?.merchant_private_key_configured
              ? t('Configured')
              : t('Not configured')}
            {' · '}
            {t('Alipay public key')}:{' '}
            {config?.alipay_public_key_configured
              ? t('Configured')
              : t('Not configured')}
          </p>
        </div>
        <form
          className='grid gap-3 sm:grid-cols-2'
          onSubmit={(event) => {
            event.preventDefault()
            saveMutation.mutate({
              ...form,
              merchant_private_key: form.merchant_private_key || undefined,
              alipay_public_key: form.alipay_public_key || undefined,
            })
          }}
        >
          <label className='grid gap-1'>
            <span>{t('Alipay app ID')}</span>
            <Input
              autoComplete='off'
              name='alipay_app_id'
              value={form.app_id}
              onChange={(event) => update('app_id', event.target.value)}
            />
          </label>
          <label className='grid gap-1'>
            <span>{t('Seller ID')}</span>
            <Input
              autoComplete='off'
              name='alipay_seller_id'
              value={form.seller_id}
              onChange={(event) => update('seller_id', event.target.value)}
            />
          </label>
          <label className='grid gap-1'>
            <span>{t('Alipay gateway URL')}</span>
            <select
              aria-label={t('Alipay gateway URL')}
              className='bg-background h-9 rounded-md border px-3'
              name='alipay_gateway_url'
              value={form.gateway_url}
              onChange={(event) => update('gateway_url', event.target.value)}
            >
              <option value='https://openapi.alipay.com/gateway.do'>
                {t('Production gateway')}
              </option>
              <option value='https://openapi-sandbox.dl.alipaydev.com/gateway.do'>
                {t('Sandbox gateway')}
              </option>
            </select>
          </label>
          <label className='grid gap-1'>
            <span>{t('Notify URL')}</span>
            <Input
              autoComplete='url'
              name='alipay_notify_url'
              value={form.notify_url}
              onChange={(event) => update('notify_url', event.target.value)}
            />
          </label>
          <label className='grid gap-1'>
            <span>{t('Return URL')}</span>
            <Input
              autoComplete='url'
              name='alipay_return_url'
              value={form.return_url}
              onChange={(event) => update('return_url', event.target.value)}
            />
          </label>
          <label className='grid gap-1'>
            <span>
              {t('Merchant RSA2 private key')}
              {config?.merchant_private_key_configured
                ? ` · ${t('Saved; leave blank to keep current')}`
                : ''}
            </span>
            <Input
              type='password'
              autoComplete='new-password'
              name='merchant_private_key'
              value={form.merchant_private_key}
              onChange={(event) =>
                update('merchant_private_key', event.target.value)
              }
            />
          </label>
          <label className='grid gap-1'>
            <span>
              {t('Alipay RSA2 public key')}
              {config?.alipay_public_key_configured
                ? ` · ${t('Saved; leave blank to keep current')}`
                : ''}
            </span>
            <Input
              type='password'
              autoComplete='new-password'
              name='alipay_public_key'
              value={form.alipay_public_key}
              onChange={(event) =>
                update('alipay_public_key', event.target.value)
              }
            />
          </label>
          <p className='text-muted-foreground sm:col-span-2'>
            {t(
              'Paste key as one line with literal backslash-n separators or as base64 PEM. Keys are encrypted at rest and never returned by the API.'
            )}
          </p>
          {props.showQRMode !== false && (
            <label className='grid gap-1'>
              <span>{t('QR display mode')}</span>
              <select
                aria-label={t('QR display mode')}
                className='bg-background h-9 rounded-md border px-3'
                value={form.qr_pay_mode}
                onChange={(event) => update('qr_pay_mode', event.target.value)}
                disabled={busy}
              >
                <option value='0'>0 · {t('Compact landscape')}</option>
                <option value='1'>1 · {t('Portrait')}</option>
                <option value='3'>3 · {t('Mini')}</option>
                <option value='4'>4 · {t('Embedded QR')}</option>
              </select>
            </label>
          )}
          <label className='flex items-center gap-2 sm:col-span-2'>
            <input
              type='checkbox'
              checked={form.channel_enabled}
              onChange={(event) =>
                update('channel_enabled', event.target.checked)
              }
            />
            <span>{t('Enable Alipay PC channel')}</span>
          </label>
          <div className='flex flex-wrap gap-2 sm:col-span-2'>
            <Button type='submit' disabled={busy}>
              {busy ? t('Saving') : t('Save Alipay settings')}
            </Button>
            {config?.config_source === 'admin' && (
              <Button
                type='button'
                variant='outline'
                disabled={busy}
                onClick={() => environmentMutation.mutate()}
              >
                {t('Use server environment settings')}
              </Button>
            )}
          </div>
        </form>
        <div className='border-t pt-3 text-muted-foreground'>
          <p>
            {t('Alipay API method')}: alipay.trade.page.pay · RSA2 · JSON ·
            UTF-8
          </p>
          <p>
            {t('Application gateway (set in Alipay Open Platform)')}:{' '}
            https://d2s.site/api/v1/alipay/app-gateway
          </p>
          <p>
            {t(
              'The global purchase switch and payment compliance gate remain separate and must also be enabled before checkout.'
            )}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}
