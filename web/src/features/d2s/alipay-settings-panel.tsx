import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { AlipayPCConfigForm } from './alipay-config-form'
import { getD2SAdminAlipayPCSettings } from './api'

export function AlipayPCSettingsPanel() {
  const { t } = useTranslation()
  const settings = useQuery({
    queryKey: ['d2s-admin', 'alipay-settings'],
    queryFn: getD2SAdminAlipayPCSettings,
  })
  if (settings.isPending) return <p role='status'>{t('Loading')}</p>
  if (settings.isError || !settings.data?.success || !settings.data.data) {
    return (
      <div className='space-y-3'>
        <p role='alert'>{t('Unable to load Alipay settings')}</p>
        <Button type='button' onClick={() => void settings.refetch()}>
          {t('Retry')}
        </Button>
      </div>
    )
  }
  return (
    <AlipayPCConfigForm
      key={JSON.stringify(settings.data.data)}
      settings={settings.data.data}
      onSaved={() => void settings.refetch()}
    />
  )
}
