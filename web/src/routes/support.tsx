import { zodResolver } from '@hookform/resolvers/zod'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { submitD2SSupportTicket } from '@/features/d2s/api'
import { resolveAuthentication } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

const supportSchema = z.object({
  category: z.enum([
    'PAYMENT',
    'ORDER_LICENSE',
    'DOWNLOAD',
    'SOFTWARE',
    'ACCOUNT',
    'OTHER',
  ]),
  contact_email: z.string().trim().email().max(254),
  order_id: z.string().trim().max(64),
  subject: z.string().trim().min(1).max(200),
  message: z.string().trim().min(10).max(5000),
  website: z.string().max(255),
})

type SupportForm = z.infer<typeof supportSchema>

const defaultValues: SupportForm = {
  category: 'PAYMENT',
  contact_email: '',
  order_id: '',
  subject: '',
  message: '',
  website: '',
}

export function D2SSupportPage() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [ticketID, setTicketID] = useState('')
  const [requestError, setRequestError] = useState('')
  const userDefaultValues = {
    ...defaultValues,
    contact_email: user?.email ?? '',
  }
  const form = useForm<SupportForm>({
    resolver: zodResolver(supportSchema),
    defaultValues: userDefaultValues,
  })

  const onSubmit = form.handleSubmit(async (values) => {
    setRequestError('')
    try {
      const result = await submitD2SSupportTicket(values)
      const id = result.data?.ticket_id
      if (!result.success || !id) throw new Error('SUPPORT_SUBMISSION_FAILED')
      setTicketID(id)
      form.reset(userDefaultValues)
    } catch {
      setRequestError(t('Your message could not be sent. Please try again.'))
    }
  })

  return (
    <main className='mx-auto max-w-3xl space-y-6 px-4 py-10 sm:px-6'>
      <Link
        className='text-sm underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-4'
        to='/d2s'
      >
        {t('Back to Desktop2Stereo account')}
      </Link>
      <header className='space-y-2'>
        <h1 className='text-3xl font-semibold'>
          {t('Complaints and support')}
        </h1>
        <p className='text-muted-foreground'>
          {t(
            'Tell us about a payment, order, license, download, account, or software issue. Provide an email where we can reach you.'
          )}
        </p>
        {user ? (
          <Link
            className='inline-block text-sm underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-4'
            to='/d2s/support-tickets'
          >
            {t('View my support tickets')}
          </Link>
        ) : null}
      </header>

      {ticketID ? (
        <section
          aria-live='polite'
          className='space-y-3 rounded-xl border p-6'
          role='status'
        >
          <h2 className='text-xl font-medium'>{t('Message received')}</h2>
          <p>
            {t('Your reference number is')} <code>{ticketID}</code>
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Keep this number. For a live conversation, use the Enterprise WeChat support button.'
            )}
          </p>
          {user && (
            <Link
              className='inline-block underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-4'
              to='/d2s/support-tickets'
            >
              {t('View my support tickets')}
            </Link>
          )}
          <button
            className='hover:bg-muted rounded-md border px-4 py-2 focus-visible:outline-2 focus-visible:outline-offset-2'
            onClick={() => setTicketID('')}
            type='button'
          >
            {t('Send another message')}
          </button>
        </section>
      ) : (
        <form
          className='space-y-5 rounded-xl border p-5 sm:p-7'
          onSubmit={onSubmit}
        >
          <label className='grid gap-2 text-sm'>
            <span>{t('What do you need help with?')}</span>
            <select
              className='bg-background rounded-md border px-3 py-2'
              {...form.register('category')}
            >
              <option value='PAYMENT'>{t('Payment not credited')}</option>
              <option value='ORDER_LICENSE'>{t('Order or license')}</option>
              <option value='DOWNLOAD'>{t('Download')}</option>
              <option value='SOFTWARE'>{t('Software usage')}</option>
              <option value='ACCOUNT'>{t('Account')}</option>
              <option value='OTHER'>{t('Other')}</option>
            </select>
          </label>
          <label className='grid gap-2 text-sm'>
            <span>{t('Contact email')}</span>
            <input
              autoComplete='email'
              className='bg-background rounded-md border px-3 py-2'
              type='email'
              aria-invalid={Boolean(form.formState.errors.contact_email)}
              {...form.register('contact_email')}
            />
            {form.formState.errors.contact_email && (
              <span className='text-destructive' role='alert'>
                {t('Enter a valid email address.')}
              </span>
            )}
          </label>
          <label className='grid gap-2 text-sm'>
            <span>{t('Order number (optional)')}</span>
            <input
              autoComplete='off'
              className='bg-background rounded-md border px-3 py-2'
              maxLength={64}
              {...form.register('order_id')}
            />
          </label>
          <label className='grid gap-2 text-sm'>
            <span>{t('Subject')}</span>
            <input
              className='bg-background rounded-md border px-3 py-2'
              maxLength={200}
              aria-invalid={Boolean(form.formState.errors.subject)}
              {...form.register('subject')}
            />
            {form.formState.errors.subject && (
              <span className='text-destructive' role='alert'>
                {t('Enter a subject (up to 200 characters).')}
              </span>
            )}
          </label>
          <label className='grid gap-2 text-sm'>
            <span>{t('Describe the issue')}</span>
            <textarea
              className='bg-background min-h-40 rounded-md border px-3 py-2'
              maxLength={5000}
              aria-invalid={Boolean(form.formState.errors.message)}
              {...form.register('message')}
            />
            <span className='text-muted-foreground'>
              {t(
                'Do not include passwords, verification codes, or payment card details.'
              )}
            </span>
            {form.formState.errors.message && (
              <span className='text-destructive' role='alert'>
                {t('Describe the issue in 10 to 5000 characters.')}
              </span>
            )}
          </label>
          <label className='sr-only'>
            Website
            <input
              autoComplete='off'
              tabIndex={-1}
              {...form.register('website')}
            />
          </label>
          <p className='text-muted-foreground text-xs'>
            {t(
              'To handle your request, we store your contact email, any order number and message, and basic request details such as IP address and browser type. See the'
            )}{' '}
            <Link className='underline underline-offset-2' to='/privacy-policy'>
              {t('Privacy Policy')}
            </Link>
            .
          </p>
          {requestError && (
            <p
              aria-live='polite'
              className='text-destructive text-sm'
              role='alert'
            >
              {requestError}
            </p>
          )}
          <button
            className='bg-primary text-primary-foreground rounded-md px-5 py-2.5 hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 disabled:opacity-50'
            disabled={form.formState.isSubmitting}
            type='submit'
          >
            {form.formState.isSubmitting ? t('Sending…') : t('Submit message')}
          </button>
        </form>
      )}
    </main>
  )
}

export const Route = createFileRoute('/support')({
  beforeLoad: async () => {
    await resolveAuthentication()
    const { auth } = useAuthStore.getState()
    if (!auth.user || !auth.accessToken) {
      throw redirect({ to: '/sign-in', search: { redirect: '/support' } })
    }
  },
  component: D2SSupportPage,
})
