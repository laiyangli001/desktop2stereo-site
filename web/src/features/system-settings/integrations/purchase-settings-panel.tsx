import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

const queryKey = ['system-settings', 'd2s-purchases']
type PurchaseSettings = { enabled: boolean }

export function PurchaseSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const settings = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await api.get('/api/option/d2s-purchases')
      if (
        !response.data.success ||
        typeof response.data.data?.enabled !== 'boolean'
      ) {
        throw new Error('Unable to load purchase settings')
      }
      return response.data.data as PurchaseSettings
    },
  })
  const save = useMutation({
    mutationFn: async (enabled: boolean) => {
      const response = await api.put('/api/option/d2s-purchases', { enabled })
      if (
        !response.data.success ||
        typeof response.data.data?.enabled !== 'boolean'
      ) {
        throw new Error('Unable to save purchase settings')
      }
      return response.data.data as PurchaseSettings
    },
    onSuccess: (data) => {
      queryClient.setQueryData(queryKey, data)
      void queryClient.invalidateQueries({ queryKey: ['d2s'] })
      toast.success(t('Purchase settings saved'))
    },
    onError: () => toast.error(t('Unable to save purchase settings')),
  })

  return (
    <section
      className='mb-6 space-y-3 rounded-lg border p-4'
      aria-labelledby='d2s-purchases-title'
    >
      <div className='flex items-center justify-between gap-4'>
        <label
          id='d2s-purchases-title'
          htmlFor='d2s-purchases-enabled'
          className='font-medium'
        >
          {t('Desktop2Stereo purchase master switch')}
        </label>
        <Switch
          id='d2s-purchases-enabled'
          checked={settings.data?.enabled ?? false}
          disabled={!settings.isSuccess || save.isPending}
          onCheckedChange={(enabled) => save.mutate(enabled)}
        />
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Applies to Desktop2Stereo purchases across all payment channels. Changes take effect immediately. Disabling blocks new quotes, orders and checkout; existing payment notifications and refunds continue.'
        )}
      </p>
      {settings.isPending && <p role='status'>{t('Loading')}</p>}
      {settings.isError && (
        <div className='space-y-2'>
          <p role='alert'>{t('Unable to load purchase settings')}</p>
          <Button
            type='button'
            variant='outline'
            onClick={() => void settings.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )}
    </section>
  )
}
