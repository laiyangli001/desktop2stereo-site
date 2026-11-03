import { toIntlLocale } from '@/i18n/languages'

export function formatD2SMoney(
  amountMinor: number,
  currency: string,
  locale?: string
): string {
  const normalizedCurrency = currency.toUpperCase()
  if (!/^[A-Z]{3}$/.test(normalizedCurrency)) {
    const amount = new Intl.NumberFormat(toIntlLocale(locale), {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amountMinor / 100)
    return `${currency} ${amount}`
  }

  return new Intl.NumberFormat(toIntlLocale(locale), {
    style: 'currency',
    currency: normalizedCurrency,
    currencyDisplay: 'code',
  })
    .formatToParts(amountMinor / 100)
    .map((part) => (part.type === 'literal' ? ' ' : part.value))
    .join('')
}

export function formatD2SDate(timestamp: number, locale?: string): string {
  if (!timestamp) return '—'
  return new Intl.DateTimeFormat(toIntlLocale(locale), {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp * 1000))
}
