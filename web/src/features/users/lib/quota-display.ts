/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'

/** Convert stored quota units to the configured amount without exchange rates. */
export function quotaUnitsToUserAmount(units: number): number {
  const { config, meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') return units
  return config.quotaPerUnit > 0 ? units / config.quotaPerUnit : 0
}

/** Format a user quota amount without applying CNY/USD exchange rates. */
export function formatUserQuotaAmount(units: number): string {
  const amount = quotaUnitsToUserAmount(units)
  return `${amount.toLocaleString(undefined, { maximumFractionDigits: 6 })} ${getCurrencyLabel()}`
}
