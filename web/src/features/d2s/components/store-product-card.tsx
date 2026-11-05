import { Check, CheckCircle2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'

import type { D2SStoreProduct } from '../api'
import { formatD2SMoney } from '../lib/format'

import { STORE_PRODUCT_CONTENT } from '../lib/store-products'

export function StoreProductCard(props: {
  product: D2SStoreProduct
  selected: boolean
  disabled: boolean
  onSelect: () => void
  currency: 'CNY' | 'USD'
}) {
  const { t, i18n } = useTranslation()
  const content = STORE_PRODUCT_CONTENT[props.product.id]
  const amount = props.product.prices[props.currency]
  const otherCurrency = props.currency === 'CNY' ? 'USD' : 'CNY'
  const otherAmount = props.product.prices[otherCurrency]
  const locale = i18n.resolvedLanguage || i18n.language
  const displayAmount =
    Number.isSafeInteger(amount) && (amount ?? 0) > 0
      ? new Intl.NumberFormat(toIntlLocale(locale), {
          style: 'currency',
          currency: props.currency,
          currencyDisplay: 'narrowSymbol',
        }).format((amount ?? 0) / 100)
      : t('Price unavailable')
  return (
    <label
      className={`relative flex min-w-0 cursor-pointer flex-col overflow-hidden rounded-xl border-2 bg-card transition-colors focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 ${props.selected ? 'border-primary shadow-sm' : 'border-border hover:border-primary/50'} ${props.disabled ? 'cursor-wait opacity-70' : ''}`}
    >
      <input
        type='radio'
        name='store-product'
        value={props.product.id}
        aria-label={t(content.title)}
        className='sr-only'
        checked={props.selected}
        disabled={props.disabled}
        onChange={props.onSelect}
      />
      <div
        className={`flex items-center justify-between gap-2 px-5 py-3 text-sm font-semibold ${props.selected ? 'bg-primary text-primary-foreground' : 'bg-muted/60'}`}
      >
        <span>{t(content.title)}</span>
        {props.product.id === 'license' && (
          <span className='rounded-full border border-current/20 px-2 py-0.5 text-[10px]'>
            {t('Recommended')}
          </span>
        )}
      </div>
      <div className='flex flex-1 flex-col p-5'>
        <div className='space-y-1 py-3'>
          {props.product.starting_at && (
            <p className='text-muted-foreground text-xs'>{t('Starting at')}</p>
          )}
          <p className='break-words text-3xl font-semibold tracking-tight tabular-nums'>
            {displayAmount}
          </p>
          {Number.isSafeInteger(otherAmount) && (otherAmount ?? 0) > 0 && (
            <p className='text-muted-foreground text-xs'>
              {formatD2SMoney(otherAmount ?? 0, otherCurrency, locale)}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>{t(content.unit)}</p>
        </div>
        <p className='text-muted-foreground mt-3 min-h-10 text-sm leading-6'>
          {t(content.description)}
        </p>
        <ul className='my-5 space-y-3 text-sm'>
          {content.features.map((feature) => (
            <li key={feature} className='flex items-start gap-2'>
              <Check
                className='mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400'
                aria-hidden='true'
              />
              <span>{t(feature)}</span>
            </li>
          ))}
        </ul>
      </div>
      <div
        className={`flex items-center justify-center gap-2 px-5 py-3 text-sm font-medium ${props.selected ? 'bg-primary/10 text-primary' : 'border-t bg-muted/20'}`}
      >
        {props.selected && (
          <CheckCircle2 className='size-4' aria-hidden='true' />
        )}
        {props.selected ? t('Selected') : t('Select product')}
      </div>
    </label>
  )
}
