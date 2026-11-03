import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getD2SMySupportTickets } from '@/features/d2s/api'
import { toIntlLocale } from '@/i18n/languages'

const statusLabels: Record<string, string> = {
  OPEN: 'Waiting for review',
  IN_PROGRESS: 'In progress',
  RESOLVED: 'Resolved',
  CLOSED: 'Closed',
}

const categoryLabels: Record<string, string> = {
  PAYMENT: 'Payment not credited',
  ORDER_LICENSE: 'Order or license',
  DOWNLOAD: 'Download',
  SOFTWARE: 'Software usage',
  ACCOUNT: 'Account',
  OTHER: 'Other',
}

export function D2SMySupportTickets() {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['d2s', 'my-support-tickets', page],
    queryFn: () => getD2SMySupportTickets(page),
  })
  const tickets = query.data?.data?.items ?? []
  const total = query.data?.data?.total ?? 0

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('My support tickets')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-4xl space-y-5'>
          <Link
            className='inline-block text-sm underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-4'
            to='/d2s'
          >
            {t('Back to authorization management')}
          </Link>
          {query.isPending && (
            <p aria-live='polite' role='status'>
              {t('Loading your support tickets…')}
            </p>
          )}
          {query.isError && (
            <p className='text-destructive' role='alert'>
              {t('Unable to load your support tickets.')}
            </p>
          )}
          {query.isSuccess && tickets.length === 0 && (
            <p
              aria-live='polite'
              className='rounded-lg border p-5'
              role='status'
            >
              {t('You have not submitted any tickets while signed in.')}
            </p>
          )}
          <div className='space-y-4'>
            {tickets.map((ticket) => (
              <article
                className='space-y-3 rounded-lg border p-5'
                key={ticket.id}
              >
                <header className='flex flex-wrap items-start justify-between gap-3'>
                  <div className='min-w-0 break-words'>
                    <h2 className='font-semibold break-words'>
                      {ticket.subject}
                    </h2>
                    <p className='text-muted-foreground text-xs'>
                      {t('Reference')}:{' '}
                      <code className='break-all'>{ticket.id}</code>
                    </p>
                  </div>
                  <span className='rounded-full border px-3 py-1 text-xs'>
                    {t(statusLabels[ticket.status] ?? ticket.status)}
                  </span>
                </header>
                <dl className='grid gap-2 text-sm sm:grid-cols-2'>
                  <div>
                    <dt className='text-muted-foreground'>{t('Category')}</dt>
                    <dd>
                      {t(categoryLabels[ticket.category] ?? ticket.category)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('Order number')}
                    </dt>
                    <dd className='break-all'>
                      {ticket.order_id || t('Not provided')}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>{t('Submitted')}</dt>
                    <dd>
                      {new Intl.DateTimeFormat(
                        toIntlLocale(i18n.resolvedLanguage || i18n.language),
                        {
                          dateStyle: 'medium',
                          timeStyle: 'short',
                          timeZone: 'UTC',
                        }
                      ).format(new Date(ticket.created_at * 1000))}{' '}
                      UTC
                    </dd>
                  </div>
                </dl>
                <p className='text-sm break-words whitespace-pre-wrap'>
                  {ticket.message}
                </p>
                {ticket.admin_reply && (
                  <div className='rounded-md border-l-4 p-4'>
                    <h3 className='mb-2 text-sm font-semibold'>
                      {t('Response from support')}
                    </h3>
                    <p className='text-sm break-words whitespace-pre-wrap'>
                      {ticket.admin_reply}
                    </p>
                  </div>
                )}
              </article>
            ))}
          </div>
          {total > 50 && (
            <nav
              aria-label={t('Support ticket pages')}
              className='flex items-center gap-3'
            >
              <button
                className='hover:bg-muted rounded border px-3 py-2 focus-visible:outline-2 focus-visible:outline-offset-2 disabled:opacity-50'
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
                type='button'
              >
                {t('Previous page')}
              </button>
              <span aria-live='polite' role='status'>
                {t('Page {{page}} of {{pages}}', {
                  page,
                  pages: Math.ceil(total / 50),
                })}
              </span>
              <button
                className='hover:bg-muted rounded border px-3 py-2 focus-visible:outline-2 focus-visible:outline-offset-2 disabled:opacity-50'
                disabled={page >= Math.ceil(total / 50) || query.isFetching}
                onClick={() => setPage((current) => current + 1)}
                type='button'
              >
                {t('Next page')}
              </button>
            </nav>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
