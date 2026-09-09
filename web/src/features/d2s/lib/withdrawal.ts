import { z } from 'zod'

export function createD2SWithdrawalFormSchema(minWithdrawalMinor = 5000) {
  return z.object({
    amountMinor: z
      .number()
      .int('Amount must be a whole number')
      .min(
        minWithdrawalMinor,
        'Withdrawal amount is below the configured minimum'
      ),
    alipayAccount: z.string().trim().min(1, 'Alipay account is required'),
    realName: z.string().trim().min(1, 'Real name is required'),
  })
}

export const d2sWithdrawalFormSchema = createD2SWithdrawalFormSchema()

export type D2SWithdrawalFormValues = z.infer<typeof d2sWithdrawalFormSchema>
