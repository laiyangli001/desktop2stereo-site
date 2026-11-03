import { Link, useRouterState } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

export function D2SSupportFloatingButton() {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const [openPath, setOpenPath] = useState<string | null>(null)
  const open = openPath === pathname
  const [wechatURL, setWechatURL] = useState('')

  useEffect(() => {
    let active = true
    fetch('/api/v1/support/config', { cache: 'no-store' })
      .then((response) => response.json())
      .then((result: { data?: { enterprise_wechat_url?: string } }) => {
        if (active && typeof result.data?.enterprise_wechat_url === 'string') {
          setWechatURL(result.data.enterprise_wechat_url)
        }
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [])

  if (pathname.startsWith('/d2s-admin-console')) return null

  return (
    <aside className='fixed top-1/2 right-4 z-50 -translate-y-1/2'>
      {open && (
        <section
          aria-label={t('Support contact options')}
          className='bg-background mb-3 w-64 space-y-3 rounded-xl border p-4 shadow-xl'
        >
          <h2 className='font-semibold'>{t('How can we help?')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Sign in with a registered account to submit a ticket. Enterprise WeChat is also available.'
            )}
          </p>
          <Link
            className='hover:bg-muted block rounded-md border px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-offset-2'
            to='/support'
          >
            {t('Submit a complaint or question')}
          </Link>
          {wechatURL ? (
            <a
              className='bg-primary text-primary-foreground block rounded-md px-3 py-2 text-center text-sm hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2'
              href={wechatURL}
              rel='noopener noreferrer'
              target='_blank'
            >
              {t('Add enterprise WeChat support')}
            </a>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t('Enterprise WeChat support link is not configured yet.')}
            </p>
          )}
        </section>
      )}
      <button
        aria-expanded={open}
        aria-label={
          open ? t('Close support options') : t('Complaints and support')
        }
        className='bg-primary text-primary-foreground ml-auto flex min-h-12 items-center justify-center rounded-full px-4 text-sm font-medium shadow-lg hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2'
        onClick={() =>
          setOpenPath((current) => (current === pathname ? null : pathname))
        }
        type='button'
      >
        {t('Complaints and support')}
      </button>
    </aside>
  )
}
