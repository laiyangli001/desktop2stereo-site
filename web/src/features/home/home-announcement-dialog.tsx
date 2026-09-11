/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Megaphone } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useAnnouncements } from '@/features/dashboard/hooks/use-status-data'
import { getAnnouncementKey } from '@/hooks/use-notifications'
import { isLikelyHtml } from '@/lib/content-format'
import { formatDateTimeObject } from '@/lib/time'
import { useNotificationStore } from '@/stores/notification-store'

export function HomeAnnouncementDialog() {
  const { t } = useTranslation()
  const { items, loading } = useAnnouncements()
  const [open, setOpen] = useState(false)
  const [neverShowAgain, setNeverShowAgain] = useState(false)
  const handledKey = useRef('')
  const {
    dismissAnnouncements,
    isAnnouncementDismissed,
  } = useNotificationStore()

  const announcement = items[0]
  const announcementKey = useMemo(
    () =>
      announcement
        ? getAnnouncementKey(announcement as unknown as Record<string, unknown>)
        : '',
    [announcement]
  )

  useEffect(() => {
    if (
      loading ||
      !announcement ||
      !announcementKey ||
      handledKey.current === announcementKey
    ) {
      return
    }

    handledKey.current = announcementKey
    if (!isAnnouncementDismissed(announcementKey)) {
      setOpen(true)
    }
  }, [announcement, announcementKey, isAnnouncementDismissed, loading])

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (!nextOpen && announcementKey && neverShowAgain) {
      dismissAnnouncements([announcementKey])
    }
  }

  if (!announcement) {
    return null
  }

  const contentIsHtml = isLikelyHtml(announcement.content)

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={
        <span className='flex items-center gap-2'>
          <span className='flex size-10 items-center justify-center rounded-2xl bg-pink-500/10 text-pink-500'>
            <Megaphone className='size-4' />
          </span>
          {t('System Announcements')}
        </span>
      }
      description={
        announcement.publishDate
          ? formatDateTimeObject(new Date(announcement.publishDate))
          : t('Latest platform updates and notices')
      }
      contentClassName='max-w-[calc(100%-1.5rem)] gap-0 rounded-3xl border border-border/80 p-0 shadow-2xl sm:max-w-5xl [&_[data-slot=dialog-close]]:top-6 [&_[data-slot=dialog-close]]:right-6 [&_[data-slot=dialog-close]]:size-12 [&_[data-slot=dialog-close]]:rounded-2xl [&_[data-slot=dialog-close]]:border-2 [&_[data-slot=dialog-close]]:border-blue-200 [&_[data-slot=dialog-close]]:bg-background sm:[&_[data-slot=dialog-close]]:top-8 sm:[&_[data-slot=dialog-close]]:right-8'
      headerClassName='border-b bg-background px-6 py-6 sm:px-10 sm:py-8'
      titleClassName='text-2xl font-semibold tracking-tight sm:text-3xl'
      descriptionClassName='mt-2 text-base sm:text-lg'
      overlayClassName='bg-slate-950/55 supports-backdrop-filter:backdrop-blur-md'
      bodyClassName='p-0'
      footer={
        <div className='flex w-full flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
          <label className='text-muted-foreground flex cursor-pointer items-center gap-3 text-sm sm:text-base'>
            <Checkbox
              checked={neverShowAgain}
              onCheckedChange={(checked) => setNeverShowAgain(checked === true)}
              aria-label={t('Do not show automatically on this device')}
            />
            <span>
              {t('Do not show automatically on this device')}{' '}
              <span className='text-muted-foreground/80'>
                ({t('You can still view it from the notification center')})
              </span>
            </span>
          </label>
          <Button
            className='h-11 min-w-28 rounded-xl px-6 text-base font-semibold'
            onClick={() => setOpen(false)}
          >
            {t('Got it')}
          </Button>
        </div>
      }
      footerClassName='-mx-0 -mb-0 rounded-b-3xl px-6 py-5 sm:px-10 sm:py-6'
    >
      <ScrollArea className='max-h-[min(62vh,560px)] px-6 py-6 sm:px-10 sm:py-8'>
        <div className='prose prose-base dark:prose-invert max-w-none'>
          <RichContent
            content={announcement.content}
            mode={contentIsHtml ? 'html' : 'markdown'}
            htmlVariant='inline'
          />
          {announcement.extra ? (
            <div className='text-muted-foreground mt-4 border-t pt-4 text-sm'>
              <RichContent breaks content={announcement.extra} />
            </div>
          ) : null}
        </div>
      </ScrollArea>
    </Dialog>
  )
}
