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
  const handledKey = useRef('')
  const { isAnnouncementRead, markAnnouncementsRead } = useNotificationStore()

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
    if (!isAnnouncementRead(announcementKey)) {
      setOpen(true)
    }
  }, [announcement, announcementKey, isAnnouncementRead, loading])

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (!nextOpen && announcementKey) {
      markAnnouncementsRead([announcementKey])
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
          <span className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-full'>
            <Megaphone className='size-4' />
          </span>
          {t('System Announcement')}
        </span>
      }
      description={
        announcement.publishDate
          ? formatDateTimeObject(new Date(announcement.publishDate))
          : t('Latest platform updates and notices')
      }
      contentClassName='sm:max-w-xl shadow-2xl'
      overlayClassName='bg-black/45 supports-backdrop-filter:backdrop-blur-sm'
      bodyClassName='space-y-4'
    >
      <ScrollArea className='max-h-[min(62vh,560px)] pr-3'>
        <div className='prose prose-sm dark:prose-invert max-w-none'>
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
