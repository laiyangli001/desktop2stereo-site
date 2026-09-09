import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  TencentSESEnabled: z.boolean(),
  TencentSESRegion: z.string().min(1),
  TencentSESSecretId: z.string(),
  TencentSESSecretKey: z.string(),
  TencentSESFromEmail: z.string(),
  TencentSESFromName: z.string(),
  TencentSESReplyTo: z.string(),
  TencentSESSubjectPrefix: z.string(),
  TencentSESTimeoutSeconds: z.string(),
  TencentSESRetryCount: z.string(),
  TencentSESTemplates: z.string(),
})

type TencentSESFormValues = z.infer<typeof schema>
type TemplateMappingValue = number | Record<string, number>
type TemplateMapping = Record<string, TemplateMappingValue>
type EmailDeliveryLog = {
  id: number
  created_at: number
  scene: string
  recipient: string
  status: string
  request_id?: string
  message_id?: string
  error?: string
}

function parseTemplateMapping(
  raw: string,
  invalidMessage: string
): TemplateMapping {
  const parsed: unknown = JSON.parse(raw || '{}')
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(invalidMessage)
  }
  const mapping: TemplateMapping = {}
  for (const [scene, value] of Object.entries(parsed)) {
    if (!scene.trim()) throw new Error(invalidMessage)
    if (typeof value === 'number' && Number.isInteger(value) && value > 0) {
      mapping[scene] = value
      continue
    }
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(invalidMessage)
    }
    const localized: Record<string, number> = {}
    for (const [language, templateId] of Object.entries(value)) {
      if (
        !language.trim() ||
        typeof templateId !== 'number' ||
        !Number.isInteger(templateId) ||
        templateId <= 0
      ) {
        throw new Error(invalidMessage)
      }
      localized[language] = templateId
    }
    if (Object.keys(localized).length === 0) throw new Error(invalidMessage)
    mapping[scene] = localized
  }
  if (Object.keys(mapping).length === 0) throw new Error(invalidMessage)
  return mapping
}

function firstTemplateId(
  mapping: TemplateMapping,
  language: string
): [string, number] | null {
  const normalizedLanguage = language.toLowerCase().replace('_', '-')
  const languageCandidates = normalizedLanguage.startsWith('en')
    ? ['en', 'zhCN']
    : ['zhCN', 'en']
  for (const [scene, value] of Object.entries(mapping)) {
    if (typeof value === 'number') return [scene, value]
    for (const candidate of languageCandidates) {
      if (value[candidate]) return [scene, value[candidate]]
    }
    const fallback = Object.values(value)[0]
    if (fallback) return [scene, fallback]
  }
  return null
}

type Props = { defaultValues: TencentSESFormValues }

