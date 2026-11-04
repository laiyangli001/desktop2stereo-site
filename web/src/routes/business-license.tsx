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
import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const copy = {
  en: {
    back: 'Back to home',
    company: 'Yantai Tulishi Software Development Co., Ltd.',
    description: 'Company registration information is shown below.',
    title: 'Business license',
  },
  zh: {
    back: '返回首页',
    company: '烟台图立视软件开发有限公司',
    description: '以下为本公司营业执照公示信息。',
    title: '营业执照公示',
  },
}

function BusinessLicensePage() {
  const { i18n } = useTranslation()
  const text =
    i18n.language === 'zhCN' || i18n.language.startsWith('zh')
      ? copy.zh
      : copy.en

  return (
    <main className='bg-background text-foreground min-h-screen'>
      <div className='mx-auto max-w-5xl px-4 py-8 sm:px-6 md:py-12'>
        <nav aria-label={text.back}>
          <Link
            to='/'
            className='text-primary hover:text-primary/80 inline-flex min-h-11 items-center gap-2 rounded-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-4'
          >
            <ArrowLeft aria-hidden='true' className='size-4' />
            {text.back}
          </Link>
        </nav>

        <header className='my-8 space-y-3'>
          <p className='text-primary text-sm font-semibold'>{text.company}</p>
          <h1 className='text-3xl font-bold tracking-tight sm:text-4xl'>
            {text.title}
          </h1>
          <p className='text-muted-foreground'>{text.description}</p>
        </header>

        <figure className='bg-card overflow-hidden rounded-xl border p-2 shadow-sm sm:p-4'>
          <img
            src='/business-license.png'
            alt={text.company}
            className='mx-auto block h-auto w-full rounded-md object-contain'
            decoding='async'
          />
        </figure>
      </div>
    </main>
  )
}

export const Route = createFileRoute('/business-license')({
  component: BusinessLicensePage,
})
