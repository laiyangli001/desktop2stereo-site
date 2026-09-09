import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { createD2SWithdrawal } from '../api'
import {
  createD2SWithdrawalFormSchema,
  d2sWithdrawalFormSchema,
} from '../lib/withdrawal'

type D2SWithdrawalFormValues = z.infer<typeof d2sWithdrawalFormSchema>

type WithdrawalRequestFormProps = {
  availableMinor: number
  minWithdrawalMinor?: number
  onSuccess: () => void
}

export function WithdrawalRequestForm(props: WithdrawalRequestFormProps) {
  const { t } = useTranslation()
  const minWithdrawalMinor = props.minWithdrawalMinor ?? 5000
  const form = useForm<D2SWithdrawalFormValues>({
    resolver: zodResolver(
      createD2SWithdrawalFormSchema(minWithdrawalMinor)
    ),
    defaultValues: {
      amountMinor: minWithdrawalMinor,
      alipayAccount: '',
      realName: '',
    },
  })

  async function onSubmit(data: D2SWithdrawalFormValues) {
    try {
      const result = await createD2SWithdrawal({
        amount_minor: data.amountMinor,
        alipay_account: data.alipayAccount,
        real_name: data.realName,
      })
      if (!result.success) {
        toast.error(result.error?.message || t('Operation failed'))
        return
      }
      form.reset()
      props.onSuccess()
      toast.success(t('Withdrawal request submitted'))
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='grid gap-3'>
        <FormField
          control={form.control}
          name='amountMinor'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Amount (minor units)')}</FormLabel>
              <FormControl>
                <Input
                  name={field.name}
                  ref={field.ref}
                  onBlur={field.onBlur}
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(event.target.valueAsNumber)
                  }
                  type='number'
                  min={minWithdrawalMinor}
                  max={props.availableMinor}
                  step={1}
                  inputMode='numeric'
                  aria-describedby='d2s-withdrawal-amount-help'
                />
              </FormControl>
              <p
                id='d2s-withdrawal-amount-help'
                className='text-muted-foreground text-xs'
              >
                {t('Available CNY balance: {{amount}} minor units', {
                  amount: props.availableMinor,
                })}
              </p>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='alipayAccount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Alipay account')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  autoComplete='username'
                  placeholder={t('Alipay account')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='realName'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Real name')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  autoComplete='name'
                  placeholder={t('Real name')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button
          type='submit'
          disabled={
            form.formState.isSubmitting ||
            props.availableMinor < minWithdrawalMinor
          }
        >
          {form.formState.isSubmitting
            ? t('Submitting')
            : t('Request withdrawal')}
        </Button>
      </form>
    </Form>
  )
}
