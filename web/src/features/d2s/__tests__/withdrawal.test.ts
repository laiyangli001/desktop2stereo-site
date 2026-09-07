import { describe, expect, it } from 'vitest'

import { d2sWithdrawalFormSchema } from '../lib/withdrawal'

describe('d2sWithdrawalFormSchema', () => {
  it('accepts a valid minimum withdrawal', () => {
    const result = d2sWithdrawalFormSchema.safeParse({
      amountMinor: 5000,
      alipayAccount: 'user@example.com',
      realName: 'Test User',
    })

    expect(result.success).toBe(true)
  })

  it('rejects amounts below the server minimum', () => {
    const result = d2sWithdrawalFormSchema.safeParse({
      amountMinor: 4999,
      alipayAccount: 'user@example.com',
      realName: 'Test User',
    })

    expect(result.success).toBe(false)
  })

  it('rejects missing payout identity fields', () => {
    const result = d2sWithdrawalFormSchema.safeParse({
      amountMinor: 5000,
      alipayAccount: ' ',
      realName: '',
    })

    expect(result.success).toBe(false)
  })
})
