import { z } from 'zod'

export const d2sWithdrawalFormSchema = z.object({
  amountMinor: z
    .number()
    .int('Amount must be a whole number')
    .min(5000, 'Minimum withdrawal is 5000 minor units'),
  alipayAccount: z.string().trim().min(1, 'Alipay account is required'),
  realName: z.string().trim().min(1, 'Real name is required'),
})

export type D2SWithdrawalFormValues = z.infer<typeof d2sWithdrawalFormSchema>