export function TencentSESSettingsSection({ defaultValues }: Props) {
  const { t, i18n } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const form = useForm<TencentSESFormValues>({
    resolver: zodResolver(schema),
    defaultValues,
  })
  useResetForm(form, defaultValues)
  const [testRecipient, setTestRecipient] = useState('')
  const [logPage, setLogPage] = useState(1)
  const logsQuery = useQuery({
    queryKey: ['tencent-ses-delivery-logs', logPage],
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        data: {
          logs: EmailDeliveryLog[]
          page: number
          page_size: number
          has_more: boolean
        }
      }>(`/api/option/tencent-ses/logs?page=${logPage}`)
      return response.data.data
    },
  })
  const deleteLog = useMutation({
    mutationFn: (id: number) =>
      api.delete(`/api/option/tencent-ses/logs/${id}`),
    onSuccess: async () => {
      if ((logsQuery.data?.logs.length ?? 0) === 1 && logPage > 1) {
        setLogPage((page) => page - 1)
      }
      toast.success(t('Email delivery log deleted'))
      await queryClient.invalidateQueries({
        queryKey: ['tencent-ses-delivery-logs'],
      })
    },
    onError: () => toast.error(t('Unable to delete email delivery log')),
  })
  const deleteAllLogs = useMutation({
    mutationFn: () => api.delete('/api/option/tencent-ses/logs'),
    onSuccess: async () => {
      setLogPage(1)
      toast.success(t('All email delivery logs deleted'))
      await queryClient.invalidateQueries({
        queryKey: ['tencent-ses-delivery-logs'],
      })
    },
    onError: () => toast.error(t('Unable to delete email delivery logs')),
  })

  const testConnection = async () => {
    try {
      const response = await api.post<{ success: boolean; message: string }>(
        '/api/option/tencent-ses/test-connection'
      )
      if (response.data.success) toast.success(response.data.message)
      else toast.error(response.data.message)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Connection test failed')
      )
    }
  }

  const testSend = async () => {
    if (!testRecipient.trim()) {
      toast.error(t('Enter a test recipient first'))
      return
    }
    try {
      const templates = parseTemplateMapping(
        form.getValues('TencentSESTemplates'),
        t('Template mapping must be a JSON object of positive numeric IDs')
      )
      const selected = firstTemplateId(
        templates,
        i18n.resolvedLanguage || i18n.language
      )
      if (!selected) {
        toast.error(t('Configure at least one TemplateID first'))
        return
      }
      const [scene, templateId] = selected
      const response = await api.post<{ success: boolean; message: string }>(
        '/api/option/tencent-ses/test-send',
        {
          scene,
          template_id: Number(templateId),
          to: testRecipient.trim(),
          subject: t('Tencent SES test'),
          data: {},
        }
      )
      if (response.data.success) toast.success(response.data.message)
      else toast.error(response.data.message)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Test email failed')
      )
    }
  }

  const onSubmit = async (values: TencentSESFormValues) => {
    let templates: TemplateMapping
    try {
      templates = parseTemplateMapping(
        values.TencentSESTemplates,
        t('Template mapping must be a JSON object of positive numeric IDs')
      )
    } catch (error) {
      form.setError('TencentSESTemplates', {
        message:
          error instanceof Error
            ? error.message
            : t('Invalid template mapping'),
      })
      return
    }

    const updates: Array<{ key: string; value: string | boolean }> = [
      { key: 'TencentSESEnabled', value: values.TencentSESEnabled },
      { key: 'TencentSESRegion', value: values.TencentSESRegion.trim() },
      { key: 'TencentSESFromEmail', value: values.TencentSESFromEmail.trim() },
      { key: 'TencentSESFromName', value: values.TencentSESFromName.trim() },
      { key: 'TencentSESReplyTo', value: values.TencentSESReplyTo.trim() },
      {
        key: 'TencentSESSubjectPrefix',
        value: values.TencentSESSubjectPrefix.trim(),
      },
      {
        key: 'TencentSESTimeoutSeconds',
        value: values.TencentSESTimeoutSeconds.trim(),
      },
      {
        key: 'TencentSESRetryCount',
        value: values.TencentSESRetryCount.trim(),
      },
      { key: 'TencentSESTemplates', value: JSON.stringify(templates) },
    ]
    if (values.TencentSESSecretId.trim())
      updates.push({
        key: 'TencentSESSecretId',
        value: values.TencentSESSecretId.trim(),
      })
    if (values.TencentSESSecretKey.trim())
      updates.push({
        key: 'TencentSESSecretKey',
        value: values.TencentSESSecretKey.trim(),
      })
    for (const update of updates) await updateOption.mutateAsync(update)
  }

  return (
    <div className='space-y-8'>
      <SettingsSection title={t('Email delivery')}>
        <Form {...form}>
          <SettingsForm
            onSubmit={form.handleSubmit(onSubmit)}
            autoComplete='off'
          >
            <SettingsPageFormActions
              onSave={form.handleSubmit(onSubmit)}
              isSaving={updateOption.isPending}
              saveLabel={t('Save email delivery settings')}
            />
            <FormField
              control={form.control}
              name='TencentSESEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable email delivery')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Use approved Tencent Cloud templates for transactional email'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={testConnection}
                disabled={updateOption.isPending}
              >
                {t('Test email delivery connection')}
              </Button>
              <FormDescription className='self-center'>
                {t(
                  'Save credentials and at least one TemplateID before testing.'
                )}
              </FormDescription>
            </div>
            <FormItem>
              <FormLabel>{t('Test recipient')}</FormLabel>
              <FormControl>
                <Input
                  value={testRecipient}
                  onChange={(event) => setTestRecipient(event.target.value)}
                  placeholder='you@example.com'
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Used only when you click Send test email; it is not persisted.'
                )}
              </FormDescription>
            </FormItem>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={testSend}
                disabled={updateOption.isPending}
              >
                {t('Send test email')}
              </Button>
            </div>
            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='TencentSESRegion'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Tencent Cloud region')}</FormLabel>
                    <FormControl>
                      <Input placeholder='ap-guangzhou' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('For example, ap-guangzhou or ap-hongkong')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentSESFromEmail'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('From address')}</FormLabel>
                    <FormControl>
                      <Input placeholder='noreply@example.com' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('Must belong to the verified SES identity')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentSESFromName'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Sender name')}</FormLabel>
                    <FormControl>
                      <Input placeholder='New API' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Optional display name shown before the verified sender address'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentSESReplyTo'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Reply-to address')}</FormLabel>
                    <FormControl>
                      <Input placeholder='support@example.com' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentSESSubjectPrefix'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Subject prefix')}</FormLabel>
                    <FormControl>
                      <Input placeholder='[New API] ' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name='TencentSESSecretId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>SecretId</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Leave blank to keep existing credential')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='TencentSESSecretKey'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>SecretKey</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Leave blank to keep existing credential')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className='grid gap-6 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='TencentSESTimeoutSeconds'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Request timeout (seconds)')}</FormLabel>
                    <FormControl>
                      <Input type='number' min='1' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentSESRetryCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Retry count')}</FormLabel>
                    <FormControl>
                      <Input type='number' min='0' max='5' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name='TencentSESTemplates'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Template ID mapping')}</FormLabel>
                  <FormControl>
                    <Textarea
                      className='min-h-48 font-mono'
                      placeholder='{"email_verification":{"zhCN":123,"en":456},"password_reset":{"zhCN":789,"en":790}}'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Use a language mapping such as zhCN and en for localized templates. Legacy scene-to-ID JSON remains supported.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsForm>
        </Form>
      </SettingsSection>
      <SettingsSection title={t('Recent email delivery logs')}>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <span className='text-muted-foreground text-sm'>
            {t('Page {{page}}', { page: logPage })}
          </span>
          <Button
            type='button'
            variant='destructive'
            disabled={
              deleteAllLogs.isPending ||
              deleteLog.isPending ||
              logsQuery.isLoading
            }
            onClick={() => {
              if (window.confirm(t('Confirm delete all email delivery logs'))) {
                deleteAllLogs.mutate()
              }
            }}
          >
            {deleteAllLogs.isPending ? t('Deleting…') : t('Delete all records')}
          </Button>
        </div>
        <div className='space-y-2 text-sm'>
          {(logsQuery.data?.logs ?? []).map((log) => (
            <div key={log.id} className='bg-muted/20 rounded-lg border p-3'>
              <div className='flex flex-wrap justify-between gap-2'>
                <span className='font-medium'>
                  {log.scene} → {log.recipient}
                </span>
                <div className='flex items-center gap-2'>
                  <span
                    className={
                      log.status === 'sent'
                        ? 'text-green-600'
                        : 'text-destructive'
                    }
                  >
                    {log.status}
                  </span>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    disabled={deleteLog.isPending || deleteAllLogs.isPending}
                    onClick={() => {
                      if (
                        window.confirm(t('Confirm delete email delivery log'))
                      ) {
                        deleteLog.mutate(log.id)
                      }
                    }}
                  >
                    {t('Delete')}
                  </Button>
                </div>
              </div>
              <div className='text-muted-foreground mt-1 text-xs'>
                RequestId: {log.request_id || '-'} · MessageId:{' '}
                {log.message_id || '-'}
              </div>
              {log.error && (
                <div className='text-destructive mt-1 text-xs'>{log.error}</div>
              )}
            </div>
          ))}
          {!logsQuery.isLoading && logsQuery.data?.logs.length === 0 && (
            <p className='text-muted-foreground'>
              {t('No email delivery logs yet.')}
            </p>
          )}
        </div>
        {(logPage > 1 || logsQuery.data?.has_more) && (
          <div className='mt-4 flex flex-wrap items-center justify-between gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={logPage <= 1 || logsQuery.isFetching}
              onClick={() => setLogPage((page) => page - 1)}
            >
              {t('Previous page')}
            </Button>
            <span className='text-muted-foreground text-sm'>
              {t('Page {{page}}', { page: logPage })}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={!logsQuery.data?.has_more || logsQuery.isFetching}
              onClick={() => setLogPage((page) => page + 1)}
            >
              {t('Next page')}
            </Button>
          </div>
        )}
      </SettingsSection>
    </div>
  )
}
